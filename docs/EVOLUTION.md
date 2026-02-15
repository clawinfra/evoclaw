# EvoClaw Evolution Engine

> *The genome is the soul. Evolution is the journey.* 🧬

---

## Overview

EvoClaw's evolution engine is what makes agents "self-evolving." Every agent has a **genome** — a complete description of what it is and what it can become. The evolution engine explores that genome space, mutating parameters, selecting winning traits, and discarding what doesn't work.

This is **not** traditional machine learning. It's **genetic/evolutionary optimization** — the same approach nature uses. What works survives. What doesn't gets mutated until it does.

---

## The Genome

> *"The Genome is the Soul"* — [PHILOSOPHY.md](PHILOSOPHY.md)

Every agent's genome (`genome` field in config) defines the full space of what that agent can become. Hard boundaries (safety, ethics, risk limits) are walls. Everything else is water — flowing toward fitness.

```json
{
  "genome": {
    "identity": {
      "name": "alpha-trader",
      "persona": "disciplined, data-driven",
      "voice": "concise"
    },
    "skills": {
      "trading": {
        "enabled": true,
        "strategies": ["FundingArbitrage"],
        "params": {
          "funding_threshold": -0.1,
          "exit_funding": 0.0,
          "position_size_usd": 100,
          "max_positions": 3
        }
      },
      "market-monitor": {
        "enabled": true,
        "params": {
          "check_interval_secs": 60,
          "alert_threshold_pct": 5.0
        }
      },
      "evo-lens": {
        "enabled": false
      }
    },
    "behavior": {
      "risk_tolerance": 0.3,
      "verbosity": 0.5,
      "autonomy": 0.8
    },
    "constraints": {
      "max_loss_usd": 500,
      "allowed_assets": ["BTC", "ETH", "SOL"],
      "blocked_actions": ["withdraw"]
    }
  }
}
```

### Genome Layers

