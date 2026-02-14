package governance

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// VFMEntry represents a single cost tracking entry.
type VFMEntry struct {
	Timestamp   time.Time `json:"timestamp"`
	AgentID     string    `json:"agent_id"`
	Model       string    `json:"model"`
	InputTokens int       `json:"input_tokens"`
	OutputTokens int      `json:"output_tokens"`
	CostUSD     float64   `json:"cost_usd"`
	TaskType    string    `json:"task_type,omitempty"` // simple, medium, complex, critical
	Value       string    `json:"value,omitempty"`     // description of value delivered
}

// VFMStats represents cost statistics for an agent.
type VFMStats struct {
	TotalCostUSD     float64            `json:"total_cost_usd"`
	TotalInputTokens int64              `json:"total_input_tokens"`
	TotalOutputTokens int64             `json:"total_output_tokens"`
	RequestCount     int64              `json:"request_count"`
	CostByModel      map[string]float64 `json:"cost_by_model"`
	CostByTaskType   map[string]float64 `json:"cost_by_task_type"`
	AvgCostPerRequest float64           `json:"avg_cost_per_request"`
	Period           string             `json:"period"` // e.g., "2026-02" for monthly
}

// VFMBudget represents spending limits.
type VFMBudget struct {
	DailyLimitUSD   float64 `json:"daily_limit_usd"`
	MonthlyLimitUSD float64 `json:"monthly_limit_usd"`
	AlertThreshold  float64 `json:"alert_threshold"` // 0.0-1.0, alert when this % of budget used
}

// VFM implements Value-For-Money tracking protocol.
type VFM struct {
	baseDir string
	logger  *slog.Logger
	mu      sync.RWMutex
	budgets map[string]*VFMBudget
	
	// In-memory daily stats cache
	dailyStats map[string]*VFMStats // key: agentID:date
}

// NewVFM creates a new VFM instance.
func NewVFM(baseDir string, logger *slog.Logger) (*VFM, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("create VFM directory: %w", err)
	}
	return &VFM{
		baseDir:    baseDir,
		logger:     logger.With("component", "vfm"),
		budgets:    make(map[string]*VFMBudget),
		dailyStats: make(map[string]*VFMStats),
	}, nil
}

func (v *VFM) logPath(agentID string) string {
	return filepath.Join(v.baseDir, agentID+"_costs.jsonl")
}

func (v *VFM) budgetPath(agentID string) string {
	return filepath.Join(v.baseDir, agentID+"_budget.json")
}

// SetBudget sets spending limits for an agent.
func (v *VFM) SetBudget(agentID string, budget *VFMBudget) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	v.budgets[agentID] = budget

	data, err := json.MarshalIndent(budget, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal budget: %w", err)
	}

	if err := os.WriteFile(v.budgetPath(agentID), data, 0644); err != nil {
		return fmt.Errorf("write budget: %w", err)
	}

	v.logger.Info("VFM budget set", "agent", agentID, "daily", budget.DailyLimitUSD, "monthly", budget.MonthlyLimitUSD)
	return nil
}

// TrackCost records a cost entry.
func (v *VFM) TrackCost(agentID, model string, inputTokens, outputTokens int, costUSD float64) error {
	return v.TrackCostWithMeta(agentID, model, inputTokens, outputTokens, costUSD, "", "")
}

