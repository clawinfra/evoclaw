# 🐋 Container Deployment Guide

Deploy EvoClaw using Podman (recommended) or Docker.

## Table of Contents

- [Quick Start](#quick-start)
- [Podman (Recommended)](#podman-recommended)
- [Docker (Fallback)](#docker-fallback)
- [Podman Pod Setup](#podman-pod-setup)
- [Systemd Integration](#systemd-integration)
- [Production Checklist](#production-checklist)

---

## Quick Start

```bash
# 1. Configure
cp evoclaw.example.json evoclaw.json
cp edge-agent/agent.example.toml edge-agent/agent.toml
# Edit both files with your API keys and settings

# 2. Launch (auto-detects Podman or Docker)
make up

# 3. Check status
curl http://localhost:8420/api/status
make status
```

---

## Podman (Recommended)

[Podman](https://podman.io) is a daemonless, rootless container engine. It's the recommended runtime for EvoClaw because:

- **No daemon** — containers are regular processes, managed by systemd
- **Rootless** — run containers without root privileges
- **Pod support** — group containers into a shared network namespace
- **OCI-compatible** — uses the same images and Dockerfiles

### Install Podman

```bash
# Fedora/RHEL/CentOS
sudo dnf install -y podman podman-compose

# Debian/Ubuntu (22.04+)
sudo apt install -y podman podman-compose

# Arch Linux
sudo pacman -S podman podman-compose

# macOS
brew install podman podman-compose
podman machine init && podman machine start

# Verify
podman --version
podman-compose --version
```

### Build Images

```bash
# Build both images
make build

# Or individually:
podman build -t evoclaw-orchestrator -f orchestrator.Dockerfile .
podman build -t evoclaw-edge-agent -f edge-agent/Dockerfile ./edge-agent
```

### Run with podman-compose

The existing `docker-compose.yml` is fully compatible with `podman-compose`:

```bash
# Start everything
make up
# Equivalent to: podman-compose -f docker-compose.yml -p evoclaw up -d

# View logs
make logs

# Check status
make status

# Stop
make down
```

### Run with Podman Pod (Alternative)

Podman pods group containers into a shared network namespace (like Kubernetes pods). This is the most "Podman-native" approach:

```bash
# Create the pod and start all services
./deploy/podman-pod.sh up

# Status
./deploy/podman-pod.sh status

# Logs
./deploy/podman-pod.sh logs

# Stop and clean up
./deploy/podman-pod.sh down
```

Inside a pod, all containers share `localhost`. The orchestrator connects to MQTT at `localhost:1883`, just like in docker-compose.

---

## Docker (Fallback)

If Podman isn't available, Docker works identically:

```bash
# Start with Docker
make up-docker
# Equivalent to: docker compose -f docker-compose.yml -p evoclaw up -d

# Or use the script:
./scripts/deploy.sh --docker up
```

Everything else (logs, status, down) uses the same commands — the Makefile auto-detects the runtime.

---

## Podman Pod Setup

The `deploy/podman-pod.sh` script manages the pod lifecycle:

```
┌─── Pod: evoclaw ──────────────────────────────────────┐
│   Shared network namespace                             │
│   Published ports: 8420 (API), 1883 (MQTT)            │
│                                                        │
│   ┌───────────────┐ ┌──────────────┐ ┌─────────────┐ │
│   │  Mosquitto    │ │ Orchestrator │ │ Edge Agent  │ │
│   │  :1883        │ │ :8420        │ │             │ │
│   └───────────────┘ └──────────────┘ └─────────────┘ │
└────────────────────────────────────────────────────────┘
```

All containers communicate via `localhost` within the pod — no bridge networks needed.

### Generate systemd units from the pod

After the pod is running, auto-generate systemd service files:

```bash
# Generate systemd units
./deploy/podman-pod.sh systemd

# This creates files in deploy/systemd/generated/:
#   pod-evoclaw.service
#   container-evoclaw-mosquitto.service
#   container-evoclaw-orchestrator.service
#   container-evoclaw-edge-agent.service

# Install them
sudo cp deploy/systemd/generated/*.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now pod-evoclaw.service
```

This way, the entire pod starts on boot and is managed by systemd.

---

## Systemd Integration

### Pre-built service files

EvoClaw ships with systemd service files for individual containers:

```bash
# Install all three services
make install-systemd

# Enable and start
sudo systemctl enable --now evoclaw-mosquitto
sudo systemctl enable --now evoclaw-orchestrator
sudo systemctl enable --now evoclaw-edge-agent

# Check status
sudo systemctl status evoclaw-{mosquitto,orchestrator,edge-agent}
```

Service files are in `deploy/systemd/`:

| File | Description |
|---|---|
| `evoclaw-mosquitto.service` | MQTT broker (Podman container) |
| `evoclaw-orchestrator.service` | Go orchestrator (Podman container) |
| `evoclaw-edge-agent.service` | Rust agent (Podman container) |
| `evoclaw-agent-bare.service` | Rust agent (bare metal binary) |

### Rootless Podman with systemd

For rootless containers, install to user systemd:

```bash
mkdir -p ~/.config/systemd/user/
cp deploy/systemd/evoclaw-*.service ~/.config/systemd/user/

# Edit service files: change /usr/bin/podman to podman path
# Remove "sudo" references

systemctl --user daemon-reload
systemctl --user enable --now evoclaw-mosquitto
systemctl --user enable --now evoclaw-orchestrator

# Enable lingering so services start without login
loginctl enable-linger $USER
```

---

## Production Checklist

Before deploying to production:

- [ ] **MQTT authentication** — Set `allow_anonymous false` and configure password file
- [ ] **TLS for MQTT** — Enable TLS on port 8883 with proper certificates
- [ ] **API authentication** — Add auth middleware to the HTTP API
- [ ] **Config file permissions** — `chmod 600 evoclaw.json` (contains API keys)
- [ ] **Named volumes** — Ensure data persistence with named volumes
- [ ] **Resource limits** — Set memory/CPU limits in compose or systemd
- [ ] **Log rotation** — Configure journald or container log limits
- [ ] **Monitoring** — Set up health checks and alerting
- [ ] **Backups** — Back up `/app/data` volume regularly
- [ ] **Firewall** — Only expose necessary ports (8420, 1883)

---

## Next Steps

- [Edge Deployment](./edge-deployment.md) — Bare metal deployment to Pi and ARM devices
- [Configuration](../getting-started/configuration.md) — Full configuration reference
- [Quick Start](../getting-started/quickstart.md) — Getting started from scratch
