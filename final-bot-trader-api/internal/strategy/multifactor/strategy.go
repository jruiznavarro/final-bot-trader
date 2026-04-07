package multifactor

import (
	"fmt"
	"math"

	"final-bot-trader-api/internal/exchange/model"
	"final-bot-trader-api/internal/strategy"
)

// Config holds the multi-factor strategy configuration
type Config struct {
	// EMAs for trend
	FastEMA int
	SlowEMA int

	// RSI
	RSIPeriod       int
	RSIOverbought   float64
	RSIOversold     float64

	// Volume
	VolumePeriod    int
	VolumeThreshold float64 // Multiplier vs average

	// ATR for stops
	ATRPeriod       int
	ATRStopMult     float64 // SL distance = ATR * this
	ATRTargetMult   float64 // TP distance = ATR * this

	// Regime
	RequireTrend bool    // Only trade in trending regimes
	MinADX       float64 // Minimum ADX to trade

	// Ranging mode (mean-reversion) — for stable pairs like BTC/ETH
	// When enabled, trades Bollinger Band bounces in ranging markets instead of skipping them.
	EnableRangingMode    bool    // Activate mean-reversion in ranging regime
	BollingerPeriod      int     // BB SMA period (20)
	BollingerStdDev      float64 // BB std-dev multiplier (2.0)
	RangingRSIOversold   float64 // RSI threshold to enter long in range (32)
	RangingRSIOverbought float64 // RSI threshold to enter short in range (68)
	RangingATRStopMult   float64 // SL = ATR * this beyond the touched band (1.0)
}

// DefaultConfig returns optimized defaults based on walk-forward validation.
// Used for volatile altcoins (ENAUSDT, SUIUSDT, etc.) — momentum/breakout only.
func DefaultConfig() Config {
	return Config{
		FastEMA:         7,   // Optimized from 9
		SlowEMA:         17,  // Optimized from 21
		RSIPeriod:       14,
		RSIOverbought:   65,  // More conservative: don't enter overbought
		RSIOversold:     35,  // More conservative: don't enter oversold
		VolumePeriod:    20,
		VolumeThreshold: 1.2, // Increased: require above-average volume
		ATRPeriod:       14,
		ATRStopMult:     2.2, // Wider stops to reduce premature stop-outs (was 1.8)
		ATRTargetMult:   3.3, // Maintain 1.5 R:R ratio (was 2.7)
		RequireTrend:    true,
		MinADX:          28,  // Only trade in stronger trends (was 25)
		// Ranging mode disabled for alts — they have explosive moves, not ranges
		EnableRangingMode: false,
	}
}

// HybridConfig returns a config tuned for stable pairs (BTC, ETH).
// Combines momentum entries in trending markets with Bollinger Band
// mean-reversion entries in ranging markets.
func HybridConfig() Config {
	return Config{
		// Trend mode — same as DefaultConfig
		FastEMA:         7,
		SlowEMA:         17,
		RSIPeriod:       14,
		RSIOverbought:   65,
		RSIOversold:     35,
		VolumePeriod:    20,
		VolumeThreshold: 1.2,
		ATRPeriod:       14,
		ATRStopMult:     2.2,
		ATRTargetMult:   3.3,
		RequireTrend:    false, // we handle regime routing ourselves
		MinADX:          28,
		// Ranging mode — mean-reversion via Bollinger Bands
		EnableRangingMode:    true,
		BollingerPeriod:      20,
		BollingerStdDev:      2.0,
		RangingRSIOversold:   32, // tighter than trending oversold (35)
		RangingRSIOverbought: 68, // tighter than trending overbought (65)
		RangingATRStopMult:   1.0, // SL just beyond the band
	}
}

// MultiFactorStrategy implements a comprehensive trading strategy
type MultiFactorStrategy struct {
	config   Config
	regime   *RegimeDetector
	symbol   string
}

// NewMultiFactorStrategy creates a new multi-factor strategy
func NewMultiFactorStrategy(symbol string, config Config) *MultiFactorStrategy {
	return &MultiFactorStrategy{
		config: config,
		regime: DefaultRegimeDetector(),
		symbol: symbol,
	}
}

