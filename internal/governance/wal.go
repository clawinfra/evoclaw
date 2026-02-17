// Package governance provides self-governance protocols for autonomous agents.
package governance

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/google/uuid"
)

// WALEntry represents a single write-ahead log entry.
type WALEntry struct {
	ID         string    `json:"id"`
	Timestamp  time.Time `json:"timestamp"`
	AgentID    string    `json:"agent_id"`
	ActionType string    `json:"action_type"` // correction, decision, analysis, state_change
	Content    string    `json:"content"`
	Applied    bool      `json:"applied"`
}

// WALStatus represents the current state of an agent's WAL.
type WALStatus struct {
	TotalEntries     int               `json:"total_entries"`
	Applied          int               `json:"applied"`
	Unapplied        int               `json:"unapplied"`
	UnappliedEntries int               `json:"unapplied_entries"` // alias for Unapplied
	BufferSize       int               `json:"buffer_size"`
	ActionTypes      map[string]int    `json:"action_types"`
	LastEntry        time.Time         `json:"last_entry"`
}

// WAL implements a write-ahead log for agent state persistence.
type WAL struct {
	baseDir string
	logger  *slog.Logger
	mu      sync.RWMutex
	buffers map[string][]WALEntry // in-memory buffer per agent
}

// NewWAL creates a new WAL instance.
func NewWAL(baseDir string, logger *slog.Logger) (*WAL, error) {
	walDir := filepath.Join(baseDir, "wal")
	if err := os.MkdirAll(walDir, 0755); err != nil {
		return nil, fmt.Errorf("create WAL directory: %w", err)
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &WAL{
		baseDir: walDir,
		logger:  logger.With("component", "wal"),
		buffers: make(map[string][]WALEntry),
	}, nil
}

func (w *WAL) walPath(agentID string) string {
	return filepath.Join(w.baseDir, agentID+".jsonl")
}

func (w *WAL) generateID() string {
	now := time.Now()
	return fmt.Sprintf("wal_%s_%s", now.Format("20060102_150405"), uuid.New().String()[:8])
}

// Append immediately writes an entry to the WAL file.
func (w *WAL) Append(agentID, actionType, content string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	entry := WALEntry{
		ID:         w.generateID(),
		Timestamp:  time.Now(),
		AgentID:    agentID,
		ActionType: actionType,
		Content:    content,
		Applied:    false,
	}

	f, err := os.OpenFile(w.walPath(agentID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open WAL file: %w", err)
	}
	defer f.Close()

	data, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal entry: %w", err)
	}

	if _, err := f.Write(append(data, '\n')); err != nil {
		return fmt.Errorf("write entry: %w", err)
	}

	w.logger.Debug("WAL append", "agent", agentID, "type", actionType, "id", entry.ID)
	return nil
}

// BufferAdd adds an entry to the in-memory buffer (not yet persisted).
func (w *WAL) BufferAdd(agentID, actionType, content string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	entry := WALEntry{
		ID:         w.generateID(),
		Timestamp:  time.Now(),
		AgentID:    agentID,
		ActionType: actionType,
		Content:    content,
		Applied:    false,
	}

	w.buffers[agentID] = append(w.buffers[agentID], entry)
	w.logger.Debug("WAL buffer add", "agent", agentID, "type", actionType, "buffer_size", len(w.buffers[agentID]))
	return nil
}

// FlushBuffer writes all buffered entries to disk.
func (w *WAL) FlushBuffer(agentID string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	buffer, ok := w.buffers[agentID]
	if !ok || len(buffer) == 0 {
		return nil
	}

	f, err := os.OpenFile(w.walPath(agentID), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open WAL file: %w", err)
	}
	defer f.Close()

	for _, entry := range buffer {
		data, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("marshal entry: %w", err)
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("write entry: %w", err)
		}
	}

	w.logger.Info("WAL buffer flushed", "agent", agentID, "entries", len(buffer))
	delete(w.buffers, agentID)
	return nil
}

// Replay returns all unapplied entries for an agent.
func (w *WAL) Replay(agentID string) ([]WALEntry, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	entries, err := w.readAllEntries(agentID)
	if err != nil {
		return nil, err
	}

	var unapplied []WALEntry
	for _, e := range entries {
		if !e.Applied {
			unapplied = append(unapplied, e)
		}
	}

	w.logger.Debug("WAL replay", "agent", agentID, "unapplied", len(unapplied))
	return unapplied, nil
}

// MarkApplied marks an entry as applied.
func (w *WAL) MarkApplied(agentID, entryID string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	entries, err := w.readAllEntries(agentID)
	if err != nil {
		return err
	}

	found := false
	for i := range entries {
		if entries[i].ID == entryID {
			entries[i].Applied = true
			found = true
			break
		}
	}

	if !found {
		return fmt.Errorf("entry not found: %s", entryID)
	}

	return w.writeAllEntries(agentID, entries)
}

// Status returns the current WAL status for an agent.
func (w *WAL) Status(agentID string) (*WALStatus, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	entries, err := w.readAllEntries(agentID)
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	status := &WALStatus{
		TotalEntries: len(entries),
		ActionTypes:  make(map[string]int),
		BufferSize:   len(w.buffers[agentID]),
	}

	for _, e := range entries {
		if e.Applied {
			status.Applied++
		} else {
			status.Unapplied++
		}
		status.ActionTypes[e.ActionType]++
		if e.Timestamp.After(status.LastEntry) {
			status.LastEntry = e.Timestamp
		}
	}
	status.UnappliedEntries = status.Unapplied

	return status, nil
}

// Prune removes old entries, keeping only the most recent `keep` entries.
func (w *WAL) Prune(agentID string, keep int) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	entries, err := w.readAllEntries(agentID)
	if err != nil {
		return err
	}

	if len(entries) <= keep {
		return nil
	}

	// Keep only the most recent entries
	pruned := entries[len(entries)-keep:]
	w.logger.Info("WAL pruned", "agent", agentID, "removed", len(entries)-keep, "kept", keep)
	return w.writeAllEntries(agentID, pruned)
}

func (w *WAL) readAllEntries(agentID string) ([]WALEntry, error) {
	f, err := os.Open(w.walPath(agentID))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("open WAL file: %w", err)
	}
	defer f.Close()

	var entries []WALEntry
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var entry WALEntry
		if err := json.Unmarshal(scanner.Bytes(), &entry); err != nil {
			continue // Skip malformed entries
		}
		entries = append(entries, entry)
	}

	return entries, scanner.Err()
}

func (w *WAL) writeAllEntries(agentID string, entries []WALEntry) error {
	f, err := os.Create(w.walPath(agentID))
	if err != nil {
		return fmt.Errorf("create WAL file: %w", err)
	}
	defer f.Close()

	for _, entry := range entries {
		data, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("marshal entry: %w", err)
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			return fmt.Errorf("write entry: %w", err)
		}
	}

	return nil
}
