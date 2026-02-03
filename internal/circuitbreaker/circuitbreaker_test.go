package circuitbreaker

import (
	"context"
	"errors"
	"testing"
	"time"

	hterrors "github.com/sahmadiut/half-tunnel/internal/errors"
)

func TestCircuitBreaker_DefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.MaxFailures != 5 {
		t.Errorf("expected MaxFailures 5, got %d", cfg.MaxFailures)
	}
	if cfg.Timeout != 30*time.Second {
		t.Errorf("expected Timeout 30s, got %v", cfg.Timeout)
	}
	if cfg.MaxHalfOpenRequests != 1 {
		t.Errorf("expected MaxHalfOpenRequests 1, got %d", cfg.MaxHalfOpenRequests)
	}
}

func TestCircuitBreaker_StartsInClosedState(t *testing.T) {
	cb := New(DefaultConfig())
	if cb.State() != StateClosed {
		t.Errorf("expected StateClosed, got %v", cb.State())
	}
}

func TestCircuitBreaker_AllowsRequestsWhenClosed(t *testing.T) {
	cb := New(DefaultConfig())
	if !cb.Allow() {
		t.Error("expected Allow() to return true when closed")
	}
}

func TestCircuitBreaker_OpensAfterMaxFailures(t *testing.T) {
	config := &Config{
		MaxFailures:         3,
		Timeout:             1 * time.Second,
		MaxHalfOpenRequests: 1,
	}
	cb := New(config)

	cb.RecordFailure()
	cb.RecordFailure()
	if cb.State() != StateClosed {
		t.Error("should still be closed after 2 failures")
	}

	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Errorf("expected StateOpen after 3 failures, got %v", cb.State())
	}
}

func TestCircuitBreaker_DoesNotAllowWhenOpen(t *testing.T) {
	config := &Config{
		MaxFailures:         1,
		Timeout:             1 * time.Second,
		MaxHalfOpenRequests: 1,
	}
	cb := New(config)

	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Fatal("expected StateOpen")
	}

	if cb.Allow() {
		t.Error("expected Allow() to return false when open")
	}
}

func TestCircuitBreaker_TransitionsToHalfOpenAfterTimeout(t *testing.T) {
	config := &Config{
		MaxFailures:         1,
		Timeout:             50 * time.Millisecond,
		MaxHalfOpenRequests: 1,
	}
	cb := New(config)

	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Fatal("expected StateOpen")
	}

	time.Sleep(60 * time.Millisecond)

	if cb.State() != StateHalfOpen {
		t.Errorf("expected StateHalfOpen after timeout, got %v", cb.State())
	}
}

func TestCircuitBreaker_HalfOpenAllowsLimitedRequests(t *testing.T) {
	config := &Config{
		MaxFailures:         1,
		Timeout:             50 * time.Millisecond,
		MaxHalfOpenRequests: 2,
	}
	cb := New(config)

	cb.RecordFailure()
	time.Sleep(60 * time.Millisecond)

	// First two requests should be allowed
	if !cb.Allow() {
		t.Error("first half-open request should be allowed")
	}
	if !cb.Allow() {
		t.Error("second half-open request should be allowed")
	}
	// Third should be denied
	if cb.Allow() {
		t.Error("third half-open request should be denied")
	}
}

func TestCircuitBreaker_ClosesOnSuccessInHalfOpen(t *testing.T) {
	config := &Config{
		MaxFailures:         1,
		Timeout:             50 * time.Millisecond,
		MaxHalfOpenRequests: 1,
	}
	cb := New(config)

	cb.RecordFailure()
	time.Sleep(60 * time.Millisecond)

	cb.Allow() // use the half-open request
	cb.RecordSuccess()

	if cb.State() != StateClosed {
		t.Errorf("expected StateClosed after success in half-open, got %v", cb.State())
	}
}

func TestCircuitBreaker_OpensOnFailureInHalfOpen(t *testing.T) {
	config := &Config{
		MaxFailures:         1,
		Timeout:             50 * time.Millisecond,
		MaxHalfOpenRequests: 1,
	}
	cb := New(config)

	cb.RecordFailure()
	time.Sleep(60 * time.Millisecond)

	cb.Allow() // use the half-open request
	cb.RecordFailure()

	if cb.State() != StateOpen {
		t.Errorf("expected StateOpen after failure in half-open, got %v", cb.State())
	}
}

