package config

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sahmadiut/half-tunnel/internal/constants"
)

func TestDefaultClientConfig(t *testing.T) {
	cfg := DefaultClientConfig()

	if cfg.Client.Name == "" {
		t.Error("Client name should have a default value")
	}
	if cfg.Client.Upstream.URL == "" {
		t.Error("Upstream URL should have a default value")
	}
	if cfg.Client.Downstream.URL == "" {
		t.Error("Downstream URL should have a default value")
	}
	if cfg.SOCKS5.ListenPort != 1080 {
		t.Errorf("Expected SOCKS5 port 1080, got %d", cfg.SOCKS5.ListenPort)
	}
	if cfg.Tunnel.Connection.ReadBufferSize != constants.LargeBufferSize {
		t.Errorf("Expected ReadBufferSize %d, got %d", constants.LargeBufferSize, cfg.Tunnel.Connection.ReadBufferSize)
	}
	if cfg.Tunnel.Connection.WriteBufferSize != constants.LargeBufferSize {
		t.Errorf("Expected WriteBufferSize %d, got %d", constants.LargeBufferSize, cfg.Tunnel.Connection.WriteBufferSize)
	}
	if cfg.Tunnel.Connection.BufferMode != "large" {
		t.Errorf("Expected BufferMode large, got %s", cfg.Tunnel.Connection.BufferMode)
	}
}

func TestClientConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*ClientConfig)
		wantErr bool
	}{
		{
			name:    "valid config",
			modify:  func(c *ClientConfig) {},
			wantErr: false,
		},
		{
			name: "missing upstream URL",
			modify: func(c *ClientConfig) {
				c.Client.Upstream.URL = ""
			},
			wantErr: true,
		},
		{
			name: "missing downstream URL",
			modify: func(c *ClientConfig) {
				c.Client.Downstream.URL = ""
			},
			wantErr: true,
		},
		{
			name: "invalid SOCKS5 port",
			modify: func(c *ClientConfig) {
				c.SOCKS5.Enabled = true
				c.SOCKS5.ListenPort = 0
			},
			wantErr: true,
		},
		{
			name: "invalid DNS port",
			modify: func(c *ClientConfig) {
				c.DNS.Enabled = true
				c.DNS.ListenPort = 70000
			},
			wantErr: true,
		},
		{
			name: "invalid encryption algorithm",
			modify: func(c *ClientConfig) {
				c.Tunnel.Encryption.Enabled = true
				c.Tunnel.Encryption.Algorithm = "invalid"
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultClientConfig()
			tt.modify(cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestParsePortForwardString(t *testing.T) {
	tests := []struct {
		name       string
		input      string
		wantListen int
		wantRemote int
		wantHost   string
		wantErr    bool
	}{
		{
			name:       "single port",
			input:      "2083",
			wantListen: 2083,
			wantRemote: 2083,
			wantHost:   "127.0.0.1",
			wantErr:    false,
		},
		{
			name:       "listen:remote",
			input:      "8080:80",
			wantListen: 8080,
			wantRemote: 80,
			wantHost:   "127.0.0.1",
			wantErr:    false,
		},
		{
			name:       "listen:host:remote",
			input:      "8080:example.com:80",
			wantListen: 8080,
			wantRemote: 80,
			wantHost:   "example.com",
			wantErr:    false,
		},
		{
			name:    "invalid port",
			input:   "abc",
			wantErr: true,
		},
		{
			name:    "too many parts",
			input:   "8080:host:80:extra",
			wantErr: true,
		},
		{
			name:       "port range first port",
			input:      "1000-1005",
			wantListen: 1000,
			wantRemote: 1000,
			wantHost:   "127.0.0.1",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pf, err := ParsePortForwardString(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePortForwardString() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if pf.ListenPort != tt.wantListen {
				t.Errorf("ListenPort = %d, want %d", pf.ListenPort, tt.wantListen)
			}
			if pf.RemotePort != tt.wantRemote {
				t.Errorf("RemotePort = %d, want %d", pf.RemotePort, tt.wantRemote)
			}
			if pf.RemoteHost != tt.wantHost {
				t.Errorf("RemoteHost = %s, want %s", pf.RemoteHost, tt.wantHost)
			}
		})
	}
}

func TestParsePortForwards(t *testing.T) {
	tests := []struct {
		name    string
		input   []interface{}
		want    int
		wantErr bool
	}{
		{
			name:    "empty",
			input:   []interface{}{},
			want:    0,
			wantErr: false,
		},
		{
			name:    "single int",
			input:   []interface{}{2083},
			want:    1,
			wantErr: false,
		},
		{
			name:    "float64 (YAML parsed)",
			input:   []interface{}{float64(2083)},
			want:    1,
			wantErr: false,
		},
		{
			name:    "string",
			input:   []interface{}{"8080:80"},
			want:    1,
			wantErr: false,
		},
		{
			name: "map with port",
			input: []interface{}{
				map[string]interface{}{"port": 443},
			},
			want:    1,
			wantErr: false,
		},
		{
			name: "map with full config",
			input: []interface{}{
				map[string]interface{}{
					"name":        "web",
					"listen_host": "0.0.0.0",
					"listen_port": 8080,
					"remote_host": "example.com",
					"remote_port": 80,
					"protocol":    "tcp",
				},
			},
			want:    1,
			wantErr: false,
		},
		{
			name: "mixed formats",
			input: []interface{}{
				2083,
				"8080:80",
				map[string]interface{}{"port": 443},
			},
			want:    3,
			wantErr: false,
		},
		{
			name: "map missing port",
			input: []interface{}{
				map[string]interface{}{"name": "test"},
			},
			wantErr: true,
		},
		{
			name:    "port range",
			input:   []interface{}{"1000-1005"},
			want:    6, // 6 ports: 1000, 1001, 1002, 1003, 1004, 1005
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParsePortForwards(tt.input)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParsePortForwards() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if err != nil {
				return
			}
			if len(result) != tt.want {
				t.Errorf("ParsePortForwards() len = %d, want %d", len(result), tt.want)
			}
		})
	}
}

func TestLoadClientConfigFromFile(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "client.yml")

	configContent := `
client:
  name: "test-client"
  upstream:
    url: "wss://up.example.com/ws"
    tls:
      enabled: true
      skip_verify: false
  downstream:
    url: "wss://down.example.com/ws"
    tls:
      enabled: true
      skip_verify: false
port_forwards:
  - 2083
  - port: 443
  - listen_port: 8080
    remote_host: "example.com"
    remote_port: 80
socks5:
  enabled: true
  listen_host: "127.0.0.1"
  listen_port: 1080
logging:
  level: "debug"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg, err := LoadClientConfigFromFile(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Client.Name != "test-client" {
		t.Errorf("Expected client name 'test-client', got '%s'", cfg.Client.Name)
	}
	if !cfg.SOCKS5.Enabled {
		t.Error("Expected SOCKS5 to be enabled")
	}

	portForwards, err := cfg.GetPortForwards()
	if err != nil {
		t.Fatalf("Failed to parse port forwards: %v", err)
	}
	if len(portForwards) != 3 {
		t.Errorf("Expected 3 port forwards, got %d", len(portForwards))
	}
}

func TestLoadClientConfigFileNotFound(t *testing.T) {
	_, err := LoadClientConfigFromFile("/nonexistent/path/client.yml")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}
