package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/clawinfra/evoclaw/internal/cloudsync"
)

const (
	ColdRetentionYears = 10
)

// ColdMemory represents the cold tier — unlimited archive in Turso
type ColdMemory struct {
	client   *cloudsync.Client
	agentID  string
	logger   *slog.Logger
}

// ColdEntry represents a single cold memory entry
type ColdEntry struct {
	ID               string    `json:"id"`
	AgentID          string    `json:"agent_id"`
	Timestamp        int64     `json:"timestamp"` // Unix timestamp
	EventType        string    `json:"event_type"`
	Category         string    `json:"category"`
	Content          string    `json:"content"` // JSON blob
	DistilledSummary string    `json:"distilled_summary"`
	Importance       float64   `json:"importance"`
	AccessCount      int       `json:"access_count"`
	LastAccessed     *int64    `json:"last_accessed,omitempty"`
	CreatedAt        int64     `json:"created_at"`
}

// NewColdMemory creates a new cold memory store
func NewColdMemory(client *cloudsync.Client, agentID string, logger *slog.Logger) *ColdMemory {
	if logger == nil {
		logger = slog.Default()
	}

	return &ColdMemory{
		client:  client,
		agentID: agentID,
		logger:  logger,
	}
}

// InitSchema creates the cold_memory table if it doesn't exist
func (c *ColdMemory) InitSchema(ctx context.Context) error {
	// Turso HTTP pipeline API requires one statement per execute request.
	// Bundling multiple statements causes SQL_MANY_STATEMENTS error.
	// Use BatchExecute to run each DDL statement individually in order.
	statements := []cloudsync.Statement{
		{SQL: `CREATE TABLE IF NOT EXISTS cold_memory (
	id TEXT PRIMARY KEY,
	agent_id TEXT NOT NULL,
	timestamp INTEGER NOT NULL,
	event_type TEXT NOT NULL,
	category TEXT NOT NULL,
	content TEXT NOT NULL,
	distilled_summary TEXT NOT NULL,
	importance REAL DEFAULT 0.5,
	access_count INTEGER DEFAULT 0,
	last_accessed INTEGER,
	created_at INTEGER NOT NULL
)`},
		{SQL: `CREATE INDEX IF NOT EXISTS idx_cold_category ON cold_memory(agent_id, category)`},
		{SQL: `CREATE INDEX IF NOT EXISTS idx_cold_timestamp ON cold_memory(agent_id, timestamp DESC)`},
		{SQL: `CREATE INDEX IF NOT EXISTS idx_cold_importance ON cold_memory(agent_id, importance DESC)`},
	}

	c.logger.Info("initializing cold_memory schema")
	return c.client.BatchExecute(ctx, statements)
}

// Add archives a warm entry to cold storage
func (c *ColdMemory) Add(ctx context.Context, entry *WarmEntry) error {
	// Convert content to JSON string
	contentJSON, err := json.Marshal(entry.Content)
	if err != nil {
		return fmt.Errorf("marshal content: %w", err)
	}

	// Generate distilled summary
	summary := entry.Content.Fact
	if len(summary) > 100 {
		summary = summary[:100]
	}

	now := time.Now().Unix()
	lastAccessed := entry.LastAccessed.Unix()

	sql := `
INSERT INTO cold_memory (
	id, agent_id, timestamp, event_type, category,
	content, distilled_summary, importance,
	access_count, last_accessed, created_at
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
`

	err = c.client.Execute(ctx, sql,
		entry.ID,
		c.agentID,
		entry.Timestamp.Unix(),
		entry.EventType,
		entry.Category,
		string(contentJSON),
		summary,
		entry.Importance,
		entry.AccessCount,
		lastAccessed,
		now,
	)

	if err != nil {
		return fmt.Errorf("insert cold entry: %w", err)
	}

	c.logger.Debug("archived to cold storage",
		"id", entry.ID,
		"category", entry.Category)

	return nil
}

