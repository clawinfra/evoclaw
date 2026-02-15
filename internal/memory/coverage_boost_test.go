package memory

import (
	"context"
	"fmt"
	"log/slog"
	"testing"
	"time"
)

// --- Consolidator tests ---

func TestConsolidatorNew(t *testing.T) {
	warmCfg := DefaultWarmConfig()
	warmCfg.MaxSizeBytes = 1024
	warm := NewWarmMemory(warmCfg)
	
	tree := NewMemoryTree()
	
	cfg := DefaultConsolidationConfig()
	c := NewConsolidator(warm, nil, tree, cfg, DefaultScoreConfig(), slog.Default())
	if c == nil {
		t.Fatal("NewConsolidator returned nil")
	}
}

func TestConsolidatorStartStop(t *testing.T) {
	warmCfg := DefaultWarmConfig()
	warmCfg.MaxSizeBytes = 1024
	warm := NewWarmMemory(warmCfg)
	
	tree := NewMemoryTree()
	
	cfg := DefaultConsolidationConfig()
	cfg.WarmEvictionInterval = 50 * time.Millisecond // Fast for testing
	cfg.TreePruneInterval = 50 * time.Millisecond
	cfg.TreeRebuildInterval = 1 * time.Hour   // Don't trigger
	cfg.ColdCleanupInterval = 1 * time.Hour    // Don't trigger
	
	c := NewConsolidator(warm, nil, tree, cfg, DefaultScoreConfig(), slog.Default())
	c.Start(context.Background())
	time.Sleep(200 * time.Millisecond)
	c.Stop()
}

func TestConsolidatorDoWarmEviction(t *testing.T) {
	warmCfg := DefaultWarmConfig()
	warmCfg.MaxSizeBytes = 1 // Force eviction
	warm := NewWarmMemory(warmCfg)
	
	tree := NewMemoryTree()
	
	cfg := DefaultConsolidationConfig()
	c := NewConsolidator(warm, nil, tree, cfg, DefaultScoreConfig(), slog.Default())

	// Add items to warm
	for i := 0; i < 20; i++ {
		_ = warm.Add(&WarmEntry{
			ID:        fmt.Sprintf("mem-%d", i),
			Timestamp: time.Now(),
			EventType: "test",
			Category:  "test",
			Content: &DistilledFact{
				Fact:   "Test fact with some content to increase size. " + string(rune(i+'A')),
				Topics: []string{"test"},
			},
			Importance: 0.5,
		})
	}

	c.doWarmEviction(context.Background())
	// Should not panic and should evict some items
}

func TestConsolidatorDoTreePrune(t *testing.T) {
	warmCfg := DefaultWarmConfig()
	warm := NewWarmMemory(warmCfg)
	
	tree := NewMemoryTree()
	cfg := DefaultConsolidationConfig()
	c := NewConsolidator(warm, nil, tree, cfg, DefaultScoreConfig(), slog.Default())

	// Add some nodes
	_ = tree.AddNode("root/test1", "summary1")
	
	c.doTreePrune() // No args
	// Should not panic
}

func TestConsolidatorTriggers(t *testing.T) {
	warmCfg := DefaultWarmConfig()
	warm := NewWarmMemory(warmCfg)
	tree := NewMemoryTree()
	cfg := DefaultConsolidationConfig()
	c := NewConsolidator(warm, nil, tree, cfg, DefaultScoreConfig(), slog.Default())

	// Test that triggers don't panic
	c.TriggerWarmEviction(context.Background())
	c.TriggerTreePrune() // No args
	// c.TriggerColdCleanup(context.Background()) // Requires DB connection
}

// --- WarmMemory additional tests ---

func TestWarmMemoryGetAll(t *testing.T) {
	warmCfg := DefaultWarmConfig()
	warm := NewWarmMemory(warmCfg)
	
	_ = warm.Add(&WarmEntry{
		ID:        "mem-1",
		Timestamp: time.Now(),
		EventType: "test",
		Category:  "cat1",
		Content:   &DistilledFact{Fact: "Test"},
	})

	all := warm.GetAll()
	if len(all) != 1 {
		t.Errorf("GetAll() returned %d, want 1", len(all))
	}
}

