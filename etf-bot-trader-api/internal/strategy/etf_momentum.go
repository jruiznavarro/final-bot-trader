package strategy

import (
	"fmt"
	"math"

	"etf-bot-trader-api/internal/exchange"
	"etf-bot-trader-api/internal/indicator"
)

// ETFMomentumConfig holds strategy configuration
type ETFMomentumConfig struct {
	// EMAs for trend
	FastEMA int
	SlowEMA int

	// RSI
	RSIPeriod     int
	RSIOverbought float64
	RSIOversold   float64

	// ATR for stops
	ATRPeriod     int
	ATRStopMult   float64 // SL distance = ATR * this
	ATRTargetMult float64 // TP distance = ATR * this

	// ADX for trend strength
	ADXPeriod int
	MinADX    float64

	// Volume
	MinVolumeRatio float64 // Minimum volume vs average

	// Momentum
	MomentumPeriod   int
	MinMomentum      float64 // Minimum momentum % to enter
}

// DefaultETFMomentumConfig returns optimized defaults for ETFs
func DefaultETFMomentumConfig() ETFMomentumConfig {
	return ETFMomentumConfig{
		FastEMA:          8,
		SlowEMA:          21,
		RSIPeriod:        14,
		RSIOverbought:    80,   // More room for overbought (ETFs can trend longer)
		RSIOversold:      20,   // More room for oversold
		ATRPeriod:        14,
		ATRStopMult:      2.0,  // Normal stops for ETFs
		ATRTargetMult:    3.0,  // 1.5 R:R ratio
		ADXPeriod:        14,
		MinADX:           20,   // Only trade when there's a decent trend
		MinVolumeRatio:   0.1,  // Very low for IEX data (much sparser than SIP)
		MomentumPeriod:   5,    // Shorter momentum period
		MinMomentum:      0.1,  // 0.1% minimum momentum (very low for stable ETFs)
	}
}

// ETFMomentumStrategy implements a momentum strategy for ETFs
type ETFMomentumStrategy struct {
	config ETFMomentumConfig
	symbol string
}

// NewETFMomentumStrategy creates a new ETF momentum strategy
func NewETFMomentumStrategy(symbol string, config ETFMomentumConfig) *ETFMomentumStrategy {
	return &ETFMomentumStrategy{
		config: config,
		symbol: symbol,
	}
}

// Name returns the strategy name
func (s *ETFMomentumStrategy) Name() string {
	return fmt.Sprintf("ETFMomentum(%d/%d)", s.config.FastEMA, s.config.SlowEMA)
}

// MinimumCandles returns minimum candles needed
func (s *ETFMomentumStrategy) MinimumCandles() int {
	return max(s.config.SlowEMA, s.config.RSIPeriod, s.config.ATRPeriod, s.config.ADXPeriod*2) + 10
}

// Analyze analyzes candles and generates a signal
func (s *ETFMomentumStrategy) Analyze(candles []exchange.Candle) (*Signal, error) {
	if len(candles) < s.MinimumCandles() {
		return nil, ErrInsufficientData
	}

	// Calculate indicators
	fastEMA := indicator.EMA(candles, s.config.FastEMA)
	slowEMA := indicator.EMA(candles, s.config.SlowEMA)
	rsi := indicator.RSI(candles, s.config.RSIPeriod)
	atr := indicator.ATR(candles, s.config.ATRPeriod)
	adx := indicator.ADX(candles, s.config.ADXPeriod)
	momentum := indicator.Momentum(candles, s.config.MomentumPeriod)

	// Get latest values
	n := len(candles) - 1
	currentPrice := candles[n].Close
	currentVolume := candles[n].Volume

	lastFastEMA := fastEMA[n]
	lastSlowEMA := slowEMA[n]
	lastRSI := rsi[n]
	lastATR := atr[n-1] // ATR is offset by 1
	lastMomentum := momentum[n]

	// Calculate ADX (need to handle offset)
	var lastADX float64
	if len(adx) > 0 {
		lastADX = adx[len(adx)-1]
	}

	// Calculate average volume
	avgVolume := s.calculateAvgVolume(candles, 20)
	volumeRatio := currentVolume / avgVolume

	// Check trend strength
	if lastADX < s.config.MinADX {
		return nil, ErrNoSignal
	}

	// Check volume (disabled for IEX data - volume is unreliable)
	// if volumeRatio < s.config.MinVolumeRatio {
	// 	return nil, ErrNoSignal
	// }
	_ = volumeRatio // Suppress unused variable warning

	// Generate signal
	signal := s.generateSignal(
		currentPrice,
		lastFastEMA,
		lastSlowEMA,
		lastRSI,
		lastATR,
		lastADX,
		lastMomentum,
		candles,
	)

	if signal == nil {
		return nil, ErrNoSignal
	}

	return signal, nil
}

