# EvoClaw Cloud Sync & Disaster Recovery

> *The device is a vessel. The soul flows through the cloud.
> Break the vessel, pour into a new one. Same water.* 🌊

---

## Principle: Cloud-First Memory

The device is replaceable. The relationship is not.

If a child's companion toy breaks, the replacement must wake up and say:
*"Hey Mia! I missed you! How's Biscuit doing?"*

NOT: *"Hello. I am your new companion. What is your name?"*

**The device is a terminal. The soul lives in the cloud.**

---

## Architecture

```
┌─────────────┐              ┌──────────────────────┐
│  Device A   │──── sync ───▶│    EvoClaw Cloud     │
│  (original) │   encrypted  │                      │
└──────┬──────┘              │  ┌────────────────┐  │
       │                     │  │  Agent Soul     │  │
       │ 💥 lost/broken      │  │  ────────────   │  │
       │                     │  │  core_memory    │  │
┌──────┴──────┐              │  │  genome.toml    │  │
│  Device B   │◀── restore ──│  │  persona/*      │  │
│  (replace)  │   < 2 min    │  │  warm_memory    │  │
│             │              │  │  evolution_log   │  │
└─────────────┘              │  │  full_archive   │  │
                             │  └────────────────┘  │
                             └──────────────────────┘
```

---

## Tiered Memory Model

### Hot Memory (on-device, always loaded)
- **Size:** ~100KB
- **Contents:** Current conversation, active genome, today's context, safety boundaries
- **Sync:** After every conversation (critical sync)

### Warm Memory (on-device, indexed)
- **Size:** ~5MB cap
- **Contents:** Recent conversations (7 days), learned preferences, relationship data
- **Sync:** Hourly
- **Eviction:** Oldest conversations distilled → core memory, then deleted locally

### Cold Memory (cloud only)
- **Size:** Unlimited
- **Contents:** Full conversation archive, evolution history, old genomes
- **Sync:** Daily full backup when on WiFi

### Core Memory (permanent, never deleted)
- **Size:** ~10-50KB
- **Contents:** The essence of the relationship
- **Sync:** After every conversation (critical sync)
- **Eviction:** NEVER

```json
{
  "owner": {
    "name": "Margaret",
    "preferred_name": "Maggie",
    "birthday": "1948-03-15",
    "family": ["David (son, Melbourne)", "Sarah (daughter, London)"],
    "late_husband": "Arthur, met at a dance in 1972",
    "pet": "Whiskers (cat)"
  },
  "relationship": {
    "first_met": "2026-01-15",
    "total_conversations": 847,
    "trust_level": 0.95,
    "topics_loved": ["gardening", "Arthur stories", "David's kids"],
    "topics_avoid": ["hospital stays", "driving"]
  },
  "personality_learned": {
    "humor": "dry, loves puns",
    "morning_mood": "cheerful",
    "evening_mood": "reflective, sometimes lonely"
  }
}
```

---

## Memory Distillation

Raw conversations are compressed before archival:

```
RAW (500 bytes):
User: "My dog Biscuit was sick yesterday but he's better now, 
       we went to the vet and they said it was just a stomach bug"
Agent: "Oh no! I'm glad Biscuit is feeling better. Stomach bugs 
        are no fun. Did the vet give him any medicine?"

DISTILLED (80 bytes):
{"fact": "dog Biscuit was sick, stomach bug, recovered", 
 "date": "2026-02-06", "emotion": "concern→relief"}
```

**5-10x compression** by extracting what matters. The orchestrator's LLM performs distillation before syncing back to the device.

---

## Sync Protocol

### 1. Heartbeat (every 60s when online)
```
POST /api/agent/{id}/heartbeat
{
  "status": "alive",
  "storage_used_kb": 4200,
  "battery_pct": 82,
  "last_sync": "2026-02-06T10:30:00Z"
}
```

### 2. Critical Sync (after every conversation)
```
POST /api/agent/{id}/sync/critical
{
  "core_memory": { ... },
  "genome": { ... },
  "persona": { ... }
}
```
**Must succeed before agent enters idle.** Retries with exponential backoff.

### 3. Warm Sync (every hour)
```
POST /api/agent/{id}/sync/warm
{
  "conversations": [ ...last hour... ],
  "metrics": { ... },
  "evolution_events": [ ... ]
}
```

### 4. Full Sync (daily, on WiFi)
```
POST /api/agent/{id}/sync/full
{
  "archive": [ ...everything since last full sync... ]
}
```

---

## Disaster Scenarios

### 🔨 Device Broken / Damaged
```
Recovery time: < 2 minutes

1. New device powers on
2. User scans QR code or enters agent ID + recovery PIN
3. Cloud pushes: core_memory + genome + persona (< 100KB)
4. Agent is immediately functional with full personality
5. Warm memory syncs in background (minutes)
6. Full archive available on demand

Result: Agent remembers everything. Zero relationship loss.
```

### 🏠 Device Stolen
```
1. Owner marks device stolen (app / web / phone call)
2. Cloud sends KILL signal on next heartbeat
3. Device wipes all local memory + encryption keys
4. Agent ID locked — cannot reactivate without owner auth
5. New device gets full restore from cloud
6. Stolen device is a useless brick

Result: Thief gets nothing. Owner loses nothing.
```

### 🌊 Cloud Down / Unreachable
```
1. Device continues operating with local memory
2. Sync operations queued locally
3. Rolling window manages storage pressure
4. Core memory always preserved on-device
5. When cloud returns: queued syncs execute in order

Result: Agent never stops working. Backup resumes automatically.
```

