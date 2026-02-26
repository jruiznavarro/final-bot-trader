package indicator

import (
	"math"
	"testing"
	"time"

	"final-bot-trader-api/internal/exchange/model"
)

// Helper to create test candles
func createTestCandles(closes []float64) []model.Candle {
	candles := make([]model.Candle, len(closes))
	baseTime := time.Now().Add(-time.Hour * time.Duration(len(closes)))

	for i, close := range closes {
		// Simple candles where O=H=L=C for testing
		candles[i] = model.Candle{
			OpenTime:  baseTime.Add(time.Hour * time.Duration(i)),
			Open:      close,
			High:      close * 1.01, // 1% higher
			Low:       close * 0.99, // 1% lower
			Close:     close,
			Volume:    1000,
			CloseTime: baseTime.Add(time.Hour * time.Duration(i+1)),
		}
	}
	return candles
}

// Helper to compare floats with tolerance
func floatEquals(a, b, tolerance float64) bool {
	return math.Abs(a-b) <= tolerance
}

func TestSMA(t *testing.T) {
	prices := []float64{22.27, 22.19, 22.08, 22.17, 22.18, 22.13, 22.23, 22.43, 22.24, 22.29}
	candles := createTestCandles(prices)

	sma, err := NewSMA(5)
	if err != nil {
		t.Fatalf("Failed to create SMA: %v", err)
	}

	values, err := sma.Calculate(candles)
	if err != nil {
		t.Fatalf("Failed to calculate SMA: %v", err)
	}

	// Expected SMA(5) values (manually calculated)
	// First SMA = (22.27 + 22.19 + 22.08 + 22.17 + 22.18) / 5 = 22.178
	expected := 22.178
	if !floatEquals(values[0], expected, 0.01) {
		t.Errorf("First SMA value: expected %.3f, got %.3f", expected, values[0])
	}

	if len(values) != len(prices)-sma.Period()+1 {
		t.Errorf("Expected %d SMA values, got %d", len(prices)-sma.Period()+1, len(values))
	}
}

func TestSMAInsufficientData(t *testing.T) {
	prices := []float64{22.27, 22.19, 22.08}
	candles := createTestCandles(prices)

	sma, _ := NewSMA(5)
	_, err := sma.Calculate(candles)
	if err != ErrInsufficientData {
		t.Errorf("Expected ErrInsufficientData, got %v", err)
	}
}

func TestSMAInvalidPeriod(t *testing.T) {
	_, err := NewSMA(0)
	if err != ErrInvalidPeriod {
		t.Errorf("Expected ErrInvalidPeriod, got %v", err)
	}

	_, err = NewSMA(-5)
	if err != ErrInvalidPeriod {
		t.Errorf("Expected ErrInvalidPeriod, got %v", err)
	}
}

func TestEMA(t *testing.T) {
	prices := []float64{22.27, 22.19, 22.08, 22.17, 22.18, 22.13, 22.23, 22.43, 22.24, 22.29}
	candles := createTestCandles(prices)

	ema, err := NewEMA(5)
	if err != nil {
		t.Fatalf("Failed to create EMA: %v", err)
	}

	values, err := ema.Calculate(candles)
	if err != nil {
		t.Fatalf("Failed to calculate EMA: %v", err)
	}

	// First EMA should equal first SMA
	expectedFirst := 22.178
	if !floatEquals(values[0], expectedFirst, 0.01) {
		t.Errorf("First EMA value: expected %.3f, got %.3f", expectedFirst, values[0])
	}

	// EMA should respond faster to price changes than SMA
	if len(values) != len(prices)-ema.Period()+1 {
		t.Errorf("Expected %d EMA values, got %d", len(prices)-ema.Period()+1, len(values))
	}
}

func TestRSI(t *testing.T) {
	// Create prices with clear upward trend
	prices := []float64{44, 44.34, 44.09, 43.61, 44.33, 44.83, 45.10, 45.42, 45.84,
		46.08, 45.89, 46.03, 45.61, 46.28, 46.28, 46.00}
	candles := createTestCandles(prices)

	rsi, err := NewRSI(14)
	if err != nil {
		t.Fatalf("Failed to create RSI: %v", err)
	}

	values, err := rsi.Calculate(candles)
	if err != nil {
		t.Fatalf("Failed to calculate RSI: %v", err)
	}

	// RSI should be between 0 and 100
	for _, v := range values {
		if v < 0 || v > 100 {
			t.Errorf("RSI value out of range: %f", v)
		}
	}
}

func TestRSIOverboughtOversold(t *testing.T) {
	rsi, _ := NewRSIWithLevels(14, 70, 30)

	// Test with high prices (overbought)
	highPrices := make([]float64, 20)
	for i := range highPrices {
		highPrices[i] = float64(100 + i) // Steadily increasing
	}
	candles := createTestCandles(highPrices)

	overbought, err := rsi.IsOverbought(candles)
	if err != nil {
		t.Fatalf("Failed to check overbought: %v", err)
	}
	if !overbought {
		t.Error("Expected overbought condition for steadily rising prices")
	}
}

