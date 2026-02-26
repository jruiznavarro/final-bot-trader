package backtest

import (
	"fmt"
	"sort"
	"time"

	"etf-bot-trader-api/internal/exchange"
	"etf-bot-trader-api/internal/strategy"
)

// Trade represents a backtest trade
type Trade struct {
	Symbol     string
	Side       string
	EntryTime  time.Time
	ExitTime   time.Time
	EntryPrice float64
	ExitPrice  float64
	Quantity   float64
	StopLoss   float64
	TakeProfit float64
	PnL        float64
	PnLPct     float64
	ExitReason string
	Reason     string
}

// Result holds backtest results
type Result struct {
	Symbol         string
	StartDate      time.Time
	EndDate        time.Time
	InitialCapital float64
	FinalCapital   float64
	TotalReturn    float64
	TotalReturnPct float64
	TotalTrades    int
	WinningTrades  int
	LosingTrades   int
	WinRate        float64
	AvgWin         float64
	AvgLoss        float64
	ProfitFactor   float64
	MaxDrawdown    float64
	MaxDrawdownPct float64
	SharpeRatio    float64
	Trades         []Trade
}

// Config holds backtest configuration
type Config struct {
	InitialCapital   float64
	PositionSizePct  float64
	Commission       float64 // Per trade commission
	Slippage         float64 // Slippage as % of price
}

// DefaultConfig returns default backtest configuration
func DefaultConfig() Config {
	return Config{
		InitialCapital:  10000,
		PositionSizePct: 0.10,  // 10% per trade (need more for expensive ETFs like SPY/QQQ)
		Commission:      0,      // Alpaca is commission-free
		Slippage:        0.01,   // 0.01% slippage
	}
}

// Engine runs backtests
type Engine struct {
	config Config
}

// NewEngine creates a new backtest engine
func NewEngine(config Config) *Engine {
	return &Engine{config: config}
}

