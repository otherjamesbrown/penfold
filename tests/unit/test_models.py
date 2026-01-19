"""Unit tests for SQLAlchemy model constraints and validation.

These tests focus on model constraints, field validation, and data integrity
for Database Schema User Story 1. Tests follow TDD principles and should
initially fail until models are fully implemented with proper validation.
"""

import pytest
import uuid
from datetime import datetime, timezone
from decimal import Decimal
from typing import Any, Dict

from sqlalchemy import inspect, select
from sqlalchemy.exc import IntegrityError, DataError, StatementError
from sqlalchemy.ext.asyncio import AsyncSession

from penf_lib.storage.models import (
    Base,
    TimestampMixin,
    TenantMixin,
    SoftDeleteMixin,
    Source,
    Assertion,
    Person,
    Project,
    Team,
    ProcessingEvent,
    ProcessingJob,
    ProcessingResult,
    Embedding,
)


class TestTimestampMixin:
    """Test TimestampMixin functionality."""

    def test_timestamp_mixin_fields_exist(self):
        """Test that TimestampMixin defines required fields."""
        # This test will initially pass as the fields are defined
        assert hasattr(TimestampMixin, 'created_at')
        assert hasattr(TimestampMixin, 'updated_at')

    def test_timestamp_fields_not_nullable(self):
        """Test that timestamp fields are properly configured as not nullable."""
        # Check that fields are configured correctly
        assert TimestampMixin.created_at.nullable is False
        assert TimestampMixin.updated_at.nullable is False

    @pytest.mark.asyncio
    async def test_timestamp_auto_population(self, test_session: AsyncSession, sample_tenant_id: str):
        """Test that timestamps are automatically populated on create/update."""
        # This test will fail initially until proper auto-timestamp functionality is implemented
        source = Source(
            tenant_id=sample_tenant_id,
            source_system="test",
            external_id="test123",
            content_hash="a" * 64,
        )

        test_session.add(source)
        await test_session.flush()

        # Should have created_at set
        assert source.created_at is not None
        assert isinstance(source.created_at, datetime)

        # Update and check updated_at changes
        original_created = source.created_at
        original_updated = source.updated_at

        source.source_system = "updated"
        await test_session.flush()

        # created_at should not change, updated_at should
        assert source.created_at == original_created
        assert source.updated_at > original_updated


class TestTenantMixin:
    """Test TenantMixin functionality."""

    def test_tenant_mixin_fields_exist(self):
        """Test that TenantMixin defines required fields."""
        assert hasattr(TenantMixin, 'tenant_id')

    def test_tenant_id_not_nullable(self):
        """Test that tenant_id is not nullable."""
        assert TenantMixin.tenant_id.nullable is False

    def test_tenant_id_has_index(self):
        """Test that tenant_id has an index."""
        assert TenantMixin.tenant_id.index is True

    @pytest.mark.asyncio
    async def test_tenant_id_required(self, test_session: AsyncSession):
        """Test that tenant_id is required for model creation."""
        # This test should fail initially until proper validation is implemented
        source = Source(
            source_system="test",
            external_id="test123",
            content_hash="a" * 64,
            # tenant_id intentionally omitted
        )

        test_session.add(source)

        with pytest.raises((IntegrityError, StatementError)):
            await test_session.flush()

    @pytest.mark.asyncio
    async def test_tenant_id_uuid_validation(self, test_session: AsyncSession):
        """Test that tenant_id accepts valid UUIDs."""
        valid_uuid = str(uuid.uuid4())

        source = Source(
            tenant_id=valid_uuid,
            source_system="test",
            external_id="test123",
            content_hash="a" * 64,
        )

        test_session.add(source)
        await test_session.flush()

        assert str(source.tenant_id) == valid_uuid


class TestSoftDeleteMixin:
    """Test SoftDeleteMixin functionality."""

    def test_soft_delete_mixin_fields_exist(self):
        """Test that SoftDeleteMixin defines required fields."""
        assert hasattr(SoftDeleteMixin, 'deleted_at')
        assert hasattr(SoftDeleteMixin, 'is_deleted')

    def test_is_deleted_default_false(self):
        """Test that is_deleted defaults to False."""
        assert SoftDeleteMixin.is_deleted.default.arg is False

    def test_is_deleted_not_nullable(self):
        """Test that is_deleted is not nullable."""
        assert SoftDeleteMixin.is_deleted.nullable is False

    def test_deleted_at_nullable(self):
        """Test that deleted_at is nullable."""
        assert SoftDeleteMixin.deleted_at.nullable is True

    @pytest.mark.asyncio
    async def test_soft_delete_defaults(self, test_session: AsyncSession, sample_tenant_id: str):
        """Test that soft delete fields have proper defaults."""
        source = Source(
            tenant_id=sample_tenant_id,
            source_system="test",
            external_id="test123",
            content_hash="a" * 64,
        )

        test_session.add(source)
        await test_session.flush()

        assert source.is_deleted is False
        assert source.deleted_at is None


