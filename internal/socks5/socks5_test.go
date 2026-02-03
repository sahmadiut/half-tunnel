package socks5

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"
)

func TestServerListenAndServe(t *testing.T) {
	// Create a simple handler that just sends success
	handler := func(ctx context.Context, req *ConnectRequest) error {
		// Echo handler - just close immediately
		req.ClientConn.Close()
		return nil
	}

	config := &Config{
		ListenAddr: "127.0.0.1:0", // Use any available port
	}

	server := NewServer(config, handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start server in goroutine
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = server.ListenAndServe(ctx)
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Verify server is listening
	addr := server.Addr()
	if addr == nil {
		t.Fatal("Server address is nil")
	}

	// Connect to server
	conn, err := net.Dial("tcp", addr.String())
	if err != nil {
		t.Fatalf("Failed to connect to server: %v", err)
	}
	defer conn.Close()

	// Send SOCKS5 greeting (no auth)
	_, err = conn.Write([]byte{0x05, 0x01, 0x00})
	if err != nil {
		t.Fatalf("Failed to send greeting: %v", err)
	}

	// Read response
	resp := make([]byte, 2)
	_, err = io.ReadFull(conn, resp)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	if resp[0] != 0x05 || resp[1] != 0x00 {
		t.Errorf("Unexpected response: %v", resp)
	}

	// Cleanup
	cancel()
	server.Close()
	wg.Wait()
}

func TestServerWithAuth(t *testing.T) {
	handler := func(ctx context.Context, req *ConnectRequest) error {
		req.ClientConn.Close()
		return nil
	}

	config := &Config{
		ListenAddr: "127.0.0.1:0",
		Username:   "testuser",
		Password:   "testpass",
	}

	server := NewServer(config, handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = server.ListenAndServe(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	conn, err := net.Dial("tcp", server.Addr().String())
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Send greeting with username/password auth method
	_, err = conn.Write([]byte{0x05, 0x01, 0x02})
	if err != nil {
		t.Fatalf("Failed to send greeting: %v", err)
	}

	// Read response
	resp := make([]byte, 2)
	_, err = io.ReadFull(conn, resp)
	if err != nil {
		t.Fatalf("Failed to read response: %v", err)
	}

	if resp[0] != 0x05 || resp[1] != 0x02 {
		t.Errorf("Expected auth method 0x02, got %v", resp)
	}

	// Send username/password
	authReq := []byte{0x01}                                  // Version
	authReq = append(authReq, byte(len(config.Username)))    // Username length
	authReq = append(authReq, []byte(config.Username)...)    // Username
	authReq = append(authReq, byte(len(config.Password)))    // Password length
	authReq = append(authReq, []byte(config.Password)...)    // Password

	_, err = conn.Write(authReq)
	if err != nil {
		t.Fatalf("Failed to send auth: %v", err)
	}

	// Read auth response
	authResp := make([]byte, 2)
	_, err = io.ReadFull(conn, authResp)
	if err != nil {
		t.Fatalf("Failed to read auth response: %v", err)
	}

	if authResp[1] != 0x00 {
		t.Errorf("Expected auth success, got %v", authResp)
	}

	cancel()
	server.Close()
	wg.Wait()
}

func TestParseConnectRequest(t *testing.T) {
	handler := func(ctx context.Context, req *ConnectRequest) error {
		if req.DestHost != "127.0.0.1" {
			t.Errorf("Expected host 127.0.0.1, got %s", req.DestHost)
		}
		if req.DestPort != 8080 {
			t.Errorf("Expected port 8080, got %d", req.DestPort)
		}
		req.ClientConn.Close()
		return nil
	}

	config := &Config{
		ListenAddr: "127.0.0.1:0",
	}

	server := NewServer(config, handler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		_ = server.ListenAndServe(ctx)
	}()

	// Wait for server to start listening
	time.Sleep(100 * time.Millisecond)

	addr := server.Addr()
	if addr == nil {
		t.Fatal("Server address is nil")
	}

	conn, err := net.Dial("tcp", addr.String())
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Send greeting
	_, _ = conn.Write([]byte{0x05, 0x01, 0x00})

	// Read greeting response
	resp := make([]byte, 2)
	_, _ = io.ReadFull(conn, resp)

	// Send CONNECT request to 127.0.0.1:8080
	connectReq := []byte{
		0x05, // Version
		0x01, // CONNECT
		0x00, // Reserved
		0x01, // IPv4
		127, 0, 0, 1, // IP address
		0x1F, 0x90, // Port 8080 (big endian)
	}
	_, _ = conn.Write(connectReq)

	// Give the server time to process
	time.Sleep(50 * time.Millisecond)

	cancel()
	server.Close()
	wg.Wait()
}

func TestFormatDestination(t *testing.T) {
	result := FormatDestination("example.com", 443)
	expected := "example.com:443"
	if result != expected {
		t.Errorf("Expected %s, got %s", expected, result)
	}
}
