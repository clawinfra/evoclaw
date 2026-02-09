package memory

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

const (
	MaxTreeNodes      = 50
	MaxTreeDepth      = 4
	MaxTreeSizeBytes  = 2048
	MaxNodeSummary    = 100
	MaxChildrenPerNode = 10
)

// TreeNode represents a node in the memory tree index
type TreeNode struct {
	Path        string      `json:"path"`         // e.g., "active_projects/garden_replanting"
	Summary     string      `json:"summary"`      // Brief description (max 100 chars)
	WarmCount   int         `json:"warm_count"`   // Number of warm memories in this category
	ColdCount   int         `json:"cold_count"`   // Number of cold memories in this category
	LastUpdated time.Time   `json:"last_updated"` // Last modification timestamp
	Children    []*TreeNode `json:"children,omitempty"`
}

// MemoryTree is the hierarchical index of all memories
type MemoryTree struct {
	Root      *TreeNode `json:"root"`
	NodeCount int       `json:"node_count"`
	Version   int       `json:"version"`
}

// NewMemoryTree creates a new empty memory tree
func NewMemoryTree() *MemoryTree {
	return &MemoryTree{
		Root: &TreeNode{
			Path:        "",
			Summary:     "Memory Root",
			LastUpdated: time.Now(),
			Children:    make([]*TreeNode, 0),
		},
		NodeCount: 1,
		Version:   1,
	}
}

