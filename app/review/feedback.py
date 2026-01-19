"""
User Feedback Collection for AI Improvement

Collects user feedback on AI-generated content to improve model accuracy,
tracks correction patterns, and provides data for retraining.
"""
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, insert, update, delete, desc, and_, func
from sqlalchemy.orm import mapped_column, Mapped
from sqlalchemy.dialects.postgresql import UUID, JSONB
from sqlalchemy import String, DateTime, Text, Integer, Float, Boolean
from sqlalchemy.sql import func as sql_func
from pydantic import BaseModel
from typing import Dict, Any, List, Optional
from datetime import datetime, timedelta
from enum import Enum
import uuid
import json

from app.database import Base


class FeedbackType(Enum):
    TRANSCRIPTION_QUALITY = "transcription_quality"
    SPEAKER_IDENTIFICATION = "speaker_identification"
    SUMMARY_ACCURACY = "summary_accuracy"
    KEY_POINTS_RELEVANCE = "key_points_relevance"
    ACTION_ITEMS_COMPLETENESS = "action_items_completeness"
    ENTITY_RESOLUTION = "entity_resolution"
    SEARCH_RELEVANCE = "search_relevance"


class FeedbackRating(Enum):
    EXCELLENT = 5
    GOOD = 4
    FAIR = 3
    POOR = 2
    VERY_POOR = 1


class UserFeedback(Base):
    """Model for storing user feedback on AI outputs"""
    __tablename__ = "user_feedback"

    id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    meeting_file_id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), nullable=False)
    feedback_type: Mapped[str] = mapped_column(String(50), nullable=False)
    component: Mapped[str] = mapped_column(String(100), nullable=False)  # transcription, summary, search, etc.
    rating: Mapped[int] = mapped_column(Integer, nullable=False)  # 1-5 scale
    feedback_text: Mapped[Optional[str]] = mapped_column(Text, nullable=True)
    specific_issues: Mapped[Optional[Dict[str, Any]]] = mapped_column(JSONB, nullable=True)
    suggestions: Mapped[Optional[str]] = mapped_column(Text, nullable=True)
    user_id: Mapped[str] = mapped_column(String(100), nullable=False)
    ai_confidence: Mapped[Optional[float]] = mapped_column(Float, nullable=True)
    correction_applied: Mapped[bool] = mapped_column(Boolean, default=False)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=sql_func.now())
    metadata: Mapped[Optional[Dict[str, Any]]] = mapped_column(JSONB, nullable=True)


class CorrectionPattern(Base):
    """Model for tracking correction patterns to improve AI"""
    __tablename__ = "correction_patterns"

    id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    pattern_type: Mapped[str] = mapped_column(String(50), nullable=False)
    original_text: Mapped[str] = mapped_column(Text, nullable=False)
    corrected_text: Mapped[str] = mapped_column(Text, nullable=False)
    context: Mapped[Optional[Dict[str, Any]]] = mapped_column(JSONB, nullable=True)
    frequency: Mapped[int] = mapped_column(Integer, default=1)
    confidence_impact: Mapped[Optional[float]] = mapped_column(Float, nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=sql_func.now())
    last_seen: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=sql_func.now())


class FeedbackSubmission(BaseModel):
    """Model for feedback submission"""
    meeting_id: str
    feedback_type: str
    component: str
    rating: int  # 1-5
    feedback_text: Optional[str] = None
    specific_issues: Optional[Dict[str, Any]] = None
    suggestions: Optional[str] = None
    ai_confidence: Optional[float] = None


class CorrectionSubmission(BaseModel):
    """Model for correction submission"""
    meeting_id: str
    pattern_type: str  # transcription_error, speaker_misidentification, etc.
    original_text: str
    corrected_text: str
    context: Optional[Dict[str, Any]] = None


