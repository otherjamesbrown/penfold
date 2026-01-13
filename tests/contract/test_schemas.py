"""Contract tests for schema validation in Penfold storage layer.

These tests validate that all core entity models work correctly with proper validation,
constraints, and relationships. They follow TDD principles and should fail initially
until models are properly implemented.

Testing scope:
- Core entity model creation with validation
- Required field enforcement
- Foreign key relationships
- Unique constraints
- Soft delete functionality
- Database schema integrity
"""

import pytest
import uuid
from datetime import datetime, timezone
from decimal import Decimal
from sqlalchemy.exc import IntegrityError, StatementError
from sqlalchemy import text

from penf_lib.storage.models import (
    Source,
    Assertion,
    Person,
    Project,
    Team,
    Embedding,
    ProcessingEvent,
    ProcessingJob,
    ProcessingResult,
)


class TestSourceModel:
    """Test Source model validation and constraints."""

    @pytest.mark.asyncio
    async def test_source_creation_with_valid_data(self, test_session, sample_tenant_id, sample_source_data):
        """Test that Source can be created with valid data."""
        source = Source(
            tenant_id=sample_tenant_id,
            **sample_source_data
        )

        test_session.add(source)
        await test_session.flush()

        # Should have ID and timestamps
        assert source.id is not None
        assert source.created_at is not None
        assert source.updated_at is not None
        assert source.tenant_id == sample_tenant_id
        assert source.processing_status == "pending"  # Default value
        assert source.is_deleted is False  # Default value

    @pytest.mark.asyncio
    async def test_source_required_fields_enforced(self, test_session, sample_tenant_id):
        """Test that required fields are enforced on Source creation."""
        # Missing source_system should fail
        with pytest.raises((IntegrityError, StatementError)):
            source = Source(
                tenant_id=sample_tenant_id,
                external_id="test_id",
                content_hash="a" * 64,
            )
            test_session.add(source)
            await test_session.flush()

        # Missing external_id should fail
        with pytest.raises((IntegrityError, StatementError)):
            source = Source(
                tenant_id=sample_tenant_id,
                source_system="gmail",
                content_hash="a" * 64,
            )
            test_session.add(source)
            await test_session.flush()

        # Missing content_hash should fail
        with pytest.raises((IntegrityError, StatementError)):
            source = Source(
                tenant_id=sample_tenant_id,
                source_system="gmail",
                external_id="test_id",
            )
            test_session.add(source)
            await test_session.flush()

    @pytest.mark.asyncio
    async def test_source_unique_external_constraint(self, test_session, sample_tenant_id, sample_source_data):
        """Test that unique constraint on tenant_id + source_system + external_id works."""
        # Create first source
        source1 = Source(
            tenant_id=sample_tenant_id,
            **sample_source_data
        )
        test_session.add(source1)
        await test_session.flush()

        # Try to create duplicate - should fail
        with pytest.raises(IntegrityError):
            source2 = Source(
                tenant_id=sample_tenant_id,
                source_system=sample_source_data["source_system"],
                external_id=sample_source_data["external_id"],
                content_hash="b" * 64,  # Different hash, but same system+external_id
            )
            test_session.add(source2)
            await test_session.flush()

    @pytest.mark.asyncio
    async def test_source_soft_delete_functionality(self, test_session, sample_tenant_id, sample_source_data):
        """Test that soft delete functionality works correctly."""
        source = Source(
            tenant_id=sample_tenant_id,
            **sample_source_data
        )
        test_session.add(source)
        await test_session.flush()

        # Mark as deleted
        source.is_deleted = True
        source.deleted_at = datetime.now(timezone.utc)
        await test_session.flush()

        assert source.is_deleted is True
        assert source.deleted_at is not None


