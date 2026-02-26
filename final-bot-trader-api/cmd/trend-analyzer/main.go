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

type TrendStats struct {
	Symbol string
	Category string

	// Trend Metrics
	AvgADX           float64 // Average ADX (trend strength)
	TimeInTrend      float64 // % of time ADX > 25
	AvgTrendDuration int     // Average candles per trend
	MaxTrendDuration int     // Longest trend
	TrendCount       int     // Number of distinct trends

	// Momentum vs Mean Reversion
	MomentumScore    float64 // Positive = momentum works, Negative = mean reversion works
	ConsecutiveAvg   float64 // Average consecutive same-direction candles

	// Moving Average Respect
	MA20Respect      float64 // % of time price respects MA20 as support/resistance
	MA50Respect      float64 // % of time price respects MA50

	// Breakout Behavior
	BreakoutSuccess  float64 // % of breakouts that follow through
	FakeoutRate      float64 // % of breakouts that reverse

	// Overall Tradability Score
	TrendScore       float64
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
	fmt.Println("  TREND ANALYSIS - TRENDEABILIDAD")
	fmt.Println("  Que tan bien sigue tendencias cada crypto?")
	fmt.Println("==============================================\n")

	var allStats []TrendStats

	for _, file := range files {
		symbol := strings.TrimSuffix(filepath.Base(file), "_4h.csv")
		candles, err := loadCSV(file)
		if err != nil {
			fmt.Printf("Error loading %s: %v\n", symbol, err)
			continue
		}

		if len(candles) < 100 {
			continue
		}

		stats := analyzeTrends(symbol, candles)
		allStats = append(allStats, stats)
	}

	// Sort by Trend Score
	sort.Slice(allStats, func(i, j int) bool {
		return allStats[i].TrendScore > allStats[j].TrendScore
	})

	// Print ADX Analysis
	fmt.Println("=== TREND STRENGTH (ADX Analysis) ===")
	fmt.Println("ADX > 25 = Trending, ADX < 20 = Ranging\n")
	fmt.Printf("%-15s %-10s %8s %10s %12s %10s\n",
		"Symbol", "Category", "AvgADX", "InTrend%", "AvgDuration", "Trends")
	fmt.Println(strings.Repeat("-", 70))

	for _, s := range allStats {
		fmt.Printf("%-15s %-10s %8.1f %10.1f%% %12d %10d\n",
			s.Symbol, s.Category, s.AvgADX, s.TimeInTrend,
			s.AvgTrendDuration, s.TrendCount)
	}

	// Momentum vs Mean Reversion
	fmt.Println("\n=== MOMENTUM vs MEAN REVERSION ===")
	fmt.Println("Score > 0 = Momentum funciona mejor")
	fmt.Println("Score < 0 = Mean reversion funciona mejor\n")

	sort.Slice(allStats, func(i, j int) bool {
		return allStats[i].MomentumScore > allStats[j].MomentumScore
	})

	fmt.Printf("%-15s %-10s %12s %12s %15s\n",
		"Symbol", "Category", "MomScore", "ConsecAvg", "Comportamiento")
	fmt.Println(strings.Repeat("-", 65))

	for _, s := range allStats {
		behavior := "NEUTRAL"
		if s.MomentumScore > 0.02 {
			behavior = "MOMENTUM ✓"
		} else if s.MomentumScore < -0.02 {
			behavior = "MEAN-REV ✗"
		}
		fmt.Printf("%-15s %-10s %+12.4f %12.2f %15s\n",
			s.Symbol, s.Category, s.MomentumScore, s.ConsecutiveAvg, behavior)
	}

	// Moving Average Analysis
	fmt.Println("\n=== MOVING AVERAGE RESPECT ===")
	fmt.Println("Higher % = Price respects MA as support/resistance\n")

	sort.Slice(allStats, func(i, j int) bool {
		return allStats[i].MA20Respect > allStats[j].MA20Respect
	})

	fmt.Printf("%-15s %-10s %12s %12s\n",
		"Symbol", "Category", "MA20 Respect", "MA50 Respect")
	fmt.Println(strings.Repeat("-", 52))

	for _, s := range allStats {
		fmt.Printf("%-15s %-10s %11.1f%% %11.1f%%\n",
			s.Symbol, s.Category, s.MA20Respect, s.MA50Respect)
	}

	// Breakout Analysis
	fmt.Println("\n=== BREAKOUT BEHAVIOR ===")
	fmt.Println("Success% = Breakouts that follow through\n")

	sort.Slice(allStats, func(i, j int) bool {
		return allStats[i].BreakoutSuccess > allStats[j].BreakoutSuccess
	})

	fmt.Printf("%-15s %-10s %12s %12s\n",
		"Symbol", "Category", "Success%", "Fakeout%")
	fmt.Println(strings.Repeat("-", 52))

	for _, s := range allStats {
		fmt.Printf("%-15s %-10s %11.1f%% %11.1f%%\n",
			s.Symbol, s.Category, s.BreakoutSuccess, s.FakeoutRate)
	}

	// Final Ranking
	fmt.Println("\n=== FINAL TREND SCORE RANKING ===")
	fmt.Println("Combines: ADX, Momentum, MA Respect, Breakout Success\n")

	sort.Slice(allStats, func(i, j int) bool {
		return allStats[i].TrendScore > allStats[j].TrendScore
	})

	fmt.Printf("%-5s %-15s %-10s %10s %50s\n",
		"Rank", "Symbol", "Category", "Score", "Assessment")
	fmt.Println(strings.Repeat("-", 95))

	for i, s := range allStats {
		assessment := getAssessment(s)
		fmt.Printf("%-5d %-15s %-10s %10.2f %50s\n",
			i+1, s.Symbol, s.Category, s.TrendScore, assessment)
	}

	// Recommendations
	fmt.Println("\n" + strings.Repeat("=", 70))
	fmt.Println("  RECOMENDACIONES PARA EL BOT")
	fmt.Println(strings.Repeat("=", 70))

	fmt.Println("\n🟢 MEJORES PARA TREND FOLLOWING (Top 10):")
	for i := 0; i < 10 && i < len(allStats); i++ {
		s := allStats[i]
		fmt.Printf("   %d. %s - Score: %.2f (ADX: %.1f, Momentum: %+.3f)\n",
			i+1, s.Symbol, s.TrendScore, s.AvgADX, s.MomentumScore)
	}

	fmt.Println("\n🔴 EVITAR PARA TREND FOLLOWING (Bottom 5):")
	for i := len(allStats) - 5; i < len(allStats); i++ {
		if i >= 0 {
			s := allStats[i]
			fmt.Printf("   • %s - Score: %.2f (muy errático o mean-reverting)\n",
				s.Symbol, s.TrendScore)
		}
	}

	// Category summary
	fmt.Println("\n📊 RESUMEN POR CATEGORÍA:")
	catScores := make(map[string][]float64)
	for _, s := range allStats {
		catScores[s.Category] = append(catScores[s.Category], s.TrendScore)
	}

	type CatAvg struct {
		Cat string
		Avg float64
	}
	var catAvgs []CatAvg
	for cat, scores := range catScores {
		sum := 0.0
		for _, sc := range scores {
			sum += sc
		}
		catAvgs = append(catAvgs, CatAvg{cat, sum / float64(len(scores))})
	}
	sort.Slice(catAvgs, func(i, j int) bool {
		return catAvgs[i].Avg > catAvgs[j].Avg
	})

	for _, ca := range catAvgs {
		emoji := "⚪"
		if ca.Avg > 60 {
			emoji = "🟢"
		} else if ca.Avg < 45 {
			emoji = "🔴"
		}
		fmt.Printf("   %s %s: %.2f\n", emoji, ca.Cat, ca.Avg)
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
		if i == 0 {
			continue
		}

		t, _ := time.Parse("2006-01-02 15:04:05", record[0])
		open, _ := strconv.ParseFloat(record[1], 64)
		high, _ := strconv.ParseFloat(record[2], 64)
		low, _ := strconv.ParseFloat(record[3], 64)
		close, _ := strconv.ParseFloat(record[4], 64)
		volume, _ := strconv.ParseFloat(record[5], 64)

		candles = append(candles, Candle{t, open, high, low, close, volume})
	}

	return candles, nil
}

