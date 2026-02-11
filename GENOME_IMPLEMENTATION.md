# Genome Architecture Implementation

## Summary

Successfully implemented the complete genome architecture for EvoClaw as specified in `docs/EVOLUTION.md`. The genome system provides a typed, structured representation of an agent's complete genetic makeup, enabling per-skill evolution and fine-grained parameter optimization.

## What Was Implemented

### 1. Core Genome Structures (Go)

**File:** `internal/genome/genome.go`

Created typed genome structures:
- `Genome` - Complete genetic makeup
- `GenomeIdentity` - Agent identity (name, persona, voice)
- `SkillGenome` - Per-skill evolvable parameters
- `GenomeBehavior` - Behavioral traits (risk tolerance, verbosity, autonomy)
- `GenomeConstraints` - Hard boundaries (max loss, allowed assets, blocked actions)

**Features:**
- Full JSON serialization/deserialization
- Validation methods
- Legacy conversion (map[string]interface{} ↔ typed Genome)
- Clone, GetSkill, SetSkill, EnabledSkills methods

**Tests:** `internal/genome/genome_test.go`
- Default genome validation
- Genome validation (bounds checking)
- Genome cloning
- Skill operations
- JSON serialization
- Legacy format conversion

### 2. Config Integration

**File:** `internal/config/config.go`

Updated `AgentDef` to use typed `Genome`:
```go
// Before
Genome map[string]interface{} `json:"genome,omitempty"`

// After
Genome *Genome `json:"genome,omitempty"`
```

Re-exported genome types in config package for convenience.

### 3. Per-Skill Evolution Engine

**File:** `internal/evolution/engine.go`

Extended evolution engine with skill-level methods:

- `GetGenome(agentID)` - Retrieve agent's genome from disk
- `UpdateGenome(agentID, genome)` - Persist genome to disk
- `EvaluateSkill(agentID, skillName, metrics)` - Evaluate specific skill fitness
- `MutateSkill(agentID, skillName, mutationRate)` - Mutate skill parameters
- `ShouldEvolveSkill(agentID, skillName, minFitness, minSamples)` - Evolution decision per skill

**Key Changes:**
- Evolution now works per-skill instead of per-agent
- Each skill has its own fitness score and version
- Mutation resets skill fitness for re-evaluation
- Supports float, int, and bool parameter mutations

### 4. Orchestrator Updates

**File:** `internal/orchestrator/orchestrator.go`

Updated orchestrator for per-skill evolution:

- `evaluateAgents()` - Now iterates through each enabled skill
- `getSkillMetrics()` - Extracts skill-specific metrics (with skill name prefix)
- `evolveSkill()` - Performs evolution on a single skill
- Backward compatible with legacy agent-level evolution

**Evolution Flow:**
```
for each agent:
    for each skill in genome.skills:
        if skill.enabled && has_enough_samples:
            fitness = evaluate(skill)
            if fitness < threshold:
                mutate(skill.params)
                skill.version++
                skill.fitness = 0 (reset)
```

### 5. Rust Genome Structures

**File:** `edge-agent/src/genome.rs`

Rust genome structures matching Go implementation:

```rust
pub struct Genome {
    pub identity: GenomeIdentity,
    pub skills: HashMap<String, SkillGenome>,
    pub behavior: GenomeBehavior,
    pub constraints: GenomeConstraints,
}
```

**Features:**
- Serde serialization
- Validation methods
- Typed parameter getters (get_f64, get_i64, get_string, get_bool)
- update_skill_params() for evolution
- Comprehensive unit tests

### 6. Rust Genome Update Handler

**File:** `edge-agent/src/commands.rs`

Added `handle_update_genome` command:
- Parses genome from MQTT message
- Validates genome structure
- Updates strategy engine from genome skills
- Reports updated genome status

### 7. API Endpoints

**File:** `internal/api/genome.go`

New REST API endpoints:

| Method | Endpoint | Description |
|--------|----------|-------------|
| GET | `/api/agents/{id}/genome` | Get agent's complete genome |
| PUT | `/api/agents/{id}/genome` | Update agent's genome |
| GET | `/api/agents/{id}/genome/skills/{skill}` | Get specific skill genome |
| PUT | `/api/agents/{id}/genome/skills/{skill}/params` | Update skill parameters |

**Features:**
- Full genome CRUD operations
- Per-skill parameter updates
- Validation of behavior bounds (0.0-1.0)
- Returns 404 for non-existent agents/skills

### 8. Web Dashboard

**Files:** `web/index.html`, `web/app.js`

Enhanced agent detail view with genome display:

**Genome Display Sections:**

1. **Identity** (🎭)
   - Name, Persona, Voice

