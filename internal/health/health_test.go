package health

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestNewHandler(t *testing.T) {
	t.Run("with nil config uses defaults", func(t *testing.T) {
		h := NewHandler(nil)
		if h.checkTimeout != 5*time.Second {
			t.Errorf("expected 5s timeout, got %v", h.checkTimeout)
		}
	})

	t.Run("with custom config", func(t *testing.T) {
		cfg := &Config{CheckTimeout: 10 * time.Second}
		h := NewHandler(cfg)
		if h.checkTimeout != 10*time.Second {
			t.Errorf("expected 10s timeout, got %v", h.checkTimeout)
		}
	})
}

func TestHandler_RegisterCheck(t *testing.T) {
	h := NewHandler(nil)
	h.RegisterCheck("test", func(ctx context.Context) error {
		return nil
	})

	h.checksMu.RLock()
	_, exists := h.checks["test"]
	h.checksMu.RUnlock()

	if !exists {
		t.Error("check should be registered")
	}
}

func TestHandler_UnregisterCheck(t *testing.T) {
	h := NewHandler(nil)
	h.RegisterCheck("test", func(ctx context.Context) error {
		return nil
	})
	h.UnregisterCheck("test")

	h.checksMu.RLock()
	_, exists := h.checks["test"]
	h.checksMu.RUnlock()

	if exists {
		t.Error("check should be unregistered")
	}
}

func TestHandler_Healthz(t *testing.T) {
	h := NewHandler(nil)
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rec := httptest.NewRecorder()

	h.Healthz()(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var response Response
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response.Status != StatusHealthy {
		t.Errorf("expected status healthy, got %v", response.Status)
	}
}

func TestHandler_Readyz(t *testing.T) {
	t.Run("healthy with no checks", func(t *testing.T) {
		h := NewHandler(nil)
		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()

		h.Readyz()(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}

		var response Response
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if response.Status != StatusHealthy {
			t.Errorf("expected status healthy, got %v", response.Status)
		}
	})

	t.Run("healthy with passing checks", func(t *testing.T) {
		h := NewHandler(nil)
		h.RegisterCheck("check1", func(ctx context.Context) error {
			return nil
		})
		h.RegisterCheck("check2", func(ctx context.Context) error {
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()

		h.Readyz()(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", rec.Code)
		}

		var response Response
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if response.Status != StatusHealthy {
			t.Errorf("expected status healthy, got %v", response.Status)
		}
		if len(response.Checks) != 2 {
			t.Errorf("expected 2 checks, got %d", len(response.Checks))
		}
	})

	t.Run("unhealthy with failing check", func(t *testing.T) {
		h := NewHandler(nil)
		h.RegisterCheck("check1", func(ctx context.Context) error {
			return nil
		})
		h.RegisterCheck("check2", func(ctx context.Context) error {
			return errors.New("database unavailable")
		})

		req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
		rec := httptest.NewRecorder()

		h.Readyz()(rec, req)

		if rec.Code != http.StatusServiceUnavailable {
			t.Errorf("expected status 503, got %d", rec.Code)
		}

		var response Response
		if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}

		if response.Status != StatusUnhealthy {
			t.Errorf("expected status unhealthy, got %v", response.Status)
		}
	})
}

func TestHandler_ServeHTTP(t *testing.T) {
	h := NewHandler(nil)

	tests := []struct {
		path       string
		wantStatus int
	}{
		{"/healthz", http.StatusOK},
		{"/livez", http.StatusOK},
		{"/readyz", http.StatusOK},
		{"/unknown", http.StatusNotFound},
	}

	for _, tt := range tests {
		t.Run(tt.path, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, tt.path, nil)
			rec := httptest.NewRecorder()

			h.ServeHTTP(rec, req)

			if rec.Code != tt.wantStatus {
				t.Errorf("expected status %d, got %d", tt.wantStatus, rec.Code)
			}
		})
	}
}

func TestStatus_String(t *testing.T) {
	if StatusHealthy != "healthy" {
		t.Error("StatusHealthy should be 'healthy'")
	}
	if StatusDegraded != "degraded" {
		t.Error("StatusDegraded should be 'degraded'")
	}
	if StatusUnhealthy != "unhealthy" {
		t.Error("StatusUnhealthy should be 'unhealthy'")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.CheckTimeout != 5*time.Second {
		t.Errorf("expected 5s timeout, got %v", cfg.CheckTimeout)
	}
}

func TestDefaultServerConfig(t *testing.T) {
	cfg := DefaultServerConfig()
	if cfg.Addr != ":8080" {
		t.Errorf("expected :8080, got %s", cfg.Addr)
	}
	if cfg.HealthzPath != "/healthz" {
		t.Errorf("expected /healthz, got %s", cfg.HealthzPath)
	}
	if cfg.ReadyzPath != "/readyz" {
		t.Errorf("expected /readyz, got %s", cfg.ReadyzPath)
	}
}

func TestServer_RegisterCheck(t *testing.T) {
	s := NewServer(nil)
	s.RegisterCheck("test", func(ctx context.Context) error {
		return nil
	})

	s.handler.checksMu.RLock()
	_, exists := s.handler.checks["test"]
	s.handler.checksMu.RUnlock()

	if !exists {
		t.Error("check should be registered")
	}
}

func TestServer_Addr(t *testing.T) {
	cfg := &ServerConfig{
		Addr:        ":9090",
		HealthzPath: "/healthz",
		ReadyzPath:  "/readyz",
	}
	s := NewServer(cfg)
	if s.Addr() != ":9090" {
		t.Errorf("expected :9090, got %s", s.Addr())
	}
}
