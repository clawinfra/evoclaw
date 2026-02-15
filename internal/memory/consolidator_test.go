package memory

import (
	"log/slog"
	"testing"
	"time"
)

func TestDefaultConsolidationConfig(t *testing.T) {
	cfg := DefaultConsolidationConfig()
	if !cfg.Enabled {
		t.Error("expected enabled by default")
	}
	if cfg.WarmEvictionInterval != time.Hour {
		t.Errorf("WarmEvictionInterval = %v, want 1h", cfg.WarmEvictionInterval)
	}
	if cfg.TreePruneInterval != 24*time.Hour {
		t.Errorf("TreePruneInterval = %v, want 24h", cfg.TreePruneInterval)
	}
}

func TestNewConsolidator(t *testing.T) {
	logger := slog.Default()
	warm := NewWarmMemory(WarmConfig{MaxSizeBytes: 100000, RetentionDays: 30})
	tree := NewMemoryTree()

	cfg := DefaultConsolidationConfig()
	cfg.Enabled = false

	c := NewConsolidator(warm, nil, tree, cfg, DefaultScoreConfig(), logger)
	if c == nil {
		t.Fatal("expected non-nil consolidator")
	}
}

func TestNewConsolidatorNilLogger(t *testing.T) {
	warm := NewWarmMemory(WarmConfig{MaxSizeBytes: 100000, RetentionDays: 30})
	tree := NewMemoryTree()
	cfg := DefaultConsolidationConfig()
	cfg.Enabled = false

	c := NewConsolidator(warm, nil, tree, cfg, DefaultScoreConfig(), nil)
	if c == nil {
		t.Fatal("expected non-nil consolidator")
	}
}
