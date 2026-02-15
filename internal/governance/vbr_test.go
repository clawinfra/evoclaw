package governance

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewVBR(t *testing.T) {
	tmpDir := t.TempDir()
	logger := nil

	vbr, err := NewVBR(tmpDir, logger)
	if err != nil {
		t.Fatalf("NewVBR failed: %v", err)
	}

	if vbr == nil {
		t.Fatal("expected non-nil VBR")
	}

	// Check directory was created
	vbrDir := filepath.Join(tmpDir, "vbr")
	if _, err := os.Stat(vbrDir); os.IsNotExist(err) {
		t.Error("vbr directory not created")
	}
}

func TestVBRCheckFileExists(t *testing.T) {
	tmpDir := t.TempDir()
	vbr, _ := NewVBR(tmpDir, nil)

	// Create a test file
	testFile := filepath.Join(tmpDir, "test.txt")
	_ = os.WriteFile(testFile, []byte("test"), 0644)

	// Check existing file
	passed, err := vbr.Check("task1", "file_exists", testFile)
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if !passed {
		t.Error("expected true for existing file")
	}

	// Check non-existing file
	passed, err = vbr.Check("task2", "file_exists", "/nonexistent/file.txt")
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if passed {
		t.Error("expected false for non-existing file")
	}
}

func TestVBRCheckCommand(t *testing.T) {
	tmpDir := t.TempDir()
	vbr, _ := NewVBR(tmpDir, nil)

	// Test command that succeeds
	passed, err := vbr.Check("task1", "command", "echo test")
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if !passed {
		t.Error("expected true for successful command")
	}

	// Test command that fails
	passed, err = vbr.Check("task2", "command", "exit 1")
	if err != nil {
		t.Fatalf("Check failed: %v", err)
	}

	if passed {
		t.Error("expected false for failed command")
	}
}

func TestVBRLog(t *testing.T) {
	tmpDir := t.TempDir()
	vbr, _ := NewVBR(tmpDir, nil)
	agentID := "test-agent"

	// Log a passed check
	err := vbr.Log(agentID, "task1", true, "Check passed")
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Log a failed check
	err = vbr.Log(agentID, "task2", false, "Check failed")
	if err != nil {
		t.Fatalf("Log failed: %v", err)
	}

	// Verify stats
	stats, err := vbr.Stats(agentID)
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if stats.TotalChecks != 2 {
		t.Errorf("expected 2 checks, got %d", stats.TotalChecks)
	}

	if stats.PassedChecks != 1 {
		t.Errorf("expected 1 passed, got %d", stats.PassedChecks)
	}

	if stats.FailedChecks != 1 {
		t.Errorf("expected 1 failed, got %d", stats.FailedChecks)
	}
}

func TestVBRStats(t *testing.T) {
	tmpDir := t.TempDir()
	vbr, _ := NewVBR(tmpDir, nil)
	agentID := "test-agent-stats"

	// Log some checks
	for i := 0; i < 10; i++ {
		passed := i%3 != 0 // Pass 2/3 of the time
		_ = vbr.Log(agentID, "task", passed, "Check")
	}

	// Get stats
	stats, err := vbr.Stats(agentID)
	if err != nil {
		t.Fatalf("Stats failed: %v", err)
	}

	if stats.TotalChecks != 10 {
		t.Errorf("expected 10 checks, got %d", stats.TotalChecks)
	}

	expectedPassRate := 7.0 / 10.0
	if stats.PassRate != expectedPassRate {
		t.Errorf("expected pass rate %.2f, got %.2f", expectedPassRate, stats.PassRate)
	}
}

func TestVBRAgentIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	vbr, _ := NewVBR(tmpDir, nil)

	agent1 := "agent-1"
	agent2 := "agent-2"

	_ = vbr.Log(agent1, "task1", true, "Agent 1 check")
	_ = vbr.Log(agent2, "task2", false, "Agent 2 check")

	// Check isolation
	stats1, _ := vbr.Stats(agent1)
	stats2, _ := vbr.Stats(agent2)

	if stats1.TotalChecks != 1 {
		t.Errorf("agent1: expected 1 check, got %d", stats1.TotalChecks)
	}

	if stats2.TotalChecks != 1 {
		t.Errorf("agent2: expected 1 check, got %d", stats2.TotalChecks)
	}

	if stats1.PassedChecks != 1 {
		t.Errorf("agent1: expected 1 passed, got %d", stats1.PassedChecks)
	}

	if stats2.FailedChecks != 1 {
		t.Errorf("agent2: expected 1 failed, got %d", stats2.FailedChecks)
	}
}
