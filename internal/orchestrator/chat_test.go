package orchestrator

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestChatSync_Success(t *testing.T) {
	o := New(testConfig(), testLogger())
	ch := newMockChannel("mock-channel")
	p := newMockProvider("mock")
	p.setResponse("mock-model-1", "Hello from agent!")

	o.RegisterChannel(ch)
	o.RegisterProvider(p)

	if err := o.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer o.Stop()

	time.Sleep(50 * time.Millisecond)

	req := ChatSyncRequest{
		AgentID: "test-agent",
		UserID:  "user-123",
		Message: "Hello agent!",
	}

	resp, err := o.ChatSync(context.Background(), req)
	if err != nil {
		t.Fatalf("ChatSync failed: %v", err)
	}

	if resp == nil {
		t.Fatal("expected response, got nil")
	}

	if resp.AgentID != "test-agent" {
		t.Errorf("expected agent test-agent, got %s", resp.AgentID)
	}

	if resp.Response == "" {
		t.Error("expected non-empty response")
	}

	if resp.ElapsedMs < 0 {
		t.Error("elapsed time should be non-negative")
	}

	if resp.Model == "" {
		t.Error("expected non-empty model")
	}
}

func TestChatSync_WithHistory(t *testing.T) {
	o := New(testConfig(), testLogger())
	ch := newMockChannel("mock-channel")
	p := newMockProvider("mock")
	p.setResponse("mock-model-1", "Based on our conversation, here is my answer.")

	o.RegisterChannel(ch)
	o.RegisterProvider(p)

	if err := o.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer o.Stop()

	time.Sleep(50 * time.Millisecond)

	req := ChatSyncRequest{
		AgentID: "test-agent",
		UserID:  "user-123",
		Message: "What did I ask earlier?",
		History: []ChatMessage{
			{Role: "user", Content: "What is the weather?"},
			{Role: "assistant", Content: "It's sunny today."},
		},
	}

	resp, err := o.ChatSync(context.Background(), req)
	if err != nil {
		t.Fatalf("ChatSync failed: %v", err)
	}

	if resp.Response == "" {
		t.Error("expected non-empty response")
	}

	// Verify provider was called with history
	if p.getCalls() != 1 {
		t.Errorf("expected 1 provider call, got %d", p.getCalls())
	}
}

func TestChatSync_AgentNotFound(t *testing.T) {
	o := New(testConfig(), testLogger())
	ch := newMockChannel("mock-channel")
	p := newMockProvider("mock")

	o.RegisterChannel(ch)
	o.RegisterProvider(p)

	if err := o.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer o.Stop()

	time.Sleep(50 * time.Millisecond)

	req := ChatSyncRequest{
		AgentID: "nonexistent-agent",
		UserID:  "user-123",
		Message: "Hello!",
	}

	_, err := o.ChatSync(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for nonexistent agent")
	}
}

func TestChatSync_NoProvider(t *testing.T) {
	o := New(testConfig(), testLogger())
	ch := newMockChannel("mock-channel")

	o.RegisterChannel(ch)
	// No provider registered

	if err := o.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer o.Stop()

	time.Sleep(50 * time.Millisecond)

	req := ChatSyncRequest{
		AgentID: "test-agent",
		UserID:  "user-123",
		Message: "Hello!",
	}

	_, err := o.ChatSync(context.Background(), req)
	if err == nil {
		t.Fatal("expected error when no provider available")
	}
}

func TestChatSync_LLMError(t *testing.T) {
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
	defer o.Stop()

	time.Sleep(50 * time.Millisecond)

	req := ChatSyncRequest{
		AgentID: "test-agent",
		UserID:  "user-123",
		Message: "Hello!",
	}

	_, err := o.ChatSync(context.Background(), req)
	if err == nil {
		t.Fatal("expected error on LLM failure")
	}

	// Check error metrics were updated
	o.mu.RLock()
	agent := o.agents["test-agent"]
	o.mu.RUnlock()

	agent.mu.RLock()
	errCount := agent.ErrorCount
	failedActions := agent.Metrics.FailedActions
	agent.mu.RUnlock()

	if errCount == 0 {
		t.Error("expected error count > 0")
	}
	if failedActions == 0 {
		t.Error("expected failed actions > 0")
	}
}

