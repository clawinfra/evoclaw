package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/clawinfra/evoclaw/internal/config"
)

func TestChainAddPreset(t *testing.T) {
	// Create temp config
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "evoclaw.json")

	// Create default config
	cfg := config.DefaultConfig()
	if err := cfg.Save(cfgPath); err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	// Add BSC testnet
	args := []string{"add", "bsc-testnet", "--wallet", "0x1234567890abcdef"}
	exitCode := ChainCommand(args, cfgPath)

	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}

	// Reload and verify
	reloaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Failed to reload config: %v", err)
	}

	chain, ok := reloaded.GetChain("bsc-testnet")
	if !ok {
		t.Fatal("Chain not found after adding")
	}

	if chain.Type != "evm" {
		t.Errorf("Type = %q, want %q", chain.Type, "evm")
	}

	if chain.ChainID != 97 {
		t.Errorf("ChainID = %d, want 97", chain.ChainID)
	}

	if chain.Wallet != "0x1234567890abcdef" {
		t.Errorf("Wallet = %q, want %q", chain.Wallet, "0x1234567890abcdef")
	}
}

func TestChainAddCustom(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "evoclaw.json")

	cfg := config.DefaultConfig()
	if err := cfg.Save(cfgPath); err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	// Add custom chain
	args := []string{
		"add", "my-custom-chain",
		"--type", "evm",
		"--rpc", "https://custom.example.com",
		"--chain-id", "12345",
		"--wallet", "0xabcdef",
	}
	exitCode := ChainCommand(args, cfgPath)

	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}

	reloaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Failed to reload config: %v", err)
	}

	chain, ok := reloaded.GetChain("my-custom-chain")
	if !ok {
		t.Fatal("Chain not found after adding")
	}

	if chain.ChainID != 12345 {
		t.Errorf("ChainID = %d, want 12345", chain.ChainID)
	}
}

func TestChainRemove(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "evoclaw.json")

	cfg := config.DefaultConfig()
	cfg.AddChain("test-chain", config.ChainConfig{
		Enabled: true,
		Type:    "evm",
		RPCURL:  "https://test.example.com",
	})
	if err := cfg.Save(cfgPath); err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	// Remove chain
	args := []string{"remove", "test-chain"}
	exitCode := ChainCommand(args, cfgPath)

	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}

	reloaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Failed to reload config: %v", err)
	}

	if _, ok := reloaded.GetChain("test-chain"); ok {
		t.Error("Chain still exists after removal")
	}
}

func TestChainList(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "evoclaw.json")

	cfg := config.DefaultConfig()
	cfg.AddChain("chain1", config.ChainConfig{
		Enabled: true,
		Type:    "evm",
		Name:    "Chain One",
		RPCURL:  "https://chain1.example.com",
		ChainID: 111,
	})
	cfg.AddChain("chain2", config.ChainConfig{
		Enabled: false,
		Type:    "solana",
		Name:    "Chain Two",
		RPCURL:  "https://chain2.example.com",
	})
	if err := cfg.Save(cfgPath); err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	// Capture stdout (this is a simple test, in real scenario would use io redirection)
	args := []string{"list"}
	exitCode := ChainCommand(args, cfgPath)

	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}
}

func TestChainAddMissingRequired(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "evoclaw.json")

	cfg := config.DefaultConfig()
	if err := cfg.Save(cfgPath); err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	// Try to add chain without required fields
	args := []string{"add", "unknown-chain"}
	exitCode := ChainCommand(args, cfgPath)

	if exitCode == 0 {
		t.Error("Expected non-zero exit code for missing required fields")
	}
}

func TestChainHelp(t *testing.T) {
	// Redirect stderr to avoid noise in test output
	oldStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	defer func() {
		os.Stderr = oldStderr
		_ = w.Close()
		_ = r.Close()
	}()

	args := []string{"help"}
	exitCode := ChainCommand(args, "evoclaw.json")

	if exitCode != 0 {
		t.Errorf("Expected exit code 0 for help, got %d", exitCode)
	}
}

