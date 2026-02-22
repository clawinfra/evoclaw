package interfaces_test

import (
	"context"
	"testing"
	"time"

	iface "github.com/clawinfra/evoclaw/internal/interfaces"
)

// --- Mock implementations to verify interface contracts ---

type mockProvider struct{}

func (m *mockProvider) Name() string                                                  { return "mock" }
func (m *mockProvider) Chat(_ context.Context, _ iface.ChatRequest) (*iface.ChatResponse, error) {
	return &iface.ChatResponse{Content: "hello", Model: "mock-1"}, nil
}
func (m *mockProvider) Models() []string                { return []string{"mock-1"} }
func (m *mockProvider) HealthCheck(_ context.Context) error { return nil }

type mockMemory struct{}

func (m *mockMemory) Store(_ context.Context, _ string, _ []byte, _ map[string]string) error {
	return nil
}
func (m *mockMemory) Retrieve(_ context.Context, _ string, _ int) ([]iface.MemoryEntry, error) {
	return []iface.MemoryEntry{{Key: "k1", Content: []byte("data"), Timestamp: time.Now()}}, nil
}
func (m *mockMemory) Delete(_ context.Context, _ string) error  { return nil }
func (m *mockMemory) HealthCheck(_ context.Context) error        { return nil }

type mockTool struct{}

func (m *mockTool) Name() string        { return "mock-tool" }
func (m *mockTool) Description() string { return "a mock tool" }
func (m *mockTool) Execute(_ context.Context, _ map[string]interface{}) (*iface.ToolResult, error) {
	return &iface.ToolResult{Output: "done"}, nil
}
func (m *mockTool) Schema() iface.ToolSchema {
	return iface.ToolSchema{Name: "mock-tool", Description: "a mock tool"}
}

type mockRegistry struct{ tools map[string]iface.Tool }

func (r *mockRegistry) Register(t iface.Tool) error { r.tools[t.Name()] = t; return nil }
func (r *mockRegistry) Get(name string) (iface.Tool, bool) { t, ok := r.tools[name]; return t, ok }
func (r *mockRegistry) List() []iface.Tool {
	out := make([]iface.Tool, 0, len(r.tools))
	for _, t := range r.tools {
		out = append(out, t)
	}
	return out
}

type mockChannel struct{ closed bool }

func (m *mockChannel) Name() string { return "mock-chan" }
func (m *mockChannel) Send(_ context.Context, _ iface.OutboundMessage) error { return nil }
func (m *mockChannel) Receive(_ context.Context) (<-chan iface.InboundMessage, error) {
	ch := make(chan iface.InboundMessage, 1)
	ch <- iface.InboundMessage{From: "user", Content: "hi", Timestamp: time.Now()}
	close(ch)
	return ch, nil
}
func (m *mockChannel) Close() error { m.closed = true; return nil }

type mockObserver struct{ flushed bool }

func (m *mockObserver) OnRequest(_ context.Context, _ iface.ObservedRequest)   {}
func (m *mockObserver) OnResponse(_ context.Context, _ iface.ObservedResponse) {}
func (m *mockObserver) OnError(_ context.Context, _ iface.ObservedError)       {}
func (m *mockObserver) Flush() error { m.flushed = true; return nil }

// --- Tests ---

func TestProviderContract(t *testing.T) {
	var p iface.Provider = &mockProvider{}
	if p.Name() != "mock" {
		t.Fatal("expected mock")
	}
	resp, err := p.Chat(context.Background(), iface.ChatRequest{Model: "mock-1"})
	if err != nil || resp.Content != "hello" {
		t.Fatal("chat failed")
	}
	if len(p.Models()) != 1 {
		t.Fatal("expected 1 model")
	}
	if err := p.HealthCheck(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestMemoryBackendContract(t *testing.T) {
	var m iface.MemoryBackend = &mockMemory{}
	ctx := context.Background()
	if err := m.Store(ctx, "k1", []byte("data"), nil); err != nil {
		t.Fatal(err)
	}
	entries, err := m.Retrieve(ctx, "k1", 10)
	if err != nil || len(entries) != 1 {
		t.Fatal("retrieve failed")
	}
	if err := m.Delete(ctx, "k1"); err != nil {
		t.Fatal(err)
	}
	if err := m.HealthCheck(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestToolContract(t *testing.T) {
	var tool iface.Tool = &mockTool{}
	if tool.Name() != "mock-tool" {
		t.Fatal("wrong name")
	}
	res, err := tool.Execute(context.Background(), nil)
	if err != nil || res.Output != "done" {
		t.Fatal("execute failed")
	}
	s := tool.Schema()
	if s.Name != "mock-tool" {
		t.Fatal("wrong schema name")
	}
}

func TestToolRegistryContract(t *testing.T) {
	var reg iface.ToolRegistry = &mockRegistry{tools: make(map[string]iface.Tool)}
	tool := &mockTool{}
	if err := reg.Register(tool); err != nil {
		t.Fatal(err)
	}
	got, ok := reg.Get("mock-tool")
	if !ok || got.Name() != "mock-tool" {
		t.Fatal("get failed")
	}
	if len(reg.List()) != 1 {
		t.Fatal("list failed")
	}
}

func TestChannelContract(t *testing.T) {
	var ch iface.Channel = &mockChannel{}
	if ch.Name() != "mock-chan" {
		t.Fatal("wrong name")
	}
	if err := ch.Send(context.Background(), iface.OutboundMessage{To: "u", Content: "hi"}); err != nil {
		t.Fatal(err)
	}
	msgs, err := ch.Receive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	msg := <-msgs
	if msg.Content != "hi" {
		t.Fatal("wrong content")
	}
	if err := ch.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestObserverContract(t *testing.T) {
	obs := &mockObserver{}
	var o iface.Observer = obs
	ctx := context.Background()
	o.OnRequest(ctx, iface.ObservedRequest{ID: "r1", Model: "m"})
	o.OnResponse(ctx, iface.ObservedResponse{RequestID: "r1", Success: true})
	o.OnError(ctx, iface.ObservedError{RequestID: "r1", Error: "oops"})
	if err := o.Flush(); err != nil {
		t.Fatal(err)
	}
	if !obs.flushed {
		t.Fatal("flush not called")
	}
}
