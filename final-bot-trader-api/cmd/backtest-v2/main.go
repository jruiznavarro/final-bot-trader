// backtest-v2 replays the REAL MTFStrategy (same code path as live) over 4h
// historical candles, with exits that mirror the live engine: signal-defined
// ATR SL/TP plus the TrailingStopManager approximated bar-by-bar. Costs are
// Bitunix taker fees (0.06% per side) plus slippage (0.03% per side).
//
// It compares strategy variants to decide what to deploy:
//
//	A  current-live        TP 3.3 ATR + trailing (act 4.0%, trail 1.5%)
//	B  +btc-filter         A gated by BTC daily EMA50 (LONG only in BTC bull, SHORT only in bear)
//	C  btc+trail-only-2.5  B without fixed TP, trailing act 2.5% / trail 2.0%
//	D  btc+trail-only-4.0  B without fixed TP, trailing act 4.0% / trail 1.5%
//	E  daily-primary       D but primary trend from 1d candles (aggregated) instead of 4h
//
// Usage: backtest-v2 [-data data/historical]
package main

import (
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"final-bot-trader-api/internal/exchange/model"
	"final-bot-trader-api/internal/strategy"
	"final-bot-trader-api/internal/strategy/multifactor"
)

var symbols = []string{
	"BTCUSDT", "ETHUSDT", "SOLUSDT", "XRPUSDT", "DOGEUSDT", "LINKUSDT",
	"SUIUSDT", "ADAUSDT", "AVAXUSDT", "LTCUSDT", "AAVEUSDT", "NEARUSDT",
	"ICPUSDT", "HBARUSDT", "FILUSDT", "BCHUSDT", "DOTUSDT", "UNIUSDT",
	"1000PEPEUSDT", "1000SHIBUSDT", "WIFUSDT", "1000BONKUSDT", "ARBUSDT",
	"ONDOUSDT", "WLDUSDT", "ENAUSDT", "TAOUSDT", "APTUSDT", "HYPEUSDT", "TRUMPUSDT",
}

const (
	takerFee     = 0.0006 // Bitunix futures taker, per side
	slippage     = 0.0003 // per side
	posPct       = 0.05   // 5% of balance per trade
	leverage     = 3.0
	initialBal   = 1000.0
	lookback     = 80 // candles of history fed to the strategy
	cooldownBars = 1  // 1 bar = 4h, matches live CooldownPeriod
)

type Variant struct {
	Name         string
	BTCFilter    bool    // gate direction by BTC daily EMA50
	UseFixedTP   bool    // honor signal.TP; if false, trailing/SL only
	TrailActPct  float64 // trailing activation profit %
	TrailPct     float64 // trail distance %
	DailyPrimary bool    // primary TF = aggregated 1d instead of 4h
}

var variants = []Variant{
	{Name: "A current-live", UseFixedTP: true, TrailActPct: 4.0, TrailPct: 1.5},
	{Name: "B +btc-filter", BTCFilter: true, UseFixedTP: true, TrailActPct: 4.0, TrailPct: 1.5},
	{Name: "C btc+trail-2.5/2.0", BTCFilter: true, TrailActPct: 2.5, TrailPct: 2.0},
	{Name: "D btc+trail-4.0/1.5", BTCFilter: true, TrailActPct: 4.0, TrailPct: 1.5},
	{Name: "E daily-primary", BTCFilter: true, TrailActPct: 4.0, TrailPct: 1.5, DailyPrimary: true},
	{Name: "F trail-only-no-btc", TrailActPct: 4.0, TrailPct: 1.5},
}

type Trade struct {
	Symbol    string
	Side      string
	Entry     float64
	Exit      float64
	EntryTime time.Time
	ExitTime  time.Time
	PnL       float64 // USDT, net of fees+slippage
	Reason    string
}

type SymbolResult struct {
	Symbol  string
	Trades  int
	Wins    int
	PnL     float64
	PnL1    float64 // first half of period
	PnL2    float64 // second half
	GrossW  float64
	GrossL  float64
	MaxDD   float64
	balance float64
}

