#!/bin/bash
# EvoClaw Config Recovery Tool
# Restore agent configuration from Turso backup after hardware failure
# Usage: ./restore-config.sh [output_file]

set -e

OUTPUT_FILE="${1:-evoclaw-restored.json}"
TURSO_URL="${EVOCLAW_TURSO_URL}"
TURSO_TOKEN="${EVOCLAW_TURSO_TOKEN}"

# Validate credentials
if [ -z "$TURSO_URL" ] || [ -z "$TURSO_TOKEN" ]; then
    echo "❌ Missing Turso credentials!"
    echo "Set environment variables:"
    echo "  export EVOCLAW_TURSO_URL=\"libsql://your-db.turso.io\""
    echo "  export EVOCLAW_TURSO_TOKEN=\"eyJ...\""
    exit 1
fi

echo "🔍 Fetching available config backups from Turso..."

# List all backups
BACKUPS=$(curl -s "$TURSO_URL/v2/pipeline" \
  -H "Authorization: Bearer $TURSO_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"requests": [{"type": "execute", "stmt": {"sql": "SELECT id, agent_id, device_id, created_at, notes FROM config_backups ORDER BY created_at DESC LIMIT 20"}}]}')

# Check if jq is available
if ! command -v jq >/dev/null 2>&1; then
    echo "❌ jq is required for this script"
    echo "Install: apt-get install jq  # or  brew install jq"
    exit 1
fi

# Display backups
echo ""
echo "Available backups:"
echo "=================="

BACKUP_COUNT=$(echo "$BACKUPS" | jq -r '.results[0].response.result.rows | length')

if [ "$BACKUP_COUNT" -eq 0 ]; then
    echo "No backups found in database."
    exit 1
fi

echo "$BACKUPS" | jq -r '.results[0].response.result.rows[] | 
  "[\(.[0].value)] \(.[1].value) - Device: \(.[2].value[0:8])... - \(.[3].value) - \(.[4].value)"'

echo ""
echo -n "Enter backup ID to restore (or 'q' to quit): "
read BACKUP_ID

if [ "$BACKUP_ID" = "q" ] || [ -z "$BACKUP_ID" ]; then
    echo "Cancelled."
    exit 0
fi

# Fetch the specific backup
echo "📥 Fetching backup #$BACKUP_ID..."
BACKUP_DATA=$(curl -s "$TURSO_URL/v2/pipeline" \
  -H "Authorization: Bearer $TURSO_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"requests\": [{\"type\": \"execute\", \"stmt\": {\"sql\": \"SELECT config_json FROM config_backups WHERE id = ?\", \"args\": [{\"type\": \"text\", \"value\": \"$BACKUP_ID\"}]}}]}")

CONFIG_B64=$(echo "$BACKUP_DATA" | jq -r '.results[0].response.result.rows[0][0].value' 2>/dev/null)

if [ -z "$CONFIG_B64" ] || [ "$CONFIG_B64" = "null" ]; then
    echo "❌ Backup #$BACKUP_ID not found!"
    exit 1
fi

# Decode and save
if command -v base64 >/dev/null 2>&1; then
    echo "$CONFIG_B64" | base64 -d > "$OUTPUT_FILE" 2>/dev/null || echo "$CONFIG_B64" | base64 -D > "$OUTPUT_FILE"
else
    echo "❌ base64 command not found"
    exit 1
fi

if [ $? -eq 0 ] && [ -s "$OUTPUT_FILE" ]; then
    echo "✅ Config restored to: $OUTPUT_FILE"
    echo ""
    echo "Next steps:"
    echo "  1. Review the restored config: cat $OUTPUT_FILE"
    if [ -f "evoclaw.json" ]; then
        echo "  2. Backup current config: mv evoclaw.json evoclaw.json.backup"
        echo "  3. Use restored config: mv $OUTPUT_FILE evoclaw.json"
    else
        echo "  2. Use restored config: mv $OUTPUT_FILE evoclaw.json"
    fi
    echo "  4. Start agent: ./evoclaw --config evoclaw.json"
else
    echo "❌ Failed to decode config!"
    exit 1
fi