### 📴 Extended Offline (weeks/months)
```
1. Device operates independently using local memory
2. Rolling window evicts old conversations (distills first)
3. Core memory preserved (permanent)
4. Evolution continues locally
5. On reconnection:
   - Pushes all accumulated data to cloud
   - Receives genome updates from orchestrator
   - Catches up on any evolution decisions
   
Result: Agent degrades gracefully. Core relationship intact.
```

### 💀 Total Loss (device + cloud region)
```
1. Cloud replicates across 2+ regions (active-passive)
2. If primary region fails, secondary promotes
3. RPO (Recovery Point Objective): < 1 hour
4. RTO (Recovery Time Objective): < 5 minutes

For self-hosted: owner is responsible for backup strategy.
```

---

## Encryption

**End-to-end encryption is non-negotiable.** Not even we can read agent memory.

```
Device                              Cloud
┌──────────────┐                   ┌──────────────┐
│ Agent Key    │                   │              │
│ (derived)    │──── encrypted ───▶│  Encrypted   │
│              │     AES-256-GCM   │  Blobs       │
│ = device_key │                   │              │
│ + owner_pin  │◀── encrypted ────│  Cannot read │
│ + HKDF       │     blobs only    │  without key │
└──────────────┘                   └──────────────┘
```

### Key Derivation
```
agent_key = HKDF(
  ikm = device_key || owner_secret,
  salt = agent_id,
  info = "evoclaw-memory-v1"
)
```

### Owner Secret Options
- PIN code (simple, for elderly users)
- Biometric (fingerprint/face on phone app)
- Recovery phrase (12 words, for advanced users)

### Key Recovery
If owner forgets their secret: **memory is gone by design.**
- Privacy > convenience
- No backdoor, no master key
- Owner can generate new secret → agent starts fresh relationship
- Family/caregiver can be set as recovery contact (optional)

---

## Multi-Device Sync

Same agent soul, multiple devices:

```
🧸 Bedroom companion (primary)
     ↕ cloud sync (real-time)
📱 Phone app (on the go)
     ↕ cloud sync (real-time)
🏠 Kitchen hub (daytime)

All three share one agent soul.
Maggie talks to Bloop in bed at night,
continues the conversation in the kitchen at breakfast.
Seamless.
```

### Conflict Resolution
When two devices sync simultaneously:
1. **Conversations:** Append-only, merge by timestamp
2. **Core memory:** Last-write-wins with vector clock
3. **Genome:** Only orchestrator can update (single source of truth)
4. **Persona:** Manual merge with owner notification if conflict

---

## Evolution-Driven Memory Management

The evolution engine can tune memory under storage pressure:

```toml
[genome.memory]
distillation_aggression = 0.7    # 0-1, higher = compress more
warm_retention_days = 7          # Auto-evict after N days
core_update_frequency = "daily"  # How often to distill to core
sync_frequency_minutes = 60      # Warm sync interval
```

Under storage pressure, the evolution engine can:
- Increase distillation aggression (compress more aggressively)
- Shorten warm retention (evict older conversations faster)
- Prioritize which facts make it to core memory
- Increase sync frequency (offload to cloud faster)

**The agent evolves its own memory management based on its container's constraints.** 🌊

---

## Implementation Plan

### Rust Edge Agent
```
src/
  sync/
    mod.rs           # Sync orchestration & scheduling
    critical.rs      # Core memory + genome sync (every conversation)
    warm.rs          # Recent memory sync (hourly)
    full.rs          # Complete archive sync (daily)
    encryption.rs    # AES-256-GCM E2E encryption
    recovery.rs      # Device restore flow
    kill_switch.rs   # Remote wipe on theft
    queue.rs         # Offline sync queue
    conflict.rs      # Multi-device conflict resolution
```

### Go Cloud Service
```
cmd/evoclaw-cloud/
  main.go                # Cloud sync server
internal/
  storage/
    blob.go              # Encrypted blob storage (S3/local/multi-region)
    replication.go       # Cross-region replication
  restore/
    restore.go           # Device recovery API
    qr.go                # QR code generation for pairing
  security/
    kill_switch.go       # Remote wipe management
    device_registry.go   # Device tracking & authentication
    encryption.go        # Key management (never stores raw keys)
  api/
    sync_handlers.go     # Heartbeat, critical, warm, full sync endpoints
    device_handlers.go   # Pair, restore, kill, status endpoints
```

---

## Configuration

```toml
# agent.toml cloud sync section

[cloud]
enabled = true
endpoint = "https://sync.evoclaw.io"
region = "ap-southeast-2"              # Primary region

[cloud.sync]
heartbeat_interval_seconds = 60
critical_sync = true                    # After every conversation
warm_sync_interval_minutes = 60
full_sync_interval_hours = 24
full_sync_require_wifi = true

[cloud.encryption]
algorithm = "AES-256-GCM"
key_derivation = "HKDF-SHA256"

[cloud.recovery]
allow_family_recovery = true
recovery_contacts = ["david@email.com"]  # Can trigger restore
max_devices = 3                          # Multi-device limit

[cloud.storage]
max_warm_mb = 5
max_core_kb = 50
distill_before_evict = true
```

---

*The device is a vessel. The soul flows through the cloud.
Break the vessel, pour into a new one. Same water.* 🌊🧬