func main() {
	dataDir := flag.String("data", "data/historical", "historical data directory")
	flag.Parse()

	btcCandles, err := loadCSV(filepath.Join(*dataDir, "BTCUSDT_4h.csv"))
	if err != nil {
		fmt.Println("cannot load BTCUSDT data:", err)
		os.Exit(1)
	}
	btcBull := precomputeBTCBull(btcCandles)

	all := map[string][]model.Candle{}
	for _, s := range symbols {
		c, err := loadCSV(filepath.Join(*dataDir, s+"_4h.csv"))
		if err != nil || len(c) < 400 {
			fmt.Printf("skip %s: insufficient data\n", s)
			continue
		}
		all[s] = c
	}

	type variantSummary struct {
		Name    string         `json:"name"`
		PnL     float64        `json:"pnl"`
		Trades  int            `json:"trades"`
		WinRate float64        `json:"win_rate"`
		PF      float64        `json:"profit_factor"`
		PerSym  []SymbolResult `json:"per_symbol"`
	}
	var summaries []variantSummary

	for _, v := range variants {
		var results []SymbolResult
		for _, s := range symbols {
			candles, ok := all[s]
			if !ok {
				continue
			}
			r := runSymbol(s, candles, btcCandles, btcBull, v)
			results = append(results, r)
		}

		var pnl, gw, gl float64
		var trades, wins int
		for _, r := range results {
			pnl += r.PnL
			trades += r.Trades
			wins += r.Wins
			gw += r.GrossW
			gl += r.GrossL
		}
		wr, pf := 0.0, 0.0
		if trades > 0 {
			wr = float64(wins) / float64(trades) * 100
		}
		if gl > 0 {
			pf = gw / gl
		}

		fmt.Printf("\n══════ %s ══════\n", v.Name)
		fmt.Printf("TOTAL: %d trades | WR %.1f%% | PF %.2f | PnL %+.2f USDT (sobre %d cuentas de %.0f)\n",
			trades, wr, pf, pnl, len(results), initialBal)

		sort.Slice(results, func(i, j int) bool { return results[i].PnL > results[j].PnL })
		fmt.Printf("%-14s %6s %5s %9s %9s %9s %7s\n", "symbol", "trades", "WR%", "pnl", "1ªmitad", "2ªmitad", "maxDD%")
		for _, r := range results {
			if r.Trades == 0 {
				continue
			}
			fmt.Printf("%-14s %6d %5.0f %+9.2f %+9.2f %+9.2f %7.1f\n",
				r.Symbol, r.Trades, float64(r.Wins)/float64(r.Trades)*100, r.PnL, r.PnL1, r.PnL2, r.MaxDD)
		}
		summaries = append(summaries, variantSummary{
			Name: v.Name, PnL: pnl, Trades: trades, WinRate: wr, PF: pf, PerSym: results,
		})
	}

	out, _ := json.MarshalIndent(summaries, "", "  ")
	os.WriteFile("backtest_v2_results.json", out, 0644)
	fmt.Println("\nresultados guardados en backtest_v2_results.json")
}

// precomputeBTCBull returns, for each BTC 4h bar index, whether BTC is above
// its daily EMA50 (bull) using only data up to and including that bar.
func precomputeBTCBull(btc []model.Candle) []bool {
	bull := make([]bool, len(btc))
	daily := aggregateDaily(btc)
	// map each 4h bar to the index of its (possibly partial) daily candle
	dayIdx := 0
	for i, c := range btc {
		day := c.OpenTime.UTC().Truncate(24 * time.Hour)
		for dayIdx < len(daily)-1 && daily[dayIdx].OpenTime.Before(day) {
			dayIdx++
		}
		// EMA50 over daily closes up to dayIdx, with the current day's close
		// replaced by this bar's close (what live would see mid-day)
		end := dayIdx + 1
		if end < 55 {
			bull[i] = true // not enough history: no filter
			continue
		}
		closes := make([]float64, end)
		for j := 0; j < end; j++ {
			closes[j] = daily[j].Close
		}
		closes[end-1] = c.Close
		e := emaLast(closes, 50)
		bull[i] = c.Close > e
	}
	return bull
}

