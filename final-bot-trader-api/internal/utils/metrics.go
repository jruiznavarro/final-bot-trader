package utils

import (
	"log"
	"sync"
	"time"
)

// APIMetrics holds metrics for API calls
type APIMetrics struct {
	TotalCalls     int64
	SuccessCalls   int64
	FailedCalls    int64
	TotalLatency   time.Duration
	LastCallTime   time.Time
	LastError      error
	LastErrorTime  time.Time
}

// MetricsCollector collects and reports metrics
type MetricsCollector struct {
	mu       sync.RWMutex
	metrics  map[string]*APIMetrics // key: "exchange:endpoint"
	cbMetrics map[string]int64      // circuit breaker failures by exchange
}

var (
	collector     *MetricsCollector
	collectorOnce sync.Once
)

// GetMetricsCollector returns the singleton metrics collector
func GetMetricsCollector() *MetricsCollector {
	collectorOnce.Do(func() {
		collector = &MetricsCollector{
			metrics:   make(map[string]*APIMetrics),
			cbMetrics: make(map[string]int64),
		}
	})
	return collector
}

// RecordAPICall records an API call metric
func (mc *MetricsCollector) RecordAPICall(exchange, endpoint, method string, duration time.Duration, err error) {
	mc.mu.Lock()
	defer mc.mu.Unlock()

	key := exchange + ":" + endpoint
	m, exists := mc.metrics[key]
	if !exists {
		m = &APIMetrics{}
		mc.metrics[key] = m
	}

	m.TotalCalls++
	m.TotalLatency += duration
	m.LastCallTime = time.Now()

	if err != nil {
		m.FailedCalls++
		m.LastError = err
		m.LastErrorTime = time.Now()
	} else {
		m.SuccessCalls++
	}
}

// RecordCircuitBreakerFailure records a circuit breaker failure
func (mc *MetricsCollector) RecordCircuitBreakerFailure(exchange string) {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	mc.cbMetrics[exchange]++
}

// GetMetrics returns a copy of the metrics for an endpoint
func (mc *MetricsCollector) GetMetrics(exchange, endpoint string) *APIMetrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	key := exchange + ":" + endpoint
	m, exists := mc.metrics[key]
	if !exists {
		return nil
	}

	// Return a copy
	return &APIMetrics{
		TotalCalls:    m.TotalCalls,
		SuccessCalls:  m.SuccessCalls,
		FailedCalls:   m.FailedCalls,
		TotalLatency:  m.TotalLatency,
		LastCallTime:  m.LastCallTime,
		LastError:     m.LastError,
		LastErrorTime: m.LastErrorTime,
	}
}

// GetAllMetrics returns all collected metrics
func (mc *MetricsCollector) GetAllMetrics() map[string]*APIMetrics {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	result := make(map[string]*APIMetrics)
	for k, v := range mc.metrics {
		result[k] = &APIMetrics{
			TotalCalls:    v.TotalCalls,
			SuccessCalls:  v.SuccessCalls,
			FailedCalls:   v.FailedCalls,
			TotalLatency:  v.TotalLatency,
			LastCallTime:  v.LastCallTime,
			LastError:     v.LastError,
			LastErrorTime: v.LastErrorTime,
		}
	}
	return result
}

// AverageLatency returns the average latency for an endpoint
func (m *APIMetrics) AverageLatency() time.Duration {
	if m.TotalCalls == 0 {
		return 0
	}
	return time.Duration(int64(m.TotalLatency) / m.TotalCalls)
}

// SuccessRate returns the success rate as a percentage
func (m *APIMetrics) SuccessRate() float64 {
	if m.TotalCalls == 0 {
		return 0
	}
	return float64(m.SuccessCalls) / float64(m.TotalCalls) * 100
}

// Helper functions for direct use

// RecordAPIMetrics records API call metrics (convenience function)
func RecordAPIMetrics(exchange, endpoint, method string, duration time.Duration, err error) {
	GetMetricsCollector().RecordAPICall(exchange, endpoint, method, duration, err)
}

// RecordCircuitBreakerFailure records a circuit breaker failure (convenience function)
func RecordCircuitBreakerFailure(exchange string) {
	GetMetricsCollector().RecordCircuitBreakerFailure(exchange)
	log.Printf("[Metrics] Circuit breaker failure recorded for %s", exchange)
}

// ExtractEndpoint extracts the endpoint name from a full path
func ExtractEndpoint(path string) string {
	// Remove query string if present
	if idx := len(path) - 1; idx > 0 {
		for i := 0; i < len(path); i++ {
			if path[i] == '?' {
				path = path[:i]
				break
			}
		}
	}
	return path
}
