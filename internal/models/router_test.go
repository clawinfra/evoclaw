package models

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"

	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/orchestrator"
)

// mockProvider implements ModelProvider for testing
type mockProvider struct {
	name        string
	models      []config.Model
	chatFunc    func(ctx context.Context, req orchestrator.ChatRequest) (*orchestrator.ChatResponse, error)
	shouldFail  bool
	failureMode string
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) Models() []config.Model {
	return m.models
}

func (m *mockProvider) Chat(ctx context.Context, req orchestrator.ChatRequest) (*orchestrator.ChatResponse, error) {
	if m.shouldFail {
		return nil, errors.New("mock provider error: " + m.failureMode)
	}
	if m.chatFunc != nil {
		return m.chatFunc(ctx, req)
	}
	return &orchestrator.ChatResponse{
		Content:      "Mock response",
		Model:        req.Model,
		TokensInput:  100,
		TokensOutput: 50,
	}, nil
}

func newTestRouter() *Router {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	return NewRouter(logger)
}

func TestNewRouter(t *testing.T) {
	r := newTestRouter()

	if r == nil {
		t.Fatal("expected non-nil router")
	}

	if r.providers == nil {
		t.Error("expected providers map to be initialized")
	}

	if r.models == nil {
		t.Error("expected models map to be initialized")
	}

	if r.costs == nil {
		t.Error("expected costs tracker to be initialized")
	}
}

func TestRegisterProvider(t *testing.T) {
	r := newTestRouter()

	provider := &mockProvider{
		name: "test-provider",
		models: []config.Model{
			{
				ID:            "model-1",
				Name:          "Test Model 1",
				ContextWindow: 100000,
				CostInput:     1.0,
				CostOutput:    2.0,
			},
			{
				ID:            "model-2",
				Name:          "Test Model 2",
				ContextWindow: 200000,
				CostInput:     3.0,
				CostOutput:    6.0,
			},
		},
	}

	r.RegisterProvider(provider)

	// Check provider was registered
	if len(r.providers) != 1 {
		t.Errorf("expected 1 provider, got %d", len(r.providers))
	}

	// Check models were indexed
	if len(r.models) != 2 {
		t.Errorf("expected 2 models, got %d", len(r.models))
	}

	// Check model IDs are in correct format
	if _, ok := r.models["test-provider/model-1"]; !ok {
		t.Error("expected model test-provider/model-1 to be registered")
	}

	if _, ok := r.models["test-provider/model-2"]; !ok {
		t.Error("expected model test-provider/model-2 to be registered")
	}
}

func TestChat(t *testing.T) {
	r := newTestRouter()

	provider := &mockProvider{
		name: "test-provider",
		models: []config.Model{
			{
				ID:            "model-1",
				Name:          "Test Model",
				ContextWindow: 100000,
				CostInput:     1.0,
				CostOutput:    2.0,
			},
		},
	}

	r.RegisterProvider(provider)

	req := orchestrator.ChatRequest{
		Model:        "model-1",
		SystemPrompt: "Test prompt",
		Messages: []orchestrator.ChatMessage{
			{Role: "user", Content: "Hello"},
		},
		MaxTokens:   1000,
		Temperature: 0.7,
	}

	resp, err := r.Chat(context.Background(), "test-provider/model-1", req, nil)
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}

	if resp.Content != "Mock response" {
		t.Errorf("expected 'Mock response', got '%s'", resp.Content)
	}

	// Check cost was tracked
	cost := r.GetCost("test-provider/model-1")
	if cost.TotalRequests != 1 {
		t.Errorf("expected 1 request, got %d", cost.TotalRequests)
	}

	if cost.TotalTokensIn != 100 {
		t.Errorf("expected 100 input tokens, got %d", cost.TotalTokensIn)
	}

	if cost.TotalTokensOut != 50 {
		t.Errorf("expected 50 output tokens, got %d", cost.TotalTokensOut)
	}

	// Calculate expected cost
	expectedCost := (100 * 1.0 / 1_000_000) + (50 * 2.0 / 1_000_000)
	if cost.TotalCostUSD != expectedCost {
		t.Errorf("expected cost %f, got %f", expectedCost, cost.TotalCostUSD)
	}
}

