package agents

import (
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

func newTestMemoryStore(t *testing.T) *MemoryStore {
	t.Helper()
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	m, err := NewMemoryStore(tmpDir, logger)
	if err != nil {
		t.Fatalf("failed to create memory store: %v", err)
	}
	return m
}

// ---- construction ----

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

func TestGetMemoryDefaults(t *testing.T) {
	m := newTestMemoryStore(t)
	mem := m.Get("agent-1")

	if mem == nil {
		t.Fatal("expected non-nil memory")
	}
	if mem.AgentID != "agent-1" {
		t.Errorf("AgentID: got %q, want %q", mem.AgentID, "agent-1")
	}
	if len(mem.Messages) != 0 {
		t.Errorf("Messages: got %d, want 0", len(mem.Messages))
	}
	if mem.MaxMessages != defaultMaxMessages {
		t.Errorf("MaxMessages: got %d, want %d", mem.MaxMessages, defaultMaxMessages)
	}
	if mem.TokenLimit != defaultTokenLimit {
		t.Errorf("TokenLimit: got %d, want %d", mem.TokenLimit, defaultTokenLimit)
	}
	// Second Get should return the same cached pointer.
	mem2 := m.Get("agent-1")
	if mem != mem2 {
		t.Error("expected same memory instance from cache on second Get")
	}
}

// ---- Add / Get ----

func TestAddMessage(t *testing.T) {
	m := newTestMemoryStore(t)
	mem := m.Get("agent-1")

	mem.Add("user", "Hello")
	mem.Add("assistant", "Hi there!")
	mem.Add("user", "How are you?")

	if len(mem.Messages) != 3 {
		t.Errorf("len(Messages): got %d, want 3", len(mem.Messages))
	}
	if mem.Messages[0].Role != "user" {
		t.Errorf("Messages[0].Role: got %q, want %q", mem.Messages[0].Role, "user")
	}
	if mem.Messages[0].Content != "Hello" {
		t.Errorf("Messages[0].Content: got %q, want %q", mem.Messages[0].Content, "Hello")
	}
	if mem.TotalTokens <= 0 {
		t.Error("expected positive TotalTokens after adding messages")
	}
}

func TestGetMessages_ReturnsCopy(t *testing.T) {
	m := newTestMemoryStore(t)
	mem := m.Get("agent-1")
	mem.Add("user", "Message 1")
	mem.Add("assistant", "Message 2")
	mem.Add("user", "Message 3")

	msgs := mem.GetMessages()
	if len(msgs) != 3 {
		t.Errorf("len(msgs): got %d, want 3", len(msgs))
	}
	// Mutating the returned slice must not affect internal state.
	msgs[0].Content = "mutated"
	if mem.Messages[0].Content == "mutated" {
		t.Error("GetMessages must return a copy, not the internal slice")
	}
}

func TestGetRecentMessages(t *testing.T) {
	m := newTestMemoryStore(t)
	mem := m.Get("agent-1")
	for i := 0; i < 10; i++ {
		mem.Add("user", string(rune('a'+i)))
	}

	recent := mem.GetRecentMessages(3)
	if len(recent) != 3 {
		t.Fatalf("GetRecentMessages(3): got %d, want 3", len(recent))
	}
	if recent[2].Content != "j" {
		t.Errorf("last recent message: got %q, want %q", recent[2].Content, "j")
	}
}

func TestGetRecentMessages_MoreThanAvailable(t *testing.T) {
	m := newTestMemoryStore(t)
	mem := m.Get("agent-1")
	mem.Add("user", "a")
	mem.Add("user", "b")

	recent := mem.GetRecentMessages(10)
	if len(recent) != 2 {
		t.Errorf("GetRecentMessages(10) with 2 stored: got %d, want 2", len(recent))
	}
}

func TestClear(t *testing.T) {
	m := newTestMemoryStore(t)
	mem := m.Get("agent-1")
	for i := 0; i < 5; i++ {
		mem.Add("user", "msg")
	}
	mem.Clear()

	if len(mem.Messages) != 0 {
		t.Errorf("after Clear, len(Messages): got %d, want 0", len(mem.Messages))
	}
	if mem.TotalTokens != 0 {
		t.Errorf("after Clear, TotalTokens: got %d, want 0", mem.TotalTokens)
	}
	if mem.CompactionCount != 0 {
		t.Errorf("after Clear, CompactionCount: got %d, want 0", mem.CompactionCount)
	}
}

// ---- compaction ----

func TestCompact_MessageCountLimit_PreservesHeadAndTail(t *testing.T) {
	m := newTestMemoryStore(t)
	mem := m.Get("agent-1")
	mem.MaxMessages = 10 // small limit for the test

	// Add more messages than the limit.
	total := 30
	for i := 0; i < total; i++ {
		mem.Add("user", "message")
	}

	// Must be within the limit after compaction.
	if len(mem.Messages) > mem.MaxMessages {
		t.Errorf("after compaction, len(Messages) %d > MaxMessages %d", len(mem.Messages), mem.MaxMessages)
	}

	// At least one compaction must have occurred.
	if mem.CompactionCount == 0 {
		t.Error("expected CompactionCount > 0 after exceeding MaxMessages")
	}

	// A compaction marker must be present (role "assistant", contains "Compaction").
	hasMarker := false
	for _, msg := range mem.Messages {
		if msg.Role == "assistant" && strings.Contains(msg.Content, "Compaction") {
			hasMarker = true
			break
		}
	}
	if !hasMarker {
		t.Error("expected a compaction marker message in the history")
	}
}

func TestCompact_HeadMessagesPreserved(t *testing.T) {
	m := newTestMemoryStore(t)
	mem := m.Get("agent-1")
	mem.MaxMessages = 10

	// The first headKeepMessages messages should survive compaction.
	sentinels := make([]string, headKeepMessages)
	for i := 0; i < headKeepMessages; i++ {
		sentinels[i] = "SENTINEL-" + string(rune('A'+i))
		mem.Add("user", sentinels[i])
	}
	// Now flood with filler to trigger compaction.
	for i := 0; i < 30; i++ {
		mem.Add("user", "filler")
	}

	// All sentinel messages must still be present.
	for _, s := range sentinels {
		found := false
		for _, msg := range mem.Messages {
			if msg.Content == s {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("head sentinel %q was lost during compaction", s)
		}
	}
}

func TestCompact_TokenLimit(t *testing.T) {
	m := newTestMemoryStore(t)
	mem := m.Get("agent-1")
	mem.TokenLimit = 50 // extremely tight

	longText := strings.Repeat("word ", 30) // ~10 tokens each
	for i := 0; i < 20; i++ {
		mem.Add("user", longText)
	}

	// Should have been trimmed — not all 20 remain.
	if len(mem.Messages) >= 20 {
		t.Errorf("expected token trimming, but all %d messages remain", len(mem.Messages))
	}
	// Floor must hold.
	if len(mem.Messages) < minMessagesAfterTrim {
		t.Errorf("len(Messages) %d < minMessagesAfterTrim %d", len(mem.Messages), minMessagesAfterTrim)
	}
}

func TestCompact_CompactionCountIncrements(t *testing.T) {
	m := newTestMemoryStore(t)
	mem := m.Get("agent-1")
	mem.MaxMessages = 8

	for i := 0; i < 40; i++ {
		mem.Add("user", "msg")
	}

	if mem.CompactionCount == 0 {
		t.Error("CompactionCount should be > 0 after many messages")
	}
}

// ---- estimateTokens ----

func TestEstimateTokens_Empty(t *testing.T) {
	if got := estimateTokens(""); got != 0 {
		t.Errorf("estimateTokens(%q): got %d, want 0", "", got)
	}
}

func TestEstimateTokens_Short(t *testing.T) {
	// "Hello" = 5 runes → ceil(5/3) = 2
	got := estimateTokens("Hello")
	if got < 1 || got > 3 {
		t.Errorf("estimateTokens(%q): got %d, want 1-3", "Hello", got)
	}
}

func TestEstimateTokens_ConservativeForCode(t *testing.T) {
	// Code is punctuation-dense; our estimate should be at least as large as
	// the naive len/4 estimate so we never under-count.
	code := `func foo(x int) (int, error) { return x * 2, nil }`
	conservative := estimateTokens(code)
	naive := len(code) / 4
	if conservative < naive {
		t.Errorf("conservative estimate %d < naive estimate %d for code", conservative, naive)
	}
}

func TestEstimateTokens_MultiByte(t *testing.T) {
	// CJK: each rune is 3 bytes in UTF-8 but represents roughly 1 token.
	// Our rune-based estimate should be saner than byte/4.
	cjk := "你好世界这是一个测试" // 10 CJK runes = 30 bytes
	byteEstimate := len(cjk) / 4  // 7 — too high if chars are 1 token each
	runeEstimate := estimateTokens(cjk)

	// runeEstimate should be in the range of actual token count (~4-10).
	if runeEstimate > byteEstimate+5 {
		t.Errorf("CJK estimate %d is unreasonably higher than byte estimate %d", runeEstimate, byteEstimate)
	}
	if runeEstimate < 1 {
		t.Errorf("CJK estimate %d: expected at least 1", runeEstimate)
	}
}

func TestEstimateTokens_LongEnglish(t *testing.T) {
	text := strings.Repeat("word ", 100) // 100 words ≈ 100 tokens
	got := estimateTokens(text)
	// Expect 100-200 tokens (our estimate runs conservative).
	if got < 80 || got > 250 {
		t.Errorf("estimateTokens(100 words): got %d, want roughly 80-250", got)
	}
}

// ---- recalculateTokens ----

func TestRecalculateTokens(t *testing.T) {
	m := newTestMemoryStore(t)
	mem := m.Get("agent-1")
	mem.Add("user", "Message 1")
	mem.Add("user", "Message 2")
	mem.Add("user", "Message 3")
	correct := mem.TotalTokens

	mem.mu.Lock()
	mem.TotalTokens = 9999
	mem.recalculateTokens()
	mem.mu.Unlock()

	if mem.TotalTokens != correct {
		t.Errorf("after recalculate: got %d, want %d", mem.TotalTokens, correct)
	}
}

// ---- persistence ----

func TestSaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	m1, _ := NewMemoryStore(tmpDir, logger)
	mem := m1.Get("agent-1")
	mem.Add("user", "Hello")
	mem.Add("assistant", "Hi there!")
	mem.Add("user", "How are you?")
	if err := m1.Save("agent-1"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	m2, _ := NewMemoryStore(tmpDir, logger)
	loaded := m2.Get("agent-1")

	if len(loaded.Messages) != 3 {
		t.Fatalf("loaded messages: got %d, want 3", len(loaded.Messages))
	}
	if loaded.Messages[0].Content != "Hello" {
		t.Errorf("Messages[0]: got %q, want %q", loaded.Messages[0].Content, "Hello")
	}
	if loaded.Messages[2].Content != "How are you?" {
		t.Errorf("Messages[2]: got %q, want %q", loaded.Messages[2].Content, "How are you?")
	}
}

func TestLoadMigration_HighTokenLimit(t *testing.T) {
	// Records written with the old 100k default must be migrated down on load.
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	m, _ := NewMemoryStore(tmpDir, logger)

	mem := m.Get("agent-migration")
	// Simulate the old unsafe defaults.
	mem.mu.Lock()
	mem.TokenLimit = 100_000
	mem.MaxMessages = 100
	mem.mu.Unlock()
	if err := m.Save("agent-migration"); err != nil {
		t.Fatalf("Save: %v", err)
	}

	m2, _ := NewMemoryStore(tmpDir, logger)
	loaded := m2.Get("agent-migration")

	if loaded.TokenLimit > defaultTokenLimit*2 {
		t.Errorf("migration: TokenLimit not clamped; got %d", loaded.TokenLimit)
	}
	if loaded.MaxMessages > defaultMaxMessages*2 {
		t.Errorf("migration: MaxMessages not clamped; got %d", loaded.MaxMessages)
	}
}

func TestSaveAll(t *testing.T) {
	m := newTestMemoryStore(t)
	for i := 0; i < 3; i++ {
		id := string(rune('a'+i)) + "-agent"
		m.Get(id).Add("user", "hello from "+id)
	}
	if err := m.SaveAll(); err != nil {
		t.Fatalf("SaveAll: %v", err)
	}
	for i := 0; i < 3; i++ {
		id := string(rune('a'+i)) + "-agent"
		if _, err := os.Stat(m.memoryPath(id)); os.IsNotExist(err) {
			t.Errorf("expected memory file for %s to exist after SaveAll", id)
		}
	}
}

// ---- stats ----

func TestGetStats(t *testing.T) {
	m := newTestMemoryStore(t)

	stats := m.GetStats()
	if stats["cached_agents"].(int) != 0 {
		t.Errorf("empty store: cached_agents = %d, want 0", stats["cached_agents"])
	}

	m.Get("agent-1").Add("user", "msg1")
	m.Get("agent-1").Add("user", "msg2")
	m.Get("agent-2").Add("user", "msg3")

	stats = m.GetStats()
	if stats["cached_agents"].(int) != 2 {
		t.Errorf("cached_agents: got %d, want 2", stats["cached_agents"])
	}
	if stats["total_messages"].(int) != 3 {
		t.Errorf("total_messages: got %d, want 3", stats["total_messages"])
	}
	if stats["total_tokens"].(int) <= 0 {
		t.Error("total_tokens should be > 0")
	}
	if _, ok := stats["total_compactions"]; !ok {
		t.Error("stats should include total_compactions")
	}
}

// ---- concurrency ----

func TestConcurrentAccess(t *testing.T) {
	m := newTestMemoryStore(t)
	mem := m.Get("agent-1")
	done := make(chan struct{}, 30)

	for i := 0; i < 10; i++ {
		go func(n int) {
			mem.Add("user", "msg")
			done <- struct{}{}
		}(i)
	}
	for i := 0; i < 10; i++ {
		go func() {
			mem.GetMessages()
			done <- struct{}{}
		}()
	}
	for i := 0; i < 10; i++ {
		go func() {
			mem.GetRecentMessages(5)
			done <- struct{}{}
		}()
	}
	for i := 0; i < 30; i++ {
		<-done
	}
	// Data race detector would catch any issues; just verify count.
	if len(mem.Messages) == 0 {
		t.Error("expected at least some messages after concurrent adds")
	}
}

// ---- misc ----

func TestLastAccessedUpdates(t *testing.T) {
	m := newTestMemoryStore(t)
	mem := m.Get("agent-1")
	first := mem.LastAccessed
	time.Sleep(15 * time.Millisecond)
	mem2 := m.Get("agent-1")
	if !mem2.LastAccessed.After(first) {
		t.Error("expected LastAccessed to be updated on subsequent Get")
	}
}

func TestConversationMemoryType(t *testing.T) {
	m := newTestMemoryStore(t)
	mem := m.Get("agent-1")
	mem.Add("user", "Test message")

	if len(mem.Messages) != 1 {
		t.Fatalf("len(Messages): got %d, want 1", len(mem.Messages))
	}
	msg := mem.Messages[0]
	if msg.Role != "user" {
		t.Errorf("Role: got %q, want %q", msg.Role, "user")
	}
	if msg.Content != "Test message" {
		t.Errorf("Content: got %q, want %q", msg.Content, "Test message")
	}
}
