package hybrid

// SearchResult represents a single search result from any source.
type SearchResult struct {
	DocID   string
	ChunkID string
	Text    string
	Heading string
	Score   float64
	Source  string // "keyword" or "vector"
}

// MergeResults combines keyword and vector results with weighted scoring and deduplication.
func MergeResults(keywordResults, vectorResults []SearchResult, keywordWeight, vectorWeight float64) []SearchResult {
	// Normalize scores within each set
	normalizeScores(keywordResults)
	normalizeScores(vectorResults)

	// Merge by chunk ID, combining scores
	merged := make(map[string]*SearchResult)

	for i := range keywordResults {
		r := keywordResults[i]
		key := r.DocID + ":" + r.ChunkID
		if existing, ok := merged[key]; ok {
			existing.Score += r.Score * keywordWeight
		} else {
			r.Score *= keywordWeight
			r.Source = "keyword"
			merged[key] = &r
		}
	}

	for i := range vectorResults {
		r := vectorResults[i]
		key := r.DocID + ":" + r.ChunkID
		if existing, ok := merged[key]; ok {
			existing.Score += r.Score * vectorWeight
			existing.Source = "hybrid"
		} else {
			r.Score *= vectorWeight
			r.Source = "vector"
			merged[key] = &r
		}
	}

	// Sort by score descending
	results := make([]SearchResult, 0, len(merged))
	for _, r := range merged {
		results = append(results, *r)
	}
	sortResults(results)
	return results
}

func normalizeScores(results []SearchResult) {
	if len(results) == 0 {
		return
	}
	maxScore := results[0].Score
	for _, r := range results[1:] {
		if r.Score > maxScore {
			maxScore = r.Score
		}
	}
	if maxScore <= 0 {
		return
	}
	for i := range results {
		results[i].Score /= maxScore
	}
}

func sortResults(results []SearchResult) {
	// Simple insertion sort (result sets are small)
	for i := 1; i < len(results); i++ {
		for j := i; j > 0 && results[j].Score > results[j-1].Score; j-- {
			results[j], results[j-1] = results[j-1], results[j]
		}
	}
}
