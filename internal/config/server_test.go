package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDefaultServerConfig(t *testing.T) {
	cfg := DefaultServerConfig()

	if cfg.Server.Name == "" {
		t.Error("Server name should have a default value")
	}
	if cfg.Server.Upstream.Port != 8443 {
		t.Errorf("Expected upstream port 8443, got %d", cfg.Server.Upstream.Port)
	}
	if cfg.Server.Downstream.Port != 8444 {
		t.Errorf("Expected downstream port 8444, got %d", cfg.Server.Downstream.Port)
	}
	if cfg.Tunnel.Session.Timeout != 5*time.Minute {
		t.Errorf("Expected session timeout 5m, got %v", cfg.Tunnel.Session.Timeout)
	}
}

func TestServerConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		modify  func(*ServerConfig)
		wantErr bool
	}{
		{
			name:    "valid config",
			modify:  func(c *ServerConfig) {},
			wantErr: false,
		},
		{
			name: "invalid upstream port",
			modify: func(c *ServerConfig) {
				c.Server.Upstream.Port = 0
			},
			wantErr: true,
		},
		{
			name: "invalid downstream port",
			modify: func(c *ServerConfig) {
				c.Server.Downstream.Port = 70000
			},
			wantErr: true,
		},
		{
			name: "TLS enabled without cert",
			modify: func(c *ServerConfig) {
				c.Server.Upstream.TLS.Enabled = true
				c.Server.Upstream.TLS.CertFile = ""
			},
			wantErr: true,
		},
		{
			name: "TLS enabled without key",
			modify: func(c *ServerConfig) {
				c.Server.Upstream.TLS.Enabled = true
				c.Server.Upstream.TLS.CertFile = "/path/to/cert"
				c.Server.Upstream.TLS.KeyFile = ""
			},
			wantErr: true,
		},
		{
			name: "invalid encryption algorithm",
			modify: func(c *ServerConfig) {
				c.Tunnel.Encryption.Enabled = true
				c.Tunnel.Encryption.Algorithm = "invalid"
			},
			wantErr: true,
		},
		{
			name: "valid encryption algorithm aes-256-gcm",
			modify: func(c *ServerConfig) {
				c.Tunnel.Encryption.Enabled = true
				c.Tunnel.Encryption.Algorithm = "aes-256-gcm"
			},
			wantErr: false,
		},
		{
			name: "valid encryption algorithm chacha20-poly1305",
			modify: func(c *ServerConfig) {
				c.Tunnel.Encryption.Enabled = true
				c.Tunnel.Encryption.Algorithm = "chacha20-poly1305"
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := DefaultServerConfig()
			tt.modify(cfg)
			err := cfg.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestLoadServerConfigFromFile(t *testing.T) {
	// Create a temporary config file
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "server.yml")

	configContent := `
server:
  name: "test-server"
  upstream:
    host: "0.0.0.0"
    port: 9443
    path: "/ws/up"
    tls:
      enabled: false
  downstream:
    host: "0.0.0.0"
    port: 9444
    path: "/ws/down"
    tls:
      enabled: false
access:
  allowed_networks:
    - "0.0.0.0/0"
  max_streams_per_session: 50
tunnel:
  session:
    timeout: "10m"
    max_sessions: 500
logging:
  level: "debug"
`
	if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
		t.Fatalf("Failed to write config file: %v", err)
	}

	cfg, err := LoadServerConfigFromFile(configPath)
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Server.Name != "test-server" {
		t.Errorf("Expected server name 'test-server', got '%s'", cfg.Server.Name)
	}
	if cfg.Server.Upstream.Port != 9443 {
		t.Errorf("Expected upstream port 9443, got %d", cfg.Server.Upstream.Port)
	}
	if cfg.Access.MaxStreamsPerSession != 50 {
		t.Errorf("Expected max_streams_per_session 50, got %d", cfg.Access.MaxStreamsPerSession)
	}
}

func TestLoadServerConfigFileNotFound(t *testing.T) {
	_, err := LoadServerConfigFromFile("/nonexistent/path/server.yml")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}