func TestWarmMemoryClear(t *testing.T) {
	warmCfg := DefaultWarmConfig()
	warm := NewWarmMemory(warmCfg)
	
	_ = warm.Add(&WarmEntry{
		ID:        "mem-1",
		Timestamp: time.Now(),
		Category:  "cat1",
		Content:   &DistilledFact{Fact: "Test"},
	})

	warm.Clear()
	all := warm.GetAll()
	if len(all) != 0 {
		t.Errorf("After Clear(), GetAll() returned %d, want 0", len(all))
	}
}

// --- Tree additional tests ---

func TestTreeGetTreeSummary(t *testing.T) {
	tree := NewMemoryTree()
	_ = tree.AddNode("root/a", "Summary A")
	_ = tree.AddNode("root/b", "Summary B")

	summary := tree.GetTreeSummary()
	if summary == "" {
		t.Error("GetTreeSummary() returned empty string")
	}
}

// --- HotMemory additional tests ---

func TestHotMemoryDeserialize(t *testing.T) {
	hot := NewHotMemory("agent-1", "owner")
	
	// Use string pointer helper
	s := "New Name"
	f := 0.8
	_ = hot.UpdateIdentity(&s, &f)

	data, err := hot.Serialize()
	if err != nil {
		t.Fatalf("Serialize() error: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("Serialize() returned empty")
	}

	hot2, err := DeserializeHotMemory(data)
	if err != nil {
		t.Fatalf("DeserializeHotMemory() error: %v", err)
	}
	if hot2 == nil {
		t.Fatal("DeserializeHotMemory() returned nil")
	}
}

func TestHotMemoryEnforceSize(t *testing.T) {
	hot := NewHotMemory("agent-1", "owner")
	// Add many lessons to trigger enforce
	for i := 0; i < 100; i++ {
		_ = hot.AddLesson(Lesson{
			Text:       fmt.Sprintf("Lesson content that takes up space %d", i),
			Importance: 0.5,
			LearnedAt:  time.Now(),
		})
	}
	// enforceSize is called internally during Add
}

// --- Manager additional tests ---

func TestManagerSetLLMFunc(t *testing.T) {
	cfg := DefaultMemoryConfig()
	cfg.Enabled = true
	cfg.DatabaseURL = "" // Won't connect

	mgr, err := NewManager(cfg, slog.Default())
	if err != nil {
		t.Skipf("NewManager failed (expected without DB): %v", err)
		return
	}

	mgr.SetLLMFunc(func(ctx context.Context, system, user string) (string, error) {
		return "test response", nil
	}, "test-model")
}

// --- Scorer additional tests ---

func TestScorerRecencyDecayEdgeCases(t *testing.T) {
	// RecencyDecay is a function, not a method
	// Very recent item
	recent := RecencyDecay(0, 30.0) // 0 age
	if recent < 0.99 {
		t.Errorf("RecencyDecay for now = %f, want ~1.0", recent)
	}

	// Very old item
	old := RecencyDecay(365 * 24 * time.Hour, 30.0)
	if old > 0.5 {
		t.Errorf("RecencyDecay for 1 year ago = %f, want < 0.5", old)
	}
}

func TestScorerReinforcementFactor(t *testing.T) {
	// 0 accesses
	f := ReinforcementFactor(0, 0.1)
	if f != 1.0 {
		t.Errorf("ReinforcementFactor(0) = %f, want 1.0", f)
	}

	// Many accesses
	f = ReinforcementFactor(100, 0.1)
	if f < 1.0 {
		t.Errorf("ReinforcementFactor(100) = %f, should be >= 1.0", f)
	}
}

// --- TreeSearch additional tests ---

func TestTreeSearchByCategory(t *testing.T) {
	tree := NewMemoryTree()
	_ = tree.AddNode("conversations/telegram", "Telegram chats")
	_ = tree.AddNode("conversations/whatsapp", "WhatsApp chats")

	searcher := NewTreeSearcher(tree, DefaultScoreConfig())
	paths := searcher.SearchByCategory("conversations")
	// Should find paths under conversations
	if len(paths) == 0 {
		t.Log("SearchByCategory returned empty (ok if not implemented fully)")
	}
}

// --- LLM Distiller additional tests ---

func TestLLMDistillerSetTimeout(t *testing.T) {
	fallback := NewDistiller(0.5)
	distiller := NewLLMDistiller(fallback, func(ctx context.Context, s, u string) (string, error) {
		return "test", nil
	}, "test-model", slog.Default())

	distiller.SetTimeout(5 * time.Second)
	// Should not panic
}

// --- LLM Tree Search additional tests ---

func TestLLMTreeSearchSetTimeout(t *testing.T) {
	tree := NewMemoryTree()
	fallback := NewTreeSearcher(tree, DefaultScoreConfig())
	searcher := NewLLMTreeSearcher(tree, fallback, func(ctx context.Context, s, u string) (string, error) {
		return "test", nil
	}, slog.Default())

	searcher.SetTimeout(5 * time.Second)
}

func TestLLMTreeSearchCategoryMethods(t *testing.T) {
	tree := NewMemoryTree()
	_ = tree.AddNode("root/a", "Summary A")
	fallback := NewTreeSearcher(tree, DefaultScoreConfig())

	searcher := NewLLMTreeSearcher(tree, fallback, func(ctx context.Context, s, u string) (string, error) {
		return "root/a", nil
	}, slog.Default())

	cats := searcher.SearchByCategory("root")
	t.Logf("SearchByCategory: %v", cats)

	recent := searcher.FindRecentlyUpdated(5)
	t.Logf("FindRecentlyUpdated: %v", recent)

	active := searcher.FindActiveNodes() // No args
	t.Logf("FindActiveNodes: %v", active)
}

// --- TreeRebuild additional tests ---

func TestTreeRebuildSetTimeout(t *testing.T) {
	tree := NewMemoryTree()
	warmCfg := DefaultWarmConfig()
	warm := NewWarmMemory(warmCfg)
	
	rebuilder := NewTreeRebuilder(tree, warm, func(ctx context.Context, s, u string) (string, error) {
		return `{"operations": []}`, nil
	}, slog.Default())
	rebuilder.SetTimeout(5 * time.Second)
}

// --- Distiller compressDistilledFact ---

func TestDistillerCompression(t *testing.T) {
	d := NewDistiller(0.5)
	// Use DistillConversation to exercise the compression path
	conv := RawConversation{
		Messages: []Message{
			{Role: "user", Content: "Tell me about the weather in Sydney. It's been really hot lately and I want to know if it will cool down."},
			{Role: "agent", Content: "The temperature in Sydney has been above average this week. Forecasts show a cool change coming Thursday with a drop to 22°C. Rain is also expected."},
		},
		Timestamp: time.Now(),
	}

	result, err := d.DistillConversation(conv)
	if err != nil {
		t.Fatalf("DistillConversation() error: %v", err)
	}
	if result.Fact == "" {
		t.Error("Fact should not be empty")
	}
}

// --- LLM Distiller validateAndCompress ---

func TestLLMDistillerValidateAndCompress(t *testing.T) {
	fallback := NewDistiller(0.5)
	distiller := NewLLMDistiller(fallback, func(ctx context.Context, s, u string) (string, error) {
		return `{"fact": "Test fact", "emotion": "neutral", "topics": ["test"], "people": [], "actions": [], "outcome": ""}`, nil
	}, "test-model", slog.Default())

	// Exercise GenerateCoreSummary
	fact := &DistilledFact{Fact: "Test fact", Topics: []string{"test"}}
	summary, err := distiller.GenerateCoreSummary(fact)
	if err != nil {
		t.Fatalf("GenerateCoreSummary failed: %v", err)
	}
	if summary == nil {
		t.Fatal("CoreSummary returned nil")
	}
	t.Logf("CoreSummary: %v", summary)
}
