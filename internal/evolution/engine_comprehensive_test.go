package evolution

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/clawinfra/evoclaw/internal/config"
)

func setupTestEngine(t *testing.T) *Engine {
	t.Helper()
	dir := t.TempDir()
	return NewEngine(dir, slog.Default())
}

func setupTestEngineWithGenome(t *testing.T, agentID string) *Engine {
	t.Helper()
	dir := t.TempDir()
	eng := NewEngine(dir, slog.Default())

	genome := &config.Genome{
		Identity: config.GenomeIdentity{Name: agentID, Persona: "test", Voice: "balanced"},
		Skills: map[string]config.SkillGenome{
			"trading": {
				Enabled: true, Weight: 0.7, Fitness: 0.5, Version: 3,
				Params:       map[string]interface{}{"threshold": -0.1, "size": 100.0, "active": true},
				Dependencies: []string{},
				EvalCount:    5,
			},
			"monitoring": {
				Enabled: true, Weight: 0.5, Fitness: 0.3, Version: 2,
				Params:       map[string]interface{}{"interval": 60.0},
				Dependencies: []string{"trading"},
				EvalCount:    3,
			},
			"disabled-skill": {
				Enabled: false, Weight: 0.1, Fitness: 0.1, Version: 1,
				Params: map[string]interface{}{},
			},
		},
		Behavior: config.GenomeBehavior{
			RiskTolerance: 0.3, Verbosity: 0.5, Autonomy: 0.5,
			PromptStyle:      "balanced",
			ToolPreferences:  map[string]float64{},
			ResponsePatterns: []string{},
		},
		Constraints: config.GenomeConstraints{MaxLossUSD: 1000.0},
	}

	if err := eng.UpdateGenome(agentID, genome); err != nil {
		t.Fatalf("setup genome: %v", err)
	}
	return eng
}

func TestNewEngineComprehensiveV2(t *testing.T) {
	eng := setupTestEngine(t)
	if eng == nil {
		t.Fatal("expected non-nil engine")
	}
	if eng.Firewall == nil {
		t.Error("expected firewall")
	}
}

func TestGetSetStrategyComprehensiveV2(t *testing.T) {
	eng := setupTestEngine(t)

	eng.SetStrategy("agent1", &Strategy{
		Params:  map[string]float64{"threshold": 0.5},
		Version: 1,
	})

	s := eng.GetStrategy("agent1")
	if s == nil {
		t.Fatal("expected non-nil strategy")
	}
}

func TestEvaluateComprehensiveV2(t *testing.T) {
	eng := setupTestEngine(t)
	eng.SetStrategy("agent1", &Strategy{
		Params:  map[string]float64{"threshold": 0.5},
		Version: 1,
	})

	metrics := map[string]float64{
		"success_rate": 0.8,
		"response_time": 0.5,
		"cost_efficiency": 0.9,
	}

	// First evaluation
	fitness1 := eng.Evaluate("agent1", metrics)
	if fitness1 <= 0 {
		t.Errorf("expected positive fitness, got %f", fitness1)
	}

	// Second evaluation - should use EMA
	fitness2 := eng.Evaluate("agent1", metrics)
	if fitness2 <= 0 {
		t.Errorf("expected positive fitness, got %f", fitness2)
	}

	// Non-existent agent
	if f := eng.Evaluate("nonexistent", metrics); f != 0 {
		t.Errorf("expected 0 for non-existent agent, got %f", f)
	}
}

func TestMutateComprehensiveV2(t *testing.T) {
	eng := setupTestEngine(t)
	eng.SetStrategy("agent1", &Strategy{
		Params:  map[string]float64{"threshold": 0.5, "size": 100},
		Fitness: 0.3,
		Version: 1,
	})

	mutated, err := eng.Mutate("agent1", 0.3)
	if err != nil {
		t.Fatalf("Mutate() error: %v", err)
	}
	if mutated == nil {
		t.Fatal("expected non-nil mutated strategy")
	}
}