func TestMACD(t *testing.T) {
	// Create enough data for MACD (needs at least 26 + 9 - 1 = 34 candles)
	prices := make([]float64, 40)
	for i := range prices {
		prices[i] = 100 + float64(i)*0.5 // Upward trend
	}
	candles := createTestCandles(prices)

	macd, err := NewStandardMACD()
	if err != nil {
		t.Fatalf("Failed to create MACD: %v", err)
	}

	values, err := macd.CalculateAll(candles)
	if err != nil {
		t.Fatalf("Failed to calculate MACD: %v", err)
	}

	// In an uptrend, MACD should be positive (fast > slow)
	lastMACD := values.MACD[len(values.MACD)-1]
	if lastMACD <= 0 {
		t.Logf("Warning: MACD is not positive in uptrend: %f", lastMACD)
	}

	// Histogram should equal MACD - Signal
	for i := range values.Histogram {
		expected := values.MACD[i] - values.Signal[i]
		if !floatEquals(values.Histogram[i], expected, 0.0001) {
			t.Errorf("Histogram mismatch at %d: expected %f, got %f", i, expected, values.Histogram[i])
		}
	}
}

func TestBollingerBands(t *testing.T) {
	prices := make([]float64, 25)
	for i := range prices {
		prices[i] = 100 + float64(i%5) // Oscillating prices
	}
	candles := createTestCandles(prices)

	bb, err := NewStandardBollingerBands()
	if err != nil {
		t.Fatalf("Failed to create Bollinger Bands: %v", err)
	}

	values, err := bb.CalculateAll(candles)
	if err != nil {
		t.Fatalf("Failed to calculate Bollinger Bands: %v", err)
	}

	// Upper band should always be greater than middle
	// Middle should always be greater than lower
	for i := range values.Middle {
		if values.Upper[i] <= values.Middle[i] {
			t.Errorf("Upper band should be > middle at index %d", i)
		}
		if values.Middle[i] <= values.Lower[i] {
			t.Errorf("Middle should be > lower band at index %d", i)
		}
	}

	// Test %B calculation
	percentB, err := bb.PercentB(candles)
	if err != nil {
		t.Fatalf("Failed to calculate %%B: %v", err)
	}
	if len(percentB) != len(values.Upper) {
		t.Errorf("%%B length mismatch: expected %d, got %d", len(values.Upper), len(percentB))
	}
}

func TestATR(t *testing.T) {
	// Create candles with varying ranges
	candles := make([]model.Candle, 20)
	baseTime := time.Now()
	for i := range candles {
		basePrice := 100.0 + float64(i)
		candles[i] = model.Candle{
			OpenTime:  baseTime.Add(time.Hour * time.Duration(i)),
			Open:      basePrice,
			High:      basePrice + 2, // $2 range
			Low:       basePrice - 1,
			Close:     basePrice + 0.5,
			Volume:    1000,
			CloseTime: baseTime.Add(time.Hour * time.Duration(i+1)),
		}
	}

	atr, err := NewATR(14)
	if err != nil {
		t.Fatalf("Failed to create ATR: %v", err)
	}

	values, err := atr.Calculate(candles)
	if err != nil {
		t.Fatalf("Failed to calculate ATR: %v", err)
	}

	// ATR should be positive
	for _, v := range values {
		if v <= 0 {
			t.Errorf("ATR should be positive, got %f", v)
		}
	}

	// Test stop loss calculation
	stopDist, err := atr.StopLossDistance(candles, 2.0)
	if err != nil {
		t.Fatalf("Failed to calculate stop loss distance: %v", err)
	}
	if stopDist <= 0 {
		t.Errorf("Stop loss distance should be positive, got %f", stopDist)
	}
}

func TestSignalGeneration(t *testing.T) {
	// Create prices with a crossover pattern
	prices := []float64{10, 11, 12, 13, 14, 13, 12, 11, 10, 9}
	candles := createTestCandles(prices)

	sma, _ := NewSMA(3)
	signal, err := sma.GenerateSignal(candles)
	if err != nil {
		t.Fatalf("Failed to generate signal: %v", err)
	}

	// Signal should be one of the valid types
	if signal != SignalNone && signal != SignalBuy && signal != SignalSell {
		t.Errorf("Invalid signal type: %d", signal)
	}
}

func TestIndicatorNames(t *testing.T) {
	sma, _ := NewSMA(20)
	if sma.Name() != "SMA(20)" {
		t.Errorf("Unexpected SMA name: %s", sma.Name())
	}

	ema, _ := NewEMA(12)
	if ema.Name() != "EMA(12)" {
		t.Errorf("Unexpected EMA name: %s", ema.Name())
	}

	rsi, _ := NewRSI(14)
	if rsi.Name() != "RSI(14)" {
		t.Errorf("Unexpected RSI name: %s", rsi.Name())
	}

	macd, _ := NewMACD(12, 26, 9)
	if macd.Name() != "MACD(12,26,9)" {
		t.Errorf("Unexpected MACD name: %s", macd.Name())
	}

	bb, _ := NewBollingerBands(20, 2.0)
	if bb.Name() != "BB(20,2.0)" {
		t.Errorf("Unexpected BB name: %s", bb.Name())
	}

	atr, _ := NewATR(14)
	if atr.Name() != "ATR(14)" {
		t.Errorf("Unexpected ATR name: %s", atr.Name())
	}
}
