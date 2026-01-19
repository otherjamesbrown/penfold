"""
Archive table utilities for long-term retention and audit trails.

This module provides archive table functionality for:
- Long-term storage of deleted records
- Audit trails and compliance
- Data recovery operations
- Historical data analysis
"""

import logging
from typing import Dict, List, Optional, Any, Type
from datetime import datetime, timedelta
from enum import Enum

from sqlalchemy import (
    Column,
    Integer,
    String,
    Text,
    DateTime,
    Boolean,
    ForeignKey,
    Index,
    event,
    DECIMAL,
)
from sqlalchemy.dialects.postgresql import UUID, JSONB, BIGINT
from sqlalchemy.ext.declarative import declarative_base
from sqlalchemy.orm import Session, relationship
from sqlalchemy.sql import func

from .models import (
    Base,
    TimestampMixin,
    TenantMixin,
    Source,
    Assertion,
    Person,
    Project,
    Team,
)

logger = logging.getLogger(__name__)


class ArchiveReason(Enum):
    """Reasons for archiving records."""

    SOFT_DELETE = "soft_delete"
    RETENTION_POLICY = "retention_policy"
    COMPLIANCE = "compliance"
    MANUAL_ARCHIVE = "manual_archive"
    DATA_MIGRATION = "data_migration"


class ArchiveBase(Base):
    """Base class for all archive tables."""

    __abstract__ = True

    # Archive metadata
    archive_id = Column(BIGINT, primary_key=True)
    original_id = Column(BIGINT, nullable=False, comment="Original record ID")
    archived_at = Column(
        DateTime(timezone=True),
        server_default=func.now(),
        nullable=False,
        comment="When this record was archived",
    )
    archived_by = Column(
        String(100),
        nullable=False,
        default="system",
        comment="User or system that archived this record",
    )
    archive_reason = Column(
        String(50),
        nullable=False,
        comment="Reason for archiving (soft_delete, retention_policy, etc.)",
    )

    # Original record state
    record_snapshot = Column(
        JSONB,
        nullable=False,
        comment="Complete snapshot of the original record",
    )

    # Metadata about the archival process
    archive_metadata = Column(
        JSONB,
        default={},
        comment="Additional metadata about the archival process",
    )

    # Retention management
    retention_expires_at = Column(
        DateTime(timezone=True),
        comment="When this archive record can be permanently deleted",
    )

    # Tenant isolation for multi-tenant environments
    tenant_id = Column(
        UUID(as_uuid=True),
        nullable=False,
        index=True,
        comment="Tenant ID for Row-Level Security",
    )


class SourceArchive(ArchiveBase):
    """Archive table for Source entities."""

    __tablename__ = "source_archive"

    # Key fields from original for querying
    source_system = Column(String(50), comment="Original source system")
    external_id = Column(String(255), comment="Original external ID")
    content_hash = Column(String(64), comment="Original content hash")
    source_timestamp = Column(
        DateTime(timezone=True),
        comment="Original source timestamp",
    )

    __table_args__ = (
        Index("idx_source_archive_tenant", "tenant_id", "archived_at"),
        Index("idx_source_archive_original_id", "original_id"),
        Index("idx_source_archive_system", "source_system", "external_id"),
        Index("idx_source_archive_retention", "retention_expires_at"),
    )


class AssertionArchive(ArchiveBase):
    """Archive table for Assertion entities."""

    __tablename__ = "assertion_archive"

    # Key fields from original for querying
    original_source_id = Column(BIGINT, comment="Original source ID")
    assertion_type = Column(String(50), comment="Original assertion type")
    confidence_score = Column(DECIMAL(4, 3), comment="Original confidence score")

    __table_args__ = (
        Index("idx_assertion_archive_tenant", "tenant_id", "archived_at"),
        Index("idx_assertion_archive_original_id", "original_id"),
        Index("idx_assertion_archive_source", "original_source_id"),
        Index("idx_assertion_archive_type", "assertion_type"),
        Index("idx_assertion_archive_retention", "retention_expires_at"),
    )


class PersonArchive(ArchiveBase):
    """Archive table for Person entities."""

    __tablename__ = "person_archive"

    # Key fields from original for querying
    canonical_name = Column(String(200), comment="Original canonical name")
    company = Column(String(200), comment="Original company")

    __table_args__ = (
        Index("idx_person_archive_tenant", "tenant_id", "archived_at"),
        Index("idx_person_archive_original_id", "original_id"),
        Index("idx_person_archive_name", "canonical_name"),
        Index("idx_person_archive_company", "company"),
        Index("idx_person_archive_retention", "retention_expires_at"),
    )


