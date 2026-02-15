package scheduler

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
)

// Scheduler manages all scheduled jobs
type Scheduler struct {
	jobs     map[string]*Job
	runners  map[string]*JobRunner
	executor Executor
	logger   *slog.Logger
	mu       sync.RWMutex
	ctx      context.Context
	cancel   context.CancelFunc
}

// Config holds scheduler configuration
type Config struct {
	Enabled bool   `json:"enabled"`
	Jobs    []*Job `json:"jobs"`
}

// NewScheduler creates a new scheduler
func NewScheduler(executor Executor, logger *slog.Logger) *Scheduler {
	if logger == nil {
		logger = slog.Default()
	}

	return &Scheduler{
		jobs:     make(map[string]*Job),
		runners:  make(map[string]*JobRunner),
		executor: executor,
		logger:   logger.With("component", "scheduler"),
	}
}

// Start initializes and starts all enabled jobs
func (s *Scheduler) Start(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.ctx, s.cancel = context.WithCancel(ctx)
	s.logger.Info("starting scheduler", "jobs", len(s.jobs))

	// Start all enabled jobs
	for id, job := range s.jobs {
		if !job.Enabled {
			s.logger.Debug("skipping disabled job", "job", id)
			continue
		}

		runner := NewJobRunner(job, s.executor, s.logger)
		s.runners[id] = runner
		go runner.Start(s.ctx)
	}

	s.logger.Info("scheduler started", "active_jobs", len(s.runners))
	return nil
}

// Stop stops all job runners
func (s *Scheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.logger.Info("stopping scheduler")

	// Cancel context to stop all runners
	if s.cancel != nil {
		s.cancel()
	}

	// Wait for all runners to stop
	for id, runner := range s.runners {
		runner.Stop()
		s.logger.Debug("stopped job runner", "job", id)
	}

	s.runners = make(map[string]*JobRunner)
	s.logger.Info("scheduler stopped")
}

// AddJob adds a new job to the scheduler
func (s *Scheduler) AddJob(job *Job) error {
	if err := job.Validate(); err != nil {
		return fmt.Errorf("invalid job: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check for duplicate ID
	if _, exists := s.jobs[job.ID]; exists {
		return fmt.Errorf("job with ID %s already exists", job.ID)
	}

	// Add job
	s.jobs[job.ID] = job

	// Start runner if scheduler is running and job is enabled
	if s.ctx != nil && job.Enabled {
		runner := NewJobRunner(job, s.executor, s.logger)
		s.runners[job.ID] = runner
		go runner.Start(s.ctx)
		s.logger.Info("job added and started", "job", job.ID)
	} else {
		s.logger.Info("job added", "job", job.ID, "enabled", job.Enabled)
	}

	return nil
}

// RemoveJob removes a job from the scheduler
func (s *Scheduler) RemoveJob(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if job exists
	if _, exists := s.jobs[id]; !exists {
		return fmt.Errorf("job not found: %s", id)
	}

	// Stop runner if running
	if runner, exists := s.runners[id]; exists {
		runner.Stop()
		delete(s.runners, id)
	}

	// Remove job
	delete(s.jobs, id)
	s.logger.Info("job removed", "job", id)

	return nil
}

// UpdateJob updates an existing job
func (s *Scheduler) UpdateJob(job *Job) error {
	if err := job.Validate(); err != nil {
		return fmt.Errorf("invalid job: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Check if job exists
	if _, exists := s.jobs[job.ID]; !exists {
		return fmt.Errorf("job not found: %s", job.ID)
	}

	// Stop old runner if running
	if runner, exists := s.runners[job.ID]; exists {
		runner.Stop()
		delete(s.runners, job.ID)
	}

	// Update job
	s.jobs[job.ID] = job

	// Start new runner if scheduler is running and job is enabled
	if s.ctx != nil && job.Enabled {
		runner := NewJobRunner(job, s.executor, s.logger)
		s.runners[job.ID] = runner
		go runner.Start(s.ctx)
		s.logger.Info("job updated and restarted", "job", job.ID)
	} else {
		s.logger.Info("job updated", "job", job.ID, "enabled", job.Enabled)
	}

	return nil
}

// GetJob retrieves a job by ID
func (s *Scheduler) GetJob(id string) (*Job, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	job, exists := s.jobs[id]
	if !exists {
		return nil, fmt.Errorf("job not found: %s", id)
	}

	return job.Clone(), nil
}

// ListJobs returns all jobs
func (s *Scheduler) ListJobs() []*Job {
	s.mu.RLock()
	defer s.mu.RUnlock()

	jobs := make([]*Job, 0, len(s.jobs))
	for _, job := range s.jobs {
		jobs = append(jobs, job.Clone())
	}

	return jobs
}

// RunJobNow triggers a job immediately (bypassing schedule)
func (s *Scheduler) RunJobNow(id string) error {
	s.mu.RLock()
	job, exists := s.jobs[id]
	s.mu.RUnlock()

	if !exists {
		return fmt.Errorf("job not found: %s", id)
	}

	// Create temporary runner and execute once
	runner := NewJobRunner(job, s.executor, s.logger)
	ctx := context.Background()
	runner.executeJob(ctx)

	return nil
}

// LoadJobs loads jobs from configuration
func (s *Scheduler) LoadJobs(jobs []*Job) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, job := range jobs {
		if err := job.Validate(); err != nil {
			s.logger.Warn("invalid job in config, skipping",
				"job", job.ID,
				"error", err)
			continue
		}

		s.jobs[job.ID] = job
		s.logger.Debug("loaded job from config", "job", job.ID)
	}

	s.logger.Info("jobs loaded", "count", len(s.jobs))
	return nil
}

// GetStats returns scheduler statistics
func (s *Scheduler) GetStats() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()

	totalRuns := int64(0)
	totalErrors := int64(0)
	activeJobs := 0

	for _, job := range s.jobs {
		totalRuns += job.State.RunCount
		totalErrors += job.State.ErrorCount
		if job.Enabled {
			activeJobs++
		}
	}

	return map[string]interface{}{
		"total_jobs":   len(s.jobs),
		"active_jobs":  activeJobs,
		"running_jobs": len(s.runners),
		"total_runs":   totalRuns,
		"total_errors": totalErrors,
	}
}
