# Phase 1 Agentic Tool Loop - Implementation Notes

## What Was Implemented

### 1. Tool Schema Generation (`internal/orchestrator/tools.go`)

**Components:**
- `ToolDefinition`: Tool metadata from skill.toml files
- `ToolSchema`: LLM-compatible JSON Schema format
- `ToolManager`: Discovers, loads, and filters tools by agent capabilities

**Key Features:**
- Scans `~/.evoclaw/skills/*/skill.toml` for tool definitions
- Filters tools by agent capabilities (file_read, file_write, shell, etc.)
- Caches generated schemas for performance
- Converts tool definitions to OpenAI-compatible function calling format

**Timeout Defaults:**
- File ops (read, write, edit, glob, grep): 5s
- Web access (websearch, webfetch, codesearch): 30s
- Execution (bash): 30s
- Git operations: 60s
- Default fallback: 10s

### 2. Tool Loop (`internal/orchestrator/toolloop.go`)

**Components:**
- `ToolLoop`: Multi-turn execution loop manager
- `ToolCall`: Tool invocation from LLM
- `ToolResult`: Execution result from edge agent
- `ToolLoopMetrics`: Performance tracking

**Flow:**
1. Generate tool schemas for agent's capabilities
2. Call LLM with user message + tools
3. Check if LLM wants to call a tool
4. If yes:
   - Parse tool_call (name, arguments)
   - Send command to edge agent via MQTT
   - Wait for result (with timeout)
   - Add result to conversation as "tool" message
   - Loop back to step 2
5. If no tool call: Break loop, return final response

**Safeguards:**
- Max 10 iterations (configurable)
- Max 3 consecutive errors (configurable)
- Per-tool timeout enforcement
- Graceful error handling with LLM feedback

### 3. Orchestrator Integration (`internal/orchestrator/orchestrator.go`)

**Changes:**
- Added `toolManager`, `toolLoop`, `resultRegistry` to Orchestrator struct
- Initialize ToolManager on first agent with capabilities
- Modified `processWithAgent()` to route through tool loop when available
- Added fallback to legacy direct LLM mode
- Extended `ChatMessage` with ToolCalls and ToolCallID fields
- Extended `ChatResponse` with ToolCalls field
- Added `ChatRequest.Tools` field for function calling

**Backward Compatibility:**
- If agent has no capabilities → legacy mode (no tools)
- If toolLoop not initialized → direct LLM call
- Existing tests should pass unchanged

### 4. Helper Functions (`internal/orchestrator/orchestrator_helpers.go`)

**New Methods:**
- `processDirect()`: Legacy LLM call without tools
- `RegisterResultHandler()`: Register callback for tool result
- `DeliverToolResult()`: Deliver result to waiting handler
- `indexChar()`: Helper for string indexing

**Result Registry Pattern:**
- Orchestrator maintains map of request_id → result channel
- ToolLoop registers handler before sending command
- MQTT handler delivers result when received
- Prevents deadlocks with 5s timeout on delivery

### 5. Edge Agent Tool Handler (`edge-agent/src/commands.rs`)

**New Command:**
- `handle_tool()`: Routes tool execution to appropriate binary

**Execution Flow:**
1. Parse tool name, parameters, timeout from command
2. Build binary path: `/home/pi/.evoclaw/skills/desktop-tools/bin/dt-{tool_name}`
3. Build command arguments from parameters (tool-specific logic)
4. Execute with tokio::time::timeout
5. Return structured result (success/error with metadata)

**Parameter Mapping:**
- `read`: --path, --offset, --limit
- `bash`: -c "{command}"
- `write`: --path, {content}
- Generic: --{key} {value} for other tools

**Result Format:**
```rust
{
  "status": "success" | "error",
  "tool": "bash",
  "result": "stdout content",
  "stderr": "stderr content",
  "exit_code": 0,
  "elapsed_ms": 123,
  "request_id": "tool-1234567890",
  "error_type": "timeout" | "not_found" | "execution_failed"
}
```

## Testing Strategy

### Unit Tests (Not Yet Implemented)

**Tools Tests:**
- LoadSkillDefinitions with valid/invalid toml
- DefinitionToSchema conversion
- FilterByCapabilities logic
- GetToolTimeout defaults

**ToolLoop Tests:**
- Single tool call execution
- Multi-turn loop (simulated)
- Timeout handling
- Error limit enforcement
- Result formatting

### Integration Tests (Manual)

**Test 1: Simple Question (No Tool)**
```
User: "What is 2+2?"
Expected: LLM responds with "4" without tool calls
```

**Test 2: GPIO Check (Tool Call)**
```
User: "Check GPIO pin 529 status"
Expected: LLM calls bash tool, executes "raspi-gpio get 529", returns result
```

