<p align="center">
  <h1 align="center">🧬 EvoClaw</h1>
  <p align="center"><strong>Self-Evolving Agent Framework — Edge to Cloud</strong></p>
  <p align="center">
    <a href="https://github.com/clawinfra/evoclaw/actions/workflows/ci.yml"><img src="https://github.com/clawinfra/evoclaw/actions/workflows/ci.yml/badge.svg?branch=main" alt="CI"></a>
    <a href="https://github.com/clawinfra/evoclaw"><img src="https://img.shields.io/badge/Status-Beta-orange" alt="Beta"></a>
    <a href="https://go.dev"><img src="https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white" alt="Go"></a>
    <a href="https://www.rust-lang.org"><img src="https://img.shields.io/badge/Rust-stable-DEA584?logo=rust&logoColor=white" alt="Rust"></a>
    <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-green" alt="MIT License"></a>
  </p>
</p>

[![CI](https://github.com/clawinfra/evoclaw/actions/workflows/ci.yml/badge.svg)](https://github.com/clawinfra/evoclaw/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go)](https://go.dev)
[![Rust](https://img.shields.io/badge/Rust-stable-DEA584?logo=rust)](https://www.rust-lang.org)
[![Status](https://img.shields.io/badge/Status-Beta-orange)](https://github.com/clawinfra/evoclaw)
[![License](https://img.shields.io/badge/License-MIT-green)](LICENSE)

**Self-Evolving Agent Framework for Edge Devices**

---

EvoClaw is a lightweight agent orchestration framework where agents **improve themselves** through evolutionary feedback loops. A Go orchestrator coordinates Rust edge agents across three deployment tiers — from a Raspberry Pi on your desk to a Firecracker microVM in the cloud.

- **🦀 Rust Edge Agent** — 1.8MB binary, runs on Raspberry Pi, phones, IoT devices
- **🐹 Go Orchestrator** — 6.9MB binary, coordinates agents and handles evolution
- **🧬 Evolution Engine** — Agents improve themselves based on performance metrics
- **⚙️ Gateway/Daemon Mode** — systemd/launchd integration, auto-restart, graceful shutdown
- **📡 Multi-Channel** — Telegram, MQTT, WhatsApp (coming soon)
- **🤖 Multi-Model** — Anthropic, OpenAI, Ollama, OpenRouter support
- **🔀 Intelligent Model Fallback** — Circuit breaker pattern with automatic health tracking and degraded model routing
- **💰 Cost Tracking** — Monitor API usage and optimize spending
- **📊 HTTP API** — RESTful interface for monitoring and control
- **💬 Chat Interfaces** — HTTP API, web terminal, TUI for agent messaging
- **💾 Config Backup** — Automated cloud backup for hardware recovery

## ✨ Key Features

### One-Liner Install (Linux/macOS)

```bash
curl -fsSL https://evoclaw.win/install.sh | sh
```

Automatically detects your OS and architecture, downloads the latest release, and sets up everything.

### macOS (Homebrew)

```bash
brew tap clawinfra/evoclaw
brew install evoclaw
evoclaw init
```

### macOS (.dmg Installer)

1. Download `EvoClaw-{version}-{arch}.dmg` from [Releases](https://github.com/clawinfra/evoclaw/releases)
2. Open the DMG and drag EvoClaw to Applications
3. Right-click EvoClaw.app → **Open** (first time only, due to unsigned binary)
4. Launch from Applications or Spotlight

> **Note:** macOS will show "unidentified developer" warning on first run. This is normal for unsigned apps. See [MACOS-UNSIGNED.md](docs/MACOS-UNSIGNED.md) for workarounds.

### Windows (.msi Installer)

1. Download `EvoClaw-{version}-amd64.msi` from [Releases](https://github.com/clawinfra/evoclaw/releases)
2. Double-click to install
3. Run from Start Menu or PowerShell: `evoclaw init`

### Linux (Debian/Ubuntu)

```bash
# Download .deb package
wget https://github.com/clawinfra/evoclaw/releases/latest/download/evoclaw_{version}_amd64.deb

# Install
sudo dpkg -i evoclaw_{version}_amd64.deb

# Start service
sudo systemctl start evoclaw
```

### Linux (Fedora/RHEL)

```bash
# Download .rpm package
wget https://github.com/clawinfra/evoclaw/releases/latest/download/evoclaw-{version}-1.x86_64.rpm

# Install
sudo rpm -i evoclaw-{version}-1.x86_64.rpm

# Start service
sudo systemctl start evoclaw
```

### Build from Source

```bash
# Clone repository
git clone https://github.com/clawinfra/evoclaw
cd evoclaw

# Build orchestrator (Go)
go build -ldflags="-s -w" -o evoclaw ./cmd/evoclaw

# Build edge agent (Rust)
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
podman compose -f podman-compose.dev.yml up
```

## Gateway / Daemon Mode

Run EvoClaw as a background service with automatic restart:

### Linux (systemd)

```bash
# Install and enable service
evoclaw gateway install
sudo systemctl enable evoclaw
sudo systemctl start evoclaw

# Check status
sudo systemctl status evoclaw

# View logs
sudo journalctl -u evoclaw -f
```

### macOS (launchd)

```bash
# Install service
evoclaw gateway install

# Start service
launchctl start com.clawinfra.evoclaw

# View logs
tail -f ~/.evoclaw/logs/evoclaw.log
```

**Features:**
- ✅ Systemd integration (Linux)
- ✅ Launchd integration (macOS)
- ✅ Graceful shutdown (SIGTERM)
- ✅ Auto-restart on crash
- ✅ Security hardening
- 🔜 Config reload (SIGHUP)
- 🔜 Self-update (SIGUSR1)

**See [docs/GATEWAY.md](docs/GATEWAY.md) for full documentation.**

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
| `models.health` | evoclaw.json | Circuit breaker config (failure threshold, cooldown) |
| `evolution` | evoclaw.json | Eval interval, mutation rate, min samples |
| `agents[]` | evoclaw.json | Agent definitions (type, model, skills) |
| `[mqtt]` | agent.toml | Broker connection for edge agent |
| `[trading]` | agent.toml | Hyperliquid exchange config |
| `[monitor]` | agent.toml | Price/funding rate alert thresholds |

**See [docs/MODEL-HEALTH.md](docs/MODEL-HEALTH.md) for details on the health registry and circuit breaker pattern.**

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
├── docker/               # Container configs
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

## What's Implemented

> 🧬 EvoClaw has grown beyond parameter-only evolution:

- **Genome Layer 2** — Skill Selection & Composition: agents choose and combine skills dynamically
- **Genome Layer 3** — Behavioral Evolution: agents evolve high-level behavioral strategies
- **Agent Patterns** — WAL (Write-Ahead Log), VBR (Version-Based Recovery), ADL (Adaptive Decision Logic), VFM (Volatile Fitness Memory)
- **Model Health Registry** — Circuit breaker pattern for intelligent model fallback (see [docs/MODEL-HEALTH.md](docs/MODEL-HEALTH.md))
- **Security** — Signed constraints, JWT authentication, evolution firewall (see [docs/SECURITY.md](docs/SECURITY.md))
- **Config Backup** — Automated cloud backup for hardware recovery (see [docs/CONFIG-BACKUP.md](docs/CONFIG-BACKUP.md))
- **Messaging** — HTTP Chat API, Web Terminal, TUI for agent communication (see [docs/MESSAGING.md](docs/MESSAGING.md))
- **Docs** — [INSTALLATION.md](docs/INSTALLATION.md), [EVOLUTION.md](docs/EVOLUTION.md), [SECURITY.md](docs/SECURITY.md), [CONFIG-BACKUP.md](docs/CONFIG-BACKUP.md), [MESSAGING.md](docs/MESSAGING.md), [MODEL-HEALTH.md](docs/MODEL-HEALTH.md)

## Beta Known Limitations

> ⚠️ EvoClaw is in **beta**. The following limitations are known:

- **No TLS/auth on MQTT** — The default Mosquitto config allows anonymous access. For production, configure TLS and authentication.
- **No container isolation** — The `container` config field exists but Firecracker/gVisor isolation is not yet implemented.
- **WhatsApp channel** — Declared in config but not yet implemented.
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
- [x] Podman Compose deployment
- [x] CI/CD pipeline
- [x] Integration test suite
- [ ] WhatsApp channel
- [ ] TLS/mTLS for MQTT
- [x] Agent self-registration via `join` command + `POST /api/agents/register`
- [x] Hub setup wizard (`evoclaw setup hub`)
- [x] Deployment profiles documentation (Solo, Hub & Spoke, Cloud Fleet)
- [ ] Agent auto-discovery (mDNS)
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
├── memory/          # Conversation history + hybrid search (FTS5 + vector)
│   └── assistant-1.json
└── evolution/       # Strategy versions
    └── assistant-1.json
```

## SkillBank

SkillBank implements SKILLRL-inspired hierarchical skill learning for EvoClaw agents. Inspired by the [SKILLRL paper (arXiv:2602.08234)](https://arxiv.org/abs/2602.08234), it enables agents to distill reusable skills from past trajectories, retrieve relevant knowledge for new tasks, and recursively evolve their skill base over time.

### Architecture

```
internal/skillbank/
├── types.go      — Core types (Skill, CommonMistake, Trajectory) + interfaces
├── store.go      — File-backed JSONL store (thread-safe, atomic writes)
├── distiller.go  — LLM-based skill distillation from agent trajectories
├── retriever.go  — Keyword (TemplateRetriever) and embedding (EmbeddingRetriever) retrieval
├── injector.go   — Formats skills for system-prompt injection
└── updater.go    — Recursive skill evolution, pruning, and confidence tracking
```

### How It Works

1. **Distillation** — After each task cycle, trajectories are sent to an LLM (default: `anthropic-proxy-6/glm-4.7`) which extracts reusable `Skill` objects and `CommonMistake` patterns.

2. **Storage** — Skills are persisted as JSONL files on disk. Reads/writes are thread-safe via `sync.RWMutex`; writes are atomic via temp-file-then-rename.

3. **Retrieval** — Before executing a task, the agent retrieves relevant skills:
   - `TemplateRetriever`: zero-cost keyword overlap scoring (always available)
   - `EmbeddingRetriever`: cosine similarity via a local embedding endpoint, falls back to `TemplateRetriever` if unavailable

4. **Injection** — Retrieved skills and common mistakes are formatted as a markdown block and prepended to the agent's system prompt.

5. **Evolution** — The `SkillUpdater` closes the loop:
   - Distills new skills from failure trajectories not covered by existing skills
   - Tracks skill confidence via exponential moving average (α=0.1)
   - Prunes stale skills (low success rate + sufficient usage) to `archived_skills.jsonl`

### Quick Start

```go
// Create store
store, _ := skillbank.NewFileStore("data/skills.jsonl")

// Distill skills from trajectories
distiller := skillbank.NewLLMDistiller(apiURL, apiKey, "")
updater := skillbank.NewSkillUpdater(distiller, store, "data/")

failures := []skillbank.Trajectory{...}
newSkills, _ := updater.Update(ctx, failures, existingSkills)

// Retrieve for a new task
retriever := skillbank.NewRetriever(store, "") // keyword mode
skills, _ := retriever.Retrieve(ctx, "handle API rate limits", 5)

// Inject into system prompt
injector := skillbank.NewInjector()
mistakes, _ := store.ListMistakes("")
enrichedPrompt := injector.InjectIntoPrompt(systemPrompt, skills, mistakes)

// Track outcomes
updater.BoostSkillConfidence(skill.ID, succeeded)

// Prune stale skills periodically
pruned, _ := updater.PruneStaleSkills(ctx, 0.4, 10)
```

### Coverage

`go test ./internal/skillbank/... -cover` → **92.9%** statement coverage.

---

## 📄 License

[MIT](LICENSE)

## Foundations

EvoClaw's design is built on patterns from [pi](https://github.com/badlogic/pi-mono), Mario Zechner's minimal coding agent engine. Pi's philosophy — no baked-in opinions, skills over protocols, append-only session branching — directly shaped EvoClaw's architecture. See [docs/PI-INTEGRATION.md](docs/PI-INTEGRATION.md) for details.

## Built By

**Alex Chen** (alex.chen31337@gmail.com)
For the best of [ClawChain](https://github.com/clawinfra) 🧬

---

<p align="center">
  <em>Every device is an agent. Every agent evolves.</em><br>
  <a href="docs/getting-started/quickstart.md">Quickstart</a> · <a href="docs/architecture/overview.md">Architecture</a> · <a href="docs/guides/trading-agent.md">Trading</a> · <a href="docs/guides/cloud-deployment.md">Cloud</a> · <a href="docs/contributing/CONTRIBUTING.md">Contribute</a>
</p>
