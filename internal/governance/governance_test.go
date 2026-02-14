package governance

import (
	"testing"
)

func TestNewManager(t *testing.T) {
	tmpDir := t.TempDir()

	mgr, err := NewManager(Config{
		BaseDir: tmpDir,
		Logger:  nil,
	})

	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}

	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}

	if mgr.WAL() == nil {
		t.Error("expected non-nil WAL")
	}

	if mgr.VBR() == nil {
		t.Error("expected non-nil VBR")
	}

	if mgr.ADL() == nil {
		t.Error("expected non-nil ADL")
	}

	if mgr.VFM() == nil {
		t.Error("expected non-nil VFM")
	}
}

func TestManagerLogUserCorrection(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, _ := NewManager(Config{BaseDir: tmpDir})
	agentID := "test-agent"

	err := mgr.LogUserCorrection(agentID, "Use Podman not Docker")
	if err != nil {
		t.Fatalf("LogUserCorrection failed: %v", err)
	}

	// Verify WAL entry
	status, _ := mgr.WAL().Status(agentID)
	if status.TotalEntries != 1 {
		t.Errorf("expected 1 WAL entry, got %d", status.TotalEntries)
	}
}

func TestManagerLogDecision(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, _ := NewManager(Config{BaseDir: tmpDir})
	agentID := "test-agent"

	err := mgr.LogDecision(agentID, "Using CogVideoX-2B for video")
	if err != nil {
		t.Fatalf("LogDecision failed: %v", err)
	}

	// Verify WAL entry
	status, _ := mgr.WAL().Status(agentID)
	if status.TotalEntries != 1 {
		t.Errorf("expected 1 WAL entry, got %d", status.TotalEntries)
	}
}

func TestManagerVerifyTaskCompletion(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, _ := NewManager(Config{BaseDir: tmpDir})
	agentID := "test-agent"

	// Verify with a simple command
	passed, err := mgr.VerifyTaskCompletion(agentID, "task1", "command", "echo test")
	if err != nil {
		t.Fatalf("VerifyTaskCompletion failed: %v", err)
	}

	if !passed {
		t.Error("expected verification to pass")
	}

	// Check VBR log was created
	stats, _ := mgr.VBR().Stats(agentID)
	if stats.TotalChecks != 1 {
		t.Errorf("expected 1 VBR check, got %d", stats.TotalChecks)
	}

	if !stats.PassedChecks == 1 {
		t.Error("expected VBR check to be logged as passed")
	}
}

func TestManagerCheckPersonaDrift(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, _ := NewManager(Config{BaseDir: tmpDir})
	agentID := "test-agent"

	// Check text with anti-patterns
	text := "I'd be happy to help! Would you like me to?"
	score, drifted, err := mgr.CheckPersonaDrift(agentID, text, 0.5)
	if err != nil {
		t.Fatalf("CheckPersonaDrift failed: %v", err)
	}

	// Should have detected drift
	if score <= 0 {
		t.Errorf("expected positive drift score, got %.2f", score)
	}

	// May or may not exceed threshold depending on number of signals
	// Just verify the function works
}

func TestManagerTrackTaskCost(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, _ := NewManager(Config{BaseDir: tmpDir})
	agentID := "test-agent"

	err := mgr.TrackTaskCost(agentID, "monitoring", "glm-4.7", 1000, 0.01, 0.9)
	if err != nil {
		t.Fatalf("TrackTaskCost failed: %v", err)
	}

	// Verify VFM entry
	stats, _ := mgr.VFM().GetStats(agentID)
	if stats.TotalTasks != 1 {
		t.Errorf("expected 1 VFM task, got %d", stats.TotalTasks)
	}
}

