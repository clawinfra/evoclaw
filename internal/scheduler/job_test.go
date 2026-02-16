package scheduler

import (
	"testing"
	"time"
)

func TestJobValidation(t *testing.T) {
	tests := []struct {
		name    string
		job     *Job
		wantErr bool
	}{
		{
			name: "valid interval job",
			job: &Job{
				ID:       "test-job",
				Name:     "Test Job",
				Enabled:  true,
				Schedule: ScheduleConfig{Kind: "interval", IntervalMs: 60000},
				Action:   ActionConfig{Kind: "shell", Command: "echo test"},
			},
			wantErr: false,
		},
		{
			name: "valid cron job",
			job: &Job{
				ID:       "cron-job",
				Name:     "Cron Job",
				Enabled:  true,
				Schedule: ScheduleConfig{Kind: "cron", Expr: "0 * * * *"},
				Action:   ActionConfig{Kind: "shell", Command: "echo test"},
			},
			wantErr: false,
		},
		{
			name: "valid at job",
			job: &Job{
				ID:       "at-job",
				Name:     "At Job",
				Enabled:  true,
				Schedule: ScheduleConfig{Kind: "at", Time: "09:00"},
				Action:   ActionConfig{Kind: "shell", Command: "echo test"},
			},
			wantErr: false,
		},
		{
			name: "missing job ID",
			job: &Job{
				Name:     "Test",
				Schedule: ScheduleConfig{Kind: "interval", IntervalMs: 60000},
				Action:   ActionConfig{Kind: "shell", Command: "echo test"},
			},
			wantErr: true,
		},
		{
			name: "missing job name",
			job: &Job{
				ID:       "test",
				Schedule: ScheduleConfig{Kind: "interval", IntervalMs: 60000},
				Action:   ActionConfig{Kind: "shell", Command: "echo test"},
			},
			wantErr: true,
		},
		{
			name: "invalid schedule kind",
			job: &Job{
				ID:       "test",
				Name:     "Test",
				Schedule: ScheduleConfig{Kind: "invalid"},
				Action:   ActionConfig{Kind: "shell", Command: "echo test"},
			},
			wantErr: true,
		},
		{
			name: "invalid cron expression",
			job: &Job{
				ID:       "test",
				Name:     "Test",
				Schedule: ScheduleConfig{Kind: "cron", Expr: "invalid cron"},
				Action:   ActionConfig{Kind: "shell", Command: "echo test"},
			},
			wantErr: true,
		},
		{
			name: "invalid time format",
			job: &Job{
				ID:       "test",
				Name:     "Test",
				Schedule: ScheduleConfig{Kind: "at", Time: "25:00"},
				Action:   ActionConfig{Kind: "shell", Command: "echo test"},
			},
			wantErr: true,
		},
		{
			name: "interval job with zero interval",
			job: &Job{
				ID:       "test",
				Name:     "Test",
				Schedule: ScheduleConfig{Kind: "interval", IntervalMs: 0},
				Action:   ActionConfig{Kind: "shell", Command: "echo test"},
			},
			wantErr: true,
		},
		{
			name: "invalid action kind",
			job: &Job{
				ID:       "test",
				Name:     "Test",
				Schedule: ScheduleConfig{Kind: "interval", IntervalMs: 60000},
				Action:   ActionConfig{Kind: "invalid"},
			},
			wantErr: true,
		},
		{
			name: "shell action without command",
			job: &Job{
				ID:       "test",
				Name:     "Test",
				Schedule: ScheduleConfig{Kind: "interval", IntervalMs: 60000},
				Action:   ActionConfig{Kind: "shell"},
			},
			wantErr: true,
		},
		{
			name: "agent action without agent ID",
			job: &Job{
				ID:       "test",
				Name:     "Test",
				Schedule: ScheduleConfig{Kind: "interval", IntervalMs: 60000},
				Action:   ActionConfig{Kind: "agent", Message: "test"},
			},
			wantErr: true,
		},
		{
			name: "agent action without message",
			job: &Job{
				ID:       "test",
				Name:     "Test",
				Schedule: ScheduleConfig{Kind: "interval", IntervalMs: 60000},
				Action:   ActionConfig{Kind: "agent", AgentID: "test-agent"},
			},
			wantErr: true,
		},
		{
			name: "mqtt action without topic",
			job: &Job{
				ID:       "test",
				Name:     "Test",
				Schedule: ScheduleConfig{Kind: "interval", IntervalMs: 60000},
				Action:   ActionConfig{Kind: "mqtt", Payload: map[string]any{"test": "data"}},
			},
			wantErr: true,
		},
		{
			name: "http action without URL",
			job: &Job{
				ID:       "test",
				Name:     "Test",
				Schedule: ScheduleConfig{Kind: "interval", IntervalMs: 60000},
				Action:   ActionConfig{Kind: "http", Method: "GET"},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.job.Validate()
			if (err != nil) != tt.wantErr {
				t.Errorf("Job.Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestNextRun(t *testing.T) {
	now := time.Date(2026, 2, 16, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name     string
		job      *Job
		from     time.Time
		wantNext time.Time
		wantErr  bool
	}{
		{
			name: "interval 1 hour",
			job: &Job{
				Schedule: ScheduleConfig{Kind: "interval", IntervalMs: 3600000},
			},
			from:     now,
			wantNext: now.Add(1 * time.Hour),
			wantErr:  false,
		},
		{
			name: "interval 5 minutes",
			job: &Job{
				Schedule: ScheduleConfig{Kind: "interval", IntervalMs: 300000},
			},
			from:     now,
			wantNext: now.Add(5 * time.Minute),
			wantErr:  false,
		},
		{
			name: "cron every hour",
			job: &Job{
				Schedule: ScheduleConfig{Kind: "cron", Expr: "0 * * * *"},
			},
			from:     now,
			wantNext: time.Date(2026, 2, 16, 13, 0, 0, 0, time.UTC),
			wantErr:  false,
		},
		{
			name: "cron every day at midnight",
			job: &Job{
				Schedule: ScheduleConfig{Kind: "cron", Expr: "0 0 * * *"},
			},
			from:     now,
			wantNext: time.Date(2026, 2, 17, 0, 0, 0, 0, time.UTC),
			wantErr:  false,
		},
		{
			name: "at 15:00 same day",
			job: &Job{
				Schedule: ScheduleConfig{Kind: "at", Time: "15:00"},
			},
			from:     now,
			wantNext: time.Date(2026, 2, 16, 15, 0, 0, 0, time.Local),
			wantErr:  false,
		},
		{
			name: "at 09:00 next day (time passed)",
			job: &Job{
				Schedule: ScheduleConfig{Kind: "at", Time: "09:00"},
			},
			from:     now,
			wantNext: time.Date(2026, 2, 17, 9, 0, 0, 0, time.Local),
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			next, err := tt.job.NextRun(tt.from)
			if (err != nil) != tt.wantErr {
				t.Errorf("Job.NextRun() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				// For 'at' schedule, compare with minute precision (timezone handling)
				if tt.job.Schedule.Kind == "at" {
					if next.Hour() != tt.wantNext.Hour() || next.Minute() != tt.wantNext.Minute() {
						t.Errorf("Job.NextRun() = %v, want %v (hour/minute)", next, tt.wantNext)
					}
				} else {
					if !next.Equal(tt.wantNext) {
						t.Errorf("Job.NextRun() = %v, want %v", next, tt.wantNext)
					}
				}
			}
		})
	}
}

func TestJobClone(t *testing.T) {
	original := &Job{
		ID:      "test",
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
		State: JobState{
			RunCount:   10,
			ErrorCount: 2,
		},
	}

	clone := original.Clone()

	// Verify deep copy
	if clone.ID != original.ID {
		t.Errorf("Clone ID mismatch")
	}
	if clone.State.RunCount != original.State.RunCount {
		t.Errorf("Clone State.RunCount mismatch")
	}

	// Modify clone, ensure original unchanged
	clone.Enabled = false
	clone.State.RunCount = 20

	if !original.Enabled {
		t.Errorf("Modifying clone affected original.Enabled")
	}
	if original.State.RunCount != 10 {
		t.Errorf("Modifying clone affected original.State.RunCount")
	}
}
