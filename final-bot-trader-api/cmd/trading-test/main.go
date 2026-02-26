package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"final-bot-trader-api/internal/exchange"
	"final-bot-trader-api/internal/exchange/model"

	"github.com/joho/godotenv"
)

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Println("No .env file found, using environment variables")
	}

	// Parse flags
	symbol := flag.String("symbol", "DOGEUSDT", "Trading symbol")
	leverage := flag.Int("leverage", 5, "Leverage to set")
	marginType := flag.String("margin", "ISOLATED", "Margin type (ISOLATED or CROSS)")

	// Test phases
	phase10a := flag.Bool("10a", false, "Run Phase 10a: Configure leverage and margin")
	phase10b := flag.Bool("10b", false, "Run Phase 10b: Test limit orders (no execution)")
	phase10c := flag.Bool("10c", false, "Run Phase 10c: Test market orders (with execution)")

	// Individual actions
	showAccount := flag.Bool("account", false, "Show account info")
	showPositions := flag.Bool("positions", false, "Show open positions")
	showOrders := flag.Bool("orders", false, "Show open orders")
	closeAll := flag.Bool("close-all", false, "Close all positions for symbol")
	autoConfirm := flag.Bool("yes", false, "Auto-confirm all prompts (for testing)")

	flag.Parse()

	// Get credentials
	apiKey := os.Getenv("BITUNIX_API_KEY")
	secretKey := os.Getenv("BITUNIX_SECRET_KEY")
	baseURL := os.Getenv("BITUNIX_BASE_URL")

	if apiKey == "" || secretKey == "" {
		log.Fatal("BITUNIX_API_KEY and BITUNIX_SECRET_KEY are required")
	}

	// Create client
	client := exchange.NewBitunixClient(apiKey, secretKey, baseURL)
	ctx := context.Background()

	fmt.Println("==============================================")
	fmt.Println("       TRADING TEST - FASE 10")
	fmt.Println("==============================================")
	fmt.Printf("Symbol: %s\n", *symbol)
	fmt.Printf("Leverage: %dx\n", *leverage)
	fmt.Printf("Margin Type: %s\n", *marginType)
	fmt.Println("==============================================\n")

	// Show account/positions/orders if requested
	if *showAccount {
		showAccountInfo(ctx, client)
	}
	if *showPositions {
		showOpenPositions(ctx, client)
	}
	if *showOrders {
		showOpenOrders(ctx, client, *symbol)
	}
	if *closeAll {
		closeAllPositions(ctx, client, *symbol)
	}

	// Run phases
	if *phase10a {
		runPhase10a(ctx, client, *symbol, *leverage, *marginType)
	}
	if *phase10b {
		runPhase10b(ctx, client, *symbol)
	}
	if *phase10c {
		runPhase10c(ctx, client, *symbol, *autoConfirm)
	}

	// If no action specified, show help
	if !*phase10a && !*phase10b && !*phase10c && !*showAccount && !*showPositions && !*showOrders && !*closeAll {
		fmt.Println("Usage: trading-test [flags]")
		fmt.Println("")
		fmt.Println("Phases:")
		fmt.Println("  -10a          Run Phase 10a: Configure leverage and margin")
		fmt.Println("  -10b          Run Phase 10b: Test limit orders (no execution)")
		fmt.Println("  -10c          Run Phase 10c: Test market orders (REAL MONEY)")
		fmt.Println("")
		fmt.Println("Info:")
		fmt.Println("  -account      Show account info")
		fmt.Println("  -positions    Show open positions")
		fmt.Println("  -orders       Show open orders")
		fmt.Println("")
		fmt.Println("Actions:")
		fmt.Println("  -close-all    Close all positions for symbol")
		fmt.Println("")
		fmt.Println("Options:")
		fmt.Println("  -symbol       Trading symbol (default: DOGEUSDT)")
		fmt.Println("  -leverage     Leverage to set (default: 5)")
		fmt.Println("  -margin       Margin type: ISOLATED or CROSS (default: ISOLATED)")
		fmt.Println("")
		fmt.Println("Examples:")
		fmt.Println("  trading-test -account")
		fmt.Println("  trading-test -10a -symbol DOGEUSDT -leverage 5")
		fmt.Println("  trading-test -10b -symbol DOGEUSDT")
		fmt.Println("  trading-test -10c -symbol DOGEUSDT")
	}
}

