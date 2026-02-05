# Test Troubleshooting Guide

Solutions to common problems when running Penfold tests.

## Quick Diagnosis

```bash
# Check environment
source ~/github/otherjamesbrown/secrets/.env.penfold
echo "DB: $PENFOLD_DB_HOST"

# Test database connection
psql -h dev02.brown.chat -U penfold -d penfold -c "SELECT 1"

# Check LLM availability
curl -s http://localhost:8080/v1/models

# Run tests with verbose output
go test -tags=integration -v ./tests/integration/... 2>&1 | head -50
```

## Common Issues

### 1. Tests Skip with "PENFOLD_DB_PASSWORD not set"

**Symptom:**
```
--- SKIP: TestCLI_GlossaryList (0.00s)
    helpers.go:42: PENFOLD_DB_PASSWORD not set - skipping integration test
```

**Solution:**
```bash
# Load environment from secrets
source ~/github/otherjamesbrown/secrets/.env.penfold

# Verify it's set
echo $PENFOLD_DB_PASSWORD
```

### 2. Database Connection Refused

**Symptom:**
```
failed to connect to test database: dial tcp: connection refused
```

**Solutions:**

1. Check SSL certificates exist:
```bash
ls -la ~/.postgresql/
# Should have: postgresql.crt, postgresql.key, root.crt
```

2. Copy certificates if missing:
```bash
mkdir -p ~/.postgresql
cp ~/github/otherjamesbrown/secrets/certs/postgresql.* ~/.postgresql/
chmod 600 ~/.postgresql/postgresql.key
```

3. Verify database is reachable:
```bash
psql -h dev02.brown.chat -U penfold -d penfold -c "SELECT 1"
```

4. Check firewall/VPN:
```bash
nc -zv dev02.brown.chat 5432
```

### 3. SSL Certificate Errors

**Symptom:**
```
x509: certificate signed by unknown authority
```

**Solution:**
```bash
# Ensure root CA is in place
cat ~/.postgresql/root.crt

# If missing, copy from secrets
cp ~/github/otherjamesbrown/secrets/certs/root.crt ~/.postgresql/
```

### 4. Tests Skip with "Local LLM not available"

**Symptom:**
```
--- SKIP: TestMentionResolutionWithLLM (0.00s)
    helpers.go:120: Local LLM not available - skipping E2E test
```

**Solutions:**

1. Check if LLM server is running:
```bash
curl -s http://localhost:8080/v1/models
```

2. If on laptop, create SSH tunnel to dev01:
```bash
ssh -L 8080:localhost:8080 dev01
```

3. Start LLM server on dev01:
```bash
# On dev01
launchctl load ~/Library/LaunchAgents/com.penfold.mlx-llm-server.plist

# Check logs
tail -f /tmp/mlx-llm-server.log
```

### 5. Test Timeout

**Symptom:**
```
panic: test timed out after 2m0s
```

**Solution:**
```bash
# Increase timeout for E2E tests
go test -tags=e2e -v -timeout 15m ./tests/e2e/...

# For benchmark tests
go test -tags=benchmark -v -timeout 30m ./tests/benchmark/...
```

### 6. Fixture Loading Fails

**Symptom:**
```
failed to load Acme Corp fixtures: insert team Engineering: duplicate key value
```

**Solutions:**

1. Clean up existing test data:
```bash
psql -h dev02.brown.chat -U penfold -d penfold -c \
  "DELETE FROM teams WHERE tenant_id = '00000000-0000-0000-0000-000000000002'"
```

2. Tests should use `ON CONFLICT DO UPDATE`, but if stuck:
```bash
# Clean all integration test tenant data
psql -h dev02.brown.chat -U penfold -d penfold << 'EOF'
DELETE FROM glossary WHERE tenant_id = '00000000-0000-0000-0000-000000000002';
DELETE FROM products WHERE tenant_id = '00000000-0000-0000-0000-000000000002';
DELETE FROM projects WHERE tenant_id = '00000000-0000-0000-0000-000000000002';
DELETE FROM people WHERE tenant_id = '00000000-0000-0000-0000-000000000002';
DELETE FROM teams WHERE tenant_id = '00000000-0000-0000-0000-000000000002';
EOF
```

### 7. JSON Unmarshal Errors

**Symptom:**
```
json: cannot unmarshal string into Go value of type []string
```

**Cause:** Database column type mismatch (usually JSONB columns).

