package wal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// ActionType categorizes WAL entries
type ActionType string

const (
	ActionCorrection  ActionType = "correction"
	ActionDecision    ActionType = "decision"
	ActionStateChange ActionType = "state_change"
)

// Entry represents a single WAL entry
type Entry struct {
	Timestamp time.Time       `json:"timestamp"`
	AgentID   string          `json:"agent_id"`
	Action    ActionType      `json:"action_type"`
	Payload   json.RawMessage `json:"payload"`
	Applied   bool            `json:"applied"`
}

// WAL is an append-only write-ahead log for agent state
type WAL struct {
	dir     string
	mu      sync.Mutex
	entries []Entry
}

// New creates or opens a WAL in the given directory
func New(dir string) (*WAL, error) {
	if err := os.MkdirAll(dir, 0750); err != nil {
		return nil, fmt.Errorf("create wal dir: %w", err)
	}
	w := &WAL{dir: dir}
	if err := w.load(); err != nil {
		return nil, fmt.Errorf("load wal: %w", err)
	}
	return w, nil
}

// Append writes an entry to the WAL before the agent acts
func (w *WAL) Append(agentID string, action ActionType, payload interface{}) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	raw, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	entry := Entry{
		Timestamp: time.Now().UTC(),
		AgentID:   agentID,
		Action:    action,
		Payload:   raw,
		Applied:   false,
	}
	w.entries = append(w.entries, entry)
	return w.persist()
}

// MarkApplied marks an entry as applied by index
func (w *WAL) MarkApplied(index int) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if index < 0 || index >= len(w.entries) {
		return fmt.Errorf("index %d out of range [0, %d)", index, len(w.entries))
	}
	w.entries[index].Applied = true
	return w.persist()
}

// Unapplied returns all entries that haven't been applied yet
func (w *WAL) Unapplied() []Entry {
	w.mu.Lock()
	defer w.mu.Unlock()

	var result []Entry
	for _, e := range w.entries {
		if !e.Applied {
			result = append(result, e)
		}
	}
	return result
}

// UnappliedForAgent returns unapplied entries for a specific agent
func (w *WAL) UnappliedForAgent(agentID string) []Entry {
	w.mu.Lock()
	defer w.mu.Unlock()

	var result []Entry
	for _, e := range w.entries {
		if !e.Applied && e.AgentID == agentID {
			result = append(result, e)
		}
	}
	return result
}

// Entries returns all entries (snapshot)
func (w *WAL) Entries() []Entry {
	w.mu.Lock()
	defer w.mu.Unlock()
	out := make([]Entry, len(w.entries))
	copy(out, w.entries)
	return out
}

// Len returns the number of entries
func (w *WAL) Len() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	return len(w.entries)
}

func (w *WAL) walPath() string {
	return filepath.Join(w.dir, "wal.json")
}

func (w *WAL) persist() error {
	data, err := json.MarshalIndent(w.entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(w.walPath(), data, 0640)
}

func (w *WAL) load() error {
	data, err := os.ReadFile(w.walPath())
	if err != nil {
		if os.IsNotExist(err) {
			w.entries = nil
			return nil
		}
		return err
	}
	return json.Unmarshal(data, &w.entries)
}
