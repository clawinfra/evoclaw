package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func newTestLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func newTestManagerServer(t *testing.T) *httptest.Server {
	t.Helper()

	var sandboxCount atomic.Int32

	mux := http.NewServeMux()

	mux.HandleFunc("POST /sandboxes", func(w http.ResponseWriter, r *http.Request) {
		idx := sandboxCount.Add(1)
		now := time.Now()
		var req e2bCreateRequest
		json.NewDecoder(r.Body).Decode(&req)

		resp := e2bSandboxResponse{
			SandboxID:  "sb-mgr-" + fmt.Sprintf("%d", idx),
			TemplateID: req.TemplateID,
			ClientID:   "cl-mgr-" + fmt.Sprintf("%d", idx),
			StartedAt:  now.Format(time.RFC3339),
			EndAt:      now.Add(time.Duration(req.Timeout) * time.Second).Format(time.RFC3339),
			Metadata:   req.Metadata,
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resp)
	})

	mux.HandleFunc("GET /sandboxes", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(e2bListResponse{})
	})

	mux.HandleFunc("DELETE /sandboxes/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	mux.HandleFunc("POST /sandboxes/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	return httptest.NewServer(mux)
}

// fmt is used for test sandbox ID generation

func newTestManager(t *testing.T) (*Manager, *httptest.Server) {
	t.Helper()
	server := newTestManagerServer(t)

	config := DefaultManagerConfig()
	config.E2BAPIKey = "test-mgr-key"
	config.MaxAgents = 5
	config.CreditBudgetUSD = 10.0
	config.HealthCheckIntervalSec = 3600 // Don't run health checks during tests
	config.KeepAliveIntervalSec = 3600

	client := NewE2BClient(config.E2BAPIKey)
	client.SetBaseURL(server.URL)

	mgr := NewManagerWithClient(config, client, newTestLogger())

	return mgr, server
}

func TestDefaultManagerConfig(t *testing.T) {
	cfg := DefaultManagerConfig()

	if cfg.DefaultTemplate != "evoclaw-agent" {
		t.Errorf("unexpected default template: %s", cfg.DefaultTemplate)
	}
	if cfg.DefaultTimeoutSec != 300 {
		t.Errorf("unexpected default timeout: %d", cfg.DefaultTimeoutSec)
	}
	if cfg.MaxAgents != 10 {
		t.Errorf("unexpected max agents: %d", cfg.MaxAgents)
	}
	if cfg.HealthCheckIntervalSec != 60 {
		t.Errorf("unexpected health check interval: %d", cfg.HealthCheckIntervalSec)
	}
	if cfg.CreditBudgetUSD != 50.0 {
		t.Errorf("unexpected credit budget: %f", cfg.CreditBudgetUSD)
	}
}

func TestNewManager(t *testing.T) {
	config := DefaultManagerConfig()
	config.E2BAPIKey = "test-key"

	mgr := NewManager(config, newTestLogger())
	if mgr == nil {
		t.Fatal("expected non-nil manager")
	}
	if mgr.IsStarted() {
		t.Error("manager should not be started initially")
	}
}

func TestManagerStart(t *testing.T) {
	mgr, server := newTestManager(t)
	defer server.Close()

	err := mgr.Start()
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	if !mgr.IsStarted() {
		t.Error("manager should be started")
	}

	// Double start should fail
	err = mgr.Start()
	if err == nil {
		t.Error("expected error for double start")
	}

	mgr.Stop()
}

func TestManagerStop(t *testing.T) {
	mgr, server := newTestManager(t)
	defer server.Close()

	// Stop before start should be ok
	err := mgr.Stop()
	if err != nil {
		t.Fatalf("Stop before start failed: %v", err)
	}

	mgr.Start()

	err = mgr.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if mgr.IsStarted() {
		t.Error("manager should not be started after stop")
	}
}

func TestManagerSpawnAgent(t *testing.T) {
	mgr, server := newTestManager(t)
	defer server.Close()

	ctx := context.Background()

	sandbox, err := mgr.SpawnAgent(ctx, AgentConfig{
		TemplateID: "evoclaw-agent",
		AgentID:    "mgr-agent-1",
		AgentType:  "trader",
	})
	if err != nil {
		t.Fatalf("SpawnAgent failed: %v", err)
	}

	if sandbox.SandboxID == "" {
		t.Error("expected non-empty sandbox ID")
	}
	if sandbox.AgentID != "mgr-agent-1" {
		t.Errorf("expected agent ID 'mgr-agent-1', got '%s'", sandbox.AgentID)
	}
}

