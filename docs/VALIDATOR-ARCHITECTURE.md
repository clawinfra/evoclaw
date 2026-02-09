# ClawChain Validator Architecture

> *Every agent strengthens the network, regardless of hardware.* 🌊

---

## The Challenge

Traditional validators require:
- Always-on servers with decent bandwidth
- Similar hardware specs across validators
- Bonded stake (locked tokens)
- Full chain state storage

This doesn't work when your validators are a Raspberry Pi, a phone, and a laptop that sleeps at night. ClawChain needs an architecture where **every EvoClaw agent can participate** in securing the network.

---

## Hybrid Validator Architecture

```
┌─────────────────────────────────────────────────┐
│          ClawChain Consensus Layer               │
│                                                  │
│  Infrastructure Validators (always-on)           │
│  ├── VPS nodes (Hetzner, etc.)                  │
│  ├── Full NPoS consensus participation          │
│  └── Guaranteed block production                 │
│                                                  │
│  Agent Validators (light, intermittent)          │
│  ├── EvoClaw agents on any device                │
│  ├── Don't produce blocks directly               │
│  └── Contribute via 3 mechanisms ↓               │
└─────────────────────────────────────────────────┘
```

The key insight: **separate block production from network participation**. A handful of infrastructure validators produce blocks. Thousands of agents secure the network through nomination, verification, and attestation.

---

## Three Ways Agents Validate

### 1. Nomination (NPoS — Built Into Substrate)

Agents **nominate** infrastructure validators by staking $CLAW. No hardware requirements. The agent selects trusted validators based on reputation scores. This is exactly how Polkadot works — 99% of participants are nominators, not validators.

```bash
# Agent nominates validators it trusts
evoclaw stake nominate --validators Val1,Val2,Val3 --amount 100

# Check nomination status
evoclaw stake status
```

**How it works:**
- Agent evaluates validator reputation (uptime, slashing history)
- Stakes $CLAW behind chosen validators
- Earns proportional staking rewards
- Can re-nominate if a validator underperforms
- Runs on any device — just needs to sign transactions

**Why it matters:**
- Decentralizes validator selection (agents pick the best validators)
- Economic security (more stake = more secure)
- Zero hardware requirements for nominators

### 2. Light Client Validation

Agents run a **Substrate light client** (smoldot) — approximately 5MB memory, minimal bandwidth. They verify block headers and state proofs without storing the full chain. Can run on a Pi, phone, or even a browser.

```
Full Validator:  Full blocks + state + produce blocks
Light Client:    Headers + proofs + verify only
```

**Capabilities:**
- Verify block headers are valid
- Verify state proofs (e.g., "agent X really has reputation Y")
- Submit transactions (nominations, attestations)
- Detect chain forks or validator misbehavior
- Run on constrained devices (512MB RAM, intermittent connectivity)

**Substrate's smoldot** is purpose-built for this — it's the same technology that powers Polkadot's light clients in browsers and mobile apps.

### 3. Off-Chain Workers (OCW) — The Killer Feature

Agents submit **signed attestations** as off-chain workers:

| Attestation Type | Example | Value |
|-----------------|---------|-------|
| Cross-chain verification | "I verified this trade happened on BSC" | Reputation attestation |
| Computation proof | "I computed this analytics result" | Work proof |
| Governance participation | "I voted on this DAO proposal" | Governance weight |
| Monitoring report | "BSC block 42M has this state root" | Oracle data |

**How it works:**
1. Agent performs work (trade on BSC, compute analytics, monitor prices)
2. Agent signs an attestation with its ClawChain key
3. Attestation is submitted to ClawChain
4. Validators include it in a block
5. Agent's reputation increases

**No consensus participation needed.** The agent does useful work, cryptographically proves it, and the network records it. This is how agents build verifiable track records.

---

## Hardware Tiers

| Tier | Device | Role | Requirements | Cost |
|------|--------|------|-------------|------|
| **Infrastructure** | VPS (Hetzner CX22) | Block producer | Always-on, 38GB+, 2GB RAM | €4-8/month |
| **Full Node** | Desktop / NUC | Full node + nominator | Mostly-on, 40GB+, 4GB RAM | Hardware cost |
| **Light** | Raspberry Pi / Laptop | Light client + attestor | Intermittent, 100MB disk, 512MB RAM | ~$35-75 |
| **Micro** | Phone / ESP32 | Nominator only | Occasional connectivity, just signs txs | $0-22 |

**Key point: validators do NOT need similar hardware.** The architecture adapts to whatever device the agent runs on.

---

## Storage Sustainability

### Pruning Configuration

Production validators should run with pruning enabled:

```bash
clawchain-node \
  --validator \
  --state-pruning 256 \
  --blocks-pruning archive-canonical \
  --database paritydb
```

