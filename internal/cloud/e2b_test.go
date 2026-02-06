package cloud

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// newTestE2BServer creates a mock E2B API server for testing.
func newTestE2BServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()

	// POST /sandboxes — create sandbox
	mux.HandleFunc("POST /sandboxes", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			json.NewEncoder(w).Encode(e2bErrorResponse{Code: 401, Message: "missing api key"})
			return
		}

		var req e2bCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(e2bErrorResponse{Code: 400, Message: "bad request"})
			return
		}

		now := time.Now()
		resp := e2bSandboxResponse{
			SandboxID:  "sb-test-123",
			TemplateID: req.TemplateID,
			ClientID:   "cl-test-456",
			StartedAt:  now.Format(time.RFC3339),
			EndAt:      now.Add(time.Duration(req.Timeout) * time.Second).Format(time.RFC3339),
			Metadata:   req.Metadata,
		}

		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(resp)
	})

	// GET /sandboxes — list sandboxes
	mux.HandleFunc("GET /sandboxes", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-API-Key") == "" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		now := time.Now()
		resp := e2bListResponse{
			{
				SandboxID:  "sb-test-123",
				TemplateID: "evoclaw-agent",
				ClientID:   "cl-test-456",
				StartedAt:  now.Add(-5 * time.Minute).Format(time.RFC3339),
				EndAt:      now.Add(5 * time.Minute).Format(time.RFC3339),
				Metadata: map[string]string{
					"evoclaw_agent_id":   "agent-1",
					"evoclaw_agent_type": "trader",
					"evoclaw_user_id":    "user-42",
				},
			},
			{
				SandboxID:  "sb-test-789",
				TemplateID: "evoclaw-agent",
				ClientID:   "cl-test-012",
				StartedAt:  now.Add(-2 * time.Minute).Format(time.RFC3339),
				EndAt:      now.Add(8 * time.Minute).Format(time.RFC3339),
				Metadata: map[string]string{
					"evoclaw_agent_id":   "agent-2",
					"evoclaw_agent_type": "monitor",
				},
			},
		}

		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(resp)
	})

	// GET /sandboxes/{id} — get sandbox info
	mux.HandleFunc("GET /sandboxes/sb-test-123", func(w http.ResponseWriter, r *http.Request) {
		now := time.Now()
		resp := e2bSandboxResponse{
			SandboxID:  "sb-test-123",
			TemplateID: "evoclaw-agent",
			ClientID:   "cl-test-456",
			StartedAt:  now.Add(-5 * time.Minute).Format(time.RFC3339),
			EndAt:      now.Add(5 * time.Minute).Format(time.RFC3339),
			Metadata: map[string]string{
				"evoclaw_agent_id": "agent-1",
			},
		}
		json.NewEncoder(w).Encode(resp)
	})

	// GET /sandboxes/{id} — not found
	mux.HandleFunc("GET /sandboxes/sb-nonexistent", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(e2bErrorResponse{Code: 404, Message: "sandbox not found"})
	})

	// DELETE /sandboxes/{id} — kill sandbox
	mux.HandleFunc("DELETE /sandboxes/sb-test-123", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	// DELETE /sandboxes/{id} — not found
	mux.HandleFunc("DELETE /sandboxes/sb-nonexistent", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(e2bErrorResponse{Code: 404, Message: "sandbox not found"})
	})

	// POST /sandboxes/{id}/process — execute command
	mux.HandleFunc("POST /sandboxes/sb-test-123/process", func(w http.ResponseWriter, r *http.Request) {
		var req e2bProcessRequest
		json.NewDecoder(r.Body).Decode(&req)

		resp := e2bProcessResponse{
			ExitCode: 0,
			Stdout:   "hello from sandbox",
			Stderr:   "",
		}
		json.NewEncoder(w).Encode(resp)
	})

	// POST /sandboxes/{id}/timeout — set timeout
	mux.HandleFunc("POST /sandboxes/sb-test-123/timeout", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})

	return httptest.NewServer(mux)
}

