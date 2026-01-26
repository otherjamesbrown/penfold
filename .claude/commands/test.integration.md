# Run Integration Tests

Execute integration tests (database tests) with structured output.

## Arguments: $ARGUMENTS

Optional: Specific test pattern (e.g., `TestDatabaseConnection`, `TestFixture.*`)
If not provided, runs all integration tests.

## Prerequisites

Integration tests require:
- `PENFOLD_DB_PASSWORD` environment variable set
- PostgreSQL accessible at dev02.brown.chat
- Test database `penfold_test_integration` with migrations applied

## Instructions

### Step 1: Check Prerequisites

```bash
if [ -z "$PENFOLD_DB_PASSWORD" ]; then
    echo "❌ PENFOLD_DB_PASSWORD not set"
    echo "Run: source ~/github/otherjamesbrown/secrets/.env.penfold"
    exit 1
fi
```

### Step 2: Set Test Database

```bash
export PENFOLD_DB_NAME=penfold_test_integration
```

### Step 3: Run Tests with JSON Output

```bash
# Build test pattern
if [ -n "$ARGUMENTS" ]; then
    RUN_FLAG="-run $ARGUMENTS"
else
    RUN_FLAG=""
fi

go test -tags=integration -json $RUN_FLAG ./tests/integration/... 2>&1 | tee /tmp/test-integration.json
```

### Step 4: Parse and Categorize Results

Parse JSON output (same format as unit tests):
- **Passed**: Action="pass" with Test name
- **Failed**: Action="fail" with Test name
- **Skipped**: Action="skip" with Test name (usually missing DB credentials)

### Step 5: Present Structured Summary

```
## Integration Test Results

| Status | Count |
|--------|-------|
| ✅ Passed | X |
| ❌ Failed | Y |
| ⏭️ Skipped | Z |

**Database**: penfold_test_integration @ dev02.brown.chat
**Duration**: X.XXs
```

### Step 6: Show Failures First

```
### ❌ Failed Tests

| Test | Error |
|------|-------|
| TestFixtureLoading | connection refused |
```

Include relevant output lines for debugging.

### Step 7: Show Skipped Tests

```
### ⏭️ Skipped Tests

| Test | Reason |
|------|--------|
| TestDatabaseConnection | PENFOLD_DB_PASSWORD not set |
```

### Step 8: Show Passed Tests

```
### ✅ Passed Tests

- TestDatabaseConnection
- TestFixtureLoading
- TestPeopleQuery
```

### Step 9: Database Diagnostics (if failures)

If tests fail, provide diagnostic commands:

```
### 🔧 Diagnostics

Check database connection:
  psql -h dev02.brown.chat -U penfold -d penfold_test_integration -c "SELECT 1"

Check migrations:
  PENFOLD_DB_NAME=penfold_test_integration go run ./cmd/penf migrate status

Re-run with verbose output:
  go test -tags=integration -v ./tests/integration/... -run TestName
```

## Performance Target

Integration tests should complete in <60s total.
