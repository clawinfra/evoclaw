package hybrid

import (
	"math"
	"testing"
)

func TestLocalHashEmbedder_Deterministic(t *testing.T) {
	e := NewLocalHashEmbedder(0)
	a, _ := e.Embed("the quick brown fox")
	b, _ := e.Embed("the quick brown fox")
	if len(a) != 256 || len(b) != 256 {
		t.Fatalf("expected 256 dims, got %d and %d", len(a), len(b))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("embedding not deterministic at dim %d: %v vs %v", i, a[i], b[i])
		}
	}
}

func TestLocalHashEmbedder_Normalized(t *testing.T) {
	e := NewLocalHashEmbedder(0)
	v, _ := e.Embed("network and service management")
	var norm float64
	for _, x := range v {
		norm += x * x
	}
	if math.Abs(math.Sqrt(norm)-1.0) > 1e-9 {
		t.Errorf("expected unit L2 norm, got %f", math.Sqrt(norm))
	}
}

func TestLocalHashEmbedder_EmptyIsNil(t *testing.T) {
	e := NewLocalHashEmbedder(0)
	if v, _ := e.Embed("   !!!  "); v != nil {
		t.Errorf("expected nil embedding for token-free input, got len %d", len(v))
	}
}

// TestLocalHashEmbedder_SemanticOrdering verifies that lexically related texts
// score higher cosine similarity than unrelated ones — the property the hybrid
// vector path depends on.
func TestLocalHashEmbedder_SemanticOrdering(t *testing.T) {
	e := NewLocalHashEmbedder(0)
	q, _ := e.Embed("autonomous network fault recovery")
	related, _ := e.Embed("the system recovers automatically from a network fault")
	unrelated, _ := e.Embed("a recipe for chocolate chip cookies")

	simRelated := CosineSimilarity(q, related)
	simUnrelated := CosineSimilarity(q, unrelated)

	if simRelated <= simUnrelated {
		t.Errorf("expected related text to score higher: related=%.3f unrelated=%.3f", simRelated, simUnrelated)
	}
}

// TestStore_LocalEmbedder_VectorPathActive confirms that, with the local embedder,
// the store's vector path returns results (previously a no-op).
func TestStore_LocalEmbedder_VectorPathActive(t *testing.T) {
	cfg := Config{
		DBPath:            ":memory:",
		EmbeddingProvider: "local",
		ChunkSize:         100,
		ChunkOverlap:      10,
		CacheSize:         16,
	}
	s, err := New(cfg)
	if err != nil {
		t.Fatalf("new store: %v", err)
	}
	defer func() { _ = s.Close() }()

	if _, ok := s.embedder.(*LocalHashEmbedder); !ok {
		t.Fatalf("expected LocalHashEmbedder, got %T", s.embedder)
	}

	ctx := t.Context()
	if err := s.Store(ctx, "d1", "autonomous service management with runtime adaptation", nil); err != nil {
		t.Fatalf("store: %v", err)
	}
	// A query with no lexical overlap so any hit must come from the vector path.
	vec, err := s.vectorSearch(ctx, "runtime adaptation autonomous", 5)
	if err != nil {
		t.Fatalf("vector search: %v", err)
	}
	if len(vec) == 0 {
		t.Error("expected vector search to return results with the local embedder")
	}
}
