package wasm

import (
	"context"
	"encoding/json"
	"testing"
)

func TestNewWASMAgent_MissingAgentID(t *testing.T) {
	_, err := NewWASMAgent(AgentConfig{})
	if err == nil {
		t.Fatal("expected error for missing AgentID")
	}
}

func TestNewWASMAgent_Valid(t *testing.T) {
	agent, err := NewWASMAgent(AgentConfig{
		AgentID:  "wasm-agent",
		Model:    "claude-3-haiku",
		LogLevel: "debug",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if agent == nil {
		t.Fatal("expected non-nil agent")
	}
}

func TestWASMAgent_Lifecycle(t *testing.T) {
	agent, err := NewWASMAgent(AgentConfig{AgentID: "lifecycle-wasm", Model: "claude-3-haiku"})
	if err != nil {
		t.Fatalf("create: %v", err)
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

func TestWASMAgent_MessageHandling(t *testing.T) {
	agent, _ := NewWASMAgent(AgentConfig{AgentID: "msg-wasm", Model: "claude-3-haiku"})
	ctx := context.Background()
	_ = agent.Start(ctx)

	resp := agent.SendMessage("hello wasm")

	var result map[string]string
	if err := json.Unmarshal([]byte(resp), &result); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if result["status"] != "ok" {
		t.Errorf("expected status=ok, got %q", result["status"])
	}
	if result["agent_id"] != "msg-wasm" {
		t.Errorf("expected agent_id=msg-wasm, got %q", result["agent_id"])
	}
	if result["echo"] != "hello wasm" {
		t.Errorf("expected echo=hello wasm, got %q", result["echo"])
	}
}

func TestWASMAgent_MessageHandling_NotRunning(t *testing.T) {
	agent, _ := NewWASMAgent(AgentConfig{AgentID: "stopped-wasm"})

	resp := agent.SendMessage("hello")
	var result map[string]string
	_ = json.Unmarshal([]byte(resp), &result)
	if result["error"] == "" {
		t.Error("expected error response when not running")
	}
}

func TestWASMAgent_GetStatus(t *testing.T) {
	agent, _ := NewWASMAgent(AgentConfig{AgentID: "status-wasm", Model: "claude-3-haiku"})
	ctx := context.Background()
	_ = agent.Start(ctx)

	var status map[string]interface{}
	if err := json.Unmarshal([]byte(agent.GetStatus()), &status); err != nil {
		t.Fatalf("parse status: %v", err)
	}
	if status["agent_id"] != "status-wasm" {
		t.Errorf("wrong agent_id: %v", status["agent_id"])
	}
	if status["model"] != "claude-3-haiku" {
		t.Errorf("wrong model: %v", status["model"])
	}
}
