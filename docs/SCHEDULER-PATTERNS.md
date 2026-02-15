# Scheduler Integration Patterns

**Common patterns for integrating periodic tasks into EvoClaw.**

---

## Pattern 1: Memory Consolidation

**Problem:** Memory system needs periodic cleanup (warm eviction, tree pruning, cold cleanup).

**Old Approach (Hardcoded Tickers):**
```go
// internal/memory/consolidator.go
ticker := time.NewTicker(1 * time.Hour)
defer ticker.Stop()

for {
    select {
    case <-ticker.C:
        c.doWarmEviction(ctx)
    }
}
```

**New Approach (Scheduler):**
```json
{
  "scheduler": {
    "enabled": true,
    "jobs": [
      {
        "id": "memory-warm-eviction",
        "schedule": {"kind": "interval", "intervalMs": 3600000},
        "action": {
          "kind": "shell",
          "command": "evoclaw memory consolidate --mode quick"
        }
      },
      {
        "id": "memory-tree-prune",
        "schedule": {"kind": "cron", "expr": "0 2 * * *"},
        "action": {
          "kind": "shell",
          "command": "evoclaw memory consolidate --mode daily"
        }
      },
      {
        "id": "memory-full-consolidation",
        "schedule": {"kind": "cron", "expr": "0 3 1 * *"},
        "action": {
          "kind": "shell",
          "command": "evoclaw memory consolidate --mode monthly"
        }
      }
    ]
  }
}
```

**Benefits:**
- ✅ User-configurable intervals
- ✅ No code changes to adjust timing
- ✅ Observable via API (`GET /api/scheduler/jobs/memory-warm-eviction`)
- ✅ Can disable/enable at runtime

---

## Pattern 2: IoT Sensor Reading

**Use Case:** Raspberry Pi reads temperature sensor every 5 minutes, analyzes hourly.

**Configuration:**
```json
{
  "scheduler": {
    "enabled": true,
    "jobs": [
      {
        "id": "temp-sensor-read",
        "name": "Read Temperature Sensor",
        "enabled": true,
        "schedule": {
          "kind": "interval",
          "intervalMs": 300000
        },
        "action": {
          "kind": "shell",
          "command": "/home/pi/sensors/read_temp.sh > /tmp/temp.json"
        }
      },
      {
        "id": "temp-analysis",
        "name": "Analyze Temperature Data",
        "enabled": true,
        "schedule": {
          "kind": "interval",
          "intervalMs": 3600000
        },
        "action": {
          "kind": "agent",
          "agentId": "iot-agent",
          "message": "Read temperature data from /tmp/temp.json. Calculate hourly average, min, max. Alert if temperature exceeds 30°C or drops below 10°C. Format output as JSON."
        }
      }
    ]
  },
  "agents": [
    {
      "id": "iot-agent",
      "name": "IoT Analysis Agent",
      "systemPrompt": "You are an IoT sensor analysis agent. Analyze temperature data and provide alerts.",
      "model": "ollama/llama3.3:8b",
      "enabled": true
    }
  ]
}
```

**Workflow:**
1. Every 5 min: Shell script reads sensor → writes `/tmp/temp.json`
2. Every hour: Agent reads JSON → analyzes trends → sends alert if needed

---

## Pattern 3: Cloud Sync

**Use Case:** Edge device syncs data to cloud every 6 hours via MQTT.

**Configuration:**
```json
{
  "mqtt": {
    "port": 1883,
    "host": "0.0.0.0"
  },
  "scheduler": {
    "enabled": true,
    "jobs": [
      {
        "id": "cloud-sync",
        "name": "Cloud Data Sync",
        "enabled": true,
        "schedule": {
          "kind": "cron",
          "expr": "0 */6 * * *",
          "timezone": "UTC"
        },
        "action": {
          "kind": "mqtt",
          "topic": "iot/device-001/sync",
          "payload": {
            "device": "device-001",
            "type": "full-sync",
            "timestamp": "{{NOW}}"
          }
        }
      }
    ]
  }
}
```

