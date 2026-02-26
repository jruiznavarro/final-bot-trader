package strategy

import (
	"final-bot-trader-api/internal/exchange/model"
	"final-bot-trader-api/internal/indicator"
)

// RiskManager handles position sizing and risk calculations
type RiskManager struct {
	// Risk parameters
	MaxRiskPerTrade   float64 // Maximum risk per trade as percentage of balance (e.g., 0.02 = 2%)
	DefaultSLPercent  float64 // Default stop loss percentage if not calculated
	DefaultTPPercent  float64 // Default take profit percentage if not calculated
	RiskRewardRatio   float64 // Minimum risk/reward ratio (e.g., 2.0 = 1:2)
	ATRMultiplierSL   float64 // ATR multiplier for stop loss
	ATRMultiplierTP   float64 // ATR multiplier for take profit
	MaxPositionSize   float64 // Maximum position size as percentage of balance
	MaxSLPercent      float64 // Maximum SL distance as % of entry price
	atr               *indicator.ATR
}

// RiskConfig holds configuration for risk management
type RiskConfig struct {
	MaxRiskPerTrade   float64
	DefaultSLPercent  float64
	DefaultTPPercent  float64
	RiskRewardRatio   float64
	ATRMultiplierSL   float64
	ATRMultiplierTP   float64
	MaxPositionSize   float64
	MaxSLPercent      float64 // Maximum SL distance as % of entry price (e.g., 0.015 = 1.5%)
	ATRPeriod         int
}

// DefaultRiskConfig returns default risk management settings
func DefaultRiskConfig() RiskConfig {
	return RiskConfig{
		MaxRiskPerTrade:   0.02,   // 2% risk per trade
		DefaultSLPercent:  0.015,  // 1.5% stop loss (default fallback)
		DefaultTPPercent:  0.04,   // 4% take profit
		RiskRewardRatio:   2.0,    // 1:2 risk/reward
		ATRMultiplierSL:   1.5,    // 1.5x ATR for SL
		ATRMultiplierTP:   3.0,    // 3x ATR for TP
		MaxPositionSize:   0.10,   // 10% max position
		MaxSLPercent:      0.015,  // Max 1.5% SL distance from entry
		ATRPeriod:         14,
	}
}

// NewRiskManager creates a new risk manager with given config
func NewRiskManager(config RiskConfig) (*RiskManager, error) {
	atr, err := indicator.NewATR(config.ATRPeriod)
	if err != nil {
		return nil, err
	}

	return &RiskManager{
		MaxRiskPerTrade:  config.MaxRiskPerTrade,
		DefaultSLPercent: config.DefaultSLPercent,
		DefaultTPPercent: config.DefaultTPPercent,
		RiskRewardRatio:  config.RiskRewardRatio,
		ATRMultiplierSL:  config.ATRMultiplierSL,
		ATRMultiplierTP:  config.ATRMultiplierTP,
		MaxPositionSize:  config.MaxPositionSize,
		MaxSLPercent:     config.MaxSLPercent,
		atr:              atr,
	}, nil
}

// CalculatePositionSize calculates the position size based on risk
func (rm *RiskManager) CalculatePositionSize(balance, entryPrice, stopLoss float64) float64 {
	if entryPrice <= 0 || stopLoss <= 0 {
		return 0
	}

	// Calculate risk per unit
	riskPerUnit := abs(entryPrice - stopLoss)
	if riskPerUnit == 0 {
		return 0
	}

	// Maximum risk amount
	maxRiskAmount := balance * rm.MaxRiskPerTrade

	// Position size based on risk
	positionSize := maxRiskAmount / riskPerUnit

	// Apply maximum position size limit
	maxPosition := (balance * rm.MaxPositionSize) / entryPrice
	if positionSize > maxPosition {
		positionSize = maxPosition
	}

	return positionSize
}

