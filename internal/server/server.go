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
	"github.com/sahmadiut/half-tunnel/internal/constants"
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
	// UpstreamTLS holds TLS settings for upstream server
	UpstreamTLS TLSConfig
	// DownstreamAddr is the address to listen for downstream connections (Domain B)
	DownstreamAddr string
	// DownstreamPath is the WebSocket path for downstream connections
	DownstreamPath string
	// DownstreamTLS holds TLS settings for downstream server
	DownstreamTLS TLSConfig
	// Session settings
	SessionTimeout time.Duration
	MaxSessions    int
	// Connection settings
	ReadBufferSize  int
	WriteBufferSize int
	MaxMessageSize  int
	DialTimeout     time.Duration
}

// TLSConfig holds TLS certificate settings.
type TLSConfig struct {
	Enabled  bool
	CertFile string
	KeyFile  string
}

// DefaultConfig returns default server configuration.
func DefaultConfig() *Config {
	return &Config{
		UpstreamAddr:    ":8080",
		UpstreamPath:    "/upstream",
		DownstreamAddr:  ":8081",
		DownstreamPath:  "/downstream",
		UpstreamTLS:     TLSConfig{},
		DownstreamTLS:   TLSConfig{},
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

	// Connection metrics
	metrics   ConnectionMetrics
	metricsMu sync.RWMutex

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

// ConnectionMetrics holds metrics for monitoring data transfer.
type ConnectionMetrics struct {
	BytesSent       int64
	BytesReceived   int64
	PacketsSent     int64
	PacketsReceived int64
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
		ReadBufferSize:   s.config.ReadBufferSize,
		WriteBufferSize:  s.config.WriteBufferSize,
		MaxMessageSize:   int64(s.config.MaxMessageSize),
		HandshakeTimeout: s.config.DialTimeout,
	}

	// Create upstream handler
	s.upstreamHandler = transport.NewServerHandler(transportConfig, s.log.WithStr("direction", "upstream"))

	// Create downstream handler
	s.downstreamHandler = transport.NewServerHandler(transportConfig, s.log.WithStr("direction", "downstream"))

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
		if s.config.UpstreamTLS.Enabled {
			s.log.Info().
				Str("addr", s.config.UpstreamAddr).
				Bool("tls", true).
				Str("cert_file", s.config.UpstreamTLS.CertFile).
				Msg("Starting upstream server with TLS")
			if err := s.upstreamServer.ListenAndServeTLS(s.config.UpstreamTLS.CertFile, s.config.UpstreamTLS.KeyFile); err != nil && err != http.ErrServerClosed {
				s.log.Error().Err(err).Msg("Upstream server error")
			}
			return
		}
		s.log.Info().Str("addr", s.config.UpstreamAddr).Bool("tls", false).Msg("Starting upstream server")
		if err := s.upstreamServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Error().Err(err).Msg("Upstream server error")
		}
	}()

	// Start downstream server
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		if s.config.DownstreamTLS.Enabled {
			s.log.Info().
				Str("addr", s.config.DownstreamAddr).
				Bool("tls", true).
				Str("cert_file", s.config.DownstreamTLS.CertFile).
				Msg("Starting downstream server with TLS")
			if err := s.downstreamServer.ListenAndServeTLS(s.config.DownstreamTLS.CertFile, s.config.DownstreamTLS.KeyFile); err != nil && err != http.ErrServerClosed {
				s.log.Error().Err(err).Msg("Downstream server error")
			}
			return
		}
		s.log.Info().Str("addr", s.config.DownstreamAddr).Bool("tls", false).Msg("Starting downstream server")
		if err := s.downstreamServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			s.log.Error().Err(err).Msg("Downstream server error")
		}
	}()

	// Start connection handlers
	s.wg.Add(1)
	go s.handleUpstreamConnections(ctx)

	s.wg.Add(1)
	go s.handleDownstreamConnections(ctx)

	// Start periodic metrics logging
	s.wg.Add(1)
	go s.logMetricsPeriodically(ctx)

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
	s.log.Info().
		Str("remote_addr", conn.RemoteAddr()).
		Msg("Upstream connection established")

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

		// Record received packet metrics
		s.recordPacketReceived(int64(len(data)))

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
		s.log.Debug().Err(err).
			Str("remote_addr", conn.RemoteAddr()).
			Msg("Failed to read initial downstream packet")
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

	s.log.Info().
		Str("session_id", pkt.SessionID.String()).
		Str("remote_addr", conn.RemoteAddr()).
		Msg("Client downstream connected")

	// Keep reading (for keep-alive, etc.)
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
			s.downstreamConnsMu.Lock()
			delete(s.downstreamConns, pkt.SessionID)
			s.downstreamConnsMu.Unlock()
			conn.Close()
			return
		}

		reply, replyErr := s.handleDownstreamPacket(pkt.SessionID, data)
		if replyErr != nil {
			s.log.Debug().Err(replyErr).Msg("Failed to handle downstream packet")
			continue
		}
		if len(reply) > 0 {
			if writeErr := conn.Write(reply); writeErr != nil {
				s.log.Debug().Err(writeErr).Msg("Failed to write downstream reply")
				return
			}
		}
	}
}

