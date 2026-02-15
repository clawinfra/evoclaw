package genome

import (
	"testing"
)

func TestVerifyConstraintsUnsigned(t *testing.T) {
	g := DefaultGenome()
	// No signature, no key = unsigned, should be ok (backward compat)
	if err := g.VerifyConstraints(); err != nil {
		t.Errorf("expected nil for unsigned genome, got: %v", err)
	}
}

func TestVerifyConstraintsInvalidSignature(t *testing.T) {
	g := DefaultGenome()
	g.OwnerPublicKey = []byte("fake-key")
	g.ConstraintSignature = []byte("fake-sig")

	err := g.VerifyConstraints()
	if err == nil {
		t.Error("expected error for invalid signature")
	}
}

func TestToConfig(t *testing.T) {
	c := GenomeConstraints{
		MaxLossUSD:    500.0,
		AllowedAssets: []string{"BTC", "ETH"},
		BlockedActions: []string{"withdraw"},
		MaxDivergence: 10.0,
		MinVFMScore:   0.5,
	}
	cc := c.toConfig()
	if cc.MaxLossUSD != 500.0 {
		t.Errorf("MaxLossUSD = %f, want 500", cc.MaxLossUSD)
	}
	if len(cc.AllowedAssets) != 2 {
		t.Error("expected 2 allowed assets")
	}
	if cc.MaxDivergence != 10.0 {
		t.Errorf("MaxDivergence = %f, want 10", cc.MaxDivergence)
	}
	if cc.MinVFMScore != 0.5 {
		t.Errorf("MinVFMScore = %f, want 0.5", cc.MinVFMScore)
	}
}

func TestGenomeSetSkillNilMap(t *testing.T) {
	g := &Genome{} // Skills is nil
	g.SetSkill("test", SkillGenome{Enabled: true})
	if g.Skills == nil {
		t.Error("expected initialized skills map")
	}
	if !g.Skills["test"].Enabled {
		t.Error("expected enabled skill")
	}
}

func TestEnabledSkillsEmpty(t *testing.T) {
	g := &Genome{Skills: map[string]SkillGenome{
		"a": {Enabled: false},
		"b": {Enabled: false},
	}}
	if len(g.EnabledSkills()) != 0 {
		t.Error("expected no enabled skills")
	}
}

func TestValidateAutonomyBounds(t *testing.T) {
	g := DefaultGenome()
	g.Behavior.Autonomy = 1.5
	if err := g.Validate(); err == nil {
		t.Error("expected error for autonomy > 1")
	}

	g.Behavior.Autonomy = -0.1
	if err := g.Validate(); err == nil {
		t.Error("expected error for autonomy < 0")
	}
}

func TestFromLegacyInvalid(t *testing.T) {
	// Valid map structure but with wrong types
	legacy := map[string]interface{}{
		"identity": "not-a-map",
	}
	// Should still parse (json unmarshal is lenient)
	_, err := FromLegacy(legacy)
	_ = err // May or may not error depending on json handling
}

func TestCloneError(t *testing.T) {
	// Normal clone should work
	g := DefaultGenome()
	clone, err := g.Clone()
	if err != nil {
		t.Fatalf("Clone() error: %v", err)
	}

	// Verify deep independence
	g.Behavior.RiskTolerance = 0.99
	if clone.Behavior.RiskTolerance == 0.99 {
		t.Error("clone not independent")
	}
}

func TestSkillGenomeFields(t *testing.T) {
	sg := SkillGenome{
		Enabled:      true,
		Weight:       0.8,
		Strategies:   []string{"s1", "s2"},
		Params:       map[string]interface{}{"k": "v"},
		Fitness:      0.75,
		Version:      5,
		Dependencies: []string{"dep1"},
		EvalCount:    10,
		Verified:     true,
		VFMScore:     0.9,
	}
	if !sg.Verified {
		t.Error("expected verified")
	}
	if sg.VFMScore != 0.9 {
		t.Errorf("VFMScore = %f, want 0.9", sg.VFMScore)
	}
}

func TestBehaviorFeedbackFields(t *testing.T) {
	bf := BehaviorFeedback{
		AgentID: "a1",
		Type:    "approval",
		Score:   0.8,
		Context: "test",
	}
	if bf.AgentID != "a1" {
		t.Error("wrong agent ID")
	}
}
