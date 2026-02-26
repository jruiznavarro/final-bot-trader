package main

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"final-bot-trader-api/internal/backtest"
	"final-bot-trader-api/internal/exchange/model"
	"final-bot-trader-api/internal/strategy"
	"final-bot-trader-api/internal/strategy/multifactor"
)

// Optimized symbol list (removed LINKUSDT which had -10.20%)
var optimizedSymbols = []string{
	"DOGEUSDT",      // Best: +22.77%, Sharpe: 2.47
	"WLDUSDT",       // +19.19%, Sharpe: 1.86
	"1000PEPEUSDT",  // +14.08%, Sharpe: 1.09
	"ARBUSDT",       // +10.35%, Sharpe: 1.04
	"AAVEUSDT",      // +9.84%, Sharpe: 1.44
	"WIFUSDT",       // +8.69%, Sharpe: 0.81
	"FILUSDT",       // +4.56%, Sharpe: 0.69
	"SOLUSDT",       // +2.54%, Sharpe: 0.42
	"TAOUSDT",       // +2.33%, Sharpe: 0.13
	"SUIUSDT",       // +2.07%, Sharpe: 0.63
}

type ParamSet struct {
	FastEMA       int
	SlowEMA       int
	RSIPeriod     int
	ATRStopMult   float64
	ATRTargetMult float64
	MinADX        float64
}

type OptimResult struct {
	Params         ParamSet
	PortfolioReturn float64
	AvgSharpe       float64
	WinRate         float64
	MaxDrawdown     float64
	TotalTrades     int
	ProfitableSyms  int
}

