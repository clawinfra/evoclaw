#!/usr/bin/env bash
# intelligent-router installation script
# Auto-configures SOUL.md, AGENTS.md, and creates helper scripts

set -euo pipefail

SKILL_NAME="intelligent-router"
SKILL_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
WORKSPACE_ROOT="$(cd "$SKILL_DIR/../.." && pwd)"

echo "🔧 Installing $SKILL_NAME v2.2.0..."

# 1. Install Python dependencies (none for this skill)
echo "📦 No Python dependencies required"

# 2. Update SOUL.md
echo "📝 Updating SOUL.md..."
SOUL_FILE="$WORKSPACE_ROOT/SOUL.md"
if ! grep -q "Always classify before spawning" "$SOUL_FILE"; then
    # Add lesson to SOUL.md
    sed -i '/^\*\*Lessons learned:\*\*/a - **Always classify before spawning** — Use intelligent-router for cost optimization (80-95% savings)' "$SOUL_FILE"
    echo "   ✅ Added routing lesson to SOUL.md"
else
    echo "   ⏭️  SOUL.md already configured"
fi

# 3. Update AGENTS.md
echo "📝 Updating AGENTS.md..."
AGENTS_FILE="$WORKSPACE_ROOT/AGENTS.md"
if ! grep -q "## Sub-Agent Spawning Protocol" "$AGENTS_FILE"; then
    # Insert Sub-Agent Spawning Protocol section before "## Tools"
    cat > /tmp/spawning-protocol.txt <<'EOF'

## Sub-Agent Spawning Protocol

**Before spawning sub-agents, classify the task for optimal model selection:**

1. **Run classification:**
   ```bash
   python3 skills/intelligent-router/scripts/router.py classify "task description"
   ```

2. **Use recommended model in sessions_spawn:**
   ```python
   sessions_spawn(
       task="task description",
       model="<recommended-model-id>",
       label="descriptive-label"
   )
   ```

3. **Or use the spawn helper (shows command, doesn't execute):**
   ```bash
   python3 skills/intelligent-router/scripts/spawn_helper.py "task description"
   ```

**Why this matters:**
- Saves 80-95% on costs by using cheaper models for simple tasks
- Preserves quality by using premium models for complex work
- Automatic fallback chains if primary model fails

**Tier guidelines:**
- **SIMPLE** (monitoring, checks, summaries) → GLM-4.7, cheap models
- **MEDIUM** (code fixes, research, patches) → DeepSeek V3.2, Llama 3.3 70B
- **COMPLEX** (features, architecture, debugging) → Sonnet 4.5, Gemini 3 Pro
- **REASONING** (proofs, formal logic) → DeepSeek R1 32B, QwQ 32B
- **CRITICAL** (security, production) → Opus 4.6

**Don't guess** — let the router classify. It uses weighted 15-dimension scoring.

EOF
    sed -i '/^## Tools$/r /tmp/spawning-protocol.txt' "$AGENTS_FILE"
    rm /tmp/spawning-protocol.txt
    echo "   ✅ Added Sub-Agent Spawning Protocol to AGENTS.md"
else
    echo "   ⏭️  AGENTS.md already configured"
fi

# 4. Wrapper script already exists (spawn_helper.py)
echo "🔨 Wrapper scripts:"
echo "   ✅ spawn_helper.py already created"

# 5. No cron jobs needed for this skill
echo "⏰ No cron jobs required"

# 6. Validate installation
echo "✅ Validating installation..."

if [[ ! -f "$SKILL_DIR/scripts/spawn_helper.py" ]]; then
    echo "❌ spawn_helper.py not found"
    exit 1
fi

if [[ ! -f "$SKILL_DIR/config.json" ]]; then
    echo "❌ config.json not found"
    exit 1
fi

# Test router.py
if ! python3 "$SKILL_DIR/scripts/router.py" health > /dev/null 2>&1; then
    echo "❌ Router health check failed"
    exit 1
fi

echo ""
echo "✅ $SKILL_NAME v2.2.0 installed successfully"
echo "✅ Updated SOUL.md (routing lesson)"
echo "✅ Updated AGENTS.md (spawning protocol)"
echo "✅ Wrapper: scripts/spawn_helper.py"
echo "✅ Health check: PASSED"
echo ""
echo "📋 Usage:"
echo "   python3 skills/intelligent-router/scripts/router.py classify \"your task\""
echo "   python3 skills/intelligent-router/scripts/spawn_helper.py \"your task\""