// AddNode adds a new node to the tree
func (t *MemoryTree) AddNode(path, summary string) error {
	if t.NodeCount >= MaxTreeNodes {
		return fmt.Errorf("tree is full (max %d nodes)", MaxTreeNodes)
	}

	parts := strings.Split(path, "/")
	if len(parts) > MaxTreeDepth {
		return fmt.Errorf("path depth exceeds max %d", MaxTreeDepth)
	}

	if len(summary) > MaxNodeSummary {
		summary = summary[:MaxNodeSummary]
	}

	// Navigate to parent and add child
	parent := t.Root
	for i := 0; i < len(parts)-1; i++ {
		parentPath := strings.Join(parts[:i+1], "/")
		found := false
		for _, child := range parent.Children {
			if child.Path == parentPath {
				parent = child
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("parent path %s does not exist", parentPath)
		}
	}

	// Check if node already exists
	for _, child := range parent.Children {
		if child.Path == path {
			return fmt.Errorf("node %s already exists", path)
		}
	}

	// Check max children per node
	if len(parent.Children) >= MaxChildrenPerNode {
		return fmt.Errorf("parent has max %d children", MaxChildrenPerNode)
	}

	// Add new node
	node := &TreeNode{
		Path:        path,
		Summary:     summary,
		LastUpdated: time.Now(),
		Children:    make([]*TreeNode, 0),
	}
	parent.Children = append(parent.Children, node)
	t.NodeCount++
	t.Version++

	return nil
}

// RemoveNode removes a node and all its children
func (t *MemoryTree) RemoveNode(path string) error {
	if path == "" {
		return fmt.Errorf("cannot remove root node")
	}

	parts := strings.Split(path, "/")
	parentPath := ""
	if len(parts) > 1 {
		parentPath = strings.Join(parts[:len(parts)-1], "/")
	}

	parent := t.FindNode(parentPath)
	if parent == nil {
		return fmt.Errorf("parent not found")
	}

	// Find and remove the node
	for i, child := range parent.Children {
		if child.Path == path {
			// Count nodes being removed (including descendants)
			removed := t.countNodes(child)
			parent.Children = append(parent.Children[:i], parent.Children[i+1:]...)
			t.NodeCount -= removed
			t.Version++
			return nil
		}
	}

	return fmt.Errorf("node %s not found", path)
}

// FindNode finds a node by path
func (t *MemoryTree) FindNode(path string) *TreeNode {
	if path == "" {
		return t.Root
	}

	parts := strings.Split(path, "/")
	current := t.Root

	for _, part := range parts {
		found := false
		for _, child := range current.Children {
			if strings.HasSuffix(child.Path, "/"+part) || child.Path == part {
				current = child
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}

	return current
}

// UpdateNode updates a node's summary and counts
func (t *MemoryTree) UpdateNode(path string, summary *string, warmCount, coldCount *int) error {
	node := t.FindNode(path)
	if node == nil {
		return fmt.Errorf("node %s not found", path)
	}

	if summary != nil {
		if len(*summary) > MaxNodeSummary {
			*summary = (*summary)[:MaxNodeSummary]
		}
		node.Summary = *summary
	}

	if warmCount != nil {
		node.WarmCount = *warmCount
	}

	if coldCount != nil {
		node.ColdCount = *coldCount
	}

	node.LastUpdated = time.Now()
	t.Version++

	return nil
}

// IncrementCounts increments warm or cold counts for a node
func (t *MemoryTree) IncrementCounts(path string, warmDelta, coldDelta int) error {
	node := t.FindNode(path)
	if node == nil {
		return fmt.Errorf("node %s not found", path)
	}

	node.WarmCount += warmDelta
	node.ColdCount += coldDelta
	node.LastUpdated = time.Now()
	t.Version++

	return nil
}

// Serialize converts the tree to JSON
func (t *MemoryTree) Serialize() ([]byte, error) {
	data, err := json.Marshal(t)
	if err != nil {
		return nil, fmt.Errorf("marshal tree: %w", err)
	}

	if len(data) > MaxTreeSizeBytes {
		return nil, fmt.Errorf("tree size %d exceeds max %d bytes", len(data), MaxTreeSizeBytes)
	}

	return data, nil
}

// Deserialize loads a tree from JSON
func DeserializeTree(data []byte) (*MemoryTree, error) {
	var tree MemoryTree
	if err := json.Unmarshal(data, &tree); err != nil {
		return nil, fmt.Errorf("unmarshal tree: %w", err)
	}

	// Recount nodes
	tree.NodeCount = tree.countNodes(tree.Root)

	return &tree, nil
}

// countNodes recursively counts all nodes in a subtree
func (t *MemoryTree) countNodes(node *TreeNode) int {
	count := 1
	for _, child := range node.Children {
		count += t.countNodes(child)
	}
	return count
}

// GetAllPaths returns all node paths in the tree
func (t *MemoryTree) GetAllPaths() []string {
	paths := make([]string, 0, t.NodeCount)
	t.collectPaths(t.Root, &paths)
	return paths
}

func (t *MemoryTree) collectPaths(node *TreeNode, paths *[]string) {
	if node.Path != "" {
		*paths = append(*paths, node.Path)
	}
	for _, child := range node.Children {
		t.collectPaths(child, paths)
	}
}

// GetLeafNodes returns all leaf nodes (nodes with no children)
func (t *MemoryTree) GetLeafNodes() []*TreeNode {
	leaves := make([]*TreeNode, 0)
	t.collectLeaves(t.Root, &leaves)
	return leaves
}

func (t *MemoryTree) collectLeaves(node *TreeNode, leaves *[]*TreeNode) {
	if len(node.Children) == 0 && node.Path != "" {
		*leaves = append(*leaves, node)
	}
	for _, child := range node.Children {
		t.collectLeaves(child, leaves)
	}
}

// PruneDeadNodes removes nodes with zero warm and zero cold counts
// and no recent updates (> 60 days)
func (t *MemoryTree) PruneDeadNodes(maxAgeDays int) int {
	removed := 0
	t.pruneRecursive(t.Root, maxAgeDays, &removed)
	return removed
}

func (t *MemoryTree) pruneRecursive(node *TreeNode, maxAgeDays int, removed *int) {
	// Process children (reverse order to handle removal safely)
	for i := len(node.Children) - 1; i >= 0; i-- {
		child := node.Children[i]
		t.pruneRecursive(child, maxAgeDays, removed)

		// Remove if dead (no memories and old)
		age := time.Since(child.LastUpdated).Hours() / 24.0
		if child.WarmCount == 0 && child.ColdCount == 0 && age > float64(maxAgeDays) {
			// Remove this child
			count := t.countNodes(child)
			node.Children = append(node.Children[:i], node.Children[i+1:]...)
			t.NodeCount -= count
			*removed += count
			t.Version++
		}
	}
}

// GetDepth returns the maximum depth of the tree
func (t *MemoryTree) GetDepth() int {
	return t.getNodeDepth(t.Root, 0)
}

func (t *MemoryTree) getNodeDepth(node *TreeNode, current int) int {
	if len(node.Children) == 0 {
		return current
	}

	maxDepth := current
	for _, child := range node.Children {
		depth := t.getNodeDepth(child, current+1)
		if depth > maxDepth {
			maxDepth = depth
		}
	}

	return maxDepth
}

// GetTreeSummary returns a human-readable summary of the tree
func (t *MemoryTree) GetTreeSummary() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Memory Tree (v%d, %d nodes, depth %d)\n", t.Version, t.NodeCount, t.GetDepth()))
	t.printNodeSummary(t.Root, 0, &sb)
	return sb.String()
}

func (t *MemoryTree) printNodeSummary(node *TreeNode, indent int, sb *strings.Builder) {
	if node.Path != "" {
		prefix := strings.Repeat("  ", indent)
		name := node.Path
		if idx := strings.LastIndex(name, "/"); idx >= 0 {
			name = name[idx+1:]
		}
		sb.WriteString(fmt.Sprintf("%s├─ %s: %s [%d warm, %d cold]\n",
			prefix, name, node.Summary, node.WarmCount, node.ColdCount))
	}

	for _, child := range node.Children {
		t.printNodeSummary(child, indent+1, sb)
	}
}