func runSymbol(symbol string, candles, btc []model.Candle, btcBull []bool, v Variant) SymbolResult {
	cfg := multifactor.MTFConfig{
		PrimaryInterval:       "4h",
		EntryInterval:         "4h",
		StrategyConfig:        multifactor.DefaultConfig(),
		RequireTrendAlignment: true,
		MinPrimaryADX:         25,
	}
	if v.DailyPrimary {
		cfg.PrimaryInterval = "1d"
	}
	strat := multifactor.NewMTFStrategy(symbol, cfg)

	res := SymbolResult{Symbol: symbol, balance: initialBal}
	maxBal := initialBal
	half := candles[len(candles)/2].OpenTime

	// align BTC bars by timestamp
	btcIdxByTime := map[int64]int{}
	for i, c := range btc {
		btcIdxByTime[c.OpenTime.Unix()] = i
	}

	var pos *Trade
	var sl, tp float64
	var trailBest, trailLevel float64
	trailActive := false
	cooldownUntil := -1

	closeTrade := func(t *Trade, exit float64, when time.Time, reason string, i int) {
		// slippage on exit
		if t.Side == "LONG" {
			exit *= 1 - slippage
		} else {
			exit *= 1 + slippage
		}
		notional := res.balance * posPct * leverage
		qty := notional / t.Entry
		var raw float64
		if t.Side == "LONG" {
			raw = (exit - t.Entry) * qty
		} else {
			raw = (t.Entry - exit) * qty
		}
		fees := (t.Entry + exit) * qty * takerFee
		t.PnL = raw - fees
		t.Exit = exit
		t.ExitTime = when
		t.Reason = reason

		res.balance += t.PnL
		res.PnL += t.PnL
		if when.Before(half) {
			res.PnL1 += t.PnL
		} else {
			res.PnL2 += t.PnL
		}
		res.Trades++
		if t.PnL >= 0 {
			res.Wins++
			res.GrossW += t.PnL
		} else {
			res.GrossL += -t.PnL
		}
		if res.balance > maxBal {
			maxBal = res.balance
		}
		dd := (maxBal - res.balance) / maxBal * 100
		if dd > res.MaxDD {
			res.MaxDD = dd
		}
		cooldownUntil = i + cooldownBars
	}

	for i := lookback; i < len(candles)-1; i++ {
		bar := candles[i]

		if pos != nil {
			isLong := pos.Side == "LONG"
			exited := false

			// 1) gap through SL at open
			if !exited && ((isLong && bar.Open <= sl) || (!isLong && bar.Open >= sl)) {
				closeTrade(pos, bar.Open, bar.OpenTime, "Stop Loss (gap)", i)
				exited = true
			}
			// 2) trailing level from previous bars (live polls every 15s; a level
			//    raised this bar is only actionable next bar — conservative)
			if !exited && trailActive && ((isLong && bar.Low <= trailLevel) || (!isLong && bar.High >= trailLevel)) {
				closeTrade(pos, trailLevel, bar.CloseTime, "Trailing stop", i)
				exited = true
			}
			// 3) hard SL (conservative: checked before TP when both touch)
			if !exited && ((isLong && bar.Low <= sl) || (!isLong && bar.High >= sl)) {
				closeTrade(pos, sl, bar.CloseTime, "Stop Loss", i)
				exited = true
			}
			// 4) fixed TP
			if !exited && v.UseFixedTP && ((isLong && bar.High >= tp) || (!isLong && bar.Low <= tp)) {
				closeTrade(pos, tp, bar.CloseTime, "Take Profit", i)
				exited = true
			}

			if exited {
				pos = nil
				trailActive = false
				continue
			}

			// update trailing state with this bar's extreme
			ext := bar.High
			profitPct := (ext - pos.Entry) / pos.Entry * 100
			if !isLong {
				ext = bar.Low
				profitPct = (pos.Entry - ext) / pos.Entry * 100
			}
			if !trailActive && profitPct >= v.TrailActPct {
				trailActive = true
				trailBest = ext
				trailLevel = trail(ext, v.TrailPct, isLong)
			} else if trailActive {
				if (isLong && ext > trailBest) || (!isLong && ext < trailBest) {
					trailBest = ext
					nl := trail(ext, v.TrailPct, isLong)
					if (isLong && nl > trailLevel) || (!isLong && nl < trailLevel) {
						trailLevel = nl
					}
				}
			}
			continue
		}

		if i <= cooldownUntil {
			continue
		}

		// build inputs exactly like the live engine
		hist := candles[i-lookback+1 : i+1]
		daily := aggregateDaily(hist)
		primary := hist
		if v.DailyPrimary {
			// daily primary needs more history than the entry lookback window
			start := i - 6*60 // ~60 days of 4h bars
			if start < 0 {
				start = 0
			}
			primary = aggregateDaily(candles[start : i+1])
			if len(primary) < 50 {
				continue
			}
		}

		sig, err := strat.AnalyzeMTF(primary, hist, daily)
		if err != nil || sig == nil {
			continue
		}

		side := "LONG"
		if sig.Type == strategy.SignalSell {
			side = "SHORT"
		}

		// BTC master filter: direction must match the BTC daily regime
		if v.BTCFilter {
			bi, ok := btcIdxByTime[bar.OpenTime.Unix()]
			if !ok {
				continue
			}
			if (side == "LONG" && !btcBull[bi]) || (side == "SHORT" && btcBull[bi]) {
				continue
			}
		}

		next := candles[i+1]
		entry := next.Open
		if side == "LONG" {
			entry *= 1 + slippage
		} else {
			entry *= 1 - slippage
		}
		pos = &Trade{Symbol: symbol, Side: side, Entry: entry, EntryTime: next.OpenTime}
		sl, tp = sig.SL, sig.TP
		trailActive = false
	}

	return res
}

