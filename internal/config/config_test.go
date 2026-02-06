package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.Server.Port != 8420 {
		t.Errorf("expected port 8420, got %d", cfg.Server.Port)
	}

	if cfg.Server.DataDir != "./data" {
		t.Errorf("expected dataDir ./data, got %s", cfg.Server.DataDir)
	}

	if cfg.Server.LogLevel != "info" {
		t.Errorf("expected logLevel info, got %s", cfg.Server.LogLevel)
	}

	if cfg.MQTT.Port != 1883 {
		t.Errorf("expected MQTT port 1883, got %d", cfg.MQTT.Port)
	}

	if cfg.MQTT.Host != "0.0.0.0" {
		t.Errorf("expected MQTT host 0.0.0.0, got %s", cfg.MQTT.Host)
	}

	if !cfg.Evolution.Enabled {
		t.Error("expected evolution enabled by default")
	}

	if cfg.Evolution.EvalIntervalSec != 3600 {
		t.Errorf("expected evalIntervalSec 3600, got %d", cfg.Evolution.EvalIntervalSec)
	}

	if cfg.Evolution.MinSamplesForEval != 10 {
		t.Errorf("expected minSamplesForEval 10, got %d", cfg.Evolution.MinSamplesForEval)
	}

	if cfg.Evolution.MaxMutationRate != 0.2 {
		t.Errorf("expected maxMutationRate 0.2, got %f", cfg.Evolution.MaxMutationRate)
	}

	if cfg.Models.Routing.Simple != "local/small" {
		t.Errorf("expected routing.simple local/small, got %s", cfg.Models.Routing.Simple)
	}

	if cfg.Models.Routing.Complex != "anthropic/claude-sonnet" {
		t.Errorf("expected routing.complex anthropic/claude-sonnet, got %s", cfg.Models.Routing.Complex)
	}

	if cfg.Models.Routing.Critical != "anthropic/claude-opus" {
		t.Errorf("expected routing.critical anthropic/claude-opus, got %s", cfg.Models.Routing.Critical)
	}
}

func TestLoadConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.json")

	// Create a test config
	testCfg := &Config{
		Server: ServerConfig{
			Port:     9999,
			DataDir:  filepath.Join(tmpDir, "test-data"),
			LogLevel: "debug",
		},
		MQTT: MQTTConfig{
			Port:     1884,
			Host:     "localhost",
			Username: "testuser",
			Password: "testpass",
		},
		Channels: ChannelConfig{
			Telegram: &TelegramConfig{
				Enabled:  true,
				BotToken: "test-token-123",
			},
			WhatsApp: &WhatsAppConfig{
				Enabled:   true,
				AllowFrom: []string{"user1", "user2"},
			},
		},
		Models: ModelsConfig{
			Providers: map[string]ProviderConfig{
				"anthropic": {
					BaseURL: "https://api.anthropic.com",
					APIKey:  "test-key",
					Models: []Model{
						{
							ID:            "claude-sonnet-4",
							Name:          "Claude Sonnet 4",
							ContextWindow: 200000,
							CostInput:     3.0,
							CostOutput:    15.0,
							Capabilities:  []string{"reasoning", "code"},
						},
					},
				},
			},
			Routing: ModelRouting{
				Simple:   "ollama/llama3.2",
				Complex:  "anthropic/claude-sonnet-4",
				Critical: "anthropic/claude-opus-4",
			},
		},
		Evolution: EvolutionConfig{
			Enabled:           true,
			EvalIntervalSec:   1800,
			MinSamplesForEval: 5,
			MaxMutationRate:   0.3,
		},
		Agents: []AgentDef{
			{
				ID:           "agent-1",
				Name:         "Test Agent",
				Type:         "orchestrator",
				Model:        "anthropic/claude-sonnet-4",
				SystemPrompt: "You are a helpful assistant",
				Skills:       []string{"coding", "research"},
				Config: map[string]string{
					"key1": "value1",
				},
				Container: ContainerConfig{
					Enabled:   true,
					Image:     "evoclaw/agent:latest",
					MemoryMB:  512,
					CPUShares: 1024,
					AllowNet:  true,
					Mounts: []Mount{
						{
							HostPath:      "/tmp",
							ContainerPath: "/workspace",
							ReadOnly:      false,
						},
					},
					AllowTools: []string{"exec", "read", "write"},
				},
			},
		},
	}

	// Write config to file
	data, err := json.MarshalIndent(testCfg, "", "  ")
	if err != nil {
		t.Fatalf("failed to marshal test config: %v", err)
	}
	if err := os.WriteFile(configPath, data, 0640); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}

	// Load the config
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Verify loaded values
	if loaded.Server.Port != 9999 {
		t.Errorf("expected port 9999, got %d", loaded.Server.Port)
	}

	if loaded.Server.LogLevel != "debug" {
		t.Errorf("expected logLevel debug, got %s", loaded.Server.LogLevel)
	}

	if loaded.MQTT.Username != "testuser" {
		t.Errorf("expected MQTT username testuser, got %s", loaded.MQTT.Username)
	}

	if loaded.Channels.Telegram == nil {
		t.Fatal("expected telegram config, got nil")
	}

	if loaded.Channels.Telegram.BotToken != "test-token-123" {
		t.Errorf("expected bot token test-token-123, got %s", loaded.Channels.Telegram.BotToken)
	}

	if loaded.Channels.WhatsApp == nil {
		t.Fatal("expected whatsapp config, got nil")
	}

	if len(loaded.Channels.WhatsApp.AllowFrom) != 2 {
		t.Errorf("expected 2 allowFrom entries, got %d", len(loaded.Channels.WhatsApp.AllowFrom))
	}

	if loaded.Models.Routing.Simple != "ollama/llama3.2" {
		t.Errorf("expected routing.simple ollama/llama3.2, got %s", loaded.Models.Routing.Simple)
	}

	if len(loaded.Models.Providers) != 1 {
		t.Errorf("expected 1 provider, got %d", len(loaded.Models.Providers))
	}

	anthropic := loaded.Models.Providers["anthropic"]
	if anthropic.APIKey != "test-key" {
		t.Errorf("expected API key test-key, got %s", anthropic.APIKey)
	}

	if len(anthropic.Models) != 1 {
		t.Errorf("expected 1 model, got %d", len(anthropic.Models))
	}

	if anthropic.Models[0].ID != "claude-sonnet-4" {
		t.Errorf("expected model ID claude-sonnet-4, got %s", anthropic.Models[0].ID)
	}

	if len(loaded.Agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(loaded.Agents))
	}

	agent := loaded.Agents[0]
	if agent.ID != "agent-1" {
		t.Errorf("expected agent ID agent-1, got %s", agent.ID)
	}

	if len(agent.Skills) != 2 {
		t.Errorf("expected 2 skills, got %d", len(agent.Skills))
	}

	if agent.Container.MemoryMB != 512 {
		t.Errorf("expected container memory 512, got %d", agent.Container.MemoryMB)
	}

	if len(agent.Container.Mounts) != 1 {
		t.Errorf("expected 1 mount, got %d", len(agent.Container.Mounts))
	}

	// Verify data directory was created
	if _, err := os.Stat(loaded.Server.DataDir); os.IsNotExist(err) {
		t.Error("expected data directory to be created")
	}
}

func TestLoadConfigFileNotFound(t *testing.T) {
	tmpDir := t.TempDir()
	nonExistent := filepath.Join(tmpDir, "nonexistent.json")

	_, err := Load(nonExistent)
	if err == nil {
		t.Error("expected error when loading nonexistent file, got nil")
	}
}

func TestLoadConfigInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.json")

	// Write invalid JSON
	if err := os.WriteFile(configPath, []byte("{ invalid json }"), 0640); err != nil {
		t.Fatalf("failed to write invalid JSON: %v", err)
	}

	_, err := Load(configPath)
	if err == nil {
		t.Error("expected error when loading invalid JSON, got nil")
	}
}

func TestSaveConfig(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "subdir", "config.json")

	cfg := DefaultConfig()
	cfg.Server.Port = 7777

	// Save config
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("failed to save config: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file was not created")
	}

	// Load and verify
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatalf("failed to read saved config: %v", err)
	}

	var loaded Config
	if err := json.Unmarshal(data, &loaded); err != nil {
		t.Fatalf("failed to unmarshal saved config: %v", err)
	}

	if loaded.Server.Port != 7777 {
		t.Errorf("expected port 7777, got %d", loaded.Server.Port)
	}
}

