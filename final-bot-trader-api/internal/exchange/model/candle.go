package model

import "time"

// Candle represents OHLCV (Open, High, Low, Close, Volume) candlestick data
type Candle struct {
	OpenTime  time.Time
	Open      float64
	High      float64
	Low       float64
	Close     float64
	Volume    float64
	CloseTime time.Time
}

// IsValid checks if the candle has valid data
func (c *Candle) IsValid() bool {
	return c.Open > 0 && c.High > 0 && c.Low > 0 && c.Close > 0 &&
		c.High >= c.Low &&
		c.High >= c.Open && c.High >= c.Close &&
		c.Low <= c.Open && c.Low <= c.Close
}

// IsBullish returns true if the candle closed higher than it opened
func (c *Candle) IsBullish() bool {
	return c.Close > c.Open
}

// IsBearish returns true if the candle closed lower than it opened
func (c *Candle) IsBearish() bool {
	return c.Close < c.Open
}

// Body returns the absolute difference between open and close
func (c *Candle) Body() float64 {
	if c.Close > c.Open {
		return c.Close - c.Open
	}
	return c.Open - c.Close
}

// Range returns the difference between high and low
func (c *Candle) Range() float64 {
	return c.High - c.Low
}

// UpperWick returns the upper shadow/wick length
func (c *Candle) UpperWick() float64 {
	if c.IsBullish() {
		return c.High - c.Close
	}
	return c.High - c.Open
}

// LowerWick returns the lower shadow/wick length
func (c *Candle) LowerWick() float64 {
	if c.IsBullish() {
		return c.Open - c.Low
	}
	return c.Close - c.Low
}
