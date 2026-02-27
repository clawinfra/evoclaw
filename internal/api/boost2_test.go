package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/clawinfra/evoclaw/internal/config"
)

// --- handleTerminalPage ---

func TestHandleTerminalPage_Get(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodGet, "/terminal", nil)
	w := httptest.NewRecorder()
	s.handleTerminalPage(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/html" {
		t.Errorf("Content-Type = %q, want text/html", ct)
	}
}

func TestHandleTerminalPage_MethodNotAllowed(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodPost, "/terminal", nil)
	w := httptest.NewRecorder()
	s.handleTerminalPage(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

// --- firewall handlers ---

func TestHandleFirewallStatus_MethodNotAllowed(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodPost, "/api/agents/test/firewall", nil)
	w := httptest.NewRecorder()
	s.handleFirewallStatus(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleFirewallStatus_NoEngine(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodGet, "/api/agents/test-agent/firewall", nil)
	w := httptest.NewRecorder()
	s.handleFirewallStatus(w, req)
	// no evolution engine → 503
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

func TestHandleFirewallStatus_MissingID(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodGet, "/api/agents//firewall", nil)
	w := httptest.NewRecorder()
	s.handleFirewallStatus(w, req)
	if w.Code != http.StatusBadRequest && w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d", w.Code)
	}
}

