package main

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"final-bot-trader-api/internal/api"
	"final-bot-trader-api/internal/database"
	"final-bot-trader-api/internal/exchange"
	"final-bot-trader-api/internal/livetrading"
	"final-bot-trader-api/internal/telegram"

	"github.com/joho/godotenv"
)

func main() {
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println("  ⚠️  LIVE TRADING - REAL MONEY")
	fmt.Println("══════════════════════════════════════════════════════════════")
	fmt.Println()

	// Load .env
	godotenv.Load()

	apiKey := os.Getenv("BITUNIX_API_KEY")
	apiSecret := os.Getenv("BITUNIX_SECRET_KEY")

	if apiKey == "" || apiSecret == "" {
		fmt.Println("Error: API credentials not set")
		os.Exit(1)
	}

	// Parse command line flags
	dryRun := false
	autoConfirm := false
	for _, arg := range os.Args[1:] {
		if arg == "--dry-run" || arg == "-d" {
			dryRun = true
		}
		if arg == "--auto-confirm" || arg == "-y" {
			autoConfirm = true
		}
	}

	// Create configuration
	config := livetrading.DefaultConfig()
	config.PositionSizeUSDT = 10   // Minimum: $10 per trade
	config.Leverage = 3            // 3x leverage (conservative)
	config.MaxDailyLoss = 50       // Stop if losing $50/day
	config.MaxDailyTrades = 20     // Max 20 trades/day
	config.MaxOpenPositions = 3    // Max 3 positions at once
	config.VolumeThreshold = 0.7   // Relaxed volume threshold
	config.DryRun = dryRun

	// Display configuration
	fmt.Println("Configuration:")
	fmt.Printf("  Position Size:      $%.2f USDT\n", config.PositionSizeUSDT)
	fmt.Printf("  Leverage:           %dx\n", config.Leverage)
	fmt.Printf("  Max Daily Loss:     $%.2f\n", config.MaxDailyLoss)
	fmt.Printf("  Max Daily Trades:   %d\n", config.MaxDailyTrades)
	fmt.Printf("  Max Open Positions: %d\n", config.MaxOpenPositions)
	if config.UseMultiTimeframe {
		fmt.Printf("  Strategy:           Multi-Timeframe (%s + %s)\n", config.PrimaryInterval, config.EntryInterval)
	} else {
		fmt.Printf("  Interval:           %s\n", config.Interval)
	}
	fmt.Printf("  Symbols:            %d coins\n", len(config.Symbols))
	fmt.Printf("  Mode:               %s\n", modeString(dryRun))
	fmt.Println()

	// Safety confirmation (skip for dry run or auto-confirm)
	if !dryRun && !autoConfirm {
		fmt.Println("⚠️  WARNING: This will trade with REAL MONEY!")
		fmt.Println()
		fmt.Print("Type 'YES' to confirm: ")

		reader := bufio.NewReader(os.Stdin)
		input, _ := reader.ReadString('\n')
		input = strings.TrimSpace(input)

		if input != "YES" {
			fmt.Println("Aborted. Use --dry-run flag for simulation mode.")
			os.Exit(0)
		}
		fmt.Println()
	} else if autoConfirm {
		fmt.Println("Auto-confirm enabled (24/7 mode)")
		fmt.Println()
	}

	// Create Telegram client
	tgToken := os.Getenv("TELEGRAM_BOT_TOKEN")
	tgChatID := os.Getenv("TELEGRAM_CHAT_ID")
	tgClient := telegram.NewClient(tgToken, tgChatID)

	if tgClient.IsConfigured() {
		fmt.Println("Telegram notifications: ENABLED")
	} else {
		fmt.Println("Telegram notifications: DISABLED (set TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID)")
	}
	fmt.Println()

	// Create client and engine
	client := exchange.NewBitunixClient(apiKey, apiSecret, "")
	engine := livetrading.NewEngine(client, tgClient, config)

	// Connect to database (optional)
	dbConfig := database.ConfigFromEnv()
	db, err := database.Connect(dbConfig)
	if err != nil {
		fmt.Printf("Database: DISABLED (connection failed: %v)\n", err)
	} else {
		defer db.Close()
		if err := db.RunMigrations(); err != nil {
			fmt.Printf("Database: WARNING (migrations failed: %v)\n", err)
		} else {
			fmt.Println("Database: ENABLED (PostgreSQL)")
			tradeRepo := database.NewTradeRepository(db)
			engine.SetTradeRepository(tradeRepo)
		}
	}
	fmt.Println()

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

	// Initialize (set leverage, get symbol info)
	fmt.Println("Initializing...")
	if err := engine.Initialize(ctx); err != nil {
		fmt.Printf("Error initializing: %v\n", err)
		os.Exit(1)
	}

	// Start trailing stop manager (only in live mode)
	var trailingStopMgr *livetrading.TrailingStopManager
	if !dryRun {
		tsConfig := livetrading.DefaultTrailingStopConfig()
		trailingStopMgr = livetrading.NewTrailingStopManager(client, engine, tgClient, tsConfig)
		go trailingStopMgr.Start(ctx)
	} else {
		fmt.Println("Trailing stop: DISABLED (dry run mode)")
	}

	// Status ticker
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

	fmt.Println()
	fmt.Println(strings.Repeat("═", 60))
	fmt.Printf("  LIVE TRADING STARTED - %s\n", modeString(dryRun))
	fmt.Println(strings.Repeat("═", 60))
	fmt.Println()
	fmt.Println("Monitoring signals... Press Ctrl+C to stop")
	fmt.Println()

	// Start API server
	apiPort := os.Getenv("API_PORT")
	if apiPort == "" {
		apiPort = "8080"
	}
	apiServer := api.NewBotServer(engine, apiPort)
	go func() {
		if err := apiServer.Start(); err != nil && err.Error() != "http: Server closed" {
			log.Printf("API server error: %v", err)
		}
	}()
	fmt.Printf("API server running on http://localhost:%s\n", apiPort)
	fmt.Printf("  GET /health           - Health check\n")
	fmt.Printf("  GET /api/v1/bot/status    - Bot status\n")
	fmt.Printf("  GET /api/v1/bot/positions - Open positions\n")
	fmt.Printf("  GET /api/v1/bot/trades    - Trade stats\n")
	fmt.Println()

	// Send startup notification
	engine.NotifyStartup()

	// Run engine
	if err := engine.Run(ctx); err != nil && err != context.Canceled {
		fmt.Printf("Error: %v\n", err)
	}

	// Stop trailing stop manager
	if trailingStopMgr != nil {
		trailingStopMgr.Stop()
	}

	// Shutdown API server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	apiServer.Shutdown(shutdownCtx)

	// Send shutdown notification
	engine.NotifyShutdown()

	// Final status
	fmt.Println()
	fmt.Println(strings.Repeat("═", 60))
	fmt.Println("  FINAL STATUS")
	fmt.Println(strings.Repeat("═", 60))
	printStatus(engine.GetStatus())

	// Show open positions
	openTrades := engine.GetOpenTrades()
	if len(openTrades) > 0 {
		fmt.Println()
		fmt.Println("Open Positions:")
		for _, t := range openTrades {
			fmt.Printf("  - %s %s @ %.4f (SL: %.4f, TP: %.4f)\n",
				t.Side, t.Symbol, t.EntryPrice, t.StopLoss, t.TakeProfit)
		}
		fmt.Println()
		fmt.Println("⚠️  Positions remain open with TP/SL orders on exchange")
	}

	// Save state
	if err := engine.SaveState(); err != nil {
		fmt.Printf("Error saving state: %v\n", err)
	} else {
		fmt.Println()
		fmt.Println("State saved to:", config.StateFile)
	}
}

