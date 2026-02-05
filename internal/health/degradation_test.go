package health

import (
	"testing"
	"time"
)

func TestDegradationMode_String(t *testing.T) {
	tests := []struct {
		mode     DegradationMode
		expected string
	}{
		{ModeNormal, "normal"},
		{ModeDegraded, "degraded"},
		{ModeRecovering, "recovering"},
		{ModeFailed, "failed"},
		{DegradationMode(99), "unknown"},
	}

	for _, tt := range tests {
		if tt.mode.String() != tt.expected {
			t.Errorf("expected %q, got %q", tt.expected, tt.mode.String())
		}
	}
}

func TestGracefulDegradation_DefaultConfig(t *testing.T) {
	cfg := DefaultDegradationConfig()
	if cfg.QueueSize != 1000 {
		t.Errorf("expected QueueSize 1000, got %d", cfg.QueueSize)
	}
	if cfg.QueueTimeout != 30*time.Second {
		t.Errorf("expected QueueTimeout 30s, got %v", cfg.QueueTimeout)
	}
	if cfg.RecoveryTimeout != 5*time.Minute {
		t.Errorf("expected RecoveryTimeout 5m, got %v", cfg.RecoveryTimeout)
	}
	if !cfg.FallbackEnabled {
		t.Error("expected FallbackEnabled to be true")
	}
}

func TestGracefulDegradation_New(t *testing.T) {
	gd := NewGracefulDegradation(nil)
	if gd == nil {
		t.Fatal("expected non-nil GracefulDegradation")
	}
	if gd.Mode() != ModeNormal {
		t.Errorf("expected ModeNormal, got %v", gd.Mode())
	}
}

func TestGracefulDegradation_ModeTransitions(t *testing.T) {
	gd := NewGracefulDegradation(DefaultDegradationConfig())

	// Normal -> Degraded
	gd.EnterDegradedMode()
	if gd.Mode() != ModeDegraded {
		t.Errorf("expected ModeDegraded, got %v", gd.Mode())
	}

	// Degraded -> Recovering
	gd.BeginRecovery()
	if gd.Mode() != ModeRecovering {
		t.Errorf("expected ModeRecovering, got %v", gd.Mode())
	}

	// Recovering -> Normal
	gd.RecoveryComplete()
	if gd.Mode() != ModeNormal {
		t.Errorf("expected ModeNormal, got %v", gd.Mode())
	}

	// Can also go to Failed
	gd.MarkFailed()
	if gd.Mode() != ModeFailed {
		t.Errorf("expected ModeFailed, got %v", gd.Mode())
	}
}

func TestGracefulDegradation_OnModeChange(t *testing.T) {
	gd := NewGracefulDegradation(DefaultDegradationConfig())

	var transitions []struct{ old, new DegradationMode }
	gd.SetOnModeChange(func(old, new DegradationMode) {
		transitions = append(transitions, struct{ old, new DegradationMode }{old, new})
	})

	gd.EnterDegradedMode()
	gd.BeginRecovery()
	gd.RecoveryComplete()

	if len(transitions) != 3 {
		t.Fatalf("expected 3 transitions, got %d", len(transitions))
	}

	if transitions[0].old != ModeNormal || transitions[0].new != ModeDegraded {
		t.Error("unexpected first transition")
	}
	if transitions[1].old != ModeDegraded || transitions[1].new != ModeRecovering {
		t.Error("unexpected second transition")
	}
	if transitions[2].old != ModeRecovering || transitions[2].new != ModeNormal {
		t.Error("unexpected third transition")
	}
}

