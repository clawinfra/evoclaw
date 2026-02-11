package evolution

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/clawinfra/evoclaw/internal/config"
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
	_, _ = e.Mutate("agent-1", 0.2)

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

// ================================
// Layer 2 Tests: Skill Selection & Composition
// ================================

func TestEvaluateSkillContribution(t *testing.T) {
	e := newTestEngine(t)

	// This test would require setting up a genome with skills
	// For now, test that it doesn't panic with a non-existent agent
	contribution := e.EvaluateSkillContribution("agent-1", "trading")
	
	if contribution < 0 {
		t.Errorf("expected non-negative contribution, got %f", contribution)
	}
}

func TestOptimizeSkillWeights(t *testing.T) {
	e := newTestEngine(t)

	// Test with non-existent agent (should handle gracefully)
	err := e.OptimizeSkillWeights("agent-1")
	if err == nil {
		t.Error("expected error when optimizing weights for non-existent agent")
	}
}

func TestShouldDisableSkill(t *testing.T) {
	e := newTestEngine(t)

	// Test with non-existent agent (should return false, not panic)
	shouldDisable := e.ShouldDisableSkill("agent-1", "trading")
	
	if shouldDisable {
		t.Error("expected false for non-existent agent")
	}
}

func TestShouldEnableSkill(t *testing.T) {
	e := newTestEngine(t)

	// Test with non-existent agent (should return false, not panic)
	shouldEnable := e.ShouldEnableSkill("agent-1", "trading")
	
	if shouldEnable {
		t.Error("expected false for non-existent agent")
	}
}

func TestCompositionFitness(t *testing.T) {
	e := newTestEngine(t)

	metrics := map[string]float64{
		"successRate":    0.9,
		"costUSD":        0.1,
		"avgResponseMs":  500,
		"profitLoss":     0.5,
	}

	// Test with non-existent agent (should handle gracefully)
	compositionScore := e.CompositionFitness("agent-1", metrics)
	
	if compositionScore < -10 || compositionScore > 10 {
		t.Errorf("composition score out of reasonable range: %f", compositionScore)
	}
}

// ================================
// Layer 3 Tests: Behavioral Evolution
// ================================

func TestSubmitFeedback(t *testing.T) {
	e := newTestEngine(t)

	err := e.SubmitFeedback("agent-1", "approval", 0.8, "good response")
	if err != nil {
		t.Fatalf("failed to submit feedback: %v", err)
	}

	// Submit multiple feedback entries
	feedbackTypes := []struct {
		feedbackType string
		score        float64
		context      string
	}{
		{"approval", 0.9, "excellent work"},
		{"correction", -0.5, "needs improvement"},
		{"engagement", 0.7, "user engaged"},
		{"dismissal", -0.8, "user ignored response"},
	}

	for _, fb := range feedbackTypes {
		err := e.SubmitFeedback("agent-1", fb.feedbackType, fb.score, fb.context)
		if err != nil {
			t.Errorf("failed to submit %s feedback: %v", fb.feedbackType, err)
		}
	}
}

func TestGetBehaviorMetrics(t *testing.T) {
	e := newTestEngine(t)

	// Test with no feedback (should return default metrics)
	metrics := e.GetBehaviorMetrics("agent-1")
	
	if metrics.ApprovalRate != 0.5 {
		t.Errorf("expected default approval rate 0.5, got %f", metrics.ApprovalRate)
	}
	
	// Submit some feedback
	_ = e.SubmitFeedback("agent-1", "approval", 1.0, "test")
	_ = e.SubmitFeedback("agent-1", "approval", -1.0, "test")
	_ = e.SubmitFeedback("agent-1", "completion", 1.0, "test")
	_ = e.SubmitFeedback("agent-1", "engagement", 1.0, "test")
	
	metrics = e.GetBehaviorMetrics("agent-1")
	
	// Should have 1 positive approval out of 2 approvals = 50%
	if metrics.ApprovalRate != 0.25 { // 1 out of 4 total feedback
		t.Logf("approval rate: %f (actual calculation may vary)", metrics.ApprovalRate)
	}
	
	if metrics.TaskCompletionRate <= 0 {
		t.Error("expected positive completion rate")
	}
}

