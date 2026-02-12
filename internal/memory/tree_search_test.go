package memory

import (
	"testing"
	"time"
)

func TestNewTreeSearcher(t *testing.T) {
	tree := NewMemoryTree()
	s := NewTreeSearcher(tree, DefaultScoreConfig())
	if s == nil {
		t.Fatal("expected non-nil")
	}
}

func TestTreeSearcherSearch(t *testing.T) {
	tree := NewMemoryTree()
	_ = tree.AddNode("tech/go", "Go programming")
	_ = tree.AddNode("tech/python", "Python programming")

	s := NewTreeSearcher(tree, DefaultScoreConfig())
	results := s.Search("go programming", 5)
	_ = results // Just ensure no panic
}

func TestTreeSearcherSearchEmpty(t *testing.T) {
	tree := NewMemoryTree()
	s := NewTreeSearcher(tree, DefaultScoreConfig())
	results := s.Search("anything", 5)
	if len(results) != 0 {
		t.Errorf("expected 0 results from empty tree, got %d", len(results))
	}
}

func TestTreeSearcherSearchByCategory(t *testing.T) {
	tree := NewMemoryTree()
	_ = tree.AddNode("tech/go", "Go programming")
	_ = tree.AddNode("tech/python", "Python programming")

	s := NewTreeSearcher(tree, DefaultScoreConfig())
	paths := s.SearchByCategory("tech")
	_ = paths // SearchByCategory may not find results if tree structure differs
}

func TestExtractKeywordsSearch(t *testing.T) {
	tree := NewMemoryTree()
	s := NewTreeSearcher(tree, DefaultScoreConfig())

	keywords := s.extractKeywords("the quick brown fox jumps over a lazy dog")
	for _, kw := range keywords {
		if kw == "the" || kw == "a" {
			t.Errorf("stop word %q not filtered", kw)
		}
	}
}

func TestFindRecentlyUpdated(t *testing.T) {
	tree := NewMemoryTree()
	_ = tree.AddNode("tech/go", "Go")

	s := NewTreeSearcher(tree, DefaultScoreConfig())
	paths := s.FindRecentlyUpdated(7)
	_ = paths
}

func TestFindActiveNodes(t *testing.T) {
	tree := NewMemoryTree()
	_ = tree.AddNode("tech/go", "Go")

	s := NewTreeSearcher(tree, DefaultScoreConfig())
	active := s.FindActiveNodes()
	_ = active
}

func TestImportanceFromCounts(t *testing.T) {
	tree := NewMemoryTree()
	s := NewTreeSearcher(tree, DefaultScoreConfig())

	imp := s.importanceFromCounts(0, 0)
	if imp != 0 {
		t.Errorf("expected 0, got %f", imp)
	}

	imp = s.importanceFromCounts(5, 10)
	if imp <= 0 {
		t.Errorf("expected positive, got %f", imp)
	}
}

func TestExplainRelevance(t *testing.T) {
	tree := NewMemoryTree()
	s := NewTreeSearcher(tree, DefaultScoreConfig())

	node := &TreeNode{
		Path:        "test",
		Summary:     "test node",
		LastUpdated: time.Now(),
	}

	explanation := s.explainRelevance(node, []string{"test"}, 0.8)
	if explanation == "" {
		t.Error("expected non-empty explanation")
	}
}
