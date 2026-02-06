#!/bin/bash
# EvoClaw Edge Agent — E2B Sandbox Entrypoint
#
# This script:
# 1. Patches agent.toml with environment variables injected by E2B
# 2. Waits for MQTT broker connectivity
# 3. Starts the edge agent
#
# Environment variables (set via E2B sandbox metadata/env):
#   EVOCLAW_AGENT_ID       — Unique agent ID (default: auto-generated)
#   EVOCLAW_AGENT_TYPE     — Agent type: trader, monitor, sensor (default: trader)
#   MQTT_BROKER            — MQTT broker hostname (default: broker.evoclaw.io)
#   MQTT_PORT              — MQTT broker port (default: 1883)
#   MQTT_USERNAME          — MQTT auth username
#   MQTT_PASSWORD          — MQTT auth password
#   ORCHESTRATOR_URL       — Orchestrator HTTP API URL
#   HYPERLIQUID_API_KEY    — Trading API key
#   HYPERLIQUID_API_SECRET — Trading API secret
#   EVOCLAW_PAPER_MODE     — Enable paper trading (default: true)
#   EVOCLAW_LOG_LEVEL      — Log level: debug, info, warn, error
#   EVOCLAW_GENOME         — JSON-encoded genome/strategy parameters

set -euo pipefail

CONFIG="/opt/evoclaw/agent.toml"
AGENT_BIN="/opt/evoclaw/evoclaw-agent"
LOG_DIR="/var/log/evoclaw"

# Generate agent ID if not provided
AGENT_ID="${EVOCLAW_AGENT_ID:-e2b-$(hostname | cut -c1-8)-$(date +%s | tail -c 5)}"

echo "[entrypoint] EvoClaw Edge Agent starting in E2B sandbox"
echo "[entrypoint] Agent ID: ${AGENT_ID}"
echo "[entrypoint] Agent Type: ${EVOCLAW_AGENT_TYPE:-trader}"
echo "[entrypoint] MQTT Broker: ${MQTT_BROKER:-broker.evoclaw.io}:${MQTT_PORT:-1883}"

# Patch agent.toml with environment overrides
patch_config() {
    local tmp="/tmp/agent.toml"
    cp "$CONFIG" "$tmp"

    # Agent settings
    sed -i "s|^id = .*|id = \"${AGENT_ID}\"|" "$tmp"
    sed -i "s|^type = .*|type = \"${EVOCLAW_AGENT_TYPE:-trader}\"|" "$tmp"

    # MQTT settings
    if [ -n "${MQTT_BROKER:-}" ]; then
        sed -i "s|^broker = .*|broker = \"${MQTT_BROKER}\"|" "$tmp"
    fi
    if [ -n "${MQTT_PORT:-}" ]; then
        sed -i "s|^port = .*|port = ${MQTT_PORT}|" "$tmp"
    fi
    if [ -n "${MQTT_USERNAME:-}" ]; then
        sed -i "s|^username = .*|username = \"${MQTT_USERNAME}\"|" "$tmp"
    fi
    if [ -n "${MQTT_PASSWORD:-}" ]; then
        sed -i "s|^password = .*|password = \"${MQTT_PASSWORD}\"|" "$tmp"
    fi

    # Orchestrator
    if [ -n "${ORCHESTRATOR_URL:-}" ]; then
        sed -i "s|^url = .*|url = \"${ORCHESTRATOR_URL}\"|" "$tmp"
    fi

    # Trading credentials
    if [ -n "${HYPERLIQUID_API_KEY:-}" ]; then
        sed -i "s|^api_key = .*|api_key = \"${HYPERLIQUID_API_KEY}\"|" "$tmp"
    fi
    if [ -n "${HYPERLIQUID_API_SECRET:-}" ]; then
        sed -i "s|^api_secret = .*|api_secret = \"${HYPERLIQUID_API_SECRET}\"|" "$tmp"
    fi

    # Paper mode
    if [ -n "${EVOCLAW_PAPER_MODE:-}" ]; then
        sed -i "s|^paper_mode = .*|paper_mode = ${EVOCLAW_PAPER_MODE}|" "$tmp"
    fi

    # Log level
    if [ -n "${EVOCLAW_LOG_LEVEL:-}" ]; then
        sed -i "s|^level = .*|level = \"${EVOCLAW_LOG_LEVEL}\"|" "$tmp"
    fi

    cp "$tmp" "$CONFIG"
    rm -f "$tmp"
}

# Wait for MQTT broker to be reachable
wait_for_mqtt() {
    local broker="${MQTT_BROKER:-broker.evoclaw.io}"
    local port="${MQTT_PORT:-1883}"
    local max_attempts=30
    local attempt=0

    echo "[entrypoint] Waiting for MQTT broker ${broker}:${port}..."
    while [ $attempt -lt $max_attempts ]; do
        if mosquitto_pub -h "$broker" -p "$port" -t "evoclaw/ping" -m "ping" -q 0 \
            ${MQTT_USERNAME:+-u "$MQTT_USERNAME"} \
            ${MQTT_PASSWORD:+-P "$MQTT_PASSWORD"} \
            2>/dev/null; then
            echo "[entrypoint] MQTT broker reachable"
            return 0
        fi
        attempt=$((attempt + 1))
        sleep 1
    done

    echo "[entrypoint] WARNING: MQTT broker not reachable after ${max_attempts}s, starting anyway"
    return 0
}

# Apply genome/strategy parameters if provided
apply_genome() {
    if [ -n "${EVOCLAW_GENOME:-}" ]; then
        echo "[entrypoint] Applying genome parameters"
        # Parse JSON genome and patch strategy section
        local genome_type
        genome_type=$(echo "$EVOCLAW_GENOME" | jq -r '.type // empty' 2>/dev/null || true)
        if [ -n "$genome_type" ]; then
            sed -i "s|^type = .*|type = \"${genome_type}\"|" "$CONFIG"
        fi

        local lookback
        lookback=$(echo "$EVOCLAW_GENOME" | jq -r '.lookback_periods // empty' 2>/dev/null || true)
        if [ -n "$lookback" ]; then
            sed -i "s|^lookback_periods = .*|lookback_periods = ${lookback}|" "$CONFIG"
        fi

        local entry_thresh
        entry_thresh=$(echo "$EVOCLAW_GENOME" | jq -r '.entry_threshold // empty' 2>/dev/null || true)
        if [ -n "$entry_thresh" ]; then
            sed -i "s|^entry_threshold = .*|entry_threshold = ${entry_thresh}|" "$CONFIG"
        fi

        local exit_thresh
        exit_thresh=$(echo "$EVOCLAW_GENOME" | jq -r '.exit_threshold // empty' 2>/dev/null || true)
        if [ -n "$exit_thresh" ]; then
            sed -i "s|^exit_threshold = .*|exit_threshold = ${exit_thresh}|" "$CONFIG"
        fi

        local stop_loss
        stop_loss=$(echo "$EVOCLAW_GENOME" | jq -r '.stop_loss_pct // empty' 2>/dev/null || true)
        if [ -n "$stop_loss" ]; then
            sed -i "s|^stop_loss_pct = .*|stop_loss_pct = ${stop_loss}|" "$CONFIG"
        fi
    fi
}

# Main
patch_config
apply_genome
wait_for_mqtt

echo "[entrypoint] Starting EvoClaw edge agent..."
exec "$AGENT_BIN" \
    --id "$AGENT_ID" \
    --config "$CONFIG" \
    2>&1 | tee "${LOG_DIR}/agent.log"
