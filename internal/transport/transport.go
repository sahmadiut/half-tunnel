// Package transport provides WebSocket connection managers for the Half-Tunnel system.
package transport

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sahmadiut/half-tunnel/internal/constants"
)

// Errors
var (
	ErrConnectionClosed = errors.New("connection closed")
	ErrWriteTimeout     = errors.New("write timeout")
	ErrReadTimeout      = errors.New("read timeout")
)

// Config holds transport configuration.
type Config struct {
	URL              string
	TLSConfig        *tls.Config
	PingInterval     time.Duration
	PongTimeout      time.Duration
	WriteTimeout     time.Duration
	ReadTimeout      time.Duration
	MaxMessageSize   int64
	HandshakeTimeout time.Duration
	ReadBufferSize   int
	WriteBufferSize  int
	// ResolveIP allows manual IP specification instead of DNS lookup
	ResolveIP string
	// IPVersion forces IPv4 ("4") or IPv6 ("6"), empty for auto
	IPVersion string
	// TCPNoDelay disables Nagle's algorithm for lower latency
	TCPNoDelay bool
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig(url string) *Config {
	return &Config{
		URL:              url,
		PingInterval:     30 * time.Second,
		PongTimeout:      10 * time.Second,
		WriteTimeout:     10 * time.Second,
		ReadTimeout:      60 * time.Second,
		MaxMessageSize:   1024 * 1024, // 1MB
		HandshakeTimeout: 10 * time.Second,
		ReadBufferSize:   constants.DefaultBufferSize,
		WriteBufferSize:  constants.DefaultBufferSize,
		TCPNoDelay:       true,
	}
}

// Connection represents a WebSocket connection with health monitoring.
type Connection struct {
	conn     *websocket.Conn
	config   *Config
	mu       sync.Mutex
	closed   bool
	closedCh chan struct{}
}

// createDialer creates a net.Dialer configured based on Config settings.
func createDialer(config *Config) *net.Dialer {
	dialer := &net.Dialer{
		Timeout: config.HandshakeTimeout,
	}

	// Configure TCP keep-alive
	dialer.KeepAlive = 30 * time.Second

	return dialer
}

// getNetworkType returns the network type based on IP version setting.
func getNetworkType(ipVersion string) string {
	switch ipVersion {
	case "4":
		return "tcp4"
	case "6":
		return "tcp6"
	default:
		return "tcp"
	}
}

// createCustomDialContext creates a dial context function that uses ResolveIP if set.
func createCustomDialContext(config *Config) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		dialer := createDialer(config)

		// Determine network type
		netType := network
		if config.IPVersion != "" {
			netType = getNetworkType(config.IPVersion)
		}

		// If ResolveIP is set, use it instead of DNS lookup
		if config.ResolveIP != "" {
			_, port, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			addr = net.JoinHostPort(config.ResolveIP, port)
		}

		conn, err := dialer.DialContext(ctx, netType, addr)
		if err != nil {
			return nil, err
		}

		// Apply TCP options (best effort - failures are rare and non-critical)
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			if config.TCPNoDelay {
				// SetNoDelay error is ignored as it's a performance optimization
				// and rarely fails. The connection is still usable if it fails.
				_ = tcpConn.SetNoDelay(true)
			}
		}

		return conn, nil
	}
}

// Dial creates a new WebSocket connection.
func Dial(ctx context.Context, config *Config) (*Connection, error) {
	dialer := websocket.Dialer{
		TLSClientConfig:  config.TLSConfig,
		HandshakeTimeout: config.HandshakeTimeout,
		NetDialContext:   createCustomDialContext(config),
	}
	if config.ReadBufferSize > 0 {
		dialer.ReadBufferSize = config.ReadBufferSize
	}
	if config.WriteBufferSize > 0 {
		dialer.WriteBufferSize = config.WriteBufferSize
	}

	// For TLS connections with ResolveIP, we need to ensure the TLS config
	// has the correct ServerName set based on the original URL
	if config.ResolveIP != "" && config.TLSConfig != nil {
		parsedURL, err := url.Parse(config.URL)
		if err == nil && config.TLSConfig.ServerName == "" {
			// Clone the TLS config and set ServerName from the URL host
			tlsConfig := config.TLSConfig.Clone()
			tlsConfig.ServerName = parsedURL.Hostname()
			dialer.TLSClientConfig = tlsConfig
		}
	}

	conn, _, err := dialer.DialContext(ctx, config.URL, http.Header{})
	if err != nil {
		return nil, err
	}

	conn.SetReadLimit(config.MaxMessageSize)

	c := &Connection{
		conn:     conn,
		config:   config,
		closedCh: make(chan struct{}),
	}

	return c, nil
}

