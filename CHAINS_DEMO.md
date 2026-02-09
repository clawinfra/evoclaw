# Multi-Chain Support Demo

EvoClaw now supports multi-chain configuration for agents to interact with multiple blockchains.

## Architecture

```
ClawChain (Home)
    ↕ (reputation, identity, governance)
ChainAdapter Registry
    ↕
Execution Chains (BSC, ETH, Solana, Hyperliquid, etc.)
```

## Quick Start

### 1. Add a chain

```bash
# Add BSC testnet (preset with defaults)
evoclaw chain add bsc-testnet --wallet 0x2331...

# Add custom EVM chain
evoclaw chain add my-chain \
  --type evm \
  --rpc https://custom.example.com \
  --chain-id 12345 \
  --wallet 0x...

# Add Hyperliquid
evoclaw chain add hyperliquid --wallet 0x...
```

### 2. List chains

```bash
evoclaw chain list
```

Output:
```
Chains:

  bsc-testnet          EVM (97)        BNB Smart Chain Testnet        ✅ enabled
  hyperliquid          HYPERLIQUID     Hyperliquid DEX                ✅ enabled

Total: 2 chain(s)
```

### 3. Remove a chain

```bash
evoclaw chain remove bsc-testnet
```

## Supported Presets

The following chains have built-in presets (no need to specify RPC, chain ID, etc.):

### BNB Chain
- `bsc` - BNB Smart Chain Mainnet (56)
- `bsc-testnet` - BNB Smart Chain Testnet (97)
- `opbnb` - opBNB Mainnet (204)
- `opbnb-testnet` - opBNB Testnet (5611)

### Ethereum
- `ethereum` - Ethereum Mainnet (1)
- `ethereum-sepolia` - Ethereum Sepolia Testnet (11155111)

### Layer 2s
- `arbitrum` - Arbitrum One (42161)
- `optimism` - Optimism (10)
- `polygon` - Polygon (137)
- `base` - Base (8453)

### Non-EVM
- `hyperliquid` - Hyperliquid DEX
- `solana` - Solana Mainnet
- `solana-devnet` - Solana Devnet

## Config Format

Chains are stored in `evoclaw.json`:

```json
{
  "chains": {
    "bsc-testnet": {
      "enabled": true,
      "type": "evm",
      "name": "BNB Smart Chain Testnet",
      "rpcUrl": "https://data-seed-prebsc-1-s1.binance.org:8545",
      "chainId": 97,
      "wallet": "0x2331234567890abcdef",
      "explorer": "https://testnet.bscscan.com"
    }
  }
}
```

## Backward Compatibility

The old `onchain` config is automatically migrated to the new `chains` format:

```json
{
  "onchain": {
    "enabled": true,
    "rpcUrl": "https://...",
    "chainId": 97
  }
}
```

→ Becomes:

```json
{
  "chains": {
    "bsc-testnet": {
      "enabled": true,
      "type": "evm",
      "rpcUrl": "https://...",
      "chainId": 97
    }
  }
}
```

## Development

### Adding a New Preset

Edit `internal/config/chains.go`:

```go
ChainPresets = map[string]ChainPreset{
  "my-chain": {
    ID:       "my-chain",
    Type:     "evm",
    Name:     "My Custom Chain",
    RPCURL:   "https://rpc.mychain.com",
    ChainID:  99999,
    Explorer: "https://explorer.mychain.com",
  },
}
```

### Wiring into Startup

Chains are automatically loaded from config and registered in `cmd/evoclaw/main.go`:

```go
// Create chain registry and setup chains
app.ChainRegistry = onchain.NewChainRegistry(app.Logger)
if err := setupChains(app.ChainRegistry, cfg, app.Logger); err != nil {
  return nil, fmt.Errorf("setup chains: %w", err)
}
```

EVM chains use the existing `BSCClient` (it's generic for any EVM).

## Demo Flow

```bash
# Initialize config
evoclaw init  # (if init command exists, otherwise it creates default)

# Add BSC testnet
evoclaw chain add bsc-testnet --wallet 0x2331234567890abcdef

# Start the agent
evoclaw

# Agent is now ready to trade/DEX/DAO on BSC testnet
```

## Testing

```bash
# Run all tests
go test ./...

# Run chain-specific tests
go test ./internal/config -v
go test ./internal/cli -v
```

## Next Steps

- [ ] Implement Solana adapter
- [ ] Implement Hyperliquid adapter
- [ ] Implement Substrate/Polkadot adapter
- [ ] Add chain switching in agent actions
- [ ] Cross-chain reputation aggregation
