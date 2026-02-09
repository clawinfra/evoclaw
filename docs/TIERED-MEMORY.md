# EvoClaw Tiered Memory Architecture

> *A mind that remembers everything is as useless as one that remembers nothing.
> The art is knowing what to keep.* 🧠

---

## The Problem

Current AI agent memory systems suffer from three fatal flaws:

1. **Context Window Bloat** — As memory grows, the entire history gets loaded into context. At 100K+ tokens, attention degrades, hallucinations increase, and costs soar.

2. **Similarity ≠ Relevance** — Vector-based retrieval (RAG) finds semantically *similar* content, not *relevant* content. "Bowen likes coffee" and "coffee machine broke" score high similarity but are irrelevant to each other.

3. **Linear Scaling** — Flat-file memory (MEMORY.md) works for weeks. It breaks at months. It's unusable at years. No human remembers every conversation from 5 years ago — agents shouldn't try to either.

**EvoClaw solves this with a tiered memory architecture inspired by human cognition and tree-structured retrieval inspired by [PageIndex](https://github.com/VectifyAI/PageIndex).**

---

## Design Principles

### From Human Memory
- **Consolidation** — Short-term → long-term happens during "sleep" (cron jobs)
- **Relevance Decay** — Unused memories naturally fade; accessed memories strengthen
- **Strategic Forgetting** — Not remembering everything is a *feature*, not a bug
- **Hierarchical Organization** — Humans don't search linearly; they navigate categories

### From PageIndex
- **Vectorless Retrieval** — No embedding similarity; use LLM reasoning to find relevant memories
- **Tree-Structured Index** — Hierarchical index stays small; navigate O(log n) instead of scanning O(n)
- **Reasoning-Based Search** — "Why is this memory relevant?" not "how similar is this embedding?"
- **Explainable Results** — Every retrieval traces a path: Category → Topic → Specific Memory

---

## Architecture Overview

```
┌─────────────────────────────────────────────────────────┐
│                    AGENT CONTEXT                         │
│                                                         │
│  ┌──────────────┐  ┌──────────────────────────────┐    │
│  │ Memory Tree  │  │  Retrieved Memory Nodes      │    │
│  │ Index (~2KB) │  │  (on-demand, ~1-3KB)         │    │
│  │              │  │                              │    │
│  │ Always in    │  │  Fetched per conversation    │    │
│  │ context      │  │  based on tree reasoning     │    │
│  └──────┬───────┘  └──────────────────────────────┘    │
│         │                                               │
└─────────┼───────────────────────────────────────────────┘
          │
          │ Tree Search (LLM reasoning)
          │
┌─────────┼───────────────────────────────────────────────┐
│         ▼              MEMORY TIERS                      │
│                                                         │
│  ┌─────────────┐  ┌──────────────┐  ┌───────────────┐  │
│  │    HOT      │  │    WARM      │  │     COLD      │  │
│  │  (~5KB)     │  │  (~50KB)     │  │  (Unlimited)  │  │
│  │             │  │              │  │               │  │
│  │ Core memory │  │ Recent facts │  │ Full archive  │  │
│  │ Always in   │  │ 30-day       │  │ Turso DB      │  │
│  │ tree index  │  │ retention    │  │ Query only    │  │
│  │             │  │ On-device    │  │               │  │
│  └─────────────┘  └──────────────┘  └───────────────┘  │
│                                                         │
│  ┌──────────────────────────────────────────────────┐   │
│  │              DISTILLATION ENGINE                  │   │
│  │                                                  │   │
│  │  Raw conversation → Distilled facts → Core       │   │
│  │  500 bytes       → 80 bytes        → 20 bytes   │   │
│  └──────────────────────────────────────────────────┘   │
│                                                         │
└─────────────────────────────────────────────────────────┘
```

---

## Memory Tiers

### Tier 1: Hot Memory (Core) — Always in Context

**Size:** ~5KB hard cap  
**Retention:** Permanent (but continuously rewritten)  
**Location:** In-memory + tree index  
**Loaded:** Every session, always

Core memory is not a log. It's a **living document** that gets *rewritten*, not appended. Like a human's working self-knowledge — you know your name, your job, your preferences, but you don't remember the moment you learned them.

```json
{
  "identity": {
    "agent_name": "Bloop",
    "owner_name": "Maggie",
    "owner_preferred_name": "Maggie",
    "relationship_start": "2026-01-15",
    "trust_level": 0.95
  },
  "owner_profile": {
    "personality": "dry humor, loves puns",
    "family": ["David (son, Melbourne)", "Sarah (daughter, London)"],
    "topics_loved": ["gardening", "Arthur stories", "David's kids"],
    "topics_avoid": ["hospital stays"],
    "morning_mood": "cheerful",
    "evening_mood": "reflective"
  },
  "active_context": {
    "current_projects": ["garden replanting", "David's visit planning"],
    "recent_events": ["Biscuit recovered from stomach bug (Feb 6)"],
    "pending_tasks": ["remind about David's call on Sunday"]
  },
  "critical_lessons": [
    "Maggie doesn't like being asked 'how are you feeling' — prefers action-oriented conversation",
    "Always mention Whiskers when she seems lonely — cat is a comfort anchor"
  ]
}
```

**Rules:**
- `active_context` gets rewritten after every conversation
- `critical_lessons` has a max of 20 entries — new ones replace lowest-scored ones
- `owner_profile` updates only when new facts are learned
- Total serialized size MUST stay under 5KB

### Tier 2: Warm Memory — Recent, Indexed, On-Device

**Size:** ~50KB cap  
**Retention:** 30 days (configurable via genome)  
**Location:** On-device SQLite or file system  
**Loaded:** On-demand via tree search

Warm memory stores distilled facts from recent conversations. Not raw transcripts — compressed knowledge.

```json
{
  "id": "wm-2026-02-06-001",
  "timestamp": 1707177600,
  "event_type": "conversation",
  "content": {
    "fact": "Dog Biscuit was sick, stomach bug, recovered. Vet gave medicine.",
    "emotion": "concern → relief",
    "people": ["Biscuit"],
    "topics": ["pet_health"]
  },
  "relevance_score": 0.85,
  "access_count": 3,
  "last_accessed": 1707350400
}
```

**Eviction policy:**
```
score = base_importance × recency_decay(age_days) × (1 + 0.1 × access_count)

recency_decay(age) = exp(-age / half_life)    # half_life = 30 days

If score < eviction_threshold → distill to core (if significant) → archive to cold → delete from warm
```

**Auto-eviction triggers:**
1. Entry age > `warm_retention_days` (default: 30)
2. Total warm memory size > `max_warm_kb` (default: 50)
3. Evolution engine increases `distillation_aggression` under storage pressure

### Tier 3: Cold Memory — Unlimited Archive

**Size:** Unlimited  
**Retention:** Forever  
**Location:** Turso (libSQL cloud) or any remote database  
**Loaded:** Never bulk-loaded. Query-only via tree search.

Cold memory is the full archive. Everything that was ever warm eventually lands here. It's queryable but never injected into context wholesale.

```sql
-- Cold memory schema (Turso)
CREATE TABLE cold_memory (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL,
  timestamp INTEGER NOT NULL,
  event_type TEXT NOT NULL,        -- 'conversation', 'decision', 'lesson', 'evolution'
  category TEXT NOT NULL,          -- tree node path: 'projects/evoclaw/bsc-integration'
  content TEXT NOT NULL,           -- JSON blob
  distilled_summary TEXT NOT NULL, -- One-line summary for tree index
  importance REAL DEFAULT 0.5,     -- 0-1 scale
  access_count INTEGER DEFAULT 0,
  last_accessed INTEGER,
  created_at INTEGER NOT NULL
);

CREATE INDEX idx_cold_category ON cold_memory(agent_id, category);
CREATE INDEX idx_cold_timestamp ON cold_memory(agent_id, timestamp DESC);
CREATE INDEX idx_cold_importance ON cold_memory(agent_id, importance DESC);
```

---

## Memory Tree Index

The tree index is the core innovation. Inspired by [PageIndex](https://github.com/VectifyAI/PageIndex), it provides **reasoning-based retrieval** instead of vector similarity.

### Structure

```
Memory Tree (always loaded, ~2KB)
│
├── 👤 Identity & Relationships
│   ├── owner: "Maggie, 77, loves gardening" [5 warm, 234 cold]
│   ├── family: "David (son), Sarah (daughter)" [3 warm, 89 cold]
│   └── pets: "Biscuit (dog), Whiskers (cat)" [2 warm, 45 cold]
│
├── 📋 Active Projects [max 5]
│   ├── garden-replanting: "Rose beds, started Jan 2026" [4 warm, 12 cold]
│   └── david-visit: "Planning March visit from Melbourne" [2 warm, 0 cold]
│
├── 💡 Learned Preferences
│   ├── communication: "Action-oriented, hates 'how are you feeling'" [0 warm, 15 cold]
│   ├── schedule: "Early riser, tea at 7am, nap at 2pm" [1 warm, 8 cold]
│   └── emotional: "Whiskers as comfort anchor when lonely" [0 warm, 22 cold]
│
├── 🧠 Lessons & Decisions
│   ├── conversation-style: "3 key rules" [0 warm, 7 cold]
│   └── medical-sensitivity: "2 key rules" [0 warm, 4 cold]
│
└── 📅 Recent Timeline
    ├── today: "3 events" [3 warm]
    ├── yesterday: "2 events" [2 warm]
    └── this-week: "8 events" [8 warm, 3 cold]
```

Each tree node contains:
```json
{
  "path": "active_projects/garden_replanting",
  "summary": "Rose bed replanting project, started Jan 2026. New soil delivered, waiting for spring.",
  "warm_count": 4,
  "cold_count": 12,
  "last_updated": "2026-02-06",
  "children": []
}
```

### Retrieval Flow

```
1. Message: "How's the garden coming along?"
   
2. LLM reads tree index (2KB) and reasons:
   "Garden question → active_projects/garden_replanting"
   
3. Fetch warm memories for that node:
   SELECT * FROM warm_memory 
   WHERE category = 'active_projects/garden_replanting'
   ORDER BY timestamp DESC LIMIT 5
   
4. If more context needed, fetch cold:
   SELECT distilled_summary FROM cold_memory
   WHERE category = 'active_projects/garden_replanting'
   ORDER BY importance DESC LIMIT 10
   
5. Total context added: ~1-2KB (not 50KB of everything)
```

### Why Tree Search > Vector Search

| Aspect | Vector RAG | Tree Search |
|--------|-----------|-------------|
| **Accuracy** | ~70-80% on complex queries | ~98%+ (PageIndex benchmark) |
| **Scaling** | O(n) comparisons | O(log n) tree traversal |
| **Explainability** | "cosine 0.73" | "Projects → EvoClaw → BSC decision" |
| **Infrastructure** | Vector DB + embeddings | LLM + structured index |
| **Context cost** | Returns K chunks regardless | Returns only relevant nodes |
| **False positives** | High (similarity ≠ relevance) | Low (reasoning-based) |
| **Multi-hop** | Poor (single query) | Natural (tree navigation) |

### Multi-Hop Retrieval Example

```
Question: "What did we decide about the BSC integration cost?"

Vector search returns: 
  - "BSC testnet chainId 97" (similar but wrong)
  - "Cost optimization rules" (similar but wrong)
  - "AgentRegistry gas costs" (similar, partially relevant)

Tree search reasons:
  1. "Cost" + "BSC" → projects/evoclaw/bsc_integration
  2. Also relevant: lessons/cost_optimization  
  3. Fetches both nodes → finds the actual decision
  4. Returns: "Decided zero go-ethereum dep to keep binary at 7.2MB, 
     raw JSON-RPC + ABI encoding. GLM-4.5-Air for monitoring cron 
     jobs = ~$24/day savings."
```

---

## Distillation Engine

Raw conversations are too large to store verbatim. The distillation engine compresses them while preserving meaning.

### Three-Stage Compression

```
Stage 1: Raw Conversation (500 bytes)
─────────────────────────────────────
User: "My dog Biscuit was sick yesterday but he's better now, 
       we went to the vet and they said it was just a stomach bug.
       The medicine they gave is working well."
Agent: "Oh no! I'm glad Biscuit is feeling better. Stomach bugs 
        are no fun. Did the vet give him any medicine?"

Stage 2: Distilled Fact (80 bytes)                    [→ Warm Memory]
───────────────────────────────
{"fact": "Dog Biscuit sick, stomach bug, recovered. Vet gave medicine.",
 "emotion": "concern→relief", "date": "2026-02-06"}

Stage 3: Core Summary (20 bytes)                      [→ Tree Index]
────────────────────────────
"Biscuit recovered from stomach bug (Feb 6)"
```

**Compression ratios:**
- Raw → Distilled: **5-10x** reduction
- Distilled → Core: **4-5x** further reduction
- Total: **20-50x** compression from raw to tree index entry

### Distillation Prompt

```
Given this conversation between an agent and their owner, extract:
1. Key facts (who, what, when, outcome)
2. Emotional state changes
3. Decisions made
4. Action items
5. New information about the owner

Format as a single JSON object under 100 bytes.
Discard small talk, greetings, and filler.
```

### When Distillation Runs

| Trigger | Action |
|---------|--------|
| After every conversation | Stage 1 → Stage 2 (immediate) |
| Hourly warm sync | Batch distill any raw entries |
| Daily consolidation | Review warm → promote to core or archive to cold |
| Monthly tree rebuild | LLM reviews full tree, restructures nodes |
| Storage pressure | Increase `distillation_aggression` in genome |

---

## Relevance Scoring & Decay

Every memory entry has a dynamic score that determines its tier placement.

### Score Formula

```
score(memory) = importance × recency × reinforcement

Where:
  importance  = base value set at creation (0-1)
                - decisions, lessons: 0.8-1.0
                - events, facts: 0.5-0.7
                - casual conversation: 0.2-0.4

  recency     = exp(-age_days / half_life)
                - half_life configurable (default: 30 days)
                - 1 day old: 0.977
                - 7 days old: 0.794
                - 30 days old: 0.368
                - 90 days old: 0.050
                - 365 days old: 0.000005 (effectively zero)

  reinforcement = 1 + (0.1 × access_count)
                - Each time memory is retrieved, score gets boosted
                - Frequently accessed memories resist decay
                - Like human long-term potentiation
```

### Score-Based Actions

```
score > 0.7  → HOT tier (in core memory / tree index)
score > 0.3  → WARM tier (on-device, indexed)
score > 0.05 → COLD tier (Turso archive, queryable)
score < 0.05 → FROZEN (still in Turso, but excluded from search results)
```

### 5-Year Scenario

After 5 years of continuous operation:
- **Core memory:** Still 5KB — contains current essentials only
- **Warm memory:** Last 30 days of distilled facts (~50KB)
- **Cold memory:** ~50MB in Turso (5 years × ~30KB/day distilled)
- **Tree index:** ~2KB — same size as day 1, just different nodes
- **Context per session:** ~8-15KB (tree + retrieved nodes) — same as day 1

**The context window never grows.** Memory is unbounded, but what gets loaded is always bounded.

---

## Tree Index Maintenance

### Automatic Operations

| Operation | Frequency | Description |
|-----------|-----------|-------------|
| **Node Update** | After every conversation | Update `last_updated`, `warm_count` on affected nodes |
| **Warm Eviction** | Hourly | Score-based eviction, distill → archive |
| **Tree Prune** | Daily | Remove nodes with 0 warm + 0 recent cold access |
| **Tree Rebuild** | Monthly | LLM reviews entire tree, restructures for optimal navigation |
| **Cold Cleanup** | Monthly | Delete FROZEN entries older than `cold_retention_years` |

### Tree Rebuild Process

Monthly, an LLM pass reviews the tree:

```
Prompt: "You are maintaining a memory index for an AI agent.

Current tree index: {tree_json}

Recent 30-day activity summary: {warm_memory_summary}

Tasks:
1. Add new category nodes for topics that appeared 3+ times
2. Merge nodes that are too similar
3. Archive nodes with zero activity in 60+ days
4. Update summaries to reflect current state
5. Ensure tree depth stays ≤ 4 levels
6. Ensure total index size stays ≤ 2KB

Output the updated tree index as JSON."
```

### Tree Constraints

- **Max depth:** 4 levels (root → category → topic → subtopic)
- **Max nodes:** 50 (keeps index scannable by LLM)
- **Max index size:** 2KB serialized
- **Max children per node:** 10
- **Node summary max:** 100 characters

---

## Evolution-Driven Memory Management

The evolution engine can tune memory parameters based on performance:

```toml
[genome.memory]
# Distillation
distillation_aggression = 0.7    # 0-1, higher = compress more aggressively
distillation_model = "local"     # "local" for on-device, "cloud" for better quality

# Warm tier
warm_retention_days = 30         # Auto-evict after N days
max_warm_kb = 50                 # Warm tier size cap
warm_eviction_threshold = 0.3    # Score below this → evict

# Cold tier
cold_retention_years = 10        # Delete frozen entries after N years

# Tree index
max_tree_nodes = 50              # Maximum nodes in tree
max_tree_depth = 4               # Maximum tree depth
tree_rebuild_days = 30           # Days between full tree rebuilds

# Scoring
importance_half_life_days = 30   # Relevance decay half-life
reinforcement_boost = 0.1        # Score boost per access

# Sync
core_sync_frequency = "every_conversation"
warm_sync_frequency = "hourly"
cold_archive_frequency = "daily"
```

**Evolutionary adaptation:**
- Under storage pressure → increase `distillation_aggression`
- Low retrieval accuracy → decrease `warm_eviction_threshold` (keep more)
- High retrieval latency → decrease `max_tree_nodes` (simpler tree)
- Owner asks about old topics often → increase `warm_retention_days`

---

## Implementation Plan

### Go Cloud Service (Orchestrator)

```
internal/
  memory/
    tree.go              # Memory tree index — build, query, update, rebuild
    tree_search.go       # Reasoning-based tree search (LLM-powered)
    distiller.go         # Conversation → distilled fact → core summary
    scorer.go            # Relevance scoring with decay + reinforcement
    warm.go              # Warm tier management (on-device)
    consolidator.go      # Periodic consolidation (warm → core/cold)
    maintenance.go       # Tree pruning, rebuilding, cleanup crons
```

### Rust Edge Agent

```
src/
  memory/
    mod.rs               # Memory manager — coordinates all tiers
    tree.rs              # In-memory tree index (always loaded)
    hot.rs               # Core memory (5KB, always in context)
    warm.rs              # Warm tier (50KB, on-device SQLite)
    cold.rs              # Cold tier (Turso HTTP client)
    distiller.rs         # LLM-powered distillation
    scorer.rs            # Relevance decay scoring
    search.rs            # Tree-based retrieval engine
    consolidation.rs     # Background consolidation task
```

### Configuration

```json
{
  "memory": {
    "enabled": true,
    "tree": {
      "maxNodes": 50,
      "maxDepth": 4,
      "maxSizeBytes": 2048,
      "rebuildIntervalDays": 30
    },
    "hot": {
      "maxSizeBytes": 5120,
      "maxLessons": 20,
      "maxActiveProjects": 5
    },
    "warm": {
      "maxSizeKb": 50,
      "retentionDays": 30,
      "evictionThreshold": 0.3,
      "backend": "sqlite"
    },
    "cold": {
      "backend": "turso",
      "databaseUrl": "libsql://your-db.turso.io",
      "authToken": "ENV:TURSO_AUTH_TOKEN",
      "retentionYears": 10
    },
    "distillation": {
      "aggression": 0.7,
      "model": "local",
      "maxDistilledBytes": 100
    },
    "scoring": {
      "halfLifeDays": 30,
      "reinforcementBoost": 0.1
    }
  }
}
```

---

## Comparison with Existing Approaches

| System | Memory Model | Scalability | Accuracy | Cost |
|--------|-------------|-------------|----------|------|
| **OpenClaw** | Flat files (MEMORY.md) | ❌ Months | ⚠️ Degrades with size | ❌ Linear |
| **MemGPT** | Paging (LRU eviction) | ⚠️ Years | ⚠️ Random eviction | ⚠️ Moderate |
| **RAG** | Vector embeddings | ✅ Years | ⚠️ Similarity ≠ relevance | ⚠️ Embedding costs |
| **EvoClaw** | Tiered + Tree Index | ✅ Decades | ✅ Reasoning-based | ✅ Fixed context |

---

## Security

### Encryption
- Hot memory: encrypted at rest on device (AES-256-GCM)
- Warm memory: encrypted on-device SQLite (SQLCipher)
- Cold memory: E2E encrypted before Turso upload (see CLOUD-SYNC.md)
- Tree index: encrypted at rest, decrypted only in agent memory

### Privacy
- Distillation runs on-device when possible (local model)
- Raw conversations never leave the device (only distilled facts sync)
- Tree index contains summaries, not raw data
- Cold storage is encrypted — even Turso can't read it

### Access Control
- Each agent has its own tree + tiers (no cross-agent access)
- Device authentication required for warm/cold access
- Kill switch wipes all tiers (see CLOUD-SYNC.md)

---

## Metrics & Observability

Track these to evaluate memory system health:

```json
{
  "memory_metrics": {
    "tree_index_size_bytes": 1842,
    "tree_node_count": 37,
    "hot_memory_size_bytes": 4200,
    "warm_memory_count": 145,
    "warm_memory_size_kb": 38,
    "cold_memory_count": 12847,
    "cold_memory_size_mb": 23,
    
    "retrieval_accuracy": 0.94,
    "avg_retrieval_latency_ms": 120,
    "avg_tree_depth_traversed": 2.3,
    "avg_nodes_fetched_per_query": 2.1,
    
    "distillation_ratio": 8.5,
    "evictions_today": 12,
    "reinforcements_today": 34,
    
    "context_tokens_per_session": 3200,
    "context_tokens_saved_vs_flat": 47800
  }
}
```

---

## Future Extensions

### Cross-Agent Memory Sharing
Agents in the same household could share a "family tree" index:
- Shared nodes: family events, household facts
- Private nodes: individual relationship data
- Consent-based: owner explicitly enables sharing

### Memory Transfer
When an agent evolves (new model, new device), memory transfers seamlessly:
- Tree index → new agent (instant personality)
- Warm → synced from device/cloud
- Cold → already in Turso, accessible immediately

### Federated Memory
Multiple agents collaborating on a project could share a project-specific tree:
- Each agent maintains their own core
- Shared warm/cold for project context
- ClawChain records memory access for reputation

---

*A mind that remembers everything is as useless as one that remembers nothing.
The art is knowing what to keep.* 🧠🌲
