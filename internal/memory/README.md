# EvoClaw Tiered Memory System

Production-ready implementation of the tiered memory architecture described in `/docs/TIERED-MEMORY.md`.

## Architecture

```
┌─────────────────────────────────────────────────┐
│              MEMORY MANAGER                      │
│                                                 │
│  ┌──────────┐  ┌──────────┐  ┌──────────────┐  │
│  │   HOT    │  │   WARM   │  │     COLD     │  │
│  │  (5KB)   │  │  (50KB)  │  │  (Unlimited) │  │
│  │          │  │          │  │              │  │
│  │ Always   │  │ 30-day   │  │ Turso DB     │  │
│  │ loaded   │  │ on-device│  │ query-only   │  │
│  └──────────┘  └──────────┘  └──────────────┘  │
│                                                 │
│  ┌─────────────────────────────────────────┐   │
│  │         TREE INDEX (~2KB)               │   │
│  │  Hierarchical navigation + search       │   │
│  └─────────────────────────────────────────┘   │
└─────────────────────────────────────────────────┘
```

## Quick Start

```go
package main

import (
    "context"
    "log/slog"
    
    "github.com/clawinfra/evoclaw/internal/memory"
)

func main() {
    // Configure memory system
    cfg := memory.DefaultMemoryConfig()
    cfg.AgentID = "agent-001"
    cfg.AgentName = "Bloop"
    cfg.OwnerName = "Maggie"
    cfg.DatabaseURL = "libsql://your-db.turso.io"
    cfg.AuthToken = "your-token"
    
    // Create manager
    mgr, err := memory.NewManager(cfg, slog.Default())
    if err != nil {
        log.Fatal(err)
    }
    
    // Start background tasks
    ctx := context.Background()
    if err := mgr.Start(ctx); err != nil {
        log.Fatal(err)
    }
    defer mgr.Stop()
    
    // Process a conversation
    conv := memory.RawConversation{
        Timestamp: time.Now(),
        Messages: []memory.Message{
            {Role: "user", Content: "Let's work on the garden today"},
            {Role: "agent", Content: "Great idea!"},
        },
    }
    
    err = mgr.ProcessConversation(ctx, conv, "projects/garden", 0.8)
    if err != nil {
        log.Fatal(err)
    }
    
    // Retrieve relevant memories
    results, err := mgr.Retrieve(ctx, "what are we doing with the garden?", 5)
    if err != nil {
        log.Fatal(err)
    }
    
    for _, memory := range results {
        log.Printf("Found: %s (category: %s)", memory.Content.Fact, memory.Category)
    }
    
    // Access hot memory (always in context)
    hot := mgr.GetHotMemory()
    log.Printf("Agent: %s, Owner: %s", hot.Identity.AgentName, hot.Identity.OwnerName)
    
    // Add a critical lesson
    mgr.AddLesson("Always ask before making changes", "communication", 0.9)
    
    // Update owner profile
    personality := "loves gardening, dry humor"
    mgr.UpdateOwnerProfile(&personality, nil, nil, nil)
}
```

## Components

### Manager (`manager.go`)
Public API coordinating all tiers.

**Key Methods:**
- `NewManager(cfg, logger)` - Create memory system
- `Start(ctx)` - Initialize and start background tasks
- `ProcessConversation(ctx, conv, category, importance)` - Store conversation
- `Retrieve(ctx, query, maxResults)` - Find relevant memories
- `GetHotMemory()` - Access core memory
- `GetTree()` - Access tree index
- `GetStats(ctx)` - Memory statistics
- `AddLesson(text, category, importance)` - Add critical lesson
- `UpdateOwnerProfile(...)` - Update owner info
- `AddProject(name, description)` - Track active project

### Tree Index (`tree.go`, `tree_search.go`)
Hierarchical memory organization with reasoning-based retrieval.

**Constraints:**
- Max 50 nodes
- Max depth 4
- Max 2KB serialized
- Max 10 children per node
- Max 100 chars per summary

