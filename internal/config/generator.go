// Package config provides configuration generation for the Half-Tunnel system.
package config

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"text/template"
)

// ConfigGenerator handles interactive and flag-based config generation.
type ConfigGenerator struct {
	reader        io.Reader
	writer        io.Writer
	isInteractive bool
}

// GenerateOptions holds options for config generation.
type GenerateOptions struct {
	// Common options
	OutputPath string

	// Server options
	UpstreamPort   int
	DownstreamPort int
	TLSCert        string
	TLSKey         string
	ServerName     string

	// Client options
	UpstreamURL   string
	DownstreamURL string
	PortForwards  []string
	SOCKS5Port    int
	ClientName    string
	EnableSOCKS5  bool
}

// NewConfigGenerator creates a new config generator.
func NewConfigGenerator(reader io.Reader, writer io.Writer, interactive bool) *ConfigGenerator {
	return &ConfigGenerator{
		reader:        reader,
		writer:        writer,
		isInteractive: interactive,
	}
}

// NewInteractiveGenerator creates a generator for interactive mode.
func NewInteractiveGenerator() *ConfigGenerator {
	return NewConfigGenerator(os.Stdin, os.Stdout, true)
}

// NewNonInteractiveGenerator creates a generator for non-interactive mode.
func NewNonInteractiveGenerator() *ConfigGenerator {
	return NewConfigGenerator(nil, os.Stdout, false)
}

// GenerateClientConfig generates a client configuration.
func (g *ConfigGenerator) GenerateClientConfig(opts GenerateOptions) (*ClientConfig, error) {
	cfg := DefaultClientConfig()

	if g.isInteractive {
		return g.generateClientConfigInteractive(cfg)
	}

	return g.generateClientConfigFromOptions(cfg, opts)
}

// GenerateServerConfig generates a server configuration.
func (g *ConfigGenerator) GenerateServerConfig(opts GenerateOptions) (*ServerConfig, error) {
	cfg := DefaultServerConfig()

	if g.isInteractive {
		return g.generateServerConfigInteractive(cfg)
	}

	return g.generateServerConfigFromOptions(cfg, opts)
}

// generateClientConfigInteractive creates client config via interactive prompts.
func (g *ConfigGenerator) generateClientConfigInteractive(cfg *ClientConfig) (*ClientConfig, error) {
	scanner := bufio.NewScanner(g.reader)

	g.printLine("\n🔧 Half-Tunnel Client Configuration Generator\n")

	// Client name
	cfg.Client.Name = g.promptWithDefault(scanner, "Enter a name for this client", cfg.Client.Name)

	// Upstream URL
	cfg.Client.Upstream.URL = g.promptWithDefault(scanner, "Upstream server URL", cfg.Client.Upstream.URL)

	// Downstream URL
	cfg.Client.Downstream.URL = g.promptWithDefault(scanner, "Downstream server URL", cfg.Client.Downstream.URL)

	// TLS verification
	if g.promptYesNo(scanner, "Enable TLS verification?", true) {
		cfg.Client.Upstream.TLS.Enabled = true
		cfg.Client.Upstream.TLS.SkipVerify = false
		cfg.Client.Downstream.TLS.Enabled = true
		cfg.Client.Downstream.TLS.SkipVerify = false

		caFile := g.promptWithDefault(scanner, "CA certificate path (optional, press Enter to skip)", "")
		if caFile != "" {
			cfg.Client.Upstream.TLS.CAFile = caFile
			cfg.Client.Downstream.TLS.CAFile = caFile
		}
	} else {
		cfg.Client.Upstream.TLS.SkipVerify = true
		cfg.Client.Downstream.TLS.SkipVerify = true
	}

	// Port forwarding
	g.printLine("\n📡 Port Forwarding Rules\n")
	var portForwards []interface{}
	for g.promptYesNo(scanner, "Add a port forward?", true) {
		portStr := g.promptWithDefault(scanner, "Port to forward", "")
		if portStr == "" {
			continue
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			g.printLine("Invalid port number, skipping...\n")
			continue
		}

		if g.promptYesNo(scanner, "Same port on remote?", true) {
			portForwards = append(portForwards, port)
		} else {
			remoteHost := g.promptWithDefault(scanner, "Remote host (leave empty for dynamic)", "")
			remotePortStr := g.promptWithDefault(scanner, "Remote port", portStr)
			remotePort, _ := strconv.Atoi(remotePortStr)

			pf := map[string]interface{}{
				"listen_port": port,
				"remote_port": remotePort,
			}
			if remoteHost != "" {
				pf["remote_host"] = remoteHost
			}
			portForwards = append(portForwards, pf)
		}
	}
	cfg.PortForwards = portForwards

	// SOCKS5
	g.printLine("\n🧦 SOCKS5 Proxy\n")
	cfg.SOCKS5.Enabled = g.promptYesNo(scanner, "Enable SOCKS5 proxy?", true)
	if cfg.SOCKS5.Enabled {
		socksPortStr := g.promptWithDefault(scanner, "SOCKS5 listen port", strconv.Itoa(cfg.SOCKS5.ListenPort))
		if port, err := strconv.Atoi(socksPortStr); err == nil {
			cfg.SOCKS5.ListenPort = port
		}
	}

	return cfg, nil
}

