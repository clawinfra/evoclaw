# EvoClaw Scheduler

**Periodic task execution for edge devices and IoT deployments.**

---

## Overview

The EvoClaw scheduler enables periodic tasks to run automatically without external cron or systemd timers. Perfect for IoT edge devices that need to:

- Read sensors at regular intervals
- Analyze data periodically
- Sync to cloud on schedule
- Send reports at specific times

## Features

- ✅ **Self-contained** - No external cron needed
- ✅ **Edge-friendly** - Lightweight, minimal dependencies
- ✅ **Observable** - Built-in state tracking (runs, errors, duration)
- ✅ **Flexible schedules** - Interval, cron expressions, daily-at-time
- ✅ **Multiple action types** - Shell, agent messages, MQTT, HTTP
- ✅ **Persistent** - Jobs defined in config, survive restarts

## Configuration

Add to `evoclaw.json`:

```json
{
  "scheduler": {
    "enabled": true,
    "jobs": [
      {
        "id": "sensor-read",
        "name": "Read Temperature Sensor",
        "enabled": true,
        "schedule": {
          "kind": "interval",
          "intervalMs": 300000
        },
        "action": {
          "kind": "shell",
          "command": "./sensors/read_temp.sh"
        }
      }
    ]
  }
}
```

## Schedule Types

### Interval
Run every N milliseconds:
```json
{
  "kind": "interval",
  "intervalMs": 300000  // 5 minutes
}
```

### Cron Expression
Run on cron schedule:
```json
{
  "kind": "cron",
  "expr": "0 */6 * * *",  // Every 6 hours
  "timezone": "UTC"       // Optional, defaults to local
}
```

Standard cron format: `minute hour day month weekday`

### Daily At Time
Run once per day at specific time:
```json
{
  "kind": "at",
  "time": "09:00",         // HH:MM format
  "timezone": "America/New_York"  // Optional
}
```

## Action Types

### Shell Command
Execute shell scripts:
```json
{
  "kind": "shell",
  "command": "./sensors/read_temp.sh",
  "args": ["--format", "json"]  // Optional
}
```

### Agent Message
Send message to an agent for processing:
```json
{
  "kind": "agent",
  "agentId": "iot-agent",
  "message": "Analyze sensor data from /tmp/sensors/*.json and report anomalies"
}
```

### MQTT Publish
Publish to MQTT topic:
```json
{
  "kind": "mqtt",
  "topic": "iot/sync",
  "payload": {
    "device": "pi-001",
    "type": "full-sync"
  }
}
```

### HTTP Request
Make HTTP calls:
```json
{
  "kind": "http",
  "method": "POST",
  "url": "https://api.example.com/reports",
  "headers": {
    "Authorization": "Bearer YOUR_TOKEN"
  },
  "payload": {
    "device": "pi-001",
    "type": "daily-summary"
  }
}
```

## CLI Commands

### List Jobs
```bash
evoclaw schedule list
```

Output:
```
ID              NAME                    SCHEDULE        ACTION          ENABLED  RUNS  ERRORS
--              ----                    --------        ------          -------  ----  ------
sensor-read     Read Temperature        Every 5m        Shell: read...  yes      -     -
hourly-report   Hourly Analysis         Every 1h        Agent: iot...   yes      -     -
cloud-sync      Cloud Sync              Cron: 0 */6...  MQTT: iot/sync  yes      -     -
```

### Add Job
Create job config file (`sensor-job.json`):
```json
{
  "id": "new-sensor",
  "name": "New Sensor Job",
  "enabled": true,
  "schedule": {
    "kind": "interval",
    "intervalMs": 600000
  },
  "action": {
    "kind": "shell",
    "command": "./sensors/read_new_sensor.sh"
  }
}
```

Add to config:
```bash
evoclaw schedule add --config sensor-job.json
```

### Remove Job
```bash
evoclaw schedule remove sensor-read
```

### Check Job Status
```bash
evoclaw schedule status sensor-read
```

Output:
```
Job: Read Temperature Sensor
ID: sensor-read
Enabled: true
Schedule: Every 5m
Action: Shell: ./sensors/read_temp.sh

Note: Runtime stats require API access
      Use API endpoint /api/scheduler/jobs/sensor-read for live status
```

## IoT Use Case Example

**Raspberry Pi weather station:**

```json
{
  "scheduler": {
    "enabled": true,
    "jobs": [
      {
        "id": "read-sensors",
        "name": "Read All Sensors",
        "enabled": true,
        "schedule": {"kind": "interval", "intervalMs": 300000},
        "action": {
          "kind": "shell",
          "command": "/home/pi/weather/read_all.sh"
        }
      },
      {
        "id": "analyze-data",
        "name": "Hourly Weather Analysis",
        "enabled": true,
        "schedule": {"kind": "interval", "intervalMs": 3600000},
        "action": {
          "kind": "agent",
          "agentId": "weather-agent",
          "message": "Read sensor data from /tmp/weather/*.json. Calculate hourly averages. Detect anomalies (sudden temp drops >5°C, pressure changes >5hPa). Generate human-readable summary."
        }
      },
      {
        "id": "cloud-backup",
        "name": "Cloud Backup",
        "enabled": true,
        "schedule": {
          "kind": "cron",
          "expr": "0 3 * * *",
          "timezone": "America/Chicago"
        },
        "action": {
          "kind": "http",
          "method": "POST",
          "url": "https://weather-api.example.com/upload",
          "headers": {"Authorization": "Bearer SECRET"},
          "payload": {"station": "home-001"}
        }
      }
    ]
  },
  "agents": [
    {
      "id": "weather-agent",
      "name": "Weather Analysis Agent",
      "systemPrompt": "You are a weather data analyst. Analyze sensor readings, detect patterns and anomalies, and provide clear summaries for non-technical users.",
      "model": "ollama/llama3.3:8b",
      "enabled": true
    }
  ]
}
```

