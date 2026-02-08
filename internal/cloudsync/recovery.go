package cloudsync

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
)

// RecoveryManager handles agent restoration from cloud
type RecoveryManager struct {
	client *Client
	logger *slog.Logger
}

// NewRecoveryManager creates a new recovery manager
func NewRecoveryManager(client *Client, logger *slog.Logger) *RecoveryManager {
	if logger == nil {
		logger = slog.Default()
	}
	return &RecoveryManager{
		client: client,
		logger: logger,
	}
}

// RestoreAgent pulls full agent state from cloud
func (r *RecoveryManager) RestoreAgent(ctx context.Context, agentID string) (*AgentMemory, error) {
	r.logger.Info("restoring agent from cloud", "agent_id", agentID)

	// Get agent record
	agentRow, err := r.client.QueryOne(ctx,
		`SELECT agent_id, name, model, capabilities, genome, persona 
		 FROM agents WHERE agent_id = ? AND status = 'active'`,
		agentID,
	)
	if err != nil {
		return nil, fmt.Errorf("agent not found: %w", err)
	}

	memory := &AgentMemory{
		AgentID: agentRow[0].(string),
		Name:    agentRow[1].(string),
		Model:   agentRow[2].(string),
	}

	// Parse capabilities
	if agentRow[3] != nil {
		if err := json.Unmarshal([]byte(agentRow[3].(string)), &memory.Capabilities); err != nil {
			return nil, fmt.Errorf("parse capabilities: %w", err)
		}
	}

	// Parse genome
	if agentRow[4] != nil {
		if err := json.Unmarshal([]byte(agentRow[4].(string)), &memory.Genome); err != nil {
			return nil, fmt.Errorf("parse genome: %w", err)
		}
	}

	// Parse persona
	if agentRow[5] != nil {
		if err := json.Unmarshal([]byte(agentRow[5].(string)), &memory.Persona); err != nil {
			return nil, fmt.Errorf("parse persona: %w", err)
		}
	}

	// Get core memory
	coreResp, err := r.client.Query(ctx,
		`SELECT content FROM core_memory 
		 WHERE agent_id = ? 
		 ORDER BY updated_at DESC LIMIT 1`,
		agentID,
	)
	if err == nil && len(coreResp.Rows) > 0 {
		if coreResp.Rows[0][0] != nil {
			if err := json.Unmarshal([]byte(coreResp.Rows[0][0].(string)), &memory.CoreMemory); err != nil {
				r.logger.Warn("failed to parse core memory", "error", err)
			}
		}
	}

	r.logger.Info("agent restored successfully",
		"agent_id", agentID,
		"name", memory.Name)

	return memory, nil
}

// RestoreToDevice pairs a new device with an existing agent
func (r *RecoveryManager) RestoreToDevice(ctx context.Context, agentID, deviceID, deviceKey string) (*AgentMemory, error) {
	r.logger.Info("restoring agent to new device",
		"agent_id", agentID,
		"device_id", deviceID)

	// First restore the agent
	memory, err := r.RestoreAgent(ctx, agentID)
	if err != nil {
		return nil, err
	}

	// Register new device
	now := currentTimestamp()
	if err := r.client.Execute(ctx,
		`INSERT INTO devices (device_id, agent_id, device_key, device_name, device_type, last_heartbeat, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(device_id) DO UPDATE SET
		   agent_id = excluded.agent_id,
		   device_key = excluded.device_key,
		   last_heartbeat = excluded.last_heartbeat`,
		deviceID,
		agentID,
		deviceKey,
		"recovery-device",
		"replacement",
		now,
		now,
	); err != nil {
		return nil, fmt.Errorf("register device: %w", err)
	}

	r.logger.Info("device paired successfully",
		"agent_id", agentID,
		"device_id", deviceID)

	return memory, nil
}