// CalculateStopLoss calculates stop loss price based on ATR, capped at MaxSLPercent
func (rm *RiskManager) CalculateStopLoss(candles []model.Candle, entryPrice float64, isLong bool) (float64, error) {
	atrValue, err := rm.atr.Value(candles)
	if err != nil {
		// Fallback to percentage-based SL
		if isLong {
			return entryPrice * (1 - rm.DefaultSLPercent), nil
		}
		return entryPrice * (1 + rm.DefaultSLPercent), nil
	}

	distance := atrValue * rm.ATRMultiplierSL

	// Cap SL distance to MaxSLPercent of entry price
	if rm.MaxSLPercent > 0 {
		maxDistance := entryPrice * rm.MaxSLPercent
		if distance > maxDistance {
			distance = maxDistance
		}
	}

	if isLong {
		return entryPrice - distance, nil
	}
	return entryPrice + distance, nil
}

// CalculateTakeProfit calculates take profit price based on ATR
func (rm *RiskManager) CalculateTakeProfit(candles []model.Candle, entryPrice float64, isLong bool) (float64, error) {
	atrValue, err := rm.atr.Value(candles)
	if err != nil {
		// Fallback to percentage-based TP
		if isLong {
			return entryPrice * (1 + rm.DefaultTPPercent), nil
		}
		return entryPrice * (1 - rm.DefaultTPPercent), nil
	}

	distance := atrValue * rm.ATRMultiplierTP

	if isLong {
		return entryPrice + distance, nil
	}
	return entryPrice - distance, nil
}

// CalculateTPFromSL calculates TP based on SL and risk/reward ratio
func (rm *RiskManager) CalculateTPFromSL(entryPrice, stopLoss float64, isLong bool) float64 {
	slDistance := abs(entryPrice - stopLoss)
	tpDistance := slDistance * rm.RiskRewardRatio

	if isLong {
		return entryPrice + tpDistance
	}
	return entryPrice - tpDistance
}

// ApplyRiskManagement applies risk management to a signal
func (rm *RiskManager) ApplyRiskManagement(signal *Signal, candles []model.Candle, balance float64) error {
	if signal == nil || signal.Type == SignalNone {
		return nil
	}

	isLong := signal.Type == SignalBuy
	entryPrice := signal.Price

	// Calculate stop loss
	sl, err := rm.CalculateStopLoss(candles, entryPrice, isLong)
	if err != nil {
		return err
	}
	signal.SL = sl

	// Calculate take profit
	tp, err := rm.CalculateTakeProfit(candles, entryPrice, isLong)
	if err != nil {
		return err
	}
	signal.TP = tp

	// Ensure minimum risk/reward ratio
	slDistance := abs(entryPrice - sl)
	tpDistance := abs(tp - entryPrice)

	if slDistance > 0 && tpDistance/slDistance < rm.RiskRewardRatio {
		// Adjust TP to meet minimum R:R
		signal.TP = rm.CalculateTPFromSL(entryPrice, sl, isLong)
	}

	// Calculate position size
	signal.Quantity = rm.CalculatePositionSize(balance, entryPrice, sl)

	return nil
}

// ValidateTrade validates if a trade meets risk criteria
func (rm *RiskManager) ValidateTrade(entryPrice, stopLoss, takeProfit, balance float64) (bool, string) {
	// Check if prices are valid
	if entryPrice <= 0 || stopLoss <= 0 || takeProfit <= 0 {
		return false, "invalid price values"
	}

	// Calculate risk/reward ratio
	slDistance := abs(entryPrice - stopLoss)
	tpDistance := abs(takeProfit - entryPrice)

	if slDistance == 0 {
		return false, "stop loss too close to entry"
	}

	rr := tpDistance / slDistance
	if rr < rm.RiskRewardRatio {
		return false, "risk/reward ratio too low"
	}

	// Calculate position size
	positionSize := rm.CalculatePositionSize(balance, entryPrice, stopLoss)
	if positionSize <= 0 {
		return false, "calculated position size is zero"
	}

	// Calculate risk amount
	riskAmount := positionSize * slDistance
	riskPercent := riskAmount / balance

	if riskPercent > rm.MaxRiskPerTrade {
		return false, "risk exceeds maximum allowed"
	}

	return true, ""
}

// CalculateTrailingStop calculates a trailing stop based on ATR
func (rm *RiskManager) CalculateTrailingStop(candles []model.Candle, currentPrice float64, isLong bool) (float64, error) {
	return rm.atr.CalculateTrailingStop(candles, currentPrice, isLong, rm.ATRMultiplierSL)
}
