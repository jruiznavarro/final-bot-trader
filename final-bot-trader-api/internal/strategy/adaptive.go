package strategy

import (
	"fmt"
	"math"

	"final-bot-trader-api/internal/exchange/model"
	"final-bot-trader-api/internal/indicator"
)

// MarketRegime represents the current market conditions
type MarketRegime string

const (
	RegimeTrending    MarketRegime = "TRENDING"     // Strong directional movement
	RegimeRanging     MarketRegime = "RANGING"      // Sideways, mean-reverting
	RegimeVolatile    MarketRegime = "VOLATILE"     // High volatility, unpredictable
	RegimeQuiet       MarketRegime = "QUIET"        // Low volatility, consolidation
)

// AdaptiveStrategy adjusts its behavior based on detected market regime
// In trending markets: uses momentum/trend following
// In ranging markets: uses mean reversion
// In volatile markets: reduces position size and widens stops
// In quiet markets: waits for breakout
type AdaptiveStrategy struct {
	BaseStrategy

	// Regime detection indicators
	atr          *indicator.ATR
	atrLong      *indicator.ATR      // Longer ATR for regime comparison
	emaFast      *indicator.EMA
	emaSlow      *indicator.EMA
	rsi          *indicator.RSI
	bollinger    *indicator.BollingerBands

	// Configuration
	config AdaptiveConfig

	// Current state
	currentRegime MarketRegime
	regimeHistory []MarketRegime
}

// AdaptiveConfig holds configuration for the adaptive strategy
type AdaptiveConfig struct {
	// ATR periods for regime detection
	ATRShortPeriod int
	ATRLongPeriod  int

	// Trend detection
	EMAFastPeriod int
	EMASlowPeriod int

	// RSI for momentum
	RSIPeriod int

	// Bollinger for volatility
	BollingerPeriod     int
	BollingerDeviations float64

	// Regime thresholds
	TrendStrengthMin     float64 // Minimum EMA separation for trending regime
	VolatilityExpandRatio float64 // ATR short/long ratio for volatile regime
	VolatilityContractRatio float64 // ATR short/long ratio for quiet regime

	// Strategy parameters per regime
	TrendingRSIEntry    float64 // RSI threshold for trend following entry
	RangingRSIOversold  float64 // RSI oversold for mean reversion buy
	RangingRSIOverbought float64 // RSI overbought for mean reversion sell

	// Risk adjustments per regime
	TrendingRiskMultiplier  float64 // Position size multiplier in trending
	RangingRiskMultiplier   float64 // Position size multiplier in ranging
	VolatileRiskMultiplier  float64 // Position size multiplier in volatile
	QuietRiskMultiplier     float64 // Position size multiplier in quiet
}

// DefaultAdaptiveConfig returns default configuration
func DefaultAdaptiveConfig() AdaptiveConfig {
	return AdaptiveConfig{
		ATRShortPeriod: 14,
		ATRLongPeriod:  50,
		EMAFastPeriod:  21,
		EMASlowPeriod:  55,
		RSIPeriod:      14,
		BollingerPeriod:     20,
		BollingerDeviations: 2.0,

		TrendStrengthMin:       1.0,  // 1% EMA separation
		VolatilityExpandRatio:  1.5,  // ATR short 50% higher than long
		VolatilityContractRatio: 0.7, // ATR short 30% lower than long

		TrendingRSIEntry:     55,
		RangingRSIOversold:   30,
		RangingRSIOverbought: 70,

		TrendingRiskMultiplier: 1.0,
		RangingRiskMultiplier:  0.8,
		VolatileRiskMultiplier: 0.5,
		QuietRiskMultiplier:    0.3,
	}
}

// NewAdaptiveStrategy creates a new adaptive strategy
func NewAdaptiveStrategy(symbol string, config AdaptiveConfig) (*AdaptiveStrategy, error) {
	atr, err := indicator.NewATR(config.ATRShortPeriod)
	if err != nil {
		return nil, err
	}

	atrLong, err := indicator.NewATR(config.ATRLongPeriod)
	if err != nil {
		return nil, err
	}

	emaFast, err := indicator.NewEMA(config.EMAFastPeriod)
	if err != nil {
		return nil, err
	}

	emaSlow, err := indicator.NewEMA(config.EMASlowPeriod)
	if err != nil {
		return nil, err
	}

	rsi, err := indicator.NewRSI(config.RSIPeriod)
	if err != nil {
		return nil, err
	}

	bollinger, err := indicator.NewBollingerBands(config.BollingerPeriod, config.BollingerDeviations)
	if err != nil {
		return nil, err
	}

	return &AdaptiveStrategy{
		BaseStrategy:  NewBaseStrategy(symbol),
		atr:           atr,
		atrLong:       atrLong,
		emaFast:       emaFast,
		emaSlow:       emaSlow,
		rsi:           rsi,
		bollinger:     bollinger,
		config:        config,
		currentRegime: RegimeQuiet,
		regimeHistory: make([]MarketRegime, 0),
	}, nil
}