class TestSourceModel:
    """Test Source model constraints and validation."""

    @pytest.mark.asyncio
    async def test_source_required_fields(self, test_session: AsyncSession, sample_tenant_id: str):
        """Test that Source model enforces required fields."""
        # Test missing source_system - should fail
        with pytest.raises((IntegrityError, StatementError)):
            source = Source(
                tenant_id=sample_tenant_id,
                external_id="test123",
                content_hash="a" * 64,
                # source_system missing
            )
            test_session.add(source)
            await test_session.flush()

        # Test missing external_id - should fail
        with pytest.raises((IntegrityError, StatementError)):
            source = Source(
                tenant_id=sample_tenant_id,
                source_system="test",
                content_hash="a" * 64,
                # external_id missing
            )
            test_session.add(source)
            await test_session.flush()

        # Test missing content_hash - should fail
        with pytest.raises((IntegrityError, StatementError)):
            source = Source(
                tenant_id=sample_tenant_id,
                source_system="test",
                external_id="test123",
                # content_hash missing
            )
            test_session.add(source)
            await test_session.flush()

    @pytest.mark.asyncio
    async def test_source_field_lengths(self, test_session: AsyncSession, sample_tenant_id: str):
        """Test Source model field length constraints."""
        # Test source_system length limit (50 chars)
        with pytest.raises((DataError, StatementError)):
            source = Source(
                tenant_id=sample_tenant_id,
                source_system="a" * 51,  # Too long
                external_id="test123",
                content_hash="a" * 64,
            )
            test_session.add(source)
            await test_session.flush()

        # Test external_id length limit (255 chars)
        with pytest.raises((DataError, StatementError)):
            source = Source(
                tenant_id=sample_tenant_id,
                source_system="test",
                external_id="a" * 256,  # Too long
                content_hash="a" * 64,
            )
            test_session.add(source)
            await test_session.flush()

        # Test content_hash length limit (64 chars)
        with pytest.raises((DataError, StatementError)):
            source = Source(
                tenant_id=sample_tenant_id,
                source_system="test",
                external_id="test123",
                content_hash="a" * 65,  # Too long
            )
            test_session.add(source)
            await test_session.flush()

    @pytest.mark.asyncio
    async def test_source_processing_status_default(self, test_session: AsyncSession, sample_tenant_id: str):
        """Test Source model processing_status default value."""
        source = Source(
            tenant_id=sample_tenant_id,
            source_system="test",
            external_id="test123",
            content_hash="a" * 64,
        )

        test_session.add(source)
        await test_session.flush()

        assert source.processing_status == "pending"

    @pytest.mark.asyncio
    async def test_source_unique_constraint(self, test_session: AsyncSession, sample_tenant_id: str):
        """Test Source unique constraint on (tenant_id, source_system, external_id)."""
        # Create first source
        source1 = Source(
            tenant_id=sample_tenant_id,
            source_system="gmail",
            external_id="msg_123",
            content_hash="a" * 64,
        )
        test_session.add(source1)
        await test_session.flush()

        # Try to create duplicate - should fail
        with pytest.raises(IntegrityError):
            source2 = Source(
                tenant_id=sample_tenant_id,
                source_system="gmail",
                external_id="msg_123",  # Same as above
                content_hash="b" * 64,  # Different hash
            )
            test_session.add(source2)
            await test_session.flush()

    @pytest.mark.asyncio
    async def test_source_ingestion_metadata_default(self, test_session: AsyncSession, sample_tenant_id: str):
        """Test Source model ingestion_metadata default value."""
        source = Source(
            tenant_id=sample_tenant_id,
            source_system="test",
            external_id="test123",
            content_hash="a" * 64,
        )

        test_session.add(source)
        await test_session.flush()

        assert source.ingestion_metadata == {}