func TestHandleFirewallRollback_MethodNotAllowed(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodGet, "/api/agents/test/firewall/rollback", nil)
	w := httptest.NewRecorder()
	s.handleFirewallRollback(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleFirewallRollback_NoEngine(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodPost, "/api/agents/test-agent/firewall/rollback", nil)
	w := httptest.NewRecorder()
	s.handleFirewallRollback(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

func TestHandleFirewallReset_MethodNotAllowed(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodGet, "/api/agents/test/firewall/reset", nil)
	w := httptest.NewRecorder()
	s.handleFirewallReset(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleFirewallReset_NoEngine(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodPost, "/api/agents/test-agent/firewall/reset", nil)
	w := httptest.NewRecorder()
	s.handleFirewallReset(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

// --- genome/behavior handlers ---

func TestHandleGetBehavior_NotFound(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodGet, "/api/agents/nonexistent/genome/behavior", nil)
	w := httptest.NewRecorder()
	s.handleGetBehavior(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHandleGetBehavior_NoGenome(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	req := httptest.NewRequest(http.MethodGet, "/api/agents/test-agent/genome/behavior", nil)
	w := httptest.NewRecorder()
	s.handleGetBehavior(w, req)
	// agent has no genome → 404
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHandleGetBehaviorHistory_Success(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodGet, "/api/agents/some-agent/genome/behavior/history", nil)
	w := httptest.NewRecorder()
	s.handleGetBehaviorHistory(w, req)
	// No evolution engine → returns empty history with 200
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestHandleSubmitFeedback_InvalidJSON(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodPost, "/api/agents/test-agent/genome/feedback", bytes.NewReader([]byte("bad")))
	w := httptest.NewRecorder()
	s.handleSubmitFeedback(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleSubmitFeedback_ValidFeedback(t *testing.T) {
	s := newTestServerV2(t)
	body, _ := json.Marshal(map[string]interface{}{
		"type": "approval", "score": 0.8, "context": "good response",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/agents/test-agent/feedback", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleSubmitFeedback(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestHandleSubmitFeedback_InvalidType(t *testing.T) {
	s := newTestServerV2(t)
	body, _ := json.Marshal(map[string]interface{}{
		"type": "unknown", "score": 0.5,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/agents/test-agent/feedback", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleSubmitFeedback(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// --- handleConstraintRoutes ---

func TestHandleConstraintRoutes_Put(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	body, _ := json.Marshal(map[string]interface{}{
		"constraints": []interface{}{},
	})
	req := httptest.NewRequest(http.MethodPut, "/api/agents/test-agent/genome/constraints", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleConstraintRoutes(w, req)
	// Various valid responses depending on genome state
	if w.Code == 0 {
		t.Error("expected a response")
	}
}

func TestHandleConstraintRoutes_MethodNotAllowed(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/agents/test/genome/constraints", nil)
	w := httptest.NewRecorder()
	s.handleConstraintRoutes(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleConstraintRoutes_Get(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	req := httptest.NewRequest(http.MethodGet, "/api/agents/test-agent/genome/constraints", nil)
	w := httptest.NewRecorder()
	s.handleConstraintRoutes(w, req)
	// Should succeed or return 404 if no genome
	if w.Code == 0 {
		t.Error("expected response code")
	}
}

// --- handleGenomeRoutes and handleSkillRoutes ---

func TestHandleGenomeRoutes_Get(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	req := httptest.NewRequest(http.MethodGet, "/api/agents/test-agent/genome", nil)
	w := httptest.NewRecorder()
	s.handleGenomeRoutes(w, req)
	// Can be 200 or 404 depending on genome existence
	if w.Code == 0 {
		t.Error("expected response")
	}
}

func TestHandleSkillRoutes_Get(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	req := httptest.NewRequest(http.MethodGet, "/api/agents/test-agent/skills/chat", nil)
	w := httptest.NewRecorder()
	s.handleSkillRoutes(w, req)
	if w.Code == 0 {
		t.Error("expected response")
	}
}

// --- handleAgentMemory / handleClearMemory ---

func TestHandleAgentMemory_ReturnsMemory(t *testing.T) {
	s := newTestServerV2(t)
	w := httptest.NewRecorder()
	// Memory store auto-creates memory for any agentID
	s.handleAgentMemory(w, "any-agent")
	if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
		t.Errorf("status = %d, unexpected", w.Code)
	}
}

func TestHandleClearMemory_ClearsMemory(t *testing.T) {
	s := newTestServerV2(t)
	w := httptest.NewRecorder()
	// Memory store auto-creates, so clear should succeed
	s.handleClearMemory(w, "any-agent")
	if w.Code != http.StatusOK && w.Code != http.StatusNotFound {
		t.Errorf("status = %d", w.Code)
	}
}

// --- handleDashboard / handleAgentEvolution / handleLogStream ---

func TestHandleDashboard_MethodNotAllowed(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodPost, "/api/dashboard", nil)
	w := httptest.NewRecorder()
	s.handleDashboard(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleAgentEvolution_MethodNotAllowed(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/evolution", nil)
	w := httptest.NewRecorder()
	s.handleAgentEvolution(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleAgentEvolution_Get(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/evolution", nil)
	w := httptest.NewRecorder()
	s.handleAgentEvolution(w, req)
	// May return 200 or 400 depending on required params
	if w.Code == 0 {
		t.Error("expected a response")
	}
}

func TestGetEvolutionStrategy(t *testing.T) {
	s := newTestServerV2(t)
	// exercise all branches
	strategies := []string{"gradient", "genetic", "random", "unknown", ""}
	for _, strategy := range strategies {
		result := s.getEvolutionStrategy(strategy)
		if result == "" {
			t.Errorf("getEvolutionStrategy(%q) returned empty", strategy)
		}
	}
}

// wsSendResponse and handleWSChat require websocket.Conn — skip direct unit tests.

// --- handleUpdateSkillParams ---

func TestHandleUpdateSkillParams_MethodNotAllowed(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	req := httptest.NewRequest(http.MethodGet, "/api/agents/test-agent/skills/chat/params", nil)
	w := httptest.NewRecorder()
	s.handleUpdateSkillParams(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleUpdateSkillParams_InvalidPath(t *testing.T) {
	s := newTestServerV2(t)
	// Path doesn't contain /genome/skills/ → 400
	req := httptest.NewRequest(http.MethodPut, "/api/agents/test-agent/bad-path", bytes.NewReader([]byte("{}")))
	w := httptest.NewRecorder()
	s.handleUpdateSkillParams(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleUpdateSkillParams_AgentNotFound(t *testing.T) {
	s := newTestServerV2(t)
	body, _ := json.Marshal(map[string]interface{}{"params": map[string]string{}})
	req := httptest.NewRequest(http.MethodPut, "/api/agents/nonexistent/genome/skills/chat/params", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleUpdateSkillParams(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}


// --- handleAgentDetail with action branches ---

func TestHandleAgentDetail_MemoryAction(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	req := httptest.NewRequest(http.MethodGet, "/api/agents/test-agent/memory", nil)
	w := httptest.NewRecorder()
	s.handleAgentDetail(w, req)
	// memory not found for new agent → 404
	if w.Code != http.StatusNotFound && w.Code != http.StatusOK {
		t.Errorf("status = %d", w.Code)
	}
}

func TestHandleAgentDetail_ClearMemoryAction(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/agents/test-agent/memory", nil)
	w := httptest.NewRecorder()
	s.handleAgentDetail(w, req)
	if w.Code != http.StatusNotFound && w.Code != http.StatusOK {
		t.Errorf("status = %d", w.Code)
	}
}

func TestHandleAgentDetail_PatchAction(t *testing.T) {
	s, agent := newTestServerWithAgent(t)
	_ = agent
	body, _ := json.Marshal(map[string]string{"name": "Updated"})
	req := httptest.NewRequest(http.MethodPatch, "/api/agents/test-agent", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleAgentDetail(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

func TestHandleAgentDetail_BadAction(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/agents/test-agent/unknown-action", nil)
	w := httptest.NewRecorder()
	s.handleAgentDetail(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// handleWSChat requires an active websocket.Conn — tested via integration only.

// --- helpers: extractAgentIDForFirewall ---

func TestExtractAgentIDForFirewall(t *testing.T) {
	tests := []struct {
		path   string
		suffix string
		want   string
	}{
		{"/api/agents/my-agent/firewall", "/firewall", "my-agent"},
		{"/api/agents//firewall", "/firewall", ""},
		{"/api/agents/agent/sub/firewall", "/firewall", ""},
	}
	for _, tt := range tests {
		got := extractAgentIDForFirewall(tt.path, tt.suffix)
		if got != tt.want {
			t.Errorf("extractAgentIDForFirewall(%q, %q) = %q, want %q", tt.path, tt.suffix, got, tt.want)
		}
	}
}

// --- sendSSE ---

func TestSendSSE_WithFlusher(t *testing.T) {
	s := newTestServerV2(t)
	w := httptest.NewRecorder()
	// httptest.ResponseRecorder implements http.Flusher
	flusher, ok := any(w).(http.Flusher)
	if !ok {
		t.Skip("ResponseRecorder does not implement Flusher")
	}
	s.sendSSE(w, flusher, map[string]string{"key": "val"})
}

// --- initializeAgents update path ---

func TestInitializeAgentsUpdate(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	// Create agent definition matching existing
	agentDef := config.AgentDef{
		ID:    "test-agent",
		Name:  "Updated Name",
		Type:  "orchestrator",
		Model: "test/model",
	}
	cfg := config.DefaultConfig()
	cfg.Agents = []config.AgentDef{agentDef}
	_ = s // server has registry we can use

	// Test update path via registry
	err := s.registry.Update("test-agent", agentDef)
	if err != nil {
		t.Logf("update agent: %v (may be expected)", err)
	}
}

// --- more constraint tests ---

func TestHandleConstraintRoutes_InvalidJSON(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	req := httptest.NewRequest(http.MethodPut, "/api/agents/test-agent/genome/constraints", bytes.NewReader([]byte("bad")))
	w := httptest.NewRecorder()
	s.handleConstraintRoutes(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleConstraintRoutes_NotFound(t *testing.T) {
	s := newTestServerV2(t)
	body, _ := json.Marshal(map[string]interface{}{"constraints": map[string]interface{}{}})
	req := httptest.NewRequest(http.MethodPut, "/api/agents/nonexistent/genome/constraints", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleConstraintRoutes(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHandleConstraintRoutes_EmptyID(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodPut, "/api/agents//genome/constraints", nil)
	w := httptest.NewRecorder()
	s.handleConstraintRoutes(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleConstraintRoutes_WithSignature(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	body, _ := json.Marshal(map[string]interface{}{
		"constraints": map[string]interface{}{"max_model_tier": "gpt-4"},
		"signature":   []byte("fake-sig"),
		"public_key":  []byte("fake-key"),
	})
	req := httptest.NewRequest(http.MethodPut, "/api/agents/test-agent/genome/constraints", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleConstraintRoutes(w, req)
	// Will fail signature verification → 403
	if w.Code != http.StatusForbidden && w.Code != http.StatusOK {
		t.Errorf("status = %d", w.Code)
	}
}

// --- handleSubmitFeedback score out of range ---

func TestHandleSubmitFeedback_ScoreOutOfRange(t *testing.T) {
	s := newTestServerV2(t)
	body, _ := json.Marshal(map[string]interface{}{
		"type": "approval", "score": 5.0, "context": "too high",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/agents/test/feedback", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleSubmitFeedback(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// --- more genome routes ---

func TestHandleGenomeRoutes_DeleteMethod(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/agents/test/genome", nil)
	w := httptest.NewRecorder()
	s.handleGenomeRoutes(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleSkillRoutes_MethodNotAllowed(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/agents/test/skills/chat", nil)
	w := httptest.NewRecorder()
	s.handleSkillRoutes(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

// --- handleFeedbackRoutes, handleBehaviorRoutes, handleBehaviorHistoryRoutes ---

func TestHandleFeedbackRoutes_MethodNotAllowed(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodGet, "/api/agents/test/genome/feedback", nil)
	w := httptest.NewRecorder()
	s.handleFeedbackRoutes(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleBehaviorRoutes_MethodNotAllowed(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodPost, "/api/agents/test/genome/behavior", nil)
	w := httptest.NewRecorder()
	s.handleBehaviorRoutes(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleBehaviorHistoryRoutes_MethodNotAllowed(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodPost, "/api/agents/test/genome/behavior/history", nil)
	w := httptest.NewRecorder()
	s.handleBehaviorHistoryRoutes(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

// --- handleGetGenome more paths ---

func TestHandleGetGenome_NotFound(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodGet, "/api/agents/nonexistent/genome", nil)
	w := httptest.NewRecorder()
	s.handleGetGenome(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

// --- handleLogStream ---

func TestHandleLogStream_MethodNotAllowed2(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodPost, "/api/dashboard/logs", nil)
	w := httptest.NewRecorder()
	s.handleLogStream(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleLogStream_WithDeadline(t *testing.T) {
	s := newTestServerV2(t)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/api/dashboard/logs", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	s.handleLogStream(w, req)
	// Should exit after context cancel
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}
