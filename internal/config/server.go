// Package config provides configuration loading for the Half-Tunnel system.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// ServerConfig represents the complete server configuration.
type ServerConfig struct {
	Server        ServerSettings     `mapstructure:"server"`
	Access        AccessConfig       `mapstructure:"access"`
	Tunnel        ServerTunnelConfig `mapstructure:"tunnel"`
	Chisel        ChiselServerConfig `mapstructure:"chisel"`
	Logging       LoggingConfig      `mapstructure:"logging"`
	Observability ObservConfig       `mapstructure:"observability"`
}

// ChiselServerConfig holds chisel tunnel configuration for server side.
// When enabled, chisel server listens for incoming chisel client connections and forwards
// traffic to the actual upstream/downstream handlers.
// Host and TLS settings are derived from the existing upstream/downstream configuration.
type ChiselServerConfig struct {
	Enabled        bool `mapstructure:"enabled"`
	UpstreamPort   int  `mapstructure:"upstream_port"`   // Port for upstream chisel server to listen on
	DownstreamPort int  `mapstructure:"downstream_port"` // Port for downstream chisel server to listen on
}

// ServerSettings holds server-specific settings.
type ServerSettings struct {
	Name       string         `mapstructure:"name"`
	Upstream   ServerEndpoint `mapstructure:"upstream"`
	Downstream ServerEndpoint `mapstructure:"downstream"`
}

// ServerEndpoint defines a server listener endpoint.
type ServerEndpoint struct {
	Host string          `mapstructure:"host"`
	Port int             `mapstructure:"port"`
	Path string          `mapstructure:"path"`
	TLS  ServerTLSConfig `mapstructure:"tls"`
}

// ServerTLSConfig holds TLS configuration for server endpoints.
type ServerTLSConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	CertFile string `mapstructure:"cert_file"`
	KeyFile  string `mapstructure:"key_file"`
}

// AccessConfig defines server-side access control.
type AccessConfig struct {
	AllowedNetworks      []string `mapstructure:"allowed_networks"`
	BlockedNetworks      []string `mapstructure:"blocked_networks"`
	MaxStreamsPerSession int      `mapstructure:"max_streams_per_session"`
}

// ServerTunnelConfig holds tunnel settings for the server.
type ServerTunnelConfig struct {
	Session    ServerSessionConfig    `mapstructure:"session"`
	Connection ServerConnectionConfig `mapstructure:"connection"`
	Encryption EncryptionConfig       `mapstructure:"encryption"`
}

// ServerSessionConfig holds session management settings for server.
type ServerSessionConfig struct {
	Timeout     time.Duration `mapstructure:"timeout"`
	MaxSessions int           `mapstructure:"max_sessions"`
}

// ServerConnectionConfig holds connection settings for server.
type ServerConnectionConfig struct {
	ReadBufferSize    int           `mapstructure:"read_buffer_size"`
	WriteBufferSize   int           `mapstructure:"write_buffer_size"`
	KeepaliveInterval time.Duration `mapstructure:"keepalive_interval"`
	MaxMessageSize    int           `mapstructure:"max_message_size"`
	BufferMode        string        `mapstructure:"buffer_mode"` // small, default, large, max
	TCPNoDelay        bool          `mapstructure:"tcp_nodelay"` // Disable Nagle's algorithm
}

// EncryptionConfig holds encryption settings.
type EncryptionConfig struct {
	Enabled   bool   `mapstructure:"enabled"`
	Algorithm string `mapstructure:"algorithm"`
}

// LoggingConfig holds logging configuration.
type LoggingConfig struct {
	Level  string `mapstructure:"level"`
	Format string `mapstructure:"format"`
	Output string `mapstructure:"output"`
}

// ObservConfig holds observability configuration.
type ObservConfig struct {
	Metrics MetricsConfig `mapstructure:"metrics"`
	Health  HealthConfig  `mapstructure:"health"`
}

// MetricsConfig holds metrics endpoint configuration.
type MetricsConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Port    int    `mapstructure:"port"`
	Path    string `mapstructure:"path"`
}

