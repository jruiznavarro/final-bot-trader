package multifactor

import (
	"fmt"

	"final-bot-trader-api/internal/exchange/model"
	"final-bot-trader-api/internal/strategy"
)

// MTFConfig holds multi-timeframe strategy configuration
type MTFConfig struct {
	// Primary timeframe (higher) for trend direction
	PrimaryInterval string // e.g., "4h"

	// Entry timeframe (lower) for precise entries
	EntryInterval string // e.g., "1h"

	// Base strategy config
	StrategyConfig Config

	// MTF specific settings
	RequireTrendAlignment bool    // Require both TFs to agree on direction
	MinPrimaryADX         float64 // Minimum ADX on primary TF
}

// DefaultMTFConfig returns default multi-timeframe configuration
func DefaultMTFConfig() MTFConfig {
	return MTFConfig{
		PrimaryInterval:       "4h",
		EntryInterval:         "1h",
		StrategyConfig:        DefaultConfig(),
		RequireTrendAlignment: true,
		MinPrimaryADX:         28, // Aligned with strategy MinADX (was 20)
	}
}

// MTFStrategy implements multi-timeframe analysis
type MTFStrategy struct {
	config         MTFConfig
	primaryRegime  *RegimeDetector
	entryStrategy  *MultiFactorStrategy
	symbol         string
	lastPrimaryDir TrendDirection
}

// TrendDirection represents the primary trend direction
type TrendDirection int

const (
	TrendNeutral TrendDirection = iota
	TrendBullish
	TrendBearish
)

func (t TrendDirection) String() string {
	switch t {
	case TrendBullish:
		return "BULLISH"
	case TrendBearish:
		return "BEARISH"
	default:
		return "NEUTRAL"
	}
}

// NewMTFStrategy creates a new multi-timeframe strategy
func NewMTFStrategy(symbol string, config MTFConfig) *MTFStrategy {
	return &MTFStrategy{
		config:        config,
		primaryRegime: DefaultRegimeDetector(),
		entryStrategy: NewMultiFactorStrategy(symbol, config.StrategyConfig),
		symbol:        symbol,
	}
}

// Name returns the strategy name
func (s *MTFStrategy) Name() string {
	return fmt.Sprintf("MTF(%s+%s)", s.config.PrimaryInterval, s.config.EntryInterval)
}

// AnalyzeMTF analyzes both timeframes and generates a signal.
// primaryCandles: 4h candles for trend direction
// entryCandles:   1h candles for entry timing
// dailyCandles:   1d candles for macro RSI filter (nil/empty to skip)
//
// Daily RSI filter: if the daily RSI < 38, the market is oversold on the
// daily chart and a bounce is likely — SHORT entries are blocked to avoid
// being caught short at a local bottom during a recovery.
func (s *MTFStrategy) AnalyzeMTF(primaryCandles, entryCandles, dailyCandles []model.Candle) (*strategy.Signal, error) {
	if len(primaryCandles) < 50 {
		return nil, strategy.ErrInsufficientData
	}
	if len(entryCandles) < s.entryStrategy.MinimumCandles() {
		return nil, strategy.ErrInsufficientData
	}

	// Step 1: Analyze primary timeframe for trend direction
	primaryRegime, primaryADX, _ := s.primaryRegime.DetectRegime(primaryCandles)

	// Check minimum ADX on primary
	if primaryADX < s.config.MinPrimaryADX {
		return nil, strategy.ErrNoSignal
	}

	// Determine primary trend direction
	var primaryDir TrendDirection
	switch primaryRegime {
	case RegimeTrendingUp:
		primaryDir = TrendBullish
	case RegimeTrendingDown:
		primaryDir = TrendBearish
	default:
		primaryDir = TrendNeutral
	}

	s.lastPrimaryDir = primaryDir

	// If neutral on primary, don't trade
	if primaryDir == TrendNeutral && s.config.RequireTrendAlignment {
		return nil, strategy.ErrNoSignal
	}

	// Step 2: Get signal from entry timeframe
	entrySignal, err := s.entryStrategy.Analyze(entryCandles)
	if err != nil {
		return nil, err
	}

	// Step 3: Check alignment if required
	if s.config.RequireTrendAlignment {
		signalDir := TrendBullish
		if entrySignal.Type == strategy.SignalSell {
			signalDir = TrendBearish
		}

		// Only trade if entry signal aligns with primary trend
		if signalDir != primaryDir {
			return nil, strategy.ErrNoSignal
		}
	}

	// Step 4: Daily RSI filter — block entries against a macro extreme.
	// SHORTs blocked when daily RSI < 38: a deeply oversold daily chart means the
	// market is exhausted to the downside and prone to sharp bounces (root cause
	// of April 12-13 losses). LONGs blocked when daily RSI > 65 for the symmetric
	// reason: live results showed LONGs were the losing side (-4.74 USDT, 39% WR),
	// mostly entries chasing an already-extended move that then mean-reverted.
	if len(dailyCandles) >= 15 {
		dailyRSI := LastRSI(dailyCandles, 14)
		if entrySignal.Type == strategy.SignalSell && dailyRSI < 38 {
			return nil, strategy.ErrNoSignal
		}
		if entrySignal.Type == strategy.SignalBuy && dailyRSI > 65 {
			return nil, strategy.ErrNoSignal
		}
	}

	// Update signal reason with MTF context
	entrySignal.Reason = fmt.Sprintf("[%s %s] %s",
		s.config.PrimaryInterval,
		primaryDir.String(),
		entrySignal.Reason)

	return entrySignal, nil
}

// GetPrimaryTrend returns the current primary trend direction
func (s *MTFStrategy) GetPrimaryTrend() TrendDirection {
	return s.lastPrimaryDir
}

// GetPrimaryInterval returns the primary timeframe interval
func (s *MTFStrategy) GetPrimaryInterval() string {
	return s.config.PrimaryInterval
}

// GetEntryInterval returns the entry timeframe interval
func (s *MTFStrategy) GetEntryInterval() string {
	return s.config.EntryInterval
}

// AnalyzePrimaryOnly analyzes only the primary timeframe
// Useful for getting trend context without entry signal
func (s *MTFStrategy) AnalyzePrimaryOnly(primaryCandles []model.Candle) (TrendDirection, float64, error) {
	if len(primaryCandles) < 50 {
		return TrendNeutral, 0, strategy.ErrInsufficientData
	}

	regime, adx, _ := s.primaryRegime.DetectRegime(primaryCandles)

	var dir TrendDirection
	switch regime {
	case RegimeTrendingUp:
		dir = TrendBullish
	case RegimeTrendingDown:
		dir = TrendBearish
	default:
		dir = TrendNeutral
	}

	s.lastPrimaryDir = dir
	return dir, adx, nil
}
