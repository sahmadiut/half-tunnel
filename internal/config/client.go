// Package config provides configuration loading for the Half-Tunnel system.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// ClientConfig represents the complete client configuration.
type ClientConfig struct {
	Client        ClientSettings     `mapstructure:"client"`
	PortForwards  []interface{}      `mapstructure:"port_forwards"`
	SOCKS5        SOCKS5Config       `mapstructure:"socks5"`
	Tunnel        ClientTunnelConfig `mapstructure:"tunnel"`
	DNS           DNSConfig          `mapstructure:"dns"`
	Logging       LoggingConfig      `mapstructure:"logging"`
	Observability ClientObservConfig `mapstructure:"observability"`
}

// ClientSettings holds client-specific settings.
type ClientSettings struct {
	Name            string         `mapstructure:"name"`
	ExitOnPortInUse bool           `mapstructure:"exit_on_port_in_use"`
	ListenOnConnect bool           `mapstructure:"listen_on_connect"`
	Upstream        ClientEndpoint `mapstructure:"upstream"`
	Downstream      ClientEndpoint `mapstructure:"downstream"`
}

// ClientEndpoint defines a client connection endpoint.
type ClientEndpoint struct {
	URL string          `mapstructure:"url"`
	TLS ClientTLSConfig `mapstructure:"tls"`
}

// ClientTLSConfig holds TLS configuration for client connections.
type ClientTLSConfig struct {
	Enabled    bool   `mapstructure:"enabled"`
	SkipVerify bool   `mapstructure:"skip_verify"`
	CAFile     string `mapstructure:"ca_file"`
}

// PortForward defines a port forwarding rule with smart defaults.
type PortForward struct {
	Name       string `mapstructure:"name,omitempty" yaml:"name,omitempty"`
	ListenHost string `mapstructure:"listen_host,omitempty" yaml:"listen_host,omitempty"`
	ListenPort int    `mapstructure:"listen_port,omitempty" yaml:"listen_port,omitempty"`
	Port       int    `mapstructure:"port,omitempty" yaml:"port,omitempty"`
	RemoteHost string `mapstructure:"remote_host,omitempty" yaml:"remote_host,omitempty"`
	RemotePort int    `mapstructure:"remote_port,omitempty" yaml:"remote_port,omitempty"`
	Protocol   string `mapstructure:"protocol,omitempty" yaml:"protocol,omitempty"`
}

// SOCKS5Config holds SOCKS5 proxy configuration.
type SOCKS5Config struct {
	Enabled    bool       `mapstructure:"enabled"`
	ListenHost string     `mapstructure:"listen_host"`
	ListenPort int        `mapstructure:"listen_port"`
	Auth       SOCKS5Auth `mapstructure:"auth"`
}

// SOCKS5Auth holds SOCKS5 authentication settings.
type SOCKS5Auth struct {
	Enabled  bool   `mapstructure:"enabled"`
	Username string `mapstructure:"username"`
	Password string `mapstructure:"password"`
}

// ClientTunnelConfig holds tunnel settings for the client.
type ClientTunnelConfig struct {
	Reconnect  ReconnectConfig        `mapstructure:"reconnect"`
	Connection ClientConnectionConfig `mapstructure:"connection"`
	Encryption EncryptionConfig       `mapstructure:"encryption"`
}

// ReconnectConfig holds reconnection strategy settings.
type ReconnectConfig struct {
	Enabled      bool          `mapstructure:"enabled"`
	InitialDelay time.Duration `mapstructure:"initial_delay"`
	MaxDelay     time.Duration `mapstructure:"max_delay"`
	Multiplier   float64       `mapstructure:"multiplier"`
	Jitter       float64       `mapstructure:"jitter"`
}

// ClientConnectionConfig holds connection settings for client.
type ClientConnectionConfig struct {
	ReadBufferSize    int           `mapstructure:"read_buffer_size"`
	WriteBufferSize   int           `mapstructure:"write_buffer_size"`
	KeepaliveInterval time.Duration `mapstructure:"keepalive_interval"`
	DialTimeout       time.Duration `mapstructure:"dial_timeout"`
}

// DNSConfig holds DNS settings for VPN mode.
type DNSConfig struct {
	Enabled         bool     `mapstructure:"enabled"`
	ListenHost      string   `mapstructure:"listen_host"`
	ListenPort      int      `mapstructure:"listen_port"`
	UpstreamServers []string `mapstructure:"upstream_servers"`
}

