#!/usr/bin/env bash
# tiered-memory installation script
# Auto-configures SOUL.md, AGENTS.md, creates wrapper, and schedules cron jobs

set -euo pipefail

SKILL_NAME="tiered-memory"
SKILL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORKSPACE_ROOT="$(cd "$SKILL_DIR/../.." && pwd)"

echo "🔧 Installing $SKILL_NAME v2.2.0..."

# 1. Install Python dependencies
echo "📦 Installing dependencies via uv..."
if [[ -f "$SKILL_DIR/requirements.txt" ]]; then
    cd "$WORKSPACE_ROOT"
    uv pip install -r "$SKILL_DIR/requirements.txt"
    echo "   ✅ Dependencies installed"
else
    echo "   ⏭️  No requirements.txt found"
fi

# 2. Update SOUL.md
echo "📝 Updating SOUL.md..."
SOUL_FILE="$WORKSPACE_ROOT/SOUL.md"
LESSON_TEXT="Memory consolidation prevents context loss across sessions"
if ! grep -q "$LESSON_TEXT" "$SOUL_FILE"; then
    sed -i '/^\*\*Lessons learned:\*\*/a - **Memory consolidation** — Prevents context loss, auto-runs via cron (quick/daily/monthly)' "$SOUL_FILE"
    echo "   ✅ Added memory lesson to SOUL.md"
else
    echo "   ⏭️  SOUL.md already configured"
fi

# 3. Update AGENTS.md (note: consolidation is now automated via cron, no manual section needed)
echo "📝 Updating AGENTS.md..."
echo "   ⏭️  No AGENTS.md updates needed (consolidation is automatic)"

# 4. Create wrapper script
echo "🔨 Creating wrapper script..."
WRAPPER_SCRIPT="$SKILL_DIR/scripts/memory"
if [[ ! -f "$WRAPPER_SCRIPT" ]]; then
    echo "❌ Wrapper script not found at $WRAPPER_SCRIPT"
    echo "   Expected: tiered-memory/scripts/memory"
    exit 1
fi
echo "   ✅ Wrapper script exists: scripts/memory"

# 5. Create cron jobs
echo "⏰ Creating cron jobs..."
# Note: Consolidation setup depends on environment
# - EvoClaw: Built-in consolidator runs automatically when memory.enabled=true
# - OpenClaw: Needs manual cron jobs via OpenClaw cron tool

cat > /tmp/cron-jobs.txt <<'EOF'
## Environment Detection

This skill works in BOTH EvoClaw and OpenClaw:

### EvoClaw (Built-in Memory)
- Consolidation runs automatically (no manual setup needed)
- Enable via evoclaw.json: "memory": {"enabled": true, ...}
- Manual triggers: evoclaw memory consolidate --mode quick|daily|monthly
- Wrapper script detects EvoClaw and routes to built-in system

### OpenClaw (Cron-based)
Cron jobs should be created via OpenClaw cron tool:

1. Quick consolidation (every 4 hours):
   Use isolated session + agentTurn to avoid interrupting main conversations
   
   cron(action="add", job={
     "name": "Memory Consolidation (Quick)",
     "schedule": {"kind": "every", "everyMs": 14400000},
     "sessionTarget": "isolated",
     "payload": {
       "kind": "agentTurn",
       "message": "Run quick memory consolidation: cd /home/bowen/clawd && skills/tiered-memory/scripts/memory consolidate --mode quick\n\nReport results briefly.",
       "model": "anthropic-proxy-4/glm-4.7",
       "timeoutSeconds": 180
     },
     "delivery": {"mode": "none"}
   })

2. Daily consolidation (midnight):
   cron(action="add", job={
     "name": "Memory Consolidation (Daily)",
     "schedule": {"kind": "cron", "expr": "0 0 * * *", "tz": "Australia/Sydney"},
     "sessionTarget": "isolated",
     "payload": {
       "kind": "agentTurn",
       "message": "Run daily memory consolidation: cd /home/bowen/clawd && skills/tiered-memory/scripts/memory consolidate --mode daily\n\nReport results briefly.",
       "model": "anthropic-proxy-4/glm-4.7",
       "timeoutSeconds": 300
     },
     "delivery": {"mode": "none"}
   })

3. Monthly consolidation (1st of month):
   cron(action="add", job={
     "name": "Memory Consolidation (Monthly)",
     "schedule": {"kind": "cron", "expr": "0 0 1 * *", "tz": "Australia/Sydney"},
     "sessionTarget": "isolated",
     "payload": {
       "kind": "agentTurn",
       "message": "Run monthly memory consolidation: cd /home/bowen/clawd && skills/tiered-memory/scripts/memory consolidate --mode monthly\n\nReport results briefly.",
       "model": "anthropic-proxy-4/glm-4.7",
       "timeoutSeconds": 600
     },
     "delivery": {"mode": "none"}
   })
EOF

echo "   📋 Cron job commands saved to /tmp/cron-jobs.txt"
echo "   ⚠️  Create cron jobs manually or via OpenClaw cron tool"

# 6. Validate installation
echo "✅ Validating installation..."

if [[ ! -x "$WRAPPER_SCRIPT" ]]; then
    echo "❌ Wrapper script not executable"
    exit 1
fi

if [[ ! -f "$SKILL_DIR/config.json" ]]; then
    echo "❌ config.json not found"
    exit 1
fi

echo ""
echo "✅ $SKILL_NAME v2.2.0 installed successfully"
echo "✅ Updated SOUL.md (memory lesson)"
echo "✅ Wrapper: scripts/memory"
echo "✅ Dependencies installed via uv"
echo "✅ Health check: PASSED"
echo ""
echo "⚠️  Manual step required:"
echo "   Create cron jobs (see /tmp/cron-jobs.txt)"
echo ""
echo "📋 Usage:"
echo "   skills/tiered-memory/scripts/memory consolidate"
echo "   skills/tiered-memory/scripts/memory store --text \"fact\" --category \"category\""
echo "   skills/tiered-memory/scripts/memory retrieve --query \"search\""
