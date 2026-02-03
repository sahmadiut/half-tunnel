// Package server provides the Half-Tunnel exit server implementation.
package server

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/sahmadiut/half-tunnel/internal/protocol"
	"github.com/sahmadiut/half-tunnel/internal/session"
	"github.com/sahmadiut/half-tunnel/internal/socks5"
	"github.com/sahmadiut/half-tunnel/internal/transport"
	"github.com/sahmadiut/half-tunnel/pkg/logger"
)

// Config holds server configuration.
type Config struct {
	// UpstreamAddr is the address to listen for upstream connections (Domain A)
	UpstreamAddr string
	// UpstreamPath is the WebSocket path for upstream connections
	UpstreamPath string
	// DownstreamAddr is the address to listen for downstream connections (Domain B)
	DownstreamAddr string
	// DownstreamPath is the WebSocket path for downstream connections
	DownstreamPath string
	// Session settings
	SessionTimeout time.Duration
	MaxSessions    int
	// Connection settings
	ReadBufferSize  int
	WriteBufferSize int
	MaxMessageSize  int
	DialTimeout     time.Duration
}

// DefaultConfig returns default server configuration.
func DefaultConfig() *Config {
	return &Config{
		UpstreamAddr:    ":8080",
		UpstreamPath:    "/upstream",
		DownstreamAddr:  ":8081",
		DownstreamPath:  "/downstream",
		SessionTimeout:  5 * time.Minute,
		MaxSessions:     1000,
		ReadBufferSize:  32768,
		WriteBufferSize: 32768,
		MaxMessageSize:  65536,
		DialTimeout:     10 * time.Second,
	}
}

// Server is the Half-Tunnel exit server.
type Server struct {
	config       *Config
	log          *logger.Logger
	sessionStore *session.Store
	
	// Transport handlers
	upstreamHandler   *transport.ServerHandler
	downstreamHandler *transport.ServerHandler
	
	// HTTP servers
	upstreamServer   *http.Server
	downstreamServer *http.Server
	
	// Session to downstream connection mapping
	downstreamConns   map[uuid.UUID]*transport.Connection
	downstreamConnsMu sync.RWMutex
	
	// Stream to destination connection mapping (NAT table)
	natTable   map[natKey]*natEntry
	natTableMu sync.RWMutex
	
	// State
	running  int32
	shutdown chan struct{}
	wg       sync.WaitGroup
}

// natKey uniquely identifies a stream within a session.
type natKey struct {
	SessionID uuid.UUID
	StreamID  uint32
}

// natEntry holds the destination connection for a stream.
type natEntry struct {
	conn     net.Conn
	destAddr string
	created  time.Time
}

// New creates a new Half-Tunnel server.
func New(config *Config, log *logger.Logger) *Server {
	if config == nil {
		config = DefaultConfig()
	}
	if log == nil {
		log = logger.NewDefault()
	}

	return &Server{
		config:          config,
		log:             log,
		sessionStore:    session.NewStore(config.SessionTimeout),
		downstreamConns: make(map[uuid.UUID]*transport.Connection),
		natTable:        make(map[natKey]*natEntry),
		shutdown:        make(chan struct{}),
	}
}

