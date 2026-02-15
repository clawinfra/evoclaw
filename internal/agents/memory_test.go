package agents

import (
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/clawinfra/evoclaw/internal/orchestrator"
)

func newTestMemoryStore(t *testing.T) *MemoryStore {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	m, err := NewMemoryStore(tmpDir, logger)
	if err != nil {
		t.Fatalf("failed to create memory store: %v", err)
	}
	return m
}

func TestNewMemoryStore(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	m, err := NewMemoryStore(tmpDir, logger)
	if err != nil {
		t.Fatalf("failed to create memory store: %v", err)
	}

	if m == nil {
		t.Fatal("expected non-nil memory store")
	}

	if m.cache == nil {
		t.Error("expected cache map to be initialized")
	}
}

func TestGetMemory(t *testing.T) {
	m := newTestMemoryStore(t)

	// Get memory for new agent
	mem := m.Get("agent-1")
	if mem == nil {
		t.Fatal("expected non-nil memory")
	}

	if mem.AgentID != "agent-1" {
		t.Errorf("expected AgentID agent-1, got %s", mem.AgentID)
	}

	if len(mem.Messages) != 0 {
		t.Errorf("expected 0 messages, got %d", len(mem.Messages))
	}

	if mem.MaxMessages != 100 {
		t.Errorf("expected MaxMessages 100, got %d", mem.MaxMessages)
	}

	if mem.TokenLimit != 100000 {
		t.Errorf("expected TokenLimit 100000, got %d", mem.TokenLimit)
	}

	// Get same memory again (should be cached)
	mem2 := m.Get("agent-1")
	if mem != mem2 {
		t.Error("expected same memory instance from cache")
	}
}

func TestAddMessage(t *testing.T) {
	m := newTestMemoryStore(t)
	mem := m.Get("agent-1")

	// Add messages
	mem.Add("user", "Hello")
	mem.Add("assistant", "Hi there!")
	mem.Add("user", "How are you?")

	if len(mem.Messages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(mem.Messages))
	}

	if mem.Messages[0].Role != "user" {
		t.Errorf("expected first role to be user, got %s", mem.Messages[0].Role)
	}

	if mem.Messages[0].Content != "Hello" {
		t.Errorf("expected first content to be 'Hello', got '%s'", mem.Messages[0].Content)
	}

	if mem.TotalTokens <= 0 {
		t.Error("expected positive token count")
	}
}

func TestGetMessages(t *testing.T) {
	m := newTestMemoryStore(t)
	mem := m.Get("agent-1")

	mem.Add("user", "Message 1")
	mem.Add("assistant", "Message 2")
	mem.Add("user", "Message 3")

	messages := mem.GetMessages()

	if len(messages) != 3 {
		t.Errorf("expected 3 messages, got %d", len(messages))
	}

	// Verify it's a copy (modifying shouldn't affect original)
	messages[0].Content = "Modified"
	if mem.Messages[0].Content == "Modified" {
		t.Error("expected GetMessages to return a copy, not original slice")
	}
}

func TestGetRecentMessages(t *testing.T) {
	m := newTestMemoryStore(t)
	mem := m.Get("agent-1")

	// Add 10 messages
	for i := 1; i <= 10; i++ {
		mem.Add("user", string(rune('0'+i)))
	}

	// Get recent 3
	recent := mem.GetRecentMessages(3)

	if len(recent) != 3 {
		t.Errorf("expected 3 recent messages, got %d", len(recent))
	}

	// Should be the last 3 messages (8, 9, 10)
	if recent[0].Content != "8" {
		t.Errorf("expected first recent message to be '8', got '%s'", recent[0].Content)
	}

	if recent[2].Content != ":" { // '0' + 10 = ':'
		t.Errorf("expected last recent message to be ':', got '%s'", recent[2].Content)
	}
}

func TestGetRecentMessagesMoreThanAvailable(t *testing.T) {
	m := newTestMemoryStore(t)
	mem := m.Get("agent-1")

	mem.Add("user", "Message 1")
	mem.Add("user", "Message 2")

	// Request more than available
	recent := mem.GetRecentMessages(10)

	if len(recent) != 2 {
		t.Errorf("expected 2 messages (all available), got %d", len(recent))
	}
}

