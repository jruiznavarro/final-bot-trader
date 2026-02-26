package livetrading

import (
	"fmt"
	"log"
	"sync"
	"time"

	"final-bot-trader-api/internal/telegram"
)

// TradingCircuitBreakerConfig holds configuration for the trading circuit breaker
type TradingCircuitBreakerConfig struct {
	// Consecutive losses trigger
	MaxConsecutiveLosses int // Pause after N consecutive losses

	// Drawdown triggers
	MaxDailyDrawdownPct  float64 // Max daily loss as % of initial balance
	MaxTotalDrawdownPct  float64 // Max total loss as % of initial balance

	// Cooldown periods
	ConsecutiveLossCooldown time.Duration // Cooldown after consecutive losses
	DrawdownCooldown        time.Duration // Cooldown after drawdown trigger

	// Recovery settings
	ReducedSizeOnRecovery float64 // Position size multiplier on recovery (e.g., 0.5 = 50%)
	GradualRecoveryTrades int     // Number of trades with reduced size before full recovery
}

// DefaultTradingCircuitBreakerConfig returns sensible defaults
func DefaultTradingCircuitBreakerConfig() TradingCircuitBreakerConfig {
	return TradingCircuitBreakerConfig{
		MaxConsecutiveLosses:    3,                  // Pause after 3 consecutive losses
		MaxDailyDrawdownPct:     5.0,                // Pause if daily loss > 5%
		MaxTotalDrawdownPct:     10.0,               // Pause if total loss > 10%
		ConsecutiveLossCooldown: 3 * time.Hour,      // 3 hour cooldown after consecutive losses (was 2h)
		DrawdownCooldown:        4 * time.Hour,      // 4 hour cooldown after drawdown
		ReducedSizeOnRecovery:   0.5,                // 50% position size on recovery
		GradualRecoveryTrades:   3,                  // 3 trades at reduced size
	}
}

// TradingCircuitBreakerState represents the current state
type TradingCircuitBreakerState int

const (
	TradingStateNormal TradingCircuitBreakerState = iota
	TradingStatePaused
	TradingStateRecovery
)

func (s TradingCircuitBreakerState) String() string {
	switch s {
	case TradingStateNormal:
		return "NORMAL"
	case TradingStatePaused:
		return "PAUSED"
	case TradingStateRecovery:
		return "RECOVERY"
	default:
		return "UNKNOWN"
	}
}

// TradingCircuitBreaker manages trading risk limits
type TradingCircuitBreaker struct {
	config   TradingCircuitBreakerConfig
	telegram *telegram.Client

	// State tracking
	state              TradingCircuitBreakerState
	pauseReason        string
	pauseStartTime     time.Time
	cooldownEndTime    time.Time

	// Loss tracking
	consecutiveLosses  int
	dailyPnL           float64
	totalPnL           float64
	initialBalance     float64
	lastDailyReset     time.Time

	// Recovery tracking
	recoveryTradesCount int

	mu sync.RWMutex
}

// NewTradingCircuitBreaker creates a new trading circuit breaker
func NewTradingCircuitBreaker(config TradingCircuitBreakerConfig, tg *telegram.Client, initialBalance float64) *TradingCircuitBreaker {
	return &TradingCircuitBreaker{
		config:         config,
		telegram:       tg,
		state:          TradingStateNormal,
		initialBalance: initialBalance,
		lastDailyReset: time.Now().Truncate(24 * time.Hour),
	}
}

// CanTrade returns true if trading is allowed, false if paused
func (tcb *TradingCircuitBreaker) CanTrade() bool {
	tcb.mu.Lock()
	defer tcb.mu.Unlock()

	// Check if cooldown has expired
	if tcb.state == TradingStatePaused {
		if time.Now().After(tcb.cooldownEndTime) {
			tcb.transitionToRecovery()
		} else {
			return false
		}
	}

	return true
}

