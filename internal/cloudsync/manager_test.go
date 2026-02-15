package cloudsync

import (
	"context"
	"log/slog"
	"testing"

	"github.com/clawinfra/evoclaw/internal/config"
)

func TestNewManagerDisabled(t *testing.T) {
	cfg := config.CloudSyncConfig{Enabled: false}
	m, err := NewManager(cfg, slog.Default())
	if err != nil {
		t.Fatalf("NewManager() error: %v", err)
	}
	if m.IsEnabled() {
		t.Error("expected disabled")
	}
	if m.DeviceID() != "" {
		t.Error("expected empty device ID")
	}
}

func TestNewManagerMissingURL(t *testing.T) {
	cfg := config.CloudSyncConfig{Enabled: true}
	_, err := NewManager(cfg, slog.Default())
	if err == nil {
		t.Error("expected error for missing URL")
	}
}

func TestNewManagerMissingToken(t *testing.T) {
	cfg := config.CloudSyncConfig{Enabled: true, DatabaseURL: "http://localhost"}
	_, err := NewManager(cfg, slog.Default())
	if err == nil {
		t.Error("expected error for missing token")
	}
}

func TestNewManagerValid(t *testing.T) {
	cfg := config.CloudSyncConfig{
		Enabled:     true,
		DatabaseURL: "http://localhost:8080",
		AuthToken:   "test-token",
	}
	m, err := NewManager(cfg, slog.Default())
	if err != nil {
		t.Fatalf("NewManager() error: %v", err)
	}
	if !m.IsEnabled() {
		t.Error("expected enabled")
	}
	if m.DeviceID() == "" {
		t.Error("expected auto-generated device ID")
	}
	if m.DeviceKey() == "" {
		t.Error("expected auto-generated device key")
	}
}

func TestManagerDisabledOperations(t *testing.T) {
	cfg := config.CloudSyncConfig{Enabled: false}
	m, _ := NewManager(cfg, slog.Default())
	ctx := context.Background()

	// All operations should return nil/error gracefully
	if err := m.InitSchema(ctx); err != nil {
		t.Errorf("InitSchema() error: %v", err)
	}
	if err := m.Start(ctx); err != nil {
		t.Errorf("Start() error: %v", err)
	}
	if err := m.Stop(); err != nil {
		t.Errorf("Stop() error: %v", err)
	}
	if err := m.SyncCritical(ctx, nil); err != nil {
		t.Errorf("SyncCritical() error: %v", err)
	}
	if err := m.SyncWarm(ctx, nil); err != nil {
		t.Errorf("SyncWarm() error: %v", err)
	}
	if err := m.SyncFull(ctx, nil); err != nil {
		t.Errorf("SyncFull() error: %v", err)
	}

	_, err := m.RestoreAgent(ctx, "agent1")
	if err == nil {
		t.Error("expected error for disabled RestoreAgent")
	}

	_, err = m.RestoreToDevice(ctx, "agent1", "dev1", "key1")
	if err == nil {
		t.Error("expected error for disabled RestoreToDevice")
	}

	_, err = m.GetWarmMemory(ctx, "agent1", 10)
	if err == nil {
		t.Error("expected error for disabled GetWarmMemory")
	}

	_, err = m.GetEvolutionHistory(ctx, "agent1", 10)
	if err == nil {
		t.Error("expected error for disabled GetEvolutionHistory")
	}

	_, err = m.GetActionHistory(ctx, "agent1", 10)
	if err == nil {
		t.Error("expected error for disabled GetActionHistory")
	}

	err = m.MarkDeviceStolen(ctx, "dev1")
	if err == nil {
		t.Error("expected error for disabled MarkDeviceStolen")
	}

	_, err = m.ListDevices(ctx, "agent1")
	if err == nil {
		t.Error("expected error for disabled ListDevices")
	}

	count, err := m.CleanupExpiredMemory(ctx)
	if err != nil {
		t.Errorf("CleanupExpiredMemory() error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0, got %d", count)
	}
}

func TestOfflineQueueComprehensive(t *testing.T) {
	q := NewOfflineQueue(3)

	// Empty queue
	if q.Size() != 0 {
		t.Error("expected empty queue")
	}
	if op := q.Dequeue(); op != nil {
		t.Error("expected nil dequeue from empty queue")
	}

	// Enqueue operations
	q.Enqueue(&SyncOperation{Type: "warm", AgentID: "a1", Timestamp: 1})
	q.Enqueue(&SyncOperation{Type: "critical", AgentID: "a1", Timestamp: 2})
	q.Enqueue(&SyncOperation{Type: "warm", AgentID: "a1", Timestamp: 3})

	if q.Size() != 3 {
		t.Errorf("expected size 3, got %d", q.Size())
	}

	// Enqueue when full - should evict oldest non-critical
	q.Enqueue(&SyncOperation{Type: "full", AgentID: "a1", Timestamp: 4})
	if q.Size() != 3 {
		t.Errorf("expected size 3 after eviction, got %d", q.Size())
	}

	// Dequeue
	op := q.Dequeue()
	if op == nil {
		t.Fatal("expected non-nil dequeue")
	}
	if op.Type != "critical" {
		t.Errorf("expected critical (oldest non-evicted), got %q", op.Type)
	}

	// Clear
	q.Clear()
	if q.Size() != 0 {
		t.Error("expected empty after clear")
	}
}

func TestOfflineQueueAllCritical(t *testing.T) {
	q := NewOfflineQueue(2)
	q.Enqueue(&SyncOperation{Type: "critical", AgentID: "a1", Timestamp: 1})
	q.Enqueue(&SyncOperation{Type: "critical", AgentID: "a1", Timestamp: 2})
	// Full queue of critical ops - should evict oldest critical
	q.Enqueue(&SyncOperation{Type: "critical", AgentID: "a1", Timestamp: 3})
	if q.Size() != 2 {
		t.Errorf("expected size 2, got %d", q.Size())
	}
}

func TestOfflineQueueDefaultMaxSize(t *testing.T) {
	q := NewOfflineQueue(0)
	if q.maxSize != 1000 {
		t.Errorf("expected default maxSize 1000, got %d", q.maxSize)
	}

	q2 := NewOfflineQueue(-5)
	if q2.maxSize != 1000 {
		t.Errorf("expected default maxSize 1000, got %d", q2.maxSize)
	}
}
