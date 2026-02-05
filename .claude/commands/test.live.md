# Run Live Tests

Execute live tests (cloud API tests) with structured output.

## Arguments: $ARGUMENTS

Optional: Specific test pattern (e.g., `TestGemini.*`, `TestGmail.*`)
If not provided, runs all live tests.

## Prerequisites

Live tests require cloud API credentials:
- `GEMINI_API_KEY` for Gemini API tests
- Gmail OAuth tokens for Gmail API tests

**Warning**: Live tests incur API costs and make real network calls.

## Instructions

### Step 0: Run Health Check

Before running tests, verify infrastructure is healthy:

**Run `/penf.health` first.** If any critical services are down (Gateway, AI Coordinator), fix those before proceeding.

### Step 1: Check Prerequisites

```bash
# Check for API keys
MISSING=""
if [ -z "$GEMINI_API_KEY" ]; then
    MISSING="$MISSING GEMINI_API_KEY"
fi

if [ -n "$MISSING" ]; then
    echo "⚠️  Missing API keys:$MISSING"
    echo "Tests requiring these keys will be skipped."
fi
```

### Step 2: Run Tests with JSON Output

```bash
# Build test pattern
if [ -n "$ARGUMENTS" ]; then
    RUN_FLAG="-run $ARGUMENTS"
else
    RUN_FLAG=""
fi

go test -tags=live -json -timeout 2m $RUN_FLAG ./tests/live/... 2>&1 | tee /tmp/test-live.json
```

### Step 3: Parse and Categorize Results

Parse JSON output:
- **Passed**: Action="pass" with Test name
- **Failed**: Action="fail" with Test name
- **Skipped**: Action="skip" (missing API keys)

### Step 4: Present Structured Summary

```
## Live Test Results

| Status | Count |
|--------|-------|
| ✅ Passed | X |
| ❌ Failed | Y |
| ⏭️ Skipped | Z |

**APIs Tested**: Gemini, Gmail
**Duration**: X.XXs
```

### Step 5: Show Failures First

```
### ❌ Failed Tests

| Test | Error |
|------|-------|
| TestGeminiAPIConnection | 401 Unauthorized |
```

For API failures, show:
- HTTP status code
- Error message from API
- Request details (without exposing keys)

### Step 6: Show Skipped Tests

```
### ⏭️ Skipped Tests

| Test | Reason |
|------|--------|
| TestGeminiBasicCompletion | GEMINI_API_KEY not set |
| TestGmailAPIConnection | Gmail OAuth not configured |
```

### Step 7: Show Passed Tests

```
### ✅ Passed Tests

| Test | API | Duration |
|------|-----|----------|
| TestGeminiAPIConnection | Gemini | 0.8s |
| TestGeminiBasicCompletion | Gemini | 1.2s |
| TestGeminiEmbeddings | Gemini | 0.9s |
```

### Step 8: API Diagnostics (if failures)

```
### 🔧 API Diagnostics

Check Gemini API key:
  curl -s "https://generativelanguage.googleapis.com/v1beta/models?key=$GEMINI_API_KEY" | head -20

Test Gemini connectivity:
  curl -s "https://generativelanguage.googleapis.com/v1beta/models/gemini-pro:generateContent?key=$GEMINI_API_KEY" \
    -H "Content-Type: application/json" \
    -d '{"contents":[{"parts":[{"text":"Say hello"}]}]}'
```

### Step 9: Cost Warning

```
### 💰 API Usage Note

Live tests make real API calls that may incur costs:
- Gemini API: ~$0.001 per test
- Gmail API: Free tier usually sufficient

Run sparingly and only when validating API connectivity.
```

## Performance Target

Live tests vary based on network latency.
Typical: 5-30s total for all live tests.
