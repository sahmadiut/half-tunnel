// Package retry provides retry functionality with exponential backoff for the Half-Tunnel system.
package retry

import (
	"context"
	"math"
	"math/rand"
	"sync"
	"time"

	hterrors "github.com/sahmadiut/half-tunnel/internal/errors"
)

// Config holds retry configuration settings.
type Config struct {
	// InitialDelay is the initial delay before the first retry.
	InitialDelay time.Duration
	// MaxDelay is the maximum delay between retries.
	MaxDelay time.Duration
	// Multiplier is the exponential backoff multiplier.
	Multiplier float64
	// Jitter is the random jitter factor (0.0 to 1.0).
	Jitter float64
	// MaxAttempts is the maximum number of retry attempts (0 = unlimited).
	MaxAttempts int
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		InitialDelay: 1 * time.Second,
		MaxDelay:     60 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.1,
		MaxAttempts:  0, // unlimited
	}
}

// Retryer handles retry logic with exponential backoff.
type Retryer struct {
	config   *Config
	attempts int
	rng      *rand.Rand
	mu       sync.Mutex
}

// New creates a new Retryer with the given configuration.
func New(config *Config) *Retryer {
	if config == nil {
		config = DefaultConfig()
	}
	return &Retryer{
		config:   config,
		attempts: 0,
		rng:      rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Reset resets the retry state.
func (r *Retryer) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.attempts = 0
}

// Attempts returns the current number of attempts.
func (r *Retryer) Attempts() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.attempts
}

// NextDelay calculates the next delay based on exponential backoff with jitter.
func (r *Retryer) NextDelay() time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()

	r.attempts++

	// Calculate base delay with exponential backoff
	delay := float64(r.config.InitialDelay) * math.Pow(r.config.Multiplier, float64(r.attempts-1))

	// Apply jitter
	if r.config.Jitter > 0 {
		jitterRange := delay * r.config.Jitter
		jitter := (r.rng.Float64() * 2 * jitterRange) - jitterRange
		delay += jitter
	}

	// Clamp to max delay
	if delay > float64(r.config.MaxDelay) {
		delay = float64(r.config.MaxDelay)
	}

	return time.Duration(delay)
}

// ShouldRetry returns true if we should retry.
func (r *Retryer) ShouldRetry() bool {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.config.MaxAttempts <= 0 {
		return true
	}
	return r.attempts < r.config.MaxAttempts
}

// Wait waits for the next retry delay or until the context is cancelled.
// Returns an error if the context is cancelled or max retries is reached.
func (r *Retryer) Wait(ctx context.Context) error {
	if !r.ShouldRetry() {
		return hterrors.ErrMaxRetries
	}

	delay := r.NextDelay()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(delay):
		return nil
	}
}

// RetryFunc is a function that can be retried.
type RetryFunc func(ctx context.Context) error

// Do executes the function with retries.
// It returns the first successful result or the last error after all retries.
func (r *Retryer) Do(ctx context.Context, fn RetryFunc) error {
	for {
		err := fn(ctx)
		if err == nil {
			r.Reset()
			return nil
		}

		// Check if error is retryable
		if !hterrors.IsRetryable(err) {
			return err
		}

		// Wait for next retry
		if waitErr := r.Wait(ctx); waitErr != nil {
			return waitErr
		}
	}
}

// DoWithResult executes a function with retries and returns a result.
type RetryFuncWithResult[T any] func(ctx context.Context) (T, error)

// DoWithResult executes the function with retries and returns the result.
func DoWithResult[T any](ctx context.Context, r *Retryer, fn RetryFuncWithResult[T]) (T, error) {
	var zero T
	for {
		result, err := fn(ctx)
		if err == nil {
			r.Reset()
			return result, nil
		}

		// Check if error is retryable
		if !hterrors.IsRetryable(err) {
			return zero, err
		}

		// Wait for next retry
		if waitErr := r.Wait(ctx); waitErr != nil {
			return zero, waitErr
		}
	}
}

// Backoff calculates a delay using exponential backoff with jitter.
// This is a standalone function for one-off backoff calculations.
func Backoff(attempt int, initialDelay, maxDelay time.Duration, multiplier, jitter float64) time.Duration {
	if attempt <= 0 {
		return initialDelay
	}

	delay := float64(initialDelay) * math.Pow(multiplier, float64(attempt-1))

	// Apply jitter
	if jitter > 0 {
		jitterRange := delay * jitter
		jitterValue := (rand.Float64() * 2 * jitterRange) - jitterRange
		delay += jitterValue
	}

	// Clamp to max delay
	if delay > float64(maxDelay) {
		delay = float64(maxDelay)
	}

	return time.Duration(delay)
}
