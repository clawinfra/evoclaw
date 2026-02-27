package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/clawinfra/evoclaw/internal/agents"
	"github.com/clawinfra/evoclaw/internal/channels"
	"github.com/clawinfra/evoclaw/internal/cloud"
	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/models"
	"github.com/clawinfra/evoclaw/internal/orchestrator"
	"github.com/clawinfra/evoclaw/internal/saas"
	"github.com/clawinfra/evoclaw/internal/security"
)

// Server is the HTTP API server
type Server struct {
	port        int
	orch        *orchestrator.Orchestrator
	registry    *agents.Registry
	memory      *agents.MemoryStore
	router      *models.Router
	evolution   interface{} // Evolution engine interface
	logger      *slog.Logger
	httpServer  *http.Server
	webFS       fs.FS // embedded web dashboard assets (optional)
	jwtSecret   []byte
	httpChannel *channels.HTTPChannel // HTTP channel for request-response pairs
	wsChannel   *channels.WSChannel   // WebSocket channel for terminal connections
	wsTimeout   time.Duration         // timeout for WS chat responses (default 30 s)
	cloudMgr    *cloud.Manager        // E2B cloud sandbox manager
	saasSvc     *saas.Service         // Multi-tenant SaaS service
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
	jwtSecret := security.GetJWTSecret()
	if jwtSecret == nil {
		logger.Warn("EVOCLAW_JWT_SECRET not set — running in dev mode (unauthenticated API access)")
	}
	
	// Create and register HTTP channel (only if orchestrator exists)
	httpChannel := channels.NewHTTPChannel()
	wsChannel := channels.NewWSChannel(logger)
	if orch != nil {
		orch.RegisterChannel(httpChannel)
		orch.RegisterChannel(wsChannel)
	}

	return &Server{
		port:        port,
		orch:        orch,
		registry:    registry,
		memory:      memory,
		router:      router,
		logger:      logger.With("component", "api"),
		jwtSecret:   jwtSecret,
		httpChannel: httpChannel,
		wsChannel:   wsChannel,
		wsTimeout:   30 * time.Second,
	}
}

// SetWebFS sets the embedded filesystem for the web dashboard
func (s *Server) SetWebFS(webFS fs.FS) {
	s.webFS = webFS
}

// SetEvolution sets the evolution engine interface
func (s *Server) SetEvolution(evolution interface{}) {
	s.evolution = evolution
}

// Start starts the HTTP server
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	// Auth endpoint (unauthenticated — before auth middleware)
	mux.HandleFunc("/api/auth/token", s.handleAuthToken)

	// WebSocket terminal (auth handled inside handler via ?token= param)
	mux.HandleFunc("/api/terminal/ws", s.handleTerminalWS)

	// Terminal web UI
	mux.HandleFunc("/terminal", s.handleTerminalPage)
	
	// Register API routes (protected by auth middleware applied at handler level)
	mux.HandleFunc("/api/status", s.handleStatus)
	mux.HandleFunc("/api/chat", s.handleChat)
	mux.HandleFunc("/api/chat/stream", s.handleChatStream)
	mux.HandleFunc("/api/agents", s.handleAgents)
	mux.HandleFunc("/api/agents/", s.handleAgentDetail)
	mux.HandleFunc("/api/models", s.handleModels)
	mux.HandleFunc("/api/costs", s.handleCosts)
	mux.HandleFunc("/api/dashboard", s.handleDashboard)
	mux.HandleFunc("/api/logs/stream", s.handleLogStream)
	mux.HandleFunc("/api/memory/stats", s.handleMemoryStats)
	mux.HandleFunc("/api/memory/tree", s.handleMemoryTree)
	mux.HandleFunc("/api/memory/retrieve", s.handleMemoryRetrieve)
	
	// Scheduler API routes
	mux.HandleFunc("/api/scheduler/status", s.handleSchedulerStatus)
	mux.HandleFunc("/api/scheduler/jobs", s.handleSchedulerJobs)
	mux.HandleFunc("/api/scheduler/jobs/", s.handleSchedulerJobRoutes)
	
	// Genome API routes
	mux.HandleFunc("/api/agents/{id}/genome", s.handleGenomeRoutes)
	mux.HandleFunc("/api/agents/{id}/genome/skills/{skill}", s.handleSkillRoutes)
	mux.HandleFunc("/api/agents/{id}/genome/skills/{skill}/params", s.handleUpdateSkillParams)
	mux.HandleFunc("/api/agents/{id}/genome/constraints", s.handleConstraintRoutes)
	
	// Layer 3: Behavioral Evolution API routes
	mux.HandleFunc("/api/agents/{id}/feedback", s.handleFeedbackRoutes)
	mux.HandleFunc("/api/agents/{id}/genome/behavior", s.handleBehaviorRoutes)
	mux.HandleFunc("/api/agents/{id}/behavior/history", s.handleBehaviorHistoryRoutes)

	// Security Layer 3: Evolution Firewall API routes
	mux.HandleFunc("/api/agents/{id}/firewall", s.handleFirewallStatus)
	mux.HandleFunc("/api/agents/{id}/firewall/rollback", s.handleFirewallRollback)
	mux.HandleFunc("/api/agents/{id}/firewall/reset", s.handleFirewallReset)

	// Serve embedded web dashboard
	if s.webFS != nil {
		fileServer := http.FileServer(http.FS(s.webFS))
		mux.Handle("/", fileServer)
		s.logger.Info("web dashboard enabled at /")
	}

	// Apply JWT auth middleware to all routes (skips /api/auth/token via path check)
	authedHandler := s.jwtAuthWrapper(mux)

	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf(":%d", s.port),
		Handler:      s.corsMiddleware(s.loggingMiddleware(authedHandler)),
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // 0 = no write timeout (required for long-lived WS connections)
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

