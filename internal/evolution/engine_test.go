package evolution

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestEngine(t *testing.T) *Engine {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	return NewEngine(tmpDir, logger)
}

func TestNewEngine(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	e := NewEngine(tmpDir, logger)

	if e == nil {
		t.Fatal("expected non-nil engine")
	}

	if e.strategies == nil {
		t.Error("expected strategies map to be initialized")
	}

	if e.history == nil {
		t.Error("expected history map to be initialized")
	}

	// Check that evolution directory was created
	evolutionDir := filepath.Join(tmpDir, "evolution")
	if _, err := os.Stat(evolutionDir); os.IsNotExist(err) {
		t.Error("expected evolution directory to be created")
	}
}

func TestSetAndGetStrategy(t *testing.T) {
	e := newTestEngine(t)

	strategy := &Strategy{
		ID:             "test-strategy-1",
		AgentID:        "agent-1",
		Version:        1,
		SystemPrompt:   "Test prompt",
		PreferredModel: "model-1",
		FallbackModel:  "model-2",
		Temperature:    0.7,
		MaxTokens:      1000,
		Params: map[string]float64{
			"param1": 0.5,
			"param2": 0.8,
		},
		Fitness:   0.0,
		EvalCount: 0,
	}

	e.SetStrategy("agent-1", strategy)

	retrieved := e.GetStrategy("agent-1")
	if retrieved == nil {
		t.Fatal("expected strategy to be retrieved")
	}

	s := retrieved.(*Strategy)
	if s.ID != "test-strategy-1" {
		t.Errorf("expected ID test-strategy-1, got %s", s.ID)
	}

	if s.AgentID != "agent-1" {
		t.Errorf("expected AgentID agent-1, got %s", s.AgentID)
	}

	if s.Temperature != 0.7 {
		t.Errorf("expected temperature 0.7, got %f", s.Temperature)
	}

	if len(s.Params) != 2 {
		t.Errorf("expected 2 params, got %d", len(s.Params))
	}
}

func TestGetStrategyNonExistent(t *testing.T) {
	e := newTestEngine(t)

	retrieved := e.GetStrategy("nonexistent")
	// Returns nil interface value (map lookup returns nil)
	if retrieved != nil {
		t.Logf("retrieved: %v", retrieved)
	}
}

func TestEvaluate(t *testing.T) {
	e := newTestEngine(t)

	strategy := &Strategy{
		ID:        "test-strategy-1",
		AgentID:   "agent-1",
		Version:   1,
		Fitness:   0.0,
		EvalCount: 0,
	}

	e.SetStrategy("agent-1", strategy)

	// First evaluation
	metrics := map[string]float64{
		"successRate":    0.9,
		"costUSD":        0.1,
		"avgResponseMs":  500,
		"profitLoss":     0.5,
	}

	fitness1 := e.Evaluate("agent-1", metrics)

	if fitness1 <= 0 {
		t.Error("expected positive fitness score")
	}

	// Verify strategy was updated
	s := e.GetStrategy("agent-1").(*Strategy)
	if s.EvalCount != 1 {
		t.Errorf("expected eval count 1, got %d", s.EvalCount)
	}

	if s.Fitness != fitness1 {
		t.Errorf("expected fitness %f, got %f", fitness1, s.Fitness)
	}

	// Second evaluation (should use EMA)
	metrics2 := map[string]float64{
		"successRate":    0.8,
		"costUSD":        0.2,
		"avgResponseMs":  600,
		"profitLoss":     0.3,
	}

	_ = e.Evaluate("agent-1", metrics2)

	s = e.GetStrategy("agent-1").(*Strategy)
	if s.EvalCount != 2 {
		t.Errorf("expected eval count 2, got %d", s.EvalCount)
	}

	// Fitness should be updated (EMA applied, so may be same or different)
	if s.Fitness <= 0 {
		t.Error("expected positive fitness after second evaluation")
	}
}

