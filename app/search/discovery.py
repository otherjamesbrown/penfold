"""
Meeting Discovery and Recommendation Features

Provides intelligent meeting discovery, trending topics, and personalized
recommendations based on user behavior and content analysis.
"""
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, func, desc, and_, or_
from typing import List, Dict, Any, Optional, Set, Tuple
from datetime import datetime, timedelta
from collections import defaultdict, Counter
import numpy as np

from app.database import MeetingFile, MeetingTranscript, MeetingSummary, MeetingTopic, MeetingParticipant, MeetingInsight
from app.search.semantic import semantic_search
from app.search.fulltext import fulltext_search


class MeetingDiscoveryService:
    """Service for meeting discovery and recommendation features"""

    def __init__(self):
        self.trending_window_days = 7
        self.recommendation_limit = 20
        self.min_similarity_threshold = 0.6
        self.recent_activity_days = 30

    async def get_trending_topics(
        self,
        db: AsyncSession,
        privacy_levels: List[str],
        days_back: int = None,
        limit: int = 10
    ) -> List[Dict[str, Any]]:
        """Get trending topics from recent meetings"""

        days = days_back or self.trending_window_days
        cutoff_date = datetime.utcnow() - timedelta(days=days)

        try:
            # Get topics from recent meetings
            stmt = select(
                MeetingTopic.topic_name,
                MeetingTopic.topic_category,
                func.count(MeetingTopic.id).label("mention_count"),
                func.avg(MeetingTopic.relevance_score).label("avg_relevance"),
                func.max(MeetingTopic.last_mentioned).label("last_mentioned"),
                func.count(func.distinct(MeetingTopic.meeting_file_id)).label("meeting_count")
            ).select_from(
                MeetingTopic.join(MeetingFile, MeetingTopic.meeting_file_id == MeetingFile.id)
            ).where(
                and_(
                    MeetingFile.privacy_level.in_(privacy_levels),
                    MeetingFile.created_at >= cutoff_date,
                    MeetingTopic.relevance_score >= 0.5  # Filter for relevant topics
                )
            ).group_by(
                MeetingTopic.topic_name,
                MeetingTopic.topic_category
            ).order_by(
                desc(func.count(MeetingTopic.id)),
                desc(func.avg(MeetingTopic.relevance_score))
            ).limit(limit)

            result = await db.execute(stmt)
            rows = result.fetchall()

            trending_topics = []
            for row in rows:
                # Calculate trend score based on mentions and recency
                trend_score = self._calculate_trend_score(
                    row.mention_count,
                    row.meeting_count,
                    row.avg_relevance,
                    row.last_mentioned
                )

                trending_topics.append({
                    "topic": row.topic_name,
                    "category": row.topic_category,
                    "mention_count": row.mention_count,
                    "meeting_count": row.meeting_count,
                    "average_relevance": round(float(row.avg_relevance), 2),
                    "last_mentioned": row.last_mentioned.isoformat() + "Z" if row.last_mentioned else None,
                    "trend_score": trend_score
                })

            # Sort by trend score
            trending_topics.sort(key=lambda x: x["trend_score"], reverse=True)

            print(f"📈 Found {len(trending_topics)} trending topics in last {days} days")
            return trending_topics

        except Exception as e:
            print(f"❌ Trending topics query failed: {str(e)}")
            return []

    async def get_meeting_recommendations(
        self,
        user_id: str,
        db: AsyncSession,
        privacy_levels: List[str],
        limit: int = None,
        recommendation_types: List[str] = None
    ) -> Dict[str, List[Dict[str, Any]]]:
        """Get personalized meeting recommendations for a user"""

        limit = limit or self.recommendation_limit

        # Default recommendation types
        if recommendation_types is None:
            recommendation_types = ["similar_content", "trending_topics", "recent_activity", "followed_participants"]

        recommendations = {}

        try:
            # Similar content recommendations (based on recent meetings)
            if "similar_content" in recommendation_types:
                recommendations["similar_content"] = await self._get_similar_content_recommendations(
                    user_id, db, privacy_levels, limit // 2
                )

            # Trending topic recommendations
            if "trending_topics" in recommendation_types:
                recommendations["trending_topics"] = await self._get_trending_recommendations(
                    db, privacy_levels, limit // 3
                )

            # Recent activity recommendations
            if "recent_activity" in recommendation_types:
                recommendations["recent_activity"] = await self._get_recent_activity_recommendations(
                    db, privacy_levels, limit // 3
                )

            # Followed participants recommendations
            if "followed_participants" in recommendation_types:
                recommendations["followed_participants"] = await self._get_participant_recommendations(
                    user_id, db, privacy_levels, limit // 4
                )

            print(f"🎯 Generated {len(recommendations)} recommendation categories for user {user_id}")
            return recommendations

        except Exception as e:
            print(f"❌ Recommendations failed: {str(e)}")
            return {}

    async def discover_related_meetings_by_topic(
        self,
        topic_name: str,
        db: AsyncSession,
        privacy_levels: List[str],
        limit: int = 20,
        min_relevance: float = 0.6
    ) -> List[Dict[str, Any]]:
        """Discover meetings related to a specific topic"""

        try:
            # Find meetings that discuss this topic
            stmt = select(
                MeetingTopic.meeting_file_id,
                MeetingTopic.relevance_score,
                MeetingTopic.context_snippets,
                MeetingFile.filename,
                MeetingFile.created_at,
                MeetingFile.privacy_level,
                MeetingTranscript.confidence_score
            ).select_from(
                MeetingTopic
                .join(MeetingFile, MeetingTopic.meeting_file_id == MeetingFile.id)
                .outerjoin(MeetingTranscript, MeetingFile.id == MeetingTranscript.meeting_file_id)
            ).where(
                and_(
                    MeetingFile.privacy_level.in_(privacy_levels),
                    MeetingTopic.topic_name.icontains(topic_name),
                    MeetingTopic.relevance_score >= min_relevance
                )
            ).order_by(
                desc(MeetingTopic.relevance_score),
                desc(MeetingFile.created_at)
            ).limit(limit)

            result = await db.execute(stmt)
            rows = result.fetchall()

            related_meetings = []
            for row in rows:
                related_meetings.append({
                    "meeting_id": str(row.meeting_file_id),
                    "filename": row.filename,
                    "topic_relevance": round(float(row.relevance_score), 2),
                    "confidence_score": float(row.confidence_score) if row.confidence_score else None,
                    "privacy_level": row.privacy_level,
                    "created_at": row.created_at.isoformat() + "Z",
                    "context_snippets": row.context_snippets or {},
                    "discovery_type": "topic_based"
                })

            print(f"🔍 Found {len(related_meetings)} meetings related to topic: {topic_name}")
            return related_meetings

        except Exception as e:
            print(f"❌ Topic-based discovery failed: {str(e)}")
            return []

    async def get_meeting_insights_summary(
        self,
        db: AsyncSession,
        privacy_levels: List[str],
        days_back: int = 7,
        insight_types: List[str] = None
    ) -> Dict[str, Any]:
        """Get aggregated insights from recent meetings"""

        cutoff_date = datetime.utcnow() - timedelta(days=days_back)

        try:
            # Default insight types
            if insight_types is None:
                insight_types = ["risk", "opportunity", "decision", "action_required"]

            # Get insights grouped by type
            stmt = select(
                MeetingInsight.insight_type,
                func.count(MeetingInsight.id).label("count"),
                func.avg(MeetingInsight.confidence_score).label("avg_confidence")
            ).select_from(
                MeetingInsight.join(MeetingFile, MeetingInsight.meeting_file_id == MeetingFile.id)
            ).where(
                and_(
                    MeetingFile.privacy_level.in_(privacy_levels),
                    MeetingFile.created_at >= cutoff_date,
                    MeetingInsight.insight_type.in_(insight_types)
                )
            ).group_by(MeetingInsight.insight_type)

            result = await db.execute(stmt)
            insights_summary = {row.insight_type: {"count": row.count, "confidence": round(float(row.avg_confidence), 2)}
                             for row in result.fetchall()}

            # Get top insights by type
            top_insights = {}
            for insight_type in insight_types:
                stmt = select(
                    MeetingInsight.insight_text,
                    MeetingInsight.confidence_score,
                    MeetingInsight.supporting_evidence,
                    MeetingFile.filename,
                    MeetingFile.created_at
                ).select_from(
                    MeetingInsight.join(MeetingFile, MeetingInsight.meeting_file_id == MeetingFile.id)
                ).where(
                    and_(
                        MeetingFile.privacy_level.in_(privacy_levels),
                        MeetingFile.created_at >= cutoff_date,
                        MeetingInsight.insight_type == insight_type
                    )
                ).order_by(
                    desc(MeetingInsight.confidence_score)
                ).limit(3)

                result = await db.execute(stmt)
                top_insights[insight_type] = [
                    {
                        "insight": row.insight_text,
                        "confidence": round(float(row.confidence_score), 2),
                        "evidence": row.supporting_evidence,
                        "source_meeting": row.filename,
                        "date": row.created_at.isoformat() + "Z"
                    }
                    for row in result.fetchall()
                ]

            return {
                "period_days": days_back,
                "insights_summary": insights_summary,
                "top_insights": top_insights,
                "generated_at": datetime.utcnow().isoformat() + "Z"
            }

        except Exception as e:
            print(f"❌ Insights summary failed: {str(e)}")
            return {"error": str(e)}

    async def find_meetings_by_participants(
        self,
        participant_names: List[str],
        db: AsyncSession,
        privacy_levels: List[str],
        require_all: bool = False,
        limit: int = 20
    ) -> List[Dict[str, Any]]:
        """Find meetings that include specific participants"""

        if not participant_names:
            return []

        try:
            # Build participant filter based on whether all participants are required
            participant_conditions = []
            for name in participant_names:
                # Search in speaker segments for participant names
                condition = func.jsonb_path_exists(
                    MeetingTranscript.speaker_segments,
                    f'$[*] ? (@.speaker like_regex "{name}" flag "i")'
                )
                participant_conditions.append(condition)

            # Combine conditions
            if require_all:
                participant_filter = and_(*participant_conditions)
            else:
                participant_filter = or_(*participant_conditions)

            stmt = select(
                MeetingTranscript.meeting_file_id,
                MeetingFile.filename,
                MeetingFile.created_at,
                MeetingFile.privacy_level,
                MeetingTranscript.speaker_segments,
                func.count().over().label("total_matches")
            ).select_from(
                MeetingTranscript.join(MeetingFile, MeetingTranscript.meeting_file_id == MeetingFile.id)
            ).where(
                and_(
                    MeetingFile.privacy_level.in_(privacy_levels),
                    MeetingTranscript.speaker_segments.isnot(None),
                    participant_filter
                )
            ).order_by(
                desc(MeetingFile.created_at)
            ).limit(limit)

            result = await db.execute(stmt)
            rows = result.fetchall()

            meetings = []
            for row in rows:
                # Extract matching participants
                matching_participants = self._extract_matching_participants(
                    row.speaker_segments, participant_names
                )

                meetings.append({
                    "meeting_id": str(row.meeting_file_id),
                    "filename": row.filename,
                    "created_at": row.created_at.isoformat() + "Z",
                    "privacy_level": row.privacy_level,
                    "matching_participants": matching_participants,
                    "search_participants": participant_names,
                    "discovery_type": "participant_based"
                })

            print(f"👥 Found {len(meetings)} meetings with participants: {participant_names}")
            return meetings

        except Exception as e:
            print(f"❌ Participant-based discovery failed: {str(e)}")
            return []

    async def get_meeting_activity_timeline(
        self,
        db: AsyncSession,
        privacy_levels: List[str],
        days_back: int = 30,
        granularity: str = "daily"  # daily, weekly
    ) -> Dict[str, Any]:
        """Get timeline of meeting activity for visualization"""

        cutoff_date = datetime.utcnow() - timedelta(days=days_back)

        try:
            # Determine date grouping based on granularity
            if granularity == "weekly":
                date_trunc = func.date_trunc("week", MeetingFile.created_at)
            else:
                date_trunc = func.date_trunc("day", MeetingFile.created_at)

            stmt = select(
                date_trunc.label("period"),
                func.count(MeetingFile.id).label("meeting_count"),
                func.avg(MeetingTranscript.confidence_score).label("avg_quality"),
                func.count(func.distinct(MeetingParticipant.person_id)).label("unique_participants")
            ).select_from(
                MeetingFile
                .outerjoin(MeetingTranscript, MeetingFile.id == MeetingTranscript.meeting_file_id)
                .outerjoin(MeetingParticipant, MeetingFile.id == MeetingParticipant.meeting_file_id)
            ).where(
                and_(
                    MeetingFile.privacy_level.in_(privacy_levels),
                    MeetingFile.created_at >= cutoff_date,
                    MeetingFile.upload_completed_at.isnot(None)
                )
            ).group_by(
                date_trunc
            ).order_by(
                date_trunc
            )

            result = await db.execute(stmt)
            rows = result.fetchall()

            timeline_data = []
            for row in rows:
                timeline_data.append({
                    "period": row.period.isoformat() if row.period else None,
                    "meeting_count": row.meeting_count,
                    "average_quality": round(float(row.avg_quality), 2) if row.avg_quality else 0,
                    "unique_participants": row.unique_participants or 0
                })

            return {
                "granularity": granularity,
                "period_days": days_back,
                "timeline": timeline_data,
                "total_meetings": sum(item["meeting_count"] for item in timeline_data),
                "generated_at": datetime.utcnow().isoformat() + "Z"
            }

        except Exception as e:
            print(f"❌ Timeline query failed: {str(e)}")
            return {"error": str(e)}

    async def _get_similar_content_recommendations(
        self,
        user_id: str,
        db: AsyncSession,
        privacy_levels: List[str],
        limit: int
    ) -> List[Dict[str, Any]]:
        """Get recommendations based on similar content to user's recent activity"""

        # In a full implementation, this would track user's recent searches/views
        # For now, we'll use recent meetings as a proxy

        try:
            # Get recent meetings to find similar content
            cutoff_date = datetime.utcnow() - timedelta(days=self.recent_activity_days)

            stmt = select(MeetingFile.id).where(
                and_(
                    MeetingFile.privacy_level.in_(privacy_levels),
                    MeetingFile.created_at >= cutoff_date
                )
            ).order_by(desc(MeetingFile.created_at)).limit(5)

            result = await db.execute(stmt)
            recent_meeting_ids = [str(row.id) for row in result.fetchall()]

            # Find similar meetings for each recent meeting
            similar_meetings = []
            for meeting_id in recent_meeting_ids:
                similar = await semantic_search.find_similar_meetings(
                    meeting_id, db, limit=3, similarity_threshold=self.min_similarity_threshold
                )
                similar_meetings.extend(similar)

            # Remove duplicates and limit
            seen_ids = set()
            unique_similar = []
            for meeting in similar_meetings:
                meeting_id = meeting["meeting_id"]
                if meeting_id not in seen_ids:
                    seen_ids.add(meeting_id)
                    meeting["recommendation_reason"] = "Similar to your recent activity"
                    unique_similar.append(meeting)

            return unique_similar[:limit]

        except Exception as e:
            print(f"❌ Similar content recommendations failed: {str(e)}")
            return []

    async def _get_trending_recommendations(
        self,
        db: AsyncSession,
        privacy_levels: List[str],
        limit: int
    ) -> List[Dict[str, Any]]:
        """Get recommendations based on trending topics"""

        try:
            trending_topics = await self.get_trending_topics(db, privacy_levels, limit=5)

            trending_meetings = []
            for topic_info in trending_topics[:3]:  # Top 3 trending topics
                topic_meetings = await self.discover_related_meetings_by_topic(
                    topic_info["topic"], db, privacy_levels, limit=3
                )
                for meeting in topic_meetings:
                    meeting["recommendation_reason"] = f"Trending topic: {topic_info['topic']}"
                    trending_meetings.append(meeting)

            return trending_meetings[:limit]

        except Exception as e:
            print(f"❌ Trending recommendations failed: {str(e)}")
            return []

    async def _get_recent_activity_recommendations(
        self,
        db: AsyncSession,
        privacy_levels: List[str],
        limit: int
    ) -> List[Dict[str, Any]]:
        """Get recommendations based on recent high-quality meetings"""

        try:
            cutoff_date = datetime.utcnow() - timedelta(days=3)  # Very recent

            stmt = select(
                MeetingFile.id,
                MeetingFile.filename,
                MeetingFile.created_at,
                MeetingFile.privacy_level,
                MeetingTranscript.confidence_score,
                MeetingSummary.quality_score
            ).select_from(
                MeetingFile
                .join(MeetingTranscript, MeetingFile.id == MeetingTranscript.meeting_file_id)
                .join(MeetingSummary, MeetingFile.id == MeetingSummary.meeting_file_id)
            ).where(
                and_(
                    MeetingFile.privacy_level.in_(privacy_levels),
                    MeetingFile.created_at >= cutoff_date,
                    MeetingTranscript.confidence_score >= 0.8,  # High quality transcripts
                    MeetingSummary.quality_score >= 0.8        # High quality summaries
                )
            ).order_by(
                desc(MeetingSummary.quality_score),
                desc(MeetingFile.created_at)
            ).limit(limit)

            result = await db.execute(stmt)
            rows = result.fetchall()

            recent_meetings = []
            for row in rows:
                recent_meetings.append({
                    "meeting_id": str(row.id),
                    "filename": row.filename,
                    "created_at": row.created_at.isoformat() + "Z",
                    "privacy_level": row.privacy_level,
                    "confidence_score": float(row.confidence_score),
                    "quality_score": float(row.quality_score),
                    "recommendation_reason": "High-quality recent meeting",
                    "discovery_type": "recent_activity"
                })

            return recent_meetings

        except Exception as e:
            print(f"❌ Recent activity recommendations failed: {str(e)}")
            return []

    async def _get_participant_recommendations(
        self,
        user_id: str,
        db: AsyncSession,
        privacy_levels: List[str],
        limit: int
    ) -> List[Dict[str, Any]]:
        """Get recommendations based on frequently appearing participants"""

        # This would integrate with a user preference system in production
        # For now, return empty list as we don't have user preference tracking
        return []

    def _calculate_trend_score(
        self,
        mention_count: int,
        meeting_count: int,
        avg_relevance: float,
        last_mentioned: datetime
    ) -> float:
        """Calculate trend score for a topic"""

        # Base score from activity
        activity_score = min(mention_count * 0.1 + meeting_count * 0.2, 1.0)

        # Relevance factor
        relevance_factor = avg_relevance

        # Recency factor (higher for more recent mentions)
        if last_mentioned:
            hours_ago = (datetime.utcnow() - last_mentioned).total_seconds() / 3600
            recency_factor = max(0.1, 1.0 - (hours_ago / (24 * 7)))  # Decay over a week
        else:
            recency_factor = 0.1

        # Combined trend score
        trend_score = activity_score * 0.4 + relevance_factor * 0.4 + recency_factor * 0.2

        return round(trend_score, 3)

    def _extract_matching_participants(
        self,
        speaker_segments: List[Dict],
        participant_names: List[str]
    ) -> List[Dict[str, Any]]:
        """Extract participant information that matches the search criteria"""

        if not speaker_segments or not participant_names:
            return []

        matching = []
        for segment in speaker_segments:
            if isinstance(segment, dict):
                speaker = segment.get("speaker", "").lower()
                for name in participant_names:
                    if name.lower() in speaker:
                        matching.append({
                            "speaker_label": segment.get("speaker", ""),
                            "matched_name": name,
                            "confidence": "high" if name.lower() == speaker else "medium"
                        })
                        break

        # Deduplicate while preserving order
        seen = set()
        unique_matching = []
        for item in matching:
            key = (item["speaker_label"], item["matched_name"])
            if key not in seen:
                seen.add(key)
                unique_matching.append(item)

        return unique_matching


# Global discovery service
discovery_service = MeetingDiscoveryService()