// AgentRegisterRequest is the JSON body for POST /api/agents/register
type AgentRegisterRequest struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Host string `json:"host"`
}

// AgentRegisterResponse is the JSON response for POST /api/agents/register
type AgentRegisterResponse struct {
	Status     string `json:"status"`
	ID         string `json:"id"`
	MQTTBroker string `json:"mqtt_broker"`
	MQTTPort   int    `json:"mqtt_port"`
}

// handleAgentRegister handles POST /api/agents/register for edge agent self-registration
func (s *Server) handleAgentRegister(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AgentRegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	if req.ID == "" {
		http.Error(w, "id is required", http.StatusBadRequest)
		return
	}
	if req.Type == "" {
		req.Type = "monitor"
	}

	// Register the agent in the registry (create if not exists)
	agentDef := config.AgentDef{
		ID:   req.ID,
		Name: req.ID,
		Type: req.Type,
		Config: map[string]string{
			"host":        req.Host,
			"registered":  "dynamic",
		},
	}

	if _, err := s.registry.Get(req.ID); err != nil {
		// Agent doesn't exist, create it
		if _, err := s.registry.Create(agentDef); err != nil {
			s.logger.Error("failed to register agent", "id", req.ID, "error", err)
			http.Error(w, "failed to register agent", http.StatusInternalServerError)
			return
		}
	} else {
		// Agent already exists, update it
		if err := s.registry.Update(req.ID, agentDef); err != nil {
			s.logger.Error("failed to update agent", "id", req.ID, "error", err)
			http.Error(w, "failed to update agent", http.StatusInternalServerError)
			return
		}
	}

	s.logger.Info("agent registered via API", "id", req.ID, "type", req.Type, "host", req.Host)

	resp := AgentRegisterResponse{
		Status:     "registered",
		ID:         req.ID,
		MQTTBroker: s.getMQTTBroker(),
		MQTTPort:   s.getMQTTPort(),
	}

	w.WriteHeader(http.StatusCreated)
	s.respondJSON(w, resp)
}

// getMQTTBroker returns the MQTT broker address from orchestrator config
func (s *Server) getMQTTBroker() string {
	if s.orch != nil {
		cfg := s.orch.GetConfig()
		if cfg != nil && cfg.MQTT.Host != "" {
			return cfg.MQTT.Host
		}
	}
	return "localhost"
}

// getMQTTPort returns the MQTT broker port from orchestrator config
func (s *Server) getMQTTPort() int {
	if s.orch != nil {
		cfg := s.orch.GetConfig()
		if cfg != nil && cfg.MQTT.Port > 0 {
			return cfg.MQTT.Port
		}
	}
	return 1883
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
	case action == "evolution" && r.Method == http.MethodGet:
		s.handleAgentEvolution(w, r)
	case action == "memory" && r.Method == http.MethodGet:
		s.handleAgentMemory(w, agentID)
	case action == "memory" && r.Method == http.MethodDelete:
		s.handleClearMemory(w, agentID)
	case action == "skills" && r.Method == http.MethodGet:
		s.handleAgentSkills(w, agentID)
	case action == "" && r.Method == http.MethodGet:
		// Get agent details
		s.respondJSON(w, agent.GetSnapshot())
	case action == "" && r.Method == http.MethodPatch:
		// Update agent settings
		s.handleAgentUpdate(w, r, agentID, agent)
	default:
		http.Error(w, "invalid action or method", http.StatusBadRequest)
	}
}

