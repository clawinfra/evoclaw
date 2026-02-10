package agents

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/clawinfra/evoclaw/internal/config"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func newTestRegistry(t *testing.T) *Registry {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	r, err := NewRegistry(tmpDir, logger)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}
	return r
}

func TestNewRegistry(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))

	r, err := NewRegistry(tmpDir, logger)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	if r == nil {
		t.Fatal("expected non-nil registry")
	}

	if r.agents == nil {
		t.Error("expected agents map to be initialized")
	}
}

func TestCreateAgent(t *testing.T) {
	r := newTestRegistry(t)

	def := config.AgentDef{
		ID:    "test-agent-1",
		Name:  "Test Agent",
		Type:  "orchestrator",
		Model: "test/model",
		Skills: []string{"coding", "research"},
	}

	agent, err := r.Create(def)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	if agent.ID != "test-agent-1" {
		t.Errorf("expected ID test-agent-1, got %s", agent.ID)
	}

	if agent.Status != "idle" {
		t.Errorf("expected status idle, got %s", agent.Status)
	}

	if agent.MessageCount != 0 {
		t.Errorf("expected message count 0, got %d", agent.MessageCount)
	}

	if agent.Metrics.Custom == nil {
		t.Error("expected custom metrics map to be initialized")
	}
}

func TestCreateDuplicateAgent(t *testing.T) {
	r := newTestRegistry(t)

	def := config.AgentDef{
		ID:   "test-agent-1",
		Name: "Test Agent",
		Type: "orchestrator",
	}

	_, err := r.Create(def)
	if err != nil {
		t.Fatalf("failed to create first agent: %v", err)
	}

	// Try to create duplicate
	_, err = r.Create(def)
	if err == nil {
		t.Error("expected error when creating duplicate agent")
	}
}

func TestGetAgent(t *testing.T) {
	r := newTestRegistry(t)

	def := config.AgentDef{
		ID:   "test-agent-1",
		Name: "Test Agent",
	}

	_, err := r.Create(def)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Get existing agent
	agent, err := r.Get("test-agent-1")
	if err != nil {
		t.Fatalf("failed to get agent: %v", err)
	}

	if agent.ID != "test-agent-1" {
		t.Errorf("expected ID test-agent-1, got %s", agent.ID)
	}

	// Get nonexistent agent
	_, err = r.Get("nonexistent")
	if err == nil {
		t.Error("expected error when getting nonexistent agent")
	}
}

func TestListAgents(t *testing.T) {
	r := newTestRegistry(t)

	// Empty list
	agents := r.List()
	if len(agents) != 0 {
		t.Errorf("expected 0 agents, got %d", len(agents))
	}

	// Create some agents
	for i := 1; i <= 3; i++ {
		def := config.AgentDef{
			ID:   string(rune(i)) + "-agent",
			Name: "Agent " + string(rune(i)),
		}
		r.Create(def)
	}

	agents = r.List()
	if len(agents) != 3 {
		t.Errorf("expected 3 agents, got %d", len(agents))
	}
}

func TestUpdateAgent(t *testing.T) {
	r := newTestRegistry(t)

	def := config.AgentDef{
		ID:    "test-agent-1",
		Name:  "Original Name",
		Model: "original/model",
	}

	_, err := r.Create(def)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Update agent
	updatedDef := config.AgentDef{
		ID:    "test-agent-1",
		Name:  "Updated Name",
		Model: "updated/model",
	}

	err = r.Update("test-agent-1", updatedDef)
	if err != nil {
		t.Fatalf("failed to update agent: %v", err)
	}

	// Verify update
	agent, _ := r.Get("test-agent-1")
	if agent.Def.Name != "Updated Name" {
		t.Errorf("expected name 'Updated Name', got '%s'", agent.Def.Name)
	}

	if agent.Def.Model != "updated/model" {
		t.Errorf("expected model updated/model, got %s", agent.Def.Model)
	}
}

func TestUpdateNonexistentAgent(t *testing.T) {
	r := newTestRegistry(t)

	def := config.AgentDef{
		ID:   "nonexistent",
		Name: "Test",
	}

	err := r.Update("nonexistent", def)
	if err == nil {
		t.Error("expected error when updating nonexistent agent")
	}
}

