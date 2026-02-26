package exchange

import (
	"context"

	"final-bot-trader-api/internal/exchange/model"
)

// Client defines the interface for interacting with a cryptocurrency exchange
type Client interface {
	// Market Data
	// GetPrice returns the current price for a symbol
	GetPrice(ctx context.Context, symbol string) (float64, error)

	// GetKlines returns historical candlestick data
	GetKlines(ctx context.Context, symbol, interval string, limit int, startTime, endTime int64) ([]model.Candle, error)

	// GetSymbolInfo returns trading rules and precision for a symbol
	GetSymbolInfo(ctx context.Context, symbol string) (*model.SymbolInfo, error)

	// Account Information
	// GetAccountInfo returns general account information
	GetAccountInfo(ctx context.Context) (*model.AccountInfo, error)

	// GetBalance returns the balance for all assets
	GetBalance(ctx context.Context) ([]model.Balance, error)

	// Order Management
	// PlaceOrder places a new order
	PlaceOrder(ctx context.Context, order *model.OrderRequest) (*model.OrderResponse, error)

	// CancelOrder cancels an existing order
	CancelOrder(ctx context.Context, symbol, orderID string) error

	// GetOrderStatus returns the status of an order
	GetOrderStatus(ctx context.Context, symbol, orderID string) (*model.OrderStatus, error)

	// GetOpenOrders returns all open orders for a symbol
	GetOpenOrders(ctx context.Context, symbol string) ([]model.OrderStatus, error)

	// Position Management
	// GetPositions returns all open positions
	GetPositions(ctx context.Context) ([]model.Position, error)

	// SetLeverage sets the leverage for a symbol
	SetLeverage(ctx context.Context, symbol string, leverage int) error

	// SetMarginType sets the margin type (ISOLATED or CROSS) for a symbol
	SetMarginType(ctx context.Context, symbol string, marginType string) error

	// SetTPSL sets Take Profit and Stop Loss for an existing position
	SetTPSL(ctx context.Context, symbol string, tpPrice, slPrice float64, pricePrecision int) error
}

// Ensure BitunixClient implements Client interface (compile-time check)
var _ Client = (*BitunixClient)(nil)