func TestChatSync_WithEvolution(t *testing.T) {
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
	defer o.Stop()

	time.Sleep(50 * time.Millisecond)

	req := ChatSyncRequest{
		AgentID: "test-agent",
		UserID:  "user-123",
		Message: "Hello!",
	}

	_, err := o.ChatSync(context.Background(), req)
	if err != nil {
		t.Fatalf("ChatSync failed: %v", err)
	}

	// Verify evolution was notified
	e.mu.Lock()
	fitnessLen := len(e.fitness)
	e.mu.Unlock()

	if fitnessLen == 0 {
		t.Error("expected evolution.Evaluate to be called")
	}
}

func TestChatSync_WithReporter(t *testing.T) {
	o := New(testConfig(), testLogger())
	ch := newMockChannel("mock-channel")
	p := newMockProvider("mock")
	reporter := &mockReporter{}

	o.RegisterChannel(ch)
	o.RegisterProvider(p)
	o.SetAgentReporter(reporter)

	if err := o.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer o.Stop()

	time.Sleep(50 * time.Millisecond)

	req := ChatSyncRequest{
		AgentID: "test-agent",
		UserID:  "user-123",
		Message: "Hello!",
	}

	_, err := o.ChatSync(context.Background(), req)
	if err != nil {
		t.Fatalf("ChatSync failed: %v", err)
	}

	if reporter.messages == 0 {
		t.Error("expected reporter.RecordMessage to be called")
	}
	if reporter.metricUpdates == 0 {
		t.Error("expected reporter.UpdateMetrics to be called")
	}
}

func TestChatSync_ContextCancelled(t *testing.T) {
	o := New(testConfig(), testLogger())
	ch := newMockChannel("mock-channel")
	p := &mockSlowProvider{
		mockProvider: newMockProvider("mock"),
		delay:        2 * time.Second,
	}

	o.RegisterChannel(ch)
	o.RegisterProvider(p)

	if err := o.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer o.Stop()

	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	req := ChatSyncRequest{
		AgentID: "test-agent",
		UserID:  "user-123",
		Message: "Hello!",
	}

	_, err := o.ChatSync(ctx, req)
	if err == nil {
		t.Fatal("expected error on context cancellation")
	}
}

func TestListAgentIDs(t *testing.T) {
	o := New(testConfig(), testLogger())
	ch := newMockChannel("mock-channel")
	p := newMockProvider("mock")

	o.RegisterChannel(ch)
	o.RegisterProvider(p)

	if err := o.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer o.Stop()

	time.Sleep(50 * time.Millisecond)

	ids := o.ListAgentIDs()
	if len(ids) != 1 {
		t.Errorf("expected 1 agent ID, got %d", len(ids))
	}

	if ids[0] != "test-agent" {
		t.Errorf("expected test-agent, got %s", ids[0])
	}
}

func TestGetAgentInfo(t *testing.T) {
	o := New(testConfig(), testLogger())
	ch := newMockChannel("mock-channel")
	p := newMockProvider("mock")

	o.RegisterChannel(ch)
	o.RegisterProvider(p)

	if err := o.Start(); err != nil {
		t.Fatalf("failed to start: %v", err)
	}
	defer o.Stop()

	time.Sleep(50 * time.Millisecond)

	info := o.GetAgentInfo("test-agent")
	if info == nil {
		t.Fatal("expected agent info, got nil")
	}
	if info.ID != "test-agent" {
		t.Errorf("expected test-agent, got %s", info.ID)
	}

	// Nonexistent
	info = o.GetAgentInfo("nonexistent")
	if info != nil {
		t.Error("expected nil for nonexistent agent")
	}
}

// Mock reporter for testing
type mockReporter struct {
	messages      int
	errors        int
	metricUpdates int
}

func (m *mockReporter) RecordMessage(id string) error {
	m.messages++
	return nil
}

func (m *mockReporter) RecordError(id string) error {
	m.errors++
	return nil
}

func (m *mockReporter) UpdateMetrics(id string, tokensUsed int, costUSD float64, responseMs int64, success bool) error {
	m.metricUpdates++
	return nil
}

// Mock slow provider for context cancellation testing
type mockSlowProvider struct {
	*mockProvider
	delay time.Duration
}

func (m *mockSlowProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	select {
	case <-ctx.Done():
		return nil, fmt.Errorf("context cancelled: %w", ctx.Err())
	case <-time.After(m.delay):
		return m.mockProvider.Chat(ctx, req)
	}
}