// Run runs a backtest on the given candles
func (e *Engine) Run(symbol string, dailyCandles, hourlyCandles []exchange.Candle) (*Result, error) {
	if len(hourlyCandles) < 100 {
		return nil, fmt.Errorf("insufficient hourly candles: %d", len(hourlyCandles))
	}
	if len(dailyCandles) < 50 {
		return nil, fmt.Errorf("insufficient daily candles: %d", len(dailyCandles))
	}

	strat := strategy.NewETFMomentumStrategy(symbol, strategy.DefaultETFMomentumConfig())

	capital := e.config.InitialCapital
	var trades []Trade
	var inPosition bool
	var currentTrade *Trade
	var equityCurve []float64

	// Build a map of daily candles by date for quick lookup
	dailyByDate := make(map[string][]exchange.Candle)
	for _, dc := range dailyCandles {
		dateKey := dc.Time.Format("2006-01-02")
		dailyByDate[dateKey] = append(dailyByDate[dateKey], dc)
	}

	// Get sorted dates
	var dates []string
	for d := range dailyByDate {
		dates = append(dates, d)
	}
	sort.Strings(dates)

	// Iterate through hourly candles
	for i := 100; i < len(hourlyCandles); i++ {
		currentCandle := hourlyCandles[i]
		currentPrice := currentCandle.Close

		// Check if we're in a position
		if inPosition && currentTrade != nil {
			// Check stop loss
			hitSL := false
			hitTP := false

			if currentTrade.Side == "long" {
				if currentCandle.Low <= currentTrade.StopLoss {
					hitSL = true
					currentTrade.ExitPrice = currentTrade.StopLoss
				} else if currentCandle.High >= currentTrade.TakeProfit {
					hitTP = true
					currentTrade.ExitPrice = currentTrade.TakeProfit
				}
			} else {
				if currentCandle.High >= currentTrade.StopLoss {
					hitSL = true
					currentTrade.ExitPrice = currentTrade.StopLoss
				} else if currentCandle.Low <= currentTrade.TakeProfit {
					hitTP = true
					currentTrade.ExitPrice = currentTrade.TakeProfit
				}
			}

			if hitSL || hitTP {
				currentTrade.ExitTime = currentCandle.Time

				// Apply slippage
				slippage := currentTrade.ExitPrice * (e.config.Slippage / 100)
				if currentTrade.Side == "long" {
					if hitSL {
						currentTrade.ExitPrice -= slippage
					} else {
						currentTrade.ExitPrice -= slippage // Conservative
					}
				} else {
					if hitSL {
						currentTrade.ExitPrice += slippage
					} else {
						currentTrade.ExitPrice += slippage
					}
				}

				// Calculate PnL
				if currentTrade.Side == "long" {
					currentTrade.PnL = (currentTrade.ExitPrice - currentTrade.EntryPrice) * currentTrade.Quantity
				} else {
					currentTrade.PnL = (currentTrade.EntryPrice - currentTrade.ExitPrice) * currentTrade.Quantity
				}
				currentTrade.PnL -= e.config.Commission * 2 // Entry + exit commission
				currentTrade.PnLPct = (currentTrade.PnL / (currentTrade.EntryPrice * currentTrade.Quantity)) * 100

				if hitSL {
					currentTrade.ExitReason = "Stop Loss"
				} else {
					currentTrade.ExitReason = "Take Profit"
				}

				capital += currentTrade.PnL
				trades = append(trades, *currentTrade)
				inPosition = false
				currentTrade = nil
			}
		}

		// Look for new signals if not in position
		if !inPosition {
			// Get recent candles for analysis
			recentHourly := hourlyCandles[i-99 : i+1]

			// Find matching daily candles (last 90 days)
			var recentDaily []exchange.Candle
			currentDate := currentCandle.Time
			for j := 0; j < 90 && len(recentDaily) < 90; j++ {
				checkDate := currentDate.AddDate(0, 0, -j).Format("2006-01-02")
				if candles, ok := dailyByDate[checkDate]; ok {
					recentDaily = append(recentDaily, candles...)
				}
			}

			// Reverse to get chronological order
			for i, j := 0, len(recentDaily)-1; i < j; i, j = i+1, j-1 {
				recentDaily[i], recentDaily[j] = recentDaily[j], recentDaily[i]
			}

			// Use multi-timeframe analysis if we have enough daily data
			var signal *strategy.Signal
			var err error
			if len(recentDaily) >= 50 {
				signal, err = strat.AnalyzeWithDailyTrend(recentDaily, recentHourly)
			} else {
				signal, err = strat.Analyze(recentHourly)
			}
			if err == nil && signal != nil {
				// Calculate position size
				positionValue := capital * e.config.PositionSizePct
				quantity := positionValue / currentPrice
				quantity = float64(int(quantity)) // Round to whole shares

				if quantity >= 1 {
					// Apply entry slippage
					entryPrice := currentPrice
					slippage := entryPrice * (e.config.Slippage / 100)
					if signal.Type == strategy.SignalBuy {
						entryPrice += slippage
					} else {
						entryPrice -= slippage
					}

					side := "long"
					if signal.Type == strategy.SignalSell {
						side = "short"
					}

					currentTrade = &Trade{
						Symbol:     symbol,
						Side:       side,
						EntryTime:  currentCandle.Time,
						EntryPrice: entryPrice,
						Quantity:   quantity,
						StopLoss:   signal.StopLoss,
						TakeProfit: signal.TakeProfit,
						Reason:     signal.Reason,
					}
					inPosition = true
				}
			}
		}

		equityCurve = append(equityCurve, capital)
	}

	// Close any open position at end
	if inPosition && currentTrade != nil {
		lastCandle := hourlyCandles[len(hourlyCandles)-1]
		currentTrade.ExitTime = lastCandle.Time
		currentTrade.ExitPrice = lastCandle.Close
		currentTrade.ExitReason = "End of backtest"

		if currentTrade.Side == "long" {
			currentTrade.PnL = (currentTrade.ExitPrice - currentTrade.EntryPrice) * currentTrade.Quantity
		} else {
			currentTrade.PnL = (currentTrade.EntryPrice - currentTrade.ExitPrice) * currentTrade.Quantity
		}
		currentTrade.PnL -= e.config.Commission * 2
		currentTrade.PnLPct = (currentTrade.PnL / (currentTrade.EntryPrice * currentTrade.Quantity)) * 100

		capital += currentTrade.PnL
		trades = append(trades, *currentTrade)
	}

	// Calculate statistics
	result := &Result{
		Symbol:         symbol,
		StartDate:      hourlyCandles[100].Time,
		EndDate:        hourlyCandles[len(hourlyCandles)-1].Time,
		InitialCapital: e.config.InitialCapital,
		FinalCapital:   capital,
		TotalReturn:    capital - e.config.InitialCapital,
		TotalReturnPct: ((capital - e.config.InitialCapital) / e.config.InitialCapital) * 100,
		TotalTrades:    len(trades),
		Trades:         trades,
	}

	// Win/Loss stats
	var totalWins, totalLosses float64
	for _, t := range trades {
		if t.PnL >= 0 {
			result.WinningTrades++
			totalWins += t.PnL
		} else {
			result.LosingTrades++
			totalLosses += -t.PnL
		}
	}

	if result.TotalTrades > 0 {
		result.WinRate = float64(result.WinningTrades) / float64(result.TotalTrades) * 100
	}
	if result.WinningTrades > 0 {
		result.AvgWin = totalWins / float64(result.WinningTrades)
	}
	if result.LosingTrades > 0 {
		result.AvgLoss = totalLosses / float64(result.LosingTrades)
	}
	if totalLosses > 0 {
		result.ProfitFactor = totalWins / totalLosses
	}

	// Calculate max drawdown
	peak := e.config.InitialCapital
	for _, equity := range equityCurve {
		if equity > peak {
			peak = equity
		}
		drawdown := peak - equity
		if drawdown > result.MaxDrawdown {
			result.MaxDrawdown = drawdown
			result.MaxDrawdownPct = (drawdown / peak) * 100
		}
	}

	// Calculate Sharpe ratio (simplified)
	if len(trades) > 1 {
		var returns []float64
		for _, t := range trades {
			returns = append(returns, t.PnLPct)
		}
		avgReturn := result.TotalReturnPct / float64(len(trades))
		var variance float64
		for _, r := range returns {
			variance += (r - avgReturn) * (r - avgReturn)
		}
		stdDev := 0.0
		if len(returns) > 1 {
			stdDev = variance / float64(len(returns)-1)
			if stdDev > 0 {
				stdDev = sqrt(stdDev)
			}
		}
		if stdDev > 0 {
			result.SharpeRatio = (avgReturn) / stdDev
		}
	}

	return result, nil
}

