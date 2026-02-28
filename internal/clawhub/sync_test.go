package clawhub

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestClawHubSync_PullSkills(t *testing.T) {
	srv := mockServer(t)
	defer srv.Close()

	c := NewClient(srv.URL+"/v1", "test-key")
	localDir := t.TempDir()

	// First sync — should download weather-v2
	// (bird-cli will fail GetSkill because there's no mock for it → skipped gracefully)
	n, err := c.SyncSkills(context.Background(), localDir, SyncOptions{})
	if err != nil {
		t.Fatalf("SyncSkills: %v", err)
	}
	// weather-v2 succeeds; bird-cli returns 404 → skipped
	if n < 1 {
		t.Errorf("expected at least 1 skill synced, got %d", n)
	}

	// Check skill dir was created
	skillDir := filepath.Join(localDir, "weather-v2")
	if _, err := os.Stat(skillDir); os.IsNotExist(err) {
		t.Fatal("expected weather-v2 skill dir to exist")
	}

	// Check SKILL.md was written
	skillMD := filepath.Join(skillDir, "SKILL.md")
	if _, err := os.Stat(skillMD); os.IsNotExist(err) {
		t.Fatal("expected SKILL.md to exist")
	}

	// Check skill-meta.json was written with correct data
	metaPath := filepath.Join(skillDir, "skill-meta.json")
	metaData, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("read skill-meta.json: %v", err)
	}
	var meta SkillMeta
	if err := json.Unmarshal(metaData, &meta); err != nil {
		t.Fatalf("parse skill-meta.json: %v", err)
	}
	if meta.ID != "weather-v2" {
		t.Errorf("unexpected meta ID: %q", meta.ID)
	}
}

func TestClawHubSync_SkipExisting(t *testing.T) {
	srv := mockServer(t)
	defer srv.Close()

	c := NewClient(srv.URL+"/v1", "test-key")
	localDir := t.TempDir()

	// Pre-create the weather-v2 dir to simulate existing skill
	if err := os.MkdirAll(filepath.Join(localDir, "weather-v2"), 0o755); err != nil {
		t.Fatal(err)
	}

	n, err := c.SyncSkills(context.Background(), localDir, SyncOptions{Overwrite: false})
	if err != nil {
		t.Fatalf("SyncSkills: %v", err)
	}
	// weather-v2 should be skipped
	if n != 0 {
		t.Errorf("expected 0 synced (all skipped), got %d", n)
	}
}

func TestClawHubSync_Overwrite(t *testing.T) {
	srv := mockServer(t)
	defer srv.Close()

	c := NewClient(srv.URL+"/v1", "test-key")
	localDir := t.TempDir()

	// Pre-create with stale content
	skillDir := filepath.Join(localDir, "weather-v2")
	_ = os.MkdirAll(skillDir, 0o755)
	_ = os.WriteFile(filepath.Join(skillDir, "stale.txt"), []byte("old"), 0o644)

	n, err := c.SyncSkills(context.Background(), localDir, SyncOptions{Overwrite: true})
	if err != nil {
		t.Fatalf("SyncSkills: %v", err)
	}
	if n < 1 {
		t.Errorf("expected at least 1 synced (overwrite), got %d", n)
	}
}

func TestIsSubPath(t *testing.T) {
	cases := []struct {
		base   string
		target string
		expect bool
	}{
		{"/tmp/skills/w", "/tmp/skills/w/SKILL.md", true},
		{"/tmp/skills/w", "/tmp/skills/w/sub/file.txt", true},
		{"/tmp/skills/w", "/tmp/skills/other/file.txt", false},
		{"/tmp/skills/w", "/tmp/skills/w/../evil.txt", false},
		{"/tmp/skills/w", "/etc/passwd", false},
	}

	for _, tc := range cases {
		got := isSubPath(tc.base, tc.target)
		if got != tc.expect {
			t.Errorf("isSubPath(%q, %q) = %v, want %v", tc.base, tc.target, got, tc.expect)
		}
	}
}

func TestWriteSkill_ReadmeOnly(t *testing.T) {
	// Skill with no Files but a Readme — should write SKILL.md
	skill := &Skill{
		SkillMeta: SkillMeta{ID: "readme-only", Name: "ReadmeOnly", Version: "1.0.0"},
		Files:     map[string]string{},
		Readme:    "# ReadmeOnly\nThis skill has only a readme.",
	}
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "readme-only")
	if err := writeSkill(skillDir, skill); err != nil {
		t.Fatalf("writeSkill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(skillDir, "SKILL.md")); os.IsNotExist(err) {
		t.Fatal("expected SKILL.md to be written from Readme")
	}
}

