package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"final-bot-trader-api/internal/exchange"
	"final-bot-trader-api/internal/exchange/model"
	"final-bot-trader-api/internal/strategy"
)

// BotConfig holds the bot configuration
type BotConfig struct {
	Leverage       int
	MarginPerTrade float64 // USDT per trade
	MaxPositions   int
	TPPercent      float64 // Take profit percentage
	SLPercent      float64 // Stop loss percentage
	Interval       string  // Candle interval (1h, 4h, 1d)
	CheckInterval  time.Duration
	Symbols        []string
	DryRun         bool // If true, don't execute real trades
}

// Position tracking
type TrackedPosition struct {
	Symbol     string
	Side       string
	Quantity   float64
	EntryPrice float64
	TP         float64
	SL         float64
	OpenTime   time.Time
}

// Bot is the main trading bot
type Bot struct {
	config    BotConfig
	client    *exchange.BitunixClient
	strategy  strategy.Strategy
	positions map[string]*TrackedPosition
}

func main() {
	// Flags
	dryRun := flag.Bool("dry-run", false, "Run without executing real trades")
	interval := flag.String("interval", "4h", "Candle interval (1h, 4h, 1d)")
	checkMins := flag.Int("check", 15, "Check interval in minutes")
	leverage := flag.Int("leverage", 5, "Leverage to use")
	marginPerTrade := flag.Float64("margin", 10.0, "USDT margin per trade")
	maxPositions := flag.Int("max-positions", 5, "Maximum simultaneous positions")
	tpPercent := flag.Float64("tp", 2.0, "Take profit percentage")
	slPercent := flag.Float64("sl", 2.0, "Stop loss percentage")

	flag.Parse()

	// Get credentials
	apiKey := os.Getenv("BITUNIX_API_KEY")
	secretKey := os.Getenv("BITUNIX_SECRET_KEY")
	baseURL := os.Getenv("BITUNIX_BASE_URL")

	if apiKey == "" || secretKey == "" {
		log.Fatal("BITUNIX_API_KEY and BITUNIX_SECRET_KEY are required")
	}

	// Configuration
	config := BotConfig{
		Leverage:       *leverage,
		MarginPerTrade: *marginPerTrade,
		MaxPositions:   *maxPositions,
		TPPercent:      *tpPercent,
		SLPercent:      *slPercent,
		Interval:       *interval,
		CheckInterval:  time.Duration(*checkMins) * time.Minute,
		DryRun:         *dryRun,
		Symbols: []string{
			"ETHUSDT",
			"SOLUSDT",
			"DOGEUSDT",
			"XRPUSDT",
			"DOTUSDT",
			"1000SHIBUSDT",
			"UNIUSDT",
		},
	}

	// Create client and strategy
	client := exchange.NewBitunixClient(apiKey, secretKey, baseURL)

	// Use SMA Crossover strategy (best performer in backtests)
	// Symbol is set per-analysis, so we use a placeholder here
	strat, err := strategy.NewSMACrossover("", 5, 20)
	if err != nil {
		log.Fatalf("Failed to create strategy: %v", err)
	}

	bot := &Bot{
		config:    config,
		client:    client,
		strategy:  strat,
		positions: make(map[string]*TrackedPosition),
	}

	// Print banner
	fmt.Println("==============================================")
	fmt.Println("       COPY TRADING BOT")
	fmt.Println("==============================================")
	fmt.Printf("Strategy:    %s\n", strat.Name())
	fmt.Printf("Leverage:    %dx\n", config.Leverage)
	fmt.Printf("Margin/Trade: %.2f USDT\n", config.MarginPerTrade)
	fmt.Printf("Max Positions: %d\n", config.MaxPositions)
	fmt.Printf("TP/SL:       +%.1f%% / -%.1f%%\n", config.TPPercent, config.SLPercent)
	fmt.Printf("Interval:    %s\n", config.Interval)
	fmt.Printf("Check Every: %v\n", config.CheckInterval)
	fmt.Printf("Symbols:     %v\n", config.Symbols)
	if config.DryRun {
		fmt.Println("\n⚠️  DRY RUN MODE - No real trades will be executed")
	}
	fmt.Println("==============================================")
	fmt.Println("")

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		fmt.Println("\n\nShutting down gracefully...")
		cancel()
	}()

	// Initial setup
	bot.setupLeverage(ctx)

	// Run the bot
	bot.run(ctx)
}

