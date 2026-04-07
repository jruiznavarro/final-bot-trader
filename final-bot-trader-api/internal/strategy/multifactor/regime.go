package multifactor

import (
	"final-bot-trader-api/internal/exchange/model"
	"math"
)

// MarketRegime represents the current market state
type MarketRegime int

const (
	RegimeUnknown MarketRegime = iota
	RegimeTrendingUp
	RegimeTrendingDown
	RegimeRanging
	RegimeHighVolatility
)

func (r MarketRegime) String() string {
	switch r {
	case RegimeTrendingUp:
		return "TRENDING_UP"
	case RegimeTrendingDown:
		return "TRENDING_DOWN"
	case RegimeRanging:
		return "RANGING"
	case RegimeHighVolatility:
		return "HIGH_VOLATILITY"
	default:
		return "UNKNOWN"
	}
}

// RegimeDetector detects the current market regime
type RegimeDetector struct {
	ADXPeriod        int
	ADXTrendThreshold float64 // ADX > this = trending
	ADXRangeThreshold float64 // ADX < this = ranging
	VolatilityMult   float64 // ATR > avg * this = high vol
	MinDIMargin      float64 // +DI and -DI must differ by at least this to declare a direction
}

// DefaultRegimeDetector returns a detector with sensible defaults
func DefaultRegimeDetector() *RegimeDetector {
	return &RegimeDetector{
		ADXPeriod:        14,
		ADXTrendThreshold: 25,
		ADXRangeThreshold: 20,
		VolatilityMult:   1.5,
		MinDIMargin:      5.0, // require 5-point DI gap to avoid flip-flops during bounces
	}
}

// DetectRegime analyzes candles and returns the current market regime
func (d *RegimeDetector) DetectRegime(candles []model.Candle) (MarketRegime, float64, float64) {
	if len(candles) < d.ADXPeriod*3 {
		return RegimeUnknown, 0, 0
	}

	// Calculate ADX and directional indicators
	adx, plusDI, minusDI := d.calculateADX(candles)

	// Calculate ATR for volatility check
	atr := d.calculateATR(candles, 14)
	avgATR := d.calculateAvgATR(candles, 50)

	// Check for high volatility first
	if avgATR > 0 && atr > avgATR*d.VolatilityMult {
		return RegimeHighVolatility, adx, atr
	}

	// Check for trending vs ranging.
	// Require a minimum DI margin to avoid flip-flopping between TRENDING_UP and
	// TRENDING_DOWN during brief counter-trend bounces (e.g. dead-cat bounces in
	// a crash). Without a margin, a 0.1-point DI crossover on a 4h bounce is
	// enough to declare TRENDING_UP and trigger a losing LONG.
	if adx > d.ADXTrendThreshold {
		if plusDI > minusDI+d.MinDIMargin {
			return RegimeTrendingUp, adx, atr
		}
		if minusDI > plusDI+d.MinDIMargin {
			return RegimeTrendingDown, adx, atr
		}
		// DI values are within margin: strong trend exists but direction is
		// ambiguous (transition/squeeze). Treat as ranging to avoid bad entries.
		return RegimeRanging, adx, atr
	}

	if adx < d.ADXRangeThreshold {
		return RegimeRanging, adx, atr
	}

	// Neutral zone - use price action
	return d.detectByPriceAction(candles), adx, atr
}