class TestAssertionModel:
    """Test Assertion model constraints and validation."""

    @pytest.mark.asyncio
    async def test_assertion_required_fields(self, test_session: AsyncSession, test_source):
        """Test that Assertion model enforces required fields."""
        # Test missing source_id - should fail
        with pytest.raises((IntegrityError, StatementError)):
            assertion = Assertion(
                tenant_id=test_source.tenant_id,
                assertion_type="decision",
                content="Test decision",
                # source_id missing
            )
            test_session.add(assertion)
            await test_session.flush()

        # Test missing assertion_type - should fail
        with pytest.raises((IntegrityError, StatementError)):
            assertion = Assertion(
                tenant_id=test_source.tenant_id,
                source_id=test_source.id,
                content="Test decision",
                # assertion_type missing
            )
            test_session.add(assertion)
            await test_session.flush()

        # Test missing content - should fail
        with pytest.raises((IntegrityError, StatementError)):
            assertion = Assertion(
                tenant_id=test_source.tenant_id,
                source_id=test_source.id,
                assertion_type="decision",
                # content missing
            )
            test_session.add(assertion)
            await test_session.flush()

    @pytest.mark.asyncio
    async def test_assertion_field_lengths(self, test_session: AsyncSession, test_source):
        """Test Assertion model field length constraints."""
        # Test assertion_type length limit (50 chars)
        with pytest.raises((DataError, StatementError)):
            assertion = Assertion(
                tenant_id=test_source.tenant_id,
                source_id=test_source.id,
                assertion_type="a" * 51,  # Too long
                content="Test content",
            )
            test_session.add(assertion)
            await test_session.flush()

    @pytest.mark.asyncio
    async def test_assertion_confidence_score_validation(self, test_session: AsyncSession, test_source):
        """Test Assertion confidence_score field validation."""
        # Test valid confidence score (0.000-1.000)
        assertion = Assertion(
            tenant_id=test_source.tenant_id,
            source_id=test_source.id,
            assertion_type="decision",
            content="Test decision",
            confidence_score=Decimal("0.850"),
        )
        test_session.add(assertion)
        await test_session.flush()

        assert assertion.confidence_score == Decimal("0.850")

        # Test confidence score precision (should handle 3 decimal places)
        assertion2 = Assertion(
            tenant_id=test_source.tenant_id,
            source_id=test_source.id,
            assertion_type="risk",
            content="Test risk",
            confidence_score=Decimal("0.999"),
        )
        test_session.add(assertion2)
        await test_session.flush()

    @pytest.mark.asyncio
    async def test_assertion_default_values(self, test_session: AsyncSession, test_source):
        """Test Assertion model default values."""
        assertion = Assertion(
            tenant_id=test_source.tenant_id,
            source_id=test_source.id,
            assertion_type="decision",
            content="Test decision",
        )

        test_session.add(assertion)
        await test_session.flush()

        # Check defaults
        assert assertion.processing_metadata == {}
        assert assertion.related_entities == {}
        assert assertion.validation_feedback == {}
        assert assertion.user_validated is False

    @pytest.mark.asyncio
    async def test_assertion_foreign_key_constraint(self, test_session: AsyncSession, sample_tenant_id: str):
        """Test Assertion foreign key constraint to Source."""
        # Try to create assertion with invalid source_id - should fail
        with pytest.raises(IntegrityError):
            assertion = Assertion(
                tenant_id=sample_tenant_id,
                source_id=999999,  # Non-existent source
                assertion_type="decision",
                content="Test decision",
            )
            test_session.add(assertion)
            await test_session.flush()


class TestPersonModel:
    """Test Person model constraints and validation."""

    @pytest.mark.asyncio
    async def test_person_required_fields(self, test_session: AsyncSession, sample_tenant_id: str):
        """Test that Person model enforces required fields."""
        # Test missing canonical_name - should fail
        with pytest.raises((IntegrityError, StatementError)):
            person = Person(
                tenant_id=sample_tenant_id,
                # canonical_name missing
            )
            test_session.add(person)
            await test_session.flush()

    @pytest.mark.asyncio
    async def test_person_field_lengths(self, test_session: AsyncSession, sample_tenant_id: str):
        """Test Person model field length constraints."""
        # Test canonical_name length limit (200 chars)
        with pytest.raises((DataError, StatementError)):
            person = Person(
                tenant_id=sample_tenant_id,
                canonical_name="a" * 201,  # Too long
            )
            test_session.add(person)
            await test_session.flush()

        # Test job_title length limit (200 chars)
        with pytest.raises((DataError, StatementError)):
            person = Person(
                tenant_id=sample_tenant_id,
                canonical_name="John Doe",
                job_title="a" * 201,  # Too long
            )
            test_session.add(person)
            await test_session.flush()

    @pytest.mark.asyncio
    async def test_person_array_fields_default(self, test_session: AsyncSession, sample_tenant_id: str):
        """Test Person model array field defaults."""
        person = Person(
            tenant_id=sample_tenant_id,
            canonical_name="John Doe",
        )

        test_session.add(person)
        await test_session.flush()

        assert person.aliases == []

    @pytest.mark.asyncio
    async def test_person_confidence_score_validation(self, test_session: AsyncSession, sample_tenant_id: str):
        """Test Person confidence_score field validation."""
        person = Person(
            tenant_id=sample_tenant_id,
            canonical_name="John Doe",
            confidence_score=Decimal("0.950"),
        )

        test_session.add(person)
        await test_session.flush()

        assert person.confidence_score == Decimal("0.950")

    @pytest.mark.asyncio
    async def test_person_entity_resolution_metadata_default(self, test_session: AsyncSession, sample_tenant_id: str):
        """Test Person model entity_resolution_metadata default."""
        person = Person(
            tenant_id=sample_tenant_id,
            canonical_name="John Doe",
        )

        test_session.add(person)
        await test_session.flush()

        assert person.entity_resolution_metadata == {}


