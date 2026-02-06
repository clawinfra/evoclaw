# Hyperliquid Testnet Integration Guide

## Overview

The EvoClaw edge agent supports both Hyperliquid **mainnet** and **testnet** via the `network_mode` config option. By default, agents start in **testnet + paper trading** mode for safety.

## API Endpoints

| Network  | URL                                      |
| -------- | ---------------------------------------- |
| Mainnet  | `https://api.hyperliquid.xyz`            |
| Testnet  | `https://api.hyperliquid-testnet.xyz`    |

## Getting Testnet Funds

1. Go to https://app.hyperliquid-testnet.xyz
2. Connect your wallet (MetaMask, etc.)
3. Use the testnet faucet to get USDC
4. You can also bridge testnet ETH from Arbitrum Sepolia

## Configuration

### Testnet + Paper Trading (safest, default)

```toml
[trading]
wallet_address = "0xYourAddress"
private_key_path = "keys/testnet.key"
network_mode = "testnet"
trading_mode = "paper"       # Simulates trades locally
```

### Testnet + Live Trading (real testnet orders)

```toml
[trading]
wallet_address = "0xYourAddress"
private_key_path = "keys/testnet.key"
network_mode = "testnet"
trading_mode = "live"
```

### Mainnet + Live Trading (real money!)

```toml
[trading]
wallet_address = "0xYourAddress"
private_key_path = "keys/mainnet.key"
network_mode = "mainnet"
trading_mode = "live"
max_position_size_usd = 1000.0
max_leverage = 3.0
```

## EIP-712 Signing

All order signing is done **natively in Rust** using `alloy` crates. No Python dependency required.

The signing follows Hyperliquid's L1 action scheme:
1. msgpack-serialize the action
2. Append nonce + vault indicator
3. keccak256 hash the payload
4. Wrap in a "phantom agent" EIP-712 message
5. Sign with the configured private key

Source identifier: `"a"` for mainnet, `"b"` for testnet.

## Private Key Setup

Store your private key in a file:

```bash
mkdir -p keys
echo "0xYourPrivateKeyHex" > keys/testnet.key
chmod 600 keys/testnet.key
```

**Never commit private keys to git!**

## Risk Management

Add a `[risk]` section to your config:

```toml
[risk]
max_position_size_usd = 5000.0
max_daily_loss_usd = 500.0
max_open_positions = 5
cooldown_after_losses_secs = 300
consecutive_loss_limit = 3
```

Emergency stop via MQTT: send `{"command": "risk", "action": "emergency_stop"}` to `evoclaw/agents/{id}/commands`.

## MQTT Trading Commands

| Topic                                    | Action          | Description              |
| ---------------------------------------- | --------------- | ------------------------ |
| `evoclaw/agents/{id}/commands`           | `trade.order`   | Place an order           |
| `evoclaw/agents/{id}/commands`           | `trade.cancel`  | Cancel an order          |
| `evoclaw/agents/{id}/commands`           | `trade.positions` | Get positions          |
| `evoclaw/agents/{id}/commands`           | `trade.pnl`     | Get P&L                  |
| `evoclaw/agents/{id}/commands`           | `trade.balance`  | Get balance             |
| `evoclaw/agents/{id}/commands`           | `trade.fills`    | Get trade history       |
| `evoclaw/agents/{id}/commands`           | `trade.cancel_all` | Cancel all orders    |
| `evoclaw/agents/{id}/commands`           | `risk.emergency_stop` | Halt all trading |
| `evoclaw/agents/{id}/commands`           | `risk.status`    | Get risk state          |
