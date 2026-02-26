package model

// AccountInfo represents general account information
type AccountInfo struct {
	TotalBalance     float64 // Total equity including unrealized PnL
	AvailableBalance float64 // Available for trading
	MarginBalance    float64 // Used as margin
	UnrealizedPnl    float64 // Total unrealized PnL
}

// Balance represents the balance of a specific asset/margin coin
type Balance struct {
	Asset              string  // e.g., "USDT"
	Balance            float64 // Total balance
	AvailableBalance   float64 // Available for trading
	CrossWalletBalance float64 // Cross wallet balance
	CrossUnPnl         float64 // Cross unrealized PnL
	MaxWithdrawAmount  float64 // Maximum withdrawable amount
	MarginAvailable    bool    // Whether margin is available
	UpdateTime         int64   // Last update timestamp
}

// UsedMarginPercent returns the percentage of balance used as margin
func (a *AccountInfo) UsedMarginPercent() float64 {
	if a.TotalBalance == 0 {
		return 0
	}
	return (a.MarginBalance / a.TotalBalance) * 100
}

// AvailablePercent returns the percentage of balance available
func (a *AccountInfo) AvailablePercent() float64 {
	if a.TotalBalance == 0 {
		return 0
	}
	return (a.AvailableBalance / a.TotalBalance) * 100
}
