# AGENTS.md — EvoClaw Agent Harness

EvoClaw is a self-evolving AI agent framework written in Go. It provides the runtime, orchestrator,
skill bank, memory system, and evolution engine that ClawChain agents run on.
This file is a **table of contents** — not a reference manual. Follow the links.

---

## Repo Map

```
cmd/                      — binary entry points
  evoclaw/                  main agent binary (HTTP API + TUI)
  evoclaw-tui/              standalone TUI
  init-turso/               DB initialisation utility
  tui/                      shared TUI library

internal/                 — private packages (never imported by external code)
  agents/                   agent lifecycle, conversation, context management
  orchestrator/             session orchestration, tool loop, parallel dispatch
  orchestrator/rsi/         RSI (recursive self-improvement) logger
  skillbank/                skill extraction pipeline (distiller→retriever→injector→updater)
  evolution/                agent genome evolution engine
  genome/                   genome encoding/decoding, mutation, crossover
  rsi/                      RSI loop (observer→analyzer→fixer)
  interfaces/               core interface definitions (Channel, Memory, Provider, Tool)
  config/                   config loading (TOML)
  memory/                   memory store, hybrid retrieval
  models/                   LLM provider abstraction
  router/                   intelligent model routing
  clawchain/                ClawChain RPC client
  onchain/                  on-chain transaction helpers
  channels/                 messaging channel adapters (Telegram, Discord, WhatsApp)
  cloud/                    cloud sync (config, memory backup)
  scheduler/                cron job management
  security/                 auth, rate limiting
  wal/                      write-ahead log

pkg/                      — public library packages (safe for external import)

skills/                   — bundled OpenClaw skills (Python)
edge-agent/               — Rust edge agent (IoT/embedded targets)
docs/                     — architecture, quality, and convention docs (READ THESE FIRST)
scripts/                  — CI helpers, agent-lint, codegen
```

---

## Layer Rules (CRITICAL)

```
cmd/ → internal/ → pkg/        ← only direction allowed
cmd/ → pkg/                    ← ok
internal/ → pkg/               ← ok

internal/ → cmd/               ← FORBIDDEN (reverse dependency)
pkg/ → internal/               ← FORBIDDEN (reverse dependency)
pkg/ → cmd/                    ← FORBIDDEN (reverse dependency)
```

See [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) for the full layer diagram and dependency rules.

---

## Key Docs

| File | What it covers |
|------|---------------|
| [`docs/ARCHITECTURE.md`](docs/ARCHITECTURE.md) | Layer diagram, package responsibilities, interface patterns, SKILLRL pipeline |
| [`docs/QUALITY.md`](docs/QUALITY.md) | Coverage targets, mocking patterns, table-driven tests, global state rules |
| [`docs/CONVENTIONS.md`](docs/CONVENTIONS.md) | Naming, godoc, error handling, interface design |
| [`docs/EXECUTION_PLAN_TEMPLATE.md`](docs/EXECUTION_PLAN_TEMPLATE.md) | Template for planning complex tasks |

---

## How to Build & Test

```bash
# Build binary
go build ./cmd/evoclaw

# Run all tests
go test ./... -count=1 -timeout 120s

# Run lints (must pass before PR)
golangci-lint run ./...

# Run agent-specific lints (architectural invariants)
bash scripts/agent-lint.sh

# Run tests with race detector
go test -race ./... -timeout 120s

# Python scripts (skills, tools)
uv run python scripts/<name>.py
```

---

## Agent Invariants (non-negotiable)

1. **`cmd/` never imports from `internal/` packages via circular paths.** Layer rule is absolute.
2. **`internal/` never imports from `cmd/`.** Always move shared code to `pkg/`.
3. **No circular imports.** Run `go build ./...` — circular imports are compile errors.
4. **All exported functions/types must have godoc comments.** No exceptions.
5. **All new packages must have ≥90% test coverage.** Enforced in CI.
6. **Use interfaces, not concrete types, for dependencies.** See `internal/interfaces/`.
7. **No global mutable state in packages.** Pass state through constructors or function args.
8. **Table-driven tests preferred** for any function with multiple input/output cases.
9. **`uv run python`** for all Python scripts. Never `python3` or bare `python`.
10. **For complex tasks**, create an execution plan using `docs/EXECUTION_PLAN_TEMPLATE.md` first.

---

## CI Gates

Every PR runs:
- `go build ./...`
- `go test ./... -count=1`
- `go vet ./...`
- `bash scripts/agent-lint.sh` (architectural invariants)

All must pass. Failures include remediation instructions.

---

*This file must stay under 150 lines. See `scripts/agent-lint.sh` Rule 5.*
