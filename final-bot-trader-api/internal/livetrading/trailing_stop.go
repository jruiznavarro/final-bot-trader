package livetrading

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"final-bot-trader-api/internal/exchange"
	"final-bot-trader-api/internal/telegram"
)

// TrailingStopConfig holds trailing stop configuration
type TrailingStopConfig struct {
	Enabled          bool    // Enable trailing stop
	ActivationPct    float64 // Minimum profit % to activate trailing (e.g., 0.5 = 0.5%)
	TrailPct         float64 // Trail distance as % of price (e.g., 0.3 = 0.3%)
	CheckIntervalSec int     // How often to check prices (seconds)
}

// DefaultTrailingStopConfig returns default trailing stop configuration.
// Parameters are calibrated to the multifactor strategy's ATR-based TP/SL:
//   - Strategy SL = ATR×2.2 (~4.4% avg) | TP = ATR×3.3 (~6.6% avg) → R:R 1:1.5
//   - Activation at 4.0% = ~60% of the TP target: trailing only fires when the
//     trade has already demonstrated strong directional movement.
//   - Trail of 1.5% gives enough buffer for 4h crypto volatility without
//     cutting the position before it reaches the TP.
//   - Minimum locked profit when trailing fires: 4.0% - 1.5% = 2.5%
//     (vs -4.4% SL = still 2.5:4.4 favorable, and most wins reach TP first).
func DefaultTrailingStopConfig() TrailingStopConfig {
	return TrailingStopConfig{
		Enabled:          true,
		ActivationPct:    4.0, // Activate at ~60% of TP target (~6.6%)
		TrailPct:         1.5, // 1.5% buffer — enough for 4h crypto volatility
		CheckIntervalSec: 15,  // Check every 15 seconds
	}
}

// TrailingStopManager manages trailing stops for open positions.
// Trailing progress (activation, best price, trail level) is persisted on each
// Trade via the engine, so a bot restart resumes trails where they were.
type TrailingStopManager struct {
	config   TrailingStopConfig
	client   *exchange.BitunixClient
	engine   *Engine
	telegram *telegram.Client
	stopCh   chan struct{}
	running  bool
	mu       sync.RWMutex
}

// NewTrailingStopManager creates a new trailing stop manager
func NewTrailingStopManager(client *exchange.BitunixClient, engine *Engine, tg *telegram.Client, config TrailingStopConfig) *TrailingStopManager {
	return &TrailingStopManager{
		config:   config,
		client:   client,
		engine:   engine,
		telegram: tg,
		stopCh:   make(chan struct{}),
	}
}

// Start starts the trailing stop manager
func (m *TrailingStopManager) Start(ctx context.Context) {
	if !m.config.Enabled {
		log.Println("Trailing stop: DISABLED")
		return
	}

	m.running = true
	log.Printf("Trailing stop: ENABLED (activation: %.2f%%, trail: %.2f%%)",
		m.config.ActivationPct, m.config.TrailPct)

	ticker := time.NewTicker(time.Duration(m.config.CheckIntervalSec) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.running = false
			return
		case <-m.stopCh:
			m.running = false
			return
		case <-ticker.C:
			m.checkAndUpdateStops(ctx)
		}
	}
}

// Stop stops the trailing stop manager
func (m *TrailingStopManager) Stop() {
	if m.running {
		close(m.stopCh)
	}
}

func (m *TrailingStopManager) checkAndUpdateStops(ctx context.Context) {
	openTrades := m.engine.GetOpenTrades()
	if len(openTrades) == 0 {
		return
	}

	for _, trade := range openTrades {
		if err := m.checkTradeTrailingStop(ctx, &trade); err != nil {
			log.Printf("[%s] Trailing stop error: %v", trade.Symbol, err)
		}
	}
}

