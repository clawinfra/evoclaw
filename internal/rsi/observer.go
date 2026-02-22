package rsi

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// Observer captures outcomes from orchestrator actions and stores them.
type Observer struct {
	cfg      Config
	logger   *slog.Logger
	mu       sync.Mutex
	outcomes []Outcome
	filePath string
}

// NewObserver creates a new Observer.
func NewObserver(cfg Config, logger *slog.Logger) *Observer {
	return &Observer{
		cfg:      cfg,
		logger:   logger,
		outcomes: make([]Outcome, 0, 1024),
		filePath: filepath.Join(cfg.DataDir, "outcomes.jsonl"),
	}
}

// Record appends an outcome to the store and persists to JSONL.
func (o *Observer) Record(outcome Outcome) error {
	if outcome.ID == "" {
		outcome.ID = uuid.New().String()
	}
	if outcome.Timestamp.IsZero() {
		outcome.Timestamp = time.Now()
	}

	o.mu.Lock()
	o.outcomes = append(o.outcomes, outcome)

	// Trim if over max
	if len(o.outcomes) > o.cfg.MaxOutcomes {
		excess := len(o.outcomes) - o.cfg.MaxOutcomes
		o.outcomes = o.outcomes[excess:]
	}
	o.mu.Unlock()

	// Persist to JSONL
	if err := o.appendToFile(outcome); err != nil {
		o.logger.Warn("failed to persist outcome", "error", err)
		return err
	}

	// Check for immediate recurrence
	o.checkRecurrence(outcome)

	return nil
}

// RecordFromAgent records an outcome from a processWithAgent call.
func (o *Observer) RecordFromAgent(agentID, model string, msg, response string, elapsed time.Duration, err error) {
	outcome := Outcome{
		Timestamp:  time.Now(),
		Source:     SourceEvoClaw,
		TaskType:   "agent_chat",
		Model:      model,
		DurationMs: elapsed.Milliseconds(),
		Success:    err == nil,
		Quality:    1.0,
		Tags:       []string{"agent:" + agentID},
	}

	if err != nil {
		outcome.ErrorMessage = err.Error()
		outcome.Issues = detectIssues(err.Error())
		outcome.Quality = 0.0
	} else if strings.TrimSpace(response) == "" {
		outcome.Issues = []string{IssueEmptyResponse}
		outcome.Quality = 0.0
		outcome.Success = false
	}

	if recordErr := o.Record(outcome); recordErr != nil {
		o.logger.Warn("failed to record agent outcome", "error", recordErr)
	}
}

// RecordToolCall records an outcome from a tool execution.
func (o *Observer) RecordToolCall(toolName string, result *ToolResult, elapsed time.Duration) {
	outcome := Outcome{
		Timestamp:  time.Now(),
		Source:     SourceEvoClaw,
		TaskType:   "tool_call",
		DurationMs: elapsed.Milliseconds(),
		Success:    result != nil && result.Status == "success",
		Quality:    1.0,
		Tags:       []string{"tool:" + toolName},
	}

	if result != nil && result.Error != "" {
		outcome.ErrorMessage = result.Error
		outcome.Issues = detectIssues(result.Error)
		outcome.Quality = 0.0
		outcome.Success = false
	}

	if recordErr := o.Record(outcome); recordErr != nil {
		o.logger.Warn("failed to record tool outcome", "error", recordErr)
	}
}

// Outcomes returns a copy of recent outcomes within the analysis window.
func (o *Observer) Outcomes(window time.Duration) []Outcome {
	cutoff := time.Now().Add(-window)
	o.mu.Lock()
	defer o.mu.Unlock()

	var result []Outcome
	for _, out := range o.outcomes {
		if out.Timestamp.After(cutoff) {
			result = append(result, out)
		}
	}
	return result
}

// AllOutcomes returns all stored outcomes.
func (o *Observer) AllOutcomes() []Outcome {
	o.mu.Lock()
	defer o.mu.Unlock()
	result := make([]Outcome, len(o.outcomes))
	copy(result, o.outcomes)
	return result
}

// appendToFile writes a single outcome to the JSONL file.
func (o *Observer) appendToFile(outcome Outcome) error {
	dir := filepath.Dir(o.filePath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create data dir: %w", err)
	}

	f, err := os.OpenFile(o.filePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open outcomes file: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(outcome)
	if err != nil {
		return fmt.Errorf("marshal outcome: %w", err)
	}

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write outcome: %w", err)
	}

	return nil
}

// checkRecurrence checks if the same issue has occurred 3+ times recently.
func (o *Observer) checkRecurrence(latest Outcome) {
	if len(latest.Issues) == 0 {
		return
	}

	window := 15 * time.Minute
	cutoff := time.Now().Add(-window)

	o.mu.Lock()
	defer o.mu.Unlock()

	for _, issue := range latest.Issues {
		count := 0
		for i := len(o.outcomes) - 1; i >= 0; i-- {
			out := o.outcomes[i]
			if out.Timestamp.Before(cutoff) {
				break
			}
			for _, oi := range out.Issues {
				if oi == issue {
					count++
					break
				}
			}
		}

		if count >= o.cfg.RecurrenceThreshold {
			o.logger.Warn("immediate recurrence detected",
				"issue", issue,
				"count", count,
				"window", window,
			)
		}
	}
}

// detectIssues analyzes an error message and returns issue categories.
func detectIssues(errMsg string) []string {
	lower := strings.ToLower(errMsg)
	var issues []string

	patterns := map[string][]string{
		IssueRateLimit:     {"rate limit", "rate_limit", "429", "too many requests", "quota"},
		IssueEmptyResponse: {"empty response", "empty_response", "no content", "null response"},
		IssueTimeout:       {"timeout", "timed out", "deadline exceeded", "context deadline"},
		IssueModelError:    {"model error", "model_error", "unknown model", "invalid model", "500"},
		IssueToolFailure:   {"tool error", "tool_failure", "tool failed", "execution failed"},
		IssueContextLoss:   {"context loss", "context_loss", "context too long", "token limit"},
		IssueSessionReset:  {"session reset", "session_reset", "connection reset", "eof"},
	}

	for issue, keywords := range patterns {
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				issues = append(issues, issue)
				break
			}
		}
	}

	if len(issues) == 0 {
		issues = append(issues, IssueUnknown)
	}

	return issues
}
