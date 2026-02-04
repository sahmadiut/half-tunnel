// Package chisel provides WebSocket tunnel transport using chisel.
// Chisel is a fast TCP/UDP tunnel, transported over HTTP/WebSocket.
// When enabled, all half-tunnel traffic is routed through chisel tunnels
// instead of direct WebSocket connections.
//
// This package manages:
// - Starting chisel client processes (on client side)
// - Starting chisel server processes (on server side)
// - Forwarding local half-tunnel traffic through the chisel tunnels
package chisel

import (
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"sync"
	"time"

	"github.com/sahmadiut/half-tunnel/pkg/logger"
)

// ClientConfig holds configuration for the chisel client transport.
// The server host is extracted from the upstream/downstream URLs.
type ClientConfig struct {
	// UpstreamURL is the original upstream WebSocket URL (used to extract server host)
	UpstreamURL string
	// DownstreamURL is the original downstream WebSocket URL (used to extract server host)
	DownstreamURL string
	// UpstreamChiselPort is the port for the upstream chisel tunnel
	UpstreamChiselPort int
	// DownstreamChiselPort is the port for the downstream chisel tunnel
	DownstreamChiselPort int
	// TargetUpstreamPort is the original upstream port that chisel forwards to
	TargetUpstreamPort int
	// TargetDownstreamPort is the original downstream port that chisel forwards to
	TargetDownstreamPort int
}

// ServerConfig holds configuration for the chisel server transport.
// Host settings are derived from the existing upstream/downstream configuration.
type ServerConfig struct {
	// UpstreamHost is the host from upstream configuration
	UpstreamHost string
	// DownstreamHost is the host from downstream configuration
	DownstreamHost string
	// UpstreamChiselPort is the port for upstream chisel server
	UpstreamChiselPort int
	// DownstreamChiselPort is the port for downstream chisel server
	DownstreamChiselPort int
	// TargetUpstreamPort is the original upstream port
	TargetUpstreamPort int
	// TargetDownstreamPort is the original downstream port
	TargetDownstreamPort int
}

// Client represents a chisel client that forwards traffic through chisel tunnels.
type Client struct {
	config        *ClientConfig
	log           *logger.Logger
	upstreamCmd   *exec.Cmd
	downstreamCmd *exec.Cmd
	mu            sync.Mutex
	running       bool
	cancel        context.CancelFunc
}

// NewClient creates a new chisel client transport.
func NewClient(config *ClientConfig, log *logger.Logger) *Client {
	if log == nil {
		log = logger.NewDefault()
	}
	return &Client{
		config: config,
		log:    log,
	}
}

// extractHost extracts the host from a WebSocket URL.
func extractHost(wsURL string) (string, error) {
	parsed, err := url.Parse(wsURL)
	if err != nil {
		return "", err
	}
	return parsed.Hostname(), nil
}

// Start starts the chisel client processes for upstream and downstream tunnels.
// It starts chisel clients that connect to the chisel servers and forward
// traffic from local ports to the remote half-tunnel handlers.
func (c *Client) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.running {
		return fmt.Errorf("chisel client already running")
	}

	// Extract upstream host from URL
	upstreamHost, err := extractHost(c.config.UpstreamURL)
	if err != nil {
		return fmt.Errorf("failed to parse upstream URL: %w", err)
	}

	// Extract downstream host from URL
	downstreamHost, err := extractHost(c.config.DownstreamURL)
	if err != nil {
		return fmt.Errorf("failed to parse downstream URL: %w", err)
	}

	ctx, cancel := context.WithCancel(ctx)
	c.cancel = cancel

	// Start upstream chisel client
	// Forward local upstream chisel port to remote upstream target
	upstreamRemote := fmt.Sprintf("127.0.0.1:%d:127.0.0.1:%d",
		c.config.UpstreamChiselPort,
		c.config.TargetUpstreamPort,
	)
	upstreamServerURL := fmt.Sprintf("http://%s:%d",
		upstreamHost,
		c.config.UpstreamChiselPort,
	)

	c.log.Info().
		Str("server", upstreamServerURL).
		Str("remote", upstreamRemote).
		Msg("Starting upstream chisel client")

	c.upstreamCmd = exec.CommandContext(ctx, "chisel", "client",
		upstreamServerURL,
		upstreamRemote,
	)

	if err := c.upstreamCmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("failed to start upstream chisel client: %w", err)
	}

	// Start downstream chisel client
	downstreamRemote := fmt.Sprintf("127.0.0.1:%d:127.0.0.1:%d",
		c.config.DownstreamChiselPort,
		c.config.TargetDownstreamPort,
	)
	downstreamServerURL := fmt.Sprintf("http://%s:%d",
		downstreamHost,
		c.config.DownstreamChiselPort,
	)

	c.log.Info().
		Str("server", downstreamServerURL).
		Str("remote", downstreamRemote).
		Msg("Starting downstream chisel client")

	c.downstreamCmd = exec.CommandContext(ctx, "chisel", "client",
		downstreamServerURL,
		downstreamRemote,
	)

	if err := c.downstreamCmd.Start(); err != nil {
		// Stop upstream if downstream fails
		if c.upstreamCmd != nil && c.upstreamCmd.Process != nil {
			_ = c.upstreamCmd.Process.Kill()
		}
		cancel()
		return fmt.Errorf("failed to start downstream chisel client: %w", err)
	}

	c.running = true

	// Wait for tunnels to be established
	time.Sleep(2 * time.Second)

	c.log.Info().Msg("Chisel client tunnels established")
	return nil
}