**MQTT Message Published:**
```json
{
  "device": "device-001",
  "type": "full-sync",
  "timestamp": "2026-02-16T09:00:00Z"
}
```

Cloud listener subscribes to `iot/+/sync` and triggers backup.

---

## Pattern 4: Daily Reports

**Use Case:** Send daily summary report to API endpoint every morning.

**Configuration:**
```json
{
  "scheduler": {
    "enabled": true,
    "jobs": [
      {
        "id": "daily-report",
        "name": "Daily Summary Report",
        "enabled": true,
        "schedule": {
          "kind": "at",
          "time": "09:00",
          "timezone": "America/New_York"
        },
        "action": {
          "kind": "http",
          "method": "POST",
          "url": "https://api.example.com/reports/daily",
          "headers": {
            "Authorization": "Bearer YOUR_API_TOKEN",
            "Content-Type": "application/json"
          },
          "payload": {
            "device": "iot-001",
            "report_type": "daily-summary",
            "date": "{{TODAY}}"
          }
        }
      }
    ]
  }
}
```

**HTTP POST sent at 9 AM EST daily:**
```http
POST /reports/daily HTTP/1.1
Host: api.example.com
Authorization: Bearer YOUR_API_TOKEN
Content-Type: application/json

{
  "device": "iot-001",
  "report_type": "daily-summary",
  "date": "2026-02-16"
}
```

---

## Pattern 5: Health Checks

**Use Case:** Monitor system health every 15 minutes, alert if issues detected.

**Configuration:**
```json
{
  "scheduler": {
    "enabled": true,
    "jobs": [
      {
        "id": "health-check",
        "name": "System Health Check",
        "enabled": true,
        "schedule": {
          "kind": "interval",
          "intervalMs": 900000
        },
        "action": {
          "kind": "agent",
          "agentId": "health-monitor",
          "message": "Run system health checks: 1) Check disk space (df -h), 2) Check memory usage (free -h), 3) Check running processes (ps aux | grep evoclaw), 4) Check network connectivity (ping -c 1 8.8.8.8). Alert if: disk >90%, memory >90%, evoclaw not running, or network down."
        }
      }
    ]
  },
  "agents": [
    {
      "id": "health-monitor",
      "name": "Health Monitoring Agent",
      "systemPrompt": "You are a system health monitoring agent. Run diagnostics and alert on issues.",
      "model": "ollama/llama3.3:8b",
      "enabled": true
    }
  ]
}
```

---

## Pattern 6: Backup Rotation

**Use Case:** Create backups daily, rotate old backups weekly.

**Configuration:**
```json
{
  "scheduler": {
    "enabled": true,
    "jobs": [
      {
        "id": "daily-backup",
        "name": "Daily Database Backup",
        "enabled": true,
        "schedule": {
          "kind": "cron",
          "expr": "0 2 * * *",
          "timezone": "UTC"
        },
        "action": {
          "kind": "shell",
          "command": "/usr/local/bin/backup-db.sh daily"
        }
      },
      {
        "id": "weekly-backup-rotation",
        "name": "Weekly Backup Rotation",
        "enabled": true,
        "schedule": {
          "kind": "cron",
          "expr": "0 3 * * 0",
          "timezone": "UTC"
        },
        "action": {
          "kind": "shell",
          "command": "/usr/local/bin/rotate-backups.sh --keep 4"
        }
      }
    ]
  }
}
```

**Scripts:**
```bash
# /usr/local/bin/backup-db.sh
#!/bin/bash
DATE=$(date +%Y-%m-%d)
sqlite3 ~/.evoclaw/data.db ".backup /backups/db-$DATE.sqlite3"

# /usr/local/bin/rotate-backups.sh
#!/bin/bash
find /backups -name "db-*.sqlite3" -mtime +28 -delete  # Delete >4 weeks old
```

---

## Pattern 7: Heartbeat / Ping

**Use Case:** Send heartbeat ping to monitoring service every minute.

