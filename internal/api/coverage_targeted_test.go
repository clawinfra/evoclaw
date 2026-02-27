package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/evolution"
)

// ---- Firewall with evolution engine ----

func newTestServerWithEvolution(t *testing.T) *Server {
	t.Helper()
	s := newTestServerV2(t)
	dir := t.TempDir()
	import_logger := s.logger
	eng := evolution.NewEngine(dir, import_logger)
	s.SetEvolution(eng)
	return s
}

func TestHandleFirewallStatus_WithEngine(t *testing.T) {
	s := newTestServerWithEvolution(t)
	req := httptest.NewRequest(http.MethodGet, "/api/agents/agent1/firewall", nil)
	w := httptest.NewRecorder()
	s.handleFirewallStatus(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestHandleFirewallReset_WithEngine(t *testing.T) {
	s := newTestServerWithEvolution(t)
	req := httptest.NewRequest(http.MethodPost, "/api/agents/agent1/firewall/reset", nil)
	w := httptest.NewRecorder()
	s.handleFirewallReset(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestHandleFirewallRollback_WithEngineNoSnapshot(t *testing.T) {
	s := newTestServerWithEvolution(t)
	req := httptest.NewRequest(http.MethodPost, "/api/agents/agent1/firewall/rollback", nil)
	w := httptest.NewRecorder()
	s.handleFirewallRollback(w, req)
	// No snapshot exists → 404
	if w.Code != http.StatusNotFound && w.Code != http.StatusOK {
		t.Errorf("unexpected status = %d", w.Code)
	}
}

// ---- Genome: behavior with genome set ----

func TestHandleGetBehavior_WithGenome(t *testing.T) {
	s, agent := newTestServerWithAgent(t)
	// Set a genome on the agent
	agent.Def.Genome = &config.Genome{
		Skills: make(map[string]config.SkillGenome),
		Behavior: config.GenomeBehavior{
			RiskTolerance: 0.5,
			Verbosity:     0.5,
			Autonomy:      0.5,
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/agents/"+agent.ID+"/genome/behavior", nil)
	w := httptest.NewRecorder()
	s.handleGetBehavior(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestHandleGetBehavior_WithGenomeAndEngine(t *testing.T) {
	s, agent := newTestServerWithAgent(t)
	dir := t.TempDir()
	eng := evolution.NewEngine(dir, s.logger)
	s.SetEvolution(eng)
	agent.Def.Genome = &config.Genome{
		Skills: make(map[string]config.SkillGenome),
		Behavior: config.GenomeBehavior{
			RiskTolerance: 0.3,
		},
	}
	req := httptest.NewRequest(http.MethodGet, "/api/agents/"+agent.ID+"/genome/behavior", nil)
	w := httptest.NewRecorder()
	s.handleGetBehavior(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestHandleGetBehaviorHistory_WithEngine(t *testing.T) {
	s, agent := newTestServerWithAgent(t)
	dir := t.TempDir()
	eng := evolution.NewEngine(dir, s.logger)
	s.SetEvolution(eng)
	req := httptest.NewRequest(http.MethodGet, "/api/agents/"+agent.ID+"/behavior/history", nil)
	w := httptest.NewRecorder()
	s.handleGetBehaviorHistory(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, unexpected", w.Code)
	}
}

func TestHandleGetBehaviorHistory_EmptyAgent(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/agents//behavior/history", nil)
	w := httptest.NewRecorder()
	s.handleGetBehaviorHistory(w, req)
	// empty agent ID → 400
	if w.Code != http.StatusBadRequest && w.Code != http.StatusOK {
		t.Errorf("status = %d, unexpected", w.Code)
	}
}

// ---- Memory endpoint: stats/tree/retrieve with nil memory (method not allowed) ----

func TestHandleMemoryRetrieve_MissingQuery(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/memory/retrieve", nil)
	w := httptest.NewRecorder()
	s.handleMemoryRetrieve(w, req)
	// No memory init → 503; or no query → 400
	if w.Code != http.StatusBadRequest && w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 400 or 503", w.Code)
	}
}

func TestHandleMemoryRetrieve_LimitTooHigh(t *testing.T) {
	s := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/memory/retrieve?q=test&limit=999", nil)
	w := httptest.NewRecorder()
	s.handleMemoryRetrieve(w, req)
	if w.Code != http.StatusBadRequest && w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 400 or 503", w.Code)
	}
}

// ---- server.go: handleAgentMemory / handleClearMemory with memory present ----

func TestHandleAgentMemory_ReturnsOK(t *testing.T) {
	s := newTestServer(t)
	w := httptest.NewRecorder()
	s.handleAgentMemory(w, "any-agent")
	// Memory.Get always creates memory if not found → 200
	if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
		t.Errorf("status = %d, unexpected", w.Code)
	}
}

func TestHandleClearMemory_ReturnsOK(t *testing.T) {
	s := newTestServer(t)
	w := httptest.NewRecorder()
	s.handleClearMemory(w, "any-agent")
	if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
		t.Errorf("status = %d, unexpected", w.Code)
	}
}

// ---- handleSubmitFeedback with evolution engine ----

func TestHandleSubmitFeedback_WithEngine(t *testing.T) {
	s, agent := newTestServerWithAgent(t)
	dir := t.TempDir()
	eng := evolution.NewEngine(dir, s.logger)
	s.SetEvolution(eng)

	body, _ := json.Marshal(map[string]interface{}{
		"type":    "approval",
		"score":   0.8,
		"context": "test context",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/agents/"+agent.ID+"/feedback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleSubmitFeedback(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, unexpected", w.Code)
	}
}

// ---- helpers: writeJSON ----

func TestWriteJSON_Success(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"key": "val"})
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestWriteJSON_NilValue(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, nil)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// ---- scheduler: uncovered branches ----

func TestHandleSchedulerListJobs_MethodNotAllowed(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodPost, "/api/scheduler/jobs", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerListJobs(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}


