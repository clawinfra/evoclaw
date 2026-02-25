# Agentic Tool Loop for EvoClaw Orchestrator

> **Status:** Design Document  
> **Version:** 1.0  
> **Last Updated:** 2025-02-15

## Overview

### Problem Statement

EvoClaw edge agents have access to 30 tools (desktop-tools skill) but the orchestrator currently just forwards LLM text responses. Edge agents cannot execute natural language - they need structured tool commands.

**Current Flow (broken):**
```
User → Orchestrator → LLM → text response → Edge Agent (can't execute)
```

**Required Flow:**
```
User → Orchestrator → LLM (with tools in prompt) → tool_call 
     → Edge Agent executes tool → result 
     → LLM (with result) → final response → User
```

### Motivation

1. **Tool Parity:** Edge agents should have full access to the same 30 tools as OpenClaw
2. **Structured Execution:** LLMs generate tool calls, edge agents execute them
3. **Multi-Turn Reasoning:** LLMs can chain multiple tools to solve complex tasks
4. **Error Recovery:** Failed tools can be retried or alternatives attempted
5. **Observability:** Tool usage, performance, and errors tracked for evolution

### Goals

- Enable LLM-driven tool invocation on edge agents
- Maintain backward compatibility with existing message flow
- Support multi-tool execution and parallel calls
- Provide robust error handling and timeouts
- Track tool metrics for evolution engine

---

## Architecture

### High-Level Diagram

