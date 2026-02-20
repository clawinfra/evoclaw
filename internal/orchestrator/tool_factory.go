package orchestrator

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// BuiltinToolOptions configures built-in tool factories.
type BuiltinToolOptions struct {
	// Cwd is the working directory for relative path resolution.
	Cwd string

	// Backend selects which FileOps/ExecOps implementation to use.
	// Defaults to LocalBackend() if nil.
	Backend *ToolBackend

	// MaxReadBytes limits file read size. Default: 512KB.
	MaxReadBytes int64

	// MaxReadLines limits line count for text files. Default: 2000.
	MaxReadLines int

	// BashTimeout overrides default bash execution timeout.
	BashTimeout time.Duration
}

// defaults fills zero-valued options with sensible defaults.
func (o BuiltinToolOptions) defaults() BuiltinToolOptions {
	if o.Backend == nil {
		o.Backend = LocalBackend()
	}
	if o.MaxReadBytes == 0 {
		o.MaxReadBytes = 512 * 1024 // 512KB
	}
	if o.MaxReadLines == 0 {
		o.MaxReadLines = 2000
	}
	if o.BashTimeout == 0 {
		o.BashTimeout = 30 * time.Second
	}
	return o
}

// BuiltinTool is a self-describing, executable tool with pluggable backend.
type BuiltinTool struct {
	name        string
	description string
	schema      map[string]interface{} // JSON Schema for parameters
	backend     *ToolBackend
	opts        BuiltinToolOptions
	executeFunc func(ctx context.Context, args map[string]interface{}) (*ToolOutput, error)
}

// Name returns the tool name.
func (t *BuiltinTool) Name() string { return t.name }

// Description returns the tool description.
func (t *BuiltinTool) Description() string { return t.description }

// Schema returns the JSON Schema for the tool's parameters.
func (t *BuiltinTool) Schema() map[string]interface{} { return t.schema }

// Execute runs the tool with the given arguments using the configured backend.
func (t *BuiltinTool) Execute(ctx context.Context, args map[string]interface{}) (*ToolOutput, error) {
	return t.executeFunc(ctx, args)
}

// ---------------------------------------------------------------------------
// resolvePath resolves a potentially relative path against the tool's cwd.
// ---------------------------------------------------------------------------
func resolvePath(cwd, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(cwd, path)
}

// ---------------------------------------------------------------------------
// NewReadTool — read file contents (text with limits, binary as base64)
// ---------------------------------------------------------------------------

// NewReadTool creates a read tool with the given options.
func NewReadTool(opts BuiltinToolOptions) *BuiltinTool {
	opts = opts.defaults()

	t := &BuiltinTool{
		name:        "read",
		description: "Read the contents of a file. Text files are returned with optional line limits. Binary/image files are returned as base64.",
		schema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the file to read (relative to working directory or absolute)",
				},
				"offset": map[string]interface{}{
					"type":        "integer",
					"description": "Line number to start reading from (1-indexed)",
				},
				"limit": map[string]interface{}{
					"type":        "integer",
					"description": "Maximum number of lines to return",
				},
			},
			"required": []string{"path"},
		},
		backend: opts.Backend,
		opts:    opts,
	}

	t.executeFunc = func(ctx context.Context, args map[string]interface{}) (*ToolOutput, error) {
		start := time.Now()
		path, _ := args["path"].(string)
		if path == "" {
			return &ToolOutput{
				Content: []ContentBlock{ErrorBlock("invalid_params", "path is required")},
			}, nil
		}
		resolved := resolvePath(opts.Cwd, path)

		data, err := opts.Backend.File.ReadFile(ctx, resolved)
		if err != nil {
			return &ToolOutput{
				Content:   []ContentBlock{ErrorBlock("read_error", err.Error())},
				ElapsedMs: time.Since(start).Milliseconds(),
			}, nil
		}

		// Check if it's an image — return base64
		mime := http.DetectContentType(data)
		if strings.HasPrefix(mime, "image/") {
			encoded := base64.StdEncoding.EncodeToString(data)
			return &ToolOutput{
				Content:   []ContentBlock{ImageBlock(mime, encoded)},
				ElapsedMs: time.Since(start).Milliseconds(),
			}, nil
		}

		// Text file: apply offset and limit
		offset := 0
		if v, ok := args["offset"]; ok {
			if f, ok := v.(float64); ok {
				offset = int(f)
			}
		}
		limit := opts.MaxReadLines
		if v, ok := args["limit"]; ok {
			if f, ok := v.(float64); ok {
				limit = int(f)
			}
		}

		// Truncate by bytes first
		if int64(len(data)) > opts.MaxReadBytes {
			data = data[:opts.MaxReadBytes]
		}

		// Apply line offset/limit
		scanner := bufio.NewScanner(bytes.NewReader(data))
		var lines []string
		lineNum := 0
		for scanner.Scan() {
			lineNum++
			if offset > 0 && lineNum < offset {
				continue
			}
			lines = append(lines, scanner.Text())
			if len(lines) >= limit {
				break
			}
		}

		text := strings.Join(lines, "\n")
		return &ToolOutput{
			Content:   []ContentBlock{TextBlock(text)},
			ElapsedMs: time.Since(start).Milliseconds(),
		}, nil
	}

	return t
}

