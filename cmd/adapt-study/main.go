// Command adapt-study runs a controlled, repeated ablation of EvoClaw's guarded
// adaptation mechanism (the paper's Algorithm 1). It isolates the effect of the
// fitness-based accept/reject rule with rollback by comparing three policies over
// the same stream of noisy candidate proposals, across many seeded runs:
//
//   - guarded : the shipped rule — accept iff f' >= f - delta, else roll back
//               (driven through the real evolution engine: mutate -> snapshot ->
//               accept-or-rollback).
//   - greedy  : accept every candidate (models adaptation without rollback).
//   - static  : never adapt (fixed configuration).
//
// It reports, across runs, mean final and mean-over-time deployed fitness per
// policy, how often guarded beats static, and the worst single-step regression
// under guarded — an empirical check of Proposition X.2 (bounded per-step
// regression: no accepted step drops deployed fitness by more than delta).
//
// Usage:
//
//	go run ./cmd/adapt-study                 # or: make repro-adaptation
//	go run ./cmd/adapt-study -runs 30 -steps 40 -delta 0.02 -sigma 0.5
//
// This is a controlled mechanism ablation on a synthetic, stationary-noise
// environment. It isolates the adaptation rule; it is not a production-workload
// or outcome claim. It is deterministic given the seed base and reproduces
// byte-for-byte via `make repro-adaptation`.
package main

import (
	"flag"
	"fmt"
	"log/slog"
	"math"
	"math/rand"
	"os"
	"time"

	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/evolution"
)

const (
	agentID   = "study-agent"
	skillName = "svc"
	seedBase  = 1000
)

// candidateMetrics maps a scalar noise draw p in [-1,1] to a metrics map whose
// engine fitness is 0.6 + 0.3*p (a monotone, invertible operational signal).
func candidateMetrics(p float64) map[string]float64 {
	if p < -1 {
		p = -1
	}
	if p > 1 {
		p = 1
	}
	return map[string]float64{
		"successRate":   0,
		"costUSD":       0,
		"avgResponseMs": 0,
		"profitLoss":    p,
	}
}

// runResult holds the per-run outcome for one policy.
type runResult struct {
	finalFitness float64
	meanFitness  float64
	maxRegress   float64 // largest single-step drop in deployed fitness
}

func main() {
	runs := flag.Int("runs", 30, "number of seeded runs per policy")
	steps := flag.Int("steps", 40, "adaptation steps per run")
	delta := flag.Float64("delta", 0.02, "regression tolerance (Algorithm 1 delta)")
	sigma := flag.Float64("sigma", 0.5, "std dev of the stationary candidate-noise distribution")
	flag.Parse()

	f0 := evolution.Fitness(candidateMetrics(0)) // incumbent baseline (p=0 -> 0.6)

	guarded := make([]runResult, *runs)
	greedy := make([]runResult, *runs)
	static := make([]runResult, *runs)
	for i := 0; i < *runs; i++ {
		seed := int64(seedBase + i)
		draws := drawStream(seed, *steps, *sigma)
		guarded[i] = runGuarded(draws, *delta, f0)
		greedy[i] = runGreedy(draws, f0)
		static[i] = runStatic(*steps, f0)
	}

	fmt.Println("EvoClaw controlled adaptation study — guarded vs greedy vs static")
	fmt.Printf("%d runs x %d steps, delta=%.3f, noise sigma=%.2f, baseline fitness f0=%.3f\n",
		*runs, *steps, *delta, *sigma, f0)
	fmt.Println("=================================================================")
	fmt.Printf("\n%-9s %14s %16s %18s\n", "policy", "final (mean)", "over-time (mean)", "worst step-regress")
	fmt.Println("-----------------------------------------------------------------")
	printPolicy("guarded", guarded)
	printPolicy("greedy", greedy)
	printPolicy("static", static)

	winG := fracGreaterEqual(guarded, static)
	winGvGreedy := fracGreaterEqual(guarded, greedy)
	worst := worstRegression(guarded)

	fmt.Printf("\nGuarded final fitness >= static in %.0f%% of runs; >= greedy in %.0f%% of runs.\n",
		100*winG, 100*winGvGreedy)
	fmt.Printf("Worst single-step regression under guarded across all runs: %.4f (delta=%.3f).\n", worst, *delta)
	if worst <= *delta+1e-9 {
		fmt.Printf("Empirical Proposition X.2 holds: no guarded step regressed by more than delta. ✓\n")
	} else {
		fmt.Printf("WARNING: a guarded step regressed by more than delta (%.4f > %.3f).\n", worst, *delta)
		os.Exit(1)
	}
	fmt.Println("\nScope: controlled synthetic mechanism ablation isolating the accept/reject")
	fmt.Println("rule; not a production-outcome claim. Deterministic; see `make repro-adaptation`.")
}

