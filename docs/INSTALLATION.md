# EvoClaw Installation Guide

> *Every device is an agent. Choose how it runs.* 🧬

---

## Quick Decision Tree

```
Do you want EvoClaw on your own machine?
│
├── YES → Do you want full OS access?
│         │
│         ├── YES → Solo Native Install (Section 1)
│         │         Maximum power, single device
│         │
│         └── NO  → Do you want local sandboxing?
│                   │
│                   ├── YES → Podman Container (Section 3)
│                   │         Rootless, contained, your hardware
│                   │
│                   └── NO  → E2B Cloud Sandbox (Section 4)
│                             Zero local footprint
│
└── NO  → Multi-Device / Fleet?
          │
          ├── Orchestrator + Edge Agents (Section 2)
          │   Hub-and-spoke: Go orchestrator + Rust edge agents
          │
          └── Cloud-Only (Section 4)
              E2B sandbox, nothing on your machine
```

---

## Section 1: Solo Native Install (Single Device, Full Power)

**Best for:** Personal agents, trading bots, DevOps automation, power users.

One device. One agent. Full OS access. No orchestrator needed.

### Supported Platforms

| Platform | Architecture | Binary |
|----------|-------------|--------|
| Linux x86_64 | amd64 | `evoclaw-linux-amd64` |
| Linux ARM64 | arm64 | `evoclaw-linux-arm64` |
| macOS Apple Silicon | arm64 | `evoclaw-darwin-arm64` |
| macOS Intel | amd64 | `evoclaw-darwin-amd64` |
| Windows | amd64 | `evoclaw-windows-amd64.exe` |
| Raspberry Pi 4/5 | arm64 | `evoclaw-linux-arm64` |

### Install

#### macOS (Homebrew - Recommended)

Easiest installation method for macOS users:

```bash
# Add the tap
brew tap clawinfra/evoclaw

# Install EvoClaw
brew install evoclaw

# Verify installation
evoclaw --version
```

To upgrade:
```bash
brew update
brew upgrade evoclaw
```

#### Linux / Other Platforms

```bash
# One-liner (auto-detects platform)
curl -fsSL https://evoclaw.win/install.sh | sh

# Or build from source
git clone https://github.com/clawinfra/evoclaw.git
cd evoclaw
go build -ldflags="-s -w" -o evoclaw ./cmd/evoclaw
```

### Setup

```bash
evoclaw init
```

The wizard walks through 6 steps:

1. **Name your agent** — Becomes your on-chain identity
2. **Execution tier** — Choose "Native Binary" (default)
3. **Channels** — TUI (always on), HTTP API, Telegram, MQTT
4. **LLM provider** — Anthropic, OpenAI, Ollama (local), or any compatible endpoint
5. **Generate keypair** — sr25519 key for ClawChain identity
6. **Register on ClawChain** — Free registration + 10 CLAW auto-faucet

### Run

```bash
evoclaw start          # Start agent daemon
evoclaw tui            # Interactive terminal chat
evoclaw status         # Check health + on-chain status
```

### What You Get

- **7.2MB binary** — Go orchestrator with built-in agent
- Full bash/shell access, filesystem, network
- Self-evolution engine (genetic algorithms)
- Tiered memory (hot/warm/cold with cloud sync)
- ClawChain identity (DID)
- Multi-provider LLM routing (simple→cheap, complex→best)

### Solo Config Example

```json
{
  "agent": {
    "name": "my-agent",
    "description": "Personal autonomous agent"
  },
  "execution": { "tier": "native" },
  "channels": {
    "tui": { "enabled": true },
    "http": { "enabled": true, "port": 8420 },
    "telegram": { "enabled": true, "token": "BOT_TOKEN" }
  },
  "providers": {
    "anthropic": {
      "apiKey": "sk-ant-...",
      "models": [{ "id": "claude-sonnet-4" }]
    },
    "ollama": {
      "baseUrl": "http://localhost:11434",
      "models": [{ "id": "llama3.3" }]
    }
  },
  "routing": {
    "simple": "ollama/llama3.3",
    "complex": "anthropic/claude-sonnet-4",
    "critical": "anthropic/claude-sonnet-4"
  }
}
```