// ---------------------------------------------------------------------------
// NewWriteTool — write content to a file
// ---------------------------------------------------------------------------

// NewWriteTool creates a file write tool.
func NewWriteTool(opts BuiltinToolOptions) *BuiltinTool {
	opts = opts.defaults()

	t := &BuiltinTool{
		name:        "write",
		description: "Write content to a file. Creates the file if it doesn't exist, overwrites if it does. Automatically creates parent directories.",
		schema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the file to write",
				},
				"content": map[string]interface{}{
					"type":        "string",
					"description": "Content to write to the file",
				},
			},
			"required": []string{"path", "content"},
		},
		backend: opts.Backend,
		opts:    opts,
	}

	t.executeFunc = func(ctx context.Context, args map[string]interface{}) (*ToolOutput, error) {
		start := time.Now()
		path, _ := args["path"].(string)
		content, _ := args["content"].(string)
		if path == "" {
			return &ToolOutput{
				Content: []ContentBlock{ErrorBlock("invalid_params", "path is required")},
			}, nil
		}
		resolved := resolvePath(opts.Cwd, path)

		// Ensure parent directory exists
		dir := filepath.Dir(resolved)
		if err := opts.Backend.File.MkdirAll(ctx, dir, 0755); err != nil {
			return &ToolOutput{
				Content:   []ContentBlock{ErrorBlock("mkdir_error", err.Error())},
				ElapsedMs: time.Since(start).Milliseconds(),
			}, nil
		}

		if err := opts.Backend.File.WriteFile(ctx, resolved, []byte(content), 0644); err != nil {
			return &ToolOutput{
				Content:   []ContentBlock{ErrorBlock("write_error", err.Error())},
				ElapsedMs: time.Since(start).Milliseconds(),
			}, nil
		}

		return &ToolOutput{
			Content:   []ContentBlock{TextBlock(fmt.Sprintf("Successfully wrote %d bytes to %s", len(content), path))},
			ElapsedMs: time.Since(start).Milliseconds(),
		}, nil
	}

	return t
}

// ---------------------------------------------------------------------------
// NewEditTool — exact string replacement in a file
// ---------------------------------------------------------------------------

