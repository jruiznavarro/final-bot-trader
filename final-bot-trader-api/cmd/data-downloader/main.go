package main

import (
	"context"
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"final-bot-trader-api/internal/exchange"
	"final-bot-trader-api/internal/exchange/model"
)

// Symbol categories for analysis
var symbolCategories = map[string]string{
	// Layer 1 - Major
	"BTCUSDT":  "L1-Major",
	"ETHUSDT":  "L1-Major",
	"SOLUSDT":  "L1-Major",
	"ADAUSDT":  "L1-Major",
	"AVAXUSDT": "L1-Major",
	"NEARUSDT": "L1-Major",
	"SUIUSDT":  "L1-Major",
	"APTUSDT":  "L1-Major",

	// Layer 1 - Alt
	"XRPUSDT":  "L1-Alt",
	"DOGEUSDT": "L1-Alt",
	"LTCUSDT":  "L1-Alt",
	"BCHUSDT":  "L1-Alt",
	"ETCUSDT":  "L1-Alt",
	"XLMUSDT":  "L1-Alt",
	"TRXUSDT":  "L1-Alt",
	"HBARUSDT": "L1-Alt",

	// DeFi
	"LINKUSDT": "DeFi",
	"UNIUSDT":  "DeFi",
	"AAVEUSDT": "DeFi",
	"CRVUSDT":  "DeFi",
	"ENAUSDT":  "DeFi",

	// Infrastructure
	"FILUSDT": "Infra",
	"ICPUSDT": "Infra",
	"ARBUSDT": "L2",
	"DOTUSDT": "Infra",

	// Meme - Established
	"1000PEPEUSDT": "Meme",
	"1000SHIBUSDT": "Meme",
	"1000BONKUSDT": "Meme",
	"WIFUSDT":      "Meme",

	// Gaming/Metaverse
	"AXSUSDT":   "Gaming",
	"GALAUSDT":  "Gaming",
	"WLDUSDT":   "AI",
	"TAOUSDT":   "AI",
	"ONDOUSDT":  "RWA",
	"HYPEUSDT":  "New",
	"TRUMPUSDT": "Meme-Political",
}

// Selected symbols for comprehensive analysis
var analysisSymbols = []string{
	// Tier 1: Highest volume, most stable
	"BTCUSDT",
	"ETHUSDT",
	"SOLUSDT",
	"XRPUSDT",

	// Tier 2: High volume, established
	"DOGEUSDT",
	"LINKUSDT",
	"SUIUSDT",
	"ADAUSDT",
	"AVAXUSDT",
	"LTCUSDT",

	// Tier 3: Medium volume, diverse categories
	"AAVEUSDT",
	"NEARUSDT",
	"ICPUSDT",
	"HBARUSDT",
	"FILUSDT",
	"BCHUSDT",
	"DOTUSDT",
	"UNIUSDT",

	// Tier 4: Meme coins (higher volatility potential)
	"1000PEPEUSDT",
	"1000SHIBUSDT",
	"WIFUSDT",
	"1000BONKUSDT",

	// Tier 5: Newer/Trending
	"ARBUSDT",
	"ONDOUSDT",
	"WLDUSDT",
	"ENAUSDT",
	"TAOUSDT",
	"APTUSDT",
	"HYPEUSDT",
	"TRUMPUSDT",
}

