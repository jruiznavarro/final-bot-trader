package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
)

type TickerData struct {
	Symbol    string `json:"symbol"`
	LastPrice string `json:"lastPrice"`
	MarkPrice string `json:"markPrice"`
	Open      string `json:"open"`
	High      string `json:"high"`
	Low       string `json:"low"`
	QuoteVol  string `json:"quoteVol"`  // USDT volume
	BaseVol   string `json:"baseVol"`   // Coin volume
}

type BitunixResponse struct {
	Code int          `json:"code"`
	Msg  string       `json:"msg"`
	Data []TickerData `json:"data"`
}

func main() {
	baseURL := os.Getenv("BITUNIX_BASE_URL")
	if baseURL == "" {
		baseURL = "https://fapi.bitunix.com"
	}

	// Get all tickers
	resp, err := http.Get(baseURL + "/api/v1/futures/market/tickers")
	if err != nil {
		log.Fatalf("Error fetching tickers: %v", err)
	}
	defer resp.Body.Close()

	var result BitunixResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		log.Fatalf("Error decoding response: %v", err)
	}

	if result.Code != 0 {
		log.Fatalf("API error: %s", result.Msg)
	}

	// Filter USDT pairs and sort by volume
	type SymbolInfo struct {
		Symbol    string
		QuoteVol  float64
		Price     float64
		Open      float64
		High      float64
		Low       float64
		Change24h float64
	}

	var symbols []SymbolInfo
	for _, t := range result.Data {
		if !strings.HasSuffix(t.Symbol, "USDT") {
			continue
		}

		quoteVol, _ := strconv.ParseFloat(t.QuoteVol, 64)
		price, _ := strconv.ParseFloat(t.LastPrice, 64)
		open, _ := strconv.ParseFloat(t.Open, 64)
		high, _ := strconv.ParseFloat(t.High, 64)
		low, _ := strconv.ParseFloat(t.Low, 64)

		change := 0.0
		if open > 0 {
			change = ((price - open) / open) * 100
		}

		symbols = append(symbols, SymbolInfo{
			Symbol:    t.Symbol,
			QuoteVol:  quoteVol,
			Price:     price,
			Open:      open,
			High:      high,
			Low:       low,
			Change24h: change,
		})
	}

	// Sort by quote volume (USDT volume)
	sort.Slice(symbols, func(i, j int) bool {
		return symbols[i].QuoteVol > symbols[j].QuoteVol
	})

	fmt.Println("==============================================")
	fmt.Println("  BITUNIX FUTURES - ALL USDT PAIRS")
	fmt.Println("  Sorted by 24h Volume (USDT)")
	fmt.Println("==============================================")
	fmt.Println("")
	fmt.Printf("%-20s %15s %15s %10s\n", "Symbol", "Volume (USDT)", "Price", "24h %")
	fmt.Println("----------------------------------------------------------------------")

	for i, s := range symbols {
		volStr := formatVolume(s.QuoteVol)
		fmt.Printf("%-20s %15s %15.6f %+10.2f%%\n",
			s.Symbol, volStr, s.Price, s.Change24h)

		if i == 59 { // Show top 60
			break
		}
	}

	fmt.Println("----------------------------------------------------------------------")
	fmt.Printf("\nTotal USDT pairs available: %d\n", len(symbols))

	fmt.Println("\n==============================================")
	fmt.Println("  TOP 40 CANDIDATES FOR ANALYSIS")
	fmt.Println("  (Volume > $1M, excluding stablecoins)")
	fmt.Println("==============================================")
	fmt.Println("")

	// Select top 40 by volume (excluding stablecoins and very low volume)
	var candidates []string
	for _, s := range symbols {
		// Skip stablecoins
		if strings.Contains(s.Symbol, "USDC") ||
			strings.Contains(s.Symbol, "TUSD") ||
			strings.Contains(s.Symbol, "BUSD") ||
			strings.Contains(s.Symbol, "DAI") ||
			strings.Contains(s.Symbol, "FDUSD") {
			continue
		}

		// Skip very low volume (< 500K USDT daily)
		if s.QuoteVol < 500000 {
			continue
		}

		candidates = append(candidates, s.Symbol)
		if len(candidates) >= 40 {
			break
		}
	}

	fmt.Printf("Selected %d symbols for analysis:\n\n", len(candidates))
	for i, c := range candidates {
		// Find the symbol info
		for _, s := range symbols {
			if s.Symbol == c {
				fmt.Printf("%2d. %-18s Vol: %12s  Price: %.6f\n",
					i+1, c, formatVolume(s.QuoteVol), s.Price)
				break
			}
		}
	}

	// Print as Go slice
	fmt.Println("\n// Go code for copying:")
	fmt.Println("symbols := []string{")
	for _, c := range candidates {
		fmt.Printf("\t\"%s\",\n", c)
	}
	fmt.Println("}")
}

func formatVolume(vol float64) string {
	if vol >= 1e9 {
		return fmt.Sprintf("%.2fB", vol/1e9)
	} else if vol >= 1e6 {
		return fmt.Sprintf("%.2fM", vol/1e6)
	} else if vol >= 1e3 {
		return fmt.Sprintf("%.2fK", vol/1e3)
	}
	return fmt.Sprintf("%.2f", vol)
}
