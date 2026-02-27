#!/usr/bin/env bash
# EvoClaw — Container Deployment Script
#
# Podman-first, Docker-compatible.
#
# Usage:
#   ./scripts/deploy.sh up          — Start all services
#   ./scripts/deploy.sh down        — Stop all services
#   ./scripts/deploy.sh build       — Build images
#   ./scripts/deploy.sh logs        — Tail logs
#   ./scripts/deploy.sh status      — Show status
#   ./scripts/deploy.sh --docker up — Force Docker runtime

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

# ── Colors ──────────────────────────────────────────────────────
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

log()  { echo -e "${GREEN}[evoclaw]${NC} $*"; }
warn() { echo -e "${YELLOW}[evoclaw]${NC} $*"; }
err()  { echo -e "${RED}[evoclaw]${NC} $*" >&2; }

# ── Detect runtime ──────────────────────────────────────────────
FORCE_DOCKER=0

# Parse flags
while [[ "${1:-}" == --* ]]; do
    case "$1" in
        --docker)  FORCE_DOCKER=1; shift ;;
        --podman)  FORCE_DOCKER=0; shift ;;
        *)         err "Unknown flag: $1"; exit 1 ;;
    esac
done

detect_runtime() {
    if [[ $FORCE_DOCKER -eq 1 ]]; then
        COMPOSE="docker compose"
        RUNTIME="docker"
        return
    fi

    if command -v podman-compose &>/dev/null; then
        COMPOSE="podman-compose"
        RUNTIME="podman"
    elif command -v podman &>/dev/null && command -v docker-compose &>/dev/null; then
        # podman exists but no podman-compose — try docker-compose with podman socket
        COMPOSE="docker-compose"
        RUNTIME="podman"
    elif docker compose version &>/dev/null 2>&1; then
        COMPOSE="docker compose"
        RUNTIME="docker"
    elif command -v docker-compose &>/dev/null; then
        COMPOSE="docker-compose"
        RUNTIME="docker"
    else
        err "No container compose tool found."
        err "Install one of: podman-compose, docker compose, docker-compose"
        exit 1
    fi

    log "Detected runtime: $RUNTIME (compose: $COMPOSE)"
}

detect_runtime

PROJECT="evoclaw"
COMPOSE_FILE="docker-compose.yml"

# ── Commands ────────────────────────────────────────────────────

cmd_up() {
    $COMPOSE -f "$COMPOSE_FILE" -p "$PROJECT" up -d
    echo ""
    log "✅ EvoClaw is running ($RUNTIME)"
    log "   Orchestrator: http://localhost:8420/api/status"
    log "   MQTT Broker:  localhost:1883"
}

cmd_down() {
    $COMPOSE -f "$COMPOSE_FILE" -p "$PROJECT" down
    log "✅ EvoClaw stopped."
}

cmd_build() {
    log "Building orchestrator image..."
    $RUNTIME build -t evoclaw-orchestrator -f orchestrator.Dockerfile .

    log "Building edge-agent image..."
    $RUNTIME build -t evoclaw-edge-agent -f edge-agent/Dockerfile ./edge-agent

    log "✅ All images built."
}

cmd_logs() {
    $COMPOSE -f "$COMPOSE_FILE" -p "$PROJECT" logs -f
}

cmd_status() {
    $COMPOSE -f "$COMPOSE_FILE" -p "$PROJECT" ps
}

cmd_restart() {
    $COMPOSE -f "$COMPOSE_FILE" -p "$PROJECT" restart
}

# ── Main ────────────────────────────────────────────────────────

case "${1:-help}" in
    up)       cmd_up ;;
    down)     cmd_down ;;
    build)    cmd_build ;;
    logs)     cmd_logs ;;
    status)   cmd_status ;;
    restart)  cmd_restart ;;
    *)
        echo "EvoClaw Deployment Script"
        echo ""
        echo "Usage: $0 [--docker|--podman] {up|down|build|logs|status|restart}"
        echo ""
        echo "Flags:"
        echo "  --docker   Force Docker runtime"
        echo "  --podman   Force Podman runtime (default if available)"
        echo ""
        echo "Commands:"
        echo "  up         Start all services (detached)"
        echo "  down       Stop and remove containers"
        echo "  build      Build container images"
        echo "  logs       Tail logs from all services"
        echo "  status     Show container status"
        echo "  restart    Restart all services"
        ;;
esac
