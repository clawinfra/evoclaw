// Package rsi implements the Recursive Self-Improvement loop for EvoClaw.
// It observes outcomes from all orchestrator actions, detects patterns,
// and auto-fixes safe categories while proposing fixes for unsafe ones.
package rsi

import (
	"time"
)

// Source identifies where an outcome originated.
type Source string

const (
	SourceOpenClaw       Source = "openclaw"
	SourceEvoClaw        Source = "evoclaw"
	SourceCron           Source = "cron"
	SourceSubAgent       Source = "subagent"
	SourceSelfGovernance Source = "self_governance"
	SourceOperational    Source = "operational"
)

// Issue categories detected from outcomes.
const (
	IssueRateLimit     = "rate_limit"
	IssueEmptyResponse = "empty_response"
	IssueTimeout       = "timeout"
	IssueModelError    = "model_error"
	IssueToolFailure   = "tool_failure"
	IssueContextLoss   = "context_loss"
	IssueSessionReset  = "session_reset"
	IssueUnknown       = "unknown"
)

// Fix type constants.
const (
	FixTypeAuto   = "auto"
	FixTypeManual = "manual"
)

// Fix status constants.
const (
	FixStatusPending  = "pending"
	FixStatusApplied  = "applied"
	FixStatusVerified = "verified"
	FixStatusFailed   = "failed"
)

// Safe fix categories that can be auto-applied.
var SafeCategories = map[string]bool{
	"routing_config":    true,
	"threshold_tuning":  true,
	"retry_logic":       true,
	"model_selection":   true,
}

// Outcome represents the result of a single orchestrator action.
type Outcome struct {
	ID           string    `json:"id"`
	Timestamp    time.Time `json:"timestamp"`
	Source       Source    `json:"source"`
	TaskType     string    `json:"task_type"`
	Success      bool      `json:"success"`
	Quality      float64   `json:"quality"`       // 0.0-1.0
	Issues       []string  `json:"issues"`         // detected issue categories
	ErrorMessage string    `json:"error_message"`
	Model        string    `json:"model"`
	DurationMs   int64     `json:"duration_ms"`
	Notes        string    `json:"notes,omitempty"`
	Tags         []string  `json:"tags,omitempty"`
}

// Pattern represents a detected recurring issue across outcomes.
type Pattern struct {
	ID              string    `json:"id"`
	Category        string    `json:"category"`        // e.g. "routing_config", "model_error"
	TaskType        string    `json:"task_type"`
	Issue           string    `json:"issue"`            // primary issue type
	Frequency       int       `json:"frequency"`        // occurrences in window
	ImpactScore     float64   `json:"impact_score"`     // 0.0-1.0
	FailureRate     float64   `json:"failure_rate"`     // 0.0-1.0
	Description     string    `json:"description"`
	SampleErrors    []string  `json:"sample_errors"`    // representative error messages
	SuggestedAction string    `json:"suggested_action"`
	Sources         []Source  `json:"sources"`          // which sources contributed
	FirstSeen       time.Time `json:"first_seen"`
	LastSeen        time.Time `json:"last_seen"`
}

// Fix represents a proposed or applied fix for a detected pattern.
type Fix struct {
	ID           string    `json:"id"`
	PatternID    string    `json:"pattern_id"`
	Type         string    `json:"type"`          // "auto" or "manual"
	Status       string    `json:"status"`        // "pending", "applied", "verified", "failed"
	TargetFile   string    `json:"target_file"`
	Changes      string    `json:"changes"`       // description of changes
	SafeCategory bool      `json:"safe_category"`
	CreatedAt    time.Time `json:"created_at"`
}

// Config controls RSI loop behavior.
type Config struct {
	// MaxOutcomes is the maximum number of outcomes to keep in the store.
	MaxOutcomes int `json:"max_outcomes"`

	// AnalysisWindow is how far back to look when analyzing patterns.
	AnalysisWindow time.Duration `json:"analysis_window"`

	// AnalysisInterval is how often to run the analysis cycle.
	AnalysisInterval time.Duration `json:"analysis_interval"`

	// RecurrenceThreshold is the minimum occurrences to flag a pattern.
	RecurrenceThreshold int `json:"recurrence_threshold"`

	// AutoFixEnabled controls whether safe fixes are auto-applied.
	AutoFixEnabled bool `json:"auto_fix_enabled"`

	// SafeCategories lists categories eligible for auto-fix.
	SafeCategories []string `json:"safe_categories"`

	// DataDir is the directory for storing outcomes and proposals.
	DataDir string `json:"data_dir"`
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() Config {
	return Config{
		MaxOutcomes:         10000,
		AnalysisWindow:      6 * time.Hour,
		AnalysisInterval:    1 * time.Hour,
		RecurrenceThreshold: 3,
		AutoFixEnabled:      true,
		SafeCategories:      []string{"routing_config", "threshold_tuning", "retry_logic", "model_selection"},
		DataDir:             "data/rsi",
	}
}

// ToolResult mirrors the orchestrator's ToolResult for observer convenience.
type ToolResult struct {
	Tool      string
	Status    string
	Result    string
	Error     string
	ExitCode  int
	ElapsedMs int64
}
