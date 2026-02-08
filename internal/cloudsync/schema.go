package cloudsync

import (
	"context"
	"fmt"
)

// Schema defines all Turso database tables for EvoClaw cloud sync
const Schema = `
-- Agents table: Agent identity and genome
CREATE TABLE IF NOT EXISTS agents (
    agent_id TEXT PRIMARY KEY,
    device_key TEXT NOT NULL,
    name TEXT NOT NULL,
    model TEXT NOT NULL,
    capabilities TEXT, -- JSON array
    genome TEXT, -- JSON genome configuration
    persona TEXT, -- JSON persona data
    status TEXT DEFAULT 'active', -- active, stolen, deleted
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL
);

-- Core memory: Permanent relationship data (NEVER deleted)
CREATE TABLE IF NOT EXISTS core_memory (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    content TEXT NOT NULL, -- Encrypted JSON blob
    memory_type TEXT NOT NULL, -- owner_profile, relationship, personality_learned
    version INTEGER DEFAULT 1,
    created_at INTEGER NOT NULL,
    updated_at INTEGER NOT NULL,
    FOREIGN KEY (agent_id) REFERENCES agents(agent_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_core_memory_agent ON core_memory(agent_id);
CREATE INDEX IF NOT EXISTS idx_core_memory_type ON core_memory(agent_id, memory_type);

-- Warm memory: Recent conversations and events (30 day retention)
CREATE TABLE IF NOT EXISTS warm_memory (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    content TEXT NOT NULL, -- Encrypted conversation/event data
    event_type TEXT NOT NULL, -- conversation, action, observation
    timestamp INTEGER NOT NULL,
    distilled INTEGER DEFAULT 0, -- 0=raw, 1=compressed
    created_at INTEGER NOT NULL,
    expires_at INTEGER NOT NULL, -- Auto-evict after this timestamp
    FOREIGN KEY (agent_id) REFERENCES agents(agent_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_warm_memory_agent ON warm_memory(agent_id);
CREATE INDEX IF NOT EXISTS idx_warm_memory_timestamp ON warm_memory(agent_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_warm_memory_expires ON warm_memory(expires_at);

-- Evolution log: All evolution events with fitness scores
CREATE TABLE IF NOT EXISTS evolution_log (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    event_type TEXT NOT NULL, -- evaluation, mutation, selection
    fitness_score REAL,
    genome_before TEXT, -- JSON snapshot before mutation
    genome_after TEXT, -- JSON snapshot after mutation
    metrics TEXT, -- JSON performance metrics
    timestamp INTEGER NOT NULL,
    FOREIGN KEY (agent_id) REFERENCES agents(agent_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_evolution_log_agent ON evolution_log(agent_id);
CREATE INDEX IF NOT EXISTS idx_evolution_log_timestamp ON evolution_log(agent_id, timestamp DESC);

-- Action log: Agent actions synced from on-chain or local
CREATE TABLE IF NOT EXISTS action_log (
    id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    action_type TEXT NOT NULL, -- trade, message, skill_execution
    action_data TEXT NOT NULL, -- Encrypted JSON
    result TEXT, -- success, failure, pending
    error TEXT,
    on_chain_tx TEXT, -- Transaction hash if synced on-chain
    timestamp INTEGER NOT NULL,
    FOREIGN KEY (agent_id) REFERENCES agents(agent_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_action_log_agent ON action_log(agent_id);
CREATE INDEX IF NOT EXISTS idx_action_log_timestamp ON action_log(agent_id, timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_action_log_type ON action_log(agent_id, action_type);

-- Devices: Registered devices for multi-device sync
CREATE TABLE IF NOT EXISTS devices (
    device_id TEXT PRIMARY KEY,
    agent_id TEXT NOT NULL,
    device_key TEXT NOT NULL UNIQUE,
    device_name TEXT,
    device_type TEXT, -- phone, tablet, companion, hub
    last_heartbeat INTEGER,
    last_sync INTEGER,
    status TEXT DEFAULT 'active', -- active, offline, stolen
    created_at INTEGER NOT NULL,
    FOREIGN KEY (agent_id) REFERENCES agents(agent_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_devices_agent ON devices(agent_id);
CREATE INDEX IF NOT EXISTS idx_devices_key ON devices(device_key);

-- Sync state: Track what's been synced per device
CREATE TABLE IF NOT EXISTS sync_state (
    device_id TEXT NOT NULL,
    agent_id TEXT NOT NULL,
    sync_type TEXT NOT NULL, -- critical, warm, full
    last_sync_at INTEGER NOT NULL,
    last_sync_version INTEGER DEFAULT 1,
    sync_cursor TEXT, -- Opaque cursor for incremental sync
    PRIMARY KEY (device_id, agent_id, sync_type),
    FOREIGN KEY (device_id) REFERENCES devices(device_id) ON DELETE CASCADE,
    FOREIGN KEY (agent_id) REFERENCES agents(agent_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_sync_state_device ON sync_state(device_id);
CREATE INDEX IF NOT EXISTS idx_sync_state_agent ON sync_state(agent_id);
`

