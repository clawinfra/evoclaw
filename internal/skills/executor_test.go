package skills

import (
	"context"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"
)

func TestExecuteSimple(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	exec := NewExecutor(logger)

	tool := &ToolDef{
		Name:    "test",
		Command: "echo",
		Args:    []string{"$MSG"},
		Timeout: 5 * time.Second,
	}
	skill := &Skill{
		Manifest: SkillManifest{Name: "test-skill"},
		Dir:      os.TempDir(),
	}

	result := exec.Execute(context.Background(), tool, skill, map[string]string{"MSG": "hello world"})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if strings.TrimSpace(result.Stdout) != "hello world" {
		t.Errorf("expected 'hello world', got %q", result.Stdout)
	}
	if result.ExitCode != 0 {
		t.Errorf("expected exit 0, got %d", result.ExitCode)
	}
}

func TestExecuteTimeout(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	exec := NewExecutor(logger)

	tool := &ToolDef{
		Name:    "slow",
		Command: "sleep",
		Args:    []string{"10"},
		Timeout: 100 * time.Millisecond,
	}
	skill := &Skill{
		Manifest: SkillManifest{Name: "test"},
		Dir:      os.TempDir(),
	}

	result := exec.Execute(context.Background(), tool, skill, nil)
	if result.Err == nil {
		t.Error("expected timeout error")
	}
}

func TestExecuteFailure(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	exec := NewExecutor(logger)

	tool := &ToolDef{
		Name:    "fail",
		Command: "false",
		Timeout: 5 * time.Second,
	}
	skill := &Skill{
		Manifest: SkillManifest{Name: "test"},
		Dir:      os.TempDir(),
	}

	result := exec.Execute(context.Background(), tool, skill, nil)
	if result.Err == nil {
		t.Error("expected error")
	}
	if result.ExitCode == 0 {
		t.Error("expected non-zero exit code")
	}
}

func TestExecuteEnvInjection(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	exec := NewExecutor(logger)

	tool := &ToolDef{
		Name:    "env-test",
		Command: "sh",
		Args:    []string{"-c", "echo $SKILL_ARG_NAME"},
		Timeout: 5 * time.Second,
	}
	skill := &Skill{
		Manifest: SkillManifest{Name: "test"},
		Dir:      os.TempDir(),
	}

	result := exec.Execute(context.Background(), tool, skill, map[string]string{"NAME": "alice"})
	if result.Err != nil {
		t.Fatal(result.Err)
	}
	if strings.TrimSpace(result.Stdout) != "alice" {
		t.Errorf("expected 'alice', got %q", result.Stdout)
	}
}
