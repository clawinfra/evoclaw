# Development Environment Setup

## Prerequisites

| Tool | Version | Purpose |
|------|---------|---------|
| Go | 1.23+ | Orchestrator |
| Rust | 1.75+ | Edge agent |
| Mosquitto | 2.x | MQTT broker (optional) |
| Docker | 24+ | Container builds (optional) |
| jq | 1.6+ | JSON processing (optional) |

## Setup

### Clone

```bash
git clone https://github.com/clawinfra/evoclaw.git
cd evoclaw
```

### Go Orchestrator

```bash
# Verify Go installation
go version  # Should be 1.23+

# Download dependencies
go mod download

# Build
go build -o evoclaw ./cmd/evoclaw

# Run tests
go test ./...

# Run with race detection
go test -race ./...

# Coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

### Rust Edge Agent

```bash
cd edge-agent

# Verify Rust installation
rustc --version  # Should be 1.75+
cargo --version

# Build
cargo build

# Run tests
cargo test

# Clippy (linter)
cargo clippy -- -W clippy::all

# Format
cargo fmt

# Coverage (requires cargo-llvm-cov)
cargo install cargo-llvm-cov
cargo llvm-cov --html
```

### MQTT Broker

```bash
# Install Mosquitto
# macOS
brew install mosquitto

# Ubuntu/Debian
sudo apt install mosquitto mosquitto-clients

# Start
mosquitto -v  # Verbose mode

# Test
mosquitto_pub -t "test" -m "hello"
mosquitto_sub -t "test"
```

## Project Structure

```
evoclaw/
├── cmd/evoclaw/           # Main entry point
│   ├── main.go            # App setup, startup, shutdown
│   ├── main_test.go       # Integration tests
│   └── web/               # Embedded dashboard (copied from web/)
├── internal/
│   ├── api/               # HTTP API server
│   │   ├── server.go      # Routes, middleware, handlers
│   │   ├── dashboard.go   # Dashboard + evolution + SSE endpoints
│   │   └── server_test.go
│   ├── agents/            # Agent management
│   │   ├── registry.go    # Agent CRUD, health checks
│   │   ├── memory.go      # Conversation memory
│   │   └── *_test.go
│   ├── channels/          # Communication adapters
│   │   ├── telegram.go    # Telegram bot
│   │   ├── mqtt.go        # MQTT client
│   │   └── *_test.go
│   ├── config/            # Configuration
│   │   ├── config.go      # Structs, load/save
│   │   └── config_test.go
│   ├── evolution/         # Evolution engine
│   │   ├── engine.go      # Fitness, mutation, revert
│   │   └── engine_test.go
│   ├── models/            # LLM providers
│   │   ├── router.go      # Model selection, fallback
│   │   ├── anthropic.go   # Claude API client
│   │   ├── openai.go      # OpenAI/compatible client
│   │   ├── ollama.go      # Ollama client
│   │   └── *_test.go
│   └── orchestrator/      # Core orchestration
│       ├── orchestrator.go # Message routing, agent coordination
│       └── orchestrator_test.go
├── edge-agent/            # Rust edge agent
│   ├── src/
│   │   ├── main.rs        # Entry point
│   │   ├── lib.rs         # Module declarations
│   │   ├── agent.rs       # Agent core loop
│   │   ├── config.rs      # TOML config parser
│   │   ├── mqtt.rs        # MQTT communication
│   │   ├── trading.rs     # Hyperliquid client
│   │   ├── strategy.rs    # Strategy engine
│   │   ├── evolution.rs   # Local evolution
│   │   ├── monitor.rs     # Market monitoring
│   │   ├── metrics.rs     # Metric collection
│   │   └── commands.rs    # Command handling
│   ├── tests/
│   │   └── integration_test.rs
│   ├── Cargo.toml
│   └── agent.example.toml
├── web/                   # Dashboard source
│   ├── index.html         # SPA entry point
│   ├── style.css          # Dark theme styles
│   └── app.js             # Alpine.js application
├── docs/                  # Documentation
├── docker-compose.yml
└── evoclaw.example.json   # Example config
```

## Development Workflow

### 1. Create a feature branch

```bash
git checkout -b feat/my-feature
```

### 2. Make changes

Edit code, add tests.

### 3. Run tests

```bash
# Go
go test ./...

# Rust
cd edge-agent && cargo test
```

### 4. Update dashboard (if API changed)

```bash
# Edit web/ files
# Copy to embed location
cp web/* cmd/evoclaw/web/

# Rebuild
go build ./cmd/evoclaw/
```

### 5. Commit and push

```bash
git add .
git commit -m "feat: my new feature"
git push origin feat/my-feature
```

### 6. Open PR

Open a Pull Request on GitHub.

## Running Locally

### Full Stack

```bash
# Terminal 1: MQTT broker
mosquitto -v

# Terminal 2: Orchestrator
./evoclaw --config evoclaw.json

# Terminal 3: Edge agent
cd edge-agent
cargo run -- --config agent.toml

# Terminal 4: Watch logs
curl -N http://localhost:8420/api/logs/stream
```

### Docker

```bash
docker compose up -d
docker compose logs -f
```

## IDE Setup

### VS Code

Recommended extensions:
- `golang.go` — Go support
- `rust-lang.rust-analyzer` — Rust support
- `tamasfe.even-better-toml` — TOML support

### GoLand / RustRover

JetBrains IDEs work out of the box with the standard project structure.

## See Also

- [Contributing Guide](CONTRIBUTING.md)
- [Architecture Overview](../architecture/overview.md)
- [Architecture Decisions](architecture-decisions.md)
