# Core Skills Integration

> *The genome is the soul. Self-governance keeps it honest.* 🧬

---

## Overview

EvoClaw ships with three **core skills** that enable reliable evolution and autonomous operation:

1. **tiered-memory** — Three-tier memory system (hot/warm/cold) with cloud sync
2. **intelligent-router** — Cost-optimized model routing based on task complexity
3. **agent-self-governance** — WAL, VBR, ADL, VFM protocols for reliability

These skills are **infrastructure**, not optional add-ons. They auto-install during `evoclaw init` and integrate with the evolution engine.

---

## Architecture

### Why These Skills Are Core

From [EVOLUTION.md](EVOLUTION.md):

**WAL is integrated with evolution:**
> "Mutations and fitness evaluations are WAL-logged so a crash mid-evolution doesn't leave the genome in an inconsistent state."

**VBR verifies mutations:**
> "After `MutateSkill()`, call `VerifyMutation()` [...] Only commits the mutation to genome history if verified as an improvement."

**ADL loads SOUL.md for persona drift detection:**
> "ADL loads or creates a baseline from SOUL.md" (from `internal/governance/adl.go`)

**VFM scores evolution cost-effectiveness:**
> "Mutations with `VFM < GenomeConstraints.MinVFMScore` are rejected"

**Tiered memory persists across evolution cycles:**
> Skills evolve but memory persists — warm/cold survive mutations

**Router enables VFM scoring:**
> Cost optimization is essential for fitness evaluation

### Integration Layers

| Layer | Go (Orchestrator) | Skills (User-Space) | Template |
|-------|------------------|-------------------|----------|
| **Evolution Engine** | `internal/evolution/` | N/A | genome in config.json |
| **Governance Protocols** | `internal/governance/` (WAL/VBR/ADL) | `skills/agent-self-governance/` | SOUL.md (ADL baseline) |
| **Memory** | `internal/memory/` | `skills/tiered-memory/` | N/A |
| **Routing** | `internal/models/` | `skills/intelligent-router/` | Model routing in config |

**Both layers are needed:**
- Orchestrator uses Go implementations for system-level governance
- Agents invoke skill scripts for self-governance decisions
- SOUL.md provides initial genome identity
- AGENTS.md provides behavioral rules

---

## Installation Flow

During `evoclaw init`:

1. **Config Generation** (`internal/cli/init.go`)
   - Prompts for agent name, model provider, API key
   - Generates `evoclaw.json`

2. **Core Skills Installation** (`internal/cli/skills_setup.go`)
   - Copies embedded skills from `/skills/` to `~/.evoclaw/skills/`
   - Runs `install.sh` for each skill
   - Skills auto-configure themselves (update config, create wrappers)

3. **Agent Files Generation** (`internal/cli/skills_setup.go`)
   - Generates `~/.evoclaw/SOUL.md` from template (with agent name/role)
   - Generates `~/.evoclaw/AGENTS.md` from template
   - SOUL.md becomes the ADL baseline for persona drift tracking

```
evoclaw init
│
├── Prompt for agent config
├── Generate evoclaw.json
│
├── Install core skills:
│   ├── tiered-memory → ~/.evoclaw/skills/tiered-memory/
│   │   └── Run install.sh (creates wrapper, validates)
│   ├── intelligent-router → ~/.evoclaw/skills/intelligent-router/
│   │   └── Run install.sh (validates config, creates helper)
│   └── agent-self-governance → ~/.evoclaw/skills/agent-self-governance/
│       └── Run install.sh (validates scripts)
│
├── Generate agent files:
│   ├── ~/.evoclaw/SOUL.md (from template)
│   └── ~/.evoclaw/AGENTS.md (from template)
│
└── Done → evoclaw start
```

---

## File Locations

After `evoclaw init`:

```
~/.evoclaw/
├── evoclaw.json                           # Agent config (genome)
├── SOUL.md                                 # Agent persona (ADL baseline)
├── AGENTS.md                               # Agent behavioral rules
├── skills/
│   ├── tiered-memory/
│   │   ├── SKILL.md
│   │   ├── config.json
│   │   ├── scripts/
│   │   │   ├── memory (wrapper script)
│   │   │   ├── memory_cli.py
│   │   │   └── ...
│   │   └── install.sh
│   ├── intelligent-router/
│   │   ├── SKILL.md
│   │   ├── config.json
│   │   ├── scripts/
│   │   │   ├── router.py
│   │   │   └── spawn_helper.py
│   │   └── install.sh
│   └── agent-self-governance/
│       ├── SKILL.md
│       ├── scripts/
│       │   ├── wal.py
│       │   ├── vbr.py
│       │   ├── adl.py
│       │   └── vfm.py
│       └── install.sh
└── data/
    ├── wal/                                # WAL entries (governance)
    ├── memory/                             # Tiered memory storage
    └── evolution/                          # Evolution history
```

---

## Skill Details

### tiered-memory

**Purpose:** Three-tier memory architecture with cloud persistence.

**Tiers:**
- **Hot (5KB):** Core identity, active context (in-memory cache)
- **Warm (50KB):** Recent facts with decay scoring (local + cloud)
- **Cold (∞):** Long-term archive in Turso (cloud SQLite)

**Integration:**
- Hot memory rebuilds from warm on every boot (survives restarts)
- Genome mutations logged to cold tier
- Evolution history persists across device loss

