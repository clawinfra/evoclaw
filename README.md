# 🧬 EvoClaw

**Self-Evolving Agent Framework for Edge Devices**

EvoClaw is a lightweight, evolution-powered agent orchestration framework designed to run on resource-constrained edge devices. Every device becomes an agent. Every agent evolves.

## Features

- **🦀 Rust Edge Agent** - 1.8MB binary, runs on Raspberry Pi, phones, IoT devices
- **🐹 Go Orchestrator** - 6.9MB binary, coordinates agents and handles evolution
- **🧬 Evolution Engine** - Agents improve themselves based on performance metrics
- **📡 Multi-Channel** - Telegram, MQTT, Terminal TUI
- **🤖 Multi-Model** - Anthropic, OpenAI, Ollama, OpenRouter support
- **💰 Cost Tracking** - Monitor API usage and optimize spending
- **📊 HTTP API** - RESTful interface for monitoring and control

## Quick Start

### 1. Configure

```bash
cp evoclaw.example.json evoclaw.json
# Edit evoclaw.json with your API keys
```

### 2. Run

```bash
./evoclaw --config evoclaw.json
```

### 3. API Endpoints

```bash
# System status
curl http://localhost:8420/api/status

# List agents
curl http://localhost:8420/api/agents

# Agent metrics
curl http://localhost:8420/api/agents/assistant-1/metrics

# Trigger evolution
curl -X POST http://localhost:8420/api/agents/assistant-1/evolve

# View conversation memory
curl http://localhost:8420/api/agents/assistant-1/memory

# List available models
curl http://localhost:8420/api/models

# Cost tracking
curl http://localhost:8420/api/costs
```

## Architecture

```
┌─────────────────────────────────────────────────┐
│           🧬 EvoClaw Orchestrator               │
│  ┌──────────────────────────────────────────┐   │
│  │   Evolution Engine (Strategy Mutation)   │   │
│  └──────────────────────────────────────────┘   │
│                      ↓                           │
│  ┌──────────────────────────────────────────┐   │
│  │  Agent Registry + Memory Store           │   │
│  └──────────────────────────────────────────┘   │
│                      ↓                           │
│  ┌──────────────────────────────────────────┐   │
│  │  Model Router (Multi-Provider + Fallback)│   │
│  └──────────────────────────────────────────┘   │
│         ↓                ↓                ↓      │
│    Anthropic         OpenAI          Ollama      │
│    (Claude)          (GPT)           (Local)     │
└─────────────────────────────────────────────────┘
         ↕                ↕                ↕
    Telegram           MQTT         Terminal TUI
         ↕                ↕                ↕
      Users      Edge Agents (Rust)   SSH / Local
```

## Channel Support

### Telegram
- HTTP long polling (no webhook needed)
- Send/receive text messages
- Reply support

### MQTT
- Agent-to-orchestrator communication
- Topics:
  - `evoclaw/agents/{id}/commands` - orchestrator → agent
  - `evoclaw/agents/{id}/reports` - agent → orchestrator
  - `evoclaw/agents/{id}/status` - heartbeats
  - `evoclaw/broadcast` - orchestrator → all agents

### Terminal TUI (Planned)
- Interactive chat in any terminal — SSH into a headless box and talk to your agent
- Built with [Bubble Tea](https://github.com/charmbracelet/bubbletea) (Go TUI framework)
- Split-pane layout: agent status + conversation + input
- Works over SSH, tmux, screen — no GUI, no browser, no port forwarding
- First-class citizen alongside Telegram, not an afterthought

## Model Routing

The router intelligently selects models based on task complexity:

- **Simple tasks** → Cheap local models (Ollama)
- **Complex tasks** → Mid-tier models (Claude Sonnet, GPT-4o)
- **Critical tasks** → Best available (Claude Opus)

Fallback chains ensure reliability even when primary models fail.

## Evolution

Agents track performance metrics:
- Success rate
- Response time
- Token usage
- Cost efficiency
- Custom metrics (trading profits, etc.)

When fitness drops below threshold:
1. **Evaluate** current strategy
2. **Mutate** parameters (temperature, prompts, model selection)
3. **Test** new strategy
4. **Revert** if worse than previous

## Configuration

See `evoclaw.example.json` for a complete configuration example.

### Key Sections

**Server**
```json
{
  "port": 8420,
  "dataDir": "./data",
  "logLevel": "info"
}
```

**Model Providers**
```json
{
  "providers": {
    "anthropic": {
      "apiKey": "YOUR_KEY",
      "models": [...]
    }
  },
  "routing": {
    "simple": "ollama/llama3.2:3b",
    "complex": "anthropic/claude-sonnet-4-20250514",
    "critical": "anthropic/claude-opus-4-20250514"
  }
}
```

**Evolution**
```json
{
  "enabled": true,
  "evalIntervalSec": 3600,
  "minSamplesForEval": 10,
  "maxMutationRate": 0.2
}
```

## Development

### Build Orchestrator

```bash
go build -ldflags="-s -w" -o evoclaw ./cmd/evoclaw
```

### Build Edge Agent

```bash
cd edge-agent
cargo build --release
```

### Run Tests

```bash
go test ./...
cd edge-agent && cargo test
```

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

## Roadmap

### ✅ Done
- [x] Go orchestrator core
- [x] Telegram channel
- [x] MQTT channel (agent ↔ orchestrator)
- [x] Multi-provider model router (Anthropic, OpenAI, Ollama, OpenRouter, NVIDIA NIM)
- [x] Cost tracking
- [x] Agent registry + memory
- [x] HTTP API
- [x] Evolution engine integration
- [x] Rust edge agent (1.8MB, runs on Raspberry Pi)
- [x] Intelligent model routing (task complexity → model selection)

### 🚧 In Progress
- [ ] **Terminal TUI** — Interactive chat interface for headless/SSH environments (Bubble Tea)
- [ ] **Prompt mutation** — LLM-powered strategy improvement (evolve system prompts)

### 📋 Planned
- [ ] **Agent skill system** — Hot-loadable skills (tools, APIs, protocols) per agent
- [ ] **Distributed agent mesh** — Peer-to-peer agent discovery and collaboration
- [ ] **Container isolation** — Firecracker/gVisor sandboxing for untrusted agent code
- [ ] **Advanced evolution** — Genetic algorithms, tournament selection, crossover
- [ ] **Web dashboard** — Monitoring UI (low priority — TUI covers most needs)
- [ ] **ClawChain integration** — On-chain agent identity, reputation, payments

## License

MIT

## Built By

**Alex Chen** (alex.chen31337@gmail.com)  
For the best of [ClawChain](https://github.com/clawinfra) 🧬

---

*Every device is an agent. Every agent evolves.*
