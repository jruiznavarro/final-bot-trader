package indicator

import (
	"fmt"

	"final-bot-trader-api/internal/exchange/model"
)

// ATR implements the Average True Range indicator
type ATR struct {
	period int
}

// NewATR creates a new ATR indicator with the given period
// Standard period is 14
func NewATR(period int) (*ATR, error) {
	if period <= 0 {
		return nil, ErrInvalidPeriod
	}
	return &ATR{period: period}, nil
}

// Name returns the indicator name
func (a *ATR) Name() string {
	return fmt.Sprintf("ATR(%d)", a.period)
}

// Period returns the lookback period
func (a *ATR) Period() int {
	return a.period
}

// Calculate calculates the ATR for the given candles
func (a *ATR) Calculate(candles []model.Candle) ([]float64, error) {
	if len(candles) < a.period+1 {
		return nil, ErrInsufficientData
	}

	// Calculate True Range for each candle
	trueRanges := calculateTrueRange(candles)

	// Calculate ATR using Wilder's smoothing (similar to RSI)
	return calculateSmoothedAverage(trueRanges, a.period)
}

// calculateTrueRange calculates the True Range for each candle
// TR = max(High - Low, |High - Previous Close|, |Low - Previous Close|)
func calculateTrueRange(candles []model.Candle) []float64 {
	if len(candles) < 2 {
		return nil
	}

	trueRanges := make([]float64, len(candles)-1)

	for i := 1; i < len(candles); i++ {
		high := candles[i].High
		low := candles[i].Low
		prevClose := candles[i-1].Close

		// Calculate the three components of True Range
		highLow := high - low
		highPrevClose := abs(high - prevClose)
		lowPrevClose := abs(low - prevClose)

		// True Range is the maximum of the three
		tr := max(highLow, max(highPrevClose, lowPrevClose))
		trueRanges[i-1] = tr
	}

	return trueRanges
}

// calculateSmoothedAverage calculates Wilder's smoothed average
func calculateSmoothedAverage(values []float64, period int) ([]float64, error) {
	if len(values) < period {
		return nil, ErrInsufficientData
	}

	result := make([]float64, len(values)-period+1)

	// First value is SMA
	var sum float64
	for i := 0; i < period; i++ {
		sum += values[i]
	}
	result[0] = sum / float64(period)

	// Subsequent values use Wilder's smoothing
	// ATR = (Previous ATR * (period - 1) + Current TR) / period
	for i := period; i < len(values); i++ {
		result[i-period+1] = (result[i-period]*float64(period-1) + values[i]) / float64(period)
	}

	return result, nil
}

// Value returns the current ATR value (last value)
func (a *ATR) Value(candles []model.Candle) (float64, error) {
	values, err := a.Calculate(candles)
	if err != nil {
		return 0, err
	}
	if len(values) == 0 {
		return 0, ErrInsufficientData
	}
	return values[len(values)-1], nil
}

// StopLossDistance returns the suggested stop loss distance based on ATR
// multiplier is typically 1.5 to 3.0
func (a *ATR) StopLossDistance(candles []model.Candle, multiplier float64) (float64, error) {
	atrValue, err := a.Value(candles)
	if err != nil {
		return 0, err
	}
	return atrValue * multiplier, nil
}

// TakeProfitDistance returns the suggested take profit distance based on ATR
// multiplier is typically 2.0 to 4.0
func (a *ATR) TakeProfitDistance(candles []model.Candle, multiplier float64) (float64, error) {
	atrValue, err := a.Value(candles)
	if err != nil {
		return 0, err
	}
	return atrValue * multiplier, nil
}

// CalculateTrailingStop calculates a trailing stop price
// For long positions: currentPrice - (ATR * multiplier)
// For short positions: currentPrice + (ATR * multiplier)
func (a *ATR) CalculateTrailingStop(candles []model.Candle, currentPrice float64, isLong bool, multiplier float64) (float64, error) {
	atrValue, err := a.Value(candles)
	if err != nil {
		return 0, err
	}

	distance := atrValue * multiplier
	if isLong {
		return currentPrice - distance, nil
	}
	return currentPrice + distance, nil
}