```
┌─────────────────────────────────────────────────────────────────┐
│                         User Message                             │
│                    "Take a screenshot"                           │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                       Orchestrator                               │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  1. Message Router                                      │   │
│  │     - Selects agent (hash-based routing)                  │   │
│  │     - Routes to inbox                                     │   │
│  └────────────────────┬────────────────────────────────────┘   │
│                       │                                         │
│                       ▼                                         │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  2. Tool Schema Generator (NEW)                        │   │
│  │     - Converts skill.toml → LLM tool schemas             │   │
│  │     - Filters tools by agent capabilities                 │   │
│  │     - Adds tool metadata (timeout, sandboxing)           │   │
│  └────────────────────┬────────────────────────────────────┘   │
│                       │                                         │
│                       ▼                                         │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  3. Tool Loop (NEW)                                     │   │
│  │     ┌─────────────────────────────────────────┐         │   │
│  │     │ Loop (max_iterations = 10)              │         │   │
│  │     │   1. Call LLM with tools in prompt     │         │   │
│  │     │   2. Check if LLM wants to call tools   │         │   │
│  │     │   3. If yes:                             │         │   │
│  │     │      - Parse tool_call from response     │         │   │
│  │     │      - Send command to edge agent        │         │   │
│  │     │      - Wait for result (with timeout)    │         │   │
│  │     │      - Add result to conversation        │         │   │
│  │     │      - Loop back to step 1               │         │   │
│  │     │   4. If no tool call:                    │         │   │
│  │     │      - Break loop, return final response  │         │   │
│  │     └─────────────────────────────────────────┘         │   │
│  └────────────────────┬────────────────────────────────────┘   │
│                       │                                         │
│                       ▼                                         │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  4. Response Handler                                    │   │
│  │     - Sends final response to user                       │   │
│  │     - Updates metrics                                    │   │
│  └─────────────────────────────────────────────────────────┘   │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             │ MQTT: evoclaw/agents/agent-id/commands
                             │    {"command":"execute","payload":{...}}
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                      Edge Agent (Rust)                          │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  1. Command Receiver (commands.rs)                      │   │
│  │     - Parses command from orchestrator                   │   │
│  │     - Validates tool name and parameters                 │   │
│  └────────────────────┬────────────────────────────────────┘   │
│                       │                                         │
│                       ▼                                         │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  2. Tool Executor                                       │   │
│  │     - Executes tool binary (e.g., bin/dt-bash)           │   │
│  │     - Captures stdout/stderr                            │   │
│  │     - Enforces timeout                                   │   │
│  └────────────────────┬────────────────────────────────────┘   │
│                       │                                         │
│                       ▼                                         │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  3. Result Reporter                                      │   │
│  │     - Publishes result to MQTT                           │   │
│  │     - Topic: evoclaw/agents/agent-id/reports            │   │
│  └─────────────────────────────────────────────────────────┘   │
└────────────────────────────┬────────────────────────────────────┘
                             │
                             │ MQTT: evoclaw/agents/agent-id/reports
                             │    {"status":"success","result":"..."}
                             ▼
┌─────────────────────────────────────────────────────────────────┐
│                     Back to Orchestrator                         │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  5. Tool Result Handler (NEW)                          │   │
│  │     - Receives result from MQTT                         │   │
│  │     - Adds to conversation history                      │   │
│  │     - Continues tool loop                               │   │
│  └─────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

### Components

#### 1. Tool Schema Generator (`internal/orchestrator/tools.go`)

**Purpose:** Convert `skill.toml` tool definitions into LLM-compatible tool schemas

**Responsibilities:**
- Parse `skill.toml` files from `~/.evoclaw/skills/*/`
- Generate JSON Schema for each tool
- Filter tools by agent capabilities
- Add tool metadata (timeout, sandboxing, required permissions)

**Input (skill.toml):**
```toml
[[tools]]
name = "bash"
binary = "bin/dt-bash"
description = "Execute shell commands"
```

**Output (LLM tool schema):**
```json
{
  "name": "bash",
  "description": "Execute shell commands with sandboxing",
  "parameters": {
    "type": "object",
    "properties": {
      "command": {
        "type": "string",
        "description": "Shell command to execute"
      },
      "timeout_ms": {
        "type": "integer",
        "description": "Timeout in milliseconds (default: 30000)"
      }
    },
    "required": ["command"]
  }
}
```

#### 2. Tool Loop (`internal/orchestrator/orchestrator.go`)

**Purpose:** Multi-turn loop for LLM-driven tool invocation

**Flow:**
1. Generate tool schemas for available tools
2. Call LLM with user message + tool schemas
3. Check if LLM wants to call a tool
4. If yes:
   - Parse tool_call (tool name, arguments)
   - Send command to edge agent via MQTT
   - Wait for result (with timeout)
   - Add result to conversation history
   - Loop back to step 2
5. If no tool call:
   - Break loop
   - Return final response to user

#### 3. Edge Agent Command Handler (`edge-agent/src/commands.rs`)

**Current State:** Handles `execute`, `update_strategy`, `evolution` commands

**Changes Required:**
- Add `tool` command handler
- Parse tool name and parameters from payload
- Execute tool binary from skill directory
- Capture stdout/stderr
- Return structured result

---

## Message Flow

### Tool Invocation Flow

#### Step 1: User Sends Message

```
User: "Check the CPU temperature"
```

#### Step 2: Orchestrator Generates Tool Schemas

**Orchestrator → Tool Schema Generator:**
```go
tools := orchestrator.GenerateToolSchemas(agentID)
```

**Result:**
```json
[
  {
    "name": "bash",
    "description": "Execute shell commands",
    "parameters": {
      "type": "object",
      "properties": {
        "command": {"type": "string"}
      },
      "required": ["command"]
    }
  }
]
```

#### Step 3: LLM Decides to Call Tool

**Orchestrator → LLM:**
```json
{
  "model": "claude-sonnet-4",
  "messages": [
    {"role": "user", "content": "Check the CPU temperature"}
  ],
  "tools": [
    {
      "name": "bash",
      "description": "Execute shell commands",
      "parameters": {...}
    }
  ]
}
```

**LLM → Orchestrator:**
```json
{
  "content": "I'll check the CPU temperature for you.",
  "tool_calls": [
    {
      "id": "call_abc123",
      "name": "bash",
      "arguments": {
        "command": "vcgencmd measure_temp"
      }
    }
  ]
}
```

#### Step 4: Orchestrator Sends Tool Command to Edge Agent

**Orchestrator → Edge Agent (MQTT):**

**Topic:** `evoclaw/agents/living-room-pi/commands`

**Payload:**
```json
{
  "command": "tool",
  "payload": {
    "tool": "bash",
    "parameters": {
      "command": "vcgencmd measure_temp"
    },
    "timeout_ms": 30000
  },
  "request_id": "tool-1739635200000"
}
```

#### Step 5: Edge Agent Executes Tool

**Edge Agent:**
```rust
match cmd.command.as_str() {
    "tool" => {
        let tool_name = cmd.payload["tool"].as_str().unwrap();
        let params = cmd.payload["parameters"].clone();
        
        let result = execute_tool(tool_name, params).await?;
        
        mqtt.report("result", json!({
            "status": "success",
            "tool": tool_name,
            "result": result.stdout,
            "stderr": result.stderr,
            "exit_code": result.exit_code
        })).await;
    }
    _ => { /* ... */ }
}
```

#### Step 6: Edge Agent Returns Result

**Edge Agent → Orchestrator (MQTT):**

**Topic:** `evoclaw/agents/living-room-pi/reports`

**Payload:**
```json
{
  "status": "success",
  "tool": "bash",
  "result": "temp=45.6°C",
  "stderr": "",
  "exit_code": 0,
  "elapsed_ms": 123
}
```

#### Step 7: Orchestrator Feeds Result Back to LLM

**Orchestrator → LLM:**
```json
{
  "model": "claude-sonnet-4",
  "messages": [
    {"role": "user", "content": "Check the CPU temperature"},
    {
      "role": "assistant", 
      "content": "I'll check the CPU temperature for you.",
      "tool_calls": [...]
    },
    {
      "role": "tool",
      "tool_call_id": "call_abc123",
      "content": "temp=45.6°C"
    }
  ],
  "tools": [...]
}
```

#### Step 8: LLM Generates Final Response

**LLM → Orchestrator:**
```json
{
  "content": "The CPU temperature is 45.6°C, which is within normal operating range."
}
```

#### Step 9: Orchestrator Sends Response to User

```
User: "The CPU temperature is 45.6°C, which is within normal operating range."
```

---

## Tool Schema Format

### LLM Tool Schema (OpenAI-Compatible)

```json
{
  "name": "tool_name",
  "description": "Human-readable description of what the tool does",
  "parameters": {
    "type": "object",
    "properties": {
      "param1": {
        "type": "string",
        "description": "Description of parameter"
      },
      "param2": {
        "type": "integer",
        "description": "Another parameter"
      }
    },
    "required": ["param1"]
  }
}
```

### Tool Metadata (EvoClaw Extensions)

In addition to standard LLM tool schema, EvoClaw adds metadata:

```json
{
  "name": "bash",
  "description": "Execute shell commands",
  "parameters": {...},
  "evoclaw": {
    "binary": "bin/dt-bash",
    "timeout_ms": 30000,
    "sandbox": true,
    "permissions": ["shell", "network"],
    "version": "2.0.0",
    "skill": "desktop-tools"
  }
}
```

### Tool Categories

#### File Operations
- `read` - Read file contents
- `write` - Create/overwrite files
- `edit` - String replacements
- `glob` - Find files by pattern
- `grep` - Search file contents

#### Web Access
- `websearch` - Brave API search
- `webfetch` - Fetch and extract URLs
- `codesearch` - Search programming docs

#### Execution
- `bash` - Execute shell commands (sandboxed)

#### Interaction
- `question` - Ask user for input

#### Project Management
- `todowrite` - Add tasks
- `todoread` - Read/filter tasks
- `task` - Launch sub-agents
- `skill` - Load workflows

#### Sessions
- `sessions_list` - List sessions
- `sessions_history` - Get history
- `sessions_send` - Send message
- `sessions_spawn` - Spawn subagent
- `session_status` - Get status
- `agents_list` - List agents

#### Git
- `git_status` - Git status
- `git_diff` - Show diff
- `git_commit` - Commit changes
- `git_log` - Show history
- `git_branch` - List branches

#### Advanced
- `apply_patch` - Apply unified diff
- `browser` - Browser automation
- `canvas` - UI presentation
- `nodes` - Device management
- `image` - Vision analysis

---

## Command Format to Edge Agents

### Tool Command Schema

**Topic:** `evoclaw/agents/{agent-id}/commands`

```json
{
  "command": "tool",
  "payload": {
    "tool": "bash",
    "parameters": {
      "command": "ls -la"
    },
    "timeout_ms": 30000,
    "request_id": "req-1739635200000000000"
  },
  "request_id": "req-1739635200000000000"
}
```

### Response Schema

**Topic:** `evoclaw/agents/{agent-id}/reports`

**Success Response:**
```json
{
  "status": "success",
  "tool": "bash",
  "result": "file1.txt\nfile2.txt\n",
  "stderr": "",
  "exit_code": 0,
  "elapsed_ms": 123
}
```

**Error Response:**
```json
{
  "status": "error",
  "tool": "bash",
  "error": "Command timed out after 30000ms",
  "error_type": "timeout"
}
```

### Error Types

- `timeout` - Tool execution exceeded timeout
- `not_found` - Tool binary not found
- `permission_denied` - Insufficient permissions
- `invalid_params` - Invalid tool parameters
- `execution_failed` - Tool returned non-zero exit code

---

## Result Handling and Multi-Turn Loops

### Tool Loop State Machine

```
┌──────────────────┐
│  Start Loop      │
│  (max_iter=10)   │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ Call LLM with    │
│ tools + history  │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐      ┌─────────────────────┐
│ Tool call?       │──No──▶│ Break loop          │
└────────┬─────────┘      └─────────────────────┘
         │ Yes
         ▼
