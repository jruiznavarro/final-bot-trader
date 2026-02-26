package strategy

import (
	"fmt"
	"math"

	"final-bot-trader-api/internal/exchange/model"
	"final-bot-trader-api/internal/indicator"
)

// ConfluenceStrategy implements a multi-indicator confluence strategy
// It only generates signals when multiple indicators agree (confluence)
// This reduces false signals significantly compared to single-indicator strategies
type ConfluenceStrategy struct {
	BaseStrategy

	// Trend indicators
	emaFast   *indicator.EMA // Fast EMA for trend direction
	emaSlow   *indicator.EMA // Slow EMA for trend confirmation
	macd      *indicator.MACD

	// Momentum indicators
	rsi       *indicator.RSI

	// Volatility indicators
	atr       *indicator.ATR
	bollinger *indicator.BollingerBands

	// Configuration
	config ConfluenceConfig

	// Scoring thresholds
	minScoreForEntry float64 // Minimum score to generate a signal (0-100)
}

// ConfluenceConfig holds configuration for the confluence strategy
type ConfluenceConfig struct {
	// EMA periods
	EMAFastPeriod int
	EMASlowPeriod int

	// MACD settings
	MACDFast   int
	MACDSlow   int
	MACDSignal int

	// RSI settings
	RSIPeriod     int
	RSIOverbought float64
	RSIOversold   float64

	// ATR settings
	ATRPeriod          int
	ATRVolatilityMin   float64 // Minimum ATR as % of price to trade
	ATRVolatilityMax   float64 // Maximum ATR as % of price (avoid extreme volatility)

	// Bollinger settings
	BollingerPeriod     int
	BollingerDeviations float64

	// Scoring weights (must sum to 100)
	WeightTrend      float64 // EMA alignment weight
	WeightMACD       float64 // MACD signal weight
	WeightRSI        float64 // RSI confirmation weight
	WeightBollinger  float64 // Bollinger position weight
	WeightVolatility float64 // Volatility filter weight

	// Entry threshold
	MinConfluenceScore float64 // Minimum score (0-100) to generate signal
}

// DefaultConfluenceConfig returns a well-tested default configuration
func DefaultConfluenceConfig() ConfluenceConfig {
	return ConfluenceConfig{
		// Trend: 21/55 EMA is a classic combination
		EMAFastPeriod: 21,
		EMASlowPeriod: 55,

		// Standard MACD
		MACDFast:   12,
		MACDSlow:   26,
		MACDSignal: 9,

		// RSI with slightly tighter levels for confluence
		RSIPeriod:     14,
		RSIOverbought: 65, // Tighter than standard 70
		RSIOversold:   35, // Tighter than standard 30

		// ATR for volatility
		ATRPeriod:        14,
		ATRVolatilityMin: 0.5,  // Minimum 0.5% daily range
		ATRVolatilityMax: 5.0,  // Maximum 5% daily range

		// Standard Bollinger
		BollingerPeriod:     20,
		BollingerDeviations: 2.0,

		// Scoring weights (total = 100)
		WeightTrend:      25,
		WeightMACD:       25,
		WeightRSI:        20,
		WeightBollinger:  15,
		WeightVolatility: 15,

		// Require 70% confluence for entry
		MinConfluenceScore: 70,
	}
}

// AggressiveConfluenceConfig for more frequent trading
func AggressiveConfluenceConfig() ConfluenceConfig {
	config := DefaultConfluenceConfig()
	config.EMAFastPeriod = 9
	config.EMASlowPeriod = 21
	config.RSIOverbought = 60
	config.RSIOversold = 40
	config.MinConfluenceScore = 60
	return config
}

// ConservativeConfluenceConfig for fewer but higher quality signals
func ConservativeConfluenceConfig() ConfluenceConfig {
	config := DefaultConfluenceConfig()
	config.EMAFastPeriod = 34
	config.EMASlowPeriod = 89
	config.RSIOverbought = 70
	config.RSIOversold = 30
	config.MinConfluenceScore = 80
	return config
}

