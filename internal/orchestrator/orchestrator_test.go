package orchestrator

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/clawinfra/evoclaw/internal/config"
)

// Mock Channel
type mockChannel struct {
	name    string
	rcvChan chan Message
	sent    []Response
	mu      sync.Mutex
	started bool
	stopped bool
}

func newMockChannel(name string) *mockChannel {
	return &mockChannel{
		name:    name,
		rcvChan: make(chan Message, 100),
		sent:    []Response{},
	}
}

func (m *mockChannel) Name() string {
	return m.name
}

func (m *mockChannel) Start(ctx context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.started = true
	return nil
}

func (m *mockChannel) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopped = true
	close(m.rcvChan)
	return nil
}

func (m *mockChannel) Send(ctx context.Context, msg Response) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = append(m.sent, msg)
	return nil
}

func (m *mockChannel) Receive() <-chan Message {
	return m.rcvChan
}

func (m *mockChannel) sendMessage(msg Message) {
	m.rcvChan <- msg
}

func (m *mockChannel) getSent() []Response {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Response{}, m.sent...)
}

// Mock ModelProvider
type mockProvider struct {
	name      string
	responses map[string]string // model -> response
	calls     int
	mu        sync.Mutex
}

func newMockProvider(name string) *mockProvider {
	return &mockProvider{
		name:      name,
		responses: make(map[string]string),
	}
}

func (m *mockProvider) Name() string {
	return m.name
}

func (m *mockProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.calls++
	
	response := m.responses[req.Model]
	if response == "" {
		response = "mock response"
	}
	
	return &ChatResponse{
		Content:      response,
		Model:        req.Model,
		TokensInput:  100,
		TokensOutput: 50,
		FinishReason: "stop",
	}, nil
}

func (m *mockProvider) Models() []config.Model {
	return []config.Model{
		{ID: "mock-model-1", Name: m.name, ContextWindow: 4096},
		{ID: "mock-model-2", Name: m.name, ContextWindow: 8192},
	}
}

func (m *mockProvider) setResponse(model, response string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.responses[model] = response
}

func (m *mockProvider) getCalls() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

// Mock EvolutionEngine
type mockEvolution struct {
	strategies map[string]interface{}
	fitness    map[string]float64
	mutations  int
	mu         sync.Mutex
}

func newMockEvolution() *mockEvolution {
	return &mockEvolution{
		strategies: make(map[string]interface{}),
		fitness:    make(map[string]float64),
	}
}

func (m *mockEvolution) GetStrategy(agentID string) interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.strategies[agentID]
}

func (m *mockEvolution) Evaluate(agentID string, metrics map[string]float64) float64 {
	m.mu.Lock()
	defer m.mu.Unlock()
	// Use success rate from metrics to compute fitness
	successRate, ok := metrics["successRate"]
	if !ok {
		successRate = 0.5
	}
	m.fitness[agentID] = successRate
	return successRate
}

func (m *mockEvolution) ShouldEvolve(agentID string, minFitness float64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	fitness, ok := m.fitness[agentID]
	if !ok {
		return false
	}
	return fitness < minFitness
}

func (m *mockEvolution) Mutate(agentID string, mutationRate float64) (interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mutations++
	return map[string]float64{"mutated": mutationRate}, nil
}

// Test helpers
func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelError, // Quiet during tests
	}))
}

func testConfig() *config.Config {
	return &config.Config{
		Agents: []config.AgentDef{
			{
				ID:           "test-agent",
				Name:         "test-agent",
				Model:        "mock/mock-model-1",
				SystemPrompt: "You are a test agent",
				Type:         "orchestrator",
			},
		},
		Models: config.ModelsConfig{
			Providers: map[string]config.ProviderConfig{
				"mock": {
					Models: []config.Model{
						{ID: "mock-model-1", Name: "mock", ContextWindow: 4096},
					},
				},
			},
		},
		Evolution: config.EvolutionConfig{
			Enabled:           true,
			EvalIntervalSec:   1,
			MinSamplesForEval: 5,
			MaxMutationRate:   0.1,
		},
	}
}

// Tests
func TestNew(t *testing.T) {
	cfg := testConfig()
	logger := testLogger()
	
	o := New(cfg, logger)
	
	if o == nil {
		t.Fatal("expected orchestrator, got nil")
	}
	if o.cfg != cfg {
		t.Error("config not set correctly")
	}
	if o.logger != logger {
		t.Error("logger not set correctly")
	}
	if o.channels == nil {
		t.Error("channels map not initialized")
	}
	if o.providers == nil {
		t.Error("providers map not initialized")
	}
	if o.agents == nil {
		t.Error("agents map not initialized")
	}
}

