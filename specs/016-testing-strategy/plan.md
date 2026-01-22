# Implementation Plan: Testing Strategy

**Branch**: `016-testing-strategy` | **Date**: 2026-01-22 | **Spec**: [spec.md](spec.md)
**Input**: Feature specification from `/specs/016-testing-strategy/spec.md`

## Summary

Implement a comprehensive testing strategy with four test categories (unit, integration, E2E, live) to address the current gap where 160 test files exist but are almost entirely unit tests with mocked dependencies. The implementation establishes integration tests against real PostgreSQL on home-01, E2E tests with local LLM (Qwen via MLX), and a mock organization ("Acme Corp") fixture for realistic end-to-end validation.

## Technical Context

**Language/Version**: Go 1.22+
**Primary Dependencies**: testify (assertions/mocking), pgx/v5 (PostgreSQL), yaml.v3 (fixtures)
**Storage**: PostgreSQL 16 + pgvector on home-01.brown.chat (separate test databases)
**Testing**: go test with testify, build tags for test isolation
**Target Platform**: macOS (dev01 for E2E with MLX), Linux (CI for unit/integration)
**Project Type**: Single project - test infrastructure addition
**Performance Goals**: Unit < 10s, Integration < 60s, E2E < 5min total suite
**Constraints**: E2E requires Apple Silicon (MLX LLM), network access to home-01
**Scale/Scope**: Extend 160 existing tests → target 200+ with integration/E2E coverage

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Status | Notes |
|-----------|--------|-------|
| III. Test-First Development | ✅ PASS | This spec establishes TDD infrastructure |
| IV. Integration Testing | ✅ PASS | Directly addresses integration test gap |
| V. Observability & Debugging | ✅ PASS | Tests will use structured logging |
| VII. Simplicity & YAGNI | ✅ PASS | Uses existing infra (home-01 DB, dev01 LLM) |
| Code Quality: 80% coverage | ✅ PASS | Spec targets 80%+ for core packages |

**Gate Result**: PASS - All constitution principles satisfied

## Project Structure

### Documentation (this feature)

```text
specs/016-testing-strategy/
├── plan.md              # This file
├── research.md          # Phase 0 output
├── data-model.md        # Phase 1 output (fixture schema)
├── quickstart.md        # Phase 1 output
├── contracts/           # Phase 1 output (test helper interfaces)
└── tasks.md             # Phase 2 output (/speckit.tasks)
```

### Source Code (repository root)

```text
tests/
├── integration/
│   ├── db_test.go            # PostgreSQL integration tests
│   ├── redis_test.go         # Redis integration tests
│   ├── search_test.go        # Vector search integration tests
│   ├── helpers.go            # Database setup/teardown helpers
│   └── testdata/             # SQL fixtures for integration tests
├── e2e/
│   ├── ingest_test.go        # Email ingestion E2E
│   ├── search_test.go        # Search query E2E
│   ├── mentions_test.go      # Mention resolution E2E
│   ├── environment.go        # E2E test harness
│   ├── assertions.go         # Semantic assertion helpers
│   └── helpers.go            # Fixture loading, cleanup
├── live/
│   ├── gemini_test.go        # Real Gemini API tests
│   └── gmail_test.go         # Real Gmail API tests (build tag: live)
└── fixtures/
    └── acme-corp/
        ├── people.yaml       # 20+ people with aliases
        ├── teams.yaml        # 5+ teams
        ├── projects.yaml     # 10+ projects
        ├── products.yaml     # Products
        ├── glossary.yaml     # 50+ terms
        ├── emails/           # Sample .eml files
        └── meetings/         # Sample transcripts
```

**Structure Decision**: Single `tests/` directory at repo root with subdirectories per test category. Fixtures in `tests/fixtures/acme-corp/`. Integration tests can also be co-located as `*_integration_test.go` in service directories.

## Complexity Tracking

> No constitution violations requiring justification.

| Aspect | Decision | Rationale |
|--------|----------|-----------|
| DB isolation | Separate databases on home-01 | Simpler than testcontainers, reuses existing infra |
| LLM for E2E | Real local LLM (Qwen) | Validates actual LLM behavior, not just mocks |
| CI runner | Self-hosted on dev01 | Required for MLX, no additional cost |

## Phase 0: Research Items

1. **Go testcontainers vs direct DB** - Confirm direct DB approach is sufficient
2. **Build tag patterns** - Best practices for `//go:build` tags in Go
3. **Fixture loading** - YAML parsing and database seeding patterns
4. **GitHub Actions self-hosted runner** - Setup and configuration

## Phase 1: Design Artifacts

1. **data-model.md** - Fixture YAML schemas for Acme Corp
2. **contracts/** - Test helper interfaces (Environment, Assertions)
3. **quickstart.md** - How to run each test category

## Next Steps

1. Generate `research.md` (Phase 0)
2. Generate `data-model.md`, `contracts/`, `quickstart.md` (Phase 1)
3. Run `/speckit.tasks` to generate implementation tasks
