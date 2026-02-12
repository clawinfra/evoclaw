
# Messaging Interfaces

EvoClaw provides multiple ways to interact with agents:

## 1. HTTP Chat API

**POST /api/chat**

Send messages to agents via REST API.

**Request:**
```json
{
  "agent": "agent-id",
  "message": "What is 2+2?",
  "from": "optional-sender-id"
}
```

**Response:**
```json
{
  "agent": "agent-id",
  "message": "4",
  "model": "ollama/qwen2.5:1.5b",
  "timestamp": "2026-02-12T14:30:00Z"
}
```

**Example:**
```bash
curl -X POST http://localhost:8421/api/chat \
  -H "Content-Type: application/json" \
  -d '{
    "agent": "my-agent",
    "message": "Hello, world!"
  }'
```

## 2. Web Terminal

**URL:** http://localhost:8421/terminal

Browser-based chat interface with:
- Agent selection dropdown
- Chat history
- Real-time messaging
- Dark terminal theme

**Features:**
- ✅ Select any available agent
- ✅ Send messages via input box
- ✅ View conversation history
- ✅ Keyboard shortcuts (Enter to send)
- ✅ Connection status indicator

## 3. TUI (Terminal UI)

**Command:** `evoclaw-tui`

Terminal-based chat client for command-line users.

**Installation:**
```bash
go build -o evoclaw-tui ./cmd/evoclaw-tui
```

**Usage:**
```bash
# Auto-select agent (if only one available)
./evoclaw-tui

# Specify agent
./evoclaw-tui --agent my-agent

# Custom API URL
./evoclaw-tui --api http://localhost:8421
```

**Features:**
- ✅ Colored terminal output
- ✅ Agent selection
- ✅ Interactive chat loop
- ✅ Type 'exit' or 'quit' to exit

## 4. MQTT (Advanced)

For edge agents and programmatic access.

**Send Command:**
```bash
mosquitto_pub -h localhost \
  -t "evoclaw/agents/agent-id/command" \
  -m '{"action":"query","message":"Status?"}'
```

**Subscribe to Responses:**
```bash
mosquitto_sub -h localhost \
  -t "evoclaw/agents/+/response" -v
```

## 5. Telegram Bot

For mobile and messaging app access.

**Setup:**
1. Get bot token from @BotFather
2. Enable in config:
   ```json
   {
     "channels": {
       "telegram": {
         "enabled": true,
         "botToken": "YOUR_TOKEN",
         "allowedUsers": [YOUR_USER_ID]
       }
     }
   }
   ```
3. Message your bot on Telegram

## Comparison

| Interface | Best For | Pros | Cons |
|-----------|----------|------|------|
| **HTTP API** | Integration, automation | Simple, RESTful, language-agnostic | No streaming (yet) |
| **Web Terminal** | Quick testing, demos | Visual, user-friendly, no install | Requires browser |
| **TUI** | CLI users, SSH access | Fast, keyboard-driven, lightweight | Terminal-only |
| **MQTT** | Edge agents, IoT | Real-time, pub/sub, scalable | More complex setup |
| **Telegram** | Mobile, daily use | Familiar UI, notifications, cross-device | Requires bot setup |

## Testing the Intelligent Router

Once you have a messaging interface set up, test the router with queries of varying complexity:

**SIMPLE tier (GLM 4.5 Air - $0.10/M tokens):**
```
What is 2+2?
```

**MEDIUM tier (Llama 3.3 70B - $0.40/M tokens):**
```
Explain recursion in programming
```

**COMPLEX tier (DeepSeek V3.2 - $0.85/M tokens):**
```
Analyze the time complexity of QuickSort
```

**REASONING tier (R1 32B - $0.20/M tokens):**
```
If all Bloops are Razzies, are all Bloops definitely Lazzies? Explain step-by-step.
```

Check logs to see which model was selected:
```bash
tail -f evoclaw.log | grep -i "router\|tier"
```

## Architecture

