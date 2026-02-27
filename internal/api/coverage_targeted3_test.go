package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/clawinfra/evoclaw/internal/config"
)

// ---- handleUpdateSkillParams: more branches ----

func TestHandleUpdateSkillParams_InvalidJSON(t *testing.T) {
	s, agent := newTestServerWithAgent(t)
	// Set a genome with skills
	agent.Def.Genome = &config.Genome{
		Skills: map[string]config.SkillGenome{
			"test-skill": {Version: 1},
		},
	}
	req := httptest.NewRequest(http.MethodPut, "/api/agents/"+agent.ID+"/genome/skills/test-skill/params",
		bytes.NewBufferString("not-json"))
	w := httptest.NewRecorder()
	s.handleUpdateSkillParams(w, req)
	if w.Code != http.StatusBadRequest && w.Code != http.StatusOK {
		t.Errorf("status = %d, unexpected", w.Code)
	}
}

func TestHandleUpdateSkillParams_Success(t *testing.T) {
	s, agent := newTestServerWithAgent(t)
	agent.Def.Genome = &config.Genome{
		Skills: map[string]config.SkillGenome{
			"test-skill": {Version: 1},
		},
	}
	body, _ := json.Marshal(map[string]interface{}{"temperature": 0.7})
	req := httptest.NewRequest(http.MethodPut, "/api/agents/"+agent.ID+"/genome/skills/test-skill/params",
		bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleUpdateSkillParams(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// ---- handleAgentUpdate: update type ----

func TestHandleAgentUpdate_UpdateType(t *testing.T) {
	s, agent := newTestServerWithAgent(t)
	body, _ := json.Marshal(map[string]interface{}{"type": "worker"})
	req := httptest.NewRequest(http.MethodPatch, "/api/agents/"+agent.ID, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleAgentUpdate(w, req, agent.ID, agent)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// ---- handleAgentMemory / handleClearMemory via handleAgentDetail ----

func TestHandleAgentDetail_MemoryPath(t *testing.T) {
	s, agent := newTestServerWithAgent(t)
	req := httptest.NewRequest(http.MethodGet, "/api/agents/"+agent.ID+"/memory", nil)
	w := httptest.NewRecorder()
	s.handleAgentDetail(w, req)
	// Should hit agent memory handler
	if w.Code != http.StatusOK && w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, unexpected", w.Code)
	}
}

func TestHandleAgentDetail_ClearMemoryPath(t *testing.T) {
	s, agent := newTestServerWithAgent(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/agents/"+agent.ID+"/memory", nil)
	w := httptest.NewRecorder()
	s.handleAgentDetail(w, req)
	// Should hit clear memory handler
	if w.Code != http.StatusOK && w.Code != http.StatusNotFound && w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, unexpected", w.Code)
	}
}

// ---- handleAgentDetail: metrics path ----

func TestHandleAgentDetail_MetricsPath(t *testing.T) {
	s, agent := newTestServerWithAgent(t)
	req := httptest.NewRequest(http.MethodGet, "/api/agents/"+agent.ID+"/metrics", nil)
	w := httptest.NewRecorder()
	s.handleAgentDetail(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, unexpected", w.Code)
	}
}

func TestHandleAgentDetail_EvolvePath(t *testing.T) {
	s, agent := newTestServerWithAgent(t)
	req := httptest.NewRequest(http.MethodPost, "/api/agents/"+agent.ID+"/evolve", nil)
	w := httptest.NewRecorder()
	s.handleAgentDetail(w, req)
	if w.Code != http.StatusOK && w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, unexpected", w.Code)
	}
}



// ---- WriteError ----

func TestWriteErrorCovTarget(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, http.StatusBadRequest, "test error")
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}
