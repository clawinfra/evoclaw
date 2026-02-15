package cloudsync

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
)

// SyncEngine manages cloud synchronization for agent memory
type SyncEngine struct {
	client       *Client
	config       SyncConfig
	logger       *slog.Logger
	offlineQueue *OfflineQueue
	stopCh       chan struct{}
	wg           sync.WaitGroup
	mu           sync.RWMutex
	running      bool
}

// SyncConfig holds cloud sync configuration
type SyncConfig struct {
	Enabled                  bool
	DeviceID                 string
	DeviceKey                string
	HeartbeatIntervalSeconds int
	CriticalSyncEnabled      bool
	WarmSyncIntervalMinutes  int
	FullSyncIntervalHours    int
	FullSyncRequireWiFi      bool
	MaxOfflineQueueSize      int
}

// AgentMemory represents an agent's complete memory state
type AgentMemory struct {
	AgentID      string                 `json:"agent_id"`
	Name         string                 `json:"name"`
	Model        string                 `json:"model"`
	Capabilities []string               `json:"capabilities"`
	Genome       map[string]interface{} `json:"genome"`
	Persona      map[string]interface{} `json:"persona"`
	CoreMemory   map[string]interface{} `json:"core_memory"`
}

// MemorySnapshot is a point-in-time snapshot for sync
type MemorySnapshot struct {
	AgentID     string                   `json:"agent_id"`
	Timestamp   int64                    `json:"timestamp"`
	CoreMemory  map[string]interface{}   `json:"core_memory,omitempty"`
	WarmMemory  []WarmMemoryEntry        `json:"warm_memory,omitempty"`
	Evolution   []EvolutionEntry         `json:"evolution,omitempty"`
	Actions     []ActionEntry            `json:"actions,omitempty"`
	Genome      map[string]interface{}   `json:"genome,omitempty"`
}

// WarmMemoryEntry represents a conversation or event
type WarmMemoryEntry struct {
	ID        string                 `json:"id"`
	EventType string                 `json:"event_type"`
	Content   map[string]interface{} `json:"content"`
	Timestamp int64                  `json:"timestamp"`
	Distilled bool                   `json:"distilled"`
}

// EvolutionEntry tracks evolution events
type EvolutionEntry struct {
	ID           string                 `json:"id"`
	EventType    string                 `json:"event_type"`
	FitnessScore float64                `json:"fitness_score,omitempty"`
	GenomeBefore map[string]interface{} `json:"genome_before,omitempty"`
	GenomeAfter  map[string]interface{} `json:"genome_after,omitempty"`
	Metrics      map[string]float64     `json:"metrics,omitempty"`
	Timestamp    int64                  `json:"timestamp"`
}

// ActionEntry represents an agent action
type ActionEntry struct {
	ID         string                 `json:"id"`
	ActionType string                 `json:"action_type"`
	Data       map[string]interface{} `json:"data"`
	Result     string                 `json:"result"`
	Error      string                 `json:"error,omitempty"`
	OnChainTx  string                 `json:"on_chain_tx,omitempty"`
	Timestamp  int64                  `json:"timestamp"`
}

// NewSyncEngine creates a new sync engine
func NewSyncEngine(client *Client, config SyncConfig, logger *slog.Logger) *SyncEngine {
	if logger == nil {
		logger = slog.Default()
	}

	return &SyncEngine{
		client:       client,
		config:       config,
		logger:       logger,
		offlineQueue: NewOfflineQueue(config.MaxOfflineQueueSize),
		stopCh:       make(chan struct{}),
	}
}

// Start begins background sync operations
func (s *SyncEngine) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.running {
		return fmt.Errorf("sync engine already running")
	}

	if !s.config.Enabled {
		s.logger.Info("cloud sync disabled, skipping start")
		return nil
	}

	s.running = true

	// Register device
	if err := s.registerDevice(ctx); err != nil {
		s.logger.Warn("failed to register device", "error", err)
	}

	// Start background goroutines
	s.wg.Add(3)
	go s.heartbeatLoop(ctx)
	go s.warmSyncLoop(ctx)
	go s.fullSyncLoop(ctx)

	s.logger.Info("cloud sync engine started",
		"device_id", s.config.DeviceID,
		"heartbeat_interval", s.config.HeartbeatIntervalSeconds,
		"warm_sync_interval", s.config.WarmSyncIntervalMinutes,
		"full_sync_interval", s.config.FullSyncIntervalHours)

	return nil
}

