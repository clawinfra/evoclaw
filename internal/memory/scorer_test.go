package memory

import (
	"math"
	"testing"
	"time"
)

func TestCalculateScore(t *testing.T) {
	cfg := DefaultScoreConfig()

	tests := []struct {
		name        string
		importance  float64
		ageDays     int
		accessCount int
		wantTier    MemoryTier
	}{
		{
			name:        "fresh high importance",
			importance:  0.9,
			ageDays:     1,
			accessCount: 0,
			wantTier:    TierHot, // score ~0.88
		},
		{
			name:        "week old medium importance",
			importance:  0.6,
			ageDays:     7,
			accessCount: 2,
			wantTier:    TierWarm, // score ~0.57
		},
		{
			name:        "month old low importance",
			importance:  0.4,
			ageDays:     30,
			accessCount: 0,
			wantTier:    TierCold, // score ~0.15
		},
		{
			name:        "old very low importance",
			importance:  0.1,
			ageDays:     90,
			accessCount: 0,
			wantTier:    TierFrozen, // score ~0.005
		},
		{
			name:        "old but frequently accessed",
			importance:  0.5,
			ageDays:     60,
			accessCount: 10,
			wantTier:    TierWarm, // reinforcement keeps it warm
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			createdAt := time.Now().AddDate(0, 0, -tt.ageDays)
			score := CalculateScore(tt.importance, createdAt, tt.accessCount, cfg)
			tier := CalculateTier(score)

			if tier != tt.wantTier {
				t.Errorf("got tier %v, want %v (score: %.4f)", tier, tt.wantTier, score)
			}
		})
	}
}

func TestRecencyDecay(t *testing.T) {
	cfg := DefaultScoreConfig()

	tests := []struct {
		name     string
		ageDays  int
		wantMin  float64
		wantMax  float64
	}{
		{"1 day", 1, 0.97, 0.98},
		{"7 days", 7, 0.78, 0.80},
		{"30 days", 30, 0.36, 0.38},
		{"90 days", 90, 0.04, 0.06},
		{"365 days", 365, 0.0, 0.00001},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			age := time.Duration(tt.ageDays) * 24 * time.Hour
			decay := RecencyDecay(age, cfg.HalfLifeDays)

			if decay < tt.wantMin || decay > tt.wantMax {
				t.Errorf("decay for %d days: got %.4f, want between %.4f and %.4f",
					tt.ageDays, decay, tt.wantMin, tt.wantMax)
			}
		})
	}
}

func TestReinforcementFactor(t *testing.T) {
	boost := 0.1

	tests := []struct {
		accessCount int
		want        float64
	}{
		{0, 1.0},
		{1, 1.1},
		{5, 1.5},
		{10, 2.0},
		{20, 3.0},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			factor := ReinforcementFactor(tt.accessCount, boost)
			if math.Abs(factor-tt.want) > 0.01 {
				t.Errorf("access_count=%d: got %.2f, want %.2f",
					tt.accessCount, factor, tt.want)
			}
		})
	}
}

func TestCalculateTier(t *testing.T) {
	tests := []struct {
		score float64
		want  MemoryTier
	}{
		{0.95, TierHot},
		{0.70, TierHot},
		{0.69, TierWarm},
		{0.50, TierWarm},
		{0.30, TierWarm},
		{0.29, TierCold},
		{0.10, TierCold},
		{0.05, TierCold},
		{0.04, TierFrozen},
		{0.01, TierFrozen},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			got := CalculateTier(tt.score)
			if got != tt.want {
				t.Errorf("score %.2f: got %v, want %v", tt.score, got, tt.want)
			}
		})
	}
}

func TestShouldEvictFromWarm(t *testing.T) {
	tests := []struct {
		name          string
		score         float64
		ageDays       int
		retentionDays int
		threshold     float64
		want          bool
	}{
		{"high score, recent", 0.8, 5, 30, 0.3, false},
		{"low score", 0.2, 5, 30, 0.3, true},
		{"old age", 0.5, 35, 30, 0.3, true},
		{"edge case - exactly retention", 0.5, 30, 30, 0.3, false},
		{"edge case - exactly threshold", 0.3, 10, 30, 0.3, false},
		{"both conditions", 0.2, 35, 30, 0.3, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			age := time.Duration(tt.ageDays) * 24 * time.Hour
			got := ShouldEvictFromWarm(tt.score, age, tt.retentionDays, tt.threshold)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestShouldDeleteFromCold(t *testing.T) {
	tests := []struct {
		name           string
		score          float64
		ageYears       int
		retentionYears int
		want           bool
	}{
		{"frozen and old", 0.03, 15, 10, true},
		{"frozen but recent", 0.03, 5, 10, false},
		{"not frozen", 0.1, 15, 10, false},
		{"active memory", 0.5, 15, 10, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			age := time.Duration(tt.ageYears) * 365 * 24 * time.Hour
			got := ShouldDeleteFromCold(tt.score, age, tt.retentionYears)
			if got != tt.want {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestTierThresholds(t *testing.T) {
	thresholds := TierThresholds()

	if thresholds[TierHot] != 0.7 {
		t.Errorf("hot threshold: got %.2f, want 0.7", thresholds[TierHot])
	}
	if thresholds[TierWarm] != 0.3 {
		t.Errorf("warm threshold: got %.2f, want 0.3", thresholds[TierWarm])
	}
	if thresholds[TierCold] != 0.05 {
		t.Errorf("cold threshold: got %.2f, want 0.05", thresholds[TierCold])
	}
}
