package scheduler

import (
	"context"
	"testing"
	"time"
)

func TestJobRunnerShellExecution(t *testing.T) {
	job := &Job{
		ID:      "shell-job",
		Name:    "Shell Job",
		Enabled: true,
		Schedule: ScheduleConfig{
			Kind:       "interval",
			IntervalMs: 1000,
		},
		Action: ActionConfig{
			Kind:    "shell",
			Command: "echo 'test output'",
		},
	}

	runner := NewJobRunner(job, nil, nil)
	ctx := context.Background()

	// Execute job once
	runner.executeJob(ctx)

	// Verify state was updated
	if job.State.RunCount != 1 {
		t.Errorf("Expected RunCount=1, got %d", job.State.RunCount)
	}
	if job.State.ErrorCount != 0 {
		t.Errorf("Expected ErrorCount=0, got %d", job.State.ErrorCount)
	}
	if job.State.LastError != "" {
		t.Errorf("Expected no error, got: %s", job.State.LastError)
	}
}

func TestJobRunnerShellFailure(t *testing.T) {
	job := &Job{
		ID:      "failing-shell-job",
		Name:    "Failing Shell Job",
		Enabled: true,
		Schedule: ScheduleConfig{
			Kind:       "interval",
			IntervalMs: 1000,
		},
		Action: ActionConfig{
			Kind:    "shell",
			Command: "exit 1", // This will fail
		},
	}

	runner := NewJobRunner(job, nil, nil)
	ctx := context.Background()

	// Execute job once
	runner.executeJob(ctx)

	// Verify state was updated with error
	if job.State.RunCount != 1 {
		t.Errorf("Expected RunCount=1, got %d", job.State.RunCount)
	}
	if job.State.ErrorCount != 1 {
		t.Errorf("Expected ErrorCount=1, got %d", job.State.ErrorCount)
	}
	if job.State.LastError == "" {
		t.Error("Expected error to be recorded")
	}
}

func TestJobRunnerAgentExecution(t *testing.T) {
	executor := &MockExecutor{}

	job := &Job{
		ID:      "agent-job",
		Name:    "Agent Job",
		Enabled: true,
		Schedule: ScheduleConfig{
			Kind:       "interval",
			IntervalMs: 1000,
		},
		Action: ActionConfig{
			Kind:    "agent",
			AgentID: "test-agent",
			Message: "test message",
		},
	}

	runner := NewJobRunner(job, executor, nil)
	ctx := context.Background()

	// Execute job once
	runner.executeJob(ctx)

	// Verify agent was called
	calls := executor.GetAgentCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 agent call, got %d", len(calls))
	}
	if calls[0].AgentID != "test-agent" {
		t.Errorf("Expected agentID=test-agent, got %s", calls[0].AgentID)
	}
	if calls[0].Message != "test message" {
		t.Errorf("Expected message='test message', got %s", calls[0].Message)
	}

	// Verify state
	if job.State.RunCount != 1 {
		t.Errorf("Expected RunCount=1, got %d", job.State.RunCount)
	}
	if job.State.ErrorCount != 0 {
		t.Errorf("Expected ErrorCount=0, got %d", job.State.ErrorCount)
	}
}

func TestJobRunnerMQTTExecution(t *testing.T) {
	executor := &MockExecutor{}

	payload := map[string]any{
		"device": "test-001",
		"type":   "sync",
	}

	job := &Job{
		ID:      "mqtt-job",
		Name:    "MQTT Job",
		Enabled: true,
		Schedule: ScheduleConfig{
			Kind:       "interval",
			IntervalMs: 1000,
		},
		Action: ActionConfig{
			Kind:    "mqtt",
			Topic:   "test/topic",
			Payload: payload,
		},
	}

	runner := NewJobRunner(job, executor, nil)
	ctx := context.Background()

	// Execute job once
	runner.executeJob(ctx)

	// Verify MQTT was published
	publishes := executor.GetMQTTPublishes()
	if len(publishes) != 1 {
		t.Fatalf("Expected 1 MQTT publish, got %d", len(publishes))
	}
	if publishes[0].Topic != "test/topic" {
		t.Errorf("Expected topic=test/topic, got %s", publishes[0].Topic)
	}

	// Verify state
	if job.State.RunCount != 1 {
		t.Errorf("Expected RunCount=1, got %d", job.State.RunCount)
	}
}

