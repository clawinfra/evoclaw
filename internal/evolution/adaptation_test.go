package evolution

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/clawinfra/evoclaw/internal/config"
)

// improvingMetrics yields a high computeFitness score (good candidate).
func improvingMetrics() map[string]float64 {
	return map[string]float64{
		"successRate":   1.0,
		"costUSD":       0.0,
		"avgResponseMs": 0.0,
		"profitLoss":    1.0,
	}
}

// regressingMetrics yields a low computeFitness score (bad candidate).
func regressingMetrics() map[string]float64 {
	return map[string]float64{
		"successRate":   0.0,
		"costUSD":       10.0,
		"avgResponseMs": 5000.0,
		"profitLoss":    -0.9,
	}
}

// TestTryEvolveSkill_Accept verifies the full guarded loop accepts a candidate
// whose trial fitness holds within the regression tolerance.
func TestTryEvolveSkill_Accept(t *testing.T) {
	e := newTestEngine(t)

	g := &config.Genome{
		Skills: map[string]config.SkillGenome{
			"trading": {
				Enabled: true,
				Fitness: 0.30,
				Version: 0,
				Params:  map[string]interface{}{"threshold": -0.1},
			},
		},
	}
	if err := e.UpdateGenome("agent-1", g); err != nil {
		t.Fatalf("setup genome: %v", err)
	}

	outcome, err := e.TryEvolveSkill("agent-1", "trading", 0.3, 0.05, improvingMetrics())
	if err != nil {
		t.Fatalf("TryEvolveSkill: %v", err)
	}
	if !outcome.Accepted {
		t.Fatalf("expected candidate ACCEPTED, got rollback (post=%.3f)", outcome.PostFitness)
	}
	if outcome.Decision != "accept" {
		t.Errorf("expected decision=accept, got %q", outcome.Decision)
	}

	got, err := e.GetGenome("agent-1")
	if err != nil {
		t.Fatalf("get genome: %v", err)
	}
	sk := got.Skills["trading"]
	if !sk.Verified {
		t.Error("accepted candidate should be marked Verified (VBR)")
	}
	if sk.Version != 1 {
		t.Errorf("expected version 1 after accepted mutation, got %d", sk.Version)
	}
}

// TestTryEvolveSkill_Rollback verifies the full guarded loop rolls a regressing
// candidate back to the incumbent genome.
func TestTryEvolveSkill_Rollback(t *testing.T) {
	e := newTestEngine(t)

	g := &config.Genome{
		Skills: map[string]config.SkillGenome{
			"trading": {
				Enabled: true,
				Fitness: 0.90, // strong incumbent
				Version: 0,
				Params:  map[string]interface{}{"threshold": -0.1},
			},
		},
	}
	if err := e.UpdateGenome("agent-1", g); err != nil {
		t.Fatalf("setup genome: %v", err)
	}

	outcome, err := e.TryEvolveSkill("agent-1", "trading", 0.3, 0.05, regressingMetrics())
	if err != nil {
		t.Fatalf("TryEvolveSkill: %v", err)
	}
	if outcome.Accepted {
		t.Fatalf("expected candidate ROLLED BACK, got accept (post=%.3f)", outcome.PostFitness)
	}
	if outcome.Decision != "rollback" {
		t.Errorf("expected decision=rollback, got %q", outcome.Decision)
	}

	// After rollback the incumbent genome must be restored exactly.
	got, err := e.GetGenome("agent-1")
	if err != nil {
		t.Fatalf("get genome: %v", err)
	}
	sk := got.Skills["trading"]
	if sk.Version != 0 {
		t.Errorf("expected version restored to 0 after rollback, got %d", sk.Version)
	}
	if sk.Fitness != 0.90 {
		t.Errorf("expected incumbent fitness 0.90 restored, got %.3f", sk.Fitness)
	}
	if v, _ := sk.Params["threshold"].(float64); v != -0.1 {
		t.Errorf("expected incumbent param threshold=-0.1 restored, got %v", sk.Params["threshold"])
	}
}

// TestAdaptationLogPersisted verifies each accept/rollback decision is appended
// to adaptation-log.jsonl as a durable, machine-readable change history.
func TestAdaptationLogPersisted(t *testing.T) {
	e := newTestEngine(t)

	g := &config.Genome{
		Skills: map[string]config.SkillGenome{
			"trading": {Enabled: true, Fitness: 0.30, Params: map[string]interface{}{"threshold": -0.1}},
		},
	}
	if err := e.UpdateGenome("agent-1", g); err != nil {
		t.Fatalf("setup genome: %v", err)
	}
	if _, err := e.TryEvolveSkill("agent-1", "trading", 0.3, 0.05, improvingMetrics()); err != nil {
		t.Fatalf("TryEvolveSkill: %v", err)
	}

	path := filepath.Join(e.dataDir, "adaptation-log.jsonl")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("adaptation log not written: %v", err)
	}
	defer func() { _ = f.Close() }()

	sc := bufio.NewScanner(f)
	lines := 0
	for sc.Scan() {
		var rec AdaptationOutcome
		if err := json.Unmarshal(sc.Bytes(), &rec); err != nil {
			t.Fatalf("adaptation log line is not valid JSON: %v", err)
		}
		if rec.AgentID != "agent-1" || rec.SkillName != "trading" {
			t.Errorf("unexpected log record: %+v", rec)
		}
		lines++
	}
	if lines != 1 {
		t.Errorf("expected 1 adaptation log record, got %d", lines)
	}
}
