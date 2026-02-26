package main

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Candle struct {
	Time   time.Time
	Open   float64
	High   float64
	Low    float64
	Close  float64
	Volume float64
}

type SymbolStats struct {
	Symbol          string
	Category        string
	CandleCount     int
	StartDate       time.Time
	EndDate         time.Time
	TotalReturn     float64
	AnnualizedReturn float64
	Volatility      float64
	SharpeRatio     float64
	MaxDrawdown     float64
	AvgDailyRange   float64 // (High-Low)/Close
	TrendStrength   float64 // Percentage of time in trend
	BestMonth       string
	WorstMonth      string
	WinRate4H       float64 // % of 4h candles that closed positive
}

var categories = map[string]string{
	"BTCUSDT": "L1-Major", "ETHUSDT": "L1-Major", "SOLUSDT": "L1-Major",
	"XRPUSDT": "L1-Alt", "DOGEUSDT": "L1-Alt", "LINKUSDT": "DeFi",
	"SUIUSDT": "L1-Major", "ADAUSDT": "L1-Major", "AVAXUSDT": "L1-Major",
	"LTCUSDT": "L1-Alt", "AAVEUSDT": "DeFi", "NEARUSDT": "L1-Major",
	"ICPUSDT": "Infra", "HBARUSDT": "L1-Alt", "FILUSDT": "Infra",
	"BCHUSDT": "L1-Alt", "DOTUSDT": "Infra", "UNIUSDT": "DeFi",
	"1000PEPEUSDT": "Meme", "1000SHIBUSDT": "Meme", "WIFUSDT": "Meme",
	"1000BONKUSDT": "Meme", "ARBUSDT": "L2", "ONDOUSDT": "RWA",
	"WLDUSDT": "AI", "ENAUSDT": "DeFi", "TAOUSDT": "AI",
	"APTUSDT": "L1-Major", "HYPEUSDT": "New", "TRUMPUSDT": "Meme-Political",
}

func main() {
	dataDir := "data/historical"

	files, err := filepath.Glob(filepath.Join(dataDir, "*_4h.csv"))
	if err != nil {
		fmt.Printf("Error reading directory: %v\n", err)
		return
	}

	fmt.Println("==============================================")
	fmt.Println("  MARKET DATA ANALYSIS")
	fmt.Println("  30 Cryptocurrencies - 4H Timeframe")
	fmt.Println("==============================================\n")

	var allStats []SymbolStats

	for _, file := range files {
		symbol := strings.TrimSuffix(filepath.Base(file), "_4h.csv")
		candles, err := loadCSV(file)
		if err != nil {
			fmt.Printf("Error loading %s: %v\n", symbol, err)
			continue
		}

		stats := analyzeSymbol(symbol, candles)
		allStats = append(allStats, stats)
	}

	// Sort by Sharpe Ratio
	sort.Slice(allStats, func(i, j int) bool {
		return allStats[i].SharpeRatio > allStats[j].SharpeRatio
	})

	// Print comprehensive analysis
	fmt.Println("=== PERFORMANCE RANKING (by Sharpe Ratio) ===\n")
	fmt.Printf("%-15s %-12s %10s %10s %10s %10s %10s\n",
		"Symbol", "Category", "Return%", "Volatility", "Sharpe", "MaxDD%", "WinRate%")
	fmt.Println(strings.Repeat("-", 85))

	for _, s := range allStats {
		fmt.Printf("%-15s %-12s %+10.2f %10.2f %10.2f %10.2f %10.1f\n",
			s.Symbol, s.Category, s.TotalReturn, s.Volatility,
			s.SharpeRatio, s.MaxDrawdown, s.WinRate4H)
	}

	// Category analysis
	fmt.Println("\n=== CATEGORY ANALYSIS ===\n")
	categoryStats := make(map[string][]SymbolStats)
	for _, s := range allStats {
		categoryStats[s.Category] = append(categoryStats[s.Category], s)
	}

	fmt.Printf("%-15s %8s %10s %10s %10s\n", "Category", "Count", "AvgReturn", "AvgVol", "AvgSharpe")
	fmt.Println(strings.Repeat("-", 55))

	for cat, stats := range categoryStats {
		var avgReturn, avgVol, avgSharpe float64
		for _, s := range stats {
			avgReturn += s.TotalReturn
			avgVol += s.Volatility
			avgSharpe += s.SharpeRatio
		}
		n := float64(len(stats))
		fmt.Printf("%-15s %8d %+10.2f %10.2f %10.2f\n",
			cat, len(stats), avgReturn/n, avgVol/n, avgSharpe/n)
	}

	// Volatility ranking
	sort.Slice(allStats, func(i, j int) bool {
		return allStats[i].Volatility > allStats[j].Volatility
	})

	fmt.Println("\n=== VOLATILITY RANKING (High to Low) ===\n")
	fmt.Printf("%-15s %-12s %12s %12s\n", "Symbol", "Category", "Volatility%", "AvgRange%")
	fmt.Println(strings.Repeat("-", 55))

	for _, s := range allStats {
		fmt.Printf("%-15s %-12s %12.2f %12.2f\n",
			s.Symbol, s.Category, s.Volatility, s.AvgDailyRange)
	}

	// Trading opportunity score
	fmt.Println("\n=== TRADING OPPORTUNITY SCORE ===")
	fmt.Println("(Combines: Volatility, Trend Strength, Win Rate)\n")

	for i := range allStats {
		// Score: Volatility * TrendStrength * WinRate adjustment
		allStats[i].TrendStrength = calculateOpportunityScore(allStats[i])
	}

	sort.Slice(allStats, func(i, j int) bool {
		return allStats[i].TrendStrength > allStats[j].TrendStrength
	})

	fmt.Printf("%-5s %-15s %-12s %12s\n", "Rank", "Symbol", "Category", "Score")
	fmt.Println(strings.Repeat("-", 50))

	for i, s := range allStats {
		fmt.Printf("%-5d %-15s %-12s %12.2f\n", i+1, s.Symbol, s.Category, s.TrendStrength)
	}

	// Recommendations
	fmt.Println("\n=== RECOMMENDATIONS ===\n")

	// Top performers
	sort.Slice(allStats, func(i, j int) bool {
		return allStats[i].TrendStrength > allStats[j].TrendStrength
	})

	fmt.Println("TOP 10 for Trading Bot (balanced score):")
	for i := 0; i < 10 && i < len(allStats); i++ {
		s := allStats[i]
		fmt.Printf("  %d. %s (Score: %.2f, Vol: %.2f%%, Sharpe: %.2f)\n",
			i+1, s.Symbol, s.TrendStrength, s.Volatility, s.SharpeRatio)
	}

	fmt.Println("\nHIGH RISK/HIGH REWARD (highest volatility):")
	sort.Slice(allStats, func(i, j int) bool {
		return allStats[i].Volatility > allStats[j].Volatility
	})
	for i := 0; i < 5 && i < len(allStats); i++ {
		s := allStats[i]
		fmt.Printf("  %d. %s (Vol: %.2f%%, Return: %+.2f%%)\n",
			i+1, s.Symbol, s.Volatility, s.TotalReturn)
	}

	fmt.Println("\nLOWER RISK (lowest volatility + positive Sharpe):")
	sort.Slice(allStats, func(i, j int) bool {
		return allStats[i].Volatility < allStats[j].Volatility
	})
	for i := 0; i < 5 && i < len(allStats); i++ {
		s := allStats[i]
		if s.SharpeRatio > 0 {
			fmt.Printf("  %d. %s (Vol: %.2f%%, Sharpe: %.2f)\n",
				i+1, s.Symbol, s.Volatility, s.SharpeRatio)
		}
	}
}

