# MQTT Protocol Reference

MQTT is the primary communication protocol between the orchestrator and edge agents. EvoClaw uses standard MQTT v3.1.1.

## Connection

### Broker Settings

Default: `localhost:1883`

```toml
# Edge agent config
[mqtt]
broker = "localhost"
port = 1883
keep_alive_secs = 30
```

```json
// Orchestrator config
{
  "mqtt": {
    "host": "0.0.0.0",
    "port": 1883,
    "username": "",
    "password": ""
  }
}
```

### Client IDs

- Orchestrator: `evoclaw-orchestrator`
- Edge agents: `evoclaw-agent-{agent_id}`

## Topic Hierarchy

```
evoclaw/
├── agents/
│   └── {agent_id}/
│       ├── commands     # Orchestrator → Agent
│       ├── reports      # Agent → Orchestrator
│       └── status       # Agent → Orchestrator (heartbeats)
└── broadcast            # Orchestrator → All Agents
```

## Message Types

### Heartbeat

**Topic:** `evoclaw/agents/{agent_id}/status`
**Direction:** Agent → Orchestrator
**QoS:** 0 (at most once)
**Frequency:** Every `keep_alive_secs` seconds

```json
{
  "type": "heartbeat",
  "agent_id": "hl-trader-1",
  "timestamp": "2026-02-06T10:30:00Z",
  "status": "running",
  "uptime_secs": 3600,
  "metrics": {
    "total_actions": 156,
    "successful_actions": 142,
    "failed_actions": 14,
    "tokens_used": 45000,
    "cost_usd": 0.234,
    "custom": {
      "totalTrades": 12,
      "winRate": 0.667,
      "profitLoss": 127.40
    }
  }
}
```

### Command

**Topic:** `evoclaw/agents/{agent_id}/commands`
**Direction:** Orchestrator → Agent
**QoS:** 1 (at least once)

#### Execute Strategy

```json
{
  "type": "command",
  "id": "cmd-550e8400-e29b",
  "action": "execute_strategy",
  "params": {
    "strategy": "FundingArbitrage",
    "asset": "ETH-PERP",
    "max_size_usd": 5000
  },
  "timestamp": "2026-02-06T10:30:00Z"
}
```

#### Update Parameters

```json
{
  "type": "command",
  "id": "cmd-550e8400-e29c",
  "action": "update_params",
  "params": {
    "temperature": 0.65,
    "minFundingRate": 0.0015,
    "positionSizePct": 0.12
  },
  "timestamp": "2026-02-06T10:30:00Z"
}
```

#### Evolution Update

```json
{
  "type": "command",
  "id": "cmd-550e8400-e29d",
  "action": "evolution_update",
  "params": {
    "strategy_version": 4,
    "temperature": 0.65,
    "maxTokens": 4096,
    "custom_params": {
      "minFundingRate": 0.0015,
      "positionSizePct": 0.12
    }
  },
  "timestamp": "2026-02-06T10:30:00Z"
}
```

#### Pause/Resume

```json
{
  "type": "command",
  "id": "cmd-550e8400-e29e",
  "action": "pause",
  "timestamp": "2026-02-06T10:30:00Z"
}
```

```json
{
  "type": "command",
  "id": "cmd-550e8400-e29f",
  "action": "resume",
  "timestamp": "2026-02-06T10:30:00Z"
}
```

### Report

**Topic:** `evoclaw/agents/{agent_id}/reports`
**Direction:** Agent → Orchestrator
**QoS:** 1 (at least once)

#### Command Result

```json
{
  "type": "report",
  "agent_id": "hl-trader-1",
  "command_id": "cmd-550e8400-e29b",
  "status": "completed",
  "result": {
    "action": "placed_order",
    "asset": "ETH-PERP",
    "side": "buy",
    "size": 2.5,
    "price": 3245.50,
    "order_id": "0x1234..."
  },
  "timestamp": "2026-02-06T10:30:01Z"
}
```

#### Error Report

```json
{
  "type": "report",
  "agent_id": "hl-trader-1",
  "command_id": "cmd-550e8400-e29b",
  "status": "error",
  "error": "insufficient margin for order",
  "timestamp": "2026-02-06T10:30:01Z"
}
```

#### Market Alert

```json
{
  "type": "report",
  "agent_id": "hl-trader-1",
  "status": "alert",
  "result": {
    "alert_type": "price_movement",
    "asset": "ETH",
    "change_pct": 5.2,
    "current_price": 3245.50,
    "message": "ETH moved +5.2% in the last hour"
  },
  "timestamp": "2026-02-06T10:30:00Z"
}
```

### Broadcast

**Topic:** `evoclaw/broadcast`
**Direction:** Orchestrator → All Agents
**QoS:** 1 (at least once)

```json
{
  "type": "broadcast",
  "action": "pause_trading",
  "reason": "high volatility detected",
  "timestamp": "2026-02-06T10:30:00Z"
}
```

## Subscription Patterns

### Orchestrator Subscribes To:

```
evoclaw/agents/+/reports    # All agent reports
evoclaw/agents/+/status     # All agent heartbeats
```

### Agent Subscribes To:

```
evoclaw/agents/{my_id}/commands  # My commands
evoclaw/broadcast                 # Global broadcasts
```

## QoS Summary

| Message Type | QoS | Retained | Rationale |
|-------------|-----|----------|-----------|
| Heartbeat | 0 | No | Missing one is OK |
| Command | 1 | No | Must be delivered |
| Report | 1 | No | Must be delivered |
| Broadcast | 1 | No | Must reach all agents |

## See Also

- [Communication Architecture](../architecture/communication.md)
- [REST API](rest-api.md)
- [Edge Agent](../architecture/edge-agent.md)
