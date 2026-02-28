//go:build ios

// Package ios provides an EvoClaw agent adapter for iOS platform.
// It enables EvoClaw agents to run as iOS background tasks and handle
// custom URL scheme callbacks via gomobile bindings.
//
// # Building for iOS
//
// Prerequisites:
//
//	go install golang.org/x/mobile/cmd/gomobile@latest
//	gomobile init
//	# Xcode and iOS SDK required (macOS only)
//
// Build XCFramework:
//
//	gomobile bind -target ios -o EvoClaw.xcframework github.com/clawinfra/evoclaw/internal/platform/ios
//
// The generated XCFramework can be added to Xcode projects. Only interfaces
// and primitive types are exported in the gomobile API surface.
//
// # Background Fetch
//
// Register the agent as a background fetch provider in Info.plist:
//
//	<key>BGTaskSchedulerPermittedIdentifiers</key>
//	<array>
//	  <string>com.evoclaw.agent.fetch</string>
//	</array>
//
// Then call PerformBackgroundFetch() from your AppDelegate's
// application(_:performFetchWithCompletionHandler:).
package ios

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
)

// AgentConfig holds configuration for an iOS EvoClaw agent.
type AgentConfig struct {
	// AgentID is the unique identifier for this agent instance.
	AgentID string
	// APIKey is the LLM provider API key.
	APIKey string
	// Model is the LLM model name.
	Model string
	// DataDir is the path to the app's Documents or Application Support directory.
	DataDir string
	// LogLevel controls verbosity: "debug", "info", "warn", "error".
	LogLevel string
	// URLScheme is the custom URL scheme for deep linking (e.g. "evoclaw").
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

// NewIOSAgent creates a new IOSAgent with the given configuration.
func NewIOSAgent(config AgentConfig) (*IOSAgent, error) {
	if config.AgentID == "" {
		return nil, fmt.Errorf("ios: AgentID is required")
	}
	if config.DataDir == "" {
		return nil, fmt.Errorf("ios: DataDir is required")
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
		logger: slog.New(slog.NewJSONHandler(nil, &slog.HandlerOptions{Level: level})),
		status: agentStatus{
			AgentID:   config.AgentID,
			Model:     config.Model,
			Platform:  "ios",
			URLScheme: config.URLScheme,
		},
	}, nil
}

// Start starts the agent as an iOS background task.
// Must be called from the main thread or a background task context.
func (a *IOSAgent) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.running {
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	a.running = true
	a.status.Running = true

	a.logger.Info("ios agent started", "agent_id", a.config.AgentID)
	go a.run(ctx)
	return nil
}

// Stop gracefully shuts down the iOS agent.
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

	a.logger.Info("ios agent stopped", "agent_id", a.config.AgentID)
	return nil
}

// HandleURLScheme processes a custom URL scheme callback.
// url is the full URL string (e.g. "evoclaw://message?text=hello").
// Returns an error if the URL cannot be handled.
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

	a.logger.Info("handling url scheme", "url", url)
	// TODO: parse URL and route to appropriate handler
	return nil
}

// PerformBackgroundFetch executes a background fetch cycle.
// Call this from AppDelegate's performFetchWithCompletionHandler.
// Returns "newData", "noData", or "failed" for the iOS completion handler.
func (a *IOSAgent) PerformBackgroundFetch() string {
	a.mu.Lock()
	running := a.running
	a.mu.Unlock()

	if !running {
		return "failed"
	}

	a.logger.Info("performing background fetch")
	// TODO: implement actual fetch logic
	return "noData"
}

// GetStatus returns the current agent status as a JSON string.
func (a *IOSAgent) GetStatus() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	data, _ := json.Marshal(a.status)
	return string(data)
}

func (a *IOSAgent) run(ctx context.Context) {
	<-ctx.Done()
	a.logger.Info("ios agent event loop exited", "agent_id", a.config.AgentID)
}