func TestGracefulDegradation_QueuePacket(t *testing.T) {
	config := &DegradationConfig{
		QueueSize:       10,
		QueueTimeout:    30 * time.Second,
		RecoveryTimeout: 5 * time.Minute,
		FallbackEnabled: true,
	}
	gd := NewGracefulDegradation(config)

	// Should not queue in normal mode
	queued := gd.QueuePacket(1, []byte("test"))
	if queued {
		t.Error("should not queue in normal mode")
	}

	// Enter degraded mode
	gd.EnterDegradedMode()

	// Should queue in degraded mode
	queued = gd.QueuePacket(1, []byte("test data"))
	if !queued {
		t.Error("should queue in degraded mode")
	}

	if gd.QueuedCount() != 1 {
		t.Errorf("expected 1 queued packet, got %d", gd.QueuedCount())
	}
}

func TestGracefulDegradation_QueueOverflow(t *testing.T) {
	config := &DegradationConfig{
		QueueSize:       3,
		QueueTimeout:    30 * time.Second,
		RecoveryTimeout: 5 * time.Minute,
		FallbackEnabled: true,
	}
	gd := NewGracefulDegradation(config)

	var droppedPackets []QueuedPacket
	gd.SetOnPacketDrop(func(packet QueuedPacket, reason string) {
		droppedPackets = append(droppedPackets, packet)
	})

	gd.EnterDegradedMode()

	// Queue up to capacity
	for i := 0; i < 3; i++ {
		gd.QueuePacket(uint32(i), []byte("packet"))
	}

	if gd.QueuedCount() != 3 {
		t.Errorf("expected 3 queued packets, got %d", gd.QueuedCount())
	}

	// Queue one more - should drop oldest
	gd.QueuePacket(99, []byte("overflow"))

	if gd.QueuedCount() != 3 {
		t.Errorf("expected 3 queued packets after overflow, got %d", gd.QueuedCount())
	}

	if len(droppedPackets) != 1 {
		t.Errorf("expected 1 dropped packet, got %d", len(droppedPackets))
	}
}

func TestGracefulDegradation_DrainQueue(t *testing.T) {
	config := &DegradationConfig{
		QueueSize:       10,
		QueueTimeout:    100 * time.Millisecond, // Short timeout for testing
		RecoveryTimeout: 5 * time.Minute,
		FallbackEnabled: true,
	}
	gd := NewGracefulDegradation(config)

	gd.EnterDegradedMode()

	// Queue some packets
	gd.QueuePacket(1, []byte("packet1"))
	gd.QueuePacket(2, []byte("packet2"))

	// Drain immediately
	packets := gd.DrainQueue()
	if len(packets) != 2 {
		t.Errorf("expected 2 packets, got %d", len(packets))
	}

	if gd.QueuedCount() != 0 {
		t.Errorf("expected 0 queued after drain, got %d", gd.QueuedCount())
	}
}

func TestGracefulDegradation_DrainQueueWithTimeout(t *testing.T) {
	config := &DegradationConfig{
		QueueSize:       10,
		QueueTimeout:    50 * time.Millisecond,
		RecoveryTimeout: 5 * time.Minute,
		FallbackEnabled: true,
	}
	gd := NewGracefulDegradation(config)

	var droppedPackets []QueuedPacket
	gd.SetOnPacketDrop(func(packet QueuedPacket, reason string) {
		droppedPackets = append(droppedPackets, packet)
	})

	gd.EnterDegradedMode()

	// Queue some packets
	gd.QueuePacket(1, []byte("old"))

	// Wait for timeout
	time.Sleep(100 * time.Millisecond)

	gd.QueuePacket(2, []byte("new"))

	// Drain - old packet should be dropped
	packets := gd.DrainQueue()
	if len(packets) != 1 {
		t.Errorf("expected 1 packet (new one), got %d", len(packets))
	}

	if len(droppedPackets) != 1 {
		t.Errorf("expected 1 dropped packet, got %d", len(droppedPackets))
	}
}

