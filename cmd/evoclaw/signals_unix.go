//go:build !windows

package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func setupSignalHandlers(ctx context.Context, cancel context.CancelFunc, logger *slog.Logger) {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGUSR1)

	go func() {
		for sig := range sigChan {
			switch sig {
			case syscall.SIGINT, syscall.SIGTERM:
				logger.Info("shutdown signal received", "signal", sig)
				cancel()
				
			case syscall.SIGHUP:
				logger.Info("reload signal received - config reload not yet implemented")
				// TODO: Reload config
				
			case syscall.SIGUSR1:
				logger.Info("update signal received - self-update not yet implemented")
				// TODO: Trigger self-update
			}
		}
	}()
}
