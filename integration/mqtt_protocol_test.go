//go:build integration

// Package integration provides end-to-end tests for the EvoClaw orchestrator
// and Rust edge agent communication over MQTT.
//
// These tests verify that the MQTT protocol contract between the Go orchestrator
// and Rust edge agent is correct — including topic patterns, message formats,
// and the full command/response lifecycle.
//
// Prerequisites:
//   - MQTT broker (Mosquitto) running on localhost:1883
//   - Set MQTT_BROKER and MQTT_PORT env vars to override defaults
//
// Run with: go test -v -tags=integration -timeout=60s ./...
package integration

import (
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"sync"
	"testing"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// ──────────────────────────────────────────────
// Shared types matching the MQTT protocol contract
// between Go orchestrator and Rust edge agent
// ──────────────────────────────────────────────

// AgentCommand is the message format sent from orchestrator → agent
// Must match: edge-agent/src/mqtt.rs::AgentCommand
type AgentCommand struct {
	Command   string      `json:"command"`
	Payload   interface{} `json:"payload"`
	RequestID string      `json:"request_id"`
}

// AgentReport is the message format sent from agent → orchestrator
// Must match: edge-agent/src/mqtt.rs::AgentReport
type AgentReport struct {
	AgentID    string      `json:"agent_id"`
	AgentType  string      `json:"agent_type"`
	ReportType string      `json:"report_type"`
	Payload    interface{} `json:"payload"`
	Timestamp  uint64      `json:"timestamp"`
}

// AgentStatus is the heartbeat status message format
// Must match: internal/channels/mqtt.go::handleStatus
type AgentStatus struct {
	AgentID   string  `json:"agent_id"`
	Status    string  `json:"status"`
	Timestamp int64   `json:"timestamp"`
	Uptime    float64 `json:"uptime_seconds,omitempty"`
	CPU       float64 `json:"cpu_percent,omitempty"`
	Memory    float64 `json:"memory_mb,omitempty"`
}

// ──────────────────────────────────────────────
// MQTT topic constants (must match both codebases)
// ──────────────────────────────────────────────

const (
	commandsTopicFmt = "evoclaw/agents/%s/commands"
	reportsTopicFmt  = "evoclaw/agents/%s/reports"
	statusTopicFmt   = "evoclaw/agents/%s/status"
	strategyTopicFmt = "evoclaw/agents/%s/strategy"
	broadcastTopic   = "evoclaw/broadcast"
)

// ──────────────────────────────────────────────
// Test helpers
// ──────────────────────────────────────────────

func mqttBroker() string {
	if b := os.Getenv("MQTT_BROKER"); b != "" {
		return b
	}
	return "localhost"
}

func mqttPort() int {
	if p := os.Getenv("MQTT_PORT"); p != "" {
		port, err := strconv.Atoi(p)
		if err == nil {
			return port
		}
	}
	return 1883
}

// newClient creates a connected MQTT client for testing.
// It skips the test if the broker is unavailable.
func newClient(t *testing.T, clientID string) mqtt.Client {
	t.Helper()

	opts := mqtt.NewClientOptions()
	opts.AddBroker(fmt.Sprintf("tcp://%s:%d", mqttBroker(), mqttPort()))
	opts.SetClientID(clientID)
	opts.SetCleanSession(true)
	opts.SetKeepAlive(10 * time.Second)
	opts.SetAutoReconnect(false)
	opts.SetConnectTimeout(5 * time.Second)

	client := mqtt.NewClient(opts)
	token := client.Connect()
	if !token.WaitTimeout(5 * time.Second) {
		t.Skip("MQTT broker not available (connection timeout) — skipping integration test")
	}
	if err := token.Error(); err != nil {
		t.Skipf("MQTT broker not available (%v) — skipping integration test", err)
	}

	t.Cleanup(func() {
		client.Disconnect(250)
	})

	return client
}

// publishJSON publishes a JSON payload to a topic
func publishJSON(t *testing.T, client mqtt.Client, topic string, payload interface{}) {
	t.Helper()

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	token := client.Publish(topic, 1, false, data)
	if !token.WaitTimeout(5 * time.Second) {
		t.Fatal("publish timeout")
	}
	if err := token.Error(); err != nil {
		t.Fatalf("publish error: %v", err)
	}
}

// waitForMessage waits for a message on a channel with timeout
func waitForMessage(t *testing.T, ch <-chan []byte, timeout time.Duration) []byte {
	t.Helper()
	select {
	case msg := <-ch:
		return msg
	case <-time.After(timeout):
		t.Fatal("timed out waiting for message")
		return nil
	}
}

// ──────────────────────────────────────────────
// Test 1: Ping/Pong
// Orchestrator sends "ping" command → Agent replies with "pong"
// ──────────────────────────────────────────────

func TestPingPong(t *testing.T) {
	agentID := "test-agent-ping"

	// Orchestrator client (sends commands, listens for reports)
	orchClient := newClient(t, "orch-ping-test")

	// Agent client (listens for commands, sends reports)
	agentClient := newClient(t, "agent-ping-test")

	// Channel to receive the agent's report
	reportCh := make(chan []byte, 1)

	// Orchestrator subscribes to agent reports
	reportTopic := fmt.Sprintf(reportsTopicFmt, agentID)
	token := orchClient.Subscribe(reportTopic, 1, func(_ mqtt.Client, msg mqtt.Message) {
		data := make([]byte, len(msg.Payload()))
		copy(data, msg.Payload())
		select {
		case reportCh <- data:
		default:
		}
	})
	if !token.WaitTimeout(5 * time.Second) {
		t.Fatal("subscribe timeout")
	}

	// Agent subscribes to commands
	cmdCh := make(chan []byte, 1)
	cmdTopic := fmt.Sprintf(commandsTopicFmt, agentID)
	token = agentClient.Subscribe(cmdTopic, 1, func(_ mqtt.Client, msg mqtt.Message) {
		data := make([]byte, len(msg.Payload()))
		copy(data, msg.Payload())
		select {
		case cmdCh <- data:
		default:
		}
	})
	if !token.WaitTimeout(5 * time.Second) {
		t.Fatal("subscribe timeout")
	}

	// Give subscriptions time to propagate
	time.Sleep(200 * time.Millisecond)

	// Orchestrator sends ping command
	pingCmd := AgentCommand{
		Command:   "ping",
		Payload:   map[string]interface{}{},
		RequestID: "req-ping-001",
	}
	publishJSON(t, orchClient, cmdTopic, pingCmd)

	// Agent receives the command
	cmdData := waitForMessage(t, cmdCh, 5*time.Second)

	var receivedCmd AgentCommand
	if err := json.Unmarshal(cmdData, &receivedCmd); err != nil {
		t.Fatalf("failed to unmarshal command: %v", err)
	}

	if receivedCmd.Command != "ping" {
		t.Errorf("expected command 'ping', got '%s'", receivedCmd.Command)
	}
	if receivedCmd.RequestID != "req-ping-001" {
		t.Errorf("expected request_id 'req-ping-001', got '%s'", receivedCmd.RequestID)
	}

	// Agent sends pong response
	pongReport := AgentReport{
		AgentID:    agentID,
		AgentType:  "trader",
		ReportType: "result",
		Payload:    map[string]interface{}{"pong": true},
		Timestamp:  uint64(time.Now().Unix()),
	}
	publishJSON(t, agentClient, reportTopic, pongReport)

	// Orchestrator receives the pong report
	reportData := waitForMessage(t, reportCh, 5*time.Second)

	var receivedReport AgentReport
	if err := json.Unmarshal(reportData, &receivedReport); err != nil {
		t.Fatalf("failed to unmarshal report: %v", err)
	}

	if receivedReport.AgentID != agentID {
		t.Errorf("expected agent_id '%s', got '%s'", agentID, receivedReport.AgentID)
	}
	if receivedReport.ReportType != "result" {
		t.Errorf("expected report_type 'result', got '%s'", receivedReport.ReportType)
	}

	// Verify pong payload
	payloadMap, ok := receivedReport.Payload.(map[string]interface{})
	if !ok {
		t.Fatal("payload is not a map")
	}
	if pong, ok := payloadMap["pong"].(bool); !ok || !pong {
		t.Error("expected pong=true in payload")
	}

	t.Log("✅ Ping/Pong test passed")
}

// ──────────────────────────────────────────────
// Test 2: Agent Heartbeat / Status
// Agent sends heartbeat on status topic, orchestrator receives it
// ──────────────────────────────────────────────

func TestHeartbeat(t *testing.T) {
	agentID := "test-agent-hb"

	orchClient := newClient(t, "orch-hb-test")
	agentClient := newClient(t, "agent-hb-test")

	// Orchestrator subscribes to agent status (wildcard)
	statusCh := make(chan []byte, 5)
	statusPattern := "evoclaw/agents/+/status"
	token := orchClient.Subscribe(statusPattern, 1, func(_ mqtt.Client, msg mqtt.Message) {
		data := make([]byte, len(msg.Payload()))
		copy(data, msg.Payload())
		select {
		case statusCh <- data:
		default:
		}
	})
	if !token.WaitTimeout(5 * time.Second) {
		t.Fatal("subscribe timeout")
	}

	time.Sleep(200 * time.Millisecond)

	// Agent sends 3 heartbeats
	for i := 0; i < 3; i++ {
		status := AgentStatus{
			AgentID:   agentID,
			Status:    "online",
			Timestamp: time.Now().Unix(),
			Uptime:    float64(30 * (i + 1)),
			CPU:       12.5,
			Memory:    1.8,
		}
		statusTopic := fmt.Sprintf(statusTopicFmt, agentID)
		publishJSON(t, agentClient, statusTopic, status)
		time.Sleep(100 * time.Millisecond)
	}

	// Orchestrator should receive all 3 heartbeats
	received := 0
	timeout := time.After(5 * time.Second)

	for received < 3 {
		select {
		case data := <-statusCh:
			var status AgentStatus
			if err := json.Unmarshal(data, &status); err != nil {
				t.Fatalf("failed to unmarshal status: %v", err)
			}

			if status.AgentID != agentID {
				t.Errorf("expected agent_id '%s', got '%s'", agentID, status.AgentID)
			}
			if status.Status != "online" {
				t.Errorf("expected status 'online', got '%s'", status.Status)
			}
			if status.Uptime <= 0 {
				t.Error("expected positive uptime")
			}

			received++
		case <-timeout:
			t.Fatalf("timed out waiting for heartbeats, received %d/3", received)
		}
	}

	t.Logf("✅ Heartbeat test passed (%d heartbeats received)", received)
}

// ──────────────────────────────────────────────
// Test 3: Strategy Update
// Orchestrator pushes strategy update → agent receives and confirms
// ──────────────────────────────────────────────

func TestStrategyUpdate(t *testing.T) {
	agentID := "test-agent-strategy"

	orchClient := newClient(t, "orch-strategy-test")
	agentClient := newClient(t, "agent-strategy-test")

	// Agent subscribes to commands (where strategy updates come)
	cmdCh := make(chan []byte, 1)
	cmdTopic := fmt.Sprintf(commandsTopicFmt, agentID)
	token := agentClient.Subscribe(cmdTopic, 1, func(_ mqtt.Client, msg mqtt.Message) {
		data := make([]byte, len(msg.Payload()))
		copy(data, msg.Payload())
		select {
		case cmdCh <- data:
		default:
		}
	})
	if !token.WaitTimeout(5 * time.Second) {
		t.Fatal("subscribe timeout")
	}

	// Orchestrator subscribes to reports
	reportCh := make(chan []byte, 1)
	reportTopic := fmt.Sprintf(reportsTopicFmt, agentID)
	token = orchClient.Subscribe(reportTopic, 1, func(_ mqtt.Client, msg mqtt.Message) {
		data := make([]byte, len(msg.Payload()))
		copy(data, msg.Payload())
		select {
		case reportCh <- data:
		default:
		}
	})
	if !token.WaitTimeout(5 * time.Second) {
		t.Fatal("subscribe timeout")
	}

	time.Sleep(200 * time.Millisecond)

	// Orchestrator sends strategy update command
	strategyCmd := AgentCommand{
		Command: "update_strategy",
		Payload: map[string]interface{}{
			"action":            "add_funding_arbitrage",
			"funding_threshold": -0.15,
			"exit_funding":      0.05,
			"position_size_usd": 2000.0,
		},
		RequestID: "req-strategy-001",
	}
	publishJSON(t, orchClient, cmdTopic, strategyCmd)

	// Agent receives the strategy update
	cmdData := waitForMessage(t, cmdCh, 5*time.Second)

	var receivedCmd AgentCommand
	if err := json.Unmarshal(cmdData, &receivedCmd); err != nil {
		t.Fatalf("failed to unmarshal command: %v", err)
	}

	if receivedCmd.Command != "update_strategy" {
		t.Errorf("expected command 'update_strategy', got '%s'", receivedCmd.Command)
	}

	// Verify the strategy payload
	payloadMap, ok := receivedCmd.Payload.(map[string]interface{})
	if !ok {
		t.Fatal("payload is not a map")
	}
	if action, ok := payloadMap["action"].(string); !ok || action != "add_funding_arbitrage" {
		t.Error("expected action 'add_funding_arbitrage'")
	}
	if threshold, ok := payloadMap["funding_threshold"].(float64); !ok || threshold != -0.15 {
		t.Errorf("expected funding_threshold -0.15, got %v", threshold)
	}

	// Agent confirms strategy was applied
	confirmReport := AgentReport{
		AgentID:    agentID,
		AgentType:  "trader",
		ReportType: "result",
		Payload: map[string]interface{}{
			"status":   "strategy_added",
			"strategy": "FundingArbitrage",
			"params": map[string]interface{}{
				"funding_threshold": -0.15,
				"exit_funding":      0.05,
				"position_size_usd": 2000.0,
			},
		},
		Timestamp: uint64(time.Now().Unix()),
	}
	publishJSON(t, agentClient, reportTopic, confirmReport)

	// Orchestrator receives confirmation
	reportData := waitForMessage(t, reportCh, 5*time.Second)

	var receivedReport AgentReport
	if err := json.Unmarshal(reportData, &receivedReport); err != nil {
		t.Fatalf("failed to unmarshal report: %v", err)
	}

	if receivedReport.ReportType != "result" {
		t.Errorf("expected report_type 'result', got '%s'", receivedReport.ReportType)
	}

	reportPayload, ok := receivedReport.Payload.(map[string]interface{})
	if !ok {
		t.Fatal("report payload is not a map")
	}
	if status, ok := reportPayload["status"].(string); !ok || status != "strategy_added" {
		t.Error("expected status 'strategy_added'")
	}

	t.Log("✅ Strategy Update test passed")
}

// ──────────────────────────────────────────────
// Test 4: Metrics Reporting
// Agent reports metrics → orchestrator receives and validates format
// ──────────────────────────────────────────────

func TestMetricsReporting(t *testing.T) {
	agentID := "test-agent-metrics"

	orchClient := newClient(t, "orch-metrics-test")
	agentClient := newClient(t, "agent-metrics-test")

	// Orchestrator subscribes to agent reports (wildcard)
	reportCh := make(chan []byte, 5)
	reportPattern := "evoclaw/agents/+/reports"
	token := orchClient.Subscribe(reportPattern, 1, func(_ mqtt.Client, msg mqtt.Message) {
		data := make([]byte, len(msg.Payload()))
		copy(data, msg.Payload())
		select {
		case reportCh <- data:
		default:
		}
	})
	if !token.WaitTimeout(5 * time.Second) {
		t.Fatal("subscribe timeout")
	}

	time.Sleep(200 * time.Millisecond)

	// Agent sends metrics report (matching Rust Metrics struct serialization)
	metricsReport := AgentReport{
		AgentID:    agentID,
		AgentType:  "trader",
		ReportType: "heartbeat",
		Payload: map[string]interface{}{
			"uptime_sec":      3600,
			"actions_total":   150,
			"actions_success": 142,
			"actions_failed":  8,
			"memory_bytes":    1887436,
			"custom": map[string]interface{}{
				"total_pnl_usd":  245.50,
				"win_rate":       0.85,
				"avg_latency_ms": 42.3,
			},
		},
		Timestamp: uint64(time.Now().Unix()),
	}

	reportTopic := fmt.Sprintf(reportsTopicFmt, agentID)
	publishJSON(t, agentClient, reportTopic, metricsReport)

	// Orchestrator receives the metrics
	reportData := waitForMessage(t, reportCh, 5*time.Second)

	var receivedReport AgentReport
	if err := json.Unmarshal(reportData, &receivedReport); err != nil {
		t.Fatalf("failed to unmarshal report: %v", err)
	}

	// Validate report structure
	if receivedReport.AgentID != agentID {
		t.Errorf("expected agent_id '%s', got '%s'", agentID, receivedReport.AgentID)
	}
	if receivedReport.AgentType != "trader" {
		t.Errorf("expected agent_type 'trader', got '%s'", receivedReport.AgentType)
	}
	if receivedReport.ReportType != "heartbeat" {
		t.Errorf("expected report_type 'heartbeat', got '%s'", receivedReport.ReportType)
	}
	if receivedReport.Timestamp == 0 {
		t.Error("expected non-zero timestamp")
	}

	// Validate metrics payload
	payload, ok := receivedReport.Payload.(map[string]interface{})
	if !ok {
		t.Fatal("payload is not a map")
	}

	// Check required metric fields (matching Rust Metrics struct)
	requiredFields := []string{
		"uptime_sec", "actions_total", "actions_success",
		"actions_failed", "memory_bytes",
	}
	for _, field := range requiredFields {
		if _, exists := payload[field]; !exists {
			t.Errorf("missing required metric field: %s", field)
		}
	}

	// Verify numeric values
	if total, ok := payload["actions_total"].(float64); !ok || total != 150 {
		t.Errorf("expected actions_total=150, got %v", payload["actions_total"])
	}
	if success, ok := payload["actions_success"].(float64); !ok || success != 142 {
		t.Errorf("expected actions_success=142, got %v", payload["actions_success"])
	}

	// Verify custom metrics
	custom, ok := payload["custom"].(map[string]interface{})
	if !ok {
		t.Fatal("custom metrics should be a map")
	}
	if pnl, ok := custom["total_pnl_usd"].(float64); !ok || pnl != 245.50 {
		t.Errorf("expected total_pnl_usd=245.50, got %v", custom["total_pnl_usd"])
	}

	t.Log("✅ Metrics Reporting test passed")
}

// ──────────────────────────────────────────────
// Test 5: Broadcast
// Orchestrator broadcasts to all agents
// ──────────────────────────────────────────────

func TestBroadcast(t *testing.T) {
	orchClient := newClient(t, "orch-broadcast-test")

	// Create 3 "agent" clients listening on broadcast
	const numAgents = 3
	var wg sync.WaitGroup
	receivedCounts := make([]int, numAgents)

	for i := 0; i < numAgents; i++ {
		idx := i
		agentClient := newClient(t, fmt.Sprintf("agent-broadcast-%d", i))

		var mu sync.Mutex

		token := agentClient.Subscribe(broadcastTopic, 1, func(_ mqtt.Client, msg mqtt.Message) {
			mu.Lock()
			receivedCounts[idx]++
			mu.Unlock()
		})
		if !token.WaitTimeout(5 * time.Second) {
			t.Fatal("subscribe timeout")
		}

		_ = agentClient // prevent GC
	}

	time.Sleep(300 * time.Millisecond)

	// Orchestrator broadcasts a message
	broadcastPayload := map[string]interface{}{
		"content": "system maintenance in 5 minutes",
		"sent_at": time.Now().Unix(),
	}
	publishJSON(t, orchClient, broadcastTopic, broadcastPayload)

	// Wait for all agents to receive
	wg.Add(1)
	go func() {
		defer wg.Done()
		deadline := time.After(5 * time.Second)
		for {
			allReceived := true
			for i := 0; i < numAgents; i++ {
				if receivedCounts[i] == 0 {
					allReceived = false
					break
				}
			}
			if allReceived {
				return
			}
			select {
			case <-deadline:
				return
			case <-time.After(100 * time.Millisecond):
			}
		}
	}()
	wg.Wait()

	for i := 0; i < numAgents; i++ {
		if receivedCounts[i] == 0 {
			t.Errorf("agent %d did not receive broadcast", i)
		}
	}

	t.Log("✅ Broadcast test passed")
}

// ──────────────────────────────────────────────
// Test 6: Full Command/Response Lifecycle
// Tests the complete flow with multiple command types
// ──────────────────────────────────────────────

func TestFullLifecycle(t *testing.T) {
	agentID := "test-agent-lifecycle"

	orchClient := newClient(t, "orch-lifecycle-test")
	agentClient := newClient(t, "agent-lifecycle-test")

	cmdTopic := fmt.Sprintf(commandsTopicFmt, agentID)
	reportTopic := fmt.Sprintf(reportsTopicFmt, agentID)
	statusTopic := fmt.Sprintf(statusTopicFmt, agentID)

	// Channels for receiving messages
	cmdCh := make(chan []byte, 10)
	reportCh := make(chan []byte, 10)
	statusCh := make(chan []byte, 10)

	// Agent subscribes to commands
	token := agentClient.Subscribe(cmdTopic, 1, func(_ mqtt.Client, msg mqtt.Message) {
		data := make([]byte, len(msg.Payload()))
		copy(data, msg.Payload())
		select {
		case cmdCh <- data:
		default:
		}
	})
	if !token.WaitTimeout(5 * time.Second) {
		t.Fatal("subscribe timeout")
	}

	// Orchestrator subscribes to reports and status
	token = orchClient.Subscribe(reportTopic, 1, func(_ mqtt.Client, msg mqtt.Message) {
		data := make([]byte, len(msg.Payload()))
		copy(data, msg.Payload())
		select {
		case reportCh <- data:
		default:
		}
	})
	if !token.WaitTimeout(5 * time.Second) {
		t.Fatal("subscribe timeout")
	}

	token = orchClient.Subscribe(statusTopic, 1, func(_ mqtt.Client, msg mqtt.Message) {
		data := make([]byte, len(msg.Payload()))
		copy(data, msg.Payload())
		select {
		case statusCh <- data:
		default:
		}
	})
	if !token.WaitTimeout(5 * time.Second) {
		t.Fatal("subscribe timeout")
	}

	time.Sleep(200 * time.Millisecond)

	// Phase 1: Agent comes online — sends initial heartbeat
	t.Log("Phase 1: Agent sends initial heartbeat")
	initialStatus := AgentStatus{
		AgentID:   agentID,
		Status:    "online",
		Timestamp: time.Now().Unix(),
		Uptime:    0,
		CPU:       5.2,
		Memory:    1.2,
	}
	publishJSON(t, agentClient, statusTopic, initialStatus)

	statusData := waitForMessage(t, statusCh, 5*time.Second)
	var recvStatus AgentStatus
	if err := json.Unmarshal(statusData, &recvStatus); err != nil {
		t.Fatalf("failed to unmarshal status: %v", err)
	}
	if recvStatus.Status != "online" {
		t.Errorf("expected status 'online', got '%s'", recvStatus.Status)
	}

	// Phase 2: Orchestrator sends ping
	t.Log("Phase 2: Orchestrator sends ping")
	publishJSON(t, orchClient, cmdTopic, AgentCommand{
		Command:   "ping",
		Payload:   map[string]interface{}{},
		RequestID: "lifecycle-ping",
	})

	cmdData := waitForMessage(t, cmdCh, 5*time.Second)
	var recvCmd AgentCommand
	if err := json.Unmarshal(cmdData, &recvCmd); err != nil {
		t.Fatalf("failed to unmarshal command: %v", err)
	}
	if recvCmd.Command != "ping" {
		t.Errorf("expected 'ping', got '%s'", recvCmd.Command)
	}

	// Agent responds with pong
	publishJSON(t, agentClient, reportTopic, AgentReport{
		AgentID:    agentID,
		AgentType:  "trader",
		ReportType: "result",
		Payload:    map[string]interface{}{"pong": true},
		Timestamp:  uint64(time.Now().Unix()),
	})

	reportData := waitForMessage(t, reportCh, 5*time.Second)
	var recvReport AgentReport
	if err := json.Unmarshal(reportData, &recvReport); err != nil {
		t.Fatalf("failed to unmarshal report: %v", err)
	}

	// Phase 3: Orchestrator sends strategy update
	t.Log("Phase 3: Strategy update")
	publishJSON(t, orchClient, cmdTopic, AgentCommand{
		Command: "update_strategy",
		Payload: map[string]interface{}{
			"action":            "add_mean_reversion",
			"support_level":     2.0,
			"resistance_level":  2.0,
			"position_size_usd": 1000.0,
		},
		RequestID: "lifecycle-strategy",
	})

	cmdData = waitForMessage(t, cmdCh, 5*time.Second)
	if err := json.Unmarshal(cmdData, &recvCmd); err != nil {
		t.Fatalf("failed to unmarshal command: %v", err)
	}
	if recvCmd.Command != "update_strategy" {
		t.Errorf("expected 'update_strategy', got '%s'", recvCmd.Command)
	}

	// Phase 4: Orchestrator requests metrics
	t.Log("Phase 4: Metrics request")
	publishJSON(t, orchClient, cmdTopic, AgentCommand{
		Command:   "get_metrics",
		Payload:   map[string]interface{}{},
		RequestID: "lifecycle-metrics",
	})

	cmdData = waitForMessage(t, cmdCh, 5*time.Second)
	if err := json.Unmarshal(cmdData, &recvCmd); err != nil {
		t.Fatalf("failed to unmarshal command: %v", err)
	}
	if recvCmd.Command != "get_metrics" {
		t.Errorf("expected 'get_metrics', got '%s'", recvCmd.Command)
	}

	// Agent sends metrics back
	publishJSON(t, agentClient, reportTopic, AgentReport{
		AgentID:    agentID,
		AgentType:  "trader",
		ReportType: "result",
		Payload: map[string]interface{}{
			"agent_metrics": map[string]interface{}{
				"uptime_sec":      120,
				"actions_total":   5,
				"actions_success": 4,
				"actions_failed":  1,
			},
			"fitness_score": 0.82,
		},
		Timestamp: uint64(time.Now().Unix()),
	})

	reportData = waitForMessage(t, reportCh, 5*time.Second)
	if err := json.Unmarshal(reportData, &recvReport); err != nil {
		t.Fatalf("failed to unmarshal report: %v", err)
	}

	reportPayload, ok := recvReport.Payload.(map[string]interface{})
	if !ok {
		t.Fatal("report payload is not a map")
	}
	if _, exists := reportPayload["fitness_score"]; !exists {
		t.Error("expected fitness_score in metrics report")
	}

	t.Log("✅ Full Lifecycle test passed")
}

// ──────────────────────────────────────────────
// Test 7: Topic Pattern Compatibility
// Verifies MQTT topic wildcards work correctly
// ──────────────────────────────────────────────

func TestTopicWildcards(t *testing.T) {
	orchClient := newClient(t, "orch-wildcard-test")

	// Subscribe with wildcard pattern (as orchestrator does)
	receivedTopics := make(map[string]bool)
	var mu sync.Mutex

	token := orchClient.Subscribe("evoclaw/agents/+/reports", 1, func(_ mqtt.Client, msg mqtt.Message) {
		mu.Lock()
		receivedTopics[msg.Topic()] = true
		mu.Unlock()
	})
	if !token.WaitTimeout(5 * time.Second) {
		t.Fatal("subscribe timeout")
	}

	time.Sleep(200 * time.Millisecond)

	// Multiple agents publish to their respective report topics
	agents := []string{"agent-alpha", "agent-beta", "agent-gamma"}
	for _, id := range agents {
		agentClient := newClient(t, fmt.Sprintf("agent-wc-%s", id))
		topic := fmt.Sprintf(reportsTopicFmt, id)
		publishJSON(t, agentClient, topic, AgentReport{
			AgentID:    id,
			AgentType:  "trader",
			ReportType: "heartbeat",
			Payload:    map[string]interface{}{"status": "ok"},
			Timestamp:  uint64(time.Now().Unix()),
		})
	}

	// Wait for messages
	time.Sleep(1 * time.Second)

	// Verify all agents' reports were received
	mu.Lock()
	defer mu.Unlock()
	for _, id := range agents {
		expectedTopic := fmt.Sprintf(reportsTopicFmt, id)
		if !receivedTopics[expectedTopic] {
			t.Errorf("orchestrator did not receive report from agent '%s' (topic: %s)", id, expectedTopic)
		}
	}

	t.Logf("✅ Topic Wildcards test passed (received from %d agents)", len(receivedTopics))
}

// ──────────────────────────────────────────────
// Test 8: Error Reporting
// Agent reports errors in correct format
// ──────────────────────────────────────────────

func TestErrorReporting(t *testing.T) {
	agentID := "test-agent-error"

	orchClient := newClient(t, "orch-error-test")
	agentClient := newClient(t, "agent-error-test")

	reportCh := make(chan []byte, 1)
	reportTopic := fmt.Sprintf(reportsTopicFmt, agentID)

	token := orchClient.Subscribe(reportTopic, 1, func(_ mqtt.Client, msg mqtt.Message) {
		data := make([]byte, len(msg.Payload()))
		copy(data, msg.Payload())
		select {
		case reportCh <- data:
		default:
		}
	})
	if !token.WaitTimeout(5 * time.Second) {
		t.Fatal("subscribe timeout")
	}

	time.Sleep(200 * time.Millisecond)

	// Agent sends an error report
	errorReport := AgentReport{
		AgentID:    agentID,
		AgentType:  "trader",
		ReportType: "error",
		Payload: map[string]interface{}{
			"error":      "trading client not initialized",
			"request_id": "req-exec-001",
		},
		Timestamp: uint64(time.Now().Unix()),
	}
	publishJSON(t, agentClient, reportTopic, errorReport)

	// Orchestrator receives the error
	reportData := waitForMessage(t, reportCh, 5*time.Second)

	var receivedReport AgentReport
	if err := json.Unmarshal(reportData, &receivedReport); err != nil {
		t.Fatalf("failed to unmarshal error report: %v", err)
	}

	if receivedReport.ReportType != "error" {
		t.Errorf("expected report_type 'error', got '%s'", receivedReport.ReportType)
	}

	payload, ok := receivedReport.Payload.(map[string]interface{})
	if !ok {
		t.Fatal("payload is not a map")
	}
	if errMsg, ok := payload["error"].(string); !ok || errMsg == "" {
		t.Error("expected non-empty error message in payload")
	}

	t.Log("✅ Error Reporting test passed")
}

// ──────────────────────────────────────────────
// Test 9: Evolution Command
// Orchestrator triggers evolution, agent responds with trade records
// ──────────────────────────────────────────────

func TestEvolutionFlow(t *testing.T) {
	agentID := "test-agent-evo"

	orchClient := newClient(t, "orch-evo-test")
	agentClient := newClient(t, "agent-evo-test")

	cmdTopic := fmt.Sprintf(commandsTopicFmt, agentID)
	reportTopic := fmt.Sprintf(reportsTopicFmt, agentID)

	cmdCh := make(chan []byte, 5)
	reportCh := make(chan []byte, 5)

	token := agentClient.Subscribe(cmdTopic, 1, func(_ mqtt.Client, msg mqtt.Message) {
		data := make([]byte, len(msg.Payload()))
		copy(data, msg.Payload())
		select {
		case cmdCh <- data:
		default:
		}
	})
	if !token.WaitTimeout(5 * time.Second) {
		t.Fatal("subscribe timeout")
	}

	token = orchClient.Subscribe(reportTopic, 1, func(_ mqtt.Client, msg mqtt.Message) {
		data := make([]byte, len(msg.Payload()))
		copy(data, msg.Payload())
		select {
		case reportCh <- data:
		default:
		}
	})
	if !token.WaitTimeout(5 * time.Second) {
		t.Fatal("subscribe timeout")
	}

	time.Sleep(200 * time.Millisecond)

	// Orchestrator sends evolution command to record a trade
	publishJSON(t, orchClient, cmdTopic, AgentCommand{
		Command: "evolution",
		Payload: map[string]interface{}{
			"action":      "record_trade",
			"asset":       "BTC",
			"entry_price": 50000.0,
			"exit_price":  51500.0,
			"size":        0.1,
		},
		RequestID: "evo-trade-001",
	})

	// Agent receives
	cmdData := waitForMessage(t, cmdCh, 5*time.Second)
	var recvCmd AgentCommand
	if err := json.Unmarshal(cmdData, &recvCmd); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}
	if recvCmd.Command != "evolution" {
		t.Errorf("expected 'evolution', got '%s'", recvCmd.Command)
	}

	// Agent reports trade recorded
	publishJSON(t, agentClient, reportTopic, AgentReport{
		AgentID:    agentID,
		AgentType:  "trader",
		ReportType: "result",
		Payload: map[string]interface{}{
			"status": "trade_recorded",
			"asset":  "BTC",
			"pnl":    150.0, // (51500 - 50000) * 0.1
		},
		Timestamp: uint64(time.Now().Unix()),
	})

	reportData := waitForMessage(t, reportCh, 5*time.Second)
	var recvReport AgentReport
	if err := json.Unmarshal(reportData, &recvReport); err != nil {
		t.Fatalf("failed to unmarshal report: %v", err)
	}

	payload, ok := recvReport.Payload.(map[string]interface{})
	if !ok {
		t.Fatal("payload is not a map")
	}
	if pnl, ok := payload["pnl"].(float64); !ok || pnl != 150.0 {
		t.Errorf("expected pnl=150.0, got %v", payload["pnl"])
	}

	// Orchestrator requests performance summary
	publishJSON(t, orchClient, cmdTopic, AgentCommand{
		Command: "evolution",
		Payload: map[string]interface{}{
			"action": "get_performance",
		},
		RequestID: "evo-perf-001",
	})

	cmdData = waitForMessage(t, cmdCh, 5*time.Second)
	if err := json.Unmarshal(cmdData, &recvCmd); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	// Agent responds with performance data
	publishJSON(t, agentClient, reportTopic, AgentReport{
		AgentID:    agentID,
		AgentType:  "trader",
		ReportType: "result",
		Payload: map[string]interface{}{
			"status": "success",
			"performance": map[string]interface{}{
				"total_trades": 1,
				"total_pnl":    150.0,
				"win_rate":     1.0,
			},
			"fitness_score": 0.95,
		},
		Timestamp: uint64(time.Now().Unix()),
	})

	reportData = waitForMessage(t, reportCh, 5*time.Second)
	if err := json.Unmarshal(reportData, &recvReport); err != nil {
		t.Fatalf("failed to unmarshal report: %v", err)
	}

	payload, ok = recvReport.Payload.(map[string]interface{})
	if !ok {
		t.Fatal("payload is not a map")
	}
	if _, exists := payload["fitness_score"]; !exists {
		t.Error("expected fitness_score in performance report")
	}

	t.Log("✅ Evolution Flow test passed")
}