// Start starts the server.
func (s *Server) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&s.running, 0, 1) {
		return fmt.Errorf("server already running")
	}

	transportConfig := &transport.ServerConfig{
		ReadBufferSize:  s.config.ReadBufferSize,
		WriteBufferSize: s.config.WriteBufferSize,
		MaxMessageSize:  int64(s.config.MaxMessageSize),
	}

	// Create upstream handler
	s.upstreamHandler = transport.NewServerHandler(transportConfig)

	// Create downstream handler
	s.downstreamHandler = transport.NewServerHandler(transportConfig)

	// Set up upstream HTTP server
	upstreamMux := http.NewServeMux()
	upstreamMux.Handle(s.config.UpstreamPath, s.upstreamHandler)
	s.upstreamServer = &http.Server{
		Addr:    s.config.UpstreamAddr,
		Handler: upstreamMux,
	}

	// Set up downstream HTTP server
	downstreamMux := http.NewServeMux()
	downstreamMux.Handle(s.config.DownstreamPath, s.downstreamHandler)
	s.downstreamServer = &http.Server{
		Addr:    s.config.DownstreamAddr,
		Handler: downstreamMux,
	}

	// Start upstream server
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.log.Info().Str("addr", s.config.UpstreamAddr).Msg("Starting upstream server")
		if err := s.upstreamServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Error().Err(err).Msg("Upstream server error")
		}
	}()

	// Start downstream server
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.log.Info().Str("addr", s.config.DownstreamAddr).Msg("Starting downstream server")
		if err := s.downstreamServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Error().Err(err).Msg("Downstream server error")
		}
	}()

	// Start connection handlers
	s.wg.Add(1)
	go s.handleUpstreamConnections(ctx)

	s.wg.Add(1)
	go s.handleDownstreamConnections(ctx)

	return nil
}

// Stop stops the server gracefully.
func (s *Server) Stop(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&s.running, 1, 0) {
		return nil
	}

	close(s.shutdown)

	// Shutdown HTTP servers
	if s.upstreamServer != nil {
		_ = s.upstreamServer.Shutdown(ctx)
	}
	if s.downstreamServer != nil {
		_ = s.downstreamServer.Shutdown(ctx)
	}

	// Close handlers
	if s.upstreamHandler != nil {
		s.upstreamHandler.Close()
	}
	if s.downstreamHandler != nil {
		s.downstreamHandler.Close()
	}

	// Close all NAT entries
	s.natTableMu.Lock()
	for _, entry := range s.natTable {
		entry.conn.Close()
	}
	s.natTable = make(map[natKey]*natEntry)
	s.natTableMu.Unlock()

	// Close all downstream connections
	s.downstreamConnsMu.Lock()
	for _, conn := range s.downstreamConns {
		conn.Close()
	}
	s.downstreamConns = make(map[uuid.UUID]*transport.Connection)
	s.downstreamConnsMu.Unlock()

	// Close session store
	s.sessionStore.Close()

	s.wg.Wait()

	s.log.Info().Msg("Server stopped")
	return nil
}

// handleUpstreamConnections handles new upstream connections.
func (s *Server) handleUpstreamConnections(ctx context.Context) {
	defer s.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.shutdown:
			return
		case <-s.upstreamHandler.ClosedChan():
			return
		case conn := <-s.upstreamHandler.Accept():
			if conn == nil {
				continue
			}
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				s.handleUpstreamConnection(ctx, conn)
			}()
		}
	}
}

// handleDownstreamConnections handles new downstream connections.
func (s *Server) handleDownstreamConnections(ctx context.Context) {
	defer s.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.shutdown:
			return
		case <-s.downstreamHandler.ClosedChan():
			return
		case conn := <-s.downstreamHandler.Accept():
			if conn == nil {
				continue
			}
			// Downstream connections are registered when we receive a handshake
			// For now, we'll read the first packet to get the session ID
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				s.registerDownstreamConnection(ctx, conn)
			}()
		}
	}
}

// handleUpstreamConnection handles packets from an upstream connection.
func (s *Server) handleUpstreamConnection(ctx context.Context, conn *transport.Connection) {
	defer conn.Close()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.shutdown:
			return
		default:
		}

		data, err := conn.Read()
		if err != nil {
			if !conn.IsClosed() {
				s.log.Debug().Err(err).Msg("Error reading from upstream")
			}
			return
		}

		pkt, err := protocol.Unmarshal(data)
		if err != nil {
			s.log.Error().Err(err).Msg("Error unmarshaling packet")
			continue
		}

		s.handleUpstreamPacket(ctx, pkt)
	}
}

