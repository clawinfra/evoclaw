package agents

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/clawinfra/evoclaw/internal/orchestrator"
)

// MemoryStore manages conversation memory for agents
type MemoryStore struct {
	dataDir string
	logger  *slog.Logger
	mu      sync.RWMutex
	// In-memory cache of recent conversations
	cache map[string]*ConversationMemory
}

// ConversationMemory stores chat history for an agent
type ConversationMemory struct {
	AgentID      string                     `json:"agent_id"`
	Messages     []orchestrator.ChatMessage `json:"messages"`
	MaxMessages  int                        `json:"max_messages"`
	TotalTokens  int                        `json:"total_tokens"`
	TokenLimit   int                        `json:"token_limit"`
	LastAccessed time.Time                  `json:"last_accessed"`
	mu           sync.RWMutex
}

// NewMemoryStore creates a new memory store
func NewMemoryStore(dataDir string, logger *slog.Logger) (*MemoryStore, error) {
	memoryDir := filepath.Join(dataDir, "memory")
	if err := os.MkdirAll(memoryDir, 0750); err != nil {
		return nil, fmt.Errorf("create memory dir: %w", err)
	}

	return &MemoryStore{
		dataDir: memoryDir,
		logger:  logger.With("component", "memory"),
		cache:   make(map[string]*ConversationMemory),
	}, nil
}

// Get retrieves or creates conversation memory for an agent
func (m *MemoryStore) Get(agentID string) *ConversationMemory {
	m.mu.RLock()
	mem, ok := m.cache[agentID]
	m.mu.RUnlock()

	if ok {
		mem.mu.Lock()
		mem.LastAccessed = time.Now()
		mem.mu.Unlock()
		return mem
	}

	// Try to load from disk
	mem = m.loadFromDisk(agentID)
	if mem != nil {
		m.mu.Lock()
		m.cache[agentID] = mem
		m.mu.Unlock()
		return mem
	}

	// Create new memory
	mem = &ConversationMemory{
		AgentID:      agentID,
		Messages:     make([]orchestrator.ChatMessage, 0),
		MaxMessages:  100,    // Keep last 100 messages
		TokenLimit:   100000, // Rough token limit
		LastAccessed: time.Now(),
	}

	m.mu.Lock()
	m.cache[agentID] = mem
	m.mu.Unlock()

	m.logger.Info("conversation memory created", "agent", agentID)
	return mem
}

// Add adds a message to the conversation history
func (c *ConversationMemory) Add(role, content string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	msg := orchestrator.ChatMessage{
		Role:    role,
		Content: content,
	}

	c.Messages = append(c.Messages, msg)
	c.TotalTokens += estimateTokens(content)

	// Trim if we exceed limits
	c.trim()

	c.LastAccessed = time.Now()
}

// GetMessages returns all messages in the conversation
func (c *ConversationMemory) GetMessages() []orchestrator.ChatMessage {
	c.mu.RLock()
	defer c.mu.RUnlock()

	// Return a copy
	msgs := make([]orchestrator.ChatMessage, len(c.Messages))
	copy(msgs, c.Messages)
	return msgs
}

// GetRecentMessages returns the last N messages
func (c *ConversationMemory) GetRecentMessages(n int) []orchestrator.ChatMessage {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if n >= len(c.Messages) {
		msgs := make([]orchestrator.ChatMessage, len(c.Messages))
		copy(msgs, c.Messages)
		return msgs
	}

	start := len(c.Messages) - n
	msgs := make([]orchestrator.ChatMessage, n)
	copy(msgs, c.Messages[start:])
	return msgs
}

// Clear removes all messages from memory
func (c *ConversationMemory) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Messages = make([]orchestrator.ChatMessage, 0)
	c.TotalTokens = 0
	c.LastAccessed = time.Now()
}

// trim removes old messages when limits are exceeded
func (c *ConversationMemory) trim() {
	// Trim by message count
	if len(c.Messages) > c.MaxMessages {
		// Keep most recent messages
		keep := c.MaxMessages / 2 // Keep half
		c.Messages = c.Messages[len(c.Messages)-keep:]
		c.recalculateTokens()
	}

	// Trim by token count (rough estimate)
	for c.TotalTokens > c.TokenLimit && len(c.Messages) > 10 {
		// Remove oldest message
		c.Messages = c.Messages[1:]
		c.recalculateTokens()
	}
}

