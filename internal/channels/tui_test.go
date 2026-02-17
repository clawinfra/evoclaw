package channels

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/clawinfra/evoclaw/internal/types"
)

func TestNewTUI(t *testing.T) {
	logger := slog.Default()
	ch := NewTUI(logger, nil)
	if ch == nil {
		t.Fatal("NewTUI returned nil")
	}
	if ch.Name() != "tui" {
		t.Errorf("Name() = %q, want %q", ch.Name(), "tui")
	}
}

func TestTUIReceive(t *testing.T) {
	logger := slog.Default()
	ch := NewTUI(logger, nil)
	recv := ch.Receive()
	if recv == nil {
		t.Fatal("Receive() returned nil channel")
	}
}

func TestTUIStop(t *testing.T) {
	logger := slog.Default()
	ch := NewTUI(logger, nil)
	// Stop before Start should not panic
	if err := ch.Stop(); err != nil {
		t.Errorf("Stop() returned error: %v", err)
	}
}

func TestTUISendWithoutProgram(t *testing.T) {
	logger := slog.Default()
	ch := NewTUI(logger, nil)
	// Send without a running program should not panic, just buffer
	err := ch.Send(context.Background(), types.Response{
		AgentID: "agent-1",
		Content: "Hello!",
	})
	if err != nil {
		t.Errorf("Send() returned error: %v", err)
	}
}

func TestTUISendUserMessage(t *testing.T) {
	logger := slog.Default()
	ch := NewTUI(logger, nil)
	ch.sendUserMessage("test message")
	select {
	case msg := <-ch.inbox:
		if msg.Content != "test message" {
			t.Errorf("Content = %q, want %q", msg.Content, "test message")
		}
		if msg.Channel != "tui" {
			t.Errorf("Channel = %q, want %q", msg.Channel, "tui")
		}
		if msg.From != "user" {
			t.Errorf("From = %q, want %q", msg.From, "user")
		}
	case <-time.After(time.Second):
		t.Fatal("Timeout waiting for message")
	}
}

func TestNewTUIModel(t *testing.T) {
	logger := slog.Default()
	ch := NewTUI(logger, nil)
	m := newTUIModel(ch)
	if m.channel != ch {
		t.Error("channel reference mismatch")
	}
	if len(m.messages) != 0 {
		t.Errorf("messages should be empty, got %d", len(m.messages))
	}
}

func TestTUIModelInit(t *testing.T) {
	logger := slog.Default()
	ch := NewTUI(logger, nil)
	m := newTUIModel(ch)
	cmd := m.Init()
	if cmd == nil {
		t.Error("Init() should return a Cmd")
	}
}

func TestTUIModelViewNotReady(t *testing.T) {
	logger := slog.Default()
	ch := NewTUI(logger, nil)
	m := newTUIModel(ch)
	view := m.View()
	if view == "" {
		t.Error("View() should return non-empty when not ready")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		name     string
		d        time.Duration
		expected string
	}{
		{"seconds", 30 * time.Second, "30s"},
		{"minutes", 5 * time.Minute, "5m"},
		{"hours", 3*time.Hour + 15*time.Minute, "3h 15m"},
		{"days", 48*time.Hour + 6*time.Hour, "2d 6h"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := formatDuration(tt.d)
			if result != tt.expected {
				t.Errorf("formatDuration(%v) = %q, want %q", tt.d, result, tt.expected)
			}
		})
	}
}

func TestTUIRenderSidebar(t *testing.T) {
	logger := slog.Default()
	agents := []types.AgentInfo{
		{
			ID:           "agent-1",
			Status:       "running",
			MessageCount: 42,
			StartedAt:    time.Now().Add(-1 * time.Hour),
			Metrics: types.AgentMetrics{
				AvgResponseMs: 150.0,
				TokensUsed:    5000,
				CostUSD:       0.05,
			},
		},
		{
			ID:           "agent-2",
			Status:       "evolving",
			MessageCount: 0,
			StartedAt:    time.Now().Add(-30 * time.Minute),
		},
		{
			ID:           "agent-3",
			Status:       "idle",
			MessageCount: 0,
			StartedAt:    time.Now(),
		},
	}
	ch := NewTUI(logger, func() []types.AgentInfo { return agents })
	m := newTUIModel(ch)
	m.height = 40
	m.width = 80
	sidebar := m.renderSidebar()
	if sidebar == "" {
		t.Error("renderSidebar() returned empty string")
	}
}

func TestTUIRenderSidebarNoAgents(t *testing.T) {
	logger := slog.Default()
	ch := NewTUI(logger, func() []types.AgentInfo { return nil })
	m := newTUIModel(ch)
	m.height = 40
	sidebar := m.renderSidebar()
	if sidebar == "" {
		t.Error("renderSidebar() returned empty string")
	}
}

func TestTUIRenderSidebarNilFn(t *testing.T) {
	logger := slog.Default()
	ch := NewTUI(logger, nil)
	m := newTUIModel(ch)
	m.height = 40
	sidebar := m.renderSidebar()
	if sidebar == "" {
		t.Error("renderSidebar() returned empty string")
	}
}

func TestTUIRenderChat(t *testing.T) {
	logger := slog.Default()
	ch := NewTUI(logger, nil)
	m := newTUIModel(ch)

	// Empty messages
	chat := m.renderChat()
	if chat == "" {
		t.Error("renderChat() returned empty for no messages")
	}

	// With messages
	m.messages = []chatEntry{
		{sender: "You", content: "Hello", time: time.Now(), isUser: true},
		{sender: "agent-1", content: "Hi there!", time: time.Now(), isUser: false},
	}
	chat = m.renderChat()
	if chat == "" {
		t.Error("renderChat() returned empty with messages")
	}
}

func TestTUIRenderSidebarLargeTokens(t *testing.T) {
	logger := slog.Default()
	agents := []types.AgentInfo{
		{
			ID:           "agent-1",
			Status:       "running",
			MessageCount: 1,
			StartedAt:    time.Now(),
			Metrics: types.AgentMetrics{
				TokensUsed: 500, // < 1000, should show raw number
			},
		},
	}
	ch := NewTUI(logger, func() []types.AgentInfo { return agents })
	m := newTUIModel(ch)
	m.height = 40
	sidebar := m.renderSidebar()
	if sidebar == "" {
		t.Error("renderSidebar() returned empty string")
	}
}

func TestTickCmd(t *testing.T) {
	cmd := tickCmd()
	if cmd == nil {
		t.Error("tickCmd() should return a non-nil Cmd")
	}
}
