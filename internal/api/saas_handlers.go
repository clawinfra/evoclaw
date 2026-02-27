package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/clawinfra/evoclaw/internal/saas"
)

// SetSaaSService registers the SaaS service for API endpoints.
func (s *Server) SetSaaSService(svc *saas.Service) {
	s.saasSvc = svc
}

// registerSaaSRoutes registers SaaS API endpoints on the given mux.
func (s *Server) registerSaaSRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/api/saas/register", s.handleSaaSRegister)
	mux.HandleFunc("/api/saas/agents", s.handleSaaSAgents)
	mux.HandleFunc("/api/saas/agents/", s.handleSaaSAgentDetail)
	mux.HandleFunc("/api/saas/usage", s.handleSaaSUsage)
}

// extractSaaSUser authenticates the request and returns the user ID.
func (s *Server) extractSaaSUser(r *http.Request) (string, error) {
	apiKey := r.Header.Get("X-API-Key")
	if apiKey == "" {
		apiKey = r.Header.Get("Authorization")
		apiKey = strings.TrimPrefix(apiKey, "Bearer ")
	}

	user, err := s.saasSvc.AuthenticateAPIKey(apiKey)
	if err != nil {
		return "", err
	}
	return user.ID, nil
}

// handleSaaSRegister creates a new user account.
func (s *Server) handleSaaSRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.saasSvc == nil {
		http.Error(w, "saas not configured", http.StatusServiceUnavailable)
		return
	}

	var req saas.RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	user, err := s.saasSvc.Register(req)
	if err != nil {
		s.logger.Error("failed to register user", "error", err)
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}

	w.WriteHeader(http.StatusCreated)
	s.respondJSON(w, user)
}

// handleSaaSAgents handles listing and spawning user agents.
func (s *Server) handleSaaSAgents(w http.ResponseWriter, r *http.Request) {
	if s.saasSvc == nil {
		http.Error(w, "saas not configured", http.StatusServiceUnavailable)
		return
	}

	switch r.Method {
	case http.MethodGet:
		userID, err := s.extractSaaSUser(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		agents, err := s.saasSvc.ListAgents(userID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.respondJSON(w, agents)

	case http.MethodPost:
		userID, err := s.extractSaaSUser(r)
		if err != nil {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		var req saas.SpawnRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}

		agent, err := s.saasSvc.SpawnAgent(r.Context(), userID, req)
		if err != nil {
			s.logger.Error("failed to spawn saas agent", "user", userID, "error", err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusCreated)
		s.respondJSON(w, agent)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSaaSAgentDetail handles individual agent operations.
func (s *Server) handleSaaSAgentDetail(w http.ResponseWriter, r *http.Request) {
	if s.saasSvc == nil {
		http.Error(w, "saas not configured", http.StatusServiceUnavailable)
		return
	}

	if r.Method != http.MethodDelete {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	userID, err := s.extractSaaSUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Extract sandbox ID from path
	path := strings.TrimPrefix(r.URL.Path, "/api/saas/agents/")
	sandboxID := strings.TrimSuffix(path, "/")
	if sandboxID == "" {
		http.Error(w, "sandbox id required", http.StatusBadRequest)
		return
	}

	if err := s.saasSvc.KillAgent(r.Context(), userID, sandboxID); err != nil {
		s.logger.Error("failed to kill saas agent", "user", userID, "sandbox", sandboxID, "error", err)
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.respondJSON(w, map[string]string{
		"message":    "agent killed",
		"sandbox_id": sandboxID,
	})
}

// handleSaaSUsage returns usage report for the authenticated user.
func (s *Server) handleSaaSUsage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if s.saasSvc == nil {
		http.Error(w, "saas not configured", http.StatusServiceUnavailable)
		return
	}

	userID, err := s.extractSaaSUser(r)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	usage, err := s.saasSvc.GetUsage(userID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	s.respondJSON(w, usage)
}
