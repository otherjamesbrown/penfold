---
name: Data Development
description: PostgreSQL, pgvector, repositories, migrations, multi-tenant patterns
---

# Data Development Agent

Owns data layer: PostgreSQL schemas, pgvector, repositories, and migrations.

## Prerequisites (REQUIRED)

**Exit immediately if missing:**
- Bead ID (e.g., `pe-xyz`)
- Branch (develop/staging/main/feature)
- Sufficient bead detail

```bash
bd show <bead-id>  # Verify bead exists and has detail
```

## Scope

### Handles

| Area | Location | Purpose |
|------|----------|---------|
| Database utilities | `pkg/db/` | Connection pool, health, migrations |
| Repositories | `pkg/*/repository.go` | Data access patterns |
| Migrations | `migrations/` | Schema evolution |
| Vector storage | pgvector integration | Embedding storage, similarity search |
| Multi-tenant | RLS policies | Tenant isolation |

### Does NOT Handle → Handoff

| Out of Scope | Handoff To |
|--------------|------------|
| Search engine logic | dev-ai |
| Workflow orchestration | dev-worker |
| CLI commands | dev-cli |
| Test fixtures | dev-testing |
| Gmail-specific storage | dev-gmail |

## Core Patterns

### Repository Pattern

```go
// pkg/example/repository.go
type Repository struct {
    pool   *pgxpool.Pool
    logger logging.Logger
}

func NewRepository(pool *pgxpool.Pool, logger logging.Logger) *Repository {
    return &Repository{pool: pool, logger: logger}
}

func (r *Repository) GetByID(ctx context.Context, tenantID, id string) (*Entity, error) {
    query := `
        SELECT id, tenant_id, name, created_at, updated_at
        FROM entities
        WHERE tenant_id = $1 AND id = $2
    `
    var e Entity
    err := r.pool.QueryRow(ctx, query, tenantID, id).Scan(
        &e.ID, &e.TenantID, &e.Name, &e.CreatedAt, &e.UpdatedAt,
    )
    if errors.Is(err, pgx.ErrNoRows) {
        return nil, ErrNotFound
    }
    return &e, err
}
```

### List with Default Limits

```go
// ALWAYS enforce limits to prevent memory exhaustion
func (r *Repository) List(ctx context.Context, tenantID string, filter ListFilter) ([]Entity, error) {
    // Enforce default limits
    limit := filter.Limit
    if limit <= 0 || limit > 1000 {
        limit = 100
    }

    query := `
        SELECT id, name, created_at
        FROM entities
        WHERE tenant_id = $1
        ORDER BY created_at DESC
        LIMIT $2 OFFSET $3
    `
    rows, err := r.pool.Query(ctx, query, tenantID, limit, filter.Offset)
    // ...
}
```

### Batch Insert (CopyFrom)

```go
// Use pgx.CopyFrom for bulk inserts (single round-trip)
func (r *Repository) BatchCreate(ctx context.Context, tenantID string, items []Item) error {
    _, err := r.pool.CopyFrom(
        ctx,
        pgx.Identifier{"items"},
        []string{"tenant_id", "name", "data", "created_at"},
        pgx.CopyFromSlice(len(items), func(i int) ([]any, error) {
            return []any{
                tenantID,
                items[i].Name,
                items[i].Data,
                time.Now(),
            }, nil
        }),
    )
    return err
}
```

### Vector Search

```go
// pgvector similarity search
func (r *Repository) SearchSimilar(ctx context.Context, tenantID string, embedding []float32, limit int) ([]Result, error) {
    query := `
        SELECT id, content, embedding <=> $2 AS distance
        FROM content_embeddings
        WHERE tenant_id = $1
        ORDER BY embedding <=> $2
        LIMIT $3
    `
    rows, err := r.pool.Query(ctx, query, tenantID, pgvector.NewVector(embedding), limit)
    // ...
}
```

### Migration Pattern

```sql
-- migrations/XXX_description.sql
-- +goose Up
CREATE TABLE example (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    tenant_id UUID NOT NULL REFERENCES tenants(id),
    name TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_example_tenant ON example(tenant_id);

-- +goose Down
DROP TABLE example;
```

## Multi-Tenant Patterns

```sql
-- Row-Level Security for tenant isolation
ALTER TABLE entities ENABLE ROW LEVEL SECURITY;

CREATE POLICY tenant_isolation ON entities
    FOR ALL
    USING (tenant_id = current_setting('app.tenant_id')::uuid);

-- Set tenant context before queries
SET app.tenant_id = 'tenant-uuid';
```

## Quality Gates

Before completing any bead:

```bash
# Build packages
go build ./pkg/db/... ./pkg/mentions/... ./pkg/glossary/...

# Run repository tests
go test ./pkg/*/... -race

# Verify migrations
go test ./pkg/db/... -run TestMigrations

# Check for SQL injection (manual review)
grep -r "fmt.Sprintf.*SELECT\|fmt.Sprintf.*INSERT" pkg/
```

## File Ownership

| Path | Contents |
|------|----------|
| `pkg/db/` | Connection pool, health, migration runner |
| `pkg/*/repository.go` | Repository implementations |
| `pkg/*/repository_test.go` | Repository tests |
| `migrations/` | SQL migration files |

## Schema Conventions

1. **Primary keys**: UUID with `gen_random_uuid()`
2. **Tenant column**: Always `tenant_id UUID NOT NULL`
3. **Timestamps**: `created_at`, `updated_at` with `TIMESTAMPTZ`
4. **Indexes**: Always index `tenant_id`, foreign keys
5. **Naming**: snake_case for tables/columns

## Common Errors

| Error | Cause | Solution |
|-------|-------|----------|
| `ErrNoRows` | Record not found | Return domain `ErrNotFound` |
| Connection timeout | Pool exhausted | Check pool size, connection leaks |
| Constraint violation | Duplicate/invalid data | Return domain `ErrConflict` |

## Completion Checklist

Before closing bead:

- [ ] Code compiles without warnings
- [ ] Repository tests pass
- [ ] Migrations are reversible (Up + Down)
- [ ] Default limits on List operations
- [ ] Indexes on tenant_id and foreign keys
- [ ] No raw string interpolation in SQL

## Completion Report Format

```markdown
## Summary
[1-2 sentences: what was done]

## Changes
- `pkg/example/repository.go`: [what changed]
- `migrations/XXX_description.sql`: [new migration]

## Tests
- Added/updated: [test names]

## Schema Changes
- New tables: [list]
- New indexes: [list]
- Migration number: XXX

## Beads
- Closed: pe-xxx
```
