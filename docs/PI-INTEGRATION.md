# Pi Integration

> How EvoClaw relates to pi and OpenClaw — and what patterns it inherits.

---

## The Relationship

[Pi](https://github.com/badlogic/pi-mono) is a minimal, extensible coding agent created by Mario Zechner. It provides the core engine for [OpenClaw](https://github.com/openclaw/openclaw) — as pi's README states: *"See openclaw/openclaw for a real-world SDK integration."*

**EvoClaw** is the evolution-powered agent framework within the ClawChain ecosystem. It sits alongside OpenClaw:

```
pi (core engine)
 └─► OpenClaw (personal AI assistant — uses pi as SDK)
 └─► EvoClaw (edge agent framework — adopts pi's design patterns)
```

OpenClaw is the human-facing agent. EvoClaw is the framework for deploying thousands of small, evolving agents on edge devices. They share a common ancestor in pi's design philosophy.

---

## Adopted Patterns

### 1. JSONL Tree Session Format

Pi stores conversation sessions as append-only JSONL files where each entry carries an `id` and `parentId`. This creates an implicit tree — multiple conversation branches coexist in a single file without duplicating shared history.

EvoClaw adopts this format in its Rust edge agent (`edge-agent/src/session.rs`).

**Why it matters for edge:**
- Append-only writes are safe on unreliable storage (SD cards, flash)
- No database dependency — just a file
- Branching enables A/B testing of agent responses without losing history
- Compaction can prune branches while preserving the active one

#### Format Spec

Each line is a self-contained JSON object:

```jsonl
{"id":"uuid1","parent_id":null,"role":"user","content":"What's the temperature?","ts":1700000000}
{"id":"uuid2","parent_id":"uuid1","role":"assistant","content":"Currently 22°C.","ts":1700000001}
{"id":"uuid3","parent_id":"uuid2","role":"user","content":"And humidity?","ts":1700000010}
{"id":"uuid4","parent_id":"uuid2","role":"user","content":"Convert to Fahrenheit","ts":1700000015}
```

In this example, `uuid3` and `uuid4` are two branches from the same parent (`uuid2`). To reconstruct a branch, walk the `parent_id` chain from any leaf back to `null`.

#### Fields

| Field       | Type             | Description                                           |
|-------------|------------------|-------------------------------------------------------|
| `id`        | `string`         | Unique entry identifier (UUID v4)                     |
| `parent_id` | `string \| null` | Parent entry ID, or `null` for conversation roots     |
| `role`      | `string`         | One of `"user"`, `"assistant"`, `"tool"`              |
| `content`   | `string`         | Message content                                       |
| `ts`        | `integer`        | Unix timestamp (seconds)                              |
| `metadata`  | `object \| null` | Optional — tool call info, model used, token counts   |

### 2. Skills Over Protocols

Pi's core philosophy: *"Build CLI tools with READMEs (skills), or build an extension that adds MCP support."* Skills are executable scripts with documentation. The agent reads the README to understand capabilities, then invokes the script.

EvoClaw uses the same pattern:
- **Pi:** Skills are directories with `README.md` files. Pi reads them to understand available tools.
- **EvoClaw:** Skills are directories with `SKILL.md` manifests and `agent.toml` tool definitions. The agent loads them at startup.

The mapping is direct:

| Pi Concept       | EvoClaw Equivalent        |
|------------------|---------------------------|
| `README.md`      | `SKILL.md` + `README.md`  |
| `package.json` → `pi.skills` | `agent.toml` tool defs |
| `pi install`     | `evoclaw skill add`       |
| npm registry     | ClawHub                   |

### 3. Extension / Plugin API

Pi exposes an event-driven extension API: `registerTool()`, `on("tool_call", handler)`. EvoClaw's Go orchestrator provides an equivalent pattern for plugins (see [PLUGIN-API.md](PLUGIN-API.md)).

### 4. Compaction

Pi auto-compacts sessions when context approaches the limit. For edge agents running on devices with 512MB RAM, this is essential — not optional. EvoClaw treats compaction as a first-class operation, triggered by the evolution engine or context pressure.

### 5. No Baked-In Opinions

Pi doesn't ship MCP, sub-agents, or plan mode in its core. EvoClaw follows the same principle: the core is a tool loop with skills. Everything else — evolution, trading, monitoring — is composed from skills and extensions.

---

## ClawHub ↔ Pi Packages Interoperability

### Current State

Skills can be dual-published with both an EvoClaw `SKILL.md` manifest and a pi-compatible `package.json` with the `pi` key. See [SKILLS-SYSTEM.md](SKILLS-SYSTEM.md#pi-package-compatibility) for the dual manifest format.

### Roadmap

1. **Phase 1 (Current):** Manual dual-manifest authoring. Skills work in both systems but require both entry points.
2. **Phase 2:** `evoclaw skill init --pi-compat` generates both manifests from a single source. ClawHub accepts pi-format packages directly.
3. **Phase 3:** Unified skill registry. ClawHub and npm/pi packages are discoverable from either tool. `evoclaw skill add` can install pi packages; `pi install` can pull from ClawHub.
4. **Phase 4:** Runtime compatibility layer. EvoClaw agents can load pi extensions natively (requires bridging TypeScript extensions to subprocess calls).

---

## What EvoClaw Adds Beyond Pi

Pi is a coding agent for developers. EvoClaw takes the same foundation and extends it for a different domain:

| Capability             | Pi                        | EvoClaw                          |
|------------------------|---------------------------|----------------------------------|
| Target                 | Developer workstations    | Edge devices (IoT, phones, SBCs) |
| Binary size            | Node.js runtime           | 1.8MB Rust binary                |
| Session format         | JSONL tree                | JSONL tree (same)                |
| Evolution              | —                         | Genome-driven mutation           |
| Multi-agent            | Single agent              | MQTT mesh, orchestrator          |
| Deployment             | Local                     | systemd/launchd/K8s/containers   |
| On-chain anchoring     | —                         | ClawChain integration            |
| Skill system           | npm + README              | ClawHub + SKILL.md               |

The lineage is clear: pi's patterns form the foundation. EvoClaw builds a different house on top.

---

*Standing on the shoulders of minimal agents.* 🧬
