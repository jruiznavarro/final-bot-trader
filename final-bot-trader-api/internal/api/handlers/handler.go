package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"final-bot-trader-api/internal/backtest"
	"final-bot-trader-api/internal/exchange/model"
	"final-bot-trader-api/internal/strategy"

	"github.com/go-chi/chi/v5"
)

// Handler holds all HTTP handlers and their dependencies
type Handler struct {
	// In a real implementation, this would hold the exchange client
	// exchangeClient exchange.Client
}

// NewHandler creates a new handler with dependencies
func NewHandler() *Handler {
	return &Handler{}
}

// HealthCheck returns the health status
func (h *Handler) HealthCheck(w http.ResponseWriter, r *http.Request) {
	JSON(w, http.StatusOK, map[string]interface{}{
		"status":    "healthy",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"version":   "1.0.0",
	})
}

// Welcome returns a welcome message
func (h *Handler) Welcome(w http.ResponseWriter, r *http.Request) {
	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "Welcome to Final Bot Trader API",
		"version": "1.0.0",
		"docs":    "/api/v1",
	})
}

// GetPrice returns the current price for a symbol
func (h *Handler) GetPrice(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol")
	if symbol == "" {
		BadRequest(w, "symbol is required")
		return
	}

	// TODO: Implement actual price fetching from exchange
	JSON(w, http.StatusOK, map[string]interface{}{
		"symbol":    symbol,
		"price":     0.0,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"message":   "Exchange client not configured - mock response",
	})
}

// GetKlines returns historical kline/candlestick data
func (h *Handler) GetKlines(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol")
	if symbol == "" {
		BadRequest(w, "symbol is required")
		return
	}

	interval := r.URL.Query().Get("interval")
	if interval == "" {
		interval = "1h"
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 100
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}

	// TODO: Implement actual kline fetching from exchange
	JSON(w, http.StatusOK, map[string]interface{}{
		"symbol":   symbol,
		"interval": interval,
		"limit":    limit,
		"candles":  []model.Candle{},
		"message":  "Exchange client not configured - mock response",
	})
}

// GetAccount returns account information
func (h *Handler) GetAccount(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement actual account info from exchange
	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "Exchange client not configured - mock response",
	})
}

// GetBalance returns account balance
func (h *Handler) GetBalance(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement actual balance from exchange
	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "Exchange client not configured - mock response",
	})
}

// GetPositions returns open positions
func (h *Handler) GetPositions(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement actual positions from exchange
	JSON(w, http.StatusOK, map[string]interface{}{
		"positions": []interface{}{},
		"message":   "Exchange client not configured - mock response",
	})
}

// GetOrders returns open orders
func (h *Handler) GetOrders(w http.ResponseWriter, r *http.Request) {
	// TODO: Implement actual orders from exchange
	JSON(w, http.StatusOK, map[string]interface{}{
		"orders":  []interface{}{},
		"message": "Exchange client not configured - mock response",
	})
}

// PlaceOrderRequest represents an order placement request
type PlaceOrderRequest struct {
	Symbol   string  `json:"symbol"`
	Side     string  `json:"side"`     // BUY or SELL
	Type     string  `json:"type"`     // MARKET or LIMIT
	Quantity float64 `json:"quantity"`
	Price    float64 `json:"price,omitempty"`    // Required for LIMIT orders
	StopLoss float64 `json:"stop_loss,omitempty"`
	TakeProfit float64 `json:"take_profit,omitempty"`
}

// PlaceOrder places a new order
func (h *Handler) PlaceOrder(w http.ResponseWriter, r *http.Request) {
	var req PlaceOrderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	if req.Symbol == "" || req.Side == "" || req.Type == "" || req.Quantity <= 0 {
		BadRequest(w, "symbol, side, type, and quantity are required")
		return
	}

	// TODO: Implement actual order placement
	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "Exchange client not configured - mock response",
		"order":   req,
	})
}

// CancelOrder cancels an order
func (h *Handler) CancelOrder(w http.ResponseWriter, r *http.Request) {
	orderID := chi.URLParam(r, "orderID")
	if orderID == "" {
		BadRequest(w, "orderID is required")
		return
	}

	// TODO: Implement actual order cancellation
	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "Exchange client not configured - mock response",
		"orderID": orderID,
	})
}

