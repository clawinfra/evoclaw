package agents

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/clawinfra/evoclaw/internal/config"
)

// Registry manages all agents and their state
type Registry struct {
	agents  map[string]*Agent
	dataDir string
	logger  *slog.Logger
	mu      sync.RWMutex
}

// Agent represents a running agent
type Agent struct {
	ID            string          `json:"id"`
	Def           config.AgentDef `json:"def"`
	Status        string          `json:"status"` // "idle", "running", "error", "evolving"
	StartedAt     time.Time       `json:"started_at"`
	LastActive    time.Time       `json:"last_active"`
	LastHeartbeat time.Time       `json:"last_heartbeat"`
	MessageCount  int64           `json:"message_count"`
	ErrorCount    int64           `json:"error_count"`
	Metrics       Metrics         `json:"metrics"`
	SkillData     *SkillData      `json:"skill_data,omitempty"`
	mu            sync.RWMutex
}

// Metrics tracks agent performance
type Metrics struct {
	TotalActions      int64              `json:"total_actions"`
	SuccessfulActions int64              `json:"successful_actions"`
	FailedActions     int64              `json:"failed_actions"`
	AvgResponseMs     float64            `json:"avg_response_ms"`
	TokensUsed        int64              `json:"tokens_used"`
	CostUSD           float64            `json:"cost_usd"`
	Custom            map[string]float64 `json:"custom,omitempty"`
}

// SkillStatus represents a single skill's status for an agent
type SkillStatus struct {
	Name             string                 `json:"name"`
	Enabled          bool                   `json:"enabled"`
	Capabilities     []string               `json:"capabilities"`
	TickIntervalSecs int                    `json:"tick_interval_secs"`
	LastTick         *time.Time             `json:"last_tick,omitempty"`
	LastReport       map[string]interface{} `json:"last_report,omitempty"`
}

// SkillData tracks skill reports for an agent
type SkillData struct {
	Skills     []SkillStatus            `json:"skills"`
	LastUpdate time.Time                `json:"last_update"`
	Reports    []map[string]interface{} `json:"recent_reports,omitempty"`
}

// NewRegistry creates a new agent registry
func NewRegistry(dataDir string, logger *slog.Logger) (*Registry, error) {
	agentsDir := filepath.Join(dataDir, "agents")
	if err := os.MkdirAll(agentsDir, 0750); err != nil {
		return nil, fmt.Errorf("create agents dir: %w", err)
	}

	return &Registry{
		agents:  make(map[string]*Agent),
		dataDir: agentsDir,
		logger:  logger.With("component", "registry"),
	}, nil
}

// Create adds a new agent to the registry
func (r *Registry) Create(def config.AgentDef) (*Agent, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.agents[def.ID]; exists {
		return nil, fmt.Errorf("agent already exists: %s", def.ID)
	}

	agent := &Agent{
		ID:        def.ID,
		Def:       def,
		Status:    "idle",
		StartedAt: time.Now(),
		Metrics: Metrics{
			Custom: make(map[string]float64),
		},
	}

	r.agents[def.ID] = agent

	// Persist to disk
	if err := r.save(agent); err != nil {
		r.logger.Error("failed to persist agent", "id", def.ID, "error", err)
		// Don't fail creation if save fails
	}

	r.logger.Info("agent created", "id", def.ID, "type", def.Type)
	return agent, nil
}

// Get retrieves an agent by ID
func (r *Registry) Get(id string) (*Agent, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agent, ok := r.agents[id]
	if !ok {
		return nil, fmt.Errorf("agent not found: %s", id)
	}

	return agent, nil
}

// List returns all agents
func (r *Registry) List() []*Agent {
	r.mu.RLock()
	defer r.mu.RUnlock()

	agents := make([]*Agent, 0, len(r.agents))
	for _, a := range r.agents {
		agents = append(agents, a)
	}
	return agents
}

// Update modifies an agent's definition
func (r *Registry) Update(id string, def config.AgentDef) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	agent, ok := r.agents[id]
	if !ok {
		return fmt.Errorf("agent not found: %s", id)
	}

	agent.mu.Lock()
	agent.Def = def
	agent.mu.Unlock()

	if err := r.save(agent); err != nil {
		return fmt.Errorf("save agent: %w", err)
	}

	r.logger.Info("agent updated", "id", id)
	return nil
}

// Delete removes an agent
func (r *Registry) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	agent, ok := r.agents[id]
	if !ok {
		return fmt.Errorf("agent not found: %s", id)
	}

	delete(r.agents, id)

	// Delete from disk
	path := r.agentPath(id)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		r.logger.Error("failed to delete agent file", "id", id, "error", err)
	}

	r.logger.Info("agent deleted", "id", id, "type", agent.Def.Type)
	return nil
}

// UpdateStatus changes an agent's status
func (r *Registry) UpdateStatus(id, status string) error {
	agent, err := r.Get(id)
	if err != nil {
		return err
	}

	agent.mu.Lock()
	agent.Status = status
	agent.LastActive = time.Now()
	agent.mu.Unlock()

	return nil
}

// RecordHeartbeat updates the agent's last heartbeat time
func (r *Registry) RecordHeartbeat(id string) error {
	agent, err := r.Get(id)
	if err != nil {
		return err
	}

	agent.mu.Lock()
	agent.LastHeartbeat = time.Now()
	agent.mu.Unlock()

	return nil
}