func (d *RegimeDetector) calculateADX(candles []model.Candle) (adx, plusDI, minusDI float64) {
	period := d.ADXPeriod
	if len(candles) < period*2 {
		return 20, 50, 50 // Neutral defaults
	}

	var plusDMs, minusDMs, trs []float64

	for i := 1; i < len(candles); i++ {
		high := candles[i].High
		low := candles[i].Low
		prevHigh := candles[i-1].High
		prevLow := candles[i-1].Low
		prevClose := candles[i-1].Close

		// True Range
		tr := math.Max(high-low, math.Max(
			math.Abs(high-prevClose),
			math.Abs(low-prevClose)))
		trs = append(trs, tr)

		// Directional Movement
		upMove := high - prevHigh
		downMove := prevLow - low

		if upMove > downMove && upMove > 0 {
			plusDMs = append(plusDMs, upMove)
		} else {
			plusDMs = append(plusDMs, 0)
		}

		if downMove > upMove && downMove > 0 {
			minusDMs = append(minusDMs, downMove)
		} else {
			minusDMs = append(minusDMs, 0)
		}
	}

	// Smooth with EMA
	smoothPlusDM := ema(plusDMs, period)
	smoothMinusDM := ema(minusDMs, period)
	smoothTR := ema(trs, period)

	// Calculate latest DI values
	lastIdx := len(smoothTR) - 1
	if lastIdx < 0 || smoothTR[lastIdx] == 0 {
		return 20, 50, 50
	}

	plusDI = (smoothPlusDM[lastIdx] / smoothTR[lastIdx]) * 100
	minusDI = (smoothMinusDM[lastIdx] / smoothTR[lastIdx]) * 100

	// Calculate DX series
	var dxSeries []float64
	for i := 0; i < len(smoothTR); i++ {
		if smoothTR[i] == 0 {
			dxSeries = append(dxSeries, 0)
			continue
		}
		pdi := (smoothPlusDM[i] / smoothTR[i]) * 100
		mdi := (smoothMinusDM[i] / smoothTR[i]) * 100
		diSum := pdi + mdi
		if diSum == 0 {
			dxSeries = append(dxSeries, 0)
		} else {
			dxSeries = append(dxSeries, math.Abs(pdi-mdi)/diSum*100)
		}
	}

	// ADX is EMA of DX
	adxSeries := ema(dxSeries, period)
	if len(adxSeries) > 0 {
		adx = adxSeries[len(adxSeries)-1]
	}

	return adx, plusDI, minusDI
}

func (d *RegimeDetector) calculateATR(candles []model.Candle, period int) float64 {
	if len(candles) < period+1 {
		return 0
	}

	var trs []float64
	for i := 1; i < len(candles); i++ {
		tr := math.Max(candles[i].High-candles[i].Low,
			math.Max(
				math.Abs(candles[i].High-candles[i-1].Close),
				math.Abs(candles[i].Low-candles[i-1].Close)))
		trs = append(trs, tr)
	}

	atrSeries := ema(trs, period)
	if len(atrSeries) > 0 {
		return atrSeries[len(atrSeries)-1]
	}
	return 0
}

func (d *RegimeDetector) calculateAvgATR(candles []model.Candle, period int) float64 {
	if len(candles) < period+1 {
		return 0
	}

	var sum float64
	count := 0
	for i := len(candles) - period; i < len(candles) && i > 0; i++ {
		tr := math.Max(candles[i].High-candles[i].Low,
			math.Max(
				math.Abs(candles[i].High-candles[i-1].Close),
				math.Abs(candles[i].Low-candles[i-1].Close)))
		sum += tr
		count++
	}

	if count > 0 {
		return sum / float64(count)
	}
	return 0
}

func (d *RegimeDetector) detectByPriceAction(candles []model.Candle) MarketRegime {
	// Look at recent swing highs/lows
	recent := candles[len(candles)-20:]

	var highs, lows []float64
	for _, c := range recent {
		highs = append(highs, c.High)
		lows = append(lows, c.Low)
	}

	// Check for higher highs and higher lows (uptrend)
	// or lower highs and lower lows (downtrend)
	firstHalf := recent[:10]
	secondHalf := recent[10:]

	firstHighAvg := avg(getHighs(firstHalf))
	secondHighAvg := avg(getHighs(secondHalf))
	firstLowAvg := avg(getLows(firstHalf))
	secondLowAvg := avg(getLows(secondHalf))

	if secondHighAvg > firstHighAvg && secondLowAvg > firstLowAvg {
		return RegimeTrendingUp
	}
	if secondHighAvg < firstHighAvg && secondLowAvg < firstLowAvg {
		return RegimeTrendingDown
	}

	return RegimeRanging
}

func getHighs(candles []model.Candle) []float64 {
	var highs []float64
	for _, c := range candles {
		highs = append(highs, c.High)
	}
	return highs
}

func getLows(candles []model.Candle) []float64 {
	var lows []float64
	for _, c := range candles {
		lows = append(lows, c.Low)
	}
	return lows
}

func avg(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func ema(values []float64, period int) []float64 {
	if len(values) == 0 {
		return []float64{}
	}

	result := make([]float64, len(values))
	multiplier := 2.0 / float64(period+1)

	// First value is SMA
	sum := 0.0
	for i := 0; i < period && i < len(values); i++ {
		sum += values[i]
	}
	result[0] = sum / float64(min(period, len(values)))

	for i := 1; i < len(values); i++ {
		result[i] = (values[i]-result[i-1])*multiplier + result[i-1]
	}

	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