// ClosePosition closes a position
func (h *Handler) ClosePosition(w http.ResponseWriter, r *http.Request) {
	symbol := chi.URLParam(r, "symbol")
	if symbol == "" {
		BadRequest(w, "symbol is required")
		return
	}

	// TODO: Implement actual position closing
	JSON(w, http.StatusOK, map[string]interface{}{
		"message": "Exchange client not configured - mock response",
		"symbol":  symbol,
	})
}

// BacktestRequest represents a backtest request
type BacktestRequest struct {
	Strategy       string                 `json:"strategy"`
	Symbol         string                 `json:"symbol"`
	Interval       string                 `json:"interval"`
	StartDate      string                 `json:"start_date,omitempty"`
	EndDate        string                 `json:"end_date,omitempty"`
	InitialBalance float64                `json:"initial_balance"`
	Parameters     map[string]interface{} `json:"parameters,omitempty"`
}

// RunBacktest runs a backtest with the given configuration
func (h *Handler) RunBacktest(w http.ResponseWriter, r *http.Request) {
	var req BacktestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	if req.Strategy == "" {
		BadRequest(w, "strategy is required")
		return
	}

	if req.Symbol == "" {
		req.Symbol = "BTCUSDT"
	}

	if req.Interval == "" {
		req.Interval = "1h"
	}

	if req.InitialBalance <= 0 {
		req.InitialBalance = 10000
	}

	// Create strategy based on name
	var strat strategy.Strategy
	var err error

	switch req.Strategy {
	case "sma_crossover", "sma":
		shortPeriod := 10
		longPeriod := 20
		if p, ok := req.Parameters["short_period"].(float64); ok {
			shortPeriod = int(p)
		}
		if p, ok := req.Parameters["long_period"].(float64); ok {
			longPeriod = int(p)
		}
		strat, err = strategy.NewSMACrossover(req.Symbol, shortPeriod, longPeriod)

	case "rsi":
		period := 14
		overbought := 70.0
		oversold := 30.0
		if p, ok := req.Parameters["period"].(float64); ok {
			period = int(p)
		}
		if p, ok := req.Parameters["overbought"].(float64); ok {
			overbought = p
		}
		if p, ok := req.Parameters["oversold"].(float64); ok {
			oversold = p
		}
		strat, err = strategy.NewRSIStrategyWithLevels(req.Symbol, period, overbought, oversold)

	case "confluence":
		config := strategy.DefaultConfluenceConfig()
		strat, err = strategy.NewConfluenceStrategy(req.Symbol, config)

	case "adaptive":
		config := strategy.DefaultAdaptiveConfig()
		strat, err = strategy.NewAdaptiveStrategy(req.Symbol, config)

	default:
		BadRequest(w, "unknown strategy: "+req.Strategy)
		return
	}

	if err != nil {
		InternalError(w, "failed to create strategy: "+err.Error())
		return
	}

	// For now, return a mock result since we don't have historical data
	// In a real implementation, we would fetch candles from the exchange
	JSON(w, http.StatusOK, map[string]interface{}{
		"message":  "Backtest requires historical data - mock response",
		"strategy": strat.Name(),
		"config": map[string]interface{}{
			"symbol":          req.Symbol,
			"interval":        req.Interval,
			"initial_balance": req.InitialBalance,
			"parameters":      strat.Parameters(),
		},
	})
}

// ListStrategies returns available strategies
func (h *Handler) ListStrategies(w http.ResponseWriter, r *http.Request) {
	strategies := []map[string]interface{}{
		{
			"name":        "sma_crossover",
			"description": "Simple Moving Average Crossover strategy",
			"parameters": map[string]interface{}{
				"short_period": map[string]interface{}{
					"type":    "int",
					"default": 10,
					"min":     2,
					"max":     50,
				},
				"long_period": map[string]interface{}{
					"type":    "int",
					"default": 20,
					"min":     5,
					"max":     200,
				},
			},
		},
		{
			"name":        "rsi",
			"description": "RSI Overbought/Oversold strategy",
			"parameters": map[string]interface{}{
				"period": map[string]interface{}{
					"type":    "int",
					"default": 14,
					"min":     2,
					"max":     50,
				},
				"overbought": map[string]interface{}{
					"type":    "float",
					"default": 70,
					"min":     50,
					"max":     90,
				},
				"oversold": map[string]interface{}{
					"type":    "float",
					"default": 30,
					"min":     10,
					"max":     50,
				},
			},
		},
		{
			"name":        "confluence",
			"description": "Multi-indicator confluence strategy combining EMA, MACD, RSI, and Bollinger Bands",
			"parameters": map[string]interface{}{
				"min_confluence": map[string]interface{}{
					"type":    "float",
					"default": 70,
					"min":     50,
					"max":     95,
				},
			},
		},
		{
			"name":        "adaptive",
			"description": "Adaptive strategy that adjusts based on detected market regime",
			"parameters": map[string]interface{}{
				"ema_fast": map[string]interface{}{
					"type":    "int",
					"default": 21,
				},
				"ema_slow": map[string]interface{}{
					"type":    "int",
					"default": 55,
				},
			},
		},
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"strategies": strategies,
	})
}

