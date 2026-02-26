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
	RequireTrend    bool    // Only trade in trending regimes
	MinADX          float64 // Minimum ADX to trade
}

// DefaultConfig returns optimized defaults based on walk-forward validation
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

// Analyze analyzes candles and generates a signal
func (s *MultiFactorStrategy) Analyze(candles []model.Candle) (*strategy.Signal, error) {
	if len(candles) < s.MinimumCandles() {
		return nil, strategy.ErrInsufficientData
	}

	// Step 1: Detect market regime
	regime, adx, atr := s.regime.DetectRegime(candles)

	// Step 2: Check if we should trade in this regime
	if s.config.RequireTrend && regime == RegimeRanging {
		return nil, strategy.ErrNoSignal
	}

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

	// Step 4: Generate signal based on multiple factors
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

	// LONG conditions - now includes momentum check
	longConditions := []bool{
		regime == RegimeTrendingUp || regime == RegimeHighVolatility,
		fastEMA > slowEMA,              // EMA crossover bullish
		price > fastEMA,                 // Price above fast EMA
		rsi > 40 && rsi < s.config.RSIOverbought, // RSI not overbought
		s.isHigherLow(candles),         // Structure confirmation
		recentMomentum >= 0,            // Don't go long if price falling
	}

	// SHORT conditions - now includes momentum check
	shortConditions := []bool{
		regime == RegimeTrendingDown || regime == RegimeHighVolatility,
		fastEMA < slowEMA,              // EMA crossover bearish
		price < fastEMA,                 // Price below fast EMA
		rsi < 60 && rsi > s.config.RSIOversold, // RSI not oversold
		s.isLowerHigh(candles),         // Structure confirmation
		recentMomentum <= 0,            // Don't go short if price rising
	}

	longScore := countTrue(longConditions)
	shortScore := countTrue(shortConditions)

	// Require at least 5 out of 6 conditions (including momentum)
	minScore := 5

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

	// Calculate position size based on risk
	// (This will be refined by risk manager)
	confidence := float64(max(longScore, shortScore)) / 6.0

	return &strategy.Signal{
		Type:       signalType,
		Symbol:     s.symbol,
		Price:      price,
		SL:         stopLoss,
		TP:         takeProfit,
		Confidence: confidence,
		Reason:     fmt.Sprintf("Regime: %s, RSI: %.1f, Momentum: %.2f%%, Score: %d/6", regime, rsi, recentMomentum*100, max(longScore, shortScore)),
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

// isHigherLow checks if recent price action shows higher lows (bullish)
func (s *MultiFactorStrategy) isHigherLow(candles []model.Candle) bool {
	if len(candles) < 10 {
		return false
	}

	recent := candles[len(candles)-10:]

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
	firstHalf := recent[:5]
	secondHalf := recent[5:]
	return minLow(secondHalf) > minLow(firstHalf)*0.99
}

// isLowerHigh checks if recent price action shows lower highs (bearish)
func (s *MultiFactorStrategy) isLowerHigh(candles []model.Candle) bool {
	if len(candles) < 10 {
		return false
	}

	recent := candles[len(candles)-10:]

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
	firstHalf := recent[:5]
	secondHalf := recent[5:]
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
