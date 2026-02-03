package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	hterrors "github.com/sahmadiut/half-tunnel/internal/errors"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.InitialDelay != 1*time.Second {
		t.Errorf("expected InitialDelay 1s, got %v", cfg.InitialDelay)
	}
	if cfg.MaxDelay != 60*time.Second {
		t.Errorf("expected MaxDelay 60s, got %v", cfg.MaxDelay)
	}
	if cfg.Multiplier != 2.0 {
		t.Errorf("expected Multiplier 2.0, got %v", cfg.Multiplier)
	}
	if cfg.Jitter != 0.1 {
		t.Errorf("expected Jitter 0.1, got %v", cfg.Jitter)
	}
}

func TestRetryer_Reset(t *testing.T) {
	r := New(DefaultConfig())
	r.NextDelay() // increment attempts
	r.NextDelay()
	if r.Attempts() != 2 {
		t.Errorf("expected 2 attempts, got %d", r.Attempts())
	}
	r.Reset()
	if r.Attempts() != 0 {
		t.Errorf("expected 0 attempts after reset, got %d", r.Attempts())
	}
}

func TestRetryer_NextDelay(t *testing.T) {
	config := &Config{
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
		Jitter:       0, // no jitter for predictable tests
	}
	r := New(config)

	// First delay should be InitialDelay
	d1 := r.NextDelay()
	if d1 != 100*time.Millisecond {
		t.Errorf("expected 100ms, got %v", d1)
	}

	// Second delay should be 200ms (100ms * 2)
	d2 := r.NextDelay()
	if d2 != 200*time.Millisecond {
		t.Errorf("expected 200ms, got %v", d2)
	}

	// Third delay should be 400ms (200ms * 2)
	d3 := r.NextDelay()
	if d3 != 400*time.Millisecond {
		t.Errorf("expected 400ms, got %v", d3)
	}
}

func TestRetryer_NextDelay_ClampsToMax(t *testing.T) {
	config := &Config{
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     300 * time.Millisecond,
		Multiplier:   10.0,
		Jitter:       0,
	}
	r := New(config)

	r.NextDelay() // 100ms
	r.NextDelay() // 1000ms, but clamped to 300ms
	d3 := r.NextDelay()
	if d3 != 300*time.Millisecond {
		t.Errorf("expected 300ms (clamped), got %v", d3)
	}
}

func TestRetryer_NextDelay_WithJitter(t *testing.T) {
	config := &Config{
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
		Jitter:       0.5, // 50% jitter
	}
	r := New(config)

	// With 50% jitter, first delay should be between 50ms and 150ms
	d := r.NextDelay()
	if d < 50*time.Millisecond || d > 150*time.Millisecond {
		t.Errorf("expected delay between 50ms and 150ms, got %v", d)
	}
}

func TestRetryer_ShouldRetry(t *testing.T) {
	t.Run("unlimited retries", func(t *testing.T) {
		config := &Config{
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
			MaxAttempts:  0, // unlimited
		}
		r := New(config)
		for i := 0; i < 100; i++ {
			if !r.ShouldRetry() {
				t.Error("unlimited retries should always return true")
			}
			r.NextDelay()
		}
	})

	t.Run("limited retries", func(t *testing.T) {
		config := &Config{
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
			MaxAttempts:  3,
		}
		r := New(config)

		if !r.ShouldRetry() {
			t.Error("should retry on first attempt")
		}
		r.NextDelay() // attempt 1
		r.NextDelay() // attempt 2
		r.NextDelay() // attempt 3

		if r.ShouldRetry() {
			t.Error("should not retry after max attempts")
		}
	})
}

