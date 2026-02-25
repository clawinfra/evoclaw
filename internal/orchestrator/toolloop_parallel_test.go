package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/clawinfra/evoclaw/internal/config"
)

// ---------------------------------------------------------------------------
// mockEdgeExecutor — a configurable fake execFunc for ToolLoop
// ---------------------------------------------------------------------------

type mockEdgeExecutor struct {
	results map[string]*ToolResult     // per call-ID result (nil = return error)
	errors  map[string]error           // per call-ID error
	latency map[string]time.Duration   // per call-ID sleep before returning
	callLog []string                   // records call IDs in execution order
	mu      sync.Mutex
}

func newMockEdge() *mockEdgeExecutor {
	return &mockEdgeExecutor{
		results: make(map[string]*ToolResult),
		errors:  make(map[string]error),
		latency: make(map[string]time.Duration),
	}
}

// exec is the execFunc-compatible signature
func (m *mockEdgeExecutor) exec(agent *AgentState, call ToolCall) (*ToolResult, error) {
	if lat, ok := m.latency[call.ID]; ok {
		time.Sleep(lat)
	}
	m.mu.Lock()
	m.callLog = append(m.callLog, call.ID)
	m.mu.Unlock()

	if err, ok := m.errors[call.ID]; ok && err != nil {
		return nil, err
	}
	if res, ok := m.results[call.ID]; ok {
		return res, nil
	}
	return &ToolResult{Tool: call.Name, Status: "success", Result: "ok", ElapsedMs: 1}, nil
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func makeToolLoop(maxParallel int, fn func(*AgentState, ToolCall) (*ToolResult, error)) *ToolLoop {
	return &ToolLoop{
		logger:         slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})),
		maxIterations:  10,
		errorLimit:     3,
		defaultTimeout: 30 * time.Second,
		maxParallel:    maxParallel,
		execFunc:       fn,
	}
}

func makeAgent(id string) *AgentState {
	return &AgentState{ID: id, Def: config.AgentDef{SystemPrompt: "test"}}
}

func makeCall(id, name string) ToolCall {
	return ToolCall{ID: id, Name: name, Arguments: map[string]interface{}{}}
}

func successResult(tool string) *ToolResult {
	return &ToolResult{Tool: tool, Status: "success", Result: "ok", ElapsedMs: 1}
}

// ---------------------------------------------------------------------------
// 1. TestParallel_SingleCall — fast path: no goroutine overhead, ParallelBatches=0
// ---------------------------------------------------------------------------

func TestParallel_SingleCall(t *testing.T) {
	mock := newMockEdge()
	mock.results["call-1"] = successResult("tool-a")

	tl := makeToolLoop(5, mock.exec)
	agent := makeAgent("agent-1")

	calls := []ToolCall{makeCall("call-1", "tool-a")}
	results := tl.executeParallel(context.Background(), agent, calls)

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err != nil {
		t.Errorf("expected no error, got %v", results[0].Err)
	}
	if results[0].Result.Status != "success" {
		t.Errorf("expected success, got %s", results[0].Result.Status)
	}
	if results[0].Index != 0 {
		t.Errorf("expected index 0, got %d", results[0].Index)
	}
}

// ---------------------------------------------------------------------------
// 2. TestParallel_TwoCalls — 2×200ms calls; wall time must be <300ms
// ---------------------------------------------------------------------------

func TestParallel_TwoCalls(t *testing.T) {
	mock := newMockEdge()
	mock.latency["c1"] = 200 * time.Millisecond
	mock.latency["c2"] = 200 * time.Millisecond

	tl := makeToolLoop(5, mock.exec)
	agent := makeAgent("a")

	calls := []ToolCall{makeCall("c1", "t"), makeCall("c2", "t")}
	start := time.Now()
	results := tl.executeParallel(context.Background(), agent, calls)
	elapsed := time.Since(start)

	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	for _, r := range results {
		if r.Err != nil {
			t.Errorf("unexpected error: %v", r.Err)
		}
	}
	if elapsed >= 300*time.Millisecond {
		t.Errorf("parallel execution took %v, expected <300ms (parallelism not working)", elapsed)
	}
}

// ---------------------------------------------------------------------------
// 3. TestParallel_OneFailsOneSucceeds — both results returned
// ---------------------------------------------------------------------------