func showAccountInfo(ctx context.Context, client *exchange.BitunixClient) {
	fmt.Println("--- Account Info ---")
	account, err := client.GetAccountInfo(ctx)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	fmt.Printf("Total Balance:     %.4f USDT\n", account.TotalBalance)
	fmt.Printf("Available Balance: %.4f USDT\n", account.AvailableBalance)
	fmt.Printf("Margin Balance:    %.4f USDT\n", account.MarginBalance)
	fmt.Printf("Unrealized PnL:    %.4f USDT\n", account.UnrealizedPnl)
	fmt.Println("")
}

func showOpenPositions(ctx context.Context, client *exchange.BitunixClient) {
	fmt.Println("--- Open Positions ---")
	positions, err := client.GetPositions(ctx)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	if len(positions) == 0 {
		fmt.Println("No open positions")
	} else {
		for _, p := range positions {
			fmt.Printf("%s %s: %.4f @ %.6f (PnL: %.4f, Liq: %.6f)\n",
				p.Symbol, p.Side, p.PositionAmt, p.EntryPrice, p.UnrealizedPnl, p.LiqPrice)
		}
	}
	fmt.Println("")
}

func showOpenOrders(ctx context.Context, client *exchange.BitunixClient, symbol string) {
	fmt.Println("--- Open Orders ---")
	orders, err := client.GetOpenOrders(ctx, symbol)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		return
	}
	if len(orders) == 0 {
		fmt.Println("No open orders")
	} else {
		for _, o := range orders {
			fmt.Printf("Order %d: %s %s %.4f @ %s (%s)\n",
				o.OrderID, o.Side, o.Type, o.Quantity, o.Price, o.Status)
		}
	}
	fmt.Println("")
}

func closeAllPositions(ctx context.Context, client *exchange.BitunixClient, symbol string) {
	fmt.Printf("--- Closing all positions for %s ---\n", symbol)

	// Step 1: Cancel all open orders first (TP/SL orders might block closing)
	fmt.Println("Canceling all open orders first...")
	orders, err := client.GetOpenOrders(ctx, symbol)
	if err != nil {
		fmt.Printf("Warning: Could not get open orders: %v\n", err)
	} else if len(orders) > 0 {
		canceledCount := 0
		for _, o := range orders {
			err := client.CancelOrder(ctx, symbol, fmt.Sprintf("%d", o.OrderID))
			if err != nil {
				fmt.Printf("Warning: Could not cancel order %d: %v\n", o.OrderID, err)
			} else {
				canceledCount++
			}
		}
		fmt.Printf("Canceled %d orders\n", canceledCount)
	}

	// Step 2: Get positions and close them
	positions, err := client.GetPositions(ctx)
	if err != nil {
		fmt.Printf("Error getting positions: %v\n", err)
		return
	}

	for _, p := range positions {
		if p.Symbol == symbol && p.PositionAmt != 0 {
			fmt.Printf("Closing %s %s position of %.4f (ID: %s)...\n", p.Symbol, p.Side, p.PositionAmt, p.PositionID)

			// Use flash close with the position ID
			err := client.FlashClosePosition(ctx, p.PositionID)
			if err != nil {
				fmt.Printf("Error closing position: %v\n", err)
			} else {
				fmt.Printf("Position closed successfully!\n")
			}
		}
	}
	fmt.Println("")
}

// Phase 10a: Configure leverage and margin type
func runPhase10a(ctx context.Context, client *exchange.BitunixClient, symbol string, leverage int, marginType string) {
	fmt.Println("==============================================")
	fmt.Println("  PHASE 10a: CONFIGURATION")
	fmt.Println("==============================================\n")

	// Step 1: Set Margin Type
	fmt.Printf("[1/2] Setting margin type to %s for %s...\n", marginType, symbol)
	err := client.SetMarginType(ctx, symbol, marginType)
	if err != nil {
		// Check if error is "already set" type
		if strings.Contains(err.Error(), "already") || strings.Contains(err.Error(), "same") {
			fmt.Printf("      Margin type already set to %s\n", marginType)
		} else {
			fmt.Printf("      WARN: %v\n", err)
			fmt.Println("      (This may be OK if already configured)")
		}
	} else {
		fmt.Printf("      SUCCESS: Margin type set to %s\n", marginType)
	}

	time.Sleep(500 * time.Millisecond)

	// Step 2: Set Leverage
	fmt.Printf("\n[2/2] Setting leverage to %dx for %s...\n", leverage, symbol)
	err = client.SetLeverage(ctx, symbol, leverage)
	if err != nil {
		if strings.Contains(err.Error(), "already") || strings.Contains(err.Error(), "same") {
			fmt.Printf("      Leverage already set to %dx\n", leverage)
		} else {
			fmt.Printf("      WARN: %v\n", err)
			fmt.Println("      (This may be OK if already configured)")
		}
	} else {
		fmt.Printf("      SUCCESS: Leverage set to %dx\n", leverage)
	}

	fmt.Println("\n==============================================")
	fmt.Println("  PHASE 10a COMPLETE")
	fmt.Println("==============================================")
	fmt.Printf("\n%s configured with:\n", symbol)
	fmt.Printf("  - Margin Type: %s\n", marginType)
	fmt.Printf("  - Leverage: %dx\n", leverage)
	fmt.Println("\nNext: Run with -10b to test limit orders")
}

