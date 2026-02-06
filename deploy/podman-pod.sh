#!/usr/bin/env bash
# EvoClaw — Podman Pod Setup
#
# Alternative to podman-compose: creates a single Podman pod with all services.
# All containers share the same network namespace (localhost communication).
#
# Usage:
#   ./deploy/podman-pod.sh up       — Create pod and start all containers
#   ./deploy/podman-pod.sh down     — Stop and remove pod
#   ./deploy/podman-pod.sh status   — Show pod status
#   ./deploy/podman-pod.sh restart  — Restart all containers in the pod
#   ./deploy/podman-pod.sh logs     — Tail logs from all containers

set -euo pipefail

POD_NAME="evoclaw"
MOSQUITTO_IMAGE="eclipse-mosquitto:2"
ORCHESTRATOR_IMAGE="evoclaw-orchestrator"
EDGE_AGENT_IMAGE="evoclaw-edge-agent"

# Paths (relative to repo root)
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"

MOSQUITTO_CONF="${REPO_ROOT}/docker/mosquitto.conf"
EVOCLAW_CONFIG="${REPO_ROOT}/evoclaw.json"
AGENT_CONFIG="${REPO_ROOT}/edge-agent/agent.toml"

# ── Colors ──────────────────────────────────────────────────────
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m' # No Color

log()   { echo -e "${GREEN}[evoclaw]${NC} $*"; }
warn()  { echo -e "${YELLOW}[evoclaw]${NC} $*"; }
err()   { echo -e "${RED}[evoclaw]${NC} $*" >&2; }

# ── Functions ───────────────────────────────────────────────────

check_images() {
    local missing=0
    for img in "$ORCHESTRATOR_IMAGE" "$EDGE_AGENT_IMAGE"; do
        if ! podman image exists "$img" 2>/dev/null; then
            warn "Image '$img' not found locally."
            missing=1
        fi
    done
    if [[ $missing -eq 1 ]]; then
        warn "Build images first: make build"
        warn "Continuing anyway (podman will try to pull)..."
    fi
}

check_configs() {
    if [[ ! -f "$EVOCLAW_CONFIG" ]]; then
        err "Missing $EVOCLAW_CONFIG — copy evoclaw.example.json and edit it."
        exit 1
    fi
    if [[ ! -f "$AGENT_CONFIG" ]]; then
        err "Missing $AGENT_CONFIG — copy agent.example.toml and edit it."
        exit 1
    fi
}

pod_up() {
    log "Creating EvoClaw pod..."
    check_configs
    check_images

    # Remove existing pod if present
    if podman pod exists "$POD_NAME" 2>/dev/null; then
        warn "Pod '$POD_NAME' already exists. Removing..."
        podman pod rm -f "$POD_NAME"
    fi

    # Create pod with published ports
    podman pod create \
        --name "$POD_NAME" \
        -p 8420:8420 \
        -p 1883:1883

    log "Starting Mosquitto MQTT broker..."
    podman run -d \
        --pod "$POD_NAME" \
        --name "${POD_NAME}-mosquitto" \
        -v "${MOSQUITTO_CONF}:/mosquitto/config/mosquitto.conf:ro,Z" \
        --restart unless-stopped \
        "$MOSQUITTO_IMAGE"

    # Wait for MQTT to be ready
    log "Waiting for MQTT broker..."
    for i in $(seq 1 15); do
        if podman exec "${POD_NAME}-mosquitto" \
            mosquitto_sub -t '$SYS/#' -C 1 -W 2 >/dev/null 2>&1; then
            break
        fi
        sleep 1
    done

    log "Starting Go orchestrator..."
    podman run -d \
        --pod "$POD_NAME" \
        --name "${POD_NAME}-orchestrator" \
        -v "${EVOCLAW_CONFIG}:/app/evoclaw.json:ro,Z" \
        -e LOG_LEVEL=info \
        --restart unless-stopped \
        "$ORCHESTRATOR_IMAGE"

    # Wait for orchestrator health
    log "Waiting for orchestrator..."
    for i in $(seq 1 20); do
        if podman exec "${POD_NAME}-orchestrator" \
            wget -qO- http://localhost:8420/api/status >/dev/null 2>&1; then
            break
        fi
        sleep 1
    done

    log "Starting Rust edge agent..."
    podman run -d \
        --pod "$POD_NAME" \
        --name "${POD_NAME}-edge-agent" \
        -v "${AGENT_CONFIG}:/app/agent.toml:ro,Z" \
        -e RUST_LOG=info \
        --restart unless-stopped \
        "$EDGE_AGENT_IMAGE"

    echo ""
    log "✅ EvoClaw pod is running!"
    echo ""
    log "   Orchestrator: http://localhost:8420/api/status"
    log "   MQTT Broker:  localhost:1883"
    echo ""
    log "   Pod status:   podman pod ps"
    log "   Containers:   podman ps --pod --filter pod=$POD_NAME"
    log "   Logs:         podman logs -f ${POD_NAME}-orchestrator"
}

