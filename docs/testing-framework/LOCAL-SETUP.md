# Local Test Setup

Quick reference for running tests locally.

## Commands by Test Type

| Type | Command | Prerequisites |
|------|---------|---------------|
| Unit | `go test ./pkg/...` | None |
| Integration | `go test -tags=integration ./tests/integration/...` | DB certs |
| E2E | `go test -tags=e2e -timeout 15m ./tests/e2e/...` | DB + LLM |
| Live | `go test -tags=live ./tests/live/...` | API keys |
| Benchmark | `go test -tags=benchmark ./tests/benchmark/...` | LLM |

## Setup Checklist

```bash
# 1. Load secrets
source ~/github/otherjamesbrown/secrets/.env.penfold

# 2. Apply pending migrations (integration/E2E)
penf migrate up

# 3. Verify DB (integration/E2E)
psql -h dev02.brown.chat -U penfold -d penfold -c "SELECT 1"

# 4. Verify LLM (E2E only)
curl -s http://localhost:8080/v1/models
```

> **Note:** Migrations are NOT applied automatically during test runs. If the DB is behind,
> `SetupTestDB` will fail immediately with the list of pending migrations and the fix command.
> See [MIGRATIONS.md](./MIGRATIONS.md) for the full workflow.

## Environment Variables

| Variable | Used By | Default |
|----------|---------|---------|
| `PENFOLD_DB_HOST` | integration, e2e | `dev02.brown.chat` |
| `PENFOLD_DB_USER` | integration, e2e | `penfold` |
| `LLM_URL` | e2e | `http://localhost:8080` |
| `GEMINI_API_KEY` | live | - |

## Test Tenant IDs

| Test Type | Tenant ID |
|-----------|-----------|
| Integration | `00000000-0000-0000-0000-000000000002` |
| E2E | `00000000-0000-0000-0000-000000000001` |

## SSL Certificates

Required in `~/.postgresql/`:
- `postgresql.crt`
- `postgresql.key` (chmod 600)
- `root.crt`

Copy from: `~/github/otherjamesbrown/secrets/certs/`
