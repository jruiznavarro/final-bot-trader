package utils

import (
	"context"
	"math"
	"time"
)

// RetryConfig holds configuration for retry logic
type RetryConfig struct {
	MaxRetries        int
	InitialDelay      time.Duration
	MaxDelay          time.Duration
	BackoffMultiplier float64
}

// DefaultRetryConfig returns sensible defaults for retry logic
func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:        3,
		InitialDelay:      1 * time.Second,
		MaxDelay:          30 * time.Second,
		BackoffMultiplier: 2.0,
	}
}

// RetryableFunc is a function that can be retried
type RetryableFunc func() error

// IsRetryable is a function that determines if an error is retryable
type IsRetryable func(error) bool

// Retry executes a function with exponential backoff retry logic
func Retry(ctx context.Context, config RetryConfig, fn RetryableFunc, isRetryable IsRetryable) error {
	var lastErr error

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		// Check if context is cancelled
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Execute the function
		err := fn()
		if err == nil {
			return nil
		}

		lastErr = err

		// Check if error is retryable
		if isRetryable != nil && !isRetryable(err) {
			return err
		}

		// Don't wait after the last attempt
		if attempt < config.MaxRetries {
			delay := calculateDelay(attempt, config)

			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(delay):
			}
		}
	}

	return lastErr
}

// calculateDelay calculates the delay for a given attempt using exponential backoff
func calculateDelay(attempt int, config RetryConfig) time.Duration {
	delay := float64(config.InitialDelay) * math.Pow(config.BackoffMultiplier, float64(attempt))

	if delay > float64(config.MaxDelay) {
		return config.MaxDelay
	}

	return time.Duration(delay)
}

// RetryWithResult retries a function that returns a value and an error
func RetryWithResult[T any](ctx context.Context, config RetryConfig, fn func() (T, error), isRetryable IsRetryable) (T, error) {
	var result T
	var lastErr error

	for attempt := 0; attempt <= config.MaxRetries; attempt++ {
		select {
		case <-ctx.Done():
			return result, ctx.Err()
		default:
		}

		var err error
		result, err = fn()
		if err == nil {
			return result, nil
		}

		lastErr = err

		if isRetryable != nil && !isRetryable(err) {
			return result, err
		}

		if attempt < config.MaxRetries {
			delay := calculateDelay(attempt, config)

			select {
			case <-ctx.Done():
				return result, ctx.Err()
			case <-time.After(delay):
			}
		}
	}

	return result, lastErr
}