func TestParallel_OneFailsOneSucceeds(t *testing.T) {
	mock := newMockEdge()
	mock.errors["fail"] = errors.New("boom")
	mock.results["ok"] = successResult("tool")

	tl := makeToolLoop(5, mock.exec)
	agent := makeAgent("a")

	calls := []ToolCall{makeCall("fail", "tool"), makeCall("ok", "tool")}
	results := tl.executeParallel(context.Background(), agent, calls)

	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}
	// Index 0 should fail
	if results[0].Err == nil {
		t.Error("expected index 0 to have an error")
	}
	// Index 1 should succeed
	if results[1].Err != nil {
		t.Errorf("expected index 1 success, got %v", results[1].Err)
	}
	if results[1].Result.Status != "success" {
		t.Errorf("expected success status, got %s", results[1].Result.Status)
	}
}

// ---------------------------------------------------------------------------
// 4. TestParallel_AllFail — all 3 fail
// ---------------------------------------------------------------------------

func TestParallel_AllFail(t *testing.T) {
	mock := newMockEdge()
	for _, id := range []string{"a", "b", "c"} {
		mock.errors[id] = fmt.Errorf("error-%s", id)
	}

	tl := makeToolLoop(5, mock.exec)
	agent := makeAgent("a")

	calls := []ToolCall{makeCall("a", "t"), makeCall("b", "t"), makeCall("c", "t")}
	results := tl.executeParallel(context.Background(), agent, calls)

	errorCount := 0
	for _, r := range results {
		if r.Err != nil {
			errorCount++
		}
	}
	if errorCount != 3 {
		t.Errorf("expected 3 errors, got %d", errorCount)
	}
}

// ---------------------------------------------------------------------------
// 5. TestParallel_OrderPreserved — latencies 300/100/200ms; results in order [0,1,2]
// ---------------------------------------------------------------------------

func TestParallel_OrderPreserved(t *testing.T) {
	mock := newMockEdge()
	mock.latency["i0"] = 300 * time.Millisecond
	mock.latency["i1"] = 100 * time.Millisecond
	mock.latency["i2"] = 200 * time.Millisecond
	mock.results["i0"] = &ToolResult{Tool: "t", Status: "success", Result: "r0"}
	mock.results["i1"] = &ToolResult{Tool: "t", Status: "success", Result: "r1"}
	mock.results["i2"] = &ToolResult{Tool: "t", Status: "success", Result: "r2"}

	tl := makeToolLoop(5, mock.exec)
	agent := makeAgent("a")

	calls := []ToolCall{makeCall("i0", "t"), makeCall("i1", "t"), makeCall("i2", "t")}
	results := tl.executeParallel(context.Background(), agent, calls)

	if len(results) != 3 {
		t.Fatalf("want 3 results, got %d", len(results))
	}
	for i, want := range []string{"r0", "r1", "r2"} {
		if results[i].Index != i {
			t.Errorf("result[%d].Index = %d, want %d", i, results[i].Index, i)
		}
		if results[i].Result.Result != want {
			t.Errorf("result[%d].Result = %q, want %q", i, results[i].Result.Result, want)
		}
	}
}

// ---------------------------------------------------------------------------
// 6. TestParallel_ConcurrencyLimit — 8 calls, maxParallel=3 → ≤3 concurrent
// ---------------------------------------------------------------------------

func TestParallel_ConcurrencyLimit(t *testing.T) {
	var current, peak atomic.Int32

	exec := func(agent *AgentState, call ToolCall) (*ToolResult, error) {
		n := current.Add(1)
		for {
			old := peak.Load()
			if n <= old {
				break
			}
			if peak.CompareAndSwap(old, n) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond)
		current.Add(-1)
		return successResult(call.Name), nil
	}

	tl := makeToolLoop(3, exec)
	agent := makeAgent("a")

	calls := make([]ToolCall, 8)
	for i := range calls {
		calls[i] = makeCall(fmt.Sprintf("c%d", i), "t")
	}

	results := tl.executeParallel(context.Background(), agent, calls)
	if len(results) != 8 {
		t.Fatalf("want 8 results, got %d", len(results))
	}

	if got := int(peak.Load()); got > 3 {
		t.Errorf("peak concurrency = %d, want ≤3", got)
	}
}

// ---------------------------------------------------------------------------
// 7. TestParallel_ContextCancelled — pre-cancelled context → goroutines bail fast
// ---------------------------------------------------------------------------