┌──────────────────┐
│ Parse tool_call │
│ (name, args)    │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐      ┌─────────────────────┐
│ Validate tool?   │──No──▶│ Return error to LLM │
└────────┬─────────┘      │ Continue loop       │
         │ Yes             └─────────────────────┘
         ▼
┌──────────────────┐
│ Send command to  │
│ edge agent       │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐      ┌─────────────────────┐
│ Wait for result  │──X───▶│ Timeout → Error LLM │
│ (with timeout)   │      │ Continue loop       │
└────────┬─────────┘      └─────────────────────┘
         │ Result
         ▼
┌──────────────────┐
│ Add result to    │
│ conversation as  │
│ tool message     │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐
│ Increment iter   │
│ iter++           │
└────────┬─────────┘
         │
         ▼
┌──────────────────┐      ┌─────────────────────┐
│ iter < max?      │──No──▶│ Break loop          │
└────────┬─────────┘      └─────────────────────┘
         │ Yes
         └──────▶ Loop back to LLM call
```

### Conversation History Format

```json
[
  {"role": "user", "content": "Check CPU temp"},
  {
    "role": "assistant",
    "content": "I'll check the CPU temperature.",
    "tool_calls": [
      {
        "id": "call_abc123",
        "name": "bash",
        "arguments": {"command": "vcgencmd measure_temp"}
      }
    ]
  },
  {
    "role": "tool",
    "tool_call_id": "call_abc123",
    "content": "temp=45.6°C"
  },
  {"role": "assistant", "content": "The CPU is at 45.6°C."}
]
```

### Loop Termination Conditions

1. **No tool call** - LLM responds with text only
2. **Max iterations** - Reached 10 iterations (configurable)
3. **Error limit** - 3 consecutive tool errors (configurable)
4. **User cancellation** - Context cancelled

---

## Error Handling and Timeouts

### Timeout Strategy

| Tool Type | Default Timeout | Max Timeout |
|-----------|-----------------|-------------|
| File ops (read, write, edit) | 5s | 30s |
| Web access (websearch, webfetch) | 30s | 120s |
| Execution (bash) | 30s | 300s |
| Git operations | 60s | 300s |
| Other | 10s | 60s |

### Error Recovery

#### 1. Tool Not Found

**LLM Prompt:**
```
Error: Tool 'git_status' not found. Available tools: bash, read, write.
Please choose an available tool or explain to the user.
```

#### 2. Timeout

**LLM Prompt:**
```
Error: Tool 'bash' timed out after 30s. Command: 'sleep 60'.
Try again with a shorter timeout or explain to the user.
```

#### 3. Permission Denied

**LLM Prompt:**
```
Error: Permission denied for tool 'bash' (requires: shell).
This agent does not have shell access. Explain to the user.
```

#### 4. Invalid Parameters

**LLM Prompt:**
```
Error: Invalid parameters for 'read': missing 'path'.
Required: [path]. Optional: [offset, limit].
```

### Error Metrics Tracking

```go
type ToolMetrics struct {
    TotalCalls       int64
    SuccessCount     int64
    ErrorCount       int64
    TimeoutCount     int64
    AvgLatencyMs     float64
    LastError        string
   LastErrorTime     time.Time
}
```

---

## Security Considerations

### Sandboxing Strategy

#### 1. Tool-Level Sandboxing

Each tool specifies its sandboxing requirements in `skill.toml`:

```toml
[[tools]]
name = "bash"
binary = "bin/dt-bash"
description = "Execute shell commands"
sandboxing = "firejail"  # Options: none, firejail, bubblewrap, podman
```

#### 2. Agent Capability Filtering

Tools are filtered by agent capabilities:

```toml
[agent]
capabilities = ["file_read", "file_write", "shell"]

