package main

import (
	"context"
	"fmt"
	"os"

	"final-bot-trader-api/internal/exchange"
	"final-bot-trader-api/internal/exchange/model"
	"final-bot-trader-api/internal/strategy/multifactor"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	apiKey := os.Getenv("BITUNIX_API_KEY")
	apiSecret := os.Getenv("BITUNIX_SECRET_KEY")
	client := exchange.NewBitunixClient(apiKey, apiSecret, "")
	ctx := context.Background()

	symbol := "DOGEUSDT"

	fmt.Printf("Debugging signal conditions for %s\n\n", symbol)

	candles, err := client.GetKlines(ctx, symbol, "4h", 100, 0, 0)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	// Get regime
	detector := multifactor.DefaultRegimeDetector()
	regime, adx, atr := detector.DetectRegime(candles)

	fmt.Printf("Market Regime: %s\n", regime)
	fmt.Printf("ADX: %.2f\n", adx)
	fmt.Printf("ATR: %.6f\n", atr)
	fmt.Println()

	// Calculate indicators manually
	config := multifactor.DefaultConfig()

	// Get closes for EMA
	closes := make([]float64, len(candles))
	for i, c := range candles {
		closes[i] = c.Close
	}

	fastEMA := ema(closes, config.FastEMA)
	slowEMA := ema(closes, config.SlowEMA)

	lastFast := fastEMA[len(fastEMA)-1]
	lastSlow := slowEMA[len(slowEMA)-1]
	currentPrice := candles[len(candles)-1].Close

	// Calculate RSI
	rsi := calculateRSI(candles, config.RSIPeriod)
	lastRSI := rsi[len(rsi)-1]

	// Calculate volume ratio
	volRatio := calculateVolumeRatio(candles, config.VolumePeriod)

	fmt.Println("Current Values:")
	fmt.Printf("  Price:        %.4f\n", currentPrice)
	fmt.Printf("  Fast EMA(%d): %.4f\n", config.FastEMA, lastFast)
	fmt.Printf("  Slow EMA(%d): %.4f\n", config.SlowEMA, lastSlow)
	fmt.Printf("  RSI(%d):      %.2f\n", config.RSIPeriod, lastRSI)
	fmt.Printf("  Volume Ratio: %.2f (threshold: %.2f)\n", volRatio, config.VolumeThreshold)
	fmt.Println()

	// Check SHORT conditions
	fmt.Println("SHORT Conditions (need 4/5):")
	cond1 := regime == multifactor.RegimeTrendingDown || regime == multifactor.RegimeHighVolatility
	cond2 := lastFast < lastSlow
	cond3 := currentPrice < lastFast
	cond4 := lastRSI < 60 && lastRSI > config.RSIOversold
	cond5 := isLowerHigh(candles)

	printCondition(1, "Regime TRENDING_DOWN or HIGH_VOL", cond1, fmt.Sprintf("Regime=%s", regime))
	printCondition(2, "Fast EMA < Slow EMA", cond2, fmt.Sprintf("%.4f < %.4f", lastFast, lastSlow))
	printCondition(3, "Price < Fast EMA", cond3, fmt.Sprintf("%.4f < %.4f", currentPrice, lastFast))
	printCondition(4, "RSI between 30-60", cond4, fmt.Sprintf("RSI=%.2f", lastRSI))
	printCondition(5, "Lower highs forming", cond5, "Structure check")

	score := 0
	if cond1 {
		score++
	}
	if cond2 {
		score++
	}
	if cond3 {
		score++
	}
	if cond4 {
		score++
	}
	if cond5 {
		score++
	}

	fmt.Printf("\nTotal Score: %d/5 (need 4 for signal)\n", score)

	if volRatio < config.VolumeThreshold {
		fmt.Printf("\n⚠️  Volume too low: %.2f < %.2f threshold\n", volRatio, config.VolumeThreshold)
	}

	if score >= 4 && volRatio >= config.VolumeThreshold {
		fmt.Println("\n✅ All conditions met - SHORT signal should be generated!")
	} else {
		fmt.Printf("\n❌ Missing %d conditions for SHORT signal\n", 4-score)
	}
}

func printCondition(num int, name string, met bool, detail string) {
	status := "❌"
	if met {
		status = "✅"
	}
	fmt.Printf("  %s %d. %-30s (%s)\n", status, num, name, detail)
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

func isLowerHigh(candles []model.Candle) bool {
	if len(candles) < 10 {
		return false
	}
	recent := candles[len(candles)-10:]
	firstHalf := recent[:5]
	secondHalf := recent[5:]

	maxFirst := firstHalf[0].High
	for _, c := range firstHalf {
		if c.High > maxFirst {
			maxFirst = c.High
		}
	}

	maxSecond := secondHalf[0].High
	for _, c := range secondHalf {
		if c.High > maxSecond {
			maxSecond = c.High
		}
	}

	return maxSecond < maxFirst*1.01
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