// NewEditTool creates a file edit tool (exact string replacement).
func NewEditTool(opts BuiltinToolOptions) *BuiltinTool {
	opts = opts.defaults()

	t := &BuiltinTool{
		name:        "edit",
		description: "Edit a file by replacing exact text. The old_string must match exactly (including whitespace).",
		schema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Path to the file to edit",
				},
				"old_string": map[string]interface{}{
					"type":        "string",
					"description": "Exact text to find and replace",
				},
				"new_string": map[string]interface{}{
					"type":        "string",
					"description": "New text to replace the old text with",
				},
			},
			"required": []string{"path", "old_string", "new_string"},
		},
		backend: opts.Backend,
		opts:    opts,
	}

	t.executeFunc = func(ctx context.Context, args map[string]interface{}) (*ToolOutput, error) {
		start := time.Now()
		path, _ := args["path"].(string)
		oldStr, _ := args["old_string"].(string)
		newStr, _ := args["new_string"].(string)

		if path == "" || oldStr == "" {
			return &ToolOutput{
				Content: []ContentBlock{ErrorBlock("invalid_params", "path and old_string are required")},
			}, nil
		}
		resolved := resolvePath(opts.Cwd, path)

		data, err := opts.Backend.File.ReadFile(ctx, resolved)
		if err != nil {
			return &ToolOutput{
				Content:   []ContentBlock{ErrorBlock("read_error", err.Error())},
				ElapsedMs: time.Since(start).Milliseconds(),
			}, nil
		}

		content := string(data)
		if !strings.Contains(content, oldStr) {
			return &ToolOutput{
				Content:   []ContentBlock{ErrorBlock("not_found", "old_string not found in file")},
				ElapsedMs: time.Since(start).Milliseconds(),
			}, nil
		}

		// Replace first occurrence only
		newContent := strings.Replace(content, oldStr, newStr, 1)

		if err := opts.Backend.File.WriteFile(ctx, resolved, []byte(newContent), 0644); err != nil {
			return &ToolOutput{
				Content:   []ContentBlock{ErrorBlock("write_error", err.Error())},
				ElapsedMs: time.Since(start).Milliseconds(),
			}, nil
		}

		return &ToolOutput{
			Content:   []ContentBlock{TextBlock(fmt.Sprintf("Successfully edited %s", path))},
			ElapsedMs: time.Since(start).Milliseconds(),
		}, nil
	}

	return t
}

// ---------------------------------------------------------------------------
// NewBashTool — execute shell commands
// ---------------------------------------------------------------------------

// NewBashTool creates a bash execution tool.
func NewBashTool(opts BuiltinToolOptions) *BuiltinTool {
	opts = opts.defaults()

	t := &BuiltinTool{
		name:        "bash",
		description: "Execute a shell command using bash. Returns stdout, stderr, and exit code.",
		schema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"command": map[string]interface{}{
					"type":        "string",
					"description": "Shell command to execute",
				},
				"timeout_ms": map[string]interface{}{
					"type":        "integer",
					"description": "Timeout in milliseconds (default: 30000)",
				},
			},
			"required": []string{"command"},
		},
		backend: opts.Backend,
		opts:    opts,
	}

	t.executeFunc = func(ctx context.Context, args map[string]interface{}) (*ToolOutput, error) {
		start := time.Now()
		command, _ := args["command"].(string)
		if command == "" {
			return &ToolOutput{
				Content: []ContentBlock{ErrorBlock("invalid_params", "command is required")},
			}, nil
		}

		timeout := opts.BashTimeout
		if v, ok := args["timeout_ms"]; ok {
			if f, ok := v.(float64); ok {
				timeout = time.Duration(f) * time.Millisecond
			}
		}

		ctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()

		stdout, stderr, exitCode, err := opts.Backend.Exec.Run(ctx, "bash", []string{"-c", command}, nil, opts.Cwd)
		if err != nil {
			return &ToolOutput{
				Content:   []ContentBlock{ErrorBlock("exec_error", err.Error())},
				ElapsedMs: time.Since(start).Milliseconds(),
			}, nil
		}

		var blocks []ContentBlock
		if stdout != "" {
			blocks = append(blocks, TextBlock(stdout))
		}
		if stderr != "" {
			blocks = append(blocks, TextBlock("STDERR: "+stderr))
		}
		if len(blocks) == 0 {
			blocks = append(blocks, TextBlock("(no output)"))
		}

		return &ToolOutput{
			Content:   blocks,
			ElapsedMs: time.Since(start).Milliseconds(),
			ExitCode:  exitCode,
		}, nil
	}

	return t
}

