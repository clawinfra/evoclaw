# EvoClaw Edge Agent 🧬

**Lightweight, self-evolving agent runtime for edge devices**

Built in Rust for maximum performance and minimal resource usage. Designed to run on resource-constrained devices while maintaining full trading capabilities.

## 📊 Architecture

```
┌─────────────────────────────────────────────────┐
│              Edge Agent (Rust)                  │
├─────────────────────────────────────────────────┤
│                                                 │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐     │
│  │ Trading  │  │ Monitor  │  │ Strategy │     │
│  │ Client   │  │  Engine  │  │  Engine  │     │
│  └──────────┘  └──────────┘  └──────────┘     │
│       │              │              │          │
│       └──────────────┴──────────────┘          │
│                      │                         │
│              ┌───────▼────────┐                │
│              │ Evolution      │                │
│              │ Tracker        │                │
│              └───────┬────────┘                │
│                      │                         │
│              ┌───────▼────────┐                │
│              │ MQTT Client    │                │
│              └───────┬────────┘                │
└──────────────────────┼──────────────────────────┘
                       │
                       ▼
              ┌────────────────┐
              │  Orchestrator  │
              │     (Go)       │
              └────────────────┘
```

## 🚀 Features

### ✅ Phase 1: Modular Architecture
- **Clean separation of concerns**: Each module has a single responsibility
- **Config-driven**: TOML configuration files for easy deployment
- **Metrics collection**: Track uptime, actions, memory usage
- **MQTT communication**: Reliable pub/sub messaging with orchestrator

### ✅ Phase 2: Trading Agent
- **Hyperliquid integration**: Full REST API client
- **EIP-712 signing**: Python helper for complex cryptographic operations
- **Order management**: Limit orders, stop-loss, take-profit
- **Position monitoring**: Real-time P&L tracking
- **Risk management**: Configurable position sizes and leverage limits

### ✅ Phase 3: Monitor Agent
- **Price alerts**: Configurable threshold-based alerts
- **Funding rate monitoring**: Detect extreme funding conditions
- **Price movement detection**: Track significant market moves
- **MQTT alerts**: Report all events to orchestrator

### ✅ Phase 4: Strategy Engine
- **Pluggable strategies**: Easy to add new strategies
- **FundingArbitrage**: Capture funding rate opportunities
- **MeanReversion**: Trade support/resistance levels
- **Parameterized**: All strategies are tunable via MQTT
- **Hot-swappable**: Update parameters without restart

### ✅ Phase 5: Self-Evolution Support
- **Performance tracking**: Win rate, Sharpe ratio, drawdown
- **Fitness scoring**: Multi-factor weighted fitness function
- **Trade history**: Full audit trail of all trades
- **Hot-swap capable**: Reset and update strategies on-the-fly

## 📦 Build & Deploy

### Prerequisites
```bash
# Install Rust (if not already installed)
curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh
source $HOME/.cargo/env

# Build dependencies (Ubuntu/Debian)
sudo apt-get install build-essential
```

### Build
```bash
# Development build
cargo build

# Release build (optimized for size and speed)
cargo build --release

# Binary location
./target/release/evoclaw-agent
```

**Binary size**: 3.2MB (optimized for edge deployment)

### Run
```bash
# Using CLI arguments
./evoclaw-agent \
  --id agent-001 \
  --agent-type trader \
  --broker mqtt.example.com \
  --port 1883 \
  --orchestrator http://orchestrator:8420

# Using config file
./evoclaw-agent --config agent.toml
```

### Example Configuration

```toml
agent_id = "trader-001"
agent_type = "trader"

[mqtt]
broker = "mqtt.example.com"
port = 1883
keep_alive_secs = 30

[orchestrator]
url = "http://orchestrator:8420"

[trading]
hyperliquid_api = "https://api.hyperliquid.xyz"
wallet_address = "0x..."
private_key_path = "keys/private.key"
max_position_size_usd = 1000.0
max_leverage = 3.0

[monitor]
price_alert_threshold_pct = 5.0
funding_rate_threshold_pct = 0.1
check_interval_secs = 60
```

## 🎯 Command Reference

All commands are sent via MQTT to `evoclaw/agents/{agent_id}/commands`

### Trading Commands
```json
// Get market prices
{
  "command": "execute",
  "payload": {
    "action": "get_prices"
  },
  "request_id": "req-001"
}

// Get current positions
{
  "command": "execute",
  "payload": {
    "action": "get_positions"
  },
  "request_id": "req-002"
}

// Place limit order
{
  "command": "execute",
  "payload": {
    "action": "place_order",
    "asset": 0,
    "is_buy": true,
    "price": 50000.0,
    "size": 0.1
  },
  "request_id": "req-003"
}

// Monitor positions
{
  "command": "execute",
  "payload": {
    "action": "monitor_positions"
  },
  "request_id": "req-004"
}
```

### Monitor Commands
```json
// Add price alert
{
  "command": "execute",
  "payload": {
    "action": "add_price_alert",
    "coin": "BTC",
    "target_price": 60000.0,
    "alert_type": "above"
  },
  "request_id": "req-005"
}

// Get monitor status
{
  "command": "execute",
  "payload": {
    "action": "status"
  },
  "request_id": "req-006"
}
```

