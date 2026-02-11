package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/clawinfra/evoclaw/internal/config"
)

// handleGenomeRoutes multiplexes genome-related routes
func (s *Server) handleGenomeRoutes(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleGetGenome(w, r)
	case http.MethodPut:
		s.handleUpdateGenome(w, r)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSkillRoutes multiplexes skill-related routes
func (s *Server) handleSkillRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.handleGetSkill(w, r)
	} else {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleGetGenome returns an agent's genome
// GET /api/agents/{id}/genome
func (s *Server) handleGetGenome(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract agent ID from path: /api/agents/{id}/genome
	path := strings.TrimPrefix(r.URL.Path, "/api/agents/")
	agentID := strings.TrimSuffix(path, "/genome")

	if agentID == "" {
		http.Error(w, "agent ID required", http.StatusBadRequest)
		return
	}

	// Get agent from registry
	agent := s.registry.Get(agentID)
	if agent == nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	// Get genome from agent definition
	var genome *config.Genome
	if agent.Config != nil && agent.Config.Genome != nil {
		genome = agent.Config.Genome
	} else {
		// Return default genome if none exists
		genome = &config.Genome{
			Identity: config.GenomeIdentity{
				Name:    agent.ID,
				Persona: "default agent",
				Voice:   "balanced",
			},
			Skills: make(map[string]config.SkillGenome),
			Behavior: config.GenomeBehavior{
				RiskTolerance: 0.3,
				Verbosity:     0.5,
				Autonomy:      0.5,
			},
			Constraints: config.GenomeConstraints{
				MaxLossUSD: 1000.0,
			},
		}
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(genome)
}

// handleUpdateGenome updates an agent's genome
// PUT /api/agents/{id}/genome
func (s *Server) handleUpdateGenome(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract agent ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/agents/")
	agentID := strings.TrimSuffix(path, "/genome")

	if agentID == "" {
		http.Error(w, "agent ID required", http.StatusBadRequest)
		return
	}

	// Get agent from registry
	agent := s.registry.Get(agentID)
	if agent == nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	// Parse genome from request body
	var genome config.Genome
	if err := json.NewDecoder(r.Body).Decode(&genome); err != nil {
		http.Error(w, fmt.Sprintf("invalid genome JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Validate genome (basic validation)
	if genome.Behavior.RiskTolerance < 0 || genome.Behavior.RiskTolerance > 1 {
		http.Error(w, "risk_tolerance must be between 0 and 1", http.StatusBadRequest)
		return
	}
	if genome.Behavior.Verbosity < 0 || genome.Behavior.Verbosity > 1 {
		http.Error(w, "verbosity must be between 0 and 1", http.StatusBadRequest)
		return
	}
	if genome.Behavior.Autonomy < 0 || genome.Behavior.Autonomy > 1 {
		http.Error(w, "autonomy must be between 0 and 1", http.StatusBadRequest)
		return
	}

	// Update agent's genome in config
	if agent.Config != nil {
		agent.Config.Genome = &genome
	}

	s.logger.Info("genome updated via API",
		"agent", agentID,
		"skills", len(genome.Skills),
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "success",
		"agent_id": agentID,
		"genome":   genome,
	})
}

// handleGetSkill returns a specific skill from an agent's genome
// GET /api/agents/{id}/genome/skills/{skill}
func (s *Server) handleGetSkill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract agent ID and skill name from path
	path := strings.TrimPrefix(r.URL.Path, "/api/agents/")
	parts := strings.Split(path, "/genome/skills/")
	if len(parts) != 2 {
		http.Error(w, "invalid path format", http.StatusBadRequest)
		return
	}

	agentID := parts[0]
	skillName := parts[1]

	if agentID == "" || skillName == "" {
		http.Error(w, "agent ID and skill name required", http.StatusBadRequest)
		return
	}

	// Get agent from registry
	agent := s.registry.Get(agentID)
	if agent == nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	// Get genome
	if agent.Config == nil || agent.Config.Genome == nil {
		http.Error(w, "genome not found", http.StatusNotFound)
		return
	}

	// Get skill
	skill, ok := agent.Config.Genome.Skills[skillName]
	if !ok {
		http.Error(w, "skill not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(skill)
}

// handleUpdateSkillParams updates parameters for a specific skill
// PUT /api/agents/{id}/genome/skills/{skill}/params
func (s *Server) handleUpdateSkillParams(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract agent ID and skill name from path
	path := strings.TrimPrefix(r.URL.Path, "/api/agents/")
	parts := strings.Split(path, "/genome/skills/")
	if len(parts) != 2 {
		http.Error(w, "invalid path format", http.StatusBadRequest)
		return
	}

	agentID := parts[0]
	skillPath := strings.TrimSuffix(parts[1], "/params")

	if agentID == "" || skillPath == "" {
		http.Error(w, "agent ID and skill name required", http.StatusBadRequest)
		return
	}

	// Get agent from registry
	agent := s.registry.Get(agentID)
	if agent == nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	// Get genome
	if agent.Config == nil || agent.Config.Genome == nil {
		http.Error(w, "genome not found", http.StatusNotFound)
		return
	}

	// Get skill
	skill, ok := agent.Config.Genome.Skills[skillPath]
	if !ok {
		http.Error(w, "skill not found", http.StatusNotFound)
		return
	}

	// Parse new params from request body
	var params map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		http.Error(w, fmt.Sprintf("invalid params JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Update skill params
	skill.Params = params
	skill.Version++
	agent.Config.Genome.Skills[skillPath] = skill

	s.logger.Info("skill params updated via API",
		"agent", agentID,
		"skill", skillPath,
		"version", skill.Version,
	)

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"agent":   agentID,
		"skill":   skillPath,
		"version": skill.Version,
		"params":  params,
	})
}
