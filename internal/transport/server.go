// Package transport provides WebSocket connection managers for the Half-Tunnel system.
package transport

import (
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sahmadiut/half-tunnel/internal/constants"
	"github.com/sahmadiut/half-tunnel/pkg/logger"
)

// ServerConfig holds server transport configuration.
type ServerConfig struct {
	ReadBufferSize    int
	WriteBufferSize   int
	MaxMessageSize    int64
	ChannelBufferSize int // Buffer size for connection channel
	HandshakeTimeout  time.Duration
}

// DefaultServerConfig returns a ServerConfig with sensible defaults.
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		ReadBufferSize:    constants.DefaultBufferSize,
		WriteBufferSize:   constants.DefaultBufferSize,
		MaxMessageSize:    1024 * 1024, // 1MB
		ChannelBufferSize: constants.DefaultChannelBufferSize,
		HandshakeTimeout:  10 * time.Second,
	}
}

// ServerHandler handles WebSocket upgrades for the server.
type ServerHandler struct {
	upgrader websocket.Upgrader
	config   *ServerConfig
	connCh   chan *Connection
	closeCh  chan struct{} // closed when handler is shutting down
	mu       sync.RWMutex
	closed   bool
	log      *logger.Logger
}

// NewServerHandler creates a new server handler.
func NewServerHandler(config *ServerConfig, log *logger.Logger) *ServerHandler {
	if config == nil {
		config = DefaultServerConfig()
	}
	if log == nil {
		log = logger.NewDefault()
	}

	channelBufferSize := config.ChannelBufferSize
	if channelBufferSize <= 0 {
		channelBufferSize = constants.DefaultChannelBufferSize
	}
	handshakeTimeout := config.HandshakeTimeout
	if handshakeTimeout <= 0 {
		handshakeTimeout = 10 * time.Second
	}

	return &ServerHandler{
		upgrader: websocket.Upgrader{
			ReadBufferSize:   config.ReadBufferSize,
			WriteBufferSize:  config.WriteBufferSize,
			HandshakeTimeout: handshakeTimeout,
			CheckOrigin: func(r *http.Request) bool {
				return true // Allow all origins for tunnel connections
			},
		},
		config:  config,
		connCh:  make(chan *Connection, channelBufferSize),
		closeCh: make(chan struct{}),
		log:     log,
	}
}

// ServeHTTP upgrades HTTP connections to WebSocket.
func (h *ServerHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	closed := h.closed
	h.mu.RUnlock()

	if closed {
		http.Error(w, "server closed", http.StatusServiceUnavailable)
		return
	}

	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		h.log.Error().Err(err).
			Str("remote_addr", r.RemoteAddr).
			Str("path", r.URL.Path).
			Msg("WebSocket upgrade failed")
		http.Error(w, "websocket upgrade failed", http.StatusBadRequest)
		return
	}

	conn.SetReadLimit(h.config.MaxMessageSize)

	c := &Connection{
		conn: conn,
		config: &Config{
			MaxMessageSize: h.config.MaxMessageSize,
		},
		closedCh: make(chan struct{}),
	}

	// Non-blocking send to connection channel, or drop if closed
	select {
	case h.connCh <- c:
		h.log.Info().
			Str("remote_addr", conn.RemoteAddr().String()).
			Msg("Accepted WebSocket connection")
	case <-h.closeCh:
		// Handler is closing, close the connection
		c.Close()
		h.log.Debug().
			Str("remote_addr", conn.RemoteAddr().String()).
			Msg("Rejected connection: handler closing")
	default:
		// Channel full, close connection
		c.Close()
		h.log.Warn().
			Str("remote_addr", conn.RemoteAddr().String()).
			Int("buffer_size", cap(h.connCh)).
			Msg("Rejected connection: channel full")
	}
}

// Accept returns a channel that receives new connections.
func (h *ServerHandler) Accept() <-chan *Connection {
	return h.connCh
}

// ClosedChan returns a channel that is closed when the handler is closed.
func (h *ServerHandler) ClosedChan() <-chan struct{} {
	return h.closeCh
}

// Close closes the handler.
func (h *ServerHandler) Close() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.closed {
		return nil
	}

	h.closed = true
	close(h.closeCh) // Signal that we're closing

	// Drain and close any pending connections
	for {
		select {
		case conn := <-h.connCh:
			if conn != nil {
				conn.Close()
			}
		default:
			return nil
		}
	}
}