// ---------------------------------------------------------------------------
// NewGrepTool — search file contents
// ---------------------------------------------------------------------------

// NewGrepTool creates a grep search tool.
func NewGrepTool(opts BuiltinToolOptions) *BuiltinTool {
	opts = opts.defaults()

	t := &BuiltinTool{
		name:        "grep",
		description: "Search for a pattern in files using grep. Returns matching lines with file names and line numbers.",
		schema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"pattern": map[string]interface{}{
					"type":        "string",
					"description": "Regular expression pattern to search for",
				},
				"path": map[string]interface{}{
					"type":        "string",
					"description": "File or directory to search in (default: working directory)",
				},
				"include": map[string]interface{}{
					"type":        "string",
					"description": "File glob pattern to include (e.g., '*.go')",
				},
			},
			"required": []string{"pattern"},
		},
		backend: opts.Backend,
		opts:    opts,
	}

	t.executeFunc = func(ctx context.Context, args map[string]interface{}) (*ToolOutput, error) {
		start := time.Now()
		pattern, _ := args["pattern"].(string)
		if pattern == "" {
			return &ToolOutput{
				Content: []ContentBlock{ErrorBlock("invalid_params", "pattern is required")},
			}, nil
		}

		searchPath := opts.Cwd
		if v, _ := args["path"].(string); v != "" {
			searchPath = resolvePath(opts.Cwd, v)
		}

		grepArgs := []string{"-rn", "--color=never"}
		if v, _ := args["include"].(string); v != "" {
			grepArgs = append(grepArgs, "--include="+v)
		}
		grepArgs = append(grepArgs, pattern, searchPath)

		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		stdout, _, exitCode, err := opts.Backend.Exec.Run(ctx, "grep", grepArgs, nil, opts.Cwd)
		if err != nil {
			return &ToolOutput{
				Content:   []ContentBlock{ErrorBlock("exec_error", err.Error())},
				ElapsedMs: time.Since(start).Milliseconds(),
			}, nil
		}

		if exitCode == 1 && stdout == "" {
			return &ToolOutput{
				Content:   []ContentBlock{TextBlock("No matches found")},
				ElapsedMs: time.Since(start).Milliseconds(),
				ExitCode:  exitCode,
			}, nil
		}

		// Truncate output if too long
		if len(stdout) > int(opts.MaxReadBytes) {
			stdout = stdout[:opts.MaxReadBytes] + "\n... (truncated)"
		}

		return &ToolOutput{
			Content:   []ContentBlock{TextBlock(stdout)},
			ElapsedMs: time.Since(start).Milliseconds(),
			ExitCode:  exitCode,
		}, nil
	}

	return t
}

// ---------------------------------------------------------------------------
// NewFindTool — find files by name/pattern
// ---------------------------------------------------------------------------