// recalculateTokens recounts total tokens in memory
func (c *ConversationMemory) recalculateTokens() {
	total := 0
	for _, msg := range c.Messages {
		total += estimateTokens(msg.Content)
	}
	c.TotalTokens = total
}

// Save persists the conversation memory to disk
func (m *MemoryStore) Save(agentID string) error {
	mem := m.Get(agentID)
	if mem == nil {
		return fmt.Errorf("no memory for agent: %s", agentID)
	}

	return m.saveMemory(agentID, mem)
}

// saveMemory is an internal helper that saves a memory without calling Get
// This avoids deadlocks when called while holding locks
func (m *MemoryStore) saveMemory(agentID string, mem *ConversationMemory) error {
	mem.mu.RLock()
	defer mem.mu.RUnlock()

	data, err := json.MarshalIndent(mem, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal memory: %w", err)
	}

	path := m.memoryPath(agentID)
	if err := os.WriteFile(path, data, 0640); err != nil {
		return fmt.Errorf("write memory file: %w", err)
	}

	m.logger.Debug("memory saved", "agent", agentID, "messages", len(mem.Messages))
	return nil
}

// SaveAll persists all cached memories to disk
func (m *MemoryStore) SaveAll() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for agentID := range m.cache {
		if err := m.Save(agentID); err != nil {
			m.logger.Error("failed to save memory", "agent", agentID, "error", err)
		}
	}

	return nil
}

// loadFromDisk attempts to load memory from disk
func (m *MemoryStore) loadFromDisk(agentID string) *ConversationMemory {
	path := m.memoryPath(agentID)
	data, err := os.ReadFile(path)
	if err != nil {
		if !os.IsNotExist(err) {
			m.logger.Error("failed to read memory file", "agent", agentID, "error", err)
		}
		return nil
	}

	var mem ConversationMemory
	if err := json.Unmarshal(data, &mem); err != nil {
		m.logger.Error("failed to parse memory file", "agent", agentID, "error", err)
		return nil
	}

	mem.LastAccessed = time.Now()
	m.logger.Info("memory loaded", "agent", agentID, "messages", len(mem.Messages))
	return &mem
}

// memoryPath returns the file path for an agent's memory
func (m *MemoryStore) memoryPath(agentID string) string {
	return filepath.Join(m.dataDir, agentID+".json")
}

// Cleanup removes old memory files that haven't been accessed recently
func (m *MemoryStore) Cleanup(maxAgeHours int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	threshold := time.Now().Add(-time.Duration(maxAgeHours) * time.Hour)

	// Clean up in-memory cache
	for agentID, mem := range m.cache {
		mem.mu.RLock()
		lastAccess := mem.LastAccessed
		mem.mu.RUnlock()

		if lastAccess.Before(threshold) {
			// Save before removing (using internal helper to avoid deadlock)
			if err := m.saveMemory(agentID, mem); err != nil {
				m.logger.Error("failed to save memory during cleanup", "agent", agentID, "error", err)
			}
			delete(m.cache, agentID)
			m.logger.Info("memory evicted from cache", "agent", agentID)
		}
	}

	// Clean up old files on disk
	entries, err := os.ReadDir(m.dataDir)
	if err != nil {
		return fmt.Errorf("read memory dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}

		path := filepath.Join(m.dataDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}

		if info.ModTime().Before(threshold) {
			if err := os.Remove(path); err != nil {
				m.logger.Error("failed to delete old memory file", "path", path, "error", err)
			} else {
				m.logger.Info("old memory file deleted", "path", path)
			}
		}
	}

	return nil
}

// GetStats returns statistics about the memory store
func (m *MemoryStore) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	totalMessages := 0
	totalTokens := 0

	for _, mem := range m.cache {
		mem.mu.RLock()
		totalMessages += len(mem.Messages)
		totalTokens += mem.TotalTokens
		mem.mu.RUnlock()
	}

	return map[string]interface{}{
		"cached_agents":  len(m.cache),
		"total_messages": totalMessages,
		"total_tokens":   totalTokens,
	}
}

// estimateTokens provides a rough token count estimate
// Real token counting would use tiktoken or similar
func estimateTokens(text string) int {
	// Rough estimate: ~4 chars per token for English
	return len(text) / 4
}
