// Package main provides the entry point for the Half-Tunnel client.
package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sahmadiut/half-tunnel/internal/client"
	"github.com/sahmadiut/half-tunnel/internal/config"
	"github.com/sahmadiut/half-tunnel/internal/metrics"
	"github.com/sahmadiut/half-tunnel/internal/retry"
	"github.com/sahmadiut/half-tunnel/pkg/logger"
)

var (
	version   = "dev"
	commit    = "none"
	buildDate = "unknown"
)

func main() {
	// Parse command line flags
	configPath := flag.String("config", "", "Path to configuration file")
	showVersion := flag.Bool("version", false, "Show version information")
	flag.Parse()

	if *showVersion {
		fmt.Printf("half-tunnel client %s (commit: %s, built: %s)\n", version, commit, buildDate)
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.LoadClientConfig(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Invalid configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log, err := logger.New(logger.Config{
		Level:  cfg.Logging.Level,
		Format: cfg.Logging.Format,
		Output: cfg.Logging.Output,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	log.Info().
		Str("version", version).
		Str("upstream", cfg.Client.Upstream.URL).
		Str("downstream", cfg.Client.Downstream.URL).
		Msg("Starting Half-Tunnel client")

	// Set up context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	go func() {
		select {
		case sig := <-sigCh:
			log.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
			cancel()
		case <-ctx.Done():
		}
	}()

	// Build SOCKS5 address from configuration
	socks5Addr := fmt.Sprintf("%s:%d", cfg.SOCKS5.ListenHost, cfg.SOCKS5.ListenPort)

	// Parse port forwards from configuration
	portForwards, err := cfg.GetPortForwards()
	if err != nil {
		log.Error().Err(err).Msg("Failed to parse port forwards")
		os.Exit(1)
	}

	// Convert config port forwards to client port forwards
	clientPortForwards := make([]client.PortForward, len(portForwards))
	for i, pf := range portForwards {
		clientPortForwards[i] = client.PortForward{
			Name:       pf.Name,
			ListenHost: pf.ListenHost,
			ListenPort: pf.ListenPort,
			RemoteHost: pf.RemoteHost,
			RemotePort: pf.RemotePort,
		}
	}

	readTimeout := time.Duration(0)
	if cfg.Tunnel.Connection.KeepaliveInterval > 0 {
		readTimeout = cfg.Tunnel.Connection.KeepaliveInterval * 2
	}

	// Create client configuration
	clientConfig := &client.Config{
		UpstreamURL:      cfg.Client.Upstream.URL,
		DownstreamURL:    cfg.Client.Downstream.URL,
		SOCKS5Addr:       socks5Addr,
		SOCKS5Enabled:    cfg.SOCKS5.Enabled,
		PortForwards:     clientPortForwards,
		ReconnectEnabled: cfg.Tunnel.Reconnect.Enabled,
		ReconnectConfig: &retry.Config{
			InitialDelay: cfg.Tunnel.Reconnect.InitialDelay,
			MaxDelay:     cfg.Tunnel.Reconnect.MaxDelay,
			Multiplier:   cfg.Tunnel.Reconnect.Multiplier,
			Jitter:       cfg.Tunnel.Reconnect.Jitter,
		},
		PingInterval:     cfg.Tunnel.Connection.KeepaliveInterval,
		WriteTimeout:     cfg.Tunnel.Connection.DialTimeout,
		ReadTimeout:      readTimeout,
		DialTimeout:      cfg.Tunnel.Connection.DialTimeout,
		HandshakeTimeout: cfg.Tunnel.Connection.DialTimeout,
	}

	// Set SOCKS5 authentication if enabled
	if cfg.SOCKS5.Auth.Enabled {
		clientConfig.SOCKS5Username = cfg.SOCKS5.Auth.Username
		clientConfig.SOCKS5Password = cfg.SOCKS5.Auth.Password
	}

	// Create and start the client
	c := client.New(clientConfig, log)
	if err := c.Start(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to start client")
		os.Exit(1)
	}

	// Start metrics server if enabled
	var metricsServer *metrics.Server
	if cfg.Observability.Metrics.Enabled {
		addr := fmt.Sprintf(":%d", cfg.Observability.Metrics.Port)
		metricsServer = metrics.NewServer(&metrics.ServerConfig{
			Addr: addr,
			Path: cfg.Observability.Metrics.Path,
		})
		go func() {
			if err := metricsServer.Start(); err != nil && err != http.ErrServerClosed {
				log.Error().Err(err).Msg("Metrics server error")
			}
		}()
		log.Info().Str("addr", addr).Str("path", cfg.Observability.Metrics.Path).Msg("Metrics server started")
	}

	// Log startup info
	if cfg.SOCKS5.Enabled {
		log.Info().
			Str("session_id", c.GetSessionID().String()).
			Str("socks5_addr", socks5Addr).
			Int("port_forwards", len(clientPortForwards)).
			Msg("Client is ready")
	} else {
		log.Info().
			Str("session_id", c.GetSessionID().String()).
			Int("port_forwards", len(clientPortForwards)).
			Msg("Client is ready")
	}

	// Wait for shutdown
	<-ctx.Done()
	log.Info().Msg("Shutting down client")

	if metricsServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			log.Error().Err(err).Msg("Metrics server shutdown error")
		}
		shutdownCancel()
	}

	// Stop the client
	if err := c.Stop(); err != nil {
		log.Error().Err(err).Msg("Error stopping client")
	}
}
