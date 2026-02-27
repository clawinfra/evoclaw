# Create Your First Agent

This guide walks you through creating, configuring, and running your first EvoClaw agent — from hub setup to a remote edge agent connected via the dashboard.

## The Fastest Way: `join`

If you already have an EvoClaw orchestrator running (see [Quickstart](quickstart.md)), adding an agent is one command:

```bash
evoclaw-agent join YOUR_HUB_IP
```

That's it. The agent auto-configures, registers with the hub, and starts. The rest of this guide explains the details.

---

## Understanding Agent Types

EvoClaw supports several agent types:

| Type | Purpose | Example Use Case |
|------|---------|-----------------|
| `orchestrator` | General-purpose LLM agent | Chat assistant, task runner |
| `trader` | Trading agent | Hyperliquid perpetual futures |
| `monitor` | Monitoring agent | Price alerts, system monitoring |
| `sensor` | Data collection agent | IoT sensors, environmental data |
| `governance` | Governance/oversight agent | Risk management, compliance |

## Option 1: Join a Hub (Recommended)

The `join` command is the easiest way to deploy an edge agent on any device.

### Step 1: Set up the hub

On your main machine (PC, server, VPS):

```bash
# Initialize the hub
evoclaw setup hub

# Start the orchestrator
./evoclaw --config evoclaw.json
```

### Step 2: Join from the edge device

On the remote device (Raspberry Pi, laptop, another server):

```bash
# Build or install the agent binary
cd evoclaw/edge-agent
cargo build --release
sudo cp target/release/evoclaw-agent /usr/local/bin/

# Join the hub
evoclaw-agent join 192.168.99.44
```

Output:

```
🧬 EvoClaw Agent Setup
  Hub: 192.168.99.44:8420 ✓ (v0.1.0, 1 agent online)
  MQTT: 192.168.99.44:1883 ✓
  Agent ID: raspberrypi-a3f2
  Type: monitor
  Config: /home/pi/.evoclaw/agent.toml ✓

🚀 Agent started! Connected to hub.
  Dashboard: http://192.168.99.44:8420
```

### Step 3: Verify in the dashboard

Open `http://192.168.99.44:8420` in your browser. Your new agent appears in the agent list with:

- Status: online
- Heartbeat: every 30 seconds
- Type: monitor (or whatever you specified)

You can also verify via API:

```bash
curl http://192.168.99.44:8420/api/agents | jq
```

### Customizing the join

```bash
# Specify agent type and ID
evoclaw-agent join 192.168.99.44 --id my-trader --type trader

# Use custom ports
evoclaw-agent join 192.168.99.44 --port 9000 --mqtt-port 2883

# Generate config only — edit before starting
evoclaw-agent join 192.168.99.44 --no-start
vim ~/.evoclaw/agent.toml
evoclaw-agent --config ~/.evoclaw/agent.toml
```

---

## Option 2: Define Agents in Config

For agents running on the same machine as the orchestrator, define them in `evoclaw.json`:

```json
{
  "agents": [
    {
      "id": "my-agent",
      "name": "My First Agent",
      "type": "orchestrator",
      "model": "anthropic/claude-sonnet-4-20250514",
      "systemPrompt": "You are a helpful coding assistant. You write clean, well-documented code and explain your reasoning.",
      "skills": ["chat", "code"],
      "config": {
        "max_tokens": "4096"
      },
      "container": {
        "enabled": false,
        "memoryMb": 512,
        "cpuShares": 256,
        "allowNet": true
      }
    }
  ]
}
```

### Key Fields

- **`id`** — Unique identifier. Used in API paths: `/api/agents/my-agent`
- **`name`** — Display name shown in the dashboard
- **`type`** — Determines agent behavior and available features
- **`model`** — Default LLM model (format: `provider/model-id`)
- **`systemPrompt`** — The system prompt that defines the agent's personality and behavior
- **`skills`** — Capabilities enabled for this agent

## Multiple Agents

You can define multiple agents with different roles:

```json
{
  "agents": [
    {
      "id": "assistant",
      "name": "General Assistant",
      "type": "orchestrator",
      "model": "anthropic/claude-sonnet-4-20250514",
      "systemPrompt": "You are a helpful general-purpose assistant.",
      "skills": ["chat", "search"]
    },
    {
      "id": "code-review",
      "name": "Code Reviewer",
      "type": "orchestrator",
      "model": "anthropic/claude-sonnet-4-20250514",
      "systemPrompt": "You are an expert code reviewer. Focus on bugs, security issues, and performance.",
      "skills": ["code"]
    },
    {
      "id": "eth-trader",
      "name": "ETH Trader",
      "type": "trader",
      "model": "anthropic/claude-sonnet-4-20250514",
      "systemPrompt": "You are a cautious crypto trader focused on ETH perpetual futures.",
      "skills": ["trading"],
      "config": {
        "exchange": "hyperliquid",
        "max_position_usd": "5000"
      }
    }
  ]
}
```

## Interact via API

Once EvoClaw is running, interact with your agent:

```bash
# Get agent status
curl http://localhost:8420/api/agents/my-agent | jq

# Get agent metrics
curl http://localhost:8420/api/agents/my-agent/metrics | jq

# View conversation memory
curl http://localhost:8420/api/agents/my-agent/memory | jq

# Trigger evolution
curl -X POST http://localhost:8420/api/agents/my-agent/evolve | jq
```

## Interact via Telegram

If you've configured a Telegram bot (see [Configuration](configuration.md)):

1. Open your bot in Telegram
2. Send a message — the orchestrator routes it to an appropriate agent
3. The agent responds through Telegram

## Monitor in Dashboard

Open [http://localhost:8420](http://localhost:8420) to see your agent in action:

- **Agents view** — Status, message count, error rate
- **Agent Detail** — Click an agent for metrics, evolution history, conversations
- **Evolution view** — Watch fitness scores change over time

## Enable Evolution

With evolution enabled (`evolution.enabled: true`), your agent will:

1. **Accumulate metrics** — Track success rate, response time, token usage
2. **Get evaluated** — Every `evalIntervalSec` seconds, fitness is calculated
3. **Evolve if needed** — If fitness drops below threshold (0.6), parameters mutate
4. **Self-improve** — Temperature, token limits, and custom params adjust

See [Evolution Engine](../architecture/evolution.md) for details.

## Next Steps

- [Deployment Profiles](../guides/deployment-profiles.md) — Solo, Hub & Spoke, Cloud Fleet
- [Trading Agent Guide](../guides/trading-agent.md) — Set up a Hyperliquid trader
- [Companion Agent Guide](../guides/companion-agent.md) — Build a companion device
- [Architecture Overview](../architecture/overview.md) — Understand the system
- [Model Routing](../guides/model-routing.md) — Optimize model selection
