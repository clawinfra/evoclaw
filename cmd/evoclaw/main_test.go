package main

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"github.com/clawinfra/evoclaw/internal/agents"
	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/models"
	"github.com/clawinfra/evoclaw/internal/onchain"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		input string
		want  slog.Level
	}{
		{"debug", slog.LevelDebug},
		{"info", slog.LevelInfo},
		{"warn", slog.LevelWarn},
		{"error", slog.LevelError},
		{"unknown", slog.LevelInfo},
		{"", slog.LevelInfo},
	}
	for _, tt := range tests {
		if got := parseLogLevel(tt.input); got != tt.want {
			t.Errorf("parseLogLevel(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestLoadConfigDefault(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "evoclaw.json")
	logger := slog.Default()

	cfg, err := loadConfig(path, logger)
	if err != nil {
		t.Fatalf("loadConfig() error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil config")
	}

	// File should be created
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected config file to be created")
	}
}

func TestLoadConfigExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "evoclaw.json")
	logger := slog.Default()

	// Create config first
	cfg := config.DefaultConfig()
	cfg.Save(path)

	// Load it
	loaded, err := loadConfig(path, logger)
	if err != nil {
		t.Fatalf("loadConfig() error: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil config")
	}
}

func TestLoadConfigInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "evoclaw.json")

	os.WriteFile(path, []byte("invalid json"), 0644)
	_, err := loadConfig(path, slog.Default())
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestInitializeAgents(t *testing.T) {
	dir := t.TempDir()
	logger := slog.Default()
	reg, _ := agents.NewRegistry(dir, logger)

	cfg := config.DefaultConfig()
	cfg.Agents = []config.AgentDef{
		{ID: "test-1", Name: "Test", Type: "orchestrator", Model: "test/model"},
	}

	err := initializeAgents(reg, cfg, logger)
	if err != nil {
		t.Fatalf("initializeAgents() error: %v", err)
	}

	agents := reg.List()
	if len(agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(agents))
	}
}

func TestSetupChains(t *testing.T) {
	logger := slog.Default()
	reg := onchain.NewChainRegistry(logger)

	cfg := config.DefaultConfig()
	// No chains configured
	err := setupChains(reg, cfg, logger)
	if err != nil {
		t.Fatalf("setupChains() error: %v", err)
	}
}

func TestPrintBanner(t *testing.T) {
	dir := t.TempDir()
	logger := slog.Default()
	reg, _ := agents.NewRegistry(dir, logger)
	router := models.NewRouter(logger)

	app := &App{
		Config:   config.DefaultConfig(),
		Logger:   logger,
		Registry: reg,
		Router:   router,
	}
	printBanner(app)
}