**Operations:**
- `AddNode(path, summary)` - Create category node
- `RemoveNode(path)` - Delete node and children
- `FindNode(path)` - Lookup by path
- `UpdateNode(path, summary, warmCount, coldCount)` - Update metadata
- `IncrementCounts(path, warmDelta, coldDelta)` - Adjust counts
- `PruneDeadNodes(maxAgeDays)` - Remove unused nodes

**Search:**
- Keyword/topic matching (rule-based for now)
- Scores by: keyword overlap + recency + importance
- Ready for LLM-powered search later

### Hot Memory (`hot.go`)
Core memory (5KB max), always in context.

**Structure:**
- `Identity` - Agent/owner info, trust level
- `OwnerProfile` - Personality, family, preferences, schedule
- `ActiveContext` - Current projects, recent events, pending tasks
- `CriticalLessons` - Max 20 high-importance lessons

**Auto-pruning:**
- Lessons: removes lowest-importance when at capacity
- Events: keeps last 10 only
- Tasks: max 10 pending
- Enforces 5KB size limit by progressively pruning

### Warm Memory (`warm.go`)
Recent facts (50KB cap, 30-day retention).

**Features:**
- In-memory storage (ready for SQLite backend)
- Score-based eviction: `score = importance × recency × reinforcement`
- Access count tracking (reinforcement learning)
- Automatic archival to cold tier

**Eviction Triggers:**
- Score below threshold (default: 0.3)
- Age exceeds retention (default: 30 days)
- Size exceeds capacity (default: 50KB)

### Cold Memory (`cold.go`)
Unlimited archive in Turso (libSQL cloud).

**Features:**
- Zero CGO (uses Turso HTTP API)
- Query by: category, importance, time range
- Access tracking for reinforcement
- Frozen entry cleanup (score < 0.05, age > retention)

**Schema:**
```sql
CREATE TABLE cold_memory (
  id TEXT PRIMARY KEY,
  agent_id TEXT NOT NULL,
  timestamp INTEGER NOT NULL,
  event_type TEXT NOT NULL,
  category TEXT NOT NULL,
  content TEXT NOT NULL,
  distilled_summary TEXT NOT NULL,
  importance REAL DEFAULT 0.5,
  access_count INTEGER DEFAULT 0,
  last_accessed INTEGER,
  created_at INTEGER NOT NULL
);
```

### Distiller (`distiller.go`)
Three-stage compression:

1. **Raw → Distilled (Stage 1 → 2)**
   - Extract: fact, emotion, people, topics, actions, outcome
   - Target: <100 bytes JSON
   - Rule-based for now (ready for LLM later)

2. **Distilled → Core Summary (Stage 2 → 3)**
   - One-line summary
   - Target: <30 bytes
   - Used in tree index

**Compression ratio:** 20-50x from raw to core

### Scorer (`scorer.go`)
Relevance scoring with decay and reinforcement.

**Formula:**
```
score = importance × recency_decay(age) × (1 + 0.1 × access_count)

recency_decay(age) = exp(-age_days / half_life)
```

**Tier Thresholds:**
- `score >= 0.7` → Hot
- `score >= 0.3` → Warm
- `score >= 0.05` → Cold
- `score < 0.05` → Frozen (deleted after retention period)

### Consolidator (`consolidator.go`)
Background maintenance tasks.

**Tasks:**
- **Warm Eviction** (hourly): Archive expired entries to cold
- **Tree Prune** (daily): Remove dead nodes (zero counts, 60+ days old)
- **Tree Rebuild** (monthly): LLM-powered restructuring (stub for now)
- **Cold Cleanup** (monthly): Delete frozen entries beyond retention

**Manual Triggers:**
- `TriggerWarmEviction(ctx)`
- `TriggerTreePrune()`
- `TriggerColdCleanup(ctx)`

## Configuration

