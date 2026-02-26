package backtest

import (
	"testing"
	"time"

	"final-bot-trader-api/internal/exchange/model"
	"final-bot-trader-api/internal/strategy"
)

// Create test candles with trend pattern
func createTrendCandles(basePrice float64, count int, uptrend bool) []model.Candle {
	candles := make([]model.Candle, count)
	baseTime := time.Now().Add(-time.Hour * time.Duration(count))
	price := basePrice

	for i := 0; i < count; i++ {
		if uptrend {
			price = basePrice + float64(i)*0.5
		} else {
			price = basePrice - float64(i)*0.5
		}

		candles[i] = model.Candle{
			OpenTime:  baseTime.Add(time.Hour * time.Duration(i)),
			Open:      price,
			High:      price * 1.02,
			Low:       price * 0.98,
			Close:     price * 1.01,
			Volume:    1000,
			CloseTime: baseTime.Add(time.Hour * time.Duration(i+1)),
		}
	}
	return candles
}

// Create candles with crossover pattern for SMA strategy
func createCrossoverCandles() []model.Candle {
	// Prices that create a crossover:
	// First downtrend, then uptrend (golden cross)
	prices := []float64{
		110, 108, 106, 104, 102, 100, 98, 96, 94, 92, // Downtrend
		93, 95, 97, 99, 101, 103, 105, 107, 109, 111, // Uptrend
		113, 115, 117, 119, 121, 123, 125, 127, 129, 131, // Continue up
	}

	candles := make([]model.Candle, len(prices))
	baseTime := time.Now().Add(-time.Hour * time.Duration(len(prices)))

	for i, price := range prices {
		candles[i] = model.Candle{
			OpenTime:  baseTime.Add(time.Hour * time.Duration(i)),
			Open:      price,
			High:      price * 1.01,
			Low:       price * 0.99,
			Close:     price,
			Volume:    1000,
			CloseTime: baseTime.Add(time.Hour * time.Duration(i+1)),
		}
	}
	return candles
}

func TestTrade(t *testing.T) {
	trade := &Trade{
		ID:         1,
		Symbol:     "BTCUSDT",
		Type:       TradeTypeLong,
		Status:     TradeStatusOpen,
		EntryPrice: 100,
		Quantity:   1,
		StopLoss:   95,
		TakeProfit: 110,
		EntryTime:  time.Now(),
	}

	// Test stop loss check
	if !trade.CheckStopLoss(94) {
		t.Error("Should trigger stop loss at 94")
	}
	if trade.CheckStopLoss(96) {
		t.Error("Should not trigger stop loss at 96")
	}

	// Test take profit check
	if !trade.CheckTakeProfit(111) {
		t.Error("Should trigger take profit at 111")
	}
	if trade.CheckTakeProfit(109) {
		t.Error("Should not trigger take profit at 109")
	}

	// Test unrealized PnL
	pnl := trade.UnrealizedPnL(105)
	if pnl != 5 {
		t.Errorf("Expected unrealized PnL of 5, got %f", pnl)
	}
}

func TestTradeClose(t *testing.T) {
	trade := &Trade{
		ID:         1,
		Symbol:     "BTCUSDT",
		Type:       TradeTypeLong,
		Status:     TradeStatusOpen,
		EntryPrice: 100,
		Quantity:   10,
		EntryTime:  time.Now(),
		Commission: 2,
	}

	exitTime := time.Now().Add(time.Hour)
	trade.Close(110, exitTime, "Take Profit")

	if trade.Status != TradeStatusClosed {
		t.Error("Trade should be closed")
	}

	// PnL = (110 - 100) * 10 - 2 = 98
	expectedPnL := 98.0
	if trade.PnL != expectedPnL {
		t.Errorf("Expected PnL %f, got %f", expectedPnL, trade.PnL)
	}

	if !trade.IsWinning() {
		t.Error("Trade should be winning")
	}
}

func TestShortTrade(t *testing.T) {
	trade := &Trade{
		ID:         1,
		Symbol:     "BTCUSDT",
		Type:       TradeTypeShort,
		Status:     TradeStatusOpen,
		EntryPrice: 100,
		Quantity:   10,
		StopLoss:   105,
		TakeProfit: 90,
		EntryTime:  time.Now(),
	}

	// Short SL is above entry
	if !trade.CheckStopLoss(106) {
		t.Error("Should trigger stop loss for short at 106")
	}

	// Short TP is below entry
	if !trade.CheckTakeProfit(89) {
		t.Error("Should trigger take profit for short at 89")
	}

	// Unrealized PnL for short
	pnl := trade.UnrealizedPnL(95)
	if pnl != 50 { // (100 - 95) * 10
		t.Errorf("Expected unrealized PnL of 50, got %f", pnl)
	}
}

