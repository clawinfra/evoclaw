# Architecture — EvoClaw

## Layer Diagram

```
┌──────────────────────────────────────────────────────────────────┐
│  cmd/                                                             │
│  (binary entry points — wires everything together)               │
│                                                                   │
│  cmd/evoclaw → internal/orchestrator → internal/agents           │
│             → internal/config                                     │
│             → internal/channels                                   │
│             → internal/scheduler                                  │
├──────────────────────────────────────────────────────────────────┤
│  internal/                                                        │
│  (private business logic — not importable by external packages)   │
│                                                                   │
│  orchestrator → agents → interfaces (Channel, Memory, Provider)  │
│              → skillbank                                          │
│              → models (LLM providers)                             │
│              → router                                             │
│                                                                   │
│  skillbank → [distiller → retriever → injector → updater]        │
│  evolution → genome                                               │
│  rsi       → [observer → analyzer → fixer]                       │
│  onchain   → clawchain                                            │
├──────────────────────────────────────────────────────────────────┤
│  pkg/                                                             │
│  (public libraries — safe for external import)                    │
│  (currently minimal — grow as stable APIs emerge)                 │
├──────────────────────────────────────────────────────────────────┤
│  edge-agent/ (Rust, separate crate — IoT/embedded targets)        │
└──────────────────────────────────────────────────────────────────┘
```

### The Layer Rule (ABSOLUTE)

```
ALLOWED:    cmd → internal → pkg
ALLOWED:    cmd → pkg
FORBIDDEN:  internal → cmd     (reverse dependency)
FORBIDDEN:  pkg → internal     (breaks public API isolation)
FORBIDDEN:  pkg → cmd          (reverse dependency)
```

Violation = compile error + `scripts/agent-lint.sh` failure.

If you need to share code between `internal/` packages and `cmd/`, the code belongs in `pkg/`.
If you need to share code between two `internal/` packages, put it in a third `internal/` package
that neither imports the other.

---

## Key Package Responsibilities

### `internal/orchestrator`

The core session manager. Owns:
- Agent session lifecycle (create, resume, destroy)
- Tool loop execution (LLM → tools → LLM → ...)
- Parallel tool dispatch
- Message inbox management
- RSI logger integration

**Primary types:** `Orchestrator`, `Session`, `ToolLoop`
**Interfaces it uses:** `Provider` (LLM), `Tool`, `Memory`, `Channel`

### `internal/agents`

Agent state management. Owns:
- Conversation history and compaction
- Agent persona and config
- Context injection (memory, skills, WAL)
- Message routing between sessions

**Does NOT own:** LLM calls (that's orchestrator), channel I/O (that's channels package)

### `internal/skillbank`

The SKILLRL pipeline — extracts reusable skills from task trajectories.

```
Observer records trajectory
       ↓
Distiller batch-processes → skill candidates (LLM call)
       ↓
Retriever deduplicates → checks existing skill store
       ↓
Injector formats → adds to agent context on next session
       ↓
Updater maintains → skill quality scores, pruning
```

**Key files:** `distiller.go`, `retriever.go`, `injector.go`, `updater.go`, `store.go`

### `internal/evolution`

Agent genome evolution engine. Owns:
- Applying mutations to agent genomes
- Firewall (prevents harmful mutations)
- Fitness evaluation

### `internal/genome`

Genome encoding/decoding. Owns:
- Serialisation of agent personality/capability descriptors
- Mutation and crossover operations (genetic algorithm primitives)
- Genome validation

### `internal/rsi`

Recursive Self-Improvement loop. Owns:
- `observer.go` — records task outcomes (success/fail/quality)
- `analyzer.go` — finds patterns in failures
- `fixer.go` — generates improvement proposals (new skills, routing fixes, SOUL.md patches)
- `loop.go` — orchestrates the observe→analyze→fix cycle

### `internal/interfaces`

Core interface definitions. **This package has no dependencies on other internal packages.**

```go
// interfaces/provider.go
type Provider interface {
    Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
    Stream(ctx context.Context, req CompletionRequest) (<-chan StreamChunk, error)
}

// interfaces/tool.go
type Tool interface {
    Name() string
    Description() string
    Execute(ctx context.Context, params json.RawMessage) (json.RawMessage, error)
}

// interfaces/memory.go
type Memory interface {
    Store(ctx context.Context, key string, value []byte) error
    Retrieve(ctx context.Context, query string, limit int) ([]MemoryEntry, error)
}
```

All inter-package dependencies go through interfaces. **Never depend on a concrete struct
from another package** — always define and depend on an interface.

### `internal/router`

Intelligent model routing. Classifies tasks into tiers (SIMPLE/MEDIUM/COMPLEX/REASONING/CRITICAL)
and maps them to the appropriate LLM provider. Used by orchestrator before spawning sub-agents.

---

## Dependency Injection Pattern

EvoClaw uses constructor-based dependency injection. No `init()` functions with side effects,
no package-level variables that are mutated at runtime.

```go
// ✅ CORRECT — inject dependencies through constructor
type Orchestrator struct {
    provider Provider        // interface, not concrete type
    memory   Memory          // interface
    tools    []Tool          // interface slice
    skillbank *skillbank.SkillBank
}

func New(provider Provider, memory Memory, tools []Tool, sb *skillbank.SkillBank) *Orchestrator {
    return &Orchestrator{provider: provider, memory: memory, tools: tools, skillbank: sb}
}

// ❌ WRONG — package-level mutable state
var globalProvider Provider   // shared mutable state = test pollution + race conditions
var globalMemory Memory
```

---

## SKILLRL Pipeline (detail)

The Skill Reinforcement Learning pipeline extracts durable knowledge from agent task traces.

```
1. Observer (internal/orchestrator/rsi)
   - Wraps each tool call + result as a trajectory step
   - Records success/failure, latency, token cost

2. Distiller (internal/skillbank/distiller.go)
   - Batches trajectories (N=10)
   - Calls LLM to extract "what principle made this work/fail?"
   - Output: SkillCandidate{title, principle, when_to_apply, confidence}

3. Retriever (internal/skillbank/retriever.go)
   - Deduplicates against existing skill store (embedding similarity)
   - Merges near-duplicate skills (reinforces existing vs. creating noise)

4. Injector (internal/skillbank/injector.go)
   - Selects top-K skills by relevance for the current task
   - Formats them for injection into agent system prompt

5. Updater (internal/skillbank/updater.go)
   - Tracks skill usage and outcome
   - Adjusts confidence scores
   - Prunes low-performing skills
```

---

## Testing Architecture

Tests use the real package types but with injected mock dependencies.
See `docs/QUALITY.md` for mocking patterns.

```
internal/<package>/
  <package>.go              ← implementation
  <package>_test.go         ← unit tests (same package, access unexported)
  <package>_coverage_test.go ← additional tests to hit coverage targets
  mock_<dep>.go             ← mock implementations (only in test files or _test packages)
```
