package main

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"final-bot-trader-api/internal/exchange"
	"final-bot-trader-api/internal/exchange/model"
	"final-bot-trader-api/internal/strategy"
	"final-bot-trader-api/internal/strategy/multifactor"

	"github.com/joho/godotenv"
)

// Symbols to test
var symbols = []string{
	"DOGEUSDT", "WLDUSDT", "1000PEPEUSDT", "ARBUSDT", "AAVEUSDT",
	"WIFUSDT", "FILUSDT", "SOLUSDT", "TAOUSDT", "SUIUSDT",
}

func main() {
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println("  PAPER TRADING - SINGLE RUN TEST")
	fmt.Println("  Testing signal generation with live data")
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println()

	// Load .env file
	godotenv.Load()

	// Load environment variables
	apiKey := os.Getenv("BITUNIX_API_KEY")
	apiSecret := os.Getenv("BITUNIX_SECRET_KEY") // Note: SECRET_KEY not API_SECRET

	if apiKey == "" || apiSecret == "" {
		fmt.Println("Error: BITUNIX_API_KEY and BITUNIX_SECRET_KEY must be set")
		fmt.Println("Check your .env file")
		os.Exit(1)
	}

	// Create Bitunix client
	client := exchange.NewBitunixClient(apiKey, apiSecret, "")
	ctx := context.Background()

	fmt.Println("Testing connection and signal generation...\n")

	// Create strategy config with relaxed volume threshold
	stratConfig := multifactor.DefaultConfig()
	stratConfig.VolumeThreshold = 0.7 // Relaxed for testing

	var signalCount, longCount, shortCount int

	fmt.Printf("%-14s %10s %10s %8s %10s %10s %s\n",
		"Symbol", "Price", "ADX", "Regime", "Signal", "Conf", "Reason")
	fmt.Println(strings.Repeat("─", 90))

	for _, symbol := range symbols {
		// Get candles
		candles, err := client.GetKlines(ctx, symbol, "4h", 100, 0, 0)
		if err != nil {
			fmt.Printf("%-14s Error: %v\n", symbol, err)
			continue
		}

		if len(candles) < 60 {
			fmt.Printf("%-14s Insufficient data (%d candles)\n", symbol, len(candles))
			continue
		}

		currentPrice := candles[len(candles)-1].Close

		// Detect regime
		regime := detectRegime(candles)

		// Get strategy signal
		strat := multifactor.NewMultiFactorStrategy(symbol, stratConfig)
		signal, err := strat.Analyze(candles)

		signalStr := "NONE"
		confStr := "-"
		reason := "-"

		if err == nil && signal != nil {
			signalCount++
			if signal.Type == strategy.SignalBuy {
				signalStr = "\033[32mLONG\033[0m"
				longCount++
			} else if signal.Type == strategy.SignalSell {
				signalStr = "\033[31mSHORT\033[0m"
				shortCount++
			}
			confStr = fmt.Sprintf("%.0f%%", signal.Confidence*100)
			reason = truncate(signal.Reason, 30)
		} else if err != nil && err != strategy.ErrNoSignal {
			reason = truncate(err.Error(), 30)
		}

		fmt.Printf("%-14s %10.4f %10s %8s %10s %10s %s\n",
			symbol, currentPrice, regime.adx, regime.name, signalStr, confStr, reason)

		time.Sleep(200 * time.Millisecond) // Rate limiting
	}

	fmt.Println(strings.Repeat("─", 90))
	fmt.Println()
	fmt.Println("Summary:")
	fmt.Printf("  Total Symbols:  %d\n", len(symbols))
	fmt.Printf("  Active Signals: %d\n", signalCount)
	fmt.Printf("  Long Signals:   %d\n", longCount)
	fmt.Printf("  Short Signals:  %d\n", shortCount)
	fmt.Printf("  No Signal:      %d\n", len(symbols)-signalCount)

	if signalCount > 0 {
		fmt.Println()
		fmt.Println("✅ Paper trading system is working correctly!")
		fmt.Println("   Run 'paper-trading' to start continuous monitoring.")
	} else {
		fmt.Println()
		fmt.Println("ℹ️  No signals currently active.")
		fmt.Println("   This is normal - strategy waits for optimal conditions.")
		fmt.Println("   Run 'paper-trading' to monitor continuously.")
	}
}

type regimeInfo struct {
	name string
	adx  string
}

func detectRegime(candles []model.Candle) regimeInfo {
	detector := multifactor.DefaultRegimeDetector()
	regime, adx, _ := detector.DetectRegime(candles)

	return regimeInfo{
		name: regime.String(),
		adx:  fmt.Sprintf("%.1f", adx),
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