func analyzeTrends(symbol string, candles []Candle) TrendStats {
	stats := TrendStats{
		Symbol:   symbol,
		Category: categories[symbol],
	}

	// Calculate ADX
	adxValues := calculateADX(candles, 14)
	stats.AvgADX = mean(adxValues)

	// Time in trend (ADX > 25)
	trendCount := 0
	for _, adx := range adxValues {
		if adx > 25 {
			trendCount++
		}
	}
	stats.TimeInTrend = float64(trendCount) / float64(len(adxValues)) * 100

	// Trend duration analysis
	stats.AvgTrendDuration, stats.MaxTrendDuration, stats.TrendCount = analyzeTrendDurations(adxValues)

	// Momentum analysis
	stats.MomentumScore, stats.ConsecutiveAvg = analyzeMomentum(candles)

	// Moving average respect
	stats.MA20Respect = analyzeMAResp(candles, 20)
	stats.MA50Respect = analyzeMAResp(candles, 50)

	// Breakout analysis
	stats.BreakoutSuccess, stats.FakeoutRate = analyzeBreakouts(candles, 20)

	// Calculate overall trend score
	stats.TrendScore = calculateTrendScore(stats)

	return stats
}

func calculateADX(candles []Candle, period int) []float64 {
	if len(candles) < period*2 {
		return []float64{20} // Default neutral
	}

	var adxValues []float64
	var plusDM, minusDM, tr []float64

	// Calculate +DM, -DM, TR
	for i := 1; i < len(candles); i++ {
		high := candles[i].High
		low := candles[i].Low
		prevHigh := candles[i-1].High
		prevLow := candles[i-1].Low
		prevClose := candles[i-1].Close

		// True Range
		tr1 := high - low
		tr2 := math.Abs(high - prevClose)
		tr3 := math.Abs(low - prevClose)
		tr = append(tr, math.Max(tr1, math.Max(tr2, tr3)))

		// Directional Movement
		upMove := high - prevHigh
		downMove := prevLow - low

		if upMove > downMove && upMove > 0 {
			plusDM = append(plusDM, upMove)
		} else {
			plusDM = append(plusDM, 0)
		}

		if downMove > upMove && downMove > 0 {
			minusDM = append(minusDM, downMove)
		} else {
			minusDM = append(minusDM, 0)
		}
	}

	// Smooth the values
	smoothPlusDM := ema(plusDM, period)
	smoothMinusDM := ema(minusDM, period)
	smoothTR := ema(tr, period)

	// Calculate DI+ and DI-
	var dx []float64
	for i := 0; i < len(smoothTR); i++ {
		if smoothTR[i] == 0 {
			continue
		}
		plusDI := (smoothPlusDM[i] / smoothTR[i]) * 100
		minusDI := (smoothMinusDM[i] / smoothTR[i]) * 100

		diSum := plusDI + minusDI
		if diSum == 0 {
			dx = append(dx, 0)
		} else {
			dx = append(dx, math.Abs(plusDI-minusDI)/diSum*100)
		}
	}

	// ADX is EMA of DX
	adxValues = ema(dx, period)

	return adxValues
}

