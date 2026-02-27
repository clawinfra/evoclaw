package api

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/orchestrator"
)

// ---- handleAgentUpdate: update model (with valid model) ----

func TestHandleAgentUpdate_UpdateModel(t *testing.T) {
	s, agent := newTestServerWithAgent(t)
	// First register a model so we can update to it
	s.router.RegisterProvider(&mockModelProvider{})

	body, _ := json.Marshal(map[string]interface{}{"model": "test-model"})
	req := httptest.NewRequest(http.MethodPatch, "/api/agents/"+agent.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleAgentUpdate(w, req, agent.ID, agent)
	if w.Code != http.StatusOK && w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 200 or 400", w.Code)
	}
}

func TestHandleAgentUpdate_UpdateAllFields(t *testing.T) {
	s, agent := newTestServerWithAgent(t)
	body, _ := json.Marshal(map[string]interface{}{
		"name":  "Updated Agent",
		"type":  "worker",
		"model": "", // empty model should be skipped
	})
	req := httptest.NewRequest(http.MethodPatch, "/api/agents/"+agent.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleAgentUpdate(w, req, agent.ID, agent)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// ---- handleGenomeRoutes: PATCH not allowed ----

func TestHandleGenomeRoutes_PATCH(t *testing.T) {
	s, agent := newTestServerWithAgent(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/agents/"+agent.ID+"/genome", nil)
	w := httptest.NewRecorder()
	s.handleGenomeRoutes(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleGenomeRoutes_POST(t *testing.T) {
	s, agent := newTestServerWithAgent(t)
	req := httptest.NewRequest(http.MethodPost, "/api/agents/"+agent.ID+"/genome", nil)
	w := httptest.NewRecorder()
	s.handleGenomeRoutes(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

// ---- handleSchedulerJobRoutes: unsupported method ----

func TestHandleSchedulerJobRoutes_PUT(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodPut, "/api/scheduler/jobs/some-job-id", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerJobRoutes(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleSchedulerJobRoutes_HEAD(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodHead, "/api/scheduler/jobs/some-job-id", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerJobRoutes(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

// ---- handleAgentEvolution: POST triggers evolution ----

func TestHandleAgentEvolution_Trigger(t *testing.T) {
	s, agent := newTestServerWithAgent(t)
	req := httptest.NewRequest(http.MethodPost, "/api/agents/"+agent.ID+"/evolution", nil)
	w := httptest.NewRecorder()
	s.handleAgentEvolution(w, req)
	// Returns 200 with message
	if w.Code != http.StatusOK && w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, unexpected", w.Code)
	}
}

func TestHandleAgentEvolution_GET(t *testing.T) {
	s, agent := newTestServerWithAgent(t)
	req := httptest.NewRequest(http.MethodGet, "/api/agents/"+agent.ID+"/evolution", nil)
	w := httptest.NewRecorder()
	s.handleAgentEvolution(w, req)
	// Dashboard may treat GET differently
	if w.Code != http.StatusOK && w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, unexpected", w.Code)
	}
}

// ---- handleAgentDetail: handleMetrics ----

func TestHandleAgentMetrics_Handler(t *testing.T) {
	s, agent := newTestServerWithAgent(t)
	w := httptest.NewRecorder()
	s.handleAgentMetrics(w, agent)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestHandleAgentEvolve_Handler(t *testing.T) {
	s, agent := newTestServerWithAgent(t)
	w := httptest.NewRecorder()
	s.handleAgentEvolve(w, agent)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// ---- writeJSON: more paths ----

func TestWriteJSON_EmptyObject(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]interface{}{})
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestWriteJSON_EmptySlice(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, []string{})
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// Mock model provider for testing
type mockModelProvider struct{}

func (m *mockModelProvider) Name() string {
	return "mock"
}

func (m *mockModelProvider) Models() []config.Model {
	return []config.Model{
		{ID: "test-model", Name: "Test Model"},
	}
}

func (m *mockModelProvider) Chat(ctx context.Context, req orchestrator.ChatRequest) (*orchestrator.ChatResponse, error) {
	return &orchestrator.ChatResponse{
		Content:      "test response",
		TokensInput:  10,
		TokensOutput: 5,
	}, nil
}
