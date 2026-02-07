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

func TestNewOpenAIProvider(t *testing.T) {
	cfg := config.ProviderConfig{
		BaseURL: "https://api.openai.com",
		APIKey:  "test-key",
		Models: []config.Model{
			{ID: "gpt-4", Name: "GPT-4"},
		},
	}

	p := NewOpenAIProvider("openai", cfg)

	if p.Name() != "openai" {
		t.Errorf("expected name 'openai', got '%s'", p.Name())
	}

	if len(p.Models()) != 1 {
		t.Errorf("expected 1 model, got %d", len(p.Models()))
	}

	if p.apiKey != "test-key" {
		t.Errorf("expected API key 'test-key', got %s", p.apiKey)
	}
}

func TestOpenAIChatSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			t.Errorf("expected path /chat/completions, got %s", r.URL.Path)
		}

		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		// Check authorization header
		if r.Header.Get("Authorization") == "" {
			t.Error("expected Authorization header")
		}

		// Send mock response (use raw JSON to avoid struct issues)
		resp := `{
			"id": "chatcmpl-123",
			"object": "chat.completion",
			"model": "gpt-4",
			"choices": [{
				"index": 0,
				"message": {
					"role": "assistant",
					"content": "Hello! I'm here to help."
				},
				"finish_reason": "stop"
			}],
			"usage": {
				"prompt_tokens": 120,
				"completion_tokens": 30,
				"total_tokens": 150
			}
		}`

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(resp))
	}))
	defer server.Close()

	cfg := config.ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Models:  []config.Model{{ID: "gpt-4"}},
	}

	p := NewOpenAIProvider("openai", cfg)

	req := orchestrator.ChatRequest{
		Model:        "gpt-4",
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

	if resp.Content != "Hello! I'm here to help." {
		t.Errorf("unexpected content: %s", resp.Content)
	}

	if resp.Model != "gpt-4" {
		t.Errorf("expected model gpt-4, got %s", resp.Model)
	}

	if resp.TokensInput != 120 {
		t.Errorf("expected 120 input tokens, got %d", resp.TokensInput)
	}

	if resp.TokensOutput != 30 {
		t.Errorf("expected 30 output tokens, got %d", resp.TokensOutput)
	}

	if resp.FinishReason != "stop" {
		t.Errorf("expected finish reason 'stop', got '%s'", resp.FinishReason)
	}
}

func TestOpenAIChatError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error": {"message": "Invalid API key", "type": "invalid_request_error"}}`))
	}))
	defer server.Close()

	cfg := config.ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "bad-key",
		Models:  []config.Model{{ID: "gpt-4"}},
	}

	p := NewOpenAIProvider("openai", cfg)

	req := orchestrator.ChatRequest{
		Model:    "gpt-4",
		Messages: []orchestrator.ChatMessage{{Role: "user", Content: "Hello"}},
	}

	_, err := p.Chat(context.Background(), req)
	if err == nil {
		t.Error("expected error when authentication fails")
	}
}

func TestOpenAIChatWithSystemPrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody openAIRequest
		json.NewDecoder(r.Body).Decode(&reqBody)

		// Should have system message + user message
		if len(reqBody.Messages) != 2 {
			t.Errorf("expected 2 messages, got %d", len(reqBody.Messages))
		}

		if reqBody.Messages[0].Role != "system" {
			t.Errorf("expected first message to be system, got %s", reqBody.Messages[0].Role)
		}

		resp := `{"id": "chatcmpl-123", "model": "gpt-4", "choices": [{"message": {"role": "assistant", "content": "OK"}, "finish_reason": "stop"}]}`
		w.Write([]byte(resp))
	}))
	defer server.Close()

	cfg := config.ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Models:  []config.Model{{ID: "gpt-4"}},
	}

	p := NewOpenAIProvider("openai", cfg)

	req := orchestrator.ChatRequest{
		Model:        "gpt-4",
		SystemPrompt: "You are helpful",
		Messages:     []orchestrator.ChatMessage{{Role: "user", Content: "Hi"}},
	}

	_, err := p.Chat(context.Background(), req)
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}
}