// Name returns the strategy name
func (s *MultiFactorStrategy) Name() string {
	return fmt.Sprintf("MultiFactor(%d/%d)", s.config.FastEMA, s.config.SlowEMA)
}

// MinimumCandles returns minimum candles needed
func (s *MultiFactorStrategy) MinimumCandles() int {
	return max(s.config.SlowEMA, s.config.RSIPeriod, s.config.VolumePeriod, 50) + 10
}

// Analyze analyzes candles and generates a signal.
// Routes to mean-reversion logic in ranging markets (when EnableRangingMode=true)
// and to momentum logic in trending/high-volatility markets.
func (s *MultiFactorStrategy) Analyze(candles []model.Candle) (*strategy.Signal, error) {
	if len(candles) < s.MinimumCandles() {
		return nil, strategy.ErrInsufficientData
	}

	// Step 1: Detect market regime
	regime, adx, atr := s.regime.DetectRegime(candles)

	// Step 2: Route by regime
	if s.config.EnableRangingMode {
		// In hybrid mode, try BB mean-reversion first regardless of ADX regime.
		// We use BB width (not ADX) as the ranging indicator: if bands are not
		// rapidly expanding (no breakout), price at the band is a reversion candidate.
		signal, err := s.analyzeRanging(candles, atr)
		if err == nil {
			return signal, nil
		}
		// No ranging signal — if the regime detector says pure ranging, don't
		// enter the trending path either (no clean signal in either direction).
		if regime == RegimeRanging {
			return nil, strategy.ErrNoSignal
		}
	} else if regime == RegimeRanging {
		return nil, strategy.ErrNoSignal
	}

	// Trending / high-volatility path — apply ADX filter
	if adx < s.config.MinADX {
		return nil, strategy.ErrNoSignal
	}

	// Step 3: Calculate indicators
	fastEMA := s.calculateEMA(candles, s.config.FastEMA)
	slowEMA := s.calculateEMA(candles, s.config.SlowEMA)
	rsi := s.calculateRSI(candles, s.config.RSIPeriod)
	volumeRatio := s.calculateVolumeRatio(candles, s.config.VolumePeriod)

	currentPrice := candles[len(candles)-1].Close
	lastFastEMA := fastEMA[len(fastEMA)-1]
	lastSlowEMA := slowEMA[len(slowEMA)-1]
	lastRSI := rsi[len(rsi)-1]

	// Step 4: Generate momentum signal
	signal := s.generateSignal(
		regime,
		currentPrice,
		lastFastEMA,
		lastSlowEMA,
		lastRSI,
		volumeRatio,
		atr,
		candles,
	)

	if signal == nil {
		return nil, strategy.ErrNoSignal
	}

	return signal, nil
}

