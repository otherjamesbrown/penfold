"""
Review Queue Management Interface

Provides interface for managing review tasks, assigning reviewers,
prioritizing work, and tracking review completion metrics.
"""
from fastapi import APIRouter, HTTPException, Depends, Query
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, update, delete, and_, or_, desc, func
from pydantic import BaseModel
from typing import Dict, Any, List, Optional
from datetime import datetime, timedelta
from enum import Enum
import uuid

from app.database import get_db, ReviewQueue, MeetingFile, ProcessingJob
from app.search.privacy import privacy_filter, UserRole

router = APIRouter()


class ReviewStatus(Enum):
    PENDING = "pending"
    IN_REVIEW = "in_review"
    COMPLETED = "completed"
    ESCALATED = "escalated"
    CANCELLED = "cancelled"


class ReviewPriority(Enum):
    LOW = "low"
    MEDIUM = "medium"
    HIGH = "high"
    CRITICAL = "critical"


class ReviewAssignment(BaseModel):
    """Model for assigning reviews to users"""
    review_ids: List[str]
    assigned_to: str
    priority: Optional[str] = "medium"
    due_date: Optional[datetime] = None
    notes: Optional[str] = None


class ReviewCompletion(BaseModel):
    """Model for completing reviews"""
    review_id: str
    resolution: str  # completed, escalated, invalid
    resolution_notes: str
    quality_rating: Optional[int] = None  # 1-5 rating
    time_spent_minutes: Optional[int] = None


class ReviewQueueFilter(BaseModel):
    """Model for filtering review queue"""
    review_types: Optional[List[str]] = None
    statuses: Optional[List[str]] = None
    assigned_to: Optional[str] = None
    priority: Optional[str] = None
    created_after: Optional[datetime] = None
    created_before: Optional[datetime] = None
    meeting_privacy_levels: Optional[List[str]] = None


