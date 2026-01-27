# Testing Standards

> **Status:** DRAFT - Created after critical testing gaps discovered in production (2026-01-26)

This document defines the testing standards for Penfold. These are mandatory, not optional.

---

## Test Pyramid

```
        ┌─────────────────────┐
        │    E2E Tests        │  ← Full system, CLI to DB
        │   (Must FAIL)       │     Runs on self-hosted runner
        ├─────────────────────┤
        │  Integration Tests  │  ← Real DB, real services
        │   (Must FAIL)       │     Runs in CI with containers
        ├─────────────────────┤
        │    Unit Tests       │  ← Isolated logic only
        │  (Mocks allowed)    │     Mocks for external deps
        └─────────────────────┘
```

---

## Mandatory Rules

### Rule 1: Tests Must Fail on Failure

**NEVER use `t.Skip()` or `t.Skipf()` when a test operation fails.**

```go
// ❌ WRONG - Hides failures
if result.ExitCode != 0 {
    t.Skipf("Command failed - ensure services are running")
}

// ✅ CORRECT - Fails the test
if result.ExitCode != 0 {
    t.Fatalf("Command failed (exit %d): %s", result.ExitCode, result.Stderr)
}
```

**When Skip is Acceptable:**
- Missing optional infrastructure (e.g., LLM for non-LLM tests)
- Build tag filtering (`//go:build integration`)
- Platform-specific tests on wrong platform

**When Skip is NOT Acceptable:**
- Service not running that test requires
- Database connection failed
- Any operation the test is designed to verify

### Rule 2: Mocks Are Last Resort

**Use mocks ONLY when you cannot use the real implementation.**

```go
// ❌ WRONG - Mocking what you can test for real
tenantRepo := &mockTenantRepository{
    getByRefFn: func(ctx context.Context, ref string) (*Tenant, error) {
        return &Tenant{ID: "uuid"}, nil
    },
}

// ✅ CORRECT - Test against real database
db := SetupTestDB(t)
tenantRepo := storage.NewTenantRepository(db)
// Insert test tenant, then test real behavior
```

**When Mocks Are Acceptable:**
- External APIs (Gmail, Temporal, third-party services)
- Time-dependent operations (use clock interface)
- Network calls to services not in test container

**When Mocks Are NOT Acceptable:**
- Database repositories (use test database)
- Internal service interfaces (use real implementations)
- Business logic validation

### Rule 3: Integration Tests for Every Service

Every service that has database operations MUST have integration tests.

