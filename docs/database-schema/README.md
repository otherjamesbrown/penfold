# Penfold Database Schema Documentation

> **Status**: ✅ **Production Ready** - Implementation complete with comprehensive testing
> **Last Updated**: 2026-01-13
> **Version**: 1.0

## Overview

The Penfold database schema provides a multi-tenant storage layer for personal AI-powered information management. Built on PostgreSQL 16+ with pgvector extension, it enables hybrid relational and vector storage with complete tenant isolation.

## Quick Start

### Prerequisites
- PostgreSQL 16+ with pgvector extension
- Redis (for event processing)
- Python 3.12+

### Setup Database
```bash
# Install dependencies
pip install -r requirements.txt

# Run migrations
alembic upgrade head

# Verify setup
pytest tests/unit/storage/ -v
```

### Basic Usage
```python
from penf_lib.storage import create_async_session, TenantRepository, SourceRepository

async def main():
    async with create_async_session() as session:
        # Create tenant
        tenant_repo = TenantRepository()
        tenant = await tenant_repo.create(session, {
            "name": "work",
            "context_type": "work",
            "settings": {"timezone": "UTC"}
        })

        # Store content with tenant isolation
        source_repo = SourceRepository()
        source = await source_repo.create(session, {
            "source_system": "gmail",
            "external_id": "msg_123",
            "raw_content": "Meeting scheduled for tomorrow",
            "content_type": "email"
        }, tenant_id=tenant.id)
```

## Architecture

### Multi-Tenant Isolation
- **Row-Level Security (RLS)**: Automatic tenant filtering via PostgreSQL policies
- **Session Context**: Persistent tenant selection via `current_setting('app.current_tenant')`
- **Shared Entities**: People can be linked across tenants while maintaining data isolation
- **Complete Separation**: All other entities (projects, sources, assertions) are tenant-isolated

### Core Entities

#### Primary Data Entities
- **Source**: Raw content from external systems (email, Slack, documents, meetings)
- **Assertion**: AI-extracted information (decisions, risks, commitments, milestones)
- **Person**: Canonical person records with cross-tenant linking capability
- **Project**: Hierarchical project structure with timeline and artifact tracking
- **Team**: Organizational units with member relationships

#### Processing Framework
- **ProcessingEvent**: Published events for content processing workflows
- **ProcessingJob**: Individual AI processing tasks with state management
- **ProcessingResult**: AI processor outputs with confidence and attribution
- **Subscription**: Configuration for event type handling and filtering

#### Vector Storage
- **Embedding**: 768-dimensional vectors optimized for nomic-embed-text model
- **HNSW Indexing**: High-performance similarity search (M=16, ef_construction=200)

### Performance Characteristics

| Operation | Target | Validated |
|-----------|--------|-----------|
| CRUD Operations | <100ms (10K records/tenant) | ✅ |
| Vector Similarity Search | <500ms (100K vectors/tenant) | ✅ |
| Event Processing | <50ms (pub/sub operations) | ✅ |
| Migration Time | <15 minutes | ✅ |
| Concurrent Connections | 50+ simultaneous | ✅ |

## Multi-Tenant Usage

### Creating and Managing Tenants
```python
# Create tenants
work_tenant = await tenant_repo.create(session, {
    "name": "work",
    "context_type": "work"
})

personal_tenant = await tenant_repo.create(session, {
    "name": "personal",
    "context_type": "personal"
})

# Switch tenant context
await session.execute(text("SET app.current_tenant = :tenant_id"),
                      {"tenant_id": work_tenant.id})

# All subsequent operations are automatically filtered to this tenant
```

### Cross-Tenant Person Linking
```python
# Create person in work context
work_person = await person_repo.create(session, {
    "canonical_name": "John Smith",
    "email_addresses": ["john.smith@company.com"]
}, tenant_id=work_tenant.id)

# Switch to personal context
await session.execute(text("SET app.current_tenant = :tenant_id"),
                      {"tenant_id": personal_tenant.id})

# Create same person in personal context
personal_person = await person_repo.create(session, {
    "canonical_name": "John Smith",
    "email_addresses": ["john.smith@gmail.com"]
}, tenant_id=personal_tenant.id)

# Link across tenants
link_repo = CrossTenantPersonLinkRepository()
await link_repo.create_link(session, work_person.id, personal_person.id,
                           confidence_score=0.95)
```

