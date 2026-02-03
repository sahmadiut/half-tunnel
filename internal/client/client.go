// Package client provides the Half-Tunnel entry client implementation.
package client

import (
	"context"
	"fmt"
	"io"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/sahmadiut/half-tunnel/internal/constants"
	"github.com/sahmadiut/half-tunnel/internal/mux"
	"github.com/sahmadiut/half-tunnel/internal/protocol"
	"github.com/sahmadiut/half-tunnel/internal/session"
	"github.com/sahmadiut/half-tunnel/internal/socks5"
	"github.com/sahmadiut/half-tunnel/internal/transport"
	"github.com/sahmadiut/half-tunnel/pkg/logger"
)

// Config holds client configuration.
type Config struct {
	// UpstreamURL is the WebSocket URL for the upstream connection (Domain A)
	UpstreamURL string
	// DownstreamURL is the WebSocket URL for the downstream connection (Domain B)
	DownstreamURL string
	// SOCKS5Addr is the local address to listen for SOCKS5 connections
	SOCKS5Addr string
	// SOCKS5Username and SOCKS5Password for optional authentication
	SOCKS5Username string
	SOCKS5Password string
	// Connection settings
	PingInterval     time.Duration
	WriteTimeout     time.Duration
	ReadTimeout      time.Duration
	DialTimeout      time.Duration
	HandshakeTimeout time.Duration
}

// DefaultConfig returns default client configuration.
func DefaultConfig() *Config {
	return &Config{
		UpstreamURL:      "ws://localhost:8080/upstream",
		DownstreamURL:    "ws://localhost:8081/downstream",
		SOCKS5Addr:       "127.0.0.1:1080",
		PingInterval:     30 * time.Second,
		WriteTimeout:     10 * time.Second,
		ReadTimeout:      60 * time.Second,
		DialTimeout:      10 * time.Second,
		HandshakeTimeout: 10 * time.Second,
	}
}

// Client is the Half-Tunnel entry client.
type Client struct {
	config   *Config
	log      *logger.Logger
	session  *session.Session
	mux      *mux.Multiplexer
	upstream *transport.Connection
	downstream *transport.Connection
	socks5   *socks5.Server
	
	// Stream management
	streamConns  map[uint32]*streamConn
	streamConnsMu sync.RWMutex
	
	// State
	running  int32
	shutdown chan struct{}
	wg       sync.WaitGroup
	mu       sync.RWMutex
}

// streamConn holds the connection associated with a stream.
type streamConn struct {
	conn     net.Conn
	streamID uint32
	done     chan struct{}
}

// New creates a new Half-Tunnel client.
func New(config *Config, log *logger.Logger) *Client {
	if config == nil {
		config = DefaultConfig()
	}
	if log == nil {
		log = logger.NewDefault()
	}

	return &Client{
		config:      config,
		log:         log,
		streamConns: make(map[uint32]*streamConn),
		shutdown:    make(chan struct{}),
	}
}

// Start starts the client and connects to the server.
func (c *Client) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&c.running, 0, 1) {
		return fmt.Errorf("client already running")
	}

	// Create a new session
	c.session = session.New()
	c.mux = mux.NewMultiplexer(c.session)

	c.log.Info().
		Str("session_id", c.session.ID.String()).
		Msg("Created new session")

	// Connect to upstream (Domain A)
	upstreamConfig := transport.DefaultConfig(c.config.UpstreamURL)
	upstreamConfig.HandshakeTimeout = c.config.HandshakeTimeout
	upstreamConfig.WriteTimeout = c.config.WriteTimeout

	var err error
	c.upstream, err = transport.Dial(ctx, upstreamConfig)
	if err != nil {
		return fmt.Errorf("failed to connect to upstream: %w", err)
	}

	c.log.Info().
		Str("url", c.config.UpstreamURL).
		Msg("Connected to upstream")

	// Connect to downstream (Domain B)
	downstreamConfig := transport.DefaultConfig(c.config.DownstreamURL)
	downstreamConfig.HandshakeTimeout = c.config.HandshakeTimeout
	downstreamConfig.ReadTimeout = c.config.ReadTimeout

	c.downstream, err = transport.Dial(ctx, downstreamConfig)
	if err != nil {
		c.upstream.Close()
		return fmt.Errorf("failed to connect to downstream: %w", err)
	}

	c.log.Info().
		Str("url", c.config.DownstreamURL).
		Msg("Connected to downstream")

	// Send handshake packet
	if err := c.sendHandshake(); err != nil {
		c.cleanup()
		return fmt.Errorf("failed to send handshake: %w", err)
	}

	// Set packet handler for sending through upstream
	c.mux.SetPacketHandler(c.sendPacket)

	// Start downstream reader goroutine
	c.wg.Add(1)
	go c.readDownstream(ctx)

	// Start SOCKS5 server
	socks5Config := &socks5.Config{
		ListenAddr: c.config.SOCKS5Addr,
		Username:   c.config.SOCKS5Username,
		Password:   c.config.SOCKS5Password,
	}
	c.socks5 = socks5.NewServer(socks5Config, c.handleConnect)

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		if err := c.socks5.ListenAndServe(ctx); err != nil {
			c.log.Error().Err(err).Msg("SOCKS5 server error")
		}
	}()

	c.log.Info().
		Str("addr", c.config.SOCKS5Addr).
		Msg("SOCKS5 proxy started")

	return nil
}

