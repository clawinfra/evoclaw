package android

import (
	"context"
	"encoding/json"
	"testing"
)

func TestNewAndroidAgent_MissingAgentID(t *testing.T) {
	_, err := NewAndroidAgent(AgentConfig{})
	if err == nil {
		t.Fatal("expected error for missing AgentID")
	}
}

func TestNewAndroidAgent_Valid(t *testing.T) {
	agent, err := NewAndroidAgent(AgentConfig{
		AgentID:  "test-agent",
		Model:    "claude-3-haiku",
		DataDir:  t.TempDir(),
		LogLevel: "debug",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agent == nil {
		t.Fatal("expected non-nil agent")
	}
}

func TestAndroidAgent_Lifecycle(t *testing.T) {
	agent, err := NewAndroidAgent(AgentConfig{
		AgentID: "lifecycle-agent",
		Model:   "claude-3-haiku",
		DataDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("create agent: %v", err)
	}

	ctx := context.Background()

	// Start
	if err := agent.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Double-start should be no-op
	if err := agent.Start(ctx); err != nil {
		t.Fatalf("second Start: %v", err)
	}

	// Status should show running
	var status map[string]interface{}
	if err := json.Unmarshal([]byte(agent.GetStatus()), &status); err != nil {
		t.Fatalf("parse status: %v", err)
	}
	if status["running"] != true {
		t.Errorf("expected running=true, got %v", status["running"])
	}
	if status["agent_id"] != "lifecycle-agent" {
		t.Errorf("expected agent_id=lifecycle-agent, got %v", status["agent_id"])
	}

	// Stop
	if err := agent.Stop(); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Double-stop should be no-op
	if err := agent.Stop(); err != nil {
		t.Fatalf("second Stop: %v", err)
	}

	// Status should show not running
	if err := json.Unmarshal([]byte(agent.GetStatus()), &status); err != nil {
		t.Fatalf("parse status: %v", err)
	}
	if status["running"] != false {
		t.Errorf("expected running=false after stop, got %v", status["running"])
	}
}

func TestAndroidAgent_HandleIntent_SendMessage(t *testing.T) {
	agent, _ := NewAndroidAgent(AgentConfig{AgentID: "intent-agent", DataDir: t.TempDir()})
	ctx := context.Background()
	_ = agent.Start(ctx)

	err := agent.HandleIntent("com.evoclaw.ACTION_SEND_MESSAGE", map[string]string{
		"message": "hello from Android",
	})
	if err != nil {
		t.Fatalf("HandleIntent: %v", err)
	}
}

func TestAndroidAgent_HandleIntent_MissingMessage(t *testing.T) {
	agent, _ := NewAndroidAgent(AgentConfig{AgentID: "intent-agent", DataDir: t.TempDir()})
	ctx := context.Background()
	_ = agent.Start(ctx)

	err := agent.HandleIntent("com.evoclaw.ACTION_SEND_MESSAGE", map[string]string{})
	if err == nil {
		t.Fatal("expected error for missing message extra")
	}
}

func TestAndroidAgent_HandleIntent_NotRunning(t *testing.T) {
	agent, _ := NewAndroidAgent(AgentConfig{AgentID: "stopped-agent", DataDir: t.TempDir()})

	err := agent.HandleIntent("com.evoclaw.ACTION_SEND_MESSAGE", map[string]string{"message": "hi"})
	if err == nil {
		t.Fatal("expected error when agent not running")
	}
}

func TestAndroidAgent_HandleIntent_Stop(t *testing.T) {
	agent, _ := NewAndroidAgent(AgentConfig{AgentID: "stop-agent", DataDir: t.TempDir()})
	ctx := context.Background()
	_ = agent.Start(ctx)

	if err := agent.HandleIntent("com.evoclaw.ACTION_STOP", nil); err != nil {
		t.Fatalf("HandleIntent STOP: %v", err)
	}

	var status map[string]interface{}
	_ = json.Unmarshal([]byte(agent.GetStatus()), &status)
	if status["running"] != false {
		t.Error("expected agent stopped after STOP intent")
	}
}

func TestAndroidAgent_HandleIntent_Unknown(t *testing.T) {
	agent, _ := NewAndroidAgent(AgentConfig{AgentID: "unk-agent", DataDir: t.TempDir()})
	ctx := context.Background()
	_ = agent.Start(ctx)

	// Unknown intents should not error
	if err := agent.HandleIntent("com.evoclaw.UNKNOWN", map[string]string{}); err != nil {
		t.Fatalf("unexpected error for unknown intent: %v", err)
	}
}
