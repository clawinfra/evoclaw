//go:build windows

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
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

func setupSignalHandlers(ctx context.Context, cancel context.CancelFunc, logger *slog.Logger) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		for sig := range sigChan {
			switch sig {
			case syscall.SIGINT, syscall.SIGTERM:
				logger.Info("shutdown signal received", "signal", sig)
				cancel()
			}
		}
	}()
}
