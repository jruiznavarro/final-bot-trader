package model

// PositionSide represents the side of a position
type PositionSide string

const (
	PositionSideLong  PositionSide = "LONG"
	PositionSideShort PositionSide = "SHORT"
)

// MarginMode represents the margin mode
type MarginMode string

const (
	MarginModeIsolated MarginMode = "ISOLATED"
	MarginModeCross    MarginMode = "CROSS"
)

// HistoryPosition represents a closed position from the exchange's history,
// including the exchange-reported realized PnL, fees and funding.
type HistoryPosition struct {
	PositionID  string  // Unique position identifier
	Symbol      string
	Side        string  // LONG or SHORT
	MaxQty      float64 // Maximum quantity held
	EntryPrice  float64 // Average entry price
	ClosePrice  float64 // Average close price
	RealizedPnL float64 // Exchange-reported realized PnL (net of fees and funding)
	Fee         float64 // Total trading fees paid (negative or positive per exchange convention)
	Funding     float64 // Total funding paid/received
	CreateTime  int64   // Position open time (ms)
	CloseTime   int64   // Position close time (ms)
}

// Position represents an open trading position
type Position struct {
	PositionID    string  // Unique position identifier
	Symbol        string
	Side          string  // LONG or SHORT
	PositionAmt   float64 // Position quantity
	EntryPrice    float64 // Average entry price
	UnrealizedPnl float64 // Unrealized profit/loss
	LiqPrice      float64 // Liquidation price
	Leverage      int     // Current leverage
	MarginMode    string  // ISOLATED or CROSS
	Margin        float64 // Position margin
}

// IsLong returns true if the position is long
func (p *Position) IsLong() bool {
	return p.Side == string(PositionSideLong)
}

// IsShort returns true if the position is short
func (p *Position) IsShort() bool {
	return p.Side == string(PositionSideShort)
}

// IsOpen returns true if the position has quantity > 0
func (p *Position) IsOpen() bool {
	return p.PositionAmt != 0
}

// NotionalValue returns the notional value of the position
func (p *Position) NotionalValue() float64 {
	if p.PositionAmt < 0 {
		return -p.PositionAmt * p.EntryPrice
	}
	return p.PositionAmt * p.EntryPrice
}

// PnlPercent returns the unrealized PnL as a percentage of entry value
func (p *Position) PnlPercent() float64 {
	notional := p.NotionalValue()
	if notional == 0 {
		return 0
	}
	return (p.UnrealizedPnl / notional) * 100
}
