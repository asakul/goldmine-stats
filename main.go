package main

import ("database/sql"
		"log"
		"cppio"
		"sync"
		"fmt"
		"time"
		"encoding/json"
		"net/http"
		"html/template"
		_ "github.com/mattn/go-sqlite3"
		"gopkg.in/tomb.v2"
		"github.com/paked/configure")

type Trade struct {
	Account string
	Security string
	Price float64
	Quantity int // Positive value - buy, negative - sell
	Volume float64
	VolumeCurrency string
	StrategyId string
	SignalId string
	Comment string
	Timestamp uint64
	Useconds uint32
}

type JsonTradeFields struct {
	Account string `json:"account"`
	Security string `json:"security"`
	Price float64 `json:"price"`
	Quantity int `json:"quantity"`
	Volume float64 `json:"volume"`
	VolumeCurrency string `json:"volume-currency"`
	Operation string `json:"operation"`
	ExecutionTime string `json:"execution-time"`
	Strategy string `json:"strategy"`
	Signal_id string `json:"signal-id"`
	Order_comment string `json:"order-comment"`
}

type JsonTrade struct {
	Trade JsonTradeFields `json:"trade"`
}

func convertTrade(t JsonTradeFields) (Trade, error) {
	// If 'operation' is 'sell', then we should negate quantity field
	var quantityFactor int
	if t.Operation == "buy" {
		quantityFactor = 1
	} else if t.Operation == "sell" {
		quantityFactor = -1
	} else {
		return Trade{}, fmt.Errorf("Error while parsing JSON: invalid 'operation' field: [%s]", t.Operation)
	}
	ts, err := time.Parse("2006-01-02 15:04:05.000", t.ExecutionTime)
	if err != nil {
		return Trade {}, err
	}
	return Trade {Account : t.Account,
		Security : t.Security,
		Price : t.Price,
		Quantity : t.Quantity * quantityFactor,
		Volume : t.Volume,
		VolumeCurrency : t.VolumeCurrency,
		StrategyId : t.Strategy,
		SignalId : t.Signal_id,
		Comment : t.Order_comment,
		Timestamp : uint64(ts.Unix()),
		Useconds : uint32(ts.Nanosecond() / 1000)}, nil
}