func TestRegisterChannel(t *testing.T) {
	o := New(testConfig(), testLogger())
	ch := newMockChannel("test-channel")
	
	o.RegisterChannel(ch)
	
	o.mu.RLock()
	registered := o.channels["test-channel"]
	o.mu.RUnlock()
	
	if registered != ch {
		t.Error("channel not registered correctly")
	}
}

func TestRegisterProvider(t *testing.T) {
	o := New(testConfig(), testLogger())
	p := newMockProvider("test-provider")
	
	o.RegisterProvider(p)
	
	o.mu.RLock()
	registered := o.providers["test-provider"]
	o.mu.RUnlock()
	
	if registered != p {
		t.Error("provider not registered correctly")
	}
}

func TestSetEvolutionEngine(t *testing.T) {
	o := New(testConfig(), testLogger())
	e := newMockEvolution()
	
	o.SetEvolutionEngine(e)
	
	if o.evolution != e {
		t.Error("evolution engine not set correctly")
	}
}

func TestStartAndStop(t *testing.T) {
	o := New(testConfig(), testLogger())
	ch := newMockChannel("mock-channel")
	p := newMockProvider("mock")
	
	o.RegisterChannel(ch)
	o.RegisterProvider(p)
	
	// Start orchestrator
	if err := o.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	
	// Verify channel started
	if !ch.started {
		t.Error("channel not started")
	}
	
	// Check agent is created
	time.Sleep(50 * time.Millisecond)
	o.mu.RLock()
	agent := o.agents["test-agent"]
	o.mu.RUnlock()
	
	if agent == nil {
		t.Fatal("test-agent not created")
	}
	if agent.Status != "idle" {
		t.Errorf("expected idle, got %s", agent.Status)
	}
	
	// Stop orchestrator
	if err := o.Stop(); err != nil {
		t.Fatalf("failed to stop: %v", err)
	}
	
	// Verify channel stopped
	if !ch.stopped {
		t.Error("channel not stopped")
	}
}

func TestMessageRouting(t *testing.T) {
	o := New(testConfig(), testLogger())
	ch := newMockChannel("mock-channel")
	p := newMockProvider("mock")
	p.setResponse("mock/mock-model-1", "Hello from agent!")
	
	o.RegisterChannel(ch)
	o.RegisterProvider(p)
	
	if err := o.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer func() { _ = o.Stop() }()
	
	// Wait for agent to be created
	time.Sleep(50 * time.Millisecond)
	
	// Send a message
	msg := Message{
		ID:        "msg-1",
		Channel:   "mock-channel",
		From:      "user-123",
		To:        "test-agent",
		Content:   "Hello agent!",
		Timestamp: time.Now(),
	}
	
	ch.sendMessage(msg)
	
	// Wait for processing
	time.Sleep(200 * time.Millisecond)
	
	// Check response was sent
	sent := ch.getSent()
	if len(sent) == 0 {
		t.Fatal("no response sent")
	}
	
	resp := sent[0]
	if resp.Content == "" {
		t.Error("response content is empty")
	}
	if resp.AgentID != "test-agent" {
		t.Errorf("expected agent test-agent, got %s", resp.AgentID)
	}
	if resp.To != "user-123" {
		t.Errorf("expected to user-123, got %s", resp.To)
	}
}

func TestListAgents(t *testing.T) {
	o := New(testConfig(), testLogger())
	ch := newMockChannel("mock-channel")
	p := newMockProvider("mock")
	
	o.RegisterChannel(ch)
	o.RegisterProvider(p)
	
	if err := o.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer func() { _ = o.Stop() }()
	
	time.Sleep(50 * time.Millisecond)
	
	agents := o.ListAgents()
	if len(agents) != 1 {
		t.Errorf("expected 1 agent, got %d", len(agents))
	}
	
	agent := agents[0]
	if agent.ID != "test-agent" {
		t.Errorf("expected test-agent, got %s", agent.ID)
	}
}

func TestGetAgentMetrics(t *testing.T) {
	o := New(testConfig(), testLogger())
	ch := newMockChannel("mock-channel")
	p := newMockProvider("mock")
	
	o.RegisterChannel(ch)
	o.RegisterProvider(p)
	
	if err := o.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer func() { _ = o.Stop() }()
	
	time.Sleep(50 * time.Millisecond)
	
	metrics, err := o.GetAgentMetrics("test-agent")
	if err != nil {
		t.Fatalf("failed to get metrics: %v", err)
	}
	
	if metrics == nil {
		t.Fatal("metrics should not be nil")
	}
}

