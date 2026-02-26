package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"etf-bot-trader-api/internal/backtest"
	"etf-bot-trader-api/internal/exchange"

	"github.com/joho/godotenv"
)

func main() {
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println("  ETF TRADING BOT - BACKTEST")
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println()

	// Load .env
	godotenv.Load()

	apiKey := os.Getenv("ALPACA_API_KEY")
	apiSecret := os.Getenv("ALPACA_API_SECRET")

	if apiKey == "" || apiSecret == "" {
		fmt.Println("Error: ALPACA_API_KEY and ALPACA_API_SECRET must be set")
		os.Exit(1)
	}

	// Create Alpaca client (paper mode for data access)
	client := exchange.NewAlpacaClient(apiKey, apiSecret, true)
	ctx := context.Background()

	// Symbols to backtest
	symbols := []string{"SPY", "QQQ", "IWM"}

	// Time range (last 1 year)
	end := time.Now()
	start := end.AddDate(-1, 0, 0)

	fmt.Printf("Backtest period: %s to %s\n", start.Format("2006-01-02"), end.Format("2006-01-02"))
	fmt.Println()

	// Backtest configuration
	config := backtest.DefaultConfig()
	config.InitialCapital = 10000
	config.PositionSizePct = 0.15  // 15% per trade (need more for expensive ETFs like SPY ~$700)

	engine := backtest.NewEngine(config)

	for _, symbol := range symbols {
		fmt.Printf("Fetching data for %s...\n", symbol)

		// Get daily candles
		dailyCandles, err := client.GetBars(ctx, symbol, "1d", start, end)
		if err != nil {
			fmt.Printf("Error getting daily candles for %s: %v\n", symbol, err)
			continue
		}
		fmt.Printf("  Daily candles: %d\n", len(dailyCandles))

		// Get hourly candles (last 6 months due to data limits)
		hourlyStart := end.AddDate(0, -6, 0)
		hourlyCandles, err := client.GetBars(ctx, symbol, "1h", hourlyStart, end)
		if err != nil {
			fmt.Printf("Error getting hourly candles for %s: %v\n", symbol, err)
			continue
		}
		fmt.Printf("  Hourly candles: %d\n", len(hourlyCandles))

		// Run backtest
		result, err := engine.Run(symbol, dailyCandles, hourlyCandles)
		if err != nil {
			fmt.Printf("Error running backtest for %s: %v\n", symbol, err)
			continue
		}

		backtest.PrintResult(result)
	}

	fmt.Println()
	fmt.Println("Backtest complete!")
}