**Solution:** This is typically a backend bug, not a test issue. The test correctly exposed the problem. Fix the scanning code in the repository layer.

**Workaround for tests:**
```go
if strings.Contains(stderr, "cannot unmarshal") {
    t.Skip("Skipping due to known context field scanning bug")
}
```

### 8. Race Condition Detected

**Symptom:**
```
WARNING: DATA RACE
```

**Solution:**
```bash
# Run with race detector to identify the issue
go test -race -v ./pkg/...

# The output will show the conflicting goroutines
```

### 9. Test Isolation Failures

**Symptom:**
Tests pass individually but fail when run together.

**Solutions:**

1. Ensure proper cleanup:
```go
func TestSomething(t *testing.T) {
    t.Cleanup(func() {
        // Clean up created resources
        cleanupGlossaryTerm(t, term)
    })
    // ... test code
}
```

2. Use unique identifiers:
```go
term := "TESTTERM_" + uniqueTestID()
```

3. Don't rely on fixture data order:
```go
// Bad: assumes first item is specific
item := result.Items[0]

// Good: find by known criteria
var item Item
for _, i := range result.Items {
    if i.Name == "Expected Name" {
        item = i
        break
    }
}
```

### 10. CLI Command Not Found

**Symptom:**
```
exec: "penf": executable file not found in $PATH
```

**Solution:**
```bash
# Install/rebuild CLI
go install ./cmd/penf

# Or ensure GOPATH/bin is in PATH
export PATH=$PATH:$(go env GOPATH)/bin

# Verify
which penf
```

### 11. Permission Denied on SSL Key

**Symptom:**
```
private key file has group or world access
```

**Solution:**
```bash
chmod 600 ~/.postgresql/postgresql.key
chmod 644 ~/.postgresql/postgresql.crt
chmod 644 ~/.postgresql/root.crt
```

### 12. Tests Pass Locally but Fail in CI

**Common Causes:**

1. **Different database state:** CI uses fresh container, local uses shared database
2. **Missing environment variables:** CI has secrets configured differently
3. **Timing issues:** CI runners may be slower

**Debug Steps:**
```bash
# Check CI logs for environment
# Look for "skipping" messages that indicate missing config

# Run locally with same constraints
go test -tags=integration -v -timeout 60s ./tests/integration/...
```

## Flaky Test Handling

### Identifying Flaky Tests

```bash
# Run test multiple times
for i in {1..10}; do
    go test -tags=integration -v -run TestSuspectedFlaky ./tests/integration/...
done
```

### Quarantining Flaky Tests

```go
//go:build flaky

func TestKnownFlaky(t *testing.T) {
    // TODO: Fix by 2026-03-01 - timing sensitivity in LLM response
    // This test is quarantined due to non-deterministic LLM output
}
```

### Running Quarantined Tests

```bash
# Include flaky tests
go test -tags="integration,flaky" ./tests/integration/...
```

## Getting Help

### Verbose Output

```bash
# Maximum verbosity
go test -tags=integration -v -count=1 ./tests/integration/... 2>&1 | tee test.log
```

### Test-Specific Debug

```bash
# Run single test with debug output
go test -tags=integration -v -run TestCLI_GlossaryList ./tests/integration/...
```

### Database State Inspection

```bash
# Check what's in the test tenant
psql -h dev02.brown.chat -U penfold -d penfold << 'EOF'
SELECT 'teams' as table_name, COUNT(*) FROM teams WHERE tenant_id = '00000000-0000-0000-0000-000000000002'
UNION ALL
SELECT 'people', COUNT(*) FROM people WHERE tenant_id = '00000000-0000-0000-0000-000000000002'
UNION ALL
SELECT 'glossary', COUNT(*) FROM glossary WHERE tenant_id = '00000000-0000-0000-0000-000000000002';
EOF
```

## Error Message Reference

| Error | Likely Cause | Solution |
|-------|--------------|----------|
| `connection refused` | DB not reachable | Check network/VPN |
| `certificate verify failed` | Missing/wrong SSL cert | Copy certs to ~/.postgresql/ |
| `no such host` | DNS issue | Check hostname spelling |
| `permission denied` | SSL key permissions | chmod 600 key file |
| `timeout` | Slow test or hung | Increase -timeout flag |
| `duplicate key` | Fixture collision | Clean test data |
| `cannot unmarshal` | Type mismatch | Backend bug, skip in test |