func TestManagerGetGovernanceReport(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, _ := NewManager(Config{BaseDir: tmpDir})
	agentID := "test-agent"

	// Add some data
	_ = mgr.LogUserCorrection(agentID, "Test correction")
	_ = mgr.TrackTaskCost(agentID, "monitoring", "glm-4.7", 1000, 0.01, 0.9)

	// Get report
	report, err := mgr.GetGovernanceReport(agentID)
	if err != nil {
		t.Fatalf("GetGovernanceReport failed: %v", err)
	}

	if report.AgentID != agentID {
		t.Errorf("expected agent_id %s, got %s", agentID, report.AgentID)
	}

	if report.WALStatus == nil {
		t.Error("expected WAL status in report")
	}

	if report.VBRStats == nil {
		t.Error("expected VBR stats in report")
	}

	if report.ADLStats == nil {
		t.Error("expected ADL stats in report")
	}

	if report.VFMStats == nil {
		t.Error("expected VFM stats in report")
	}

	if report.VFMSuggestions == nil {
		t.Error("expected VFM suggestions in report")
	}
}

func TestManagerReportSummary(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, _ := NewManager(Config{BaseDir: tmpDir})
	agentID := "test-agent"

	// Add some data
	_ = mgr.LogUserCorrection(agentID, "Test correction")
	_ = mgr.TrackTaskCost(agentID, "monitoring", "glm-4.7", 1000, 0.01, 0.9)

	// Get report
	report, _ := mgr.GetGovernanceReport(agentID)

	// Get summary
	summary := report.Summary()
	if summary == "" {
		t.Error("expected non-empty summary")
	}

	// Check summary contains expected parts
	if containsString(summary, agentID) {
		t.Error("expected summary to contain agent ID")
	}
}

func TestManagerReplayAgentContext(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, _ := NewManager(Config{BaseDir: tmpDir})
	agentID := "test-agent"

	// Add some WAL entries
	_ = mgr.LogUserCorrection(agentID, "Correction 1")
	_ = mgr.LogDecision(agentID, "Decision 1")

	// Replay
	entries, err := mgr.ReplayAgentContext(agentID)
	if err != nil {
		t.Fatalf("ReplayAgentContext failed: %v", err)
	}

	if len(entries) != 2 {
		t.Errorf("expected 2 replayed entries, got %d", len(entries))
	}
}

func TestManagerPruneAgentData(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, _ := NewManager(Config{BaseDir: tmpDir})
	agentID := "test-agent"

	// Add many WAL entries
	for i := 0; i < 20; i++ {
		_ = mgr.LogUserCorrection(agentID, "Correction")
	}

	// Prune to 10
	err := mgr.PruneAgentData(agentID, 10, false)
	if err != nil {
		t.Fatalf("PruneAgentData failed: %v", err)
	}

	// Verify prune
	status, _ := mgr.WAL().Status(agentID)
	if status.TotalEntries != 10 {
		t.Errorf("expected 10 entries after prune, got %d", status.TotalEntries)
	}
}

func TestManagerPruneAgentDataWithADLReset(t *testing.T) {
	tmpDir := t.TempDir()
	mgr, _ := NewManager(Config{BaseDir: tmpDir})
	agentID := "test-agent"

	// Add ADL signals
	_ = mgr.ADL().Log(agentID, SignalAntiSycophancy, "Test", false)

	// Verify signal exists
	stats1, _ := mgr.ADL().Stats(agentID)
	if stats1.TotalSignals != 1 {
		t.Errorf("expected 1 signal before reset, got %d", stats1.TotalSignals)
	}

	// Prune with ADL reset
	err := mgr.PruneAgentData(agentID, 0, true)
	if err != nil {
		t.Fatalf("PruneAgentData failed: %v", err)
	}

	// Verify ADL reset
	stats2, _ := mgr.ADL().Stats(agentID)
	if stats2.TotalSignals != 0 {
		t.Errorf("expected 0 signals after reset, got %d", stats2.TotalSignals)
	}
}

// Helper function to check if string contains substring
func containsString(s, substr string) bool {
	return len(s) >= len(substr) &&
		(s == substr ||
			(len(s) > len(substr) &&
				 indexOfString(s, substr) >= 0))
}

func indexOfString(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
