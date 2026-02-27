# Cloud Deployment Guide — E2B Sandboxes

Deploy EvoClaw agents as Firecracker microVM sandboxes on E2B for instant scaling, zero-ops cloud execution, and multi-tenant isolation.

## Why E2B for EvoClaw?

[E2B](https://e2b.dev) provides Firecracker-based microVM sandboxes designed for AI agent workloads:

| Feature | Benefit for EvoClaw |
|---------|-------------------|
| ~100ms cold start | Spawn agents instantly for burst evolution |
| Firecracker isolation | Tenant-safe: each user's agent runs in a separate microVM |
| Dockerfile templates | Reuse our existing Rust agent binary |
| REST API | Native Go integration, no SDK dependency |
| $100 free credits | ~1,000 hours of agent runtime to start |
| Auto-termination | No runaway costs — sandboxes die after timeout |

## Prerequisites

1. **E2B account** — Sign up at [e2b.dev](https://e2b.dev) ($100 free credits)
2. **E2B CLI** — `npm i -g @e2b/cli`
3. **E2B API key** — From your E2B dashboard
4. **Compiled edge agent** — `cd edge-agent && cargo build --release`

## Quick Start

### 1. Set your E2B API key

```bash
export E2B_API_KEY="e2b_your_key_here"
```

Or add to `evoclaw.json`:

```json
{
  "cloud": {
    "enabled": true,
    "e2bApiKey": "e2b_your_key_here",
    "defaultTemplate": "evoclaw-agent",
    "maxAgents": 10,
    "creditBudgetUsd": 50.0
  }
}
```

### 2. Build the sandbox template

```bash
cd deploy/e2b

# Build the edge agent first
cd ../../edge-agent
cargo build --release --target x86_64-unknown-linux-gnu
cd ../deploy/e2b

# Build E2B template
e2b template build --name evoclaw-agent --dockerfile e2b.Dockerfile
```

### 3. Spawn your first cloud agent

**Via CLI:**
```bash
evoclaw cloud spawn --template evoclaw-agent --config agent.toml
```

**Via API:**
```bash
curl -X POST http://localhost:8420/api/cloud/spawn \
  -H "Content-Type: application/json" \
  -d '{
    "template_id": "evoclaw-agent",
    "agent_id": "my-first-cloud-agent",
    "agent_type": "trader",
    "mqtt_broker": "broker.evoclaw.io",
    "mqtt_port": 1883,
    "timeout_sec": 600
  }'
```

### 4. Check your agents

```bash
# List running cloud agents
evoclaw cloud list

# Via API
curl http://localhost:8420/api/cloud
```

### 5. Monitor costs

```bash
evoclaw cloud costs

# Via API
curl http://localhost:8420/api/cloud/costs
```

## Template Configuration

The E2B sandbox template lives in `deploy/e2b/`:

```
deploy/e2b/
├── e2b.Dockerfile   # Sandbox image definition
├── e2b.toml         # E2B template config
├── agent.toml       # Default agent configuration
└── entrypoint.sh    # Boot script (config injection, MQTT check)
```

### Environment Variables

These can be set at spawn time to configure the agent:

| Variable | Description | Default |
|----------|-------------|---------|
| `EVOCLAW_AGENT_ID` | Unique agent identifier | Auto-generated |
| `EVOCLAW_AGENT_TYPE` | Agent role: trader, monitor, sensor | `trader` |
| `MQTT_BROKER` | MQTT broker hostname | `broker.evoclaw.io` |
| `MQTT_PORT` | MQTT broker port | `1883` |
| `MQTT_USERNAME` | MQTT auth username | (empty) |
| `MQTT_PASSWORD` | MQTT auth password | (empty) |
| `ORCHESTRATOR_URL` | Orchestrator HTTP API | `https://api.evoclaw.io:8420` |
| `HYPERLIQUID_API_KEY` | Trading API key | (empty) |
| `HYPERLIQUID_API_SECRET` | Trading API secret | (empty) |
| `EVOCLAW_PAPER_MODE` | Paper trading mode | `true` |
| `EVOCLAW_LOG_LEVEL` | Log verbosity | `info` |
| `EVOCLAW_GENOME` | JSON strategy parameters | (empty) |

### Custom Templates

Create variants for different agent roles:

```bash
# Trading agent with extra market data tools
e2b template build --name evoclaw-trader --dockerfile e2b-trader.Dockerfile

# Monitor agent with alerting tools
e2b template build --name evoclaw-monitor --dockerfile e2b-monitor.Dockerfile
```

## Scaling Strategies

### On-Demand (Default)

Spin up agents when needed, auto-terminate after idle timeout.

```bash
evoclaw cloud spawn --template evoclaw-agent --timeout 600
```

Best for: Development, testing, ad-hoc analysis.

### Scheduled Mode

Run agents during market hours, sleep otherwise. Configure via the orchestrator's scheduled scaling:

```json
{
  "mode": "scheduled",
  "schedule": {
    "start": "09:00",
    "end": "16:00",
    "timezone": "America/New_York",
    "days": ["mon", "tue", "wed", "thu", "fri"]
  }
}
```

Best for: Trading agents that only need to run during market hours.

### Burst Mode

Spawn N agents simultaneously for tournament-style evolution:

```bash
# Via API
curl -X POST http://localhost:8420/api/saas/agents \
  -H "X-API-Key: evo_your_key" \
  -d '{
    "agent_type": "trader",
    "mode": "burst",
    "count": 10,
    "genome": "{\"type\":\"momentum\",\"lookback_periods\":20}"
  }'
```

The orchestrator spawns 10 agents with the same strategy genome, lets them compete, and selects the fittest for the next generation.

Best for: Strategy optimization, parameter sweeping, evolution tournaments.

## Agent-as-a-Service (SaaS Mode)

Enable multi-tenant mode to let users manage their own agents:

### 1. Enable SaaS

```json
{
  "cloud": {
    "enabled": true,
    "e2bApiKey": "e2b_your_key"
  }
}
```

### 2. Register a user

```bash
curl -X POST http://localhost:8420/api/saas/register \
  -d '{"email": "user@example.com", "max_agents": 5}'
```

Response includes the user's API key.

### 3. User spawns agents

```bash
curl -X POST http://localhost:8420/api/saas/agents \
  -H "X-API-Key: evo_user_key_here" \
  -d '{"agent_type": "trader", "mode": "on-demand"}'
```

### 4. User checks usage

```bash
curl http://localhost:8420/api/saas/usage \
  -H "X-API-Key: evo_user_key_here"
```

## Cost Optimization

### E2B Pricing (~$0.0001/sec per sandbox)

| Duration | Cost per agent | Notes |
|----------|---------------|-------|
| 5 min | ~$0.03 | Quick test |
| 1 hour | ~$0.36 | Strategy evaluation |
| 8 hours | ~$2.88 | Full trading day |
| 24 hours | ~$8.64 | Continuous monitoring |

### Tips

1. **Set budget limits** — `creditBudgetUsd` prevents runaway spending
2. **Use timeouts** — Sandboxes auto-terminate, no lingering costs
3. **Burst wisely** — Spawn 10 agents for 5 min = $0.30, not $3.60/day each
4. **Paper mode first** — Test strategies without risking real money
5. **Keep-alive only when needed** — The manager refreshes timeouts automatically; disable for short-lived tasks
6. **Monitor costs** — `evoclaw cloud costs` shows real-time spending

### Budget Controls

```json
{
  "cloud": {
    "creditBudgetUsd": 50.0,
    "maxAgents": 10
  }
}
```

The manager refuses to spawn new agents when:
- Agent count reaches `maxAgents`
- Estimated spend reaches `creditBudgetUsd`
- Individual user exceeds their `creditLimitUsd` (SaaS mode)

## Deployment Comparison

| Feature | Edge (Bare Metal) | Self-Hosted (Podman) | Cloud (E2B) |
|---------|-------------------|---------------------|-------------|
| **Binary** | Raw Rust 3.2MB | Containerized | Sandbox VM |
| **Cold Start** | Instant | ~1s | ~100ms |
| **Isolation** | None (process) | Container | Firecracker microVM |
| **Cost** | Hardware only | Hardware + Docker overhead | ~$0.36/hr per agent |
| **Scaling** | Manual | Docker Compose | API-driven, instant |
| **Multi-tenant** | No | Possible | Built-in isolation |
| **Best For** | Pi, IoT, phones | Your server | SaaS, burst, zero-ops |
| **Setup** | Copy binary + config | `make up` | `e2b template build` |
| **Internet** | Your network | Your network | E2B-managed |
| **Persistence** | Local disk | Docker volumes | Ephemeral (sandbox) |

## Troubleshooting

### Agent won't connect to MQTT

- Check `MQTT_BROKER` and `MQTT_PORT` environment variables
- Ensure your MQTT broker is publicly accessible (cloud agents can't reach `localhost`)
- Check firewall rules for MQTT port

### Sandbox times out too quickly

- Increase `timeout_sec` in spawn config
- The manager's keep-alive loop extends timeouts automatically
- Set `defaultTimeoutSec` in cloud config

### Budget exhausted

- Check spending: `evoclaw cloud costs`
- Increase `creditBudgetUsd` in config
- Kill idle agents: `evoclaw cloud kill <id>`

### Template build fails

- Ensure edge agent is compiled: `cd edge-agent && cargo build --release`
- Check Dockerfile path in `e2b.toml`
- Run `e2b template build --verbose` for detailed output

## Architecture

```
                    ┌─────────────────────┐
                    │   Orchestrator (Go)  │
                    │     :8420 HTTP API   │
                    └──────┬──────────────┘
                           │
              ┌────────────┼────────────┐
              │            │            │
     ┌────────▼─┐   ┌─────▼────┐  ┌───▼──────────┐
     │   MQTT   │   │ Cloud    │  │ SaaS Service │
     │  Broker  │   │ Manager  │  │ (Tenants)    │
     └────┬─────┘   └────┬─────┘  └──────┬───────┘
          │              │               │
     ┌────▼─────┐   ┌────▼─────┐   ┌────▼─────┐
     │ Edge     │   │ E2B API  │   │ E2B API  │
     │ Agent    │   │          │   │ (per user)│
     │ (local)  │   └────┬─────┘   └────┬─────┘
     └──────────┘        │              │
                    ┌────▼──────────────▼────┐
                    │   E2B Firecracker VMs  │
                    │  ┌─────┐  ┌─────┐     │
                    │  │Agent│  │Agent│ ... │
                    │  └─────┘  └─────┘     │
                    └───────────────────────┘
```

## Next Steps

- [Architecture Overview](../architecture/overview.md) — Full system architecture
- [Edge Agent](../architecture/edge-agent.md) — Rust agent internals
- [Trading Agent Guide](trading-agent.md) — Configure a Hyperliquid trader
- [Companion Agent Guide](companion-agent.md) — Build a personal AI companion