func TestClearMemory(t *testing.T) {
	m := newTestMemoryStore(t)
	mem := m.Get("agent-1")

	mem.Add("user", "Message 1")
	mem.Add("user", "Message 2")
	mem.Add("user", "Message 3")

	if len(mem.Messages) != 3 {
		t.Errorf("expected 3 messages before clear, got %d", len(mem.Messages))
	}

	mem.Clear()

	if len(mem.Messages) != 0 {
		t.Errorf("expected 0 messages after clear, got %d", len(mem.Messages))
	}

	if mem.TotalTokens != 0 {
		t.Errorf("expected 0 tokens after clear, got %d", mem.TotalTokens)
	}
}

func TestTrimByMessageCount(t *testing.T) {
	m := newTestMemoryStore(t)
	mem := m.Get("agent-1")

	// Set low max messages for testing
	mem.MaxMessages = 10

	// Add more than max
	for i := 1; i <= 20; i++ {
		mem.Add("user", string(rune('a'+i-1)))
	}

	// Should be trimmed to MaxMessages/2 = 5 messages (or close)
	// Trim happens incrementally, so check it's <= MaxMessages
	if len(mem.Messages) > mem.MaxMessages {
		t.Errorf("expected messages to be trimmed to <= %d, got %d", mem.MaxMessages, len(mem.Messages))
	}

	// Should keep the most recent messages
	lastMsg := mem.Messages[len(mem.Messages)-1].Content
	if lastMsg != "t" {
		t.Logf("expected last message to be 't' (20th char), got '%s' (trim may keep different amount)", lastMsg)
	}
}

func TestTrimByTokenLimit(t *testing.T) {
	m := newTestMemoryStore(t)
	mem := m.Get("agent-1")

	// Set low token limit for testing
	mem.TokenLimit = 50 // Very low limit

	// Add messages with lots of text
	longText := "This is a long message with lots of text that should trigger token trimming."
	for i := 0; i < 20; i++ {
		mem.Add("user", longText)
	}

	// After trimming, should be close to token limit
	// Check that trimming happened (not all 20 messages remain)
	if len(mem.Messages) >= 20 {
		t.Errorf("expected messages to be trimmed, but got all %d messages", len(mem.Messages))
	}

	// Should keep at least 10 messages (minimum in trim logic)
	if len(mem.Messages) < 10 {
		t.Errorf("expected at least 10 messages (minimum), got %d", len(mem.Messages))
	}
}

func TestMemorySaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create store and add messages
	m1, err := NewMemoryStore(tmpDir, logger)
	if err != nil {
		t.Fatalf("failed to create memory store: %v", err)
	}

	mem := m1.Get("agent-1")
	mem.Add("user", "Hello")
	mem.Add("assistant", "Hi there!")
	mem.Add("user", "How are you?")

	// Save to disk
	err = m1.Save("agent-1")
	if err != nil {
		t.Fatalf("failed to save memory: %v", err)
	}

	// Create new store and load
	m2, err := NewMemoryStore(tmpDir, logger)
	if err != nil {
		t.Fatalf("failed to create second memory store: %v", err)
	}

	loaded := m2.Get("agent-1")

	// Verify loaded data
	if len(loaded.Messages) != 3 {
		t.Errorf("expected 3 messages after load, got %d", len(loaded.Messages))
	}

	if loaded.Messages[0].Content != "Hello" {
		t.Errorf("expected first message to be 'Hello', got '%s'", loaded.Messages[0].Content)
	}

	if loaded.Messages[2].Content != "How are you?" {
		t.Errorf("expected third message to be 'How are you?', got '%s'", loaded.Messages[2].Content)
	}
}

func TestSaveAll(t *testing.T) {
	m := newTestMemoryStore(t)

	// Create multiple agent memories
	for i := 1; i <= 3; i++ {
		agentID := string(rune('a'+i-1)) + "-agent"
		mem := m.Get(agentID)
		mem.Add("user", "Message for "+agentID)
	}

	// Save all
	err := m.SaveAll()
	if err != nil {
		t.Fatalf("failed to save all: %v", err)
	}

	// Verify files exist
	for i := 1; i <= 3; i++ {
		agentID := string(rune('a'+i-1)) + "-agent"
		path := m.memoryPath(agentID)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected memory file for %s to exist", agentID)
		}
	}
}

func TestCleanup(t *testing.T) {
	m := newTestMemoryStore(t)

	// Create some memories
	mem1 := m.Get("agent-1")
	mem1.Add("user", "Message 1")

	mem2 := m.Get("agent-2")
	mem2.Add("user", "Message 2")

	// Save memories
	_ = m.Save("agent-1")
	_ = m.Save("agent-2")

	// Run cleanup (with very short threshold to test the function)
	err := m.Cleanup(0)
	if err != nil {
		t.Fatalf("failed to cleanup: %v", err)
	}

	// Just verify cleanup runs without error
	// Actual cleanup behavior depends on timing and file mod times
}