// generateClientConfigFromOptions creates client config from CLI options.
func (g *ConfigGenerator) generateClientConfigFromOptions(cfg *ClientConfig, opts GenerateOptions) (*ClientConfig, error) {
	if opts.ClientName != "" {
		cfg.Client.Name = opts.ClientName
	}
	if opts.UpstreamURL != "" {
		cfg.Client.Upstream.URL = opts.UpstreamURL
	}
	if opts.DownstreamURL != "" {
		cfg.Client.Downstream.URL = opts.DownstreamURL
	}

	// Parse port forwards
	var portForwards []interface{}
	for _, pf := range opts.PortForwards {
		// Check if it's a port range
		if isPortRange(pf) {
			portForwards = append(portForwards, pf)
			continue
		}
		parsed, err := ParsePortForwardString(pf)
		if err != nil {
			return nil, fmt.Errorf("invalid port forward %q: %w", pf, err)
		}
		// Convert to simple format if possible (remote_host is default 127.0.0.1)
		if parsed.RemoteHost == "127.0.0.1" && parsed.ListenPort == parsed.RemotePort {
			portForwards = append(portForwards, parsed.ListenPort)
		} else {
			pfMap := map[string]interface{}{
				"listen_port": parsed.ListenPort,
				"remote_port": parsed.RemotePort,
			}
			if parsed.RemoteHost != "" && parsed.RemoteHost != "127.0.0.1" {
				pfMap["remote_host"] = parsed.RemoteHost
			}
			portForwards = append(portForwards, pfMap)
		}
	}
	cfg.PortForwards = portForwards

	cfg.SOCKS5.Enabled = opts.EnableSOCKS5
	if opts.SOCKS5Port > 0 {
		cfg.SOCKS5.ListenPort = opts.SOCKS5Port
		cfg.SOCKS5.Enabled = true
	}

	return cfg, nil
}

// generateServerConfigInteractive creates server config via interactive prompts.
func (g *ConfigGenerator) generateServerConfigInteractive(cfg *ServerConfig) (*ServerConfig, error) {
	scanner := bufio.NewScanner(g.reader)

	g.printLine("\n🔧 Half-Tunnel Server Configuration Generator\n")

	// Server name
	cfg.Server.Name = g.promptWithDefault(scanner, "Enter a name for this server", cfg.Server.Name)

	// Upstream port
	upstreamPortStr := g.promptWithDefault(scanner, "Upstream listener port", strconv.Itoa(cfg.Server.Upstream.Port))
	if port, err := strconv.Atoi(upstreamPortStr); err == nil {
		cfg.Server.Upstream.Port = port
	}

	// Downstream port
	downstreamPortStr := g.promptWithDefault(scanner, "Downstream listener port", strconv.Itoa(cfg.Server.Downstream.Port))
	if port, err := strconv.Atoi(downstreamPortStr); err == nil {
		cfg.Server.Downstream.Port = port
	}

	// TLS
	if g.promptYesNo(scanner, "Enable TLS?", false) {
		cfg.Server.Upstream.TLS.Enabled = true
		cfg.Server.Downstream.TLS.Enabled = true

		cfg.Server.Upstream.TLS.CertFile = g.promptWithDefault(scanner, "TLS certificate path", "/etc/half-tunnel/certs/server.crt")
		cfg.Server.Upstream.TLS.KeyFile = g.promptWithDefault(scanner, "TLS key path", "/etc/half-tunnel/certs/server.key")
		cfg.Server.Downstream.TLS.CertFile = cfg.Server.Upstream.TLS.CertFile
		cfg.Server.Downstream.TLS.KeyFile = cfg.Server.Upstream.TLS.KeyFile
	}

	// Max sessions
	maxSessionsStr := g.promptWithDefault(scanner, "Maximum concurrent sessions", strconv.Itoa(cfg.Tunnel.Session.MaxSessions))
	if maxSessions, err := strconv.Atoi(maxSessionsStr); err == nil {
		cfg.Tunnel.Session.MaxSessions = maxSessions
	}

	return cfg, nil
}

