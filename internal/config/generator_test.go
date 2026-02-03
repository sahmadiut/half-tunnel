package config

import (
	"bytes"
	"strings"
	"testing"
)

func TestNewConfigGenerator(t *testing.T) {
	reader := strings.NewReader("test input\n")
	writer := &bytes.Buffer{}

	gen := NewConfigGenerator(reader, writer, true)
	if gen == nil {
		t.Error("Expected non-nil generator")
	}
	if !gen.isInteractive {
		t.Error("Expected interactive mode")
	}
}

func TestNewNonInteractiveGenerator(t *testing.T) {
	gen := NewNonInteractiveGenerator()
	if gen == nil {
		t.Error("Expected non-nil generator")
	}
	if gen.isInteractive {
		t.Error("Expected non-interactive mode")
	}
}

func TestGenerateClientConfigFromOptions(t *testing.T) {
	gen := NewNonInteractiveGenerator()

	opts := GenerateOptions{
		ClientName:    "test-client",
		UpstreamURL:   "wss://up.example.com/ws",
		DownstreamURL: "wss://down.example.com/ws",
		PortForwards:  []string{"2083", "8080:80", "8080:example.com:80"},
		SOCKS5Port:    1080,
		EnableSOCKS5:  true,
	}

	cfg, err := gen.GenerateClientConfig(opts)
	if err != nil {
		t.Fatalf("GenerateClientConfig() error = %v", err)
	}

	if cfg.Client.Name != "test-client" {
		t.Errorf("Client.Name = %s, want test-client", cfg.Client.Name)
	}
	if cfg.Client.Upstream.URL != "wss://up.example.com/ws" {
		t.Errorf("Client.Upstream.URL = %s, want wss://up.example.com/ws", cfg.Client.Upstream.URL)
	}
	if cfg.SOCKS5.ListenPort != 1080 {
		t.Errorf("SOCKS5.ListenPort = %d, want 1080", cfg.SOCKS5.ListenPort)
	}
	if len(cfg.PortForwards) != 3 {
		t.Errorf("PortForwards len = %d, want 3", len(cfg.PortForwards))
	}
}

func TestGenerateServerConfigFromOptions(t *testing.T) {
	gen := NewNonInteractiveGenerator()

	opts := GenerateOptions{
		ServerName:     "test-server",
		UpstreamPort:   9443,
		DownstreamPort: 9444,
		TLSCert:        "/path/to/cert.pem",
		TLSKey:         "/path/to/key.pem",
	}

	cfg, err := gen.GenerateServerConfig(opts)
	if err != nil {
		t.Fatalf("GenerateServerConfig() error = %v", err)
	}

	if cfg.Server.Name != "test-server" {
		t.Errorf("Server.Name = %s, want test-server", cfg.Server.Name)
	}
	if cfg.Server.Upstream.Port != 9443 {
		t.Errorf("Server.Upstream.Port = %d, want 9443", cfg.Server.Upstream.Port)
	}
	if cfg.Server.Downstream.Port != 9444 {
		t.Errorf("Server.Downstream.Port = %d, want 9444", cfg.Server.Downstream.Port)
	}
	if !cfg.Server.Upstream.TLS.Enabled {
		t.Error("Expected TLS to be enabled when cert is provided")
	}
}

func TestRenderClientConfigYAML(t *testing.T) {
	cfg := DefaultClientConfig()
	cfg.PortForwards = []interface{}{2083, map[string]interface{}{"port": 443}}

	yaml, err := RenderClientConfigYAML(cfg)
	if err != nil {
		t.Fatalf("RenderClientConfigYAML() error = %v", err)
	}

	if !strings.Contains(yaml, "client:") {
		t.Error("YAML should contain 'client:'")
	}
	if !strings.Contains(yaml, "port_forwards:") {
		t.Error("YAML should contain 'port_forwards:'")
	}
	if !strings.Contains(yaml, "socks5:") {
		t.Error("YAML should contain 'socks5:'")
	}
}

