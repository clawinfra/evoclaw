# Dashboard Chat Guide

Chat with your agents directly from the EvoClaw web dashboard.

## Accessing the Chat

1. Open the EvoClaw dashboard at `http://localhost:8420`
2. Click **💬 Chat** in the sidebar navigation

## Features

### Agent Selector
Use the dropdown in the top-right corner to switch between available agents. Each agent may have different capabilities and model configurations.

### Sending Messages
Type your message in the input area at the bottom and press Enter or click **Send ➤**. The message is sent to your selected agent's LLM.

### Message Display
- **Your messages** appear on the right with a blue accent
- **Agent responses** appear on the left with a dark background
- **System messages** (errors, etc.) appear centered with a yellow accent
- Each message shows a timestamp
- Agent responses include model name, response time, and token count

### Conversation History
- Chat history is stored both on the server (in MemoryStore) and locally (in localStorage)
- The last 20 messages are included as context for new LLM requests
- History persists across page reloads
- Use **🗑️ Clear** to reset the conversation

### Markdown Rendering
Agent responses support basic markdown:
- **Bold** text
- *Italic* text
- `Inline code`
- Code blocks with ``` delimiters

## API Endpoints

The chat widget uses these REST endpoints:

### POST /api/chat
Send a message and get a synchronous response.

```json
// Request
{
  "agent_id": "pi1-edge",
  "message": "What is the CPU temperature?",
  "conversation_id": "conv-123"  // optional
}

// Response
{
  "agent_id": "pi1-edge",
  "response": "The current CPU temperature is 48.3°C...",
  "model": "ollama/qwen2.5:1.5b",
  "elapsed_ms": 2880,
  "tokens_input": 120,
  "tokens_output": 54,
  "timestamp": "2025-02-07T12:00:00Z"
}
```

### GET /api/chat/history
Retrieve conversation history for an agent.

```
GET /api/chat/history?agent_id=pi1-edge&limit=50
```

```json
{
  "agent_id": "pi1-edge",
  "message_count": 4,
  "messages": [
    {"role": "user", "content": "Hello"},
    {"role": "assistant", "content": "Hi! How can I help?"},
    ...
  ]
}
```

### GET /api/chat/stream (SSE)
Stream chat responses via Server-Sent Events.

```
GET /api/chat/stream?agent_id=pi1-edge&message=What+is+the+weather
```

Events:
- `{"type": "thinking", "agent_id": "pi1-edge"}` — processing started
- `{"type": "response", "response": "...", ...}` — full response
- `{"type": "done"}` — stream complete
- `{"type": "error", "error": "..."}` — error occurred

## How It Works

```
Dashboard UI → POST /api/chat → Server → ChatSync → LLM Provider
     ↑                                                    ↓
     └──────────── JSON Response ←─── ChatSyncResponse ←──┘
```

1. User types a message in the chat input
2. Dashboard sends POST to `/api/chat`
3. Server loads conversation history from MemoryStore
4. `ChatSync` builds the prompt with system prompt + history + new message
5. LLM provider generates a response
6. Response is stored in memory and returned to the dashboard
7. Dashboard renders the response with metadata

## Configuration

The chat widget works with any agent configured in `evoclaw.json`. No additional configuration is needed beyond having agents and models set up.

The conversation context window is managed by the MemoryStore:
- Last 20 messages included in LLM context
- Up to 100 messages stored per conversation
- Token limit of 100,000 per conversation before trimming