func loadCSV(filename string) ([]Candle, error) {
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

	var candles []Candle
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

		candles = append(candles, Candle{
			Time:   t,
			Open:   open,
			High:   high,
			Low:    low,
			Close:  close,
			Volume: volume,
		})
	}

	return candles, nil
}

func analyzeSymbol(symbol string, candles []Candle) SymbolStats {
	stats := SymbolStats{
		Symbol:      symbol,
		Category:    categories[symbol],
		CandleCount: len(candles),
		StartDate:   candles[0].Time,
		EndDate:     candles[len(candles)-1].Time,
	}

	// Calculate returns
	var returns []float64
	var positiveCandles int
	var totalRange float64

	for i := 1; i < len(candles); i++ {
		ret := (candles[i].Close - candles[i-1].Close) / candles[i-1].Close
		returns = append(returns, ret)

		if candles[i].Close > candles[i].Open {
			positiveCandles++
		}

		rangePercent := (candles[i].High - candles[i].Low) / candles[i].Close * 100
		totalRange += rangePercent
	}

	stats.WinRate4H = float64(positiveCandles) / float64(len(candles)-1) * 100
	stats.AvgDailyRange = totalRange / float64(len(candles)-1)

	// Total return
	stats.TotalReturn = (candles[len(candles)-1].Close - candles[0].Close) / candles[0].Close * 100

	// Annualized return
	days := stats.EndDate.Sub(stats.StartDate).Hours() / 24
	stats.AnnualizedReturn = stats.TotalReturn * (365 / days)

	// Volatility (annualized)
	meanReturn := mean(returns)
	var variance float64
	for _, r := range returns {
		variance += (r - meanReturn) * (r - meanReturn)
	}
	variance /= float64(len(returns))
	stats.Volatility = math.Sqrt(variance) * math.Sqrt(6*365) * 100 // 6 4h candles per day

	// Sharpe Ratio (assuming 0% risk-free rate)
	if stats.Volatility > 0 {
		stats.SharpeRatio = stats.AnnualizedReturn / stats.Volatility
	}

	// Max Drawdown
	stats.MaxDrawdown = calculateMaxDrawdown(candles)

	return stats
}

func calculateMaxDrawdown(candles []Candle) float64 {
	peak := candles[0].Close
	maxDD := 0.0

	for _, c := range candles {
		if c.Close > peak {
			peak = c.Close
		}
		dd := (peak - c.Close) / peak * 100
		if dd > maxDD {
			maxDD = dd
		}
	}

	return maxDD
}

func calculateOpportunityScore(s SymbolStats) float64 {
	// Normalize components
	volScore := math.Min(s.Volatility/100, 1.5) // Cap at 150% vol
	sharpeScore := (s.SharpeRatio + 1) / 2      // Normalize around 0
	winRateScore := s.WinRate4H / 100

	// Combine: We want volatility (opportunity) but also positive expectancy
	score := volScore * 30 + sharpeScore * 40 + winRateScore * 30

	// Penalty for extreme drawdowns
	if s.MaxDrawdown > 80 {
		score *= 0.7
	} else if s.MaxDrawdown > 60 {
		score *= 0.85
	}

	return score
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}
