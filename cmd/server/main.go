// Package main provides the entry point for the Half-Tunnel server.
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

	"github.com/fsnotify/fsnotify"
	"github.com/sahmadiut/half-tunnel/internal/config"
	"github.com/sahmadiut/half-tunnel/internal/health"
	"github.com/sahmadiut/half-tunnel/internal/metrics"
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
	hotReload := flag.Bool("hot-reload", false, "Enable hot reload of configuration file")
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
		Bool("hot_reload", *hotReload).
		Msg("Starting Half-Tunnel server")

	// Set up context for graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown and reload signals
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	go func() {
		for {
			select {
			case sig := <-sigCh:
				switch sig {
				case syscall.SIGHUP:
					log.Info().Msg("Received SIGHUP, reloading configuration...")
					log.Info().Msg("Config reload requested - restarting service")
					cancel()
					return
				case syscall.SIGINT, syscall.SIGTERM:
					log.Info().Str("signal", sig.String()).Msg("Received shutdown signal")
					cancel()
					return
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	// Create server configuration
	serverConfig := &server.Config{
		UpstreamAddr:    upstreamAddr,
		UpstreamPath:    cfg.Server.Upstream.Path,
		UpstreamTLS:     server.TLSConfig{Enabled: cfg.Server.Upstream.TLS.Enabled, CertFile: cfg.Server.Upstream.TLS.CertFile, KeyFile: cfg.Server.Upstream.TLS.KeyFile},
		DownstreamAddr:  downstreamAddr,
		DownstreamPath:  cfg.Server.Downstream.Path,
		DownstreamTLS:   server.TLSConfig{Enabled: cfg.Server.Downstream.TLS.Enabled, CertFile: cfg.Server.Downstream.TLS.CertFile, KeyFile: cfg.Server.Downstream.TLS.KeyFile},
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

	// Start hot reload watcher if enabled
	if *hotReload && *configPath != "" {
		watcher, err := fsnotify.NewWatcher()
		if err != nil {
			log.Warn().Err(err).Msg("Failed to create config watcher, hot reload disabled")
		} else {
			defer watcher.Close()
			if err := watcher.Add(*configPath); err != nil {
				log.Warn().Err(err).Str("path", *configPath).Msg("Failed to watch config file")
			} else {
				log.Info().Str("path", *configPath).Msg("Watching config file for changes")
				go func() {
					for {
						select {
						case event, ok := <-watcher.Events:
							if !ok {
								return
							}
							if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
								log.Info().Str("path", event.Name).Msg("Config file changed, triggering reload...")
								// Send SIGHUP to self to trigger reload
								_ = syscall.Kill(syscall.Getpid(), syscall.SIGHUP)
							}
						case err, ok := <-watcher.Errors:
							if !ok {
								return
							}
							log.Warn().Err(err).Msg("Config watcher error")
						case <-ctx.Done():
							return
						}
					}
				}()
			}
		}
	}

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

	var healthServer *health.Server
	if cfg.Observability.Health.Enabled {
		addr := fmt.Sprintf(":%d", cfg.Observability.Health.Port)
		readyzPath := "/readyz"
		if cfg.Observability.Health.Path == "/readyz" {
			readyzPath = "/healthz"
		}
		healthServer = health.NewServer(&health.ServerConfig{
			Addr:        addr,
			HealthzPath: cfg.Observability.Health.Path,
			ReadyzPath:  readyzPath,
		})
		go func() {
			if err := healthServer.Start(); err != nil && err != http.ErrServerClosed {
				log.Error().Err(err).Msg("Health server error")
			}
		}()
		log.Info().Str("addr", addr).Str("path", cfg.Observability.Health.Path).Msg("Health server started")
	}

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

	if metricsServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			log.Error().Err(err).Msg("Metrics server shutdown error")
		}
		shutdownCancel()
	}

	if healthServer != nil {
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := healthServer.Shutdown(shutdownCtx); err != nil {
			log.Error().Err(err).Msg("Health server shutdown error")
		}
		shutdownCancel()
	}

	// Stop the server with a timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := s.Stop(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("Error stopping server")
	}
}
