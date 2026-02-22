//go:build !windows

package main

import (
	"log/slog"
	"os"
	"syscall"

	"github.com/clawinfra/evoclaw/internal/config"
)

// getShutdownSignals returns the signals to listen for on Unix systems
func getShutdownSignals() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGUSR1}
}

// handlePlatformSignal handles platform-specific signals, returns true if should continue loop
func handlePlatformSignal(sig os.Signal, logger *slog.Logger) bool {
	switch sig {
	case syscall.SIGHUP:
		logger.Info("SIGHUP received, reloading configuration")
		reloadConfig(logger)
		return true // continue loop
	case syscall.SIGUSR1:
		logger.Info("update signal received - self-update not yet implemented")
		return true // continue loop
	}
	return false // don't continue, proceed to shutdown
}

// activeConfig holds the running config for SIGHUP reload.
// Set by the main startup code.
var activeConfig *config.Config
var activeConfigPath string

// SetActiveConfig sets the config instance used for SIGHUP reloads.
func SetActiveConfig(cfg *config.Config, path string) {
	activeConfig = cfg
	activeConfigPath = path
}

func reloadConfig(logger *slog.Logger) {
	if activeConfig == nil || activeConfigPath == "" {
		logger.Error("config reload: no active config set")
		return
	}

	result, err := activeConfig.Reload(activeConfigPath)
	if err != nil {
		logger.Error("config reload failed", "error", err)
		return
	}

	result.LogResult(logger)
}
