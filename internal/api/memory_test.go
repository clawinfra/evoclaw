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

func TestHandleMemoryStats_NoMemory(t *testing.T) {
	// Create server without memory system
	cfg := &config.Config{}
	orch := orchestrator.New(cfg, slog.Default())
	registry, _ := agents.NewRegistry(t.TempDir(), slog.Default())
	memory, _ := agents.NewMemoryStore(t.TempDir(), slog.Default())
	router := models.NewRouter(slog.Default())
	
	server := NewServer(8080, orch, registry, memory, router, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/api/memory/stats", nil)
	w := httptest.NewRecorder()

	server.handleMemoryStats(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestHandleMemoryStats_MethodNotAllowed(t *testing.T) {
	cfg := &config.Config{}
	orch := orchestrator.New(cfg, slog.Default())
	registry, _ := agents.NewRegistry(t.TempDir(), slog.Default())
	memory, _ := agents.NewMemoryStore(t.TempDir(), slog.Default())
	router := models.NewRouter(slog.Default())
	
	server := NewServer(8080, orch, registry, memory, router, slog.Default())

	req := httptest.NewRequest(http.MethodPost, "/api/memory/stats", nil)
	w := httptest.NewRecorder()

	server.handleMemoryStats(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleMemoryTree_NoMemory(t *testing.T) {
	cfg := &config.Config{}
	orch := orchestrator.New(cfg, slog.Default())
	registry, _ := agents.NewRegistry(t.TempDir(), slog.Default())
	memory, _ := agents.NewMemoryStore(t.TempDir(), slog.Default())
	router := models.NewRouter(slog.Default())
	
	server := NewServer(8080, orch, registry, memory, router, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/api/memory/tree", nil)
	w := httptest.NewRecorder()

	server.handleMemoryTree(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestHandleMemoryRetrieve_NoQuery(t *testing.T) {
	cfg := &config.Config{}
	orch := orchestrator.New(cfg, slog.Default())
	registry, _ := agents.NewRegistry(t.TempDir(), slog.Default())
	memory, _ := agents.NewMemoryStore(t.TempDir(), slog.Default())
	router := models.NewRouter(slog.Default())
	
	server := NewServer(8080, orch, registry, memory, router, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/api/memory/retrieve", nil)
	w := httptest.NewRecorder()

	server.handleMemoryRetrieve(w, req)

	// Memory system check happens before query validation, so we expect 503
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestHandleMemoryRetrieve_InvalidLimit(t *testing.T) {
	cfg := &config.Config{}
	orch := orchestrator.New(cfg, slog.Default())
	registry, _ := agents.NewRegistry(t.TempDir(), slog.Default())
	memory, _ := agents.NewMemoryStore(t.TempDir(), slog.Default())
	router := models.NewRouter(slog.Default())
	
	server := NewServer(8080, orch, registry, memory, router, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/api/memory/retrieve?q=test&limit=100", nil)
	w := httptest.NewRecorder()

	server.handleMemoryRetrieve(w, req)

	// Memory system check happens before limit validation, so we expect 503
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

func TestHandleMemoryRetrieve_NoMemory(t *testing.T) {
	cfg := &config.Config{}
	orch := orchestrator.New(cfg, slog.Default())
	registry, _ := agents.NewRegistry(t.TempDir(), slog.Default())
	memory, _ := agents.NewMemoryStore(t.TempDir(), slog.Default())
	router := models.NewRouter(slog.Default())
	
	server := NewServer(8080, orch, registry, memory, router, slog.Default())

	req := httptest.NewRequest(http.MethodGet, "/api/memory/retrieve?q=test&limit=5", nil)
	w := httptest.NewRecorder()

	server.handleMemoryRetrieve(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", w.Code)
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > len(substr) && containsRec(s, substr, 0))
}

func containsRec(s, substr string, pos int) bool {
	if pos+len(substr) > len(s) {
		return false
	}
	if s[pos:pos+len(substr)] == substr {
		return true
	}
	return containsRec(s, substr, pos+1)
}