class TestAssertionModel:
    """Test Assertion model validation and constraints."""

    @pytest.mark.asyncio
    async def test_assertion_creation_with_valid_data(self, test_session, test_source, sample_tenant_id, sample_assertion_data):
        """Test that Assertion can be created with valid data."""
        assertion = Assertion(
            tenant_id=sample_tenant_id,
            source_id=test_source.id,
            **sample_assertion_data
        )

        test_session.add(assertion)
        await test_session.flush()

        assert assertion.id is not None
        assert assertion.source_id == test_source.id
        assert assertion.user_validated is False  # Default value
        assert assertion.confidence_score == Decimal('0.950')

    @pytest.mark.asyncio
    async def test_assertion_required_fields_enforced(self, test_session, test_source, sample_tenant_id):
        """Test that required fields are enforced on Assertion creation."""
        # Missing assertion_type should fail
        with pytest.raises((IntegrityError, StatementError)):
            assertion = Assertion(
                tenant_id=sample_tenant_id,
                source_id=test_source.id,
                content="Test content"
            )
            test_session.add(assertion)
            await test_session.flush()

        # Missing content should fail
        with pytest.raises((IntegrityError, StatementError)):
            assertion = Assertion(
                tenant_id=sample_tenant_id,
                source_id=test_source.id,
                assertion_type="decision"
            )
            test_session.add(assertion)
            await test_session.flush()

        # Missing source_id should fail
        with pytest.raises((IntegrityError, StatementError)):
            assertion = Assertion(
                tenant_id=sample_tenant_id,
                assertion_type="decision",
                content="Test content"
            )
            test_session.add(assertion)
            await test_session.flush()

    @pytest.mark.asyncio
    async def test_assertion_foreign_key_relationship(self, test_session, test_source, sample_tenant_id):
        """Test that foreign key relationship to Source works correctly."""
        assertion = Assertion(
            tenant_id=sample_tenant_id,
            source_id=test_source.id,
            assertion_type="decision",
            content="Test assertion content"
        )
        test_session.add(assertion)
        await test_session.flush()

        # Should be able to access source through relationship
        assert assertion.source is not None
        assert assertion.source.id == test_source.id

        # Source should have assertion in its assertions collection
        assert len(test_source.assertions) > 0
        assert assertion in test_source.assertions

    @pytest.mark.asyncio
    async def test_assertion_invalid_source_id_fails(self, test_session, sample_tenant_id):
        """Test that referencing non-existent source_id fails."""
        with pytest.raises(IntegrityError):
            assertion = Assertion(
                tenant_id=sample_tenant_id,
                source_id=999999,  # Non-existent source ID
                assertion_type="decision",
                content="Test content"
            )
            test_session.add(assertion)
            await test_session.flush()


class TestPersonModel:
    """Test Person model validation and constraints."""

    @pytest.mark.asyncio
    async def test_person_creation_with_valid_data(self, test_session, sample_tenant_id, sample_person_data):
        """Test that Person can be created with valid data."""
        person = Person(
            tenant_id=sample_tenant_id,
            **sample_person_data
        )

        test_session.add(person)
        await test_session.flush()

        assert person.id is not None
        assert person.canonical_name == sample_person_data["canonical_name"]
        assert person.aliases == sample_person_data["aliases"]
        assert person.email_addresses == sample_person_data["email_addresses"]

    @pytest.mark.asyncio
    async def test_person_required_fields_enforced(self, test_session, sample_tenant_id):
        """Test that required fields are enforced on Person creation."""
        # Missing canonical_name should fail
        with pytest.raises((IntegrityError, StatementError)):
            person = Person(
                tenant_id=sample_tenant_id,
                job_title="Engineer"
            )
            test_session.add(person)
            await test_session.flush()

    @pytest.mark.asyncio
    async def test_person_array_fields_work(self, test_session, sample_tenant_id):
        """Test that ARRAY fields work correctly for Person."""
        person = Person(
            tenant_id=sample_tenant_id,
            canonical_name="Test Person",
            aliases=["Alias 1", "Alias 2", "Alias 3"],
            email_addresses=["email1@test.com", "email2@test.com"],
            phone_numbers=["+1-555-0001", "+1-555-0002"]
        )

        test_session.add(person)
        await test_session.flush()

        assert len(person.aliases) == 3
        assert len(person.email_addresses) == 2
        assert len(person.phone_numbers) == 2
        assert "Alias 1" in person.aliases
        assert "email1@test.com" in person.email_addresses