func (s *ETFMomentumStrategy) generateSignal(
	price, fastEMA, slowEMA, rsi, atr, adx, momentum float64,
	candles []exchange.Candle,
) *Signal {

	// LONG conditions
	longConditions := []bool{
		fastEMA > slowEMA,                           // Bullish EMA crossover
		price > fastEMA,                              // Price above fast EMA
		rsi > 40 && rsi < s.config.RSIOverbought,    // RSI healthy
		momentum > s.config.MinMomentum,              // Positive momentum
		s.isUptrend(candles),                         // Higher highs and lows
	}

	// SHORT conditions
	shortConditions := []bool{
		fastEMA < slowEMA,                           // Bearish EMA crossover
		price < fastEMA,                              // Price below fast EMA
		rsi < 60 && rsi > s.config.RSIOversold,      // RSI healthy
		momentum < -s.config.MinMomentum,             // Negative momentum
		s.isDowntrend(candles),                       // Lower highs and lows
	}

	longScore := countTrue(longConditions)
	shortScore := countTrue(shortConditions)

	// Require at least 4 out of 5 conditions for higher quality signals
	minScore := 4

	var signalType SignalType
	var stopLoss, takeProfit float64

	if longScore >= minScore && longScore > shortScore {
		signalType = SignalBuy
		stopLoss = price - (atr * s.config.ATRStopMult)
		takeProfit = price + (atr * s.config.ATRTargetMult)
	} else if shortScore >= minScore && shortScore > longScore {
		signalType = SignalSell
		stopLoss = price + (atr * s.config.ATRStopMult)
		takeProfit = price - (atr * s.config.ATRTargetMult)
	} else {
		return nil
	}

	confidence := float64(max(longScore, shortScore)) / 5.0

	return &Signal{
		Type:       signalType,
		Symbol:     s.symbol,
		Price:      price,
		StopLoss:   stopLoss,
		TakeProfit: takeProfit,
		Confidence: confidence,
		Reason: fmt.Sprintf("ADX: %.1f, RSI: %.1f, Momentum: %.2f%%, Score: %d/5",
			adx, rsi, momentum, max(longScore, shortScore)),
	}
}

func (s *ETFMomentumStrategy) isUptrend(candles []exchange.Candle) bool {
	if len(candles) < 10 {
		return false
	}

	recent := candles[len(candles)-10:]

	// Check if price is above 10-period average (simple trend confirmation)
	var sum float64
	for i := 0; i < 10; i++ {
		sum += recent[i].Close
	}
	avg10 := sum / 10

	return recent[9].Close > avg10
}

func (s *ETFMomentumStrategy) isDowntrend(candles []exchange.Candle) bool {
	if len(candles) < 10 {
		return false
	}

	recent := candles[len(candles)-10:]

	// Check if price is below 10-period average (simple trend confirmation)
	var sum float64
	for i := 0; i < 10; i++ {
		sum += recent[i].Close
	}
	avg10 := sum / 10

	return recent[9].Close < avg10
}

func (s *ETFMomentumStrategy) calculateAvgVolume(candles []exchange.Candle, period int) float64 {
	if len(candles) < period+1 {
		return 1
	}

	sum := 0.0
	for i := len(candles) - period - 1; i < len(candles)-1; i++ {
		sum += candles[i].Volume
	}
	return sum / float64(period)
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
func (s *ETFMomentumStrategy) CalculateATR(candles []exchange.Candle) float64 {
	atr := indicator.ATR(candles, s.config.ATRPeriod)
	if len(atr) > 0 {
		return atr[len(atr)-1]
	}
	return 0
}

// AnalyzeWithDailyTrend uses daily candles for trend and hourly for entry
func (s *ETFMomentumStrategy) AnalyzeWithDailyTrend(dailyCandles, hourlyCandles []exchange.Candle) (*Signal, error) {
	if len(dailyCandles) < 50 || len(hourlyCandles) < s.MinimumCandles() {
		return nil, ErrInsufficientData
	}

	// Get daily trend direction
	dailyFastEMA := indicator.EMA(dailyCandles, s.config.FastEMA)
	dailySlowEMA := indicator.EMA(dailyCandles, s.config.SlowEMA)
	dailyADX := indicator.ADX(dailyCandles, s.config.ADXPeriod)

	n := len(dailyCandles) - 1
	dailyTrendUp := dailyFastEMA[n] > dailySlowEMA[n]
	dailyTrendDown := dailyFastEMA[n] < dailySlowEMA[n]

	// Check daily ADX (use lower threshold for daily - ETFs trend slowly)
	var lastDailyADX float64
	if len(dailyADX) > 0 {
		lastDailyADX = dailyADX[len(dailyADX)-1]
	}
	// Use half of MinADX for daily check (e.g., 6 instead of 12)
	if lastDailyADX < s.config.MinADX*0.5 {
		return nil, ErrNoSignal
	}

	// Get hourly signal
	signal, err := s.Analyze(hourlyCandles)
	if err != nil {
		return nil, err
	}

	// Filter: only trade in direction of daily trend
	if signal.Type == SignalBuy && !dailyTrendUp {
		return nil, ErrNoSignal
	}
	if signal.Type == SignalSell && !dailyTrendDown {
		return nil, ErrNoSignal
	}

	// Update reason with daily context
	signal.Reason = fmt.Sprintf("[Daily %s] %s",
		map[bool]string{true: "BULLISH", false: "BEARISH"}[dailyTrendUp],
		signal.Reason)

	return signal, nil
}

// Helper to calculate absolute value
func abs(x float64) float64 {
	return math.Abs(x)
}
