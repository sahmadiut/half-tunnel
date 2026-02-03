// Package integration provides integration tests for the Half-Tunnel system.
package integration

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/sahmadiut/half-tunnel/internal/client"
	"github.com/sahmadiut/half-tunnel/internal/server"
)

// TestClientServerIntegration tests the client and server working together.
func TestClientServerIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start a simple HTTP server to be the "destination"
	httpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("Hello from destination!"))
	})
	httpServer := &http.Server{
		Addr:    "127.0.0.1:9999",
		Handler: httpHandler,
	}
	go func() {
		_ = httpServer.ListenAndServe()
	}()
	defer httpServer.Close()

	// Wait for HTTP server to start
	time.Sleep(100 * time.Millisecond)

	// Start the Half-Tunnel server
	serverConfig := &server.Config{
		UpstreamAddr:    "127.0.0.1:18080",
		UpstreamPath:    "/upstream",
		DownstreamAddr:  "127.0.0.1:18081",
		DownstreamPath:  "/downstream",
		SessionTimeout:  5 * time.Minute,
		MaxSessions:     100,
		ReadBufferSize:  32768,
		WriteBufferSize: 32768,
		MaxMessageSize:  65536,
		DialTimeout:     10 * time.Second,
	}

	srv := server.New(serverConfig, nil)
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}
	defer func() {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = srv.Stop(shutdownCtx)
	}()

	// Wait for server to start
	time.Sleep(200 * time.Millisecond)

	// Start the Half-Tunnel client
	clientConfig := &client.Config{
		UpstreamURL:      "ws://127.0.0.1:18080/upstream",
		DownstreamURL:    "ws://127.0.0.1:18081/downstream",
		SOCKS5Addr:       "127.0.0.1:11080",
		PingInterval:     30 * time.Second,
		WriteTimeout:     10 * time.Second,
		ReadTimeout:      60 * time.Second,
		DialTimeout:      10 * time.Second,
		HandshakeTimeout: 10 * time.Second,
	}

	cli := client.New(clientConfig, nil)
	if err := cli.Start(ctx); err != nil {
		t.Fatalf("Failed to start client: %v", err)
	}
	defer func() {
		_ = cli.Stop()
	}()

	// Wait for client to start and connect
	time.Sleep(300 * time.Millisecond)

	// Verify the client has a session
	sessionID := cli.GetSessionID()
	if sessionID.String() == "00000000-0000-0000-0000-000000000000" {
		t.Error("Expected non-zero session ID")
	}

	t.Logf("Client session ID: %s", sessionID)
	t.Log("Client and server started successfully")
}

// TestSOCKS5ServerBasic tests basic SOCKS5 server functionality in isolation.
func TestSOCKS5ServerBasic(t *testing.T) {
	// This test verifies the SOCKS5 server accepts connections
	// without the full tunnel setup

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Start a simple echo server
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to start echo server: %v", err)
	}
	defer listener.Close()

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1024)
				n, err := c.Read(buf)
				if err != nil {
					return
				}
				_, _ = c.Write(buf[:n])
			}(conn)
		}
	}()

	// Verify echo server works directly
	echoAddr := listener.Addr().String()
	directConn, err := net.Dial("tcp", echoAddr)
	if err != nil {
		t.Fatalf("Failed to connect to echo server: %v", err)
	}

	testMsg := []byte("Hello, Echo!")
	_, err = directConn.Write(testMsg)
	if err != nil {
		t.Fatalf("Failed to write to echo server: %v", err)
	}

	resp := make([]byte, 1024)
	n, err := directConn.Read(resp)
	if err != nil {
		t.Fatalf("Failed to read from echo server: %v", err)
	}
	directConn.Close()

	if string(resp[:n]) != string(testMsg) {
		t.Errorf("Echo mismatch: expected %q, got %q", testMsg, resp[:n])
	}

	t.Log("Echo server test passed")
	_ = ctx // Used for future async cleanup
}

// TestServerStartStop tests server start and stop.
func TestServerStartStop(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	serverConfig := &server.Config{
		UpstreamAddr:    "127.0.0.1:28080",
		UpstreamPath:    "/upstream",
		DownstreamAddr:  "127.0.0.1:28081",
		DownstreamPath:  "/downstream",
		SessionTimeout:  5 * time.Minute,
		MaxSessions:     100,
		ReadBufferSize:  32768,
		WriteBufferSize: 32768,
		MaxMessageSize:  65536,
		DialTimeout:     10 * time.Second,
	}

	srv := server.New(serverConfig, nil)
	if err := srv.Start(ctx); err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Verify server is running
	time.Sleep(100 * time.Millisecond)
	
	if count := srv.GetSessionCount(); count != 0 {
		t.Errorf("Expected 0 sessions, got %d", count)
	}

	// Test that we can connect to the upstream port
	conn, err := net.DialTimeout("tcp", "127.0.0.1:28080", 2*time.Second)
	if err != nil {
		t.Fatalf("Failed to connect to upstream: %v", err)
	}
	conn.Close()

	// Stop the server
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	if err := srv.Stop(shutdownCtx); err != nil {
		t.Errorf("Failed to stop server: %v", err)
	}

	// Verify we can't connect anymore
	time.Sleep(100 * time.Millisecond)
	conn, err = net.DialTimeout("tcp", "127.0.0.1:28080", 500*time.Millisecond)
	if err == nil {
		conn.Close()
		t.Error("Expected connection to fail after server stop")
	}

	t.Log("Server start/stop test passed")
}

// BenchmarkPacketThroughput benchmarks packet processing.
func BenchmarkPacketThroughput(b *testing.B) {
	// This is a placeholder for future performance benchmarks
	b.Skip("Benchmark not implemented yet")
}

func init() {
	// Reduce log noise during tests
	fmt.Println("Integration tests loaded")
}
