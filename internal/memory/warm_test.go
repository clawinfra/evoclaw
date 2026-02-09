package memory

import (
	"testing"
	"time"
)

func TestNewWarmMemory(t *testing.T) {
	cfg := DefaultWarmConfig()
	warm := NewWarmMemory(cfg)

	if warm == nil {
		t.Fatal("warm memory is nil")
	}

	if warm.Count() != 0 {
		t.Errorf("initial count: got %d, want 0", warm.Count())
	}
}

func TestWarmAdd(t *testing.T) {
	cfg := DefaultWarmConfig()
	warm := NewWarmMemory(cfg)

	entry := &WarmEntry{
		ID:          "test-1",
		Timestamp:   time.Now(),
		EventType:   "conversation",
		Category:    "projects/test",
		Content:     &DistilledFact{Fact: "Test fact", Date: time.Now()},
		Importance:  0.5,
		AccessCount: 0,
	}

	err := warm.Add(entry)
	if err != nil {
		t.Fatalf("add failed: %v", err)
	}

	if warm.Count() != 1 {
		t.Errorf("count: got %d, want 1", warm.Count())
	}
}

func TestWarmGet(t *testing.T) {
	cfg := DefaultWarmConfig()
	warm := NewWarmMemory(cfg)

	entry := &WarmEntry{
		ID:          "test-1",
		Timestamp:   time.Now(),
		EventType:   "conversation",
		Category:    "projects/test",
		Content:     &DistilledFact{Fact: "Test fact", Date: time.Now()},
		Importance:  0.5,
		AccessCount: 0,
	}

	warm.Add(entry)

	retrieved, err := warm.Get("test-1")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}

	if retrieved.AccessCount != 1 {
		t.Errorf("access count: got %d, want 1", retrieved.AccessCount)
	}

	// Get again
	retrieved2, _ := warm.Get("test-1")
	if retrieved2.AccessCount != 2 {
		t.Errorf("access count after second get: got %d, want 2", retrieved2.AccessCount)
	}
}

func TestWarmGetByCategory(t *testing.T) {
	cfg := DefaultWarmConfig()
	warm := NewWarmMemory(cfg)

	// Add entries in different categories
	warm.Add(&WarmEntry{
		ID:         "1",
		Category:   "projects/a",
		Content:    &DistilledFact{Fact: "A1", Date: time.Now()},
		Importance: 0.5,
		Timestamp:  time.Now(),
	})
	warm.Add(&WarmEntry{
		ID:         "2",
		Category:   "projects/a",
		Content:    &DistilledFact{Fact: "A2", Date: time.Now()},
		Importance: 0.5,
		Timestamp:  time.Now(),
	})
	warm.Add(&WarmEntry{
		ID:         "3",
		Category:   "projects/b",
		Content:    &DistilledFact{Fact: "B1", Date: time.Now()},
		Importance: 0.5,
		Timestamp:  time.Now(),
	})

	results := warm.GetByCategory("projects/a")
	if len(results) != 2 {
		t.Errorf("category results: got %d, want 2", len(results))
	}
}

func TestWarmGetRecent(t *testing.T) {
	cfg := DefaultWarmConfig()
	warm := NewWarmMemory(cfg)

	// Add entries with different timestamps
	now := time.Now()
	warm.Add(&WarmEntry{
		ID:         "1",
		Timestamp:  now.Add(-3 * time.Hour),
		Category:   "test",
		Content:    &DistilledFact{Fact: "Old", Date: now},
		Importance: 0.5,
	})
	warm.Add(&WarmEntry{
		ID:         "2",
		Timestamp:  now.Add(-1 * time.Hour),
		Category:   "test",
		Content:    &DistilledFact{Fact: "Recent", Date: now},
		Importance: 0.5,
	})
	warm.Add(&WarmEntry{
		ID:         "3",
		Timestamp:  now,
		Category:   "test",
		Content:    &DistilledFact{Fact: "Newest", Date: now},
		Importance: 0.5,
	})

	recent := warm.GetRecent(2)
	if len(recent) != 2 {
		t.Errorf("recent count: got %d, want 2", len(recent))
	}

	// Should be sorted newest first
	if recent[0].ID != "3" {
		t.Errorf("first entry: got %s, want '3'", recent[0].ID)
	}
}

