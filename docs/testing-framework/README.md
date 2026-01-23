# Penfold Testing Framework

Go testing framework for Penfold's AI-first applications, using build tags for test categorization, testify for assertions, and YAML fixtures for test data.

## Quick Start

```bash
# Run unit tests (fast, fully mocked)
go test ./pkg/... -short

# Run integration tests (with PostgreSQL)
source ~/github/otherjamesbrown/secrets/.env.penfold
export PENFOLD_DB_NAME=penfold_test_integration
go test -tags=integration ./tests/integration/...

# Run E2E tests (with PostgreSQL + LLM, on dev01)
export PENFOLD_DB_NAME=penfold_test_e2e
export LLM_URL=http://localhost:8080
go test -tags=e2e ./tests/e2e/... -v

# Run live tests (incurs API costs)
export GEMINI_API_KEY=...
go test -tags=live ./tests/live/... -v
```

## Testing Tiers

### 1. Unit Tests (<100ms per test)
- **Purpose**: Fast, isolated component testing
- **AI Strategy**: Full mocking with testify/mock
- **Database**: None (mocked)
- **Location**: Co-located with source (e.g., `pkg/mentions/resolver_test.go`)

```go
// pkg/mentions/resolver_test.go
package mentions

import (
    "context"
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
)

type MockLLMClient struct {
    mock.Mock
}

func (m *MockLLMClient) Complete(ctx context.Context, prompt string) (string, error) {
    args := m.Called(ctx, prompt)
    return args.String(0), args.Error(1)
}

func TestResolveMention_ExactMatch(t *testing.T) {
    mockLLM := new(MockLLMClient)
    mockLLM.On("Complete", mock.Anything, mock.Anything).
        Return(`{"person_id": 1, "confidence": 0.95}`, nil)

    resolver := NewResolver(mockLLM)
    result, err := resolver.Resolve(context.Background(), "John Smith")

    assert.NoError(t, err)
    assert.Equal(t, int64(1), result.PersonID)
    mockLLM.AssertExpectations(t)
}
```

### 2. Integration Tests (<60s total)
- **Purpose**: Database and multi-component testing
- **AI Strategy**: Mocked (no LLM calls)
- **Database**: PostgreSQL with automatic cleanup
- **Location**: `tests/integration/`

```go
//go:build integration

package integration

import (
    "context"
    "testing"
    "github.com/stretchr/testify/require"
)

func TestDatabaseConnection(t *testing.T) {
    db := SetupTestDB(t)  // Connects to penfold_test_integration

    ctx := context.Background()
    var result int
    err := db.Pool.QueryRow(ctx, "SELECT 1").Scan(&result)

    require.NoError(t, err)
    require.Equal(t, 1, result)
}

func TestFixtureLoading(t *testing.T) {
    db := SetupTestDB(t)
    loader := db.FixtureLoader()

    ctx := context.Background()
    err := loader.LoadAcmeCorp(ctx)
    require.NoError(t, err)

    // Verify people loaded
    var count int
    err = db.Pool.QueryRow(ctx, "SELECT COUNT(*) FROM people").Scan(&count)
    require.NoError(t, err)
    require.GreaterOrEqual(t, count, 20)
}
```

### 3. E2E Tests (<5min total)
- **Purpose**: Complete workflow validation with real LLM
- **AI Strategy**: Real local LLM (Qwen via vLLM-MLX)
- **Database**: PostgreSQL with test fixtures
- **Location**: `tests/e2e/`

```go
//go:build e2e

package e2e

import (
    "context"
    "testing"
    "github.com/stretchr/testify/require"
    "github.com/stretchr/testify/assert"
)

func TestMentionResolutionWithLLM(t *testing.T) {
    env := SetupE2EEnvironment(t)  // Requires DB + LLM

    // Load fixtures
    err := env.LoadFixture("acme-corp")
    require.NoError(t, err)

    client := NewLLMClient(env.LLMURL)
    ctx := context.Background()

    response, err := client.CompleteWithSystem(ctx,
        "You are a mention resolution system...",
        `Text: "John mentioned the timeline concerns."
Who is mentioned?`,
    )
    require.NoError(t, err)

    // Semantic assertion (LLM output is non-deterministic)
    assert.Contains(t, response, "John")
}
```

