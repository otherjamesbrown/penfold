# Implementation Plan: Search and Query Interface

**Branch**: `007-search-interface` | **Date**: 2026-01-15 | **Spec**: [spec.md](./spec.md)
**Input**: Feature specification from `/specs/007-search-interface/spec.md`

**Note**: This template is filled in by the `/speckit.plan` command. See `.specify/templates/commands/plan.md` for the execution workflow.

## Summary

Unified search interface for Penfold enabling natural language queries across emails, meetings, documents, and other content types. Implements hybrid search combining PostgreSQL full-text search with pgvector semantic similarity, temporal query parsing for "contextual time machine" functionality, cross-content correlation discovery, and advanced filtering with relevance ranking. Integrates with existing AI coordination layer for content processing results and relationship discovery.

## Technical Context

**Language/Version**: Python 3.12
**Primary Dependencies**: SQLAlchemy 2.0+ with asyncpg driver, pgvector for vector similarity, click for CLI, rich for output formatting, pydantic for query/response validation
**Storage**: PostgreSQL 16+ with pgvector extension (hybrid relational + vector search), Redis for caching search results
**Testing**: pytest with async fixtures, contract tests for search API, integration tests for search pipeline
**Target Platform**: Mac Mini M4 with 32GB RAM (local development), Linux server deployment
**Project Type**: Library with CLI interface - search service layer consuming indexed content
**Performance Goals**: Sub-15 second search response time (per FR-003), sub-500ms for cached queries, 50 concurrent search queries without degradation (SC-005)
**Constraints**: Local deployment resources, offline capability for locally-stored content, 100k content items scale (per assumptions)
**Scale/Scope**: 100k content items indexed, typical 3-10 word queries, 85% relevance rate target, 90% timeline reconstruction accuracy

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

**I. Specification-First Development**: Complete specification with user stories, acceptance criteria, functional requirements, success criteria, and edge cases documented
**II. CLI + Library Architecture**: Search designed as standalone library (`penf_lib/search/`) with clear interfaces, exposed via CLI commands
**III. Test-First Development**: Plan includes TDD with unit tests, integration tests, and contract tests for all search components
**IV. Integration Testing**: Contract tests planned for search API, vector operations, and AI coordination integration
**V. Observability & Debugging**: Structured logging for all search operations, query performance tracking, analytics collection
**VI. Versioning & Breaking Changes**: Search API versioned, breaking changes require migration support
**VII. Simplicity & YAGNI**: Core search functionality first (P1 stories), advanced filtering (P2) and analytics (P3) as follow-on
**VIII. Temporal-First Data Organization**: Primary feature - temporal queries, timeline reconstruction, chronological context
**IX. Research-Driven Design**: Hybrid search (full-text + vector) based on current semantic search best practices
**X. Local-First AI with Privacy**: All search operations local, no cloud dependencies for query processing

**Gate Status**: PASS - All constitution principles satisfied

**Post-Design Re-evaluation**: CONFIRMED
- **Test-First Development**: Comprehensive testing strategy with unit, integration, and contract tests documented in project structure
- **Integration Testing**: Contract tests defined for search API (search-api.yaml), query schema validation (query-schema.json)
- **Observability**: SearchAnalytics model for usage tracking, query performance logging, execution time metrics in all responses
- **Simplicity**: Core P1 functionality (natural language search, temporal queries, correlations) prioritized; P2 (advanced filtering) and P3 (analytics) clearly scoped as follow-on

## Project Structure

### Documentation (this feature)

```text
specs/007-search-interface/
├── plan.md              # This file (/speckit.plan command output)
├── research.md          # Phase 0 output (/speckit.plan command)
├── data-model.md        # Phase 1 output (/speckit.plan command)
├── quickstart.md        # Phase 1 output (/speckit.plan command)
├── contracts/           # Phase 1 output (/speckit.plan command)
│   ├── search-api.yaml  # OpenAPI spec for search operations
│   └── query-schema.json # JSON Schema for query parameters
└── tasks.md             # Phase 2 output (/speckit.tasks command - NOT created by /speckit.plan)
```

### Source Code (repository root)

```text
penf_lib/
├── search/                    # NEW: Search service module
│   ├── __init__.py
│   ├── query_parser.py        # Natural language query parsing and temporal expressions
│   ├── search_engine.py       # Hybrid search orchestration (full-text + vector)
│   ├── ranking.py             # Result ranking and relevance scoring
│   ├── filters.py             # Filter application and refinement
│   ├── correlations.py        # Cross-content relationship discovery
│   ├── suggestions.py         # Query suggestions and autocomplete
│   ├── session.py             # Search session and history management
│   ├── analytics.py           # Search usage analytics (P3)
│   └── models.py              # Pydantic models for queries and results
├── storage/
│   └── repositories/
│       └── search.py          # NEW: Search-specific data access
├── cli/
│   └── search_commands.py     # NEW: Search CLI commands
└── __init__.py

tests/
├── unit/
│   ├── test_query_parser.py   # Query parsing unit tests
│   ├── test_ranking.py        # Ranking algorithm tests
│   └── test_filters.py        # Filter logic tests
├── integration/
│   ├── test_search_engine.py  # End-to-end search tests
│   ├── test_correlations.py   # Cross-content correlation tests
│   └── test_temporal.py       # Temporal query tests
└── contract/
    └── test_search_api.py     # Search API contract tests
```

**Structure Decision**: New `penf_lib/search/` module following existing library architecture pattern. Integrates with existing `storage/vector.py` for similarity search and `storage/repositories/` for data access. CLI commands under `penf_lib/cli/search_commands.py` following existing CLI patterns with click and rich. Search module consumes existing models (Source, Assertion, Person, Project, Embedding) without modification.

## Complexity Tracking

> **Fill ONLY if Constitution Check has violations that must be justified**

No violations - design follows established patterns and reuses existing infrastructure.
