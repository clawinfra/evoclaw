package rsi

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
)

// Fixer proposes and applies fixes for detected patterns.
type Fixer struct {
	cfg    Config
	logger *slog.Logger
}

// NewFixer creates a new Fixer.
func NewFixer(cfg Config, logger *slog.Logger) *Fixer {
	return &Fixer{
		cfg:    cfg,
		logger: logger,
	}
}

// ProposeFix generates a fix proposal for a detected pattern.
func (f *Fixer) ProposeFix(pattern Pattern) (*Fix, error) {
	isSafe := f.isSafeCategory(pattern.Category)

	fix := &Fix{
		ID:           uuid.New().String(),
		PatternID:    pattern.ID,
		Type:         FixTypeManual,
		Status:       FixStatusPending,
		SafeCategory: isSafe,
		CreatedAt:    time.Now(),
	}

	if isSafe {
		fix.Type = FixTypeAuto
	}

	// Generate fix details based on pattern
	switch pattern.Category {
	case "routing_config":
		fix.TargetFile = "config.toml"
		fix.Changes = fmt.Sprintf("Adjust routing for %s: %s", pattern.TaskType, pattern.SuggestedAction)

	case "threshold_tuning":
		fix.TargetFile = "config.toml"
		fix.Changes = fmt.Sprintf("Tune thresholds for %s pattern (frequency=%d, impact=%.2f)",
			pattern.Issue, pattern.Frequency, pattern.ImpactScore)

	case "retry_logic":
		fix.TargetFile = "config.toml"
		fix.Changes = fmt.Sprintf("Adjust retry logic: %s (failure_rate=%.2f)",
			pattern.SuggestedAction, pattern.FailureRate)

	case "model_selection":
		fix.TargetFile = "config.toml"
		fix.Changes = fmt.Sprintf("Update model selection: %s", pattern.SuggestedAction)

	default:
		fix.TargetFile = ""
		fix.Changes = fmt.Sprintf("Manual fix needed: %s\nDescription: %s\nSuggested: %s",
			pattern.Issue, pattern.Description, pattern.SuggestedAction)
	}

	return fix, nil
}

// ApplyIfSafe applies a fix if it's in a safe category. Returns (applied, error).
func (f *Fixer) ApplyIfSafe(fix *Fix) (bool, error) {
	if !fix.SafeCategory {
		// Write proposal to file for human review
		if err := f.writeProposal(fix); err != nil {
			return false, fmt.Errorf("write proposal: %w", err)
		}
		f.logger.Info("fix proposal written for review",
			"fix_id", fix.ID,
			"pattern_id", fix.PatternID,
			"category", fix.Changes,
		)
		return false, nil
	}

	if !f.cfg.AutoFixEnabled {
		if err := f.writeProposal(fix); err != nil {
			return false, fmt.Errorf("write proposal: %w", err)
		}
		f.logger.Info("auto-fix disabled, proposal written",
			"fix_id", fix.ID,
		)
		return false, nil
	}

	// For now, safe fixes are logged and marked as applied.
	// Actual config modification would go here in a production system.
	fix.Status = FixStatusApplied
	f.logger.Info("safe fix applied",
		"fix_id", fix.ID,
		"pattern_id", fix.PatternID,
		"target", fix.TargetFile,
		"changes", fix.Changes,
	)

	// Write applied fix record
	if err := f.writeAppliedFix(fix); err != nil {
		f.logger.Warn("failed to record applied fix", "error", err)
	}

	return true, nil
}

// isSafeCategory checks if a category is in the safe list.
func (f *Fixer) isSafeCategory(category string) bool {
	for _, safe := range f.cfg.SafeCategories {
		if safe == category {
			return true
		}
	}
	return false
}

// writeProposal writes a fix proposal to a file for human/agent review.
func (f *Fixer) writeProposal(fix *Fix) error {
	dir := filepath.Join(f.cfg.DataDir, "proposals")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create proposals dir: %w", err)
	}

	data, err := json.MarshalIndent(fix, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal fix: %w", err)
	}

	path := filepath.Join(dir, fix.ID+".json")
	return os.WriteFile(path, data, 0o644)
}

// writeAppliedFix records an applied fix.
func (f *Fixer) writeAppliedFix(fix *Fix) error {
	dir := filepath.Join(f.cfg.DataDir, "applied")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create applied dir: %w", err)
	}

	data, err := json.MarshalIndent(fix, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal fix: %w", err)
	}

	path := filepath.Join(dir, fix.ID+".json")
	return os.WriteFile(path, data, 0o644)
}