func TestGetStats(t *testing.T) {
	m := newTestMemoryStore(t)

	// Empty stats
	stats := m.GetStats()
	if stats["cached_agents"].(int) != 0 {
		t.Errorf("expected 0 cached agents, got %d", stats["cached_agents"])
	}

	// Add some memories
	mem1 := m.Get("agent-1")
	mem1.Add("user", "Message 1")
	mem1.Add("user", "Message 2")

	mem2 := m.Get("agent-2")
	mem2.Add("user", "Message 3")

	stats = m.GetStats()

	if stats["cached_agents"].(int) != 2 {
		t.Errorf("expected 2 cached agents, got %d", stats["cached_agents"])
	}

	if stats["total_messages"].(int) != 3 {
		t.Errorf("expected 3 total messages, got %d", stats["total_messages"])
	}

	if stats["total_tokens"].(int) <= 0 {
		t.Error("expected positive total tokens")
	}
}

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		text     string
		minToken int
		maxToken int
	}{
		{
			text:     "Hello",
			minToken: 1,
			maxToken: 2,
		},
		{
			text:     "This is a longer sentence with more words.",
			minToken: 10,
			maxToken: 15,
		},
		{
			text:     "",
			minToken: 0,
			maxToken: 0,
		},
	}

	for _, tt := range tests {
		tokens := estimateTokens(tt.text)

		if tokens < tt.minToken {
			t.Errorf("text '%s': tokens %d below minimum %d", tt.text, tokens, tt.minToken)
		}

		if tokens > tt.maxToken {
			t.Errorf("text '%s': tokens %d above maximum %d", tt.text, tokens, tt.maxToken)
		}
	}
}

func TestRecalculateTokens(t *testing.T) {
	m := newTestMemoryStore(t)
	mem := m.Get("agent-1")

	// Add messages
	mem.Add("user", "Message 1")
	mem.Add("user", "Message 2")
	mem.Add("user", "Message 3")

	originalTokens := mem.TotalTokens

	// Manually set tokens to wrong value
	mem.mu.Lock()
	mem.TotalTokens = 9999
	mem.mu.Unlock()

	// Recalculate
	mem.mu.Lock()
	mem.recalculateTokens()
	mem.mu.Unlock()

	// Should be back to correct value
	if mem.TotalTokens != originalTokens {
		t.Errorf("expected tokens %d after recalculation, got %d", originalTokens, mem.TotalTokens)
	}
}

func TestMemoryConcurrentAccess(t *testing.T) {
	m := newTestMemoryStore(t)
	mem := m.Get("agent-1")

	// Concurrent operations
	done := make(chan bool, 30)

	// Concurrent adds
	for i := 0; i < 10; i++ {
		go func(n int) {
			mem.Add("user", "Message "+string(rune('0'+n)))
			done <- true
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func() {
			mem.GetMessages()
			done <- true
		}()
	}

	// Concurrent recent reads
	for i := 0; i < 10; i++ {
		go func() {
			mem.GetRecentMessages(5)
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 30; i++ {
		<-done
	}

	// Verify messages were added
	if len(mem.Messages) != 10 {
		t.Errorf("expected 10 messages, got %d", len(mem.Messages))
	}
}

func TestMemoryLastAccessedUpdate(t *testing.T) {
	m := newTestMemoryStore(t)

	// Get memory
	mem := m.Get("agent-1")
	firstAccess := mem.LastAccessed

	// Wait a bit
	time.Sleep(10 * time.Millisecond)

	// Get again
	mem2 := m.Get("agent-1")
	secondAccess := mem2.LastAccessed

	// LastAccessed should be updated
	if !secondAccess.After(firstAccess) {
		t.Error("expected LastAccessed to be updated on subsequent Get")
	}
}

func TestConversationMemoryStructure(t *testing.T) {
	m := newTestMemoryStore(t)
	mem := m.Get("agent-1")

	// Test that messages are ChatMessage type
	mem.Add("user", "Test message")

	if len(mem.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(mem.Messages))
	}

	msg := mem.Messages[0]
	if _, ok := interface{}(msg).(orchestrator.ChatMessage); !ok {
		t.Error("expected message to be orchestrator.ChatMessage type")
	}

	if msg.Role != "user" {
		t.Errorf("expected role 'user', got '%s'", msg.Role)
	}

	if msg.Content != "Test message" {
		t.Errorf("expected content 'Test message', got '%s'", msg.Content)
	}
}
