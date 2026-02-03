// Package circuitbreaker provides a circuit breaker implementation for the Half-Tunnel system.
package circuitbreaker

import (
	"context"
	"sync"
	"time"

	hterrors "github.com/sahmadiut/half-tunnel/internal/errors"
)

// State represents the state of the circuit breaker.
type State int

const (
	// StateClosed means the circuit is closed and requests are allowed.
	StateClosed State = iota
	// StateOpen means the circuit is open and requests are not allowed.
	StateOpen
	// StateHalfOpen means the circuit is half-open and a trial request is allowed.
	StateHalfOpen
)

// String returns the string representation of the state.
func (s State) String() string {
	switch s {
	case StateClosed:
		return "closed"
	case StateOpen:
		return "open"
	case StateHalfOpen:
		return "half-open"
	default:
		return "unknown"
	}
}

// Config holds circuit breaker configuration.
type Config struct {
	// MaxFailures is the number of failures before opening the circuit.
	MaxFailures int
	// Timeout is the duration the circuit stays open before transitioning to half-open.
	Timeout time.Duration
	// MaxHalfOpenRequests is the number of requests allowed in half-open state.
	MaxHalfOpenRequests int
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		MaxFailures:         5,
		Timeout:             30 * time.Second,
		MaxHalfOpenRequests: 1,
	}
}

// CircuitBreaker implements the circuit breaker pattern.
type CircuitBreaker struct {
	config *Config

	state            State
	failures         int
	successes        int
	halfOpenRequests int
	lastFailureTime  time.Time
	openedAt         time.Time

	mu sync.RWMutex

	// Callbacks
	onStateChange func(from, to State)
}

// New creates a new CircuitBreaker with the given configuration.
func New(config *Config) *CircuitBreaker {
	if config == nil {
		config = DefaultConfig()
	}
	return &CircuitBreaker{
		config: config,
		state:  StateClosed,
	}
}

// SetOnStateChange sets the callback function for state changes.
func (cb *CircuitBreaker) SetOnStateChange(fn func(from, to State)) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.onStateChange = fn
}

// State returns the current state of the circuit breaker.
func (cb *CircuitBreaker) State() State {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.currentState()
}

// currentState returns the current state, updating from open to half-open if timeout passed.
// Must be called with at least a read lock held.
func (cb *CircuitBreaker) currentState() State {
	if cb.state == StateOpen && time.Since(cb.openedAt) >= cb.config.Timeout {
		return StateHalfOpen
	}
	return cb.state
}

// Allow returns true if a request should be allowed through.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	state := cb.currentState()

	switch state {
	case StateClosed:
		return true
	case StateOpen:
		return false
	case StateHalfOpen:
		// Update state to half-open if timeout has passed
		if cb.state == StateOpen {
			cb.transitionTo(StateHalfOpen)
		}
		if cb.halfOpenRequests < cb.config.MaxHalfOpenRequests {
			cb.halfOpenRequests++
			return true
		}
		return false
	}

	return false
}

// RecordSuccess records a successful request.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	state := cb.currentState()

	switch state {
	case StateClosed:
		// Reset failure count on success
		cb.failures = 0
	case StateHalfOpen:
		cb.successes++
		// If we have enough successes in half-open state, close the circuit
		if cb.successes >= cb.config.MaxHalfOpenRequests {
			cb.transitionTo(StateClosed)
		}
	}
}

// RecordFailure records a failed request.
func (cb *CircuitBreaker) RecordFailure() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	state := cb.currentState()
	cb.lastFailureTime = time.Now()

	switch state {
	case StateClosed:
		cb.failures++
		if cb.failures >= cb.config.MaxFailures {
			cb.transitionTo(StateOpen)
		}
	case StateHalfOpen:
		// Any failure in half-open state opens the circuit
		cb.transitionTo(StateOpen)
	}
}

// transitionTo transitions to a new state.
// Must be called with the lock held.
func (cb *CircuitBreaker) transitionTo(newState State) {
	if cb.state == newState {
		return
	}

	oldState := cb.state
	cb.state = newState

	switch newState {
	case StateClosed:
		cb.failures = 0
		cb.successes = 0
		cb.halfOpenRequests = 0
	case StateOpen:
		cb.openedAt = time.Now()
		cb.halfOpenRequests = 0
		cb.successes = 0
	case StateHalfOpen:
		cb.halfOpenRequests = 0
		cb.successes = 0
	}

	if cb.onStateChange != nil {
		cb.onStateChange(oldState, newState)
	}
}

// Execute executes a function through the circuit breaker.
// Returns ErrCircuitOpen if the circuit is open.
func (cb *CircuitBreaker) Execute(fn func() error) error {
	if !cb.Allow() {
		return hterrors.ErrCircuitOpen
	}

	err := fn()
	if err != nil {
		cb.RecordFailure()
		return err
	}

	cb.RecordSuccess()
	return nil
}

// ExecuteWithContext executes a function through the circuit breaker with context support.
func (cb *CircuitBreaker) ExecuteWithContext(ctx context.Context, fn func(ctx context.Context) error) error {
	if !cb.Allow() {
		return hterrors.ErrCircuitOpen
	}

	err := fn(ctx)
	if err != nil {
		cb.RecordFailure()
		return err
	}

	cb.RecordSuccess()
	return nil
}

// Reset resets the circuit breaker to closed state.
func (cb *CircuitBreaker) Reset() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.transitionTo(StateClosed)
}

// Stats returns statistics about the circuit breaker.
type Stats struct {
	State            State
	Failures         int
	Successes        int
	HalfOpenRequests int
	LastFailureTime  time.Time
	OpenedAt         time.Time
}

// Stats returns the current statistics.
func (cb *CircuitBreaker) Stats() Stats {
	cb.mu.RLock()
	defer cb.mu.RUnlock()

	return Stats{
		State:            cb.currentState(),
		Failures:         cb.failures,
		Successes:        cb.successes,
		HalfOpenRequests: cb.halfOpenRequests,
		LastFailureTime:  cb.lastFailureTime,
		OpenedAt:         cb.openedAt,
	}
}
