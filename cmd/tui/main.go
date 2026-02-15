// Command tui starts the EvoClaw TUI — an interactive terminal interface
// for chatting with agents and monitoring the orchestrator.
//
// Usage:
//
//	go run ./cmd/tui --config evoclaw.json
//	# or after building:
//	./evoclaw-tui --config evoclaw.json
//
// The TUI provides:
//   - Split-pane layout: agent sidebar + chat panel + input
//   - Real-time agent status (online/idle/evolving, message count, cost)
//   - Full chat with any registered agent
//   - Works over SSH, tmux, screen — no GUI needed
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/clawinfra/evoclaw/internal/channels"
	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/models"
	"github.com/clawinfra/evoclaw/internal/orchestrator"
)

func main() {
	configPath := flag.String("config", "evoclaw.json", "path to config file")
	flag.Parse()

	// Load config
	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error loading config: %v\n", err)
		os.Exit(1)
	}

	// Set up logging to file (stdout is owned by the TUI)
	logFile, err := os.OpenFile("evoclaw-tui.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error opening log file: %v\n", err)
		os.Exit(1)
	}
	defer logFile.Close() //nolint:errcheck

	logger := slog.New(slog.NewJSONHandler(logFile, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))

	// Create orchestrator
	orch := orchestrator.New(cfg, logger)

	// Register model providers from config
	registerProviders(orch, cfg, logger)

	// Create TUI channel — pass the orchestrator's ListAgents for sidebar updates
	tuiCh := channels.NewTUI(logger, orch.ListAgents)
	orch.RegisterChannel(tuiCh)

	// Start orchestrator (which starts the TUI channel)
	if err := orch.Start(); err != nil {
		fmt.Fprintf(os.Stderr, "error starting orchestrator: %v\n", err)
		os.Exit(1)
	}

	// Wait for signal or TUI exit
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	<-ctx.Done()

	logger.Info("shutting down")
	if err := orch.Stop(); err != nil {
		fmt.Printf("Error stopping orchestrator: %v\n", err)
	}
}

// registerProviders sets up LLM providers from config
func registerProviders(orch *orchestrator.Orchestrator, cfg *config.Config, logger *slog.Logger) {
	for name, provCfg := range cfg.Models.Providers {
		switch name {
		case "anthropic":
			orch.RegisterProvider(models.NewAnthropicProvider(provCfg))
		case "openai":
			orch.RegisterProvider(models.NewOpenAIProvider(name, provCfg))
		case "ollama":
			orch.RegisterProvider(models.NewOllamaProvider(provCfg))
		default:
			// Try as OpenAI-compatible (covers z.ai, nvidia-nim, etc.)
			orch.RegisterProvider(models.NewOpenAIProvider(name, provCfg))
		}
	}
}