// GetPositionSizeMultiplier returns the multiplier for position size
// Returns 1.0 for normal, reduced value for recovery mode
func (tcb *TradingCircuitBreaker) GetPositionSizeMultiplier() float64 {
	tcb.mu.RLock()
	defer tcb.mu.RUnlock()

	if tcb.state == TradingStateRecovery {
		return tcb.config.ReducedSizeOnRecovery
	}
	return 1.0
}

// RecordTrade records the result of a trade
func (tcb *TradingCircuitBreaker) RecordTrade(pnl float64) {
	tcb.mu.Lock()
	defer tcb.mu.Unlock()

	// Check for daily reset
	today := time.Now().Truncate(24 * time.Hour)
	if today.After(tcb.lastDailyReset) {
		tcb.dailyPnL = 0
		tcb.lastDailyReset = today
		log.Println("[CircuitBreaker] Daily PnL reset")
	}

	// Update PnL
	tcb.dailyPnL += pnl
	tcb.totalPnL += pnl

	// Track consecutive losses
	if pnl < 0 {
		tcb.consecutiveLosses++
		log.Printf("[CircuitBreaker] Loss recorded: %.4f USDT (consecutive: %d)", pnl, tcb.consecutiveLosses)
	} else {
		tcb.consecutiveLosses = 0
		log.Printf("[CircuitBreaker] Win recorded: %.4f USDT (streak reset)", pnl)

		// Check recovery completion
		if tcb.state == TradingStateRecovery {
			tcb.recoveryTradesCount++
			if tcb.recoveryTradesCount >= tcb.config.GradualRecoveryTrades {
				tcb.transitionToNormal()
			}
		}
	}

	// Check triggers
	tcb.checkTriggers()
}

// checkTriggers checks all circuit breaker triggers
func (tcb *TradingCircuitBreaker) checkTriggers() {
	// Only check if in normal or recovery state
	if tcb.state == TradingStatePaused {
		return
	}

	// Check consecutive losses
	if tcb.consecutiveLosses >= tcb.config.MaxConsecutiveLosses {
		tcb.triggerPause(
			fmt.Sprintf("%d consecutive losses", tcb.consecutiveLosses),
			tcb.config.ConsecutiveLossCooldown,
		)
		return
	}

	// Check daily drawdown
	if tcb.initialBalance > 0 {
		dailyDrawdownPct := (-tcb.dailyPnL / tcb.initialBalance) * 100
		if dailyDrawdownPct > tcb.config.MaxDailyDrawdownPct {
			tcb.triggerPause(
				fmt.Sprintf("Daily drawdown %.1f%% > %.1f%%", dailyDrawdownPct, tcb.config.MaxDailyDrawdownPct),
				tcb.config.DrawdownCooldown,
			)
			return
		}

		// Check total drawdown
		totalDrawdownPct := (-tcb.totalPnL / tcb.initialBalance) * 100
		if totalDrawdownPct > tcb.config.MaxTotalDrawdownPct {
			tcb.triggerPause(
				fmt.Sprintf("Total drawdown %.1f%% > %.1f%%", totalDrawdownPct, tcb.config.MaxTotalDrawdownPct),
				tcb.config.DrawdownCooldown,
			)
			return
		}
	}
}

// triggerPause pauses trading
func (tcb *TradingCircuitBreaker) triggerPause(reason string, cooldown time.Duration) {
	tcb.state = TradingStatePaused
	tcb.pauseReason = reason
	tcb.pauseStartTime = time.Now()
	tcb.cooldownEndTime = time.Now().Add(cooldown)

	log.Printf("[CircuitBreaker] 🛑 TRADING PAUSED: %s (cooldown: %v)", reason, cooldown)

	// Send Telegram notification
	if tcb.telegram != nil && tcb.telegram.IsConfigured() {
		msg := fmt.Sprintf(`⚠️ <b>CIRCUIT BREAKER ACTIVADO</b>

🛑 <b>Trading Pausado</b>
━━━━━━━━━━━━━━━━━━━━━

📍 <b>Razón:</b> %s
⏱ <b>Cooldown:</b> %v
🕐 <b>Reanuda:</b> %s

📊 <b>Estado actual:</b>
• PnL Hoy: <code>%+.4f USDT</code>
• PnL Total: <code>%+.4f USDT</code>
• Pérdidas seguidas: <code>%d</code>

<i>El bot se reanudará automáticamente después del cooldown.</i>`,
			reason,
			cooldown,
			tcb.cooldownEndTime.Format("15:04:05"),
			tcb.dailyPnL,
			tcb.totalPnL,
			tcb.consecutiveLosses,
		)
		tcb.telegram.SendMessage(msg)
	}
}