func (b *Bot) setupLeverage(ctx context.Context) {
	fmt.Println("Setting up leverage for all symbols...")
	for _, symbol := range b.config.Symbols {
		err := b.client.SetLeverage(ctx, symbol, b.config.Leverage)
		if err != nil {
			log.Printf("Warning: Could not set leverage for %s: %v", symbol, err)
		} else {
			fmt.Printf("  %s: %dx leverage set\n", symbol, b.config.Leverage)
		}
		time.Sleep(200 * time.Millisecond) // Rate limiting
	}
	fmt.Println("")
}

func (b *Bot) run(ctx context.Context) {
	// Initial check
	b.checkMarket(ctx)

	ticker := time.NewTicker(b.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			fmt.Println("Bot stopped.")
			return
		case <-ticker.C:
			b.checkMarket(ctx)
		}
	}
}

func (b *Bot) checkMarket(ctx context.Context) {
	timestamp := time.Now().Format("2006-01-02 15:04:05")
	fmt.Printf("\n[%s] Checking market...\n", timestamp)

	// Update tracked positions from exchange
	b.syncPositions(ctx)

	// Count current positions
	openCount := len(b.positions)
	fmt.Printf("  Open positions: %d/%d\n", openCount, b.config.MaxPositions)

	// Check each symbol
	for _, symbol := range b.config.Symbols {
		// Skip if we already have a position in this symbol
		if _, exists := b.positions[symbol]; exists {
			continue
		}

		// Skip if we've reached max positions
		if openCount >= b.config.MaxPositions {
			break
		}

		// Analyze symbol
		signal := b.analyzeSymbol(ctx, symbol)
		if signal != nil && signal.Type != strategy.SignalNone {
			if b.config.DryRun {
				fmt.Printf("  [DRY RUN] Would open %s position on %s\n", signal.Type, symbol)
			} else {
				b.openPosition(ctx, symbol, signal)
				openCount++
			}
		}

		time.Sleep(500 * time.Millisecond) // Rate limiting
	}
}

func (b *Bot) syncPositions(ctx context.Context) {
	positions, err := b.client.GetPositions(ctx)
	if err != nil {
		log.Printf("Error getting positions: %v", err)
		return
	}

	// Clear and rebuild from exchange data
	b.positions = make(map[string]*TrackedPosition)
	for _, p := range positions {
		if p.PositionAmt != 0 {
			b.positions[p.Symbol] = &TrackedPosition{
				Symbol:     p.Symbol,
				Side:       p.Side,
				Quantity:   p.PositionAmt,
				EntryPrice: p.EntryPrice,
			}
			fmt.Printf("  Position: %s %s %.4f @ %.6f (PnL: %.4f)\n",
				p.Symbol, p.Side, p.PositionAmt, p.EntryPrice, p.UnrealizedPnl)
		}
	}
}

func (b *Bot) analyzeSymbol(ctx context.Context, symbol string) *strategy.Signal {
	// Get candles (0, 0 means use default time range)
	candles, err := b.client.GetKlines(ctx, symbol, b.config.Interval, 100, 0, 0)
	if err != nil {
		log.Printf("Error getting candles for %s: %v", symbol, err)
		return nil
	}

	if len(candles) < b.strategy.MinimumCandles() {
		return nil
	}

	// Analyze
	signal, err := b.strategy.Analyze(candles)
	if err != nil {
		if err != strategy.ErrNoSignal {
			log.Printf("Strategy error for %s: %v", symbol, err)
		}
		return nil
	}

	return signal
}

