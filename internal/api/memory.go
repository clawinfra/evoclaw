package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"
)

// handleMemoryStats returns memory system statistics
func (s *Server) handleMemoryStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if memory system is available
	if s.orch == nil || s.orch.GetMemory() == nil {
		http.Error(w, "memory system not initialized", http.StatusServiceUnavailable)
		return
	}

	mem := s.orch.GetMemory()
	stats, err := mem.GetStats(r.Context())
	if err != nil {
		s.logger.Error("failed to get memory stats", "error", err)
		http.Error(w, "failed to get memory stats", http.StatusInternalServerError)
		return
	}

	// Get warm stats for category breakdown
	warmStats := mem.GetWarm().GetStats()
	
	// Build top categories list
	topCategories := make([]map[string]interface{}, 0)
	for i, cat := range warmStats.TopCategories {
		if i >= 5 { // Top 5 categories
			break
		}
		topCategories = append(topCategories, map[string]interface{}{
			"category": cat.Category,
			"count":    cat.Count,
		})
	}

	// Calculate hot percentage
	hotPct := 0.0
	if stats.HotCapacity > 0 {
		hotPct = float64(stats.HotSizeBytes) / float64(stats.HotCapacity) * 100
	}

	// Get consolidator last run time (estimate based on current time - not tracked yet)
	lastConsolidation := time.Now().Add(-1 * time.Hour) // Placeholder

	response := map[string]interface{}{
		"hot": map[string]interface{}{
			"size_bytes": stats.HotSizeBytes,
			"max_bytes":  stats.HotCapacity,
			"pct_used":   hotPct,
		},
		"warm": map[string]interface{}{
			"count":          stats.WarmCount,
			"size_bytes":     stats.WarmSizeBytes,
			"max_kb":         stats.WarmCapacity / 1024,
			"top_categories": topCategories,
		},
		"cold": map[string]interface{}{
			"count":   stats.ColdCount,
			"backend": "turso",
		},
		"tree": map[string]interface{}{
			"nodes":     stats.TreeNodes,
			"max_nodes": 50, // From memory config MaxTreeNodes
			"depth":     stats.TreeDepth,
		},
		"scoring": map[string]interface{}{
			"half_life_days":      30.0, // From memory config
			"eviction_threshold":  0.3,  // From memory config
		},
		"metrics": map[string]interface{}{
			"last_consolidation": lastConsolidation.Format(time.RFC3339),
			"total_stored":       stats.WarmCount + stats.ColdCount,
			"total_evicted":      stats.ColdCount,
			"total_retrieved":    0, // Not tracked yet
		},
	}

	s.respondJSON(w, response)
}

// handleMemoryTree returns the tree index structure
func (s *Server) handleMemoryTree(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if memory system is available
	if s.orch == nil || s.orch.GetMemory() == nil {
		http.Error(w, "memory system not initialized", http.StatusServiceUnavailable)
		return
	}

	mem := s.orch.GetMemory()
	tree := mem.GetTree()

	// Serialize the tree
	treeData, err := tree.Serialize()
	if err != nil {
		s.logger.Error("failed to serialize tree", "error", err)
		http.Error(w, "failed to serialize tree", http.StatusInternalServerError)
		return
	}

	// Parse the JSON to return as structured response
	var treeStructure interface{}
	if err := json.Unmarshal(treeData, &treeStructure); err != nil {
		s.logger.Error("failed to parse tree data", "error", err)
		http.Error(w, "failed to parse tree data", http.StatusInternalServerError)
		return
	}

	response := map[string]interface{}{
		"tree": treeStructure,
		"metadata": map[string]interface{}{
			"node_count": tree.NodeCount,
			"depth":      tree.GetDepth(),
			"size_bytes": len(treeData),
		},
	}

	s.respondJSON(w, response)
}

// handleMemoryRetrieve searches and retrieves memories
func (s *Server) handleMemoryRetrieve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if memory system is available
	if s.orch == nil || s.orch.GetMemory() == nil {
		http.Error(w, "memory system not initialized", http.StatusServiceUnavailable)
		return
	}

	// Parse query parameters
	query := r.URL.Query().Get("q")
	if query == "" {
		http.Error(w, "query parameter 'q' required", http.StatusBadRequest)
		return
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 5 // default
	if limitStr != "" {
		parsedLimit, err := strconv.Atoi(limitStr)
		if err != nil || parsedLimit < 1 || parsedLimit > 50 {
			http.Error(w, "limit must be between 1 and 50", http.StatusBadRequest)
			return
		}
		limit = parsedLimit
	}

	mem := s.orch.GetMemory()
	memories, err := mem.Retrieve(r.Context(), query, limit)
	if err != nil {
		s.logger.Error("failed to retrieve memories", "query", query, "error", err)
		http.Error(w, fmt.Sprintf("failed to retrieve memories: %v", err), http.StatusInternalServerError)
		return
	}

	// Build response
	results := make([]map[string]interface{}, 0, len(memories))
	for _, m := range memories {
		result := map[string]interface{}{
			"id":            m.ID,
			"timestamp":     m.Timestamp.Format(time.RFC3339),
			"event_type":    m.EventType,
			"category":      m.Category,
			"importance":    m.Importance,
			"access_count":  m.AccessCount,
			"last_accessed": m.LastAccessed.Format(time.RFC3339),
			"created_at":    m.CreatedAt.Format(time.RFC3339),
		}

		// Add content if available
		if m.Content != nil {
			result["content"] = map[string]interface{}{
				"fact":    m.Content.Fact,
				"emotion": m.Content.Emotion,
				"people":  m.Content.People,
				"topics":  m.Content.Topics,
				"actions": m.Content.Actions,
				"outcome": m.Content.Outcome,
			}
		}

		results = append(results, result)
	}

	response := map[string]interface{}{
		"query":   query,
		"limit":   limit,
		"count":   len(results),
		"results": results,
	}

	s.respondJSON(w, response)
}