func TestGetAgentMetricsNonexistent(t *testing.T) {
	o := New(testConfig(), testLogger())
	
	_, err := o.GetAgentMetrics("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent agent")
	}
}

func TestSelectAgent(t *testing.T) {
	cfg := testConfig()
	cfg.Agents = append(cfg.Agents, config.AgentDef{
		ID:           "agent-2",
		Name:         "agent-2",
		Model:        "mock/mock-model-1",
		SystemPrompt: "You are agent 2",
		Type:         "orchestrator",
	})
	
	o := New(cfg, testLogger())
	ch := newMockChannel("mock-channel")
	p := newMockProvider("mock")
	
	o.RegisterChannel(ch)
	o.RegisterProvider(p)
	
	if err := o.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer func() { _ = o.Stop() }()
	
	time.Sleep(50 * time.Millisecond)
	
	// Test selection returns an agent
	msg := Message{
		ID:      "msg-1",
		Channel: "mock-channel",
		From:    "user",
		Content: "test",
	}
	
	agentID := o.selectAgent(msg)
	if agentID == "" {
		t.Error("failed to select an agent")
	}
	
	// Verify selected agent exists
	o.mu.RLock()
	agent, ok := o.agents[agentID]
	o.mu.RUnlock()
	
	if !ok {
		t.Errorf("selected agent %s not found", agentID)
	}
	if agent == nil {
		t.Error("agent should not be nil")
	}
}

func TestEvolutionLoop(t *testing.T) {
	cfg := testConfig()
	cfg.Evolution.EvalIntervalSec = 1
	cfg.Evolution.MinSamplesForEval = 2
	cfg.Evolution.MaxMutationRate = 0.9
	cfg.Agents[0].Genome = &config.Genome{
		Skills: map[string]config.SkillGenome{
			"trading": {Enabled: true, Weight: 1.0, Params: map[string]interface{}{}},
		},
	}
	
	o := New(cfg, testLogger())
	ch := newMockChannel("mock-channel")
	p := newMockProvider("mock")
	e := newMockEvolution()
	
	o.RegisterChannel(ch)
	o.RegisterProvider(p)
	o.SetEvolutionEngine(e)
	
	if err := o.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer func() { _ = o.Stop() }()
	
	// Wait for agent creation
	time.Sleep(50 * time.Millisecond)
	
	// Inject some actions to trigger evolution
	o.mu.RLock()
	agent := o.agents["test-agent"]
	o.mu.RUnlock()
	
	if agent == nil {
		t.Fatal("test-agent not found")
	}
	
	// Simulate some activity
	agent.mu.Lock()
	agent.Metrics.TotalActions = 10
	agent.Metrics.SuccessfulActions = 3 // Low success rate to trigger evolution
	agent.mu.Unlock()
	
	// Wait for evolution cycle
	time.Sleep(1500 * time.Millisecond)
	
	// Check that evolution ran
	e.mu.Lock()
	mutations := e.mutations
	e.mu.Unlock()
	if mutations == 0 {
		t.Error("evolution should have triggered mutations")
	}
}

func TestRouteOutgoing_UnknownChannel(t *testing.T) {
	o := New(testConfig(), testLogger())
	ch := newMockChannel("mock-channel")
	
	o.RegisterChannel(ch)
	
	if err := o.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer func() { _ = o.Stop() }()
	
	time.Sleep(50 * time.Millisecond)
	
	// Send response to unknown channel
	o.outbox <- Response{
		Channel: "unknown-channel",
		Content: "test",
		To:      "user-123",
	}
	
	// Wait for processing
	time.Sleep(100 * time.Millisecond)
	
	// Should not crash, just log error
	sent := ch.getSent()
	if len(sent) != 0 {
		t.Error("expected no messages sent to wrong channel")
	}
}

func TestSelectModel_UseAgentModel(t *testing.T) {
	cfg := testConfig()
	cfg.Agents[0].Model = "custom/special-model"
	
	o := New(cfg, testLogger())
	ch := newMockChannel("mock-channel")
	p := newMockProvider("mock")
	
	o.RegisterChannel(ch)
	o.RegisterProvider(p)
	
	if err := o.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer func() { _ = o.Stop() }()
	
	time.Sleep(50 * time.Millisecond)
	
	o.mu.RLock()
	agent := o.agents["test-agent"]
	o.mu.RUnlock()
	
	msg := Message{Content: "test"}
	model := o.selectModel(msg, agent)
	
	if model != "custom/special-model" {
		t.Errorf("expected custom/special-model, got %s", model)
	}
}

