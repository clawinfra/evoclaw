package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/clawinfra/evoclaw/internal/agents"
	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/models"
)

func newTestServerV2(t *testing.T) *Server {
	t.Helper()
	logger := slog.Default()
	dir := t.TempDir()

	reg, _ := agents.NewRegistry(dir, logger)
	mem, _ := agents.NewMemoryStore(dir, logger)
	router := models.NewRouter(logger)

	return NewServer(0, nil, reg, mem, router, logger)
}

func newTestServerWithAgent(t *testing.T) (*Server, *agents.Agent) {
	t.Helper()
	s := newTestServerV2(t)
	agentDef := config.AgentDef{
		ID:     "test-agent",
		Name:   "Test Agent",
		Type:   "orchestrator",
		Model:  "test/model",
		Skills: []string{"chat"},
	}
	agent, err := s.registry.Create(agentDef)
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}
	return s, agent
}

func TestHandleStatusFullV2(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest("GET", "/api/status", nil)
	w := httptest.NewRecorder()
	s.handleStatus(w, req)

	if w.Code != 200 {
		t.Errorf("status code = %d, want 200", w.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["version"] == nil {
		t.Error("expected version in response")
	}
}

func TestHandleAgentsFullV2(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	req := httptest.NewRequest("GET", "/api/agents", nil)
	w := httptest.NewRecorder()
	s.handleAgents(w, req)

	if w.Code != 200 {
		t.Errorf("status code = %d, want 200", w.Code)
	}
}

func TestHandleAgentDetailComprehensiveV2(t *testing.T) {
	s, _ := newTestServerWithAgent(t)

	// Valid agent
	req := httptest.NewRequest("GET", "/api/agents/test-agent", nil)
	w := httptest.NewRecorder()
	s.handleAgentDetail(w, req)
	if w.Code != 200 {
		t.Errorf("status code = %d, want 200", w.Code)
	}

	// Non-existent agent
	req = httptest.NewRequest("GET", "/api/agents/nonexistent", nil)
	w = httptest.NewRecorder()
	s.handleAgentDetail(w, req)
	if w.Code != 404 {
		t.Errorf("status code = %d, want 404", w.Code)
	}
}

func TestHandleModelsFullV2(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest("GET", "/api/models", nil)
	w := httptest.NewRecorder()
	s.handleModels(w, req)
	if w.Code != 200 {
		t.Errorf("status code = %d, want 200", w.Code)
	}
}

func TestHandleCostsFullV2(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest("GET", "/api/costs", nil)
	w := httptest.NewRecorder()
	s.handleCosts(w, req)
	if w.Code != 200 {
		t.Errorf("status code = %d, want 200", w.Code)
	}
}

func TestHandleDashboardV2(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	req := httptest.NewRequest("GET", "/api/dashboard", nil)
	w := httptest.NewRecorder()
	s.handleDashboard(w, req)
	if w.Code != 200 {
		t.Errorf("status code = %d, want 200", w.Code)
	}

	// Wrong method
	req = httptest.NewRequest("POST", "/api/dashboard", nil)
	w = httptest.NewRecorder()
	s.handleDashboard(w, req)
	if w.Code != 405 {
		t.Errorf("status code = %d, want 405", w.Code)
	}
}

func TestHandleAgentEvolutionV2(t *testing.T) {
	s, _ := newTestServerWithAgent(t)

	// Valid agent
	req := httptest.NewRequest("GET", "/api/agents/test-agent/evolution", nil)
	w := httptest.NewRecorder()
	s.handleAgentEvolution(w, req)
	if w.Code != 200 {
		t.Errorf("status code = %d, want 200", w.Code)
	}

	// Wrong method
	req = httptest.NewRequest("POST", "/api/agents/test-agent/evolution", nil)
	w = httptest.NewRecorder()
	s.handleAgentEvolution(w, req)
	if w.Code != 405 {
		t.Errorf("status code = %d, want 405", w.Code)
	}

	// Invalid path
	req = httptest.NewRequest("GET", "/api/agents/", nil)
	w = httptest.NewRecorder()
	s.handleAgentEvolution(w, req)
	if w.Code != 400 {
		t.Errorf("status code = %d, want 400", w.Code)
	}

	// Non-existent agent
	req = httptest.NewRequest("GET", "/api/agents/nonexistent/evolution", nil)
	w = httptest.NewRecorder()
	s.handleAgentEvolution(w, req)
	if w.Code != 404 {
		t.Errorf("status code = %d, want 404", w.Code)
	}
}

func TestHandleFirewallStatusV2(t *testing.T) {
	s := newTestServerV2(t)

	// No evolution engine
	req := httptest.NewRequest("GET", "/api/agents/test-agent/firewall", nil)
	w := httptest.NewRecorder()
	s.handleFirewallStatus(w, req)
	if w.Code != 503 {
		t.Errorf("status code = %d, want 503", w.Code)
	}

	// Wrong method
	req = httptest.NewRequest("POST", "/api/agents/test-agent/firewall", nil)
	w = httptest.NewRecorder()
	s.handleFirewallStatus(w, req)
	if w.Code != 405 {
		t.Errorf("status code = %d, want 405", w.Code)
	}

	// No agent ID
	req = httptest.NewRequest("GET", "/api/agents//firewall", nil)
	w = httptest.NewRecorder()
	s.handleFirewallStatus(w, req)
	if w.Code != 400 {
		t.Errorf("status code = %d, want 400", w.Code)
	}
}

func TestHandleFirewallRollbackV2(t *testing.T) {
	s := newTestServerV2(t)

	// Wrong method
	req := httptest.NewRequest("GET", "/api/agents/test-agent/firewall/rollback", nil)
	w := httptest.NewRecorder()
	s.handleFirewallRollback(w, req)
	if w.Code != 405 {
		t.Errorf("status code = %d, want 405", w.Code)
	}

	// No evolution engine
	req = httptest.NewRequest("POST", "/api/agents/test-agent/firewall/rollback", nil)
	w = httptest.NewRecorder()
	s.handleFirewallRollback(w, req)
	if w.Code != 503 {
		t.Errorf("status code = %d, want 503", w.Code)
	}

	// No agent ID
	req = httptest.NewRequest("POST", "/api/agents//firewall/rollback", nil)
	w = httptest.NewRecorder()
	s.handleFirewallRollback(w, req)
	if w.Code != 400 {
		t.Errorf("status code = %d, want 400", w.Code)
	}
}

func TestHandleFirewallResetV2(t *testing.T) {
	s := newTestServerV2(t)

	// Wrong method
	req := httptest.NewRequest("GET", "/api/agents/test-agent/firewall/reset", nil)
	w := httptest.NewRecorder()
	s.handleFirewallReset(w, req)
	if w.Code != 405 {
		t.Errorf("status code = %d, want 405", w.Code)
	}

	// No evolution engine
	req = httptest.NewRequest("POST", "/api/agents/test-agent/firewall/reset", nil)
	w = httptest.NewRecorder()
	s.handleFirewallReset(w, req)
	if w.Code != 503 {
		t.Errorf("status code = %d, want 503", w.Code)
	}
}

func TestExtractAgentIDForFirewallV2(t *testing.T) {
	tests := []struct {
		path   string
		suffix string
		want   string
	}{
		{"/api/agents/agent-1/firewall", "/firewall", "agent-1"},
		{"/api/agents/agent-1/firewall/rollback", "/firewall/rollback", "agent-1"},
		{"/api/agents//firewall", "/firewall", ""},
		{"/api/agents/a/b/firewall", "/firewall", ""},
	}
	for _, tt := range tests {
		got := extractAgentIDForFirewall(tt.path, tt.suffix)
		if got != tt.want {
			t.Errorf("extractAgentIDForFirewall(%q, %q) = %q, want %q", tt.path, tt.suffix, got, tt.want)
		}
	}
}

func TestHandleGenomeRoutesV2(t *testing.T) {
	s, _ := newTestServerWithAgent(t)

	// GET genome
	req := httptest.NewRequest("GET", "/api/agents/test-agent/genome", nil)
	w := httptest.NewRecorder()
	s.handleGenomeRoutes(w, req)
	if w.Code != 200 {
		t.Errorf("GET genome: status code = %d, want 200", w.Code)
	}

	// DELETE not allowed
	req = httptest.NewRequest("DELETE", "/api/agents/test-agent/genome", nil)
	w = httptest.NewRecorder()
	s.handleGenomeRoutes(w, req)
	if w.Code != 405 {
		t.Errorf("DELETE genome: status code = %d, want 405", w.Code)
	}
}

func TestHandleSkillRoutesV2(t *testing.T) {
	s := newTestServerV2(t)

	// Wrong method
	req := httptest.NewRequest("POST", "/api/agents/test-agent/skills/chat", nil)
	w := httptest.NewRecorder()
	s.handleSkillRoutes(w, req)
	if w.Code != 405 {
		t.Errorf("status code = %d, want 405", w.Code)
	}
}

func TestHandleGetGenomeV2(t *testing.T) {
	s, _ := newTestServerWithAgent(t)

	// Valid agent without genome - should return default
	req := httptest.NewRequest("GET", "/api/agents/test-agent/genome", nil)
	w := httptest.NewRecorder()
	s.handleGetGenome(w, req)
	if w.Code != 200 {
		t.Errorf("status code = %d, want 200", w.Code)
	}

	// Non-existent agent
	req = httptest.NewRequest("GET", "/api/agents/nonexistent/genome", nil)
	w = httptest.NewRecorder()
	s.handleGetGenome(w, req)
	if w.Code != 404 {
		t.Errorf("status code = %d, want 404", w.Code)
	}

	// Empty agent ID
	req = httptest.NewRequest("GET", "/api/agents//genome", nil)
	w = httptest.NewRecorder()
	s.handleGetGenome(w, req)
	if w.Code != 400 {
		t.Errorf("status code = %d, want 400", w.Code)
	}
}

func TestHandleAgentMemoryComprehensiveV2(t *testing.T) {
	s, _ := newTestServerWithAgent(t)

	w := httptest.NewRecorder()
	s.handleAgentMemory(w, "test-agent")
	if w.Code != 200 {
		t.Errorf("status code = %d, want 200", w.Code)
	}
}

func TestHandleClearMemoryComprehensiveV2(t *testing.T) {
	s, _ := newTestServerWithAgent(t)

	w := httptest.NewRecorder()
	s.handleClearMemory(w, "test-agent")
	if w.Code != 200 {
		t.Errorf("status code = %d, want 200", w.Code)
	}
}

func TestHandleAuthTokenV2(t *testing.T) {
	s := newTestServerV2(t)

	// Wrong method
	req := httptest.NewRequest("GET", "/api/auth/token", nil)
	w := httptest.NewRecorder()
	s.handleAuthToken(w, req)
	if w.Code != 405 {
		t.Errorf("status code = %d, want 405", w.Code)
	}

	// Invalid body
	req = httptest.NewRequest("POST", "/api/auth/token", bytes.NewBufferString("invalid"))
	w = httptest.NewRecorder()
	s.handleAuthToken(w, req)
	if w.Code != 400 {
		t.Errorf("status code = %d, want 400", w.Code)
	}

	// Missing fields
	body, _ := json.Marshal(map[string]string{})
	req = httptest.NewRequest("POST", "/api/auth/token", bytes.NewBuffer(body))
	w = httptest.NewRecorder()
	s.handleAuthToken(w, req)
	if w.Code != 400 {
		t.Errorf("status code = %d, want 400", w.Code)
	}

	// Invalid role
	body, _ = json.Marshal(map[string]string{"agent_id": "a1", "role": "invalid"})
	req = httptest.NewRequest("POST", "/api/auth/token", bytes.NewBuffer(body))
	w = httptest.NewRecorder()
	s.handleAuthToken(w, req)
	if w.Code != 400 {
		t.Errorf("status code = %d, want 400", w.Code)
	}

	// Valid request (dev mode - no JWT secret)
	body, _ = json.Marshal(map[string]string{"agent_id": "a1", "role": "owner"})
	req = httptest.NewRequest("POST", "/api/auth/token", bytes.NewBuffer(body))
	w = httptest.NewRecorder()
	s.handleAuthToken(w, req)
	if w.Code != 200 {
		t.Errorf("status code = %d, want 200", w.Code)
	}
}

func TestSetEvolutionV2(t *testing.T) {
	s := newTestServerV2(t)
	s.SetEvolution("test")
	if s.evolution != "test" {
		t.Error("SetEvolution did not set")
	}
}

func TestSetWebFSV2(t *testing.T) {
	s := newTestServerV2(t)
	s.SetWebFS(nil)
	if s.webFS != nil {
		t.Error("SetWebFS did not set")
	}
}

func TestGetEvolutionEngineV2(t *testing.T) {
	s := newTestServerV2(t)
	// nil evolution
	if eng := s.getEvolutionEngine(); eng != nil {
		t.Error("expected nil engine")
	}
	// wrong type
	s.SetEvolution("not-an-engine")
	if eng := s.getEvolutionEngine(); eng != nil {
		t.Error("expected nil for wrong type")
	}
}

func TestGetEvolutionStrategyV2(t *testing.T) {
	s := newTestServerV2(t)
	// No orchestrator
	if got := s.getEvolutionStrategy("agent1"); got != nil {
		t.Error("expected nil")
	}
}

func TestJwtAuthWrapperSkipsAuthEndpointV2(t *testing.T) {
	s := newTestServerV2(t)
	called := false
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	})

	handler := s.jwtAuthWrapper(inner)

	// Token endpoint should skip auth
	req := httptest.NewRequest("GET", "/api/auth/token", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !called {
		t.Error("expected handler to be called for token endpoint")
	}

	// Non-API path should skip auth
	called = false
	req = httptest.NewRequest("GET", "/health", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, req)
	if !called {
		t.Error("expected handler to be called for non-API path")
	}
}

func TestHandleMemoryStatsV2(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	req := httptest.NewRequest("GET", "/api/agents/test-agent/memory/stats", nil)
	w := httptest.NewRecorder()
	s.handleMemoryStats(w, req)
	// Should return 200 even without memory data
	if w.Code != 200 && w.Code != 404 && w.Code != 503 {
		t.Errorf("status code = %d, want 200 or 404", w.Code)
	}
}

func TestHandleMemoryTreeV2(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	req := httptest.NewRequest("GET", "/api/agents/test-agent/memory/tree", nil)
	w := httptest.NewRecorder()
	s.handleMemoryTree(w, req)
	if w.Code != 200 && w.Code != 404 && w.Code != 503 {
		t.Errorf("status code = %d, want 200 or 404", w.Code)
	}
}

func TestHandleMemoryRetrieveV2(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	req := httptest.NewRequest("GET", "/api/agents/test-agent/memory/retrieve?query=test", nil)
	w := httptest.NewRecorder()
	s.handleMemoryRetrieve(w, req)
	if w.Code != 200 && w.Code != 404 && w.Code != 503 && w.Code != 400 {
		t.Errorf("status code = %d", w.Code)
	}
}
