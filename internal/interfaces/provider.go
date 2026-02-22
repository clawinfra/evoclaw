package interfaces

import "context"

// Provider is the interface for LLM providers.
// Implementations include OpenAI, Anthropic, Ollama, etc.
type Provider interface {
	// Name returns the provider identifier (e.g., "openai", "anthropic").
	Name() string

	// Chat sends a chat request and returns the response.
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)

	// Models returns the list of model identifiers this provider supports.
	Models() []string

	// HealthCheck verifies the provider is reachable and functional.
	HealthCheck(ctx context.Context) error
}