func TestDeleteAgent(t *testing.T) {
	r := newTestRegistry(t)

	def := config.AgentDef{
		ID:   "test-agent-1",
		Name: "Test Agent",
	}

	_, err := r.Create(def)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Delete agent
	err = r.Delete("test-agent-1")
	if err != nil {
		t.Fatalf("failed to delete agent: %v", err)
	}

	// Verify deletion
	_, err = r.Get("test-agent-1")
	if err == nil {
		t.Error("expected error when getting deleted agent")
	}

	// Verify file was deleted
	path := r.agentPath("test-agent-1")
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("expected agent file to be deleted")
	}
}

func TestDeleteNonexistentAgent(t *testing.T) {
	r := newTestRegistry(t)

	err := r.Delete("nonexistent")
	if err == nil {
		t.Error("expected error when deleting nonexistent agent")
	}
}

func TestUpdateStatus(t *testing.T) {
	r := newTestRegistry(t)

	def := config.AgentDef{
		ID:   "test-agent-1",
		Name: "Test Agent",
	}

	_, err := r.Create(def)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Update status
	err = r.UpdateStatus("test-agent-1", "running")
	if err != nil {
		t.Fatalf("failed to update status: %v", err)
	}

	agent, _ := r.Get("test-agent-1")
	if agent.Status != "running" {
		t.Errorf("expected status running, got %s", agent.Status)
	}

	// LastActive should be updated
	if agent.LastActive.IsZero() {
		t.Error("expected LastActive to be set")
	}
}

func TestRecordHeartbeat(t *testing.T) {
	r := newTestRegistry(t)

	def := config.AgentDef{
		ID:   "test-agent-1",
		Name: "Test Agent",
	}

	agent, err := r.Create(def)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Initial heartbeat should be zero
	if !agent.LastHeartbeat.IsZero() {
		t.Error("expected initial heartbeat to be zero")
	}

	// Record heartbeat
	err = r.RecordHeartbeat("test-agent-1")
	if err != nil {
		t.Fatalf("failed to record heartbeat: %v", err)
	}

	agent, _ = r.Get("test-agent-1")
	if agent.LastHeartbeat.IsZero() {
		t.Error("expected heartbeat to be recorded")
	}
}

func TestRecordMessage(t *testing.T) {
	r := newTestRegistry(t)

	def := config.AgentDef{
		ID:   "test-agent-1",
		Name: "Test Agent",
	}

	_, err := r.Create(def)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Record multiple messages
	for i := 0; i < 5; i++ {
		err = r.RecordMessage("test-agent-1")
		if err != nil {
			t.Fatalf("failed to record message: %v", err)
		}
	}

	agent, _ := r.Get("test-agent-1")
	if agent.MessageCount != 5 {
		t.Errorf("expected message count 5, got %d", agent.MessageCount)
	}

	if agent.LastActive.IsZero() {
		t.Error("expected LastActive to be set")
	}
}

func TestRecordError(t *testing.T) {
	r := newTestRegistry(t)

	def := config.AgentDef{
		ID:   "test-agent-1",
		Name: "Test Agent",
	}

	_, err := r.Create(def)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Record multiple errors
	for i := 0; i < 3; i++ {
		err = r.RecordError("test-agent-1")
		if err != nil {
			t.Fatalf("failed to record error: %v", err)
		}
	}

	agent, _ := r.Get("test-agent-1")
	if agent.ErrorCount != 3 {
		t.Errorf("expected error count 3, got %d", agent.ErrorCount)
	}

	if agent.Metrics.FailedActions != 3 {
		t.Errorf("expected 3 failed actions, got %d", agent.Metrics.FailedActions)
	}
}