// TrackCostWithMeta records a cost entry with task metadata.
func (v *VFM) TrackCostWithMeta(agentID, model string, inputTokens, outputTokens int, costUSD float64, taskType, value string) error {
	v.mu.Lock()
	defer v.mu.Unlock()

	entry := VFMEntry{
		Timestamp:    time.Now(),
		AgentID:      agentID,
		Model:        model,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		CostUSD:      costUSD,
		TaskType:     taskType,
		Value:        value,
	}

	f, err := os.OpenFile(v.logPath(agentID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open cost log: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write entry: %w", err)
	}

	// Update daily cache
	dateKey := fmt.Sprintf("%s:%s", agentID, time.Now().Format("2006-01-02"))
	stats, ok := v.dailyStats[dateKey]
	if !ok {
		stats = &VFMStats{
			CostByModel:    make(map[string]float64),
			CostByTaskType: make(map[string]float64),
			Period:         time.Now().Format("2006-01-02"),
		}
		v.dailyStats[dateKey] = stats
	}
	stats.TotalCostUSD += costUSD
	stats.TotalInputTokens += int64(inputTokens)
	stats.TotalOutputTokens += int64(outputTokens)
	stats.RequestCount++
	stats.CostByModel[model] += costUSD
	if taskType != "" {
		stats.CostByTaskType[taskType] += costUSD
	}

	v.logger.Debug("VFM tracked", "agent", agentID, "model", model, "cost", costUSD, "daily_total", stats.TotalCostUSD)

	// Check budget alerts
	if budget, ok := v.budgets[agentID]; ok {
		if budget.DailyLimitUSD > 0 && stats.TotalCostUSD >= budget.DailyLimitUSD*budget.AlertThreshold {
			v.logger.Warn("VFM budget alert",
				"agent", agentID,
				"daily_spent", stats.TotalCostUSD,
				"daily_limit", budget.DailyLimitUSD,
				"threshold", budget.AlertThreshold)
		}
	}

	return nil
}

// GetStats returns cost statistics for an agent.
func (v *VFM) GetStats(agentID string) (*VFMStats, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	f, err := os.Open(v.logPath(agentID))
	if err != nil {
		if os.IsNotExist(err) {
			return &VFMStats{
				CostByModel:    make(map[string]float64),
				CostByTaskType: make(map[string]float64),
			}, nil
		}
		return nil, fmt.Errorf("open cost log: %w", err)
	}
	defer f.Close()

	stats := &VFMStats{
		CostByModel:    make(map[string]float64),
		CostByTaskType: make(map[string]float64),
	}

	decoder := json.NewDecoder(f)
	for {
		var entry VFMEntry
		if err := decoder.Decode(&entry); err != nil {
			break
		}
		stats.TotalCostUSD += entry.CostUSD
		stats.TotalInputTokens += int64(entry.InputTokens)
		stats.TotalOutputTokens += int64(entry.OutputTokens)
		stats.RequestCount++
		stats.CostByModel[entry.Model] += entry.CostUSD
		if entry.TaskType != "" {
			stats.CostByTaskType[entry.TaskType] += entry.CostUSD
		}
	}

	if stats.RequestCount > 0 {
		stats.AvgCostPerRequest = stats.TotalCostUSD / float64(stats.RequestCount)
	}

	return stats, nil
}

// GetDailyStats returns today's statistics.
func (v *VFM) GetDailyStats(agentID string) (*VFMStats, error) {
	v.mu.RLock()
	defer v.mu.RUnlock()

	dateKey := fmt.Sprintf("%s:%s", agentID, time.Now().Format("2006-01-02"))
	if stats, ok := v.dailyStats[dateKey]; ok {
		// Calculate average
		statsCopy := *stats
		if statsCopy.RequestCount > 0 {
			statsCopy.AvgCostPerRequest = statsCopy.TotalCostUSD / float64(statsCopy.RequestCount)
		}
		return &statsCopy, nil
	}

	return &VFMStats{
		CostByModel:    make(map[string]float64),
		CostByTaskType: make(map[string]float64),
		Period:         time.Now().Format("2006-01-02"),
	}, nil
}

// CheckBudget checks if spending is within budget limits.
func (v *VFM) CheckBudget(agentID string) (bool, float64, error) {
	budget, ok := v.budgets[agentID]
	if !ok {
		// Load from disk
		data, err := os.ReadFile(v.budgetPath(agentID))
		if err != nil {
			if os.IsNotExist(err) {
				return true, 0, nil // No budget set = unlimited
			}
			return false, 0, err
		}
		budget = &VFMBudget{}
		if err := json.Unmarshal(data, budget); err != nil {
			return false, 0, err
		}
		v.mu.Lock()
		v.budgets[agentID] = budget
		v.mu.Unlock()
	}

	daily, err := v.GetDailyStats(agentID)
	if err != nil {
		return false, 0, err
	}

	withinBudget := daily.TotalCostUSD < budget.DailyLimitUSD
	remaining := budget.DailyLimitUSD - daily.TotalCostUSD

	return withinBudget, remaining, nil
}
