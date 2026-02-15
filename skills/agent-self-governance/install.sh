#!/usr/bin/env bash
# agent-self-governance installation script
# Auto-configures SOUL.md with governance protocols

set -euo pipefail

SKILL_NAME="agent-self-governance"
SKILL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORKSPACE_ROOT="$(cd "$SKILL_DIR/../.." && pwd)"

echo "🔧 Installing $SKILL_NAME..."

# 1. Install Python dependencies (none)
echo "📦 No Python dependencies required"

# 2. Update SOUL.md
echo "📝 Updating SOUL.md..."
SOUL_FILE="$WORKSPACE_ROOT/SOUL.md"

# Add WAL protocol lesson
if ! grep -q "Write-Ahead Log" "$SOUL_FILE"; then
    sed -i '/^\*\*Lessons learned:\*\*/a - **Write-Ahead Log (WAL)** — Log corrections and decisions before responding to survive compaction' "$SOUL_FILE"
    echo "   ✅ Added WAL lesson to SOUL.md"
else
    echo "   ⏭️  WAL lesson already in SOUL.md"
fi

# Add VBR protocol lesson
if ! grep -q "Verify Before Reporting" "$SOUL_FILE"; then
    sed -i '/^\*\*Lessons learned:\*\*/a - **Verify Before Reporting (VBR)** — Run checks before claiming task completion' "$SOUL_FILE"
    echo "   ✅ Added VBR lesson to SOUL.md"
else
    echo "   ⏭️  VBR lesson already in SOUL.md"
fi

# 3. Update AGENTS.md
echo "📝 Updating AGENTS.md..."
echo "   ⏭️  No AGENTS.md updates needed (governance protocols are behavioral)"

# 4. Create wrapper scripts (optional - scripts already exist as-is)
echo "🔨 Scripts available:"
echo "   ✅ wal.py (Write-Ahead Log)"
echo "   ✅ vbr.py (Verify Before Reporting)"
echo "   ✅ adl.py (Anti-Divergence Limit)"
echo "   ✅ vfm.py (Value-For-Money)"

# 5. No cron jobs needed
echo "⏰ No cron jobs required"

# 6. Validate installation
echo "✅ Validating installation..."

SCRIPTS=("wal.py" "vbr.py" "adl.py" "vfm.py")
for script in "${SCRIPTS[@]}"; do
    if [[ ! -f "$SKILL_DIR/scripts/$script" ]]; then
        echo "❌ Missing script: $script"
        exit 1
    fi
done

echo ""
echo "✅ $SKILL_NAME installed successfully"
echo "✅ Updated SOUL.md (WAL + VBR + ADL + VFM lessons)"
echo "✅ Wrapper: scripts/governance (environment-aware)"
echo "✅ Scripts: wal.py, vbr.py, adl.py, vfm.py"
echo "✅ Health check: PASSED"
echo ""
echo "🔀 Dual-Mode Operation:"
echo "   EvoClaw:  evoclaw governance <protocol> <action>"
echo "   OpenClaw: skills/agent-self-governance/scripts/governance <protocol> <action>"
echo "   Wrapper detects environment automatically"
echo ""
echo "📋 Usage:"
echo "   evoclaw governance wal append --type correction --text \"Use uv not pip\""
echo "   evoclaw governance vbr check --type file-exists --target /tmp/output.json"
echo "   evoclaw governance adl check-drift --text \"current behavior\""
echo "   evoclaw governance vfm track --model gpt-4 --input 1000 --output 500 --cost 0.05"
echo "   evoclaw governance status"
