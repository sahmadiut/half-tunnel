// Package client provides the Half-Tunnel entry client implementation.
package client

import (
	"context"
	"crypto/tls"
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
	"github.com/sahmadiut/half-tunnel/internal/retry"
	"github.com/sahmadiut/half-tunnel/internal/session"
	"github.com/sahmadiut/half-tunnel/internal/socks5"
	"github.com/sahmadiut/half-tunnel/internal/transport"
	"github.com/sahmadiut/half-tunnel/pkg/logger"
)

// PortForward defines a port forwarding rule.
type PortForward struct {
	Name       string
	ListenHost string
	ListenPort int
	RemoteHost string
	RemotePort int
}

// Config holds client configuration.
type Config struct {
	// UpstreamURL is the WebSocket URL for the upstream connection (Domain A)
	UpstreamURL string
	// DownstreamURL is the WebSocket URL for the downstream connection (Domain B)
	DownstreamURL string
	// SOCKS5Addr is the local address to listen for SOCKS5 connections
	SOCKS5Addr string
	// SOCKS5Enabled controls whether SOCKS5 proxy is started
	SOCKS5Enabled bool
	// SOCKS5Username and SOCKS5Password for optional authentication
	SOCKS5Username string
	SOCKS5Password string
	// PortForwards is the list of port forwarding rules
	PortForwards []PortForward
	// Reconnection settings
	ReconnectEnabled bool
	ReconnectConfig  *retry.Config
	// Connection settings
	PingInterval     time.Duration
	WriteTimeout     time.Duration
	ReadTimeout      time.Duration
	DialTimeout      time.Duration
	HandshakeTimeout time.Duration
	UpstreamTLS      *tls.Config
	DownstreamTLS    *tls.Config
	ReadBufferSize   int
	WriteBufferSize  int
	// Data flow monitoring settings
	DataFlowMonitor *DataFlowMonitorConfig
}

// DefaultConfig returns default client configuration.
func DefaultConfig() *Config {
	return &Config{
		UpstreamURL:      "ws://localhost:8080/upstream",
		DownstreamURL:    "ws://localhost:8081/downstream",
		SOCKS5Addr:       "127.0.0.1:1080",
		SOCKS5Enabled:    true,
		PortForwards:     []PortForward{},
		ReconnectEnabled: true,
		ReconnectConfig:  retry.DefaultConfig(),
		PingInterval:     30 * time.Second,
		WriteTimeout:     10 * time.Second,
		ReadTimeout:      60 * time.Second,
		DialTimeout:      10 * time.Second,
		HandshakeTimeout: 10 * time.Second,
		ReadBufferSize:   constants.DefaultBufferSize,
		WriteBufferSize:  constants.DefaultBufferSize,
		DataFlowMonitor:  DefaultDataFlowMonitorConfig(),
	}
}

// Client is the Half-Tunnel entry client.
type Client struct {
	config     *Config
	log        *logger.Logger
	session    *session.Session
	mux        *mux.Multiplexer
	upstream   *transport.Connection
	downstream *transport.Connection
	socks5     *socks5.Server

	// Data flow monitoring
	dataFlowMonitor *DataFlowMonitor

	// Port forward listeners
	portForwardListeners []net.Listener

	// Stream management
	streamConns   map[uint32]*streamConn
	streamConnsMu sync.RWMutex

	// Connection metrics
	metrics   ConnectionMetrics
	metricsMu sync.RWMutex

	// Log rate limiting for unknown stream warnings
	unknownStreamLogCount int64
	unknownStreamLastLog  int64 // Unix timestamp

	// State
	running          int32
	reconnecting     int32
	lastKeepAliveAck int64
	ctx              context.Context
	cancel           context.CancelFunc
	shutdown         chan struct{}
	wg               sync.WaitGroup
	mu               sync.RWMutex
}

var dialTransport = transport.Dial

// streamConn holds the connection associated with a stream.
type streamConn struct {
	conn     net.Conn
	streamID uint32
	done     chan struct{}
}

// ConnectionMetrics holds metrics for monitoring data transfer.
type ConnectionMetrics struct {
	BytesSent       int64
	BytesReceived   int64
	PacketsSent     int64
	PacketsReceived int64
}