### 4. Live Tests (Varies)
- **Purpose**: Cloud API connectivity validation
- **AI Strategy**: Real cloud APIs (Gemini, etc.)
- **Database**: Optional
- **Location**: `tests/live/`

```go
//go:build live

package live

import (
    "testing"
    "github.com/stretchr/testify/require"
)

func TestGeminiAPIConnection(t *testing.T) {
    apiKey := RequireGeminiAPIKey(t)  // Skips if not set

    resp, err := callGeminiAPI(apiKey, "What is 2+2?")
    require.NoError(t, err)
    require.Contains(t, resp, "4")
}
```

## Test Environment Setup

### Local Development
```bash
# Set up environment
source ~/github/otherjamesbrown/secrets/.env.penfold

# Run migrations on test databases (one-time)
PENFOLD_DB_NAME=penfold_test_integration go run ./cmd/penf migrate up
PENFOLD_DB_NAME=penfold_test_e2e go run ./cmd/penf migrate up

# Run unit tests
go test ./pkg/... -short

# Run with verbose output
go test ./pkg/... -v
```

### Test Databases
```bash
# Integration tests use:
export PENFOLD_DB_NAME=penfold_test_integration

# E2E tests use:
export PENFOLD_DB_NAME=penfold_test_e2e

# Both connect to home-01.brown.chat
```

### LLM Setup (E2E Tests)
E2E tests require the local LLM server on dev01:
- **Model**: Qwen2.5-32B-Instruct-4bit via vLLM-MLX
- **Endpoint**: http://localhost:8080
- **API**: OpenAI-compatible

```bash
# Check LLM availability
curl -s http://localhost:8080/v1/models

# Start if not running
launchctl load ~/Library/LaunchAgents/com.penfold.mlx-llm-server.plist
```

## AI Model Mocking

### Unit Test Mocking
```go
// Use testify/mock for deterministic AI responses
type MockLLMClient struct {
    mock.Mock
}

func (m *MockLLMClient) Complete(ctx context.Context, prompt string) (string, error) {
    args := m.Called(ctx, prompt)
    return args.String(0), args.Error(1)
}

func TestSummarization(t *testing.T) {
    mockLLM := new(MockLLMClient)
    mockLLM.On("Complete", mock.Anything, mock.MatchedBy(func(p string) bool {
        return strings.Contains(p, "summarize")
    })).Return("Concise summary of the content", nil)

    summarizer := NewSummarizer(mockLLM)
    result, err := summarizer.Summarize(ctx, "Long content...")

    assert.NoError(t, err)
    assert.Contains(t, result, "summary")
}
```

### E2E LLM Testing
```go
// Use real LLM with semantic assertions
func TestLLMIntegration(t *testing.T) {
    env := SetupE2EEnvironment(t)
    client := NewLLMClient(env.LLMURL)

    response, err := client.CompleteWithSystem(ctx,
        "Extract person names from the text",
        "John Smith met with Sarah Chen yesterday",
    )
    require.NoError(t, err)

    // Semantic assertion - verify structure, not exact text
    assert.Contains(t, response, "John")
    assert.Contains(t, response, "Sarah")
}
```

## Test Data Management

### Acme Corp Fixtures
The `tests/fixtures/acme-corp/` directory contains a complete mock organization:

| File | Contents |
|------|----------|
| `people.yaml` | 20 employees with aliases, titles, teams, managers |
| `teams.yaml` | 7 teams with descriptions |
| `projects.yaml` | 10 projects with assignments and status |
| `products.yaml` | Product definitions with team ownership |
| `glossary.yaml` | 50+ business terms, acronyms, and linked entities |
| `emails/` | Sample RFC 5322 emails |
| `meetings/` | Sample meeting transcripts |

