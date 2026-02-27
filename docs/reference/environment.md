# Environment Variables

EvoClaw supports configuration via environment variables for deployment flexibility.

## Orchestrator Environment Variables

| Variable | Config Key | Default | Description |
|----------|-----------|---------|-------------|
| `EVOCLAW_PORT` | `server.port` | `8420` | HTTP API port |
| `EVOCLAW_DATA_DIR` | `server.dataDir` | `./data` | Data directory |
| `EVOCLAW_LOG_LEVEL` | `server.logLevel` | `info` | Log level |
| `EVOCLAW_MQTT_HOST` | `mqtt.host` | `0.0.0.0` | MQTT broker host |
| `EVOCLAW_MQTT_PORT` | `mqtt.port` | `1883` | MQTT broker port |
| `EVOCLAW_MQTT_USERNAME` | `mqtt.username` | — | MQTT auth user |
| `EVOCLAW_MQTT_PASSWORD` | `mqtt.password` | — | MQTT auth password |
| `ANTHROPIC_API_KEY` | `models.providers.anthropic.apiKey` | — | Anthropic API key |
| `OPENAI_API_KEY` | `models.providers.openai.apiKey` | — | OpenAI API key |
| `OPENROUTER_API_KEY` | `models.providers.openrouter.apiKey` | — | OpenRouter API key |
| `TELEGRAM_BOT_TOKEN` | `channels.telegram.botToken` | — | Telegram bot token |

## Edge Agent Environment Variables

| Variable | Config Key | Default | Description |
|----------|-----------|---------|-------------|
| `EVOCLAW_AGENT_ID` | `agent_id` | — | Agent identifier |
| `EVOCLAW_AGENT_TYPE` | `agent_type` | — | Agent type |
| `EVOCLAW_MQTT_BROKER` | `mqtt.broker` | `localhost` | MQTT broker |
| `EVOCLAW_MQTT_PORT` | `mqtt.port` | `1883` | MQTT port |
| `EVOCLAW_ORCHESTRATOR_URL` | `orchestrator.url` | `http://localhost:8420` | Orchestrator API URL |
| `HL_API_URL` | `trading.hyperliquid_api` | `https://api.hyperliquid.xyz` | Hyperliquid API |
| `HL_WALLET_ADDRESS` | `trading.wallet_address` | — | Wallet address |
| `HL_PRIVATE_KEY_PATH` | `trading.private_key_path` | — | Private key file path |

## Usage

### Docker Compose

```yaml
services:
  orchestrator:
    environment:
      - EVOCLAW_LOG_LEVEL=debug
      - ANTHROPIC_API_KEY=sk-ant-your-key
      - TELEGRAM_BOT_TOKEN=your-token
```

### Shell

```bash
export ANTHROPIC_API_KEY="sk-ant-your-key"
export EVOCLAW_LOG_LEVEL="debug"
./evoclaw --config evoclaw.json
```

### systemd

```ini
[Service]
Environment=EVOCLAW_LOG_LEVEL=info
Environment=ANTHROPIC_API_KEY=sk-ant-your-key
EnvironmentFile=/opt/evoclaw/.env
```

### .env File

```bash
# /opt/evoclaw/.env
ANTHROPIC_API_KEY=sk-ant-your-key
OPENAI_API_KEY=sk-your-key
TELEGRAM_BOT_TOKEN=your-token
EVOCLAW_LOG_LEVEL=info
```

> ⚠️ Never commit `.env` files to version control. The `.gitignore` already excludes them.

## Priority

Configuration priority (highest first):

1. Environment variables
2. Config file (`evoclaw.json`)
3. Default values

## See Also

- [Configuration Guide](../getting-started/configuration.md)
- [Config Schema](config-schema.md)
- [Deployment Guide](../guides/deployment.md)
