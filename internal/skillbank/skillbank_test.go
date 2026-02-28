package skillbank

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func tempStore(t *testing.T) *FileStore {
	t.Helper()
	dir := t.TempDir()
	fs, err := NewFileStore(filepath.Join(dir, "skills.jsonl"))
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	return fs
}

func makeSkill(id, category, taskType string) Skill {
	now := time.Now()
	return Skill{
		ID:          id,
		Title:       "Test Skill " + id,
		Principle:   "Always verify inputs before processing",
		WhenToApply: "when receiving external data",
		Category:    category,
		TaskType:    taskType,
		Source:      SourceManual,
		Confidence:  0.8,
		SuccessRate: 0.8,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
}

func makeMistake(id, taskType string) CommonMistake {
	return CommonMistake{
		ID:           id,
		Description:  "Ignoring error returns",
		WhyItHappens: "Developer forgot to check",
		HowToAvoid:   "Always check returned errors",
		TaskType:     taskType,
	}
}

func makeTrajectory(taskType string, success bool) Trajectory {
	return Trajectory{
		TaskDescription: "Test task for " + taskType,
		TaskType:        taskType,
		Steps: []TrajectoryStep{
			{Action: "run tests", Observation: "tests passed", Timestamp: time.Now()},
		},
		Success: success,
		Quality: 0.9,
	}
}

// ---------------------------------------------------------------------------
// Store tests
// ---------------------------------------------------------------------------

func TestFileStore_AddGetListUpdateDelete(t *testing.T) {
	fs := tempStore(t)

	s := makeSkill("skill-1", "general", "")

	// Add
	if err := fs.Add(s); err != nil {
		t.Fatalf("Add: %v", err)
	}
	if fs.Count() != 1 {
		t.Fatalf("Count want 1, got %d", fs.Count())
	}

	// Duplicate add
	if err := fs.Add(s); err != ErrDuplicateID {
		t.Fatalf("expected ErrDuplicateID, got %v", err)
	}

	// Get
	got, err := fs.Get("skill-1")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Title != s.Title {
		t.Errorf("Title mismatch: want %q got %q", s.Title, got.Title)
	}

	// Get not found
	if _, err := fs.Get("nonexistent"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}

	// List all
	skills, err := fs.List("")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("List want 1, got %d", len(skills))
	}

	// List by category
	skills, err = fs.List("general")
	if err != nil {
		t.Fatalf("List category: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("List category want 1, got %d", len(skills))
	}

	skills, err = fs.List("nonexistent-category")
	if err != nil {
		t.Fatalf("List missing category: %v", err)
	}
	if len(skills) != 0 {
		t.Fatalf("List missing category want 0, got %d", len(skills))
	}

	// Update
	s.Title = "Updated Title"
	if err := fs.Update(s); err != nil {
		t.Fatalf("Update: %v", err)
	}
	got, _ = fs.Get("skill-1")
	if got.Title != "Updated Title" {
		t.Errorf("Update not persisted: got %q", got.Title)
	}

	// Update not found
	fake := makeSkill("does-not-exist", "general", "")
	if err := fs.Update(fake); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound on Update, got %v", err)
	}

	// Delete
	if err := fs.Delete("skill-1"); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if fs.Count() != 0 {
		t.Fatalf("Count after delete want 0, got %d", fs.Count())
	}

	// Delete not found
	if err := fs.Delete("skill-1"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound on Delete, got %v", err)
	}
}