class ProjectArchive(ArchiveBase):
    """Archive table for Project entities."""

    __tablename__ = "project_archive"

    # Key fields from original for querying
    name = Column(String(200), comment="Original project name")
    status = Column(String(50), comment="Original project status")
    start_date = Column(DateTime(timezone=True), comment="Original start date")
    end_date = Column(DateTime(timezone=True), comment="Original end date")

    __table_args__ = (
        Index("idx_project_archive_tenant", "tenant_id", "archived_at"),
        Index("idx_project_archive_original_id", "original_id"),
        Index("idx_project_archive_name", "name"),
        Index("idx_project_archive_status", "status"),
        Index("idx_project_archive_timeline", "start_date", "end_date"),
        Index("idx_project_archive_retention", "retention_expires_at"),
    )


class TeamArchive(ArchiveBase):
    """Archive table for Team entities."""

    __tablename__ = "team_archive"

    # Key fields from original for querying
    name = Column(String(200), comment="Original team name")
    team_type = Column(String(100), comment="Original team type")
    lead_person_id = Column(BIGINT, comment="Original lead person ID")

    __table_args__ = (
        Index("idx_team_archive_tenant", "tenant_id", "archived_at"),
        Index("idx_team_archive_original_id", "original_id"),
        Index("idx_team_archive_name", "name"),
        Index("idx_team_archive_type", "team_type"),
        Index("idx_team_archive_retention", "retention_expires_at"),
    )