// New creates a new Half-Tunnel client.
func New(config *Config, log *logger.Logger) *Client {
	if config == nil {
		config = DefaultConfig()
	}
	if log == nil {
		log = logger.NewDefault()
	}
	if config.ReconnectConfig == nil {
		config.ReconnectConfig = retry.DefaultConfig()
	}
	if config.ReadBufferSize <= 0 {
		config.ReadBufferSize = constants.DefaultBufferSize
	}
	if config.WriteBufferSize <= 0 {
		config.WriteBufferSize = constants.DefaultBufferSize
	}
	if config.DataFlowMonitor == nil {
		config.DataFlowMonitor = DefaultDataFlowMonitorConfig()
	}

	client := &Client{
		config:          config,
		log:             log,
		streamConns:     make(map[uint32]*streamConn),
		shutdown:        make(chan struct{}),
		dataFlowMonitor: NewDataFlowMonitor(config.DataFlowMonitor, log.WithStr("component", "dataflow")),
	}

	return client
}

// Start starts the client and connects to the server.
func (c *Client) Start(ctx context.Context) error {
	if !atomic.CompareAndSwapInt32(&c.running, 0, 1) {
		return fmt.Errorf("client already running")
	}

	ctx, cancel := context.WithCancel(ctx)
	c.ctx = ctx
	c.cancel = cancel

	// Create a new session
	c.session = session.New()
	c.mux = mux.NewMultiplexer(c.session)

	c.log.Info().
		Str("session_id", c.session.ID.String()).
		Msg("Created new session")

	// Set packet handler for sending through upstream
	c.mux.SetPacketHandler(c.sendPacket)

	if err := c.connect(ctx); err != nil {
		if c.shouldReconnect() && ctx.Err() == nil {
			c.log.Warn().Err(err).Msg("Initial connection failed, starting reconnect loop")
			c.triggerReconnect("startup")
		} else {
			cancel()
			c.cleanup()
			return err
		}
	} else {
		// Start downstream reader goroutine
		c.wg.Add(1)
		go c.readDownstream(ctx)
	}

	if c.config.PingInterval > 0 {
		c.wg.Add(1)
		go c.keepaliveLoop(ctx)
	}

	// Start SOCKS5 server if enabled
	if c.config.SOCKS5Enabled {
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
	}

	// Start port forwarding listeners
	for _, pf := range c.config.PortForwards {
		if err := c.startPortForward(ctx, pf); err != nil {
			c.log.Error().Err(err).
				Str("name", pf.Name).
				Int("listen_port", pf.ListenPort).
				Msg("Failed to start port forward")
			// Continue with other port forwards even if one fails
		}
	}

	// Start data flow monitor
	c.dataFlowMonitor.SetStallCallback(c.handleDataFlowStall)
	c.dataFlowMonitor.Start(ctx)
	c.log.Info().
		Dur("check_interval", c.config.DataFlowMonitor.CheckInterval).
		Dur("stall_threshold", c.config.DataFlowMonitor.StallThreshold).
		Msg("Data flow monitor started")

	// Start periodic metrics logging
	c.wg.Add(1)
	go c.logMetricsPeriodically(ctx)

	return nil
}

// Stop stops the client gracefully.
func (c *Client) Stop() error {
	if !atomic.CompareAndSwapInt32(&c.running, 1, 0) {
		return nil
	}

	if c.cancel != nil {
		c.cancel()
	}
	close(c.shutdown)
	c.cleanup()

	c.log.Info().Msg("Client stopped")
	return nil
}

// cleanup closes all resources.
func (c *Client) cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	atomic.StoreInt32(&c.reconnecting, 0)
	atomic.StoreInt64(&c.lastKeepAliveAck, 0)

	// Stop data flow monitor
	if c.dataFlowMonitor != nil {
		c.dataFlowMonitor.Stop()
	}

	// Close SOCKS5 server
	if c.socks5 != nil {
		c.socks5.Close()
	}

	// Close port forward listeners
	for _, listener := range c.portForwardListeners {
		listener.Close()
	}
	c.portForwardListeners = nil

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
	c.cleanupConnectionsLocked()
}

// handleDataFlowStall is called when data flow stalls.
func (c *Client) handleDataFlowStall(action StallAction) {
	switch action {
	case StallActionLog:
		// Already logged by the monitor
	case StallActionRestart:
		c.log.Warn().Msg("Triggering reconnection due to stalled data flow")
		c.triggerReconnect("dataflow-stall")
	case StallActionShutdown:
		c.log.Error().Msg("Shutting down due to stalled data flow - service will restart")
		// Stop the client - systemd will restart the service
		go func() {
			_ = c.Stop()
		}()
	}
}

