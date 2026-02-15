package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/clawinfra/evoclaw/internal/scheduler"
)

// handleSchedulerStatus returns scheduler statistics
func (s *Server) handleSchedulerStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sched := s.orch.GetScheduler()
	if sched == nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"enabled": false,
			"message": "Scheduler not enabled",
		})
		return
	}

	stats := sched.GetStats()
	writeJSON(w, http.StatusOK, stats)
}

// handleSchedulerJobs handles /api/scheduler/jobs (list or add)
func (s *Server) handleSchedulerJobs(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		s.handleSchedulerListJobs(w, r)
	case http.MethodPost:
		s.handleSchedulerAddJob(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSchedulerJobRoutes routes /api/scheduler/jobs/:id requests
func (s *Server) handleSchedulerJobRoutes(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	
	// Check if path ends with /run
	if strings.HasSuffix(path, "/run") {
		s.handleSchedulerRunJob(w, r)
		return
	}
	
	// Otherwise route based on method
	switch r.Method {
	case http.MethodGet:
		s.handleSchedulerGetJob(w, r)
	case http.MethodPatch:
		s.handleSchedulerUpdateJob(w, r)
	case http.MethodDelete:
		s.handleSchedulerRemoveJob(w, r)
	default:
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSchedulerListJobs returns all jobs
func (s *Server) handleSchedulerListJobs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	sched := s.orch.GetScheduler()
	if sched == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "Scheduler not available",
		})
		return
	}

	jobs := sched.ListJobs()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"jobs":  jobs,
		"count": len(jobs),
	})
}

// handleSchedulerGetJob returns a specific job
func (s *Server) handleSchedulerGetJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract job ID from path
	jobID := r.URL.Path[len("/api/scheduler/jobs/"):]
	if jobID == "" {
		http.Error(w, "Job ID required", http.StatusBadRequest)
		return
	}

	sched := s.orch.GetScheduler()
	if sched == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "Scheduler not available",
		})
		return
	}

	job, err := sched.GetJob(jobID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, job)
}

// handleSchedulerRunJob triggers a job immediately
func (s *Server) handleSchedulerRunJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract job ID from path
	jobID := r.URL.Path[len("/api/scheduler/jobs/"):]
	jobID = jobID[:len(jobID)-len("/run")]

	sched := s.orch.GetScheduler()
	if sched == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "Scheduler not available",
		})
		return
	}

	if err := sched.RunJobNow(jobID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "Job triggered",
		"job_id":  jobID,
	})
}

// handleSchedulerUpdateJob updates a job (enable/disable)
func (s *Server) handleSchedulerUpdateJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPatch {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract job ID from path
	jobID := r.URL.Path[len("/api/scheduler/jobs/"):]

	var update struct {
		Enabled *bool `json:"enabled"`
	}

	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
		return
	}

	sched := s.orch.GetScheduler()
	if sched == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "Scheduler not available",
		})
		return
	}

	// Get current job
	job, err := sched.GetJob(jobID)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": err.Error(),
		})
		return
	}

	// Update enabled status
	if update.Enabled != nil {
		job.Enabled = *update.Enabled
		if err := sched.UpdateJob(job); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{
				"error": err.Error(),
			})
			return
		}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"message": "Job updated",
		"job":     job,
	})
}

// handleSchedulerAddJob adds a new job
func (s *Server) handleSchedulerAddJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var job scheduler.Job
	if err := json.NewDecoder(r.Body).Decode(&job); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": "Invalid request body",
		})
		return
	}

	sched := s.orch.GetScheduler()
	if sched == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "Scheduler not available",
		})
		return
	}

	if err := sched.AddJob(&job); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"message": "Job added",
		"job":     job,
	})
}

// handleSchedulerRemoveJob removes a job
func (s *Server) handleSchedulerRemoveJob(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract job ID from path
	jobID := r.URL.Path[len("/api/scheduler/jobs/"):]

	sched := s.orch.GetScheduler()
	if sched == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"error": "Scheduler not available",
		})
		return
	}

	if err := sched.RemoveJob(jobID); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{
			"error": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"message": "Job removed",
		"job_id":  jobID,
	})
}