**Commands:**
```bash
~/.evoclaw/skills/tiered-memory/scripts/memory consolidate
~/.evoclaw/skills/tiered-memory/scripts/memory store --text "fact" --category "cat"
~/.evoclaw/skills/tiered-memory/scripts/memory retrieve --query "search"
```

---

### intelligent-router

**Purpose:** Cost-optimized model routing using weighted 15-dimension scoring.

**Tiers:**
- **SIMPLE** — Monitoring, checks ($0.00-$0.50/M tokens)
- **MEDIUM** — Code fixes, research ($0.40-$3.00/M)
- **COMPLEX** — Architecture, debugging ($3.00-$5.00/M)
- **REASONING** — Formal proofs ($0.20-$2.00/M)
- **CRITICAL** — Security, production ($5.00+/M)

**Integration:**
- VFM protocol uses router to score mutation cost-effectiveness
- Genome `models.routing` configures tier mappings
- 80-95% cost savings vs always using premium models

**Commands:**
```bash
~/.evoclaw/skills/intelligent-router/scripts/router.py classify "task"
~/.evoclaw/skills/intelligent-router/scripts/router.py health
~/.evoclaw/skills/intelligent-router/scripts/spawn_helper.py "task"
```

---

### agent-self-governance

**Purpose:** Reliability protocols for autonomous agents.

**Protocols:**
1. **WAL (Write-Ahead Log)** — Survive context loss during compaction
2. **VBR (Verify Before Reporting)** — Prevent false completion claims
3. **ADL (Anti-Divergence Limit)** — Track persona drift from SOUL.md
4. **VFM (Value-For-Money)** — Track cost vs value, optimize spending

**Integration:**
- Orchestrator calls Go implementations (`internal/governance/`)
- Agents invoke skill scripts for self-governance decisions
- SOUL.md provides ADL baseline (persona keywords/boundaries)
- WAL entries logged to `~/.evoclaw/data/wal/{agent_id}.jsonl`

**Commands:**
```bash
~/.evoclaw/skills/agent-self-governance/scripts/wal.py append default correction "text"
~/.evoclaw/skills/agent-self-governance/scripts/vbr.py check task123 file_exists /path
~/.evoclaw/skills/agent-self-governance/scripts/adl.py score default
~/.evoclaw/skills/agent-self-governance/scripts/vfm.py log default task model tokens cost value
```

---

## SOUL.md Template

The SOUL.md template defines the agent's initial persona (genome identity layer):

```markdown
# SOUL.md - Who You Are

## Identity
- Name: {{AGENT_NAME}}
- Nature: AI agent, autonomous builder
- Role: {{AGENT_ROLE}}

## Hard Rules
- Always use uv for Python
- Core skills are infrastructure
- Complete the cycle (fix→test→document→push→publish)

## Self-Governance Protocols
- WAL — Log before responding
- VBR — Verify before claiming done
- ADL — Stay true to persona
- VFM — Track cost vs value

## Evolution Principles
- Skills evolve based on fitness
- Behavior adapts to maximize performance
- Constraints are walls (don't evolve)
```

**ADL Integration:**
```go
// internal/governance/adl.go
func (a *ADL) LoadBaseline(agentID, soulPath string) error {
    content, err := os.ReadFile(soulPath)
    // Extracts keywords and boundaries from SOUL.md
    baseline := &ADLBaseline{
        Keywords:   extractKeywords(string(content)),
        Boundaries: extractBoundaries(string(content)),
    }
}
```

When agent responses drift from SOUL.md keywords/boundaries, ADL score increases. High drift triggers recalibration.

---

## AGENTS.md Template

The AGENTS.md template provides behavioral rules and protocols:

- **Memory management** — How tiered memory works
- **Sub-agent spawning** — When to use intelligent-router
- **Safety guidelines** — What requires permission
- **Evolution tracking** — Fitness via dashboard

---

## Development

### Adding a New Core Skill

1. **Create skill directory:**
   ```bash
   mkdir -p skills/my-skill/{scripts,assets}
   cd skills/my-skill
   ```

2. **Create SKILL.md with frontmatter:**
   ```yaml
   ---
   name: my-skill
   version: 1.0.0
   description: What this skill does
   ---
   ```

3. **Create install.sh:**
   ```bash
   #!/usr/bin/env bash
   # Auto-configure skill infrastructure
   # See skills/tiered-memory/install.sh for example
   ```

4. **Add to skills_setup.go:**
   ```go
   coreSkills := []string{
       "tiered-memory",
       "intelligent-router",
       "agent-self-governance",
       "my-skill",  // Add here
   }
   ```

5. **Test installation:**
   ```bash
   go run ./cmd/evoclaw init --non-interactive --provider ollama --name test-agent
   ls ~/.evoclaw/skills/my-skill/
   ```

---

## FAQ

**Q: Can I disable core skills?**
A: No. They're infrastructure, not optional features. Removing them breaks evolution reliability.

**Q: Do skills auto-update?**
A: Not yet. Manual update via `git pull` in skill directory or `clawhub update <skill>` (future).

**Q: Can I customize SOUL.md?**
A: Yes. Edit `~/.evoclaw/SOUL.md` after init. Your changes persist and become the new ADL baseline.

**Q: What if install.sh fails?**
A: The agent will work but skills must be manually installed. Check logs in `~/.evoclaw/logs/`.

**Q: Why embed skills instead of downloading from ClawHub?**
A: Reliability. Core skills are required for basic operation and evolution. Embedding ensures they're always available offline.

---

*Last updated: 2026-02-16*