// Stop stops the chisel client processes.
func (c *Client) Stop() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.running {
		return nil
	}

	if c.cancel != nil {
		c.cancel()
	}

	// Stop upstream chisel
	if c.upstreamCmd != nil && c.upstreamCmd.Process != nil {
		if err := c.upstreamCmd.Process.Kill(); err != nil {
			c.log.Debug().Err(err).Msg("Error killing upstream chisel client")
		}
	}

	// Stop downstream chisel
	if c.downstreamCmd != nil && c.downstreamCmd.Process != nil {
		if err := c.downstreamCmd.Process.Kill(); err != nil {
			c.log.Debug().Err(err).Msg("Error killing downstream chisel client")
		}
	}

	c.running = false
	c.log.Info().Msg("Chisel client stopped")
	return nil
}

// GetUpstreamAddr returns the local address for the upstream chisel tunnel.
func (c *Client) GetUpstreamAddr() string {
	return fmt.Sprintf("ws://127.0.0.1:%d", c.config.UpstreamChiselPort)
}

// GetDownstreamAddr returns the local address for the downstream chisel tunnel.
func (c *Client) GetDownstreamAddr() string {
	return fmt.Sprintf("ws://127.0.0.1:%d", c.config.DownstreamChiselPort)
}

// Server represents a chisel server that accepts chisel client connections
// and forwards traffic to the actual upstream/downstream handlers.
type Server struct {
	config        *ServerConfig
	log           *logger.Logger
	upstreamCmd   *exec.Cmd
	downstreamCmd *exec.Cmd
	mu            sync.Mutex
	running       bool
	cancel        context.CancelFunc
}

// NewServer creates a new chisel server transport.
func NewServer(config *ServerConfig, log *logger.Logger) *Server {
	if log == nil {
		log = logger.NewDefault()
	}
	return &Server{
		config: config,
		log:    log,
	}
}

// Start starts the chisel server processes for upstream and downstream tunnels.
func (s *Server) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("chisel server already running")
	}

	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	// Start upstream chisel server
	upstreamAddr := fmt.Sprintf("%s:%d", s.config.UpstreamHost, s.config.UpstreamChiselPort)
	targetUpstreamAddr := fmt.Sprintf("127.0.0.1:%d", s.config.TargetUpstreamPort)

	s.log.Info().
		Str("addr", upstreamAddr).
		Str("target", targetUpstreamAddr).
		Msg("Starting upstream chisel server")

	s.upstreamCmd = exec.CommandContext(ctx, "chisel", "server",
		"--port", fmt.Sprintf("%d", s.config.UpstreamChiselPort),
		"--host", s.config.UpstreamHost,
		"--backend", targetUpstreamAddr,
	)

	if err := s.upstreamCmd.Start(); err != nil {
		cancel()
		return fmt.Errorf("failed to start upstream chisel server: %w", err)
	}

	// Start downstream chisel server
	downstreamAddr := fmt.Sprintf("%s:%d", s.config.DownstreamHost, s.config.DownstreamChiselPort)
	targetDownstreamAddr := fmt.Sprintf("127.0.0.1:%d", s.config.TargetDownstreamPort)

	s.log.Info().
		Str("addr", downstreamAddr).
		Str("target", targetDownstreamAddr).
		Msg("Starting downstream chisel server")

	s.downstreamCmd = exec.CommandContext(ctx, "chisel", "server",
		"--port", fmt.Sprintf("%d", s.config.DownstreamChiselPort),
		"--host", s.config.DownstreamHost,
		"--backend", targetDownstreamAddr,
	)

	if err := s.downstreamCmd.Start(); err != nil {
		// Stop upstream if downstream fails
		if s.upstreamCmd != nil && s.upstreamCmd.Process != nil {
			_ = s.upstreamCmd.Process.Kill()
		}
		cancel()
		return fmt.Errorf("failed to start downstream chisel server: %w", err)
	}

	s.running = true

	// Wait for servers to be ready
	time.Sleep(2 * time.Second)

	s.log.Info().Msg("Chisel servers started")
	return nil
}

// Stop stops the chisel server processes.
func (s *Server) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	if s.cancel != nil {
		s.cancel()
	}

	// Stop upstream chisel server
	if s.upstreamCmd != nil && s.upstreamCmd.Process != nil {
		if err := s.upstreamCmd.Process.Kill(); err != nil {
			s.log.Debug().Err(err).Msg("Error killing upstream chisel server")
		}
	}

	// Stop downstream chisel server
	if s.downstreamCmd != nil && s.downstreamCmd.Process != nil {
		if err := s.downstreamCmd.Process.Kill(); err != nil {
			s.log.Debug().Err(err).Msg("Error killing downstream chisel server")
		}
	}

	s.running = false
	s.log.Info().Msg("Chisel servers stopped")
	return nil
}

// IsChiselAvailable checks if the chisel binary is available in PATH.
func IsChiselAvailable() bool {
	_, err := exec.LookPath("chisel")
	return err == nil
}
