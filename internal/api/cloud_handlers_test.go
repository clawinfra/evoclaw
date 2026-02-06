package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/clawinfra/evoclaw/internal/agents"
	"github.com/clawinfra/evoclaw/internal/cloud"
	"github.com/clawinfra/evoclaw/internal/models"
	"github.com/clawinfra/evoclaw/internal/orchestrator"
)

// newMockE2BServer creates a mock E2B API for cloud handler tests.
func newMockE2BServer() *httptest.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("POST /sandboxes", func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"sandboxID":  "sb-cloud-test-1",
			"templateID": "evoclaw-agent",
			"clientID":   "cl-test-1",
			"startedAt":  now.Format(time.RFC3339),
			"endAt":      now.Add(5 * time.Minute).Format(time.RFC3339),
			"metadata":   map[string]string{"evoclaw_agent_id": "test-agent"},
		})
	})

	mux.HandleFunc("GET /sandboxes", func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		json.NewEncoder(w).Encode([]map[string]interface{}{
			{
				"sandboxID":  "sb-cloud-test-1",
				"templateID": "evoclaw-agent",
				"clientID":   "cl-test-1",
				"startedAt":  now.Add(-5 * time.Minute).Format(time.RFC3339),
				"endAt":      now.Add(5 * time.Minute).Format(time.RFC3339),
				"metadata":   map[string]string{"evoclaw_agent_id": "agent-1"},
			},
		})
	})

	mux.HandleFunc("GET /sandboxes/sb-cloud-test-1", func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"sandboxID":  "sb-cloud-test-1",
			"templateID": "evoclaw-agent",
			"startedAt":  now.Add(-5 * time.Minute).Format(time.RFC3339),
			"endAt":      now.Add(5 * time.Minute).Format(time.RFC3339),
			"metadata":   map[string]string{"evoclaw_agent_id": "agent-1"},
		})
	})

	mux.HandleFunc("DELETE /sandboxes/sb-cloud-test-1", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	return httptest.NewServer(mux)
}

func newTestServerWithCloud(t *testing.T) (*Server, *httptest.Server) {
	t.Helper()

	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	registry, _ := agents.NewRegistry(tmpDir, logger)
	memory, _ := agents.NewMemoryStore(tmpDir, logger)
	router := models.NewRouter(logger)
	orch := &orchestrator.Orchestrator{}

	mockE2B := newMockE2BServer()

	config := cloud.DefaultManagerConfig()
	config.E2BAPIKey = "test-key"
	config.HealthCheckIntervalSec = 3600
	config.KeepAliveIntervalSec = 3600

	e2bClient := cloud.NewE2BClient(config.E2BAPIKey)
	e2bClient.SetBaseURL(mockE2B.URL)

	mgr := cloud.NewManagerWithClient(config, e2bClient, logger)

	srv := NewServer(8420, orch, registry, memory, router, logger)
	srv.SetCloudManager(mgr)

	return srv, mockE2B
}

func TestSetCloudManager(t *testing.T) {
	srv := newTestServer(t)
	if srv.cloudMgr != nil {
		t.Error("expected nil cloud manager initially")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	config := cloud.DefaultManagerConfig()
	mgr := cloud.NewManager(config, logger)
	srv.SetCloudManager(mgr)

	if srv.cloudMgr == nil {
		t.Error("expected non-nil cloud manager after set")
	}
}

func TestHandleCloudList(t *testing.T) {
	srv, mockE2B := newTestServerWithCloud(t)
	defer mockE2B.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/cloud", nil)
	w := httptest.NewRecorder()

	srv.handleCloudList(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleCloudList_NoManager(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/cloud", nil)
	w := httptest.NewRecorder()

	srv.handleCloudList(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleCloudList_WrongMethod(t *testing.T) {
	srv, mockE2B := newTestServerWithCloud(t)
	defer mockE2B.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/cloud", nil)
	w := httptest.NewRecorder()

	srv.handleCloudList(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleCloudSpawn(t *testing.T) {
	srv, mockE2B := newTestServerWithCloud(t)
	defer mockE2B.Close()

	body := cloud.AgentConfig{
		TemplateID: "evoclaw-agent",
		AgentID:    "spawn-test",
		AgentType:  "trader",
	}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/cloud/spawn", bytes.NewReader(bodyBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	srv.handleCloudSpawn(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCloudSpawn_NoManager(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/cloud/spawn", nil)
	w := httptest.NewRecorder()

	srv.handleCloudSpawn(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleCloudSpawn_WrongMethod(t *testing.T) {
	srv, mockE2B := newTestServerWithCloud(t)
	defer mockE2B.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/cloud/spawn", nil)
	w := httptest.NewRecorder()

	srv.handleCloudSpawn(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleCloudSpawn_BadBody(t *testing.T) {
	srv, mockE2B := newTestServerWithCloud(t)
	defer mockE2B.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/cloud/spawn", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()

	srv.handleCloudSpawn(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleCloudCosts(t *testing.T) {
	srv, mockE2B := newTestServerWithCloud(t)
	defer mockE2B.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/cloud/costs", nil)
	w := httptest.NewRecorder()

	srv.handleCloudCosts(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var costs cloud.CostSnapshot
	json.NewDecoder(w.Body).Decode(&costs)

	if costs.BudgetUSD <= 0 {
		t.Error("expected positive budget")
	}
}

func TestHandleCloudCosts_NoManager(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/cloud/costs", nil)
	w := httptest.NewRecorder()

	srv.handleCloudCosts(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleCloudCosts_WrongMethod(t *testing.T) {
	srv, mockE2B := newTestServerWithCloud(t)
	defer mockE2B.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/cloud/costs", nil)
	w := httptest.NewRecorder()

	srv.handleCloudCosts(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleCloudDetail_GetStatus(t *testing.T) {
	srv, mockE2B := newTestServerWithCloud(t)
	defer mockE2B.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/cloud/sb-cloud-test-1", nil)
	w := httptest.NewRecorder()

	srv.handleCloudDetail(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCloudDetail_Kill(t *testing.T) {
	srv, mockE2B := newTestServerWithCloud(t)
	defer mockE2B.Close()

	req := httptest.NewRequest(http.MethodDelete, "/api/cloud/sb-cloud-test-1", nil)
	w := httptest.NewRecorder()

	srv.handleCloudDetail(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleCloudDetail_NoManager(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/cloud/sb-1", nil)
	w := httptest.NewRecorder()

	srv.handleCloudDetail(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleCloudDetail_EmptyID(t *testing.T) {
	srv, mockE2B := newTestServerWithCloud(t)
	defer mockE2B.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/cloud/", nil)
	w := httptest.NewRecorder()

	srv.handleCloudDetail(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleCloudDetail_WrongMethod(t *testing.T) {
	srv, mockE2B := newTestServerWithCloud(t)
	defer mockE2B.Close()

	req := httptest.NewRequest(http.MethodPut, "/api/cloud/sb-1", nil)
	w := httptest.NewRecorder()

	srv.handleCloudDetail(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}
