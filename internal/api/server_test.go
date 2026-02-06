package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/clawinfra/evoclaw/internal/agents"
	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/models"
	"github.com/clawinfra/evoclaw/internal/orchestrator"
)

func newTestServer(t *testing.T) *Server {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	registry, _ := agents.NewRegistry(tmpDir, logger)
	memory, _ := agents.NewMemoryStore(tmpDir, logger)
	router := models.NewRouter(logger)
	orch := &orchestrator.Orchestrator{} // Mock orchestrator

	return NewServer(8420, orch, registry, memory, router, logger)
}

func TestNewServer(t *testing.T) {
	s := newTestServer(t)

	if s == nil {
		t.Fatal("expected non-nil server")
	}

	if s.port != 8420 {
		t.Errorf("expected port 8420, got %d", s.port)
	}
}

func TestHandleStatus(t *testing.T) {
	s := newTestServer(t)

	// Create test agent
	def := config.AgentDef{
		ID:   "test-agent-1",
		Name: "Test Agent",
	}
	s.registry.Create(def)

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	w := httptest.NewRecorder()

	s.handleStatus(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["version"] != "0.1.0" {
		t.Errorf("expected version 0.1.0, got %v", response["version"])
	}

	if response["agents"] != float64(1) {
		t.Errorf("expected 1 agent, got %v", response["agents"])
	}
}

func TestHandleStatusMethodNotAllowed(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/status", nil)
	w := httptest.NewRecorder()

	s.handleStatus(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleAgentsGet(t *testing.T) {
	s := newTestServer(t)

	// Create test agents
	s.registry.Create(config.AgentDef{ID: "agent-1", Name: "Agent 1"})
	s.registry.Create(config.AgentDef{ID: "agent-2", Name: "Agent 2"})

	req := httptest.NewRequest(http.MethodGet, "/api/agents", nil)
	w := httptest.NewRecorder()

	s.handleAgents(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response []agents.Agent
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(response) != 2 {
		t.Errorf("expected 2 agents, got %d", len(response))
	}
}

func TestHandleModelsGet(t *testing.T) {
	s := newTestServer(t)

	// Register test provider
	provider := &mockProvider{
		name: "test-provider",
		models: []config.Model{
			{ID: "model-1", Name: "Test Model"},
		},
	}
	s.router.RegisterProvider(provider)

	req := httptest.NewRequest(http.MethodGet, "/api/models", nil)
	w := httptest.NewRecorder()

	s.handleModels(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Just check it's valid JSON
	var response interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// Response should be an array
	if _, ok := response.([]interface{}); !ok {
		t.Error("expected response to be array")
	}
}

func TestHandleModelsMethodNotAllowed(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/models", nil)
	w := httptest.NewRecorder()

	s.handleModels(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleCostsGet(t *testing.T) {
	s := newTestServer(t)

	// Register provider and make a request to track cost
	provider := &mockProvider{
		name: "test-provider",
		models: []config.Model{
			{ID: "model-1", CostInput: 1.0, CostOutput: 2.0},
		},
	}
	s.router.RegisterProvider(provider)

	// Make a chat request to track cost
	req := orchestrator.ChatRequest{
		Model:   "model-1",
		Messages: []orchestrator.ChatMessage{{Role: "user", Content: "Hi"}},
	}
	s.router.Chat(context.Background(), "test-provider/model-1", req, nil)

	// Test GET /api/costs
	httpReq := httptest.NewRequest(http.MethodGet, "/api/costs", nil)
	w := httptest.NewRecorder()

	s.handleCosts(w, httpReq)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var response map[string]*models.ModelCost
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(response) != 1 {
		t.Errorf("expected 1 cost entry, got %d", len(response))
	}

	cost := response["test-provider/model-1"]
	if cost.TotalRequests != 1 {
		t.Errorf("expected 1 request, got %d", cost.TotalRequests)
	}
}

func TestHandleCostsMethodNotAllowed(t *testing.T) {
	s := newTestServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/costs", nil)
	w := httptest.NewRecorder()

	s.handleCosts(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestCORSMiddleware(t *testing.T) {
	s := newTestServer(t)

	handler := s.corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/status", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	// Check CORS headers
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("expected CORS origin header to be set")
	}

	if w.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("expected CORS methods header to be set")
	}
}

func TestCORSOptions(t *testing.T) {
	s := newTestServer(t)

	handler := s.corsMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodOptions, "/api/status", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 for OPTIONS, got %d", w.Code)
	}
}

func TestRespondJSON(t *testing.T) {
	s := newTestServer(t)

	w := httptest.NewRecorder()
	data := map[string]string{
		"message": "test",
		"status":  "ok",
	}

	s.respondJSON(w, data)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if w.Header().Get("Content-Type") != "application/json" {
		t.Error("expected Content-Type application/json")
	}

	var response map[string]string
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if response["message"] != "test" {
		t.Errorf("expected message 'test', got '%s'", response["message"])
	}
}

func TestHandleAgentDetail(t *testing.T) {
	s := newTestServer(t)
	
	// Create test agent
	def := config.AgentDef{ID: "test-agent", Name: "Test Agent"}
	s.registry.Create(def)
	
	// Test GET /api/agents/{id}
	req := httptest.NewRequest(http.MethodGet, "/api/agents/test-agent", nil)
	w := httptest.NewRecorder()
	
	s.handleAgentDetail(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	
	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	
	if response["id"] != "test-agent" {
		t.Errorf("expected agent id test-agent, got %v", response["id"])
	}
}

func TestHandleAgentDetailNotFound(t *testing.T) {
	s := newTestServer(t)
	
	req := httptest.NewRequest(http.MethodGet, "/api/agents/nonexistent", nil)
	w := httptest.NewRecorder()
	
	s.handleAgentDetail(w, req)
	
	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestHandleAgentDetailInvalidPath(t *testing.T) {
	s := newTestServer(t)
	
	// Empty agent ID will cause a "not found" error when looking up the agent
	req := httptest.NewRequest(http.MethodGet, "/api/agents/", nil)
	w := httptest.NewRecorder()
	
	s.handleAgentDetail(w, req)
	
	// Empty string as agentID will result in agent not found (404)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestHandleAgentMetrics(t *testing.T) {
	s := newTestServer(t)
	
	// Create test agent
	def := config.AgentDef{ID: "test-agent", Name: "Test Agent"}
	s.registry.Create(def)
	
	// Test GET /api/agents/{id}/metrics
	req := httptest.NewRequest(http.MethodGet, "/api/agents/test-agent/metrics", nil)
	w := httptest.NewRecorder()
	
	s.handleAgentDetail(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	
	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	
	if response["agent_id"] != "test-agent" {
		t.Errorf("expected agent_id test-agent, got %v", response["agent_id"])
	}
	
	if _, ok := response["metrics"]; !ok {
		t.Error("expected metrics field in response")
	}
}

func TestHandleAgentEvolve(t *testing.T) {
	s := newTestServer(t)
	
	// Create test agent
	def := config.AgentDef{ID: "test-agent", Name: "Test Agent"}
	s.registry.Create(def)
	
	// Test POST /api/agents/{id}/evolve
	req := httptest.NewRequest(http.MethodPost, "/api/agents/test-agent/evolve", nil)
	w := httptest.NewRecorder()
	
	s.handleAgentDetail(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	
	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	
	if response["message"] != "evolution triggered" {
		t.Errorf("expected evolution triggered message, got %v", response["message"])
	}
}

func TestHandleAgentMemory(t *testing.T) {
	s := newTestServer(t)
	
	// Create test agent
	def := config.AgentDef{ID: "test-agent", Name: "Test Agent"}
	s.registry.Create(def)
	
	// Add some memory
	mem := s.memory.Get("test-agent")
	mem.Add("user", "Hello")
	mem.Add("assistant", "Hi there!")
	
	// Test GET /api/agents/{id}/memory
	req := httptest.NewRequest(http.MethodGet, "/api/agents/test-agent/memory", nil)
	w := httptest.NewRecorder()
	
	s.handleAgentDetail(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	
	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	
	if response["agent_id"] != "test-agent" {
		t.Errorf("expected agent_id test-agent, got %v", response["agent_id"])
	}
	
	messageCount := response["message_count"].(float64)
	if messageCount != 2 {
		t.Errorf("expected 2 messages, got %v", messageCount)
	}
}

func TestHandleClearMemory(t *testing.T) {
	s := newTestServer(t)
	
	// Create test agent
	def := config.AgentDef{ID: "test-agent", Name: "Test Agent"}
	s.registry.Create(def)
	
	// Add some memory
	mem := s.memory.Get("test-agent")
	mem.Add("user", "Hello")
	mem.Add("assistant", "Hi there!")
	
	// Test DELETE /api/agents/{id}/memory
	req := httptest.NewRequest(http.MethodDelete, "/api/agents/test-agent/memory", nil)
	w := httptest.NewRecorder()
	
	s.handleAgentDetail(w, req)
	
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	
	var response map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	
	if response["message"] != "memory cleared" {
		t.Errorf("expected memory cleared message, got %v", response["message"])
	}
	
	// Verify memory was cleared
	messages := mem.GetMessages()
	if len(messages) != 0 {
		t.Errorf("expected 0 messages after clear, got %d", len(messages))
	}
}

func TestHandleAgentMemoryNotFound(t *testing.T) {
	s := newTestServer(t)
	
	// Test GET memory for nonexistent agent (but with valid ID format)
	// Note: memory.Get creates a new memory if it doesn't exist,
	// so we can't actually test "not found" easily. Test the handler path instead.
	
	req := httptest.NewRequest(http.MethodGet, "/api/agents/nonexistent/memory", nil)
	w := httptest.NewRecorder()
	
	s.handleAgentDetail(w, req)
	
	// Should return 404 because agent doesn't exist in registry
	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestRespondJSONError(t *testing.T) {
	s := newTestServer(t)
	
	w := httptest.NewRecorder()
	// Create a value that can't be marshaled to JSON
	data := make(chan int)
	
	s.respondJSON(w, data)
	
	// Should return 500 for marshal errors
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected status 500, got %d", w.Code)
	}
}

func TestLoggingMiddleware(t *testing.T) {
	s := newTestServer(t)
	
	called := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("test response"))
	})
	
	wrapped := s.loggingMiddleware(handler)
	
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	w := httptest.NewRecorder()
	
	wrapped.ServeHTTP(w, req)
	
	if !called {
		t.Error("handler was not called")
	}
	
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

// mockProvider implements ModelProvider for testing
type mockProvider struct {
	name    string
	models  []config.Model
	chatErr error
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) Models() []config.Model {
	return m.models
}

func (m *mockProvider) Chat(ctx context.Context, req orchestrator.ChatRequest) (*orchestrator.ChatResponse, error) {
	if m.chatErr != nil {
		return nil, m.chatErr
	}
	return &orchestrator.ChatResponse{
		Content:      "Mock response",
		Model:        req.Model,
		TokensInput:  100,
		TokensOutput: 50,
	}, nil
}