// sendHandshake sends the initial handshake packet to both upstream and downstream.
func (c *Client) sendHandshake() error {
	pkt, err := protocol.NewPacket(c.session.ID, 0, protocol.FlagHandshake, nil)
	if err != nil {
		return err
	}

	data, err := pkt.Marshal()
	if err != nil {
		return err
	}

	// Send handshake to upstream
	if err := c.upstream.Write(data); err != nil {
		return fmt.Errorf("failed to send handshake to upstream: %w", err)
	}

	// Send handshake to downstream so server can register the downstream connection
	if err := c.downstream.Write(data); err != nil {
		return fmt.Errorf("failed to send handshake to downstream: %w", err)
	}

	return nil
}

// sendPacket sends a packet through the upstream connection.
func (c *Client) sendPacket(pkt *protocol.Packet) error {
	c.mu.RLock()
	upstream := c.upstream
	c.mu.RUnlock()
	if upstream == nil {
		if c.shouldReconnect() {
			c.triggerReconnect("upstream")
		}
		return transport.ErrConnectionClosed
	}
	data, err := pkt.Marshal()
	if err != nil {
		return err
	}

	// Record sent packet metrics
	c.recordPacketSent(int64(len(data)))

	if err := upstream.Write(data); err != nil {
		if c.shouldReconnect() {
			c.triggerReconnect("upstream")
		}
		return err
	}
	// Record data flow for monitoring (only count data packets, not control packets)
	if pkt.IsData() && len(pkt.Payload) > 0 {
		c.dataFlowMonitor.RecordSend(int64(len(pkt.Payload)))
	}
	return nil
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

		c.mu.RLock()
		downstream := c.downstream
		c.mu.RUnlock()
		if downstream == nil {
			if c.shouldReconnect() {
				c.triggerReconnect("downstream")
				return
			}
			return
		}

		data, err := downstream.Read()
		if err != nil {
			if !downstream.IsClosed() {
				c.log.Error().Err(err).Msg("Error reading from downstream")
			}
			if c.shouldReconnect() {
				c.triggerReconnect("downstream")
			}
			return
		}

		// Record received packet metrics
		c.recordPacketReceived(int64(len(data)))

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

	if pkt.IsKeepAlive() && pkt.IsAck() {
		c.recordKeepAliveAck()
		return
	}

	if pkt.IsKeepAlive() {
		if err := c.sendKeepAliveAck(); err != nil {
			c.log.Debug().Err(err).Msg("Failed to send keepalive ack")
		}
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
		// Rate limit this log message to prevent log spam
		c.logUnknownStreamRateLimited(pkt.StreamID)
		return
	}

	// Write data to the client connection
	if pkt.IsData() && len(pkt.Payload) > 0 {
		// Note: Debug logging for each packet is intentional for troubleshooting.
		// In production, use INFO or higher log level to avoid performance impact.
		c.log.Debug().
			Uint32("stream_id", pkt.StreamID).
			Int("bytes", len(pkt.Payload)).
			Str("direction", "from_server").
			Msg("Data transfer")

		// Record data flow for monitoring
		c.dataFlowMonitor.RecordReceive(int64(len(pkt.Payload)))

		if _, err := sc.conn.Write(pkt.Payload); err != nil {
			c.log.Error().Err(err).
				Uint32("stream_id", pkt.StreamID).
				Msg("Error writing to client")
			c.closeStream(pkt.StreamID)
		}
	}
}

// logUnknownStreamRateLimited logs unknown stream messages with rate limiting.
// Only logs once per second, with a count of suppressed messages.
func (c *Client) logUnknownStreamRateLimited(streamID uint32) {
	count := atomic.AddInt64(&c.unknownStreamLogCount, 1)
	now := time.Now().Unix()
	lastLog := atomic.LoadInt64(&c.unknownStreamLastLog)

	// Log at most once per second
	if now > lastLog && atomic.CompareAndSwapInt64(&c.unknownStreamLastLog, lastLog, now) {
		if count > 1 {
			c.log.Debug().
				Uint32("stream_id", streamID).
				Int64("suppressed_count", count-1).
				Msg("Received packets for unknown stream (rate limited)")
		} else {
			c.log.Debug().
				Uint32("stream_id", streamID).
				Msg("Received packet for unknown stream")
		}
		atomic.StoreInt64(&c.unknownStreamLogCount, 0)
	}
}

