package client

import (
	"bytes"
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/sahmadiut/half-tunnel/internal/mux"
	"github.com/sahmadiut/half-tunnel/internal/protocol"
	"github.com/sahmadiut/half-tunnel/internal/session"
	"github.com/sahmadiut/half-tunnel/internal/socks5"
	"github.com/sahmadiut/half-tunnel/internal/transport"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig()

	if config.UpstreamURL != "ws://localhost:8080/upstream" {
		t.Errorf("Expected UpstreamURL ws://localhost:8080/upstream, got %s", config.UpstreamURL)
	}
	if config.DownstreamURL != "ws://localhost:8081/downstream" {
		t.Errorf("Expected DownstreamURL ws://localhost:8081/downstream, got %s", config.DownstreamURL)
	}
	if config.SOCKS5Addr != "127.0.0.1:1080" {
		t.Errorf("Expected SOCKS5Addr 127.0.0.1:1080, got %s", config.SOCKS5Addr)
	}
	if !config.ReconnectEnabled {
		t.Error("Expected reconnect enabled by default")
	}
	if config.ReconnectConfig == nil {
		t.Error("Expected reconnect config to be set")
	}
	if config.ExitOnPortInUse {
		t.Error("Expected ExitOnPortInUse to default to false")
	}
	if config.ListenOnConnect {
		t.Error("Expected ListenOnConnect to default to false")
	}
}

func TestNewClient(t *testing.T) {
	client := New(nil, nil)
	if client == nil {
		t.Fatal("Expected non-nil client")
	}
	if client.config == nil {
		t.Error("Expected non-nil config")
	}
	if client.log == nil {
		t.Error("Expected non-nil logger")
	}
}

func TestFormatConnectPayloadIPv4(t *testing.T) {
	payload := formatConnectPayload("192.168.1.1", 8080)

	if len(payload) != 7 {
		t.Fatalf("Expected payload length 7, got %d", len(payload))
	}

	if payload[0] != socks5.AddrTypeIPv4 {
		t.Errorf("Expected address type IPv4, got %d", payload[0])
	}

	ip := net.IP(payload[1:5])
	if ip.String() != "192.168.1.1" {
		t.Errorf("Expected IP 192.168.1.1, got %s", ip.String())
	}

	port := uint16(payload[5])<<8 | uint16(payload[6])
	if port != 8080 {
		t.Errorf("Expected port 8080, got %d", port)
	}
}

func TestFormatConnectPayloadIPv6(t *testing.T) {
	payload := formatConnectPayload("::1", 443)

	if len(payload) != 19 {
		t.Fatalf("Expected payload length 19, got %d", len(payload))
	}

	if payload[0] != socks5.AddrTypeIPv6 {
		t.Errorf("Expected address type IPv6, got %d", payload[0])
	}

	ip := net.IP(payload[1:17])
	if ip.String() != "::1" {
		t.Errorf("Expected IP ::1, got %s", ip.String())
	}

	port := uint16(payload[17])<<8 | uint16(payload[18])
	if port != 443 {
		t.Errorf("Expected port 443, got %d", port)
	}
}

func TestFormatConnectPayloadDomain(t *testing.T) {
	payload := formatConnectPayload("example.com", 80)

	expectedLen := 1 + 1 + len("example.com") + 2 // type + len + domain + port
	if len(payload) != expectedLen {
		t.Fatalf("Expected payload length %d, got %d", expectedLen, len(payload))
	}

	if payload[0] != socks5.AddrTypeDomain {
		t.Errorf("Expected address type Domain, got %d", payload[0])
	}

	domainLen := int(payload[1])
	if domainLen != len("example.com") {
		t.Errorf("Expected domain length %d, got %d", len("example.com"), domainLen)
	}

	domain := string(payload[2 : 2+domainLen])
	if domain != "example.com" {
		t.Errorf("Expected domain example.com, got %s", domain)
	}

	portOffset := 2 + domainLen
	port := uint16(payload[portOffset])<<8 | uint16(payload[portOffset+1])
	if port != 80 {
		t.Errorf("Expected port 80, got %d", port)
	}
}

func TestClientNotRunning(t *testing.T) {
	client := New(nil, nil)

	// Stop should not error when not running
	err := client.Stop()
	if err != nil {
		t.Errorf("Stop() on non-running client should not error: %v", err)
	}
}

func TestStartLocalListenersExitOnPortInUse(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to allocate listener: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	port := listener.Addr().(*net.TCPAddr).Port

	config := DefaultConfig()
	config.SOCKS5Enabled = false
	config.ExitOnPortInUse = true
	config.PortForwards = []PortForward{
		{
			ListenHost: "127.0.0.1",
			ListenPort: port,
			RemoteHost: "127.0.0.1",
			RemotePort: port,
		},
	}

	client := New(config, nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	if err := client.startLocalListeners(ctx); err == nil {
		t.Fatal("Expected error when port is already in use")
	}
}

func TestStartTriggersReconnectOnFailure(t *testing.T) {
	originalDial := dialTransport
	defer func() { dialTransport = originalDial }()

	dialTransport = func(ctx context.Context, config *transport.Config) (*transport.Connection, error) {
		return nil, context.DeadlineExceeded
	}

	config := DefaultConfig()
	config.SOCKS5Enabled = false
	config.PingInterval = 0
	config.ReconnectEnabled = true
	config.DialTimeout = time.Millisecond

	client := New(config, nil)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)

	if err := client.Start(ctx); err != nil {
		t.Fatalf("Start returned error: %v", err)
	}

	if client.IsConnected() {
		t.Fatal("Expected client to be disconnected after dial failure")
	}

	_ = client.Stop()
}