func TestFileStore_Persistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "skills.jsonl")

	// Write some skills
	fs1, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("NewFileStore: %v", err)
	}
	for i := 0; i < 5; i++ {
		s := makeSkill(fmt.Sprintf("skill-%d", i), "general", "")
		if err := fs1.Add(s); err != nil {
			t.Fatalf("Add skill-%d: %v", i, err)
		}
	}
	m := makeMistake("mistake-1", "coding")
	if err := fs1.AddMistake(m); err != nil {
		t.Fatalf("AddMistake: %v", err)
	}

	// Reload
	fs2, err := NewFileStore(path)
	if err != nil {
		t.Fatalf("reload NewFileStore: %v", err)
	}
	if fs2.Count() != 5 {
		t.Fatalf("persistence: want 5 skills, got %d", fs2.Count())
	}

	mistakes, err := fs2.ListMistakes("")
	if err != nil {
		t.Fatalf("ListMistakes: %v", err)
	}
	if len(mistakes) != 1 {
		t.Fatalf("persistence: want 1 mistake, got %d", len(mistakes))
	}
	if mistakes[0].ID != "mistake-1" {
		t.Errorf("mistake ID mismatch: got %q", mistakes[0].ID)
	}
}

func TestFileStore_ThreadSafety(t *testing.T) {
	fs := tempStore(t)
	n := 50
	var wg sync.WaitGroup

	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s := makeSkill(fmt.Sprintf("concurrent-%d", i), "general", "")
			_ = fs.Add(s)
		}(i)
	}
	wg.Wait()

	if fs.Count() != n {
		t.Fatalf("thread safety: want %d skills, got %d", n, fs.Count())
	}

	// Concurrent reads while writing
	var wg2 sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg2.Add(2)
		go func(i int) {
			defer wg2.Done()
			_, _ = fs.List("")
		}(i)
		go func(i int) {
			defer wg2.Done()
			s := makeSkill(fmt.Sprintf("concurrent2-%d", i), "general", "")
			_ = fs.Add(s)
		}(i)
	}
	wg2.Wait()
}

