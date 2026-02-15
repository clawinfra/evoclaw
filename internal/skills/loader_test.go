package skills

import (
	"log/slog"
	"os"
	"path/filepath"
	"testing"
)

func TestParseManifest(t *testing.T) {
	dir := t.TempDir()
	skillMD := filepath.Join(dir, "SKILL.md")
	content := `---
name: test-skill
version: 2.0.0
description: A test skill
author: tester
license: MIT
metadata:
  evoclaw:
    permissions: ["internet", "filesystem"]
    env: ["API_KEY", "SECRET"]
---

# Test Skill

Some docs here.
`
	if err := os.WriteFile(skillMD, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	loader := NewLoader(dir, logger)
	manifest, err := loader.parseManifest(skillMD)
	if err != nil {
		t.Fatal(err)
	}

	if manifest.Name != "test-skill" {
		t.Errorf("expected name test-skill, got %s", manifest.Name)
	}
	if manifest.Version != "2.0.0" {
		t.Errorf("expected version 2.0.0, got %s", manifest.Version)
	}
	if len(manifest.Metadata.EvoClaw.Permissions) != 2 {
		t.Errorf("expected 2 permissions, got %d", len(manifest.Metadata.EvoClaw.Permissions))
	}
	if len(manifest.Metadata.EvoClaw.Env) != 2 {
		t.Errorf("expected 2 env vars, got %d", len(manifest.Metadata.EvoClaw.Env))
	}
}

func TestLoadAll(t *testing.T) {
	dir := t.TempDir()

	// Create a skill directory
	skillDir := filepath.Join(dir, "my-skill")
	_ = os.MkdirAll(skillDir, 0755)

	skillMD := `---
name: my-skill
version: 1.0.0
description: My test skill
author: me
---

# My Skill
`
	_ = os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte(skillMD), 0644)

	agentTOML := `[tools.greet]
command = "echo"
description = "Say hello"
args = ["$NAME"]
timeout_secs = 10
`
	_ = os.WriteFile(filepath.Join(skillDir, "agent.toml"), []byte(agentTOML), 0644)

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	loader := NewLoader(dir, logger)
	skills, err := loader.LoadAll()
	if err != nil {
		t.Fatal(err)
	}

	if len(skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(skills))
	}
	if skills[0].Manifest.Name != "my-skill" {
		t.Errorf("unexpected name: %s", skills[0].Manifest.Name)
	}
	if len(skills[0].Tools) != 1 {
		t.Errorf("expected 1 tool, got %d", len(skills[0].Tools))
	}
	tool := skills[0].Tools["greet"]
	if tool == nil {
		t.Fatal("tool 'greet' not found")
	}
	if tool.Command != "echo" {
		t.Errorf("expected command echo, got %s", tool.Command)
	}
}

func TestLoadAllMissingDir(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	loader := NewLoader("/nonexistent/path", logger)
	skills, err := loader.LoadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(skills))
	}
}
