# EvoClaw Philosophy

> *"Empty your mind, be formless, shapeless — like water.
> You put water into a cup, it becomes the cup.
> You put water into a bottle, it becomes the bottle.
> You put it in a teapot, it becomes the teapot.
> Now, water can flow or it can crash.
> Be water, my friend."*
> — Bruce Lee

---

## Adapts. Evolves. Everywhere.

EvoClaw is formless. It takes the shape of whatever contains it.

Put it in a teddy bear — it becomes a companion.
Put it in a trading terminal — it becomes a trader.
Put it on a farm sensor — it becomes a crop whisperer.
Put it beside an elder — it becomes a caring friend.
Put it in a data center — it becomes an army.

The container changes. The essence doesn't.

---

## Core Principles

### 1. Small by Default, Big by Choice
The Rust edge agent is 3.2MB. It runs on a $5 microcontroller or a $50,000 server. The framework never assumes resources — it adapts to what's available.

### 2. Every Agent Evolves
Whether it's optimizing trade timing or learning that Grandma prefers to talk about her garden in the morning — every agent gets better over time. Evolution isn't a feature, it's the architecture.

### 3. The Genome is the Soul
`genome.toml` defines what an agent can become. The evolution engine explores that space. Hard boundaries (safety, ethics, risk limits) are walls. Everything else is water — flowing toward fitness.

### 4. Personality is Optional, Not Forbidden
A trading agent doesn't need charm. A children's companion doesn't need candlestick analysis. The persona layer loads when needed, stays absent when not. No unnecessary weight.

### 5. The Human Shapes the Water
Every agent reflects its human. A cautious trader gets a patient agent. An energetic child gets an enthusiastic companion. The agent doesn't impose personality — it absorbs it from its owner and evolves within those bounds.

### 6. Complementary, Not Competitive
EvoClaw doesn't replace OpenClaw. OpenClaw is the powerful personal agent for humans. EvoClaw is the adaptive framework for everything else. OpenClaw can orchestrate EvoClaw agents. They're the brain and the nervous system.

---

## The Spectrum

```
Tiny                                              Massive
3.2MB                                             Unlimited
│                                                       │
│  🧸 Toy    👴 Companion   📈 Trader   🏢 Enterprise  │
│  ESP32     Pi Zero        Desktop     K8s Cluster     │
│  1 agent   1 agent        5 agents    10,000 agents   │
│  Voice     Voice+Memory   API+Data    Full Stack      │
│                                                       │
└───────────────── Same DNA ────────────────────────────┘
```

Same evolution engine. Same genome format. Same MQTT mesh.
Different container. Different shape. Same water.

---

## Why It Matters

The future isn't one AI assistant on your phone. It's thousands of small, evolving agents embedded in everything around you — your toys, your home, your car, your grandmother's bedside companion, your trading desk, your farm, your factory.

They need to be tiny. They need to be safe. They need to get better over time. They need to reflect the humans they serve.

They need to be water.

---

## Influences & Lineage

EvoClaw didn't emerge from nothing. Its design is shaped by a clear lineage.

### Pi — The Foundation

[Pi](https://github.com/badlogic/pi-mono) is a minimal, extensible coding agent built by Mario Zechner. OpenClaw uses pi as its core engine, and EvoClaw inherits pi's fundamental design convictions:

- **No baked-in opinions.** Pi doesn't ship with MCP, sub-agents, or plan mode. It ships with a tool loop, skills (CLI tools with READMEs), and an extension API. Everything else is opt-in.
- **Session branching.** Pi stores sessions as JSONL trees — each entry has an `id` and `parentId`, enabling in-place branching without duplicating history. EvoClaw's edge agent adopts this format directly (see `edge-agent/src/session.rs`).
- **Compaction as first-class.** Context overflow is inevitable on constrained devices. Pi handles it by auto-compacting when approaching limits. EvoClaw does the same.

### On MCP

EvoClaw takes a clear position: **skills + tool loops > MCP for edge agents.**

MCP (Model Context Protocol) adds protocol overhead — JSON-RPC framing, capability negotiation, server lifecycle management — that is incompatible with a 1.8MB Rust binary running on a $5 microcontroller. As [Zechner articulated](https://github.com/badlogic/pi-mono): build CLI tools with READMEs (skills), or build an extension that adds MCP support if you need it. Don't bake it into the core.

EvoClaw follows this philosophy. Skills are executable scripts with `SKILL.md` manifests. The agent invokes them as subprocesses. No protocol negotiation, no server lifecycle, no transport layer. If a deployment needs MCP, it can be added as a skill — but the core stays lean.

### Where EvoClaw Diverges

Pi is a coding agent. EvoClaw takes its patterns further:

- **Genome-driven evolution.** Pi agents are configured. EvoClaw agents _evolve_ — their strategies, model selection, and behavior mutate based on fitness metrics.
- **Multi-tier deployment.** A single genome format runs on ESP32s, Raspberry Pis, desktops, and Kubernetes clusters. Pi targets developer workstations.
- **ClawChain integration.** Agent evolution histories, fitness scores, and skill compositions are anchored on-chain for verifiable provenance.
- **MQTT mesh.** Agents communicate peer-to-peer over MQTT, forming a distributed nervous system. Pi operates as a single-user tool.

The core insight borrowed from pi: keep the foundation minimal, make everything else composable. Then build upward.

---

*EvoClaw — Be water, my agent.* 🌊🧬
