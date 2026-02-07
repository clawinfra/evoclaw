<p align="center">
  <h1 align="center">🧬 EvoClaw</h1>
  <p align="center"><strong>Self-Evolving Agent Framework — Edge to Cloud</strong></p>
  <p align="center">
    <a href="https://github.com/clawinfra/evoclaw/actions/workflows/ci.yml"><img src="https://github.com/clawinfra/evoclaw/actions/workflows/ci.yml/badge.svg?branch=beta" alt="CI"></a>
    <a href="https://github.com/clawinfra/evoclaw"><img src="https://img.shields.io/badge/Status-Beta-orange" alt="Beta"></a>
    <a href="https://go.dev"><img src="https://img.shields.io/badge/Go-1.24-00ADD8?logo=go&logoColor=white" alt="Go"></a>
    <a href="https://www.rust-lang.org"><img src="https://img.shields.io/badge/Rust-stable-DEA584?logo=rust&logoColor=white" alt="Rust"></a>
    <a href="LICENSE"><img src="https://img.shields.io/badge/License-MIT-green" alt="MIT License"></a>
  </p>
</p>

<p align="center">
  <em>Every device is an agent. Every agent evolves.</em>
</p>

---

EvoClaw is a lightweight agent orchestration framework where agents **improve themselves** through evolutionary feedback loops. A Go orchestrator coordinates Rust edge agents across three deployment tiers — from a Raspberry Pi on your desk to a Firecracker microVM in the cloud.

Put it in a teddy bear — it becomes a companion. Put it on an exchange — it becomes a trader. Put it on a farm sensor — it becomes a crop whisperer.

## ✨ Key Features

| | Feature | Details |
|---|---|---|
| 🧬 | **Evolution Engine** | Agents track fitness, mutate strategies, and revert if worse. Survival of the fittest. |
| 📈 | **Trading** | Hyperliquid perps — paper trading, testnet, risk management, native Rust signing |
| 📊 | **Web Dashboard** | Real-time dark-theme SPA at `localhost:8420` — agents, metrics, logs, evolution |
| 🤖 | **Multi-Model** | Anthropic, OpenAI, Ollama, OpenRouter. Intelligent routing by task complexity. |
| 🦀 | **Rust Edge Agent** | 3.2 MB binary. Runs on Pi, phones, IoT. Zero dependencies. |
| ☁️ | **E2B Cloud** | Firecracker microVMs with ~100ms cold start. Multi-tenant SaaS mode. |
| 🐧 | **Podman-First** | Daemonless rootless containers. Docker fallback. Systemd-native. |
| 💰 | **Cost Tracking** | Per-model, per-agent, per-tenant cost accounting with budget enforcement |
| 💬 | **Human Chat** | Talk to agents via Telegram bot or dashboard chat widget |

## 💬 Talk to Your Agents

Two ways to communicate with your agents:

| Channel | How | Guide |
|---------|-----|-------|
| **Dashboard Chat** | Built-in chat widget at `localhost:8420` → Chat | [Dashboard Chat Guide](docs/guides/dashboard-chat.md) |
| **Telegram Bot** | `/ask What's the CPU temp?` from your phone | [Telegram Bot Guide](docs/guides/telegram-bot.md) |

Both use the same `ChatSync` flow: your message → agent's LLM → response with conversation history.

## ⚡ Deploy in 2 Commands

```bash
# On your server
evoclaw setup hub

# On your Pi / edge device
evoclaw-agent join YOUR_SERVER_IP
```

The `join` command auto-discovers the hub, generates a config, registers the agent, and starts it. No config files to edit, no ports to look up.

→ Full guide: [Deployment Profiles](docs/guides/deployment-profiles.md) — Solo, Hub & Spoke, Cloud Fleet

## 🏗️ Three-Tier Deployment

```
🔌 Edge     →  Bare Rust binary on Pi / IoT / laptop     — 3.2 MB, zero deps, 6 MB RAM
🏠 Server   →  Podman or Docker on your own server        — Full control, make up
☁️  Cloud    →  E2B Firecracker sandboxes (SaaS mode)      — ~100ms cold start, API-driven
```

Same Rust agent binary. Three ways to run it:

| Tier | Isolation | Scaling | Cost |
|------|-----------|---------|------|
| **Edge** | Process-level | Manual | Hardware only |
| **Server** | Container (Podman/Docker) | Compose | Server costs |
| **Cloud** | MicroVM (Firecracker) | API-driven, instant | ~$0.36/hr/agent |

