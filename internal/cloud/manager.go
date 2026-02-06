package cloud

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// ManagerConfig holds configuration for the cloud agent manager.
type ManagerConfig struct {
	// E2BAPIKey is the E2B API key for authentication.
	E2BAPIKey string `json:"e2b_api_key"`

	// DefaultTemplate is the E2B template to use when not specified.
	DefaultTemplate string `json:"default_template"`

	// DefaultTimeoutSec is the default sandbox lifetime.
	DefaultTimeoutSec int `json:"default_timeout_sec"`

	// MaxAgents is the maximum number of concurrent cloud agents.
	MaxAgents int `json:"max_agents"`

	// HealthCheckIntervalSec is how often to check agent health.
	HealthCheckIntervalSec int `json:"health_check_interval_sec"`

	// KeepAliveIntervalSec is how often to extend sandbox timeouts.
	KeepAliveIntervalSec int `json:"keep_alive_interval_sec"`

	// MQTTBroker is the default MQTT broker for agents.
	MQTTBroker string `json:"mqtt_broker"`

	// MQTTPort is the default MQTT port.
	MQTTPort int `json:"mqtt_port"`

	// OrchestratorURL is the orchestrator API URL.
	OrchestratorURL string `json:"orchestrator_url"`

	// CreditBudgetUSD is the maximum E2B spending limit.
	CreditBudgetUSD float64 `json:"credit_budget_usd"`
}

// DefaultManagerConfig returns sensible defaults.
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		DefaultTemplate:        "evoclaw-agent",
		DefaultTimeoutSec:      300,
		MaxAgents:              10,
		HealthCheckIntervalSec: 60,
		KeepAliveIntervalSec:   120,
		MQTTBroker:             "localhost",
		MQTTPort:               1883,
		OrchestratorURL:        "http://localhost:8420",
		CreditBudgetUSD:        50.0,
	}
}

// CostTracker tracks E2B credit usage.
type CostTracker struct {
	mu                sync.RWMutex
	totalSandboxes    int64
	totalUptimeSec    int64
	estimatedCostUSD  float64
	budgetUSD         float64
	costPerSecondUSD  float64 // E2B pricing: ~$0.0001/sec for 1 vCPU 256MB
}

// CostSnapshot is a point-in-time view of costs.
type CostSnapshot struct {
	TotalSandboxes   int64   `json:"total_sandboxes"`
	ActiveSandboxes  int     `json:"active_sandboxes"`
	TotalUptimeSec   int64   `json:"total_uptime_sec"`
	EstimatedCostUSD float64 `json:"estimated_cost_usd"`
	BudgetUSD        float64 `json:"budget_usd"`
	BudgetRemaining  float64 `json:"budget_remaining"`
}

// Manager orchestrates cloud agents via E2B sandboxes.
type Manager struct {
	client  *E2BClient
	config  ManagerConfig
	costs   *CostTracker
	logger  *slog.Logger
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.RWMutex
	started bool
}

// NewManager creates a new cloud agent manager.
func NewManager(config ManagerConfig, logger *slog.Logger) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	client := NewE2BClient(config.E2BAPIKey)

	return &Manager{
		client: client,
		config: config,
		costs: &CostTracker{
			budgetUSD:        config.CreditBudgetUSD,
			costPerSecondUSD: 0.0001, // ~$0.36/hr per sandbox
		},
		logger: logger.With("component", "cloud-manager"),
		ctx:    ctx,
		cancel: cancel,
	}
}

// NewManagerWithClient creates a manager with a custom E2B client (for testing).
func NewManagerWithClient(config ManagerConfig, client *E2BClient, logger *slog.Logger) *Manager {
	ctx, cancel := context.WithCancel(context.Background())

	return &Manager{
		client: client,
		config: config,
		costs: &CostTracker{
			budgetUSD:        config.CreditBudgetUSD,
			costPerSecondUSD: 0.0001,
		},
		logger: logger.With("component", "cloud-manager"),
		ctx:    ctx,
		cancel: cancel,
	}
}

// Start begins the manager's background goroutines (health checks, keep-alive).
func (m *Manager) Start() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.started {
		return fmt.Errorf("manager already started")
	}

	m.started = true
	m.logger.Info("cloud agent manager started",
		"maxAgents", m.config.MaxAgents,
		"budget", m.config.CreditBudgetUSD,
	)

	// Background health checker
	go m.healthCheckLoop()

	// Background keep-alive
	go m.keepAliveLoop()

	// Background cost tracker
	go m.costTrackingLoop()

	return nil
}

