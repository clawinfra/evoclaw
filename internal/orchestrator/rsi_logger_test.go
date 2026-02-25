package orchestrator

import (
	"bufio"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// mockRSILogger — captures outcomes for assertion
// ---------------------------------------------------------------------------

type mockRSILogger struct{ outcomes []RSIOutcome }

func (m *mockRSILogger) LogOutcome(_ context.Context, o RSIOutcome) error {
	m.outcomes = append(m.outcomes, o)
	return nil
}

// ---------------------------------------------------------------------------
// 1. TestJSONLRSILogger_WritesRecord
// ---------------------------------------------------------------------------

func TestJSONLRSILogger_WritesRecord(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "outcomes.jsonl")

	logger := NewJSONLRSILogger(path)

	outcome := RSIOutcome{
		AgentID:    "agent-1",
		Source:     "evoclaw",
		TaskType:   "code_generation",
		Model:      "gpt-4",
		Success:    true,
		Quality:    5,
		DurationMs: 1234,
		Issues:     []string{},
		Tags:       []string{"toolloop"},
		Notes:      "2 tool calls, 0 parallel batches",
	}

	if err := logger.LogOutcome(context.Background(), outcome); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read outcomes file: %v", err)
	}

	var got RSIOutcome
	if err := json.Unmarshal(data[:len(data)-1], &got); err != nil {
		t.Fatalf("failed to parse JSON line: %v", err)
	}

	if got.AgentID != "agent-1" {
		t.Errorf("AgentID: got %q want %q", got.AgentID, "agent-1")
	}
	if got.Source != "evoclaw" {
		t.Errorf("Source: got %q want %q", got.Source, "evoclaw")
	}
	if got.ID == "" {
		t.Error("ID should be auto-generated but is empty")
	}
	if got.Timestamp == "" {
		t.Error("Timestamp should be auto-generated but is empty")
	}
	if !got.Success {
		t.Error("Success should be true")
	}
	if got.Quality != 5 {
		t.Errorf("Quality: got %d want 5", got.Quality)
	}
}

// ---------------------------------------------------------------------------
// 2. TestJSONLRSILogger_AppendsNotOverwrites
// ---------------------------------------------------------------------------

func TestJSONLRSILogger_AppendsNotOverwrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "outcomes.jsonl")
	logger := NewJSONLRSILogger(path)

	for i := 0; i < 3; i++ {
		err := logger.LogOutcome(context.Background(), RSIOutcome{
			AgentID: "agent-append",
			Source:  "evoclaw",
			Success: true,
		})
		if err != nil {
			t.Fatalf("iteration %d: unexpected error: %v", i, err)
		}
	}

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("failed to open file: %v", err)
	}
	defer f.Close()

	lineCount := 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if len(line) == 0 {
			continue
		}
		lineCount++
		var o RSIOutcome
		if err := json.Unmarshal([]byte(line), &o); err != nil {
			t.Errorf("line %d: invalid JSON: %v", lineCount, err)
		}
	}

	if lineCount != 3 {
		t.Errorf("expected 3 lines, got %d", lineCount)
	}
}

// ---------------------------------------------------------------------------
// 3. TestJSONLRSILogger_NoopWhenPathMissing
// ---------------------------------------------------------------------------

func TestJSONLRSILogger_NoopWhenPathMissing(t *testing.T) {
	// Directory does not exist → should return NoopRSILogger
	path := "/nonexistent-dir-abc123/outcomes.jsonl"
	logger := NewJSONLRSILogger(path)

	if _, ok := logger.(NoopRSILogger); !ok {
		t.Error("expected NoopRSILogger when directory does not exist")
	}

	// Calling LogOutcome on NoopRSILogger should not error or panic
	if err := logger.LogOutcome(context.Background(), RSIOutcome{}); err != nil {
		t.Errorf("NoopRSILogger.LogOutcome returned error: %v", err)
	}
}

// ---------------------------------------------------------------------------
// 4. TestQualityDerivation — table-driven, 7+ cases
// ---------------------------------------------------------------------------

