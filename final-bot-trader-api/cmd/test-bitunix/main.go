package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"final-bot-trader-api/internal/exchange"

	"github.com/joho/godotenv"
)

func main() {
	// Load .env file if exists
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Parse flags
	testAll := flag.Bool("all", false, "Run all tests")
	testPrice := flag.Bool("price", false, "Test GetPrice")
	testKlines := flag.Bool("klines", false, "Test GetKlines")
	testAccount := flag.Bool("account", false, "Test GetAccountInfo")
	testBalance := flag.Bool("balance", false, "Test GetBalance")
	testPositions := flag.Bool("positions", false, "Test GetPositions")
	testOrders := flag.Bool("orders", false, "Test GetOpenOrders")
	testSymbol := flag.Bool("symbol", false, "Test GetSymbolInfo")
	symbol := flag.String("s", "BTCUSDT", "Symbol to test")
	interval := flag.String("i", "1h", "Interval for klines")
	limit := flag.Int("l", 100, "Limit for klines")

	flag.Parse()

	// Get credentials
	apiKey := os.Getenv("BITUNIX_API_KEY")
	secretKey := os.Getenv("BITUNIX_SECRET_KEY")
	baseURL := os.Getenv("BITUNIX_BASE_URL")

	if apiKey == "" || secretKey == "" {
		log.Println("WARNING: BITUNIX_API_KEY and/or BITUNIX_SECRET_KEY not set")
		log.Println("Some tests (account, balance, positions, orders) will fail")
		log.Println("Public endpoints (price, klines) should still work")
	}

	// Create client
	client := exchange.NewBitunixClient(apiKey, secretKey, baseURL)

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	fmt.Println("==============================================")
	fmt.Println("       BITUNIX API READ TESTS")
	fmt.Println("==============================================")
	fmt.Printf("Symbol: %s\n", *symbol)
	fmt.Printf("Base URL: %s\n", client.BaseURL)
	fmt.Println("==============================================\n")

	// Run tests based on flags
	if *testAll || *testPrice {
		runTestPrice(ctx, client, *symbol)
	}

	if *testAll || *testKlines {
		runTestKlines(ctx, client, *symbol, *interval, *limit)
	}

	if *testAll || *testSymbol {
		runTestSymbolInfo(ctx, client, *symbol)
	}

	if *testAll || *testAccount {
		runTestAccount(ctx, client)
	}

	if *testAll || *testBalance {
		runTestBalance(ctx, client)
	}

	if *testAll || *testPositions {
		runTestPositions(ctx, client)
	}

	if *testAll || *testOrders {
		runTestOrders(ctx, client, *symbol)
	}

	// If no specific test was requested, show help
	if !*testAll && !*testPrice && !*testKlines && !*testAccount &&
		!*testBalance && !*testPositions && !*testOrders && !*testSymbol {
		fmt.Println("Usage: test-bitunix [flags]")
		fmt.Println("")
		fmt.Println("Flags:")
		flag.PrintDefaults()
		fmt.Println("")
		fmt.Println("Examples:")
		fmt.Println("  test-bitunix -all                    # Run all tests")
		fmt.Println("  test-bitunix -price -s ETHUSDT       # Test price for ETH")
		fmt.Println("  test-bitunix -klines -s BTCUSDT -i 4h -l 200  # Get 200 4h candles")
		fmt.Println("  test-bitunix -account -balance       # Test account endpoints")
	}
}

func runTestPrice(ctx context.Context, client *exchange.BitunixClient, symbol string) {
	fmt.Println("----------------------------------------------")
	fmt.Println("TEST: GetPrice")
	fmt.Println("----------------------------------------------")

	start := time.Now()
	price, err := client.GetPrice(ctx, symbol)
	duration := time.Since(start)

	if err != nil {
		fmt.Printf("  FAIL: %v\n", err)
	} else {
		fmt.Printf("  Symbol: %s\n", symbol)
		fmt.Printf("  Price: %.8f\n", price)
		fmt.Printf("  Duration: %v\n", duration)
		fmt.Println("  PASS")
	}
	fmt.Println("")
}

func runTestKlines(ctx context.Context, client *exchange.BitunixClient, symbol, interval string, limit int) {
	fmt.Println("----------------------------------------------")
	fmt.Println("TEST: GetKlines")
	fmt.Println("----------------------------------------------")

	start := time.Now()
	candles, err := client.GetKlines(ctx, symbol, interval, limit, 0, 0)
	duration := time.Since(start)

	if err != nil {
		fmt.Printf("  FAIL: %v\n", err)
	} else {
		fmt.Printf("  Symbol: %s\n", symbol)
		fmt.Printf("  Interval: %s\n", interval)
		fmt.Printf("  Requested: %d candles\n", limit)
		fmt.Printf("  Received: %d candles\n", len(candles))
		fmt.Printf("  Duration: %v\n", duration)

		if len(candles) > 0 {
			first := candles[0]
			last := candles[len(candles)-1]
			fmt.Printf("  First candle: %s (O:%.2f H:%.2f L:%.2f C:%.2f)\n",
				first.OpenTime.Format("2006-01-02 15:04"),
				first.Open, first.High, first.Low, first.Close)
			fmt.Printf("  Last candle:  %s (O:%.2f H:%.2f L:%.2f C:%.2f)\n",
				last.OpenTime.Format("2006-01-02 15:04"),
				last.Open, last.High, last.Low, last.Close)
		}
		fmt.Println("  PASS")
	}
	fmt.Println("")
}

