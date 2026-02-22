# Changelog

All notable changes to EvoClaw will be documented in this file.

## [v0.4.0] — 2026-02-22

### Added
- **feat(rsi):** RSI (Recursive Self-Improvement) promoted to core primitive — `internal/rsi/` package with observer, analyzer, fixer, and loop components (86.5% test coverage, 18 tests)
- **feat(orchestrator):** Auto-feed RSI from `processWithAgent` and `executeToolCall` — every agent action now feeds the RSI loop automatically
- **docs:** ADR-005 — RSI Core Primitive architecture decision record

### Fixed
- **fix(toolloop):** Use loop's final response directly instead of redundant LLM call (was returning empty API responses)

### Previous (since v0.3.2)
- **feat(clawchain):** Auto-discovery DID registration module (ADR-003)

## [v0.3.2] — 2026-02-15

_See [GitHub release](https://github.com/clawinfra/evoclaw/releases/tag/v0.3.2)._

## [v0.3.0] — 2026-02-01

_See [GitHub release](https://github.com/clawinfra/evoclaw/releases/tag/v0.3.0)._

## [v0.2.0] — 2026-01-15

_See [GitHub release](https://github.com/clawinfra/evoclaw/releases/tag/v0.2.0)._

## [v0.1.0-beta] — 2025-12-01

_Initial beta release._
