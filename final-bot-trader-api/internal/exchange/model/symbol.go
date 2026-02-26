package model

// SymbolInfo contains trading rules and precision information for a symbol
type SymbolInfo struct {
	Symbol      string  // e.g., "BTCUSDT"
	BaseCoin    string  // e.g., "BTC"
	QuoteCoin   string  // e.g., "USDT"
	MinQuantity float64 // Minimum order quantity
	MaxQuantity float64 // Maximum order quantity
	StepSize    float64 // Quantity step size
	MinPrice    float64 // Minimum price
	MaxPrice    float64 // Maximum price
	TickSize    float64 // Price tick size
	MinNotional float64 // Minimum notional value
}

// QuantityPrecision returns the number of decimal places for quantity
func (s *SymbolInfo) QuantityPrecision() int {
	if s.StepSize == 0 {
		return 0
	}
	precision := 0
	stepSize := s.StepSize
	for stepSize < 1 {
		stepSize *= 10
		precision++
	}
	return precision
}

// PricePrecision returns the number of decimal places for price
func (s *SymbolInfo) PricePrecision() int {
	if s.TickSize == 0 {
		return 0
	}
	precision := 0
	tickSize := s.TickSize
	for tickSize < 1 {
		tickSize *= 10
		precision++
	}
	return precision
}

// RoundQuantity rounds a quantity to the valid step size
func (s *SymbolInfo) RoundQuantity(qty float64) float64 {
	if s.StepSize == 0 {
		return qty
	}
	steps := int(qty / s.StepSize)
	return float64(steps) * s.StepSize
}

// RoundPrice rounds a price to the valid tick size
func (s *SymbolInfo) RoundPrice(price float64) float64 {
	if s.TickSize == 0 {
		return price
	}
	ticks := int(price / s.TickSize)
	return float64(ticks) * s.TickSize
}

// ValidateQuantity checks if a quantity is valid for this symbol
func (s *SymbolInfo) ValidateQuantity(qty float64) bool {
	return qty >= s.MinQuantity && (s.MaxQuantity == 0 || qty <= s.MaxQuantity)
}

// ValidatePrice checks if a price is valid for this symbol
func (s *SymbolInfo) ValidatePrice(price float64) bool {
	return price >= s.MinPrice && (s.MaxPrice == 0 || price <= s.MaxPrice)
}
