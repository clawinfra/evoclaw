# EvoClaw Component Relationships

## Overview

EvoClaw uses a hub-and-spoke architecture with MQTT as the decoupling layer. Every component communicates through well-defined relationships.

## Relationship Map

| From | To | Cardinality | Description |
|------|----|-------------|-------------|
| Orchestrator | Broker | **1:1** | One orchestrator connects to one MQTT broker |
| Broker | Agents | **1:N** | One broker serves many agents |
| Orchestrator | Agents | **1:N** | One orchestrator manages many agents |
| Orchestrator | Models | **1:N** | One orchestrator routes to many LLM providers |
| Agent | Skills | **1:N** | One agent loads many skills |
| Human | Orchestrator | **N:1** | Many humans talk to one orchestrator |
| Agent | Device | **N:1** | Many agents can run on one device |

## Architecture Diagram

```
👤👤👤 Humans
│  Telegram Bot, Web Dashboard, REST API
│  (N:1 — many humans, one orchestrator)
│
▼
┌─────────────────────────────────────────┐
│           🖥️ ORCHESTRATOR               │
│                                         │
│  ┌──────────┐  ┌──────────┐            │
│  │ ChatSync │  │  Router  │            │
│  └────┬─────┘  └────┬─────┘            │
│       │              │                  │
│       ▼              ▼                  │
│  ┌──────────────────────────┐           │
│  │  🤖 Model Providers (1:N) │           │
│  │  ├── Ollama (local)      │           │
│  │  ├── OpenAI (cloud)      │           │
│  │  └── Anthropic (cloud)   │           │
│  └──────────────────────────┘           │
│                                         │
│  ┌──────────────────────────┐           │
│  │  Agent Registry          │           │
│  │  Evolution Engine        │           │
│  │  Memory Store            │           │
│  └──────────────────────────┘           │
└────────────────┬────────────────────────┘
                 │ (1:1)
                 ▼
┌─────────────────────────────────────────┐
│           📡 MQTT BROKER                │
│           (Mosquitto)                   │
│                                         │
│  Topics:                                │
│  ├── evoclaw/agents/+/commands  (down)  │
│  ├── evoclaw/agents/+/reports   (up)    │
│  └── evoclaw/agents/+/status    (up)    │
└──┬──────────┬──────────┬────────────────┘
   │ (1:N)    │          │
   ▼          ▼          ▼
┌────────┐ ┌────────┐ ┌────────┐
│🍓Agent₁│ │🍓Agent₂│ │🍓Agent₃│
│        │ │        │ │        │
│Skills: │ │Skills: │ │Skills: │
│├ sysmon│ │├ gpio  │ │├ camera│
│├ price │ │├ buzzer│ │└ motion│
│└ gpio  │ │└ temp  │ │        │
│        │ │        │ │        │
│ Pi 1   │ │ Pi 2   │ │ Pi 3   │
└────────┘ └────────┘ └────────┘
```

## Communication Flows

### Human → Agent (query)
```
Human ──HTTP/WS──→ Orchestrator ──MQTT──→ Broker ──MQTT──→ Agent
Human ←─HTTP/WS──← Orchestrator ←─MQTT──← Broker ←─MQTT──← Agent
```

### Human → LLM (chat)
```
Human ──HTTP──→ Orchestrator ──HTTP──→ Ollama
Human ←─HTTP──← Orchestrator ←─HTTP──← Ollama
```

### Agent → Orchestrator (report)
```
Agent ──MQTT──→ Broker ──MQTT──→ Orchestrator ──→ Registry/Dashboard
```

### Agent Skill Tick (periodic)
```
Agent Skill ──tick()──→ SkillReport ──MQTT──→ Broker ──→ Orchestrator
```

## Data Flow Example: "What's the Pi temperature?"

```
1. 👤 Human types in Dashboard Chat
2. POST /api/chat → Orchestrator.ChatSync()
3. Orchestrator → Ollama: "User asks about Pi temperature"
4. Ollama → "I'll check the system monitor skill"
5. Orchestrator → MQTT publish → evoclaw/agents/pi1/commands
6. Broker → delivers to Agent subscriber
7. Agent → SkillRegistry → SystemMonitorSkill.handle("status")
8. SystemMonitorSkill reads /sys/class/thermal → 48.3°C
9. Agent → MQTT publish → evoclaw/agents/pi1/reports
10. Broker → delivers to Orchestrator subscriber
11. Orchestrator → Ollama: "Temperature is 48.3°C, format response"
12. Ollama → "The Pi CPU is at 48.3°C, within normal range"
13. HTTP response → Dashboard → Human sees answer
```

## Scaling Patterns

### Current: Single Hub
```
1 Orchestrator → 1 Broker → N Agents
```
Good for: Home lab, small deployments, development.

### Future: High Availability
```
N Orchestrators → 1 Broker Cluster → N Agents
```
Multiple orchestrators share a broker cluster for failover.

### Future: Federated
```
Site A: Orchestrator₁ → Broker₁ → Agents
                            ↕ (bridge)
Site B: Orchestrator₂ → Broker₂ → Agents
```
MQTT broker bridging connects separate sites.

### Future: Agent-to-Agent (P2P)
```
Agent₁ ──MQTT──→ Broker ──MQTT──→ Agent₂
```
Agents communicate directly via broker without orchestrator involvement.
Useful for: sensor → actuator chains, collaborative tasks.

## Key Design Principles

1. **Broker is the nervous system** — all inter-component communication flows through MQTT
2. **Agents are independent** — they don't know about the orchestrator, only the broker
3. **Skills are modular** — agents load capabilities at startup via config
4. **Models are pluggable** — orchestrator routes to any LLM provider
5. **Humans are N:1** — multiple interfaces (Telegram, Web, API) to one orchestrator
