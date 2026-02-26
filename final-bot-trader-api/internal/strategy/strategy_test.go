package strategy

import (
	"testing"
	"time"

	"final-bot-trader-api/internal/exchange/model"
)

// Helper to create test candles with specific close prices
func createTestCandles(closes []float64) []model.Candle {
	candles := make([]model.Candle, len(closes))
	baseTime := time.Now().Add(-time.Hour * time.Duration(len(closes)))

	for i, close := range closes {
		candles[i] = model.Candle{
			OpenTime:  baseTime.Add(time.Hour * time.Duration(i)),
			Open:      close,
			High:      close * 1.01,
			Low:       close * 0.99,
			Close:     close,
			Volume:    1000,
			CloseTime: baseTime.Add(time.Hour * time.Duration(i+1)),
		}
	}
	return candles
}

// Create candles with specific OHLC values
func createOHLCCandles(data [][4]float64) []model.Candle {
	candles := make([]model.Candle, len(data))
	baseTime := time.Now().Add(-time.Hour * time.Duration(len(data)))

	for i, ohlc := range data {
		candles[i] = model.Candle{
			OpenTime:  baseTime.Add(time.Hour * time.Duration(i)),
			Open:      ohlc[0],
			High:      ohlc[1],
			Low:       ohlc[2],
			Close:     ohlc[3],
			Volume:    1000,
			CloseTime: baseTime.Add(time.Hour * time.Duration(i+1)),
		}
	}
	return candles
}

func TestSMACrossover(t *testing.T) {
	// Create a crossover pattern
	// Prices that will create a golden cross (fast SMA crosses above slow SMA)
	prices := []float64{
		100, 99, 98, 97, 96, 95, // Downtrend - slow SMA above fast
		96, 97, 98, 99, 100, 101, 102, 103, 104, 105, // Uptrend - fast SMA catches up
	}
	candles := createTestCandles(prices)

	strategy, err := NewSMACrossover("BTCUSDT", 3, 7)
	if err != nil {
		t.Fatalf("Failed to create strategy: %v", err)
	}

	if strategy.Name() != "SMA_Crossover(3,7)" {
		t.Errorf("Unexpected name: %s", strategy.Name())
	}

	if strategy.MinimumCandles() != 8 {
		t.Errorf("Expected minimum candles 8, got %d", strategy.MinimumCandles())
	}

	signal, err := strategy.Analyze(candles)
	// May or may not generate signal depending on exact crossover
	if err != nil && err != ErrNoSignal {
		t.Logf("Analysis result: %v", err)
	}
	if signal != nil {
		t.Logf("Signal generated: %+v", signal)
	}
}

func TestSMACrossoverInvalidParameters(t *testing.T) {
	// Fast period >= slow period should fail
	_, err := NewSMACrossover("BTCUSDT", 10, 5)
	if err == nil {
		t.Error("Expected error for fast >= slow")
	}

	// Zero period should fail
	_, err = NewSMACrossover("BTCUSDT", 0, 5)
	if err == nil {
		t.Error("Expected error for zero period")
	}
}

func TestSMACrossoverSetParameters(t *testing.T) {
	strategy, _ := NewSMACrossover("BTCUSDT", 5, 10)

	err := strategy.SetParameters(map[string]interface{}{
		"fast_period": 8,
		"slow_period": 20,
	})
	if err != nil {
		t.Fatalf("Failed to set parameters: %v", err)
	}

	params := strategy.Parameters()
	if params["fast_period"] != 8 {
		t.Errorf("Fast period not updated: %v", params["fast_period"])
	}
	if params["slow_period"] != 20 {
		t.Errorf("Slow period not updated: %v", params["slow_period"])
	}
}

func TestRSIStrategy(t *testing.T) {
	// Create prices that go into oversold territory then recover
	prices := make([]float64, 25)
	for i := range prices {
		if i < 15 {
			prices[i] = 100 - float64(i)*2 // Downtrend into oversold
		} else {
			prices[i] = prices[14] + float64(i-14)*3 // Recovery
		}
	}
	candles := createTestCandles(prices)

	strategy, err := NewRSIStrategy("BTCUSDT", 14)
	if err != nil {
		t.Fatalf("Failed to create strategy: %v", err)
	}

	if strategy.Name() != "RSI_Strategy(14,70,30)" {
		t.Errorf("Unexpected name: %s", strategy.Name())
	}

	signal, err := strategy.Analyze(candles)
	if err != nil && err != ErrNoSignal {
		t.Logf("Analysis result: %v", err)
	}
	if signal != nil {
		t.Logf("Signal generated: %+v", signal)
		if signal.Indicators["rsi"] == 0 {
			t.Error("RSI indicator not set in signal")
		}
	}
}

func TestRSIStrategyCustomLevels(t *testing.T) {
	strategy, err := NewRSIStrategyWithLevels("BTCUSDT", 14, 80, 20)
	if err != nil {
		t.Fatalf("Failed to create strategy: %v", err)
	}

	params := strategy.Parameters()
	if params["overbought"] != 80.0 {
		t.Errorf("Overbought level not set correctly: %v", params["overbought"])
	}
	if params["oversold"] != 20.0 {
		t.Errorf("Oversold level not set correctly: %v", params["oversold"])
	}
}

func TestRSIStrategyInvalidParameters(t *testing.T) {
	// Overbought <= Oversold should fail
	_, err := NewRSIStrategyWithLevels("BTCUSDT", 14, 30, 70)
	if err == nil {
		t.Error("Expected error for overbought <= oversold")
	}

	// Zero period should fail
	_, err = NewRSIStrategy("BTCUSDT", 0)
	if err == nil {
		t.Error("Expected error for zero period")
	}
}

