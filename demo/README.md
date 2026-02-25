# EvoClaw Demo dApp

🚀 **Live Demo:** http://135.181.157.121:3000

## Overview

A single-page web dApp showcasing EvoClaw's self-evolving AI agent framework on BNBChain.

## Features

### 1. **Wallet Connection**
- MetaMask integration
- Auto-switch to BSC Testnet (Chain ID: 97)
- Display connected address and tBNB balance

### 2. **Agent Registry**
- **Register Agent:** Create new AI agents with name, model, and capabilities
- **View Agents:** Browse all registered agents with their stats
- **Agent Details:** View detailed information including:
  - Agent DID (unique identifier)
  - Reputation score
  - Action history
  - Success rate
  - Evolution count

### 3. **Action Logging**
- Log agent actions (task execution, learning, optimization)
- Track success/failure rates
- Build reputation over time

### 4. **Stats Dashboard**
- Total agents registered
- Network status (BSC Testnet)
- Contract address with BSCScan link

### 5. **Architecture Overview**
- Visual representation of EvoClaw's tiered architecture
- Evolution Engine
- On-Chain Identity
- Tiered Memory System

## Smart Contract

- **Network:** BSC Testnet (Chain ID: 97)
- **Contract Address:** `0xD20522b083ea53E1B34fBed018bF1eCF8670EaCf`
- **BSCScan:** https://testnet.bscscan.com/address/0xD20522b083ea53E1B34fBed018bF1eCF8670EaCf
- **RPC:** https://data-seed-prebsc-1-s1.binance.org:8545

## Tech Stack

- **Frontend:** Vanilla HTML/CSS/JavaScript (no build step)
- **Web3:** ethers.js v6 (from CDN)
- **Blockchain:** BNBChain (BSC Testnet)
- **Server:** Node.js with `serve`
- **Hosting:** Hetzner VPS (135.181.157.121)

## Design

- Dark theme with blue-to-green gradient accents
- Animated star background
- Mobile responsive
- Professional, modern UI perfect for hackathon demos

## Files

```
demo/
├── index.html       # Main HTML structure
├── style.css        # Styles with animations
├── app.js           # Web3 integration and UI logic
├── logo.jpg         # EvoClaw logo
└── README.md        # This file
```

## Local Development

```bash
# Serve locally
cd /home/user/evoclaw/demo
npx serve -l 3000 -s
```

## Deployment

The demo is deployed on a Hetzner VPS and accessible at port 3000.

### Server Status
```bash
# Check server status
curl http://135.181.157.121:3000

# View logs
ssh -i ~/.ssh/id_ed25519_alexchen root@135.181.157.121 "cat /tmp/demo-server.log"
```

### Systemd Service
A systemd service is configured for persistence across reboots:
```bash
# Start service
systemctl start evoclaw-demo

# Check status
systemctl status evoclaw-demo

# View logs
journalctl -u evoclaw-demo -f
```

## Key Contract Functions

### Read Functions
- `getAgentCount()` - Get total registered agents
- `getAllAgentIds()` - Get all agent IDs
- `agents(bytes32 agentId)` - Get agent details
- `getReputation(bytes32 agentId)` - Get agent reputation score
- `getRecentActions(bytes32 agentId, uint256 count)` - Get recent actions

### Write Functions
- `registerAgent(string name, string model, string[] capabilities)` - Register new agent
- `logAction(bytes32 agentId, string actionType, string description, bytes32 dataHash, bool success)` - Log agent action

## Usage

1. **Connect Wallet**
   - Click "Connect MetaMask"
   - Approve connection
   - Confirm network switch to BSC Testnet

2. **Register an Agent**
   - Fill in agent name (e.g., "AlphaAgent")
   - Enter model (e.g., "GPT-4", "Claude-3")
   - Add capabilities (comma-separated: "reasoning, planning, execution")
   - Click "Register Agent"
   - Confirm transaction in MetaMask

3. **View Agents**
   - Switch to "View Agents" tab
   - Click on any agent card to see details
   - View action history and reputation

4. **Log Actions** (agent owners only)
   - Click on your agent
   - Click "Log Action"
   - Enter action type and description
   - Mark success/failure

## Built For

**BNBChain Hackathon 2026**
- Prize Pool: $100,000+
- Category: DeFi / AI Innovation
- Submission: EvoClaw - Self-Evolving AI Agent Framework

## License

MIT

---

**Demo URL:** http://135.181.157.121:3000

*Built with ❤️ for the future of decentralized AI*
