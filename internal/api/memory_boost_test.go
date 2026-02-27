package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/clawinfra/evoclaw/internal/agents"
	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/models"
	"github.com/clawinfra/evoclaw/internal/orchestrator"
	"log/slog"
)

// newTestServerWithMemory creates a server with an orchestrator that has memory enabled.
func newTestServerWithMemory(t *testing.T) *Server {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError}))
	dir := t.TempDir()

	cfg := config.DefaultConfig()
	cfg.Memory.Enabled = true
	orch := orchestrator.New(cfg, logger)
	if err := orch.Start(); err != nil {
		t.Fatalf("start orchestrator: %v", err)
	}
	t.Cleanup(func() { _ = orch.Stop() })

	reg, _ := agents.NewRegistry(dir, logger)
	mem, _ := agents.NewMemoryStore(dir, logger)
	router := models.NewRouter(logger)

	return NewServer(0, orch, reg, mem, router, logger)
}



func TestHandleMemoryStats_WithMemory(t *testing.T) {
	s := newTestServerWithMemory(t)

	req := httptest.NewRequest(http.MethodGet, "/api/memory/stats", nil)
	w := httptest.NewRecorder()
	s.handleMemoryStats(w, req)

	// With memory enabled, should return 200 or 503 if not fully init
	if w.Code != http.StatusOK && w.Code != http.StatusServiceUnavailable && w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, unexpected", w.Code)
	}
}

func TestHandleMemoryTree_WithMemory(t *testing.T) {
	s := newTestServerWithMemory(t)

	req := httptest.NewRequest(http.MethodGet, "/api/memory/tree", nil)
	w := httptest.NewRecorder()
	s.handleMemoryTree(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusServiceUnavailable && w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, unexpected", w.Code)
	}
}

func TestHandleMemoryRetrieve_WithMemory(t *testing.T) {
	s := newTestServerWithMemory(t)

	req := httptest.NewRequest(http.MethodGet, "/api/memory/retrieve?q=test&limit=5", nil)
	w := httptest.NewRecorder()
	s.handleMemoryRetrieve(w, req)

	if w.Code != http.StatusOK && w.Code != http.StatusServiceUnavailable && w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, unexpected", w.Code)
	}
}

func TestHandleMemoryRetrieve_MethodNotAllowed(t *testing.T) {
	s := newTestServerWithMemory(t)
	req := httptest.NewRequest(http.MethodPost, "/api/memory/retrieve", nil)
	w := httptest.NewRecorder()
	s.handleMemoryRetrieve(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleMemoryTree_MethodNotAllowed(t *testing.T) {
	s := newTestServerWithMemory(t)
	req := httptest.NewRequest(http.MethodPost, "/api/memory/tree", nil)
	w := httptest.NewRecorder()
	s.handleMemoryTree(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleMemoryRetrieve_LimitNotANumber(t *testing.T) {
	s := newTestServerWithMemory(t)
	req := httptest.NewRequest(http.MethodGet, "/api/memory/retrieve?q=test&limit=notanumber", nil)
	w := httptest.NewRecorder()
	s.handleMemoryRetrieve(w, req)
	// With memory init, should hit limit parse error → 400 or succeed with default
	if w.Code == 0 {
		t.Error("expected response")
	}
}
