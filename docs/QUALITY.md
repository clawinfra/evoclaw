# Quality Standards — EvoClaw

## Test Coverage

**Target: 90% per package** (enforced in CI for PRs that modify package code).

```bash
# Per-package coverage
go test -coverprofile=coverage.out ./internal/<package>/...
go tool cover -func=coverage.out | tail -1

# Full coverage report
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out -o coverage.html
```

### What to test in every package

- **Happy path:** function succeeds with valid inputs, returns expected output
- **Error paths:** all returned error values are reachable from at least one test
- **Boundary conditions:** empty slices, zero values, max-length inputs, context cancellation
- **Concurrency:** if the code uses goroutines or channels, run with `-race`

```bash
# Run with race detector (required for orchestrator and skillbank packages)
go test -race ./internal/orchestrator/... -timeout 60s
go test -race ./internal/skillbank/... -timeout 60s
```

---

## Interface Mocking

Use `testify/mock` for interface mocking. Do **not** use concrete implementations in tests.

```go
// mocks/mock_provider.go (or inline in _test.go files)
package mocks

import (
    "context"
    "github.com/stretchr/testify/mock"
    "github.com/clawinfra/evoclaw/internal/interfaces"
)

type MockProvider struct {
    mock.Mock
}

func (m *MockProvider) Complete(ctx context.Context, req interfaces.CompletionRequest) (interfaces.CompletionResponse, error) {
    args := m.Called(ctx, req)
    return args.Get(0).(interfaces.CompletionResponse), args.Error(1)
}
```

Usage in tests:
```go
func TestOrchestrator_RunsTool(t *testing.T) {
    mockProvider := new(mocks.MockProvider)
    mockProvider.On("Complete", mock.Anything, mock.Anything).
        Return(interfaces.CompletionResponse{Content: "result"}, nil)

    orch := orchestrator.New(mockProvider, nil, nil, nil)
    // ...
    mockProvider.AssertExpectations(t)
}
```

---

## Table-Driven Tests

Preferred for any function with ≥3 input/output cases.

```go
func TestRouter_Classify(t *testing.T) {
    tests := []struct {
        name     string
        task     string
        wantTier router.Tier
    }{
        {
            name:     "monitoring task → simple tier",
            task:     "check if service is healthy",
            wantTier: router.TierSimple,
        },
        {
            name:     "multi-file refactor → complex tier",
            task:     "refactor the orchestrator to add streaming support across 5 files",
            wantTier: router.TierComplex,
        },
        {
            name:     "security audit → critical tier",
            task:     "audit production auth system for vulnerabilities",
            wantTier: router.TierCritical,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            r := router.New()
            got, err := r.Classify(tt.task)
            require.NoError(t, err)
            assert.Equal(t, tt.wantTier, got.Tier)
        })
    }
}
```

---

## No Global State

**No package-level variables that are mutated at runtime.**

```go
// ❌ WRONG
var defaultProvider Provider   // shared state = test pollution
var mu sync.Mutex

func GetProvider() Provider {
    mu.Lock()
    defer mu.Unlock()
    return defaultProvider
}

// ✅ CORRECT — pass state through constructors
type Service struct {
    provider Provider
}

func NewService(p Provider) *Service {
    return &Service{provider: p}
}
```

Why: Global state makes tests order-dependent, causes race conditions, and prevents parallel test runs.

Exceptions (acceptable global state):
- `var ErrNotFound = errors.New("not found")` — immutable sentinel errors
- `var logger = slog.Default()` — logging (read-only after init)

---

## Error Handling

```go
// ✅ CORRECT — wrap errors with context
func (s *SkillBank) Distill(ctx context.Context, traj []Trajectory) ([]Skill, error) {
    resp, err := s.provider.Complete(ctx, req)
    if err != nil {
        return nil, fmt.Errorf("skillbank distill: %w", err)
    }
    // ...
}

// ❌ WRONG — naked error (loses context)
if err != nil {
    return nil, err
}

// ❌ WRONG — log and return (double-reports the error)
if err != nil {
    log.Printf("error: %v", err)
    return nil, err
}
```

Error messages should be lowercase and not end with punctuation (Go convention).

---

## Goroutine Lifecycle

Every goroutine must have a defined exit condition.

```go
// ✅ CORRECT — goroutine exits on context cancellation
func (s *Scheduler) Run(ctx context.Context) error {
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case task := <-s.queue:
            go s.execute(ctx, task)
        }
    }
}

// ❌ WRONG — goroutine with no exit
go func() {
    for task := range s.queue {
        s.execute(task)  // what stops this?
    }
}()
```

---

## CI Gates Summary

| Gate | Command | Failure = block PR? |
|------|---------|---------------------|
| Build | `go build ./...` | Yes |
| Tests | `go test ./... -count=1` | Yes |
| Vet | `go vet ./...` | Yes |
| Agent lints | `bash scripts/agent-lint.sh` | Yes |
| Race detector | `go test -race ./internal/orchestrator/... ./internal/skillbank/...` | Yes |
| Coverage | `go test -coverprofile=... ./...` (≥90%) | Yes (package PRs) |
| Lint (golangci) | `golangci-lint run ./...` | Yes |
