package papertrading

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sync"
	"time"

	"final-bot-trader-api/internal/exchange"
	"final-bot-trader-api/internal/exchange/model"
	"final-bot-trader-api/internal/strategy"
	"final-bot-trader-api/internal/strategy/multifactor"
)

// PaperPosition represents a simulated position
type PaperPosition struct {
	ID         string    `json:"id"`
	Symbol     string    `json:"symbol"`
	Side       string    `json:"side"` // "LONG" or "SHORT"
	EntryPrice float64   `json:"entry_price"`
	Quantity   float64   `json:"quantity"`
	StopLoss   float64   `json:"stop_loss"`
	TakeProfit float64   `json:"take_profit"`
	EntryTime  time.Time `json:"entry_time"`
	Reason     string    `json:"reason"`
}

// PaperTrade represents a completed simulated trade
type PaperTrade struct {
	ID         string    `json:"id"`
	Symbol     string    `json:"symbol"`
	Side       string    `json:"side"`
	EntryPrice float64   `json:"entry_price"`
	ExitPrice  float64   `json:"exit_price"`
	Quantity   float64   `json:"quantity"`
	PnL        float64   `json:"pnl"`
	PnLPercent float64   `json:"pnl_percent"`
	EntryTime  time.Time `json:"entry_time"`
	ExitTime   time.Time `json:"exit_time"`
	ExitReason string    `json:"exit_reason"`
	Commission float64   `json:"commission"`
}

// Portfolio represents the paper trading portfolio
type Portfolio struct {
	InitialBalance float64         `json:"initial_balance"`
	Balance        float64         `json:"balance"`
	Equity         float64         `json:"equity"`
	OpenPositions  []PaperPosition `json:"open_positions"`
	ClosedTrades   []PaperTrade    `json:"closed_trades"`
	TotalPnL       float64         `json:"total_pnl"`
	WinCount       int             `json:"win_count"`
	LossCount      int             `json:"loss_count"`
	StartTime      time.Time       `json:"start_time"`
	LastUpdate     time.Time       `json:"last_update"`
}

// Config holds paper trading configuration
type Config struct {
	Symbols         []string
	InitialBalance  float64
	PositionSize    float64 // Percentage of balance per position
	Commission      float64 // Commission rate (e.g., 0.0005 = 0.05%)
	Slippage        float64 // Slippage rate
	Leverage        int
	Interval        string  // Candle interval (e.g., "4h")
	StateFile       string  // File to persist state
	VolumeThreshold float64 // Volume threshold for signals (default 1.0)
}

// DefaultConfig returns default paper trading configuration
func DefaultConfig() Config {
	return Config{
		Symbols: []string{
			"DOGEUSDT", "WLDUSDT", "1000PEPEUSDT", "ARBUSDT", "AAVEUSDT",
			"WIFUSDT", "FILUSDT", "SOLUSDT", "TAOUSDT", "SUIUSDT",
		},
		InitialBalance:  10000,
		PositionSize:    0.10, // 10% per position
		Commission:      0.0005,
		Slippage:        0.0003,
		Leverage:        5,
		Interval:        "4h",
		StateFile:       "paper_trading_state.json",
		VolumeThreshold: 1.0, // Default: require at least average volume
	}
}

// Engine is the paper trading engine
type Engine struct {
	config     Config
	client     exchange.Client
	portfolio  *Portfolio
	strategies map[string]*multifactor.MultiFactorStrategy
	mu         sync.RWMutex
	running    bool
	stopCh     chan struct{}
}

