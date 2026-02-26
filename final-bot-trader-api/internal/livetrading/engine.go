package livetrading

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"final-bot-trader-api/internal/database"
	"final-bot-trader-api/internal/exchange"
	"final-bot-trader-api/internal/exchange/model"
	"final-bot-trader-api/internal/strategy"
	"final-bot-trader-api/internal/strategy/multifactor"
	"final-bot-trader-api/internal/telegram"
)

// Trade represents an executed trade
type Trade struct {
	ID            string    `json:"id"`
	Symbol        string    `json:"symbol"`
	Side          string    `json:"side"`
	EntryPrice    float64   `json:"entry_price"`
	Quantity      float64   `json:"quantity"`
	StopLoss      float64   `json:"stop_loss"`
	TakeProfit    float64   `json:"take_profit"`
	EntryTime     time.Time `json:"entry_time"`
	OrderID       int64     `json:"order_id"`
	PositionID    string    `json:"position_id"`
	Reason        string    `json:"reason"`
	Status        string    `json:"status"` // "OPEN", "CLOSED", "ERROR"
	ExitPrice     float64   `json:"exit_price,omitempty"`
	ExitTime      time.Time `json:"exit_time,omitempty"`
	PnL           float64   `json:"pnl,omitempty"`
	ExitReason    string    `json:"exit_reason,omitempty"`
}

// TradingState holds the current trading state
type TradingState struct {
	Trades         []Trade   `json:"trades"`
	TotalPnL       float64   `json:"total_pnl"`
	WinCount       int       `json:"win_count"`
	LossCount      int       `json:"loss_count"`
	StartTime      time.Time `json:"start_time"`
	LastUpdate     time.Time `json:"last_update"`
	DailyPnL       float64   `json:"daily_pnl"`
	DailyTradeCount int      `json:"daily_trade_count"`
	LastDailyReset time.Time `json:"last_daily_reset"`
}

// Config holds live trading configuration
type Config struct {
	Symbols           []string
	PositionSizeUSDT  float64 // Fixed USDT amount per trade (fallback)
	PositionSizePct   float64 // Position size as % of balance (e.g., 0.10 = 10%)
	Leverage          int
	Interval          string  // Deprecated: use PrimaryInterval
	PrimaryInterval   string  // Higher timeframe for trend (e.g., "4h")
	EntryInterval     string  // Lower timeframe for entries (e.g., "1h")
	UseMultiTimeframe bool    // Enable multi-timeframe analysis
	StateFile         string
	VolumeThreshold   float64
	MaxDailyLoss      float64 // Max daily loss in USDT
	MaxDailyTrades    int     // Max trades per day
	MaxOpenPositions  int     // Max simultaneous positions
	DryRun            bool    // If true, log signals but don't execute
}

// DefaultConfig returns default live trading configuration
func DefaultConfig() Config {
	return Config{
		// Optimized symbol list based on LIVE RESULTS (2026-02-09)
		// Only symbols with positive real PnL or good win rate
		Symbols: []string{
			"BTCUSDT",       // Live: -2.41 but 50% WR, keep per user request
			"ETHUSDT",       // Live: -2.91 but 64% WR, good potential
			"SOLUSDT",       // Live: +2.87, 100% WR - BEST PERFORMER
			"DOGEUSDT",      // Live: +0.04, 71% WR - profitable
			"FILUSDT",       // Live: +0.17, 67% WR - profitable
			// Removed based on live losses: WIFUSDT (-2.19), AAVEUSDT (-0.17)
		},
		PositionSizeUSDT:  12,    // Fallback: $12 per trade (was $16)
		PositionSizePct:   0.08,  // 8% of account balance per trade (was 10%)
		Leverage:          3,     // Conservative 3x leverage for live trading
		Interval:          "4h",  // Deprecated: kept for compatibility
		PrimaryInterval:   "4h",  // Higher TF for trend direction
		EntryInterval:     "1h",  // Lower TF for entry timing
		UseMultiTimeframe: true,  // Enable MTF by default
		StateFile:         "/app/data/live_trading_state.json",
		VolumeThreshold:   1.0,   // Require at least average volume
		MaxDailyLoss:      5,     // Stop if losing $5/day (3% of $165)
		MaxDailyTrades:    8,     // Max 8 trades per day (conservative)
		MaxOpenPositions:  2,     // Max 2 positions at once (reduce risk)
		DryRun:            false,
	}
}

// Engine is the live trading engine
type Engine struct {
	config         Config
	client         *exchange.BitunixClient
	telegram       *telegram.Client
	tradeRepo      *database.TradeRepository // Optional: for persisting trades
	circuitBreaker *TradingCircuitBreaker    // Risk management circuit breaker
	state          *TradingState
	strategies     map[string]*multifactor.MultiFactorStrategy // Single TF strategies
	mtfStrategies  map[string]*multifactor.MTFStrategy         // Multi TF strategies
	symbolInfo     map[string]*model.SymbolInfo
	mu             sync.RWMutex
	running        bool
	stopCh         chan struct{}
}

