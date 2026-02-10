# EvoClaw Evolution Engine

> *Agents that improve themselves. Automatically.* 🧬

---

## Overview

EvoClaw's evolution engine is what makes it "self-evolving" — agents automatically measure their own performance, and when they underperform, the system mutates their strategy parameters to find better configurations. No human intervention required.

This is **not** traditional machine learning. It's **genetic/evolutionary optimization** — the same approach nature uses. Strategies that work survive. Strategies that don't get mutated until they do.

```
┌─────────────────────────────────────────────────────────┐
│                  Evolution Loop                          │
│                                                          │
│   ┌──────────┐     ┌──────────┐     ┌──────────┐       │
│   │  Agent    │────▶│ Evaluate │────▶│  Decide  │       │
│   │ performs  │     │ fitness  │     │  evolve? │       │
│   │ actions   │     │          │     │          │       │
│   └──────────┘     └──────────┘     └────┬─────┘       │
│                                      YES │  NO          │
│   ┌──────────┐     ┌──────────┐     ┌────▼─────┐       │
│   │  Reset   │◀────│  Mutate  │◀────│  Below   │       │
│   │ metrics  │     │ strategy │     │threshold │       │
│   └──────────┘     └──────────┘     └──────────┘       │
│        │                                                 │
│        └──────────── repeat ─────────────────────────▶  │
└─────────────────────────────────────────────────────────┘
```

---

## How It Works

### 1. Performance Tracking

Every agent action is tracked by the orchestrator:

| Metric | What It Measures |
|--------|-----------------|
| **Success Rate** | % of actions that succeeded |
| **Avg Response Time** | Latency in milliseconds |
| **Cost (USD)** | API spend per action |
| **Total Actions** | Number of actions taken |
| **Custom Metrics** | Strategy-specific (e.g., PnL, win rate) |

For trading agents, the Rust edge agent tracks additional metrics via `EvolutionTracker`:

| Metric | What It Measures |
|--------|-----------------|
| **Win Rate** | % of profitable trades |
| **Total PnL** | Cumulative profit/loss |
| **Sharpe Ratio** | Risk-adjusted returns (higher = better) |
| **Max Drawdown** | Worst peak-to-trough decline |
| **Avg Profit/Trade** | Mean PnL per trade |

### 2. Fitness Score

Every evaluation cycle, the engine calculates a **fitness score** (0–100) for each agent using a weighted function:

```
Fitness = (Sharpe Score × 40%) + (Win Rate × 30%) + (PnL Score × 20%) + (Drawdown Penalty × 10%)
```

| Component | Weight | Calculation |
|-----------|--------|------------|
| **Sharpe Score** | 40% | `min(sharpe_ratio / 3.0, 1.0) × 40` |
| **Win Rate** | 30% | `(win_rate / 100) × 30` |
| **PnL Score** | 20% | `clamp(total_pnl / 10000, 0, 1) × 20` |
| **Drawdown Penalty** | 10% | `(1.0 - min(max_drawdown / 5000, 1)) × 10` |

A fitness of **60+** is considered acceptable. Below that triggers evolution.

### 3. Evolution Decision

The orchestrator runs an evaluation loop at a configurable interval (default: every 3600 seconds / 1 hour):

```go
// Pseudocode
for each agent:
    if agent.totalActions < minSamplesForEval:
        skip  // Not enough data yet
    
    fitness = evolution.Evaluate(agent, metrics)
    
    if fitness < 0.6:  // Below threshold
        agent.status = "evolving"
        evolution.Mutate(agent, mutationRate)
        agent.resetMetrics()  // Fresh start with new params
```

**Key safeguards:**
- Agents need a **minimum number of actions** before evaluation (default: 10)
- Metrics are **reset after mutation** so the new strategy gets a fair evaluation
- Evolution events are **synced to cloud** (Turso) for history tracking

### 4. Strategy Mutation

When an agent needs to evolve, the engine mutates its strategy parameters:

```rust
// Example: FundingArbitrage strategy mutation
Before: { funding_threshold: -0.1, exit_funding: 0.0, position_size: 100 }
After:  { funding_threshold: -0.15, exit_funding: 0.02, position_size: 120 }
```

The mutation rate controls how much parameters change (0.0 = no change, 1.0 = maximum randomization). The default `maxMutationRate` is conservative to avoid wild swings.

**What gets mutated:**
- Strategy parameters (thresholds, sizes, limits)
- NOT the strategy type itself (FundingArbitrage stays FundingArbitrage)
- NOT the agent's identity or memory