func analyzeTrendDurations(adxValues []float64) (avgDuration, maxDuration, trendCount int) {
	inTrend := false
	currentDuration := 0
	var durations []int

	for _, adx := range adxValues {
		if adx > 25 {
			if !inTrend {
				inTrend = true
				trendCount++
			}
			currentDuration++
		} else {
			if inTrend && currentDuration > 0 {
				durations = append(durations, currentDuration)
				if currentDuration > maxDuration {
					maxDuration = currentDuration
				}
			}
			inTrend = false
			currentDuration = 0
		}
	}

	if len(durations) > 0 {
		sum := 0
		for _, d := range durations {
			sum += d
		}
		avgDuration = sum / len(durations)
	}

	return
}

func analyzeMomentum(candles []Candle) (momentumScore, consecutiveAvg float64) {
	// Momentum: Does following the previous candle's direction work?
	var momentumWins, momentumLosses int
	var consecutiveCounts []int
	consecutive := 1
	lastDirection := 0

	for i := 2; i < len(candles); i++ {
		prevReturn := candles[i-1].Close - candles[i-2].Close
		currReturn := candles[i].Close - candles[i-1].Close

		// Momentum: if previous was up, does current go up?
		if (prevReturn > 0 && currReturn > 0) || (prevReturn < 0 && currReturn < 0) {
			momentumWins++
		} else {
			momentumLosses++
		}

		// Consecutive same direction
		direction := 0
		if candles[i].Close > candles[i].Open {
			direction = 1
		} else if candles[i].Close < candles[i].Open {
			direction = -1
		}

		if direction == lastDirection && direction != 0 {
			consecutive++
		} else {
			if consecutive > 1 {
				consecutiveCounts = append(consecutiveCounts, consecutive)
			}
			consecutive = 1
		}
		lastDirection = direction
	}

	total := momentumWins + momentumLosses
	if total > 0 {
		// Score: positive if momentum > 50%, negative if < 50%
		momentumScore = (float64(momentumWins)/float64(total) - 0.5)
	}

	if len(consecutiveCounts) > 0 {
		sum := 0
		for _, c := range consecutiveCounts {
			sum += c
		}
		consecutiveAvg = float64(sum) / float64(len(consecutiveCounts))
	}

	return
}

func analyzeMAResp(candles []Candle, period int) float64 {
	if len(candles) < period+20 {
		return 50
	}

	ma := sma(candles, period)
	respectCount := 0
	totalTests := 0

	for i := period + 5; i < len(candles)-5; i++ {
		price := candles[i].Close
		maValue := ma[i-period]

		// Check if price touched MA and bounced
		touchedMA := math.Abs(price-maValue)/maValue < 0.01 // Within 1%

		if touchedMA {
			totalTests++
			// Check next 5 candles for bounce
			futurePrices := []float64{}
			for j := i + 1; j <= i+5 && j < len(candles); j++ {
				futurePrices = append(futurePrices, candles[j].Close)
			}

			if len(futurePrices) > 0 {
				// Did it bounce (move away from MA)?
				avgFuture := mean(futurePrices)
				if math.Abs(avgFuture-maValue) > math.Abs(price-maValue) {
					respectCount++
				}
			}
		}
	}

	if totalTests > 0 {
		return float64(respectCount) / float64(totalTests) * 100
	}
	return 50
}