func TestChatWithFallback(t *testing.T) {
	r := newTestRouter()

	// Primary provider that fails
	primaryProvider := &mockProvider{
		name:        "primary",
		shouldFail:  true,
		failureMode: "primary down",
		models: []config.Model{
			{ID: "model-1", CostInput: 1.0, CostOutput: 2.0},
		},
	}

	// Fallback provider that succeeds
	fallbackProvider := &mockProvider{
		name: "fallback",
		models: []config.Model{
			{ID: "model-2", CostInput: 0.5, CostOutput: 1.0},
		},
		chatFunc: func(ctx context.Context, req orchestrator.ChatRequest) (*orchestrator.ChatResponse, error) {
			return &orchestrator.ChatResponse{
				Content:      "Fallback response",
				Model:        req.Model,
				TokensInput:  100,
				TokensOutput: 50,
			}, nil
		},
	}

	r.RegisterProvider(primaryProvider)
	r.RegisterProvider(fallbackProvider)

	req := orchestrator.ChatRequest{
		Model:   "model-1",
		Messages: []orchestrator.ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	resp, err := r.Chat(context.Background(), "primary/model-1", req, []string{"fallback/model-2"})
	if err != nil {
		t.Fatalf("chat with fallback failed: %v", err)
	}

	if resp.Content != "Fallback response" {
		t.Errorf("expected fallback response, got '%s'", resp.Content)
	}

	// Check that fallback costs were tracked
	fallbackCost := r.GetCost("fallback/model-2")
	if fallbackCost.TotalRequests != 1 {
		t.Errorf("expected 1 fallback request, got %d", fallbackCost.TotalRequests)
	}
}

func TestChatAllFallbacksFail(t *testing.T) {
	r := newTestRouter()

	// All providers fail
	primaryProvider := &mockProvider{
		name:        "primary",
		shouldFail:  true,
		failureMode: "primary error",
		models: []config.Model{
			{ID: "model-1", CostInput: 1.0, CostOutput: 2.0},
		},
	}

	fallback1Provider := &mockProvider{
		name:        "fallback1",
		shouldFail:  true,
		failureMode: "fallback1 error",
		models: []config.Model{
			{ID: "model-2", CostInput: 0.5, CostOutput: 1.0},
		},
	}

	r.RegisterProvider(primaryProvider)
	r.RegisterProvider(fallback1Provider)

	req := orchestrator.ChatRequest{
		Model:   "model-1",
		Messages: []orchestrator.ChatMessage{
			{Role: "user", Content: "Hello"},
		},
	}

	_, err := r.Chat(context.Background(), "primary/model-1", req, []string{"fallback1/model-2"})
	if err == nil {
		t.Error("expected error when all models fail")
	}
}

func TestParseModelID(t *testing.T) {
	r := newTestRouter()

	provider := &mockProvider{
		name:   "test-provider",
		models: []config.Model{{ID: "model-1"}},
	}

	r.RegisterProvider(provider)

	tests := []struct {
		name        string
		modelID     string
		expectError bool
		expectProv  string
		expectModel string
	}{
		{
			name:        "valid format",
			modelID:     "test-provider/model-1",
			expectError: false,
			expectProv:  "test-provider",
			expectModel: "model-1",
		},
		{
			name:        "invalid format - no slash",
			modelID:     "test-provider-model-1",
			expectError: true,
		},
		{
			name:        "invalid format - too many slashes",
			modelID:     "test/provider/model-1",
			expectError: true, // "test" provider doesn't exist
		},
		{
			name:        "nonexistent provider",
			modelID:     "nonexistent/model-1",
			expectError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			prov, model, err := r.parseModelID(tt.modelID)

			if tt.expectError {
				if err == nil {
					t.Error("expected error, got nil")
				}
			} else {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}

				if prov != nil && prov.Name() != tt.expectProv {
					t.Errorf("expected provider %s, got %s", tt.expectProv, prov.Name())
				}

				if model != tt.expectModel {
					t.Errorf("expected model %s, got %s", tt.expectModel, model)
				}
			}
		})
	}
}

func TestGetModel(t *testing.T) {
	r := newTestRouter()

	provider := &mockProvider{
		name: "test-provider",
		models: []config.Model{
			{
				ID:            "model-1",
				Name:          "Test Model",
				ContextWindow: 100000,
			},
		},
	}

	r.RegisterProvider(provider)

	// Get existing model
	info, err := r.GetModel("test-provider/model-1")
	if err != nil {
		t.Fatalf("failed to get model: %v", err)
	}

	if info.ID != "test-provider/model-1" {
		t.Errorf("expected ID test-provider/model-1, got %s", info.ID)
	}

	if info.Provider != "test-provider" {
		t.Errorf("expected provider test-provider, got %s", info.Provider)
	}

	if info.Config.Name != "Test Model" {
		t.Errorf("expected name 'Test Model', got %s", info.Config.Name)
	}

	// Get nonexistent model
	_, err = r.GetModel("nonexistent/model-1")
	if err == nil {
		t.Error("expected error for nonexistent model")
	}
}

func TestListModels(t *testing.T) {
	r := newTestRouter()

	provider1 := &mockProvider{
		name: "provider1",
		models: []config.Model{
			{ID: "model-1"},
			{ID: "model-2"},
		},
	}

	provider2 := &mockProvider{
		name: "provider2",
		models: []config.Model{
			{ID: "model-3"},
		},
	}

	r.RegisterProvider(provider1)
	r.RegisterProvider(provider2)

	models := r.ListModels()

	if len(models) != 3 {
		t.Errorf("expected 3 models, got %d", len(models))
	}

	// Check that all models are present
	ids := make(map[string]bool)
	for _, m := range models {
		ids[m.ID] = true
	}

	expectedIDs := []string{
		"provider1/model-1",
		"provider1/model-2",
		"provider2/model-3",
	}

	for _, id := range expectedIDs {
		if !ids[id] {
			t.Errorf("expected model %s in list", id)
		}
	}
}

