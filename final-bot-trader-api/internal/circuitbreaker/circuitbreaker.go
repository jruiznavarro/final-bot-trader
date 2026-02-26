package circuitbreaker

import (
	"errors"
	"sync"
	"time"
)

// State represents the current state of the circuit breaker
type State int

const (
	StateClosed State = iota // Normal operation, requests allowed
	StateOpen                // Circuit is open, requests blocked
	StateHalfOpen            // Testing if service recovered
)

func (s State) String() string {
	switch s {
	case StateClosed:
		return "CLOSED"
	case StateOpen:
		return "OPEN"
	case StateHalfOpen:
		return "HALF_OPEN"
	default:
		return "UNKNOWN"
	}
}

// Config holds the configuration for the circuit breaker
type Config struct {
	MaxFailures      int           // Number of failures before opening
	ResetTimeout     time.Duration // Time to wait before half-open
	HalfOpenMaxCalls int           // Max calls allowed in half-open state
}

// DefaultConfig returns sensible defaults for circuit breaker
func DefaultConfig() Config {
	return Config{
		MaxFailures:      5,
		ResetTimeout:     30 * time.Second,
		HalfOpenMaxCalls: 3,
	}
}

// CircuitBreaker implements the circuit breaker pattern
type CircuitBreaker struct {
	config          Config
	state           State
	failures        int
	successes       int
	lastFailureTime time.Time
	halfOpenCalls   int
	mu              sync.RWMutex
	onStateChange   func(from, to State)
}

// ErrCircuitOpen is returned when the circuit is open
var ErrCircuitOpen = errors.New("circuit breaker is OPEN")

// ErrTooManyCalls is returned when too many calls in half-open state
var ErrTooManyCalls = errors.New("too many calls in half-open state")

// NewCircuitBreaker creates a new circuit breaker with the given config
func NewCircuitBreaker(config Config) *CircuitBreaker {
	return &CircuitBreaker{
		config: config,
		state:  StateClosed,
	}
}

// SetStateChangeCallback sets a callback function for state changes
func (cb *CircuitBreaker) SetStateChangeCallback(fn func(from, to State)) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.onStateChange = fn
}

// State returns the current state of the circuit breaker
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Call executes the given function if the circuit allows it
// isFailure is a function that determines if an error should count as a failure
func (cb *CircuitBreaker) Call(fn func() error, isFailure func(error) bool) error {
	if err := cb.beforeCall(); err != nil {
		return err
	}

	err := fn()
	cb.afterCall(err, isFailure)
	return err
}

// beforeCall checks if the call should be allowed
func (cb *CircuitBreaker) beforeCall() error {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	switch cb.state {
	case StateClosed:
		return nil

	case StateOpen:
		// Check if reset timeout has passed
		if time.Since(cb.lastFailureTime) >= cb.config.ResetTimeout {
			cb.setState(StateHalfOpen)
			cb.halfOpenCalls = 1
			return nil
		}
		return ErrCircuitOpen

	case StateHalfOpen:
		cb.halfOpenCalls++
		if cb.halfOpenCalls > cb.config.HalfOpenMaxCalls {
			return ErrTooManyCalls
		}
		return nil
	}

	return nil
}

// afterCall updates the state based on the result
func (cb *CircuitBreaker) afterCall(err error, isFailure func(error) bool) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	// Determine if this should count as a failure
	failed := err != nil && (isFailure == nil || isFailure(err))

	switch cb.state {
	case StateClosed:
		if failed {
			cb.failures++
			cb.lastFailureTime = time.Now()
			if cb.failures >= cb.config.MaxFailures {
				cb.setState(StateOpen)
			}
		} else {
			// Reset failures on success
			cb.failures = 0
		}

	case StateHalfOpen:
		if failed {
			// Back to open state
			cb.setState(StateOpen)
			cb.lastFailureTime = time.Now()
		} else {
			cb.successes++
			// If enough successes, close the circuit
			if cb.successes >= cb.config.HalfOpenMaxCalls {
				cb.setState(StateClosed)
			}
		}
	}
}

// setState changes the state and calls the callback if set
func (cb *CircuitBreaker) setState(newState State) {
	if cb.state == newState {
		return
	}

	oldState := cb.state
	cb.state = newState

	// Reset counters on state change
	switch newState {
	case StateClosed:
		cb.failures = 0
		cb.successes = 0
	case StateHalfOpen:
		cb.successes = 0
		cb.halfOpenCalls = 0
	case StateOpen:
		cb.successes = 0
	}

	// Call callback if set
	if cb.onStateChange != nil {
		go cb.onStateChange(oldState, newState)
	}
}

// Reset manually resets the circuit breaker to closed state
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.setState(StateClosed)
}

// Failures returns the current failure count
func (cb *CircuitBreaker) Failures() int {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.failures
}
