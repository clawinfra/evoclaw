package onchain

import (
	"testing"
)

func TestPresetsNotEmpty(t *testing.T) {
	if len(Presets) == 0 {
		t.Fatal("Presets map is empty")
	}
}

func TestAllRequiredPresetsExist(t *testing.T) {
	required := []string{
		"bsc", "bsc-testnet",
		"eth", "arbitrum", "base",
		"opbnb", "polygon",
		"clawchain",
	}
	for _, id := range required {
		p, ok := GetPreset(id)
		if !ok {
			t.Errorf("preset %q not found", id)
			continue
		}
		if p.ID == "" {
			t.Errorf("preset %q has empty ID", id)
		}
		if p.Type == "" {
			t.Errorf("preset %q has empty Type", id)
		}
		if p.Name == "" {
			t.Errorf("preset %q has empty Name", id)
		}
		if p.RPC == "" {
			t.Errorf("preset %q has empty RPC", id)
		}
	}
}

func TestPresetTypes(t *testing.T) {
	evmChains := []string{"bsc", "bsc-testnet", "eth", "arbitrum", "base", "opbnb", "polygon"}
	for _, id := range evmChains {
		p, _ := GetPreset(id)
		if p.Type != "evm" {
			t.Errorf("preset %q type = %q, want evm", id, p.Type)
		}
	}

	p, _ := GetPreset("clawchain")
	if p.Type != "substrate" {
		t.Errorf("preset clawchain type = %q, want substrate", p.Type)
	}
}

func TestGetPresetUnknown(t *testing.T) {
	_, ok := GetPreset("nonexistent-chain-xyz")
	if ok {
		t.Error("expected false for unknown preset, got true")
	}
}

func TestPresetIDMatchesKey(t *testing.T) {
	for key, p := range Presets {
		if p.ID != key {
			t.Errorf("Presets[%q].ID = %q, want %q", key, p.ID, key)
		}
	}
}
