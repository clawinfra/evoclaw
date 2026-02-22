package migrate

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func setupTestSource(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	// MEMORY.md
	os.WriteFile(filepath.Join(dir, "MEMORY.md"), []byte("# Memory\nTest memory content"), 0644)

	// memory/2026-02-20.md
	memDir := filepath.Join(dir, "memory")
	os.MkdirAll(memDir, 0750)
	os.WriteFile(filepath.Join(memDir, "2026-02-20.md"), []byte("Daily note"), 0644)
	os.WriteFile(filepath.Join(memDir, "2026-02-21.md"), []byte("Another note"), 0644)

	// SOUL.md
	os.WriteFile(filepath.Join(dir, "SOUL.md"), []byte("I am a helpful AI assistant."), 0644)

	// IDENTITY.md
	os.WriteFile(filepath.Join(dir, "IDENTITY.md"), []byte("# Alex\nAI Assistant for Bowen"), 0644)

	// AGENTS.md
	os.WriteFile(filepath.Join(dir, "AGENTS.md"), []byte("## Voice\nUse balanced voice style"), 0644)

	// skills/
	skillDir := filepath.Join(dir, "skills", "tiered-memory")
	os.MkdirAll(skillDir, 0750)
	os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("Tiered memory skill"), 0644)

	skill2 := filepath.Join(dir, "skills", "web-search")
	os.MkdirAll(skill2, 0750)

	// config.json
	cfg := map[string]any{
		"models": map[string]any{
			"providers": map[string]any{
				"anthropic": map[string]any{
					"baseUrl": "https://api.anthropic.com",
					"apiKey":  "sk-test",
				},
			},
		},
		"channels": map[string]any{
			"telegram": map[string]any{
				"enabled":  true,
				"botToken": "123:ABC",
			},
		},
		"heartbeat": map[string]any{
			"intervalMs": 30000,
			"prompt":     "check stuff",
		},
	}
	cfgData, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(dir, "config.json"), cfgData, 0644)

	// crons.json
	crons := []map[string]any{
		{"id": "heartbeat", "name": "Heartbeat", "schedule": "*/30 * * * *", "command": "check", "enabled": true},
		{"id": "backup", "name": "Backup", "schedule": "0 2 * * *", "command": "backup", "model": "local/small", "enabled": true},
	}
	cronData, _ := json.Marshal(crons)
	os.WriteFile(filepath.Join(dir, "crons.json"), cronData, 0644)

	return dir
}

func TestMemoryMigration(t *testing.T) {
	src := setupTestSource(t)
	target := t.TempDir()

	result, err := OpenClaw(Options{Source: src, Target: target})
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	if len(result.Memory) < 3 {
		t.Errorf("expected at least 3 memory files, got %d: %v", len(result.Memory), result.Memory)
	}

	// Verify files exist
	if _, err := os.Stat(filepath.Join(target, "memory", "MEMORY.md")); err != nil {
		t.Error("MEMORY.md not copied")
	}
	if _, err := os.Stat(filepath.Join(target, "memory", "2026-02-20.md")); err != nil {
		t.Error("daily note not copied")
	}
}

func TestIdentityConversion(t *testing.T) {
	src := setupTestSource(t)
	target := t.TempDir()

	result, err := OpenClaw(Options{Source: src, Target: target})
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	if len(result.Identity) == 0 {
		t.Fatal("expected identity fields to be migrated")
	}

	// Verify agent.toml exists
	agentToml := filepath.Join(target, "agent.toml")
	data, err := os.ReadFile(agentToml)
	if err != nil {
		t.Fatalf("agent.toml not created: %v", err)
	}

	content := string(data)
	if !contains(content, "name") {
		t.Error("agent.toml missing name field")
	}
	if !contains(content, "Alex") {
		t.Error("agent.toml missing extracted name 'Alex'")
	}
}

func TestConfigMapping(t *testing.T) {
	src := setupTestSource(t)
	target := t.TempDir()

	result, err := OpenClaw(Options{Source: src, Target: target})
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	if len(result.Config) == 0 {
		t.Fatal("expected config fields to be migrated")
	}

	// Check provider mapped
	foundProvider := false
	for _, c := range result.Config {
		if contains(c, "anthropic") {
			foundProvider = true
		}
	}
	if !foundProvider {
		t.Error("expected anthropic provider in config")
	}

	// Check telegram mapped
	foundTelegram := false
	for _, c := range result.Config {
		if contains(c, "telegram") {
			foundTelegram = true
		}
	}
	if !foundTelegram {
		t.Error("expected telegram channel in config")
	}

	// Verify config.toml exists
	if _, err := os.Stat(filepath.Join(target, "config.toml")); err != nil {
		t.Error("config.toml not created")
	}
}

func TestSkillsMigration(t *testing.T) {
	src := setupTestSource(t)
	target := t.TempDir()

	result, err := OpenClaw(Options{Source: src, Target: target})
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	if len(result.Skills) != 2 {
		t.Errorf("expected 2 skills, got %d: %v", len(result.Skills), result.Skills)
	}
}

func TestCronMigration(t *testing.T) {
	src := setupTestSource(t)
	target := t.TempDir()

	result, err := OpenClaw(Options{Source: src, Target: target})
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	if len(result.Cron) != 2 {
		t.Errorf("expected 2 cron jobs, got %d: %v", len(result.Cron), result.Cron)
	}
}

func TestDryRunProducesNoWrites(t *testing.T) {
	src := setupTestSource(t)
	target := t.TempDir()

	result, err := OpenClaw(Options{Source: src, Target: target, DryRun: true})
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Should still report what would be migrated
	if len(result.Memory) == 0 {
		t.Error("dry run should report memory files")
	}

	// But target should be empty
	entries, _ := os.ReadDir(target)
	if len(entries) != 0 {
		t.Errorf("dry run should not write files, found %d entries", len(entries))
	}
}

func TestSourceNotExist(t *testing.T) {
	_, err := OpenClaw(Options{Source: "/nonexistent/path", Target: t.TempDir()})
	if err == nil {
		t.Fatal("expected error for nonexistent source")
	}
}

func TestEmptySource(t *testing.T) {
	src := t.TempDir() // empty dir
	target := t.TempDir()

	result, err := OpenClaw(Options{Source: src, Target: target})
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Should succeed with empty results
	if len(result.Memory) != 0 || len(result.Identity) != 0 {
		t.Error("expected empty results for empty source")
	}
}

func TestFullMigration(t *testing.T) {
	src := setupTestSource(t)
	target := t.TempDir()

	result, err := OpenClaw(Options{Source: src, Target: target})
	if err != nil {
		t.Fatalf("migration failed: %v", err)
	}

	// Verify all sections populated
	if len(result.Memory) == 0 {
		t.Error("missing memory")
	}
	if len(result.Identity) == 0 {
		t.Error("missing identity")
	}
	if len(result.Skills) == 0 {
		t.Error("missing skills")
	}
	if len(result.Config) == 0 {
		t.Error("missing config")
	}
	if len(result.Cron) == 0 {
		t.Error("missing cron")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(s) > 0 && containsStr(s, substr))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