func TestSelectModel_UseDefaultComplex(t *testing.T) {
	cfg := testConfig()
	cfg.Agents[0].Model = "" // No model specified
	
	o := New(cfg, testLogger())
	ch := newMockChannel("mock-channel")
	p := newMockProvider("mock")
	
	o.RegisterChannel(ch)
	o.RegisterProvider(p)
	
	if err := o.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer func() { _ = o.Stop() }()
	
	time.Sleep(50 * time.Millisecond)
	
	o.mu.RLock()
	agent := o.agents["test-agent"]
	o.mu.RUnlock()
	
	msg := Message{Content: "test"}
	model := o.selectModel(msg, agent)
	
	if model != cfg.Models.Routing.Complex {
		t.Errorf("expected %s, got %s", cfg.Models.Routing.Complex, model)
	}
}

func TestProcessWithAgent_NoProvider(t *testing.T) {
	o := New(testConfig(), testLogger())
	ch := newMockChannel("mock-channel")
	
	o.RegisterChannel(ch)
	// Don't register any provider
	
	if err := o.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer func() { _ = o.Stop() }()
	
	time.Sleep(50 * time.Millisecond)
	
	o.mu.RLock()
	agent := o.agents["test-agent"]
	o.mu.RUnlock()
	
	msg := Message{
		ID:      "msg-1",
		Channel: "mock-channel",
		From:    "user-123",
		Content: "test",
	}
	
	o.processWithAgent(agent, msg, "nonexistent/model")
	
	time.Sleep(100 * time.Millisecond)
	
	// Should increment error count
	agent.mu.RLock()
	status := agent.Status
	agent.mu.RUnlock()
	
	if status != "idle" {
		t.Errorf("expected idle after error, got %s", status)
	}
}

func TestProcessWithAgent_LLMError(t *testing.T) {
	o := New(testConfig(), testLogger())
	ch := newMockChannel("mock-channel")
	p := &mockProviderWithError{
		mockProvider: newMockProvider("mock"),
		shouldError:  true,
	}
	
	o.RegisterChannel(ch)
	o.RegisterProvider(p)
	
	if err := o.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer func() { _ = o.Stop() }()
	
	time.Sleep(50 * time.Millisecond)
	
	// Send message
	msg := Message{
		ID:      "msg-1",
		Channel: "mock-channel",
		From:    "user-123",
		Content: "test",
	}
	
	ch.sendMessage(msg)
	
	time.Sleep(200 * time.Millisecond)
	
	// Check error was recorded
	o.mu.RLock()
	agent := o.agents["test-agent"]
	o.mu.RUnlock()
	
	agent.mu.RLock()
	errorCount := agent.ErrorCount
	failedActions := agent.Metrics.FailedActions
	agent.mu.RUnlock()
	
	if errorCount == 0 {
		t.Error("expected error count > 0")
	}
	if failedActions == 0 {
		t.Error("expected failed actions > 0")
	}
}

func TestProcessWithAgent_WithEvolution(t *testing.T) {
	o := New(testConfig(), testLogger())
	ch := newMockChannel("mock-channel")
	p := newMockProvider("mock")
	e := newMockEvolution()
	
	o.RegisterChannel(ch)
	o.RegisterProvider(p)
	o.SetEvolutionEngine(e)
	
	if err := o.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer func() { _ = o.Stop() }()
	
	time.Sleep(50 * time.Millisecond)
	
	// Send message
	msg := Message{
		ID:      "msg-1",
		Channel: "mock-channel",
		From:    "user-123",
		Content: "test",
	}
	
	ch.sendMessage(msg)
	
	time.Sleep(200 * time.Millisecond)
	
	// Verify evolution.Evaluate was called
	e.mu.Lock()
	fitnessLen := len(e.fitness)
	e.mu.Unlock()
	if fitnessLen == 0 {
		t.Error("expected evolution.Evaluate to be called")
	}
}

func TestFindProvider_NoProviders(t *testing.T) {
	o := New(testConfig(), testLogger())
	
	provider := o.findProvider("any/model")
	if provider != nil {
		t.Error("expected nil provider when none registered")
	}
}

func TestFindProvider_ReturnsFirst(t *testing.T) {
	o := New(testConfig(), testLogger())
	p := newMockProvider("mock")
	
	o.RegisterProvider(p)
	
	provider := o.findProvider("any/model")
	if provider != p {
		t.Error("expected to return registered provider")
	}
}

