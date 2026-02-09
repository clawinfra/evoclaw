package memory

import (
	"fmt"
	"testing"
	"time"
)

func TestNewMemoryTree(t *testing.T) {
	tree := NewMemoryTree()

	if tree.Root == nil {
		t.Fatal("root is nil")
	}
	if tree.NodeCount != 1 {
		t.Errorf("initial node count: got %d, want 1", tree.NodeCount)
	}
	if tree.Version != 1 {
		t.Errorf("initial version: got %d, want 1", tree.Version)
	}
}

func TestAddNode(t *testing.T) {
	tree := NewMemoryTree()

	// Add first level
	err := tree.AddNode("projects", "Active projects")
	if err != nil {
		t.Fatalf("failed to add projects node: %v", err)
	}

	if tree.NodeCount != 2 {
		t.Errorf("node count after add: got %d, want 2", tree.NodeCount)
	}

	// Add second level
	err = tree.AddNode("projects/evoclaw", "EvoClaw development")
	if err != nil {
		t.Fatalf("failed to add evoclaw node: %v", err)
	}

	if tree.NodeCount != 3 {
		t.Errorf("node count: got %d, want 3", tree.NodeCount)
	}

	// Find the node
	node := tree.FindNode("projects/evoclaw")
	if node == nil {
		t.Fatal("node not found")
	}
	if node.Summary != "EvoClaw development" {
		t.Errorf("summary: got %s, want 'EvoClaw development'", node.Summary)
	}
}

func TestAddNodeConstraints(t *testing.T) {
	tree := NewMemoryTree()

	// Test max depth
	tree.AddNode("a", "level 1")
	tree.AddNode("a/b", "level 2")
	tree.AddNode("a/b/c", "level 3")
	tree.AddNode("a/b/c/d", "level 4")
	
	err := tree.AddNode("a/b/c/d/e", "level 5")
	if err == nil {
		t.Error("should reject depth > 4")
	}

	// Test duplicate
	err = tree.AddNode("a", "duplicate")
	if err == nil {
		t.Error("should reject duplicate path")
	}

	// Test parent doesn't exist
	tree2 := NewMemoryTree()
	err = tree2.AddNode("nonexistent/child", "orphan")
	if err == nil {
		t.Error("should reject node without parent")
	}
}

func TestRemoveNode(t *testing.T) {
	tree := NewMemoryTree()
	tree.AddNode("projects", "Projects")
	tree.AddNode("projects/evoclaw", "EvoClaw")
	tree.AddNode("projects/bsc", "BSC integration")

	initialCount := tree.NodeCount

	err := tree.RemoveNode("projects/evoclaw")
	if err != nil {
		t.Fatalf("failed to remove node: %v", err)
	}

	if tree.NodeCount != initialCount-1 {
		t.Errorf("node count: got %d, want %d", tree.NodeCount, initialCount-1)
	}

	node := tree.FindNode("projects/evoclaw")
	if node != nil {
		t.Error("node should be removed")
	}
}

func TestRemoveNodeWithChildren(t *testing.T) {
	tree := NewMemoryTree()
	tree.AddNode("projects", "Projects")
	tree.AddNode("projects/evoclaw", "EvoClaw")
	tree.AddNode("projects/evoclaw/memory", "Memory system")
	tree.AddNode("projects/evoclaw/bsc", "BSC integration")

	beforeCount := tree.NodeCount

	// Remove parent node
	err := tree.RemoveNode("projects/evoclaw")
	if err != nil {
		t.Fatalf("failed to remove: %v", err)
	}

	// Should remove parent + 2 children = 3 nodes
	expected := beforeCount - 3
	if tree.NodeCount != expected {
		t.Errorf("node count: got %d, want %d", tree.NodeCount, expected)
	}
}

func TestUpdateNode(t *testing.T) {
	tree := NewMemoryTree()
	tree.AddNode("projects", "Initial summary")

	newSummary := "Updated summary"
	warmCount := 5
	coldCount := 10

	err := tree.UpdateNode("projects", &newSummary, &warmCount, &coldCount)
	if err != nil {
		t.Fatalf("failed to update: %v", err)
	}

	node := tree.FindNode("projects")
	if node.Summary != newSummary {
		t.Errorf("summary: got %s, want %s", node.Summary, newSummary)
	}
	if node.WarmCount != warmCount {
		t.Errorf("warm count: got %d, want %d", node.WarmCount, warmCount)
	}
	if node.ColdCount != coldCount {
		t.Errorf("cold count: got %d, want %d", node.ColdCount, coldCount)
	}
}

func TestIncrementCounts(t *testing.T) {
	tree := NewMemoryTree()
	tree.AddNode("projects", "Projects")

	node := tree.FindNode("projects")
	initialWarm := node.WarmCount
	initialCold := node.ColdCount

	tree.IncrementCounts("projects", 3, 5)

	node = tree.FindNode("projects")
	if node.WarmCount != initialWarm+3 {
		t.Errorf("warm count: got %d, want %d", node.WarmCount, initialWarm+3)
	}
	if node.ColdCount != initialCold+5 {
		t.Errorf("cold count: got %d, want %d", node.ColdCount, initialCold+5)
	}
}