**Workflow:**
1. Every 5 min: Shell script reads sensors → writes JSON
2. Every hour: Agent reads JSON → analyzes → detects anomalies
3. Daily 3 AM: HTTP POST sends backup to cloud

## API Endpoints

*(To be implemented)*

- `GET /api/scheduler/status` - Scheduler stats
- `GET /api/scheduler/jobs` - List all jobs
- `GET /api/scheduler/jobs/:id` - Job detail with runtime stats
- `POST /api/scheduler/jobs/:id/run` - Trigger job immediately
- `PATCH /api/scheduler/jobs/:id` - Update job (enable/disable/modify)

## Implementation Details

### Architecture
```
Scheduler (coordinator)
  ├── Job 1 → JobRunner (goroutine with ticker)
  ├── Job 2 → JobRunner
  └── Job 3 → JobRunner
```

Each job runs in its own goroutine with:
- Dedicated ticker for schedule
- Error tracking
- Duration measurement
- State persistence

### Integration with Orchestrator
The scheduler is part of the orchestrator lifecycle:
```go
orchestrator.Start()
  → scheduler.Start(ctx)
    → for each job: runner.Start(ctx)

orchestrator.Stop()
  → scheduler.Stop()
    → stop all runners
```

### Executor Interface
Orchestrator implements `scheduler.Executor`:
```go
type Executor interface {
    ExecuteAgent(ctx context.Context, agentID, message string) error
    PublishMQTT(ctx context.Context, topic string, payload map[string]any) error
}
```

Shell and HTTP actions execute directly without orchestrator.

## Comparison: OpenClaw vs EvoClaw

| Feature | OpenClaw Cron | EvoClaw Scheduler |
|---------|---------------|-------------------|
| Config | Gateway DB | evoclaw.json |
| Session support | isolated/main | N/A (direct) |
| Delivery modes | announce/none | N/A |
| Edge deployment | Requires Gateway | Native |
| Shell commands | via agentTurn | Native |
| Agent messages | ✅ | ✅ |
| MQTT publish | ❌ | ✅ |
| HTTP requests | ❌ | ✅ |
| Restart required | No | Yes |

## Migration from OpenClaw Cron

OpenClaw memory consolidation cron job:
```json
{
  "name": "Memory Consolidation (Quick)",
  "schedule": {"kind": "every", "everyMs": 14400000},
  "sessionTarget": "isolated",
  "payload": {
    "kind": "agentTurn",
    "message": "Run quick memory consolidation...",
    "model": "anthropic-proxy-4/glm-4.7"
  }
}
```

EvoClaw equivalent:
```json
{
  "id": "memory-consolidation",
  "name": "Memory Consolidation (Quick)",
  "enabled": true,
  "schedule": {"kind": "interval", "intervalMs": 14400000},
  "action": {
    "kind": "shell",
    "command": "evoclaw memory consolidate --mode quick"
  }
}
```

**Key differences:**
- No `sessionTarget` - runs directly
- No `payload.kind` - action type is explicit
- Uses EvoClaw CLI instead of agent message

## Best Practices

### Error Handling
Jobs track errors automatically. For critical tasks:
```json
{
  "action": {
    "kind": "shell",
    "command": "./critical_task.sh || ./fallback.sh"
  }
}
```

### Resource Management
Avoid overlapping intervals:
```json
// Bad: Job takes 10 min, runs every 5 min
{"intervalMs": 300000}  // Will queue up

// Good: Allow buffer time
{"intervalMs": 900000}  // 15 min interval for 10 min task
```

### Timezone Awareness
Always specify timezone for `at` and `cron` schedules:
```json
{
  "kind": "at",
  "time": "09:00",
  "timezone": "America/New_York"  // Explicit, not local
}
```

### Testing Jobs
Test shell commands manually before scheduling:
```bash
# Test command
./sensors/read_temp.sh

# Add to scheduler
evoclaw schedule add --config sensor-job.json

# Trigger immediately to verify
evoclaw schedule run sensor-read

# Check logs
evoclaw logs | grep scheduler
```

## Troubleshooting

### Job Not Running
1. Check enabled status: `evoclaw schedule list`
2. Verify scheduler enabled in config: `"scheduler": {"enabled": true}`
3. Check logs: Look for "scheduler" component messages
4. Verify next run time calculation (cron expression valid?)

### Shell Command Fails
1. Test command manually: `./your_script.sh`
2. Check permissions: `chmod +x ./your_script.sh`
3. Use absolute paths: `/home/pi/sensors/read.sh` not `./read.sh`
4. Check working directory context

### Agent Message Not Received
1. Verify agent ID exists: Check `agents` in config
2. Ensure agent is running: Check orchestrator logs
3. Check agent inbox isn't blocked (unlikely but possible)

## Future Enhancements

- [ ] Web UI for job management
- [ ] Job dependencies (run B after A completes)
- [ ] Retry policies for failed jobs
- [ ] Job execution history persistence
- [ ] Real-time job logs via WebSocket
- [ ] Job templates library
- [ ] Conditional execution (run only if X)

---

**See also:**
- [Examples](../examples/scheduler-iot.json) - Complete IoT config
- [Memory CLI](./TIERED-MEMORY.md) - Integration with memory consolidation
- [API Documentation](./API.md) - Scheduler API endpoints
