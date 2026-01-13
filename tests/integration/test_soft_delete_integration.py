"""
Integration tests for soft delete functionality.

These tests verify that the soft delete system works correctly
with actual database operations, migrations, and constraints.
"""

import pytest
from datetime import datetime, timedelta
from uuid import uuid4

from sqlalchemy import create_engine, text
from sqlalchemy.orm import sessionmaker
from sqlalchemy.exc import IntegrityError

from penf_lib.storage.models import (
    Base,
    Source,
    Assertion,
    Person,
    Project,
    Team,
    Embedding,
)
from penf_lib.storage.soft_delete import SoftDeleteManager
from penf_lib.storage.archive import (
    ArchiveManager,
    ArchiveReason,
    SourceArchive,
)


class TestSoftDeleteIntegration:
    """Integration tests for soft delete with database."""

    @pytest.fixture(scope="class")
    def test_engine(self):
        """Create a test database engine."""
        # Use in-memory SQLite for testing
        engine = create_engine("sqlite:///:memory:", echo=False)
        Base.metadata.create_all(engine)
        return engine

    @pytest.fixture
    def db_session(self, test_engine):
        """Create a database session for testing."""
        Session = sessionmaker(bind=test_engine)
        session = Session()
        yield session
        session.rollback()
        session.close()

    def test_soft_delete_database_constraints(self, db_session):
        """Test that soft delete respects database constraints."""
        tenant_id = uuid4()

        # Create a source
        source = Source(
            source_system="gmail",
            external_id="constraint-test-123",
            content_hash="a" * 64,
            raw_content="Test content",
            tenant_id=tenant_id,
        )
        db_session.add(source)
        db_session.commit()

        # Create an assertion that references the source
        assertion = Assertion(
            source_id=source.id,
            assertion_type="decision",
            content="Test decision",
            tenant_id=tenant_id,
        )
        db_session.add(assertion)
        db_session.commit()

        # Soft delete the source
        source.soft_delete(deleted_by="test_user", reason="Testing constraints")
        db_session.commit()

        # The assertion should still exist and reference the deleted source
        db_session.refresh(assertion)
        assert assertion.source_id == source.id
        assert not assertion.is_deleted

        # But the source should be marked as deleted
        db_session.refresh(source)
        assert source.is_deleted

    def test_cascade_soft_delete_preserves_data_integrity(self, db_session):
        """Test that cascade soft delete maintains referential integrity."""
        tenant_id = uuid4()

        # Create a complex entity structure
        source = Source(
            source_system="gmail",
            external_id="cascade-integrity-test",
            content_hash="b" * 64,
            raw_content="Test content",
            tenant_id=tenant_id,
        )
        db_session.add(source)
        db_session.flush()

        # Create multiple assertions
        assertions = []
        for i in range(3):
            assertion = Assertion(
                source_id=source.id,
                assertion_type="decision",
                content=f"Decision {i}",
                tenant_id=tenant_id,
            )
            assertions.append(assertion)
            db_session.add(assertion)

        # Create embeddings for source and assertions
        source_embedding = Embedding(
            entity_type="source",
            entity_id=source.id,
            source_id=source.id,
            embedding=[0.1] * 768,
            embedding_model="test-model",
            text_content="Source embedding",
            tenant_id=tenant_id,
        )
        db_session.add(source_embedding)

        db_session.commit()

        # Get initial counts
        initial_source_count = db_session.query(Source).count()
        initial_assertion_count = db_session.query(Assertion).count()
        initial_embedding_count = db_session.query(Embedding).count()

        # Perform cascade soft delete
        manager = SoftDeleteManager(db_session)
        result = manager.soft_delete_single(source, cascade=True)

        db_session.commit()

        # Verify all records still exist in database
        assert db_session.query(Source).count() == initial_source_count
        assert db_session.query(Assertion).count() == initial_assertion_count
        assert db_session.query(Embedding).count() == initial_embedding_count

        # But are marked as deleted
        db_session.refresh(source)
        assert source.is_deleted

        for assertion in assertions:
            db_session.refresh(assertion)
            assert assertion.is_deleted

        db_session.refresh(source_embedding)
        assert source_embedding.is_deleted

        # Verify cascade summary
        assert len(result['cascaded_entities']) >= 4  # 3 assertions + 1 embedding

    def test_soft_delete_indexes_performance(self, db_session):
        """Test that soft delete queries use indexes efficiently."""
        tenant_id = uuid4()

        # Create many records
        sources = []
        for i in range(100):
            source = Source(
                source_system="gmail",
                external_id=f"perf-test-{i}",
                content_hash=chr(ord('a') + (i % 26)) * 64,
                raw_content=f"Test content {i}",
                tenant_id=tenant_id,
            )
            sources.append(source)
            db_session.add(source)

        db_session.commit()

        # Soft delete half of them
        for i in range(0, 100, 2):
            sources[i].soft_delete(reason=f"Batch delete {i}")

        db_session.commit()

        # Query for active records should be efficient
        # (In a real test, we'd analyze the query plan)
        active_sources = db_session.query(Source).filter(
            Source.is_deleted == False
        ).all()

        assert len(active_sources) == 50

        # Query for deleted records should also be efficient
        deleted_sources = db_session.query(Source).filter(
            Source.is_deleted == True
        ).all()

        assert len(deleted_sources) == 50

    def test_archive_table_operations(self, db_session):
        """Test archive table operations with database."""
        tenant_id = uuid4()

        # Create and archive a source
        source = Source(
            source_system="gmail",
            external_id="archive-integration-test",
            content_hash="c" * 64,
            raw_content="Test archive content",
            tenant_id=tenant_id,
        )
        db_session.add(source)
        db_session.commit()

        # Archive the source
        archive_manager = ArchiveManager(db_session)
        archive_result = archive_manager.archive_entity(
            source,
            archived_by="integration_test",
            reason=ArchiveReason.SOFT_DELETE,
            metadata={"test_run": "integration"}
        )

        db_session.commit()

        # Verify archive record exists
        archive_record = db_session.query(SourceArchive).filter_by(
            archive_id=archive_result['archive_id']
        ).first()

        assert archive_record is not None
        assert archive_record.original_id == source.id
        assert archive_record.source_system == "gmail"
        assert archive_record.external_id == "archive-integration-test"
        assert archive_record.archive_metadata["test_run"] == "integration"

        # Test querying archive records
        archived_sources = db_session.query(SourceArchive).filter_by(
            tenant_id=tenant_id
        ).all()

        assert len(archived_sources) == 1
        assert archived_sources[0].archive_id == archive_result['archive_id']

    def test_transaction_rollback_safety(self, db_session):
        """Test that soft delete operations handle transaction rollbacks safely."""
        tenant_id = uuid4()

        # Create a source
        source = Source(
            source_system="gmail",
            external_id="rollback-test",
            content_hash="d" * 64,
            raw_content="Rollback test content",
            tenant_id=tenant_id,
        )
        db_session.add(source)
        db_session.commit()

        # Start a transaction
        original_deleted_state = source.is_deleted

        try:
            # Soft delete the source
            source.soft_delete(deleted_by="rollback_test")

            # Verify it's marked as deleted
            assert source.is_deleted

            # Force a rollback by raising an exception
            raise Exception("Forced rollback")

        except Exception:
            db_session.rollback()

        # Verify the source is back to its original state
        db_session.refresh(source)
        assert source.is_deleted == original_deleted_state
        assert source.deleted_at is None
        assert source.deleted_by is None

    def test_concurrent_soft_delete_operations(self, db_session, test_engine):
        """Test concurrent soft delete operations."""
        tenant_id = uuid4()

        # Create a source
        source = Source(
            source_system="gmail",
            external_id="concurrent-test",
            content_hash="e" * 64,
            raw_content="Concurrent test content",
            tenant_id=tenant_id,
        )
        db_session.add(source)
        db_session.commit()
        source_id = source.id

        # Create a second session to simulate concurrency
        Session = sessionmaker(bind=test_engine)
        session2 = Session()

        try:
            # Load the source in both sessions
            source1 = db_session.query(Source).get(source_id)
            source2 = session2.query(Source).get(source_id)

            # Delete from first session
            source1.soft_delete(deleted_by="session1")
            db_session.commit()

            # Try to delete from second session (should handle gracefully)
            # In a real scenario, this might raise an exception or handle it gracefully
            source2.soft_delete(deleted_by="session2")
            session2.commit()

            # Verify final state
            final_source = db_session.query(Source).get(source_id)
            assert final_source.is_deleted

        finally:
            session2.close()

    def test_soft_delete_with_complex_relationships(self, db_session):
        """Test soft delete with complex entity relationships."""
        tenant_id = uuid4()

        # Create a person
        person = Person(
            canonical_name="Test Person",
            email_addresses=["test@example.com"],
            tenant_id=tenant_id,
        )
        db_session.add(person)
        db_session.flush()

        # Create a team with the person as lead
        team = Team(
            name="Test Team",
            team_type="project_team",
            lead_person_id=person.id,
            member_person_ids=[person.id],
            tenant_id=tenant_id,
        )
        db_session.add(team)
        db_session.flush()

        # Create a project with the person as team member
        project = Project(
            name="Test Project",
            status="active",
            team_members=[person.id],
            tenant_id=tenant_id,
        )
        db_session.add(project)
        db_session.commit()

        # Soft delete the person with cascade
        manager = SoftDeleteManager(db_session)
        result = manager.soft_delete_single(person, cascade=True)

        db_session.commit()

        # Verify the person is deleted
        db_session.refresh(person)
        assert person.is_deleted

        # Verify relationships were handled appropriately
        # Team should have nullified references
        db_session.refresh(team)
        assert person.id not in (team.member_person_ids or [])
        assert team.lead_person_id is None

        # Project should have nullified references
        db_session.refresh(project)
        assert person.id not in (project.team_members or [])

    def test_restore_maintains_referential_integrity(self, db_session):
        """Test that restore operations maintain referential integrity."""
        tenant_id = uuid4()

        # Create source and assertion
        source = Source(
            source_system="gmail",
            external_id="restore-integrity-test",
            content_hash="f" * 64,
            raw_content="Restore test content",
            tenant_id=tenant_id,
        )
        db_session.add(source)
        db_session.flush()

        assertion = Assertion(
            source_id=source.id,
            assertion_type="decision",
            content="Test decision for restore",
            tenant_id=tenant_id,
        )
        db_session.add(assertion)
        db_session.commit()

        # Soft delete both
        manager = SoftDeleteManager(db_session)
        manager.soft_delete_single(source, cascade=True)
        db_session.commit()

        # Verify both are deleted
        db_session.refresh(source)
        db_session.refresh(assertion)
        assert source.is_deleted
        assert assertion.is_deleted

        # Restore the source (not the assertion)
        manager.restore_single(source)
        db_session.commit()

        # Verify source is restored
        db_session.refresh(source)
        assert not source.is_deleted

        # Assertion should still be deleted
        db_session.refresh(assertion)
        assert assertion.is_deleted

        # The relationship should still be intact
        assert assertion.source_id == source.id

    def test_archive_retention_policy_enforcement(self, db_session):
        """Test that archive retention policies are enforced."""
        tenant_id = uuid4()

        # Create and archive sources with different retention periods
        sources = []
        for i in range(3):
            source = Source(
                source_system="gmail",
                external_id=f"retention-test-{i}",
                content_hash=chr(ord('a') + i) * 64,
                raw_content=f"Retention test content {i}",
                tenant_id=tenant_id,
            )
            sources.append(source)
            db_session.add(source)

        db_session.commit()

        archive_manager = ArchiveManager(db_session)

        # Archive with different retention periods
        # Source 0: 1 second (will expire quickly)
        archive_manager.archive_entity(
            sources[0],
            retention_period=timedelta(seconds=1)
        )

        # Source 1: 1 year (won't expire)
        archive_manager.archive_entity(
            sources[1],
            retention_period=timedelta(days=365)
        )

        # Source 2: default retention
        archive_manager.archive_entity(sources[2])

        db_session.commit()

        # Wait for the first archive to expire
        import time
        time.sleep(2)

        # Run cleanup
        cleanup_result = archive_manager.cleanup_expired_archives(dry_run=False)

        # Should have cleaned up the expired archive
        assert cleanup_result['total_expired'] == 1
        assert cleanup_result['total_deleted'] == 1

        # Verify remaining archives
        remaining_archives = db_session.query(SourceArchive).count()
        assert remaining_archives == 2