func TestCircuitBreaker_Execute(t *testing.T) {
	t.Run("successful execution", func(t *testing.T) {
		cb := New(DefaultConfig())
		err := cb.Execute(func() error {
			return nil
		})
		if err != nil {
			t.Errorf("expected no error, got %v", err)
		}
	})

	t.Run("returns function error", func(t *testing.T) {
		cb := New(DefaultConfig())
		expectedErr := errors.New("test error")
		err := cb.Execute(func() error {
			return expectedErr
		})
		if err != expectedErr {
			t.Errorf("expected %v, got %v", expectedErr, err)
		}
	})

	t.Run("returns ErrCircuitOpen when open", func(t *testing.T) {
		config := &Config{
			MaxFailures:         1,
			Timeout:             1 * time.Second,
			MaxHalfOpenRequests: 1,
		}
		cb := New(config)
		cb.RecordFailure()

		err := cb.Execute(func() error {
			t.Error("function should not be called when circuit is open")
			return nil
		})
		if !errors.Is(err, hterrors.ErrCircuitOpen) {
			t.Errorf("expected ErrCircuitOpen, got %v", err)
		}
	})
}

func TestCircuitBreaker_ExecuteWithContext(t *testing.T) {
	cb := New(DefaultConfig())
	ctx := context.Background()

	err := cb.ExecuteWithContext(ctx, func(ctx context.Context) error {
		return nil
	})
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestCircuitBreaker_Reset(t *testing.T) {
	config := &Config{
		MaxFailures:         1,
		Timeout:             1 * time.Second,
		MaxHalfOpenRequests: 1,
	}
	cb := New(config)

	cb.RecordFailure()
	if cb.State() != StateOpen {
		t.Fatal("expected StateOpen")
	}

	cb.Reset()
	if cb.State() != StateClosed {
		t.Errorf("expected StateClosed after reset, got %v", cb.State())
	}
}

func TestCircuitBreaker_Stats(t *testing.T) {
	config := &Config{
		MaxFailures:         3,
		Timeout:             1 * time.Second,
		MaxHalfOpenRequests: 1,
	}
	cb := New(config)

	cb.RecordFailure()
	cb.RecordFailure()

	stats := cb.Stats()
	if stats.State != StateClosed {
		t.Errorf("expected StateClosed, got %v", stats.State)
	}
	if stats.Failures != 2 {
		t.Errorf("expected 2 failures, got %d", stats.Failures)
	}
}

func TestCircuitBreaker_OnStateChange(t *testing.T) {
	config := &Config{
		MaxFailures:         1,
		Timeout:             50 * time.Millisecond,
		MaxHalfOpenRequests: 1,
	}
	cb := New(config)

	var transitions []struct{ from, to State }
	cb.SetOnStateChange(func(from, to State) {
		transitions = append(transitions, struct{ from, to State }{from, to})
	})

	cb.RecordFailure() // closed -> open
	time.Sleep(60 * time.Millisecond)
	cb.Allow()         // triggers open -> half-open check
	cb.RecordSuccess() // half-open -> closed

	if len(transitions) != 3 {
		t.Fatalf("expected 3 transitions, got %d", len(transitions))
	}

	if transitions[0].from != StateClosed || transitions[0].to != StateOpen {
		t.Errorf("expected closed->open, got %v->%v", transitions[0].from, transitions[0].to)
	}
	if transitions[1].from != StateOpen || transitions[1].to != StateHalfOpen {
		t.Errorf("expected open->half-open, got %v->%v", transitions[1].from, transitions[1].to)
	}
	if transitions[2].from != StateHalfOpen || transitions[2].to != StateClosed {
		t.Errorf("expected half-open->closed, got %v->%v", transitions[2].from, transitions[2].to)
	}
}

func TestState_String(t *testing.T) {
	tests := []struct {
		state    State
		expected string
	}{
		{StateClosed, "closed"},
		{StateOpen, "open"},
		{StateHalfOpen, "half-open"},
		{State(99), "unknown"},
	}

	for _, tt := range tests {
		if tt.state.String() != tt.expected {
			t.Errorf("expected %q, got %q", tt.expected, tt.state.String())
		}
	}
}
