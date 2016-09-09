
package db

import ("database/sql"
		"sync"
		"log"
		"../goldmine"
		"gopkg.in/tomb.v2"
	)

func insertTrade(db *sql.DB, trade goldmine.Trade) error {
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

func DeleteTrade(dbFilename string, id int) error {
	db, err := sql.Open("sqlite3", dbFilename)
	if err != nil {
		log.Fatalf("Unable to open database: %s", err.Error())
	}
	defer db.Close()

	stmt, err := db.Prepare("DELETE FROM trades WHERE id = ?")
	if err != nil {
		return err
	}

	_, err = stmt.Exec(id)

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

func WriteDatabase(dbFilename string, trades chan goldmine.Trade, t *tomb.Tomb, wg sync.WaitGroup) {
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

func ReadAllTrades(dbFilename string) []goldmine.Trade {
	db, err := sql.Open("sqlite3", dbFilename)
	if err != nil {
		log.Fatalf("Unable to open database: %s", err.Error())
	}
	defer db.Close()

	var trades []goldmine.Trade
	rows, err := db.Query("SELECT id, account, security, price, quantity, volume, volumeCurrency, strategyId, signalId, comment, timestamp, useconds FROM trades")
	if err != nil {
		log.Printf("Unable to open DB: %s", err.Error())
		return trades
	}
	defer rows.Close()
	for rows.Next() {
		var t goldmine.Trade
		err = rows.Scan(&t.TradeId, &t.Account, &t.Security, &t.Price, &t.Quantity, &t.Volume, &t.VolumeCurrency, &t.StrategyId, &t.SignalId, &t.Comment, &t.Timestamp, &t.Useconds)
		if err != nil {
			log.Printf("Unable to get trades: %s", err.Error())
			return trades
		}
		trades = append(trades, t)
	}

	return trades
}
