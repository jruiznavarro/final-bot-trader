package backtest

import (
	"errors"
	"time"
)

// Common errors
var (
	ErrInsufficientBalance = errors.New("insufficient balance")
	ErrNoOpenPosition      = errors.New("no open position")
	ErrPositionAlreadyOpen = errors.New("position already open")
)

// Portfolio represents a simulated trading portfolio
type Portfolio struct {
	InitialBalance float64
	Balance        float64
	Equity         float64 // Balance + unrealized PnL
	OpenTrade      *Trade
	ClosedTrades   []*Trade
	MaxBalance     float64
	MinBalance     float64
	PeakEquity     float64
	CurrentDrawdown float64
	MaxDrawdown    float64
	tradeCounter   int
}

// NewPortfolio creates a new portfolio with initial balance
func NewPortfolio(initialBalance float64) *Portfolio {
	return &Portfolio{
		InitialBalance: initialBalance,
		Balance:        initialBalance,
		Equity:         initialBalance,
		MaxBalance:     initialBalance,
		MinBalance:     initialBalance,
		PeakEquity:     initialBalance,
		ClosedTrades:   make([]*Trade, 0),
	}
}

// OpenPosition opens a new position
func (p *Portfolio) OpenPosition(trade *Trade) error {
	if p.OpenTrade != nil {
		return ErrPositionAlreadyOpen
	}

	requiredMargin := trade.EntryPrice * trade.Quantity
	if requiredMargin > p.Balance {
		return ErrInsufficientBalance
	}

	p.tradeCounter++
	trade.ID = p.tradeCounter
	p.OpenTrade = trade

	return nil
}

// ClosePosition closes the current open position
func (p *Portfolio) ClosePosition(exitPrice float64, exitTime time.Time, exitReason string) error {
	if p.OpenTrade == nil {
		return ErrNoOpenPosition
	}

	p.OpenTrade.Close(exitPrice, exitTime, exitReason)

	// Update balance
	p.Balance += p.OpenTrade.PnL

	// Track max/min balance
	if p.Balance > p.MaxBalance {
		p.MaxBalance = p.Balance
	}
	if p.Balance < p.MinBalance {
		p.MinBalance = p.Balance
	}

	// Move to closed trades
	p.ClosedTrades = append(p.ClosedTrades, p.OpenTrade)
	p.OpenTrade = nil

	// Update equity
	p.Equity = p.Balance

	return nil
}

// UpdateEquity updates the portfolio equity based on current price
func (p *Portfolio) UpdateEquity(currentPrice float64) {
	if p.OpenTrade != nil {
		unrealizedPnL := p.OpenTrade.UnrealizedPnL(currentPrice)
		p.Equity = p.Balance + unrealizedPnL
	} else {
		p.Equity = p.Balance
	}

	// Track peak equity and drawdown
	if p.Equity > p.PeakEquity {
		p.PeakEquity = p.Equity
	}

	if p.PeakEquity > 0 {
		p.CurrentDrawdown = (p.PeakEquity - p.Equity) / p.PeakEquity * 100
		if p.CurrentDrawdown > p.MaxDrawdown {
			p.MaxDrawdown = p.CurrentDrawdown
		}
	}
}

// HasOpenPosition returns true if there's an open position
func (p *Portfolio) HasOpenPosition() bool {
	return p.OpenTrade != nil
}

// TotalPnL returns the total realized PnL
func (p *Portfolio) TotalPnL() float64 {
	var total float64
	for _, trade := range p.ClosedTrades {
		total += trade.PnL
	}
	return total
}

// TotalTrades returns the number of closed trades
func (p *Portfolio) TotalTrades() int {
	return len(p.ClosedTrades)
}

// WinningTrades returns the number of winning trades
func (p *Portfolio) WinningTrades() int {
	count := 0
	for _, trade := range p.ClosedTrades {
		if trade.IsWinning() {
			count++
		}
	}
	return count
}

// LosingTrades returns the number of losing trades
func (p *Portfolio) LosingTrades() int {
	count := 0
	for _, trade := range p.ClosedTrades {
		if trade.IsLosing() {
			count++
		}
	}
	return count
}

// WinRate returns the win rate as a percentage
func (p *Portfolio) WinRate() float64 {
	if len(p.ClosedTrades) == 0 {
		return 0
	}
	return float64(p.WinningTrades()) / float64(len(p.ClosedTrades)) * 100
}

// AverageWin returns the average winning trade PnL
func (p *Portfolio) AverageWin() float64 {
	wins := p.WinningTrades()
	if wins == 0 {
		return 0
	}

	var total float64
	for _, trade := range p.ClosedTrades {
		if trade.IsWinning() {
			total += trade.PnL
		}
	}
	return total / float64(wins)
}

// AverageLoss returns the average losing trade PnL (as positive number)
func (p *Portfolio) AverageLoss() float64 {
	losses := p.LosingTrades()
	if losses == 0 {
		return 0
	}

	var total float64
	for _, trade := range p.ClosedTrades {
		if trade.IsLosing() {
			total += -trade.PnL // Convert to positive
		}
	}
	return total / float64(losses)
}

// ProfitFactor returns the profit factor (gross profit / gross loss)
func (p *Portfolio) ProfitFactor() float64 {
	var grossProfit, grossLoss float64
	for _, trade := range p.ClosedTrades {
		if trade.IsWinning() {
			grossProfit += trade.PnL
		} else {
			grossLoss += -trade.PnL
		}
	}

	if grossLoss == 0 {
		if grossProfit > 0 {
			return 100 // Infinite profit factor, cap at 100
		}
		return 0
	}

	return grossProfit / grossLoss
}

// ReturnPercent returns the total return as a percentage
func (p *Portfolio) ReturnPercent() float64 {
	if p.InitialBalance == 0 {
		return 0
	}
	return (p.Balance - p.InitialBalance) / p.InitialBalance * 100
}
