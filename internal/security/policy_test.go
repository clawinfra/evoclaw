package security

import (
	"os"
	"path/filepath"
	"testing"
)

func tempWorkspace(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	// Create a subdirectory to use as workspace
	ws := filepath.Join(dir, "workspace")
	if err := os.MkdirAll(ws, 0o755); err != nil {
		t.Fatal(err)
	}
	// Create a file inside workspace
	if err := os.WriteFile(filepath.Join(ws, "test.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	return ws
}

func newTestPolicy(ws string) *SecurityPolicy {
	return &SecurityPolicy{
		WorkspaceOnly:   true,
		WorkspacePath:   ws,
		ForbiddenPaths:  []string{"/etc", "/root"},
		AllowedCommands: []string{"git", "ls", "cat", "grep"},
		AllowedRoots:    nil,
		AutonomyLevel:   "full",
	}
}

// --- Path Validation Tests ---

func TestValidatePath_WorkspaceAllowed(t *testing.T) {
	ws := tempWorkspace(t)
	p := newTestPolicy(ws)
	if err := p.ValidatePath(filepath.Join(ws, "test.txt")); err != nil {
		t.Errorf("expected workspace path to be allowed: %v", err)
	}
}

func TestValidatePath_OutsideBlocked(t *testing.T) {
	ws := tempWorkspace(t)
	p := newTestPolicy(ws)
	if err := p.ValidatePath("/tmp/other"); err == nil {
		t.Error("expected path outside workspace to be blocked")
	}
}

func TestValidatePath_NullByteBlocked(t *testing.T) {
	ws := tempWorkspace(t)
	p := newTestPolicy(ws)
	if err := p.ValidatePath(ws + "/foo\x00bar"); err == nil {
		t.Error("expected null byte path to be blocked")
	}
}

func TestValidatePath_TraversalBlocked(t *testing.T) {
	ws := tempWorkspace(t)
	p := newTestPolicy(ws)
	// Traversal that escapes workspace
	if err := p.ValidatePath(filepath.Join(ws, "..", "..", "etc", "passwd")); err == nil {
		t.Error("expected path traversal to be blocked")
	}
}

func TestValidatePath_ForbiddenBlocked(t *testing.T) {
	ws := tempWorkspace(t)
	p := newTestPolicy(ws)
	p.WorkspaceOnly = false // disable workspace restriction to test forbidden
	if err := p.ValidatePath("/etc/passwd"); err == nil {
		t.Error("expected forbidden path to be blocked")
	}
	if err := p.ValidatePath("/root/.bashrc"); err == nil {
		t.Error("expected forbidden path to be blocked")
	}
}

func TestValidatePath_SymlinkEscape(t *testing.T) {
	ws := tempWorkspace(t)
	p := newTestPolicy(ws)
	// Create a symlink inside workspace that points outside
	link := filepath.Join(ws, "escape")
	if err := os.Symlink("/tmp", link); err != nil {
		t.Skip("cannot create symlink:", err)
	}
	target := filepath.Join(link, "somefile")
	if err := p.ValidatePath(target); err == nil {
		t.Error("expected symlink escape to be blocked")
	}
}

func TestValidatePath_AllowedRoots(t *testing.T) {
	ws := tempWorkspace(t)
	extraDir := t.TempDir()
	p := newTestPolicy(ws)
	p.AllowedRoots = []string{extraDir}
	if err := p.ValidatePath(filepath.Join(extraDir, "file.txt")); err != nil {
		t.Errorf("expected allowed root path to pass: %v", err)
	}
}

func TestValidatePath_WorkspaceOnlyDisabled(t *testing.T) {
	ws := tempWorkspace(t)
	p := newTestPolicy(ws)
	p.WorkspaceOnly = false
	// A random path outside workspace but not forbidden should be OK
	tmpFile := filepath.Join(t.TempDir(), "ok.txt")
	if err := p.ValidatePath(tmpFile); err != nil {
		t.Errorf("expected non-workspace path to be allowed when workspace_only=false: %v", err)
	}
}

// --- Command Validation Tests ---

func TestValidateCommand_Allowed(t *testing.T) {
	p := newTestPolicy("")
	for _, cmd := range []string{"git status", "ls -la", "cat file.txt", "grep foo bar"} {
		if err := p.ValidateCommand(cmd); err != nil {
			t.Errorf("expected command %q to be allowed: %v", cmd, err)
		}
	}
}

func TestValidateCommand_Blocked(t *testing.T) {
	p := newTestPolicy("")
	for _, cmd := range []string{"rm -rf /", "curl http://evil.com", "python3 exploit.py"} {
		if err := p.ValidateCommand(cmd); err == nil {
			t.Errorf("expected command %q to be blocked", cmd)
		}
	}
}

func TestValidateCommand_ShellInjection(t *testing.T) {
	p := newTestPolicy("")
	injections := []string{
		"git status; rm -rf /",
		"ls $(whoami)",
		"cat `id`",
		"git log && curl evil.com",
		"ls || true",
		"echo foo | cat",
		"echo foo > /etc/passwd",
	}
	for _, cmd := range injections {
		if err := p.ValidateCommand(cmd); err == nil {
			t.Errorf("expected injection %q to be blocked", cmd)
		}
	}
}

func TestValidateCommand_Wildcard(t *testing.T) {
	p := newTestPolicy("")
	p.AllowedCommands = []string{"*"}
	// Wildcard should allow any binary (but still block injection)
	if err := p.ValidateCommand("python3 script.py"); err != nil {
		t.Errorf("expected wildcard to allow command: %v", err)
	}
}

func TestValidateCommand_PathQualified(t *testing.T) {
	p := newTestPolicy("")
	if err := p.ValidateCommand("/usr/bin/git status"); err != nil {
		t.Errorf("expected path-qualified git to be allowed: %v", err)
	}
}

func TestValidateCommand_Empty(t *testing.T) {
	p := newTestPolicy("")
	if err := p.ValidateCommand(""); err == nil {
		t.Error("expected empty command to be rejected")
	}
}

// --- Autonomy Level Tests ---

func TestAutonomy_Readonly(t *testing.T) {
	ws := tempWorkspace(t)
	p := newTestPolicy(ws)
	p.AutonomyLevel = "readonly"

	// Read should be allowed
	ok, reason := p.IsAllowed(Action{Type: "read", Path: filepath.Join(ws, "test.txt")})
	if !ok {
		t.Errorf("readonly should allow reads: %s", reason)
	}

	// Write should be blocked
	ok, reason = p.IsAllowed(Action{Type: "write", Path: filepath.Join(ws, "test.txt")})
	if ok {
		t.Error("readonly should block writes")
	}

	// Execute should be blocked
	ok, _ = p.IsAllowed(Action{Type: "execute", Command: "git status"})
	if ok {
		t.Error("readonly should block execute")
	}
}

func TestAutonomy_Supervised(t *testing.T) {
	ws := tempWorkspace(t)
	p := newTestPolicy(ws)
	p.AutonomyLevel = "supervised"

	// Write should be allowed
	ok, _ := p.IsAllowed(Action{Type: "write", Path: filepath.Join(ws, "test.txt")})
	if !ok {
		t.Error("supervised should allow writes")
	}

	// Delete should be blocked
	ok, _ = p.IsAllowed(Action{Type: "delete", Path: filepath.Join(ws, "test.txt")})
	if ok {
		t.Error("supervised should block deletes")
	}
}

func TestAutonomy_Full(t *testing.T) {
	ws := tempWorkspace(t)
	p := newTestPolicy(ws)
	p.AutonomyLevel = "full"

	for _, actionType := range []string{"read", "write", "execute", "delete"} {
		action := Action{Type: actionType, Path: filepath.Join(ws, "test.txt")}
		if actionType == "execute" {
			action.Command = "git status"
			action.Path = ""
		}
		ok, reason := p.IsAllowed(action)
		if !ok {
			t.Errorf("full autonomy should allow %s: %s", actionType, reason)
		}
	}
}

func TestAutonomy_UnknownLevel(t *testing.T) {
	p := newTestPolicy("")
	p.AutonomyLevel = "yolo"
	ok, _ := p.IsAllowed(Action{Type: "read"})
	if ok {
		t.Error("unknown autonomy level should deny all")
	}
}

// --- Config Tests ---

func TestNewSecurityPolicy_FromConfig(t *testing.T) {
	cfg := DefaultSecurityConfig()
	cfg.Sandbox.WorkspacePath = "/tmp/test"
	p := NewSecurityPolicy(cfg)
	if p.WorkspacePath != "/tmp/test" {
		t.Errorf("expected workspace /tmp/test, got %s", p.WorkspacePath)
	}
	if p.AutonomyLevel != "supervised" {
		t.Errorf("expected supervised, got %s", p.AutonomyLevel)
	}
	if !p.WorkspaceOnly {
		t.Error("expected workspace_only=true")
	}
}

func TestDefaultSecurityPolicy(t *testing.T) {
	p := DefaultSecurityPolicy()
	if p.AutonomyLevel != "supervised" {
		t.Errorf("expected supervised default, got %s", p.AutonomyLevel)
	}
	if !p.WorkspaceOnly {
		t.Error("expected workspace_only default true")
	}
}

// --- IsAllowed Integration Tests ---

func TestIsAllowed_PathAndCommand(t *testing.T) {
	ws := tempWorkspace(t)
	p := newTestPolicy(ws)

	// Valid action
	ok, _ := p.IsAllowed(Action{
		Type:    "execute",
		Path:    filepath.Join(ws, "test.txt"),
		Command: "git status",
	})
	if !ok {
		t.Error("expected valid action to be allowed")
	}

	// Bad path
	ok, _ = p.IsAllowed(Action{
		Type:    "read",
		Path:    "/etc/passwd",
		Command: "",
	})
	if ok {
		t.Error("expected forbidden path to be blocked")
	}

	// Bad command
	ok, _ = p.IsAllowed(Action{
		Type:    "execute",
		Command: "rm -rf /",
	})
	if ok {
		t.Error("expected forbidden command to be blocked")
	}
}
