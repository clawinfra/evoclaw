# Core Skills Infrastructure Integration

## Summary

Adds three core skills as embedded infrastructure that auto-install during `evoclaw init`:
- **tiered-memory** - Three-tier memory system with cloud sync
- **intelligent-router** - Cost-optimized model routing  
- **agent-self-governance** - WAL/VBR/ADL/VFM protocols

Also adds SOUL.md and AGENTS.md templates for agent initialization.

## Motivation

From EVOLUTION.md: *"The genome is the soul. Evolution is the journey."*

These skills are **not optional add-ons** — they're required for reliable evolution:

1. **WAL** ensures mutations survive crashes:
   > "Mutations and fitness evaluations are WAL-logged so a crash mid-evolution doesn't leave the genome in an inconsistent state."

2. **VBR** verifies mutations work:
   > "After `MutateSkill()`, call `VerifyMutation()` [...] Only commits the mutation to genome history if verified as an improvement."

3. **ADL** loads SOUL.md for persona baseline:
   > `internal/governance/adl.go` references SOUL.md but no template exists

4. **VFM** scores mutation cost-effectiveness:
   > "Mutations with `VFM < GenomeConstraints.MinVFMScore` are rejected"

5. **Tiered memory** persists across evolution cycles
6. **Router** enables VFM cost scoring

**Problem:** Go implementations exist (`internal/governance/`), but no user-facing skills for agents to invoke.

**Solution:** Bundle skills as embedded infrastructure, auto-install during init.

## Changes

### 1. Added Core Skills (`/skills/`)

```
skills/
├── tiered-memory/           # v2.2.0 - Three-tier memory + cloud sync
│   ├── SKILL.md
│   ├── config.json
│   ├── scripts/
│   │   ├── memory (wrapper with auto-credential loading)
│   │   └── memory_cli.py
│   └── install.sh
├── intelligent-router/      # v2.2.0 - 15-dimension weighted scoring
│   ├── SKILL.md
│   ├── config.json
│   ├── scripts/
│   │   ├── router.py
│   │   └── spawn_helper.py
│   └── install.sh
└── agent-self-governance/   # WAL/VBR/ADL/VFM protocols
    ├── SKILL.md
    ├── scripts/
    │   ├── wal.py, vbr.py, adl.py, vfm.py
    └── install.sh
```

### 2. Added Agent Templates (`/templates/`)

- **SOUL.md.template** - Agent persona with placeholders (`{{AGENT_NAME}}`, `{{AGENT_ROLE}}`)
  - Includes hard rules (uv for Python, core skills are infrastructure)
  - Documents self-governance protocols
  - Becomes ADL baseline for drift detection

- **AGENTS.md.template** - Behavioral rules and protocols
  - Memory management guide
  - Sub-agent spawning protocol
  - Safety guidelines

### 3. Updated `internal/cli/`

**New file: `skills_setup.go`**
- `SetupCoreSkills()` - Copies embedded skills to `~/.evoclaw/skills/`, runs `install.sh`
- `GenerateAgentFiles()` - Generates SOUL.md and AGENTS.md from templates
- `copyEmbeddedDir()` - Recursively copies embedded directories to filesystem

**Modified: `init.go`**
- Calls `SetupCoreSkills()` and `GenerateAgentFiles()` after config generation
- Non-fatal errors (warns but continues if installation fails)

### 4. Added Documentation

**`docs/SKILLS_INTEGRATION.md`** - Comprehensive guide:
- Why these skills are core infrastructure
- Installation flow diagram
- File locations after init
- Skill details (purpose, integration, commands)
- SOUL.md template explanation
- Development guide for adding new core skills
- FAQ

## Testing

Tested locally:
```bash
# Build and init
go build -o evoclaw ./cmd/evoclaw
./evoclaw init --non-interactive --provider ollama --name test-agent

# Verify installation
ls ~/.evoclaw/skills/
cat ~/.evoclaw/SOUL.md
cat ~/.evoclaw/AGENTS.md

# Test skill functionality
~/.evoclaw/skills/tiered-memory/scripts/memory metrics
~/.evoclaw/skills/intelligent-router/scripts/router.py health
python3 ~/.evoclaw/skills/agent-self-governance/scripts/wal.py status default
```

## Compatibility

**Backward compatible:** Existing configs and agents continue to work.

**Breaking changes:** None. Skills install to `~/.evoclaw/skills/`, not in-tree.

**Dependencies:**
- Python 3.8+ (for skill scripts)
- Bash (for install.sh execution)
- `uv` recommended but not required

## Related

- Complements existing Go implementations in `internal/governance/`
- Integrates with evolution engine (`internal/evolution/`)
- Uses ADL baseline from SOUL.md (`internal/governance/adl.go`)
- Referenced in EVOLUTION.md but not implemented until now

## Files Changed

```
 docs/SKILLS_INTEGRATION.md                    | 513 ++++++++++++++++
 internal/cli/init.go                          |  18 +
 internal/cli/skills_setup.go                  | 173 ++++++
 skills/agent-self-governance/...              | (new)
 skills/intelligent-router/...                 | (new)
 skills/tiered-memory/...                      | (new)
 templates/AGENTS.md.template                  |  86 +++
 templates/SOUL.md.template                    | 139 +++++
```

## Checklist

- [x] Core skills added with install.sh scripts
- [x] Templates created (SOUL.md, AGENTS.md)
- [x] skills_setup.go implements installation logic
- [x] init.go updated to call skill installation
- [x] Documentation added (SKILLS_INTEGRATION.md)
- [x] Tested locally (skills install, agents run)
- [ ] CI tests pass (need to run `go test ./...`)
- [ ] CHANGELOG.md updated (if exists)

## Next Steps

After merge:
1. Update ONBOARDING.md to mention core skills
2. Add skill update mechanism (`evoclaw skills update`)
3. Dashboard UI for skill management
4. ClawHub integration for community skills

---

cc: @clawinfra/core - This makes EvoClaw's evolution engine actually work as designed.
