package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"etf-bot-trader-api/internal/exchange"
	"etf-bot-trader-api/internal/livetrading"

	"github.com/joho/godotenv"
)

func main() {
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println("  ETF TRADING BOT - PAPER TRADING MODE")
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println()

	// Load .env
	godotenv.Load()

	apiKey := os.Getenv("ALPACA_API_KEY")
	apiSecret := os.Getenv("ALPACA_API_SECRET")

	if apiKey == "" || apiSecret == "" {
		fmt.Println("Error: ALPACA_API_KEY and ALPACA_API_SECRET must be set")
		fmt.Println("Create a .env file with:")
		fmt.Println("  ALPACA_API_KEY=your_key")
		fmt.Println("  ALPACA_API_SECRET=your_secret")
		os.Exit(1)
	}

	// Create Alpaca client (paper mode)
	client := exchange.NewAlpacaClient(apiKey, apiSecret, true)

	// Test connection
	ctx := context.Background()
	account, err := client.GetAccount(ctx)
	if err != nil {
		fmt.Printf("Error connecting to Alpaca: %v\n", err)
		os.Exit(1)
	}

	fmt.Println("Connected to Alpaca (Paper Trading)")
	fmt.Printf("  Account Equity:  $%.2f\n", account.Equity)
	fmt.Printf("  Buying Power:    $%.2f\n", account.BuyingPower)
	fmt.Printf("  Cash:            $%.2f\n", account.Cash)
	fmt.Println()

	// Check market status
	isOpen, err := client.IsMarketOpen(ctx)
	if err != nil {
		log.Printf("Warning: Could not check market status: %v", err)
	} else if isOpen {
		fmt.Println("Market Status: OPEN")
	} else {
		nextOpen, _ := client.GetNextMarketOpen(ctx)
		fmt.Printf("Market Status: CLOSED (opens %s)\n", nextOpen.Format("Mon Jan 2 15:04 MST"))
	}
	fmt.Println()

	// Create configuration
	config := livetrading.DefaultConfig()
	config.PositionSizeUSD = 1000   // $1000 per trade (paper money)
	config.PositionSizePct = 0.05   // 5% of equity per trade
	config.MaxDailyLoss = 200       // Stop if losing $200/day
	config.MaxDailyTrades = 8       // Max 8 trades/day
	config.MaxOpenPositions = 2     // Max 2 positions at once
	config.CheckInterval = 5 * time.Minute

	// Display configuration
	fmt.Println("Configuration:")
	fmt.Printf("  Symbols:           %v\n", config.Symbols)
	fmt.Printf("  Position Size:     $%.0f (or %.0f%% of equity)\n", config.PositionSizeUSD, config.PositionSizePct*100)
	fmt.Printf("  Max Daily Loss:    $%.0f\n", config.MaxDailyLoss)
	fmt.Printf("  Max Daily Trades:  %d\n", config.MaxDailyTrades)
	fmt.Printf("  Max Open Positions: %d\n", config.MaxOpenPositions)
	fmt.Printf("  Check Interval:    %v\n", config.CheckInterval)
	fmt.Println()

	// Create engine
	engine := livetrading.NewEngine(client, config)

	// Load previous state
	if err := engine.LoadState(); err != nil {
		fmt.Printf("Warning: Could not load state: %v\n", err)
	}

	// Setup context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("\n\nShutting down...")
		cancel()
	}()

	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("  PAPER TRADING STARTED")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println()
	fmt.Println("Monitoring signals... Press Ctrl+C to stop")
	fmt.Println()

	// Run engine
	if err := engine.Run(ctx); err != nil && err != context.Canceled {
		fmt.Printf("Error: %v\n", err)
	}

	// Final status
	fmt.Println()
	fmt.Println("═══════════════════════════════════════════════════════════════")
	fmt.Println("  FINAL STATUS")
	fmt.Println("═══════════════════════════════════════════════════════════════")
	printStatus(engine.GetStatus())

	// Save state
	if err := engine.SaveState(); err != nil {
		fmt.Printf("Error saving state: %v\n", err)
	} else {
		fmt.Println()
		fmt.Println("State saved to:", config.StateFile)
	}
}

func printStatus(status map[string]interface{}) {
	fmt.Printf("  Mode:            PAPER TRADING\n")
	fmt.Printf("  Symbols:         %v\n", status["symbols"])
	fmt.Printf("  Open Positions:  %d\n", status["open_positions"])
	fmt.Printf("  Closed Trades:   %d\n", status["closed_trades"])
	fmt.Printf("  Today's Trades:  %d\n", status["daily_trades"])

	dailyPnL := status["daily_pnl"].(float64)
	color := "\033[32m"
	if dailyPnL < 0 {
		color = "\033[31m"
	}
	fmt.Printf("  Today's PnL:     %s$%+.2f\033[0m\n", color, dailyPnL)

	totalPnL := status["total_pnl"].(float64)
	color = "\033[32m"
	if totalPnL < 0 {
		color = "\033[31m"
	}
	fmt.Printf("  Total PnL:       %s$%+.2f\033[0m\n", color, totalPnL)
	fmt.Printf("  Win Rate:        %.1f%%\n", status["win_rate"])
}