func TestRiskManager(t *testing.T) {
	config := DefaultRiskConfig()
	rm, err := NewRiskManager(config)
	if err != nil {
		t.Fatalf("Failed to create risk manager: %v", err)
	}

	// Test position size calculation
	balance := 10000.0
	entryPrice := 100.0
	stopLoss := 98.0 // 2% stop loss

	positionSize := rm.CalculatePositionSize(balance, entryPrice, stopLoss)

	// With 2% max risk and 2% SL, position should be about 100% of balance
	expectedMax := balance * config.MaxPositionSize / entryPrice
	if positionSize > expectedMax {
		t.Errorf("Position size %f exceeds max %f", positionSize, expectedMax)
	}
	if positionSize <= 0 {
		t.Error("Position size should be positive")
	}
}

func TestRiskManagerStopLossCalculation(t *testing.T) {
	config := DefaultRiskConfig()
	rm, _ := NewRiskManager(config)

	// Create candles for ATR calculation
	data := make([][4]float64, 20)
	for i := range data {
		base := 100.0 + float64(i)
		data[i] = [4]float64{base, base + 2, base - 1, base + 0.5}
	}
	candles := createOHLCCandles(data)

	entryPrice := 120.0

	// Long position
	slLong, err := rm.CalculateStopLoss(candles, entryPrice, true)
	if err != nil {
		t.Fatalf("Failed to calculate SL: %v", err)
	}
	if slLong >= entryPrice {
		t.Errorf("Long SL should be below entry: SL=%f, Entry=%f", slLong, entryPrice)
	}

	// Short position
	slShort, err := rm.CalculateStopLoss(candles, entryPrice, false)
	if err != nil {
		t.Fatalf("Failed to calculate SL: %v", err)
	}
	if slShort <= entryPrice {
		t.Errorf("Short SL should be above entry: SL=%f, Entry=%f", slShort, entryPrice)
	}
}

func TestRiskManagerTakeProfitCalculation(t *testing.T) {
	config := DefaultRiskConfig()
	rm, _ := NewRiskManager(config)

	data := make([][4]float64, 20)
	for i := range data {
		base := 100.0 + float64(i)
		data[i] = [4]float64{base, base + 2, base - 1, base + 0.5}
	}
	candles := createOHLCCandles(data)

	entryPrice := 120.0

	// Long position
	tpLong, err := rm.CalculateTakeProfit(candles, entryPrice, true)
	if err != nil {
		t.Fatalf("Failed to calculate TP: %v", err)
	}
	if tpLong <= entryPrice {
		t.Errorf("Long TP should be above entry: TP=%f, Entry=%f", tpLong, entryPrice)
	}

	// Short position
	tpShort, err := rm.CalculateTakeProfit(candles, entryPrice, false)
	if err != nil {
		t.Fatalf("Failed to calculate TP: %v", err)
	}
	if tpShort >= entryPrice {
		t.Errorf("Short TP should be below entry: TP=%f, Entry=%f", tpShort, entryPrice)
	}
}

func TestRiskManagerValidateTrade(t *testing.T) {
	config := DefaultRiskConfig()
	rm, _ := NewRiskManager(config)

	balance := 10000.0

	// Valid trade with good R:R
	valid, reason := rm.ValidateTrade(100, 98, 106, balance) // R:R = 3:1
	if !valid {
		t.Errorf("Trade should be valid: %s", reason)
	}

	// Invalid: poor R:R
	valid, reason = rm.ValidateTrade(100, 98, 100.5, balance) // R:R = 0.25:1
	if valid {
		t.Error("Trade with poor R:R should be invalid")
	}

	// Invalid: SL too close
	valid, reason = rm.ValidateTrade(100, 100, 105, balance)
	if valid {
		t.Error("Trade with SL at entry should be invalid")
	}
}

func TestApplyRiskManagement(t *testing.T) {
	config := DefaultRiskConfig()
	rm, _ := NewRiskManager(config)

	data := make([][4]float64, 20)
	for i := range data {
		base := 100.0 + float64(i)
		data[i] = [4]float64{base, base + 2, base - 1, base + 0.5}
	}
	candles := createOHLCCandles(data)

	signal := &Signal{
		Type:  SignalBuy,
		Price: 120.0,
	}

	err := rm.ApplyRiskManagement(signal, candles, 10000.0)
	if err != nil {
		t.Fatalf("Failed to apply risk management: %v", err)
	}

	if signal.SL <= 0 {
		t.Error("SL not set")
	}
	if signal.TP <= 0 {
		t.Error("TP not set")
	}
	if signal.Quantity <= 0 {
		t.Error("Quantity not set")
	}

	// Verify SL is below entry for long
	if signal.SL >= signal.Price {
		t.Error("Long SL should be below entry price")
	}

	// Verify TP is above entry for long
	if signal.TP <= signal.Price {
		t.Error("Long TP should be above entry price")
	}
}

func TestSignalTypes(t *testing.T) {
	if SignalBuy.String() != "BUY" {
		t.Errorf("Unexpected SignalBuy string: %s", SignalBuy.String())
	}
	if SignalSell.String() != "SELL" {
		t.Errorf("Unexpected SignalSell string: %s", SignalSell.String())
	}
	if SignalNone.String() != "NONE" {
		t.Errorf("Unexpected SignalNone string: %s", SignalNone.String())
	}
}
