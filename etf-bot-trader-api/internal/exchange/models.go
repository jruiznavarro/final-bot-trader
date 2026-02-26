package exchange

import "time"

// Candle represents OHLCV data
type Candle struct {
	Time   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
}

// Position represents an open position
type Position struct {
	Symbol        string
	Quantity      float64
	EntryPrice    float64
	CurrentPrice  float64
	MarketValue   float64
	UnrealizedPnL float64
	Side          string // "long" or "short"
}

// Order represents a trade order
type Order struct {
	ID            string
	Symbol        string
	Side          string  // "buy" or "sell"
	Type          string  // "market", "limit", "stop", "stop_limit"
	Quantity      float64
	LimitPrice    float64
	StopPrice     float64
	Status        string
	FilledQty     float64
	FilledAvgPrice float64
	CreatedAt     time.Time
}

// AccountInfo represents account information
type AccountInfo struct {
	Cash             float64
	PortfolioValue   float64
	BuyingPower      float64
	Equity           float64
	DayTradeCount    int
	PatternDayTrader bool
}

// Quote represents current price data
type Quote struct {
	Symbol    string
	BidPrice  float64
	AskPrice  float64
	LastPrice float64
	Volume    float64
	Timestamp time.Time
}
