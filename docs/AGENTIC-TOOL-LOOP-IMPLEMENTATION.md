# Agentic Tool Loop - Implementation Plan

> **Status:** Ready for Development  
> **Version:** 1.0  
> **Last Updated:** 2025-02-15

## Overview

This document details the code changes required to implement the agentic tool loop for EvoClaw orchestrator and edge agents.

---

## Table of Contents

1. [New Files](#new-files)
2. [Modified Files](#modified-files)
3. [Data Structures](#data-structures)
4. [API Changes](#api-changes)
5. [Testing Strategy](#testing-strategy)
6. [Deployment Checklist](#deployment-checklist)

---

## New Files

### 1. `internal/orchestrator/tools.go`

**Purpose:** Tool schema generation and management

**Exports:**
```go
package orchestrator

// ToolDefinition represents a tool from skill.toml
type ToolDefinition struct {
    Name        string            `toml:"name"`
    Binary      string            `toml:"binary"`
    Description string            `toml:"description"`
    Parameters  ToolParameters    `toml:"parameters"`
    Sandbox     string            `toml:"sandboxing"`
    Timeout     int               `toml:"timeout_ms"`
    Permissions []string          `toml:"permissions"`
    Metadata    map[string]string `toml:"metadata"`
}

// ToolSchema represents an LLM-compatible tool schema
type ToolSchema struct {
    Name        string                 `json:"name"`
    Description string                 `json:"description"`
    Parameters  map[string]interface{} `json:"parameters"`
    EvoClawMeta ToolMetadata           `json:"evoclaw,omitempty"`
}

// ToolMetadata contains EvoClaw-specific extensions
type ToolMetadata struct {
    Binary      string   `json:"binary"`
    Timeout     int      `json:"timeout_ms"`
    Sandbox     bool     `json:"sandbox"`
    Permissions []string `json:"permissions"`
    Version     string   `json:"version"`
    Skill       string   `json:"skill"`
}

// ToolParameters defines parameter schema
type ToolParameters struct {
    Properties map[string]ParameterDef `toml:"properties"`
    Required   []string                `toml:"required"`
}

// ParameterDef defines a single parameter
type ParameterDef struct {
    Type        string `toml:"type"`
    Description string `toml:"description"`
    Default     any    `toml:"default,omitempty"`
}

// ToolManager handles tool schema generation
type ToolManager struct {
    skillsPath   string
    capabilities []string
    logger       *slog.Logger
    cache        map[string][]ToolSchema
    mu           sync.RWMutex
}

// NewToolManager creates a new tool manager
func NewToolManager(skillsPath string, capabilities []string, logger *slog.Logger) *ToolManager

// GenerateSchemas generates LLM tool schemas for all available tools
func (tm *ToolManager) GenerateSchemas() ([]ToolSchema, error)

// LoadSkillDefinitions loads tool definitions from skill.toml
func (tm *ToolManager) LoadSkillDefinitions(skillPath string) ([]ToolDefinition, error)

// DefinitionToSchema converts a tool definition to LLM schema
func (tm *ToolManager) DefinitionToSchema(def ToolDefinition) (ToolSchema, error)

// FilterByCapabilities filters tools by agent capabilities
func (tm *ToolManager) FilterByCapabilities(tools []ToolDefinition) []ToolDefinition

// GetToolTimeout returns the default timeout for a tool
func (tm *ToolManager) GetToolTimeout(toolName string) time.Duration
```

**Implementation Outline:**

```go
package orchestrator

import (
    "embed"
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "sync"
    "time"

    "github.com/BurntSushi/toml"
    "log/slog"
)

// NewToolManager creates a new tool manager
func NewToolManager(skillsPath string, capabilities []string, logger *slog.Logger) *ToolManager {
    if skillsPath == "" {
        // Default to ~/.evoclaw/skills
        home, _ := os.UserHomeDir()
        skillsPath = filepath.Join(home, ".evoclaw", "skills")
    }
    
    return &ToolManager{
        skillsPath:   skillsPath,
        capabilities: capabilities,
        logger:       logger.With("component", "tool_manager"),
        cache:        make(map[string][]ToolSchema),
    }
}

// GenerateSchemas generates LLM tool schemas for all available tools
func (tm *ToolManager) GenerateSchemas() ([]ToolSchema, error) {
    tm.mu.RLock()
    if cached, ok := tm.cache["all"]; ok {
        tm.mu.RUnlock()
        return cached, nil
    }
    tm.mu.RUnlock()

    tm.mu.Lock()
    defer tm.mu.Unlock()

    // Discover all skills
    skillDirs, err := os.ReadDir(tm.skillsPath)
    if err != nil {
        return nil, fmt.Errorf("read skills directory: %w", err)
    }

    var allTools []ToolDefinition
    for _, skillDir := range skillDirs {
        if !skillDir.IsDir() {
            continue
        }

        skillPath := filepath.Join(tm.skillsPath, skillDir.Name())
        tomlPath := filepath.Join(skillPath, "skill.toml")

        // Load skill.toml
        tools, err := tm.LoadSkillDefinitions(tomlPath)
        if err != nil {
            tm.logger.Warn("failed to load skill", "skill", skillDir.Name(), "error", err)
            continue
        }

        allTools = append(allTools, tools...)
    }

    // Filter by capabilities
    filtered := tm.FilterByCapabilities(allTools)

    // Convert to schemas
    schemas := make([]ToolSchema, 0, len(filtered))
    for _, tool := range filtered {
        schema, err := tm.DefinitionToSchema(tool)
        if err != nil {
            tm.logger.Warn("failed to convert tool to schema", "tool", tool.Name, "error", err)
            continue
        }
        schemas = append(schemas, schema)
    }

    // Cache results
    tm.cache["all"] = schemas

    tm.logger.Info("generated tool schemas", "count", len(schemas))
    return schemas, nil
}

// LoadSkillDefinitions loads tool definitions from skill.toml
func (tm *ToolManager) LoadSkillDefinitions(tomlPath string) ([]ToolDefinition, error) {
    data, err := os.ReadFile(tomlPath)
    if err != nil {
        return nil, fmt.Errorf("read skill.toml: %w", err)
    }

    var skillConfig struct {
        Tools []ToolDefinition `toml:"tools"`
    }

    if err := toml.Unmarshal(data, &skillConfig); err != nil {
        return nil, fmt.Errorf("parse skill.toml: %w", err)
    }

    return skillConfig.Tools, nil
}

// DefinitionToSchema converts a tool definition to LLM schema
func (tm *ToolManager) DefinitionToSchema(def ToolDefinition) (ToolSchema, error) {
    schema := ToolSchema{
        Name:        def.Name,
        Description: def.Description,
        Parameters: map[string]interface{}{
            "type":       "object",
            "properties": make(map[string]interface{}),
        },
        EvoClawMeta: ToolMetadata{
            Binary:      def.Binary,
            Timeout:     def.Timeout,
            Permissions: def.Permissions,
            Version:     def.Metadata["version"],
            Skill:       def.Metadata["skill"],
        },
    }

    // Convert parameters to JSON Schema format
    properties := make(map[string]interface{})
    for name, param := range def.Parameters.Properties {
        properties[name] = map[string]interface{}{
            "type":        param.Type,
            "description": param.Description,
        }
        if param.Default != nil {
            properties[name].(map[string]interface{})["default"] = param.Default
        }
    }

    schema.Parameters["properties"] = properties

    if len(def.Parameters.Required) > 0 {
        schema.Parameters["required"] = def.Parameters.Required
    }

    return schema, nil
}

// FilterByCapabilities filters tools by agent capabilities
func (tm *ToolManager) FilterByCapabilities(tools []ToolDefinition) []ToolDefinition {
    var filtered []ToolDefinition

    for _, tool := range tools {
        if tm.toolAllowed(tool) {
            filtered = append(filtered, tool)
        }
    }

    return filtered
}

// toolAllowed checks if a tool is allowed based on agent capabilities
func (tm *ToolManager) toolAllowed(tool ToolDefinition) bool {
    // If no capabilities specified, allow all
    if len(tm.capabilities) == 0 {
        return true
    }

    // Check if tool requires any capability that agent has
    for _, perm := range tool.Permissions {
        for _, cap := range tm.capabilities {
            if perm == cap {
                return true
            }
        }
    }

    // If tool has no permissions, allow it
    if len(tool.Permissions) == 0 {
        return true
    }

    return false
}

// GetToolTimeout returns the default timeout for a tool
func (tm *ToolManager) GetToolTimeout(toolName string) time.Duration {
    // Default timeouts by tool category
    defaults := map[string]time.Duration{
        "read":    5 * time.Second,
        "write":   5 * time.Second,
        "edit":    5 * time.Second,
        "glob":    5 * time.Second,
        "grep":    5 * time.Second,
        "bash":    30 * time.Second,
        "websearch": 30 * time.Second,
        "webfetch":   30 * time.Second,
        "codesearch": 30 * time.Second,
        "question":   10 * time.Second,
        "git_status": 60 * time.Second,
        "git_diff":   60 * time.Second,
        "git_commit": 60 * time.Second,
        "git_log":    60 * time.Second,
        "git_branch": 60 * time.Second,
    }

    if timeout, ok := defaults[toolName]; ok {
        return timeout
    }

    return 10 * time.Second // Default fallback
}
```

---

### 2. `internal/orchestrator/tooloop.go`

**Purpose:** Multi-turn tool execution loop

**Exports:**
```go
package orchestrator

// ToolLoop manages the multi-turn tool execution loop
type ToolLoop struct {
    orchestrator *Orchestrator
    toolManager  *ToolManager
    logger       *slog.Logger
    
    // Config
    maxIterations int
    errorLimit    int
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
    Tool      string        `json:"tool"`
    Status    string        `json:"status"` // "success", "error"
    Result    string        `json:"result,omitempty"`
    Error     string        `json:"error,omitempty"`
    ErrorType string        `json:"error_type,omitempty"`
    ElapsedMs int64         `json:"elapsed_ms"`
    ExitCode  int           `json:"exit_code,omitempty"`
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
func NewToolLoop(orch *Orchestrator, tm *ToolManager) *ToolLoop

// Execute runs the tool loop for a message
func (tl *ToolLoop) Execute(agent *AgentState, msg Message, model string) (*Response, *ToolLoopMetrics, error)

// callLLM calls the LLM with conversation history and tools
func (tl *ToolLoop) callLLM(messages []ChatMessage, tools []ToolSchema, model string) (*ChatResponse, []ToolCall, error)

// executeToolCall executes a single tool call
func (tl *ToolLoop) executeToolCall(agent *AgentState, toolCall ToolCall) (*ToolResult, error)

// waitForToolResult waits for a tool result from the edge agent
func (tl *ToolLoop) waitForToolResult(requestID string, timeout time.Duration) (*ToolResult, error)

// formatToolResult formats a tool result as a tool message for the LLM
func (tl *ToolLoop) formatToolResult(toolCall ToolCall, result *ToolResult) ChatMessage

// shouldContinueLoop checks if the loop should continue
func (tl *ToolLoop) shouldContinueLoop(iteration int, consecutiveErrors int) bool
```

**Implementation Outline:**

```go
package orchestrator

import (
    "context"
    "encoding/json"
    "fmt"
    "time"
    
    "log/slog"
)

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
    
    // Initialize conversation history
    messages := []ChatMessage{
        {Role: "user", Content: msg.Content},
    }
    
    consecutiveErrors := 0
    
    // Tool loop
    for iteration := 0; iteration < tl.maxIterations; iteration++ {
        metrics.TotalIterations++
        
        // Call LLM
        llmResp, toolCalls, err := tl.callLLM(messages, tools, model)
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
            toolCallsData, _ := json.Marshal(toolCalls)
            // Some LLMs expect tool_calls in a specific format
            assistantMsg.ToolCalls = toolCalls
        }
        
        messages = append(messages, assistantMsg)
        
        // If no tool calls, we're done
        if len(toolCalls) == 0 {
            tl.logger.Info("tool loop complete", "iterations", iteration+1)
            break
        }
        
        // Execute each tool call
        for _, toolCall := range toolCalls {
            metrics.ToolCalls++
            
            toolResult, err := tl.executeToolCall(agent, toolCall)
            if err != nil {
                consecutiveErrors++
                metrics.ErrorCount++
                
                // Add error as tool result
                errorMsg := fmt.Sprintf("Error executing %s: %v", toolCall.Name, err)
                messages = append(messages, ChatMessage{
                    Role:         "tool",
                    ToolCallID:   toolCall.ID,
                    Content:      errorMsg,
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
    finalResp, _, err := tl.callLLM(messages, tools, model)
    if err != nil {
        return nil, metrics, fmt.Errorf("final LLM call: %w", err)
    }
    
    return &Response{
        AgentID: agent.ID,
        Content: finalResp.Content,
        Channel: msg.Channel,
        To:      msg.From,
        ReplyTo: msg.ID,
        Model:   model,
    }, metrics, nil
}

// callLLM calls the LLM with conversation history and tools
func (tl *ToolLoop) callLLM(messages []ChatMessage, tools []ToolSchema, model string) (*ChatResponse, []ToolCall, error) {
    // Find provider
    provider := tl.orchestrator.findProvider(model)
    if provider == nil {
        return nil, nil, fmt.Errorf("no provider for model: %s", model)
    }
    
    // Prepare request with tools
    req := ChatRequest{
        Model:        model,
        SystemPrompt: "", // Use agent's system prompt
        Messages:     messages,
        MaxTokens:    4096,
        Temperature:  0.7,
        Tools:        tools, // NEW: Include tools
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
    
    // Send command via MQTT
    topic := fmt.Sprintf("evoclaw/agents/%s/commands", agent.ID)
    payload, _ := json.Marshal(cmd)
    
    // Get MQTT channel
    mqttChan, ok := tl.orchestrator.channels["mqtt"]
    if !ok {
        return nil, fmt.Errorf("MQTT channel not available")
    }
    
    // Publish command
    // (This requires adding a method to MQTTChannel to send raw commands)
    // For now, we'll use a special Response with tool command
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
    errChan := make(chan error, 1)
    
    // Register result handler (this requires adding a result registry to orchestrator)
    tl.orchestrator.RegisterResultHandler(requestID, func(result *ToolResult) {
        resultChan <- result
    })
    
    select {
    case result := <-resultChan:
        return result, nil
    case err := <-errChan:
        return nil, err
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
```

---

## Modified Files

### 1. `internal/orchestrator/orchestrator.go`

**Changes:**

1. **Add ToolManager to Orchestrator struct:**
```go
type Orchestrator struct {
    // ... existing fields ...
    
    // NEW: Tool management
    toolManager  *ToolManager
    toolLoop     *ToolLoop
    
    // NEW: Result registry for tool calls
    resultRegistry map[string]chan *ToolResult
    resultMu       sync.RWMutex
}
```

2. **Initialize ToolManager in New():**
```go
func New(cfg *config.Config, logger *slog.Logger) *Orchestrator {
    // ... existing code ...
    
    o := &Orchestrator{
        // ... existing fields ...
        resultRegistry: make(map[string]chan *ToolResult),
    }
    
    // Initialize tool manager if tools enabled
    if cfg.Tools.Enabled {
        o.toolManager = NewToolManager(
            cfg.Tools.SkillsPath,
            cfg.Agents[0].Capabilities, // Use first agent's capabilities
            logger,
        )
        o.toolLoop = NewToolLoop(o, o.toolManager)
    }
    
    return o
}
```

3. **Modify processWithAgent to use tool loop:**
```go
func (o *Orchestrator) processWithAgent(agent *AgentState, msg Message, model string) {
    start := time.Now()
    
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
    
    var response *Response
    var err error
    
    // NEW: Use tool loop if enabled
    if o.toolLoop != nil {
        response, _, err = o.toolLoop.Execute(agent, msg, model)
    } else {
        // Legacy: direct LLM call without tools
        response, err = o.processDirect(agent, msg, model)
    }
    
    if err != nil {
        o.logger.Error("processing failed", "error", err)
        agent.mu.Lock()
        agent.ErrorCount++
        agent.Metrics.FailedActions++
        agent.mu.Unlock()
        
        // Record failure in health registry
        if o.healthRegistry != nil {
            errType := router.ClassifyError(err)
            o.healthRegistry.RecordFailure(model, errType)
        }
        return
    }
    
    // ... rest of existing code (metrics, evolution, cloud sync, etc.) ...
}
```

4. **Add result handler registration:**
```go
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
        case <-time.After(5 * time.Second):
            o.logger.Warn("timeout delivering tool result", "request_id", requestID)
        }
    }
}
```

5. **Add legacy processDirect for non-tool mode:**
```go
// processDirect processes a message without tools (legacy mode)
func (o *Orchestrator) processDirect(agent *AgentState, msg Message, model string) (*Response, error) {
    req := ChatRequest{
        Model:        model,
        SystemPrompt: agent.Def.SystemPrompt,
        Messages: []ChatMessage{
            {Role: "user", Content: msg.Content},
        },
        MaxTokens:   4096,
        Temperature: 0.7,
    }
    
    provider := o.findProvider(model)
    if provider == nil {
        return nil, fmt.Errorf("no provider for model: %s", model)
    }
    
    resp, err := provider.Chat(o.ctx, req)
    if err != nil {
        return nil, err
    }
    
    return &Response{
        AgentID:   agent.ID,
        Content:   resp.Content,
        Channel:   msg.Channel,
        To:        msg.From,
        ReplyTo:   msg.ID,
        MessageID: msg.ID,
        Model:     model,
    }, nil
}
```

---

### 2. `internal/channels/mqtt.go`

**Changes:**

1. **Modify Send() to handle tool commands:**
```go
func (m *MQTTChannel) Send(ctx context.Context, msg orchestrator.Response) error {
    if !m.client.IsConnected() {
        return fmt.Errorf("mqtt not connected")
    }
    
    topic := fmt.Sprintf(commandsTopic, msg.To)
    
    // Check if this is a tool command
    if msg.Metadata != nil {
        if command, ok := msg.Metadata["command"]; ok && command == "tool" {
            // Parse tool command from msg.Content
            var toolCmd EdgeAgentCommand
            if err := json.Unmarshal([]byte(msg.Content), &toolCmd); err != nil {
                return fmt.Errorf("parse tool command: %w", err)
            }
            
            payload, err := json.Marshal(toolCmd)
            if err != nil {
                return fmt.Errorf("marshal tool command: %w", err)
            }
            
            token := m.client.Publish(topic, 1, false, payload)
            if !token.WaitTimeout(5 * time.Second) {
                return fmt.Errorf("publish timeout")
            }
            if err := token.Error(); err != nil {
                return fmt.Errorf("publish: %w", err)
            }
            
            m.logger.Debug("tool command sent", "topic", topic, "tool", toolCmd.Payload["tool"])
            return nil
        }
    }
    
    // Existing message handling...
    // ... rest of existing Send() code ...
}
```

2. **Modify handleMessage to route tool results:**
```go
func (m *MQTTChannel) handleMessage(client mqtt.Client, mqttMsg mqtt.Message) {
    m.wg.Add(1)
    defer m.wg.Done()
    
    m.logger.Debug("mqtt message received", "topic", mqttMsg.Topic())
    
    var payload map[string]interface{}
    if err := json.Unmarshal(mqttMsg.Payload(), &payload); err != nil {
        m.logger.Error("failed to parse mqtt message", "error", err)
        return
    }
    
    // Check if this is a tool result
    if status, ok := payload["status"].(string); ok && (status == "success" || status == "error") {
        if _, hasTool := payload["tool"]; hasTool {
            // This is a tool result
            m.handleToolResult(payload)
            return
        }
    }
    
    // Existing message handling...
    // ... rest of existing handleMessage() code ...
}

// handleToolResult processes a tool execution result
func (m *MQTTChannel) handleToolResult(payload map[string]interface{}) {
    requestID, _ := payload["request_id"].(string)
    
    result := &orchestrator.ToolResult{
        Tool:      payload["tool"].(string),
        Status:    payload["status"].(string),
        Result:    payload["result"].(string),
        Error:     payload["error"].(string),
        ErrorType: payload["error_type"].(string),
    }
    
    if elapsedMs, ok := payload["elapsed_ms"].(float64); ok {
        result.ElapsedMs = int64(elapsedMs)
    }
    
    if exitCode, ok := payload["exit_code"].(float64); ok {
        result.ExitCode = int(exitCode)
    }
    
    // Deliver to orchestrator's result registry
    // This requires access to orchestrator, which we need to add
    // For now, log it
    m.logger.Debug("tool result received", "request_id", requestID, "tool", result.Tool, "status", result.Status)
}
```

3. **Add reference to orchestrator in MQTTChannel:**
```go
type MQTTChannel struct {
    // ... existing fields ...
    
    // NEW: Reference to orchestrator for result delivery
    orchestrator *Orchestrator
}

func NewMQTT(broker string, port int, username, password string, logger *slog.Logger, orch *Orchestrator) *MQTTChannel {
    return &MQTTChannel{
        // ... existing fields ...
        orchestrator: orch,
    }
}
```

---

### 3. `edge-agent/src/commands.rs`

**Changes:**

1. **Add tool command handler:**
```rust
impl EdgeAgent {
    pub async fn handle_command(&mut self, cmd: AgentCommand) {
        info!(
            command = %cmd.command,
            request_id = %cmd.request_id,
            "received command"
        );

        let result = match cmd.command.as_str() {
            "ping" => self.handle_ping(&cmd).await,
            "execute" => self.handle_execute(&cmd).await,
            "tool" => self.handle_tool(&cmd).await,  // NEW
            "update_strategy" => self.handle_update_strategy(&cmd).await,
            "update_genome" => self.handle_update_genome(&cmd).await,
            "get_metrics" => self.handle_get_metrics(&cmd).await,
            "evolution" => self.handle_evolution(&cmd).await,
            "shutdown" => self.handle_shutdown(&cmd).await,
            _ => {
                warn!(command = %cmd.command, "unknown command");
                Err(format!("unknown command: {}", cmd.command).into())
            }
        };

        // Report result or error
        match result {
            Ok(response) => {
                self.metrics.record_success();
                let _ = self.mqtt.report("result", response).await;
            }
            Err(e) => {
                self.metrics.record_failure();
                let error_payload = serde_json::json!({
                    "error": e.to_string(),
                    "request_id": cmd.request_id
                });
                let _ = self.mqtt.report("error", error_payload).await;
            }
        }
    }

    // NEW: Handle tool execution command
    async fn handle_tool(&mut self, cmd: &AgentCommand) -> CommandResult {
        let tool_name = cmd.payload.get("tool")
            .and_then(|v| v.as_str())
            .ok_or("missing tool name")?;

        let parameters = cmd.payload.get("parameters")
            .and_then(|v| v.as_object())
            .ok_or("missing parameters")?
            .clone();

        let timeout_ms = cmd.payload.get("timeout_ms")
            .and_then(|v| as_u64(v))
            .unwrap_or(30000);

        info!(
            tool = %tool_name,
            timeout_ms,
            "executing tool"
        );

        let start = std::time::Instant::now();

        // Execute tool
        let result = self.execute_tool(tool_name, parameters, timeout_ms).await?;

        let elapsed = start.elapsed().as_millis();

        Ok(serde_json::json!({
            "status": "success",
            "tool": tool_name,
            "result": result.stdout,
            "stderr": result.stderr,
            "exit_code": result.exit_code,
            "elapsed_ms": elapsed,
            "request_id": cmd.request_id
        }))
    }

    // NEW: Execute a tool binary
    async fn execute_tool(
        &self,
        tool_name: &str,
        parameters: serde_json::Map<String, serde_json::Value>,
        timeout_ms: u64
    ) -> Result<ToolExecutionResult, Box<dyn std::error::Error>> {
        // Get skill directory
        let skill_dir = std::path::PathBuf::from("/home/pi/.evoclaw/skills/desktop-tools");
        let bin_path = skill_dir.join("bin").join(format!("dt-{}", tool_name));

        // Build command arguments from parameters
        let args = self.build_tool_args(tool_name, parameters)?;

        // Execute with timeout
        let output = tokio::time::timeout(
            std::time::Duration::from_millis(timeout_ms),
            tokio::process::Command::new(&bin_path)
                .args(&args)
                .output()
        ).await??;

        Ok(ToolExecutionResult {
            stdout: String::from_utf8_lossy(&output.stdout).to_string(),
            stderr: String::from_utf8_lossy(&output.stderr).to_string(),
            exit_code: output.status.code().unwrap_or(-1),
        })
    }

    // NEW: Build command-line arguments from tool parameters
    fn build_tool_args(
        &self,
        tool_name: &str,
        parameters: serde_json::Map<String, serde_json::Value>
    ) -> Result<Vec<String>, Box<dyn std::error::Error>> {
        let mut args = Vec::new();

        match tool_name {
            "read" => {
                if let Some(path) = parameters.get("path").and_then(|v| v.as_str()) {
                    args.push("--path".to_string());
                    args.push(path.to_string());
                }
                if let Some(offset) = parameters.get("offset").and_then(|v| v.as_i64()) {
                    args.push("--offset".to_string());
                    args.push(offset.to_string());
                }
                if let Some(limit) = parameters.get("limit").and_then(|v| v.as_i64()) {
                    args.push("--limit".to_string());
                    args.push(limit.to_string());
                }
            }
            "bash" => {
                if let Some(command) = parameters.get("command").and_then(|v| v.as_str()) {
                    args.push("-c".to_string());
                    args.push(command.to_string());
                }
            }
            "write" => {
                if let Some(path) = parameters.get("path").and_then(|v| v.as_str()) {
                    args.push("--path".to_string());
                    args.push(path.to_string());
                }
                if let Some(content) = parameters.get("content").and_then(|v| v.as_str()) {
                    args.push(content.to_string());
                }
            }
            _ => {
                // Generic parameter handling
                for (key, value) in parameters {
                    args.push(format!("--{}", key));
                    if let Some(s) = value.as_str() {
                        args.push(s.to_string());
                    } else if let Some(n) = value.as_i64() {
                        args.push(n.to_string());
                    } else if let Some(b) = value.as_bool() {
                        args.push(b.to_string());
                    }
                }
            }
        }

        Ok(args)
    }
}

// Helper to convert JSON Value to u64
fn as_u64(v: &serde_json::Value) -> Option<u64> {
    v.as_u64().or_else(|| v.as_i64().and_then(|n| if n >= 0 { Some(n as u64) } else { None }))
}

// Tool execution result
struct ToolExecutionResult {
    stdout: String,
    stderr: String,
    exit_code: i32,
}
```

---

### 4. `internal/config/config.go`

**Add tools configuration:**

```go
type Config struct {
    // ... existing fields ...
    
    Tools ToolsConfig `toml:"tools"`
}

type ToolsConfig struct {
    Enabled     bool     `toml:"enabled"`
    SkillsPath  string   `toml:"skills_path"`
    Sandboxing  bool     `toml:"enable_sandboxing"`
    SandboxEngine string `toml:"sandbox_engine"`
    Permissions  []string `toml:"permissions"`
}
```

---

## Data Structures

### Tool Call Flow

```
LLM Response
├── Content: "I'll check the file..."
└── ToolCalls: [
    {
        ID: "call_abc123",
        Name: "read",
        Arguments: {
            "path": "/etc/hostname"
        }
    }
]
         ↓
Orchestrator sends command via MQTT
         ↓
Edge Agent executes tool
         ↓
Edge Agent sends result via MQTT
         ↓
Orchestrator creates tool message:
{
    Role: "tool",
    ToolCallID: "call_abc123",
    Content: "raspberrypi\n"
}
         ↓
LLM generates final response
```

---

## API Changes

### ChatRequest (Modified)

```go
type ChatRequest struct {
    Model        string
    SystemPrompt string
    Messages     []ChatMessage
    MaxTokens    int
    Temperature  float64
    
    // NEW: Tools for function calling
    Tools []ToolSchema `json:"