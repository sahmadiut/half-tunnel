package errors

import (
	"errors"
	"testing"
)

func TestTunnelError(t *testing.T) {
	t.Run("Error returns formatted message", func(t *testing.T) {
		err := NewTunnelError("connect", ErrUpstreamUnavailable, errors.New("dial timeout"), "server unreachable")
		expected := "connect: upstream connection unavailable: dial timeout (server unreachable)"
		if err.Error() != expected {
			t.Errorf("expected %q, got %q", expected, err.Error())
		}
	})

	t.Run("Error without details", func(t *testing.T) {
		err := NewTunnelError("connect", ErrConnectionClosed, errors.New("EOF"), "")
		expected := "connect: connection closed: EOF"
		if err.Error() != expected {
			t.Errorf("expected %q, got %q", expected, err.Error())
		}
	})

	t.Run("Error without underlying error", func(t *testing.T) {
		err := NewTunnelError("connect", ErrCircuitOpen, nil, "")
		expected := "connect: circuit breaker is open"
		if err.Error() != expected {
			t.Errorf("expected %q, got %q", expected, err.Error())
		}
	})

	t.Run("Unwrap returns underlying error", func(t *testing.T) {
		underlying := errors.New("underlying error")
		err := NewTunnelError("test", ErrConnectionClosed, underlying, "")
		if !errors.Is(err.Unwrap(), underlying) {
			t.Error("Unwrap should return underlying error")
		}
	})

	t.Run("Is matches kind error", func(t *testing.T) {
		err := NewTunnelError("test", ErrUpstreamUnavailable, errors.New("timeout"), "")
		if !errors.Is(err, ErrUpstreamUnavailable) {
			t.Error("errors.Is should match the kind error")
		}
	})
}

func TestWrap(t *testing.T) {
	underlying := errors.New("network error")
	err := Wrap("dial", ErrConnectionTimeout, underlying)

	if err.Op != "dial" {
		t.Errorf("expected Op 'dial', got %q", err.Op)
	}
	if !errors.Is(err, ErrConnectionTimeout) {
		t.Error("wrapped error should match ErrConnectionTimeout")
	}
	if !errors.Is(err, underlying) {
		t.Error("wrapped error should contain underlying error")
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		err       error
		retryable bool
	}{
		{nil, false},
		{ErrUpstreamUnavailable, true},
		{ErrDownstreamUnavailable, true},
		{ErrConnectionClosed, true},
		{ErrConnectionTimeout, true},
		{ErrHandshakeFailed, true},
		{ErrReconnectFailed, true},
		{ErrMaxSessionsReached, false},
		{ErrMaxStreamsReached, false},
		{ErrMaxRetries, false},
		{ErrSessionNotFound, false},
		{ErrInvalidPacket, false},
		{ErrProtocolVersion, false},
		{Wrap("test", ErrUpstreamUnavailable, nil), true},
		{Wrap("test", ErrMaxRetries, nil), false},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := IsRetryable(tt.err)
			if got != tt.retryable {
				t.Errorf("IsRetryable(%v) = %v, want %v", tt.err, got, tt.retryable)
			}
		})
	}
}

func TestIsTransient(t *testing.T) {
	tests := []struct {
		err       error
		transient bool
	}{
		{nil, false},
		{ErrCircuitOpen, true},
		{ErrCircuitTimeout, true},
		{ErrConnectionTimeout, true},
		{ErrSessionExpired, false},
		{ErrStreamClosed, false},
		{Wrap("test", ErrCircuitOpen, nil), true},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := IsTransient(tt.err)
			if got != tt.transient {
				t.Errorf("IsTransient(%v) = %v, want %v", tt.err, got, tt.transient)
			}
		})
	}
}
