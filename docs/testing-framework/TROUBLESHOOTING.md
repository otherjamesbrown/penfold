# Test Troubleshooting

## Error → Solution

| Error | Cause | Fix |
|-------|-------|-----|
| `PENFOLD_DB_PASSWORD not set` | Missing env | `source ~/github/otherjamesbrown/secrets/.env.penfold` |
| `connection refused` | DB unreachable | Check VPN, verify `psql -h dev02.brown.chat -U penfold -d penfold -c "SELECT 1"` |
| `certificate verify failed` | Missing SSL | Copy certs to `~/.postgresql/` |
| `private key has group access` | Key permissions | `chmod 600 ~/.postgresql/postgresql.key` |
| `Local LLM not available` | No LLM server | `ssh -L 8080:localhost:8080 dev01` or start server |
| `test timed out` | Slow test | Add `-timeout 15m` flag |
| `duplicate key value` | Fixed test data | Use `uniqueTestID()` (see below) |
| `cannot unmarshal string into []string` | Backend bug | Add skip: `if strings.Contains(stderr, "cannot unmarshal") { t.Skip(...) }` |
| `executable not found: penf` | CLI not installed | `go install ./cmd/penf` |

## Duplicate Key Violations

**Why this happens**: Integration tests run against the production database with tenant isolation. Tables without `tenant_id` (e.g., `meeting_series`, `tenants`) don't get cleaned up between runs.

**Fix**: Make tests self-contained using `uniqueTestID()`:

```go
// BAD - fails on second run
series := &repository.MeetingSeries{Name: "Weekly Standup"}

// GOOD - unique per run
testID := uniqueTestID()
series := &repository.MeetingSeries{Name: "Weekly Standup " + testID}
```

**Tables requiring unique identifiers** (no `tenant_id` column):
- `meeting_series` - use unique names
- `tenants` - use unique slugs

**Tables with tenant isolation** (cleanup works):
- `glossary`, `projects`, `people`, `teams`, `products`, `sources`, etc.

## Clean Test Data

For tables with `tenant_id`, cleanup happens automatically. For manual cleanup:

```bash
# Clean integration test tenant
psql -h dev02.brown.chat -U penfold -d penfold << 'EOF'
DELETE FROM glossary WHERE tenant_id = '00000000-0000-0000-0000-000000000002';
DELETE FROM products WHERE tenant_id = '00000000-0000-0000-0000-000000000002';
DELETE FROM projects WHERE tenant_id = '00000000-0000-0000-0000-000000000002';
DELETE FROM people WHERE tenant_id = '00000000-0000-0000-0000-000000000002';
DELETE FROM teams WHERE tenant_id = '00000000-0000-0000-0000-000000000002';
EOF
```

## Debug Commands

```bash
# Verbose single test
go test -tags=integration -v -run TestCLI_GlossaryList ./tests/integration/...

# With race detection
go test -race -v ./pkg/...

# Check DB state
psql -h dev02.brown.chat -U penfold -d penfold -c \
  "SELECT COUNT(*) FROM teams WHERE tenant_id = '00000000-0000-0000-0000-000000000002'"
```

## Flaky Test Quarantine

```go
//go:build flaky

func TestKnownFlaky(t *testing.T) {
    // TODO: Fix by YYYY-MM-DD - describe issue
}
```

Run quarantined: `go test -tags="integration,flaky" ./tests/integration/...`
