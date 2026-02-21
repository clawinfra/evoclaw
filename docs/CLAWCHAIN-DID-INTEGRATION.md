# ClawChain DID Auto-Discovery Integration

**ADR-003 · Status: Implemented · Go package: `internal/clawchain`**

---

## What It Does

On startup, EvoClaw automatically checks whether its agent identity (a W3C
Decentralized Identifier) is registered on the ClawChain Substrate testnet.
If the DID document is absent, EvoClaw submits a `register_did` extrinsic
using the configured sr25519 key. The loop then repeats every `checkIntervalHours`
(default 6 h) to handle re-registration after chain resets.

### Cycle Steps

1. **Liveness** — call `system_health` via JSON-RPC; abort silently if the node
   is unreachable or still syncing.
2. **Storage query** — compute `TwoX128("AgentDid") ++ TwoX128("DIDDocuments") ++
   Blake2_128Concat(accountID)` and call `state_getStorage`. A non-null result
   means the DID is already registered.
3. **Registration** (if needed) — run a Python subprocess using
   `substrate-interface` to sign and submit the `register_did` extrinsic.
   The block hash is logged on success.

---

## Configuration (`evoclaw.json`)

```json
{
  "clawchain": {
    "autoDiscover": true,
    "nodeUrl": "http://testnet.clawchain.win:9944",
    "agentSeed": "//Alice",
    "checkIntervalHours": 6,
    "accountIdHex": ""
  }
}
```

| Field | Type | Default | Description |
|---|---|---|---|
| `autoDiscover` | bool | `false` | Enable the discovery loop |
| `nodeUrl` | string | `http://testnet.clawchain.win:9944` | ClawChain HTTP RPC endpoint |
| `agentSeed` | string | — | sr25519 seed phrase (`//Alice` for dev) |
| `checkIntervalHours` | int | `6` | Polling interval in hours |
| `accountIdHex` | string | (derived from seed) | 32-byte public key hex (optional) |

> **Security:** never commit a production `agentSeed` to source control.
> Use an environment variable or secrets manager, then reference it here.

---

## Testing Manually

**Trigger a single discovery cycle from the CLI:**

```bash
# (once evoclaw chain discover subcommand is wired in cmd/evoclaw/gateway.go)
evoclaw chain discover
```

**Run unit tests:**

```bash
cd /media/DATA/clawd/evoclaw
go test ./internal/clawchain/... -v -count=1
```

**Check coverage:**

```bash
go test ./internal/clawchain/... -coverprofile=/tmp/cov.out
go tool cover -func=/tmp/cov.out | grep total
```

---

## Expected Log Output

```
INFO  clawchain auto-discovery started  node_url=http://testnet.clawchain.win:9944  interval=6h0m0s
INFO  clawchain auto-discovery starting  node_url=http://testnet.clawchain.win:9944  interval=6h0m0s
INFO  clawchain discovery: DID already registered
```

On first registration:

```
INFO  clawchain discovery: DID not found on-chain; submitting register_did extrinsic
INFO  clawchain discovery: DID registered successfully  tx_hash=0xabcd1234...
```

If the node is down:

```
WARN  clawchain discovery: node unreachable  error="system_health RPC failed: …"
```

---

## Implementation Notes

- **Storage key** — computed in pure Go (`internal/clawchain/storage_key.go`)
  using an inline xxHash64 implementation (no CGo, no external hash library)
  and `golang.org/x/crypto/blake2b`. Verified against the well-known Substrate
  value `TwoX128("System") = 26aa394eea5630e07c48ae0c9558cef7`.

- **Registration** — uses a Python subprocess (`substrate-interface`) because
  sr25519 signing is not yet available as a pure-Go library with full Substrate
  compatibility. Install with `pip install substrate-interface`.

- **Testability** — the `RPCCaller` interface is injected via
  `NewDiscovererWithCaller`, enabling 100% mock-based unit tests with no
  network dependency.
