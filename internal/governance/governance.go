package governance

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// Manager coordinates all self-governance protocols.
type Manager struct {
	wal    *WAL
	vbr    *VBR
	adl    *ADL
	vfm    *VFM
	logger *slog.Logger
}

func (m *Manager) WAL() *WAL { return m.wal }
func (m *Manager) VBR() *VBR { return m.vbr }
func (m *Manager) ADL() *ADL { return m.adl }
func (m *Manager) VFM() *VFM { return m.vfm }

// Config holds configuration for the governance manager.
type Config struct {
	BaseDir string `json:"base_dir"` // Base directory for all governance data
	Logger  *slog.Logger
}

// DefaultConfig returns default configuration.
func DefaultConfig() Config {
	homeDir, _ := os.UserHomeDir()
	return Config{
		BaseDir: filepath.Join(homeDir, ".evoclaw", "governance"),
		Logger:  slog.Default(),
	}
}

// NewManager creates a new governance manager with all protocols initialized.
func NewManager(cfg Config) (*Manager, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}

	logger := cfg.Logger.With("component", "governance")

	// Create base directory
	if err := os.MkdirAll(cfg.BaseDir, 0755); err != nil {
		return nil, fmt.Errorf("create governance directory: %w", err)
	}

	// Initialize WAL (creates its own "wal" subdirectory)
	wal, err := NewWAL(cfg.BaseDir, logger)
	if err != nil {
		return nil, fmt.Errorf("init WAL: %w", err)
	}

	// Initialize VBR (creates its own "vbr" subdirectory)
	vbr, err := NewVBR(cfg.BaseDir, logger)
	if err != nil {
		return nil, fmt.Errorf("init VBR: %w", err)
	}

	// Initialize ADL (creates its own "adl" subdirectory)
	adl, err := NewADL(cfg.BaseDir, logger)
	if err != nil {
		return nil, fmt.Errorf("init ADL: %w", err)
	}

	// Initialize VFM (creates its own "vfm" subdirectory)
	vfm, err := NewVFM(cfg.BaseDir, logger)
	if err != nil {
		return nil, fmt.Errorf("init VFM: %w", err)
	}

	logger.Info("governance manager initialized", "base_dir", cfg.BaseDir)

	return &Manager{
		wal:    wal,
		vbr:    vbr,
		adl:    adl,
		vfm:    vfm,
		logger: logger,
	}, nil
}

// SessionStart should be called at the start of each agent session.
// It replays WAL entries and checks for drift.
func (m *Manager) SessionStart(agentID, soulPath string) error {
	// Load ADL baseline from SOUL.md
	if soulPath != "" {
		if err := m.adl.LoadBaseline(agentID, soulPath); err != nil {
			m.logger.Warn("failed to load ADL baseline", "agent", agentID, "error", err)
			// Don't fail session start for this
		}
	}

	// Replay unapplied WAL entries
	entries, err := m.wal.Replay(agentID)
	if err != nil {
		return fmt.Errorf("WAL replay: %w", err)
	}

	if len(entries) > 0 {
		m.logger.Info("WAL entries to replay", "agent", agentID, "count", len(entries))
	}

	return nil
}

// PreCompaction should be called before conversation compaction.
// It flushes the WAL buffer to preserve important context.
func (m *Manager) PreCompaction(agentID string) error {
	if err := m.wal.FlushBuffer(agentID); err != nil {
		return fmt.Errorf("flush WAL buffer: %w", err)
	}
	m.logger.Debug("pre-compaction flush complete", "agent", agentID)
	return nil
}

// RecordCorrection logs a user correction (high priority, immediate write).
func (m *Manager) RecordCorrection(agentID, content string) error {
	return m.wal.Append(agentID, "correction", content)
}

// RecordDecision logs a key decision (high priority, immediate write).
func (m *Manager) RecordDecision(agentID, content string) error {
	return m.wal.Append(agentID, "decision", content)
}

// BufferAnalysis buffers an analysis for later flush.
func (m *Manager) BufferAnalysis(agentID, content string) error {
	return m.wal.BufferAdd(agentID, "analysis", content)
}

// VerifyAndReport verifies a task and returns whether to report completion.
func (m *Manager) VerifyAndReport(agentID, taskID string, checkType VBRCheckType, target string) (bool, error) {
	passed, err := m.vbr.Check(taskID, checkType, target)
	if err != nil {
		return false, err
	}

	notes := ""
	if !passed {
		notes = fmt.Sprintf("verification failed for %s: %s", checkType, target)
	}

	if err := m.vbr.Log(agentID, taskID, passed, notes); err != nil {
		m.logger.Warn("failed to log VBR result", "error", err)
	}

	return passed, nil
}

// CheckDrift checks for persona drift and returns the drift score.
func (m *Manager) CheckDrift(agentID, currentBehavior string) (float64, error) {
	return m.adl.CheckDrift(agentID, currentBehavior)
}

// TrackCost records API usage cost with task type.
func (m *Manager) TrackCost(agentID, taskType, model string, tokens int, costUSD float64) error {
	return m.vfm.TrackCost(agentID, taskType, model, tokens, costUSD)
}