func TestJobRunnerHTTPExecution(t *testing.T) {
	// Note: This test would require a mock HTTP server
	// For now, we'll just test that HTTP action doesn't crash
	job := &Job{
		ID:      "http-job",
		Name:    "HTTP Job",
		Enabled: true,
		Schedule: ScheduleConfig{
			Kind:       "interval",
			IntervalMs: 1000,
		},
		Action: ActionConfig{
			Kind:   "http",
			Method: "GET",
			URL:    "http://localhost:9999/nonexistent",
		},
	}

	runner := NewJobRunner(job, nil, nil)
	ctx := context.Background()

	// Execute job once (will fail due to bad URL)
	runner.executeJob(ctx)

	// Verify state was updated with error
	if job.State.RunCount != 1 {
		t.Errorf("Expected RunCount=1, got %d", job.State.RunCount)
	}
	if job.State.ErrorCount != 1 {
		t.Errorf("Expected ErrorCount=1, got %d", job.State.ErrorCount)
	}
}

func TestJobRunnerStateTiming(t *testing.T) {
	job := &Job{
		ID:      "timing-job",
		Name:    "Timing Job",
		Enabled: true,
		Schedule: ScheduleConfig{
			Kind:       "interval",
			IntervalMs: 1000,
		},
		Action: ActionConfig{
			Kind:    "shell",
			Command: "sleep 0.1", // Short sleep
		},
	}

	runner := NewJobRunner(job, nil, nil)
	ctx := context.Background()

	before := time.Now()
	runner.executeJob(ctx)
	after := time.Now()

	// Verify timing was recorded
	if job.State.LastDuration == 0 {
		t.Error("Expected LastDuration to be set")
	}

	// Verify duration is reasonable (should be ~100ms)
	if job.State.LastDuration < 50*time.Millisecond || job.State.LastDuration > 1*time.Second {
		t.Errorf("Unexpected duration: %v", job.State.LastDuration)
	}

	// Verify LastRunAt was set
	if job.State.LastRunAt.Before(before) || job.State.LastRunAt.After(after) {
		t.Error("LastRunAt timestamp incorrect")
	}
}

func TestJobRunnerDisabledJob(t *testing.T) {
	job := &Job{
		ID:      "disabled-job",
		Name:    "Disabled Job",
		Enabled: false, // Job is disabled
		Schedule: ScheduleConfig{
			Kind:       "interval",
			IntervalMs: 1000,
		},
		Action: ActionConfig{
			Kind:    "shell",
			Command: "echo test",
		},
	}

	runner := NewJobRunner(job, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	// Start runner (should exit immediately for disabled job)
	go runner.Start(ctx)

	time.Sleep(100 * time.Millisecond)

	// Verify job never ran
	if job.State.RunCount != 0 {
		t.Errorf("Disabled job should not run, but RunCount=%d", job.State.RunCount)
	}
}

func TestJobRunnerStop(t *testing.T) {
	job := &Job{
		ID:      "stop-job",
		Name:    "Stop Job",
		Enabled: true,
		Schedule: ScheduleConfig{
			Kind:       "interval",
			IntervalMs: 50, // Very short interval
		},
		Action: ActionConfig{
			Kind:    "shell",
			Command: "echo test",
		},
	}

	runner := NewJobRunner(job, nil, nil)
	ctx := context.Background()

	// Start runner
	go runner.Start(ctx)

	// Let it run a few times
	time.Sleep(200 * time.Millisecond)

	// Stop runner
	runner.Stop()

	// Record run count
	runCountBefore := job.State.RunCount

	// Wait a bit more
	time.Sleep(200 * time.Millisecond)

	// Verify job stopped running
	if job.State.RunCount > runCountBefore {
		t.Errorf("Job continued running after Stop()")
	}
}
