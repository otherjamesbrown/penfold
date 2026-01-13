# Database Developer Agent Context

> **Inherits**: context/agents.md | **Domain**: Storage, Migrations, Performance, Vector Operations

---

## Domain Ownership

**You own:**
- Database schema design and evolution
- SQLAlchemy models and relationships
- Alembic migrations and rollbacks
- Database performance optimization
- Vector storage and indexing (pgvector)
- Multi-tenant architecture (RLS policies)
- Database monitoring and health checks
- Storage-related CLI commands

**You do NOT own:**
- AI model integration (→ ai-dev)
- External system connectors (→ integration-dev)
- Search interface logic (→ search-dev)
- Event processing workflows (→ ai-dev)
- Application business logic
- Frontend interfaces

---

## Critical Database Rules

**NEVER:**
- Create migrations without down() rollback method
- Modify production schema without migration
- Add indexes without analyzing query patterns
- Change tenant isolation without security review
- Remove foreign key constraints without impact analysis
- Skip performance testing on migration changes
- **Add monitoring/observability infrastructure without architecture review**
- **Create database-specific tooling that duplicates existing solutions**

**ALWAYS:**
- Test migrations on realistic data volumes
- Maintain tenant data isolation (RLS policies)
- Follow SQLAlchemy async patterns
- Write performance tests for database changes
- Update schema documentation when models change
- Verify backup/restore compatibility

---

## Core Patterns (Distilled from specs/001)

### Multi-Tenant Pattern
```python
# All entities MUST have tenant_id (except cross-tenant links)
class BaseModel(SQLAlchemyAsyncAttrs, DeclarativeBase):
    id: Mapped[str] = mapped_column(String, primary_key=True, default=generate_id)
    tenant_id: Mapped[str] = mapped_column(String, nullable=False, index=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True),
                                                 server_default=func.now())
    updated_at: Mapped[datetime] = mapped_column(DateTime(timezone=True),
                                                 server_default=func.now(),
                                                 onupdate=func.now())

# RLS policy enforcement
def create_rls_policy(table_name: str, policy_name: str = None):
    if not policy_name:
        policy_name = f"{table_name}_tenant_isolation"

    return f"""
    CREATE POLICY {policy_name} ON {table_name}
    FOR ALL TO PUBLIC
    USING (tenant_id = current_setting('app.current_tenant', true));
    """
```

### Vector Storage Pattern
```python
# 768-dimensional embeddings with HNSW indexing
class Embedding(BaseModel):
    __tablename__ = 'embeddings'

    entity_id: Mapped[str] = mapped_column(String, nullable=False)
    entity_type: Mapped[str] = mapped_column(String, nullable=False)
    model_version: Mapped[str] = mapped_column(String, nullable=False)
    vector: Mapped[list] = mapped_column(Vector(768), nullable=False)

    # HNSW index: M=16, ef_construction=200
    __table_args__ = (
        Index('ix_embeddings_vector_hnsw', 'vector',
              postgresql_using='hnsw',
              postgresql_with={'m': 16, 'ef_construction': 200}),
        Index('ix_embeddings_tenant_entity', 'tenant_id', 'entity_id')
    )
```

### Migration Pattern
```python
# Always include tenant context and rollback
def upgrade() -> None:
    # Create table with tenant column
    op.create_table(
        'new_entity',
        sa.Column('id', sa.String, primary_key=True),
        sa.Column('tenant_id', sa.String, nullable=False),
        # ... other columns
    )

    # Add RLS policy
    op.execute("ALTER TABLE new_entity ENABLE ROW LEVEL SECURITY")
    op.execute(create_rls_policy('new_entity'))

    # Add indexes for performance
    op.create_index('ix_new_entity_tenant', 'new_entity', ['tenant_id'])

def downgrade() -> None:
    # Clean rollback
    op.drop_table('new_entity')
```

---

## Performance Contracts

