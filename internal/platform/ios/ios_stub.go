//go:build !ios

// Package ios provides an EvoClaw agent adapter for iOS platform.
// On non-iOS builds, this package exports stub types and constructors
// that allow cross-platform code to reference the package without build errors.
package ios

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
)

// AgentConfig holds configuration for an iOS EvoClaw agent.
type AgentConfig struct {
	AgentID   string
	APIKey    string
	Model     string
	DataDir   string
	LogLevel  string
	URLScheme string
}

// IOSAgent wraps core agent functionality for the iOS platform.
type IOSAgent struct {
	config  AgentConfig
	logger  *slog.Logger
	cancel  context.CancelFunc
	mu      sync.Mutex
	running bool
	status  agentStatus
}

type agentStatus struct {
	Running   bool   `json:"running"`
	AgentID   string `json:"agent_id"`
	Model     string `json:"model"`
	Platform  string `json:"platform"`
	URLScheme string `json:"url_scheme,omitempty"`
}

// NewIOSAgent creates a stub IOSAgent for non-iOS platforms.
func NewIOSAgent(config AgentConfig) (*IOSAgent, error) {
	if config.AgentID == "" {
		return nil, fmt.Errorf("ios: AgentID is required")
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

	return &IOSAgent{
		config: config,
		logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})),
		status: agentStatus{
			AgentID:   config.AgentID,
			Model:     config.Model,
			Platform:  "stub",
			URLScheme: config.URLScheme,
		},
	}, nil
}

// Start is a stub — simulates agent start for tests.
func (a *IOSAgent) Start(_ context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.running {
		return nil
	}
	a.running = true
	a.status.Running = true
	return nil
}

// Stop is a stub — simulates agent stop for tests.
func (a *IOSAgent) Stop() error {
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

// HandleURLScheme is a stub — simulates URL scheme handling for tests.
func (a *IOSAgent) HandleURLScheme(url string) error {
	a.mu.Lock()
	running := a.running
	a.mu.Unlock()

	if !running {
		return fmt.Errorf("ios: agent is not running")
	}
	if url == "" {
		return fmt.Errorf("ios: empty URL")
	}
	return nil
}

// PerformBackgroundFetch is a stub — returns "noData" on non-iOS.
func (a *IOSAgent) PerformBackgroundFetch() string {
	a.mu.Lock()
	running := a.running
	a.mu.Unlock()
	if !running {
		return "failed"
	}
	return "noData"
}

// GetStatus returns the current agent status as a JSON string.
func (a *IOSAgent) GetStatus() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	data, _ := json.Marshal(a.status)
	return string(data)
}