func TestHandleMessage_NoAgents(t *testing.T) {
	cfg := testConfig()
	cfg.Agents = []config.AgentDef{} // No agents
	
	o := New(cfg, testLogger())
	ch := newMockChannel("mock-channel")
	
	o.RegisterChannel(ch)
	
	if err := o.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer func() { _ = o.Stop() }()
	
	time.Sleep(50 * time.Millisecond)
	
	msg := Message{
		ID:      "msg-1",
		Channel: "mock-channel",
		From:    "user-123",
		Content: "test",
	}
	
	ch.sendMessage(msg)
	
	time.Sleep(100 * time.Millisecond)
	
	// Should not crash
	sent := ch.getSent()
	if len(sent) != 0 {
		t.Error("expected no responses when no agents")
	}
}

func TestHandleMessage_AgentNotFound(t *testing.T) {
	o := New(testConfig(), testLogger())
	ch := newMockChannel("mock-channel")
	
	o.RegisterChannel(ch)
	
	if err := o.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer func() { _ = o.Stop() }()
	
	time.Sleep(50 * time.Millisecond)
	
	// Manually delete the agent
	o.mu.Lock()
	delete(o.agents, "test-agent")
	o.mu.Unlock()
	
	msg := Message{
		ID:      "msg-1",
		Channel: "mock-channel",
		From:    "user-123",
		Content: "test",
	}
	
	// This will fail because selectAgent returns first agent (now none)
	o.handleMessage(msg)
	
	time.Sleep(100 * time.Millisecond)
	
	// Should not crash
}

func TestEvaluateAgents_NoEvolution(t *testing.T) {
	o := New(testConfig(), testLogger())
	
	// evolution is nil
	o.evaluateAgents()
	
	// Should not crash
}

func TestEvaluateAgents_InsufficientSamples(t *testing.T) {
	cfg := testConfig()
	cfg.Evolution.MinSamplesForEval = 100
	
	o := New(cfg, testLogger())
	ch := newMockChannel("mock-channel")
	p := newMockProvider("mock")
	e := newMockEvolution()
	
	o.RegisterChannel(ch)
	o.RegisterProvider(p)
	o.SetEvolutionEngine(e)
	
	if err := o.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer func() { _ = o.Stop() }()
	
	time.Sleep(50 * time.Millisecond)
	
	// Agent has only a few actions
	o.mu.RLock()
	agent := o.agents["test-agent"]
	o.mu.RUnlock()
	
	agent.mu.Lock()
	agent.Metrics.TotalActions = 5 // Less than MinSamplesForEval
	agent.Metrics.SuccessfulActions = 5
	agent.mu.Unlock()
	
	// Run evaluation
	o.evaluateAgents()
	
	// Should not trigger evolution
	if e.mutations > 0 {
		t.Error("expected no mutations with insufficient samples")
	}
}

func TestEvaluateAgents_HighFitness(t *testing.T) {
	cfg := testConfig()
	cfg.Evolution.MinSamplesForEval = 5
	
	o := New(cfg, testLogger())
	ch := newMockChannel("mock-channel")
	p := newMockProvider("mock")
	e := newMockEvolution()
	
	o.RegisterChannel(ch)
	o.RegisterProvider(p)
	o.SetEvolutionEngine(e)
	
	if err := o.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer func() { _ = o.Stop() }()
	
	time.Sleep(50 * time.Millisecond)
	
	// Agent has good performance
	o.mu.RLock()
	agent := o.agents["test-agent"]
	o.mu.RUnlock()
	
	agent.mu.Lock()
	agent.Metrics.TotalActions = 10
	agent.Metrics.SuccessfulActions = 10 // 100% success rate
	agent.mu.Unlock()
	
	// Run evaluation
	o.evaluateAgents()
	
	// Should not trigger evolution (fitness > 0.6)
	if e.mutations > 0 {
		t.Error("expected no mutations with high fitness")
	}
}

func TestStop_ChannelError(t *testing.T) {
	o := New(testConfig(), testLogger())
	ch := &mockChannelWithError{
		mockChannel: newMockChannel("error-channel"),
		stopError:   true,
	}
	
	o.RegisterChannel(ch)
	
	if err := o.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	
	time.Sleep(50 * time.Millisecond)
	
	// Stop should handle error gracefully
	if err := o.Stop(); err != nil {
		t.Fatalf("Stop returned error: %v", err)
	}
}

// Mock provider with error capability
type mockProviderWithError struct {
	*mockProvider
	shouldError bool
}

func (m *mockProviderWithError) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if m.shouldError {
		return nil, fmt.Errorf("simulated LLM error")
	}
	return m.mockProvider.Chat(ctx, req)
}

// Mock channel with error capability
type mockChannelWithError struct {
	*mockChannel
	stopError bool
}

func (m *mockChannelWithError) Stop() error {
	if m.stopError {
		return fmt.Errorf("simulated stop error")
	}
	return m.mockChannel.Stop()
}
