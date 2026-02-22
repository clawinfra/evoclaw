package hybrid

import (
	"context"
	"database/sql"
	"fmt"
	"sync"

	_ "modernc.org/sqlite"
)

// MemoryBackend is the interface for pluggable memory storage backends.
type MemoryBackend interface {
	Store(ctx context.Context, docID, text string, metadata map[string]string) error
	Search(ctx context.Context, query string, limit int) ([]SearchResult, error)
	Close() error
}

// Store is the hybrid search store combining FTS5 keyword search with vector similarity.
type Store struct {
	db       *sql.DB
	fts      *FTSIndex
	embedder EmbeddingProvider
	cache    *EmbeddingCache
	cfg      Config
	mu       sync.RWMutex
}

// New creates a new hybrid Store with the given config.
func New(cfg Config) (*Store, error) {
	cfg.validate()

	db, err := sql.Open("sqlite", cfg.DBPath)
	if err != nil {
		return nil, fmt.Errorf("hybrid: open db: %w", err)
	}

	// Enable WAL mode for better concurrency
	if _, err := db.Exec(`PRAGMA journal_mode=WAL`); err != nil {
		db.Close()
		return nil, fmt.Errorf("hybrid: wal mode: %w", err)
	}

	s := &Store{
		db:    db,
		fts:   NewFTSIndex(db),
		cache: NewEmbeddingCache(cfg.CacheSize),
		cfg:   cfg,
	}

	// Select embedding provider
	switch cfg.EmbeddingProvider {
	case "none", "":
		s.embedder = &NoopEmbedder{}
	default:
		s.embedder = &NoopEmbedder{}
	}

	if err := s.migrate(); err != nil {
		db.Close()
		return nil, fmt.Errorf("hybrid: migrate: %w", err)
	}

	return s, nil
}

// migrate creates tables on first run.
func (s *Store) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS chunks (
			doc_id   TEXT NOT NULL,
			chunk_id TEXT NOT NULL,
			heading  TEXT NOT NULL DEFAULT '',
			content  TEXT NOT NULL,
			embedding BLOB,
			PRIMARY KEY (doc_id, chunk_id)
		)`,
		`CREATE VIRTUAL TABLE IF NOT EXISTS chunks_fts USING fts5(
			doc_id, chunk_id, heading, content,
			content='chunks',
			content_rowid='rowid'
		)`,
		`CREATE INDEX IF NOT EXISTS idx_chunks_doc ON chunks(doc_id)`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migrate %q: %w", stmt[:40], err)
		}
	}
	return nil
}

// Store indexes a document by chunking, embedding, and inserting into both FTS5 and vector stores.
func (s *Store) Store(ctx context.Context, docID, text string, metadata map[string]string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	chunks := ChunkText(text, s.cfg.ChunkSize, s.cfg.ChunkOverlap)
	if len(chunks) == 0 {
		return nil
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	// Delete existing chunks for this doc
	if _, err := tx.ExecContext(ctx, `DELETE FROM chunks WHERE doc_id = ?`, docID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM chunks_fts WHERE doc_id = ?`, docID); err != nil {
		return err
	}

	for _, chunk := range chunks {
		chunkID := fmt.Sprintf("%s-%d", docID, chunk.Index)

		// Generate embedding
		var embBlob []byte
		if emb, err := s.getEmbedding(chunk.Text); err == nil && len(emb) > 0 {
			embBlob = EncodeEmbedding(emb)
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO chunks(doc_id, chunk_id, heading, content, embedding) VALUES(?, ?, ?, ?, ?)`,
			docID, chunkID, chunk.Heading, chunk.Text, embBlob,
		); err != nil {
			return err
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO chunks_fts(doc_id, chunk_id, heading, content) VALUES(?, ?, ?, ?)`,
			docID, chunkID, chunk.Heading, chunk.Text,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// Search performs hybrid search combining FTS5 keyword and vector similarity results.
func (s *Store) Search(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	if limit <= 0 {
		limit = 10
	}

	// Keyword search via FTS5
	kwResults, err := s.fts.Search(query, limit)
	if err != nil {
		// FTS may fail on special chars; treat as empty
		kwResults = nil
	}

	// Vector search
	vecResults, err := s.vectorSearch(ctx, query, limit)
	if err != nil {
		vecResults = nil
	}

	// Merge
	return MergeResults(kwResults, vecResults, s.cfg.KeywordWeight, s.cfg.VectorWeight), nil
}

// vectorSearch finds chunks by cosine similarity to the query embedding.
func (s *Store) vectorSearch(ctx context.Context, query string, limit int) ([]SearchResult, error) {
	queryEmb, err := s.getEmbedding(query)
	if err != nil || len(queryEmb) == 0 {
		return nil, nil
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT doc_id, chunk_id, heading, content, embedding FROM chunks WHERE embedding IS NOT NULL`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var embBlob []byte
		if err := rows.Scan(&r.DocID, &r.ChunkID, &r.Heading, &r.Text, &embBlob); err != nil {
			return nil, err
		}
		emb := DecodeEmbedding(embBlob)
		r.Score = CosineSimilarity(queryEmb, emb)
		r.Source = "vector"
		if r.Score > 0 {
			results = append(results, r)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sortResults(results)
	if len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

// getEmbedding returns an embedding for text, using cache.
func (s *Store) getEmbedding(text string) ([]float64, error) {
	if cached := s.cache.Get(text); cached != nil {
		return cached, nil
	}
	emb, err := s.embedder.Embed(text)
	if err != nil {
		return nil, err
	}
	if len(emb) > 0 {
		s.cache.Put(text, emb)
	}
	return emb, nil
}

// Close closes the underlying database.
func (s *Store) Close() error {
	return s.db.Close()
}

// FTS returns the FTS index for direct keyword operations.
func (s *Store) FTS() *FTSIndex {
	return s.fts
}

// DB returns the underlying database (for testing/advanced use).
func (s *Store) DB() *sql.DB {
	return s.db
}
