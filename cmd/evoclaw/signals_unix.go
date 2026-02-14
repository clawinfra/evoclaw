//go:build !windows

package main

import (
	"log/slog"
	"os"
	"syscall"
)

// getShutdownSignals returns the signals to listen for on Unix systems
func getShutdownSignals() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGUSR1}
}

// handlePlatformSignal handles platform-specific signals, returns true if should continue loop
func handlePlatformSignal(sig os.Signal, logger *slog.Logger) bool {
	switch sig {
	case syscall.SIGHUP:
		logger.Info("reload signal received - config reload not yet implemented")
		return true // continue loop
	case syscall.SIGUSR1:
		logger.Info("update signal received - self-update not yet implemented")
		return true // continue loop
	}
	return false // don't continue, proceed to shutdown
}
