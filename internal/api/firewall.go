package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/clawinfra/evoclaw/internal/evolution"
)

// handleFirewallStatus returns the firewall status for an agent.
// GET /api/agents/{id}/firewall
func (s *Server) handleFirewallStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agentID := extractAgentIDForFirewall(r.URL.Path, "/firewall")
	if agentID == "" {
		http.Error(w, "agent ID required", http.StatusBadRequest)
		return
	}

	eng := s.getEvolutionEngine()
	if eng == nil {
		http.Error(w, "evolution engine not available", http.StatusServiceUnavailable)
		return
	}

	status := eng.Firewall.GetFirewallStatus(agentID)
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(status)
}

// handleFirewallRollback triggers a manual rollback for an agent.
// POST /api/agents/{id}/firewall/rollback
func (s *Server) handleFirewallRollback(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agentID := extractAgentIDForFirewall(r.URL.Path, "/firewall/rollback")
	if agentID == "" {
		http.Error(w, "agent ID required", http.StatusBadRequest)
		return
	}

	eng := s.getEvolutionEngine()
	if eng == nil {
		http.Error(w, "evolution engine not available", http.StatusServiceUnavailable)
		return
	}

	genome, err := eng.Firewall.Snapshots.Rollback(agentID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}

	// Persist the rolled-back genome
	if err := eng.UpdateGenome(agentID, genome); err != nil {
		http.Error(w, "failed to persist rollback: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.logger.Info("manual rollback via API", "agent", agentID)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "success",
		"agent_id": agentID,
		"message":  "genome rolled back to last snapshot",
	})
}

// handleFirewallReset resets the circuit breaker for an agent.
// POST /api/agents/{id}/firewall/reset
func (s *Server) handleFirewallReset(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agentID := extractAgentIDForFirewall(r.URL.Path, "/firewall/reset")
	if agentID == "" {
		http.Error(w, "agent ID required", http.StatusBadRequest)
		return
	}

	eng := s.getEvolutionEngine()
	if eng == nil {
		http.Error(w, "evolution engine not available", http.StatusServiceUnavailable)
		return
	}

	eng.Firewall.Breaker.Reset(agentID)

	s.logger.Info("circuit breaker reset via API", "agent", agentID)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "success",
		"agent_id": agentID,
		"message":  "circuit breaker reset to closed",
	})
}

// getEvolutionEngine returns the evolution engine if set.
func (s *Server) getEvolutionEngine() *evolution.Engine {
	if s.evolution == nil {
		return nil
	}
	eng, _ := s.evolution.(*evolution.Engine)
	return eng
}

// extractAgentIDForFirewall extracts agent ID from paths like /api/agents/{id}/firewall/...
func extractAgentIDForFirewall(path, suffix string) string {
	path = strings.TrimPrefix(path, "/api/agents/")
	path = strings.TrimSuffix(path, suffix)
	path = strings.TrimRight(path, "/")
	if path == "" || strings.Contains(path, "/") {
		return ""
	}
	return path
}
