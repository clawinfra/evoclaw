package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"log/slog"
)

// ToolLoop manages the multi-turn tool execution loop
type ToolLoop struct {
	orchestrator *Orchestrator
	toolManager  *ToolManager
	logger       *slog.Logger

	// Config
	maxIterations  int
	errorLimit     int
	defaultTimeout time.Duration
}

// ToolCall represents a tool invocation from the LLM
type ToolCall struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// ToolResult represents the result of a tool execution
type ToolResult struct {
	Tool      string `json:"tool"`
	Status    string `json:"status"` // "success", "error"
	Result    string `json:"result,omitempty"`
	Error     string `json:"error,omitempty"`
	ErrorType string `json:"error_type,omitempty"`
	ElapsedMs int64  `json:"elapsed_ms"`
	ExitCode  int    `json:"exit_code,omitempty"`
}

// ToolLoopMetrics tracks tool loop performance
type ToolLoopMetrics struct {
	TotalIterations int
	ToolCalls       int
	SuccessCount    int
	ErrorCount      int
	TimeoutCount    int
	TotalDuration   time.Duration
}

// NewToolLoop creates a new tool loop
func NewToolLoop(orch *Orchestrator, tm *ToolManager) *ToolLoop {
	return &ToolLoop{
		orchestrator:   orch,
		toolManager:    tm,
		logger:         orch.logger.With("component", "tool_loop"),
		maxIterations:  10, // Configurable
		errorLimit:     3,  // Configurable
		defaultTimeout: 30 * time.Second,
	}
}

// Execute runs the tool loop for a message
func (tl *ToolLoop) Execute(agent *AgentState, msg Message, model string) (*Response, *ToolLoopMetrics, error) {
	startTime := time.Now()
	metrics := &ToolLoopMetrics{}

	// Generate tool schemas
	tools, err := tl.toolManager.GenerateSchemas()
	if err != nil {
		return nil, nil, fmt.Errorf("generate tool schemas: %w", err)
	}

	// Append edge_call tool if any edge agents are online.
	// This is a single generic tool — no per-device schema needed.
	if schema, ok := tl.orchestrator.buildEdgeCallSchema(); ok {
		tools = append(tools, schema)
	}

	// Initialize conversation history
	messages := []ChatMessage{
		{Role: "user", Content: msg.Content},
	}

	consecutiveErrors := 0

	// Tool loop
	for iteration := 0; iteration < tl.maxIterations; iteration++ {
		metrics.TotalIterations++

		// Call LLM
		llmResp, toolCalls, err := tl.callLLM(messages, tools, model, agent.Def.SystemPrompt)
		if err != nil {
			return nil, nil, fmt.Errorf("call LLM: %w", err)
		}

		// Add assistant response to history
		assistantMsg := ChatMessage{
			Role:    "assistant",
			Content: llmResp.Content,
		}

		// Include tool calls if present
		if len(toolCalls) > 0 {
			assistantMsg.ToolCalls = toolCalls
		}

		messages = append(messages, assistantMsg)

		// If no tool calls, we're done
		if len(toolCalls) == 0 {
			tl.logger.Info("tool loop complete", "iterations", iteration+1)
			break
		}

		// Execute each tool call (Phase 1: single tool only)
		for _, toolCall := range toolCalls {
			metrics.ToolCalls++

			toolResult, err := tl.executeToolCall(agent, toolCall)
			if err != nil {
				consecutiveErrors++
				metrics.ErrorCount++

				// Add error as tool result
				errorMsg := fmt.Sprintf("Error executing %s: %v", toolCall.Name, err)
				messages = append(messages, ChatMessage{
					Role:       "tool",
					ToolCallID: toolCall.ID,
					Content:    errorMsg,
				})

				if consecutiveErrors >= tl.errorLimit {
					return nil, nil, fmt.Errorf("too many consecutive errors (%d)", consecutiveErrors)
				}
				continue
			}

			consecutiveErrors = 0 // Reset error counter

			if toolResult.Status == "success" {
				metrics.SuccessCount++
			} else {
				metrics.ErrorCount++
				if toolResult.ErrorType == "timeout" {
					metrics.TimeoutCount++
				}
			}

			// Add tool result to conversation
			toolMsg := tl.formatToolResult(toolCall, toolResult)
			messages = append(messages, toolMsg)
		}
	}

	metrics.TotalDuration = time.Since(startTime)

	// Final LLM call to generate response
	finalResp, _, err := tl.callLLM(messages, tools, model, agent.Def.SystemPrompt)
	if err != nil {
		return nil, metrics, fmt.Errorf("final LLM call: %w", err)
	}

	return &Response{
		AgentID:   agent.ID,
		Content:   finalResp.Content,
		Channel:   msg.Channel,
		To:        msg.From,
		ReplyTo:   msg.ID,
		MessageID: msg.ID,
		Model:     model,
	}, metrics, nil
}