func sqrt(x float64) float64 {
	if x <= 0 {
		return 0
	}
	z := x
	for i := 0; i < 10; i++ {
		z = (z + x/z) / 2
	}
	return z
}

// PrintResult prints backtest results
func PrintResult(r *Result) {
	fmt.Printf("\n═══════════════════════════════════════════════════════════════\n")
	fmt.Printf("  BACKTEST RESULTS: %s\n", r.Symbol)
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
	fmt.Printf("  Period:          %s to %s\n", r.StartDate.Format("2006-01-02"), r.EndDate.Format("2006-01-02"))
	fmt.Printf("  Initial Capital: $%.2f\n", r.InitialCapital)
	fmt.Printf("  Final Capital:   $%.2f\n", r.FinalCapital)
	fmt.Println()
	fmt.Printf("  Total Return:    $%.2f (%.2f%%)\n", r.TotalReturn, r.TotalReturnPct)
	fmt.Printf("  Max Drawdown:    $%.2f (%.2f%%)\n", r.MaxDrawdown, r.MaxDrawdownPct)
	fmt.Println()
	fmt.Printf("  Total Trades:    %d\n", r.TotalTrades)
	fmt.Printf("  Winning Trades:  %d (%.1f%%)\n", r.WinningTrades, r.WinRate)
	fmt.Printf("  Losing Trades:   %d\n", r.LosingTrades)
	fmt.Println()
	fmt.Printf("  Avg Win:         $%.2f\n", r.AvgWin)
	fmt.Printf("  Avg Loss:        $%.2f\n", r.AvgLoss)
	fmt.Printf("  Profit Factor:   %.2f\n", r.ProfitFactor)
	fmt.Printf("  Sharpe Ratio:    %.2f\n", r.SharpeRatio)
	fmt.Printf("═══════════════════════════════════════════════════════════════\n")
}