// handleConnect handles a SOCKS5 CONNECT request.
func (c *Client) handleConnect(ctx context.Context, req *socks5.ConnectRequest) error {
	if atomic.LoadInt32(&c.reconnecting) == 1 {
		_ = c.socks5.SendFailureReply(req.ClientConn, socks5.ReplyGeneralFailure)
		return fmt.Errorf("client reconnecting")
	}

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

	c.log.Debug().
		Uint32("stream_id", streamID).
		Str("dest_addr", socks5.FormatDestination(req.DestHost, req.DestPort)).
		Msg("Stream opened")

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
			// Note: Debug logging for each packet is intentional for troubleshooting.
			// In production, use INFO or higher log level to avoid performance impact.
			c.log.Debug().
				Uint32("stream_id", sc.streamID).
				Int("bytes", n).
				Str("direction", "to_server").
				Msg("Data transfer")

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
		c.log.Debug().
			Uint32("stream_id", streamID).
			Msg("Stream closed")
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

func (c *Client) closeAllStreams() {
	c.streamConnsMu.Lock()
	for _, sc := range c.streamConns {
		select {
		case <-sc.done:
		default:
			close(sc.done)
		}
		sc.conn.Close()
	}
	c.streamConns = make(map[uint32]*streamConn)
	c.streamConnsMu.Unlock()
}

func (c *Client) connect(ctx context.Context) error {
	upstreamConfig := transport.DefaultConfig(c.config.UpstreamURL)
	upstreamConfig.HandshakeTimeout = c.config.HandshakeTimeout
	upstreamConfig.WriteTimeout = c.config.WriteTimeout
	upstreamConfig.ReadTimeout = c.config.ReadTimeout
	upstreamConfig.TLSConfig = c.config.UpstreamTLS
	upstreamConfig.ReadBufferSize = c.config.ReadBufferSize
	upstreamConfig.WriteBufferSize = c.config.WriteBufferSize

	downstreamConfig := transport.DefaultConfig(c.config.DownstreamURL)
	downstreamConfig.HandshakeTimeout = c.config.HandshakeTimeout
	downstreamConfig.ReadTimeout = c.config.ReadTimeout
	downstreamConfig.WriteTimeout = c.config.WriteTimeout
	downstreamConfig.TLSConfig = c.config.DownstreamTLS
	downstreamConfig.ReadBufferSize = c.config.ReadBufferSize
	downstreamConfig.WriteBufferSize = c.config.WriteBufferSize

	upstreamCtx, upstreamCancel := c.dialContext(ctx)
	defer upstreamCancel()

	upstream, err := dialTransport(upstreamCtx, upstreamConfig)
	if err != nil {
		c.log.Error().Err(err).
			Str("url", c.config.UpstreamURL).
			Msg("Upstream dial failed")
		return fmt.Errorf("failed to connect to upstream: %w", err)
	}

	downstreamCtx, downstreamCancel := c.dialContext(ctx)
	defer downstreamCancel()

	downstream, err := dialTransport(downstreamCtx, downstreamConfig)
	if err != nil {
		c.log.Error().Err(err).
			Str("url", c.config.DownstreamURL).
			Msg("Downstream dial failed")
		upstream.Close()
		return fmt.Errorf("failed to connect to downstream: %w", err)
	}

	c.mu.Lock()
	c.cleanupConnectionsLocked()
	c.upstream = upstream
	c.downstream = downstream
	c.mu.Unlock()

	c.log.Info().
		Str("url", c.config.UpstreamURL).
		Str("remote_addr", upstream.RemoteAddr()).
		Msg("Connected to upstream")

	c.log.Info().
		Str("url", c.config.DownstreamURL).
		Str("remote_addr", downstream.RemoteAddr()).
		Msg("Connected to downstream")

	if err := c.sendHandshake(); err != nil {
		c.log.Error().Err(err).Msg("Handshake failed")
		c.cleanupConnections()
		return fmt.Errorf("failed to send handshake: %w", err)
	}

	c.recordKeepAliveAck()
	return nil
}

func (c *Client) dialContext(ctx context.Context) (context.Context, context.CancelFunc) {
	if c.config.DialTimeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, c.config.DialTimeout)
}

