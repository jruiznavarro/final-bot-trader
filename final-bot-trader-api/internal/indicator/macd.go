package indicator

import (
	"fmt"

	"final-bot-trader-api/internal/exchange/model"
)

// MACD implements the Moving Average Convergence Divergence indicator
type MACD struct {
	fastPeriod   int
	slowPeriod   int
	signalPeriod int
}

// MACDValues holds the calculated MACD values
type MACDValues struct {
	MACD      []float64 // MACD line (fast EMA - slow EMA)
	Signal    []float64 // Signal line (EMA of MACD)
	Histogram []float64 // Histogram (MACD - Signal)
}

// NewMACD creates a new MACD indicator with custom periods
// Standard MACD uses 12, 26, 9
func NewMACD(fastPeriod, slowPeriod, signalPeriod int) (*MACD, error) {
	if fastPeriod <= 0 || slowPeriod <= 0 || signalPeriod <= 0 {
		return nil, ErrInvalidPeriod
	}
	if fastPeriod >= slowPeriod {
		return nil, ErrInvalidParameter
	}
	return &MACD{
		fastPeriod:   fastPeriod,
		slowPeriod:   slowPeriod,
		signalPeriod: signalPeriod,
	}, nil
}

// NewStandardMACD creates a MACD with standard periods (12, 26, 9)
func NewStandardMACD() (*MACD, error) {
	return NewMACD(12, 26, 9)
}

// Name returns the indicator name
func (m *MACD) Name() string {
	return fmt.Sprintf("MACD(%d,%d,%d)", m.fastPeriod, m.slowPeriod, m.signalPeriod)
}

// Period returns the minimum lookback period required
func (m *MACD) Period() int {
	return m.slowPeriod + m.signalPeriod - 1
}

// Calculate returns the MACD line values
func (m *MACD) Calculate(candles []model.Candle) ([]float64, error) {
	values, err := m.CalculateAll(candles)
	if err != nil {
		return nil, err
	}
	return values.MACD, nil
}

// CalculateAll calculates all MACD components
func (m *MACD) CalculateAll(candles []model.Candle) (*MACDValues, error) {
	if len(candles) < m.Period() {
		return nil, ErrInsufficientData
	}

	prices := extractClosePrices(candles)

	// Calculate fast EMA
	fastEMA, err := CalculateEMA(prices, m.fastPeriod)
	if err != nil {
		return nil, err
	}

	// Calculate slow EMA
	slowEMA, err := CalculateEMA(prices, m.slowPeriod)
	if err != nil {
		return nil, err
	}

	// Align arrays - slow EMA is shorter
	offset := m.slowPeriod - m.fastPeriod
	alignedFastEMA := fastEMA[offset:]

	// Calculate MACD line (fast - slow)
	macdLine := make([]float64, len(slowEMA))
	for i := range slowEMA {
		macdLine[i] = alignedFastEMA[i] - slowEMA[i]
	}

	// Calculate signal line (EMA of MACD)
	signalLine, err := CalculateEMA(macdLine, m.signalPeriod)
	if err != nil {
		return nil, err
	}

	// Align MACD line with signal line
	signalOffset := m.signalPeriod - 1
	alignedMACD := macdLine[signalOffset:]

	// Calculate histogram (MACD - Signal)
	histogram := make([]float64, len(signalLine))
	for i := range signalLine {
		histogram[i] = alignedMACD[i] - signalLine[i]
	}

	return &MACDValues{
		MACD:      alignedMACD,
		Signal:    signalLine,
		Histogram: histogram,
	}, nil
}

// CalculateMulti implements MultiValueIndicator interface
func (m *MACD) CalculateMulti(candles []model.Candle) (map[string][]float64, error) {
	values, err := m.CalculateAll(candles)
	if err != nil {
		return nil, err
	}

	return map[string][]float64{
		"macd":      values.MACD,
		"signal":    values.Signal,
		"histogram": values.Histogram,
	}, nil
}

// GenerateSignal generates signals based on MACD crossovers
func (m *MACD) GenerateSignal(candles []model.Candle) (SignalType, error) {
	values, err := m.CalculateAll(candles)
	if err != nil {
		return SignalNone, err
	}

	if len(values.MACD) < 2 || len(values.Signal) < 2 {
		return SignalNone, ErrInsufficientData
	}

	currentMACD := values.MACD[len(values.MACD)-1]
	previousMACD := values.MACD[len(values.MACD)-2]
	currentSignal := values.Signal[len(values.Signal)-1]
	previousSignal := values.Signal[len(values.Signal)-2]

	// Bullish crossover: MACD crosses above signal line
	if previousMACD <= previousSignal && currentMACD > currentSignal {
		return SignalBuy, nil
	}

	// Bearish crossover: MACD crosses below signal line
	if previousMACD >= previousSignal && currentMACD < currentSignal {
		return SignalSell, nil
	}

	return SignalNone, nil
}

// CurrentValues returns the current MACD, Signal, and Histogram values
func (m *MACD) CurrentValues(candles []model.Candle) (macd, signal, histogram float64, err error) {
	values, err := m.CalculateAll(candles)
	if err != nil {
		return 0, 0, 0, err
	}

	if len(values.MACD) == 0 {
		return 0, 0, 0, ErrInsufficientData
	}

	return values.MACD[len(values.MACD)-1],
		values.Signal[len(values.Signal)-1],
		values.Histogram[len(values.Histogram)-1], nil
}