// generateServerConfigFromOptions creates server config from CLI options.
func (g *ConfigGenerator) generateServerConfigFromOptions(cfg *ServerConfig, opts GenerateOptions) (*ServerConfig, error) {
	if opts.ServerName != "" {
		cfg.Server.Name = opts.ServerName
	}
	if opts.UpstreamPort > 0 {
		cfg.Server.Upstream.Port = opts.UpstreamPort
	}
	if opts.DownstreamPort > 0 {
		cfg.Server.Downstream.Port = opts.DownstreamPort
	}
	if opts.TLSCert != "" {
		cfg.Server.Upstream.TLS.Enabled = true
		cfg.Server.Upstream.TLS.CertFile = opts.TLSCert
		cfg.Server.Downstream.TLS.Enabled = true
		cfg.Server.Downstream.TLS.CertFile = opts.TLSCert
	}
	if opts.TLSKey != "" {
		cfg.Server.Upstream.TLS.KeyFile = opts.TLSKey
		cfg.Server.Downstream.TLS.KeyFile = opts.TLSKey
	}

	return cfg, nil
}

// promptWithDefault prompts for input with a default value.
func (g *ConfigGenerator) promptWithDefault(scanner *bufio.Scanner, prompt, defaultVal string) string {
	if defaultVal != "" {
		g.printLine(fmt.Sprintf("? %s [%s]: ", prompt, defaultVal))
	} else {
		g.printLine(fmt.Sprintf("? %s: ", prompt))
	}

	if scanner.Scan() {
		input := strings.TrimSpace(scanner.Text())
		if input != "" {
			return input
		}
	}
	return defaultVal
}

// promptYesNo prompts for a yes/no answer.
func (g *ConfigGenerator) promptYesNo(scanner *bufio.Scanner, prompt string, defaultVal bool) bool {
	defaultStr := "Y/n"
	if !defaultVal {
		defaultStr = "y/N"
	}
	g.printLine(fmt.Sprintf("? %s (%s): ", prompt, defaultStr))

	if scanner.Scan() {
		input := strings.ToLower(strings.TrimSpace(scanner.Text()))
		if input == "" {
			return defaultVal
		}
		return input == "y" || input == "yes"
	}
	return defaultVal
}

// printLine prints a line to the writer.
func (g *ConfigGenerator) printLine(s string) {
	if g.writer != nil {
		fmt.Fprint(g.writer, s)
	}
}

