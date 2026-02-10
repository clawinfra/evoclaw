package cloudsync

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestSyncEngine_CriticalSync(t *testing.T) {
	var receivedRequests int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedRequests++
		resp := PipelineResponse{
			Results: []BatchResult{
				{Type: "ok"},
				{Type: "ok"},
				{Type: "ok"},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", slog.Default())
	config := SyncConfig{
		Enabled:                  true,
		DeviceID:                 "test-device",
		DeviceKey:                "test-key",
		CriticalSyncEnabled:      true,
		MaxOfflineQueueSize:      100,
	}

	engine := NewSyncEngine(client, config, slog.Default())

	memory := &AgentMemory{
		AgentID:      "agent-1",
		Name:         "Test Agent",
		Model:        "gpt-4",
		Capabilities: []string{"chat", "code"},
		Genome: map[string]interface{}{
			"temperature": 0.7,
		},
		Persona: map[string]interface{}{
			"personality": "friendly",
		},
		CoreMemory: map[string]interface{}{
			"owner": "Alice",
		},
	}

	ctx := context.Background()
	err := engine.CriticalSync(ctx, memory)
	if err != nil {
		t.Fatalf("CriticalSync failed: %v", err)
	}

	if receivedRequests != 1 {
		t.Errorf("expected 1 request, got %d", receivedRequests)
	}
}

func TestSyncEngine_WarmSync(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req PipelineRequest
		json.NewDecoder(r.Body).Decode(&req)

		// Should have 3 statements: 2 warm memory + 1 sync state update
		if len(req.Requests) != 3 {
			t.Errorf("expected 3 requests, got %d", len(req.Requests))
		}

		results := make([]BatchResult, len(req.Requests))
		for i := range results {
			results[i] = BatchResult{Type: "ok"}
		}
		resp := PipelineResponse{Results: results}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", slog.Default())
	config := SyncConfig{
		Enabled:             true,
		DeviceID:            "test-device",
		MaxOfflineQueueSize: 100,
	}

	engine := NewSyncEngine(client, config, slog.Default())

	snapshot := &MemorySnapshot{
		AgentID:   "agent-1",
		Timestamp: time.Now().Unix(),
		WarmMemory: []WarmMemoryEntry{
			{
				ID:        uuid.New().String(),
				EventType: "conversation",
				Content: map[string]interface{}{
					"message": "Hello world",
				},
				Timestamp: time.Now().Unix(),
				Distilled: false,
			},
			{
				ID:        uuid.New().String(),
				EventType: "action",
				Content: map[string]interface{}{
					"action": "search",
				},
				Timestamp: time.Now().Unix(),
				Distilled: true,
			},
		},
	}

	ctx := context.Background()
	err := engine.WarmSync(ctx, snapshot)
	if err != nil {
		t.Fatalf("WarmSync failed: %v", err)
	}
}

func TestSyncEngine_OfflineQueue(t *testing.T) {
	// Server that always fails
	failCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		failCount++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", slog.Default())
	config := SyncConfig{
		Enabled:                  true,
		DeviceID:                 "test-device",
		DeviceKey:                "test-key",
		CriticalSyncEnabled:      true,
		MaxOfflineQueueSize:      100,
	}

	engine := NewSyncEngine(client, config, slog.Default())

	memory := &AgentMemory{
		AgentID: "agent-1",
		Name:    "Test Agent",
		Model:   "gpt-4",
	}

	ctx := context.Background()
	err := engine.CriticalSync(ctx, memory)
	
	// Should fail but queue the operation
	if err == nil {
		t.Fatal("expected error when server fails")
	}

	// Check queue size
	if engine.offlineQueue.Size() != 1 {
		t.Errorf("expected queue size 1, got %d", engine.offlineQueue.Size())
	}
}

func TestSyncEngine_FullSync(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req PipelineRequest
		json.NewDecoder(r.Body).Decode(&req)

		results := make([]BatchResult, len(req.Requests))
		for i := range results {
			results[i] = BatchResult{Type: "ok"}
		}
		resp := PipelineResponse{Results: results}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", slog.Default())
	config := SyncConfig{
		Enabled:             true,
		DeviceID:            "test-device",
		MaxOfflineQueueSize: 100,
	}

	engine := NewSyncEngine(client, config, slog.Default())

	snapshot := &MemorySnapshot{
		AgentID:   "agent-1",
		Timestamp: time.Now().Unix(),
		Evolution: []EvolutionEntry{
			{
				ID:           uuid.New().String(),
				EventType:    "mutation",
				FitnessScore: 0.85,
				Metrics: map[string]float64{
					"accuracy": 0.92,
				},
				Timestamp: time.Now().Unix(),
			},
		},
		Actions: []ActionEntry{
			{
				ID:         uuid.New().String(),
				ActionType: "trade",
				Data: map[string]interface{}{
					"symbol": "BTC",
					"amount": 0.1,
				},
				Result:    "success",
				Timestamp: time.Now().Unix(),
			},
		},
	}

	ctx := context.Background()
	err := engine.FullSync(ctx, snapshot)
	if err != nil {
		t.Fatalf("FullSync failed: %v", err)
	}
}

func TestSyncEngine_StartStop(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := PipelineResponse{
			Results: []BatchResult{{Type: "ok"}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-token", slog.Default())
	config := SyncConfig{
		Enabled:                  true,
		DeviceID:                 "test-device",
		DeviceKey:                "test-key",
		HeartbeatIntervalSeconds: 1,
		WarmSyncIntervalMinutes:  1,
		FullSyncIntervalHours:    1,
		MaxOfflineQueueSize:      100,
	}

	engine := NewSyncEngine(client, config, slog.Default())

	ctx := context.Background()
	if err := engine.Start(ctx); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Let it run briefly
	time.Sleep(100 * time.Millisecond)

	if err := engine.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Create a new engine to test fresh start (reusing stopped engine resets state)
	engine2 := NewSyncEngine(client, config, slog.Default())
	if err := engine2.Start(ctx); err != nil {
		t.Fatalf("Second engine start failed: %v", err)
	}
	engine2.Stop()
}

func TestOfflineQueue_EnqueueDequeue(t *testing.T) {
	queue := NewOfflineQueue(10)

	op := &SyncOperation{
		Type:      "critical",
		AgentID:   "agent-1",
		Timestamp: time.Now().Unix(),
	}

	if !queue.Enqueue(op) {
		t.Fatal("Enqueue failed")
	}

	if queue.Size() != 1 {
		t.Errorf("expected size 1, got %d", queue.Size())
	}

	dequeued := queue.Dequeue()
	if dequeued == nil {
		t.Fatal("Dequeue returned nil")
	}

	if dequeued.AgentID != "agent-1" {
		t.Errorf("expected agent-1, got %s", dequeued.AgentID)
	}

	if queue.Size() != 0 {
		t.Errorf("expected size 0, got %d", queue.Size())
	}
}

func TestOfflineQueue_MaxSize(t *testing.T) {
	queue := NewOfflineQueue(3)

	// Fill queue
	for i := 0; i < 3; i++ {
		queue.Enqueue(&SyncOperation{
			Type:    "warm",
			AgentID: "agent-1",
		})
	}

	// Add one more critical operation - should evict oldest non-critical
	queue.Enqueue(&SyncOperation{
		Type:    "critical",
		AgentID: "agent-2",
	})

	if queue.Size() != 3 {
		t.Errorf("expected size 3, got %d", queue.Size())
	}
}
