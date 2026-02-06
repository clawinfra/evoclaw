# Architecture Decision Records

Key technical decisions made during EvoClaw development.

---

## ADR-001: Go + Rust Split Architecture

**Date:** 2026-01-15
**Status:** Accepted

### Context

Need a framework that runs on both cloud servers and tiny edge devices.

### Decision

- **Go** for the orchestrator — simple, fast compilation, great stdlib, easy to deploy
- **Rust** for edge agents — tiny binaries, no runtime, memory safe, cross-compilation

### Consequences

- Two codebases to maintain
- MQTT bridge needed for communication
- Can optimize each component for its environment
- Go orchestrator: 6.9MB, Rust agent: 3.2MB

---

## ADR-002: MQTT for Agent Communication

**Date:** 2026-01-16
**Status:** Accepted

### Context

Need reliable, lightweight messaging between orchestrator and edge agents, potentially over unreliable networks.

### Decision

Use MQTT v3.1.1 with Mosquitto broker.

### Alternatives Considered

- **gRPC** — Too heavy for constrained devices, requires HTTP/2
- **WebSocket** — Good but MQTT has better IoT tooling and QoS levels
- **Redis Pub/Sub** — Requires Redis server, less suited for edge networks
- **NATS** — Great but less mature on embedded targets

### Consequences

- Lightweight protocol (small header overhead)
- QoS levels for reliability
- Wide client library support (Go, Rust, C, Python)
- Requires MQTT broker deployment

---

## ADR-003: JSON for All Serialization

**Date:** 2026-01-17
**Status:** Accepted

### Context

Need a serialization format for config, state, API responses, and MQTT messages.

### Decision

Use JSON everywhere.

### Alternatives Considered

- **Protobuf** — More efficient but harder to debug, requires schema files
- **MessagePack** — Binary, smaller but not human-readable
- **CBOR** — Similar to MessagePack

### Consequences

- Human-readable (great for debugging)
- Universal support
- Slightly larger than binary formats
- Edge agent uses TOML for config (more human-friendly for hand-editing)

---

## ADR-004: Embedded Web Dashboard

**Date:** 2026-02-06
**Status:** Accepted

### Context

Need a monitoring dashboard without requiring a separate frontend build step or deployment.

### Decision

Single-page app using Alpine.js + Chart.js, embedded in the Go binary via `embed` package.

### Alternatives Considered

- **React/Vue SPA** — Requires Node.js build step, larger bundle
- **htmx** — Good for simple CRUD, less ideal for real-time dashboards
- **Server-side templates** — Would need Go template engine, harder to make interactive

### Consequences

- Zero build step (vanilla JS)
- Single binary deployment (dashboard included)
- CDN dependency for Alpine.js and Chart.js (could be vendored)
- ~70KB total frontend code

---

## ADR-005: Evolution via Parameter Mutation

**Date:** 2026-01-20
**Status:** Accepted

### Context

Need agents to improve over time without human intervention.

### Decision

Simple parameter mutation with fitness-based evaluation. Start with EMA-smoothed fitness scoring and gaussian-like parameter perturbation.

### Alternatives Considered

- **Genetic algorithms** — Tournament selection, crossover — more complex, planned for future
- **Bayesian optimization** — Better for small parameter spaces, complex implementation
- **Reinforcement learning** — Requires training infrastructure, too heavy for v1
- **LLM-powered mutation** — Use an LLM to improve prompts — planned for future

### Consequences

- Simple and predictable
- Works for continuous parameters
- History + reversion provides safety
- Limited: can't evolve discrete choices (model selection, prompt text) yet

---

## ADR-006: Multi-Provider Model Router

**Date:** 2026-01-22
**Status:** Accepted

### Context

Don't want to be locked into a single LLM provider. Need resilience and cost optimization.

### Decision

Model router with complexity-based selection and automatic fallback chains.

### Consequences

- Provider-agnostic agent definitions
- Automatic failover if a provider goes down
- Cost tracking per model
- Easy to add new providers (implement `ModelProvider` interface)

---

## ADR-007: File-Based State Persistence

**Date:** 2026-01-18
**Status:** Accepted

### Context

Need to persist agent state, conversation memory, and evolution strategies across restarts.

### Decision

JSON files in the data directory. No database.

### Alternatives Considered

- **SQLite** — More structured but overkill for current scale
- **bbolt/BoltDB** — Key-value store, more complex
- **Redis** — Requires separate server

### Consequences

- Simple to backup (tar the data dir)
- Human-readable state files
- Easy to debug
- Won't scale past ~1000 agents on a single machine
- May migrate to SQLite in future if needed