func TestGracefulDegradation_RecoveryAttempts(t *testing.T) {
	gd := NewGracefulDegradation(DefaultDegradationConfig())

	if gd.RecoveryAttempts() != 0 {
		t.Errorf("expected 0 recovery attempts, got %d", gd.RecoveryAttempts())
	}

	gd.EnterDegradedMode()
	gd.BeginRecovery()

	if gd.RecoveryAttempts() != 1 {
		t.Errorf("expected 1 recovery attempt, got %d", gd.RecoveryAttempts())
	}

	gd.SetMode(ModeDegraded)
	gd.BeginRecovery()

	if gd.RecoveryAttempts() != 2 {
		t.Errorf("expected 2 recovery attempts, got %d", gd.RecoveryAttempts())
	}

	// Successful recovery should reset count
	gd.RecoveryComplete()
	if gd.RecoveryAttempts() != 0 {
		t.Errorf("expected 0 recovery attempts after success, got %d", gd.RecoveryAttempts())
	}
}

func TestGracefulDegradation_IsRecoveryTimedOut(t *testing.T) {
	config := &DegradationConfig{
		QueueSize:       10,
		QueueTimeout:    30 * time.Second,
		RecoveryTimeout: 50 * time.Millisecond, // Short for testing
		FallbackEnabled: true,
	}
	gd := NewGracefulDegradation(config)

	// Should not be timed out initially
	if gd.IsRecoveryTimedOut() {
		t.Error("should not be timed out initially")
	}

	gd.BeginRecovery()

	// Wait for timeout
	time.Sleep(70 * time.Millisecond)

	if !gd.IsRecoveryTimedOut() {
		t.Error("should be timed out after waiting")
	}
}

func TestGracefulDegradation_Stats(t *testing.T) {
	gd := NewGracefulDegradation(DefaultDegradationConfig())

	gd.EnterDegradedMode()
	gd.QueuePacket(1, []byte("test"))
	gd.BeginRecovery()

	stats := gd.Stats()

	if stats.Mode != ModeRecovering {
		t.Errorf("expected ModeRecovering, got %v", stats.Mode)
	}
	if stats.QueuedPackets != 1 {
		t.Errorf("expected 1 queued packet, got %d", stats.QueuedPackets)
	}
	if stats.RecoveryAttempts != 1 {
		t.Errorf("expected 1 recovery attempt, got %d", stats.RecoveryAttempts)
	}
}

func TestGracefulDegradation_ShouldFallback(t *testing.T) {
	gd := NewGracefulDegradation(DefaultDegradationConfig())

	// Should not fallback in normal mode
	if gd.ShouldFallback() {
		t.Error("should not fallback in normal mode")
	}

	// Should fallback in degraded mode
	gd.EnterDegradedMode()
	if !gd.ShouldFallback() {
		t.Error("should fallback in degraded mode")
	}

	// Should fallback in recovering mode
	gd.BeginRecovery()
	if !gd.ShouldFallback() {
		t.Error("should fallback in recovering mode")
	}
}

func TestGracefulDegradation_ShouldFallbackDisabled(t *testing.T) {
	config := &DegradationConfig{
		QueueSize:       10,
		QueueTimeout:    30 * time.Second,
		RecoveryTimeout: 5 * time.Minute,
		FallbackEnabled: false,
	}
	gd := NewGracefulDegradation(config)

	gd.EnterDegradedMode()
	if gd.ShouldFallback() {
		t.Error("should not fallback when disabled")
	}
}

func TestGracefulDegradation_Reset(t *testing.T) {
	gd := NewGracefulDegradation(DefaultDegradationConfig())

	gd.EnterDegradedMode()
	gd.QueuePacket(1, []byte("test"))
	gd.BeginRecovery()

	if gd.QueuedCount() != 1 {
		t.Errorf("expected 1 queued packet before reset, got %d", gd.QueuedCount())
	}

	gd.Reset()

	if gd.Mode() != ModeNormal {
		t.Errorf("expected ModeNormal after reset, got %v", gd.Mode())
	}
	if gd.QueuedCount() != 0 {
		t.Errorf("expected 0 queued packets after reset, got %d", gd.QueuedCount())
	}
	if gd.RecoveryAttempts() != 0 {
		t.Errorf("expected 0 recovery attempts after reset, got %d", gd.RecoveryAttempts())
	}
}