# Only tools matching capabilities are exposed to LLM
```

#### 3. Parameter Sanitization

- **File paths:** Validate and restrict to workspace
- **Shell commands:** Block dangerous commands (rm -rf /, etc.)
- **URLs:** Whitelist allowed domains
- **Network:** Restrict to allowed IPs/ports

### Permission System

#### Tool Permissions

| Permission | Tools Requiring It | Description |
|------------|-------------------|-------------|
| `file_read` | read, glob, grep | Read files from workspace |
| `file_write` | write, edit | Modify files in workspace |
| `shell` | bash | Execute shell commands |
| `network` | websearch, webfetch | Make network requests |
| `git` | git_* | Git operations |
| `session` | sessions_* | Manage sub-agent sessions |

#### Agent Capability Mapping

```toml
[agent]
type = "desktop"
capabilities = [
    "file_read",
    "file_write",
    "shell",
    "network",
    "git",
    "session"
]
```

### Audit Logging

All tool invocations are logged:

```json
{
  "timestamp": "2025-02-15T13:13:00Z",
  "agent_id": "desktop-agent",
  "tool": "bash",
  "parameters": {"command": "ls -la"},
  "result": "success",
  "exit_code": 0,
  "elapsed_ms": 123
}
```

Logs stored in:
- Hot: In-memory for recent operations
- Warm: Local file for last 7 days
- Cold: Turso cloud DB for long-term audit

---

## Example Flows

### Example 1: GPIO Check

**User:**
```
"Check if GPIO pin 17 is high"
```

**LLM Tool Call:**
```json
{
  "name": "bash",
  "arguments": {
    "command": "raspi-gpio get 17"
  }
}
```

**Edge Agent Execution:**
```bash
$ raspi-gpio get 17
GPIO 17: level=1 fsel=0 func=INPUT
```

**LLM Response:**
```
"GPIO pin 17 is currently HIGH (level=1). It's configured as an input."
```

### Example 2: File Read

**User:**
```
"What's in /etc/hostname?"
```

**LLM Tool Call:**
```json
{
  "name": "read",
  "arguments": {
    "path": "/etc/hostname"
  }
}
```

**Edge Agent Execution:**
```bash
$ bin/dt-read --path /etc/hostname
raspberrypi
```

**LLM Response:**
```
"The hostname is 'raspberrypi'."
```

### Example 3: Multi-Tool Chain

**User:**
```
"List the last 3 commits and show the diff for the most recent one"
```

**Turn 1 - LLM calls git_log:**
```json
{
  "name": "git_log",
  "arguments": {
    "max_count": 3
  }
}
```

**Result:**
```
commit abc123 (HEAD -> main)
Author: Bowen
Date:   2025-02-15
    Fix bug

