package governance

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewVFM(t *testing.T) {
	tmpDir := t.TempDir()
	logger := nil

	vfm, err := NewVFM(tmpDir, logger)
	if err != nil {
		t.Fatalf("NewVFM failed: %v", err)
	}

	if vfm == nil {
		t.Fatal("expected non-nil VFM")
	}

	// Check directory was created
	vfmDir := filepath.Join(tmpDir, "vfm")
	if _, err := os.Stat(vfmDir); os.IsNotExist(err) {
		t.Error("vfm directory not created")
	}
}

func TestVFMTrackCost(t *testing.T) {
	tmpDir := t.TempDir()
	vfm, _ := NewVFM(tmpDir, nil)
	agentID := "test-agent"

	err := vfm.TrackCost(agentID, "monitoring", "glm-4.7", 1000, 0.01)
	if err != nil {
		t.Fatalf("TrackCost failed: %v", err)
	}

	// Verify stats
	stats, err := vfm.GetStats(agentID)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.TotalTasks != 1 {
		t.Errorf("expected 1 task, got %d", stats.TotalTasks)
	}

	if stats.TotalTokens != 1000 {
		t.Errorf("expected 1000 tokens, got %d", stats.TotalTokens)
	}

	if stats.TotalCostUSD != 0.01 {
		t.Errorf("expected 0.01 cost, got %.4f", stats.TotalCostUSD)
	}
}

func TestVFMTrackCostWithValue(t *testing.T) {
	tmpDir := t.TempDir()
	vfm, _ := NewVFM(tmpDir, nil)
	agentID := "test-agent"

	err := vfm.TrackCostWithValue(agentID, "debugging", "claude-sonnet", 5000, 0.15, 0.8)
	if err != nil {
		t.Fatalf("TrackCostWithValue failed: %v", err)
	}

	// Verify stats
	stats, err := vfm.GetStats(agentID)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.TotalValue != 0.8 {
		t.Errorf("expected 0.8 total value, got %.2f", stats.TotalValue)
	}

	if stats.AverageValue != 0.8 {
		t.Errorf("expected 0.8 average value, got %.2f", stats.AverageValue)
	}
}

func TestVFMStats(t *testing.T) {
	tmpDir := t.TempDir()
	vfm, _ := NewVFM(tmpDir, nil)
	agentID := "test-agent-stats"

	// Track multiple tasks
	_ = vfm.TrackCostWithValue(agentID, "task1", "model1", 1000, 0.01, 1.0)
	_ = vfm.TrackCostWithValue(agentID, "task2", "model2", 2000, 0.02, 0.5)
	_ = vfm.TrackCostWithValue(agentID, "task3", "model3", 1500, 0.015, 0.9)

	// Get stats
	stats, err := vfm.GetStats(agentID)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.TotalTasks != 3 {
		t.Errorf("expected 3 tasks, got %d", stats.TotalTasks)
	}

	if stats.TotalTokens != 4500 {
		t.Errorf("expected 4500 tokens, got %d", stats.TotalTokens)
	}

	if stats.TotalCostUSD != 0.045 {
		t.Errorf("expected 0.045 cost, got %.4f", stats.TotalCostUSD)
	}

	expectedAvgValue := (1.0 + 0.5 + 0.9) / 3.0
	if stats.AverageValue != expectedAvgValue {
		t.Errorf("expected %.2f average value, got %.2f", expectedAvgValue, stats.AverageValue)
	}
}

func TestVFMModelUsage(t *testing.T) {
	tmpDir := t.TempDir()
	vfm, _ := NewVFM(tmpDir, nil)
	agentID := "test-agent-usage"

	// Track different models
	_ = vfm.TrackCost(agentID, "monitoring", "glm-4.7", 1000, 0.01)
	_ = vfm.TrackCost(agentID, "monitoring", "glm-4.7", 1500, 0.015)
	_ = vfm.TrackCost(agentID, "coding", "claude-sonnet", 5000, 0.15)

	// Get model usage
	usage, err := vfm.GetModelUsage(agentID)
	if err != nil {
		t.Fatalf("GetModelUsage failed: %v", err)
	}

	if len(usage) != 2 {
		t.Errorf("expected 2 models, got %d", len(usage))
	}

	// Find glm-4.7 usage
	var glmUsage *ModelUsage
	for i := range usage {
		if usage[i].Model == "glm-4.7" {
			glmUsage = &usage[i]
			break
		}
	}

	if glmUsage == nil {
		t.Fatal("glm-4.7 usage not found")
	}

	if glmUsage.TaskCount != 2 {
		t.Errorf("expected 2 tasks for glm-4.7, got %d", glmUsage.TaskCount)
	}

	if glmUsage.Tokens != 2500 {
		t.Errorf("expected 2500 tokens for glm-4.7, got %d", glmUsage.Tokens)
	}
}

