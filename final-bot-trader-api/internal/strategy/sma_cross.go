package strategy

import (
	"fmt"

	"final-bot-trader-api/internal/exchange/model"
	"final-bot-trader-api/internal/indicator"
)

// SMACrossover implements a simple SMA crossover strategy
// Buy when fast SMA crosses above slow SMA
// Sell when fast SMA crosses below slow SMA
type SMACrossover struct {
	BaseStrategy
	fastPeriod int
	slowPeriod int
	fastSMA    *indicator.SMA
	slowSMA    *indicator.SMA
}

// NewSMACrossover creates a new SMA crossover strategy
func NewSMACrossover(symbol string, fastPeriod, slowPeriod int) (*SMACrossover, error) {
	if fastPeriod <= 0 || slowPeriod <= 0 {
		return nil, ErrInvalidParameters
	}
	if fastPeriod >= slowPeriod {
		return nil, fmt.Errorf("fast period (%d) must be less than slow period (%d)", fastPeriod, slowPeriod)
	}

	fastSMA, err := indicator.NewSMA(fastPeriod)
	if err != nil {
		return nil, err
	}

	slowSMA, err := indicator.NewSMA(slowPeriod)
	if err != nil {
		return nil, err
	}

	return &SMACrossover{
		BaseStrategy: NewBaseStrategy(symbol),
		fastPeriod:   fastPeriod,
		slowPeriod:   slowPeriod,
		fastSMA:      fastSMA,
		slowSMA:      slowSMA,
	}, nil
}

// Name returns the strategy name
func (s *SMACrossover) Name() string {
	return fmt.Sprintf("SMA_Crossover(%d,%d)", s.fastPeriod, s.slowPeriod)
}

// Description returns the strategy description
func (s *SMACrossover) Description() string {
	return fmt.Sprintf("SMA Crossover strategy: Buy when SMA(%d) crosses above SMA(%d), Sell when it crosses below",
		s.fastPeriod, s.slowPeriod)
}

// MinimumCandles returns the minimum candles required
func (s *SMACrossover) MinimumCandles() int {
	return s.slowPeriod + 1 // Need at least slowPeriod + 1 for crossover detection
}

// Parameters returns the strategy parameters
func (s *SMACrossover) Parameters() map[string]interface{} {
	return map[string]interface{}{
		"fast_period": s.fastPeriod,
		"slow_period": s.slowPeriod,
	}
}

// SetParameters updates the strategy parameters
func (s *SMACrossover) SetParameters(params map[string]interface{}) error {
	if fast, ok := params["fast_period"].(int); ok {
		if fast <= 0 {
			return ErrInvalidParameters
		}
		s.fastPeriod = fast
	}

	if slow, ok := params["slow_period"].(int); ok {
		if slow <= 0 {
			return ErrInvalidParameters
		}
		s.slowPeriod = slow
	}

	if s.fastPeriod >= s.slowPeriod {
		return ErrInvalidParameters
	}

	// Recreate indicators with new periods
	var err error
	s.fastSMA, err = indicator.NewSMA(s.fastPeriod)
	if err != nil {
		return err
	}
	s.slowSMA, err = indicator.NewSMA(s.slowPeriod)
	if err != nil {
		return err
	}

	return nil
}

// Analyze analyzes the candles and returns a trading signal
func (s *SMACrossover) Analyze(candles []model.Candle) (*Signal, error) {
	if len(candles) < s.MinimumCandles() {
		return nil, ErrInsufficientData
	}

	// Calculate fast and slow SMAs
	fastValues, err := s.fastSMA.Calculate(candles)
	if err != nil {
		return nil, err
	}

	slowValues, err := s.slowSMA.Calculate(candles)
	if err != nil {
		return nil, err
	}

	// Align the arrays (slow SMA is shorter)
	offset := len(fastValues) - len(slowValues)
	alignedFast := fastValues[offset:]

	if len(alignedFast) < 2 || len(slowValues) < 2 {
		return nil, ErrInsufficientData
	}

	// Get current and previous values
	currentFast := alignedFast[len(alignedFast)-1]
	previousFast := alignedFast[len(alignedFast)-2]
	currentSlow := slowValues[len(slowValues)-1]
	previousSlow := slowValues[len(slowValues)-2]

	currentPrice := candles[len(candles)-1].Close

	// Detect crossovers
	// Golden Cross: fast crosses above slow -> BUY
	if previousFast <= previousSlow && currentFast > currentSlow {
		signal := s.CreateSignal(SignalBuy, currentPrice,
			fmt.Sprintf("Golden Cross: SMA(%d)=%.2f crossed above SMA(%d)=%.2f",
				s.fastPeriod, currentFast, s.slowPeriod, currentSlow))
		signal.Indicators["fast_sma"] = currentFast
		signal.Indicators["slow_sma"] = currentSlow
		signal.Confidence = calculateCrossoverConfidence(currentFast, currentSlow, previousFast, previousSlow)
		return signal, nil
	}

	// Death Cross: fast crosses below slow -> SELL
	if previousFast >= previousSlow && currentFast < currentSlow {
		signal := s.CreateSignal(SignalSell, currentPrice,
			fmt.Sprintf("Death Cross: SMA(%d)=%.2f crossed below SMA(%d)=%.2f",
				s.fastPeriod, currentFast, s.slowPeriod, currentSlow))
		signal.Indicators["fast_sma"] = currentFast
		signal.Indicators["slow_sma"] = currentSlow
		signal.Confidence = calculateCrossoverConfidence(currentFast, currentSlow, previousFast, previousSlow)
		return signal, nil
	}

	return nil, ErrNoSignal
}

// calculateCrossoverConfidence calculates confidence based on crossover strength
func calculateCrossoverConfidence(currentFast, currentSlow, previousFast, previousSlow float64) float64 {
	// Calculate the angle/strength of the crossover
	currentDiff := (currentFast - currentSlow) / currentSlow * 100
	previousDiff := (previousFast - previousSlow) / previousSlow * 100

	// Stronger crossovers have more confidence
	crossoverStrength := abs(currentDiff - previousDiff)

	// Normalize to 0-1 range (assuming max strength of 2%)
	confidence := crossoverStrength / 2.0
	if confidence > 1.0 {
		confidence = 1.0
	}
	if confidence < 0.3 {
		confidence = 0.3 // Minimum confidence
	}

	return confidence
}

func abs(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}