func TestWarmDelete(t *testing.T) {
	cfg := DefaultWarmConfig()
	warm := NewWarmMemory(cfg)

	warm.Add(&WarmEntry{
		ID:         "test-1",
		Category:   "test",
		Content:    &DistilledFact{Fact: "Test", Date: time.Now()},
		Importance: 0.5,
		Timestamp:  time.Now(),
	})

	err := warm.Delete("test-1")
	if err != nil {
		t.Fatalf("delete failed: %v", err)
	}

	if warm.Count() != 0 {
		t.Errorf("count after delete: got %d, want 0", warm.Count())
	}

	_, err = warm.Get("test-1")
	if err == nil {
		t.Error("should fail to get deleted entry")
	}
}

func TestWarmEvictExpired(t *testing.T) {
	cfg := DefaultWarmConfig()
	cfg.RetentionDays = 7
	cfg.EvictionThreshold = 0.3
	warm := NewWarmMemory(cfg)

	// Add old entry with low score
	old := time.Now().AddDate(0, 0, -10)
	warm.Add(&WarmEntry{
		ID:          "old",
		Timestamp:   old,
		Category:    "test",
		Content:     &DistilledFact{Fact: "Old", Date: old},
		Importance:  0.2, // low importance
		AccessCount: 0,
	})

	// Add recent entry
	warm.Add(&WarmEntry{
		ID:         "recent",
		Timestamp:  time.Now(),
		Category:   "test",
		Content:    &DistilledFact{Fact: "Recent", Date: time.Now()},
		Importance: 0.5,
	})

	evicted := warm.EvictExpired()

	if len(evicted) == 0 {
		t.Error("should have evicted old entry")
	}

	if warm.Count() != 1 {
		t.Errorf("count after eviction: got %d, want 1", warm.Count())
	}
}

func TestWarmSizeLimit(t *testing.T) {
	cfg := DefaultWarmConfig()
	cfg.MaxSizeBytes = 1024 // Small limit for testing
	warm := NewWarmMemory(cfg)

	// Add entries until we hit the limit
	for i := 0; i < 100; i++ {
		entry := &WarmEntry{
			ID:         string(rune('a' + i)),
			Timestamp:  time.Now(),
			Category:   "test",
			Content:    &DistilledFact{Fact: "A reasonably long fact to take up space", Date: time.Now()},
			Importance: 0.5,
		}

		err := warm.Add(entry)
		if err != nil {
			// Hit capacity limit, which is expected
			break
		}
	}

	size := warm.GetSize()
	if size > cfg.MaxSizeBytes {
		t.Errorf("size %d exceeds limit %d", size, cfg.MaxSizeBytes)
	}
}

func TestWarmSerialize(t *testing.T) {
	cfg := DefaultWarmConfig()
	warm := NewWarmMemory(cfg)

	warm.Add(&WarmEntry{
		ID:         "test",
		Timestamp:  time.Now(),
		Category:   "test",
		Content:    &DistilledFact{Fact: "Test", Date: time.Now()},
		Importance: 0.5,
	})

	data, err := warm.Serialize()
	if err != nil {
		t.Fatalf("serialize failed: %v", err)
	}

	if len(data) == 0 {
		t.Error("serialized data is empty")
	}

	// Deserialize into new instance
	warm2 := NewWarmMemory(cfg)
	err = warm2.Deserialize(data)
	if err != nil {
		t.Fatalf("deserialize failed: %v", err)
	}

	if warm2.Count() != warm.Count() {
		t.Errorf("count after deserialize: got %d, want %d", warm2.Count(), warm.Count())
	}
}

func TestWarmStats(t *testing.T) {
	cfg := DefaultWarmConfig()
	warm := NewWarmMemory(cfg)

	now := time.Now()
	warm.Add(&WarmEntry{
		ID:         "1",
		Timestamp:  now.Add(-2 * time.Hour),
		Category:   "test",
		Content:    &DistilledFact{Fact: "First", Date: now},
		Importance: 0.5,
	})
	warm.Add(&WarmEntry{
		ID:         "2",
		Timestamp:  now,
		Category:   "test",
		Content:    &DistilledFact{Fact: "Second", Date: now},
		Importance: 0.5,
	})

	stats := warm.GetStats()

	if stats.TotalEntries != 2 {
		t.Errorf("total entries: got %d, want 2", stats.TotalEntries)
	}

	if stats.OldestEntry.IsZero() || stats.NewestEntry.IsZero() {
		t.Error("timestamps not set")
	}

	if stats.NewestEntry.Before(stats.OldestEntry) {
		t.Error("newest should be after oldest")
	}
}