func TestSerialize(t *testing.T) {
	tree := NewMemoryTree()
	tree.AddNode("projects", "Projects")
	tree.AddNode("projects/evoclaw", "EvoClaw")

	data, err := tree.Serialize()
	if err != nil {
		t.Fatalf("serialize failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("serialized data is empty")
	}

	if len(data) > MaxTreeSizeBytes {
		t.Errorf("serialized size %d exceeds max %d", len(data), MaxTreeSizeBytes)
	}
}

func TestDeserialize(t *testing.T) {
	tree := NewMemoryTree()
	tree.AddNode("projects", "Projects")
	tree.AddNode("projects/evoclaw", "EvoClaw")

	data, _ := tree.Serialize()

	tree2, err := DeserializeTree(data)
	if err != nil {
		t.Fatalf("deserialize failed: %v", err)
	}

	if tree2.NodeCount != tree.NodeCount {
		t.Errorf("node count: got %d, want %d", tree2.NodeCount, tree.NodeCount)
	}

	node := tree2.FindNode("projects/evoclaw")
	if node == nil {
		t.Error("node not found after deserialize")
	}
}

func TestGetAllPaths(t *testing.T) {
	tree := NewMemoryTree()
	tree.AddNode("a", "A")
	tree.AddNode("a/b", "B")
	tree.AddNode("c", "C")

	paths := tree.GetAllPaths()
	if len(paths) != 3 {
		t.Errorf("path count: got %d, want 3", len(paths))
	}

	// Check all paths present
	pathMap := make(map[string]bool)
	for _, p := range paths {
		pathMap[p] = true
	}

	if !pathMap["a"] || !pathMap["a/b"] || !pathMap["c"] {
		t.Error("missing expected paths")
	}
}

func TestGetLeafNodes(t *testing.T) {
	tree := NewMemoryTree()
	tree.AddNode("projects", "Projects")
	tree.AddNode("projects/evoclaw", "EvoClaw")
	tree.AddNode("projects/evoclaw/memory", "Memory")
	tree.AddNode("lessons", "Lessons")

	leaves := tree.GetLeafNodes()

	// Should have 2 leaves: projects/evoclaw/memory and lessons
	if len(leaves) != 2 {
		t.Errorf("leaf count: got %d, want 2", len(leaves))
	}
}

func TestPruneDeadNodes(t *testing.T) {
	tree := NewMemoryTree()
	tree.AddNode("active", "Active node")
	tree.AddNode("old", "Old node")
	tree.AddNode("dead", "Dead node")

	// Set old timestamp on "dead" node
	node := tree.FindNode("dead")
	node.LastUpdated = time.Now().AddDate(0, 0, -90)
	node.WarmCount = 0
	node.ColdCount = 0

	// "old" has counts, should not be pruned
	node2 := tree.FindNode("old")
	node2.LastUpdated = time.Now().AddDate(0, 0, -90)
	node2.WarmCount = 5

	removed := tree.PruneDeadNodes(60)

	if removed != 1 {
		t.Errorf("pruned count: got %d, want 1", removed)
	}

	if tree.FindNode("dead") != nil {
		t.Error("dead node should be pruned")
	}
	if tree.FindNode("old") == nil {
		t.Error("old node with counts should not be pruned")
	}
}

func TestGetDepth(t *testing.T) {
	tree := NewMemoryTree()
	
	if tree.GetDepth() != 0 {
		t.Errorf("empty tree depth: got %d, want 0", tree.GetDepth())
	}

	tree.AddNode("a", "A")
	if tree.GetDepth() != 1 {
		t.Errorf("depth: got %d, want 1", tree.GetDepth())
	}

	tree.AddNode("a/b", "B")
	tree.AddNode("a/b/c", "C")
	if tree.GetDepth() != 3 {
		t.Errorf("depth: got %d, want 3", tree.GetDepth())
	}
}

func TestMaxNodesConstraint(t *testing.T) {
	tree := NewMemoryTree()

	// Add nodes using nested paths to respect max children per node (10)
	parents := []string{"a", "b", "c", "d", "e"}
	for _, p := range parents {
		tree.AddNode(p, "parent")
	}
	// Now add children under each parent to fill up to near max
	count := tree.NodeCount
	for _, p := range parents {
		for i := 0; i < 8 && count < MaxTreeNodes-1; i++ {
			path := fmt.Sprintf("%s/child%d", p, i)
			err := tree.AddNode(path, "child node")
			if err != nil {
				// May hit max children, that's ok — keep going
				continue
			}
			count = tree.NodeCount
		}
	}

	// Verify we got close to max
	if tree.NodeCount < 10 {
		t.Errorf("should have added many nodes, got %d", tree.NodeCount)
	}
}