// Name returns the strategy name
func (s *AdaptiveStrategy) Name() string {
	return fmt.Sprintf("Adaptive(%d/%d)", s.config.EMAFastPeriod, s.config.EMASlowPeriod)
}

// Description returns the strategy description
func (s *AdaptiveStrategy) Description() string {
	return "Adaptive strategy that detects market regime (trending/ranging/volatile/quiet) " +
		"and adjusts its trading approach accordingly. Uses trend-following in trends, " +
		"mean-reversion in ranges, and reduces risk in volatile or quiet markets."
}

// MinimumCandles returns the minimum candles required
func (s *AdaptiveStrategy) MinimumCandles() int {
	return s.config.ATRLongPeriod + 10
}

// Parameters returns the strategy parameters
func (s *AdaptiveStrategy) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"atr_short":       s.config.ATRShortPeriod,
		"atr_long":        s.config.ATRLongPeriod,
		"ema_fast":        s.config.EMAFastPeriod,
		"ema_slow":        s.config.EMASlowPeriod,
		"rsi_period":      s.config.RSIPeriod,
		"current_regime":  string(s.currentRegime),
	}
}

// SetParameters updates strategy parameters
func (s *AdaptiveStrategy) SetParameters(params map[string]interface{}) error {
	return nil // Complex strategy - parameters fixed at creation
}

// Analyze analyzes the candles and returns a trading signal
func (s *AdaptiveStrategy) Analyze(candles []model.Candle) (*Signal, error) {
	if len(candles) < s.MinimumCandles() {
		return nil, ErrInsufficientData
	}

	// Detect current market regime
	regime, regimeInfo, err := s.detectRegime(candles)
	if err != nil {
		return nil, err
	}

	s.currentRegime = regime
	s.regimeHistory = append(s.regimeHistory, regime)
	if len(s.regimeHistory) > 100 {
		s.regimeHistory = s.regimeHistory[1:]
	}

	// Generate signal based on regime
	var signal *Signal

	switch regime {
	case RegimeTrending:
		signal, err = s.analyzeTrending(candles, regimeInfo)
	case RegimeRanging:
		signal, err = s.analyzeRanging(candles, regimeInfo)
	case RegimeVolatile:
		signal, err = s.analyzeVolatile(candles, regimeInfo)
	case RegimeQuiet:
		// In quiet regime, wait for breakout
		signal, err = s.analyzeQuiet(candles, regimeInfo)
	}

	if err != nil {
		return nil, err
	}

	if signal != nil {
		signal.Indicators["regime"] = float64(regimeToInt(regime))
		signal.Indicators["regime_name"] = 0 // Will be overwritten in display

		// Adjust confidence based on regime consistency
		regimeConsistency := s.calculateRegimeConsistency()
		signal.Confidence *= regimeConsistency
	}

	return signal, nil
}

// RegimeInfo holds information about the detected regime
type RegimeInfo struct {
	Regime        MarketRegime
	TrendDirection SignalType  // Bullish or Bearish trend
	TrendStrength float64     // Strength of the trend (0-100)
	Volatility    float64     // Current volatility level
	EMASeparation float64     // EMA fast/slow separation %
	ATRRatio      float64     // Short ATR / Long ATR
}

