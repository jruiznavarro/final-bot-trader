package indicator

import (
	"errors"

	"final-bot-trader-api/internal/exchange/model"
)

// Common errors
var (
	ErrInsufficientData = errors.New("insufficient data for calculation")
	ErrInvalidPeriod    = errors.New("invalid period: must be greater than 0")
	ErrInvalidParameter = errors.New("invalid parameter value")
)

// SignalType represents a trading signal
type SignalType int

const (
	SignalNone SignalType = iota
	SignalBuy
	SignalSell
)

func (s SignalType) String() string {
	switch s {
	case SignalBuy:
		return "BUY"
	case SignalSell:
		return "SELL"
	default:
		return "NONE"
	}
}

// Indicator is the interface that all technical indicators must implement
type Indicator interface {
	// Name returns the name of the indicator
	Name() string

	// Calculate calculates the indicator values for the given candles
	// Returns a slice of float64 values, one for each candle where calculation is possible
	Calculate(candles []model.Candle) ([]float64, error)

	// Period returns the lookback period required for this indicator
	Period() int
}

// MultiValueIndicator returns multiple values per candle (like MACD, Bollinger Bands)
type MultiValueIndicator interface {
	Indicator

	// CalculateMulti returns multiple series of values
	CalculateMulti(candles []model.Candle) (map[string][]float64, error)
}

// SignalGenerator can generate trading signals
type SignalGenerator interface {
	// GenerateSignal returns a trading signal based on the indicator values
	GenerateSignal(candles []model.Candle) (SignalType, error)
}

// extractClosePrices extracts close prices from candles
func extractClosePrices(candles []model.Candle) []float64 {
	prices := make([]float64, len(candles))
	for i, c := range candles {
		prices[i] = c.Close
	}
	return prices
}

// extractHighPrices extracts high prices from candles
func extractHighPrices(candles []model.Candle) []float64 {
	prices := make([]float64, len(candles))
	for i, c := range candles {
		prices[i] = c.High
	}
	return prices
}

// extractLowPrices extracts low prices from candles
func extractLowPrices(candles []model.Candle) []float64 {
	prices := make([]float64, len(candles))
	for i, c := range candles {
		prices[i] = c.Low
	}
	return prices
}

// sum calculates the sum of a slice
func sum(values []float64) float64 {
	var total float64
	for _, v := range values {
		total += v
	}
	return total
}

// mean calculates the average of a slice
func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	return sum(values) / float64(len(values))
}

// max returns the maximum value in a slice
func max(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

// min returns the minimum value in a slice
func min(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}

// abs returns the absolute value
func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