func main() {
	dataDir := "data/historical"

	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println("  STRATEGY OPTIMIZATION")
	fmt.Println("  Walk-Forward Analysis on 10 symbols")
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println()

	// Load all data
	symbolCandles := make(map[string][]model.Candle)
	for _, symbol := range optimizedSymbols {
		filename := filepath.Join(dataDir, symbol+"_4h.csv")
		candles, err := loadCSV(filename)
		if err != nil {
			fmt.Printf("Error loading %s: %v\n", symbol, err)
			continue
		}
		symbolCandles[symbol] = candles
	}

	// Parameter grid (reduced for efficiency)
	paramSets := []ParamSet{
		// Baseline
		{9, 21, 14, 1.5, 2.5, 20},
		// Faster EMAs
		{5, 13, 14, 1.5, 2.5, 20},
		{7, 17, 14, 1.5, 2.5, 20},
		// Slower EMAs
		{12, 26, 14, 1.5, 2.5, 20},
		{13, 34, 14, 1.5, 2.5, 20},
		// Different R:R ratios
		{9, 21, 14, 1.5, 3.0, 20}, // 1:2 R:R
		{9, 21, 14, 1.0, 3.0, 20}, // 1:3 R:R (tighter SL)
		{9, 21, 14, 2.0, 4.0, 20}, // 1:2 R:R (wider)
		// Different ADX thresholds
		{9, 21, 14, 1.5, 2.5, 15}, // More trades
		{9, 21, 14, 1.5, 2.5, 25}, // Fewer but stronger trends
		// RSI variations
		{9, 21, 10, 1.5, 2.5, 20},
		{9, 21, 21, 1.5, 2.5, 20},
		// Best combo attempts
		{7, 21, 14, 1.5, 3.0, 20},
		{9, 26, 14, 1.0, 2.5, 25},
	}

	// Bitunix costs
	engineConfig := backtest.EngineConfig{
		InitialBalance: 1000,
		Commission:     0.0005, // 0.05% taker
		Slippage:       0.0003, // 0.03% slippage
		Verbose:        false,
	}

	riskConfig := strategy.DefaultRiskConfig()
	riskConfig.MaxRiskPerTrade = 0.02
	riskConfig.MaxPositionSize = 0.20
	rm, _ := strategy.NewRiskManager(riskConfig)
	engineConfig.RiskManager = rm

	engine, _ := backtest.NewEngine(engineConfig)

	var results []OptimResult

	fmt.Printf("Testing %d parameter combinations...\n\n", len(paramSets))

	for i, params := range paramSets {
		config := multifactor.Config{
			FastEMA:         params.FastEMA,
			SlowEMA:         params.SlowEMA,
			RSIPeriod:       params.RSIPeriod,
			RSIOverbought:   70,
			RSIOversold:     30,
			VolumePeriod:    20,
			VolumeThreshold: 1.0,
			ATRPeriod:       14,
			ATRStopMult:     params.ATRStopMult,
			ATRTargetMult:   params.ATRTargetMult,
			RequireTrend:    true,
			MinADX:          params.MinADX,
		}

		var totalReturn, totalSharpe, maxDD float64
		var totalTrades, profitSyms int
		var winCount, totalCount int

		for _, symbol := range optimizedSymbols {
			candles := symbolCandles[symbol]
			if len(candles) == 0 {
				continue
			}

			strat := multifactor.NewMultiFactorStrategy(symbol, config)
			result, err := engine.Run(strat, candles, symbol, "4h")
			if err != nil {
				continue
			}

			totalReturn += result.TotalReturn
			totalSharpe += result.SharpeRatio
			totalTrades += result.TotalTrades
			if result.MaxDrawdownPct > maxDD {
				maxDD = result.MaxDrawdownPct
			}
			if result.TotalReturn > 0 {
				profitSyms++
			}
			winCount += result.WinningTrades
			totalCount += result.TotalTrades
		}

		numSyms := float64(len(optimizedSymbols))
		portfolioReturn := totalReturn / numSyms
		avgSharpe := totalSharpe / numSyms
		winRate := 0.0
		if totalCount > 0 {
			winRate = float64(winCount) / float64(totalCount) * 100
		}

		results = append(results, OptimResult{
			Params:          params,
			PortfolioReturn: portfolioReturn,
			AvgSharpe:       avgSharpe,
			WinRate:         winRate,
			MaxDrawdown:     maxDD,
			TotalTrades:     totalTrades,
			ProfitableSyms:  profitSyms,
		})

		fmt.Printf("[%2d/%d] EMA(%d/%d) RSI:%d ATR:%.1f/%.1f ADX:%.0f => Return: %+6.2f%%, Sharpe: %.2f, Trades: %d\n",
			i+1, len(paramSets),
			params.FastEMA, params.SlowEMA, params.RSIPeriod,
			params.ATRStopMult, params.ATRTargetMult, params.MinADX,
			portfolioReturn, avgSharpe, totalTrades)
	}

	// Sort by Sharpe ratio
	sort.Slice(results, func(i, j int) bool {
		return results[i].AvgSharpe > results[j].AvgSharpe
	})

	fmt.Println()
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println("  TOP 5 PARAMETER SETS (by Sharpe Ratio)")
	fmt.Println("══════════════════════════════════════════════════════════════")

	fmt.Printf("\n%-5s %-12s %-8s %-10s %-10s %-8s %-8s %-8s\n",
		"Rank", "EMA", "RSI", "ATR(SL/TP)", "MinADX", "Return%", "Sharpe", "Trades")
	fmt.Println("─────────────────────────────────────────────────────────────────────────")

	for i := 0; i < 5 && i < len(results); i++ {
		r := results[i]
		fmt.Printf("%-5d %-12s %-8d %-10s %-10.0f %+7.2f%% %8.2f %8d\n",
			i+1,
			fmt.Sprintf("%d/%d", r.Params.FastEMA, r.Params.SlowEMA),
			r.Params.RSIPeriod,
			fmt.Sprintf("%.1f/%.1f", r.Params.ATRStopMult, r.Params.ATRTargetMult),
			r.Params.MinADX,
			r.PortfolioReturn,
			r.AvgSharpe,
			r.TotalTrades)
	}

	// Best parameters
	best := results[0]
	fmt.Println()
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println("  BEST PARAMETERS FOUND")
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Printf(`
  FastEMA:        %d
  SlowEMA:        %d
  RSI Period:     %d
  ATR SL Mult:    %.1f
  ATR TP Mult:    %.1f
  Min ADX:        %.0f

  Expected Results:
  ─────────────────
  Portfolio Return: %+.2f%%
  Average Sharpe:   %.2f
  Win Rate:         %.1f%%
  Max Drawdown:     %.1f%%
  Total Trades:     %d
  Profitable Syms:  %d/%d
`,
		best.Params.FastEMA, best.Params.SlowEMA, best.Params.RSIPeriod,
		best.Params.ATRStopMult, best.Params.ATRTargetMult, best.Params.MinADX,
		best.PortfolioReturn, best.AvgSharpe, best.WinRate,
		best.MaxDrawdown, best.TotalTrades, best.ProfitableSyms, len(optimizedSymbols))

	// Walk-forward validation on best params
	fmt.Println()
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println("  WALK-FORWARD VALIDATION (Best Parameters)")
	fmt.Println("  Training: First 70% | Testing: Last 30%")
	fmt.Println("══════════════════════════════════════════════════════════════")

	walkForwardValidation(engine, symbolCandles, best.Params)
}