// registerDownstreamConnection reads the first packet to get session ID and registers the connection.
func (s *Server) registerDownstreamConnection(ctx context.Context, conn *transport.Connection) {
	// Read the first packet to get the session ID
	data, err := conn.Read()
	if err != nil {
		conn.Close()
		return
	}

	pkt, err := protocol.Unmarshal(data)
	if err != nil {
		s.log.Error().Err(err).Msg("Error unmarshaling initial downstream packet")
		conn.Close()
		return
	}

	// Register the downstream connection for this session
	s.downstreamConnsMu.Lock()
	s.downstreamConns[pkt.SessionID] = conn
	s.downstreamConnsMu.Unlock()

	s.log.Debug().
		Str("session_id", pkt.SessionID.String()).
		Msg("Registered downstream connection")

	// Keep reading (for keep-alive, etc.)
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.shutdown:
			return
		default:
		}

		_, err := conn.Read()
		if err != nil {
			s.downstreamConnsMu.Lock()
			delete(s.downstreamConns, pkt.SessionID)
			s.downstreamConnsMu.Unlock()
			conn.Close()
			return
		}
	}
}

// handleUpstreamPacket handles a packet received from upstream.
func (s *Server) handleUpstreamPacket(ctx context.Context, pkt *protocol.Packet) {
	// Get or create session
	sess := s.sessionStore.GetOrCreate(pkt.SessionID)

	s.log.Debug().
		Str("session_id", pkt.SessionID.String()).
		Uint32("stream_id", pkt.StreamID).
		Bool("handshake", pkt.IsHandshake()).
		Bool("data", pkt.IsData()).
		Bool("fin", pkt.IsFin()).
		Msg("Received upstream packet")

	// Handle handshake for new streams (contains destination info)
	if pkt.IsHandshake() && pkt.IsData() && len(pkt.Payload) > 0 {
		destHost, destPort, err := parseConnectPayload(pkt.Payload)
		if err != nil {
			s.log.Error().Err(err).Msg("Error parsing connect payload")
			return
		}

		// Connect to destination
		destAddr := fmt.Sprintf("%s:%d", destHost, destPort)
		s.log.Debug().
			Str("dest", destAddr).
			Uint32("stream_id", pkt.StreamID).
			Msg("Connecting to destination")

		conn, err := net.DialTimeout("tcp", destAddr, s.config.DialTimeout)
		if err != nil {
			s.log.Error().Err(err).Str("dest", destAddr).Msg("Failed to connect to destination")
			// Send FIN packet back
			_ = s.sendDownstreamPacket(pkt.SessionID, pkt.StreamID, protocol.FlagFin, nil)
			return
		}

		// Register in NAT table
		key := natKey{SessionID: pkt.SessionID, StreamID: pkt.StreamID}
		entry := &natEntry{
			conn:     conn,
			destAddr: destAddr,
			created:  time.Now(),
		}

		s.natTableMu.Lock()
		s.natTable[key] = entry
		s.natTableMu.Unlock()

		// Mark stream as active
		stream := sess.GetStream(pkt.StreamID)
		stream.SetState(session.StateActive)

		// Start forwarding responses from destination to downstream
		go s.forwardDestToDownstream(ctx, pkt.SessionID, pkt.StreamID, conn)

		return
	}

	// Handle FIN packets
	if pkt.IsFin() {
		s.closeNatEntry(pkt.SessionID, pkt.StreamID)
		return
	}

	// Handle data packets - forward to destination
	if pkt.IsData() && len(pkt.Payload) > 0 {
		key := natKey{SessionID: pkt.SessionID, StreamID: pkt.StreamID}
		s.natTableMu.RLock()
		entry, exists := s.natTable[key]
		s.natTableMu.RUnlock()

		if !exists {
			s.log.Debug().
				Uint32("stream_id", pkt.StreamID).
				Msg("No NAT entry for stream")
			return
		}

		if _, err := entry.conn.Write(pkt.Payload); err != nil {
			s.log.Error().Err(err).
				Uint32("stream_id", pkt.StreamID).
				Msg("Error writing to destination")
			s.closeNatEntry(pkt.SessionID, pkt.StreamID)
		}
	}
}