## Vector Operations

### Storing Embeddings
```python
from penf_lib.storage.repositories import EmbeddingRepository

embedding_repo = EmbeddingRepository()

# Store vector for content
await embedding_repo.create(session, {
    "entity_id": source.id,
    "entity_type": "source",
    "vector": [0.1, 0.2, ...],  # 768-dimensional vector
    "model_version": "nomic-embed-text-1.5"
}, tenant_id=tenant.id)
```

### Similarity Search
```python
from penf_lib.storage.vector import VectorSearchService

search_service = VectorSearchService()

# Find similar content
similar = await search_service.similarity_search(
    session=session,
    query_vector=[0.15, 0.18, ...],
    entity_type="source",
    tenant_id=tenant.id,
    limit=10,
    similarity_threshold=0.8
)

for result in similar:
    print(f"Entity: {result.entity_id}, Similarity: {result.similarity}")
```

## Event-Driven Processing

### Publishing Events
```python
from penf_lib.storage.events import EventPublisher

publisher = EventPublisher()

# Publish content ingestion event
await publisher.publish_event(
    session=session,
    event_type="content.ingested",
    event_data={
        "source_id": source.id,
        "content_type": "email",
        "processing_priority": "high"
    },
    tenant_id=tenant.id
)
```

### Processing Jobs
```python
from penf_lib.storage.jobs import JobManager

job_manager = JobManager()

# Create processing job
job = await job_manager.create_job(
    session=session,
    event_id=event.id,
    processor_name="ai_extraction",
    tenant_id=tenant.id
)

# Update job status
await job_manager.update_status(session, job.id, "in_progress")
await job_manager.update_status(session, job.id, "completed",
                               result_data={"extracted_entities": [...]})
```

## Database Schema

### Entity Relationship Diagram
```
┌─────────────┐    ┌─────────────┐    ┌─────────────┐
│   Tenant    │    │   Source    │    │ Assertion   │
│             │◄──┤             │◄──┤             │
│ • name      │    │ • content   │    │ • content   │
│ • context   │    │ • metadata  │    │ • confidence│
└─────────────┘    └─────────────┘    └─────────────┘
       │                   │                   │
       │           ┌─────────────┐    ┌─────────────┐
       │           │  Embedding  │    │   Person    │
       └──────────►│             │    │             │
                   │ • vector    │    │ • name      │
                   │ • model     │    │ • emails    │
                   └─────────────┘    └─────────────┘
                                             │
                   ┌─────────────────────────┘
                   │
       ┌─────────────────────┐    ┌─────────────┐
       │CrossTenantPersonLink│    │   Project   │
       │                     │    │             │
       │ • person_id_1       │    │ • name      │
       │ • person_id_2       │    │ • timeline  │
       │ • confidence        │    │ • artifacts │
       └─────────────────────┘    └─────────────┘
```

### Key Tables
| Table | Purpose | Tenant Isolated |
|-------|---------|-----------------|
| tenants | Tenant definitions | N/A (system-level) |
| sources | Raw content storage | Yes |
| assertions | Extracted information | Yes |
| people | Person entities | Yes (with cross-tenant linking) |
| projects | Project organization | Yes |
| teams | Team structures | Yes |
| embeddings | Vector storage | Yes |
| processing_events | Event distribution | Yes |
| processing_jobs | Job management | Yes |
| cross_tenant_person_links | Cross-tenant person resolution | No (by design) |

## Migration Management

### Creating Migrations
```bash
# Auto-generate migration
alembic revision --autogenerate -m "add new entity"

# Custom migration
alembic revision -m "custom data migration"
```

### Migration Best Practices
1. **Always include rollback logic** in `downgrade()` function
2. **Add RLS policies** for new tenant-isolated tables
3. **Test migrations** with realistic data volumes
4. **Verify tenant isolation** after schema changes
5. **Update indexes** for performance optimization