// NewConfluenceStrategy creates a new multi-indicator confluence strategy
func NewConfluenceStrategy(symbol string, config ConfluenceConfig) (*ConfluenceStrategy, error) {
	// Validate weights
	totalWeight := config.WeightTrend + config.WeightMACD + config.WeightRSI +
		config.WeightBollinger + config.WeightVolatility
	if math.Abs(totalWeight-100) > 0.01 {
		return nil, fmt.Errorf("weights must sum to 100, got %.2f", totalWeight)
	}

	// Create indicators
	emaFast, err := indicator.NewEMA(config.EMAFastPeriod)
	if err != nil {
		return nil, fmt.Errorf("failed to create fast EMA: %w", err)
	}

	emaSlow, err := indicator.NewEMA(config.EMASlowPeriod)
	if err != nil {
		return nil, fmt.Errorf("failed to create slow EMA: %w", err)
	}

	macd, err := indicator.NewMACD(config.MACDFast, config.MACDSlow, config.MACDSignal)
	if err != nil {
		return nil, fmt.Errorf("failed to create MACD: %w", err)
	}

	rsi, err := indicator.NewRSIWithLevels(config.RSIPeriod, config.RSIOverbought, config.RSIOversold)
	if err != nil {
		return nil, fmt.Errorf("failed to create RSI: %w", err)
	}

	atr, err := indicator.NewATR(config.ATRPeriod)
	if err != nil {
		return nil, fmt.Errorf("failed to create ATR: %w", err)
	}

	bollinger, err := indicator.NewBollingerBands(config.BollingerPeriod, config.BollingerDeviations)
	if err != nil {
		return nil, fmt.Errorf("failed to create Bollinger Bands: %w", err)
	}

	return &ConfluenceStrategy{
		BaseStrategy:     NewBaseStrategy(symbol),
		emaFast:          emaFast,
		emaSlow:          emaSlow,
		macd:             macd,
		rsi:              rsi,
		atr:              atr,
		bollinger:        bollinger,
		config:           config,
		minScoreForEntry: config.MinConfluenceScore,
	}, nil
}

// Name returns the strategy name
func (s *ConfluenceStrategy) Name() string {
	return fmt.Sprintf("Confluence(%d/%d,%.0f%%)",
		s.config.EMAFastPeriod, s.config.EMASlowPeriod, s.config.MinConfluenceScore)
}

// Description returns the strategy description
func (s *ConfluenceStrategy) Description() string {
	return "Multi-indicator confluence strategy that combines trend (EMA), momentum (MACD, RSI), " +
		"and volatility (ATR, Bollinger) indicators. Only generates signals when multiple indicators agree."
}

// MinimumCandles returns the minimum candles required
func (s *ConfluenceStrategy) MinimumCandles() int {
	// Need enough data for the slowest indicator
	maxPeriod := s.config.EMASlowPeriod
	if s.config.MACDSlow+s.config.MACDSignal > maxPeriod {
		maxPeriod = s.config.MACDSlow + s.config.MACDSignal
	}
	if s.config.BollingerPeriod > maxPeriod {
		maxPeriod = s.config.BollingerPeriod
	}
	return maxPeriod + 5 // Buffer for calculations
}

// Parameters returns the strategy parameters
func (s *ConfluenceStrategy) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"ema_fast":            s.config.EMAFastPeriod,
		"ema_slow":            s.config.EMASlowPeriod,
		"macd_fast":           s.config.MACDFast,
		"macd_slow":           s.config.MACDSlow,
		"macd_signal":         s.config.MACDSignal,
		"rsi_period":          s.config.RSIPeriod,
		"rsi_overbought":      s.config.RSIOverbought,
		"rsi_oversold":        s.config.RSIOversold,
		"atr_period":          s.config.ATRPeriod,
		"bollinger_period":    s.config.BollingerPeriod,
		"min_confluence":      s.config.MinConfluenceScore,
	}
}

