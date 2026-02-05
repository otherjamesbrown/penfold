# Testing Patterns

Go testing patterns for Penfold's four-tier test architecture.

## 1. Build Tag Test Categorization

**Pattern**: Use Go build tags to separate test tiers with different dependencies

```go
//go:build integration

package integration

// This file only compiles when: go test -tags=integration
```

**Implementation**:
| Tag | Purpose | Dependencies |
|-----|---------|--------------|
| (none) | Unit tests | Mocks only |
| `integration` | Database tests | PostgreSQL |
| `e2e` | End-to-end tests | PostgreSQL + LLM |
| `live` | Cloud API tests | External APIs |

## 2. Test Database Isolation

**Pattern**: Separate test databases with automatic cleanup

```go
// tests/integration/helpers.go
func SetupTestDB(t *testing.T) *TestDB {
    t.Helper()

    // Connect to dedicated test database
    dbName := getEnvOrDefault("PENFOLD_DB_NAME", "penfold_test_integration")
    pool, err := pgxpool.New(ctx, connStr)
    require.NoError(t, err)

    testDB := &TestDB{Pool: pool, Name: dbName}

    // Register cleanup
    t.Cleanup(func() {
        pool.Close()
    })

    return testDB
}

// Truncate between tests for isolation
func (db *TestDB) TruncateAllTables(t *testing.T) {
    // Truncates all user tables, preserves schema_migrations
}
```

**Key Points**:
- Integration tests: `penfold_test_integration` on dev02
- E2E tests: `penfold_test_e2e` on dev02
- Cleanup via `t.Cleanup()` for automatic resource release

## 3. YAML Fixture Loading

**Pattern**: Type-safe YAML fixtures with database loader

```go
// pkg/testfixtures/types.go
type PersonFixture struct {
    ID            int64    `yaml:"id"`
    CanonicalName string   `yaml:"canonical_name"`
    Email         string   `yaml:"email"`
    Aliases       []string `yaml:"aliases"`
}

// pkg/testfixtures/loader.go
func (l *Loader) LoadAcmeCorp(ctx context.Context) error {
    // Load in dependency order
    if err := l.LoadTeams(ctx); err != nil { return err }
    if err := l.LoadPeople(ctx); err != nil { return err }
    if err := l.LoadProjects(ctx); err != nil { return err }
    if err := l.LoadGlossary(ctx); err != nil { return err }
    return nil
}
```

**Fixture Files**:
- `tests/fixtures/acme-corp/people.yaml` - 20 employees
- `tests/fixtures/acme-corp/teams.yaml` - 7 teams
- `tests/fixtures/acme-corp/glossary.yaml` - 50+ terms

## 4. LLM Testing with Semantic Assertions

**Pattern**: Test LLM behavior with structure verification, not exact text matching

```go
// E2E test with real LLM
func TestMentionResolution(t *testing.T) {
    env := SetupE2EEnvironment(t)
    client := NewLLMClient(env.LLMURL)

    response, err := client.Complete(ctx, prompt)
    require.NoError(t, err)

    // Semantic assertion - verify structure, not exact text
    assert.Contains(t, response, "John Smith")  // Person resolved
    assert.NotContains(t, response, "UNKNOWN")  // Not unresolved
}

// Unit test with mock
func TestMentionResolution_Mock(t *testing.T) {
    mockLLM := new(MockLLMClient)
    mockLLM.On("Complete", mock.Anything, mock.Anything).
        Return(`{"person_id": 1, "confidence": 0.95}`, nil)

    // Deterministic, fast test
}
```

**Key Points**:
- E2E: Use `temperature=0` for more deterministic output
- E2E: Assert on structure and key content, not exact text
- Unit: Full mocking for speed and determinism

## 5. Graceful Test Skipping

**Pattern**: Skip tests when prerequisites unavailable

```go
func SetupE2EEnvironment(t *testing.T) *E2EEnv {
    password := os.Getenv("PENFOLD_DB_PASSWORD")
    if password == "" {
        t.Skip("PENFOLD_DB_PASSWORD not set - skipping E2E test")
    }

    env := &E2EEnv{...}

    if !env.LLMAvailable() {
        t.Skip("Local LLM not available - skipping E2E test")
    }

    return env
}

// Live test pattern
func RequireGeminiAPIKey(t *testing.T) string {
    key := os.Getenv("GEMINI_API_KEY")
    if key == "" {
        t.Skip("GEMINI_API_KEY not set - skipping live test")
    }
    return key
}
```

## 6. Test Helper Organization

**Pattern**: Helpers in same package as tests, shared fixtures in `pkg/testfixtures`

