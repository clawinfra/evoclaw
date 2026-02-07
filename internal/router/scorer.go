package router

import (
	"math"
	"regexp"
	"strings"
	"unicode/utf8"
)

// Dimension names — used as keys for weight overrides.
const (
	DimReasoningMarkers   = "reasoning_markers"
	DimCodePresence       = "code_presence"
	DimSimpleIndicators   = "simple_indicators"
	DimMultiStepPatterns  = "multi_step_patterns"
	DimTechnicalTerms     = "technical_terms"
	DimTokenCount         = "token_count"
	DimCreativeMarkers    = "creative_markers"
	DimQuestionComplexity = "question_complexity"
	DimConstraintCount    = "constraint_count"
	DimImperativeVerbs    = "imperative_verbs"
	DimOutputFormat       = "output_format"
	DimDomainSpecificity  = "domain_specificity"
	DimReferenceComplexity = "reference_complexity"
	DimNegationComplexity = "negation_complexity"
)

// DimensionScore holds a single dimension's result.
type DimensionScore struct {
	Name   string  `json:"name"`
	Raw    float64 `json:"raw"`    // raw score before weighting
	Weight float64 `json:"weight"` // weight applied
	Score  float64 `json:"score"`  // raw * weight (contribution to total)
}

// ScoreResult holds the full scoring output.
type ScoreResult struct {
	Dimensions []DimensionScore `json:"dimensions"`
	RawTotal   float64          `json:"rawTotal"`   // sum of weighted scores
	Normalised float64          `json:"normalised"` // clamped to [0,1]
}

// defaultWeights are the starting weights for each dimension.
// These are tuned to produce good tier separation on realistic prompts.
// The weights are intentionally larger than 1.0 in sum so that complex prompts
// accumulate enough raw score to push through the sigmoid into higher tiers.
var defaultWeights = map[string]float64{
	DimReasoningMarkers:    0.30,
	DimCodePresence:        0.25,
	DimSimpleIndicators:    -0.20, // negative — pushes simple prompts down
	DimMultiStepPatterns:   0.20,
	DimTechnicalTerms:      0.15,
	DimTokenCount:          0.15,
	DimCreativeMarkers:     0.10,
	DimQuestionComplexity:  0.20,
	DimConstraintCount:     0.15,
	DimImperativeVerbs:     0.08,
	DimOutputFormat:        0.10,
	DimDomainSpecificity:   0.12,
	DimReferenceComplexity: 0.10,
	DimNegationComplexity:  0.06,
}

// Scorer evaluates prompt complexity across 14 dimensions.
type Scorer struct {
	weights map[string]float64
}

// NewScorer creates a Scorer, optionally merging weight overrides.
func NewScorer(overrides map[string]float64) *Scorer {
	w := make(map[string]float64, len(defaultWeights))
	for k, v := range defaultWeights {
		w[k] = v
	}
	for k, v := range overrides {
		if _, ok := w[k]; ok {
			w[k] = v
		}
	}
	return &Scorer{weights: w}
}

// Score evaluates a prompt and returns the full result.
func (s *Scorer) Score(prompt string) ScoreResult {
	lower := strings.ToLower(prompt)
	words := strings.Fields(lower)

	dims := []DimensionScore{
		s.dim(DimReasoningMarkers, scoreReasoningMarkers(lower)),
		s.dim(DimCodePresence, scoreCodePresence(lower)),
		s.dim(DimSimpleIndicators, scoreSimpleIndicators(lower, words)),
		s.dim(DimMultiStepPatterns, scoreMultiStepPatterns(lower)),
		s.dim(DimTechnicalTerms, scoreTechnicalTerms(lower, words)),
		s.dim(DimTokenCount, scoreTokenCount(prompt)),
		s.dim(DimCreativeMarkers, scoreCreativeMarkers(lower)),
		s.dim(DimQuestionComplexity, scoreQuestionComplexity(lower)),
		s.dim(DimConstraintCount, scoreConstraintCount(lower)),
		s.dim(DimImperativeVerbs, scoreImperativeVerbs(lower, words)),
		s.dim(DimOutputFormat, scoreOutputFormat(lower)),
		s.dim(DimDomainSpecificity, scoreDomainSpecificity(lower)),
		s.dim(DimReferenceComplexity, scoreReferenceComplexity(lower)),
		s.dim(DimNegationComplexity, scoreNegationComplexity(lower)),
	}

	var total float64
	for _, d := range dims {
		total += d.Score
	}

	// Normalise to [0,1] using a sigmoid-like function.
	// With weights summing to ~2, typical raw totals range ~[-0.20, 1.5].
	// Centre at 0.30 so that moderate prompts land around 0.5.
	normalised := sigmoid(total, 3.5, 0.30)

	return ScoreResult{
		Dimensions: dims,
		RawTotal:   total,
		Normalised: normalised,
	}
}

