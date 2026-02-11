package memory

import (
	"log/slog"
	"testing"
)

func TestNewColdMemory(t *testing.T) {
	cm := NewColdMemory(nil, "agent1", slog.Default())
	if cm == nil {
		t.Fatal("expected non-nil")
	}
	if cm.agentID != "agent1" {
		t.Errorf("agentID = %q, want agent1", cm.agentID)
	}
}

func TestNewColdMemoryNilLogger(t *testing.T) {
	cm := NewColdMemory(nil, "agent1", nil)
	if cm == nil {
		t.Fatal("expected non-nil")
	}
}

func TestColdEntryFields(t *testing.T) {
	entry := ColdEntry{
		ID:               "cold-1",
		AgentID:          "agent1",
		Timestamp:        1000,
		EventType:        "conversation",
		Category:         "general",
		Content:          `{"msg":"hello"}`,
		DistilledSummary: "greeting",
		Importance:       0.8,
		AccessCount:      5,
		CreatedAt:        1000,
	}
	if entry.ID != "cold-1" {
		t.Error("wrong ID")
	}
	if entry.Importance != 0.8 {
		t.Errorf("Importance = %f, want 0.8", entry.Importance)
	}
}
