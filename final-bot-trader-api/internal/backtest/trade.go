package backtest

import (
	"time"

	"final-bot-trader-api/internal/strategy"
)

// TradeStatus represents the status of a trade
type TradeStatus string

const (
	TradeStatusOpen   TradeStatus = "OPEN"
	TradeStatusClosed TradeStatus = "CLOSED"
)

// TradeType represents the type of trade
type TradeType string

const (
	TradeTypeLong  TradeType = "LONG"
	TradeTypeShort TradeType = "SHORT"
)

// Trade represents a simulated trade
type Trade struct {
	ID           int
	Symbol       string
	Type         TradeType
	Status       TradeStatus
	EntryPrice   float64
	ExitPrice    float64
	Quantity     float64
	StopLoss     float64
	TakeProfit   float64
	EntryTime    time.Time
	ExitTime     time.Time
	EntryReason  string
	ExitReason   string
	Commission   float64
	Slippage     float64
	PnL          float64
	PnLPercent   float64
	HoldDuration time.Duration
}

// IsWinning returns true if the trade is profitable
func (t *Trade) IsWinning() bool {
	return t.PnL > 0
}

// IsLosing returns true if the trade is unprofitable
func (t *Trade) IsLosing() bool {
	return t.PnL < 0
}

// CalculatePnL calculates the profit/loss for a closed trade
func (t *Trade) CalculatePnL() {
	if t.Status != TradeStatusClosed {
		return
	}

	// Calculate raw PnL
	var rawPnL float64
	if t.Type == TradeTypeLong {
		rawPnL = (t.ExitPrice - t.EntryPrice) * t.Quantity
	} else {
		rawPnL = (t.EntryPrice - t.ExitPrice) * t.Quantity
	}

	// Subtract commission
	t.PnL = rawPnL - t.Commission

	// Calculate percentage
	entryValue := t.EntryPrice * t.Quantity
	if entryValue > 0 {
		t.PnLPercent = (t.PnL / entryValue) * 100
	}

	// Calculate hold duration
	if !t.ExitTime.IsZero() && !t.EntryTime.IsZero() {
		t.HoldDuration = t.ExitTime.Sub(t.EntryTime)
	}
}

// NewTradeFromSignal creates a new trade from a strategy signal
func NewTradeFromSignal(id int, signal *strategy.Signal, commission float64) *Trade {
	tradeType := TradeTypeLong
	if signal.Type == strategy.SignalSell {
		tradeType = TradeTypeShort
	}

	return &Trade{
		ID:          id,
		Symbol:      signal.Symbol,
		Type:        tradeType,
		Status:      TradeStatusOpen,
		EntryPrice:  signal.Price,
		Quantity:    signal.Quantity,
		StopLoss:    signal.SL,
		TakeProfit:  signal.TP,
		EntryTime:   signal.Timestamp,
		EntryReason: signal.Reason,
		Commission:  commission,
	}
}

// Close closes the trade with the given exit information
func (t *Trade) Close(exitPrice float64, exitTime time.Time, exitReason string) {
	t.ExitPrice = exitPrice
	t.ExitTime = exitTime
	t.ExitReason = exitReason
	t.Status = TradeStatusClosed
	t.CalculatePnL()
}

// CheckStopLoss checks if stop loss is hit
func (t *Trade) CheckStopLoss(currentPrice float64) bool {
	if t.StopLoss <= 0 {
		return false
	}

	if t.Type == TradeTypeLong {
		return currentPrice <= t.StopLoss
	}
	return currentPrice >= t.StopLoss
}

// CheckTakeProfit checks if take profit is hit
func (t *Trade) CheckTakeProfit(currentPrice float64) bool {
	if t.TakeProfit <= 0 {
		return false
	}

	if t.Type == TradeTypeLong {
		return currentPrice >= t.TakeProfit
	}
	return currentPrice <= t.TakeProfit
}

// UnrealizedPnL calculates unrealized PnL for an open trade
func (t *Trade) UnrealizedPnL(currentPrice float64) float64 {
	if t.Status != TradeStatusOpen {
		return t.PnL
	}

	if t.Type == TradeTypeLong {
		return (currentPrice - t.EntryPrice) * t.Quantity
	}
	return (t.EntryPrice - currentPrice) * t.Quantity
}
