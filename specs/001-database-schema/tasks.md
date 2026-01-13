# Tasks: Database Schema and Storage Layer

**Input**: Design documents from `/specs/001-database-schema/`
**Prerequisites**: plan.md ✓, spec.md ✓, research.md ✓, data-model.md ✓, contracts/ ✓

**⚠️ IMPORTANT**: Run `/speckit.analyze` before `/speckit.implement` to verify plan completeness and validate implementation readiness.

**Tests**: Test tasks included based on TDD requirements from constitution compliance

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

Based on plan.md structure:
- **Library project**: `penf_lib/storage/`, `penf_lib/cli/`, `tests/`
- All paths are relative to repository root

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: Project initialization and basic structure

- [x] T001 Create project directory structure per implementation plan
- [x] T002 Initialize Python 3.12 project with pyproject.toml and dependencies
- [x] T003 [P] Configure development tools (ruff, mypy, black, isort) in pyproject.toml
- [ ] T004 [P] Setup PostgreSQL 16+ with pgvector extension for development
- [ ] T005 [P] Setup Redis server for event processing development
- [x] T006 [P] Create environment configuration in penf_lib/storage/config.py

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core infrastructure that MUST be complete before ANY user story can be implemented

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [x] T007 Setup database connection and session management in penf_lib/storage/connections.py
- [x] T008 [P] Initialize Alembic migration framework in penf_lib/storage/migrations/
- [x] T009 [P] Create base SQLAlchemy models with mixins in penf_lib/storage/models.py
- [x] T010 [P] Setup Redis connection and client configuration in penf_lib/storage/connections.py
- [x] T011 [P] Create test database fixtures in tests/fixtures/database.py
- [x] T012 [P] Setup pytest configuration with asyncio support in tests/conftest.py
- [x] T013 Create CLI base structure in penf_lib/cli/__init__.py

**Checkpoint**: Foundation ready - user story implementation can now begin in parallel

---

## Phase 3: User Story 1 - Schema Definition and Migration (Priority: P1) 🎯 MVP

**Goal**: Define database schema for core entities (Source, Assertion, Person, Project, Team) with proper relationships and constraints

**Independent Test**: Can be fully tested by creating schema migration scripts and verifying table creation with proper constraints, relationships, and indexes

### Tests for User Story 1 ⚠️

> **NOTE: Write these tests FIRST, ensure they FAIL before implementation**

- [ ] T014 [P] [US1] Contract test for schema validation in tests/contract/test_schemas.py
- [ ] T015 [P] [US1] Integration test for migration execution in tests/integration/test_migrations.py
- [ ] T016 [P] [US1] Unit test for model constraints in tests/unit/test_models.py

### Implementation for User Story 1

- [ ] T017 [P] [US1] Create Source model with validation in penf_lib/storage/models.py
- [ ] T018 [P] [US1] Create Assertion model with relationships in penf_lib/storage/models.py
- [ ] T019 [P] [US1] Create Person model with entity resolution fields in penf_lib/storage/models.py
- [ ] T020 [P] [US1] Create Project model with hierarchy support in penf_lib/storage/models.py
- [ ] T021 [P] [US1] Create Team model with organizational structure in penf_lib/storage/models.py
- [ ] T022 [US1] Generate initial migration with all core entities using Alembic
- [ ] T023 [US1] Add database constraints and validation rules for core entities
- [ ] T024 [US1] Implement soft delete and archive table structure
- [ ] T025 [US1] Add Row-Level Security (RLS) policies for tenant isolation
- [ ] T026 [US1] Create migration CLI commands in penf_lib/cli/migrations.py

**Checkpoint**: At this point, User Story 1 should be fully functional and testable independently

---

## Phase 4: User Story 2 - Data Storage Operations (Priority: P1)

**Goal**: Store and retrieve entity data with proper validation and transaction support to maintain data integrity across all operations

**Independent Test**: Can be fully tested by performing create, read, update, delete operations on each entity type and verifying data persistence and retrieval accuracy

### Tests for User Story 2 ⚠️

- [ ] T027 [P] [US2] Contract test for CRUD API endpoints in tests/contract/test_storage_api.py
- [ ] T028 [P] [US2] Integration test for transaction handling in tests/integration/test_transactions.py
- [ ] T029 [P] [US2] Unit test for repository operations in tests/unit/test_repositories.py

### Implementation for User Story 2

- [ ] T030 [P] [US2] Create Source repository with CRUD operations in penf_lib/storage/repositories/source.py
- [ ] T031 [P] [US2] Create Assertion repository with CRUD operations in penf_lib/storage/repositories/assertion.py
- [ ] T032 [P] [US2] Create Person repository with entity resolution in penf_lib/storage/repositories/person.py
- [ ] T033 [P] [US2] Create Project repository with hierarchy queries in penf_lib/storage/repositories/project.py
- [ ] T034 [P] [US2] Create Team repository with organizational queries in penf_lib/storage/repositories/team.py
- [ ] T035 [US2] Implement transaction management and error handling across repositories
- [ ] T036 [US2] Add validation and business rules enforcement for all entities
- [ ] T037 [US2] Create database service layer in penf_lib/storage/service.py
- [ ] T038 [US2] Implement concurrent operation handling and connection pooling
- [ ] T039 [US2] Add database CLI commands in penf_lib/cli/database.py

