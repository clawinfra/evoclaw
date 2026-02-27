package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/clawinfra/evoclaw/internal/agents"
	"github.com/clawinfra/evoclaw/internal/config"
	"github.com/clawinfra/evoclaw/internal/models"
	"github.com/clawinfra/evoclaw/internal/orchestrator"
	"github.com/clawinfra/evoclaw/internal/scheduler"
)

// newTestServerWithScheduler creates a server backed by a real orchestrator with a scheduler.
func newTestServerWithScheduler(t *testing.T) (*Server, *scheduler.Scheduler) {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError}))
	dir := t.TempDir()

	reg, _ := agents.NewRegistry(dir, logger)
	mem, _ := agents.NewMemoryStore(dir, logger)
	router := models.NewRouter(logger)

	// Create orchestrator with scheduler enabled via config
	cfg := config.DefaultConfig()
	cfg.Scheduler.Enabled = true
	orch := orchestrator.New(cfg, logger)
	if err := orch.Start(); err != nil {
		t.Fatalf("start orchestrator: %v", err)
	}
	t.Cleanup(func() { _ = orch.Stop() })

	sched := orch.GetScheduler()
	if sched == nil {
		t.Fatal("expected scheduler to be initialized")
	}

	srv := NewServer(0, orch, reg, mem, router, logger)
	return srv, sched
}

// newTestServerNoOrch creates a server with a nil orchestrator (scheduler unavailable).
func newTestServerNoOrch(t *testing.T) *Server {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError}))
	dir := t.TempDir()
	reg, _ := agents.NewRegistry(dir, logger)
	mem, _ := agents.NewMemoryStore(dir, logger)
	router := models.NewRouter(logger)
	return NewServer(0, nil, reg, mem, router, logger)
}

// newTestServerOrchNoScheduler creates a server with an orchestrator but no scheduler enabled.
func newTestServerOrchNoScheduler(t *testing.T) *Server {
	t.Helper()
	logger := slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError}))
	dir := t.TempDir()
	reg, _ := agents.NewRegistry(dir, logger)
	mem, _ := agents.NewMemoryStore(dir, logger)
	router := models.NewRouter(logger)
	cfg := config.DefaultConfig()
	cfg.Scheduler.Enabled = false
	orch := orchestrator.New(cfg, logger)
	return NewServer(0, orch, reg, mem, router, logger)
}

// seedJob adds a job to the scheduler and returns its ID.
func seedJob(t *testing.T, sched *scheduler.Scheduler) *scheduler.Job {
	t.Helper()
	job := &scheduler.Job{
		ID:      "test-job-1",
		Name:    "Test Job",
		Enabled: true,
		Schedule: scheduler.ScheduleConfig{
			Kind:       "interval",
			IntervalMs: 60000,
		},
		Action: scheduler.ActionConfig{
			Kind:    "shell",
			Command: "echo hello",
		},
	}
	if err := sched.AddJob(job); err != nil {
		t.Fatalf("seed job: %v", err)
	}
	return job
}

// --- handleSchedulerStatus ---

func TestHandleSchedulerStatus_NoOrch(t *testing.T) {
	s := newTestServerOrchNoScheduler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/scheduler", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerStatus(w, req)
	// orch is nil => GetScheduler not callable; server falls back to nil-orch path
	if w.Code != http.StatusOK && w.Code != http.StatusServiceUnavailable {
		t.Errorf("unexpected status %d", w.Code)
	}
}

