package handlers

import (
	"encoding/json"
	"net/http"
	"time"

	"final-bot-trader-api/internal/livetrading"

	"github.com/go-chi/chi/v5"
)

// BotHandler handles trading bot specific endpoints
type BotHandler struct {
	engine *livetrading.Engine
}

// NewBotHandler creates a new bot handler
func NewBotHandler(engine *livetrading.Engine) *BotHandler {
	return &BotHandler{engine: engine}
}

// GetBotStatus returns the current bot status
func (h *BotHandler) GetBotStatus(w http.ResponseWriter, r *http.Request) {
	if h.engine == nil {
		JSON(w, http.StatusOK, map[string]interface{}{
			"error":     "Bot engine not initialized",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	status := h.engine.GetStatus()
	status["timestamp"] = time.Now().Format(time.RFC3339)

	JSON(w, http.StatusOK, status)
}

// GetBotPositions returns open positions from the bot
func (h *BotHandler) GetBotPositions(w http.ResponseWriter, r *http.Request) {
	if h.engine == nil {
		JSON(w, http.StatusOK, map[string]interface{}{
			"error":     "Bot engine not initialized",
			"positions": []interface{}{},
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	trades := h.engine.GetOpenTrades()

	JSON(w, http.StatusOK, map[string]interface{}{
		"count":     len(trades),
		"positions": trades,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// GetBotTrades returns trade statistics
func (h *BotHandler) GetBotTrades(w http.ResponseWriter, r *http.Request) {
	if h.engine == nil {
		JSON(w, http.StatusOK, map[string]interface{}{
			"error":     "Bot engine not initialized",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	status := h.engine.GetStatus()

	JSON(w, http.StatusOK, map[string]interface{}{
		"open_positions": status["open_positions"],
		"closed_trades":  status["closed_trades"],
		"daily_trades":   status["daily_trades"],
		"total_pnl":      status["total_pnl"],
		"daily_pnl":      status["daily_pnl"],
		"win_count":      status["win_count"],
		"loss_count":     status["loss_count"],
		"win_rate":       status["win_rate"],
		"timestamp":      time.Now().Format(time.RFC3339),
	})
}

// GetBotConfig returns current bot configuration
func (h *BotHandler) GetBotConfig(w http.ResponseWriter, r *http.Request) {
	if h.engine == nil {
		JSON(w, http.StatusOK, map[string]interface{}{
			"error":     "Bot engine not initialized",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	status := h.engine.GetStatus()

	JSON(w, http.StatusOK, map[string]interface{}{
		"position_size": status["position_size"],
		"leverage":      status["leverage"],
		"dry_run":       status["dry_run"],
		"running":       status["running"],
		"start_time":    status["start_time"],
		"last_update":   status["last_update"],
		"timestamp":     time.Now().Format(time.RFC3339),
	})
}

// GetCircuitBreakerStatus returns the circuit breaker status
func (h *BotHandler) GetCircuitBreakerStatus(w http.ResponseWriter, r *http.Request) {
	if h.engine == nil {
		JSON(w, http.StatusOK, map[string]interface{}{
			"error":     "Bot engine not initialized",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	cb := h.engine.GetCircuitBreaker()
	if cb == nil {
		JSON(w, http.StatusOK, map[string]interface{}{
			"error":     "Circuit breaker not initialized",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	status := cb.GetStatus()
	status["timestamp"] = time.Now().Format(time.RFC3339)

	JSON(w, http.StatusOK, status)
}

// ResetCircuitBreaker resets the circuit breaker to normal state
func (h *BotHandler) ResetCircuitBreaker(w http.ResponseWriter, r *http.Request) {
	if h.engine == nil {
		JSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":     "Bot engine not initialized",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	h.engine.ResetCircuitBreaker()

	JSON(w, http.StatusOK, map[string]interface{}{
		"message":   "Circuit breaker reset to normal state",
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// GetTradesHistory returns all trades history
func (h *BotHandler) GetTradesHistory(w http.ResponseWriter, r *http.Request) {
	if h.engine == nil {
		JSON(w, http.StatusOK, map[string]interface{}{
			"error":  "Bot engine not initialized",
			"trades": []interface{}{},
		})
		return
	}

	trades := h.engine.GetAllTrades()

	JSON(w, http.StatusOK, map[string]interface{}{
		"count":     len(trades),
		"trades":    trades,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// GetClosedTrades returns only closed trades
func (h *BotHandler) GetClosedTrades(w http.ResponseWriter, r *http.Request) {
	if h.engine == nil {
		JSON(w, http.StatusOK, map[string]interface{}{
			"error":  "Bot engine not initialized",
			"trades": []interface{}{},
		})
		return
	}

	trades := h.engine.GetClosedTrades()

	JSON(w, http.StatusOK, map[string]interface{}{
		"count":     len(trades),
		"trades":    trades,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// GetSymbolStats returns stats grouped by symbol
func (h *BotHandler) GetSymbolStats(w http.ResponseWriter, r *http.Request) {
	if h.engine == nil {
		JSON(w, http.StatusOK, map[string]interface{}{
			"error":   "Bot engine not initialized",
			"symbols": map[string]interface{}{},
		})
		return
	}

	stats := h.engine.GetTradesBySymbol()

	JSON(w, http.StatusOK, map[string]interface{}{
		"symbols":   stats,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// GetEquityCurve returns equity curve data
func (h *BotHandler) GetEquityCurve(w http.ResponseWriter, r *http.Request) {
	if h.engine == nil {
		JSON(w, http.StatusOK, map[string]interface{}{
			"error": "Bot engine not initialized",
			"curve": []interface{}{},
		})
		return
	}

	curve := h.engine.GetEquityCurve()

	JSON(w, http.StatusOK, map[string]interface{}{
		"count":     len(curve),
		"curve":     curve,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// ClosePositionFictitious closes a position only in the application/database without calling Bitunix
func (h *BotHandler) ClosePositionFictitious(w http.ResponseWriter, r *http.Request) {
	if h.engine == nil {
		JSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":     "Bot engine not initialized",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// Get trade ID from URL path parameter
	tradeID := chi.URLParam(r, "tradeID")
	if tradeID == "" {
		JSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":     "Trade ID is required",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// Close the trade fictitiously
	if err := h.engine.CloseTradeFictitious(r.Context(), tradeID); err != nil {
		JSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":     err.Error(),
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"message":   "Position closed fictitiously",
		"trade_id":  tradeID,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}

// UpdateTrade updates a closed trade's prices and recalculates PnL
func (h *BotHandler) UpdateTrade(w http.ResponseWriter, r *http.Request) {
	if h.engine == nil {
		JSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":     "Bot engine not initialized",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// Get trade ID from URL path parameter
	tradeID := chi.URLParam(r, "tradeID")
	if tradeID == "" {
		JSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":     "Trade ID is required",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// Parse request body
	var req struct {
		EntryPrice *float64 `json:"entry_price,omitempty"`
		ExitPrice  *float64 `json:"exit_price,omitempty"`
	}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		JSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":     "Invalid request body: " + err.Error(),
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	if req.EntryPrice == nil && req.ExitPrice == nil {
		JSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":     "At least one of entry_price or exit_price must be provided",
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	// Update the trade
	if err := h.engine.UpdateTradeClosed(r.Context(), tradeID, req.EntryPrice, req.ExitPrice); err != nil {
		JSON(w, http.StatusBadRequest, map[string]interface{}{
			"error":     err.Error(),
			"timestamp": time.Now().Format(time.RFC3339),
		})
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"message":   "Trade updated successfully",
		"trade_id":  tradeID,
		"timestamp": time.Now().Format(time.RFC3339),
	})
}