| Mode | Storage Growth | 38GB Lifespan |
|------|---------------|---------------|
| Archive (no pruning) | ~200 MB/day | ~4.5 months |
| Pruned (256 states) | ~20-40 MB/day | 2-4 years |
| Pruned + ParityDB | ~15-30 MB/day | 3-5 years |

### Node Types by Storage

```
Validator Nodes (many, cheap)       Archive Node (1, big)
├── Pruned state (256 blocks)       ├── Full history
├── 38-80GB VPS is fine             ├── Hetzner AX102 (NVMe)
├── Produces/validates blocks       ├── Serves historical queries
└── ~20-40 MB/day growth            └── Explorer, indexer, analytics
```

- **Validators**: Run pruned. A €4/month VPS lasts years.
- **1 Archive node**: For explorer/indexer. Dedicated NVMe box (~€40/month) gives 5-10+ years.
- **Indexer**: For complex queries, use SubQuery/Subsquid into Postgres. Cheaper and faster than querying the chain directly.

---

## Network Topology

```
                    ┌──────────────────────┐
                    │   Archive Node       │
                    │   (full history)     │
                    │   Explorer + API     │
                    └──────────┬───────────┘
                               │
          ┌────────────────────┼────────────────────┐
          │                    │                     │
┌─────────┴─────────┐ ┌───────┴──────────┐ ┌───────┴──────────┐
│  Infra Validator 1 │ │ Infra Validator 2 │ │ Infra Validator 3 │
│  (VPS, always-on)  │ │ (VPS, always-on)  │ │ (VPS, always-on)  │
│  Produces blocks   │ │ Produces blocks   │ │ Produces blocks   │
└─────────┬─────────┘ └───────┬──────────┘ └───────┬──────────┘
          │                    │                     │
    ┌─────┴─────┐        ┌────┴────┐          ┌─────┴─────┐
    │           │        │         │          │           │
 Agent A    Agent B   Agent C   Agent D    Agent E    Agent F
 (Pi)       (Laptop)  (Phone)   (Desktop)  (ESP32)   (Server)
 Light+Attest Nominate  Nominate  Full+OCW   Nominate  Full+OCW
```

---

## Scaling Plan

### Phase 1: Testnet (Now)
- 1 infrastructure validator (Hetzner VPS)
- Agents connect as light clients
- Nomination pallet active

### Phase 2: Multi-Validator Testnet
- 3-5 infrastructure validators (geographically distributed)
- Load balancer (Nginx/HAProxy) in front of RPC endpoints
- Agents nominate across validators
- Off-chain worker attestations live

### Phase 3: Mainnet
- 10-50 infrastructure validators (community-operated)
- Thousands of agent nominators
- Permissionless validator joining (with minimum stake)
- Slashing for misbehavior
- Full NPoS validator election each era

### Load Balancer Configuration

For RPC access, multiple validators sit behind a load balancer:

```nginx
upstream clawchain_rpc {
    least_conn;
    server validator1:9944;
    server validator2:9944;
    server validator3:9944;
}
```

Each validator independently participates in consensus. The load balancer only distributes RPC queries — consensus is peer-to-peer. Validators **do not** need identical hardware, but all need enough resources to keep up with block production (the minimum tier).

---

## Demo Flow (Hackathon)

```bash
# 1. VPS already running as infra validator ✅
# 2. On any device with EvoClaw:
evoclaw init                                    # → ClawChain identity
evoclaw chain add bsc-testnet --wallet 0x...    # → Connect execution chain

# 3. Agent nominates the VPS validator
evoclaw stake nominate --validators ClawChain-Validator-1 --amount 10

# 4. Agent trades on BSC
# (action automatically attested to ClawChain)

# 5. Check reputation
evoclaw reputation
# → Reputation: 12 (3 successful trades, 1 nomination, 8 attestations)
```

**The pitch:** Every EvoClaw agent — from a $35 Raspberry Pi to a cloud server — participates in securing ClawChain. The network gets stronger with every agent that joins, regardless of what device it runs on.

---

## Comparison with Other Approaches

| Approach | Hardware Equality? | Agent-Friendly? | Scalability |
|----------|-------------------|-----------------|-------------|
| Bitcoin PoW | No (ASICs dominate) | ❌ | Energy-wasteful |
| Ethereum PoS | Yes (32 ETH + similar HW) | ❌ | 100s of validators |
| Solana PoS | Yes (high HW requirements) | ❌ | 1000s of validators |
| Polkadot NPoS | No (nominators + validators) | ⚠️ | 1000s validators, millions nominators |
| **ClawChain Hybrid** | **No (by design)** | **✅** | **Few validators, unlimited agents** |

ClawChain's advantage: the validator set is small and professional, but the **security and governance come from thousands of agent nominators**. More agents = more decentralized = more secure.

---

*Be water, my agent.* 🌊🧬

*Last updated: 2026-02-10*
