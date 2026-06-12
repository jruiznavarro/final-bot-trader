package main

import (
	"fmt"
	"os"

	"final-bot-trader-api/internal/telegram"

	"github.com/joho/godotenv"
)

func main() {
	godotenv.Load()

	token := os.Getenv("TELEGRAM_BOT_TOKEN")
	chatID := os.Getenv("TELEGRAM_CHAT_ID")

	if token == "" || chatID == "" {
		fmt.Println("Error: TELEGRAM_BOT_TOKEN and TELEGRAM_CHAT_ID must be set in .env")
		fmt.Println()
		fmt.Println("How to configure:")
		fmt.Println("1. Create a bot with @BotFather on Telegram")
		fmt.Println("2. Get your chat ID with @userinfobot or @getmyid_bot")
		fmt.Println("3. Add to .env:")
		fmt.Println("   TELEGRAM_BOT_TOKEN=your_token")
		fmt.Println("   TELEGRAM_CHAT_ID=your_chat_id")
		os.Exit(1)
	}

	client := telegram.NewClient(token, chatID)

	fmt.Println("Testing Telegram notifications...")
	fmt.Println()

	// Test startup notification
	fmt.Print("Sending startup notification... ")
	if err := client.NotifyStartup("TEST MODE", 0.05, 3, []string{"BTCUSDT"}, "4h", "1h", 5, 3); err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		fmt.Println("OK")
	}

	// Test position opened
	fmt.Print("Sending position opened notification... ")
	if err := client.NotifyPositionOpened("BTCUSDT", "LONG", 50000.50, 0.002, 52500.00, 48000.00, "Test: RSI oversold, EMA crossover"); err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		fmt.Println("OK")
	}

	// Test position closed
	fmt.Print("Sending position closed notification... ")
	if err := client.NotifyPositionClosed("BTCUSDT", "LONG", "Take Profit hit", 50000.50, 52500.00, 5.00); err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		fmt.Println("OK")
	}

	// Test shutdown
	fmt.Print("Sending shutdown notification... ")
	if err := client.NotifyShutdown(2, 15.50); err != nil {
		fmt.Printf("ERROR: %v\n", err)
	} else {
		fmt.Println("OK")
	}

	fmt.Println()
	fmt.Println("Check your Telegram for messages from Copy Trading Bot!")
}
