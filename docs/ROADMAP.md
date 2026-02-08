# EvoClaw Roadmap

> *Adapts. Evolves. Everywhere.* 🌊

---

## Phase 1: Foundation ✅ (Complete)

**Go Orchestrator + Rust Edge Agent**

- ✅ Go orchestrator (3,549 lines, 6.9MB)
  - Telegram + MQTT channel adapters
  - Model router with fallback chains
  - Agent registry + conversation memory
  - HTTP API (9 endpoints)
  - Evolution engine integration
- ✅ Rust edge agent (2,040 lines, 3.2MB)
  - Hyperliquid trading API client
  - Price/funding monitor with alerts
  - Strategy engine (FundingArbitrage + MeanReversion)
  - Self-evolution with fitness scoring
- ✅ Test coverage: Go 88.3%, Rust 90.45%
- ✅ Philosophy & vision documented

---

## Phase 1b: Terminal & Web Access 🖥️ (In Progress)

**Terminal TUI + Web Terminal + BSC On-Chain**

- ✅ Terminal TUI — 759 lines, Bubble Tea split-pane chat interface
  - Left pane: agent status, metrics, uptime
  - Right pane: conversation with scroll
  - `evoclaw tui` — single command, 7.2MB binary
  - SSH-friendly — access agents from anywhere, no browser needed
- ✅ BSC On-Chain Integration — 1,444 lines
  - `AgentRegistry.sol` — agent registration, action logging, evolution tracking, reputation
  - `internal/onchain/adapter.go` — multi-chain adapter interface with ChainRegistry
  - `internal/onchain/bsc.go` — BSC/opBNB JSON-RPC client (zero go-ethereum dep)
  - Supports BSC mainnet/testnet + opBNB mainnet/testnet
  - Orchestrator auto-logs agent actions on-chain
- 🔜 Web Terminal (ttyd/xterm.js)
  - Browser-based terminal — no SSH client needed
  - Expose TUI via web for demos and remote access
  - Auth layer (Cloudflare Access or app-level)
  - Embed in dashboard as a "Terminal" tab
- 🔜 Contract Deployment
  - Deploy AgentRegistry to BSC testnet
  - Real transaction signing
  - MetaMask/wallet integration in web dashboard

---

## Phase 2: Platform Expansion 📱 (Next)

### 2a. Android App
**Priority: HIGH — 3 billion devices, zero hardware needed**

| Component | Tech | Notes |
|-----------|------|-------|
| UI Layer | Kotlin + Jetpack Compose | Native Android experience |
| Agent Core | Rust via JNI/FFI | Same EvoClaw agent, compiled for Android |
| Voice | Android AudioRecord + MediaPlayer | Mic capture → cloud STT → agent → cloud TTS → speaker |
| Background | Foreground Service | Persistent notification, always-on option |
| Comms | MQTT + HTTP | Connects to orchestrator |

**Milestones:**
1. Cross-compile Rust agent to `aarch64-linux-android` 
2. JNI bridge: Kotlin ↔ Rust FFI
3. Minimal chat UI (text-based agent interaction)
4. Voice pipeline (mic → cloud STT → agent → cloud TTS → speaker)
5. Background service (agent runs when app is closed)
6. Companion mode (phone pairs with nearby EvoClaw hardware)
7. Play Store release

**Target:** Prototype in 4-6 weeks

### 2b. iOS Remote App
**Priority: MEDIUM — dashboard/remote, not full agent**

| Component | Tech | Notes |
|-----------|------|-------|
| UI Layer | SwiftUI | Native iOS experience |
| Agent Core | Cloud or companion device | iOS restrictions prevent persistent background agent |
| Bridge | swift-bridge crate | For any on-device Rust logic |
| Comms | WebSocket + Push Notifications | Server pushes, app responds |

**Approach:** iPhone is the remote control, not the brain
- View conversation history
- Adjust personality/genome settings
- Monitor evolution metrics
- Override/guide agent behavior
- Receive push notifications for alerts

**Milestones:**
1. SwiftUI dashboard wireframe
2. WebSocket connection to orchestrator
3. Genome editor (adjust personality sliders)
4. Evolution metrics visualization
5. Push notification integration
6. App Store submission

**Target:** Prototype 6-8 weeks after Android

### 2c. WASM Browser Playground
**Priority: LOW — demo/marketing tool**

- Compile Rust agent to `wasm32-unknown-unknown`
- Browser-based EvoClaw playground
- "Try an evolving agent in your browser"
- Great for onboarding new contributors

---

## Phase 3: Companion Devices 🧸

### 3a. Voice Pipeline
- Cloud STT integration (Whisper API / Deepgram)
- Cloud TTS integration (ElevenLabs / OpenAI TTS)
- Audio streaming over MQTT/WebSocket
- Wake word detection (lightweight on-device)
- Emotion detection from voice tone

### 3b. Persona Engine
- character.toml + personality.md + boundaries.md
- Per-user memory (remembers names, interests, history)
- Evolving personality (funnier if child laughs, calmer if elder is anxious)
- Age-appropriate content filtering
- Multi-language support

### 3c. Reference Hardware
- **Companion Toy:** Pi Zero 2W + mic + speaker (~$42 BOM)
- **Wearable Agent:** ESP32-S3 + mic + speaker (~$22 BOM)
- **Home Hub:** Pi 4 + mic array + speaker + display
- Open-source hardware designs + 3D printable enclosures

---

## Phase 4: Trader Agent 📈

- Hyperliquid full integration (live trading)
- EIP-712 signing in native Rust (remove Python dependency)
- Multi-exchange support (Binance, Bybit, dYdX)
- Trader personality profiles (style.toml)
- Portfolio management across agents
- Backtesting framework
- Risk management system

---

## Phase 5: Agent Mesh 🕸️

- Multi-agent coordination (agents collaborate on tasks)
- Agent discovery (MQTT-based service registry)
- Skill sharing (agent teaches agent)
- Collective evolution (swarm fitness)
- ClawChain integration (on-chain agent identity + reputation)

---

## Phase 6: ClawOS 🌊

**Parked — see [clawinfra/clawos](https://github.com/clawinfra/clawos)**

- ClawOS Lite: Minimal Buildroot Linux (~30MB image)
- ClawOS Core: seL4-based microkernel (<5MB image)
- Agent as first-class OS citizen
- <500ms boot time
- Over-the-air genome updates

---

## Architecture Targets

```
Platform         Rust Agent    Go Orchestrator    Status
─────────────────────────────────────────────────────────
Linux x86_64     ✅            ✅                 Done
Linux ARM64      ✅            ✅                 Done  
Linux ARMv7      ✅            ✅                 Done
Terminal TUI     N/A           ✅                 Done (Phase 1b)
Web Terminal     N/A           🔜                 Phase 1b
BSC/opBNB        N/A           ✅                 Done (Phase 1b)
Android          🔜            ❌ (cloud)         Phase 2a
iOS              ⚠️ (limited)  ❌ (cloud)         Phase 2b
macOS            ✅            ✅                 Supported
Windows          ✅            ✅                 Supported
WASM             🔜            ⚠️ (TinyGo)       Phase 2c
ESP32 (no_std)   🔜            ❌                 Phase 3
RISC-V           ✅            ✅                 Supported
Bare metal       🔜            ❌                 Phase 3
```

---

## The Water Principle

> *"Empty your mind, be formless, shapeless — like water."*

EvoClaw doesn't choose a platform. It flows to wherever it's needed.
A phone. A toy. A server. A chip. A browser.

Same DNA. Same evolution. Different container.

**Be water, my agent.** 🌊🧬

---

*Last updated: 2026-02-09*