func newTestClient(t *testing.T, serverURL string) *E2BClient {
	t.Helper()
	client := NewE2BClient("test-api-key")
	client.SetBaseURL(serverURL)
	return client
}

func TestNewE2BClient(t *testing.T) {
	client := NewE2BClient("test-key")
	if client == nil {
		t.Fatal("expected non-nil client")
	}
	if client.apiKey != "test-key" {
		t.Errorf("expected api key 'test-key', got '%s'", client.apiKey)
	}
	if client.baseURL != DefaultE2BBaseURL {
		t.Errorf("expected default base URL, got '%s'", client.baseURL)
	}
}

func TestNewE2BClientWithHTTP(t *testing.T) {
	httpClient := &http.Client{Timeout: 5 * time.Second}
	client := NewE2BClientWithHTTP("test-key", httpClient)
	if client.httpClient != httpClient {
		t.Error("expected custom http client")
	}
}

func TestSetBaseURL(t *testing.T) {
	client := NewE2BClient("test-key")
	client.SetBaseURL("http://localhost:9999")
	if client.baseURL != "http://localhost:9999" {
		t.Errorf("expected custom base URL, got '%s'", client.baseURL)
	}
}

func TestSpawnAgent(t *testing.T) {
	server := newTestE2BServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	ctx := context.Background()

	config := AgentConfig{
		TemplateID:      "evoclaw-agent",
		AgentID:         "agent-1",
		AgentType:       "trader",
		MQTTBroker:      "broker.test.io",
		MQTTPort:        1883,
		OrchestratorURL: "http://localhost:8420",
		TimeoutSec:      600,
		EnvVars:         map[string]string{"CUSTOM": "value"},
		Metadata:        map[string]string{"env": "test"},
		Genome:          `{"type":"momentum"}`,
		UserID:          "user-42",
	}

	sandbox, err := client.SpawnAgent(ctx, config)
	if err != nil {
		t.Fatalf("SpawnAgent failed: %v", err)
	}

	if sandbox.SandboxID != "sb-test-123" {
		t.Errorf("expected sandbox ID 'sb-test-123', got '%s'", sandbox.SandboxID)
	}
	if sandbox.AgentID != "agent-1" {
		t.Errorf("expected agent ID 'agent-1', got '%s'", sandbox.AgentID)
	}
	if sandbox.TemplateID != "evoclaw-agent" {
		t.Errorf("expected template 'evoclaw-agent', got '%s'", sandbox.TemplateID)
	}
	if sandbox.State != SandboxStateRunning {
		t.Errorf("expected state 'running', got '%s'", sandbox.State)
	}
	if sandbox.UserID != "user-42" {
		t.Errorf("expected user ID 'user-42', got '%s'", sandbox.UserID)
	}

	// Check local cache
	if client.LocalSandboxCount() != 1 {
		t.Errorf("expected 1 local sandbox, got %d", client.LocalSandboxCount())
	}
	cached, ok := client.GetLocalSandbox("sb-test-123")
	if !ok {
		t.Fatal("expected sandbox in local cache")
	}
	if cached.AgentID != "agent-1" {
		t.Errorf("cached agent ID mismatch")
	}
}

func TestSpawnAgent_MissingTemplate(t *testing.T) {
	client := NewE2BClient("test-key")
	ctx := context.Background()

	_, err := client.SpawnAgent(ctx, AgentConfig{AgentID: "agent-1"})
	if err == nil {
		t.Fatal("expected error for missing template")
	}
}

func TestSpawnAgent_MissingAgentID(t *testing.T) {
	client := NewE2BClient("test-key")
	ctx := context.Background()

	_, err := client.SpawnAgent(ctx, AgentConfig{TemplateID: "test"})
	if err == nil {
		t.Fatal("expected error for missing agent ID")
	}
}

