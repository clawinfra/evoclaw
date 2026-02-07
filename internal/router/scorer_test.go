package router

import (
	"testing"
)

func TestNewScorer(t *testing.T) {
	s := NewScorer(nil)
	if s == nil {
		t.Fatal("expected non-nil scorer")
	}
	if len(s.weights) != 14 {
		t.Errorf("expected 14 dimension weights, got %d", len(s.weights))
	}
}

func TestNewScorerWithOverrides(t *testing.T) {
	overrides := map[string]float64{
		DimReasoningMarkers: 0.30,
		DimCodePresence:     0.20,
		"nonexistent_dim":   0.99, // should be ignored
	}
	s := NewScorer(overrides)

	if s.weights[DimReasoningMarkers] != 0.30 {
		t.Errorf("expected reasoning_markers weight 0.30, got %f", s.weights[DimReasoningMarkers])
	}
	if s.weights[DimCodePresence] != 0.20 {
		t.Errorf("expected code_presence weight 0.20, got %f", s.weights[DimCodePresence])
	}
	// Non-overridden should keep default
	if s.weights[DimTokenCount] != defaultWeights[DimTokenCount] {
		t.Errorf("expected token_count to keep default weight")
	}
}

func TestScoreReturns14Dimensions(t *testing.T) {
	s := NewScorer(nil)
	result := s.Score("Hello world")

	if len(result.Dimensions) != 14 {
		t.Errorf("expected 14 dimensions, got %d", len(result.Dimensions))
	}

	// Check all dimension names are present
	expected := map[string]bool{
		DimReasoningMarkers:    false,
		DimCodePresence:        false,
		DimSimpleIndicators:    false,
		DimMultiStepPatterns:   false,
		DimTechnicalTerms:      false,
		DimTokenCount:          false,
		DimCreativeMarkers:     false,
		DimQuestionComplexity:  false,
		DimConstraintCount:     false,
		DimImperativeVerbs:     false,
		DimOutputFormat:        false,
		DimDomainSpecificity:   false,
		DimReferenceComplexity: false,
		DimNegationComplexity:  false,
	}

	for _, d := range result.Dimensions {
		expected[d.Name] = true
	}

	for name, found := range expected {
		if !found {
			t.Errorf("dimension %s not found in result", name)
		}
	}
}

func TestNormalisedScoreInRange(t *testing.T) {
	s := NewScorer(nil)

	prompts := []string{
		"hi",
		"hello",
		"What is the capital of France?",
		"Explain how TCP/IP works in detail, including the three-way handshake, flow control mechanisms, and congestion avoidance algorithms.",
		"Prove by mathematical induction that for all n ≥ 1, the sum of the first n odd numbers equals n². Show each step rigorously.",
		"Write a Go function that implements a concurrent-safe LRU cache with O(1) get and put operations. Include proper error handling, mutex usage, and unit tests.",
	}

	for _, p := range prompts {
		result := s.Score(p)
		if result.Normalised < 0 || result.Normalised > 1 {
			t.Errorf("normalised score out of [0,1] for %q: %f", p[:min(len(p), 50)], result.Normalised)
		}
	}
}

func TestSimplePromptsScoreLow(t *testing.T) {
	s := NewScorer(nil)

	simplePrompts := []string{
		"hi",
		"hello",
		"thanks",
		"yes",
		"ok",
		"how are you",
		"good morning",
	}

	for _, p := range simplePrompts {
		result := s.Score(p)
		if result.Normalised > 0.35 {
			t.Errorf("simple prompt %q scored too high: %.3f (expected < 0.35)", p, result.Normalised)
			for _, d := range result.Dimensions {
				if d.Score != 0 {
					t.Logf("  %s: raw=%.3f weight=%.3f score=%.3f", d.Name, d.Raw, d.Weight, d.Score)
				}
			}
		}
	}
}

func TestComplexPromptsScoreHigh(t *testing.T) {
	s := NewScorer(nil)

	complexPrompts := []string{
		"Prove that the halting problem is undecidable using a diagonal argument. Show each step of the proof rigorously and explain the contradiction that arises.",
		"Implement a distributed consensus algorithm in Go based on Raft protocol. Include leader election with log replication and handle network partitions. Must handle at least 5 nodes with eventual consistency guarantee. Ensure thread safety using mutex and proper concurrent data structure design.",
		"Analyze the time complexity of the following recursive algorithm and prove it is O(n log n) using the master theorem. Then optimize it to achieve O(n) using dynamic programming with memoization.",
	}

	for _, p := range complexPrompts {
		result := s.Score(p)
		if result.Normalised < 0.45 {
			t.Errorf("complex prompt scored too low: %.3f (expected > 0.45)\nprompt: %s", result.Normalised, p[:min(len(p), 80)])
			for _, d := range result.Dimensions {
				if d.Score != 0 {
					t.Logf("  %s: raw=%.3f weight=%.3f score=%.3f", d.Name, d.Raw, d.Weight, d.Score)
				}
			}
		}
	}
}