// Stop gracefully shuts down the manager and all cloud agents.
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if !m.started {
		return nil
	}

	m.logger.Info("stopping cloud agent manager, draining agents...")

	m.cancel()

	// Kill all running sandboxes
	sandboxes, err := m.client.ListAgents(context.Background())
	if err != nil {
		m.logger.Error("failed to list agents during shutdown", "error", err)
	} else {
		for _, s := range sandboxes {
			m.logger.Info("killing sandbox", "id", s.SandboxID, "agent", s.AgentID)
			if err := m.client.KillAgent(context.Background(), s.SandboxID); err != nil {
				m.logger.Error("failed to kill sandbox", "id", s.SandboxID, "error", err)
			}
		}
	}

	m.started = false
	m.logger.Info("cloud agent manager stopped")
	return nil
}

// SpawnAgent creates a new cloud agent.
func (m *Manager) SpawnAgent(ctx context.Context, config AgentConfig) (*Sandbox, error) {
	// Check agent limit
	if m.client.LocalSandboxCount() >= m.config.MaxAgents {
		return nil, fmt.Errorf("agent limit reached (%d/%d)", m.client.LocalSandboxCount(), m.config.MaxAgents)
	}

	// Check budget
	if m.IsBudgetExhausted() {
		return nil, fmt.Errorf("E2B credit budget exhausted (spent $%.2f of $%.2f)",
			m.costs.estimatedCostUSD, m.costs.budgetUSD)
	}

	// Apply defaults
	if config.TemplateID == "" {
		config.TemplateID = m.config.DefaultTemplate
	}
	if config.TimeoutSec <= 0 {
		config.TimeoutSec = m.config.DefaultTimeoutSec
	}
	if config.MQTTBroker == "" {
		config.MQTTBroker = m.config.MQTTBroker
	}
	if config.MQTTPort == 0 {
		config.MQTTPort = m.config.MQTTPort
	}
	if config.OrchestratorURL == "" {
		config.OrchestratorURL = m.config.OrchestratorURL
	}

	sandbox, err := m.client.SpawnAgent(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("spawn agent: %w", err)
	}

	// Track cost
	m.costs.mu.Lock()
	m.costs.totalSandboxes++
	m.costs.mu.Unlock()

	m.logger.Info("cloud agent spawned",
		"sandbox_id", sandbox.SandboxID,
		"agent_id", sandbox.AgentID,
		"template", sandbox.TemplateID,
	)

	return sandbox, nil
}

// KillAgent terminates a cloud agent sandbox.
func (m *Manager) KillAgent(ctx context.Context, sandboxID string) error {
	// Record uptime before killing
	if s, ok := m.client.GetLocalSandbox(sandboxID); ok {
		uptimeSec := int64(time.Since(s.StartedAt).Seconds())
		m.costs.mu.Lock()
		m.costs.totalUptimeSec += uptimeSec
		m.costs.estimatedCostUSD += float64(uptimeSec) * m.costs.costPerSecondUSD
		m.costs.mu.Unlock()
	}

	if err := m.client.KillAgent(ctx, sandboxID); err != nil {
		return fmt.Errorf("kill agent: %w", err)
	}

	m.logger.Info("cloud agent killed", "sandbox_id", sandboxID)
	return nil
}

// ListAgents returns all running cloud agents.
func (m *Manager) ListAgents(ctx context.Context) ([]Sandbox, error) {
	return m.client.ListAgents(ctx)
}

// GetAgentStatus returns the status of a specific cloud agent.
func (m *Manager) GetAgentStatus(ctx context.Context, sandboxID string) (*Status, error) {
	return m.client.GetAgentStatus(ctx, sandboxID)
}

// SendCommand executes a command in a cloud agent's sandbox.
func (m *Manager) SendCommand(ctx context.Context, sandboxID string, cmd Command) (*CommandResponse, error) {
	return m.client.SendCommand(ctx, sandboxID, cmd)
}

// GetCosts returns the current cost snapshot.
func (m *Manager) GetCosts() CostSnapshot {
	m.costs.mu.RLock()
	defer m.costs.mu.RUnlock()

	return CostSnapshot{
		TotalSandboxes:   m.costs.totalSandboxes,
		ActiveSandboxes:  m.client.LocalSandboxCount(),
		TotalUptimeSec:   m.costs.totalUptimeSec,
		EstimatedCostUSD: m.costs.estimatedCostUSD,
		BudgetUSD:        m.costs.budgetUSD,
		BudgetRemaining:  m.costs.budgetUSD - m.costs.estimatedCostUSD,
	}
}