// analyzeRanging generates mean-reversion signals using Bollinger Bands + RSI.
// Uses BB width (not ADX regime) to detect consolidation: if bands are not
// rapidly expanding (no breakout in progress), touching a band with an extreme
// RSI is a reversion entry.
// TP = middle band (the mean). SL = ATR×RangingATRStopMult beyond the band.
func (s *MultiFactorStrategy) analyzeRanging(candles []model.Candle, atr float64) (*strategy.Signal, error) {
	upper, middle, lower := s.calculateBollingerBands(candles, s.config.BollingerPeriod, s.config.BollingerStdDev)
	if upper == 0 || lower == 0 || middle == 0 {
		return nil, strategy.ErrNoSignal
	}

	// Guard: skip if bands are rapidly expanding (breakout in progress).
	// Compare current width to the average of the last 50 candles.
	currentWidth := (upper - lower) / middle
	avgWidth := s.calculateAvgBandWidth(candles, 50)
	if avgWidth > 0 && currentWidth > avgWidth*1.4 {
		return nil, strategy.ErrNoSignal // bands expanding = directional move, not a range
	}

	rsi := s.calculateRSI(candles, s.config.RSIPeriod)
	lastRSI := rsi[len(rsi)-1]
	price := candles[len(candles)-1].Close
	lastCandle := candles[len(candles)-1]
	prevCandle := candles[len(candles)-2]

	// Trend direction filter: use 100-period EMA to determine bias.
	// Only take mean-reversion entries aligned with the macro trend to avoid
	// shorting bull-market upper-band breakouts or longing bear-market breakdowns.
	longBias, shortBias := s.calcTrendBias(candles, 100)

	// LONG: price at/below lower band + RSI oversold + bullish candle reversal + trend allows longs
	nearLower := price <= lower*1.005
	longConfirm := lastCandle.Close > lastCandle.Open || prevCandle.Close > prevCandle.Open
	if nearLower && lastRSI < s.config.RangingRSIOversold && longConfirm && longBias {
		sl := lower - atr*s.config.RangingATRStopMult
		return &strategy.Signal{
			Type:       strategy.SignalBuy,
			Symbol:     s.symbol,
			Price:      price,
			SL:         sl,
			TP:         middle,
			Confidence: 0.70,
			Reason: fmt.Sprintf("Range-reversion LONG: RSI=%.1f at lower BB (%.4f → mid %.4f)",
				lastRSI, lower, middle),
		}, nil
	}

	// SHORT: price at/above upper band + RSI overbought + bearish candle reversal + trend allows shorts
	nearUpper := price >= upper*0.995
	shortConfirm := lastCandle.Close < lastCandle.Open || prevCandle.Close < prevCandle.Open
	if nearUpper && lastRSI > s.config.RangingRSIOverbought && shortConfirm && shortBias {
		sl := upper + atr*s.config.RangingATRStopMult
		return &strategy.Signal{
			Type:       strategy.SignalSell,
			Symbol:     s.symbol,
			Price:      price,
			SL:         sl,
			TP:         middle,
			Confidence: 0.70,
			Reason: fmt.Sprintf("Range-reversion SHORT: RSI=%.1f at upper BB (%.4f → mid %.4f)",
				lastRSI, upper, middle),
		}, nil
	}

	return nil, strategy.ErrNoSignal
}

// calcTrendBias returns (allowLong, allowShort) based on the 100-period EMA slope.
// Price above EMA = bullish bias (allow longs, block shorts).
// Price below EMA = bearish bias (allow shorts, block longs).
// Within 1% of EMA = neutral (allow both).
func (s *MultiFactorStrategy) calcTrendBias(candles []model.Candle, emaPeriod int) (longBias, shortBias bool) {
	if len(candles) < emaPeriod {
		return true, true // not enough data — allow both
	}
	emaVals := s.calculateEMA(candles, emaPeriod)
	longEMA := emaVals[len(emaVals)-1]
	price := candles[len(candles)-1].Close
	pct := (price - longEMA) / longEMA
	if pct > 0.01 { // price >1% above EMA → bull bias
		return true, false
	}
	if pct < -0.01 { // price >1% below EMA → bear bias
		return false, true
	}
	return true, true // neutral zone
}

// calculateAvgBandWidth calculates the average (upper-lower)/middle BB width
// over the last N candles — used to detect band expansion (breakouts).
func (s *MultiFactorStrategy) calculateAvgBandWidth(candles []model.Candle, lookback int) float64 {
	if len(candles) < s.config.BollingerPeriod+lookback {
		return 0
	}
	var widths []float64
	start := len(candles) - lookback
	for i := start; i < len(candles); i++ {
		u, m, l := s.calculateBollingerBands(candles[:i+1], s.config.BollingerPeriod, s.config.BollingerStdDev)
		if m > 0 {
			widths = append(widths, (u-l)/m)
		}
	}
	if len(widths) == 0 {
		return 0
	}
	sum := 0.0
	for _, w := range widths {
		sum += w
	}
	return sum / float64(len(widths))
}

