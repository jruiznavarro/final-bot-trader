package indicator

import (
	"math"

	"etf-bot-trader-api/internal/exchange"
)

// EMA calculates Exponential Moving Average
func EMA(candles []exchange.Candle, period int) []float64 {
	if len(candles) < period {
		return nil
	}

	closes := make([]float64, len(candles))
	for i, c := range candles {
		closes[i] = c.Close
	}

	return emaFromValues(closes, period)
}

func emaFromValues(values []float64, period int) []float64 {
	if len(values) < period {
		return nil
	}

	result := make([]float64, len(values))
	multiplier := 2.0 / float64(period+1)

	// First EMA is SMA
	sum := 0.0
	for i := 0; i < period; i++ {
		sum += values[i]
	}
	result[period-1] = sum / float64(period)

	// Calculate subsequent EMAs
	for i := period; i < len(values); i++ {
		result[i] = (values[i]-result[i-1])*multiplier + result[i-1]
	}

	return result
}

// SMA calculates Simple Moving Average
func SMA(candles []exchange.Candle, period int) []float64 {
	if len(candles) < period {
		return nil
	}

	result := make([]float64, len(candles))

	for i := period - 1; i < len(candles); i++ {
		sum := 0.0
		for j := i - period + 1; j <= i; j++ {
			sum += candles[j].Close
		}
		result[i] = sum / float64(period)
	}

	return result
}

// RSI calculates Relative Strength Index
func RSI(candles []exchange.Candle, period int) []float64 {
	if len(candles) < period+1 {
		return nil
	}

	gains := make([]float64, len(candles)-1)
	losses := make([]float64, len(candles)-1)

	for i := 1; i < len(candles); i++ {
		change := candles[i].Close - candles[i-1].Close
		if change > 0 {
			gains[i-1] = change
			losses[i-1] = 0
		} else {
			gains[i-1] = 0
			losses[i-1] = -change
		}
	}

	avgGain := emaFromValues(gains, period)
	avgLoss := emaFromValues(losses, period)

	result := make([]float64, len(candles))
	for i := period; i < len(candles); i++ {
		if avgLoss[i-1] == 0 {
			result[i] = 100
		} else {
			rs := avgGain[i-1] / avgLoss[i-1]
			result[i] = 100 - (100 / (1 + rs))
		}
	}

	return result
}

// ATR calculates Average True Range
func ATR(candles []exchange.Candle, period int) []float64 {
	if len(candles) < period+1 {
		return nil
	}

	trs := make([]float64, len(candles)-1)
	for i := 1; i < len(candles); i++ {
		high := candles[i].High
		low := candles[i].Low
		prevClose := candles[i-1].Close

		tr := math.Max(high-low, math.Max(
			math.Abs(high-prevClose),
			math.Abs(low-prevClose),
		))
		trs[i-1] = tr
	}

	return emaFromValues(trs, period)
}

// MACD calculates Moving Average Convergence Divergence
type MACDResult struct {
	MACD      []float64
	Signal    []float64
	Histogram []float64
}

func MACD(candles []exchange.Candle, fastPeriod, slowPeriod, signalPeriod int) *MACDResult {
	if len(candles) < slowPeriod {
		return nil
	}

	fastEMA := EMA(candles, fastPeriod)
	slowEMA := EMA(candles, slowPeriod)

	macdLine := make([]float64, len(candles))
	for i := slowPeriod - 1; i < len(candles); i++ {
		macdLine[i] = fastEMA[i] - slowEMA[i]
	}

	// Signal line is EMA of MACD
	signalLine := emaFromValues(macdLine[slowPeriod-1:], signalPeriod)

	// Pad signal line to match length
	paddedSignal := make([]float64, len(candles))
	for i, v := range signalLine {
		paddedSignal[slowPeriod-1+i] = v
	}

	// Histogram
	histogram := make([]float64, len(candles))
	for i := slowPeriod + signalPeriod - 2; i < len(candles); i++ {
		histogram[i] = macdLine[i] - paddedSignal[i]
	}

	return &MACDResult{
		MACD:      macdLine,
		Signal:    paddedSignal,
		Histogram: histogram,
	}
}

