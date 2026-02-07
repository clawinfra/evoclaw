package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleAgentRegister_Success(t *testing.T) {
	s := newTestServer(t)

	body := `{"id":"raspberrypi-a3f2","type":"monitor","host":"192.168.99.25"}`
	req := httptest.NewRequest(http.MethodPost, "/api/agents/register", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleAgentRegister(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp AgentRegisterResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Status != "registered" {
		t.Errorf("expected status 'registered', got '%s'", resp.Status)
	}
	if resp.ID != "raspberrypi-a3f2" {
		t.Errorf("expected id 'raspberrypi-a3f2', got '%s'", resp.ID)
	}
	if resp.MQTTPort != 1883 {
		t.Errorf("expected mqtt_port 1883, got %d", resp.MQTTPort)
	}

	// Verify agent was actually created in registry
	agent, err := s.registry.Get("raspberrypi-a3f2")
	if err != nil {
		t.Fatalf("agent should exist in registry: %v", err)
	}
	if agent.Def.Type != "monitor" {
		t.Errorf("expected agent type 'monitor', got '%s'", agent.Def.Type)
	}
	if agent.Def.Config["host"] != "192.168.99.25" {
		t.Errorf("expected host '192.168.99.25', got '%s'", agent.Def.Config["host"])
	}
}

func TestHandleAgentRegister_DefaultType(t *testing.T) {
	s := newTestServer(t)

	body := `{"id":"sensor-1","host":"192.168.1.10"}`
	req := httptest.NewRequest(http.MethodPost, "/api/agents/register", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	s.handleAgentRegister(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("expected status 201, got %d", w.Code)
	}

	agent, err := s.registry.Get("sensor-1")
	if err != nil {
		t.Fatalf("agent should exist: %v", err)
	}
	if agent.Def.Type != "monitor" {
		t.Errorf("expected default type 'monitor', got '%s'", agent.Def.Type)
	}
}

func TestHandleAgentRegister_MissingID(t *testing.T) {
	s := newTestServer(t)

	body := `{"type":"monitor","host":"192.168.99.25"}`
	req := httptest.NewRequest(http.MethodPost, "/api/agents/register", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	s.handleAgentRegister(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleAgentRegister_InvalidJSON(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/agents/register", bytes.NewBufferString("not json"))
	w := httptest.NewRecorder()

	s.handleAgentRegister(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleAgentRegister_MethodNotAllowed(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/agents/register", nil)
	w := httptest.NewRecorder()

	s.handleAgentRegister(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleAgentRegister_UpdateExisting(t *testing.T) {
	s := newTestServer(t)

	// Register first time
	body := `{"id":"pi-1234","type":"monitor","host":"192.168.1.10"}`
	req := httptest.NewRequest(http.MethodPost, "/api/agents/register", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	s.handleAgentRegister(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("first register failed: %d", w.Code)
	}

	// Register again with updated type
	body = `{"id":"pi-1234","type":"trader","host":"192.168.1.10"}`
	req = httptest.NewRequest(http.MethodPost, "/api/agents/register", bytes.NewBufferString(body))
	w = httptest.NewRecorder()
	s.handleAgentRegister(w, req)

	if w.Code != http.StatusCreated {
		t.Errorf("second register failed: %d", w.Code)
	}

	// Verify update
	agent, _ := s.registry.Get("pi-1234")
	if agent.Def.Type != "trader" {
		t.Errorf("expected updated type 'trader', got '%s'", agent.Def.Type)
	}
}

func TestHandleAgentRegister_ResponseContainsMQTTInfo(t *testing.T) {
	s := newTestServer(t)

	body := `{"id":"test-agent","type":"sensor","host":"10.0.0.5"}`
	req := httptest.NewRequest(http.MethodPost, "/api/agents/register", bytes.NewBufferString(body))
	w := httptest.NewRecorder()

	s.handleAgentRegister(w, req)

	var resp AgentRegisterResponse
	json.NewDecoder(w.Body).Decode(&resp)

	// Default MQTT info when no orchestrator config
	if resp.MQTTBroker == "" {
		t.Error("expected non-empty mqtt_broker")
	}
	if resp.MQTTPort == 0 {
		t.Error("expected non-zero mqtt_port")
	}
}