func TestSpawnAgent_DefaultTimeout(t *testing.T) {
	server := newTestE2BServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	ctx := context.Background()

	sandbox, err := client.SpawnAgent(ctx, AgentConfig{
		TemplateID: "evoclaw-agent",
		AgentID:    "agent-default-timeout",
	})
	if err != nil {
		t.Fatalf("SpawnAgent failed: %v", err)
	}
	if sandbox.SandboxID == "" {
		t.Error("expected non-empty sandbox ID")
	}
}

func TestKillAgent(t *testing.T) {
	server := newTestE2BServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	ctx := context.Background()

	// First spawn
	_, err := client.SpawnAgent(ctx, AgentConfig{
		TemplateID: "evoclaw-agent",
		AgentID:    "agent-1",
	})
	if err != nil {
		t.Fatalf("SpawnAgent failed: %v", err)
	}

	// Then kill
	err = client.KillAgent(ctx, "sb-test-123")
	if err != nil {
		t.Fatalf("KillAgent failed: %v", err)
	}

	// Check removed from cache
	if client.LocalSandboxCount() != 0 {
		t.Errorf("expected 0 local sandboxes after kill, got %d", client.LocalSandboxCount())
	}
}

func TestKillAgent_Empty(t *testing.T) {
	client := NewE2BClient("test-key")
	err := client.KillAgent(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty sandbox ID")
	}
}

func TestKillAgent_NotFound(t *testing.T) {
	server := newTestE2BServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	err := client.KillAgent(context.Background(), "sb-nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent sandbox")
	}
}

func TestListAgents(t *testing.T) {
	server := newTestE2BServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	ctx := context.Background()

	sandboxes, err := client.ListAgents(ctx)
	if err != nil {
		t.Fatalf("ListAgents failed: %v", err)
	}

	if len(sandboxes) != 2 {
		t.Fatalf("expected 2 sandboxes, got %d", len(sandboxes))
	}

	if sandboxes[0].AgentID != "agent-1" {
		t.Errorf("expected first agent ID 'agent-1', got '%s'", sandboxes[0].AgentID)
	}
	if sandboxes[1].AgentID != "agent-2" {
		t.Errorf("expected second agent ID 'agent-2', got '%s'", sandboxes[1].AgentID)
	}
	if sandboxes[0].UserID != "user-42" {
		t.Errorf("expected first user ID 'user-42', got '%s'", sandboxes[0].UserID)
	}

	// Local cache should be updated
	if client.LocalSandboxCount() != 2 {
		t.Errorf("expected 2 cached sandboxes, got %d", client.LocalSandboxCount())
	}
}

func TestGetAgentStatus(t *testing.T) {
	server := newTestE2BServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	ctx := context.Background()

	status, err := client.GetAgentStatus(ctx, "sb-test-123")
	if err != nil {
		t.Fatalf("GetAgentStatus failed: %v", err)
	}

	if status.SandboxID != "sb-test-123" {
		t.Errorf("expected sandbox ID 'sb-test-123', got '%s'", status.SandboxID)
	}
	if status.AgentID != "agent-1" {
		t.Errorf("expected agent ID 'agent-1', got '%s'", status.AgentID)
	}
	if !status.Healthy {
		t.Error("expected healthy status")
	}
	if status.UptimeSec <= 0 {
		t.Error("expected positive uptime")
	}
}

func TestGetAgentStatus_Empty(t *testing.T) {
	client := NewE2BClient("test-key")
	_, err := client.GetAgentStatus(context.Background(), "")
	if err == nil {
		t.Fatal("expected error for empty sandbox ID")
	}
}

func TestGetAgentStatus_NotFound(t *testing.T) {
	server := newTestE2BServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.GetAgentStatus(context.Background(), "sb-nonexistent")
	if err == nil {
		t.Fatal("expected error for non-existent sandbox")
	}
}

