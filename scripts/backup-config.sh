#!/bin/bash
# EvoClaw Automated Config Backup
# Backs up agent configuration to Turso for hardware recovery
# Usage: ./backup-config.sh [config_file] [device_id_file]

set -e

# Configuration
CONFIG_FILE="${1:-evoclaw.json}"
DEVICE_ID_FILE="${2:-.evoclaw-device-id}"
TURSO_URL="${EVOCLAW_TURSO_URL}"
TURSO_TOKEN="${EVOCLAW_TURSO_TOKEN}"

# Validate inputs
if [ ! -f "$CONFIG_FILE" ]; then
    echo "❌ Config file not found: $CONFIG_FILE"
    echo "Usage: $0 [config_file] [device_id_file]"
    exit 1
fi

if [ -z "$TURSO_URL" ] || [ -z "$TURSO_TOKEN" ]; then
    echo "❌ Missing Turso credentials!"
    echo "Set environment variables:"
    echo "  export EVOCLAW_TURSO_URL=\"libsql://your-db.turso.io\""
    echo "  export EVOCLAW_TURSO_TOKEN=\"eyJ...\""
    exit 1
fi

# Get agent IDs from config
if command -v jq >/dev/null 2>&1; then
    AGENT_IDS=$(jq -r '.agents[]?.id // .agent_id // empty' "$CONFIG_FILE" | tr '\n' ',' | sed 's/,$//')
else
    echo "⚠️  jq not found, using config filename as agent_id"
    AGENT_IDS=$(basename "$CONFIG_FILE" .json)
fi

# Get or generate device ID
if [ -f "$DEVICE_ID_FILE" ]; then
    DEVICE_ID=$(cat "$DEVICE_ID_FILE")
else
    if command -v uuidgen >/dev/null 2>&1; then
        DEVICE_ID=$(uuidgen)
    else
        DEVICE_ID=$(cat /proc/sys/kernel/random/uuid 2>/dev/null || echo "unknown-$(date +%s)")
    fi
    echo "$DEVICE_ID" > "$DEVICE_ID_FILE"
    echo "📝 Generated new device ID: ${DEVICE_ID:0:8}..."
fi

# Get current timestamp
TIMESTAMP=$(date +%s)

# Base64 encode config to avoid escaping issues
if command -v base64 >/dev/null 2>&1; then
    CONFIG_B64=$(cat "$CONFIG_FILE" | base64 -w 0 2>/dev/null || cat "$CONFIG_FILE" | base64)
else
    echo "❌ base64 command not found"
    exit 1
fi

# Ensure config_backups table exists
curl -s "$TURSO_URL/v2/pipeline" \
  -H "Authorization: Bearer $TURSO_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"requests": [{"type": "execute", "stmt": {"sql": "CREATE TABLE IF NOT EXISTS config_backups (id INTEGER PRIMARY KEY AUTOINCREMENT, agent_id TEXT NOT NULL, device_id TEXT, config_json TEXT NOT NULL, created_at TEXT NOT NULL, notes TEXT)"}}]}' \
  >/dev/null 2>&1

# Backup to Turso
RESULT=$(curl -s "$TURSO_URL/v2/pipeline" \
  -H "Authorization: Bearer $TURSO_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{
    \"requests\": [{
      \"type\": \"execute\",
      \"stmt\": {
        \"sql\": \"INSERT INTO config_backups (agent_id, device_id, config_json, created_at, notes) VALUES (?, ?, ?, ?, ?)\",
        \"args\": [
          {\"type\": \"text\", \"value\": \"$AGENT_IDS\"},
          {\"type\": \"text\", \"value\": \"$DEVICE_ID\"},
          {\"type\": \"text\", \"value\": \"$CONFIG_B64\"},
          {\"type\": \"text\", \"value\": \"$TIMESTAMP\"},
          {\"type\": \"text\", \"value\": \"Auto-backup\"}
        ]
      }
    }]
  }")

# Check if successful
if echo "$RESULT" | grep -q '"affected_row_count"' && echo "$RESULT" | grep -q '"value":"1"'; then
    echo "✅ Config backed up to Turso"
    [ -n "$AGENT_IDS" ] && echo "   Agents: $AGENT_IDS"
    echo "   Device: ${DEVICE_ID:0:8}..."
    exit 0
else
    echo "⚠️  Backup may have failed (check Turso connectivity)"
    if command -v jq >/dev/null 2>&1; then
        echo "$RESULT" | jq '.error // .results[0]' 2>/dev/null || echo "$RESULT"
    fi
    exit 1
fi
