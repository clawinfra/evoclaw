package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"
)

// TreeRebuilder uses an LLM to restructure the memory tree
type TreeRebuilder struct {
	tree    *MemoryTree
	warm    *WarmMemory
	llmFunc LLMCallFunc
	logger  *slog.Logger
	timeout time.Duration
}

// RebuildOperation represents a tree restructuring operation
type RebuildOperation struct {
	Type   string `json:"type"`   // "add", "remove", "merge", "rename"
	Path   string `json:"path"`   // target path
	NewPath string `json:"new_path,omitempty"` // for rename/merge
	Summary string `json:"summary,omitempty"`  // for add
	Reason  string `json:"reason"` // explanation
}

// RebuildPlan is the LLM's suggested tree restructuring
type RebuildPlan struct {
	Operations []RebuildOperation `json:"operations"`
	Rationale  string             `json:"rationale"`
}

// NewTreeRebuilder creates a new tree rebuilder
func NewTreeRebuilder(tree *MemoryTree, warm *WarmMemory, llmFunc LLMCallFunc, logger *slog.Logger) *TreeRebuilder {
	if logger == nil {
		logger = slog.Default()
	}

	return &TreeRebuilder{
		tree:    tree,
		warm:    warm,
		llmFunc: llmFunc,
		logger:  logger,
		timeout: 60 * time.Second, // Longer timeout for complex reasoning
	}
}

// RebuildTree analyzes the current tree and warm memory, then restructures the tree
func (r *TreeRebuilder) RebuildTree(ctx context.Context) error {
	if r.llmFunc == nil {
		return fmt.Errorf("no LLM function provided")
	}

	r.logger.Info("starting tree rebuild")

	// Analyze current state
	treeText := r.serializeTreeState()
	warmSummary := r.summarizeWarmMemory()

	// Generate rebuild plan
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()

	plan, err := r.generateRebuildPlan(ctx, treeText, warmSummary)
	if err != nil {
		return fmt.Errorf("generate rebuild plan: %w", err)
	}

	if len(plan.Operations) == 0 {
		r.logger.Info("no rebuild operations suggested")
		return nil
	}

	r.logger.Info("rebuild plan generated",
		"operations", len(plan.Operations),
		"rationale", plan.Rationale)

	// Apply operations carefully
	applied := 0
	for i, op := range plan.Operations {
		if err := r.applyOperation(op); err != nil {
			r.logger.Warn("failed to apply operation",
				"operation", i+1,
				"type", op.Type,
				"path", op.Path,
				"error", err)
			continue
		}
		applied++
		r.logger.Debug("applied operation",
			"type", op.Type,
			"path", op.Path,
			"reason", op.Reason)
	}

	r.logger.Info("tree rebuild complete",
		"total_operations", len(plan.Operations),
		"applied", applied)

	return nil
}

// generateRebuildPlan asks the LLM to suggest tree restructuring
func (r *TreeRebuilder) generateRebuildPlan(ctx context.Context, treeText, warmSummary string) (*RebuildPlan, error) {
	systemPrompt := buildRebuildSystemPrompt()
	userPrompt := buildRebuildUserPrompt(treeText, warmSummary)

	// Call LLM
	response, err := r.llmFunc(ctx, systemPrompt, userPrompt)
	if err != nil {
		return nil, fmt.Errorf("LLM call failed: %w", err)
	}

	// Parse response
	plan, err := parseRebuildResponse(response)
	if err != nil {
		return nil, fmt.Errorf("parse LLM response: %w", err)
	}

	return plan, nil
}

// serializeTreeState creates a text representation of the current tree
func (r *TreeRebuilder) serializeTreeState() string {
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Current Tree: %d nodes, depth %d\n\n", r.tree.NodeCount, r.tree.GetDepth()))
	r.serializeNode(r.tree.Root, 0, &sb)
	return sb.String()
}

