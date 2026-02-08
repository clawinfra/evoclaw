package cloudsync

import (
	"sync"
)

// SyncOperation represents a queued sync operation
type SyncOperation struct {
	Type      string      // "critical", "warm", "full"
	AgentID   string
	Data      interface{} // *AgentMemory or *MemorySnapshot
	Timestamp int64
}

// OfflineQueue buffers sync operations when cloud is unreachable
type OfflineQueue struct {
	mu       sync.Mutex
	queue    []*SyncOperation
	maxSize  int
}

// NewOfflineQueue creates a new offline queue
func NewOfflineQueue(maxSize int) *OfflineQueue {
	if maxSize <= 0 {
		maxSize = 1000 // default
	}
	return &OfflineQueue{
		queue:   make([]*SyncOperation, 0, maxSize),
		maxSize: maxSize,
	}
}

// Enqueue adds an operation to the queue
func (q *OfflineQueue) Enqueue(op *SyncOperation) bool {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.queue) >= q.maxSize {
		// Queue full - drop oldest non-critical operation
		q.evictOldest()
	}

	q.queue = append(q.queue, op)
	return true
}

// Dequeue removes and returns the oldest operation
func (q *OfflineQueue) Dequeue() *SyncOperation {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.queue) == 0 {
		return nil
	}

	op := q.queue[0]
	q.queue = q.queue[1:]
	return op
}

// Size returns the current queue size
func (q *OfflineQueue) Size() int {
	q.mu.Lock()
	defer q.mu.Unlock()
	return len(q.queue)
}

// Clear empties the queue
func (q *OfflineQueue) Clear() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queue = q.queue[:0]
}

// evictOldest removes the oldest non-critical operation
// Must be called with lock held
func (q *OfflineQueue) evictOldest() {
	// Try to evict oldest non-critical operation
	for i := 0; i < len(q.queue); i++ {
		if q.queue[i].Type != "critical" {
			q.queue = append(q.queue[:i], q.queue[i+1:]...)
			return
		}
	}
	
	// If all are critical, drop the oldest anyway
	if len(q.queue) > 0 {
		q.queue = q.queue[1:]
	}
}
