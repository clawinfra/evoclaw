# EvoClaw Skills System

> *Modular capabilities. Infinite potential.* 🧩

---

## Overview

The Skills System allows EvoClaw agents to dynamically extend their capabilities without recompiling the core binary. Skills are modular packages containing tools, logic, and assets that can be installed, updated, and managed at runtime.

**Key Principles:**
1.  **Sandboxed Execution**: Skills run as subprocesses (scripts/binaries) invoked by the agent.
2.  **Standard Interface**: All skills expose a `SKILL.md` manifest and executable entry points.
3.  **Cloud-First Distribution**: Skills are fetched from ClawHub (or Git) and updated OTA.
4.  **Language Agnostic**: A skill can be written in Bash, Python, Node.js, Rust, or Go.

---

## Skill Structure

A standard skill package looks like this:

```
my-skill/
├── SKILL.md           # Manifest & Documentation (Required)
├── agent.toml         # Tool definitions (Auto-injected into agent config)
├── scripts/           # Executable logic
│   ├── run.sh
│   └── helper.py
├── assets/            # Static files (images, templates)
└── README.md          # User-facing guide
```

### SKILL.md Format

The manifest defines metadata and installation instructions:

```yaml
---
name: market-monitor
version: 1.0.0
description: Real-time market monitoring via Twitter & Reddit
author: clawinfra
license: MIT
metadata:
  evoclaw:
    permissions: ["internet", "filesystem"]
    env: ["TWITTER_TOKEN", "REDDIT_ID"]
---

# Market Monitor

Usage instructions go here...
```

---

## Tool Definition (`agent.toml` fragment)

Skills provide tool definitions that the EvoClaw agent loads at startup.

```toml
[tools.market_check]
command = "~/.evoclaw/skills/market-monitor/scripts/check.sh"
description = "Check current market sentiment and news"
args = ["$QUERY", "$LIMIT"]
env = ["API_KEY=${MARKET_API_KEY}"]
timeout_secs = 60
```

---

## Core Skills

### 1. Evo-Lens (`evo-lens`)
**Visual Identity Generator**
- **Function**: Generates selfies/photos of the agent's persona.
- **Backend**: Uses Flux/SDXL via ComfyUI (self-hosted) or Fal.ai.
- **Config**: `gender`, `style` (cyberpunk/casual), `reference_image`.
- **Usage**: "Send me a selfie", "Show me what you're seeing".

### 2. Market Monitor (`market-monitor`)
**Real-Time Intelligence**
- **Function**: Aggregates signals from Twitter (KOLs), Reddit, and News.
- **Backend**: `bird` CLI, `reddit-cli`, Brave Search.
- **Usage**: Background cron jobs for alerts + on-demand queries.

### 3. Intelligent Router (`intelligent-router`)
**Cost/Performance Optimization**
- **Function**: Routes tasks to the optimal LLM based on complexity.
- **Tiers**:
  - **Simple**: GLM-4.5-Air / Llama-3-8B (Monitoring, Summaries)
  - **Standard**: GPT-4o-mini / Claude-Haiku (General chat)
  - **Complex**: Claude-3.5-Sonnet / GPT-4o (Coding, Analysis)
  - **Critical**: Claude-3-Opus / o1 (Architecture, Security)

---

## Skill Lifecycle

1.  **Discovery**: Agent queries ClawHub for available skills.
2.  **Installation**:
    - Clones skill repo to `~/.evoclaw/skills/<name>/`.
    - Installs dependencies (if defined in `SKILL.md`).
    - Merges tool defs into `agent.toml`.
3.  **Execution**: Agent invokes tool via standard subprocess call.
4.  **Update**: `git pull` or `clawhub update` to refresh logic.

---

## Security

- **Env Injection**: Secrets (API keys) are injected at runtime from the agent's secure vault (`memory/encrypted/`).
- **Path Isolation**: Skills should only write to their own directory or explicitly allowed paths.
- **Approval Mode**: Critical tools (spending money, deleting files) require user confirmation (Human-in-the-Loop).

---

*Extendable. Composable. Powerful.* 🧩
