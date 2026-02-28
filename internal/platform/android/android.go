//go:build android

// Package android provides an EvoClaw agent adapter for Android platform.
// It enables EvoClaw agents to run as Android foreground services and handle
// Android intents via gomobile bindings.
//
// # Building for Android
//
// Prerequisites:
//
//	go install golang.org/x/mobile/cmd/gomobile@latest
//	gomobile init
//
// Build AAR (Android Archive):
//
//	gomobile bind -target android -o evoclaw.aar github.com/clawinfra/evoclaw/internal/platform/android
//
// The generated AAR can be imported into Android Studio projects. Only
// interfaces and primitive types are exported in the gomobile API surface.
package android

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
)

// AgentConfig holds configuration for an Android EvoClaw agent.
type AgentConfig struct {
	// AgentID is the unique identifier for this agent instance.
	AgentID string
	// APIKey is the LLM provider API key.
	APIKey string
	// Model is the LLM model name (e.g. "claude-3-haiku-20240307").
	Model string
	// DataDir is the path to a writable directory for agent state/memory.
	DataDir string
	// LogLevel controls verbosity: "debug", "info", "warn", "error".
	LogLevel string
}

// AndroidAgent wraps core agent functionality for the Android platform.
// It manages the agent lifecycle as an Android foreground service.
type AndroidAgent struct {
	config  AgentConfig
	logger  *slog.Logger
	cancel  context.CancelFunc
	mu      sync.Mutex
	running bool
	status  agentStatus
}

type agentStatus struct {
	Running  bool   `json:"running"`
	AgentID  string `json:"agent_id"`
	Model    string `json:"model"`
	Platform string `json:"platform"`
}

// NewAndroidAgent creates a new AndroidAgent with the given configuration.
// Returns an error if the configuration is invalid.
func NewAndroidAgent(config AgentConfig) (*AndroidAgent, error) {
	if config.AgentID == "" {
		return nil, fmt.Errorf("android: AgentID is required")
	}
	if config.DataDir == "" {
		return nil, fmt.Errorf("android: DataDir is required")
	}

	level := slog.LevelInfo
	switch config.LogLevel {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	}

	logger := slog.New(slog.NewJSONHandler(nil, &slog.HandlerOptions{Level: level}))

	return &AndroidAgent{
		config: config,
		logger: logger,
		status: agentStatus{
			AgentID:  config.AgentID,
			Model:    config.Model,
			Platform: "android",
		},
	}, nil
}

// Start starts the agent as an Android foreground service.
// It is safe to call Start multiple times; subsequent calls are no-ops.
func (a *AndroidAgent) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.running {
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	a.running = true
	a.status.Running = true

	a.logger.Info("android agent started",
		"agent_id", a.config.AgentID,
		"model", a.config.Model,
	)

	// Run agent event loop in background goroutine
	go a.run(ctx)

	return nil
}

// Stop gracefully shuts down the agent foreground service.
func (a *AndroidAgent) Stop() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if !a.running {
		return nil
	}

	if a.cancel != nil {
		a.cancel()
		a.cancel = nil
	}
	a.running = false
	a.status.Running = false

	a.logger.Info("android agent stopped", "agent_id", a.config.AgentID)
	return nil
}

// HandleIntent processes an Android intent received by the host service.
// action is the Intent action string (e.g. "com.evoclaw.ACTION_SEND_MESSAGE").
// extras contains the Intent extras as a string map.
func (a *AndroidAgent) HandleIntent(action string, extras map[string]string) error {
	a.mu.Lock()
	running := a.running
	a.mu.Unlock()

	if !running {
		return fmt.Errorf("android: agent is not running")
	}

	a.logger.Info("handling android intent", "action", action, "extras_count", len(extras))

	switch action {
	case "com.evoclaw.ACTION_SEND_MESSAGE":
		msg, ok := extras["message"]
		if !ok {
			return fmt.Errorf("android: intent missing 'message' extra")
		}
		return a.handleMessage(msg)

	case "com.evoclaw.ACTION_STOP":
		return a.Stop()

	default:
		a.logger.Warn("unknown android intent action", "action", action)
	}

	return nil
}

// GetStatus returns the current agent status as a JSON string.
// Safe to call from the Android main thread.
func (a *AndroidAgent) GetStatus() string {
	a.mu.Lock()
	defer a.mu.Unlock()

	data, _ := json.Marshal(a.status)
	return string(data)
}

// handleMessage processes an incoming message directed at the agent.
func (a *AndroidAgent) handleMessage(msg string) error {
	a.logger.Info("processing message", "len", len(msg))
	// TODO: forward to core agent router
	return nil
}

// run is the background event loop running until ctx is cancelled.
func (a *AndroidAgent) run(ctx context.Context) {
	<-ctx.Done()
	a.logger.Info("android agent event loop exited", "agent_id", a.config.AgentID)
}