```
tests/
├── integration/
│   ├── helpers.go          # SetupTestDB, EnsureAcmeCorpFixtures, runCLI
│   ├── cli_ai_test.go      # AI command tests
│   ├── cli_glossary_test.go # Glossary command tests (12 tests)
│   ├── cli_meeting_test.go  # Meeting command tests (10 tests)
│   ├── cli_content_test.go  # Content command tests (10 tests)
│   ├── cli_mentions_test.go # Process mentions/acronyms tests (5 tests)
│   └── *_test.go           # Service integration tests
├── e2e/
│   ├── helpers.go          # SetupE2EEnvironment
│   ├── assertions.go       # AssertMentionResolved, etc.
│   ├── llm_client.go       # OpenAI-compatible client
│   └── *_test.go
├── live/
│   ├── helpers.go          # RequireGeminiAPIKey, etc.
│   └── *_test.go
└── fixtures/
    └── acme-corp/          # Test fixtures (60+ glossary terms, 20 people)
        ├── glossary.yaml
        ├── people.yaml
        ├── teams.yaml
        └── projects.yaml

pkg/testfixtures/           # Shared across all test tiers
├── types.go
├── loader.go
└── validate_test.go
```

## 7. CLI Integration Test Pattern

**Pattern**: Test CLI commands against real backend with fixture data

```go
//go:build integration

func TestCLI_GlossaryList(t *testing.T) {
    db := SetupTestDB(t)
    EnsureAcmeCorpFixtures(t, db)  // Loads once per test run via sync.Once

    stdout, stderr, err := runCLI(t, "glossary", "list")

    require.NoError(t, err, "glossary list should succeed. stderr: %s", stderr)
    assert.Contains(t, stdout, "TER", "should list TER term")
}

func TestCLI_GlossaryList_JSONOutput(t *testing.T) {
    db := SetupTestDB(t)
    EnsureAcmeCorpFixtures(t, db)

    // Test JSON output format
    stdout, stderr, err := runCLI(t, "glossary", "list", "-o", "json")
    require.NoError(t, err, "should succeed. stderr: %s", stderr)

    var terms []GlossaryTerm
    err = json.Unmarshal([]byte(stdout), &terms)
    require.NoError(t, err, "should be valid JSON")
    assert.NotEmpty(t, terms, "should return terms")
}
```

**Helper Functions** (`helpers.go`):
- `runCLI(t, args...)` - Executes `penf` CLI with timeout
- `runCLIWithJSON[T](t, args...)` - Parses JSON output into typed struct
- `EnsureAcmeCorpFixtures(t, db)` - One-time fixture loading via `sync.Once`
- `assertJSONContains(t, stdout, keys...)` - Verify JSON has expected keys
- `assertJSONArrayNotEmpty(t, stdout, key)` - Verify JSON array not empty

**Key Points**:
- Tests run against real backend (Gateway + PostgreSQL)
- Fixtures loaded once per test run for performance
- Tests use tenant isolation (`00000000-0000-0000-0000-000000000002`)
- Known backend bugs handled with `t.Skip()` + explanation
- Conditional tests skip when expected data doesn't exist

## 8. CI/CD Test Execution

**Pattern**: Progressive test execution based on trigger

```yaml
# Unit tests: All pushes
unit-tests:
  runs-on: ubuntu-latest
  run: go test -short ./pkg/...

# Integration tests: PRs and main
integration-tests:
  needs: unit-tests
  if: github.event_name == 'pull_request' || github.ref == 'refs/heads/main'
  services:
    postgres:
      image: pgvector/pgvector:pg16
  run: go test -tags=integration ./tests/integration/...

# E2E tests: Main only, self-hosted runner
e2e-tests:
  needs: integration-tests
  if: github.ref == 'refs/heads/main'
  runs-on: [self-hosted, macos, ARM64]
  run: go test -tags=e2e ./tests/e2e/...
```

## 9. Flaky Test Quarantine

**Pattern**: Isolate flaky tests with dedicated build tag

```go
//go:build flaky

func TestSometimesFails(t *testing.T) {
    // TODO: Fix by 2026-02-15 - describe root cause
    // This test is quarantined due to timing sensitivity
}
```

```bash
# Run quarantined tests separately
go test -tags=flaky ./... -v
```

## Performance Targets

| Test Tier | Target Duration | Dependencies |
|-----------|-----------------|--------------|
| Unit (per test) | <100ms | None |
| Unit (total) | <10s | None |
| Integration (total) | <60s | PostgreSQL |
| E2E (total) | <5min | PostgreSQL + LLM |
| Live | Varies | Cloud APIs |

## Security Patterns

### Test Data Privacy
- Fixtures use fictional "Acme Corp" organization
- No real PII in test data
- Sample emails are synthetic

### Credential Management
```bash
# Never commit credentials
source ~/github/otherjamesbrown/secrets/.env.penfold

# Environment-specific databases
export PENFOLD_DB_NAME=penfold_test_integration
```

### API Key Isolation
- Live tests use separate API keys
- Tests skip gracefully when keys missing
- No API calls in unit or integration tests