// HealthConfig holds health endpoint configuration.
type HealthConfig struct {
	Enabled bool   `mapstructure:"enabled"`
	Port    int    `mapstructure:"port"`
	Path    string `mapstructure:"path"`
}

// DefaultServerConfig returns a ServerConfig with sensible defaults.
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Server: ServerSettings{
			Name: "exit-server-01",
			Upstream: ServerEndpoint{
				Host: "0.0.0.0",
				Port: 8443,
				Path: "/ws/upstream",
				TLS: ServerTLSConfig{
					Enabled:  false,
					CertFile: "",
					KeyFile:  "",
				},
			},
			Downstream: ServerEndpoint{
				Host: "0.0.0.0",
				Port: 8444,
				Path: "/ws/downstream",
				TLS: ServerTLSConfig{
					Enabled:  false,
					CertFile: "",
					KeyFile:  "",
				},
			},
		},
		Access: AccessConfig{
			AllowedNetworks:      []string{"0.0.0.0/0", "::/0"},
			BlockedNetworks:      []string{},
			MaxStreamsPerSession: 100,
		},
		Tunnel: ServerTunnelConfig{
			Session: ServerSessionConfig{
				Timeout:     5 * time.Minute,
				MaxSessions: 1000,
			},
			Connection: ServerConnectionConfig{
				ReadBufferSize:    32768,
				WriteBufferSize:   32768,
				KeepaliveInterval: 30 * time.Second,
				MaxMessageSize:    65536,
				BufferMode:        "default",
				TCPNoDelay:        true,
			},
			Encryption: EncryptionConfig{
				Enabled:   true,
				Algorithm: "aes-256-gcm",
			},
		},
		Chisel: ChiselServerConfig{
			Enabled:        false,
			UpstreamPort:   9000,  // Default port for upstream chisel server
			DownstreamPort: 9001,  // Default port for downstream chisel server
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
			Output: "",
		},
		Observability: ObservConfig{
			Metrics: MetricsConfig{
				Enabled: true,
				Port:    9090,
				Path:    "/metrics",
			},
			Health: HealthConfig{
				Enabled: true,
				Port:    8080,
				Path:    "/healthz",
			},
		},
	}
}

// LoadServerConfig loads server configuration from a file.
func LoadServerConfig(configPath string) (*ServerConfig, error) {
	v := viper.New()

	// Set defaults
	setServerDefaults(v)

	// Set config file
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		v.SetConfigName("server")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("./configs")
		v.AddConfigPath("/etc/half-tunnel/")
		v.AddConfigPath("$HOME/.half-tunnel/")
	}

	// Read environment variables
	v.SetEnvPrefix("HT_SERVER")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_", "-", "_"))
	v.AutomaticEnv()

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config: %w", err)
		}
		// Config file not found, use defaults
	}

	var cfg ServerConfig
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	return &cfg, nil
}

// LoadServerConfigFromFile loads server configuration from a specific file path.
func LoadServerConfigFromFile(path string) (*ServerConfig, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found: %s", path)
	}
	return LoadServerConfig(path)
}

