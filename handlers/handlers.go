
package handlers

import ("../db"
		"../goldmine"
		"html/template"
		"time"
		"log"
		"strconv"
		"net/http")

type TradesHandler struct {
	Db *db.DbHandle
	ContentDir string
}

func (handler TradesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	type TradesPageData struct {
		Title string
		Trades []goldmine.Trade
	}
	trades := db.ReadAllTrades(handler.Db)
	if len(trades) >= 2 {
		for i := 0; i < len(trades) / 2; i++ {
			a, b := i, len(trades) - i - 1
			if a == b {
				break
			} else {
				x, y := trades[a], trades[b]
				trades[a], trades[b] = y, x
			}
		}
	}

	page := TradesPageData { "Index", trades }
	t, err := template.New("index.html").Funcs(template.FuncMap {
		"Abs" : func (a int) int {
		if a < 0 {
			return -a
		} else {
			return a
		}},
		"ConvertTime" : func (t uint64, us uint32) string {
			return time.Unix(int64(t), int64(us) * 1000).Format("2006-01-02 15:04:05.000")
		}}).ParseFiles(handler.ContentDir + "/content/templates/index.html",
	handler.ContentDir + "/content/templates/navbar.html")
	if err != nil {
		log.Printf("Unable to parse template: %s", err.Error())
		return
	}
	err = t.Execute(w, page)
	if err != nil {
		log.Printf("Unable to execute template: %s", err.Error())
	}
}

type ClosedTradesHandler struct {
	Db *db.DbHandle
	ContentDir string
}

type ClosedTrade struct {
	Account string
	Security string
	EntryTime time.Time
	ExitTime time.Time
	Profit float64
	ProfitCurrency string
	Strategy string
	Direction string
}

func sign(value int) float64 {
	if value >= 0 {
		return 1
	} else {
		return -1
	}
}

func aggregateClosedTrades(trades []goldmine.Trade) []ClosedTrade {
	var result []ClosedTrade

	type BalanceKey struct {
		Account string
		Security string
		Strategy string
	}
	type BalanceEntry struct {
		balance int
		trade ClosedTrade
	}
	balance := make(map[BalanceKey]BalanceEntry)

	for _, trade := range trades {
		key := BalanceKey { trade.Account, trade.Security, trade.StrategyId }
		balanceEntry := balance[key]
		log.Printf("Trade: %s %d", trade.Security, trade.Quantity)
		log.Printf("Balance: %d", balanceEntry.balance)
		if balanceEntry.balance == 0 {
			balanceEntry.balance = trade.Quantity
			balanceEntry.trade.Account = trade.Account
			balanceEntry.trade.Security = trade.Security
			balanceEntry.trade.EntryTime = time.Unix(int64(trade.Timestamp), int64(trade.Useconds))
			balanceEntry.trade.ProfitCurrency = trade.VolumeCurrency
			balanceEntry.trade.Profit = -trade.Volume * sign(trade.Quantity)
			balanceEntry.trade.Strategy = trade.StrategyId
			if trade.Quantity > 0 {
				balanceEntry.trade.Direction = "long"
			} else {
				balanceEntry.trade.Direction = "short"
			}
			balance[key] = balanceEntry
		} else {
			balanceEntry.balance += trade.Quantity
			balanceEntry.trade.Profit += -trade.Volume * sign(trade.Quantity)

			if balanceEntry.balance == 0 {
				balanceEntry.trade.ExitTime = time.Unix(int64(trade.Timestamp), int64(trade.Useconds))
				result = append(result, balanceEntry.trade)
			}
			balance[key] = balanceEntry
		}
	}

	return result
}

func (handler ClosedTradesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	type ClosedTradesPageData struct {
		Title string
		Trades []ClosedTrade
	}
	trades := aggregateClosedTrades(db.ReadAllTrades(handler.Db))

	page := ClosedTradesPageData { "Closed trades", trades }
	t, err := template.New("closed_trades.html").Funcs(template.FuncMap {
		"Abs" : func (a int) int {
		if a < 0 {
			return -a
		} else {
			return a
		}},
		"PrintTime" : func (t time.Time) string {
			return t.Format("2006-01-02 15:04:05.000")
		}}).ParseFiles(handler.ContentDir + "/content/templates/closed_trades.html",
	handler.ContentDir + "/content/templates/navbar.html")
	if err != nil {
		log.Printf("Unable to parse template: %s", err.Error())
		return
	}
	err = t.Execute(w, page)
	if err != nil {
		log.Printf("Unable to execute template: %s", err.Error())
	}
}

type DeleteTradeHandler struct {
	Db *db.DbHandle
	ContentDir string
}

func (handler DeleteTradeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.FormValue("id"))
	if err != nil {
		w.WriteHeader(403)
		w.Write([]byte("Error"))
	}
	db.DeleteTrade(handler.Db, id)
	http.Redirect(w, r, "/trades", 302)
}
