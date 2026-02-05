# Run E2E Tests

Execute end-to-end tests with structured output. E2E tests validate complete workflows through the Gateway API.

## Arguments: $ARGUMENTS

Optional: Specific test pattern (e.g., `TestSLMPipeline.*`, `TestEnvironment`)
If not provided, runs all E2E tests.

## Prerequisites

E2E tests require:
- PostgreSQL SSL certs in `~/.postgresql/`
- PostgreSQL accessible at dev02.brown.chat
- Gateway service running on dev02
- Worker service running on dev01 (for pipeline processing)

**Note**: E2E tests call the Gateway API, which routes to backend services. They do NOT require direct LLM access from the test machine.

## Instructions

### Step 0: Run Health Check

Before running tests, verify infrastructure is healthy:

**Run `/penf.health` first.** If critical services are down (PostgreSQL, Gateway, Worker), fix those before proceeding.

### Step 1: Check Prerequisites

```bash
# Check SSL certs exist
if [ ! -f ~/.postgresql/postgresql.crt ]; then
    echo "❌ SSL certs not found in ~/.postgresql/"
    echo "Copy from dev01: scp dev01:~/.postgresql/{postgresql.crt,postgresql.key,root.crt} ~/.postgresql/"
    exit 1
fi
echo "✅ SSL certificates found"

# Check Gateway is reachable
if ! penf health > /dev/null 2>&1; then
    echo "⚠️  Gateway not reachable"
    echo "Check: penf health"
fi
```

### Step 2: Set Test Environment

```bash
# E2E tests use the main penfold database with tenant isolation
# No special database needed - tests use E2E test tenant
```

### Step 3: Run Tests with JSON Output

```bash
# Build test pattern
if [ -n "$ARGUMENTS" ]; then
    RUN_FLAG="-run $ARGUMENTS"
else
    RUN_FLAG=""
fi

go test -tags=e2e -json -timeout 5m $RUN_FLAG ./tests/e2e/... 2>&1 | tee /tmp/test-e2e.json
```

### Step 4: Parse and Categorize Results

Parse JSON output:
- **Passed**: Action="pass" with Test name
- **Failed**: Action="fail" with Test name
- **Skipped**: Action="skip" (missing prerequisites)

Note: E2E tests may have longer durations due to pipeline processing time.

### Step 5: Present Structured Summary

```
## E2E Test Results

| Status | Count |
|--------|-------|
| ✅ Passed | X |
| ❌ Failed | Y |
| ⏭️ Skipped | Z |

**Gateway**: dev02.brown.chat
**Worker**: dev01 (pipeline processing)
**Duration**: X.XXs
```

### Step 6: Show Failures First

```
### ❌ Failed Tests

| Test | Error |
|------|-------|
| TestMentionResolution | LLM timeout |
```

For LLM-related failures, show:
- The prompt sent
- The response received (if any)
- Expected vs actual behavior

### Step 7: Show Skipped Tests

```
### ⏭️ Skipped Tests

| Test | Reason |
|------|--------|
| TestMentionResolutionWithLLM | Local LLM not available |
```

### Step 8: Show Passed Tests with Timing

E2E test durations matter:

```
### ✅ Passed Tests

| Test | Duration |
|------|----------|
| TestEnvironmentSetup | 0.5s |
| TestMentionResolution | 12.3s |
| TestGlossaryLookup | 8.7s |
```

### Step 9: Pipeline Diagnostics (if failures)

```
### 🔧 Pipeline Diagnostics

Check Worker service (on dev01):
  ssh dev01 "systemctl --user status penfold-worker"
  ssh dev01 "journalctl --user -u penfold-worker -n 50"

Check Temporal workflows:
  penf pipeline status

Check source processing status:
  psql -h dev02.brown.chat -U penfold -d penfold -c \
    "SELECT processing_status, COUNT(*) FROM sources GROUP BY processing_status"

Check for stuck workflows:
  penf pipeline list --status=running
```

### Step 10: Recommendations

- If timeout waiting for processing: "Worker may not be running. Check: ssh dev01 systemctl --user status penfold-worker"
- If all pass: "E2E tests passing. Full system validation complete."
- If ingest works but processing fails: "Gateway working. Check Worker service on dev01."

## Performance Target

E2E tests should complete in <5 minutes total.
Individual LLM tests may take 10-30s due to inference time.
