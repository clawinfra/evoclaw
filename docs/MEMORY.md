# Memory System

## Overview

EvoClaw's memory system has **two layers** that work together:

1. **Tiered Memory** — LLM-powered tree-based retrieval across hot/warm/cold tiers (semantic understanding)
2. **Hybrid Search** — SQLite FTS5 + vector search for fast keyword/similarity lookups (speed)

Neither replaces the other. They serve different retrieval patterns.

## Full Architecture

```
┌──────────────────────────────────────────────────────┐
│                    Agent Query                        │
├──────────────────────┬───────────────────────────────┤
│   Tiered Memory      │      Hybrid Search            │
│   (tree navigation)  │      (SQLite FTS5 + vector)   │
│                      │                               │
│  ┌──────────────┐    │    ┌─────────────┐            │
│  │  Hot Tier    │    │    │   Store     │ ← MemoryBackend│
│  │  (today/     │    │    ├─────────────┤            │
│  │   yesterday) │    │    │  Chunker    │            │
│  ├──────────────┤    │    ├──────┬──────┤            │
│  │  Warm Tier   │    │    │ FTS5 │Vector│            │
│  │  (this week/ │    │    ├──────┴──────┤            │
│  │   month)     │    │    │   Merger    │            │
│  ├──────────────┤    │    ├─────────────┤            │
│  │  Cold Tier   │    │    │   SQLite    │            │
│  │  (archive)   │    │    └─────────────┘            │
│  └──────────────┘    │                               │
│         │            │            │                   │
│    Tree Index        │    Keyword + Vector Index      │
│  (LLM-navigated)     │    (BM25 + cosine similarity)  │
└──────────┬───────────┴────────────┬──────────────────┘
           │                        │
           └────────┬───────────────┘
                    ▼
            Merged Results
         (deduplicated, ranked)
```

## When Each Layer Is Used

| Query Type | Layer | Why |
|---|---|---|
| "What did we decide about X?" | **Tiered Memory** | Requires semantic understanding of decisions, context |
| "Find all mentions of rate_limit" | **Hybrid Search** | Fast keyword match across all stored content |
| "What happened with the GPU server?" | **Both** | Tiered finds relevant context, hybrid finds specific logs |
| "Recent project updates" | **Tiered Memory** | Tree navigates by time + topic categories |
| "Error message: connection refused" | **Hybrid Search** | Exact string matching via FTS5 |

### How They Work Together

1. **Daily notes ingestion** → Both layers index the same source material:
   - Tiered memory consolidation jobs (quick/daily/monthly) organize notes into hot → warm → cold tiers with a tree index
   - Hybrid search indexes the raw text into SQLite FTS5 + vector embeddings

2. **Query routing** — The orchestrator can:
   - Use hybrid search for fast, precise lookups (keyword matches, exact errors)
   - Use tiered memory for complex semantic queries (reasoning about past decisions)
   - Combine both: hybrid search for candidates, tiered memory for ranking/context

3. **No duplication of source data** — Both layers point to the same underlying content (daily notes, memory files). They differ only in *how* they index and retrieve.

## Tiered Memory

The tiered memory system uses **LLM-powered tree navigation** for retrieval:

- **O(log n) search** — navigates category tree, doesn't scan everything
- **Explainable** — every result traces a reasoning path through the tree
- **No embeddings required** — pure reasoning-based retrieval
- **Context-aware** — understands "recent project updates" vs "old architecture decisions"

### Tiers

| Tier | Content | Retention | Access Pattern |
|---|---|---|---|
| **Hot** | Today + yesterday's notes | 48h | Read every session |
| **Warm** | This week/month, consolidated | 30 days | Tree-navigated on demand |
| **Cold** | Archive, compressed | Indefinite | Tree-navigated, rarely accessed |

### Consolidation

Automated jobs move data through tiers:
- **Quick** (every few hours) — ingest new daily notes into hot tier
- **Daily** — consolidate hot → warm, extract key facts
- **Monthly** — consolidate warm → cold, compress

## Hybrid Search Layer

The hybrid search layer (`internal/memory/hybrid`) adds **fast keyword + vector retrieval** as a complement to tiered memory. Zero external dependencies, pure Go.

```
┌─────────────┐
│   Store     │  ← Main entry point (implements MemoryBackend)
├─────────────┤
│  Chunker    │  ← Splits documents into overlapping chunks
├──────┬──────┤
│ FTS5 │Vector│  ← Dual search backends
├──────┴──────┤
│   Merger    │  ← Weighted result combination + dedup
├─────────────┤
│   SQLite    │  ← Single DB file, WAL mode
└─────────────┘
```

### Search Flow

1. **Ingest**: Document → Chunker (split with heading context) → FTS5 index + vector embeddings stored as BLOBs
2. **Query**: Query → FTS5 BM25 search + cosine similarity search → Merger (weighted combine, dedup, normalize) → Ranked results

## Configuration

```go
hybrid.Config{
    DBPath:            "hybrid_search.db", // SQLite file path (":memory:" for testing)
    VectorWeight:      0.7,                // Weight for vector results
    KeywordWeight:     0.3,                // Weight for keyword results
    EmbeddingProvider: "none",             // "none" or "local"
    ChunkSize:         512,                // Target chunk size (chars)
    ChunkOverlap:      50,                 // Overlap between chunks (chars)
    CacheSize:         128,                // LRU embedding cache entries
}
```

## Usage

```go
import "github.com/clawinfra/evoclaw/internal/memory/hybrid"

store, err := hybrid.New(hybrid.Config{DBPath: "memory.db"})
defer store.Close()

// Index a document
store.Store(ctx, "doc-id", "# Title\nContent here...", nil)

// Search (combines keyword + vector)
results, _ := store.Search(ctx, "search query", 10)
for _, r := range results {
    fmt.Printf("[%s] %s (score: %.3f)\n", r.Source, r.Text, r.Score)
}
```

## Components

| File | Purpose |
|------|---------|
| `config.go` | Configuration with defaults |
| `store.go` | Core store, migration, MemoryBackend interface |
| `fts.go` | FTS5 index, BM25 search, snippets, reindex |
| `vector.go` | Embedding encode/decode, cosine similarity, LRU cache |
| `merger.go` | Weighted merge, score normalization, deduplication |
| `chunker.go` | Markdown-aware text chunking with heading preservation |

## Embedding Providers

- **`none`** (default): Disables vector search; keyword-only mode
- **`local`**: Reserved for future local embedding models
- Custom: Implement the `EmbeddingProvider` interface

## Design Decisions

- **Pure Go SQLite** (`modernc.org/sqlite`): No CGO dependency, cross-compiles easily
- **WAL mode**: Better concurrent read performance
- **Standalone FTS5**: Not content-synced, for simpler insert/delete semantics
- **LRU embedding cache**: Avoids redundant embedding computation
- **Heading preservation**: Chunks retain their markdown section context for better retrieval
