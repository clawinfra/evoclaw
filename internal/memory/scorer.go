package memory

import (
	"math"
	"time"
)

// MemoryTier represents the tier a memory belongs to based on its score
type MemoryTier int

const (
	TierFrozen MemoryTier = iota // score < 0.05 - excluded from search
	TierCold                      // score >= 0.05 - archived, queryable
	TierWarm                      // score >= 0.3 - on-device, indexed
	TierHot                       // score >= 0.7 - core memory, always in context
)

// ScoreConfig holds parameters for relevance scoring
type ScoreConfig struct {
	// Half-life for recency decay in days (default: 30)
	HalfLifeDays float64
	// Reinforcement boost per access (default: 0.1)
	ReinforcementBoost float64
}

// DefaultScoreConfig returns default scoring parameters
func DefaultScoreConfig() ScoreConfig {
	return ScoreConfig{
		HalfLifeDays:       30.0,
		ReinforcementBoost: 0.1,
	}
}

// CalculateScore computes the relevance score for a memory entry
//
// score = importance × recency_decay(age) × (1 + boost × access_count)
//
// Where:
//   - importance: base value (0-1) set at creation
//   - recency_decay: exp(-age_days / half_life)
//   - reinforcement: 1 + (boost × access_count)
func CalculateScore(importance float64, createdAt time.Time, accessCount int, cfg ScoreConfig) float64 {
	if cfg.HalfLifeDays <= 0 {
		cfg.HalfLifeDays = 30.0
	}
	if cfg.ReinforcementBoost < 0 {
		cfg.ReinforcementBoost = 0.1
	}

	// Calculate age in days
	age := time.Since(createdAt).Hours() / 24.0

	// Recency decay: exp(-age / half_life)
	recency := math.Exp(-age / cfg.HalfLifeDays)

	// Reinforcement: 1 + (boost × access_count)
	reinforcement := 1.0 + (cfg.ReinforcementBoost * float64(accessCount))

	return importance * recency * reinforcement
}

// CalculateTier determines which tier a memory belongs to based on its score
func CalculateTier(score float64) MemoryTier {
	if score >= 0.7 {
		return TierHot
	}
	if score >= 0.3 {
		return TierWarm
	}
	if score >= 0.05 {
		return TierCold
	}
	return TierFrozen
}

// RecencyDecay calculates just the recency component
func RecencyDecay(age time.Duration, halfLifeDays float64) float64 {
	if halfLifeDays <= 0 {
		halfLifeDays = 30.0
	}
	ageDays := age.Hours() / 24.0
	return math.Exp(-ageDays / halfLifeDays)
}

// ReinforcementFactor calculates just the reinforcement component
func ReinforcementFactor(accessCount int, boost float64) float64 {
	if boost < 0 {
		boost = 0.1
	}
	return 1.0 + (boost * float64(accessCount))
}

// TierThresholds returns the score thresholds for each tier
func TierThresholds() map[MemoryTier]float64 {
	return map[MemoryTier]float64{
		TierHot:    0.7,
		TierWarm:   0.3,
		TierCold:   0.05,
		TierFrozen: 0.0,
	}
}

// ShouldEvictFromWarm determines if a memory should be evicted from warm tier
func ShouldEvictFromWarm(score float64, age time.Duration, retentionDays int, threshold float64) bool {
	// Evict if score below threshold OR age exceeds retention
	ageDays := int(age.Hours() / 24.0)
	return score < threshold || ageDays > retentionDays
}

// ShouldDeleteFromCold determines if a frozen memory should be deleted
func ShouldDeleteFromCold(score float64, age time.Duration, retentionYears int) bool {
	// Only delete frozen entries older than retention period
	if score >= 0.05 {
		return false
	}
	ageYears := age.Hours() / (24.0 * 365.25)
	return ageYears > float64(retentionYears)
}