func TestParallel_ContextCancelled(t *testing.T) {
	slowExec := func(agent *AgentState, call ToolCall) (*ToolResult, error) {
		time.Sleep(5 * time.Second) // would hang test if goroutine didn't bail
		return successResult(call.Name), nil
	}

	tl := makeToolLoop(5, slowExec)
	agent := makeAgent("a")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // pre-cancel before executeParallel

	calls := []ToolCall{makeCall("c1", "t"), makeCall("c2", "t"), makeCall("c3", "t")}
	start := time.Now()
	results := tl.executeParallel(ctx, agent, calls)
	elapsed := time.Since(start)

	if elapsed > 2*time.Second {
		t.Errorf("cancelled context should return quickly, took %v", elapsed)
	}
	if len(results) != 3 {
		t.Fatalf("want 3 results, got %d", len(results))
	}
	// All should have context errors since context was pre-cancelled
	for i, r := range results {
		if r.Err == nil {
			t.Errorf("result[%d]: expected context error, got nil", i)
		}
	}
}

// ---------------------------------------------------------------------------
// 8. TestParallel_MetricsWallTimeSaved — 2×200ms; WallTimeSavedMs ≈ 200ms (±100ms)
// ---------------------------------------------------------------------------

func TestParallel_MetricsWallTimeSaved(t *testing.T) {
	const latency = 200 * time.Millisecond

	mock := newMockEdge()
	mock.latency["m1"] = latency
	mock.latency["m2"] = latency
	// Set ElapsedMs in results to simulate per-call elapsed time reporting
	mock.results["m1"] = &ToolResult{Tool: "t", Status: "success", Result: "ok", ElapsedMs: latency.Milliseconds()}
	mock.results["m2"] = &ToolResult{Tool: "t", Status: "success", Result: "ok", ElapsedMs: latency.Milliseconds()}

	tl := makeToolLoop(5, mock.exec)
	agent := makeAgent("a")

	calls := []ToolCall{makeCall("m1", "t"), makeCall("m2", "t")}

	batchStart := time.Now()
	batchResults := tl.executeParallel(context.Background(), agent, calls)
	batchWall := time.Since(batchStart)

	// Compute the same WallTimeSavedMs calculation that Execute() uses
	var sumElapsed int64
	for _, r := range batchResults {
		if r.Result != nil {
			sumElapsed += r.Result.ElapsedMs
		}
	}
	saved := sumElapsed - batchWall.Milliseconds()
	if saved < 0 {
		saved = 0
	}

	// With 2×200ms in parallel, sum≈400ms, wall≈200ms → saved≈200ms
	// Allow generous ±150ms tolerance for CI environments
	if saved < 50 || saved > 400 {
		t.Errorf("WallTimeSavedMs = %d ms, expected ~200ms (50–400ms range)", saved)
	}
}

// ---------------------------------------------------------------------------
// Mock LLM provider for full Execute() tests
// ---------------------------------------------------------------------------

type toolLoopMockProvider struct {
	name        string
	callCount   int
	mu          sync.Mutex
	responses   []mockLLMResponse // response sequence per call
}

type mockLLMResponse struct {
	content   string
	toolCalls []ToolCall
}

func (p *toolLoopMockProvider) Name() string { return p.name }

func (p *toolLoopMockProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	idx := p.callCount
	p.callCount++
	if idx < len(p.responses) {
		r := p.responses[idx]
		return &ChatResponse{Content: r.content, ToolCalls: r.toolCalls}, nil
	}
	return &ChatResponse{Content: "done"}, nil
}

func (p *toolLoopMockProvider) Models() []config.Model {
	return []config.Model{{ID: p.name}}
}

// newTestOrchestratorForToolLoop builds a minimal Orchestrator for ToolLoop integration tests.
func newTestOrchestratorForToolLoop(t *testing.T, provider *toolLoopMockProvider) *Orchestrator {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	cfg := &config.Config{
		Agents: []config.AgentDef{},
		Models: config.ModelsConfig{
			Providers: map[string]config.ProviderConfig{
				"test": {
					Models: []config.Model{{ID: provider.name}},
				},
			},
		},
	}
	orch := New(cfg, logger)
	orch.RegisterProvider(provider)
	return orch
}

// ---------------------------------------------------------------------------
// 9. TestExecute_MultiToolLLMResponse — full Execute() with 2 tool_calls
// ---------------------------------------------------------------------------