// Stop stops the client gracefully.
func (c *Client) Stop() error {
	if !atomic.CompareAndSwapInt32(&c.running, 1, 0) {
		return nil
	}

	close(c.shutdown)
	c.cleanup()
	c.wg.Wait()

	c.log.Info().Msg("Client stopped")
	return nil
}

// cleanup closes all resources.
func (c *Client) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Close SOCKS5 server
	if c.socks5 != nil {
		c.socks5.Close()
	}

	// Close all stream connections
	c.streamConnsMu.Lock()
	for _, sc := range c.streamConns {
		close(sc.done)
		sc.conn.Close()
	}
	c.streamConns = make(map[uint32]*streamConn)
	c.streamConnsMu.Unlock()

	// Close multiplexer
	if c.mux != nil {
		c.mux.Close()
	}

	// Close transport connections
	if c.upstream != nil {
		c.upstream.Close()
	}
	if c.downstream != nil {
		c.downstream.Close()
	}
}

// sendHandshake sends the initial handshake packet.
func (c *Client) sendHandshake() error {
	pkt, err := protocol.NewPacket(c.session.ID, 0, protocol.FlagHandshake, nil)
	if err != nil {
		return err
	}

	data, err := pkt.Marshal()
	if err != nil {
		return err
	}

	return c.upstream.Write(data)
}

// sendPacket sends a packet through the upstream connection.
func (c *Client) sendPacket(pkt *protocol.Packet) error {
	data, err := pkt.Marshal()
	if err != nil {
		return err
	}
	return c.upstream.Write(data)
}

// readDownstream reads packets from the downstream connection.
func (c *Client) readDownstream(ctx context.Context) {
	defer c.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.shutdown:
			return
		default:
		}

		data, err := c.downstream.Read()
		if err != nil {
			if !c.downstream.IsClosed() {
				c.log.Error().Err(err).Msg("Error reading from downstream")
			}
			return
		}

		pkt, err := protocol.Unmarshal(data)
		if err != nil {
			c.log.Error().Err(err).Msg("Error unmarshaling packet")
			continue
		}

		// Handle the packet
		c.handleDownstreamPacket(pkt)
	}
}

// handleDownstreamPacket handles a packet received from downstream.
func (c *Client) handleDownstreamPacket(pkt *protocol.Packet) {
	// Verify session ID
	if pkt.SessionID != c.session.ID {
		c.log.Warn().
			Str("expected", c.session.ID.String()).
			Str("got", pkt.SessionID.String()).
			Msg("Received packet with wrong session ID")
		return
	}

	// Handle FIN packets
	if pkt.IsFin() {
		c.closeStream(pkt.StreamID)
		return
	}

	// Find the stream connection
	c.streamConnsMu.RLock()
	sc, exists := c.streamConns[pkt.StreamID]
	c.streamConnsMu.RUnlock()

	if !exists {
		c.log.Debug().
			Uint32("stream_id", pkt.StreamID).
			Msg("Received packet for unknown stream")
		return
	}

	// Write data to the client connection
	if pkt.IsData() && len(pkt.Payload) > 0 {
		if _, err := sc.conn.Write(pkt.Payload); err != nil {
			c.log.Error().Err(err).
				Uint32("stream_id", pkt.StreamID).
				Msg("Error writing to client")
			c.closeStream(pkt.StreamID)
		}
	}
}