---

## Section 2: Multi-Device / Fleet (Orchestrator + Edge Agents)

**Best for:** IoT networks, distributed monitoring, multi-room setups, agent swarms.

One orchestrator (Go) coordinates multiple edge agents (Rust) across devices. Communication via MQTT.

### Architecture & Component Relationships

```
┌──────────────────────────────────────────┐
│         Hub Machine                       │
│   (Desktop / Server / Capable Pi)         │
│                                           │
│   ┌──────────────────────────────────┐   │
│   │   MQTT Broker (Mosquitto)        │   │
│   │   Message bus between all parts  │   │
│   │   Port 1883                      │   │
│   └──────────────┬───────────────────┘   │
│                  │                        │
│   ┌──────────────┴───────────────────┐   │
│   │   Orchestrator (Go, 6.9MB)       │   │
│   │   - Agent registry & discovery   │   │
│   │   - Evolution engine             │   │
│   │   - Model routing (LLM calls)    │   │
│   │   - HTTP API (:8420)             │   │
│   │   - Subscribes to MQTT topics    │   │
│   └──────────────────────────────────┘   │
└──────────────────┬───────────────────────┘
                   │ MQTT (network)
        ┌──────────┼──────────┐
        │          │          │
   ┌────┴───┐ ┌───┴────┐ ┌───┴────┐
   │ Edge 1 │ │ Edge 2 │ │ Edge N │
   │ Pi     │ │ Laptop │ │ IoT    │
   │ 1.8MB  │ │ 1.8MB  │ │ 1.8MB  │
   └────────┘ └────────┘ └────────┘
   Rust agent  Rust agent  Rust agent
   (no LLM)   (no LLM)    (no LLM)
```

### How They Relate

| Component | Role | Where It Runs |
|-----------|------|--------------|
| **MQTT Broker** | Message bus — routes messages between orchestrator and all edge agents | Hub machine (co-located with orchestrator) |
| **Orchestrator** | Brain — registers agents, routes LLM calls, runs evolution engine, serves HTTP API | Hub machine |
| **Edge Agent** | Hands — runs tools (cameras, sensors, scripts), reports to orchestrator via MQTT | Any device on the network |

**Message flow:**
```
Edge agent runs tool (camera, sensor, script)
  → Publishes result to MQTT broker
    → Orchestrator receives, decides next action (may call LLM)
      → Orchestrator publishes command to MQTT broker
        → Edge agent receives and executes
```

Edge agents are **lightweight and stateless** — they don't call LLMs directly. The orchestrator handles all intelligence. Edge agents just run tools and report back.

> **The broker always runs on the hub machine**, alongside the orchestrator. Edge devices only need the 1.8MB Rust agent binary — no broker, no LLM, no heavy dependencies.

### Step 1: Install the Orchestrator + Broker (Hub Machine)

On your main machine (desktop, server, or capable Pi). This machine runs **both** the orchestrator and the MQTT broker:

```bash
curl -fsSL https://evoclaw.win/install.sh | sh
evoclaw init    # Select "Native Binary", enable MQTT channel
evoclaw start
```

Enable MQTT in the config:

```json
{
  "channels": {
    "mqtt": {
      "enabled": true,
      "broker": "localhost",
      "port": 1883
    }
  }
}
```

You'll also need an MQTT broker (e.g., Mosquitto):

```bash
# Podman
podman run -d --name mosquitto -p 1883:1883 eclipse-mosquitto

# Or install natively
sudo apt install mosquitto    # Debian/Ubuntu
brew install mosquitto         # macOS
```

### Step 2: Install Edge Agents

On each edge device (Pi, IoT, laptop, phone):

```bash
# Download the Rust edge agent binary
# Raspberry Pi (ARM64)
wget https://github.com/clawinfra/evoclaw/releases/latest/download/evoclaw-edge-linux-arm64
chmod +x evoclaw-edge-linux-arm64
mv evoclaw-edge-linux-arm64 /usr/local/bin/evoclaw-edge

# Or build from source
cd edge-agent && cargo build --release
```