func walkForwardValidation(engine *backtest.Engine, symbolCandles map[string][]model.Candle, params ParamSet) {
	config := multifactor.Config{
		FastEMA:         params.FastEMA,
		SlowEMA:         params.SlowEMA,
		RSIPeriod:       params.RSIPeriod,
		RSIOverbought:   70,
		RSIOversold:     30,
		VolumePeriod:    20,
		VolumeThreshold: 1.0,
		ATRPeriod:       14,
		ATRStopMult:     params.ATRStopMult,
		ATRTargetMult:   params.ATRTargetMult,
		RequireTrend:    true,
		MinADX:          params.MinADX,
	}

	var trainReturn, testReturn float64
	var trainSharpe, testSharpe float64

	fmt.Printf("\n%-14s %12s %12s %12s %12s\n", "Symbol", "Train Ret%", "Test Ret%", "Train Shrp", "Test Shrp")
	fmt.Println("─────────────────────────────────────────────────────────────────────")

	for _, symbol := range optimizedSymbols {
		candles := symbolCandles[symbol]
		if len(candles) < 100 {
			continue
		}

		// Split 70/30
		splitIdx := int(float64(len(candles)) * 0.70)
		trainCandles := candles[:splitIdx]
		testCandles := candles[splitIdx:]

		strat := multifactor.NewMultiFactorStrategy(symbol, config)

		// Train period
		trainResult, err := engine.Run(strat, trainCandles, symbol, "4h")
		if err != nil {
			continue
		}

		// Test period (out-of-sample)
		testResult, err := engine.Run(strat, testCandles, symbol, "4h")
		if err != nil {
			continue
		}

		trainReturn += trainResult.TotalReturn
		testReturn += testResult.TotalReturn
		trainSharpe += trainResult.SharpeRatio
		testSharpe += testResult.SharpeRatio

		fmt.Printf("%-14s %+11.2f%% %+11.2f%% %12.2f %12.2f\n",
			symbol, trainResult.TotalReturn, testResult.TotalReturn,
			trainResult.SharpeRatio, testResult.SharpeRatio)
	}

	numSyms := float64(len(optimizedSymbols))
	fmt.Println("─────────────────────────────────────────────────────────────────────")
	fmt.Printf("%-14s %+11.2f%% %+11.2f%% %12.2f %12.2f\n",
		"AVERAGE", trainReturn/numSyms, testReturn/numSyms,
		trainSharpe/numSyms, testSharpe/numSyms)

	fmt.Println()
	if testReturn/numSyms > 0 {
		fmt.Println("  ✅ Strategy is profitable on OUT-OF-SAMPLE data!")
		fmt.Println("     This suggests the strategy is NOT overfitted.")
	} else {
		fmt.Println("  ⚠️  Strategy underperforms on out-of-sample data.")
		fmt.Println("     Consider using more conservative parameters.")
	}

	degradation := (trainReturn/numSyms - testReturn/numSyms) / (trainReturn/numSyms) * 100
	if degradation < 30 {
		fmt.Printf("  ✅ Performance degradation: %.1f%% (acceptable < 30%%)\n", degradation)
	} else {
		fmt.Printf("  ⚠️  Performance degradation: %.1f%% (concerning > 30%%)\n", degradation)
	}
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
		if i == 0 {
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