// forwardDestToDownstream forwards data from destination to downstream.
func (s *Server) forwardDestToDownstream(ctx context.Context, sessionID uuid.UUID, streamID uint32, destConn net.Conn) {
	defer s.closeNatEntry(sessionID, streamID)

	buf := make([]byte, 32768)

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.shutdown:
			return
		default:
		}

		n, err := destConn.Read(buf)
		if err != nil {
			if err != io.EOF {
				s.log.Debug().Err(err).
					Uint32("stream_id", streamID).
					Msg("Error reading from destination")
			}
			// Send FIN packet
			_ = s.sendDownstreamPacket(sessionID, streamID, protocol.FlagFin, nil)
			return
		}

		if n > 0 {
			if err := s.sendDownstreamPacket(sessionID, streamID, protocol.FlagData, buf[:n]); err != nil {
				s.log.Error().Err(err).
					Uint32("stream_id", streamID).
					Msg("Error sending downstream packet")
				return
			}
		}
	}
}

// sendDownstreamPacket sends a packet through the downstream connection.
func (s *Server) sendDownstreamPacket(sessionID uuid.UUID, streamID uint32, flags protocol.Flag, payload []byte) error {
	s.downstreamConnsMu.RLock()
	conn, exists := s.downstreamConns[sessionID]
	s.downstreamConnsMu.RUnlock()

	if !exists {
		return fmt.Errorf("no downstream connection for session %s", sessionID)
	}

	pkt, err := protocol.NewPacket(sessionID, streamID, flags, payload)
	if err != nil {
		return err
	}

	data, err := pkt.Marshal()
	if err != nil {
		return err
	}

	return conn.Write(data)
}

// closeNatEntry closes a NAT entry.
func (s *Server) closeNatEntry(sessionID uuid.UUID, streamID uint32) {
	key := natKey{SessionID: sessionID, StreamID: streamID}

	s.natTableMu.Lock()
	entry, exists := s.natTable[key]
	if exists {
		delete(s.natTable, key)
	}
	s.natTableMu.Unlock()

	if exists && entry.conn != nil {
		entry.conn.Close()
	}
}

// parseConnectPayload parses the destination from a connect packet payload.
// Format: [1 byte address type][address][2 bytes port]
func parseConnectPayload(payload []byte) (string, uint16, error) {
	if len(payload) < 3 {
		return "", 0, fmt.Errorf("payload too short")
	}

	addrType := payload[0]
	var host string
	var portOffset int

	switch addrType {
	case socks5.AddrTypeIPv4:
		if len(payload) < 7 {
			return "", 0, fmt.Errorf("payload too short for IPv4")
		}
		host = net.IP(payload[1:5]).String()
		portOffset = 5

	case socks5.AddrTypeDomain:
		domainLen := int(payload[1])
		if len(payload) < 2+domainLen+2 {
			return "", 0, fmt.Errorf("payload too short for domain")
		}
		host = string(payload[2 : 2+domainLen])
		portOffset = 2 + domainLen

	case socks5.AddrTypeIPv6:
		if len(payload) < 19 {
			return "", 0, fmt.Errorf("payload too short for IPv6")
		}
		host = net.IP(payload[1:17]).String()
		portOffset = 17

	default:
		return "", 0, fmt.Errorf("unsupported address type: %d", addrType)
	}

	port := binary.BigEndian.Uint16(payload[portOffset : portOffset+2])
	return host, port, nil
}

// GetSessionCount returns the current number of active sessions.
func (s *Server) GetSessionCount() int {
	return s.sessionStore.Count()
}

// GetNatEntryCount returns the current number of NAT entries.
func (s *Server) GetNatEntryCount() int {
	s.natTableMu.RLock()
	defer s.natTableMu.RUnlock()
	return len(s.natTable)
}
