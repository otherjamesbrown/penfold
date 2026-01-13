"""
Soft delete utilities and helper functions for the Penfold storage layer.

This module provides comprehensive soft delete functionality including:
- Cascade deletion with relationship preservation
- Batch operations for performance
- Query helpers for filtering
- Archive operations for audit trails
"""

import logging
from typing import Dict, List, Optional, Set, Type, Any, Union
from datetime import datetime
from enum import Enum

from sqlalchemy.orm import Session, Query
from sqlalchemy.exc import SQLAlchemyError
from sqlalchemy import and_, or_, inspect

from .models import (
    Base,
    SoftDeleteMixin,
    Source,
    Assertion,
    Person,
    Project,
    Team,
    Embedding,
)

logger = logging.getLogger(__name__)


class CascadeMode(Enum):
    """Cascade modes for soft delete operations."""

    RESTRICT = "restrict"  # Prevent deletion if dependencies exist
    CASCADE = "cascade"    # Delete all dependent records
    SET_NULL = "set_null"  # Set foreign key references to NULL
    PRESERVE = "preserve"  # Keep references but mark as orphaned


class SoftDeleteError(Exception):
    """Custom exception for soft delete operations."""
    pass


class SoftDeleteManager:
    """
    Manager class for handling soft delete operations across the schema.

    Provides centralized logic for complex soft delete scenarios including
    cascade deletion, batch operations, and relationship management.
    """

    # Define cascade relationships between entities
    CASCADE_RELATIONSHIPS = {
        Source: {
            'assertions': (Assertion, 'source_id', CascadeMode.CASCADE),
            'embeddings': (Embedding, 'source_id', CascadeMode.CASCADE),
        },
        Assertion: {
            'embeddings': (Embedding, 'assertion_id', CascadeMode.CASCADE),
        },
        Person: {
            'embeddings': (Embedding, 'person_id', CascadeMode.CASCADE),
            # Projects and teams reference people but should not cascade
            'project_memberships': (Project, 'team_members', CascadeMode.SET_NULL),
            'team_memberships': (Team, 'member_person_ids', CascadeMode.SET_NULL),
            'team_leadership': (Team, 'lead_person_id', CascadeMode.SET_NULL),
        },
        Project: {
            'embeddings': (Embedding, 'project_id', CascadeMode.CASCADE),
            'children': (Project, 'parent_id', CascadeMode.SET_NULL),
        },
        Team: {
            'embeddings': (Embedding, 'team_id', CascadeMode.CASCADE),
            'children': (Team, 'parent_team_id', CascadeMode.SET_NULL),
        },
    }

    def __init__(self, session: Session):
        """Initialize the soft delete manager with a database session."""
        self.session = session

    def soft_delete_single(
        self,
        entity: SoftDeleteMixin,
        deleted_by: str = "system",
        reason: str = None,
        cascade: bool = True,
    ) -> Dict[str, Any]:
        """
        Soft delete a single entity with optional cascade.

        Args:
            entity: The entity to soft delete
            deleted_by: User or system performing the deletion
            reason: Reason for deletion
            cascade: Whether to cascade delete related entities

        Returns:
            Dict containing deletion summary and affected entities

        Raises:
            SoftDeleteError: If deletion fails or constraints are violated
        """
        try:
            if entity.is_deleted:
                raise SoftDeleteError(f"Entity {entity.__class__.__name__}:{entity.id} is already deleted")

            deletion_summary = {
                'primary_entity': {
                    'type': entity.__class__.__name__,
                    'id': entity.id,
                    'deleted_at': datetime.utcnow(),
                },
                'cascaded_entities': [],
                'warnings': [],
            }

            # Handle cascade deletion if requested
            if cascade:
                cascaded = self._cascade_delete(entity, deleted_by, reason)
                deletion_summary['cascaded_entities'] = cascaded

            # Perform the primary deletion
            entity.soft_delete(deleted_by, reason)

            # Update timestamp for summary
            deletion_summary['primary_entity']['deleted_at'] = entity.deleted_at

            self.session.flush()  # Ensure changes are visible in this transaction

            logger.info(
                f"Soft deleted {entity.__class__.__name__}:{entity.id} "
                f"by {deleted_by}, cascaded {len(deletion_summary['cascaded_entities'])} related entities"
            )

            return deletion_summary

        except Exception as e:
            self.session.rollback()
            logger.error(f"Failed to soft delete {entity.__class__.__name__}:{entity.id}: {str(e)}")
            raise SoftDeleteError(f"Soft delete failed: {str(e)}")

    def soft_delete_batch(
        self,
        entities: List[SoftDeleteMixin],
        deleted_by: str = "system",
        reason: str = None,
        cascade: bool = True,
    ) -> Dict[str, Any]:
        """
        Soft delete multiple entities in a batch operation.

        Args:
            entities: List of entities to soft delete
            deleted_by: User or system performing the deletion
            reason: Reason for deletion
            cascade: Whether to cascade delete related entities

        Returns:
            Dict containing batch deletion summary

        Raises:
            SoftDeleteError: If batch deletion fails
        """
        if not entities:
            return {'deleted_count': 0, 'entities': [], 'warnings': []}

        try:
            batch_summary = {
                'deleted_count': 0,
                'entities': [],
                'warnings': [],
                'started_at': datetime.utcnow(),
            }

            for entity in entities:
                if entity.is_deleted:
                    batch_summary['warnings'].append(
                        f"{entity.__class__.__name__}:{entity.id} already deleted"
                    )
                    continue

                entity_summary = self.soft_delete_single(entity, deleted_by, reason, cascade)
                batch_summary['entities'].append(entity_summary)
                batch_summary['deleted_count'] += 1

            batch_summary['completed_at'] = datetime.utcnow()

            logger.info(
                f"Batch soft delete completed: {batch_summary['deleted_count']} entities deleted "
                f"by {deleted_by}"
            )

            return batch_summary

        except Exception as e:
            self.session.rollback()
            logger.error(f"Batch soft delete failed: {str(e)}")
            raise SoftDeleteError(f"Batch soft delete failed: {str(e)}")

    def restore_single(
        self,
        entity: SoftDeleteMixin,
        restored_by: str = "system",
        restore_cascade: bool = False,
    ) -> Dict[str, Any]:
        """
        Restore a soft-deleted entity.

        Args:
            entity: The entity to restore
            restored_by: User or system performing the restoration
            restore_cascade: Whether to restore cascaded entities

        Returns:
            Dict containing restoration summary

        Raises:
            SoftDeleteError: If restoration fails
        """
        try:
            if not entity.is_deleted:
                raise SoftDeleteError(f"Entity {entity.__class__.__name__}:{entity.id} is not deleted")

            restoration_summary = {
                'primary_entity': {
                    'type': entity.__class__.__name__,
                    'id': entity.id,
                    'restored_at': datetime.utcnow(),
                },
                'restored_cascade': [],
                'warnings': [],
            }

            # Restore the primary entity
            entity.restore(restored_by)

            # Handle cascade restoration if requested
            if restore_cascade:
                # This is more complex as we need to track what was cascade deleted
                # Implementation would require additional tracking metadata
                restoration_summary['warnings'].append(
                    "Cascade restoration not implemented - requires deletion tracking"
                )

            self.session.flush()

            logger.info(
                f"Restored {entity.__class__.__name__}:{entity.id} by {restored_by}"
            )

            return restoration_summary

        except Exception as e:
            self.session.rollback()
            logger.error(f"Failed to restore {entity.__class__.__name__}:{entity.id}: {str(e)}")
            raise SoftDeleteError(f"Restoration failed: {str(e)}")

    def _cascade_delete(
        self,
        entity: SoftDeleteMixin,
        deleted_by: str,
        reason: str,
    ) -> List[Dict[str, Any]]:
        """
        Perform cascade deletion based on defined relationships.

        Args:
            entity: The primary entity being deleted
            deleted_by: User performing the deletion
            reason: Reason for deletion

        Returns:
            List of cascaded deletion summaries
        """
        cascaded = []
        entity_type = type(entity)

        if entity_type not in self.CASCADE_RELATIONSHIPS:
            return cascaded

        relationships = self.CASCADE_RELATIONSHIPS[entity_type]

        for relationship_name, (target_model, foreign_key, cascade_mode) in relationships.items():
            try:
                if cascade_mode == CascadeMode.CASCADE:
                    # Find and cascade delete related entities
                    related_entities = self.session.query(target_model).filter(
                        getattr(target_model, foreign_key) == entity.id,
                        target_model.is_deleted == False
                    ).all()

                    for related_entity in related_entities:
                        if hasattr(related_entity, 'soft_delete'):
                            related_entity.soft_delete(
                                deleted_by,
                                f"Cascade from {entity.__class__.__name__}:{entity.id}"
                            )
                            cascaded.append({
                                'type': target_model.__name__,
                                'id': related_entity.id,
                                'cascade_mode': cascade_mode.value,
                                'relationship': relationship_name,
                            })

                elif cascade_mode == CascadeMode.SET_NULL:
                    # Set foreign key references to NULL
                    if foreign_key.endswith('_ids'):  # Array field
                        # Handle array fields (like team_members)
                        related_entities = self.session.query(target_model).filter(
                            getattr(target_model, foreign_key).any(entity.id)
                        ).all()

                        for related_entity in related_entities:
                            current_array = getattr(related_entity, foreign_key) or []
                            updated_array = [id for id in current_array if id != entity.id]
                            setattr(related_entity, foreign_key, updated_array)

                    else:
                        # Handle single foreign key references
                        self.session.query(target_model).filter(
                            getattr(target_model, foreign_key) == entity.id
                        ).update({foreign_key: None})

                    cascaded.append({
                        'type': target_model.__name__,
                        'cascade_mode': cascade_mode.value,
                        'relationship': relationship_name,
                        'action': 'nullified_references',
                    })

            except Exception as e:
                logger.warning(
                    f"Failed to cascade {cascade_mode.value} for {relationship_name}: {str(e)}"
                )
                # Continue with other relationships
                continue

        return cascaded

    def get_deletion_impact(self, entity: SoftDeleteMixin) -> Dict[str, Any]:
        """
        Analyze the impact of deleting an entity without actually deleting it.

        Args:
            entity: The entity to analyze

        Returns:
            Dict containing impact analysis
        """
        impact = {
            'entity': {
                'type': entity.__class__.__name__,
                'id': entity.id,
                'is_deleted': entity.is_deleted,
            },
            'dependencies': {},
            'cascade_count': 0,
            'blocking_dependencies': [],
        }

        entity_type = type(entity)

        if entity_type not in self.CASCADE_RELATIONSHIPS:
            return impact

        relationships = self.CASCADE_RELATIONSHIPS[entity_type]

        for relationship_name, (target_model, foreign_key, cascade_mode) in relationships.items():
            try:
                if cascade_mode == CascadeMode.CASCADE:
                    related_count = self.session.query(target_model).filter(
                        getattr(target_model, foreign_key) == entity.id,
                        target_model.is_deleted == False
                    ).count()

                elif cascade_mode == CascadeMode.SET_NULL:
                    if foreign_key.endswith('_ids'):
                        related_count = self.session.query(target_model).filter(
                            getattr(target_model, foreign_key).any(entity.id)
                        ).count()
                    else:
                        related_count = self.session.query(target_model).filter(
                            getattr(target_model, foreign_key) == entity.id
                        ).count()

                else:
                    related_count = 0

                impact['dependencies'][relationship_name] = {
                    'target_model': target_model.__name__,
                    'cascade_mode': cascade_mode.value,
                    'affected_count': related_count,
                }

                if cascade_mode == CascadeMode.CASCADE:
                    impact['cascade_count'] += related_count
                elif cascade_mode == CascadeMode.RESTRICT and related_count > 0:
                    impact['blocking_dependencies'].append(relationship_name)

            except Exception as e:
                logger.warning(f"Failed to analyze impact for {relationship_name}: {str(e)}")
                continue

        return impact