func TestRevertComprehensiveV2(t *testing.T) {
	eng := setupTestEngine(t)
	eng.SetStrategy("agent1", &Strategy{
		Params: map[string]float64{"threshold": 0.5}, Version: 1, Fitness: 0.8,
	})

	// Mutate
	eng.Mutate("agent1", 0.3)

	// Revert
	if err := eng.Revert("agent1"); err != nil {
		t.Fatalf("Revert() error: %v", err)
	}

	// Revert non-existent
	if err := eng.Revert("nonexistent"); err == nil {
		t.Error("expected error for non-existent agent")
	}
}

func TestShouldEvolveComprehensiveV2(t *testing.T) {
	eng := setupTestEngine(t)
	eng.SetStrategy("agent1", &Strategy{
		Params: map[string]float64{"threshold": 0.5}, Fitness: 0.3, EvalCount: 5,
	})

	if !eng.ShouldEvolve("agent1", 0.5) {
		t.Error("expected should evolve (fitness 0.3 < min 0.5)")
	}
	if eng.ShouldEvolve("agent1", 0.2) {
		t.Error("expected should NOT evolve (fitness 0.3 > min 0.2)")
	}
	if eng.ShouldEvolve("nonexistent", 0.5) {
		t.Error("expected false for non-existent agent")
	}
}

func TestSaveLoadStrategiesV2(t *testing.T) {
	dir := t.TempDir()
	eng := NewEngine(dir, slog.Default())

	eng.SetStrategy("agent1", &Strategy{
		Params: map[string]float64{"threshold": 0.5}, Fitness: 0.8, Version: 3,
	})

	// Create new engine from same dir - should load strategies
	eng2 := NewEngine(dir, slog.Default())
	s := eng2.GetStrategy("agent1")
	if s == nil {
		t.Fatal("expected loaded strategy")
	}
	strat := s.(*Strategy)
	if strat.Fitness != 0.8 {
		t.Errorf("fitness = %f, want 0.8", strat.Fitness)
	}
}

func TestGetUpdateGenomeV2(t *testing.T) {
	eng := setupTestEngineWithGenome(t, "agent1")

	genome, err := eng.GetGenome("agent1")
	if err != nil {
		t.Fatalf("GetGenome() error: %v", err)
	}
	if genome.Identity.Name != "agent1" {
		t.Errorf("name = %q, want agent1", genome.Identity.Name)
	}

	// Non-existent
	_, err = eng.GetGenome("nonexistent")
	if err == nil {
		t.Error("expected error for non-existent genome")
	}
}

func TestEvaluateSkillV2(t *testing.T) {
	eng := setupTestEngineWithGenome(t, "agent1")

	metrics := map[string]float64{"success_rate": 0.9}
	fitness, err := eng.EvaluateSkill("agent1", "trading", metrics)
	if err != nil {
		t.Fatalf("EvaluateSkill() error: %v", err)
	}
	if fitness <= 0 {
		t.Errorf("expected positive fitness, got %f", fitness)
	}

	// Non-existent skill
	_, err = eng.EvaluateSkill("agent1", "nonexistent", metrics)
	if err == nil {
		t.Error("expected error for non-existent skill")
	}
}

func TestMutateSkillV2(t *testing.T) {
	eng := setupTestEngineWithGenome(t, "agent1")

	err := eng.MutateSkill("agent1", "trading", 0.3)
	if err != nil {
		t.Fatalf("MutateSkill() error: %v", err)
	}

	// Non-existent skill
	err = eng.MutateSkill("agent1", "nonexistent", 0.3)
	if err == nil {
		t.Error("expected error for non-existent skill")
	}
}

func TestShouldEvolveSkillV2(t *testing.T) {
	eng := setupTestEngineWithGenome(t, "agent1")

	should, err := eng.ShouldEvolveSkill("agent1", "trading", 0.8, 2)
	if err != nil {
		t.Fatalf("ShouldEvolveSkill() error: %v", err)
	}
	if !should {
		t.Error("expected should evolve (fitness 0.5 < 0.8)")
	}

	// Disabled skill
	should, err = eng.ShouldEvolveSkill("agent1", "disabled-skill", 0.8, 1)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if should {
		t.Error("expected false for disabled skill")
	}

	// Non-existent skill
	_, err = eng.ShouldEvolveSkill("agent1", "nonexistent", 0.8, 2)
	if err == nil {
		t.Error("expected error")
	}
}

