//go:build windows

package main

import (
	"log/slog"
	"os"
	"syscall"
)

// getShutdownSignals returns the signals to listen for on Windows
func getShutdownSignals() []os.Signal {
	return []os.Signal{syscall.SIGINT, syscall.SIGTERM}
}

// handlePlatformSignal handles platform-specific signals, returns true if should continue loop
func handlePlatformSignal(sig os.Signal, logger *slog.Logger) bool {
	// Windows only handles SIGINT and SIGTERM, no special cases
	return false
}
