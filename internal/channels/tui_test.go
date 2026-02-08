package channels

import (
	"testing"
	"time"

	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/orchestrator"
)

func TestTUIChannelName(t *testing.T) {
	ch := NewTUI(testLogger(), nil)
	if ch.Name() != "tui" {
		t.Errorf("expected channel name 'tui', got %q", ch.Name())
	}
}

func TestTUIChannelSendUserMessage(t *testing.T) {
	ch := NewTUI(testLogger(), nil)

	// Send a user message
	go ch.sendUserMessage("hello agent")

	select {
	case msg := <-ch.Receive():
		if msg.Content != "hello agent" {
			t.Errorf("expected 'hello agent', got %q", msg.Content)
		}
		if msg.Channel != "tui" {
			t.Errorf("expected channel 'tui', got %q", msg.Channel)
		}
		if msg.From != "user" {
			t.Errorf("expected from 'user', got %q", msg.From)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for message")
	}
}

func TestTUIChannelSidebar(t *testing.T) {
	agents := []orchestrator.AgentInfo{
		{
			ID:     "test-agent",
			Def:    config.AgentDef{ID: "test-agent", Name: "Test Agent"},
			Status: "running",
			Metrics: orchestrator.AgentMetrics{
				TotalActions:      10,
				SuccessfulActions: 9,
				TokensUsed:        5000,
				CostUSD:           0.05,
			},
			StartedAt:    time.Now().Add(-2 * time.Hour),
			MessageCount: 10,
		},
	}

	ch := NewTUI(testLogger(), func() []orchestrator.AgentInfo {
		return agents
	})

	if ch.agentsFn == nil {
		t.Fatal("agentsFn should not be nil")
	}

	result := ch.agentsFn()
	if len(result) != 1 {
		t.Fatalf("expected 1 agent, got %d", len(result))
	}
	if result[0].ID != "test-agent" {
		t.Errorf("expected agent ID 'test-agent', got %q", result[0].ID)
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		dur    time.Duration
		expect string
	}{
		{30 * time.Second, "30s"},
		{5 * time.Minute, "5m"},
		{2*time.Hour + 30*time.Minute, "2h 30m"},
		{3*24*time.Hour + 4*time.Hour, "3d 4h"},
	}

	for _, tt := range tests {
		got := formatDuration(tt.dur)
		if got != tt.expect {
			t.Errorf("formatDuration(%v) = %q, want %q", tt.dur, got, tt.expect)
		}
	}
}
