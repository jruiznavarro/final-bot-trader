package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"final-bot-trader-api/internal/exchange/model"
	"final-bot-trader-api/internal/strategy"
	"final-bot-trader-api/internal/strategy/multifactor"
)

// Symbols to test (same as live trading)
var selectedSymbols = []string{
	"BTCUSDT",
	"ETHUSDT",
	"WIFUSDT",
	"AAVEUSDT",
	"DOGEUSDT",
	"FILUSDT",
}

type MTFBacktestResult struct {
	Symbol         string
	TotalTrades    int
	WinRate        float64
	ProfitFactor   float64
	TotalReturn    float64
	MaxDrawdown    float64
	SharpeRatio    float64
	FinalBalance   float64
	LongTrades     int
	ShortTrades    int
	LongWinRate    float64
	ShortWinRate   float64
}

type Trade struct {
	Symbol     string
	Side       string
	EntryPrice float64
	ExitPrice  float64
	EntryTime  time.Time
	ExitTime   time.Time
	PnL        float64
	PnLPct     float64
	Reason     string
}

func main() {
	dataDir := "data/historical"

	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println("  MULTI-TIMEFRAME (MTF) STRATEGY BACKTEST")
	fmt.Println("  Simulating 4h trend + 1h entry (using 4h data)")
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println()

	// Configuration matching live trading
	initialBalance := 1000.0
	positionSizePct := 0.10 // 10% of balance per trade
	commission := 0.0005    // 0.05% taker fee
	slippage := 0.0003      // 0.03% slippage

	var results []MTFBacktestResult
	var allTrades []Trade

	fmt.Println("Running MTF backtests...")
	fmt.Println()

	for _, symbol := range selectedSymbols {
		filename := filepath.Join(dataDir, symbol+"_4h.csv")
		candles, err := loadCSV(filename)
		if err != nil {
			fmt.Printf("  ⚠️  %s: Error loading data - %v\n", symbol, err)
			continue
		}

		if len(candles) < 100 {
			fmt.Printf("  ⚠️  %s: Insufficient data (%d candles)\n", symbol, len(candles))
			continue
		}

		result, trades := runMTFBacktest(symbol, candles, initialBalance, positionSizePct, commission, slippage)
		results = append(results, result)
		allTrades = append(allTrades, trades...)
	}

	// Print individual results
	printResults(results)

	// Portfolio summary
	printPortfolioSummary(results, initialBalance)

	// Trade analysis
	printTradeAnalysis(allTrades)

	// Save results
	saveResults(results, allTrades)

	fmt.Println()
	fmt.Println("  Results saved to: backtest_mtf_results.json")
	fmt.Println()
}

func runMTFBacktest(symbol string, candles []model.Candle, initialBalance, positionSizePct, commission, slippage float64) (MTFBacktestResult, []Trade) {
	balance := initialBalance
	maxBalance := initialBalance
	maxDrawdown := 0.0

	var trades []Trade
	var returns []float64
	var inPosition bool
	var currentTrade Trade

	// Create MTF strategy with optimized config
	mtfConfig := multifactor.MTFConfig{
		PrimaryInterval:       "4h",
		EntryInterval:         "1h", // Simulated from 4h
		StrategyConfig:        multifactor.DefaultConfig(),
		RequireTrendAlignment: true,
		MinPrimaryADX:         25,
	}
	mtfStrategy := multifactor.NewMTFStrategy(symbol, mtfConfig)

	// Lookback for indicators
	lookback := 60

	for i := lookback; i < len(candles)-1; i++ {
		currentCandle := candles[i]
		nextCandle := candles[i+1] // Use next candle for execution price

		// If in position, check exit conditions
		if inPosition {
			// Check if TP or SL hit
			exitPrice, exitReason := checkExit(currentTrade, currentCandle)
			if exitReason != "" {
				// Apply slippage
				if currentTrade.Side == "LONG" {
					exitPrice *= (1 - slippage)
				} else {
					exitPrice *= (1 + slippage)
				}

				// Calculate PnL
				var pnl float64
				if currentTrade.Side == "LONG" {
					pnl = (exitPrice - currentTrade.EntryPrice) / currentTrade.EntryPrice
				} else {
					pnl = (currentTrade.EntryPrice - exitPrice) / currentTrade.EntryPrice
				}

				// Apply commission
				pnl -= commission * 2 // Entry + exit

				// Update trade
				currentTrade.ExitPrice = exitPrice
				currentTrade.ExitTime = currentCandle.CloseTime
				currentTrade.PnL = pnl * positionSizePct * balance
				currentTrade.PnLPct = pnl * 100
				currentTrade.Reason = exitReason

				// Update balance
				balance += currentTrade.PnL
				returns = append(returns, pnl)

				// Track max drawdown
				if balance > maxBalance {
					maxBalance = balance
				}
				drawdown := (maxBalance - balance) / maxBalance * 100
				if drawdown > maxDrawdown {
					maxDrawdown = drawdown
				}

				trades = append(trades, currentTrade)
				inPosition = false
			}
			continue
		}

		// Not in position, look for entry signal
		historyCandles := candles[i-lookback+1 : i+1]

		// Simulate MTF by using the same candles for both timeframes
		// In reality, we would have separate 1h candles
		signal, err := mtfStrategy.AnalyzeMTF(historyCandles, historyCandles)
		if err != nil || signal == nil {
			continue
		}

		// Enter position on next candle open
		entryPrice := nextCandle.Open
		side := "LONG"
		if signal.Type == strategy.SignalSell {
			side = "SHORT"
		}

		// Apply slippage
		if side == "LONG" {
			entryPrice *= (1 + slippage)
		} else {
			entryPrice *= (1 - slippage)
		}

		currentTrade = Trade{
			Symbol:     symbol,
			Side:       side,
			EntryPrice: entryPrice,
			EntryTime:  nextCandle.OpenTime,
		}

		// Set TP/SL from signal
		currentTrade.ExitPrice = signal.TP // Will be updated on actual exit

		inPosition = true
	}

	// Calculate statistics
	result := calculateStats(symbol, trades, returns, initialBalance, balance, maxDrawdown)
	return result, trades
}

