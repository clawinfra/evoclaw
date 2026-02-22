package hybrid

import (
	"container/list"
	"encoding/binary"
	"math"
	"sync"
)

// EmbeddingProvider generates embeddings for text.
type EmbeddingProvider interface {
	Embed(text string) ([]float64, error)
	Dims() int
}

// NoopEmbedder returns zero-length embeddings (disables vector search).
type NoopEmbedder struct{}

func (n *NoopEmbedder) Embed(string) ([]float64, error) { return nil, nil }
func (n *NoopEmbedder) Dims() int                       { return 0 }

// EmbeddingCache is a thread-safe LRU cache for embeddings.
type EmbeddingCache struct {
	mu       sync.Mutex
	capacity int
	items    map[string]*list.Element
	order    *list.List
}

type cacheEntry struct {
	key       string
	embedding []float64
}

// NewEmbeddingCache creates a new LRU embedding cache.
func NewEmbeddingCache(capacity int) *EmbeddingCache {
	if capacity <= 0 {
		capacity = 128
	}
	return &EmbeddingCache{
		capacity: capacity,
		items:    make(map[string]*list.Element),
		order:    list.New(),
	}
}

// Get retrieves an embedding from cache. Returns nil if not found.
func (c *EmbeddingCache) Get(key string) []float64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.order.MoveToFront(el)
		return el.Value.(*cacheEntry).embedding
	}
	return nil
}

// Put stores an embedding in cache with LRU eviction.
func (c *EmbeddingCache) Put(key string, embedding []float64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		c.order.MoveToFront(el)
		el.Value.(*cacheEntry).embedding = embedding
		return
	}
	if c.order.Len() >= c.capacity {
		back := c.order.Back()
		if back != nil {
			c.order.Remove(back)
			delete(c.items, back.Value.(*cacheEntry).key)
		}
	}
	el := c.order.PushFront(&cacheEntry{key: key, embedding: embedding})
	c.items[key] = el
}

// Len returns the number of cached entries.
func (c *EmbeddingCache) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// EncodeEmbedding serializes a float64 slice to bytes.
func EncodeEmbedding(v []float64) []byte {
	buf := make([]byte, 8*len(v))
	for i, f := range v {
		binary.LittleEndian.PutUint64(buf[i*8:], math.Float64bits(f))
	}
	return buf
}

// DecodeEmbedding deserializes bytes to a float64 slice.
func DecodeEmbedding(b []byte) []float64 {
	n := len(b) / 8
	v := make([]float64, n)
	for i := range n {
		v[i] = math.Float64frombits(binary.LittleEndian.Uint64(b[i*8:]))
	}
	return v
}

// CosineSimilarity computes cosine similarity between two vectors.
func CosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += a[i] * b[i]
		normA += a[i] * a[i]
		normB += b[i] * b[i]
	}
	if normA == 0 || normB == 0 {
		return 0
	}
	return dot / (math.Sqrt(normA) * math.Sqrt(normB))
}