```go
type MemoryConfig struct {
    Enabled    bool
    AgentID    string
    AgentName  string
    OwnerName  string
    
    // Turso connection
    DatabaseURL string
    AuthToken   string
    
    // Tree settings
    TreeMaxNodes    int     // default: 50
    TreeMaxDepth    int     // default: 4
    TreeRebuildDays int     // default: 30
    
    // Hot tier
    HotMaxBytes   int  // default: 5120 (5KB)
    HotMaxLessons int  // default: 20
    
    // Warm tier
    WarmMaxKB             int     // default: 50
    WarmRetentionDays     int     // default: 30
    WarmEvictionThreshold float64 // default: 0.3
    
    // Cold tier
    ColdRetentionYears int  // default: 10
    
    // Distillation
    DistillationAggression float64 // 0-1, default: 0.7
    MaxDistilledBytes      int     // default: 100
    
    // Scoring
    HalfLifeDays       float64  // default: 30.0
    ReinforcementBoost float64  // default: 0.1
    
    // Consolidation
    Consolidation ConsolidationConfig
}
```

## Integration with Orchestrator

Add to your agent config:

```json
{
  "memory": {
    "enabled": true,
    "tree": {
      "maxNodes": 50,
      "maxDepth": 4,
      "rebuildIntervalDays": 30
    },
    "hot": {
      "maxSizeBytes": 5120,
      "maxLessons": 20
    },
    "warm": {
      "maxSizeKb": 50,
      "retentionDays": 30,
      "evictionThreshold": 0.3
    },
    "cold": {
      "backend": "turso",
      "databaseUrl": "libsql://your-db.turso.io",
      "authToken": "ENV:TURSO_AUTH_TOKEN",
      "retentionYears": 10
    },
    "distillation": {
      "aggression": 0.7
    },
    "scoring": {
      "halfLifeDays": 30.0,
      "reinforcementBoost": 0.1
    }
  }
}
```

## Testing

```bash
# Run all tests
go test ./internal/memory/... -v

# Run specific test suites
go test ./internal/memory -run TestTree -v
go test ./internal/memory -run TestScorer -v
go test ./internal/memory -run TestWarm -v

# Build check
go build ./internal/memory/...
```

## Performance Characteristics

**Context Size:**
- Hot: ~5KB (always loaded)
- Tree: ~2KB (always loaded)
- Retrieved memories: ~1-3KB per query
- **Total per session: ~8-15KB** (constant, regardless of agent age)

**Scaling:**
- Hot: O(1) access
- Warm: O(n) scan, but n ≤ ~500 entries
- Cold: O(log n) indexed queries
- Tree search: O(log n) traversal

**5-Year Scenario:**
- Hot: Still 5KB
- Warm: Last 30 days (~50KB)
- Cold: ~50MB in Turso (30KB/day × 1825 days)
- Tree: Still 2KB, different nodes
- **Context: Same as day 1**

## Next Steps

### LLM Integration (Not Yet Implemented)
1. **Tree Rebuild:** Monthly LLM pass to restructure based on access patterns
2. **Distillation:** Replace rule-based with LLM-powered compression
3. **Tree Search:** Use LLM reasoning instead of keyword matching

Example future tree search:
```go
// Instead of keyword matching, use LLM to reason about relevance
prompt := fmt.Sprintf(`
Given this memory tree:
%s

User query: %s

Which categories are relevant? Return paths only.
`, tree.Serialize(), query)

paths := llm.Complete(prompt)
```

### Evolution Integration
The evolution engine can tune memory parameters:
- Increase `distillationAggression` under storage pressure
- Adjust `warmRetentionDays` based on access patterns
- Tune `evictionThreshold` for retrieval accuracy

### Cross-Agent Memory (Future)
- Shared tree nodes for household facts
- Private nodes for personal relationships
- Consent-based memory sharing

## References

- Design doc: `/docs/TIERED-MEMORY.md`
- Inspiration: [PageIndex](https://github.com/VectifyAI/PageIndex) (tree-based retrieval)
- Cloud sync: `/internal/cloudsync/` (Turso HTTP client)
