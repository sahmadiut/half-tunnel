// Package health provides health check and monitoring for the Half-Tunnel system.
package health

import (
	"context"
	"sync"
	"sync/atomic"
	"time"
)

// DegradationMode represents the current operational mode of the tunnel.
type DegradationMode int32

const (
	// ModeNormal indicates the tunnel is operating normally.
	ModeNormal DegradationMode = iota
	// ModeDegraded indicates the tunnel is operating in degraded mode (some features unavailable).
	ModeDegraded
	// ModeRecovering indicates the tunnel is recovering from a failure.
	ModeRecovering
	// ModeFailed indicates the tunnel has failed and cannot recover automatically.
	ModeFailed
)

// String returns a string representation of the degradation mode.
func (m DegradationMode) String() string {
	switch m {
	case ModeNormal:
		return "normal"
	case ModeDegraded:
		return "degraded"
	case ModeRecovering:
		return "recovering"
	case ModeFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// DegradationConfig holds configuration for graceful degradation.
type DegradationConfig struct {
	// QueueSize is the maximum number of packets to queue during reconnection.
	QueueSize int
	// QueueTimeout is how long to wait before dropping queued packets.
	QueueTimeout time.Duration
	// RecoveryTimeout is the maximum time to spend in recovery mode.
	RecoveryTimeout time.Duration
	// FallbackEnabled indicates whether to fall back to single-path mode.
	FallbackEnabled bool
}

// DefaultDegradationConfig returns a config with sensible defaults.
func DefaultDegradationConfig() *DegradationConfig {
	return &DegradationConfig{
		QueueSize:       1000,
		QueueTimeout:    30 * time.Second,
		RecoveryTimeout: 5 * time.Minute,
		FallbackEnabled: true,
	}
}

// QueuedPacket represents a packet waiting to be sent during reconnection.
type QueuedPacket struct {
	Data      []byte
	Timestamp time.Time
	StreamID  uint32
}

// GracefulDegradation manages graceful degradation and recovery.
type GracefulDegradation struct {
	config *DegradationConfig

	// Current mode
	mode int32 // atomic, stores DegradationMode

	// Packet queue for buffering during reconnection
	queue      []QueuedPacket
	queueMu    sync.Mutex
	queueCond  *sync.Cond
	queueCount int32 // atomic

	// Recovery state
	degradedAt    int64 // atomic: UnixNano when degraded mode started
	recoveringAt  int64 // atomic: UnixNano when recovery started
	recoveryCount int32 // atomic: number of recovery attempts

	// Callbacks
	onModeChange func(old, new DegradationMode)
	onPacketDrop func(packet QueuedPacket, reason string)
}

// NewGracefulDegradation creates a new graceful degradation handler.
func NewGracefulDegradation(config *DegradationConfig) *GracefulDegradation {
	if config == nil {
		config = DefaultDegradationConfig()
	}
	gd := &GracefulDegradation{
		config: config,
		mode:   int32(ModeNormal),
		queue:  make([]QueuedPacket, 0, config.QueueSize),
	}
	gd.queueCond = sync.NewCond(&gd.queueMu)
	return gd
}

// SetOnModeChange sets the callback for mode changes.
func (gd *GracefulDegradation) SetOnModeChange(fn func(old, new DegradationMode)) {
	gd.onModeChange = fn
}

// SetOnPacketDrop sets the callback for dropped packets.
func (gd *GracefulDegradation) SetOnPacketDrop(fn func(packet QueuedPacket, reason string)) {
	gd.onPacketDrop = fn
}

// Mode returns the current degradation mode.
func (gd *GracefulDegradation) Mode() DegradationMode {
	return DegradationMode(atomic.LoadInt32(&gd.mode))
}

// SetMode sets the degradation mode.
func (gd *GracefulDegradation) SetMode(mode DegradationMode) {
	old := DegradationMode(atomic.SwapInt32(&gd.mode, int32(mode)))
	if old != mode {
		now := time.Now().UnixNano()
		switch mode {
		case ModeDegraded:
			atomic.StoreInt64(&gd.degradedAt, now)
		case ModeRecovering:
			atomic.StoreInt64(&gd.recoveringAt, now)
			atomic.AddInt32(&gd.recoveryCount, 1)
		case ModeNormal:
			// Reset recovery count on successful recovery
			atomic.StoreInt32(&gd.recoveryCount, 0)
		}
		if gd.onModeChange != nil {
			gd.onModeChange(old, mode)
		}
	}
}

// EnterDegradedMode transitions to degraded mode.
func (gd *GracefulDegradation) EnterDegradedMode() {
	gd.SetMode(ModeDegraded)
}

// BeginRecovery transitions to recovery mode.
func (gd *GracefulDegradation) BeginRecovery() {
	gd.SetMode(ModeRecovering)
}

// RecoveryComplete transitions back to normal mode.
func (gd *GracefulDegradation) RecoveryComplete() {
	gd.SetMode(ModeNormal)
}

// MarkFailed marks the tunnel as failed.
func (gd *GracefulDegradation) MarkFailed() {
	gd.SetMode(ModeFailed)
}

// QueuePacket adds a packet to the queue during reconnection.
// Returns true if the packet was queued, false if the queue is full or timeout expired.
func (gd *GracefulDegradation) QueuePacket(streamID uint32, data []byte) bool {
	mode := gd.Mode()
	if mode == ModeNormal {
		// No need to queue when in normal mode
		return false
	}

	gd.queueMu.Lock()
	defer gd.queueMu.Unlock()

	// Check queue size
	if len(gd.queue) >= gd.config.QueueSize {
		// Drop oldest packet to make room
		if len(gd.queue) > 0 {
			dropped := gd.queue[0]
			gd.queue = gd.queue[1:]
			atomic.AddInt32(&gd.queueCount, -1)
			if gd.onPacketDrop != nil {
				gd.onPacketDrop(dropped, "queue full")
			}
		}
	}

	// Add packet to queue
	packet := QueuedPacket{
		Data:      make([]byte, len(data)),
		Timestamp: time.Now(),
		StreamID:  streamID,
	}
	copy(packet.Data, data)
	gd.queue = append(gd.queue, packet)
	atomic.AddInt32(&gd.queueCount, 1)
	gd.queueCond.Signal()
	return true
}

// DrainQueue returns all queued packets and clears the queue.
// Packets older than QueueTimeout are dropped.
func (gd *GracefulDegradation) DrainQueue() []QueuedPacket {
	gd.queueMu.Lock()
	defer gd.queueMu.Unlock()

	now := time.Now()
	result := make([]QueuedPacket, 0, len(gd.queue))

	for _, packet := range gd.queue {
		if now.Sub(packet.Timestamp) > gd.config.QueueTimeout {
			if gd.onPacketDrop != nil {
				gd.onPacketDrop(packet, "timeout")
			}
			continue
		}
		result = append(result, packet)
	}

	// Clear the queue
	gd.queue = gd.queue[:0]
	atomic.StoreInt32(&gd.queueCount, 0)

	return result
}

// QueuedCount returns the number of packets in the queue.
func (gd *GracefulDegradation) QueuedCount() int {
	return int(atomic.LoadInt32(&gd.queueCount))
}

// RecoveryAttempts returns the number of recovery attempts.
func (gd *GracefulDegradation) RecoveryAttempts() int {
	return int(atomic.LoadInt32(&gd.recoveryCount))
}

// IsRecoveryTimedOut returns true if the recovery has taken too long.
func (gd *GracefulDegradation) IsRecoveryTimedOut() bool {
	recoveringAt := atomic.LoadInt64(&gd.recoveringAt)
	if recoveringAt == 0 {
		return false
	}
	elapsed := time.Since(time.Unix(0, recoveringAt))
	return elapsed > gd.config.RecoveryTimeout
}

// DegradationStats contains statistics about degradation state.
type DegradationStats struct {
	Mode             DegradationMode
	QueuedPackets    int
	RecoveryAttempts int
	DegradedDuration time.Duration
	RecoveringFor    time.Duration
}

// Stats returns current degradation statistics.
func (gd *GracefulDegradation) Stats() DegradationStats {
	mode := gd.Mode()
	now := time.Now()

	var degradedDuration, recoveringFor time.Duration
	if degradedAt := atomic.LoadInt64(&gd.degradedAt); degradedAt > 0 {
		degradedDuration = now.Sub(time.Unix(0, degradedAt))
	}
	if recoveringAt := atomic.LoadInt64(&gd.recoveringAt); recoveringAt > 0 && (mode == ModeRecovering) {
		recoveringFor = now.Sub(time.Unix(0, recoveringAt))
	}

	return DegradationStats{
		Mode:             mode,
		QueuedPackets:    gd.QueuedCount(),
		RecoveryAttempts: gd.RecoveryAttempts(),
		DegradedDuration: degradedDuration,
		RecoveringFor:    recoveringFor,
	}
}

// ShouldFallback returns true if fallback to single-path mode should be attempted.
func (gd *GracefulDegradation) ShouldFallback() bool {
	if !gd.config.FallbackEnabled {
		return false
	}
	mode := gd.Mode()
	return mode == ModeDegraded || mode == ModeRecovering
}

// WaitForPackets blocks until there are packets in the queue or the context is cancelled.
func (gd *GracefulDegradation) WaitForPackets(ctx context.Context) bool {
	done := make(chan struct{})
	go func() {
		gd.queueMu.Lock()
		for len(gd.queue) == 0 && gd.Mode() != ModeNormal {
			gd.queueCond.Wait()
		}
		gd.queueMu.Unlock()
		close(done)
	}()

	select {
	case <-ctx.Done():
		gd.queueCond.Broadcast() // Wake up the waiting goroutine
		return false
	case <-done:
		return true
	}
}

// Reset resets the degradation state to normal.
func (gd *GracefulDegradation) Reset() {
	gd.queueMu.Lock()
	gd.queue = gd.queue[:0]
	gd.queueMu.Unlock()

	atomic.StoreInt32(&gd.queueCount, 0)
	atomic.StoreInt64(&gd.degradedAt, 0)
	atomic.StoreInt64(&gd.recoveringAt, 0)
	atomic.StoreInt32(&gd.recoveryCount, 0)
	gd.SetMode(ModeNormal)
}
