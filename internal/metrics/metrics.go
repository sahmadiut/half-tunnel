// Package metrics provides Prometheus metrics for the Half-Tunnel system.
package metrics

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// Namespace is the Prometheus namespace for Half-Tunnel metrics.
const Namespace = "halftunnel"

// Collector holds all Prometheus metrics for the Half-Tunnel system.
type Collector struct {
	// Connection metrics
	PacketsSent     *prometheus.CounterVec
	PacketsReceived *prometheus.CounterVec
	BytesSent       *prometheus.CounterVec
	BytesReceived   *prometheus.CounterVec

	// Session metrics
	ActiveSessions prometheus.Gauge
	TotalSessions  prometheus.Counter

	// Stream metrics
	ActiveStreams prometheus.Gauge
	TotalStreams  prometheus.Counter

	// Latency metrics
	StreamLatency  *prometheus.HistogramVec
	PacketLatency  *prometheus.HistogramVec

	// Connection status
	ConnectionStatus *prometheus.GaugeVec

	// Error metrics
	Errors *prometheus.CounterVec

	// Circuit breaker metrics
	CircuitBreakerState   *prometheus.GaugeVec
	CircuitBreakerTrips   *prometheus.CounterVec

	// Reconnection metrics
	ReconnectAttempts *prometheus.CounterVec
	ReconnectSuccess  *prometheus.CounterVec
	ReconnectFailure  *prometheus.CounterVec
}

// NewCollector creates a new metrics collector with all metrics registered.
func NewCollector() *Collector {
	c := &Collector{
		PacketsSent: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: Namespace,
				Name:      "packets_sent_total",
				Help:      "Total number of packets sent",
			},
			[]string{"direction"}, // "upstream" or "downstream"
		),
		PacketsReceived: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: Namespace,
				Name:      "packets_received_total",
				Help:      "Total number of packets received",
			},
			[]string{"direction"},
		),
		BytesSent: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: Namespace,
				Name:      "bytes_sent_total",
				Help:      "Total bytes sent",
			},
			[]string{"direction"},
		),
		BytesReceived: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: Namespace,
				Name:      "bytes_received_total",
				Help:      "Total bytes received",
			},
			[]string{"direction"},
		),
		ActiveSessions: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: Namespace,
				Name:      "active_sessions",
				Help:      "Number of currently active sessions",
			},
		),
		TotalSessions: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: Namespace,
				Name:      "sessions_total",
				Help:      "Total number of sessions created",
			},
		),
		ActiveStreams: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Namespace: Namespace,
				Name:      "active_streams",
				Help:      "Number of currently active streams",
			},
		),
		TotalStreams: prometheus.NewCounter(
			prometheus.CounterOpts{
				Namespace: Namespace,
				Name:      "streams_total",
				Help:      "Total number of streams created",
			},
		),
		StreamLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: Namespace,
				Name:      "stream_latency_seconds",
				Help:      "Latency of stream operations in seconds",
				Buckets:   prometheus.ExponentialBuckets(0.001, 2, 15), // 1ms to ~16s
			},
			[]string{"operation"}, // "connect", "first_byte", "total"
		),
		PacketLatency: prometheus.NewHistogramVec(
			prometheus.HistogramOpts{
				Namespace: Namespace,
				Name:      "packet_latency_seconds",
				Help:      "Latency of packet operations in seconds",
				Buckets:   prometheus.ExponentialBuckets(0.0001, 2, 15), // 0.1ms to ~1.6s
			},
			[]string{"direction"},
		),
		ConnectionStatus: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: Namespace,
				Name:      "connection_status",
				Help:      "Connection status (1 = connected, 0 = disconnected)",
			},
			[]string{"connection"}, // "upstream", "downstream"
		),
		Errors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: Namespace,
				Name:      "errors_total",
				Help:      "Total number of errors",
			},
			[]string{"type"}, // "connection", "protocol", "timeout", etc.
		),
		CircuitBreakerState: prometheus.NewGaugeVec(
			prometheus.GaugeOpts{
				Namespace: Namespace,
				Name:      "circuit_breaker_state",
				Help:      "Circuit breaker state (0 = closed, 1 = open, 2 = half-open)",
			},
			[]string{"name"},
		),
		CircuitBreakerTrips: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: Namespace,
				Name:      "circuit_breaker_trips_total",
				Help:      "Total number of circuit breaker trips",
			},
			[]string{"name"},
		),
		ReconnectAttempts: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: Namespace,
				Name:      "reconnect_attempts_total",
				Help:      "Total number of reconnection attempts",
			},
			[]string{"connection"},
		),
		ReconnectSuccess: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: Namespace,
				Name:      "reconnect_success_total",
				Help:      "Total number of successful reconnections",
			},
			[]string{"connection"},
		),
		ReconnectFailure: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Namespace: Namespace,
				Name:      "reconnect_failure_total",
				Help:      "Total number of failed reconnections",
			},
			[]string{"connection"},
		),
	}

	return c
}

// Register registers all metrics with the given registry.
func (c *Collector) Register(reg prometheus.Registerer) error {
	collectors := []prometheus.Collector{
		c.PacketsSent,
		c.PacketsReceived,
		c.BytesSent,
		c.BytesReceived,
		c.ActiveSessions,
		c.TotalSessions,
		c.ActiveStreams,
		c.TotalStreams,
		c.StreamLatency,
		c.PacketLatency,
		c.ConnectionStatus,
		c.Errors,
		c.CircuitBreakerState,
		c.CircuitBreakerTrips,
		c.ReconnectAttempts,
		c.ReconnectSuccess,
		c.ReconnectFailure,
	}

	for _, collector := range collectors {
		if err := reg.Register(collector); err != nil {
			return err
		}
	}

	return nil
}