class ReviewQueueService:
    """Service for review queue management operations"""

    def __init__(self):
        self.review_types = {
            "speaker_identification": {
                "name": "Speaker Identification",
                "description": "Match speakers to known persons",
                "avg_time_minutes": 5,
                "required_skills": ["audio_analysis", "people_knowledge"]
            },
            "project_linking": {
                "name": "Project Linking",
                "description": "Link topics to projects",
                "avg_time_minutes": 3,
                "required_skills": ["project_knowledge", "domain_expertise"]
            },
            "transcription_quality": {
                "name": "Transcription Quality",
                "description": "Review transcript accuracy",
                "avg_time_minutes": 10,
                "required_skills": ["language_skills", "attention_to_detail"]
            }
        }

        self.auto_assignment_rules = {
            "round_robin": "Distribute evenly among available reviewers",
            "expertise": "Assign based on required skills",
            "workload": "Assign to reviewer with lowest current workload",
            "priority": "Assign high priority items to senior reviewers"
        }

    async def get_review_queue(
        self,
        db: AsyncSession,
        filters: ReviewQueueFilter = None,
        page: int = 1,
        page_size: int = 50,
        sort_by: str = "created_at",
        sort_order: str = "desc"
    ) -> Dict[str, Any]:
        """Get review queue with filtering and pagination"""

        try:
            # Build base query
            query = select(ReviewQueue)

            # Apply filters
            filter_conditions = []
            if filters:
                if filters.review_types:
                    filter_conditions.append(ReviewQueue.review_type.in_(filters.review_types))
                if filters.statuses:
                    filter_conditions.append(ReviewQueue.status.in_(filters.statuses))
                if filters.assigned_to:
                    filter_conditions.append(ReviewQueue.assigned_to == uuid.UUID(filters.assigned_to))
                if filters.created_after:
                    filter_conditions.append(ReviewQueue.created_at >= filters.created_after)
                if filters.created_before:
                    filter_conditions.append(ReviewQueue.created_at <= filters.created_before)

            if filter_conditions:
                query = query.where(and_(*filter_conditions))

            # Apply sorting
            if sort_by == "created_at":
                sort_column = ReviewQueue.created_at
            elif sort_by == "priority":
                # Custom priority sorting would go here
                sort_column = ReviewQueue.created_at
            else:
                sort_column = ReviewQueue.created_at

            if sort_order == "desc":
                query = query.order_by(desc(sort_column))
            else:
                query = query.order_by(sort_column)

            # Apply pagination
            offset = (page - 1) * page_size
            query = query.offset(offset).limit(page_size)

            # Execute query
            result = await db.execute(query)
            review_items = result.scalars().all()

            # Get total count for pagination
            count_query = select(func.count(ReviewQueue.id))
            if filter_conditions:
                count_query = count_query.where(and_(*filter_conditions))

            count_result = await db.execute(count_query)
            total_count = count_result.scalar()

            # Format items for display
            formatted_items = []
            for item in review_items:
                formatted_item = await self._format_review_item(item, db)
                formatted_items.append(formatted_item)

            # Calculate queue statistics
            stats = await self._calculate_queue_stats(db, filters)

            return {
                "items": formatted_items,
                "pagination": {
                    "page": page,
                    "page_size": page_size,
                    "total_count": total_count,
                    "total_pages": (total_count + page_size - 1) // page_size,
                    "has_next": page * page_size < total_count,
                    "has_prev": page > 1
                },
                "statistics": stats,
                "filters_applied": filters.dict() if filters else {},
                "sort": {"by": sort_by, "order": sort_order}
            }

        except Exception as e:
            raise HTTPException(status_code=500, detail=f"Failed to get review queue: {str(e)}")

    async def assign_reviews(
        self,
        assignment: ReviewAssignment,
        assigner_user_id: str,
        db: AsyncSession
    ) -> Dict[str, Any]:
        """Assign review items to a user"""

        try:
            successful_assignments = []
            failed_assignments = []

            # Calculate due date if not provided
            due_date = assignment.due_date
            if not due_date:
                # Default due date based on priority
                if assignment.priority == "critical":
                    due_date = datetime.utcnow() + timedelta(hours=4)
                elif assignment.priority == "high":
                    due_date = datetime.utcnow() + timedelta(hours=24)
                else:
                    due_date = datetime.utcnow() + timedelta(days=3)

            for review_id in assignment.review_ids:
                try:
                    # Update review assignment
                    stmt = update(ReviewQueue).where(
                        and_(
                            ReviewQueue.id == uuid.UUID(review_id),
                            ReviewQueue.status == "pending"  # Only assign pending items
                        )
                    ).values(
                        assigned_to=uuid.UUID(assignment.assigned_to) if assignment.assigned_to != "unassigned" else None,
                        status="in_review" if assignment.assigned_to != "unassigned" else "pending",
                        user_feedback={
                            "assignment_notes": assignment.notes,
                            "assigned_by": assigner_user_id,
                            "assigned_at": datetime.utcnow().isoformat(),
                            "priority": assignment.priority,
                            "due_date": due_date.isoformat()
                        }
                    )

                    result = await db.execute(stmt)
                    if result.rowcount > 0:
                        successful_assignments.append(review_id)
                    else:
                        failed_assignments.append({
                            "review_id": review_id,
                            "reason": "Item not found or not in pending status"
                        })

                except Exception as e:
                    failed_assignments.append({
                        "review_id": review_id,
                        "reason": str(e)
                    })

            await db.commit()

            return {
                "success": len(successful_assignments) > 0,
                "successful_assignments": len(successful_assignments),
                "failed_assignments": len(failed_assignments),
                "assigned_to": assignment.assigned_to,
                "priority": assignment.priority,
                "due_date": due_date.isoformat() + "Z",
                "assigned_by": assigner_user_id,
                "failed_details": failed_assignments,
                "timestamp": datetime.utcnow().isoformat() + "Z"
            }

        except Exception as e:
            await db.rollback()
            raise HTTPException(status_code=500, detail=f"Assignment failed: {str(e)}")

    async def complete_review(
        self,
        completion: ReviewCompletion,
        reviewer_user_id: str,
        db: AsyncSession
    ) -> Dict[str, Any]:
        """Mark a review as completed"""

        try:
            # Get the review item
            stmt = select(ReviewQueue).where(
                ReviewQueue.id == uuid.UUID(completion.review_id)
            )
            result = await db.execute(stmt)
            review_item = result.scalar_one_or_none()

            if not review_item:
                raise HTTPException(status_code=404, detail="Review item not found")

            if review_item.status not in ["pending", "in_review"]:
                raise HTTPException(status_code=400, detail="Review item is not in reviewable state")

            # Update review status
            resolution_status = "completed"
            if completion.resolution == "escalated":
                resolution_status = "escalated"
            elif completion.resolution == "invalid":
                resolution_status = "cancelled"

            stmt = update(ReviewQueue).where(
                ReviewQueue.id == uuid.UUID(completion.review_id)
            ).values(
                status=resolution_status,
                resolved_at=datetime.utcnow(),
                resolution_notes=completion.resolution_notes,
                user_feedback={
                    **review_item.user_feedback,
                    "completed_by": reviewer_user_id,
                    "completed_at": datetime.utcnow().isoformat(),
                    "resolution": completion.resolution,
                    "quality_rating": completion.quality_rating,
                    "time_spent_minutes": completion.time_spent_minutes,
                    "resolution_notes": completion.resolution_notes
                }
            )

            await db.execute(stmt)

            # Update reviewer stats (in a real system, this would update a reviewer metrics table)
            await self._update_reviewer_stats(
                reviewer_user_id, completion, review_item.review_type, db
            )

            await db.commit()

            return {
                "success": True,
                "review_id": completion.review_id,
                "resolution": completion.resolution,
                "final_status": resolution_status,
                "quality_rating": completion.quality_rating,
                "time_spent_minutes": completion.time_spent_minutes,
                "completed_by": reviewer_user_id,
                "timestamp": datetime.utcnow().isoformat() + "Z"
            }

        except HTTPException:
            raise
        except Exception as e:
            await db.rollback()
            raise HTTPException(status_code=500, detail=f"Review completion failed: {str(e)}")

    async def bulk_assign_reviews(
        self,
        assignment_rule: str,
        filters: ReviewQueueFilter,
        available_reviewers: List[str],
        assigner_user_id: str,
        db: AsyncSession
    ) -> Dict[str, Any]:
        """Automatically assign reviews based on rules"""

        try:
            # Get pending reviews matching filters
            filter_conditions = [ReviewQueue.status == "pending"]
            if filters:
                if filters.review_types:
                    filter_conditions.append(ReviewQueue.review_type.in_(filters.review_types))

            stmt = select(ReviewQueue).where(and_(*filter_conditions))
            result = await db.execute(stmt)
            pending_reviews = result.scalars().all()

            assignments = []

            if assignment_rule == "round_robin":
                # Distribute evenly
                for i, review in enumerate(pending_reviews):
                    reviewer = available_reviewers[i % len(available_reviewers)]
                    assignments.append((str(review.id), reviewer))

            elif assignment_rule == "workload":
                # Assign to reviewer with lowest workload
                # This would need a workload calculation in a real system
                reviewer_workloads = {r: 0 for r in available_reviewers}
                for review in pending_reviews:
                    min_workload_reviewer = min(reviewer_workloads, key=reviewer_workloads.get)
                    assignments.append((str(review.id), min_workload_reviewer))
                    reviewer_workloads[min_workload_reviewer] += 1

            elif assignment_rule == "expertise":
                # Assign based on review type (simplified)
                for review in pending_reviews:
                    # In a real system, this would match reviewer skills to review requirements
                    reviewer = available_reviewers[0]  # Simplified assignment
                    assignments.append((str(review.id), reviewer))

            # Apply assignments
            successful_count = 0
            failed_count = 0

            for review_id, reviewer in assignments:
                try:
                    assignment = ReviewAssignment(
                        review_ids=[review_id],
                        assigned_to=reviewer,
                        priority="medium"
                    )
                    await self.assign_reviews(assignment, assigner_user_id, db)
                    successful_count += 1
                except Exception:
                    failed_count += 1

            return {
                "success": successful_count > 0,
                "assignment_rule": assignment_rule,
                "total_items": len(pending_reviews),
                "successful_assignments": successful_count,
                "failed_assignments": failed_count,
                "reviewers_used": len(available_reviewers),
                "assigned_by": assigner_user_id,
                "timestamp": datetime.utcnow().isoformat() + "Z"
            }

        except Exception as e:
            raise HTTPException(status_code=500, detail=f"Bulk assignment failed: {str(e)}")

    async def get_reviewer_dashboard(
        self,
        reviewer_user_id: str,
        db: AsyncSession
    ) -> Dict[str, Any]:
        """Get personalized dashboard for a reviewer"""

        try:
            # Get assigned reviews
            stmt = select(ReviewQueue).where(
                and_(
                    ReviewQueue.assigned_to == uuid.UUID(reviewer_user_id),
                    ReviewQueue.status.in_(["in_review", "pending"])
                )
            ).order_by(desc(ReviewQueue.created_at))

            result = await db.execute(stmt)
            assigned_reviews = result.scalars().all()

            # Format for dashboard
            formatted_reviews = []
            for review in assigned_reviews:
                formatted_item = await self._format_review_item(review, db)
                # Add dashboard-specific fields
                due_date = None
                if review.user_feedback and "due_date" in review.user_feedback:
                    due_date = review.user_feedback["due_date"]

                formatted_item.update({
                    "due_date": due_date,
                    "is_overdue": self._is_overdue(due_date),
                    "priority": review.user_feedback.get("priority", "medium") if review.user_feedback else "medium",
                    "estimated_time": self.review_types.get(review.review_type, {}).get("avg_time_minutes", 5)
                })
                formatted_reviews.append(formatted_item)

            # Calculate reviewer stats
            reviewer_stats = await self._get_reviewer_stats(reviewer_user_id, db)

            # Get queue summary
            queue_summary = await self._get_queue_summary_for_reviewer(reviewer_user_id, db)

            return {
                "reviewer_id": reviewer_user_id,
                "assigned_reviews": formatted_reviews,
                "stats": reviewer_stats,
                "queue_summary": queue_summary,
                "review_types": self.review_types,
                "generated_at": datetime.utcnow().isoformat() + "Z"
            }

        except Exception as e:
            raise HTTPException(status_code=500, detail=f"Dashboard generation failed: {str(e)}")

    async def _format_review_item(
        self,
        review_item: ReviewQueue,
        db: AsyncSession
    ) -> Dict[str, Any]:
        """Format a review item for display"""

        # Get meeting information
        stmt = select(
            MeetingFile.filename,
            MeetingFile.created_at,
            MeetingFile.privacy_level
        ).where(MeetingFile.id == review_item.meeting_file_id)

        result = await db.execute(stmt)
        meeting_info = result.first()

        # Calculate age and urgency
        age_hours = (datetime.utcnow() - review_item.created_at).total_seconds() / 3600
        is_urgent = age_hours > 24  # Items older than 24 hours are urgent

        return {
            "review_id": str(review_item.id),
            "meeting_id": str(review_item.meeting_file_id),
            "review_type": review_item.review_type,
            "status": review_item.status,
            "created_at": review_item.created_at.isoformat() + "Z",
            "assigned_to": str(review_item.assigned_to) if review_item.assigned_to else None,
            "resolved_at": review_item.resolved_at.isoformat() + "Z" if review_item.resolved_at else None,
            "age_hours": round(age_hours, 1),
            "is_urgent": is_urgent,
            "meeting_info": {
                "filename": meeting_info.filename if meeting_info else "Unknown",
                "created_at": meeting_info.created_at.isoformat() + "Z" if meeting_info else None,
                "privacy_level": meeting_info.privacy_level if meeting_info else "unknown"
            },
            "ai_suggestions": review_item.ai_suggestions or {},
            "user_feedback": review_item.user_feedback or {},
            "resolution_notes": review_item.resolution_notes,
            "review_type_info": self.review_types.get(review_item.review_type, {})
        }

    async def _calculate_queue_stats(
        self,
        db: AsyncSession,
        filters: ReviewQueueFilter = None
    ) -> Dict[str, Any]:
        """Calculate review queue statistics"""

        try:
            # Base query conditions
            base_conditions = []
            if filters:
                if filters.review_types:
                    base_conditions.append(ReviewQueue.review_type.in_(filters.review_types))

            # Count by status
            stmt = select(
                ReviewQueue.status,
                func.count(ReviewQueue.id).label("count")
            )
            if base_conditions:
                stmt = stmt.where(and_(*base_conditions))
            stmt = stmt.group_by(ReviewQueue.status)

            result = await db.execute(stmt)
            status_counts = {row.status: row.count for row in result.fetchall()}

            # Count by review type
            stmt = select(
                ReviewQueue.review_type,
                func.count(ReviewQueue.id).label("count")
            )
            if base_conditions:
                stmt = stmt.where(and_(*base_conditions))
            stmt = stmt.group_by(ReviewQueue.review_type)

            result = await db.execute(stmt)
            type_counts = {row.review_type: row.count for row in result.fetchall()}

            # Age analysis
            cutoff_24h = datetime.utcnow() - timedelta(hours=24)
            cutoff_7d = datetime.utcnow() - timedelta(days=7)

            stmt = select(func.count(ReviewQueue.id)).where(
                and_(
                    ReviewQueue.status == "pending",
                    ReviewQueue.created_at < cutoff_24h,
                    *base_conditions
                )
            )
            result = await db.execute(stmt)
            overdue_count = result.scalar()

            stmt = select(func.count(ReviewQueue.id)).where(
                and_(
                    ReviewQueue.status == "completed",
                    ReviewQueue.resolved_at >= cutoff_7d,
                    *base_conditions
                )
            )
            result = await db.execute(stmt)
            completed_last_week = result.scalar()

            return {
                "by_status": status_counts,
                "by_type": type_counts,
                "overdue_items": overdue_count,
                "completed_last_week": completed_last_week,
                "total_pending": status_counts.get("pending", 0),
                "total_in_review": status_counts.get("in_review", 0),
                "total_completed": status_counts.get("completed", 0)
            }

        except Exception as e:
            print(f"⚠️ Stats calculation failed: {str(e)}")
            return {}

    async def _update_reviewer_stats(
        self,
        reviewer_id: str,
        completion: ReviewCompletion,
        review_type: str,
        db: AsyncSession
    ) -> None:
        """Update reviewer performance statistics"""

        # In a full implementation, this would update a ReviewerStats table
        print(f"📊 Reviewer {reviewer_id} completed {review_type} review: {completion.resolution}")

    async def _get_reviewer_stats(
        self,
        reviewer_id: str,
        db: AsyncSession
    ) -> Dict[str, Any]:
        """Get performance stats for a reviewer"""

        try:
            # Count completed reviews
            stmt = select(func.count(ReviewQueue.id)).where(
                and_(
                    ReviewQueue.assigned_to == uuid.UUID(reviewer_id),
                    ReviewQueue.status == "completed"
                )
            )
            result = await db.execute(stmt)
            total_completed = result.scalar()

            # Count pending/in_review
            stmt = select(func.count(ReviewQueue.id)).where(
                and_(
                    ReviewQueue.assigned_to == uuid.UUID(reviewer_id),
                    ReviewQueue.status.in_(["pending", "in_review"])
                )
            )
            result = await db.execute(stmt)
            current_workload = result.scalar()

            return {
                "total_completed": total_completed,
                "current_workload": current_workload,
                "completion_rate": "N/A",  # Would be calculated from historical data
                "average_time": "N/A",     # Would be calculated from time tracking
                "quality_rating": "N/A"    # Would be calculated from ratings
            }

        except Exception:
            return {"error": "Stats calculation failed"}

    async def _get_queue_summary_for_reviewer(
        self,
        reviewer_id: str,
        db: AsyncSession
    ) -> Dict[str, Any]:
        """Get queue summary relevant to a reviewer"""

        # Total pending items (not assigned to anyone)
        stmt = select(func.count(ReviewQueue.id)).where(
            and_(
                ReviewQueue.status == "pending",
                ReviewQueue.assigned_to.is_(None)
            )
        )
        result = await db.execute(stmt)
        unassigned_pending = result.scalar()

        return {
            "unassigned_pending": unassigned_pending,
            "available_for_pickup": unassigned_pending  # Simplified
        }

    def _is_overdue(self, due_date_str: str) -> bool:
        """Check if a review is overdue"""

        if not due_date_str:
            return False

        try:
            due_date = datetime.fromisoformat(due_date_str.replace("Z", "+00:00"))
            return datetime.utcnow() > due_date.replace(tzinfo=None)
        except Exception:
            return False