### Example Migration
```python
def upgrade() -> None:
    # Create new table
    op.create_table(
        'new_entity',
        sa.Column('id', sa.String, primary_key=True),
        sa.Column('tenant_id', sa.String, nullable=False),
        # ... other columns
    )

    # Enable RLS
    op.execute("ALTER TABLE new_entity ENABLE ROW LEVEL SECURITY")
    op.execute("""
        CREATE POLICY new_entity_tenant_isolation ON new_entity
        FOR ALL TO PUBLIC
        USING (tenant_id = current_setting('app.current_tenant', true))
    """)

    # Add performance indexes
    op.create_index('ix_new_entity_tenant', 'new_entity', ['tenant_id'])

def downgrade() -> None:
    op.drop_table('new_entity')
```

## Testing

### Unit Tests
```bash
pytest tests/unit/storage/ -v
```

### Integration Tests
```bash
pytest tests/integration/storage/ -v
```

### Performance Tests
```bash
pytest tests/performance/ -k database -v
```

## Configuration

### Database Settings
```python
# penf_lib/storage/config.py
DATABASE_URL = "postgresql+asyncpg://user:pass@localhost/penfold"
DATABASE_POOL_SIZE = 20
DATABASE_MAX_OVERFLOW = 30
DATABASE_POOL_TIMEOUT = 30
```

### Vector Settings
```python
VECTOR_DIMENSIONS = 768  # nomic-embed-text compatible
HNSW_M = 16              # HNSW index parameter
HNSW_EF_CONSTRUCTION = 200  # Index construction parameter
```

## Troubleshooting

### Common Issues

#### Slow Queries
```sql
-- Check query performance
EXPLAIN (ANALYZE, BUFFERS) SELECT * FROM sources WHERE tenant_id = 'xxx';

-- Verify indexes
SELECT schemaname, tablename, indexname, idx_tup_read, idx_tup_fetch
FROM pg_stat_user_indexes WHERE tablename = 'sources';
```

#### Migration Problems
```bash
# Check current migration
alembic current

# View migration history
alembic history

# Rollback if needed
alembic downgrade -1
```

#### Tenant Isolation Issues
```sql
-- Verify RLS policies
SELECT schemaname, tablename, policyname, cmd, qual
FROM pg_policies WHERE tablename = 'sources';

-- Test tenant isolation
SET app.current_tenant = 'work-tenant-id';
SELECT count(*) FROM sources;  -- Should only show work tenant data
```

#### Vector Index Performance
```sql
-- Check HNSW index usage
SELECT indexname, idx_scan, idx_tup_read, idx_tup_fetch
FROM pg_stat_user_indexes
WHERE indexname LIKE '%vector%';
```

## API Reference

### Repository Classes
- `TenantRepository`: Tenant management operations
- `SourceRepository`: Content storage and retrieval
- `AssertionRepository`: AI-extracted information management
- `PersonRepository`: Person entity operations with cross-tenant support
- `ProjectRepository`: Project hierarchy management
- `EmbeddingRepository`: Vector storage and similarity search
- `EventRepository`: Event processing workflows
- `JobRepository`: Processing job management

### Service Classes
- `VectorSearchService`: High-level vector similarity operations
- `EventPublisher`: Event distribution and subscription
- `JobManager`: Processing job lifecycle management
- `TenantManager`: Multi-tenant session and context management

## Security Considerations

### Data Isolation
- All tenant data is automatically isolated via RLS policies
- No application-level filtering required - PostgreSQL enforces isolation
- Cross-tenant data access is impossible without explicit system privileges

### Access Control
- Database users have minimal required permissions
- Connection pooling isolates tenant sessions
- Audit logs capture all data access patterns

### Backup and Recovery
- Tenant data can be backed up independently
- Point-in-time recovery maintains tenant isolation
- Cross-tenant restoration requires explicit administrator action

## Production Deployment

### System Requirements
- PostgreSQL 16+ with pgvector extension
- Minimum 8GB RAM for vector operations
- SSD storage for optimal vector index performance
- Redis for event processing (optional but recommended)

### Monitoring
- Query performance via `pg_stat_statements`
- Vector index efficiency via `pg_stat_user_indexes`
- Tenant isolation verification via automated tests
- Event processing throughput monitoring

### Scaling Considerations
- Horizontal read scaling via read replicas
- Tenant-based partitioning for high-volume deployments
- Vector index optimization for large embedding collections
- Connection pool tuning for concurrent usage patterns

## Support

For issues or questions:
1. Check the troubleshooting section above
2. Review integration tests for usage patterns
3. Consult the agent context at `context/database-dev/agents.md`
4. Create a bead for database-related issues