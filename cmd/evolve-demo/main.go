// Command evolve-demo reproduces the guarded adaptation loop of EvoClaw's
// evolution engine end to end (the paper's Algorithm 1: Evolution Loop with
// Rollback). It runs two guarded mutation steps on a seeded genome — one whose
// candidate improves fitness (ACCEPTED) and one whose candidate regresses
// (ROLLED BACK) — and prints the outcomes plus the durable adaptation log.
//
// Usage:
//
//	go run ./cmd/evolve-demo      # or: make repro-evolution
//
// This is a deterministic, offline reproduction: no network, no LLM, no cloud.
package main

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/evolution"
)

const (
	agentID   = "demo-agent"
	skillName = "trading"
	delta     = 0.05 // regression tolerance (Algorithm 1)
	mutRate   = 0.3
)

func main() {
	dir, err := os.MkdirTemp("", "evoclaw-evolve-demo-")
	if err != nil {
		fatal(err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn}))
	engine := evolution.NewEngine(dir, logger)

	fmt.Println("EvoClaw guarded adaptation loop — Algorithm 1 (Evolution Loop with Rollback)")
	fmt.Println("============================================================================")

	// --- Case 1: a candidate that improves fitness should be ACCEPTED. ---
	seedGenome(engine, 0.30)
	accept, err := engine.TryEvolveSkill(agentID, skillName, mutRate, delta, improvingTrial())
	if err != nil {
		fatal(err)
	}
	printOutcome("Case 1 (improving candidate)", accept)
	if !accept.Accepted {
		fatalf("expected ACCEPT in case 1, got rollback")
	}

	// --- Case 2: a candidate that regresses should be ROLLED BACK. ---
	seedGenome(engine, 0.90)
	rollback, err := engine.TryEvolveSkill(agentID, skillName, mutRate, delta, regressingTrial())
	if err != nil {
		fatal(err)
	}
	printOutcome("Case 2 (regressing candidate)", rollback)
	if rollback.Accepted {
		fatalf("expected ROLLBACK in case 2, got accept")
	}

	// Confirm the incumbent genome was restored after rollback.
	g, err := engine.GetGenome(agentID)
	if err != nil {
		fatal(err)
	}
	if v, _ := g.Skills[skillName].Params["threshold"].(float64); v != -0.1 {
		fatalf("rollback did not restore incumbent params: threshold=%v", g.Skills[skillName].Params["threshold"])
	}
	fmt.Println("\nRollback restored the incumbent genome (threshold = -0.1). ✓")

	fmt.Println("\nAdaptation log (durable change history, one JSON record per decision):")
	printAdaptationLog(dir)

	fmt.Println("\nReproduced: mutate -> trial -> fitness-based accept/reject -> rollback, end to end. ✓")
}

func seedGenome(engine *evolution.Engine, incumbentFitness float64) {
	g := &config.Genome{
		Identity: config.GenomeIdentity{Name: agentID},
		Skills: map[string]config.SkillGenome{
			skillName: {
				Enabled: true,
				Fitness: incumbentFitness,
				Version: 0,
				Params:  map[string]interface{}{"threshold": -0.1},
			},
		},
	}
	if err := engine.UpdateGenome(agentID, g); err != nil {
		fatal(err)
	}
}

func improvingTrial() map[string]float64 {
	return map[string]float64{"successRate": 1.0, "costUSD": 0.0, "avgResponseMs": 0.0, "profitLoss": 1.0}
}

func regressingTrial() map[string]float64 {
	return map[string]float64{"successRate": 0.0, "costUSD": 10.0, "avgResponseMs": 5000.0, "profitLoss": -0.9}
}

func printOutcome(label string, o evolution.AdaptationOutcome) {
	fmt.Printf("\n%s\n", label)
	fmt.Printf("  decision    : %s\n", o.Decision)
	fmt.Printf("  preFitness  : %.4f\n", o.PreFitness)
	fmt.Printf("  postFitness : %.4f\n", o.PostFitness)
	fmt.Printf("  delta       : %.4f  (accept iff post >= pre - delta, unless circuit breaker trips)\n", o.Delta)
	fmt.Printf("  reason      : %s\n", o.Reason)
}

func printAdaptationLog(dir string) {
	path := filepath.Join(dir, "evolution", "adaptation-log.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		fmt.Printf("  (no adaptation log found: %v)\n", err)
		return
	}
	for _, line := range splitNonEmpty(string(data)) {
		var pretty map[string]any
		if err := json.Unmarshal([]byte(line), &pretty); err == nil {
			fmt.Printf("  %s\n", line)
		}
	}
}

func splitNonEmpty(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			if i > start {
				out = append(out, s[start:i])
			}
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "evolve-demo:", err)
	os.Exit(1)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "evolve-demo: "+format+"\n", args...)
	os.Exit(1)
}
