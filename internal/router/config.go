package router

// RouterConfig holds the configuration for the intelligent LLM router.
type RouterConfig struct {
	// Enabled toggles intelligent routing. When false, the default model is used.
	Enabled bool `json:"enabled"`

	// TierModels maps each tier to a "provider/model" string.
	TierModels map[Tier]string `json:"tierModels"`

	// TierCosts maps each tier to its approximate cost per million tokens (avg of input+output).
	// Used for savings estimation. If zero, defaults are used.
	TierCosts map[Tier]float64 `json:"tierCosts"`

	// DefaultTier is the tier used when routing is disabled or scoring is inconclusive.
	DefaultTier Tier `json:"defaultTier"`

	// Weights allows overriding the default dimension weights.
	// Keys are dimension names (e.g. "reasoning_markers"), values are weights.
	Weights map[string]float64 `json:"weights,omitempty"`

	// Thresholds defines the score boundaries for tier selection.
	// Thresholds[0] = simple/medium boundary (default 0.25)
	// Thresholds[1] = medium/complex boundary (default 0.50)
	// Thresholds[2] = complex/reasoning boundary (default 0.75)
	Thresholds [3]float64 `json:"thresholds"`

	// LogDecisions logs each routing decision at INFO level.
	LogDecisions bool `json:"logDecisions"`
}

// DefaultRouterConfig returns sensible defaults.
func DefaultRouterConfig() RouterConfig {
	return RouterConfig{
		Enabled: true,
		TierModels: map[Tier]string{
			TierSimple:    "deepseek/deepseek-chat",
			TierMedium:    "openai/gpt-4o-mini",
			TierComplex:   "anthropic/claude-sonnet-4-20250514",
			TierReasoning: "openai/o3",
		},
		TierCosts: map[Tier]float64{
			TierSimple:    0.27,
			TierMedium:    0.60,
			TierComplex:   15.0,
			TierReasoning: 10.0,
		},
		DefaultTier: TierComplex,
		Thresholds:  [3]float64{0.25, 0.50, 0.75},
		LogDecisions: true,
	}
}

// CostPerMillion returns the cost/M for a tier, falling back to defaults.
func (c *RouterConfig) CostPerMillion(tier Tier) float64 {
	if v, ok := c.TierCosts[tier]; ok && v > 0 {
		return v
	}
	defaults := map[Tier]float64{
		TierSimple:    0.27,
		TierMedium:    0.60,
		TierComplex:   15.0,
		TierReasoning: 10.0,
	}
	return defaults[tier]
}
