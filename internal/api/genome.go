package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/evolution"
	"github.com/clawinfra/evoclaw/internal/security"
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
	agent, err := s.registry.Get(agentID)
	if err != nil || agent == nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	// Get genome from agent definition
	var genome *config.Genome
	if agent.Def.Genome != nil {
		genome = agent.Def.Genome
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
	_ = json.NewEncoder(w).Encode(genome)
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
	agent, err := s.registry.Get(agentID)
	if err != nil || agent == nil {
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
	agent.Def.Genome = &genome

	s.logger.Info("genome updated via API",
		"agent", agentID,
		"skills", len(genome.Skills),
	)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
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
	agent, err := s.registry.Get(agentID)
	if err != nil || agent == nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	// Get genome
	if agent.Def.Genome == nil {
		http.Error(w, "genome not found", http.StatusNotFound)
		return
	}

	// Get skill
	skill, ok := agent.Def.Genome.Skills[skillName]
	if !ok {
		http.Error(w, "skill not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(skill)
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
	agent, err := s.registry.Get(agentID)
	if err != nil || agent == nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	// Get genome
	if agent.Def.Genome == nil {
		http.Error(w, "genome not found", http.StatusNotFound)
		return
	}

	// Get skill
	skill, ok := agent.Def.Genome.Skills[skillPath]
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
	agent.Def.Genome.Skills[skillPath] = skill

	s.logger.Info("skill params updated via API",
		"agent", agentID,
		"skill", skillPath,
		"version", skill.Version,
	)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "success",
		"agent":   agentID,
		"skill":   skillPath,
		"version": skill.Version,
		"params":  params,
	})
}

// ================================
// Security Layer 1: Signed Constraints API
// ================================

// handleConstraintRoutes handles PUT /api/agents/{id}/genome/constraints
func (s *Server) handleConstraintRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract agent ID from path: /api/agents/{id}/genome/constraints
	path := strings.TrimPrefix(r.URL.Path, "/api/agents/")
	agentID := strings.TrimSuffix(path, "/genome/constraints")
	if agentID == "" {
		http.Error(w, "agent ID required", http.StatusBadRequest)
		return
	}

	agent, err := s.registry.Get(agentID)
	if err != nil || agent == nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	// Parse signed constraints update
	var req struct {
		Constraints config.GenomeConstraints `json:"constraints"`
		Signature   []byte                  `json:"signature"`
		PublicKey   []byte                  `json:"public_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, fmt.Sprintf("invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Verify signature
	ok, err := security.VerifyConstraints(req.Constraints, req.Signature, req.PublicKey)
	if err != nil {
		http.Error(w, fmt.Sprintf("signature verification error: %v", err), http.StatusForbidden)
		return
	}
	if !ok {
		http.Error(w, "invalid constraint signature", http.StatusForbidden)
		return
	}

	// Apply signed constraints
	if agent.Def.Genome == nil {
		agent.Def.Genome = &config.Genome{
			Skills: make(map[string]config.SkillGenome),
		}
	}
	agent.Def.Genome.Constraints = req.Constraints
	agent.Def.Genome.ConstraintSignature = req.Signature
	agent.Def.Genome.OwnerPublicKey = req.PublicKey

	s.logger.Info("signed constraints updated via API", "agent", agentID)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":      "success",
		"agent_id":    agentID,
		"constraints": req.Constraints,
	})
}

// ================================
// Layer 3: Behavioral Evolution API
// ================================

// handleFeedbackRoutes multiplexes feedback-related routes
func (s *Server) handleFeedbackRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		s.handleSubmitFeedback(w, r)
	} else {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleBehaviorRoutes multiplexes behavior-related routes
func (s *Server) handleBehaviorRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.handleGetBehavior(w, r)
	} else {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleBehaviorHistoryRoutes handles behavioral evolution history
func (s *Server) handleBehaviorHistoryRoutes(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		s.handleGetBehaviorHistory(w, r)
	} else {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSubmitFeedback submits user feedback on agent behavior
// POST /api/agents/{id}/feedback
func (s *Server) handleSubmitFeedback(w http.ResponseWriter, r *http.Request) {
	// Extract agent ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/agents/")
	agentID := strings.TrimSuffix(path, "/feedback")

	if agentID == "" {
		http.Error(w, "agent ID required", http.StatusBadRequest)
		return
	}

	// Parse feedback from request body
	var feedback struct {
		Type    string  `json:"type"`
		Score   float64 `json:"score"`
		Context string  `json:"context"`
	}

	if err := json.NewDecoder(r.Body).Decode(&feedback); err != nil {
		http.Error(w, fmt.Sprintf("invalid feedback JSON: %v", err), http.StatusBadRequest)
		return
	}

	// Validate feedback type
	validTypes := map[string]bool{
		"approval":   true,
		"correction": true,
		"engagement": true,
		"dismissal":  true,
		"completion": true,
	}

	if !validTypes[feedback.Type] {
		http.Error(w, "invalid feedback type", http.StatusBadRequest)
		return
	}

	// Validate score range
	if feedback.Score < -1.0 || feedback.Score > 1.0 {
		http.Error(w, "score must be between -1.0 and 1.0", http.StatusBadRequest)
		return
	}

	// Submit feedback to evolution engine
	if s.evolution != nil {
		if eng, ok := s.evolution.(*evolution.Engine); ok {
			if err := eng.SubmitFeedback(agentID, feedback.Type, feedback.Score, feedback.Context); err != nil {
				http.Error(w, fmt.Sprintf("failed to submit feedback: %v", err), http.StatusInternalServerError)
				return
			}
		}
	}

	s.logger.Info("feedback submitted",
		"agent", agentID,
		"type", feedback.Type,
		"score", feedback.Score,
	)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "success",
		"agent_id": agentID,
		"feedback": feedback,
	})
}

// handleGetBehavior returns an agent's behavioral genome
// GET /api/agents/{id}/genome/behavior
func (s *Server) handleGetBehavior(w http.ResponseWriter, r *http.Request) {
	// Extract agent ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/agents/")
	agentID := strings.TrimSuffix(path, "/genome/behavior")

	if agentID == "" {
		http.Error(w, "agent ID required", http.StatusBadRequest)
		return
	}

	// Get agent from registry
	agent, err := s.registry.Get(agentID)
	if err != nil || agent == nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	// Get behavior from genome
	if agent.Def.Genome == nil {
		http.Error(w, "genome not found", http.StatusNotFound)
		return
	}

	behavior := agent.Def.Genome.Behavior

	// Get behavioral metrics if evolution engine is available
	var metrics map[string]interface{}
	if s.evolution != nil {
		if eng, ok := s.evolution.(*evolution.Engine); ok {
			behaviorMetrics := eng.GetBehaviorMetrics(agentID)
			metrics = map[string]interface{}{
				"approval_rate":        behaviorMetrics.ApprovalRate,
				"task_completion_rate": behaviorMetrics.TaskCompletionRate,
				"cost_efficiency":      behaviorMetrics.CostEfficiency,
				"engagement_score":     behaviorMetrics.EngagementScore,
				"behavioral_fitness":   eng.BehavioralFitness(agentID),
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"behavior": behavior,
		"metrics":  metrics,
	})
}

// handleGetBehaviorHistory returns behavioral evolution history
// GET /api/agents/{id}/behavior/history
func (s *Server) handleGetBehaviorHistory(w http.ResponseWriter, r *http.Request) {
	// Extract agent ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/agents/")
	agentID := strings.TrimSuffix(path, "/behavior/history")

	if agentID == "" {
		http.Error(w, "agent ID required", http.StatusBadRequest)
		return
	}

	// Get feedback history from evolution engine
	var history []interface{}
	if s.evolution != nil {
		if eng, ok := s.evolution.(*evolution.Engine); ok {
			feedbackHistory, err := eng.GetBehaviorHistory(agentID)
			if err != nil {
				http.Error(w, fmt.Sprintf("failed to get behavior history: %v", err), http.StatusInternalServerError)
				return
			}

			history = make([]interface{}, len(feedbackHistory))
			for i, fb := range feedbackHistory {
				history[i] = map[string]interface{}{
					"timestamp": fb.Timestamp,
					"type":      fb.Type,
					"score":     fb.Score,
					"context":   fb.Context,
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"agent_id": agentID,
		"history":  history,
	})
}