# Query helper functions
def active_query(query: Query) -> Query:
    """
    Filter a query to only return active (non-deleted) records.

    Args:
        query: SQLAlchemy query object

    Returns:
        Filtered query excluding soft-deleted records
    """
    model = query.column_descriptions[0]['type']
    if hasattr(model, 'is_deleted'):
        return query.filter(model.is_deleted == False)
    return query


def deleted_query(query: Query) -> Query:
    """
    Filter a query to only return soft-deleted records.

    Args:
        query: SQLAlchemy query object

    Returns:
        Filtered query including only soft-deleted records
    """
    model = query.column_descriptions[0]['type']
    if hasattr(model, 'is_deleted'):
        return query.filter(model.is_deleted == True)
    return query.filter(False)  # Return empty result if no soft delete support


def with_deleted_query(query: Query) -> Query:
    """
    Return query that includes both active and deleted records.

    Args:
        query: SQLAlchemy query object

    Returns:
        Unfiltered query
    """
    return query


# Convenience functions for common operations
def soft_delete_with_cascade(
    session: Session,
    entity: SoftDeleteMixin,
    deleted_by: str = "system",
    reason: str = None,
) -> Dict[str, Any]:
    """
    Convenience function to soft delete an entity with cascade.

    Args:
        session: Database session
        entity: Entity to delete
        deleted_by: User performing deletion
        reason: Reason for deletion

    Returns:
        Deletion summary
    """
    manager = SoftDeleteManager(session)
    return manager.soft_delete_single(entity, deleted_by, reason, cascade=True)


def restore_entity(
    session: Session,
    entity: SoftDeleteMixin,
    restored_by: str = "system",
) -> Dict[str, Any]:
    """
    Convenience function to restore a soft-deleted entity.

    Args:
        session: Database session
        entity: Entity to restore
        restored_by: User performing restoration

    Returns:
        Restoration summary
    """
    manager = SoftDeleteManager(session)
    return manager.restore_single(entity, restored_by)


def analyze_deletion_impact(
    session: Session,
    entity: SoftDeleteMixin,
) -> Dict[str, Any]:
    """
    Convenience function to analyze deletion impact without deleting.

    Args:
        session: Database session
        entity: Entity to analyze

    Returns:
        Impact analysis
    """
    manager = SoftDeleteManager(session)
    return manager.get_deletion_impact(entity)