func analyzeBreakouts(candles []Candle, period int) (successRate, fakeoutRate float64) {
	if len(candles) < period+20 {
		return 50, 50
	}

	successCount := 0
	fakeoutCount := 0
	totalBreakouts := 0

	for i := period; i < len(candles)-10; i++ {
		// Calculate recent high/low
		var recentHigh, recentLow float64 = candles[i-period].High, candles[i-period].Low
		for j := i - period; j < i; j++ {
			if candles[j].High > recentHigh {
				recentHigh = candles[j].High
			}
			if candles[j].Low < recentLow {
				recentLow = candles[j].Low
			}
		}

		// Check for breakout
		if candles[i].Close > recentHigh {
			totalBreakouts++
			// Check follow-through in next 10 candles
			maxAfter := candles[i].Close
			minAfter := candles[i].Close
			for j := i + 1; j <= i+10 && j < len(candles); j++ {
				if candles[j].High > maxAfter {
					maxAfter = candles[j].High
				}
				if candles[j].Low < minAfter {
					minAfter = candles[j].Low
				}
			}

			// Success if it went higher, fakeout if it reversed below breakout
			if maxAfter > candles[i].Close*1.02 {
				successCount++
			}
			if minAfter < recentHigh {
				fakeoutCount++
			}
		} else if candles[i].Close < recentLow {
			totalBreakouts++
			maxAfter := candles[i].Close
			minAfter := candles[i].Close
			for j := i + 1; j <= i+10 && j < len(candles); j++ {
				if candles[j].High > maxAfter {
					maxAfter = candles[j].High
				}
				if candles[j].Low < minAfter {
					minAfter = candles[j].Low
				}
			}

			if minAfter < candles[i].Close*0.98 {
				successCount++
			}
			if maxAfter > recentLow {
				fakeoutCount++
			}
		}
	}

	if totalBreakouts > 0 {
		successRate = float64(successCount) / float64(totalBreakouts) * 100
		fakeoutRate = float64(fakeoutCount) / float64(totalBreakouts) * 100
	}

	return
}

func calculateTrendScore(s TrendStats) float64 {
	// Weights for different factors
	adxScore := math.Min(s.AvgADX/40, 1) * 25           // Max 25 points
	trendTimeScore := s.TimeInTrend / 100 * 25          // Max 25 points
	momentumScore := (s.MomentumScore + 0.1) * 100      // Normalized, max ~20 points
	maRespScore := (s.MA20Respect + s.MA50Respect) / 4  // Max 25 points
	breakoutScore := s.BreakoutSuccess / 4              // Max 25 points

	total := adxScore + trendTimeScore + momentumScore + maRespScore + breakoutScore

	// Penalty for high fakeout rate
	if s.FakeoutRate > 60 {
		total *= 0.85
	}

	return total
}

func getAssessment(s TrendStats) string {
	var parts []string

	if s.AvgADX > 30 {
		parts = append(parts, "Strong trends")
	} else if s.AvgADX < 20 {
		parts = append(parts, "Weak trends")
	}

	if s.MomentumScore > 0.02 {
		parts = append(parts, "Momentum works")
	} else if s.MomentumScore < -0.02 {
		parts = append(parts, "Mean-reverting")
	}

	if s.BreakoutSuccess > 55 {
		parts = append(parts, "Good breakouts")
	} else if s.FakeoutRate > 60 {
		parts = append(parts, "Many fakeouts")
	}

	if len(parts) == 0 {
		return "Average behavior"
	}
	return strings.Join(parts, ", ")
}

func sma(candles []Candle, period int) []float64 {
	var result []float64
	for i := period - 1; i < len(candles); i++ {
		sum := 0.0
		for j := i - period + 1; j <= i; j++ {
			sum += candles[j].Close
		}
		result = append(result, sum/float64(period))
	}
	return result
}

func ema(values []float64, period int) []float64 {
	if len(values) == 0 {
		return []float64{}
	}

	result := make([]float64, len(values))
	multiplier := 2.0 / float64(period+1)

	// First EMA is SMA
	sum := 0.0
	for i := 0; i < period && i < len(values); i++ {
		sum += values[i]
	}
	result[0] = sum / float64(min(period, len(values)))

	for i := 1; i < len(values); i++ {
		result[i] = (values[i]-result[i-1])*multiplier + result[i-1]
	}

	return result
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

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
