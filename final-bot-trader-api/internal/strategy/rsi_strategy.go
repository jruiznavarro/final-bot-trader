package strategy

import (
	"fmt"

	"final-bot-trader-api/internal/exchange/model"
	"final-bot-trader-api/internal/indicator"
)

// RSIStrategy implements an RSI-based mean reversion strategy
// Buy when RSI is oversold and starts to rise
// Sell when RSI is overbought and starts to fall
type RSIStrategy struct {
	BaseStrategy
	period     int
	overbought float64
	oversold   float64
	rsi        *indicator.RSI
}

// NewRSIStrategy creates a new RSI strategy with default levels (70/30)
func NewRSIStrategy(symbol string, period int) (*RSIStrategy, error) {
	return NewRSIStrategyWithLevels(symbol, period, 70, 30)
}

// NewRSIStrategyWithLevels creates a new RSI strategy with custom levels
func NewRSIStrategyWithLevels(symbol string, period int, overbought, oversold float64) (*RSIStrategy, error) {
	if period <= 0 {
		return nil, ErrInvalidParameters
	}
	if overbought <= oversold || overbought > 100 || oversold < 0 {
		return nil, ErrInvalidParameters
	}

	rsi, err := indicator.NewRSIWithLevels(period, overbought, oversold)
	if err != nil {
		return nil, err
	}

	return &RSIStrategy{
		BaseStrategy: NewBaseStrategy(symbol),
		period:       period,
		overbought:   overbought,
		oversold:     oversold,
		rsi:          rsi,
	}, nil
}

// Name returns the strategy name
func (s *RSIStrategy) Name() string {
	return fmt.Sprintf("RSI_Strategy(%d,%.0f,%.0f)", s.period, s.overbought, s.oversold)
}

// Description returns the strategy description
func (s *RSIStrategy) Description() string {
	return fmt.Sprintf("RSI Mean Reversion: Buy when RSI(%d) exits oversold (<%0.f), Sell when exits overbought (>%.0f)",
		s.period, s.oversold, s.overbought)
}

// MinimumCandles returns the minimum candles required
func (s *RSIStrategy) MinimumCandles() int {
	return s.period + 2 // RSI needs period+1, plus one more for crossover detection
}

// Parameters returns the strategy parameters
func (s *RSIStrategy) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"period":     s.period,
		"overbought": s.overbought,
		"oversold":   s.oversold,
	}
}

// SetParameters updates the strategy parameters
func (s *RSIStrategy) SetParameters(params map[string]interface{}) error {
	if period, ok := params["period"].(int); ok {
		if period <= 0 {
			return ErrInvalidParameters
		}
		s.period = period
	}

	if overbought, ok := params["overbought"].(float64); ok {
		s.overbought = overbought
	}

	if oversold, ok := params["oversold"].(float64); ok {
		s.oversold = oversold
	}

	if s.overbought <= s.oversold {
		return ErrInvalidParameters
	}

	// Recreate RSI with new parameters
	var err error
	s.rsi, err = indicator.NewRSIWithLevels(s.period, s.overbought, s.oversold)
	return err
}

// Analyze analyzes the candles and returns a trading signal
func (s *RSIStrategy) Analyze(candles []model.Candle) (*Signal, error) {
	if len(candles) < s.MinimumCandles() {
		return nil, ErrInsufficientData
	}

	rsiValues, err := s.rsi.Calculate(candles)
	if err != nil {
		return nil, err
	}

	if len(rsiValues) < 2 {
		return nil, ErrInsufficientData
	}

	currentRSI := rsiValues[len(rsiValues)-1]
	previousRSI := rsiValues[len(rsiValues)-2]
	currentPrice := candles[len(candles)-1].Close

	// Buy signal: RSI exits oversold zone (crosses above oversold level)
	if previousRSI <= s.oversold && currentRSI > s.oversold {
		signal := s.CreateSignal(SignalBuy, currentPrice,
			fmt.Sprintf("RSI exiting oversold: %.2f -> %.2f (threshold: %.0f)",
				previousRSI, currentRSI, s.oversold))
		signal.Indicators["rsi"] = currentRSI
		signal.Indicators["previous_rsi"] = previousRSI
		signal.Confidence = calculateRSIConfidence(currentRSI, s.oversold, true)
		return signal, nil
	}

	// Sell signal: RSI exits overbought zone (crosses below overbought level)
	if previousRSI >= s.overbought && currentRSI < s.overbought {
		signal := s.CreateSignal(SignalSell, currentPrice,
			fmt.Sprintf("RSI exiting overbought: %.2f -> %.2f (threshold: %.0f)",
				previousRSI, currentRSI, s.overbought))
		signal.Indicators["rsi"] = currentRSI
		signal.Indicators["previous_rsi"] = previousRSI
		signal.Confidence = calculateRSIConfidence(currentRSI, s.overbought, false)
		return signal, nil
	}

	return nil, ErrNoSignal
}

// calculateRSIConfidence calculates confidence based on RSI extremity
func calculateRSIConfidence(currentRSI, threshold float64, isBuy bool) float64 {
	var distance float64
	if isBuy {
		// For buy signals, further from 50 (into oversold) is more confident
		distance = threshold - currentRSI
	} else {
		// For sell signals, further from 50 (into overbought) is more confident
		distance = currentRSI - threshold
	}

	// Normalize: 0-20 points from threshold maps to 0.3-1.0 confidence
	confidence := 0.3 + (abs(distance) / 20.0) * 0.7
	if confidence > 1.0 {
		confidence = 1.0
	}

	return confidence
}

// GetCurrentRSI returns the current RSI value
func (s *RSIStrategy) GetCurrentRSI(candles []model.Candle) (float64, error) {
	return s.rsi.Value(candles)
}

// IsOverbought checks if current RSI is in overbought territory
func (s *RSIStrategy) IsOverbought(candles []model.Candle) (bool, error) {
	rsi, err := s.GetCurrentRSI(candles)
	if err != nil {
		return false, err
	}
	return rsi >= s.overbought, nil
}

// IsOversold checks if current RSI is in oversold territory
func (s *RSIStrategy) IsOversold(candles []model.Candle) (bool, error) {
	rsi, err := s.GetCurrentRSI(candles)
	if err != nil {
		return false, err
	}
	return rsi <= s.oversold, nil
}
