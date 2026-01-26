# Test Summary

Run all available test tiers and present a comprehensive overview.

## Arguments: $ARGUMENTS

Optional:
- `quick` - Only run unit tests (fastest)
- `full` - Run unit + integration + e2e (requires DB + LLM)
- `all` - Run everything including live tests (incurs API costs)

Default: `quick`

## Instructions

### Step 1: Determine Test Scope

Based on arguments and available prerequisites:

```bash
MODE="${ARGUMENTS:-quick}"

# Check what's available
HAS_DB=false
HAS_LLM=false
HAS_GEMINI=false

if [ -n "$PENFOLD_DB_PASSWORD" ]; then
    HAS_DB=true
fi

LLM_URL="${LLM_URL:-http://localhost:8080}"
if curl -s "$LLM_URL/v1/models" > /dev/null 2>&1; then
    HAS_LLM=true
fi

if [ -n "$GEMINI_API_KEY" ]; then
    HAS_GEMINI=true
fi
```

### Step 2: Run Tests Based on Mode

**Quick mode** (default):
```bash
go test -short -json ./pkg/... 2>&1 | tee /tmp/test-unit.json
```

**Full mode**:
```bash
# Unit tests
go test -short -json ./pkg/... 2>&1 | tee /tmp/test-unit.json

# Integration tests (if DB available)
if [ "$HAS_DB" = true ]; then
    export PENFOLD_DB_NAME=penfold_test_integration
    go test -tags=integration -json ./tests/integration/... 2>&1 | tee /tmp/test-integration.json
fi

# E2E tests (if DB + LLM available)
if [ "$HAS_DB" = true ] && [ "$HAS_LLM" = true ]; then
    export PENFOLD_DB_NAME=penfold_test_e2e
    go test -tags=e2e -json -timeout 5m ./tests/e2e/... 2>&1 | tee /tmp/test-e2e.json
fi
```

**All mode**:
```bash
# Everything above plus...
if [ "$HAS_GEMINI" = true ]; then
    go test -tags=live -json -timeout 2m ./tests/live/... 2>&1 | tee /tmp/test-live.json
fi
```

### Step 3: Parse All Results

Aggregate results from all test runs into categories:

```
Tier        | Passed | Failed | Skipped | Duration
------------|--------|--------|---------|----------
Unit        |   45   |   2    |    3    |   4.2s
Integration |   12   |   0    |    0    |  18.5s
E2E         |    5   |   1    |    2    |  45.3s
Live        |    3   |   0    |    2    |   5.1s
------------|--------|--------|---------|----------
TOTAL       |   65   |   3    |    7    |  73.1s
```

### Step 4: Present Executive Summary

```
## Test Summary

### Overall Status: ⚠️ 3 Failures

| Tier | Status | Pass | Fail | Skip |
|------|--------|------|------|------|
| Unit | ⚠️ | 45 | 2 | 3 |
| Integration | ✅ | 12 | 0 | 0 |
| E2E | ⚠️ | 5 | 1 | 2 |
| Live | ✅ | 3 | 0 | 2 |

**Total Duration**: 73.1s
```

### Step 5: Show All Failures

```
### ❌ All Failures (3)

| Tier | Package | Test | Error |
|------|---------|------|-------|
| Unit | pkg/config | TestDSNFormat | Expected postgres://, got postgresql:// |
| Unit | pkg/ingest/storage | TestStoreEmail | unknown field 'Metadata' |
| E2E | tests/e2e | TestMentionResolution | LLM timeout after 30s |
```

### Step 6: Show Flaky Tests (if detected)

If a test passed on retry or has inconsistent results:

```
### 🎲 Potentially Flaky Tests

| Tier | Test | Notes |
|------|------|-------|
| E2E | TestLLMResponse | Passed on 2nd attempt |
```

### Step 7: Coverage Summary (if available)

```
### 📊 Coverage

| Package | Coverage |
|---------|----------|
| pkg/mentions | 82.3% |
| pkg/glossary | 78.1% |
| pkg/search | 65.4% |

**Overall**: 74.2%
```

### Step 8: Prerequisites Status

```
### 🔧 Environment

| Prerequisite | Status |
|--------------|--------|
| Database (dev02) | ✅ Connected |
| LLM (localhost:8080) | ✅ Available |
| Gemini API | ⏭️ Not configured |
| Gmail OAuth | ⏭️ Not configured |
```

### Step 9: Recommendations

Based on results:

```
### 📋 Recommended Actions

1. **Fix Unit Test Failures** (blocking)
   - `pkg/config`: Update DSN format test expectation
   - `pkg/ingest/storage`: Remove deprecated Metadata field

2. **Investigate E2E Timeout**
   - Check LLM server load: `curl localhost:8080/v1/models`
   - Consider increasing timeout for LLM tests

3. **Increase Coverage**
   - `pkg/search` is below 80% target
   - Run: `go test -coverprofile=coverage.out ./pkg/search/...`
```

### Step 10: Quick Commands

```
### 🚀 Quick Commands

Re-run failed tests:
  go test -v ./pkg/config/... -run TestDSNFormat
  go test -v ./pkg/ingest/storage/... -run TestStoreEmail
  go test -tags=e2e -v ./tests/e2e/... -run TestMentionResolution

Run specific tier:
  /test.unit
  /test.integration
  /test.e2e
  /test.live
```

## Modes Reference

| Mode | Unit | Integration | E2E | Live | Use Case |
|------|------|-------------|-----|------|----------|
| quick | ✅ | ❌ | ❌ | ❌ | Fast feedback during development |
| full | ✅ | ✅ | ✅ | ❌ | Pre-commit validation |
| all | ✅ | ✅ | ✅ | ✅ | Full system validation |