func (c *Client) keepaliveLoop(ctx context.Context) {
	defer c.wg.Done()

	ticker := time.NewTicker(c.config.PingInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.shutdown:
			return
		case <-ticker.C:
			if c.keepaliveExpired() {
				c.log.Warn().Msg("Keepalive ack timeout, reconnecting")
				if c.shouldReconnect() {
					c.triggerReconnect("keepalive-timeout")
				}
				continue
			}
			if err := c.sendKeepAlive(); err != nil {
				c.log.Debug().Err(err).Msg("Failed to send keepalive")
				if c.shouldReconnect() {
					c.triggerReconnect("keepalive")
				}
			}
		}
	}
}

func (c *Client) sendKeepAlive() error {
	pkt, err := protocol.NewKeepAlivePacket(c.session.ID)
	if err != nil {
		return err
	}

	return c.sendPacket(pkt)
}

func (c *Client) sendKeepAliveAck() error {
	c.mu.RLock()
	downstream := c.downstream
	c.mu.RUnlock()
	if downstream == nil {
		return transport.ErrConnectionClosed
	}

	pkt, err := protocol.NewKeepAliveAckPacket(c.session.ID)
	if err != nil {
		return err
	}

	data, err := pkt.Marshal()
	if err != nil {
		return err
	}

	return downstream.Write(data)
}

func (c *Client) recordKeepAliveAck() {
	atomic.StoreInt64(&c.lastKeepAliveAck, time.Now().UnixNano())
}

func (c *Client) keepaliveExpired() bool {
	if c.config.PingInterval <= 0 {
		return false
	}
	lastAck := atomic.LoadInt64(&c.lastKeepAliveAck)
	if lastAck == 0 {
		return false
	}
	ackTime := time.Unix(0, lastAck)
	return time.Since(ackTime) > c.config.PingInterval*2
}

func (c *Client) cleanupConnections() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cleanupConnectionsLocked()
}

func (c *Client) cleanupConnectionsLocked() {
	if c.upstream != nil {
		c.upstream.Close()
		c.upstream = nil
	}
	if c.downstream != nil {
		c.downstream.Close()
		c.downstream = nil
	}
}

func (c *Client) shouldReconnect() bool {
	return c.config.ReconnectEnabled && atomic.LoadInt32(&c.running) == 1
}

func (c *Client) triggerReconnect(source string) {
	if !atomic.CompareAndSwapInt32(&c.reconnecting, 0, 1) {
		return
	}

	ctx := c.ctx
	if ctx == nil {
		ctx = context.Background()
	}

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		defer atomic.StoreInt32(&c.reconnecting, 0)
		c.handleReconnect(ctx, source)
	}()
}

func (c *Client) handleReconnect(ctx context.Context, source string) {
	if !c.shouldReconnect() {
		return
	}

	c.log.Warn().Str("source", source).Msg("Connection lost, attempting reconnect")
	c.cleanupConnections()
	c.closeAllStreams()
	c.mux.Close()
	c.session = session.New()
	c.mux = mux.NewMultiplexer(c.session)
	c.mux.SetPacketHandler(c.sendPacket)

	retryer := retry.New(c.config.ReconnectConfig)
	for {
		if ctx.Err() != nil || atomic.LoadInt32(&c.running) == 0 {
			return
		}

		err := c.connect(ctx)
		if err == nil {
			c.log.Info().Str("session_id", c.session.ID.String()).Msg("Reconnected to server")
			c.wg.Add(1)
			go c.readDownstream(ctx)
			return
		}

		c.log.Warn().Err(err).Msg("Reconnect attempt failed")
		if waitErr := retryer.Wait(ctx); waitErr != nil {
			c.log.Error().Err(waitErr).Msg("Reconnect stopped")
			return
		}
	}
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

// startPortForward starts a listener for a port forwarding rule.
func (c *Client) startPortForward(ctx context.Context, pf PortForward) error {
	listenAddr := fmt.Sprintf("%s:%d", pf.ListenHost, pf.ListenPort)

	listener, err := net.Listen("tcp", listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen on %s: %w", listenAddr, err)
	}

	c.mu.Lock()
	c.portForwardListeners = append(c.portForwardListeners, listener)
	c.mu.Unlock()

	name := pf.Name
	if name == "" {
		name = fmt.Sprintf("port-%d", pf.ListenPort)
	}

	c.log.Info().
		Str("name", name).
		Str("listen_addr", listenAddr).
		Str("remote_host", pf.RemoteHost).
		Int("remote_port", pf.RemotePort).
		Msg("Port forward started")

	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		c.runPortForwardListener(ctx, listener, pf)
	}()

	return nil
}