class TestProjectModel:
    """Test Project model validation and constraints."""

    @pytest.mark.asyncio
    async def test_project_creation_with_valid_data(self, test_session, sample_tenant_id, sample_project_data):
        """Test that Project can be created with valid data."""
        project = Project(
            tenant_id=sample_tenant_id,
            **sample_project_data
        )

        test_session.add(project)
        await test_session.flush()

        assert project.id is not None
        assert project.name == sample_project_data["name"]
        assert project.status == "active"  # Default from sample data
        assert project.artifacts == sample_project_data["artifacts"]

    @pytest.mark.asyncio
    async def test_project_required_fields_enforced(self, test_session, sample_tenant_id):
        """Test that required fields are enforced on Project creation."""
        # Missing name should fail
        with pytest.raises((IntegrityError, StatementError)):
            project = Project(
                tenant_id=sample_tenant_id,
                description="Test description",
                status="active"
            )
            test_session.add(project)
            await test_session.flush()

    @pytest.mark.asyncio
    async def test_project_hierarchical_relationship(self, test_session, sample_tenant_id):
        """Test that hierarchical parent-child relationship works."""
        # Create parent project
        parent_project = Project(
            tenant_id=sample_tenant_id,
            name="Parent Project",
            description="Parent project description"
        )
        test_session.add(parent_project)
        await test_session.flush()

        # Create child project
        child_project = Project(
            tenant_id=sample_tenant_id,
            name="Child Project",
            description="Child project description",
            parent_id=parent_project.id
        )
        test_session.add(child_project)
        await test_session.flush()

        # Test relationships
        assert child_project.parent_id == parent_project.id
        assert child_project.parent == parent_project
        assert child_project in parent_project.children

    @pytest.mark.asyncio
    async def test_project_jsonb_fields_work(self, test_session, sample_tenant_id):
        """Test that JSONB fields work correctly for Project."""
        artifacts = [
            {"type": "document", "url": "https://example.com/doc1"},
            {"type": "spreadsheet", "url": "https://example.com/sheet1"}
        ]
        milestones = [
            {"name": "Phase 1 Complete", "date": "2024-03-01", "status": "completed"},
            {"name": "Phase 2 Complete", "date": "2024-06-01", "status": "pending"}
        ]

        project = Project(
            tenant_id=sample_tenant_id,
            name="Test Project with JSONB",
            artifacts=artifacts,
            milestones=milestones
        )

        test_session.add(project)
        await test_session.flush()

        assert project.artifacts == artifacts
        assert project.milestones == milestones
        assert len(project.artifacts) == 2
        assert project.artifacts[0]["type"] == "document"


class TestTeamModel:
    """Test Team model validation and constraints."""

    @pytest.mark.asyncio
    async def test_team_creation_with_valid_data(self, test_session, sample_tenant_id):
        """Test that Team can be created with valid data."""
        team = Team(
            tenant_id=sample_tenant_id,
            name="Engineering Team",
            description="Software engineering team",
            team_type="department",
            member_person_ids=[1, 2, 3],
            location="San Francisco"
        )

        test_session.add(team)
        await test_session.flush()

        assert team.id is not None
        assert team.name == "Engineering Team"
        assert team.team_type == "department"
        assert team.member_person_ids == [1, 2, 3]

    @pytest.mark.asyncio
    async def test_team_required_fields_enforced(self, test_session, sample_tenant_id):
        """Test that required fields are enforced on Team creation."""
        # Missing name should fail
        with pytest.raises((IntegrityError, StatementError)):
            team = Team(
                tenant_id=sample_tenant_id,
                description="Test team"
            )
            test_session.add(team)
            await test_session.flush()

    @pytest.mark.asyncio
    async def test_team_hierarchical_relationship(self, test_session, sample_tenant_id):
        """Test that hierarchical parent-child relationship works for teams."""
        # Create parent team
        parent_team = Team(
            tenant_id=sample_tenant_id,
            name="Engineering Department",
            team_type="department"
        )
        test_session.add(parent_team)
        await test_session.flush()

        # Create child team
        child_team = Team(
            tenant_id=sample_tenant_id,
            name="Backend Team",
            team_type="project team",
            parent_team_id=parent_team.id
        )
        test_session.add(child_team)
        await test_session.flush()

        # Test relationships
        assert child_team.parent_team_id == parent_team.id
        assert child_team.parent_team == parent_team
        assert child_team in parent_team.children


