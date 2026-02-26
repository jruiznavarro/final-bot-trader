package utils

import (
	"fmt"
	"time"
)

// RateLimitError represents an HTTP 429 rate limit error
type RateLimitError struct {
	StatusCode int
	RetryAfter time.Duration
	Message    string
	Remaining  int       // Requests remaining in current window
	ResetTime  time.Time // When the rate limit resets
}

// Error implements the error interface
func (e *RateLimitError) Error() string {
	return fmt.Sprintf("rate limit exceeded (HTTP %d): %s, retry after %v", e.StatusCode, e.Message, e.RetryAfter)
}

// IsRateLimitError checks if an error is a RateLimitError
func IsRateLimitError(err error) bool {
	_, ok := err.(*RateLimitError)
	return ok
}

// APIError represents a general API error
type APIError struct {
	StatusCode int
	Code       string
	Message    string
}

// Error implements the error interface
func (e *APIError) Error() string {
	if e.Code != "" {
		return fmt.Sprintf("API error %d (code: %s): %s", e.StatusCode, e.Code, e.Message)
	}
	return fmt.Sprintf("API error %d: %s", e.StatusCode, e.Message)
}

// IsAPIError checks if an error is an APIError
func IsAPIError(err error) bool {
	_, ok := err.(*APIError)
	return ok
}

// ExchangeError represents an exchange-specific error
type ExchangeError struct {
	Exchange string
	Code     int
	Message  string
}

// Error implements the error interface
func (e *ExchangeError) Error() string {
	return fmt.Sprintf("%s error (code: %d): %s", e.Exchange, e.Code, e.Message)
}

// InsufficientBalanceError indicates insufficient balance for an operation
type InsufficientBalanceError struct {
	Required  float64
	Available float64
	Asset     string
}

// Error implements the error interface
func (e *InsufficientBalanceError) Error() string {
	return fmt.Sprintf("insufficient %s balance: required %.8f, available %.8f", e.Asset, e.Required, e.Available)
}

// OrderError represents an error related to order operations
type OrderError struct {
	OrderID string
	Symbol  string
	Message string
}

// Error implements the error interface
func (e *OrderError) Error() string {
	return fmt.Sprintf("order error for %s (order: %s): %s", e.Symbol, e.OrderID, e.Message)
}