func TestVFMSuggest(t *testing.T) {
	tmpDir := t.TempDir()
	vfm, _ := NewVFM(tmpDir, nil)
	agentID := "test-agent-suggest"

	// Track premium model usage on routine tasks
	for i := 0; i < 5; i++ {
		_ = vfm.TrackCost(agentID, "monitoring", "claude-opus-4", 10000, 0.20)
	}

	// Get suggestions
	suggestions, err := vfm.Suggest(agentID)
	if err != nil {
		t.Fatalf("Suggest failed: %v", err)
	}

	if len(suggestions) == 0 {
		t.Error("expected at least one suggestion")
	}

	// Check suggestion content
	sugg := suggestions[0]
	if sugg.CurrentModel != "claude-opus-4" {
		t.Errorf("expected current model claude-opus-4, got %s", sugg.CurrentModel)
	}

	if sugg.SuggestedModel == "" {
		t.Error("expected suggested model to be set")
	}

	if sugg.Reason == "" {
		t.Error("expected reason to be set")
	}
}

func TestVFMAgentIsolation(t *testing.T) {
	tmpDir := t.TempDir()
	vfm, _ := NewVFM(tmpDir, nil)

	agent1 := "agent-1"
	agent2 := "agent-2"

	_ = vfm.TrackCost(agent1, "task1", "model1", 1000, 0.01)
	_ = vfm.TrackCost(agent2, "task2", "model2", 2000, 0.02)

	// Check isolation
	stats1, _ := vfm.GetStats(agent1)
	stats2, _ := vfm.GetStats(agent2)

	if stats1.TotalTasks != 1 {
		t.Errorf("agent1: expected 1 task, got %d", stats1.TotalTasks)
	}

	if stats2.TotalTasks != 1 {
		t.Errorf("agent2: expected 1 task, got %d", stats2.TotalTasks)
	}

	if stats1.TotalTokens != 1000 {
		t.Errorf("agent1: expected 1000 tokens, got %d", stats1.TotalTokens)
	}

	if stats2.TotalTokens != 2000 {
		t.Errorf("agent2: expected 2000 tokens, got %d", stats2.TotalTokens)
	}
}

func TestVFMEmptyStats(t *testing.T) {
	tmpDir := t.TempDir()
	vfm, _ := NewVFM(tmpDir, nil)
	agentID := "test-agent-empty"

	// Get stats for agent with no data
	stats, err := vfm.GetStats(agentID)
	if err != nil {
		t.Fatalf("GetStats failed: %v", err)
	}

	if stats.TotalTasks != 0 {
		t.Errorf("expected 0 tasks, got %d", stats.TotalTasks)
	}

	if stats.TotalTokens != 0 {
		t.Errorf("expected 0 tokens, got %d", stats.TotalTokens)
	}

	if stats.TotalCostUSD != 0.0 {
		t.Errorf("expected 0.0 cost, got %.4f", stats.TotalCostUSD)
	}

	if stats.VFMScore != 0 {
		t.Errorf("expected 0 VFM score, got %.2f", stats.VFMScore)
	}
}

func TestVFMVFMScore(t *testing.T) {
	tmpDir := t.TempDir()
	vfm, _ := NewVFM(tmpDir, nil)
	agentID := "test-agent-score"

	// Track tasks with good value (high value, low cost)
	_ = vfm.TrackCostWithValue(agentID, "task1", "budget-model", 1000, 0.01, 0.9)
	_ = vfm.TrackCostWithValue(agentID, "task2", "budget-model", 1500, 0.015, 0.8)

	stats, _ := vfm.GetStats(agentID)

	// VFM score should be high (good value for money)
	// score = total_value / (total_cost / 10) = 1.7 / (0.025 / 10) = 680
	if stats.VFMScore <= 0 {
		t.Errorf("expected positive VFM score, got %.2f", stats.VFMScore)
	}
}