pod_down() {
    log "Stopping EvoClaw pod..."
    if podman pod exists "$POD_NAME" 2>/dev/null; then
        podman pod stop "$POD_NAME"
        podman pod rm "$POD_NAME"
        log "✅ Pod removed."
    else
        warn "Pod '$POD_NAME' not found."
    fi
}

pod_status() {
    if podman pod exists "$POD_NAME" 2>/dev/null; then
        echo ""
        log "Pod info:"
        podman pod ps --filter name="$POD_NAME"
        echo ""
        log "Containers:"
        podman ps --pod --filter pod="$POD_NAME" --format "table {{.Names}}\t{{.Status}}\t{{.Ports}}"
    else
        warn "Pod '$POD_NAME' not found. Run: $0 up"
    fi
}

pod_restart() {
    log "Restarting EvoClaw pod..."
    if podman pod exists "$POD_NAME" 2>/dev/null; then
        podman pod restart "$POD_NAME"
        log "✅ Pod restarted."
    else
        err "Pod '$POD_NAME' not found."
        exit 1
    fi
}

pod_logs() {
    if podman pod exists "$POD_NAME" 2>/dev/null; then
        # Tail logs from all containers in the pod
        podman logs -f "${POD_NAME}-orchestrator" &
        podman logs -f "${POD_NAME}-edge-agent" &
        podman logs -f "${POD_NAME}-mosquitto" &
        wait
    else
        err "Pod '$POD_NAME' not found."
        exit 1
    fi
}

generate_systemd() {
    log "Generating systemd unit files for pod..."
    local outdir="${REPO_ROOT}/deploy/systemd/generated"
    mkdir -p "$outdir"
    podman generate systemd --new --name "$POD_NAME" --files --restart-policy=always -t 30
    mv pod-*.service container-*.service "$outdir/" 2>/dev/null || true
    log "✅ Systemd files written to $outdir/"
    log "   Install: sudo cp $outdir/*.service /etc/systemd/system/"
    log "   Enable:  sudo systemctl enable --now pod-${POD_NAME}.service"
}

# ── Main ────────────────────────────────────────────────────────

case "${1:-help}" in
    up)         pod_up ;;
    down)       pod_down ;;
    status)     pod_status ;;
    restart)    pod_restart ;;
    logs)       pod_logs ;;
    systemd)    generate_systemd ;;
    *)
        echo "EvoClaw Podman Pod Manager"
        echo ""
        echo "Usage: $0 {up|down|status|restart|logs|systemd}"
        echo ""
        echo "  up       — Create pod and start all services"
        echo "  down     — Stop and remove the pod"
        echo "  status   — Show pod and container status"
        echo "  restart  — Restart all containers"
        echo "  logs     — Tail logs from all containers"
        echo "  systemd  — Generate systemd unit files for the pod"
        ;;
esac
