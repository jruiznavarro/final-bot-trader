package indicator

import (
	"fmt"

	"final-bot-trader-api/internal/exchange/model"
)

// SMA implements the Simple Moving Average indicator
type SMA struct {
	period int
}

// NewSMA creates a new SMA indicator with the given period
func NewSMA(period int) (*SMA, error) {
	if period <= 0 {
		return nil, ErrInvalidPeriod
	}
	return &SMA{period: period}, nil
}

// Name returns the indicator name
func (s *SMA) Name() string {
	return fmt.Sprintf("SMA(%d)", s.period)
}

// Period returns the lookback period
func (s *SMA) Period() int {
	return s.period
}

// Calculate calculates the SMA for the given candles
func (s *SMA) Calculate(candles []model.Candle) ([]float64, error) {
	if len(candles) < s.period {
		return nil, ErrInsufficientData
	}

	prices := extractClosePrices(candles)
	return CalculateSMA(prices, s.period)
}

// CalculateSMA calculates SMA from price data
func CalculateSMA(prices []float64, period int) ([]float64, error) {
	if len(prices) < period {
		return nil, ErrInsufficientData
	}
	if period <= 0 {
		return nil, ErrInvalidPeriod
	}

	result := make([]float64, len(prices)-period+1)

	// Calculate first SMA
	var windowSum float64
	for i := 0; i < period; i++ {
		windowSum += prices[i]
	}
	result[0] = windowSum / float64(period)

	// Calculate remaining SMAs using sliding window
	for i := period; i < len(prices); i++ {
		windowSum = windowSum - prices[i-period] + prices[i]
		result[i-period+1] = windowSum / float64(period)
	}

	return result, nil
}

// GenerateSignal generates a signal based on price crossing SMA
func (s *SMA) GenerateSignal(candles []model.Candle) (SignalType, error) {
	if len(candles) < s.period+1 {
		return SignalNone, ErrInsufficientData
	}

	smaValues, err := s.Calculate(candles)
	if err != nil {
		return SignalNone, err
	}

	if len(smaValues) < 2 {
		return SignalNone, ErrInsufficientData
	}

	prices := extractClosePrices(candles)
	currentPrice := prices[len(prices)-1]
	previousPrice := prices[len(prices)-2]
	currentSMA := smaValues[len(smaValues)-1]
	previousSMA := smaValues[len(smaValues)-2]

	// Bullish crossover: price crosses above SMA
	if previousPrice <= previousSMA && currentPrice > currentSMA {
		return SignalBuy, nil
	}

	// Bearish crossover: price crosses below SMA
	if previousPrice >= previousSMA && currentPrice < currentSMA {
		return SignalSell, nil
	}

	return SignalNone, nil
}

// Value returns the current SMA value (last value)
func (s *SMA) Value(candles []model.Candle) (float64, error) {
	values, err := s.Calculate(candles)
	if err != nil {
		return 0, err
	}
	if len(values) == 0 {
		return 0, ErrInsufficientData
	}
	return values[len(values)-1], nil
}