class ArchiveManager:
    """
    Manager class for archive operations.

    Handles archiving, querying, and cleanup of archived records.
    """

    # Mapping of entity models to their archive models
    ARCHIVE_MODEL_MAPPING = {
        Source: SourceArchive,
        Assertion: AssertionArchive,
        Person: PersonArchive,
        Project: ProjectArchive,
        Team: TeamArchive,
    }

    # Default retention periods by entity type
    DEFAULT_RETENTION_PERIODS = {
        Source: timedelta(days=2555),        # 7 years for audit compliance
        Assertion: timedelta(days=2555),     # 7 years for audit compliance
        Person: timedelta(days=3650),        # 10 years for HR compliance
        Project: timedelta(days=3650),       # 10 years for project records
        Team: timedelta(days=1825),          # 5 years for organizational records
    }

    def __init__(self, session: Session):
        """Initialize the archive manager with a database session."""
        self.session = session

    def archive_entity(
        self,
        entity: Any,
        archived_by: str = "system",
        reason: ArchiveReason = ArchiveReason.SOFT_DELETE,
        retention_period: Optional[timedelta] = None,
        metadata: Optional[Dict[str, Any]] = None,
    ) -> Dict[str, Any]:
        """
        Archive a single entity to the corresponding archive table.

        Args:
            entity: The entity to archive
            archived_by: User or system performing the archival
            reason: Reason for archiving
            retention_period: Custom retention period (uses default if None)
            metadata: Additional metadata for the archive record

        Returns:
            Dict containing archive summary

        Raises:
            ValueError: If entity type is not supported for archiving
        """
        entity_type = type(entity)

        if entity_type not in self.ARCHIVE_MODEL_MAPPING:
            raise ValueError(f"Entity type {entity_type.__name__} is not supported for archiving")

        archive_model = self.ARCHIVE_MODEL_MAPPING[entity_type]

        try:
            # Create record snapshot
            record_snapshot = self._create_record_snapshot(entity)

            # Calculate retention expiration
            if retention_period is None:
                retention_period = self.DEFAULT_RETENTION_PERIODS.get(
                    entity_type,
                    timedelta(days=2555)  # Default 7 years
                )

            retention_expires_at = datetime.utcnow() + retention_period

            # Create archive record
            archive_record = archive_model(
                original_id=entity.id,
                archived_by=archived_by,
                archive_reason=reason.value,
                record_snapshot=record_snapshot,
                archive_metadata=metadata or {},
                retention_expires_at=retention_expires_at,
                tenant_id=getattr(entity, 'tenant_id', None),
            )

            # Copy key fields for querying
            self._copy_key_fields(entity, archive_record)

            self.session.add(archive_record)
            self.session.flush()

            archive_summary = {
                'archive_id': archive_record.archive_id,
                'original_id': entity.id,
                'entity_type': entity_type.__name__,
                'archived_at': archive_record.archived_at,
                'archived_by': archived_by,
                'reason': reason.value,
                'retention_expires_at': retention_expires_at,
            }

            logger.info(
                f"Archived {entity_type.__name__}:{entity.id} "
                f"to archive ID {archive_record.archive_id}"
            )

            return archive_summary

        except Exception as e:
            logger.error(f"Failed to archive {entity_type.__name__}:{entity.id}: {str(e)}")
            raise

    def restore_from_archive(
        self,
        archive_id: int,
        restored_by: str = "system",
    ) -> Dict[str, Any]:
        """
        Restore an entity from archive back to the main table.

        Args:
            archive_id: ID of the archive record to restore
            restored_by: User or system performing the restoration

        Returns:
            Dict containing restoration summary

        Raises:
            ValueError: If archive record is not found or restoration fails
        """
        # Find the archive record
        archive_record = None
        original_model = None

        for entity_model, archive_model in self.ARCHIVE_MODEL_MAPPING.items():
            archive_record = self.session.query(archive_model).filter_by(
                archive_id=archive_id
            ).first()

            if archive_record:
                original_model = entity_model
                break

        if not archive_record:
            raise ValueError(f"Archive record with ID {archive_id} not found")

        try:
            # Restore the entity from snapshot
            record_snapshot = archive_record.record_snapshot

            # Create new entity instance
            restored_entity = original_model()

            # Restore fields from snapshot
            for field_name, field_value in record_snapshot.items():
                if hasattr(restored_entity, field_name):
                    setattr(restored_entity, field_name, field_value)

            # Reset deletion fields
            if hasattr(restored_entity, 'is_deleted'):
                restored_entity.is_deleted = False
                restored_entity.deleted_at = None
                restored_entity.deleted_by = None
                restored_entity.deletion_reason = None

            # Update restoration metadata
            restored_entity.updated_at = datetime.utcnow()

            self.session.add(restored_entity)
            self.session.flush()

            restoration_summary = {
                'archive_id': archive_id,
                'restored_entity_id': restored_entity.id,
                'entity_type': original_model.__name__,
                'restored_at': datetime.utcnow(),
                'restored_by': restored_by,
            }

            logger.info(
                f"Restored {original_model.__name__} from archive ID {archive_id} "
                f"to new ID {restored_entity.id}"
            )

            return restoration_summary

        except Exception as e:
            logger.error(f"Failed to restore from archive ID {archive_id}: {str(e)}")
            raise

    def cleanup_expired_archives(
        self,
        dry_run: bool = True,
        batch_size: int = 1000,
    ) -> Dict[str, Any]:
        """
        Clean up expired archive records based on retention policies.

        Args:
            dry_run: If True, only report what would be deleted
            batch_size: Number of records to process in each batch

        Returns:
            Dict containing cleanup summary
        """
        cleanup_summary = {
            'dry_run': dry_run,
            'started_at': datetime.utcnow(),
            'tables_processed': {},
            'total_expired': 0,
            'total_deleted': 0,
        }

        current_time = datetime.utcnow()

        for entity_model, archive_model in self.ARCHIVE_MODEL_MAPPING.items():
            table_name = archive_model.__tablename__

            # Count expired records
            expired_count = self.session.query(archive_model).filter(
                archive_model.retention_expires_at <= current_time
            ).count()

            cleanup_summary['tables_processed'][table_name] = {
                'expired_count': expired_count,
                'deleted_count': 0,
            }

            cleanup_summary['total_expired'] += expired_count

            if not dry_run and expired_count > 0:
                # Delete expired records in batches
                deleted_count = 0

                while True:
                    expired_batch = self.session.query(archive_model).filter(
                        archive_model.retention_expires_at <= current_time
                    ).limit(batch_size).all()

                    if not expired_batch:
                        break

                    for record in expired_batch:
                        self.session.delete(record)
                        deleted_count += 1

                    self.session.commit()  # Commit each batch

                cleanup_summary['tables_processed'][table_name]['deleted_count'] = deleted_count
                cleanup_summary['total_deleted'] += deleted_count

        cleanup_summary['completed_at'] = datetime.utcnow()

        logger.info(
            f"Archive cleanup {'simulation' if dry_run else 'completed'}: "
            f"{cleanup_summary['total_expired']} expired, "
            f"{cleanup_summary['total_deleted']} deleted"
        )

        return cleanup_summary

    def get_archive_statistics(self) -> Dict[str, Any]:
        """
        Get statistics about archived data.

        Returns:
            Dict containing archive statistics
        """
        stats = {
            'generated_at': datetime.utcnow(),
            'tables': {},
            'totals': {
                'total_archived': 0,
                'total_expired': 0,
                'oldest_archive': None,
                'newest_archive': None,
            },
        }

        current_time = datetime.utcnow()

        for entity_model, archive_model in self.ARCHIVE_MODEL_MAPPING.items():
            table_name = archive_model.__tablename__

            total_count = self.session.query(archive_model).count()

            expired_count = self.session.query(archive_model).filter(
                archive_model.retention_expires_at <= current_time
            ).count()

            oldest_record = self.session.query(archive_model).order_by(
                archive_model.archived_at.asc()
            ).first()

            newest_record = self.session.query(archive_model).order_by(
                archive_model.archived_at.desc()
            ).first()

            stats['tables'][table_name] = {
                'total_count': total_count,
                'expired_count': expired_count,
                'oldest_archive': oldest_record.archived_at if oldest_record else None,
                'newest_archive': newest_record.archived_at if newest_record else None,
            }

            stats['totals']['total_archived'] += total_count
            stats['totals']['total_expired'] += expired_count

            # Track global oldest/newest
            if oldest_record and (
                not stats['totals']['oldest_archive'] or
                oldest_record.archived_at < stats['totals']['oldest_archive']
            ):
                stats['totals']['oldest_archive'] = oldest_record.archived_at

            if newest_record and (
                not stats['totals']['newest_archive'] or
                newest_record.archived_at > stats['totals']['newest_archive']
            ):
                stats['totals']['newest_archive'] = newest_record.archived_at

        return stats

    def _create_record_snapshot(self, entity: Any) -> Dict[str, Any]:
        """
        Create a complete snapshot of an entity's state.

        Args:
            entity: The entity to snapshot

        Returns:
            Dict containing entity state
        """
        snapshot = {}

        # Get all columns from the entity
        mapper = entity.__class__.__mapper__

        for column in mapper.columns:
            column_name = column.name
            value = getattr(entity, column_name, None)

            # Handle datetime serialization
            if isinstance(value, datetime):
                snapshot[column_name] = value.isoformat()
            else:
                snapshot[column_name] = value

        return snapshot

    def _copy_key_fields(self, entity: Any, archive_record: Any):
        """
        Copy key fields from entity to archive record for querying.

        Args:
            entity: The original entity
            archive_record: The archive record to populate
        """
        entity_type = type(entity)

        if entity_type == Source:
            archive_record.source_system = entity.source_system
            archive_record.external_id = entity.external_id
            archive_record.content_hash = entity.content_hash
            archive_record.source_timestamp = entity.source_timestamp

        elif entity_type == Assertion:
            archive_record.original_source_id = entity.source_id
            archive_record.assertion_type = entity.assertion_type
            archive_record.confidence_score = entity.confidence_score

        elif entity_type == Person:
            archive_record.canonical_name = entity.canonical_name
            archive_record.company = entity.company

        elif entity_type == Project:
            archive_record.name = entity.name
            archive_record.status = entity.status
            archive_record.start_date = entity.start_date
            archive_record.end_date = entity.end_date

        elif entity_type == Team:
            archive_record.name = entity.name
            archive_record.team_type = entity.team_type
            archive_record.lead_person_id = entity.lead_person_id


# Convenience functions
def archive_entity(
    session: Session,
    entity: Any,
    archived_by: str = "system",
    reason: ArchiveReason = ArchiveReason.SOFT_DELETE,
) -> Dict[str, Any]:
    """
    Convenience function to archive an entity.

    Args:
        session: Database session
        entity: Entity to archive
        archived_by: User performing archival
        reason: Reason for archiving

    Returns:
        Archive summary
    """
    manager = ArchiveManager(session)
    return manager.archive_entity(entity, archived_by, reason)


def restore_from_archive(
    session: Session,
    archive_id: int,
    restored_by: str = "system",
) -> Dict[str, Any]:
    """
    Convenience function to restore from archive.

    Args:
        session: Database session
        archive_id: Archive record ID
        restored_by: User performing restoration

    Returns:
        Restoration summary
    """
    manager = ArchiveManager(session)
    return manager.restore_from_archive(archive_id, restored_by)