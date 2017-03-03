package main

import ("log"
		"os"
		"sync"
		"fmt"
		"time"
		"./goldmine"
		"./db"
		"./handlers"
		"encoding/json"
		"net/http"
		zmq "github.com/pebbe/zmq4"
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

func sendHeartbeatResponse(peerId string, socket* zmq.Socket) {
	msg := make([]string, 3)
	msg[0] = peerId
	msg[2] = "{ \"response\" : \"ok\"}"
	socket.SendMessage(msg)
}

func handleClient(server* zmq.Socket, trades chan goldmine.Trade, t *tomb.Tomb, wg sync.WaitGroup) {
	wg.Add(1)
	defer wg.Done()
	//log.Printf("Waiting for next message")

	msg, err := server.RecvMessage(0)
	if err != nil {
		return
	}
	//log.Printf("Incoming message")

	if len(msg) >= 3 {
		//log.Printf("Incoming json: %s", msg[2])
		var incomingMessage interface{}

		jsonErr := json.Unmarshal([]byte(msg[2]), &incomingMessage)
		if jsonErr != nil {
			log.Printf("Error: unable to parse incoming JSON: %s", jsonErr.Error())
			return
		}
		msgMap := incomingMessage.(map[string]interface{})
		if cmd, ok := msgMap["command"]; ok {
			switch cmd.(type) {
			case string:
				sendHeartbeatResponse(msg[0], server)
			default:
				log.Printf("Invalid cmd field")

			}
		} else if _, ok := msgMap["trade"]; ok {
			log.Printf("Incoming trade")
			var trade JsonTrade
			err := json.Unmarshal([]byte(msg[2]), &trade)
			if err != nil {
				log.Printf("Trade parsing error: %s", err.Error())
				return
			}
			log.Printf("Trade: sec: %s/account: %s", trade.Trade.Security, trade.Trade.Account)
			parsedTrade, err := convertTrade(trade.Trade)
			if err != nil {
				log.Printf("Trade parsing error: %s", err.Error())
				return
			}
			log.Printf("Trade parsed")
			trades <- parsedTrade
		}

	} else {
		log.Printf("Error: invalid message size: %d", len(msg))
		return
	}
}

func listenClients(endpoint string, trades chan goldmine.Trade, t *tomb.Tomb, wg sync.WaitGroup) error {
	defer wg.Done()
	ctx, err := zmq.NewContext()
	if err != nil {
		return err
	}
	server, err := ctx.NewSocket(zmq.ROUTER)
	if err != nil {
		return err
	}

	log.Printf("Listening on %s", endpoint)
	server.Bind(endpoint)

	for {
		if !t.Alive() {
			return nil
		}

		handleClient(server, trades, t, wg)
	}
}

func httpServer(dbHandle *db.DbHandle, t *tomb.Tomb, contentDir string) {
	http.Handle("/delete_trade", handlers.DeleteTradeHandler {dbHandle, contentDir})
	http.Handle("/trades/", handlers.TradesHandler {dbHandle, contentDir})
	http.Handle("/closed_trades/", handlers.ClosedTradesHandler {dbHandle, contentDir})
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir(contentDir + "/content/static"))))
	log.Printf("HTTP: Listening on 5541")
	http.ListenAndServe(":5541", nil)
}

func main () {
	conf := configure.New()
	dbFilename := conf.String("db-filename", "trades.db", "Where database will be stored")
	endpoint := conf.String("endpoint", "", "What endpoint to listen")
	contentDir := conf.String("content-dir", ".", "Directory where static content and templates are stored")
	conf.Use(configure.NewEnvironment())
	conf.Use(configure.NewFlag())
	if _, err := os.Stat("/etc/goldmine-stats-config.json"); err == nil {
		conf.Use(configure.NewJSONFromFile("/etc/goldmine-stats-config.json"))
	}
	conf.Parse()

	trades := make(chan goldmine.Trade)
	var wg sync.WaitGroup
	var theTomb tomb.Tomb

	dbHandle, err := db.Open(*dbFilename)
	if err != nil {
		log.Printf("Error: unable to open database: %s", err)
	}
	defer db.Close(dbHandle)
	wg.Add(2)
	go db.WriteDatabase(dbHandle, trades, &theTomb, wg)
	go listenClients(*endpoint, trades, &theTomb, wg)
	go httpServer(dbHandle, &theTomb, *contentDir)

	wg.Wait()
}
