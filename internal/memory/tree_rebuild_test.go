package memory

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// mockRebuildLLM returns a mock rebuild plan
func mockRebuildLLM(ctx context.Context, systemPrompt, userPrompt string) (string, error) {
	plan := RebuildPlan{
		Operations: []RebuildOperation{
			{
				Type:    "add",
				Path:    "health",
				Summary: "Health and fitness tracking",
				Reason:  "frequent health discussions",
			},
			{
				Type:   "remove",
				Path:   "temp/old",
				Reason: "empty and unused",
			},
		},
		Rationale: "Add health category and remove unused nodes",
	}

	data, _ := json.Marshal(plan)
	return string(data), nil
}

func TestTreeRebuilder_RebuildTree(t *testing.T) {
	tree := NewMemoryTree()
	_ = tree.AddNode("temp", "Temporary")
	_ = tree.AddNode("temp/old", "Old temp files")

	warm := NewWarmMemory(DefaultWarmConfig())

	rebuilder := NewTreeRebuilder(tree, warm, mockRebuildLLM, nil)

	err := rebuilder.RebuildTree(context.Background())
	if err != nil {
		t.Fatalf("RebuildTree failed: %v", err)
	}

	// Should have added health node
	healthNode := tree.FindNode("health")
	if healthNode == nil {
		t.Error("expected health node to be added")
	}

	// Should have removed temp/old node (it has no memories)
	oldNode := tree.FindNode("temp/old")
	if oldNode != nil {
		t.Error("expected temp/old node to be removed")
	}
}

func TestTreeRebuilder_NoLLMFunc(t *testing.T) {
	tree := NewMemoryTree()
	warm := NewWarmMemory(DefaultWarmConfig())

	rebuilder := NewTreeRebuilder(tree, warm, nil, nil)

	err := rebuilder.RebuildTree(context.Background())
	if err == nil {
		t.Error("expected error when no LLM function provided")
	}
}

func TestTreeRebuilder_AddOperation(t *testing.T) {
	tree := NewMemoryTree()
	warm := NewWarmMemory(DefaultWarmConfig())

	rebuilder := NewTreeRebuilder(tree, warm, nil, nil)

	op := RebuildOperation{
		Type:    "add",
		Path:    "test/new",
		Summary: "New test node",
		Reason:  "testing",
	}

	// Need to create parent first
	_ = tree.AddNode("test", "Test category")

	err := rebuilder.applyOperation(op)
	if err != nil {
		t.Fatalf("applyAdd failed: %v", err)
	}

	node := tree.FindNode("test/new")
	if node == nil {
		t.Fatal("expected node to be added")
	}

	if node.Summary != "New test node" {
		t.Errorf("unexpected summary: %s", node.Summary)
	}
}

func TestTreeRebuilder_RemoveOperation(t *testing.T) {
	tree := NewMemoryTree()
	_ = tree.AddNode("toremove", "Node to remove")

	warm := NewWarmMemory(DefaultWarmConfig())
	rebuilder := NewTreeRebuilder(tree, warm, nil, nil)

	op := RebuildOperation{
		Type:   "remove",
		Path:   "toremove",
		Reason: "testing removal",
	}

	err := rebuilder.applyOperation(op)
	if err != nil {
		t.Fatalf("applyRemove failed: %v", err)
	}

	node := tree.FindNode("toremove")
	if node != nil {
		t.Error("expected node to be removed")
	}
}

func TestTreeRebuilder_RemoveProtection(t *testing.T) {
	tree := NewMemoryTree()
	_ = tree.AddNode("protected", "Protected node")
	_ = tree.IncrementCounts("protected", 5, 0) // Has warm memories

	warm := NewWarmMemory(DefaultWarmConfig())
	rebuilder := NewTreeRebuilder(tree, warm, nil, nil)

	op := RebuildOperation{
		Type:   "remove",
		Path:   "protected",
		Reason: "try to remove",
	}

	err := rebuilder.applyOperation(op)
	if err == nil {
		t.Error("expected error when trying to remove node with memories")
	}

	// Node should still exist
	node := tree.FindNode("protected")
	if node == nil {
		t.Error("node should not have been removed")
	}
}

