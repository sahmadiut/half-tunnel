package transport

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sahmadiut/half-tunnel/internal/constants"
	"github.com/sahmadiut/half-tunnel/pkg/logger"
)

func TestServerHandler(t *testing.T) {
	handler := NewServerHandler(nil, logger.NewDefault())

	// Create test server
	server := httptest.NewServer(handler)
	defer server.Close()

	// Convert http URL to ws URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	// Connect to the WebSocket
	dialer := websocket.Dialer{}
	conn, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer conn.Close()

	// Accept the connection
	select {
	case c := <-handler.Accept():
		if c == nil {
			t.Fatal("Received nil connection")
		}
		// Write a test message
		err := c.Write([]byte("hello"))
		if err != nil {
			t.Errorf("Failed to write: %v", err)
		}

		// Read on the client side
		_, msg, err := conn.ReadMessage()
		if err != nil {
			t.Errorf("Failed to read: %v", err)
		}
		if string(msg) != "hello" {
			t.Errorf("Expected 'hello', got '%s'", msg)
		}

		c.Close()
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for connection")
	}

	handler.Close()
}

func TestServerHandlerClosed(t *testing.T) {
	handler := NewServerHandler(nil, logger.NewDefault())
	handler.Close()

	// Create test server
	server := httptest.NewServer(handler)
	defer server.Close()

	// Create a request
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status %d, got %d", http.StatusServiceUnavailable, w.Code)
	}
}

func TestDefaultServerConfig(t *testing.T) {
	config := DefaultServerConfig()

	if config.ReadBufferSize != constants.DefaultBufferSize {
		t.Errorf("Expected ReadBufferSize %d, got %d", constants.DefaultBufferSize, config.ReadBufferSize)
	}
	if config.WriteBufferSize != constants.DefaultBufferSize {
		t.Errorf("Expected WriteBufferSize %d, got %d", constants.DefaultBufferSize, config.WriteBufferSize)
	}
	if config.MaxMessageSize != 1024*1024 {
		t.Errorf("Expected MaxMessageSize 1MB, got %d", config.MaxMessageSize)
	}
}
