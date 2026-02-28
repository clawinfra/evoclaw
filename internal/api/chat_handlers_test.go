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

	"github.com/clawinfra/evoclaw/internal/agents"
	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/models"
	"github.com/clawinfra/evoclaw/internal/orchestrator"
)

// newTestChatServer creates a server with a functioning orchestrator for chat tests
func newTestChatServer(t *testing.T) *Server {
	t.Helper()
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	cfg := &config.Config{
		Server: config.ServerConfig{Port: 8420, DataDir: tmpDir},
		Agents: []config.AgentDef{
			{
				ID:           "test-agent",
				Name:         "Test Agent",
				Model:        "test-provider/model-1",
				SystemPrompt: "You are a test agent",
				Type:         "monitor",
			},
		},
		Models: config.ModelsConfig{
			Routing: config.ModelRouting{
				Simple:   "test-provider/model-1",
				Complex:  "test-provider/model-1",
				Critical: "test-provider/model-1",
			},
		},
	}

	registry, _ := agents.NewRegistry(tmpDir, logger)
	memory, _ := agents.NewMemoryStore(tmpDir, logger)
	router := models.NewRouter(logger)

	// Create orchestrator with config and agents
	orch := orchestrator.New(cfg, logger)

	// Register mock provider
	provider := &mockProvider{
		name: "test-provider",
		models: []config.Model{
			{ID: "model-1", Name: "Test Model", ContextWindow: 4096},
		},
	}
	orch.RegisterProvider(provider)

	// Create a mock channel so Start works
	mockCh := &mockChanForChat{msgs: make(chan orchestrator.Message, 100)}
	orch.RegisterChannel(mockCh)

	// Start orchestrator
	if err := orch.Start(); err != nil {
		t.Fatalf("failed to start orchestrator: %v", err)
	}

	// Create agent in registry
	if _, err := registry.Create(config.AgentDef{
		ID:    "test-agent",
		Name:  "Test Agent",
		Type:  "monitor",
		Model: "test-provider/model-1",
	}); err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	s := NewServer(8420, orch, registry, memory, router, logger)
	t.Cleanup(func() {
		if err := orch.Stop(); err != nil {
			t.Logf("failed to stop orchestrator: %v", err)
		}
	})
	return s
}

// mockChanForChat implements orchestrator.Channel for tests
type mockChanForChat struct {
	msgs chan orchestrator.Message
}

func (m *mockChanForChat) Name() string                                              { return "mock" }
func (m *mockChanForChat) Start(ctx context.Context) error                           { return nil }
func (m *mockChanForChat) Stop() error                                               { close(m.msgs); return nil }
func (m *mockChanForChat) Send(ctx context.Context, msg orchestrator.Response) error  { return nil }
func (m *mockChanForChat) Receive() <-chan orchestrator.Message                      { return m.msgs }

func TestHandleChat_Success(t *testing.T) {
	s := newTestChatServer(t)

	body := ChatRequest{
		AgentID: "test-agent",
		Message: "Hello!",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleChat(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp ChatResponseJSON
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.AgentID != "test-agent" {
		t.Errorf("expected agent test-agent, got %s", resp.AgentID)
	}

	if resp.Response == "" {
		t.Error("expected non-empty response")
	}

	if resp.Timestamp == "" {
		t.Error("expected non-empty timestamp")
	}
}

func TestHandleChat_MethodNotAllowed(t *testing.T) {
	s := newTestChatServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/chat", nil)
	w := httptest.NewRecorder()

	s.handleChat(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleChat_EmptyMessage(t *testing.T) {
	s := newTestChatServer(t)

	body := ChatRequest{
		AgentID: "test-agent",
		Message: "",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleChat(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleChat_InvalidJSON(t *testing.T) {
	s := newTestChatServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader([]byte("invalid json")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleChat(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleChat_AgentNotFound(t *testing.T) {
	s := newTestChatServer(t)

	body := ChatRequest{
		AgentID: "nonexistent",
		Message: "Hello!",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleChat(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestHandleChat_DefaultAgent(t *testing.T) {
	s := newTestChatServer(t)

	body := ChatRequest{
		AgentID: "", // empty = use first agent
		Message: "Hello!",
	}
	jsonBody, _ := json.Marshal(body)

	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(jsonBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	s.handleChat(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleChatHistory_Success(t *testing.T) {
	s := newTestChatServer(t)

	// Add some chat history
	mem := s.memory.Get("test-agent")
	mem.Add("user", "Hello")
	mem.Add("assistant", "Hi there!")
	mem.Add("user", "How are you?")
	mem.Add("assistant", "I'm doing great!")

	req := httptest.NewRequest(http.MethodGet, "/api/chat/history?agent_id=test-agent&limit=10", nil)
	w := httptest.NewRecorder()

	s.handleChatHistory(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["agent_id"] != "test-agent" {
		t.Errorf("expected agent_id test-agent, got %v", resp["agent_id"])
	}

	if resp["message_count"].(float64) != 4 {
		t.Errorf("expected 4 messages, got %v", resp["message_count"])
	}
}

func TestHandleChatHistory_MethodNotAllowed(t *testing.T) {
	s := newTestChatServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/chat/history?agent_id=test-agent", nil)
	w := httptest.NewRecorder()

	s.handleChatHistory(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleChatHistory_MissingAgentID(t *testing.T) {
	s := newTestChatServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/chat/history", nil)
	w := httptest.NewRecorder()

	s.handleChatHistory(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleChatHistory_WithConversationID(t *testing.T) {
	s := newTestChatServer(t)

	// Add history to specific conversation
	mem := s.memory.Get("test-agent:conv-123")
	mem.Add("user", "Specific conversation message")
	mem.Add("assistant", "Specific response")

	req := httptest.NewRequest(http.MethodGet, "/api/chat/history?agent_id=test-agent&conversation_id=conv-123", nil)
	w := httptest.NewRecorder()

	s.handleChatHistory(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp["message_count"].(float64) != 2 {
		t.Errorf("expected 2 messages, got %v", resp["message_count"])
	}
}

func TestHandleChatStream_MethodNotAllowed(t *testing.T) {
	s := newTestChatServer(t)

	req := httptest.NewRequest(http.MethodPost, "/api/chat/stream?agent_id=test-agent&message=hi", nil)
	w := httptest.NewRecorder()

	s.handleChatStream(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestHandleChatStream_MissingParams(t *testing.T) {
	s := newTestChatServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/chat/stream", nil)
	w := httptest.NewRecorder()

	s.handleChatStream(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestHandleChatStream_AgentNotFound(t *testing.T) {
	s := newTestChatServer(t)

	req := httptest.NewRequest(http.MethodGet, "/api/chat/stream?agent_id=nonexistent&message=hi", nil)
	w := httptest.NewRecorder()

	s.handleChatStream(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}
