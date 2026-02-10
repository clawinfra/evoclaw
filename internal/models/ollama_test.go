package models

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/orchestrator"
)

func TestNewOllamaProvider(t *testing.T) {
	cfg := config.ProviderConfig{
		BaseURL: "http://localhost:11434",
		Models: []config.Model{
			{ID: "llama3.2", Name: "Llama 3.2"},
		},
	}

	p := NewOllamaProvider(cfg)

	if p.Name() != "ollama" {
		t.Errorf("expected name 'ollama', got '%s'", p.Name())
	}

	if len(p.Models()) != 1 {
		t.Errorf("expected 1 model, got %d", len(p.Models()))
	}

	if p.baseURL != "http://localhost:11434" {
		t.Errorf("expected baseURL http://localhost:11434, got %s", p.baseURL)
	}
}

func TestNewOllamaProviderDefaultURL(t *testing.T) {
	cfg := config.ProviderConfig{
		Models: []config.Model{
			{ID: "llama3.2"},
		},
	}

	p := NewOllamaProvider(cfg)

	if p.baseURL != "http://localhost:11434" {
		t.Errorf("expected default baseURL, got %s", p.baseURL)
	}
}

func TestOllamaChatSuccess(t *testing.T) {
	// Create mock Ollama server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			t.Errorf("expected path /api/chat, got %s", r.URL.Path)
		}

		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		// Decode request to verify format
		var reqBody ollamaChatRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if reqBody.Model != "llama3.2" {
			t.Errorf("expected model llama3.2, got %s", reqBody.Model)
		}

		if reqBody.Stream {
			t.Error("expected stream to be false")
		}

		// Send mock response
		resp := ollamaChatResponse{
			Model: "llama3.2",
			Message: ollamaMessage{
				Role:    "assistant",
				Content: "Hello! How can I help you?",
			},
			Done:            true,
			PromptEvalCount: 150,
			EvalCount:       20,
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	// Create provider with mock server URL
	cfg := config.ProviderConfig{
		BaseURL: server.URL,
		Models: []config.Model{
			{ID: "llama3.2"},
		},
	}

	p := NewOllamaProvider(cfg)

	// Make chat request
	req := orchestrator.ChatRequest{
		Model:        "llama3.2",
		SystemPrompt: "You are a helpful assistant",
		Messages: []orchestrator.ChatMessage{
			{Role: "user", Content: "Hello"},
		},
		Temperature: 0.7,
		MaxTokens:   1000,
	}

	resp, err := p.Chat(context.Background(), req)
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}

	if resp.Content != "Hello! How can I help you?" {
		t.Errorf("expected 'Hello! How can I help you?', got '%s'", resp.Content)
	}

	if resp.Model != "llama3.2" {
		t.Errorf("expected model llama3.2, got %s", resp.Model)
	}

	if resp.TokensInput != 150 {
		t.Errorf("expected 150 input tokens, got %d", resp.TokensInput)
	}

	if resp.TokensOutput != 20 {
		t.Errorf("expected 20 output tokens, got %d", resp.TokensOutput)
	}

	if resp.FinishReason != "stop" {
		t.Errorf("expected finish reason 'stop', got '%s'", resp.FinishReason)
	}
}

func TestOllamaChatError(t *testing.T) {
	// Create mock server that returns error
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal server error"))
	}))
	defer server.Close()

	cfg := config.ProviderConfig{
		BaseURL: server.URL,
		Models:  []config.Model{{ID: "llama3.2"}},
	}

	p := NewOllamaProvider(cfg)

	req := orchestrator.ChatRequest{
		Model:    "llama3.2",
		Messages: []orchestrator.ChatMessage{{Role: "user", Content: "Hello"}},
	}

	_, err := p.Chat(context.Background(), req)
	if err == nil {
		t.Error("expected error when server returns 500")
	}
}

func TestOllamaChatWithSystemPrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody ollamaChatRequest
		_ = json.NewDecoder(r.Body).Decode(&reqBody)

		// Should have system message + user message
		if len(reqBody.Messages) != 2 {
			t.Errorf("expected 2 messages (system + user), got %d", len(reqBody.Messages))
		}

		if reqBody.Messages[0].Role != "system" {
			t.Errorf("expected first message to be system, got %s", reqBody.Messages[0].Role)
		}

		if reqBody.Messages[0].Content != "You are helpful" {
			t.Errorf("expected system prompt, got '%s'", reqBody.Messages[0].Content)
		}

		resp := ollamaChatResponse{
			Model:   "llama3.2",
			Message: ollamaMessage{Role: "assistant", Content: "OK"},
			Done:    true,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := config.ProviderConfig{
		BaseURL: server.URL,
		Models:  []config.Model{{ID: "llama3.2"}},
	}

	p := NewOllamaProvider(cfg)

	req := orchestrator.ChatRequest{
		Model:        "llama3.2",
		SystemPrompt: "You are helpful",
		Messages:     []orchestrator.ChatMessage{{Role: "user", Content: "Hi"}},
	}

	_, err := p.Chat(context.Background(), req)
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}
}

func TestOllamaChatMultipleMessages(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody ollamaChatRequest
		json.NewDecoder(r.Body).Decode(&reqBody)

		// Should have 3 messages
		if len(reqBody.Messages) != 3 {
			t.Errorf("expected 3 messages, got %d", len(reqBody.Messages))
		}

		resp := ollamaChatResponse{
			Model:   "llama3.2",
			Message: ollamaMessage{Role: "assistant", Content: "Response"},
			Done:    true,
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := config.ProviderConfig{
		BaseURL: server.URL,
		Models:  []config.Model{{ID: "llama3.2"}},
	}

	p := NewOllamaProvider(cfg)

	req := orchestrator.ChatRequest{
		Model: "llama3.2",
		Messages: []orchestrator.ChatMessage{
			{Role: "user", Content: "Hello"},
			{Role: "assistant", Content: "Hi"},
			{Role: "user", Content: "How are you?"},
		},
	}

	_, err := p.Chat(context.Background(), req)
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}
}