func runTestSymbolInfo(ctx context.Context, client *exchange.BitunixClient, symbol string) {
	fmt.Println("----------------------------------------------")
	fmt.Println("TEST: GetSymbolInfo")
	fmt.Println("----------------------------------------------")

	start := time.Now()
	info, err := client.GetSymbolInfo(ctx, symbol)
	duration := time.Since(start)

	if err != nil {
		fmt.Printf("  FAIL: %v\n", err)
	} else {
		fmt.Printf("  Symbol: %s\n", info.Symbol)
		fmt.Printf("  Min Quantity: %.6f\n", info.MinQuantity)
		fmt.Printf("  Step Size: %.6f\n", info.StepSize)
		fmt.Printf("  Min Price: %.6f\n", info.MinPrice)
		fmt.Printf("  Tick Size: %.6f\n", info.TickSize)
		fmt.Printf("  Duration: %v\n", duration)
		fmt.Println("  PASS")
	}
	fmt.Println("")
}

func runTestAccount(ctx context.Context, client *exchange.BitunixClient) {
	fmt.Println("----------------------------------------------")
	fmt.Println("TEST: GetAccountInfo")
	fmt.Println("----------------------------------------------")

	start := time.Now()
	account, err := client.GetAccountInfo(ctx)
	duration := time.Since(start)

	if err != nil {
		if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "sign") {
			fmt.Println("  SKIP: Authentication required (check API credentials)")
		} else {
			fmt.Printf("  FAIL: %v\n", err)
		}
	} else {
		fmt.Printf("  Total Balance: %.4f USDT\n", account.TotalBalance)
		fmt.Printf("  Available Balance: %.4f USDT\n", account.AvailableBalance)
		fmt.Printf("  Margin Balance: %.4f USDT\n", account.MarginBalance)
		fmt.Printf("  Unrealized PnL: %.4f USDT\n", account.UnrealizedPnl)
		fmt.Printf("  Duration: %v\n", duration)
		fmt.Println("  PASS")
	}
	fmt.Println("")
}

func runTestBalance(ctx context.Context, client *exchange.BitunixClient) {
	fmt.Println("----------------------------------------------")
	fmt.Println("TEST: GetBalance")
	fmt.Println("----------------------------------------------")

	start := time.Now()
	balances, err := client.GetBalance(ctx)
	duration := time.Since(start)

	if err != nil {
		if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "sign") {
			fmt.Println("  SKIP: Authentication required (check API credentials)")
		} else {
			fmt.Printf("  FAIL: %v\n", err)
		}
	} else {
		fmt.Printf("  Number of assets: %d\n", len(balances))
		for _, b := range balances {
			if b.Balance > 0 {
				fmt.Printf("    %s: %.4f (available: %.4f)\n",
					b.Asset, b.Balance, b.AvailableBalance)
			}
		}
		fmt.Printf("  Duration: %v\n", duration)
		fmt.Println("  PASS")
	}
	fmt.Println("")
}

func runTestPositions(ctx context.Context, client *exchange.BitunixClient) {
	fmt.Println("----------------------------------------------")
	fmt.Println("TEST: GetPositions")
	fmt.Println("----------------------------------------------")

	start := time.Now()
	positions, err := client.GetPositions(ctx)
	duration := time.Since(start)

	if err != nil {
		if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "sign") {
			fmt.Println("  SKIP: Authentication required (check API credentials)")
		} else {
			fmt.Printf("  FAIL: %v\n", err)
		}
	} else {
		fmt.Printf("  Open positions: %d\n", len(positions))
		for _, p := range positions {
			fmt.Printf("    %s %s: %.4f @ %.2f (PnL: %.4f)\n",
				p.Symbol, p.Side, p.PositionAmt, p.EntryPrice, p.UnrealizedPnl)
		}
		if len(positions) == 0 {
			fmt.Println("    (no open positions)")
		}
		fmt.Printf("  Duration: %v\n", duration)
		fmt.Println("  PASS")
	}
	fmt.Println("")
}

func runTestOrders(ctx context.Context, client *exchange.BitunixClient, symbol string) {
	fmt.Println("----------------------------------------------")
	fmt.Println("TEST: GetOpenOrders")
	fmt.Println("----------------------------------------------")

	start := time.Now()
	orders, err := client.GetOpenOrders(ctx, symbol)
	duration := time.Since(start)

	if err != nil {
		if strings.Contains(err.Error(), "401") || strings.Contains(err.Error(), "sign") {
			fmt.Println("  SKIP: Authentication required (check API credentials)")
		} else {
			fmt.Printf("  FAIL: %v\n", err)
		}
	} else {
		fmt.Printf("  Open orders for %s: %d\n", symbol, len(orders))
		for _, o := range orders {
			fmt.Printf("    Order %d: %s %s %.4f @ %s (%s)\n",
				o.OrderID, o.Side, o.Type, o.Quantity, o.Price, o.Status)
		}
		if len(orders) == 0 {
			fmt.Println("    (no open orders)")
		}
		fmt.Printf("  Duration: %v\n", duration)
		fmt.Println("  PASS")
	}
	fmt.Println("")
}

// prettyJSON formats JSON for display
func prettyJSON(data interface{}) string {
	b, err := json.MarshalIndent(data, "  ", "  ")
	if err != nil {
		return fmt.Sprintf("%v", data)
	}
	return string(b)
}
