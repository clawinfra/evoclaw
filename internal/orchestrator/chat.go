package orchestrator

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// ChatSyncRequest represents a synchronous chat request
type ChatSyncRequest struct {
	AgentID        string
	UserID         string
	Message        string
	ConversationID string
	// History is prior messages to include in context
	History []ChatMessage
}

// ChatSyncResponse represents the response from a synchronous chat
type ChatSyncResponse struct {
	AgentID      string `json:"agent_id"`
	Response     string `json:"response"`
	Model        string `json:"model"`
	ElapsedMs    int64  `json:"elapsed_ms"`
	TokensInput  int    `json:"tokens_input"`
	TokensOutput int    `json:"tokens_output"`
}

// ChatSync sends a message to an agent and waits for the LLM response.
// This is the synchronous version of processWithAgent used by Telegram bot and Dashboard chat.
func (o *Orchestrator) ChatSync(ctx context.Context, req ChatSyncRequest) (*ChatSyncResponse, error) {
	start := time.Now()

	// 1. Find agent
	o.mu.RLock()
	agent, ok := o.agents[req.AgentID]
	o.mu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("agent not found: %s", req.AgentID)
	}

	// 2. Select model
	model := agent.Def.Model
	if model == "" {
		model = o.cfg.Models.Routing.Complex
	}

	// Mark agent as running
	agent.mu.Lock()
	agent.Status = "running"
	agent.LastActive = time.Now()
	agent.MessageCount++
	agent.mu.Unlock()

	defer func() {
		agent.mu.Lock()
		agent.Status = "idle"
		agent.mu.Unlock()
	}()

	// 3. Build chat request with conversation history
	modelName := model
	if parts := strings.SplitN(model, "/", 2); len(parts) == 2 {
		modelName = parts[1]
	}

	messages := make([]ChatMessage, 0, len(req.History)+1)
	// Include conversation history
	for _, msg := range req.History {
		messages = append(messages, msg)
	}
	// Add current user message
	messages = append(messages, ChatMessage{Role: "user", Content: req.Message})

	chatReq := ChatRequest{
		Model:        modelName,
		SystemPrompt: agent.Def.SystemPrompt,
		Messages:     messages,
		MaxTokens:    4096,
		Temperature:  0.7,
	}

	// 4. Find provider
	provider := o.findProvider(model)
	if provider == nil {
		agent.mu.Lock()
		agent.ErrorCount++
		agent.Metrics.FailedActions++
		agent.mu.Unlock()
		return nil, fmt.Errorf("no provider for model: %s", model)
	}

	// 5. Call LLM provider
	resp, err := provider.Chat(ctx, chatReq)
	if err != nil {
		agent.mu.Lock()
		agent.ErrorCount++
		agent.Metrics.FailedActions++
		agent.mu.Unlock()
		return nil, fmt.Errorf("LLM error: %w", err)
	}

	elapsed := time.Since(start)

	// 6. Update metrics
	agent.mu.Lock()
	agent.Metrics.TotalActions++
	agent.Metrics.SuccessfulActions++
	agent.Metrics.TokensUsed += int64(resp.TokensInput + resp.TokensOutput)
	n := float64(agent.Metrics.TotalActions)
	agent.Metrics.AvgResponseMs = agent.Metrics.AvgResponseMs*(n-1)/n + float64(elapsed.Milliseconds())/n
	agent.mu.Unlock()

	// Report to evolution engine if available
	if o.evolution != nil {
		agent.mu.RLock()
		metrics := agent.Metrics
		agent.mu.RUnlock()
		successRate := float64(metrics.SuccessfulActions) / float64(metrics.TotalActions)
		evalMetrics := map[string]float64{
			"successRate":   successRate,
			"avgResponseMs": metrics.AvgResponseMs,
			"costUSD":       metrics.CostUSD,
			"totalActions":  float64(metrics.TotalActions),
		}
		o.evolution.Evaluate(agent.ID, evalMetrics)
	}

	// Report to external reporter
	o.mu.RLock()
	reporter := o.reporter
	o.mu.RUnlock()
	if reporter != nil {
		_ = reporter.RecordMessage(req.AgentID)
		_ = reporter.UpdateMetrics(req.AgentID, resp.TokensInput+resp.TokensOutput, 0, elapsed.Milliseconds(), true)
	}

	o.logger.Info("chat sync completed",
		"agent", req.AgentID,
		"model", model,
		"elapsed", elapsed,
		"tokens", resp.TokensInput+resp.TokensOutput,
	)

	return &ChatSyncResponse{
		AgentID:      req.AgentID,
		Response:     resp.Content,
		Model:        model,
		ElapsedMs:    elapsed.Milliseconds(),
		TokensInput:  resp.TokensInput,
		TokensOutput: resp.TokensOutput,
	}, nil
}

// ListAgentIDs returns just the IDs of registered agents
func (o *Orchestrator) ListAgentIDs() []string {
	o.mu.RLock()
	defer o.mu.RUnlock()

	ids := make([]string, 0, len(o.agents))
	for id := range o.agents {
		ids = append(ids, id)
	}
	return ids
}

// GetAgentInfo returns agent info by ID (returns nil if not found)
func (o *Orchestrator) GetAgentInfo(agentID string) *AgentInfo {
	o.mu.RLock()
	agent, ok := o.agents[agentID]
	o.mu.RUnlock()

	if !ok {
		return nil
	}

	agent.mu.RLock()
	defer agent.mu.RUnlock()

	info := &AgentInfo{
		ID:           agent.ID,
		Def:          agent.Def,
		Status:       agent.Status,
		StartedAt:    agent.StartedAt,
		LastActive:   agent.LastActive,
		MessageCount: agent.MessageCount,
		ErrorCount:   agent.ErrorCount,
		Metrics:      agent.Metrics,
	}
	return info
}
