package orchestrator

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"log/slog"

	"golang.org/x/sync/errgroup"

	rsiPkg "github.com/clawinfra/evoclaw/internal/rsi"
	"github.com/clawinfra/evoclaw/internal/security"
)

// ToolLoopOption is a functional option for configuring a ToolLoop.
type ToolLoopOption func(*ToolLoop)

// WithRSILogger wires an RSILogger into the ToolLoop so that every Execute()
// call automatically emits one outcome record.
func WithRSILogger(logger RSILogger) ToolLoopOption {
	return func(tl *ToolLoop) { tl.rsiLogger = logger }
}

// ToolLoop manages the multi-turn tool execution loop
type ToolLoop struct {
	orchestrator *Orchestrator
	toolManager  *ToolManager
	logger       *slog.Logger

	// Config
	maxIterations  int
	errorLimit     int
	defaultTimeout time.Duration
	maxParallel    int
	execFunc       func(agent *AgentState, call ToolCall) (*ToolResult, error)

	// RSI auto-logging
	rsiLogger RSILogger
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
	ParallelBatches int
	MaxConcurrency  int
	WallTimeSavedMs int64
}

// parallelToolResult holds the outcome of a single tool call executed in parallel.
type parallelToolResult struct {
	Index  int
	Call   ToolCall
	Result *ToolResult
	Err    error
}

// NewToolLoop creates a new tool loop
func NewToolLoop(orch *Orchestrator, tm *ToolManager, opts ...ToolLoopOption) *ToolLoop {
	tl := &ToolLoop{
		orchestrator:   orch,
		toolManager:    tm,
		logger:         orch.logger.With("component", "tool_loop"),
		maxIterations:  10, // Configurable
		errorLimit:     3,  // Configurable
		defaultTimeout: 30 * time.Second,
		maxParallel:    5,
		rsiLogger:      NoopRSILogger{},
	}
	for _, opt := range opts {
		opt(tl)
	}
	return tl
}

// executeParallel executes a batch of tool calls concurrently and returns
// results in the original call order. For a single call, it takes the fast
// path with no goroutine overhead.
func (tl *ToolLoop) executeParallel(ctx context.Context, agent *AgentState, calls []ToolCall) []parallelToolResult {
	fn := tl.execFunc
	if fn == nil {
		fn = tl.executeToolCall
	}

	results := make([]parallelToolResult, len(calls))

	if len(calls) == 1 {
		// Fast path — no goroutines
		res, err := fn(agent, calls[0])
		results[0] = parallelToolResult{Index: 0, Call: calls[0], Result: res, Err: err}
		return results
	}

	// Fan-out with bounded concurrency
	g, gCtx := errgroup.WithContext(ctx)
	g.SetLimit(tl.maxParallel)

	for i, call := range calls {
		i, call := i, call // capture loop vars
		g.Go(func() error {
			// Fast-bail if parent context already cancelled
			select {
			case <-gCtx.Done():
				results[i] = parallelToolResult{Index: i, Call: call, Err: gCtx.Err()}
				return nil
			default:
			}
			res, err := fn(agent, call)
			// Store at pre-allocated index — no mutex needed (unique index per goroutine)
			results[i] = parallelToolResult{Index: i, Call: call, Result: res, Err: err}
			return nil // never propagate errors; capture in result
		})
	}

	_ = g.Wait() // errgroup never sees non-nil errors from goroutines above
	return results
}

// logRSIOutcome emits one RSI outcome record at every Execute exit point.
// It is a no-op when rsiLogger is nil or a NoopRSILogger.
func (tl *ToolLoop) logRSIOutcome(agentID, model string, metrics *ToolLoopMetrics, toolNames []string, elapsed time.Duration) {
	if tl.rsiLogger == nil {
		return
	}
	outcome := RSIOutcome{
		Source:     "evoclaw",
		AgentID:    agentID,
		Model:      model,
		Success:    metrics.ErrorCount == 0,
		Quality:    DeriveQuality(metrics.ErrorCount, metrics.ToolCalls),
		DurationMs: elapsed.Milliseconds(),
		TaskType:   DeriveTaskType(toolNames),
		Notes:      fmt.Sprintf("%d tool calls, %d parallel batches", metrics.ToolCalls, metrics.ParallelBatches),
		Tags:       []string{"toolloop"},
	}
	if metrics.ParallelBatches > 0 {
		outcome.Tags = append(outcome.Tags, "parallel")
	}
	if err := tl.rsiLogger.LogOutcome(context.Background(), outcome); err != nil {
		tl.logger.Warn("failed to log RSI outcome", "error", err)
	}
}

