# Changelog

All notable changes to EvoClaw will be documented in this file.

## [v0.6.1] — 2026-03-05

### Added
- **feat(skillbank):** SKILLRL-inspired skillbank — Phases 1-3 (`internal/skillbank/`): FileStore (JSONL, thread-safe), distiller (LLM-based skill extraction), retriever (keyword+embedding), updater (recursive evolution, EMA confidence) — 92.9% coverage, 40+ tests (#22)
- **feat(rsi):** Wire `skillbank.RecordTrajectory()` into RSI loop observer — every agent trajectory now feeds the skill bank for future distillation (#23)

### Fixed  
- **fix(ci):** Remove deprecated `version` field from `.golangci.yml` (golangci-lint v1.64+)
- **fix(lint):** Resolve all errcheck violations across 42 files — `_ =` suppression pattern applied consistently

### Security
- ClawChain audit: all 1 CRITICAL + 4 HIGH + 3 MEDIUM + 3 LOW findings resolved (see claw-chain repo)

## [v0.6.0] — 2026-02-28

### Added
- **feat(platform):** Phase 2 — Android, iOS, WASM platform support + ClawHub integration
- **fix(cloud):** Check error return from json.Decode in cloud CLI
- **fix(cloud):** Remove unused variable in cloud manager test
- **ci:** Fix Rust test assertions and Go lint issues
- **fix(rust):** Resolve Rust clippy errors from beta merge (OrderRequest, Signature)
- **ci:** Fix Rust compilation errors from partial beta merge

## [v0.5.0] — 2026-02-22

### Added
- **feat(interfaces):** Trait-driven interfaces — Provider, Memory, Tool, Channel, Observer formalized as Go interfaces (ADR-006) (#9)
- **feat(memory):** Hybrid search layer — SQLite FTS5 + vector with weighted merge, pure Go, zero CGO via modernc.org/sqlite (#10)
- **feat(security):** Workspace sandboxing — symlink escape detection, forbidden paths, command allowlists, autonomy levels (readonly/supervised/full) (#11)
- **feat(config):** SIGHUP hot-reload — hot-apply config changes without restart (#12)
- **feat(migrate):** OpenClaw migration tool — `evoclaw migrate openclaw` with dry-run support (#13)
- **docs:** ADR-006 (Trait-Driven Interfaces), SECURITY.md, MEMORY.md, MIGRATION.md, updated GATEWAY.md and PLUGIN-API.md

### Fixed
- **fix(rsi):** Remove unused outcomeGroup type (lint cleanup)

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