// SetParameters updates strategy parameters
func (s *ConfluenceStrategy) SetParameters(params map[string]interface{}) error {
	// This is a complex strategy - for now just allow changing the threshold
	if score, ok := params["min_confluence"].(float64); ok {
		if score < 0 || score > 100 {
			return ErrInvalidParameters
		}
		s.minScoreForEntry = score
		s.config.MinConfluenceScore = score
	}
	return nil
}

// ConfluenceScore holds the breakdown of the confluence score
type ConfluenceScore struct {
	TrendScore      float64 // EMA alignment score
	MACDScore       float64 // MACD signal score
	RSIScore        float64 // RSI position score
	BollingerScore  float64 // Bollinger position score
	VolatilityScore float64 // Volatility filter score
	TotalScore      float64 // Weighted total
	Direction       SignalType // Suggested direction
	Reasons         []string // Explanation of each score
}

// Analyze analyzes the candles and returns a trading signal
func (s *ConfluenceStrategy) Analyze(candles []model.Candle) (*Signal, error) {
	if len(candles) < s.MinimumCandles() {
		return nil, ErrInsufficientData
	}

	// Calculate confluence score
	score, err := s.CalculateConfluence(candles)
	if err != nil {
		return nil, err
	}

	// Check if score meets threshold
	if score.TotalScore < s.minScoreForEntry {
		return nil, ErrNoSignal
	}

	// Check if we have a clear direction
	if score.Direction == SignalNone {
		return nil, ErrNoSignal
	}

	currentPrice := candles[len(candles)-1].Close

	// Create signal
	signal := s.CreateSignal(score.Direction, currentPrice,
		fmt.Sprintf("Confluence score: %.1f%% (%s)", score.TotalScore, summarizeReasons(score.Reasons)))

	// Add indicator values
	signal.Indicators["confluence_score"] = score.TotalScore
	signal.Indicators["trend_score"] = score.TrendScore
	signal.Indicators["macd_score"] = score.MACDScore
	signal.Indicators["rsi_score"] = score.RSIScore
	signal.Indicators["bollinger_score"] = score.BollingerScore
	signal.Indicators["volatility_score"] = score.VolatilityScore

	// Confidence based on score
	signal.Confidence = score.TotalScore / 100.0

	return signal, nil
}

// CalculateConfluence calculates the full confluence score
func (s *ConfluenceStrategy) CalculateConfluence(candles []model.Candle) (*ConfluenceScore, error) {
	score := &ConfluenceScore{
		Reasons: make([]string, 0),
	}

	currentPrice := candles[len(candles)-1].Close

	// 1. Calculate Trend Score (EMA alignment)
	trendScore, trendDir, trendReason := s.calculateTrendScore(candles, currentPrice)
	score.TrendScore = trendScore
	score.Reasons = append(score.Reasons, trendReason)

	// 2. Calculate MACD Score
	macdScore, macdDir, macdReason := s.calculateMACDScore(candles)
	score.MACDScore = macdScore
	score.Reasons = append(score.Reasons, macdReason)

	// 3. Calculate RSI Score
	rsiScore, rsiDir, rsiReason := s.calculateRSIScore(candles)
	score.RSIScore = rsiScore
	score.Reasons = append(score.Reasons, rsiReason)

	// 4. Calculate Bollinger Score
	bollingerScore, bollingerDir, bollingerReason := s.calculateBollingerScore(candles, currentPrice)
	score.BollingerScore = bollingerScore
	score.Reasons = append(score.Reasons, bollingerReason)

	// 5. Calculate Volatility Filter Score
	volScore, volReason := s.calculateVolatilityScore(candles, currentPrice)
	score.VolatilityScore = volScore
	score.Reasons = append(score.Reasons, volReason)

	// Calculate weighted total
	score.TotalScore = (trendScore*s.config.WeightTrend +
		macdScore*s.config.WeightMACD +
		rsiScore*s.config.WeightRSI +
		bollingerScore*s.config.WeightBollinger +
		volScore*s.config.WeightVolatility) / 100.0

	// Determine overall direction by voting
	score.Direction = s.determineDirection(trendDir, macdDir, rsiDir, bollingerDir)

	return score, nil
}

