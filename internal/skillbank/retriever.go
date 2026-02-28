package skillbank

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
	"unicode"
)

// NewRetriever returns an EmbeddingRetriever if embeddingURL is non-empty and
// the endpoint is reachable; otherwise it returns a TemplateRetriever.
func NewRetriever(store Store, embeddingURL string) Retriever {
	if embeddingURL != "" {
		er := &EmbeddingRetriever{
			store:        store,
			embeddingURL: strings.TrimRight(embeddingURL, "/"),
			client:       &http.Client{Timeout: 10 * time.Second},
			fallback:     &TemplateRetriever{store: store},
		}
		return er
	}
	return &TemplateRetriever{store: store}
}

// ---------------------------------------------------------------------------
// TemplateRetriever — keyword matching, zero cost
// ---------------------------------------------------------------------------

// TemplateRetriever scores skills by word-overlap ratio against the task description.
type TemplateRetriever struct {
	store Store
}

// Retrieve returns the top-k skills ranked by keyword overlap with taskDescription.
func (r *TemplateRetriever) Retrieve(ctx context.Context, taskDescription string, k int) ([]Skill, error) {
	skills, err := r.store.List("")
	if err != nil {
		return nil, err
	}

	queryTokens := tokenize(taskDescription)
	if len(queryTokens) == 0 || len(skills) == 0 {
		return nil, nil
	}

	type scored struct {
		skill Skill
		score float64
	}

	candidates := make([]scored, 0, len(skills))
	for _, s := range skills {
		score := overlapScore(queryTokens, s)
		if score > 0 {
			candidates = append(candidates, scored{s, score})
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	if k > len(candidates) {
		k = len(candidates)
	}
	out := make([]Skill, k)
	for i := range out {
		out[i] = candidates[i].skill
	}
	return out, nil
}

// overlapScore computes the Jaccard-like word overlap ratio between query tokens and a skill.
func overlapScore(queryTokens map[string]struct{}, s Skill) float64 {
	skillText := strings.ToLower(s.Title + " " + s.Principle + " " + s.WhenToApply + " " + s.TaskType)
	skillTokens := tokenize(skillText)

	if len(skillTokens) == 0 {
		return 0
	}

	var overlap float64
	for t := range queryTokens {
		if _, ok := skillTokens[t]; ok {
			overlap++
		}
	}

	// Score = overlap / sqrt(|query| * |skill|) — geometric mean normalisation
	return overlap / math.Sqrt(float64(len(queryTokens))*float64(len(skillTokens)))
}

// tokenize splits text into a lowercase token set, filtering stop words.
func tokenize(text string) map[string]struct{} {
	tokens := make(map[string]struct{})
	text = strings.ToLower(text)
	fields := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	for _, f := range fields {
		if len(f) > 2 && !stopWords[f] {
			tokens[f] = struct{}{}
		}
	}
	return tokens
}

// stopWords is a small set of common English words excluded from matching.
var stopWords = map[string]bool{
	"the": true, "and": true, "for": true, "that": true, "this": true,
	"with": true, "are": true, "not": true, "but": true, "was": true,
	"you": true, "all": true, "can": true, "from": true, "have": true,
	"has": true, "its": true, "they": true, "when": true, "your": true,
	"will": true, "more": true, "one": true, "use": true, "any": true,
}

// ---------------------------------------------------------------------------
// EmbeddingRetriever — cosine similarity via local embedding endpoint
// ---------------------------------------------------------------------------

// EmbeddingRetriever scores skills by cosine similarity using a local embedding service.
// Falls back to TemplateRetriever if the endpoint is unavailable.
type EmbeddingRetriever struct {
	store        Store
	embeddingURL string
	client       *http.Client
	fallback     Retriever
}

// Retrieve returns top-k skills by cosine similarity to the task description.
// Falls back to keyword matching if the embedding endpoint is unreachable.
func (r *EmbeddingRetriever) Retrieve(ctx context.Context, taskDescription string, k int) ([]Skill, error) {
	queryVec, err := r.embed(ctx, taskDescription)
	if err != nil {
		// Graceful fallback to keyword matching
		return r.fallback.Retrieve(ctx, taskDescription, k)
	}

	skills, err := r.store.List("")
	if err != nil {
		return nil, err
	}

	type scored struct {
		skill Skill
		score float64
	}

	candidates := make([]scored, 0, len(skills))
	for _, s := range skills {
		text := s.Title + " " + s.Principle + " " + s.WhenToApply
		vec, err := r.embed(ctx, text)
		if err != nil {
			continue
		}
		score := cosineSimilarity(queryVec, vec)
		candidates = append(candidates, scored{s, score})
	}

	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	if k > len(candidates) {
		k = len(candidates)
	}
	out := make([]Skill, k)
	for i := range out {
		out[i] = candidates[i].skill
	}
	return out, nil
}

// embed calls the local embedding endpoint and returns a float64 vector.
func (r *EmbeddingRetriever) embed(ctx context.Context, text string) ([]float64, error) {
	body, err := json.Marshal(map[string]string{"text": text})
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.embeddingURL+"/embed", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embedding endpoint unavailable: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("embedding endpoint returned %d", resp.StatusCode)
	}

	var result struct {
		Embedding []float64 `json:"embedding"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Embedding, nil
}

// cosineSimilarity computes the cosine similarity between two vectors.
// Returns 0 if either vector has zero magnitude.
func cosineSimilarity(a, b []float64) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, magA, magB float64
	for i := range a {
		dot += a[i] * b[i]
		magA += a[i] * a[i]
		magB += b[i] * b[i]
	}
	if magA == 0 || magB == 0 {
		return 0
	}
	return dot / (math.Sqrt(magA) * math.Sqrt(magB))
}