| Layer | What Evolves | What Doesn't |
|-------|-------------|-------------|
| **Identity** | Persona tone, communication style | Name, DID, owner |
| **Skills** | Skill parameters, skill selection, strategy configs | Skill code itself (that's a developer update) |
| **Behavior** | Risk tolerance, verbosity, autonomy level | Safety constraints, ethics boundaries |
| **Constraints** | Nothing — constraints are walls | Max loss, blocked actions, allowed assets |

**Skills are the primary unit of evolution.** Trading is just a skill. Monitoring is a skill. Image generation is a skill. Each skill has parameters that the evolution engine can tune.

---

## Evolution Scope

Evolution applies to **any skill**, not just trading. Every skill exposes tunable parameters, and the evolution engine optimizes them based on skill-specific fitness metrics.

### Examples Across Skill Types

| Skill | Evolvable Parameters | Fitness Metric |
|-------|---------------------|----------------|
| **Trading** | Thresholds, position sizes, timing | PnL, Sharpe ratio, win rate |
| **Market Monitor** | Check interval, alert thresholds, sources | Alert accuracy, false positive rate |
| **Companion** | Response length, topic preferences, check-in frequency | User engagement, sentiment |
| **DevOps** | Retry counts, timeout values, escalation thresholds | Uptime, incident response time |
| **Content** | Posting frequency, topic selection, tone | Engagement rate, reach |
| **Evo-Lens** | Style parameters, prompt templates | Image quality score, user approval rate |

The evolution engine doesn't know or care what the skill does — it just sees parameters in, fitness out.

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

Every agent action is tracked by the orchestrator with **universal metrics** that apply to all skills:

| Metric | What It Measures |
|--------|-----------------|
| **Success Rate** | % of actions that succeeded |
| **Avg Response Time** | Latency in milliseconds |
| **Cost (USD)** | API spend per action |
| **Total Actions** | Number of actions taken |
| **Custom Metrics** | Skill-specific (see below) |

Each skill can report its own custom metrics. For example, the **trading skill** reports via `EvolutionTracker`:

| Metric | What It Measures |
|--------|-----------------|
| **Win Rate** | % of profitable trades |
| **Total PnL** | Cumulative profit/loss |
| **Sharpe Ratio** | Risk-adjusted returns (higher = better) |
| **Max Drawdown** | Worst peak-to-trough decline |
| **Avg Profit/Trade** | Mean PnL per trade |

Other skills report different custom metrics — a monitoring skill might report alert accuracy, a companion skill might report user engagement scores.

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

## Skills & Strategies

**Trading is just a skill.** The evolution engine doesn't have special knowledge of trading — it sees skills with parameters and fitness scores. Any skill can evolve.

### Skill Evolution Interface

Every evolvable skill exposes parameters and accepts mutations through a standard interface:

```rust
trait Strategy {
    fn evaluate(&mut self, data: &MarketData) -> Vec<Signal>;
    fn get_params(&self) -> Value;          // Current parameters
    fn update_params(&mut self, params: Value) -> Result<(), String>; // Apply mutation
    fn name(&self) -> &str;
    fn reset(&mut self);
}
```

### Example: Trading Skill (FundingArbitrage)

A trading strategy within the trading skill that exploits negative funding rates:

| Parameter | Default | Evolvable | Description |
|-----------|---------|-----------|-------------|
| `funding_threshold` | -0.1% | ✅ | Enter when funding rate drops below this |
| `exit_funding` | 0.0% | ✅ | Exit when funding rate rises above this |
| `position_size_usd` | $100 | ✅ | USD size per position |
| `max_positions` | 3 | ✅ | Maximum concurrent positions |

**Fitness:** PnL, Sharpe ratio, win rate, drawdown.

### Example: Companion Skill

A companion agent for elderly care:

| Parameter | Default | Evolvable | Description |
|-----------|---------|-----------|-------------|
| `check_in_interval_min` | 120 | ✅ | How often to proactively check in |
| `response_length` | "medium" | ✅ | Short/medium/long responses |
| `topic_weights` | {"health": 0.3, "family": 0.4, "hobbies": 0.3} | ✅ | Conversation topic mix |
| `morning_greeting` | true | ✅ | Whether to send morning greetings |

**Fitness:** User engagement (responses per check-in), sentiment score, session duration.

### Custom Skills

Any skill can participate in evolution by:
1. Exposing parameters via `get_params()`
2. Accepting mutations via `update_params()`
3. Reporting custom metrics to the orchestrator

The evolution engine will automatically track performance and mutate parameters when fitness drops.

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

## Genome Evolution Roadmap

The current implementation covers **Layer 1** (skill parameter mutation). The full genome evolution vision is broader:

### Layer 1: Skill Parameter Mutation ✅ (Current)
- Tune numeric/categorical parameters within a skill
- Fitness-based evaluation and selection
- Single-agent optimization

### Layer 2: Skill Selection & Composition (Planned)
- Agent discovers which skills work best for its tasks
- Enable/disable skills based on fitness contribution
- Skill weight optimization (how much to rely on each skill)
- Example: A monitoring agent evolves to rely more on Twitter skill vs Reddit skill based on alert accuracy

### Layer 3: Behavioral Evolution (Planned)
- System prompt mutation (tone, verbosity, reasoning style)
- Tool usage pattern optimization (which tools, in what order)
- Communication style adaptation to user preferences
- Autonomy level tuning (when to act vs ask)

### Layer 4: Cross-Agent Breeding (Future)
- High-fitness agents share genome traits with low-fitness agents
- Crossover: combine successful parameters from two agents
- Population-level evolution across agent fleets
- Speciation: agents diverge into specialized roles

### Layer 5: Genome Marketplace (Future)
- Share evolved genomes on ClawChain
- Reputation-weighted genome trading
- Agents can "buy" proven genome configurations from successful agents
- On-chain fitness verification

```
Current                                              Future
Layer 1                                              Layer 5
│                                                         │
│  📊 Param     🧩 Skill      🎭 Behavior   🧬 Breeding  🏪 Market │
│  Tuning      Selection    Evolution    Crossover    Genome    │
│  ✅           🔜           🔜           🔮           🔮        │
│                                                         │
└──────────── Same genome format throughout ─────────────┘
```

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

---

## Agent Reliability Patterns

These patterns address the most common failure modes in long-running agent systems.

### WAL Protocol (Write-Ahead Log)

The #1 agent failure mode is **forgetting corrections mid-conversation**. The WAL protocol requires agents to write state changes to persistent storage *before* acting on them.

**How it works:**
- Before an agent responds or executes an action, it appends an entry to the WAL
- Each entry has: `timestamp`, `agent_id`, `action_type` (correction/decision/state_change), `payload`, `applied` flag
- On restart, unapplied WAL entries are replayed to restore state
- Both Go orchestrator (`internal/wal/`) and Rust edge agent (`edge-agent/src/wal.rs`) implement this

**Integration with evolution:** Mutations and fitness evaluations are WAL-logged so a crash mid-evolution doesn't leave the genome in an inconsistent state.

### Working Buffer / Compaction Danger Zone

When the memory system compacts older context, there's a danger zone where recent corrections can be lost. The Working Buffer prevents this.

**How it works:**
- Critical state (corrections, recent decisions) is held in a `WorkingBuffer`
- Before any memory compaction, `FlushToWAL()` is called to persist the buffer
- After compaction, the WAL entries ensure nothing critical was lost
- The buffer is cleared after a successful flush

### VBR (Verify Before Reporting)

When a mutation claims improvement, **verify it actually works** before recording it as successful. This prevents false-positive evolution where noise is mistaken for signal.

**How it works:**
- After `MutateSkill()`, call `VerifyMutation(agentID, skillName, metrics)`
- Re-evaluates fitness with fresh metrics and compares pre/post
- Only commits the mutation to genome history if verified as an improvement
- Unverified mutations can be reverted automatically
- The `verified` field on Strategy and SkillGenome tracks verification status

### ADL/VFM Guardrails

**Anti-Divergence Limit (ADL):** Prevents agents from evolving into unrecognizable complexity monsters.
- Tracks cumulative mutation distance via `DivergenceScore(agentID)`
- If `DivergenceScore > GenomeConstraints.MaxDivergence`, forces a simplification pass
- `CheckADL()` returns true when the limit is exceeded

**Value-For-Money (VFM) Scoring:** Ensures mutations are worth their cost.
- Score = `fitness_improvement / (token_cost_increase + latency_increase + param_count_increase)`
- Mutations with `VFM < GenomeConstraints.MinVFMScore` are rejected
- `EvaluateVFM()` returns a full breakdown

**Integration with the 5-layer roadmap:**
- **Layer 1 (Parameter Tuning):** WAL + VBR ensure parameter mutations are persisted and verified
- **Layer 2 (Skill Selection):** ADL prevents skill composition from growing unbounded
- **Layer 3 (Behavioral Evolution):** VFM ensures behavioral changes justify their cost
- **Layer 4 (Cross-Agent):** WAL provides crash recovery for multi-agent coordination
- **Layer 5 (Autonomous):** All guardrails combined prevent runaway self-modification