// calculateBollingerBands returns upper, middle (SMA), and lower Bollinger Bands.
func (s *MultiFactorStrategy) calculateBollingerBands(candles []model.Candle, period int, stdDevMult float64) (upper, middle, lower float64) {
	if len(candles) < period {
		return 0, 0, 0
	}

	recent := candles[len(candles)-period:]

	sum := 0.0
	for _, c := range recent {
		sum += c.Close
	}
	middle = sum / float64(period)

	variance := 0.0
	for _, c := range recent {
		diff := c.Close - middle
		variance += diff * diff
	}
	stdDev := math.Sqrt(variance / float64(period))

	upper = middle + stdDevMult*stdDev
	lower = middle - stdDevMult*stdDev
	return
}

func (s *MultiFactorStrategy) generateSignal(
	regime MarketRegime,
	price, fastEMA, slowEMA, rsi, volumeRatio, atr float64,
	candles []model.Candle,
) *strategy.Signal {

	// Check volume confirmation
	if volumeRatio < s.config.VolumeThreshold {
		return nil
	}

	// Calculate recent momentum to avoid entering against current direction
	recentMomentum := s.calculateRecentMomentum(candles, 5)

	var signalType strategy.SignalType
	var stopLoss, takeProfit float64

	// Macro trend filter: 50-period EMA determines the dominant direction.
	// Block LONGs when price is more than 2% below the 50-EMA (sustained downtrend)
	// and block SHORTs when price is more than 2% above it (sustained uptrend).
	// This prevents entering counter-trend on dead-cat bounces or bull-market dips.
	macroEMA := s.calculateEMA(candles, 50)
	lastMacroEMA := macroEMA[len(macroEMA)-1]
	macroPct := (price - lastMacroEMA) / lastMacroEMA
	macroAllowLong := macroPct > -0.02  // price not more than 2% below 50-EMA
	macroAllowShort := macroPct < 0.02  // price not more than 2% above 50-EMA

	// LONG conditions - now includes momentum check
	longConditions := []bool{
		regime == RegimeTrendingUp || regime == RegimeHighVolatility,
		fastEMA > slowEMA,              // EMA crossover bullish
		price > fastEMA,                 // Price above fast EMA
		rsi > 40 && rsi < s.config.RSIOverbought, // RSI not overbought
		s.isHigherLow(candles),         // Structure confirmation
		recentMomentum >= 0,            // Don't go long if price falling
		macroAllowLong,                 // Price not deep below 50-EMA (no LONG in crashes)
	}

	// SHORT conditions - now includes momentum check
	shortConditions := []bool{
		regime == RegimeTrendingDown || regime == RegimeHighVolatility,
		fastEMA < slowEMA,              // EMA crossover bearish
		price < fastEMA,                 // Price below fast EMA
		rsi < 60 && rsi > s.config.RSIOversold, // RSI not oversold
		s.isLowerHigh(candles),         // Structure confirmation
		recentMomentum <= 0,            // Don't go short if price rising
		macroAllowShort,                // Price not deep above 50-EMA (no SHORT in rallies)
	}

	longScore := countTrue(longConditions)
	shortScore := countTrue(shortConditions)

	// Require at least 6 out of 7 conditions.
	// The macro-EMA filter is condition 7 and acts as a hard gate: if price is
	// deep against the 50-EMA, that condition alone drops the score below the
	// threshold even if all other 6 are met.
	minScore := 6

	if longScore >= minScore && longScore > shortScore {
		signalType = strategy.SignalBuy
		stopLoss = price - (atr * s.config.ATRStopMult)
		takeProfit = price + (atr * s.config.ATRTargetMult)
	} else if shortScore >= minScore && shortScore > longScore {
		signalType = strategy.SignalSell
		stopLoss = price + (atr * s.config.ATRStopMult)
		takeProfit = price - (atr * s.config.ATRTargetMult)
	} else {
		return nil
	}

	confidence := float64(max(longScore, shortScore)) / 7.0

	return &strategy.Signal{
		Type:       signalType,
		Symbol:     s.symbol,
		Price:      price,
		SL:         stopLoss,
		TP:         takeProfit,
		Confidence: confidence,
		Reason:     fmt.Sprintf("Regime: %s, RSI: %.1f, Momentum: %.2f%%, Score: %d/7", regime, rsi, recentMomentum*100, max(longScore, shortScore)),
	}
}

