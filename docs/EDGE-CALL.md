# Edge Call — Generic Tool Interface for Edge Agents

## Problem

Edge devices (e.g. Pi sensor nodes, trading agents) have diverse and evolving tool sets. The naive approach — defining explicit tool schemas in the orchestrator for every edge capability — doesn't scale:

- Adding a new tool to an edge device requires an orchestrator code change
- The orchestrator ends up mirroring every edge tool definition
- Schema drift becomes a maintenance burden across many devices

## Solution

A single generic `edge_call` tool in the orchestrator's tool loop, combined with **capability advertisement** on the edge. The orchestrator never needs to know *what* tools an edge device has — it only needs to be able to *ask* it things.

```
User query
    │
    ▼
Orchestrator LLM
    │  sees: edge_call(agent_id, query) + online agent capabilities
    ▼
edge_call tool
    │  sends: MQTT prompt → evoclaw/agents/{id}/commands
    ▼
Edge Agent LLM + local tool loop
    │  handles: routing, execution, tool calls
    ▼
AgentReport result
    │  returns: natural language answer
    ▼
Orchestrator LLM  →  User
```

## Architecture

### Capability Advertisement (Edge → Orchestrator)

On startup, each edge agent publishes a **retained** MQTT message:

```
Topic:   evoclaw/agents/{agent_id}/capabilities
Payload: { "agent_id": "alex-eye", "capabilities": "Pi sensor node — temperature, CPU/disk stats, camera" }
QoS:     1  (at least once)
Retain:  true  ← orchestrator receives this immediately on (re)connect
```

Because the message is retained, timing doesn't matter — the orchestrator always gets the latest capability summary on startup or reconnect, without requiring the edge agent to be online at that exact moment.

The capability summary is a **one-liner**, not a schema. The LLM reads it in plain English to decide whether and how to call the agent.

### Dynamic Tool Schema (Orchestrator)

The orchestrator dynamically builds a single `edge_call` tool schema on each request, incorporating all currently-online edge agents:

```json
{
  "name": "edge_call",
  "description": "Call an edge agent to handle a query using its own tools and sensors. Online edge agents:\n  - alex-eye: Pi sensor node — temperature, CPU/memory/disk stats, camera snapshot",
  "parameters": {
    "agent_id": "string — ID of the target edge agent",
    "query":    "string — natural language query (agent handles routing)",
    "action":   "string — optional: specific action name for structured calls",
    "params":   "object — optional: parameters for the structured action"
  }
}
```

If no edge agents are online, the tool is not included in the LLM's context. No dead tools.

### Tool Execution (Orchestrator)

When the LLM calls `edge_call`:

1. Orchestrator validates `agent_id` is online via MQTT heartbeat tracking
2. Sends a `command: "prompt"` message to `evoclaw/agents/{id}/commands`
3. Waits up to 60s for an `AgentReport` with `report_type: "result"`
4. Returns the natural language content as the tool result

```go
// Natural language mode (recommended)
edge_call(agent_id="alex-eye", query="what's the CPU temperature right now?")

// Structured mode (for precise control)
edge_call(agent_id="alex-eye", action="snapshot", params={"resolution": "640x480"})
```

In structured mode, the query becomes: `"Execute action: snapshot with params: {...}"`.

### Edge Agent Behavior

The edge agent receives the query as a normal `command: "prompt"` message — identical to what the orchestrator already sends for direct agent queries. The edge agent's own LLM+tool loop handles routing:

- Routes to the right tool (temperature sensor, camera, disk check, etc.)
- Executes locally
- Returns a natural language result via `AgentReport`

**No changes needed to edge tool definitions when adding new tools.** Only the `capabilities` field in `agent.toml` needs updating.

## Configuration

### Edge Agent (`agent.toml`)

```toml
agent_id   = "alex-eye"
agent_type = "monitor"

# One-line summary of what this agent can do.
# Shown to the orchestrator LLM to decide when/how to call this agent.
# If omitted, a default is generated from agent_type.
capabilities = "Pi sensor node — CPU/memory/disk stats, temperature, process monitoring, file system"
```

If `capabilities` is not set, the agent auto-generates a default:

| `agent_type` | Default capabilities |
|---|---|
| `sensor` / `monitor` | `{id} sensor node — temperature, CPU/memory/disk stats, process monitoring, camera snapshot` |
| `trader` | `{id} trading agent — market data, order execution, position management` |
| `governance` | `{id} governance agent — on-chain voting, proposal management` |
| *(other)* | `{id} edge agent` |

### Orchestrator

No configuration needed. The `edge_call` tool is automatically registered when MQTT is enabled and at least one edge agent is online. Capabilities are received via MQTT retained messages.

## Implementation

| File | Change |
|---|---|
| `edge-agent/src/config.rs` | Added optional `capabilities: Option<String>` to `Config` |
| `edge-agent/src/mqtt.rs` | `advertise_capabilities()` — publishes retained capability message |
| `edge-agent/src/agent.rs` | Calls `advertise_capabilities()` after `subscribe()` on startup |
| `internal/channels/mqtt.go` | `EdgeAgentInfo.Capabilities` field; `handleCapabilities()` handler; `evoclaw/agents/+/capabilities` subscription; `GetEdgeAgentCapabilities()`, `GetOnlineAgentsWithCapabilities()` |
| `internal/orchestrator/orchestrator.go` | `buildEdgeCallSchema()` — builds dynamic tool schema from online agents |
| `internal/orchestrator/toolloop.go` | Appends `edge_call` schema in `Execute()`; `executeEdgeCall()` handles tool execution via `SendPromptAndWait` |

## Scaling Properties

| Property | Before | After |
|---|---|---|
| New tool on edge device | Requires orchestrator schema update | Config one-liner only |
| New edge device type | Requires new tool definitions | `capabilities` field + restart |
| Orchestrator tool count | O(devices × tools) | O(1) always |
| Schema drift | High maintenance risk | Impossible — schemas live on edge |
| Offline agent | Tool still visible, calls fail | Tool removed from LLM context |

## Example

User: *"What's the temperature on the Pi?"*

```
LLM sees:
  Tool: edge_call
  Online agents:
    - alex-eye: Pi sensor node — CPU/memory/disk stats, temperature, process monitoring

LLM calls:
  edge_call(agent_id="alex-eye", query="What is the current CPU temperature?")

alex-eye LLM:
  → calls temperature tool
  → returns "CPU temperature is 42.3°C"

Orchestrator LLM:
  → "The Pi (alex-eye) is running at 42.3°C."
```

User: *"What's the disk usage and are there any processes using a lot of CPU?"*

```
LLM calls:
  edge_call(agent_id="alex-eye", query="What is the disk usage and which processes are using the most CPU?")

alex-eye handles both tool calls in its own loop, returns combined answer.
Orchestrator never knew about disk_usage or process_list tools.
```

## MQTT Topics Reference

| Topic | Direction | Retain | Purpose |
|---|---|---|---|
| `evoclaw/agents/{id}/capabilities` | Edge → Orch | **Yes** | Capability advertisement on startup |
| `evoclaw/agents/{id}/commands` | Orch → Edge | No | Commands + prompts |
| `evoclaw/agents/{id}/reports` | Edge → Orch | No | Results + heartbeats |
| `evoclaw/agents/{id}/status` | Edge → Orch | No | Status heartbeats |
