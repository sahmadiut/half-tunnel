// Package health provides health check endpoints for the Half-Tunnel system.
package health

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"
)

// Status represents the health status of a component.
type Status string

const (
	// StatusHealthy indicates the component is healthy.
	StatusHealthy Status = "healthy"
	// StatusDegraded indicates the component is functioning but with issues.
	StatusDegraded Status = "degraded"
	// StatusUnhealthy indicates the component is not functioning.
	StatusUnhealthy Status = "unhealthy"
)

// Check is a function that performs a health check.
type Check func(ctx context.Context) error

// CheckResult represents the result of a health check.
type CheckResult struct {
	Name    string        `json:"name"`
	Status  Status        `json:"status"`
	Message string        `json:"message,omitempty"`
	Latency time.Duration `json:"latency"`
}

// Response is the health check response.
type Response struct {
	Status    Status        `json:"status"`
	Timestamp time.Time     `json:"timestamp"`
	Checks    []CheckResult `json:"checks,omitempty"`
}

// Handler provides HTTP health check endpoints.
type Handler struct {
	checks       map[string]Check
	checksMu     sync.RWMutex
	checkTimeout time.Duration
}

// Config holds configuration for the health handler.
type Config struct {
	// CheckTimeout is the maximum time allowed for each health check.
	CheckTimeout time.Duration
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		CheckTimeout: 5 * time.Second,
	}
}

// NewHandler creates a new health handler.
func NewHandler(config *Config) *Handler {
	if config == nil {
		config = DefaultConfig()
	}
	return &Handler{
		checks:       make(map[string]Check),
		checkTimeout: config.CheckTimeout,
	}
}

// RegisterCheck registers a health check.
func (h *Handler) RegisterCheck(name string, check Check) {
	h.checksMu.Lock()
	defer h.checksMu.Unlock()
	h.checks[name] = check
}

// UnregisterCheck removes a health check.
func (h *Handler) UnregisterCheck(name string) {
	h.checksMu.Lock()
	defer h.checksMu.Unlock()
	delete(h.checks, name)
}

// Healthz returns a simple health check handler for liveness probes.
// It returns 200 OK if the service is running.
func (h *Handler) Healthz() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		response := Response{
			Status:    StatusHealthy,
			Timestamp: time.Now(),
		}
		_ = json.NewEncoder(w).Encode(response)
	}
}

// Readyz returns a comprehensive health check handler for readiness probes.
// It runs all registered checks and returns 200 if all pass, 503 otherwise.
func (h *Handler) Readyz() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), h.checkTimeout)
		defer cancel()

		response := h.runChecks(ctx)

		w.Header().Set("Content-Type", "application/json")
		if response.Status == StatusHealthy {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}
		_ = json.NewEncoder(w).Encode(response)
	}
}

// runChecks runs all registered health checks.
func (h *Handler) runChecks(ctx context.Context) *Response {
	h.checksMu.RLock()
	checks := make(map[string]Check, len(h.checks))
	for name, check := range h.checks {
		checks[name] = check
	}
	h.checksMu.RUnlock()

	if len(checks) == 0 {
		return &Response{
			Status:    StatusHealthy,
			Timestamp: time.Now(),
			Checks:    nil,
		}
	}

	type result struct {
		name   string
		result CheckResult
	}

	resultsCh := make(chan result, len(checks))
	var wg sync.WaitGroup

	for name, check := range checks {
		wg.Add(1)
		go func(name string, check Check) {
			defer wg.Done()

			start := time.Now()
			err := check(ctx)
			latency := time.Since(start)

			r := CheckResult{
				Name:    name,
				Latency: latency,
			}

			if err != nil {
				r.Status = StatusUnhealthy
				r.Message = err.Error()
			} else {
				r.Status = StatusHealthy
			}

			resultsCh <- result{name: name, result: r}
		}(name, check)
	}

	wg.Wait()
	close(resultsCh)

	response := &Response{
		Status:    StatusHealthy,
		Timestamp: time.Now(),
		Checks:    make([]CheckResult, 0, len(checks)),
	}

	for r := range resultsCh {
		response.Checks = append(response.Checks, r.result)
		if r.result.Status == StatusUnhealthy {
			response.Status = StatusUnhealthy
		} else if r.result.Status == StatusDegraded && response.Status == StatusHealthy {
			response.Status = StatusDegraded
		}
	}

	return response
}

// ServeHTTP implements http.Handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Route based on path
	switch r.URL.Path {
	case "/healthz", "/livez":
		h.Healthz()(w, r)
	case "/readyz":
		h.Readyz()(w, r)
	default:
		http.NotFound(w, r)
	}
}

// Server is a standalone HTTP server for health checks.
type Server struct {
	handler *Handler
	server  *http.Server
	addr    string
}

// ServerConfig holds configuration for the health server.
type ServerConfig struct {
	Addr         string
	HealthzPath  string
	ReadyzPath   string
	CheckTimeout time.Duration
}

// DefaultServerConfig returns a ServerConfig with sensible defaults.
func DefaultServerConfig() *ServerConfig {
	return &ServerConfig{
		Addr:         ":8080",
		HealthzPath:  "/healthz",
		ReadyzPath:   "/readyz",
		CheckTimeout: 5 * time.Second,
	}
}

// NewServer creates a new health server.
func NewServer(config *ServerConfig) *Server {
	if config == nil {
		config = DefaultServerConfig()
	}
	if config.ReadyzPath == "" {
		config.ReadyzPath = "/readyz"
	}
	if config.HealthzPath == "" {
		config.HealthzPath = "/healthz"
	}

	handler := NewHandler(&Config{CheckTimeout: config.CheckTimeout})

	mux := http.NewServeMux()
	mux.HandleFunc(config.HealthzPath, handler.Healthz())
	mux.HandleFunc(config.ReadyzPath, handler.Readyz())

	return &Server{
		handler: handler,
		addr:    config.Addr,
		server: &http.Server{
			Addr:         config.Addr,
			Handler:      mux,
			ReadTimeout:  10 * time.Second,
			WriteTimeout: 10 * time.Second,
		},
	}
}

// RegisterCheck registers a health check with the server.
func (s *Server) RegisterCheck(name string, check Check) {
	s.handler.RegisterCheck(name, check)
}

// Start starts the health server.
func (s *Server) Start() error {
	return s.server.ListenAndServe()
}

// Shutdown gracefully shuts down the health server.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

// Addr returns the server address.
func (s *Server) Addr() string {
	return s.addr
}
