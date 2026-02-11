package genome

import (
	"encoding/json"
	"testing"
)

func TestDefaultGenome(t *testing.T) {
	g := DefaultGenome()
	
	if err := g.Validate(); err != nil {
		t.Errorf("default genome validation failed: %v", err)
	}
	
	if g.Identity.Name == "" {
		t.Error("identity name should not be empty")
	}
	
	if g.Skills == nil {
		t.Error("skills map should be initialized")
	}
}

func TestGenomeValidation(t *testing.T) {
	tests := []struct {
		name      string
		genome    *Genome
		wantError bool
	}{
		{
			name:      "valid genome",
			genome:    DefaultGenome(),
			wantError: false,
		},
		{
			name: "invalid risk tolerance (too high)",
			genome: &Genome{
				Behavior: GenomeBehavior{
					RiskTolerance: 1.5,
					Verbosity:     0.5,
					Autonomy:      0.5,
				},
				Identity:    GenomeIdentity{Name: "test"},
				Skills:      make(map[string]SkillGenome),
				Constraints: GenomeConstraints{},
			},
			wantError: true,
		},
		{
			name: "invalid verbosity (negative)",
			genome: &Genome{
				Behavior: GenomeBehavior{
					RiskTolerance: 0.5,
					Verbosity:     -0.1,
					Autonomy:      0.5,
				},
				Identity:    GenomeIdentity{Name: "test"},
				Skills:      make(map[string]SkillGenome),
				Constraints: GenomeConstraints{},
			},
			wantError: true,
		},
		{
			name: "negative max loss",
			genome: &Genome{
				Behavior: GenomeBehavior{
					RiskTolerance: 0.5,
					Verbosity:     0.5,
					Autonomy:      0.5,
				},
				Identity: GenomeIdentity{Name: "test"},
				Skills:   make(map[string]SkillGenome),
				Constraints: GenomeConstraints{
					MaxLossUSD: -100,
				},
			},
			wantError: true,
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.genome.Validate()
			if (err != nil) != tt.wantError {
				t.Errorf("Validate() error = %v, wantError %v", err, tt.wantError)
			}
		})
	}
}

func TestGenomeClone(t *testing.T) {
	original := DefaultGenome()
	original.Identity.Name = "test-agent"
	original.Skills["trading"] = SkillGenome{
		Enabled: true,
		Params: map[string]interface{}{
			"threshold": -0.1,
		},
		Fitness: 0.75,
		Version: 3,
	}
	
	clone, err := original.Clone()
	if err != nil {
		t.Fatalf("Clone() failed: %v", err)
	}
	
	// Verify clone is independent
	clone.Identity.Name = "cloned-agent"
	if original.Identity.Name == clone.Identity.Name {
		t.Error("clone modified original")
	}
	
	// Verify skills are cloned
	if len(clone.Skills) != len(original.Skills) {
		t.Error("skills not properly cloned")
	}
}

func TestSkillOperations(t *testing.T) {
	g := DefaultGenome()
	
	// Test SetSkill
	skill := SkillGenome{
		Enabled: true,
		Params: map[string]interface{}{
			"threshold": -0.1,
		},
		Fitness: 0.5,
		Version: 1,
	}
	g.SetSkill("trading", skill)
	
	// Test GetSkill
	retrieved := g.GetSkill("trading")
	if retrieved == nil {
		t.Fatal("GetSkill() returned nil for existing skill")
	}
	if !retrieved.Enabled {
		t.Error("skill should be enabled")
	}
	
	// Test GetSkill for non-existent skill
	if g.GetSkill("nonexistent") != nil {
		t.Error("GetSkill() should return nil for non-existent skill")
	}
	
	// Test EnabledSkills
	g.SetSkill("monitoring", SkillGenome{Enabled: false})
	enabled := g.EnabledSkills()
	if len(enabled) != 1 {
		t.Errorf("expected 1 enabled skill, got %d", len(enabled))
	}
}

func TestJSONSerialization(t *testing.T) {
	g := DefaultGenome()
	g.Skills["trading"] = SkillGenome{
		Enabled:    true,
		Strategies: []string{"FundingArbitrage"},
		Params: map[string]interface{}{
			"threshold":      -0.1,
			"position_size":  100.0,
			"max_positions":  3.0,
		},
		Fitness: 0.75,
		Version: 2,
	}
	
	// Marshal to JSON
	data, err := json.Marshal(g)
	if err != nil {
		t.Fatalf("json.Marshal() failed: %v", err)
	}
	
	// Unmarshal back
	var decoded Genome
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("json.Unmarshal() failed: %v", err)
	}
	
	// Verify
	tradingSkill := decoded.GetSkill("trading")
	if tradingSkill == nil {
		t.Fatal("trading skill not found after unmarshal")
	}
	if tradingSkill.Fitness != 0.75 {
		t.Errorf("fitness = %f, want 0.75", tradingSkill.Fitness)
	}
	if tradingSkill.Version != 2 {
		t.Errorf("version = %d, want 2", tradingSkill.Version)
	}
}

func TestLegacyConversion(t *testing.T) {
	legacy := map[string]interface{}{
		"identity": map[string]interface{}{
			"name":    "legacy-agent",
			"persona": "test persona",
			"voice":   "verbose",
		},
		"skills": map[string]interface{}{
			"trading": map[string]interface{}{
				"enabled": true,
				"params": map[string]interface{}{
					"threshold": -0.1,
				},
				"fitness": 0.8,
				"version": 1.0,
			},
		},
		"behavior": map[string]interface{}{
			"risk_tolerance": 0.5,
			"verbosity":      0.7,
			"autonomy":       0.6,
		},
		"constraints": map[string]interface{}{
			"max_loss_usd": 1000.0,
		},
	}
	
	// Convert from legacy
	g, err := FromLegacy(legacy)
	if err != nil {
		t.Fatalf("FromLegacy() failed: %v", err)
	}
	
	// Verify conversion
	if g.Identity.Name != "legacy-agent" {
		t.Errorf("identity name = %s, want legacy-agent", g.Identity.Name)
	}
	
	skill := g.GetSkill("trading")
	if skill == nil {
		t.Fatal("trading skill not found")
	}
	if !skill.Enabled {
		t.Error("trading skill should be enabled")
	}
	
	// Convert back to legacy
	converted, err := g.ToLegacy()
	if err != nil {
		t.Fatalf("ToLegacy() failed: %v", err)
	}
	
	// Verify round-trip
	identity, ok := converted["identity"].(map[string]interface{})
	if !ok {
		t.Fatal("identity not found in converted legacy")
	}
	if identity["name"] != "legacy-agent" {
		t.Error("identity name lost in round-trip")
	}
}