// ClientObservConfig holds client observability configuration.
type ClientObservConfig struct {
	Metrics MetricsConfig `mapstructure:"metrics"`
}

// DefaultClientConfig returns a ClientConfig with sensible defaults.
func DefaultClientConfig() *ClientConfig {
	return &ClientConfig{
		Client: ClientSettings{
			Name:            "entry-client-01",
			ExitOnPortInUse: false,
			ListenOnConnect: false,
			Upstream: ClientEndpoint{
				URL: "wss://domain-a.example.com:8443/ws/upstream",
				TLS: ClientTLSConfig{
					Enabled:    true,
					SkipVerify: false,
					CAFile:     "",
				},
			},
			Downstream: ClientEndpoint{
				URL: "wss://domain-b.example.com:8444/ws/downstream",
				TLS: ClientTLSConfig{
					Enabled:    true,
					SkipVerify: false,
					CAFile:     "",
				},
			},
		},
		PortForwards: []interface{}{},
		SOCKS5: SOCKS5Config{
			Enabled:    true,
			ListenHost: "127.0.0.1",
			ListenPort: 1080,
			Auth: SOCKS5Auth{
				Enabled:  false,
				Username: "",
				Password: "",
			},
		},
		Tunnel: ClientTunnelConfig{
			Reconnect: ReconnectConfig{
				Enabled:      true,
				InitialDelay: 1 * time.Second,
				MaxDelay:     60 * time.Second,
				Multiplier:   2.0,
				Jitter:       0.1,
			},
			Connection: ClientConnectionConfig{
				ReadBufferSize:    32768,
				WriteBufferSize:   32768,
				KeepaliveInterval: 30 * time.Second,
				DialTimeout:       10 * time.Second,
			},
			Encryption: EncryptionConfig{
				Enabled:   true,
				Algorithm: "aes-256-gcm",
			},
		},
		DNS: DNSConfig{
			Enabled:         false,
			ListenHost:      "127.0.0.1",
			ListenPort:      5353,
			UpstreamServers: []string{"8.8.8.8:53", "1.1.1.1:53"},
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "",
		},
		Observability: ClientObservConfig{
			Metrics: MetricsConfig{
				Enabled: true,
				Port:    9091,
				Path:    "/metrics",
			},
		},
	}
}

// LoadClientConfig loads client configuration from a file.
func LoadClientConfig(configPath string) (*ClientConfig, error) {
	v := viper.New()

	// Set defaults
	setClientDefaults(v)

	// Set config file
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("client")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("./configs")
		v.AddConfigPath("/etc/half-tunnel/")
		v.AddConfigPath("$HOME/.half-tunnel/")
	}

	// Read environment variables
	v.SetEnvPrefix("HT_CLIENT")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config: %w", err)
		}
		// Config file not found, use defaults
	}

	var cfg ClientConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	return &cfg, nil
}

// LoadClientConfigFromFile loads client configuration from a specific file path.
func LoadClientConfigFromFile(path string) (*ClientConfig, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found: %s", path)
	}
	return LoadClientConfig(path)
}