**Checkpoint**: At this point, User Stories 1 AND 2 should both work independently

---

## Phase 5: User Story 3 - Event-Driven Processing Framework (Priority: P2)

**Goal**: Store and manage events, processing jobs, and results from multiple AI models for pub-sub coordination

**Independent Test**: Can be fully tested by publishing processing events, subscribing multiple processors, and verifying proper job state management and result storage

### Tests for User Story 3 ⚠️

- [ ] T040 [P] [US3] Contract test for event API endpoints in tests/contract/test_events_api.py
- [ ] T041 [P] [US3] Integration test for pub-sub workflow in tests/integration/test_events.py
- [ ] T042 [P] [US3] Unit test for job state management in tests/unit/test_processing_jobs.py

### Implementation for User Story 3

- [ ] T043 [P] [US3] Create ProcessingEvent model with retention policies in penf_lib/storage/models.py
- [ ] T044 [P] [US3] Create ProcessingJob model with state machine in penf_lib/storage/models.py
- [ ] T045 [P] [US3] Create ProcessingResult model with comparison support in penf_lib/storage/models.py
- [ ] T046 [P] [US3] Create Subscription model with filtering in penf_lib/storage/models.py
- [ ] T047 [US3] Implement Redis event publisher in penf_lib/storage/events.py
- [ ] T048 [US3] Implement Redis event subscriber with error handling in penf_lib/storage/events.py
- [ ] T049 [US3] Create job management service with retry logic in penf_lib/storage/jobs.py
- [ ] T050 [US3] Add PostgreSQL LISTEN/NOTIFY fallback mechanism in penf_lib/storage/events.py
- [ ] T051 [US3] Implement MessagePack serialization for event payloads in penf_lib/storage/events.py
- [ ] T052 [US3] Create event processing migration with retention cleanup
- [ ] T053 [US3] Add event monitoring and metrics collection

**Checkpoint**: At this point, User Stories 1, 2, AND 3 should all work independently

---

## Phase 6: User Story 4 - Vector Storage for Semantic Search (Priority: P2)

**Goal**: Store and query high-dimensional embeddings for semantic search capabilities with HNSW indexing

**Independent Test**: Can be fully tested by storing embeddings and performing similarity searches with known query vectors to verify accurate nearest-neighbor retrieval

### Tests for User Story 4 ⚠️

- [ ] T054 [P] [US4] Contract test for vector API endpoints in tests/contract/test_vector_api.py
- [ ] T055 [P] [US4] Integration test for vector search performance in tests/integration/test_vector.py
- [ ] T056 [P] [US4] Unit test for embedding storage operations in tests/unit/test_embeddings.py

### Implementation for User Story 4

- [ ] T057 [US4] Create Embedding model with 768-dimensional vector support in penf_lib/storage/models.py
- [ ] T058 [US4] Implement HNSW index creation with optimized parameters in migration
- [ ] T059 [US4] Create vector storage operations in penf_lib/storage/vector.py
- [ ] T060 [US4] Implement similarity search with configurable parameters in penf_lib/storage/vector.py
- [ ] T061 [US4] Add vector embedding repository with batch operations in penf_lib/storage/repositories/embedding.py
- [ ] T062 [US4] Create vector search service with caching in penf_lib/storage/vector.py
- [ ] T063 [US4] Implement embedding lifecycle management and cleanup
- [ ] T064 [US4] Add vector performance monitoring and index optimization
- [ ] T065 [US4] Create vector CLI commands for index management in penf_lib/cli/vector.py

**Checkpoint**: At this point, User Stories 1-4 should all work independently

---

## Phase 7: User Story 5 - Performance Monitoring and Optimization (Priority: P3)

**Goal**: Database performance monitoring and query optimization capabilities for effective scaling

**Independent Test**: Can be fully tested by executing performance benchmarks and monitoring query execution plans with various data loads

### Tests for User Story 5 ⚠️

- [ ] T066 [P] [US5] Performance benchmark tests in tests/performance/test_benchmarks.py
- [ ] T067 [P] [US5] Integration test for monitoring alerts in tests/integration/test_monitoring.py
- [ ] T068 [P] [US5] Unit test for performance metrics in tests/unit/test_performance.py

### Implementation for User Story 5

- [ ] T069 [US5] Define database observability requirements for centralized monitoring (see specs/011-observability-framework)
- [ ] T070 [US5] Implement query execution plan analysis in penf_lib/storage/optimization.py
- [ ] T071 [US5] Integrate with centralized observability framework for database metrics
- [ ] T072 [US5] Create index usage analysis and recommendations in penf_lib/storage/optimization.py
- [ ] T073 [US5] Implement automated query optimization suggestions
- [ ] T074 [US5] Request database alerting from centralized framework
- [ ] T075 [US5] Create database CLI commands with observability integration
- [ ] T076 [US5] Add database health instrumentation for centralized monitoring

