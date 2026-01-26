# Quickstart: Testing Strategy

**Feature**: 016-testing-strategy
**Date**: 2026-01-22

## Prerequisites

### For Unit Tests
- Go 1.22+
- No additional dependencies

### For Integration Tests
- Go 1.22+
- Network access to dev02.brown.chat
- Environment variables:
  ```bash
  export PENFOLD_DB_HOST=dev02.brown.chat
  export PENFOLD_DB_PORT=5432
  export PENFOLD_DB_NAME=penfold_test_integration
  export PENFOLD_DB_USER=penfold
  export PENFOLD_DB_PASSWORD=<from secrets>
  ```

### For E2E Tests
- All integration prerequisites, plus:
- Running on dev01.brown.chat (Apple Silicon required)
- MLX LLM server running: `http://localhost:8080`
- Environment:
  ```bash
  export PENFOLD_DB_NAME=penfold_test_e2e
  export LLM_URL=http://localhost:8080
  ```

### For Live Tests
- API keys configured:
  ```bash
  export GEMINI_API_KEY=<your-key>
  export GMAIL_OAUTH_CREDENTIALS=<path-to-credentials>
  ```

---

## Running Tests

### Unit Tests (Fast, No Dependencies)

```bash
# Run all unit tests
go test ./... -short

# Run with race detector
go test ./... -short -race

# Run specific package
go test ./pkg/mentions/... -short

# With verbose output
go test ./... -short -v
```

**Expected duration**: < 10 seconds

---

### Integration Tests (Requires Database)

```bash
# Source environment
source ~/github/otherjamesbrown/secrets/.env.penfold
export PENFOLD_DB_NAME=penfold_test_integration

# Run all integration tests
go test -tags=integration ./...

# Run specific integration tests
go test -tags=integration ./tests/integration/...

# Run with verbose output
go test -tags=integration -v ./tests/integration/...
```

**Expected duration**: < 60 seconds

---

### E2E Tests (Requires Database + LLM)

```bash
# Must run on dev01.brown.chat
# Verify LLM is available
curl -s http://localhost:8080/v1/models

# Source environment
source ~/github/otherjamesbrown/secrets/.env.penfold
export PENFOLD_DB_NAME=penfold_test_e2e
export LLM_URL=http://localhost:8080

# Run E2E tests
go test -tags=e2e ./tests/e2e/... -v

# Run specific E2E test
go test -tags=e2e -run TestEmailIngestion ./tests/e2e/...
```

**Expected duration**: < 5 minutes

---

### Live Tests (Requires API Keys)

```bash
# These incur costs - run sparingly
export GEMINI_API_KEY=<your-key>

# Run live tests
go test -tags=live ./tests/live/... -v
```

**Expected duration**: Varies (network-dependent)

---

### All Tests with Coverage

```bash
# Run all tests with coverage report
go test ./... -coverprofile=coverage.out -covermode=atomic

# View coverage
go tool cover -html=coverage.out

# Check coverage percentage
go tool cover -func=coverage.out | grep total
```

---

## Test Database Setup

### Create Test Databases (One-Time)

```bash
# Connect to dev02 PostgreSQL
psql "host=dev02.brown.chat user=penfold password=<password> dbname=penfold"

# Create test databases
CREATE DATABASE penfold_test_integration;
CREATE DATABASE penfold_test_e2e;

# Grant permissions
GRANT ALL PRIVILEGES ON DATABASE penfold_test_integration TO penfold;
GRANT ALL PRIVILEGES ON DATABASE penfold_test_e2e TO penfold;

# Enable pgvector in each database
\c penfold_test_integration
CREATE EXTENSION IF NOT EXISTS vector;

\c penfold_test_e2e
CREATE EXTENSION IF NOT EXISTS vector;
```

### Apply Migrations to Test Databases

```bash
# Migrations are in ./migrations/*.sql - apply using psql
# Integration DB
psql -h dev02.brown.chat -U penfold -d penfold_test_integration -f migrations/*.sql

# E2E DB
psql -h dev02.brown.chat -U penfold -d penfold_test_e2e -f migrations/*.sql
```

---

## CI/CD

### GitHub Actions Workflow

Tests run automatically on:
- **Push to any branch**: Unit tests
- **Pull request**: Unit + Integration tests
- **Push to main**: Unit + Integration + E2E tests

### Self-Hosted Runner (E2E)

E2E tests run on the self-hosted runner on dev01.brown.chat:

```bash
# Check runner status
ssh dev01.brown.chat "~/actions-runner/svc.sh status"

# View runner logs
ssh dev01.brown.chat "tail -f ~/actions-runner/_diag/Runner_*.log"
```

---

## Troubleshooting

### "Local LLM not available" Skip

E2E tests skip if LLM is unavailable:
```
=== SKIP: TestEmailIngestion_MentionResolution
    environment.go:45: Local LLM not available - skipping E2E test
```

**Fix**: Ensure MLX LLM server is running on dev01:
```bash
launchctl list | grep penfold.mlx-llm-server
# If not running:
launchctl load ~/Library/LaunchAgents/com.penfold.mlx-llm-server.plist
```

### Database Connection Failed

```
Error: failed to connect to host=dev02.brown.chat
```

**Fix**: Check network and credentials:
```bash
# Test connection
psql "host=dev02.brown.chat user=penfold dbname=penfold_test_integration"

# Verify environment
echo $PENFOLD_DB_HOST
echo $PENFOLD_DB_PASSWORD
```

### Flaky Test Quarantine

If a test is intermittently failing:
```go
//go:build flaky

func TestSometimesFails(t *testing.T) {
    // TODO: Fix by YYYY-MM-DD - describe root cause
}
```

Run quarantined tests separately:
```bash
go test -tags=flaky ./... -v
```

---

## Directory Reference

```
tests/
├── go.mod                # Test module
├── integration/          # DB integration tests
│   ├── db_test.go        # Database connectivity, pgvector
│   ├── fixtures_test.go  # Fixture loading tests
│   ├── helpers.go        # SetupTestDB, TruncateAllTables
│   └── testdata/         # SQL seed scripts
├── e2e/                  # End-to-end tests (requires LLM)
│   ├── environment_test.go    # Setup verification
│   ├── mention_resolution_test.go  # LLM-based mention tests
│   ├── helpers.go        # SetupE2EEnvironment
│   ├── assertions.go     # Semantic test assertions
│   └── llm_client.go     # OpenAI-compatible LLM client
├── live/                 # Cloud API tests (incurs costs)
│   ├── gemini_test.go    # Gemini API tests
│   ├── gmail_test.go     # Gmail API tests
│   └── helpers.go        # RequireGeminiAPIKey, etc.
└── fixtures/
    └── acme-corp/        # Mock organization (20 people, 50+ terms)
        ├── people.yaml   # 20 employees with aliases
        ├── teams.yaml    # 7 teams with hierarchy
        ├── projects.yaml # 10 projects
        ├── products.yaml # 5 products
        ├── glossary.yaml # 50+ terms with expansions
        └── emails/       # 10 sample emails (RFC 5322)

pkg/testfixtures/         # Fixture loader package
├── types.go              # PersonFixture, TeamFixture, etc.
├── loader.go             # LoadAcmeCorp, LoadPeople, etc.
└── validate_test.go      # YAML validation tests
```
