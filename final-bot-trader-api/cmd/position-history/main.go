// position-history prints recent closed positions and current funding rates
// from Bitunix. Useful to audit the bot's real (exchange-reported) PnL.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"final-bot-trader-api/internal/exchange"

	"github.com/joho/godotenv"
)

func main() {
	symbol := flag.String("symbol", "", "symbol filter (empty = all)")
	limit := flag.Int("limit", 20, "max positions to fetch")
	funding := flag.String("funding", "", "also print funding rate for this symbol")
	flag.Parse()

	godotenv.Load()
	apiKey := os.Getenv("BITUNIX_API_KEY")
	apiSecret := os.Getenv("BITUNIX_SECRET_KEY")
	if apiKey == "" || apiSecret == "" {
		fmt.Println("BITUNIX_API_KEY and BITUNIX_SECRET_KEY required")
		os.Exit(1)
	}

	client := exchange.NewBitunixClient(apiKey, apiSecret, "")
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	positions, err := client.GetHistoryPositions(ctx, *symbol, *limit)
	if err != nil {
		fmt.Println("error:", err)
		os.Exit(1)
	}

	fmt.Printf("%d closed positions:\n", len(positions))
	fmt.Printf("%-13s %-6s %12s %12s %10s %8s %8s  %s\n",
		"symbol", "side", "entry", "close", "realPnL", "fee", "funding", "closed")
	var total, fees, fund float64
	for _, p := range positions {
		closed := time.UnixMilli(p.CloseTime).Format("2006-01-02 15:04")
		fmt.Printf("%-13s %-6s %12.6f %12.6f %+10.4f %+8.4f %+8.4f  %s\n",
			p.Symbol, p.Side, p.EntryPrice, p.ClosePrice, p.RealizedPnL, p.Fee, p.Funding, closed)
		total += p.RealizedPnL
		fees += p.Fee
		fund += p.Funding
	}
	fmt.Printf("\nTOTAL realizedPnL: %+.4f | fees: %+.4f | funding: %+.4f\n", total, fees, fund)

	if *funding != "" {
		rate, err := client.GetFundingRate(ctx, *funding)
		if err != nil {
			fmt.Println("funding rate error:", err)
		} else {
			fmt.Printf("\n%s funding rate: %+.6f%% per 8h\n", *funding, rate*100)
		}
	}
}