func (b *Bot) openPosition(ctx context.Context, symbol string, signal *strategy.Signal) {
	// Get current price
	price, err := b.client.GetPrice(ctx, symbol)
	if err != nil {
		log.Printf("Error getting price for %s: %v", symbol, err)
		return
	}

	// Calculate quantity based on margin and leverage
	notional := b.config.MarginPerTrade * float64(b.config.Leverage)
	quantity := notional / price

	// Apply minimum quantities
	quantity = b.applyMinQuantity(symbol, quantity)

	// Calculate TP and SL
	var tpPrice, slPrice float64
	var side string

	if signal.Type == strategy.SignalBuy {
		side = "BUY"
		tpPrice = price * (1 + b.config.TPPercent/100)
		slPrice = price * (1 - b.config.SLPercent/100)
	} else {
		side = "SELL"
		tpPrice = price * (1 - b.config.TPPercent/100)
		slPrice = price * (1 + b.config.SLPercent/100)
	}

	fmt.Printf("\n  Opening %s position on %s:\n", side, symbol)
	fmt.Printf("    Price: %.6f\n", price)
	fmt.Printf("    Quantity: %.4f\n", quantity)
	fmt.Printf("    TP: %.6f (+%.1f%%)\n", tpPrice, b.config.TPPercent)
	fmt.Printf("    SL: %.6f (-%.1f%%)\n", slPrice, b.config.SLPercent)

	order := &model.OrderRequest{
		Symbol:            symbol,
		Side:              side,
		TradeSide:         "OPEN",
		Type:              "MARKET",
		Quantity:          quantity,
		TP:                tpPrice,
		SL:                slPrice,
		QuantityPrecision: b.getQuantityPrecision(symbol),
		PricePrecision:    b.getPricePrecision(symbol),
	}

	resp, err := b.client.PlaceOrder(ctx, order)
	if err != nil {
		log.Printf("    ERROR: %v", err)
		return
	}

	fmt.Printf("    SUCCESS: Order ID = %d\n", resp.OrderID)

	// Track locally
	b.positions[symbol] = &TrackedPosition{
		Symbol:     symbol,
		Side:       side,
		Quantity:   quantity,
		EntryPrice: price,
		TP:         tpPrice,
		SL:         slPrice,
		OpenTime:   time.Now(),
	}
}

func (b *Bot) applyMinQuantity(symbol string, qty float64) float64 {
	minQty := map[string]float64{
		"BTCUSDT":      0.001,
		"ETHUSDT":      0.01,
		"SOLUSDT":      0.1,
		"DOGEUSDT":     53,
		"XRPUSDT":      10,
		"DOTUSDT":      1,
		"1000SHIBUSDT": 100,
		"UNIUSDT":      1,
	}

	if min, exists := minQty[symbol]; exists && qty < min {
		return min
	}
	return qty
}

func (b *Bot) getQuantityPrecision(symbol string) int {
	precisions := map[string]int{
		"BTCUSDT":      4,
		"ETHUSDT":      3,
		"SOLUSDT":      2,
		"DOGEUSDT":     0,
		"XRPUSDT":      1,
		"DOTUSDT":      1,
		"1000SHIBUSDT": 0,
		"UNIUSDT":      1,
	}
	if p, exists := precisions[symbol]; exists {
		return p
	}
	return 2
}

func (b *Bot) getPricePrecision(symbol string) int {
	precisions := map[string]int{
		"BTCUSDT":      1,
		"ETHUSDT":      2,
		"SOLUSDT":      2,
		"DOGEUSDT":     6,
		"XRPUSDT":      4,
		"DOTUSDT":      4,
		"1000SHIBUSDT": 6,
		"UNIUSDT":      4,
	}
	if p, exists := precisions[symbol]; exists {
		return p
	}
	return 4
}
