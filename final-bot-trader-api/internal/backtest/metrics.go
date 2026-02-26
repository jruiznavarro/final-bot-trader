package backtest

import (
	"math"
	"time"
)

// BacktestResult holds the complete results of a backtest
type BacktestResult struct {
	// Time period
	StartDate time.Time
	EndDate   time.Time
	Duration  time.Duration

	// Capital
	InitialBalance float64
	FinalBalance   float64
	PeakBalance    float64

	// Trade statistics
	TotalTrades    int
	WinningTrades  int
	LosingTrades   int
	WinRate        float64

	// PnL metrics
	TotalPnL       float64
	TotalProfit    float64
	TotalLoss      float64
	ProfitFactor   float64
	AverageWin     float64
	AverageLoss    float64
	LargestWin     float64
	LargestLoss    float64
	AverageTrade   float64

	// Risk metrics
	MaxDrawdown    float64
	MaxDrawdownPct float64
	SharpeRatio    float64
	SortinoRatio   float64
	CalmarRatio    float64

	// Time metrics
	AverageHoldTime time.Duration
	MaxHoldTime     time.Duration
	MinHoldTime     time.Duration

	// Return metrics
	TotalReturn    float64 // Percentage
	AnnualizedReturn float64

	// Strategy info
	StrategyName   string
	Symbol         string
	Interval       string

	// All trades
	Trades []*Trade
}

// CalculateMetrics calculates all metrics from portfolio and trades
func CalculateMetrics(portfolio *Portfolio, startDate, endDate time.Time, strategyName, symbol, interval string) *BacktestResult {
	result := &BacktestResult{
		StartDate:      startDate,
		EndDate:        endDate,
		Duration:       endDate.Sub(startDate),
		InitialBalance: portfolio.InitialBalance,
		FinalBalance:   portfolio.Balance,
		PeakBalance:    portfolio.MaxBalance,
		TotalTrades:    portfolio.TotalTrades(),
		WinningTrades:  portfolio.WinningTrades(),
		LosingTrades:   portfolio.LosingTrades(),
		WinRate:        portfolio.WinRate(),
		TotalPnL:       portfolio.TotalPnL(),
		MaxDrawdown:    portfolio.MaxDrawdown,
		MaxDrawdownPct: portfolio.MaxDrawdown,
		StrategyName:   strategyName,
		Symbol:         symbol,
		Interval:       interval,
		Trades:         portfolio.ClosedTrades,
	}

	// Calculate profit/loss breakdown
	for _, trade := range portfolio.ClosedTrades {
		if trade.IsWinning() {
			result.TotalProfit += trade.PnL
			if trade.PnL > result.LargestWin {
				result.LargestWin = trade.PnL
			}
		} else {
			result.TotalLoss += -trade.PnL
			if -trade.PnL > result.LargestLoss {
				result.LargestLoss = -trade.PnL
			}
		}
	}

	result.AverageWin = portfolio.AverageWin()
	result.AverageLoss = portfolio.AverageLoss()
	result.ProfitFactor = portfolio.ProfitFactor()

	if result.TotalTrades > 0 {
		result.AverageTrade = result.TotalPnL / float64(result.TotalTrades)
	}

	// Calculate return
	result.TotalReturn = portfolio.ReturnPercent()

	// Calculate annualized return
	if result.Duration.Hours() > 0 {
		years := result.Duration.Hours() / (24 * 365)
		if years > 0 {
			result.AnnualizedReturn = math.Pow(1+result.TotalReturn/100, 1/years) - 1
			result.AnnualizedReturn *= 100
		}
	}

	// Calculate hold time metrics
	calculateHoldTimeMetrics(result, portfolio.ClosedTrades)

	// Calculate risk-adjusted returns
	calculateRiskMetrics(result, portfolio.ClosedTrades)

	// Calculate Calmar Ratio (annualized return / max drawdown)
	if result.MaxDrawdownPct > 0 {
		result.CalmarRatio = result.AnnualizedReturn / result.MaxDrawdownPct
	}

	return result
}

