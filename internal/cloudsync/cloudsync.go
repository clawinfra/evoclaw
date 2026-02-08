// Package cloudsync provides cloud synchronization for EvoClaw agent memory
// using Turso (libSQL) as the cloud database backend.
//
// This package implements the CLOUD-SYNC.md design specification with:
// - Zero CGO dependencies (uses Turso HTTP API)
// - Tiered memory model (core, warm, cold)
// - Multi-device sync with conflict resolution
// - Offline queue for unreliable connections
// - Disaster recovery (device replacement, theft protection)
package cloudsync

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/google/uuid"
)

// Manager is the main interface for cloud sync operations
type Manager struct {
	client   *Client
	engine   *SyncEngine
	recovery *RecoveryManager
	config   config.CloudSyncConfig
	logger   *slog.Logger
}

// NewManager creates a new cloud sync manager from config
func NewManager(cfg config.CloudSyncConfig, logger *slog.Logger) (*Manager, error) {
	if !cfg.Enabled {
		return &Manager{config: cfg, logger: logger}, nil
	}

	if cfg.DatabaseURL == "" {
		return nil, fmt.Errorf("database URL is required")
	}

	if cfg.AuthToken == "" {
		return nil, fmt.Errorf("auth token is required")
	}

	if logger == nil {
		logger = slog.Default()
	}

	// Generate device ID if not set
	if cfg.DeviceID == "" {
		cfg.DeviceID = uuid.New().String()
	}

	// Generate device key if not set
	if cfg.DeviceKey == "" {
		cfg.DeviceKey = uuid.New().String()
	}

	// Set defaults
	if cfg.HeartbeatIntervalSeconds <= 0 {
		cfg.HeartbeatIntervalSeconds = 60
	}
	if cfg.WarmSyncIntervalMinutes <= 0 {
		cfg.WarmSyncIntervalMinutes = 60
	}
	if cfg.FullSyncIntervalHours <= 0 {
		cfg.FullSyncIntervalHours = 24
	}
	if cfg.MaxOfflineQueueSize <= 0 {
		cfg.MaxOfflineQueueSize = 1000
	}

	client := NewClient(cfg.DatabaseURL, cfg.AuthToken, logger)
	
	syncConfig := SyncConfig{
		Enabled:                  cfg.Enabled,
		DeviceID:                 cfg.DeviceID,
		DeviceKey:                cfg.DeviceKey,
		HeartbeatIntervalSeconds: cfg.HeartbeatIntervalSeconds,
		CriticalSyncEnabled:      cfg.CriticalSyncEnabled,
		WarmSyncIntervalMinutes:  cfg.WarmSyncIntervalMinutes,
		FullSyncIntervalHours:    cfg.FullSyncIntervalHours,
		FullSyncRequireWiFi:      cfg.FullSyncRequireWiFi,
		MaxOfflineQueueSize:      cfg.MaxOfflineQueueSize,
	}

	engine := NewSyncEngine(client, syncConfig, logger)
	recovery := NewRecoveryManager(client, logger)

	return &Manager{
		client:   client,
		engine:   engine,
		recovery: recovery,
		config:   cfg,
		logger:   logger,
	}, nil
}

// InitSchema creates all database tables
func (m *Manager) InitSchema(ctx context.Context) error {
	if !m.config.Enabled {
		return nil
	}
	return m.client.InitSchema(ctx)
}

// Start begins background sync operations
func (m *Manager) Start(ctx context.Context) error {
	if !m.config.Enabled {
		m.logger.Info("cloud sync disabled")
		return nil
	}
	return m.engine.Start(ctx)
}

// Stop gracefully shuts down sync operations
func (m *Manager) Stop() error {
	if !m.config.Enabled {
		return nil
	}
	return m.engine.Stop()
}

// SyncCritical syncs core memory and genome (after every conversation)
func (m *Manager) SyncCritical(ctx context.Context, memory *AgentMemory) error {
	if !m.config.Enabled {
		return nil
	}
	return m.engine.CriticalSync(ctx, memory)
}

// SyncWarm syncs recent events (hourly)
func (m *Manager) SyncWarm(ctx context.Context, snapshot *MemorySnapshot) error {
	if !m.config.Enabled {
		return nil
	}
	return m.engine.WarmSync(ctx, snapshot)
}

// SyncFull performs complete backup (daily)
func (m *Manager) SyncFull(ctx context.Context, snapshot *MemorySnapshot) error {
	if !m.config.Enabled {
		return nil
	}
	return m.engine.FullSync(ctx, snapshot)
}

// RestoreAgent pulls full agent state from cloud
func (m *Manager) RestoreAgent(ctx context.Context, agentID string) (*AgentMemory, error) {
	if !m.config.Enabled {
		return nil, fmt.Errorf("cloud sync disabled")
	}
	return m.recovery.RestoreAgent(ctx, agentID)
}

// RestoreToDevice pairs a new device with an existing agent
func (m *Manager) RestoreToDevice(ctx context.Context, agentID, deviceID, deviceKey string) (*AgentMemory, error) {
	if !m.config.Enabled {
		return nil, fmt.Errorf("cloud sync disabled")
	}
	return m.recovery.RestoreToDevice(ctx, agentID, deviceID, deviceKey)
}

// GetWarmMemory retrieves recent conversations
func (m *Manager) GetWarmMemory(ctx context.Context, agentID string, limit int) ([]WarmMemoryEntry, error) {
	if !m.config.Enabled {
		return nil, fmt.Errorf("cloud sync disabled")
	}
	return m.recovery.GetWarmMemory(ctx, agentID, limit)
}

// GetEvolutionHistory retrieves evolution log
func (m *Manager) GetEvolutionHistory(ctx context.Context, agentID string, limit int) ([]EvolutionEntry, error) {
	if !m.config.Enabled {
		return nil, fmt.Errorf("cloud sync disabled")
	}
	return m.recovery.GetEvolutionHistory(ctx, agentID, limit)
}

// GetActionHistory retrieves action log
func (m *Manager) GetActionHistory(ctx context.Context, agentID string, limit int) ([]ActionEntry, error) {
	if !m.config.Enabled {
		return nil, fmt.Errorf("cloud sync disabled")
	}
	return m.recovery.GetActionHistory(ctx, agentID, limit)
}

// MarkDeviceStolen marks a device as stolen
func (m *Manager) MarkDeviceStolen(ctx context.Context, deviceID string) error {
	if !m.config.Enabled {
		return fmt.Errorf("cloud sync disabled")
	}
	return m.recovery.MarkDeviceStolen(ctx, deviceID)
}

// ListDevices returns all devices for an agent
func (m *Manager) ListDevices(ctx context.Context, agentID string) ([]DeviceInfo, error) {
	if !m.config.Enabled {
		return nil, fmt.Errorf("cloud sync disabled")
	}
	return m.recovery.ListDevices(ctx, agentID)
}

// CleanupExpiredMemory deletes warm memory past expiration
func (m *Manager) CleanupExpiredMemory(ctx context.Context) (int64, error) {
	if !m.config.Enabled {
		return 0, nil
	}
	return m.client.CleanupExpiredMemory(ctx)
}

// DeviceID returns the current device ID
func (m *Manager) DeviceID() string {
	return m.config.DeviceID
}

// DeviceKey returns the current device key
func (m *Manager) DeviceKey() string {
	return m.config.DeviceKey
}

// IsEnabled returns whether cloud sync is enabled
func (m *Manager) IsEnabled() bool {
	return m.config.Enabled
}
