// Package main provides the entry point for the Half-Tunnel client.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sahmadiut/half-tunnel/internal/client"
	"github.com/sahmadiut/half-tunnel/internal/config"
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

	go func() {
		sig := <-sigCh
		log.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
		cancel()
	}()

	// Build SOCKS5 address from configuration
	socks5Addr := fmt.Sprintf("%s:%d", cfg.SOCKS5.ListenHost, cfg.SOCKS5.ListenPort)

	// Create client configuration
	clientConfig := &client.Config{
		UpstreamURL:      cfg.Client.Upstream.URL,
		DownstreamURL:    cfg.Client.Downstream.URL,
		SOCKS5Addr:       socks5Addr,
		PingInterval:     cfg.Tunnel.Connection.KeepaliveInterval,
		WriteTimeout:     cfg.Tunnel.Connection.DialTimeout,
		ReadTimeout:      cfg.Tunnel.Connection.DialTimeout,
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

	log.Info().
		Str("session_id", c.GetSessionID().String()).
		Str("socks5_addr", socks5Addr).
		Msg("Client is ready")

	// Wait for shutdown
	<-ctx.Done()
	log.Info().Msg("Shutting down client")

	// Stop the client
	if err := c.Stop(); err != nil {
		log.Error().Err(err).Msg("Error stopping client")
	}
}