func trail(best, pct float64, isLong bool) float64 {
	if isLong {
		return best * (1 - pct/100)
	}
	return best * (1 + pct/100)
}

// aggregateDaily groups 4h candles into UTC daily candles (last day may be partial,
// matching what the live engine sees when it fetches 1d klines mid-day).
func aggregateDaily(candles []model.Candle) []model.Candle {
	var out []model.Candle
	var cur *model.Candle
	for _, c := range candles {
		day := c.OpenTime.UTC().Truncate(24 * time.Hour)
		if cur == nil || !cur.OpenTime.Equal(day) {
			if cur != nil {
				out = append(out, *cur)
			}
			cc := c
			cc.OpenTime = day
			cc.CloseTime = day.Add(24 * time.Hour)
			cur = &cc
			continue
		}
		if c.High > cur.High {
			cur.High = c.High
		}
		if c.Low < cur.Low {
			cur.Low = c.Low
		}
		cur.Close = c.Close
		cur.Volume += c.Volume
	}
	if cur != nil {
		out = append(out, *cur)
	}
	return out
}

func emaLast(values []float64, period int) float64 {
	if len(values) == 0 {
		return 0
	}
	k := 2.0 / float64(period+1)
	n := period
	if n > len(values) {
		n = len(values)
	}
	sum := 0.0
	for i := 0; i < n; i++ {
		sum += values[i]
	}
	e := sum / float64(n)
	for i := 1; i < len(values); i++ {
		e = (values[i]-e)*k + e
	}
	return e
}

func loadCSV(filename string) ([]model.Candle, error) {
	f, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	records, err := csv.NewReader(f).ReadAll()
	if err != nil {
		return nil, err
	}

	var candles []model.Candle
	for i, rec := range records {
		if i == 0 {
			continue
		}
		t, err := time.Parse("2006-01-02 15:04:05", rec[0])
		if err != nil {
			return nil, fmt.Errorf("bad timestamp %q: %w", rec[0], err)
		}
		open, _ := strconv.ParseFloat(rec[1], 64)
		high, _ := strconv.ParseFloat(rec[2], 64)
		low, _ := strconv.ParseFloat(rec[3], 64)
		cl, _ := strconv.ParseFloat(rec[4], 64)
		vol, _ := strconv.ParseFloat(rec[5], 64)
		if open <= 0 || high <= 0 || low <= 0 || cl <= 0 || math.IsNaN(vol) {
			continue
		}
		candles = append(candles, model.Candle{
			OpenTime: t, CloseTime: t.Add(4 * time.Hour),
			Open: open, High: high, Low: low, Close: cl, Volume: vol,
		})
	}
	return candles, nil
}