// transitionToRecovery transitions to recovery mode
func (tcb *TradingCircuitBreaker) transitionToRecovery() {
	tcb.state = TradingStateRecovery
	tcb.recoveryTradesCount = 0
	tcb.consecutiveLosses = 0

	log.Printf("[CircuitBreaker] 🔄 Entering RECOVERY mode (%.0f%% position size for %d trades)",
		tcb.config.ReducedSizeOnRecovery*100, tcb.config.GradualRecoveryTrades)

	if tcb.telegram != nil && tcb.telegram.IsConfigured() {
		msg := fmt.Sprintf(`🔄 <b>MODO RECUPERACIÓN</b>

Trading reanudado con precaución:
• Tamaño posición: <code>%.0f%%</code>
• Trades hasta normalizar: <code>%d</code>

<i>El tamaño de posición volverá al 100%% después de %d trades exitosos.</i>`,
			tcb.config.ReducedSizeOnRecovery*100,
			tcb.config.GradualRecoveryTrades,
			tcb.config.GradualRecoveryTrades,
		)
		tcb.telegram.SendMessage(msg)
	}
}

// transitionToNormal transitions back to normal mode
func (tcb *TradingCircuitBreaker) transitionToNormal() {
	tcb.state = TradingStateNormal
	tcb.recoveryTradesCount = 0

	log.Println("[CircuitBreaker] ✅ Back to NORMAL trading mode")

	if tcb.telegram != nil && tcb.telegram.IsConfigured() {
		msg := `✅ <b>MODO NORMAL</b>

Trading reanudado al 100% de capacidad.

<i>Sistema estabilizado.</i>`
		tcb.telegram.SendMessage(msg)
	}
}

// GetStatus returns the current status
func (tcb *TradingCircuitBreaker) GetStatus() map[string]interface{} {
	tcb.mu.RLock()
	defer tcb.mu.RUnlock()

	status := map[string]interface{}{
		"state":              tcb.state.String(),
		"can_trade":          tcb.state != TradingStatePaused,
		"position_multiplier": tcb.GetPositionSizeMultiplierLocked(),
		"consecutive_losses": tcb.consecutiveLosses,
		"daily_pnl":          tcb.dailyPnL,
		"total_pnl":          tcb.totalPnL,
	}

	if tcb.state == TradingStatePaused {
		status["pause_reason"] = tcb.pauseReason
		status["cooldown_remaining"] = time.Until(tcb.cooldownEndTime).String()
	}

	if tcb.state == TradingStateRecovery {
		status["recovery_trades_remaining"] = tcb.config.GradualRecoveryTrades - tcb.recoveryTradesCount
	}

	return status
}

// GetPositionSizeMultiplierLocked returns multiplier without locking (must be called with lock held)
func (tcb *TradingCircuitBreaker) GetPositionSizeMultiplierLocked() float64 {
	if tcb.state == TradingStateRecovery {
		return tcb.config.ReducedSizeOnRecovery
	}
	return 1.0
}

// Reset manually resets the circuit breaker
func (tcb *TradingCircuitBreaker) Reset() {
	tcb.mu.Lock()
	defer tcb.mu.Unlock()

	tcb.state = TradingStateNormal
	tcb.consecutiveLosses = 0
	tcb.recoveryTradesCount = 0

	log.Println("[CircuitBreaker] Manually reset to NORMAL state")
}

// SetInitialBalance updates the initial balance for drawdown calculations
func (tcb *TradingCircuitBreaker) SetInitialBalance(balance float64) {
	tcb.mu.Lock()
	defer tcb.mu.Unlock()
	tcb.initialBalance = balance
}
