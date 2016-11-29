
package main

import ("encoding/json"
	"os"
	"fmt"
	zmq "github.com/pebbe/zmq4"
	"github.com/jessevdk/go-flags"
)

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

type Options struct {
	Endpoint string `short:"e" long:"endpoint"`
	Account string `short:"a" long:"account"`
	Security string `short:"s" long:"security"`
	Price float64 `short:"p" long:"price"`
	Quantity int `short:"q" long:"quantity"`
	Operation string `short:"o" long:"operation"`
	VolumeCurrency string `short:"c" long:"currency"`
	PointPrice float64 `short:"r" long:"pprice"`
	ExecutionTime string `short:"t" long:"time"`
	Strategy string `long:"strategy"`
	Signal string `long:"signal"`
	Comment string `long:"comment"`
}

func main() {
	var options Options
	_, err := flags.Parse(&options)
	if err != nil {
		panic(err)
		os.Exit(1)
	}

	if options.PointPrice == 0 {
		options.PointPrice = 1
	}

	absQuantity := options.Quantity
	if options.Operation == "sell" {
		options.Quantity = -options.Quantity
	} else if options.Operation != "buy" {
		panic(fmt.Errorf("Invalid operation, should be either 'buy' or 'sell, got %s", options.Operation))
		os.Exit(1)
	}
	trade := JsonTrade { JsonTradeFields {
		Account : options.Account,
		Security : options.Security,
		Price : options.Price,
		Quantity : absQuantity,
		Volume : options.Price * float64(absQuantity) * options.PointPrice,
		VolumeCurrency : options.VolumeCurrency,
		Operation : options.Operation,
		ExecutionTime : options.ExecutionTime,
		Strategy : options.Strategy,
		Signal_id : options.Signal,
		Order_comment : options.Comment}}
	b, jsonErr := json.Marshal(trade)
	if jsonErr != nil {
		panic(jsonErr)
	}

	client, err := zmq.NewSocket(zmq.REQ)
	if err != nil {
		panic(err)
	}

	err = client.Connect(options.Endpoint)
	defer client.Close()
	if err != nil {
		panic(err)
	}
	fmt.Printf("Sending: %s", b)
	msg := make([]string, 1)
	msg[0] = string(b)
	client.SendMessage(msg)

	msg, err = client.RecvMessage(0)
	if err != nil {
		panic(err)
	}
	fmt.Printf("Got response")
}
