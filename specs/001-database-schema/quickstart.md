# Quickstart: Database Schema and Storage Layer

**Feature**: Database Schema and Storage Layer
**Phase**: 1 - Implementation Ready
**Target**: Developers implementing the foundational storage layer

## Overview

This guide provides step-by-step instructions for implementing Penfold's foundational database schema and storage layer. The implementation provides hybrid relational and vector storage with event-driven processing capabilities.

## Prerequisites

### System Requirements
- **PostgreSQL 16+**: Core database with advanced features
- **pgvector Extension**: Vector similarity search capabilities
- **Redis**: Event distribution and caching
- **Python 3.12+**: Implementation language
- **Mac Mini M4 32GB RAM**: Development environment (or equivalent)

### Development Tools
```bash
# Core dependencies
pip install sqlalchemy[asyncio] alembic asyncpg redis msgpack
pip install pgvector pytest pytest-asyncio

# Development tools
pip install ruff mypy black isort
```

## Quick Start (5 minutes)

### 1. Database Setup
```bash
# Install PostgreSQL with pgvector (macOS with Homebrew)
brew install postgresql@16
brew install pgvector

# Start PostgreSQL
brew services start postgresql@16

# Create database and enable pgvector
createdb penfold_dev
psql penfold_dev -c "CREATE EXTENSION vector;"
psql penfold_dev -c "CREATE EXTENSION ltree;"
```

### 2. Redis Setup
```bash
# Install and start Redis
brew install redis
brew services start redis

# Verify Redis is running
redis-cli ping  # Should return "PONG"
```

### 3. Project Structure Setup
```bash
# Create the library structure
mkdir -p penf_lib/storage
mkdir -p penf_lib/cli
mkdir -p tests/{fixtures,unit,integration,contract}

# Initialize Python modules
touch penf_lib/__init__.py
touch penf_lib/storage/__init__.py
touch penf_lib/cli/__init__.py
```

### 4. Configuration
Create `penf_lib/storage/config.py`:
```python
import os
from urllib.parse import quote_plus

class DatabaseConfig:
    # Database connection
    DB_HOST = os.getenv('DB_HOST', 'localhost')
    DB_PORT = os.getenv('DB_PORT', '5432')
    DB_NAME = os.getenv('DB_NAME', 'penfold_dev')
    DB_USER = os.getenv('DB_USER', os.getenv('USER'))
    DB_PASSWORD = os.getenv('DB_PASSWORD', '')

    # Redis connection
    REDIS_HOST = os.getenv('REDIS_HOST', 'localhost')
    REDIS_PORT = os.getenv('REDIS_PORT', '6379')
    REDIS_DB = os.getenv('REDIS_DB', '0')

    @property
    def database_url(self) -> str:
        password_part = f":{quote_plus(self.DB_PASSWORD)}" if self.DB_PASSWORD else ""
        return f"postgresql+asyncpg://{self.DB_USER}{password_part}@{self.DB_HOST}:{self.DB_PORT}/{self.DB_NAME}"

    @property
    def redis_url(self) -> str:
        return f"redis://{self.REDIS_HOST}:{self.REDIS_PORT}/{self.REDIS_DB}"

config = DatabaseConfig()
```

