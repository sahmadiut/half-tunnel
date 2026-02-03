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
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Initialize logger
	log, err := logger.New(logger.Config{
		Level:  cfg.Log.Level,
		Format: cfg.Log.Format,
		Output: cfg.Log.Output,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	log.Info().
		Str("version", version).
		Str("upstream", cfg.Client.UpstreamURL).
		Str("downstream", cfg.Client.DownstreamURL).
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

	// Create client configuration
	clientConfig := &client.Config{
		UpstreamURL:      cfg.Client.UpstreamURL,
		DownstreamURL:    cfg.Client.DownstreamURL,
		SOCKS5Addr:       cfg.Client.ListenAddr,
		PingInterval:     cfg.Client.Connection.PingInterval,
		WriteTimeout:     cfg.Client.Connection.WriteTimeout,
		ReadTimeout:      cfg.Client.Connection.ReadTimeout,
		DialTimeout:      10 * cfg.Client.Connection.WriteTimeout, // Default dial timeout
		HandshakeTimeout: cfg.Client.Connection.WriteTimeout,
	}

	// Create and start the client
	c := client.New(clientConfig, log)
	if err := c.Start(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to start client")
		os.Exit(1)
	}

	log.Info().
		Str("session_id", c.GetSessionID().String()).
		Str("socks5_addr", cfg.Client.ListenAddr).
		Msg("Client is ready")

	// Wait for shutdown
	<-ctx.Done()
	log.Info().Msg("Shutting down client")

	// Stop the client
	if err := c.Stop(); err != nil {
		log.Error().Err(err).Msg("Error stopping client")
	}
}