// handleAgentUpdate updates agent settings (model, name, type)
func (s *Server) handleAgentUpdate(w http.ResponseWriter, r *http.Request, agentID string, agent *agents.Agent) {
	var update struct {
		Name  string `json:"name"`
		Type  string `json:"type"`
		Model string `json:"model"`
	}
	
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}

	// Validate model if specified
	if update.Model != "" {
		models := s.router.ListModels()
		modelExists := false
		for _, m := range models {
			if m.ID == update.Model {
				modelExists = true
				break
			}
		}
		if !modelExists {
			http.Error(w, fmt.Sprintf("model not found: %s", update.Model), http.StatusBadRequest)
			return
		}
	}

	// Get current definition and update
	def := agent.Def
	if update.Name != "" {
		def.Name = update.Name
	}
	if update.Type != "" {
		def.Type = update.Type
	}
	if update.Model != "" {
		def.Model = update.Model
	}
	
	// Apply update via registry
	if err := s.registry.Update(agentID, def); err != nil {
		http.Error(w, fmt.Sprintf("failed to update agent: %v", err), http.StatusInternalServerError)
		return
	}

	s.logger.Info("agent updated", "agent", agentID, "model", def.Model)

	s.respondJSON(w, map[string]interface{}{
		"status": "ok",
		"agent":  agentID,
		"name":   def.Name,
		"type":   def.Type,
		"model":  def.Model,
	})
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

// handleAgentSkills returns skill data for an agent
func (s *Server) handleAgentSkills(w http.ResponseWriter, agentID string) {
	skillData, err := s.registry.GetSkillData(agentID)
	if err != nil {
		http.Error(w, "agent not found", http.StatusNotFound)
		return
	}

	s.respondJSON(w, map[string]interface{}{
		"agent_id":       agentID,
		"skills":         skillData.Skills,
		"last_update":    skillData.LastUpdate,
		"recent_reports": skillData.Reports,
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

// jwtAuthWrapper applies JWT authentication to all /api/ routes except /api/auth/token.
func (s *Server) jwtAuthWrapper(next http.Handler) http.Handler {
	authMW := security.AuthMiddleware(s.jwtSecret)
	authed := authMW(next)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Skip auth for token endpoint, WS terminal (has own auth), and non-API routes
		if r.URL.Path == "/api/auth/token" ||
			r.URL.Path == "/api/terminal/ws" ||
			!strings.HasPrefix(r.URL.Path, "/api/") {
			next.ServeHTTP(w, r)
			return
		}
		authed.ServeHTTP(w, r)
	})
}

// handleAuthToken generates a JWT token. In production, this should validate
// API keys or owner credentials. For now it accepts a JSON body with agent_id and role.
func (s *Server) handleAuthToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		AgentID string `json:"agent_id"`
		Role    string `json:"role"`
		APIKey  string `json:"api_key"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, `{"error":"invalid request body"}`, http.StatusBadRequest)
		return
	}

	if req.AgentID == "" || req.Role == "" {
		http.Error(w, `{"error":"agent_id and role required"}`, http.StatusBadRequest)
		return
	}

	// Validate role
	validRole := false
	for _, r := range security.ValidRoles {
		if r == req.Role {
			validRole = true
			break
		}
	}
	if !validRole {
		http.Error(w, `{"error":"invalid role"}`, http.StatusBadRequest)
		return
	}

	secret := s.jwtSecret
	if secret == nil {
		// Dev mode: use a default secret for token generation
		secret = []byte("evoclaw-dev-secret")
	}

	token, err := security.GenerateToken(req.AgentID, req.Role, secret, 24*time.Hour)
	if err != nil {
		s.logger.Error("failed to generate token", "error", err)
		http.Error(w, `{"error":"token generation failed"}`, http.StatusInternalServerError)
		return
	}

	s.respondJSON(w, map[string]interface{}{
		"token":      token,
		"expires_in": 86400,
		"token_type": "Bearer",
	})
}

// respondJSON writes a JSON response
func (s *Server) respondJSON(w http.ResponseWriter, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(data); err != nil {
		s.logger.Error("failed to encode JSON", "error", err)
		http.Error(w, "internal server error", http.StatusInternalServerError)
	}
}

// generateMessageID creates a unique message ID
func generateMessageID() string {
	return "msg_" + strings.ReplaceAll(time.Now().Format("20060102150405.000000"), ".", "_")
}
