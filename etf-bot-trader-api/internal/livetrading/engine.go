package livetrading

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"etf-bot-trader-api/internal/exchange"
	"etf-bot-trader-api/internal/strategy"
)

// Trade represents an executed trade
type Trade struct {
	ID         string    `json:"id"`
	Symbol     string    `json:"symbol"`
	Side       string    `json:"side"`
	EntryPrice float64   `json:"entry_price"`
	Quantity   float64   `json:"quantity"`
	StopLoss   float64   `json:"stop_loss"`
	TakeProfit float64   `json:"take_profit"`
	EntryTime  time.Time `json:"entry_time"`
	OrderID    string    `json:"order_id"`
	Reason     string    `json:"reason"`
	Status     string    `json:"status"` // "OPEN", "CLOSED"
	ExitPrice  float64   `json:"exit_price,omitempty"`
	ExitTime   time.Time `json:"exit_time,omitempty"`
	PnL        float64   `json:"pnl,omitempty"`
	ExitReason string    `json:"exit_reason,omitempty"`
}

// TradingState holds the current trading state
type TradingState struct {
	Trades          []Trade   `json:"trades"`
	TotalPnL        float64   `json:"total_pnl"`
	WinCount        int       `json:"win_count"`
	LossCount       int       `json:"loss_count"`
	StartTime       time.Time `json:"start_time"`
	LastUpdate      time.Time `json:"last_update"`
	DailyPnL        float64   `json:"daily_pnl"`
	DailyTradeCount int       `json:"daily_trade_count"`
	LastDailyReset  time.Time `json:"last_daily_reset"`
}

// Config holds live trading configuration
type Config struct {
	Symbols          []string // ETF symbols to trade
	PositionSizeUSD  float64  // Fixed USD amount per trade
	PositionSizePct  float64  // Position size as % of equity
	MaxDailyLoss     float64  // Max daily loss in USD
	MaxDailyTrades   int      // Max trades per day
	MaxOpenPositions int      // Max simultaneous positions
	StateFile        string   // File to persist state
	CheckInterval    time.Duration
}

// DefaultConfig returns default configuration
func DefaultConfig() Config {
	return Config{
		Symbols: []string{
			"SPY",  // S&P 500
			"QQQ",  // Nasdaq 100
			"IWM",  // Russell 2000
		},
		PositionSizeUSD:  500,            // $500 per trade
		PositionSizePct:  0.05,           // 5% of equity per trade
		MaxDailyLoss:     100,            // Stop if losing $100/day
		MaxDailyTrades:   6,              // Max 6 trades per day
		MaxOpenPositions: 2,              // Max 2 positions at once
		StateFile:        "data/etf_trading_state.json",
		CheckInterval:    5 * time.Minute, // Check every 5 minutes
	}
}

// Engine is the live trading engine
type Engine struct {
	config     Config
	client     *exchange.AlpacaClient
	state      *TradingState
	strategies map[string]*strategy.ETFMomentumStrategy
	mu         sync.RWMutex
	running    bool
	stopCh     chan struct{}
}

// NewEngine creates a new live trading engine
func NewEngine(client *exchange.AlpacaClient, config Config) *Engine {
	state := &TradingState{
		Trades:         []Trade{},
		StartTime:      time.Now(),
		LastUpdate:     time.Now(),
		LastDailyReset: time.Now().Truncate(24 * time.Hour),
	}

	// Create strategies for each symbol
	stratConfig := strategy.DefaultETFMomentumConfig()
	strategies := make(map[string]*strategy.ETFMomentumStrategy)
	for _, symbol := range config.Symbols {
		strategies[symbol] = strategy.NewETFMomentumStrategy(symbol, stratConfig)
	}

	return &Engine{
		config:     config,
		client:     client,
		state:      state,
		strategies: strategies,
		stopCh:     make(chan struct{}),
	}
}

// LoadState loads trading state from file
func (e *Engine) LoadState() error {
	data, err := os.ReadFile(e.config.StateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	return json.Unmarshal(data, e.state)
}

// SaveState saves trading state to file
func (e *Engine) SaveState() error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	data, err := json.MarshalIndent(e.state, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(e.config.StateFile, data, 0644)
}

// Run starts the trading engine
func (e *Engine) Run(ctx context.Context) error {
	e.mu.Lock()
	e.running = true
	e.mu.Unlock()

	ticker := time.NewTicker(e.config.CheckInterval)
	defer ticker.Stop()

	log.Println("ETF Trading Engine started")
	log.Printf("Symbols: %v", e.config.Symbols)
	log.Printf("Check interval: %v", e.config.CheckInterval)

	// Run immediately
	e.processAllSymbols(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-e.stopCh:
			return nil
		case <-ticker.C:
			// Check if market is open
			isOpen, err := e.client.IsMarketOpen(ctx)
			if err != nil {
				log.Printf("Error checking market status: %v", err)
				continue
			}

			if !isOpen {
				nextOpen, _ := e.client.GetNextMarketOpen(ctx)
				log.Printf("Market closed. Next open: %v", nextOpen.Format("2006-01-02 15:04 MST"))
				continue
			}

			e.processAllSymbols(ctx)
		}
	}
}

