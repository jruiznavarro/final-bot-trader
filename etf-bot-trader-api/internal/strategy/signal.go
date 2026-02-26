package strategy

import "errors"

// SignalType represents the type of trading signal
type SignalType int

const (
	SignalNone SignalType = iota
	SignalBuy
	SignalSell
)

func (s SignalType) String() string {
	switch s {
	case SignalBuy:
		return "BUY"
	case SignalSell:
		return "SELL"
	default:
		return "NONE"
	}
}

// Signal represents a trading signal
type Signal struct {
	Type       SignalType
	Symbol     string
	Price      float64
	StopLoss   float64
	TakeProfit float64
	Confidence float64
	Reason     string
}

// Common errors
var (
	ErrNoSignal         = errors.New("no signal")
	ErrInsufficientData = errors.New("insufficient data")
)
