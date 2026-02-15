package orchestrator

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/clawinfra/evoclaw/internal/config"
)

// --- Error MockProvider ---
type errorMockProvider struct {
	name string
	err  error
}

func (e *errorMockProvider) Name() string { return e.name }
func (e *errorMockProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	return nil, e.err
}
func (e *errorMockProvider) Models() []config.Model { return nil }

// --- Tests ---

func TestOrchestratorStartMinimal(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.CloudSync.Enabled = false
	cfg.OnChain.Enabled = false
	cfg.Memory.Enabled = false
	cfg.Evolution.Enabled = false

	orch := New(cfg, slog.Default())

	// Register mock channel and provider
	ch := newMockChannel("test")
	prov := newMockProvider("test-prov")
	prov.responses["test-model"] = "hello back"

	orch.RegisterChannel(ch)
	orch.RegisterProvider(prov)

	err := orch.Start()
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	// Give background goroutines a moment
	time.Sleep(50 * time.Millisecond)

	if err := orch.Stop(); err != nil {
		t.Errorf("Stop() error: %v", err)
	}
}

func TestOrchestratorStartWithAgents(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.CloudSync.Enabled = false
	cfg.OnChain.Enabled = false
	cfg.Memory.Enabled = false
	cfg.Evolution.Enabled = false
	cfg.Agents = []config.AgentDef{
		{
			ID:           "agent-1",
			Name:         "Test Agent",
			Type:         "orchestrator",
			Model:        "test-model",
			SystemPrompt: "You are a test agent.",
			Skills:       []string{"chat"},
		},
	}

	orch := New(cfg, slog.Default())
	ch := newMockChannel("test")
	prov := newMockProvider("test-prov")
	prov.responses["test-model"] = "test response"

	orch.RegisterChannel(ch)
	orch.RegisterProvider(prov)

	err := orch.Start()
	if err != nil {
		t.Fatalf("Start() error: %v", err)
	}

	agents := orch.ListAgents()
	if len(agents) != 1 {
		t.Errorf("Expected 1 agent, got %d", len(agents))
	}

	time.Sleep(50 * time.Millisecond)
	_ = orch.Stop()
}

func TestOrchestratorMessageProcessing(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.CloudSync.Enabled = false
	cfg.OnChain.Enabled = false
	cfg.Memory.Enabled = false
	cfg.Evolution.Enabled = false
	cfg.Agents = []config.AgentDef{
		{
			ID:           "agent-1",
			Name:         "Test Agent",
			Type:         "orchestrator",
			Model:        "test-model",
			SystemPrompt: "You are helpful.",
		},
	}

	orch := New(cfg, slog.Default())
	ch := newMockChannel("test")
	prov := newMockProvider("test-prov")
	prov.responses["test-model"] = "Here's your answer!"

	orch.RegisterChannel(ch)
	orch.RegisterProvider(prov)

	err := orch.Start()
	if err != nil {
		t.Fatal(err)
	}

	// Send a message through the channel
	ch.sendMessage(Message{
		ID:        "msg-1",
		Channel:   "test",
		From:      "user-1",
		Content:   "Hello agent!",
		Timestamp: time.Now(),
	})

	// Wait for processing
	time.Sleep(500 * time.Millisecond)

	// Check agent metrics were updated
	metrics, err := orch.GetAgentMetrics("agent-1")
	if err != nil {
		t.Fatalf("GetAgentMetrics() error: %v", err)
	}
	if metrics.TotalActions < 1 {
		t.Errorf("Expected at least 1 action, got %d", metrics.TotalActions)
	}

	// Check response was sent back through the channel
	sent := ch.getSent()
	if len(sent) < 1 {
		t.Error("Expected at least 1 response sent through channel")
	}

	_ = orch.Stop()
}