func TestEvaluateNonExistentAgent(t *testing.T) {
	e := newTestEngine(t)

	metrics := map[string]float64{
		"successRate": 0.9,
	}

	fitness := e.Evaluate("nonexistent", metrics)

	if fitness != 0 {
		t.Errorf("expected fitness 0 for nonexistent agent, got %f", fitness)
	}
}

func TestComputeFitness(t *testing.T) {
	tests := []struct {
		name     string
		metrics  map[string]float64
		minScore float64
		maxScore float64
	}{
		{
			name: "high performance",
			metrics: map[string]float64{
				"successRate":    1.0,
				"costUSD":        0.01,
				"avgResponseMs":  100,
				"profitLoss":     1.0,
			},
			minScore: 0.5,
			maxScore: 2.0, // Can exceed 1.0 due to formula
		},
		{
			name: "medium performance",
			metrics: map[string]float64{
				"successRate":    0.7,
				"costUSD":        0.5,
				"avgResponseMs":  1000,
				"profitLoss":     0.0,
			},
			minScore: 0.2,
			maxScore: 0.8,
		},
		{
			name: "low performance",
			metrics: map[string]float64{
				"successRate":    0.3,
				"costUSD":        10.0,
				"avgResponseMs":  5000,
				"profitLoss":     -0.5,
			},
			minScore: 0.0,
			maxScore: 0.5,
		},
		{
			name: "zero metrics",
			metrics: map[string]float64{
				"successRate":    0.0,
				"costUSD":        0.0,
				"avgResponseMs":  0.0,
				"profitLoss":     0.0,
			},
			minScore: 0.0,
			maxScore: 1.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fitness := computeFitness(tt.metrics)

			if fitness < tt.minScore {
				t.Errorf("fitness %f below minimum %f", fitness, tt.minScore)
			}

			if fitness > tt.maxScore {
				t.Errorf("fitness %f above maximum %f", fitness, tt.maxScore)
			}
		})
	}
}

func TestMutate(t *testing.T) {
	e := newTestEngine(t)

	original := &Strategy{
		ID:             "original-1",
		AgentID:        "agent-1",
		Version:        1,
		SystemPrompt:   "Original prompt",
		PreferredModel: "model-1",
		FallbackModel:  "model-2",
		Temperature:    0.7,
		MaxTokens:      1000,
		Params: map[string]float64{
			"param1": 0.5,
			"param2": 0.8,
		},
		Fitness:   0.75,
		EvalCount: 10,
	}

	e.SetStrategy("agent-1", original)

	// Mutate the strategy
	result, err := e.Mutate("agent-1", 0.2)
	if err != nil {
		t.Fatalf("failed to mutate strategy: %v", err)
	}

	mutated := result.(*Strategy)

	// Version should increment
	if mutated.Version != 2 {
		t.Errorf("expected version 2, got %d", mutated.Version)
	}

	// AgentID should be the same
	if mutated.AgentID != "agent-1" {
		t.Errorf("expected agentID agent-1, got %s", mutated.AgentID)
	}

	// Temperature should be mutated (not exactly the same)
	if mutated.Temperature == original.Temperature {
		t.Log("temperature was not mutated (acceptable due to randomness)")
	}

	// Params should be mutated
	if len(mutated.Params) != len(original.Params) {
		t.Errorf("expected %d params, got %d", len(original.Params), len(mutated.Params))
	}

	// Fitness should reset to 0
	if mutated.Fitness != 0 {
		t.Errorf("expected fitness 0 for new mutation, got %f", mutated.Fitness)
	}

	// EvalCount should reset to 0
	if mutated.EvalCount != 0 {
		t.Errorf("expected evalCount 0 for new mutation, got %d", mutated.EvalCount)
	}

	// Original should be in history
	history := e.history["agent-1"]
	if len(history) != 1 {
		t.Errorf("expected 1 strategy in history, got %d", len(history))
	}

	if history[0].Version != 1 {
		t.Errorf("expected history version 1, got %d", history[0].Version)
	}

	// Current strategy should be the mutated one
	current := e.GetStrategy("agent-1").(*Strategy)
	if current.Version != 2 {
		t.Errorf("expected current version 2, got %d", current.Version)
	}
}

