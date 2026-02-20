package orchestrator

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// ContentBlock tests
// ---------------------------------------------------------------------------

func TestContentBlock_TextBlock(t *testing.T) {
	b := TextBlock("hello world")
	if b.Kind != ContentKindText {
		t.Fatalf("expected kind %q, got %q", ContentKindText, b.Kind)
	}
	if b.Text != "hello world" {
		t.Fatalf("expected text %q, got %q", "hello world", b.Text)
	}
}

func TestContentBlock_ErrorBlock(t *testing.T) {
	b := ErrorBlock("not_found", "file not found")
	if b.Kind != ContentKindError {
		t.Fatalf("expected kind %q, got %q", ContentKindError, b.Kind)
	}
	if b.ErrCode != "not_found" {
		t.Fatalf("expected err_code %q, got %q", "not_found", b.ErrCode)
	}
	if b.Text != "file not found" {
		t.Fatalf("expected text %q, got %q", "file not found", b.Text)
	}
}

func TestContentBlock_ImageBlock(t *testing.T) {
	b := ImageBlock("image/png", "abc123base64")
	if b.Kind != ContentKindImage {
		t.Fatalf("expected kind %q, got %q", ContentKindImage, b.Kind)
	}
	if b.MimeType != "image/png" {
		t.Fatalf("expected mime %q, got %q", "image/png", b.MimeType)
	}
	if b.Data != "abc123base64" {
		t.Fatalf("expected data %q, got %q", "abc123base64", b.Data)
	}
}

func TestContentBlock_HasError(t *testing.T) {
	out := &ToolOutput{
		Content: []ContentBlock{TextBlock("ok")},
	}
	if out.HasError() {
		t.Fatal("expected HasError=false for text-only output")
	}

	out.Content = append(out.Content, ErrorBlock("fail", "something broke"))
	if !out.HasError() {
		t.Fatal("expected HasError=true when error block present")
	}
}

func TestContentBlock_Text(t *testing.T) {
	out := &ToolOutput{
		Content: []ContentBlock{
			TextBlock("line 1"),
			ErrorBlock("warn", "ignored"),
			TextBlock("line 2"),
		},
	}
	text := out.Text()
	if text != "line 1\nline 2" {
		t.Fatalf("expected %q, got %q", "line 1\nline 2", text)
	}
}

func TestContentBlock_ToLegacyResult(t *testing.T) {
	out := &ToolOutput{
		Content:   []ContentBlock{TextBlock("result data")},
		ElapsedMs: 42,
		ExitCode:  0,
	}

	lr := out.ToLegacyResult("bash")
	if lr.Tool != "bash" {
		t.Fatalf("expected tool %q, got %q", "bash", lr.Tool)
	}
	if lr.Status != "success" {
		t.Fatalf("expected status %q, got %q", "success", lr.Status)
	}
	if lr.Result != "result data" {
		t.Fatalf("expected result %q, got %q", "result data", lr.Result)
	}
	if lr.ElapsedMs != 42 {
		t.Fatalf("expected elapsed 42, got %d", lr.ElapsedMs)
	}

	// Error case
	errOut := &ToolOutput{
		Content: []ContentBlock{ErrorBlock("timeout", "timed out")},
	}
	elr := errOut.ToLegacyResult("bash")
	if elr.Status != "error" {
		t.Fatalf("expected status %q, got %q", "error", elr.Status)
	}
	if elr.ErrorType != "timeout" {
		t.Fatalf("expected error_type %q, got %q", "timeout", elr.ErrorType)
	}
}

// ---------------------------------------------------------------------------
// Tool factory tests
// ---------------------------------------------------------------------------

func TestNewReadTool_LocalFile(t *testing.T) {
	dir := t.TempDir()
	content := "line1\nline2\nline3\nline4\nline5\n"
	if err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	tool := NewReadTool(BuiltinToolOptions{Cwd: dir})
	if tool.Name() != "read" {
		t.Fatalf("expected name %q, got %q", "read", tool.Name())
	}

	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": "test.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.HasError() {
		t.Fatalf("unexpected error: %s", out.Text())
	}
	if !strings.Contains(out.Text(), "line1") {
		t.Fatalf("expected output to contain 'line1', got: %s", out.Text())
	}
	if !strings.Contains(out.Text(), "line5") {
		t.Fatalf("expected output to contain 'line5', got: %s", out.Text())
	}
}