// setClientDefaults sets default values for client configuration.
func setClientDefaults(v *viper.Viper) {
	defaults := DefaultClientConfig()

	v.SetDefault("client.name", defaults.Client.Name)
	v.SetDefault("client.exit_on_port_in_use", defaults.Client.ExitOnPortInUse)
	v.SetDefault("client.listen_on_connect", defaults.Client.ListenOnConnect)
	v.SetDefault("client.upstream.url", defaults.Client.Upstream.URL)
	v.SetDefault("client.upstream.tls.enabled", defaults.Client.Upstream.TLS.Enabled)
	v.SetDefault("client.upstream.tls.skip_verify", defaults.Client.Upstream.TLS.SkipVerify)
	v.SetDefault("client.downstream.url", defaults.Client.Downstream.URL)
	v.SetDefault("client.downstream.tls.enabled", defaults.Client.Downstream.TLS.Enabled)
	v.SetDefault("client.downstream.tls.skip_verify", defaults.Client.Downstream.TLS.SkipVerify)

	v.SetDefault("socks5.enabled", defaults.SOCKS5.Enabled)
	v.SetDefault("socks5.listen_host", defaults.SOCKS5.ListenHost)
	v.SetDefault("socks5.listen_port", defaults.SOCKS5.ListenPort)
	v.SetDefault("socks5.auth.enabled", defaults.SOCKS5.Auth.Enabled)

	v.SetDefault("tunnel.reconnect.enabled", defaults.Tunnel.Reconnect.Enabled)
	v.SetDefault("tunnel.reconnect.initial_delay", defaults.Tunnel.Reconnect.InitialDelay)
	v.SetDefault("tunnel.reconnect.max_delay", defaults.Tunnel.Reconnect.MaxDelay)
	v.SetDefault("tunnel.reconnect.multiplier", defaults.Tunnel.Reconnect.Multiplier)
	v.SetDefault("tunnel.reconnect.jitter", defaults.Tunnel.Reconnect.Jitter)
	v.SetDefault("tunnel.connection.read_buffer_size", defaults.Tunnel.Connection.ReadBufferSize)
	v.SetDefault("tunnel.connection.write_buffer_size", defaults.Tunnel.Connection.WriteBufferSize)
	v.SetDefault("tunnel.connection.keepalive_interval", defaults.Tunnel.Connection.KeepaliveInterval)
	v.SetDefault("tunnel.connection.dial_timeout", defaults.Tunnel.Connection.DialTimeout)
	v.SetDefault("tunnel.encryption.enabled", defaults.Tunnel.Encryption.Enabled)
	v.SetDefault("tunnel.encryption.algorithm", defaults.Tunnel.Encryption.Algorithm)

	v.SetDefault("dns.enabled", defaults.DNS.Enabled)
	v.SetDefault("dns.listen_host", defaults.DNS.ListenHost)
	v.SetDefault("dns.listen_port", defaults.DNS.ListenPort)
	v.SetDefault("dns.upstream_servers", defaults.DNS.UpstreamServers)

	v.SetDefault("logging.level", defaults.Logging.Level)
	v.SetDefault("logging.format", defaults.Logging.Format)
	v.SetDefault("logging.output", defaults.Logging.Output)

	v.SetDefault("observability.metrics.enabled", defaults.Observability.Metrics.Enabled)
	v.SetDefault("observability.metrics.port", defaults.Observability.Metrics.Port)
	v.SetDefault("observability.metrics.path", defaults.Observability.Metrics.Path)
}

// GetPortForwards parses the flexible port_forwards configuration and returns normalized PortForward entries.
func (c *ClientConfig) GetPortForwards() ([]PortForward, error) {
	return ParsePortForwards(c.PortForwards)
}

// ParsePortForwards handles flexible YAML input formats for port forwarding.
// Supports:
// - int: just port number (e.g., 2083)
// - string: port specification (e.g., "8080", "8080:80", "8080:example.com:80", "1000-1200")
// - map: full object with optional fields
func ParsePortForwards(raw []interface{}) ([]PortForward, error) {
	var result []PortForward
	for i, entry := range raw {
		// Check if it's a port range string
		if str, ok := entry.(string); ok && isPortRange(str) {
			ports, err := ParsePortForwardStringRange(str)
			if err != nil {
				return nil, fmt.Errorf("port_forwards[%d]: %w", i, err)
			}
			result = append(result, ports...)
			continue
		}

		pf, err := parsePortForwardEntry(entry)
		if err != nil {
			return nil, fmt.Errorf("port_forwards[%d]: %w", i, err)
		}
		result = append(result, *pf)
	}
	return result, nil
}

// parsePortForwardEntry parses a single port forward entry.
func parsePortForwardEntry(entry interface{}) (*PortForward, error) {
	switch v := entry.(type) {
	case int:
		// Simple format: just port number
		return &PortForward{
			ListenHost: "0.0.0.0",
			ListenPort: v,
			RemoteHost: "127.0.0.1",
			RemotePort: v,
			Protocol:   "tcp",
		}, nil
	case float64:
		// YAML sometimes parses numbers as float64
		port := int(v)
		return &PortForward{
			ListenHost: "0.0.0.0",
			ListenPort: port,
			RemoteHost: "127.0.0.1",
			RemotePort: port,
			Protocol:   "tcp",
		}, nil
	case string:
		// String format: "port", "listen:remote", "listen:host:remote", or "start-end" (port range)
		return ParsePortForwardString(v)
	case map[string]interface{}:
		// Object format: parse fields with defaults
		return parsePortForwardMap(v)
	default:
		return nil, fmt.Errorf("unsupported port forward type: %T", entry)
	}
}

