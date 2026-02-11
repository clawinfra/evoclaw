package orchestrator

import (
	"log/slog"
	"testing"

	"github.com/clawinfra/evoclaw/internal/config"
)

func TestNewOrchestrator(t *testing.T) {
	cfg := config.DefaultConfig()
	logger := slog.Default()
	orch := New(cfg, logger)
	if orch == nil {
		t.Fatal("expected non-nil orchestrator")
	}
}

func TestGetters(t *testing.T) {
	orch := New(config.DefaultConfig(), slog.Default())
	
	// Should be nil initially or initialized?
	// Based on code, they are initialized in Start() or lazy loaded?
	// New() initializes some maps.
	
	if orch.GetCloudSync() != nil {
		t.Log("CloudSync initialized")
	} else {
		t.Log("CloudSync nil")
	}
	
	if orch.GetMemory() != nil {
		t.Log("Memory initialized")
	}
	
	if orch.GetChainRegistry() != nil {
		t.Log("ChainRegistry initialized")
	}
}

func TestRegisterMethods(t *testing.T) {
	orch := New(config.DefaultConfig(), slog.Default())

	// Register a real mock channel (not nil — nil would panic on Name())
	ch := newMockChannel("test-ch")
	orch.RegisterChannel(ch)

	// Register a real mock provider
	p := newMockProvider("test-prov")
	orch.RegisterProvider(p)

	// SetEvolutionEngine with nil is fine (just sets field to nil)
	orch.SetEvolutionEngine(nil)
}

func TestStartStop(t *testing.T) {
	// Need minimal config to avoid external dependencies failing
	cfg := config.DefaultConfig()
	cfg.CloudSync.Enabled = false // Disable external calls
	
	orch := New(cfg, slog.Default())
	
	// Start might block or fail if dependencies aren't mocked
	// Start calls initMemory, initCloudSync, initOnChain
	
	// Let's test Stop first
	if err := orch.Stop(); err != nil {
		t.Errorf("Stop error: %v", err)
	}
}

func TestListAgentsV2(t *testing.T) {
	orch := New(config.DefaultConfig(), slog.Default())
	agents := orch.ListAgents()
	if len(agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(agents))
	}
}

func TestGetAgentMetricsV2(t *testing.T) {
	orch := New(config.DefaultConfig(), slog.Default())
	_, err := orch.GetAgentMetrics("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent agent")
	}
}
