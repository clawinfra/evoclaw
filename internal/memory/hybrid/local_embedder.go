package hybrid

import (
	"hash/fnv"
	"math"
	"strings"
	"unicode"
)

// LocalHashEmbedder is a dependency-free, deterministic text embedder based on
// the hashing trick (feature hashing). It maps token unigrams and bigrams into a
// fixed-dimensional, L2-normalized vector using signed FNV-1a hashing. It needs no
// model download, no network, and no GPU — matching EvoClaw's resource-frugal,
// edge-first design — while making the hybrid store's vector-similarity path
// functional and reproducible offline.
//
// It is not a neural embedding: it captures lexical overlap (including local word
// order via bigrams), not deep semantics. Deployments needing semantic embeddings
// can supply a model-backed EmbeddingProvider instead.
type LocalHashEmbedder struct {
	dims int
}

// NewLocalHashEmbedder creates a LocalHashEmbedder with the given dimensionality
// (dims <= 0 defaults to 256).
func NewLocalHashEmbedder(dims int) *LocalHashEmbedder {
	if dims <= 0 {
		dims = 256
	}
	return &LocalHashEmbedder{dims: dims}
}

// Dims returns the embedding dimensionality.
func (e *LocalHashEmbedder) Dims() int { return e.dims }

// Embed returns an L2-normalized feature-hashed embedding of text. Empty or
// token-free input yields a zero-length slice (which disables vector scoring for
// that item), consistent with the EmbeddingProvider contract.
func (e *LocalHashEmbedder) Embed(text string) ([]float64, error) {
	tokens := splitTokens(text)
	if len(tokens) == 0 {
		return nil, nil
	}

	vec := make([]float64, e.dims)
	add := func(feature string) {
		h := fnv.New64a()
		_, _ = h.Write([]byte(feature))
		sum := h.Sum64()
		idx := int(sum % uint64(e.dims))
		// A sign bit drawn from the high bit of the hash reduces the systematic
		// bias that unsigned feature hashing introduces on collisions.
		if sum&(1<<63) != 0 {
			vec[idx] -= 1.0
		} else {
			vec[idx] += 1.0
		}
	}

	for i, tok := range tokens {
		add(tok) // unigram
		if i+1 < len(tokens) {
			add(tok + " " + tokens[i+1]) // bigram captures local word order
		}
	}

	var norm float64
	for _, v := range vec {
		norm += v * v
	}
	if norm == 0 {
		return nil, nil
	}
	norm = math.Sqrt(norm)
	for i := range vec {
		vec[i] /= norm
	}
	return vec, nil
}

// splitTokens lowercases text and splits it into alphanumeric tokens.
func splitTokens(text string) []string {
	return strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsNumber(r)
	})
}
