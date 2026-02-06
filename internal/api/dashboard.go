package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// handleDashboard returns aggregated dashboard metrics
func (s *Server) handleDashboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agentList := s.registry.List()
	allCosts := s.router.GetAllCosts()

	totalCost := 0.0
	totalRequests := int64(0)
	totalTokensIn := int64(0)
	totalTokensOut := int64(0)
	for _, cost := range allCosts {
		totalCost += cost.TotalCostUSD
		totalRequests += cost.TotalRequests
		totalTokensIn += cost.TotalTokensIn
		totalTokensOut += cost.TotalTokensOut
	}

	// Aggregate agent metrics
	totalMessages := int64(0)
	totalErrors := int64(0)
	totalActions := int64(0)
	totalSuccessful := int64(0)
	evolvingCount := 0
	for _, a := range agentList {
		snap := a.GetSnapshot()
		totalMessages += snap.MessageCount
		totalErrors += snap.ErrorCount
		totalActions += snap.Metrics.TotalActions
		totalSuccessful += snap.Metrics.SuccessfulActions
		if snap.Status == "evolving" {
			evolvingCount++
		}
	}

	successRate := 0.0
	if totalActions > 0 {
		successRate = float64(totalSuccessful) / float64(totalActions)
	}

	dashboard := map[string]interface{}{
		"version":         "0.1.0",
		"agents":          len(agentList),
		"models":          len(s.router.ListModels()),
		"evolving_agents": evolvingCount,
		"total_cost":      totalCost,
		"total_requests":  totalRequests,
		"total_tokens_in": totalTokensIn,
		"total_tokens_out": totalTokensOut,
		"total_messages":  totalMessages,
		"total_errors":    totalErrors,
		"total_actions":   totalActions,
		"success_rate":    successRate,
		"timestamp":       time.Now().UTC().Format(time.RFC3339),
	}

	s.respondJSON(w, dashboard)
}

// handleAgentEvolution returns evolution data for an agent
func (s *Server) handleAgentEvolution(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract agent ID from path: /api/agents/{id}/evolution
	path := strings.TrimPrefix(r.URL.Path, "/api/agents/")
	parts := strings.Split(path, "/")
	if len(parts) < 2 || parts[1] != "evolution" {
		http.Error(w, "invalid path", http.StatusBadRequest)
		return
	}

	agentID := parts[0]

	// Check agent exists
	_, err := s.registry.Get(agentID)
	if err != nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	// Get evolution data from orchestrator's evolution engine
	if s.orch != nil {
		// Try to get evolution data
		strategy := s.getEvolutionStrategy(agentID)
		if strategy != nil {
			s.respondJSON(w, strategy)
			return
		}
	}

	// Return empty evolution data if no evolution engine
	s.respondJSON(w, map[string]interface{}{
		"agent_id":  agentID,
		"version":   0,
		"fitness":   0.0,
		"evalCount": 0,
		"params":    map[string]float64{},
	})
}

// getEvolutionStrategy tries to extract evolution strategy data
func (s *Server) getEvolutionStrategy(agentID string) interface{} {
	// Access orchestrator's evolution engine if available
	if s.orch == nil {
		return nil
	}

	// Check if orchestrator has evolution data via reflection-free approach
	// The evolution engine is accessed through the orchestrator
	return nil
}

// handleLogStream provides Server-Sent Events for real-time log streaming
func (s *Server) handleLogStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	ctx := r.Context()
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Send initial connection message
	s.sendSSE(w, flusher, map[string]interface{}{
		"time":      time.Now().Format("15:04:05"),
		"level":     "info",
		"component": "api",
		"message":   "log stream connected",
	})

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			// Send heartbeat/status log
			agentList := s.registry.List()
			s.sendSSE(w, flusher, map[string]interface{}{
				"time":      time.Now().Format("15:04:05"),
				"level":     "info",
				"component": "system",
				"message":   fmt.Sprintf("heartbeat: %d agents online", len(agentList)),
			})
		}
	}
}

// sendSSE writes a Server-Sent Event
func (s *Server) sendSSE(w http.ResponseWriter, flusher http.Flusher, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "data: %s\n\n", jsonData)
	flusher.Flush()
}