func insertTrade(db *sql.DB, trade Trade) error {
	stmt, err := db.Prepare("INSERT INTO trades(account, security, price, quantity, volume, volumeCurrency, strategyId, signalId, comment, timestamp, useconds) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
	if err != nil {
		return err
	}

	_, err = stmt.Exec(trade.Account, trade.Security, trade.Price, trade.Quantity, trade.Volume, trade.VolumeCurrency, trade.StrategyId, trade.SignalId,
		trade.Comment, trade.Timestamp, trade.Useconds)

	if err != nil {
		return err
	}

	return nil
}

func createSchema(db *sql.DB) error {
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS trades(id INTEGER PRIMARY KEY, account TEXT, security TEXT, price REAL, quantity INTEGER, volume REAL, volumeCurrency TEXT, strategyId TEXT, signalId TEXT, comment TEXT, timestamp INTEGER, useconds INTEGER)")
	if err != nil {
		return err
	}
	return nil
}

func writeDatabase(dbFilename string, trades chan Trade, t *tomb.Tomb, wg sync.WaitGroup) {
	defer wg.Done()
	db, err := sql.Open("sqlite3", dbFilename)
	if err != nil {
		log.Fatalf("Unable to open database: %s", err.Error())
	}
	defer db.Close()
	err = createSchema(db)
	if err != nil {
		log.Fatalf("Unable to ping database: %s", err.Error())
	}
	for {
		select {
		case trade := <-trades:
			err = insertTrade(db, trade)
			if err != nil {
				log.Print(err.Error())
			}
		case <-t.Dying():
			return
		}
	}
}

func handleClient(client cppio.IoLine, trades chan Trade, t *tomb.Tomb, wg sync.WaitGroup) {
	defer client.Close()

	wg.Add(1)
	defer wg.Done()

	client.SetOptionInt(cppio.OReceiveTimeout, 500)

	proto := cppio.CreateMessageProtocol(client)
	defer proto.Close()

	for {
		if !t.Alive() {
			return
		}
		msg := cppio.CreateMessage()
		err := proto.Read(msg)
		if err != nil && !err.Timeout() {
			log.Printf("Error: %s", err.Error())
			break
		}
		if msg.Size() >= 1 {
			log.Printf("Incoming json: %s", msg.GetFrame(0))
			var trade JsonTrade
			jsonErr := json.Unmarshal(msg.GetFrame(0), &trade)
			if jsonErr != nil {
				log.Printf("Error: unable to parse incoming JSON: %s", jsonErr.Error())
				continue
			}

			log.Printf("Trade: sec: %s/account: %s", trade.Trade.Security, trade.Trade.Account)
			parsedTrade, err := convertTrade(trade.Trade)
			if err != nil {
				log.Printf("%s", err.Error())
				continue
			}
			trades <- parsedTrade

		} else {
			log.Printf("Error: invalid message size: %d", msg.Size())
			continue
		}
	}
}

func listenClients(endpoint string, trades chan Trade, t *tomb.Tomb, wg sync.WaitGroup) error {
	defer wg.Done()
	server, err := cppio.CreateServer(endpoint)
	if err != nil {
		return err
	}
	log.Printf("Listening on %s", endpoint)

	for {
		if !t.Alive() {
			return nil
		}

		client := server.WaitConnection(500)
		if client.IsValid() {
			go handleClient(client, trades, t, wg)
		}
	}
}

func readAllTrades(dbFilename string) []Trade {
	db, err := sql.Open("sqlite3", dbFilename)
	if err != nil {
		log.Fatalf("Unable to open database: %s", err.Error())
	}

	var trades []Trade
	rows, err := db.Query("SELECT account, security, price, quantity, volume, volumeCurrency, strategyId, signalId, comment, timestamp, useconds FROM trades")
	if err != nil {
		log.Printf("Unable to open DB: %s", err.Error())
		return trades
	}
	defer rows.Close()
	for rows.Next() {
		var t Trade
		err = rows.Scan(&t.Account, &t.Security, &t.Price, &t.Quantity, &t.Volume, &t.VolumeCurrency, &t.StrategyId, &t.SignalId, &t.Comment, &t.Timestamp, &t.Useconds)
		if err != nil {
			log.Printf("Unable to get trades: %s", err.Error())
			return trades
		}
		trades = append(trades, t)
	}

	return trades
}

type IndexHandler struct {
	dbFilename string
}

func (handler IndexHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	type IndexPageData struct {
		Title string
		Trades []Trade
	}
	trades := readAllTrades(handler.dbFilename)
	page := IndexPageData { "Index", trades }
	t, err := template.New("index.html").Funcs(template.FuncMap {
		"Abs" : func (a int) int {
		if a < 0 {
			return -a
		} else {
			return a
		}},
		"ConvertTime" : func (t uint64, us uint32) string {
			return time.Unix(int64(t), int64(us) * 1000).Format("2006-01-02 15:04:05.000")
		}}).ParseFiles("content/templates/index.html")
	if err != nil {
		log.Printf("Unable to parse template: %s", err.Error())
		return
	}
	err = t.Execute(w, page)
	if err != nil {
		log.Printf("Unable to execute template: %s", err.Error())
	}
}

func httpServer(dbFilename string, t *tomb.Tomb) {
	index := IndexHandler {dbFilename}
	http.Handle("/", index)
	http.ListenAndServe(":5541", nil)
}

func main () {
	conf := configure.New()
	dbFilename := conf.String("db-filename", "trades.db", "Where database will be stored")
	endpoint := conf.String("endpoint", "", "What endpoint to listen")
	conf.Use(configure.NewEnvironment())
	conf.Use(configure.NewFlag())
	conf.Use(configure.NewJSONFromFile("goldmine-stats-config.json"))
	conf.Parse()

	trades := make(chan Trade)
	var wg sync.WaitGroup
	var theTomb tomb.Tomb

	wg.Add(2)
	go writeDatabase(*dbFilename, trades, &theTomb, wg)
	go listenClients(*endpoint, trades, &theTomb, wg)
	go httpServer(*dbFilename, &theTomb)

	wg.Wait()
}
