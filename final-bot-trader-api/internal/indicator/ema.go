package indicator

import (
	"fmt"

	"final-bot-trader-api/internal/exchange/model"
)

// EMA implements the Exponential Moving Average indicator
type EMA struct {
	period     int
	multiplier float64
}

// NewEMA creates a new EMA indicator with the given period
func NewEMA(period int) (*EMA, error) {
	if period <= 0 {
		return nil, ErrInvalidPeriod
	}
	// EMA multiplier: 2 / (period + 1)
	multiplier := 2.0 / float64(period+1)
	return &EMA{period: period, multiplier: multiplier}, nil
}

// Name returns the indicator name
func (e *EMA) Name() string {
	return fmt.Sprintf("EMA(%d)", e.period)
}

// Period returns the lookback period
func (e *EMA) Period() int {
	return e.period
}

// Calculate calculates the EMA for the given candles
func (e *EMA) Calculate(candles []model.Candle) ([]float64, error) {
	if len(candles) < e.period {
		return nil, ErrInsufficientData
	}

	prices := extractClosePrices(candles)
	return CalculateEMA(prices, e.period)
}

// CalculateEMA calculates EMA from price data
func CalculateEMA(prices []float64, period int) ([]float64, error) {
	if len(prices) < period {
		return nil, ErrInsufficientData
	}
	if period <= 0 {
		return nil, ErrInvalidPeriod
	}

	multiplier := 2.0 / float64(period+1)
	result := make([]float64, len(prices)-period+1)

	// First EMA is the SMA of the first 'period' prices
	var initialSum float64
	for i := 0; i < period; i++ {
		initialSum += prices[i]
	}
	result[0] = initialSum / float64(period)

	// Calculate remaining EMAs
	for i := period; i < len(prices); i++ {
		// EMA = (Close - Previous EMA) * multiplier + Previous EMA
		previousEMA := result[i-period]
		result[i-period+1] = (prices[i]-previousEMA)*multiplier + previousEMA
	}

	return result, nil
}

// CalculateEMAWithSeed calculates EMA starting from a seed value
func CalculateEMAWithSeed(prices []float64, period int, seed float64) ([]float64, error) {
	if len(prices) == 0 {
		return nil, ErrInsufficientData
	}
	if period <= 0 {
		return nil, ErrInvalidPeriod
	}

	multiplier := 2.0 / float64(period+1)
	result := make([]float64, len(prices))

	// Use seed as initial EMA
	result[0] = (prices[0]-seed)*multiplier + seed

	// Calculate remaining EMAs
	for i := 1; i < len(prices); i++ {
		result[i] = (prices[i]-result[i-1])*multiplier + result[i-1]
	}

	return result, nil
}

// GenerateSignal generates a signal based on price crossing EMA
func (e *EMA) GenerateSignal(candles []model.Candle) (SignalType, error) {
	if len(candles) < e.period+1 {
		return SignalNone, ErrInsufficientData
	}

	emaValues, err := e.Calculate(candles)
	if err != nil {
		return SignalNone, err
	}

	if len(emaValues) < 2 {
		return SignalNone, ErrInsufficientData
	}

	prices := extractClosePrices(candles)
	currentPrice := prices[len(prices)-1]
	previousPrice := prices[len(prices)-2]
	currentEMA := emaValues[len(emaValues)-1]
	previousEMA := emaValues[len(emaValues)-2]

	// Bullish crossover: price crosses above EMA
	if previousPrice <= previousEMA && currentPrice > currentEMA {
		return SignalBuy, nil
	}

	// Bearish crossover: price crosses below EMA
	if previousPrice >= previousEMA && currentPrice < currentEMA {
		return SignalSell, nil
	}

	return SignalNone, nil
}

// Value returns the current EMA value (last value)
func (e *EMA) Value(candles []model.Candle) (float64, error) {
	values, err := e.Calculate(candles)
	if err != nil {
		return 0, err
	}
	if len(values) == 0 {
		return 0, ErrInsufficientData
	}
	return values[len(values)-1], nil
}