// Stop gracefully shuts down sync operations
func (s *SyncEngine) Stop() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !s.running {
		return nil
	}

	s.logger.Info("stopping cloud sync engine")
	close(s.stopCh)
	s.wg.Wait()
	s.running = false

	return nil
}

// CriticalSync syncs core memory and genome (after every conversation)
func (s *SyncEngine) CriticalSync(ctx context.Context, memory *AgentMemory) error {
	if !s.config.CriticalSyncEnabled {
		return nil
	}

	s.logger.Debug("critical sync started", "agent_id", memory.AgentID)

	// Prepare statements
	now := currentTimestamp()
	
	coreMemoryJSON, err := json.Marshal(memory.CoreMemory)
	if err != nil {
		return fmt.Errorf("marshal core memory: %w", err)
	}

	genomeJSON, err := json.Marshal(memory.Genome)
	if err != nil {
		return fmt.Errorf("marshal genome: %w", err)
	}

	personaJSON, err := json.Marshal(memory.Persona)
	if err != nil {
		return fmt.Errorf("marshal persona: %w", err)
	}

	capabilitiesJSON, err := json.Marshal(memory.Capabilities)
	if err != nil {
		return fmt.Errorf("marshal capabilities: %w", err)
	}

	statements := []Statement{
		// Upsert agent
		{
			SQL: `INSERT INTO agents (agent_id, device_key, name, model, capabilities, genome, persona, created_at, updated_at)
				  VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
				  ON CONFLICT(agent_id) DO UPDATE SET
				    name = excluded.name,
				    model = excluded.model,
				    capabilities = excluded.capabilities,
				    genome = excluded.genome,
				    persona = excluded.persona,
				    updated_at = excluded.updated_at`,
			Args: []interface{}{
				memory.AgentID,
				s.config.DeviceKey,
				memory.Name,
				memory.Model,
				string(capabilitiesJSON),
				string(genomeJSON),
				string(personaJSON),
				now,
				now,
			},
		},
		// Upsert core memory
		{
			SQL: `INSERT INTO core_memory (id, agent_id, content, memory_type, created_at, updated_at)
				  VALUES (?, ?, ?, ?, ?, ?)
				  ON CONFLICT(id) DO UPDATE SET
				    content = excluded.content,
				    updated_at = excluded.updated_at`,
			Args: []interface{}{
				uuid.New().String(),
				memory.AgentID,
				string(coreMemoryJSON),
				"full",
				now,
				now,
			},
		},
		// Update sync state
		{
			SQL: `INSERT INTO sync_state (device_id, agent_id, sync_type, last_sync_at, last_sync_version)
				  VALUES (?, ?, ?, ?, ?)
				  ON CONFLICT(device_id, agent_id, sync_type) DO UPDATE SET
				    last_sync_at = excluded.last_sync_at,
				    last_sync_version = last_sync_version + 1`,
			Args: []interface{}{
				s.config.DeviceID,
				memory.AgentID,
				"critical",
				now,
				1,
			},
		},
	}

	// Execute with retry
	if err := s.client.BatchExecute(ctx, statements); err != nil {
		// Queue for offline retry
		s.offlineQueue.Enqueue(&SyncOperation{
			Type:      "critical",
			AgentID:   memory.AgentID,
			Data:      memory,
			Timestamp: now,
		})
		return fmt.Errorf("critical sync failed (queued for retry): %w", err)
	}

	s.logger.Info("critical sync completed", "agent_id", memory.AgentID)
	return nil
}

