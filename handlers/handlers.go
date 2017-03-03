
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
	trades := db.ReadAllTrades(handler.Db, "")
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

func sign(value int) float64 {
	if value >= 0 {
		return 1
	} else {
		return -1
	}
}

type DataPoint struct {
	Year int
	Month int
	Day int
	Hour int
	Minute int
	Second int
	Value float64
}
type ProfitSeries struct {
	Name string
	Points []DataPoint
}

func makeCumulativePnLForAccount(account string, trades []db.ClosedTrade) ProfitSeries {
	var result ProfitSeries
	result.Name = account
	current := 0.0
	for _, trade := range(trades) {
		if trade.Account == account {
			current += trade.Profit
			result.Points = append(result.Points, DataPoint { trade.ExitTime.Year(), int(trade.ExitTime.Month()), trade.ExitTime.Day(), trade.ExitTime.Hour(), trade.ExitTime.Minute(), trade.ExitTime.Second(), current})
		}
	}
	return result
}

func makeCumulativePnL(accounts []string, trades []db.ClosedTrade) []ProfitSeries {
	var profits []ProfitSeries
	for _, account := range(accounts) {
		profits = append(profits, makeCumulativePnLForAccount(account, trades))
	}
	return profits
}

func (handler ClosedTradesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	log.Printf("ClosedTrades handler")
	type ClosedTradesPageData struct {
		Title string
		Trades []db.ClosedTrade
		Accounts []string
		CurrentAccount string
		CumulativeProfits []ProfitSeries
	}
	accounts, err := db.GetAllAccounts(handler.Db)
	if err != nil {
		log.Printf("Unable to obtain accounts: %s", err.Error())
		return
	}
	currentAccount := r.FormValue("account")

	err = db.BalanceTrades(handler.Db)
	if err != nil {
		log.Printf("Unable to balance trades: %s", err.Error())
	}
	trades, err := db.GetAllClosedTrades(handler.Db)
	if err != nil {
		log.Printf("Unable to obtain trades: %s", err.Error())
		return
	}
	if currentAccount != "" {
		filteredTrades := make([]db.ClosedTrade, 0)
		for _, trade := range trades {
			if trade.Account == currentAccount {
				filteredTrades = append(filteredTrades, trade)
			}
		}
		trades = filteredTrades
	}

	cumulativePnL := makeCumulativePnL(accounts, trades)

	page := ClosedTradesPageData { "Closed trades", trades, accounts, currentAccount, cumulativePnL }
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
