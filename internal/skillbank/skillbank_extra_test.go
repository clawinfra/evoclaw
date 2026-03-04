package skillbank

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// LLMDistiller tests using httptest servers
// ---------------------------------------------------------------------------

func makeTestChatServer(t *testing.T, responseContent string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": responseContent}},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func makeErrorServer(t *testing.T, code int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server error", code)
	}))
}

func TestNewLLMDistiller(t *testing.T) {
	d := NewLLMDistiller("http://localhost:8080", "key", "")
	if d.model != defaultDistillerModel {
		t.Errorf("default model: want %q, got %q", defaultDistillerModel, d.model)
	}
	d2 := NewLLMDistiller("http://localhost:8080", "key", "custom-model")
	if d2.model != "custom-model" {
		t.Errorf("custom model: want custom-model, got %q", d2.model)
	}
}

func TestLLMDistiller_Distill_Success(t *testing.T) {
	responseContent := `{
		"skills": [
			{
				"title": "Error Handling",
				"principle": "Always check returned errors",
				"when_to_apply": "when making function calls",
				"category": "general",
				"task_type": "coding",
				"confidence": 0.85
			}
		],
		"mistakes": [
			{
				"description": "Ignored error returns",
				"why_it_happens": "Developer forgot",
				"how_to_avoid": "Always check errors",
				"task_type": "coding"
			}
		]
	}`

	srv := makeTestChatServer(t, responseContent)
	defer srv.Close()

	d := NewLLMDistiller(srv.URL, "test-key", "test-model")
	trajectories := []Trajectory{makeTrajectory("coding", false)}

	skills, mistakes, err := d.Distill(context.Background(), trajectories)
	if err != nil {
		t.Fatalf("Distill: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("want 1 skill, got %d", len(skills))
	}
	if skills[0].Title != "Error Handling" {
		t.Errorf("skill title: want 'Error Handling', got %q", skills[0].Title)
	}
	if len(mistakes) != 1 {
		t.Fatalf("want 1 mistake, got %d", len(mistakes))
	}
}

func TestLLMDistiller_Distill_BatchesByTen(t *testing.T) {
	callCount := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": `{"skills":[],"mistakes":[]}`}},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	d := NewLLMDistiller(srv.URL, "", "model")

	// 25 trajectories → 3 batches (10, 10, 5)
	trajectories := make([]Trajectory, 25)
	for i := range trajectories {
		trajectories[i] = makeTrajectory(fmt.Sprintf("type-%d", i), true)
	}

	_, _, err := d.Distill(context.Background(), trajectories)
	if err != nil {
		t.Fatalf("Distill 25: %v", err)
	}
	if callCount != 3 {
		t.Errorf("expected 3 batches, got %d", callCount)
	}
}

func TestLLMDistiller_Distill_HTTPError(t *testing.T) {
	srv := makeErrorServer(t, http.StatusInternalServerError)
	defer srv.Close()

	d := NewLLMDistiller(srv.URL, "", "model")
	_, _, err := d.Distill(context.Background(), []Trajectory{makeTrajectory("coding", false)})
	if err == nil {
		t.Error("expected error on HTTP 500")
	}
}

func TestLLMDistiller_Distill_InvalidJSON(t *testing.T) {
	srv := makeTestChatServer(t, "not json at all!!!")
	defer srv.Close()

	d := NewLLMDistiller(srv.URL, "", "model")
	_, _, err := d.Distill(context.Background(), []Trajectory{makeTrajectory("coding", false)})
	if err == nil {
		t.Error("expected error for invalid JSON response")
	}
}

func TestLLMDistiller_Distill_EmptyChoices(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		resp := map[string]any{"choices": []any{}}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	d := NewLLMDistiller(srv.URL, "", "model")
	_, _, err := d.Distill(context.Background(), []Trajectory{makeTrajectory("coding", false)})
	if err == nil {
		t.Error("expected error for empty choices")
	}
}

func TestLLMDistiller_Distill_EmptyTrajectories(t *testing.T) {
	d := NewLLMDistiller("http://localhost:9999", "", "model")
	// No trajectories → no HTTP calls, no error
	skills, mistakes, err := d.Distill(context.Background(), nil)
	if err != nil {
		t.Fatalf("Distill empty: %v", err)
	}
	if len(skills) != 0 || len(mistakes) != 0 {
		t.Error("expected empty results for empty trajectories")
	}
}

func TestBuildDistillPrompt(t *testing.T) {
	trajectories := []Trajectory{
		{
			TaskDescription: "Fix the bug",
			TaskType:        "coding",
			Steps: []TrajectoryStep{
				{Action: "read code", Observation: "found bug", Timestamp: time.Now()},
				{Action: "fix code", Observation: "bug fixed", Timestamp: time.Now()},
			},
			Success: true,
			Quality: 0.9,
		},
	}

	prompt := buildDistillPrompt(trajectories)
	if prompt == "" {
		t.Fatal("expected non-empty prompt")
	}
	if len(prompt) < 50 {
		t.Errorf("prompt seems too short: %q", prompt)
	}
}

// ---------------------------------------------------------------------------
// EmbeddingRetriever with httptest server
// ---------------------------------------------------------------------------

func makeEmbeddingServer(t *testing.T, vec []float64) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/embed" {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		resp := map[string]any{"embedding": vec}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	}))
}

