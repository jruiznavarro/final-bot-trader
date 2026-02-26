package exchange

import (
	"context"
	"fmt"
	"time"

	"github.com/alpacahq/alpaca-trade-api-go/v3/alpaca"
	"github.com/alpacahq/alpaca-trade-api-go/v3/marketdata"
	"github.com/shopspring/decimal"
)

// AlpacaClient wraps the Alpaca API
type AlpacaClient struct {
	trading    *alpaca.Client
	marketData *marketdata.Client
	isPaper    bool
}

// NewAlpacaClient creates a new Alpaca client
func NewAlpacaClient(apiKey, apiSecret string, paper bool) *AlpacaClient {
	baseURL := "https://api.alpaca.markets"
	if paper {
		baseURL = "https://paper-api.alpaca.markets"
	}

	tradingClient := alpaca.NewClient(alpaca.ClientOpts{
		APIKey:    apiKey,
		APISecret: apiSecret,
		BaseURL:   baseURL,
	})

	marketDataClient := marketdata.NewClient(marketdata.ClientOpts{
		APIKey:    apiKey,
		APISecret: apiSecret,
	})

	return &AlpacaClient{
		trading:    tradingClient,
		marketData: marketDataClient,
		isPaper:    paper,
	}
}

// GetAccount returns account information
func (c *AlpacaClient) GetAccount(ctx context.Context) (*AccountInfo, error) {
	account, err := c.trading.GetAccount()
	if err != nil {
		return nil, fmt.Errorf("failed to get account: %w", err)
	}

	cash, _ := account.Cash.Float64()
	portfolioValue, _ := account.PortfolioValue.Float64()
	buyingPower, _ := account.BuyingPower.Float64()
	equity, _ := account.Equity.Float64()

	return &AccountInfo{
		Cash:             cash,
		PortfolioValue:   portfolioValue,
		BuyingPower:      buyingPower,
		Equity:           equity,
		DayTradeCount:    int(account.DaytradeCount),
		PatternDayTrader: account.PatternDayTrader,
	}, nil
}

// GetPositions returns all open positions
func (c *AlpacaClient) GetPositions(ctx context.Context) ([]Position, error) {
	positions, err := c.trading.GetPositions()
	if err != nil {
		return nil, fmt.Errorf("failed to get positions: %w", err)
	}

	result := make([]Position, len(positions))
	for i, pos := range positions {
		qty, _ := pos.Qty.Float64()
		entryPrice, _ := pos.AvgEntryPrice.Float64()
		currentPrice, _ := pos.CurrentPrice.Float64()
		marketValue, _ := pos.MarketValue.Float64()
		unrealizedPnL, _ := pos.UnrealizedPL.Float64()

		side := "long"
		if qty < 0 {
			side = "short"
			qty = -qty
		}

		result[i] = Position{
			Symbol:        pos.Symbol,
			Quantity:      qty,
			EntryPrice:    entryPrice,
			CurrentPrice:  currentPrice,
			MarketValue:   marketValue,
			UnrealizedPnL: unrealizedPnL,
			Side:          side,
		}
	}

	return result, nil
}

// GetPosition returns a specific position
func (c *AlpacaClient) GetPosition(ctx context.Context, symbol string) (*Position, error) {
	pos, err := c.trading.GetPosition(symbol)
	if err != nil {
		return nil, err
	}

	qty, _ := pos.Qty.Float64()
	entryPrice, _ := pos.AvgEntryPrice.Float64()
	currentPrice, _ := pos.CurrentPrice.Float64()
	marketValue, _ := pos.MarketValue.Float64()
	unrealizedPnL, _ := pos.UnrealizedPL.Float64()

	side := "long"
	if qty < 0 {
		side = "short"
		qty = -qty
	}

	return &Position{
		Symbol:        pos.Symbol,
		Quantity:      qty,
		EntryPrice:    entryPrice,
		CurrentPrice:  currentPrice,
		MarketValue:   marketValue,
		UnrealizedPnL: unrealizedPnL,
		Side:          side,
	}, nil
}