func (s *Scorer) dim(name string, raw float64) DimensionScore {
	w := s.weights[name]
	return DimensionScore{
		Name:   name,
		Raw:    raw,
		Weight: w,
		Score:  raw * w,
	}
}

// sigmoid maps x to [0,1] with adjustable steepness and centre.
func sigmoid(x, steepness, centre float64) float64 {
	v := 1.0 / (1.0 + math.Exp(-steepness*(x-centre)))
	return clamp01(v)
}

func clamp01(v float64) float64 {
	if v < 0 {
		return 0
	}
	if v > 1 {
		return 1
	}
	return v
}

// ── Dimension scoring functions ──────────────────────────────────────────────

// Compiled regexes (package-level, compiled once).
var (
	reCodeFence       = regexp.MustCompile("(?s)```")
	reInlineCode      = regexp.MustCompile("`[^`]+`")
	reStepPatterns    = regexp.MustCompile(`\b(step\s*\d|first\b.*then\b|next\b|finally\b|phase\s*\d|stage\s*\d)`)
	reNumberedList    = regexp.MustCompile(`(?m)^\s*\d+[\.\)]\s`)
	reBulletList      = regexp.MustCompile(`(?m)^\s*[-*•]\s`)
	reConstraintWords = regexp.MustCompile(`\b(must|should|require|ensure|constraint|limit|at least|at most|no more than|no fewer than|between \d+ and \d+|exactly \d+|only if|unless|except)\b`)
	reNegationComplexity = regexp.MustCompile(`\b(not|don't|doesn't|shouldn't|mustn't|cannot|can't|never|neither|nor|without|except|exclude|avoid|instead of)\b`)
	reOutputFormat    = regexp.MustCompile(`\b(json|xml|csv|yaml|markdown|table|list|bullet|format as|output as|return as|give me a|in the format)\b`)
	reQuestionWords   = regexp.MustCompile(`\b(why|how|explain|compare|contrast|analyse|analyze|evaluate|what causes|what if|implications|trade-?offs?|pros and cons)\b`)
	reSimpleQuestion  = regexp.MustCompile(`\b(what is|who is|when|where|define|name|list)\b`)
	reCreative        = regexp.MustCompile(`\b(write|compose|create|generate|draft|story|poem|song|script|imagine|creative|fiction|narrative|dialogue)\b`)
	reImperative      = regexp.MustCompile(`\b(implement|build|design|architect|refactor|optimise|optimize|debug|deploy|migrate|integrate|configure|set up|transform|convert|parse|validate|benchmark|profile)\b`)
	reReference       = regexp.MustCompile(`\b(according to|based on|as described in|refer to|see above|the previous|the following|given that|assuming|in context of|with respect to)\b`)
)

// Reasoning markers — signals that the prompt needs deep thinking.
var reasoningKeywords = []string{
	"prove", "proof", "derive", "derivation",
	"theorem", "lemma", "corollary",
	"step by step", "step-by-step", "chain of thought",
	"reason through", "think carefully",
	"why does", "why would", "why is",
	"mathematical", "equation", "formula",
	"logic", "logical", "deduce", "infer",
	"contradict", "contradiction", "paradox",
	"recursive", "recursion", "induction",
	"optimality", "optimal", "minimize", "maximize",
	"complexity analysis", "big-o", "time complexity",
	"formal", "formally", "rigorously",
}

func scoreReasoningMarkers(lower string) float64 {
	count := 0
	for _, kw := range reasoningKeywords {
		if strings.Contains(lower, kw) {
			count++
		}
	}
	// Saturates at ~5 matches → 1.0
	return clamp01(float64(count) / 5.0)
}

