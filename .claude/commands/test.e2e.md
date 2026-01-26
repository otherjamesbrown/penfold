# Run E2E Tests

Execute end-to-end tests (database + LLM) with structured output.

## Arguments: $ARGUMENTS

Optional: Specific test pattern (e.g., `TestMentionResolution.*`, `TestEnvironment`)
If not provided, runs all E2E tests.

## Prerequisites

E2E tests require:
- `PENFOLD_DB_PASSWORD` environment variable set
- PostgreSQL accessible at dev02.brown.chat
- Test database `penfold_test_e2e` with migrations applied
- Local LLM server running at http://localhost:8080 (vLLM-MLX with Qwen)

**Note**: E2E tests should be run on dev01 where the LLM server is available.

## Instructions

### Step 1: Check Prerequisites

```bash
# Check database credentials
if [ -z "$PENFOLD_DB_PASSWORD" ]; then
    echo "❌ PENFOLD_DB_PASSWORD not set"
    echo "Run: source ~/github/otherjamesbrown/secrets/.env.penfold"
    exit 1
fi

# Check LLM availability
LLM_URL="${LLM_URL:-http://localhost:8080}"
if ! curl -s "$LLM_URL/v1/models" > /dev/null 2>&1; then
    echo "⚠️  LLM server not available at $LLM_URL"
    echo "Start with: launchctl load ~/Library/LaunchAgents/com.penfold.mlx-llm-server.plist"
fi
```

### Step 2: Set Test Environment

```bash
export PENFOLD_DB_NAME=penfold_test_e2e
export LLM_URL="${LLM_URL:-http://localhost:8080}"
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
- **Skipped**: Action="skip" (missing DB or LLM)

Note: E2E tests may have longer durations due to LLM inference.

### Step 5: Present Structured Summary

```
## E2E Test Results

| Status | Count |
|--------|-------|
| ✅ Passed | X |
| ❌ Failed | Y |
| ⏭️ Skipped | Z |

**Database**: penfold_test_e2e @ dev02.brown.chat
**LLM**: Qwen2.5-32B @ localhost:8080
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

### Step 9: LLM Diagnostics (if failures)

```
### 🔧 LLM Diagnostics

Check LLM server:
  curl -s http://localhost:8080/v1/models | jq .

Test LLM inference:
  curl -s http://localhost:8080/v1/chat/completions \
    -H "Content-Type: application/json" \
    -d '{"model":"qwen2.5-32b-instruct","messages":[{"role":"user","content":"Say hello"}]}'

Start LLM server:
  launchctl load ~/Library/LaunchAgents/com.penfold.mlx-llm-server.plist

Check LLM logs:
  tail -f /tmp/mlx-llm-server.log
```

### Step 10: Recommendations

- If LLM tests slow: "Consider running on dev01 for better LLM performance"
- If all pass: "E2E tests passing. Full system validation complete."
- If DB tests pass but LLM fail: "Database layer working. Check LLM server status."

## Performance Target

E2E tests should complete in <5 minutes total.
Individual LLM tests may take 10-30s due to inference time.