// PlaceMarketOrder places a market order
func (c *AlpacaClient) PlaceMarketOrder(ctx context.Context, symbol string, qty float64, side string) (*Order, error) {
	orderSide := alpaca.Buy
	positionIntent := alpaca.BuyToOpen
	if side == "sell" {
		orderSide = alpaca.Sell
		positionIntent = alpaca.SellToClose
	}

	qtyDecimal := decimal.NewFromFloat(qty)

	req := alpaca.PlaceOrderRequest{
		Symbol:         symbol,
		Qty:            &qtyDecimal,
		Side:           orderSide,
		Type:           alpaca.Market,
		TimeInForce:    alpaca.Day,
		PositionIntent: positionIntent,
	}

	order, err := c.trading.PlaceOrder(req)
	if err != nil {
		return nil, fmt.Errorf("failed to place order: %w", err)
	}

	filledQty, _ := order.FilledQty.Float64()
	filledAvgPrice, _ := order.FilledAvgPrice.Float64()

	return &Order{
		ID:             order.ID,
		Symbol:         order.Symbol,
		Side:           side,
		Type:           "market",
		Quantity:       qty,
		Status:         string(order.Status),
		FilledQty:      filledQty,
		FilledAvgPrice: filledAvgPrice,
		CreatedAt:      order.CreatedAt,
	}, nil
}

// PlaceBracketOrder places an order with take profit and stop loss
func (c *AlpacaClient) PlaceBracketOrder(ctx context.Context, symbol string, qty float64, side string, takeProfit, stopLoss float64) (*Order, error) {
	orderSide := alpaca.Buy
	if side == "sell" {
		orderSide = alpaca.Sell
	}

	qtyDecimal := decimal.NewFromFloat(qty)
	tpDecimal := decimal.NewFromFloat(takeProfit)
	slDecimal := decimal.NewFromFloat(stopLoss)

	tp := alpaca.TakeProfit{LimitPrice: &tpDecimal}
	sl := alpaca.StopLoss{StopPrice: &slDecimal}

	order, err := c.trading.PlaceOrder(alpaca.PlaceOrderRequest{
		Symbol:        symbol,
		Qty:           &qtyDecimal,
		Side:          orderSide,
		Type:          alpaca.Market,
		TimeInForce:   alpaca.GTC, // Good til cancelled for bracket
		OrderClass:    alpaca.Bracket,
		TakeProfit:    &tp,
		StopLoss:      &sl,
	})
	if err != nil {
		// If bracket fails, try simple market order
		order, err = c.trading.PlaceOrder(alpaca.PlaceOrderRequest{
			Symbol:      symbol,
			Qty:         &qtyDecimal,
			Side:        orderSide,
			Type:        alpaca.Market,
			TimeInForce: alpaca.Day,
		})
		if err != nil {
			return nil, fmt.Errorf("failed to place order: %w", err)
		}
	}

	filledQty, _ := order.FilledQty.Float64()
	filledAvgPrice, _ := order.FilledAvgPrice.Float64()

	return &Order{
		ID:             order.ID,
		Symbol:         order.Symbol,
		Side:           side,
		Type:           "market",
		Quantity:       qty,
		Status:         string(order.Status),
		FilledQty:      filledQty,
		FilledAvgPrice: filledAvgPrice,
		CreatedAt:      order.CreatedAt,
	}, nil
}

// ClosePosition closes a position
func (c *AlpacaClient) ClosePosition(ctx context.Context, symbol string) error {
	_, err := c.trading.ClosePosition(symbol, alpaca.ClosePositionRequest{})
	if err != nil {
		return fmt.Errorf("failed to close position: %w", err)
	}
	return nil
}

// CloseAllPositions closes all positions
func (c *AlpacaClient) CloseAllPositions(ctx context.Context) error {
	_, err := c.trading.CloseAllPositions(alpaca.CloseAllPositionsRequest{})
	if err != nil {
		return fmt.Errorf("failed to close all positions: %w", err)
	}
	return nil
}