class FeedbackCollectionService:
    """Service for collecting and analyzing user feedback"""

    def __init__(self):
        self.feedback_types = {
            FeedbackType.TRANSCRIPTION_QUALITY: {
                "name": "Transcription Quality",
                "description": "Accuracy of speech-to-text conversion",
                "common_issues": ["word_errors", "punctuation", "speaker_confusion", "background_noise"]
            },
            FeedbackType.SPEAKER_IDENTIFICATION: {
                "name": "Speaker Identification",
                "description": "Accuracy of speaker detection and labeling",
                "common_issues": ["wrong_speaker", "speaker_confusion", "missing_speaker", "duplicate_speakers"]
            },
            FeedbackType.SUMMARY_ACCURACY: {
                "name": "Summary Accuracy",
                "description": "Quality and accuracy of AI-generated summaries",
                "common_issues": ["missing_points", "incorrect_emphasis", "factual_errors", "too_brief"]
            },
            FeedbackType.KEY_POINTS_RELEVANCE: {
                "name": "Key Points Relevance",
                "description": "Relevance and completeness of extracted key points",
                "common_issues": ["irrelevant_points", "missed_important", "wrong_priority", "too_many"]
            },
            FeedbackType.ACTION_ITEMS_COMPLETENESS: {
                "name": "Action Items Completeness",
                "description": "Accuracy of action item extraction and assignment",
                "common_issues": ["missed_actions", "wrong_assignee", "unclear_deadline", "not_actionable"]
            },
            FeedbackType.ENTITY_RESOLUTION: {
                "name": "Entity Resolution",
                "description": "Accuracy of person and project linking",
                "common_issues": ["wrong_person", "missing_person", "wrong_project", "duplicate_entities"]
            },
            FeedbackType.SEARCH_RELEVANCE: {
                "name": "Search Relevance",
                "description": "Quality of search results and ranking",
                "common_issues": ["irrelevant_results", "missing_relevant", "poor_ranking", "slow_performance"]
            }
        }

    async def submit_feedback(
        self,
        feedback: FeedbackSubmission,
        user_id: str,
        db: AsyncSession
    ) -> Dict[str, Any]:
        """Submit user feedback on AI-generated content"""

        try:
            # Validate feedback type
            if feedback.feedback_type not in [ft.value for ft in FeedbackType]:
                raise ValueError(f"Invalid feedback type: {feedback.feedback_type}")

            # Validate rating
            if not 1 <= feedback.rating <= 5:
                raise ValueError("Rating must be between 1 and 5")

            # Create feedback record
            feedback_id = uuid.uuid4()
            stmt = insert(UserFeedback).values(
                id=feedback_id,
                meeting_file_id=uuid.UUID(feedback.meeting_id),
                feedback_type=feedback.feedback_type,
                component=feedback.component,
                rating=feedback.rating,
                feedback_text=feedback.feedback_text,
                specific_issues=feedback.specific_issues,
                suggestions=feedback.suggestions,
                user_id=user_id,
                ai_confidence=feedback.ai_confidence,
                metadata={
                    "submitted_via": "manual_review",
                    "ip_address": None,  # Would be captured from request
                    "user_agent": None  # Would be captured from request
                }
            )

            await db.execute(stmt)
            await db.commit()

            # Analyze feedback for patterns
            await self._analyze_feedback_patterns(feedback, db)

            return {
                "success": True,
                "feedback_id": str(feedback_id),
                "feedback_type": feedback.feedback_type,
                "component": feedback.component,
                "rating": feedback.rating,
                "submitted_by": user_id,
                "timestamp": datetime.utcnow().isoformat() + "Z",
                "message": "Thank you for your feedback! It will help improve AI accuracy."
            }

        except Exception as e:
            await db.rollback()
            raise RuntimeError(f"Failed to submit feedback: {str(e)}")

    async def submit_correction(
        self,
        correction: CorrectionSubmission,
        user_id: str,
        db: AsyncSession
    ) -> Dict[str, Any]:
        """Submit a correction pattern for AI training"""

        try:
            # Check if this pattern already exists
            stmt = select(CorrectionPattern).where(
                and_(
                    CorrectionPattern.pattern_type == correction.pattern_type,
                    CorrectionPattern.original_text == correction.original_text,
                    CorrectionPattern.corrected_text == correction.corrected_text
                )
            )

            result = await db.execute(stmt)
            existing_pattern = result.scalar_one_or_none()

            if existing_pattern:
                # Update frequency and last_seen
                stmt = update(CorrectionPattern).where(
                    CorrectionPattern.id == existing_pattern.id
                ).values(
                    frequency=existing_pattern.frequency + 1,
                    last_seen=datetime.utcnow(),
                    context={
                        **existing_pattern.context,
                        "latest_correction": {
                            "user_id": user_id,
                            "meeting_id": correction.meeting_id,
                            "timestamp": datetime.utcnow().isoformat()
                        }
                    }
                )
                await db.execute(stmt)
                pattern_id = existing_pattern.id
                action = "updated"

            else:
                # Create new pattern
                pattern_id = uuid.uuid4()
                stmt = insert(CorrectionPattern).values(
                    id=pattern_id,
                    pattern_type=correction.pattern_type,
                    original_text=correction.original_text,
                    corrected_text=correction.corrected_text,
                    context={
                        **correction.context,
                        "first_corrected_by": user_id,
                        "meeting_id": correction.meeting_id
                    },
                    frequency=1
                )
                await db.execute(stmt)
                action = "created"

            await db.commit()

            return {
                "success": True,
                "pattern_id": str(pattern_id),
                "pattern_type": correction.pattern_type,
                "action": action,
                "frequency": existing_pattern.frequency + 1 if existing_pattern else 1,
                "submitted_by": user_id,
                "timestamp": datetime.utcnow().isoformat() + "Z"
            }

        except Exception as e:
            await db.rollback()
            raise RuntimeError(f"Failed to submit correction: {str(e)}")

    async def get_feedback_analytics(
        self,
        db: AsyncSession,
        days_back: int = 30,
        component: str = None
    ) -> Dict[str, Any]:
        """Get analytics on user feedback for AI improvement"""

        try:
            cutoff_date = datetime.utcnow() - timedelta(days=days_back)

            # Base filters
            base_filters = [UserFeedback.created_at >= cutoff_date]
            if component:
                base_filters.append(UserFeedback.component == component)

            # Average rating by feedback type
            stmt = select(
                UserFeedback.feedback_type,
                sql_func.avg(UserFeedback.rating).label("avg_rating"),
                sql_func.count(UserFeedback.id).label("feedback_count")
            ).where(and_(*base_filters)).group_by(UserFeedback.feedback_type)

            result = await db.execute(stmt)
            rating_by_type = {}
            for row in result.fetchall():
                rating_by_type[row.feedback_type] = {
                    "average_rating": round(float(row.avg_rating), 2),
                    "feedback_count": row.feedback_count
                }

            # Rating distribution
            stmt = select(
                UserFeedback.rating,
                sql_func.count(UserFeedback.id).label("count")
            ).where(and_(*base_filters)).group_by(UserFeedback.rating)

            result = await db.execute(stmt)
            rating_distribution = {row.rating: row.count for row in result.fetchall()}

            # Component performance
            stmt = select(
                UserFeedback.component,
                sql_func.avg(UserFeedback.rating).label("avg_rating"),
                sql_func.count(UserFeedback.id).label("feedback_count")
            ).where(and_(*base_filters)).group_by(UserFeedback.component)

            result = await db.execute(stmt)
            component_performance = {}
            for row in result.fetchall():
                component_performance[row.component] = {
                    "average_rating": round(float(row.avg_rating), 2),
                    "feedback_count": row.feedback_count
                }

            # Common issues analysis
            common_issues = await self._analyze_common_issues(base_filters, db)

            # Improvement trends
            trends = await self._calculate_improvement_trends(cutoff_date, db)

            return {
                "period_days": days_back,
                "total_feedback": sum(item["feedback_count"] for item in rating_by_type.values()),
                "rating_by_type": rating_by_type,
                "rating_distribution": rating_distribution,
                "component_performance": component_performance,
                "common_issues": common_issues,
                "improvement_trends": trends,
                "generated_at": datetime.utcnow().isoformat() + "Z"
            }

        except Exception as e:
            raise RuntimeError(f"Failed to generate feedback analytics: {str(e)}")

    async def get_correction_patterns(
        self,
        db: AsyncSession,
        pattern_type: str = None,
        min_frequency: int = 2
    ) -> List[Dict[str, Any]]:
        """Get correction patterns for AI training"""

        try:
            filters = [CorrectionPattern.frequency >= min_frequency]
            if pattern_type:
                filters.append(CorrectionPattern.pattern_type == pattern_type)

            stmt = select(CorrectionPattern).where(
                and_(*filters)
            ).order_by(desc(CorrectionPattern.frequency))

            result = await db.execute(stmt)
            patterns = result.scalars().all()

            pattern_list = []
            for pattern in patterns:
                pattern_list.append({
                    "pattern_id": str(pattern.id),
                    "pattern_type": pattern.pattern_type,
                    "original_text": pattern.original_text,
                    "corrected_text": pattern.corrected_text,
                    "frequency": pattern.frequency,
                    "confidence_impact": pattern.confidence_impact,
                    "first_seen": pattern.created_at.isoformat() + "Z",
                    "last_seen": pattern.last_seen.isoformat() + "Z",
                    "context": pattern.context or {}
                })

            return pattern_list

        except Exception as e:
            raise RuntimeError(f"Failed to get correction patterns: {str(e)}")

    async def get_ai_improvement_recommendations(
        self,
        db: AsyncSession
    ) -> Dict[str, Any]:
        """Generate recommendations for AI model improvements"""

        try:
            # Get recent feedback analytics
            analytics = await self.get_feedback_analytics(db, days_back=30)

            # Get high-frequency correction patterns
            patterns = await self.get_correction_patterns(db, min_frequency=3)

            recommendations = []

            # Analyze rating performance
            for feedback_type, data in analytics["rating_by_type"].items():
                avg_rating = data["average_rating"]
                if avg_rating < 3.5:
                    recommendations.append({
                        "type": "performance_improvement",
                        "component": feedback_type,
                        "priority": "high" if avg_rating < 3.0 else "medium",
                        "issue": f"Low average rating: {avg_rating}/5",
                        "recommendation": f"Review and improve {feedback_type} algorithms",
                        "feedback_count": data["feedback_count"]
                    })

            # Analyze correction patterns
            pattern_groups = {}
            for pattern in patterns[:10]:  # Top 10 patterns
                pattern_type = pattern["pattern_type"]
                if pattern_type not in pattern_groups:
                    pattern_groups[pattern_type] = []
                pattern_groups[pattern_type].append(pattern)

            for pattern_type, type_patterns in pattern_groups.items():
                if len(type_patterns) >= 3:  # Multiple patterns of same type
                    total_frequency = sum(p["frequency"] for p in type_patterns)
                    recommendations.append({
                        "type": "pattern_correction",
                        "component": pattern_type,
                        "priority": "high" if total_frequency >= 10 else "medium",
                        "issue": f"{len(type_patterns)} frequent {pattern_type} patterns",
                        "recommendation": f"Retrain model to address {pattern_type} issues",
                        "pattern_count": len(type_patterns),
                        "total_frequency": total_frequency
                    })

            # Sort by priority
            priority_order = {"high": 0, "medium": 1, "low": 2}
            recommendations.sort(key=lambda x: priority_order[x["priority"]])

            return {
                "recommendations": recommendations,
                "analytics_summary": {
                    "total_feedback": analytics["total_feedback"],
                    "average_rating": round(sum(
                        data["average_rating"] * data["feedback_count"]
                        for data in analytics["rating_by_type"].values()
                    ) / analytics["total_feedback"], 2) if analytics["total_feedback"] > 0 else 0,
                    "pattern_count": len(patterns)
                },
                "generated_at": datetime.utcnow().isoformat() + "Z"
            }

        except Exception as e:
            raise RuntimeError(f"Failed to generate recommendations: {str(e)}")

    async def _analyze_feedback_patterns(
        self,
        feedback: FeedbackSubmission,
        db: AsyncSession
    ) -> None:
        """Analyze feedback for patterns that might indicate systematic issues"""

        # In a full implementation, this would perform pattern analysis
        # For now, just log significant feedback
        if feedback.rating <= 2:
            print(f"⚠️ Poor rating ({feedback.rating}/5) for {feedback.component} - {feedback.feedback_type}")

    async def _analyze_common_issues(
        self,
        base_filters: List,
        db: AsyncSession
    ) -> Dict[str, int]:
        """Analyze common issues mentioned in feedback"""

        try:
            # In a full implementation, this would use NLP to extract issues
            # For now, return placeholder data based on specific_issues field
            stmt = select(UserFeedback.specific_issues).where(
                and_(*base_filters, UserFeedback.specific_issues.isnot(None))
            )

            result = await db.execute(stmt)
            issue_counts = {}

            for row in result.fetchall():
                issues = row.specific_issues
                if isinstance(issues, dict):
                    for issue_type, details in issues.items():
                        issue_counts[issue_type] = issue_counts.get(issue_type, 0) + 1

            return dict(sorted(issue_counts.items(), key=lambda x: x[1], reverse=True)[:10])

        except Exception:
            return {}

    async def _calculate_improvement_trends(
        self,
        cutoff_date: datetime,
        db: AsyncSession
    ) -> Dict[str, Any]:
        """Calculate improvement trends over time"""

        try:
            # Compare last 2 weeks vs previous 2 weeks
            mid_date = cutoff_date + timedelta(days=15)

            # Recent period
            stmt = select(sql_func.avg(UserFeedback.rating)).where(
                UserFeedback.created_at >= mid_date
            )
            result = await db.execute(stmt)
            recent_avg = result.scalar()

            # Earlier period
            stmt = select(sql_func.avg(UserFeedback.rating)).where(
                and_(
                    UserFeedback.created_at >= cutoff_date,
                    UserFeedback.created_at < mid_date
                )
            )
            result = await db.execute(stmt)
            earlier_avg = result.scalar()

            if recent_avg and earlier_avg:
                trend = round(float(recent_avg) - float(earlier_avg), 2)
                trend_direction = "improving" if trend > 0 else "declining" if trend < 0 else "stable"
            else:
                trend = 0
                trend_direction = "insufficient_data"

            return {
                "recent_average": round(float(recent_avg), 2) if recent_avg else None,
                "earlier_average": round(float(earlier_avg), 2) if earlier_avg else None,
                "trend": trend,
                "trend_direction": trend_direction
            }

        except Exception:
            return {"trend_direction": "calculation_failed"}


# Global feedback collection service
feedback_service = FeedbackCollectionService()