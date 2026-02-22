# Migration Guide: OpenClaw → EvoClaw

> Migrate your existing OpenClaw setup to EvoClaw with one command.

---

## Prerequisites

- EvoClaw installed (`evoclaw version` works)
- Access to your OpenClaw home directory (default `~/.openclaw`)
- Backup of your OpenClaw data (recommended)

---

## Quick Start

```bash
# Preview what will be migrated (no changes made)
evoclaw migrate openclaw --dry-run

# Run the migration
evoclaw migrate openclaw

# Custom paths
evoclaw migrate openclaw --source /path/to/openclaw --target /path/to/evoclaw
```

---

## What's Migrated

| Category | Source (OpenClaw) | Target (EvoClaw) | Status |
|----------|-------------------|-------------------|--------|
| **Memory** | `MEMORY.md`, `memory/*.md` | `memory/` directory | ✅ Full copy |
| **Identity** | `SOUL.md`, `IDENTITY.md`, `AGENTS.md` | `agent.toml` identity section | ✅ Parsed & mapped |
| **Skills** | `skills/` directories | `plugins.json` manifest | ✅ Listed & mapped |
| **Config** | `config.json` / `gateway.json` | `config.toml` | ✅ Providers, channels, heartbeat |
| **Cron Jobs** | `crons.json` / `scheduler.json` | `scheduler.json` | ✅ Schedule & action mapping |

---

## What Needs Manual Setup

These items cannot be automatically migrated:

- **API keys** — Review `config.toml` and update any provider API keys
- **Webhook URLs** — Re-register webhooks with new EvoClaw endpoints
- **Custom skill code** — Skills are listed but may need adaptation for EvoClaw plugin API
- **Advanced scheduler patterns** — Complex cron expressions may need review
- **Container/Quadlet configs** — Not migrated, use `evoclaw gateway install`
- **Blockchain/chain configs** — Review and update chain settings manually

---

## Step-by-Step Guide

### 1. Backup OpenClaw

```bash
cp -r ~/.openclaw ~/.openclaw.backup
```

### 2. Dry Run

```bash
evoclaw migrate openclaw --dry-run
```

Review the output. It shows exactly what will be copied, mapped, and created.

### 3. Run Migration

```bash
evoclaw migrate openclaw
```

### 4. Review Generated Files

```bash
# Check identity
cat ~/.evoclaw/agent.toml

# Check config
cat ~/.evoclaw/config.toml

# Check memory
ls ~/.evoclaw/memory/

# Check plugins
cat ~/.evoclaw/plugins.json

# Check scheduler
cat ~/.evoclaw/scheduler.json
```

### 5. Update API Keys

Edit `~/.evoclaw/config.toml` and fill in your API keys for each provider.

### 6. Start EvoClaw

```bash
evoclaw start --config ~/.evoclaw/config.toml
```

---

## Rollback

If something goes wrong:

```bash
# Remove EvoClaw data
rm -rf ~/.evoclaw

# Restore OpenClaw backup
cp -r ~/.openclaw.backup ~/.openclaw
```

Your original OpenClaw installation is never modified during migration.

---

## CLI Reference

```
evoclaw migrate openclaw [flags]

Flags:
  --source DIR    OpenClaw home directory (default: ~/.openclaw)
  --target DIR    EvoClaw home directory (default: ~/.evoclaw)
  --dry-run       Show what would be migrated without writing files
```

---

**Status:** Implemented ✅  
**Added:** 2026-02-22
