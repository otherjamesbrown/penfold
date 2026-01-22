# Testing Development Agent Context

This context enables AI agents to work effectively with Penfold's Go testing framework, implementing comprehensive testing strategies including database integration, LLM testing, and fixture management.

## 🎯 Agent Expertise

**Primary Skills**: Go testing, build tags, testify assertions, pgx database testing, LLM integration testing, fixture management

**Key Responsibilities**:
- Test infrastructure design with build tags for test categorization
- Database integration testing with real PostgreSQL + pgvector
- E2E testing with local LLM (vLLM-MLX/Qwen)
- Fixture management for mock organizations
- CI/CD integration with self-hosted runners

## 🏗️ Test Architecture

### Four-Tier Test Taxonomy

| Tier | Build Tag | Dependencies | Location | Duration |
|------|-----------|--------------|----------|----------|
| Unit | (none) | Mocks only | Co-located with source | <10s total |
| Integration | `integration` | Real PostgreSQL | `tests/integration/` | <60s |
| E2E | `e2e` | PostgreSQL + LLM | `tests/e2e/` | <5min |
| Live | `live` | Cloud APIs | `tests/live/` | Varies |

### Directory Structure

```
pkg/                           # Unit tests co-located with source
├── mentions/
│   ├── resolver.go
│   └── resolver_test.go      # Unit test (no build tag)
├── glossary/
│   └── glossary_test.go
└── testfixtures/             # Shared fixture library
    ├── types.go              # PersonFixture, TeamFixture, etc.
    ├── loader.go             # LoadAcmeCorp(), LoadPeople(), etc.
    └── validate_test.go      # YAML validation tests

tests/                        # Special test categories
├── go.mod                    # Separate module for tests
├── integration/              # Build tag: integration
│   ├── helpers.go            # SetupTestDB, TruncateAllTables
│   ├── db_test.go
│   └── fixtures_test.go
├── e2e/                      # Build tag: e2e
│   ├── helpers.go            # SetupE2EEnvironment, LLMClient
│   ├── assertions.go         # Semantic assertions
│   ├── llm_client.go         # OpenAI-compatible client
│   └── mention_resolution_test.go
├── live/                     # Build tag: live
│   ├── helpers.go            # RequireGeminiAPIKey, etc.
│   ├── gemini_test.go
│   └── gmail_test.go
└── fixtures/
    └── acme-corp/            # Mock organization
        ├── people.yaml       # 20 employees
        ├── teams.yaml        # 7 teams
        ├── projects.yaml     # 10 projects
        ├── glossary.yaml     # 50+ terms
        └── emails/           # 10 sample emails
```

## 🔧 Implementation Patterns

### Unit Tests (Co-located, No Build Tag)

```go
// pkg/mentions/resolver_test.go
package mentions

import (
    "testing"
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
)

// Mock the LLM client for unit tests
type MockLLMClient struct {
    mock.Mock
}

func (m *MockLLMClient) Complete(ctx context.Context, prompt string) (string, error) {
    args := m.Called(ctx, prompt)
    return args.String(0), args.Error(1)
}

func TestResolveMention_ExactMatch(t *testing.T) {
    mockLLM := new(MockLLMClient)
    mockLLM.On("Complete", mock.Anything, mock.Anything).Return(`{"person_id": 1}`, nil)

    resolver := NewResolver(mockLLM)
    result, err := resolver.Resolve(context.Background(), "John Smith")

    assert.NoError(t, err)
    assert.Equal(t, int64(1), result.PersonID)
    mockLLM.AssertExpectations(t)
}
```

### Integration Tests (Build Tag: integration)

```go
//go:build integration

// tests/integration/db_test.go
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

### E2E Tests (Build Tag: e2e)

```go
//go:build e2e

// tests/e2e/mention_resolution_test.go
package e2e

import (
    "context"
    "testing"
    "github.com/stretchr/testify/require"
)

func TestMentionResolutionWithLLM(t *testing.T) {
    env := SetupE2EEnvironment(t)  // Requires DB + LLM

    // Load fixtures
    err := env.LoadFixture("acme-corp")
    require.NoError(t, err)

    client := NewLLMClient(env.LLMURL)
    ctx := context.Background()

    // Build context from database
    peopleContext := buildPeopleContext(t, env)

    // Test with real LLM
    response, err := client.CompleteWithSystem(ctx,
        "You are a mention resolution system...",
        fmt.Sprintf(`Text: "John mentioned the timeline concerns."

%s

Who is mentioned?`, peopleContext),
    )
    require.NoError(t, err)

    // Semantic assertion (LLM output is non-deterministic)
    assert.Contains(t, response, "John Smith")
}
```

### Live Tests (Build Tag: live)

```go
//go:build live

