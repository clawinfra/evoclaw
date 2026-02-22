// Package interfaces defines the core trait-driven interfaces for EvoClaw.
// All subsystems (providers, memory, tools, channels, observers) implement
// these interfaces, making them swappable via configuration.
package interfaces

import "time"

// ChatMessage represents a single message in a conversation.
type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the input to an LLM provider.
type ChatRequest struct {
	Model        string        `json:"model"`
	SystemPrompt string        `json:"system_prompt,omitempty"`
	Messages     []ChatMessage `json:"messages"`
	MaxTokens    int           `json:"max_tokens,omitempty"`
	Temperature  float64       `json:"temperature,omitempty"`
	Tools        []ToolSchema  `json:"tools,omitempty"`
}

// ChatResponse is the output from an LLM provider.
type ChatResponse struct {
	Content    string     `json:"content"`
	Model      string     `json:"model"`
	TokensIn   int        `json:"tokens_in"`
	TokensOut  int        `json:"tokens_out"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	FinishReason string   `json:"finish_reason,omitempty"`
}

// ToolCall represents a tool invocation requested by the LLM.
type ToolCall struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ToolSchema describes a tool's input schema.
type ToolSchema struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ToolResult is the output of a tool execution.
type ToolResult struct {
	Output string `json:"output"`
	Error  string `json:"error,omitempty"`
}

// MemoryEntry is a single item returned from memory retrieval.
type MemoryEntry struct {
	Key       string            `json:"key"`
	Content   []byte            `json:"content"`
	Metadata  map[string]string `json:"metadata,omitempty"`
	Score     float64           `json:"score,omitempty"`
	Timestamp time.Time         `json:"timestamp"`
}

// OutboundMessage is a message sent through a channel.
type OutboundMessage struct {
	To      string `json:"to"`
	Content string `json:"content"`
	ReplyTo string `json:"reply_to,omitempty"`
}

// InboundMessage is a message received from a channel.
type InboundMessage struct {
	From      string    `json:"from"`
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
	Channel   string    `json:"channel"`
	ID        string    `json:"id"`
}

// ObservedRequest captures telemetry for an incoming request.
type ObservedRequest struct {
	ID        string    `json:"id"`
	Model     string    `json:"model"`
	TokensIn  int       `json:"tokens_in"`
	Timestamp time.Time `json:"timestamp"`
}

// ObservedResponse captures telemetry for a completed response.
type ObservedResponse struct {
	RequestID  string        `json:"request_id"`
	Model      string        `json:"model"`
	TokensOut  int           `json:"tokens_out"`
	Latency    time.Duration `json:"latency"`
	Success    bool          `json:"success"`
}

// ObservedError captures telemetry for an error.
type ObservedError struct {
	RequestID string `json:"request_id"`
	Model     string `json:"model"`
	Error     string `json:"error"`
	Retryable bool   `json:"retryable"`
}