// handleConnect handles a SOCKS5 CONNECT request.
func (c *Client) handleConnect(ctx context.Context, req *socks5.ConnectRequest) error {
	// Open a new stream
	streamID, err := c.mux.OpenStream()
	if err != nil {
		_ = c.socks5.SendFailureReply(req.ClientConn, socks5.ReplyGeneralFailure)
		return err
	}

	c.log.Debug().
		Uint32("stream_id", streamID).
		Str("dest", socks5.FormatDestination(req.DestHost, req.DestPort)).
		Msg("Opening stream for CONNECT request")

	// Send connect packet to server
	connectPayload := formatConnectPayload(req.DestHost, req.DestPort)
	if err := c.mux.SendPacket(streamID, protocol.FlagData|protocol.FlagHandshake, connectPayload); err != nil {
		_ = c.mux.CloseStream(streamID)
		_ = c.socks5.SendFailureReply(req.ClientConn, socks5.ReplyGeneralFailure)
		return err
	}

	// Register the stream connection
	sc := &streamConn{
		conn:     req.ClientConn,
		streamID: streamID,
		done:     make(chan struct{}),
	}

	c.streamConnsMu.Lock()
	c.streamConns[streamID] = sc
	c.streamConnsMu.Unlock()

	// Send success reply to SOCKS5 client
	if err := c.socks5.SendSuccessReply(req.ClientConn, "0.0.0.0", 0); err != nil {
		c.closeStream(streamID)
		return err
	}

	// Start reading from client and forwarding to upstream
	go c.forwardClientToUpstream(ctx, sc)

	// Wait for the stream to complete
	<-sc.done

	return nil
}

// forwardClientToUpstream forwards data from the client to upstream.
func (c *Client) forwardClientToUpstream(ctx context.Context, sc *streamConn) {
	buf := make([]byte, constants.DefaultBufferSize)

	for {
		select {
		case <-ctx.Done():
			c.closeStream(sc.streamID)
			return
		case <-c.shutdown:
			c.closeStream(sc.streamID)
			return
		case <-sc.done:
			return
		default:
		}

		n, err := sc.conn.Read(buf)
		if err != nil {
			if err != io.EOF {
				c.log.Debug().Err(err).
					Uint32("stream_id", sc.streamID).
					Msg("Error reading from client")
			}
			// Send FIN packet
			_ = c.mux.SendPacket(sc.streamID, protocol.FlagFin, nil)
			c.closeStream(sc.streamID)
			return
		}

		if n > 0 {
			if err := c.mux.SendPacket(sc.streamID, protocol.FlagData, buf[:n]); err != nil {
				c.log.Error().Err(err).
					Uint32("stream_id", sc.streamID).
					Msg("Error sending packet")
				c.closeStream(sc.streamID)
				return
			}
		}
	}
}

// closeStream closes a stream and its associated connection.
func (c *Client) closeStream(streamID uint32) {
	c.streamConnsMu.Lock()
	sc, exists := c.streamConns[streamID]
	if exists {
		delete(c.streamConns, streamID)
	}
	c.streamConnsMu.Unlock()

	if exists {
		select {
		case <-sc.done:
			// Already closed
		default:
			close(sc.done)
		}
		sc.conn.Close()
	}

	_ = c.mux.CloseStream(streamID)
}

// formatConnectPayload creates the payload for a connect request.
// Format: [1 byte address type][address][2 bytes port]
// Address type: 1 = IPv4, 3 = domain, 4 = IPv6
func formatConnectPayload(host string, port uint16) []byte {
	ip := net.ParseIP(host)
	
	var payload []byte
	if ip != nil {
		if ip4 := ip.To4(); ip4 != nil {
			// IPv4
			payload = make([]byte, 1+4+2)
			payload[0] = socks5.AddrTypeIPv4
			copy(payload[1:5], ip4)
		} else {
			// IPv6
			payload = make([]byte, 1+16+2)
			payload[0] = socks5.AddrTypeIPv6
			copy(payload[1:17], ip.To16())
		}
	} else {
		// Domain name
		payload = make([]byte, 1+1+len(host)+2)
		payload[0] = socks5.AddrTypeDomain
		payload[1] = byte(len(host))
		copy(payload[2:2+len(host)], host)
	}
	
	// Add port at the end
	portOffset := len(payload) - 2
	payload[portOffset] = byte(port >> 8)
	payload[portOffset+1] = byte(port)
	
	return payload
}

// GetSessionID returns the current session ID.
func (c *Client) GetSessionID() uuid.UUID {
	if c.session == nil {
		return uuid.Nil
	}
	return c.session.ID
}