// NewEngine creates a new paper trading engine
func NewEngine(client exchange.Client, config Config) *Engine {
	portfolio := &Portfolio{
		InitialBalance: config.InitialBalance,
		Balance:        config.InitialBalance,
		Equity:         config.InitialBalance,
		OpenPositions:  []PaperPosition{},
		ClosedTrades:   []PaperTrade{},
		StartTime:      time.Now(),
		LastUpdate:     time.Now(),
	}

	strategies := make(map[string]*multifactor.MultiFactorStrategy)
	stratConfig := multifactor.DefaultConfig()

	// Apply volume threshold from config
	if config.VolumeThreshold > 0 {
		stratConfig.VolumeThreshold = config.VolumeThreshold
	}

	for _, symbol := range config.Symbols {
		strategies[symbol] = multifactor.NewMultiFactorStrategy(symbol, stratConfig)
	}

	return &Engine{
		config:     config,
		client:     client,
		portfolio:  portfolio,
		strategies: strategies,
		stopCh:     make(chan struct{}),
	}
}

// LoadState loads portfolio state from file
func (e *Engine) LoadState() error {
	data, err := os.ReadFile(e.config.StateFile)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No state file, start fresh
		}
		return err
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	return json.Unmarshal(data, e.portfolio)
}

// SaveState saves portfolio state to file
func (e *Engine) SaveState() error {
	e.mu.RLock()
	defer e.mu.RUnlock()

	data, err := json.MarshalIndent(e.portfolio, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(e.config.StateFile, data, 0644)
}

// Run starts the paper trading engine
func (e *Engine) Run(ctx context.Context) error {
	e.mu.Lock()
	e.running = true
	e.mu.Unlock()

	// Calculate interval duration
	intervalDuration := parseInterval(e.config.Interval)

	// Main loop
	ticker := time.NewTicker(intervalDuration)
	defer ticker.Stop()

	// Run immediately on start
	e.processAllSymbols(ctx)

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-e.stopCh:
			return nil
		case <-ticker.C:
			e.processAllSymbols(ctx)
		}
	}
}

// Stop stops the paper trading engine
func (e *Engine) Stop() {
	e.mu.Lock()
	e.running = false
	e.mu.Unlock()
	close(e.stopCh)
}

// ProcessOnce runs a single iteration of signal checking
func (e *Engine) ProcessOnce(ctx context.Context) {
	e.processAllSymbols(ctx)
}

func (e *Engine) processAllSymbols(ctx context.Context) {
	for _, symbol := range e.config.Symbols {
		if err := e.processSymbol(ctx, symbol); err != nil {
			fmt.Printf("[%s] Error processing %s: %v\n", time.Now().Format("15:04:05"), symbol, err)
		}
	}

	// Update equity and save state
	e.updateEquity(ctx)
	if err := e.SaveState(); err != nil {
		fmt.Printf("[%s] Error saving state: %v\n", time.Now().Format("15:04:05"), err)
	}
}

func (e *Engine) processSymbol(ctx context.Context, symbol string) error {
	// Get recent candles
	candles, err := e.client.GetKlines(ctx, symbol, e.config.Interval, 100, 0, 0)
	if err != nil {
		return fmt.Errorf("failed to get candles: %w", err)
	}

	if len(candles) < 60 {
		return fmt.Errorf("insufficient candles: %d", len(candles))
	}

	currentPrice := candles[len(candles)-1].Close
	currentTime := time.Now()

	// Check existing positions for SL/TP
	e.checkPositionExits(symbol, candles[len(candles)-1], currentTime)

	// Check if we already have a position for this symbol
	if e.hasOpenPosition(symbol) {
		return nil
	}

	// Get strategy signal
	strat := e.strategies[symbol]
	signal, err := strat.Analyze(candles)
	if err != nil {
		if err == strategy.ErrNoSignal {
			return nil
		}
		return err
	}

	// Open new position
	e.openPosition(symbol, signal, currentPrice, currentTime)

	return nil
}

func (e *Engine) hasOpenPosition(symbol string) bool {
	e.mu.RLock()
	defer e.mu.RUnlock()

	for _, pos := range e.portfolio.OpenPositions {
		if pos.Symbol == symbol {
			return true
		}
	}
	return false
}