**Test 3: File Read (Tool Call)**
```
User: "What's in /etc/hostname?"
Expected: LLM calls read tool, returns hostname
```

### Build and Run

**Go (Orchestrator):**
```bash
cd /media/DATA/clawd/evoclaw
go build -o bin/evoclaw-orchestrator ./cmd/orchestrator
./bin/evoclaw-orchestrator --config config/evoclaw.toml
```

**Rust (Edge Agent):**
```bash
cd /media/DATA/clawd/evoclaw/edge-agent
cargo build --release
cargo test
```

## Known Limitations (Phase 1)

1. **Single Tool Only:** LLM can only call one tool per turn
   - Phase 2: Support parallel tool execution

2. **No Streaming:** Tool results must complete before next LLM call
   - Phase 3: Add streaming for long-running tools

3. **No MQTT Tool Result Routing:** Not implemented in this phase
   - Requires MQTT channel modification to deliver tool results
   - Currently tool loop will timeout waiting for result

4. **Hardcoded Tool Path:** Edge agent assumes `/home/pi/.evoclaw/skills/desktop-tools`
   - Should be configurable in agent.toml

5. **Basic Parameter Mapping:** Only read, bash, write have specialized handlers
   - Other tools use generic key-value mapping
   - May not match tool's expected CLI format

## Next Steps (Phase 2)

1. **Implement MQTT Tool Result Delivery:**
   - Modify `internal/channels/mqtt.go` to detect tool results
   - Route to `orchestrator.DeliverToolResult()`

2. **Add Multi-Tool Support:**
   - Process array of tool_calls from LLM
   - Execute independent tools in parallel
   - Wait for all results before next LLM call

3. **Improve Parameter Mapping:**
   - Add tool-specific handlers for all 30 tools
   - Match tool's actual CLI interface

4. **Configuration:**
   - Add tool loop settings to config.toml
   - Make tool paths configurable
   - Add per-agent capability filters

## Files Modified/Created

### Created
- `internal/orchestrator/tools.go` (7498 bytes)
- `internal/orchestrator/toolloop.go` (7647 bytes)
- `internal/orchestrator/orchestrator_helpers.go` (2027 bytes)

### Modified
- `internal/orchestrator/orchestrator.go` (+ ToolManager integration, modified processWithAgent)
- `edge-agent/src/commands.rs` (+ handle_tool, build_tool_args)

### Not Modified (TODO)
- `internal/channels/mqtt.go` (tool result delivery)
- `internal/config/config.go` (tool configuration)
- Model providers (add tools to ChatRequest)

## Deployment Notes

### Before First Run

1. **Install Tools on Edge Device:**
   ```bash
   # Install desktop-tools skill
   mkdir -p ~/.evoclaw/skills
   cp -r /path/to/desktop-tools ~/.evoclaw/skills/
   chmod +x ~/.evoclaw/skills/desktop-tools/bin/*
   ```

2. **Configure Agent Capabilities:**
   ```toml
   [agents]
     [[agents]]
       id = "desktop-agent"
       capabilities = ["file_read", "file_write", "shell", "network"]
   ```

3. **Enable Tool Loop:**
   ```toml
   [orchestrator]
     tool_loop_enabled = true
   ```

### Monitoring

**Logs:**
- Orchestrator: `component=tool_loop`, `component=tool_manager`
- Edge Agent: `tool=`, `executing tool`

**Metrics:**
- ToolLoopMetrics: iterations, tool_calls, success_count, error_count, timeout_count
- Per-tool timing tracked in ToolResult.ElapsedMs

## Troubleshooting

**Issue: Tool Loop Timeouts**
- Check: Is edge agent running?
- Check: Is tool binary installed?
- Check: MQTT broker connectivity

**Issue: "tool not found"**
- Verify tool path in edge-agent handle_tool()
- Check tool binary exists and is executable
- Verify skill.toml has correct binary path

**Issue: Permission Denied**
- Check agent capabilities in config
- Verify tool permissions in skill.toml
- Check file permissions on tool binary

**Issue: LLM Not Calling Tools**
- Verify tool loop enabled in config
- Check tool schemas generated (look at logs)
- Verify LLM provider supports function calling
- Check system prompt encourages tool use

## References

- Design: `/media/DATA/clawd/evoclaw/docs/AGENTIC-TOOL-LOOP.md`
- Implementation: `/media/DATA/clawd/evoclaw/docs/AGENTIC-TOOL-LOOP-IMPLEMENTATION.md`
- Skill Config: `/home/user/.evoclaw/skills/desktop-tools/skill.toml`
