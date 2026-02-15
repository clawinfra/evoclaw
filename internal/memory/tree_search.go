package memory

import (
	"sort"
	"strings"
	"time"
)

// SearchResult represents a node found during tree search
type SearchResult struct {
	Path      string
	Score     float64
	Relevance string // explanation of why this node is relevant
}

// TreeSearcher performs reasoning-based retrieval over the memory tree
type TreeSearcher struct {
	tree *MemoryTree
	cfg  ScoreConfig
}

// NewTreeSearcher creates a new tree searcher
func NewTreeSearcher(tree *MemoryTree, cfg ScoreConfig) *TreeSearcher {
	return &TreeSearcher{
		tree: tree,
		cfg:  cfg,
	}
}

// Search finds relevant nodes in the tree based on a query
// For now, implements keyword/topic matching (not LLM-powered yet)
// Returns top-K relevant paths with scores
func (s *TreeSearcher) Search(query string, topK int) []SearchResult {
	if topK <= 0 {
		topK = 5
	}

	// Extract keywords from query
	keywords := s.extractKeywords(query)

	// Score all nodes
	results := make([]SearchResult, 0)
	s.scoreNodes(s.tree.Root, keywords, &results)

	// Sort by score descending
	sort.Slice(results, func(i, j int) bool {
		return results[i].Score > results[j].Score
	})

	// Return top-K
	if len(results) > topK {
		results = results[:topK]
	}

	return results
}

// scoreNodes recursively scores all nodes in the tree
func (s *TreeSearcher) scoreNodes(node *TreeNode, keywords []string, results *[]SearchResult) {
	if node.Path == "" {
		// Skip root
		for _, child := range node.Children {
			s.scoreNodes(child, keywords, results)
		}
		return
	}

	score := s.scoreNode(node, keywords)
	if score > 0 {
		*results = append(*results, SearchResult{
			Path:      node.Path,
			Score:     score,
			Relevance: s.explainRelevance(node, keywords, score),
		})
	}

	// Recurse to children
	for _, child := range node.Children {
		s.scoreNodes(child, keywords, results)
	}
}

// scoreNode calculates relevance score for a single node
func (s *TreeSearcher) scoreNode(node *TreeNode, keywords []string) float64 {
	// Keyword matching score
	keywordScore := s.keywordOverlap(node, keywords)

	// Recency bonus
	age := time.Since(node.LastUpdated)
	recencyScore := RecencyDecay(age, s.cfg.HalfLifeDays)

	// Importance based on memory counts (nodes with more memories are more important)
	importanceScore := s.importanceFromCounts(node.WarmCount, node.ColdCount)

	// Combined score
	return keywordScore * 0.6 + recencyScore * 0.2 + importanceScore * 0.2
}

// keywordOverlap calculates how many keywords match the node's path and summary
func (s *TreeSearcher) keywordOverlap(node *TreeNode, keywords []string) float64 {
	if len(keywords) == 0 {
		return 0
	}

	text := strings.ToLower(node.Path + " " + node.Summary)
	matches := 0

	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			matches++
		}
	}

	return float64(matches) / float64(len(keywords))
}

// importanceFromCounts calculates importance score based on warm/cold counts
func (s *TreeSearcher) importanceFromCounts(warmCount, coldCount int) float64 {
	total := float64(warmCount + coldCount)
	if total == 0 {
		return 0
	}

	// Warm memories are more important than cold
	weighted := float64(warmCount)*1.5 + float64(coldCount)

	// Normalize to 0-1 (assume max ~100 memories per node)
	normalized := weighted / 100.0
	if normalized > 1.0 {
		normalized = 1.0
	}

	return normalized
}

// explainRelevance generates a human-readable explanation of why a node is relevant
func (s *TreeSearcher) explainRelevance(node *TreeNode, keywords []string, score float64) string {
	matches := make([]string, 0)
	text := strings.ToLower(node.Path + " " + node.Summary)

	for _, kw := range keywords {
		if strings.Contains(text, kw) {
			matches = append(matches, kw)
		}
	}

	if len(matches) > 0 {
		return "matched: " + strings.Join(matches, ", ")
	}

	return "contextual"
}

// extractKeywords extracts meaningful keywords from a query
func (s *TreeSearcher) extractKeywords(query string) []string {
	// Simple tokenization and stopword removal
	query = strings.ToLower(query)
	
	// Common stopwords
	stopwords := map[string]bool{
		"the": true, "a": true, "an": true, "and": true, "or": true,
		"but": true, "is": true, "are": true, "was": true, "were": true,
		"be": true, "been": true, "being": true, "have": true, "has": true,
		"had": true, "do": true, "does": true, "did": true, "will": true,
		"would": true, "could": true, "should": true, "may": true, "might": true,
		"can": true, "to": true, "of": true, "in": true, "for": true,
		"on": true, "at": true, "by": true, "with": true, "from": true,
		"about": true, "what": true, "how": true, "when": true, "where": true,
		"who": true, "which": true, "this": true, "that": true, "these": true,
		"those": true, "it": true, "its": true, "my": true, "your": true,
		"their": true, "our": true, "me": true, "you": true, "them": true,
		"us": true, "i": true, "we": true, "they": true,
	}

	// Split on whitespace and punctuation
	words := strings.FieldsFunc(query, func(r rune) bool {
		return r == ' ' || r == ',' || r == '.' || r == '!' || r == '?' || r == ';' || r == ':'
	})

	keywords := make([]string, 0)
	for _, word := range words {
		word = strings.TrimSpace(word)
		if len(word) > 2 && !stopwords[word] {
			keywords = append(keywords, word)
		}
	}

	return keywords
}

// SearchByCategory finds all nodes under a category path
func (s *TreeSearcher) SearchByCategory(categoryPath string) []string {
	node := s.tree.FindNode(categoryPath)
	if node == nil {
		return nil
	}

	paths := make([]string, 0)
	s.collectPathsUnder(node, &paths)
	return paths
}

// collectPathsUnder recursively collects all paths under a node
func (s *TreeSearcher) collectPathsUnder(node *TreeNode, paths *[]string) {
	if node.Path != "" {
		*paths = append(*paths, node.Path)
	}
	for _, child := range node.Children {
		s.collectPathsUnder(child, paths)
	}
}

// FindRecentlyUpdated returns nodes updated within the last N days
func (s *TreeSearcher) FindRecentlyUpdated(days int) []string {
	cutoff := time.Now().AddDate(0, 0, -days)
	paths := make([]string, 0)
	s.findRecentNodes(s.tree.Root, cutoff, &paths)
	return paths
}

func (s *TreeSearcher) findRecentNodes(node *TreeNode, cutoff time.Time, paths *[]string) {
	if node.Path != "" && node.LastUpdated.After(cutoff) {
		*paths = append(*paths, node.Path)
	}
	for _, child := range node.Children {
		s.findRecentNodes(child, cutoff, paths)
	}
}

// FindActiveNodes returns nodes with warm memories (actively used)
func (s *TreeSearcher) FindActiveNodes() []string {
	paths := make([]string, 0)
	s.findActiveNodesRecursive(s.tree.Root, &paths)
	return paths
}

func (s *TreeSearcher) findActiveNodesRecursive(node *TreeNode, paths *[]string) {
	if node.Path != "" && node.WarmCount > 0 {
		*paths = append(*paths, node.Path)
	}
	for _, child := range node.Children {
		s.findActiveNodesRecursive(child, paths)
	}
}