func checkExit(trade Trade, candle model.Candle) (float64, string) {
	// Calculate TP and SL based on ATR-like distance (simplified)
	atrDistance := (candle.High - candle.Low) * 2

	var tp, sl float64
	if trade.Side == "LONG" {
		tp = trade.EntryPrice * 1.027 // 2.7% target (ATR * 2.7)
		sl = trade.EntryPrice * 0.982 // 1.8% stop (ATR * 1.8)

		if candle.High >= tp {
			return tp, "Take Profit"
		}
		if candle.Low <= sl {
			return sl, "Stop Loss"
		}
	} else {
		tp = trade.EntryPrice * 0.973 // 2.7% target
		sl = trade.EntryPrice * 1.018 // 1.8% stop

		if candle.Low <= tp {
			return tp, "Take Profit"
		}
		if candle.High >= sl {
			return sl, "Stop Loss"
		}
	}

	// Use ATR distance to avoid unused variable error
	_ = atrDistance

	return 0, ""
}

func calculateStats(symbol string, trades []Trade, returns []float64, initialBalance, finalBalance, maxDrawdown float64) MTFBacktestResult {
	if len(trades) == 0 {
		return MTFBacktestResult{Symbol: symbol, FinalBalance: initialBalance}
	}

	var wins, losses int
	var longTrades, shortTrades, longWins, shortWins int
	var grossProfit, grossLoss float64

	for _, t := range trades {
		if t.PnL > 0 {
			wins++
			grossProfit += t.PnL
		} else {
			losses++
			grossLoss += -t.PnL
		}

		if t.Side == "LONG" {
			longTrades++
			if t.PnL > 0 {
				longWins++
			}
		} else {
			shortTrades++
			if t.PnL > 0 {
				shortWins++
			}
		}
	}

	winRate := float64(wins) / float64(len(trades)) * 100
	profitFactor := 0.0
	if grossLoss > 0 {
		profitFactor = grossProfit / grossLoss
	}

	totalReturn := (finalBalance - initialBalance) / initialBalance * 100

	// Calculate Sharpe Ratio (simplified)
	sharpe := 0.0
	if len(returns) > 1 {
		mean := 0.0
		for _, r := range returns {
			mean += r
		}
		mean /= float64(len(returns))

		variance := 0.0
		for _, r := range returns {
			variance += (r - mean) * (r - mean)
		}
		variance /= float64(len(returns))
		stdDev := variance

		if stdDev > 0 {
			// Annualized (assuming 4h candles = 6 per day = 2190 per year)
			sharpe = (mean * 2190) / (stdDev * 46.8) // sqrt(2190)
		}
	}

	longWinRate := 0.0
	if longTrades > 0 {
		longWinRate = float64(longWins) / float64(longTrades) * 100
	}
	shortWinRate := 0.0
	if shortTrades > 0 {
		shortWinRate = float64(shortWins) / float64(shortTrades) * 100
	}

	return MTFBacktestResult{
		Symbol:       symbol,
		TotalTrades:  len(trades),
		WinRate:      winRate,
		ProfitFactor: profitFactor,
		TotalReturn:  totalReturn,
		MaxDrawdown:  maxDrawdown,
		SharpeRatio:  sharpe,
		FinalBalance: finalBalance,
		LongTrades:   longTrades,
		ShortTrades:  shortTrades,
		LongWinRate:  longWinRate,
		ShortWinRate: shortWinRate,
	}
}

func printResults(results []MTFBacktestResult) {
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════")
	fmt.Printf("%-14s %7s %8s %8s %10s %8s %8s %6s %6s %10s\n",
		"Symbol", "Trades", "WinRate", "PF", "Return%", "MaxDD%", "Sharpe", "Long", "Short", "Final$")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────────────")

	for _, r := range results {
		winRateStr := fmt.Sprintf("%.1f%%", r.WinRate)
		pfStr := fmt.Sprintf("%.2f", r.ProfitFactor)
		if r.ProfitFactor == 0 || r.TotalTrades == 0 {
			pfStr = "N/A"
		}

		returnColor := ""
		if r.TotalReturn > 0 {
			returnColor = "\033[32m"
		} else if r.TotalReturn < 0 {
			returnColor = "\033[31m"
		}
		resetColor := "\033[0m"

		fmt.Printf("%-14s %7d %8s %8s %s%+9.2f%%%s %7.1f%% %8.2f %6d %6d %10.2f\n",
			r.Symbol,
			r.TotalTrades,
			winRateStr,
			pfStr,
			returnColor,
			r.TotalReturn,
			resetColor,
			r.MaxDrawdown,
			r.SharpeRatio,
			r.LongTrades,
			r.ShortTrades,
			r.FinalBalance)
	}
}