### Strategy Commands
```json
// Add FundingArbitrage strategy
{
  "command": "update_strategy",
  "payload": {
    "action": "add_funding_arbitrage",
    "funding_threshold": -0.1,
    "exit_funding": 0.0,
    "position_size_usd": 1000.0
  },
  "request_id": "req-007"
}

// Add MeanReversion strategy
{
  "command": "update_strategy",
  "payload": {
    "action": "add_mean_reversion",
    "support_level": 2.0,
    "resistance_level": 2.0,
    "position_size_usd": 1000.0
  },
  "request_id": "req-008"
}

// Update strategy parameters
{
  "command": "update_strategy",
  "payload": {
    "action": "update_params",
    "strategy": "FundingArbitrage",
    "params": {
      "funding_threshold": -0.15,
      "position_size_usd": 2000.0
    }
  },
  "request_id": "req-009"
}

// Get all strategy parameters
{
  "command": "update_strategy",
  "payload": {
    "action": "get_params"
  },
  "request_id": "req-010"
}
```

### Evolution Commands
```json
// Record a trade for evolution tracking
{
  "command": "evolution",
  "payload": {
    "action": "record_trade",
    "asset": "BTC",
    "entry_price": 50000.0,
    "exit_price": 51000.0,
    "size": 0.1
  },
  "request_id": "req-011"
}

// Get performance metrics
{
  "command": "evolution",
  "payload": {
    "action": "get_performance"
  },
  "request_id": "req-012"
}

// Get trade history
{
  "command": "evolution",
  "payload": {
    "action": "get_trade_history"
  },
  "request_id": "req-013"
}
```

### General Commands
```json
// Ping (health check)
{
  "command": "ping",
  "payload": {},
  "request_id": "req-014"
}

// Get metrics (includes evolution fitness)
{
  "command": "get_metrics",
  "payload": {},
  "request_id": "req-015"
}

// Shutdown
{
  "command": "shutdown",
  "payload": {},
  "request_id": "req-016"
}
```

## 📈 Evolution Fitness Function

The fitness score is a weighted combination of:

- **40%** Sharpe ratio (risk-adjusted returns)
- **30%** Win rate
- **20%** Total P&L (normalized)
- **10%** Drawdown penalty

```
fitness = (sharpe_score * 0.4) + 
          (win_rate_score * 0.3) + 
          (pnl_score * 0.2) + 
          (drawdown_penalty * 0.1)
```

Score range: 0-100

## 🔧 Development

### Code Quality
```bash
# Format code
cargo fmt

# Run clippy (strict mode)
cargo clippy -- -D warnings

# Run tests
cargo test
```

### Module Structure
```
src/
├── main.rs           # Entry point & CLI parsing
├── agent.rs          # EdgeAgent core logic
├── config.rs         # Configuration structs
├── metrics.rs        # Metrics collection
├── mqtt.rs           # MQTT communication layer
├── commands.rs       # Command handler dispatch
├── trading.rs        # Hyperliquid trading client
├── monitor.rs        # Price & funding alerts
├── strategy.rs       # Strategy engine & strategies
└── evolution.rs      # Performance tracking & fitness

scripts/
└── hl_sign.py        # EIP-712 signing helper
```

## 🛡️ Safety & Error Handling

- **No `unwrap()` in production code**: All errors are properly handled with `Result<?>`
- **Graceful degradation**: Agent continues running even if individual commands fail
- **Automatic reconnection**: MQTT client reconnects on network failures
- **Resource limits**: Configurable position sizes and leverage caps
- **Structured logging**: All events are logged with `tracing`

## 🚢 Production Deployment

### Systemd Service
```ini
[Unit]
Description=EvoClaw Edge Agent
After=network.target

[Service]
Type=simple
User=evoclaw
WorkingDirectory=/opt/evoclaw/edge-agent
ExecStart=/opt/evoclaw/edge-agent/evoclaw-agent --config /etc/evoclaw/agent.toml
Restart=always
RestartSec=10

[Install]
WantedBy=multi-user.target
```

### Docker
```dockerfile
FROM rust:1.93-slim as builder
WORKDIR /app
COPY . .
RUN cargo build --release

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y python3 python3-pip && rm -rf /var/lib/apt/lists/*
RUN pip3 install eth-account --break-system-packages
COPY --from=builder /app/target/release/evoclaw-agent /usr/local/bin/
COPY --from=builder /app/scripts /opt/evoclaw/scripts
CMD ["evoclaw-agent"]
```

## 📊 Performance

- **Binary size**: 3.2MB (stripped, LTO enabled)
- **Memory usage**: ~5-10MB runtime
- **Startup time**: <100ms
- **MQTT latency**: <10ms (local network)
- **API latency**: Depends on Hyperliquid response time

## 🔮 Future Enhancements

- [ ] WebSocket support for real-time market data
- [ ] More sophisticated strategies (ML-based, multi-timeframe)
- [ ] Backtesting framework
- [ ] Portfolio-level risk management
- [ ] Cross-exchange arbitrage
- [ ] Advanced order types (trailing stops, OCO)

## 🤝 Integration with Orchestrator

The Go orchestrator manages:
- Agent deployment and lifecycle
- Evolution engine (genetic algorithms)
- Multi-agent coordination
- Strategy parameter tuning
- Performance evaluation
- Fitness-based selection

Agents report back:
- Heartbeats (every 30s)
- Trade execution results
- Performance metrics
- Fitness scores

## 📝 License

MIT

---

**Built by Alex Chen for EvoClaw** 🧬  
*Ship fast, stay safe.*