// mockConn is a mock net.Conn that captures written data.
type mockConn struct {
	writeBuf bytes.Buffer
	mu       sync.Mutex
}

func (c *mockConn) Read(b []byte) (n int, err error)   { return 0, nil }
func (c *mockConn) Write(b []byte) (n int, err error)  { c.mu.Lock(); defer c.mu.Unlock(); return c.writeBuf.Write(b) }
func (c *mockConn) Close() error                       { return nil }
func (c *mockConn) LocalAddr() net.Addr                { return nil }
func (c *mockConn) RemoteAddr() net.Addr               { return nil }
func (c *mockConn) SetDeadline(t time.Time) error      { return nil }
func (c *mockConn) SetReadDeadline(t time.Time) error  { return nil }
func (c *mockConn) SetWriteDeadline(t time.Time) error { return nil }

func (c *mockConn) getWrittenData() []byte {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.writeBuf.Bytes()
}

// TestHandleDownstreamPacketOutOfOrder verifies that packets arriving out of order
// are reassembled correctly before being written to the client connection.
func TestHandleDownstreamPacketOutOfOrder(t *testing.T) {
	config := DefaultConfig()
	config.SOCKS5Enabled = false
	config.ReconnectEnabled = false

	client := New(config, nil)

	// Set up session and multiplexer
	client.session = session.New()
	client.mux = mux.NewMultiplexer(client.session)
	client.dataFlowMonitor = NewDataFlowMonitor(config.DataFlowMonitor, client.log)

	// Create a mock connection to capture written data
	mockClientConn := &mockConn{}

	// Open a stream and register the mock connection
	streamID, err := client.mux.OpenStream()
	if err != nil {
		t.Fatalf("Failed to open stream: %v", err)
	}

	sc := &streamConn{
		conn:     mockClientConn,
		streamID: streamID,
		done:     make(chan struct{}),
	}

	client.streamConns = map[uint32]*streamConn{
		streamID: sc,
	}

	// Create packets with data that arrives out of order
	// Packet 2 arrives first, then Packet 0, then Packet 1
	pkt0, _ := protocol.NewPacket(client.session.ID, streamID, protocol.FlagData, []byte("AAA"))
	pkt0.SeqNum = 0

	pkt1, _ := protocol.NewPacket(client.session.ID, streamID, protocol.FlagData, []byte("BBB"))
	pkt1.SeqNum = 1

	pkt2, _ := protocol.NewPacket(client.session.ID, streamID, protocol.FlagData, []byte("CCC"))
	pkt2.SeqNum = 2

	// Simulate packets arriving out of order: 2, 0, 1
	// Packet 2 arrives first - should NOT be written yet (waiting for 0)
	client.handleDownstreamPacket(pkt2)
	data := mockClientConn.getWrittenData()
	if len(data) != 0 {
		t.Errorf("Expected no data written after packet 2 (out of order), got %d bytes: %s", len(data), string(data))
	}

	// Packet 0 arrives - should trigger flush of packet 0 only
	client.handleDownstreamPacket(pkt0)
	data = mockClientConn.getWrittenData()
	if string(data) != "AAA" {
		t.Errorf("Expected 'AAA' after packet 0, got '%s'", string(data))
	}

	// Packet 1 arrives - should flush packet 1 and then packet 2 (which was buffered)
	client.handleDownstreamPacket(pkt1)
	data = mockClientConn.getWrittenData()
	if string(data) != "AAABBBCCC" {
		t.Errorf("Expected 'AAABBBCCC' after all packets, got '%s'", string(data))
	}
}

// TestHandleDownstreamPacketInOrder verifies that packets arriving in order
// are written immediately to the client connection.
func TestHandleDownstreamPacketInOrder(t *testing.T) {
	config := DefaultConfig()
	config.SOCKS5Enabled = false
	config.ReconnectEnabled = false

	client := New(config, nil)

	// Set up session and multiplexer
	client.session = session.New()
	client.mux = mux.NewMultiplexer(client.session)
	client.dataFlowMonitor = NewDataFlowMonitor(config.DataFlowMonitor, client.log)

	// Create a mock connection to capture written data
	mockClientConn := &mockConn{}

	// Open a stream and register the mock connection
	streamID, err := client.mux.OpenStream()
	if err != nil {
		t.Fatalf("Failed to open stream: %v", err)
	}

	sc := &streamConn{
		conn:     mockClientConn,
		streamID: streamID,
		done:     make(chan struct{}),
	}

	client.streamConns = map[uint32]*streamConn{
		streamID: sc,
	}

	// Create packets that arrive in order
	pkt0, _ := protocol.NewPacket(client.session.ID, streamID, protocol.FlagData, []byte("First"))
	pkt0.SeqNum = 0

	pkt1, _ := protocol.NewPacket(client.session.ID, streamID, protocol.FlagData, []byte("Second"))
	pkt1.SeqNum = 1

	pkt2, _ := protocol.NewPacket(client.session.ID, streamID, protocol.FlagData, []byte("Third"))
	pkt2.SeqNum = 2

	// Process packets in order
	client.handleDownstreamPacket(pkt0)
	data := mockClientConn.getWrittenData()
	if string(data) != "First" {
		t.Errorf("Expected 'First' after packet 0, got '%s'", string(data))
	}

	client.handleDownstreamPacket(pkt1)
	data = mockClientConn.getWrittenData()
	if string(data) != "FirstSecond" {
		t.Errorf("Expected 'FirstSecond' after packet 1, got '%s'", string(data))
	}

	client.handleDownstreamPacket(pkt2)
	data = mockClientConn.getWrittenData()
	if string(data) != "FirstSecondThird" {
		t.Errorf("Expected 'FirstSecondThird' after packet 2, got '%s'", string(data))
	}
}