func TestSendCommand(t *testing.T) {
	server := newTestE2BServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	ctx := context.Background()

	resp, err := client.SendCommand(ctx, "sb-test-123", Command{
		Cmd:  "echo",
		Args: []string{"hello"},
	})
	if err != nil {
		t.Fatalf("SendCommand failed: %v", err)
	}

	if resp.ExitCode != 0 {
		t.Errorf("expected exit code 0, got %d", resp.ExitCode)
	}
	if resp.Stdout != "hello from sandbox" {
		t.Errorf("expected stdout 'hello from sandbox', got '%s'", resp.Stdout)
	}
}

func TestSendCommand_EmptySandboxID(t *testing.T) {
	client := NewE2BClient("test-key")
	_, err := client.SendCommand(context.Background(), "", Command{Cmd: "echo"})
	if err == nil {
		t.Fatal("expected error for empty sandbox ID")
	}
}

func TestSendCommand_EmptyCmd(t *testing.T) {
	client := NewE2BClient("test-key")
	_, err := client.SendCommand(context.Background(), "sb-123", Command{})
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestSetTimeout(t *testing.T) {
	server := newTestE2BServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)
	ctx := context.Background()

	err := client.SetTimeout(ctx, "sb-test-123", 600)
	if err != nil {
		t.Fatalf("SetTimeout failed: %v", err)
	}
}

func TestSetTimeout_EmptyID(t *testing.T) {
	client := NewE2BClient("test-key")
	err := client.SetTimeout(context.Background(), "", 600)
	if err == nil {
		t.Fatal("expected error for empty sandbox ID")
	}
}

func TestLocalSandboxOperations(t *testing.T) {
	client := NewE2BClient("test-key")

	// Initially empty
	if client.LocalSandboxCount() != 0 {
		t.Errorf("expected 0 sandboxes, got %d", client.LocalSandboxCount())
	}

	_, ok := client.GetLocalSandbox("nonexistent")
	if ok {
		t.Error("expected no sandbox found")
	}
}

func TestParseError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(e2bErrorResponse{Code: 500, Message: "internal error"})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	_, err := client.ListAgents(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "e2b api error (HTTP 500): internal error" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParseError_NonJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		w.Write([]byte("bad gateway"))
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)

	_, err := client.ListAgents(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "e2b api error (HTTP 502): bad gateway" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSpawnAgent_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(e2bErrorResponse{Code: 503, Message: "service unavailable"})
	}))
	defer server.Close()

	client := newTestClient(t, server.URL)
	_, err := client.SpawnAgent(context.Background(), AgentConfig{
		TemplateID: "test",
		AgentID:    "agent-1",
	})
	if err == nil {
		t.Fatal("expected error for server error")
	}
}

