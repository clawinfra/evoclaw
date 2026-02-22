package rsi

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testConfig(t *testing.T) Config {
	t.Helper()
	dir := t.TempDir()
	return Config{
		MaxOutcomes:         1000,
		AnalysisWindow:      1 * time.Hour,
		AnalysisInterval:    1 * time.Second,
		RecurrenceThreshold: 3,
		AutoFixEnabled:      true,
		SafeCategories:      []string{"routing_config", "threshold_tuning", "retry_logic", "model_selection"},
		DataDir:             dir,
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestOutcomeRecording(t *testing.T) {
	cfg := testConfig(t)
	observer := NewObserver(cfg, testLogger())

	outcome := Outcome{
		Source:     SourceEvoClaw,
		TaskType:   "agent_chat",
		Success:    true,
		Quality:    0.9,
		Model:      "test-model",
		DurationMs: 150,
	}

	if err := observer.Record(outcome); err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	all := observer.AllOutcomes()
	if len(all) != 1 {
		t.Fatalf("expected 1 outcome, got %d", len(all))
	}
	if all[0].ID == "" {
		t.Error("outcome ID should be auto-generated")
	}
	if all[0].Source != SourceEvoClaw {
		t.Errorf("expected source evoclaw, got %s", all[0].Source)
	}

	// Check JSONL file was written
	data, err := os.ReadFile(filepath.Join(cfg.DataDir, "outcomes.jsonl"))
	if err != nil {
		t.Fatalf("JSONL file not written: %v", err)
	}
	if len(data) == 0 {
		t.Error("JSONL file is empty")
	}
}

func TestOutcomeMaxTrim(t *testing.T) {
	cfg := testConfig(t)
	cfg.MaxOutcomes = 5
	observer := NewObserver(cfg, testLogger())

	for i := 0; i < 10; i++ {
		_ = observer.Record(Outcome{
			Source:   SourceEvoClaw,
			TaskType: "test",
			Success:  true,
		})
	}

	all := observer.AllOutcomes()
	if len(all) != 5 {
		t.Fatalf("expected 5 outcomes after trim, got %d", len(all))
	}
}

func TestRecordFromAgent(t *testing.T) {
	cfg := testConfig(t)
	observer := NewObserver(cfg, testLogger())

	// Success
	observer.RecordFromAgent("agent-1", "gpt-4", "hello", "world", 100*time.Millisecond, nil)
	all := observer.AllOutcomes()
	if len(all) != 1 || !all[0].Success {
		t.Error("expected 1 successful outcome")
	}

	// Empty response
	observer.RecordFromAgent("agent-1", "gpt-4", "hello", "", 100*time.Millisecond, nil)
	all = observer.AllOutcomes()
	if len(all) != 2 || all[1].Success {
		t.Error("empty response should be a failure")
	}
	if len(all[1].Issues) == 0 || all[1].Issues[0] != IssueEmptyResponse {
		t.Error("should detect empty_response issue")
	}
}

func TestRecordToolCall(t *testing.T) {
	cfg := testConfig(t)
	observer := NewObserver(cfg, testLogger())

	result := &ToolResult{
		Tool:   "exec",
		Status: "success",
		Result: "ok",
	}
	observer.RecordToolCall("exec", result, 50*time.Millisecond)

	all := observer.AllOutcomes()
	if len(all) != 1 || !all[0].Success {
		t.Error("expected successful tool call outcome")
	}

	// Failed tool call
	failResult := &ToolResult{
		Tool:   "exec",
		Status: "error",
		Error:  "timeout: command timed out",
	}
	observer.RecordToolCall("exec", failResult, 30*time.Second)

	all = observer.AllOutcomes()
	if len(all) != 2 || all[1].Success {
		t.Error("expected failed tool call outcome")
	}
}

func TestPatternDetection(t *testing.T) {
	cfg := testConfig(t)
	observer := NewObserver(cfg, testLogger())
	analyzer := NewAnalyzer(observer, cfg, testLogger())

	// Record 5 rate limit errors
	for i := 0; i < 5; i++ {
		_ = observer.Record(Outcome{
			Source:       SourceEvoClaw,
			TaskType:     "agent_chat",
			Success:      false,
			ErrorMessage: "rate limit exceeded: 429 too many requests",
			Issues:       []string{IssueRateLimit},
			Timestamp:    time.Now().Add(-time.Duration(i) * time.Minute),
		})
	}

	patterns, err := analyzer.Analyze()
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	if len(patterns) == 0 {
		t.Fatal("expected at least 1 pattern")
	}

	found := false
	for _, p := range patterns {
		if p.Issue == IssueRateLimit || p.Issue == "error_cluster:"+IssueRateLimit {
			found = true
			if p.Frequency < 3 {
				t.Errorf("expected frequency >= 3, got %d", p.Frequency)
			}
			break
		}
	}
	if !found {
		t.Error("expected rate_limit pattern")
	}
}

func TestRecurrenceDetection(t *testing.T) {
	cfg := testConfig(t)
	cfg.RecurrenceThreshold = 3
	observer := NewObserver(cfg, testLogger())

	// Record 3 timeout errors in quick succession
	for i := 0; i < 3; i++ {
		_ = observer.Record(Outcome{
			Source:       SourceEvoClaw,
			TaskType:     "tool_call",
			Success:      false,
			ErrorMessage: "timeout: deadline exceeded",
			Issues:       []string{IssueTimeout},
			Timestamp:    time.Now(),
		})
	}

	// The recurrence check happens inside Record(), logged as a warning.
	// Verify the outcomes are stored
	all := observer.AllOutcomes()
	if len(all) != 3 {
		t.Fatalf("expected 3 outcomes, got %d", len(all))
	}
}

func TestHealthScore(t *testing.T) {
	cfg := testConfig(t)
	observer := NewObserver(cfg, testLogger())
	analyzer := NewAnalyzer(observer, cfg, testLogger())

	// No data = 1.0
	if score := analyzer.HealthScore(); score != 1.0 {
		t.Errorf("empty health score should be 1.0, got %.2f", score)
	}

	// All successes = high score
	for i := 0; i < 10; i++ {
		_ = observer.Record(Outcome{
			Source:    SourceEvoClaw,
			TaskType:  "agent_chat",
			Success:   true,
			Quality:   1.0,
			Timestamp: time.Now(),
		})
	}
	score := analyzer.HealthScore()
	if score < 0.9 {
		t.Errorf("all-success health score should be > 0.9, got %.2f", score)
	}

	// Add failures
	for i := 0; i < 20; i++ {
		_ = observer.Record(Outcome{
			Source:       SourceEvoClaw,
			TaskType:     "agent_chat",
			Success:      false,
			Quality:      0.0,
			ErrorMessage: "error",
			Timestamp:    time.Now(),
		})
	}
	score = analyzer.HealthScore()
	if score > 0.5 {
		t.Errorf("mixed health score should be < 0.5, got %.2f", score)
	}
}

func TestSafeVsUnsafeFixCategorization(t *testing.T) {
	cfg := testConfig(t)
	fixer := NewFixer(cfg, testLogger())

	// Safe category pattern
	safePattern := Pattern{
		ID:              "p1",
		Category:        "routing_config",
		Issue:           IssueEmptyResponse,
		SuggestedAction: "Fix routing",
	}

	fix, err := fixer.ProposeFix(safePattern)
	if err != nil {
		t.Fatalf("ProposeFix failed: %v", err)
	}
	if !fix.SafeCategory {
		t.Error("routing_config should be safe category")
	}
	if fix.Type != FixTypeAuto {
		t.Error("safe fix should be auto type")
	}

	// Unsafe category pattern
	unsafePattern := Pattern{
		ID:              "p2",
		Category:        "operational",
		Issue:           IssueToolFailure,
		SuggestedAction: "Check tools",
	}

	fix2, err := fixer.ProposeFix(unsafePattern)
	if err != nil {
		t.Fatalf("ProposeFix failed: %v", err)
	}
	if fix2.SafeCategory {
		t.Error("operational should not be safe category")
	}
	if fix2.Type != FixTypeManual {
		t.Error("unsafe fix should be manual type")
	}
}

func TestApplyIfSafe(t *testing.T) {
	cfg := testConfig(t)
	fixer := NewFixer(cfg, testLogger())

	// Safe fix should be applied
	safeFix := &Fix{
		ID:           "f1",
		PatternID:    "p1",
		Type:         FixTypeAuto,
		Status:       FixStatusPending,
		SafeCategory: true,
		TargetFile:   "config.toml",
		Changes:      "adjust routing",
		CreatedAt:    time.Now(),
	}

	applied, err := fixer.ApplyIfSafe(safeFix)
	if err != nil {
		t.Fatalf("ApplyIfSafe failed: %v", err)
	}
	if !applied {
		t.Error("safe fix should be applied")
	}
	if safeFix.Status != FixStatusApplied {
		t.Error("status should be applied")
	}

	// Unsafe fix should not be applied
	unsafeFix := &Fix{
		ID:           "f2",
		PatternID:    "p2",
		Type:         FixTypeManual,
		Status:       FixStatusPending,
		SafeCategory: false,
		Changes:      "manual fix",
		CreatedAt:    time.Now(),
	}

	applied, err = fixer.ApplyIfSafe(unsafeFix)
	if err != nil {
		t.Fatalf("ApplyIfSafe failed: %v", err)
	}
	if applied {
		t.Error("unsafe fix should not be applied")
	}

	// Verify proposal file was written
	proposalFile := filepath.Join(cfg.DataDir, "proposals", "f2.json")
	if _, err := os.Stat(proposalFile); os.IsNotExist(err) {
		t.Error("proposal file should be written for unsafe fix")
	}
}

func TestDetectIssues(t *testing.T) {
	tests := []struct {
		errMsg   string
		expected string
	}{
		{"rate limit exceeded: 429", IssueRateLimit},
		{"empty response from model", IssueEmptyResponse},
		{"timeout: deadline exceeded", IssueTimeout},
		{"unknown model: gpt-5", IssueModelError},
		{"tool failed: execution failed", IssueToolFailure},
		{"context too long: token limit", IssueContextLoss},
		{"connection reset by peer", IssueSessionReset},
		{"something went wrong", IssueUnknown},
	}

	for _, tt := range tests {
		issues := detectIssues(tt.errMsg)
		found := false
		for _, issue := range issues {
			if issue == tt.expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("detectIssues(%q) = %v, expected to contain %s", tt.errMsg, issues, tt.expected)
		}
	}
}

func TestLoopCreation(t *testing.T) {
	cfg := testConfig(t)
	loop := NewLoop(cfg, testLogger())

	if loop.Observer() == nil {
		t.Error("observer should not be nil")
	}
	if loop.HealthScore() != 1.0 {
		t.Error("initial health score should be 1.0")
	}
	if len(loop.Patterns()) != 0 {
		t.Error("initial patterns should be empty")
	}
}

func TestCrossSourceCorrelation(t *testing.T) {
	cfg := testConfig(t)
	observer := NewObserver(cfg, testLogger())
	analyzer := NewAnalyzer(observer, cfg, testLogger())

	now := time.Now()

	// Session resets
	for i := 0; i < 3; i++ {
		_ = observer.Record(Outcome{
			Source:       SourceOpenClaw,
			TaskType:     "agent_chat",
			Success:      false,
			Issues:       []string{IssueSessionReset},
			ErrorMessage: "connection reset",
			Timestamp:    now.Add(-time.Duration(i) * time.Minute),
		})
	}

	// Context loss close in time
	_ = observer.Record(Outcome{
		Source:       SourceEvoClaw,
		TaskType:     "agent_chat",
		Success:      false,
		Issues:       []string{IssueContextLoss},
		ErrorMessage: "context too long",
		Timestamp:    now.Add(-1 * time.Minute),
	})

	patterns, err := analyzer.Analyze()
	if err != nil {
		t.Fatalf("Analyze failed: %v", err)
	}

	foundCompound := false
	for _, p := range patterns {
		if p.Category == "compound" {
			foundCompound = true
			break
		}
	}
	if !foundCompound {
		t.Error("expected compound cross-source pattern")
	}
}

func TestAutoFixDisabled(t *testing.T) {
	cfg := testConfig(t)
	cfg.AutoFixEnabled = false
	fixer := NewFixer(cfg, testLogger())

	fix := &Fix{
		ID:           "f-disabled",
		PatternID:    "p1",
		Type:         FixTypeAuto,
		Status:       FixStatusPending,
		SafeCategory: true,
		Changes:      "adjust",
		CreatedAt:    time.Now(),
	}

	applied, err := fixer.ApplyIfSafe(fix)
	if err != nil {
		t.Fatalf("ApplyIfSafe failed: %v", err)
	}
	if applied {
		t.Error("should not apply when auto-fix is disabled")
	}
}

func TestLoopRunCycle(t *testing.T) {
	cfg := testConfig(t)
	loop := NewLoop(cfg, testLogger())

	// Add some outcomes with patterns
	for i := 0; i < 5; i++ {
		_ = loop.Observer().Record(Outcome{
			Source:       SourceEvoClaw,
			TaskType:     "agent_chat",
			Success:      false,
			ErrorMessage: "rate limit exceeded",
			Issues:       []string{IssueRateLimit},
			Timestamp:    time.Now(),
		})
	}

	// Run a cycle manually
	loop.runCycle()

	if loop.HealthScore() >= 1.0 {
		t.Error("health score should drop after failures")
	}
	patterns := loop.Patterns()
	if len(patterns) == 0 {
		t.Error("should detect patterns after cycle")
	}
}

func TestFixerAllCategories(t *testing.T) {
	cfg := testConfig(t)
	fixer := NewFixer(cfg, testLogger())

	categories := []struct {
		cat  string
		safe bool
	}{
		{"routing_config", true},
		{"threshold_tuning", true},
		{"retry_logic", true},
		{"model_selection", true},
		{"operational", false},
		{"compound", false},
	}

	for _, tc := range categories {
		fix, _ := fixer.ProposeFix(Pattern{
			ID:              "p-" + tc.cat,
			Category:        tc.cat,
			Issue:           "test",
			SuggestedAction: "test action",
		})
		if fix.SafeCategory != tc.safe {
			t.Errorf("category %s: expected safe=%v, got %v", tc.cat, tc.safe, fix.SafeCategory)
		}
	}
}

func TestSuggestAction(t *testing.T) {
	issues := []string{
		IssueRateLimit, IssueEmptyResponse, IssueTimeout, IssueModelError,
		IssueToolFailure, IssueContextLoss, IssueSessionReset, IssueUnknown,
	}
	for _, issue := range issues {
		action := suggestAction(issue)
		if action == "" {
			t.Errorf("suggestAction(%s) returned empty", issue)
		}
	}
}

func TestCategorizeIssue(t *testing.T) {
	tests := map[string]string{
		IssueRateLimit:     "retry_logic",
		IssueTimeout:       "retry_logic",
		IssueModelError:    "model_selection",
		IssueEmptyResponse: "routing_config",
		IssueToolFailure:   "operational",
	}
	for issue, expected := range tests {
		got := categorizeIssue(issue)
		if got != expected {
			t.Errorf("categorizeIssue(%s) = %s, want %s", issue, got, expected)
		}
	}
}

func TestTokenOverlap(t *testing.T) {
	a := tokenize("rate limit exceeded too many requests")
	b := tokenize("rate limit exceeded please retry")

	overlap := tokenOverlap(a, b)
	if overlap < 0.4 {
		t.Errorf("expected overlap > 0.4, got %.2f", overlap)
	}

	c := tokenize("completely different error message")
	overlap2 := tokenOverlap(a, c)
	if overlap2 > 0.3 {
		t.Errorf("expected low overlap, got %.2f", overlap2)
	}
}