// GetBars returns historical OHLCV data
func (c *AlpacaClient) GetBars(ctx context.Context, symbol string, timeframe string, start, end time.Time) ([]Candle, error) {
	tf := marketdata.OneHour
	switch timeframe {
	case "1m":
		tf = marketdata.OneMin
	case "5m":
		tf = marketdata.NewTimeFrame(5, marketdata.Min)
	case "15m":
		tf = marketdata.NewTimeFrame(15, marketdata.Min)
	case "30m":
		tf = marketdata.NewTimeFrame(30, marketdata.Min)
	case "1h":
		tf = marketdata.OneHour
	case "4h":
		tf = marketdata.NewTimeFrame(4, marketdata.Hour)
	case "1d":
		tf = marketdata.OneDay
	}

	bars, err := c.marketData.GetBars(symbol, marketdata.GetBarsRequest{
		TimeFrame: tf,
		Start:     start,
		End:       end,
		Feed:      "iex", // Use IEX feed (free) instead of SIP
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get bars: %w", err)
	}

	candles := make([]Candle, len(bars))
	for i, bar := range bars {
		candles[i] = Candle{
			Time:   bar.Timestamp,
			Open:   bar.Open,
			High:   bar.High,
			Low:    bar.Low,
			Close:  bar.Close,
			Volume: float64(bar.Volume),
		}
	}

	return candles, nil
}

// GetLatestBar returns the most recent bar
func (c *AlpacaClient) GetLatestBar(ctx context.Context, symbol string) (*Candle, error) {
	bar, err := c.marketData.GetLatestBar(symbol, marketdata.GetLatestBarRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get latest bar: %w", err)
	}

	return &Candle{
		Time:   bar.Timestamp,
		Open:   bar.Open,
		High:   bar.High,
		Low:    bar.Low,
		Close:  bar.Close,
		Volume: float64(bar.Volume),
	}, nil
}

// GetLatestQuote returns the latest quote
func (c *AlpacaClient) GetLatestQuote(ctx context.Context, symbol string) (*Quote, error) {
	quote, err := c.marketData.GetLatestQuote(symbol, marketdata.GetLatestQuoteRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get latest quote: %w", err)
	}

	return &Quote{
		Symbol:    symbol,
		BidPrice:  quote.BidPrice,
		AskPrice:  quote.AskPrice,
		Timestamp: quote.Timestamp,
	}, nil
}

// GetLatestTrade returns the latest trade
func (c *AlpacaClient) GetLatestTrade(ctx context.Context, symbol string) (*Quote, error) {
	trade, err := c.marketData.GetLatestTrade(symbol, marketdata.GetLatestTradeRequest{})
	if err != nil {
		return nil, fmt.Errorf("failed to get latest trade: %w", err)
	}

	return &Quote{
		Symbol:    symbol,
		LastPrice: trade.Price,
		Volume:    float64(trade.Size),
		Timestamp: trade.Timestamp,
	}, nil
}

// IsMarketOpen checks if the market is open
func (c *AlpacaClient) IsMarketOpen(ctx context.Context) (bool, error) {
	clock, err := c.trading.GetClock()
	if err != nil {
		return false, fmt.Errorf("failed to get clock: %w", err)
	}
	return clock.IsOpen, nil
}

// GetNextMarketOpen returns when the market opens next
func (c *AlpacaClient) GetNextMarketOpen(ctx context.Context) (time.Time, error) {
	clock, err := c.trading.GetClock()
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get clock: %w", err)
	}
	return clock.NextOpen, nil
}

// GetNextMarketClose returns when the market closes next
func (c *AlpacaClient) GetNextMarketClose(ctx context.Context) (time.Time, error) {
	clock, err := c.trading.GetClock()
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to get clock: %w", err)
	}
	return clock.NextClose, nil
}

// IsPaper returns true if using paper trading
func (c *AlpacaClient) IsPaper() bool {
	return c.isPaper
}