class TestEmbeddingModel:
    """Test Embedding model validation and constraints."""

    @pytest.mark.asyncio
    async def test_embedding_creation_with_source(self, test_session, test_source, sample_tenant_id, sample_embedding_vector):
        """Test that Embedding can be created with source reference."""
        embedding = Embedding(
            tenant_id=sample_tenant_id,
            entity_type="source",
            entity_id=test_source.id,
            source_id=test_source.id,
            embedding=sample_embedding_vector,
            embedding_model="nomic-embed-text",
            text_content="Sample text content for embedding"
        )

        test_session.add(embedding)
        await test_session.flush()

        assert embedding.id is not None
        assert embedding.entity_type == "source"
        assert embedding.entity_id == test_source.id
        assert embedding.source_id == test_source.id
        assert len(embedding.embedding) == 768
        assert embedding.search_count == 0  # Default value

    @pytest.mark.asyncio
    async def test_embedding_required_fields_enforced(self, test_session, sample_tenant_id):
        """Test that required fields are enforced on Embedding creation."""
        # Missing entity_type should fail
        with pytest.raises((IntegrityError, StatementError)):
            embedding = Embedding(
                tenant_id=sample_tenant_id,
                entity_id=1,
                embedding=[0.1] * 768,
                embedding_model="test-model"
            )
            test_session.add(embedding)
            await test_session.flush()

        # Missing embedding should fail
        with pytest.raises((IntegrityError, StatementError)):
            embedding = Embedding(
                tenant_id=sample_tenant_id,
                entity_type="source",
                entity_id=1,
                embedding_model="test-model"
            )
            test_session.add(embedding)
            await test_session.flush()

    @pytest.mark.asyncio
    async def test_embedding_vector_dimension(self, test_session, test_source, sample_tenant_id):
        """Test that embedding vector has correct dimensions."""
        # 768-dimensional vector should work
        valid_embedding = [0.1] * 768
        embedding = Embedding(
            tenant_id=sample_tenant_id,
            entity_type="source",
            entity_id=test_source.id,
            source_id=test_source.id,
            embedding=valid_embedding,
            embedding_model="nomic-embed-text"
        )

        test_session.add(embedding)
        await test_session.flush()

        assert len(embedding.embedding) == 768

        # TODO: Test that wrong dimensions fail once pgvector validation is in place
        # This might not fail in development without proper pgvector setup


class TestProcessingEventModel:
    """Test ProcessingEvent model validation and constraints."""

    @pytest.mark.asyncio
    async def test_processing_event_creation(self, test_session, sample_tenant_id):
        """Test that ProcessingEvent can be created with valid data."""
        event_payload = {"source_id": 123, "action": "extracted"}

        event = ProcessingEvent(
            tenant_id=sample_tenant_id,
            event_type="content.ingested",
            payload=event_payload,
            publisher="ingestion-service"
        )

        test_session.add(event)
        await test_session.flush()

        assert event.id is not None
        assert event.event_id is not None  # Should be auto-generated UUID
        assert event.event_type == "content.ingested"
        assert event.payload == event_payload
        assert event.subscriber_count == 0  # Default value
        assert event.processing_status == "published"  # Default value

    @pytest.mark.asyncio
    async def test_processing_event_required_fields(self, test_session, sample_tenant_id):
        """Test that required fields are enforced."""
        # Missing event_type should fail
        with pytest.raises((IntegrityError, StatementError)):
            event = ProcessingEvent(
                tenant_id=sample_tenant_id,
                payload={"test": "data"}
            )
            test_session.add(event)
            await test_session.flush()

        # Missing payload should fail
        with pytest.raises((IntegrityError, StatementError)):
            event = ProcessingEvent(
                tenant_id=sample_tenant_id,
                event_type="test.event"
            )
            test_session.add(event)
            await test_session.flush()


