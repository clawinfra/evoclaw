package memory

import (
	"context"
	"os"
	"testing"
	"time"
)

func init() {
	// Use short timeout for tests to avoid hanging on Turso connections
	os.Setenv("MEMORY_TEST_MODE", "1")
}

func TestNewManager(t *testing.T) {
	cfg := DefaultMemoryConfig()
	cfg.AgentID = "test-agent"
	cfg.AgentName = "TestBot"
	cfg.OwnerName = "TestOwner"
	cfg.DatabaseURL = "libsql://test.turso.io"
	cfg.AuthToken = "test-token"

	mgr, err := NewManager(cfg, nil)
	if err != nil {
		t.Fatalf("failed to create manager: %v", err)
	}

	if mgr == nil {
		t.Fatal("manager is nil")
	}

	if mgr.hot == nil {
		t.Error("hot memory not initialized")
	}
	if mgr.warm == nil {
		t.Error("warm memory not initialized")
	}
	if mgr.cold == nil {
		t.Error("cold memory not initialized")
	}
	if mgr.tree == nil {
		t.Error("tree not initialized")
	}
}

func TestManagerValidation(t *testing.T) {
	tests := []struct {
		name    string
		cfg     MemoryConfig
		wantErr bool
	}{
		{
			name: "disabled",
			cfg: MemoryConfig{
				Enabled: false,
			},
			wantErr: true,
		},
		{
			name: "missing agent_id",
			cfg: MemoryConfig{
				Enabled:   true,
				AgentName: "test",
				OwnerName: "owner",
			},
			wantErr: true,
		},
		{
			name: "missing database_url",
			cfg: MemoryConfig{
				Enabled:   true,
				AgentID:   "test",
				AgentName: "test",
				OwnerName: "owner",
			},
			wantErr: true,
		},
		{
			name: "valid config",
			cfg: MemoryConfig{
				Enabled:     true,
				AgentID:     "test",
				AgentName:   "test",
				OwnerName:   "owner",
				DatabaseURL: "libsql://test.turso.io",
				AuthToken:   "token",
			},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := NewManager(tt.cfg, nil)
			if (err != nil) != tt.wantErr {
				t.Errorf("error = %v, wantErr = %v", err, tt.wantErr)
			}
		})
	}
}

func TestProcessConversation(t *testing.T) {
	cfg := DefaultMemoryConfig()
	cfg.AgentID = "test-agent"
	cfg.AgentName = "TestBot"
	cfg.OwnerName = "TestOwner"
	cfg.DatabaseURL = "libsql://test.turso.io"
	cfg.AuthToken = "test-token"

	mgr, _ := NewManager(cfg, nil)

	conv := RawConversation{
		Timestamp: time.Now(),
		Messages: []Message{
			{Role: "user", Content: "We need to work on the EvoClaw memory system"},
			{Role: "agent", Content: "Great! I'll help with that."},
		},
	}

	// Add a tree node first
	mgr.tree.AddNode("projects", "Active projects")
	mgr.tree.AddNode("projects/evoclaw", "EvoClaw development")

	err := mgr.ProcessConversation(context.Background(), conv, "projects/evoclaw", 0.8)
	if err != nil {
		t.Fatalf("process conversation failed: %v", err)
	}

	// Check warm memory
	if mgr.warm.Count() != 1 {
		t.Errorf("warm count: got %d, want 1", mgr.warm.Count())
	}

	// Check tree was updated
	node := mgr.tree.FindNode("projects/evoclaw")
	if node.WarmCount != 1 {
		t.Errorf("tree warm count: got %d, want 1", node.WarmCount)
	}
}

func TestRetrieve(t *testing.T) {
	cfg := DefaultMemoryConfig()
	cfg.AgentID = "test-agent"
	cfg.AgentName = "TestBot"
	cfg.OwnerName = "TestOwner"
	cfg.DatabaseURL = "libsql://test.turso.io"
	cfg.AuthToken = "test-token"

	mgr, _ := NewManager(cfg, nil)

	// Setup tree
	mgr.tree.AddNode("projects", "Projects")
	mgr.tree.AddNode("projects/garden", "Garden work")

	// Add conversation about garden
	conv := RawConversation{
		Timestamp: time.Now(),
		Messages: []Message{
			{Role: "user", Content: "I planted roses in the garden today"},
		},
	}

	mgr.ProcessConversation(context.Background(), conv, "projects/garden", 0.7)

	// Retrieve memories about garden (short timeout to avoid hanging on cold tier)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	results, err := mgr.Retrieve(ctx, "tell me about the garden", 5)
	if err != nil {
		t.Fatalf("retrieve failed: %v", err)
	}

	if len(results) == 0 {
		t.Error("should retrieve garden-related memories")
	}
}

func TestGetHotMemory(t *testing.T) {
	cfg := DefaultMemoryConfig()
	cfg.AgentID = "test-agent"
	cfg.AgentName = "TestBot"
	cfg.OwnerName = "TestOwner"
	cfg.DatabaseURL = "libsql://test.turso.io"
	cfg.AuthToken = "test-token"

	mgr, _ := NewManager(cfg, nil)

	hot := mgr.GetHotMemory()
	if hot == nil {
		t.Fatal("hot memory is nil")
	}

	if hot.Identity.AgentName != "TestBot" {
		t.Errorf("agent name: got %s, want TestBot", hot.Identity.AgentName)
	}
	if hot.Identity.OwnerName != "TestOwner" {
		t.Errorf("owner name: got %s, want TestOwner", hot.Identity.OwnerName)
	}
}