func TestTreeRebuilder_MergeOperation(t *testing.T) {
	tree := NewMemoryTree()
	_ = tree.AddNode("source", "Source node")
	_ = tree.AddNode("target", "Target node")
	_ = tree.IncrementCounts("source", 3, 2)

	warm := NewWarmMemory(DefaultWarmConfig())
	
	// Add a warm entry for source
	entry := &WarmEntry{
		ID:        "test1",
		Timestamp: time.Now(),
		EventType: "test",
		Category:  "source",
		Content:   &DistilledFact{Fact: "test"},
		Importance: 0.5,
	}
	_ = warm.Add(entry)

	rebuilder := NewTreeRebuilder(tree, warm, nil, nil)

	op := RebuildOperation{
		Type:    "merge",
		Path:    "source",
		NewPath: "target",
		Reason:  "consolidate",
	}

	err := rebuilder.applyOperation(op)
	if err != nil {
		t.Fatalf("applyMerge failed: %v", err)
	}

	// Source should be removed
	sourceNode := tree.FindNode("source")
	if sourceNode != nil {
		t.Error("expected source node to be removed")
	}

	// Target should have inherited counts
	targetNode := tree.FindNode("target")
	if targetNode == nil {
		t.Fatal("target node should exist")
	}

	if targetNode.WarmCount != 3 {
		t.Errorf("expected warm count 3, got %d", targetNode.WarmCount)
	}

	if targetNode.ColdCount != 2 {
		t.Errorf("expected cold count 2, got %d", targetNode.ColdCount)
	}

	// Warm entry category should be updated
	retrieved := warm.GetByCategory("target")
	if len(retrieved) != 1 {
		t.Errorf("expected 1 entry in target category, got %d", len(retrieved))
	}
}

func TestTreeRebuilder_RenameOperation(t *testing.T) {
	tree := NewMemoryTree()
	_ = tree.AddNode("oldname", "Old name")
	_ = tree.IncrementCounts("oldname", 2, 1)

	warm := NewWarmMemory(DefaultWarmConfig())
	entry := &WarmEntry{
		ID:        "test1",
		Timestamp: time.Now(),
		EventType: "test",
		Category:  "oldname",
		Content:   &DistilledFact{Fact: "test"},
		Importance: 0.5,
	}
	_ = warm.Add(entry)

	rebuilder := NewTreeRebuilder(tree, warm, nil, nil)

	op := RebuildOperation{
		Type:    "rename",
		Path:    "oldname",
		NewPath: "newname",
		Reason:  "better name",
	}

	err := rebuilder.applyOperation(op)
	if err != nil {
		t.Fatalf("applyRename failed: %v", err)
	}

	// Old node should be removed
	oldNode := tree.FindNode("oldname")
	if oldNode != nil {
		t.Error("expected old node to be removed")
	}

	// New node should exist with same data
	newNode := tree.FindNode("newname")
	if newNode == nil {
		t.Fatal("expected new node to exist")
	}

	if newNode.WarmCount != 2 {
		t.Errorf("expected warm count 2, got %d", newNode.WarmCount)
	}

	// Warm entry category should be updated
	retrieved := warm.GetByCategory("newname")
	if len(retrieved) != 1 {
		t.Errorf("expected 1 entry in newname category, got %d", len(retrieved))
	}
}

func TestParseRebuildResponse_WithMarkdown(t *testing.T) {
	response := `
` + "```json" + `
{
  "operations": [
    {"type": "add", "path": "test", "summary": "Test", "reason": "testing"}
  ],
  "rationale": "Test rebuild"
}
` + "```"

	plan, err := parseRebuildResponse(response)
	if err != nil {
		t.Fatalf("parseRebuildResponse failed: %v", err)
	}

	if len(plan.Operations) != 1 {
		t.Fatalf("expected 1 operation, got %d", len(plan.Operations))
	}

	if plan.Operations[0].Type != "add" {
		t.Errorf("unexpected operation type: %s", plan.Operations[0].Type)
	}

	if plan.Rationale != "Test rebuild" {
		t.Errorf("unexpected rationale: %s", plan.Rationale)
	}
}

func TestTreeRebuilder_SerializeTreeState(t *testing.T) {
	tree := NewMemoryTree()
	_ = tree.AddNode("test", "Test node")
	_ = tree.IncrementCounts("test", 5, 3)

	warm := NewWarmMemory(DefaultWarmConfig())
	rebuilder := NewTreeRebuilder(tree, warm, nil, nil)

	treeText := rebuilder.serializeTreeState()

	// Should contain node info
	if !containsHelper(treeText, "test") {
		t.Error("expected tree text to contain node path")
	}

	if !containsHelper(treeText, "Test node") {
		t.Error("expected tree text to contain summary")
	}

	if !containsHelper(treeText, "5 warm") {
		t.Error("expected tree text to contain warm count")
	}
}

func TestTreeRebuilder_SummarizeWarmMemory(t *testing.T) {
	warm := NewWarmMemory(DefaultWarmConfig())

	// Add some entries
	for i := 0; i < 5; i++ {
		entry := &WarmEntry{
			ID:        string(rune('a' + i)),
			Timestamp: time.Now(),
			EventType: "test",
			Category:  "testcat",
			Content:   &DistilledFact{Fact: "test"},
			Importance: 0.5,
		}
		_ = warm.Add(entry)
	}

	tree := NewMemoryTree()
	rebuilder := NewTreeRebuilder(tree, warm, nil, nil)

	summary := rebuilder.summarizeWarmMemory()

	// Should contain entry count
	if !containsHelper(summary, "5 entries") {
		t.Error("expected summary to contain entry count")
	}

	// Should contain category info
	if !containsHelper(summary, "testcat") {
		t.Error("expected summary to contain category")
	}
}