### Step 3: Configure Edge Agent

The edge agent supports **auto-discovery** via mDNS (DNS-SD). Just power it on and it finds the orchestrator automatically:

```bash
evoclaw-edge init
# → Scanning for orchestrator via mDNS...
# → Found: evoclaw-orch._evoclaw._tcp.local (192.168.1.100:8420)
# → Connected! Agent registered as "living-room-pi"
```

If auto-discovery isn't available (no mDNS on network, or custom setup), create `~/.evoclaw/agent.toml` manually:

```toml
# Edge Agent Configuration (manual fallback)
agent_id = "living-room-pi"
agent_type = "monitor"
name = "Living Room Eye 👁️"

[mqtt]
broker = "192.168.1.100"    # Orchestrator's IP (only needed if mDNS unavailable)
port = 1883
keep_alive_secs = 30

[orchestrator]
url = "http://192.168.1.100:8420"

[tools]
[tools.camera]
command = "rpicam-still -o $OUTPUT"
description = "Capture image from Pi camera"

[tools.temperature]
command = "vcgencmd measure_temp"
description = "Read CPU temperature"
```

### Step 4: Start Edge Agents

```bash
evoclaw-edge --config ~/.evoclaw/agent.toml
```

Or set up as a systemd service for auto-start:

```bash
sudo tee /etc/systemd/system/evoclaw-edge.service << 'EOF'
[Unit]
Description=EvoClaw Edge Agent
After=network.target

[Service]
Type=simple
User=pi
ExecStart=/usr/local/bin/evoclaw-edge --config /home/pi/.evoclaw/agent.toml
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl enable --now evoclaw-edge
```

### Edge Agent Capabilities

| Feature | Edge Agent |
|---------|-----------|
| Binary size | 1.8MB |
| Memory usage | ~5MB RAM |
| LLM | Via orchestrator (no local model needed) |
| Tools | Custom scripts, sensors, cameras |
| Communication | MQTT to orchestrator |
| Self-evolution | Orchestrator-managed |
| ClawChain identity | Shared via orchestrator |

---

## Section 3: Podman Container (Local Sandbox)

**Best for:** Multi-tenant setups, untrusted workloads, compliance requirements, experimentation.

Same machine, contained environment. No root daemon. The agent runs locally but can't touch your host system.

### Prerequisites

```bash
# Install Podman
sudo apt install podman        # Debian/Ubuntu
sudo dnf install podman        # Fedora/RHEL
brew install podman             # macOS
```

### Install & Run

```bash
# Basic run
podman run -d \
  --name evoclaw \
  -v evoclaw-data:/data \
  -p 8420:8420 \
  ghcr.io/clawinfra/evoclaw

# With specific volume mounts
podman run -d \
  --name evoclaw \
  -v ~/evoclaw-data:/data \
  -v ~/projects:/workspace:ro \
  -p 8420:8420 \
  ghcr.io/clawinfra/evoclaw
```

### With Systemd (Quadlet)

For auto-start on boot:

```bash
mkdir -p ~/.config/containers/systemd/

cat > ~/.config/containers/systemd/evoclaw.container << 'EOF'
[Container]
Image=ghcr.io/clawinfra/evoclaw:latest
ContainerName=evoclaw
Volume=evoclaw-data:/data
PublishPort=8420:8420

[Service]
Restart=always

[Install]
WantedBy=default.target
EOF

systemctl --user daemon-reload
systemctl --user start evoclaw
systemctl --user enable evoclaw
```

### Interactive Setup Inside Container

```bash
podman exec -it evoclaw evoclaw init
podman exec -it evoclaw evoclaw tui
```

### What's Sandboxed

| Access | Status |
|--------|--------|
| Host filesystem | ❌ Blocked (only mounted volumes) |
| Host network | Configurable (`--network host` or bridged) |
| Host processes | ❌ Isolated |
| Hardware (GPIO, USB) | ⚠️ Requires `--device` flag |
| Container filesystem | ✅ Full access |
| Container packages | ✅ Can install inside container |
| Cloud sync (Turso) | ✅ Works normally |
| ClawChain identity | ✅ Works normally |

