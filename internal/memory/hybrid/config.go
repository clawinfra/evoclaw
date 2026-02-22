// Package hybrid provides a SQLite FTS5 + vector hybrid search layer.
package hybrid

// Config holds hybrid search configuration.
type Config struct {
	// DBPath is the SQLite database file path. Use ":memory:" for in-memory.
	DBPath string
	// VectorWeight is the weight for vector similarity results (default 0.7).
	VectorWeight float64
	// KeywordWeight is the weight for keyword/FTS5 results (default 0.3).
	KeywordWeight float64
	// EmbeddingProvider selects the embedding backend: "none" or "local".
	EmbeddingProvider string
	// ChunkSize is the target chunk size in characters (default 512).
	ChunkSize int
	// ChunkOverlap is the overlap between chunks in characters (default 50).
	ChunkOverlap int
	// CacheSize is the max number of LRU cache entries for embeddings (default 128).
	CacheSize int
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		DBPath:            "hybrid_search.db",
		VectorWeight:      0.7,
		KeywordWeight:     0.3,
		EmbeddingProvider: "none",
		ChunkSize:         512,
		ChunkOverlap:      50,
		CacheSize:         128,
	}
}

// validate fills zero-value fields with defaults.
func (c *Config) validate() {
	if c.VectorWeight == 0 && c.KeywordWeight == 0 {
		c.VectorWeight = 0.7
		c.KeywordWeight = 0.3
	}
	if c.ChunkSize <= 0 {
		c.ChunkSize = 512
	}
	if c.ChunkOverlap < 0 {
		c.ChunkOverlap = 50
	}
	if c.CacheSize <= 0 {
		c.CacheSize = 128
	}
}
