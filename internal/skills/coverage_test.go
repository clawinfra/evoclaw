package skills

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestListSkills(t *testing.T) {
	r := NewRegistry(slog.Default())
	skill := &Skill{
		Manifest: SkillManifest{Name: "test-skill", Version: "1.0.0"},
		Tools:    map[string]*ToolDef{"tool1": {Name: "tool1"}},
	}
	_ = r.Register(skill)

	skills := r.ListSkills()
	if len(skills) != 1 {
		t.Errorf("ListSkills() returned %d, want 1", len(skills))
	}
}

func TestListTools(t *testing.T) {
	r := NewRegistry(slog.Default())
	skill := &Skill{
		Manifest: SkillManifest{Name: "test-skill", Version: "1.0.0"},
		Tools:    map[string]*ToolDef{"tool1": {Name: "tool1"}, "tool2": {Name: "tool2"}},
	}
	_ = r.Register(skill)

	tools := r.ListTools()
	// Should include fully-qualified names (test-skill.tool1, test-skill.tool2) plus short names
	if len(tools) == 0 {
		t.Error("ListTools() returned empty")
	}
}

func TestDefaultSkillsDir(t *testing.T) {
	dir := DefaultSkillsDir()
	if dir == "" {
		t.Error("DefaultSkillsDir() returned empty string")
	}
	if !filepath.IsAbs(dir) {
		// At least shouldn't be empty
		t.Logf("DefaultSkillsDir() = %q (relative, possibly fallback)", dir)
	}
}

func TestExpandHome(t *testing.T) {
	// No tilde
	result := expandHome("/usr/bin/test")
	if result != "/usr/bin/test" {
		t.Errorf("expandHome(%q) = %q, expected unchanged", "/usr/bin/test", result)
	}

	// With tilde
	result = expandHome("~/test/path")
	home, err := os.UserHomeDir()
	if err == nil {
		expected := filepath.Join(home, "/test/path")
		if result != expected {
			t.Errorf("expandHome(~/test/path) = %q, want %q", result, expected)
		}
	}
}

func TestLoadAllNonExistentDir(t *testing.T) {
	loader := NewLoader("/nonexistent/path/skills", slog.Default())
	skills, err := loader.LoadAll()
	if err != nil {
		t.Errorf("LoadAll() on nonexistent dir should return nil err, got: %v", err)
	}
	if skills != nil {
		t.Errorf("LoadAll() on nonexistent dir should return nil skills, got: %v", skills)
	}
}

func TestLoadAllEmptyDir(t *testing.T) {
	tmpDir := t.TempDir()
	loader := NewLoader(tmpDir, slog.Default())
	skills, err := loader.LoadAll()
	if err != nil {
		t.Errorf("LoadAll() on empty dir should return nil err, got: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("LoadAll() on empty dir should return empty, got %d", len(skills))
	}
}

func TestLoadAllWithValidSkill(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a skill directory with SKILL.md
	skillDir := filepath.Join(tmpDir, "test-skill")
	_ = os.MkdirAll(skillDir, 0755)
	_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: test-skill
version: "1.0.0"
description: A test skill
---
# Test Skill
`), 0644)

	loader := NewLoader(tmpDir, slog.Default())
	skills, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll() error: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("Expected 1 skill, got %d", len(skills))
	}
	if skills[0].Manifest.Name != "test-skill" {
		t.Errorf("Skill name = %q, want %q", skills[0].Manifest.Name, "test-skill")
	}
}

func TestLoadAllWithBadSkillMd(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "bad-skill")
	_ = os.MkdirAll(skillDir, 0755)
	// No frontmatter
	_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`No frontmatter here`), 0644)

	loader := NewLoader(tmpDir, slog.Default())
	skills, _ := loader.LoadAll()
	// Should skip bad skills
	if len(skills) != 0 {
		t.Errorf("Expected 0 skills (bad frontmatter), got %d", len(skills))
	}
}

func TestLoadAllWithAgentToml(t *testing.T) {
	tmpDir := t.TempDir()
	skillDir := filepath.Join(tmpDir, "toml-skill")
	_ = os.MkdirAll(skillDir, 0755)

	_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(`---
name: toml-skill
version: "2.0.0"
description: Skill with tools
---
# Toml Skill
`), 0644)

	_ = os.WriteFile(filepath.Join(skillDir, "agent.toml"), []byte(`[tools.my_tool]
command = "echo hello"
description = "Does stuff"
timeout_secs = 10
`), 0644)

	loader := NewLoader(tmpDir, slog.Default())
	skills, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll() error: %v", err)
	}
	if len(skills) != 1 {
		t.Fatalf("Expected 1 skill, got %d", len(skills))
	}
	if len(skills[0].Tools) != 1 {
		t.Errorf("Expected 1 tool, got %d", len(skills[0].Tools))
	}
}

func TestLoadAllSkipsFiles(t *testing.T) {
	tmpDir := t.TempDir()
	// Create a regular file (not a directory) — should be skipped
	_ = os.WriteFile(filepath.Join(tmpDir, "not-a-dir.txt"), []byte("test"), 0644)

	loader := NewLoader(tmpDir, slog.Default())
	skills, err := loader.LoadAll()
	if err != nil {
		t.Fatalf("LoadAll() error: %v", err)
	}
	if len(skills) != 0 {
		t.Errorf("Expected 0 skills (only files), got %d", len(skills))
	}
}

func TestExpandHomeNoTilde(t *testing.T) {
	result := expandHome("relative/path")
	if result != "relative/path" {
		t.Errorf("expandHome(relative/path) = %q", result)
	}
}
