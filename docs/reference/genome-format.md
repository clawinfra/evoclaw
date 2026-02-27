# Genome / Strategy Format

The "genome" defines an agent's evolvable strategy — the parameters that the evolution engine can mutate to improve performance.

## Orchestrator Strategy (JSON)

Stored in `data/evolution/{agent_id}.json`:

```json
{
  "id": "eth-trader-v3",
  "agentId": "eth-trader",
  "version": 3,
  "createdAt": "2026-02-06T10:30:00Z",
  "systemPrompt": "You are a cautious crypto trader...",
  "preferredModel": "anthropic/claude-sonnet-4-20250514",
  "fallbackModel": "openai/gpt-4o",
  "temperature": 0.65,
  "maxTokens": 4096,
  "params": {
    "minFundingRate": 0.0015,
    "positionSizePct": 0.12,
    "stopLossPct": 0.03,
    "takeProfitPct": 0.05,
    "maxHoldTimeSec": 3600
  },
  "fitness": 0.72,
  "evalCount": 24
}
```

### Fields

| Field | Type | Evolvable | Description |
|-------|------|-----------|-------------|
| `id` | string | No | Strategy ID (format: `{agentId}-v{version}`) |
| `agentId` | string | No | Owning agent |
| `version` | int | No | Increments on mutation |
| `createdAt` | string | No | ISO 8601 timestamp |
| `systemPrompt` | string | Future | LLM system prompt |
| `preferredModel` | string | Future | Primary model preference |
| `fallbackModel` | string | Future | Fallback model |
| `temperature` | float | **Yes** | LLM temperature (0.0–2.0) |
| `maxTokens` | int | No | Response length limit |
| `params` | map | **Yes** | Custom evolvable parameters |
| `fitness` | float | No | Current fitness score (0.0–1.0) |
| `evalCount` | int | No | Number of evaluations |

### Custom Parameters

The `params` map holds strategy-specific evolvable values. These are the primary targets for mutation.

#### Trading Agent Parameters

| Parameter | Default | Range | Description |
|-----------|---------|-------|-------------|
| `minFundingRate` | 0.001 | 0.0001–0.01 | Min funding rate for entry |
| `positionSizePct` | 0.1 | 0.01–0.5 | Fraction of max position |
| `stopLossPct` | 0.03 | 0.01–0.1 | Stop loss threshold |
| `takeProfitPct` | 0.05 | 0.02–0.2 | Take profit threshold |
| `maxHoldTimeSec` | 3600 | 300–86400 | Maximum position hold time |
| `lookbackPeriod` | 20 | 5–100 | Rolling average window |
| `stdDevThreshold` | 2.0 | 0.5–5.0 | Entry signal threshold |

#### Orchestrator Agent Parameters

| Parameter | Default | Range | Description |
|-----------|---------|-------|-------------|
| `verbosity` | 0.5 | 0.1–1.0 | Response verbosity preference |
| `creativityBias` | 0.5 | 0.0–1.0 | Creative vs factual balance |

## Edge Agent Style (TOML)

Edge agents can define their trading style in a TOML file:

```toml
# style.toml - Trading personality genome

[identity]
name = "Cautious Ethena"
strategy_family = "funding_arbitrage"
risk_profile = "conservative"

[parameters]
# Entry conditions
min_funding_rate = 0.001
entry_confidence_threshold = 0.7

# Position sizing
max_position_pct = 0.1        # 10% of max allowed
scale_in_steps = 3             # Enter in 3 tranches

# Exit conditions
stop_loss_pct = 0.03           # 3% stop loss
take_profit_pct = 0.05         # 5% take profit
max_hold_time_hours = 24       # Close after 24h regardless
trailing_stop_pct = 0.02       # 2% trailing stop

# Timing
check_interval_secs = 60       # Check market every 60s
cooldown_after_loss_secs = 300  # Wait 5min after a loss

[evolution]
# Boundaries that cannot be exceeded
max_leverage = 5.0
max_position_usd = 5000.0
min_balance_usd = 100.0        # Never trade below this

[personality]
# These affect how the agent communicates about its trades
reporting_verbosity = "concise"
risk_commentary = true
```

## Version History

The evolution engine maintains a history of strategies:

```
Version 1: Initial strategy (default parameters)
Version 2: Temperature reduced from 0.7 → 0.65 (fitness improved)
Version 3: positionSizePct increased 0.10 → 0.12 (more aggressive)
Version 4: Reverted to v3 (v4 performed worse)
```

History is stored in memory and used for reversion decisions.

## Evolution Constraints

Hard boundaries that evolution cannot exceed:

1. **Temperature**: 0.0–2.0
2. **Custom params**: -1000 to 1000 (general bounds)
3. **Strategy-specific**: Defined in `style.toml` or agent config
4. **Risk limits**: `max_leverage`, `max_position_usd` are immutable

## See Also

- [Evolution Engine](../architecture/evolution.md)
- [Custom Strategy Guide](../guides/custom-strategy.md)
- [Metrics Reference](metrics.md)