# Global review queue service
review_queue_service = ReviewQueueService()


# API Routes for review queue management

@router.get("/review-queue")
async def get_review_queue_items(
    page: int = Query(1, description="Page number"),
    page_size: int = Query(50, description="Items per page"),
    review_types: List[str] = Query(None, description="Filter by review types"),
    statuses: List[str] = Query(None, description="Filter by statuses"),
    assigned_to: str = Query(None, description="Filter by assigned reviewer"),
    sort_by: str = Query("created_at", description="Sort field"),
    sort_order: str = Query("desc", description="Sort order"),
    db: AsyncSession = Depends(get_db)
):
    """Get review queue with filtering and pagination"""

    try:
        filters = ReviewQueueFilter(
            review_types=review_types,
            statuses=statuses,
            assigned_to=assigned_to
        )

        queue_data = await review_queue_service.get_review_queue(
            db, filters, page, page_size, sort_by, sort_order
        )

        return queue_data

    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.post("/review-queue/assign")
async def assign_review_items(
    assignment: ReviewAssignment,
    db: AsyncSession = Depends(get_db)
):
    """Assign review items to a user"""

    # Mock assigner for demo
    assigner_user_id = "demo_admin"

    try:
        result = await review_queue_service.assign_reviews(
            assignment, assigner_user_id, db
        )
        return result

    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.post("/review-queue/complete")
