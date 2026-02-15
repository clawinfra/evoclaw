package orchestrator

import (
	"fmt"
	"log/slog"
	"time"
)

// processDirect processes a message without tools (legacy mode)
func (o *Orchestrator) processDirect(agent *AgentState, msg Message, model string) (*ChatResponse, error) {
	// Build chat request
	req := ChatRequest{
		Model:        model,
		SystemPrompt: agent.Def.SystemPrompt,
		Messages: []ChatMessage{
			{Role: "user", Content: msg.Content},
		},
		MaxTokens:   4096,
		Temperature: 0.7,
	}

	// Find provider for this model
	provider := o.findProvider(model)
	if provider == nil {
		return nil, fmt.Errorf("no provider for model: %s", model)
	}

	// Strip provider prefix from model name (e.g., "nvidia/meta/llama-3.3-70b" → "meta/llama-3.3-70b")
	modelName := model
	if idx := indexChar(model, '/'); idx > 0 {
		modelName = model[idx+1:]
	}
	req.Model = modelName

	// Call LLM
	resp, err := provider.Chat(o.ctx, req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// RegisterResultHandler registers a handler for a tool result
func (o *Orchestrator) RegisterResultHandler(requestID string, handler func(*ToolResult)) {
	o.resultMu.Lock()
	defer o.resultMu.Unlock()

	o.resultRegistry[requestID] = make(chan *ToolResult, 1)

	go func() {
		result := <-o.resultRegistry[requestID]
		handler(result)
		delete(o.resultRegistry, requestID)
	}()
}

// DeliverToolResult delivers a tool result to the waiting handler
func (o *Orchestrator) DeliverToolResult(requestID string, result *ToolResult) {
	o.resultMu.RLock()
	ch, ok := o.resultRegistry[requestID]
	o.resultMu.RUnlock()

	if ok {
		select {
		case ch <- result:
			o.logger.Debug("tool result delivered", "request_id", requestID)
		case <-time.After(5 * time.Second):
			o.logger.Warn("timeout delivering tool result", "request_id", requestID)
		}
	} else {
		o.logger.Warn("no handler for tool result", "request_id", requestID)
	}
}

// indexChar is a helper to find a character in a string
func indexChar(s string, c rune) int {
	for i, r := range s {
		if r == c {
			return i
		}
	}
	return -1
}