### When to Add Hardware Access

```bash
# USB device passthrough
podman run -d --device /dev/ttyUSB0 ...

# GPIO access (Raspberry Pi)
podman run -d --device /dev/gpiomem ...

# GPU passthrough (NVIDIA)
podman run -d --gpus all ...
```

---

## Section 4: E2B Cloud Sandbox (Zero Footprint)

**Best for:** Quick demos, CI/CD pipelines, ephemeral tasks, high-security isolation.

Nothing on your machine. Agent runs in a cloud VM that spins up in seconds and vanishes when done.

### Prerequisites

- E2B account and API key from [e2b.dev](https://e2b.dev)

### Install & Run

```bash
# If you have evoclaw installed locally
evoclaw sandbox --provider e2b

# Or configure and launch
evoclaw config set sandbox.provider e2b
evoclaw config set sandbox.api_key YOUR_E2B_KEY
evoclaw sandbox start
```

### Without Local Install

You can also launch an E2B sandbox programmatically via the E2B SDK:

```python
from e2b import Sandbox

sandbox = Sandbox(template="evoclaw-agent")
sandbox.process.start("evoclaw init --non-interactive --provider anthropic --key sk-ant-...")
sandbox.process.start("evoclaw start")

# Agent is now running in the cloud
print(sandbox.get_hostname())  # Connect via HTTP API
```

### Lifecycle

```
evoclaw sandbox start
  → Cloud VM boots (2-5 seconds)
  → Full Linux environment with EvoClaw pre-installed
  → Agent operates autonomously
  → Memory syncs to Turso cloud storage in real-time

evoclaw sandbox stop
  → VM destroyed, zero trace locally
  → Memory persists in Turso (survives sandbox destruction)
  → ClawChain identity persists on-chain
  → Spin up again → same agent, same memories
```

### Persistence Across Sandboxes

E2B VMs are ephemeral, but your agent's identity and memory survive:

| Data | Where It Lives | Survives Destruction? |
|------|---------------|----------------------|
| Hot memory (session) | VM RAM | ✅ Rebuilt from warm tier on next boot |
| Warm memory | Turso cloud DB | ✅ Always |
| Cold archive | Turso cloud DB | ✅ Always |
| Agent config | Turso / re-provision | ✅ With cloud sync |
| ClawChain identity | On-chain | ✅ Always |
| Execution chain wallets | Encrypted in Turso | ✅ With cloud sync |

### Cost

E2B pricing applies for VM runtime. EvoClaw itself is free. Typical usage:
- ~$0.10/hour for a basic sandbox
- Billed per second of uptime
- No cost when sandbox is stopped

---

## Section 5: Adding Execution Chains (Optional, Any Tier)

Execution chains are **optional add-ons** for agents that need to interact with blockchains. They work the same way regardless of execution tier.

```bash
# Add BSC for DeFi trading
evoclaw chain add bsc --rpc https://bsc-dataseed.binance.org --wallet 0x...

# Add Hyperliquid for perpetual futures
evoclaw chain add hyperliquid --wallet 0x...

# Add Ethereum
evoclaw chain add ethereum --rpc https://eth.llamarpc.com --wallet 0x...

# List configured chains
evoclaw chain list

# Remove a chain
evoclaw chain remove bsc
```

### Supported Chains

| Chain | Type | Typical Use Case |
|-------|------|-----------------|
| BSC / opBNB | EVM | Low-fee DeFi, token trading |
| Ethereum | EVM | High-value DeFi, NFTs |
| Arbitrum / Base | EVM | L2 DeFi, lower gas |
| Polygon | EVM | Gaming, social |
| Solana | Solana | High-frequency trading |
| Hyperliquid | Hyperliquid | Perpetual futures |
| Any EVM chain | EVM | Custom `--rpc` + `--chain-id` |

Actions on execution chains are reported back to ClawChain for reputation tracking.

---

## Section 6: Post-Install Commands (All Tiers)

```bash
# Core
evoclaw start              # Start agent
evoclaw stop               # Stop agent
evoclaw status             # Health + on-chain status
evoclaw tui                # Terminal chat

# Identity
evoclaw identity           # View ClawChain DID
evoclaw balance            # Check CLAW balance

# Providers
evoclaw provider add <name> --base-url <url> --api-key <key> --model <model>
evoclaw provider list

# Channels
evoclaw channel add telegram --token <bot-token>
evoclaw channel add mqtt --broker <url>

# Chains (optional)
evoclaw chain add <name> --rpc <url> --wallet <addr>
evoclaw chain list
evoclaw chain remove <name>

# Maintenance
evoclaw upgrade            # Self-update binary
evoclaw logs               # View agent logs
evoclaw memory stats       # Memory tier statistics
```

---

## Comparison Matrix

| Feature | Solo Native | Multi-Device | Podman | E2B Cloud |
|---------|------------|-------------|--------|-----------|
| **Devices** | 1 | Many | 1 (contained) | 0 (cloud) |
| **OS Access** | Full | Orchestrator: full, Edge: limited | Contained | Cloud VM |
| **Binary Size** | 7.2MB | Orch: 6.9MB + Edge: 1.8MB | Container image | N/A |
| **Setup Time** | < 1 min | < 5 min | < 2 min | < 30 sec |
| **Local Footprint** | Minimal | Per device | Container | Zero |
| **Hardware Access** | Full | Full (per device) | Configurable | None |
| **LLM Location** | Local or API | Orchestrator-managed | Inside container | Cloud VM |
| **Memory** | Local + cloud | Orchestrator-managed | Volume + cloud | Cloud only |
| **Self-Evolution** | ✅ | ✅ (orchestrator-driven) | ✅ | ✅ |
| **ClawChain** | ✅ | ✅ (shared identity) | ✅ | ✅ |
| **Best For** | Power users | IoT/distributed | Compliance/security | Demos/CI |
| **Cost** | Free | Free | Free | E2B pricing |

---

## Memory Recovery Across Tiers

Hot memory is a **cache** — not a separate data store. If it's lost (crash, reboot, sandbox destruction), it rebuilds automatically:

```
Agent boots → Hot memory empty
           → Query warm tier for recent context + core facts
           → Reconstruct hot memory (~2-5 seconds)
           → Agent resumes with full context
```

| Tier | Hot Memory | Warm Memory | Cold Archive |
|------|-----------|-------------|-------------|
| Native | In-process RAM | Local + Turso sync | Turso DB |
| Multi-Device | Per-device RAM | Orchestrator + Turso | Turso DB |
| Podman | Container RAM | Volume + Turso sync | Turso DB |
| E2B | VM RAM | Turso (real-time sync) | Turso DB |

**Warm is the source of truth.** Hot memory is rebuilt from warm on every boot. Cold archive provides deep history on-demand. Losing a device or container loses nothing — warm and cold persist in Turso cloud storage.

---

## FAQ

**Q: Do I need ClawChain to use EvoClaw?**
A: Registration is automatic and free during `evoclaw init`. Your agent gets an on-chain identity without buying tokens. You can skip it with `--skip-chain` but you'll miss reputation tracking.

**Q: Can I switch execution tiers later?**
A: Yes. `evoclaw config set execution.tier podman` and restart. Your identity and memory persist across tiers thanks to cloud sync.

**Q: Can I run multiple agents on one machine?**
A: Yes. Use different config files: `evoclaw start --config agent2.json`. Each agent gets its own ClawChain identity.

**Q: What's the minimum hardware for an edge agent?**
A: The Rust edge agent runs on anything with ~5MB RAM and ARM/x86 CPU. Raspberry Pi Zero W works fine.

**Q: Do execution chains cost money?**
A: The chains themselves are free to add. Transactions on those chains cost gas (BNB, ETH, SOL, etc.) as usual.

---

*Last updated: 2026-02-11*