### Loading Fixtures
```go
import "github.com/otherjamesbrown/penfold/pkg/testfixtures"

func TestWithFixtures(t *testing.T) {
    db := SetupTestDB(t)

    loader := testfixtures.NewLoader(db.Pool, "tests/fixtures/acme-corp")

    ctx := context.Background()
    err := loader.LoadAcmeCorp(ctx)  // Loads all fixtures in dependency order
    require.NoError(t, err)

    // Or load individually
    err = loader.LoadTeams(ctx)     // Teams first
    err = loader.LoadPeople(ctx)    // People depend on teams
    err = loader.LoadProjects(ctx)  // Projects depend on teams and people
    err = loader.LoadProducts(ctx)
    err = loader.LoadGlossary(ctx)

    // Cleanup
    err = loader.TruncateAll(ctx)
}

// For custom tenant isolation
func TestWithTenant(t *testing.T) {
    db := SetupTestDB(t)

    tenantID := "custom-tenant-uuid"
    loader := testfixtures.NewLoaderWithTenant(db.Pool, "tests/fixtures/acme-corp", tenantID)

    ctx := context.Background()
    err := loader.LoadAcmeCorp(ctx)
    require.NoError(t, err)
}
```

### Fixture Types
```go
// pkg/testfixtures/types.go
type PersonFixture struct {
    ID            int64    `yaml:"id"`
    CanonicalName string   `yaml:"canonical_name"`
    Email         string   `yaml:"email"`
    Aliases       []string `yaml:"aliases"`
    Title         string   `yaml:"title"`
    TeamID        int64    `yaml:"team_id"`
    ManagerID     *int64   `yaml:"manager_id"`
}

type TeamFixture struct {
    ID          int64  `yaml:"id"`
    Name        string `yaml:"name"`
    Description string `yaml:"description"`
}

type ProjectFixture struct {
    ID          int64  `yaml:"id"`
    Name        string `yaml:"name"`
    Slug        string `yaml:"slug"`
    Description string `yaml:"description"`
    Status      string `yaml:"status"`
    OwnerID     int64  `yaml:"owner_id"`
    TeamID      int64  `yaml:"team_id"`
}

type ProductFixture struct {
    ID          int64  `yaml:"id"`
    Name        string `yaml:"name"`
    Slug        string `yaml:"slug"`
    Description string `yaml:"description"`
    TeamID      *int64 `yaml:"team_id"`
}

type GlossaryTermFixture struct {
    Term             string   `yaml:"term"`
    Expansion        *string  `yaml:"expansion"`
    Definition       *string  `yaml:"definition"`
    Context          *string  `yaml:"context"`
    Aliases          []string `yaml:"aliases"`
    LinkedEntityType *string  `yaml:"linked_entity_type"`
    LinkedEntityID   *int64   `yaml:"linked_entity_id"`
}
```

## Test Configuration

### Build Tags
```go
//go:build integration    // Database tests
//go:build e2e            // Full system tests with LLM
//go:build live           // Cloud API tests
//go:build flaky          // Quarantined tests
```

### Environment Variables
```bash
# Database configuration
export PENFOLD_DB_HOST=home-01.brown.chat
export PENFOLD_DB_PORT=5432
export PENFOLD_DB_USER=penfold
export PENFOLD_DB_PASSWORD=<from secrets>
export PENFOLD_DB_NAME=penfold_test_integration

# LLM configuration (E2E tests)
export LLM_URL=http://localhost:8080

# Cloud API keys (Live tests)
export GEMINI_API_KEY=<your key>
```

### Graceful Skipping
```go
func SetupTestDB(t *testing.T) *TestDB {
    password := os.Getenv("PENFOLD_DB_PASSWORD")
    if password == "" {
        t.Skip("PENFOLD_DB_PASSWORD not set - skipping integration test")
    }
    // ...
}

func SetupE2EEnvironment(t *testing.T) *E2EEnv {
    env := &E2EEnv{...}
    if !env.LLMAvailable() {
        t.Skip("Local LLM not available - skipping E2E test")
    }
    return env
}
```

## Performance Targets

| Test Tier | Target Duration | Dependencies |
|-----------|-----------------|--------------|
| Unit (per test) | <100ms | None |
| Unit (total) | <10s | None |
| Integration (total) | <60s | PostgreSQL |
| E2E (total) | <5min | PostgreSQL + LLM |
| Live | Varies | Cloud APIs |

## Best Practices

### Test Organization
- Co-locate unit tests with source (`pkg/mentions/resolver_test.go`)
- Place integration tests in `tests/integration/`
- Place E2E tests in `tests/e2e/`
- Place live tests in `tests/live/`
- Use descriptive test names: `TestMentionResolution_ExactMatch`

