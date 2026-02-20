# Built-in Tool Architecture (Pi-Inspired)

> **Status:** Implemented  
> **Version:** 1.0  
> **Last Updated:** 2025-02-20

## Overview

EvoClaw's tool system now supports **built-in tools** alongside the existing TOML-based tool definitions. This architecture is inspired by [pi's](https://github.com/anthropics/anthropic-cli) tool system and brings three key patterns to the Go orchestrator:

1. **Operations injection** — pluggable backend interfaces (FileOps, ExecOps) decouple tool logic from execution environment
2. **Factory pattern** — `NewReadTool(opts)` creates tools configured for a specific working directory, backend, and limits
3. **Rich structured returns** — tools return `[]ContentBlock` (text, image, error) instead of raw strings
4. **Tool groups** — `CodingTools()`, `ReadOnlyTools()`, `AllTools()` preset collections

## Architecture

```
┌─────────────────────────────────────────────────────────┐
│                    ToolManager                          │
│  ┌──────────────┐  ┌──────────────────────────────┐    │
│  │ TOML Tools   │  │ Built-in Tools (pi-style)    │    │
│  │ (skill.toml) │  │ RegisterBuiltinTools(...)     │    │
│  └──────────────┘  └──────────────────────────────┘    │
│           │                    │                        │
│           └────────┬───────────┘                        │
│                    ▼                                    │
│         GenerateSchemasWithBuiltins()                   │
│         (built-in tools override TOML by name)          │
└─────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────┐
│                   BuiltinTool                           │
│  ┌─────────┐  ┌──────────┐  ┌──────────────────┐      │
│  │  Name    │  │  Schema  │  │  Execute(ctx,     │      │
│  │  Desc    │  │  (JSON)  │  │    args) →        │      │
│  └─────────┘  └──────────┘  │  *ToolOutput      │      │
│                              └────────┬─────────┘      │
│                                       │                 │
│                              ┌────────▼─────────┐      │
│                              │   ToolBackend     │      │
│                              │  ┌─────────────┐  │      │
│                              │  │  FileOps    │  │      │
│                              │  │  ExecOps    │  │      │
│                              │  └─────────────┘  │      │
│                              └──────────────────┘      │
└─────────────────────────────────────────────────────────┘
```

## Operations Injection (FileOps, ExecOps)

Every tool receives a `ToolBackend` that abstracts file system and command execution operations. This allows the same tool logic to target different environments.

### Interfaces

```go
// FileOps — pluggable file system operations
type FileOps interface {
    ReadFile(ctx context.Context, path string) ([]byte, error)
    WriteFile(ctx context.Context, path string, data []byte, perm os.FileMode) error
    Stat(ctx context.Context, path string) (os.FileInfo, error)
    ReadDir(ctx context.Context, path string) ([]fs.DirEntry, error)
    MkdirAll(ctx context.Context, path string, perm os.FileMode) error
    Remove(ctx context.Context, path string) error
}

// ExecOps — pluggable command execution
type ExecOps interface {
    Run(ctx context.Context, cmd string, args []string, env []string, workdir string) (
        stdout string, stderr string, exitCode int, err error)
}
```

### Implementations

| Implementation | Target | Status |
|---------------|--------|--------|
| `LocalFileOps` | Local filesystem | ✅ Complete |
| `LocalExecOps` | Local subprocess | ✅ Complete |
| `SSHFileOps` | Remote via SSH | 🔲 Stub (embeds LocalFileOps, needs golang.org/x/crypto) |
| `SSHExecOps` | Remote exec via SSH | 🔲 Planned |

### ToolBackend

```go
// ToolBackend bundles FileOps + ExecOps for a target environment
type ToolBackend struct {
    File FileOps
    Exec ExecOps
    Name string  // "local", "gpu-server", "pi-edge"
}

// Create backends
local := LocalBackend()
remote := RemoteBackend("gpu-server", "10.0.0.44", "peter", "~/.ssh/id_ed25519")
```

## Factory Pattern

Tools are created via factory functions, each accepting `BuiltinToolOptions`:

```go
type BuiltinToolOptions struct {
    Cwd          string         // Working directory for relative paths
    Backend      *ToolBackend   // Defaults to LocalBackend()
    MaxReadBytes int64          // Default: 512KB
    MaxReadLines int            // Default: 2000
    BashTimeout  time.Duration  // Default: 30s
}
```

### Available Tools

| Factory | Tool Name | Description |
|---------|-----------|-------------|
| `NewReadTool(opts)` | `read` | Read files (text with limits, images as base64) |
| `NewWriteTool(opts)` | `write` | Write content, auto-create parent dirs |
| `NewEditTool(opts)` | `edit` | Exact string replacement in files |
| `NewBashTool(opts)` | `bash` | Execute shell commands with timeout |
| `NewGrepTool(opts)` | `grep` | Search file contents with regex |
| `NewFindTool(opts)` | `find` | Find files by name pattern |

### Tool Group Presets

```go
// Standard coding — read, bash, edit, write
tools := CodingTools("/home/user/project")

// Exploration only — read, grep, find
tools := ReadOnlyTools("/home/user/project")

// Everything — all 6 tools
tools := AllTools("/home/user/project")

// Target a remote machine
tools := RemoteTools("/home/pi", "gpu-server", "10.0.0.44", "peter", "~/.ssh/key")
```

## ContentBlock Rich Returns

Tools return `*ToolOutput` containing `[]ContentBlock` instead of raw strings:

```go
type ContentBlock struct {
    Kind     ContentKind  // "text", "image", "error"
    Text     string       // Text content or error message
    MimeType string       // Image MIME type (e.g., "image/png")
    Data     string       // Base64-encoded image data
    ErrCode  string       // Machine-readable error code
}

type ToolOutput struct {
    Content   []ContentBlock
    ElapsedMs int64
    ExitCode  int
}
```

### Constructors

```go
TextBlock("file contents here")
ImageBlock("image/png", base64Data)
ErrorBlock("not_found", "file does not exist")
```

### Backward Compatibility

`ToolOutput` can be converted to the legacy `ToolResult` format:

```go
output := tool.Execute(ctx, args)
legacyResult := output.ToLegacyResult("bash")
// legacyResult.Tool = "bash"
// legacyResult.Status = "success" | "error"
// legacyResult.Result = concatenated text blocks
```

## Targeting Remote Backends

### GPU Server Example

```go
// Create tools targeting the GPU server
gpuTools := RemoteTools(
    "/data/ai-stack",        // working directory on remote
    "gpu-server",            // backend name for logging
    "10.0.0.44",             // host
    "peter",                 // user
    "~/.ssh/id_ed25519",     // SSH key path
)

// Register with ToolManager
toolManager.RegisterBuiltinTools(gpuTools)

// Tools now use SSHFileOps for file operations
// (currently falls back to local — SSH impl is a TODO)
```

### Custom Configuration

```go
opts := BuiltinToolOptions{
    Cwd:          "/data/models",
    Backend:      RemoteBackend("gpu", "10.0.0.44", "peter", key),
    MaxReadBytes: 1024 * 1024,     // 1MB read limit
    MaxReadLines: 5000,             // 5000 line limit
    BashTimeout:  2 * time.Minute, // Long timeout for training
}

tools := CustomTools(opts)
toolManager.RegisterBuiltinTools(tools)
```

## Integration with ToolManager

Built-in tools integrate with the existing TOML-based ToolManager:

```go
// 1. Create ToolManager (existing pattern)
tm := NewToolManager(skillsPath, capabilities, logger)

// 2. Register built-in tools (new)
tm.RegisterBuiltinTools(CodingTools("/home/user/project"))

// 3. Generate schemas for LLM (new method)
schemas, _ := tm.GenerateSchemasWithBuiltins()
// Built-in tools override TOML tools with the same name

// 4. Look up a built-in tool for direct execution
if tool := tm.GetBuiltinTool("read"); tool != nil {
    output, _ := tool.Execute(ctx, args)
}
```

## File Layout

```
internal/orchestrator/
├── tool_ops.go           # FileOps, ExecOps interfaces + implementations
├── content_block.go      # ContentBlock, ToolOutput types
├── tool_factory.go       # BuiltinTool, factory functions, presets
├── tool_factory_test.go  # Tests for all of the above
├── tools.go              # ToolManager (TOML) + RegisterBuiltinTools
└── toolloop.go           # ToolLoop (unchanged, uses legacy ToolResult)
```

## Future Work

- **SSH backend**: Implement real `SSHFileOps` and `SSHExecOps` using `golang.org/x/crypto/ssh`
- **HTTP backend**: For tools targeting web APIs or cloud services
- **Tool loop integration**: Update `ToolLoop.Execute()` to use `BuiltinTool.Execute()` directly instead of MQTT for local tools
- **Streaming output**: `ContentBlock` stream for long-running tools
- **Tool composition**: Chain tools together (pipe read output into grep)

## See Also

- [AGENTIC-TOOL-LOOP.md](./AGENTIC-TOOL-LOOP.md) — Original tool loop design (MQTT-based edge agent flow)
- [SKILLS-SYSTEM.md](./SKILLS-SYSTEM.md) — TOML-based skill definitions
- [PI-INTEGRATION.md](./PI-INTEGRATION.md) — Raspberry Pi edge agent integration
