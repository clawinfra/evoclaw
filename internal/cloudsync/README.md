# EvoClaw Cloud Sync

Cloud synchronization for EvoClaw agent memory using Turso (libSQL) as the cloud database backend.

## Features

- **Zero CGO dependencies** — Uses Turso HTTP API instead of native libSQL bindings
- **Tiered memory model** — Core (permanent), Warm (30 days), Cold (unlimited archive)
- **Multi-device sync** — Same agent across multiple devices with conflict resolution
- **Offline queue** — Buffers sync operations when cloud is unreachable
- **Disaster recovery** — Device replacement, theft protection, kill switch
- **Exponential backoff** — Automatic retry with backoff on network failures

## Architecture

```
┌─────────────┐              ┌──────────────────────┐
│  Device A   │──── sync ───▶│    Turso Cloud DB    │
│  (original) │   encrypted  │                      │
└──────┬──────┘              │  ┌────────────────┐  │
       │                     │  │  Agent Soul     │  │
       │ 💥 lost/broken      │  │  ────────────   │  │
       │                     │  │  core_memory    │  │
┌──────┴──────┐              │  │  genome.toml    │  │
│  Device B   │◀── restore ──│  │  persona/*      │  │
│  (replace)  │   < 2 min    │  │  warm_memory    │  │
│             │              │  │  evolution_log   │  │
└─────────────┘              │  │  full_archive   │  │
                             │  └────────────────┘  │
                             └──────────────────────┘
```

## Database Schema

### Tables

- **`agents`** — Agent identity, genome, capabilities
- **`core_memory`** — Permanent relationship data (NEVER deleted)
- **`warm_memory`** — Recent conversations/events (30 day retention)
- **`evolution_log`** — Evolution events with fitness scores
- **`action_log`** — Agent actions (on-chain or local)
- **`devices`** — Registered devices for multi-device sync
- **`sync_state`** — Sync tracking per device

## Usage

### Initialize Database

```bash
export TURSO_DATABASE_URL="libsql://your-db.turso.io"
export TURSO_AUTH_TOKEN="your-auth-token"
go run cmd/init-turso/main.go
```

### Configuration

Add to `evoclaw.json`:

```json
{
  "cloudSync": {
    "enabled": true,
    "databaseUrl": "libsql://your-db.turso.io",
    "authToken": "your-auth-token",
    "deviceId": "auto-generated",
    "deviceKey": "auto-generated",
    "heartbeatIntervalSeconds": 60,
    "criticalSyncEnabled": true,
    "warmSyncIntervalMinutes": 60,
    "fullSyncIntervalHours": 24,
    "fullSyncRequireWifi": true,
    "maxOfflineQueueSize": 1000
  }
}
```

### Code Integration

```go
import (
    "github.com/clawinfra/evoclaw/internal/cloudsync"
    "github.com/clawinfra/evoclaw/internal/config"
)

// Create manager
cfg := config.CloudSyncConfig{ /* ... */ }
manager, err := cloudsync.NewManager(cfg, logger)
if err != nil {
    log.Fatal(err)
}

// Start background sync
ctx := context.Background()
if err := manager.Start(ctx); err != nil {
    log.Fatal(err)
}
defer manager.Stop()

// Critical sync after every conversation
memory := &cloudsync.AgentMemory{
    AgentID: "agent-1",
    Name: "Bloop",
    CoreMemory: map[string]interface{}{
        "owner": "Maggie",
        "relationship": "trusted companion",
    },
}
if err := manager.SyncCritical(ctx, memory); err != nil {
    log.Printf("Sync failed: %v", err)
}

// Restore agent to new device
restored, err := manager.RestoreToDevice(ctx, "agent-1", "new-device-id", "new-device-key")
if err != nil {
    log.Fatal(err)
}
```

## Sync Operations

### Critical Sync (After Every Conversation)
- Syncs core memory and genome
- Must succeed before agent enters idle
- Retries with exponential backoff
- Queued offline if cloud unreachable

### Warm Sync (Hourly)
- Syncs recent conversations and events
- 30 day retention (auto-evicted)
- Background operation

### Full Sync (Daily)
- Complete backup of evolution log and action log
- Unlimited retention
- Only on WiFi (configurable)

## Recovery Operations

### Restore Agent
```go
memory, err := manager.RestoreAgent(ctx, "agent-1")
```

### Restore to New Device
```go
memory, err := manager.RestoreToDevice(ctx, "agent-1", "new-device", "new-key")
```

### Get Recent Conversations
```go
entries, err := manager.GetWarmMemory(ctx, "agent-1", 100)
```

### Mark Device as Stolen
```go
err := manager.MarkDeviceStolen(ctx, "stolen-device-id")
```

## Testing

```bash
# Run all tests
go test ./internal/cloudsync/... -v

# Run specific test
go test ./internal/cloudsync/... -run TestTursoClient_Execute -v

# With coverage
go test ./internal/cloudsync/... -cover
```

## Implementation Details

### Turso HTTP API

Uses Turso's `/v2/pipeline` endpoint for batch operations:

```json
POST https://your-db.turso.io/v2/pipeline
Authorization: Bearer <token>

{
  "requests": [
    {
      "type": "execute",
      "stmt": {
        "sql": "INSERT INTO agents (agent_id, name) VALUES (?, ?)",
        "args": ["agent-1", "Bloop"]
      }
    }
  ]
}
```

### Offline Queue

When cloud is unreachable:
1. Operations are queued locally
2. Non-critical operations evicted if queue full
3. Retry on next heartbeat/warm sync
4. Critical operations prioritized

### URL Conversion

Automatically converts `libsql://` URLs to `https://` for HTTP API:

```go
libsql://db.turso.io → https://db.turso.io
```

## Design Docs

See `/docs/CLOUD-SYNC.md` for full specification.

## License

Part of the EvoClaw project.