func TestUpdateMetrics(t *testing.T) {
	r := newTestRegistry(t)

	def := config.AgentDef{
		ID:   "test-agent-1",
		Name: "Test Agent",
	}

	_, err := r.Create(def)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Record successful action
	err = r.UpdateMetrics("test-agent-1", 1000, 0.05, 500, true)
	if err != nil {
		t.Fatalf("failed to update metrics: %v", err)
	}

	agent, _ := r.Get("test-agent-1")
	if agent.Metrics.TotalActions != 1 {
		t.Errorf("expected 1 total action, got %d", agent.Metrics.TotalActions)
	}

	if agent.Metrics.SuccessfulActions != 1 {
		t.Errorf("expected 1 successful action, got %d", agent.Metrics.SuccessfulActions)
	}

	if agent.Metrics.TokensUsed != 1000 {
		t.Errorf("expected 1000 tokens used, got %d", agent.Metrics.TokensUsed)
	}

	if agent.Metrics.CostUSD != 0.05 {
		t.Errorf("expected cost 0.05, got %f", agent.Metrics.CostUSD)
	}

	if agent.Metrics.AvgResponseMs != 500 {
		t.Errorf("expected avg response 500ms, got %f", agent.Metrics.AvgResponseMs)
	}

	// Record failed action
	err = r.UpdateMetrics("test-agent-1", 500, 0.02, 300, false)
	if err != nil {
		t.Fatalf("failed to update metrics: %v", err)
	}

	agent, _ = r.Get("test-agent-1")
	if agent.Metrics.TotalActions != 2 {
		t.Errorf("expected 2 total actions, got %d", agent.Metrics.TotalActions)
	}

	if agent.Metrics.FailedActions != 1 {
		t.Errorf("expected 1 failed action, got %d", agent.Metrics.FailedActions)
	}

	if agent.Metrics.TokensUsed != 1500 {
		t.Errorf("expected 1500 tokens used, got %d", agent.Metrics.TokensUsed)
	}

	if agent.Metrics.CostUSD != 0.07 {
		t.Errorf("expected cost 0.07, got %f", agent.Metrics.CostUSD)
	}

	// Average should be updated
	expectedAvg := (500.0 + 300.0) / 2.0
	if agent.Metrics.AvgResponseMs != expectedAvg {
		t.Errorf("expected avg response %f, got %f", expectedAvg, agent.Metrics.AvgResponseMs)
	}
}

func TestCheckHealth(t *testing.T) {
	r := newTestRegistry(t)

	// Create agents with different heartbeat times
	for i := 1; i <= 3; i++ {
		def := config.AgentDef{
			ID:   string(rune('a'+i-1)) + "-agent",
			Name: "Agent " + string(rune('0'+i)),
		}
		r.Create(def)
	}

	// Record heartbeats at different times
	r.RecordHeartbeat("a-agent")
	time.Sleep(10 * time.Millisecond)
	r.RecordHeartbeat("b-agent")
	// Don't record heartbeat for c-agent

	// Simulate old heartbeat for a-agent
	agent, _ := r.Get("a-agent")
	agent.mu.Lock()
	agent.LastHeartbeat = time.Now().Add(-120 * time.Second)
	agent.mu.Unlock()

	// Check health with 60 second timeout
	unhealthy := r.CheckHealth(60)

	// a-agent should be unhealthy (old heartbeat)
	// b-agent should be healthy (recent heartbeat)
	// c-agent has zero heartbeat, so won't be flagged
	if len(unhealthy) != 1 {
		t.Errorf("expected 1 unhealthy agent, got %d: %v", len(unhealthy), unhealthy)
	}

	if len(unhealthy) > 0 && unhealthy[0] != "a-agent" {
		t.Errorf("expected a-agent to be unhealthy, got %s", unhealthy[0])
	}
}

