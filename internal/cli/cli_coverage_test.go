package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/clawinfra/evoclaw/internal/config"
)

// --- InitCommand tests (non-interactive mode) ---

func TestInitCommandNonInteractive(t *testing.T) {
	tmpDir := t.TempDir()
	output := filepath.Join(tmpDir, "evoclaw.json")

	args := []string{
		"--non-interactive",
		"--provider", "anthropic",
		"--key", "sk-test-key",
		"--name", "test-agent",
		"--skip-chain",
		"--output", output,
	}
	exitCode := InitCommand(args)
	if exitCode != 0 {
		t.Fatalf("InitCommand returned %d, want 0", exitCode)
	}

	// Verify file was created
	if _, err := os.Stat(output); os.IsNotExist(err) {
		t.Fatal("config file was not created")
	}

	// Load and verify config
	cfg, err := config.Load(output)
	if err != nil {
		t.Fatalf("Failed to load created config: %v", err)
	}
	if len(cfg.Agents) == 0 || cfg.Agents[0].Name != "test-agent" {
		t.Errorf("Agent name = %q, want %q", cfg.Agents[0].Name, "test-agent")
	}
}

func TestInitCommandNonInteractiveOllama(t *testing.T) {
	tmpDir := t.TempDir()
	output := filepath.Join(tmpDir, "evoclaw.json")

	args := []string{
		"--non-interactive",
		"--provider", "ollama",
		"--name", "local-agent",
		"--output", output,
	}
	exitCode := InitCommand(args)
	if exitCode != 0 {
		t.Fatalf("InitCommand returned %d, want 0", exitCode)
	}
}

func TestInitCommandNonInteractiveOpenRouter(t *testing.T) {
	tmpDir := t.TempDir()
	output := filepath.Join(tmpDir, "evoclaw.json")

	args := []string{
		"--non-interactive",
		"--provider", "openrouter",
		"--key", "sk-or-test",
		"--name", "or-agent",
		"--output", output,
	}
	exitCode := InitCommand(args)
	if exitCode != 0 {
		t.Fatalf("InitCommand returned %d, want 0", exitCode)
	}
}

func TestInitCommandNonInteractiveOpenAI(t *testing.T) {
	tmpDir := t.TempDir()
	output := filepath.Join(tmpDir, "evoclaw.json")

	args := []string{
		"--non-interactive",
		"--provider", "openai",
		"--key", "sk-test",
		"--name", "openai-agent",
		"--output", output,
	}
	exitCode := InitCommand(args)
	if exitCode != 0 {
		t.Fatalf("InitCommand returned %d, want 0", exitCode)
	}
}

func TestInitCommandMissingProvider(t *testing.T) {
	tmpDir := t.TempDir()
	output := filepath.Join(tmpDir, "evoclaw.json")

	args := []string{
		"--non-interactive",
		"--name", "agent",
		"--output", output,
	}
	exitCode := InitCommand(args)
	if exitCode == 0 {
		t.Fatal("Expected non-zero exit when provider is missing")
	}
}

func TestInitCommandMissingKeyForNonOllama(t *testing.T) {
	tmpDir := t.TempDir()
	output := filepath.Join(tmpDir, "evoclaw.json")

	args := []string{
		"--non-interactive",
		"--provider", "anthropic",
		"--name", "agent",
		"--output", output,
	}
	exitCode := InitCommand(args)
	if exitCode == 0 {
		t.Fatal("Expected non-zero exit when API key is missing for non-ollama provider")
	}
}

func TestInitCommandMissingName(t *testing.T) {
	tmpDir := t.TempDir()
	output := filepath.Join(tmpDir, "evoclaw.json")

	args := []string{
		"--non-interactive",
		"--provider", "ollama",
		"--output", output,
	}
	exitCode := InitCommand(args)
	if exitCode == 0 {
		t.Fatal("Expected non-zero exit when name is missing")
	}
}

// --- ChainCommand edge cases ---

func TestChainCommandNoArgs(t *testing.T) {
	exitCode := ChainCommand(nil, "evoclaw.json")
	if exitCode == 0 {
		t.Error("Expected non-zero exit for no args")
	}
}

func TestChainCommandUnknownSubcommand(t *testing.T) {
	exitCode := ChainCommand([]string{"invalid"}, "evoclaw.json")
	if exitCode == 0 {
		t.Error("Expected non-zero exit for unknown subcommand")
	}
}

func TestChainRemoveNoArgs(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "evoclaw.json")
	cfg := config.DefaultConfig()
	_ = cfg.Save(cfgPath)

	exitCode := ChainCommand([]string{"remove"}, cfgPath)
	if exitCode == 0 {
		t.Error("Expected non-zero exit for remove without chain-id")
	}
}

func TestChainRemoveNonexistent(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "evoclaw.json")
	cfg := config.DefaultConfig()
	_ = cfg.Save(cfgPath)

	exitCode := ChainCommand([]string{"remove", "nonexistent"}, cfgPath)
	if exitCode == 0 {
		t.Error("Expected non-zero exit for removing nonexistent chain")
	}
}

