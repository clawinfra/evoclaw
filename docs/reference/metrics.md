# Metrics Reference

EvoClaw tracks comprehensive metrics for monitoring, evolution, and optimization.

## Agent Metrics

Tracked in `internal/agents/registry.go`:

```go
type Metrics struct {
    TotalActions      int64              `json:"total_actions"`
    SuccessfulActions int64              `json:"successful_actions"`
    FailedActions     int64              `json:"failed_actions"`
    AvgResponseMs     float64            `json:"avg_response_ms"`
    TokensUsed        int64              `json:"tokens_used"`
    CostUSD           float64            `json:"cost_usd"`
    Custom            map[string]float64 `json:"custom,omitempty"`
}
```

### Core Metrics

| Metric | Type | Description |
|--------|------|-------------|
| `total_actions` | counter | Total LLM requests made |
| `successful_actions` | counter | Successful completions |
| `failed_actions` | counter | Failed requests (errors, timeouts) |
| `avg_response_ms` | gauge | Running average response time (ms) |
| `tokens_used` | counter | Total tokens consumed (input + output) |
| `cost_usd` | counter | Total API cost in USD |

### Derived Metrics

| Metric | Formula | Description |
|--------|---------|-------------|
| Success Rate | `successful / total` | Fraction of successful actions |
| Error Rate | `failed / total` | Fraction of failed actions |
| Cost per Action | `cost_usd / total` | Average cost per request |
| Tokens per Action | `tokens_used / total` | Average tokens per request |

### Custom Metrics

Agent-specific metrics stored in the `custom` map:

#### Trading Agents

| Key | Type | Description |
|-----|------|-------------|
| `totalTrades` | counter | Total trades executed |
| `winRate` | gauge | Fraction of profitable trades |
| `profitLoss` | gauge | Cumulative P&L in USD |
| `sharpeRatio` | gauge | Risk-adjusted return measure |
| `maxDrawdown` | gauge | Maximum peak-to-trough decline |
| `avgHoldTimeSec` | gauge | Average position hold time |

#### Monitor Agents

| Key | Type | Description |
|-----|------|-------------|
| `alertsTriggered` | counter | Total alerts sent |
| `checksPerformed` | counter | Total monitoring checks |

## Model Cost Metrics

Tracked in `internal/models/router.go`:

```go
type ModelCost struct {
    TotalRequests   int64   // Total API calls
    TotalTokensIn   int64   // Total input tokens
    TotalTokensOut  int64   // Total output tokens
    TotalCostUSD    float64 // Total cost in USD
    LastRequestTime int64   // Unix timestamp of last request
}
```

| Metric | Description |
|--------|-------------|
| `TotalRequests` | Number of API calls to this model |
| `TotalTokensIn` | Input tokens consumed |
| `TotalTokensOut` | Output tokens generated |
| `TotalCostUSD` | `(tokensIn × costInput + tokensOut × costOutput) / 1M` |

## Evolution Metrics

Used by the fitness function (`internal/evolution/engine.go`):

| Input Metric | Weight | Direction | Description |
|-------------|--------|-----------|-------------|
| `successRate` | 40% | Higher = better | Primary effectiveness measure |
| `profitLoss` | 30% | Higher = better | Trading performance (normalized) |
| `costUSD` | 20% | Lower = better | API cost efficiency |
| `avgResponseMs` | 10% | Lower = better | Response speed |

### Fitness Score

```
fitness = 0.4 × successRate
        + 0.3 × max(0, profitLoss + 1.0)
        + 0.2 × (1 / (1 + costUSD))
        + 0.1 × (1 / (1 + avgResponseMs/1000))
```

Score range: 0.0 (terrible) → ~1.0 (excellent)

Threshold for evolution trigger: **0.6** (configurable)

### Fitness Smoothing

Fitness uses Exponential Moving Average (EMA):

```
new_fitness = 0.3 × raw_fitness + 0.7 × previous_fitness
```

This prevents a single bad evaluation from triggering unnecessary mutation.

## Accessing Metrics

### REST API

```bash
# Agent metrics
curl http://localhost:8420/api/agents/{id}/metrics

# Model costs
curl http://localhost:8420/api/costs

# Dashboard aggregates
curl http://localhost:8420/api/dashboard
```

### Web Dashboard

The dashboard displays metrics in several views:
- **Overview** — Aggregated system metrics
- **Agent Detail** — Per-agent metrics with charts
- **Models** — Cost breakdown by model
- **Evolution** — Fitness scores and trends

### MQTT Heartbeat

Edge agents report metrics via MQTT heartbeat:

```json
{
  "type": "heartbeat",
  "metrics": {
    "total_actions": 156,
    "successful_actions": 142,
    "cost_usd": 0.234,
    "custom": {
      "totalTrades": 12,
      "winRate": 0.667,
      "profitLoss": 127.40
    }
  }
}
```

## See Also

- [Evolution Engine](../architecture/evolution.md)
- [REST API](../api/rest-api.md)
- [Genome Format](genome-format.md)