// WarmSync syncs recent events (hourly)
func (s *SyncEngine) WarmSync(ctx context.Context, snapshot *MemorySnapshot) error {
	s.logger.Debug("warm sync started", "agent_id", snapshot.AgentID)

	if len(snapshot.WarmMemory) == 0 {
		s.logger.Debug("no warm memory to sync")
		return nil
	}

	now := currentTimestamp()
	expiresAt := now + (30 * 24 * 3600) // 30 days from now

	statements := make([]Statement, 0, len(snapshot.WarmMemory)+1)

	// Insert warm memory entries
	for _, entry := range snapshot.WarmMemory {
		contentJSON, err := json.Marshal(entry.Content)
		if err != nil {
			return fmt.Errorf("marshal warm memory content: %w", err)
		}

		distilled := 0
		if entry.Distilled {
			distilled = 1
		}

		statements = append(statements, Statement{
			SQL: `INSERT INTO warm_memory (id, agent_id, content, event_type, timestamp, distilled, created_at, expires_at)
				  VALUES (?, ?, ?, ?, ?, ?, ?, ?)
				  ON CONFLICT(id) DO NOTHING`,
			Args: []interface{}{
				entry.ID,
				snapshot.AgentID,
				string(contentJSON),
				entry.EventType,
				entry.Timestamp,
				distilled,
				now,
				expiresAt,
			},
		})
	}

	// Update sync state
	statements = append(statements, Statement{
		SQL: `INSERT INTO sync_state (device_id, agent_id, sync_type, last_sync_at, last_sync_version)
			  VALUES (?, ?, ?, ?, ?)
			  ON CONFLICT(device_id, agent_id, sync_type) DO UPDATE SET
			    last_sync_at = excluded.last_sync_at,
			    last_sync_version = last_sync_version + 1`,
		Args: []interface{}{
			s.config.DeviceID,
			snapshot.AgentID,
			"warm",
			now,
			1,
		},
	})

	if err := s.client.BatchExecute(ctx, statements); err != nil {
		s.offlineQueue.Enqueue(&SyncOperation{
			Type:      "warm",
			AgentID:   snapshot.AgentID,
			Data:      snapshot,
			Timestamp: now,
		})
		return fmt.Errorf("warm sync failed (queued for retry): %w", err)
	}

	s.logger.Info("warm sync completed",
		"agent_id", snapshot.AgentID,
		"entries", len(snapshot.WarmMemory))
	return nil
}

// FullSync performs a complete backup (daily)
func (s *SyncEngine) FullSync(ctx context.Context, snapshot *MemorySnapshot) error {
	s.logger.Debug("full sync started", "agent_id", snapshot.AgentID)

	now := currentTimestamp()
	statements := make([]Statement, 0)

	// Sync evolution log
	for _, entry := range snapshot.Evolution {
		beforeJSON, _ := json.Marshal(entry.GenomeBefore)
		afterJSON, _ := json.Marshal(entry.GenomeAfter)
		metricsJSON, _ := json.Marshal(entry.Metrics)

		statements = append(statements, Statement{
			SQL: `INSERT INTO evolution_log (id, agent_id, event_type, fitness_score, genome_before, genome_after, metrics, timestamp)
				  VALUES (?, ?, ?, ?, ?, ?, ?, ?)
				  ON CONFLICT(id) DO NOTHING`,
			Args: []interface{}{
				entry.ID,
				snapshot.AgentID,
				entry.EventType,
				entry.FitnessScore,
				string(beforeJSON),
				string(afterJSON),
				string(metricsJSON),
				entry.Timestamp,
			},
		})
	}

	// Sync action log
	for _, action := range snapshot.Actions {
		dataJSON, _ := json.Marshal(action.Data)

		statements = append(statements, Statement{
			SQL: `INSERT INTO action_log (id, agent_id, action_type, action_data, result, error, on_chain_tx, timestamp)
				  VALUES (?, ?, ?, ?, ?, ?, ?, ?)
				  ON CONFLICT(id) DO NOTHING`,
			Args: []interface{}{
				action.ID,
				snapshot.AgentID,
				action.ActionType,
				string(dataJSON),
				action.Result,
				action.Error,
				action.OnChainTx,
				action.Timestamp,
			},
		})
	}

	// Update sync state
	statements = append(statements, Statement{
		SQL: `INSERT INTO sync_state (device_id, agent_id, sync_type, last_sync_at, last_sync_version)
			  VALUES (?, ?, ?, ?, ?)
			  ON CONFLICT(device_id, agent_id, sync_type) DO UPDATE SET
			    last_sync_at = excluded.last_sync_at,
			    last_sync_version = last_sync_version + 1`,
		Args: []interface{}{
			s.config.DeviceID,
			snapshot.AgentID,
			"full",
			now,
			1,
		},
	})

	if err := s.client.BatchExecute(ctx, statements); err != nil {
		s.offlineQueue.Enqueue(&SyncOperation{
			Type:      "full",
			AgentID:   snapshot.AgentID,
			Data:      snapshot,
			Timestamp: now,
		})
		return fmt.Errorf("full sync failed (queued for retry): %w", err)
	}

	s.logger.Info("full sync completed",
		"agent_id", snapshot.AgentID,
		"evolution_entries", len(snapshot.Evolution),
		"action_entries", len(snapshot.Actions))
	return nil
}