commit def456
Author: Bowen
Date:   2025-02-14
    Add feature

commit ghi789
Author: Bowen
Date:   2025-02-13
    Initial commit
```

**Turn 2 - LLM calls git_diff:**
```json
{
  "name": "git_diff",
  "arguments": {
    "commit": "abc123"
  }
}
```

**Result:**
```
diff --git a/main.go b/main.go
index 123..456 100644
--- a/main.go
+++ b/main.go
@@ -1,3 +1,4 @@
 package main
+func fix() {}
```

**Turn 3 - LLM responds:**
```
"Here are the last 3 commits:

1. abc123 - Fix bug (most recent)
2. def456 - Add feature
3. ghi789 - Initial commit

The diff for the most recent commit (abc123) shows a 'fix()' function was added to main.go."
```

---

## Implementation Plan

### Phase 1: Basic Tool Loop (Current Sprint)

**Goals:**
- Single tool call per turn
- Basic error handling
- Tool schema generation from skill.toml

**Files to Modify:**

1. **`internal/orchestrator/tools.go`** (NEW)
   - `GenerateToolSchemas(agentID) -> []ToolSchema`
   - `LoadSkillDefinitions(skillPath) -> []ToolDefinition`
   - `ToolDefinitionToLLM(tool) -> ToolSchema`
   - `FilterToolsByCapabilities(tools, capabilities) -> []ToolSchema`

2. **`internal/orchestrator/orchestrator.go`**
   - Modify `processWithAgent()` to support tool loop
   - Add `executeToolLoop(agent, msg, model) -> Response`
   - Add `sendToolCommand(agentID, toolCall) -> ToolResult`
   - Add `waitForToolResult(requestID, timeout) -> ToolResult`
   - Update ChatRequest to include tools

3. **`internal/channels/mqtt.go`**
   - Modify `Send()` to handle tool commands
   - Add `SendToolCommand(agentID, toolCall) -> error`
   - Modify message handler to route tool results correctly

4. **`edge-agent/src/commands.rs`**
   - Add `handle_tool(&cmd) -> CommandResult`
   - Parse tool name and parameters
   - Execute tool binary
   - Return structured result

### Phase 2: Multi-Tool and Parallel Execution (✅ Implemented)

**Goals:**
- Multiple tool calls in single LLM response
- Parallel execution where safe
- Tool dependency resolution

**Changes:**
- Support array of tool_calls in LLM response
- Execute independent tools in parallel (goroutines)
- Wait for all results before next LLM call
- Track tool dependencies for sequential execution

**Implementation notes (`internal/orchestrator/toolloop.go`):**

`executeParallel()` is the fan-out/fan-in engine for parallel tool execution:

- **Single-call fast path:** when only one tool call is present, `executeParallel` skips goroutine overhead entirely and calls `execFunc` (or `executeToolCall` as fallback) directly.
- **Multi-call path:** uses `golang.org/x/sync/errgroup` with `g.SetLimit(maxParallel)` (default: 5) to bound concurrency. Goroutines never return errors — per-call errors are captured in a pre-allocated `parallelToolResult` slice indexed by original call order, guaranteeing result ordering without post-sort.
- **Context propagation:** the errgroup context is passed to each goroutine; a pre-flight `select` on `gCtx.Done()` lets goroutines bail immediately if the parent context is cancelled.
- **New metrics:** `ParallelBatches`, `MaxConcurrency`, and `WallTimeSavedMs` (sum of individual elapsed times minus actual wall time) are updated in `Execute()` for every multi-call batch.
- **Backward compat:** single-call batches do not increment `ParallelBatches`, preserving Phase 1 behaviour exactly.

### Phase 3: Tool Result Streaming (Future)

**Goals:**
- Stream tool output as it arrives
- Support long-running tools (e.g., file download)
- Real-time progress updates

**Changes:**
- Use Server-Sent Events (SSE) for streaming
- Buffer partial results
- Update LLM context incrementally

### Phase 4: Cross-Agent Tool Delegation (Future)

**Goals:**
- Agent A can ask Agent B to execute a tool
- Tool discovery across agents
- Load balancing

**Changes:**
- Registry of agent capabilities
- Tool routing based on availability
- Federation protocol for tool delegation

---

## Testing Strategy

### Unit Tests

1. **Tool Schema Generation**
   - Test parsing of skill.toml
   - Test LLM schema generation
   - Test capability filtering

2. **Tool Loop Logic**
   - Test single tool call
   - Test multi-turn loop
   - Test timeout handling
   - Test error recovery

3. **Command Handler**
   - Test tool command parsing
   - Test parameter validation
   - Test result serialization

### Integration Tests

1. **End-to-End Tool Invocation**
   - Mock MQTT broker
   - Mock LLM responses
   - Verify full flow

2. **Error Scenarios**
   - Tool not found
   - Timeout
   - Permission denied
   - Invalid parameters

### Manual Testing

1. **Real MQTT + LLM**
   - Test with actual edge agent
   - Test with actual LLM (Claude, GPT-4)
   - Verify tool execution

2. **Complex Multi-Tool Tasks**
   - File operations + git
   - Web search + file write
   - Bash + read

---

## Configuration

### Orchestrator Config

```toml
[orchestrator]
tool_loop_enabled = true
max_tool_iterations = 10
tool_error_limit = 3
default_tool_timeout_ms = 30000

