#!/bin/bash
# Agent harness linter — errors are written to be agent-readable
# Every error message includes: what it is, how to fix it, which doc to consult.
set -euo pipefail

ERRORS=0
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$REPO_ROOT"

echo "=== EvoClaw Agent Lint ==="
echo "Repo: $REPO_ROOT"
echo ""

# ---------------------------------------------------------------------------
# Rule 1: No reverse dependency (internal/ → cmd/ forbidden)
# ---------------------------------------------------------------------------
echo "[1/5] Checking for reverse dependencies (internal → cmd)..."
REVERSE_DEPS=$(grep -rn '"github.com/clawinfra/evoclaw/cmd' internal/ 2>/dev/null || true)
if [ -n "$REVERSE_DEPS" ]; then
  echo ""
  echo "LINT ERROR [reverse-dependency]: internal/ package imports from cmd/"
  echo "$REVERSE_DEPS"
  echo ""
  echo "  WHAT: internal/ packages must not import from cmd/. This creates a reverse dependency"
  echo "        that breaks the cmd → internal → pkg layer rule and causes circular imports."
  echo "  FIX:  Move the shared logic to pkg/ so both cmd/ and internal/ can import it."
  echo "        Or inline the logic in internal/ without referencing cmd/."
  echo "  REF:  docs/ARCHITECTURE.md#layer-rules"
  ERRORS=$((ERRORS+1))
fi

# Rule 1b: No reverse dependency (pkg/ → internal/ forbidden)
PKG_REVERSE=$(grep -rn '"github.com/clawinfra/evoclaw/internal' pkg/ 2>/dev/null || true)
if [ -n "$PKG_REVERSE" ]; then
  echo ""
  echo "LINT ERROR [reverse-dependency]: pkg/ package imports from internal/"
  echo "$PKG_REVERSE"
  echo ""
  echo "  WHAT: pkg/ is the public API layer. It must not depend on internal/ packages."
  echo "        external consumers would inherit that transitive dependency."
  echo "  FIX:  Move the needed code from internal/ to pkg/, or define an interface in pkg/"
  echo "        that internal/ implements and passes in."
  echo "  REF:  docs/ARCHITECTURE.md#layer-rules"
  ERRORS=$((ERRORS+1))
fi

# ---------------------------------------------------------------------------
# Rule 2: All exported symbols must have godoc comments
# ---------------------------------------------------------------------------
echo "[2/5] Checking godoc coverage..."
# go vet checks for missing godoc on exported symbols
MISSING_DOCS=$(go vet ./... 2>&1 | grep -E "exported .* should have comment" || true)
if [ -n "$MISSING_DOCS" ]; then
  DOC_COUNT=$(echo "$MISSING_DOCS" | wc -l | tr -d ' ')
  echo ""
  echo "LINT ERROR [missing-godoc]: $DOC_COUNT exported symbol(s) missing godoc comments"
  echo "$MISSING_DOCS" | head -10
  if [ "$DOC_COUNT" -gt 10 ]; then
    echo "  ... ($(( DOC_COUNT - 10 )) more)"
  fi
  echo ""
  echo "  WHAT: All exported functions, types, methods, and constants require godoc comments."
  echo "        Agents and external consumers use godoc to understand APIs without reading code."
  echo "  FIX:  Add a comment starting with the symbol name above each exported declaration:"
  echo "        // MyFunction does X and returns Y."
  echo "        func MyFunction() ..."
  echo "  REF:  docs/CONVENTIONS.md#godoc"
  ERRORS=$((ERRORS+1))
fi

# ---------------------------------------------------------------------------
# Rule 3: No global mutable state in internal packages
# ---------------------------------------------------------------------------
echo "[3/5] Checking for global mutable state..."
# Look for package-level var declarations that are not constants or sentinel errors
GLOBAL_STATE=$(grep -rn "^var [A-Z]" internal/ 2>/dev/null \
  | grep -v "_test.go" \
  | grep -v "Err[A-Z]" \
  | grep -v "// " \
  | grep -v "embed.FS" \
  || true)
if [ -n "$GLOBAL_STATE" ]; then
  echo ""
  echo "LINT WARNING [global-state]: Found exported global variables in internal/ packages:"
  echo "$GLOBAL_STATE"
  echo ""
  echo "  WHAT: Package-level mutable state causes test pollution, race conditions, and"
  echo "        makes packages hard to use in parallel or multiple instances."
  echo "  FIX:  Move state into a struct. Pass it through the constructor:"
  echo "        type Service struct { state MyState }"
  echo "        func New(state MyState) *Service { return &Service{state: state} }"
  echo "  REF:  docs/QUALITY.md#no-global-state"
  echo "  NOTE: This is a warning, not an error. Sentinel errors (ErrXxx) are allowed."
  # Don't increment ERRORS — this is a warning
fi

# ---------------------------------------------------------------------------
# Rule 4: Run go build to catch compile errors
# ---------------------------------------------------------------------------
echo "[4/5] Running go build..."
if ! go build ./... 2>&1; then
  echo ""
  echo "LINT ERROR [build-failure]: go build ./... failed"
  echo ""
  echo "  WHAT: The codebase does not compile. No further checks are meaningful."
  echo "  FIX:  Run 'go build ./...' locally and fix all compile errors before pushing."
  echo "  REF:  Any compiler error message is self-explanatory."
  ERRORS=$((ERRORS+1))
fi

# ---------------------------------------------------------------------------
# Rule 5: AGENTS.md must stay under 150 lines
# ---------------------------------------------------------------------------
echo "[5/5] Checking AGENTS.md length..."
if [ -f "AGENTS.md" ]; then
  AGENTS_LINES=$(wc -l < AGENTS.md)
  if [ "$AGENTS_LINES" -gt 150 ]; then
    echo ""
    echo "LINT ERROR [agents-too-long]: AGENTS.md is $AGENTS_LINES lines (max 150)"
    echo "  WHAT: AGENTS.md is a table of contents, not a reference manual."
    echo "        Long AGENTS.md files burn agent context on navigation instead of work."
    echo "  FIX:  Move detailed content to docs/ and replace with a pointer in AGENTS.md."
    echo "        Example: '## Architecture → See docs/ARCHITECTURE.md'"
    echo "  REF:  AGENTS.md itself (table of contents philosophy)"
    ERRORS=$((ERRORS+1))
  fi
fi

# ---------------------------------------------------------------------------
# Summary
# ---------------------------------------------------------------------------
echo ""
echo "=== Lint complete: $ERRORS error(s) ==="
if [ $ERRORS -gt 0 ]; then
  echo ""
  echo "Fix all errors above before opening a PR."
  echo "Each error includes WHAT (the problem), FIX (how to resolve), REF (which doc to read)."
  exit 1
else
  echo "All checks passed. ✓"
fi