// callLLM calls the LLM with conversation history and tools
func (tl *ToolLoop) callLLM(messages []ChatMessage, tools []ToolSchema, model, systemPrompt string) (*ChatResponse, []ToolCall, error) {
	// Find provider
	provider := tl.orchestrator.findProvider(model)
	if provider == nil {
		return nil, nil, fmt.Errorf("no provider for model: %s", model)
	}

	// Extract just the model ID (after the /) for the API request
	modelID := model
	if idx := strings.Index(model, "/"); idx > 0 {
		modelID = model[idx+1:]
	}

	// Prepare request with tools
	req := ChatRequest{
		Model:        modelID,
		SystemPrompt: systemPrompt, // Pass agent's system prompt
		Messages:     messages,
		Tools:        tools, // Include tool schemas for function calling
		MaxTokens:    4096,
		Temperature:  0.7,
	}

	// Call LLM
	resp, err := provider.Chat(tl.orchestrator.ctx, req)
	if err != nil {
		return nil, nil, err
	}

	// Parse tool calls from response
	var toolCalls []ToolCall
	if resp.ToolCalls != nil {
		toolCalls = resp.ToolCalls
	}

	return resp, toolCalls, nil
}

// executeToolCall executes a single tool call
func (tl *ToolLoop) executeToolCall(agent *AgentState, toolCall ToolCall) (*ToolResult, error) {
	start := time.Now()

	// edge_call: NL passthrough to a named edge agent's own tool loop.
	// No tool schema registration needed on the edge — it handles routing itself.
	if toolCall.Name == "edge_call" {
		return tl.executeEdgeCall(agent, toolCall, start)
	}

	// Generate request ID
	requestID := fmt.Sprintf("tool-%d", time.Now().UnixNano())

	// Build command for edge agent
	cmd := EdgeAgentCommand{
		Command:   "tool",
		RequestID: requestID,
		Payload: map[string]interface{}{
			"tool":       toolCall.Name,
			"parameters": toolCall.Arguments,
			"timeout_ms": tl.toolManager.GetToolTimeout(toolCall.Name).Milliseconds(),
		},
	}

	// Serialize command
	payload, err := json.Marshal(cmd)
	if err != nil {
		return nil, fmt.Errorf("marshal tool command: %w", err)
	}

	// Get MQTT channel
	mqttChan, ok := tl.orchestrator.channels["mqtt"]
	if !ok {
		return nil, fmt.Errorf("MQTT channel not available")
	}

	// Send command via MQTT
	resp := Response{
		AgentID: agent.ID,
		Content: string(payload),
		Channel: "mqtt",
		To:      agent.ID,
		Metadata: map[string]string{
			"command":    "tool",
			"request_id": requestID,
		},
	}

	if err := mqttChan.Send(tl.orchestrator.ctx, resp); err != nil {
		return nil, fmt.Errorf("send tool command: %w", err)
	}

	// Wait for result
	timeout := tl.toolManager.GetToolTimeout(toolCall.Name)
	result, err := tl.waitForToolResult(requestID, timeout)
	if err != nil {
		return nil, err
	}

	result.ElapsedMs = time.Since(start).Milliseconds()
	return result, nil
}

