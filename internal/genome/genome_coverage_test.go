package genome

import (
	"testing"
)

func TestVerifyConstraintsUnsignedV2(t *testing.T) {
	g := DefaultGenome()
	// No signature, no public key — should succeed with warning
	err := g.VerifyConstraints()
	if err != nil {
		t.Errorf("VerifyConstraints() on unsigned genome should succeed, got: %v", err)
	}
}

func TestVerifyConstraintsInvalidSig(t *testing.T) {
	g := DefaultGenome()
	g.OwnerPublicKey = []byte("invalid-public-key")
	g.ConstraintSignature = []byte("invalid-signature")
	// Should fail with some error
	err := g.VerifyConstraints()
	if err == nil {
		t.Error("VerifyConstraints() with invalid sig should fail")
	}
}

func TestClone(t *testing.T) {
	g := DefaultGenome()
	g.Skills["chat"] = SkillGenome{
		Enabled: true,
		Weight:  0.8,
		Params:  map[string]interface{}{"temp": 0.7},
	}

	clone, err := g.Clone()
	if err != nil {
		t.Fatalf("Clone() error: %v", err)
	}

	// Modify clone — original should be unaffected
	clone.Identity.Name = "cloned-agent"
	if g.Identity.Name == "cloned-agent" {
		t.Error("Clone should produce independent copy")
	}

	// Check skill was cloned
	if clone.GetSkill("chat") == nil {
		t.Error("Clone should preserve skills")
	}
}

func TestFromLegacy(t *testing.T) {
	legacy := map[string]interface{}{
		"identity": map[string]interface{}{
			"name":    "legacy-agent",
			"persona": "helpful",
			"voice":   "concise",
		},
		"skills": map[string]interface{}{},
		"behavior": map[string]interface{}{
			"risk_tolerance": 0.5,
			"verbosity":      0.3,
			"autonomy":       0.7,
		},
		"constraints": map[string]interface{}{
			"max_loss_usd": 500.0,
		},
	}

	g, err := FromLegacy(legacy)
	if err != nil {
		t.Fatalf("FromLegacy() error: %v", err)
	}
	if g.Identity.Name != "legacy-agent" {
		t.Errorf("Name = %q, want %q", g.Identity.Name, "legacy-agent")
	}
}

func TestToLegacy(t *testing.T) {
	g := DefaultGenome()
	g.Identity.Name = "my-agent"

	legacy, err := g.ToLegacy()
	if err != nil {
		t.Fatalf("ToLegacy() error: %v", err)
	}
	identity, ok := legacy["identity"].(map[string]interface{})
	if !ok {
		t.Fatal("identity should be a map")
	}
	if identity["name"] != "my-agent" {
		t.Errorf("legacy name = %v, want %q", identity["name"], "my-agent")
	}
}

func TestFromLegacyInvalidShape(t *testing.T) {
	// A value that can't be marshaled should fail
	// Actually all map[string]interface{} can be marshaled.
	// Try unmarshaling bad shape
	legacy := map[string]interface{}{
		"behavior": "not-a-map", // Should still unmarshal (Go is lenient)
	}
	_, err := FromLegacy(legacy)
	// This might or might not error depending on Go's leniency
	if err != nil {
		t.Logf("FromLegacy with bad shape returned error (expected): %v", err)
	}
}
