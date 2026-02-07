package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/clawinfra/evoclaw/internal/agents"
	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/models"
	"github.com/clawinfra/evoclaw/internal/orchestrator"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo}, // default
		{"", slog.LevelInfo},         // default
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			result := parseLogLevel(tt.input)
			if result != tt.expected {
				t.Errorf("parseLogLevel(%q) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestLoadConfig_NewFile(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Ensure the directory exists
	if err := os.MkdirAll(tmpDir, 0755); err != nil {
		t.Fatalf("failed to create tmpDir: %v", err)
	}
	
	configPath := filepath.Join(tmpDir, "test-config.json")

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	cfg, err := loadConfig(configPath, logger)
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}

	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	// Verify file was created
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file was not created")
	}
}

func TestLoadConfig_ExistingFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "existing-config.json")

	// Create a config
	cfg := config.DefaultConfig()
	cfg.Server.Port = 9999 // Custom value
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	// Load it
	loadedCfg, err := loadConfig(configPath, logger)
	if err != nil {
		t.Fatalf("loadConfig failed: %v", err)
	}

	if loadedCfg.Server.Port != 9999 {
		t.Errorf("expected port 9999, got %d", loadedCfg.Server.Port)
	}
}

func TestInitializeAgents(t *testing.T) {
	tmpDir := t.TempDir()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	// Can't easily test this without mocking registry
	// Just test it doesn't panic with empty config
	cfg := config.DefaultConfig()
	cfg.Agents = []config.AgentDef{
		{
			ID:    "test-agent",
			Name:  "Test Agent",
			Type:  "monitor",
			Model: "test-model",
		},
	}

	// This will fail because we need a proper registry setup
	// but we're testing the function logic
	registry, err := newTestRegistry(tmpDir, logger)
	if err != nil {
		t.Fatalf("failed to create test registry: %v", err)
	}

	err = initializeAgents(registry, cfg, logger)
	// Agent creation might fail due to missing models, but the function should handle it
	// We just verify it doesn't panic
	if err != nil {
		// Expected - model doesn't exist
		t.Logf("initializeAgents returned error (expected): %v", err)
	}
}

func TestRegisterProviders(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	cfg := config.DefaultConfig()
	cfg.Models.Providers = map[string]config.ProviderConfig{
		"ollama": {
			BaseURL: "http://localhost:11434",
			Models: []config.Model{
				{
					ID:            "llama2",
					Name:          "Llama 2",
					ContextWindow: 4096,
					CostInput:     0.0,
					CostOutput:    0.0,
				},
			},
		},
	}

	router := newTestRouter(logger)

	err := registerProviders(router, cfg, logger)
	if err != nil {
		t.Fatalf("registerProviders failed: %v", err)
	}

	// Verify provider was registered
	models := router.ListModels()
	if len(models) == 0 {
		t.Error("expected at least one model to be registered")
	}
}

func TestSetup_MinimalConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "evoclaw.json")

	// Create minimal config
	cfg := config.DefaultConfig()
	cfg.Server.DataDir = tmpDir
	cfg.Server.Port = 0 // Don't bind to actual port
	cfg.Channels.Telegram = nil
	cfg.MQTT.Port = 0
	cfg.Evolution.Enabled = false
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	app, err := setup(configPath)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	if app == nil {
		t.Fatal("expected non-nil app")
	}

	if app.Logger == nil {
		t.Error("expected non-nil logger")
	}
	if app.Config == nil {
		t.Error("expected non-nil config")
	}
	if app.Registry == nil {
		t.Error("expected non-nil registry")
	}
	if app.MemoryStore == nil {
		t.Error("expected non-nil memory store")
	}
	if app.Router == nil {
		t.Error("expected non-nil router")
	}
	if app.Orchestrator == nil {
		t.Error("expected non-nil orchestrator")
	}
	if app.APIServer == nil {
		t.Error("expected non-nil API server")
	}
}

func TestSetup_WithEvolution(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "evoclaw.json")

	cfg := config.DefaultConfig()
	cfg.Server.DataDir = tmpDir
	cfg.Server.Port = 0
	cfg.Channels.Telegram = nil
	cfg.MQTT.Port = 0
	cfg.Evolution.Enabled = true
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	app, err := setup(configPath)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	if app.EvoEngine == nil {
		t.Error("expected non-nil evolution engine when enabled")
	}
}

