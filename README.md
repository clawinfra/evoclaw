# 🧬 EvoClaw

[![CI](https://github.com/clawinfra/evoclaw/actions/workflows/ci.yml/badge.svg)](https://github.com/clawinfra/evoclaw/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go)](https://go.dev)
[![Rust](https://img.shields.io/badge/Rust-stable-DEA584?logo=rust)](https://www.rust-lang.org)
[![Status](https://img.shields.io/badge/Status-Beta-orange)](https://github.com/clawinfra/evoclaw)
[![License](https://img.shields.io/badge/License-MIT-green)](LICENSE)

**Self-Evolving Agent Framework for Edge Devices**

EvoClaw is a lightweight, evolution-powered agent orchestration framework designed to run on resource-constrained edge devices. Every device becomes an agent. Every agent evolves.

## Features

- **🦀 Rust Edge Agent** — 1.8MB binary, runs on Raspberry Pi, phones, IoT devices
- **🐹 Go Orchestrator** — 6.9MB binary, coordinates agents and handles evolution
- **🧬 Evolution Engine** — Agents improve themselves based on performance metrics
- **📡 Multi-Channel** — Telegram, MQTT, WhatsApp (coming soon)
- **🤖 Multi-Model** — Anthropic, OpenAI, Ollama, OpenRouter support
- **💰 Cost Tracking** — Monitor API usage and optimize spending
- **📊 HTTP API** — RESTful interface for monitoring and control

## Quick Start

### Native Binary (Default)

Full OS access — bash, filesystem, network. Maximum power.

```bash
# One-liner install
curl -fsSL https://evoclaw.win/install.sh | sh

# Or build from source
go build -ldflags="-s -w" -o evoclaw ./cmd/evoclaw
cd edge-agent && cargo build --release

# Run
./evoclaw --config evoclaw.json
```

### Podman Container (Opt-in Sandbox)

Local sandbox — rootless, daemonless. Your machine, contained.

```bash
podman run -d --name evoclaw \
  -v evoclaw-data:/data \
  -p 8080:8080 \
  ghcr.io/clawinfra/evoclaw
```

### E2B Cloud Sandbox (Opt-in Cloud)

Remote sandbox — zero local footprint. Try without installing.

```bash
evoclaw sandbox --provider e2b
```

> **See [docs/EXECUTION-TIERS.md](docs/EXECUTION-TIERS.md) for full details on all three tiers.**

### Development Mode

```bash
# Build from source with hot-reload
podman compose -f compose.dev.yml up
```

## Architecture

```
┌──────────────────────────────────────────────────────────┐
│              🧬 EvoClaw Orchestrator (Go)                │
│                                                          │
│  ┌────────────────────────────────────────────────────┐  │
│  │         Evolution Engine (Strategy Mutation)       │  │
│  └────────────────────────────────────────────────────┘  │
│                          ↕                                │
│  ┌──────────────┐  ┌──────────┐  ┌──────────────────┐   │
│  │ Agent        │  │  Model   │  │   HTTP API       │   │
│  │ Registry +   │  │  Router  │  │   :8420           │   │
│  │ Memory Store │  │(Multi-   │  │  /api/status     │   │
│  │              │  │ Provider)│  │  /api/agents     │   │
│  └──────────────┘  └──────────┘  └──────────────────┘   │
│         ↕                ↕                                │
│    Anthropic         OpenAI          Ollama               │
│    (Claude)          (GPT)           (Local)              │
└──────────────────────────────────────────────────────────┘
         ↕                            ↕
    ┌─────────┐               ┌──────────────┐
    │Telegram │               │  MQTT Broker │
    │  Bot    │               │ (Mosquitto)  │
    └─────────┘               └──────────────┘
         ↕                        ↕       ↕
      Users              ┌────────┐ ┌────────┐
                         │🦀 Edge │ │🦀 Edge │
                         │Agent 1 │ │Agent 2 │
                         │(Trader)│ │(Monitor)│
                         └────────┘ └────────┘
```

## MQTT Protocol

Orchestrator and edge agents communicate over MQTT with structured JSON messages:

| Topic Pattern | Direction | Purpose |
|---|---|---|
| `evoclaw/agents/{id}/commands` | orchestrator → agent | Send commands (ping, execute, update_strategy) |
| `evoclaw/agents/{id}/reports` | agent → orchestrator | Report results, errors, metrics |
| `evoclaw/agents/{id}/status` | agent → orchestrator | Heartbeats every 30s |
| `evoclaw/agents/{id}/strategy` | orchestrator → agent | Strategy updates |
| `evoclaw/broadcast` | orchestrator → all | Broadcast to all agents |

### Command Format
```json
{
  "command": "ping",
  "payload": {},
  "request_id": "req-001"
}
```

### Report Format
```json
{
  "agent_id": "hl-trader-1",
  "agent_type": "trader",
  "report_type": "result",
  "payload": {"pong": true},
  "timestamp": 1707300000
}
```

## API Endpoints

```bash
curl http://localhost:8420/api/status          # System status
curl http://localhost:8420/api/agents           # List agents
curl http://localhost:8420/api/agents/ID/metrics # Agent metrics
curl -X POST http://localhost:8420/api/agents/ID/evolve  # Trigger evolution
curl http://localhost:8420/api/agents/ID/memory # Conversation memory
curl http://localhost:8420/api/models           # Available models
curl http://localhost:8420/api/costs            # Cost tracking
```

## Model Routing

The router intelligently selects models based on task complexity:

- **Simple tasks** → Cheap local models (Ollama)
- **Complex tasks** → Mid-tier models (Claude Sonnet, GPT-4o)
- **Critical tasks** → Best available (Claude Opus)

Fallback chains ensure reliability even when primary models fail.

## Evolution

Agents track performance metrics:
- Success rate, response time, token usage, cost efficiency
- Custom metrics (trading PnL, win rate, Sharpe ratio)

When fitness drops below threshold:
1. **Evaluate** current strategy
2. **Mutate** parameters (temperature, prompts, model selection)
3. **Test** new strategy
4. **Revert** if worse than previous

## Configuration

See [`evoclaw.example.json`](evoclaw.example.json) for orchestrator config and [`edge-agent/agent.example.toml`](edge-agent/agent.example.toml) for edge agent config.

### Key Config Sections

| Section | File | Description |
|---|---|---|
| `server` | evoclaw.json | HTTP port, data dir, log level |
| `mqtt` | evoclaw.json | Broker host/port/auth |
| `channels.telegram` | evoclaw.json | Telegram bot token |
| `models.providers` | evoclaw.json | LLM API keys (Anthropic, OpenAI, Ollama) |
| `models.routing` | evoclaw.json | Task complexity → model mapping |
| `evolution` | evoclaw.json | Eval interval, mutation rate, min samples |
| `agents[]` | evoclaw.json | Agent definitions (type, model, skills) |
| `[mqtt]` | agent.toml | Broker connection for edge agent |
| `[trading]` | agent.toml | Hyperliquid exchange config |
| `[monitor]` | agent.toml | Price/funding rate alert thresholds |

## Development

### Run Tests

```bash
# Go orchestrator (8 packages, 88%+ coverage)
go test -race ./...

# Rust edge agent (172 unit + 10 integration tests, 90%+ coverage)
cd edge-agent && cargo test

# End-to-end integration tests (requires MQTT broker)
cd integration && go test -v -tags=integration ./...
```

### Project Structure

```
evoclaw/
├── cmd/evoclaw/          # Go entrypoint
├── internal/
│   ├── orchestrator/     # Core orchestration loop
│   ├── channels/         # Telegram, MQTT adapters
│   ├── models/           # LLM provider router
│   ├── evolution/        # Evolution engine
│   ├── agents/           # Agent registry + memory
│   ├── api/              # HTTP API server
│   └── config/           # Configuration loading
├── edge-agent/           # Rust edge agent
│   ├── src/
│   │   ├── agent.rs      # Agent lifecycle
│   │   ├── mqtt.rs       # MQTT client
│   │   ├── commands.rs   # Command handlers
│   │   ├── trading.rs    # Hyperliquid client
│   │   ├── strategy.rs   # Trading strategies
│   │   ├── evolution.rs  # Evolution tracker
│   │   ├── metrics.rs    # Performance metrics
│   │   ├── monitor.rs    # Market monitoring
│   │   └── config.rs     # TOML config
│   └── tests/            # Integration tests
├── integration/          # E2E MQTT protocol tests
├── docker/               # Docker configs
├── docs/                 # Documentation
└── assets/               # Logos and images
```

## Contributing

1. **Fork** the repository
2. **Branch** from `main`: `git checkout -b feature/your-feature`
3. **Test** your changes: `go test ./...` and `cd edge-agent && cargo test`
4. **Lint**: `golangci-lint run` and `cargo clippy`
5. **Commit** with clear messages: `feat:`, `fix:`, `docs:`, `ci:`
6. **PR** against `main` — CI must pass

### Code Standards
- Go: `gofmt`, `golangci-lint`, 88%+ test coverage
- Rust: `rustfmt`, `clippy -D warnings`, 90%+ test coverage
- Integration tests must not break existing unit tests
- All new features need tests

## Beta Known Limitations

> ⚠️ EvoClaw is in **beta**. The following limitations are known:

- **No TLS/auth on MQTT** — The default Mosquitto config allows anonymous access. For production, configure TLS and authentication.
- **No container isolation** — The `container` config field exists but Firecracker/gVisor isolation is not yet implemented.
- **WhatsApp channel** — Declared in config but not yet implemented.
- **Evolution engine** — Strategy mutation is parameter-only; LLM-powered prompt mutation is on the roadmap.
- **Single orchestrator** — No HA/clustering support yet. The orchestrator is a single process.
- **Edge agent auto-discovery** — Agents must be manually configured; no mDNS/auto-registration yet.
- **Private key management** — Keys are stored as files; no vault/KMS integration.
- **Hyperliquid integration** — Trading client makes HTTP calls but order signing requires the external Python script (`scripts/hl_sign.py`).

## Roadmap

- [x] Go orchestrator core
- [x] Telegram channel
- [x] MQTT channel
- [x] Multi-provider model router
- [x] Cost tracking
- [x] Agent registry + memory
- [x] HTTP API
- [x] Evolution engine integration
- [x] Rust edge agent with trading/monitoring
- [x] Docker Compose deployment
- [x] CI/CD pipeline
- [x] Integration test suite
- [ ] WhatsApp channel
- [ ] Prompt mutation (LLM-powered strategy improvement)
- [ ] Container isolation (Firecracker/gVisor)
- [ ] Distributed agent mesh
- [ ] Advanced evolution (genetic algorithms, tournament selection)
- [ ] Web dashboard UI
- [ ] TLS/mTLS for MQTT
- [ ] Agent auto-discovery (mDNS)

## Data Persistence

EvoClaw stores state in the configured `dataDir`:

```
data/
├── agents/          # Agent state (JSON)
│   └── assistant-1.json
├── memory/          # Conversation history
│   └── assistant-1.json
└── evolution/       # Strategy versions
    └── assistant-1.json
```

## License

MIT

## Built By

**Alex Chen** (alex.chen31337@gmail.com)
For the best of [ClawChain](https://github.com/clawinfra) 🧬

---

*Every device is an agent. Every agent evolves.*
