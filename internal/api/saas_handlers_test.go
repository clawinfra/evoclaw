package api

import (
	"bytes"
	"context"
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
	"github.com/clawinfra/evoclaw/internal/saas"
)

func newTestServerWithSaaS(t *testing.T) (*Server, *httptest.Server, *saas.Service) {
	t.Helper()

	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	registry, _ := agents.NewRegistry(tmpDir, logger)
	memory, _ := agents.NewMemoryStore(tmpDir, logger)
	router := models.NewRouter(logger)
	orch := &orchestrator.Orchestrator{}

	// Mock E2B
	mockE2B := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/sandboxes":
			now := time.Now()
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"sandboxID": "sb-saas-test",
				"startedAt": now.Format(time.RFC3339),
				"endAt":     now.Add(5 * time.Minute).Format(time.RFC3339),
			})
		case r.Method == http.MethodGet && r.URL.Path == "/sandboxes":
			json.NewEncoder(w).Encode([]interface{}{})
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	config := cloud.DefaultManagerConfig()
	config.E2BAPIKey = "test-key"
	config.HealthCheckIntervalSec = 3600
	config.KeepAliveIntervalSec = 3600

	e2bClient := cloud.NewE2BClient(config.E2BAPIKey)
	e2bClient.SetBaseURL(mockE2B.URL)
	mgr := cloud.NewManagerWithClient(config, e2bClient, logger)

	store := saas.NewTenantStore()
	svc := saas.NewService(store, mgr, logger)

	srv := NewServer(8420, orch, registry, memory, router, logger)
	srv.SetSaaSService(svc)

	return srv, mockE2B, svc
}