class TestProjectModel:
    """Test Project model constraints and validation."""

    @pytest.mark.asyncio
    async def test_project_required_fields(self, test_session: AsyncSession, sample_tenant_id: str):
        """Test that Project model enforces required fields."""
        # Test missing name - should fail
        with pytest.raises((IntegrityError, StatementError)):
            project = Project(
                tenant_id=sample_tenant_id,
                # name missing
            )
            test_session.add(project)
            await test_session.flush()

    @pytest.mark.asyncio
    async def test_project_field_lengths(self, test_session: AsyncSession, sample_tenant_id: str):
        """Test Project model field length constraints."""
        # Test name length limit (200 chars)
        with pytest.raises((DataError, StatementError)):
            project = Project(
                tenant_id=sample_tenant_id,
                name="a" * 201,  # Too long
            )
            test_session.add(project)
            await test_session.flush()

    @pytest.mark.asyncio
    async def test_project_default_values(self, test_session: AsyncSession, sample_tenant_id: str):
        """Test Project model default values."""
        project = Project(
            tenant_id=sample_tenant_id,
            name="Test Project",
        )

        test_session.add(project)
        await test_session.flush()

        # Check defaults
        assert project.status == "active"
        assert project.artifacts == []
        assert project.milestones == []

    @pytest.mark.asyncio
    async def test_project_hierarchy_constraint(self, test_session: AsyncSession, sample_tenant_id: str):
        """Test Project parent_id foreign key constraint."""
        # Create parent project
        parent = Project(
            tenant_id=sample_tenant_id,
            name="Parent Project",
        )
        test_session.add(parent)
        await test_session.flush()

        # Create child project
        child = Project(
            tenant_id=sample_tenant_id,
            name="Child Project",
            parent_id=parent.id,
        )
        test_session.add(child)
        await test_session.flush()

        assert child.parent_id == parent.id

        # Try invalid parent_id - should fail
        with pytest.raises(IntegrityError):
            invalid_child = Project(
                tenant_id=sample_tenant_id,
                name="Invalid Child",
                parent_id=999999,  # Non-existent parent
            )
            test_session.add(invalid_child)
            await test_session.flush()

    @pytest.mark.asyncio
    async def test_project_decimal_fields(self, test_session: AsyncSession, sample_tenant_id: str):
        """Test Project decimal field validation."""
        project = Project(
            tenant_id=sample_tenant_id,
            name="Budget Project",
            budget_allocated=Decimal("50000.00"),
            budget_spent=Decimal("25000.50"),
        )

        test_session.add(project)
        await test_session.flush()

        assert project.budget_allocated == Decimal("50000.00")
        assert project.budget_spent == Decimal("25000.50")


class TestTeamModel:
    """Test Team model constraints and validation."""

    @pytest.mark.asyncio
    async def test_team_required_fields(self, test_session: AsyncSession, sample_tenant_id: str):
        """Test that Team model enforces required fields."""
        # Test missing name - should fail
        with pytest.raises((IntegrityError, StatementError)):
            team = Team(
                tenant_id=sample_tenant_id,
                # name missing
            )
            test_session.add(team)
            await test_session.flush()

    @pytest.mark.asyncio
    async def test_team_field_lengths(self, test_session: AsyncSession, sample_tenant_id: str):
        """Test Team model field length constraints."""
        # Test name length limit (200 chars)
        with pytest.raises((DataError, StatementError)):
            team = Team(
                tenant_id=sample_tenant_id,
                name="a" * 201,  # Too long
            )
            test_session.add(team)
            await test_session.flush()

    @pytest.mark.asyncio
    async def test_team_hierarchy_constraint(self, test_session: AsyncSession, sample_tenant_id: str):
        """Test Team parent_team_id foreign key constraint."""
        # Create parent team
        parent = Team(
            tenant_id=sample_tenant_id,
            name="Parent Team",
        )
        test_session.add(parent)
        await test_session.flush()

        # Create child team
        child = Team(
            tenant_id=sample_tenant_id,
            name="Child Team",
            parent_team_id=parent.id,
        )
        test_session.add(child)
        await test_session.flush()

        assert child.parent_team_id == parent.id

        # Try invalid parent_team_id - should fail
        with pytest.raises(IntegrityError):
            invalid_child = Team(
                tenant_id=sample_tenant_id,
                name="Invalid Child Team",
                parent_team_id=999999,  # Non-existent parent
            )
            test_session.add(invalid_child)
            await test_session.flush()


