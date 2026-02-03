// Package errors defines custom error types for the Half-Tunnel system.
package errors

import (
	"errors"
	"fmt"
)

// Sentinel errors for the Half-Tunnel system.
var (
	// Session errors
	ErrSessionExpired     = errors.New("session expired")
	ErrSessionNotFound    = errors.New("session not found")
	ErrMaxSessionsReached = errors.New("maximum sessions reached")

	// Stream errors
	ErrStreamClosed      = errors.New("stream is closed")
	ErrStreamNotFound    = errors.New("stream not found")
	ErrMaxStreamsReached = errors.New("maximum streams per session reached")

	// Transport errors
	ErrUpstreamUnavailable   = errors.New("upstream connection unavailable")
	ErrDownstreamUnavailable = errors.New("downstream connection unavailable")
	ErrConnectionClosed      = errors.New("connection closed")
	ErrConnectionTimeout     = errors.New("connection timeout")
	ErrHandshakeFailed       = errors.New("handshake failed")

	// Reconnection errors
	ErrReconnectFailed = errors.New("reconnection failed")
	ErrMaxRetries      = errors.New("maximum retry attempts exceeded")

	// Circuit breaker errors
	ErrCircuitOpen    = errors.New("circuit breaker is open")
	ErrCircuitTimeout = errors.New("circuit breaker timeout")

	// Protocol errors
	ErrInvalidPacket   = errors.New("invalid packet")
	ErrProtocolVersion = errors.New("unsupported protocol version")
)

// TunnelError represents an error with additional context.
type TunnelError struct {
	Op      string // Operation that failed
	Kind    error  // Category of error
	Err     error  // Underlying error
	Details string // Additional details
}

// Error returns the error message.
func (e *TunnelError) Error() string {
	if e.Details != "" {
		return fmt.Sprintf("%s: %s: %v (%s)", e.Op, e.Kind, e.Err, e.Details)
	}
	if e.Err != nil {
		return fmt.Sprintf("%s: %s: %v", e.Op, e.Kind, e.Err)
	}
	return fmt.Sprintf("%s: %s", e.Op, e.Kind)
}

// Unwrap returns the underlying error.
func (e *TunnelError) Unwrap() error {
	return e.Err
}

// Is reports whether the error matches the target error.
func (e *TunnelError) Is(target error) bool {
	return errors.Is(e.Kind, target)
}

// NewTunnelError creates a new TunnelError.
func NewTunnelError(op string, kind error, err error, details string) *TunnelError {
	return &TunnelError{
		Op:      op,
		Kind:    kind,
		Err:     err,
		Details: details,
	}
}

// Wrap wraps an error with operation context.
func Wrap(op string, kind error, err error) *TunnelError {
	return &TunnelError{
		Op:   op,
		Kind: kind,
		Err:  err,
	}
}

// IsRetryable returns true if the error is retryable.
func IsRetryable(err error) bool {
	if err == nil {
		return false
	}

	// These errors are not retryable
	if errors.Is(err, ErrMaxSessionsReached) ||
		errors.Is(err, ErrMaxStreamsReached) ||
		errors.Is(err, ErrMaxRetries) ||
		errors.Is(err, ErrSessionNotFound) ||
		errors.Is(err, ErrInvalidPacket) ||
		errors.Is(err, ErrProtocolVersion) {
		return false
	}

	// Connection and transport errors are typically retryable
	if errors.Is(err, ErrUpstreamUnavailable) ||
		errors.Is(err, ErrDownstreamUnavailable) ||
		errors.Is(err, ErrConnectionClosed) ||
		errors.Is(err, ErrConnectionTimeout) ||
		errors.Is(err, ErrHandshakeFailed) ||
		errors.Is(err, ErrReconnectFailed) {
		return true
	}

	return false
}

// IsTransient returns true if the error is transient and might resolve on retry.
func IsTransient(err error) bool {
	if err == nil {
		return false
	}

	if errors.Is(err, ErrCircuitOpen) ||
		errors.Is(err, ErrCircuitTimeout) ||
		errors.Is(err, ErrConnectionTimeout) {
		return true
	}

	return false
}
