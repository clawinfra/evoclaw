package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/clawinfra/evoclaw/internal/cloud"
)

// SetCloudManager registers the cloud agent manager for API endpoints.
func (s *Server) SetCloudManager(mgr *cloud.Manager) {
	s.cloudMgr = mgr
}

// registerCloudRoutes registers cloud API endpoints on the given mux.
func (s *Server) registerCloudRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/cloud", s.handleCloudList)
	mux.HandleFunc("/api/cloud/spawn", s.handleCloudSpawn)
	mux.HandleFunc("/api/cloud/costs", s.handleCloudCosts)
	mux.HandleFunc("/api/cloud/", s.handleCloudDetail)
}

// handleCloudList returns all running cloud agents.
func (s *Server) handleCloudList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.cloudMgr == nil {
		http.Error(w, "cloud manager not configured", http.StatusServiceUnavailable)
		return
	}

	sandboxes, err := s.cloudMgr.ListAgents(r.Context())
	if err != nil {
		s.logger.Error("failed to list cloud agents", "error", err)
		http.Error(w, "failed to list cloud agents", http.StatusInternalServerError)
		return
	}

	s.respondJSON(w, sandboxes)
}

// handleCloudSpawn creates a new cloud agent sandbox.
func (s *Server) handleCloudSpawn(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.cloudMgr == nil {
		http.Error(w, "cloud manager not configured", http.StatusServiceUnavailable)
		return
	}

	var config cloud.AgentConfig
	if err := json.NewDecoder(r.Body).Decode(&config); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	sandbox, err := s.cloudMgr.SpawnAgent(r.Context(), config)
	if err != nil {
		s.logger.Error("failed to spawn cloud agent", "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusCreated)
	s.respondJSON(w, sandbox)
}

// handleCloudCosts returns E2B credit usage.
func (s *Server) handleCloudCosts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.cloudMgr == nil {
		http.Error(w, "cloud manager not configured", http.StatusServiceUnavailable)
		return
	}

	costs := s.cloudMgr.GetCosts()
	s.respondJSON(w, costs)
}

// handleCloudDetail handles individual cloud agent operations.
// Routes: DELETE /api/cloud/{id} — kill agent
//
//	GET /api/cloud/{id} — get agent status
func (s *Server) handleCloudDetail(w http.ResponseWriter, r *http.Request) {
	if s.cloudMgr == nil {
		http.Error(w, "cloud manager not configured", http.StatusServiceUnavailable)
		return
	}

	// Extract sandbox ID from path: /api/cloud/{id}
	path := strings.TrimPrefix(r.URL.Path, "/api/cloud/")
	sandboxID := strings.TrimSuffix(path, "/")

	if sandboxID == "" {
		http.Error(w, "sandbox id required", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodDelete:
		if err := s.cloudMgr.KillAgent(r.Context(), sandboxID); err != nil {
			s.logger.Error("failed to kill cloud agent", "sandbox_id", sandboxID, "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.respondJSON(w, map[string]string{
			"message":    "agent killed",
			"sandbox_id": sandboxID,
		})

	case http.MethodGet:
		status, err := s.cloudMgr.GetAgentStatus(r.Context(), sandboxID)
		if err != nil {
			s.logger.Error("failed to get cloud agent status", "sandbox_id", sandboxID, "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.respondJSON(w, status)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}