func (e *Engine) openPosition(symbol string, signal *strategy.Signal, price float64, timestamp time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Calculate position size
	positionValue := e.portfolio.Balance * e.config.PositionSize * float64(e.config.Leverage)
	quantity := positionValue / price

	// Apply slippage
	entryPrice := price
	if signal.Type == strategy.SignalBuy {
		entryPrice = price * (1 + e.config.Slippage)
	} else {
		entryPrice = price * (1 - e.config.Slippage)
	}

	side := "LONG"
	if signal.Type == strategy.SignalSell {
		side = "SHORT"
	}

	position := PaperPosition{
		ID:         fmt.Sprintf("%s-%d", symbol, timestamp.UnixNano()),
		Symbol:     symbol,
		Side:       side,
		EntryPrice: entryPrice,
		Quantity:   quantity,
		StopLoss:   signal.SL,
		TakeProfit: signal.TP,
		EntryTime:  timestamp,
		Reason:     signal.Reason,
	}

	e.portfolio.OpenPositions = append(e.portfolio.OpenPositions, position)

	// Deduct commission from balance
	commission := entryPrice * quantity * e.config.Commission
	e.portfolio.Balance -= commission

	fmt.Printf("\n[%s] 📈 OPENED %s %s @ %.4f\n",
		timestamp.Format("2006-01-02 15:04"),
		side, symbol, entryPrice)
	fmt.Printf("         Quantity: %.4f | SL: %.4f | TP: %.4f\n",
		quantity, signal.SL, signal.TP)
	fmt.Printf("         Reason: %s\n", signal.Reason)
}

func (e *Engine) checkPositionExits(symbol string, candle model.Candle, timestamp time.Time) {
	e.mu.Lock()
	defer e.mu.Unlock()

	var remainingPositions []PaperPosition

	for _, pos := range e.portfolio.OpenPositions {
		if pos.Symbol != symbol {
			remainingPositions = append(remainingPositions, pos)
			continue
		}

		var exitPrice float64
		var exitReason string

		if pos.Side == "LONG" {
			// Check stop loss
			if candle.Low <= pos.StopLoss {
				exitPrice = pos.StopLoss * (1 - e.config.Slippage)
				exitReason = "Stop Loss"
			}
			// Check take profit
			if candle.High >= pos.TakeProfit {
				exitPrice = pos.TakeProfit * (1 - e.config.Slippage)
				exitReason = "Take Profit"
			}
		} else { // SHORT
			// Check stop loss
			if candle.High >= pos.StopLoss {
				exitPrice = pos.StopLoss * (1 + e.config.Slippage)
				exitReason = "Stop Loss"
			}
			// Check take profit
			if candle.Low <= pos.TakeProfit {
				exitPrice = pos.TakeProfit * (1 + e.config.Slippage)
				exitReason = "Take Profit"
			}
		}

		if exitPrice > 0 {
			e.closeTrade(pos, exitPrice, exitReason, timestamp)
		} else {
			remainingPositions = append(remainingPositions, pos)
		}
	}

	e.portfolio.OpenPositions = remainingPositions
}

func (e *Engine) closeTrade(pos PaperPosition, exitPrice float64, reason string, timestamp time.Time) {
	// Calculate PnL
	var pnl float64
	if pos.Side == "LONG" {
		pnl = (exitPrice - pos.EntryPrice) * pos.Quantity
	} else {
		pnl = (pos.EntryPrice - exitPrice) * pos.Quantity
	}

	// Deduct exit commission
	commission := exitPrice * pos.Quantity * e.config.Commission
	pnl -= commission

	pnlPercent := pnl / (pos.EntryPrice * pos.Quantity) * 100

	trade := PaperTrade{
		ID:         pos.ID,
		Symbol:     pos.Symbol,
		Side:       pos.Side,
		EntryPrice: pos.EntryPrice,
		ExitPrice:  exitPrice,
		Quantity:   pos.Quantity,
		PnL:        pnl,
		PnLPercent: pnlPercent,
		EntryTime:  pos.EntryTime,
		ExitTime:   timestamp,
		ExitReason: reason,
		Commission: commission * 2, // Entry + exit
	}

	e.portfolio.ClosedTrades = append(e.portfolio.ClosedTrades, trade)
	e.portfolio.Balance += pos.EntryPrice*pos.Quantity + pnl
	e.portfolio.TotalPnL += pnl

	if pnl > 0 {
		e.portfolio.WinCount++
		fmt.Printf("\n[%s] ✅ CLOSED %s %s @ %.4f (%s)\n",
			timestamp.Format("2006-01-02 15:04"),
			pos.Side, pos.Symbol, exitPrice, reason)
		fmt.Printf("         PnL: +$%.2f (+%.2f%%)\n", pnl, pnlPercent)
	} else {
		e.portfolio.LossCount++
		fmt.Printf("\n[%s] ❌ CLOSED %s %s @ %.4f (%s)\n",
			timestamp.Format("2006-01-02 15:04"),
			pos.Side, pos.Symbol, exitPrice, reason)
		fmt.Printf("         PnL: -$%.2f (%.2f%%)\n", -pnl, pnlPercent)
	}
}

