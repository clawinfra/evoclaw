#!/usr/bin/env bash
# build-wasm.sh — Build EvoClaw as a WebAssembly module
#
# Usage:
#   bash scripts/build-wasm.sh [output-dir]
#
# Output (default: dist/):
#   dist/evoclaw.wasm    — WASM binary
#   dist/wasm_exec.js    — Go WASM runtime shim
#
# Requirements:
#   - Go 1.21+
#   - GOROOT must point to your Go installation

set -euo pipefail

DIST="${1:-dist}"
MODULE="github.com/clawinfra/evoclaw/internal/platform/wasm/cmd"

echo "🔨 Building EvoClaw WASM..."
echo "   Output dir: $DIST"
echo "   Module:     $MODULE"

mkdir -p "$DIST"

# Compile to WASM
GOOS=js GOARCH=wasm go build \
  -ldflags="-s -w" \
  -o "$DIST/evoclaw.wasm" \
  "./$MODULE/" 2>/dev/null || {
    # Module may not exist yet — build from wasm package itself
    GOOS=js GOARCH=wasm go build \
      -ldflags="-s -w" \
      -o "$DIST/evoclaw.wasm" \
      ./internal/platform/wasm/
  }

echo "   ✅ evoclaw.wasm ($(du -h "$DIST/evoclaw.wasm" | cut -f1))"

# Copy wasm_exec.js from Go installation
WASM_EXEC_JS="$(go env GOROOT)/misc/wasm/wasm_exec.js"
if [ -f "$WASM_EXEC_JS" ]; then
  cp "$WASM_EXEC_JS" "$DIST/wasm_exec.js"
  echo "   ✅ wasm_exec.js"
else
  echo "   ⚠️  wasm_exec.js not found at $WASM_EXEC_JS"
  echo "      Download from: https://raw.githubusercontent.com/golang/go/master/misc/wasm/wasm_exec.js"
fi

echo ""
echo "✅ WASM build complete. Files in $DIST/:"
ls -lh "$DIST/"
echo ""
echo "To serve locally:"
echo "  cd $DIST && python3 -m http.server 8080"
echo "  Open: http://localhost:8080/index.html"
echo ""
echo "Or copy examples/wasm/index.html to $DIST/ for a demo."