func TestRegistrySaveAndLoad(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))

	// Create registry and add agents
	r1, err := NewRegistry(tmpDir, logger)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}

	def1 := config.AgentDef{
		ID:    "agent-1",
		Name:  "Test Agent 1",
		Type:  "orchestrator",
		Model: "test/model",
	}

	def2 := config.AgentDef{
		ID:    "agent-2",
		Name:  "Test Agent 2",
		Type:  "trader",
		Model: "test/model2",
	}

	r1.Create(def1)
	r1.Create(def2)

	// Record some activity
	r1.RecordMessage("agent-1")
	r1.UpdateMetrics("agent-1", 1000, 0.05, 500, true)

	// Save all
	err = r1.SaveAll()
	if err != nil {
		t.Fatalf("failed to save all: %v", err)
	}

	// Create new registry and load
	r2, err := NewRegistry(tmpDir, logger)
	if err != nil {
		t.Fatalf("failed to create second registry: %v", err)
	}

	err = r2.Load()
	if err != nil {
		t.Fatalf("failed to load: %v", err)
	}

	// Verify agents were loaded
	agents := r2.List()
	if len(agents) != 2 {
		t.Errorf("expected 2 agents after load, got %d", len(agents))
	}

	// Verify agent-1 details
	agent1, err := r2.Get("agent-1")
	if err != nil {
		t.Fatalf("failed to get agent-1: %v", err)
	}

	if agent1.Def.Name != "Test Agent 1" {
		t.Errorf("expected name 'Test Agent 1', got '%s'", agent1.Def.Name)
	}

	if agent1.MessageCount != 1 {
		t.Errorf("expected message count 1, got %d", agent1.MessageCount)
	}

	if agent1.Metrics.TokensUsed != 1000 {
		t.Errorf("expected 1000 tokens used, got %d", agent1.Metrics.TokensUsed)
	}
}

func TestGetSnapshot(t *testing.T) {
	r := newTestRegistry(t)

	def := config.AgentDef{
		ID:   "test-agent-1",
		Name: "Test Agent",
	}

	agent, err := r.Create(def)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Update agent state
	r.RecordMessage("test-agent-1")
	r.UpdateMetrics("test-agent-1", 1000, 0.05, 500, true)

	// Get snapshot
	snapshot := agent.GetSnapshot()

	// Verify snapshot has correct data
	if snapshot.ID != "test-agent-1" {
		t.Errorf("expected ID test-agent-1, got %s", snapshot.ID)
	}

	if snapshot.MessageCount != 1 {
		t.Errorf("expected message count 1, got %d", snapshot.MessageCount)
	}

	if snapshot.Metrics.TokensUsed != 1000 {
		t.Errorf("expected 1000 tokens used, got %d", snapshot.Metrics.TokensUsed)
	}
}

