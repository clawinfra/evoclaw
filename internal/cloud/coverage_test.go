package cloud

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// TestRunHealthCheck directly tests the health check logic.
func TestRunHealthCheck(t *testing.T) {
	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/sandboxes":
			resp := e2bListResponse{
				{
					SandboxID:  "sb-hc-1",
					TemplateID: "evoclaw-agent",
					StartedAt:  now.Add(-5 * time.Minute).Format(time.RFC3339),
					EndAt:      now.Add(5 * time.Minute).Format(time.RFC3339),
					Metadata:   map[string]string{"evoclaw_agent_id": "agent-hc-1"},
				},
			}
			json.NewEncoder(w).Encode(resp)
		case r.Method == http.MethodGet && r.URL.Path == "/sandboxes/sb-hc-1":
			resp := e2bSandboxResponse{
				SandboxID: "sb-hc-1",
				StartedAt: now.Add(-5 * time.Minute).Format(time.RFC3339),
				EndAt:     now.Add(5 * time.Minute).Format(time.RFC3339),
				Metadata:  map[string]string{"evoclaw_agent_id": "agent-hc-1"},
			}
			json.NewEncoder(w).Encode(resp)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	config := DefaultManagerConfig()
	config.E2BAPIKey = "test-key"
	client := NewE2BClient(config.E2BAPIKey)
	client.SetBaseURL(server.URL)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := NewManagerWithClient(config, client, logger)

	// Run health check directly
	mgr.runHealthCheck()
}

// TestRunHealthCheck_Error tests health check when list fails.
func TestRunHealthCheck_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	config := DefaultManagerConfig()
	config.E2BAPIKey = "test-key"
	client := NewE2BClient(config.E2BAPIKey)
	client.SetBaseURL(server.URL)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := NewManagerWithClient(config, client, logger)

	mgr.runHealthCheck() // Should not panic
}

// TestRunHealthCheck_StatusError tests health check when status fails.
func TestRunHealthCheck_StatusError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/sandboxes" {
			now := time.Now()
			resp := e2bListResponse{
				{
					SandboxID: "sb-hc-err",
					StartedAt: now.Format(time.RFC3339),
					EndAt:     now.Add(5 * time.Minute).Format(time.RFC3339),
					Metadata:  map[string]string{"evoclaw_agent_id": "agent-err"},
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}
		// Status endpoint returns error
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(e2bErrorResponse{Code: 404, Message: "not found"})
	}))
	defer server.Close()

	config := DefaultManagerConfig()
	config.E2BAPIKey = "test-key"
	client := NewE2BClient(config.E2BAPIKey)
	client.SetBaseURL(server.URL)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := NewManagerWithClient(config, client, logger)

	mgr.runHealthCheck() // Should not panic, logs warning
}

// TestRefreshTimeouts directly tests the keep-alive logic.
func TestRefreshTimeouts(t *testing.T) {
	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/sandboxes":
			resp := e2bListResponse{
				{
					SandboxID: "sb-ka-1",
					StartedAt: now.Add(-2 * time.Minute).Format(time.RFC3339),
					EndAt:     now.Add(3 * time.Minute).Format(time.RFC3339),
					Metadata:  map[string]string{"evoclaw_agent_id": "agent-ka-1"},
				},
			}
			json.NewEncoder(w).Encode(resp)
		case r.Method == http.MethodPost:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	config := DefaultManagerConfig()
	config.E2BAPIKey = "test-key"
	client := NewE2BClient(config.E2BAPIKey)
	client.SetBaseURL(server.URL)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := NewManagerWithClient(config, client, logger)

	mgr.refreshTimeouts()
}

// TestRefreshTimeouts_Error tests refresh when list fails.
func TestRefreshTimeouts_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	config := DefaultManagerConfig()
	config.E2BAPIKey = "test-key"
	client := NewE2BClient(config.E2BAPIKey)
	client.SetBaseURL(server.URL)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := NewManagerWithClient(config, client, logger)

	mgr.refreshTimeouts() // Should not panic
}

// TestRefreshTimeouts_TimeoutError tests when timeout set fails.
func TestRefreshTimeouts_TimeoutError(t *testing.T) {
	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/sandboxes" {
			resp := e2bListResponse{
				{
					SandboxID: "sb-ka-err",
					StartedAt: now.Format(time.RFC3339),
					EndAt:     now.Add(5 * time.Minute).Format(time.RFC3339),
					Metadata:  map[string]string{"evoclaw_agent_id": "agent-ka-err"},
				},
			}
			json.NewEncoder(w).Encode(resp)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	config := DefaultManagerConfig()
	config.E2BAPIKey = "test-key"
	client := NewE2BClient(config.E2BAPIKey)
	client.SetBaseURL(server.URL)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := NewManagerWithClient(config, client, logger)

	mgr.refreshTimeouts() // Should log warning, not panic
}