class TestProcessingJobModel:
    """Test ProcessingJob model validation and constraints."""

    @pytest.mark.asyncio
    async def test_processing_job_creation(self, test_session, sample_tenant_id):
        """Test that ProcessingJob can be created with valid data."""
        # Create a processing event first
        event = ProcessingEvent(
            tenant_id=sample_tenant_id,
            event_type="content.ingested",
            payload={"source_id": 123}
        )
        test_session.add(event)
        await test_session.flush()

        # Create processing job
        job = ProcessingJob(
            tenant_id=sample_tenant_id,
            event_id=event.event_id,
            processor_name="extraction-processor",
            job_type="content_extraction"
        )

        test_session.add(job)
        await test_session.flush()

        assert job.id is not None
        assert job.job_id is not None  # Should be auto-generated UUID
        assert job.event_id == event.event_id
        assert job.status == "queued"  # Default value
        assert job.priority == 100  # Default value
        assert job.retry_count == 0  # Default value
        assert job.max_retries == 3  # Default value

    @pytest.mark.asyncio
    async def test_processing_job_foreign_key_constraint(self, test_session, sample_tenant_id):
        """Test that foreign key constraint to ProcessingEvent works."""
        # Try to create job with non-existent event_id
        fake_event_id = uuid.uuid4()

        with pytest.raises(IntegrityError):
            job = ProcessingJob(
                tenant_id=sample_tenant_id,
                event_id=fake_event_id,
                processor_name="test-processor",
                job_type="test_job"
            )
            test_session.add(job)
            await test_session.flush()


class TestProcessingResultModel:
    """Test ProcessingResult model validation and constraints."""

    @pytest.mark.asyncio
    async def test_processing_result_creation(self, test_session, sample_tenant_id):
        """Test that ProcessingResult can be created with valid data."""
        # Create event and job first
        event = ProcessingEvent(
            tenant_id=sample_tenant_id,
            event_type="content.ingested",
            payload={"source_id": 123}
        )
        test_session.add(event)
        await test_session.flush()

        job = ProcessingJob(
            tenant_id=sample_tenant_id,
            event_id=event.event_id,
            processor_name="test-processor",
            job_type="extraction"
        )
        test_session.add(job)
        await test_session.flush()

        # Create processing result
        result_data = {"assertions": [{"type": "decision", "content": "We chose option A"}]}

        result = ProcessingResult(
            tenant_id=sample_tenant_id,
            job_id=job.job_id,
            result_type="assertion_extraction",
            result_data=result_data,
            confidence_score=Decimal('0.850'),
            model_name="llama-3.1-8b"
        )

        test_session.add(result)
        await test_session.flush()

        assert result.id is not None
        assert result.job_id == job.job_id
        assert result.result_data == result_data
        assert result.confidence_score == Decimal('0.850')

        # Test relationship
        assert result.job == job
        assert result in job.results


