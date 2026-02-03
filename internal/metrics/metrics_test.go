package metrics

import (
	"strings"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

func TestNewCollector(t *testing.T) {
	c := NewCollector()
	if c == nil {
		t.Fatal("expected non-nil collector")
	}
	if c.PacketsSent == nil {
		t.Error("PacketsSent should be initialized")
	}
	if c.ActiveSessions == nil {
		t.Error("ActiveSessions should be initialized")
	}
}

func TestCollector_Register(t *testing.T) {
	c := NewCollector()
	registry := prometheus.NewRegistry()

	err := c.Register(registry)
	if err != nil {
		t.Fatalf("failed to register: %v", err)
	}

	// Try to register again should fail
	err = c.Register(registry)
	if err == nil {
		t.Error("expected error when registering twice")
	}
}

func TestCollector_RecordPacketSent(t *testing.T) {
	c := NewCollector()
	registry := prometheus.NewRegistry()
	c.MustRegister(registry)

	c.RecordPacketSent("upstream", 100)
	c.RecordPacketSent("upstream", 200)
	c.RecordPacketSent("downstream", 50)

	// Check packets_sent counter
	expected := `
# HELP halftunnel_packets_sent_total Total number of packets sent
# TYPE halftunnel_packets_sent_total counter
halftunnel_packets_sent_total{direction="downstream"} 1
halftunnel_packets_sent_total{direction="upstream"} 2
`
	if err := testutil.CollectAndCompare(c.PacketsSent, strings.NewReader(expected)); err != nil {
		t.Errorf("packets sent mismatch: %v", err)
	}

	// Check bytes_sent counter
	expectedBytes := `
# HELP halftunnel_bytes_sent_total Total bytes sent
# TYPE halftunnel_bytes_sent_total counter
halftunnel_bytes_sent_total{direction="downstream"} 50
halftunnel_bytes_sent_total{direction="upstream"} 300
`
	if err := testutil.CollectAndCompare(c.BytesSent, strings.NewReader(expectedBytes)); err != nil {
		t.Errorf("bytes sent mismatch: %v", err)
	}
}

func TestCollector_RecordPacketReceived(t *testing.T) {
	c := NewCollector()
	registry := prometheus.NewRegistry()
	c.MustRegister(registry)

	c.RecordPacketReceived("downstream", 100)

	expected := `
# HELP halftunnel_packets_received_total Total number of packets received
# TYPE halftunnel_packets_received_total counter
halftunnel_packets_received_total{direction="downstream"} 1
`
	if err := testutil.CollectAndCompare(c.PacketsReceived, strings.NewReader(expected)); err != nil {
		t.Errorf("packets received mismatch: %v", err)
	}
}

func TestCollector_SessionMetrics(t *testing.T) {
	c := NewCollector()
	registry := prometheus.NewRegistry()
	c.MustRegister(registry)

	c.RecordSessionCreated()
	c.RecordSessionCreated()
	c.RecordSessionClosed()

	// Active sessions should be 1 (2 created - 1 closed)
	activeSessions := testutil.ToFloat64(c.ActiveSessions)
	if activeSessions != 1 {
		t.Errorf("expected 1 active session, got %v", activeSessions)
	}

	// Total sessions should be 2
	totalSessions := testutil.ToFloat64(c.TotalSessions)
	if totalSessions != 2 {
		t.Errorf("expected 2 total sessions, got %v", totalSessions)
	}
}

func TestCollector_StreamMetrics(t *testing.T) {
	c := NewCollector()
	registry := prometheus.NewRegistry()
	c.MustRegister(registry)

	c.RecordStreamCreated()
	c.RecordStreamCreated()
	c.RecordStreamCreated()
	c.RecordStreamClosed()

	activeStreams := testutil.ToFloat64(c.ActiveStreams)
	if activeStreams != 2 {
		t.Errorf("expected 2 active streams, got %v", activeStreams)
	}

	totalStreams := testutil.ToFloat64(c.TotalStreams)
	if totalStreams != 3 {
		t.Errorf("expected 3 total streams, got %v", totalStreams)
	}
}

func TestCollector_RecordStreamLatency(t *testing.T) {
	c := NewCollector()
	registry := prometheus.NewRegistry()
	c.MustRegister(registry)

	c.RecordStreamLatency("connect", 100*time.Millisecond)
	c.RecordStreamLatency("connect", 200*time.Millisecond)

	// Just verify it doesn't panic and histogram has data
	count := testutil.CollectAndCount(c.StreamLatency)
	if count != 1 {
		t.Errorf("expected 1 histogram metric, got %d", count)
	}
}

func TestCollector_RecordPacketLatency(t *testing.T) {
	c := NewCollector()
	registry := prometheus.NewRegistry()
	c.MustRegister(registry)

	c.RecordPacketLatency("upstream", 10*time.Millisecond)

	count := testutil.CollectAndCount(c.PacketLatency)
	if count != 1 {
		t.Errorf("expected 1 histogram metric, got %d", count)
	}
}

func TestCollector_SetConnectionStatus(t *testing.T) {
	c := NewCollector()
	registry := prometheus.NewRegistry()
	c.MustRegister(registry)

	c.SetConnectionStatus("upstream", true)
	c.SetConnectionStatus("downstream", false)

	expected := `
# HELP halftunnel_connection_status Connection status (1 = connected, 0 = disconnected)
# TYPE halftunnel_connection_status gauge
halftunnel_connection_status{connection="downstream"} 0
halftunnel_connection_status{connection="upstream"} 1
`
	if err := testutil.CollectAndCompare(c.ConnectionStatus, strings.NewReader(expected)); err != nil {
		t.Errorf("connection status mismatch: %v", err)
	}
}

func TestCollector_RecordError(t *testing.T) {
	c := NewCollector()
	registry := prometheus.NewRegistry()
	c.MustRegister(registry)

	c.RecordError("connection")
	c.RecordError("connection")
	c.RecordError("protocol")

	expected := `
# HELP halftunnel_errors_total Total number of errors
# TYPE halftunnel_errors_total counter
halftunnel_errors_total{type="connection"} 2
halftunnel_errors_total{type="protocol"} 1
`
	if err := testutil.CollectAndCompare(c.Errors, strings.NewReader(expected)); err != nil {
		t.Errorf("errors mismatch: %v", err)
	}
}

func TestCollector_CircuitBreakerMetrics(t *testing.T) {
	c := NewCollector()
	registry := prometheus.NewRegistry()
	c.MustRegister(registry)

	c.SetCircuitBreakerState("downstream", 1) // open
	c.RecordCircuitBreakerTrip("downstream")

	stateValue := testutil.ToFloat64(c.CircuitBreakerState.WithLabelValues("downstream"))
	if stateValue != 1 {
		t.Errorf("expected state 1 (open), got %v", stateValue)
	}

	tripValue := testutil.ToFloat64(c.CircuitBreakerTrips.WithLabelValues("downstream"))
	if tripValue != 1 {
		t.Errorf("expected 1 trip, got %v", tripValue)
	}
}

func TestCollector_ReconnectMetrics(t *testing.T) {
	c := NewCollector()
	registry := prometheus.NewRegistry()
	c.MustRegister(registry)

	c.RecordReconnectAttempt("upstream")
	c.RecordReconnectAttempt("upstream")
	c.RecordReconnectSuccess("upstream")
	c.RecordReconnectFailure("downstream")

	attemptValue := testutil.ToFloat64(c.ReconnectAttempts.WithLabelValues("upstream"))
	if attemptValue != 2 {
		t.Errorf("expected 2 attempts, got %v", attemptValue)
	}

	successValue := testutil.ToFloat64(c.ReconnectSuccess.WithLabelValues("upstream"))
	if successValue != 1 {
		t.Errorf("expected 1 success, got %v", successValue)
	}

	failureValue := testutil.ToFloat64(c.ReconnectFailure.WithLabelValues("downstream"))
	if failureValue != 1 {
		t.Errorf("expected 1 failure, got %v", failureValue)
	}
}

func TestDefaultServerConfig(t *testing.T) {
	cfg := DefaultServerConfig()
	if cfg.Addr != ":9090" {
		t.Errorf("expected :9090, got %s", cfg.Addr)
	}
	if cfg.Path != "/metrics" {
		t.Errorf("expected /metrics, got %s", cfg.Path)
	}
}

func TestServer(t *testing.T) {
	cfg := &ServerConfig{
		Addr: ":9099",
		Path: "/metrics",
	}
	s := NewServer(cfg)

	if s.Collector() == nil {
		t.Error("expected non-nil collector")
	}
	if s.Registry() == nil {
		t.Error("expected non-nil registry")
	}
	if s.Addr() != ":9099" {
		t.Errorf("expected :9099, got %s", s.Addr())
	}
}