func TestManagerSpawnAgent_Defaults(t *testing.T) {
	mgr, server := newTestManager(t)
	defer server.Close()

	ctx := context.Background()

	// Spawn with no template/broker/etc — should use defaults
	sandbox, err := mgr.SpawnAgent(ctx, AgentConfig{
		AgentID: "default-agent",
	})
	if err != nil {
		t.Fatalf("SpawnAgent with defaults failed: %v", err)
	}

	if sandbox.SandboxID == "" {
		t.Error("expected non-empty sandbox ID")
	}
}

func TestManagerSpawnAgent_Limit(t *testing.T) {
	mgr, server := newTestManager(t)
	defer server.Close()

	ctx := context.Background()

	// Spawn up to the limit
	for i := 0; i < 5; i++ {
		_, err := mgr.SpawnAgent(ctx, AgentConfig{
			AgentID: fmt.Sprintf("limit-agent-%d", i),
		})
		if err != nil {
			t.Fatalf("SpawnAgent %d failed: %v", i, err)
		}
	}

	// Next one should fail
	_, err := mgr.SpawnAgent(ctx, AgentConfig{
		AgentID: "over-limit",
	})
	if err == nil {
		t.Fatal("expected error for exceeding agent limit")
	}
}

func TestManagerSpawnAgent_BudgetExhausted(t *testing.T) {
	mgr, server := newTestManager(t)
	defer server.Close()

	// Exhaust the budget
	mgr.costs.mu.Lock()
	mgr.costs.estimatedCostUSD = 10.0
	mgr.costs.mu.Unlock()

	_, err := mgr.SpawnAgent(context.Background(), AgentConfig{
		AgentID: "budget-agent",
	})
	if err == nil {
		t.Fatal("expected error for exhausted budget")
	}
}

func TestManagerKillAgent(t *testing.T) {
	mgr, server := newTestManager(t)
	defer server.Close()

	ctx := context.Background()

	sandbox, err := mgr.SpawnAgent(ctx, AgentConfig{
		AgentID: "kill-agent",
	})
	if err != nil {
		t.Fatalf("SpawnAgent failed: %v", err)
	}

	err = mgr.KillAgent(ctx, sandbox.SandboxID)
	if err != nil {
		t.Fatalf("KillAgent failed: %v", err)
	}
}

func TestManagerListAgents(t *testing.T) {
	mgr, server := newTestManager(t)
	defer server.Close()

	sandboxes, err := mgr.ListAgents(context.Background())
	if err != nil {
		t.Fatalf("ListAgents failed: %v", err)
	}

	// The test server returns empty list
	if sandboxes == nil {
		// That's fine, empty is ok
	}
}

func TestManagerGetCosts(t *testing.T) {
	mgr, server := newTestManager(t)
	defer server.Close()

	costs := mgr.GetCosts()
	if costs.BudgetUSD != 10.0 {
		t.Errorf("expected budget 10.0, got %f", costs.BudgetUSD)
	}
	if costs.EstimatedCostUSD != 0 {
		t.Errorf("expected 0 estimated cost, got %f", costs.EstimatedCostUSD)
	}
	if costs.BudgetRemaining != 10.0 {
		t.Errorf("expected 10.0 remaining, got %f", costs.BudgetRemaining)
	}

	// Spawn an agent to affect costs
	mgr.SpawnAgent(context.Background(), AgentConfig{AgentID: "cost-agent"})

	costs = mgr.GetCosts()
	if costs.TotalSandboxes != 1 {
		t.Errorf("expected 1 total sandbox, got %d", costs.TotalSandboxes)
	}
	if costs.ActiveSandboxes != 1 {
		t.Errorf("expected 1 active sandbox, got %d", costs.ActiveSandboxes)
	}
}

func TestManagerIsBudgetExhausted(t *testing.T) {
	mgr, server := newTestManager(t)
	defer server.Close()

	if mgr.IsBudgetExhausted() {
		t.Error("budget should not be exhausted initially")
	}

	mgr.costs.mu.Lock()
	mgr.costs.estimatedCostUSD = 10.0
	mgr.costs.mu.Unlock()

	if !mgr.IsBudgetExhausted() {
		t.Error("budget should be exhausted")
	}
}