**Configuration:**
```json
{
  "scheduler": {
    "enabled": true,
    "jobs": [
      {
        "id": "heartbeat-ping",
        "name": "Heartbeat Ping",
        "enabled": true,
        "schedule": {
          "kind": "interval",
          "intervalMs": 60000
        },
        "action": {
          "kind": "http",
          "method": "POST",
          "url": "https://monitor.example.com/heartbeat",
          "headers": {
            "Authorization": "Bearer HEARTBEAT_TOKEN"
          },
          "payload": {
            "device_id": "iot-001",
            "status": "alive",
            "timestamp": "{{NOW}}"
          }
        }
      }
    ]
  }
}
```

Monitor service expects ping every minute. If missing >2 minutes, triggers alert.

---

## Pattern 8: Model Performance Tracking

**Use Case:** Log model performance metrics hourly for evolution tracking.

**Configuration:**
```json
{
  "scheduler": {
    "enabled": true,
    "jobs": [
      {
        "id": "model-perf-log",
        "name": "Model Performance Logging",
        "enabled": true,
        "schedule": {
          "kind": "interval",
          "intervalMs": 3600000
        },
        "action": {
          "kind": "agent",
          "agentId": "perf-tracker",
          "message": "Query model performance from /api/costs endpoint. Calculate: total_cost_usd, tokens_used, avg_response_time_ms. Write summary to /data/perf-log.jsonl in append mode."
        }
      }
    ]
  }
}
```

Creates timestamped log entries for evolution engine to analyze.

---

## Best Practices

### 1. Avoid Overlapping Executions
If a job takes 10 minutes, don't schedule it every 5 minutes:
```json
// Bad
{"intervalMs": 300000}  // 5 min interval, 10 min execution → queue builds up

// Good
{"intervalMs": 900000}  // 15 min interval, safe buffer
```

### 2. Use Appropriate Action Types
- **shell**: Simple scripts, file operations, system commands
- **agent**: AI analysis, decision-making, natural language processing
- **mqtt**: Event publication, message bus integration
- **http**: API calls, webhook delivery, external service integration

### 3. Handle Failures Gracefully
Shell commands should handle errors:
```bash
#!/bin/bash
set -euo pipefail  # Exit on error

# Your task
if ! ./critical-task.sh; then
    echo "Task failed, running fallback"
    ./fallback-task.sh
fi
```

### 4. Log Job Outputs
Redirect output for debugging:
```json
{
  "action": {
    "kind": "shell",
    "command": "./task.sh >> /var/log/evoclaw/task.log 2>&1"
  }
}
```

### 5. Use Timezone-Aware Schedules
Always specify timezone for `at` and `cron`:
```json
{
  "schedule": {
    "kind": "at",
    "time": "09:00",
    "timezone": "America/New_York"  // Not system local time
  }
}
```

### 6. Monitor Job State
Use API to check job status:
```bash
# Check if job is failing
curl http://localhost:8420/api/scheduler/jobs/sensor-read | jq '.state.errorCount'
```

### 7. Test Jobs Manually First
Before scheduling:
```bash
# Test shell command
./sensors/read_temp.sh

# Test agent message
evoclaw chat "Test message for agent"

# Test HTTP endpoint
curl -X POST https://api.example.com/test
```

---

## Migration from Hardcoded Tickers

**Before (Go code):**
```go
ticker := time.NewTicker(1 * time.Hour)
defer ticker.Stop()

for {
    select {
    case <-ticker.C:
        doTask()
    }
}
```

**After (Config):**
```json
{
  "scheduler": {
    "jobs": [
      {
        "id": "task",
        "schedule": {"kind": "interval", "intervalMs": 3600000},
        "action": {"kind": "shell", "command": "evoclaw task"}
      }
    ]
  }
}
```

**Benefits:**
- User-configurable without code changes
- Observable via API
- Can disable/enable at runtime
- Centralized job management

---

**See Also:**
- [Scheduler Documentation](./SCHEDULER.md) - Complete scheduler guide
- [API Reference](./API.md) - Full API documentation
- [Examples](../examples/) - Production configs
