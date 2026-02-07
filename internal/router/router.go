package router

import (
	"fmt"
	"log/slog"
	"sync"
	"time"
)

// RoutingDecision captures the result of a routing decision for logging/analysis.
type RoutingDecision struct {
	Prompt     string      `json:"-"` // not serialised (may be large)
	Score      ScoreResult `json:"score"`
	Tier       Tier        `json:"tier"`
	Model      string      `json:"model"`
	Timestamp  time.Time   `json:"timestamp"`
	DurationUs int64       `json:"durationUs"` // scoring latency in microseconds
}

// CostSavings tracks estimated cost savings from intelligent routing.
type CostSavings struct {
	mu              sync.RWMutex
	TotalRequests   int64              `json:"totalRequests"`
	RequestsByTier  map[Tier]int64     `json:"requestsByTier"`
	EstimatedCost   float64            `json:"estimatedCost"`   // actual estimated cost with routing
	BaselineCost    float64            `json:"baselineCost"`    // cost if everything used the default (complex) tier
	SavedUSD        float64            `json:"savedUsd"`        // baseline - estimated
	SavingsPercent  float64            `json:"savingsPercent"`  // (saved / baseline) * 100
	AvgTokens       float64            `json:"avgTokens"`       // running average tokens per request
	totalTokens     int64
}

// Router performs intelligent model selection based on prompt complexity analysis.
type Router struct {
	cfg    RouterConfig
	scorer *Scorer
	logger *slog.Logger
	stats  *CostSavings
}

// New creates a Router with the given config.
func New(cfg RouterConfig, logger *slog.Logger) *Router {
	return &Router{
		cfg:    cfg,
		scorer: NewScorer(cfg.Weights),
		logger: logger.With("component", "intelligent-router"),
		stats: &CostSavings{
			RequestsByTier: make(map[Tier]int64),
		},
	}
}

// Route analyses the prompt and returns the best model + full decision.
func (r *Router) Route(prompt string) RoutingDecision {
	start := time.Now()

	if !r.cfg.Enabled {
		model := r.modelForTier(r.cfg.DefaultTier)
		return RoutingDecision{
			Prompt: prompt,
			Tier:   r.cfg.DefaultTier,
			Model:  model,
			Timestamp: start,
		}
	}

	// Score the prompt across all 14 dimensions
	result := r.scorer.Score(prompt)

	// Map normalised score to tier
	tier := SelectTier(result.Normalised, r.cfg.Thresholds)

	// Get model for this tier
	model := r.modelForTier(tier)

	elapsed := time.Since(start)

	decision := RoutingDecision{
		Prompt:     prompt,
		Score:      result,
		Tier:       tier,
		Model:      model,
		Timestamp:  start,
		DurationUs: elapsed.Microseconds(),
	}

	if r.cfg.LogDecisions {
		r.logger.Info("routing decision",
			"tier", tier.String(),
			"model", model,
			"normalised_score", fmt.Sprintf("%.3f", result.Normalised),
			"raw_score", fmt.Sprintf("%.3f", result.RawTotal),
			"duration_us", elapsed.Microseconds(),
			"prompt_len", len(prompt),
		)
	}

	return decision
}

// RouteAndTrack performs routing and also tracks cost savings.
// estimatedTokens is the estimated total tokens (input+output) for cost calculation.
func (r *Router) RouteAndTrack(prompt string, estimatedTokens int) RoutingDecision {
	decision := r.Route(prompt)

	r.stats.mu.Lock()
	defer r.stats.mu.Unlock()

	r.stats.TotalRequests++
	r.stats.RequestsByTier[decision.Tier]++
	r.stats.totalTokens += int64(estimatedTokens)
	r.stats.AvgTokens = float64(r.stats.totalTokens) / float64(r.stats.TotalRequests)

	tokens := float64(estimatedTokens)

	// Cost of this request at its routed tier
	actualCostPerM := r.cfg.CostPerMillion(decision.Tier)
	r.stats.EstimatedCost += tokens * actualCostPerM / 1_000_000

	// Cost if we had used the default/complex tier
	baselineCostPerM := r.cfg.CostPerMillion(r.cfg.DefaultTier)
	r.stats.BaselineCost += tokens * baselineCostPerM / 1_000_000

	// Update savings
	r.stats.SavedUSD = r.stats.BaselineCost - r.stats.EstimatedCost
	if r.stats.BaselineCost > 0 {
		r.stats.SavingsPercent = (r.stats.SavedUSD / r.stats.BaselineCost) * 100
	}

	return decision
}

// GetSavings returns a snapshot of cost savings.
func (r *Router) GetSavings() CostSavings {
	r.stats.mu.RLock()
	defer r.stats.mu.RUnlock()

	// Return a copy
	tiersCopy := make(map[Tier]int64, len(r.stats.RequestsByTier))
	for k, v := range r.stats.RequestsByTier {
		tiersCopy[k] = v
	}

	return CostSavings{
		TotalRequests:  r.stats.TotalRequests,
		RequestsByTier: tiersCopy,
		EstimatedCost:  r.stats.EstimatedCost,
		BaselineCost:   r.stats.BaselineCost,
		SavedUSD:       r.stats.SavedUSD,
		SavingsPercent: r.stats.SavingsPercent,
		AvgTokens:      r.stats.AvgTokens,
	}
}

// SavingsReport returns a human-readable cost savings report.
func (r *Router) SavingsReport() string {
	s := r.GetSavings()

	report := fmt.Sprintf("=== LLM Router Cost Report ===\n")
	report += fmt.Sprintf("Total Requests:    %d\n", s.TotalRequests)
	report += fmt.Sprintf("Baseline Cost:     $%.4f (all %s)\n", s.BaselineCost, r.cfg.DefaultTier)
	report += fmt.Sprintf("Routed Cost:       $%.4f\n", s.EstimatedCost)
	report += fmt.Sprintf("Saved:             $%.4f (%.1f%%)\n", s.SavedUSD, s.SavingsPercent)
	report += fmt.Sprintf("Avg Tokens/Req:    %.0f\n", s.AvgTokens)
	report += fmt.Sprintf("\nTier Distribution:\n")
	for _, tier := range []Tier{TierSimple, TierMedium, TierComplex, TierReasoning} {
		count := s.RequestsByTier[tier]
		pct := 0.0
		if s.TotalRequests > 0 {
			pct = float64(count) / float64(s.TotalRequests) * 100
		}
		model := r.cfg.TierModels[tier]
		report += fmt.Sprintf("  %-10s %5d (%5.1f%%)  → %s\n", tier, count, pct, model)
	}
	return report
}

// Config returns the current router config (read-only snapshot).
func (r *Router) Config() RouterConfig {
	return r.cfg
}

// modelForTier resolves a tier to a model string.
func (r *Router) modelForTier(tier Tier) string {
	if model, ok := r.cfg.TierModels[tier]; ok {
		return model
	}
	// Fallback: try default tier, then hard fallback
	if model, ok := r.cfg.TierModels[r.cfg.DefaultTier]; ok {
		return model
	}
	return "anthropic/claude-sonnet-4-20250514"
}
