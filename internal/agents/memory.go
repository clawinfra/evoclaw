package agents

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/clawinfra/evoclaw/internal/orchestrator"
)

const (
	// defaultMaxMessages is the maximum number of messages to retain before compaction.
	// Keeping this low prevents unbounded context growth when history is wired into LLM calls.
	defaultMaxMessages = 50

	// defaultTokenLimit is the soft token ceiling for conversation history.
	// Set conservatively to leave room for system prompt (~10-20k), tools (~5-10k),
	// and model response (~8k) within typical 128k context windows.
	defaultTokenLimit = 32_000

	// headKeepMessages is the number of earliest messages preserved during compaction.
	// These establish the conversation frame / initial instructions.
	headKeepMessages = 5

	// minMessagesAfterTrim is the floor to avoid trimming into a degenerate state.
	minMessagesAfterTrim = 10
)

// MemoryStore manages conversation memory for agents.
type MemoryStore struct {
	dataDir string
	logger  *slog.Logger
	mu      sync.RWMutex
	cache   map[string]*ConversationMemory
}

// ConversationMemory stores chat history for an agent with bounded growth.
type ConversationMemory struct {
	AgentID        string                     `json:"agent_id"`
	Messages       []orchestrator.ChatMessage `json:"messages"`
	MaxMessages    int                        `json:"max_messages"`
	TotalTokens    int                        `json:"total_tokens"`
	TokenLimit     int                        `json:"token_limit"`
	CompactionCount int                       `json:"compaction_count"`
	LastAccessed   time.Time                  `json:"last_accessed"`
	mu             sync.RWMutex
}

// NewMemoryStore creates a new memory store.
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

// Get retrieves or creates conversation memory for an agent.
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

	// Try to load from disk.
	mem = m.loadFromDisk(agentID)
	if mem != nil {
		m.mu.Lock()
		m.cache[agentID] = mem
		m.mu.Unlock()
		return mem
	}

	// Create new memory with safe defaults.
	mem = &ConversationMemory{
		AgentID:      agentID,
		Messages:     make([]orchestrator.ChatMessage, 0),
		MaxMessages:  defaultMaxMessages,
		TokenLimit:   defaultTokenLimit,
		LastAccessed: time.Now(),
	}

	m.mu.Lock()
	m.cache[agentID] = mem
	m.mu.Unlock()

	m.logger.Info("conversation memory created", "agent", agentID)
	return mem
}

// Add appends a message to the conversation history and enforces limits.
func (c *ConversationMemory) Add(role, content string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Messages = append(c.Messages, orchestrator.ChatMessage{
		Role:    role,
		Content: content,
	})
	c.TotalTokens += estimateTokens(content)
	c.compact()
	c.LastAccessed = time.Now()
}

// GetMessages returns a copy of all messages.
func (c *ConversationMemory) GetMessages() []orchestrator.ChatMessage {
	c.mu.RLock()
	defer c.mu.RUnlock()

	msgs := make([]orchestrator.ChatMessage, len(c.Messages))
	copy(msgs, c.Messages)
	return msgs
}

// GetRecentMessages returns the last n messages.
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

// Clear wipes the conversation history.
func (c *ConversationMemory) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.Messages = make([]orchestrator.ChatMessage, 0)
	c.TotalTokens = 0
	c.CompactionCount = 0
	c.LastAccessed = time.Now()
}

// compact enforces MaxMessages and TokenLimit using a head+tail strategy.
//
// Instead of dropping oldest messages wholesale (lossy), we preserve the first
// headKeepMessages turns (conversation framing) and the most recent turns, and
// insert a synthetic marker so the model knows context was compacted.
//
// Must be called with c.mu held.
func (c *ConversationMemory) compact() {
	changed := false

	// --- Phase 1: message count limit ---
	if len(c.Messages) > c.MaxMessages {
		// Slots: headKeepMessages + 1 marker + tailKeep = MaxMessages.
		// tailKeep is at least 1 so we always keep something after the marker.
		tailKeep := c.MaxMessages - headKeepMessages - 1
		if tailKeep < 1 {
			tailKeep = 1
		}
		// Clamp head so the math stays valid when MaxMessages is very small.
		head := headKeepMessages
		if head+1+tailKeep > c.MaxMessages {
			head = c.MaxMessages - tailKeep - 1
			if head < 0 {
				head = 0
			}
		}

		total := head + 1 + tailKeep
		if len(c.Messages) > total {
			dropped := len(c.Messages) - total
			headMsgs := c.Messages[:head]
			tailMsgs := c.Messages[len(c.Messages)-tailKeep:]

			marker := orchestrator.ChatMessage{
				Role: "assistant",
				Content: fmt.Sprintf(
					"[Compaction #%d: %d earlier messages summarised. Conversation continues from most recent %d messages.]",
					c.CompactionCount+1, dropped, tailKeep,
				),
			}

			compacted := make([]orchestrator.ChatMessage, 0, total)
			compacted = append(compacted, headMsgs...)
			compacted = append(compacted, marker)
			compacted = append(compacted, tailMsgs...)
			c.Messages = compacted
			c.CompactionCount++
			changed = true
		}
	}

	// --- Phase 2: token limit ---
	// Remove messages after the head one at a time until we're under budget.
	// Keep at least minMessagesAfterTrim total to avoid degenerating to nothing.
	for c.TotalTokens > c.TokenLimit && len(c.Messages) > minMessagesAfterTrim {
		if len(c.Messages) > headKeepMessages {
			// Drop the oldest non-head message.
			c.Messages = append(c.Messages[:headKeepMessages], c.Messages[headKeepMessages+1:]...)
		} else {
			c.Messages = c.Messages[1:]
		}
		changed = true
	}

	if changed {
		c.recalculateTokens()
	}
}