func main() {
	outputDir := flag.String("output", "data/historical", "Output directory for CSV files")
	interval := flag.String("interval", "4h", "Candle interval")
	years := flag.Int("years", 2, "Years of history to download")
	flag.Parse()

	apiKey := os.Getenv("BITUNIX_API_KEY")
	secretKey := os.Getenv("BITUNIX_SECRET_KEY")
	baseURL := os.Getenv("BITUNIX_BASE_URL")

	if apiKey == "" || secretKey == "" {
		log.Fatal("BITUNIX_API_KEY and BITUNIX_SECRET_KEY required")
	}

	client := exchange.NewBitunixClient(apiKey, secretKey, baseURL)
	ctx := context.Background()

	// Create output directory
	if err := os.MkdirAll(*outputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	fmt.Println("==============================================")
	fmt.Println("  HISTORICAL DATA DOWNLOADER")
	fmt.Println("==============================================")
	fmt.Printf("Symbols:   %d\n", len(analysisSymbols))
	fmt.Printf("Interval:  %s\n", *interval)
	fmt.Printf("History:   %d years\n", *years)
	fmt.Printf("Output:    %s\n", *outputDir)
	fmt.Println("==============================================\n")

	endTime := time.Now()
	startTime := endTime.AddDate(-*years, 0, 0)

	summary := make(map[string]int)
	errors := make(map[string]string)

	for i, symbol := range analysisSymbols {
		category := symbolCategories[symbol]
		if category == "" {
			category = "Unknown"
		}

		fmt.Printf("[%d/%d] %s (%s)... ", i+1, len(analysisSymbols), symbol, category)

		candles, err := downloadSymbolData(ctx, client, symbol, *interval, startTime, endTime)
		if err != nil {
			fmt.Printf("ERROR: %v\n", err)
			errors[symbol] = err.Error()
			continue
		}

		if len(candles) == 0 {
			fmt.Printf("NO DATA\n")
			errors[symbol] = "no data returned"
			continue
		}

		// Save to CSV
		filename := filepath.Join(*outputDir, fmt.Sprintf("%s_%s.csv", symbol, *interval))
		if err := saveToCSV(candles, filename); err != nil {
			fmt.Printf("SAVE ERROR: %v\n", err)
			errors[symbol] = err.Error()
			continue
		}

		summary[symbol] = len(candles)
		fmt.Printf("OK (%d candles, %s to %s)\n",
			len(candles),
			candles[0].OpenTime.Format("2006-01-02"),
			candles[len(candles)-1].OpenTime.Format("2006-01-02"))

		time.Sleep(500 * time.Millisecond) // Rate limiting
	}

	// Print summary
	fmt.Println("\n==============================================")
	fmt.Println("  DOWNLOAD SUMMARY")
	fmt.Println("==============================================")
	fmt.Printf("Successful: %d\n", len(summary))
	fmt.Printf("Failed:     %d\n", len(errors))
	fmt.Println("")

	if len(errors) > 0 {
		fmt.Println("Errors:")
		for sym, err := range errors {
			fmt.Printf("  %s: %s\n", sym, err)
		}
	}

	// Save summary
	summaryFile := filepath.Join(*outputDir, "download_summary.txt")
	f, _ := os.Create(summaryFile)
	defer f.Close()

	fmt.Fprintf(f, "Download Summary - %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(f, "Interval: %s\n", *interval)
	fmt.Fprintf(f, "Period: %s to %s\n\n", startTime.Format("2006-01-02"), endTime.Format("2006-01-02"))

	for sym, count := range summary {
		category := symbolCategories[sym]
		fmt.Fprintf(f, "%s (%s): %d candles\n", sym, category, count)
	}

	fmt.Printf("\nSummary saved to: %s\n", summaryFile)
}

func downloadSymbolData(ctx context.Context, client *exchange.BitunixClient, symbol, interval string, startTime, endTime time.Time) ([]model.Candle, error) {
	var allCandles []model.Candle

	// Download in chunks (API limit)
	chunkSize := 500
	currentEnd := endTime

	for currentEnd.After(startTime) {
		currentStart := currentEnd.AddDate(0, -3, 0) // 3 months chunks
		if currentStart.Before(startTime) {
			currentStart = startTime
		}

		candles, err := client.GetKlines(ctx, symbol, interval, chunkSize,
			currentStart.UnixMilli(), currentEnd.UnixMilli())
		if err != nil {
			return nil, err
		}

		if len(candles) == 0 {
			break
		}

		allCandles = append(allCandles, candles...)
		currentEnd = currentStart.Add(-time.Hour) // Move back

		time.Sleep(200 * time.Millisecond) // Rate limiting
	}

	// Sort by time
	sort.Slice(allCandles, func(i, j int) bool {
		return allCandles[i].OpenTime.Before(allCandles[j].OpenTime)
	})

	// Remove duplicates
	if len(allCandles) > 1 {
		unique := []model.Candle{allCandles[0]}
		for i := 1; i < len(allCandles); i++ {
			if allCandles[i].OpenTime != allCandles[i-1].OpenTime {
				unique = append(unique, allCandles[i])
			}
		}
		allCandles = unique
	}

	return allCandles, nil
}

func saveToCSV(candles []model.Candle, filename string) error {
	f, err := os.Create(filename)
	if err != nil {
		return err
	}
	defer f.Close()

	writer := csv.NewWriter(f)
	defer writer.Flush()

	// Header
	writer.Write([]string{"timestamp", "open", "high", "low", "close", "volume"})

	// Data
	for _, c := range candles {
		writer.Write([]string{
			c.OpenTime.Format("2006-01-02 15:04:05"),
			fmt.Sprintf("%.8f", c.Open),
			fmt.Sprintf("%.8f", c.High),
			fmt.Sprintf("%.8f", c.Low),
			fmt.Sprintf("%.8f", c.Close),
			fmt.Sprintf("%.8f", c.Volume),
		})
	}

	return nil
}
