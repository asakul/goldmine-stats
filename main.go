package main

import ("log"
		"cppio"
		"sync"
		"fmt"
		"time"
		"./goldmine"
		"./db"
		"./handlers"
		"encoding/json"
		"net/http"
		_ "github.com/mattn/go-sqlite3"
		"gopkg.in/tomb.v2"
		"github.com/paked/configure")

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

func convertTrade(t JsonTradeFields) (goldmine.Trade, error) {
	// If 'operation' is 'sell', then we should negate quantity field
	var quantityFactor int
	if t.Operation == "buy" {
		quantityFactor = 1
	} else if t.Operation == "sell" {
		quantityFactor = -1
	} else {
		return goldmine.Trade{}, fmt.Errorf("Error while parsing JSON: invalid 'operation' field: [%s]", t.Operation)
	}
	ts, err := time.Parse("2006-01-02 15:04:05.000", t.ExecutionTime)
	if err != nil {
		return goldmine.Trade {}, err
	}
	return goldmine.Trade {Account : t.Account,
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

func handleClient(client cppio.IoLine, trades chan goldmine.Trade, t *tomb.Tomb, wg sync.WaitGroup) {
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

func listenClients(endpoint string, trades chan goldmine.Trade, t *tomb.Tomb, wg sync.WaitGroup) error {
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


func httpServer(dbFilename string, t *tomb.Tomb) {
	trades := handlers.TradesHandler {dbFilename}
	http.Handle("/trades/", trades)
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("content/static"))))
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

	trades := make(chan goldmine.Trade)
	var wg sync.WaitGroup
	var theTomb tomb.Tomb

	wg.Add(2)
	go db.WriteDatabase(*dbFilename, trades, &theTomb, wg)
	go listenClients(*endpoint, trades, &theTomb, wg)
	go httpServer(*dbFilename, &theTomb)

	wg.Wait()
}