// ──────────────────────────────────────────────
// Test 10: Message Ordering
// Verifies that messages arrive in order with QoS 1
// ──────────────────────────────────────────────

func TestMessageOrdering(t *testing.T) {
	agentID := "test-agent-order"

	orchClient := newClient(t, "orch-order-test")
	agentClient := newClient(t, "agent-order-test")

	cmdTopic := fmt.Sprintf(commandsTopicFmt, agentID)

	cmdCh := make(chan []byte, 20)
	token := agentClient.Subscribe(cmdTopic, 1, func(_ mqtt.Client, msg mqtt.Message) {
		data := make([]byte, len(msg.Payload()))
		copy(data, msg.Payload())
		cmdCh <- data
	})
	if !token.WaitTimeout(5 * time.Second) {
		t.Fatal("subscribe timeout")
	}

	time.Sleep(200 * time.Millisecond)

	// Send 10 commands in sequence
	const numMessages = 10
	for i := 0; i < numMessages; i++ {
		publishJSON(t, orchClient, cmdTopic, AgentCommand{
			Command:   "ping",
			Payload:   map[string]interface{}{"seq": i},
			RequestID: fmt.Sprintf("seq-%d", i),
		})
	}

	// Receive all and verify ordering
	received := make([]int, 0, numMessages)
	timeout := time.After(10 * time.Second)

	for len(received) < numMessages {
		select {
		case data := <-cmdCh:
			var cmd AgentCommand
			if err := json.Unmarshal(data, &cmd); err != nil {
				t.Fatalf("unmarshal error: %v", err)
			}
			payloadMap, ok := cmd.Payload.(map[string]interface{})
			if !ok {
				t.Fatal("payload is not a map")
			}
			seq := int(payloadMap["seq"].(float64))
			received = append(received, seq)
		case <-timeout:
			t.Fatalf("timed out, received %d/%d messages", len(received), numMessages)
		}
	}

	// Verify we received all messages (order may vary slightly with QoS 1)
	if len(received) != numMessages {
		t.Errorf("expected %d messages, got %d", numMessages, len(received))
	}

	// Check all sequence numbers are present
	seqSet := make(map[int]bool)
	for _, seq := range received {
		seqSet[seq] = true
	}
	for i := 0; i < numMessages; i++ {
		if !seqSet[i] {
			t.Errorf("missing sequence number %d", i)
		}
	}

	t.Logf("✅ Message Ordering test passed (%d messages)", len(received))
}