func TestNewReadTool_WithOffsetLimit(t *testing.T) {
	dir := t.TempDir()
	var lines []string
	for i := 1; i <= 10; i++ {
		lines = append(lines, "line"+string(rune('0'+i)))
	}
	// Create file with lines "lineA" through "lineJ" for clearer testing
	content := "lineA\nlineB\nlineC\nlineD\nlineE\nlineF\nlineG\nlineH\nlineI\nlineJ\n"
	if err := os.WriteFile(filepath.Join(dir, "test.txt"), []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	tool := NewReadTool(BuiltinToolOptions{Cwd: dir})
	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":   "test.txt",
		"offset": float64(3),
		"limit":  float64(2),
	})
	if err != nil {
		t.Fatal(err)
	}
	text := out.Text()
	if !strings.Contains(text, "lineC") {
		t.Fatalf("expected 'lineC' in output (offset=3), got: %s", text)
	}
	if !strings.Contains(text, "lineD") {
		t.Fatalf("expected 'lineD' in output (limit=2), got: %s", text)
	}
	if strings.Contains(text, "lineE") {
		t.Fatalf("did not expect 'lineE' (limit=2), got: %s", text)
	}
}

func TestNewReadTool_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	tool := NewReadTool(BuiltinToolOptions{Cwd: dir})
	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"path": "nonexistent.txt",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.HasError() {
		t.Fatal("expected error for missing file")
	}
}

func TestNewWriteTool(t *testing.T) {
	dir := t.TempDir()
	tool := NewWriteTool(BuiltinToolOptions{Cwd: dir})
	if tool.Name() != "write" {
		t.Fatalf("expected name %q, got %q", "write", tool.Name())
	}

	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":    "subdir/output.txt",
		"content": "hello from write tool",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.HasError() {
		t.Fatalf("unexpected error: %s", out.Text())
	}

	// Verify file was written
	data, err := os.ReadFile(filepath.Join(dir, "subdir", "output.txt"))
	if err != nil {
		t.Fatalf("failed to read written file: %v", err)
	}
	if string(data) != "hello from write tool" {
		t.Fatalf("expected %q, got %q", "hello from write tool", string(data))
	}
}

func TestNewEditTool(t *testing.T) {
	dir := t.TempDir()
	original := "hello world\nfoo bar\nbaz qux\n"
	if err := os.WriteFile(filepath.Join(dir, "edit.txt"), []byte(original), 0644); err != nil {
		t.Fatal(err)
	}

	tool := NewEditTool(BuiltinToolOptions{Cwd: dir})
	if tool.Name() != "edit" {
		t.Fatalf("expected name %q, got %q", "edit", tool.Name())
	}

	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":       "edit.txt",
		"old_string": "foo bar",
		"new_string": "FOO BAR REPLACED",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.HasError() {
		t.Fatalf("unexpected error: %s", out.Text())
	}

	// Verify
	data, err := os.ReadFile(filepath.Join(dir, "edit.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "FOO BAR REPLACED") {
		t.Fatalf("expected edited content, got: %s", string(data))
	}
	if strings.Contains(string(data), "foo bar") {
		t.Fatal("original text should have been replaced")
	}
}

func TestNewEditTool_NotFound(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "edit.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	tool := NewEditTool(BuiltinToolOptions{Cwd: dir})
	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"path":       "edit.txt",
		"old_string": "nonexistent string",
		"new_string": "replacement",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !out.HasError() {
		t.Fatal("expected error when old_string not found")
	}
}

func TestNewBashTool(t *testing.T) {
	dir := t.TempDir()
	tool := NewBashTool(BuiltinToolOptions{Cwd: dir})
	if tool.Name() != "bash" {
		t.Fatalf("expected name %q, got %q", "bash", tool.Name())
	}

	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"command": "echo 'hello from bash'",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.HasError() {
		t.Fatalf("unexpected error: %s", out.Text())
	}
	if !strings.Contains(out.Text(), "hello from bash") {
		t.Fatalf("expected 'hello from bash', got: %s", out.Text())
	}
}

func TestNewBashTool_ExitCode(t *testing.T) {
	tool := NewBashTool(BuiltinToolOptions{Cwd: t.TempDir()})
	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"command": "exit 42",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.ExitCode != 42 {
		t.Fatalf("expected exit code 42, got %d", out.ExitCode)
	}
}

func TestNewGrepTool(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "search.txt"), []byte("alpha\nbeta\ngamma\ndelta\n"), 0644); err != nil {
		t.Fatal(err)
	}

	tool := NewGrepTool(BuiltinToolOptions{Cwd: dir})
	if tool.Name() != "grep" {
		t.Fatalf("expected name %q, got %q", "grep", tool.Name())
	}

	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "beta",
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.HasError() {
		t.Fatalf("unexpected error: %s", out.Text())
	}
	if !strings.Contains(out.Text(), "beta") {
		t.Fatalf("expected grep output to contain 'beta', got: %s", out.Text())
	}
	if strings.Contains(out.Text(), "alpha") {
		t.Fatalf("did not expect 'alpha' in grep results, got: %s", out.Text())
	}
}

