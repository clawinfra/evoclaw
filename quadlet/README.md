# Quadlet Container Files

Systemd Quadlet `.container` files for running EvoClaw services with Podman.

## Usage

Copy files to your systemd user directory:

```bash
mkdir -p ~/.config/containers/systemd/
cp *.container ~/.config/containers/systemd/
systemctl --user daemon-reload
systemctl --user start evoclaw-mosquitto evoclaw-orchestrator evoclaw-edge-agent
```

## Files

- `evoclaw-mosquitto.container` — MQTT broker
- `evoclaw-orchestrator.container` — Go orchestrator
- `evoclaw-edge-agent.container` — Rust edge agent

Services are ordered via `After=` dependencies and auto-restart on failure.
