package backtest

import (
	"errors"
	"fmt"
	"log"

	"final-bot-trader-api/internal/exchange/model"
	"final-bot-trader-api/internal/strategy"
)

// Common errors
var (
	ErrNoData         = errors.New("no candle data provided")
	ErrInvalidBalance = errors.New("invalid initial balance")
)

// EngineConfig holds configuration for the backtest engine
type EngineConfig struct {
	InitialBalance float64
	Commission     float64 // Commission per trade (as decimal, e.g., 0.001 = 0.1%)
	Slippage       float64 // Slippage per trade (as decimal, e.g., 0.001 = 0.1%)
	RiskManager    *strategy.RiskManager
	Verbose        bool    // Enable verbose logging
}

// DefaultEngineConfig returns default engine configuration
func DefaultEngineConfig() EngineConfig {
	rm, _ := strategy.NewRiskManager(strategy.DefaultRiskConfig())
	return EngineConfig{
		InitialBalance: 10000,
		Commission:     0.001, // 0.1%
		Slippage:       0.0005, // 0.05%
		RiskManager:    rm,
	}
}

// Engine is the backtesting engine
type Engine struct {
	config    EngineConfig
	portfolio *Portfolio
	strategy  strategy.Strategy
	symbol    string
	interval  string
}

// NewEngine creates a new backtest engine
func NewEngine(config EngineConfig) (*Engine, error) {
	if config.InitialBalance <= 0 {
		return nil, ErrInvalidBalance
	}

	return &Engine{
		config:    config,
		portfolio: NewPortfolio(config.InitialBalance),
	}, nil
}

// Run executes the backtest on the given candles
func (e *Engine) Run(strat strategy.Strategy, candles []model.Candle, symbol, interval string) (*BacktestResult, error) {
	if len(candles) == 0 {
		return nil, ErrNoData
	}

	if len(candles) < strat.MinimumCandles() {
		return nil, fmt.Errorf("insufficient candles: need %d, got %d", strat.MinimumCandles(), len(candles))
	}

	e.strategy = strat
	e.symbol = symbol
	e.interval = interval
	e.portfolio = NewPortfolio(e.config.InitialBalance)

	if e.config.Verbose {
		log.Printf("Starting backtest: %s on %s %s with %d candles", strat.Name(), symbol, interval, len(candles))
	}

	// Iterate through candles
	for i := strat.MinimumCandles(); i < len(candles); i++ {
		currentCandle := candles[i]
		historicalCandles := candles[:i+1]

		// Update portfolio equity
		e.portfolio.UpdateEquity(currentCandle.Close)

		// Check open position for SL/TP
		if e.portfolio.HasOpenPosition() {
			e.checkExitConditions(currentCandle)
			continue // Don't open new position while one is open
		}

		// Analyze for new signals
		signal, err := strat.Analyze(historicalCandles)
		if err != nil {
			if err != strategy.ErrNoSignal {
				log.Printf("Strategy error at %s: %v", currentCandle.OpenTime, err)
			}
			continue
		}

		// Apply risk management
		if e.config.RiskManager != nil {
			err = e.config.RiskManager.ApplyRiskManagement(signal, historicalCandles, e.portfolio.Balance)
			if err != nil {
				log.Printf("Risk management error: %v", err)
				continue
			}
		}

		// Open position
		e.openPosition(signal, currentCandle)
	}

	// Close any remaining open position at the end
	if e.portfolio.HasOpenPosition() {
		lastCandle := candles[len(candles)-1]
		e.portfolio.ClosePosition(lastCandle.Close, lastCandle.CloseTime, "End of backtest")
	}

	// Calculate results
	startDate := candles[0].OpenTime
	endDate := candles[len(candles)-1].CloseTime
	result := CalculateMetrics(e.portfolio, startDate, endDate, strat.Name(), symbol, interval)

	if e.config.Verbose {
		log.Printf("Backtest complete: %d trades, %.2f%% return, %.2f%% max drawdown",
			result.TotalTrades, result.TotalReturn, result.MaxDrawdownPct)
	}

	return result, nil
}

