package memory

import (
	"context"
	"encoding/json"
	"testing"
)

// mockSearchLLM returns mock search results
func mockSearchLLM(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	results := []LLMSearchResult{
		{Path: "projects/evoclaw", Relevance: 0.9, Reason: "directly about EvoClaw project"},
		{Path: "work/meetings", Relevance: 0.6, Reason: "may contain work discussions"},
	}

	data, _ := json.Marshal(results)
	return string(data), nil
}

// mockFailingSearchLLM simulates LLM failure
func mockFailingSearchLLM(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	return "", context.DeadlineExceeded
}

func TestLLMTreeSearcher_Search(t *testing.T) {
	tree := NewMemoryTree()
	tree.AddNode("projects", "Active projects")
	tree.AddNode("projects/evoclaw", "EvoClaw agent orchestrator")
	tree.AddNode("work", "Work-related")
	tree.AddNode("work/meetings", "Meeting notes")

	fallback := NewTreeSearcher(tree, DefaultScoreConfig())
	searcher := NewLLMTreeSearcher(tree, fallback, mockSearchLLM, nil)

	results := searcher.Search("EvoClaw development", 5)

	if len(results) == 0 {
		t.Fatal("expected search results")
	}

	// Should find projects/evoclaw with high relevance
	found := false
	for _, r := range results {
		if r.Path == "projects/evoclaw" {
			found = true
			if r.Score < 0.8 {
				t.Errorf("expected high relevance for projects/evoclaw, got %.2f", r.Score)
			}
		}
	}

	if !found {
		t.Error("expected to find projects/evoclaw in results")
	}
}

func TestLLMTreeSearcher_FallbackOnFailure(t *testing.T) {
	tree := NewMemoryTree()
	tree.AddNode("test", "Test node")

	fallback := NewTreeSearcher(tree, DefaultScoreConfig())
	searcher := NewLLMTreeSearcher(tree, fallback, mockFailingSearchLLM, nil)

	results := searcher.Search("test", 5)

	// Should succeed via fallback (even if no matches)
	if results == nil {
		t.Fatal("expected non-nil results from fallback")
	}
}

func TestLLMTreeSearcher_NoLLMFunc(t *testing.T) {
	tree := NewMemoryTree()
	tree.AddNode("example", "Example node")

	fallback := NewTreeSearcher(tree, DefaultScoreConfig())
	searcher := NewLLMTreeSearcher(tree, fallback, nil, nil)

	results := searcher.Search("example", 5)

	// Should use fallback immediately
	if results == nil {
		t.Fatal("expected non-nil results from fallback")
	}
}

func TestLLMTreeSearcher_RelevanceFiltering(t *testing.T) {
	// Mock LLM that returns low-relevance results
	lowRelevanceLLM := func(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
		results := []LLMSearchResult{
			{Path: "irrelevant/path", Relevance: 0.2, Reason: "weak match"},
			{Path: "relevant/path", Relevance: 0.8, Reason: "strong match"},
		}
		data, _ := json.Marshal(results)
		return string(data), nil
	}

	tree := NewMemoryTree()
	tree.AddNode("irrelevant", "Irrelevant")
	tree.AddNode("irrelevant/path", "Irrelevant path")
	tree.AddNode("relevant", "Relevant")
	tree.AddNode("relevant/path", "Relevant path")

	fallback := NewTreeSearcher(tree, DefaultScoreConfig())
	searcher := NewLLMTreeSearcher(tree, fallback, lowRelevanceLLM, nil)

	results := searcher.Search("test query", 5)

	// Should filter out results with relevance <= 0.3
	for _, r := range results {
		if r.Score <= 0.3 {
			t.Errorf("expected to filter out low-relevance result: %s (%.2f)", r.Path, r.Score)
		}
	}
}

func TestLLMTreeSearcher_TopKLimit(t *testing.T) {
	// Mock LLM that returns many results
	manyResultsLLM := func(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
		results := []LLMSearchResult{
			{Path: "path1", Relevance: 0.9, Reason: "match1"},
			{Path: "path2", Relevance: 0.8, Reason: "match2"},
			{Path: "path3", Relevance: 0.7, Reason: "match3"},
			{Path: "path4", Relevance: 0.6, Reason: "match4"},
			{Path: "path5", Relevance: 0.5, Reason: "match5"},
			{Path: "path6", Relevance: 0.4, Reason: "match6"},
		}
		data, _ := json.Marshal(results)
		return string(data), nil
	}

	tree := NewMemoryTree()
	for i := 1; i <= 6; i++ {
		tree.AddNode("path", "Path")
	}

	fallback := NewTreeSearcher(tree, DefaultScoreConfig())
	searcher := NewLLMTreeSearcher(tree, fallback, manyResultsLLM, nil)

	results := searcher.Search("test", 3)

	if len(results) > 3 {
		t.Errorf("expected max 3 results, got %d", len(results))
	}
}

func TestParseTreeSearchResponse_WithMarkdown(t *testing.T) {
	response := "```json\n[{\"path\":\"test/path\",\"relevance\":0.9,\"reason\":\"test\"}]\n```"

	results, err := parseTreeSearchResponse(response)
	if err != nil {
		t.Fatalf("parseTreeSearchResponse failed: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	if results[0].Path != "test/path" {
		t.Errorf("unexpected path: %s", results[0].Path)
	}

	if results[0].Score != 0.9 {
		t.Errorf("unexpected score: %.2f", results[0].Score)
	}
}

func TestSerializeTreeForLLM(t *testing.T) {
	tree := NewMemoryTree()
	tree.AddNode("projects", "Active projects")
	tree.AddNode("projects/evoclaw", "EvoClaw orchestrator")
	tree.AddNode("personal", "Personal life")

	tree.IncrementCounts("projects/evoclaw", 5, 10)

	fallback := NewTreeSearcher(tree, DefaultScoreConfig())
	searcher := NewLLMTreeSearcher(tree, fallback, nil, nil)

	treeText := searcher.serializeTreeForLLM()

	// Should contain node paths and summaries
	if !contains(treeText, "projects/evoclaw") {
		t.Error("expected tree text to contain projects/evoclaw")
	}

	if !contains(treeText, "EvoClaw orchestrator") {
		t.Error("expected tree text to contain node summary")
	}

	if !contains(treeText, "5 warm") {
		t.Error("expected tree text to contain warm count")
	}
}

func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && s != substr && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsHelper(s, substr)))
}

func containsHelper(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