### 5. Core Models Implementation
Create `penf_lib/storage/models.py`:
```python
from datetime import datetime
from typing import Optional, Dict, Any, List
import uuid

from sqlalchemy import Column, Integer, String, Text, DateTime, Boolean, DECIMAL, ARRAY, Index
from sqlalchemy.dialects.postgresql import UUID, JSONB, BIGINT
from sqlalchemy.ext.declarative import declarative_base
from sqlalchemy.sql import func
from pgvector.sqlalchemy import Vector

Base = declarative_base()

class TimestampMixin:
    """Mixin for created_at/updated_at timestamps"""
    created_at = Column(DateTime(timezone=True), server_default=func.now(), nullable=False)
    updated_at = Column(DateTime(timezone=True), server_default=func.now(), onupdate=func.now(), nullable=False)

class TenantMixin:
    """Mixin for tenant isolation"""
    tenant_id = Column(UUID(as_uuid=True), nullable=False, index=True)

class Source(Base, TimestampMixin, TenantMixin):
    __tablename__ = 'sources'

    id = Column(BIGINT, primary_key=True)

    # Content identification
    source_system = Column(String(50), nullable=False)
    external_id = Column(String(255), nullable=False)
    content_hash = Column(String(64), nullable=False)

    # Content storage
    raw_content = Column(Text)
    content_type = Column(String(100))
    content_size = Column(Integer)

    # Metadata
    ingestion_metadata = Column(JSONB, default={})
    processing_status = Column(String(20), default='pending')

    # Temporal tracking
    source_timestamp = Column(DateTime(timezone=True))

    __table_args__ = (
        Index('idx_sources_tenant_created', 'tenant_id', 'created_at'),
        Index('idx_sources_unique_external', 'tenant_id', 'source_system', 'external_id', unique=True),
        Index('idx_sources_hash', 'content_hash'),
    )

class Assertion(Base, TimestampMixin, TenantMixin):
    __tablename__ = 'assertions'

    id = Column(BIGINT, primary_key=True)
    source_id = Column(BIGINT, nullable=False)  # Foreign key handled by application

    # Assertion classification
    assertion_type = Column(String(50), nullable=False)
    content = Column(Text, nullable=False)
    context = Column(Text)

    # AI processing metadata
    confidence_score = Column(DECIMAL(4,3))
    extraction_model = Column(String(100))
    processing_metadata = Column(JSONB, default={})

    # Relationships and tags
    related_entities = Column(JSONB, default={})
    tags = Column(ARRAY(String))

    # Validation
    user_validated = Column(Boolean, default=False)
    validation_feedback = Column(JSONB, default={})

    __table_args__ = (
        Index('idx_assertions_tenant_type', 'tenant_id', 'assertion_type', 'created_at'),
        Index('idx_assertions_confidence', 'confidence_score'),
        Index('idx_assertions_source', 'source_id', 'created_at'),
    )

class ProcessingEvent(Base, TimestampMixin, TenantMixin):
    __tablename__ = 'processing_events'

    id = Column(BIGINT, primary_key=True)
    event_id = Column(UUID(as_uuid=True), default=uuid.uuid4, unique=True)

    # Event details
    event_type = Column(String(100), nullable=False)
    payload = Column(JSONB, nullable=False)
    payload_size = Column(Integer)

    # Processing tracking
    publisher = Column(String(100))
    subscriber_count = Column(Integer, default=0)
    processing_status = Column(String(20), default='published')

    # Temporal with retention
    published_at = Column(DateTime(timezone=True), server_default=func.now())
    expires_at = Column(DateTime(timezone=True), server_default=func.now() + func.make_interval(days=30))

    __table_args__ = (
        Index('idx_events_tenant_type', 'tenant_id', 'event_type', 'published_at'),
        Index('idx_events_expiry', 'expires_at'),
    )

class Embedding(Base, TimestampMixin, TenantMixin):
    __tablename__ = 'embeddings'

    id = Column(BIGINT, primary_key=True)

    # Entity linkage (polymorphic)
    entity_type = Column(String(50), nullable=False)
    entity_id = Column(BIGINT, nullable=False)
    source_id = Column(BIGINT)
    assertion_id = Column(BIGINT)

    # Vector data
    embedding = Column(Vector(768), nullable=False)
    embedding_model = Column(String(100), nullable=False)
    model_version = Column(String(50))

    # Content reference
    text_content = Column(Text)
    content_hash = Column(String(64))

    # Quality tracking
    generation_confidence = Column(DECIMAL(4,3))
    search_count = Column(Integer, default=0)
    last_searched_at = Column(DateTime(timezone=True))

    __table_args__ = (
        Index('idx_embeddings_vector_hnsw', 'embedding', postgresql_using='hnsw',
              postgresql_with={'m': 24, 'ef_construction': 100}),
        Index('idx_embeddings_entity', 'tenant_id', 'entity_type', 'entity_id'),
        Index('idx_embeddings_model', 'embedding_model', 'model_version'),
    )
```

### 6. Database Connection Setup
Create `penf_lib/storage/connections.py`:
```python
from sqlalchemy.ext.asyncio import create_async_engine, AsyncSession
from sqlalchemy.orm import sessionmaker
from sqlalchemy.pool import QueuePool
import redis.asyncio as redis
from pgvector import register_vector
from .config import config
from .models import Base

# Create async engine with connection pooling
engine = create_async_engine(
    config.database_url,
    poolclass=QueuePool,
    pool_size=10,
    max_overflow=5,
    pool_recycle=3600,
    pool_pre_ping=True,
    echo=False,  # Set to True for SQL debugging
)

# Async session factory
async_session = sessionmaker(
    engine, class_=AsyncSession, expire_on_commit=False
)

# Redis connection
redis_client = redis.from_url(config.redis_url, decode_responses=True)

async def init_database():
    """Initialize database schema"""
    async with engine.begin() as conn:
        # Register vector type
        await conn.run_sync(register_vector)
        # Create all tables
        await conn.run_sync(Base.metadata.create_all)

async def get_session() -> AsyncSession:
    """Get database session"""
    async with async_session() as session:
        yield session

async def set_tenant_context(session: AsyncSession, tenant_id: str):
    """Set tenant context for RLS"""
    await session.execute(f"SET app.tenant_id = '{tenant_id}'")
```

