# Installation

EvoClaw can be installed from pre-built binaries, compiled from source, or run via Docker.

## Prerequisites

- **Orchestrator**: Go 1.23+ (for building from source)
- **Edge Agent**: Rust 1.75+ with Cargo (for building from source)
- **MQTT Broker**: Mosquitto (optional, for agent mesh communication)

## Option 1: Pre-built Binaries

Download the latest release for your platform from [GitHub Releases](https://github.com/clawinfra/evoclaw/releases):

```bash
# Linux x86_64
curl -LO https://github.com/clawinfra/evoclaw/releases/latest/download/evoclaw-linux-amd64.tar.gz
tar xzf evoclaw-linux-amd64.tar.gz
sudo mv evoclaw /usr/local/bin/

# Linux ARM64 (Raspberry Pi 4, etc.)
curl -LO https://github.com/clawinfra/evoclaw/releases/latest/download/evoclaw-linux-arm64.tar.gz
tar xzf evoclaw-linux-arm64.tar.gz

# macOS
curl -LO https://github.com/clawinfra/evoclaw/releases/latest/download/evoclaw-darwin-arm64.tar.gz
tar xzf evoclaw-darwin-arm64.tar.gz
```

### Edge Agent Binary

```bash
# Linux x86_64
curl -LO https://github.com/clawinfra/evoclaw/releases/latest/download/evoclaw-agent-linux-amd64.tar.gz

# Linux ARM64
curl -LO https://github.com/clawinfra/evoclaw/releases/latest/download/evoclaw-agent-linux-arm64.tar.gz

# Linux ARMv7 (Raspberry Pi Zero, etc.)
curl -LO https://github.com/clawinfra/evoclaw/releases/latest/download/evoclaw-agent-linux-armv7.tar.gz
```

## Option 2: Build from Source

### Go Orchestrator

```bash
git clone https://github.com/clawinfra/evoclaw.git
cd evoclaw

# Build optimized binary
go build -ldflags="-s -w" -o evoclaw ./cmd/evoclaw

# Verify
./evoclaw --version
# EvoClaw v0.1.0 (built dev)
```

### Rust Edge Agent

```bash
cd edge-agent

# Build optimized release binary (3.2MB)
cargo build --release

# Binary is at target/release/evoclaw-agent
ls -la target/release/evoclaw-agent
```

### Cross-Compilation

Build for different architectures:

```bash
# Orchestrator for ARM64
GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o evoclaw-arm64 ./cmd/evoclaw

# Edge agent for ARM64
rustup target add aarch64-unknown-linux-gnu
cargo build --release --target aarch64-unknown-linux-gnu

# Edge agent for ARMv7 (Pi Zero)
rustup target add armv7-unknown-linux-gnueabihf
cargo build --release --target armv7-unknown-linux-gnueabihf
```

## Option 3: Containers (Podman / Docker)

The fastest way to get the full stack running. **Podman** is the recommended runtime (daemonless, rootless), but Docker works identically.

### Install a container runtime

```bash
# Podman (recommended)
sudo apt install -y podman podman-compose    # Debian/Ubuntu
sudo dnf install -y podman podman-compose    # Fedora/RHEL

# Docker (fallback)
# See https://docs.docker.com/engine/install/
```

### Start the full stack

```bash
git clone https://github.com/clawinfra/evoclaw.git
cd evoclaw
cp evoclaw.example.json evoclaw.json
cp edge-agent/agent.example.toml edge-agent/agent.toml
# Edit both files with your API keys

# Auto-detects Podman or Docker
make up

# Or force a specific runtime:
make up-docker              # Docker
podman-compose up -d        # Podman directly
./deploy/podman-pod.sh up   # Podman pod (native)
```

This starts:
- **Mosquitto** MQTT broker on port 1883
- **EvoClaw orchestrator** on port 8420
- **Edge agent** connected via MQTT

### Build images

```bash
# Build both images (uses auto-detected runtime)
make build

# Or individually:
podman build -t evoclaw-orchestrator -f orchestrator.Dockerfile .
podman build -t evoclaw-edge-agent -f edge-agent/Dockerfile ./edge-agent
```

### Systemd integration

For production, install systemd services:

```bash
make install-systemd
sudo systemctl enable --now evoclaw-mosquitto evoclaw-orchestrator evoclaw-edge-agent
```

→ See [Container Deployment Guide](../guides/container-deployment.md) for full details.

## Verify Installation

```bash
# Start the orchestrator
./evoclaw --config evoclaw.json

# In another terminal, check the API
curl http://localhost:8420/api/status
```

Expected output:
```json
{
  "version": "0.1.0",
  "agents": 0,
  "models": 0,
  "total_cost": 0
}
```

Open [http://localhost:8420](http://localhost:8420) to see the web dashboard.

## Next Steps

- [Configuration](configuration.md) — Set up your config file
- [Quick Start](quickstart.md) — Get running in 5 minutes
- [First Agent](first-agent.md) — Create your first EvoClaw agent
- [Container Deployment](../guides/container-deployment.md) — Podman pods, systemd, production
- [Edge Deployment](../guides/edge-deployment.md) — Deploy to Raspberry Pi and ARM devices
