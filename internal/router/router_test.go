package router

import (
	"log/slog"
	"os"
	"testing"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestNew(t *testing.T) {
	cfg := DefaultRouterConfig()
	r := New(cfg, newTestLogger())

	if r == nil {
		t.Fatal("expected non-nil router")
	}
	if r.scorer == nil {
		t.Error("expected non-nil scorer")
	}
	if r.stats == nil {
		t.Error("expected non-nil stats")
	}
}

func TestRouteSimplePrompt(t *testing.T) {
	cfg := DefaultRouterConfig()
	r := New(cfg, newTestLogger())

	decision := r.Route("hi")

	if decision.Tier != TierSimple {
		t.Errorf("expected TierSimple for 'hi', got %s (score=%.3f)", decision.Tier, decision.Score.Normalised)
	}
	if decision.Model != cfg.TierModels[TierSimple] {
		t.Errorf("expected model %s, got %s", cfg.TierModels[TierSimple], decision.Model)
	}
}

func TestRouteComplexPrompt(t *testing.T) {
	cfg := DefaultRouterConfig()
	r := New(cfg, newTestLogger())

	prompt := "Design a microservice architecture for a distributed trading system. " +
		"Must handle 100k events per second with at-most-once delivery. " +
		"Implement circuit breakers, rate limiting, and provide formal proofs of consistency. " +
		"Use Go with gRPC and ensure thread safety with proper mutex usage."

	decision := r.Route(prompt)

	// Should be at least Complex tier
	if decision.Tier < TierComplex {
		t.Errorf("expected at least TierComplex, got %s (score=%.3f)", decision.Tier, decision.Score.Normalised)
		for _, d := range decision.Score.Dimensions {
			if d.Score != 0 {
				t.Logf("  %s: raw=%.3f score=%.3f", d.Name, d.Raw, d.Score)
			}
		}
	}
}

func TestRouteDisabled(t *testing.T) {
	cfg := DefaultRouterConfig()
	cfg.Enabled = false
	cfg.DefaultTier = TierComplex
	r := New(cfg, newTestLogger())

	// Even a simple prompt should use the default tier when routing is disabled
	decision := r.Route("hi")

	if decision.Tier != TierComplex {
		t.Errorf("expected default tier TierComplex when disabled, got %s", decision.Tier)
	}
	if decision.Model != cfg.TierModels[TierComplex] {
		t.Errorf("expected model %s, got %s", cfg.TierModels[TierComplex], decision.Model)
	}
}

func TestRouteAndTrackCostSavings(t *testing.T) {
	cfg := DefaultRouterConfig()
	r := New(cfg, newTestLogger())

	// Simulate 5 simple, 3 medium, 2 complex requests
	simplePrompts := []string{
		"hi", "hello", "thanks", "ok", "bye",
	}
	mediumPrompts := []string{
		"Explain how DNS works and the difference between A and CNAME records.",
		"Summarize the key features of Go 1.22 release including range-over-func and improved routing.",
		"Compare REST and GraphQL for a typical web application with moderate complexity.",
	}
	complexPrompts := []string{
		"Design and implement a distributed lock manager using Raft consensus with Go. Must handle network partitions, ensure linearizability, and include formal correctness proofs step by step.",
		"Prove by mathematical induction that the time complexity of merge sort is O(n log n). Then derive the recurrence relation and solve it using the master theorem. Implement the optimized version in Go.",
	}

	estimatedTokens := 2000 // average per request

	for _, p := range simplePrompts {
		r.RouteAndTrack(p, estimatedTokens)
	}
	for _, p := range mediumPrompts {
		r.RouteAndTrack(p, estimatedTokens)
	}
	for _, p := range complexPrompts {
		r.RouteAndTrack(p, estimatedTokens)
	}

	savings := r.GetSavings()

	if savings.TotalRequests != 10 {
		t.Errorf("expected 10 total requests, got %d", savings.TotalRequests)
	}

	// Simple requests should have been cheap → there should be savings
	if savings.SavedUSD <= 0 {
		t.Error("expected positive cost savings from routing simple prompts cheaply")
	}

	if savings.SavingsPercent <= 0 {
		t.Errorf("expected positive savings percentage, got %.1f%%", savings.SavingsPercent)
	}

	// Baseline should always be >= estimated (since we're routing some to cheaper models)
	if savings.BaselineCost < savings.EstimatedCost {
		t.Errorf("baseline cost ($%.4f) should be >= estimated cost ($%.4f)",
			savings.BaselineCost, savings.EstimatedCost)
	}

	t.Logf("Savings: $%.4f (%.1f%%) — baseline $%.4f, routed $%.4f",
		savings.SavedUSD, savings.SavingsPercent, savings.BaselineCost, savings.EstimatedCost)
	t.Logf("Tier distribution: %v", savings.RequestsByTier)
}

func TestSavingsReport(t *testing.T) {
	cfg := DefaultRouterConfig()
	r := New(cfg, newTestLogger())

	r.RouteAndTrack("hi", 500)
	r.RouteAndTrack("hello", 500)
	r.RouteAndTrack("Explain TCP/IP in detail with all protocol layers and their interactions", 3000)

	report := r.SavingsReport()

	if report == "" {
		t.Error("expected non-empty savings report")
	}

	// Should contain key sections
	for _, expected := range []string{"LLM Router Cost Report", "Total Requests", "Saved", "Tier Distribution"} {
		if !containsStr(report, expected) {
			t.Errorf("report missing expected section: %s", expected)
		}
	}

	t.Log("\n" + report)
}

func TestSelectTier(t *testing.T) {
	thresholds := [3]float64{0.25, 0.50, 0.75}

	tests := []struct {
		score    float64
		expected Tier
	}{
		{0.0, TierSimple},
		{0.10, TierSimple},
		{0.24, TierSimple},
		{0.25, TierMedium},
		{0.30, TierMedium},
		{0.49, TierMedium},
		{0.50, TierComplex},
		{0.60, TierComplex},
		{0.74, TierComplex},
		{0.75, TierReasoning},
		{0.90, TierReasoning},
		{1.00, TierReasoning},
	}

	for _, tt := range tests {
		result := SelectTier(tt.score, thresholds)
		if result != tt.expected {
			t.Errorf("SelectTier(%.2f) = %s, want %s", tt.score, result, tt.expected)
		}
	}
}

func TestTierString(t *testing.T) {
	tests := []struct {
		tier     Tier
		expected string
	}{
		{TierSimple, "SIMPLE"},
		{TierMedium, "MEDIUM"},
		{TierComplex, "COMPLEX"},
		{TierReasoning, "REASONING"},
		{Tier(99), "UNKNOWN"},
	}

	for _, tt := range tests {
		if tt.tier.String() != tt.expected {
			t.Errorf("Tier(%d).String() = %s, want %s", tt.tier, tt.tier.String(), tt.expected)
		}
	}
}

func TestTierJSON(t *testing.T) {
	tier := TierComplex
	data, err := tier.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}
	if string(data) != `"COMPLEX"` {
		t.Errorf("expected \"COMPLEX\", got %s", string(data))
	}

	var decoded Tier
	err = decoded.UnmarshalJSON(data)
	if err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}
	if decoded != TierComplex {
		t.Errorf("expected TierComplex, got %s", decoded)
	}

	// Test integer unmarshal
	err = decoded.UnmarshalJSON([]byte("1"))
	if err != nil {
		t.Fatalf("UnmarshalJSON(int) failed: %v", err)
	}
	if decoded != TierMedium {
		t.Errorf("expected TierMedium from int 1, got %s", decoded)
	}
}

