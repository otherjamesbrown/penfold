# Run Integration Tests

Execute integration tests (database tests) with structured output.

## Arguments: $ARGUMENTS

Optional: Specific test pattern (e.g., `TestDatabaseConnection`, `TestFixture.*`)
If not provided, runs all integration tests.

## Prerequisites

Integration tests require:
- PostgreSQL SSL certs in `~/.postgresql/` (postgresql.crt, postgresql.key, root.crt)
- PostgreSQL accessible at dev02.brown.chat
- Test database `penfold_test_integration` with migrations applied

To set up SSL certs, copy from dev01:
```bash
mkdir -p ~/.postgresql
scp dev01:~/.postgresql/postgresql.crt ~/.postgresql/
scp dev01:~/.postgresql/postgresql.key ~/.postgresql/
scp dev01:~/.postgresql/root.crt ~/.postgresql/
chmod 600 ~/.postgresql/postgresql.key
```

## Instructions

### Step 0: Run Health Check

Before running tests, verify infrastructure is healthy:

**Run `/penf.health` first.** If any critical services are down (PostgreSQL, Gateway), fix those before proceeding.

### Step 1: Check Prerequisites and Run Tests

```bash
# Check SSL certs exist
if [ ! -f ~/.postgresql/postgresql.crt ]; then
    echo "❌ SSL certs not found in ~/.postgresql/"
    echo "Copy from dev01: scp dev01:~/.postgresql/{postgresql.crt,postgresql.key,root.crt} ~/.postgresql/"
    exit 1
fi
echo "✅ SSL certificates found"

# Build test pattern
if [ -n "$ARGUMENTS" ]; then
    RUN_FLAG="-run $ARGUMENTS"
else
    RUN_FLAG=""
fi

# Run tests (uses SSL cert auth configured in helpers.go)
PENFOLD_DB_NAME=penfold_test_integration go test -tags=integration -json $RUN_FLAG ./tests/integration/... 2>&1 | tee /tmp/test-integration.json
```

### Step 2: Parse and Categorize Results

Parse JSON output (same format as unit tests):
- **Passed**: Action="pass" with Test name
- **Failed**: Action="fail" with Test name
- **Skipped**: Action="skip" with Test name (usually missing DB credentials)

### Step 3: Present Structured Summary

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

### Step 4: Show Failures First

```
### ❌ Failed Tests

| Test | Error |
|------|-------|
| TestFixtureLoading | connection refused |
```

Include relevant output lines for debugging.

### Step 5: Show Skipped Tests

```
### ⏭️ Skipped Tests

| Test | Reason |
|------|--------|
| TestDatabaseConnection | PENFOLD_DB_PASSWORD not set |
```

### Step 6: Show Passed Tests

```
### ✅ Passed Tests

- TestDatabaseConnection
- TestFixtureLoading
- TestPeopleQuery
```

### Step 7: Database Diagnostics (if failures)

If tests fail, provide diagnostic commands:

```
### 🔧 Diagnostics

Check database connection:
  psql -h dev02.brown.chat -U penfold -d penfold_test_integration -c "SELECT 1"

Check database schema:
  psql -h dev02.brown.chat -U penfold -d penfold_test_integration -c "\dt"

Re-run with verbose output:
  go test -tags=integration -v ./tests/integration/... -run TestName
```

## Performance Target

Integration tests should complete in <60s total.
