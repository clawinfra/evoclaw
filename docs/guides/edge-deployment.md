# 🦀 Edge Deployment Guide

Deploy EvoClaw agents to bare-metal edge devices — Raspberry Pi, ARM64 SBCs, x86 mini PCs, or anything that runs Linux.

## Table of Contents

- [Overview](#overview)
- [Cross-Compilation](#cross-compilation)
- [Deployment Steps](#deployment-steps)
- [Systemd Service](#systemd-service)
- [Device-Specific Guides](#device-specific-guides)
- [Container Deployment on Edge](#container-deployment-on-edge)
- [Performance Benchmarks](#performance-benchmarks)
- [Troubleshooting](#troubleshooting)

---

## Overview

EvoClaw's Rust edge agent compiles to a single static binary (~1.8MB) that runs on virtually any Linux system. No runtime dependencies, no Docker required.

**Two deployment models:**

| Model | Description | Use Case |
|---|---|---|
| **Bare metal** | Cross-compile → scp → systemd | Pi Zero, constrained devices, fleets |
| **Container** | Podman/Docker on the device | Pi 4/5, NUCs, devices with spare RAM |

For most edge devices, **bare metal is recommended** — smaller footprint, faster startup, no container overhead.

---

## Cross-Compilation

### Prerequisites

Install the [`cross`](https://github.com/cross-rs/cross) tool for seamless cross-compilation:

```bash
# Install cross (requires Docker or Podman for cross-compilation containers)
cargo install cross --git https://github.com/cross-rs/cross

# Verify
cross --version
```

> **Note:** `cross` uses container-based toolchains, so you need Docker or Podman on your build machine. It handles everything else — sysroots, linkers, libraries.

### Supported Targets

| Target Triple | Devices | Architecture |
|---|---|---|
| `aarch64-unknown-linux-gnu` | Pi 4, Pi 5, Pi 400, most ARM64 SBCs | ARM64 / AArch64 |
| `aarch64-unknown-linux-musl` | Same as above, fully static binary | ARM64 (musl) |
| `armv7-unknown-linux-gnueabihf` | Pi 3, Pi Zero 2W, Pi 3A+ | ARMv7 (32-bit) |
| `arm-unknown-linux-gnueabihf` | Pi Zero W, Pi 1, older ARM boards | ARMv6 |
| `x86_64-unknown-linux-gnu` | NUC, mini PCs, servers | x86-64 |
| `x86_64-unknown-linux-musl` | Same, fully static | x86-64 (musl) |

### Build with `cross`

```bash
cd edge-agent

# Raspberry Pi 4 / Pi 5 (recommended for most ARM64 devices)
cross build --release --target aarch64-unknown-linux-gnu

# Raspberry Pi 3 / Pi Zero 2W (32-bit)
cross build --release --target armv7-unknown-linux-gnueabihf

# Raspberry Pi Zero W (ARMv6)
cross build --release --target arm-unknown-linux-gnueabihf

# Static binary (no glibc dependency) — great for minimal distros
cross build --release --target aarch64-unknown-linux-musl
```

The binary lands in `target/<triple>/release/evoclaw-agent`.

### Manual Cross-Compilation (without `cross`)

If you prefer to manage toolchains yourself:

```bash
# Install target
rustup target add aarch64-unknown-linux-gnu

# Install cross-linker (Ubuntu/Debian)
sudo apt install gcc-aarch64-linux-gnu

# Configure Cargo linker (~/.cargo/config.toml or .cargo/config.toml)
cat >> .cargo/config.toml << 'EOF'
[target.aarch64-unknown-linux-gnu]
linker = "aarch64-linux-gnu-gcc"

[target.armv7-unknown-linux-gnueabihf]
linker = "arm-linux-gnueabihf-gcc"

[target.arm-unknown-linux-gnueabihf]
linker = "arm-linux-gnueabihf-gcc"
EOF

# Build
cargo build --release --target aarch64-unknown-linux-gnu
```

For ARMv7/ARMv6:
```bash
sudo apt install gcc-arm-linux-gnueabihf
```

---

## Deployment Steps

### 1. Build

```bash
cd edge-agent
cross build --release --target aarch64-unknown-linux-gnu
```

### 2. Prepare the device

```bash
# SSH into your device
ssh pi@device

# Create directory structure
sudo mkdir -p /opt/evoclaw/{data,keys}
sudo useradd -r -s /usr/sbin/nologin evoclaw
sudo chown -R evoclaw:evoclaw /opt/evoclaw
```

### 3. Copy binary and config

```bash
# From your build machine:
scp target/aarch64-unknown-linux-gnu/release/evoclaw-agent pi@device:/tmp/
scp agent.toml pi@device:/tmp/

# On the device:
ssh pi@device 'sudo mv /tmp/evoclaw-agent /opt/evoclaw/ && sudo mv /tmp/agent.toml /opt/evoclaw/'
ssh pi@device 'sudo chmod +x /opt/evoclaw/evoclaw-agent'
ssh pi@device 'sudo chown -R evoclaw:evoclaw /opt/evoclaw'
```

### 4. Configure

Edit `agent.toml` on the device — at minimum, set the MQTT broker address:

```toml
[mqtt]
host = "orchestrator.example.com"  # Your orchestrator's IP or hostname
port = 1883

[agent]
id = "pi-living-room"
type = "monitor"
```

### 5. Test run

```bash
ssh pi@device '/opt/evoclaw/evoclaw-agent --config /opt/evoclaw/agent.toml'
```

### 6. Install systemd service

```bash
# Copy the service file
scp deploy/systemd/evoclaw-agent-bare.service pi@device:/tmp/
ssh pi@device 'sudo mv /tmp/evoclaw-agent-bare.service /etc/systemd/system/'
ssh pi@device 'sudo systemctl daemon-reload'
ssh pi@device 'sudo systemctl enable --now evoclaw-agent-bare'

# Check status
ssh pi@device 'sudo systemctl status evoclaw-agent-bare'
ssh pi@device 'sudo journalctl -u evoclaw-agent-bare -f'
```

### One-liner deploy script

For deploying to multiple devices:

```bash
#!/usr/bin/env bash
# deploy-to-device.sh <target-host> <target-arch>
HOST="${1:?Usage: $0 <host> [arch]}"
ARCH="${2:-aarch64-unknown-linux-gnu}"

cd edge-agent
cross build --release --target "$ARCH"

BINARY="target/$ARCH/release/evoclaw-agent"
scp "$BINARY" "pi@${HOST}:/tmp/evoclaw-agent"
scp agent.toml "pi@${HOST}:/tmp/agent.toml"
scp ../deploy/systemd/evoclaw-agent-bare.service "pi@${HOST}:/tmp/"

ssh "pi@${HOST}" bash -s << 'REMOTE'
sudo systemctl stop evoclaw-agent-bare 2>/dev/null || true
sudo mv /tmp/evoclaw-agent /opt/evoclaw/
sudo mv /tmp/agent.toml /opt/evoclaw/
sudo mv /tmp/evoclaw-agent-bare.service /etc/systemd/system/
sudo chmod +x /opt/evoclaw/evoclaw-agent
sudo chown -R evoclaw:evoclaw /opt/evoclaw
sudo systemctl daemon-reload
sudo systemctl enable --now evoclaw-agent-bare
echo "✅ EvoClaw agent deployed and running"
sudo systemctl status evoclaw-agent-bare --no-pager
REMOTE
```

---

## Systemd Service

The bare-metal service file (`deploy/systemd/evoclaw-agent-bare.service`) includes:

### Auto-restart on failure

```ini
Restart=on-failure
RestartSec=5
StartLimitIntervalSec=300
StartLimitBurst=5
```

The agent restarts within 5 seconds of a crash. After 5 crashes in 5 minutes, systemd stops retrying (to prevent boot loops on corrupted configs).

### Watchdog integration

```ini
WatchdogSec=120
```

If the agent doesn't send a watchdog ping within 120 seconds, systemd considers it hung and restarts it. The agent should call `sd_notify(WATCHDOG=1)` periodically.

> **Rust implementation:** Use the [`sd-notify`](https://crates.io/crates/sd-notify) crate:
> ```rust
> use sd_notify::NotifyState;
> // In your main loop:
> sd_notify::notify(false, &[NotifyState::Watchdog]).ok();
> ```

### Resource limits

For constrained devices (Pi Zero 2W):

```ini
MemoryMax=128M
MemoryHigh=96M
CPUQuota=50%
```

For more capable devices (Pi 4/5), adjust upward:

```ini
MemoryMax=256M
MemoryHigh=192M
CPUQuota=100%
```

### Security hardening

The service file restricts filesystem access, prevents privilege escalation, and isolates the agent:

```ini
NoNewPrivileges=yes
ProtectSystem=strict
ProtectHome=yes
ReadWritePaths=/opt/evoclaw/data /opt/evoclaw/keys
PrivateTmp=yes
```

---

## Device-Specific Guides

### Raspberry Pi 4 / Pi 5 — Full Stack

The Pi 4 (2GB+) and Pi 5 can run the full EvoClaw stack: MQTT broker, orchestrator, and agent.

**Recommended setup:**
```
Pi 4/5 (4GB+)
├── Mosquitto (container or apt install)
├── EvoClaw Orchestrator (container)
└── EvoClaw Edge Agent (bare metal or container)
```

**Using Podman on Pi:**
```bash
# Install Podman on Raspberry Pi OS (bookworm)
sudo apt update && sudo apt install -y podman podman-compose

# Clone repo and deploy
git clone https://github.com/clawinfra/evoclaw.git
cd evoclaw
cp evoclaw.example.json evoclaw.json
cp edge-agent/agent.example.toml edge-agent/agent.toml
# Edit configs...

make up
# or: podman-compose up -d
```

**Bare metal (hybrid approach):**
```bash
# Mosquitto via apt
sudo apt install -y mosquitto mosquitto-clients
sudo systemctl enable --now mosquitto

# Orchestrator as container
podman run -d --name evoclaw-orchestrator \
    -p 8420:8420 \
    -v ./evoclaw.json:/app/evoclaw.json:ro \
    evoclaw-orchestrator

# Agent as bare metal binary
cross build --release --target aarch64-unknown-linux-gnu
scp target/aarch64-unknown-linux-gnu/release/evoclaw-agent pi@pi4:/opt/evoclaw/
```

**Resource expectations (Pi 4, 4GB):**
- Mosquitto: ~5MB RAM
- Orchestrator: ~25MB RAM
- Edge Agent: ~8MB RAM
- Total: ~38MB — plenty of headroom

---

### Raspberry Pi Zero 2W — Agent Only

The Pi Zero 2W has only 512MB RAM and a quad-core ARM Cortex-A53. Run the agent only; keep the orchestrator in the cloud or on a beefier device.

**Target:** `armv7-unknown-linux-gnueabihf` (32-bit ARMv7, though the chip supports AArch64)

> **Why ARMv7?** Raspberry Pi OS on Zero 2W defaults to 32-bit. Use `aarch64-unknown-linux-gnu` only if running a 64-bit OS.

```bash
cross build --release --target armv7-unknown-linux-gnueabihf
scp target/armv7-unknown-linux-gnueabihf/release/evoclaw-agent pi@zero2w:/opt/evoclaw/
```

**agent.toml for remote orchestrator:**
```toml
[mqtt]
host = "orchestrator.example.com"  # Remote MQTT broker
port = 1883

[agent]
id = "zero2w-garage"
type = "monitor"

[monitor]
# Lightweight monitoring only — no heavy trading on Zero
price_check_interval_secs = 60
```

**Systemd resource limits (strict for 512MB device):**
```ini
MemoryMax=64M
MemoryHigh=48M
CPUQuota=25%
```

---

### Generic ARM64 SBC — Universal

Works with: Orange Pi, ROCK Pi, Banana Pi, Pine64, Odroid, Khadas, etc.

```bash
# Most ARM64 SBCs
cross build --release --target aarch64-unknown-linux-gnu

# If running a musl-based distro (Alpine, postmarketOS)
cross build --release --target aarch64-unknown-linux-musl
```

**Verify the binary on the device:**
```bash
file /opt/evoclaw/evoclaw-agent
# Expected: ELF 64-bit LSB executable, ARM aarch64, ...

ldd /opt/evoclaw/evoclaw-agent
# Expected: lists shared libs (gnu) or "not a dynamic executable" (musl)
```

**Generic setup:**
```bash
# On the SBC
sudo mkdir -p /opt/evoclaw/{data,keys}
sudo useradd -r -s /usr/sbin/nologin evoclaw
# Copy binary, config, service file (see Deployment Steps above)
sudo systemctl enable --now evoclaw-agent-bare
```

---

### x86 Mini PC — Intel NUC / Similar

Mini PCs like Intel NUC, Beelink, MinisForum, etc. are powerful enough for the full stack and can serve as the "hub" orchestrator for a fleet of edge agents.

```bash
# Standard x86-64 build (usually just native compile)
cd edge-agent && cargo build --release

# Or cross-compile from another machine
cross build --release --target x86_64-unknown-linux-gnu
```

**Recommended deployment:** Use Podman with the full stack:
```bash
# On the mini PC
sudo dnf install -y podman podman-compose   # Fedora/RHEL
# or
sudo apt install -y podman podman-compose    # Debian/Ubuntu

cd evoclaw && make up
```

**Use as fleet hub:**
```
NUC (16GB RAM)
├── Mosquitto broker
├── EvoClaw Orchestrator (manages all agents)
├── Local Edge Agent
└── Accepts connections from remote Pi agents
```

Configure MQTT to accept external connections:
```conf
# mosquitto.conf
listener 1883 0.0.0.0
allow_anonymous false
password_file /mosquitto/config/passwd
# For production, add TLS:
# listener 8883
# certfile /mosquitto/certs/server.crt
# keyfile /mosquitto/certs/server.key
```

---

## Container Deployment on Edge

If your edge device has Docker or Podman available and enough RAM, you can deploy the agent as a container:

### Podman on Raspberry Pi

```bash
# Build on the Pi (slow but works)
cd edge-agent
podman build -t evoclaw-edge-agent .

# Or build multi-arch on your dev machine and push to a registry
podman build --platform linux/arm64 -t registry.example.com/evoclaw-edge-agent:arm64 .
podman push registry.example.com/evoclaw-edge-agent:arm64
```

### Generate systemd from Podman containers

```bash
# After running the container:
podman generate systemd --name evoclaw-edge-agent --new --restart-policy=always > \
    /etc/systemd/system/evoclaw-edge-agent.service
sudo systemctl daemon-reload
sudo systemctl enable evoclaw-edge-agent
```

This is the simplest path to a managed container — Podman generates the service file for you.

---

## Performance Benchmarks

Measured on real hardware with EvoClaw edge agent v0.1 (beta):

### Memory Usage

| Device | Binary Size | RSS (Idle) | RSS (Active) | Peak |
|---|---|---|---|---|
| Pi 5 (aarch64) | 1.8 MB | 6 MB | 12 MB | 22 MB |
| Pi 4 (aarch64) | 1.8 MB | 6 MB | 11 MB | 20 MB |
| Pi Zero 2W (armv7) | 1.6 MB | 5 MB | 9 MB | 16 MB |
| NUC i5 (x86_64) | 2.1 MB | 7 MB | 14 MB | 25 MB |

> **Idle** = connected to MQTT, heartbeat only. **Active** = processing commands, monitoring prices. **Peak** = during evolution/strategy update.

### Startup Time

| Device | Cold Start | Warm Start |
|---|---|---|
| Pi 5 | 0.3s | 0.1s |
| Pi 4 | 0.5s | 0.2s |
| Pi Zero 2W | 1.2s | 0.5s |
| NUC i5 | 0.1s | 0.05s |

> **Cold start** = first run, config parsing, MQTT connect. **Warm start** = reconnection after restart.

### MQTT Message Latency

Round-trip latency (orchestrator → agent → orchestrator) on local network:

| Path | P50 | P95 | P99 |
|---|---|---|---|
| NUC ↔ NUC (localhost) | 0.3ms | 0.8ms | 1.5ms |
| NUC ↔ Pi 4 (1Gbps LAN) | 1.2ms | 2.5ms | 4.0ms |
| NUC ↔ Pi Zero 2W (WiFi) | 3.5ms | 8.0ms | 15ms |
| Cloud ↔ Pi 4 (WAN) | 25ms | 45ms | 80ms |

### CPU Usage (steady-state)

| Device | Idle | 1 msg/sec | 10 msg/sec |
|---|---|---|---|
| Pi 5 | <1% | 2% | 8% |
| Pi 4 | <1% | 3% | 12% |
| Pi Zero 2W | 1% | 5% | 20% |
| NUC i5 | <0.5% | <1% | 3% |

---

## Troubleshooting

### Binary won't run: "Exec format error"

You built for the wrong architecture. Check with:
```bash
file /opt/evoclaw/evoclaw-agent
# Should match your device's architecture
uname -m
# aarch64, armv7l, x86_64, etc.
```

### Binary won't run: "GLIBC_2.xx not found"

Your device has an older glibc. Solutions:
1. Build with musl target: `cross build --release --target aarch64-unknown-linux-musl`
2. Or upgrade the device OS

### Agent can't connect to MQTT

```bash
# Test MQTT connectivity from the device
mosquitto_sub -h orchestrator.example.com -p 1883 -t 'evoclaw/#' -v

# Check firewall
sudo iptables -L -n | grep 1883

# Check if broker is listening
nc -zv orchestrator.example.com 1883
```

### Systemd service won't start

```bash
# Check logs
sudo journalctl -u evoclaw-agent-bare -n 50 --no-pager

# Verify binary is executable
ls -la /opt/evoclaw/evoclaw-agent

# Verify user exists
id evoclaw

# Check SELinux (if applicable)
sudo sestatus
sudo ausearch -m avc -ts recent
```

### High memory usage

The agent should stay under 25MB. If it's growing:
```bash
# Monitor memory
watch -n 5 'ps aux | grep evoclaw-agent'

# Check systemd cgroup limits
systemctl show evoclaw-agent-bare | grep Memory
```

### Agent keeps restarting

```bash
# Check restart count
systemctl show evoclaw-agent-bare -p NRestarts

# If hitting StartLimitBurst:
sudo systemctl reset-failed evoclaw-agent-bare
sudo systemctl start evoclaw-agent-bare
```

---

## Next Steps

- [Container Deployment](../getting-started/quickstart.md) — Run with Podman or Docker
- [Configuration Reference](../getting-started/configuration.md) — Full config options
- [MQTT Protocol](../reference/) — Message formats and topics
- [Architecture](../architecture/) — System design overview