// tests/live/gemini_test.go
package live

import (
    "testing"
    "github.com/stretchr/testify/require"
)

func TestGeminiAPIConnection(t *testing.T) {
    apiKey := RequireGeminiAPIKey(t)  // Skips if not set

    // Test real API call
    resp, err := callGeminiAPI(apiKey, "What is 2+2?")
    require.NoError(t, err)
    require.Contains(t, resp, "4")
}
```

## 🗃️ Fixture Management

### Acme Corp Mock Organization

The `tests/fixtures/acme-corp/` directory contains a complete mock organization:

**People (20 employees)**:
- John Smith (VP Engineering)
- Sarah Chen (Product Manager)
- Marcus Rodriguez (Sales Director)
- ...with aliases, titles, team assignments

**Teams (7 teams)**:
- Engineering, Product, Sales (top-level)
- Platform, Frontend, Infrastructure, Design (sub-teams)

**Glossary (50+ terms)**:
- TER → Technical Execution Review
- MVP → Minimum Viable Product
- Project Alpha → linked to project_id: 1

**Sample Emails (10)**:
- Project updates, incident responses, code reviews
- RFC 5322 format with realistic headers

### Loading Fixtures

```go
import "github.com/otherjamesbrown/penfold/pkg/testfixtures"

func TestWithFixtures(t *testing.T) {
    db := SetupTestDB(t)

    loader := testfixtures.NewLoader(db.Pool, "tests/fixtures/acme-corp")

    ctx := context.Background()
    err := loader.LoadAcmeCorp(ctx)  // Loads all fixtures
    require.NoError(t, err)

    // Or load individually
    err = loader.LoadPeople(ctx)
    err = loader.LoadGlossary(ctx)
}
```

## 🖥️ Infrastructure

### Database Setup

```bash
# Integration tests use: penfold_test_integration
# E2E tests use: penfold_test_e2e

# Create databases (one-time)
psql -h home-01.brown.chat -U penfold -d penfold <<EOF
CREATE DATABASE penfold_test_integration;
CREATE DATABASE penfold_test_e2e;
\c penfold_test_integration
CREATE EXTENSION IF NOT EXISTS vector;
\c penfold_test_e2e
CREATE EXTENSION IF NOT EXISTS vector;
EOF

# Run migrations
PENFOLD_DB_NAME=penfold_test_integration go run ./cmd/penf migrate up
PENFOLD_DB_NAME=penfold_test_e2e go run ./cmd/penf migrate up
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

## 🚀 Running Tests

```bash
# Unit tests (fast, no dependencies)
go test ./pkg/... -short

# Integration tests (requires DB)
source ~/github/otherjamesbrown/secrets/.env.penfold
export PENFOLD_DB_NAME=penfold_test_integration
go test -tags=integration ./tests/integration/...

# E2E tests (requires DB + LLM, run on dev01)
export PENFOLD_DB_NAME=penfold_test_e2e
export LLM_URL=http://localhost:8080
go test -tags=e2e ./tests/e2e/... -v

# Live tests (incurs API costs)
export GEMINI_API_KEY=...
go test -tags=live ./tests/live/... -v

# All tests with coverage
go test ./pkg/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

## 📊 CI/CD Integration

### GitHub Actions Workflow

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

## 🎯 Success Criteria

When implementing tests:

✅ **Unit tests** are co-located with source, use mocks, run in <10s
✅ **Integration tests** use build tag, test real DB operations
✅ **E2E tests** use build tag, test with real LLM, use semantic assertions
✅ **Live tests** use build tag, skip gracefully when credentials missing
✅ **Fixtures** are loaded via `pkg/testfixtures` loader
✅ **Coverage** meets 80% for core packages

## 🔗 Key Resources

- **Spec**: `specs/016-testing-strategy/spec.md`
- **Quickstart**: `specs/016-testing-strategy/quickstart.md`
- **Fixtures**: `tests/fixtures/acme-corp/`
- **Loader**: `pkg/testfixtures/`
- **CI Workflow**: `.github/workflows/test.yml`
