package health

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

// MockPingable is a mock implementation of Pingable for testing
type MockPingable struct {
	latency time.Duration
	err     error
	pingCount int32
}

func (m *MockPingable) Ping(ctx context.Context) (time.Duration, error) {
	atomic.AddInt32(&m.pingCount, 1)
	if m.err != nil {
		return 0, m.err
	}
	return m.latency, nil
}

func (m *MockPingable) GetPingCount() int {
	return int(atomic.LoadInt32(&m.pingCount))
}

func TestConnectionMonitor_DefaultConfig(t *testing.T) {
	cfg := DefaultConnectionMonitorConfig()
	if cfg.PingInterval != 30*time.Second {
		t.Errorf("expected PingInterval 30s, got %v", cfg.PingInterval)
	}
	if cfg.PongTimeout != 10*time.Second {
		t.Errorf("expected PongTimeout 10s, got %v", cfg.PongTimeout)
	}
	if cfg.MaxFailures != 3 {
		t.Errorf("expected MaxFailures 3, got %d", cfg.MaxFailures)
	}
}

func TestConnectionMonitor_New(t *testing.T) {
	m := NewConnectionMonitor(nil)
	if m == nil {
		t.Fatal("expected non-nil monitor")
	}
	
	health := m.GetHealth()
	if !health.IsAlive {
		t.Error("expected initial state to be alive")
	}
}

func TestConnectionMonitor_GetHealth(t *testing.T) {
	m := NewConnectionMonitor(DefaultConnectionMonitorConfig())
	
	health := m.GetHealth()
	if !health.IsAlive {
		t.Error("expected IsAlive to be true initially")
	}
	if health.FailureCount != 0 {
		t.Errorf("expected FailureCount 0, got %d", health.FailureCount)
	}
}

func TestConnectionMonitor_RecordPingPong(t *testing.T) {
	m := NewConnectionMonitor(DefaultConnectionMonitorConfig())
	
	m.RecordPing()
	health := m.GetHealth()
	if health.LastPingTime.IsZero() {
		t.Error("expected LastPingTime to be set")
	}
	
	m.RecordPong(50 * time.Millisecond)
	health = m.GetHealth()
	if health.LastPongTime.IsZero() {
		t.Error("expected LastPongTime to be set")
	}
	if health.Latency != 50*time.Millisecond {
		t.Errorf("expected Latency 50ms, got %v", health.Latency)
	}
}

func TestConnectionMonitor_RecordFailure(t *testing.T) {
	config := &ConnectionMonitorConfig{
		PingInterval: 1 * time.Second,
		PongTimeout:  100 * time.Millisecond,
		MaxFailures:  3,
	}
	m := NewConnectionMonitor(config)
	
	// Record failures
	m.RecordFailure()
	m.RecordFailure()
	
	health := m.GetHealth()
	if health.FailureCount != 2 {
		t.Errorf("expected FailureCount 2, got %d", health.FailureCount)
	}
	if !health.IsAlive {
		t.Error("expected to still be alive after 2 failures")
	}
	
	// Third failure should mark unhealthy
	m.RecordFailure()
	health = m.GetHealth()
	if health.IsAlive {
		t.Error("expected to be marked unhealthy after 3 failures")
	}
}

func TestConnectionMonitor_RecoveryAfterFailure(t *testing.T) {
	config := &ConnectionMonitorConfig{
		PingInterval: 1 * time.Second,
		PongTimeout:  100 * time.Millisecond,
		MaxFailures:  2,
	}
	m := NewConnectionMonitor(config)
	
	// Mark as unhealthy
	m.RecordFailure()
	m.RecordFailure()
	if m.GetHealth().IsAlive {
		t.Error("expected to be unhealthy")
	}
	
	// Successful pong should reset failure count
	m.RecordPong(10 * time.Millisecond)
	health := m.GetHealth()
	if !health.IsAlive {
		t.Error("expected to be alive after successful pong")
	}
	if health.FailureCount != 0 {
		t.Errorf("expected FailureCount 0, got %d", health.FailureCount)
	}
}

func TestConnectionMonitor_OnHealthChange(t *testing.T) {
	config := &ConnectionMonitorConfig{
		PingInterval: 1 * time.Second,
		PongTimeout:  100 * time.Millisecond,
		MaxFailures:  1,
	}
	m := NewConnectionMonitor(config)
	
	var callbackCalled bool
	var receivedHealth ConnectionHealth
	m.SetOnHealthChange(func(health ConnectionHealth) {
		callbackCalled = true
		receivedHealth = health
	})
	
	// Trigger unhealthy state
	m.RecordFailure()
	
	if !callbackCalled {
		t.Error("expected callback to be called")
	}
	if receivedHealth.IsAlive {
		t.Error("expected health to show unhealthy")
	}
}

func TestConnectionMonitor_StartStop(t *testing.T) {
	config := &ConnectionMonitorConfig{
		PingInterval: 50 * time.Millisecond,
		PongTimeout:  20 * time.Millisecond,
		MaxFailures:  3,
	}
	m := NewConnectionMonitor(config)
	
	mockConn := &MockPingable{latency: 5 * time.Millisecond}
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	healthCh := m.Start(ctx, mockConn)
	if healthCh == nil {
		t.Fatal("expected non-nil health channel")
	}
	
	if !m.IsRunning() {
		t.Error("expected monitor to be running")
	}
	
	// Wait for at least one health check
	time.Sleep(70 * time.Millisecond)
	
	m.Stop()
	
	if m.IsRunning() {
		t.Error("expected monitor to be stopped")
	}
	
	if mockConn.GetPingCount() == 0 {
		t.Error("expected at least one ping")
	}
}

func TestConnectionMonitor_PingError(t *testing.T) {
	config := &ConnectionMonitorConfig{
		PingInterval: 30 * time.Millisecond,
		PongTimeout:  20 * time.Millisecond,
		MaxFailures:  2,
	}
	m := NewConnectionMonitor(config)
	
	mockConn := &MockPingable{err: errors.New("connection failed")}
	
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	
	m.Start(ctx, mockConn)
	
	// Wait for enough failures
	time.Sleep(100 * time.Millisecond)
	
	health := m.GetHealth()
	if health.IsAlive {
		t.Error("expected to be unhealthy after ping failures")
	}
	
	m.Stop()
}

func TestMonitorConnection_ConvenienceFunction(t *testing.T) {
	mockConn := &MockPingable{latency: 5 * time.Millisecond}
	
	healthCh := MonitorConnection(mockConn, 50*time.Millisecond)
	if healthCh == nil {
		t.Fatal("expected non-nil health channel")
	}
	
	// Wait a bit and cancel
	time.Sleep(70 * time.Millisecond)
}
