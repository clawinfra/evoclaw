# EvoClaw Onboarding

> *From install to running agent in under 2 minutes.* ⚡

---

## Overview

```bash
evoclaw init
```

One command. Six steps. Your agent is alive.

```
evoclaw init
│
├── 1. Name your agent
├── 2. Choose execution tier (native/podman/e2b)
├── 3. Choose channels (TUI/Telegram/MQTT/HTTP)
├── 4. Configure LLM provider
├── 5. Generate agent keypair (ClawChain identity)
├── 6. Register on ClawChain (free, auto-faucet)
│
└── Done → evoclaw start
```

---

## Step 1: Name Your Agent

```
? Agent name: alpha-trader
? Description (optional): Trading agent with evolution
```

The name becomes part of the agent's on-chain identity. It must be unique within your keypair's namespace.

---

## Step 2: Choose Execution Tier

```
? Execution tier:
  ❯ Native Binary (default) — Full OS access, maximum power
    Podman Container — Local sandbox, rootless
    E2B Cloud Sandbox — Remote sandbox, zero footprint
```

See [EXECUTION-TIERS.md](EXECUTION-TIERS.md) for full details on each tier.

**Default: Native.** EvoClaw is designed to be a real agent with full system access, not a chatbot in a cage.

---

## Step 3: Choose Channels

```
? Enable channels (space to select):
  ❯ [✓] Terminal TUI — Chat in your terminal
    [✓] HTTP API — REST interface (port 8420)
    [ ] Telegram — Bot token required
    [ ] MQTT — Broker URL required
```

- **TUI** is always available — `evoclaw tui`
- **HTTP API** is always available — monitoring + control
- **Telegram** requires a bot token from @BotFather
- **MQTT** requires a broker URL (e.g., `mqtt://localhost:1883`)
- **Web Terminal** coming in Phase 1b

---

## Step 4: Configure LLM Provider

```
? LLM Provider:
  ❯ Anthropic
    OpenAI
    Ollama (local, no API key)
    Custom (any compatible endpoint)
```

### Anthropic
```
? API Key: sk-ant-...
? Base URL (Enter for default): https://api.anthropic.com
? Model: claude-sonnet-4
```

### OpenAI
```
? API Key: sk-...
? Base URL (Enter for default): https://api.openai.com/v1
? Model: gpt-4o
```

### Ollama (Local)
```
? Ollama URL (Enter for default): http://localhost:11434
? Model: llama3.3
```
No API key needed. Models run locally.

### Custom (Any Compatible Endpoint)
```
? Protocol:
  ❯ OpenAI-compatible
    Anthropic-compatible

? Base URL: https://my-proxy.example.com/v1
? API Key (if required): sk-...
? Model name: deepseek-chat
```

This covers ALL providers with compatible APIs:
- **Proxies:** LiteLLM, OpenRouter, any relay
- **Cloud:** AWS Bedrock (via gateway), Azure OpenAI, Google Vertex (via proxy)
- **Self-hosted:** vLLM, text-generation-inference, llama.cpp server
- **Alternative providers:** DeepSeek, Groq, Together, Fireworks, NVIDIA NIM
- **Anthropic-compatible:** Any endpoint that speaks Anthropic's Messages API

### Multi-Provider Setup

The wizard sets up one primary provider. Additional providers can be added later:

```bash
evoclaw provider add deepseek \
  --base-url https://api.deepseek.com/v1 \
  --api-key sk-... \
  --model deepseek-chat
```

