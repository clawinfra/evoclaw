package cloudsync

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRecoveryManager_RestoreAgent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Mock agent query response
		resp := PipelineResponse{
			Results: []BatchResult{
				{
					Type: "ok",
					Response: &QueryResponse{
						Columns: []string{"agent_id", "name", "model", "capabilities", "genome", "persona"},
						Rows: [][]interface{}{
							{
								"agent-1",
								"TestAgent",
								"gpt-4",
								`["chat","code"]`,
								`{"temperature":0.7}`,
								`{"personality":"friendly"}`,
							},
						},
					},
				},
				// Mock core memory query
				{
					Type: "ok",
					Response: &QueryResponse{
						Columns: []string{"content"},
						Rows: [][]interface{}{
							{`{"owner":"Alice"}`},
						},
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", slog.Default())
	manager := NewRecoveryManager(client, slog.Default())

	ctx := context.Background()
	memory, err := manager.RestoreAgent(ctx, "agent-1")
	if err != nil {
		t.Fatalf("RestoreAgent failed: %v", err)
	}

	if memory.AgentID != "agent-1" {
		t.Errorf("expected agent-1, got %s", memory.AgentID)
	}

	if memory.Name != "TestAgent" {
		t.Errorf("expected TestAgent, got %s", memory.Name)
	}

	if len(memory.Capabilities) != 2 {
		t.Errorf("expected 2 capabilities, got %d", len(memory.Capabilities))
	}

	if memory.Genome["temperature"] != 0.7 {
		t.Errorf("unexpected genome temperature: %v", memory.Genome["temperature"])
	}
}

func TestRecoveryManager_RestoreToDevice(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		
		// RestoreAgent makes 2 queries (agent + core memory), then RestoreToDevice makes 1 insert
		if requestCount <= 2 {
			// First two requests: restore agent (agent query + core memory query)
			resp := PipelineResponse{
				Results: []BatchResult{
					{
						Type: "ok",
						Response: &QueryResponse{
							Columns: []string{"agent_id", "name", "model", "capabilities", "genome", "persona"},
							Rows: [][]interface{}{
								{"agent-1", "TestAgent", "gpt-4", `[]`, `{}`, `{}`},
							},
						},
					},
					{Type: "ok", Response: &QueryResponse{Rows: [][]interface{}{}}},
				},
			}
			_ = json.NewEncoder(w).Encode(resp)
		} else {
			// Third request: register device
			resp := PipelineResponse{
				Results: []BatchResult{{Type: "ok"}},
			}
			_ = json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", slog.Default())
	manager := NewRecoveryManager(client, slog.Default())

	ctx := context.Background()
	memory, err := manager.RestoreToDevice(ctx, "agent-1", "new-device", "new-key")
	if err != nil {
		t.Fatalf("RestoreToDevice failed: %v", err)
	}

	if memory.AgentID != "agent-1" {
		t.Errorf("expected agent-1, got %s", memory.AgentID)
	}

	// Should be 2 requests (both queries in RestoreAgent) + 1 (device registration)
	if requestCount < 2 {
		t.Errorf("expected at least 2 requests, got %d", requestCount)
	}
}

func TestRecoveryManager_GetWarmMemory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := PipelineResponse{
			Results: []BatchResult{
				{
					Type: "ok",
					Response: &QueryResponse{
						Columns: []string{"id", "event_type", "content", "timestamp", "distilled"},
						Rows: [][]interface{}{
							{
								"entry-1",
								"conversation",
								`{"message":"Hello"}`,
								float64(1234567890),
								float64(0),
							},
							{
								"entry-2",
								"action",
								`{"action":"search"}`,
								float64(1234567900),
								float64(1),
							},
						},
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", slog.Default())
	manager := NewRecoveryManager(client, slog.Default())

	ctx := context.Background()
	entries, err := manager.GetWarmMemory(ctx, "agent-1", 10)
	if err != nil {
		t.Fatalf("GetWarmMemory failed: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(entries))
	}

	if entries[0].EventType != "conversation" {
		t.Errorf("expected conversation, got %s", entries[0].EventType)
	}

	if !entries[1].Distilled {
		t.Error("expected entry 2 to be distilled")
	}
}

func TestRecoveryManager_GetEvolutionHistory(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := PipelineResponse{
			Results: []BatchResult{
				{
					Type: "ok",
					Response: &QueryResponse{
						Columns: []string{"id", "event_type", "fitness_score", "genome_before", "genome_after", "metrics", "timestamp"},
						Rows: [][]interface{}{
							{
								"evo-1",
								"mutation",
								float64(0.85),
								`{"temp":0.7}`,
								`{"temp":0.8}`,
								`{"accuracy":0.9}`,
								float64(1234567890),
							},
						},
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", slog.Default())
	manager := NewRecoveryManager(client, slog.Default())

	ctx := context.Background()
	entries, err := manager.GetEvolutionHistory(ctx, "agent-1", 10)
	if err != nil {
		t.Fatalf("GetEvolutionHistory failed: %v", err)
	}

	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}

	if entries[0].EventType != "mutation" {
		t.Errorf("expected mutation, got %s", entries[0].EventType)
	}

	if entries[0].FitnessScore != 0.85 {
		t.Errorf("expected fitness 0.85, got %f", entries[0].FitnessScore)
	}
}

func TestRecoveryManager_MarkDeviceStolen(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := PipelineResponse{
			Results: []BatchResult{{Type: "ok"}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", slog.Default())
	manager := NewRecoveryManager(client, slog.Default())

	ctx := context.Background()
	err := manager.MarkDeviceStolen(ctx, "device-1")
	if err != nil {
		t.Fatalf("MarkDeviceStolen failed: %v", err)
	}
}

func TestRecoveryManager_ListDevices(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := PipelineResponse{
			Results: []BatchResult{
				{
					Type: "ok",
					Response: &QueryResponse{
						Columns: []string{"device_id", "device_name", "device_type", "last_heartbeat", "last_sync", "status", "created_at"},
						Rows: [][]interface{}{
							{
								"device-1",
								"Primary Device",
								"phone",
								float64(1234567890),
								float64(1234567880),
								"active",
								float64(1234560000),
							},
							{
								"device-2",
								"Secondary Device",
								"tablet",
								float64(1234567850),
								nil,
								"offline",
								float64(1234560100),
							},
						},
					},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", slog.Default())
	manager := NewRecoveryManager(client, slog.Default())

	ctx := context.Background()
	devices, err := manager.ListDevices(ctx, "agent-1")
	if err != nil {
		t.Fatalf("ListDevices failed: %v", err)
	}

	if len(devices) != 2 {
		t.Fatalf("expected 2 devices, got %d", len(devices))
	}

	if devices[0].DeviceID != "device-1" {
		t.Errorf("expected device-1, got %s", devices[0].DeviceID)
	}

	if devices[0].Status != "active" {
		t.Errorf("expected active, got %s", devices[0].Status)
	}

	if devices[1].LastSync != 0 {
		t.Errorf("expected 0 for nil last_sync, got %d", devices[1].LastSync)
	}
}