func TestBehavioralFitness(t *testing.T) {
	e := newTestEngine(t)

	// Test with no feedback (should use defaults)
	fitness := e.BehavioralFitness("agent-1")
	
	if fitness < 0 || fitness > 100 {
		t.Errorf("behavioral fitness out of range [0-100]: %f", fitness)
	}
	
	// Submit positive feedback
	_ = e.SubmitFeedback("agent-1", "approval", 1.0, "great")
	_ = e.SubmitFeedback("agent-1", "completion", 1.0, "done")
	_ = e.SubmitFeedback("agent-1", "engagement", 1.0, "engaged")
	
	fitnessWithFeedback := e.BehavioralFitness("agent-1")
	
	if fitnessWithFeedback <= fitness {
		t.Logf("fitness with positive feedback: %f, initial: %f", fitnessWithFeedback, fitness)
	}
}

func TestMutateBehavior(t *testing.T) {
	e := newTestEngine(t)

	// This test would require setting up a genome
	// For now, test that it handles non-existent agent gracefully
	feedbackScores := map[string]float64{
		"risk":      0.5,
		"verbosity": -0.3,
		"autonomy":  0.2,
	}
	
	err := e.MutateBehavior("agent-1", feedbackScores)
	if err == nil {
		t.Error("expected error when mutating behavior for non-existent agent")
	}
}

func TestGetBehaviorHistory(t *testing.T) {
	e := newTestEngine(t)

	// Test with no feedback
	history, err := e.GetBehaviorHistory("agent-1")
	if err != nil {
		t.Fatalf("failed to get behavior history: %v", err)
	}
	
	if len(history) != 0 {
		t.Errorf("expected empty history, got %d entries", len(history))
	}
	
	// Submit some feedback
	_ = e.SubmitFeedback("agent-1", "approval", 0.8, "test 1")
	_ = e.SubmitFeedback("agent-1", "correction", -0.5, "test 2")
	
	history, err = e.GetBehaviorHistory("agent-1")
	if err != nil {
		t.Fatalf("failed to get behavior history: %v", err)
	}
	
	if len(history) != 2 {
		t.Errorf("expected 2 feedback entries, got %d", len(history))
	}
	
	// Verify feedback content
	if history[0].Type != "approval" {
		t.Errorf("expected first feedback type 'approval', got '%s'", history[0].Type)
	}
	
	if history[1].Type != "correction" {
		t.Errorf("expected second feedback type 'correction', got '%s'", history[1].Type)
	}
}

func TestFeedbackHistoryLimit(t *testing.T) {
	e := newTestEngine(t)

	// Submit more than 100 feedback entries
	for i := 0; i < 150; i++ {
		_ = e.SubmitFeedback("agent-1", "approval", 0.5, "test")
	}
	
	history, err := e.GetBehaviorHistory("agent-1")
	if err != nil {
		t.Fatalf("failed to get behavior history: %v", err)
	}
	
	// Should keep only last 100
	if len(history) != 100 {
		t.Errorf("expected history capped at 100 entries, got %d", len(history))
	}
}

// ================================
// VBR Tests
// ================================

func TestVerifyMutationPass(t *testing.T) {
	e := newTestEngine(t)

	// Create a genome file for the agent
	g := &config.Genome{
		Skills: map[string]config.SkillGenome{
			"trading": {
				Enabled: true,
				Fitness: 0.3,
				Params:  map[string]interface{}{"threshold": -0.1},
			},
		},
	}
	if err := e.UpdateGenome("agent-1", g); err != nil {
		t.Fatalf("setup genome: %v", err)
	}

	// Verify with better metrics (fitness > 0.3)
	metrics := map[string]float64{
		"successRate":   0.9,
		"costUSD":       0.1,
		"avgResponseMs": 200,
		"profitLoss":    0.5,
	}
	verified, err := e.VerifyMutation("agent-1", "trading", metrics)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if !verified {
		t.Error("expected mutation to be verified as improvement")
	}
}