// GetWarmMemory retrieves recent conversations for an agent
func (r *RecoveryManager) GetWarmMemory(ctx context.Context, agentID string, limit int) ([]WarmMemoryEntry, error) {
	if limit <= 0 {
		limit = 100
	}

	resp, err := r.client.Query(ctx,
		`SELECT id, event_type, content, timestamp, distilled
		 FROM warm_memory
		 WHERE agent_id = ?
		 ORDER BY timestamp DESC
		 LIMIT ?`,
		agentID,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query warm memory: %w", err)
	}

	entries := make([]WarmMemoryEntry, 0, len(resp.Rows))
	for _, row := range resp.Rows {
		entry := WarmMemoryEntry{
			ID:        row[0].(string),
			EventType: row[1].(string),
			Timestamp: int64(row[3].(float64)),
		}

		// Parse content JSON
		if row[2] != nil {
			if err := json.Unmarshal([]byte(row[2].(string)), &entry.Content); err != nil {
				r.logger.Warn("failed to parse warm memory content",
					"id", entry.ID,
					"error", err)
				continue
			}
		}

		// Parse distilled flag
		if row[4] != nil {
			entry.Distilled = int64(row[4].(float64)) == 1
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// GetEvolutionHistory retrieves evolution log for an agent
func (r *RecoveryManager) GetEvolutionHistory(ctx context.Context, agentID string, limit int) ([]EvolutionEntry, error) {
	if limit <= 0 {
		limit = 100
	}

	resp, err := r.client.Query(ctx,
		`SELECT id, event_type, fitness_score, genome_before, genome_after, metrics, timestamp
		 FROM evolution_log
		 WHERE agent_id = ?
		 ORDER BY timestamp DESC
		 LIMIT ?`,
		agentID,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query evolution log: %w", err)
	}

	entries := make([]EvolutionEntry, 0, len(resp.Rows))
	for _, row := range resp.Rows {
		entry := EvolutionEntry{
			ID:        row[0].(string),
			EventType: row[1].(string),
			Timestamp: int64(row[6].(float64)),
		}

		if row[2] != nil {
			entry.FitnessScore = row[2].(float64)
		}

		if row[3] != nil {
			json.Unmarshal([]byte(row[3].(string)), &entry.GenomeBefore)
		}

		if row[4] != nil {
			json.Unmarshal([]byte(row[4].(string)), &entry.GenomeAfter)
		}

		if row[5] != nil {
			json.Unmarshal([]byte(row[5].(string)), &entry.Metrics)
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// GetActionHistory retrieves action log for an agent
func (r *RecoveryManager) GetActionHistory(ctx context.Context, agentID string, limit int) ([]ActionEntry, error) {
	if limit <= 0 {
		limit = 100
	}

	resp, err := r.client.Query(ctx,
		`SELECT id, action_type, action_data, result, error, on_chain_tx, timestamp
		 FROM action_log
		 WHERE agent_id = ?
		 ORDER BY timestamp DESC
		 LIMIT ?`,
		agentID,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("query action log: %w", err)
	}

	entries := make([]ActionEntry, 0, len(resp.Rows))
	for _, row := range resp.Rows {
		entry := ActionEntry{
			ID:         row[0].(string),
			ActionType: row[1].(string),
			Result:     row[3].(string),
			Timestamp:  int64(row[6].(float64)),
		}

		if row[2] != nil {
			json.Unmarshal([]byte(row[2].(string)), &entry.Data)
		}

		if row[4] != nil {
			entry.Error = row[4].(string)
		}

		if row[5] != nil {
			entry.OnChainTx = row[5].(string)
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// MarkDeviceStolen marks a device as stolen and disables it
func (r *RecoveryManager) MarkDeviceStolen(ctx context.Context, deviceID string) error {
	r.logger.Warn("marking device as stolen", "device_id", deviceID)

	return r.client.Execute(ctx,
		"UPDATE devices SET status = 'stolen' WHERE device_id = ?",
		deviceID,
	)
}

// KillAgent marks an agent as deleted (for kill switch)
func (r *RecoveryManager) KillAgent(ctx context.Context, agentID string) error {
	r.logger.Warn("killing agent", "agent_id", agentID)

	return r.client.Execute(ctx,
		"UPDATE agents SET status = 'deleted' WHERE agent_id = ?",
		agentID,
	)
}

// ListDevices returns all devices for an agent
func (r *RecoveryManager) ListDevices(ctx context.Context, agentID string) ([]DeviceInfo, error) {
	resp, err := r.client.Query(ctx,
		`SELECT device_id, device_name, device_type, last_heartbeat, last_sync, status, created_at
		 FROM devices
		 WHERE agent_id = ?
		 ORDER BY last_heartbeat DESC`,
		agentID,
	)
	if err != nil {
		return nil, fmt.Errorf("query devices: %w", err)
	}

	devices := make([]DeviceInfo, 0, len(resp.Rows))
	for _, row := range resp.Rows {
		device := DeviceInfo{
			DeviceID:   row[0].(string),
			DeviceName: safeString(row[1]),
			DeviceType: safeString(row[2]),
			Status:     safeString(row[5]),
		}

		if row[3] != nil {
			device.LastHeartbeat = int64(row[3].(float64))
		}

		if row[4] != nil {
			device.LastSync = int64(row[4].(float64))
		}

		if row[6] != nil {
			device.CreatedAt = int64(row[6].(float64))
		}

		devices = append(devices, device)
	}

	return devices, nil
}

// DeviceInfo represents device metadata
type DeviceInfo struct {
	DeviceID      string `json:"device_id"`
	DeviceName    string `json:"device_name"`
	DeviceType    string `json:"device_type"`
	LastHeartbeat int64  `json:"last_heartbeat"`
	LastSync      int64  `json:"last_sync"`
	Status        string `json:"status"`
	CreatedAt     int64  `json:"created_at"`
}

// Helper to safely convert interface{} to string
func safeString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
