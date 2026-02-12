package memory

import (
	"testing"
	"time"
)

func TestNewHotMemory(t *testing.T) {
	h := NewHotMemory("agent1", "owner1")
	if h == nil {
		t.Fatal("expected non-nil")
	}
	if h.Identity.AgentName != "agent1" {
		t.Errorf("AgentName = %q, want agent1", h.Identity.AgentName)
	}
	if h.Identity.OwnerName != "owner1" {
		t.Errorf("OwnerName = %q, want owner1", h.Identity.OwnerName)
	}
}

func TestUpdateIdentity(t *testing.T) {
	h := NewHotMemory("agent1", "owner1")
	name := "Alex"
	trust := 0.9
	if err := h.UpdateIdentity(&name, &trust); err != nil {
		t.Fatalf("error: %v", err)
	}
	if h.Identity.OwnerPreferredName != "Alex" {
		t.Errorf("PreferredName = %q", h.Identity.OwnerPreferredName)
	}
	if h.Identity.TrustLevel != 0.9 {
		t.Errorf("TrustLevel = %f", h.Identity.TrustLevel)
	}
	// Partial update
	if err := h.UpdateIdentity(nil, nil); err != nil {
		t.Fatalf("error: %v", err)
	}
}

func TestUpdateProfile(t *testing.T) {
	h := NewHotMemory("agent1", "owner1")
	personality := "analytical"
	family := []string{"spouse"}
	loved := []string{"tech"}
	avoid := []string{"politics"}
	if err := h.UpdateProfile(&personality, &family, &loved, &avoid); err != nil {
		t.Fatalf("error: %v", err)
	}
	if h.OwnerProfile.Personality != "analytical" {
		t.Errorf("Personality = %q", h.OwnerProfile.Personality)
	}
}

func TestAddPreference(t *testing.T) {
	h := NewHotMemory("agent1", "owner1")
	if err := h.AddPreference("color", "blue"); err != nil {
		t.Fatalf("error: %v", err)
	}
	if h.OwnerProfile.Preferences["color"] != "blue" {
		t.Error("preference not set")
	}
}

func TestProjectOperations(t *testing.T) {
	h := NewHotMemory("agent1", "owner1")
	p := Project{Name: "evoclaw", Description: "building"}
	if err := h.AddProject(p); err != nil {
		t.Fatalf("AddProject error: %v", err)
	}
	if len(h.ActiveContext.CurrentProjects) != 1 {
		t.Error("expected 1 project")
	}
	if err := h.RemoveProject("evoclaw"); err != nil {
		t.Fatalf("RemoveProject error: %v", err)
	}
	if len(h.ActiveContext.CurrentProjects) != 0 {
		t.Error("expected 0 projects")
	}
	if err := h.RemoveProject("nonexistent"); err == nil {
		t.Error("expected error for removing nonexistent")
	}
}

func TestEventOperations(t *testing.T) {
	h := NewHotMemory("agent1", "owner1")
	e := Event{Date: time.Now(), Description: "new year"}
	if err := h.AddEvent(e); err != nil {
		t.Fatalf("error: %v", err)
	}
	for i := 0; i < 60; i++ {
		_ = h.AddEvent(Event{Date: time.Now(), Description: "event"})
	}
}

func TestTaskOperations(t *testing.T) {
	h := NewHotMemory("agent1", "owner1")
	task := Task{Description: "build tests", Priority: "high"}
	if err := h.AddTask(task); err != nil {
		t.Fatalf("error: %v", err)
	}
	if err := h.RemoveTask("build tests"); err != nil {
		t.Fatalf("error: %v", err)
	}
	if err := h.RemoveTask("nonexistent"); err == nil {
		t.Error("expected error")
	}
}

func TestLessonOperations(t *testing.T) {
	h := NewHotMemory("agent1", "owner1")
	lesson := Lesson{Category: "testing", Text: "always test edge cases", Importance: 0.8}
	if err := h.AddLesson(lesson); err != nil {
		t.Fatalf("error: %v", err)
	}
	for i := 0; i < 110; i++ {
		_ = h.AddLesson(Lesson{Category: "testing", Text: "lesson", Importance: 0.5})
	}
}

func TestSerializeHotMemory(t *testing.T) {
	h := NewHotMemory("agent1", "owner1")
	_ = h.AddPreference("color", "blue")
	data, err := h.Serialize()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty data")
	}
}

func TestGetSize(t *testing.T) {
	h := NewHotMemory("agent1", "owner1")
	size, err := h.GetSize()
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if size <= 0 {
		t.Error("expected positive size")
	}
}

func TestClearActiveContext(t *testing.T) {
	h := NewHotMemory("agent1", "owner1")
	_ = h.AddEvent(Event{Date: time.Now(), Description: "event"})
	_ = h.AddTask(Task{Description: "task"})

	h.ClearActiveContext()
	// ClearActiveContext keeps projects but clears events and tasks
	if len(h.ActiveContext.RecentEvents) != 0 {
		t.Error("expected cleared events")
	}
	if len(h.ActiveContext.PendingTasks) != 0 {
		t.Error("expected cleared tasks")
	}
}