func TestExecute_MultiToolLLMResponse(t *testing.T) {
	// LLM call 1: returns 2 tool_calls
	// LLM call 2 (summary): returns final text
	provider := &toolLoopMockProvider{
		name: "test/model",
		responses: []mockLLMResponse{
			{
				content: "",
				toolCalls: []ToolCall{
					makeCall("tc1", "tool_a"),
					makeCall("tc2", "tool_b"),
				},
			},
			{content: "Final answer from 2 tools"},
		},
	}

	orch := newTestOrchestratorForToolLoop(t, provider)
	tm := NewToolManager("", nil, orch.logger)

	tl := &ToolLoop{
		orchestrator:   orch,
		toolManager:    tm,
		logger:         orch.logger,
		maxIterations:  10,
		errorLimit:     3,
		defaultTimeout: 30 * time.Second,
		maxParallel:    5,
		execFunc: func(agent *AgentState, call ToolCall) (*ToolResult, error) {
			return &ToolResult{Tool: call.Name, Status: "success", Result: "result-" + call.ID, ElapsedMs: 10}, nil
		},
	}

	agent := makeAgent("agent-execute")
	msg := Message{Content: "do two things", Channel: "test", From: "user1"}

	resp, metrics, err := tl.Execute(agent, msg, "test/model")
	if err != nil {
		t.Fatalf("Execute() failed: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.Content != "Final answer from 2 tools" {
		t.Errorf("response content = %q, want %q", resp.Content, "Final answer from 2 tools")
	}
	if metrics.ToolCalls != 2 {
		t.Errorf("ToolCalls = %d, want 2", metrics.ToolCalls)
	}
	if metrics.ParallelBatches != 1 {
		t.Errorf("ParallelBatches = %d, want 1", metrics.ParallelBatches)
	}
	if metrics.MaxConcurrency != 2 {
		t.Errorf("MaxConcurrency = %d, want 2", metrics.MaxConcurrency)
	}
	if metrics.SuccessCount != 2 {
		t.Errorf("SuccessCount = %d, want 2", metrics.SuccessCount)
	}
}

// ---------------------------------------------------------------------------
// 10. TestExecute_BackwardCompatSingleTool — single tool_call; ParallelBatches=0
// ---------------------------------------------------------------------------

func TestExecute_BackwardCompatSingleTool(t *testing.T) {
	// LLM call 1: returns 1 tool_call
	// LLM call 2 (summary): returns final text
	provider := &toolLoopMockProvider{
		name: "test/model",
		responses: []mockLLMResponse{
			{
				content:   "",
				toolCalls: []ToolCall{makeCall("tc1", "tool_a")},
			},
			{content: "Single tool answer"},
		},
	}

	orch := newTestOrchestratorForToolLoop(t, provider)
	tm := NewToolManager("", nil, orch.logger)

	tl := &ToolLoop{
		orchestrator:   orch,
		toolManager:    tm,
		logger:         orch.logger,
		maxIterations:  10,
		errorLimit:     3,
		defaultTimeout: 30 * time.Second,
		maxParallel:    5,
		execFunc: func(agent *AgentState, call ToolCall) (*ToolResult, error) {
			return &ToolResult{Tool: call.Name, Status: "success", Result: "single-result", ElapsedMs: 5}, nil
		},
	}

	agent := makeAgent("agent-compat")
	msg := Message{Content: "do one thing", Channel: "test", From: "user1"}

	resp, metrics, err := tl.Execute(agent, msg, "test/model")
	if err != nil {
		t.Fatalf("Execute() failed: %v", err)
	}
	if resp == nil {
		t.Fatal("expected non-nil response")
	}
	if resp.Content != "Single tool answer" {
		t.Errorf("response content = %q, want %q", resp.Content, "Single tool answer")
	}
	if metrics.ToolCalls != 1 {
		t.Errorf("ToolCalls = %d, want 1", metrics.ToolCalls)
	}
	// Single call must NOT increment ParallelBatches — backward compat
	if metrics.ParallelBatches != 0 {
		t.Errorf("ParallelBatches = %d, want 0 (backward compat)", metrics.ParallelBatches)
	}
	if metrics.SuccessCount != 1 {
		t.Errorf("SuccessCount = %d, want 1", metrics.SuccessCount)
	}
}