func (r *TreeRebuilder) serializeNode(node *TreeNode, indent int, sb *strings.Builder) {
	if node.Path != "" {
		prefix := strings.Repeat("  ", indent)
		age := time.Since(node.LastUpdated).Hours() / 24.0
		sb.WriteString(fmt.Sprintf("%s- %s | %s | %d warm, %d cold | age: %.0f days\n",
			prefix, node.Path, node.Summary, node.WarmCount, node.ColdCount, age))
	}
	for _, child := range node.Children {
		r.serializeNode(child, indent+1, sb)
	}
}

// summarizeWarmMemory creates a summary of warm memory categories and patterns
func (r *TreeRebuilder) summarizeWarmMemory() string {
	stats := r.warm.GetStats()
	
	var sb strings.Builder
	sb.WriteString(fmt.Sprintf("Warm Memory: %d entries, %.1f KB\n\n", 
		stats.TotalEntries, float64(stats.TotalSizeBytes)/1024.0))
	
	sb.WriteString("Top categories:\n")
	for i, cat := range stats.TopCategories {
		if i >= 10 {
			break
		}
		sb.WriteString(fmt.Sprintf("  - %s: %d entries\n", cat.Category, cat.Count))
	}

	return sb.String()
}

// buildRebuildSystemPrompt creates the system prompt for tree rebuilding
func buildRebuildSystemPrompt() string {
	return `You are a memory tree architect. Analyze the current memory tree structure and warm memory patterns, then suggest improvements.

Goals:
- Merge similar or duplicate nodes
- Add new categories for frequently used topics
- Remove dead nodes (0 memories, old)
- Improve organization and findability
- Keep important memories accessible

Constraints:
- Max 50 nodes total
- Max depth 4
- Each node max 10 children
- Don't lose nodes that have warm or cold memories

Return a JSON object with operations:
{
  "operations": [
    {"type": "merge", "path": "projects/old_project", "new_path": "archive/projects", "reason": "consolidate old projects"},
    {"type": "add", "path": "daily/health", "summary": "Health tracking and exercise", "reason": "frequent health discussions"},
    {"type": "remove", "path": "temp/notes", "reason": "empty and unused for 90 days"}
  ],
  "rationale": "Overall strategy for the restructuring"
}

Operation types:
- "add": Create new node (requires path, summary)
- "remove": Delete node and children (requires path)
- "merge": Move node contents to another path (requires path, new_path)
- "rename": Change node path (requires path, new_path)

Max 10 operations. Focus on high-impact changes.`
}

// buildRebuildUserPrompt formats the tree state for the LLM
func buildRebuildUserPrompt(treeText, warmSummary string) string {
	var sb strings.Builder
	sb.WriteString(treeText)
	sb.WriteString("\n\n")
	sb.WriteString(warmSummary)
	sb.WriteString("\n\nSuggest tree restructuring operations (JSON):")
	return sb.String()
}

// parseRebuildResponse parses the LLM's rebuild plan
func parseRebuildResponse(response string) (*RebuildPlan, error) {
	// Clean up response
	response = strings.TrimSpace(response)
	response = strings.TrimPrefix(response, "```json")
	response = strings.TrimPrefix(response, "```")
	response = strings.TrimSuffix(response, "```")
	response = strings.TrimSpace(response)

	var plan RebuildPlan
	if err := json.Unmarshal([]byte(response), &plan); err != nil {
		return nil, fmt.Errorf("unmarshal JSON: %w (response: %s)", err, response)
	}

	return &plan, nil
}

// applyOperation applies a single rebuild operation to the tree
func (r *TreeRebuilder) applyOperation(op RebuildOperation) error {
	switch op.Type {
	case "add":
		return r.applyAdd(op)
	case "remove":
		return r.applyRemove(op)
	case "merge":
		return r.applyMerge(op)
	case "rename":
		return r.applyRename(op)
	default:
		return fmt.Errorf("unknown operation type: %s", op.Type)
	}
}