[orchestrator.tools]
skills_path = "~/.evoclaw/skills"
enable_sandboxing = true
sandbox_engine = "firejail"  # none, firejail, bubblewrap, podman

[orchestrator.tools.permissions]
file_read = true
file_write = true
shell = true
network = true
git = true
session = true
```

### Edge Agent Config

```toml
[agent]
capabilities = ["file_read", "file_write", "shell"]

[tools]
sandboxing = "firejail"
timeout_ms = 30000

[tools.paths]
bin = "~/.evoclaw/skills/desktop-tools/bin"
```

---

## Monitoring and Observability

### Metrics to Track

1. **Tool Usage**
   - Calls per tool
   - Success/error rate
   - Average latency

2. **Loop Performance**
   - Average iterations per request
   - Tool call frequency
   - Time to final response

3. **Error Tracking**
   - Error types (timeout, not_found, etc.)
   - Error rate per tool
   - Recovery success rate

### Health Checks

```bash
# Check tool loop status
evoclaw status --tools

# View tool metrics
evoclaw metrics --tools

# View recent tool calls
evoclaw logs --filter=tool
```

---

## Open Questions

1. **Tool Result Size Limits**
   - What if a tool returns 100MB of data?
   - Should we truncate? Stream to file?

2. **Long-Running Tools**
   - How to handle tools that take hours?
   - Background execution pattern?

3. **Tool Versioning**
   - How to handle multiple versions of same tool?
   - Migration strategy?

4. **Cross-Platform Tools**
   - Tools work differently on Linux vs macOS vs Windows
   - How to handle platform-specific tools?

5. **LLM Cost Management**
   - Tools increase LLM token usage
   - How to track and control costs?

---

## Built-in Tool Architecture (Pi-Inspired)

> **See [TOOL-ARCHITECTURE.md](./TOOL-ARCHITECTURE.md) for the full design.**

The orchestrator now supports **built-in tools** created via factory functions alongside TOML-loaded tools. Key concepts:

- **Operations injection** — `FileOps` and `ExecOps` interfaces abstract local/SSH/HTTP backends
- **Factory pattern** — `NewReadTool(opts)`, `CodingTools(cwd)`, `RemoteTools(...)` create configured tool sets
- **ContentBlock returns** — `ToolOutput` with `TextBlock`, `ImageBlock`, `ErrorBlock` replaces raw strings
- **Backward compatible** — `ToolOutput.ToLegacyResult()` converts to the existing `ToolResult` format

---

## References

- [OpenAI Function Calling](https://platform.openai.com/docs/guides/function-calling)
- [Anthropic Tool Use](https://docs.anthropic.com/claude/docs/tool-use)
- [EvoClaw INSTALLATION.md](./INSTALLATION.md)
- [EvoClaw MESSAGING.md](./MESSAGING.md)
- [EvoClaw TOOL-ARCHITECTURE.md](./TOOL-ARCHITECTURE.md)
- [Desktop-Tools Skill](~/.evoclaw/skills/desktop-tools/skill.toml)
