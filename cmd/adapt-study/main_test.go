package main

import (
	"math"
	"testing"

	"github.com/clawinfra/evoclaw/internal/evolution"
)

func TestCandidateMetricsFitnessMap(t *testing.T) {
	// Engine fitness of candidateMetrics(p) must equal 0.6 + 0.3*p, clamped.
	cases := []struct{ p, want float64 }{
		{0, 0.6}, {1, 0.9}, {-1, 0.3}, {2, 0.9}, {-2, 0.3},
	}
	for _, c := range cases {
		got := evolution.Fitness(candidateMetrics(c.p))
		if math.Abs(got-c.want) > 1e-9 {
			t.Errorf("p=%.1f: fitness=%.4f want %.4f", c.p, got, c.want)
		}
	}
}

func TestDrawStreamDeterministic(t *testing.T) {
	a := drawStream(42, 10, 0.5)
	b := drawStream(42, 10, 0.5)
	if len(a) != 10 {
		t.Fatalf("expected 10 draws, got %d", len(a))
	}
	for i := range a {
		if a[i] != b[i] {
			t.Fatalf("stream not deterministic at %d: %v vs %v", i, a[i], b[i])
		}
	}
	if c := drawStream(43, 10, 0.5); c[0] == a[0] {
		t.Error("different seeds should produce different streams")
	}
}

func TestRunGuarded_NeverRegressesBeyondDelta(t *testing.T) {
	f0 := evolution.Fitness(candidateMetrics(0))
	delta := 0.02
	// An adversarial stream of mostly-worse candidates: guarded must hold near f0.
	draws := drawStream(7, 50, 0.6)
	r := runGuarded(draws, delta, f0)
	if r.maxRegress > delta+1e-9 {
		t.Errorf("guarded regressed beyond delta: %.4f > %.3f", r.maxRegress, delta)
	}
	if r.finalFitness < f0-delta {
		t.Errorf("guarded final %.4f fell below f0-delta %.4f", r.finalFitness, f0-delta)
	}
}

func TestRunStaticIsFlat(t *testing.T) {
	f0 := 0.6
	r := runStatic(20, f0)
	if r.finalFitness != f0 || r.meanFitness != f0 || r.maxRegress != 0 {
		t.Errorf("static should be flat at f0: %+v", r)
	}
}

func TestMeanStd(t *testing.T) {
	m, s := meanStd([]float64{1, 1, 1})
	if m != 1 || s != 0 {
		t.Errorf("constant series: mean=%.3f std=%.3f", m, s)
	}
	m, _ = meanStd([]float64{2, 4})
	if math.Abs(m-3) > 1e-9 {
		t.Errorf("mean of {2,4}=%.3f want 3", m)
	}
}

func TestFracGreaterEqual(t *testing.T) {
	a := []runResult{{finalFitness: 0.9}, {finalFitness: 0.6}}
	b := []runResult{{finalFitness: 0.6}, {finalFitness: 0.6}}
	if f := fracGreaterEqual(a, b); f != 1.0 {
		t.Errorf("expected 1.0, got %.2f", f)
	}
}