class TestModelRelationships:
    """Test model relationships and foreign key constraints."""

    @pytest.mark.asyncio
    async def test_source_assertion_relationship(self, test_session: AsyncSession, test_source):
        """Test Source-Assertion relationship."""
        # Create assertion linked to source
        assertion = Assertion(
            tenant_id=test_source.tenant_id,
            source_id=test_source.id,
            assertion_type="decision",
            content="Test decision from source",
        )
        test_session.add(assertion)
        await test_session.flush()

        # Test relationship navigation
        await test_session.refresh(test_source, ['assertions'])
        assert len(test_source.assertions) == 1
        assert test_source.assertions[0].content == "Test decision from source"

        # Test back-reference
        assert assertion.source.id == test_source.id

    @pytest.mark.asyncio
    async def test_project_hierarchy_relationship(self, test_session: AsyncSession, sample_tenant_id: str):
        """Test Project hierarchical relationships."""
        # Create parent project
        parent = Project(
            tenant_id=sample_tenant_id,
            name="Parent Project",
        )
        test_session.add(parent)
        await test_session.flush()

        # Create child projects
        child1 = Project(
            tenant_id=sample_tenant_id,
            name="Child Project 1",
            parent_id=parent.id,
        )
        child2 = Project(
            tenant_id=sample_tenant_id,
            name="Child Project 2",
            parent_id=parent.id,
        )
        test_session.add_all([child1, child2])
        await test_session.flush()

        # Test parent->children relationship
        await test_session.refresh(parent, ['children'])
        assert len(parent.children) == 2
        assert {child.name for child in parent.children} == {"Child Project 1", "Child Project 2"}

        # Test child->parent relationship
        assert child1.parent_id == parent.id
        assert child2.parent_id == parent.id


class TestModelIndexes:
    """Test that models have proper database indexes."""

    def test_source_indexes_exist(self):
        """Test Source model has required indexes."""
        source_table = Source.__table__
        index_names = {idx.name for idx in source_table.indexes}

        # Check for key indexes from model definition
        assert "idx_sources_tenant_created" in index_names
        assert "idx_sources_unique_external" in index_names
        assert "idx_sources_hash" in index_names
        assert "idx_sources_status" in index_names
        assert "idx_sources_soft_delete" in index_names

    def test_assertion_indexes_exist(self):
        """Test Assertion model has required indexes."""
        assertion_table = Assertion.__table__
        index_names = {idx.name for idx in assertion_table.indexes}

        # Check for key indexes from model definition
        assert "idx_assertions_tenant_type" in index_names
        assert "idx_assertions_confidence" in index_names
        assert "idx_assertions_source" in index_names
        assert "idx_assertions_validated" in index_names
        assert "idx_assertions_soft_delete" in index_names

    def test_person_indexes_exist(self):
        """Test Person model has required indexes."""
        person_table = Person.__table__
        index_names = {idx.name for idx in person_table.indexes}

        assert "idx_people_tenant_name" in index_names
        assert "idx_people_aliases" in index_names
        assert "idx_people_emails" in index_names
        assert "idx_people_soft_delete" in index_names

    def test_project_indexes_exist(self):
        """Test Project model has required indexes."""
        project_table = Project.__table__
        index_names = {idx.name for idx in project_table.indexes}

        assert "idx_projects_tenant_name" in index_names
        assert "idx_projects_status" in index_names
        assert "idx_projects_hierarchy" in index_names
        assert "idx_projects_timeline" in index_names
        assert "idx_projects_soft_delete" in index_names

    def test_team_indexes_exist(self):
        """Test Team model has required indexes."""
        team_table = Team.__table__
        index_names = {idx.name for idx in team_table.indexes}

        assert "idx_teams_tenant_name" in index_names
        assert "idx_teams_type" in index_names
        assert "idx_teams_lead" in index_names
        assert "idx_teams_members" in index_names
        assert "idx_teams_soft_delete" in index_names


