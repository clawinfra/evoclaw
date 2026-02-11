package models

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/orchestrator"
)

// AnthropicProvider implements ModelProvider for Anthropic's API
type AnthropicProvider struct {
	name    string
	baseURL string
	apiKey  string
	models  []config.Model
	client  *http.Client
}

type anthropicRequest struct {
	Model     string             `json:"model"`
	MaxTokens int                `json:"max_tokens"`
	System    string             `json:"system,omitempty"`
	Messages  []anthropicMessage `json:"messages"`
}

type anthropicMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type anthropicResponse struct {
	ID      string `json:"id"`
	Type    string `json:"type"`
	Role    string `json:"role"`
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	Model      string `json:"model"`
	StopReason string `json:"stop_reason"`
	Usage      struct {
		InputTokens  int `json:"input_tokens"`
		OutputTokens int `json:"output_tokens"`
	} `json:"usage"`
}

type anthropicError struct {
	Type  string `json:"type"`
	Error struct {
		Type    string `json:"type"`
		Message string `json:"message"`
	} `json:"error"`
}

// NewAnthropicProvider creates a new Anthropic provider
func NewAnthropicProvider(cfg config.ProviderConfig) *AnthropicProvider {
	baseURL := cfg.BaseURL
	if baseURL == "" {
		baseURL = "https://api.anthropic.com"
	}
	return &AnthropicProvider{
		baseURL: baseURL,
		apiKey:  cfg.APIKey,
		models:  cfg.Models,
		client: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

func (p *AnthropicProvider) Name() string {
	if p.name != "" {
		return p.name
	}
	return "anthropic"
}

// SetName overrides the default provider name (for multiple Anthropic-compatible endpoints)
func (p *AnthropicProvider) SetName(name string) {
	p.name = name
}

func (p *AnthropicProvider) Models() []config.Model { return p.models }

func (p *AnthropicProvider) Chat(ctx context.Context, req orchestrator.ChatRequest) (*orchestrator.ChatResponse, error) {
	// Convert messages
	msgs := make([]anthropicMessage, len(req.Messages))
	for i, m := range req.Messages {
		msgs[i] = anthropicMessage{
			Role:    m.Role,
			Content: m.Content,
		}
	}

	maxTokens := req.MaxTokens
	if maxTokens == 0 {
		maxTokens = 4096
	}

	body := anthropicRequest{
		Model:     req.Model,
		MaxTokens: maxTokens,
		System:    req.SystemPrompt,
		Messages:  msgs,
	}

	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, "POST", p.baseURL+"/v1/messages", bytes.NewReader(jsonBody))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("x-api-key", p.apiKey)
	httpReq.Header.Set("anthropic-version", "2023-06-01")

	resp, err := p.client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != 200 {
		var apiErr anthropicError
		if err := json.Unmarshal(respBody, &apiErr); err != nil {
			return nil, fmt.Errorf("API error %d (failed to parse error body: %w)", resp.StatusCode, err)
		}
		return nil, fmt.Errorf("API error %d: %s - %s",
			resp.StatusCode, apiErr.Error.Type, apiErr.Error.Message)
	}

	var apiResp anthropicResponse
	if err := json.Unmarshal(respBody, &apiResp); err != nil {
		return nil, fmt.Errorf("unmarshal response: %w", err)
	}

	content := ""
	for _, c := range apiResp.Content {
		if c.Type == "text" {
			content += c.Text
		}
	}

	return &orchestrator.ChatResponse{
		Content:      content,
		Model:        apiResp.Model,
		TokensInput:  apiResp.Usage.InputTokens,
		TokensOutput: apiResp.Usage.OutputTokens,
		FinishReason: apiResp.StopReason,
	}, nil
}