func (s *ConfluenceStrategy) calculateTrendScore(candles []model.Candle, currentPrice float64) (float64, SignalType, string) {
	fastEMA, err := s.emaFast.Calculate(candles)
	if err != nil || len(fastEMA) == 0 {
		return 0, SignalNone, "EMA: insufficient data"
	}

	slowEMA, err := s.emaSlow.Calculate(candles)
	if err != nil || len(slowEMA) == 0 {
		return 0, SignalNone, "EMA: insufficient data"
	}

	currentFast := fastEMA[len(fastEMA)-1]
	currentSlow := slowEMA[len(slowEMA)-1]

	// Calculate EMA slope (momentum)
	var fastSlope, slowSlope float64
	if len(fastEMA) >= 3 {
		fastSlope = (fastEMA[len(fastEMA)-1] - fastEMA[len(fastEMA)-3]) / fastEMA[len(fastEMA)-3] * 100
	}
	if len(slowEMA) >= 3 {
		slowSlope = (slowEMA[len(slowEMA)-1] - slowEMA[len(slowEMA)-3]) / slowEMA[len(slowEMA)-3] * 100
	}

	var score float64
	var direction SignalType
	var reason string

	// Bullish: Fast > Slow, price > both EMAs, both sloping up
	if currentFast > currentSlow && currentPrice > currentFast {
		score = 70 // Base score for bullish alignment
		direction = SignalBuy

		// Bonus for strong slope
		if fastSlope > 0 && slowSlope > 0 {
			score += 30 * math.Min(fastSlope, 1.0) // Up to 30 bonus
		}
		reason = fmt.Sprintf("EMA Bullish: Fast(%.0f) > Slow(%.0f), Price above", currentFast, currentSlow)
	} else if currentFast < currentSlow && currentPrice < currentFast {
		// Bearish: Fast < Slow, price < both EMAs
		score = 70
		direction = SignalSell

		if fastSlope < 0 && slowSlope < 0 {
			score += 30 * math.Min(abs(fastSlope), 1.0)
		}
		reason = fmt.Sprintf("EMA Bearish: Fast(%.0f) < Slow(%.0f), Price below", currentFast, currentSlow)
	} else {
		// Mixed signals
		score = 30
		direction = SignalNone
		reason = "EMA: Mixed/Neutral trend"
	}

	return math.Min(score, 100), direction, reason
}

func (s *ConfluenceStrategy) calculateMACDScore(candles []model.Candle) (float64, SignalType, string) {
	macdValues, err := s.macd.CalculateAll(candles)
	if err != nil || len(macdValues.MACD) < 2 {
		return 0, SignalNone, "MACD: insufficient data"
	}

	currentMACD := macdValues.MACD[len(macdValues.MACD)-1]
	previousMACD := macdValues.MACD[len(macdValues.MACD)-2]
	currentSignal := macdValues.Signal[len(macdValues.Signal)-1]
	previousSignal := macdValues.Signal[len(macdValues.Signal)-2]
	currentHist := macdValues.Histogram[len(macdValues.Histogram)-1]
	previousHist := macdValues.Histogram[len(macdValues.Histogram)-2]

	var score float64
	var direction SignalType
	var reason string

	// Check for crossover (highest score)
	if previousMACD <= previousSignal && currentMACD > currentSignal {
		score = 100
		direction = SignalBuy
		reason = "MACD: Bullish crossover"
	} else if previousMACD >= previousSignal && currentMACD < currentSignal {
		score = 100
		direction = SignalSell
		reason = "MACD: Bearish crossover"
	} else if currentMACD > currentSignal {
		// Above signal line but no crossover
		score = 60
		// Bonus if histogram is growing
		if currentHist > previousHist {
			score += 20
		}
		direction = SignalBuy
		reason = "MACD: Above signal line"
	} else if currentMACD < currentSignal {
		score = 60
		if currentHist < previousHist {
			score += 20
		}
		direction = SignalSell
		reason = "MACD: Below signal line"
	} else {
		score = 30
		direction = SignalNone
		reason = "MACD: Neutral"
	}

	// Check zero line position for extra confirmation
	if direction == SignalBuy && currentMACD > 0 {
		score = math.Min(score+10, 100)
		reason += ", above zero"
	} else if direction == SignalSell && currentMACD < 0 {
		score = math.Min(score+10, 100)
		reason += ", below zero"
	}

	return score, direction, reason
}