// Stop stops the trading engine
func (e *Engine) Stop() {
	e.mu.Lock()
	e.running = false
	e.mu.Unlock()
	close(e.stopCh)
}

func (e *Engine) processAllSymbols(ctx context.Context) {
	// Reset daily counters if needed
	e.checkDailyReset()

	// Check safety limits
	if !e.checkSafetyLimits() {
		log.Println("Safety limits reached, skipping this cycle")
		return
	}

	// Sync positions with exchange
	e.syncPositions(ctx)

	for _, symbol := range e.config.Symbols {
		if err := e.processSymbol(ctx, symbol); err != nil {
			log.Printf("[%s] Error: %v", symbol, err)
		}
	}

	e.state.LastUpdate = time.Now()
	if err := e.SaveState(); err != nil {
		log.Printf("Error saving state: %v", err)
	}
}

func (e *Engine) checkDailyReset() {
	e.mu.Lock()
	defer e.mu.Unlock()

	today := time.Now().Truncate(24 * time.Hour)
	if today.After(e.state.LastDailyReset) {
		log.Println("Resetting daily counters")
		e.state.DailyPnL = 0
		e.state.DailyTradeCount = 0
		e.state.LastDailyReset = today
	}
}

func (e *Engine) checkSafetyLimits() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// Check daily loss limit
	if e.state.DailyPnL < -e.config.MaxDailyLoss {
		log.Printf("Daily loss limit reached: $%.2f", e.state.DailyPnL)
		return false
	}

	// Check daily trade limit
	if e.state.DailyTradeCount >= e.config.MaxDailyTrades {
		log.Printf("Daily trade limit reached: %d trades", e.state.DailyTradeCount)
		return false
	}

	return true
}

