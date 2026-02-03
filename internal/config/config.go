// Package config provides configuration loading for the Half-Tunnel system.
package config

import (
	"fmt"
	"os"
	"time"

	"github.com/spf13/viper"
)

// Config holds all configuration for the Half-Tunnel system.
// This is the legacy combined configuration used by the existing client and server.
type Config struct {
	Client LegacyClientConfig `mapstructure:"client"`
	Server LegacyServerConfig `mapstructure:"server"`
	Log    LogConfig          `mapstructure:"log"`
}

// LegacyClientConfig holds client-specific configuration (legacy format).
type LegacyClientConfig struct {
	// Listen address for local proxy (e.g., "127.0.0.1:1080" for SOCKS5)
	ListenAddr string `mapstructure:"listen_addr"`
	// Upstream WebSocket URL (Domain A)
	UpstreamURL string `mapstructure:"upstream_url"`
	// Downstream WebSocket URL (Domain B)
	DownstreamURL string `mapstructure:"downstream_url"`
	// TLS configuration
	TLS TLSConfig `mapstructure:"tls"`
	// Connection settings
	Connection ConnectionConfig `mapstructure:"connection"`
}

// LegacyServerConfig holds server-specific configuration (legacy format).
type LegacyServerConfig struct {
	// Upstream listener address (Domain A)
	UpstreamAddr string `mapstructure:"upstream_addr"`
	// Downstream listener address (Domain B)
	DownstreamAddr string `mapstructure:"downstream_addr"`
	// TLS configuration
	TLS TLSConfig `mapstructure:"tls"`
	// Session settings
	Session SessionConfig `mapstructure:"session"`
}

// TLSConfig holds TLS-related configuration.
type TLSConfig struct {
	// Enable TLS
	Enabled bool `mapstructure:"enabled"`
	// Path to certificate file
	CertFile string `mapstructure:"cert_file"`
	// Path to key file
	KeyFile string `mapstructure:"key_file"`
	// Path to CA certificate for client verification
	CAFile string `mapstructure:"ca_file"`
	// Skip certificate verification (insecure, for testing only)
	InsecureSkipVerify bool `mapstructure:"insecure_skip_verify"`
}

// ConnectionConfig holds connection-related settings.
type ConnectionConfig struct {
	// Ping interval for WebSocket keep-alive
	PingInterval time.Duration `mapstructure:"ping_interval"`
	// Timeout for pong response
	PongTimeout time.Duration `mapstructure:"pong_timeout"`
	// Write timeout
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
	// Read timeout
	ReadTimeout time.Duration `mapstructure:"read_timeout"`
	// Reconnect settings
	Reconnect LegacyReconnectConfig `mapstructure:"reconnect"`
}

// LegacyReconnectConfig holds reconnection settings (legacy format).
type LegacyReconnectConfig struct {
	// Enable automatic reconnection
	Enabled bool `mapstructure:"enabled"`
	// Initial delay before first reconnect attempt
	InitialDelay time.Duration `mapstructure:"initial_delay"`
	// Maximum delay between reconnect attempts
	MaxDelay time.Duration `mapstructure:"max_delay"`
	// Maximum number of reconnect attempts (0 = unlimited)
	MaxAttempts int `mapstructure:"max_attempts"`
}

// SessionConfig holds session management settings.
type SessionConfig struct {
	// Idle timeout before session eviction
	IdleTimeout time.Duration `mapstructure:"idle_timeout"`
	// Maximum number of concurrent sessions
	MaxSessions int `mapstructure:"max_sessions"`
	// Maximum number of streams per session
	MaxStreamsPerSession int `mapstructure:"max_streams_per_session"`
}

