# REST API Reference

The EvoClaw HTTP API runs on port 8420 (configurable). All responses are JSON.

## Base URL

```
http://localhost:8420
```

## Endpoints

### System

#### `GET /api/status`

Returns system status and aggregated metrics.

**Response:**
```json
{
  "version": "0.1.0",
  "uptime": -1234567890,
  "agents": 3,
  "models": 5,
  "memory": {
    "total_entries": 12,
    "total_tokens": 45000
  },
  "total_cost": 2.3456
}
```

| Field | Type | Description |
|-------|------|-------------|
| `version` | string | EvoClaw version |
| `uptime` | int | Uptime in nanoseconds (Go duration) |
| `agents` | int | Number of registered agents |
| `models` | int | Number of available models |
| `memory` | object | Memory store statistics |
| `total_cost` | float | Total API cost in USD |

#### `GET /api/dashboard`

Aggregated dashboard metrics.

**Response:**
```json
{
  "version": "0.1.0",
  "agents": 3,
  "models": 5,
  "evolving_agents": 1,
  "total_cost": 2.3456,
  "total_requests": 156,
  "total_tokens_in": 234567,
  "total_tokens_out": 89012,
  "total_messages": 42,
  "total_errors": 3,
  "total_actions": 39,
  "success_rate": 0.923,
  "timestamp": "2026-02-06T10:30:00Z"
}
```

---

### Agents

#### `GET /api/agents`

List all registered agents.

**Response:**
```json
[
  {
    "id": "assistant-1",
    "def": {
      "id": "assistant-1",
      "name": "General Assistant",
      "type": "orchestrator",
      "model": "anthropic/claude-sonnet-4-20250514",
      "systemPrompt": "You are a helpful assistant.",
      "skills": ["chat"],
      "config": {},
      "container": { "enabled": false }
    },
    "status": "idle",
    "started_at": "2026-02-06T08:00:00Z",
    "last_active": "2026-02-06T10:25:00Z",
    "last_heartbeat": "2026-02-06T10:29:30Z",
    "message_count": 42,
    "error_count": 3,
    "metrics": {
      "total_actions": 39,
      "successful_actions": 36,
      "failed_actions": 3,
      "avg_response_ms": 1234.5,
      "tokens_used": 45000,
      "cost_usd": 0.234
    }
  }
]
```

#### `GET /api/agents/{id}`

Get details for a specific agent.

**Parameters:**
| Param | Location | Description |
|-------|----------|-------------|
| `id` | path | Agent ID |

**Response:** Same schema as individual agent in list above.

**Error:** `404 Not Found` if agent doesn't exist.

#### `GET /api/agents/{id}/metrics`

Get performance metrics for an agent.

**Response:**
```json
{
  "agent_id": "assistant-1",
  "metrics": {
    "total_actions": 39,
    "successful_actions": 36,
    "failed_actions": 3,
    "avg_response_ms": 1234.5,
    "tokens_used": 45000,
    "cost_usd": 0.234,
    "custom": {
      "profitLoss": 127.40
    }
  },
  "status": "idle",
  "uptime": 9000.5
}
```

#### `GET /api/agents/{id}/memory`

Get conversation memory for an agent.

**Response:**
```json
{
  "agent_id": "assistant-1",
  "message_count": 24,
  "total_tokens": 12000,
  "messages": [
    {
      "role": "user",
      "content": "What is EvoClaw?"
    },
    {
      "role": "assistant",
      "content": "EvoClaw is a self-evolving agent framework..."
    }
  ]
}
```

#### `DELETE /api/agents/{id}/memory`

Clear conversation memory for an agent.

**Response:**
```json
{
  "message": "memory cleared",
  "agent_id": "assistant-1"
}
```

#### `POST /api/agents/{id}/evolve`

Trigger evolution (strategy mutation) for an agent.

**Response:**
```json
{
  "message": "evolution triggered",
  "agent_id": "assistant-1"
}
```

#### `GET /api/agents/{id}/evolution`

Get evolution/strategy data for an agent.

**Response:**
```json
{
  "agent_id": "assistant-1",
  "version": 3,
  "fitness": 0.72,
  "evalCount": 24,
  "temperature": 0.65,
  "params": {
    "minFundingRate": 0.0015,
    "positionSizePct": 0.12
  }
}
```

---

### Models

#### `GET /api/models`

List all available models.

**Response:**
```json
[
  {
    "ID": "anthropic/claude-sonnet-4-20250514",
    "Provider": "anthropic",
    "Config": {
      "id": "claude-sonnet-4-20250514",
      "name": "Claude Sonnet 4",
      "contextWindow": 200000,
      "costInput": 3.0,
      "costOutput": 15.0,
      "capabilities": ["reasoning", "code", "vision"]
    }
  }
]
```

#### `GET /api/costs`

Get cost tracking data for all models.

**Response:**
```json
{
  "anthropic/claude-sonnet-4-20250514": {
    "TotalRequests": 156,
    "TotalTokensIn": 234567,
    "TotalTokensOut": 89012,
    "TotalCostUSD": 2.34,
    "LastRequestTime": 0
  }
}
```

---

### Real-Time

#### `GET /api/logs/stream`

Server-Sent Events (SSE) endpoint for real-time log streaming.

**Headers:**
```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
```

**Events:**
```
data: {"time":"10:30:05","level":"info","component":"api","message":"HTTP request completed"}

data: {"time":"10:30:10","level":"info","component":"system","message":"heartbeat: 3 agents online"}
```

See [WebSocket/SSE endpoints](websocket.md) for details.

---

### Web Dashboard

#### `GET /`

Serves the embedded web dashboard (HTML/CSS/JS).

The dashboard is a single-page application that uses all the above API endpoints.

---

## CORS

All endpoints include CORS headers:
```
Access-Control-Allow-Origin: *
Access-Control-Allow-Methods: GET, POST, PUT, DELETE, OPTIONS
Access-Control-Allow-Headers: Content-Type, Authorization
```

## Error Responses

Errors return plain text with appropriate HTTP status codes:

| Status | Meaning |
|--------|---------|
| `400 Bad Request` | Invalid request parameters |
| `404 Not Found` | Agent or resource not found |
| `405 Method Not Allowed` | Wrong HTTP method |
| `500 Internal Server Error` | Server error |

## See Also

- [MQTT Protocol](mqtt-protocol.md)
- [WebSocket/SSE](websocket.md)
- [Configuration](../getting-started/configuration.md)