**Required Coverage:**
| Service | Integration Test | Status |
|---------|-----------------|--------|
| gateway/ingestservice | `tests/integration/ingest_test.go` | ❌ MISSING |
| gateway/projectservice | `tests/integration/project_test.go` | ❌ MISSING |
| gateway/tenantservice | `tests/integration/tenant_test.go` | ❌ MISSING |
| gateway/entityservice | `tests/integration/entity_test.go` | ❌ MISSING |
| gateway/glossaryservice | Exists | ✅ |
| gateway/mentionsservice | Exists | ✅ |
| search/* | Exists | ✅ |

### Rule 4: E2E Tests Must Be Deterministic

E2E tests must:
1. **Start from known state** - Truncate tables, load fixtures
2. **Not depend on external state** - Don't assume data exists
3. **Assert specific outcomes** - Not just "no error"
4. **Clean up after themselves** - Or use transaction rollback

```go
// ❌ WRONG - Assumes state, weak assertions
result := env.CLI.Run(ctx, "search", "something")
if result.ExitCode == 0 {
    t.Logf("Results: %s", result.Stdout)  // Just logging, not asserting
}

// ✅ CORRECT - Known state, strong assertions
env.TruncateAllTables()
env.LoadFixture("acme-corp")
env.IngestEmail("001-project-update.eml")

result := env.CLI.Run(ctx, "search", "Project Alpha")
require.Equal(t, 0, result.ExitCode, "search should succeed")
require.Contains(t, result.Stdout, "Project Alpha", "should find ingested content")
```

### Rule 5: Test Real User Workflows

Tests must exercise the same code paths users will hit.

```go
// ❌ WRONG - Tests internal function with test tenant
svc.CreateIngestJob(ctx, &IngestJobRequest{TenantId: "test-uuid"})

// ✅ CORRECT - Tests like a user would
// User has config with tenant slug "akamai"
result := env.CLI.Run(ctx, "ingest", "email", emailPath)
// This exercises: CLI → gRPC → Gateway → TenantResolution → DB
```

### Rule 6: Fixtures Must Match Production

Test fixtures should use realistic data that mirrors production configuration.

```yaml
# ❌ WRONG - Generic test data
tenants:
  - slug: test
    name: Test Tenant

# ✅ CORRECT - Mirrors production setup
tenants:
  - slug: akamai
    name: Akamai Technologies
    id: 92271dd2-9203-4de1-91cf-2e26bef41aff  # Real UUID format
```

---

## Test Categories

### Unit Tests (`*_test.go`, no build tag)

- Run on every commit
- No external dependencies
- Fast (<1s per test)
- Test pure functions and isolated logic
- Mocks allowed for external interfaces

### Integration Tests (`//go:build integration`)

- Run in CI with Docker containers
- Real PostgreSQL with pgvector
- Test repository layer against real DB
- Test service layer with real dependencies
- Must apply migrations first

### E2E Tests (`//go:build e2e`)

- Run on main branch only
- Run on self-hosted runner with full stack
- Test CLI → Gateway → Services → DB
- Test real user workflows
- Must verify end-to-end behavior

---

## Required Test Coverage

### Per-Service Requirements

Every service in `services/` must have:

1. **Unit tests** for business logic (`service_test.go`)
2. **Integration tests** if it has DB operations (`tests/integration/<service>_test.go`)
3. **E2E test coverage** for CLI commands it supports

### Per-Feature Requirements

Every new feature must have:

1. **Unit tests** for new functions
2. **Integration tests** for DB/service interactions
3. **E2E tests** for user-facing commands
4. **Migration tests** if schema changes

---

## CI Requirements

### Test Gates

| Stage | Tests Run | Must Pass |
|-------|-----------|-----------|
| PR | Unit, Integration | Yes |
| Main | Unit, Integration, E2E | Yes |
| Release | All + Smoke tests | Yes |

### Failure Handling

- **Unit test failure** → PR blocked
- **Integration test failure** → PR blocked
- **E2E test failure** → Main build fails, alerts sent
- **No silent skips** → Tests that skip must explain why in CI output

---

## Migration Testing

Every database migration must be tested:

```go
func TestMigration021_ContentID(t *testing.T) {
    db := SetupTestDB(t)

    // Run all migrations up to but not including this one
    RunMigrationsTo(t, db, 20)

    // Verify column doesn't exist
    exists := ColumnExists(t, db, "sources", "content_id")
    require.False(t, exists)

    // Run the migration
    RunMigration(t, db, 21)

    // Verify column exists
    exists = ColumnExists(t, db, "sources", "content_id")
    require.True(t, exists)

    // Test rollback
    RollbackMigration(t, db, 21)
    exists = ColumnExists(t, db, "sources", "content_id")
    require.False(t, exists)
}
```

---

## Deployment Verification

Before marking a deployment complete:

1. **Migrations applied** - All migrations run successfully
2. **Services healthy** - All health endpoints return OK
3. **Smoke tests pass** - Basic CLI commands work

```bash
# Required smoke tests after deployment
penf status                    # Gateway reachable
penf health gateway            # All services healthy
penf search "test" --limit 1   # Search service works
penf ingest email test.eml     # Ingest pipeline works
```

---

## Fixing Existing Tests

Priority order for fixing current test gaps:

1. **P0: Remove all t.Skip on failure** - Convert to t.Fatal
2. **P1: Add missing integration tests** - Ingest, Project, Tenant, Entity
3. **P2: Add migration tests** - For all migrations
4. **P3: Add deployment smoke tests** - Automated verification
5. **P4: Reduce mock usage** - Replace with real implementations where possible