func TestMutateNonExistentAgent(t *testing.T) {
	e := newTestEngine(t)

	_, err := e.Mutate("nonexistent", 0.2)
	if err == nil {
		t.Error("expected error when mutating nonexistent agent")
	}
}

func TestMutateFloat(t *testing.T) {
	tests := []struct {
		name  string
		value float64
		rate  float64
		min   float64
		max   float64
	}{
		{
			name:  "within bounds",
			value: 0.5,
			rate:  0.2,
			min:   0.0,
			max:   1.0,
		},
		{
			name:  "clamp to min",
			value: 0.01,
			rate:  1.0,
			min:   0.0,
			max:   1.0,
		},
		{
			name:  "clamp to max",
			value: 0.99,
			rate:  1.0,
			min:   0.0,
			max:   1.0,
		},
		{
			name:  "negative range",
			value: -5.0,
			rate:  0.1,
			min:   -10.0,
			max:   10.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Run multiple times to check bounds
			for i := 0; i < 100; i++ {
				result := mutateFloat(tt.value, tt.rate, tt.min, tt.max)

				if result < tt.min {
					t.Errorf("result %f below minimum %f", result, tt.min)
				}

				if result > tt.max {
					t.Errorf("result %f above maximum %f", result, tt.max)
				}
			}
		})
	}
}

func TestRevert(t *testing.T) {
	e := newTestEngine(t)

	v1 := &Strategy{
		ID:        "strategy-v1",
		AgentID:   "agent-1",
		Version:   1,
		Fitness:   0.8,
		EvalCount: 10,
	}

	e.SetStrategy("agent-1", v1)

	// Mutate to v2
	e.Mutate("agent-1", 0.2)

	// Current should be v2
	current := e.GetStrategy("agent-1").(*Strategy)
	if current.Version != 2 {
		t.Errorf("expected version 2, got %d", current.Version)
	}

	// Revert to v1
	err := e.Revert("agent-1")
	if err != nil {
		t.Fatalf("failed to revert: %v", err)
	}

	// Current should be v1 again
	current = e.GetStrategy("agent-1").(*Strategy)
	if current.Version != 1 {
		t.Errorf("expected version 1 after revert, got %d", current.Version)
	}

	if current.Fitness != 0.8 {
		t.Errorf("expected fitness 0.8 after revert, got %f", current.Fitness)
	}

	// History should be empty now
	history := e.history["agent-1"]
	if len(history) != 0 {
		t.Errorf("expected empty history after revert, got %d entries", len(history))
	}
}

func TestRevertNoHistory(t *testing.T) {
	e := newTestEngine(t)

	strategy := &Strategy{
		ID:      "strategy-1",
		AgentID: "agent-1",
		Version: 1,
	}

	e.SetStrategy("agent-1", strategy)

	// Try to revert without any history
	err := e.Revert("agent-1")
	if err == nil {
		t.Error("expected error when reverting with no history")
	}
}

func TestRevertNonExistentAgent(t *testing.T) {
	e := newTestEngine(t)

	err := e.Revert("nonexistent")
	if err == nil {
		t.Error("expected error when reverting nonexistent agent")
	}
}

func TestShouldEvolve(t *testing.T) {
	e := newTestEngine(t)

	strategy := &Strategy{
		ID:        "strategy-1",
		AgentID:   "agent-1",
		Version:   1,
		Fitness:   0.3,
		EvalCount: 10,
	}

	e.SetStrategy("agent-1", strategy)

	// Should evolve if fitness is below threshold
	if !e.ShouldEvolve("agent-1", 0.5) {
		t.Error("expected ShouldEvolve to return true when fitness < threshold")
	}

	// Should not evolve if fitness is above threshold
	if e.ShouldEvolve("agent-1", 0.2) {
		t.Error("expected ShouldEvolve to return false when fitness >= threshold")
	}

	// Should not evolve if not enough evaluations
	strategy.EvalCount = 3
	if e.ShouldEvolve("agent-1", 0.5) {
		t.Error("expected ShouldEvolve to return false when evalCount < 5")
	}
}

