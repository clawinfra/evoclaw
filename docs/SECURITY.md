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

## Future Work

- **JWT Authentication** — API-level auth for all endpoints
- **Evolution Firewall** — rate limiting and anomaly detection on mutations
- **Key Pinning** — trust-on-first-use (TOFU) for owner public keys
- **Constraint History** — signed audit log of all constraint changes
