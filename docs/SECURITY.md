# EvoClaw Security: Signed Constraints

## Overview

EvoClaw's **Signed Constraints** (Security Layer 1) ensures that genome constraints — the hard safety boundaries governing agent behavior — cannot be modified by the evolution engine or any unauthorized party.

Constraints (`MaxLossUSD`, `BlockedActions`, `MaxDivergence`, `MinVFMScore`, `AllowedAssets`) are cryptographically signed with the owner's Ed25519 key. Every mutation attempt verifies the signature before proceeding.

## How It Works

1. **Owner generates a key pair** (Ed25519):
   ```
   evoclaw security keygen
   ```
   This produces a public key (shared with the system) and a private key (kept secret by the owner).

2. **Owner signs constraints** using their private key. The constraints are serialized to deterministic JSON and signed.

3. **Genome stores signature + public key** alongside constraints in `constraint_signature` and `owner_public_key` fields.

4. **Evolution engine verifies** the signature before every mutation (skill mutation, behavior mutation, weight optimization). If verification fails, the mutation is rejected.

5. **API enforces signatures** — `PUT /api/agents/{id}/genome/constraints` requires a valid signature in the request body.

## Threat Model

| Threat | Mitigated? | How |
|--------|-----------|-----|
| Attacker modifies constraints in genome file | ✅ | Signature verification fails |
| Evolution engine drifts constraints | ✅ | Engine never writes constraints; verified before mutations |
| API caller changes constraints without auth | ✅ | Signature required on constraint update endpoint |
| Attacker replaces public key + signature | ⚠️ | Partially — requires additional trust anchoring (see Future Work) |

## Backward Compatibility

Unsigned genomes (no `constraint_signature` or `owner_public_key`) continue to work with a warning logged. This allows gradual migration.

## API

### Update Signed Constraints

```
PUT /api/agents/{id}/genome/constraints
Content-Type: application/json

{
  "constraints": {
    "max_loss_usd": 500,
    "blocked_actions": ["margin_trade"],
    "allowed_assets": ["BTC", "ETH"],
    "max_divergence": 10.0,
    "min_vfm_score": 0.5
  },
  "signature": "<base64 Ed25519 signature>",
  "public_key": "<base64 Ed25519 public key>"
}
```

Returns `403 Forbidden` if the signature is invalid.

---

# JWT API Authentication (Security Layer 2)

## Overview

All API endpoints are protected by JWT (JSON Web Token) authentication with role-based access control (RBAC). Tokens use HMAC-SHA256 signing.

## Configuration

Set the JWT secret via environment variable:

```bash
export EVOCLAW_JWT_SECRET="your-secret-key-at-least-32-bytes"
```

**Dev mode:** If `EVOCLAW_JWT_SECRET` is not set, authentication is disabled with a warning logged. This allows local development without tokens.

## Roles

| Role | Access |
|------|--------|
| **owner** | Full access — all endpoints, all methods |
| **agent** | Limited — `GET /api/agents/{id}/genome`, `POST /api/agents/{id}/feedback`, `GET /api/agents/{id}/genome/behavior` |
| **readonly** | Read-only — all `GET` endpoints |

## Generating Tokens

### Via API

```bash
curl -X POST http://localhost:8080/api/auth/token \
  -H "Content-Type: application/json" \
  -d '{"agent_id": "agent-1", "role": "owner"}'
```

Response:
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "expires_in": 86400,
  "token_type": "Bearer"
}
```

### Using Tokens

Include the token in the `Authorization` header:

```bash
curl http://localhost:8080/api/status \
  -H "Authorization: Bearer eyJhbGciOiJIUzI1NiIs..."
```

## Token Claims

| Field | Description |
|-------|-------------|
| `agent_id` | The agent identifier |
| `role` | One of: `owner`, `agent`, `readonly` |
| `iat` | Issued-at timestamp (Unix) |
| `exp` | Expiration timestamp (Unix) |

## Edge Agent Validation

Edge agents (Rust) validate JWT tokens received from the hub using the `validate_jwt()` function in `security.rs`. This ensures that commands from the hub are authenticated.

## Error Responses

| Status | Meaning |
|--------|---------|
| 401 | Missing, invalid, or expired token |
| 403 | Valid token but insufficient role for the endpoint |

---

---

# Evolution Firewall (Security Layer 3)

## Overview

The **Evolution Firewall** prevents runaway mutations by combining three mechanisms:
1. **Rate Limiting** — caps mutation frequency per agent
2. **Circuit Breaker** — blocks mutations when fitness degrades
3. **Auto-Rollback** — restores last known good genome when circuit trips

The firewall is enabled by default and configurable.

## Rate Limiting

Each agent is limited to **10 mutations per hour** (configurable). The rate limiter uses a sliding window with in-memory tracking.

| Setting | Default | Description |
|---------|---------|-------------|
| `max_mutations_per_hour` | 10 | Maximum mutations per agent per hour |

## Circuit Breaker

The circuit breaker monitors fitness before and after each mutation.

### States

| State | Behavior |
|-------|----------|
| **Closed** | Normal — mutations allowed |
| **Open** | Mutations blocked — fitness dropped too much |
| **Half-Open** | Testing — allows 1 mutation after cooldown |

### Transitions

```
Closed --[fitness drop >30%]--> Open
Open   --[cooldown elapsed]---> Half-Open
Half-Open --[good mutation]---> Closed
Half-Open --[bad mutation]----> Open
```

| Setting | Default | Description |
|---------|---------|-------------|
| `fitness_drop_threshold` | 0.30 | Fractional fitness drop that triggers open |
| `cooldown_period` | 1 hour | Time before open → half-open transition |

## Auto-Rollback

Before every mutation, a genome snapshot is taken. If the circuit breaker trips, the last known good genome can be restored automatically or manually via API.

Snapshots use a **ring buffer** of 10 entries per agent (configurable via `max_snapshots`).

## API Endpoints

### Get Firewall Status

```
GET /api/agents/{id}/firewall
Authorization: Bearer <token>
```

Response:
```json
{
  "enabled": true,
  "rate_limit_remaining": 7,
  "max_mutations_per_hour": 10,
  "circuit_breaker_state": "closed",
  "last_snapshot_time": "2026-02-11T04:30:00Z",
  "snapshot_count": 3
}
```

**Roles:** owner, agent, readonly

### Manual Rollback

```
POST /api/agents/{id}/firewall/rollback
Authorization: Bearer <token>
```

Restores the last snapshot genome. **Roles:** owner only.

### Reset Circuit Breaker

```
POST /api/agents/{id}/firewall/reset
Authorization: Bearer <token>
```

Forces circuit breaker to closed state. **Roles:** owner only.

## Edge Agent Firewall

Edge agents (Rust) implement a local firewall in `firewall.rs` with rate limiting and circuit breaker. This ensures edge agents are protected even without hub connectivity. The edge implementation does not include full snapshot/rollback (simpler environment).

## Dashboard Integration

The firewall status for each agent is available via `GET /api/agents/{id}/firewall` and can be displayed on the web dashboard showing:
- Rate limit usage (remaining / max)
- Circuit breaker state with color coding (green=closed, red=open, yellow=half-open)
- Last snapshot timestamp
- Rollback and reset action buttons (owner only)

---

## Future Work

- **Key Pinning** — trust-on-first-use (TOFU) for owner public keys
- **Constraint History** — signed audit log of all constraint changes
- **Firewall Audit Log** — record all firewall decisions for compliance
