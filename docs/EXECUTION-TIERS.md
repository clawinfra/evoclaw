# EvoClaw Execution Tiers

> *The agent adapts to its container. The container is your choice.* 🌊

---

## Overview

EvoClaw agents can run in three execution tiers. The default is **native** — full OS access, maximum power. Sandboxing is always **opt-in**, never forced.

```
┌──────────────────────────────────────────────────────────┐
│                    Execution Tiers                        │
│                                                          │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐     │
│  │   Native    │  │   Podman    │  │     E2B     │     │
│  │  (Default)  │  │  (Opt-in)   │  │  (Opt-in)   │     │
│  │             │  │             │  │             │     │
│  │  Full OS    │  │  Local      │  │  Cloud      │     │
│  │  Access     │  │  Sandbox    │  │  Sandbox    │     │
│  │             │  │             │  │             │     │
│  │  bash, fs,  │  │  Rootless,  │  │  Remote VM, │     │
│  │  network,   │  │  contained, │  │  zero local │     │
│  │  everything │  │  your machine│  │  footprint  │     │
│  └─────────────┘  └─────────────┘  └─────────────┘     │
│                                                          │
│  Power ████████████  ███████░░░░░  █████░░░░░░░        │
│  Safety ████░░░░░░░  ████████░░░░  ████████████        │
│  Isolation  None        Container     Full VM           │
│  Latency    Zero        ~Zero         Network           │
└──────────────────────────────────────────────────────────┘
```

---

## Tier 1: Native Binary (Default)

**The agent lives on your machine with full OS access.**

This is the default and recommended mode. EvoClaw was designed to be a **real agent** — not a chatbot in a cage. To execute bash commands, manage files, interact with system services, and operate autonomously, the agent needs native access.

### Installation

```bash
# Linux / macOS
curl -fsSL https://evoclaw.win/install.sh | sh

# Or download directly
# Linux x86_64
wget https://github.com/clawinfra/evoclaw/releases/latest/download/evoclaw-linux-amd64
# Linux ARM64 (Raspberry Pi 4, etc.)
wget https://github.com/clawinfra/evoclaw/releases/latest/download/evoclaw-linux-arm64
# macOS Apple Silicon
wget https://github.com/clawinfra/evoclaw/releases/latest/download/evoclaw-darwin-arm64
# macOS Intel
wget https://github.com/clawinfra/evoclaw/releases/latest/download/evoclaw-darwin-amd64
# Windows
wget https://github.com/clawinfra/evoclaw/releases/latest/download/evoclaw-windows-amd64.exe

chmod +x evoclaw-*
```

### Capabilities

| Capability | Available |
|------------|-----------|
| Bash / shell commands | ✅ |
| Filesystem read/write | ✅ |
| Network access | ✅ |
| System service management | ✅ |
| Hardware access (GPIO, sensors) | ✅ |
| Package installation | ✅ |
| Self-evolution (modify own config) | ✅ |
| Tiered memory (local + cloud) | ✅ |
| On-chain identity (ClawChain + execution chains) | ✅ |

### When to Use

- **Power users** who want maximum agent capability
- **Edge devices** (Raspberry Pi, IoT) where containers add overhead
- **Trading agents** that need low-latency system access
- **DevOps agents** managing infrastructure
- **Any task requiring OS-level operations**

### Security Model

The agent runs as **your user**. It has the same permissions you do. This is by design — an agent that can't execute commands is just a chatbot.

For sensitive environments, use Tier 2 (Podman) or Tier 3 (E2B).

---

## Tier 2: Podman Container (Opt-in Local Sandbox)

**The agent runs in a rootless container on your machine.**

Podman provides local isolation without a daemon. The agent runs on your hardware but inside a contained environment. It can still do useful work, but filesystem access is scoped and system commands are sandboxed.

### Why Podman, Not Docker?

- **Rootless** — No root daemon, no privilege escalation risk
- **Daemonless** — No background service eating resources
- **Systemd-native** — Integrates with Quadlet for service management
- **OCI-compatible** — Same container images, different runtime
- **Security** — Better default security posture than Docker

### Installation

```bash
# Install Podman (if not already available)
# Fedora/RHEL
sudo dnf install podman

# Ubuntu/Debian
sudo apt install podman

# macOS
brew install podman

# Run EvoClaw in Podman
podman run -d \
  --name evoclaw \
  -v evoclaw-data:/data \
  -p 8080:8080 \
  ghcr.io/clawinfra/evoclaw

# Or use Quadlet for systemd integration
# Copy the quadlet file
cp evoclaw.container ~/.config/containers/systemd/
systemctl --user daemon-reload
systemctl --user start evoclaw
```

### Capabilities

| Capability | Available |
|------------|-----------|
| Bash / shell commands | ✅ (inside container) |
| Filesystem read/write | ✅ (mounted volumes only) |
| Network access | ✅ (configurable) |
| System service management | ❌ (host isolated) |
| Hardware access (GPIO, sensors) | ⚠️ (requires --device mount) |
| Package installation | ✅ (inside container) |
| Self-evolution (modify own config) | ✅ (within volume) |
| Tiered memory (local + cloud) | ✅ |
| On-chain identity | ✅ |