func TestReasoningPromptsScoreHighest(t *testing.T) {
	s := NewScorer(nil)

	reasoningPrompts := []string{
		"Prove by mathematical induction that for all n ≥ 1: 1 + 2 + ... + n = n(n+1)/2. Then derive the formula for the sum of squares and prove it rigorously step by step. Analyze the time complexity of computing this recursively vs iteratively.",
	}

	for _, p := range reasoningPrompts {
		result := s.Score(p)
		if result.Normalised < 0.55 {
			t.Errorf("reasoning prompt scored too low: %.3f (expected > 0.55)\nprompt: %s", result.Normalised, p[:min(len(p), 80)])
		}
	}
}

func TestTierOrdering(t *testing.T) {
	s := NewScorer(nil)

	// These should be in roughly ascending complexity order
	prompts := []struct {
		label  string
		prompt string
	}{
		{"greeting", "hi there"},
		{"factual", "What is the capital of Australia?"},
		{"moderate", "Explain the difference between TCP and UDP, including their use cases and performance trade-offs."},
		{"complex", "Design a microservice architecture for a high-frequency trading system. Must handle 100k events/sec, ensure at-most-once delivery, implement circuit breakers, and provide a formal proof of the consistency guarantees. Use Go with gRPC."},
	}

	prevScore := -1.0
	for _, tc := range prompts {
		result := s.Score(tc.prompt)
		if result.Normalised < prevScore-0.05 { // allow small tolerance
			t.Errorf("tier ordering violated: %q (%.3f) should be >= previous (%.3f)",
				tc.label, result.Normalised, prevScore)
		}
		prevScore = result.Normalised
		t.Logf("%-12s → score=%.3f raw=%.3f", tc.label, result.Normalised, result.RawTotal)
	}
}

// ── Individual dimension tests ──────────────────────────────────────────────

func TestScoreReasoningMarkers(t *testing.T) {
	tests := []struct {
		input    string
		minScore float64
	}{
		{"hello world", 0},
		{"prove this theorem step by step", 0.4},
		{"derive the formula using mathematical induction and prove the contradiction", 0.8},
	}

	for _, tt := range tests {
		score := scoreReasoningMarkers(tt.input)
		if score < tt.minScore {
			t.Errorf("scoreReasoningMarkers(%q) = %.2f, want >= %.2f", tt.input, score, tt.minScore)
		}
	}
}

func TestScoreCodePresence(t *testing.T) {
	tests := []struct {
		input    string
		minScore float64
	}{
		{"what is love", 0},
		{"write a function in python", 0.08},
		{"```go\nfunc main() {}\n```", 0.4},
		{"implement a `HashMap` using `struct` and `interface`", 0.2},
	}

	for _, tt := range tests {
		score := scoreCodePresence(tt.input)
		if score < tt.minScore {
			t.Errorf("scoreCodePresence(%q) = %.2f, want >= %.2f", tt.input[:min(len(tt.input), 50)], score, tt.minScore)
		}
	}
}

func TestScoreSimpleIndicators(t *testing.T) {
	tests := []struct {
		input    string
		minScore float64
	}{
		{"hi", 0.5},
		{"hello there how are you", 0.3},
		{"Explain the distributed consensus algorithms used in blockchain technology and their mathematical guarantees", 0},
	}

	for _, tt := range tests {
		words := splitWords(tt.input)
		score := scoreSimpleIndicators(tt.input, words)
		if score < tt.minScore {
			t.Errorf("scoreSimpleIndicators(%q) = %.2f, want >= %.2f", tt.input[:min(len(tt.input), 50)], score, tt.minScore)
		}
	}
}

func TestScoreMultiStepPatterns(t *testing.T) {
	tests := []struct {
		input    string
		minScore float64
	}{
		{"what time is it", 0},
		{"step 1: gather data. step 2: process it. step 3: output results.", 0.4},
		{"first do X then do Y, after that do Z, finally output", 0.3},
	}

	for _, tt := range tests {
		score := scoreMultiStepPatterns(tt.input)
		if score < tt.minScore {
			t.Errorf("scoreMultiStepPatterns(%q) = %.2f, want >= %.2f", tt.input[:min(len(tt.input), 60)], score, tt.minScore)
		}
	}
}

