package ios

import (
	"context"
	"encoding/json"
	"testing"
)

func TestNewIOSAgent_MissingAgentID(t *testing.T) {
	_, err := NewIOSAgent(AgentConfig{})
	if err == nil {
		t.Fatal("expected error for missing AgentID")
	}
}

func TestNewIOSAgent_Valid(t *testing.T) {
	agent, err := NewIOSAgent(AgentConfig{
		AgentID:   "ios-agent",
		Model:     "claude-3-haiku",
		DataDir:   t.TempDir(),
		URLScheme: "evoclaw",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agent == nil {
		t.Fatal("expected non-nil agent")
	}
}

func TestIOSAgent_Lifecycle(t *testing.T) {
	agent, err := NewIOSAgent(AgentConfig{
		AgentID: "lifecycle-ios",
		Model:   "claude-3-haiku",
		DataDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	ctx := context.Background()

	if err := agent.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Double-start = no-op
	if err := agent.Start(ctx); err != nil {
		t.Fatalf("second Start: %v", err)
	}

	var status map[string]interface{}
	_ = json.Unmarshal([]byte(agent.GetStatus()), &status)
	if status["running"] != true {
		t.Error("expected running=true")
	}

	if err := agent.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Double-stop = no-op
	if err := agent.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}

	_ = json.Unmarshal([]byte(agent.GetStatus()), &status)
	if status["running"] != false {
		t.Error("expected running=false after stop")
	}
}

func TestIOSAgent_HandleURLScheme(t *testing.T) {
	agent, _ := NewIOSAgent(AgentConfig{AgentID: "url-agent", DataDir: t.TempDir()})
	ctx := context.Background()
	_ = agent.Start(ctx)

	if err := agent.HandleURLScheme("evoclaw://message?text=hello"); err != nil {
		t.Fatalf("HandleURLScheme: %v", err)
	}
}

func TestIOSAgent_HandleURLScheme_Empty(t *testing.T) {
	agent, _ := NewIOSAgent(AgentConfig{AgentID: "url-agent2", DataDir: t.TempDir()})
	ctx := context.Background()
	_ = agent.Start(ctx)

	if err := agent.HandleURLScheme(""); err == nil {
		t.Fatal("expected error for empty URL")
	}
}

func TestIOSAgent_HandleURLScheme_NotRunning(t *testing.T) {
	agent, _ := NewIOSAgent(AgentConfig{AgentID: "stopped-ios", DataDir: t.TempDir()})

	if err := agent.HandleURLScheme("evoclaw://test"); err == nil {
		t.Fatal("expected error when not running")
	}
}

func TestIOSAgent_BackgroundFetch(t *testing.T) {
	agent, _ := NewIOSAgent(AgentConfig{AgentID: "fetch-agent", DataDir: t.TempDir()})

	// Not running → failed
	if result := agent.PerformBackgroundFetch(); result != "failed" {
		t.Errorf("expected 'failed', got %q", result)
	}

	ctx := context.Background()
	_ = agent.Start(ctx)

	result := agent.PerformBackgroundFetch()
	if result != "newData" && result != "noData" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestNewIOSAgent_LogLevels(t *testing.T) {
	for _, level := range []string{"warn", "error", "info", ""} {
		_, err := NewIOSAgent(AgentConfig{AgentID: "log-test", LogLevel: level})
		if err != nil {
			t.Errorf("level %q: %v", level, err)
		}
	}
}

func TestIOSAgent_Stop_WithCancel(t *testing.T) {
	agent, _ := NewIOSAgent(AgentConfig{AgentID: "cancel-agent", DataDir: t.TempDir()})
	ctx := context.Background()
	_, cancel := context.WithCancel(ctx)
	agent.cancel = cancel
	agent.running = true
	agent.status.Running = true

	if err := agent.Stop(); err != nil {
		t.Fatalf("Stop with cancel: %v", err)
	}
	if agent.running {
		t.Error("expected running=false after Stop")
	}
}
