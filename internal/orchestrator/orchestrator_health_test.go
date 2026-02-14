package orchestrator

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/router"
)

// TestHealthRegistryInitialization tests that the health registry is properly initialized
func TestHealthRegistryInitialization(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Port: 8420},
		Memory: config.MemoryConfigSettings{Enabled: false},
		Models: config.ModelsConfig{
			Routing: config.ModelRouting{
				Simple:   "test/simple",
				Complex:  "test/complex",
				Critical: "test/critical",
			},
			Health: config.ModelHealthConfig{
				PersistPath:      t.TempDir() + "/health.json",
				FailureThreshold: 3,
				CooldownMinutes:  5,
			},
		},
		Agents: []config.AgentDef{
			{
				ID:           "test-agent",
				Name:         "Test Agent",
				Type:         "orchestrator",
				Model:        "test/complex",
				SystemPrompt: "You are a test agent",
				Container:    config.ContainerConfig{},
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	orc := New(cfg, logger)

	// Initialize health registry
	err := orc.initHealthRegistry()
	if err != nil {
		t.Fatalf("failed to initialize health registry: %v", err)
	}

	if orc.healthRegistry == nil {
		t.Fatal("health registry not initialized")
	}

	// Check that registry has the correct config
	status := orc.healthRegistry.GetStatus()
	if status == nil {
		t.Fatal("health status is nil")
	}

	t.Log("health registry initialized successfully")
}

// TestHealthRegistryRecordsSuccess tests that successful LLM calls are recorded
func TestHealthRegistryRecordsSuccess(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Port: 8421},
		Memory: config.MemoryConfigSettings{Enabled: false},
		Models: config.ModelsConfig{
			Routing: config.ModelRouting{
				Complex: "test/model",
			},
			Health: config.ModelHealthConfig{
				PersistPath: t.TempDir() + "/health.json",
			},
		},
		Agents: []config.AgentDef{
			{
				ID:           "test-agent",
				Name:         "Test Agent",
				Model:        "test/model",
				SystemPrompt: "test",
				Container:    config.ContainerConfig{},
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	orc := New(cfg, logger)
	if err := orc.initHealthRegistry(); err != nil {
		t.Fatalf("failed to initialize health registry: %v", err)
	}

	// Simulate successful call
	orc.healthRegistry.RecordSuccess("test/model")

	// Verify it was recorded
	status, ok := orc.healthRegistry.GetModelStatus("test/model")
	if !ok {
		t.Fatal("model status not found")
	}

	if status.TotalRequests != 1 {
		t.Errorf("expected 1 request, got %d", status.TotalRequests)
	}

	if status.ConsecutiveFailures != 0 {
		t.Errorf("expected 0 consecutive failures, got %d", status.ConsecutiveFailures)
	}
}

// TestHealthRegistryRecordsFailure tests that failed LLM calls are recorded
func TestHealthRegistryRecordsFailure(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Port: 8422},
		Memory: config.MemoryConfigSettings{Enabled: false},
		Models: config.ModelsConfig{
			Routing: config.ModelRouting{
				Complex: "test/model",
			},
			Health: config.ModelHealthConfig{
				PersistPath:      t.TempDir() + "/health.json",
				FailureThreshold: 3,
			},
		},
		Agents: []config.AgentDef{
			{
				ID:        "test-agent",
				Name:      "Test Agent",
				Model:     "test/model",
				Container: config.ContainerConfig{},
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	orc := New(cfg, logger)
	if err := orc.initHealthRegistry(); err != nil {
		t.Fatalf("failed to initialize health registry: %v", err)
	}

	// Record 3 failures to trigger degraded state
	for i := 0; i < 3; i++ {
		orc.healthRegistry.RecordFailure("test/model", router.ErrRateLimited)
	}

	// Verify degraded state
	status, ok := orc.healthRegistry.GetModelStatus("test/model")
	if !ok {
		t.Fatal("model status not found")
	}

	if status.State != router.StateDegraded {
		t.Errorf("expected degraded state, got %s", status.State)
	}

	if status.ConsecutiveFailures != 3 {
		t.Errorf("expected 3 consecutive failures, got %d", status.ConsecutiveFailures)
	}

	// Verify model is not considered healthy
	if orc.healthRegistry.IsHealthy("test/model") {
		t.Error("expected model to be unhealthy")
	}
}

// TestSelectModelUsesHealthRegistry tests that model selection uses health registry
func TestSelectModelUsesHealthRegistry(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Port: 8423},
		Memory: config.MemoryConfigSettings{Enabled: false},
		Models: config.ModelsConfig{
			Routing: config.ModelRouting{
				Simple:   "test/simple",
				Complex:  "test/complex",
				Critical: "test/critical",
			},
			Health: config.ModelHealthConfig{
				PersistPath:      t.TempDir() + "/health.json",
				FailureThreshold: 3,
			},
		},
		Agents: []config.AgentDef{
			{
				ID:        "test-agent",
				Name:      "Test Agent",
				Model:     "test/complex",
				Container: config.ContainerConfig{},
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	orc := New(cfg, logger)
	if err := orc.initHealthRegistry(); err != nil {
		t.Fatalf("failed to initialize health registry: %v", err)
	}

	agent := &AgentState{
		ID:  "test-agent",
		Def: cfg.Agents[0],
	}

	msg := Message{
		ID:        "test-msg",
		Channel:   "test",
		From:      "user",
		Content:   "test message",
		Timestamp: time.Now(),
	}

	// Initially should return preferred model
	model := orc.selectModel(msg, agent)
	if model != "test/complex" {
		t.Errorf("expected test/complex, got %s", model)
	}

	// Degrade the preferred model
	for i := 0; i < 3; i++ {
		orc.healthRegistry.RecordFailure("test/complex", router.ErrRateLimited)
	}

	// Now should fallback to simple model
	model = orc.selectModel(msg, agent)
	if model != "test/simple" {
		t.Errorf("expected test/simple fallback, got %s", model)
	}
}

// TestHealthRegistryPersistence tests that health state persists correctly
func TestHealthRegistryPersistence(t *testing.T) {
	persistPath := t.TempDir() + "/health.json"

	cfg := &config.Config{
		Server: config.ServerConfig{Port: 8424},
		Memory: config.MemoryConfigSettings{Enabled: false},
		Models: config.ModelsConfig{
			Routing: config.ModelRouting{
				Complex: "test/model",
			},
			Health: config.ModelHealthConfig{
				PersistPath:      persistPath,
				FailureThreshold: 3,
			},
		},
		Agents: []config.AgentDef{
			{
				ID:        "test-agent",
				Name:      "Test Agent",
				Container: config.ContainerConfig{},
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	// Create first instance and record some state
	orc1 := New(cfg, logger)
	if err := orc1.initHealthRegistry(); err != nil {
		t.Fatalf("failed to initialize health registry: %v", err)
	}

	// Record some activity
	orc1.healthRegistry.RecordSuccess("test/model")
	orc1.healthRegistry.RecordFailure("test/model", router.ErrTimeout)

	// Persist
	if err := orc1.healthRegistry.Persist(); err != nil {
		t.Fatalf("failed to persist: %v", err)
	}

	// Create second instance and verify state loaded
	orc2 := New(cfg, logger)
	if err := orc2.initHealthRegistry(); err != nil {
		t.Fatalf("failed to initialize second health registry: %v", err)
	}

	status, ok := orc2.healthRegistry.GetModelStatus("test/model")
	if !ok {
		t.Fatal("model status not found after reload")
	}

	if status.TotalRequests != 2 {
		t.Errorf("expected 2 requests after reload, got %d", status.TotalRequests)
	}

	if status.TotalFailures != 1 {
		t.Errorf("expected 1 failure after reload, got %d", status.TotalFailures)
	}
}

// TestHealthRegistryAutoRecovery tests that models recover after cooldown
func TestHealthRegistryAutoRecovery(t *testing.T) {
	cfg := &config.Config{
		Server: config.ServerConfig{Port: 8425},
		Memory: config.MemoryConfigSettings{Enabled: false},
		Models: config.ModelsConfig{
			Routing: config.ModelRouting{
				Complex: "test/model",
			},
			Health: config.ModelHealthConfig{
				PersistPath:      t.TempDir() + "/health.json",
				FailureThreshold: 3,
				CooldownMinutes:  0, // Will be interpreted as very short
			},
		},
		Agents: []config.AgentDef{
			{
				ID:        "test-agent",
				Name:      "Test Agent",
				Container: config.ContainerConfig{},
			},
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	orc := New(cfg, logger)
	if err := orc.initHealthRegistry(); err != nil {
		t.Fatalf("failed to initialize health registry: %v", err)
	}

	// Degrade the model
	for i := 0; i < 3; i++ {
		orc.healthRegistry.RecordFailure("test/model", router.ErrRateLimited)
	}

	// Verify degraded state is recorded
	status, ok := orc.healthRegistry.GetModelStatus("test/model")
	if !ok {
		t.Fatal("model status not found")
	}

	if status.State != router.StateDegraded {
		t.Errorf("expected degraded state, got %s", status.State)
	}

	// Manually reset the model to test recovery capability
	orc.healthRegistry.ResetModel("test/model")

	// Verify it's now healthy
	if !orc.healthRegistry.IsHealthy("test/model") {
		t.Error("expected model to be healthy after reset")
	}

	status, ok = orc.healthRegistry.GetModelStatus("test/model")
	if !ok {
		t.Fatal("model status not found after reset")
	}

	if status.State != router.StateHealthy {
		t.Errorf("expected healthy state after reset, got %s", status.State)
	}

	t.Log("model can be manually reset to healthy state")
}
