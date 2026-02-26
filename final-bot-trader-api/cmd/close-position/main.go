package main

import (
	"context"
	"fmt"
	"os"

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

	// Get positions
	positions, err := client.GetPositions(ctx)
	if err != nil {
		fmt.Printf("Error getting positions: %v\n", err)
		return
	}

	if len(os.Args) < 2 {
		fmt.Println("Open positions:")
		for _, pos := range positions {
			if pos.PositionAmt != 0 {
				fmt.Printf("  %s: %s %.4f (ID: %s)\n",
					pos.Symbol, pos.Side, pos.PositionAmt, pos.PositionID)
			}
		}
		fmt.Println("\nUsage: close-position <symbol>")
		return
	}

	symbol := os.Args[1]

	for _, pos := range positions {
		if pos.Symbol == symbol && pos.PositionAmt != 0 {
			fmt.Printf("Closing %s position (ID: %s)...\n", symbol, pos.PositionID)

			err := client.FlashClosePosition(ctx, pos.PositionID)
			if err != nil {
				fmt.Printf("Error: %v\n", err)
			} else {
				fmt.Println("Position closed successfully!")
			}
			return
		}
	}

	fmt.Printf("No open position found for %s\n", symbol)
}