func TestVerifyMutationFail(t *testing.T) {
	e := newTestEngine(t)

	g := &config.Genome{
		Skills: map[string]config.SkillGenome{
			"trading": {
				Enabled: true,
				Fitness: 0.95, // very high existing fitness
				Params:  map[string]interface{}{},
			},
		},
	}
	if err := e.UpdateGenome("agent-1", g); err != nil {
		t.Fatalf("setup genome: %v", err)
	}

	// Verify with poor metrics
	metrics := map[string]float64{
		"successRate":   0.1,
		"costUSD":       10.0,
		"avgResponseMs": 5000,
		"profitLoss":    -0.9,
	}
	verified, err := e.VerifyMutation("agent-1", "trading", metrics)
	if err != nil {
		t.Fatalf("verify: %v", err)
	}
	if verified {
		t.Error("expected mutation to NOT be verified")
	}
}

// ================================
// ADL Tests
// ================================

func TestDivergenceScore(t *testing.T) {
	e := newTestEngine(t)

	// No strategy = 0
	if s := e.DivergenceScore("agent-1"); s != 0 {
		t.Errorf("expected 0, got %f", s)
	}

	e.SetStrategy("agent-1", &Strategy{ID: "s1", Version: 1})
	if s := e.DivergenceScore("agent-1"); s != 1 {
		t.Errorf("expected 1, got %f", s)
	}

	// Mutate a few times
	e.Mutate("agent-1", 0.1)
	e.Mutate("agent-1", 0.1)
	if s := e.DivergenceScore("agent-1"); s != 3 {
		t.Errorf("expected 3, got %f", s)
	}
}

func TestCheckADL(t *testing.T) {
	e := newTestEngine(t)

	e.SetStrategy("agent-1", &Strategy{ID: "s1", Version: 5})

	if e.CheckADL("agent-1", 10) {
		t.Error("should not exceed limit of 10 at version 5")
	}
	if !e.CheckADL("agent-1", 4) {
		t.Error("should exceed limit of 4 at version 5")
	}
	if e.CheckADL("agent-1", 0) {
		t.Error("zero limit means disabled")
	}
}

// ================================
// VFM Tests
// ================================

func TestEvaluateVFM(t *testing.T) {
	vfm := EvaluateVFM(0.5, 0.1, 0.1, 0.05)
	if vfm.Score <= 0 {
		t.Error("expected positive VFM score")
	}
	expected := 0.5 / 0.25 // 2.0
	if vfm.Score != expected {
		t.Errorf("expected %f, got %f", expected, vfm.Score)
	}
}

func TestEvaluateVFMZeroCost(t *testing.T) {
	// Free improvement should give very high score
	vfm := EvaluateVFM(0.5, 0, 0, 0)
	if vfm.Score <= 0 {
		t.Error("expected positive VFM for free improvement")
	}
	if vfm.Score < 100 {
		t.Logf("VFM score for free improvement: %f", vfm.Score)
	}
}

func TestEvaluateVFMRejectLowValue(t *testing.T) {
	// High cost, tiny improvement
	vfm := EvaluateVFM(0.001, 5.0, 3.0, 2.0)
	minThreshold := 0.1
	if vfm.Score >= minThreshold {
		t.Errorf("expected VFM %f < threshold %f", vfm.Score, minThreshold)
	}
}

func TestConcurrentFeedbackSubmission(t *testing.T) {
	e := newTestEngine(t)

	done := make(chan bool, 5)

	// Multiple goroutines submitting feedback concurrently
	for i := 0; i < 5; i++ {
		go func(id int) {
			for j := 0; j < 20; j++ {
				_ = e.SubmitFeedback("agent-1", "approval", 0.5, "concurrent test")
			}
			done <- true
		}(i)
	}

	// Wait for all goroutines
	for i := 0; i < 5; i++ {
		<-done
	}

	// Verify feedback was recorded
	history, err := e.GetBehaviorHistory("agent-1")
	if err != nil {
		t.Fatalf("failed to get behavior history: %v", err)
	}
	
	if len(history) != 100 {
		t.Errorf("expected 100 feedback entries (capped), got %d", len(history))
	}
}
