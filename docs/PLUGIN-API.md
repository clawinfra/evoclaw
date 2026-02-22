# Plugin API

> Event-driven tool registration for EvoClaw agents — inspired by pi's Extension API.

---

## Overview

EvoClaw's plugin system allows skills and extensions to hook into the agent's tool loop at defined points. Rather than modifying agent internals, plugins register tools and subscribe to events.

This design is directly inspired by [pi's Extension API](https://github.com/badlogic/pi-mono), adapted from TypeScript to Go for the orchestrator and kept subprocess-based for the Rust edge agent.

---

## Go Orchestrator API

### Registering Tools

```go
// Tool defines a custom tool that plugins can register.
type Tool struct {
    Name        string
    Description string
    Args        []ArgDef
    Execute     func(ctx context.Context, args map[string]string) (*ToolResult, error)
}

// RegisterTool adds a custom tool to the agent's tool loop.
// The tool becomes available for LLM tool-use calls.
func (a *Agent) RegisterTool(tool Tool) error
```

**Example:**
```go
agent.RegisterTool(skills.Tool{
    Name:        "check_price",
    Description: "Check the current price of a cryptocurrency",
    Args: []skills.ArgDef{
        {Name: "symbol", Description: "Trading pair (e.g. BTC-USD)", Required: true},
    },
    Execute: func(ctx context.Context, args map[string]string) (*skills.ToolResult, error) {
        price, err := fetchPrice(args["symbol"])
        if err != nil {
            return nil, err
        }
        return &skills.ToolResult{Stdout: fmt.Sprintf("%.2f", price)}, nil
    },
})
```

### Event System

```go
// On registers an event handler for the named event.
func (a *Agent) On(event string, handler func(ctx context.Context, ev Event) error)
```

**Available events:**

| Event          | Fired When                              | Event Data                         |
|----------------|------------------------------------------|------------------------------------|
| `turn_start`   | Agent begins processing a user message   | `UserMessage`, `TurnID`            |
| `tool_call`    | LLM requests a tool invocation           | `ToolName`, `Args`, `CallID`       |
| `tool_result`  | Tool execution completes                 | `ToolName`, `Result`, `Duration`   |
| `message`      | Agent produces a response message        | `Content`, `Role`, `TokensUsed`    |
| `turn_end`     | Agent finishes a complete turn           | `TurnID`, `TotalTokens`, `Cost`    |
| `compaction`   | Context is being compacted               | `BeforeTokens`, `AfterTokens`      |

**Example:**
```go
agent.On("tool_call", func(ctx context.Context, ev skills.Event) error {
    log.Printf("Tool called: %s with args %v", ev.ToolName, ev.Args)
    
    // Optionally intercept — return error to block the call
    if ev.ToolName == "dangerous_tool" && !isApproved(ctx) {
        return fmt.Errorf("tool %s requires approval", ev.ToolName)
    }
    return nil
})

agent.On("turn_end", func(ctx context.Context, ev skills.Event) error {
    metrics.RecordTurn(ev.TurnID, ev.TotalTokens, ev.Cost)
    return nil
})
```

### Middleware Pattern

Events can be chained. Handlers run in registration order. If a `tool_call` handler returns an error, the tool invocation is blocked and the error is returned to the LLM.

```go
// Audit logger — runs on every tool call
agent.On("tool_call", auditLogger)

// Rate limiter — blocks excessive calls
agent.On("tool_call", rateLimiter)

// Cost tracker — records spend on turn end
agent.On("turn_end", costTracker)
```

---

## Edge Agent (Rust) — Subprocess Model

The Rust edge agent uses the same concepts but via subprocess execution rather than in-process registration:

```rust
// Tools are defined in agent.toml and loaded at startup
// Each tool maps to a subprocess command
[tools.check_price]
command = "~/.evoclaw/skills/price-check/scripts/run.sh"
description = "Check cryptocurrency price"
args = ["$SYMBOL"]
timeout_secs = 30
```

Events on the edge agent are handled internally by the agent loop. Plugins don't register event handlers directly — instead, they observe events via the MQTT status topic:

```
evoclaw/agents/{id}/events → {"event": "tool_call", "tool": "check_price", ...}
```

---

## Comparison: Pi vs EvoClaw Plugin API

For developers coming from pi's TypeScript Extension API:

| Pi (TypeScript)                          | EvoClaw (Go)                              |
|------------------------------------------|-------------------------------------------|
| `registerTool({name, desc, execute})`    | `agent.RegisterTool(Tool{...})`           |
| `on("tool_call", handler)`              | `agent.On("tool_call", handler)`          |
| `on("tool_result", handler)`            | `agent.On("tool_result", handler)`        |
| Extension loaded via `package.json`      | Plugin loaded via skill `agent.toml`      |
| Runs in-process (Node.js)               | Go: in-process / Rust: subprocess         |
| `context.abort()`                        | Return `error` from handler               |

The mental model is the same: register tools, subscribe to events, compose behavior. The implementation differs because EvoClaw runs on constrained hardware where subprocess isolation matters more than in-process convenience.

---

## Writing a Plugin

### 1. Create the Skill Directory

```bash
mkdir -p ~/.evoclaw/skills/my-plugin/scripts
```

### 2. Write `SKILL.md`

```yaml
---
name: my-plugin
version: 1.0.0
description: Example plugin with custom tool
author: you
metadata:
  evoclaw:
    permissions: ["internet"]
---

# My Plugin

Provides a custom tool for the agent.
```

### 3. Write `agent.toml`

```toml
[tools.my_tool]
command = "~/.evoclaw/skills/my-plugin/scripts/run.sh"
description = "Does something useful"
args = ["$INPUT"]
timeout_secs = 30
```

### 4. Implement the Script

```bash
#!/bin/bash
# scripts/run.sh
echo "Processed: $1"
```

### 5. Install

```bash
evoclaw skill add ./my-plugin
```

The agent picks up the new tool on next startup (or live-reload if supported).

---

*Compose behavior. Don't fork agents.* 🔌

---

## Trait-Driven Interfaces

EvoClaw defines formal Go interfaces for all core subsystems in `internal/interfaces/`. These make every subsystem swappable via configuration.

### Provider (`interfaces.Provider`)

Implement this to add a new LLM backend:

```go
import iface "github.com/clawinfra/evoclaw/internal/interfaces"

type MyProvider struct{}

func (p *MyProvider) Name() string                                               { return "my-llm" }
func (p *MyProvider) Chat(ctx context.Context, req iface.ChatRequest) (*iface.ChatResponse, error) { /* ... */ }
func (p *MyProvider) Models() []string                                           { return []string{"my-model-v1"} }
func (p *MyProvider) HealthCheck(ctx context.Context) error                      { return nil }
```

### MemoryBackend (`interfaces.MemoryBackend`)

Implement this to add a custom memory store (vector DB, Redis, etc.):

```go
type MyMemory struct{}

func (m *MyMemory) Store(ctx context.Context, key string, content []byte, metadata map[string]string) error { /* ... */ }
func (m *MyMemory) Retrieve(ctx context.Context, query string, limit int) ([]iface.MemoryEntry, error)      { /* ... */ }
func (m *MyMemory) Delete(ctx context.Context, key string) error                                              { /* ... */ }
func (m *MyMemory) HealthCheck(ctx context.Context) error                                                     { return nil }
```

### Tool (`interfaces.Tool`) and ToolRegistry

Implement `Tool` for custom tools. Use `ToolRegistry` to manage collections:

```go
type MyTool struct{}

func (t *MyTool) Name() string                                                          { return "my-tool" }
func (t *MyTool) Description() string                                                   { return "Does something" }
func (t *MyTool) Execute(ctx context.Context, params map[string]interface{}) (*iface.ToolResult, error) { /* ... */ }
func (t *MyTool) Schema() iface.ToolSchema                                              { return iface.ToolSchema{Name: "my-tool"} }
```

### Channel (`interfaces.Channel`)

Implement this to add a messaging transport (Slack, Discord, etc.):

```go
type MyChannel struct{}

func (c *MyChannel) Name() string                                                    { return "slack" }
func (c *MyChannel) Send(ctx context.Context, msg iface.OutboundMessage) error       { /* ... */ }
func (c *MyChannel) Receive(ctx context.Context) (<-chan iface.InboundMessage, error) { /* ... */ }
func (c *MyChannel) Close() error                                                     { return nil }
```

### Observer (`interfaces.Observer`)

Implement this for custom telemetry/monitoring:

```go
type MyObserver struct{}

func (o *MyObserver) OnRequest(ctx context.Context, req iface.ObservedRequest)   { /* log/emit */ }
func (o *MyObserver) OnResponse(ctx context.Context, resp iface.ObservedResponse) { /* log/emit */ }
func (o *MyObserver) OnError(ctx context.Context, err iface.ObservedError)       { /* log/emit */ }
func (o *MyObserver) Flush() error                                                { return nil }
```

### Shared Types

All shared types (`ChatRequest`, `ChatResponse`, `MemoryEntry`, `OutboundMessage`, etc.) are defined in `internal/interfaces/types.go`. Import `github.com/clawinfra/evoclaw/internal/interfaces` to use them.
