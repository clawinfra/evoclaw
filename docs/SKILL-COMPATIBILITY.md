# Skill Compatibility: OpenClaw ↔ EvoClaw

**Making skills work seamlessly in both environments for smooth migration.**

---

## The Problem

EvoClaw will eventually replace OpenClaw on desktop systems. During the transition, skills must work in **both** environments without modification or user intervention.

**Challenges:**
- EvoClaw has built-in systems (memory, scheduler, governance)
- OpenClaw relies on external cron jobs and Python CLI scripts
- Skills written for OpenClaw won't work in EvoClaw (and vice versa)
- Migration path must be smooth for users

## The Solution: Environment Detection

Each skill provides:
1. **EvoClaw CLI commands** - Native Go integration with built-in systems
2. **OpenClaw Python scripts** - Standalone CLI tools
3. **Environment-aware wrapper** - Detects which system is running and routes appropriately

### Architecture Pattern

```
User runs: skills/tiered-memory/scripts/memory consolidate --mode quick

Wrapper script detects environment:
  ├─ EvoClaw present? → evoclaw memory consolidate --mode quick
  └─ EvoClaw absent?  → python3 memory_cli.py consolidate --mode quick
```

## Implementation for Each Skill

### 1. Tiered Memory

**EvoClaw (Built-in):**
```bash
evoclaw memory consolidate --mode quick|daily|monthly
evoclaw memory store --text "fact" --category "category"
evoclaw memory retrieve --query "search"
evoclaw memory status
```

**OpenClaw (Python CLI):**
```bash
python3 skills/tiered-memory/scripts/memory_cli.py consolidate --mode quick
python3 skills/tiered-memory/scripts/memory_cli.py store --text "fact" --category "cat"
python3 skills/tiered-memory/scripts/memory_cli.py retrieve --query "search"
```

**Wrapper (`skills/tiered-memory/scripts/memory`):**
```bash
#!/usr/bin/env bash
if command -v evoclaw &> /dev/null; then
    exec evoclaw memory "$@"
else
    python3 "$(dirname "$0")/memory_cli.py" "$@"
fi
```

**Consolidation:**
- **EvoClaw:** Runs automatically via built-in consolidator (1h/24h/30d intervals)
- **OpenClaw:** Requires cron jobs calling wrapper script

### 2. Intelligent Router

**EvoClaw (Built-in):**
```bash
evoclaw router classify "task description"
evoclaw router recommend "task description"
evoclaw router models
```

**OpenClaw (Python CLI):**
```bash
python3 skills/intelligent-router/scripts/router.py classify "task"
python3 skills/intelligent-router/scripts/router.py recommend "task"
python3 skills/intelligent-router/scripts/router.py models
```

**Wrapper (`skills/intelligent-router/scripts/router`):**
```bash
#!/usr/bin/env bash
if command -v evoclaw &> /dev/null; then
    exec evoclaw router "$@"
else
    python3 "$(dirname "$0")/router.py" "$@"
fi
```

### 3. Agent Self-Governance

**EvoClaw (Built-in):**
```bash
# WAL (Write-Ahead Log)
evoclaw governance wal append --type correction --text "..."
evoclaw governance wal replay
evoclaw governance wal flush

# VBR (Verify Before Reporting)
evoclaw governance vbr check --type file-exists --target /path
evoclaw governance vbr log --task-id <id> --passed true

# ADL (Anti-Divergence Limit)
evoclaw governance adl load-baseline --soul-path SOUL.md
evoclaw governance adl check-drift --text "behavior"
evoclaw governance adl report

# VFM (Value-For-Money)
evoclaw governance vfm track --model gpt-4 --input 1000 --output 500 --cost 0.05
evoclaw governance vfm check-budget
evoclaw governance vfm report

# Aggregate status
evoclaw governance status
```