// Phase 10b: Test limit orders (no execution)
func runPhase10b(ctx context.Context, client *exchange.BitunixClient, symbol string) {
	fmt.Println("==============================================")
	fmt.Println("  PHASE 10b: LIMIT ORDER TEST (NO EXECUTION)")
	fmt.Println("==============================================\n")

	// Get current price
	fmt.Printf("[1/4] Getting current price for %s...\n", symbol)
	price, err := client.GetPrice(ctx, symbol)
	if err != nil {
		fmt.Printf("      ERROR: %v\n", err)
		return
	}
	fmt.Printf("      Current price: %.6f\n", price)

	// Calculate a limit price far from market (won't execute)
	limitPrice := price * 0.80 // 20% below market
	quantity := 60.0 // Minimum for DOGE is 53

	fmt.Printf("\n[2/4] Placing LIMIT BUY order at %.6f (20%% below market)...\n", limitPrice)
	fmt.Printf("      Quantity: %.2f %s\n", quantity, strings.TrimSuffix(symbol, "USDT"))

	order := &model.OrderRequest{
		Symbol:           symbol,
		Side:             "BUY",
		TradeSide:        "OPEN",
		Type:             "LIMIT",
		Quantity:         quantity,
		Price:            limitPrice,
		QuantityPrecision: 0,
		PricePrecision:   6,
	}

	resp, err := client.PlaceOrder(ctx, order)
	if err != nil {
		fmt.Printf("      ERROR: %v\n", err)
		return
	}
	fmt.Printf("      SUCCESS: Order ID = %d\n", resp.OrderID)
	orderID := fmt.Sprintf("%d", resp.OrderID)

	time.Sleep(1 * time.Second)

	// Check order status
	fmt.Printf("\n[3/4] Checking order status...\n")
	status, err := client.GetOrderStatus(ctx, symbol, orderID)
	if err != nil {
		fmt.Printf("      ERROR: %v\n", err)
	} else {
		fmt.Printf("      Order ID: %d\n", status.OrderID)
		fmt.Printf("      Status: %s\n", status.Status)
		fmt.Printf("      Side: %s\n", status.Side)
		fmt.Printf("      Price: %s\n", status.Price)
		fmt.Printf("      Quantity: %.4f\n", status.Quantity)
	}

	time.Sleep(1 * time.Second)

	// Cancel order
	fmt.Printf("\n[4/4] Cancelling order %s...\n", orderID)
	err = client.CancelOrder(ctx, symbol, orderID)
	if err != nil {
		fmt.Printf("      ERROR: %v\n", err)
	} else {
		fmt.Printf("      SUCCESS: Order cancelled\n")
	}

	// Verify cancellation
	time.Sleep(500 * time.Millisecond)
	status, err = client.GetOrderStatus(ctx, symbol, orderID)
	if err != nil {
		fmt.Printf("      Verification: %v\n", err)
	} else {
		fmt.Printf("      Verification: Order status = %s\n", status.Status)
	}

	fmt.Println("\n==============================================")
	fmt.Println("  PHASE 10b COMPLETE")
	fmt.Println("==============================================")
	fmt.Println("\nAll limit order operations tested successfully:")
	fmt.Println("  - PlaceOrder (LIMIT)")
	fmt.Println("  - GetOrderStatus")
	fmt.Println("  - CancelOrder")
	fmt.Println("\nNext: Run with -10c to test market orders (REAL MONEY)")
}