func TestGetChainPresetsCoverage(t *testing.T) {
	// Test all documented presets exist
	presets := []string{
		"bsc", "bsc-testnet",
		"opbnb", "opbnb-testnet",
		"ethereum", "ethereum-sepolia",
		"arbitrum", "optimism", "polygon", "base",
		"hyperliquid",
		"solana", "solana-devnet",
	}

	for _, presetID := range presets {
		preset, ok := config.GetChainPreset(presetID)
		if !ok {
			t.Errorf("Preset %q not found", presetID)
			continue
		}

		if preset.Type == "" {
			t.Errorf("Preset %q has empty Type", presetID)
		}
		if preset.Name == "" {
			t.Errorf("Preset %q has empty Name", presetID)
		}
		if preset.RPCURL == "" {
			t.Errorf("Preset %q has empty RPCURL", presetID)
		}

		// EVM chains must have ChainID
		if preset.Type == "evm" && preset.ChainID == 0 {
			t.Errorf("EVM preset %q has zero ChainID", presetID)
		}
	}
}

func TestChainAddOverride(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "evoclaw.json")

	cfg := config.DefaultConfig()
	if err := cfg.Save(cfgPath); err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	// Add preset with custom RPC override
	args := []string{
		"add", "bsc-testnet",
		"--rpc", "https://custom-bsc-rpc.example.com",
		"--wallet", "0x123",
	}
	exitCode := ChainCommand(args, cfgPath)

	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}

	reloaded, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("Failed to reload config: %v", err)
	}

	chain, ok := reloaded.GetChain("bsc-testnet")
	if !ok {
		t.Fatal("Chain not found after adding")
	}

	// Should use custom RPC, not preset
	if !strings.Contains(chain.RPCURL, "custom-bsc-rpc.example.com") {
		t.Errorf("RPC URL = %q, expected custom RPC", chain.RPCURL)
	}

	// But should still get other preset values
	if chain.ChainID != 97 {
		t.Errorf("ChainID = %d, want 97 from preset", chain.ChainID)
	}
}

func TestChainStatus(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "evoclaw.json")

	cfg := config.DefaultConfig()
	cfg.AddChain("bsc-testnet", config.ChainConfig{
		Enabled: true,
		Type:    "evm",
		Name:    "BSC Testnet",
		RPCURL:  "http://127.0.0.1:19997", // unreachable — tests offline path
	})
	if err := cfg.Save(cfgPath); err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	args := []string{"status", "bsc-testnet"}
	exitCode := ChainCommand(args, cfgPath)
	// Should return 0 even when disconnected (just shows status)
	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}
}

func TestChainStatusMissingID(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "evoclaw.json")
	cfg := config.DefaultConfig()
	if err := cfg.Save(cfgPath); err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	args := []string{"status"}
	exitCode := ChainCommand(args, cfgPath)
	if exitCode == 0 {
		t.Error("Expected non-zero exit code for missing chain-id")
	}
}

func TestChainStatusNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "evoclaw.json")
	cfg := config.DefaultConfig()
	if err := cfg.Save(cfgPath); err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	args := []string{"status", "does-not-exist"}
	exitCode := ChainCommand(args, cfgPath)
	if exitCode == 0 {
		t.Error("Expected non-zero exit code for unknown chain")
	}
}

func TestChainListNoCheck(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "evoclaw.json")

	cfg := config.DefaultConfig()
	cfg.AddChain("polygon", config.ChainConfig{
		Enabled: true,
		Type:    "evm",
		Name:    "Polygon",
		RPCURL:  "https://polygon-rpc.com",
	})
	if err := cfg.Save(cfgPath); err != nil {
		t.Fatalf("Failed to create config: %v", err)
	}

	// Use --no-check to avoid live network calls in tests
	args := []string{"list", "--no-check"}
	exitCode := ChainCommand(args, cfgPath)
	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}
}