// GetStatus returns a summary status of all governance protocols.
func (m *Manager) GetStatus(agentID string) (map[string]interface{}, error) {
	status := make(map[string]interface{})

	// WAL status
	walStatus, err := m.wal.Status(agentID)
	if err != nil {
		m.logger.Warn("failed to get WAL status", "error", err)
	} else {
		status["wal"] = walStatus
	}

	// VBR stats
	vbrStats, err := m.vbr.Stats(agentID)
	if err != nil {
		m.logger.Warn("failed to get VBR stats", "error", err)
	} else {
		status["vbr"] = vbrStats
	}

	// VFM stats
	vfmStats, err := m.vfm.GetStats(agentID)
	if err != nil {
		m.logger.Warn("failed to get VFM stats", "error", err)
	} else {
		status["vfm"] = vfmStats
	}

	// Budget check
	withinBudget, remaining, err := m.vfm.CheckBudget(agentID)
	if err == nil {
		status["budget"] = map[string]interface{}{
			"within_budget": withinBudget,
			"remaining_usd": remaining,
		}
	}

	return status, nil
}

// LogUserCorrection logs a user correction with high priority.
func (m *Manager) LogUserCorrection(agentID, content string) error {
	return m.wal.Append(agentID, "correction", content)
}

// LogDecision logs a key decision with high priority.
func (m *Manager) LogDecision(agentID, content string) error {
	return m.wal.Append(agentID, "decision", content)
}

// VerifyTaskCompletion verifies a task and logs the VBR result.
func (m *Manager) VerifyTaskCompletion(agentID, taskID string, checkType VBRCheckType, target string) (bool, error) {
	passed, err := m.vbr.Check(taskID, checkType, target)
	if err != nil {
		return false, err
	}
	notes := ""
	if !passed {
		notes = fmt.Sprintf("verification failed for %s: %s", checkType, target)
	}
	if logErr := m.vbr.Log(agentID, taskID, passed, notes); logErr != nil {
		m.logger.Warn("failed to log VBR result", "error", logErr)
	}
	return passed, nil
}

// CheckPersonaDrift analyses text for ADL signals and returns (score, drifted, err).
func (m *Manager) CheckPersonaDrift(agentID, text string, threshold float64) (float64, bool, error) {
	signals, err := m.adl.Analyze(text)
	if err != nil {
		return 0, false, err
	}
	var antiPatterns, personaSignals int
	for _, s := range signals {
		if s.Positive {
			personaSignals++
		} else {
			antiPatterns++
		}
	}
	total := antiPatterns + personaSignals
	if total == 0 {
		return 0, false, nil
	}
	score := float64(antiPatterns-personaSignals) / float64(total)
	return score, score > threshold, nil
}

// TrackTaskCost records a task cost in VFM.
func (m *Manager) TrackTaskCost(agentID, taskType, model string, inputTokens int, costUSD, qualityScore float64) error {
	return m.vfm.TrackCostWithMeta(agentID, model, inputTokens, 0, costUSD, taskType, fmt.Sprintf("%.2f", qualityScore))
}

// ReplayAgentContext returns unapplied WAL entries for context restoration.
func (m *Manager) ReplayAgentContext(agentID string) ([]WALEntry, error) {
	return m.wal.Replay(agentID)
}

// PruneAgentData trims WAL to maxEntries and optionally resets ADL signals.
func (m *Manager) PruneAgentData(agentID string, maxEntries int, resetADL bool) error {
	if err := m.wal.Prune(agentID, maxEntries); err != nil {
		return fmt.Errorf("prune WAL: %w", err)
	}
	if resetADL {
		if err := m.adl.Reset(agentID); err != nil {
			return fmt.Errorf("reset ADL: %w", err)
		}
	}
	return nil
}

// GovernanceReport is a snapshot of all governance data for an agent.
type GovernanceReport struct {
	AgentID        string       `json:"agent_id"`
	WALStatus      *WALStatus   `json:"wal_status"`
	VBRStats       *VBRStats    `json:"vbr_stats"`
	ADLStats       *ADLStats    `json:"adl_stats"`
	VFMStats       *VFMStats    `json:"vfm_stats"`
	VFMSuggestions []string     `json:"vfm_suggestions"`
}

// Summary returns a human-readable one-line summary.
func (r *GovernanceReport) Summary() string {
	walEntries := 0
	if r.WALStatus != nil {
		walEntries = r.WALStatus.TotalEntries
	}
	vbrPass := 0.0
	if r.VBRStats != nil {
		vbrPass = r.VBRStats.PassRate * 100
	}
	adlSignals := 0
	if r.ADLStats != nil {
		adlSignals = r.ADLStats.TotalSignals
	}
	totalCost := 0.0
	if r.VFMStats != nil {
		totalCost = r.VFMStats.TotalCostUSD
	}
	return fmt.Sprintf("[%s] WAL:%d VBR:%.0f%% ADL:%d signals VFM:$%.4f",
		r.AgentID, walEntries, vbrPass, adlSignals, totalCost)
}

// GetGovernanceReport collects a full governance snapshot for an agent.
func (m *Manager) GetGovernanceReport(agentID string) (*GovernanceReport, error) {
	report := &GovernanceReport{AgentID: agentID}

	walStatus, err := m.wal.Status(agentID)
	if err == nil {
		report.WALStatus = walStatus
	}
	vbrStats, err := m.vbr.Stats(agentID)
	if err == nil {
		report.VBRStats = vbrStats
	}
	adlStats, err := m.adl.Stats(agentID)
	if err == nil {
		report.ADLStats = adlStats
	}
	vfmStats, err := m.vfm.GetStats(agentID)
	if err == nil {
		report.VFMStats = vfmStats
	}
	// Simple cost suggestions based on stats
	if report.VFMStats != nil && report.VFMStats.TotalCostUSD > 1.0 {
		report.VFMSuggestions = []string{"Consider routing simple tasks to cheaper models"}
	} else {
		report.VFMSuggestions = []string{}
	}

	return report, nil
}