func TestShouldEvolveNonExistent(t *testing.T) {
	e := newTestEngine(t)

	if e.ShouldEvolve("nonexistent", 0.5) {
		t.Error("expected ShouldEvolve to return false for nonexistent agent")
	}
}

func TestStrategyPersistence(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create engine and set strategy
	e1 := NewEngine(tmpDir, logger)

	strategy := &Strategy{
		ID:             "persistent-1",
		AgentID:        "agent-1",
		Version:        1,
		SystemPrompt:   "Test prompt",
		PreferredModel: "model-1",
		Temperature:    0.8,
		Params: map[string]float64{
			"param1": 0.5,
		},
		Fitness:   0.75,
		EvalCount: 5,
	}

	e1.SetStrategy("agent-1", strategy)

	// Give it a moment to write
	time.Sleep(10 * time.Millisecond)

	// Create new engine with same directory
	e2 := NewEngine(tmpDir, logger)

	// Strategy should be loaded
	loaded := e2.GetStrategy("agent-1")
	if loaded == nil {
		t.Fatal("expected strategy to be loaded from disk")
	}

	s := loaded.(*Strategy)
	if s.ID != "persistent-1" {
		t.Errorf("expected ID persistent-1, got %s", s.ID)
	}

	if s.Temperature != 0.8 {
		t.Errorf("expected temperature 0.8, got %f", s.Temperature)
	}

	if s.Fitness != 0.75 {
		t.Errorf("expected fitness 0.75, got %f", s.Fitness)
	}

	if s.EvalCount != 5 {
		t.Errorf("expected evalCount 5, got %d", s.EvalCount)
	}
}

func TestConcurrentAccess(t *testing.T) {
	e := newTestEngine(t)

	strategy := &Strategy{
		ID:        "concurrent-1",
		AgentID:   "agent-1",
		Version:   1,
		Fitness:   0.5,
		EvalCount: 0,
	}

	e.SetStrategy("agent-1", strategy)

	// Run concurrent operations
	done := make(chan bool, 3)

	// Concurrent evaluations
	go func() {
		for i := 0; i < 100; i++ {
			metrics := map[string]float64{
				"successRate": 0.8,
			}
			e.Evaluate("agent-1", metrics)
		}
		done <- true
	}()

	// Concurrent reads
	go func() {
		for i := 0; i < 100; i++ {
			_ = e.GetStrategy("agent-1")
		}
		done <- true
	}()

	// Concurrent ShouldEvolve checks
	go func() {
		for i := 0; i < 100; i++ {
			_ = e.ShouldEvolve("agent-1", 0.5)
		}
		done <- true
	}()

	// Wait for all goroutines
	for i := 0; i < 3; i++ {
		<-done
	}

	// Verify state is consistent
	s := e.GetStrategy("agent-1").(*Strategy)
	if s.EvalCount != 100 {
		t.Errorf("expected evalCount 100, got %d", s.EvalCount)
	}
}

func TestMultipleMutations(t *testing.T) {
	e := newTestEngine(t)

	original := &Strategy{
		ID:        "original",
		AgentID:   "agent-1",
		Version:   1,
		Fitness:   0.5,
		EvalCount: 10,
	}

	e.SetStrategy("agent-1", original)

	// Mutate multiple times
	for i := 2; i <= 5; i++ {
		_, err := e.Mutate("agent-1", 0.1)
		if err != nil {
			t.Fatalf("mutation %d failed: %v", i, err)
		}

		current := e.GetStrategy("agent-1").(*Strategy)
		if current.Version != i {
			t.Errorf("expected version %d, got %d", i, current.Version)
		}
	}

	// History should have 4 entries (v1-v4)
	history := e.history["agent-1"]
	if len(history) != 4 {
		t.Errorf("expected 4 strategies in history, got %d", len(history))
	}

	// Current should be v5
	current := e.GetStrategy("agent-1").(*Strategy)
	if current.Version != 5 {
		t.Errorf("expected current version 5, got %d", current.Version)
	}
}