// drawStream produces a deterministic sequence of candidate noise values.
func drawStream(seed int64, steps int, sigma float64) []float64 {
	r := rand.New(rand.NewSource(seed))
	out := make([]float64, steps)
	for i := range out {
		out[i] = r.NormFloat64() * sigma
	}
	return out
}

// runGuarded drives the real evolution engine's guarded loop over the draws.
func runGuarded(draws []float64, delta, f0 float64) runResult {
	dir, err := os.MkdirTemp("", "adapt-study-")
	if err != nil {
		fatalf("temp dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(dir) }()

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	eng := evolution.NewEngine(dir, logger)
	// Permissive firewall so the rate limiter / circuit breaker do not interfere
	// with a long controlled run; the accept/reject delta-rule is unaffected.
	eng.Firewall = evolution.NewEvolutionFirewall(evolution.FirewallConfig{
		Enabled:              true,
		MaxMutationsPerHour:  1 << 30,
		FitnessDropThreshold: 1.0, // never trips (a drop > 100% is impossible)
		CooldownPeriod:       time.Hour,
		MaxSnapshots:         4,
	})

	g := &config.Genome{
		Identity: config.GenomeIdentity{Name: agentID},
		Skills: map[string]config.SkillGenome{
			skillName: {Enabled: true, Fitness: f0, Params: map[string]interface{}{"x": 1.0}},
		},
	}
	if err := eng.UpdateGenome(agentID, g); err != nil {
		fatalf("seed genome: %v", err)
	}

	deployed := f0
	var sum, worst float64
	for _, p := range draws {
		if _, err := eng.TryEvolveSkill(agentID, skillName, 0.3, delta, candidateMetrics(p)); err != nil {
			fatalf("guarded step: %v", err)
		}
		cur := deployedFitness(eng)
		if drop := deployed - cur; drop > worst {
			worst = drop
		}
		deployed = cur
		sum += deployed
	}
	return runResult{finalFitness: deployed, meanFitness: sum / float64(len(draws)), maxRegress: worst}
}

func deployedFitness(eng *evolution.Engine) float64 {
	g, err := eng.GetGenome(agentID)
	if err != nil {
		fatalf("read genome: %v", err)
	}
	return g.Skills[skillName].Fitness
}

// runGreedy accepts every candidate (no rollback).
func runGreedy(draws []float64, f0 float64) runResult {
	deployed := f0
	var sum, worst float64
	for _, p := range draws {
		cur := evolution.Fitness(candidateMetrics(p))
		if drop := deployed - cur; drop > worst {
			worst = drop
		}
		deployed = cur
		sum += deployed
	}
	return runResult{finalFitness: deployed, meanFitness: sum / float64(len(draws)), maxRegress: worst}
}

// runStatic never adapts.
func runStatic(steps int, f0 float64) runResult {
	return runResult{finalFitness: f0, meanFitness: f0, maxRegress: 0}
}

func printPolicy(name string, rs []runResult) {
	finals := make([]float64, len(rs))
	means := make([]float64, len(rs))
	var maxReg float64
	for i, r := range rs {
		finals[i] = r.finalFitness
		means[i] = r.meanFitness
		if r.maxRegress > maxReg {
			maxReg = r.maxRegress
		}
	}
	fm, fs := meanStd(finals)
	mm, _ := meanStd(means)
	fmt.Printf("%-9s %8.3f±%.3f %16.3f %18.4f\n", name, fm, fs, mm, maxReg)
}

func fracGreaterEqual(a, b []runResult) float64 {
	n := 0
	for i := range a {
		if a[i].finalFitness >= b[i].finalFitness-1e-12 {
			n++
		}
	}
	return float64(n) / float64(len(a))
}

func worstRegression(rs []runResult) float64 {
	var w float64
	for _, r := range rs {
		if r.maxRegress > w {
			w = r.maxRegress
		}
	}
	return w
}

func meanStd(xs []float64) (mean, std float64) {
	if len(xs) == 0 {
		return 0, 0
	}
	for _, x := range xs {
		mean += x
	}
	mean /= float64(len(xs))
	for _, x := range xs {
		std += (x - mean) * (x - mean)
	}
	std = math.Sqrt(std / float64(len(xs)))
	return mean, std
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "adapt-study: "+format+"\n", args...)
	os.Exit(1)
}