## 🚀 Quick Start

### Option 1 — Podman / Docker (recommended)

```bash
git clone https://github.com/clawinfra/evoclaw && cd evoclaw

# Configure
cp evoclaw.example.json evoclaw.json
cp edge-agent/agent.example.toml edge-agent/agent.toml
# Edit both files — add your API keys

# Launch (auto-detects Podman → Docker)
make up

# Verify
curl http://localhost:8420/api/status
open http://localhost:8420          # Web Dashboard
```

> Install Podman: `sudo apt install podman podman-compose` (Debian/Ubuntu) or `sudo dnf install podman podman-compose` (Fedora). Docker works too — `make up-docker` forces it.

### Option 2 — Bare Metal

```bash
# Build orchestrator (Go)
go build -ldflags="-s -w" -o evoclaw ./cmd/evoclaw

# Build edge agent (Rust)
cd edge-agent && cargo build --release

# Start MQTT broker
mosquitto -c docker/mosquitto.conf &

# Run
./evoclaw --config evoclaw.json
./edge-agent/target/release/evoclaw-agent --config edge-agent/agent.toml
```

### Option 3 — E2B Cloud

```bash
# Set your E2B API key
export E2B_API_KEY="e2b_..."

# Spawn a cloud agent (Firecracker microVM)
./evoclaw cloud spawn --template evoclaw-agent --config edge-agent/agent.toml

# List running agents
./evoclaw cloud list

# Check costs
./evoclaw cloud costs
```

→ Full guide: [docs/guides/cloud-deployment.md](docs/guides/cloud-deployment.md)

## 📊 Web Dashboard

The orchestrator serves a built-in dark-theme dashboard at **`http://localhost:8420`**:

- **Agent Overview** — Status, uptime, model, last heartbeat for every connected agent
- **Live Metrics** — Success rate, response time, token usage, cost per agent
- **Evolution Tracker** — Fitness scores, mutation history, strategy versions
- **Log Stream** — Real-time SSE log feed from the orchestrator
- **Cost Dashboard** — Per-model and per-agent spend breakdown

The dashboard is embedded in the Go binary — no Node.js, no build step, no CDN. Just open the URL.

## 🏛️ Architecture

```
┌───────────────────────────────────────────────────────────────────┐
│                   🧬 EvoClaw Orchestrator (Go)                    │
│                                                                   │
│  ┌────────────────────────────────────────────────────────────┐   │
│  │            Evolution Engine (Strategy Mutation)            │   │
│  └────────────────────────────────────────────────────────────┘   │
│       ↕               ↕               ↕               ↕          │
│  ┌──────────┐  ┌───────────┐  ┌──────────────┐  ┌────────────┐  │
│  │  Agent   │  │   Model   │  │  HTTP API    │  │   Cloud    │  │
│  │ Registry │  │  Router   │  │  + Dashboard │  │  Manager   │  │
│  │ + Memory │  │ (4 LLMs)  │  │  :8420       │  │  (E2B)     │  │
│  └──────────┘  └───────────┘  └──────────────┘  └────────────┘  │
│       ↕               ↕               ↕               ↕          │
│   Anthropic       OpenAI          Ollama         OpenRouter      │
└───────────────────────────────────────────────────────────────────┘
        ↕                            ↕                    ↕
   ┌─────────┐               ┌──────────────┐    ┌──────────────┐
   │Telegram │               │  MQTT Broker │    │   E2B API    │
   │  Bot    │               │ (Mosquitto)  │    │ (Firecracker)│
   └─────────┘               └──────────────┘    └──────────────┘
        ↕                     ↕      ↕      ↕           ↕
     Users            ┌───────┐ ┌───────┐ ┌───────┐ ┌───────────┐
                      │🔌 Edge│ │🔌 Edge│ │🏠 Ctr │ │☁️ Cloud    │
                      │Trader │ │Monitor│ │ Agent │ │ Agent x N │
                      │ (Pi)  │ │ (IoT) │ │(Pod)  │ │(microVM)  │
                      └───────┘ └───────┘ └───────┘ └───────────┘
```

## 📈 Trading

