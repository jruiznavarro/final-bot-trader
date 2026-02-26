package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"final-bot-trader-api/internal/backtest"
	"final-bot-trader-api/internal/exchange"
	"final-bot-trader-api/internal/exchange/model"
	"final-bot-trader-api/internal/strategy"

	"github.com/joho/godotenv"
)

// Default symbols to test
var defaultSymbols = []string{
	"BTCUSDT",
	"ETHUSDT",
	"SOLUSDT",
	"BNBUSDT",
	"XRPUSDT",
	"ADAUSDT",
	"DOGEUSDT",
	"AVAXUSDT",
	"DOTUSDT",
	"LINKUSDT",
}

// StrategyResult holds results for a single strategy run
type StrategyResult struct {
	Symbol         string
	Strategy       string
	Interval       string
	TotalTrades    int
	WinRate        float64
	TotalReturn    float64
	MaxDrawdown    float64
	SharpeRatio    float64
	ProfitFactor   float64
	Parameters     map[string]interface{}
	CandlesUsed    int
	StartDate      time.Time
	EndDate        time.Time
}

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Parse flags
	symbolsFlag := flag.String("symbols", "", "Comma-separated list of symbols (default: top 10 cryptos)")
	interval := flag.String("interval", "4h", "Candle interval (1h, 4h, 1d)")
	years := flag.Float64("years", 1, "Years of historical data to fetch (max 3)")
	dataDir := flag.String("datadir", "./data", "Directory to store/cache historical data")
	outputFile := flag.String("output", "backtest_results.csv", "Output CSV file for results")
	initialBalance := flag.Float64("balance", 10000, "Initial balance for backtesting")
	downloadOnly := flag.Bool("download-only", false, "Only download data, don't run backtests")
	useCache := flag.Bool("cache", true, "Use cached data if available")
	strategyFilter := flag.String("strategy", "", "Run only specific strategy (sma, rsi, confluence, adaptive)")

	flag.Parse()

	// Validate years
	if *years > 3 {
		*years = 3
		log.Println("Warning: Maximum 3 years of data, limiting to 3")
	}
	if *years < 0.1 {
		*years = 0.1
	}

	// Parse symbols
	var symbols []string
	if *symbolsFlag != "" {
		symbols = strings.Split(*symbolsFlag, ",")
		for i := range symbols {
			symbols[i] = strings.TrimSpace(strings.ToUpper(symbols[i]))
		}
	} else {
		symbols = defaultSymbols
	}

	// Create data directory
	if err := os.MkdirAll(*dataDir, 0755); err != nil {
		log.Fatalf("Failed to create data directory: %v", err)
	}

	// Create client
	apiKey := os.Getenv("BITUNIX_API_KEY")
	secretKey := os.Getenv("BITUNIX_SECRET_KEY")
	baseURL := os.Getenv("BITUNIX_BASE_URL")
	client := exchange.NewBitunixClient(apiKey, secretKey, baseURL)

	ctx := context.Background()

	fmt.Println("==============================================")
	fmt.Println("       MASS BACKTESTING TOOL")
	fmt.Println("==============================================")
	fmt.Printf("Symbols: %v\n", symbols)
	fmt.Printf("Interval: %s\n", *interval)
	fmt.Printf("Years: %.1f\n", *years)
	fmt.Printf("Initial Balance: $%.2f\n", *initialBalance)
	fmt.Println("==============================================\n")

	// Download historical data for all symbols
	allCandles := make(map[string][]model.Candle)

	for _, symbol := range symbols {
		fmt.Printf("\n[%s] Fetching historical data...\n", symbol)

		candles, err := fetchHistoricalData(ctx, client, symbol, *interval, *years, *dataDir, *useCache)
		if err != nil {
			log.Printf("[%s] Error fetching data: %v", symbol, err)
			continue
		}

		allCandles[symbol] = candles
		fmt.Printf("[%s] Loaded %d candles (%s to %s)\n",
			symbol, len(candles),
			candles[0].OpenTime.Format("2006-01-02"),
			candles[len(candles)-1].OpenTime.Format("2006-01-02"))
	}

	if *downloadOnly {
		fmt.Println("\nDownload complete. Exiting (--download-only mode)")
		return
	}

	// Run backtests
	fmt.Println("\n==============================================")
	fmt.Println("       RUNNING BACKTESTS")
	fmt.Println("==============================================")

	var results []StrategyResult

	for symbol, candles := range allCandles {
		if len(candles) < 100 {
			log.Printf("[%s] Insufficient data (%d candles), skipping", symbol, len(candles))
			continue
		}

		fmt.Printf("\n[%s] Running backtests with %d candles...\n", symbol, len(candles))

		// Run each strategy
		stratResults := runAllStrategies(symbol, candles, *interval, *initialBalance, *strategyFilter)
		results = append(results, stratResults...)
	}

	// Sort results by total return
	sort.Slice(results, func(i, j int) bool {
		return results[i].TotalReturn > results[j].TotalReturn
	})

	// Print summary
	printSummary(results)

	// Save to CSV
	if err := saveResultsToCSV(results, *outputFile); err != nil {
		log.Printf("Error saving results: %v", err)
	} else {
		fmt.Printf("\nResults saved to: %s\n", *outputFile)
	}
}