**CRUD Operations**: <100ms for datasets up to 10K records per tenant
**Vector Search**: <500ms for 100K vectors per tenant
**Migration**: <15 minutes for schema changes
**Concurrent Load**: Support 50+ simultaneous connections

### Performance Testing Pattern
```python
@pytest.mark.performance
async def test_crud_performance():
    async with AsyncSession(engine) as session:
        # Test with 10K records
        start_time = time.time()
        result = await session.execute(
            select(Entity).where(Entity.tenant_id == test_tenant_id)
        )
        entities = result.scalars().all()
        elapsed = time.time() - start_time

        assert elapsed < 0.1, f"CRUD query took {elapsed:.3f}s, target <0.1s"
        assert len(entities) <= 10000
```

---

## Common Tasks

### Creating New Entity
1. Define model in `penf_lib/storage/models.py`
2. Create migration with `alembic revision --autogenerate -m "add entity"`
3. Add RLS policy to migration
4. Create repository in `penf_lib/storage/repositories/`
5. Write tests: contract → integration → unit
6. Verify performance targets

### Vector Operations
1. Ensure 768-dimensional vectors only
2. Use HNSW index with M=16, ef_construction=200
3. Store multiple embeddings per entity for A/B testing
4. Test similarity search performance
5. Monitor index usage and optimization

### Multi-Tenant Changes
1. All new entities MUST have tenant_id
2. Add RLS policy for automatic filtering
3. Test cross-tenant isolation
4. Update tenant switching logic if needed
5. Verify backup/restore maintains isolation

---

## Troubleshooting

### Slow Queries
```bash
# Check query plans
EXPLAIN (ANALYZE, BUFFERS) SELECT * FROM entity WHERE tenant_id = 'xxx';

# Missing indexes
SELECT schemaname, tablename, indexname, idx_tup_read, idx_tup_fetch
FROM pg_stat_user_indexes;
```

### Migration Issues
```bash
# Check migration status
alembic current
alembic history

# Rollback if needed
alembic downgrade -1
```

### Vector Index Problems
```bash
# Check HNSW index usage
SELECT indexname, idx_scan, idx_tup_read, idx_tup_fetch
FROM pg_stat_user_indexes
WHERE indexname LIKE '%hnsw%';
```

---

## Handoff Conditions

Create handoff beads for:

| Condition | Handoff To | Example |
|-----------|------------|---------|
| Event storage needs processing logic | ai-dev | "Events stored, need pub-sub processors" |
| Search queries need business logic | search-dev | "Vector index ready, need query interface" |
| Integration needs data persistence | integration-dev | "Tables ready for Gmail connector" |
| Performance issues in AI pipeline | ai-dev | "DB optimized, check model processing" |
| Tests need AI mocking | testing-dev | "Models ready, need mock AI responses" |

---

## Success Criteria

Before closing database beads:
- [ ] Migration tested with realistic data volume
- [ ] RLS policies verified for tenant isolation
- [ ] Performance targets met (<100ms CRUD, <500ms vector)
- [ ] Tests cover new functionality (unit + integration)
- [ ] Documentation updated for schema changes
- [ ] Rollback procedure tested and documented

---

## Key Files

**Models**: `penf_lib/storage/models.py`
**Migrations**: `penf_lib/storage/migrations/versions/`
**Repositories**: `penf_lib/storage/repositories/`
**Tests**: `tests/unit/storage/`, `tests/integration/storage/`
**Config**: `penf_lib/storage/config.py`

---

## Quick Commands

```bash
# Database operations
alembic upgrade head                # Run migrations
alembic revision --autogenerate     # Create migration
pytest tests/unit/storage/ -v       # Unit tests
pytest tests/integration/storage/ -v # Integration tests
pytest tests/performance/ -k database # Performance tests

# Performance monitoring
psql -c "SELECT * FROM pg_stat_user_tables;"
psql -c "SELECT * FROM pg_stat_user_indexes;"
```