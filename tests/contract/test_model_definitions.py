"""Contract tests for model definitions without database dependency.

These tests validate that models are properly defined with correct attributes,
relationships, and constraints at the class level before database integration.
"""

import pytest
import uuid
from decimal import Decimal
from sqlalchemy import inspect
from sqlalchemy.dialects.postgresql import UUID, JSONB, BIGINT, ARRAY

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
    TimestampMixin,
    TenantMixin,
    SoftDeleteMixin,
)


class TestModelDefinitions:
    """Test that model classes are correctly defined."""

    def test_source_model_attributes(self):
        """Test that Source model has all required attributes."""
        # Test inheritance
        assert issubclass(Source, TimestampMixin)
        assert issubclass(Source, TenantMixin)
        assert issubclass(Source, SoftDeleteMixin)

        # Test table name
        assert Source.__tablename__ == "sources"

        # Test that model can be instantiated
        source = Source()
        assert source is not None

        # Test required columns exist
        assert hasattr(Source, 'id')
        assert hasattr(Source, 'source_system')
        assert hasattr(Source, 'external_id')
        assert hasattr(Source, 'content_hash')
        assert hasattr(Source, 'raw_content')
        assert hasattr(Source, 'processing_status')

        # Test inherited columns
        assert hasattr(Source, 'created_at')
        assert hasattr(Source, 'updated_at')
        assert hasattr(Source, 'tenant_id')
        assert hasattr(Source, 'is_deleted')
        assert hasattr(Source, 'deleted_at')

        # Test relationships
        assert hasattr(Source, 'assertions')
        assert hasattr(Source, 'embeddings')

    def test_assertion_model_attributes(self):
        """Test that Assertion model has all required attributes."""
        assert issubclass(Assertion, TimestampMixin)
        assert issubclass(Assertion, TenantMixin)
        assert issubclass(Assertion, SoftDeleteMixin)

        assert Assertion.__tablename__ == "assertions"

        # Test required columns
        assert hasattr(Assertion, 'id')
        assert hasattr(Assertion, 'source_id')
        assert hasattr(Assertion, 'assertion_type')
        assert hasattr(Assertion, 'content')
        assert hasattr(Assertion, 'confidence_score')
        assert hasattr(Assertion, 'user_validated')

        # Test relationships
        assert hasattr(Assertion, 'source')

    def test_person_model_attributes(self):
        """Test that Person model has all required attributes."""
        assert issubclass(Person, TimestampMixin)
        assert issubclass(Person, TenantMixin)
        assert issubclass(Person, SoftDeleteMixin)

        assert Person.__tablename__ == "people"

        # Test required columns
        assert hasattr(Person, 'id')
        assert hasattr(Person, 'canonical_name')
        assert hasattr(Person, 'aliases')
        assert hasattr(Person, 'email_addresses')
        assert hasattr(Person, 'job_title')

    def test_project_model_attributes(self):
        """Test that Project model has all required attributes."""
        assert issubclass(Project, TimestampMixin)
        assert issubclass(Project, TenantMixin)
        assert issubclass(Project, SoftDeleteMixin)

        assert Project.__tablename__ == "projects"

        # Test required columns
        assert hasattr(Project, 'id')
        assert hasattr(Project, 'name')
        assert hasattr(Project, 'description')
        assert hasattr(Project, 'status')
        assert hasattr(Project, 'parent_id')
        assert hasattr(Project, 'artifacts')

        # Test relationships
        assert hasattr(Project, 'children')
        assert hasattr(Project, 'parent')

    def test_team_model_attributes(self):
        """Test that Team model has all required attributes."""
        assert issubclass(Team, TimestampMixin)
        assert issubclass(Team, TenantMixin)
        assert issubclass(Team, SoftDeleteMixin)

        assert Team.__tablename__ == "teams"

        # Test required columns
        assert hasattr(Team, 'id')
        assert hasattr(Team, 'name')
        assert hasattr(Team, 'team_type')
        assert hasattr(Team, 'parent_team_id')
        assert hasattr(Team, 'member_person_ids')

        # Test relationships
        assert hasattr(Team, 'children')

    def test_embedding_model_attributes(self):
        """Test that Embedding model has all required attributes."""
        assert issubclass(Embedding, TimestampMixin)
        assert issubclass(Embedding, TenantMixin)

        assert Embedding.__tablename__ == "embeddings"

        # Test required columns
        assert hasattr(Embedding, 'id')
        assert hasattr(Embedding, 'entity_type')
        assert hasattr(Embedding, 'entity_id')
        assert hasattr(Embedding, 'embedding')
        assert hasattr(Embedding, 'embedding_model')

        # Test foreign key references
        assert hasattr(Embedding, 'source_id')
        assert hasattr(Embedding, 'assertion_id')
        assert hasattr(Embedding, 'person_id')
        assert hasattr(Embedding, 'project_id')
        assert hasattr(Embedding, 'team_id')

    def test_processing_event_model_attributes(self):
        """Test that ProcessingEvent model has all required attributes."""
        assert issubclass(ProcessingEvent, TimestampMixin)
        assert issubclass(ProcessingEvent, TenantMixin)

        assert ProcessingEvent.__tablename__ == "processing_events"

        # Test required columns
        assert hasattr(ProcessingEvent, 'id')
        assert hasattr(ProcessingEvent, 'event_id')
        assert hasattr(ProcessingEvent, 'event_type')
        assert hasattr(ProcessingEvent, 'payload')
        assert hasattr(ProcessingEvent, 'published_at')
        assert hasattr(ProcessingEvent, 'expires_at')

    def test_processing_job_model_attributes(self):
        """Test that ProcessingJob model has all required attributes."""
        assert issubclass(ProcessingJob, TimestampMixin)
        assert issubclass(ProcessingJob, TenantMixin)

        assert ProcessingJob.__tablename__ == "processing_jobs"

        # Test required columns
        assert hasattr(ProcessingJob, 'id')
        assert hasattr(ProcessingJob, 'job_id')
        assert hasattr(ProcessingJob, 'event_id')
        assert hasattr(ProcessingJob, 'processor_name')
        assert hasattr(ProcessingJob, 'job_type')
        assert hasattr(ProcessingJob, 'status')
        assert hasattr(ProcessingJob, 'retry_count')
        assert hasattr(ProcessingJob, 'max_retries')

    def test_processing_result_model_attributes(self):
        """Test that ProcessingResult model has all required attributes."""
        assert issubclass(ProcessingResult, TimestampMixin)
        assert issubclass(ProcessingResult, TenantMixin)

        assert ProcessingResult.__tablename__ == "processing_results"

        # Test required columns
        assert hasattr(ProcessingResult, 'id')
        assert hasattr(ProcessingResult, 'job_id')
        assert hasattr(ProcessingResult, 'result_type')
        assert hasattr(ProcessingResult, 'result_data')
        assert hasattr(ProcessingResult, 'expires_at')

        # Test relationships
        assert hasattr(ProcessingResult, 'job')

    def test_mixin_functionality(self):
        """Test that mixins provide expected functionality."""
        # Test that models get mixin attributes
        source = Source()

        # TimestampMixin
        assert hasattr(source, 'created_at')
        assert hasattr(source, 'updated_at')

        # TenantMixin
        assert hasattr(source, 'tenant_id')

        # SoftDeleteMixin
        assert hasattr(source, 'deleted_at')
        assert hasattr(source, 'is_deleted')

    def test_model_instantiation_with_data(self):
        """Test that models can be instantiated with sample data."""
        tenant_id = uuid.uuid4()

        # Test Source instantiation
        source = Source(
            tenant_id=tenant_id,
            source_system="gmail",
            external_id="test_123",
            content_hash="a" * 64,
            raw_content="Test content"
        )
        assert source.tenant_id == tenant_id
        assert source.source_system == "gmail"
        assert source.external_id == "test_123"

        # Test Assertion instantiation
        assertion = Assertion(
            tenant_id=tenant_id,
            source_id=1,  # Would be FK to actual source
            assertion_type="decision",
            content="Test assertion content",
            confidence_score=Decimal('0.95')
        )
        assert assertion.assertion_type == "decision"
        assert assertion.confidence_score == Decimal('0.95')

        # Test Person instantiation
        person = Person(
            tenant_id=tenant_id,
            canonical_name="John Doe",
            aliases=["Johnny", "J. Doe"],
            email_addresses=["john@example.com"]
        )
        assert person.canonical_name == "John Doe"
        assert person.aliases == ["Johnny", "J. Doe"]

        # Test Project instantiation
        project = Project(
            tenant_id=tenant_id,
            name="Test Project",
            description="Test project description",
            status="active"
        )
        assert project.name == "Test Project"
        assert project.status == "active"

        # Test Team instantiation
        team = Team(
            tenant_id=tenant_id,
            name="Engineering Team",
            team_type="department"
        )
        assert team.name == "Engineering Team"
        assert team.team_type == "department"