func TestGetCost(t *testing.T) {
	r := newTestRouter()

	// Get cost for nonexistent model (should return zero struct)
	cost := r.GetCost("nonexistent/model")
	if cost.TotalRequests != 0 {
		t.Errorf("expected 0 requests for nonexistent model, got %d", cost.TotalRequests)
	}

	// Make a request and check cost
	provider := &mockProvider{
		name: "test-provider",
		models: []config.Model{
			{
				ID:         "model-1",
				CostInput:  1.0,
				CostOutput: 2.0,
			},
		},
	}

	r.RegisterProvider(provider)

	req := orchestrator.ChatRequest{
		Model:   "model-1",
		Messages: []orchestrator.ChatMessage{{Role: "user", Content: "Hi"}},
	}

	_, _ = r.Chat(context.Background(), "test-provider/model-1", req, nil)

	cost = r.GetCost("test-provider/model-1")
	if cost.TotalRequests != 1 {
		t.Errorf("expected 1 request, got %d", cost.TotalRequests)
	}
}

func TestGetAllCosts(t *testing.T) {
	r := newTestRouter()

	provider := &mockProvider{
		name: "test-provider",
		models: []config.Model{
			{ID: "model-1", CostInput: 1.0, CostOutput: 2.0},
			{ID: "model-2", CostInput: 0.5, CostOutput: 1.0},
		},
	}

	r.RegisterProvider(provider)

	// Make requests to both models
	req := orchestrator.ChatRequest{
		Model:   "model-1",
		Messages: []orchestrator.ChatMessage{{Role: "user", Content: "Hi"}},
	}

	_, _ = r.Chat(context.Background(), "test-provider/model-1", req, nil)
	_, _ = r.Chat(context.Background(), "test-provider/model-2", req, nil)

	costs := r.GetAllCosts()

	if len(costs) != 2 {
		t.Errorf("expected 2 cost entries, got %d", len(costs))
	}

	if costs["test-provider/model-1"].TotalRequests != 1 {
		t.Error("expected model-1 to have 1 request")
	}

	if costs["test-provider/model-2"].TotalRequests != 1 {
		t.Error("expected model-2 to have 1 request")
	}
}

func TestSelectModel(t *testing.T) {
	r := newTestRouter()

	routing := config.ModelRouting{
		Simple:   "provider/simple-model",
		Complex:  "provider/complex-model",
		Critical: "provider/critical-model",
	}

	tests := []struct {
		complexity string
		expected   string
	}{
		{"simple", "provider/simple-model"},
		{"complex", "provider/complex-model"},
		{"critical", "provider/critical-model"},
		{"unknown", "provider/complex-model"}, // Default to complex
	}

	for _, tt := range tests {
		t.Run(tt.complexity, func(t *testing.T) {
			result := r.SelectModel(tt.complexity, routing)
			if result != tt.expected {
				t.Errorf("expected %s, got %s", tt.expected, result)
			}
		})
	}
}

func TestConcurrentChat(t *testing.T) {
	r := newTestRouter()

	provider := &mockProvider{
		name: "test-provider",
		models: []config.Model{
			{ID: "model-1", CostInput: 1.0, CostOutput: 2.0},
		},
	}

	r.RegisterProvider(provider)

	// Make concurrent requests
	done := make(chan bool, 10)

	for i := 0; i < 10; i++ {
		go func() {
			req := orchestrator.ChatRequest{
				Model:   "model-1",
				Messages: []orchestrator.ChatMessage{{Role: "user", Content: "Hi"}},
			}

			_, err := r.Chat(context.Background(), "test-provider/model-1", req, nil)
			if err != nil {
				t.Errorf("concurrent chat failed: %v", err)
			}

			done <- true
		}()
	}

	// Wait for all requests
	for i := 0; i < 10; i++ {
		<-done
	}

	// Check that all requests were tracked
	cost := r.GetCost("test-provider/model-1")
	if cost.TotalRequests != 10 {
		t.Errorf("expected 10 requests, got %d", cost.TotalRequests)
	}
}

func TestChatModelNotFound(t *testing.T) {
	r := newTestRouter()

	req := orchestrator.ChatRequest{
		Model:   "model-1",
		Messages: []orchestrator.ChatMessage{{Role: "user", Content: "Hi"}},
	}

	_, err := r.Chat(context.Background(), "nonexistent/model-1", req, nil)
	if err == nil {
		t.Error("expected error for nonexistent model")
	}
}