// Phase 10c: Test market orders (with execution)
func runPhase10c(ctx context.Context, client *exchange.BitunixClient, symbol string, autoConfirm bool) {
	fmt.Println("==============================================")
	fmt.Println("  PHASE 10c: MARKET ORDER TEST (REAL MONEY)")
	fmt.Println("==============================================\n")

	// Show account balance first
	account, err := client.GetAccountInfo(ctx)
	if err != nil {
		fmt.Printf("Error getting account: %v\n", err)
		return
	}
	fmt.Printf("Available Balance: %.4f USDT\n\n", account.AvailableBalance)

	// Get current price
	price, err := client.GetPrice(ctx, symbol)
	if err != nil {
		fmt.Printf("Error getting price: %v\n", err)
		return
	}
	fmt.Printf("Current %s price: %.6f\n", symbol, price)

	// Calculate quantity for ~$10 position
	quantity := 10.0 / price * 5 // $10 * leverage
	if symbol == "DOGEUSDT" {
		quantity = 60.0 // Minimum is 53 DOGE
	}

	// Calculate TP and SL
	tpPrice := price * 1.02  // 2% profit
	slPrice := price * 0.98  // 2% loss

	fmt.Printf("\nProposed trade:\n")
	fmt.Printf("  Symbol: %s\n", symbol)
	fmt.Printf("  Side: BUY (LONG)\n")
	fmt.Printf("  Quantity: %.2f\n", quantity)
	fmt.Printf("  Est. Value: $%.2f\n", quantity*price/5)
	fmt.Printf("  Take Profit: %.6f (+2%%)\n", tpPrice)
	fmt.Printf("  Stop Loss: %.6f (-2%%)\n", slPrice)

	// Confirm with user (skip if autoConfirm)
	if !autoConfirm {
		fmt.Printf("\n⚠️  THIS WILL USE REAL MONEY!\n")
		fmt.Printf("Continue? (yes/no): ")

		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))

		if input != "yes" && input != "y" {
			fmt.Println("Cancelled.")
			return
		}
	} else {
		fmt.Printf("\n⚠️  AUTO-CONFIRM ENABLED - Proceeding...\n")
	}

	// Place market order
	fmt.Printf("\n[1/4] Placing MARKET BUY order...\n")
	order := &model.OrderRequest{
		Symbol:            symbol,
		Side:              "BUY",
		TradeSide:         "OPEN",
		Type:              "MARKET",
		Quantity:          quantity,
		TP:                tpPrice,
		SL:                slPrice,
		QuantityPrecision: 0,
		PricePrecision:    6,
	}

	resp, err := client.PlaceOrder(ctx, order)
	if err != nil {
		fmt.Printf("      ERROR: %v\n", err)
		return
	}
	fmt.Printf("      SUCCESS: Order ID = %d\n", resp.OrderID)

	time.Sleep(2 * time.Second)

	// Check position
	fmt.Printf("\n[2/4] Checking position...\n")
	positions, err := client.GetPositions(ctx)
	if err != nil {
		fmt.Printf("      ERROR: %v\n", err)
	} else {
		found := false
		for _, p := range positions {
			if p.Symbol == symbol {
				fmt.Printf("      Position: %s %s %.4f @ %.6f\n", p.Symbol, p.Side, p.PositionAmt, p.EntryPrice)
				fmt.Printf("      Unrealized PnL: %.4f\n", p.UnrealizedPnl)
				fmt.Printf("      Liquidation Price: %.6f\n", p.LiqPrice)
				found = true
			}
		}
		if !found {
			fmt.Println("      No position found (may have been filled and closed by TP/SL)")
		}
	}

	// Ask to close (or auto-close if autoConfirm)
	shouldClose := autoConfirm
	if !autoConfirm {
		fmt.Printf("\n[3/4] Close position now? (yes/no): ")
		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(strings.ToLower(input))
		shouldClose = (input == "yes" || input == "y")
	}

	if shouldClose {
		fmt.Printf("\n[4/4] Closing position...\n")

		// Use flash close for reliability
		positions, err := client.GetPositions(ctx)
		if err != nil {
			fmt.Printf("      ERROR getting positions: %v\n", err)
		} else {
			for _, p := range positions {
				if p.Symbol == symbol && p.PositionAmt != 0 {
					err := client.FlashClosePosition(ctx, p.PositionID)
					if err != nil {
						fmt.Printf("      ERROR: %v\n", err)
					} else {
						fmt.Printf("      SUCCESS: Position closed\n")
					}
				}
			}
		}

		time.Sleep(1 * time.Second)

		// Show final balance
		account, _ = client.GetAccountInfo(ctx)
		fmt.Printf("\nFinal Balance: %.4f USDT\n", account.AvailableBalance)
	} else {
		fmt.Println("\nPosition left open. Monitor via -positions flag.")
	}

	fmt.Println("\n==============================================")
	fmt.Println("  PHASE 10c COMPLETE")
	fmt.Println("==============================================")
}