func (s *AdaptiveStrategy) detectRegime(candles []model.Candle) (MarketRegime, *RegimeInfo, error) {
	currentPrice := candles[len(candles)-1].Close

	// Get ATR values
	atrShort, err := s.atr.Value(candles)
	if err != nil {
		return RegimeQuiet, nil, err
	}

	atrLong, err := s.atrLong.Value(candles)
	if err != nil {
		return RegimeQuiet, nil, err
	}

	// Get EMA values
	fastEMA, err := s.emaFast.Calculate(candles)
	if err != nil {
		return RegimeQuiet, nil, err
	}

	slowEMA, err := s.emaSlow.Calculate(candles)
	if err != nil {
		return RegimeQuiet, nil, err
	}

	currentFast := fastEMA[len(fastEMA)-1]
	currentSlow := slowEMA[len(slowEMA)-1]

	// Calculate metrics
	atrRatio := atrShort / atrLong
	emaSeparation := abs(currentFast-currentSlow) / currentSlow * 100
	volatility := atrShort / currentPrice * 100

	info := &RegimeInfo{
		Volatility:    volatility,
		EMASeparation: emaSeparation,
		ATRRatio:      atrRatio,
	}

	// Determine trend direction
	if currentFast > currentSlow {
		info.TrendDirection = SignalBuy
	} else {
		info.TrendDirection = SignalSell
	}

	// Calculate trend strength based on EMA alignment and slope
	var fastSlope float64
	if len(fastEMA) >= 5 {
		fastSlope = (fastEMA[len(fastEMA)-1] - fastEMA[len(fastEMA)-5]) / fastEMA[len(fastEMA)-5] * 100
	}
	info.TrendStrength = math.Min(abs(fastSlope)*20+emaSeparation*10, 100)

	// Classify regime
	var regime MarketRegime

	// High volatility expansion
	if atrRatio > s.config.VolatilityExpandRatio {
		regime = RegimeVolatile
	} else if atrRatio < s.config.VolatilityContractRatio {
		// Low volatility contraction
		regime = RegimeQuiet
	} else if emaSeparation > s.config.TrendStrengthMin {
		// EMAs clearly separated - trending
		regime = RegimeTrending
	} else {
		// EMAs close together - ranging
		regime = RegimeRanging
	}

	info.Regime = regime
	return regime, info, nil
}

func (s *AdaptiveStrategy) analyzeTrending(candles []model.Candle, info *RegimeInfo) (*Signal, error) {
	// In trending regime: follow the trend using momentum
	rsiValues, err := s.rsi.Calculate(candles)
	if err != nil {
		return nil, err
	}

	currentRSI := rsiValues[len(rsiValues)-1]
	currentPrice := candles[len(candles)-1].Close

	// For uptrend: buy on RSI pullbacks above threshold
	if info.TrendDirection == SignalBuy {
		// RSI pulling back but still showing strength
		if currentRSI > s.config.TrendingRSIEntry && currentRSI < 70 {
			signal := s.CreateSignal(SignalBuy, currentPrice,
				fmt.Sprintf("Trending UP: RSI=%.1f, Trend Strength=%.1f%%", currentRSI, info.TrendStrength))
			signal.Indicators["rsi"] = currentRSI
			signal.Indicators["trend_strength"] = info.TrendStrength
			signal.Confidence = info.TrendStrength / 100 * s.config.TrendingRiskMultiplier
			return signal, nil
		}
	}

	// For downtrend: sell on RSI bounces below threshold
	if info.TrendDirection == SignalSell {
		if currentRSI < (100-s.config.TrendingRSIEntry) && currentRSI > 30 {
			signal := s.CreateSignal(SignalSell, currentPrice,
				fmt.Sprintf("Trending DOWN: RSI=%.1f, Trend Strength=%.1f%%", currentRSI, info.TrendStrength))
			signal.Indicators["rsi"] = currentRSI
			signal.Indicators["trend_strength"] = info.TrendStrength
			signal.Confidence = info.TrendStrength / 100 * s.config.TrendingRiskMultiplier
			return signal, nil
		}
	}

	return nil, ErrNoSignal
}

func (s *AdaptiveStrategy) analyzeRanging(candles []model.Candle, info *RegimeInfo) (*Signal, error) {
	// In ranging regime: mean reversion at extremes
	rsiValues, err := s.rsi.Calculate(candles)
	if err != nil {
		return nil, err
	}

	bbValues, err := s.bollinger.CalculateAll(candles)
	if err != nil {
		return nil, err
	}

	currentRSI := rsiValues[len(rsiValues)-1]
	previousRSI := rsiValues[len(rsiValues)-2]
	currentPrice := candles[len(candles)-1].Close

	lower := bbValues.Lower[len(bbValues.Lower)-1]
	upper := bbValues.Upper[len(bbValues.Upper)-1]

	// Buy: RSI oversold AND price near lower Bollinger AND RSI turning up
	if currentRSI < s.config.RangingRSIOversold && currentPrice < lower*1.01 {
		if currentRSI > previousRSI { // RSI turning up
			signal := s.CreateSignal(SignalBuy, currentPrice,
				fmt.Sprintf("Ranging OVERSOLD: RSI=%.1f, Price near lower BB", currentRSI))
			signal.Indicators["rsi"] = currentRSI
			signal.Confidence = 0.7 * s.config.RangingRiskMultiplier
			return signal, nil
		}
	}

	// Sell: RSI overbought AND price near upper Bollinger AND RSI turning down
	if currentRSI > s.config.RangingRSIOverbought && currentPrice > upper*0.99 {
		if currentRSI < previousRSI { // RSI turning down
			signal := s.CreateSignal(SignalSell, currentPrice,
				fmt.Sprintf("Ranging OVERBOUGHT: RSI=%.1f, Price near upper BB", currentRSI))
			signal.Indicators["rsi"] = currentRSI
			signal.Confidence = 0.7 * s.config.RangingRiskMultiplier
			return signal, nil
		}
	}

	return nil, ErrNoSignal
}

