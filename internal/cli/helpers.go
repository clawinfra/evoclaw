package cli

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/clawinfra/evoclaw/internal/config"
)

// loadConfigFromFile loads config from the specified path
func loadConfigFromFile(configPath string) (*config.Config, error) {
	cfg, err := config.Load(configPath)
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