### When to Use

- **Multi-tenant environments** where agents share a machine
- **Untrusted workloads** or experimental agent configs
- **Compliance requirements** that mandate process isolation
- **Users who want sandboxing** without leaving their hardware

### Volume Mounts

```bash
# Mount specific directories the agent needs
podman run -d \
  -v ~/evoclaw-data:/data \           # Agent state + memory
  -v ~/projects:/workspace:ro \        # Read-only project access
  -v ~/.ssh:/ssh:ro \                  # SSH keys (read-only)
  ghcr.io/clawinfra/evoclaw
```

---

## Tier 3: E2B Cloud Sandbox (Opt-in Remote)

**The agent runs in an isolated cloud VM. Zero local footprint.**

[E2B](https://e2b.dev) provides on-demand cloud sandboxes — ephemeral VMs that spin up in seconds. The agent gets a full Linux environment in the cloud, completely isolated from your machine.

### Installation

```bash
# Configure E2B provider
evoclaw config set sandbox.provider e2b
evoclaw config set sandbox.api_key <your-e2b-key>

# Launch agent in cloud sandbox
evoclaw sandbox start

# Or one-liner
evoclaw sandbox --provider e2b
```

### Capabilities

| Capability | Available |
|------------|-----------|
| Bash / shell commands | ✅ (in cloud VM) |
| Filesystem read/write | ✅ (ephemeral, cloud storage) |
| Network access | ✅ (cloud egress) |
| System service management | ✅ (within VM) |
| Hardware access | ❌ (no local hardware) |
| Package installation | ✅ (full sudo in VM) |
| Self-evolution | ✅ (within sandbox lifecycle) |
| Tiered memory | ✅ (cloud sync via Turso) |
| On-chain identity | ✅ |

### When to Use

- **Quick experimentation** — try EvoClaw without installing anything
- **Ephemeral tasks** — spin up, do work, tear down
- **CI/CD pipelines** — agent-as-a-service in automation
- **Demos and presentations** — no local setup required
- **High-security workloads** — complete isolation from local environment

### Lifecycle

```
Start → Agent boots in cloud VM (2-5 seconds)
      → Full Linux environment with EvoClaw pre-installed
      → Agent operates autonomously
      → Memory syncs to Turso (cloud persistence)
      → On-chain identity works normally
Stop  → VM destroyed, no trace on your machine
      → Memory persists in Turso cold storage
      → On-chain identity persists on ClawChain
```

### Cloud Sync Integration

E2B sandboxes are ephemeral — the VM is destroyed when done. But EvoClaw's **Tiered Memory** with **Cloud Sync** means nothing is lost:

- **Hot memory** → Rebuilt on next boot from warm tier
- **Warm memory** → Synced to Turso in real-time
- **Cold archive** → Always in Turso, survives any sandbox destruction
- **On-chain identity** → Lives on ClawChain, independent of sandbox

The device is a vessel. The soul flows through the cloud. Break the vessel, pour into a new one. Same water. 🌊

---

## Comparison Matrix

| Feature | Native | Podman | E2B |
|---------|--------|--------|-----|
| **Default** | ✅ Yes | Opt-in | Opt-in |
| **OS Access** | Full | Contained | Cloud VM |
| **Latency** | Zero | ~Zero | Network |
| **Local Footprint** | Binary (7.2MB) | Container image | Zero |
| **Persistence** | Local + Cloud | Volume + Cloud | Cloud only |
| **Hardware Access** | Full | Configurable | None |
| **Multi-platform** | Linux/macOS/Windows/Pi | Linux/macOS | Any (browser) |
| **Setup Time** | < 1 min | < 2 min | < 30 sec |
| **Cost** | Free | Free | E2B pricing |
| **Best For** | Power users, edge | Multi-tenant, compliance | Experimentation, CI |

---

## Configuration

```toml
# evoclaw.toml

[execution]
# Tier: "native" (default), "podman", "e2b"
tier = "native"

[execution.podman]
# Only used when tier = "podman"
image = "ghcr.io/clawinfra/evoclaw:latest"
volumes = ["evoclaw-data:/data"]
network = "host"  # or "bridge" for network isolation

[execution.e2b]
# Only used when tier = "e2b"
api_key = "${E2B_API_KEY}"
template = "evoclaw-agent"
timeout = "1h"
keep_alive = true
```

---

## Philosophy

EvoClaw doesn't force you into a box. The default is freedom — full OS access, maximum power. If you want containment, you **choose** it.

> *"The agent adapts to its container. The container is your choice."*

A trading agent on your desktop? Native — it needs speed.
An experimental agent you don't fully trust? Podman — contained but local.
A quick demo for a colleague? E2B — zero setup, full experience.

Same agent. Same evolution. Same memory. Different container.

**Be water, my agent.** 🌊🧬

---

*Last updated: 2026-02-09*
