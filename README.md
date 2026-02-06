# 🧬 EvoClaw

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
- **🐧 Podman-First** — Daemonless, rootless containers with Docker fallback
- **📦 Edge-Ready** — Cross-compile to ARM64/ARMv7/x86, deploy with systemd

## Quick Start

### Podman / Docker (recommended)

```bash
# 1. Configure
cp evoclaw.example.json evoclaw.json
cp edge-agent/agent.example.toml edge-agent/agent.toml
# Edit both files with your API keys

# 2. Start (auto-detects Podman or Docker)
make up

# 3. Check status
curl http://localhost:8420/api/status
make status
```

> **Podman** is the recommended container runtime. Install it with `sudo apt install podman podman-compose` (Debian/Ubuntu) or `sudo dnf install podman podman-compose` (Fedora/RHEL). Docker works too — `make up-docker` forces Docker if both are installed.

### Podman Pod (alternative)

```bash
# Native Podman pods — all containers share localhost
make build
./deploy/podman-pod.sh up
```

### From Source

```bash
# Build orchestrator
go build -ldflags="-s -w" -o evoclaw ./cmd/evoclaw

# Build edge agent
cd edge-agent && cargo build --release

# Run (requires MQTT broker on localhost:1883)
./evoclaw --config evoclaw.json
```

### Development Mode

```bash
# Hot-reloading dev environment
make up-dev
# Or: podman-compose -f docker-compose.dev.yml up
```

### API Endpoints

```bash
curl http://localhost:8420/api/status                        # System status
curl http://localhost:8420/api/agents                        # List agents
curl http://localhost:8420/api/agents/assistant-1/metrics     # Agent metrics
curl -X POST http://localhost:8420/api/agents/assistant-1/evolve  # Trigger evolution
curl http://localhost:8420/api/agents/assistant-1/memory      # Conversation memory
curl http://localhost:8420/api/models                         # Available models
curl http://localhost:8420/api/costs                          # Cost tracking
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
    Telegram           MQTT          WhatsApp
         ↕                ↕
      Users      Edge Agents (Rust)
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

## Edge Deployment

EvoClaw's Rust agent cross-compiles to a single static binary for edge devices:

```bash
# Install cross-compilation tool
cargo install cross --git https://github.com/cross-rs/cross

# Build for Raspberry Pi 4/5 (ARM64)
cd edge-agent
cross build --release --target aarch64-unknown-linux-gnu

# Deploy to device
scp target/aarch64-unknown-linux-gnu/release/evoclaw-agent pi@device:/opt/evoclaw/
scp agent.toml pi@device:/opt/evoclaw/

# Install systemd service
scp deploy/systemd/evoclaw-agent-bare.service pi@device:/tmp/
ssh pi@device 'sudo mv /tmp/evoclaw-agent-bare.service /etc/systemd/system/ && sudo systemctl enable --now evoclaw-agent-bare'
```

**Supported targets:**
| Target | Devices |
|---|---|
| `aarch64-unknown-linux-gnu` | Pi 4, Pi 5, most ARM64 SBCs |
| `armv7-unknown-linux-gnueabihf` | Pi 3, Pi Zero 2W |
| `arm-unknown-linux-gnueabihf` | Pi Zero W, older ARM |
| `x86_64-unknown-linux-gnu` | Intel NUC, mini PCs |

**Performance on edge:**
| Device | Binary | RAM (idle) | RAM (active) | Startup |
|---|---|---|---|---|
| Pi 5 | 1.8 MB | 6 MB | 12 MB | 0.3s |
| Pi 4 | 1.8 MB | 6 MB | 11 MB | 0.5s |
| Pi Zero 2W | 1.6 MB | 5 MB | 9 MB | 1.2s |

→ Full guide: [docs/guides/edge-deployment.md](docs/guides/edge-deployment.md)

## Container Deployment

EvoClaw supports both Podman (recommended) and Docker:

```bash
make up              # Auto-detect runtime (Podman preferred)
make up-docker       # Force Docker
make down            # Stop all services
make build           # Build images
make logs            # Tail logs
make status          # Show container status
```

**Podman pod setup** (alternative to compose):
```bash
./deploy/podman-pod.sh up      # Create pod with all services
./deploy/podman-pod.sh status  # Show pod status
./deploy/podman-pod.sh down    # Tear down
```

**Systemd integration** for production servers:
```bash
make install-systemd
sudo systemctl enable --now evoclaw-mosquitto evoclaw-orchestrator evoclaw-edge-agent
```

→ Full guide: [docs/guides/container-deployment.md](docs/guides/container-deployment.md)

## Roadmap

- [x] Go orchestrator core
- [x] Telegram channel
- [x] MQTT channel
- [x] Multi-provider model router
- [x] Cost tracking
- [x] Agent registry + memory
- [x] HTTP API
- [x] Evolution engine integration
- [x] Docker Compose deployment
- [x] Podman-first container support
- [x] Bare metal edge deployment
- [x] Systemd service integration
- [x] CI/CD pipeline
- [ ] WhatsApp channel
- [ ] Prompt mutation (LLM-powered strategy improvement)
- [ ] Container isolation (Firecracker/gVisor)
- [ ] Distributed agent mesh
- [ ] Advanced evolution (genetic algorithms, tournament selection)
- [ ] Web dashboard UI
- [ ] TLS/mTLS for MQTT
- [ ] Agent auto-discovery (mDNS)

## License

MIT

## Built By

**Alex Chen** (alex.chen31337@gmail.com)  
For the best of [ClawChain](https://github.com/clawinfra) 🧬

---

*Every device is an agent. Every agent evolves.*