// calculateRecentMomentum calculates the percentage change over the last N candles
// Returns positive for upward movement, negative for downward
func (s *MultiFactorStrategy) calculateRecentMomentum(candles []model.Candle, periods int) float64 {
	if len(candles) < periods+1 {
		return 0
	}

	currentClose := candles[len(candles)-1].Close
	pastClose := candles[len(candles)-periods-1].Close

	if pastClose == 0 {
		return 0
	}

	return (currentClose - pastClose) / pastClose
}

// isHigherLow checks if recent price action shows higher lows (bullish).
// Uses 20 candles (80h on 4h TF) for a more stable structural read.
func (s *MultiFactorStrategy) isHigherLow(candles []model.Candle) bool {
	if len(candles) < 20 {
		return false
	}

	recent := candles[len(candles)-20:]

	// Find two recent swing lows
	var swingLows []float64
	for i := 2; i < len(recent)-2; i++ {
		if recent[i].Low < recent[i-1].Low &&
			recent[i].Low < recent[i-2].Low &&
			recent[i].Low < recent[i+1].Low &&
			recent[i].Low < recent[i+2].Low {
			swingLows = append(swingLows, recent[i].Low)
		}
	}

	if len(swingLows) >= 2 {
		return swingLows[len(swingLows)-1] > swingLows[len(swingLows)-2]
	}

	// Fallback: compare first half low to second half low
	firstHalf := recent[:10]
	secondHalf := recent[10:]
	return minLow(secondHalf) > minLow(firstHalf)*0.99
}

// isLowerHigh checks if recent price action shows lower highs (bearish).
// Uses 20 candles (80h on 4h TF) for a more stable structural read.
func (s *MultiFactorStrategy) isLowerHigh(candles []model.Candle) bool {
	if len(candles) < 20 {
		return false
	}

	recent := candles[len(candles)-20:]

	// Find two recent swing highs
	var swingHighs []float64
	for i := 2; i < len(recent)-2; i++ {
		if recent[i].High > recent[i-1].High &&
			recent[i].High > recent[i-2].High &&
			recent[i].High > recent[i+1].High &&
			recent[i].High > recent[i+2].High {
			swingHighs = append(swingHighs, recent[i].High)
		}
	}

	if len(swingHighs) >= 2 {
		return swingHighs[len(swingHighs)-1] < swingHighs[len(swingHighs)-2]
	}

	// Fallback: compare first half high to second half high
	firstHalf := recent[:10]
	secondHalf := recent[10:]
	return maxHigh(secondHalf) < maxHigh(firstHalf)*1.01
}

func (s *MultiFactorStrategy) calculateEMA(candles []model.Candle, period int) []float64 {
	closes := make([]float64, len(candles))
	for i, c := range candles {
		closes[i] = c.Close
	}
	return ema(closes, period)
}

func (s *MultiFactorStrategy) calculateRSI(candles []model.Candle, period int) []float64 {
	if len(candles) < period+1 {
		return []float64{50}
	}

	var gains, losses []float64

	for i := 1; i < len(candles); i++ {
		change := candles[i].Close - candles[i-1].Close
		if change > 0 {
			gains = append(gains, change)
			losses = append(losses, 0)
		} else {
			gains = append(gains, 0)
			losses = append(losses, -change)
		}
	}

	avgGain := ema(gains, period)
	avgLoss := ema(losses, period)

	var rsi []float64
	for i := 0; i < len(avgGain); i++ {
		if avgLoss[i] == 0 {
			rsi = append(rsi, 100)
		} else {
			rs := avgGain[i] / avgLoss[i]
			rsi = append(rsi, 100-(100/(1+rs)))
		}
	}

	return rsi
}