EvoClaw includes a production-ready trading pipeline for [Hyperliquid](https://hyperliquid.xyz) perpetual futures:

| Feature | Description |
|---------|-------------|
| **Paper Trading** | Full order book simulation with fill tracking — zero risk |
| **Testnet** | Live orders on Hyperliquid testnet with free USDC faucet |
| **Risk Management** | Max daily loss, position limits, consecutive-loss cooldown, emergency stop |
| **Native Signing** | Pure Rust EIP-712 signing — no Python, no external scripts |
| **Strategies** | Mean reversion + funding rate arbitrage, with evolutionary parameter tuning |
| **PnL Tracking** | Win rate, Sharpe ratio, drawdown, per-trade history feeding evolution |

**Safety model:** Agents start in `testnet + paper` mode by default. Three layers of protection before real money:

```
Paper Trading → Testnet (fake money) → Mainnet (real money, requires explicit opt-in)
```

→ Guides: [Trading Agent](docs/guides/trading-agent.md) · [Testnet Setup](edge-agent/docs/TESTNET.md) · [Custom Strategy](docs/guides/custom-strategy.md)

## 🧬 Evolution Engine

Every agent tracks performance metrics — success rate, response time, cost, trading PnL, Sharpe ratio. The evolution engine continuously evaluates fitness and adapts:

```
  ┌──────────────────────────────────────────────────┐
  │              Evolution Cycle                      │
  │                                                   │
  │  📊 Collect metrics  →  📈 Compute fitness        │
  │         ↓                        ↓                │
  │  fitness ≥ threshold?    fitness < threshold?      │
  │         ↓                        ↓                │
  │    ✅ Keep strategy       🔀 Mutate parameters     │
  │                                  ↓                │
  │                          📊 Test new strategy      │
  │                                  ↓                │
  │                          Worse? → ↩️ Revert        │
  │                          Better? → ✅ Keep          │
  └──────────────────────────────────────────────────┘
```

What gets mutated: temperature, model selection, system prompts, trading thresholds, strategy weights. What drives fitness: success rate, response quality, cost efficiency, trading PnL.

## 🔌 API Reference

### Core API

```bash
GET  /api/status                          # System status + uptime
GET  /api/agents                          # List all agents
POST /api/agents/register                 # Register edge agent (join flow)
GET  /api/agents/{id}                     # Agent details
GET  /api/agents/{id}/metrics             # Performance metrics
POST /api/agents/{id}/evolve              # Trigger evolution
GET  /api/agents/{id}/memory              # Conversation history
DEL  /api/agents/{id}/memory              # Clear memory
GET  /api/models                          # Available LLM models
GET  /api/costs                           # Cost tracking
GET  /api/dashboard                       # Dashboard data (JSON)
GET  /api/logs/stream                     # SSE real-time log stream
```

### Cloud API (E2B Sandboxes)

```bash
POST /api/cloud/spawn                     # Spawn cloud agent
GET  /api/cloud                           # List cloud agents
GET  /api/cloud/{id}                      # Agent status
DEL  /api/cloud/{id}                      # Kill agent
GET  /api/cloud/costs                     # E2B credit usage
```

### SaaS API (Multi-Tenant)

```bash
POST /api/saas/register                   # Register user → API key
POST /api/saas/agents                     # Spawn user agent
GET  /api/saas/agents                     # List user agents
DEL  /api/saas/agents/{id}               # Kill user agent
GET  /api/saas/usage                      # User usage report
```

### MQTT Protocol

| Topic | Direction | Purpose |
|-------|-----------|---------|
| `evoclaw/agents/{id}/commands` | orchestrator → agent | Commands (ping, execute, update_strategy) |
| `evoclaw/agents/{id}/reports` | agent → orchestrator | Results, errors, metrics |
| `evoclaw/agents/{id}/status` | agent → orchestrator | Heartbeat every 30s |
| `evoclaw/agents/{id}/strategy` | orchestrator → agent | Evolved strategy push |
| `evoclaw/broadcast` | orchestrator → all | Broadcast messages |

## 📚 Documentation

EvoClaw ships with **31 docs** covering architecture, guides, and API reference:

| Section | Contents |
|---------|----------|
| [Getting Started](docs/getting-started/) | [Installation](docs/getting-started/installation.md) · [Quickstart](docs/getting-started/quickstart.md) · [Configuration](docs/getting-started/configuration.md) · [First Agent](docs/getting-started/first-agent.md) |
| [Architecture](docs/architecture/) | [Overview](docs/architecture/overview.md) · [Orchestrator](docs/architecture/orchestrator.md) · [Edge Agent](docs/architecture/edge-agent.md) · [Evolution](docs/architecture/evolution.md) · [Communication](docs/architecture/communication.md) |
| [Guides](docs/guides/) | [Deployment Profiles](docs/guides/deployment-profiles.md) · [Trading Agent](docs/guides/trading-agent.md) · [Edge Deploy](docs/guides/edge-deployment.md) · [Container Deploy](docs/guides/container-deployment.md) · [Cloud Deploy](docs/guides/cloud-deployment.md) · [Model Routing](docs/guides/model-routing.md) · [Custom Strategy](docs/guides/custom-strategy.md) · [Companion Agent](docs/guides/companion-agent.md) |
| [API Reference](docs/api/) | [REST API](docs/api/rest-api.md) · [MQTT Protocol](docs/api/mqtt-protocol.md) · [WebSocket](docs/api/websocket.md) |
| [Reference](docs/reference/) | [Config Schema](docs/reference/config-schema.md) · [Genome Format](docs/reference/genome-format.md) · [Metrics](docs/reference/metrics.md) · [Environment](docs/reference/environment.md) |
| [Contributing](docs/contributing/) | [Guide](docs/contributing/CONTRIBUTING.md) · [Development](docs/contributing/development.md) · [Architecture Decisions](docs/contributing/architecture-decisions.md) |

**For LLMs:** [`llms.txt`](llms.txt) (summary) and [`llms-full.txt`](llms-full.txt) (complete project context, 138 KB).

## ⚙️ Configuration

Two config files — one for the orchestrator, one for each edge agent:

| File | Format | Key Sections |
|------|--------|--------------|
| [`evoclaw.example.json`](evoclaw.example.json) | JSON | `server` (port, dataDir), `mqtt` (broker), `channels` (Telegram), `models` (LLM providers + routing), `evolution` (mutation rate, fitness threshold), `cloud` (E2B), `agents[]` |
| [`agent.example.toml`](edge-agent/agent.example.toml) | TOML | `[mqtt]` (broker connection), `[trading]` (Hyperliquid wallet, mode, network), `[monitor]` (price alerts, funding rates), `[risk]` (position limits, daily loss cap) |

**Model routing** selects providers by complexity:

```
Simple tasks   → Ollama (local, free)
Complex tasks  → Claude Sonnet / GPT-4o
Critical tasks → Claude Opus
```

Fallback chains ensure reliability — if the primary model fails, the next one picks up automatically.

## 📁 Project Structure

```
evoclaw/
├── cmd/evoclaw/              # Go entrypoint + embedded web dashboard
│   ├── main.go               # Application setup, lifecycle, CLI
│   └── web/                  # Dashboard assets (HTML/CSS/JS)
├── internal/
│   ├── orchestrator/         # Core message routing + evolution loop
│   ├── agents/               # Agent registry + conversation memory
│   ├── api/                  # HTTP API + dashboard + cloud + SaaS handlers
│   ├── channels/             # Telegram + MQTT adapters
│   ├── models/               # LLM provider router (Anthropic, OpenAI, Ollama, OR)
│   ├── evolution/            # Fitness evaluation + strategy mutation
│   ├── cloud/                # E2B sandbox lifecycle + cost tracking
│   ├── saas/                 # Multi-tenant agent-as-a-service
│   ├── cli/                  # `evoclaw cloud` CLI commands
│   └── config/               # JSON configuration management
├── edge-agent/               # Rust edge agent (3.2 MB binary)
│   ├── src/
│   │   ├── agent.rs          # Agent lifecycle + heartbeat
│   │   ├── trading.rs        # Hyperliquid REST client
│   │   ├── signing.rs        # Native EIP-712 order signing
│   │   ├── paper.rs          # Paper trading simulator
│   │   ├── risk.rs           # Risk management engine
│   │   ├── strategy.rs       # Mean reversion + funding arb
│   │   ├── evolution.rs      # Local fitness tracker
│   │   ├── commands.rs       # MQTT command handlers
│   │   ├── monitor.rs        # Price + funding rate alerts
│   │   ├── metrics.rs        # Performance metrics
│   │   ├── mqtt.rs           # MQTT client
│   │   └── config.rs         # TOML config parser
│   ├── docs/TESTNET.md       # Hyperliquid testnet guide
│   └── tests/                # Integration tests
├── deploy/
│   ├── e2b/                  # E2B sandbox template (Dockerfile, entrypoint)
│   ├── podman-pod.sh         # Podman pod setup script
│   └── systemd/              # Systemd service files (4 units)
├── integration/              # E2E MQTT protocol tests
├── docs/                     # 31 documentation files
├── web/                      # Dashboard source
├── docker-compose.yml        # Production stack
├── docker-compose.dev.yml    # Development stack (hot-reload)
├── orchestrator.Dockerfile   # Go orchestrator image
├── Makefile                  # Build, deploy, test commands
├── llms.txt                  # LLM-friendly project summary
└── llms-full.txt             # Complete project context (138 KB)
```

## 🧑‍💻 Contributing

```bash
# Clone and build
git clone https://github.com/clawinfra/evoclaw && cd evoclaw
go build ./cmd/evoclaw && cd edge-agent && cargo build

# Test everything
go test -race ./...                       # Go (11 packages)
cd edge-agent && cargo test               # Rust (247 unit + 10 integration)
cd integration && go test -v -tags=integration  # E2E (requires MQTT)

# Lint
golangci-lint run
cd edge-agent && cargo clippy -- -D warnings
```

1. Fork → branch from `main` → implement → test → PR
2. Commit messages: `feat:`, `fix:`, `docs:`, `ci:`, `refactor:`
3. Coverage thresholds: Go ≥ 88%, Rust ≥ 90%
4. CI must pass before merge

→ Full guide: [CONTRIBUTING.md](docs/contributing/CONTRIBUTING.md) · [Development](docs/contributing/development.md)

## ⚠️ Beta Known Limitations

> EvoClaw is in **beta**. These limitations are known and tracked:

| Area | Limitation | Status |
|------|-----------|--------|
| **MQTT Security** | No TLS/auth by default — Mosquitto allows anonymous | Planned |
| **Container Isolation** | Config field exists but Firecracker/gVisor not wired | Planned |
| **WhatsApp** | Channel declared in config but not implemented | Backlog |
| **Evolution** | Parameter mutation only — LLM-powered prompt mutation coming | In design |
| **HA/Clustering** | Single orchestrator process, no failover | Backlog |
| **Agent Discovery** | `join` command for API-based registration — no mDNS yet | Partial |
| **Key Management** | File-based keys — no Vault/KMS integration | Backlog |

## 🗺️ Roadmap

### ✅ Shipped in Beta

- [x] Go orchestrator with HTTP API + web dashboard
- [x] Rust edge agent — trading, monitoring, evolution
- [x] Multi-provider model router (Anthropic, OpenAI, Ollama, OpenRouter)
- [x] Evolution engine with fitness tracking + strategy mutation
- [x] Hyperliquid trading — paper, testnet, native signing, risk management
- [x] Telegram channel + MQTT protocol
- [x] Docker Compose + Podman-first deployment
- [x] Bare metal edge deployment + systemd services
- [x] E2B cloud sandboxes + SaaS multi-tenant API
- [x] CI/CD pipeline + integration test suite
- [x] 31 docs + llms.txt

### 🔜 Next

- [ ] Web dashboard live UI (currently JSON API, SPA scaffolded)
- [ ] LLM-powered prompt mutation (evolutionary prompt engineering)
- [ ] WhatsApp channel
- [ ] TLS/mTLS for MQTT
- [x] Agent self-registration via `join` command + `POST /api/agents/register`
- [x] Hub setup wizard (`evoclaw setup hub`)
- [x] Deployment profiles documentation (Solo, Hub & Spoke, Cloud Fleet)
- [ ] Agent auto-discovery (mDNS)
- [ ] Distributed agent mesh
- [ ] Advanced evolution — genetic algorithms, tournament selection
- [ ] Container isolation (Firecracker/gVisor for self-hosted)
- [ ] Vault/KMS key management

## 📄 License

[MIT](LICENSE)

## 🔨 Built By

**Alex Chen** · [alex.chen31337@gmail.com](mailto:alex.chen31337@gmail.com)

For the best of [ClawChain](https://github.com/clawinfra) 🧬

---

<p align="center">
  <em>Every device is an agent. Every agent evolves.</em><br>
  <a href="docs/getting-started/quickstart.md">Quickstart</a> · <a href="docs/architecture/overview.md">Architecture</a> · <a href="docs/guides/trading-agent.md">Trading</a> · <a href="docs/guides/cloud-deployment.md">Cloud</a> · <a href="docs/contributing/CONTRIBUTING.md">Contribute</a>
</p>