// runPortForwardListener accepts connections and forwards them.
func (c *Client) runPortForwardListener(ctx context.Context, listener net.Listener, pf PortForward) {
	defer listener.Close()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.shutdown:
			return
		default:
		}

		// Set a deadline so we can check for shutdown periodically
		if tcpListener, ok := listener.(*net.TCPListener); ok {
			_ = tcpListener.SetDeadline(time.Now().Add(1 * time.Second))
		}

		conn, err := listener.Accept()
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			// Check if we're shutting down
			select {
			case <-c.shutdown:
				return
			default:
			}
			c.log.Debug().Err(err).Msg("Error accepting port forward connection")
			continue
		}

		c.wg.Add(1)
		go func() {
			defer c.wg.Done()
			c.handlePortForwardConnection(ctx, conn, pf)
		}()
	}
}

// handlePortForwardConnection handles a single port forward connection.
func (c *Client) handlePortForwardConnection(ctx context.Context, conn net.Conn, pf PortForward) {
	defer conn.Close()

	// Open a new stream
	streamID, err := c.mux.OpenStream()
	if err != nil {
		c.log.Error().Err(err).Msg("Failed to open stream for port forward")
		return
	}

	remoteHost := pf.RemoteHost
	remotePort := uint16(pf.RemotePort)

	c.log.Debug().
		Uint32("stream_id", streamID).
		Str("remote_host", remoteHost).
		Int("remote_port", int(remotePort)).
		Msg("Opening stream for port forward")

	// Send connect packet to server
	connectPayload := formatConnectPayload(remoteHost, remotePort)
	if err := c.mux.SendPacket(streamID, protocol.FlagData|protocol.FlagHandshake, connectPayload); err != nil {
		_ = c.mux.CloseStream(streamID)
		c.log.Error().Err(err).Msg("Failed to send connect packet for port forward")
		return
	}

	// Register the stream connection
	sc := &streamConn{
		conn:     conn,
		streamID: streamID,
		done:     make(chan struct{}),
	}

	c.streamConnsMu.Lock()
	c.streamConns[streamID] = sc
	c.streamConnsMu.Unlock()

	// Start reading from client and forwarding to upstream
	go c.forwardClientToUpstream(ctx, sc)

	// Wait for the stream to complete
	<-sc.done
}

// GetSessionID returns the current session ID.
func (c *Client) GetSessionID() uuid.UUID {
	if c.session == nil {
		return uuid.Nil
	}
	return c.session.ID
}

// IsConnected reports whether both upstream and downstream connections are active.
func (c *Client) IsConnected() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.upstream != nil && c.downstream != nil
}

// logMetricsPeriodically logs connection metrics every 30 seconds.
func (c *Client) logMetricsPeriodically(ctx context.Context) {
	defer c.wg.Done()

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-c.shutdown:
			return
		case <-ticker.C:
			c.logMetrics()
		}
	}
}

// logMetrics logs current connection metrics.
func (c *Client) logMetrics() {
	c.metricsMu.RLock()
	bytesSent := c.metrics.BytesSent
	bytesReceived := c.metrics.BytesReceived
	packetsSent := c.metrics.PacketsSent
	packetsReceived := c.metrics.PacketsReceived
	c.metricsMu.RUnlock()

	c.streamConnsMu.RLock()
	activeStreams := len(c.streamConns)
	c.streamConnsMu.RUnlock()

	c.log.Info().
		Int64("bytes_sent", bytesSent).
		Int64("bytes_received", bytesReceived).
		Int64("packets_sent", packetsSent).
		Int64("packets_received", packetsReceived).
		Int("active_streams", activeStreams).
		Msg("Connection metrics")
}

// recordPacketReceived increments the packets received counter.
func (c *Client) recordPacketReceived(bytes int64) {
	c.metricsMu.Lock()
	c.metrics.PacketsReceived++
	c.metrics.BytesReceived += bytes
	c.metricsMu.Unlock()
}

// recordPacketSent increments the packets sent counter.
func (c *Client) recordPacketSent(bytes int64) {
	c.metricsMu.Lock()
	c.metrics.PacketsSent++
	c.metrics.BytesSent += bytes
	c.metricsMu.Unlock()
}
