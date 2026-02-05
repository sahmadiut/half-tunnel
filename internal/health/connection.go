// Package health provides health check and monitoring for the Half-Tunnel system.
package health

import (
	"context"
	"sync"
	"sync/atomic"
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

// ConnectionMonitorConfig holds configuration for connection health monitoring.
type ConnectionMonitorConfig struct {
	// PingInterval is how often to send ping/health checks
	PingInterval time.Duration
	// PongTimeout is how long to wait for a pong response
	PongTimeout time.Duration
	// MaxFailures is the maximum number of consecutive failures before marking unhealthy
	MaxFailures int
}

// DefaultConnectionMonitorConfig returns a config with sensible defaults.
func DefaultConnectionMonitorConfig() *ConnectionMonitorConfig {
	return &ConnectionMonitorConfig{
		PingInterval: 30 * time.Second,
		PongTimeout:  10 * time.Second,
		MaxFailures:  3,
	}
}

// Pingable is an interface for connections that can be pinged.
type Pingable interface {
	// Ping sends a ping and waits for a pong response.
	// Returns the latency if successful, or an error if the ping failed.
	Ping(ctx context.Context) (time.Duration, error)
}

// ConnectionMonitor monitors the health of a connection.
type ConnectionMonitor struct {
	config *ConnectionMonitorConfig

	// Health state
	isAlive      int32 // atomic: 1 = alive, 0 = dead
	lastPingTime int64 // atomic: UnixNano
	lastPongTime int64 // atomic: UnixNano
	latencyNs    int64 // atomic: latency in nanoseconds
	failureCount int32 // atomic

	// Control
	running  int32
	shutdown chan struct{}
	wg       sync.WaitGroup

	// Callback for health changes
	onHealthChange func(health ConnectionHealth)
}

// NewConnectionMonitor creates a new connection monitor.
func NewConnectionMonitor(config *ConnectionMonitorConfig) *ConnectionMonitor {
	if config == nil {
		config = DefaultConnectionMonitorConfig()
	}
	return &ConnectionMonitor{
		config:   config,
		isAlive:  1, // Start assuming connection is alive
		shutdown: make(chan struct{}),
	}
}

// SetOnHealthChange sets the callback for health changes.
func (m *ConnectionMonitor) SetOnHealthChange(fn func(health ConnectionHealth)) {
	m.onHealthChange = fn
}

// GetHealth returns the current health status.
func (m *ConnectionMonitor) GetHealth() ConnectionHealth {
	lastPing := atomic.LoadInt64(&m.lastPingTime)
	lastPong := atomic.LoadInt64(&m.lastPongTime)
	latency := atomic.LoadInt64(&m.latencyNs)

	var lastPingTime, lastPongTime time.Time
	if lastPing > 0 {
		lastPingTime = time.Unix(0, lastPing)
	}
	if lastPong > 0 {
		lastPongTime = time.Unix(0, lastPong)
	}

	return ConnectionHealth{
		IsAlive:      atomic.LoadInt32(&m.isAlive) == 1,
		LastPingTime: lastPingTime,
		LastPongTime: lastPongTime,
		Latency:      time.Duration(latency),
		FailureCount: int(atomic.LoadInt32(&m.failureCount)),
	}
}

// RecordPing records that a ping was sent.
func (m *ConnectionMonitor) RecordPing() {
	atomic.StoreInt64(&m.lastPingTime, time.Now().UnixNano())
}

// RecordPong records that a pong was received with the given latency.
func (m *ConnectionMonitor) RecordPong(latency time.Duration) {
	atomic.StoreInt64(&m.lastPongTime, time.Now().UnixNano())
	atomic.StoreInt64(&m.latencyNs, int64(latency))
	atomic.StoreInt32(&m.failureCount, 0)
	atomic.StoreInt32(&m.isAlive, 1)
}

// RecordFailure records a ping failure.
func (m *ConnectionMonitor) RecordFailure() {
	failures := atomic.AddInt32(&m.failureCount, 1)
	if int(failures) >= m.config.MaxFailures {
		m.markUnhealthy()
	}
}

// markUnhealthy marks the connection as unhealthy and triggers callback.
func (m *ConnectionMonitor) markUnhealthy() {
	if atomic.CompareAndSwapInt32(&m.isAlive, 1, 0) {
		if m.onHealthChange != nil {
			m.onHealthChange(m.GetHealth())
		}
	}
}

// markHealthy marks the connection as healthy and triggers callback if state changed.
func (m *ConnectionMonitor) markHealthy() {
	if atomic.CompareAndSwapInt32(&m.isAlive, 0, 1) {
		if m.onHealthChange != nil {
			m.onHealthChange(m.GetHealth())
		}
	}
}

// Start starts monitoring the connection.
func (m *ConnectionMonitor) Start(ctx context.Context, conn Pingable) <-chan ConnectionHealth {
	if !atomic.CompareAndSwapInt32(&m.running, 0, 1) {
		return nil // Already running
	}

	healthCh := make(chan ConnectionHealth, 1)
	m.wg.Add(1)
	go m.monitorLoop(ctx, conn, healthCh)
	return healthCh
}

// Stop stops monitoring the connection.
func (m *ConnectionMonitor) Stop() {
	if !atomic.CompareAndSwapInt32(&m.running, 1, 0) {
		return
	}
	close(m.shutdown)
	m.wg.Wait()
}

// IsRunning returns whether the monitor is currently running.
func (m *ConnectionMonitor) IsRunning() bool {
	return atomic.LoadInt32(&m.running) == 1
}

// monitorLoop runs the health check loop.
func (m *ConnectionMonitor) monitorLoop(ctx context.Context, conn Pingable, healthCh chan<- ConnectionHealth) {
	defer m.wg.Done()
	defer close(healthCh)

	ticker := time.NewTicker(m.config.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.shutdown:
			return
		case <-ticker.C:
			m.performHealthCheck(ctx, conn, healthCh)
		}
	}
}

// performHealthCheck performs a single health check.
func (m *ConnectionMonitor) performHealthCheck(ctx context.Context, conn Pingable, healthCh chan<- ConnectionHealth) {
	m.RecordPing()

	pingCtx, cancel := context.WithTimeout(ctx, m.config.PongTimeout)
	defer cancel()

	latency, err := conn.Ping(pingCtx)
	if err != nil {
		m.RecordFailure()
	} else {
		m.RecordPong(latency)
		m.markHealthy()
	}

	// Send health status (non-blocking)
	health := m.GetHealth()
	select {
	case healthCh <- health:
	default:
		// Channel full, skip this update
	}
}

// MonitorConnection is a convenience function that creates a monitor and starts it.
// Returns a channel that receives health updates.
func MonitorConnection(conn Pingable, interval time.Duration) <-chan ConnectionHealth {
	config := &ConnectionMonitorConfig{
		PingInterval: interval,
		PongTimeout:  interval / 3,
		MaxFailures:  3,
	}
	monitor := NewConnectionMonitor(config)
	return monitor.Start(context.Background(), conn)
}
