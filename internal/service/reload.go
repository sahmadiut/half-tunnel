// Package service provides systemd service management for Half-Tunnel.
package service

import (
	"context"
	"fmt"
	"sync"

	"github.com/fsnotify/fsnotify"
	"github.com/rs/zerolog"
)

// ConfigReloader watches a config file for changes and calls a reload callback.
type ConfigReloader struct {
	configPath string
	watcher    *fsnotify.Watcher
	reloadFn   func() error
	log        zerolog.Logger
	mu         sync.Mutex
	running    bool
}

// NewConfigReloader creates a new config file watcher.
func NewConfigReloader(configPath string, reloadFn func() error, log zerolog.Logger) (*ConfigReloader, error) {
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, fmt.Errorf("failed to create file watcher: %w", err)
	}

	return &ConfigReloader{
		configPath: configPath,
		watcher:    watcher,
		reloadFn:   reloadFn,
		log:        log,
	}, nil
}

// Start starts watching the config file for changes.
func (r *ConfigReloader) Start(ctx context.Context) error {
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return fmt.Errorf("reloader already running")
	}
	r.running = true
	r.mu.Unlock()

	// Watch the config file
	if err := r.watcher.Add(r.configPath); err != nil {
		return fmt.Errorf("failed to watch config file: %w", err)
	}

	r.log.Info().Str("path", r.configPath).Msg("Started watching config file for changes")

	go r.watch(ctx)

	return nil
}

// watch runs the file watching loop.
func (r *ConfigReloader) watch(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			r.log.Debug().Msg("Config reloader stopped")
			return
		case event, ok := <-r.watcher.Events:
			if !ok {
				return
			}
			// Handle write and create events (some editors recreate files)
			if event.Op&fsnotify.Write == fsnotify.Write || event.Op&fsnotify.Create == fsnotify.Create {
				r.log.Info().Str("path", event.Name).Str("op", event.Op.String()).Msg("Config file changed, reloading")

				if err := r.reloadFn(); err != nil {
					r.log.Error().Err(err).Msg("Failed to reload config")
				} else {
					r.log.Info().Msg("Config reloaded successfully")
				}

				// Re-watch if file was recreated
				if event.Op&fsnotify.Create == fsnotify.Create {
					_ = r.watcher.Add(r.configPath)
				}
			}
			// Handle file removal (some editors remove and recreate)
			if event.Op&fsnotify.Remove == fsnotify.Remove {
				r.log.Warn().Str("path", event.Name).Msg("Config file removed, waiting for recreation")
				// Try to re-add the watch
				_ = r.watcher.Add(r.configPath)
			}
		case err, ok := <-r.watcher.Errors:
			if !ok {
				return
			}
			r.log.Error().Err(err).Msg("File watcher error")
		}
	}
}

// Stop stops watching the config file.
func (r *ConfigReloader) Stop() error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.running {
		return nil
	}

	r.running = false
	return r.watcher.Close()
}
