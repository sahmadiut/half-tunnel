// Package main provides the entry point for the Half-Tunnel server.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/sahmadiut/half-tunnel/internal/config"
	"github.com/sahmadiut/half-tunnel/internal/server"
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
		fmt.Printf("half-tunnel server %s (commit: %s, built: %s)\n", version, commit, buildDate)
		os.Exit(0)
	}

	// Load configuration
	cfg, err := config.LoadServerConfig(*configPath)
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

	// Construct addresses from host:port
	upstreamAddr := fmt.Sprintf("%s:%d", cfg.Server.Upstream.Host, cfg.Server.Upstream.Port)
	downstreamAddr := fmt.Sprintf("%s:%d", cfg.Server.Downstream.Host, cfg.Server.Downstream.Port)

	log.Info().
		Str("version", version).
		Str("upstream_addr", upstreamAddr).
		Str("downstream_addr", downstreamAddr).
		Msg("Starting Half-Tunnel server")

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

	// Create server configuration
	serverConfig := &server.Config{
		UpstreamAddr:    upstreamAddr,
		UpstreamPath:    cfg.Server.Upstream.Path,
		DownstreamAddr:  downstreamAddr,
		DownstreamPath:  cfg.Server.Downstream.Path,
		SessionTimeout:  cfg.Tunnel.Session.Timeout,
		MaxSessions:     cfg.Tunnel.Session.MaxSessions,
		ReadBufferSize:  cfg.Tunnel.Connection.ReadBufferSize,
		WriteBufferSize: cfg.Tunnel.Connection.WriteBufferSize,
		MaxMessageSize:  cfg.Tunnel.Connection.MaxMessageSize,
		DialTimeout:     cfg.Tunnel.Connection.KeepaliveInterval,
	}

	// Create and start the server
	s := server.New(serverConfig, log)
	if err := s.Start(ctx); err != nil {
		log.Error().Err(err).Msg("Failed to start server")
		os.Exit(1)
	}

	log.Info().Msg("Server is ready")

	// Periodic stats logging
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				log.Info().
					Int("active_sessions", s.GetSessionCount()).
					Int("nat_entries", s.GetNatEntryCount()).
					Msg("Server stats")
			}
		}
	}()

	// Wait for shutdown
	<-ctx.Done()
	log.Info().Msg("Shutting down server")

	// Stop the server with a timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := s.Stop(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Error stopping server")
	}
}