// Execute runs the tool loop for a message
func (tl *ToolLoop) Execute(agent *AgentState, msg Message, model string) (*Response, *ToolLoopMetrics, error) {
	startTime := time.Now()
	metrics := &ToolLoopMetrics{}
	var allToolNames []string

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
	var finalContent string // Tracks the final text response
	needsSummary := false   // True when loop ended after tool results (needs summarisation)

	// Tool loop
	for iteration := 0; iteration < tl.maxIterations; iteration++ {
		metrics.TotalIterations++

		// Call LLM
		llmResp, toolCalls, err := tl.callLLM(messages, tools, model, agent.Def.SystemPrompt)
		if err != nil {
			tl.logRSIOutcome(agent.ID, model, metrics, allToolNames, time.Since(startTime))
			return nil, nil, fmt.Errorf("call LLM (iteration %d): %w", iteration, err)
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

		// If no tool calls, the LLM produced its final answer — use it directly
		if len(toolCalls) == 0 {
			finalContent = llmResp.Content
			tl.logger.Info("tool loop complete", "iterations", iteration+1)
			break
		}

		// Collect tool names for RSI outcome
		for _, c := range toolCalls {
			allToolNames = append(allToolNames, c.Name)
		}

		// --- Parallel batch execution (Phase 2) ---
		batchStart := time.Now()
		batchResults := tl.executeParallel(tl.orchestrator.ctx, agent, toolCalls)
		batchWall := time.Since(batchStart)

		// Update parallel-specific metrics for multi-call batches
		if len(toolCalls) > 1 {
			metrics.ParallelBatches++
			if len(toolCalls) > metrics.MaxConcurrency {
				metrics.MaxConcurrency = len(toolCalls)
			}

			// WallTimeSavedMs = sum of individual elapsed times − actual wall time
			var sumElapsed int64
			for _, r := range batchResults {
				if r.Result != nil {
					sumElapsed += r.Result.ElapsedMs
				}
			}
			saved := sumElapsed - batchWall.Milliseconds()
			if saved > 0 {
				metrics.WallTimeSavedMs += saved
			}
		}

		// Fan-in results in original index order
		batchAllFailed := true
		for _, pr := range batchResults {
			metrics.ToolCalls++

			if pr.Err != nil {
				metrics.ErrorCount++

				// Add error as tool result
				errorMsg := fmt.Sprintf("Error executing %s: %v", pr.Call.Name, pr.Err)
				messages = append(messages, ChatMessage{
					Role:       "tool",
					ToolCallID: pr.Call.ID,
					Content:    errorMsg,
				})
				continue
			}

			batchAllFailed = false

			if pr.Result.Status == "success" {
				metrics.SuccessCount++
			} else {
				metrics.ErrorCount++
				if pr.Result.ErrorType == "timeout" {
					metrics.TimeoutCount++
				}
			}

			// Add tool result to conversation
			toolMsg := tl.formatToolResult(pr.Call, pr.Result)
			messages = append(messages, toolMsg)
		}

		// Consecutive error tracking: only count a batch as a consecutive error
		// when every call in the batch failed.
		if batchAllFailed {
			consecutiveErrors++
			if consecutiveErrors >= tl.errorLimit {
				tl.logRSIOutcome(agent.ID, model, metrics, allToolNames, time.Since(startTime))
				return nil, nil, fmt.Errorf("too many consecutive errors (%d)", consecutiveErrors)
			}
		} else {
			consecutiveErrors = 0
		}

		// If we've hit max iterations, we need a summary call
		if iteration == tl.maxIterations-1 {
			needsSummary = true
		}
	}

	metrics.TotalDuration = time.Since(startTime)

	// Only make a final LLM call if:
	// 1. The loop hit max iterations (last messages are tool results, not a text answer)
	// 2. finalContent is empty (the LLM never produced a text-only response)
	if needsSummary || finalContent == "" {
		tl.logger.Info("making summary LLM call", "reason_max_iter", needsSummary, "empty_content", finalContent == "")
		summaryResp, _, err := tl.callLLM(messages, tools, model, agent.Def.SystemPrompt)
		if err != nil {
			tl.logRSIOutcome(agent.ID, model, metrics, allToolNames, time.Since(startTime))
			return nil, metrics, fmt.Errorf("summary LLM call: %w", err)
		}
		finalContent = summaryResp.Content
	}

	tl.logRSIOutcome(agent.ID, model, metrics, allToolNames, time.Since(startTime))
	return &Response{
		AgentID:   agent.ID,
		Content:   finalContent,
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

	// Security policy check: validate tool call before execution
	if policy := tl.orchestrator.securityPolicy; policy != nil {
		action := tl.buildSecurityAction(toolCall)
		if allowed, reason := policy.IsAllowed(action); !allowed {
			return &ToolResult{
				Tool:      toolCall.Name,
				Status:    "error",
				Error:     fmt.Sprintf("blocked by security policy: %s", reason),
				ErrorType: "security_policy",
				ElapsedMs: time.Since(start).Milliseconds(),
			}, nil
		}
	}

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

	// Record in RSI loop
	if tl.orchestrator.rsiLoop != nil {
		rsiResult := &rsiPkg.ToolResult{
			Tool:      toolCall.Name,
			Status:    result.Status,
			Result:    result.Result,
			Error:     result.Error,
			ExitCode:  result.ExitCode,
			ElapsedMs: result.ElapsedMs,
		}
		tl.orchestrator.rsiLoop.Observer().RecordToolCall(toolCall.Name, rsiResult, time.Since(start))
	}

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

// buildSecurityAction maps a ToolCall to a security.Action for policy checks.
func (tl *ToolLoop) buildSecurityAction(tc ToolCall) security.Action {
	action := security.Action{Tool: tc.Name}

	switch tc.Name {
	case "read_file", "list_files":
		action.Type = "read"
		if p, ok := tc.Arguments["path"].(string); ok {
			action.Path = p
		}
	case "write_file", "edit_file":
		action.Type = "write"
		if p, ok := tc.Arguments["path"].(string); ok {
			action.Path = p
		}
	case "delete_file":
		action.Type = "delete"
		if p, ok := tc.Arguments["path"].(string); ok {
			action.Path = p
		}
	case "execute", "shell":
		action.Type = "execute"
		if c, ok := tc.Arguments["command"].(string); ok {
			action.Command = c
		}
	default:
		action.Type = "execute"
	}

	return action
}
