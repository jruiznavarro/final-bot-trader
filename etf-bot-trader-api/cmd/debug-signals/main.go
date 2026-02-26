package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"etf-bot-trader-api/internal/exchange"
	"etf-bot-trader-api/internal/indicator"
	"etf-bot-trader-api/internal/strategy"

	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found")
	}

	apiKey := os.Getenv("ALPACA_API_KEY")
	apiSecret := os.Getenv("ALPACA_API_SECRET")

	if apiKey == "" || apiSecret == "" {
		log.Fatal("ALPACA_API_KEY and ALPACA_API_SECRET must be set")
	}

	client := exchange.NewAlpacaClient(apiKey, apiSecret, true)
	ctx := context.Background()

	symbols := []string{"SPY", "QQQ", "IWM"}

	for _, symbol := range symbols {
		fmt.Printf("\n═══════════════════════════════════════════════════════════════\n")
		fmt.Printf("  DEBUG: %s\n", symbol)
		fmt.Printf("═══════════════════════════════════════════════════════════════\n")

		// Get daily candles
		end := time.Now()
		dailyStart := end.Add(-120 * 24 * time.Hour)
		dailyCandles, err := client.GetBars(ctx, symbol, "1d", dailyStart, end)
		if err != nil {
			log.Printf("[%s] Error getting daily bars: %v", symbol, err)
			continue
		}

		// Get hourly candles
		hourlyStart := end.Add(-30 * 24 * time.Hour)
		hourlyCandles, err := client.GetBars(ctx, symbol, "1h", hourlyStart, end)
		if err != nil {
			log.Printf("[%s] Error getting hourly bars: %v", symbol, err)
			continue
		}

		fmt.Printf("Daily candles: %d\n", len(dailyCandles))
		fmt.Printf("Hourly candles: %d\n", len(hourlyCandles))

		// Calculate indicators on daily
		if len(dailyCandles) >= 50 {
			dailyADX := indicator.ADX(dailyCandles, 14)
			dailyFastEMA := indicator.EMA(dailyCandles, 8)
			dailySlowEMA := indicator.EMA(dailyCandles, 21)

			n := len(dailyCandles) - 1
			fmt.Printf("\nDAILY INDICATORS:\n")
			fmt.Printf("  Last price: $%.2f\n", dailyCandles[n].Close)
			if len(dailyADX) > 0 {
				fmt.Printf("  ADX: %.2f (min required: 6.0)\n", dailyADX[len(dailyADX)-1])
			}
			fmt.Printf("  Fast EMA (8): %.2f\n", dailyFastEMA[n])
			fmt.Printf("  Slow EMA (21): %.2f\n", dailySlowEMA[n])
			fmt.Printf("  Trend: %s\n", map[bool]string{true: "BULLISH", false: "BEARISH"}[dailyFastEMA[n] > dailySlowEMA[n]])
		}

		// Calculate indicators on hourly
		if len(hourlyCandles) >= 50 {
			hourlyADX := indicator.ADX(hourlyCandles, 14)
			hourlyRSI := indicator.RSI(hourlyCandles, 14)
			hourlyMomentum := indicator.Momentum(hourlyCandles, 5)
			hourlyFastEMA := indicator.EMA(hourlyCandles, 8)
			hourlySlowEMA := indicator.EMA(hourlyCandles, 21)

			n := len(hourlyCandles) - 1
			fmt.Printf("\nHOURLY INDICATORS:\n")
			fmt.Printf("  Last price: $%.2f\n", hourlyCandles[n].Close)
			if len(hourlyADX) > 0 {
				fmt.Printf("  ADX: %.2f (min required: 12.0)\n", hourlyADX[len(hourlyADX)-1])
			}
			fmt.Printf("  RSI: %.2f\n", hourlyRSI[n])
			fmt.Printf("  Momentum: %.4f%% (min required: 0.1%%)\n", hourlyMomentum[n])
			fmt.Printf("  Fast EMA (8): %.2f\n", hourlyFastEMA[n])
			fmt.Printf("  Slow EMA (21): %.2f\n", hourlySlowEMA[n])
			fmt.Printf("  Price > Fast EMA: %v\n", hourlyCandles[n].Close > hourlyFastEMA[n])
			fmt.Printf("  Fast EMA > Slow EMA: %v\n", hourlyFastEMA[n] > hourlySlowEMA[n])

			// Check volume
			var sumVol float64
			for i := len(hourlyCandles) - 21; i < len(hourlyCandles)-1; i++ {
				sumVol += hourlyCandles[i].Volume
			}
			avgVol := sumVol / 20
			volRatio := hourlyCandles[n].Volume / avgVol
			fmt.Printf("  Volume ratio: %.2f (min required: 0.3)\n", volRatio)

			// Try to analyze with strategy
			strat := strategy.NewETFMomentumStrategy(symbol, strategy.DefaultETFMomentumConfig())

			// Test hourly-only analysis first
			signal, err := strat.Analyze(hourlyCandles)
			if err != nil {
				fmt.Printf("\nHOURLY-ONLY SIGNAL: None (%v)\n", err)
			} else if signal != nil {
				fmt.Printf("\nHOURLY-ONLY SIGNAL: %s @ $%.2f\n", signal.Type, signal.Price)
				fmt.Printf("  Reason: %s\n", signal.Reason)
			}

			// Test with daily trend
			if len(dailyCandles) >= 50 {
				signal, err = strat.AnalyzeWithDailyTrend(dailyCandles, hourlyCandles)
				if err != nil {
					fmt.Printf("DAILY+HOURLY SIGNAL: None (%v)\n", err)
				} else if signal != nil {
					fmt.Printf("DAILY+HOURLY SIGNAL: %s @ $%.2f\n", signal.Type, signal.Price)
					fmt.Printf("  Reason: %s\n", signal.Reason)
				}
			}
		}
	}
}