// MustRegister registers all metrics and panics on error.
func (c *Collector) MustRegister(reg prometheus.Registerer) {
	if err := c.Register(reg); err != nil {
		panic(err)
	}
}

// RecordPacketSent records a sent packet.
func (c *Collector) RecordPacketSent(direction string, bytes int) {
	c.PacketsSent.WithLabelValues(direction).Inc()
	c.BytesSent.WithLabelValues(direction).Add(float64(bytes))
}

// RecordPacketReceived records a received packet.
func (c *Collector) RecordPacketReceived(direction string, bytes int) {
	c.PacketsReceived.WithLabelValues(direction).Inc()
	c.BytesReceived.WithLabelValues(direction).Add(float64(bytes))
}

// RecordSessionCreated records a new session creation.
func (c *Collector) RecordSessionCreated() {
	c.ActiveSessions.Inc()
	c.TotalSessions.Inc()
}

// RecordSessionClosed records a session closure.
func (c *Collector) RecordSessionClosed() {
	c.ActiveSessions.Dec()
}

// RecordStreamCreated records a new stream creation.
func (c *Collector) RecordStreamCreated() {
	c.ActiveStreams.Inc()
	c.TotalStreams.Inc()
}

// RecordStreamClosed records a stream closure.
func (c *Collector) RecordStreamClosed() {
	c.ActiveStreams.Dec()
}

// RecordStreamLatency records stream operation latency.
func (c *Collector) RecordStreamLatency(operation string, duration time.Duration) {
	c.StreamLatency.WithLabelValues(operation).Observe(duration.Seconds())
}

// RecordPacketLatency records packet operation latency.
func (c *Collector) RecordPacketLatency(direction string, duration time.Duration) {
	c.PacketLatency.WithLabelValues(direction).Observe(duration.Seconds())
}

// SetConnectionStatus sets the connection status.
func (c *Collector) SetConnectionStatus(connection string, connected bool) {
	value := 0.0
	if connected {
		value = 1.0
	}
	c.ConnectionStatus.WithLabelValues(connection).Set(value)
}

// RecordError records an error.
func (c *Collector) RecordError(errorType string) {
	c.Errors.WithLabelValues(errorType).Inc()
}

// SetCircuitBreakerState sets the circuit breaker state.
// state: 0 = closed, 1 = open, 2 = half-open
func (c *Collector) SetCircuitBreakerState(name string, state int) {
	c.CircuitBreakerState.WithLabelValues(name).Set(float64(state))
}

// RecordCircuitBreakerTrip records a circuit breaker trip.
func (c *Collector) RecordCircuitBreakerTrip(name string) {
	c.CircuitBreakerTrips.WithLabelValues(name).Inc()
}

// RecordReconnectAttempt records a reconnection attempt.
func (c *Collector) RecordReconnectAttempt(connection string) {
	c.ReconnectAttempts.WithLabelValues(connection).Inc()
}

// RecordReconnectSuccess records a successful reconnection.
func (c *Collector) RecordReconnectSuccess(connection string) {
	c.ReconnectSuccess.WithLabelValues(connection).Inc()
}

// RecordReconnectFailure records a failed reconnection.
func (c *Collector) RecordReconnectFailure(connection string) {
	c.ReconnectFailure.WithLabelValues(connection).Inc()
}

// Server is an HTTP server that exposes Prometheus metrics.
type Server struct {
	server    *http.Server
	collector *Collector
	registry  *prometheus.Registry
	addr      string
}

// ServerConfig holds configuration for the metrics server.
type ServerConfig struct {
	Addr string
	Path string
}

// DefaultServerConfig returns a ServerConfig with sensible defaults.
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Addr: ":9090",
		Path: "/metrics",
	}
}

// NewServer creates a new metrics server.
func NewServer(config *ServerConfig) *Server {
	if config == nil {
		config = DefaultServerConfig()
	}

	registry := prometheus.NewRegistry()
	collector := NewCollector()
	collector.MustRegister(registry)

	// Also register default Go collectors
	registry.MustRegister(prometheus.NewProcessCollector(prometheus.ProcessCollectorOpts{}))
	registry.MustRegister(prometheus.NewGoCollector())

	mux := http.NewServeMux()
	mux.Handle(config.Path, promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	return &Server{
		server: &http.Server{
			Addr:         config.Addr,
			Handler:      mux,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		},
		collector: collector,
		registry:  registry,
		addr:      config.Addr,
	}
}

// Collector returns the metrics collector.
func (s *Server) Collector() *Collector {
	return s.collector
}

// Registry returns the Prometheus registry.
func (s *Server) Registry() *prometheus.Registry {
	return s.registry
}

// Start starts the metrics server.
func (s *Server) Start() error {
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the metrics server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// Addr returns the server address.
func (s *Server) Addr() string {
	return s.addr
}

// Handler returns an HTTP handler for the metrics endpoint.
func Handler(registry *prometheus.Registry) http.Handler {
	return promhttp.HandlerFor(registry, promhttp.HandlerOpts{})
}

// DefaultHandler returns an HTTP handler using the default registry.
func DefaultHandler() http.Handler {
	return promhttp.Handler()
}