func TestEvaluateSkillContributionComprehensiveV2(t *testing.T) {
	eng := setupTestEngineWithGenome(t, "agent1")

	contrib := eng.EvaluateSkillContribution("agent1", "trading")
	if contrib <= 0 {
		t.Errorf("expected positive contribution, got %f", contrib)
	}

	// Skill with dependencies
	contrib = eng.EvaluateSkillContribution("agent1", "monitoring")
	if contrib <= 0 {
		t.Errorf("expected positive contribution for monitoring, got %f", contrib)
	}

	// Non-existent
	contrib = eng.EvaluateSkillContribution("agent1", "nonexistent")
	if contrib != 0 {
		t.Errorf("expected 0 for non-existent, got %f", contrib)
	}

	// Non-existent agent
	contrib = eng.EvaluateSkillContribution("nonexistent", "trading")
	if contrib != 0 {
		t.Errorf("expected 0 for non-existent agent, got %f", contrib)
	}
}

func TestOptimizeSkillWeightsComprehensiveV2(t *testing.T) {
	eng := setupTestEngineWithGenome(t, "agent1")

	err := eng.OptimizeSkillWeights("agent1")
	if err != nil {
		t.Fatalf("OptimizeSkillWeights() error: %v", err)
	}
}

func TestShouldDisableSkillV2(t *testing.T) {
	eng := setupTestEngineWithGenome(t, "agent1")

	// Trading has decent fitness, shouldn't be disabled
	result := eng.ShouldDisableSkill("agent1", "trading")
	// Just testing it doesn't crash, result depends on logic
	_ = result
}

func TestShouldEnableSkillV2(t *testing.T) {
	eng := setupTestEngineWithGenome(t, "agent1")

	result := eng.ShouldEnableSkill("agent1", "disabled-skill")
	_ = result
}

func TestCompositionFitnessV2(t *testing.T) {
	eng := setupTestEngineWithGenome(t, "agent1")

	metrics := map[string]float64{"success_rate": 0.8, "response_time": 0.5}
	score := eng.CompositionFitness("agent1", metrics)
	_ = score // Just testing it works

	// Non-existent agent
	score = eng.CompositionFitness("nonexistent", metrics)
	if score != 0 {
		t.Errorf("expected 0 for non-existent agent, got %f", score)
	}
}

func TestSubmitFeedbackV2(t *testing.T) {
	eng := setupTestEngine(t)

	err := eng.SubmitFeedback("agent1", "approval", 0.8, "good response")
	if err != nil {
		t.Fatalf("SubmitFeedback() error: %v", err)
	}

	// Submit many to test trimming
	for i := 0; i < 110; i++ {
		eng.SubmitFeedback("agent1", "approval", 0.5, "test")
	}
}

func TestGetBehaviorMetricsV2(t *testing.T) {
	eng := setupTestEngine(t)

	// No feedback
	metrics := eng.GetBehaviorMetrics("agent1")
	if metrics.ApprovalRate != 0.5 {
		t.Errorf("expected default 0.5, got %f", metrics.ApprovalRate)
	}

	// With feedback
	eng.SubmitFeedback("agent1", "approval", 1.0, "good")
	eng.SubmitFeedback("agent1", "completion", 1.0, "done")
	eng.SubmitFeedback("agent1", "engagement", 1.0, "active")

	metrics = eng.GetBehaviorMetrics("agent1")
	if metrics.ApprovalRate <= 0 {
		t.Errorf("expected positive approval rate, got %f", metrics.ApprovalRate)
	}
}

func TestBehavioralFitnessV2(t *testing.T) {
	eng := setupTestEngine(t)
	fitness := eng.BehavioralFitness("agent1")
	if fitness <= 0 {
		t.Errorf("expected positive fitness, got %f", fitness)
	}
}

func TestMutateBehaviorV2(t *testing.T) {
	eng := setupTestEngineWithGenome(t, "agent1")

	feedbackScores := map[string]float64{
		"risk":      0.8,
		"verbosity": -0.3,
		"autonomy":  0.5,
	}

	err := eng.MutateBehavior("agent1", feedbackScores)
	if err != nil {
		t.Fatalf("MutateBehavior() error: %v", err)
	}
}