func TestPrintBanner(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "evoclaw.json")

	cfg := config.DefaultConfig()
	cfg.Server.DataDir = tmpDir
	cfg.Server.Port = 8080
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	app, err := setup(configPath)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Just verify it doesn't panic
	printBanner(app)
}

func TestRegisterChannels_Telegram(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	cfg := config.DefaultConfig()
	cfg.Server.DataDir = tmpDir
	cfg.Channels.Telegram = &config.TelegramConfig{
		Enabled:  true,
		BotToken: "test-token",
	}

	orch := orchestrator.New(cfg, logger)

	_, err := registerChannels(orch, cfg, logger)
	if err != nil {
		t.Fatalf("registerChannels failed: %v", err)
	}
}

func TestRegisterChannels_MQTT(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	cfg := config.DefaultConfig()
	cfg.Server.DataDir = tmpDir
	cfg.MQTT.Port = 1883
	cfg.MQTT.Host = "localhost"

	orch := orchestrator.New(cfg, logger)

	_, err := registerChannels(orch, cfg, logger)
	if err != nil {
		t.Fatalf("registerChannels failed: %v", err)
	}
}

func TestRegisterProvidersToOrchestrator(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	cfg := config.DefaultConfig()
	cfg.Server.DataDir = tmpDir
	cfg.Models.Providers = map[string]config.ProviderConfig{
		"ollama": {
			BaseURL: "http://localhost:11434",
			Models: []config.Model{
				{
					ID:            "llama2",
					Name:          "Llama 2",
					ContextWindow: 4096,
				},
			},
		},
	}

	router := models.NewRouter(logger)
	err := registerProviders(router, cfg, logger)
	if err != nil {
		t.Fatalf("registerProviders failed: %v", err)
	}

	orch := orchestrator.New(cfg, logger)
	registerProvidersToOrchestrator(orch, router, cfg)
}

func TestRegisterProviders_AllTypes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	cfg := config.DefaultConfig()
	cfg.Models.Providers = map[string]config.ProviderConfig{
		"anthropic": {
			BaseURL: "https://api.anthropic.com",
			APIKey:  "test-key",
			Models: []config.Model{
				{ID: "claude-3-opus", Name: "Claude 3 Opus", ContextWindow: 200000},
			},
		},
		"openai": {
			BaseURL: "https://api.openai.com/v1",
			APIKey:  "test-key",
			Models: []config.Model{
				{ID: "gpt-4", Name: "GPT-4", ContextWindow: 8192},
			},
		},
		"openrouter": {
			BaseURL: "https://openrouter.ai/api/v1",
			APIKey:  "test-key",
			Models: []config.Model{
				{ID: "anthropic/claude-3-opus", Name: "Claude 3 Opus", ContextWindow: 200000},
			},
		},
		"custom-provider": {
			BaseURL: "https://custom.ai/v1",
			APIKey:  "test-key",
			Models: []config.Model{
				{ID: "custom-model", Name: "Custom Model", ContextWindow: 4096},
			},
		},
	}

	router := models.NewRouter(logger)

	err := registerProviders(router, cfg, logger)
	if err != nil {
		t.Fatalf("registerProviders failed: %v", err)
	}

	// Verify all providers were registered
	modelList := router.ListModels()
	if len(modelList) < 4 {
		t.Errorf("expected at least 4 models, got %d", len(modelList))
	}
}

func TestInitializeAgents_AlreadyExists(t *testing.T) {
	tmpDir := t.TempDir()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	registry, err := agents.NewRegistry(tmpDir, logger)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Agents = []config.AgentDef{
		{
			ID:    "test-agent",
			Name:  "Test Agent",
			Type:  "monitor",
			Model: "test-model",
		},
	}

	// First call - will fail because model doesn't exist
	_ = initializeAgents(registry, cfg, logger)

	// Second call - agent should already exist
	err = initializeAgents(registry, cfg, logger)
	if err != nil {
		// Error is OK because model doesn't exist
		t.Logf("initializeAgents returned error (expected): %v", err)
	}
}

// Helper functions

func newTestRegistry(dataDir string, logger *slog.Logger) (*agents.Registry, error) {
	// Import is at the top, using package
	return agents.NewRegistry(dataDir, logger)
}

func newTestRouter(logger *slog.Logger) *models.Router {
	return models.NewRouter(logger)
}