// WriteClientConfigToFile writes client config to a file.
func WriteClientConfigToFile(cfg *ClientConfig, path string) error {
	content, err := RenderClientConfigYAML(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// WriteServerConfigToFile writes server config to a file.
func WriteServerConfigToFile(cfg *ServerConfig, path string) error {
	content, err := RenderServerConfigYAML(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0644)
}

// RenderClientConfigYAML renders client config as YAML.
func RenderClientConfigYAML(cfg *ClientConfig) (string, error) {
	tmpl := `# Half-Tunnel Client Configuration
client:
  name: "{{.Client.Name}}"
  exit_on_port_in_use: {{.Client.ExitOnPortInUse}}
  listen_on_connect: {{.Client.ListenOnConnect}}
  upstream:
    url: "{{.Client.Upstream.URL}}"
    tls:
      enabled: {{.Client.Upstream.TLS.Enabled}}
      skip_verify: {{.Client.Upstream.TLS.SkipVerify}}
{{- if .Client.Upstream.TLS.CAFile}}
      ca_file: "{{.Client.Upstream.TLS.CAFile}}"
{{- end}}
  downstream:
    url: "{{.Client.Downstream.URL}}"
    tls:
      enabled: {{.Client.Downstream.TLS.Enabled}}
      skip_verify: {{.Client.Downstream.TLS.SkipVerify}}
{{- if .Client.Downstream.TLS.CAFile}}
      ca_file: "{{.Client.Downstream.TLS.CAFile}}"
{{- end}}

port_forwards:
{{- range .PortForwardsRendered}}
  - {{.}}
{{- end}}
{{- if not .PortForwardsRendered}}
  # Add port forwards here
  # Examples:
  # - 2083                    # Forward port 2083
  # - port: 443               # Forward port 443
  # - listen_port: 8080
  #   remote_host: "example.com"
  #   remote_port: 80
{{- end}}

socks5:
  enabled: {{.SOCKS5.Enabled}}
  listen_host: "{{.SOCKS5.ListenHost}}"
  listen_port: {{.SOCKS5.ListenPort}}
  auth:
    enabled: {{.SOCKS5.Auth.Enabled}}

tunnel:
  reconnect:
    enabled: {{.Tunnel.Reconnect.Enabled}}
    initial_delay: "{{.Tunnel.Reconnect.InitialDelay}}"
    max_delay: "{{.Tunnel.Reconnect.MaxDelay}}"
    multiplier: {{.Tunnel.Reconnect.Multiplier}}
    jitter: {{.Tunnel.Reconnect.Jitter}}
  connection:
    read_buffer_size: {{.Tunnel.Connection.ReadBufferSize}}
    write_buffer_size: {{.Tunnel.Connection.WriteBufferSize}}
    keepalive_interval: "{{.Tunnel.Connection.KeepaliveInterval}}"
    dial_timeout: "{{.Tunnel.Connection.DialTimeout}}"
  encryption:
    enabled: {{.Tunnel.Encryption.Enabled}}
    algorithm: "{{.Tunnel.Encryption.Algorithm}}"

dns:
  enabled: {{.DNS.Enabled}}
  listen_host: "{{.DNS.ListenHost}}"
  listen_port: {{.DNS.ListenPort}}
  upstream_servers:
{{- range .DNS.UpstreamServers}}
    - "{{.}}"
{{- end}}

logging:
  level: "{{.Logging.Level}}"
  format: "{{.Logging.Format}}"
{{- if .Logging.Output}}
  output: "{{.Logging.Output}}"
{{- end}}

observability:
  metrics:
    enabled: {{.Observability.Metrics.Enabled}}
    port: {{.Observability.Metrics.Port}}
    path: "{{.Observability.Metrics.Path}}"
`

	// Prepare port forwards for rendering
	type renderData struct {
		*ClientConfig
		PortForwardsRendered []string
	}
	data := renderData{ClientConfig: cfg}

	portForwards, err := cfg.GetPortForwards()
	if err != nil {
		return "", err
	}

	for _, pf := range portForwards {
		if pf.RemoteHost == "127.0.0.1" && pf.ListenPort == pf.RemotePort && pf.ListenHost == "0.0.0.0" {
			data.PortForwardsRendered = append(data.PortForwardsRendered, strconv.Itoa(pf.ListenPort))
		} else {
			var parts []string
			if pf.Name != "" {
				parts = append(parts, fmt.Sprintf("name: %q", pf.Name))
			}
			if pf.ListenHost != "0.0.0.0" {
				parts = append(parts, fmt.Sprintf("listen_host: %q", pf.ListenHost))
			}
			parts = append(parts, fmt.Sprintf("listen_port: %d", pf.ListenPort))
			if pf.RemoteHost != "" && pf.RemoteHost != "127.0.0.1" {
				parts = append(parts, fmt.Sprintf("remote_host: %q", pf.RemoteHost))
			}
			if pf.RemotePort != pf.ListenPort {
				parts = append(parts, fmt.Sprintf("remote_port: %d", pf.RemotePort))
			}
			data.PortForwardsRendered = append(data.PortForwardsRendered, "{"+strings.Join(parts, ", ")+"}")
		}
	}

	t, err := template.New("client").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// RenderServerConfigYAML renders server config as YAML.
func RenderServerConfigYAML(cfg *ServerConfig) (string, error) {
	tmpl := `# Half-Tunnel Server Configuration
server:
  name: "{{.Server.Name}}"
  exit_on_port_in_use: {{.Server.ExitOnPortInUse}}
  upstream:
    host: "{{.Server.Upstream.Host}}"
    port: {{.Server.Upstream.Port}}
    path: "{{.Server.Upstream.Path}}"
    tls:
      enabled: {{.Server.Upstream.TLS.Enabled}}
{{- if .Server.Upstream.TLS.CertFile}}
      cert_file: "{{.Server.Upstream.TLS.CertFile}}"
      key_file: "{{.Server.Upstream.TLS.KeyFile}}"
{{- end}}
  downstream:
    host: "{{.Server.Downstream.Host}}"
    port: {{.Server.Downstream.Port}}
    path: "{{.Server.Downstream.Path}}"
    tls:
      enabled: {{.Server.Downstream.TLS.Enabled}}
{{- if .Server.Downstream.TLS.CertFile}}
      cert_file: "{{.Server.Downstream.TLS.CertFile}}"
      key_file: "{{.Server.Downstream.TLS.KeyFile}}"
{{- end}}

access:
  allowed_networks:
{{- range .Access.AllowedNetworks}}
    - "{{.}}"
{{- end}}
  blocked_networks:
{{- range .Access.BlockedNetworks}}
    - "{{.}}"
{{- end}}
  max_streams_per_session: {{.Access.MaxStreamsPerSession}}

tunnel:
  session:
    timeout: "{{.Tunnel.Session.Timeout}}"
    max_sessions: {{.Tunnel.Session.MaxSessions}}
  connection:
    read_buffer_size: {{.Tunnel.Connection.ReadBufferSize}}
    write_buffer_size: {{.Tunnel.Connection.WriteBufferSize}}
    keepalive_interval: "{{.Tunnel.Connection.KeepaliveInterval}}"
    max_message_size: {{.Tunnel.Connection.MaxMessageSize}}
  encryption:
    enabled: {{.Tunnel.Encryption.Enabled}}
    algorithm: "{{.Tunnel.Encryption.Algorithm}}"

logging:
  level: "{{.Logging.Level}}"
  format: "{{.Logging.Format}}"
{{- if .Logging.Output}}
  output: "{{.Logging.Output}}"
{{- end}}

observability:
  metrics:
    enabled: {{.Observability.Metrics.Enabled}}
    port: {{.Observability.Metrics.Port}}
    path: "{{.Observability.Metrics.Path}}"
  health:
    enabled: {{.Observability.Health.Enabled}}
    port: {{.Observability.Health.Port}}
    path: "{{.Observability.Health.Path}}"
`

	t, err := template.New("server").Parse(tmpl)
	if err != nil {
		return "", err
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, cfg); err != nil {
		return "", err
	}

	return buf.String(), nil
}

// GetSampleClientConfig returns the sample client configuration as a string.
func GetSampleClientConfig() string {
	cfg := DefaultClientConfig()
	// Add some example port forwards
	cfg.PortForwards = []interface{}{
		2083,
		8080,
		map[string]interface{}{
			"name":        "web-proxy",
			"listen_port": 8080,
			"remote_host": "example.com",
			"remote_port": 80,
		},
	}
	content, _ := RenderClientConfigYAML(cfg)
	return content
}

// GetSampleServerConfig returns the sample server configuration as a string.
func GetSampleServerConfig() string {
	cfg := DefaultServerConfig()
	content, _ := RenderServerConfigYAML(cfg)
	return content
}

// ValidateConfigFile validates a configuration file.
func ValidateConfigFile(path string, configType string) error {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("config file not found: %s", path)
	}

	switch configType {
	case "client":
		cfg, err := LoadClientConfig(path)
		if err != nil {
			return err
		}
		return cfg.Validate()
	case "server":
		cfg, err := LoadServerConfig(path)
		if err != nil {
			return err
		}
		return cfg.Validate()
	default:
		return fmt.Errorf("unknown config type: %s (use 'client' or 'server')", configType)
	}
}