func TestNewGrepTool_NoMatch(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "search.txt"), []byte("alpha\nbeta\n"), 0644); err != nil {
		t.Fatal(err)
	}

	tool := NewGrepTool(BuiltinToolOptions{Cwd: dir})
	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "zzz_nonexistent",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.Text(), "No matches") {
		t.Fatalf("expected 'No matches' message, got: %s", out.Text())
	}
}

func TestNewFindTool(t *testing.T) {
	dir := t.TempDir()
	// Create some test files
	if err := os.WriteFile(filepath.Join(dir, "readme.md"), []byte("hi"), 0644); err != nil {
		t.Fatal(err)
	}
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "notes.md"), []byte("notes"), 0644); err != nil {
		t.Fatal(err)
	}

	tool := NewFindTool(BuiltinToolOptions{Cwd: dir})
	out, err := tool.Execute(context.Background(), map[string]interface{}{
		"pattern": "*.md",
		"path":    dir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if out.HasError() {
		t.Fatalf("unexpected error: %s", out.Text())
	}
	text := out.Text()
	if !strings.Contains(text, "readme.md") {
		t.Fatalf("expected 'readme.md' in find results, got: %s", text)
	}
	if !strings.Contains(text, "notes.md") {
		t.Fatalf("expected 'notes.md' in find results, got: %s", text)
	}
}

// ---------------------------------------------------------------------------
// Tool group preset tests
// ---------------------------------------------------------------------------

func TestCodingTools(t *testing.T) {
	tools := CodingTools("/tmp")
	if len(tools) != 4 {
		t.Fatalf("expected 4 coding tools, got %d", len(tools))
	}

	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name()] = true
	}
	for _, expected := range []string{"read", "bash", "edit", "write"} {
		if !names[expected] {
			t.Fatalf("expected CodingTools to include %q", expected)
		}
	}
}

func TestReadOnlyTools(t *testing.T) {
	tools := ReadOnlyTools("/tmp")
	if len(tools) != 3 {
		t.Fatalf("expected 3 read-only tools, got %d", len(tools))
	}

	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name()] = true
	}
	for _, expected := range []string{"read", "grep", "find"} {
		if !names[expected] {
			t.Fatalf("expected ReadOnlyTools to include %q", expected)
		}
	}
	// Should NOT include write-capable tools
	for _, forbidden := range []string{"write", "edit", "bash"} {
		if names[forbidden] {
			t.Fatalf("ReadOnlyTools should NOT include %q", forbidden)
		}
	}
}

func TestAllTools(t *testing.T) {
	tools := AllTools("/tmp")
	if len(tools) != 6 {
		t.Fatalf("expected 6 tools, got %d", len(tools))
	}

	names := make(map[string]bool)
	for _, tool := range tools {
		names[tool.Name()] = true
	}
	for _, expected := range []string{"read", "bash", "edit", "write", "grep", "find"} {
		if !names[expected] {
			t.Fatalf("expected AllTools to include %q", expected)
		}
	}
}

