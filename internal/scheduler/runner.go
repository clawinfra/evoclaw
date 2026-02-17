package scheduler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
	"time"
)

// JobRunner executes a single job on schedule
type JobRunner struct {
	job       *Job
	ticker    *time.Ticker
	logger    *slog.Logger
	executor  Executor
	stopCh    chan struct{}
	doneCh    chan struct{}
}

// Executor defines interfaces for executing actions
type Executor interface {
	ExecuteAgent(ctx context.Context, agentID, message string) error
	PublishMQTT(ctx context.Context, topic string, payload map[string]any) error
}

// NewJobRunner creates a new job runner
func NewJobRunner(job *Job, executor Executor, log *slog.Logger) *JobRunner {
	if log == nil {
		log = slog.Default()
	}
	return &JobRunner{
		job:      job,
		executor: executor,
		logger:   log.With("job", job.ID),
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

// Start begins executing the job on schedule
func (r *JobRunner) Start(ctx context.Context) {
	defer close(r.doneCh)

	if !r.job.Enabled {
		r.logger.Debug("job disabled, not starting")
		return
	}

	// Calculate initial next run
	nextRun, err := r.job.NextRun(time.Now())
	if err != nil {
		r.logger.Error("failed to calculate next run", "error", err)
		return
	}
	r.job.State.NextRunAt = nextRun

	r.logger.Info("job runner started", "next_run", nextRun.Format(time.RFC3339))

	// Set up ticker based on schedule type
	var tickerDuration time.Duration
	switch r.job.Schedule.Kind {
	case "interval":
		tickerDuration = time.Duration(r.job.Schedule.IntervalMs) * time.Millisecond
	case "cron", "at":
		// Check every minute for cron/at schedules
		tickerDuration = 1 * time.Minute
	}

	r.ticker = time.NewTicker(tickerDuration)
	defer r.ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("job runner stopped (context cancelled)")
			return
		case <-r.stopCh:
			r.logger.Info("job runner stopped")
			return
		case now := <-r.ticker.C:
			// For interval schedules, always run
			// For cron/at schedules, check if it's time
			shouldRun := false
			if r.job.Schedule.Kind == "interval" {
				shouldRun = true
			} else {
				shouldRun = now.After(r.job.State.NextRunAt) || now.Equal(r.job.State.NextRunAt)
			}

			if shouldRun {
				r.executeJob(ctx)

				// Calculate next run
				nextRun, err := r.job.NextRun(time.Now())
				if err != nil {
					r.logger.Error("failed to calculate next run", "error", err)
				} else {
					r.job.State.NextRunAt = nextRun
					r.logger.Debug("next run scheduled", "next_run", nextRun.Format(time.RFC3339))
				}
			}
		}
	}
}

// Stop stops the job runner
func (r *JobRunner) Stop() {
	close(r.stopCh)
	<-r.doneCh
}

// executeJob runs the job once
func (r *JobRunner) executeJob(ctx context.Context) {
	start := time.Now()
	r.logger.Info("executing job")

	var err error
	switch r.job.Action.Kind {
	case "shell":
		err = r.executeShell(ctx)
	case "agent":
		err = r.executeAgent(ctx)
	case "mqtt":
		err = r.executeMQTT(ctx)
	case "http":
		err = r.executeHTTP(ctx)
	default:
		err = fmt.Errorf("unknown action kind: %s", r.job.Action.Kind)
	}

	duration := time.Since(start)

	// Update state
	r.job.State.LastRunAt = time.Now()
	r.job.State.LastDuration = duration
	r.job.State.RunCount++

	if err != nil {
		r.job.State.ErrorCount++
		r.job.State.LastError = err.Error()
		r.logger.Error("job failed",
			"error", err,
			"duration", duration,
			"run_count", r.job.State.RunCount,
			"error_count", r.job.State.ErrorCount)
	} else {
		r.job.State.LastError = ""
		r.logger.Info("job completed",
			"duration", duration,
			"run_count", r.job.State.RunCount)
	}
}

// executeShell runs a shell command
func (r *JobRunner) executeShell(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "sh", "-c", r.job.Action.Command)

	// Add args if provided
	if len(r.job.Action.Args) > 0 {
		cmd.Args = append(cmd.Args, r.job.Action.Args...)
	}

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("command failed: %w (output: %s)", err, string(output))
	}

	r.logger.Debug("shell command output", "output", string(output))
	return nil
}

// executeAgent sends a message to an agent
func (r *JobRunner) executeAgent(ctx context.Context) error {
	if r.executor == nil {
		return fmt.Errorf("executor not set (cannot execute agent action)")
	}

	return r.executor.ExecuteAgent(ctx, r.job.Action.AgentID, r.job.Action.Message)
}

// executeMQTT publishes to an MQTT topic
func (r *JobRunner) executeMQTT(ctx context.Context) error {
	if r.executor == nil {
		return fmt.Errorf("executor not set (cannot execute mqtt action)")
	}

	return r.executor.PublishMQTT(ctx, r.job.Action.Topic, r.job.Action.Payload)
}

// executeHTTP makes an HTTP request
func (r *JobRunner) executeHTTP(ctx context.Context) error {
	var body []byte
	var err error

	if r.job.Action.Payload != nil {
		body, err = json.Marshal(r.job.Action.Payload)
		if err != nil {
			return fmt.Errorf("marshal payload: %w", err)
		}
	}

	req, err := http.NewRequestWithContext(ctx, r.job.Action.Method, r.job.Action.URL, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}

	// Set headers
	for k, v := range r.job.Action.Headers {
		req.Header.Set(k, v)
	}

	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("http request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		return fmt.Errorf("http request failed with status: %d", resp.StatusCode)
	}

	r.logger.Debug("http request completed", "status", resp.StatusCode)
	return nil
}