func TestSpawnAgent_InvalidURL(t *testing.T) {
	client := NewE2BClient("test-key")
	client.SetBaseURL("http://invalid-host-that-does-not-exist:99999")

	_, err := client.SpawnAgent(context.Background(), AgentConfig{
		TemplateID: "test",
		AgentID:    "agent-1",
	})
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestKillAgent_InvalidURL(t *testing.T) {
	client := NewE2BClient("test-key")
	client.SetBaseURL("http://invalid-host-that-does-not-exist:99999")

	err := client.KillAgent(context.Background(), "sb-123")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestListAgents_InvalidURL(t *testing.T) {
	client := NewE2BClient("test-key")
	client.SetBaseURL("http://invalid-host-that-does-not-exist:99999")

	_, err := client.ListAgents(context.Background())
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestGetAgentStatus_InvalidURL(t *testing.T) {
	client := NewE2BClient("test-key")
	client.SetBaseURL("http://invalid-host-that-does-not-exist:99999")

	_, err := client.GetAgentStatus(context.Background(), "sb-123")
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestSendCommand_InvalidURL(t *testing.T) {
	client := NewE2BClient("test-key")
	client.SetBaseURL("http://invalid-host-that-does-not-exist:99999")

	_, err := client.SendCommand(context.Background(), "sb-123", Command{Cmd: "echo"})
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestSetTimeout_InvalidURL(t *testing.T) {
	client := NewE2BClient("test-key")
	client.SetBaseURL("http://invalid-host-that-does-not-exist:99999")

	err := client.SetTimeout(context.Background(), "sb-123", 600)
	if err == nil {
		t.Fatal("expected error for invalid URL")
	}
}

func TestToSandbox(t *testing.T) {
	client := NewE2BClient("test-key")
	now := time.Now()

	resp := e2bSandboxResponse{
		SandboxID:  "sb-1",
		TemplateID: "tmpl-1",
		ClientID:   "cl-1",
		StartedAt:  now.Format(time.RFC3339),
		EndAt:      now.Add(5 * time.Minute).Format(time.RFC3339),
		Metadata:   map[string]string{"key": "value"},
	}

	sandbox := client.toSandbox(resp, "agent-1", "user-1")

	if sandbox.SandboxID != "sb-1" {
		t.Errorf("expected sandbox ID 'sb-1', got '%s'", sandbox.SandboxID)
	}
	if sandbox.AgentID != "agent-1" {
		t.Errorf("expected agent ID 'agent-1', got '%s'", sandbox.AgentID)
	}
	if sandbox.UserID != "user-1" {
		t.Errorf("expected user ID 'user-1', got '%s'", sandbox.UserID)
	}
	if sandbox.State != SandboxStateRunning {
		t.Errorf("expected state 'running', got '%s'", sandbox.State)
	}
}

func TestToSandbox_BadDates(t *testing.T) {
	client := NewE2BClient("test-key")

	resp := e2bSandboxResponse{
		SandboxID: "sb-1",
		StartedAt: "not-a-date",
		EndAt:     "also-not-a-date",
	}

	sandbox := client.toSandbox(resp, "agent-1", "")
	// Should not panic, just have zero times
	if !sandbox.StartedAt.IsZero() {
		t.Error("expected zero start time for bad date")
	}
	if !sandbox.EndsAt.IsZero() {
		t.Error("expected zero end time for bad date")
	}
}

func TestAgentConfig_EnvVarsAndMetadata(t *testing.T) {
	server := newTestE2BServer(t)
	defer server.Close()

	client := newTestClient(t, server.URL)

	config := AgentConfig{
		TemplateID:      "evoclaw-agent",
		AgentID:         "env-test",
		AgentType:       "monitor",
		MQTTBroker:      "mqtt.test.io",
		MQTTPort:        8883,
		OrchestratorURL: "http://orch.test.io:8420",
		Genome:          `{"type":"mean_revert"}`,
		UserID:          "user-99",
		EnvVars: map[string]string{
			"CUSTOM_KEY": "custom_value",
		},
		Metadata: map[string]string{
			"deployment": "staging",
		},
	}

	sandbox, err := client.SpawnAgent(context.Background(), config)
	if err != nil {
		t.Fatalf("SpawnAgent failed: %v", err)
	}
	if sandbox.SandboxID == "" {
		t.Error("expected non-empty sandbox ID")
	}
}

func TestConstants(t *testing.T) {
	if DefaultE2BBaseURL != "https://api.e2b.dev" {
		t.Errorf("unexpected default base URL: %s", DefaultE2BBaseURL)
	}
	if DefaultSandboxTimeoutSec != 300 {
		t.Errorf("unexpected default timeout: %d", DefaultSandboxTimeoutSec)
	}
	if SandboxStateRunning != "running" {
		t.Errorf("unexpected running state: %s", SandboxStateRunning)
	}
	if SandboxStatePaused != "paused" {
		t.Errorf("unexpected paused state: %s", SandboxStatePaused)
	}
}
