# Run Unit Tests

Execute unit tests with structured output showing pass/fail/skip status.

## Arguments: $ARGUMENTS

Optional: Package pattern to test (e.g., `./pkg/mentions/...`, `./pkg/glossary/...`)
If not provided, runs all unit tests in `./pkg/...`

## Instructions

### Step 0: Run Health Check

Before running tests, verify infrastructure is healthy:

**Run `/penf.health` first.** This ensures the development environment is ready.

### Step 1: Run Tests with JSON Output

```bash
# Default: all packages, or specific pattern if provided
PATTERN="${ARGUMENTS:-./pkg/...}"
go test -short -json $PATTERN 2>&1 | tee /tmp/test-output.json
```

### Step 2: Parse and Categorize Results

Parse the JSON output to extract test results. Each line is a JSON object with fields:
- `Action`: "pass", "fail", "skip", "output", "run", "pause", "cont"
- `Package`: package path
- `Test`: test name (empty for package-level)
- `Output`: test output text
- `Elapsed`: duration in seconds

Categorize tests:
- **Passed**: Action="pass" with Test name
- **Failed**: Action="fail" with Test name
- **Skipped**: Action="skip" with Test name
- **Packages**: Action="pass"/"fail" without Test name

### Step 3: Present Structured Summary

Output a summary table:

```
## Unit Test Results

| Status | Count |
|--------|-------|
| ✅ Passed | X |
| ❌ Failed | Y |
| ⏭️ Skipped | Z |
| 📦 Packages | N |

**Duration**: X.XXs
```

### Step 4: Show Failures First (if any)

If there are failures, show them prominently:

```
### ❌ Failed Tests

| Package | Test | Output |
|---------|------|--------|
| pkg/config | TestDSNFormat | Expected X, got Y |
```

### Step 5: Show Skipped Tests

```
### ⏭️ Skipped Tests

| Package | Test | Reason |
|---------|------|--------|
| pkg/foo | TestBar | Short mode |
```

### Step 6: Show Passed Tests (collapsed if many)

If more than 10 passed tests, just show count. Otherwise list them:

```
### ✅ Passed Tests

- pkg/mentions: TestResolveMention_ExactMatch
- pkg/glossary: TestExpandAcronym
```

### Step 7: Recommendations

Based on results, provide actionable next steps:

- If failures: "Run `go test -v ./pkg/config/...` to see detailed failure output"
- If all pass: "All unit tests passing. Consider running `/test.integration` next."
- If many skipped: "X tests skipped in short mode. Run without `-short` for full coverage."

## Performance Target

Unit tests should complete in <10s total.
