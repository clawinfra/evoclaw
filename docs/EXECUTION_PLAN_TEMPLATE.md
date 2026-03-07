# Execution Plan: [Task Name]

## Status: [Planning | In Progress | Review | Complete]
## Complexity: [Low | Medium | High]
## Packages affected: [list internal/ packages]

---

## Goal

[One paragraph — what does done look like? Be concrete. Include: what feature/fix is delivered,
how it will be tested, and what CI gates will be green.]

---

## Steps

- [ ] Step 1: Read docs/ARCHITECTURE.md layer rules for affected packages
- [ ] Step 2: Identify which interfaces need to change (if any)
- [ ] Step 3: [describe implementation step]
- [ ] Step 4: [describe implementation step]
- [ ] Step 5: Write tests — table-driven, mock all external dependencies
- [ ] Step 6: Run `go test ./... -count=1` — must pass
- [ ] Step 7: Run `go test -race ./internal/<package>/...` — must pass
- [ ] Step 8: Run `bash scripts/agent-lint.sh` — must pass
- [ ] Step 9: Run `go vet ./...` — must pass
- [ ] Step 10: Check coverage: `go test -coverprofile=c.out ./internal/<package>/...` ≥ 90%
- [ ] Step 11: Update docs/ARCHITECTURE.md if package structure changed
- [ ] Step 12: Open PR with this plan linked in description

---

## Interface Changes

If this task requires changing an interface in `internal/interfaces/`:
1. Check which packages implement that interface (they all need updating)
2. Confirm the change is backwards compatible or document the breaking change
3. Update all mock implementations in test files

---

## Decisions

| Decision | Rationale | Date |
|----------|-----------|------|
| | | |

---

## Known Risks

- [List risks: cross-package changes, goroutine safety, LLM API changes, DB schema changes]

---

## Notes for Reviewer

[What should the reviewer focus on? Any tricky concurrency? Any deviations from convention?]

---

*Copy this template to `docs/plans/<task-slug>.md` before starting complex work.*
*Do not start coding until Steps 1+ are filled in and the interfaces are decided.*
