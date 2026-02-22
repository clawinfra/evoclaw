package hybrid

import (
	"database/sql"
	"fmt"
)

// FTSIndex manages FTS5 full-text search operations.
type FTSIndex struct {
	db *sql.DB
}

// NewFTSIndex creates a new FTS index wrapper.
func NewFTSIndex(db *sql.DB) *FTSIndex {
	return &FTSIndex{db: db}
}

// Index inserts or replaces a chunk in the FTS5 index.
func (f *FTSIndex) Index(docID, chunkID, heading, text string) error {
	_, err := f.db.Exec(
		`INSERT OR REPLACE INTO chunks_fts(doc_id, chunk_id, heading, content)
		 VALUES (?, ?, ?, ?)`,
		docID, chunkID, heading, text,
	)
	return err
}

// Search performs BM25-ranked keyword search returning up to limit results.
func (f *FTSIndex) Search(query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := f.db.Query(
		`SELECT doc_id, chunk_id, heading, content, bm25(chunks_fts)
		 FROM chunks_fts
		 WHERE chunks_fts MATCH ?
		 ORDER BY bm25(chunks_fts)
		 LIMIT ?`,
		query, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("fts search: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var bm25Score float64
		if err := rows.Scan(&r.DocID, &r.ChunkID, &r.Heading, &r.Text, &bm25Score); err != nil {
			return nil, err
		}
		// BM25 returns negative scores (lower = better), invert for ranking
		r.Score = -bm25Score
		r.Source = "keyword"
		results = append(results, r)
	}
	return results, rows.Err()
}

// Snippet returns highlighted snippets for a query match.
func (f *FTSIndex) Snippet(query string, limit int) ([]SearchResult, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := f.db.Query(
		`SELECT doc_id, chunk_id, heading,
		        snippet(chunks_fts, 3, '<b>', '</b>', '...', 32),
		        bm25(chunks_fts)
		 FROM chunks_fts
		 WHERE chunks_fts MATCH ?
		 ORDER BY bm25(chunks_fts)
		 LIMIT ?`,
		query, limit,
	)
	if err != nil {
		return nil, fmt.Errorf("fts snippet: %w", err)
	}
	defer rows.Close()

	var results []SearchResult
	for rows.Next() {
		var r SearchResult
		var bm25Score float64
		if err := rows.Scan(&r.DocID, &r.ChunkID, &r.Heading, &r.Text, &bm25Score); err != nil {
			return nil, err
		}
		r.Score = -bm25Score
		r.Source = "keyword"
		results = append(results, r)
	}
	return results, rows.Err()
}

// Reindex rebuilds the FTS5 index atomically from the chunks table.
func (f *FTSIndex) Reindex() error {
	tx, err := f.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM chunks_fts`); err != nil {
		return fmt.Errorf("fts reindex clear: %w", err)
	}
	if _, err := tx.Exec(
		`INSERT INTO chunks_fts(doc_id, chunk_id, heading, content)
		 SELECT doc_id, chunk_id, heading, content FROM chunks`,
	); err != nil {
		return fmt.Errorf("fts reindex insert: %w", err)
	}
	return tx.Commit()
}
