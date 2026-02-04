// Package transport provides WebSocket and Chisel connection managers for the Half-Tunnel system.
package transport

import (
	"context"
	"fmt"
	"io"
	"net"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	chiselclient "github.com/jpillora/chisel/client"
	chiselserver "github.com/jpillora/chisel/server"
	"github.com/sahmadiut/half-tunnel/pkg/logger"
)

// ChiselConfig holds configuration for Chisel transport.
type ChiselConfig struct {
	// Enabled controls whether Chisel transport is used for data transfer
	Enabled bool
	// ServerURL is the URL for the Chisel server (for client mode)
	ServerURL string
	// Host is the listening host (for server mode)
	Host string
	// Port is the starting port for Chisel tunnels
	Port int
	// TLSCert is the path to TLS certificate file (for server mode)
	TLSCert string
	// TLSKey is the path to TLS key file (for server mode)
	TLSKey string
	// KeepAlive interval for connections
	KeepAlive time.Duration
	// Fingerprint is the expected server fingerprint (for client mode)
	Fingerprint string
	// Verbose enables verbose logging
	Verbose bool
}

// DefaultChiselConfig returns a ChiselConfig with sensible defaults.
func DefaultChiselConfig() *ChiselConfig {
	return &ChiselConfig{
		Enabled:   false,
		Host:      "0.0.0.0",
		Port:      9000,
		KeepAlive: 25 * time.Second,
		Verbose:   false,
	}
}

// ChiselServerTransport wraps a Chisel server for data transport.
type ChiselServerTransport struct {
	config    *ChiselConfig
	server    *chiselserver.Server
	log       *logger.Logger
	ports     *PortManager
	running   int32
	ctx       context.Context
	cancel    context.CancelFunc
	mu        sync.RWMutex
	listeners map[int]net.Listener // Local listeners for port forwarding
}

// NewChiselServerTransport creates a new Chisel server transport.
func NewChiselServerTransport(config *ChiselConfig, log *logger.Logger) (*ChiselServerTransport, error) {
	if config == nil {
		config = DefaultChiselConfig()
	}
	if log == nil {
		log = logger.NewDefault()
	}

	chiselConfig := &chiselserver.Config{
		Reverse:   true, // Allow reverse port forwarding
		Socks5:    false,
		KeepAlive: config.KeepAlive,
	}

	if config.TLSCert != "" && config.TLSKey != "" {
		chiselConfig.TLS.Cert = config.TLSCert
		chiselConfig.TLS.Key = config.TLSKey
	}

	server, err := chiselserver.NewServer(chiselConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to create chisel server: %w", err)
	}

	return &ChiselServerTransport{
		config:    config,
		server:    server,
		log:       log.WithStr("component", "chisel-server"),
		ports:     NewPortManager(config.Port, config.Port+1000),
		listeners: make(map[int]net.Listener),
	}, nil
}

// Start starts the Chisel server.
func (t *ChiselServerTransport) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&t.running, 0, 1) {
		return fmt.Errorf("chisel server already running")
	}

	t.ctx, t.cancel = context.WithCancel(ctx)

	host := t.config.Host
	port := strconv.Itoa(t.config.Port)

	t.log.Info().
		Str("host", host).
		Str("port", port).
		Msg("Starting Chisel server")

	go func() {
		if err := t.server.StartContext(t.ctx, host, port); err != nil {
			t.log.Error().Err(err).Msg("Chisel server error")
		}
	}()

	return nil
}

// Stop stops the Chisel server.
func (t *ChiselServerTransport) Stop() error {
	if !atomic.CompareAndSwapInt32(&t.running, 1, 0) {
		return nil
	}

	if t.cancel != nil {
		t.cancel()
	}

	// Close all local listeners
	t.mu.Lock()
	for port, listener := range t.listeners {
		listener.Close()
		delete(t.listeners, port)
	}
	t.mu.Unlock()

	if t.server != nil {
		return t.server.Close()
	}

	t.log.Info().Msg("Chisel server stopped")
	return nil
}

// AllocatePort allocates a port for data transfer.
func (t *ChiselServerTransport) AllocatePort() (int, error) {
	return t.ports.Allocate()
}

// ReleasePort releases a previously allocated port.
func (t *ChiselServerTransport) ReleasePort(port int) {
	t.ports.Release(port)
}

// GetServerURL returns the URL for clients to connect to.
func (t *ChiselServerTransport) GetServerURL() string {
	scheme := "http"
	if t.config.TLSCert != "" {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s:%d", scheme, t.config.Host, t.config.Port)
}

// GetFingerprint returns the server's fingerprint for client verification.
func (t *ChiselServerTransport) GetFingerprint() string {
	if t.server != nil {
		return t.server.GetFingerprint()
	}
	return ""
}

// ChiselClientTransport wraps a Chisel client for data transport.
type ChiselClientTransport struct {
	config  *ChiselConfig
	client  *chiselclient.Client
	log     *logger.Logger
	ports   *PortManager
	running int32
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.RWMutex
	remotes []string // Current remote specifications
}

// NewChiselClientTransport creates a new Chisel client transport.
func NewChiselClientTransport(config *ChiselConfig, log *logger.Logger) (*ChiselClientTransport, error) {
	if config == nil {
		return nil, fmt.Errorf("chisel config is required")
	}
	if log == nil {
		log = logger.NewDefault()
	}

	return &ChiselClientTransport{
		config:  config,
		log:     log.WithStr("component", "chisel-client"),
		ports:   NewPortManager(config.Port, config.Port+1000),
		remotes: make([]string, 0),
	}, nil
}

// Start starts the Chisel client connection.
func (t *ChiselClientTransport) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&t.running, 0, 1) {
		return fmt.Errorf("chisel client already running")
	}

	t.ctx, t.cancel = context.WithCancel(ctx)

	return nil
}