func (e *Engine) openPosition(signal *strategy.Signal, candle model.Candle) {
	// Apply slippage to entry price
	entryPrice := signal.Price
	if signal.Type == strategy.SignalBuy {
		entryPrice = signal.Price * (1 + e.config.Slippage)
	} else {
		entryPrice = signal.Price * (1 - e.config.Slippage)
	}

	// Calculate commission
	commission := entryPrice * signal.Quantity * e.config.Commission * 2 // Entry + exit

	// Adjust signal with actual entry price
	signal.Price = entryPrice
	signal.Timestamp = candle.OpenTime

	trade := NewTradeFromSignal(0, signal, commission)

	err := e.portfolio.OpenPosition(trade)
	if err != nil {
		if e.config.Verbose {
			log.Printf("Failed to open position: %v", err)
		}
		return
	}

	if e.config.Verbose {
		log.Printf("Opened %s position at %.2f (SL: %.2f, TP: %.2f)",
			trade.Type, trade.EntryPrice, trade.StopLoss, trade.TakeProfit)
	}
}

func (e *Engine) checkExitConditions(candle model.Candle) {
	trade := e.portfolio.OpenTrade
	if trade == nil {
		return
	}

	var exitPrice float64
	var exitReason string

	// Check stop loss
	if trade.CheckStopLoss(candle.Low) && trade.Type == TradeTypeLong {
		exitPrice = trade.StopLoss * (1 - e.config.Slippage)
		exitReason = "Stop Loss"
	} else if trade.CheckStopLoss(candle.High) && trade.Type == TradeTypeShort {
		exitPrice = trade.StopLoss * (1 + e.config.Slippage)
		exitReason = "Stop Loss"
	}

	// Check take profit
	if exitPrice == 0 {
		if trade.CheckTakeProfit(candle.High) && trade.Type == TradeTypeLong {
			exitPrice = trade.TakeProfit * (1 - e.config.Slippage)
			exitReason = "Take Profit"
		} else if trade.CheckTakeProfit(candle.Low) && trade.Type == TradeTypeShort {
			exitPrice = trade.TakeProfit * (1 + e.config.Slippage)
			exitReason = "Take Profit"
		}
	}

	// Exit if condition met
	if exitPrice > 0 {
		e.portfolio.ClosePosition(exitPrice, candle.CloseTime, exitReason)
		if e.config.Verbose {
			log.Printf("Closed position at %.2f (%s), PnL: %.2f",
				exitPrice, exitReason, e.portfolio.ClosedTrades[len(e.portfolio.ClosedTrades)-1].PnL)
		}
	}
}

// RunMultiple runs backtest with multiple parameter variations
func (e *Engine) RunMultiple(strategyFactory func(params map[string]interface{}) (strategy.Strategy, error),
	paramSets []map[string]interface{},
	candles []model.Candle,
	symbol, interval string) ([]*BacktestResult, error) {

	results := make([]*BacktestResult, 0, len(paramSets))

	for i, params := range paramSets {
		strat, err := strategyFactory(params)
		if err != nil {
			log.Printf("Failed to create strategy with params %d: %v", i, err)
			continue
		}

		result, err := e.Run(strat, candles, symbol, interval)
		if err != nil {
			log.Printf("Backtest failed for params %d: %v", i, err)
			continue
		}

		results = append(results, result)
	}

	return results, nil
}

// FindBestResult finds the best result based on a metric
func FindBestResult(results []*BacktestResult, metric string) *BacktestResult {
	if len(results) == 0 {
		return nil
	}

	best := results[0]
	bestValue := getMetricValue(best, metric)

	for _, result := range results[1:] {
		value := getMetricValue(result, metric)
		if value > bestValue {
			bestValue = value
			best = result
		}
	}

	return best
}

func getMetricValue(result *BacktestResult, metric string) float64 {
	switch metric {
	case "sharpe":
		return result.SharpeRatio
	case "sortino":
		return result.SortinoRatio
	case "calmar":
		return result.CalmarRatio
	case "return":
		return result.TotalReturn
	case "profit_factor":
		return result.ProfitFactor
	case "win_rate":
		return result.WinRate
	default:
		return result.TotalReturn
	}
}

// QuickBacktest is a convenience function for quick backtesting
func QuickBacktest(strat strategy.Strategy, candles []model.Candle, symbol, interval string, initialBalance float64) (*BacktestResult, error) {
	config := DefaultEngineConfig()
	config.InitialBalance = initialBalance

	engine, err := NewEngine(config)
	if err != nil {
		return nil, err
	}

	return engine.Run(strat, candles, symbol, interval)
}