func (m *TrailingStopManager) checkTradeTrailingStop(ctx context.Context, trade *Trade) error {
	// Get current price
	currentPrice, err := m.client.GetPrice(ctx, trade.Symbol)
	if err != nil {
		return err
	}

	isLong := trade.Side == "LONG"

	// Calculate current profit percentage
	var profitPct float64
	if isLong {
		profitPct = ((currentPrice - trade.EntryPrice) / trade.EntryPrice) * 100
	} else {
		profitPct = ((trade.EntryPrice - currentPrice) / trade.EntryPrice) * 100
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if trailing stop should be activated (state lives on the trade,
	// so a restart picks up where it left off)
	if !trade.TrailActive {
		if profitPct < m.config.ActivationPct {
			return nil // Not enough profit to activate
		}
		// Activate trailing stop
		level := m.calculateTrailingLevel(currentPrice, isLong)
		m.engine.UpdateTradeTrailing(trade.ID, currentPrice, level)
		log.Printf("[%s] 🎯 Trailing stop ACTIVATED at %.2f%% profit (price: %.6f, trail: %.6f)",
			trade.Symbol, profitPct, currentPrice, level)
		return nil
	}

	// Trailing stop is active - track best price.
	// trade.StopLoss holds the current trail level.
	bestPrice := trade.TrailBest
	trailingLevel := trade.StopLoss

	priceImproved := (isLong && currentPrice > bestPrice) || (!isLong && currentPrice < bestPrice)
	if priceImproved {
		bestPrice = currentPrice
		newLevel := m.calculateTrailingLevel(bestPrice, isLong)

		// Only update if new level is better (protects more profit)
		if (isLong && newLevel > trailingLevel) || (!isLong && newLevel < trailingLevel) {
			currentProfitPct := m.calculateProfitPct(trade.EntryPrice, newLevel, isLong)
			log.Printf("[%s] 📈 Trailing stop moved: %.6f -> %.6f (locks %.2f%% profit)",
				trade.Symbol, trailingLevel, newLevel, currentProfitPct)
			trailingLevel = newLevel
		}
		m.engine.UpdateTradeTrailing(trade.ID, bestPrice, trailingLevel)
	}

	// Check if trailing stop was hit
	stopHit := (isLong && currentPrice <= trailingLevel) || (!isLong && currentPrice >= trailingLevel)

	if stopHit {
		log.Printf("[%s] 🛑 TRAILING STOP HIT! Closing position at %.6f (trail level: %.6f)",
			trade.Symbol, currentPrice, trailingLevel)

		// Close the position
		if err := m.closePosition(ctx, trade, currentPrice, profitPct); err != nil {
			log.Printf("[%s] Error closing position: %v", trade.Symbol, err)
			return err
		}
	}

	return nil
}

func (m *TrailingStopManager) calculateTrailingLevel(price float64, isLong bool) float64 {
	if isLong {
		return price * (1 - m.config.TrailPct/100)
	}
	return price * (1 + m.config.TrailPct/100)
}

func (m *TrailingStopManager) calculateProfitPct(entryPrice, exitPrice float64, isLong bool) float64 {
	if isLong {
		return ((exitPrice - entryPrice) / entryPrice) * 100
	}
	return ((entryPrice - exitPrice) / entryPrice) * 100
}

func (m *TrailingStopManager) closePosition(ctx context.Context, trade *Trade, exitPrice, profitPct float64) error {
	// Get position ID from exchange
	positions, err := m.client.GetPositions(ctx)
	if err != nil {
		return err
	}

	var positionID string
	for _, pos := range positions {
		if pos.Symbol == trade.Symbol && pos.PositionAmt != 0 {
			positionID = pos.PositionID
			break
		}
	}

	if positionID == "" {
		log.Printf("[%s] Position already closed or not found", trade.Symbol)
		return nil
	}

	// Flash close the position
	if err := m.client.FlashClosePosition(ctx, positionID); err != nil {
		return err
	}

	// Calculate actual PnL
	isLong := trade.Side == "LONG"
	var pnl float64
	if isLong {
		pnl = (exitPrice - trade.EntryPrice) * trade.Quantity
	} else {
		pnl = (trade.EntryPrice - exitPrice) * trade.Quantity
	}

	log.Printf("[%s] ✅ Position closed by trailing stop | PnL: %+.4f USDT (%.2f%%)",
		trade.Symbol, pnl, profitPct)

	// Send Telegram notification
	if m.telegram != nil && m.telegram.IsConfigured() {
		msg := fmt.Sprintf("🎯 *TRAILING STOP HIT*\n\n"+
			"Symbol: `%s`\n"+
			"Side: %s\n"+
			"Entry: `%.6f`\n"+
			"Exit: `%.6f`\n"+
			"PnL: `%+.4f USDT` (%.2f%%)\n\n"+
			"_Profit protected by trailing stop_",
			trade.Symbol, trade.Side, trade.EntryPrice, exitPrice, pnl, profitPct)
		m.telegram.SendMessage(msg)
	}

	return nil
}
