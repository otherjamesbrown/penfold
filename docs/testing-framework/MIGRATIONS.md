# Migration Workflow

Penfold uses a shared live database (`penfold` on dev02) for integration tests. This means migrations are **not** applied automatically during test runs — they must be applied deliberately before tests run.

## Why not auto-apply?

With a shared DB, automatically applying migrations during `go test` would:

- **Break in-flight PRs**: PR A's test run applies migration 155, then PR B's tests (which expect schema at 154) break in unexpected ways.
- **Mask schema drift**: If the test setup auto-migrates, tests never catch the case where the real DB is behind.

Instead, tests assert the DB is fully up to date and fail fast with an actionable error if not.

## Applying migrations

```bash
# Apply all pending migrations
penf migrate up

# Check current migration status
penf migrate status

# Apply up to a specific migration
penf migrate up --to 123_my_migration
```

## Adding a new migration

1. Create `migrations/NNN_description.sql` (next sequential number)
2. Write the SQL (UP section only; no goose DOWN section required)
3. Apply locally: `penf migrate up`
4. Run integration tests to confirm: `cd tests && go test -tags=integration ./integration/...`

## Pre-test sanity check

`TestMigrations_AllApplied` in `tests/integration/migrations_test.go` runs as part of the integration test suite. It calls `db.GetPendingMigrations` and fails immediately if any migrations are pending, printing the list of missing migrations and the fix command.

`SetupTestDB` in `tests/integration/helpers.go` also performs this check on every test that calls it, so individual tests will fail fast rather than producing confusing schema-related errors.

## CI behaviour

The CI `integration-tests` job runs against dev02 with the live `penfold` DB. Migrations are **not** run as part of CI. Before merging a PR that adds migrations, apply them to dev02 manually:

```bash
penf migrate up
```

This should be done as part of the deploy step, after the PR is merged and deployed.

## Naming conventions

Migration files follow `NNN_description.sql`:

- `NNN` — three-digit zero-padded sequential number
- `description` — snake_case description of what the migration does

Duplicate version numbers are allowed when multiple PRs add migrations at the same position, but must be resolved before merging (renumber so each version is unique).
