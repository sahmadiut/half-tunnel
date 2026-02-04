// Package client provides the Half-Tunnel entry client implementation.
package client

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"github.com/sahmadiut/half-tunnel/pkg/logger"
)

// DataFlowMonitorConfig holds configuration for the data flow monitor.
type DataFlowMonitorConfig struct {
	// CheckInterval is how often to check if data is flowing
	CheckInterval time.Duration
	// StallThreshold is how long without data before considering stalled
	StallThreshold time.Duration
	// StallAction specifies what to do when data flow stalls
	StallAction StallAction
}

// StallAction specifies what action to take when data flow stalls.
type StallAction int

const (
	// StallActionLog only logs a warning when stalled
	StallActionLog StallAction = iota
	// StallActionRestart triggers a reconnection
	StallActionRestart
	// StallActionShutdown triggers a complete shutdown (for systemd restart)
	StallActionShutdown
)

// DefaultDataFlowMonitorConfig returns default monitor configuration.
func DefaultDataFlowMonitorConfig() *DataFlowMonitorConfig {
	return &DataFlowMonitorConfig{
		CheckInterval:  30 * time.Second,
		StallThreshold: 2 * time.Minute,
		StallAction:    StallActionLog,
	}
}

// DataFlowMonitor monitors if the tunnel is actually passing data.
type DataFlowMonitor struct {
	config *DataFlowMonitorConfig
	log    *logger.Logger

	// Counters for data flow
	bytesSent     int64
	bytesReceived int64
	packetsSent   int64
	packetsRecv   int64

	// Last activity timestamps
	lastSendTime int64 // Unix nano
	lastRecvTime int64 // Unix nano

	// Last check values (to detect change)
	lastCheckBytesSent int64
	lastCheckBytesRecv int64
	lastCheckTime      time.Time

	// State
	running  int32
	shutdown chan struct{}
	wg       sync.WaitGroup

	// Callback for stall action
	onStall func(action StallAction)
}

// NewDataFlowMonitor creates a new data flow monitor.
func NewDataFlowMonitor(config *DataFlowMonitorConfig, log *logger.Logger) *DataFlowMonitor {
	if config == nil {
		config = DefaultDataFlowMonitorConfig()
	}
	if log == nil {
		log = logger.NewDefault()
	}

	return &DataFlowMonitor{
		config:   config,
		log:      log,
		shutdown: make(chan struct{}),
	}
}

// SetStallCallback sets the callback function for when data flow stalls.
func (m *DataFlowMonitor) SetStallCallback(fn func(action StallAction)) {
	m.onStall = fn
}

// RecordSend records bytes sent through the tunnel.
func (m *DataFlowMonitor) RecordSend(bytes int64) {
	atomic.AddInt64(&m.bytesSent, bytes)
	atomic.AddInt64(&m.packetsSent, 1)
	atomic.StoreInt64(&m.lastSendTime, time.Now().UnixNano())
}

// RecordReceive records bytes received through the tunnel.
func (m *DataFlowMonitor) RecordReceive(bytes int64) {
	atomic.AddInt64(&m.bytesReceived, bytes)
	atomic.AddInt64(&m.packetsRecv, 1)
	atomic.StoreInt64(&m.lastRecvTime, time.Now().UnixNano())
}

// Start starts the data flow monitor.
func (m *DataFlowMonitor) Start(ctx context.Context) {
	if !atomic.CompareAndSwapInt32(&m.running, 0, 1) {
		return // Already running
	}

	m.lastCheckTime = time.Now()
	m.lastCheckBytesSent = 0
	m.lastCheckBytesRecv = 0

	m.wg.Add(1)
	go m.monitorLoop(ctx)
}

// Stop stops the data flow monitor.
func (m *DataFlowMonitor) Stop() {
	if !atomic.CompareAndSwapInt32(&m.running, 1, 0) {
		return
	}
	close(m.shutdown)
	m.wg.Wait()
}

// Stats returns current data flow statistics.
type DataFlowStats struct {
	BytesSent     int64
	BytesReceived int64
	PacketsSent   int64
	PacketsRecv   int64
	LastSendTime  time.Time
	LastRecvTime  time.Time
	IsFlowing     bool
}

