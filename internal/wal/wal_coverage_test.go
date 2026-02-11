package wal

import (
	"testing"
)

func TestEntries(t *testing.T) {
	dir := t.TempDir()
	w, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Empty WAL
	entries := w.Entries()
	if len(entries) != 0 {
		t.Errorf("expected 0 entries, got %d", len(entries))
	}

	// Add some entries
	_ = w.Append("agent-1", ActionCorrection, map[string]string{"fix": "typo"})
	_ = w.Append("agent-2", ActionDecision, map[string]string{"choice": "A"})

	entries = w.Entries()
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}

	// Entries should be copies
	entries[0].Applied = true
	orig := w.Entries()
	if orig[0].Applied {
		t.Error("Entries() should return a copy, not a reference")
	}
}

func TestWALPersistAndReload(t *testing.T) {
	dir := t.TempDir()

	// Create and populate
	w1, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}
	_ = w1.Append("agent-1", ActionStateChange, map[string]string{"state": "running"})
	_ = w1.Append("agent-1", ActionCorrection, map[string]string{"field": "name"})
	_ = w1.MarkApplied(0)

	// Reload from same dir
	w2, err := New(dir)
	if err != nil {
		t.Fatal(err)
	}

	if w2.Len() != 2 {
		t.Errorf("expected 2 entries after reload, got %d", w2.Len())
	}

	// First entry should still be marked as applied
	entries := w2.Entries()
	if !entries[0].Applied {
		t.Error("first entry should be Applied after reload")
	}
	if entries[1].Applied {
		t.Error("second entry should not be Applied")
	}
}

func TestWorkingBufferAdd(t *testing.T) {
	dir := t.TempDir()
	w, _ := New(dir)
	b := NewWorkingBuffer("agent-1", w)

	_ = b.Add(ActionCorrection, "fix1")
	_ = b.Add(ActionDecision, "decide1")

	if b.Len() != 2 {
		t.Errorf("Buffer should have 2 items, got %d", b.Len())
	}
	if w.Len() != 0 {
		t.Errorf("WAL should be empty before flush, got %d entries", w.Len())
	}
}

func TestWorkingBufferFlushToWAL(t *testing.T) {
	dir := t.TempDir()
	w, _ := New(dir)
	b := NewWorkingBuffer("agent-1", w)

	_ = b.Add(ActionCorrection, "fix1")
	_ = b.Add(ActionDecision, "decide1")

	err := b.FlushToWAL()
	if err != nil {
		t.Fatalf("FlushToWAL() error: %v", err)
	}

	if w.Len() != 2 {
		t.Errorf("WAL should have 2 entries after flush, got %d", w.Len())
	}
	if b.Len() != 0 {
		t.Errorf("Buffer should be empty after flush, got %d", b.Len())
	}

	// Flush again should be no-op
	err = b.FlushToWAL()
	if err != nil {
		t.Fatalf("FlushToWAL() error on empty buffer: %v", err)
	}

	if w.Len() != 2 {
		t.Errorf("WAL should still have 2 entries, got %d", w.Len())
	}
}