func TestScoreTokenCount(t *testing.T) {
	short := "hi"
	if s := scoreTokenCount(short); s != 0 {
		t.Errorf("short prompt should score 0, got %.2f", s)
	}

	// ~500 chars ≈ 125 tokens → should be low-to-mid
	medium := "Lorem ipsum dolor sit amet, consectetur adipiscing elit. " +
		"Sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. " +
		"Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris. " +
		"Duis aute irure dolor in reprehenderit in voluptate velit esse. " +
		"Excepteur sint occaecat cupidatat non proident, sunt in culpa. " +
		"Sed ut perspiciatis unde omnis iste natus error sit voluptatem. " +
		"Nemo enim ipsam voluptatem quia voluptas sit aspernatur aut odit."
	if s := scoreTokenCount(medium); s < 0.01 {
		t.Errorf("medium prompt should score > 0, got %.2f", s)
	}
}

func TestScoreConstraintCount(t *testing.T) {
	none := "tell me about dogs"
	if s := scoreConstraintCount(none); s != 0 {
		t.Errorf("no constraints should score 0, got %.2f", s)
	}

	many := "you must use Go, should handle errors, require at least 95% coverage, ensure thread safety, and limit to no more than 100ms latency"
	if s := scoreConstraintCount(many); s < 0.6 {
		t.Errorf("many constraints should score high, got %.2f", s)
	}
}

func TestScoreNegationComplexity(t *testing.T) {
	none := "tell me about cats"
	if s := scoreNegationComplexity(none); s != 0 {
		t.Errorf("no negation should score 0, got %.2f", s)
	}

	complex := "do not use recursion, don't include external libraries, never mutate state, and avoid global variables without exception"
	if s := scoreNegationComplexity(complex); s < 0.5 {
		t.Errorf("complex negation should score high, got %.2f", s)
	}
}

func TestScoreOutputFormat(t *testing.T) {
	none := "what is 2+2"
	if s := scoreOutputFormat(none); s != 0 {
		t.Errorf("no format requirement should score 0, got %.2f", s)
	}

	formatted := "return as json, include a table of results, and format as markdown"
	if s := scoreOutputFormat(formatted); s > 0 {
		// should be positive
	} else {
		t.Errorf("format requirements should score > 0, got %.2f", s)
	}
}

func TestScoreDomainSpecificity(t *testing.T) {
	general := "tell me a joke"
	if s := scoreDomainSpecificity(general); s > 0.1 {
		t.Errorf("general prompt should score low domain specificity, got %.2f", s)
	}

	finance := "analyze the portfolio's hedge effectiveness using derivatives pricing with futures yield curve arbitrage"
	if s := scoreDomainSpecificity(finance); s < 0.3 {
		t.Errorf("finance domain prompt should score high, got %.2f", s)
	}
}

func TestClamp01(t *testing.T) {
	tests := []struct {
		input    float64
		expected float64
	}{
		{-0.5, 0},
		{0, 0},
		{0.5, 0.5},
		{1.0, 1.0},
		{1.5, 1.0},
	}

	for _, tt := range tests {
		result := clamp01(tt.input)
		if result != tt.expected {
			t.Errorf("clamp01(%f) = %f, want %f", tt.input, result, tt.expected)
		}
	}
}

func TestSigmoid(t *testing.T) {
	// At centre, sigmoid should be ~0.5
	v := sigmoid(0.35, 5.0, 0.35)
	if v < 0.49 || v > 0.51 {
		t.Errorf("sigmoid at centre should be ~0.5, got %.3f", v)
	}

	// Far below centre → near 0
	v = sigmoid(-1.0, 5.0, 0.35)
	if v > 0.01 {
		t.Errorf("sigmoid far below should be near 0, got %.3f", v)
	}

	// Far above centre → near 1
	v = sigmoid(2.0, 5.0, 0.35)
	if v < 0.99 {
		t.Errorf("sigmoid far above should be near 1, got %.3f", v)
	}
}

// ── Benchmark ──────────────────────────────────────────────────────────────

func BenchmarkScore(b *testing.B) {
	s := NewScorer(nil)
	prompt := "Implement a distributed consensus algorithm in Go based on Raft. " +
		"Include leader election, log replication, and handle network partitions. " +
		"Must handle at least 5 nodes. Return the implementation as Go code with tests."

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		s.Score(prompt)
	}
}

// Helper
func splitWords(s string) []string {
	result := make([]string, 0)
	word := ""
	for _, r := range s {
		if r == ' ' || r == '\t' || r == '\n' {
			if word != "" {
				result = append(result, word)
				word = ""
			}
		} else {
			word += string(r)
		}
	}
	if word != "" {
		result = append(result, word)
	}
	return result
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
