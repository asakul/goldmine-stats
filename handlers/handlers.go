
package handlers

import ("../db"
		"../goldmine"
		"html/template"
		"time"
		"log"
		"net/http")

type TradesHandler struct {
	DbFilename string
}

func (handler TradesHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	type TradesPageData struct {
		Title string
		Trades []goldmine.Trade
	}
	trades := db.ReadAllTrades(handler.DbFilename)
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