// isValidPort checks if a port number is in the valid range (1-65535).
func isValidPort(port int) bool {
	return port >= 1 && port <= 65535
}

// parsePortRange parses a port range string "start-end" and returns start, end ports.
// Returns an error if the format is invalid or ports are out of range.
func parsePortRange(spec string) (startPort, endPort int, err error) {
	parts := strings.Split(spec, "-")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid port range format: %s (expected start-end)", spec)
	}

	startPort, err = strconv.Atoi(parts[0])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid start port: %s", parts[0])
	}
	endPort, err = strconv.Atoi(parts[1])
	if err != nil {
		return 0, 0, fmt.Errorf("invalid end port: %s", parts[1])
	}

	if !isValidPort(startPort) {
		return 0, 0, fmt.Errorf("invalid start port: %d (must be 1-65535)", startPort)
	}
	if !isValidPort(endPort) {
		return 0, 0, fmt.Errorf("invalid end port: %d (must be 1-65535)", endPort)
	}
	if startPort > endPort {
		return 0, 0, fmt.Errorf("start port %d is greater than end port %d", startPort, endPort)
	}

	return startPort, endPort, nil
}

// ParsePortForwardString parses flexible port forward string formats:
// - "2083" → listen:2083, remote:2083
// - "8080:80" → listen:8080, remote:80
// - "8080:example.com:80" → listen:8080, remote:example.com:80
// - "1000-1200" → port range (returns first port, use ParsePortForwardStringRange for all)
func ParsePortForwardString(spec string) (*PortForward, error) {
	// Check for port range format (e.g., "1000-1200")
	if isPortRange(spec) {
		startPort, _, err := parsePortRange(spec)
		if err != nil {
			return nil, err
		}
		// Return the first port of the range (use ParsePortForwardStringRange for all ports)
		return &PortForward{
			ListenHost: "0.0.0.0",
			ListenPort: startPort,
			RemoteHost: "127.0.0.1",
			RemotePort: startPort,
			Protocol:   "tcp",
		}, nil
	}

	parts := strings.Split(spec, ":")
	pf := &PortForward{
		ListenHost: "0.0.0.0",
		RemoteHost: "127.0.0.1",
		Protocol:   "tcp",
	}

	switch len(parts) {
	case 1:
		// Single port
		port, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid port: %s", parts[0])
		}
		pf.ListenPort = port
		pf.RemotePort = port
	case 2:
		// listen:remote
		listenPort, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid listen port: %s", parts[0])
		}
		remotePort, err := strconv.Atoi(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid remote port: %s", parts[1])
		}
		pf.ListenPort = listenPort
		pf.RemotePort = remotePort
	case 3:
		// listen:host:remote
		listenPort, err := strconv.Atoi(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid listen port: %s", parts[0])
		}
		remotePort, err := strconv.Atoi(parts[2])
		if err != nil {
			return nil, fmt.Errorf("invalid remote port: %s", parts[2])
		}
		pf.ListenPort = listenPort
		pf.RemoteHost = parts[1]
		pf.RemotePort = remotePort
	default:
		return nil, fmt.Errorf("invalid port forward format: %s", spec)
	}

	return pf, nil
}

// ParsePortForwardStringRange parses a port range string and returns all port forwards.
// Format: "start-end" (e.g., "1000-1200")
// Returns a slice of PortForward for each port in the range.
func ParsePortForwardStringRange(spec string) ([]PortForward, error) {
	startPort, endPort, err := parsePortRange(spec)
	if err != nil {
		return nil, err
	}

	var result []PortForward
	for port := startPort; port <= endPort; port++ {
		result = append(result, PortForward{
			ListenHost: "0.0.0.0",
			ListenPort: port,
			RemoteHost: "127.0.0.1",
			RemotePort: port,
			Protocol:   "tcp",
		})
	}

	return result, nil
}