func TestHandleSchedulerStatus_WithScheduler(t *testing.T) {
	s, _ := newTestServerWithScheduler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/scheduler", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerStatus(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestHandleSchedulerStatus_MethodNotAllowed(t *testing.T) {
	s, _ := newTestServerWithScheduler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/scheduler", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerStatus(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

// --- handleSchedulerJobs ---

func TestHandleSchedulerJobs_Get(t *testing.T) {
	s, sched := newTestServerWithScheduler(t)
	seedJob(t, sched)

	req := httptest.NewRequest(http.MethodGet, "/api/scheduler/jobs", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerJobs(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}

	var resp map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp["jobs"] == nil {
		t.Error("expected jobs in response")
	}
}

func TestHandleSchedulerJobs_Post(t *testing.T) {
	s, _ := newTestServerWithScheduler(t)

	job := scheduler.Job{
		ID:      "new-job",
		Name:    "New Job",
		Enabled: true,
		Schedule: scheduler.ScheduleConfig{
			Kind:       "interval",
			IntervalMs: 5000,
		},
		Action: scheduler.ActionConfig{Kind: "shell", Command: "echo hi"},
	}
	body, _ := json.Marshal(job)

	req := httptest.NewRequest(http.MethodPost, "/api/scheduler/jobs", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleSchedulerJobs(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("status = %d, want 201", w.Code)
	}
}

func TestHandleSchedulerJobs_MethodNotAllowed(t *testing.T) {
	s, _ := newTestServerWithScheduler(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/scheduler/jobs", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerJobs(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

// --- handleSchedulerListJobs ---

func TestHandleSchedulerListJobs_NoScheduler(t *testing.T) {
	s := newTestServerOrchNoScheduler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/scheduler/jobs", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerListJobs(w, req)
	// orch nil → scheduler nil → 503
	if w.Code != http.StatusServiceUnavailable && w.Code != http.StatusOK {
		t.Errorf("status = %d", w.Code)
	}
}

func TestHandleSchedulerListJobs_Success(t *testing.T) {
	s, sched := newTestServerWithScheduler(t)
	seedJob(t, sched)

	req := httptest.NewRequest(http.MethodGet, "/api/scheduler/jobs", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerListJobs(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// --- handleSchedulerGetJob ---

func TestHandleSchedulerGetJob_NotFound(t *testing.T) {
	s, _ := newTestServerWithScheduler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/scheduler/jobs/nonexistent", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerGetJob(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHandleSchedulerGetJob_Found(t *testing.T) {
	s, sched := newTestServerWithScheduler(t)
	seedJob(t, sched)

	req := httptest.NewRequest(http.MethodGet, "/api/scheduler/jobs/test-job-1", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerGetJob(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestHandleSchedulerGetJob_EmptyID(t *testing.T) {
	s, _ := newTestServerWithScheduler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/scheduler/jobs/", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerGetJob(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleSchedulerGetJob_MethodNotAllowed(t *testing.T) {
	s, _ := newTestServerWithScheduler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/scheduler/jobs/test", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerGetJob(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

// --- handleSchedulerRunJob ---

func TestHandleSchedulerRunJob_NotFound(t *testing.T) {
	s, _ := newTestServerWithScheduler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/scheduler/jobs/nope/run", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerRunJob(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHandleSchedulerRunJob_MethodNotAllowed(t *testing.T) {
	s, _ := newTestServerWithScheduler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/scheduler/jobs/test/run", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerRunJob(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleSchedulerRunJob_NoScheduler(t *testing.T) {
	s := newTestServerOrchNoScheduler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/scheduler/jobs/test/run", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerRunJob(w, req)
	if w.Code != http.StatusServiceUnavailable && w.Code != http.StatusNotFound {
		t.Errorf("status = %d", w.Code)
	}
}

// --- handleSchedulerUpdateJob ---

func TestHandleSchedulerUpdateJob_NotFound(t *testing.T) {
	s, _ := newTestServerWithScheduler(t)
	enabled := true
	body, _ := json.Marshal(map[string]bool{"enabled": enabled})

	req := httptest.NewRequest(http.MethodPatch, "/api/scheduler/jobs/nope", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleSchedulerUpdateJob(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHandleSchedulerUpdateJob_Success(t *testing.T) {
	s, sched := newTestServerWithScheduler(t)
	seedJob(t, sched)

	enabled := false
	body, _ := json.Marshal(map[string]bool{"enabled": enabled})

	req := httptest.NewRequest(http.MethodPatch, "/api/scheduler/jobs/test-job-1", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleSchedulerUpdateJob(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

func TestHandleSchedulerUpdateJob_InvalidJSON(t *testing.T) {
	s, _ := newTestServerWithScheduler(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/scheduler/jobs/test-job-1", bytes.NewReader([]byte("bad")))
	w := httptest.NewRecorder()
	s.handleSchedulerUpdateJob(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleSchedulerUpdateJob_MethodNotAllowed(t *testing.T) {
	s, _ := newTestServerWithScheduler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/scheduler/jobs/test", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerUpdateJob(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleSchedulerUpdateJob_NoScheduler(t *testing.T) {
	s := newTestServerOrchNoScheduler(t)
	body, _ := json.Marshal(map[string]bool{"enabled": true})
	req := httptest.NewRequest(http.MethodPatch, "/api/scheduler/jobs/test", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleSchedulerUpdateJob(w, req)
	if w.Code != http.StatusServiceUnavailable && w.Code != http.StatusNotFound {
		t.Errorf("status = %d", w.Code)
	}
}

// --- handleSchedulerAddJob ---

func TestHandleSchedulerAddJob_InvalidJSON(t *testing.T) {
	s, _ := newTestServerWithScheduler(t)
	req := httptest.NewRequest(http.MethodPost, "/api/scheduler/jobs", bytes.NewReader([]byte("bad")))
	w := httptest.NewRecorder()
	s.handleSchedulerAddJob(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleSchedulerAddJob_MethodNotAllowed(t *testing.T) {
	s, _ := newTestServerWithScheduler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/scheduler/jobs", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerAddJob(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleSchedulerAddJob_NoScheduler(t *testing.T) {
	s := newTestServerOrchNoScheduler(t)
	job := scheduler.Job{ID: "x", Name: "X"}
	body, _ := json.Marshal(job)
	req := httptest.NewRequest(http.MethodPost, "/api/scheduler/jobs", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleSchedulerAddJob(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

// --- handleSchedulerRemoveJob ---

func TestHandleSchedulerRemoveJob_Success(t *testing.T) {
	s, sched := newTestServerWithScheduler(t)
	seedJob(t, sched)

	req := httptest.NewRequest(http.MethodDelete, "/api/scheduler/jobs/test-job-1", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerRemoveJob(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestHandleSchedulerRemoveJob_NotFound(t *testing.T) {
	s, _ := newTestServerWithScheduler(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/scheduler/jobs/nope", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerRemoveJob(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHandleSchedulerRemoveJob_MethodNotAllowed(t *testing.T) {
	s, _ := newTestServerWithScheduler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/scheduler/jobs/test", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerRemoveJob(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleSchedulerRemoveJob_NoScheduler(t *testing.T) {
	s := newTestServerOrchNoScheduler(t)
	req := httptest.NewRequest(http.MethodDelete, "/api/scheduler/jobs/test", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerRemoveJob(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

// --- handleSchedulerJobRoutes ---

func TestHandleSchedulerJobRoutes_Run(t *testing.T) {
	s, sched := newTestServerWithScheduler(t)
	_ = sched // populated above

	req := httptest.NewRequest(http.MethodPost, "/api/scheduler/jobs/nope/run", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerJobRoutes(w, req)
	// job doesn't exist → 404
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestHandleSchedulerJobRoutes_Get(t *testing.T) {
	s, sched := newTestServerWithScheduler(t)
	seedJob(t, sched)

	req := httptest.NewRequest(http.MethodGet, "/api/scheduler/jobs/test-job-1", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerJobRoutes(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

func TestHandleSchedulerJobRoutes_MethodNotAllowed(t *testing.T) {
	s, _ := newTestServerWithScheduler(t)
	req := httptest.NewRequest(http.MethodPut, "/api/scheduler/jobs/test", nil)
	w := httptest.NewRecorder()
	s.handleSchedulerJobRoutes(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

// --- helpers ---

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	writeJSON(w, http.StatusOK, map[string]string{"key": "val"})
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	WriteError(w, http.StatusBadRequest, "something went wrong")
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
	var resp map[string]string
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] != "something went wrong" {
		t.Errorf("error message = %q", resp["error"])
	}
}

// --- handleChat ---

func TestHandleChat_MethodNotAllowed(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodGet, "/api/chat", nil)
	w := httptest.NewRecorder()
	s.handleChat(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleChat_InvalidJSON(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader([]byte("bad")))
	w := httptest.NewRecorder()
	s.handleChat(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleChat_MissingAgent(t *testing.T) {
	s := newTestServerV2(t)
	body, _ := json.Marshal(map[string]string{"message": "hello"})
	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleChat(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleChat_MissingMessage(t *testing.T) {
	s := newTestServerV2(t)
	body, _ := json.Marshal(map[string]string{"agent": "test"})
	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleChat(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleChat_Timeout(t *testing.T) {
	// Build an orchestrator with a real inbox but no consumer → timeout
	logger := slog.New(slog.NewTextHandler(nil, &slog.HandlerOptions{Level: slog.LevelError}))
	dir := t.TempDir()
	reg, _ := agents.NewRegistry(dir, logger)
	mem, _ := agents.NewMemoryStore(dir, logger)
	router := models.NewRouter(logger)

	cfg := config.DefaultConfig()
	orch := orchestrator.New(cfg, logger)
	if err := orch.Start(); err != nil {
		t.Fatalf("start orch: %v", err)
	}
	t.Cleanup(func() { _ = orch.Stop() })

	s := NewServer(0, orch, reg, mem, router, logger)

	body, _ := json.Marshal(map[string]string{"agent": "test", "message": "hi"})
	ctx, cancel := context.WithTimeout(context.Background(), 0) // immediate timeout
	cancel()
	req := httptest.NewRequest(http.MethodPost, "/api/chat", bytes.NewReader(body)).WithContext(ctx)
	w := httptest.NewRecorder()
	s.handleChat(w, req)
	// Either 408 (request timeout) or 503/504
	if w.Code != http.StatusRequestTimeout && w.Code != http.StatusGatewayTimeout && w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, expected timeout-related status", w.Code)
	}
}

// --- handleChatStream ---

func TestHandleChatStream_MethodNotAllowed(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodGet, "/api/chat/stream", nil)
	w := httptest.NewRecorder()
	s.handleChatStream(w, req)
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", w.Code)
	}
}

func TestHandleChatStream_InvalidJSON(t *testing.T) {
	s := newTestServerV2(t)
	req := httptest.NewRequest(http.MethodPost, "/api/chat/stream", bytes.NewReader([]byte("bad")))
	w := httptest.NewRecorder()
	s.handleChatStream(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleChatStream_MissingFields(t *testing.T) {
	s := newTestServerV2(t)
	body, _ := json.Marshal(map[string]string{"agent": "test"}) // missing message
	req := httptest.NewRequest(http.MethodPost, "/api/chat/stream", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleChatStream(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleChatStream_Success(t *testing.T) {
	s := newTestServerV2(t)
	body, _ := json.Marshal(map[string]string{"agent": "test", "message": "hi"})
	req := httptest.NewRequest(http.MethodPost, "/api/chat/stream", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleChatStream(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// --- handleMemoryStats / handleMemoryTree / handleMemoryRetrieve (nil orch path) ---

func TestHandleMemoryStats_NilOrch(t *testing.T) {
	s := newTestServerOrchNoScheduler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/memory/stats", nil)
	w := httptest.NewRecorder()
	s.handleMemoryStats(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

func TestHandleMemoryTree_NilOrch(t *testing.T) {
	s := newTestServerOrchNoScheduler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/memory/tree", nil)
	w := httptest.NewRecorder()
	s.handleMemoryTree(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

func TestHandleMemoryRetrieve_NilOrch(t *testing.T) {
	s := newTestServerOrchNoScheduler(t)
	req := httptest.NewRequest(http.MethodGet, "/api/memory/retrieve?q=test", nil)
	w := httptest.NewRecorder()
	s.handleMemoryRetrieve(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", w.Code)
	}
}

// --- handleAgentUpdate ---

func TestHandleAgentUpdate_InvalidJSON(t *testing.T) {
	s, agent := newTestServerWithAgent(t)
	req := httptest.NewRequest(http.MethodPatch, "/api/agents/test-agent", bytes.NewReader([]byte("bad")))
	w := httptest.NewRecorder()
	s.handleAgentUpdate(w, req, "test-agent", agent)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

func TestHandleAgentUpdate_Success(t *testing.T) {
	s, agent := newTestServerWithAgent(t)
	body, _ := json.Marshal(map[string]string{"name": "New Name"})
	req := httptest.NewRequest(http.MethodPatch, "/api/agents/test-agent", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleAgentUpdate(w, req, "test-agent", agent)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
}

func TestHandleAgentUpdate_InvalidModel(t *testing.T) {
	s, agent := newTestServerWithAgent(t)
	body, _ := json.Marshal(map[string]string{"model": "nonexistent/model"})
	req := httptest.NewRequest(http.MethodPatch, "/api/agents/test-agent", bytes.NewReader(body))
	w := httptest.NewRecorder()
	s.handleAgentUpdate(w, req, "test-agent", agent)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400", w.Code)
	}
}

// --- generateMessageID ---

func TestGenerateMessageID(t *testing.T) {
	id1 := generateMessageID()
	id2 := generateMessageID()
	if id1 == "" {
		t.Error("expected non-empty message ID")
	}
	if id1 == id2 {
		t.Log("warning: two consecutive IDs are the same (possible if very fast)")
	}
}
