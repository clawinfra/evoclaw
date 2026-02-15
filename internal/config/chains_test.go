package config

import (
	"testing"
)

func TestGetChainPreset(t *testing.T) {
	tests := []struct {
		name      string
		chainID   string
		wantFound bool
		wantType  string
	}{
		{
			name:      "BSC mainnet",
			chainID:   "bsc",
			wantFound: true,
			wantType:  "evm",
		},
		{
			name:      "BSC testnet",
			chainID:   "bsc-testnet",
			wantFound: true,
			wantType:  "evm",
		},
		{
			name:      "Hyperliquid",
			chainID:   "hyperliquid",
			wantFound: true,
			wantType:  "hyperliquid",
		},
		{
			name:      "Unknown chain",
			chainID:   "unknown-chain",
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			preset, found := GetChainPreset(tt.chainID)
			if found != tt.wantFound {
				t.Errorf("GetChainPreset(%q) found = %v, want %v", tt.chainID, found, tt.wantFound)
			}
			if found && preset.Type != tt.wantType {
				t.Errorf("GetChainPreset(%q).Type = %v, want %v", tt.chainID, preset.Type, tt.wantType)
			}
		})
	}
}

func TestAddChain(t *testing.T) {
	cfg := DefaultConfig()

	chain := ChainConfig{
		Enabled: true,
		Type:    "evm",
		Name:    "Test Chain",
		RPCURL:  "https://test.example.com",
		ChainID: 123,
		Wallet:  "0x123",
	}

	cfg.AddChain("test-chain", chain)

	if len(cfg.Chains) != 1 {
		t.Errorf("Expected 1 chain, got %d", len(cfg.Chains))
	}

	retrieved, ok := cfg.GetChain("test-chain")
	if !ok {
		t.Fatal("Chain not found after adding")
	}

	if retrieved.Name != "Test Chain" {
		t.Errorf("Chain name = %q, want %q", retrieved.Name, "Test Chain")
	}
}

func TestRemoveChain(t *testing.T) {
	cfg := DefaultConfig()

	chain := ChainConfig{
		Enabled: true,
		Type:    "evm",
		Name:    "Test Chain",
		RPCURL:  "https://test.example.com",
	}

	cfg.AddChain("test-chain", chain)

	// Remove existing chain
	err := cfg.RemoveChain("test-chain")
	if err != nil {
		t.Errorf("RemoveChain failed: %v", err)
	}

	if len(cfg.Chains) != 0 {
		t.Errorf("Expected 0 chains after removal, got %d", len(cfg.Chains))
	}

	// Try to remove non-existent chain
	err = cfg.RemoveChain("non-existent")
	if err == nil {
		t.Error("Expected error when removing non-existent chain")
	}
}

func TestMigrateOnChainConfig(t *testing.T) {
	cfg := DefaultConfig()

	// Setup old OnChain config
	cfg.OnChain = OnChainConfig{
		Enabled: true,
		RPCURL:  "https://test-rpc.example.com",
		ChainID: 97, // BSC testnet
	}

	// No chains configured yet
	if len(cfg.Chains) != 0 {
		t.Fatal("Expected no chains before migration")
	}

	// Migrate
	cfg.MigrateOnChainConfig()

	// Should have created bsc-testnet chain
	if len(cfg.Chains) != 1 {
		t.Errorf("Expected 1 chain after migration, got %d", len(cfg.Chains))
	}

	chain, ok := cfg.GetChain("bsc-testnet")
	if !ok {
		t.Fatal("Expected bsc-testnet chain after migration")
	}

	if chain.ChainID != 97 {
		t.Errorf("Chain ID = %d, want 97", chain.ChainID)
	}

	if chain.RPCURL != "https://test-rpc.example.com" {
		t.Errorf("RPC URL = %q, want %q", chain.RPCURL, "https://test-rpc.example.com")
	}
}

func TestMigrateOnChainConfigIdempotent(t *testing.T) {
	cfg := DefaultConfig()

	// Setup old OnChain config
	cfg.OnChain = OnChainConfig{
		Enabled: true,
		RPCURL:  "https://test-rpc.example.com",
		ChainID: 97,
	}

	// First migration
	cfg.MigrateOnChainConfig()
	firstLen := len(cfg.Chains)

	// Second migration should not add duplicates
	cfg.MigrateOnChainConfig()
	secondLen := len(cfg.Chains)

	if firstLen != secondLen {
		t.Errorf("Migration not idempotent: first=%d, second=%d", firstLen, secondLen)
	}
}

func TestChainConfigSerialization(t *testing.T) {
	cfg := DefaultConfig()

	chain := ChainConfig{
		Enabled:  true,
		Type:     "evm",
		Name:     "Test Chain",
		RPCURL:   "https://test.example.com",
		ChainID:  999,
		Wallet:   "0xabc123",
		Explorer: "https://explorer.example.com",
	}

	cfg.AddChain("test-chain", chain)

	// Save and reload
	tmpFile := t.TempDir() + "/config.json"
	if err := cfg.Save(tmpFile); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load(tmpFile)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	loadedChain, ok := loaded.GetChain("test-chain")
	if !ok {
		t.Fatal("Chain not found after reload")
	}

	if loadedChain.Name != chain.Name {
		t.Errorf("Name = %q, want %q", loadedChain.Name, chain.Name)
	}
	if loadedChain.ChainID != chain.ChainID {
		t.Errorf("ChainID = %d, want %d", loadedChain.ChainID, chain.ChainID)
	}
	if loadedChain.Wallet != chain.Wallet {
		t.Errorf("Wallet = %q, want %q", loadedChain.Wallet, chain.Wallet)
	}
}