// GetStrategy returns details about a specific strategy
func (h *Handler) GetStrategy(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		BadRequest(w, "strategy name is required")
		return
	}

	// Create a sample strategy instance to get details
	var strat strategy.Strategy
	var err error

	switch name {
	case "sma_crossover", "sma":
		strat, err = strategy.NewSMACrossover("SAMPLE", 10, 20)
	case "rsi":
		strat, err = strategy.NewRSIStrategy("SAMPLE", 14)
	case "confluence":
		strat, err = strategy.NewConfluenceStrategy("SAMPLE", strategy.DefaultConfluenceConfig())
	case "adaptive":
		strat, err = strategy.NewAdaptiveStrategy("SAMPLE", strategy.DefaultAdaptiveConfig())
	default:
		NotFound(w, "strategy not found: "+name)
		return
	}

	if err != nil {
		InternalError(w, "failed to create strategy: "+err.Error())
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"name":            strat.Name(),
		"description":     strat.Description(),
		"minimum_candles": strat.MinimumCandles(),
		"parameters":      strat.Parameters(),
	})
}

// AnalyzeRequest represents a strategy analysis request
type AnalyzeRequest struct {
	Candles []model.Candle         `json:"candles"`
	Parameters map[string]interface{} `json:"parameters,omitempty"`
}

// AnalyzeWithStrategy analyzes candles with a specific strategy
func (h *Handler) AnalyzeWithStrategy(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	if name == "" {
		BadRequest(w, "strategy name is required")
		return
	}

	var req AnalyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		BadRequest(w, "invalid request body")
		return
	}

	if len(req.Candles) == 0 {
		BadRequest(w, "candles are required")
		return
	}

	// Create strategy
	var strat strategy.Strategy
	var err error

	switch name {
	case "sma_crossover", "sma":
		strat, err = strategy.NewSMACrossover("ANALYSIS", 10, 20)
	case "rsi":
		strat, err = strategy.NewRSIStrategy("ANALYSIS", 14)
	case "confluence":
		strat, err = strategy.NewConfluenceStrategy("ANALYSIS", strategy.DefaultConfluenceConfig())
	case "adaptive":
		strat, err = strategy.NewAdaptiveStrategy("ANALYSIS", strategy.DefaultAdaptiveConfig())
	default:
		NotFound(w, "strategy not found: "+name)
		return
	}

	if err != nil {
		InternalError(w, "failed to create strategy: "+err.Error())
		return
	}

	// Check minimum candles
	if len(req.Candles) < strat.MinimumCandles() {
		Error(w, http.StatusBadRequest, ErrCodeInsufficientData,
			"insufficient candles: need "+strconv.Itoa(strat.MinimumCandles())+", got "+strconv.Itoa(len(req.Candles)))
		return
	}

	// Analyze
	signal, err := strat.Analyze(req.Candles)
	if err != nil {
		if err == strategy.ErrNoSignal {
			JSON(w, http.StatusOK, map[string]interface{}{
				"signal": nil,
				"message": "no trading signal detected",
			})
			return
		}
		InternalError(w, "analysis failed: "+err.Error())
		return
	}

	JSON(w, http.StatusOK, map[string]interface{}{
		"signal": map[string]interface{}{
			"type":       signal.Type.String(),
			"price":      signal.Price,
			"confidence": signal.Confidence,
			"reason":     signal.Reason,
			"indicators": signal.Indicators,
		},
	})
}

// RunBacktestWithData runs a backtest with provided candle data
func RunBacktestWithData(strat strategy.Strategy, candles []model.Candle, symbol, interval string, initialBalance float64) (*backtest.BacktestResult, error) {
	config := backtest.DefaultEngineConfig()
	config.InitialBalance = initialBalance

	engine, err := backtest.NewEngine(config)
	if err != nil {
		return nil, err
	}

	return engine.Run(strat, candles, symbol, interval)
}