## Implementation Steps

### Phase 1: Core Database Layer (Week 1)

1. **Setup and Configuration** (Day 1)
   ```bash
   # Test database connection
   python -c "from penf_lib.storage.connections import init_database; import asyncio; asyncio.run(init_database())"
   ```

2. **Entity Models** (Day 2-3)
   - Implement all core models with proper validation
   - Add constraint checking and data validation
   - Test model creation and relationships

3. **Migration System** (Day 4-5)
   ```bash
   # Initialize Alembic
   alembic init alembic

   # Create first migration
   alembic revision --autogenerate -m "Initial schema"

   # Apply migration
   alembic upgrade head
   ```

### Phase 2: Event Processing (Week 2)

1. **Redis Integration** (Day 1-2)
   Create `penf_lib/storage/events.py`:
   ```python
   import asyncio
   import msgpack
   from typing import Dict, Any, Optional
   from .connections import redis_client

   class EventPublisher:
       async def publish(self, channel: str, event_data: Dict[str, Any]) -> int:
           """Publish event to Redis channel"""
           serialized = msgpack.packb(event_data)
           return await redis_client.publish(channel, serialized)

   class EventSubscriber:
       async def subscribe(self, channels: list, callback):
           """Subscribe to Redis channels"""
           pubsub = redis_client.pubsub()
           await pubsub.subscribe(*channels)

           async for message in pubsub.listen():
               if message['type'] == 'message':
                   data = msgpack.unpackb(message['data'])
                   await callback(message['channel'], data)
   ```

2. **Job Management** (Day 3-4)
   - Implement processing job state machine
   - Add retry logic and error handling
   - Create job monitoring and cleanup

3. **Integration Testing** (Day 5)
   - Test event publishing and subscription
   - Verify job state transitions
   - Performance testing with concurrent operations

### Phase 3: Vector Operations (Week 3)

1. **Vector Storage** (Day 1-2)
   Create `penf_lib/storage/vector.py`:
   ```python
   from typing import List, Tuple
   from sqlalchemy.ext.asyncio import AsyncSession
   from .models import Embedding

   async def store_embedding(
       session: AsyncSession,
       entity_type: str,
       entity_id: int,
       embedding: List[float],
       text_content: str,
       model_name: str
   ) -> Embedding:
       """Store vector embedding"""
       emb = Embedding(
           entity_type=entity_type,
           entity_id=entity_id,
           embedding=embedding,
           text_content=text_content,
           embedding_model=model_name
       )
       session.add(emb)
       await session.commit()
       return emb

   async def similarity_search(
       session: AsyncSession,
       query_vector: List[float],
       limit: int = 10,
       min_similarity: float = 0.7
   ) -> List[Tuple[Embedding, float]]:
       """Perform vector similarity search"""
       query = session.query(Embedding).order_by(
           Embedding.embedding.l2_distance(query_vector)
       ).limit(limit)

       results = await session.execute(query)
       return results.scalars().all()
   ```

2. **HNSW Optimization** (Day 3)
   - Implement index parameter tuning
   - Add search parameter optimization
   - Memory usage monitoring

3. **Performance Testing** (Day 4-5)
   - Benchmark vector operations
   - Test with realistic data volumes
   - Optimize query performance

### Phase 4: CLI Interface (Week 4)

1. **Database Commands** (Day 1-2)
   Create `penf_lib/cli/database.py`:
   ```python
   import click
   import asyncio
   from ..storage.connections import init_database

   @click.group()
   def database():
       """Database management commands"""
       pass

   @database.command()
   def init():
       """Initialize database schema"""
       asyncio.run(init_database())
       click.echo("Database initialized successfully")

   @database.command()
   @click.option('--host', help='Database host')
   @click.option('--port', help='Database port')
   def status(host, port):
       """Check database connection status"""
       # Implementation for connection testing
       pass
   ```

2. **Migration Commands** (Day 3)
   - Integrate Alembic with CLI
   - Add migration status and rollback commands
   - Implement schema validation

3. **Monitoring Commands** (Day 4-5)
   - Database performance monitoring
   - Vector index statistics
   - Event processing metrics

## Testing Strategy

### Test Database Setup
```python
# tests/fixtures/database.py
import pytest
import asyncio
from sqlalchemy.ext.asyncio import create_async_engine, AsyncSession
from sqlalchemy.orm import sessionmaker
from penf_lib.storage.models import Base

@pytest.fixture(scope="session")
def event_loop():
    loop = asyncio.new_event_loop()
    yield loop
    loop.close()

@pytest.fixture
async def test_engine():
    engine = create_async_engine("postgresql+asyncpg://user@localhost/test_db")
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.create_all)
    yield engine
    async with engine.begin() as conn:
        await conn.run_sync(Base.metadata.drop_all)

@pytest.fixture
async def test_session(test_engine):
    async_session = sessionmaker(test_engine, class_=AsyncSession)
    async with async_session() as session:
        yield session
```

