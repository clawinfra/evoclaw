# ADR-006: Trait-Driven Interfaces

**Status:** Accepted  
**Date:** 2026-02-22  
**Author:** Alex Chen

## Context

EvoClaw's core subsystems (LLM providers, memory, tools, channels, observability) were tightly coupled to concrete implementations. Swapping a provider or memory backend required modifying orchestrator internals.

## Decision

We introduce formal Go interfaces in `internal/interfaces/` for all core subsystems:

| Interface       | File            | Purpose                          |
|----------------|-----------------|----------------------------------|
| `Provider`      | `provider.go`   | LLM provider abstraction         |
| `MemoryBackend` | `memory.go`     | Pluggable memory storage         |
| `Tool`          | `tool.go`       | Tool execution contract          |
| `ToolRegistry`  | `tool.go`       | Tool collection management       |
| `Channel`       | `channel.go`    | Messaging transport abstraction  |
| `Observer`      | `observer.go`   | Telemetry/observability hooks    |

Shared types are in `types.go`. All interfaces use `context.Context` for cancellation and include health checks where appropriate.

## Rationale

- **Swappability:** New providers, memory backends, or channels can be added without modifying core orchestrator code.
- **Testability:** Mock implementations satisfy interfaces for unit testing.
- **Composition:** Agents are composed from trait implementations selected via config, not hard-coded dependencies.
- **Gradual migration:** Existing code retains its concrete types. The legacy `orchestrator.ModelProvider` interface is preserved with a compatibility note. New code should target `interfaces.Provider`.

## Consequences

- All new subsystem implementations should implement the canonical interfaces.
- Existing concrete types (`orchestrator.ModelProvider`) remain for backward compatibility but are soft-deprecated.
- The plugin API docs (`docs/PLUGIN-API.md`) now document how to implement each interface.
- Future work: wire interface-based dependency injection into the orchestrator constructor.