func TestEmbeddingRetriever_Retrieve_Success(t *testing.T) {
	fs := tempStore(t)

	s1 := Skill{
		ID: "emb-1", Title: "Error Handling", Principle: "Check errors",
		WhenToApply: "API calls", Category: "general", Source: SourceManual,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	s2 := Skill{
		ID: "emb-2", Title: "Retry Logic", Principle: "Retry with backoff",
		WhenToApply: "network failures", Category: "general", Source: SourceManual,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := fs.Add(s1); err != nil {
		t.Fatalf("Add s1: %v", err)
	}
	if err := fs.Add(s2); err != nil {
		t.Fatalf("Add s2: %v", err)
	}

	// Return identical embeddings so similarity=1 for all
	vec := []float64{0.1, 0.2, 0.3, 0.4}
	srv := makeEmbeddingServer(t, vec)
	defer srv.Close()

	r := NewRetriever(fs, srv.URL)
	got, err := r.Retrieve(context.Background(), "handle errors in API", 2)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) > 2 {
		t.Errorf("want ≤2 results, got %d", len(got))
	}
}

func TestEmbeddingRetriever_Retrieve_EmbedError(t *testing.T) {
	fs := tempStore(t)
	s := Skill{
		ID: "e1", Title: "Test", Principle: "p", WhenToApply: "w",
		Category: "general", Source: SourceManual,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := fs.Add(s); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Server returns HTTP error for embed
	srv := makeErrorServer(t, http.StatusInternalServerError)
	defer srv.Close()

	r := NewRetriever(fs, srv.URL)
	// Should fall back to TemplateRetriever without error
	_, err := r.Retrieve(context.Background(), "test", 3)
	if err != nil {
		t.Fatalf("Retrieve with error server should fallback, got: %v", err)
	}
}

func TestEmbeddingRetriever_Retrieve_InvalidEmbedding(t *testing.T) {
	fs := tempStore(t)
	s := Skill{
		ID: "e1", Title: "Test", Principle: "p", WhenToApply: "w",
		Category: "general", Source: SourceManual,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := fs.Add(s); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Server returns bad JSON
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer srv.Close()

	r := NewRetriever(fs, srv.URL)
	_, err := r.Retrieve(context.Background(), "test", 3)
	// Should fallback gracefully
	if err != nil {
		t.Fatalf("Retrieve with bad JSON should fallback: %v", err)
	}
}

// ---------------------------------------------------------------------------
// isCoveredBySkills (exported via test)
// ---------------------------------------------------------------------------

func TestIsCoveredBySkills(t *testing.T) {
	if isCoveredBySkills("coding", nil) {
		t.Error("no skills → not covered")
	}

	general := []Skill{makeSkill("g", "general", "")}
	if !isCoveredBySkills("coding", general) {
		t.Error("general skill should cover any task type")
	}

	specific := []Skill{makeSkill("s", "coding", "coding")}
	if !isCoveredBySkills("coding", specific) {
		t.Error("specific skill should cover its own task type")
	}
	if isCoveredBySkills("testing", specific) {
		t.Error("coding skill should not cover testing")
	}
}

// ---------------------------------------------------------------------------
// updater.go extra paths
// ---------------------------------------------------------------------------

func TestSkillUpdater_Update_DistillerError(t *testing.T) {
	fs := tempStore(t)
	md := &mockDistiller{err: fmt.Errorf("distiller unavailable")}
	u := NewSkillUpdater(md, fs, t.TempDir())

	failures := []Trajectory{makeTrajectory("new-type", false)}
	_, err := u.Update(context.Background(), failures, nil)
	if err == nil {
		t.Error("expected error when distiller fails")
	}
}

func TestSkillUpdater_Update_EmptyFailures(t *testing.T) {
	fs := tempStore(t)
	md := &mockDistiller{}
	u := NewSkillUpdater(md, fs, t.TempDir())

	added, err := u.Update(context.Background(), nil, nil)
	if err != nil {
		t.Fatalf("Update with nil failures: %v", err)
	}
	if len(added) != 0 {
		t.Errorf("empty failures → 0 added, got %d", len(added))
	}
	if md.called != 0 {
		t.Error("distiller should not be called with no failures")
	}
}

func TestSkillUpdater_NewSkillUpdater_DefaultDir(t *testing.T) {
	fs := tempStore(t)
	md := &mockDistiller{}
	u := NewSkillUpdater(md, fs, "")
	if u.archiveDir != "." {
		t.Errorf("default archive dir should be '.', got %q", u.archiveDir)
	}
}

// ---------------------------------------------------------------------------
// store.go writeJSONL and error paths
// ---------------------------------------------------------------------------

func TestFileStore_NewFileStore_InvalidPath(t *testing.T) {
	dir := t.TempDir()

	// Create a regular file where a directory is expected
	blockPath := dir + "/blocked"
	f, err := os.Create(blockPath)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	_ = f.Close()

	// Try to create store inside a file (not a dir) — should fail
	_, err = NewFileStore(blockPath + "/skills.jsonl")
	if err == nil {
		t.Error("expected error when parent path is a file")
	}
}

func TestWriteJSONL_Success(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.jsonl"

	err := writeJSONL(path, func(enc *json.Encoder) error {
		return enc.Encode(map[string]string{"key": "value"})
	})
	if err != nil {
		t.Fatalf("writeJSONL: %v", err)
	}
}

func TestWriteJSONL_EncoderError(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/test.jsonl"

	expectedErr := fmt.Errorf("encode error")
	err := writeJSONL(path, func(enc *json.Encoder) error {
		return expectedErr
	})
	if err != expectedErr {
		t.Errorf("want encode error, got %v", err)
	}
}
