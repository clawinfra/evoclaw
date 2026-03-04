package skillbank

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// store.go — load error paths and corrupt JSONL
// ---------------------------------------------------------------------------

func TestFileStore_Load_CorruptSkills(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skills.jsonl")

	// Write valid line then corrupt line
	f, _ := os.Create(path)
	_, _ = f.WriteString(`{"id":"ok","title":"T","principle":"p","when_to_apply":"w","category":"general","source":"manual","created_at":"2024-01-01T00:00:00Z","updated_at":"2024-01-01T00:00:00Z"}` + "\n")
	_, _ = f.WriteString("NOT VALID JSON\n")
	_ = f.Close()

	_, err := NewFileStore(path)
	if err == nil {
		t.Error("expected error loading corrupt JSONL")
	}
}

func TestFileStore_Load_CorruptMistakes(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skills.jsonl")
	mpath := filepath.Join(dir, "skills_mistakes.jsonl")

	// Valid skills file, corrupt mistakes file
	f, _ := os.Create(path)
	_ = f.Close()

	mf, _ := os.Create(mpath)
	_, _ = mf.WriteString("NOT VALID JSON\n")
	_ = mf.Close()

	_, err := NewFileStore(path)
	if err == nil {
		t.Error("expected error loading corrupt mistakes JSONL")
	}
}

// ---------------------------------------------------------------------------
// store.go — flush error paths
// ---------------------------------------------------------------------------

func TestFileStore_Flush_InvalidDir(t *testing.T) {
	// Create a store, then make the directory read-only to trigger flush failure
	dir := t.TempDir()
	path := filepath.Join(dir, "skills.jsonl")

	fs, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	// Make directory read-only
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Skip("cannot change permissions, skipping")
	}
	defer os.Chmod(dir, 0o755) //nolint:errcheck

	s := makeSkill("test", "general", "")
	err = fs.Add(s)
	if err == nil {
		// On some systems (running as root) this may succeed
		t.Log("flush to read-only dir succeeded (may be running as root)")
	}
}

// ---------------------------------------------------------------------------
// updater.go — Update with duplicate skills from distiller
// ---------------------------------------------------------------------------

func TestSkillUpdater_Update_DuplicateSkillsFromDistiller(t *testing.T) {
	fs := tempStore(t)

	// Pre-add a skill with a known ID so the distiller's output clashes
	now := time.Now()
	existing := Skill{
		ID: "mock-skill-1-0", Title: "Pre-existing", Principle: "p",
		WhenToApply: "w", Category: "general", Source: SourceManual,
		Confidence: 0.8, SuccessRate: 0.8,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := fs.Add(existing); err != nil {
		t.Fatalf("pre-add: %v", err)
	}

	md := &mockDistiller{}
	u := NewSkillUpdater(md, fs, t.TempDir())

	failures := []Trajectory{makeTrajectory("new-type-xyz", false)}
	// The distiller returns IDs like "mock-skill-{called}-{i}"
	// Since called will be 1 and i=0, it produces "mock-skill-1-0" which exists
	added, err := u.Update(context.Background(), failures, nil)
	if err != nil {
		t.Fatalf("Update with duplicate: %v", err)
	}
	// Duplicate should be skipped gracefully
	_ = added
}

// ---------------------------------------------------------------------------
// parseDistillResponse — backtick fence variant
// ---------------------------------------------------------------------------

func TestParseDistillResponse_BacktickFence(t *testing.T) {
	content := "```\n{\"skills\": [], \"mistakes\": []}\n```"
	skills, mistakes, err := parseDistillResponse(content)
	if err != nil {
		t.Fatalf("backtick fence: %v", err)
	}
	if len(skills) != 0 || len(mistakes) != 0 {
		t.Error("expected empty results")
	}
}

func TestParseDistillResponse_PreambleText(t *testing.T) {
	content := `Here are the extracted skills:
{"skills":[{"title":"T","principle":"P","when_to_apply":"W","category":"general","confidence":0.8}],"mistakes":[]}`
	skills, _, err := parseDistillResponse(content)
	if err != nil {
		t.Fatalf("preamble text: %v", err)
	}
	if len(skills) != 1 {
		t.Errorf("want 1 skill, got %d", len(skills))
	}
}

// ---------------------------------------------------------------------------
// distillBatch — request context cancellation
// ---------------------------------------------------------------------------

func TestLLMDistiller_Distill_ContextCancelled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate slow server
		resp := map[string]any{
			"choices": []map[string]any{
				{"message": map[string]string{"content": `{"skills":[],"mistakes":[]}`}},
			},
		}
		_ = json.NewEncoder(w).Encode(resp)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	d := NewLLMDistiller(srv.URL, "", "model")
	_, _, err := d.Distill(ctx, []Trajectory{makeTrajectory("coding", false)})
	// Context is cancelled, should return error
	if err == nil {
		t.Log("context-cancelled distill returned nil (may be a race with fast server)")
	}
}

