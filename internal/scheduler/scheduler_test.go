package scheduler

import (
	"context"
	"sync"
	"testing"
	"time"
)

// MockExecutor implements Executor interface for testing
type MockExecutor struct {
	mu              sync.Mutex
	agentCalls      []AgentCall
	mqttPublishes   []MQTTPublish
}

type AgentCall struct {
	AgentID string
	Message string
}

type MQTTPublish struct {
	Topic   string
	Payload map[string]any
}

func (m *MockExecutor) ExecuteAgent(ctx context.Context, agentID, message string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.agentCalls = append(m.agentCalls, AgentCall{AgentID: agentID, Message: message})
	return nil
}

func (m *MockExecutor) PublishMQTT(ctx context.Context, topic string, payload map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.mqttPublishes = append(m.mqttPublishes, MQTTPublish{Topic: topic, Payload: payload})
	return nil
}

func (m *MockExecutor) GetAgentCalls() []AgentCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]AgentCall{}, m.agentCalls...)
}

func (m *MockExecutor) GetMQTTPublishes() []MQTTPublish {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]MQTTPublish{}, m.mqttPublishes...)
}

func TestNewScheduler(t *testing.T) {
	executor := &MockExecutor{}
	sched := NewScheduler(executor, nil)

	if sched == nil {
		t.Fatal("NewScheduler returned nil")
	}
	if sched.executor != executor {
		t.Error("Executor not set correctly")
	}
	if len(sched.jobs) != 0 {
		t.Error("Jobs map should be empty")
	}
}

func TestSchedulerAddJob(t *testing.T) {
	executor := &MockExecutor{}
	sched := NewScheduler(executor, nil)

	job := &Job{
		ID:      "test-job",
		Name:    "Test Job",
		Enabled: true,
		Schedule: ScheduleConfig{
			Kind:       "interval",
			IntervalMs: 60000,
		},
		Action: ActionConfig{
			Kind:    "shell",
			Command: "echo test",
		},
	}

	err := sched.AddJob(job)
	if err != nil {
		t.Fatalf("AddJob failed: %v", err)
	}

	// Try adding duplicate
	err = sched.AddJob(job)
	if err == nil {
		t.Error("AddJob should fail for duplicate ID")
	}

	// Verify job was added
	retrieved, err := sched.GetJob("test-job")
	if err != nil {
		t.Fatalf("GetJob failed: %v", err)
	}
	if retrieved.ID != job.ID {
		t.Error("Retrieved job ID doesn't match")
	}
}

func TestSchedulerRemoveJob(t *testing.T) {
	executor := &MockExecutor{}
	sched := NewScheduler(executor, nil)

	job := &Job{
		ID:      "test-job",
		Name:    "Test Job",
		Enabled: true,
		Schedule: ScheduleConfig{
			Kind:       "interval",
			IntervalMs: 60000,
		},
		Action: ActionConfig{
			Kind:    "shell",
			Command: "echo test",
		},
	}

	_ = sched.AddJob(job)

	err := sched.RemoveJob("test-job")
	if err != nil {
		t.Fatalf("RemoveJob failed: %v", err)
	}

	// Verify job was removed
	_, err = sched.GetJob("test-job")
	if err == nil {
		t.Error("GetJob should fail for removed job")
	}

	// Try removing non-existent job
	err = sched.RemoveJob("non-existent")
	if err == nil {
		t.Error("RemoveJob should fail for non-existent job")
	}
}

func TestSchedulerUpdateJob(t *testing.T) {
	executor := &MockExecutor{}
	sched := NewScheduler(executor, nil)

	job := &Job{
		ID:      "test-job",
		Name:    "Test Job",
		Enabled: true,
		Schedule: ScheduleConfig{
			Kind:       "interval",
			IntervalMs: 60000,
		},
		Action: ActionConfig{
			Kind:    "shell",
			Command: "echo test",
		},
	}

	_ = sched.AddJob(job)

	// Update job
	job.Enabled = false
	err := sched.UpdateJob(job)
	if err != nil {
		t.Fatalf("UpdateJob failed: %v", err)
	}

	// Verify update
	retrieved, _ := sched.GetJob("test-job")
	if retrieved.Enabled {
		t.Error("Job should be disabled after update")
	}

	// Try updating non-existent job
	nonExistent := &Job{
		ID:      "non-existent",
		Name:    "Non-existent",
		Enabled: true,
		Schedule: ScheduleConfig{Kind: "interval", IntervalMs: 60000},
		Action:   ActionConfig{Kind: "shell", Command: "test"},
	}
	err = sched.UpdateJob(nonExistent)
	if err == nil {
		t.Error("UpdateJob should fail for non-existent job")
	}
}

