
package goldmine

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