func (s *ConfluenceStrategy) calculateRSIScore(candles []model.Candle) (float64, SignalType, string) {
	rsiValues, err := s.rsi.Calculate(candles)
	if err != nil || len(rsiValues) < 2 {
		return 0, SignalNone, "RSI: insufficient data"
	}

	currentRSI := rsiValues[len(rsiValues)-1]
	previousRSI := rsiValues[len(rsiValues)-2]

	var score float64
	var direction SignalType
	var reason string

	// Oversold conditions favor buying
	if currentRSI < s.config.RSIOversold {
		if currentRSI > previousRSI {
			// RSI turning up from oversold - strong buy
			score = 100
			reason = fmt.Sprintf("RSI(%.1f): Oversold & rising", currentRSI)
		} else {
			// Still oversold but falling - wait
			score = 60
			reason = fmt.Sprintf("RSI(%.1f): Oversold", currentRSI)
		}
		direction = SignalBuy
	} else if currentRSI > s.config.RSIOverbought {
		// Overbought conditions favor selling
		if currentRSI < previousRSI {
			score = 100
			reason = fmt.Sprintf("RSI(%.1f): Overbought & falling", currentRSI)
		} else {
			score = 60
			reason = fmt.Sprintf("RSI(%.1f): Overbought", currentRSI)
		}
		direction = SignalSell
	} else if currentRSI > 50 {
		// Bullish momentum
		score = 50 + (currentRSI-50)*0.5 // 50-75 based on RSI
		direction = SignalBuy
		reason = fmt.Sprintf("RSI(%.1f): Bullish momentum", currentRSI)
	} else {
		// Bearish momentum
		score = 50 + (50-currentRSI)*0.5
		direction = SignalSell
		reason = fmt.Sprintf("RSI(%.1f): Bearish momentum", currentRSI)
	}

	return math.Min(score, 100), direction, reason
}

func (s *ConfluenceStrategy) calculateBollingerScore(candles []model.Candle, currentPrice float64) (float64, SignalType, string) {
	bbValues, err := s.bollinger.CalculateAll(candles)
	if err != nil || len(bbValues.Upper) == 0 {
		return 0, SignalNone, "Bollinger: insufficient data"
	}

	upper := bbValues.Upper[len(bbValues.Upper)-1]
	middle := bbValues.Middle[len(bbValues.Middle)-1]
	lower := bbValues.Lower[len(bbValues.Lower)-1]
	bandWidth := (upper - lower) / middle * 100

	var score float64
	var direction SignalType
	var reason string

	// Calculate %B (position within bands)
	percentB := (currentPrice - lower) / (upper - lower)

	if percentB <= 0.1 {
		// Near or below lower band - potential buy
		score = 90
		direction = SignalBuy
		reason = fmt.Sprintf("BB: Price at lower band (%%B=%.2f)", percentB)
	} else if percentB >= 0.9 {
		// Near or above upper band - potential sell
		score = 90
		direction = SignalSell
		reason = fmt.Sprintf("BB: Price at upper band (%%B=%.2f)", percentB)
	} else if percentB > 0.5 {
		// Above middle - bullish
		score = 50 + (percentB-0.5)*80 // 50-90
		direction = SignalBuy
		reason = fmt.Sprintf("BB: Price above middle (%%B=%.2f)", percentB)
	} else {
		// Below middle - bearish
		score = 50 + (0.5-percentB)*80
		direction = SignalSell
		reason = fmt.Sprintf("BB: Price below middle (%%B=%.2f)", percentB)
	}

	// Adjust for band squeeze (low volatility often precedes big moves)
	if bandWidth < 3.0 {
		score *= 0.7 // Reduce score during squeeze
		reason += ", squeeze detected"
	}

	return math.Min(score, 100), direction, reason
}