// LogConfig holds logging configuration.
type LogConfig struct {
	// Log level: debug, info, warn, error
	Level string `mapstructure:"level"`
	// Log format: json, console
	Format string `mapstructure:"format"`
	// Output file path (empty for stdout)
	Output string `mapstructure:"output"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		Client: LegacyClientConfig{
			ListenAddr:    "127.0.0.1:1080",
			UpstreamURL:   "ws://localhost:8080/upstream",
			DownstreamURL: "ws://localhost:8081/downstream",
			TLS: TLSConfig{
				Enabled: false,
			},
			Connection: ConnectionConfig{
				PingInterval: 30 * time.Second,
				PongTimeout:  10 * time.Second,
				WriteTimeout: 10 * time.Second,
				ReadTimeout:  60 * time.Second,
				Reconnect: LegacyReconnectConfig{
					Enabled:      true,
					InitialDelay: 1 * time.Second,
					MaxDelay:     60 * time.Second,
					MaxAttempts:  0,
				},
			},
		},
		Server: LegacyServerConfig{
			UpstreamAddr:   ":8080",
			DownstreamAddr: ":8081",
			TLS: TLSConfig{
				Enabled: false,
			},
			Session: SessionConfig{
				IdleTimeout:          5 * time.Minute,
				MaxSessions:          10000,
				MaxStreamsPerSession: 1000,
			},
		},
		Log: LogConfig{
			Level:  "info",
			Format: "console",
			Output: "",
		},
	}
}

// Load loads configuration from a file.
func Load(configPath string) (*Config, error) {
	v := viper.New()

	// Set defaults
	setDefaults(v)

	// Set config file
	if configPath != "" {
		v.SetConfigFile(configPath)
	} else {
		// Look for config in standard locations
		v.SetConfigName("config")
		v.SetConfigType("yaml")
		v.AddConfigPath(".")
		v.AddConfigPath("/etc/half-tunnel/")
		v.AddConfigPath("$HOME/.half-tunnel/")
	}

	// Read environment variables
	v.SetEnvPrefix("HT")
	v.AutomaticEnv()

	// Read config file
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, fmt.Errorf("error reading config: %w", err)
		}
		// Config file not found, use defaults
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %w", err)
	}

	return &cfg, nil
}

// LoadFromFile loads configuration from a specific file path.
func LoadFromFile(path string) (*Config, error) {
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return nil, fmt.Errorf("config file not found: %s", path)
	}
	return Load(path)
}

// setDefaults sets default values in viper.
func setDefaults(v *viper.Viper) {
	defaults := DefaultConfig()

	// Client defaults
	v.SetDefault("client.listen_addr", defaults.Client.ListenAddr)
	v.SetDefault("client.upstream_url", defaults.Client.UpstreamURL)
	v.SetDefault("client.downstream_url", defaults.Client.DownstreamURL)
	v.SetDefault("client.tls.enabled", defaults.Client.TLS.Enabled)
	v.SetDefault("client.connection.ping_interval", defaults.Client.Connection.PingInterval)
	v.SetDefault("client.connection.pong_timeout", defaults.Client.Connection.PongTimeout)
	v.SetDefault("client.connection.write_timeout", defaults.Client.Connection.WriteTimeout)
	v.SetDefault("client.connection.read_timeout", defaults.Client.Connection.ReadTimeout)
	v.SetDefault("client.connection.reconnect.enabled", defaults.Client.Connection.Reconnect.Enabled)
	v.SetDefault("client.connection.reconnect.initial_delay", defaults.Client.Connection.Reconnect.InitialDelay)
	v.SetDefault("client.connection.reconnect.max_delay", defaults.Client.Connection.Reconnect.MaxDelay)
	v.SetDefault("client.connection.reconnect.max_attempts", defaults.Client.Connection.Reconnect.MaxAttempts)

	// Server defaults
	v.SetDefault("server.upstream_addr", defaults.Server.UpstreamAddr)
	v.SetDefault("server.downstream_addr", defaults.Server.DownstreamAddr)
	v.SetDefault("server.tls.enabled", defaults.Server.TLS.Enabled)
	v.SetDefault("server.session.idle_timeout", defaults.Server.Session.IdleTimeout)
	v.SetDefault("server.session.max_sessions", defaults.Server.Session.MaxSessions)
	v.SetDefault("server.session.max_streams_per_session", defaults.Server.Session.MaxStreamsPerSession)

	// Log defaults
	v.SetDefault("log.level", defaults.Log.Level)
	v.SetDefault("log.format", defaults.Log.Format)
	v.SetDefault("log.output", defaults.Log.Output)
}
