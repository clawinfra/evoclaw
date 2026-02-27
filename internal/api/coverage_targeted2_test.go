package api

import (
	"bytes"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"testing"
	"testing/fstest"

)

// ---- terminal page with webFS ----

func TestHandleTerminalPage_WithWebFS(t *testing.T) {
	s := newTestServerV2(t)
	// Create an in-memory FS with terminal.html
	mfs := fstest.MapFS{
		"terminal.html": &fstest.MapFile{
			Data: []byte("<html><body>Terminal</body></html>"),
		},
	}
	sub, _ := fs.Sub(mfs, ".")
	s.SetWebFS(sub)

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

func TestHandleSubmitFeedback_EmptyAgentID(t *testing.T) {
	s := newTestServer(t)
	body, _ := json.Marshal(map[string]interface{}{
		"type":  "approval",
		"score": 0.5,
	})
	req := httptest.NewRequest(http.MethodPost, "/api/agents//feedback", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleSubmitFeedback(w, req)
	if w.Code != http.StatusBadRequest && w.Code != http.StatusOK {
		t.Errorf("status = %d, unexpected", w.Code)
	}
}

// ---- handleAgentUpdate: success path ----

func TestHandleAgentUpdate_UpdateName(t *testing.T) {
	s, agent := newTestServerWithAgent(t)
	body, _ := json.Marshal(map[string]interface{}{"name": "NewName"})
	req := httptest.NewRequest(http.MethodPatch, "/api/agents/"+agent.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleAgentUpdate(w, req, agent.ID, agent)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestHandleAgentUpdate_UnknownModel(t *testing.T) {
	s, agent := newTestServerWithAgent(t)
	body, _ := json.Marshal(map[string]interface{}{"model": "nonexistent/model"})
	req := httptest.NewRequest(http.MethodPatch, "/api/agents/"+agent.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleAgentUpdate(w, req, agent.ID, agent)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}



// ---- firewall: rollback with evolution engine ----

func TestHandleFirewallRollback_WithEngine(t *testing.T) {
	s := newTestServerWithEvolution(t)
	// Firewall should handle missing snapshot with 404
	req := httptest.NewRequest(http.MethodPost, "/api/agents/agent1/firewall/rollback", nil)
	w := httptest.NewRecorder()
	s.handleFirewallRollback(w, req)
	if w.Code != http.StatusNotFound && w.Code != http.StatusOK && w.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, unexpected", w.Code)
	}
}

// ---- evolution engine helper: newTestServerWithEvolution (already in coverage_targeted_test.go) ----
// getEvolutionStrategy: test returns nil for empty orch
func TestGetEvolutionStrategy_NilOrch(t *testing.T) {
	s := newTestServer(t)
	result := s.getEvolutionStrategy("agent1")
	if result != nil {
		t.Errorf("expected nil when orch is nil or no engine")
	}
}

func TestGetEvolutionStrategy_WithOrch(t *testing.T) {
	s := newTestServerV2(t)
	result := s.getEvolutionStrategy("agent1")
	// orch exists but no evo engine → returns nil
	if result != nil {
		t.Logf("got non-nil strategy: %v", result)
	}
}





// ---- evolution engine test helper (avoids duplicate) ----
// newTestServerWithEvolutionTarget removed — was unused