func TestSchedulerListJobs(t *testing.T) {
	executor := &MockExecutor{}
	sched := NewScheduler(executor, nil)

	jobs := []*Job{
		{
			ID:       "job1",
			Name:     "Job 1",
			Enabled:  true,
			Schedule: ScheduleConfig{Kind: "interval", IntervalMs: 60000},
			Action:   ActionConfig{Kind: "shell", Command: "echo 1"},
		},
		{
			ID:       "job2",
			Name:     "Job 2",
			Enabled:  false,
			Schedule: ScheduleConfig{Kind: "interval", IntervalMs: 120000},
			Action:   ActionConfig{Kind: "shell", Command: "echo 2"},
		},
	}

	for _, job := range jobs {
		_ = sched.AddJob(job)
	}

	list := sched.ListJobs()
	if len(list) != 2 {
		t.Errorf("ListJobs returned %d jobs, expected 2", len(list))
	}
}

func TestSchedulerLoadJobs(t *testing.T) {
	executor := &MockExecutor{}
	sched := NewScheduler(executor, nil)

	jobs := []*Job{
		{
			ID:       "job1",
			Name:     "Job 1",
			Enabled:  true,
			Schedule: ScheduleConfig{Kind: "interval", IntervalMs: 60000},
			Action:   ActionConfig{Kind: "shell", Command: "echo 1"},
		},
		{
			ID:       "job2",
			Name:     "Job 2",
			Enabled:  true,
			Schedule: ScheduleConfig{Kind: "interval", IntervalMs: 120000},
			Action:   ActionConfig{Kind: "shell", Command: "echo 2"},
		},
	}

	err := sched.LoadJobs(jobs)
	if err != nil {
		t.Fatalf("LoadJobs failed: %v", err)
	}

	list := sched.ListJobs()
	if len(list) != 2 {
		t.Errorf("LoadJobs didn't load all jobs")
	}
}

func TestSchedulerGetStats(t *testing.T) {
	executor := &MockExecutor{}
	sched := NewScheduler(executor, nil)

	job1 := &Job{
		ID:       "job1",
		Name:     "Job 1",
		Enabled:  true,
		Schedule: ScheduleConfig{Kind: "interval", IntervalMs: 60000},
		Action:   ActionConfig{Kind: "shell", Command: "echo 1"},
		State: JobState{
			RunCount:   10,
			ErrorCount: 2,
		},
	}

	job2 := &Job{
		ID:       "job2",
		Name:     "Job 2",
		Enabled:  false,
		Schedule: ScheduleConfig{Kind: "interval", IntervalMs: 120000},
		Action:   ActionConfig{Kind: "shell", Command: "echo 2"},
		State: JobState{
			RunCount:   5,
			ErrorCount: 1,
		},
	}

	_ = sched.AddJob(job1)
	_ = sched.AddJob(job2)

	stats := sched.GetStats()

	if stats["total_jobs"] != 2 {
		t.Errorf("Expected total_jobs=2, got %v", stats["total_jobs"])
	}
	if stats["active_jobs"] != 1 {
		t.Errorf("Expected active_jobs=1, got %v", stats["active_jobs"])
	}
	if stats["total_runs"] != int64(15) {
		t.Errorf("Expected total_runs=15, got %v", stats["total_runs"])
	}
	if stats["total_errors"] != int64(3) {
		t.Errorf("Expected total_errors=3, got %v", stats["total_errors"])
	}
}

func TestSchedulerRunJobNow(t *testing.T) {
	executor := &MockExecutor{}
	sched := NewScheduler(executor, nil)

	job := &Job{
		ID:      "agent-job",
		Name:    "Agent Job",
		Enabled: true,
		Schedule: ScheduleConfig{
			Kind:       "interval",
			IntervalMs: 60000,
		},
		Action: ActionConfig{
			Kind:    "agent",
			AgentID: "test-agent",
			Message: "test message",
		},
	}

	_ = sched.AddJob(job)

	// Run job immediately
	err := sched.RunJobNow("agent-job")
	if err != nil {
		t.Fatalf("RunJobNow failed: %v", err)
	}

	// Give it time to execute
	time.Sleep(100 * time.Millisecond)

	// Verify agent was called
	calls := executor.GetAgentCalls()
	if len(calls) != 1 {
		t.Errorf("Expected 1 agent call, got %d", len(calls))
	}
	if len(calls) > 0 {
		if calls[0].AgentID != "test-agent" {
			t.Errorf("Expected agentID=test-agent, got %s", calls[0].AgentID)
		}
		if calls[0].Message != "test message" {
			t.Errorf("Expected message='test message', got %s", calls[0].Message)
		}
	}
}

func TestSchedulerStartStop(t *testing.T) {
	executor := &MockExecutor{}
	sched := NewScheduler(executor, nil)

	job := &Job{
		ID:      "test-job",
		Name:    "Test Job",
		Enabled: true,
		Schedule: ScheduleConfig{
			Kind:       "interval",
			IntervalMs: 100, // Very short interval for testing
		},
		Action: ActionConfig{
			Kind:    "shell",
			Command: "echo test",
		},
	}

	_ = sched.AddJob(job)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// Start scheduler
	err := sched.Start(ctx)
	if err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Let it run for a bit
	time.Sleep(300 * time.Millisecond)

	// Stop scheduler
	sched.Stop()

	// Verify job ran at least once
	retrieved, _ := sched.GetJob("test-job")
	if retrieved.State.RunCount == 0 {
		t.Error("Job should have run at least once")
	}
}
