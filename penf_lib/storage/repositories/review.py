"""Review repository with specialized operations for daily review workflow."""

import uuid
from datetime import datetime
from typing import List, Optional
from uuid import UUID

from sqlalchemy import select, and_, or_, func, desc, asc
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import selectinload

from penf_lib.storage.models import (
    ReviewSessionModel,
    ReviewItemModel,
    UserFeedbackModel,
    LearningRuleModel,
    BatchOperationModel,
    ReviewAnalyticsModel,
)
from .base import BaseRepository


class ReviewRepository(BaseRepository[ReviewSessionModel]):
    """Repository for review workflow entities.

    Provides database operations for review sessions, items, feedback,
    learning rules, batch operations, and analytics.
    """

    def __init__(self, session: AsyncSession):
        """Initialize review repository.

        Args:
            session: Async SQLAlchemy session
        """
        super().__init__(session, ReviewSessionModel)

    # =========================================================================
    # Session Operations
    # =========================================================================

    async def create_session(
        self, session_model: ReviewSessionModel
    ) -> ReviewSessionModel:
        """Persist a new review session.

        Args:
            session_model: ReviewSessionModel instance to persist

        Returns:
            Created review session with ID populated
        """
        self.session.add(session_model)
        await self.session.flush()
        return session_model

    async def get_session_by_id(
        self, session_id: int
    ) -> Optional[ReviewSessionModel]:
        """Get review session by ID.

        Args:
            session_id: Session ID

        Returns:
            ReviewSessionModel instance or None if not found
        """
        stmt = select(ReviewSessionModel).where(
            ReviewSessionModel.id == session_id
        )
        result = await self.session.execute(stmt)
        return result.scalar_one_or_none()

    async def get_active_session_for_user(
        self, tenant_id: UUID, user_email: str
    ) -> Optional[ReviewSessionModel]:
        """Get active review session for a user.

        Args:
            tenant_id: Tenant ID for isolation
            user_email: User's email address

        Returns:
            Active ReviewSessionModel or None if no active session exists
        """
        stmt = (
            select(ReviewSessionModel)
            .where(
                and_(
                    ReviewSessionModel.tenant_id == tenant_id,
                    ReviewSessionModel.user_email == user_email,
                    ReviewSessionModel.status == "active",
                    ReviewSessionModel.expires_at > func.now(),
                )
            )
            .order_by(desc(ReviewSessionModel.started_at))
            .limit(1)
        )
        result = await self.session.execute(stmt)
        return result.scalar_one_or_none()

    async def update_session(
        self, session_model: ReviewSessionModel
    ) -> ReviewSessionModel:
        """Update an existing review session.

        Args:
            session_model: ReviewSessionModel instance with updated values

        Returns:
            Updated ReviewSessionModel
        """
        session_model.updated_at = datetime.utcnow()
        await self.session.flush()
        return session_model

    # =========================================================================
    # Review Item Operations
    # =========================================================================

    async def get_queue_items(
        self,
        session_id: int,
        status: Optional[List[str]] = None,
        limit: int = 100,
        offset: int = 0,
    ) -> List[ReviewItemModel]:
        """Get items in the review queue with optional filtering.

        Args:
            session_id: Session ID to get items for
            status: Optional list of status values to filter by
            limit: Maximum number of items to return
            offset: Number of items to skip for pagination

        Returns:
            List of ReviewItemModel instances ordered by queue position
        """
        stmt = (
            select(ReviewItemModel)
            .where(ReviewItemModel.session_id == session_id)
            .order_by(asc(ReviewItemModel.queue_position))
        )

        if status:
            stmt = stmt.where(ReviewItemModel.status.in_(status))

        stmt = stmt.offset(offset).limit(limit)

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def create_review_item(
        self, item: ReviewItemModel
    ) -> ReviewItemModel:
        """Create a new review item.

        Args:
            item: ReviewItemModel instance to persist

        Returns:
            Created ReviewItemModel with ID populated
        """
        self.session.add(item)
        await self.session.flush()
        return item

    async def update_review_item(
        self, item: ReviewItemModel
    ) -> ReviewItemModel:
        """Update an existing review item.

        Args:
            item: ReviewItemModel instance with updated values

        Returns:
            Updated ReviewItemModel
        """
        item.updated_at = datetime.utcnow()
        await self.session.flush()
        return item

    async def get_undo_eligible_items(
        self, session_id: int, count: int = 1
    ) -> List[ReviewItemModel]:
        """Get recent review items eligible for undo.

        Args:
            session_id: Session ID to get items for
            count: Maximum number of items to return

        Returns:
            List of ReviewItemModel instances eligible for undo,
            ordered by most recently reviewed first
        """
        stmt = (
            select(ReviewItemModel)
            .where(
                and_(
                    ReviewItemModel.session_id == session_id,
                    ReviewItemModel.undo_eligible.is_(True),
                    or_(
                        ReviewItemModel.undo_deadline.is_(None),
                        ReviewItemModel.undo_deadline > func.now(),
                    ),
                    ReviewItemModel.user_decision.isnot(None),
                )
            )
            .order_by(desc(ReviewItemModel.reviewed_at))
            .limit(count)
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def get_pending_items_for_queue(
        self,
        tenant_id: UUID,
        limit: int = 100,
        confidence_threshold: float = 0.0,
    ) -> List[ReviewItemModel]:
        """Get pending review items for populating a new queue.

        Retrieves items that are pending review and not yet assigned to
        a session, ordered by AI confidence (lowest first for human review).

        Args:
            tenant_id: Tenant ID for isolation
            limit: Maximum number of items to return
            confidence_threshold: Minimum AI confidence to include

        Returns:
            List of unassigned ReviewItemModel instances ready for review
        """
        stmt = (
            select(ReviewItemModel)
            .where(
                and_(
                    ReviewItemModel.tenant_id == tenant_id,
                    ReviewItemModel.session_id.is_(None),
                    ReviewItemModel.status == "pending",
                    ReviewItemModel.ai_confidence >= confidence_threshold,
                )
            )
            .order_by(
                asc(ReviewItemModel.ai_confidence),
                desc(ReviewItemModel.business_importance),
                desc(ReviewItemModel.source_timestamp),
            )
            .limit(limit)
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def get_item_by_id(
        self, item_id: int
    ) -> Optional[ReviewItemModel]:
        """Get review item by ID.

        Args:
            item_id: Item ID

        Returns:
            ReviewItemModel instance or None if not found
        """
        stmt = select(ReviewItemModel).where(ReviewItemModel.id == item_id)
        result = await self.session.execute(stmt)
        return result.scalar_one_or_none()

    async def get_items_by_batch_id(
        self, batch_id: UUID
    ) -> List[ReviewItemModel]:
        """Get review items associated with a batch operation.

        Args:
            batch_id: Batch operation UUID

        Returns:
            List of ReviewItemModel instances in the batch
        """
        stmt = (
            select(ReviewItemModel)
            .where(ReviewItemModel.batch_id == batch_id)
            .order_by(asc(ReviewItemModel.queue_position))
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    # =========================================================================
    # Feedback Operations
    # =========================================================================

    async def create_feedback(
        self, feedback: UserFeedbackModel
    ) -> UserFeedbackModel:
        """Record user feedback on a review item.

        Args:
            feedback: UserFeedbackModel instance to persist

        Returns:
            Created UserFeedbackModel with ID populated
        """
        self.session.add(feedback)
        await self.session.flush()
        return feedback

    async def get_feedback_for_item(
        self, item_id: int
    ) -> List[UserFeedbackModel]:
        """Get all feedback for a review item.

        Args:
            item_id: Review item ID

        Returns:
            List of UserFeedbackModel instances for the item
        """
        stmt = (
            select(UserFeedbackModel)
            .where(UserFeedbackModel.review_item_id == item_id)
            .order_by(desc(UserFeedbackModel.created_at))
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def get_feedback_for_session(
        self, session_id: int, limit: int = 100
    ) -> List[UserFeedbackModel]:
        """Get all feedback for a review session.

        Args:
            session_id: Review session ID
            limit: Maximum number of feedback records to return

        Returns:
            List of UserFeedbackModel instances for the session
        """
        stmt = (
            select(UserFeedbackModel)
            .where(UserFeedbackModel.session_id == session_id)
            .order_by(desc(UserFeedbackModel.created_at))
            .limit(limit)
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    # =========================================================================
    # Learning Rule Operations
    # =========================================================================

    async def create_learning_rule(
        self, rule: LearningRuleModel
    ) -> LearningRuleModel:
        """Create a new learning rule.

        Args:
            rule: LearningRuleModel instance to persist

        Returns:
            Created LearningRuleModel with ID populated
        """
        self.session.add(rule)
        await self.session.flush()
        return rule

    async def get_active_learning_rules(
        self, tenant_id: UUID, limit: int = 100
    ) -> List[LearningRuleModel]:
        """Get active learning rules for a tenant.

        Args:
            tenant_id: Tenant ID for isolation
            limit: Maximum number of rules to return

        Returns:
            List of active LearningRuleModel instances ordered by priority
        """
        stmt = (
            select(LearningRuleModel)
            .where(
                and_(
                    LearningRuleModel.tenant_id == tenant_id,
                    LearningRuleModel.status == "active",
                    LearningRuleModel.is_deleted.is_(False),
                    or_(
                        LearningRuleModel.expires_at.is_(None),
                        LearningRuleModel.expires_at > func.now(),
                    ),
                )
            )
            .order_by(asc(LearningRuleModel.priority))
            .limit(limit)
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    async def update_learning_rule(
        self, rule: LearningRuleModel
    ) -> LearningRuleModel:
        """Update an existing learning rule.

        Args:
            rule: LearningRuleModel instance with updated values

        Returns:
            Updated LearningRuleModel
        """
        rule.updated_at = datetime.utcnow()
        await self.session.flush()
        return rule

    async def increment_rule_applied_count(
        self, rule_id: int
    ) -> Optional[LearningRuleModel]:
        """Increment the times_applied counter for a learning rule.

        Args:
            rule_id: Learning rule ID

        Returns:
            Updated LearningRuleModel or None if not found
        """
        stmt = select(LearningRuleModel).where(LearningRuleModel.id == rule_id)
        result = await self.session.execute(stmt)
        rule = result.scalar_one_or_none()

        if rule:
            rule.times_applied += 1
            rule.last_applied_at = datetime.utcnow()
            rule.updated_at = datetime.utcnow()
            await self.session.flush()

        return rule

    # =========================================================================
    # Batch Operation Operations
    # =========================================================================

    async def create_batch_operation(
        self, batch: BatchOperationModel
    ) -> BatchOperationModel:
        """Create a new batch operation record.

        Args:
            batch: BatchOperationModel instance to persist

        Returns:
            Created BatchOperationModel with ID populated
        """
        self.session.add(batch)
        await self.session.flush()
        return batch

    async def get_batch_operation(
        self, batch_id: UUID
    ) -> Optional[BatchOperationModel]:
        """Get batch operation by UUID.

        Args:
            batch_id: Batch operation UUID

        Returns:
            BatchOperationModel instance or None if not found
        """
        stmt = select(BatchOperationModel).where(
            BatchOperationModel.batch_uuid == batch_id
        )
        result = await self.session.execute(stmt)
        return result.scalar_one_or_none()

    async def update_batch_operation(
        self, batch: BatchOperationModel
    ) -> BatchOperationModel:
        """Update an existing batch operation.

        Args:
            batch: BatchOperationModel instance with updated values

        Returns:
            Updated BatchOperationModel
        """
        batch.updated_at = datetime.utcnow()
        await self.session.flush()
        return batch

    async def get_undo_eligible_batches(
        self, session_id: int, count: int = 1
    ) -> List[BatchOperationModel]:
        """Get recent batch operations eligible for undo.

        Args:
            session_id: Session ID to get batches for
            count: Maximum number of batches to return

        Returns:
            List of BatchOperationModel instances eligible for undo
        """
        stmt = (
            select(BatchOperationModel)
            .where(
                and_(
                    BatchOperationModel.session_id == session_id,
                    BatchOperationModel.undo_eligible.is_(True),
                    BatchOperationModel.status == "applied",
                    or_(
                        BatchOperationModel.undo_deadline.is_(None),
                        BatchOperationModel.undo_deadline > func.now(),
                    ),
                )
            )
            .order_by(desc(BatchOperationModel.applied_at))
            .limit(count)
        )

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    # =========================================================================
    # Analytics Operations
    # =========================================================================

    async def create_analytics_record(
        self, analytics: ReviewAnalyticsModel
    ) -> ReviewAnalyticsModel:
        """Create a new analytics record.

        Args:
            analytics: ReviewAnalyticsModel instance to persist

        Returns:
            Created ReviewAnalyticsModel with ID populated
        """
        self.session.add(analytics)
        await self.session.flush()
        return analytics

    async def get_analytics_for_period(
        self,
        tenant_id: UUID,
        period_type: str,
        start_date: datetime,
        end_date: Optional[datetime] = None,
    ) -> List[ReviewAnalyticsModel]:
        """Get analytics records for a time period.

        Args:
            tenant_id: Tenant ID for isolation
            period_type: Period granularity (daily, weekly, monthly)
            start_date: Start of the period
            end_date: Optional end of the period

        Returns:
            List of ReviewAnalyticsModel instances
        """
        stmt = (
            select(ReviewAnalyticsModel)
            .where(
                and_(
                    ReviewAnalyticsModel.tenant_id == tenant_id,
                    ReviewAnalyticsModel.period_type == period_type,
                    ReviewAnalyticsModel.period_start >= start_date,
                )
            )
            .order_by(asc(ReviewAnalyticsModel.period_start))
        )

        if end_date:
            stmt = stmt.where(ReviewAnalyticsModel.period_end <= end_date)

        result = await self.session.execute(stmt)
        return list(result.scalars().all())

    # =========================================================================
    # Statistics Operations
    # =========================================================================

    async def get_session_stats(self, session_id: int) -> dict:
        """Get statistics for a review session.

        Args:
            session_id: Session ID

        Returns:
            Dictionary with session statistics
        """
        # Get decision breakdown
        decision_stmt = (
            select(
                ReviewItemModel.user_decision,
                func.count(ReviewItemModel.id),
            )
            .where(
                and_(
                    ReviewItemModel.session_id == session_id,
                    ReviewItemModel.user_decision.isnot(None),
                )
            )
            .group_by(ReviewItemModel.user_decision)
        )

        decision_result = await self.session.execute(decision_stmt)
        decision_breakdown = dict(decision_result.all())

        # Get average review time
        avg_time_stmt = (
            select(func.avg(ReviewItemModel.review_duration_ms))
            .where(
                and_(
                    ReviewItemModel.session_id == session_id,
                    ReviewItemModel.review_duration_ms.isnot(None),
                )
            )
        )

        avg_time_result = await self.session.execute(avg_time_stmt)
        avg_time = avg_time_result.scalar() or 0

        # Get pending count
        pending_stmt = (
            select(func.count(ReviewItemModel.id))
            .where(
                and_(
                    ReviewItemModel.session_id == session_id,
                    ReviewItemModel.status == "pending",
                )
            )
        )

        pending_result = await self.session.execute(pending_stmt)
        pending_count = pending_result.scalar() or 0

        return {
            "decision_breakdown": decision_breakdown,
            "average_review_time_ms": float(avg_time),
            "pending_count": pending_count,
            "total_reviewed": sum(decision_breakdown.values()),
        }

    async def get_tenant_review_stats(
        self, tenant_id: UUID, days: int = 30
    ) -> dict:
        """Get review statistics for a tenant over a time period.

        Args:
            tenant_id: Tenant ID
            days: Number of days to look back

        Returns:
            Dictionary with tenant review statistics
        """
        cutoff = datetime.utcnow()

        # Get session count
        session_stmt = (
            select(func.count(ReviewSessionModel.id))
            .where(
                and_(
                    ReviewSessionModel.tenant_id == tenant_id,
                    ReviewSessionModel.started_at >= cutoff,
                )
            )
        )

        session_result = await self.session.execute(session_stmt)
        session_count = session_result.scalar() or 0

        # Get total items reviewed
        items_stmt = (
            select(func.sum(ReviewSessionModel.items_reviewed))
            .where(
                and_(
                    ReviewSessionModel.tenant_id == tenant_id,
                    ReviewSessionModel.started_at >= cutoff,
                )
            )
        )

        items_result = await self.session.execute(items_stmt)
        total_items = items_result.scalar() or 0

        # Get active learning rules count
        rules_stmt = (
            select(func.count(LearningRuleModel.id))
            .where(
                and_(
                    LearningRuleModel.tenant_id == tenant_id,
                    LearningRuleModel.status == "active",
                    LearningRuleModel.is_deleted.is_(False),
                )
            )
        )

        rules_result = await self.session.execute(rules_stmt)
        active_rules = rules_result.scalar() or 0

        return {
            "session_count": session_count,
            "total_items_reviewed": int(total_items),
            "active_learning_rules": active_rules,
            "period_days": days,
        }
