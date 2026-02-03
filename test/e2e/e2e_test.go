// Package e2e provides end-to-end tests for the Half-Tunnel system.
// These tests verify the complete split-path data flow.
package e2e

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/sahmadiut/half-tunnel/internal/client"
	"github.com/sahmadiut/half-tunnel/internal/server"
	"golang.org/x/net/proxy"
)

// TestEndToEndHTTPRequest tests a complete HTTP request through the tunnel.
func TestEndToEndHTTPRequest(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Start a mock HTTP server as the destination
	expectedResponse := "Hello from the destination server!"
	httpHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(expectedResponse))
	})

	httpListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create HTTP listener: %v", err)
	}
	httpServer := &http.Server{Handler: httpHandler}
	go func() {
		_ = httpServer.Serve(httpListener)
	}()
	defer httpServer.Close()

	httpAddr := httpListener.Addr().String()
	t.Logf("Mock HTTP server running at %s", httpAddr)

	// Start the Half-Tunnel server
	serverConfig := &server.Config{
		UpstreamAddr:    "127.0.0.1:38080",
		UpstreamPath:    "/upstream",
		DownstreamAddr:  "127.0.0.1:38081",
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

	// Wait for server to be ready
	time.Sleep(200 * time.Millisecond)

	// Start the Half-Tunnel client
	clientConfig := &client.Config{
		UpstreamURL:      "ws://127.0.0.1:38080/upstream",
		DownstreamURL:    "ws://127.0.0.1:38081/downstream",
		SOCKS5Addr:       "127.0.0.1:31080",
		SOCKS5Enabled:    true,
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

	// Wait for client to connect
	time.Sleep(500 * time.Millisecond)

	// Create a SOCKS5 dialer
	dialer, err := proxy.SOCKS5("tcp", "127.0.0.1:31080", nil, proxy.Direct)
	if err != nil {
		t.Fatalf("Failed to create SOCKS5 dialer: %v", err)
	}

	// Create an HTTP client that uses the SOCKS5 proxy
	httpClient := &http.Client{
		Transport: &http.Transport{
			Dial: dialer.Dial,
		},
		Timeout: 10 * time.Second,
	}

	// Make an HTTP request through the tunnel
	resp, err := httpClient.Get("http://" + httpAddr + "/")
	if err != nil {
		t.Fatalf("HTTP request failed: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	if string(body) != expectedResponse {
		t.Errorf("Response mismatch: expected %q, got %q", expectedResponse, string(body))
	}

	t.Logf("E2E test passed: response = %q", string(body))
}

// TestEndToEndMultipleStreams tests multiple concurrent connections through the tunnel.
func TestEndToEndMultipleStreams(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping e2e test in short mode")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Start a simple echo server
	echoListener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create echo listener: %v", err)
	}
	defer echoListener.Close()

	go func() {
		for {
			conn, err := echoListener.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				_, _ = io.Copy(c, c)
			}(conn)
		}
	}()

	echoAddr := echoListener.Addr().String()
	t.Logf("Echo server running at %s", echoAddr)

	// Start the Half-Tunnel server
	serverConfig := &server.Config{
		UpstreamAddr:    "127.0.0.1:48080",
		UpstreamPath:    "/upstream",
		DownstreamAddr:  "127.0.0.1:48081",
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

	time.Sleep(200 * time.Millisecond)

	// Start the Half-Tunnel client
	clientConfig := &client.Config{
		UpstreamURL:      "ws://127.0.0.1:48080/upstream",
		DownstreamURL:    "ws://127.0.0.1:48081/downstream",
		SOCKS5Addr:       "127.0.0.1:41080",
		SOCKS5Enabled:    true,
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

	time.Sleep(500 * time.Millisecond)

	// Create SOCKS5 dialer
	dialer, err := proxy.SOCKS5("tcp", "127.0.0.1:41080", nil, proxy.Direct)
	if err != nil {
		t.Fatalf("Failed to create SOCKS5 dialer: %v", err)
	}

	// Test multiple concurrent connections
	const numConnections = 5
	results := make(chan error, numConnections)

	for i := 0; i < numConnections; i++ {
		go func(id int) {
			conn, err := dialer.Dial("tcp", echoAddr)
			if err != nil {
				results <- err
				return
			}
			defer conn.Close()

			testData := []byte(fmt.Sprintf("Hello from connection %d", id))
			if _, err := conn.Write(testData); err != nil {
				results <- err
				return
			}

			buf := make([]byte, len(testData))
			if _, err := io.ReadFull(conn, buf); err != nil {
				results <- err
				return
			}

			if string(buf) != string(testData) {
				results <- fmt.Errorf("data mismatch: expected %q, got %q", testData, buf)
				return
			}

			results <- nil
		}(i)
	}

	// Collect results
	for i := 0; i < numConnections; i++ {
		if err := <-results; err != nil {
			t.Errorf("Connection %d failed: %v", i, err)
		}
	}

	t.Logf("Multiple streams test completed: %d connections tested", numConnections)
}
