package indicator

import (
	"fmt"
	"math"

	"final-bot-trader-api/internal/exchange/model"
)

// BollingerBands implements the Bollinger Bands indicator
type BollingerBands struct {
	period     int
	deviations float64
}

// BollingerValues holds the calculated Bollinger Bands values
type BollingerValues struct {
	Upper  []float64 // Upper band
	Middle []float64 // Middle band (SMA)
	Lower  []float64 // Lower band
	Width  []float64 // Band width (Upper - Lower) / Middle
}

// NewBollingerBands creates a new Bollinger Bands indicator
// Standard settings are period=20, deviations=2.0
func NewBollingerBands(period int, deviations float64) (*BollingerBands, error) {
	if period <= 0 {
		return nil, ErrInvalidPeriod
	}
	if deviations <= 0 {
		return nil, ErrInvalidParameter
	}
	return &BollingerBands{
		period:     period,
		deviations: deviations,
	}, nil
}

// NewStandardBollingerBands creates Bollinger Bands with standard settings (20, 2.0)
func NewStandardBollingerBands() (*BollingerBands, error) {
	return NewBollingerBands(20, 2.0)
}

// Name returns the indicator name
func (b *BollingerBands) Name() string {
	return fmt.Sprintf("BB(%d,%.1f)", b.period, b.deviations)
}

// Period returns the lookback period
func (b *BollingerBands) Period() int {
	return b.period
}

// Calculate returns the middle band (SMA)
func (b *BollingerBands) Calculate(candles []model.Candle) ([]float64, error) {
	values, err := b.CalculateAll(candles)
	if err != nil {
		return nil, err
	}
	return values.Middle, nil
}

// CalculateAll calculates all Bollinger Bands components
func (b *BollingerBands) CalculateAll(candles []model.Candle) (*BollingerValues, error) {
	if len(candles) < b.period {
		return nil, ErrInsufficientData
	}

	prices := extractClosePrices(candles)

	// Calculate SMA (middle band)
	middle, err := CalculateSMA(prices, b.period)
	if err != nil {
		return nil, err
	}

	resultLen := len(middle)
	upper := make([]float64, resultLen)
	lower := make([]float64, resultLen)
	width := make([]float64, resultLen)

	// Calculate standard deviation and bands
	for i := 0; i < resultLen; i++ {
		// Get the window of prices for this SMA
		windowStart := i
		windowEnd := i + b.period
		window := prices[windowStart:windowEnd]

		// Calculate standard deviation
		stdDev := calculateStdDev(window, middle[i])

		// Calculate bands
		deviation := stdDev * b.deviations
		upper[i] = middle[i] + deviation
		lower[i] = middle[i] - deviation

		// Calculate band width as percentage
		if middle[i] != 0 {
			width[i] = (upper[i] - lower[i]) / middle[i] * 100
		}
	}

	return &BollingerValues{
		Upper:  upper,
		Middle: middle,
		Lower:  lower,
		Width:  width,
	}, nil
}

// CalculateMulti implements MultiValueIndicator interface
func (b *BollingerBands) CalculateMulti(candles []model.Candle) (map[string][]float64, error) {
	values, err := b.CalculateAll(candles)
	if err != nil {
		return nil, err
	}

	return map[string][]float64{
		"upper":  values.Upper,
		"middle": values.Middle,
		"lower":  values.Lower,
		"width":  values.Width,
	}, nil
}

// GenerateSignal generates signals based on price touching bands
func (b *BollingerBands) GenerateSignal(candles []model.Candle) (SignalType, error) {
	if len(candles) < b.period+1 {
		return SignalNone, ErrInsufficientData
	}

	values, err := b.CalculateAll(candles)
	if err != nil {
		return SignalNone, err
	}

	if len(values.Upper) < 2 {
		return SignalNone, ErrInsufficientData
	}

	prices := extractClosePrices(candles)
	currentPrice := prices[len(prices)-1]
	previousPrice := prices[len(prices)-2]

	currentLower := values.Lower[len(values.Lower)-1]
	previousLower := values.Lower[len(values.Lower)-2]
	currentUpper := values.Upper[len(values.Upper)-1]
	previousUpper := values.Upper[len(values.Upper)-2]

	// Buy signal: price bounces off lower band
	if previousPrice <= previousLower && currentPrice > currentLower {
		return SignalBuy, nil
	}

	// Sell signal: price bounces off upper band
	if previousPrice >= previousUpper && currentPrice < currentUpper {
		return SignalSell, nil
	}

	return SignalNone, nil
}

// PercentB calculates the %B indicator (position within bands)
// %B = (Price - Lower) / (Upper - Lower)
// Values: <0 = below lower, 0-1 = within bands, >1 = above upper
func (b *BollingerBands) PercentB(candles []model.Candle) ([]float64, error) {
	values, err := b.CalculateAll(candles)
	if err != nil {
		return nil, err
	}

	prices := extractClosePrices(candles)
	// Align prices with band values
	offset := len(prices) - len(values.Upper)
	alignedPrices := prices[offset:]

	percentB := make([]float64, len(values.Upper))
	for i := range values.Upper {
		bandWidth := values.Upper[i] - values.Lower[i]
		if bandWidth != 0 {
			percentB[i] = (alignedPrices[i] - values.Lower[i]) / bandWidth
		}
	}

	return percentB, nil
}

// calculateStdDev calculates the standard deviation of values around a mean
func calculateStdDev(values []float64, mean float64) float64 {
	if len(values) == 0 {
		return 0
	}

	var sumSquares float64
	for _, v := range values {
		diff := v - mean
		sumSquares += diff * diff
	}

	variance := sumSquares / float64(len(values))
	return math.Sqrt(variance)
}
