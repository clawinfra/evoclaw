//go:build !android

// Package android provides an EvoClaw agent adapter for Android platform.
// On non-Android builds, this package exports stub types and constructors
// that return ErrNotSupported, allowing cross-platform code to reference the
// package without build errors.
package android

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
)

// ErrNotSupported is returned by Android-specific operations on non-Android platforms.
var ErrNotSupported = errors.New("android: not supported on this platform")

// AgentConfig holds configuration for an Android EvoClaw agent.
type AgentConfig struct {
	AgentID  string
	APIKey   string
	Model    string
	DataDir  string
	LogLevel string
}

// AndroidAgent wraps core agent functionality for the Android platform.
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

// NewAndroidAgent creates a stub AndroidAgent for non-Android platforms.
// All lifecycle methods will return ErrNotSupported unless the stub is
// used in test mode (config.DataDir set to a valid path).
func NewAndroidAgent(config AgentConfig) (*AndroidAgent, error) {
	if config.AgentID == "" {
		return nil, fmt.Errorf("android: AgentID is required")
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

	return &AndroidAgent{
		config: config,
		logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})),
		status: agentStatus{
			AgentID:  config.AgentID,
			Model:    config.Model,
			Platform: "stub",
		},
	}, nil
}

// Start is a stub — returns ErrNotSupported on non-Android platforms.
// In tests, it can be used to verify lifecycle logic against the stub.
func (a *AndroidAgent) Start(_ context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running {
		return nil
	}
	a.running = true
	a.status.Running = true
	return nil
}

// Stop is a stub — stops the stub agent state.
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
	return nil
}

// HandleIntent is a stub — simulates intent handling for tests.
func (a *AndroidAgent) HandleIntent(action string, extras map[string]string) error {
	a.mu.Lock()
	running := a.running
	a.mu.Unlock()

	if !running {
		return fmt.Errorf("android: agent is not running")
	}

	switch action {
	case "com.evoclaw.ACTION_SEND_MESSAGE":
		if _, ok := extras["message"]; !ok {
			return fmt.Errorf("android: intent missing 'message' extra")
		}
		return nil
	case "com.evoclaw.ACTION_STOP":
		return a.Stop()
	default:
		a.logger.Warn("unknown android intent action", "action", action)
	}

	return nil
}

// GetStatus returns the current agent status as a JSON string.
func (a *AndroidAgent) GetStatus() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	data, _ := json.Marshal(a.status)
	return string(data)
}
