package main

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"final-bot-trader-api/internal/backtest"
	"final-bot-trader-api/internal/exchange/model"
	"final-bot-trader-api/internal/strategy"
	"final-bot-trader-api/internal/strategy/multifactor"
)

// Selected coins from correlation analysis (diversified portfolio)
var selectedSymbols = []string{
	"SUIUSDT",       // L1 - Trend Score: 98.16
	"ENAUSDT",       // DeFi - Trend Score: 101.56 (BEST)
	"TAOUSDT",       // AI - Trend Score: 99.98
	"ARBUSDT",       // L2 - Trend Score: 99.56
	"WIFUSDT",       // Meme - Trend Score: 99.56
	"DOGEUSDT",      // Meme - Trend Score: 97.38
	"FILUSDT",       // Infra - Trend Score: 97.42
	"LINKUSDT",      // DeFi - Trend Score: 95.22
	"1000PEPEUSDT",  // Meme - Trend Score: 96.50
	"WLDUSDT",       // AI - Trend Score: 99.09
	"SOLUSDT",       // L1 - Trend Score: 95.31
	"AAVEUSDT",      // DeFi - Trend Score: 96.08
}

type SymbolResult struct {
	Symbol         string
	TotalTrades    int
	WinRate        float64
	ProfitFactor   float64
	TotalReturn    float64
	MaxDrawdown    float64
	SharpeRatio    float64
	SortinoRatio   float64
	AvgWin         float64
	AvgLoss        float64
	FinalBalance   float64
}

