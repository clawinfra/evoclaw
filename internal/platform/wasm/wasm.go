//go:build js && wasm

// Package wasm provides an EvoClaw agent adapter for WebAssembly deployment.
// It enables EvoClaw agents to compile to WASM and run in browsers or
// edge runtimes (Cloudflare Workers, Deno Deploy, etc.) via syscall/js interop.
//
// # Building for WASM
//
//	GOOS=js GOARCH=wasm go build -o dist/evoclaw.wasm ./internal/platform/wasm/cmd/
//	cp $(go env GOROOT)/misc/wasm/wasm_exec.js dist/
//
// Or use the provided build script:
//
//	bash scripts/build-wasm.sh
//
// # JavaScript API
//
// After loading the WASM module, the following functions are available globally:
//
//	evoclaw.RunAgent(configJSON)  → status JSON
//	evoclaw.SendMessage(msgJSON)  → response JSON
//	evoclaw.GetStatus()           → status JSON
//
// See examples/wasm/index.html for a complete example.
package wasm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"syscall/js"
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

var globalAgent *WASMAgent

// NewWASMAgent creates a new WASMAgent with the given configuration.
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
		logger: slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})),
	}, nil
}

// Start starts the WASM agent event loop.
func (a *WASMAgent) Start(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.running {
		return nil
	}

	ctx, cancel := context.WithCancel(ctx)
	a.cancel = cancel
	a.running = true

	a.logger.Info("wasm agent started", "agent_id", a.config.AgentID)

	// Register JS global functions
	js.Global().Set("evoclaw", js.ValueOf(map[string]interface{}{
		"RunAgent":    js.FuncOf(jsRunAgent),
		"SendMessage": js.FuncOf(jsSendMessage),
		"GetStatus":   js.FuncOf(jsGetStatus),
	}))

	go func() {
		<-ctx.Done()
		a.logger.Info("wasm agent stopped")
	}()

	return nil
}

// Stop gracefully shuts down the WASM agent.
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

// SendMessage sends a message to the agent and returns a JSON response.
func (a *WASMAgent) SendMessage(msg string) string {
	a.mu.Lock()
	running := a.running
	a.mu.Unlock()

	if !running {
		return jsonError("agent not running")
	}

	a.logger.Info("wasm message received", "len", len(msg))

	resp := map[string]string{
		"status":   "ok",
		"agent_id": a.config.AgentID,
		"echo":     msg, // TODO: forward to core agent router
	}
	data, _ := json.Marshal(resp)
	return string(data)
}

// GetStatus returns the current agent status as a JSON string.
func (a *WASMAgent) GetStatus() string {
	a.mu.Lock()
	defer a.mu.Unlock()

	status := map[string]interface{}{
		"running":  a.running,
		"agent_id": a.config.AgentID,
		"model":    a.config.Model,
		"platform": "wasm",
	}
	data, _ := json.Marshal(status)
	return string(data)
}

// JS-exported function wrappers

func jsRunAgent(this js.Value, args []js.Value) interface{} {
	if len(args) < 1 {
		return jsonError("missing configJSON argument")
	}
	configJSON := args[0].String()

	var config AgentConfig
	if err := json.Unmarshal([]byte(configJSON), &config); err != nil {
		return jsonError("invalid config JSON: " + err.Error())
	}

	agent, err := NewWASMAgent(config)
	if err != nil {
		return jsonError(err.Error())
	}

	globalAgent = agent
	if err := agent.Start(context.Background()); err != nil {
		return jsonError(err.Error())
	}

	return agent.GetStatus()
}

func jsSendMessage(this js.Value, args []js.Value) interface{} {
	if globalAgent == nil {
		return jsonError("agent not initialized, call RunAgent first")
	}
	if len(args) < 1 {
		return jsonError("missing msg argument")
	}
	return globalAgent.SendMessage(args[0].String())
}

func jsGetStatus(this js.Value, args []js.Value) interface{} {
	if globalAgent == nil {
		return jsonError("agent not initialized")
	}
	return globalAgent.GetStatus()
}

func jsonError(msg string) string {
	data, _ := json.Marshal(map[string]string{"error": msg})
	return string(data)
}