### 5. The Cycle Repeats

After mutation:
1. Metrics reset to zero
2. Agent resumes with new parameters
3. After enough actions, it's evaluated again
4. If fitness improves → keep parameters
5. If fitness still low → mutate again

Over time, agents converge on parameter configurations that maximize their fitness score.

---

## Strategies

Strategies are pluggable modules that define how an agent makes decisions. Each strategy implements the `Strategy` trait:

```rust
trait Strategy {
    fn evaluate(&mut self, data: &MarketData) -> Vec<Signal>;
    fn get_params(&self) -> Value;
    fn update_params(&mut self, params: Value) -> Result<(), String>;
    fn name(&self) -> &str;
    fn reset(&mut self);
}
```

### Built-in: FundingArbitrage

A trading strategy that exploits negative funding rates on perpetual futures:

| Parameter | Default | Description |
|-----------|---------|-------------|
| `funding_threshold` | -0.1% | Enter when funding rate drops below this |
| `exit_funding` | 0.0% | Exit when funding rate rises above this |
| `position_size_usd` | $100 | USD size per position |
| `max_positions` | 3 | Maximum concurrent positions |

**Logic:** When traders are heavily short (negative funding), longs get paid. The strategy goes long to collect funding payments, then exits when funding normalizes.

### Custom Strategies

Add your own strategy by implementing the `Strategy` trait. The evolution engine will automatically track its performance and mutate its parameters.

---

## Configuration

In `evoclaw.json`:

```json
{
  "evolution": {
    "enabled": true,
    "evalIntervalSec": 3600,
    "minSamplesForEval": 10,
    "maxMutationRate": 0.3
  }
}
```

| Setting | Default | Description |
|---------|---------|-------------|
| `enabled` | `true` | Enable/disable evolution engine |
| `evalIntervalSec` | `3600` | How often to evaluate agents (seconds) |
| `minSamplesForEval` | `10` | Minimum actions before first evaluation |
| `maxMutationRate` | `0.3` | Maximum parameter mutation rate (0.0–1.0) |

### CLI Control

```bash
# Check evolution status
evoclaw status

# View agent fitness
evoclaw agents list    # Shows fitness scores

# Manually trigger evaluation
evoclaw evolve --agent <agent-id>

# Disable evolution temporarily
evoclaw config set evolution.enabled false
```

---

## Dashboard (Orchestrator GUI)

The orchestrator's web dashboard (`http://localhost:8420`) includes an **Evolution** page accessible from the sidebar navigation.

### Evolution Overview

The top of the page shows four summary cards:

| Card | Description |
|------|-------------|
| **Evolving Agents** | Number of agents currently undergoing mutation |
| **Total Mutations** | Cumulative mutations across all agents |
| **Avg Fitness** | Mean fitness score across all agents |
| **Evolution Status** | Active (green) or disabled |

### Fitness Trends Chart

A line chart showing fitness scores over time for all agents. This is the key visualization — you can see whether agents are improving, plateauing, or struggling.

```
Fitness
1.0 ┤
    │                          ╭──────── Agent A (converged)
0.8 ┤                    ╭─────╯
    │              ╭─────╯
0.6 ┤─ ─ ─ ─ ─ ─ ─╫─ ─ ─ ─ ─ ─ ─ ─ threshold
    │        ╭─────╯
0.4 ┤  ╭─────╯          ╭──── Agent B (improving)
    │  │           ╭────╯
0.2 ┤──╯      ╭────╯
    │    ╭────╯
0.0 ┼────╯
    └──────────────────────────────── Time
      Gen 1    Gen 5    Gen 10   Gen 15
```

### Agent Evolution Table

A table showing each agent's evolution status:

| Column | Description |
|--------|-------------|
| **Agent** | Agent name/ID |
| **Strategy** | Current strategy name (e.g., FundingArbitrage) |
| **Version** | Strategy version (increments on each mutation) |
| **Fitness** | Current fitness score (color-coded: green > 0.6, yellow 0.3–0.6, red < 0.3) |
| **Evaluations** | Number of evaluation cycles completed |
| **Temperature** | Current mutation temperature (higher = more exploration) |
| **Last Mutation** | Timestamp of most recent mutation |

### Agent Detail View

Click any agent to see its detail panel, which includes:

- **Evolution History** — Chart showing fitness over generations
- **Current Strategy Parameters** — Live view of all strategy params
- **Temperature** — Current mutation temperature
- **Fitness Score** — Current fitness with breakdown
- **Version** — Strategy generation number