// isPortRange checks if a string is a port range format.
func isPortRange(spec string) bool {
	if !strings.Contains(spec, "-") || strings.Contains(spec, ":") {
		return false
	}
	parts := strings.Split(spec, "-")
	if len(parts) != 2 {
		return false
	}
	_, err1 := strconv.Atoi(parts[0])
	_, err2 := strconv.Atoi(parts[1])
	return err1 == nil && err2 == nil
}

// parsePortForwardMap parses a port forward from a map.
func parsePortForwardMap(m map[string]interface{}) (*PortForward, error) {
	pf := &PortForward{
		ListenHost: "0.0.0.0",
		RemoteHost: "127.0.0.1",
		Protocol:   "tcp",
	}

	// Parse name
	if v, ok := m["name"].(string); ok {
		pf.Name = v
	}

	// Parse listen_host
	if v, ok := m["listen_host"].(string); ok {
		pf.ListenHost = v
	}

	// Parse port (shorthand for listen_port and remote_port)
	if v, ok := m["port"]; ok {
		port, err := toInt(v)
		if err != nil {
			return nil, fmt.Errorf("invalid port: %w", err)
		}
		pf.Port = port
		pf.ListenPort = port
		pf.RemotePort = port
	}

	// Parse listen_port (overrides port)
	if v, ok := m["listen_port"]; ok {
		port, err := toInt(v)
		if err != nil {
			return nil, fmt.Errorf("invalid listen_port: %w", err)
		}
		pf.ListenPort = port
		if pf.RemotePort == 0 {
			pf.RemotePort = port
		}
	}

	// Parse remote_host
	if v, ok := m["remote_host"].(string); ok {
		pf.RemoteHost = v
	}

	// Parse remote_port
	if v, ok := m["remote_port"]; ok {
		port, err := toInt(v)
		if err != nil {
			return nil, fmt.Errorf("invalid remote_port: %w", err)
		}
		pf.RemotePort = port
	}

	// Parse protocol
	if v, ok := m["protocol"].(string); ok {
		pf.Protocol = v
	}

	// Validate that we have a port
	if pf.ListenPort == 0 {
		return nil, fmt.Errorf("port or listen_port is required")
	}

	return pf, nil
}

// toInt converts various numeric types to int.
func toInt(v interface{}) (int, error) {
	switch val := v.(type) {
	case int:
		return val, nil
	case int64:
		return int(val), nil
	case float64:
		return int(val), nil
	case string:
		return strconv.Atoi(val)
	default:
		return 0, fmt.Errorf("cannot convert %T to int", v)
	}
}

// Validate validates the client configuration.
func (c *ClientConfig) Validate() error {
	if c.Client.Upstream.URL == "" {
		return fmt.Errorf("upstream URL is required")
	}
	if c.Client.Downstream.URL == "" {
		return fmt.Errorf("downstream URL is required")
	}

	// Validate SOCKS5 port
	if c.SOCKS5.Enabled {
		if c.SOCKS5.ListenPort <= 0 || c.SOCKS5.ListenPort > 65535 {
			return fmt.Errorf("invalid SOCKS5 port: %d", c.SOCKS5.ListenPort)
		}
	}

	// Validate port forwards
	portForwards, err := c.GetPortForwards()
	if err != nil {
		return fmt.Errorf("invalid port forwards: %w", err)
	}
	for _, pf := range portForwards {
		if pf.ListenPort <= 0 || pf.ListenPort > 65535 {
			return fmt.Errorf("invalid listen port: %d", pf.ListenPort)
		}
		if pf.RemotePort <= 0 || pf.RemotePort > 65535 {
			return fmt.Errorf("invalid remote port: %d", pf.RemotePort)
		}
	}

	// Validate DNS
	if c.DNS.Enabled {
		if c.DNS.ListenPort <= 0 || c.DNS.ListenPort > 65535 {
			return fmt.Errorf("invalid DNS port: %d", c.DNS.ListenPort)
		}
	}

	// Validate encryption algorithm
	if c.Tunnel.Encryption.Enabled {
		switch c.Tunnel.Encryption.Algorithm {
		case "aes-256-gcm", "chacha20-poly1305":
			// valid
		default:
			return fmt.Errorf("invalid encryption algorithm: %s (use aes-256-gcm or chacha20-poly1305)", c.Tunnel.Encryption.Algorithm)
		}
	}

	return nil
}
