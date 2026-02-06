# WebSocket & SSE Endpoints

EvoClaw provides real-time data streaming via Server-Sent Events (SSE).

## Server-Sent Events (SSE)

### Log Stream

**Endpoint:** `GET /api/logs/stream`

Streams real-time log entries as SSE events.

#### Connection

```javascript
const source = new EventSource('http://localhost:8420/api/logs/stream');

source.onmessage = (event) => {
    const log = JSON.parse(event.data);
    console.log(`[${log.level}] ${log.component}: ${log.message}`);
};

source.onerror = (error) => {
    console.error('SSE connection error:', error);
};
```

#### Event Format

```
data: {"time":"10:30:05","level":"info","component":"api","message":"HTTP request completed"}

data: {"time":"10:30:10","level":"info","component":"system","message":"heartbeat: 3 agents online"}
```

#### Event Schema

```json
{
  "time": "10:30:05",
  "level": "info",
  "component": "api",
  "message": "HTTP request completed"
}
```

| Field | Type | Values |
|-------|------|--------|
| `time` | string | HH:MM:SS format |
| `level` | string | `debug`, `info`, `warn`, `error` |
| `component` | string | `api`, `orchestrator`, `model-router`, `registry`, `evolution`, `mqtt`, `telegram`, `system` |
| `message` | string | Human-readable log message |

#### Headers

The server sets these response headers:

```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
Access-Control-Allow-Origin: *
```

#### Heartbeat

The server sends periodic heartbeat events (every 5 seconds) to keep the connection alive:

```
data: {"time":"10:30:10","level":"info","component":"system","message":"heartbeat: 3 agents online"}
```

#### Client Example (curl)

```bash
curl -N http://localhost:8420/api/logs/stream
```

#### Client Example (Python)

```python
import requests
import json

response = requests.get(
    'http://localhost:8420/api/logs/stream',
    stream=True
)

for line in response.iter_lines():
    if line:
        line = line.decode('utf-8')
        if line.startswith('data: '):
            data = json.loads(line[6:])
            print(f"[{data['level'].upper()}] {data['component']}: {data['message']}")
```

## Dashboard Integration

The web dashboard uses SSE for the Logs view:

```javascript
// From app.js
startLogStream() {
    this.logEventSource = new EventSource('/api/logs/stream');
    this.logStreaming = true;

    this.logEventSource.onmessage = (event) => {
        const log = JSON.parse(event.data);
        this.logs.push(log);
        // Keep last 1000 entries
        if (this.logs.length > 1000) {
            this.logs = this.logs.slice(-1000);
        }
    };
}
```

## Polling Fallback

For environments where SSE isn't available, use polling:

```javascript
// Poll /api/status every 30 seconds
setInterval(async () => {
    const response = await fetch('/api/status');
    const data = await response.json();
    updateDashboard(data);
}, 30000);
```

The dashboard automatically falls back to polling if SSE fails.

## Future: WebSocket Support

WebSocket support for bidirectional real-time communication is planned:

- Agent command/control channel
- Live metric streaming
- Interactive trading controls

## Reverse Proxy Configuration

When running behind nginx or similar:

```nginx
# SSE endpoint needs special proxy settings
location /api/logs/stream {
    proxy_pass http://127.0.0.1:8420;
    proxy_set_header Connection '';
    proxy_http_version 1.1;
    chunked_transfer_encoding off;
    proxy_buffering off;
    proxy_cache off;
    proxy_read_timeout 86400;  # Keep alive for 24h
}
```

## See Also

- [REST API Reference](rest-api.md)
- [MQTT Protocol](mqtt-protocol.md)
- [Deployment Guide](../guides/deployment.md)