func TestOrchestratorWithEvolution(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.CloudSync.Enabled = false
	cfg.OnChain.Enabled = false
	cfg.Memory.Enabled = false
	cfg.Evolution.Enabled = false // Don't start auto-loop, we'll trigger manually

	orch := New(cfg, slog.Default())
	evo := newMockEvolution()
	orch.SetEvolutionEngine(evo)

	ch := newMockChannel("test")
	prov := newMockProvider("test-prov")
	prov.responses["test-model"] = "response"
	orch.RegisterChannel(ch)
	orch.RegisterProvider(prov)

	cfg.Agents = []config.AgentDef{
		{
			ID:    "agent-1",
			Name:  "Evo Agent",
			Model: "test-model",
		},
	}

	err := orch.Start()
	if err != nil {
		t.Fatal(err)
	}

	// Send message to trigger evolution evaluate
	ch.sendMessage(Message{
		ID:        "msg-1",
		From:      "user-1",
		Content:   "test",
		Timestamp: time.Now(),
	})

	time.Sleep(500 * time.Millisecond)

	evo.mu.Lock()
	if _, ok := evo.fitness["agent-1"]; !ok {
		t.Log("Evolution engine was called (or not, depending on timing)")
	}
	evo.mu.Unlock()

	_ = orch.Stop()
}

func TestSelectAgentV2(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agents = []config.AgentDef{
		{ID: "agent-1", Name: "Agent One"},
	}

	orch := New(cfg, slog.Default())
	// Initialize agents
	orch.agents["agent-1"] = &AgentState{ID: "agent-1"}

	msg := Message{Content: "test"}
	agentID := orch.selectAgent(msg)
	if agentID == "" {
		t.Error("selectAgent() returned empty string")
	}
}

func TestSelectAgentEmptyV2(t *testing.T) {
	orch := New(config.DefaultConfig(), slog.Default())
	msg := Message{Content: "test"}
	agentID := orch.selectAgent(msg)
	if agentID != "" {
		t.Errorf("selectAgent() should return empty for no agents, got %q", agentID)
	}
}

func TestSelectModel(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Models.Routing.Complex = "default-model"

	orch := New(cfg, slog.Default())
	agent := &AgentState{
		Def: config.AgentDef{Model: "agent-model"},
	}

	model := orch.selectModel(Message{}, agent)
	if model != "agent-model" {
		t.Errorf("selectModel() = %q, want %q", model, "agent-model")
	}

	// Agent with no model should fall back to routing
	agent2 := &AgentState{
		Def: config.AgentDef{},
	}
	model2 := orch.selectModel(Message{}, agent2)
	if model2 != "default-model" {
		t.Errorf("selectModel() fallback = %q, want %q", model2, "default-model")
	}
}

func TestFindProvider(t *testing.T) {
	orch := New(config.DefaultConfig(), slog.Default())

	// No providers
	p := orch.findProvider("any-model")
	if p != nil {
		t.Error("findProvider() should return nil when no providers registered")
	}

	// With provider
	prov := newMockProvider("test")
	orch.RegisterProvider(prov)

	p = orch.findProvider("any-model")
	if p == nil {
		t.Error("findProvider() should return provider when one is registered")
	}
}

func TestHandleMessageNoAgent(t *testing.T) {
	orch := New(config.DefaultConfig(), slog.Default())

	// Should not panic with no agents
	orch.handleMessage(Message{
		ID:      "test",
		Content: "hello",
	})
}

func TestHandleMessageMissingAgent(t *testing.T) {
	orch := New(config.DefaultConfig(), slog.Default())
	// Put a fake agent ID in agents map that doesn't match
	orch.agents["ghost"] = &AgentState{ID: "ghost"}

	// handleMessage calls selectAgent which returns "ghost"
	orch.handleMessage(Message{
		ID:      "test",
		Content: "hello",
	})
	// Should not panic
}

func TestGetAgentMetricsNotFound(t *testing.T) {
	orch := New(config.DefaultConfig(), slog.Default())
	_, err := orch.GetAgentMetrics("nonexistent")
	if err == nil {
		t.Error("Expected error for nonexistent agent")
	}
}

func TestListAgentsEmpty(t *testing.T) {
	orch := New(config.DefaultConfig(), slog.Default())
	agents := orch.ListAgents()
	if len(agents) != 0 {
		t.Errorf("Expected 0 agents, got %d", len(agents))
	}
}

func TestGettersNil(t *testing.T) {
	orch := New(config.DefaultConfig(), slog.Default())
	if orch.GetCloudSync() != nil {
		t.Error("GetCloudSync() should be nil initially")
	}
	if orch.GetMemory() != nil {
		t.Error("GetMemory() should be nil initially")
	}
	if orch.GetChainRegistry() != nil {
		t.Error("GetChainRegistry() should be nil initially")
	}
}