func TestRenderServerConfigYAML(t *testing.T) {
	cfg := DefaultServerConfig()

	yaml, err := RenderServerConfigYAML(cfg)
	if err != nil {
		t.Fatalf("RenderServerConfigYAML() error = %v", err)
	}

	if !strings.Contains(yaml, "server:") {
		t.Error("YAML should contain 'server:'")
	}
	if !strings.Contains(yaml, "access:") {
		t.Error("YAML should contain 'access:'")
	}
	if !strings.Contains(yaml, "tunnel:") {
		t.Error("YAML should contain 'tunnel:'")
	}
}

func TestGetSampleClientConfig(t *testing.T) {
	sample := GetSampleClientConfig()
	if sample == "" {
		t.Error("Sample client config should not be empty")
	}
	if !strings.Contains(sample, "client:") {
		t.Error("Sample should contain 'client:'")
	}
}

func TestGetSampleServerConfig(t *testing.T) {
	sample := GetSampleServerConfig()
	if sample == "" {
		t.Error("Sample server config should not be empty")
	}
	if !strings.Contains(sample, "server:") {
		t.Error("Sample should contain 'server:'")
	}
}

func TestValidateConfigFile(t *testing.T) {
	// Test with unknown type
	err := ValidateConfigFile("/nonexistent/path", "unknown")
	if err == nil {
		t.Error("Expected error for unknown config type")
	}

	// Test with non-existent file
	err = ValidateConfigFile("/nonexistent/path", "client")
	if err == nil {
		t.Error("Expected error for non-existent file")
	}
}

func TestWriteAndValidateServerConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := tmpDir + "/server-test.yml"

	cfg := DefaultServerConfig()
	err := WriteServerConfigToFile(cfg, configPath)
	if err != nil {
		t.Fatalf("WriteServerConfigToFile() error = %v", err)
	}

	err = ValidateConfigFile(configPath, "server")
	if err != nil {
		t.Errorf("ValidateConfigFile() error = %v", err)
	}
}

func TestWriteAndValidateClientConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := tmpDir + "/client-test.yml"

	cfg := DefaultClientConfig()
	err := WriteClientConfigToFile(cfg, configPath)
	if err != nil {
		t.Fatalf("WriteClientConfigToFile() error = %v", err)
	}

	err = ValidateConfigFile(configPath, "client")
	if err != nil {
		t.Errorf("ValidateConfigFile() error = %v", err)
	}
}

func TestGenerateClientConfigInteractive(t *testing.T) {
	// Simulate interactive input
	input := `test-client
wss://up.example.com/ws
wss://down.example.com/ws
Y

Y
2083
Y
n
Y
1080
`
	reader := strings.NewReader(input)
	writer := &bytes.Buffer{}

	gen := NewConfigGenerator(reader, writer, true)
	cfg, err := gen.GenerateClientConfig(GenerateOptions{})
	if err != nil {
		t.Fatalf("GenerateClientConfig() error = %v", err)
	}

	if cfg.Client.Name != "test-client" {
		t.Errorf("Client.Name = %s, want test-client", cfg.Client.Name)
	}
	if cfg.Client.Upstream.URL != "wss://up.example.com/ws" {
		t.Errorf("Client.Upstream.URL = %s, want wss://up.example.com/ws", cfg.Client.Upstream.URL)
	}
}

func TestGenerateServerConfigInteractive(t *testing.T) {
	// Simulate interactive input
	input := `test-server
9443
9444
n
1000
`
	reader := strings.NewReader(input)
	writer := &bytes.Buffer{}

	gen := NewConfigGenerator(reader, writer, true)
	cfg, err := gen.GenerateServerConfig(GenerateOptions{})
	if err != nil {
		t.Fatalf("GenerateServerConfig() error = %v", err)
	}

	if cfg.Server.Name != "test-server" {
		t.Errorf("Server.Name = %s, want test-server", cfg.Server.Name)
	}
	if cfg.Server.Upstream.Port != 9443 {
		t.Errorf("Server.Upstream.Port = %d, want 9443", cfg.Server.Upstream.Port)
	}
}