// IsBudgetExhausted returns true if the spending limit has been reached.
func (m *Manager) IsBudgetExhausted() bool {
	m.costs.mu.RLock()
	defer m.costs.mu.RUnlock()
	return m.costs.estimatedCostUSD >= m.costs.budgetUSD
}

// SpawnBurst creates N agents simultaneously for tournament evolution.
func (m *Manager) SpawnBurst(ctx context.Context, configs []AgentConfig) ([]*Sandbox, []error) {
	results := make([]*Sandbox, len(configs))
	errors := make([]error, len(configs))

	var wg sync.WaitGroup
	for i, cfg := range configs {
		wg.Add(1)
		go func(idx int, c AgentConfig) {
			defer wg.Done()
			sandbox, err := m.SpawnAgent(ctx, c)
			results[idx] = sandbox
			errors[idx] = err
		}(i, cfg)
	}
	wg.Wait()

	return results, errors
}

// KillAll terminates all running cloud agents.
func (m *Manager) KillAll(ctx context.Context) (int, error) {
	sandboxes, err := m.client.ListAgents(ctx)
	if err != nil {
		return 0, fmt.Errorf("list agents: %w", err)
	}

	killed := 0
	for _, s := range sandboxes {
		if err := m.KillAgent(ctx, s.SandboxID); err != nil {
			m.logger.Error("failed to kill sandbox", "id", s.SandboxID, "error", err)
			continue
		}
		killed++
	}

	return killed, nil
}

// IsStarted returns whether the manager is running.
func (m *Manager) IsStarted() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.started
}

// healthCheckLoop periodically checks sandbox health and restarts unhealthy ones.
func (m *Manager) healthCheckLoop() {
	interval := time.Duration(m.config.HealthCheckIntervalSec) * time.Second
	if interval <= 0 {
		interval = 60 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.runHealthCheck()
		}
	}
}

// runHealthCheck checks all sandboxes and restarts unhealthy ones.
func (m *Manager) runHealthCheck() {
	sandboxes, err := m.client.ListAgents(m.ctx)
	if err != nil {
		m.logger.Error("health check: failed to list agents", "error", err)
		return
	}

	for _, s := range sandboxes {
		status, err := m.client.GetAgentStatus(m.ctx, s.SandboxID)
		if err != nil {
			m.logger.Warn("health check: failed to get status",
				"sandbox_id", s.SandboxID,
				"error", err,
			)
			continue
		}

		if !status.Healthy {
			m.logger.Warn("unhealthy agent detected, will restart",
				"sandbox_id", s.SandboxID,
				"agent_id", s.AgentID,
			)
			// TODO: Implement auto-restart with same config
		}
	}
}

// keepAliveLoop extends sandbox timeouts to prevent premature termination.
func (m *Manager) keepAliveLoop() {
	interval := time.Duration(m.config.KeepAliveIntervalSec) * time.Second
	if interval <= 0 {
		interval = 120 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.refreshTimeouts()
		}
	}
}

// refreshTimeouts extends the timeout for all running sandboxes.
func (m *Manager) refreshTimeouts() {
	sandboxes, err := m.client.ListAgents(m.ctx)
	if err != nil {
		m.logger.Error("keep-alive: failed to list agents", "error", err)
		return
	}

	for _, s := range sandboxes {
		if err := m.client.SetTimeout(m.ctx, s.SandboxID, m.config.DefaultTimeoutSec); err != nil {
			m.logger.Warn("keep-alive: failed to refresh timeout",
				"sandbox_id", s.SandboxID,
				"error", err,
			)
		}
	}
}

// costTrackingLoop periodically updates cost estimates for running sandboxes.
func (m *Manager) costTrackingLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.updateCosts()
		}
	}
}

// updateCosts recalculates estimated costs based on active sandboxes.
func (m *Manager) updateCosts() {
	sandboxes, err := m.client.ListAgents(m.ctx)
	if err != nil {
		return
	}

	m.costs.mu.Lock()
	defer m.costs.mu.Unlock()

	// Add 30 seconds of cost per active sandbox
	activeCost := float64(len(sandboxes)) * 30.0 * m.costs.costPerSecondUSD
	m.costs.estimatedCostUSD += activeCost
	m.costs.totalUptimeSec += int64(len(sandboxes)) * 30

	if m.costs.estimatedCostUSD >= m.costs.budgetUSD*0.9 {
		m.logger.Warn("approaching E2B budget limit",
			"spent", m.costs.estimatedCostUSD,
			"budget", m.costs.budgetUSD,
		)
	}
}
