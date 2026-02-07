# Agent Skills Guide

Skills are modular capabilities that extend what an EvoClaw edge agent can do. Each skill is a self-contained plugin that handles commands, collects data on a schedule, and reports back to the orchestrator.

## Overview

The skill system provides:
- **Modular architecture** — add/remove capabilities without changing core agent code
- **Periodic data collection** — skills can tick on a schedule (e.g., collect system metrics every 30s)
- **Command routing** — orchestrator sends commands to specific skills
- **Alert generation** — skills can emit alerts when thresholds are exceeded
- **Graceful lifecycle** — init, tick, handle, shutdown

## Built-in Skills

### System Monitor (`system_monitor`)

Monitors system health on any Linux device, including Raspberry Pi.

**Capabilities:**
| Capability | Description |
|---|---|
| `system.cpu` | CPU usage percentage (from `/proc/stat`) |
| `system.memory` | RAM usage (from `/proc/meminfo`) |
| `system.disk` | Disk usage (via `statvfs`) |
| `system.temperature` | CPU temperature (from `/sys/class/thermal`) |
| `system.uptime` | System uptime (from `/proc/uptime`) |
| `system.load` | Load average (from `/proc/loadavg`) |
| `system.network` | Network bytes in/out (from `/proc/net/dev`) |

**Config:**
```toml
[skills.system_monitor]
enabled = true
tick_interval_secs = 30
```

**Commands:**
- `status` — Get a current metrics snapshot
- `history` — Get last N readings (in-memory ring buffer, last 100)
- `alert_threshold` — Set alert thresholds

**Example command:**
```json
{
  "command": "skill",
  "payload": {
    "skill": "system_monitor",
    "action": "status"
  },
  "request_id": "req-123"
}
```

**Tick report:**
```json
{
  "skill": "system_monitor",
  "report_type": "metric",
  "payload": {
    "cpu_pct": 12.5,
    "memory_used_mb": 112,
    "memory_total_mb": 427,
    "memory_pct": 26.2,
    "disk_used_gb": 2.1,
    "disk_total_gb": 3.1,
    "disk_pct": 67.7,
    "temperature_c": 48.3,
    "uptime_secs": 3847,
    "load_1m": 0.15,
    "load_5m": 0.12,
    "load_15m": 0.17,
    "net_rx_bytes": 1234567,
    "net_tx_bytes": 987654
  }
}
```

**Alert example:**
```json
{
  "skill": "system_monitor",
  "report_type": "alert",
  "payload": {
    "alert": "cpu_high",
    "value": 95.2,
    "threshold": 90.0,
    "message": "CPU usage at 95.2% (threshold: 90%)"
  }
}
```

### GPIO (`gpio`)

Controls Raspberry Pi GPIO pins via the sysfs interface. Falls back to simulation mode on non-Pi devices.

**Capabilities:**
| Capability | Description |
|---|---|
| `gpio.read` | Read pin state (HIGH/LOW) |
| `gpio.write` | Set pin state |
| `gpio.mode` | Set pin mode (input/output) |
| `gpio.pwm` | Software PWM (basic) |
| `gpio.watch` | Watch for pin state changes |

**Config:**
```toml
[skills.gpio]
enabled = true
pins = [17, 27, 22]
```

**Safety:** Only pins listed in the config are accessible. BCM numbering is used.

**Commands:**
```json
{"action": "read", "pin": 17}
{"action": "write", "pin": 17, "value": 1}
{"action": "mode", "pin": 17, "direction": "output"}
{"action": "status"}
{"action": "blink", "pin": 17, "count": 5, "interval_ms": 500}
```

### Price Monitor (`price_monitor`)

Monitors cryptocurrency prices via CoinGecko API.

**Capabilities:**
| Capability | Description |
|---|---|
| `price.check` | Get current prices |
| `price.alert` | Set price alerts |
| `price.history` | Get price history |