func TestPortfolio(t *testing.T) {
	p := NewPortfolio(10000)

	if p.Balance != 10000 {
		t.Errorf("Expected balance 10000, got %f", p.Balance)
	}

	trade := &Trade{
		Symbol:     "BTCUSDT",
		Type:       TradeTypeLong,
		Status:     TradeStatusOpen,
		EntryPrice: 100,
		Quantity:   10,
		EntryTime:  time.Now(),
	}

	err := p.OpenPosition(trade)
	if err != nil {
		t.Fatalf("Failed to open position: %v", err)
	}

	if !p.HasOpenPosition() {
		t.Error("Should have open position")
	}

	// Update equity
	p.UpdateEquity(105) // Price increased
	if p.Equity <= p.Balance {
		t.Error("Equity should be higher than balance with profitable position")
	}

	// Close position
	err = p.ClosePosition(110, time.Now(), "Take Profit")
	if err != nil {
		t.Fatalf("Failed to close position: %v", err)
	}

	if p.HasOpenPosition() {
		t.Error("Should not have open position after closing")
	}

	if p.TotalTrades() != 1 {
		t.Errorf("Expected 1 trade, got %d", p.TotalTrades())
	}
}

func TestPortfolioMetrics(t *testing.T) {
	p := NewPortfolio(10000)

	// Add some trades
	trades := []struct {
		entry, exit float64
		qty         float64
	}{
		{100, 110, 10}, // Win: +100
		{100, 95, 10},  // Loss: -50
		{100, 108, 10}, // Win: +80
	}

	for _, tr := range trades {
		trade := &Trade{
			Type:       TradeTypeLong,
			EntryPrice: tr.entry,
			Quantity:   tr.qty,
		}
		p.OpenPosition(trade)
		p.ClosePosition(tr.exit, time.Now(), "Test")
	}

	if p.WinningTrades() != 2 {
		t.Errorf("Expected 2 winning trades, got %d", p.WinningTrades())
	}

	if p.LosingTrades() != 1 {
		t.Errorf("Expected 1 losing trade, got %d", p.LosingTrades())
	}

	winRate := p.WinRate()
	if winRate < 66 || winRate > 67 { // ~66.67%
		t.Errorf("Expected win rate ~66.67%%, got %f", winRate)
	}
}

func TestBacktestEngine(t *testing.T) {
	config := DefaultEngineConfig()
	config.InitialBalance = 10000

	engine, err := NewEngine(config)
	if err != nil {
		t.Fatalf("Failed to create engine: %v", err)
	}

	// Create strategy
	strat, err := strategy.NewSMACrossover("BTCUSDT", 5, 10)
	if err != nil {
		t.Fatalf("Failed to create strategy: %v", err)
	}

	// Create candles with crossover pattern
	candles := createCrossoverCandles()

	result, err := engine.Run(strat, candles, "BTCUSDT", "1h")
	if err != nil {
		t.Fatalf("Backtest failed: %v", err)
	}

	if result.TotalTrades < 0 {
		t.Error("Total trades should not be negative")
	}

	t.Logf("Backtest result: %d trades, %.2f%% return", result.TotalTrades, result.TotalReturn)
}

func TestBacktestMetrics(t *testing.T) {
	p := NewPortfolio(10000)

	// Simulate some trades
	for i := 0; i < 10; i++ {
		trade := &Trade{
			Type:       TradeTypeLong,
			EntryPrice: 100,
			Quantity:   10,
			EntryTime:  time.Now().Add(time.Hour * time.Duration(i)),
		}
		p.OpenPosition(trade)

		exitPrice := 100.0
		if i%3 == 0 {
			exitPrice = 95 // Loss
		} else {
			exitPrice = 105 + float64(i) // Win
		}
		p.ClosePosition(exitPrice, time.Now().Add(time.Hour*time.Duration(i+1)), "Test")
	}

	startDate := time.Now().Add(-24 * time.Hour)
	endDate := time.Now()

	result := CalculateMetrics(p, startDate, endDate, "TestStrategy", "BTCUSDT", "1h")

	if result.TotalTrades != 10 {
		t.Errorf("Expected 10 trades, got %d", result.TotalTrades)
	}

	summary := result.Summary()
	if summary["total_trades"] != 10 {
		t.Error("Summary should contain correct total trades")
	}

	t.Logf("Metrics: WinRate=%.2f%%, ProfitFactor=%.2f, Sharpe=%.2f",
		result.WinRate, result.ProfitFactor, result.SharpeRatio)
}

func TestQuickBacktest(t *testing.T) {
	strat, _ := strategy.NewSMACrossover("BTCUSDT", 3, 7)
	candles := createCrossoverCandles()

	result, err := QuickBacktest(strat, candles, "BTCUSDT", "1h", 10000)
	if err != nil {
		t.Fatalf("Quick backtest failed: %v", err)
	}

	if result.InitialBalance != 10000 {
		t.Errorf("Expected initial balance 10000, got %f", result.InitialBalance)
	}
}

func TestInsufficientData(t *testing.T) {
	config := DefaultEngineConfig()
	engine, _ := NewEngine(config)

	strat, _ := strategy.NewSMACrossover("BTCUSDT", 5, 10)

	// Only 5 candles, but strategy needs 11
	candles := createTrendCandles(100, 5, true)

	_, err := engine.Run(strat, candles, "BTCUSDT", "1h")
	if err == nil {
		t.Error("Expected error for insufficient data")
	}
}