**Checkpoint**: All user stories should now be independently functional

---

## Phase 8: Polish & Cross-Cutting Concerns

**Purpose**: Improvements that affect multiple user stories

- [ ] T077 [P] Update documentation in specs/001-database-schema/quickstart.md
- [ ] T078 [P] Add comprehensive error handling across all modules
- [ ] T079 [P] Implement structured logging for all database operations
- [ ] T080 [P] Add security hardening and input validation
- [ ] T081 [P] Performance optimization across all repositories
- [ ] T082 [P] Code cleanup and refactoring for consistency
- [ ] T083 Run quickstart.md validation with real implementation
- [ ] T084 Create final integration tests for complete system workflow
- [ ] T085 [P] Implement database backup procedures with pg_dump integration in penf_lib/storage/backup.py
- [ ] T086 [P] Implement database restore procedures with validation in penf_lib/storage/backup.py
- [ ] T087 [P] Add backup/restore CLI commands in penf_lib/cli/database.py

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Stories (Phases 3-7)**: All depend on Foundational phase completion
  - User stories can proceed in parallel (if staffed)
  - Or sequentially in priority order (P1 → P1 → P2 → P2 → P3)
- **Polish (Phase 8)**: Depends on all desired user stories being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational (Phase 2) - No dependencies on other stories
- **User Story 2 (P1)**: Can start after Foundational (Phase 2) - Extends US1 models but independently testable
- **User Story 3 (P2)**: Can start after Foundational (Phase 2) - Independent event system, integrates with US1/US2
- **User Story 4 (P2)**: Can start after Foundational (Phase 2) - Independent vector system, links to US1/US2 entities
- **User Story 5 (P3)**: Can start after Foundational (Phase 2) - Monitors all previous stories but independently testable

### Within Each User Story

- Tests MUST be written and FAIL before implementation
- Models before repositories
- Repositories before services
- Services before CLI commands
- Core implementation before integration
- Story complete before moving to next priority

### Parallel Opportunities

- All Setup tasks marked [P] can run in parallel (T003, T004, T005, T006)
- All Foundational tasks marked [P] can run in parallel within Phase 2
- Once Foundational phase completes, all user stories can start in parallel
- Within each user story:
  - All tests marked [P] can run in parallel
  - All models marked [P] can run in parallel
  - All repositories marked [P] can run in parallel

---

## Parallel Example: User Story 1

```bash
# Launch all tests for User Story 1 together:
Task: "Contract test for schema validation in tests/contract/test_schemas.py"
Task: "Integration test for migration execution in tests/integration/test_migrations.py"
Task: "Unit test for model constraints in tests/unit/test_models.py"

# Launch all models for User Story 1 together:
Task: "Create Source model with validation in penf_lib/storage/models.py"
Task: "Create Assertion model with relationships in penf_lib/storage/models.py"
Task: "Create Person model with entity resolution fields in penf_lib/storage/models.py"
Task: "Create Project model with hierarchy support in penf_lib/storage/models.py"
Task: "Create Team model with organizational structure in penf_lib/storage/models.py"
```

---

## Implementation Strategy

### MVP First (User Stories 1 & 2 Only)

1. Complete Phase 1: Setup
2. Complete Phase 2: Foundational (CRITICAL - blocks all stories)
3. Complete Phase 3: User Story 1 (Schema Definition)
4. Complete Phase 4: User Story 2 (Data Operations)
5. **STOP and VALIDATE**: Test core storage functionality independently
6. Deploy/demo if ready - provides complete foundational storage layer

### Incremental Delivery

1. Complete Setup + Foundational → Foundation ready
2. Add User Story 1 → Test independently → Core schema established
3. Add User Story 2 → Test independently → Complete CRUD operations (MVP!)
4. Add User Story 3 → Test independently → Event processing enabled
5. Add User Story 4 → Test independently → Vector search capabilities
6. Add User Story 5 → Test independently → Production monitoring
7. Each story adds value without breaking previous stories

### Parallel Team Strategy

With multiple developers:

1. Team completes Setup + Foundational together
2. Once Foundational is done:
   - Developer A: User Stories 1 & 2 (core storage - sequential, high dependency)
   - Developer B: User Story 3 (event processing - independent)
   - Developer C: User Story 4 (vector operations - independent)
   - Developer D: User Story 5 (monitoring - independent)
3. Stories complete and integrate independently

---

## Summary

- **Total Tasks**: 87
- **User Story 1**: 13 tasks (schema definition and migration)
- **User Story 2**: 13 tasks (data storage operations)
- **User Story 3**: 14 tasks (event-driven processing)
- **User Story 4**: 12 tasks (vector storage and search)
- **User Story 5**: 10 tasks (performance monitoring)
- **Setup + Foundational**: 13 tasks
- **Polish**: 11 tasks
- **Parallel Opportunities**: 50 tasks marked [P] for parallel execution
- **Independent Test Criteria**: Each user story has clear acceptance tests
- **MVP Scope**: User Stories 1 & 2 provide complete foundational storage layer

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- Tests written first and must fail before implementing
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- Constitution compliance: TDD enforced, CLI+library architecture, test-first development