// waitForToolResult waits for a tool result from the edge agent
func (tl *ToolLoop) waitForToolResult(requestID string, timeout time.Duration) (*ToolResult, error) {
	ctx, cancel := context.WithTimeout(tl.orchestrator.ctx, timeout)
	defer cancel()

	// Subscribe to result channel
	resultChan := make(chan *ToolResult, 1)

	// Register result handler
	tl.orchestrator.RegisterResultHandler(requestID, func(result *ToolResult) {
		resultChan <- result
	})

	select {
	case result := <-resultChan:
		return result, nil
	case <-ctx.Done():
		return nil, fmt.Errorf("timeout waiting for tool result")
	}
}

// formatToolResult formats a tool result as a tool message for the LLM
func (tl *ToolLoop) formatToolResult(toolCall ToolCall, result *ToolResult) ChatMessage {
	content := result.Result
	if result.Status == "error" {
		content = fmt.Sprintf("Error: %s", result.Error)
	}

	return ChatMessage{
		Role:       "tool",
		ToolCallID: toolCall.ID,
		Content:    content,
	}
}

// EdgeAgentCommand represents a command sent to an edge agent
type EdgeAgentCommand struct {
	Command   string                 `json:"command"`
	RequestID string                 `json:"request_id"`
	Payload   map[string]interface{} `json:"payload"`
}

// executeEdgeCall handles the generic edge_call tool.
// It sends the query to the named edge agent via MQTT and waits for the
// agent's own LLM+tool loop to produce a natural language answer.
func (tl *ToolLoop) executeEdgeCall(agent *AgentState, toolCall ToolCall, start time.Time) (*ToolResult, error) {
	agentID, _ := toolCall.Arguments["agent_id"].(string)
	query, _ := toolCall.Arguments["query"].(string)

	if agentID == "" {
		return &ToolResult{
			Tool:   "edge_call",
			Status: "error",
			Error:  "agent_id is required",
		}, nil
	}
	if query == "" {
		// Fall back to action+params mode if query is empty
		action, _ := toolCall.Arguments["action"].(string)
		params, _ := toolCall.Arguments["params"].(map[string]interface{})
		if action != "" {
			paramJSON, _ := json.Marshal(params)
			query = fmt.Sprintf("Execute action: %s with params: %s", action, string(paramJSON))
		} else {
			return &ToolResult{
				Tool:   "edge_call",
				Status: "error",
				Error:  "either query or action is required",
			}, nil
		}
	}

	if tl.orchestrator.mqttChannel == nil {
		return &ToolResult{
			Tool:   "edge_call",
			Status: "error",
			Error:  "MQTT channel not available",
		}, nil
	}

	if !tl.orchestrator.mqttChannel.IsEdgeAgentOnline(agentID) {
		return &ToolResult{
			Tool:   "edge_call",
			Status: "error",
			Error:  fmt.Sprintf("edge agent %q is not online", agentID),
		}, nil
	}

	tl.logger.Info("dispatching edge_call", "agent", agentID, "query_len", len(query))

	ctx, cancel := context.WithTimeout(tl.orchestrator.ctx, 60*time.Second)
	defer cancel()

	resp, err := tl.orchestrator.mqttChannel.SendPromptAndWait(ctx, agentID, query, "", 60*time.Second)
	if err != nil {
		return &ToolResult{
			Tool:      "edge_call",
			Status:    "error",
			Error:     err.Error(),
			ElapsedMs: time.Since(start).Milliseconds(),
		}, nil
	}

	return &ToolResult{
		Tool:      "edge_call",
		Status:    "success",
		Result:    resp.Content,
		ElapsedMs: time.Since(start).Milliseconds(),
	}, nil
}
