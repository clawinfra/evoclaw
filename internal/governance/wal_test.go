package governance

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestNewWAL(t *testing.T) {
	tmpDir := t.TempDir()
	var logger *slog.Logger

	wal, err := NewWAL(tmpDir, logger)
	if err != nil {
		t.Fatalf("NewWAL failed: %v", err)
	}

	if wal == nil {
		t.Fatal("expected non-nil WAL")
	}

	// Check directory was created
	walDir := filepath.Join(tmpDir, "wal")
	if _, err := os.Stat(walDir); os.IsNotExist(err) {
		t.Error("wal directory not created")
	}
}

func TestWALAppend(t *testing.T) {
	tmpDir := t.TempDir()
	wal, _ := NewWAL(tmpDir, nil)
	agentID := "test-agent"

	tests := []struct {
		name       string
		actionType string
		content    string
		wantErr    bool
	}{
		{
			name:       "correction",
			actionType: "correction",
			content:    "Use Podman not Docker",
			wantErr:    false,
		},
		{
			name:       "decision",
			actionType: "decision",
			content:    "Using CogVideoX-2B for video",
			wantErr:    false,
		},
		{
			name:       "state_change",
			actionType: "state_change",
			content:    "GPU configured",
			wantErr:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := wal.Append(agentID, tt.actionType, tt.content)
			if (err != nil) != tt.wantErr {
				t.Errorf("Append() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestWALReplay(t *testing.T) {
	tmpDir := t.TempDir()
	wal, _ := NewWAL(tmpDir, nil)
	agentID := "test-agent-replay"

	// Append some entries
	_ = wal.Append(agentID, "correction", "Test 1")
	_ = wal.Append(agentID, "decision", "Test 2")
	_ = wal.Append(agentID, "analysis", "Test 3")

	// Replay should return all entries
	entries, err := wal.Replay(agentID)
	if err != nil {
		t.Fatalf("Replay failed: %v", err)
	}

	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}

	// Check all are unapplied
	for _, e := range entries {
		if e.Applied {
			t.Errorf("expected unapplied, got applied for entry %s", e.ID)
		}
	}
}

func TestWALMarkApplied(t *testing.T) {
	tmpDir := t.TempDir()
	wal, _ := NewWAL(tmpDir, nil)
	agentID := "test-agent-mark"

	// Append an entry
	_ = wal.Append(agentID, "correction", "Test")
	entries, _ := wal.Replay(agentID)

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	// Mark as applied
	err := wal.MarkApplied(agentID, entries[0].ID)
	if err != nil {
		t.Fatalf("MarkApplied failed: %v", err)
	}

	// Replay should return no unapplied entries
	unapplied, err := wal.Replay(agentID)
	if err != nil {
		t.Fatalf("Replay failed: %v", err)
	}

	if len(unapplied) != 0 {
		t.Errorf("expected 0 unapplied entries, got %d", len(unapplied))
	}
}

func TestWALStatus(t *testing.T) {
	tmpDir := t.TempDir()
	wal, _ := NewWAL(tmpDir, nil)
	agentID := "test-agent-status"

	// Append entries
	_ = wal.Append(agentID, "correction", "Test 1")
	_ = wal.Append(agentID, "decision", "Test 2")

	// Mark one as applied
	entries, _ := wal.Replay(agentID)
	_ = wal.MarkApplied(agentID, entries[0].ID)

	// Get status
	status, err := wal.Status(agentID)
	if err != nil {
		t.Fatalf("Status failed: %v", err)
	}

	if status.TotalEntries != 2 {
		t.Errorf("expected 2 total entries, got %d", status.TotalEntries)
	}

	if status.UnappliedEntries != 1 {
		t.Errorf("expected 1 unapplied entry, got %d", status.UnappliedEntries)
	}

	if status.LastEntry.IsZero() {
		t.Error("expected non-zero LastEntry time")
	}
}

func TestWALPrune(t *testing.T) {
	tmpDir := t.TempDir()
	wal, _ := NewWAL(tmpDir, nil)
	agentID := "test-agent-prune"

	// Append more entries than we want to keep
	for i := 0; i < 10; i++ {
		_ = wal.Append(agentID, "test", "Entry")
		time.Sleep(1 * time.Millisecond) // Ensure different timestamps
	}

	// Prune to 5
	err := wal.Prune(agentID, 5)
	if err != nil {
		t.Fatalf("Prune failed: %v", err)
	}

	// Check status
	status, _ := wal.Status(agentID)
	if status.TotalEntries != 5 {
		t.Errorf("expected 5 entries after prune, got %d", status.TotalEntries)
	}
}

func TestWALBufferFlush(t *testing.T) {
	tmpDir := t.TempDir()
	wal, _ := NewWAL(tmpDir, nil)
	agentID := "test-agent-buffer"

	// Add to buffer
	_ = wal.BufferAdd(agentID, "correction", "Buffered 1")
	_ = wal.BufferAdd(agentID, "decision", "Buffered 2")

	// Flush buffer
	err := wal.FlushBuffer(agentID)
	if err != nil {
		t.Fatalf("FlushBuffer failed: %v", err)
	}

	// Check entries exist
	status, _ := wal.Status(agentID)
	if status.TotalEntries != 2 {
		t.Errorf("expected 2 entries after flush, got %d", status.TotalEntries)
	}
}

func TestWALAgentIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	wal, _ := NewWAL(tmpDir, nil)

	agent1 := "agent-1"
	agent2 := "agent-2"

	_ = wal.Append(agent1, "test", "Agent 1 data")
	_ = wal.Append(agent2, "test", "Agent 2 data")

	// Check isolation
	entries1, _ := wal.Replay(agent1)
	entries2, _ := wal.Replay(agent2)

	if len(entries1) != 1 {
		t.Errorf("agent1: expected 1 entry, got %d", len(entries1))
	}

	if len(entries2) != 1 {
		t.Errorf("agent2: expected 1 entry, got %d", len(entries2))
	}

	if entries1[0].Content != "Agent 1 data" {
		t.Errorf("agent1: wrong content: %s", entries1[0].Content)
	}

	if entries2[0].Content != "Agent 2 data" {
		t.Errorf("agent2: wrong content: %s", entries2[0].Content)
	}
}