func (e *Engine) updateEquity(ctx context.Context) {
	e.mu.Lock()
	defer e.mu.Unlock()

	equity := e.portfolio.Balance

	for _, pos := range e.portfolio.OpenPositions {
		price, err := e.client.GetPrice(ctx, pos.Symbol)
		if err != nil {
			continue
		}

		var unrealizedPnL float64
		if pos.Side == "LONG" {
			unrealizedPnL = (price - pos.EntryPrice) * pos.Quantity
		} else {
			unrealizedPnL = (pos.EntryPrice - price) * pos.Quantity
		}
		equity += pos.EntryPrice*pos.Quantity + unrealizedPnL
	}

	e.portfolio.Equity = equity
	e.portfolio.LastUpdate = time.Now()
}

// GetStatus returns current portfolio status
func (e *Engine) GetStatus() map[string]interface{} {
	e.mu.RLock()
	defer e.mu.RUnlock()

	winRate := 0.0
	totalTrades := e.portfolio.WinCount + e.portfolio.LossCount
	if totalTrades > 0 {
		winRate = float64(e.portfolio.WinCount) / float64(totalTrades) * 100
	}

	returnPct := (e.portfolio.Equity - e.portfolio.InitialBalance) / e.portfolio.InitialBalance * 100

	return map[string]interface{}{
		"initial_balance": e.portfolio.InitialBalance,
		"current_balance": e.portfolio.Balance,
		"equity":          e.portfolio.Equity,
		"total_pnl":       e.portfolio.TotalPnL,
		"return_pct":      returnPct,
		"open_positions":  len(e.portfolio.OpenPositions),
		"total_trades":    totalTrades,
		"wins":            e.portfolio.WinCount,
		"losses":          e.portfolio.LossCount,
		"win_rate":        winRate,
		"running_since":   e.portfolio.StartTime,
		"last_update":     e.portfolio.LastUpdate,
	}
}

// GetOpenPositions returns all open positions
func (e *Engine) GetOpenPositions() []PaperPosition {
	e.mu.RLock()
	defer e.mu.RUnlock()

	positions := make([]PaperPosition, len(e.portfolio.OpenPositions))
	copy(positions, e.portfolio.OpenPositions)
	return positions
}

// GetClosedTrades returns all closed trades
func (e *Engine) GetClosedTrades() []PaperTrade {
	e.mu.RLock()
	defer e.mu.RUnlock()

	trades := make([]PaperTrade, len(e.portfolio.ClosedTrades))
	copy(trades, e.portfolio.ClosedTrades)
	return trades
}

func parseInterval(interval string) time.Duration {
	switch interval {
	case "1m":
		return time.Minute
	case "5m":
		return 5 * time.Minute
	case "15m":
		return 15 * time.Minute
	case "30m":
		return 30 * time.Minute
	case "1h":
		return time.Hour
	case "4h":
		return 4 * time.Hour
	case "1d":
		return 24 * time.Hour
	default:
		return 4 * time.Hour
	}
}