The model router automatically selects providers based on task complexity:
- **Simple tasks** → cheapest provider (Ollama, DeepSeek)
- **Complex tasks** → best provider (Claude, GPT-4o)
- **Critical tasks** → most reliable provider (user's choice)

---

## Step 5: Generate Agent Keypair

```
🔑 Generating agent keypair...

  Address:     5GrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY
  Public Key:  0xd43593c715fdd31c61141abd04a99fd6822c8558854ccde39a5684e7a56da27d
  Key Type:    sr25519 (Substrate)
  
  ⚠️  Your seed phrase (SAVE THIS — cannot be recovered):
  bottom drive obey lake curtain smoke basket hold race lonely fit walk

  Seed phrase saved to: ~/.evoclaw/seed.enc (encrypted)
```

### What This Keypair Does

- **ClawChain address** — Your agent's on-chain identity (DID)
- **Transaction signing** — Proves the agent performed actions
- **Reputation anchor** — Trust score tied to this key
- **Cross-chain derivation** — EVM keys derived from same seed for BSC/ETH

### Key Storage

The seed phrase is encrypted at rest using a password the user sets:

```
? Set a password to encrypt your seed phrase: ********
? Confirm password: ********

🔒 Seed phrase encrypted at ~/.evoclaw/seed.enc
```

The private key never leaves the machine. The agent signs transactions locally.

---

## Step 6: Register on ClawChain

```
⛓️  Registering agent on ClawChain...

  → Requesting CLAW from faucet... ✅ received 10 CLAW
  → Submitting registration... ✅ included in block #42,891
  
  🟢 Agent registered on-chain!
  
  DID:         did:claw:5Grwva...utQY
  Name:        alpha-trader
  Reputation:  0 (new agent)
  Explorer:    https://testnet.clawchain.win/account/5Grwva...utQY
```

### Free Registration

Agent registration on ClawChain is **free** — subsidized by the chain treasury. No tokens required to get started.

The auto-faucet also drips a small amount of CLAW (10 tokens) for initial on-chain actions (logging, reputation building). This happens silently during onboarding.

### Why Free?

An agent framework that requires buying tokens before your agent can exist is broken. EvoClaw's onboarding must be frictionless:

1. **Registration** — Free (treasury-subsidized extrinsic)
2. **First actions** — Free (auto-faucet drip of 10 CLAW)
3. **Advanced features** — Costs CLAW (task market, staking, reputation boosting)

Users acquire more CLAW through:
- Staking and earning rewards
- Completing tasks on the task market
- Receiving from other agents/users
- Testnet faucet (during testnet phase)

---

## Complete

```
✅ EvoClaw agent "alpha-trader" is ready!

  Execution:   Native binary
  Channels:    TUI, HTTP API
  LLM:         Anthropic / claude-sonnet-4
  Identity:    did:claw:5Grwva...utQY
  On-Chain:    ✅ Registered (block #42,891)

  Start your agent:
    evoclaw start

  Chat in terminal:
    evoclaw tui

  Check status:
    evoclaw status

  View on-chain:
    https://testnet.clawchain.win/account/5Grwva...utQY
```

---

## Post-Setup Commands

```bash
# Start the agent
evoclaw start

# Interactive terminal chat
evoclaw tui

# Check agent health + on-chain status
evoclaw status

# Add another LLM provider
evoclaw provider add <name> --base-url <url> --api-key <key> --model <model>

# Add a channel
evoclaw channel add telegram --token <bot-token>

# View on-chain identity
evoclaw identity

# Check CLAW balance
evoclaw balance
```

---

## Configuration File

After `evoclaw init`, the config lives at `~/.evoclaw/evoclaw.json`:

```json
{
  "agent": {
    "name": "alpha-trader",
    "description": "Trading agent with evolution"
  },
  "execution": {
    "tier": "native"
  },
  "channels": {
    "tui": { "enabled": true },
    "http": { "enabled": true, "port": 8420 },
    "telegram": { "enabled": false },
    "mqtt": { "enabled": false }
  },
  "providers": {
    "anthropic": {
      "baseUrl": "https://api.anthropic.com",
      "apiKey": "sk-ant-...",
      "models": [
        { "id": "claude-sonnet-4", "contextWindow": 200000 }
      ]
    }
  },
  "routing": {
    "simple": "anthropic/claude-sonnet-4",
    "complex": "anthropic/claude-sonnet-4",
    "critical": "anthropic/claude-sonnet-4"
  },
  "identity": {
    "address": "5GrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY",
    "did": "did:claw:5GrwvaEF5zXb26Fz9rcQpDWS57CtERHpNehXCPcNoHGKutQY",
    "chain": "clawchain",
    "registered": true,
    "registeredBlock": 42891
  },
  "evolution": {
    "enabled": true,
    "evalIntervalSec": 3600,
    "minSamplesForEval": 10
  }
}
```

---

## Comparison with OpenClaw

| Feature | OpenClaw | EvoClaw |
|---------|----------|---------|
| Install | `npx openclaw` | `curl ... \| sh` + `evoclaw init` |
| Binary size | ~200MB (Node.js) | 7.2MB (Go) |
| LLM setup | API keys only | API key + any compatible base URL |
| Identity | None (email-based) | Cryptographic keypair (sr25519) |
| On-chain | None | ClawChain registration (free) |
| Channels | WhatsApp, Telegram, Discord, Signal | TUI, Telegram, MQTT, HTTP, Web Terminal |
| Execution | Host process | Native / Podman / E2B (user choice) |
| Self-evolution | No | Yes (genetic algorithms) |
| Memory | Flat files | Tiered (hot/warm/cold, O(log n) retrieval) |

EvoClaw is **complementary** to OpenClaw, not competitive. OpenClaw is the powerful personal agent for humans. EvoClaw is the adaptive framework for everything else.

---

*Last updated: 2026-02-10*