func modeString(dryRun bool) string {
	if dryRun {
		return "🔵 DRY RUN (no real orders)"
	}
	return "🔴 LIVE (real orders)"
}

func printStatus(status map[string]interface{}) {
	fmt.Println()
	fmt.Println(strings.Repeat("─", 60))
	fmt.Printf("  Status: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Println(strings.Repeat("─", 60))

	fmt.Printf("  Mode:              %s\n", modeString(status["dry_run"].(bool)))
	fmt.Printf("  Open Positions:    %d\n", status["open_positions"])
	fmt.Printf("  Closed Trades:     %d\n", status["closed_trades"])
	fmt.Printf("  Today's Trades:    %d\n", status["daily_trades"])

	dailyPnL := status["daily_pnl"].(float64)
	color := "\033[32m"
	if dailyPnL < 0 {
		color = "\033[31m"
	}
	fmt.Printf("  Today's PnL:       %s%+.2f USDT\033[0m\n", color, dailyPnL)

	totalPnL := status["total_pnl"].(float64)
	color = "\033[32m"
	if totalPnL < 0 {
		color = "\033[31m"
	}
	fmt.Printf("  Total PnL:         %s%+.2f USDT\033[0m\n", color, totalPnL)
	fmt.Printf("  Win Rate:          %.1f%%\n", status["win_rate"])
	fmt.Println(strings.Repeat("─", 60))
}