// TestUpdateCosts directly tests cost tracking logic.
func TestUpdateCosts(t *testing.T) {
	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := e2bListResponse{
			{
				SandboxID: "sb-cost-1",
				StartedAt: now.Add(-10 * time.Minute).Format(time.RFC3339),
				EndAt:     now.Add(5 * time.Minute).Format(time.RFC3339),
				Metadata:  map[string]string{"evoclaw_agent_id": "agent-cost-1"},
			},
			{
				SandboxID: "sb-cost-2",
				StartedAt: now.Add(-3 * time.Minute).Format(time.RFC3339),
				EndAt:     now.Add(7 * time.Minute).Format(time.RFC3339),
				Metadata:  map[string]string{"evoclaw_agent_id": "agent-cost-2"},
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	config := DefaultManagerConfig()
	config.E2BAPIKey = "test-key"
	client := NewE2BClient(config.E2BAPIKey)
	client.SetBaseURL(server.URL)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := NewManagerWithClient(config, client, logger)

	initialCost := mgr.GetCosts().EstimatedCostUSD

	mgr.updateCosts()

	newCost := mgr.GetCosts().EstimatedCostUSD
	if newCost <= initialCost {
		t.Error("expected cost to increase after update")
	}

	// Check uptime increased
	costs := mgr.GetCosts()
	if costs.TotalUptimeSec <= 0 {
		t.Error("expected positive total uptime")
	}
}

// TestUpdateCosts_Error tests cost update when list fails.
func TestUpdateCosts_Error(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	config := DefaultManagerConfig()
	config.E2BAPIKey = "test-key"
	client := NewE2BClient(config.E2BAPIKey)
	client.SetBaseURL(server.URL)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := NewManagerWithClient(config, client, logger)

	mgr.updateCosts() // Should not panic
}

// TestUpdateCosts_BudgetWarning tests the 90% budget warning.
func TestUpdateCosts_BudgetWarning(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		resp := e2bListResponse{
			{
				SandboxID: "sb-budget",
				StartedAt: now.Format(time.RFC3339),
				EndAt:     now.Add(5 * time.Minute).Format(time.RFC3339),
			},
		}
		json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	config := DefaultManagerConfig()
	config.E2BAPIKey = "test-key"
	config.CreditBudgetUSD = 1.0 // Very low budget
	client := NewE2BClient(config.E2BAPIKey)
	client.SetBaseURL(server.URL)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := NewManagerWithClient(config, client, logger)

	// Set cost close to budget
	mgr.costs.mu.Lock()
	mgr.costs.estimatedCostUSD = 0.89
	mgr.costs.mu.Unlock()

	mgr.updateCosts() // Should trigger budget warning
}

// TestStopWithRunningAgents tests graceful shutdown with agents.
func TestStopWithRunningAgents(t *testing.T) {
	now := time.Now()
	var deleteCount int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			resp := e2bListResponse{
				{
					SandboxID: "sb-stop-1",
					StartedAt: now.Format(time.RFC3339),
					EndAt:     now.Add(5 * time.Minute).Format(time.RFC3339),
					Metadata:  map[string]string{"evoclaw_agent_id": "stop-agent-1"},
				},
				{
					SandboxID: "sb-stop-2",
					StartedAt: now.Format(time.RFC3339),
					EndAt:     now.Add(5 * time.Minute).Format(time.RFC3339),
					Metadata:  map[string]string{"evoclaw_agent_id": "stop-agent-2"},
				},
			}
			json.NewEncoder(w).Encode(resp)
		case http.MethodDelete:
			deleteCount++
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	config := DefaultManagerConfig()
	config.E2BAPIKey = "test-key"
	config.HealthCheckIntervalSec = 3600
	config.KeepAliveIntervalSec = 3600
	client := NewE2BClient(config.E2BAPIKey)
	client.SetBaseURL(server.URL)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := NewManagerWithClient(config, client, logger)

	mgr.Start()
	err := mgr.Stop()
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if deleteCount != 2 {
		t.Errorf("expected 2 agents killed during shutdown, got %d", deleteCount)
	}
}

// TestStopWithListError tests stop when list fails.
func TestStopWithListError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	config := DefaultManagerConfig()
	config.E2BAPIKey = "test-key"
	config.HealthCheckIntervalSec = 3600
	config.KeepAliveIntervalSec = 3600
	client := NewE2BClient(config.E2BAPIKey)
	client.SetBaseURL(server.URL)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := NewManagerWithClient(config, client, logger)

	mgr.Start()
	err := mgr.Stop()
	if err != nil {
		t.Fatalf("Stop should not fail even if list fails: %v", err)
	}
}

// TestKillAll_WithAgents tests killing all when there are active sandboxes.
func TestKillAll_WithAgents(t *testing.T) {
	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			resp := e2bListResponse{
				{
					SandboxID: "sb-ka-1",
					StartedAt: now.Format(time.RFC3339),
					EndAt:     now.Add(5 * time.Minute).Format(time.RFC3339),
					Metadata:  map[string]string{"evoclaw_agent_id": "ka-agent-1"},
				},
				{
					SandboxID: "sb-ka-2",
					StartedAt: now.Format(time.RFC3339),
					EndAt:     now.Add(5 * time.Minute).Format(time.RFC3339),
					Metadata:  map[string]string{"evoclaw_agent_id": "ka-agent-2"},
				},
			}
			json.NewEncoder(w).Encode(resp)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	config := DefaultManagerConfig()
	config.E2BAPIKey = "test-key"
	client := NewE2BClient(config.E2BAPIKey)
	client.SetBaseURL(server.URL)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := NewManagerWithClient(config, client, logger)

	killed, err := mgr.KillAll(context.Background())
	if err != nil {
		t.Fatalf("KillAll failed: %v", err)
	}
	if killed != 2 {
		t.Errorf("expected 2 killed, got %d", killed)
	}
}

// TestKillAll_WithError tests KillAll when some kills fail.
func TestKillAll_WithError(t *testing.T) {
	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			resp := e2bListResponse{
				{
					SandboxID: "sb-ke-1",
					StartedAt: now.Format(time.RFC3339),
					EndAt:     now.Add(5 * time.Minute).Format(time.RFC3339),
					Metadata:  map[string]string{"evoclaw_agent_id": "ke-agent-1"},
				},
			}
			json.NewEncoder(w).Encode(resp)
		case http.MethodDelete:
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(e2bErrorResponse{Code: 500, Message: "internal error"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	config := DefaultManagerConfig()
	config.E2BAPIKey = "test-key"
	client := NewE2BClient(config.E2BAPIKey)
	client.SetBaseURL(server.URL)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := NewManagerWithClient(config, client, logger)

	killed, err := mgr.KillAll(context.Background())
	if err != nil {
		t.Fatalf("KillAll failed: %v", err)
	}
	// Kill should fail but not cause an error
	if killed != 0 {
		t.Errorf("expected 0 killed (all failures), got %d", killed)
	}
}

// TestKillAgent_UpdatesCosts tests that killing an agent updates cost tracking.
func TestKillAgent_UpdatesCosts(t *testing.T) {
	now := time.Now()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			resp := e2bSandboxResponse{
				SandboxID: "sb-cost-kill",
				StartedAt: now.Add(-5 * time.Minute).Format(time.RFC3339),
				EndAt:     now.Add(5 * time.Minute).Format(time.RFC3339),
			}
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(resp)
		case http.MethodDelete:
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	config := DefaultManagerConfig()
	config.E2BAPIKey = "test-key"
	client := NewE2BClient(config.E2BAPIKey)
	client.SetBaseURL(server.URL)

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	mgr := NewManagerWithClient(config, client, logger)

	// Spawn agent
	sandbox, err := mgr.SpawnAgent(context.Background(), AgentConfig{
		TemplateID: "test",
		AgentID:    "cost-kill-agent",
	})
	if err != nil {
		t.Fatalf("SpawnAgent failed: %v", err)
	}

	initialCost := mgr.GetCosts().EstimatedCostUSD

	// Kill — should update costs
	err = mgr.KillAgent(context.Background(), sandbox.SandboxID)
	if err != nil {
		t.Fatalf("KillAgent failed: %v", err)
	}

	newCost := mgr.GetCosts().EstimatedCostUSD
	if newCost <= initialCost {
		t.Error("expected cost to increase after killing a running agent")
	}
}

// Unused import prevention
var _ = fmt.Sprintf