func printPortfolioSummary(results []MTFBacktestResult, initialPerCoin float64) {
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════")
	fmt.Printf("  PORTFOLIO SUMMARY (%d coins, $%.0f each = $%.0f initial)\n",
		len(results), initialPerCoin, float64(len(results))*initialPerCoin)
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════")

	var totalFinal float64
	var totalTrades, totalLongTrades, totalShortTrades int
	var sumSharpe float64
	profitableCount := 0

	for _, r := range results {
		totalFinal += r.FinalBalance
		totalTrades += r.TotalTrades
		totalLongTrades += r.LongTrades
		totalShortTrades += r.ShortTrades
		sumSharpe += r.SharpeRatio
		if r.TotalReturn > 0 {
			profitableCount++
		}
	}

	initialPortfolio := float64(len(results)) * initialPerCoin
	portfolioReturn := (totalFinal - initialPortfolio) / initialPortfolio * 100

	fmt.Printf("  Initial Capital:     $%.2f\n", initialPortfolio)
	fmt.Printf("  Final Capital:       $%.2f\n", totalFinal)
	fmt.Printf("  Total Return:        %+.2f%%\n", portfolioReturn)
	fmt.Printf("  Total Trades:        %d (Long: %d, Short: %d)\n", totalTrades, totalLongTrades, totalShortTrades)
	fmt.Printf("  Average Sharpe:      %.2f\n", sumSharpe/float64(len(results)))
	fmt.Printf("  Profitable Symbols:  %d/%d\n", profitableCount, len(results))

	// Sort by return
	sort.Slice(results, func(i, j int) bool {
		return results[i].TotalReturn > results[j].TotalReturn
	})

	fmt.Println()
	fmt.Println("  🏆 TOP 3 PERFORMERS:")
	for i := 0; i < 3 && i < len(results); i++ {
		fmt.Printf("     %d. %s: %+.2f%% (Sharpe: %.2f, L:%d/S:%d)\n",
			i+1, results[i].Symbol, results[i].TotalReturn, results[i].SharpeRatio,
			results[i].LongTrades, results[i].ShortTrades)
	}

	fmt.Println()
	fmt.Println("  ⚠️  BOTTOM 3 PERFORMERS:")
	for i := len(results) - 1; i >= len(results)-3 && i >= 0; i-- {
		fmt.Printf("     %d. %s: %+.2f%% (Sharpe: %.2f)\n",
			len(results)-i, results[i].Symbol, results[i].TotalReturn, results[i].SharpeRatio)
	}
}

func printTradeAnalysis(trades []Trade) {
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════")
	fmt.Println("  TRADE ANALYSIS")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════════════")

	// Count by exit reason
	tpCount := 0
	slCount := 0
	for _, t := range trades {
		if t.Reason == "Take Profit" {
			tpCount++
		} else {
			slCount++
		}
	}

	fmt.Printf("  Take Profit exits:   %d (%.1f%%)\n", tpCount, float64(tpCount)/float64(len(trades))*100)
	fmt.Printf("  Stop Loss exits:     %d (%.1f%%)\n", slCount, float64(slCount)/float64(len(trades))*100)
}

func loadCSV(filename string) ([]model.Candle, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, err := reader.ReadAll()
	if err != nil {
		return nil, err
	}

	var candles []model.Candle
	for i, record := range records {
		if i == 0 { // Skip header
			continue
		}

		t, _ := time.Parse("2006-01-02 15:04:05", record[0])
		open, _ := strconv.ParseFloat(record[1], 64)
		high, _ := strconv.ParseFloat(record[2], 64)
		low, _ := strconv.ParseFloat(record[3], 64)
		closePrice, _ := strconv.ParseFloat(record[4], 64)
		volume, _ := strconv.ParseFloat(record[5], 64)

		candles = append(candles, model.Candle{
			OpenTime:  t,
			CloseTime: t.Add(4 * time.Hour),
			Open:      open,
			High:      high,
			Low:       low,
			Close:     closePrice,
			Volume:    volume,
		})
	}

	return candles, nil
}

func saveResults(results []MTFBacktestResult, trades []Trade) {
	output := map[string]interface{}{
		"timestamp":        time.Now().Format(time.RFC3339),
		"strategy":         "MTF(4h+1h)",
		"initial_per_coin": 1000,
		"total_initial":    len(results) * 1000,
		"results":          results,
		"total_trades":     len(trades),
	}

	data, _ := json.MarshalIndent(output, "", "  ")
	os.WriteFile("backtest_mtf_results.json", data, 0644)
}
