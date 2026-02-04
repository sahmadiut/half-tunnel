package health

import (
	"testing"
	"time"
)

func TestNewConnectionMonitor(t *testing.T) {
	m := NewConnectionMonitor(nil)
	if m == nil {
		t.Fatal("expected non-nil monitor")
	}
	if !m.IsAlive() {
		t.Error("new monitor should be alive")
	}
}

func TestConnectionMonitor_RecordPong(t *testing.T) {
	m := NewConnectionMonitor(nil)

	m.RecordPing()
	time.Sleep(10 * time.Millisecond)
	m.RecordPong()

	health := m.GetHealth()
	if !health.IsAlive {
		t.Error("should be alive after pong")
	}
	if health.Latency < 10*time.Millisecond {
		t.Errorf("latency should be at least 10ms, got %v", health.Latency)
	}
}

func TestConnectionMonitor_RecordFailure(t *testing.T) {
	config := &ConnectionMonitorConfig{
		PingTimeout:  30 * time.Second,
		FailureLimit: 2,
	}
	m := NewConnectionMonitor(config)

	m.RecordFailure()
	if !m.IsAlive() {
		t.Error("should still be alive after 1 failure")
	}

	m.RecordFailure()
	if m.IsAlive() {
		t.Error("should not be alive after 2 failures")
	}
}

func TestConnectionMonitor_RecoveryAfterPong(t *testing.T) {
	config := &ConnectionMonitorConfig{
		PingTimeout:  30 * time.Second,
		FailureLimit: 1,
	}
	m := NewConnectionMonitor(config)

	// Record failure to make it unhealthy
	m.RecordFailure()
	if m.IsAlive() {
		t.Error("should not be alive after failure")
	}

	// Pong should recover
	m.RecordPong()
	if !m.IsAlive() {
		t.Error("should be alive after pong")
	}

	health := m.GetHealth()
	if health.FailureCount != 0 {
		t.Errorf("failure count should be 0 after pong, got %d", health.FailureCount)
	}
}

func TestConnectionMonitor_Reset(t *testing.T) {
	config := &ConnectionMonitorConfig{
		PingTimeout:  30 * time.Second,
		FailureLimit: 1,
	}
	m := NewConnectionMonitor(config)

	m.RecordFailure()
	m.RecordPing()

	m.Reset()

	health := m.GetHealth()
	if !health.IsAlive {
		t.Error("should be alive after reset")
	}
	if health.FailureCount != 0 {
		t.Errorf("failure count should be 0, got %d", health.FailureCount)
	}
	if !health.LastPingTime.IsZero() {
		t.Error("last ping time should be zero after reset")
	}
}

func TestConnectionMonitor_CheckTimeout(t *testing.T) {
	config := &ConnectionMonitorConfig{
		PingTimeout:  50 * time.Millisecond,
		FailureLimit: 3,
	}
	m := NewConnectionMonitor(config)

	// No ping sent yet - no timeout
	if m.CheckTimeout() {
		t.Error("should not timeout when no ping sent")
	}

	// Send ping
	m.RecordPing()

	// Not timed out immediately
	if m.CheckTimeout() {
		t.Error("should not timeout immediately after ping")
	}

	// Wait for timeout
	time.Sleep(60 * time.Millisecond)
	if !m.CheckTimeout() {
		t.Error("should timeout after waiting")
	}

	// Pong clears timeout
	m.RecordPong()
	if m.CheckTimeout() {
		t.Error("should not timeout after pong")
	}
}

func TestConnectionMonitor_StatusChangeCallback(t *testing.T) {
	config := &ConnectionMonitorConfig{
		PingTimeout:  30 * time.Second,
		FailureLimit: 1,
	}
	m := NewConnectionMonitor(config)

	var statusChanges []bool
	m.SetStatusChangeCallback(func(healthy bool) {
		statusChanges = append(statusChanges, healthy)
	})

	// Trigger unhealthy
	m.RecordFailure()
	if len(statusChanges) != 1 || statusChanges[0] != false {
		t.Error("expected status change to unhealthy")
	}

	// Trigger healthy
	m.RecordPong()
	if len(statusChanges) != 2 || statusChanges[1] != true {
		t.Error("expected status change to healthy")
	}
}

func TestDefaultConnectionMonitorConfig(t *testing.T) {
	config := DefaultConnectionMonitorConfig()
	if config.PingTimeout != 30*time.Second {
		t.Errorf("expected PingTimeout 30s, got %v", config.PingTimeout)
	}
	if config.FailureLimit != 3 {
		t.Errorf("expected FailureLimit 3, got %d", config.FailureLimit)
	}
}