class TestModelColumnTypes:
    """Test model column types and configurations."""

    def test_source_column_types(self):
        """Test Source model column types are correctly defined."""
        source_table = Source.__table__
        columns = {col.name: col for col in source_table.columns}

        # Test primary key
        assert columns['id'].primary_key is True
        assert str(columns['id'].type) == "BIGINT"

        # Test required string fields and their lengths
        assert columns['source_system'].nullable is False
        assert columns['source_system'].type.length == 50

        assert columns['external_id'].nullable is False
        assert columns['external_id'].type.length == 255

        assert columns['content_hash'].nullable is False
        assert columns['content_hash'].type.length == 64

        # Test optional fields
        assert columns['raw_content'].nullable is True
        assert columns['content_type'].nullable is True
        assert columns['content_size'].nullable is True

        # Test JSON fields
        assert str(columns['ingestion_metadata'].type) == "JSONB"

        # Test enum-like field
        assert columns['processing_status'].type.length == 20
        assert columns['processing_status'].nullable is False

    def test_assertion_column_types(self):
        """Test Assertion model column types are correctly defined."""
        assertion_table = Assertion.__table__
        columns = {col.name: col for col in assertion_table.columns}

        # Test foreign key
        assert columns['source_id'].nullable is False
        assert str(columns['source_id'].type) == "BIGINT"

        # Test required fields
        assert columns['assertion_type'].nullable is False
        assert columns['assertion_type'].type.length == 50
        assert columns['content'].nullable is False

        # Test decimal field
        assert str(columns['confidence_score'].type) == "DECIMAL(4, 3)"
        assert columns['confidence_score'].nullable is True

        # Test array field
        assert "ARRAY" in str(columns['tags'].type)

        # Test boolean field
        assert columns['user_validated'].nullable is True
        assert str(columns['user_validated'].type) == "BOOLEAN"

    def test_person_column_types(self):
        """Test Person model column types are correctly defined."""
        person_table = Person.__table__
        columns = {col.name: col for col in person_table.columns}

        # Test required field
        assert columns['canonical_name'].nullable is False
        assert columns['canonical_name'].type.length == 200

        # Test array fields
        assert "ARRAY" in str(columns['aliases'].type)
        assert "ARRAY" in str(columns['email_addresses'].type)

        # Test optional string fields with length limits
        assert columns['job_title'].nullable is True
        assert columns['job_title'].type.length == 200

    def test_project_column_types(self):
        """Test Project model column types are correctly defined."""
        project_table = Project.__table__
        columns = {col.name: col for col in project_table.columns}

        # Test required field
        assert columns['name'].nullable is False
        assert columns['name'].type.length == 200

        # Test self-referencing foreign key
        assert columns['parent_id'].nullable is True
        assert str(columns['parent_id'].type) == "BIGINT"

        # Test decimal fields
        assert str(columns['budget_allocated'].type) == "DECIMAL(12, 2)"
        assert str(columns['budget_spent'].type) == "DECIMAL(12, 2)"

        # Test JSON fields
        assert str(columns['artifacts'].type) == "JSONB"
        assert str(columns['milestones'].type) == "JSONB"

    def test_team_column_types(self):
        """Test Team model column types are correctly defined."""
        team_table = Team.__table__
        columns = {col.name: col for col in team_table.columns}

        # Test required field
        assert columns['name'].nullable is False
        assert columns['name'].type.length == 200

        # Test self-referencing foreign key
        assert columns['parent_team_id'].nullable is True
        assert str(columns['parent_team_id'].type) == "BIGINT"

        # Test array fields
        assert "ARRAY" in str(columns['member_person_ids'].type)

    def test_processing_event_column_types(self):
        """Test ProcessingEvent model column types are correctly defined."""
        event_table = ProcessingEvent.__table__
        columns = {col.name: col for col in event_table.columns}

        # Test UUID fields
        assert str(columns['event_id'].type) == "UUID"
        assert columns['event_id'].unique is True

        # Test required fields
        assert columns['event_type'].nullable is False
        assert columns['payload'].nullable is False

        # Test datetime with timezone
        assert columns['published_at'].nullable is False
        assert columns['expires_at'].nullable is False

    def test_processing_job_column_types(self):
        """Test ProcessingJob model column types are correctly defined."""
        job_table = ProcessingJob.__table__
        columns = {col.name: col for col in job_table.columns}

        # Test UUID fields
        assert str(columns['job_id'].type) == "UUID"
        assert columns['job_id'].unique is True

        # Test foreign key to event
        assert str(columns['event_id'].type) == "UUID"
        assert columns['event_id'].nullable is False

        # Test integer fields
        assert str(columns['retry_count'].type) == "INTEGER"
        assert str(columns['max_retries'].type) == "INTEGER"
        assert str(columns['priority'].type) == "INTEGER"

    def test_embedding_column_types(self):
        """Test Embedding model column types are correctly defined."""
        embedding_table = Embedding.__table__
        columns = {col.name: col for col in embedding_table.columns}

        # Test polymorphic fields
        assert columns['entity_type'].nullable is False
        assert columns['entity_id'].nullable is False

        # Test vector field (either Vector or ARRAY(Float))
        vector_type_str = str(columns['embedding'].type)
        assert "vector" in vector_type_str.lower() or "ARRAY" in vector_type_str

        # Test content fields
        assert columns['embedding_model'].nullable is False


