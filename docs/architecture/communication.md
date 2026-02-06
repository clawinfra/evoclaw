# Communication Protocol

EvoClaw uses multiple communication channels. The primary protocols are MQTT (for agent mesh) and HTTP (for API access).

## MQTT (Agent Mesh)

MQTT is the backbone for orchestrator ↔ edge agent communication. It's lightweight, supports QoS levels, and works well on constrained networks.

### Topic Structure

```
evoclaw/
├── agents/
│   └── {agent_id}/
│       ├── commands     # Orchestrator → Agent (commands/instructions)
│       ├── reports      # Agent → Orchestrator (results/updates)
│       └── status       # Agent → Orchestrator (heartbeats)
└── broadcast            # Orchestrator → All Agents
```

### Message Format

All MQTT messages use JSON:

#### Heartbeat (status)

```json
{
  "type": "heartbeat",
  "agent_id": "hl-trader-1",
  "timestamp": "2026-02-06T10:30:00Z",
  "status": "running",
  "metrics": {
    "total_actions": 156,
    "successful_actions": 142,
    "tokens_used": 45000,
    "cost_usd": 0.234
  }
}
```

#### Command (orchestrator → agent)

```json
{
  "type": "command",
  "id": "cmd-uuid-123",
  "action": "execute_strategy",
  "params": {
    "strategy": "FundingArbitrage",
    "asset": "ETH-PERP",
    "max_size_usd": 5000
  },
  "timestamp": "2026-02-06T10:30:00Z"
}
```

#### Report (agent → orchestrator)

```json
{
  "type": "report",
  "agent_id": "hl-trader-1",
  "command_id": "cmd-uuid-123",
  "status": "completed",
  "result": {
    "action": "placed_order",
    "asset": "ETH-PERP",
    "side": "buy",
    "size": 2.5,
    "price": 3245.50
  },
  "timestamp": "2026-02-06T10:30:01Z"
}
```

#### Evolution Update (orchestrator → agent)

```json
{
  "type": "evolution_update",
  "agent_id": "hl-trader-1",
  "strategy_version": 4,
  "params": {
    "temperature": 0.65,
    "minFundingRate": 0.0015,
    "positionSizePct": 0.12
  }
}
```

#### Broadcast

```json
{
  "type": "broadcast",
  "action": "pause_trading",
  "reason": "high volatility detected",
  "timestamp": "2026-02-06T10:30:00Z"
}
```

### QoS Levels

| Topic | QoS | Rationale |
|-------|-----|-----------|
| `commands` | 1 (at least once) | Commands must be delivered |
| `reports` | 1 (at least once) | Reports must be delivered |
| `status` | 0 (at most once) | Heartbeats can be missed |
| `broadcast` | 1 (at least once) | Broadcasts must be delivered |

### Connection Settings

```toml
[mqtt]
broker = "localhost"
port = 1883
keep_alive_secs = 30
```

The keep-alive interval doubles as the heartbeat frequency. If no heartbeat is received within `2 × keep_alive_secs`, the orchestrator marks the agent as unhealthy.

## HTTP API

The HTTP API is used for:
- Dashboard access
- External integrations
- Agent management
- Metrics querying

See [REST API Reference](../api/rest-api.md) for full documentation.

## Telegram

Telegram integration uses HTTP long polling:

```
User → Telegram API → EvoClaw (poll) → Orchestrator → Agent → Response → Telegram API → User
```

- No webhook required (works behind NAT)
- Polls every few seconds for new messages
- Supports text messages and replies

## Data Serialization

All communication uses **JSON**. This keeps things simple and debuggable:

- MQTT payloads: JSON
- HTTP API: JSON (`Content-Type: application/json`)
- Configuration: JSON (orchestrator) / TOML (edge agent)
- State files: JSON

## Security Considerations

### Current State
- MQTT: Username/password authentication (optional)
- HTTP API: No authentication (bind to localhost in production)
- CORS: Allow all origins (development mode)

### Planned
- MQTT: TLS encryption
- HTTP API: API key or JWT authentication
- Agent identity: Cryptographic agent attestation
- Channel encryption: End-to-end encryption for messages

## See Also

- [MQTT Protocol Reference](../api/mqtt-protocol.md)
- [REST API Reference](../api/rest-api.md)
- [WebSocket/SSE Endpoints](../api/websocket.md)
