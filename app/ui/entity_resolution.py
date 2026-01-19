"""
Manual Entity Resolution Interface

Provides interface for resolving failed automatic entity linking,
managing provisional entities, and manual person/project assignment.
"""
from fastapi import APIRouter, HTTPException, Depends, Query
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, update, insert, and_, or_
from pydantic import BaseModel
from typing import Dict, Any, List, Optional
from datetime import datetime
import uuid

from app.database import (
    get_db, MeetingParticipant, MeetingTopic, ReviewQueue,
    MeetingFile, MeetingTranscript
)
from app.search.privacy import privacy_filter, UserRole

router = APIRouter()


class EntityResolution(BaseModel):
    """Model for entity resolution operations"""
    entity_type: str  # participant, topic, project
    provisional_id: str
    resolution_type: str  # link_existing, create_new, mark_invalid
    target_id: Optional[str] = None  # ID of existing entity to link to
    new_entity_data: Optional[Dict[str, Any]] = None
    confidence: float = 1.0
    notes: Optional[str] = None


class ParticipantResolution(BaseModel):
    """Model for participant resolution"""
    participant_id: str
    person_id: Optional[str] = None
    canonical_name: str
    email: Optional[str] = None
    department: Optional[str] = None
    confidence: float = 1.0


class TopicResolution(BaseModel):
    """Model for topic resolution"""
    topic_id: str
    project_id: Optional[str] = None
    canonical_topic: str
    category: str
    confidence: float = 1.0