func main() {
	dataDir := "data/historical"

	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println("  MULTI-FACTOR STRATEGY BACKTEST")
	fmt.Println("  Testing on 12 diversified cryptocurrencies")
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println()

	// Bitunix realistic costs
	config := backtest.EngineConfig{
		InitialBalance: 1000,      // Start with 1000 USDT per symbol
		Commission:     0.0005,    // 0.05% taker fee (Bitunix)
		Slippage:       0.0003,    // 0.03% slippage estimate
		RiskManager:    nil,       // Use strategy's built-in risk management
		Verbose:        false,
	}

	// Create risk manager with proper position sizing
	riskConfig := strategy.DefaultRiskConfig()
	riskConfig.MaxRiskPerTrade = 0.02  // 2% risk per trade
	riskConfig.MaxPositionSize = 0.20  // 20% max position (with 5x leverage = 100% notional)
	rm, _ := strategy.NewRiskManager(riskConfig)
	config.RiskManager = rm

	engine, err := backtest.NewEngine(config)
	if err != nil {
		fmt.Printf("Error creating engine: %v\n", err)
		return
	}

	var results []SymbolResult
	var allTrades int
	var totalPnL float64

	fmt.Println("Running backtests...")
	fmt.Println()

	for _, symbol := range selectedSymbols {
		filename := filepath.Join(dataDir, symbol+"_4h.csv")
		candles, err := loadCSV(filename)
		if err != nil {
			fmt.Printf("  ⚠️  %s: Error loading data - %v\n", symbol, err)
			continue
		}

		// Create strategy with default config
		strat := multifactor.NewMultiFactorStrategy(symbol, multifactor.DefaultConfig())

		result, err := engine.Run(strat, candles, symbol, "4h")
		if err != nil {
			fmt.Printf("  ⚠️  %s: Backtest error - %v\n", symbol, err)
			continue
		}

		sr := SymbolResult{
			Symbol:       symbol,
			TotalTrades:  result.TotalTrades,
			WinRate:      result.WinRate,
			ProfitFactor: result.ProfitFactor,
			TotalReturn:  result.TotalReturn,
			MaxDrawdown:  result.MaxDrawdownPct,
			SharpeRatio:  result.SharpeRatio,
			SortinoRatio: result.SortinoRatio,
			AvgWin:       result.AverageWin,
			AvgLoss:      result.AverageLoss,
			FinalBalance: result.FinalBalance,
		}
		results = append(results, sr)
		allTrades += result.TotalTrades
		totalPnL += result.TotalPnL
	}

	// Print individual results
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Printf("%-14s %7s %8s %8s %10s %8s %8s %10s\n",
		"Symbol", "Trades", "WinRate", "PF", "Return%", "MaxDD%", "Sharpe", "Final$")
	fmt.Println("───────────────────────────────────────────────────────────────────────────────")

	for _, r := range results {
		winRateStr := fmt.Sprintf("%.1f%%", r.WinRate)
		pfStr := fmt.Sprintf("%.2f", r.ProfitFactor)
		if r.ProfitFactor == 0 || r.TotalTrades == 0 {
			pfStr = "N/A"
		}

		returnColor := ""
		if r.TotalReturn > 0 {
			returnColor = "\033[32m" // Green
		} else if r.TotalReturn < 0 {
			returnColor = "\033[31m" // Red
		}
		resetColor := "\033[0m"

		fmt.Printf("%-14s %7d %8s %8s %s%+9.2f%%%s %7.1f%% %8.2f %10.2f\n",
			r.Symbol,
			r.TotalTrades,
			winRateStr,
			pfStr,
			returnColor,
			r.TotalReturn,
			resetColor,
			r.MaxDrawdown,
			r.SharpeRatio,
			r.FinalBalance)
	}

	// Sort by return for ranking
	sort.Slice(results, func(i, j int) bool {
		return results[i].TotalReturn > results[j].TotalReturn
	})

	// Portfolio summary
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("  PORTFOLIO SUMMARY (12 coins, $1000 each = $12,000 initial)")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")

	var totalFinal float64
	var totalWins, totalLosses int
	var sumSharpe float64
	profitableCount := 0

	for _, r := range results {
		totalFinal += r.FinalBalance
		if r.TotalReturn > 0 {
			profitableCount++
		}
		if r.WinRate > 0 {
			totalWins += int(float64(r.TotalTrades) * r.WinRate / 100)
			totalLosses += r.TotalTrades - int(float64(r.TotalTrades)*r.WinRate/100)
		}
		sumSharpe += r.SharpeRatio
	}

	initialPortfolio := float64(len(results)) * 1000
	portfolioReturn := (totalFinal - initialPortfolio) / initialPortfolio * 100
	avgSharpe := sumSharpe / float64(len(results))

	fmt.Printf("  Initial Capital:     $%.2f\n", initialPortfolio)
	fmt.Printf("  Final Capital:       $%.2f\n", totalFinal)
	fmt.Printf("  Total Return:        %+.2f%%\n", portfolioReturn)
	fmt.Printf("  Total Trades:        %d\n", allTrades)
	fmt.Printf("  Overall Win Rate:    %.1f%%\n", float64(totalWins)/float64(totalWins+totalLosses)*100)
	fmt.Printf("  Average Sharpe:      %.2f\n", avgSharpe)
	fmt.Printf("  Profitable Symbols:  %d/%d\n", profitableCount, len(results))

	// Top/Bottom performers
	fmt.Println()
	fmt.Println("  🏆 TOP 3 PERFORMERS:")
	for i := 0; i < 3 && i < len(results); i++ {
		fmt.Printf("     %d. %s: %+.2f%% (Sharpe: %.2f)\n",
			i+1, results[i].Symbol, results[i].TotalReturn, results[i].SharpeRatio)
	}

	fmt.Println()
	fmt.Println("  ⚠️  BOTTOM 3 PERFORMERS:")
	for i := len(results) - 1; i >= len(results)-3 && i >= 0; i-- {
		fmt.Printf("     %d. %s: %+.2f%% (Sharpe: %.2f)\n",
			len(results)-i, results[i].Symbol, results[i].TotalReturn, results[i].SharpeRatio)
	}

	// Strategy recommendations
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")
	fmt.Println("  ANALYSIS & RECOMMENDATIONS")
	fmt.Println("═══════════════════════════════════════════════════════════════════════════════")

	if portfolioReturn > 0 {
		fmt.Println("  ✅ Strategy is PROFITABLE on historical data")
	} else {
		fmt.Println("  ❌ Strategy is NOT PROFITABLE - needs optimization")
	}

	if avgSharpe > 1 {
		fmt.Println("  ✅ Good risk-adjusted returns (Sharpe > 1)")
	} else if avgSharpe > 0.5 {
		fmt.Println("  ⚠️  Moderate risk-adjusted returns (Sharpe 0.5-1)")
	} else {
		fmt.Println("  ❌ Poor risk-adjusted returns (Sharpe < 0.5)")
	}

	// Save results to JSON
	saveResults(results, portfolioReturn, avgSharpe)

	fmt.Println()
	fmt.Println("  Results saved to: backtest_results.json")
	fmt.Println()
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
		close, _ := strconv.ParseFloat(record[4], 64)
		volume, _ := strconv.ParseFloat(record[5], 64)

		candles = append(candles, model.Candle{
			OpenTime:  t,
			CloseTime: t.Add(4 * time.Hour),
			Open:      open,
			High:      high,
			Low:       low,
			Close:     close,
			Volume:    volume,
		})
	}

	return candles, nil
}

func saveResults(results []SymbolResult, portfolioReturn, avgSharpe float64) {
	output := map[string]interface{}{
		"timestamp":        time.Now().Format(time.RFC3339),
		"strategy":         "MultiFactor(9/21)",
		"initial_per_coin": 1000,
		"total_initial":    len(results) * 1000,
		"portfolio_return": portfolioReturn,
		"avg_sharpe":       avgSharpe,
		"results":          results,
	}

	data, _ := json.MarshalIndent(output, "", "  ")
	os.WriteFile("backtest_results.json", data, 0644)
}

func formatSymbol(s string) string {
	return strings.TrimSuffix(s, "USDT")
}
