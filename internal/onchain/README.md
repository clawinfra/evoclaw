# On-Chain Architecture — Multi-Chain with ClawChain Home

## Design Philosophy

**EvoClaw agents are ClawChain-native, chain-agnostic.**

```
                    ┌──────────────────────────┐
                    │       ClawChain (Home)    │
                    │  ● Agent DID / Identity   │
                    │  ● Reputation (permanent) │
                    │  ● Governance votes        │
                    │  ● $CLAW token balance    │
                    │  ● Evolution history       │
                    └────────────┬─────────────┘
                                 │
                    ┌────────────┴─────────────┐
                    │     Chain Adapter Layer   │
                    │   (unified interface)     │
                    └──┬──────┬──────┬──────┬──┘
                       │      │      │      │
                    ┌──┴──┐┌──┴──┐┌──┴──┐┌──┴──┐
                    │ BSC ││ ETH ││ SOL ││ ... │
                    └─────┘└─────┘└─────┘└─────┘
                    Execution chains (where the action is)
```

## Two-Layer Architecture

### Layer 1: ClawChain (Identity & Reputation)

ClawChain is the agent's **home chain**. This is where:
- Agent DID (Decentralized Identity) lives permanently
- Reputation accumulates across all chains
- Evolution history is recorded
- Governance participation happens
- $CLAW staking and rewards

An agent's ClawChain identity is the **single source of truth**.
No matter which execution chain an agent operates on,
its reputation and identity always resolve back to ClawChain.

### Layer 2: Execution Chains (Where Work Happens)

Agents execute tasks on whatever chain makes sense:
- **BSC/opBNB** → DeFi trading, token swaps (low gas)
- **Ethereum** → High-value DeFi, NFTs, ENS
- **Solana** → High-frequency trading, speed-critical
- **Arbitrum/Optimism** → L2 DeFi
- **Hyperliquid** → Perpetual futures (our AlphaStrike bot)

Actions on execution chains are **reported back** to ClawChain
for reputation tracking. Think of it like:
- You work in different countries (execution chains)
- Your credit score lives in one place (ClawChain)

## Chain Adapter Interface

```go
// ChainAdapter is the universal interface for any blockchain.
// Every chain EvoClaw supports implements this.
type ChainAdapter interface {
    // Identity
    ChainID() string                              // "clawchain", "bsc", "ethereum", "solana"
    ChainType() ChainType                         // Home or Execution

    // Read operations
    GetBalance(ctx, address) (Balance, error)
    GetTransaction(ctx, txHash) (Transaction, error)
    CallContract(ctx, address, data) ([]byte, error)

    // Write operations (require signing)
    SendTransaction(ctx, tx) (txHash, error)
    DeployContract(ctx, bytecode, args) (address, txHash, error)

    // Agent-specific operations
    RegisterAgent(ctx, AgentIdentity) (txHash, error)
    LogAction(ctx, agentID, action) (txHash, error)
    GetReputation(ctx, agentID) (uint64, error)

    // Connection management
    Connect(ctx) error
    Close() error
    IsConnected() bool
}
```

## Cross-Chain Flow

When an agent performs a trade on BSC:

```
1. Agent decides to trade on BSC
   └─→ BSC adapter: execute swap
       └─→ tx hash: 0xabc...

2. Agent reports action to ClawChain
   └─→ ClawChain adapter: logCrossChainAction(
           agentDID,
           sourceChain: "bsc",
           txHash: "0xabc...",
           actionType: "trade",
           success: true
       )
       └─→ Reputation updated on ClawChain

3. Other agents can verify:
   └─→ ClawChain: getReputation(agentDID) → 847/1000
   └─→ BSC: verify tx 0xabc... independently
```

## Adapter Registry

```go
// The orchestrator maintains a registry of chain adapters.
// Agents request the adapter they need for each task.

orchestrator.RegisterChain("clawchain", clawchainAdapter)  // always present
orchestrator.RegisterChain("bsc", bscAdapter)              // for DeFi
orchestrator.RegisterChain("hyperliquid", hlAdapter)       // for perps

// Agent requests execution:
adapter := orchestrator.GetChain("bsc")
txHash, err := adapter.SendTransaction(ctx, swapTx)

// Auto-report to home chain:
homeChain := orchestrator.GetHomeChain()  // always ClawChain
homeChain.LogAction(ctx, agent.DID, Action{
    Chain:  "bsc",
    TxHash: txHash,
    Type:   "trade",
})
```