func TestVerifyMutationV2(t *testing.T) {
	eng := setupTestEngineWithGenome(t, "agent1")

	// Improvement
	metrics := map[string]float64{"success_rate": 0.9}
	verified, err := eng.VerifyMutation("agent1", "trading", metrics)
	if err != nil {
		t.Fatalf("VerifyMutation() error: %v", err)
	}
	_ = verified

	// Non-existent skill
	_, err = eng.VerifyMutation("agent1", "nonexistent", metrics)
	if err == nil {
		t.Error("expected error")
	}
}

func TestDivergenceScoreV2(t *testing.T) {
	eng := setupTestEngine(t)

	if score := eng.DivergenceScore("nonexistent"); score != 0 {
		t.Errorf("expected 0, got %f", score)
	}

	eng.SetStrategy("agent1", &Strategy{Version: 5})
	if score := eng.DivergenceScore("agent1"); score != 5 {
		t.Errorf("expected 5, got %f", score)
	}
}

func TestCheckADLV2(t *testing.T) {
	eng := setupTestEngine(t)
	eng.SetStrategy("agent1", &Strategy{Version: 10})

	if eng.CheckADL("agent1", 0) {
		t.Error("expected false when maxDivergence=0")
	}
	if !eng.CheckADL("agent1", 5) {
		t.Error("expected true when version(10) > max(5)")
	}
	if eng.CheckADL("agent1", 15) {
		t.Error("expected false when version(10) < max(15)")
	}
}

func TestGetBehaviorHistoryV2(t *testing.T) {
	eng := setupTestEngine(t)

	history, err := eng.GetBehaviorHistory("agent1")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(history) != 0 {
		t.Errorf("expected empty, got %d", len(history))
	}

	eng.SubmitFeedback("agent1", "approval", 0.8, "test")
	history, err = eng.GetBehaviorHistory("agent1")
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if len(history) != 1 {
		t.Errorf("expected 1, got %d", len(history))
	}
}

func TestVerifyGenomeConstraintsV2(t *testing.T) {
	eng := setupTestEngine(t)

	// Unsigned genome (backward compat)
	g := &config.Genome{}
	if err := eng.verifyGenomeConstraints(g); err != nil {
		t.Errorf("expected nil for unsigned genome, got: %v", err)
	}
}

func TestComputeFitnessComprehensiveV2(t *testing.T) {
	metrics := map[string]float64{
		"success_rate": 0.9,
		"response_time": 0.5,
	}
	f := computeFitness(metrics)
	if f <= 0 {
		t.Errorf("expected positive fitness, got %f", f)
	}

	// Empty metrics - still produces some value from default 0 inputs
	f = computeFitness(map[string]float64{})
	if f < 0 {
		t.Errorf("expected non-negative for empty metrics, got %f", f)
	}
}

func TestMutateFloatComprehensiveV2(t *testing.T) {
	val := mutateFloat(0.5, 0.3, 0.0, 1.0)
	if val < 0.0 || val > 1.0 {
		t.Errorf("mutateFloat out of bounds: %f", val)
	}
}

func TestEvaluateVFMV2(t *testing.T) {
	score := EvaluateVFM(0.1, 0.05, 0.02, 1.0)
	if score.Score <= 0 {
		t.Errorf("expected positive VFM score, got %f", score.Score)
	}
	if score.FitnessImprovement != 0.1 {
		t.Errorf("expected fitness improvement 0.1, got %f", score.FitnessImprovement)
	}
}

func TestLoadStrategiesInvalidJSONV2(t *testing.T) {
	dir := t.TempDir()
	evoDir := filepath.Join(dir, "evolution")
	os.MkdirAll(evoDir, 0750)

	// Write invalid JSON
	os.WriteFile(filepath.Join(evoDir, "agent1-strategy.json"), []byte("invalid"), 0640)

	// Should not panic even with invalid JSON on disk
	eng := NewEngine(dir, slog.Default())
	_ = eng // Just ensure no panic
}

func TestGenomeSerializationV2(t *testing.T) {
	eng := setupTestEngineWithGenome(t, "agent1")

	g, _ := eng.GetGenome("agent1")
	data, _ := json.Marshal(g)
	if len(data) == 0 {
		t.Error("expected non-empty JSON")
	}
}
