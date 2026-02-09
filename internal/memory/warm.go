package memory

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"time"
)

const (
	MaxWarmSizeKB      = 50
	MaxWarmSizeBytes   = MaxWarmSizeKB * 1024
	WarmRetentionDays  = 30
	WarmEvictionThreshold = 0.3
)

// WarmMemory represents the warm tier — recent facts on-device
type WarmMemory struct {
	entries map[string]*WarmEntry // keyed by ID
	mu      sync.RWMutex
	cfg     WarmConfig
}

// WarmEntry represents a single warm memory entry
type WarmEntry struct {
	ID           string         `json:"id"`
	Timestamp    time.Time      `json:"timestamp"`
	EventType    string         `json:"event_type"` // "conversation", "decision", "lesson"
	Category     string         `json:"category"`   // tree node path
	Content      *DistilledFact `json:"content"`
	Importance   float64        `json:"importance"` // 0-1
	AccessCount  int            `json:"access_count"`
	LastAccessed time.Time      `json:"last_accessed"`
	CreatedAt    time.Time      `json:"created_at"`
}

// WarmConfig holds warm tier configuration
type WarmConfig struct {
	MaxSizeBytes      int
	RetentionDays     int
	EvictionThreshold float64
	ScoreConfig       ScoreConfig
}

// DefaultWarmConfig returns default warm tier settings
func DefaultWarmConfig() WarmConfig {
	return WarmConfig{
		MaxSizeBytes:      MaxWarmSizeBytes,
		RetentionDays:     WarmRetentionDays,
		EvictionThreshold: WarmEvictionThreshold,
		ScoreConfig:       DefaultScoreConfig(),
	}
}

// NewWarmMemory creates a new warm memory store
func NewWarmMemory(cfg WarmConfig) *WarmMemory {
	return &WarmMemory{
		entries: make(map[string]*WarmEntry),
		cfg:     cfg,
	}
}

// Add adds a new warm memory entry
func (w *WarmMemory) Add(entry *WarmEntry) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Check if we're at capacity
	currentSize := w.calculateSizeUnlocked()
	entrySize := w.estimateEntrySize(entry)

	if currentSize+entrySize > w.cfg.MaxSizeBytes {
		// Try to evict to make space
		if err := w.evictLowScoreEntriesUnlocked(entrySize); err != nil {
			return fmt.Errorf("no space for new entry: %w", err)
		}
	}

	entry.CreatedAt = time.Now()
	entry.LastAccessed = time.Now()
	w.entries[entry.ID] = entry

	return nil
}

// Get retrieves a warm memory entry and increments its access count
func (w *WarmMemory) Get(id string) (*WarmEntry, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	entry, exists := w.entries[id]
	if !exists {
		return nil, fmt.Errorf("entry %s not found", id)
	}

	// Increment access count (reinforcement)
	entry.AccessCount++
	entry.LastAccessed = time.Now()

	return entry, nil
}

// GetByCategory retrieves all entries in a category
func (w *WarmMemory) GetByCategory(category string) []*WarmEntry {
	w.mu.RLock()
	defer w.mu.RUnlock()

	results := make([]*WarmEntry, 0)
	for _, entry := range w.entries {
		if entry.Category == category {
			results = append(results, entry)
		}
	}

	return results
}

// GetRecent retrieves the N most recent entries
func (w *WarmMemory) GetRecent(n int) []*WarmEntry {
	w.mu.RLock()
	defer w.mu.RUnlock()

	entries := make([]*WarmEntry, 0, len(w.entries))
	for _, entry := range w.entries {
		entries = append(entries, entry)
	}

	// Sort by timestamp descending
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Timestamp.After(entries[j].Timestamp)
	})

	if len(entries) > n {
		entries = entries[:n]
	}

	return entries
}

// Delete removes an entry
func (w *WarmMemory) Delete(id string) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if _, exists := w.entries[id]; !exists {
		return fmt.Errorf("entry %s not found", id)
	}

	delete(w.entries, id)
	return nil
}

// GetAll returns all entries
func (w *WarmMemory) GetAll() []*WarmEntry {
	w.mu.RLock()
	defer w.mu.RUnlock()

	entries := make([]*WarmEntry, 0, len(w.entries))
	for _, entry := range w.entries {
		entries = append(entries, entry)
	}

	return entries
}

