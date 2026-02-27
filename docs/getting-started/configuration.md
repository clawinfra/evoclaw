# Configuration

EvoClaw is configured via a JSON file (`evoclaw.json` by default). This document covers all configuration options.

## Config File

```bash
# Use default path
./evoclaw

# Specify custom config path
./evoclaw --config /path/to/config.json
```

If no config file exists, EvoClaw creates a default one on first run.

## Full Configuration Reference

```json
{
  "server": {
    "port": 8420,
    "dataDir": "./data",
    "logLevel": "info"
  },
  "mqtt": {
    "host": "0.0.0.0",
    "port": 1883,
    "username": "",
    "password": ""
  },
  "channels": {
    "telegram": {
      "enabled": true,
      "botToken": "YOUR_TELEGRAM_BOT_TOKEN"
    }
  },
  "models": {
    "providers": {
      "anthropic": {
        "apiKey": "sk-ant-...",
        "models": [
          {
            "id": "claude-sonnet-4-20250514",
            "name": "Claude Sonnet 4",
            "contextWindow": 200000,
            "costInput": 3.0,
            "costOutput": 15.0,
            "capabilities": ["reasoning", "code", "vision"]
          }
        ]
      },
      "ollama": {
        "baseUrl": "http://localhost:11434",
        "models": [
          {
            "id": "llama3.2:3b",
            "name": "Llama 3.2 3B",
            "contextWindow": 128000,
            "costInput": 0.0,
            "costOutput": 0.0,
            "capabilities": ["reasoning"]
          }
        ]
      }
    },
    "routing": {
      "simple": "ollama/llama3.2:3b",
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
      "name": "General Assistant",
      "type": "orchestrator",
      "model": "anthropic/claude-sonnet-4-20250514",
      "systemPrompt": "You are a helpful assistant.",
      "skills": ["chat", "search", "code"],
      "config": {},
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

## Section Reference

### `server`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `port` | int | `8420` | HTTP API and dashboard port |
| `dataDir` | string | `"./data"` | Directory for persistent data (agents, memory, evolution) |
| `logLevel` | string | `"info"` | Log level: `debug`, `info`, `warn`, `error` |

### `mqtt`

MQTT broker settings for the agent mesh.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `host` | string | `"0.0.0.0"` | MQTT broker host to connect to |
| `port` | int | `1883` | MQTT broker port |
| `username` | string | `""` | MQTT authentication username |
| `password` | string | `""` | MQTT authentication password |

### `channels`

Communication channel configurations.

#### `channels.telegram`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable Telegram bot |
| `botToken` | string | `""` | Telegram Bot API token from @BotFather |

### `models`

LLM provider and routing configuration.

#### `models.providers`

Map of provider name → provider config. Supported providers:
- `anthropic` — Claude models (native client)
- `openai` — OpenAI models (GPT-4, etc.)
- `ollama` — Local models via Ollama
- `openrouter` — OpenRouter aggregator
- Any custom provider with OpenAI-compatible API

Each provider config:

| Field | Type | Description |
|-------|------|-------------|
| `baseUrl` | string | API base URL (required for Ollama, OpenRouter) |
| `apiKey` | string | API key for authentication |
| `models` | array | List of available models |

Each model:

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Model identifier (e.g., `claude-sonnet-4-20250514`) |
| `name` | string | Human-readable name |
| `contextWindow` | int | Maximum context length in tokens |
| `costInput` | float | Cost per million input tokens (USD) |
| `costOutput` | float | Cost per million output tokens (USD) |
| `capabilities` | array | List of capabilities: `reasoning`, `code`, `vision` |

#### `models.routing`

Intelligent model selection based on task complexity:

| Field | Type | Description |
|-------|------|-------------|
| `simple` | string | Model for simple tasks (format: `provider/model-id`) |
| `complex` | string | Model for complex reasoning tasks |
| `critical` | string | Model for critical tasks (trading, financial decisions) |

### `evolution`

Evolution engine configuration.

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `true` | Enable the evolution engine |
| `evalIntervalSec` | int | `3600` | Seconds between evaluations (default: 1 hour) |
| `minSamplesForEval` | int | `10` | Minimum actions before first evaluation |
| `maxMutationRate` | float | `0.2` | Maximum strategy mutation rate (0.0–1.0) |

### `agents`

Array of agent definitions.

| Field | Type | Description |
|-------|------|-------------|
| `id` | string | Unique agent identifier |
| `name` | string | Human-readable agent name |
| `type` | string | Agent type: `orchestrator`, `trader`, `monitor`, `governance` |
| `model` | string | Default model (format: `provider/model-id`) |
| `systemPrompt` | string | System prompt for the agent's LLM |
| `skills` | array | List of enabled skills |
| `config` | object | Additional key-value configuration |
| `container` | object | Container isolation settings |

#### `agents[].container`

| Field | Type | Default | Description |
|-------|------|---------|-------------|
| `enabled` | bool | `false` | Enable container isolation |
| `image` | string | `""` | Container image |
| `memoryMb` | int | `512` | Memory limit in MB |
| `cpuShares` | int | `256` | CPU share allocation |
| `allowNet` | bool | `true` | Allow network access |
| `allowTools` | array | `[]` | Allowed tool names |
| `mounts` | array | `[]` | Volume mounts |

## Environment Variables

Config values can be overridden with environment variables. See [Environment Variables](../reference/environment.md).

## Edge Agent Configuration

The Rust edge agent uses TOML configuration. See [agent.example.toml](https://github.com/clawinfra/evoclaw/blob/main/edge-agent/agent.example.toml) and the [Edge Agent docs](../architecture/edge-agent.md).

## Next Steps

- [Quick Start](quickstart.md) — Get running in 5 minutes
- [First Agent](first-agent.md) — Create your first agent
- [Model Routing](../guides/model-routing.md) — Advanced model configuration