func TestManagerSpawnBurst(t *testing.T) {
	mgr, server := newTestManager(t)
	defer server.Close()

	configs := []AgentConfig{
		{AgentID: "burst-1"},
		{AgentID: "burst-2"},
		{AgentID: "burst-3"},
	}

	sandboxes, errors := mgr.SpawnBurst(context.Background(), configs)

	if len(sandboxes) != 3 {
		t.Fatalf("expected 3 sandboxes, got %d", len(sandboxes))
	}

	for i, err := range errors {
		if err != nil {
			t.Errorf("burst spawn %d failed: %v", i, err)
		}
	}

	for i, sb := range sandboxes {
		if sb == nil {
			t.Errorf("burst sandbox %d is nil", i)
		}
	}
}

func TestManagerKillAll(t *testing.T) {
	mgr, server := newTestManager(t)
	defer server.Close()

	killed, err := mgr.KillAll(context.Background())
	if err != nil {
		t.Fatalf("KillAll failed: %v", err)
	}

	// Test server returns empty list, so killed should be 0
	if killed != 0 {
		t.Errorf("expected 0 killed, got %d", killed)
	}
}

func TestCostSnapshot(t *testing.T) {
	snapshot := CostSnapshot{
		TotalSandboxes:   5,
		ActiveSandboxes:  3,
		TotalUptimeSec:   3600,
		EstimatedCostUSD: 0.36,
		BudgetUSD:        50.0,
		BudgetRemaining:  49.64,
	}

	if snapshot.TotalSandboxes != 5 {
		t.Errorf("unexpected total sandboxes: %d", snapshot.TotalSandboxes)
	}
	if snapshot.BudgetRemaining != 49.64 {
		t.Errorf("unexpected budget remaining: %f", snapshot.BudgetRemaining)
	}
}

func TestManagerSendCommand(t *testing.T) {
	// Create a server that handles the process endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/sandboxes" {
			now := time.Now()
			resp := e2bSandboxResponse{
				SandboxID: "sb-cmd-1",
				StartedAt: now.Format(time.RFC3339),
				EndAt:     now.Add(5 * time.Minute).Format(time.RFC3339),
			}
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(resp)
			return
		}
		if r.Method == http.MethodPost && r.URL.Path == "/sandboxes/sb-cmd-1/process" {
			json.NewEncoder(w).Encode(e2bProcessResponse{
				ExitCode: 0,
				Stdout:   "output",
				Stderr:   "",
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	config := DefaultManagerConfig()
	config.E2BAPIKey = "test-key"
	client := NewE2BClient(config.E2BAPIKey)
	client.SetBaseURL(server.URL)
	mgr := NewManagerWithClient(config, client, newTestLogger())

	resp, err := mgr.SendCommand(context.Background(), "sb-cmd-1", Command{Cmd: "echo", Args: []string{"hi"}})
	if err != nil {
		t.Fatalf("SendCommand failed: %v", err)
	}
	if resp.Stdout != "output" {
		t.Errorf("expected 'output', got '%s'", resp.Stdout)
	}
}

func TestManagerGetAgentStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		resp := e2bSandboxResponse{
			SandboxID: "sb-status-1",
			StartedAt: now.Add(-2 * time.Minute).Format(time.RFC3339),
			EndAt:     now.Add(3 * time.Minute).Format(time.RFC3339),
			Metadata:  map[string]string{"evoclaw_agent_id": "test-agent"},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	config := DefaultManagerConfig()
	config.E2BAPIKey = "test-key"
	client := NewE2BClient(config.E2BAPIKey)
	client.SetBaseURL(server.URL)
	mgr := NewManagerWithClient(config, client, newTestLogger())

	status, err := mgr.GetAgentStatus(context.Background(), "sb-status-1")
	if err != nil {
		t.Fatalf("GetAgentStatus failed: %v", err)
	}
	if status.SandboxID != "sb-status-1" {
		t.Errorf("expected 'sb-status-1', got '%s'", status.SandboxID)
	}
	if !status.Healthy {
		t.Error("expected healthy status")
	}
}

// end of tests