class TestModelConstraintDefinitions:
    """Test that model constraints are properly defined in Python."""

    def test_source_unique_constraint_definition(self):
        """Test that Source unique constraint is properly defined."""
        source_table = Source.__table__

        # Find the unique constraint
        unique_indexes = [idx for idx in source_table.indexes if idx.unique]
        constraint_names = [idx.name for idx in unique_indexes]

        assert "idx_sources_unique_external" in constraint_names

        # Get the specific constraint
        unique_constraint = next(idx for idx in unique_indexes
                               if idx.name == "idx_sources_unique_external")

        # Check columns are correct
        column_names = [col.name for col in unique_constraint.columns]
        assert "tenant_id" in column_names
        assert "source_system" in column_names
        assert "external_id" in column_names

    def test_foreign_key_constraints_definition(self):
        """Test that foreign key constraints are properly defined."""
        # Test Assertion -> Source FK
        assertion_table = Assertion.__table__
        source_id_column = assertion_table.columns['source_id']
        assert len(source_id_column.foreign_keys) == 1

        fk = list(source_id_column.foreign_keys)[0]
        assert str(fk.column) == "sources.id"

        # Test Project self-referential FK
        project_table = Project.__table__
        parent_id_column = project_table.columns['parent_id']
        assert len(parent_id_column.foreign_keys) == 1

        parent_fk = list(parent_id_column.foreign_keys)[0]
        assert str(parent_fk.column) == "projects.id"

        # Test Team self-referential FK
        team_table = Team.__table__
        parent_team_id_column = team_table.columns['parent_team_id']
        assert len(parent_team_id_column.foreign_keys) == 1

        team_fk = list(parent_team_id_column.foreign_keys)[0]
        assert str(team_fk.column) == "teams.id"

    def test_index_configurations(self):
        """Test that indexes are defined with proper names (specific types verified at DB level)."""
        # Test that array field indexes exist (type verification requires DB)
        person_table = Person.__table__
        index_names = [idx.name for idx in person_table.indexes]

        assert "idx_people_aliases" in index_names
        assert "idx_people_emails" in index_names

        # Test hierarchical index exists (type verification requires DB)
        project_table = Project.__table__
        project_index_names = [idx.name for idx in project_table.indexes]
        assert "idx_projects_hierarchy" in project_index_names

        # Test vector index exists in embedding table (when pgvector available)
        embedding_table = Embedding.__table__
        embedding_index_names = [idx.name for idx in embedding_table.indexes if idx is not None]

        # Vector index may not be defined if pgvector not available
        if any("vector" in name for name in embedding_index_names):
            assert "idx_embeddings_vector_hnsw" in embedding_index_names

    def test_default_value_definitions(self):
        """Test that default values are properly defined."""
        # Test Source defaults
        source_table = Source.__table__
        processing_status_col = source_table.columns['processing_status']
        assert processing_status_col.default is not None
        assert processing_status_col.default.arg == "pending"

        # Test SoftDeleteMixin defaults
        is_deleted_col = source_table.columns['is_deleted']
        assert is_deleted_col.default is not None
        assert is_deleted_col.default.arg is False

        # Test JSON field defaults
        ingestion_metadata_col = source_table.columns['ingestion_metadata']
        assert ingestion_metadata_col.default is not None
        assert ingestion_metadata_col.default.arg == {}


