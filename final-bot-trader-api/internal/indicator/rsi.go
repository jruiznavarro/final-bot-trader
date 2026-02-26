package indicator

import (
	"fmt"

	"final-bot-trader-api/internal/exchange/model"
)

// RSI implements the Relative Strength Index indicator
type RSI struct {
	period       int
	overbought   float64
	oversold     float64
}

// NewRSI creates a new RSI indicator with the given period
// Default overbought/oversold levels are 70/30
func NewRSI(period int) (*RSI, error) {
	return NewRSIWithLevels(period, 70, 30)
}

// NewRSIWithLevels creates a new RSI with custom overbought/oversold levels
func NewRSIWithLevels(period int, overbought, oversold float64) (*RSI, error) {
	if period <= 0 {
		return nil, ErrInvalidPeriod
	}
	if overbought <= oversold {
		return nil, ErrInvalidParameter
	}
	return &RSI{
		period:     period,
		overbought: overbought,
		oversold:   oversold,
	}, nil
}

// Name returns the indicator name
func (r *RSI) Name() string {
	return fmt.Sprintf("RSI(%d)", r.period)
}

// Period returns the lookback period
func (r *RSI) Period() int {
	return r.period
}

// Calculate calculates the RSI for the given candles
func (r *RSI) Calculate(candles []model.Candle) ([]float64, error) {
	if len(candles) < r.period+1 {
		return nil, ErrInsufficientData
	}

	prices := extractClosePrices(candles)
	return CalculateRSI(prices, r.period)
}

// CalculateRSI calculates RSI from price data
func CalculateRSI(prices []float64, period int) ([]float64, error) {
	if len(prices) < period+1 {
		return nil, ErrInsufficientData
	}
	if period <= 0 {
		return nil, ErrInvalidPeriod
	}

	// Calculate price changes
	changes := make([]float64, len(prices)-1)
	for i := 1; i < len(prices); i++ {
		changes[i-1] = prices[i] - prices[i-1]
	}

	// Separate gains and losses
	gains := make([]float64, len(changes))
	losses := make([]float64, len(changes))
	for i, change := range changes {
		if change > 0 {
			gains[i] = change
		} else {
			losses[i] = -change
		}
	}

	// Calculate initial average gain and loss (SMA)
	var avgGain, avgLoss float64
	for i := 0; i < period; i++ {
		avgGain += gains[i]
		avgLoss += losses[i]
	}
	avgGain /= float64(period)
	avgLoss /= float64(period)

	result := make([]float64, len(prices)-period)

	// Calculate first RSI
	if avgLoss == 0 {
		result[0] = 100
	} else {
		rs := avgGain / avgLoss
		result[0] = 100 - (100 / (1 + rs))
	}

	// Calculate remaining RSI using Wilder's smoothing
	for i := period; i < len(changes); i++ {
		// Smoothed averages: (prevAvg * (period-1) + currentValue) / period
		avgGain = (avgGain*float64(period-1) + gains[i]) / float64(period)
		avgLoss = (avgLoss*float64(period-1) + losses[i]) / float64(period)

		if avgLoss == 0 {
			result[i-period+1] = 100
		} else {
			rs := avgGain / avgLoss
			result[i-period+1] = 100 - (100 / (1 + rs))
		}
	}

	return result, nil
}

// GenerateSignal generates signals based on RSI levels
func (r *RSI) GenerateSignal(candles []model.Candle) (SignalType, error) {
	rsiValues, err := r.Calculate(candles)
	if err != nil {
		return SignalNone, err
	}

	if len(rsiValues) < 2 {
		return SignalNone, ErrInsufficientData
	}

	currentRSI := rsiValues[len(rsiValues)-1]
	previousRSI := rsiValues[len(rsiValues)-2]

	// Buy signal: RSI crosses above oversold level (leaving oversold zone)
	if previousRSI <= r.oversold && currentRSI > r.oversold {
		return SignalBuy, nil
	}

	// Sell signal: RSI crosses below overbought level (leaving overbought zone)
	if previousRSI >= r.overbought && currentRSI < r.overbought {
		return SignalSell, nil
	}

	return SignalNone, nil
}

// IsOverbought returns true if current RSI is above the overbought level
func (r *RSI) IsOverbought(candles []model.Candle) (bool, error) {
	value, err := r.Value(candles)
	if err != nil {
		return false, err
	}
	return value >= r.overbought, nil
}

// IsOversold returns true if current RSI is below the oversold level
func (r *RSI) IsOversold(candles []model.Candle) (bool, error) {
	value, err := r.Value(candles)
	if err != nil {
		return false, err
	}
	return value <= r.oversold, nil
}

// Value returns the current RSI value (last value)
func (r *RSI) Value(candles []model.Candle) (float64, error) {
	values, err := r.Calculate(candles)
	if err != nil {
		return 0, err
	}
	if len(values) == 0 {
		return 0, ErrInsufficientData
	}
	return values[len(values)-1], nil
}