func (s *MultiFactorStrategy) calculateVolumeRatio(candles []model.Candle, period int) float64 {
	if len(candles) < period+1 {
		return 1.0
	}

	currentVolume := candles[len(candles)-1].Volume

	var sum float64
	for i := len(candles) - period - 1; i < len(candles)-1; i++ {
		sum += candles[i].Volume
	}
	avgVolume := sum / float64(period)

	if avgVolume == 0 {
		return 1.0
	}

	return currentVolume / avgVolume
}

func countTrue(conditions []bool) int {
	count := 0
	for _, c := range conditions {
		if c {
			count++
		}
	}
	return count
}

func minLow(candles []model.Candle) float64 {
	if len(candles) == 0 {
		return 0
	}
	minVal := candles[0].Low
	for _, c := range candles {
		if c.Low < minVal {
			minVal = c.Low
		}
	}
	return minVal
}

func maxHigh(candles []model.Candle) float64 {
	if len(candles) == 0 {
		return 0
	}
	maxVal := candles[0].High
	for _, c := range candles {
		if c.High > maxVal {
			maxVal = c.High
		}
	}
	return maxVal
}

func max(values ...int) int {
	if len(values) == 0 {
		return 0
	}
	maxVal := values[0]
	for _, v := range values {
		if v > maxVal {
			maxVal = v
		}
	}
	return maxVal
}

// CalculateATR returns the current ATR value
func (s *MultiFactorStrategy) CalculateATR(candles []model.Candle) float64 {
	if len(candles) < s.config.ATRPeriod+1 {
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

	atrSeries := ema(trs, s.config.ATRPeriod)
	if len(atrSeries) > 0 {
		return atrSeries[len(atrSeries)-1]
	}
	return 0
}

// Description returns the strategy description
func (s *MultiFactorStrategy) Description() string {
	return "Multi-factor trend-following strategy combining regime detection, EMA crossover, RSI, volume confirmation, and price structure analysis"
}

// Parameters returns current strategy parameters
func (s *MultiFactorStrategy) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"fastEMA":         s.config.FastEMA,
		"slowEMA":         s.config.SlowEMA,
		"rsiPeriod":       s.config.RSIPeriod,
		"rsiOverbought":   s.config.RSIOverbought,
		"rsiOversold":     s.config.RSIOversold,
		"volumePeriod":    s.config.VolumePeriod,
		"volumeThreshold": s.config.VolumeThreshold,
		"atrPeriod":       s.config.ATRPeriod,
		"atrStopMult":     s.config.ATRStopMult,
		"atrTargetMult":   s.config.ATRTargetMult,
		"requireTrend":    s.config.RequireTrend,
		"minADX":          s.config.MinADX,
	}
}

// SetParameters updates strategy parameters
func (s *MultiFactorStrategy) SetParameters(params map[string]interface{}) error {
	if v, ok := params["fastEMA"].(int); ok {
		s.config.FastEMA = v
	}
	if v, ok := params["slowEMA"].(int); ok {
		s.config.SlowEMA = v
	}
	if v, ok := params["rsiPeriod"].(int); ok {
		s.config.RSIPeriod = v
	}
	if v, ok := params["rsiOverbought"].(float64); ok {
		s.config.RSIOverbought = v
	}
	if v, ok := params["rsiOversold"].(float64); ok {
		s.config.RSIOversold = v
	}
	if v, ok := params["volumePeriod"].(int); ok {
		s.config.VolumePeriod = v
	}
	if v, ok := params["volumeThreshold"].(float64); ok {
		s.config.VolumeThreshold = v
	}
	if v, ok := params["atrPeriod"].(int); ok {
		s.config.ATRPeriod = v
	}
	if v, ok := params["atrStopMult"].(float64); ok {
		s.config.ATRStopMult = v
	}
	if v, ok := params["atrTargetMult"].(float64); ok {
		s.config.ATRTargetMult = v
	}
	if v, ok := params["requireTrend"].(bool); ok {
		s.config.RequireTrend = v
	}
	if v, ok := params["minADX"].(float64); ok {
		s.config.MinADX = v
	}
	return nil
}
