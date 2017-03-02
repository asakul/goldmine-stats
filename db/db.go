
package db

import ("database/sql"
		"sync"
		"log"
		"../goldmine"
		"gopkg.in/tomb.v2"
		"time"
		"math"
	)

type ClosedTrade struct {
	Account string
	Security string
	EntryTime time.Time
	ExitTime time.Time
	Profit float64
	ProfitCurrency string
	Strategy string
	Direction string
	tradeIds []int
}

type DbHandle struct {
	Db *sql.DB
}

func Open(dbFilename string) (*DbHandle, error) {
	db, err := sql.Open("sqlite3", dbFilename)
	return &DbHandle {db}, err
}

func Close(handle *DbHandle) {
	handle.Db.Close()
}

func insertTrade(db *sql.DB, trade goldmine.Trade) error {
	stmt, err := db.Prepare("INSERT INTO trades(account, security, price, quantity, volume, volumeCurrency, strategyId, signalId, comment, timestamp, useconds, balanced) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)")
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

func DeleteTrade(db *DbHandle, id int) error {
	stmt, err := db.Db.Prepare("DELETE FROM trades WHERE id = ?")
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
	_, err := db.Exec("CREATE TABLE IF NOT EXISTS trades(id INTEGER PRIMARY KEY, account TEXT, security TEXT, price REAL, quantity INTEGER, volume REAL, volumeCurrency TEXT, strategyId TEXT, signalId TEXT, comment TEXT, timestamp INTEGER, useconds INTEGER, balanced INTEGER)")
	if err != nil {
		return err
	}
	_, err = db.Exec("CREATE TABLE IF NOT EXISTS closed_trades(id INTEGER PRIMARY KEY, account TEXT, security TEXT, entry_timestamp INTEGER, exit_timestamp INTEGER, profit REAL, profit_currency TEXT, strategyId TEXT)")
	if err != nil {
		return err
	}
	return nil
}

func WriteDatabase(db *DbHandle, trades chan goldmine.Trade, t *tomb.Tomb, wg sync.WaitGroup) {
	defer wg.Done()
	err := createSchema(db.Db)
	if err != nil {
		log.Fatalf("Unable to ping database: %s", err.Error())
	}
	for {
		select {
		case trade := <-trades:
			err = insertTrade(db.Db, trade)
			if err != nil {
				log.Print(err.Error())
			}
		case <-t.Dying():
			return
		}
	}
}

func ReadAllTrades(db *DbHandle, account string) []goldmine.Trade {
	var trades []goldmine.Trade
	var rows *sql.Rows
	var err error
	if account == "" {
		rows, err = db.Db.Query("SELECT id, account, security, price, quantity, volume, volumeCurrency, strategyId, signalId, comment, timestamp, useconds FROM trades ORDER BY timestamp")
	} else {
		rows, err = db.Db.Query("SELECT id, account, security, price, quantity, volume, volumeCurrency, strategyId, signalId, comment, timestamp, useconds FROM trades WHERE account = ? ORDER BY timestamp", account)
	}
	if err != nil {
		log.Printf("Unable to open DB: %s", err.Error())
		return trades
	}
	defer rows.Close()
	for rows.Next() {
		var t goldmine.Trade
		err := rows.Scan(&t.TradeId, &t.Account, &t.Security, &t.Price, &t.Quantity, &t.Volume, &t.VolumeCurrency, &t.StrategyId, &t.SignalId, &t.Comment, &t.Timestamp, &t.Useconds)
		if err != nil {
			log.Printf("Unable to get trades: %s", err.Error())
			return trades
		}
		trades = append(trades, t)
	}

	return trades
}

func GetAllAccounts(db *DbHandle) ([]string, error) {
	var result []string
	rows, err := db.Db.Query("SELECT account FROM trades GROUP BY account")
	if err != nil {
		log.Printf("Unable to obtain all accounts: %s", err.Error())
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var account string
		err = rows.Scan(&account)
		if err != nil {
			log.Printf("Unable to obtain all accounts: %s", err.Error())
			return result, err
		}
		result = append(result, account)
	}
	return result, nil
}

func GetAllClosedTrades(db * DbHandle) ([]ClosedTrade, error) {
	var result []ClosedTrade
	rows, err := db.Db.Query("SELECT account, security, entry_timestamp, exit_timestamp, profit, profit_currency, strategyId FROM closed_trades")
	if err != nil {
		log.Printf("Unable to obtain all accounts: %s", err.Error())
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var trade ClosedTrade
		var entry int64
		var exit int64
		err = rows.Scan(&trade.Account, &trade.Security, &entry, &exit, &trade.Profit, &trade.ProfitCurrency, &trade.Strategy)
		trade.EntryTime = time.Unix(entry, 0)
		trade.ExitTime = time.Unix(exit, 0)
		if err != nil {
			return result, err
		}
		result = append(result, trade)
	}
	return result, nil
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
		ks float64
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
			balanceEntry.trade.Profit = -trade.Price * float64(trade.Quantity)
			log.Printf("0profit = %f", balanceEntry.trade.Profit)
			balanceEntry.trade.Strategy = trade.StrategyId
			balanceEntry.ks = trade.Volume / (trade.Price * math.Abs(float64(trade.Quantity)))
			balanceEntry.trade.tradeIds = append(balanceEntry.trade.tradeIds, trade.TradeId)
			log.Printf("Ks = %f", balanceEntry.ks)
			if trade.Quantity > 0 {
				balanceEntry.trade.Direction = "long"
			} else {
				balanceEntry.trade.Direction = "short"
			}
			balance[key] = balanceEntry
		} else {
			log.Printf("1profit = %f", balanceEntry.trade.Profit)
			balanceEntry.balance += trade.Quantity
			balanceEntry.trade.Profit += -trade.Price * float64(trade.Quantity)
			balanceEntry.ks += trade.Volume / (trade.Price * math.Abs(float64(trade.Quantity)))
			balanceEntry.ks /= 2
			balanceEntry.trade.tradeIds = append(balanceEntry.trade.tradeIds, trade.TradeId)
			log.Printf("Ks = %f, profit = %f", balanceEntry.ks, balanceEntry.trade.Profit)

			if balanceEntry.balance == 0 {
				balanceEntry.trade.Profit = balanceEntry.trade.Profit * balanceEntry.ks
				balanceEntry.trade.ExitTime = time.Unix(int64(trade.Timestamp), int64(trade.Useconds))
				result = append(result, balanceEntry.trade)
			}
			balance[key] = balanceEntry
		}
	}
	return result
}