func TestWriteSkill_EscapingPath(t *testing.T) {
	skill := &Skill{
		SkillMeta: SkillMeta{ID: "escape-test", Name: "EscapeTest", Version: "1.0.0"},
		Files:     map[string]string{"../evil.txt": "bad content"},
	}
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "escape-test")
	err := writeSkill(skillDir, skill)
	if err == nil {
		t.Fatal("expected error for path escaping skill dir")
	}
}

func TestWriteSkill_SubdirFile(t *testing.T) {
	skill := &Skill{
		SkillMeta: SkillMeta{ID: "subdir-skill", Name: "SubdirSkill", Version: "1.0.0"},
		Files: map[string]string{
			"SKILL.md":        "# SubdirSkill",
			"scripts/run.sh":  "#!/bin/bash\necho hello",
		},
	}
	dir := t.TempDir()
	skillDir := filepath.Join(dir, "subdir-skill")
	if err := writeSkill(skillDir, skill); err != nil {
		t.Fatalf("writeSkill: %v", err)
	}
	if _, err := os.Stat(filepath.Join(skillDir, "scripts/run.sh")); os.IsNotExist(err) {
		t.Fatal("expected scripts/run.sh to be written")
	}
}

func TestClawHubClient_Context_Cancelled(t *testing.T) {
	srv := mockServer(t)
	defer srv.Close()

	c := NewClient(srv.URL+"/v1", "test-key")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled

	_, err := c.ListSkills(ctx, SkillFilter{})
	if err == nil {
		t.Fatal("expected error for cancelled context")
	}
}

func TestIsSubPath_Err(t *testing.T) {
	// Absolute target that's clearly not under base
	got := isSubPath("/tmp/a", "/etc/passwd")
	if got {
		t.Error("expected false for /etc/passwd not under /tmp/a")
	}
}

func TestClawHubSync_CreateLocalDir(t *testing.T) {
	srv := mockServer(t)
	defer srv.Close()

	c := NewClient(srv.URL+"/v1", "test-key")
	// Use a non-existent nested dir
	localDir := filepath.Join(t.TempDir(), "nested", "skills")

	n, err := c.SyncSkills(context.Background(), localDir, SyncOptions{})
	if err != nil {
		t.Fatalf("SyncSkills should create dir: %v", err)
	}
	_ = n
}

func TestClawHubSync_LocalDirIsFile(t *testing.T) {
	// Create a file where the local dir should be → MkdirAll fails
	f, err := os.CreateTemp("", "clawhub-test-*.txt")
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	defer func() { _ = os.Remove(f.Name()) }()

	srv := mockServer(t)
	defer srv.Close()

	c := NewClient(srv.URL+"/v1", "key")
	// localDir is a path inside the existing file → can't create as dir
	_, err = c.SyncSkills(context.Background(), filepath.Join(f.Name(), "subdir"), SyncOptions{})
	if err == nil {
		t.Fatal("expected error when localDir cannot be created")
	}
}

func TestClawHubSync_ListSkillsError(t *testing.T) {
	// Server immediately closes → ListSkills fails
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Hijack and close
		http.Error(w, "internal error", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(srv.URL+"/v1", "key")
	_, err := c.SyncSkills(context.Background(), t.TempDir(), SyncOptions{})
	if err == nil {
		t.Fatal("expected error when ListSkills fails")
	}
}

func TestClawHubSync_WriteSkillError(t *testing.T) {
	// Server returns a valid skill list but the skill dir pre-exists as a file
	now := time.Now()
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/skills", func(w http.ResponseWriter, r *http.Request) {
		resp := apiListResponse{
			Skills: []SkillMeta{{ID: "bad-skill", Name: "Bad", Version: "1.0.0", CreatedAt: now, UpdatedAt: now}},
		}
		_ = json.NewEncoder(w).Encode(resp)
	})
	mux.HandleFunc("/v1/skills/bad-skill", func(w http.ResponseWriter, r *http.Request) {
		skill := Skill{
			SkillMeta: SkillMeta{ID: "bad-skill", Name: "Bad", Version: "1.0.0", CreatedAt: now, UpdatedAt: now},
			Files:     map[string]string{"file.txt": "content"},
		}
		_ = json.NewEncoder(w).Encode(skill)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	localDir := t.TempDir()
	// Create a file where the skill dir should be
	f, _ := os.Create(filepath.Join(localDir, "bad-skill"))
	_ = f.Close()

	c := NewClient(srv.URL+"/v1", "key")
	// SyncSkills should gracefully skip the failing skill (not return error)
	n, err := c.SyncSkills(context.Background(), localDir, SyncOptions{Overwrite: true})
	if err != nil {
		t.Fatalf("SyncSkills should not error on individual skill failure: %v", err)
	}
	if n != 0 {
		t.Errorf("expected 0 synced (write failed), got %d", n)
	}
}