**OpenClaw (Python CLI):**
```bash
python3 skills/agent-self-governance/scripts/wal.py append default correction "..."
python3 skills/agent-self-governance/scripts/vbr.py check task123 file_exists /path
python3 skills/agent-self-governance/scripts/adl.py score default
python3 skills/agent-self-governance/scripts/vfm.py log default task model tokens cost quality
```

**Wrapper (`skills/agent-self-governance/scripts/governance`):**
```bash
#!/usr/bin/env bash
if command -v evoclaw &> /dev/null; then
    exec evoclaw governance "$@"
else
    # Route to specific protocol script
    PROTOCOL="$1"; shift
    python3 "$(dirname "$0")/${PROTOCOL}.py" "$@"
fi
```

## Migration Workflow

### User Journey: OpenClaw → EvoClaw

**Phase 1: OpenClaw Only**
```bash
# User has OpenClaw installed
skills/tiered-memory/scripts/memory consolidate --mode quick
# → Wrapper detects no evoclaw → Runs Python CLI
```

**Phase 2: Install EvoClaw**
```bash
# User installs EvoClaw alongside OpenClaw
brew install evoclaw
evoclaw init
```

**Phase 3: Automatic Switch**
```bash
# Same command, now routes to EvoClaw
skills/tiered-memory/scripts/memory consolidate --mode quick
# → Wrapper detects evoclaw → Runs: evoclaw memory consolidate --mode quick

# Built-in consolidation runs automatically
# No more manual cron job triggers needed
```

**Phase 4: Full Migration**
```bash
# User can use EvoClaw CLI directly
evoclaw memory consolidate --mode quick
evoclaw router classify "task"
evoclaw governance status
```

**Phase 5: Remove OpenClaw**
```bash
# Uninstall OpenClaw, keep using EvoClaw
# Skills continue working via EvoClaw native commands
```

## Skill Installation: Auto-Configuration

When a skill is installed (via `install.sh`), it must:

### 1. Detect Environment
```bash
if command -v evoclaw &> /dev/null; then
    echo "EvoClaw detected - using native integration"
    # Configure evoclaw.json if needed
else
    echo "OpenClaw detected - using Python CLI + cron"
    # Create wrapper scripts, document cron jobs
fi
```

### 2. Configure Built-in Systems (EvoClaw)

**tiered-memory:**
```bash
# Enable memory system in evoclaw.json
evoclaw config set memory.enabled=true
evoclaw config set memory.cold.databaseUrl="..."
evoclaw config set memory.cold.authToken="..."
```

**intelligent-router:**
- No configuration needed (uses existing model catalog)

**agent-self-governance:**
- No configuration needed (stores state in `~/.evoclaw/governance/`)

### 3. Create Cron Jobs (OpenClaw)

**tiered-memory:**
```bash
# Use OpenClaw cron tool
cron(action="add", job={
  "name": "Memory Consolidation (Quick)",
  "schedule": {"kind": "interval", "intervalMs": 14400000},
  "sessionTarget": "isolated",
  "payload": {
    "kind": "agentTurn",
    "message": "Run: skills/tiered-memory/scripts/memory consolidate --mode quick"
  }
})
```

## Testing Dual-Mode Skills

### Test Script Pattern
```bash
#!/usr/bin/env bash
# Test skill in both environments

echo "Testing skill: tiered-memory"

# Test 1: Wrapper detects EvoClaw
if command -v evoclaw &> /dev/null; then
    echo "✓ EvoClaw detected"
    evoclaw memory status || exit 1
else
    echo "✓ OpenClaw mode (no evoclaw binary)"
    skills/tiered-memory/scripts/memory status || exit 1
fi

# Test 2: Wrapper script works
skills/tiered-memory/scripts/memory status || exit 1

echo "✓ All tests passed"
```

## Best Practices for Skill Authors

### 1. Always Provide Both Interfaces
- EvoClaw CLI commands (Go)
- OpenClaw Python scripts (standalone)
- Environment-aware wrapper

