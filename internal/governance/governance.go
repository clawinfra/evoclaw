package governance

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
)

// Manager coordinates all self-governance protocols.
type Manager struct {
	WAL    *WAL
	VBR    *VBR
	ADL    *ADL
	VFM    *VFM
	logger *slog.Logger
}

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

	// Initialize WAL
	wal, err := NewWAL(filepath.Join(cfg.BaseDir, "wal"), logger)
	if err != nil {
		return nil, fmt.Errorf("init WAL: %w", err)
	}

	// Initialize VBR
	vbr, err := NewVBR(filepath.Join(cfg.BaseDir, "vbr"), logger)
	if err != nil {
		return nil, fmt.Errorf("init VBR: %w", err)
	}

	// Initialize ADL
	adl, err := NewADL(filepath.Join(cfg.BaseDir, "adl"), logger)
	if err != nil {
		return nil, fmt.Errorf("init ADL: %w", err)
	}

	// Initialize VFM
	vfm, err := NewVFM(filepath.Join(cfg.BaseDir, "vfm"), logger)
	if err != nil {
		return nil, fmt.Errorf("init VFM: %w", err)
	}

	logger.Info("governance manager initialized", "base_dir", cfg.BaseDir)

	return &Manager{
		WAL:    wal,
		VBR:    vbr,
		ADL:    adl,
		VFM:    vfm,
		logger: logger,
	}, nil
}

// SessionStart should be called at the start of each agent session.
// It replays WAL entries and checks for drift.
func (m *Manager) SessionStart(agentID, soulPath string) error {
	// Load ADL baseline from SOUL.md
	if soulPath != "" {
		if err := m.ADL.LoadBaseline(agentID, soulPath); err != nil {
			m.logger.Warn("failed to load ADL baseline", "agent", agentID, "error", err)
			// Don't fail session start for this
		}
	}

	// Replay unapplied WAL entries
	entries, err := m.WAL.Replay(agentID)
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
	if err := m.WAL.FlushBuffer(agentID); err != nil {
		return fmt.Errorf("flush WAL buffer: %w", err)
	}
	m.logger.Debug("pre-compaction flush complete", "agent", agentID)
	return nil
}

// RecordCorrection logs a user correction (high priority, immediate write).
func (m *Manager) RecordCorrection(agentID, content string) error {
	return m.WAL.Append(agentID, "correction", content)
}

// RecordDecision logs a key decision (high priority, immediate write).
func (m *Manager) RecordDecision(agentID, content string) error {
	return m.WAL.Append(agentID, "decision", content)
}

// BufferAnalysis buffers an analysis for later flush.
func (m *Manager) BufferAnalysis(agentID, content string) error {
	return m.WAL.BufferAdd(agentID, "analysis", content)
}

// VerifyAndReport verifies a task and returns whether to report completion.
func (m *Manager) VerifyAndReport(agentID, taskID string, checkType VBRCheckType, target string) (bool, error) {
	passed, err := m.VBR.Check(taskID, checkType, target)
	if err != nil {
		return false, err
	}

	notes := ""
	if !passed {
		notes = fmt.Sprintf("verification failed for %s: %s", checkType, target)
	}

	if err := m.VBR.Log(agentID, taskID, passed, notes); err != nil {
		m.logger.Warn("failed to log VBR result", "error", err)
	}

	return passed, nil
}

// CheckDrift checks for persona drift and returns the drift score.
func (m *Manager) CheckDrift(agentID, currentBehavior string) (float64, error) {
	return m.ADL.CheckDrift(agentID, currentBehavior)
}

// TrackCost records API usage cost.
func (m *Manager) TrackCost(agentID, model string, inputTokens, outputTokens int, costUSD float64) error {
	return m.VFM.TrackCost(agentID, model, inputTokens, outputTokens, costUSD)
}

// GetStatus returns a summary status of all governance protocols.
func (m *Manager) GetStatus(agentID string) (map[string]interface{}, error) {
	status := make(map[string]interface{})

	// WAL status
	walStatus, err := m.WAL.Status(agentID)
	if err != nil {
		m.logger.Warn("failed to get WAL status", "error", err)
	} else {
		status["wal"] = walStatus
	}

	// VBR stats
	vbrStats, err := m.VBR.Stats(agentID)
	if err != nil {
		m.logger.Warn("failed to get VBR stats", "error", err)
	} else {
		status["vbr"] = vbrStats
	}

	// VFM stats
	vfmStats, err := m.VFM.GetStats(agentID)
	if err != nil {
		m.logger.Warn("failed to get VFM stats", "error", err)
	} else {
		status["vfm"] = vfmStats
	}

	// Budget check
	withinBudget, remaining, err := m.VFM.CheckBudget(agentID)
	if err == nil {
		status["budget"] = map[string]interface{}{
			"within_budget": withinBudget,
			"remaining_usd": remaining,
		}
	}

	return status, nil
}