// Connect connects the Chisel client with the specified remotes.
func (t *ChiselClientTransport) Connect(remotes []string) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	// Close existing client if any
	if t.client != nil {
		t.client.Close()
		t.client = nil
	}

	chiselConfig := &chiselclient.Config{
		Fingerprint: t.config.Fingerprint,
		Server:      t.config.ServerURL,
		Remotes:     remotes,
		KeepAlive:   t.config.KeepAlive,
	}

	if t.config.TLSCert != "" {
		chiselConfig.TLS.CA = t.config.TLSCert
	}

	client, err := chiselclient.NewClient(chiselConfig)
	if err != nil {
		return fmt.Errorf("failed to create chisel client: %w", err)
	}

	t.client = client
	t.remotes = remotes

	t.log.Info().
		Str("server", t.config.ServerURL).
		Strs("remotes", remotes).
		Msg("Connecting Chisel client")

	go func() {
		if err := client.Start(t.ctx); err != nil {
			t.log.Error().Err(err).Msg("Chisel client error")
		}
	}()

	return nil
}

// Stop stops the Chisel client.
func (t *ChiselClientTransport) Stop() error {
	if !atomic.CompareAndSwapInt32(&t.running, 1, 0) {
		return nil
	}

	if t.cancel != nil {
		t.cancel()
	}

	t.mu.Lock()
	if t.client != nil {
		t.client.Close()
		t.client = nil
	}
	t.mu.Unlock()

	t.log.Info().Msg("Chisel client stopped")
	return nil
}

// AllocatePort allocates a port for data transfer.
func (t *ChiselClientTransport) AllocatePort() (int, error) {
	return t.ports.Allocate()
}

// ReleasePort releases a previously allocated port.
func (t *ChiselClientTransport) ReleasePort(port int) {
	t.ports.Release(port)
}

// AddRemote adds a remote port forwarding specification.
// Format: "local_port:remote_host:remote_port" or "R:local_port:remote_host:remote_port" for reverse
func (t *ChiselClientTransport) AddRemote(remote string) error {
	t.mu.Lock()
	t.remotes = append(t.remotes, remote)
	remotes := make([]string, len(t.remotes))
	copy(remotes, t.remotes)
	t.mu.Unlock()

	// Reconnect with updated remotes
	return t.Connect(remotes)
}

// PortManager manages port allocation for Chisel tunnels.
type PortManager struct {
	startPort int
	endPort   int
	allocated map[int]bool
	mu        sync.Mutex
}

// NewPortManager creates a new port manager.
func NewPortManager(startPort, endPort int) *PortManager {
	return &PortManager{
		startPort: startPort,
		endPort:   endPort,
		allocated: make(map[int]bool),
	}
}

// Allocate allocates the next available port.
func (pm *PortManager) Allocate() (int, error) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	for port := pm.startPort; port <= pm.endPort; port++ {
		if !pm.allocated[port] {
			pm.allocated[port] = true
			return port, nil
		}
	}
	return 0, fmt.Errorf("no available ports in range %d-%d", pm.startPort, pm.endPort)
}

// Release releases a previously allocated port.
func (pm *PortManager) Release(port int) {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	delete(pm.allocated, port)
}

// IsAllocated checks if a port is currently allocated.
func (pm *PortManager) IsAllocated(port int) bool {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.allocated[port]
}

// ChiselDataConnection represents a data connection over Chisel.
// It wraps a local TCP connection that tunnels through Chisel.
type ChiselDataConnection struct {
	conn       net.Conn
	localPort  int
	remoteAddr string
	closed     int32
	closedCh   chan struct{}
	mu         sync.Mutex
}

// NewChiselDataConnection creates a new data connection.
func NewChiselDataConnection(conn net.Conn, localPort int, remoteAddr string) *ChiselDataConnection {
	return &ChiselDataConnection{
		conn:       conn,
		localPort:  localPort,
		remoteAddr: remoteAddr,
		closedCh:   make(chan struct{}),
	}
}

// Write writes data to the connection.
func (c *ChiselDataConnection) Write(data []byte) error {
	if atomic.LoadInt32(&c.closed) == 1 {
		return ErrConnectionClosed
	}

	_, err := c.conn.Write(data)
	return err
}

// Read reads data from the connection.
func (c *ChiselDataConnection) Read() ([]byte, error) {
	if atomic.LoadInt32(&c.closed) == 1 {
		return nil, ErrConnectionClosed
	}

	buf := make([]byte, 32768)
	n, err := c.conn.Read(buf)
	if err != nil {
		if err == io.EOF {
			return nil, ErrConnectionClosed
		}
		return nil, err
	}

	return buf[:n], nil
}

// Close closes the connection.
func (c *ChiselDataConnection) Close() error {
	if !atomic.CompareAndSwapInt32(&c.closed, 0, 1) {
		return nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	close(c.closedCh)
	return c.conn.Close()
}

// IsClosed returns true if the connection is closed.
func (c *ChiselDataConnection) IsClosed() bool {
	return atomic.LoadInt32(&c.closed) == 1
}

// ClosedChan returns a channel that is closed when the connection is closed.
func (c *ChiselDataConnection) ClosedChan() <-chan struct{} {
	return c.closedCh
}

// LocalPort returns the local port used by this connection.
func (c *ChiselDataConnection) LocalPort() int {
	return c.localPort
}

// RemoteAddr returns the remote address.
func (c *ChiselDataConnection) RemoteAddr() string {
	return c.remoteAddr
}

// Verify ChiselDataConnection implements Transport interface
var _ Transport = (*ChiselDataConnection)(nil)