func TestSetSaaSService(t *testing.T) {
	srv := newTestServer(t)
	if srv.saasSvc != nil {
		t.Error("expected nil saas service initially")
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	config := cloud.DefaultManagerConfig()
	mgr := cloud.NewManager(config, logger)
	store := saas.NewTenantStore()
	svc := saas.NewService(store, mgr, logger)
	srv.SetSaaSService(svc)

	if srv.saasSvc == nil {
		t.Error("expected non-nil saas service after set")
	}
}

func TestHandleSaaSRegister(t *testing.T) {
	srv, mockE2B, _ := newTestServerWithSaaS(t)
	defer mockE2B.Close()

	body, _ := json.Marshal(saas.RegisterRequest{
		Email:     "register@test.com",
		MaxAgents: 5,
	})

	req := httptest.NewRequest(http.MethodPost, "/api/saas/register", bytes.NewReader(body))
	w := httptest.NewRecorder()

	srv.handleSaaSRegister(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSaaSRegister_NoService(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/saas/register", nil)
	w := httptest.NewRecorder()

	srv.handleSaaSRegister(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleSaaSRegister_WrongMethod(t *testing.T) {
	srv, mockE2B, _ := newTestServerWithSaaS(t)
	defer mockE2B.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/saas/register", nil)
	w := httptest.NewRecorder()

	srv.handleSaaSRegister(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleSaaSRegister_BadBody(t *testing.T) {
	srv, mockE2B, _ := newTestServerWithSaaS(t)
	defer mockE2B.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/saas/register", bytes.NewReader([]byte("not json")))
	w := httptest.NewRecorder()

	srv.handleSaaSRegister(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleSaaSRegister_DuplicateEmail(t *testing.T) {
	srv, mockE2B, svc := newTestServerWithSaaS(t)
	defer mockE2B.Close()

	if _, err := svc.Register(saas.RegisterRequest{Email: "dupe@test.com"}); err != nil {
		t.Fatalf("failed to register first user: %v", err)
	}

	body, _ := json.Marshal(saas.RegisterRequest{Email: "dupe@test.com"})
	req := httptest.NewRequest(http.MethodPost, "/api/saas/register", bytes.NewReader(body))
	w := httptest.NewRecorder()

	srv.handleSaaSRegister(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestHandleSaaSAgents_List(t *testing.T) {
	srv, mockE2B, svc := newTestServerWithSaaS(t)
	defer mockE2B.Close()

	user, _ := svc.Register(saas.RegisterRequest{Email: "list@test.com"})

	req := httptest.NewRequest(http.MethodGet, "/api/saas/agents", nil)
	req.Header.Set("X-API-Key", user.APIKey)
	w := httptest.NewRecorder()

	srv.handleSaaSAgents(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSaaSAgents_Spawn(t *testing.T) {
	srv, mockE2B, svc := newTestServerWithSaaS(t)
	defer mockE2B.Close()

	user, _ := svc.Register(saas.RegisterRequest{Email: "spawn@test.com"})

	body, _ := json.Marshal(saas.SpawnRequest{
		AgentType: "trader",
		Mode:      "on-demand",
	})

	req := httptest.NewRequest(http.MethodPost, "/api/saas/agents", bytes.NewReader(body))
	req.Header.Set("X-API-Key", user.APIKey)
	w := httptest.NewRecorder()

	srv.handleSaaSAgents(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSaaSAgents_NoAuth(t *testing.T) {
	srv, mockE2B, _ := newTestServerWithSaaS(t)
	defer mockE2B.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/saas/agents", nil)
	w := httptest.NewRecorder()

	srv.handleSaaSAgents(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleSaaSAgents_NoService(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/saas/agents", nil)
	w := httptest.NewRecorder()

	srv.handleSaaSAgents(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleSaaSAgents_WrongMethod(t *testing.T) {
	srv, mockE2B, _ := newTestServerWithSaaS(t)
	defer mockE2B.Close()

	req := httptest.NewRequest(http.MethodPut, "/api/saas/agents", nil)
	w := httptest.NewRecorder()

	srv.handleSaaSAgents(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleSaaSAgents_BadBody(t *testing.T) {
	srv, mockE2B, svc := newTestServerWithSaaS(t)
	defer mockE2B.Close()

	user, _ := svc.Register(saas.RegisterRequest{Email: "bad@test.com"})

	req := httptest.NewRequest(http.MethodPost, "/api/saas/agents", bytes.NewReader([]byte("not json")))
	req.Header.Set("X-API-Key", user.APIKey)
	w := httptest.NewRecorder()

	srv.handleSaaSAgents(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleSaaSAgentDetail_Kill(t *testing.T) {
	srv, mockE2B, svc := newTestServerWithSaaS(t)
	defer mockE2B.Close()

	user, _ := svc.Register(saas.RegisterRequest{Email: "kill@test.com"})
	agent, _ := svc.SpawnAgent(context.Background(), user.ID, saas.SpawnRequest{AgentType: "trader"})

	req := httptest.NewRequest(http.MethodDelete, "/api/saas/agents/"+agent.SandboxID, nil)
	req.Header.Set("X-API-Key", user.APIKey)
	w := httptest.NewRecorder()

	srv.handleSaaSAgentDetail(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSaaSAgentDetail_NoService(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodDelete, "/api/saas/agents/sb-1", nil)
	w := httptest.NewRecorder()

	srv.handleSaaSAgentDetail(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleSaaSAgentDetail_WrongMethod(t *testing.T) {
	srv, mockE2B, _ := newTestServerWithSaaS(t)
	defer mockE2B.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/saas/agents/sb-1", nil)
	w := httptest.NewRecorder()

	srv.handleSaaSAgentDetail(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleSaaSAgentDetail_NoAuth(t *testing.T) {
	srv, mockE2B, _ := newTestServerWithSaaS(t)
	defer mockE2B.Close()

	req := httptest.NewRequest(http.MethodDelete, "/api/saas/agents/sb-1", nil)
	w := httptest.NewRecorder()

	srv.handleSaaSAgentDetail(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleSaaSAgentDetail_EmptyID(t *testing.T) {
	srv, mockE2B, svc := newTestServerWithSaaS(t)
	defer mockE2B.Close()

	user, _ := svc.Register(saas.RegisterRequest{Email: "empty@test.com"})

	req := httptest.NewRequest(http.MethodDelete, "/api/saas/agents/", nil)
	req.Header.Set("X-API-Key", user.APIKey)
	w := httptest.NewRecorder()

	srv.handleSaaSAgentDetail(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleSaaSUsage(t *testing.T) {
	srv, mockE2B, svc := newTestServerWithSaaS(t)
	defer mockE2B.Close()

	user, _ := svc.Register(saas.RegisterRequest{Email: "usage@test.com"})

	req := httptest.NewRequest(http.MethodGet, "/api/saas/usage", nil)
	req.Header.Set("X-API-Key", user.APIKey)
	w := httptest.NewRecorder()

	srv.handleSaaSUsage(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleSaaSUsage_NoService(t *testing.T) {
	srv := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/saas/usage", nil)
	w := httptest.NewRecorder()

	srv.handleSaaSUsage(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleSaaSUsage_WrongMethod(t *testing.T) {
	srv, mockE2B, _ := newTestServerWithSaaS(t)
	defer mockE2B.Close()

	req := httptest.NewRequest(http.MethodPost, "/api/saas/usage", nil)
	w := httptest.NewRecorder()

	srv.handleSaaSUsage(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

func TestHandleSaaSUsage_NoAuth(t *testing.T) {
	srv, mockE2B, _ := newTestServerWithSaaS(t)
	defer mockE2B.Close()

	req := httptest.NewRequest(http.MethodGet, "/api/saas/usage", nil)
	w := httptest.NewRecorder()

	srv.handleSaaSUsage(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestExtractSaaSUser_Bearer(t *testing.T) {
	srv, mockE2B, svc := newTestServerWithSaaS(t)
	defer mockE2B.Close()

	user, _ := svc.Register(saas.RegisterRequest{Email: "bearer@test.com"})

	req := httptest.NewRequest(http.MethodGet, "/api/saas/agents", nil)
	req.Header.Set("Authorization", "Bearer "+user.APIKey)
	w := httptest.NewRecorder()

	srv.handleSaaSAgents(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with Bearer auth, got %d", w.Code)
	}
}