func (e *Engine) syncPositions(ctx context.Context) {
	positions, err := e.client.GetPositions(ctx)
	if err != nil {
		log.Printf("Error getting positions: %v", err)
		return
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// Create map of current positions
	positionMap := make(map[string]exchange.Position)
	for _, pos := range positions {
		positionMap[pos.Symbol] = pos
	}

	// Check for closed trades
	for i := range e.state.Trades {
		trade := &e.state.Trades[i]
		if trade.Status != "OPEN" {
			continue
		}

		if pos, exists := positionMap[trade.Symbol]; exists {
			// Update current price
			trade.ExitPrice = pos.CurrentPrice
		} else {
			// Position was closed
			trade.Status = "CLOSED"
			trade.ExitTime = time.Now()

			// Calculate PnL
			if trade.Side == "long" {
				trade.PnL = (trade.ExitPrice - trade.EntryPrice) * trade.Quantity
			} else {
				trade.PnL = (trade.EntryPrice - trade.ExitPrice) * trade.Quantity
			}

			// Determine exit reason
			if trade.Side == "long" {
				if trade.ExitPrice >= trade.TakeProfit {
					trade.ExitReason = "Take Profit"
				} else if trade.ExitPrice <= trade.StopLoss {
					trade.ExitReason = "Stop Loss"
				} else {
					trade.ExitReason = "Manual/Other"
				}
			} else {
				if trade.ExitPrice <= trade.TakeProfit {
					trade.ExitReason = "Take Profit"
				} else if trade.ExitPrice >= trade.StopLoss {
					trade.ExitReason = "Stop Loss"
				} else {
					trade.ExitReason = "Manual/Other"
				}
			}

			// Update totals
			e.state.TotalPnL += trade.PnL
			e.state.DailyPnL += trade.PnL
			if trade.PnL >= 0 {
				e.state.WinCount++
			} else {
				e.state.LossCount++
			}

			log.Printf("[%s] Position closed: %s | PnL: $%.2f", trade.Symbol, trade.ExitReason, trade.PnL)
		}
	}
}

func (e *Engine) processSymbol(ctx context.Context, symbol string) error {
	// Check if we have an open position
	if e.hasOpenPosition(symbol) {
		return nil
	}

	// Check max open positions
	if e.countOpenPositions() >= e.config.MaxOpenPositions {
		return nil
	}

	// Get candles (need more days because market is only open 6.5h/day)
	end := time.Now()
	start := end.Add(-30 * 24 * time.Hour) // 30 days of hourly data

	hourlyCandles, err := e.client.GetBars(ctx, symbol, "1h", start, end)
	if err != nil {
		return fmt.Errorf("failed to get hourly candles: %w", err)
	}

	if len(hourlyCandles) < 60 {
		return fmt.Errorf("insufficient candles: %d", len(hourlyCandles))
	}

	// Get daily candles for trend
	dailyStart := end.Add(-90 * 24 * time.Hour)
	dailyCandles, err := e.client.GetBars(ctx, symbol, "1d", dailyStart, end)
	if err != nil {
		return fmt.Errorf("failed to get daily candles: %w", err)
	}

	// Analyze with daily trend
	strat := e.strategies[symbol]
	signal, err := strat.AnalyzeWithDailyTrend(dailyCandles, hourlyCandles)
	if err != nil {
		if err == strategy.ErrNoSignal {
			return nil
		}
		return err
	}

	// Execute trade
	return e.executeTrade(ctx, symbol, signal)
}

func (e *Engine) hasOpenPosition(symbol string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, trade := range e.state.Trades {
		if trade.Symbol == symbol && trade.Status == "OPEN" {
			return true
		}
	}
	return false
}

func (e *Engine) countOpenPositions() int {
	e.mu.RLock()
	defer e.mu.RUnlock()

	count := 0
	for _, trade := range e.state.Trades {
		if trade.Status == "OPEN" {
			count++
		}
	}
	return count
}

func (e *Engine) executeTrade(ctx context.Context, symbol string, signal *strategy.Signal) error {
	// Get account info for position sizing
	account, err := e.client.GetAccount(ctx)
	if err != nil {
		return fmt.Errorf("failed to get account: %w", err)
	}

	// Calculate position size
	positionSize := e.config.PositionSizeUSD
	if e.config.PositionSizePct > 0 {
		positionSize = account.Equity * e.config.PositionSizePct
	}

	// Calculate quantity
	quantity := positionSize / signal.Price

	// Round to whole shares for ETFs
	quantity = float64(int(quantity))
	if quantity < 1 {
		quantity = 1
	}

	side := "buy"
	sideStr := "long"
	if signal.Type == strategy.SignalSell {
		side = "sell"
		sideStr = "short"
	}

	log.Printf("\n[%s] SIGNAL: %s @ $%.2f", time.Now().Format("2006-01-02 15:04"), symbol, signal.Price)
	log.Printf("         Side: %s | Qty: %.0f | SL: $%.2f | TP: $%.2f", sideStr, quantity, signal.StopLoss, signal.TakeProfit)
	log.Printf("         Reason: %s", signal.Reason)

	// Place market order (TP/SL managed separately)
	order, err := e.client.PlaceMarketOrder(ctx, symbol, quantity, side)
	if err != nil {
		log.Printf("         Order failed: %v", err)
		return err
	}

	log.Printf("         Order placed: ID=%s", order.ID)

	// Record trade
	e.mu.Lock()
	trade := Trade{
		ID:         fmt.Sprintf("%s-%d", symbol, time.Now().UnixNano()),
		Symbol:     symbol,
		Side:       sideStr,
		EntryPrice: signal.Price,
		Quantity:   quantity,
		StopLoss:   signal.StopLoss,
		TakeProfit: signal.TakeProfit,
		EntryTime:  time.Now(),
		OrderID:    order.ID,
		Reason:     signal.Reason,
		Status:     "OPEN",
	}
	e.state.Trades = append(e.state.Trades, trade)
	e.state.DailyTradeCount++
	e.mu.Unlock()

	return nil
}

// GetStatus returns current trading status
func (e *Engine) GetStatus() map[string]interface{} {
	e.mu.RLock()
	defer e.mu.RUnlock()

	openTrades := 0
	closedTrades := 0
	for _, t := range e.state.Trades {
		if t.Status == "OPEN" {
			openTrades++
		} else {
			closedTrades++
		}
	}

	winRate := 0.0
	totalClosed := e.state.WinCount + e.state.LossCount
	if totalClosed > 0 {
		winRate = float64(e.state.WinCount) / float64(totalClosed) * 100
	}

	return map[string]interface{}{
		"running":          e.running,
		"paper":            e.client.IsPaper(),
		"symbols":          e.config.Symbols,
		"open_positions":   openTrades,
		"closed_trades":    closedTrades,
		"total_pnl":        e.state.TotalPnL,
		"daily_pnl":        e.state.DailyPnL,
		"daily_trades":     e.state.DailyTradeCount,
		"win_count":        e.state.WinCount,
		"loss_count":       e.state.LossCount,
		"win_rate":         winRate,
		"start_time":       e.state.StartTime,
		"last_update":      e.state.LastUpdate,
	}
}

// GetOpenTrades returns all open trades
func (e *Engine) GetOpenTrades() []Trade {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var open []Trade
	for _, t := range e.state.Trades {
		if t.Status == "OPEN" {
			open = append(open, t)
		}
	}
	return open
}

// GetClosedTrades returns all closed trades
func (e *Engine) GetClosedTrades() []Trade {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var closed []Trade
	for _, t := range e.state.Trades {
		if t.Status == "CLOSED" {
			closed = append(closed, t)
		}
	}
	return closed
}
