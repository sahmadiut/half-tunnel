// Package health provides health check functionality for the Half-Tunnel system.
// This file contains connection health monitoring utilities.
package health

import (
	"sync"
	"time"
)

// ConnectionHealth represents the health status of a connection.
type ConnectionHealth struct {
	IsAlive      bool
	LastPingTime time.Time
	LastPongTime time.Time
	Latency      time.Duration
	FailureCount int
}

// ConnectionMonitor monitors the health of a connection.
type ConnectionMonitor struct {
	mu sync.RWMutex

	// Health state
	isAlive      bool
	lastPingTime time.Time
	lastPongTime time.Time
	latency      time.Duration
	failureCount int

	// Configuration
	pingTimeout    time.Duration
	failureLimit   int
	onStatusChange func(healthy bool)
}

// ConnectionMonitorConfig holds configuration for the connection monitor.
type ConnectionMonitorConfig struct {
	// PingTimeout is how long to wait for a pong response
	PingTimeout time.Duration
	// FailureLimit is how many consecutive failures before marking unhealthy
	FailureLimit int
}

// DefaultConnectionMonitorConfig returns default connection monitor configuration.
func DefaultConnectionMonitorConfig() *ConnectionMonitorConfig {
	return &ConnectionMonitorConfig{
		PingTimeout:  30 * time.Second,
		FailureLimit: 3,
	}
}

// NewConnectionMonitor creates a new connection health monitor.
func NewConnectionMonitor(config *ConnectionMonitorConfig) *ConnectionMonitor {
	if config == nil {
		config = DefaultConnectionMonitorConfig()
	}

	return &ConnectionMonitor{
		isAlive:      true,
		pingTimeout:  config.PingTimeout,
		failureLimit: config.FailureLimit,
	}
}

// SetStatusChangeCallback sets the callback for status changes.
func (m *ConnectionMonitor) SetStatusChangeCallback(fn func(healthy bool)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onStatusChange = fn
}

// RecordPing records that a ping was sent.
func (m *ConnectionMonitor) RecordPing() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lastPingTime = time.Now()
}

// RecordPong records that a pong was received.
func (m *ConnectionMonitor) RecordPong() {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	wasAlive := m.isAlive

	m.lastPongTime = now
	if !m.lastPingTime.IsZero() {
		m.latency = now.Sub(m.lastPingTime)
	}
	m.failureCount = 0
	m.isAlive = true

	if !wasAlive && m.onStatusChange != nil {
		m.onStatusChange(true)
	}
}

// RecordFailure records a connection failure.
func (m *ConnectionMonitor) RecordFailure() {
	m.mu.Lock()
	defer m.mu.Unlock()

	wasAlive := m.isAlive
	m.failureCount++

	if m.failureCount >= m.failureLimit {
		m.isAlive = false
		if wasAlive && m.onStatusChange != nil {
			m.onStatusChange(false)
		}
	}
}

// GetHealth returns the current connection health status.
func (m *ConnectionMonitor) GetHealth() ConnectionHealth {
	m.mu.RLock()
	defer m.mu.RUnlock()

	return ConnectionHealth{
		IsAlive:      m.isAlive,
		LastPingTime: m.lastPingTime,
		LastPongTime: m.lastPongTime,
		Latency:      m.latency,
		FailureCount: m.failureCount,
	}
}

// IsAlive returns whether the connection is considered alive.
func (m *ConnectionMonitor) IsAlive() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.isAlive
}

// Reset resets the monitor state.
func (m *ConnectionMonitor) Reset() {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.isAlive = true
	m.lastPingTime = time.Time{}
	m.lastPongTime = time.Time{}
	m.latency = 0
	m.failureCount = 0
}

// CheckTimeout checks if the connection has timed out based on ping/pong.
// Returns true if the connection appears to have timed out.
func (m *ConnectionMonitor) CheckTimeout() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()

	// If we haven't sent a ping yet, no timeout
	if m.lastPingTime.IsZero() {
		return false
	}

	// If we got a pong after the last ping, no timeout
	if m.lastPongTime.After(m.lastPingTime) {
		return false
	}

	// Check if we've exceeded the ping timeout
	return time.Since(m.lastPingTime) > m.pingTimeout
}
