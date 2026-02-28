//go:build !(js && wasm)

// Package wasm provides an EvoClaw agent adapter for WebAssembly deployment.
// On non-WASM builds, this stub allows host compilation and testing.
package wasm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
)

// AgentConfig holds configuration for a WASM EvoClaw agent.
type AgentConfig struct {
	AgentID  string `json:"agent_id"`
	APIKey   string `json:"api_key"`
	Model    string `json:"model"`
	LogLevel string `json:"log_level"`
}

// WASMAgent wraps core agent functionality for WASM deployment.
type WASMAgent struct {
	config  AgentConfig
	logger  *slog.Logger
	cancel  context.CancelFunc
	mu      sync.Mutex
	running bool
}

// NewWASMAgent creates a new WASMAgent (stub for non-WASM builds).
func NewWASMAgent(config AgentConfig) (*WASMAgent, error) {
	if config.AgentID == "" {
		return nil, fmt.Errorf("wasm: AgentID is required")
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

	return &WASMAgent{
		config: config,
		logger: slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})),
	}, nil
}

// Start starts the stub agent (no JS interop on non-WASM).
func (a *WASMAgent) Start(_ context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.running {
		return nil
	}
	a.running = true
	a.logger.Info("wasm stub agent started", "agent_id", a.config.AgentID)
	return nil
}

// Stop stops the stub agent.
func (a *WASMAgent) Stop() error {
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
	return nil
}

// SendMessage processes a message and returns a JSON response.
func (a *WASMAgent) SendMessage(msg string) string {
	a.mu.Lock()
	running := a.running
	a.mu.Unlock()

	if !running {
		return jsonError("agent not running")
	}

	resp := map[string]string{
		"status":   "ok",
		"agent_id": a.config.AgentID,
		"echo":     msg,
	}
	data, _ := json.Marshal(resp)
	return string(data)
}

// GetStatus returns the current agent status as JSON.
func (a *WASMAgent) GetStatus() string {
	a.mu.Lock()
	defer a.mu.Unlock()

	status := map[string]interface{}{
		"running":  a.running,
		"agent_id": a.config.AgentID,
		"model":    a.config.Model,
		"platform": "stub",
	}
	data, _ := json.Marshal(status)
	return string(data)
}

func jsonError(msg string) string {
	data, _ := json.Marshal(map[string]string{"error": msg})
	return string(data)
}
