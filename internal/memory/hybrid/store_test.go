package hybrid

import (
	"context"
	"testing"
)

// mockEmbedder returns deterministic embeddings based on text content.
type mockEmbedder struct {
	dims int
}

func (m *mockEmbedder) Embed(text string) ([]float64, error) {
	// Simple hash-based embedding for testing
	emb := make([]float64, m.dims)
	for i, c := range text {
		emb[i%m.dims] += float64(c) / 1000.0
	}
	// Normalize
	var norm float64
	for _, v := range emb {
		norm += v * v
	}
	if norm > 0 {
		for i := range emb {
			emb[i] /= norm
		}
	}
	return emb, nil
}

func (m *mockEmbedder) Dims() int { return m.dims }

func newTestStore(t *testing.T) *Store {
	t.Helper()
	cfg := Config{
		DBPath:        ":memory:",
		VectorWeight:  0.7,
		KeywordWeight: 0.3,
		ChunkSize:     100,
		ChunkOverlap:  10,
		CacheSize:     16,
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func newTestStoreWithEmbedder(t *testing.T) *Store {
	t.Helper()
	s := newTestStore(t)
	s.embedder = &mockEmbedder{dims: 8}
	return s
}

func TestStoreAndKeywordSearch(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	err := s.Store(ctx, "doc1", "The quick brown fox jumps over the lazy dog", nil)
	if err != nil {
		t.Fatalf("store: %v", err)
	}

	results, err := s.Search(ctx, "quick fox", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results for keyword search")
	}
	if results[0].DocID != "doc1" {
		t.Errorf("expected doc1, got %s", results[0].DocID)
	}
}

func TestStoreAndVectorSearch(t *testing.T) {
	s := newTestStoreWithEmbedder(t)
	ctx := context.Background()

	err := s.Store(ctx, "doc1", "machine learning algorithms and neural networks", nil)
	if err != nil {
		t.Fatalf("store: %v", err)
	}
	err = s.Store(ctx, "doc2", "cooking recipes for Italian pasta dishes", nil)
	if err != nil {
		t.Fatalf("store: %v", err)
	}

	results, err := s.Search(ctx, "machine learning neural", 5)
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected results")
	}
	// doc1 should be more relevant
	foundDoc1First := false
	for _, r := range results {
		if r.DocID == "doc1" {
			foundDoc1First = true
			break
		}
		if r.DocID == "doc2" {
			break
		}
	}
	if !foundDoc1First {
		t.Error("expected doc1 to rank higher than doc2")
	}
}

func TestHybridMergeScoring(t *testing.T) {
	kw := []SearchResult{
		{DocID: "d1", ChunkID: "c1", Text: "hello", Score: 5.0, Source: "keyword"},
		{DocID: "d2", ChunkID: "c2", Text: "world", Score: 3.0, Source: "keyword"},
	}
	vec := []SearchResult{
		{DocID: "d1", ChunkID: "c1", Text: "hello", Score: 0.9, Source: "vector"},
		{DocID: "d3", ChunkID: "c3", Text: "other", Score: 0.5, Source: "vector"},
	}

	merged := MergeResults(kw, vec, 0.3, 0.7)
	if len(merged) == 0 {
		t.Fatal("expected merged results")
	}

	// d1:c1 should be hybrid (appears in both)
	found := false
	for _, r := range merged {
		if r.DocID == "d1" && r.ChunkID == "c1" {
			if r.Source != "hybrid" {
				t.Errorf("expected hybrid source, got %s", r.Source)
			}
			found = true
		}
	}
	if !found {
		t.Error("expected d1:c1 in merged results")
	}
}

func TestChunkingWithHeadings(t *testing.T) {
	text := `# Introduction
This is the intro paragraph with some text.

## Section One
Content of section one goes here with details.

## Section Two
Content of section two with more details.`

	chunks := ChunkText(text, 80, 10)
	if len(chunks) == 0 {
		t.Fatal("expected chunks")
	}

	// Verify heading preservation
	foundSection := false
	for _, c := range chunks {
		if c.Heading == "## Section One" || c.Heading == "## Section Two" {
			foundSection = true
		}
	}
	if !foundSection {
		t.Error("expected chunks with section headings")
	}
}

func TestChunkingEmpty(t *testing.T) {
	chunks := ChunkText("", 100, 10)
	if chunks != nil {
		t.Error("expected nil for empty text")
	}
}

func TestCacheEviction(t *testing.T) {
	cache := NewEmbeddingCache(3)

	cache.Put("a", []float64{1})
	cache.Put("b", []float64{2})
	cache.Put("c", []float64{3})
	cache.Put("d", []float64{4}) // evicts "a"

	if cache.Get("a") != nil {
		t.Error("expected 'a' to be evicted")
	}
	if cache.Get("d") == nil {
		t.Error("expected 'd' to exist")
	}
	if cache.Len() != 3 {
		t.Errorf("expected len 3, got %d", cache.Len())
	}
}

func TestCacheUpdate(t *testing.T) {
	cache := NewEmbeddingCache(3)
	cache.Put("a", []float64{1})
	cache.Put("a", []float64{2})

	got := cache.Get("a")
	if len(got) != 1 || got[0] != 2 {
		t.Errorf("expected updated value [2], got %v", got)
	}
	if cache.Len() != 1 {
		t.Errorf("expected len 1, got %d", cache.Len())
	}
}

func TestCosineSimilarity(t *testing.T) {
	a := []float64{1, 0, 0}
	b := []float64{1, 0, 0}
	if s := CosineSimilarity(a, b); s < 0.999 {
		t.Errorf("identical vectors should have similarity ~1, got %f", s)
	}

	c := []float64{0, 1, 0}
	if s := CosineSimilarity(a, c); s > 0.001 {
		t.Errorf("orthogonal vectors should have similarity ~0, got %f", s)
	}

	// Edge cases
	if s := CosineSimilarity(nil, nil); s != 0 {
		t.Errorf("nil vectors should return 0, got %f", s)
	}
	if s := CosineSimilarity([]float64{0, 0}, []float64{0, 0}); s != 0 {
		t.Errorf("zero vectors should return 0, got %f", s)
	}
}

func TestEncodeDecodeEmbedding(t *testing.T) {
	original := []float64{1.5, -2.3, 0.0, 99.99}
	encoded := EncodeEmbedding(original)
	decoded := DecodeEmbedding(encoded)

	if len(decoded) != len(original) {
		t.Fatalf("length mismatch: %d vs %d", len(decoded), len(original))
	}
	for i := range original {
		if decoded[i] != original[i] {
			t.Errorf("index %d: %f != %f", i, decoded[i], original[i])
		}
	}
}

func TestNormalizeScores(t *testing.T) {
	results := []SearchResult{
		{Score: 10},
		{Score: 5},
		{Score: 2},
	}
	normalizeScores(results)
	if results[0].Score != 1.0 {
		t.Errorf("max should be 1.0, got %f", results[0].Score)
	}
	if results[1].Score != 0.5 {
		t.Errorf("expected 0.5, got %f", results[1].Score)
	}
}

func TestNormalizeScoresEmpty(t *testing.T) {
	normalizeScores(nil) // should not panic
	normalizeScores([]SearchResult{{Score: 0}})
}

func TestConfigDefaults(t *testing.T) {
	cfg := Config{}
	cfg.validate()
	if cfg.VectorWeight != 0.7 {
		t.Errorf("expected 0.7, got %f", cfg.VectorWeight)
	}
	if cfg.ChunkSize != 512 {
		t.Errorf("expected 512, got %d", cfg.ChunkSize)
	}
	if cfg.CacheSize != 128 {
		t.Errorf("expected 128, got %d", cfg.CacheSize)
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.DBPath != "hybrid_search.db" {
		t.Error("unexpected default DBPath")
	}
}

func TestStoreReplace(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_ = s.Store(ctx, "doc1", "original content here", nil)
	_ = s.Store(ctx, "doc1", "replaced content now", nil)

	results, _ := s.Search(ctx, "replaced", 5)
	if len(results) == 0 {
		t.Fatal("expected results after replace")
	}

	// Original should not match
	results2, _ := s.Search(ctx, "original", 5)
	if len(results2) > 0 {
		t.Error("original content should be gone after replace")
	}
}

func TestFTSReindex(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_ = s.Store(ctx, "doc1", "reindex test content for searching", nil)
	err := s.FTS().Reindex()
	if err != nil {
		t.Fatalf("reindex: %v", err)
	}

	results, _ := s.Search(ctx, "reindex", 5)
	if len(results) == 0 {
		t.Fatal("expected results after reindex")
	}
}

func TestFTSSnippet(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	_ = s.Store(ctx, "doc1", "The quick brown fox jumps over the lazy dog", nil)
	results, err := s.FTS().Snippet("quick", 5)
	if err != nil {
		t.Fatalf("snippet: %v", err)
	}
	if len(results) == 0 {
		t.Fatal("expected snippet results")
	}
}

func TestStoreSearchEmptyDB(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	results, err := s.Search(ctx, "anything", 5)
	if err != nil {
		t.Fatalf("search empty: %v", err)
	}
	if len(results) != 0 {
		t.Error("expected no results from empty db")
	}
}

func TestStoreEmptyText(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()

	err := s.Store(ctx, "doc1", "", nil)
	if err != nil {
		t.Fatalf("store empty: %v", err)
	}
}

func TestMergeResultsEmpty(t *testing.T) {
	merged := MergeResults(nil, nil, 0.3, 0.7)
	if len(merged) != 0 {
		t.Error("expected empty merge")
	}
}

func TestChunkOverlapZero(t *testing.T) {
	chunks := ChunkText("line one\nline two\nline three", 15, 0)
	if len(chunks) == 0 {
		t.Fatal("expected chunks")
	}
}

func TestChunkBadParams(t *testing.T) {
	chunks := ChunkText("hello", -1, -1)
	if len(chunks) == 0 {
		t.Fatal("expected chunks with bad params")
	}
}

func TestNoopEmbedder(t *testing.T) {
	e := &NoopEmbedder{}
	emb, err := e.Embed("test")
	if err != nil || emb != nil {
		t.Error("noop should return nil, nil")
	}
	if e.Dims() != 0 {
		t.Error("noop dims should be 0")
	}
}

func TestMemoryBackendInterface(t *testing.T) {
	// Verify Store implements MemoryBackend
	var _ MemoryBackend = (*Store)(nil)
}