func TestFileStore_Mistakes(t *testing.T) {
	fs := tempStore(t)

	m1 := makeMistake("m1", "coding")
	m2 := makeMistake("m2", "testing")
	m3 := makeMistake("m3", "coding")

	if err := fs.AddMistake(m1); err != nil {
		t.Fatalf("AddMistake m1: %v", err)
	}
	if err := fs.AddMistake(m2); err != nil {
		t.Fatalf("AddMistake m2: %v", err)
	}
	if err := fs.AddMistake(m3); err != nil {
		t.Fatalf("AddMistake m3: %v", err)
	}

	// Duplicate
	if err := fs.AddMistake(m1); err != ErrDuplicateID {
		t.Fatalf("expected ErrDuplicateID for mistake, got %v", err)
	}

	all, _ := fs.ListMistakes("")
	if len(all) != 3 {
		t.Fatalf("ListMistakes all: want 3, got %d", len(all))
	}

	coding, _ := fs.ListMistakes("coding")
	if len(coding) != 2 {
		t.Fatalf("ListMistakes coding: want 2, got %d", len(coding))
	}

	if err := fs.DeleteMistake("m1"); err != nil {
		t.Fatalf("DeleteMistake: %v", err)
	}
	if err := fs.DeleteMistake("m1"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound on second delete, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Retriever tests
// ---------------------------------------------------------------------------

func TestTemplateRetriever_KeywordMatch(t *testing.T) {
	fs := tempStore(t)

	skills := []Skill{
		{
			ID: "s1", Title: "Error Handling", Principle: "Always check returned errors",
			WhenToApply: "when making API calls", Category: "general", Source: SourceManual,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		},
		{
			ID: "s2", Title: "Retry Logic", Principle: "Retry transient failures with backoff",
			WhenToApply: "when network calls fail", Category: "general", Source: SourceManual,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		},
		{
			ID: "s3", Title: "Database Indexing", Principle: "Index frequently queried columns",
			WhenToApply: "when optimizing database queries", Category: "database", Source: SourceManual,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		},
	}

	for _, s := range skills {
		if err := fs.Add(s); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	r := &TemplateRetriever{store: fs}

	// "error handling" should match s1
	got, err := r.Retrieve(context.Background(), "how to handle errors in API calls", 3)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) == 0 {
		t.Fatal("expected at least 1 result for error handling query")
	}
	if got[0].ID != "s1" {
		t.Errorf("top result want s1, got %s", got[0].ID)
	}
}

func TestTemplateRetriever_TopK(t *testing.T) {
	fs := tempStore(t)

	for i := 0; i < 10; i++ {
		s := Skill{
			ID: fmt.Sprintf("s%d", i), Title: fmt.Sprintf("API skill %d", i),
			Principle: "Use exponential backoff for API retries",
			WhenToApply: "when calling external APIs",
			Category: "general", Source: SourceManual,
			CreatedAt: time.Now(), UpdatedAt: time.Now(),
		}
		if err := fs.Add(s); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	r := &TemplateRetriever{store: fs}

	got, err := r.Retrieve(context.Background(), "external API call with retries", 3)
	if err != nil {
		t.Fatalf("Retrieve: %v", err)
	}
	if len(got) > 3 {
		t.Errorf("TopK=3 want ≤3 results, got %d", len(got))
	}
}

func TestTemplateRetriever_EmptyStore(t *testing.T) {
	fs := tempStore(t)
	r := &TemplateRetriever{store: fs}
	got, err := r.Retrieve(context.Background(), "some task", 5)
	if err != nil {
		t.Fatalf("Retrieve on empty store: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("want 0 results on empty store, got %d", len(got))
	}
}

func TestNewRetriever_FallsBackToTemplate(t *testing.T) {
	fs := tempStore(t)
	// No embedding URL → should return TemplateRetriever
	r := NewRetriever(fs, "")
	if _, ok := r.(*TemplateRetriever); !ok {
		t.Errorf("expected TemplateRetriever when embeddingURL is empty, got %T", r)
	}
}

func TestNewRetriever_ReturnsEmbeddingRetriever(t *testing.T) {
	fs := tempStore(t)
	r := NewRetriever(fs, "http://localhost:8765")
	if _, ok := r.(*EmbeddingRetriever); !ok {
		t.Errorf("expected EmbeddingRetriever when embeddingURL is set, got %T", r)
	}
}

func TestEmbeddingRetriever_FallsBackOnError(t *testing.T) {
	fs := tempStore(t)
	s := Skill{
		ID: "s1", Title: "Error Handling", Principle: "Always check returned errors",
		WhenToApply: "when making API calls", Category: "general", Source: SourceManual,
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
	}
	if err := fs.Add(s); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Point to an unreachable embedding server — should fallback to TemplateRetriever
	r := NewRetriever(fs, "http://127.0.0.1:19999")
	got, err := r.Retrieve(context.Background(), "handle API errors", 3)
	if err != nil {
		t.Fatalf("Retrieve with fallback: %v", err)
	}
	// Template retriever might return results depending on keyword overlap
	_ = got // just ensure no panic
}

func TestCosineSimilarity(t *testing.T) {
	tests := []struct {
		a, b []float64
		want float64
	}{
		{[]float64{1, 0}, []float64{1, 0}, 1.0},
		{[]float64{1, 0}, []float64{0, 1}, 0.0},
		{[]float64{1, 1}, []float64{1, 1}, 1.0},
		{[]float64{}, []float64{}, 0.0},
		{[]float64{1}, []float64{1, 2}, 0.0}, // mismatched lengths
	}
	for _, tt := range tests {
		got := cosineSimilarity(tt.a, tt.b)
		if fmt.Sprintf("%.2f", got) != fmt.Sprintf("%.2f", tt.want) {
			t.Errorf("cosineSimilarity(%v, %v) = %.2f, want %.2f", tt.a, tt.b, got, tt.want)
		}
	}
}

// ---------------------------------------------------------------------------
// Injector tests
// ---------------------------------------------------------------------------

func TestInjector_FormatForPrompt(t *testing.T) {
	inj := NewInjector()

	skills := []Skill{
		{Title: "Systematic Exploration", WhenToApply: "goal object count not met", Principle: "Search each surface once before revisiting"},
		{Title: "Error Handling", WhenToApply: "making API calls", Principle: "Always check returned errors, never ignore them"},
	}
	mistakes := []CommonMistake{
		{Description: "Unused imports cause build failures", HowToAvoid: "remove them before committing"},
	}

	result := inj.FormatForPrompt(skills, mistakes)

	if !strings.Contains(result, "## Relevant Skills from Past Experience") {
		t.Error("missing skills header")
	}
	if !strings.Contains(result, "[Systematic Exploration]") {
		t.Error("missing skill 1 title")
	}
	if !strings.Contains(result, "[Error Handling]") {
		t.Error("missing skill 2 title")
	}
	if !strings.Contains(result, "## Common Mistakes to Avoid") {
		t.Error("missing mistakes header")
	}
	if !strings.Contains(result, "Unused imports") {
		t.Error("missing mistake description")
	}
}

func TestInjector_FormatForPrompt_Empty(t *testing.T) {
	inj := NewInjector()
	result := inj.FormatForPrompt(nil, nil)
	if result != "" {
		t.Errorf("expected empty string for no skills/mistakes, got %q", result)
	}
}

func TestInjector_FormatForPrompt_SkillsOnly(t *testing.T) {
	inj := NewInjector()
	skills := []Skill{
		{Title: "Test Skill", WhenToApply: "always", Principle: "Do something"},
	}
	result := inj.FormatForPrompt(skills, nil)
	if !strings.Contains(result, "## Relevant Skills") {
		t.Error("missing skills header")
	}
	if strings.Contains(result, "## Common Mistakes") {
		t.Error("should not have mistakes header when no mistakes")
	}
}

func TestInjector_FormatForPrompt_MistakesOnly(t *testing.T) {
	inj := NewInjector()
	mistakes := []CommonMistake{
		{Description: "Some mistake", HowToAvoid: "don't do it"},
	}
	result := inj.FormatForPrompt(nil, mistakes)
	if strings.Contains(result, "## Relevant Skills") {
		t.Error("should not have skills header when no skills")
	}
	if !strings.Contains(result, "## Common Mistakes to Avoid") {
		t.Error("missing mistakes header")
	}
}

func TestInjector_InjectIntoPrompt(t *testing.T) {
	inj := NewInjector()

	skills := []Skill{
		{Title: "Test Skill", WhenToApply: "when testing", Principle: "Write tests first"},
	}
	original := "You are a helpful AI assistant."
	result := inj.InjectIntoPrompt(original, skills, nil)

	if !strings.HasPrefix(result, "## Relevant Skills") {
		t.Error("injected block should be prepended")
	}
	if !strings.Contains(result, original) {
		t.Error("original prompt should be preserved")
	}
}

func TestInjector_InjectIntoPrompt_NoSkills(t *testing.T) {
	inj := NewInjector()
	original := "You are a helpful AI assistant."
	result := inj.InjectIntoPrompt(original, nil, nil)
	if result != original {
		t.Errorf("with no skills, prompt should be unchanged: got %q", result)
	}
}

func TestInjector_InjectIntoPrompt_EmptyPrompt(t *testing.T) {
	inj := NewInjector()
	skills := []Skill{
		{Title: "Test", WhenToApply: "always", Principle: "Do it"},
	}
	result := inj.InjectIntoPrompt("", skills, nil)
	if result == "" {
		t.Error("should return the skill block even with empty prompt")
	}
}

// ---------------------------------------------------------------------------
// Updater tests
// ---------------------------------------------------------------------------

// mockDistiller is a test double for Distiller.
type mockDistiller struct {
	skills   []Skill
	mistakes []CommonMistake
	err      error
	called   int
}

func (m *mockDistiller) Distill(_ context.Context, trajectories []Trajectory) ([]Skill, []CommonMistake, error) {
	m.called++
	if m.err != nil {
		return nil, nil, m.err
	}
	// Return one skill per trajectory to make tests deterministic
	skills := make([]Skill, 0, len(trajectories))
	for i, t := range trajectories {
		now := time.Now()
		skills = append(skills, Skill{
			ID:          fmt.Sprintf("mock-skill-%d-%d", m.called, i),
			Title:       "Mock Skill for " + t.TaskType,
			Principle:   "Mock principle",
			WhenToApply: "always",
			Category:    "general",
			TaskType:    t.TaskType,
			Source:      SourceDistilled,
			Confidence:  0.7,
			SuccessRate: 0.7,
			CreatedAt:   now,
			UpdatedAt:   now,
		})
	}
	return append(skills, m.skills...), append(m.mistakes, m.mistakes...), nil
}

func TestSkillUpdater_Update_NewSkills(t *testing.T) {
	fs := tempStore(t)
	md := &mockDistiller{}
	u := NewSkillUpdater(md, fs, t.TempDir())

	failures := []Trajectory{
		makeTrajectory("coding", false),
		makeTrajectory("testing", false),
	}

	added, err := u.Update(context.Background(), failures, nil)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(added) == 0 {
		t.Fatal("expected new skills to be added")
	}
	if md.called == 0 {
		t.Fatal("distiller should have been called")
	}
}

func TestSkillUpdater_Update_CoveredByExisting(t *testing.T) {
	fs := tempStore(t)
	md := &mockDistiller{}
	u := NewSkillUpdater(md, fs, t.TempDir())

	// Existing general skill covers everything
	existing := []Skill{makeSkill("general-1", "general", "")}

	failures := []Trajectory{makeTrajectory("coding", false)}

	added, err := u.Update(context.Background(), failures, existing)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(added) != 0 {
		t.Errorf("general skill covers all, expected 0 new skills, got %d", len(added))
	}
	if md.called != 0 {
		t.Error("distiller should not have been called when all failures are covered")
	}
}

func TestSkillUpdater_PruneStaleSkills(t *testing.T) {
	dir := t.TempDir()
	fs, _ := NewFileStore(filepath.Join(dir, "skills.jsonl"))
	md := &mockDistiller{}
	u := NewSkillUpdater(md, fs, dir)

	// Add 3 skills: 2 stale, 1 healthy
	now := time.Now()
	stale1 := Skill{
		ID: "stale-1", Title: "Stale 1", Principle: "p", WhenToApply: "w",
		Category: "general", Source: SourceManual,
		SuccessRate: 0.2, UsageCount: 20, Confidence: 0.5,
		CreatedAt: now, UpdatedAt: now,
	}
	stale2 := Skill{
		ID: "stale-2", Title: "Stale 2", Principle: "p", WhenToApply: "w",
		Category: "general", Source: SourceManual,
		SuccessRate: 0.1, UsageCount: 50, Confidence: 0.5,
		CreatedAt: now, UpdatedAt: now,
	}
	healthy := Skill{
		ID: "healthy-1", Title: "Healthy 1", Principle: "p", WhenToApply: "w",
		Category: "general", Source: SourceManual,
		SuccessRate: 0.9, UsageCount: 100, Confidence: 0.9,
		CreatedAt: now, UpdatedAt: now,
	}
	insufficientUsage := Skill{
		ID: "low-usage", Title: "Low Usage", Principle: "p", WhenToApply: "w",
		Category: "general", Source: SourceManual,
		SuccessRate: 0.1, UsageCount: 2, Confidence: 0.5, // below minUsage=5
		CreatedAt: now, UpdatedAt: now,
	}

	for _, s := range []Skill{stale1, stale2, healthy, insufficientUsage} {
		if err := fs.Add(s); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	pruned, err := u.PruneStaleSkills(context.Background(), 0.5, 5)
	if err != nil {
		t.Fatalf("PruneStaleSkills: %v", err)
	}
	if pruned != 2 {
		t.Errorf("want 2 pruned, got %d", pruned)
	}
	if fs.Count() != 2 { // healthy + low-usage remain
		t.Errorf("want 2 remaining skills, got %d", fs.Count())
	}

	// Verify archive file was created
	archivePath := filepath.Join(dir, archiveFileName)
	if _, err := os.Stat(archivePath); os.IsNotExist(err) {
		t.Error("archive file should have been created")
	}
}

func TestSkillUpdater_PruneStaleSkills_NoneToProune(t *testing.T) {
	fs := tempStore(t)
	md := &mockDistiller{}
	u := NewSkillUpdater(md, fs, t.TempDir())

	now := time.Now()
	s := Skill{
		ID: "good", Title: "Good", Principle: "p", WhenToApply: "w",
		Category: "general", Source: SourceManual,
		SuccessRate: 0.95, UsageCount: 100, Confidence: 0.9,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := fs.Add(s); err != nil {
		t.Fatalf("Add: %v", err)
	}

	pruned, err := u.PruneStaleSkills(context.Background(), 0.5, 5)
	if err != nil {
		t.Fatalf("PruneStaleSkills: %v", err)
	}
	if pruned != 0 {
		t.Errorf("want 0 pruned, got %d", pruned)
	}
}

func TestSkillUpdater_BoostConfidence(t *testing.T) {
	fs := tempStore(t)
	md := &mockDistiller{}
	u := NewSkillUpdater(md, fs, t.TempDir())

	now := time.Now()
	s := Skill{
		ID: "boost-me", Title: "Test", Principle: "p", WhenToApply: "w",
		Category: "general", Source: SourceManual,
		SuccessRate: 0.5, UsageCount: 0, Confidence: 0.5,
		CreatedAt: now, UpdatedAt: now,
	}
	if err := fs.Add(s); err != nil {
		t.Fatalf("Add: %v", err)
	}

	// Success: EMA(0.1 * 1.0 + 0.9 * 0.5) = 0.1 + 0.45 = 0.55
	if err := u.BoostSkillConfidence("boost-me", true); err != nil {
		t.Fatalf("BoostSkillConfidence succeed: %v", err)
	}
	updated, _ := fs.Get("boost-me")
	expectedRate := emaAlpha*1.0 + (1-emaAlpha)*0.5
	if fmt.Sprintf("%.4f", updated.SuccessRate) != fmt.Sprintf("%.4f", expectedRate) {
		t.Errorf("SuccessRate after success: want %.4f, got %.4f", expectedRate, updated.SuccessRate)
	}
	if updated.UsageCount != 1 {
		t.Errorf("UsageCount after boost: want 1, got %d", updated.UsageCount)
	}

	// Failure: EMA(0.1 * 0.0 + 0.9 * current)
	prevRate := updated.SuccessRate
	if err := u.BoostSkillConfidence("boost-me", false); err != nil {
		t.Fatalf("BoostSkillConfidence fail: %v", err)
	}
	updated2, _ := fs.Get("boost-me")
	expectedAfterFail := emaAlpha*0.0 + (1-emaAlpha)*prevRate
	if fmt.Sprintf("%.4f", updated2.SuccessRate) != fmt.Sprintf("%.4f", expectedAfterFail) {
		t.Errorf("SuccessRate after failure: want %.4f, got %.4f", expectedAfterFail, updated2.SuccessRate)
	}
}

func TestSkillUpdater_BoostConfidence_NotFound(t *testing.T) {
	fs := tempStore(t)
	md := &mockDistiller{}
	u := NewSkillUpdater(md, fs, t.TempDir())

	if err := u.BoostSkillConfidence("nonexistent", true); err == nil {
		t.Error("expected error for nonexistent skill")
	}
}

// ---------------------------------------------------------------------------
// Tokenizer and overlap tests
// ---------------------------------------------------------------------------

func TestTokenize(t *testing.T) {
	tokens := tokenize("Hello World, this is a test!")
	if _, ok := tokens["hello"]; !ok {
		t.Error("expected 'hello' in tokens")
	}
	if _, ok := tokens["world"]; !ok {
		t.Error("expected 'world' in tokens")
	}
	// stop word 'this' should be excluded
	if _, ok := tokens["this"]; ok {
		t.Error("stop word 'this' should be excluded")
	}
	// very short words excluded
	if _, ok := tokens["is"]; ok {
		t.Error("short word 'is' should be excluded")
	}
}

func TestFilterUncovered(t *testing.T) {
	failures := []Trajectory{
		makeTrajectory("coding", false),
		makeTrajectory("testing", false),
		makeTrajectory("database", false),
	}

	// No skills — all uncovered
	uncovered := filterUncovered(failures, nil)
	if len(uncovered) != 3 {
		t.Errorf("all failures uncovered: want 3, got %d", len(uncovered))
	}

	// General skill — covers all
	general := []Skill{makeSkill("g", "general", "")}
	uncovered = filterUncovered(failures, general)
	if len(uncovered) != 0 {
		t.Errorf("general skill covers all: want 0, got %d", len(uncovered))
	}

	// Specific skill for "coding" only
	specific := []Skill{makeSkill("s1", "coding", "coding")}
	uncovered = filterUncovered(failures, specific)
	if len(uncovered) != 2 {
		t.Errorf("coding skill covers only coding: want 2 uncovered, got %d", len(uncovered))
	}
}

func TestSkillSummary(t *testing.T) {
	s := makeSkill("test", "general", "")
	summary := SkillSummary(s)
	if !strings.Contains(summary, "Test Skill test") {
		t.Errorf("summary should contain title: %q", summary)
	}
	if !strings.Contains(summary, "confidence=") {
		t.Errorf("summary should contain confidence: %q", summary)
	}
}

// ---------------------------------------------------------------------------
// Distiller parse tests (unit, no LLM needed)
// ---------------------------------------------------------------------------

func TestParseDistillResponse_ValidJSON(t *testing.T) {
	content := `{
		"skills": [
			{
				"title": "Error Handling",
				"principle": "Always check errors",
				"when_to_apply": "when calling functions",
				"category": "general",
				"task_type": "coding",
				"confidence": 0.85
			}
		],
		"mistakes": [
			{
				"description": "Ignore errors",
				"why_it_happens": "Laziness",
				"how_to_avoid": "Always check",
				"task_type": "coding"
			}
		]
	}`

	skills, mistakes, err := parseDistillResponse(content)
	if err != nil {
		t.Fatalf("parseDistillResponse: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("want 1 skill, got %d", len(skills))
	}
	if skills[0].Title != "Error Handling" {
		t.Errorf("skill title: want 'Error Handling', got %q", skills[0].Title)
	}
	if skills[0].Source != SourceDistilled {
		t.Errorf("skill source: want 'distilled', got %q", skills[0].Source)
	}
	if len(mistakes) != 1 {
		t.Fatalf("want 1 mistake, got %d", len(mistakes))
	}
}

func TestParseDistillResponse_WithMarkdownFences(t *testing.T) {
	content := "```json\n{\"skills\": [], \"mistakes\": []}\n```"
	skills, mistakes, err := parseDistillResponse(content)
	if err != nil {
		t.Fatalf("parseDistillResponse with fences: %v", err)
	}
	if len(skills) != 0 || len(mistakes) != 0 {
		t.Error("expected empty results")
	}
}

func TestParseDistillResponse_DefaultConfidence(t *testing.T) {
	content := `{"skills": [{"title": "T", "principle": "P", "when_to_apply": "W", "category": "general"}], "mistakes": []}`
	skills, _, err := parseDistillResponse(content)
	if err != nil {
		t.Fatalf("parseDistillResponse: %v", err)
	}
	if skills[0].Confidence != 0.7 {
		t.Errorf("default confidence want 0.7, got %.2f", skills[0].Confidence)
	}
}

func TestParseDistillResponse_InvalidJSON(t *testing.T) {
	_, _, err := parseDistillResponse("not json")
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}