func TestRemoteTools(t *testing.T) {
	tools := RemoteTools("/home/pi", "gpu-server", "10.0.0.44", "peter", "/path/to/key")
	if len(tools) != 4 {
		t.Fatalf("expected 4 remote tools, got %d", len(tools))
	}

	// Verify all tools use the remote backend
	for _, tool := range tools {
		if tool.backend.Name != "gpu-server" {
			t.Fatalf("expected backend name %q, got %q", "gpu-server", tool.backend.Name)
		}
	}
}

// ---------------------------------------------------------------------------
// ToolBackend tests
// ---------------------------------------------------------------------------

func TestLocalBackend(t *testing.T) {
	b := LocalBackend()
	if b.Name != "local" {
		t.Fatalf("expected name %q, got %q", "local", b.Name)
	}
	if b.File == nil {
		t.Fatal("expected File ops to be non-nil")
	}
	if b.Exec == nil {
		t.Fatal("expected Exec ops to be non-nil")
	}
}

func TestRemoteBackend(t *testing.T) {
	b := RemoteBackend("pi", "192.168.1.10", "admin", "/path/to/key")
	if b.Name != "pi" {
		t.Fatalf("expected name %q, got %q", "pi", b.Name)
	}

	// Should be SSHFileOps (which embeds LocalFileOps as stub)
	sshOps, ok := b.File.(*SSHFileOps)
	if !ok {
		t.Fatalf("expected SSHFileOps, got %T", b.File)
	}
	if sshOps.Host != "192.168.1.10" {
		t.Fatalf("expected host %q, got %q", "192.168.1.10", sshOps.Host)
	}
}

// ---------------------------------------------------------------------------
// LocalFileOps integration tests
// ---------------------------------------------------------------------------

func TestLocalFileOps_ReadWriteStat(t *testing.T) {
	dir := t.TempDir()
	ops := &LocalFileOps{}
	ctx := context.Background()

	// Write
	path := filepath.Join(dir, "test.txt")
	if err := ops.WriteFile(ctx, path, []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}

	// Read
	data, err := ops.ReadFile(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "hello" {
		t.Fatalf("expected %q, got %q", "hello", string(data))
	}

	// Stat
	info, err := ops.Stat(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size() != 5 {
		t.Fatalf("expected size 5, got %d", info.Size())
	}

	// Remove
	if err := ops.Remove(ctx, path); err != nil {
		t.Fatal(err)
	}
	_, err = ops.Stat(ctx, path)
	if !os.IsNotExist(err) {
		t.Fatalf("expected file to be removed, got err: %v", err)
	}
}

// ---------------------------------------------------------------------------
// LocalExecOps integration tests
// ---------------------------------------------------------------------------

func TestLocalExecOps_Run(t *testing.T) {
	ops := &LocalExecOps{}
	stdout, stderr, exitCode, err := ops.Run(context.Background(), "echo", []string{"hello"}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if strings.TrimSpace(stdout) != "hello" {
		t.Fatalf("expected stdout %q, got %q", "hello", stdout)
	}
	if stderr != "" {
		t.Fatalf("expected empty stderr, got %q", stderr)
	}
}

func TestLocalExecOps_NonZeroExit(t *testing.T) {
	ops := &LocalExecOps{}
	_, _, exitCode, err := ops.Run(context.Background(), "bash", []string{"-c", "exit 7"}, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if exitCode != 7 {
		t.Fatalf("expected exit code 7, got %d", exitCode)
	}
}

// ---------------------------------------------------------------------------
// builtinToSchema test
// ---------------------------------------------------------------------------

func TestBuiltinToSchema(t *testing.T) {
	tool := NewReadTool(BuiltinToolOptions{Cwd: "/tmp"})
	schema := builtinToSchema(tool)

	if schema.Name != "read" {
		t.Fatalf("expected schema name %q, got %q", "read", schema.Name)
	}
	if schema.Description == "" {
		t.Fatal("expected non-empty description")
	}
	if schema.Parameters == nil {
		t.Fatal("expected non-nil parameters")
	}
	if schema.EvoClawMeta.Binary != "builtin" {
		t.Fatalf("expected binary %q, got %q", "builtin", schema.EvoClawMeta.Binary)
	}
}