**Config:**
```toml
[skills.price_monitor]
enabled = true
symbols = ["BTC", "ETH", "SOL"]
threshold_pct = 5.0
tick_interval_secs = 60
```

**Commands:**
- `check` / `status` — Fetch current prices
- `alert` — Create a price alert: `{"symbol": "BTC", "target_price": 100000, "direction": "above"}`
- `history` — Get recent price readings
- `list_alerts` — List all configured alerts
- `clear_alerts` — Remove all alerts

## Writing a Custom Skill

### 1. Create the skill file

Create `src/skills/my_skill.rs`:

```rust
use async_trait::async_trait;
use serde_json::Value;
use super::{Skill, SkillReport};

pub struct MySkill {
    tick_interval: u64,
}

impl MySkill {
    pub fn new(tick_interval: u64) -> Self {
        Self { tick_interval }
    }
}

#[async_trait]
impl Skill for MySkill {
    fn name(&self) -> &str { "my_skill" }

    fn capabilities(&self) -> Vec<String> {
        vec!["my_skill.feature1".to_string()]
    }

    async fn init(&mut self) -> Result<(), Box<dyn std::error::Error + Send + Sync>> {
        Ok(())
    }

    async fn handle(
        &mut self, command: &str, payload: Value,
    ) -> Result<Value, Box<dyn std::error::Error + Send + Sync>> {
        match command {
            "status" => Ok(serde_json::json!({"status": "ok"})),
            _ => Err(format!("unknown command: {}", command).into()),
        }
    }

    async fn tick(&mut self) -> Option<SkillReport> {
        Some(SkillReport {
            skill: "my_skill".to_string(),
            report_type: "metric".to_string(),
            payload: serde_json::json!({"value": 42}),
        })
    }

    fn tick_interval_secs(&self) -> u64 { self.tick_interval }

    async fn shutdown(&mut self) {}
}
```

### 2. Register in `skills/mod.rs`

Add `pub mod my_skill;` to the module file.

### 3. Wire into agent

In `agent.rs`, add creation logic in the `new()` method under the skills config section.

### 4. Add config type

In `config.rs`, add your `MySkillConfig` struct and include it in `SkillsConfig`.

## Config Reference

```toml
[skills.system_monitor]
enabled = true
tick_interval_secs = 30

[skills.gpio]
enabled = true
pins = [17, 27, 22]

[skills.price_monitor]
enabled = true
symbols = ["BTC", "ETH", "SOL"]
threshold_pct = 5.0
tick_interval_secs = 60
```

## Example: Temperature Alert System

### 1. Configure the agent

```toml
agent_id = "pi-temp-monitor"
agent_type = "monitor"

[mqtt]
broker = "192.168.1.100"
port = 1883

[orchestrator]
url = "http://192.168.1.100:8420"

[skills.system_monitor]
enabled = true
tick_interval_secs = 10
```

### 2. Set alert threshold

```json
{
  "command": "skill",
  "payload": {
    "skill": "system_monitor",
    "action": "alert_threshold",
    "params": { "temperature_c": 60.0 }
  },
  "request_id": "set-temp-threshold"
}
```

### 3. Monitor

The dashboard shows current temperature, alert history, and historical readings in the agent detail view under "Agent Skills".

## Architecture

```
┌──────────────┐     ┌──────────────┐     ┌──────────────────┐
│  Orchestrator │────▶│  Edge Agent   │────▶│  SkillRegistry   │
│   (Go)       │◀────│  (Rust)      │◀────│                  │
└──────────────┘     └──────────────┘     │  ┌─────────────┐ │
      │                                    │  │ SystemMonitor│ │
      │  MQTT commands/reports             │  ├─────────────┤ │
      │                                    │  │ GPIO        │ │
      ▼                                    │  ├─────────────┤ │
┌──────────────┐                           │  │ PriceMonitor│ │
│  Dashboard   │                           │  └─────────────┘ │
│  (Alpine.js) │                           └──────────────────┘
└──────────────┘
```
