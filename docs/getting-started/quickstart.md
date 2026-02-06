# Quick Start

Get EvoClaw running in 5 minutes.

## Prerequisites

- Go 1.24+ or a container runtime (Podman recommended, Docker works too)
- An LLM API key (Anthropic, OpenAI, or a local Ollama instance)

## Step 1: Build

### Option A: Container (fastest)

```bash
git clone https://github.com/clawinfra/evoclaw.git
cd evoclaw

# Podman (recommended)
make up

# Docker (fallback)
make up-docker
```

### Option B: From source

```bash
git clone https://github.com/clawinfra/evoclaw.git
cd evoclaw

# Build orchestrator
go build -ldflags="-s -w" -o evoclaw ./cmd/evoclaw
```

## Step 2: Configure

```bash
cp evoclaw.example.json evoclaw.json
```

Edit `evoclaw.json` — at minimum, add an LLM provider:

```json
{
  "server": {
    "port": 8420,
    "dataDir": "./data",
    "logLevel": "info"
  },
  "models": {
    "providers": {
      "anthropic": {
        "apiKey": "sk-ant-YOUR_KEY_HERE",
        "models": [
          {
            "id": "claude-sonnet-4-20250514",
            "name": "Claude Sonnet 4",
            "contextWindow": 200000,
            "costInput": 3.0,
            "costOutput": 15.0,
            "capabilities": ["reasoning", "code"]
          }
        ]
      }
    },
    "routing": {
      "simple": "anthropic/claude-sonnet-4-20250514",
      "complex": "anthropic/claude-sonnet-4-20250514",
      "critical": "anthropic/claude-sonnet-4-20250514"
    }
  },
  "evolution": {
    "enabled": true,
    "evalIntervalSec": 3600,
    "minSamplesForEval": 10,
    "maxMutationRate": 0.2
  },
  "agents": [
    {
      "id": "assistant-1",
      "name": "My First Agent",
      "type": "orchestrator",
      "model": "anthropic/claude-sonnet-4-20250514",
      "systemPrompt": "You are a helpful assistant that gives concise, accurate answers.",
      "skills": ["chat"]
    }
  ]
}
```

## Step 3: Run

```bash
./evoclaw --config evoclaw.json
```

You should see:

```
  ╔═══════════════════════════════════════╗
  ║        🧬 EvoClaw v0.1.0            ║
  ║  Self-Evolving Agent Framework        ║
  ║  Every device is an agent.            ║
  ║  Every agent evolves.                 ║
  ╚═══════════════════════════════════════╝

  🌐 API: http://localhost:8420
  📊 Dashboard: http://localhost:8420
  🤖 Agents: 1 loaded
  🧠 Models: 1 available
```

## Step 4: Verify

```bash
# Check system status
curl http://localhost:8420/api/status | jq

# List agents
curl http://localhost:8420/api/agents | jq

# Check models
curl http://localhost:8420/api/models | jq
```

## Step 5: Open the Dashboard

Navigate to [http://localhost:8420](http://localhost:8420) in your browser.

The dashboard shows:
- **Overview** — System status, agent count, API costs
- **Agents** — All registered agents with metrics
- **Models** — Available models and cost tracking
- **Evolution** — Fitness scores and mutation history

## Step 6: Add Telegram (Optional)

To interact with your agent via Telegram:

1. Create a bot with [@BotFather](https://t.me/BotFather)
2. Add the token to your config:

```json
{
  "channels": {
    "telegram": {
      "enabled": true,
      "botToken": "123456:ABC-DEF1234ghIkl-zyx57W2v1u123ew11"
    }
  }
}
```

3. Restart EvoClaw and message your bot!

## What's Next?

- [Create your first custom agent](first-agent.md)
- [Container deployment guide](../guides/container-deployment.md) — Podman pods, systemd, production
- [Edge deployment guide](../guides/edge-deployment.md) — Deploy to Raspberry Pi and ARM devices
- [Set up a trading agent](../guides/trading-agent.md)
- [Understand the architecture](../architecture/overview.md)
- [Configure model routing](../guides/model-routing.md)