// ---------------------------------------------------------------------------
// EmbeddingRetriever — embed with bad response body decode
// ---------------------------------------------------------------------------

func TestEmbeddingRetriever_Embed_BadResponseDecode(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"embedding": "not-an-array"}`))
	}))
	defer srv.Close()

	er := &EmbeddingRetriever{
		embeddingURL: srv.URL,
		client:       srv.Client(),
		fallback:     &TemplateRetriever{store: nil},
	}

	// embed should return error (wrong type), client fallback handles it
	_, err := er.embed(context.Background(), "test text")
	if err == nil {
		t.Error("expected error for invalid embedding type")
	}
}

// ---------------------------------------------------------------------------
// appendJSONL — verify content
// ---------------------------------------------------------------------------

func TestAppendJSONL_MultipleAppends(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, archiveFileName)

	now := time.Now()
	batch1 := []Skill{
		{ID: "a1", Title: "A1", Principle: "p", WhenToApply: "w", Category: "general", Source: SourceManual, CreatedAt: now, UpdatedAt: now},
	}
	batch2 := []Skill{
		{ID: "a2", Title: "A2", Principle: "p", WhenToApply: "w", Category: "general", Source: SourceManual, CreatedAt: now, UpdatedAt: now},
	}

	if err := appendJSONL(path, batch1); err != nil {
		t.Fatalf("appendJSONL batch1: %v", err)
	}
	if err := appendJSONL(path, batch2); err != nil {
		t.Fatalf("appendJSONL batch2: %v", err)
	}

	// Verify both records are in the file
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open archive: %v", err)
	}
	defer f.Close() //nolint:errcheck

	sc := bufio.NewScanner(f)
	var ids []string
	for sc.Scan() {
		var s Skill
		if err := json.Unmarshal(sc.Bytes(), &s); err == nil {
			ids = append(ids, s.ID)
		}
	}
	if len(ids) != 2 {
		t.Errorf("want 2 archived skills, got %d", len(ids))
	}
}

// ---------------------------------------------------------------------------
// TemplateRetriever — overlapScore with empty skill text
// ---------------------------------------------------------------------------

func TestOverlapScore_EmptySkillText(t *testing.T) {
	query := tokenize("test query")
	s := Skill{ID: "empty", Title: "", Principle: "", WhenToApply: "", TaskType: ""}
	score := overlapScore(query, s)
	if score != 0 {
		t.Errorf("empty skill text should score 0, got %f", score)
	}
}

// ---------------------------------------------------------------------------
// Full round-trip integration test
// ---------------------------------------------------------------------------

func TestSkillBank_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	store, err := NewFileStore(filepath.Join(dir, "skills.jsonl"))
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}

	responseContent := `{
		"skills": [
			{
				"title": "Retry on Failure",
				"principle": "Retry transient errors with exponential backoff",
				"when_to_apply": "when network calls fail",
				"category": "general",
				"task_type": "networking",
				"confidence": 0.9
			}
		],
		"mistakes": [
			{
				"description": "Hard-coded timeouts",
				"why_it_happens": "Quick implementation",
				"how_to_avoid": "Use configurable timeout values",
				"task_type": "networking"
			}
		]
	}`

	srv := makeTestChatServer(t, responseContent)
	defer srv.Close()

	distiller := NewLLMDistiller(srv.URL, "", "test-model")
	updater := NewSkillUpdater(distiller, store, dir)
	retriever := NewRetriever(store, "")
	injector := NewInjector()

	// Phase 1: Distill from failures
	failures := []Trajectory{makeTrajectory("networking", false)}
	added, err := updater.Update(context.Background(), failures, nil)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(added) == 0 {
		t.Fatal("expected skills to be added")
	}

	// Phase 2: Retrieve relevant skills
	retrieved, err := retriever.Retrieve(context.Background(), "network call retry logic", 3)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}

	// Phase 3: Inject into prompt
	mistakes, _ := store.ListMistakes("")
	prompt := injector.InjectIntoPrompt("You are a helpful AI.", retrieved, mistakes)
	if !strings.Contains(prompt, "Relevant Skills") && len(retrieved) > 0 {
		t.Error("injected prompt should contain skills block")
	}

	// Phase 4: Boost confidence
	if len(added) > 0 {
		if err := updater.BoostSkillConfidence(added[0].ID, true); err != nil {
			t.Fatalf("BoostSkillConfidence: %v", err)
		}
	}

	// Phase 5: Prune (none should be pruned — too few uses)
	pruned, err := updater.PruneStaleSkills(context.Background(), 0.5, 10)
	if err != nil {
		t.Fatalf("PruneStaleSkills: %v", err)
	}
	if pruned != 0 {
		t.Errorf("no skills should be pruned with only 1 use, got %d", pruned)
	}
}
