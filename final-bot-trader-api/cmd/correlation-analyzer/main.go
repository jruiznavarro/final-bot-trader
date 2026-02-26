package main

import (
	"encoding/csv"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Candle struct {
	Time  time.Time
	Close float64
}

type CorrelationPair struct {
	Symbol1     string
	Symbol2     string
	Correlation float64
}

func main() {
	dataDir := "data/historical"

	// Load all data
	symbolData := make(map[string][]Candle)

	files, _ := filepath.Glob(filepath.Join(dataDir, "*_4h.csv"))
	for _, file := range files {
		symbol := strings.TrimSuffix(filepath.Base(file), "_4h.csv")
		candles, err := loadCSV(file)
		if err != nil {
			continue
		}
		symbolData[symbol] = candles
	}

	fmt.Println("==============================================")
	fmt.Println("  CORRELATION ANALYSIS")
	fmt.Println("  Identifying correlated pairs")
	fmt.Println("==============================================\n")

	// Calculate returns for each symbol
	symbolReturns := make(map[string][]float64)
	for symbol, candles := range symbolData {
		var returns []float64
		for i := 1; i < len(candles); i++ {
			ret := (candles[i].Close - candles[i-1].Close) / candles[i-1].Close
			returns = append(returns, ret)
		}
		symbolReturns[symbol] = returns
	}

	// Calculate correlations
	var correlations []CorrelationPair
	symbols := make([]string, 0, len(symbolReturns))
	for s := range symbolReturns {
		symbols = append(symbols, s)
	}
	sort.Strings(symbols)

	for i := 0; i < len(symbols); i++ {
		for j := i + 1; j < len(symbols); j++ {
			corr := calculateCorrelation(symbolReturns[symbols[i]], symbolReturns[symbols[j]])
			correlations = append(correlations, CorrelationPair{
				Symbol1:     symbols[i],
				Symbol2:     symbols[j],
				Correlation: corr,
			})
		}
	}

	// Sort by correlation (highest first)
	sort.Slice(correlations, func(i, j int) bool {
		return correlations[i].Correlation > correlations[j].Correlation
	})

	// Highly correlated pairs (>0.8)
	fmt.Println("🔴 ALTA CORRELACIÓN (>0.80) - Evitar operar juntos:")
	fmt.Println(strings.Repeat("-", 55))
	count := 0
	for _, c := range correlations {
		if c.Correlation > 0.80 {
			fmt.Printf("   %.3f: %s <-> %s\n", c.Correlation, c.Symbol1, c.Symbol2)
			count++
		}
	}
	if count == 0 {
		fmt.Println("   Ninguno")
	}

	// Moderately correlated (0.6-0.8)
	fmt.Println("\n🟡 CORRELACIÓN MODERADA (0.60-0.80):")
	fmt.Println(strings.Repeat("-", 55))
	count = 0
	for _, c := range correlations {
		if c.Correlation >= 0.60 && c.Correlation <= 0.80 {
			fmt.Printf("   %.3f: %s <-> %s\n", c.Correlation, c.Symbol1, c.Symbol2)
			count++
			if count >= 20 {
				fmt.Println("   ... (más pares omitidos)")
				break
			}
		}
	}

	// Low/Negative correlation (good for diversification)
	fmt.Println("\n🟢 BAJA CORRELACIÓN (<0.40) - Buenos para diversificar:")
	fmt.Println(strings.Repeat("-", 55))
	sort.Slice(correlations, func(i, j int) bool {
		return correlations[i].Correlation < correlations[j].Correlation
	})
	count = 0
	for _, c := range correlations {
		if c.Correlation < 0.40 {
			fmt.Printf("   %.3f: %s <-> %s\n", c.Correlation, c.Symbol1, c.Symbol2)
			count++
			if count >= 15 {
				break
			}
		}
	}

	// Average correlation with BTC (market beta)
	fmt.Println("\n📊 CORRELACIÓN CON BTC (Beta de mercado):")
	fmt.Println(strings.Repeat("-", 55))

	type BetaPair struct {
		Symbol string
		Beta   float64
	}
	var betas []BetaPair

	btcReturns := symbolReturns["BTCUSDT"]
	for symbol, returns := range symbolReturns {
		if symbol == "BTCUSDT" {
			continue
		}
		corr := calculateCorrelation(btcReturns, returns)
		betas = append(betas, BetaPair{symbol, corr})
	}

	sort.Slice(betas, func(i, j int) bool {
		return betas[i].Beta > betas[j].Beta
	})

	fmt.Printf("%-18s %10s %s\n", "Symbol", "Corr BTC", "Interpretation")
	fmt.Println(strings.Repeat("-", 55))
	for _, b := range betas {
		interp := "Moderada"
		if b.Beta > 0.75 {
			interp = "Alta - Sigue a BTC"
		} else if b.Beta < 0.50 {
			interp = "Baja - Independiente"
		}
		fmt.Printf("%-18s %+10.3f %s\n", b.Symbol, b.Beta, interp)
	}

	// Recommendations for portfolio
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("  RECOMENDACIÓN: GRUPOS DE DIVERSIFICACIÓN")
	fmt.Println(strings.Repeat("=", 60))

	// Group by correlation clusters
	fmt.Println("\n📦 Para máxima diversificación, elegir 1 de cada grupo:\n")

	// Find clusters (simplified)
	fmt.Println("GRUPO 1 - L1 Major (muy correlacionados entre sí):")
	fmt.Println("   BTC, ETH, SOL - Elegir solo 1")

	fmt.Println("\nGRUPO 2 - Alt L1 (correlacionados):")
	fmt.Println("   ADA, AVAX, DOT, NEAR - Elegir 1-2")

	fmt.Println("\nGRUPO 3 - DeFi:")
	fmt.Println("   LINK, AAVE, UNI, ENA - Elegir 1-2")

	fmt.Println("\nGRUPO 4 - Meme (sorprendentemente diversos):")
	fmt.Println("   DOGE, SHIB, PEPE, BONK, WIF - Pueden coexistir")

	fmt.Println("\nGRUPO 5 - AI/Nuevos (baja correlación):")
	fmt.Println("   TAO, WLD, HYPE, TRUMP - Buenos para diversificar")

	fmt.Println("\nGRUPO 6 - Infra/L2:")
	fmt.Println("   FIL, ICP, ARB - Elegir 1-2")

	// Final selection recommendation
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("  SELECCIÓN FINAL RECOMENDADA (12 coins)")
	fmt.Println(strings.Repeat("=", 60))

	fmt.Println(`
Basado en:
- Trendeabilidad (análisis anterior)
- Diversificación (correlación)
- Volumen suficiente

SELECCIÓN ÓPTIMA:
┌─────────────────────────────────────────────────────────┐
│ 1. SUIUSDT    (L1)    - Trend Score: 98.16             │
│ 2. ENAUSDT    (DeFi)  - Trend Score: 101.56 ⭐ MEJOR    │
│ 3. TAOUSDT    (AI)    - Trend Score: 99.98             │
│ 4. ARBUSDT    (L2)    - Trend Score: 99.56             │
│ 5. WIFUSDT    (Meme)  - Trend Score: 99.56             │
│ 6. DOGEUSDT   (Meme)  - Trend Score: 97.38             │
│ 7. FILUSDT    (Infra) - Trend Score: 97.42             │
│ 8. LINKUSDT   (DeFi)  - Trend Score: 95.22             │
│ 9. 1000PEPEUSDT (Meme) - Trend Score: 96.50            │
│ 10. WLDUSDT   (AI)    - Trend Score: 99.09             │
│ 11. SOLUSDT   (L1)    - Trend Score: 95.31             │
│ 12. AAVEUSDT  (DeFi)  - Trend Score: 96.08             │
└─────────────────────────────────────────────────────────┘

EXCLUIDOS:
- BTCUSDT: Baja trendeabilidad, mejor para mean reversion
- ETHUSDT: Alta correlación con BTC/SOL
- XRPUSDT: Errático pese a buenos retornos
- LTCUSDT, BCHUSDT: Bajos scores y alta correlación con BTC
`)
}

func loadCSV(filename string) ([]Candle, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	reader := csv.NewReader(f)
	records, _ := reader.ReadAll()

	var candles []Candle
	for i, record := range records {
		if i == 0 {
			continue
		}
		t, _ := time.Parse("2006-01-02 15:04:05", record[0])
		close, _ := strconv.ParseFloat(record[4], 64)
		candles = append(candles, Candle{t, close})
	}
	return candles, nil
}

func calculateCorrelation(x, y []float64) float64 {
	n := min(len(x), len(y))
	if n < 10 {
		return 0
	}

	x = x[:n]
	y = y[:n]

	meanX := mean(x)
	meanY := mean(y)

	var sumXY, sumX2, sumY2 float64
	for i := 0; i < n; i++ {
		dx := x[i] - meanX
		dy := y[i] - meanY
		sumXY += dx * dy
		sumX2 += dx * dx
		sumY2 += dy * dy
	}

	if sumX2 == 0 || sumY2 == 0 {
		return 0
	}

	return sumXY / math.Sqrt(sumX2*sumY2)
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sum := 0.0
	for _, v := range values {
		sum += v
	}
	return sum / float64(len(values))
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