// NewEngine creates a new live trading engine
func NewEngine(client *exchange.BitunixClient, tg *telegram.Client, config Config) *Engine {
	state := &TradingState{
		Trades:         []Trade{},
		StartTime:      time.Now(),
		LastUpdate:     time.Now(),
		LastDailyReset: time.Now().Truncate(24 * time.Hour),
	}

	// Base strategy config
	stratConfig := multifactor.DefaultConfig()
	if config.VolumeThreshold > 0 {
		stratConfig.VolumeThreshold = config.VolumeThreshold
	}

	// Create strategies
	strategies := make(map[string]*multifactor.MultiFactorStrategy)
	mtfStrategies := make(map[string]*multifactor.MTFStrategy)

	// MTF config
	mtfConfig := multifactor.MTFConfig{
		PrimaryInterval:       config.PrimaryInterval,
		EntryInterval:         config.EntryInterval,
		StrategyConfig:        stratConfig,
		RequireTrendAlignment: true,
		MinPrimaryADX:         20,
	}

	for _, symbol := range config.Symbols {
		strategies[symbol] = multifactor.NewMultiFactorStrategy(symbol, stratConfig)
		mtfStrategies[symbol] = multifactor.NewMTFStrategy(symbol, mtfConfig)
	}

	// Initialize circuit breaker for risk management
	cbConfig := DefaultTradingCircuitBreakerConfig()
	// Estimate initial balance for circuit breaker
	initialBalance := config.PositionSizeUSDT * float64(config.MaxOpenPositions) * 10
	if config.PositionSizePct > 0 {
		initialBalance = config.PositionSizeUSDT / config.PositionSizePct // e.g. $16 / 0.10 = $160
	}
	circuitBreaker := NewTradingCircuitBreaker(cbConfig, tg, initialBalance)

	return &Engine{
		config:         config,
		client:         client,
		telegram:       tg,
		circuitBreaker: circuitBreaker,
		state:          state,
		strategies:     strategies,
		mtfStrategies:  mtfStrategies,
		symbolInfo:     make(map[string]*model.SymbolInfo),
		stopCh:         make(chan struct{}),
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

// SetTradeRepository sets the database repository for persisting trades
func (e *Engine) SetTradeRepository(repo *database.TradeRepository) {
	e.tradeRepo = repo
}

// Initialize sets up leverage and gets symbol info
func (e *Engine) Initialize(ctx context.Context) error {
	log.Println("Initializing live trading engine...")

	for _, symbol := range e.config.Symbols {
		// Get symbol info
		info, err := e.client.GetSymbolInfo(ctx, symbol)
		if err != nil {
			log.Printf("Warning: Could not get info for %s: %v", symbol, err)
			continue
		}
		e.symbolInfo[symbol] = info

		// Set leverage
		if err := e.client.SetLeverage(ctx, symbol, e.config.Leverage); err != nil {
			log.Printf("Warning: Could not set leverage for %s: %v", symbol, err)
		} else {
			log.Printf("Set %s leverage to %dx", symbol, e.config.Leverage)
		}

		time.Sleep(200 * time.Millisecond) // Rate limiting
	}

	log.Printf("Initialized %d symbols", len(e.symbolInfo))
	return nil
}

// Run starts the live trading engine
func (e *Engine) Run(ctx context.Context) error {
	e.mu.Lock()
	e.running = true
	e.mu.Unlock()

	// Determine check interval based on mode
	var intervalDuration time.Duration
	if e.config.UseMultiTimeframe {
		// With MTF, check at the entry timeframe interval
		intervalDuration = parseInterval(e.config.EntryInterval)
		log.Printf("Multi-timeframe mode: checking every %s (primary: %s, entry: %s)",
			e.config.EntryInterval, e.config.PrimaryInterval, e.config.EntryInterval)
	} else {
		// Legacy single timeframe mode
		intervalDuration = parseInterval(e.config.Interval)
	}

	ticker := time.NewTicker(intervalDuration)
	defer ticker.Stop()

	// Run immediately
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

// Stop stops the trading engine
func (e *Engine) Stop() {
	e.mu.Lock()
	e.running = false
	e.mu.Unlock()
	close(e.stopCh)
}

// ProcessOnce runs a single iteration
func (e *Engine) ProcessOnce(ctx context.Context) {
	e.processAllSymbols(ctx)
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
	// Check circuit breaker first
	if e.circuitBreaker != nil && !e.circuitBreaker.CanTrade() {
		return false
	}

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

	// Update open trades with position data
	positionMap := make(map[string]model.Position)
	for _, pos := range positions {
		if pos.PositionAmt != 0 {
			positionMap[pos.Symbol] = pos
		}
	}

	var closedTrades []Trade
	for i := range e.state.Trades {
		trade := &e.state.Trades[i]
		if trade.Status != "OPEN" {
			continue
		}

		if pos, exists := positionMap[trade.Symbol]; exists {
			trade.PositionID = pos.PositionID
			// Update entry price and quantity with real exchange data
			if pos.EntryPrice > 0 && pos.EntryPrice != trade.EntryPrice {
				log.Printf("[%s] Syncing entry price: %.6f -> %.6f", trade.Symbol, trade.EntryPrice, pos.EntryPrice)
				trade.EntryPrice = pos.EntryPrice
			}
			if pos.PositionAmt > 0 && pos.PositionAmt != trade.Quantity {
				log.Printf("[%s] Syncing quantity: %.4f -> %.4f", trade.Symbol, trade.Quantity, pos.PositionAmt)
				trade.Quantity = pos.PositionAmt
			}
		} else {
			// Position was closed (TP/SL hit)
			trade.Status = "CLOSED"
			trade.ExitTime = time.Now()
			closedTrades = append(closedTrades, *trade)
		}
	}

	e.mu.Unlock()

	// Calculate PnL for closed trades and send notifications
	for _, trade := range closedTrades {
		exitPrice, pnl, reason := e.calculateClosedTradePnL(ctx, &trade)
		exitTime := time.Now()

		// Update the trade in state
		e.mu.Lock()
		for i := range e.state.Trades {
			if e.state.Trades[i].ID == trade.ID {
				e.state.Trades[i].ExitPrice = exitPrice
				e.state.Trades[i].PnL = pnl
				e.state.Trades[i].ExitReason = reason
				e.state.Trades[i].ExitTime = exitTime

				// Update totals
				e.state.TotalPnL += pnl
				e.state.DailyPnL += pnl
				if pnl >= 0 {
					e.state.WinCount++
				} else {
					e.state.LossCount++
				}
				break
			}
		}
		e.mu.Unlock()

		// Update in database
		if e.tradeRepo != nil {
			dbTrade := &database.Trade{
				ID:        trade.ID,
				ExitPrice: sql.NullFloat64{Float64: exitPrice, Valid: true},
				PnL:       sql.NullFloat64{Float64: pnl, Valid: true},
				Status:    "CLOSED",
				ExitReason: sql.NullString{String: reason, Valid: true},
				ExitTime:  sql.NullTime{Time: exitTime, Valid: true},
			}
			if err := e.tradeRepo.Update(ctx, dbTrade); err != nil {
				log.Printf("Database error (trade update failed): %v", err)
			}
		}

		log.Printf("[%s] Position closed: %s | PnL: %+.4f USDT", trade.Symbol, reason, pnl)

		// Record trade result in circuit breaker
		if e.circuitBreaker != nil {
			e.circuitBreaker.RecordTrade(pnl)
		}

		// Send Telegram notification
		if e.telegram != nil {
			if pnl >= 0 {
				if err := e.telegram.NotifyTPHit(trade.Symbol, trade.Side, trade.EntryPrice, exitPrice, pnl); err != nil {
					log.Printf("Telegram notification error: %v", err)
				}
			} else {
				if err := e.telegram.NotifySLHit(trade.Symbol, trade.Side, trade.EntryPrice, exitPrice, pnl); err != nil {
					log.Printf("Telegram notification error: %v", err)
				}
			}
		}
	}
}

// calculateClosedTradePnL determines the exit price, PnL and reason for a closed trade
func (e *Engine) calculateClosedTradePnL(ctx context.Context, trade *Trade) (exitPrice, pnl float64, reason string) {
	// Get current price as best approximation of exit price
	currentPrice, err := e.client.GetPrice(ctx, trade.Symbol)
	if err != nil {
		log.Printf("Could not get price for %s: %v", trade.Symbol, err)
		currentPrice = 0
	}

	isLong := trade.Side == "LONG"

	// Use current price as exit price (position just closed, so current price is close to exit)
	if currentPrice > 0 {
		exitPrice = currentPrice
	} else if isLong {
		// Fallback: estimate based on TP/SL proximity
		exitPrice = trade.TakeProfit // optimistic fallback
	} else {
		exitPrice = trade.TakeProfit
	}

	// Calculate PnL with real prices
	if isLong {
		pnl = (exitPrice - trade.EntryPrice) * trade.Quantity
	} else {
		pnl = (trade.EntryPrice - exitPrice) * trade.Quantity
	}

	// Determine reason based on how close the exit is to TP or SL
	if isLong {
		tpDist := abs(exitPrice - trade.TakeProfit)
		slDist := abs(exitPrice - trade.StopLoss)
		if tpDist < slDist {
			reason = "Take Profit hit"
		} else {
			reason = "Stop Loss hit"
		}
	} else {
		tpDist := abs(exitPrice - trade.TakeProfit)
		slDist := abs(exitPrice - trade.StopLoss)
		if tpDist < slDist {
			reason = "Take Profit hit"
		} else {
			reason = "Stop Loss hit"
		}
	}

	// Check if trailing stop was the reason (SL was moved closer to entry)
	if isLong && trade.StopLoss > trade.EntryPrice {
		reason = "Trailing stop"
	} else if !isLong && trade.StopLoss < trade.EntryPrice {
		reason = "Trailing stop"
	}

	log.Printf("[%s] Trade closed: entry=%.6f exit=%.6f qty=%.4f pnl=%.4f reason=%s",
		trade.Symbol, trade.EntryPrice, exitPrice, trade.Quantity, pnl, reason)

	return exitPrice, pnl, reason
}

func abs(x float64) float64 {
	if x < 0 {
		return -x
	}
	return x
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

	var signal *strategy.Signal
	var currentPrice float64
	var err error

	if e.config.UseMultiTimeframe {
		// Multi-timeframe analysis
		signal, currentPrice, err = e.analyzeMultiTimeframe(ctx, symbol)
	} else {
		// Single timeframe analysis (legacy)
		signal, currentPrice, err = e.analyzeSingleTimeframe(ctx, symbol)
	}

	if err != nil {
		if err == strategy.ErrNoSignal {
			return nil
		}
		return err
	}

	if signal == nil {
		return nil
	}

	// Execute trade
	return e.executeTrade(ctx, symbol, signal, currentPrice)
}

func (e *Engine) analyzeSingleTimeframe(ctx context.Context, symbol string) (*strategy.Signal, float64, error) {
	// Get candles for single timeframe
	interval := e.config.Interval
	if interval == "" {
		interval = e.config.PrimaryInterval
	}

	candles, err := e.client.GetKlines(ctx, symbol, interval, 100, 0, 0)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get candles: %w", err)
	}

	if len(candles) < 60 {
		return nil, 0, fmt.Errorf("insufficient candles: %d", len(candles))
	}

	currentPrice := candles[len(candles)-1].Close

	strat := e.strategies[symbol]
	signal, err := strat.Analyze(candles)
	if err != nil {
		return nil, 0, err
	}

	return signal, currentPrice, nil
}

func (e *Engine) analyzeMultiTimeframe(ctx context.Context, symbol string) (*strategy.Signal, float64, error) {
	// Get primary (higher) timeframe candles
	primaryCandles, err := e.client.GetKlines(ctx, symbol, e.config.PrimaryInterval, 100, 0, 0)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get primary candles: %w", err)
	}

	if len(primaryCandles) < 50 {
		return nil, 0, fmt.Errorf("insufficient primary candles: %d", len(primaryCandles))
	}

	// Small delay to avoid rate limiting
	time.Sleep(100 * time.Millisecond)

	// Get entry (lower) timeframe candles
	entryCandles, err := e.client.GetKlines(ctx, symbol, e.config.EntryInterval, 100, 0, 0)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to get entry candles: %w", err)
	}

	if len(entryCandles) < 60 {
		return nil, 0, fmt.Errorf("insufficient entry candles: %d", len(entryCandles))
	}

	currentPrice := entryCandles[len(entryCandles)-1].Close

	// Use MTF strategy
	mtfStrat := e.mtfStrategies[symbol]
	signal, err := mtfStrat.AnalyzeMTF(primaryCandles, entryCandles)
	if err != nil {
		return nil, 0, err
	}

	return signal, currentPrice, nil
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

func (e *Engine) executeTrade(ctx context.Context, symbol string, signal *strategy.Signal, currentPrice float64) error {
	info := e.symbolInfo[symbol]
	if info == nil {
		return fmt.Errorf("no symbol info for %s", symbol)
	}

	// Calculate position size from account balance
	positionSize := e.config.PositionSizeUSDT // fallback
	if e.config.PositionSizePct > 0 {
		accountInfo, accErr := e.client.GetAccountInfo(ctx)
		if accErr == nil && accountInfo.AvailableBalance > 0 {
			positionSize = accountInfo.AvailableBalance * e.config.PositionSizePct
			log.Printf("[%s] Balance: $%.2f | Position size: $%.2f (%.0f%%)", symbol, accountInfo.AvailableBalance, positionSize, e.config.PositionSizePct*100)
		} else {
			log.Printf("[%s] Could not get balance, using fallback $%.2f", symbol, positionSize)
		}
	}

	// Apply circuit breaker position size adjustment
	if e.circuitBreaker != nil {
		multiplier := e.circuitBreaker.GetPositionSizeMultiplier()
		if multiplier < 1.0 {
			positionSize *= multiplier
			log.Printf("[%s] Recovery mode: using %.0f%% position size ($%.2f)", symbol, multiplier*100, positionSize)
		}
	}
	notionalValue := positionSize * float64(e.config.Leverage)
	quantity := notionalValue / currentPrice

	// Determine side
	side := "BUY"
	if signal.Type == strategy.SignalSell {
		side = "SELL"
	}

	sideStr := "LONG"
	if signal.Type == strategy.SignalSell {
		sideStr = "SHORT"
	}

	log.Printf("\n[%s] 📊 SIGNAL: %s @ %.4f", time.Now().Format("2006-01-02 15:04"), symbol, currentPrice)
	log.Printf("         Side: %s | Qty: %.4f | SL: %.4f | TP: %.4f", sideStr, quantity, signal.SL, signal.TP)
	log.Printf("         Reason: %s", signal.Reason)

	if e.config.DryRun {
		log.Printf("         [DRY RUN] Order not executed")
		return nil
	}

	// Round quantity and create order
	quantity = info.RoundQuantity(quantity)

	// For small-priced coins, use raw TP/SL values if RoundPrice returns 0
	tpPrice := info.RoundPrice(signal.TP)
	slPrice := info.RoundPrice(signal.SL)
	pricePrecision := info.PricePrecision()

	// Handle very small prices (like 1000PEPEUSDT at 0.004)
	if tpPrice == 0 && signal.TP > 0 {
		tpPrice = signal.TP
		pricePrecision = 6 // Use high precision for small prices
	}
	if slPrice == 0 && signal.SL > 0 {
		slPrice = signal.SL
		pricePrecision = 6
	}

	order := &model.OrderRequest{
		Symbol:            symbol,
		Side:              side,
		Type:              "MARKET",
		Quantity:          quantity,
		TP:                tpPrice,
		SL:                slPrice,
		QuantityPrecision: info.QuantityPrecision(),
		PricePrecision:    pricePrecision,
		TradeSide:         "OPEN",
	}

	// Place order
	resp, err := e.client.PlaceOrder(ctx, order)
	if err != nil {
		log.Printf("         ❌ Order failed: %v", err)
		return err
	}

	log.Printf("         ✅ Order placed: ID=%d", resp.OrderID)

	// Wait for order to fill and get real execution data from exchange
	realEntryPrice := currentPrice
	realQuantity := quantity
	time.Sleep(2 * time.Second) // Wait for order to be filled

	positions, posErr := e.client.GetPositions(ctx)
	if posErr == nil {
		for _, pos := range positions {
			if pos.Symbol == symbol && pos.PositionAmt != 0 {
				if pos.EntryPrice > 0 {
					log.Printf("[%s] Real fill price: %.6f (signal price was: %.6f)", symbol, pos.EntryPrice, currentPrice)
					realEntryPrice = pos.EntryPrice
				}
				if pos.PositionAmt > 0 {
					realQuantity = pos.PositionAmt
					log.Printf("[%s] Real fill quantity: %.4f (requested: %.4f)", symbol, realQuantity, quantity)
				}
				break
			}
		}
	} else {
		log.Printf("[%s] Warning: could not get fill price, using signal price: %v", symbol, posErr)
	}

	// Record trade with real execution data
	e.mu.Lock()
	trade := Trade{
		ID:         fmt.Sprintf("%s-%d", symbol, time.Now().UnixNano()),
		Symbol:     symbol,
		Side:       sideStr,
		EntryPrice: realEntryPrice,
		Quantity:   realQuantity,
		StopLoss:   signal.SL,
		TakeProfit: signal.TP,
		EntryTime:  time.Now(),
		OrderID:    resp.OrderID,
		Reason:     signal.Reason,
		Status:     "OPEN",
	}
	e.state.Trades = append(e.state.Trades, trade)
	e.state.DailyTradeCount++
	e.mu.Unlock()

	// Persist to database
	if e.tradeRepo != nil {
		dbTrade := &database.Trade{
			ID:         trade.ID,
			Symbol:     trade.Symbol,
			Side:       trade.Side,
			EntryPrice: trade.EntryPrice,
			Quantity:   trade.Quantity,
			StopLoss:   sql.NullFloat64{Float64: trade.StopLoss, Valid: trade.StopLoss > 0},
			TakeProfit: sql.NullFloat64{Float64: trade.TakeProfit, Valid: trade.TakeProfit > 0},
			Status:     trade.Status,
			Reason:     sql.NullString{String: trade.Reason, Valid: trade.Reason != ""},
			OrderID:    sql.NullInt64{Int64: trade.OrderID, Valid: trade.OrderID > 0},
			EntryTime:  trade.EntryTime,
		}
		if err := e.tradeRepo.Create(ctx, dbTrade); err != nil {
			log.Printf("Database error (trade not persisted): %v", err)
		}
	}

	// Send Telegram notification
	if e.telegram != nil {
		if err := e.telegram.NotifyPositionOpened(symbol, sideStr, realEntryPrice, realQuantity, signal.TP, signal.SL, signal.Reason); err != nil {
			log.Printf("Telegram notification error: %v", err)
		}
	}

	return nil
}

// CloseAllPositions closes all open positions
func (e *Engine) CloseAllPositions(ctx context.Context) error {
	positions, err := e.client.GetPositions(ctx)
	if err != nil {
		return err
	}

	for _, pos := range positions {
		if pos.PositionAmt == 0 {
			continue
		}

		log.Printf("Closing position: %s (ID: %s)", pos.Symbol, pos.PositionID)

		if err := e.client.FlashClosePosition(ctx, pos.PositionID); err != nil {
			log.Printf("Error closing %s: %v", pos.Symbol, err)
		} else {
			log.Printf("Closed %s successfully", pos.Symbol)
		}

		time.Sleep(500 * time.Millisecond)
	}

	return nil
}

// CloseTradeFictitious closes a trade only in the application/database without calling Bitunix
// This is useful when positions are manually closed on the exchange or get out of sync
func (e *Engine) CloseTradeFictitious(ctx context.Context, tradeID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Find the trade
	var trade *Trade
	var tradeIndex int = -1
	for i := range e.state.Trades {
		if e.state.Trades[i].ID == tradeID && e.state.Trades[i].Status == "OPEN" {
			trade = &e.state.Trades[i]
			tradeIndex = i
			break
		}
	}

	if trade == nil {
		return fmt.Errorf("trade %s not found or already closed", tradeID)
	}

	// Get current price to calculate PnL
	currentPrice, err := e.client.GetPrice(ctx, trade.Symbol)
	if err != nil {
		log.Printf("Could not get price for %s: %v, using entry price", trade.Symbol, err)
		currentPrice = trade.EntryPrice // Fallback to entry price (PnL = 0)
	}

	// Calculate PnL
	isLong := trade.Side == "LONG"
	var pnl float64
	if isLong {
		pnl = (currentPrice - trade.EntryPrice) * trade.Quantity
	} else {
		pnl = (trade.EntryPrice - currentPrice) * trade.Quantity
	}

	// Determine exit reason based on price
	reason := "Manual close (fictitious)"
	if currentPrice > 0 {
		if isLong {
			tpDist := abs(currentPrice - trade.TakeProfit)
			slDist := abs(currentPrice - trade.StopLoss)
			if tpDist < slDist {
				reason = "Take Profit (fictitious)"
			} else {
				reason = "Stop Loss (fictitious)"
			}
		} else {
			tpDist := abs(currentPrice - trade.TakeProfit)
			slDist := abs(currentPrice - trade.StopLoss)
			if tpDist < slDist {
				reason = "Take Profit (fictitious)"
			} else {
				reason = "Stop Loss (fictitious)"
			}
		}
	}

	exitTime := time.Now()

	// Update the trade in state
	e.state.Trades[tradeIndex].Status = "CLOSED"
	e.state.Trades[tradeIndex].ExitPrice = currentPrice
	e.state.Trades[tradeIndex].PnL = pnl
	e.state.Trades[tradeIndex].ExitReason = reason
	e.state.Trades[tradeIndex].ExitTime = exitTime

	// Update totals
	e.state.TotalPnL += pnl
	e.state.DailyPnL += pnl
	if pnl >= 0 {
		e.state.WinCount++
	} else {
		e.state.LossCount++
	}

	// Update in database
	if e.tradeRepo != nil {
		dbTrade := &database.Trade{
			ID:        trade.ID,
			ExitPrice: sql.NullFloat64{Float64: currentPrice, Valid: true},
			PnL:       sql.NullFloat64{Float64: pnl, Valid: true},
			Status:    "CLOSED",
			ExitReason: sql.NullString{String: reason, Valid: true},
			ExitTime:  sql.NullTime{Time: exitTime, Valid: true},
		}
		if err := e.tradeRepo.Update(ctx, dbTrade); err != nil {
			log.Printf("Database error (fictitious close update failed): %v", err)
			return fmt.Errorf("database update failed: %w", err)
		}
	}

	log.Printf("[%s] Trade closed fictitiously: entry=%.6f exit=%.6f qty=%.4f pnl=%.4f reason=%s",
		trade.Symbol, trade.EntryPrice, currentPrice, trade.Quantity, pnl, reason)

	// Record trade result in circuit breaker
	if e.circuitBreaker != nil {
		e.circuitBreaker.RecordTrade(pnl)
	}

	// Note: Trailing stop cleanup is handled by the TrailingStopManager
	// which monitors the engine state independently

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

	status := map[string]interface{}{
		"running":           e.running,
		"dry_run":           e.config.DryRun,
		"position_size_pct": e.config.PositionSizePct,
		"position_size_fallback": e.config.PositionSizeUSDT,
		"leverage":          e.config.Leverage,
		"open_positions":    openTrades,
		"closed_trades":     closedTrades,
		"total_pnl":         e.state.TotalPnL,
		"daily_pnl":         e.state.DailyPnL,
		"daily_trades":      e.state.DailyTradeCount,
		"win_count":         e.state.WinCount,
		"loss_count":        e.state.LossCount,
		"win_rate":          winRate,
		"start_time":        e.state.StartTime,
		"last_update":       e.state.LastUpdate,
	}

	// Add circuit breaker status
	if e.circuitBreaker != nil {
		status["circuit_breaker"] = e.circuitBreaker.GetStatus()
	}

	return status
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

// NotifyStartup sends a Telegram notification that the bot has started
func (e *Engine) NotifyStartup() {
	if e.telegram == nil {
		return
	}
	mode := "LIVE"
	if e.config.DryRun {
		mode = "DRY RUN"
	}
	if err := e.telegram.NotifyStartup(mode, e.config.PositionSizeUSDT, e.config.Leverage); err != nil {
		log.Printf("Telegram startup notification error: %v", err)
	}
}

// NotifyShutdown sends a Telegram notification that the bot is stopping
func (e *Engine) NotifyShutdown() {
	if e.telegram == nil {
		return
	}
	openCount := e.countOpenPositions()
	if err := e.telegram.NotifyShutdown(openCount, e.state.TotalPnL); err != nil {
		log.Printf("Telegram shutdown notification error: %v", err)
	}
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

// UpdateTradeStopLoss updates the stop loss for a trade (used by trailing stop)
func (e *Engine) UpdateTradeStopLoss(tradeID string, newStopLoss float64) {
	e.mu.Lock()
	defer e.mu.Unlock()

	for i := range e.state.Trades {
		if e.state.Trades[i].ID == tradeID && e.state.Trades[i].Status == "OPEN" {
			e.state.Trades[i].StopLoss = newStopLoss
			log.Printf("[%s] Stop loss updated to %.6f", e.state.Trades[i].Symbol, newStopLoss)
			break
		}
	}
}

// GetClient returns the exchange client (for trailing stop manager)
func (e *Engine) GetClient() *exchange.BitunixClient {
	return e.client
}

// GetCircuitBreaker returns the circuit breaker instance
func (e *Engine) GetCircuitBreaker() *TradingCircuitBreaker {
	return e.circuitBreaker
}

// ResetCircuitBreaker resets the circuit breaker to normal state
func (e *Engine) ResetCircuitBreaker() {
	if e.circuitBreaker != nil {
		e.circuitBreaker.Reset()
	}
}

// SetCircuitBreakerBalance updates the circuit breaker's initial balance
func (e *Engine) SetCircuitBreakerBalance(balance float64) {
	if e.circuitBreaker != nil {
		e.circuitBreaker.SetInitialBalance(balance)
	}
}

// GetAllTrades returns all trades (open and closed)
func (e *Engine) GetAllTrades() []Trade {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make([]Trade, len(e.state.Trades))
	copy(result, e.state.Trades)
	return result
}

// GetClosedTrades returns only closed trades
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

// UpdateTradeClosed updates a closed trade's prices and recalculates PnL
// This is useful when prices were incorrectly recorded
func (e *Engine) UpdateTradeClosed(ctx context.Context, tradeID string, newEntryPrice, newExitPrice *float64) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	// Find the trade
	var trade *Trade
	var tradeIndex int = -1
	for i := range e.state.Trades {
		if e.state.Trades[i].ID == tradeID {
			trade = &e.state.Trades[i]
			tradeIndex = i
			break
		}
	}

	if trade == nil {
		return fmt.Errorf("trade %s not found", tradeID)
	}

	if trade.Status != "CLOSED" {
		return fmt.Errorf("trade %s is not closed (status: %s)", tradeID, trade.Status)
	}

	// Store old values for recalculation
	oldPnL := trade.PnL
	oldEntryPrice := trade.EntryPrice
	oldExitPrice := trade.ExitPrice

	// Update prices if provided
	if newEntryPrice != nil {
		trade.EntryPrice = *newEntryPrice
	}
	if newExitPrice != nil {
		trade.ExitPrice = *newExitPrice
	}

	// Recalculate PnL
	isLong := trade.Side == "LONG"
	var newPnL float64
	if isLong {
		newPnL = (trade.ExitPrice - trade.EntryPrice) * trade.Quantity
	} else {
		newPnL = (trade.EntryPrice - trade.ExitPrice) * trade.Quantity
	}

	// Update the trade in state
	e.state.Trades[tradeIndex].EntryPrice = trade.EntryPrice
	e.state.Trades[tradeIndex].ExitPrice = trade.ExitPrice
	e.state.Trades[tradeIndex].PnL = newPnL

	// Update totals: subtract old PnL and add new PnL
	pnlDiff := newPnL - oldPnL
	e.state.TotalPnL += pnlDiff
	e.state.DailyPnL += pnlDiff

	// Update win/loss counts if PnL sign changed
	wasWin := oldPnL >= 0
	isWin := newPnL >= 0
	if wasWin != isWin {
		if wasWin {
			// Was a win, now is a loss
			e.state.WinCount--
			e.state.LossCount++
		} else {
			// Was a loss, now is a win
			e.state.WinCount++
			e.state.LossCount--
		}
	}

	// Update in database
	if e.tradeRepo != nil {
		dbTrade := &database.Trade{
			ID:        trade.ID,
			EntryPrice: trade.EntryPrice,
			ExitPrice: sql.NullFloat64{Float64: trade.ExitPrice, Valid: true},
			PnL:       sql.NullFloat64{Float64: newPnL, Valid: true},
			Status:    "CLOSED",
			ExitReason: sql.NullString{String: trade.ExitReason, Valid: trade.ExitReason != ""},
			ExitTime:  sql.NullTime{Time: trade.ExitTime, Valid: !trade.ExitTime.IsZero()},
		}
		if err := e.tradeRepo.Update(ctx, dbTrade); err != nil {
			log.Printf("Database error (trade update failed): %v", err)
			// Revert changes on error
			e.state.Trades[tradeIndex].EntryPrice = oldEntryPrice
			e.state.Trades[tradeIndex].ExitPrice = oldExitPrice
			e.state.Trades[tradeIndex].PnL = oldPnL
			e.state.TotalPnL -= pnlDiff
			e.state.DailyPnL -= pnlDiff
			if wasWin != isWin {
				if wasWin {
					e.state.WinCount++
					e.state.LossCount--
				} else {
					e.state.WinCount--
					e.state.LossCount++
				}
			}
			return fmt.Errorf("database update failed: %w", err)
		}
	}

	log.Printf("[%s] Trade updated: entry=%.6f exit=%.6f old_pnl=%.4f new_pnl=%.4f",
		trade.Symbol, trade.EntryPrice, trade.ExitPrice, oldPnL, newPnL)

	return nil
}

// GetTradesBySymbol returns trade stats grouped by symbol
func (e *Engine) GetTradesBySymbol() map[string]map[string]interface{} {
	e.mu.RLock()
	defer e.mu.RUnlock()

	result := make(map[string]map[string]interface{})
	for _, t := range e.state.Trades {
		if t.Status != "CLOSED" {
			continue
		}
		if _, exists := result[t.Symbol]; !exists {
			result[t.Symbol] = map[string]interface{}{
				"trades":    0,
				"wins":      0,
				"losses":    0,
				"total_pnl": 0.0,
				"win_rate":  0.0,
				"avg_pnl":   0.0,
			}
		}
		stats := result[t.Symbol]
		trades := stats["trades"].(int) + 1
		stats["trades"] = trades
		totalPnl := stats["total_pnl"].(float64) + t.PnL
		stats["total_pnl"] = totalPnl
		if t.PnL >= 0 {
			stats["wins"] = stats["wins"].(int) + 1
		} else {
			stats["losses"] = stats["losses"].(int) + 1
		}
		wins := stats["wins"].(int)
		if trades > 0 {
			stats["win_rate"] = float64(wins) / float64(trades) * 100
			stats["avg_pnl"] = totalPnl / float64(trades)
		}
		result[t.Symbol] = stats
	}
	return result
}

// GetEquityCurve returns equity curve data points
func (e *Engine) GetEquityCurve() []map[string]interface{} {
	e.mu.RLock()
	defer e.mu.RUnlock()

	var curve []map[string]interface{}
	cumPnl := 0.0
	for _, t := range e.state.Trades {
		if t.Status != "CLOSED" {
			continue
		}
		cumPnl += t.PnL
		curve = append(curve, map[string]interface{}{
			"timestamp": t.ExitTime.Format(time.RFC3339),
			"equity":    cumPnl,
			"trade_pnl": t.PnL,
		})
	}
	return curve
}