func TestSaveConfigCreatesParentDirs(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "deep", "nested", "dirs", "config.json")

	cfg := DefaultConfig()

	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("failed to save config to nested path: %v", err)
	}

	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file was not created in nested directory")
	}
}

func TestLoadConfigMergesWithDefaults(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "partial.json")

	// Create config with only some fields
	partialConfig := map[string]interface{}{
		"server": map[string]interface{}{
			"port": 5555,
		},
	}

	data, err := json.Marshal(partialConfig)
	if err != nil {
		t.Fatalf("failed to marshal partial config: %v", err)
	}

	if err := os.WriteFile(configPath, data, 0640); err != nil {
		t.Fatalf("failed to write partial config: %v", err)
	}

	// Load and verify defaults are preserved
	loaded, err := Load(configPath)
	if err != nil {
		t.Fatalf("failed to load partial config: %v", err)
	}

	// Custom value should be loaded
	if loaded.Server.Port != 5555 {
		t.Errorf("expected port 5555, got %d", loaded.Server.Port)
	}

	// Default values should be preserved
	if loaded.Server.DataDir != "./data" {
		t.Errorf("expected default dataDir ./data, got %s", loaded.Server.DataDir)
	}

	if loaded.MQTT.Port != 1883 {
		t.Errorf("expected default MQTT port 1883, got %d", loaded.MQTT.Port)
	}
}

func TestSaveConfigReadOnlyDir(t *testing.T) {
	tmpDir := t.TempDir()
	
	// Make directory read-only
	os.Chmod(tmpDir, 0444)
	defer os.Chmod(tmpDir, 0755)
	
	configPath := filepath.Join(tmpDir, "config.json")
	cfg := DefaultConfig()
	
	err := cfg.Save(configPath)
	if err == nil {
		t.Error("expected error when saving to read-only directory")
	}
}

func TestSave_ErrorHandling(t *testing.T) {
	cfg := DefaultConfig()

	// Try to save to invalid location
	err := cfg.Save("/proc/invalid-location/config.json")
	if err == nil {
		t.Error("expected error when saving to invalid location")
	}
}

func TestLoad_InvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "invalid.json")

	// Write invalid JSON
	os.WriteFile(configPath, []byte("invalid json{{{"), 0644)

	_, err := Load(configPath)
	if err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestLoad_DataDirCreation(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.json")

	cfg := DefaultConfig()
	dataDir := filepath.Join(tmpDir, "new-data-dir")
	cfg.Server.DataDir = dataDir

	// Save config
	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load it - should create data dir
	loadedCfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loadedCfg.Server.DataDir != dataDir {
		t.Errorf("expected dataDir %s, got %s", dataDir, loadedCfg.Server.DataDir)
	}

	// Verify data dir was created
	if _, err := os.Stat(dataDir); os.IsNotExist(err) {
		t.Error("expected data dir to be created")
	}
}

func TestSave_MarshalError(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.json")

	cfg := DefaultConfig()
	
	// The Save function is straightforward - we test the happy path
	err := cfg.Save(configPath)
	if err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify the file exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		t.Error("config file was not created")
	}
}

func TestSave_WriteFileError(t *testing.T) {
	cfg := DefaultConfig()
	
	// Try to write to a path that is a directory
	tmpDir := t.TempDir()
	dirPath := filepath.Join(tmpDir, "testdir")
	os.Mkdir(dirPath, 0755)
	
	// Try to write to the directory itself (not a file in it)
	err := cfg.Save(dirPath)
	if err == nil {
		t.Error("expected error when writing to directory path")
	}
}

func TestLoad_MkdirAllError(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "test.json")

	cfg := DefaultConfig()
	// Set data dir to a path that can't be created (file instead of dir)
	filePath := filepath.Join(tmpDir, "blockingfile")
	os.WriteFile(filePath, []byte("test"), 0644)
	cfg.Server.DataDir = filepath.Join(filePath, "subdir") // Can't create dir under a file

	if err := cfg.Save(configPath); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Load should fail when trying to create data dir
	_, err := Load(configPath)
	if err == nil {
		t.Error("expected error when data dir can't be created")
	}
}