### Sample Tests
```python
# tests/unit/test_models.py
import pytest
from penf_lib.storage.models import Source, Assertion

@pytest.mark.asyncio
async def test_source_creation(test_session):
    source = Source(
        tenant_id="550e8400-e29b-41d4-a716-446655440000",
        source_system="gmail",
        external_id="msg_123",
        content_hash="a" * 64,
        raw_content="Test email content"
    )
    test_session.add(source)
    await test_session.commit()

    assert source.id is not None
    assert source.processing_status == "pending"

@pytest.mark.asyncio
async def test_vector_search(test_session):
    # Test vector similarity search functionality
    pass
```

### Performance Benchmarks
```python
# tests/performance/test_vector_performance.py
import pytest
import time
from penf_lib.storage.vector import similarity_search

@pytest.mark.asyncio
async def test_vector_search_performance(test_session):
    # Generate test embeddings
    test_vectors = generate_test_vectors(1000)

    # Measure search performance
    start_time = time.time()
    results = await similarity_search(test_session, test_vectors[0])
    search_time = time.time() - start_time

    assert search_time < 0.5  # Must complete in under 500ms
    assert len(results) <= 10
```

## Configuration Examples

### Development Environment
```bash
# .env
DB_NAME=penfold_dev
DB_USER=developer
REDIS_DB=0

# Enable SQL logging for development
export SQLALCHEMY_ECHO=true
```

### Testing Environment
```bash
# .env.test
DB_NAME=penfold_test
DB_USER=test_user
REDIS_DB=1

# Use faster settings for tests
export POSTGRES_WORK_MEM=256MB
export REDIS_MAXMEMORY=100MB
```

### Production Environment
```bash
# .env.production
DB_NAME=penfold_production
DB_USER=penfold_app
DB_PASSWORD=<secure_password>
REDIS_DB=0

# Production optimizations
export POSTGRES_SHARED_BUFFERS=8GB
export POSTGRES_MAINTENANCE_WORK_MEM=4GB
export REDIS_MAXMEMORY=16GB
```

## Troubleshooting

### Common Issues

1. **pgvector Extension Not Found**
   ```bash
   # Fix: Install pgvector extension
   brew install pgvector
   psql -d penfold_dev -c "CREATE EXTENSION vector;"
   ```

2. **Redis Connection Failed**
   ```bash
   # Fix: Start Redis service
   brew services start redis
   redis-cli ping
   ```

3. **Migration Failures**
   ```bash
   # Fix: Reset migrations (development only)
   alembic downgrade base
   alembic upgrade head
   ```

4. **Vector Index Build Failures**
   ```sql
   -- Fix: Increase work memory
   SET maintenance_work_mem = '4GB';
   REINDEX INDEX embeddings_vector_idx;
   ```

### Performance Tuning

1. **PostgreSQL Settings**
   ```sql
   -- postgresql.conf optimizations
   shared_buffers = 8GB
   maintenance_work_mem = 4GB
   effective_cache_size = 24GB
   random_page_cost = 1.1
   ```

2. **Vector Index Optimization**
   ```sql
   -- Tune HNSW parameters for your data
   CREATE INDEX CONCURRENTLY embeddings_vector_idx ON embeddings
   USING hnsw (embedding vector_l2_ops)
   WITH (m = 32, ef_construction = 200);  -- Higher quality
   ```

3. **Connection Pool Tuning**
   ```python
   # Adjust for your workload
   engine = create_async_engine(
       database_url,
       pool_size=20,      # Increase for high concurrency
       max_overflow=10,   # Additional connections under load
       pool_recycle=1800  # Shorter for cloud deployments
   )
   ```

## Next Steps

After completing this quickstart:

1. **Review the generated artifacts** in your feature directory
2. **Run the test suite** to verify implementation
3. **Profile performance** with realistic data volumes
4. **Proceed to Phase 2** (Event Processing Framework) implementation
5. **Monitor resource usage** and optimize as needed

For detailed implementation guidance, refer to:
- [Data Model Documentation](./data-model.md)
- [API Contracts](./contracts/storage-api.yaml)
- [Research Findings](./research.md)

## Support

For implementation questions or issues:
1. Review the [troubleshooting section](#troubleshooting) above
2. Check the [constitution compliance](../../../.specify/memory/constitution.md)
3. Ensure all [success criteria](./spec.md#success-criteria) are being met