func fetchHistoricalData(ctx context.Context, client *exchange.BitunixClient, symbol, interval string, years float64, dataDir string, useCache bool) ([]model.Candle, error) {
	// Check cache first
	cacheFile := filepath.Join(dataDir, fmt.Sprintf("%s_%s_%.1fy.json", symbol, interval, years))

	if useCache {
		if candles, err := loadFromCache(cacheFile); err == nil {
			fmt.Printf("[%s] Loaded from cache: %d candles\n", symbol, len(candles))
			return candles, nil
		}
	}

	// Calculate time range
	endTime := time.Now()
	startTime := endTime.Add(-time.Duration(years*365*24) * time.Hour)

	// Calculate how many candles we need
	intervalDuration := parseIntervalDuration(interval)
	totalCandles := int(endTime.Sub(startTime) / intervalDuration)

	fmt.Printf("[%s] Need ~%d candles from %s to %s\n",
		symbol, totalCandles,
		startTime.Format("2006-01-02"),
		endTime.Format("2006-01-02"))

	// Fetch in batches (Bitunix limit is typically 1000 per request)
	const batchSize = 500
	var allCandles []model.Candle

	currentEnd := endTime
	retries := 0
	maxRetries := 3

	for currentEnd.After(startTime) {
		// Calculate batch start time
		batchStart := currentEnd.Add(-time.Duration(batchSize) * intervalDuration)
		if batchStart.Before(startTime) {
			batchStart = startTime
		}

		// Fetch batch
		candles, err := client.GetKlines(ctx, symbol, interval, batchSize,
			batchStart.UnixMilli(), currentEnd.UnixMilli())

		if err != nil {
			retries++
			if retries >= maxRetries {
				log.Printf("[%s] Failed after %d retries: %v", symbol, maxRetries, err)
				break
			}
			log.Printf("[%s] Retry %d/%d after error: %v", symbol, retries, maxRetries, err)
			time.Sleep(time.Duration(retries*2) * time.Second)
			continue
		}

		retries = 0

		if len(candles) == 0 {
			log.Printf("[%s] No more data available", symbol)
			break
		}

		// Prepend to maintain chronological order
		allCandles = append(candles, allCandles...)

		// Move window back
		currentEnd = candles[0].OpenTime.Add(-intervalDuration)

		fmt.Printf("[%s] Fetched %d candles, total: %d (until %s)\n",
			symbol, len(candles), len(allCandles), currentEnd.Format("2006-01-02"))

		// Rate limiting - wait between requests
		time.Sleep(200 * time.Millisecond)
	}

	if len(allCandles) == 0 {
		return nil, fmt.Errorf("no data fetched for %s", symbol)
	}

	// Remove duplicates and sort
	allCandles = deduplicateCandles(allCandles)

	// Save to cache
	if err := saveToCache(cacheFile, allCandles); err != nil {
		log.Printf("[%s] Warning: Failed to cache data: %v", symbol, err)
	}

	return allCandles, nil
}

func deduplicateCandles(candles []model.Candle) []model.Candle {
	seen := make(map[int64]bool)
	var result []model.Candle

	for _, c := range candles {
		key := c.OpenTime.UnixMilli()
		if !seen[key] {
			seen[key] = true
			result = append(result, c)
		}
	}

	// Sort by time
	sort.Slice(result, func(i, j int) bool {
		return result[i].OpenTime.Before(result[j].OpenTime)
	})

	return result
}

func loadFromCache(filename string) ([]model.Candle, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, err
	}

	var candles []model.Candle
	if err := json.Unmarshal(data, &candles); err != nil {
		return nil, err
	}

	return candles, nil
}

func saveToCache(filename string, candles []model.Candle) error {
	data, err := json.Marshal(candles)
	if err != nil {
		return err
	}
	return os.WriteFile(filename, data, 0644)
}

