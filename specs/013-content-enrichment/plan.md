# Implementation Plan: Unified Mention Resolution System

**Branch**: `013-content-enrichment` | **Date**: 2026-01-21 | **Spec**: [mention-resolution.md](mention-resolution.md)
**Input**: Feature specification from `/specs/013-content-enrichment/mention-resolution.md`

## Summary

Implement an LLM-driven mention resolution system that extracts mentions from content and resolves them to canonical entities (persons, terms, products, companies, projects). The system uses a multi-stage pipeline: extraction + understanding → cross-mention reasoning → entity matching → verification. All resolution decisions are made by a configurable LLM (local MLX default, Claude API escalation). Complete audit tracing and model comparison capabilities enable debugging and evaluation.

## Technical Context

**Language/Version**: Go 1.22+
**Primary Dependencies**: Cobra CLI, gRPC, Protocol Buffers, pgx (PostgreSQL driver), MLX embeddings sidecar
**Storage**: PostgreSQL 16+ with existing schema (extending `pkg/mentions/`, `pkg/glossary/`)
**Testing**: go test with testify, integration tests against PostgreSQL
**Target Platform**: Linux/macOS server (dev01 Mac Mini M4)
**Project Type**: Single Go monorepo with pkg/ libraries and cmd/penf CLI
**Performance Goals**: <30 seconds per content item (configurable), batch processing
**Constraints**: Local MLX for privacy-sensitive processing, 2 retries then human review queue
**Scale/Scope**: Async processing, retention: 90 days full traces, 1 year decisions

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| I. Specification-First | ✅ PASS | Spec complete with clarifications |
| II. CLI + Library Architecture | ✅ PASS | `pkg/mentions/` library + `penf audit` CLI |
| III. Test-First Development | ✅ PASS | Tests required for all stages |
| IV. Integration Testing | ✅ PASS | Contract tests for LLM interface |
| V. Observability & Debugging | ✅ PASS | Full audit tracing system designed |
| VI. Versioning & Breaking Changes | ✅ PASS | Schema migrations versioned |
| VII. Simplicity & YAGNI | ✅ PASS | Phased implementation, minimal first |
| VIII. Temporal-First Data | ✅ PASS | All traces/decisions timestamped |
| IX. Research-Driven Design | ✅ PASS | LLM approach based on current AI best practices |
| X. Local-First AI with Privacy | ✅ PASS | MLX local default, Claude only for escalation |

**Gate Result**: PASS - Proceed to Phase 0

## Project Structure

### Documentation (this feature)

```text
specs/013-content-enrichment/
├── plan.md              # This file
├── mention-resolution.md # Feature specification
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output
│   └── llm-interface.md # LLM contract specification
└── tasks.md             # Phase 2 output (/speckit.tasks or /speckit.beads)
```

### Source Code (repository root)

```text
pkg/
├── mentions/                    # EXISTING - extend
│   ├── types.go                 # ✓ Already has ContentMention, Candidate, etc.
│   ├── repository.go            # ✓ Interface defined
│   ├── postgres_repository.go   # ✓ Implementation
│   ├── resolver/                # NEW - LLM resolution stages
│   │   ├── resolver.go          # Main resolver orchestrator
│   │   ├── stages.go            # Stage 1-4 implementations
│   │   ├── candidates.go        # Candidate gathering (code)
│   │   ├── llm.go               # LLM provider interface
│   │   ├── mlx.go               # MLX local provider
│   │   ├── claude.go            # Claude API provider (future)
│   │   └── config.go            # LLMConfig struct
│   └── audit/                   # NEW - Audit & tracing
│       ├── trace.go             # Trace creation and management
│       ├── stages.go            # Stage recording
│       ├── decisions.go         # Decision recording
│       ├── repository.go        # Trace storage interface
│       └── postgres_repository.go
├── glossary/                    # EXISTING - extend for linked_entity
│   └── types.go                 # Add linked_entity_type, linked_entity_id
└── enrichment/                  # EXISTING - integrate
    └── pipeline/
        └── pipeline.go          # Hook mention resolution into pipeline

cmd/penf/
├── cmd/
│   ├── audit.go                 # NEW - penf audit commands
│   ├── audit_trace.go           # penf audit trace show/list/export
│   ├── audit_compare.go         # penf audit compare/comparison
│   └── review.go                # EXISTING - extend for mention resolution

migrations/
├── 016_glossary_linked_entity.sql    # Add linked_entity columns
├── 017_mention_resolution.sql        # Trace tables (if not already created)
└── 018_resolution_comparisons.sql    # Comparison tables

services/worker/
└── activities/
    └── mention_resolution.go    # Temporal activity for async processing
```

**Structure Decision**: Extend existing `pkg/mentions/` package with new `resolver/` and `audit/` subpackages. This maintains the established Go monorepo pattern and integrates cleanly with existing enrichment pipeline.

## Complexity Tracking

> No violations requiring justification. Design follows established patterns.

## Implementation Phases

### Phase 0: Research (Complete in research.md)

1. MLX local LLM integration patterns for Go
2. Structured output parsing from LLM responses
3. Batch processing patterns for LLM calls
4. Trace storage optimization (JSONB vs normalized)

### Phase 1: Design (Complete in data-model.md, contracts/)

1. Data model for traces, stages, decisions, comparisons
2. LLM interface contract (input/output schemas)
3. Integration with existing enrichment pipeline

### Phase 2: Implementation (Complete in tasks.md via /speckit.tasks or /speckit.beads)

Phased delivery:
1. **Foundation**: Trace tables, basic types, repository interfaces
2. **LLM Integration**: MLX provider, structured output parsing
3. **Resolution Pipeline**: 4-stage resolver, candidate gathering
4. **Audit System**: Trace recording, CLI commands
5. **Model Comparison**: Comparison runs, statistics
6. **Learning Loop**: Correction tracking, pattern updates

---

*Next step: Run /speckit.tasks or /speckit.beads to generate implementation tasks*