func calculateHoldTimeMetrics(result *BacktestResult, trades []*Trade) {
	if len(trades) == 0 {
		return
	}

	var totalDuration time.Duration
	result.MinHoldTime = trades[0].HoldDuration
	result.MaxHoldTime = trades[0].HoldDuration

	for _, trade := range trades {
		totalDuration += trade.HoldDuration

		if trade.HoldDuration < result.MinHoldTime {
			result.MinHoldTime = trade.HoldDuration
		}
		if trade.HoldDuration > result.MaxHoldTime {
			result.MaxHoldTime = trade.HoldDuration
		}
	}

	result.AverageHoldTime = totalDuration / time.Duration(len(trades))
}

func calculateRiskMetrics(result *BacktestResult, trades []*Trade) {
	if len(trades) < 2 {
		return
	}

	// Calculate returns for each trade
	returns := make([]float64, len(trades))
	for i, trade := range trades {
		returns[i] = trade.PnLPercent
	}

	// Calculate Sharpe Ratio (assuming risk-free rate = 0)
	avgReturn := mean(returns)
	stdDev := standardDeviation(returns, avgReturn)

	if stdDev > 0 {
		// Annualize: assuming 252 trading days
		annualizedReturn := avgReturn * 252
		annualizedStdDev := stdDev * math.Sqrt(252)
		result.SharpeRatio = annualizedReturn / annualizedStdDev
	}

	// Calculate Sortino Ratio (uses downside deviation)
	downsideReturns := make([]float64, 0)
	for _, r := range returns {
		if r < 0 {
			downsideReturns = append(downsideReturns, r)
		}
	}

	if len(downsideReturns) > 0 {
		downsideStdDev := standardDeviation(downsideReturns, 0)
		if downsideStdDev > 0 {
			annualizedReturn := avgReturn * 252
			annualizedDownsideStdDev := downsideStdDev * math.Sqrt(252)
			result.SortinoRatio = annualizedReturn / annualizedDownsideStdDev
		}
	}
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var sum float64
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func standardDeviation(values []float64, mean float64) float64 {
	if len(values) < 2 {
		return 0
	}

	var sumSquares float64
	for _, v := range values {
		diff := v - mean
		sumSquares += diff * diff
	}

	variance := sumSquares / float64(len(values)-1)
	return math.Sqrt(variance)
}

// Summary returns a formatted summary of the backtest results
func (r *BacktestResult) Summary() map[string]interface{} {
	return map[string]interface{}{
		"strategy":          r.StrategyName,
		"symbol":            r.Symbol,
		"interval":          r.Interval,
		"start_date":        r.StartDate.Format("2006-01-02"),
		"end_date":          r.EndDate.Format("2006-01-02"),
		"duration_days":     int(r.Duration.Hours() / 24),
		"initial_balance":   r.InitialBalance,
		"final_balance":     r.FinalBalance,
		"total_return_pct":  round(r.TotalReturn, 2),
		"annualized_return": round(r.AnnualizedReturn, 2),
		"total_trades":      r.TotalTrades,
		"winning_trades":    r.WinningTrades,
		"losing_trades":     r.LosingTrades,
		"win_rate":          round(r.WinRate, 2),
		"profit_factor":     round(r.ProfitFactor, 2),
		"max_drawdown_pct":  round(r.MaxDrawdownPct, 2),
		"sharpe_ratio":      round(r.SharpeRatio, 2),
		"sortino_ratio":     round(r.SortinoRatio, 2),
		"calmar_ratio":      round(r.CalmarRatio, 2),
		"average_win":       round(r.AverageWin, 2),
		"average_loss":      round(r.AverageLoss, 2),
		"largest_win":       round(r.LargestWin, 2),
		"largest_loss":      round(r.LargestLoss, 2),
	}
}

func round(val float64, precision int) float64 {
	pow := math.Pow(10, float64(precision))
	return math.Round(val*pow) / pow
}
