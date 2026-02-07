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

func TestNewAnthropicProvider(t *testing.T) {
	cfg := config.ProviderConfig{
		BaseURL: "https://api.anthropic.com",
		APIKey:  "test-key",
		Models: []config.Model{
			{ID: "claude-sonnet-4", Name: "Claude Sonnet 4"},
		},
	}

	p := NewAnthropicProvider(cfg)

	if p.Name() != "anthropic" {
		t.Errorf("expected name 'anthropic', got '%s'", p.Name())
	}

	if len(p.Models()) != 1 {
		t.Errorf("expected 1 model, got %d", len(p.Models()))
	}

	if p.apiKey != "test-key" {
		t.Errorf("expected API key 'test-key', got %s", p.apiKey)
	}
}

func TestAnthropicChatSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" {
			t.Errorf("expected path /v1/messages, got %s", r.URL.Path)
		}

		if r.Method != "POST" {
			t.Errorf("expected POST, got %s", r.Method)
		}

		// Check headers
		if r.Header.Get("x-api-key") == "" {
			t.Error("expected x-api-key header")
		}

		if r.Header.Get("anthropic-version") == "" {
			t.Error("expected anthropic-version header")
		}

		// Decode and verify request
		var reqBody anthropicRequest
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if reqBody.Model != "claude-sonnet-4" {
			t.Errorf("expected model claude-sonnet-4, got %s", reqBody.Model)
		}

		if reqBody.MaxTokens == 0 {
			t.Error("expected MaxTokens to be set")
		}

		// Send mock response
		resp := anthropicResponse{
			ID:   "msg_123",
			Type: "message",
			Role: "assistant",
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{
				{Type: "text", Text: "Hello! How can I assist you today?"},
			},
			Model:      "claude-sonnet-4",
			StopReason: "end_turn",
			Usage: struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			}{
				InputTokens:  100,
				OutputTokens: 25,
			},
		}

		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := config.ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Models:  []config.Model{{ID: "claude-sonnet-4"}},
	}

	p := NewAnthropicProvider(cfg)

	req := orchestrator.ChatRequest{
		Model:        "claude-sonnet-4",
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

	if resp.Content != "Hello! How can I assist you today?" {
		t.Errorf("unexpected content: %s", resp.Content)
	}

	if resp.Model != "claude-sonnet-4" {
		t.Errorf("expected model claude-sonnet-4, got %s", resp.Model)
	}

	if resp.TokensInput != 100 {
		t.Errorf("expected 100 input tokens, got %d", resp.TokensInput)
	}

	if resp.TokensOutput != 25 {
		t.Errorf("expected 25 output tokens, got %d", resp.TokensOutput)
	}

	if resp.FinishReason != "end_turn" {
		t.Errorf("expected finish reason 'end_turn', got '%s'", resp.FinishReason)
	}
}

func TestAnthropicChatError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		errResp := anthropicError{
			Type: "error",
			Error: struct {
				Type    string `json:"type"`
				Message string `json:"message"`
			}{
				Type:    "authentication_error",
				Message: "Invalid API key",
			},
		}
		_ = json.NewEncoder(w).Encode(errResp)
	}))
	defer server.Close()

	cfg := config.ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "bad-key",
		Models:  []config.Model{{ID: "claude-sonnet-4"}},
	}

	p := NewAnthropicProvider(cfg)

	req := orchestrator.ChatRequest{
		Model:    "claude-sonnet-4",
		Messages: []orchestrator.ChatMessage{{Role: "user", Content: "Hello"}},
	}

	_, err := p.Chat(context.Background(), req)
	if err == nil {
		t.Error("expected error when authentication fails")
	}
}

func TestAnthropicChatWithSystemPrompt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var reqBody anthropicRequest
		_ = json.NewDecoder(r.Body).Decode(&reqBody)

		if reqBody.System != "You are helpful" {
			t.Errorf("expected system prompt, got '%s'", reqBody.System)
		}

		resp := anthropicResponse{
			ID:   "msg_123",
			Type: "message",
			Role: "assistant",
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{{Type: "text", Text: "OK"}},
			Model:      "claude-sonnet-4",
			StopReason: "end_turn",
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := config.ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Models:  []config.Model{{ID: "claude-sonnet-4"}},
	}

	p := NewAnthropicProvider(cfg)

	req := orchestrator.ChatRequest{
		Model:        "claude-sonnet-4",
		SystemPrompt: "You are helpful",
		Messages:     []orchestrator.ChatMessage{{Role: "user", Content: "Hi"}},
	}

	_, err := p.Chat(context.Background(), req)
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}
}

func TestAnthropicChatMultipleContentBlocks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := anthropicResponse{
			ID:   "msg_123",
			Type: "message",
			Role: "assistant",
			Content: []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			}{
				{Type: "text", Text: "First part. "},
				{Type: "text", Text: "Second part."},
			},
			Model:      "claude-sonnet-4",
			StopReason: "end_turn",
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	cfg := config.ProviderConfig{
		BaseURL: server.URL,
		APIKey:  "test-key",
		Models:  []config.Model{{ID: "claude-sonnet-4"}},
	}

	p := NewAnthropicProvider(cfg)

	req := orchestrator.ChatRequest{
		Model:    "claude-sonnet-4",
		Messages: []orchestrator.ChatMessage{{Role: "user", Content: "Hello"}},
	}

	resp, err := p.Chat(context.Background(), req)
	if err != nil {
		t.Fatalf("chat failed: %v", err)
	}

	// Should concatenate content blocks
	expected := "First part. Second part."
	if resp.Content != expected {
		t.Errorf("expected '%s', got '%s'", expected, resp.Content)
	}
}