func (s *ConfluenceStrategy) calculateVolatilityScore(candles []model.Candle, currentPrice float64) (float64, string) {
	atrValue, err := s.atr.Value(candles)
	if err != nil {
		return 50, "ATR: insufficient data"
	}

	// ATR as percentage of price
	atrPercent := (atrValue / currentPrice) * 100

	var score float64
	var reason string

	if atrPercent < s.config.ATRVolatilityMin {
		// Too low volatility - poor trading conditions
		score = 30
		reason = fmt.Sprintf("ATR(%.2f%%): Low volatility - choppy market", atrPercent)
	} else if atrPercent > s.config.ATRVolatilityMax {
		// Too high volatility - risky
		score = 40
		reason = fmt.Sprintf("ATR(%.2f%%): High volatility - increased risk", atrPercent)
	} else {
		// Ideal volatility range
		// Score highest in the middle of the range
		midPoint := (s.config.ATRVolatilityMin + s.config.ATRVolatilityMax) / 2
		distanceFromMid := abs(atrPercent - midPoint)
		maxDistance := (s.config.ATRVolatilityMax - s.config.ATRVolatilityMin) / 2
		score = 100 - (distanceFromMid/maxDistance)*30 // 70-100 range
		reason = fmt.Sprintf("ATR(%.2f%%): Good volatility", atrPercent)
	}

	return score, reason
}

func (s *ConfluenceStrategy) determineDirection(directions ...SignalType) SignalType {
	buyVotes := 0
	sellVotes := 0

	for _, dir := range directions {
		switch dir {
		case SignalBuy:
			buyVotes++
		case SignalSell:
			sellVotes++
		}
	}

	// Need clear majority
	if buyVotes >= 3 && buyVotes > sellVotes {
		return SignalBuy
	}
	if sellVotes >= 3 && sellVotes > buyVotes {
		return SignalSell
	}

	return SignalNone
}

func summarizeReasons(reasons []string) string {
	if len(reasons) == 0 {
		return ""
	}
	if len(reasons) == 1 {
		return reasons[0]
	}
	// Return first 2 reasons
	return reasons[0] + "; " + reasons[1]
}

// GetDetailedAnalysis returns a detailed analysis for debugging/display
func (s *ConfluenceStrategy) GetDetailedAnalysis(candles []model.Candle) (map[string]interface{}, error) {
	score, err := s.CalculateConfluence(candles)
	if err != nil {
		return nil, err
	}

	return map[string]interface{}{
		"total_score":      score.TotalScore,
		"trend_score":      score.TrendScore,
		"macd_score":       score.MACDScore,
		"rsi_score":        score.RSIScore,
		"bollinger_score":  score.BollingerScore,
		"volatility_score": score.VolatilityScore,
		"direction":        score.Direction.String(),
		"threshold":        s.minScoreForEntry,
		"would_trade":      score.TotalScore >= s.minScoreForEntry && score.Direction != SignalNone,
		"reasons":          score.Reasons,
	}, nil
}