class TestTableDefinitions:
    """Test table-level definitions and constraints."""

    def test_all_models_have_table_names(self):
        """Test that all models have defined table names."""
        models = [Source, Assertion, Person, Project, Team, Embedding,
                 ProcessingEvent, ProcessingJob, ProcessingResult]

        expected_table_names = {
            "sources", "assertions", "people", "projects", "teams",
            "embeddings", "processing_events", "processing_jobs", "processing_results"
        }

        actual_table_names = {model.__tablename__ for model in models}
        assert actual_table_names == expected_table_names

    def test_all_models_have_indexes(self):
        """Test that all models have defined indexes."""
        models = [Source, Assertion, Person, Project, Team, Embedding,
                 ProcessingEvent, ProcessingJob, ProcessingResult]

        for model in models:
            assert hasattr(model, '__table_args__')
            assert model.__table_args__ is not None
            # Should have at least one index
            assert len(model.__table_args__) > 0

    def test_foreign_key_relationships_defined(self):
        """Test that foreign key relationships are properly defined."""
        # Assertion -> Source
        assert hasattr(Assertion, 'source')
        assert hasattr(Source, 'assertions')

        # Project -> Project (self-referential)
        assert hasattr(Project, 'parent')
        assert hasattr(Project, 'children')

        # Team -> Team (self-referential)
        assert hasattr(Team, 'parent_team')
        assert hasattr(Team, 'children')

        # ProcessingJob -> ProcessingEvent
        # (This would be tested in integration tests with actual FK constraints)

        # ProcessingResult -> ProcessingJob
        assert hasattr(ProcessingResult, 'job')
        assert hasattr(ProcessingJob, 'results')

        # Embedding -> multiple entities
        assert hasattr(Embedding, 'source')
        assert hasattr(Embedding, 'assertion')
        assert hasattr(Embedding, 'person')
        assert hasattr(Embedding, 'project')
        assert hasattr(Embedding, 'team')