### 2. Document Dual-Mode Operation
In `SKILL.md`:
```markdown
## Usage

### EvoClaw
\`\`\`bash
evoclaw <skill-name> <command> [options]
\`\`\`

### OpenClaw
\`\`\`bash
skills/<skill-name>/scripts/<skill-name> <command> [options]
\`\`\`

The wrapper script detects your environment automatically.
```

### 3. Install Script Detects Environment
```bash
#!/usr/bin/env bash
if command -v evoclaw &> /dev/null; then
    echo "🔧 Configuring for EvoClaw..."
    # Configure evoclaw.json
else
    echo "🔧 Configuring for OpenClaw..."
    # Document cron jobs, create wrappers
fi
```

### 4. Test in Both Environments
Before publishing:
```bash
# Test in EvoClaw
evoclaw <skill-name> <command>

# Test in OpenClaw (simulate by hiding evoclaw binary)
PATH=/usr/bin:/bin skills/<skill-name>/scripts/<skill-name> <command>
```

## Implementation Checklist

For each core skill:

- [ ] **EvoClaw CLI** (`internal/cli/<skill-name>.go`)
  - [ ] Command registration in `cmd/evoclaw/main.go`
  - [ ] Subcommand handlers (list, add, remove, status, etc.)
  - [ ] Integration with built-in system (if applicable)

- [ ] **OpenClaw Python CLI** (`skills/<skill-name>/scripts/<name>_cli.py`)
  - [ ] Standalone script with argparse
  - [ ] No dependencies on EvoClaw/OpenClaw runtime
  - [ ] Outputs JSON for programmatic use

- [ ] **Environment-Aware Wrapper** (`skills/<skill-name>/scripts/<name>`)
  - [ ] Detect `evoclaw` binary presence
  - [ ] Route to appropriate implementation
  - [ ] Executable (`chmod +x`)

- [ ] **Install Script** (`skills/<skill-name>/install.sh`)
  - [ ] Environment detection
  - [ ] EvoClaw: Configure `evoclaw.json`
  - [ ] OpenClaw: Document cron jobs
  - [ ] Update SOUL.md, AGENTS.md

- [ ] **Documentation** (`skills/<skill-name>/SKILL.md`)
  - [ ] Dual-mode usage examples
  - [ ] Environment detection explanation
  - [ ] Migration guide

- [ ] **Testing**
  - [ ] EvoClaw integration test
  - [ ] OpenClaw standalone test
  - [ ] Wrapper script test

## Comparison: OpenClaw vs EvoClaw

| Feature | OpenClaw | EvoClaw |
|---------|----------|---------|
| **Memory** | Python CLI + cron | Built-in consolidator (auto) |
| **Router** | Python CLI | Built-in CLI + catalog |
| **Governance** | Python scripts | Built-in CLI + state tracking |
| **Scheduler** | External cron | Built-in scheduler |
| **Config** | Gateway DB | evoclaw.json |
| **Skills** | `~/.openclaw/skills/` | `~/.evoclaw/skills/` |
| **CLI** | OpenClaw tools | `evoclaw <skill> ...` |

## Future: Unified Skill Format

Long-term goal: Single skill package works in both environments without wrappers.

**Skill Manifest (`skill.toml`):**
```toml
[skill]
name = "tiered-memory"
version = "2.2.0"

[interfaces.evoclaw]
cli = "internal/cli/memory.go"
integration = "internal/memory/manager.go"

[interfaces.openclaw]
cli = "scripts/memory_cli.py"
cron = "scripts/cron-jobs.json"

[interfaces.wrapper]
script = "scripts/memory"
detect = "evoclaw"
```

Runtime loads appropriate interface automatically.

---

**See Also:**
- [Tiered Memory](./TIERED-MEMORY.md) - Full memory system docs
- [Scheduler](./SCHEDULER.md) - Built-in periodic tasks
- [Intelligent Router](../skills/intelligent-router/README.md) - Cost optimization
- [Agent Self-Governance](../skills/agent-self-governance/SKILL.md) - Reliability protocols
