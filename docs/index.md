# EvoClaw Documentation

> *Self-evolving agent framework for edge devices. Go orchestrator + Rust agents.*

Welcome to the EvoClaw documentation. EvoClaw is a lightweight, evolution-powered agent orchestration framework designed to deploy AI agents on resource-constrained edge devices — from Raspberry Pi to IoT sensors to phones.

**Every device is an agent. Every agent evolves.** 🧬

---

## Quick Start

Get EvoClaw running in 5 minutes:

```bash
# Clone the repo
git clone https://github.com/clawinfra/evoclaw.git
cd evoclaw

# Build the orchestrator
go build -ldflags="-s -w" -o evoclaw ./cmd/evoclaw

# Create config from template
cp evoclaw.example.json evoclaw.json
# Edit evoclaw.json with your API keys

# Run
./evoclaw --config evoclaw.json
```

Open [http://localhost:8420](http://localhost:8420) for the web dashboard.

For the full setup guide, see [Getting Started](getting-started/quickstart.md).

---

## What is EvoClaw?

EvoClaw is a framework for deploying self-improving AI agents on any device:

- **Go Orchestrator** (6.9MB) — Coordinates agents, routes LLM requests, manages evolution
- **Rust Edge Agent** (3.2MB) — Runs on constrained hardware, executes strategies
- **Evolution Engine** — Agents improve themselves based on performance metrics
- **Multi-Channel** — Telegram, MQTT, WhatsApp (planned)
- **Multi-Model** — Anthropic, OpenAI, Ollama, OpenRouter with intelligent routing

### The Water Principle

> *"Empty your mind, be formless, shapeless — like water."* — Bruce Lee

Put EvoClaw in a teddy bear → it becomes a companion.
Put it on a trading terminal → it becomes a trader.
Put it on a farm sensor → it becomes a crop whisperer.

Same DNA. Same evolution engine. Different container.

---

## Documentation Map

### [Getting Started](getting-started/quickstart.md)
- [Installation](getting-started/installation.md) — Binary, source, or Docker
- [Configuration](getting-started/configuration.md) — Full config reference
- [Quick Start](getting-started/quickstart.md) — 5-minute guide
- [First Agent](getting-started/first-agent.md) — Create your first agent

### [Architecture](architecture/overview.md)
- [System Overview](architecture/overview.md) — How it all fits together
- [Orchestrator](architecture/orchestrator.md) — Go orchestrator deep dive
- [Edge Agent](architecture/edge-agent.md) — Rust agent deep dive
- [Evolution Engine](architecture/evolution.md) — How agents evolve
- [Communication](architecture/communication.md) — MQTT protocol & message formats

### [Guides](guides/trading-agent.md)
- [Trading Agent](guides/trading-agent.md) — Set up a Hyperliquid trading agent
- [Companion Agent](guides/companion-agent.md) — Build a companion device agent
- [Custom Strategy](guides/custom-strategy.md) — Write custom trading strategies
- [Model Routing](guides/model-routing.md) — Configure model providers & routing
- [Deployment](guides/deployment.md) — Production deployment guide

### [API Reference](api/rest-api.md)
- [REST API](api/rest-api.md) — Full HTTP API reference
- [MQTT Protocol](api/mqtt-protocol.md) — MQTT topics & message formats
- [WebSocket/SSE](api/websocket.md) — Real-time streaming endpoints

### [Reference](reference/config-schema.md)
- [Config Schema](reference/config-schema.md) — Complete JSON config schema
- [Genome Format](reference/genome-format.md) — Trading genome/style.toml format
- [Metrics](reference/metrics.md) — All tracked metrics explained
- [Environment Variables](reference/environment.md) — Environment configuration

### [Contributing](contributing/CONTRIBUTING.md)
- [How to Contribute](contributing/CONTRIBUTING.md)
- [Development Setup](contributing/development.md)
- [Architecture Decisions](contributing/architecture-decisions.md)

---

## License

MIT — see [LICENSE](https://github.com/clawinfra/evoclaw/blob/main/LICENSE) for details.

Built by **Alex Chen** for [ClawChain](https://github.com/clawinfra) 🧬
