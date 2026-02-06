# EvoClaw Integration Tests

End-to-end integration tests that verify the Go orchestrator and Rust edge agent communicate correctly over MQTT.

## Prerequisites

- **Eclipse Mosquitto** MQTT broker running on `localhost:1883`
- **Go** 1.23+ for building the orchestrator
- **Rust** (stable) for building the edge agent

## Quick Start

```bash
# Start MQTT broker (Docker)
docker run -d --name mosquitto -p 1883:1883 eclipse-mosquitto:2 \
  mosquitto -c /mosquitto-no-auth.conf

# Run integration tests
cd integration
go test -v -tags=integration -timeout=60s ./...
```

## Test Scenarios

1. **Ping/Pong** — Orchestrator sends `ping` command, agent replies with `pong`
2. **Heartbeat** — Agent sends periodic status heartbeats, orchestrator receives them
3. **Strategy Update** — Orchestrator pushes strategy update, agent applies and confirms
4. **Metrics Reporting** — Agent reports metrics, orchestrator processes them
5. **Broadcast** — Orchestrator broadcasts to all agents, agent receives

## Architecture

```
┌───────────────────┐       MQTT       ┌──────────────────┐
│  Test Harness     │◄────────────────►│ Eclipse Mosquitto│
│  (Go test code)   │                  │  (localhost:1883)│
│                   │                  └──────────────────┘
│  Simulates both:  │
│  - Orchestrator   │
│    (publish cmds) │
│  - Agent verifier │
│    (check reports)│
└───────────────────┘
```

The integration tests use Go MQTT clients to simulate both sides of the protocol,
verifying message formats, topic routing, and payload serialization are compatible
between the Go orchestrator and Rust agent codebases.

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MQTT_BROKER` | `localhost` | MQTT broker host |
| `MQTT_PORT` | `1883` | MQTT broker port |
| `INTEGRATION_TIMEOUT` | `30s` | Test timeout |