// Write sends data over the connection.
func (c *Connection) Write(data []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return ErrConnectionClosed
	}

	if c.config.WriteTimeout > 0 {
		if err := c.conn.SetWriteDeadline(time.Now().Add(c.config.WriteTimeout)); err != nil {
			return err
		}
	}

	return c.conn.WriteMessage(websocket.BinaryMessage, data)
}

// Read reads data from the connection.
func (c *Connection) Read() ([]byte, error) {
	if c.config.ReadTimeout > 0 {
		if err := c.conn.SetReadDeadline(time.Now().Add(c.config.ReadTimeout)); err != nil {
			return nil, err
		}
	}

	messageType, data, err := c.conn.ReadMessage()
	if err != nil {
		return nil, err
	}

	if messageType != websocket.BinaryMessage {
		return nil, errors.New("expected binary message")
	}

	return data, nil
}

// Close closes the connection gracefully.
func (c *Connection) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true
	close(c.closedCh)

	// Send close message (best effort, ignore errors)
	_ = c.conn.WriteMessage(
		websocket.CloseMessage,
		websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""),
	)

	return c.conn.Close()
}

// IsClosed returns true if the connection is closed.
func (c *Connection) IsClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

// ClosedChan returns a channel that is closed when the connection is closed.
func (c *Connection) ClosedChan() <-chan struct{} {
	return c.closedCh
}

// RemoteAddr returns the remote address for the connection.
func (c *Connection) RemoteAddr() string {
	if c == nil || c.conn == nil {
		return ""
	}
	return c.conn.RemoteAddr().String()
}

// Transport defines the interface for split-path transports.
type Transport interface {
	// Write sends data through the transport.
	Write(data []byte) error
	// Read reads data from the transport.
	Read() ([]byte, error)
	// Close closes the transport.
	Close() error
	// IsClosed returns true if the transport is closed.
	IsClosed() bool
}

// Manager manages upstream and downstream connections.
type Manager struct {
	upstreamConfig   *Config
	downstreamConfig *Config
	upstream         *Connection
	downstream       *Connection
	mu               sync.RWMutex
}

// NewManager creates a new transport manager.
func NewManager(upstreamConfig, downstreamConfig *Config) *Manager {
	return &Manager{
		upstreamConfig:   upstreamConfig,
		downstreamConfig: downstreamConfig,
	}
}

// Connect establishes upstream and downstream connections.
func (m *Manager) Connect(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var err error

	m.upstream, err = Dial(ctx, m.upstreamConfig)
	if err != nil {
		return err
	}

	m.downstream, err = Dial(ctx, m.downstreamConfig)
	if err != nil {
		m.upstream.Close()
		m.upstream = nil
		return err
	}

	return nil
}

// WriteUpstream writes data to the upstream connection.
func (m *Manager) WriteUpstream(data []byte) error {
	m.mu.RLock()
	conn := m.upstream
	m.mu.RUnlock()

	if conn == nil {
		return ErrConnectionClosed
	}
	return conn.Write(data)
}

// ReadDownstream reads data from the downstream connection.
func (m *Manager) ReadDownstream() ([]byte, error) {
	m.mu.RLock()
	conn := m.downstream
	m.mu.RUnlock()

	if conn == nil {
		return nil, ErrConnectionClosed
	}
	return conn.Read()
}

// Close closes both connections.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errs []error

	if m.upstream != nil {
		if err := m.upstream.Close(); err != nil {
			errs = append(errs, err)
		}
		m.upstream = nil
	}

	if m.downstream != nil {
		if err := m.downstream.Close(); err != nil {
			errs = append(errs, err)
		}
		m.downstream = nil
	}

	if len(errs) > 0 {
		return errs[0]
	}
	return nil
}

// Verify Connection implements Transport interface
var _ Transport = (*Connection)(nil)

// Verify Connection implements io.Closer
var _ io.Closer = (*Connection)(nil)