// applyAdd creates a new node
func (r *TreeRebuilder) applyAdd(op RebuildOperation) error {
	if op.Path == "" || op.Summary == "" {
		return fmt.Errorf("add operation requires path and summary")
	}

	// Check if node already exists
	if r.tree.FindNode(op.Path) != nil {
		return fmt.Errorf("node %s already exists", op.Path)
	}

	// Check constraints
	if r.tree.NodeCount >= MaxTreeNodes {
		return fmt.Errorf("tree is full (max %d nodes)", MaxTreeNodes)
	}

	// Ensure parent exists
	parts := strings.Split(op.Path, "/")
	if len(parts) > 1 {
		parentPath := strings.Join(parts[:len(parts)-1], "/")
		if r.tree.FindNode(parentPath) == nil {
			// Try to create parent first
			if err := r.tree.AddNode(parentPath, "Category"); err != nil {
				return fmt.Errorf("create parent %s: %w", parentPath, err)
			}
		}
	}

	return r.tree.AddNode(op.Path, op.Summary)
}

// applyRemove deletes a node
func (r *TreeRebuilder) applyRemove(op RebuildOperation) error {
	if op.Path == "" {
		return fmt.Errorf("remove operation requires path")
	}

	node := r.tree.FindNode(op.Path)
	if node == nil {
		return fmt.Errorf("node %s not found", op.Path)
	}

	// Safety check: don't remove nodes with memories
	if node.WarmCount > 0 || node.ColdCount > 0 {
		return fmt.Errorf("cannot remove node %s with memories (warm=%d, cold=%d)",
			op.Path, node.WarmCount, node.ColdCount)
	}

	return r.tree.RemoveNode(op.Path)
}

// applyMerge moves a node's memories to another path
func (r *TreeRebuilder) applyMerge(op RebuildOperation) error {
	if op.Path == "" || op.NewPath == "" {
		return fmt.Errorf("merge operation requires path and new_path")
	}

	source := r.tree.FindNode(op.Path)
	if source == nil {
		return fmt.Errorf("source node %s not found", op.Path)
	}

	target := r.tree.FindNode(op.NewPath)
	if target == nil {
		return fmt.Errorf("target node %s not found", op.NewPath)
	}

	// Transfer counts
	target.WarmCount += source.WarmCount
	target.ColdCount += source.ColdCount
	target.LastUpdated = time.Now()

	// Update warm memory entries to point to new path
	// (This would require updating warm memory entries, which we'll do by updating the category)
	r.warm.UpdateCategory(op.Path, op.NewPath)

	// Remove source node
	return r.tree.RemoveNode(op.Path)
}

// applyRename changes a node's path
func (r *TreeRebuilder) applyRename(op RebuildOperation) error {
	if op.Path == "" || op.NewPath == "" {
		return fmt.Errorf("rename operation requires path and new_path")
	}

	source := r.tree.FindNode(op.Path)
	if source == nil {
		return fmt.Errorf("node %s not found", op.Path)
	}

	// Check if new path already exists
	if r.tree.FindNode(op.NewPath) != nil {
		return fmt.Errorf("target path %s already exists", op.NewPath)
	}

	// Create new node with same data
	if err := r.tree.AddNode(op.NewPath, source.Summary); err != nil {
		return fmt.Errorf("create new node: %w", err)
	}

	// Transfer counts
	if err := r.tree.IncrementCounts(op.NewPath, source.WarmCount, source.ColdCount); err != nil {
		return fmt.Errorf("transfer counts: %w", err)
	}

	// Update warm memory entries
	r.warm.UpdateCategory(op.Path, op.NewPath)

	// Remove old node
	return r.tree.RemoveNode(op.Path)
}

// SetTimeout sets the LLM call timeout
func (r *TreeRebuilder) SetTimeout(timeout time.Duration) {
	r.timeout = timeout
}
