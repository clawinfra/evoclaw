# ADR-005: Promote RSI to Core Primitive

## Status

Accepted

## Date

2026-02-22

## Context

The Recursive Self-Improvement (RSI) loop currently lives as an optional OpenClaw skill (`skills/rsi-loop/`). In its current form, it is passive — it only observes manually-fed outcomes. Recurring bugs (like the toolloop empty response bug that went undetected for days) slip through because nothing auto-feeds operational data into the RSI loop.

OpenClaw has no built-in RSI capability, and there is no indication they plan to build one. If EvoClaw is to be a truly self-evolving agent framework, RSI cannot remain optional — it must be a core primitive, on the same level as the orchestrator, memory, and governance systems.

Key observations:
- The toolloop empty response bug recurred 3+ times before manual detection
- Rate limit errors and model failures follow patterns that could be auto-detected
- Cross-source correlation (e.g., session_reset + context_loss) reveals compound issues invisible to single-source analysis
- Agents running RSI get better over time. Agents without it don't. This IS the evolution in "self-evolving agent framework."

## Decision

Promote RSI from an optional external skill to a core EvoClaw package at `internal/rsi/`, at the same architectural level as `internal/orchestrator/`, `internal/memory/`, and `internal/governance/`.

### Architecture

The RSI system consists of five components forming a closed loop: **observe → analyze → fix → verify → observe**.

| File | Responsibility |
|------|---------------|
| `internal/rsi/types.go` | Core types: Outcome, Pattern, Fix, Config |
| `internal/rsi/observer.go` | Auto-captures outcomes from every processWithAgent call, tool execution, cron tick, and edge agent response |
| `internal/rsi/analyzer.go` | Real-time pattern detection with error message similarity grouping and cross-source correlation |
| `internal/rsi/fixer.go` | Auto-applies safe fixes (routing config, thresholds, retry logic); queues proposals for unsafe ones |
| `internal/rsi/loop.go` | The closed loop: periodic analysis, auto-fix cycles, health score tracking |

### Outcome Sources

All operational data feeds the same RSI loop:

- **`openclaw`** — Gateway-level errors, routing failures, model timeouts
- **`evoclaw`** — Orchestrator actions, agent responses, evolution events
- **`cron`** — Scheduled job outcomes, missed ticks
- **`subagent`** — Sub-agent task completions, failures
- **`self_governance`** — WAL/VBR/ADL protocol violations
- **`operational`** — System-level issues (disk, memory, connectivity)

### Integration Points

- `processWithAgent()` → auto-records every LLM interaction
- `executeToolCall()` → auto-records every tool execution
- Scheduler ticks → auto-recorded via observer
- Edge agent responses → auto-recorded via observer

### Safety Model

Fixes are categorized as safe or unsafe:

- **Safe (auto-apply):** `routing_config`, `threshold_tuning`, `retry_logic`, `model_selection`
- **Unsafe (human review):** Everything else — written as proposals for human/agent review

## Consequences

### Positive

- Every orchestrator action automatically feeds RSI — no manual intervention needed
- Health score becomes a first-class metric available via API
- Recurring bugs are detected within the analysis window (default 1h) instead of days
- Safe-category fixes are applied automatically, reducing operational burden
- Cross-source pattern detection catches compound issues
- The framework genuinely self-improves over time

### Negative

- Additional CPU/memory overhead for outcome storage and periodic analysis
- JSONL outcome files grow over time (mitigated by MaxOutcomes config)
- Auto-fix for safe categories introduces a small risk of incorrect fixes (mitigated by verify step)

### Risks

- False positive pattern detection could trigger unnecessary fixes
- Error message similarity grouping may over-group unrelated errors
- Auto-fix requires careful safe-category boundaries to prevent unintended changes
