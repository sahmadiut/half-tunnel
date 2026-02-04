package chisel

import (
	"testing"
)

func TestNewClient(t *testing.T) {
	config := &ClientConfig{
		UpstreamURL:          "wss://upstream.example.com:8443/ws/upstream",
		DownstreamURL:        "wss://downstream.example.com:8444/ws/downstream",
		UpstreamChiselPort:   9000,
		DownstreamChiselPort: 9001,
		TargetUpstreamPort:   8443,
		TargetDownstreamPort: 8444,
	}

	client := NewClient(config, nil)
	if client == nil {
		t.Fatal("NewClient returned nil")
	}
	if client.config != config {
		t.Error("Config not set correctly")
	}
}

func TestNewServer(t *testing.T) {
	config := &ServerConfig{
		UpstreamHost:         "0.0.0.0",
		DownstreamHost:       "0.0.0.0",
		UpstreamChiselPort:   9000,
		DownstreamChiselPort: 9001,
		TargetUpstreamPort:   8443,
		TargetDownstreamPort: 8444,
	}

	server := NewServer(config, nil)
	if server == nil {
		t.Fatal("NewServer returned nil")
	}
	if server.config != config {
		t.Error("Config not set correctly")
	}
}

func TestClientGetAddrs(t *testing.T) {
	config := &ClientConfig{
		UpstreamURL:          "wss://upstream.example.com:8443/ws/upstream",
		DownstreamURL:        "wss://downstream.example.com:8444/ws/downstream",
		UpstreamChiselPort:   9000,
		DownstreamChiselPort: 9001,
		TargetUpstreamPort:   8443,
		TargetDownstreamPort: 8444,
	}

	client := NewClient(config, nil)

	upstreamAddr := client.GetUpstreamAddr()
	if upstreamAddr != "ws://127.0.0.1:9000" {
		t.Errorf("Expected upstream addr ws://127.0.0.1:9000, got %s", upstreamAddr)
	}

	downstreamAddr := client.GetDownstreamAddr()
	if downstreamAddr != "ws://127.0.0.1:9001" {
		t.Errorf("Expected downstream addr ws://127.0.0.1:9001, got %s", downstreamAddr)
	}
}

func TestExtractHost(t *testing.T) {
	tests := []struct {
		url      string
		expected string
		wantErr  bool
	}{
		{"wss://example.com:8443/ws/upstream", "example.com", false},
		{"ws://localhost:8080/path", "localhost", false},
		{"http://192.168.1.1:9000", "192.168.1.1", false},
		{"", "", true},       // Empty URL should return an error
		{"://invalid", "", true},
		{"/path/only", "", true}, // No host in URL
	}

	for _, tt := range tests {
		host, err := extractHost(tt.url)
		if (err != nil) != tt.wantErr {
			t.Errorf("extractHost(%s) error = %v, wantErr %v", tt.url, err, tt.wantErr)
			continue
		}
		if host != tt.expected {
			t.Errorf("extractHost(%s) = %s, want %s", tt.url, host, tt.expected)
		}
	}
}

func TestIsChiselAvailable(t *testing.T) {
	// This test just checks the function doesn't panic
	// The result depends on whether chisel is installed
	_ = IsChiselAvailable()
}

func TestClientStopWhenNotRunning(t *testing.T) {
	config := &ClientConfig{
		UpstreamURL:          "wss://upstream.example.com:8443/ws/upstream",
		DownstreamURL:        "wss://downstream.example.com:8444/ws/downstream",
		UpstreamChiselPort:   9000,
		DownstreamChiselPort: 9001,
		TargetUpstreamPort:   8443,
		TargetDownstreamPort: 8444,
	}

	client := NewClient(config, nil)
	// Stopping when not running should not error
	err := client.Stop()
	if err != nil {
		t.Errorf("Stop when not running should not error: %v", err)
	}
}

func TestServerStopWhenNotRunning(t *testing.T) {
	config := &ServerConfig{
		UpstreamHost:         "0.0.0.0",
		DownstreamHost:       "0.0.0.0",
		UpstreamChiselPort:   9000,
		DownstreamChiselPort: 9001,
		TargetUpstreamPort:   8443,
		TargetDownstreamPort: 8444,
	}

	server := NewServer(config, nil)
	// Stopping when not running should not error
	err := server.Stop()
	if err != nil {
		t.Errorf("Stop when not running should not error: %v", err)
	}
}