// NewFindTool creates a find files tool.
func NewFindTool(opts BuiltinToolOptions) *BuiltinTool {
	opts = opts.defaults()

	t := &BuiltinTool{
		name:        "find",
		description: "Find files and directories by name pattern. Uses the 'find' command under the hood.",
		schema: map[string]interface{}{
			"type": "object",
			"properties": map[string]interface{}{
				"path": map[string]interface{}{
					"type":        "string",
					"description": "Directory to search in (default: working directory)",
				},
				"pattern": map[string]interface{}{
					"type":        "string",
					"description": "File name pattern to match (e.g., '*.go', 'README*')",
				},
				"type": map[string]interface{}{
					"type":        "string",
					"description": "Type filter: 'f' for files, 'd' for directories",
				},
			},
			"required": []string{"pattern"},
		},
		backend: opts.Backend,
		opts:    opts,
	}

	t.executeFunc = func(ctx context.Context, args map[string]interface{}) (*ToolOutput, error) {
		start := time.Now()
		pattern, _ := args["pattern"].(string)
		if pattern == "" {
			return &ToolOutput{
				Content: []ContentBlock{ErrorBlock("invalid_params", "pattern is required")},
			}, nil
		}

		searchPath := opts.Cwd
		if v, _ := args["path"].(string); v != "" {
			searchPath = resolvePath(opts.Cwd, v)
		}

		findArgs := []string{searchPath, "-name", pattern}
		if v, _ := args["type"].(string); v != "" {
			findArgs = append(findArgs, "-type", v)
		}
		// Exclude common noise directories
		findArgs = append(findArgs, "-not", "-path", "*/.git/*",
			"-not", "-path", "*/node_modules/*",
			"-not", "-path", "*/__pycache__/*")

		ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		defer cancel()

		stdout, _, exitCode, err := opts.Backend.Exec.Run(ctx, "find", findArgs, nil, "")
		if err != nil {
			return &ToolOutput{
				Content:   []ContentBlock{ErrorBlock("exec_error", err.Error())},
				ElapsedMs: time.Since(start).Milliseconds(),
			}, nil
		}

		if stdout == "" {
			return &ToolOutput{
				Content:   []ContentBlock{TextBlock("No files found matching pattern: " + pattern)},
				ElapsedMs: time.Since(start).Milliseconds(),
				ExitCode:  exitCode,
			}, nil
		}

		// Truncate output if too long
		if len(stdout) > int(opts.MaxReadBytes) {
			stdout = stdout[:opts.MaxReadBytes] + "\n... (truncated)"
		}

		return &ToolOutput{
			Content:   []ContentBlock{TextBlock(stdout)},
			ElapsedMs: time.Since(start).Milliseconds(),
			ExitCode:  exitCode,
		}, nil
	}

	return t
}

// ---------------------------------------------------------------------------
// Tool group presets (pi-style)
// ---------------------------------------------------------------------------

// CodingTools returns the standard set for full read/write access.
func CodingTools(cwd string) []*BuiltinTool {
	opts := BuiltinToolOptions{Cwd: cwd}
	return []*BuiltinTool{NewReadTool(opts), NewBashTool(opts), NewEditTool(opts), NewWriteTool(opts)}
}

// ReadOnlyTools returns tools for exploration without modification.
func ReadOnlyTools(cwd string) []*BuiltinTool {
	opts := BuiltinToolOptions{Cwd: cwd}
	return []*BuiltinTool{NewReadTool(opts), NewGrepTool(opts), NewFindTool(opts)}
}

// AllTools returns all built-in tools.
func AllTools(cwd string) []*BuiltinTool {
	opts := BuiltinToolOptions{Cwd: cwd}
	return []*BuiltinTool{
		NewReadTool(opts),
		NewBashTool(opts),
		NewEditTool(opts),
		NewWriteTool(opts),
		NewGrepTool(opts),
		NewFindTool(opts),
	}
}

// RemoteTools returns coding tools targeting a named remote backend.
// Useful for calling tools on the GPU server or Raspberry Pi edge agents.
func RemoteTools(cwd, remoteName, host, user, keyPath string) []*BuiltinTool {
	backend := RemoteBackend(remoteName, host, user, keyPath)
	opts := BuiltinToolOptions{Cwd: cwd, Backend: backend}
	return []*BuiltinTool{NewReadTool(opts), NewBashTool(opts), NewGrepTool(opts), NewFindTool(opts)}
}

// CustomTools creates tools with fully custom options.
// Use this when you need fine-grained control over timeouts, limits, and backends.
func CustomTools(opts BuiltinToolOptions) []*BuiltinTool {
	return []*BuiltinTool{
		NewReadTool(opts),
		NewBashTool(opts),
		NewEditTool(opts),
		NewWriteTool(opts),
		NewGrepTool(opts),
		NewFindTool(opts),
	}
}

// Ensure unused imports are handled
var _ = os.PathSeparator