### AI Testing Guidelines
- Use full mocking for unit tests (fast, deterministic)
- Use real LLM for E2E tests with semantic assertions
- Test with `temperature=0` for more deterministic output
- Validate AI response structure, not exact text
- Test confidence thresholds and edge cases

### Database Testing
- Use `t.Cleanup()` for automatic resource cleanup
- Truncate tables between tests for isolation
- Load fixtures via `pkg/testfixtures` loader
- Never use production database for tests

### Flaky Test Quarantine
```go
//go:build flaky

func TestSometimesFails(t *testing.T) {
    // TODO: Fix by 2026-02-15 - describe root cause
    // This test is quarantined due to timing sensitivity
}
```

## CI/CD Integration

```yaml
# .github/workflows/test.yml
jobs:
  unit-tests:
    runs-on: ubuntu-latest
    steps:
      - run: go test -short ./pkg/...

  integration-tests:
    runs-on: ubuntu-latest
    needs: unit-tests
    services:
      postgres:
        image: pgvector/pgvector:pg16
    steps:
      - run: go test -tags=integration ./tests/integration/...

  e2e-tests:
    runs-on: [self-hosted, macos, ARM64]  # dev01
    needs: integration-tests
    if: github.ref == 'refs/heads/main'
    steps:
      - run: go test -tags=e2e ./tests/e2e/...
```

## Directory Structure

```
pkg/                           # Unit tests co-located with source
├── mentions/
│   ├── resolver.go
│   └── resolver_test.go      # No build tag
├── glossary/
│   └── glossary_test.go
└── testfixtures/             # Shared fixture library
    ├── types.go              # PersonFixture, TeamFixture, etc.
    ├── loader.go             # NewLoader, LoadAcmeCorp, etc.
    └── validate_test.go

tests/                        # Special test categories
├── integration/              # Build tag: integration
│   ├── helpers.go            # SetupTestDB, TruncateAllTables
│   ├── db_test.go
│   ├── fixtures_test.go
│   ├── glossary_test.go
│   ├── mentions_test.go
│   ├── search_test.go
│   └── testdata/
├── e2e/                      # Build tag: e2e
│   ├── helpers.go            # SetupE2EEnvironment, LoadFixture
│   ├── assertions.go         # AssertMentionResolved, semantic helpers
│   ├── llm_client.go         # OpenAI-compatible client
│   ├── resolver_adapters.go  # Test adapters for resolver
│   ├── mention_resolution_test.go
│   ├── pipeline_test.go
│   ├── ingest_test.go
│   ├── search_test.go
│   └── environment_test.go
├── live/                     # Build tag: live
│   ├── helpers.go            # RequireGeminiAPIKey, RequireGmailCredentials
│   ├── gemini_test.go
│   └── gmail_test.go
└── fixtures/
    └── acme-corp/            # Mock organization
        ├── people.yaml       # 20 employees
        ├── teams.yaml        # 7 teams
        ├── projects.yaml     # 10 projects
        ├── products.yaml     # Products
        ├── glossary.yaml     # 50+ terms
        ├── emails/           # Sample emails
        └── meetings/         # Sample meetings
```

## Troubleshooting

### Database Connection Issues
```bash
# Verify environment
echo $PENFOLD_DB_PASSWORD

# Test connection
psql -h home-01.brown.chat -U penfold -d penfold_test_integration

# Check migrations
PENFOLD_DB_NAME=penfold_test_integration go run ./cmd/penf migrate status
```

### LLM Not Available
```bash
# Check if LLM server is running
curl -s http://localhost:8080/v1/models

# Start LLM server
launchctl load ~/Library/LaunchAgents/com.penfold.mlx-llm-server.plist

# Check logs
tail -f /tmp/mlx-llm-server.log
```

### Test Skipping
Tests skip gracefully when prerequisites are missing:
- Integration tests skip without `PENFOLD_DB_PASSWORD`
- E2E tests skip without LLM availability
- Live tests skip without API keys

For more detailed information, see:
- [AI Mocking Strategies](./ai-mocking.md)
- [Testing Patterns](../../context/architecture/testing-patterns.md)
- [Testing Agent Context](../../context/testing-dev/agents.md)
