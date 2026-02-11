package wal

import (
	"testing"
)

func TestWALWriteReadReplay(t *testing.T) {
	dir := t.TempDir()

	w, err := New(dir)
	if err != nil {
		t.Fatalf("new wal: %v", err)
	}

	// Append entries
	if err := w.Append("agent-1", ActionCorrection, map[string]string{"key": "val"}); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := w.Append("agent-1", ActionDecision, "decide-x"); err != nil {
		t.Fatalf("append: %v", err)
	}
	if err := w.Append("agent-2", ActionStateChange, 42); err != nil {
		t.Fatalf("append: %v", err)
	}

	if w.Len() != 3 {
		t.Fatalf("expected 3 entries, got %d", w.Len())
	}

	// All should be unapplied
	unapplied := w.Unapplied()
	if len(unapplied) != 3 {
		t.Fatalf("expected 3 unapplied, got %d", len(unapplied))
	}

	// Mark first applied
	if err := w.MarkApplied(0); err != nil {
		t.Fatalf("mark applied: %v", err)
	}

	unapplied = w.Unapplied()
	if len(unapplied) != 2 {
		t.Fatalf("expected 2 unapplied after mark, got %d", len(unapplied))
	}

	// Reload from disk (simulating restart)
	w2, err := New(dir)
	if err != nil {
		t.Fatalf("reload wal: %v", err)
	}

	if w2.Len() != 3 {
		t.Fatalf("expected 3 entries after reload, got %d", w2.Len())
	}

	// Only 2 should be unapplied after reload
	unapplied2 := w2.Unapplied()
	if len(unapplied2) != 2 {
		t.Fatalf("expected 2 unapplied after reload, got %d", len(unapplied2))
	}
}

func TestUnappliedForAgent(t *testing.T) {
	dir := t.TempDir()
	w, err := New(dir)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	_ = w.Append("a1", ActionCorrection, "x")
	_ = w.Append("a2", ActionDecision, "y")
	_ = w.Append("a1", ActionStateChange, "z")

	a1 := w.UnappliedForAgent("a1")
	if len(a1) != 2 {
		t.Fatalf("expected 2 for a1, got %d", len(a1))
	}

	a2 := w.UnappliedForAgent("a2")
	if len(a2) != 1 {
		t.Fatalf("expected 1 for a2, got %d", len(a2))
	}
}

func TestWorkingBufferFlush(t *testing.T) {
	dir := t.TempDir()
	w, err := New(dir)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	buf := NewWorkingBuffer("agent-1", w)

	// Add items to buffer
	_ = buf.Add(ActionCorrection, "fix-1")
	_ = buf.Add(ActionDecision, "decide-2")

	if buf.Len() != 2 {
		t.Fatalf("expected 2 buffered, got %d", buf.Len())
	}

	// WAL should be empty before flush
	if w.Len() != 0 {
		t.Fatalf("expected 0 WAL entries before flush, got %d", w.Len())
	}

	// Flush (simulating pre-compaction)
	if err := buf.FlushToWAL(); err != nil {
		t.Fatalf("flush: %v", err)
	}

	// Buffer should be empty
	if buf.Len() != 0 {
		t.Fatalf("expected 0 buffered after flush, got %d", buf.Len())
	}

	// WAL should have the entries
	if w.Len() != 2 {
		t.Fatalf("expected 2 WAL entries after flush, got %d", w.Len())
	}

	// Verify they're for the right agent
	entries := w.UnappliedForAgent("agent-1")
	if len(entries) != 2 {
		t.Fatalf("expected 2 unapplied for agent-1, got %d", len(entries))
	}
}

func TestMarkAppliedOutOfRange(t *testing.T) {
	dir := t.TempDir()
	w, err := New(dir)
	if err != nil {
		t.Fatalf("new: %v", err)
	}

	if err := w.MarkApplied(0); err == nil {
		t.Error("expected error for out of range index")
	}
	if err := w.MarkApplied(-1); err == nil {
		t.Error("expected error for negative index")
	}
}