```
┌─────────────┐     HTTP      ┌──────────────┐
│ Web Browser │ ──────────────→│              │
└─────────────┘                │              │
                               │              │
┌─────────────┐     HTTP      │   EvoClaw    │
│   curl/API  │ ──────────────→│ Orchestrator │
└─────────────┘                │              │
                               │              │
┌─────────────┐    Terminal   │   Inbox      │
│     TUI     │ ──────────────→│   Channel    │
└─────────────┘                │              │
                               │              │──→ Agents
┌─────────────┐     MQTT      │              │
│ Edge Agents │ ←─────────────→│              │
└─────────────┘                │              │
                               │              │
┌─────────────┐   Telegram    │              │
│   Mobile    │ ←─────────────→│              │
└─────────────┘                └──────────────┘
```

All interfaces feed into the same orchestrator inbox, ensuring consistent behavior regardless of how messages arrive.

## API Reference

### POST /api/chat

Send a message to an agent.

**Headers:**
- `Content-Type: application/json`
- `Authorization: Bearer TOKEN` (if JWT auth enabled)

**Body:**
```typescript
{
  agent: string;   // Required: Agent ID
  message: string; // Required: User message
  from?: string;   // Optional: Sender identifier (default: "http-api")
}
```

**Response (200 OK):**
```typescript
{
  agent: string;     // Agent that processed the message
  message: string;   // Agent's response
  model: string;     // Model used for generation
  timestamp: string; // ISO 8601 timestamp
}
```

**Error Responses:**
- `400 Bad Request` - Invalid request body or missing fields
- `404 Not Found` - Agent not found
- `408 Request Timeout` - Processing timeout (30s)
- `503 Service Unavailable` - Orchestrator not ready

### POST /api/chat/stream

Stream agent responses via Server-Sent Events (SSE).

**Status:** Coming soon

### GET /terminal

Web-based chat interface.

**Returns:** HTML page with embedded JavaScript

## Examples

### Python
```python
import requests

resp = requests.post('http://localhost:8421/api/chat', json={
    'agent': 'my-agent',
    'message': 'Hello from Python!'
})

data = resp.json()
print(f"{data['agent']}: {data['message']}")
```

### JavaScript
```javascript
async function chat(agent, message) {
    const resp = await fetch('http://localhost:8421/api/chat', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ agent, message })
    });
    const data = await resp.json();
    console.log(`${data.agent}: ${data.message}`);
}

chat('my-agent', 'Hello from JavaScript!');
```

### Go
```go
package main

import (
	"bytes"
	"encoding/json"
	"net/http"
)

func chat(agent, message string) error {
	req := map[string]string{
		"agent":   agent,
		"message": message,
	}
	
	data, _ := json.Marshal(req)
	resp, err := http.Post(
		"http://localhost:8421/api/chat",
		"application/json",
		bytes.NewBuffer(data),
	)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	
	var result map[string]string
	json.NewDecoder(resp.Body).Decode(&result)
	println(result["agent"] + ": " + result["message"])
	return nil
}
```

## Troubleshooting

### "Orchestrator not ready"

The orchestrator hasn't finished initialization. Wait a few seconds and retry.

### "Agent not found"

Check available agents:
```bash
curl http://localhost:8421/api/agents
```

### "Request timeout"

The agent is taking longer than 30 seconds to respond. This usually means:
- Model is slow or not responding
- Agent is overloaded
- Network issue with model provider

Check logs for details:
```bash
tail -f evoclaw.log | grep -i error
```

### Web terminal shows "Loading agents..."

API server may not be running or CORS is blocking requests. Check:
```bash
curl http://localhost:8421/api/status
```

## Next Steps

1. **Enable HTTP API** - Already available, just start the orchestrator
2. **Try web terminal** - Open http://localhost:8421/terminal
3. **Build TUI** - `go build -o evoclaw-tui ./cmd/evoclaw-tui`
4. **Test with different agents** - See which models get selected
5. **Integrate into your app** - Use the HTTP API from any language

---

**Version:** 1.0  
**Added:** 2026-02-12  
**Status:** Production Ready