func TestChainListEmpty(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "evoclaw.json")
	cfg := config.DefaultConfig()
	_ = cfg.Save(cfgPath)

	exitCode := ChainCommand([]string{"list"}, cfgPath)
	if exitCode != 0 {
		t.Errorf("Expected exit code 0 for empty list, got %d", exitCode)
	}
}

func TestChainListBadConfig(t *testing.T) {
	exitCode := ChainCommand([]string{"list"}, "/nonexistent/path/config.json")
	if exitCode == 0 {
		t.Error("Expected non-zero exit for bad config path")
	}
}

func TestChainAddNoChainID(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "evoclaw.json")
	cfg := config.DefaultConfig()
	_ = cfg.Save(cfgPath)

	// No chain-id argument
	exitCode := ChainCommand([]string{"add"}, cfgPath)
	if exitCode == 0 {
		t.Error("Expected non-zero exit for add without chain-id")
	}
}

func TestChainAddBadConfigPath(t *testing.T) {
	exitCode := ChainCommand([]string{"add", "bsc-testnet"}, "/nonexistent/path/config.json")
	if exitCode == 0 {
		t.Error("Expected non-zero exit for bad config path")
	}
}

func TestChainAddCustomMissingRPC(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "evoclaw.json")
	cfg := config.DefaultConfig()
	_ = cfg.Save(cfgPath)

	// Custom chain without type
	args := []string{"add", "custom-chain", "--rpc", "https://example.com"}
	exitCode := ChainCommand(args, cfgPath)
	if exitCode == 0 {
		t.Error("Expected non-zero exit for custom chain without type")
	}
}

func TestChainAddEVMMissingChainID(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "evoclaw.json")
	cfg := config.DefaultConfig()
	_ = cfg.Save(cfgPath)

	args := []string{"add", "custom-evm", "--type", "evm", "--rpc", "https://example.com"}
	exitCode := ChainCommand(args, cfgPath)
	if exitCode == 0 {
		t.Error("Expected non-zero exit for EVM chain without chain-id")
	}
}

func TestChainAddWithExplorer(t *testing.T) {
	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "evoclaw.json")
	cfg := config.DefaultConfig()
	_ = cfg.Save(cfgPath)

	args := []string{
		"add", "bsc-testnet",
		"--wallet", "0xabc",
		"--explorer", "https://custom-explorer.com",
	}
	exitCode := ChainCommand(args, cfgPath)
	if exitCode != 0 {
		t.Errorf("Expected exit code 0, got %d", exitCode)
	}
}

func TestChainRemoveBadConfigPath(t *testing.T) {
	exitCode := ChainCommand([]string{"remove", "test"}, "/nonexistent/path/config.json")
	if exitCode == 0 {
		t.Error("Expected non-zero exit for bad config path")
	}
}

// --- buildConfig coverage ---

func TestBuildConfigAllProviders(t *testing.T) {
	for _, provider := range []string{"anthropic", "openai", "openrouter", "ollama"} {
		t.Run(provider, func(t *testing.T) {
			cfg := buildConfig(provider, "test-key", "my-agent", true, true, false)
			if cfg == nil {
				t.Fatal("buildConfig returned nil")
			}
			if len(cfg.Agents) != 1 {
				t.Fatalf("Expected 1 agent, got %d", len(cfg.Agents))
			}
			if cfg.Agents[0].Name != "my-agent" {
				t.Errorf("Agent name = %q, want %q", cfg.Agents[0].Name, "my-agent")
			}
			if cfg.Models.Routing.Simple == "" {
				t.Error("Routing simple model should be set")
			}
			if cfg.Models.Routing.Complex == "" {
				t.Error("Routing complex model should be set")
			}
		})
	}
}

func TestNormalizeProviderEdgeCases(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"  ANTHROPIC  ", "anthropic"},
		{"OPENAI", "openai"},
		{"  1  ", "anthropic"},
		{"5", ""},
		{"", ""},
		{"invalid", ""},
	}
	for _, tt := range tests {
		result := normalizeProvider(tt.input)
		if result != tt.expected {
			t.Errorf("normalizeProvider(%q) = %q, want %q", tt.input, result, tt.expected)
		}
	}
}

// --- printChainHelp coverage ---
func TestPrintChainHelp(t *testing.T) {
	// Just ensure it doesn't panic
	printChainHelp()
}

func TestChainCommandDashH(t *testing.T) {
	exitCode := ChainCommand([]string{"-h"}, "evoclaw.json")
	if exitCode != 0 {
		t.Errorf("Expected exit code 0 for -h, got %d", exitCode)
	}
}

func TestChainCommandDashDashHelp(t *testing.T) {
	exitCode := ChainCommand([]string{"--help"}, "evoclaw.json")
	if exitCode != 0 {
		t.Errorf("Expected exit code 0 for --help, got %d", exitCode)
	}
}