// Code presence — detects code blocks, inline code, programming terms.
var codeKeywords = []string{
	"function", "func ", "def ", "class ", "import ",
	"return ", "if ", "for ", "while ", "var ",
	"const ", "let ", "package ", "struct ",
	"interface ", "enum ", "switch ", "case ",
	"try ", "catch ", "throw ", "async ", "await ",
	"select ", "from ", "where ", "join ",
	"docker", "kubernetes", "api ", "http",
	"golang", "python", "javascript", "typescript", "rust",
}

func scoreCodePresence(lower string) float64 {
	score := 0.0

	// Code fences are strong signal
	fences := len(reCodeFence.FindAllString(lower, -1)) / 2 // pairs
	score += float64(fences) * 0.4

	// Inline code
	inlines := len(reInlineCode.FindAllString(lower, -1))
	score += float64(inlines) * 0.1

	// Programming keywords
	kwCount := 0
	for _, kw := range codeKeywords {
		if strings.Contains(lower, kw) {
			kwCount++
		}
	}
	score += float64(kwCount) * 0.08

	return clamp01(score)
}

// Simple indicators — signals that the prompt is trivially simple.
// NOTE: patterns are matched with word-boundary awareness via containsWord.
var simplePatterns = []string{
	"hello", "hi", "hey", "thanks", "thank you",
	"good morning", "good night", "bye", "goodbye",
	"how are you", "what's up", "whats up",
}

func scoreSimpleIndicators(lower string, words []string) float64 {
	score := 0.0

	// Greetings/simple patterns — only match as whole words at start/end
	// to avoid false positives (e.g. "hi" inside "architecture")
	for _, pat := range simplePatterns {
		if containsWord(lower, pat) {
			score += 0.3
		}
	}

	// Very short prompts (< 10 words) are likely simple
	if len(words) <= 5 {
		score += 0.5
	} else if len(words) <= 10 {
		score += 0.2
	}

	// Simple questions (what is X, who is Y)
	if reSimpleQuestion.MatchString(lower) && len(words) < 15 {
		score += 0.3
	}

	return clamp01(score)
}

// containsWord checks if a word/phrase appears as a whole word (not as a substring
// of another word). Uses simple boundary checking.
func containsWord(s, word string) bool {
	idx := 0
	for {
		pos := strings.Index(s[idx:], word)
		if pos == -1 {
			return false
		}
		absPos := idx + pos
		endPos := absPos + len(word)

		// Check left boundary
		leftOK := absPos == 0 || !isWordChar(s[absPos-1])
		// Check right boundary
		rightOK := endPos >= len(s) || !isWordChar(s[endPos])

		if leftOK && rightOK {
			return true
		}

		idx = absPos + 1
		if idx >= len(s) {
			return false
		}
	}
}