func TestQualityDerivation(t *testing.T) {
	cases := []struct {
		name          string
		errorCount    int
		toolCallCount int
		want          int
	}{
		{"no tool calls → 5", 0, 0, 5},
		{"no errors → 5", 0, 10, 5},
		{"1/10 = 10% → 4", 1, 10, 4},
		{"1/9 ~11% → 4", 1, 9, 4},
		{"2/10 = 20% → 3 (boundary, not strictly < 0.20)", 2, 10, 3},
		{"3/10 = 30% → 3", 3, 10, 3},
		{"5/10 = 50% → 2 (boundary)", 5, 10, 2},
		{"7/10 = 70% → 2", 7, 10, 2},
		{"8/10 = 80% → 1 (boundary)", 8, 10, 1},
		{"10/10 = 100% → 1", 10, 10, 1},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveQuality(tc.errorCount, tc.toolCallCount)
			if got != tc.want {
				t.Errorf("DeriveQuality(%d, %d) = %d, want %d",
					tc.errorCount, tc.toolCallCount, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 5. TestTaskTypeInference — table-driven, 9 cases
// ---------------------------------------------------------------------------

func TestTaskTypeInference(t *testing.T) {
	cases := []struct {
		name      string
		toolNames []string
		want      string
	}{
		{"empty → unknown", []string{}, "unknown"},
		{"bash → code_generation", []string{"bash"}, "code_generation"},
		{"execute → code_generation", []string{"execute"}, "code_generation"},
		{"write_file → code_generation", []string{"write_file"}, "code_generation"},
		{"read_file → file_ops", []string{"read_file"}, "file_ops"},
		{"list_files → file_ops", []string{"list_files"}, "file_ops"},
		{"websearch → web_search", []string{"websearch"}, "web_search"},
		{"git_commit → code_review", []string{"git_commit"}, "code_review"},
		{"edge_call → infrastructure_ops", []string{"edge_call"}, "infrastructure_ops"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := DeriveTaskType(tc.toolNames)
			if got != tc.want {
				t.Errorf("DeriveTaskType(%v) = %q, want %q", tc.toolNames, got, tc.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// 6. TestToolLoop_LogsOutcomeOnSuccess
// ---------------------------------------------------------------------------

func TestToolLoop_LogsOutcomeOnSuccess(t *testing.T) {
	mock := &mockRSILogger{}

	tl := &ToolLoop{
		logger:    slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		rsiLogger: mock,
	}

	metrics := &ToolLoopMetrics{
		ToolCalls:       4,
		SuccessCount:    4,
		ErrorCount:      0,
		ParallelBatches: 1,
	}

	toolNames := []string{"read_file", "write_file"}
	tl.logRSIOutcome("agent-success", "claude-3-5", metrics, toolNames, 500*time.Millisecond)

	if len(mock.outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(mock.outcomes))
	}

	o := mock.outcomes[0]
	if o.AgentID != "agent-success" {
		t.Errorf("AgentID: got %q want %q", o.AgentID, "agent-success")
	}
	if o.Source != "evoclaw" {
		t.Errorf("Source: got %q want evoclaw", o.Source)
	}
	if !o.Success {
		t.Error("expected Success=true for zero errors")
	}
	if o.Quality != 5 {
		t.Errorf("Quality: got %d want 5 (0 errors)", o.Quality)
	}
	if o.TaskType != "code_generation" {
		t.Errorf("TaskType: got %q want code_generation", o.TaskType)
	}
	if o.DurationMs < 490 {
		t.Errorf("DurationMs: got %d want ~500", o.DurationMs)
	}
	foundParallel := false
	for _, tag := range o.Tags {
		if tag == "parallel" {
			foundParallel = true
		}
	}
	if !foundParallel {
		t.Error("expected 'parallel' tag when ParallelBatches > 0")
	}
}

// ---------------------------------------------------------------------------
// 7. TestToolLoop_LogsOutcomeOnFailure
// ---------------------------------------------------------------------------

func TestToolLoop_LogsOutcomeOnFailure(t *testing.T) {
	mock := &mockRSILogger{}

	tl := &ToolLoop{
		logger:    slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		rsiLogger: mock,
	}

	metrics := &ToolLoopMetrics{
		ToolCalls:       5,
		SuccessCount:    1,
		ErrorCount:      4,
		ParallelBatches: 0,
	}

	toolNames := []string{"bash", "bash", "bash"}
	tl.logRSIOutcome("agent-fail", "llama-3", metrics, toolNames, 200*time.Millisecond)

	if len(mock.outcomes) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(mock.outcomes))
	}

	o := mock.outcomes[0]
	if o.AgentID != "agent-fail" {
		t.Errorf("AgentID: got %q want %q", o.AgentID, "agent-fail")
	}
	if o.Success {
		t.Error("expected Success=false for non-zero errors")
	}
	// 4 errors / 5 calls = 80% → quality 1
	if o.Quality != 1 {
		t.Errorf("Quality: got %d want 1 (80%% error rate)", o.Quality)
	}
	if o.TaskType != "code_generation" {
		t.Errorf("TaskType: got %q want code_generation (bash tool)", o.TaskType)
	}
	foundParallel := false
	for _, tag := range o.Tags {
		if tag == "parallel" {
			foundParallel = true
		}
	}
	if foundParallel {
		t.Error("did not expect 'parallel' tag when ParallelBatches == 0")
	}
}