func (s *Server) handleDownstreamPacket(sessionID uuid.UUID, data []byte) ([]byte, error) {
	pkt, err := protocol.Unmarshal(data)
	if err != nil {
		return nil, err
	}

	if pkt.SessionID != sessionID {
		return nil, fmt.Errorf("downstream packet session mismatch")
	}

	if pkt.IsKeepAlive() && !pkt.IsAck() {
		ack, ackErr := protocol.NewKeepAliveAckPacket(sessionID)
		if ackErr != nil {
			return nil, ackErr
		}
		return ack.Marshal()
	}

	return nil, nil
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

	if pkt.IsHandshake() && pkt.StreamID == 0 {
		s.log.Info().
			Str("session_id", pkt.SessionID.String()).
			Msg("Client upstream handshake received")
	}

	if pkt.IsKeepAlive() {
		if !pkt.IsAck() {
			_ = s.sendDownstreamPacket(pkt.SessionID, pkt.StreamID, protocol.FlagKeepAlive|protocol.FlagAck, nil)
		}
		return
	}

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
			Str("dest_addr", destAddr).
			Uint32("stream_id", pkt.StreamID).
			Msg("Connecting to destination")

		conn, err := net.DialTimeout("tcp", destAddr, s.config.DialTimeout)
		if err != nil {
			s.log.Error().Err(err).Str("dest_addr", destAddr).Msg("Failed to connect to destination")
			// Send FIN packet back
			_ = s.sendDownstreamPacket(pkt.SessionID, pkt.StreamID, protocol.FlagFin, nil)
			return
		}

		s.log.Debug().
			Str("session_id", pkt.SessionID.String()).
			Uint32("stream_id", pkt.StreamID).
			Str("dest_addr", destAddr).
			Msg("Stream opened")

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
		s.log.Debug().
			Uint32("stream_id", pkt.StreamID).
			Int("bytes", len(pkt.Payload)).
			Str("direction", "to_dest").
			Msg("Data transfer")

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

	buf := make([]byte, constants.DefaultBufferSize)

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
			s.log.Debug().
				Uint32("stream_id", streamID).
				Int("bytes", n).
				Str("direction", "from_dest").
				Msg("Data transfer")

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

	// Record sent packet metrics
	s.recordPacketSent(int64(len(data)))

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
		s.log.Debug().
			Str("session_id", sessionID.String()).
			Uint32("stream_id", streamID).
			Msg("Stream closed")
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

// logMetricsPeriodically logs connection metrics every 30 seconds.
func (s *Server) logMetricsPeriodically(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-s.shutdown:
			return
		case <-ticker.C:
			s.logMetrics()
		}
	}
}

// logMetrics logs current connection metrics.
func (s *Server) logMetrics() {
	s.metricsMu.RLock()
	bytesSent := s.metrics.BytesSent
	bytesReceived := s.metrics.BytesReceived
	packetsSent := s.metrics.PacketsSent
	packetsReceived := s.metrics.PacketsReceived
	s.metricsMu.RUnlock()

	activeStreams := s.GetNatEntryCount()
	activeSessions := s.GetSessionCount()

	s.log.Info().
		Int64("bytes_sent", bytesSent).
		Int64("bytes_received", bytesReceived).
		Int64("packets_sent", packetsSent).
		Int64("packets_received", packetsReceived).
		Int("active_streams", activeStreams).
		Int("active_sessions", activeSessions).
		Msg("Connection metrics")
}

// recordPacketReceived increments the packets received counter.
func (s *Server) recordPacketReceived(bytes int64) {
	s.metricsMu.Lock()
	s.metrics.PacketsReceived++
	s.metrics.BytesReceived += bytes
	s.metricsMu.Unlock()
}

// recordPacketSent increments the packets sent counter.
func (s *Server) recordPacketSent(bytes int64) {
	s.metricsMu.Lock()
	s.metrics.PacketsSent++
	s.metrics.BytesSent += bytes
	s.metricsMu.Unlock()
}
