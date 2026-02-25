package orchestrator

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type RSIOutcome struct {
	ID         string   `json:"id"`
	Timestamp  string   `json:"ts"`
	AgentID    string   `json:"agent_id"`
	SessionID  string   `json:"session_id,omitempty"`
	Source     string   `json:"source"`
	TaskType   string   `json:"task_type"`
	Model      string   `json:"model"`
	Success    bool     `json:"success"`
	Quality    int      `json:"quality"`
	DurationMs int64    `json:"duration_ms"`
	Issues     []string `json:"issues"`
	Tags       []string `json:"tags"`
	Notes      string   `json:"notes"`
}

type RSILogger interface {
	LogOutcome(ctx context.Context, outcome RSIOutcome) error
}

type NoopRSILogger struct{}

func (NoopRSILogger) LogOutcome(_ context.Context, _ RSIOutcome) error { return nil }

type JSONLRSILogger struct{ Path string }

func NewJSONLRSILogger(path string) RSILogger {
	dir := filepath.Dir(path)
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return NoopRSILogger{}
	}
	return &JSONLRSILogger{Path: path}
}

func (l *JSONLRSILogger) LogOutcome(_ context.Context, outcome RSIOutcome) error {
	if outcome.ID == "" {
		b := make([]byte, 4)
		_, _ = rand.Read(b)
		outcome.ID = hex.EncodeToString(b)
	}
	if outcome.Timestamp == "" {
		outcome.Timestamp = time.Now().UTC().Format(time.RFC3339)
	}
	if outcome.Issues == nil {
		outcome.Issues = []string{}
	}
	if outcome.Tags == nil {
		outcome.Tags = []string{}
	}
	data, err := json.Marshal(outcome)
	if err != nil {
		return fmt.Errorf("marshal RSI outcome: %w", err)
	}
	f, err := os.OpenFile(l.Path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open RSI outcomes file: %w", err)
	}
	defer f.Close()
	_, err = f.Write(append(data, '\n'))
	return err
}

func DeriveQuality(errorCount, toolCallCount int) int {
	if toolCallCount == 0 || errorCount == 0 {
		return 5
	}
	rate := float64(errorCount) / float64(toolCallCount)
	switch {
	case rate < 0.20:
		return 4
	case rate < 0.50:
		return 3
	case rate < 0.80:
		return 2
	default:
		return 1
	}
}

func DeriveTaskType(toolNames []string) string {
	set := make(map[string]bool, len(toolNames))
	for _, n := range toolNames {
		set[strings.ToLower(n)] = true
	}
	for _, t := range []string{"bash", "execute", "shell", "exec"} {
		if set[t] {
			return "code_generation"
		}
	}
	for _, t := range []string{"write_file", "edit_file"} {
		if set[t] {
			return "code_generation"
		}
	}
	for _, t := range []string{"read_file", "list_files", "glob", "grep"} {
		if set[t] {
			return "file_ops"
		}
	}
	for _, t := range []string{"websearch", "webfetch"} {
		if set[t] {
			return "web_search"
		}
	}
	for _, t := range []string{"git_commit", "git_diff", "git_log"} {
		if set[t] {
			return "code_review"
		}
	}
	if set["edge_call"] {
		return "infrastructure_ops"
	}
	return "unknown"
}

func DefaultRSILoggerPath() string {
	if p := os.Getenv("RSI_OUTCOMES_FILE"); p != "" {
		if info, err := os.Stat(filepath.Dir(p)); err == nil && info.IsDir() {
			return p
		}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	p := filepath.Join(home, ".openclaw", "workspace", "skills", "rsi-loop", "data", "outcomes.jsonl")
	if info, err := os.Stat(filepath.Dir(p)); err == nil && info.IsDir() {
		return p
	}
	return ""
}

func NewDefaultRSILogger() RSILogger {
	p := DefaultRSILoggerPath()
	if p == "" {
		return NoopRSILogger{}
	}
	return &JSONLRSILogger{Path: p}
}
