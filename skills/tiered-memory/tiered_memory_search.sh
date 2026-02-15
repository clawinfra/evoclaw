#!/bin/bash
# Tiered memory search wrapper for agent integration
# Pure EvoClaw design - LLM tree reasoning, no embeddings required

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
MEMORY_CLI="$SCRIPT_DIR/scripts/memory_cli.py"

# Auto-detect local LLM endpoint
LLM_ENDPOINT=""
for endpoint in \
    "http://localhost:8080/v1/chat/completions" \
    "http://localhost:11434/v1/chat/completions" \
    "http://localhost:1234/v1/chat/completions"
do
    if curl -sf "$endpoint" -m 1 >/dev/null 2>&1 || \
       curl -sf "${endpoint/\/chat\/completions/\/models}" -m 1 >/dev/null 2>&1; then
        LLM_ENDPOINT="$endpoint"
        break
    fi
done

# Run search with LLM tree reasoning (fallback to keyword → grep)
python3 "$MEMORY_CLI" retrieve \
    --query "$1" \
    --limit "${2:-5}" \
    --llm-endpoint "$LLM_ENDPOINT" \
    2>&1

# Exit with retrieve's exit code
exit $?