func isWordChar(b byte) bool {
	return (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b == '_'
}

// Multi-step patterns — prompts requiring sequential processing.
func scoreMultiStepPatterns(lower string) float64 {
	score := 0.0

	// Explicit step references
	steps := len(reStepPatterns.FindAllString(lower, -1))
	score += float64(steps) * 0.2

	// Numbered lists
	numbered := len(reNumberedList.FindAllString(lower, -1))
	score += float64(numbered) * 0.15

	// Bullet lists (less strong)
	bullets := len(reBulletList.FindAllString(lower, -1))
	score += float64(bullets) * 0.08

	// Multi-sentence prompts with connectives
	connectives := 0
	for _, c := range []string{"then ", "after that", "once ", "followed by", "subsequently", "afterward"} {
		if strings.Contains(lower, c) {
			connectives++
		}
	}
	score += float64(connectives) * 0.15

	return clamp01(score)
}

// Technical terms — domain-specific vocabulary density.
var technicalTerms = []string{
	"algorithm", "data structure", "database", "schema",
	"architecture", "microservice", "distributed", "concurrent",
	"authentication", "authorization", "encryption", "hashing",
	"latency", "throughput", "scalability", "availability",
	"regression", "classification", "neural network", "transformer",
	"gradient", "backpropagation", "embedding", "tokenization",
	"protocol", "tcp", "udp", "websocket", "grpc",
	"container", "orchestration", "pipeline", "ci/cd",
	"mutex", "semaphore", "deadlock", "race condition",
	"polymorphism", "inheritance", "encapsulation", "abstraction",
	"eigenvalue", "matrix", "vector", "tensor",
	"differential", "integral", "probability", "statistics",
	"quantum", "entropy", "bayesian", "stochastic",
}

func scoreTechnicalTerms(lower string, words []string) float64 {
	count := 0
	for _, term := range technicalTerms {
		if strings.Contains(lower, term) {
			count++
		}
	}
	// Normalise: 8+ terms → 1.0
	return clamp01(float64(count) / 8.0)
}

// Token count — longer prompts tend to be more complex.
func scoreTokenCount(prompt string) float64 {
	// Approximate tokens as characters/4 (rough English average)
	chars := utf8.RuneCountInString(prompt)
	tokens := float64(chars) / 4.0

	// Sigmoid-ish mapping: ~50 tokens → 0, ~500 → 0.5, ~2000 → 1.0
	if tokens < 50 {
		return 0
	}
	return clamp01((tokens - 50) / 1950)
}

// Creative markers — prompts requiring creative generation.
func scoreCreativeMarkers(lower string) float64 {
	matches := len(reCreative.FindAllString(lower, -1))
	return clamp01(float64(matches) / 3.0)
}

// Question complexity — depth of the question being asked.
func scoreQuestionComplexity(lower string) float64 {
	score := 0.0

	// Deep question words
	deep := len(reQuestionWords.FindAllString(lower, -1))
	score += float64(deep) * 0.25

	// Multiple questions (multiple '?')
	qmarks := strings.Count(lower, "?")
	if qmarks > 1 {
		score += float64(qmarks-1) * 0.15
	}

	// Conditional reasoning
	for _, cond := range []string{"what if", "assuming", "suppose", "given that", "in the case"} {
		if strings.Contains(lower, cond) {
			score += 0.2
		}
	}

	return clamp01(score)
}

// Constraint count — explicit constraints the answer must satisfy.
func scoreConstraintCount(lower string) float64 {
	matches := len(reConstraintWords.FindAllString(lower, -1))
	// 5+ constraints → 1.0
	return clamp01(float64(matches) / 5.0)
}

// Imperative verbs — action-oriented complexity.
func scoreImperativeVerbs(lower string, words []string) float64 {
	matches := len(reImperative.FindAllString(lower, -1))
	return clamp01(float64(matches) / 4.0)
}

// Output format — structured output requirements.
func scoreOutputFormat(lower string) float64 {
	matches := len(reOutputFormat.FindAllString(lower, -1))
	return clamp01(float64(matches) / 3.0)
}

// Domain specificity — how specialised the topic is.
var domainTerms = map[string][]string{
	"finance":    {"trading", "portfolio", "hedge", "derivative", "option", "futures", "yield", "arbitrage", "liquidity", "volatility"},
	"medicine":   {"diagnosis", "symptom", "treatment", "pathology", "pharmacology", "clinical", "dosage", "prognosis"},
	"law":        {"statute", "precedent", "jurisdiction", "liability", "plaintiff", "defendant", "tort", "contractual"},
	"science":    {"hypothesis", "experiment", "observation", "peer review", "control group", "variable", "methodology"},
	"ml":         {"training", "inference", "loss function", "epoch", "batch size", "learning rate", "overfitting", "regularization"},
}

func scoreDomainSpecificity(lower string) float64 {
	maxDomainScore := 0.0
	for _, terms := range domainTerms {
		count := 0
		for _, t := range terms {
			if strings.Contains(lower, t) {
				count++
			}
		}
		ds := float64(count) / float64(len(terms))
		if ds > maxDomainScore {
			maxDomainScore = ds
		}
	}
	return clamp01(maxDomainScore * 2.0) // 50% of a domain's terms → 1.0
}

// Reference complexity — how much external context is referenced.
func scoreReferenceComplexity(lower string) float64 {
	matches := len(reReference.FindAllString(lower, -1))
	return clamp01(float64(matches) / 3.0)
}

// Negation complexity — negative constraints add reasoning difficulty.
func scoreNegationComplexity(lower string) float64 {
	matches := len(reNegationComplexity.FindAllString(lower, -1))
	// 4+ negations → 1.0
	return clamp01(float64(matches) / 4.0)
}