class EntityResolutionService:
    """Service for manual entity resolution operations"""

    def __init__(self):
        self.resolution_types = {
            "link_existing": "Link to existing entity",
            "create_new": "Create new entity",
            "mark_invalid": "Mark as invalid/ignore",
            "merge_entities": "Merge with another entity"
        }

    async def get_pending_resolutions(
        self,
        db: AsyncSession,
        entity_types: List[str] = None,
        limit: int = 50,
        priority: str = "all"
    ) -> Dict[str, Any]:
        """Get pending entity resolutions from review queue"""

        try:
            # Default entity types
            if entity_types is None:
                entity_types = ["speaker_identification", "project_linking", "topic_resolution"]

            # Build query filters
            filters = [ReviewQueue.status == "pending"]
            if entity_types:
                filters.append(ReviewQueue.review_type.in_(entity_types))

            if priority != "all":
                # Priority would be determined by AI confidence scores
                pass

            stmt = select(ReviewQueue).where(and_(*filters)).limit(limit)
            result = await db.execute(stmt)
            pending_items = result.scalars().all()

            # Group by entity type
            resolutions_by_type = {
                "speaker_identification": [],
                "project_linking": [],
                "topic_resolution": []
            }

            for item in pending_items:
                resolution_data = await self._format_resolution_item(item, db)
                if item.review_type in resolutions_by_type:
                    resolutions_by_type[item.review_type].append(resolution_data)

            # Get summary statistics
            stats = {
                "total_pending": len(pending_items),
                "by_type": {
                    entity_type: len(resolutions_by_type[entity_type])
                    for entity_type in resolutions_by_type
                },
                "oldest_pending": None,
                "avg_confidence": 0.0
            }

            if pending_items:
                oldest_item = min(pending_items, key=lambda x: x.created_at)
                stats["oldest_pending"] = oldest_item.created_at.isoformat() + "Z"

                # Calculate average AI confidence
                confidences = []
                for item in pending_items:
                    ai_suggestions = item.ai_suggestions or {}
                    if "confidence" in ai_suggestions:
                        confidences.append(ai_suggestions["confidence"])

                if confidences:
                    stats["avg_confidence"] = round(sum(confidences) / len(confidences), 2)

            return {
                "pending_resolutions": resolutions_by_type,
                "statistics": stats,
                "resolution_types": self.resolution_types
            }

        except Exception as e:
            raise HTTPException(status_code=500, detail=f"Failed to get pending resolutions: {str(e)}")

    async def resolve_participant(
        self,
        resolution: ParticipantResolution,
        user_id: str,
        db: AsyncSession
    ) -> Dict[str, Any]:
        """Resolve participant entity linking"""

        try:
            # Get the participant record
            stmt = select(MeetingParticipant).where(
                MeetingParticipant.id == uuid.UUID(resolution.participant_id)
            )
            result = await db.execute(stmt)
            participant = result.scalar_one_or_none()

            if not participant:
                raise HTTPException(status_code=404, detail="Participant not found")

            # Update participant with resolution
            stmt = update(MeetingParticipant).where(
                MeetingParticipant.id == uuid.UUID(resolution.participant_id)
            ).values(
                person_id=uuid.UUID(resolution.person_id) if resolution.person_id else None,
                identification_confidence=resolution.confidence,
                is_provisional=False,  # Mark as resolved
                contribution_stats={
                    **participant.contribution_stats,
                    "manually_resolved": True,
                    "resolved_by": user_id,
                    "resolved_at": datetime.utcnow().isoformat(),
                    "canonical_name": resolution.canonical_name,
                    "email": resolution.email,
                    "department": resolution.department
                }
            )

            await db.execute(stmt)

            # Remove from review queue
            await self._remove_from_review_queue(
                str(participant.meeting_file_id), "speaker_identification", db
            )

            # Create resolution record
            await self._create_resolution_record(
                str(participant.meeting_file_id),
                "participant",
                resolution.participant_id,
                user_id,
                "linked" if resolution.person_id else "marked_invalid",
                db
            )

            await db.commit()

            return {
                "success": True,
                "participant_id": resolution.participant_id,
                "person_id": resolution.person_id,
                "canonical_name": resolution.canonical_name,
                "confidence": resolution.confidence,
                "resolution_type": "linked" if resolution.person_id else "marked_invalid",
                "resolved_by": user_id,
                "timestamp": datetime.utcnow().isoformat() + "Z"
            }

        except HTTPException:
            raise
        except Exception as e:
            await db.rollback()
            raise HTTPException(status_code=500, detail=f"Participant resolution failed: {str(e)}")

    async def resolve_topic(
        self,
        resolution: TopicResolution,
        user_id: str,
        db: AsyncSession
    ) -> Dict[str, Any]:
        """Resolve topic/project linking"""

        try:
            # Get the topic record
            stmt = select(MeetingTopic).where(
                MeetingTopic.id == uuid.UUID(resolution.topic_id)
            )
            result = await db.execute(stmt)
            topic = result.scalar_one_or_none()

            if not topic:
                raise HTTPException(status_code=404, detail="Topic not found")

            # Update topic with resolution
            stmt = update(MeetingTopic).where(
                MeetingTopic.id == uuid.UUID(resolution.topic_id)
            ).values(
                project_id=uuid.UUID(resolution.project_id) if resolution.project_id else None,
                topic_name=resolution.canonical_topic,
                topic_category=resolution.category,
                relevance_score=resolution.confidence,
                context_snippets={
                    **topic.context_snippets,
                    "manually_resolved": True,
                    "resolved_by": user_id,
                    "resolved_at": datetime.utcnow().isoformat(),
                    "original_topic": topic.topic_name
                }
            )

            await db.execute(stmt)

            # Remove from review queue
            await self._remove_from_review_queue(
                str(topic.meeting_file_id), "project_linking", db
            )

            # Create resolution record
            await self._create_resolution_record(
                str(topic.meeting_file_id),
                "topic",
                resolution.topic_id,
                user_id,
                "linked" if resolution.project_id else "standardized",
                db
            )

            await db.commit()

            return {
                "success": True,
                "topic_id": resolution.topic_id,
                "project_id": resolution.project_id,
                "canonical_topic": resolution.canonical_topic,
                "category": resolution.category,
                "confidence": resolution.confidence,
                "resolution_type": "linked" if resolution.project_id else "standardized",
                "resolved_by": user_id,
                "timestamp": datetime.utcnow().isoformat() + "Z"
            }

        except HTTPException:
            raise
        except Exception as e:
            await db.rollback()
            raise HTTPException(status_code=500, detail=f"Topic resolution failed: {str(e)}")

    async def bulk_resolve_entities(
        self,
        resolutions: List[EntityResolution],
        user_id: str,
        db: AsyncSession
    ) -> Dict[str, Any]:
        """Apply multiple entity resolutions in batch"""

        try:
            successful_resolutions = []
            failed_resolutions = []

            for resolution in resolutions:
                try:
                    if resolution.entity_type == "participant":
                        # Convert to ParticipantResolution
                        participant_resolution = ParticipantResolution(
                            participant_id=resolution.provisional_id,
                            person_id=resolution.target_id,
                            canonical_name=resolution.new_entity_data.get("name", "Unknown") if resolution.new_entity_data else "Unknown",
                            confidence=resolution.confidence
                        )
                        result = await self.resolve_participant(participant_resolution, user_id, db)
                        successful_resolutions.append({
                            "type": "participant",
                            "id": resolution.provisional_id,
                            "result": result
                        })

                    elif resolution.entity_type == "topic":
                        # Convert to TopicResolution
                        topic_resolution = TopicResolution(
                            topic_id=resolution.provisional_id,
                            project_id=resolution.target_id,
                            canonical_topic=resolution.new_entity_data.get("topic", "Unknown") if resolution.new_entity_data else "Unknown",
                            category=resolution.new_entity_data.get("category", "general") if resolution.new_entity_data else "general",
                            confidence=resolution.confidence
                        )
                        result = await self.resolve_topic(topic_resolution, user_id, db)
                        successful_resolutions.append({
                            "type": "topic",
                            "id": resolution.provisional_id,
                            "result": result
                        })

                except Exception as e:
                    failed_resolutions.append({
                        "type": resolution.entity_type,
                        "id": resolution.provisional_id,
                        "error": str(e)
                    })

            return {
                "success": len(successful_resolutions) > 0,
                "successful_count": len(successful_resolutions),
                "failed_count": len(failed_resolutions),
                "successful_resolutions": successful_resolutions,
                "failed_resolutions": failed_resolutions,
                "processed_by": user_id,
                "timestamp": datetime.utcnow().isoformat() + "Z"
            }

        except Exception as e:
            await db.rollback()
            raise HTTPException(status_code=500, detail=f"Bulk resolution failed: {str(e)}")

    async def get_resolution_suggestions(
        self,
        entity_type: str,
        query: str,
        db: AsyncSession,
        limit: int = 10
    ) -> List[Dict[str, Any]]:
        """Get suggestions for entity resolution based on query"""

        try:
            suggestions = []

            if entity_type == "participant":
                # In a full implementation, this would search existing Person entities
                # For now, return mock suggestions
                suggestions = [
                    {
                        "id": f"person_{i}",
                        "name": f"John Doe {i}",
                        "email": f"john.doe{i}@company.com",
                        "department": "Engineering",
                        "confidence": 0.9 - (i * 0.1)
                    }
                    for i in range(1, min(limit + 1, 4))
                    if query.lower() in f"john doe {i}".lower()
                ]

            elif entity_type == "topic":
                # Mock project suggestions
                projects = [
                    {"id": "proj_1", "name": "Platform Redesign", "category": "engineering"},
                    {"id": "proj_2", "name": "Mobile App", "category": "product"},
                    {"id": "proj_3", "name": "Data Migration", "category": "infrastructure"}
                ]

                suggestions = [
                    {
                        "id": project["id"],
                        "name": project["name"],
                        "category": project["category"],
                        "confidence": 0.8
                    }
                    for project in projects
                    if query.lower() in project["name"].lower()
                ][:limit]

            return suggestions

        except Exception as e:
            print(f"⚠️ Failed to get suggestions: {str(e)}")
            return []

    async def _format_resolution_item(
        self,
        review_item: ReviewQueue,
        db: AsyncSession
    ) -> Dict[str, Any]:
        """Format a review queue item for the resolution interface"""

        # Get meeting information
        stmt = select(MeetingFile.filename, MeetingFile.created_at).where(
            MeetingFile.id == review_item.meeting_file_id
        )
        result = await db.execute(stmt)
        meeting_info = result.first()

        ai_suggestions = review_item.ai_suggestions or {}

        base_data = {
            "review_id": str(review_item.id),
            "meeting_id": str(review_item.meeting_file_id),
            "meeting_filename": meeting_info.filename if meeting_info else "Unknown",
            "meeting_date": meeting_info.created_at.isoformat() + "Z" if meeting_info else None,
            "review_type": review_item.review_type,
            "status": review_item.status,
            "created_at": review_item.created_at.isoformat() + "Z",
            "ai_suggestions": ai_suggestions,
            "confidence": ai_suggestions.get("confidence", 0.0)
        }

        # Add type-specific data
        if review_item.review_type == "speaker_identification":
            base_data.update({
                "provisional_speaker": ai_suggestions.get("speaker_label", "Unknown"),
                "suggested_matches": ai_suggestions.get("person_matches", []),
                "voice_confidence": ai_suggestions.get("voice_confidence", 0.0)
            })

        elif review_item.review_type == "project_linking":
            base_data.update({
                "topic_name": ai_suggestions.get("topic", "Unknown"),
                "suggested_projects": ai_suggestions.get("project_matches", []),
                "relevance_score": ai_suggestions.get("relevance", 0.0)
            })

        return base_data

    async def _remove_from_review_queue(
        self,
        meeting_id: str,
        review_type: str,
        db: AsyncSession
    ) -> None:
        """Remove resolved item from review queue"""

        from sqlalchemy import delete

        stmt = delete(ReviewQueue).where(
            and_(
                ReviewQueue.meeting_file_id == uuid.UUID(meeting_id),
                ReviewQueue.review_type == review_type,
                ReviewQueue.status == "pending"
            )
        )

        await db.execute(stmt)

    async def _create_resolution_record(
        self,
        meeting_id: str,
        entity_type: str,
        entity_id: str,
        user_id: str,
        resolution_type: str,
        db: AsyncSession
    ) -> None:
        """Create a record of the manual resolution for audit purposes"""

        # In a full implementation, this would create an EntityResolution record
        # For now, we'll just log the resolution
        print(f"📋 Entity resolution: {entity_type} {entity_id} in meeting {meeting_id} resolved as {resolution_type} by {user_id}")