// registerDevice registers this device with the cloud
func (s *SyncEngine) registerDevice(ctx context.Context) error {
	now := currentTimestamp()

	return s.client.Execute(ctx,
		`INSERT INTO devices (device_id, agent_id, device_key, device_name, device_type, last_heartbeat, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(device_id) DO UPDATE SET
		   last_heartbeat = excluded.last_heartbeat`,
		s.config.DeviceID,
		"unknown", // Will be set during first sync
		s.config.DeviceKey,
		"evoclaw-device",
		"orchestrator",
		now,
		now,
	)
}

// heartbeatLoop sends periodic heartbeats
func (s *SyncEngine) heartbeatLoop(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(time.Duration(s.config.HeartbeatIntervalSeconds) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.sendHeartbeat(ctx); err != nil {
				s.logger.Warn("heartbeat failed", "error", err)
			}
		}
	}
}

// warmSyncLoop performs periodic warm syncs
func (s *SyncEngine) warmSyncLoop(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(time.Duration(s.config.WarmSyncIntervalMinutes) * time.Minute)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.processOfflineQueue(ctx)
		}
	}
}

// fullSyncLoop performs periodic full syncs
func (s *SyncEngine) fullSyncLoop(ctx context.Context) {
	defer s.wg.Done()

	ticker := time.NewTicker(time.Duration(s.config.FullSyncIntervalHours) * time.Hour)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.logger.Debug("full sync timer triggered")
			// Full sync is triggered by the orchestrator with complete data
		}
	}
}

// sendHeartbeat updates device heartbeat timestamp
func (s *SyncEngine) sendHeartbeat(ctx context.Context) error {
	now := currentTimestamp()
	return s.client.Execute(ctx,
		"UPDATE devices SET last_heartbeat = ? WHERE device_id = ?",
		now,
		s.config.DeviceID,
	)
}

// processOfflineQueue attempts to sync queued operations
func (s *SyncEngine) processOfflineQueue(ctx context.Context) {
	for {
		op := s.offlineQueue.Dequeue()
		if op == nil {
			break
		}

		s.logger.Debug("processing queued sync operation",
			"type", op.Type,
			"agent_id", op.AgentID)

		var err error
		switch op.Type {
		case "critical":
			if memory, ok := op.Data.(*AgentMemory); ok {
				err = s.CriticalSync(ctx, memory)
			}
		case "warm":
			if snapshot, ok := op.Data.(*MemorySnapshot); ok {
				err = s.WarmSync(ctx, snapshot)
			}
		case "full":
			if snapshot, ok := op.Data.(*MemorySnapshot); ok {
				err = s.FullSync(ctx, snapshot)
			}
		}

		if err != nil {
			// Re-queue if still failing
			s.offlineQueue.Enqueue(op)
			s.logger.Warn("queued sync operation failed again",
				"type", op.Type,
				"agent_id", op.AgentID,
				"error", err)
			break
		}
	}
}