// EvictExpired removes entries that should be evicted
// Returns list of evicted entries for archival
func (w *WarmMemory) EvictExpired() []*WarmEntry {
	w.mu.Lock()
	defer w.mu.Unlock()

	evicted := make([]*WarmEntry, 0)

	for id, entry := range w.entries {
		score := w.calculateScore(entry)
		age := time.Since(entry.Timestamp)

		if ShouldEvictFromWarm(score, age, w.cfg.RetentionDays, w.cfg.EvictionThreshold) {
			evicted = append(evicted, entry)
			delete(w.entries, id)
		}
	}

	return evicted
}

// evictLowScoreEntriesUnlocked evicts entries to free up space (must hold lock)
func (w *WarmMemory) evictLowScoreEntriesUnlocked(neededBytes int) error {
	// Calculate scores for all entries
	type scored struct {
		entry *WarmEntry
		score float64
	}

	entries := make([]scored, 0, len(w.entries))
	for _, entry := range w.entries {
		score := w.calculateScore(entry)
		entries = append(entries, scored{entry, score})
	}

	// Sort by score ascending (lowest first)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].score < entries[j].score
	})

	// Evict lowest-scored entries until we have space
	freedBytes := 0
	for _, se := range entries {
		if freedBytes >= neededBytes {
			break
		}

		entrySize := w.estimateEntrySize(se.entry)
		delete(w.entries, se.entry.ID)
		freedBytes += entrySize
	}

	if freedBytes < neededBytes {
		return fmt.Errorf("could not free enough space (%d bytes freed, %d needed)", freedBytes, neededBytes)
	}

	return nil
}

// calculateScore computes the relevance score for an entry
func (w *WarmMemory) calculateScore(entry *WarmEntry) float64 {
	return CalculateScore(
		entry.Importance,
		entry.Timestamp,
		entry.AccessCount,
		w.cfg.ScoreConfig,
	)
}

// calculateSizeUnlocked estimates total size of warm memory (must hold lock)
func (w *WarmMemory) calculateSizeUnlocked() int {
	total := 0
	for _, entry := range w.entries {
		total += w.estimateEntrySize(entry)
	}
	return total
}

// estimateEntrySize estimates the size of a single entry in bytes
func (w *WarmMemory) estimateEntrySize(entry *WarmEntry) int {
	// Rough estimate: serialize to JSON
	data, err := json.Marshal(entry)
	if err != nil {
		return 500 // fallback estimate
	}
	return len(data)
}

// GetSize returns the current total size in bytes
func (w *WarmMemory) GetSize() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.calculateSizeUnlocked()
}

// Count returns the number of entries
func (w *WarmMemory) Count() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.entries)
}

// GetStats returns warm memory statistics
func (w *WarmMemory) GetStats() WarmStats {
	w.mu.RLock()
	defer w.mu.RUnlock()

	stats := WarmStats{
		TotalEntries: len(w.entries),
		TotalSizeBytes: w.calculateSizeUnlocked(),
		CapacityBytes: w.cfg.MaxSizeBytes,
	}

	// Calculate oldest and newest
	if len(w.entries) > 0 {
		for _, entry := range w.entries {
			if stats.OldestEntry.IsZero() || entry.Timestamp.Before(stats.OldestEntry) {
				stats.OldestEntry = entry.Timestamp
			}
			if stats.NewestEntry.IsZero() || entry.Timestamp.After(stats.NewestEntry) {
				stats.NewestEntry = entry.Timestamp
			}
		}
	}

	return stats
}

// WarmStats holds statistics about warm memory
type WarmStats struct {
	TotalEntries    int
	TotalSizeBytes  int
	CapacityBytes   int
	OldestEntry     time.Time
	NewestEntry     time.Time
}

// Serialize exports all entries to JSON
func (w *WarmMemory) Serialize() ([]byte, error) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	return json.Marshal(w.entries)
}

// Deserialize imports entries from JSON
func (w *WarmMemory) Deserialize(data []byte) error {
	w.mu.Lock()
	defer w.mu.Unlock()

	var entries map[string]*WarmEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("unmarshal warm memory: %w", err)
	}

	w.entries = entries
	return nil
}

// Clear removes all entries
func (w *WarmMemory) Clear() {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.entries = make(map[string]*WarmEntry)
}
