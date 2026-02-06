package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/clawinfra/evoclaw/internal/agents"
	"github.com/clawinfra/evoclaw/internal/cloud"
	"github.com/clawinfra/evoclaw/internal/models"
	"github.com/clawinfra/evoclaw/internal/orchestrator"
	"github.com/clawinfra/evoclaw/internal/saas"
)

// Server is the HTTP API server
type Server struct {
	port       int
	orch       *orchestrator.Orchestrator
	registry   *agents.Registry
	memory     *agents.MemoryStore
	router     *models.Router
	logger     *slog.Logger
	httpServer *http.Server
	cloudMgr   *cloud.Manager
	saasSvc    *saas.Service
}

// NewServer creates a new API server
func NewServer(
	port int,
	orch *orchestrator.Orchestrator,
	registry *agents.Registry,
	memory *agents.MemoryStore,
	router *models.Router,
	logger *slog.Logger,
) *Server {
	return &Server{
		port:     port,
		orch:     orch,
		registry: registry,
		memory:   memory,
		router:   router,
		logger:   logger.With("component", "api"),
	}
}

// Start starts the HTTP server
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Register routes
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/agents", s.handleAgents)
	mux.HandleFunc("/api/agents/", s.handleAgentDetail)
	mux.HandleFunc("/api/models", s.handleModels)
	mux.HandleFunc("/api/costs", s.handleCosts)

	// Cloud API routes (E2B sandbox management)
	s.registerCloudRoutes(mux)

	// SaaS API routes (multi-tenant agent-as-a-service)
	s.registerSaaSRoutes(mux)

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      s.corsMiddleware(s.loggingMiddleware(mux)),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	s.logger.Info("API server starting", "port", s.port)

	// Run server in goroutine
	errCh := make(chan error, 1)
	go func() {
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// Wait for shutdown or error
	select {
	case <-ctx.Done():
		s.logger.Info("shutting down API server")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.httpServer.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// loggingMiddleware logs HTTP requests
func (s *Server) loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		s.logger.Debug("http request",
			"method", r.Method,
			"path", r.URL.Path,
			"duration", time.Since(start),
		)
	})
}

// corsMiddleware adds CORS headers
func (s *Server) corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// handleStatus returns system status
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	agentList := s.registry.List()
	memStats := s.memory.GetStats()
	allCosts := s.router.GetAllCosts()

	totalCost := 0.0
	for _, cost := range allCosts {
		totalCost += cost.TotalCostUSD
	}

	status := map[string]interface{}{
		"version":    "0.1.0",
		"uptime":     time.Since(time.Now()), // TODO: track actual uptime
		"agents":     len(agentList),
		"models":     len(s.router.ListModels()),
		"memory":     memStats,
		"total_cost": totalCost,
	}

	s.respondJSON(w, status)
}

// handleAgents handles agent listing
func (s *Server) handleAgents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		agentList := s.registry.List()
		snapshots := make([]agents.Agent, len(agentList))
		for i, a := range agentList {
			snapshots[i] = a.GetSnapshot()
		}
		s.respondJSON(w, snapshots)

	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleAgentDetail handles individual agent operations
func (s *Server) handleAgentDetail(w http.ResponseWriter, r *http.Request) {
	// Extract agent ID from path: /api/agents/{id}/{action}
	path := strings.TrimPrefix(r.URL.Path, "/api/agents/")
	parts := strings.Split(path, "/")

	if len(parts) == 0 {
		http.Error(w, "agent id required", http.StatusBadRequest)
		return
	}

	agentID := parts[0]
	action := ""
	if len(parts) > 1 {
		action = parts[1]
	}

	// Get agent
	agent, err := s.registry.Get(agentID)
	if err != nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	// Handle different actions
	switch {
	case action == "metrics" && r.Method == http.MethodGet:
		s.handleAgentMetrics(w, agent)
	case action == "evolve" && r.Method == http.MethodPost:
		s.handleAgentEvolve(w, agent)
	case action == "memory" && r.Method == http.MethodGet:
		s.handleAgentMemory(w, agentID)
	case action == "memory" && r.Method == http.MethodDelete:
		s.handleClearMemory(w, agentID)
	case action == "" && r.Method == http.MethodGet:
		// Get agent details
		s.respondJSON(w, agent.GetSnapshot())
	default:
		http.Error(w, "invalid action or method", http.StatusBadRequest)
	}
}

// handleAgentMetrics returns agent performance metrics
func (s *Server) handleAgentMetrics(w http.ResponseWriter, agent *agents.Agent) {
	snapshot := agent.GetSnapshot()
	s.respondJSON(w, map[string]interface{}{
		"agent_id": agent.ID,
		"metrics":  snapshot.Metrics,
		"status":   snapshot.Status,
		"uptime":   time.Since(snapshot.StartedAt).Seconds(),
	})
}

// handleAgentEvolve triggers evolution for an agent
func (s *Server) handleAgentEvolve(w http.ResponseWriter, agent *agents.Agent) {
	// TODO: Trigger evolution engine
	s.respondJSON(w, map[string]interface{}{
		"message":  "evolution triggered",
		"agent_id": agent.ID,
	})
}

// handleAgentMemory returns agent conversation memory
func (s *Server) handleAgentMemory(w http.ResponseWriter, agentID string) {
	mem := s.memory.Get(agentID)
	if mem == nil {
		http.Error(w, "memory not found", http.StatusNotFound)
		return
	}

	messages := mem.GetMessages()
	s.respondJSON(w, map[string]interface{}{
		"agent_id":      agentID,
		"message_count": len(messages),
		"total_tokens":  mem.TotalTokens,
		"messages":      messages,
	})
}

// handleClearMemory clears agent conversation memory
func (s *Server) handleClearMemory(w http.ResponseWriter, agentID string) {
	mem := s.memory.Get(agentID)
	if mem == nil {
		http.Error(w, "memory not found", http.StatusNotFound)
		return
	}

	mem.Clear()
	if err := s.memory.Save(agentID); err != nil {
		s.logger.Error("failed to save cleared memory", "agent", agentID, "error", err)
	}

	s.respondJSON(w, map[string]interface{}{
		"message":  "memory cleared",
		"agent_id": agentID,
	})
}

// handleModels lists available models
func (s *Server) handleModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	modelList := s.router.ListModels()
	s.respondJSON(w, modelList)
}

// handleCosts returns cost tracking data
func (s *Server) handleCosts(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	costs := s.router.GetAllCosts()
	s.respondJSON(w, costs)
}

// respondJSON writes a JSON response
func (s *Server) respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Error("failed to encode JSON", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}