func TestRetryer_Wait(t *testing.T) {
	t.Run("waits for delay", func(t *testing.T) {
		config := &Config{
			InitialDelay: 50 * time.Millisecond,
			MaxDelay:     100 * time.Millisecond,
			Multiplier:   2.0,
			Jitter:       0,
			MaxAttempts:  0,
		}
		r := New(config)

		start := time.Now()
		err := r.Wait(context.Background())
		elapsed := time.Since(start)

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if elapsed < 40*time.Millisecond {
			t.Errorf("expected at least 40ms wait, got %v", elapsed)
		}
	})

	t.Run("returns error on max retries", func(t *testing.T) {
		config := &Config{
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
			MaxAttempts:  1,
		}
		r := New(config)
		r.NextDelay() // use up the attempt

		err := r.Wait(context.Background())
		if !errors.Is(err, hterrors.ErrMaxRetries) {
			t.Errorf("expected ErrMaxRetries, got %v", err)
		}
	})

	t.Run("returns error on context cancel", func(t *testing.T) {
		config := &Config{
			InitialDelay: 1 * time.Second,
			MaxDelay:     10 * time.Second,
			Multiplier:   2.0,
		}
		r := New(config)

		ctx, cancel := context.WithCancel(context.Background())
		go func() {
			time.Sleep(50 * time.Millisecond)
			cancel()
		}()

		err := r.Wait(ctx)
		if err != context.Canceled {
			t.Errorf("expected context.Canceled, got %v", err)
		}
	})
}

func TestRetryer_Do(t *testing.T) {
	t.Run("succeeds on first try", func(t *testing.T) {
		r := New(DefaultConfig())
		callCount := 0

		err := r.Do(context.Background(), func(ctx context.Context) error {
			callCount++
			return nil
		})

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if callCount != 1 {
			t.Errorf("expected 1 call, got %d", callCount)
		}
	})

	t.Run("retries retryable errors", func(t *testing.T) {
		config := &Config{
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
			MaxAttempts:  3,
		}
		r := New(config)
		callCount := 0

		err := r.Do(context.Background(), func(ctx context.Context) error {
			callCount++
			if callCount < 3 {
				return hterrors.ErrUpstreamUnavailable
			}
			return nil
		})

		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
		if callCount != 3 {
			t.Errorf("expected 3 calls, got %d", callCount)
		}
	})

	t.Run("does not retry non-retryable errors", func(t *testing.T) {
		config := &Config{
			InitialDelay: 1 * time.Millisecond,
			MaxDelay:     10 * time.Millisecond,
			Multiplier:   2.0,
			MaxAttempts:  5,
		}
		r := New(config)
		callCount := 0

		err := r.Do(context.Background(), func(ctx context.Context) error {
			callCount++
			return hterrors.ErrMaxSessionsReached // not retryable
		})

		if !errors.Is(err, hterrors.ErrMaxSessionsReached) {
			t.Errorf("expected ErrMaxSessionsReached, got %v", err)
		}
		if callCount != 1 {
			t.Errorf("expected 1 call for non-retryable error, got %d", callCount)
		}
	})
}

func TestBackoff(t *testing.T) {
	t.Run("calculates exponential backoff", func(t *testing.T) {
		initial := 100 * time.Millisecond
		max := 10 * time.Second

		d0 := Backoff(0, initial, max, 2.0, 0)
		if d0 != initial {
			t.Errorf("attempt 0 should return initial delay, got %v", d0)
		}

		d1 := Backoff(1, initial, max, 2.0, 0)
		if d1 != 100*time.Millisecond {
			t.Errorf("attempt 1 should be 100ms, got %v", d1)
		}

		d2 := Backoff(2, initial, max, 2.0, 0)
		if d2 != 200*time.Millisecond {
			t.Errorf("attempt 2 should be 200ms, got %v", d2)
		}

		d3 := Backoff(3, initial, max, 2.0, 0)
		if d3 != 400*time.Millisecond {
			t.Errorf("attempt 3 should be 400ms, got %v", d3)
		}
	})

	t.Run("clamps to max", func(t *testing.T) {
		initial := 100 * time.Millisecond
		max := 300 * time.Millisecond

		d := Backoff(10, initial, max, 2.0, 0)
		if d != max {
			t.Errorf("expected clamped to %v, got %v", max, d)
		}
	})
}
