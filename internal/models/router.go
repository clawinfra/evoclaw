package models

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/orchestrator"
)

// Router handles intelligent model selection and fallback chains
type Router struct {
	providers map[string]orchestrator.ModelProvider
	models    map[string]*ModelInfo // model ID -> full info
	costs     *CostTracker
	logger    *slog.Logger
	mu        sync.RWMutex
}

// ModelInfo contains full information about a model
type ModelInfo struct {
	ID           string
	Provider     string
	Config       config.Model
	ProviderImpl orchestrator.ModelProvider
}

// CostTracker tracks API usage costs
type CostTracker struct {
	mu    sync.RWMutex
	costs map[string]*ModelCost // model ID -> costs
}

// ModelCost tracks cost for a specific model
type ModelCost struct {
	TotalRequests   int64
	TotalTokensIn   int64
	TotalTokensOut  int64
	TotalCostUSD    float64
	LastRequestTime int64
}

// NewRouter creates a new model router
func NewRouter(logger *slog.Logger) *Router {
	return &Router{
		providers: make(map[string]orchestrator.ModelProvider),
		models:    make(map[string]*ModelInfo),
		costs: &CostTracker{
			costs: make(map[string]*ModelCost),
		},
		logger: logger.With("component", "model-router"),
	}
}

// RegisterProvider adds a provider and indexes all its models
func (r *Router) RegisterProvider(p orchestrator.ModelProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()

	providerName := p.Name()
	r.providers[providerName] = p

	// Index all models from this provider
	for _, model := range p.Models() {
		fullID := fmt.Sprintf("%s/%s", providerName, model.ID)
		r.models[fullID] = &ModelInfo{
			ID:           fullID,
			Provider:     providerName,
			Config:       model,
			ProviderImpl: p,
		}
		r.logger.Info("model registered",
			"id", fullID,
			"name", model.Name,
			"context", model.ContextWindow,
		)
	}

	r.logger.Info("provider registered",
		"name", providerName,
		"models", len(p.Models()),
	)
}

// Chat routes a chat request to the appropriate model with fallback
func (r *Router) Chat(ctx context.Context, modelID string, req orchestrator.ChatRequest, fallback []string) (*orchestrator.ChatResponse, error) {
	// Try primary model
	resp, err := r.chatSingle(ctx, modelID, req)
	if err == nil {
		return resp, nil
	}

	r.logger.Warn("primary model failed, trying fallback",
		"primary", modelID,
		"error", err,
		"fallbacks", len(fallback),
	)

	// Try fallback chain
	for i, fbModel := range fallback {
		r.logger.Info("trying fallback", "model", fbModel, "attempt", i+1)
		resp, fbErr := r.chatSingle(ctx, fbModel, req)
		if fbErr == nil {
			return resp, nil
		}
		r.logger.Warn("fallback failed", "model", fbModel, "error", fbErr)
	}

	// All failed
	return nil, fmt.Errorf("all models failed, primary error: %w", err)
}

// chatSingle performs a single chat request and tracks cost
func (r *Router) chatSingle(ctx context.Context, modelID string, req orchestrator.ChatRequest) (*orchestrator.ChatResponse, error) {
	// Parse model ID (format: "provider/model-name")
	provider, model, err := r.parseModelID(modelID)
	if err != nil {
		return nil, err
	}

	// Update request with correct model name
	req.Model = model

	// Get provider implementation
	r.mu.RLock()
	modelInfo, ok := r.models[modelID]
	r.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("model not found: %s", modelID)
	}

	// Make the request
	resp, err := provider.Chat(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("chat error: %w", err)
	}

	// Track cost
	r.trackCost(modelID, modelInfo.Config, resp)

	return resp, nil
}

// parseModelID splits "provider/model" into components
func (r *Router) parseModelID(modelID string) (orchestrator.ModelProvider, string, error) {
	parts := strings.SplitN(modelID, "/", 2)
	if len(parts) != 2 {
		return nil, "", fmt.Errorf("invalid model ID format (expected provider/model): %s", modelID)
	}

	providerName := parts[0]
	modelName := parts[1]

	r.mu.RLock()
	provider, ok := r.providers[providerName]
	r.mu.RUnlock()

	if !ok {
		return nil, "", fmt.Errorf("provider not found: %s", providerName)
	}

	return provider, modelName, nil
}

// trackCost records usage and cost for a model
func (r *Router) trackCost(modelID string, modelCfg config.Model, resp *orchestrator.ChatResponse) {
	r.costs.mu.Lock()
	defer r.costs.mu.Unlock()

	cost, ok := r.costs.costs[modelID]
	if !ok {
		cost = &ModelCost{}
		r.costs.costs[modelID] = cost
	}

	cost.TotalRequests++
	cost.TotalTokensIn += int64(resp.TokensInput)
	cost.TotalTokensOut += int64(resp.TokensOutput)

	// Calculate cost (prices are per million tokens)
	inputCost := float64(resp.TokensInput) * modelCfg.CostInput / 1_000_000
	outputCost := float64(resp.TokensOutput) * modelCfg.CostOutput / 1_000_000
	cost.TotalCostUSD += inputCost + outputCost

	r.logger.Debug("cost tracked",
		"model", modelID,
		"tokens_in", resp.TokensInput,
		"tokens_out", resp.TokensOutput,
		"cost_usd", inputCost+outputCost,
		"total_cost_usd", cost.TotalCostUSD,
	)
}

// GetCost returns cost stats for a model
func (r *Router) GetCost(modelID string) *ModelCost {
	r.costs.mu.RLock()
	defer r.costs.mu.RUnlock()

	cost, ok := r.costs.costs[modelID]
	if !ok {
		return &ModelCost{}
	}

	// Return a copy
	return &ModelCost{
		TotalRequests:  cost.TotalRequests,
		TotalTokensIn:  cost.TotalTokensIn,
		TotalTokensOut: cost.TotalTokensOut,
		TotalCostUSD:   cost.TotalCostUSD,
	}
}

// GetAllCosts returns cost stats for all models
func (r *Router) GetAllCosts() map[string]*ModelCost {
	r.costs.mu.RLock()
	defer r.costs.mu.RUnlock()

	result := make(map[string]*ModelCost)
	for id, cost := range r.costs.costs {
		result[id] = &ModelCost{
			TotalRequests:  cost.TotalRequests,
			TotalTokensIn:  cost.TotalTokensIn,
			TotalTokensOut: cost.TotalTokensOut,
			TotalCostUSD:   cost.TotalCostUSD,
		}
	}
	return result
}

// GetModel returns info about a specific model
func (r *Router) GetModel(modelID string) (*ModelInfo, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	info, ok := r.models[modelID]
	if !ok {
		return nil, fmt.Errorf("model not found: %s", modelID)
	}

	return info, nil
}

// ListModels returns all available models
func (r *Router) ListModels() []*ModelInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()

	models := make([]*ModelInfo, 0, len(r.models))
	for _, info := range r.models {
		models = append(models, info)
	}
	return models
}

// SelectModel chooses the best model based on task complexity
func (r *Router) SelectModel(complexity string, routing config.ModelRouting) string {
	switch complexity {
	case "simple":
		return routing.Simple
	case "complex":
		return routing.Complex
	case "critical":
		return routing.Critical
	default:
		return routing.Complex
	}
}