func BalanceTrades(db *DbHandle) error {
	var trades []goldmine.Trade
	var rows *sql.Rows
	var err error

	tx, err := db.Db.Begin()
	if err != nil {
		return err
	}
	rows, err = tx.Query("SELECT id, account, security, price, quantity, volume, volumeCurrency, strategyId, signalId, comment, timestamp, useconds FROM trades WHERE balanced == 0 ORDER BY timestamp")
	if err != nil {
		tx.Rollback()
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var t goldmine.Trade
		err = rows.Scan(&t.TradeId, &t.Account, &t.Security, &t.Price, &t.Quantity, &t.Volume, &t.VolumeCurrency, &t.StrategyId, &t.SignalId, &t.Comment, &t.Timestamp, &t.Useconds)
		if err != nil {
			tx.Rollback()
			log.Printf("Unable to get trades: %s", err.Error())
			return err
		}
		trades = append(trades, t)
	}
	closed := aggregateClosedTrades(trades)
	for _, closedTrade := range(closed) {
		for _, tradeId := range(closedTrade.tradeIds) {
			_, err = tx.Exec("UPDATE trades SET balanced=1 WHERE id ==$1", tradeId)
			if err != nil {
				tx.Rollback()
				return err
			}
		}
		_, err = tx.Exec("INSERT INTO closed_trades (account, security, entry_timestamp, exit_timestamp, profit, profit_currency, strategyId) VALUES ($1, $2, $3, $4, $5, $6, $7)", closedTrade.Account, closedTrade.Security, closedTrade.EntryTime.Unix(), closedTrade.ExitTime.Unix(), closedTrade.Profit, closedTrade.ProfitCurrency, closedTrade.Strategy)
		if err != nil {
			tx.Rollback()
			return err
		}
	}
	tx.Commit()
	return nil
}
