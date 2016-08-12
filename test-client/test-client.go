
package main

import ("encoding/json"
	"cppio"
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

func main() {
	trade := JsonTrade { JsonTradeFields {
		Account : "foo",
		Security : "bar",
		Price : 10,
		Quantity : 2,
		Volume : 10000,
		VolumeCurrency : "RUB",
		Operation : "buy",
		ExecutionTime : "2000-01-01 12:00:00.333",
		Strategy : "foobar",
		Signal_id : "signal" }}
	b, jsonErr := json.Marshal(trade)
	if jsonErr != nil {
		panic(jsonErr)
	}

	client, err := cppio.CreateClient("tcp://127.0.0.1:5540")
	if err != nil {
		panic(err)
	}

	proto := cppio.CreateMessageProtocol(client)
	msg := cppio.CreateMessage()
	msg.AddFrame(b)
	proto.Send(msg)
}
