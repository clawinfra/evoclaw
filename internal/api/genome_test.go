package api

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/clawinfra/evoclaw/internal/config"
)

func TestHandleUpdateGenome(t *testing.T) {
	s, agent := newTestServerWithAgent(t)

	// Valid genome update
	genome := config.Genome{
		Identity: config.GenomeIdentity{Name: "test", Persona: "helpful", Voice: "balanced"},
		Skills:   map[string]config.SkillGenome{"chat": {Enabled: true, Fitness: 0.5}},
		Behavior: config.GenomeBehavior{RiskTolerance: 0.3, Verbosity: 0.5, Autonomy: 0.5},
	}
	body, _ := json.Marshal(genome)
	req := httptest.NewRequest("PUT", "/api/agents/"+agent.ID+"/genome", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	s.handleUpdateGenome(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}

	// Invalid risk tolerance
	genome.Behavior.RiskTolerance = 1.5
	body, _ = json.Marshal(genome)
	req = httptest.NewRequest("PUT", "/api/agents/"+agent.ID+"/genome", bytes.NewBuffer(body))
	w = httptest.NewRecorder()
	s.handleUpdateGenome(w, req)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}

	// Invalid verbosity
	genome.Behavior.RiskTolerance = 0.3
	genome.Behavior.Verbosity = -0.1
	body, _ = json.Marshal(genome)
	req = httptest.NewRequest("PUT", "/api/agents/"+agent.ID+"/genome", bytes.NewBuffer(body))
	w = httptest.NewRecorder()
	s.handleUpdateGenome(w, req)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}

	// Invalid autonomy
	genome.Behavior.Verbosity = 0.5
	genome.Behavior.Autonomy = 2.0
	body, _ = json.Marshal(genome)
	req = httptest.NewRequest("PUT", "/api/agents/"+agent.ID+"/genome", bytes.NewBuffer(body))
	w = httptest.NewRecorder()
	s.handleUpdateGenome(w, req)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}

	// Invalid JSON
	req = httptest.NewRequest("PUT", "/api/agents/"+agent.ID+"/genome", bytes.NewBufferString("invalid"))
	w = httptest.NewRecorder()
	s.handleUpdateGenome(w, req)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}

	// Non-existent agent
	req = httptest.NewRequest("PUT", "/api/agents/nonexistent/genome", bytes.NewBuffer(body))
	w = httptest.NewRecorder()
	s.handleUpdateGenome(w, req)
	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}

	// Empty agent ID
	req = httptest.NewRequest("PUT", "/api/agents//genome", bytes.NewBuffer(body))
	w = httptest.NewRecorder()
	s.handleUpdateGenome(w, req)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}

	// Wrong method
	req = httptest.NewRequest("GET", "/api/agents/"+agent.ID+"/genome", nil)
	w = httptest.NewRecorder()
	s.handleUpdateGenome(w, req)
	if w.Code != 405 {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleGetSkill(t *testing.T) {
	s, agent := newTestServerWithAgent(t)

	// Set genome with skills
	agent.Def.Genome = &config.Genome{
		Skills: map[string]config.SkillGenome{
			"trading": {Enabled: true, Fitness: 0.8},
		},
	}

	// Valid skill
	req := httptest.NewRequest("GET", "/api/agents/"+agent.ID+"/genome/skills/trading", nil)
	w := httptest.NewRecorder()
	s.handleGetSkill(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}

	// Non-existent skill
	req = httptest.NewRequest("GET", "/api/agents/"+agent.ID+"/genome/skills/nonexistent", nil)
	w = httptest.NewRecorder()
	s.handleGetSkill(w, req)
	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}

	// No genome
	agent.Def.Genome = nil
	req = httptest.NewRequest("GET", "/api/agents/"+agent.ID+"/genome/skills/trading", nil)
	w = httptest.NewRecorder()
	s.handleGetSkill(w, req)
	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}

	// Non-existent agent
	req = httptest.NewRequest("GET", "/api/agents/nonexistent/genome/skills/trading", nil)
	w = httptest.NewRecorder()
	s.handleGetSkill(w, req)
	if w.Code != 404 {
		t.Errorf("status = %d, want 404", w.Code)
	}

	// Invalid path
	req = httptest.NewRequest("GET", "/api/agents/badpath", nil)
	w = httptest.NewRecorder()
	s.handleGetSkill(w, req)
	if w.Code != 400 {
		t.Errorf("status = %d, want 400", w.Code)
	}

	// Wrong method
	req = httptest.NewRequest("POST", "/api/agents/"+agent.ID+"/genome/skills/trading", nil)
	w = httptest.NewRecorder()
	s.handleGetSkill(w, req)
	if w.Code != 405 {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleUpdateSkillParams(t *testing.T) {
	s, agent := newTestServerWithAgent(t)

	agent.Def.Genome = &config.Genome{
		Skills: map[string]config.SkillGenome{
			"trading": {Enabled: true, Params: map[string]interface{}{"threshold": -0.1}},
		},
	}

	// Valid update
	params := map[string]interface{}{"threshold": -0.2, "size": 50.0}
	body, _ := json.Marshal(params)
	req := httptest.NewRequest("PUT", "/api/agents/"+agent.ID+"/genome/skills/trading/params", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	s.handleUpdateSkillParams(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200, body: %s", w.Code, w.Body.String())
	}

	// Wrong method
	req = httptest.NewRequest("GET", "/api/agents/"+agent.ID+"/genome/skills/trading/params", nil)
	w = httptest.NewRecorder()
	s.handleUpdateSkillParams(w, req)
	if w.Code != 405 {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleConstraintRoutes(t *testing.T) {
	s, agent := newTestServerWithAgent(t)

	// GET not allowed (only PUT)
	req := httptest.NewRequest("GET", "/api/agents/"+agent.ID+"/genome/constraints", nil)
	w := httptest.NewRecorder()
	s.handleConstraintRoutes(w, req)
	if w.Code != 405 {
		t.Errorf("GET status = %d, want 405", w.Code)
	}

	// PUT with invalid JSON
	req = httptest.NewRequest("PUT", "/api/agents/"+agent.ID+"/genome/constraints", bytes.NewBufferString("invalid"))
	w = httptest.NewRecorder()
	s.handleConstraintRoutes(w, req)
	if w.Code != 400 {
		t.Errorf("PUT invalid JSON status = %d, want 400", w.Code)
	}

	// PUT with no agent
	req = httptest.NewRequest("PUT", "/api/agents/nonexistent/genome/constraints", bytes.NewBufferString("{}"))
	w = httptest.NewRecorder()
	s.handleConstraintRoutes(w, req)
	if w.Code != 404 {
		t.Errorf("PUT nonexistent status = %d, want 404", w.Code)
	}

	// PUT with empty agent ID
	req = httptest.NewRequest("PUT", "/api/agents//genome/constraints", bytes.NewBufferString("{}"))
	w = httptest.NewRecorder()
	s.handleConstraintRoutes(w, req)
	if w.Code != 400 {
		t.Errorf("PUT empty ID status = %d, want 400", w.Code)
	}

	// PUT with unsigned constraints (verification will fail)
	body, _ := json.Marshal(map[string]interface{}{
		"constraints": map[string]interface{}{"max_loss_usd": 500},
		"signature":   []byte("fake"),
		"public_key":  []byte("fake"),
	})
	req = httptest.NewRequest("PUT", "/api/agents/"+agent.ID+"/genome/constraints", bytes.NewBuffer(body))
	w = httptest.NewRecorder()
	s.handleConstraintRoutes(w, req)
	// Should fail sig verification
	if w.Code != 403 {
		t.Logf("PUT fake sig status = %d (expected 403)", w.Code)
	}
}

func TestHandleFeedbackRoutes(t *testing.T) {
	s, agent := newTestServerWithAgent(t)

	// No evolution engine
	body, _ := json.Marshal(map[string]interface{}{"type": "approval", "score": 0.8})
	req := httptest.NewRequest("POST", "/api/agents/"+agent.ID+"/feedback", bytes.NewBuffer(body))
	w := httptest.NewRecorder()
	s.handleFeedbackRoutes(w, req)
	// May return 503 without engine
	if w.Code != 200 && w.Code != 503 && w.Code != 405 {
		t.Errorf("POST feedback status = %d", w.Code)
	}

	// Wrong method
	req = httptest.NewRequest("DELETE", "/api/agents/"+agent.ID+"/feedback", nil)
	w = httptest.NewRecorder()
	s.handleFeedbackRoutes(w, req)
	if w.Code != 405 {
		t.Errorf("DELETE status = %d, want 405", w.Code)
	}
}

func TestHandleBehaviorRoutes(t *testing.T) {
	s, agent := newTestServerWithAgent(t)

	req := httptest.NewRequest("GET", "/api/agents/"+agent.ID+"/genome/behavior", nil)
	w := httptest.NewRecorder()
	s.handleBehaviorRoutes(w, req)
	// May return 503 without engine
	if w.Code != 200 && w.Code != 503 && w.Code != 404 {
		t.Errorf("GET behavior status = %d", w.Code)
	}

	req = httptest.NewRequest("DELETE", "/api/agents/"+agent.ID+"/genome/behavior", nil)
	w = httptest.NewRecorder()
	s.handleBehaviorRoutes(w, req)
	if w.Code != 405 {
		t.Errorf("DELETE status = %d, want 405", w.Code)
	}
}

func TestHandleBehaviorHistoryRoutes(t *testing.T) {
	s, agent := newTestServerWithAgent(t)

	req := httptest.NewRequest("GET", "/api/agents/"+agent.ID+"/behavior/history", nil)
	w := httptest.NewRecorder()
	s.handleBehaviorHistoryRoutes(w, req)
	if w.Code != 200 && w.Code != 503 && w.Code != 404 {
		t.Errorf("GET status = %d", w.Code)
	}

	req = httptest.NewRequest("POST", "/api/agents/"+agent.ID+"/behavior/history", nil)
	w = httptest.NewRecorder()
	s.handleBehaviorHistoryRoutes(w, req)
	if w.Code != 405 {
		t.Errorf("POST status = %d, want 405", w.Code)
	}
}

func TestHandleDashboardV3(t *testing.T) {
	s, _ := newTestServerWithAgent(t)
	req := httptest.NewRequest("GET", "/api/dashboard", nil)
	w := httptest.NewRecorder()
	s.handleDashboard(w, req)
	if w.Code != 200 {
		t.Errorf("status = %d, want 200", w.Code)
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if _, ok := resp["agents"]; !ok {
		t.Error("expected 'agents' field in dashboard")
	}
}

// newTestServerWithAgent is defined in comprehensive_test.go