class TestModelValidationEdgeCases:
    """Test edge cases and boundary conditions for model validation."""

    @pytest.mark.asyncio
    async def test_empty_string_validation(self, test_session: AsyncSession, sample_tenant_id: str):
        """Test that empty strings are handled properly for required fields."""
        # Empty source_system should fail
        with pytest.raises((IntegrityError, StatementError)):
            source = Source(
                tenant_id=sample_tenant_id,
                source_system="",  # Empty string
                external_id="test123",
                content_hash="a" * 64,
            )
            test_session.add(source)
            await test_session.flush()

    @pytest.mark.asyncio
    async def test_null_vs_empty_json_fields(self, test_session: AsyncSession, sample_tenant_id: str):
        """Test handling of null vs empty JSON fields."""
        source = Source(
            tenant_id=sample_tenant_id,
            source_system="test",
            external_id="test123",
            content_hash="a" * 64,
            ingestion_metadata=None,  # Explicitly set to None
        )

        test_session.add(source)
        await test_session.flush()

        # Should default to empty dict, not None
        assert source.ingestion_metadata == {}

    @pytest.mark.asyncio
    async def test_decimal_precision_limits(self, test_session: AsyncSession, test_source):
        """Test decimal field precision and scale limits."""
        # Test confidence score with exact precision (4,3) - should work
        assertion = Assertion(
            tenant_id=test_source.tenant_id,
            source_id=test_source.id,
            assertion_type="decision",
            content="Test decision",
            confidence_score=Decimal("1.000"),  # Maximum valid value
        )
        test_session.add(assertion)
        await test_session.flush()

        assert assertion.confidence_score == Decimal("1.000")

        # Test confidence score with too much precision - behavior depends on DB
        assertion2 = Assertion(
            tenant_id=test_source.tenant_id,
            source_id=test_source.id,
            assertion_type="risk",
            content="Test risk",
            confidence_score=Decimal("0.9999"),  # 4 decimal places
        )
        test_session.add(assertion2)
        await test_session.flush()

        # Should be rounded to 3 decimal places
        assert assertion2.confidence_score == Decimal("1.000")

    @pytest.mark.asyncio
    async def test_array_field_constraints(self, test_session: AsyncSession, sample_tenant_id: str):
        """Test array field handling and constraints."""
        person = Person(
            tenant_id=sample_tenant_id,
            canonical_name="Test Person",
            aliases=["Alias 1", "Alias 2", "Alias 3"],
            email_addresses=["test1@example.com", "test2@example.com"],
        )

        test_session.add(person)
        await test_session.flush()

        assert len(person.aliases) == 3
        assert len(person.email_addresses) == 2
        assert "Alias 1" in person.aliases
        assert "test1@example.com" in person.email_addresses

    @pytest.mark.asyncio
    async def test_cascade_delete_behavior(self, test_session: AsyncSession, test_source):
        """Test cascade delete behavior for related entities."""
        # Create assertion linked to source
        assertion = Assertion(
            tenant_id=test_source.tenant_id,
            source_id=test_source.id,
            assertion_type="decision",
            content="Test decision",
        )
        test_session.add(assertion)
        await test_session.flush()

        assertion_id = assertion.id

        # Delete source - assertion should remain (no cascade delete by default)
        await test_session.delete(test_source)

        # This should fail due to foreign key constraint
        with pytest.raises(IntegrityError):
            await test_session.flush()


class TestModelTableStructure:
    """Test that model table structures match specifications."""

    def test_source_table_name(self):
        """Test Source model has correct table name."""
        assert Source.__tablename__ == "sources"

    def test_assertion_table_name(self):
        """Test Assertion model has correct table name."""
        assert Assertion.__tablename__ == "assertions"

    def test_person_table_name(self):
        """Test Person model has correct table name."""
        assert Person.__tablename__ == "people"

    def test_project_table_name(self):
        """Test Project model has correct table name."""
        assert Project.__tablename__ == "projects"

    def test_team_table_name(self):
        """Test Team model has correct table name."""
        assert Team.__tablename__ == "teams"

    def test_all_models_inherit_base(self):
        """Test that all models inherit from Base."""
        models = [Source, Assertion, Person, Project, Team, ProcessingEvent, ProcessingJob, ProcessingResult, Embedding]

        for model in models:
            assert issubclass(model, Base)

    def test_all_main_models_use_mixins(self):
        """Test that main models use all required mixins."""
        main_models = [Source, Assertion, Person, Project, Team]

        for model in main_models:
            assert issubclass(model, TimestampMixin)
            assert issubclass(model, TenantMixin)
            assert issubclass(model, SoftDeleteMixin)

    def test_processing_models_use_tenant_timestamp_only(self):
        """Test that processing models only use TimestampMixin and TenantMixin."""
        processing_models = [ProcessingEvent, ProcessingJob, ProcessingResult]

        for model in processing_models:
            assert issubclass(model, TimestampMixin)
            assert issubclass(model, TenantMixin)
            # Should NOT inherit SoftDeleteMixin (they have retention periods instead)
            assert not issubclass(model, SoftDeleteMixin)

    def test_embedding_model_mixins(self):
        """Test that Embedding model uses correct mixins."""
        assert issubclass(Embedding, TimestampMixin)
        assert issubclass(Embedding, TenantMixin)
        # Embeddings don't use soft delete (they have retention logic)
        assert not issubclass(Embedding, SoftDeleteMixin)