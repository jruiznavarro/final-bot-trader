package model

// OrderSide represents the side of an order (BUY or SELL)
type OrderSide string

const (
	OrderSideBuy  OrderSide = "BUY"
	OrderSideSell OrderSide = "SELL"
)

// OrderType represents the type of order
type OrderType string

const (
	OrderTypeMarket OrderType = "MARKET"
	OrderTypeLimit  OrderType = "LIMIT"
)

// OrderStatus represents the status of an order
type OrderStatusType string

const (
	OrderStatusNew         OrderStatusType = "NEW"
	OrderStatusPartFilled  OrderStatusType = "PART_FILLED"
	OrderStatusFilled      OrderStatusType = "FILLED"
	OrderStatusCanceled    OrderStatusType = "CANCELED"
	OrderStatusInit        OrderStatusType = "INIT"
)

// TradeSide represents the trade side (OPEN or CLOSE position)
type TradeSide string

const (
	TradeSideOpen  TradeSide = "OPEN"
	TradeSideClose TradeSide = "CLOSE"
)

// OrderRequest represents a request to place an order
type OrderRequest struct {
	Symbol            string
	Side              string  // BUY or SELL
	Type              string  // MARKET or LIMIT
	Quantity          float64
	Price             float64 // Required for LIMIT orders
	TP                float64 // Take Profit price
	SL                float64 // Stop Loss price
	QuantityPrecision int
	PricePrecision    int
	TradeSide         string  // OPEN or CLOSE
	PositionSide      string  // LONG or SHORT (for hedge mode)
	ReduceOnly        bool
}

// OrderResponse represents the response after placing an order
type OrderResponse struct {
	OrderID       int64
	Symbol        string
	Status        string
	Side          string
	Type          string
	ClientOrderID string
}

// OrderStatus represents the detailed status of an order
type OrderStatus struct {
	OrderID       int64
	Symbol        string
	Status        string
	ClientOrderID string
	Price         string
	AvgPrice      string
	OrigQty       string
	ExecutedQty   string
	CumQuote      string
	TimeInForce   string
	Type          string
	Side          string
	StopPrice     string
	Time          int64
	Quantity      float64
	FilledQty     float64
	RemainingQty  float64
	UpdateTime    int64
	TPPrice       string // Take Profit price
	SLPrice       string // Stop Loss price
}

// IsFilled returns true if the order is completely filled
func (o *OrderStatus) IsFilled() bool {
	return o.Status == string(OrderStatusFilled)
}

// IsPartiallyFilled returns true if the order is partially filled
func (o *OrderStatus) IsPartiallyFilled() bool {
	return o.Status == string(OrderStatusPartFilled)
}

// IsCanceled returns true if the order is canceled
func (o *OrderStatus) IsCanceled() bool {
	return o.Status == string(OrderStatusCanceled)
}

// IsActive returns true if the order is still active (NEW or PART_FILLED)
func (o *OrderStatus) IsActive() bool {
	return o.Status == string(OrderStatusNew) || o.Status == string(OrderStatusPartFilled)
}
