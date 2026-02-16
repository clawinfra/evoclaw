package cli

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/clawinfra/evoclaw/internal/config"
)

// loadConfigFromFile loads config from the standard location
func loadConfigFromFile() (*config.Config, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("get home directory: %w", err)
	}

	configPath := filepath.Join(homeDir, ".evoclaw", "config.toml")
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return nil, fmt.Errorf("load config: %w", err)
	}

	return cfg, nil
}

// getLogger returns a configured logger
func getLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
}