func TestModelForTierFallback(t *testing.T) {
	cfg := DefaultRouterConfig()
	// Remove one tier's model
	delete(cfg.TierModels, TierSimple)
	r := New(cfg, newTestLogger())

	// Should fall back to the default tier model
	model := r.modelForTier(TierSimple)
	if model != cfg.TierModels[cfg.DefaultTier] {
		t.Errorf("expected fallback to default tier model %s, got %s", cfg.TierModels[cfg.DefaultTier], model)
	}
}

func TestCostPerMillion(t *testing.T) {
	cfg := DefaultRouterConfig()

	if cost := cfg.CostPerMillion(TierSimple); cost != 0.27 {
		t.Errorf("expected 0.27, got %f", cost)
	}
	if cost := cfg.CostPerMillion(TierReasoning); cost != 10.0 {
		t.Errorf("expected 10.0, got %f", cost)
	}

	// Empty costs should use defaults
	cfg.TierCosts = nil
	if cost := cfg.CostPerMillion(TierSimple); cost != 0.27 {
		t.Errorf("expected default 0.27, got %f", cost)
	}
}

func TestConcurrentRouting(t *testing.T) {
	cfg := DefaultRouterConfig()
	r := New(cfg, newTestLogger())

	done := make(chan bool, 20)

	for i := 0; i < 20; i++ {
		go func(i int) {
			prompt := "tell me about cats"
			if i%2 == 0 {
				prompt = "Implement a distributed hash table with consistent hashing, virtual nodes, and replication. Must handle node failures gracefully with formal proofs."
			}
			r.RouteAndTrack(prompt, 1000)
			done <- true
		}(i)
	}

	for i := 0; i < 20; i++ {
		<-done
	}

	savings := r.GetSavings()
	if savings.TotalRequests != 20 {
		t.Errorf("expected 20 requests, got %d", savings.TotalRequests)
	}
}