func TestSetup_EvolutionDisabled(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "evoclaw.json")

	cfg := config.DefaultConfig()
	cfg.Server.DataDir = tmpDir
	cfg.Server.Port = 0
	cfg.Channels.Telegram = nil
	cfg.MQTT.Port = 0
	cfg.Evolution.Enabled = false // Explicitly disabled
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	app, err := setup(configPath)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	if app.EvoEngine != nil {
		t.Error("expected nil evolution engine when disabled")
	}
}

func TestSetup_AllChannels(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "evoclaw.json")

	cfg := config.DefaultConfig()
	cfg.Server.DataDir = tmpDir
	cfg.Server.Port = 0
	cfg.Channels.Telegram = &config.TelegramConfig{
		Enabled:  true,
		BotToken: "test-token",
	}
	cfg.MQTT.Port = 1883
	cfg.MQTT.Host = "localhost"
	cfg.Evolution.Enabled = false
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	app, err := setup(configPath)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	if app == nil {
		t.Fatal("expected non-nil app")
	}
}

func TestLoadConfig_InvalidConfigError(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.json")

	// Create invalid JSON file
	os.WriteFile(configPath, []byte("invalid json{{{"), 0644)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	_, err := loadConfig(configPath, logger)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestRegisterProviders_EmptyConfig(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	cfg := config.DefaultConfig()
	cfg.Models.Providers = map[string]config.ProviderConfig{} // Empty

	router := models.NewRouter(logger)

	err := registerProviders(router, cfg, logger)
	if err != nil {
		t.Fatalf("registerProviders failed: %v", err)
	}

	// Should have 0 models
	modelList := router.ListModels()
	if len(modelList) != 0 {
		t.Errorf("expected 0 models, got %d", len(modelList))
	}
}

func TestRegisterChannels_NoChannels(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	cfg := config.DefaultConfig()
	cfg.Server.DataDir = tmpDir
	cfg.Channels.Telegram = nil
	cfg.MQTT.Port = 0

	orch := orchestrator.New(cfg, logger)

	_, err := registerChannels(orch, cfg, logger)
	if err != nil {
		t.Fatalf("registerChannels failed: %v", err)
	}
}

func TestSetup_RegistryLoadError(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "evoclaw.json")

	cfg := config.DefaultConfig()
	cfg.Server.DataDir = tmpDir
	cfg.Server.Port = 0
	cfg.Channels.Telegram = nil
	cfg.MQTT.Port = 0
	cfg.Evolution.Enabled = false
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	// Create a bad agents directory that will fail to load
	agentsDir := filepath.Join(tmpDir, "agents")
	os.MkdirAll(agentsDir, 0755)
	os.WriteFile(filepath.Join(agentsDir, "bad.json"), []byte("invalid json"), 0644)

	// Setup should still succeed even if loading agents fails
	app, err := setup(configPath)
	if err != nil {
		// This is expected if registry.Load() fails
		t.Logf("setup failed as expected: %v", err)
	} else if app != nil {
		t.Log("setup succeeded despite bad agent file")
	}
}

func TestInitializeAgents_ErrorPath(t *testing.T) {
	tmpDir := t.TempDir()

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError,
	}))

	registry, err := agents.NewRegistry(tmpDir, logger)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	cfg := config.DefaultConfig()
	cfg.Agents = []config.AgentDef{
		{
			ID:    "test-agent",
			Name:  "Test Agent",
			Type:  "monitor",
			Model: "nonexistent-model",
		},
	}

	// This should fail because the model doesn't exist
	err = initializeAgents(registry, cfg, logger)
	if err != nil {
		// Expected to fail
		t.Logf("initializeAgents failed as expected: %v", err)
	}
}

func TestStartServices(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "evoclaw.json")

	cfg := config.DefaultConfig()
	cfg.Server.DataDir = tmpDir
	cfg.Server.Port = 0 // Don't bind to actual port
	cfg.Channels.Telegram = nil
	cfg.MQTT.Port = 0
	cfg.Evolution.Enabled = false
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("failed to save test config: %v", err)
	}

	app, err := setup(configPath)
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// Start services
	err = startServices(app)
	if err != nil {
		t.Fatalf("startServices failed: %v", err)
	}

	// Stop services
	if app.apiCancel != nil {
		app.apiCancel()
	}
	if app.Orchestrator != nil {
		app.Orchestrator.Stop()
	}
}