async def complete_review_item(
    completion: ReviewCompletion,
    db: AsyncSession = Depends(get_db)
):
    """Mark a review as completed"""

    # Mock reviewer for demo
    reviewer_user_id = "demo_reviewer"

    try:
        result = await review_queue_service.complete_review(
            completion, reviewer_user_id, db
        )
        return result

    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/review-queue/dashboard/{reviewer_id}")
async def get_reviewer_dashboard(
    reviewer_id: str,
    db: AsyncSession = Depends(get_db)
):
    """Get personalized dashboard for a reviewer"""

    try:
        dashboard_data = await review_queue_service.get_reviewer_dashboard(
            reviewer_id, db
        )
        return dashboard_data

    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.post("/review-queue/bulk-assign")
async def bulk_assign_review_items(
    assignment_rule: str = Query(..., description="Assignment rule"),
    review_types: List[str] = Query(None, description="Review types to assign"),
    available_reviewers: List[str] = Query(..., description="Available reviewer IDs"),
    db: AsyncSession = Depends(get_db)
):
    """Automatically assign reviews based on rules"""

    # Mock assigner for demo
    assigner_user_id = "demo_admin"

    try:
        filters = ReviewQueueFilter(review_types=review_types)

        result = await review_queue_service.bulk_assign_reviews(
            assignment_rule, filters, available_reviewers, assigner_user_id, db
        )
        return result

    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))