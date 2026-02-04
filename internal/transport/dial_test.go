package transport

import (
	"context"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	config := DefaultConfig("ws://example.com:8080/ws")

	if config.URL != "ws://example.com:8080/ws" {
		t.Errorf("URL = %s, want ws://example.com:8080/ws", config.URL)
	}
	if config.TCPNoDelay != true {
		t.Errorf("TCPNoDelay = %v, want true", config.TCPNoDelay)
	}
	if config.ResolveIP != "" {
		t.Errorf("ResolveIP = %s, want empty string", config.ResolveIP)
	}
	if config.IPVersion != "" {
		t.Errorf("IPVersion = %s, want empty string", config.IPVersion)
	}
	if config.HandshakeTimeout != 10*time.Second {
		t.Errorf("HandshakeTimeout = %v, want 10s", config.HandshakeTimeout)
	}
}

func TestGetNetworkType(t *testing.T) {
	tests := []struct {
		ipVersion string
		expected  string
	}{
		{"4", "tcp4"},
		{"6", "tcp6"},
		{"", "tcp"},
		{"auto", "tcp"},
		{"invalid", "tcp"},
	}

	for _, tt := range tests {
		t.Run(tt.ipVersion, func(t *testing.T) {
			result := getNetworkType(tt.ipVersion)
			if result != tt.expected {
				t.Errorf("getNetworkType(%q) = %q, want %q", tt.ipVersion, result, tt.expected)
			}
		})
	}
}

func TestCreateDialer(t *testing.T) {
	config := DefaultConfig("ws://example.com:8080/ws")
	config.HandshakeTimeout = 5 * time.Second

	dialer := createDialer(config)

	if dialer.Timeout != 5*time.Second {
		t.Errorf("Timeout = %v, want 5s", dialer.Timeout)
	}
	if dialer.KeepAlive != 30*time.Second {
		t.Errorf("KeepAlive = %v, want 30s", dialer.KeepAlive)
	}
}

func TestCreateCustomDialContext(t *testing.T) {
	config := DefaultConfig("ws://example.com:8080/ws")
	config.ResolveIP = "1.2.3.4"
	config.IPVersion = "4"
	config.TCPNoDelay = true

	dialFunc := createCustomDialContext(config)
	if dialFunc == nil {
		t.Fatal("createCustomDialContext returned nil")
	}

	// We can't fully test the dial function without a real server,
	// but we can verify it doesn't panic when called with invalid address
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	// This should fail (no server listening), but shouldn't panic
	_, err := dialFunc(ctx, "tcp", "example.com:8080")
	if err == nil {
		t.Error("Expected error when dialing non-existent server")
	}
}

func TestConfigWithResolveIP(t *testing.T) {
	config := DefaultConfig("wss://example.com:443/ws")
	config.ResolveIP = "93.184.216.34"
	config.IPVersion = "4"

	if config.ResolveIP != "93.184.216.34" {
		t.Errorf("ResolveIP = %s, want 93.184.216.34", config.ResolveIP)
	}
	if config.IPVersion != "4" {
		t.Errorf("IPVersion = %s, want 4", config.IPVersion)
	}
}

func TestWriteQueueSize(t *testing.T) {
	// Verify WriteQueueSize is reduced to 256 to prevent memory pressure
	if WriteQueueSize != 256 {
		t.Errorf("WriteQueueSize = %d, want 256", WriteQueueSize)
	}
}

func TestDefaultConfigHasKeepAliveSettings(t *testing.T) {
	config := DefaultConfig("ws://example.com:8080/ws")

	// Verify PingInterval is configured for application-level keep-alive
	if config.PingInterval != 30*time.Second {
		t.Errorf("PingInterval = %v, want 30s", config.PingInterval)
	}
	if config.PongTimeout != 10*time.Second {
		t.Errorf("PongTimeout = %v, want 10s", config.PongTimeout)
	}
}
