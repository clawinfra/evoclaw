package wal

import (
	"encoding/json"
	"sync"
	"time"
)

// WorkingBuffer holds recent critical state that must survive memory compaction.
// Before compaction occurs, FlushToWAL persists everything to the WAL so
// corrections and decisions are never lost.
type WorkingBuffer struct {
	mu      sync.Mutex
	items   []BufferItem
	wal     *WAL
	agentID string
}

// BufferItem is a single item in the working buffer
type BufferItem struct {
	Timestamp time.Time       `json:"timestamp"`
	Action    ActionType      `json:"action_type"`
	Payload   json.RawMessage `json:"payload"`
}

// NewWorkingBuffer creates a buffer backed by the given WAL
func NewWorkingBuffer(agentID string, w *WAL) *WorkingBuffer {
	return &WorkingBuffer{
		agentID: agentID,
		wal:     w,
	}
}

// Add puts an item into the working buffer
func (b *WorkingBuffer) Add(action ActionType, payload interface{}) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	b.items = append(b.items, BufferItem{
		Timestamp: time.Now().UTC(),
		Action:    action,
		Payload:   raw,
	})
	return nil
}

// Len returns the number of buffered items
func (b *WorkingBuffer) Len() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.items)
}

// FlushToWAL persists all buffered items to the WAL and clears the buffer.
// This MUST be called before any memory compaction.
func (b *WorkingBuffer) FlushToWAL() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	for _, item := range b.items {
		if err := b.wal.Append(b.agentID, item.Action, item.Payload); err != nil {
			return err
		}
	}
	b.items = nil
	return nil
}