class TestSchemaIntegrity:
    """Test overall schema integrity and cross-model constraints."""

    @pytest.mark.asyncio
    async def test_tenant_isolation(self, test_engine):
        """Test that tenant isolation works correctly."""
        from sqlalchemy.ext.asyncio import async_sessionmaker, AsyncSession

        SessionLocal = async_sessionmaker(
            test_engine,
            class_=AsyncSession,
            expire_on_commit=False,
        )

        tenant_id_1 = uuid.uuid4()
        tenant_id_2 = uuid.uuid4()

        # Create source for tenant 1
        async with SessionLocal() as session1:
            await session1.execute(
                text("SET app.tenant_id = :tenant_id"),
                {"tenant_id": str(tenant_id_1)}
            )

            source1 = Source(
                tenant_id=tenant_id_1,
                source_system="gmail",
                external_id="msg_001",
                content_hash="a" * 64,
                raw_content="Tenant 1 content"
            )
            session1.add(source1)
            await session1.commit()

        # Create source for tenant 2
        async with SessionLocal() as session2:
            await session2.execute(
                text("SET app.tenant_id = :tenant_id"),
                {"tenant_id": str(tenant_id_2)}
            )

            source2 = Source(
                tenant_id=tenant_id_2,
                source_system="gmail",
                external_id="msg_001",  # Same external_id as tenant 1
                content_hash="b" * 64,
                raw_content="Tenant 2 content"
            )
            session2.add(source2)
            await session2.commit()

        # Both sources should be created successfully (different tenants)
        # This test validates that the unique constraint is tenant-scoped

    @pytest.mark.asyncio
    async def test_cascade_relationships_soft_delete(self, test_session, sample_tenant_id):
        """Test that soft delete doesn't break relationships."""
        # Create source
        source = Source(
            tenant_id=sample_tenant_id,
            source_system="gmail",
            external_id="cascade_test",
            content_hash="c" * 64,
            raw_content="Test content for cascade"
        )
        test_session.add(source)
        await test_session.flush()

        # Create assertion linked to source
        assertion = Assertion(
            tenant_id=sample_tenant_id,
            source_id=source.id,
            assertion_type="decision",
            content="Test decision from cascade test"
        )
        test_session.add(assertion)
        await test_session.flush()

        # Create embedding linked to source
        embedding = Embedding(
            tenant_id=sample_tenant_id,
            entity_type="source",
            entity_id=source.id,
            source_id=source.id,
            embedding=[0.1] * 768,
            embedding_model="test-model",
            text_content="Test content"
        )
        test_session.add(embedding)
        await test_session.flush()

        # Soft delete the source
        source.is_deleted = True
        source.deleted_at = datetime.now(timezone.utc)
        await test_session.flush()

        # Assertions and embeddings should still exist and be accessible
        assert assertion.source.is_deleted is True
        assert embedding.source.is_deleted is True

    @pytest.mark.asyncio
    async def test_database_extensions_available(self, test_engine):
        """Test that required PostgreSQL extensions are available."""
        async with test_engine.begin() as conn:
            # Test vector extension
            result = await conn.execute(
                text("SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'vector')")
            )
            vector_exists = result.scalar()
            assert vector_exists is True, "pgvector extension not available"

            # Test ltree extension
            result = await conn.execute(
                text("SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'ltree')")
            )
            ltree_exists = result.scalar()
            assert ltree_exists is True, "ltree extension not available"

            # Test pg_trgm extension
            result = await conn.execute(
                text("SELECT EXISTS(SELECT 1 FROM pg_extension WHERE extname = 'pg_trgm')")
            )
            pg_trgm_exists = result.scalar()
            assert pg_trgm_exists is True, "pg_trgm extension not available"

    @pytest.mark.asyncio
    async def test_all_tables_created(self, test_engine):
        """Test that all expected tables are created in the database."""
        expected_tables = {
            'sources', 'assertions', 'people', 'projects', 'teams',
            'embeddings', 'processing_events', 'processing_jobs', 'processing_results'
        }

        async with test_engine.begin() as conn:
            result = await conn.execute(
                text("""
                    SELECT table_name
                    FROM information_schema.tables
                    WHERE table_schema = 'public'
                    AND table_type = 'BASE TABLE'
                """)
            )
            actual_tables = {row[0] for row in result.fetchall()}

            missing_tables = expected_tables - actual_tables
            assert not missing_tables, f"Missing tables: {missing_tables}"

    @pytest.mark.asyncio
    async def test_indexes_created(self, test_engine):
        """Test that expected indexes are created."""
        # This is a basic test - in production you'd check for specific indexes
        async with test_engine.begin() as conn:
            result = await conn.execute(
                text("""
                    SELECT COUNT(*)
                    FROM pg_indexes
                    WHERE schemaname = 'public'
                """)
            )
            index_count = result.scalar()

            # Should have many indexes based on our model definitions
            assert index_count > 20, f"Expected many indexes, found only {index_count}"