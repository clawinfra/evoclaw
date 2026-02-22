# Memory System — Hybrid Search Layer

## Overview

The hybrid search layer (`internal/memory/hybrid`) combines **SQLite FTS5 keyword search** with **vector similarity search** in a single, zero-external-dependency package. It uses `modernc.org/sqlite` (pure Go, no CGO required).

## Architecture

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
