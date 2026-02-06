package saas

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/clawinfra/evoclaw/internal/cloud"
)

// Service is the Agent-as-a-Service business logic layer.
type Service struct {
	store    *TenantStore
	cloudMgr *cloud.Manager
	logger   *slog.Logger
}

// NewService creates a new SaaS service.
func NewService(store *TenantStore, cloudMgr *cloud.Manager, logger *slog.Logger) *Service {
	return &Service{
		store:    store,
		cloudMgr: cloudMgr,
		logger:   logger.With("component", "saas"),
	}
}

// Register creates a new tenant account.
func (s *Service) Register(req RegisterRequest) (*User, error) {
	user, err := s.store.Register(req)
	if err != nil {
		return nil, fmt.Errorf("register user: %w", err)
	}

	s.logger.Info("new user registered",
		"user_id", user.ID,
		"email", user.Email,
		"max_agents", user.MaxAgents,
	)

	return user, nil
}

// SpawnAgent creates a new cloud agent for a user.
func (s *Service) SpawnAgent(ctx context.Context, userID string, req SpawnRequest) (*UserAgent, error) {
	// Validate user
	user, err := s.store.GetUser(userID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	// Check limits
	if s.store.IsUserOverLimit(userID) {
		return nil, fmt.Errorf("agent limit reached (%d max)", user.MaxAgents)
	}
	if s.store.IsUserOverBudget(userID) {
		return nil, fmt.Errorf("credit limit reached ($%.2f)", user.CreditLimitUSD)
	}

	// Handle burst mode
	if req.Mode == "burst" && req.Count > 1 {
		return s.spawnBurst(ctx, user, req)
	}

	// Generate agent ID if not provided
	agentID := req.AgentID
	if agentID == "" {
		agentID = generateID("agent")
	}

	agentType := req.AgentType
	if agentType == "" {
		agentType = "trader"
	}

	// Build cloud config
	cloudConfig := cloud.AgentConfig{
		AgentID:   agentID,
		AgentType: agentType,
		UserID:    userID,
		Genome:    req.Genome,
		EnvVars:   make(map[string]string),
	}

	// Inject user's trading credentials
	if user.HyperliquidAPIKey != "" {
		cloudConfig.EnvVars["HYPERLIQUID_API_KEY"] = user.HyperliquidAPIKey
	}
	if user.HyperliquidAPISecret != "" {
		cloudConfig.EnvVars["HYPERLIQUID_API_SECRET"] = user.HyperliquidAPISecret
	}

	// Use user's genome if request doesn't specify one
	if cloudConfig.Genome == "" && user.DefaultGenome != "" {
		cloudConfig.Genome = user.DefaultGenome
	}

	// Spawn the sandbox
	sandbox, err := s.cloudMgr.SpawnAgent(ctx, cloudConfig)
	if err != nil {
		return nil, fmt.Errorf("spawn agent: %w", err)
	}

	// Track the agent
	userAgent := UserAgent{
		SandboxID: sandbox.SandboxID,
		AgentID:   agentID,
		UserID:    userID,
		AgentType: agentType,
		Status:    "running",
		CreatedAt: time.Now(),
		Mode:      req.Mode,
	}
	if userAgent.Mode == "" {
		userAgent.Mode = "on-demand"
	}
	s.store.TrackAgent(userAgent)

	s.logger.Info("user agent spawned",
		"user_id", userID,
		"agent_id", agentID,
		"sandbox_id", sandbox.SandboxID,
		"mode", userAgent.Mode,
	)

	return &userAgent, nil
}

// spawnBurst creates multiple agents simultaneously for tournament evolution.
func (s *Service) spawnBurst(ctx context.Context, user *User, req SpawnRequest) (*UserAgent, error) {
	count := req.Count
	if count > user.MaxAgents {
		count = user.MaxAgents
	}

	currentCount := s.store.UserAgentCount(user.ID)
	available := user.MaxAgents - currentCount
	if count > available {
		count = available
	}
	if count <= 0 {
		return nil, fmt.Errorf("no agent slots available")
	}

	configs := make([]cloud.AgentConfig, count)
	for i := 0; i < count; i++ {
		agentID := generateID("burst")
		agentType := req.AgentType
		if agentType == "" {
			agentType = "trader"
		}

		configs[i] = cloud.AgentConfig{
			AgentID:   agentID,
			AgentType: agentType,
			UserID:    user.ID,
			Genome:    req.Genome,
			EnvVars:   make(map[string]string),
		}
		if user.HyperliquidAPIKey != "" {
			configs[i].EnvVars["HYPERLIQUID_API_KEY"] = user.HyperliquidAPIKey
		}
	}

	sandboxes, errs := s.cloudMgr.SpawnBurst(ctx, configs)

	var firstAgent *UserAgent
	for i, sandbox := range sandboxes {
		if errs[i] != nil {
			s.logger.Error("burst spawn failed", "index", i, "error", errs[i])
			continue
		}

		userAgent := UserAgent{
			SandboxID: sandbox.SandboxID,
			AgentID:   configs[i].AgentID,
			UserID:    user.ID,
			AgentType: configs[i].AgentType,
			Status:    "running",
			CreatedAt: time.Now(),
			Mode:      "burst",
		}
		s.store.TrackAgent(userAgent)

		if firstAgent == nil {
			ua := userAgent
			firstAgent = &ua
		}
	}

	if firstAgent == nil {
		return nil, fmt.Errorf("all burst spawns failed")
	}

	s.logger.Info("burst spawn completed",
		"user_id", user.ID,
		"requested", req.Count,
		"spawned", count,
	)

	return firstAgent, nil
}

// KillAgent terminates a user's agent.
func (s *Service) KillAgent(ctx context.Context, userID, sandboxID string) error {
	// Verify ownership
	agents := s.store.GetUserAgents(userID)
	found := false
	for _, a := range agents {
		if a.SandboxID == sandboxID {
			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("agent not found or not owned by user")
	}

	if err := s.cloudMgr.KillAgent(ctx, sandboxID); err != nil {
		return fmt.Errorf("kill agent: %w", err)
	}

	s.store.RemoveAgent(sandboxID)

	s.logger.Info("user agent killed",
		"user_id", userID,
		"sandbox_id", sandboxID,
	)

	return nil
}

// ListAgents returns all agents belonging to a user.
func (s *Service) ListAgents(userID string) ([]UserAgent, error) {
	_, err := s.store.GetUser(userID)
	if err != nil {
		return nil, fmt.Errorf("user not found: %w", err)
	}

	return s.store.GetUserAgents(userID), nil
}

// GetUsage returns usage report for a user.
func (s *Service) GetUsage(userID string) (*UsageReport, error) {
	return s.store.GetUsage(userID)
}

// AuthenticateAPIKey validates an API key and returns the user.
func (s *Service) AuthenticateAPIKey(apiKey string) (*User, error) {
	return s.store.GetUserByAPIKey(apiKey)
}