// ADX calculates Average Directional Index
func ADX(candles []exchange.Candle, period int) []float64 {
	if len(candles) < period*2 {
		return nil
	}

	plusDM := make([]float64, len(candles)-1)
	minusDM := make([]float64, len(candles)-1)
	tr := make([]float64, len(candles)-1)

	for i := 1; i < len(candles); i++ {
		high := candles[i].High
		low := candles[i].Low
		prevHigh := candles[i-1].High
		prevLow := candles[i-1].Low
		prevClose := candles[i-1].Close

		upMove := high - prevHigh
		downMove := prevLow - low

		if upMove > downMove && upMove > 0 {
			plusDM[i-1] = upMove
		}
		if downMove > upMove && downMove > 0 {
			minusDM[i-1] = downMove
		}

		tr[i-1] = math.Max(high-low, math.Max(
			math.Abs(high-prevClose),
			math.Abs(low-prevClose),
		))
	}

	smoothedPlusDM := emaFromValues(plusDM, period)
	smoothedMinusDM := emaFromValues(minusDM, period)
	smoothedTR := emaFromValues(tr, period)

	plusDI := make([]float64, len(candles))
	minusDI := make([]float64, len(candles))
	dx := make([]float64, len(candles))

	for i := period; i < len(candles); i++ {
		if smoothedTR[i-1] > 0 {
			plusDI[i] = (smoothedPlusDM[i-1] / smoothedTR[i-1]) * 100
			minusDI[i] = (smoothedMinusDM[i-1] / smoothedTR[i-1]) * 100
		}

		diSum := plusDI[i] + minusDI[i]
		if diSum > 0 {
			dx[i] = math.Abs(plusDI[i]-minusDI[i]) / diSum * 100
		}
	}

	return emaFromValues(dx[period:], period)
}

// BollingerBands calculates Bollinger Bands
type BollingerResult struct {
	Upper  []float64
	Middle []float64
	Lower  []float64
}

func BollingerBands(candles []exchange.Candle, period int, stdDev float64) *BollingerResult {
	if len(candles) < period {
		return nil
	}

	sma := SMA(candles, period)

	upper := make([]float64, len(candles))
	lower := make([]float64, len(candles))

	for i := period - 1; i < len(candles); i++ {
		// Calculate standard deviation
		sum := 0.0
		for j := i - period + 1; j <= i; j++ {
			diff := candles[j].Close - sma[i]
			sum += diff * diff
		}
		sd := math.Sqrt(sum / float64(period))

		upper[i] = sma[i] + (stdDev * sd)
		lower[i] = sma[i] - (stdDev * sd)
	}

	return &BollingerResult{
		Upper:  upper,
		Middle: sma,
		Lower:  lower,
	}
}

// VWAP calculates Volume Weighted Average Price
func VWAP(candles []exchange.Candle) []float64 {
	result := make([]float64, len(candles))

	cumVolume := 0.0
	cumVolumePrice := 0.0

	for i, c := range candles {
		typicalPrice := (c.High + c.Low + c.Close) / 3
		cumVolume += c.Volume
		cumVolumePrice += typicalPrice * c.Volume

		if cumVolume > 0 {
			result[i] = cumVolumePrice / cumVolume
		}
	}

	return result
}

// Momentum calculates price momentum
func Momentum(candles []exchange.Candle, period int) []float64 {
	if len(candles) < period+1 {
		return nil
	}

	result := make([]float64, len(candles))
	for i := period; i < len(candles); i++ {
		result[i] = ((candles[i].Close - candles[i-period].Close) / candles[i-period].Close) * 100
	}

	return result
}
