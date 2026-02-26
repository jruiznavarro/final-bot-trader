package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"final-bot-trader-api/internal/exchange"
	"final-bot-trader-api/internal/papertrading"

	"github.com/joho/godotenv"
)

func main() {
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println("  MULTI-FACTOR STRATEGY - PAPER TRADING")
	fmt.Println("  Real-time simulation with Bitunix data")
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println()

	// Load .env file
	godotenv.Load()

	// Load environment variables
	apiKey := os.Getenv("BITUNIX_API_KEY")
	apiSecret := os.Getenv("BITUNIX_SECRET_KEY")

	if apiKey == "" || apiSecret == "" {
		fmt.Println("Error: BITUNIX_API_KEY and BITUNIX_SECRET_KEY must be set")
		fmt.Println("Check your .env file")
		os.Exit(1)
	}

	// Create Bitunix client (empty baseURL uses default)
	client := exchange.NewBitunixClient(apiKey, apiSecret, "")

	// Paper trading configuration
	config := papertrading.DefaultConfig()
	config.InitialBalance = 10000   // Start with $10,000
	config.PositionSize = 0.10      // 10% per position
	config.Commission = 0.0005      // 0.05% Bitunix fee
	config.Slippage = 0.0003        // 0.03% estimated slippage
	config.Leverage = 5             // 5x leverage
	config.Interval = "4h"          // 4-hour candles
	config.StateFile = "paper_trading_state.json"
	config.VolumeThreshold = 0.7    // Relaxed for paper trading (default 1.0)

	fmt.Println("Configuration:")
	fmt.Printf("  Initial Balance:  $%.2f\n", config.InitialBalance)
	fmt.Printf("  Position Size:    %.0f%% of balance\n", config.PositionSize*100)
	fmt.Printf("  Leverage:         %dx\n", config.Leverage)
	fmt.Printf("  Commission:       %.2f%%\n", config.Commission*100)
	fmt.Printf("  Interval:         %s\n", config.Interval)
	fmt.Printf("  Symbols:          %d coins\n", len(config.Symbols))
	fmt.Println()

	// Create paper trading engine
	engine := papertrading.NewEngine(client, config)

	// Try to load existing state
	if err := engine.LoadState(); err != nil {
		fmt.Printf("Warning: Could not load state: %v\n", err)
	} else {
		status := engine.GetStatus()
		if status["total_trades"].(int) > 0 {
			fmt.Println("Resuming from saved state:")
			printStatus(status)
		}
	}

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		fmt.Println("\n\nShutting down gracefully...")
		cancel()
	}()

	// Status display ticker (every 5 minutes)
	statusTicker := time.NewTicker(5 * time.Minute)
	defer statusTicker.Stop()

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-statusTicker.C:
				printStatus(engine.GetStatus())
			}
		}
	}()

	fmt.Println("Starting paper trading...")
	fmt.Printf("Monitoring %d symbols on %s timeframe\n", len(config.Symbols), config.Interval)
	fmt.Println("Press Ctrl+C to stop")
	fmt.Println()
	fmt.Println(strings.Repeat("─", 60))

	// Run the paper trading engine
	if err := engine.Run(ctx); err != nil && err != context.Canceled {
		fmt.Printf("Error: %v\n", err)
	}

	// Final status
	fmt.Println()
	fmt.Println(strings.Repeat("═", 60))
	fmt.Println("  FINAL STATUS")
	fmt.Println(strings.Repeat("═", 60))
	printStatus(engine.GetStatus())

	// Print trade summary
	trades := engine.GetClosedTrades()
	if len(trades) > 0 {
		fmt.Println()
		fmt.Println("Recent Trades:")
		fmt.Println(strings.Repeat("─", 60))

		start := 0
		if len(trades) > 10 {
			start = len(trades) - 10
		}

		for _, trade := range trades[start:] {
			pnlSign := "+"
			if trade.PnL < 0 {
				pnlSign = ""
			}
			fmt.Printf("  %s %s %s: %s$%.2f (%.2f%%)\n",
				trade.ExitTime.Format("01-02 15:04"),
				trade.Side,
				trade.Symbol,
				pnlSign,
				trade.PnL,
				trade.PnLPercent)
		}
	}

	// Save final state
	if err := engine.SaveState(); err != nil {
		fmt.Printf("Error saving state: %v\n", err)
	} else {
		fmt.Println()
		fmt.Println("State saved to:", config.StateFile)
	}
}

func printStatus(status map[string]interface{}) {
	fmt.Println()
	fmt.Println(strings.Repeat("─", 60))
	fmt.Printf("  Status as of %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println(strings.Repeat("─", 60))

	returnPct := status["return_pct"].(float64)
	returnColor := "\033[32m" // Green
	if returnPct < 0 {
		returnColor = "\033[31m" // Red
	}
	resetColor := "\033[0m"

	fmt.Printf("  Initial Balance:   $%.2f\n", status["initial_balance"])
	fmt.Printf("  Current Equity:    $%.2f\n", status["equity"])
	fmt.Printf("  Total PnL:         %s%+.2f%s\n", returnColor, status["total_pnl"], resetColor)
	fmt.Printf("  Return:            %s%+.2f%%%s\n", returnColor, returnPct, resetColor)
	fmt.Println()
	fmt.Printf("  Open Positions:    %d\n", status["open_positions"])
	fmt.Printf("  Total Trades:      %d\n", status["total_trades"])
	fmt.Printf("  Wins/Losses:       %d / %d\n", status["wins"], status["losses"])
	fmt.Printf("  Win Rate:          %.1f%%\n", status["win_rate"])
	fmt.Println(strings.Repeat("─", 60))
}
