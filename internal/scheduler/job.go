package scheduler

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/robfig/cron/v3"
)

// Job represents a scheduled task
type Job struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Schedule ScheduleConfig `json:"schedule"`
	Action   ActionConfig   `json:"action"`
	Enabled  bool           `json:"enabled"`
	State    JobState       `json:"state"`
}

// ScheduleConfig defines when a job runs
type ScheduleConfig struct {
	Kind       string `json:"kind"` // "interval", "cron", "at"
	IntervalMs int64  `json:"intervalMs,omitempty"`
	Expr       string `json:"expr,omitempty"` // cron expression
	Time       string `json:"time,omitempty"` // "HH:MM" for daily
	Timezone   string `json:"timezone,omitempty"`
}

// ActionConfig defines what a job does
type ActionConfig struct {
	Kind    string         `json:"kind"` // "shell", "agent", "mqtt", "http"
	Command string         `json:"command,omitempty"`
	Args    []string       `json:"args,omitempty"`
	AgentID string         `json:"agentId,omitempty"`
	Message string         `json:"message,omitempty"`
	Topic   string         `json:"topic,omitempty"`
	Payload map[string]any `json:"payload,omitempty"`
	URL     string         `json:"url,omitempty"`
	Method  string         `json:"method,omitempty"` // "GET", "POST", etc.
	Headers map[string]string `json:"headers,omitempty"`
}

// JobState tracks job execution state
type JobState struct {
	LastRunAt  time.Time `json:"lastRunAt,omitempty"`
	NextRunAt  time.Time `json:"nextRunAt,omitempty"`
	RunCount   int64     `json:"runCount"`
	ErrorCount int64     `json:"errorCount"`
	LastError  string    `json:"lastError,omitempty"`
	LastDuration time.Duration `json:"lastDuration,omitempty"`
}

// Validate checks if job configuration is valid
func (j *Job) Validate() error {
	if j.ID == "" {
		return fmt.Errorf("job ID required")
	}
	if j.Name == "" {
		return fmt.Errorf("job name required")
	}

	// Validate schedule
	switch j.Schedule.Kind {
	case "interval":
		if j.Schedule.IntervalMs <= 0 {
			return fmt.Errorf("intervalMs must be positive")
		}
	case "cron":
		if j.Schedule.Expr == "" {
			return fmt.Errorf("cron expression required")
		}
		// Validate cron expression
		if _, err := cron.ParseStandard(j.Schedule.Expr); err != nil {
			return fmt.Errorf("invalid cron expression: %w", err)
		}
	case "at":
		if j.Schedule.Time == "" {
			return fmt.Errorf("time required for 'at' schedule")
		}
		// Validate HH:MM format
		if _, err := time.Parse("15:04", j.Schedule.Time); err != nil {
			return fmt.Errorf("invalid time format (use HH:MM): %w", err)
		}
	default:
		return fmt.Errorf("unknown schedule kind: %s (use interval, cron, or at)", j.Schedule.Kind)
	}

	// Validate action
	switch j.Action.Kind {
	case "shell":
		if j.Action.Command == "" {
			return fmt.Errorf("command required for shell action")
		}
	case "agent":
		if j.Action.AgentID == "" {
			return fmt.Errorf("agentId required for agent action")
		}
		if j.Action.Message == "" {
			return fmt.Errorf("message required for agent action")
		}
	case "mqtt":
		if j.Action.Topic == "" {
			return fmt.Errorf("topic required for mqtt action")
		}
	case "http":
		if j.Action.URL == "" {
			return fmt.Errorf("url required for http action")
		}
		if j.Action.Method == "" {
			j.Action.Method = "GET"
		}
	default:
		return fmt.Errorf("unknown action kind: %s (use shell, agent, mqtt, or http)", j.Action.Kind)
	}

	return nil
}

// NextRun calculates the next run time based on schedule
func (j *Job) NextRun(from time.Time) (time.Time, error) {
	switch j.Schedule.Kind {
	case "interval":
		interval := time.Duration(j.Schedule.IntervalMs) * time.Millisecond
		return from.Add(interval), nil

	case "cron":
		schedule, err := cron.ParseStandard(j.Schedule.Expr)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse cron: %w", err)
		}
		return schedule.Next(from), nil

	case "at":
		t, err := time.Parse("15:04", j.Schedule.Time)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse time: %w", err)
		}

		// Get timezone
		loc := time.Local
		if j.Schedule.Timezone != "" {
			loc, err = time.LoadLocation(j.Schedule.Timezone)
			if err != nil {
				return time.Time{}, fmt.Errorf("load timezone: %w", err)
			}
		}

		// Build next occurrence
		next := time.Date(from.Year(), from.Month(), from.Day(),
			t.Hour(), t.Minute(), 0, 0, loc)

		// If time has passed today, schedule for tomorrow
		if next.Before(from) || next.Equal(from) {
			next = next.Add(24 * time.Hour)
		}

		return next, nil

	default:
		return time.Time{}, fmt.Errorf("unknown schedule kind: %s", j.Schedule.Kind)
	}
}

// Clone creates a deep copy of the job
func (j *Job) Clone() *Job {
	data, _ := json.Marshal(j)
	var clone Job
	json.Unmarshal(data, &clone)
	return &clone
}