// setServerDefaults sets default values for server configuration.
func setServerDefaults(v *viper.Viper) {
	defaults := DefaultServerConfig()

	v.SetDefault("server.name", defaults.Server.Name)
	v.SetDefault("server.upstream.host", defaults.Server.Upstream.Host)
	v.SetDefault("server.upstream.port", defaults.Server.Upstream.Port)
	v.SetDefault("server.upstream.path", defaults.Server.Upstream.Path)
	v.SetDefault("server.upstream.tls.enabled", defaults.Server.Upstream.TLS.Enabled)
	v.SetDefault("server.downstream.host", defaults.Server.Downstream.Host)
	v.SetDefault("server.downstream.port", defaults.Server.Downstream.Port)
	v.SetDefault("server.downstream.path", defaults.Server.Downstream.Path)
	v.SetDefault("server.downstream.tls.enabled", defaults.Server.Downstream.TLS.Enabled)

	v.SetDefault("access.allowed_networks", defaults.Access.AllowedNetworks)
	v.SetDefault("access.blocked_networks", defaults.Access.BlockedNetworks)
	v.SetDefault("access.max_streams_per_session", defaults.Access.MaxStreamsPerSession)

	v.SetDefault("tunnel.session.timeout", defaults.Tunnel.Session.Timeout)
	v.SetDefault("tunnel.session.max_sessions", defaults.Tunnel.Session.MaxSessions)
	v.SetDefault("tunnel.connection.read_buffer_size", defaults.Tunnel.Connection.ReadBufferSize)
	v.SetDefault("tunnel.connection.write_buffer_size", defaults.Tunnel.Connection.WriteBufferSize)
	v.SetDefault("tunnel.connection.keepalive_interval", defaults.Tunnel.Connection.KeepaliveInterval)
	v.SetDefault("tunnel.connection.max_message_size", defaults.Tunnel.Connection.MaxMessageSize)
	v.SetDefault("tunnel.connection.buffer_mode", defaults.Tunnel.Connection.BufferMode)
	v.SetDefault("tunnel.connection.tcp_nodelay", defaults.Tunnel.Connection.TCPNoDelay)
	v.SetDefault("tunnel.encryption.enabled", defaults.Tunnel.Encryption.Enabled)
	v.SetDefault("tunnel.encryption.algorithm", defaults.Tunnel.Encryption.Algorithm)

	v.SetDefault("chisel.enabled", defaults.Chisel.Enabled)
	v.SetDefault("chisel.upstream_port", defaults.Chisel.UpstreamPort)
	v.SetDefault("chisel.downstream_port", defaults.Chisel.DownstreamPort)

	v.SetDefault("logging.level", defaults.Logging.Level)
	v.SetDefault("logging.format", defaults.Logging.Format)
	v.SetDefault("logging.output", defaults.Logging.Output)

	v.SetDefault("observability.metrics.enabled", defaults.Observability.Metrics.Enabled)
	v.SetDefault("observability.metrics.port", defaults.Observability.Metrics.Port)
	v.SetDefault("observability.metrics.path", defaults.Observability.Metrics.Path)
	v.SetDefault("observability.health.enabled", defaults.Observability.Health.Enabled)
	v.SetDefault("observability.health.port", defaults.Observability.Health.Port)
	v.SetDefault("observability.health.path", defaults.Observability.Health.Path)
}

// Validate validates the server configuration.
func (c *ServerConfig) Validate() error {
	if c.Server.Upstream.Port <= 0 || c.Server.Upstream.Port > 65535 {
		return fmt.Errorf("invalid upstream port: %d", c.Server.Upstream.Port)
	}
	if c.Server.Downstream.Port <= 0 || c.Server.Downstream.Port > 65535 {
		return fmt.Errorf("invalid downstream port: %d", c.Server.Downstream.Port)
	}
	if c.Server.Upstream.TLS.Enabled {
		if c.Server.Upstream.TLS.CertFile == "" {
			return fmt.Errorf("upstream TLS enabled but cert_file not specified")
		}
		if c.Server.Upstream.TLS.KeyFile == "" {
			return fmt.Errorf("upstream TLS enabled but key_file not specified")
		}
	}
	if c.Server.Downstream.TLS.Enabled {
		if c.Server.Downstream.TLS.CertFile == "" {
			return fmt.Errorf("downstream TLS enabled but cert_file not specified")
		}
		if c.Server.Downstream.TLS.KeyFile == "" {
			return fmt.Errorf("downstream TLS enabled but key_file not specified")
		}
	}
	// Validate chisel configuration
	if c.Chisel.Enabled {
		if c.Chisel.UpstreamPort <= 0 || c.Chisel.UpstreamPort > 65535 {
			return fmt.Errorf("invalid chisel upstream port: %d", c.Chisel.UpstreamPort)
		}
		if c.Chisel.DownstreamPort <= 0 || c.Chisel.DownstreamPort > 65535 {
			return fmt.Errorf("invalid chisel downstream port: %d", c.Chisel.DownstreamPort)
		}
	}
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