// Get retrieves a single entry by ID
func (c *ColdMemory) Get(ctx context.Context, id string) (*ColdEntry, error) {
	sql := `
SELECT id, agent_id, timestamp, event_type, category,
       content, distilled_summary, importance,
       access_count, last_accessed, created_at
FROM cold_memory
WHERE agent_id = ? AND id = ?
`

	resp, err := c.client.Query(ctx, sql, c.agentID, id)
	if err != nil {
		return nil, fmt.Errorf("query cold entry: %w", err)
	}

	if len(resp.Rows) == 0 {
		return nil, fmt.Errorf("entry not found")
	}

	entry, err := c.rowToColdEntry(resp.Rows[0])
	if err != nil {
		return nil, err
	}

	// Increment access count
	_ = c.IncrementAccess(ctx, id)

	return entry, nil
}

// GetByCategory retrieves all entries in a category
func (c *ColdMemory) GetByCategory(ctx context.Context, category string, limit int) ([]*ColdEntry, error) {
	if limit <= 0 {
		limit = 100
	}

	sql := `
SELECT id, agent_id, timestamp, event_type, category,
       content, distilled_summary, importance,
       access_count, last_accessed, created_at
FROM cold_memory
WHERE agent_id = ? AND category = ?
ORDER BY importance DESC, timestamp DESC
LIMIT ?
`

	resp, err := c.client.Query(ctx, sql, c.agentID, category, limit)
	if err != nil {
		return nil, fmt.Errorf("query by category: %w", err)
	}

	entries := make([]*ColdEntry, 0, len(resp.Rows))
	for _, row := range resp.Rows {
		entry, err := c.rowToColdEntry(row)
		if err != nil {
			c.logger.Warn("failed to parse row", "error", err)
			continue
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// QueryByImportance retrieves top N most important entries
func (c *ColdMemory) QueryByImportance(ctx context.Context, minImportance float64, limit int) ([]*ColdEntry, error) {
	if limit <= 0 {
		limit = 50
	}

	sql := `
SELECT id, agent_id, timestamp, event_type, category,
       content, distilled_summary, importance,
       access_count, last_accessed, created_at
FROM cold_memory
WHERE agent_id = ? AND importance >= ?
ORDER BY importance DESC
LIMIT ?
`

	resp, err := c.client.Query(ctx, sql, c.agentID, minImportance, limit)
	if err != nil {
		return nil, fmt.Errorf("query by importance: %w", err)
	}

	entries := make([]*ColdEntry, 0, len(resp.Rows))
	for _, row := range resp.Rows {
		entry, err := c.rowToColdEntry(row)
		if err != nil {
			c.logger.Warn("failed to parse row", "error", err)
			continue
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// QueryByTimeRange retrieves entries in a time range
func (c *ColdMemory) QueryByTimeRange(ctx context.Context, start, end time.Time, limit int) ([]*ColdEntry, error) {
	if limit <= 0 {
		limit = 100
	}

	sql := `
SELECT id, agent_id, timestamp, event_type, category,
       content, distilled_summary, importance,
       access_count, last_accessed, created_at
FROM cold_memory
WHERE agent_id = ? AND timestamp >= ? AND timestamp <= ?
ORDER BY timestamp DESC
LIMIT ?
`

	resp, err := c.client.Query(ctx, sql, c.agentID, start.Unix(), end.Unix(), limit)
	if err != nil {
		return nil, fmt.Errorf("query by time range: %w", err)
	}

	entries := make([]*ColdEntry, 0, len(resp.Rows))
	for _, row := range resp.Rows {
		entry, err := c.rowToColdEntry(row)
		if err != nil {
			c.logger.Warn("failed to parse row", "error", err)
			continue
		}
		entries = append(entries, entry)
	}

	return entries, nil
}

// IncrementAccess increments the access count for an entry
func (c *ColdMemory) IncrementAccess(ctx context.Context, id string) error {
	now := time.Now().Unix()

	sql := `
UPDATE cold_memory
SET access_count = access_count + 1, last_accessed = ?
WHERE agent_id = ? AND id = ?
`

	return c.client.Execute(ctx, sql, now, c.agentID, id)
}

// DeleteFrozen removes frozen entries (score < 0.05) older than retention period
func (c *ColdMemory) DeleteFrozen(ctx context.Context, retentionYears int, scoreConfig ScoreConfig) (int, error) {
	// Calculate cutoff timestamp
	cutoff := time.Now().AddDate(-retentionYears, 0, 0).Unix()

	// Query all old entries
	sql := `
SELECT id, timestamp, importance, access_count, created_at
FROM cold_memory
WHERE agent_id = ? AND created_at < ?
`

	resp, err := c.client.Query(ctx, sql, c.agentID, cutoff)
	if err != nil {
		return 0, fmt.Errorf("query old entries: %w", err)
	}

	// Calculate scores and identify frozen entries
	frozen := make([]string, 0)
	for _, row := range resp.Rows {
		id := row[0].(string)
		timestamp := int64(row[1].(float64))
		importance := row[2].(float64)
		accessCount := int(row[3].(float64))

		createdAt := time.Unix(timestamp, 0)
		score := CalculateScore(importance, createdAt, accessCount, scoreConfig)

		if score < 0.05 {
			frozen = append(frozen, id)
		}
	}

	if len(frozen) == 0 {
		return 0, nil
	}

	// Delete frozen entries in batches
	batchSize := 100
	deleted := 0

	for i := 0; i < len(frozen); i += batchSize {
		end := i + batchSize
		if end > len(frozen) {
			end = len(frozen)
		}

		batch := frozen[i:end]
		placeholders := ""
		for j := range batch {
			if j > 0 {
				placeholders += ","
			}
			placeholders += "?"
		}

		deleteSql := fmt.Sprintf(`
DELETE FROM cold_memory
WHERE agent_id = ? AND id IN (%s)
`, placeholders)

		args := make([]interface{}, len(batch)+1)
		args[0] = c.agentID
		for j, id := range batch {
			args[j+1] = id
		}

		if err := c.client.Execute(ctx, deleteSql, args...); err != nil {
			c.logger.Warn("failed to delete batch", "error", err)
			continue
		}

		deleted += len(batch)
	}

	c.logger.Info("deleted frozen entries",
		"count", deleted,
		"retention_years", retentionYears)

	return deleted, nil
}

// Count returns the total number of cold entries for this agent
func (c *ColdMemory) Count(ctx context.Context) (int, error) {
	sql := `SELECT COUNT(*) FROM cold_memory WHERE agent_id = ?`

	resp, err := c.client.Query(ctx, sql, c.agentID)
	if err != nil {
		return 0, fmt.Errorf("count cold entries: %w", err)
	}

	if len(resp.Rows) == 0 {
		return 0, nil
	}

	count := int(resp.Rows[0][0].(float64))
	return count, nil
}

// rowToColdEntry converts a database row to a ColdEntry
func (c *ColdMemory) rowToColdEntry(row []interface{}) (*ColdEntry, error) {
	if len(row) < 11 {
		return nil, fmt.Errorf("invalid row: expected 11 columns, got %d", len(row))
	}

	entry := &ColdEntry{
		ID:               row[0].(string),
		AgentID:          row[1].(string),
		Timestamp:        int64(row[2].(float64)),
		EventType:        row[3].(string),
		Category:         row[4].(string),
		Content:          row[5].(string),
		DistilledSummary: row[6].(string),
		Importance:       row[7].(float64),
		AccessCount:      int(row[8].(float64)),
		CreatedAt:        int64(row[10].(float64)),
	}

	// last_accessed is nullable
	if row[9] != nil {
		lastAccessed := int64(row[9].(float64))
		entry.LastAccessed = &lastAccessed
	}

	return entry, nil
}
