# Local Test Setup Guide

Step-by-step guide for running all test types on your local development machine.

## Prerequisites

### Required Software
- Go 1.24+
- PostgreSQL client (`psql`)
- Access to dev02.brown.chat (for database)
- SSH certificates in `~/.postgresql/`

### Optional (for E2E tests)
- Access to dev01 (for LLM server)
- Local LLM server running (vLLM-MLX)

## Quick Setup

```bash
# 1. Load environment variables
source ~/github/otherjamesbrown/secrets/.env.penfold

# 2. Verify database connectivity
psql -h dev02.brown.chat -U penfold -d penfold -c "SELECT 1"

# 3. Run unit tests (no setup needed)
go test ./pkg/... -short
```

## Test Type Setup

### 1. Unit Tests

**Setup Required:** None

Unit tests are fully mocked and require no external dependencies.

```bash
# Run all unit tests
go test ./pkg/... ./cmd/... ./services/...

# Run with verbose output
go test -v ./pkg/...

# Run specific package
go test ./pkg/mentions/...

# Run with race detection
go test -race ./pkg/...
```

### 2. Integration Tests

**Setup Required:** Database access + SSL certificates

#### Step 1: Configure SSL Certificates

Ensure PostgreSQL SSL certificates exist:
```bash
ls -la ~/.postgresql/
# Should contain:
#   postgresql.crt
#   postgresql.key
#   root.crt
```

If missing, copy from secrets:
```bash
mkdir -p ~/.postgresql
cp ~/github/otherjamesbrown/secrets/certs/postgresql.* ~/.postgresql/
chmod 600 ~/.postgresql/postgresql.key
```

#### Step 2: Load Environment Variables

```bash
source ~/github/otherjamesbrown/secrets/.env.penfold

# Verify required variables are set
echo $PENFOLD_DB_HOST    # Should be: dev02.brown.chat
echo $PENFOLD_DB_USER    # Should be: penfold
```

#### Step 3: Verify Database Connection

```bash
# Test connection
psql -h dev02.brown.chat -U penfold -d penfold -c "SELECT current_database()"

# Check that test tenant exists
psql -h dev02.brown.chat -U penfold -d penfold -c \
  "SELECT id, name FROM tenants WHERE id = '00000000-0000-0000-0000-000000000002'"
```

#### Step 4: Run Integration Tests

```bash
# Run all integration tests
go test -tags=integration ./tests/integration/...

# Run with verbose output
go test -tags=integration -v ./tests/integration/...

# Run specific test file
go test -tags=integration -v ./tests/integration/cli_glossary_test.go ./tests/integration/helpers.go

# Run specific test
go test -tags=integration -v -run TestCLI_GlossaryList ./tests/integration/...
```

### 3. E2E Tests

**Setup Required:** Database + LLM server

#### Step 1: Complete Integration Test Setup (above)

#### Step 2: Verify LLM Server Availability

```bash
# Check if LLM server is running on dev01
curl -s http://localhost:8080/v1/models

# If on laptop, SSH tunnel to dev01:
ssh -L 8080:localhost:8080 dev01

# Or check if running locally
curl -s http://127.0.0.1:8080/v1/models
```

#### Step 3: Start LLM Server (if not running)

On dev01:
```bash
# Check status
launchctl list | grep mlx

# Start if not running
launchctl load ~/Library/LaunchAgents/com.penfold.mlx-llm-server.plist

# Check logs
tail -f /tmp/mlx-llm-server.log
```

#### Step 4: Run E2E Tests

```bash
# Set LLM URL
export LLM_URL=http://localhost:8080

# Run all E2E tests (use longer timeout)
go test -tags=e2e -v -timeout 15m ./tests/e2e/...

# Run specific test
go test -tags=e2e -v -run TestMentionResolution ./tests/e2e/...
```

### 4. Live Tests

**Setup Required:** Cloud API keys

Live tests call real cloud APIs (Gemini, Gmail) and may incur costs.

```bash
# Set API keys
export GEMINI_API_KEY="your-api-key"
export GMAIL_CREDENTIALS_FILE="path/to/credentials.json"

# Run live tests
go test -tags=live -v ./tests/live/...
```

### 5. Benchmark Tests

**Setup Required:** LLM server

```bash
# Run quick benchmark on current model
go test -tags=benchmark -v -run TestLLMQuickBench ./tests/benchmark/

# Run full model comparison (takes several minutes)
go test -tags=benchmark -v -timeout 30m -run TestLLMModelComparison ./tests/benchmark/

# Benchmark specific model
MODEL=phi go test -tags=benchmark -v -run TestSingleModelBenchmark ./tests/benchmark/
```

## Environment Variables Reference

| Variable | Required For | Default | Description |
|----------|--------------|---------|-------------|
| `PENFOLD_DB_HOST` | Integration, E2E | `dev02.brown.chat` | Database host |
| `PENFOLD_DB_PORT` | Integration, E2E | `5432` | Database port |
| `PENFOLD_DB_USER` | Integration, E2E | `penfold` | Database user |
| `PENFOLD_DB_NAME` | Integration, E2E | `penfold` | Database name |
| `LLM_URL` | E2E | `http://localhost:8080` | LLM server URL |
| `GEMINI_API_KEY` | Live | - | Google Gemini API key |

## Test Databases

Integration and E2E tests use tenant isolation, not separate databases:

| Test Type | Tenant ID | Description |
|-----------|-----------|-------------|
| Integration | `00000000-0000-0000-0000-000000000002` | Integration test tenant |
| E2E | `00000000-0000-0000-0000-000000000001` | E2E test tenant (default) |

Tests automatically clean up their tenant's data using `t.Cleanup()`.

## Running All Tests

```bash
# Unit tests only (fast, no dependencies)
make test

# Unit tests with coverage
make test-coverage

# Integration tests
go test -tags=integration ./tests/integration/...

# E2E tests (requires LLM)
go test -tags=e2e -timeout 15m ./tests/e2e/...

# All tests sequentially
make test && \
go test -tags=integration ./tests/integration/... && \
go test -tags=e2e -timeout 15m ./tests/e2e/...
```

## CI vs Local Differences

| Aspect | CI | Local |
|--------|-----|-------|
| Database | Fresh container | Shared dev02 |
| LLM | Self-hosted runner | SSH tunnel or local |
| Cleanup | Container disposal | Tenant isolation |
| Timeouts | Strict | Configurable |

## Common Issues

See [TROUBLESHOOTING.md](./TROUBLESHOOTING.md) for solutions to common problems.