# Global entity resolution service
entity_resolution_service = EntityResolutionService()


# API Routes for entity resolution

@router.get("/entity-resolution/pending")
async def get_pending_entity_resolutions(
    entity_types: List[str] = Query(None, description="Filter by entity types"),
    limit: int = Query(50, description="Maximum number of items"),
    priority: str = Query("all", description="Priority filter"),
    db: AsyncSession = Depends(get_db)
):
    """Get pending entity resolutions from review queue"""

    try:
        pending_resolutions = await entity_resolution_service.get_pending_resolutions(
            db, entity_types, limit, priority
        )
        return pending_resolutions

    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.post("/entity-resolution/participant")
async def resolve_participant_entity(
    resolution: ParticipantResolution,
    db: AsyncSession = Depends(get_db)
):
    """Resolve participant entity linking"""

    # Mock user for demo
    user_id = "demo_user"

    try:
        result = await entity_resolution_service.resolve_participant(
            resolution, user_id, db
        )
        return result

    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.post("/entity-resolution/topic")
async def resolve_topic_entity(
    resolution: TopicResolution,
    db: AsyncSession = Depends(get_db)
):
    """Resolve topic/project linking"""

    # Mock user for demo
    user_id = "demo_user"

    try:
        result = await entity_resolution_service.resolve_topic(
            resolution, user_id, db
        )
        return result

    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.post("/entity-resolution/bulk")
async def bulk_resolve_entities(
    resolutions: List[EntityResolution],
    db: AsyncSession = Depends(get_db)
):
    """Apply multiple entity resolutions in batch"""

    # Mock user for demo
    user_id = "demo_user"

    try:
        result = await entity_resolution_service.bulk_resolve_entities(
            resolutions, user_id, db
        )
        return result

    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/entity-resolution/suggestions/{entity_type}")
async def get_entity_suggestions(
    entity_type: str,
    query: str = Query(..., description="Search query for entity suggestions"),
    limit: int = Query(10, description="Maximum number of suggestions"),
    db: AsyncSession = Depends(get_db)
):
    """Get suggestions for entity resolution"""

    try:
        if entity_type not in ["participant", "topic", "project"]:
            raise HTTPException(status_code=400, detail="Invalid entity type")

        suggestions = await entity_resolution_service.get_resolution_suggestions(
            entity_type, query, db, limit
        )

        return {
            "entity_type": entity_type,
            "query": query,
            "suggestions": suggestions,
            "count": len(suggestions)
        }

    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))