// RecordMessage increments message count
func (r *Registry) RecordMessage(id string) error {
	agent, err := r.Get(id)
	if err != nil {
		return err
	}

	agent.mu.Lock()
	agent.MessageCount++
	agent.LastActive = time.Now()
	agent.mu.Unlock()

	return nil
}

// RecordError increments error count
func (r *Registry) RecordError(id string) error {
	agent, err := r.Get(id)
	if err != nil {
		return err
	}

	agent.mu.Lock()
	agent.ErrorCount++
	agent.Metrics.FailedActions++
	agent.mu.Unlock()

	return nil
}

// UpdateMetrics updates agent performance metrics
func (r *Registry) UpdateMetrics(id string, tokensUsed int, costUSD float64, responseMs int64, success bool) error {
	agent, err := r.Get(id)
	if err != nil {
		return err
	}

	agent.mu.Lock()
	defer agent.mu.Unlock()

	agent.Metrics.TotalActions++
	if success {
		agent.Metrics.SuccessfulActions++
	} else {
		agent.Metrics.FailedActions++
	}

	agent.Metrics.TokensUsed += int64(tokensUsed)
	agent.Metrics.CostUSD += costUSD

	// Update running average response time
	n := float64(agent.Metrics.TotalActions)
	agent.Metrics.AvgResponseMs = agent.Metrics.AvgResponseMs*(n-1)/n + float64(responseMs)/n

	return nil
}

// CheckHealth identifies unhealthy agents based on heartbeat
func (r *Registry) CheckHealth(timeoutSec int) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var unhealthy []string
	threshold := time.Now().Add(-time.Duration(timeoutSec) * time.Second)

	for id, agent := range r.agents {
		agent.mu.RLock()
		lastBeat := agent.LastHeartbeat
		agent.mu.RUnlock()

		if !lastBeat.IsZero() && lastBeat.Before(threshold) {
			unhealthy = append(unhealthy, id)
		}
	}

	return unhealthy
}

// Load restores agents from disk
func (r *Registry) Load() error {
	entries, err := os.ReadDir(r.dataDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // No agents yet
		}
		return fmt.Errorf("read agents dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(r.dataDir, entry.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			r.logger.Error("failed to read agent file", "path", path, "error", err)
			continue
		}

		var agent Agent
		if err := json.Unmarshal(data, &agent); err != nil {
			r.logger.Error("failed to parse agent file", "path", path, "error", err)
			continue
		}

		r.mu.Lock()
		r.agents[agent.ID] = &agent
		r.mu.Unlock()

		r.logger.Info("agent loaded", "id", agent.ID, "type", agent.Def.Type)
	}

	return nil
}

// SaveAll persists all agents to disk
func (r *Registry) SaveAll() error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, agent := range r.agents {
		if err := r.save(agent); err != nil {
			r.logger.Error("failed to save agent", "id", agent.ID, "error", err)
		}
	}

	return nil
}

// save writes an agent to disk
func (r *Registry) save(agent *Agent) error {
	agent.mu.RLock()
	defer agent.mu.RUnlock()

	data, err := json.MarshalIndent(agent, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal agent: %w", err)
	}

	path := r.agentPath(agent.ID)
	if err := os.WriteFile(path, data, 0640); err != nil {
		return fmt.Errorf("write agent file: %w", err)
	}

	return nil
}

// agentPath returns the file path for an agent
func (r *Registry) agentPath(id string) string {
	return filepath.Join(r.dataDir, id+".json")
}

// GetSnapshot returns a safe copy of an agent (no mutex)
func (a *Agent) GetSnapshot() Agent {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return Agent{
		ID:            a.ID,
		Def:           a.Def,
		Status:        a.Status,
		StartedAt:     a.StartedAt,
		LastActive:    a.LastActive,
		LastHeartbeat: a.LastHeartbeat,
		MessageCount:  a.MessageCount,
		ErrorCount:    a.ErrorCount,
		Metrics:       a.Metrics,
		SkillData:     a.SkillData,
	}
}

// UpdateSkillData updates the skill data for an agent from a skill report
func (r *Registry) UpdateSkillData(id string, report map[string]interface{}) error {
	agent, err := r.Get(id)
	if err != nil {
		return err
	}

	agent.mu.Lock()
	defer agent.mu.Unlock()

	if agent.SkillData == nil {
		agent.SkillData = &SkillData{
			Skills:  []SkillStatus{},
			Reports: []map[string]interface{}{},
		}
	}

	agent.SkillData.LastUpdate = time.Now()

	// Keep last 50 reports
	agent.SkillData.Reports = append(agent.SkillData.Reports, report)
	if len(agent.SkillData.Reports) > 50 {
		agent.SkillData.Reports = agent.SkillData.Reports[len(agent.SkillData.Reports)-50:]
	}

	return nil
}

// GetSkillData returns the skill data for an agent
func (r *Registry) GetSkillData(id string) (*SkillData, error) {
	agent, err := r.Get(id)
	if err != nil {
		return nil, err
	}

	agent.mu.RLock()
	defer agent.mu.RUnlock()

	if agent.SkillData == nil {
		return &SkillData{
			Skills:  []SkillStatus{},
			Reports: []map[string]interface{}{},
		}, nil
	}

	return agent.SkillData, nil
}