### Mutation Timeline

A chronological timeline of all mutations across agents:

```
┌─────────────────────────────────────────────────┐
│ 🧬 Mutation Timeline                             │
│                                                   │
│ ● 09:15 — Agent "alpha-trader" mutated (v3→v4)  │
│   funding_threshold: -0.10 → -0.15               │
│   Fitness: 0.42 → evaluating...                  │
│                                                   │
│ ● 08:15 — Agent "beta-monitor" mutated (v2→v3)  │
│   position_size: 100 → 120                        │
│   Fitness: 0.55 → 0.68 ✅                        │
│                                                   │
│ ● 07:15 — Agent "alpha-trader" mutated (v2→v3)  │
│   exit_funding: 0.00 → 0.02                      │
│   Fitness: 0.38 → 0.42                           │
└─────────────────────────────────────────────────┘
```

---

## Evolution vs Traditional ML

| Aspect | EvoClaw Evolution | Traditional ML |
|--------|------------------|----------------|
| **Training data** | Live performance (no historical dataset needed) | Requires curated training data |
| **Approach** | Genetic mutation + selection | Gradient descent / backpropagation |
| **What changes** | Strategy parameters | Model weights |
| **Compute** | Minimal (parameter tweaking) | Heavy (GPU training) |
| **Feedback loop** | Real-time, continuous | Batch training → deploy → retrain |
| **Interpretability** | High (you can read the parameters) | Low (black box weights) |
| **Risk** | Controlled (mutation rate limits change) | Overfitting, catastrophic forgetting |

---

## Architecture (Code)

### Orchestrator Side (Go)

```
internal/orchestrator/orchestrator.go
├── EvolutionEngine interface    — Pluggable evolution backend
├── evolutionLoop()              — Periodic evaluation ticker
├── evaluateAgents()             — Runs fitness evaluation on all agents
└── evolveAgent()                — Triggers mutation + metric reset
```

### Edge Agent Side (Rust)

```
edge-agent/src/
├── evolution.rs     — EvolutionTracker (trade recording, fitness calculation)
├── strategy.rs      — Strategy trait + FundingArbitrage implementation
├── metrics.rs       — Performance metric collection
└── agent.rs         — Agent lifecycle (receives strategy updates via MQTT)
```

### Data Flow

```
Edge Agent                    Orchestrator
    │                              │
    │ ── trade results ──────────▶ │
    │    (via MQTT)                │
    │                              │── evaluate fitness
    │                              │── fitness < threshold?
    │                              │── mutate parameters
    │                              │
    │ ◀── strategy update ──────── │
    │    (via MQTT)                │
    │                              │
    │── apply new params           │
    │── reset metrics              │
    │── resume trading             │
```

---

## Cloud Persistence

Evolution history is synced to Turso cloud storage:

- **Every mutation** is recorded with before/after parameters
- **Fitness trends** are persisted across restarts
- **Strategy versions** are tracked (can roll back to previous generation)
- If a device is lost, evolution history survives in the cloud

---

## FAQ

**Q: Can evolution make my agent worse?**
A: Temporarily, yes — a mutation might reduce performance. But the next evaluation cycle will detect this and mutate again. Over time, the system converges toward better configurations. The mutation rate is capped to prevent catastrophic changes.

**Q: How long until my agent improves?**
A: Depends on `evalIntervalSec` and `minSamplesForEval`. With defaults (1 hour eval, 10 minimum actions), you'll see the first evolution within a few hours. Meaningful improvement typically takes 5–15 generations.

**Q: Can I manually set strategy parameters?**
A: Yes. Edit the strategy params in the config or via the dashboard. The evolution engine will use your manual settings as the starting point and evolve from there.

**Q: Does evolution work for non-trading agents?**
A: Yes. The `EvolutionEngine` interface accepts any metrics (success rate, response time, cost). Trading-specific metrics (PnL, Sharpe) are only used by the `EvolutionTracker` in trading strategies.

**Q: Can I disable evolution for a specific agent?**
A: Not currently per-agent — it's a global setting. You can set `minSamplesForEval` very high for agents you don't want to evolve, or set `evolution.enabled: false` globally.

**Q: What happens during evolution? Does the agent stop?**
A: The agent's status changes to "evolving" briefly while parameters are mutated, then immediately resumes with the new configuration. There's no downtime — it's a hot parameter swap.

---

*Last updated: 2026-02-11*