func parseIntervalDuration(interval string) time.Duration {
	switch interval {
	case "1m":
		return time.Minute
	case "5m":
		return 5 * time.Minute
	case "15m":
		return 15 * time.Minute
	case "30m":
		return 30 * time.Minute
	case "1h":
		return time.Hour
	case "2h":
		return 2 * time.Hour
	case "4h":
		return 4 * time.Hour
	case "6h":
		return 6 * time.Hour
	case "12h":
		return 12 * time.Hour
	case "1d":
		return 24 * time.Hour
	default:
		return time.Hour
	}
}

func runAllStrategies(symbol string, candles []model.Candle, interval string, initialBalance float64, strategyFilter string) []StrategyResult {
	var results []StrategyResult

	// SMA Crossover variations
	if strategyFilter == "" || strategyFilter == "sma" {
		smaConfigs := []struct {
			short, long int
		}{
			{5, 20},
			{10, 30},
			{10, 50},
			{20, 50},
			{20, 100},
			{50, 200},
		}

		for _, cfg := range smaConfigs {
			strat, err := strategy.NewSMACrossover(symbol, cfg.short, cfg.long)
			if err != nil {
				continue
			}

			result := runBacktest(strat, candles, symbol, interval, initialBalance)
			if result != nil {
				result.Parameters = map[string]interface{}{
					"short_period": cfg.short,
					"long_period":  cfg.long,
				}
				results = append(results, *result)
			}
		}
	}

	// RSI variations
	if strategyFilter == "" || strategyFilter == "rsi" {
		rsiConfigs := []struct {
			period                   int
			overbought, oversold     float64
		}{
			{14, 70, 30},
			{14, 75, 25},
			{14, 65, 35},
			{7, 70, 30},
			{21, 70, 30},
		}

		for _, cfg := range rsiConfigs {
			strat, err := strategy.NewRSIStrategyWithLevels(symbol, cfg.period, cfg.overbought, cfg.oversold)
			if err != nil {
				continue
			}

			result := runBacktest(strat, candles, symbol, interval, initialBalance)
			if result != nil {
				result.Parameters = map[string]interface{}{
					"period":     cfg.period,
					"overbought": cfg.overbought,
					"oversold":   cfg.oversold,
				}
				results = append(results, *result)
			}
		}
	}

	// Confluence strategy variations
	if strategyFilter == "" || strategyFilter == "confluence" {
		confluenceConfigs := []struct {
			name   string
			config strategy.ConfluenceConfig
		}{
			{"Default", strategy.DefaultConfluenceConfig()},
			{"Aggressive", strategy.AggressiveConfluenceConfig()},
			{"Conservative", strategy.ConservativeConfluenceConfig()},
		}

		for _, cfg := range confluenceConfigs {
			strat, err := strategy.NewConfluenceStrategy(symbol, cfg.config)
			if err != nil {
				continue
			}

			result := runBacktest(strat, candles, symbol, interval, initialBalance)
			if result != nil {
				result.Strategy = fmt.Sprintf("Confluence_%s", cfg.name)
				result.Parameters = map[string]interface{}{
					"variant":        cfg.name,
					"min_confluence": cfg.config.MinConfluenceScore,
				}
				results = append(results, *result)
			}
		}
	}

	// Adaptive strategy
	if strategyFilter == "" || strategyFilter == "adaptive" {
		strat, err := strategy.NewAdaptiveStrategy(symbol, strategy.DefaultAdaptiveConfig())
		if err == nil {
			result := runBacktest(strat, candles, symbol, interval, initialBalance)
			if result != nil {
				results = append(results, *result)
			}
		}
	}

	return results
}

func runBacktest(strat strategy.Strategy, candles []model.Candle, symbol, interval string, initialBalance float64) *StrategyResult {
	if len(candles) < strat.MinimumCandles() {
		return nil
	}

	config := backtest.DefaultEngineConfig()
	config.InitialBalance = initialBalance

	engine, err := backtest.NewEngine(config)
	if err != nil {
		return nil
	}

	result, err := engine.Run(strat, candles, symbol, interval)
	if err != nil {
		log.Printf("  [%s] %s failed: %v", symbol, strat.Name(), err)
		return nil
	}

	return &StrategyResult{
		Symbol:       symbol,
		Strategy:     strat.Name(),
		Interval:     interval,
		TotalTrades:  result.TotalTrades,
		WinRate:      result.WinRate,
		TotalReturn:  result.TotalReturn,
		MaxDrawdown:  result.MaxDrawdownPct,
		SharpeRatio:  result.SharpeRatio,
		ProfitFactor: result.ProfitFactor,
		CandlesUsed:  len(candles),
		StartDate:    result.StartDate,
		EndDate:      result.EndDate,
	}
}

