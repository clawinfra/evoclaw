package skills

import (
	"log/slog"
	"os"
	"testing"
)

func TestRegistryRegisterAndLookup(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	reg := NewRegistry(logger)

	skill := &Skill{
		Manifest: SkillManifest{Name: "test-skill", Version: "1.0.0"},
		Tools: map[string]*ToolDef{
			"hello": {Name: "hello", Command: "echo", Description: "say hi"},
		},
		Healthy: true,
	}

	if err := reg.Register(skill); err != nil {
		t.Fatal(err)
	}

	// Lookup by FQN
	tool, sk, err := reg.GetTool("test-skill.hello")
	if err != nil {
		t.Fatal(err)
	}
	if tool.Command != "echo" {
		t.Errorf("expected echo, got %s", tool.Command)
	}
	if sk.Manifest.Name != "test-skill" {
		t.Errorf("unexpected skill name: %s", sk.Manifest.Name)
	}

	// Lookup by short name
	tool2, _, err := reg.GetTool("hello")
	if err != nil {
		t.Fatal(err)
	}
	if tool2.Command != "echo" {
		t.Errorf("expected echo, got %s", tool2.Command)
	}

	// Not found
	_, _, err = reg.GetTool("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent tool")
	}

	if reg.SkillCount() != 1 {
		t.Errorf("expected 1 skill, got %d", reg.SkillCount())
	}
}

func TestRegistryDuplicateSkill(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	reg := NewRegistry(logger)

	skill := &Skill{
		Manifest: SkillManifest{Name: "dup"},
		Tools:    map[string]*ToolDef{},
	}

	if err := reg.Register(skill); err != nil {
		t.Fatal(err)
	}
	if err := reg.Register(skill); err == nil {
		t.Error("expected error for duplicate registration")
	}
}

func TestSetHealth(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	reg := NewRegistry(logger)
	skill := &Skill{
		Manifest: SkillManifest{Name: "h"},
		Tools:    map[string]*ToolDef{},
		Healthy:  true,
	}
	_ = reg.Register(skill)
	reg.SetHealth("h", false)
	if skill.Healthy {
		t.Error("expected unhealthy")
	}
}
