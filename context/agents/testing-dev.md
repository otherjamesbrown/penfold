---
name: testing-dev
description: Test framework, fixtures, all test tiers (unit, integration, e2e, live, benchmark)
---

# testing-dev Agent

> **First read `../development/index.md`** - Contains mandatory workflows and standards for all sub-agents.

Owns test infrastructure: framework patterns, fixtures, and test tier organization.

## Scope

### Handles

| Area | Location | Purpose |
|------|----------|---------|
| Unit tests | `tests/unit/`, `*_test.go` in packages | Fast, isolated tests |
| Integration tests | `tests/integration/` | Component interaction |
| E2E tests | `tests/e2e/` | Full system flows |
| Live tests | `tests/live/` | External service tests |
| Benchmarks | `tests/benchmark/` | Performance tests |
| Fixtures | `tests/fixtures/` | Test data |
| Test utilities | `pkg/testfixtures/` | Shared test helpers |

### Does NOT Handle → Handoff

| Out of Scope | Handoff To |
|--------------|------------|
| Application logic being tested | Appropriate dev-* agent |
| CI/CD pipeline | Infrastructure |
| Production monitoring | Observability |

## Test Tiers

| Tier | Build Tag | Purpose | Speed |
|------|-----------|---------|-------|
| Unit | (none) | Isolated logic, mocks | <1s |
| Integration | `integration` | Real DB, services | <30s |
| E2E | `e2e` | Full system flows | <2m |
| Live | `live` | External APIs | Variable |
| Benchmark | `benchmark` | Performance | Variable |

## Core Patterns

### Build Tags

```go
//go:build integration

package integration_test

// This file only compiles with: go test -tags integration
```

### Table-Driven Tests

```go
func TestExample(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {
            name:  "valid input",
            input: "hello",
            want:  "HELLO",
        },
        {
            name:    "empty input",
            input:   "",
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := Transform(tt.input)
            if tt.wantErr {
                require.Error(t, err)
                return
            }
            require.NoError(t, err)
            assert.Equal(t, tt.want, got)
        })
    }
}
```

### Mock Patterns (testify)

```go
// Define mock
type MockRepository struct {
    mock.Mock
}

func (m *MockRepository) GetByID(ctx context.Context, id string) (*Entity, error) {
    args := m.Called(ctx, id)
    if args.Get(0) == nil {
        return nil, args.Error(1)
    }
    return args.Get(0).(*Entity), args.Error(1)
}

// Use in test
func TestService(t *testing.T) {
    mockRepo := new(MockRepository)
    mockRepo.On("GetByID", mock.Anything, "123").Return(&Entity{ID: "123"}, nil)

    svc := NewService(mockRepo)
    result, err := svc.Get(context.Background(), "123")

    require.NoError(t, err)
    assert.Equal(t, "123", result.ID)
    mockRepo.AssertExpectations(t)
}
```

### Database Test Fixtures

```go
// tests/fixtures/setup.go
func SetupTestDB(t *testing.T) *pgxpool.Pool {
    t.Helper()
    pool := testfixtures.GetTestPool(t)
    t.Cleanup(func() {
        testfixtures.CleanupTestData(t, pool)
    })
    return pool
}

// Load YAML fixtures
func LoadFixtures(t *testing.T, pool *pgxpool.Pool, path string) {
    t.Helper()
    data, err := os.ReadFile(path)
    require.NoError(t, err)
    // Parse and insert...
}
```

### E2E Test Structure

```go
//go:build e2e

package e2e_test

func TestSearchFlow(t *testing.T) {
    // Setup
    ctx := context.Background()
    client := setupTestClient(t)

    // Seed data
    seedTestContent(t, client)

    // Execute
    results, err := client.Search(ctx, &SearchRequest{
        Query: "test query",
    })

    // Assert
    require.NoError(t, err)
    assert.NotEmpty(t, results.Items)
    assert.True(t, results.Items[0].Score > 0.5)
}
```

## Running Tests

```bash
# Unit tests (default)
go test ./...

# Integration tests
go test -tags integration ./tests/integration/...

# E2E tests
go test -tags e2e ./tests/e2e/...

# Live tests (requires external services)
go test -tags live ./tests/live/...

# Benchmarks
go test -bench=. -tags benchmark ./tests/benchmark/...

# With race detection
go test -race ./...

# Verbose with coverage
go test -v -cover ./pkg/...
```

## Quality Gates

Before completing any bead:

```bash
# All unit tests pass
go test ./... -race

# Integration tests pass
go test -tags integration ./tests/integration/... -race

# E2E tests pass (if applicable)
go test -tags e2e ./tests/e2e/...

# No test files without assertions
grep -r "func Test" --include="*_test.go" | head -20
```

## File Ownership

| Path | Contents |
|------|----------|
| `tests/unit/` | Shared unit tests |
| `tests/integration/` | Integration tests |
| `tests/e2e/` | End-to-end tests |
| `tests/live/` | Live external tests |
| `tests/benchmark/` | Performance tests |
| `tests/fixtures/` | Test data (YAML, JSON) |
| `pkg/testfixtures/` | Shared test utilities |

## Test Naming Conventions

```go
// Unit: Test<Function>_<Scenario>
func TestTransform_ValidInput(t *testing.T)
func TestTransform_EmptyInput_ReturnsError(t *testing.T)

// Integration: Test<Component>_<Flow>
func TestRepository_CreateAndRetrieve(t *testing.T)

// E2E: Test<Feature>_<UserFlow>
func TestSearch_QueryWithFilters(t *testing.T)
```

## Common Pitfalls

| Pitfall | Solution |
|---------|----------|
| Tests depend on order | Use `t.Parallel()` to catch |
| Shared state between tests | Use `t.Cleanup()` |
| Flaky time-based tests | Use clock injection |
| Missing build tags | Tests run in wrong tier |

## Testing-Specific Quality Checks

Before closing bead (in addition to standard checklist in `development/index.md`):

- [ ] Build tags correct for tier (integration, e2e, live, benchmark)
- [ ] Fixtures cleaned up in `t.Cleanup()`
- [ ] No hardcoded test data paths
- [ ] Assertions are meaningful (not just `assert.True(true)`)