func TestGetTree(t *testing.T) {
	cfg := DefaultMemoryConfig()
	cfg.AgentID = "test-agent"
	cfg.AgentName = "TestBot"
	cfg.OwnerName = "TestOwner"
	cfg.DatabaseURL = "libsql://test.turso.io"
	cfg.AuthToken = "test-token"

	mgr, _ := NewManager(cfg, nil)

	tree := mgr.GetTree()
	if tree == nil {
		t.Fatal("tree is nil")
	}

	if tree.NodeCount != 1 {
		t.Errorf("initial node count: got %d, want 1", tree.NodeCount)
	}
}

func TestGetStats(t *testing.T) {
	cfg := DefaultMemoryConfig()
	cfg.AgentID = "test-agent"
	cfg.AgentName = "TestBot"
	cfg.OwnerName = "TestOwner"
	cfg.DatabaseURL = "libsql://test.turso.io"
	cfg.AuthToken = "test-token"

	mgr, _ := NewManager(cfg, nil)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	stats, err := mgr.GetStats(ctx)
	if err != nil {
		t.Fatalf("get stats failed: %v", err)
	}

	if stats.HotCapacity != cfg.HotMaxBytes {
		t.Errorf("hot capacity: got %d, want %d", stats.HotCapacity, cfg.HotMaxBytes)
	}

	if stats.WarmCapacity != cfg.WarmMaxKB*1024 {
		t.Errorf("warm capacity: got %d, want %d", stats.WarmCapacity, cfg.WarmMaxKB*1024)
	}

	if stats.TreeNodes != 1 {
		t.Errorf("tree nodes: got %d, want 1", stats.TreeNodes)
	}
}

func TestAddLesson(t *testing.T) {
	cfg := DefaultMemoryConfig()
	cfg.AgentID = "test-agent"
	cfg.AgentName = "TestBot"
	cfg.OwnerName = "TestOwner"
	cfg.DatabaseURL = "libsql://test.turso.io"
	cfg.AuthToken = "test-token"

	mgr, _ := NewManager(cfg, nil)

	err := mgr.AddLesson("Always be polite", "communication", 0.9)
	if err != nil {
		t.Fatalf("add lesson failed: %v", err)
	}

	hot := mgr.GetHotMemory()
	if len(hot.CriticalLessons) != 1 {
		t.Errorf("lessons count: got %d, want 1", len(hot.CriticalLessons))
	}

	if hot.CriticalLessons[0].Text != "Always be polite" {
		t.Errorf("lesson text: got %s", hot.CriticalLessons[0].Text)
	}
}

func TestUpdateOwnerProfile(t *testing.T) {
	cfg := DefaultMemoryConfig()
	cfg.AgentID = "test-agent"
	cfg.AgentName = "TestBot"
	cfg.OwnerName = "TestOwner"
	cfg.DatabaseURL = "libsql://test.turso.io"
	cfg.AuthToken = "test-token"

	mgr, _ := NewManager(cfg, nil)

	personality := "friendly and helpful"
	family := []string{"Alice", "Bob"}
	topicsLoved := []string{"gardening", "cooking"}
	topicsAvoid := []string{"politics"}

	err := mgr.UpdateOwnerProfile(&personality, &family, &topicsLoved, &topicsAvoid)
	if err != nil {
		t.Fatalf("update profile failed: %v", err)
	}

	hot := mgr.GetHotMemory()
	if hot.OwnerProfile.Personality != personality {
		t.Errorf("personality: got %s", hot.OwnerProfile.Personality)
	}
	if len(hot.OwnerProfile.Family) != 2 {
		t.Errorf("family count: got %d, want 2", len(hot.OwnerProfile.Family))
	}
}

func TestAddProject(t *testing.T) {
	cfg := DefaultMemoryConfig()
	cfg.AgentID = "test-agent"
	cfg.AgentName = "TestBot"
	cfg.OwnerName = "TestOwner"
	cfg.DatabaseURL = "libsql://test.turso.io"
	cfg.AuthToken = "test-token"

	mgr, _ := NewManager(cfg, nil)

	err := mgr.AddProject("EvoClaw Memory", "Building tiered memory system")
	if err != nil {
		t.Fatalf("add project failed: %v", err)
	}

	hot := mgr.GetHotMemory()
	if len(hot.ActiveContext.CurrentProjects) != 1 {
		t.Errorf("projects count: got %d, want 1", len(hot.ActiveContext.CurrentProjects))
	}

	if hot.ActiveContext.CurrentProjects[0].Name != "EvoClaw Memory" {
		t.Errorf("project name: got %s", hot.ActiveContext.CurrentProjects[0].Name)
	}
}

func TestDefaultMemoryConfig(t *testing.T) {
	cfg := DefaultMemoryConfig()

	if !cfg.Enabled {
		t.Error("should be enabled by default")
	}

	if cfg.TreeMaxNodes != MaxTreeNodes {
		t.Errorf("tree max nodes: got %d, want %d", cfg.TreeMaxNodes, MaxTreeNodes)
	}

	if cfg.WarmMaxKB != MaxWarmSizeKB {
		t.Errorf("warm max kb: got %d, want %d", cfg.WarmMaxKB, MaxWarmSizeKB)
	}

	if cfg.HalfLifeDays != 30.0 {
		t.Errorf("half life: got %.1f, want 30.0", cfg.HalfLifeDays)
	}
}