2. **Behavior** (🎨)
   - Risk Tolerance (progress bar)
   - Verbosity (progress bar)
   - Autonomy (progress bar)

3. **Skills** (🧩)
   - Per-skill cards showing:
     - Enabled/disabled status
     - Version number
     - Fitness score
     - All parameters

4. **Constraints** (🔒)
   - Max Loss USD
   - Allowed Assets
   - Blocked Actions

**JavaScript Changes:**
- Added `agentGenome` to dashboard state
- Auto-fetch genome when viewing agent detail
- Fitness color coding (green > 0.6, yellow 0.3-0.6, red < 0.3)

## Architecture Decisions

### 1. Skill as Unit of Evolution

Instead of evolving the entire agent, we evolve individual skills. This allows:
- Fine-grained optimization
- Parallel evolution of different capabilities
- Skill-specific fitness metrics
- Independent version tracking

### 2. Genome Persistence

Genomes are stored separately from strategy configs:
- `{agent_id}-genome.json` in evolution data directory
- Allows evolution history tracking
- Enables rollback to previous genome versions

### 3. Backward Compatibility

The implementation maintains backward compatibility:
- Legacy `map[string]interface{}` genomes can be converted
- Orchestrator falls back to agent-level evolution if skill evolution unavailable
- Existing agents without genomes get default genome

### 4. Separation of Concerns

Clear separation between:
- **Genome** (structure) - What the agent *is*
- **Strategy** (behavior) - What the agent *does*
- **Metrics** (feedback) - How well it *performs*

## Testing

### Go Tests

```bash
go test ./internal/genome/...
go test ./internal/evolution/...
go test ./internal/api/...
```

### Rust Tests

```bash
cargo test --package evoclaw-edge-agent genome::tests
```

## Usage Example

### 1. Define Genome in Config

```json
{
  "agents": [
    {
      "id": "trader-001",
      "name": "Alpha Trader",
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
            },
            "fitness": 0.0,
            "version": 1
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
  ]
}
```

### 2. Evolution Cycle

```
1. Agent performs actions (trades, monitoring, etc.)
2. Orchestrator collects skill-specific metrics
3. Every eval_interval_sec:
   - For each enabled skill:
     - Calculate fitness score
     - If fitness < threshold && enough samples:
       - Mutate skill parameters
       - Increment skill.version
       - Reset skill.fitness
4. Agent continues with new parameters
5. Repeat
```

### 3. API Usage

```bash
# Get genome
curl http://localhost:8420/api/agents/trader-001/genome

# Update skill params
curl -X PUT http://localhost:8420/api/agents/trader-001/genome/skills/trading/params \
  -H "Content-Type: application/json" \
  -d '{"funding_threshold": -0.15, "position_size_usd": 120}'

# Get specific skill
curl http://localhost:8420/api/agents/trader-001/genome/skills/trading
```

## Future Enhancements

Based on docs/EVOLUTION.md roadmap:

### Layer 2: Skill Selection & Composition
- Auto-enable/disable skills based on fitness contribution
- Skill weight optimization
- Skill dependency resolution

### Layer 3: Behavioral Evolution
- System prompt mutation
- Tool usage pattern optimization
- Communication style adaptation

### Layer 4: Cross-Agent Breeding
- Genome crossover between high-fitness agents
- Population-level evolution
- Speciation

### Layer 5: Genome Marketplace
- Share evolved genomes on ClawChain
- On-chain fitness verification
- Reputation-weighted genome trading

## Files Changed

```
Modified:
- internal/config/config.go
- internal/evolution/engine.go
- internal/orchestrator/orchestrator.go
- internal/api/server.go
- edge-agent/src/lib.rs
- edge-agent/src/commands.rs
- web/index.html
- web/app.js

New Files:
- internal/genome/genome.go
- internal/genome/genome_test.go
- internal/api/genome.go
- edge-agent/src/genome.rs
```

## Verification

To verify the implementation:

1. **Start orchestrator:**
   ```bash
   evoclaw start
   ```

2. **Check dashboard:**
   - Visit http://localhost:8420
   - Navigate to agent detail
   - Verify genome display

3. **Test API:**
   ```bash
   curl http://localhost:8420/api/agents/{id}/genome
   ```

4. **Monitor evolution:**
   - Navigate to Evolution page
   - Verify per-skill fitness tracking
   - Trigger manual mutation

## Notes

- All genome data is JSON serializable
- Genome files are stored in `{dataDir}/evolution/{agent_id}-genome.json`
- Evolution events are synced to cloud if enabled
- Web dashboard auto-refreshes genome every 30 seconds

## Commit

```
git commit -m "feat: implement genome architecture for EvoClaw"
git push origin main
```

Status: ✅ **Implementation Complete**
