package main

import (
	"context"
	"fmt"
	"os"
	"sort"

	"final-bot-trader-api/internal/exchange"
	"final-bot-trader-api/internal/exchange/model"
	"final-bot-trader-api/internal/strategy/multifactor"

	"github.com/joho/godotenv"
)

var symbols = []string{
	"DOGEUSDT", "WLDUSDT", "1000PEPEUSDT", "ARBUSDT", "AAVEUSDT",
	"WIFUSDT", "FILUSDT", "SOLUSDT", "TAOUSDT", "SUIUSDT",
}

type SymbolAnalysis struct {
	Symbol      string
	Price       float64
	Regime      string
	ADX         float64
	RSI         float64
	VolRatio    float64
	Score       int
	ScoreType   string // "LONG" or "SHORT"
	Missing     string
}

func main() {
	godotenv.Load()
	client := exchange.NewBitunixClient(
		os.Getenv("BITUNIX_API_KEY"),
		os.Getenv("BITUNIX_SECRET_KEY"),
		"",
	)
	ctx := context.Background()

	fmt.Println("══════════════════════════════════════════════════════════════════")
	fmt.Println("  SIGNAL SCANNER - Detailed Analysis")
	fmt.Println("══════════════════════════════════════════════════════════════════")
	fmt.Println()

	config := multifactor.DefaultConfig()
	var analyses []SymbolAnalysis

	for _, symbol := range symbols {
		candles, err := client.GetKlines(ctx, symbol, "4h", 100, 0, 0)
		if err != nil {
			continue
		}

		analysis := analyzeSymbol(symbol, candles, config)
		analyses = append(analyses, analysis)
	}

	// Sort by score
	sort.Slice(analyses, func(i, j int) bool {
		return analyses[i].Score > analyses[j].Score
	})

	fmt.Printf("%-14s %10s %8s %8s %8s %8s %6s %s\n",
		"Symbol", "Price", "Regime", "ADX", "RSI", "VolRatio", "Score", "Missing")
	fmt.Println("────────────────────────────────────────────────────────────────────────────────")

	for _, a := range analyses {
		scoreStr := fmt.Sprintf("%d/5 %s", a.Score, a.ScoreType)
		if a.Score >= 4 {
			scoreStr = fmt.Sprintf("\033[32m%d/5 %s\033[0m", a.Score, a.ScoreType)
		}

		volColor := ""
		volReset := ""
		if a.VolRatio < 0.7 {
			volColor = "\033[33m" // Yellow
			volReset = "\033[0m"
		}

		fmt.Printf("%-14s %10.4f %8s %8.1f %8.1f %s%8.2f%s %s %s\n",
			a.Symbol, a.Price, a.Regime[:8], a.ADX, a.RSI,
			volColor, a.VolRatio, volReset,
			scoreStr, a.Missing)
	}

	fmt.Println()
	fmt.Println("Legend:")
	fmt.Println("  Score 4+/5 = Signal would trigger (if volume OK)")
	fmt.Println("  Yellow VolRatio = Below recommended threshold (0.7)")
	fmt.Println()

	// Summary
	var ready, needsVolume int
	for _, a := range analyses {
		if a.Score >= 4 {
			if a.VolRatio >= 0.7 {
				ready++
			} else {
				needsVolume++
			}
		}
	}

	fmt.Printf("Ready to trade: %d symbols\n", ready)
	fmt.Printf("Waiting for volume: %d symbols\n", needsVolume)
	fmt.Printf("Waiting for setup: %d symbols\n", len(analyses)-ready-needsVolume)
}