func (s *AdaptiveStrategy) analyzeVolatile(candles []model.Candle, info *RegimeInfo) (*Signal, error) {
	// In volatile regime: only trade with strong confirmation and reduced size
	rsiValues, err := s.rsi.Calculate(candles)
	if err != nil {
		return nil, err
	}

	currentRSI := rsiValues[len(rsiValues)-1]
	previousRSI := rsiValues[len(rsiValues)-2]
	currentPrice := candles[len(candles)-1].Close

	// Only trade extreme RSI with reversal confirmation
	if currentRSI < 25 && currentRSI > previousRSI {
		signal := s.CreateSignal(SignalBuy, currentPrice,
			fmt.Sprintf("Volatile EXTREME: RSI=%.1f reversing from extreme", currentRSI))
		signal.Indicators["rsi"] = currentRSI
		signal.Indicators["volatility"] = info.Volatility
		signal.Confidence = 0.5 * s.config.VolatileRiskMultiplier // Lower confidence in volatile
		return signal, nil
	}

	if currentRSI > 75 && currentRSI < previousRSI {
		signal := s.CreateSignal(SignalSell, currentPrice,
			fmt.Sprintf("Volatile EXTREME: RSI=%.1f reversing from extreme", currentRSI))
		signal.Indicators["rsi"] = currentRSI
		signal.Indicators["volatility"] = info.Volatility
		signal.Confidence = 0.5 * s.config.VolatileRiskMultiplier
		return signal, nil
	}

	return nil, ErrNoSignal
}

func (s *AdaptiveStrategy) analyzeQuiet(candles []model.Candle, info *RegimeInfo) (*Signal, error) {
	// In quiet regime: wait for breakout
	bbValues, err := s.bollinger.CalculateAll(candles)
	if err != nil {
		return nil, err
	}

	currentPrice := candles[len(candles)-1].Close
	previousPrice := candles[len(candles)-2].Close

	upper := bbValues.Upper[len(bbValues.Upper)-1]
	lower := bbValues.Lower[len(bbValues.Lower)-1]
	prevUpper := bbValues.Upper[len(bbValues.Upper)-2]
	prevLower := bbValues.Lower[len(bbValues.Lower)-2]

	// Breakout above upper band
	if previousPrice <= prevUpper && currentPrice > upper {
		signal := s.CreateSignal(SignalBuy, currentPrice,
			fmt.Sprintf("Quiet BREAKOUT UP: Price broke above BB upper"))
		signal.Confidence = 0.6 * s.config.QuietRiskMultiplier
		return signal, nil
	}

	// Breakdown below lower band
	if previousPrice >= prevLower && currentPrice < lower {
		signal := s.CreateSignal(SignalSell, currentPrice,
			fmt.Sprintf("Quiet BREAKDOWN: Price broke below BB lower"))
		signal.Confidence = 0.6 * s.config.QuietRiskMultiplier
		return signal, nil
	}

	return nil, ErrNoSignal
}

func (s *AdaptiveStrategy) calculateRegimeConsistency() float64 {
	if len(s.regimeHistory) < 5 {
		return 0.7 // Default
	}

	// Count how many of the last 5 readings match current regime
	matches := 0
	for i := len(s.regimeHistory) - 5; i < len(s.regimeHistory); i++ {
		if s.regimeHistory[i] == s.currentRegime {
			matches++
		}
	}

	// 5/5 matches = 1.0, 3/5 = 0.8, 1/5 = 0.6
	return 0.6 + float64(matches)*0.08
}

// GetCurrentRegime returns the current detected market regime
func (s *AdaptiveStrategy) GetCurrentRegime() MarketRegime {
	return s.currentRegime
}

// GetRegimeAnalysis returns detailed regime analysis
func (s *AdaptiveStrategy) GetRegimeAnalysis(candles []model.Candle) (map[string]interface{}, error) {
	regime, info, err := s.detectRegime(candles)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"regime":          string(regime),
		"trend_direction": info.TrendDirection.String(),
		"trend_strength":  info.TrendStrength,
		"volatility":      info.Volatility,
		"ema_separation":  info.EMASeparation,
		"atr_ratio":       info.ATRRatio,
		"consistency":     s.calculateRegimeConsistency(),
	}, nil
}

func regimeToInt(r MarketRegime) int {
	switch r {
	case RegimeTrending:
		return 1
	case RegimeRanging:
		return 2
	case RegimeVolatile:
		return 3
	case RegimeQuiet:
		return 4
	default:
		return 0
	}
}