func TestRouteDecisionHasTiming(t *testing.T) {
	cfg := DefaultRouterConfig()
	r := New(cfg, newTestLogger())

	decision := r.Route("hello")

	if decision.Timestamp.IsZero() {
		t.Error("expected non-zero timestamp")
	}
	// DurationUs can be 0 on very fast systems, just check it's not negative
	if decision.DurationUs < 0 {
		t.Errorf("expected non-negative duration, got %d", decision.DurationUs)
	}
}

func TestRouterConfig(t *testing.T) {
	cfg := DefaultRouterConfig()
	r := New(cfg, newTestLogger())

	got := r.Config()
	if !got.Enabled {
		t.Error("expected router to be enabled")
	}
	if got.DefaultTier != TierComplex {
		t.Errorf("expected default tier Complex, got %s", got.DefaultTier)
	}
}

// ── Integration-style tests with realistic prompts ──────────────────────────

func TestRealisticWorkloadDistribution(t *testing.T) {
	cfg := DefaultRouterConfig()
	r := New(cfg, newTestLogger())

	// Simulate a realistic workload
	workload := []struct {
		prompt string
		minTier Tier
		maxTier Tier
	}{
		// Simple tier
		{"hi", TierSimple, TierSimple},
		{"thanks", TierSimple, TierMedium},
		{"what time is it", TierSimple, TierMedium},

		// Medium tier
		{"Explain how HTTPS works", TierSimple, TierComplex},
		{"Summarize the differences between REST and GraphQL", TierSimple, TierComplex},

		// Complex tier
		{"Write a Go microservice with PostgreSQL integration, proper error handling, middleware for auth and logging, and Docker compose setup. Include unit tests and integration tests.", TierMedium, TierReasoning},

		// Reasoning tier
		{"Prove by mathematical induction that for all n >= 1, the sum 1^2 + 2^2 + ... + n^2 = n(n+1)(2n+1)/6. Then derive the time complexity analysis step by step and prove it is optimal.", TierComplex, TierReasoning},
	}

	for _, tc := range workload {
		decision := r.Route(tc.prompt)
		if decision.Tier < tc.minTier || decision.Tier > tc.maxTier {
			t.Errorf("prompt %q: expected tier in [%s, %s], got %s (score=%.3f)",
				tc.prompt[:min(len(tc.prompt), 60)],
				tc.minTier, tc.maxTier, decision.Tier, decision.Score.Normalised)
		}
	}
}

// ── Benchmark ──────────────────────────────────────────────────────────────

func BenchmarkRoute(b *testing.B) {
	cfg := DefaultRouterConfig()
	r := New(cfg, slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError})))
	prompt := "Implement a distributed consensus algorithm in Go based on Raft. " +
		"Include leader election, log replication, and handle network partitions."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.Route(prompt)
	}
}

func BenchmarkRouteAndTrack(b *testing.B) {
	cfg := DefaultRouterConfig()
	r := New(cfg, slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError})))
	prompt := "hello world"

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		r.RouteAndTrack(prompt, 1000)
	}
}

// Helper
func containsStr(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && // guard
		len(s) >= len(substr) &&
		findSubstr(s, substr)
}

func findSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
