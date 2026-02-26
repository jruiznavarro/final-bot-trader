package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"final-bot-trader-api/internal/exchange"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	client := exchange.NewBitunixClient(
		os.Getenv("BITUNIX_API_KEY"),
		os.Getenv("BITUNIX_SECRET_KEY"),
		"",
	)

	ctx := context.Background()

	fmt.Println("══════════════════════════════════════════════════════════")
	fmt.Printf("  MONITOR - %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println("══════════════════════════════════════════════════════════")

	positions, err := client.GetPositions(ctx)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}

	if len(positions) == 0 {
		fmt.Println("\n  No hay posiciones abiertas")
		fmt.Println("  (TP o SL se activaron)")
		return
	}

	var totalPnL float64

	fmt.Println()
	fmt.Printf("  %-14s %-6s %12s %12s %12s\n", "Symbol", "Side", "Size", "Entry", "PnL")
	fmt.Println("  ──────────────────────────────────────────────────────────")

	for _, pos := range positions {
		if pos.PositionAmt == 0 {
			continue
		}

		pnlColor := "\033[32m" // Green
		if pos.UnrealizedPnl < 0 {
			pnlColor = "\033[31m" // Red
		}
		reset := "\033[0m"

		fmt.Printf("  %-14s %-6s %12.4f %12.4f %s%+12.4f%s\n",
			pos.Symbol,
			pos.Side,
			pos.PositionAmt,
			pos.EntryPrice,
			pnlColor,
			pos.UnrealizedPnl,
			reset)

		totalPnL += pos.UnrealizedPnl
	}

	fmt.Println("  ──────────────────────────────────────────────────────────")

	totalColor := "\033[32m"
	if totalPnL < 0 {
		totalColor = "\033[31m"
	}
	fmt.Printf("  %-14s %-6s %12s %12s %s%+12.4f%s\n",
		"TOTAL", "", "", "", totalColor, totalPnL, "\033[0m")

	fmt.Println()
}
