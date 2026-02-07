# Telegram Bot Setup Guide

Connect your EvoClaw orchestrator to Telegram for human→agent communication.

## Prerequisites

- A running EvoClaw instance with at least one agent configured
- A Telegram account

## Step 1: Create a Bot with BotFather

1. Open Telegram and search for `@BotFather`
2. Send `/newbot`
3. Choose a name (e.g., "EvoClaw Agent Hub")
4. Choose a username (must end in `bot`, e.g., `evoclaw_agent_bot`)
5. Copy the **HTTP API token** (looks like `1234567890:ABCdefGHIjklMNOpqrsTUVwxyz`)

## Step 2: Configure EvoClaw

Edit your `evoclaw.json`:

```json
{
  "channels": {
    "telegram": {
      "enabled": true,
      "botToken": "YOUR_BOT_TOKEN_HERE",
      "allowedUsers": [],
      "defaultAgent": "pi1-edge"
    }
  }
}
```

### Configuration Options

| Field | Type | Description |
|-------|------|-------------|
| `enabled` | bool | Enable/disable the Telegram bot |
| `botToken` | string | Bot token from BotFather |
| `allowedUsers` | int64[] | Telegram user IDs that can use the bot. Empty = allow everyone |
| `defaultAgent` | string | Which agent handles messages by default |

### Finding Your User ID

To restrict access to specific users:

1. Send a message to `@userinfobot` on Telegram
2. It will reply with your user ID (a number like `123456789`)
3. Add it to the `allowedUsers` array

```json
"allowedUsers": [123456789, 987654321]
```

## Step 3: Start EvoClaw

```bash
./evoclaw --config evoclaw.json
```

You should see in the logs:

```
telegram channel started
telegram bot started  defaultAgent=pi1-edge
```

## Bot Commands

| Command | Description |
|---------|-------------|
| `/start` | Welcome message and help |
| `/help` | List all commands |
| `/status` | Show all agents and their metrics |
| `/agents` | List available agents |
| `/agent <id>` | Switch to talking to a specific agent |
| `/skills <agent_id>` | List an agent's configured skills |
| `/ask <question>` | Ask a question through your active agent |

### Regular Messages

Any non-command message is automatically routed to your currently selected agent's LLM. The response includes:
- The LLM's reply
- Which agent and model processed it
- Response time in milliseconds

### Switching Agents

```
/agents                  → see what's available
/agent pi1-edge          → switch to pi1-edge
What's the CPU temp?     → message goes to pi1-edge
/agent trading-agent     → switch to trading agent
Check BTC price          → message goes to trading-agent
```

## Architecture

```
Human → Telegram → TelegramBot → ChatSync → LLM Provider → Response → Telegram → Human
                      ↓
                  /commands → Direct response (no LLM)
```

The Telegram bot:
1. Receives messages via long polling (no webhooks needed)
2. Parses bot commands (`/status`, `/agents`, etc.)
3. Routes regular messages to `ChatSync` which calls the LLM
4. Sends the LLM response back via Telegram API
5. Supports Markdown formatting in responses

## Security

- **Token**: Keep your bot token secret. Don't commit it to version control.
- **User allowlist**: Use `allowedUsers` to restrict who can interact with your bot.
- **No webhooks**: Uses long polling, so no public endpoint needed.

## Troubleshooting

### Bot doesn't respond
- Check the bot token is correct
- Verify the bot is not already running elsewhere (only one instance can poll)
- Check EvoClaw logs for errors

### "agent not found" errors
- Ensure the `defaultAgent` ID matches an agent in your config
- Run `/agents` to see available agents

### Slow responses
- LLM inference time depends on your model and hardware
- Local Ollama models on Raspberry Pi may take 10-30 seconds
- Consider using a faster model for chat interactions