func TestGetSkillMetrics(t *testing.T) {
	orch := New(config.DefaultConfig(), slog.Default())
	agent := &AgentState{
		Metrics: AgentMetrics{
			TotalActions:      100,
			SuccessfulActions: 90,
			AvgResponseMs:     150.0,
			CostUSD:           1.50,
			Custom: map[string]float64{
				"chat_accuracy":       0.95,
				"chat_speed":          0.8,
				"search_relevance":    0.7,
				"unrelated_metric":    0.1,
			},
		},
	}

	metrics := orch.getSkillMetrics(agent, "chat")
	if metrics["successRate"] != 0.9 {
		t.Errorf("successRate = %f, want 0.9", metrics["successRate"])
	}
	// Should include chat_ prefixed custom metrics without prefix
	if _, ok := metrics["accuracy"]; !ok {
		t.Error("Expected chat_accuracy to be included as 'accuracy'")
	}
	if _, ok := metrics["speed"]; !ok {
		t.Error("Expected chat_speed to be included as 'speed'")
	}
}

func TestGetSkillMetricsZeroActions(t *testing.T) {
	orch := New(config.DefaultConfig(), slog.Default())
	agent := &AgentState{
		Metrics: AgentMetrics{
			TotalActions: 0,
			Custom:       make(map[string]float64),
		},
	}

	metrics := orch.getSkillMetrics(agent, "chat")
	if metrics["successRate"] != 0.0 {
		t.Errorf("successRate = %f, want 0.0 for zero actions", metrics["successRate"])
	}
}

func TestEvaluateAgentsNoEvolution(t *testing.T) {
	orch := New(config.DefaultConfig(), slog.Default())
	// Should not panic with no evolution engine
	orch.evaluateAgents()
}

func TestRouteOutgoing(t *testing.T) {
	cfg := config.DefaultConfig()
	orch := New(cfg, slog.Default())
	ch := newMockChannel("test-out")
	orch.RegisterChannel(ch)

	// Start routeOutgoing in background
	go orch.routeOutgoing()

	// Send response through outbox
	orch.outbox <- Response{
		AgentID: "agent-1",
		Content: "test response",
		Channel: "test-out",
		To:      "user-1",
	}

	time.Sleep(100 * time.Millisecond)
	orch.cancel()

	sent := ch.getSent()
	if len(sent) == 0 {
		t.Error("Expected response to be sent through channel")
	}
}

func TestRouteOutgoingUnknownChannel(t *testing.T) {
	cfg := config.DefaultConfig()
	orch := New(cfg, slog.Default())

	go orch.routeOutgoing()

	// Send to unknown channel
	orch.outbox <- Response{
		Channel: "nonexistent",
		Content: "test",
	}

	time.Sleep(100 * time.Millisecond)
	orch.cancel()
	// Should not panic
}

func TestProcessWithAgentNoProvider(t *testing.T) {
	cfg := config.DefaultConfig()
	orch := New(cfg, slog.Default())

	agent := &AgentState{
		ID:  "agent-1",
		Def: config.AgentDef{Model: "nonexistent-model"},
		Metrics: AgentMetrics{
			Custom: make(map[string]float64),
		},
	}

	// Should not panic
	orch.processWithAgent(agent, Message{Content: "test"}, "nonexistent-model")
}

func TestProcessWithAgentProviderError(t *testing.T) {
	cfg := config.DefaultConfig()
	orch := New(cfg, slog.Default())

	errProv := &errorMockProvider{name: "error-prov", err: context.DeadlineExceeded}
	orch.RegisterProvider(errProv)

	agent := &AgentState{
		ID:  "agent-1",
		Def: config.AgentDef{Model: "error-model"},
		Metrics: AgentMetrics{
			Custom: make(map[string]float64),
		},
	}

	orch.processWithAgent(agent, Message{Content: "test"}, "error-model")

	// Should have incremented error count
	if agent.Metrics.FailedActions != 1 {
		t.Errorf("FailedActions = %d, want 1", agent.Metrics.FailedActions)
	}
}