// GetStats returns current data flow statistics.
func (m *DataFlowMonitor) GetStats() DataFlowStats {
	lastSend := atomic.LoadInt64(&m.lastSendTime)
	lastRecv := atomic.LoadInt64(&m.lastRecvTime)

	var lastSendTime, lastRecvTime time.Time
	if lastSend > 0 {
		lastSendTime = time.Unix(0, lastSend)
	}
	if lastRecv > 0 {
		lastRecvTime = time.Unix(0, lastRecv)
	}

	// Determine if data is flowing (activity within stall threshold)
	isFlowing := false
	now := time.Now()
	if !lastSendTime.IsZero() && now.Sub(lastSendTime) < m.config.StallThreshold {
		isFlowing = true
	}
	if !lastRecvTime.IsZero() && now.Sub(lastRecvTime) < m.config.StallThreshold {
		isFlowing = true
	}

	return DataFlowStats{
		BytesSent:     atomic.LoadInt64(&m.bytesSent),
		BytesReceived: atomic.LoadInt64(&m.bytesReceived),
		PacketsSent:   atomic.LoadInt64(&m.packetsSent),
		PacketsRecv:   atomic.LoadInt64(&m.packetsRecv),
		LastSendTime:  lastSendTime,
		LastRecvTime:  lastRecvTime,
		IsFlowing:     isFlowing,
	}
}

// monitorLoop runs the periodic health check.
func (m *DataFlowMonitor) monitorLoop(ctx context.Context) {
	defer m.wg.Done()

	ticker := time.NewTicker(m.config.CheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-m.shutdown:
			return
		case <-ticker.C:
			m.checkDataFlow()
		}
	}
}

// checkDataFlow checks if data is flowing and takes action if stalled.
func (m *DataFlowMonitor) checkDataFlow() {
	currentBytesSent := atomic.LoadInt64(&m.bytesSent)
	currentBytesRecv := atomic.LoadInt64(&m.bytesReceived)
	now := time.Now()

	// Calculate data transferred since last check
	deltaBytesSent := currentBytesSent - m.lastCheckBytesSent
	deltaBytesRecv := currentBytesRecv - m.lastCheckBytesRecv
	elapsed := now.Sub(m.lastCheckTime)

	// Get last activity times
	lastSend := atomic.LoadInt64(&m.lastSendTime)
	lastRecv := atomic.LoadInt64(&m.lastRecvTime)

	var lastSendTime, lastRecvTime time.Time
	if lastSend > 0 {
		lastSendTime = time.Unix(0, lastSend)
	}
	if lastRecv > 0 {
		lastRecvTime = time.Unix(0, lastRecv)
	}

	// Determine last activity time
	lastActivity := lastSendTime
	if lastRecvTime.After(lastActivity) {
		lastActivity = lastRecvTime
	}

	// Check if stalled (no data flow and enough time has passed)
	timeSinceActivity := now.Sub(lastActivity)
	isStalled := !lastActivity.IsZero() && timeSinceActivity > m.config.StallThreshold

	// Log periodic stats
	if deltaBytesSent > 0 || deltaBytesRecv > 0 {
		sendRate := float64(deltaBytesSent) / elapsed.Seconds()
		recvRate := float64(deltaBytesRecv) / elapsed.Seconds()

		m.log.Info().
			Int64("bytes_sent", deltaBytesSent).
			Int64("bytes_recv", deltaBytesRecv).
			Float64("send_rate_bps", sendRate).
			Float64("recv_rate_bps", recvRate).
			Int64("total_sent", currentBytesSent).
			Int64("total_recv", currentBytesRecv).
			Msg("Data flow stats")
	} else if lastActivity.IsZero() {
		// No data has ever been transferred
		m.log.Debug().Msg("Data flow monitor: No data transferred yet")
	} else if isStalled {
		// Data was flowing but has stopped
		m.log.Warn().
			Dur("time_since_activity", timeSinceActivity).
			Time("last_send", lastSendTime).
			Time("last_recv", lastRecvTime).
			Msg("Data flow stalled - no data transferred")

		// Take action based on configuration
		if m.onStall != nil {
			m.onStall(m.config.StallAction)
		}

		switch m.config.StallAction {
		case StallActionLog:
			// Already logged above
		case StallActionRestart:
			m.log.Warn().Msg("Triggering tunnel restart due to stalled data flow")
		case StallActionShutdown:
			m.log.Error().Msg("Triggering shutdown due to stalled data flow")
		}
	} else {
		// Data is flowing normally, log at debug level
		m.log.Debug().
			Dur("time_since_activity", timeSinceActivity).
			Msg("Data flow OK")
	}

	// Update last check values
	m.lastCheckBytesSent = currentBytesSent
	m.lastCheckBytesRecv = currentBytesRecv
	m.lastCheckTime = now
}

// Reset resets all counters (typically called after reconnection).
func (m *DataFlowMonitor) Reset() {
	atomic.StoreInt64(&m.bytesSent, 0)
	atomic.StoreInt64(&m.bytesReceived, 0)
	atomic.StoreInt64(&m.packetsSent, 0)
	atomic.StoreInt64(&m.packetsRecv, 0)
	atomic.StoreInt64(&m.lastSendTime, 0)
	atomic.StoreInt64(&m.lastRecvTime, 0)
	m.lastCheckBytesSent = 0
	m.lastCheckBytesRecv = 0
	m.lastCheckTime = time.Now()
}
