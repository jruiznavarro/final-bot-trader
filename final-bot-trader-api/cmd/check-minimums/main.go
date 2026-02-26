package main

import (
	"context"
	"fmt"
	"os"

	"final-bot-trader-api/internal/exchange"
)

func main() {
	apiKey := os.Getenv("BITUNIX_API_KEY")
	secretKey := os.Getenv("BITUNIX_SECRET_KEY")
	baseURL := os.Getenv("BITUNIX_BASE_URL")

	client := exchange.NewBitunixClient(apiKey, secretKey, baseURL)
	ctx := context.Background()

	symbols := []string{
		"BTCUSDT",
		"ETHUSDT",
		"SOLUSDT",
		"DOGEUSDT",
		"XRPUSDT",
		"DOTUSDT",
		"1000SHIBUSDT",
		"UNIUSDT",
	}

	minOperation := 10.0 // 10 USDT minimum for copy trading

	fmt.Println("==============================================")
	fmt.Println("  COPY TRADING - 10 USDT MIN OPERATION")
	fmt.Println("  Balance: 100 USDT")
	fmt.Println("==============================================")
	fmt.Println("")

	// Test different leverage levels
	leverages := []int{3, 5, 10, 20}

	for _, leverage := range leverages {
		fmt.Printf("\n### LEVERAGE %dx ###\n", leverage)
		fmt.Printf("Con 10 USDT de margen = %.0f USDT de posicion\n\n", minOperation*float64(leverage))
		fmt.Printf("%-15s %10s %12s %12s %10s\n", "Symbol", "Price", "Qty @10USD", "Notional", "Valid?")
		fmt.Println("-------------------------------------------------------------")

		for _, symbol := range symbols {
			price, err := client.GetPrice(ctx, symbol)
			if err != nil {
				continue
			}

			// With 10 USDT margin and X leverage, notional = 10 * leverage
			notional := minOperation * float64(leverage)
			qty := notional / price

			// Check against known minimums
			var minQty float64
			switch symbol {
			case "BTCUSDT":
				minQty = 0.001
			case "ETHUSDT":
				minQty = 0.01
			case "SOLUSDT":
				minQty = 0.1
			case "DOGEUSDT":
				minQty = 53
			case "XRPUSDT":
				minQty = 10
			case "DOTUSDT":
				minQty = 1
			case "1000SHIBUSDT":
				minQty = 100
			case "UNIUSDT":
				minQty = 1
			default:
				minQty = 1
			}

			valid := "YES"
			if qty < minQty {
				valid = "NO"
			}

			fmt.Printf("%-15s %10.4f %12.4f %12.2f %10s\n",
				symbol, price, qty, notional, valid)
		}
	}

	fmt.Println("\n==============================================")
	fmt.Println("  RECOMENDACION PARA 100 USDT")
	fmt.Println("==============================================")
	fmt.Println("")
	fmt.Println("Con 100 USDT y operaciones de 10 USDT:")
	fmt.Println("  - Maximo 10 operaciones simultaneas")
	fmt.Println("  - Diversificacion: 5-7 pares diferentes")
	fmt.Println("")
	fmt.Println("Leverage recomendado: 5x-10x")
	fmt.Println("  - 5x = mas seguro, menos liquidaciones")
	fmt.Println("  - 10x = mas riesgo, mayor potencial")
	fmt.Println("")
}
