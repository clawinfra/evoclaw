# Security Model

EvoClaw enforces workspace sandboxing and security policies to restrict what tools can access during autonomous execution.

## Architecture

The security system consists of four components:

- **SecurityPolicy** (`internal/security/policy.go`) — Main policy engine that validates actions against configured rules
- **Path Sandbox** (`internal/security/sandbox.go`) — Validates file paths are within allowed boundaries, resolves symlinks, blocks traversal attacks
- **Command Validator** (`internal/security/command.go`) — Validates commands against an allowlist and blocks shell injection
- **Config** (`internal/security/config.go`) — TOML-based configuration for security settings

## Autonomy Levels

| Level | Read | Write | Execute | Delete |
|-------|------|-------|---------|--------|
| `readonly` | ✅ | ❌ | ❌ | ❌ |
| `supervised` | ✅ | ✅ | ✅ | ❌ |
| `full` | ✅ | ✅ | ✅ | ✅ |

## Path Sandboxing

When `workspace_only = true`, all file operations are restricted to:
1. The configured `workspace_path`
2. Any paths in `allowed_roots`

Additional protections:
- **Symlink resolution** — Symlinks are resolved via `filepath.EvalSymlinks` before boundary checks, preventing escape via symlinks pointing outside the workspace
- **Null byte injection** — Paths containing null bytes are rejected
- **Path traversal** — `../` sequences are resolved to absolute paths before checking
- **Forbidden paths** — Paths matching `forbidden_paths` prefixes (e.g., `/etc`, `~/.ssh`) are always blocked, even when `workspace_only = false`

## Command Allowlist

Only commands whose binary name appears in `allowed_commands` are permitted. Features:
- Path-qualified binaries are resolved (`/usr/bin/git` → `git`)
- Wildcard `"*"` disables the allowlist check
- Shell injection patterns are always blocked: `$(`, `` ` ``, `&&`, `||`, `;`, `|`, `>`, `<`, newlines

## Configuration

Add to your `agent.toml`:

```toml
[security.autonomy]
level = "supervised"
workspace_only = true
allowed_commands = ["git", "npm", "cargo", "ls", "cat", "grep", "find"]
forbidden_paths = ["/etc", "/root", "~/.ssh", "~/.gnupg", "~/.aws"]
allowed_roots = ["/tmp/shared"]

[security.sandbox]
workspace_path = "/home/user/project"
```

## Orchestrator Integration

The security policy is checked in the tool loop before every tool execution. If a tool call violates the policy, it returns an error result without executing the tool.

```go
orch := orchestrator.New(cfg, logger)
policy := security.NewSecurityPolicy(secCfg)
orch.SetSecurityPolicy(policy)
```