func analyzeSymbol(symbol string, candles []model.Candle, config multifactor.Config) SymbolAnalysis {
	analysis := SymbolAnalysis{Symbol: symbol}

	if len(candles) < 60 {
		analysis.Missing = "insufficient data"
		return analysis
	}

	detector := multifactor.DefaultRegimeDetector()
	regime, adx, _ := detector.DetectRegime(candles)

	analysis.Price = candles[len(candles)-1].Close
	analysis.Regime = regime.String()
	analysis.ADX = adx

	// Calculate indicators
	closes := make([]float64, len(candles))
	for i, c := range candles {
		closes[i] = c.Close
	}

	fastEMA := ema(closes, config.FastEMA)
	slowEMA := ema(closes, config.SlowEMA)
	rsi := calculateRSI(candles, config.RSIPeriod)
	volRatio := calculateVolumeRatio(candles, config.VolumePeriod)

	lastFast := fastEMA[len(fastEMA)-1]
	lastSlow := slowEMA[len(slowEMA)-1]
	lastRSI := rsi[len(rsi)-1]
	currentPrice := candles[len(candles)-1].Close

	analysis.RSI = lastRSI
	analysis.VolRatio = volRatio

	// Check LONG conditions
	longConds := []bool{
		regime == multifactor.RegimeTrendingUp || regime == multifactor.RegimeHighVolatility,
		lastFast > lastSlow,
		currentPrice > lastFast,
		lastRSI > 40 && lastRSI < config.RSIOverbought,
		isHigherLow(candles),
	}

	// Check SHORT conditions
	shortConds := []bool{
		regime == multifactor.RegimeTrendingDown || regime == multifactor.RegimeHighVolatility,
		lastFast < lastSlow,
		currentPrice < lastFast,
		lastRSI < 60 && lastRSI > config.RSIOversold,
		isLowerHigh(candles),
	}

	longScore := countTrue(longConds)
	shortScore := countTrue(shortConds)

	if longScore >= shortScore {
		analysis.Score = longScore
		analysis.ScoreType = "L"
		analysis.Missing = getMissing(longConds, []string{"regime", "ema", "price>ema", "rsi", "struct"})
	} else {
		analysis.Score = shortScore
		analysis.ScoreType = "S"
		analysis.Missing = getMissing(shortConds, []string{"regime", "ema", "price<ema", "rsi", "struct"})
	}

	return analysis
}

func getMissing(conds []bool, names []string) string {
	var missing []string
	for i, c := range conds {
		if !c && i < len(names) {
			missing = append(missing, names[i])
		}
	}
	if len(missing) == 0 {
		return "-"
	}
	result := ""
	for _, m := range missing {
		result += m + " "
	}
	return result
}

func countTrue(conds []bool) int {
	count := 0
	for _, c := range conds {
		if c {
			count++
		}
	}
	return count
}

func ema(values []float64, period int) []float64 {
	if len(values) == 0 {
		return []float64{}
	}
	result := make([]float64, len(values))
	multiplier := 2.0 / float64(period+1)
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

func calculateRSI(candles []model.Candle, period int) []float64 {
	if len(candles) < period+1 {
		return []float64{50}
	}
	var gains, losses []float64
	for i := 1; i < len(candles); i++ {
		change := candles[i].Close - candles[i-1].Close
		if change > 0 {
			gains = append(gains, change)
			losses = append(losses, 0)
		} else {
			gains = append(gains, 0)
			losses = append(losses, -change)
		}
	}
	avgGain := ema(gains, period)
	avgLoss := ema(losses, period)
	var rsi []float64
	for i := 0; i < len(avgGain); i++ {
		if avgLoss[i] == 0 {
			rsi = append(rsi, 100)
		} else {
			rs := avgGain[i] / avgLoss[i]
			rsi = append(rsi, 100-(100/(1+rs)))
		}
	}
	return rsi
}

func calculateVolumeRatio(candles []model.Candle, period int) float64 {
	if len(candles) < period+1 {
		return 1.0
	}
	currentVolume := candles[len(candles)-1].Volume
	var sum float64
	for i := len(candles) - period - 1; i < len(candles)-1; i++ {
		sum += candles[i].Volume
	}
	avgVolume := sum / float64(period)
	if avgVolume == 0 {
		return 1.0
	}
	return currentVolume / avgVolume
}

func isHigherLow(candles []model.Candle) bool {
	if len(candles) < 10 {
		return false
	}
	recent := candles[len(candles)-10:]
	firstHalf := recent[:5]
	secondHalf := recent[5:]
	return minLow(secondHalf) > minLow(firstHalf)*0.99
}

func isLowerHigh(candles []model.Candle) bool {
	if len(candles) < 10 {
		return false
	}
	recent := candles[len(candles)-10:]
	firstHalf := recent[:5]
	secondHalf := recent[5:]
	return maxHigh(secondHalf) < maxHigh(firstHalf)*1.01
}

func minLow(candles []model.Candle) float64 {
	if len(candles) == 0 {
		return 0
	}
	m := candles[0].Low
	for _, c := range candles {
		if c.Low < m {
			m = c.Low
		}
	}
	return m
}

func maxHigh(candles []model.Candle) float64 {
	if len(candles) == 0 {
		return 0
	}
	m := candles[0].High
	for _, c := range candles {
		if c.High > m {
			m = c.High
		}
	}
	return m
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
