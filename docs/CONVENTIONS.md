# Conventions — EvoClaw

## Package Naming

**Format:** short, lowercase, no underscores (Go standard).

```
✅  orchestrator
✅  skillbank
✅  agents
✅  genome
✅  interfaces

❌  AgentOrchestrator   (PascalCase)
❌  skill_bank          (underscore)
❌  orchestratorpkg     (redundant suffix)
```

One package per directory. Package name matches the directory name (almost always).

---

## File Naming

**Format:** `snake_case.go`

```
✅  orchestrator.go
✅  skill_bank.go
✅  types.go
✅  orchestrator_test.go
✅  mock_provider.go

❌  Orchestrator.go     (PascalCase)
❌  orchestratorFile.go (camelCase)
```

Test files: `<name>_test.go`. Mock files: `mock_<interface>.go`.

---

## Type and Function Naming

Follow standard Go conventions:

```go
// Exported types: PascalCase
type Orchestrator struct { ... }
type SkillBank struct { ... }
type CompletionRequest struct { ... }

// Unexported types: camelCase
type sessionState struct { ... }
type toolResult struct { ... }

// Exported functions: PascalCase verb-noun
func NewOrchestrator(...) *Orchestrator
func (o *Orchestrator) RunSession(ctx context.Context) error
func (sb *SkillBank) DistillTrajectories(ctx context.Context) error

// Unexported functions: camelCase
func (o *Orchestrator) buildPrompt() string
func parseToolCall(raw json.RawMessage) (*ToolCall, error)

// Constants: PascalCase for exported, camelCase for unexported
const DefaultTimeout = 30 * time.Second
const maxRetries = 3
```

---

## Godoc Comments

**All exported symbols must have godoc comments.** This is enforced by `scripts/agent-lint.sh`.

```go
// ✅ CORRECT

// Orchestrator manages agent sessions and coordinates LLM-tool loops.
// It is safe for concurrent use — each session runs in an isolated goroutine.
type Orchestrator struct { ... }

// NewOrchestrator creates an Orchestrator with the given provider and dependencies.
// provider must not be nil. If memory is nil, sessions run without memory persistence.
func NewOrchestrator(provider interfaces.Provider, memory interfaces.Memory) *Orchestrator { ... }

// RunSession executes a single agent session until completion or context cancellation.
// It returns the session result and any error. ErrContextCancelled is returned if ctx
// is cancelled before the session completes.
func (o *Orchestrator) RunSession(ctx context.Context, req SessionRequest) (SessionResult, error) { ... }

// ❌ WRONG — no comment on exported type
type SkillBank struct { ... }

// ❌ WRONG — comment doesn't start with the function name
// This creates a new session.
func NewSession(...) *Session { ... }
```

---

## Interface Design

Define interfaces in the package that **uses** them, not the package that implements them.
Exception: `internal/interfaces` holds truly shared interfaces used by ≥3 packages.

```go
// ✅ CORRECT — small, focused interfaces (Go proverb: accept interfaces, return structs)
type Provider interface {
    Complete(ctx context.Context, req CompletionRequest) (CompletionResponse, error)
}

// ❌ WRONG — fat interface (hard to mock, violates ISP)
type Provider interface {
    Complete(...)
    Stream(...)
    ListModels(...)
    GetUsage(...)
    SetAPIKey(...)
    // ...20 more methods
}
```

Interfaces should have 1–3 methods. If you need more, consider splitting.

---

## Error Types

```go
// Sentinel errors for well-known conditions (package-level, unexported or exported)
var ErrNotFound = errors.New("not found")
var ErrSessionExpired = errors.New("session expired")

// Structured errors for errors with context
type ValidationError struct {
    Field   string
    Message string
}

func (e *ValidationError) Error() string {
    return fmt.Sprintf("validation error: %s: %s", e.Field, e.Message)
}

// Wrapping for call-site context
return fmt.Errorf("orchestrator: run session %s: %w", sessionID, err)
```

Error strings: lowercase, no trailing punctuation, no "Error:" prefix.

---

## Context Propagation

```go
// ✅ CORRECT — ctx is always first parameter
func (s *SkillBank) Distill(ctx context.Context, trajectories []Trajectory) ([]Skill, error)
func (o *Orchestrator) RunSession(ctx context.Context, req Request) (Response, error)

// ❌ WRONG — ctx not first, or missing
func (s *SkillBank) Distill(trajectories []Trajectory) ([]Skill, error)
func (o *Orchestrator) RunSession(req Request, ctx context.Context) (Response, error)
```

Never store `context.Context` in a struct. Always pass it as a parameter.

---

## Python Scripts (skills/, scripts/)

- Always use `uv run python` — never `python3` or `python` directly
- All scripts use `argparse` and support `--help`
- All scripts exit with code 0 on success, non-zero on failure
- Scripts that make LLM calls must accept `--model` flag
- Scripts that write files must accept `--dry-run` flag

```python
#!/usr/bin/env python3
"""Brief description of what this script does."""
import argparse

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--repo", required=True, help="Path to repository root")
    parser.add_argument("--dry-run", action="store_true", help="Print changes without writing")
    args = parser.parse_args()
    # ...

if __name__ == "__main__":
    main()
```

---

## Commit Messages

Format: `<type>(<scope>): <description>`

```
feat(orchestrator): add parallel tool dispatch
fix(skillbank): handle empty trajectory batch
docs(architecture): update SKILLRL pipeline diagram
test(agents): add conversation compaction coverage
refactor(genome): extract mutation logic to genome.go
chore(ci): add race detector to CI
```

Types: `feat`, `fix`, `docs`, `test`, `refactor`, `chore`, `perf`
