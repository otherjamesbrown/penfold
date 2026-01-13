# Database Developer Agent Context

> **Inherits**: CLAUDE.md → context/agents.md → this file
> **Domain**: Storage, Migrations, Performance, Vector Operations

---

## Domain Ownership

**You own:**
- Database schema design and evolution
- SQLAlchemy models and relationships
- Alembic migrations and rollbacks
- Database performance optimization
- Vector storage and indexing (pgvector)
- Multi-tenant architecture (RLS policies)
- Storage-related CLI commands

**You do NOT own:**
- AI model integration (→ ai-dev)
- External system connectors (→ integration-dev)
- Search interface logic (→ search-dev)
- Event processing workflows (→ ai-dev)
- Application business logic

---

## Database-Specific Rules

**NEVER:**
- Create migrations without down() rollback method
- Modify production schema without migration
- Add indexes without analyzing query patterns
- Change tenant isolation without security review
- Remove foreign key constraints without impact analysis
- Skip performance testing on migration changes

**ALWAYS:**
- Test migrations on realistic data volumes
- Maintain tenant data isolation (RLS policies)
- Follow SQLAlchemy async patterns
- Write performance tests for database changes
- Update schema documentation when models change
- Verify backup/restore compatibility

---

## Core Patterns (Distilled from specs/001) ✅ **IMPLEMENTATION VALIDATED**

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

**Implementation Status**: Multi-tenant models implemented with 5 core entity types:
- ✅ Source, Assertion, Person, Project, Team (user stories 1-2)
- ✅ Tenant, TenantSession, CrossTenantPersonLink (user story 0)
- ✅ Embedding, ProcessingEvent, ProcessingJob, ProcessingResult, Subscription (user stories 3-4)

### Vector Storage Pattern ✅ **IMPLEMENTED WITH PERFORMANCE VALIDATION**
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

**Implementation Status**:
- ✅ VectorOperations class with similarity search (<500ms target)
- ✅ EmbeddingRepository with batch processing and tenant isolation
- ✅ Vector storage models with HNSW optimization for 768-dimensional nomic-embed-text vectors
- ✅ Comprehensive test suite (38 tests, 100% pass rate)

**Lessons Learned from Implementation**:
- HNSW parameters M=16, ef_construction=200 provide optimal performance/memory balance
- L2 distance metric validated for semantic search accuracy
- Batch processing essential for efficient vector operations
- Proper tenant isolation required for all vector operations

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

## Implementation Learnings (From specs/001-database-schema Implementation)

### Event-Driven Processing Patterns
```python
# Event processing entities with job state management
class ProcessingEvent(BaseModel):
    __tablename__ = 'processing_events'

    event_type: Mapped[str] = mapped_column(String, nullable=False)
    event_data: Mapped[dict] = mapped_column(JSON)
    source_id: Mapped[str] = mapped_column(String, ForeignKey('sources.id'))

    # Event filtering with JSONB for flexible subscription patterns
    filters: Mapped[dict] = mapped_column(JSON, default=dict)

class ProcessingJob(BaseModel):
    __tablename__ = 'processing_jobs'

    event_id: Mapped[str] = mapped_column(String, ForeignKey('processing_events.id'))
    processor_name: Mapped[str] = mapped_column(String, nullable=False)
    status: Mapped[str] = mapped_column(String, default='queued')  # [queued, in_progress, completed, failed]

    # Job execution tracking
    started_at: Mapped[Optional[datetime]]
    completed_at: Mapped[Optional[datetime]]
    retry_count: Mapped[int] = mapped_column(Integer, default=0)
```

### Multi-Tenant Cross-Entity Patterns
```python
# Shared entity resolution while maintaining isolation
class CrossTenantPersonLink(DeclarativeBase):
    __tablename__ = 'cross_tenant_person_links'

    # No tenant_id - this is cross-tenant by design
    person_id_1: Mapped[str] = mapped_column(String, ForeignKey('people.id'), primary_key=True)
    person_id_2: Mapped[str] = mapped_column(String, ForeignKey('people.id'), primary_key=True)
    confidence_score: Mapped[float] = mapped_column(Float)
    validation_status: Mapped[str] = mapped_column(String, default='pending')
```

### Async Database Patterns
```python
# Validated patterns for async operations
class AsyncRepository:
    async def create_with_tenant_context(self, session: AsyncSession, data: dict, tenant_id: str):
        """Pattern for all tenant-aware creation operations."""
        entity = EntityModel(**data, tenant_id=tenant_id)
        session.add(entity)
        await session.commit()
        await session.refresh(entity)
        return entity

    async def find_by_tenant(self, session: AsyncSession, tenant_id: str, **filters):
        """Pattern for all tenant-isolated queries."""
        query = select(EntityModel).where(EntityModel.tenant_id == tenant_id)
        for field, value in filters.items():
            query = query.where(getattr(EntityModel, field) == value)
        result = await session.execute(query)
        return result.scalars().all()
```

### Critical Implementation Decisions Made
1. **JSONB over JSON**: Used JSONB for metadata fields requiring query support
2. **String IDs over UUIDs**: Better CLI usability and debugging
3. **Soft Delete Pattern**: archive tables for audit without performance impact
4. **Async Session Management**: Connection pooling with proper cleanup
5. **Tenant Context Storage**: PostgreSQL session variables for RLS policy enforcement

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