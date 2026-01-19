"""Test database fixtures for Penfold storage layer."""

import asyncio
import pytest
import uuid
from typing import AsyncGenerator

from sqlalchemy.ext.asyncio import AsyncSession, create_async_engine, async_sessionmaker
from sqlalchemy import text

from penf_lib.storage.models import Base
from penf_lib.storage.config import config


@pytest.fixture(scope="session")
def event_loop():
    """Create an instance of the default event loop for the test session."""
    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    yield loop
    loop.close()


@pytest.fixture(scope="session")
async def test_engine():
    """Create test database engine and initialize schema."""
    # Use test database URL
    test_engine = create_async_engine(
        config.database.test_database_url,
        echo=False,  # Disable SQL logging during tests
        pool_pre_ping=True,
    )

    # Create all tables in test database
    async with test_engine.begin() as conn:
        # Enable required extensions
        await conn.execute(text("CREATE EXTENSION IF NOT EXISTS vector"))
        await conn.execute(text("CREATE EXTENSION IF NOT EXISTS ltree"))
        await conn.execute(text("CREATE EXTENSION IF NOT EXISTS pg_trgm"))

        # Create all tables
        await conn.run_sync(Base.metadata.create_all)

    yield test_engine

    # Clean up after all tests
    async with test_engine.begin() as conn:
        await conn.run_sync(Base.metadata.drop_all)

    await test_engine.dispose()


@pytest.fixture
async def test_session(test_engine) -> AsyncGenerator[AsyncSession, None]:
    """Create a test database session with automatic rollback."""
    TestSessionLocal = async_sessionmaker(
        test_engine,
        class_=AsyncSession,
        expire_on_commit=False,
    )

    async with TestSessionLocal() as session:
        # Set tenant context for tests
        test_tenant_id = str(uuid.uuid4())
        await session.execute(
            text("SET app.tenant_id = :tenant_id"),
            {"tenant_id": test_tenant_id}
        )

        # Begin a transaction
        transaction = await session.begin()

        try:
            yield session
        finally:
            # Always rollback to keep tests isolated
            await transaction.rollback()


@pytest.fixture
async def test_session_with_tenant(test_engine) -> AsyncGenerator[tuple[AsyncSession, str], None]:
    """Create a test database session with specific tenant ID."""
    TestSessionLocal = async_sessionmaker(
        test_engine,
        class_=AsyncSession,
        expire_on_commit=False,
    )

    test_tenant_id = str(uuid.uuid4())

    async with TestSessionLocal() as session:
        # Set tenant context
        await session.execute(
            text("SET app.tenant_id = :tenant_id"),
            {"tenant_id": test_tenant_id}
        )

        # Begin a transaction
        transaction = await session.begin()

        try:
            yield session, test_tenant_id
        finally:
            # Always rollback to keep tests isolated
            await transaction.rollback()


@pytest.fixture
def sample_tenant_id() -> str:
    """Generate a sample tenant ID for testing."""
    return str(uuid.uuid4())


@pytest.fixture
def sample_source_data():
    """Sample source data for testing."""
    return {
        "source_system": "gmail",
        "external_id": "msg_12345",
        "content_hash": "a" * 64,
        "raw_content": "This is a sample email content for testing.",
        "content_type": "text/plain",
        "content_size": 45,
        "ingestion_metadata": {"sender": "test@example.com"},
    }


@pytest.fixture
def sample_assertion_data():
    """Sample assertion data for testing."""
    return {
        "assertion_type": "decision",
        "content": "We decided to use PostgreSQL for the database.",
        "context": "Discussion about database technology choices.",
        "confidence_score": 0.95,
        "extraction_model": "test-model-v1",
        "processing_metadata": {"extraction_time": 150},
    }


@pytest.fixture
def sample_person_data():
    """Sample person data for testing."""
    return {
        "canonical_name": "John Doe",
        "aliases": ["Johnny", "J. Doe"],
        "email_addresses": ["john@example.com", "john.doe@company.com"],
        "job_title": "Senior Engineer",
        "company": "Test Company",
        "department": "Engineering",
    }


@pytest.fixture
def sample_project_data():
    """Sample project data for testing."""
    return {
        "name": "Test Project",
        "description": "A sample project for testing.",
        "status": "active",
        "project_type": "development",
        "priority": "high",
        "artifacts": [{"type": "document", "url": "https://example.com/doc"}],
    }


@pytest.fixture
def sample_embedding_vector():
    """Sample 768-dimensional embedding vector for testing."""
    import random
    random.seed(42)  # Ensure reproducible test data
    return [random.uniform(-1.0, 1.0) for _ in range(768)]


# Integration test fixtures
@pytest.fixture
async def test_source(test_session, sample_tenant_id, sample_source_data):
    """Create a test source entity."""
    from penf_lib.storage.models import Source

    source = Source(
        tenant_id=sample_tenant_id,
        **sample_source_data
    )
    test_session.add(source)
    await test_session.flush()  # Get ID without committing

    return source


@pytest.fixture
async def test_assertion(test_session, test_source, sample_tenant_id, sample_assertion_data):
    """Create a test assertion entity."""
    from penf_lib.storage.models import Assertion

    assertion = Assertion(
        tenant_id=sample_tenant_id,
        source_id=test_source.id,
        **sample_assertion_data
    )
    test_session.add(assertion)
    await test_session.flush()

    return assertion


@pytest.fixture
async def test_person(test_session, sample_tenant_id, sample_person_data):
    """Create a test person entity."""
    from penf_lib.storage.models import Person

    person = Person(
        tenant_id=sample_tenant_id,
        **sample_person_data
    )
    test_session.add(person)
    await test_session.flush()

    return person


# Performance testing fixtures
@pytest.fixture
def performance_test_data():
    """Generate data for performance testing."""
    import random

    sources = []
    for i in range(100):
        sources.append({
            "source_system": "gmail",
            "external_id": f"msg_{i:05d}",
            "content_hash": f"{i:064d}",
            "raw_content": f"Sample email content number {i}",
            "content_type": "text/plain",
            "content_size": 20 + i,
        })

    return {"sources": sources}


@pytest.fixture
async def cleanup_test_data(test_session):
    """Fixture to clean up test data after each test."""
    yield
    # Any cleanup logic can go here
    # The session rollback in test_session fixture handles most cleanup