// InitSchema creates all tables in the Turso database
func (c *Client) InitSchema(ctx context.Context) error {
	// Split schema into individual statements
	statements := []string{
		// Agents table
		`CREATE TABLE IF NOT EXISTS agents (
			agent_id TEXT PRIMARY KEY,
			device_key TEXT NOT NULL,
			name TEXT NOT NULL,
			model TEXT NOT NULL,
			capabilities TEXT,
			genome TEXT,
			persona TEXT,
			status TEXT DEFAULT 'active',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		)`,
		
		// Core memory table
		`CREATE TABLE IF NOT EXISTS core_memory (
			id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			content TEXT NOT NULL,
			memory_type TEXT NOT NULL,
			version INTEGER DEFAULT 1,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			FOREIGN KEY (agent_id) REFERENCES agents(agent_id) ON DELETE CASCADE
		)`,
		
		`CREATE INDEX IF NOT EXISTS idx_core_memory_agent ON core_memory(agent_id)`,
		`CREATE INDEX IF NOT EXISTS idx_core_memory_type ON core_memory(agent_id, memory_type)`,
		
		// Warm memory table
		`CREATE TABLE IF NOT EXISTS warm_memory (
			id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			content TEXT NOT NULL,
			event_type TEXT NOT NULL,
			timestamp INTEGER NOT NULL,
			distilled INTEGER DEFAULT 0,
			created_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL,
			FOREIGN KEY (agent_id) REFERENCES agents(agent_id) ON DELETE CASCADE
		)`,
		
		`CREATE INDEX IF NOT EXISTS idx_warm_memory_agent ON warm_memory(agent_id)`,
		`CREATE INDEX IF NOT EXISTS idx_warm_memory_timestamp ON warm_memory(agent_id, timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_warm_memory_expires ON warm_memory(expires_at)`,
		
		// Evolution log table
		`CREATE TABLE IF NOT EXISTS evolution_log (
			id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			event_type TEXT NOT NULL,
			fitness_score REAL,
			genome_before TEXT,
			genome_after TEXT,
			metrics TEXT,
			timestamp INTEGER NOT NULL,
			FOREIGN KEY (agent_id) REFERENCES agents(agent_id) ON DELETE CASCADE
		)`,
		
		`CREATE INDEX IF NOT EXISTS idx_evolution_log_agent ON evolution_log(agent_id)`,
		`CREATE INDEX IF NOT EXISTS idx_evolution_log_timestamp ON evolution_log(agent_id, timestamp DESC)`,
		
		// Action log table
		`CREATE TABLE IF NOT EXISTS action_log (
			id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			action_type TEXT NOT NULL,
			action_data TEXT NOT NULL,
			result TEXT,
			error TEXT,
			on_chain_tx TEXT,
			timestamp INTEGER NOT NULL,
			FOREIGN KEY (agent_id) REFERENCES agents(agent_id) ON DELETE CASCADE
		)`,
		
		`CREATE INDEX IF NOT EXISTS idx_action_log_agent ON action_log(agent_id)`,
		`CREATE INDEX IF NOT EXISTS idx_action_log_timestamp ON action_log(agent_id, timestamp DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_action_log_type ON action_log(agent_id, action_type)`,
		
		// Devices table
		`CREATE TABLE IF NOT EXISTS devices (
			device_id TEXT PRIMARY KEY,
			agent_id TEXT NOT NULL,
			device_key TEXT NOT NULL UNIQUE,
			device_name TEXT,
			device_type TEXT,
			last_heartbeat INTEGER,
			last_sync INTEGER,
			status TEXT DEFAULT 'active',
			created_at INTEGER NOT NULL,
			FOREIGN KEY (agent_id) REFERENCES agents(agent_id) ON DELETE CASCADE
		)`,
		
		`CREATE INDEX IF NOT EXISTS idx_devices_agent ON devices(agent_id)`,
		`CREATE INDEX IF NOT EXISTS idx_devices_key ON devices(device_key)`,
		
		// Sync state table
		`CREATE TABLE IF NOT EXISTS sync_state (
			device_id TEXT NOT NULL,
			agent_id TEXT NOT NULL,
			sync_type TEXT NOT NULL,
			last_sync_at INTEGER NOT NULL,
			last_sync_version INTEGER DEFAULT 1,
			sync_cursor TEXT,
			PRIMARY KEY (device_id, agent_id, sync_type),
			FOREIGN KEY (device_id) REFERENCES devices(device_id) ON DELETE CASCADE,
			FOREIGN KEY (agent_id) REFERENCES agents(agent_id) ON DELETE CASCADE
		)`,
		
		`CREATE INDEX IF NOT EXISTS idx_sync_state_device ON sync_state(device_id)`,
		`CREATE INDEX IF NOT EXISTS idx_sync_state_agent ON sync_state(agent_id)`,
	}
	
	// Execute each statement
	for i, sql := range statements {
		if err := c.Execute(ctx, sql); err != nil {
			return fmt.Errorf("statement %d failed: %w", i+1, err)
		}
	}

	return nil
}

// CleanupExpiredMemory deletes warm memory past its expiration time
func (c *Client) CleanupExpiredMemory(ctx context.Context) (int64, error) {
	now := currentTimestamp()
	
	req := PipelineRequest{
		Requests: []BatchRequest{
			{
				Type: "execute",
				Statement: Statement{
					SQL:  "DELETE FROM warm_memory WHERE expires_at < ?",
					Args: []interface{}{now},
				},
			},
		},
	}

	resp, err := c.executePipeline(ctx, req)
	if err != nil {
		return 0, fmt.Errorf("cleanup expired memory: %w", err)
	}

	if len(resp.Results) > 0 && resp.Results[0].Type == "ok" {
		return resp.Results[0].RowsAffected, nil
	}

	return 0, nil
}