func printSummary(results []StrategyResult) {
	fmt.Println("\n==============================================")
	fmt.Println("       BACKTEST RESULTS SUMMARY")
	fmt.Println("==============================================")
	fmt.Println("")

	if len(results) == 0 {
		fmt.Println("No results to display.")
		return
	}

	// Top 10 by return
	fmt.Println("TOP 10 BY TOTAL RETURN:")
	fmt.Println("------------------------")
	fmt.Printf("%-12s %-30s %10s %8s %10s %8s\n",
		"Symbol", "Strategy", "Return%", "WinRate", "Sharpe", "Trades")
	fmt.Println(strings.Repeat("-", 82))

	count := 10
	if len(results) < count {
		count = len(results)
	}

	for i := 0; i < count; i++ {
		r := results[i]
		fmt.Printf("%-12s %-30s %9.2f%% %7.1f%% %9.2f %8d\n",
			r.Symbol, truncate(r.Strategy, 30), r.TotalReturn, r.WinRate, r.SharpeRatio, r.TotalTrades)
	}

	// Best by symbol
	fmt.Println("\n\nBEST STRATEGY PER SYMBOL:")
	fmt.Println("--------------------------")

	bestBySymbol := make(map[string]*StrategyResult)
	for i := range results {
		r := &results[i]
		if best, exists := bestBySymbol[r.Symbol]; !exists || r.TotalReturn > best.TotalReturn {
			bestBySymbol[r.Symbol] = r
		}
	}

	var symbols []string
	for s := range bestBySymbol {
		symbols = append(symbols, s)
	}
	sort.Strings(symbols)

	for _, sym := range symbols {
		r := bestBySymbol[sym]
		fmt.Printf("%-12s: %-30s (%.2f%% return, %.1f%% win rate)\n",
			sym, r.Strategy, r.TotalReturn, r.WinRate)
	}

	// Strategy comparison
	fmt.Println("\n\nAVERAGE PERFORMANCE BY STRATEGY TYPE:")
	fmt.Println("--------------------------------------")

	stratStats := make(map[string]struct {
		count       int
		totalReturn float64
		winRate     float64
	})

	for _, r := range results {
		stratType := extractStrategyType(r.Strategy)
		stats := stratStats[stratType]
		stats.count++
		stats.totalReturn += r.TotalReturn
		stats.winRate += r.WinRate
		stratStats[stratType] = stats
	}

	for stratType, stats := range stratStats {
		avgReturn := stats.totalReturn / float64(stats.count)
		avgWinRate := stats.winRate / float64(stats.count)
		fmt.Printf("%-20s: Avg Return: %7.2f%%, Avg Win Rate: %5.1f%% (n=%d)\n",
			stratType, avgReturn, avgWinRate, stats.count)
	}
}

func extractStrategyType(stratName string) string {
	if strings.Contains(stratName, "SMA") {
		return "SMA_Crossover"
	}
	if strings.Contains(stratName, "RSI") {
		return "RSI"
	}
	if strings.Contains(stratName, "Confluence") {
		return "Confluence"
	}
	if strings.Contains(stratName, "Adaptive") {
		return "Adaptive"
	}
	return stratName
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func saveResultsToCSV(results []StrategyResult, filename string) error {
	file, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer file.Close()

	writer := csv.NewWriter(file)
	defer writer.Flush()

	// Header
	header := []string{
		"Symbol", "Strategy", "Interval", "TotalTrades", "WinRate",
		"TotalReturn", "MaxDrawdown", "SharpeRatio", "ProfitFactor",
		"CandlesUsed", "StartDate", "EndDate", "Parameters",
	}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Data rows
	for _, r := range results {
		paramsJSON, _ := json.Marshal(r.Parameters)
		row := []string{
			r.Symbol,
			r.Strategy,
			r.Interval,
			fmt.Sprintf("%d", r.TotalTrades),
			fmt.Sprintf("%.2f", r.WinRate),
			fmt.Sprintf("%.2f", r.TotalReturn),
			fmt.Sprintf("%.2f", r.MaxDrawdown),
			fmt.Sprintf("%.2f", r.SharpeRatio),
			fmt.Sprintf("%.2f", r.ProfitFactor),
			fmt.Sprintf("%d", r.CandlesUsed),
			r.StartDate.Format("2006-01-02"),
			r.EndDate.Format("2006-01-02"),
			string(paramsJSON),
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}
