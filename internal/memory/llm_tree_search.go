package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// LLMTreeSearcher uses an LLM to reason over the memory tree index
type LLMTreeSearcher struct {
	tree     *MemoryTree
	fallback *TreeSearcher // keyword-based fallback
	llmFunc  LLMCallFunc
	logger   *slog.Logger
	timeout  time.Duration
}

// LLMSearchResult represents a search result with LLM reasoning
type LLMSearchResult struct {
	Path      string  `json:"path"`
	Relevance float64 `json:"relevance"`
	Reason    string  `json:"reason"`
}

// NewLLMTreeSearcher creates a new LLM-powered tree searcher
func NewLLMTreeSearcher(tree *MemoryTree, fallback *TreeSearcher, llmFunc LLMCallFunc, logger *slog.Logger) *LLMTreeSearcher {
	if logger == nil {
		logger = slog.Default()
	}

	return &LLMTreeSearcher{
		tree:     tree,
		fallback: fallback,
		llmFunc:  llmFunc,
		logger:   logger,
		timeout:  20 * time.Second,
	}
}

// Search finds relevant nodes using LLM reasoning, falls back to keyword search
func (s *LLMTreeSearcher) Search(query string, topK int) []SearchResult {
	if topK <= 0 {
		topK = 5
	}

	// If no LLM function, use fallback immediately
	if s.llmFunc == nil {
		s.logger.Debug("no LLM function, using fallback search")
		return s.fallback.Search(query, topK)
	}

	// Try LLM search with timeout
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()

	results, err := s.searchWithLLM(ctx, query, topK)
	if err != nil {
		s.logger.Warn("LLM search failed, using fallback",
			"error", err,
			"query", query)
		return s.fallback.Search(query, topK)
	}

	s.logger.Debug("LLM search succeeded",
		"query", query,
		"results", len(results))

	return results
}

// searchWithLLM performs LLM-powered tree search
func (s *LLMTreeSearcher) searchWithLLM(ctx context.Context, query string, topK int) ([]SearchResult, error) {
	// Serialize tree to compact text format
	treeText := s.serializeTreeForLLM()

	systemPrompt := buildTreeSearchSystemPrompt()
	userPrompt := buildTreeSearchUserPrompt(treeText, query, topK)

	// Call LLM
	response, err := s.llmFunc(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// Parse LLM response
	results, err := parseTreeSearchResponse(response)
	if err != nil {
		return nil, fmt.Errorf("parse LLM response: %w", err)
	}

	// Filter results with relevance > 0.3
	filtered := make([]SearchResult, 0)
	for _, r := range results {
		if r.Score > 0.3 {
			filtered = append(filtered, r)
		}
	}

	// Limit to topK
	if len(filtered) > topK {
		filtered = filtered[:topK]
	}

	return filtered, nil
}

// serializeTreeForLLM creates a compact text representation of the tree
func (s *LLMTreeSearcher) serializeTreeForLLM() string {
	var sb strings.Builder
	sb.WriteString("Memory Tree Index:\n\n")
	s.serializeNodeForLLM(s.tree.Root, 0, &sb)
	return sb.String()
}

// serializeNodeForLLM serializes a single node and its children
func (s *LLMTreeSearcher) serializeNodeForLLM(node *TreeNode, indent int, sb *strings.Builder) {
	if node.Path != "" {
		prefix := strings.Repeat("  ", indent)
		// Format: path | summary | warm_count memories
		fmt.Fprintf(sb, "%s- %s | %s | %d warm, %d cold\n",
			prefix, node.Path, node.Summary, node.WarmCount, node.ColdCount)
	}

	for _, child := range node.Children {
		s.serializeNodeForLLM(child, indent+1, sb)
	}
}

// buildTreeSearchSystemPrompt creates the system prompt for tree search
func buildTreeSearchSystemPrompt() string {
	return `You are a memory retrieval engine. Given a tree index of an agent's memories and a query, determine which tree nodes contain relevant memories.

Analyze the tree structure, node summaries, and memory counts to identify the most relevant paths.

Return ONLY a JSON array of relevant paths with scores (0.0-1.0):
[
  {"path": "projects/evoclaw", "relevance": 0.9, "reason": "directly about EvoClaw project"},
  {"path": "work/meetings", "relevance": 0.6, "reason": "may contain work discussions"}
]

Rules:
- Only include nodes with relevance > 0.3
- Max 5 results
- Higher relevance = stronger match to query
- Consider node summaries, paths, and context`
}

// buildTreeSearchUserPrompt formats the tree and query for the LLM
func buildTreeSearchUserPrompt(treeText, query string, topK int) string {
	var sb strings.Builder
	sb.WriteString(treeText)
	sb.WriteString("\n\nQuery: ")
	sb.WriteString(query)
	sb.WriteString(fmt.Sprintf("\n\nFind the top %d most relevant nodes (JSON array):", topK))
	return sb.String()
}

// parseTreeSearchResponse parses the LLM's JSON response
func parseTreeSearchResponse(response string) ([]SearchResult, error) {
	// Clean up response - remove markdown code blocks if present
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	// Parse JSON array
	var llmResults []LLMSearchResult
	if err := json.Unmarshal([]byte(response), &llmResults); err != nil {
		return nil, fmt.Errorf("unmarshal JSON: %w (response: %s)", err, response)
	}

	// Convert to SearchResult format
	results := make([]SearchResult, len(llmResults))
	for i, r := range llmResults {
		results[i] = SearchResult{
			Path:      r.Path,
			Score:     r.Relevance,
			Relevance: r.Reason,
		}
	}

	return results, nil
}

// SetTimeout sets the LLM call timeout
func (s *LLMTreeSearcher) SetTimeout(timeout time.Duration) {
	s.timeout = timeout
}

// SearchByCategory delegates to fallback (no LLM needed for exact path lookup)
func (s *LLMTreeSearcher) SearchByCategory(categoryPath string) []string {
	return s.fallback.SearchByCategory(categoryPath)
}

// FindRecentlyUpdated delegates to fallback (no LLM needed for time-based query)
func (s *LLMTreeSearcher) FindRecentlyUpdated(days int) []string {
	return s.fallback.FindRecentlyUpdated(days)
}

// FindActiveNodes delegates to fallback (no LLM needed for count-based query)
func (s *LLMTreeSearcher) FindActiveNodes() []string {
	return s.fallback.FindActiveNodes()
}