// recalculateTokens recomputes TotalTokens from scratch.
// Must be called with c.mu held.
func (c *ConversationMemory) recalculateTokens() {
	total := 0
	for _, msg := range c.Messages {
		total += estimateTokens(msg.Content)
	}
	c.TotalTokens = total
}

// Save persists the conversation memory to disk.
func (m *MemoryStore) Save(agentID string) error {
	mem := m.Get(agentID)
	if mem == nil {
		return fmt.Errorf("no memory for agent: %s", agentID)
	}
	return m.saveMemory(agentID, mem)
}

// saveMemory is an internal helper that avoids the Get→lock deadlock path.
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

// SaveAll flushes all cached memories to disk.
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

// loadFromDisk reads and deserialises a memory file.
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

	// Migrate old records that used the unsafe 100k default.
	if mem.TokenLimit <= 0 || mem.TokenLimit > defaultTokenLimit*2 {
		mem.TokenLimit = defaultTokenLimit
	}
	if mem.MaxMessages <= 0 || mem.MaxMessages > defaultMaxMessages*2 {
		mem.MaxMessages = defaultMaxMessages
	}

	mem.LastAccessed = time.Now()
	m.logger.Info("memory loaded", "agent", agentID, "messages", len(mem.Messages))
	return &mem
}

// memoryPath returns the file path for an agent's memory.
func (m *MemoryStore) memoryPath(agentID string) string {
	return filepath.Join(m.dataDir, agentID+".json")
}

// Cleanup evicts unused in-memory entries and removes stale files.
func (m *MemoryStore) Cleanup(maxAgeHours int) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	threshold := time.Now().Add(-time.Duration(maxAgeHours) * time.Hour)

	for agentID, mem := range m.cache {
		mem.mu.RLock()
		lastAccess := mem.LastAccessed
		mem.mu.RUnlock()

		if lastAccess.Before(threshold) {
			if err := m.saveMemory(agentID, mem); err != nil {
				m.logger.Error("failed to save memory during cleanup", "agent", agentID, "error", err)
			}
			delete(m.cache, agentID)
			m.logger.Info("memory evicted from cache", "agent", agentID)
		}
	}

	entries, err := os.ReadDir(m.dataDir)
	if err != nil {
		return fmt.Errorf("read memory dir: %w", err)
	}

	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if info.ModTime().Before(threshold) {
			path := filepath.Join(m.dataDir, entry.Name())
			if err := os.Remove(path); err != nil {
				m.logger.Error("failed to delete old memory file", "path", path, "error", err)
			} else {
				m.logger.Info("old memory file deleted", "path", path)
			}
		}
	}

	return nil
}

// GetStats returns aggregate statistics for the memory store.
func (m *MemoryStore) GetStats() map[string]interface{} {
	m.mu.RLock()
	defer m.mu.RUnlock()

	totalMessages := 0
	totalTokens := 0
	totalCompactions := 0

	for _, mem := range m.cache {
		mem.mu.RLock()
		totalMessages += len(mem.Messages)
		totalTokens += mem.TotalTokens
		totalCompactions += mem.CompactionCount
		mem.mu.RUnlock()
	}

	return map[string]interface{}{
		"cached_agents":    len(m.cache),
		"total_messages":   totalMessages,
		"total_tokens":     totalTokens,
		"total_compactions": totalCompactions,
	}
}

// estimateTokens returns a conservative token estimate for a string.
//
// Uses ceil(runes/3) rather than len(bytes)/4 to handle:
//   - Code and JSON (punctuation-dense, higher token/char ratio)
//   - Multilingual content (CJK characters count as 1-2 tokens each)
//   - Unicode: rune count avoids inflating multi-byte UTF-8 sequences
//
// A real implementation would use tiktoken; this is a safe fallback.
func estimateTokens(text string) int {
	runes := utf8.RuneCountInString(text)
	// Conservative: ceil(runes / 3), minimum 1 for non-empty strings.
	tokens := (runes + 2) / 3
	if tokens == 0 && runes > 0 {
		tokens = 1
	}
	return tokens
}