func TestRegistryConcurrentAccess(t *testing.T) {
	r := newTestRegistry(t)

	def := config.AgentDef{
		ID:   "test-agent-1",
		Name: "Test Agent",
	}

	_, err := r.Create(def)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}

	// Concurrent operations
	done := make(chan bool, 30)

	// Concurrent message recording
	for i := 0; i < 10; i++ {
		go func() {
			r.RecordMessage("test-agent-1")
			done <- true
		}()
	}

	// Concurrent metric updates
	for i := 0; i < 10; i++ {
		go func() {
			r.UpdateMetrics("test-agent-1", 100, 0.01, 100, true)
			done <- true
		}()
	}

	// Concurrent reads
	for i := 0; i < 10; i++ {
		go func() {
			r.Get("test-agent-1")
			done <- true
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 30; i++ {
		<-done
	}

	// Verify counts
	agent, _ := r.Get("test-agent-1")
	if agent.MessageCount != 10 {
		t.Errorf("expected message count 10, got %d", agent.MessageCount)
	}

	if agent.Metrics.TotalActions != 10 {
		t.Errorf("expected 10 total actions, got %d", agent.Metrics.TotalActions)
	}
}

func TestLoadWithInvalidJSON(t *testing.T) {
	tmpDir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
	
	// Write an invalid JSON file
	invalidPath := tmpDir + "/invalid.json"
	_ = os.WriteFile(invalidPath, []byte("not valid json"), 0644)
	
	r, err := NewRegistry(tmpDir, logger)
	if err != nil {
		t.Fatalf("failed to create registry: %v", err)
	}
	
	// Should not error, but should log and skip the invalid file
	// Registry should have no agents loaded
	agents := r.List()
	if len(agents) != 0 {
		t.Errorf("expected 0 agents after loading invalid JSON, got %d", len(agents))
	}
}

func TestSaveError(t *testing.T) {
	r := newTestRegistry(t)
	
	def := config.AgentDef{ID: "test-agent", Name: "Test"}
	agent, err := r.Create(def)
	if err != nil {
		t.Fatalf("failed to create agent: %v", err)
	}
	
	// Make the data directory read-only to force a save error
	_ = os.Chmod(r.dataDir, 0444)
	defer os.Chmod(r.dataDir, 0755)
	
	// SaveAll should continue despite errors
	err = r.SaveAll()
	// It returns nil even if individual saves fail (logs errors instead)
	if err != nil {
		t.Errorf("SaveAll should not return error, got: %v", err)
	}
	
	_ = agent
}

func TestRegistry_LoadErrorHandling(t *testing.T) {
	tmpDir := t.TempDir()
	logger := testLogger()

	registry, _ := NewRegistry(tmpDir, logger)

	// Create an agent
	def := config.AgentDef{
		ID:   "test-agent",
		Name: "Test",
		Type: "monitor",
		Model: "test",
	}
	registry.Create(def)

	// Save it
	registry.SaveAll()

	// Create corrupted file
	agentPath := filepath.Join(tmpDir, "agents", "test-agent.json")
	_ = os.WriteFile(agentPath, []byte("invalid json{{{"), 0644)

	// Try to load - should handle error gracefully
	newRegistry, _ := NewRegistry(tmpDir, logger)
	err := newRegistry.Load()
	// Should log error but not fail completely
	if err != nil {
		t.Logf("Load with corrupted file returned error (acceptable): %v", err)
	}
}

func TestRegistry_SaveErrorHandling(t *testing.T) {
	tmpDir := t.TempDir()
	logger := testLogger()

	registry, _ := NewRegistry(tmpDir, logger)

	def := config.AgentDef{
		ID:   "test-agent",
		Name: "Test",
		Type: "monitor",
		Model: "test",
	}
	registry.Create(def)

	// Make directory read-only to trigger save error
	agentDir := filepath.Join(tmpDir, "agents")
	_ = os.MkdirAll(agentDir, 0755)
	_ = os.Chmod(agentDir, 0444)

	err := registry.SaveAll()
	if err != nil {
		t.Logf("SaveAll with read-only dir returned error (expected): %v", err)
	}

	// Restore permissions
	_ = os.Chmod(agentDir, 0755)
}

func TestMemoryStore_LoadErrorHandling(t *testing.T) {
	tmpDir := t.TempDir()
	logger := testLogger()

	memory, _ := NewMemoryStore(tmpDir, logger)

	// Create memory and save
	mem := memory.Get("test-agent")
	mem.Add("user", "Hello")
	memory.Save("test-agent")

	// Corrupt the file
	memPath := filepath.Join(tmpDir, "memory", "test-agent.json")
	_ = os.WriteFile(memPath, []byte("invalid json{{{"), 0644)

	// Create new memory store and try to load
	newMemory, _ := NewMemoryStore(tmpDir, logger)
	newMem := newMemory.Get("test-agent")
	// Should handle corrupted file gracefully
	if len(newMem.GetMessages()) > 0 {
		t.Log("Memory loaded despite corruption")
	}
}

func TestRegistry_NewRegistryErrorHandling(t *testing.T) {
	// Try to create registry in invalid location
	logger := testLogger()

	_, err := NewRegistry("/proc/invalid-location", logger)
	if err == nil {
		t.Error("expected error when creating registry in invalid location")
	}
}

func TestMemory_SaveErrorHandling(t *testing.T) {
	tmpDir := t.TempDir()
	logger := testLogger()

	memory, _ := NewMemoryStore(tmpDir, logger)
	mem := memory.Get("test-agent")
	mem.Add("user", "Hello")

	// Make directory read-only
	memDir := filepath.Join(tmpDir, "memory")
	_ = os.MkdirAll(memDir, 0755)
	_ = os.Chmod(memDir, 0444)

	err := memory.Save("test-agent")
	if err != nil {
		t.Logf("Save with read-only dir returned error (expected): %v", err)
	}

	// Restore permissions
	_ = os.Chmod(memDir, 0755)
}
