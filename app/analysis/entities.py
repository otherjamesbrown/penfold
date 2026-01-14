"""
Entity Resolution Integration

Connects meeting participants to existing person/project entities,
creates provisional entities, and manages manual review queue for
failed automatic resolutions.
"""
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, insert, update
from typing import Dict, Any, List, Optional, Tuple
import uuid
from datetime import datetime
import asyncio
import json
import re
from difflib import SequenceMatcher

from app.database import MeetingParticipant, ReviewQueue, MeetingTopic, MeetingInsight


class EntityResolver:
    """Resolves meeting participants and content to existing entities"""

    def __init__(self):
        # Confidence thresholds for automatic resolution
        self.person_match_threshold = 0.85
        self.project_match_threshold = 0.75
        self.auto_resolve_threshold = 0.9

        # Common name variations and aliases
        self.name_normalizations = {
            # Common nickname mappings
            "mike": "michael", "mike": "michael", "bob": "robert", "rob": "robert",
            "bill": "william", "will": "william", "jim": "james", "jimmy": "james",
            "dave": "david", "dan": "daniel", "chris": "christopher", "matt": "matthew",
            "tom": "thomas", "tim": "timothy", "steve": "stephen", "rick": "richard"
        }

        # Project/topic keywords for matching
        self.project_keywords = [
            "project", "initiative", "program", "workstream", "effort",
            "development", "implementation", "rollout", "launch", "phase"
        ]

    async def resolve_meeting_entities(
        self,
        meeting_id: str,
        transcript_data: Dict[str, Any],
        summary_data: Dict[str, Any],
        db: AsyncSession
    ) -> Dict[str, Any]:
        """Resolve all entities mentioned in the meeting"""

        try:
            print(f"🔗 Starting entity resolution for meeting {meeting_id}")

            # Resolve meeting participants
            participant_results = await self._resolve_participants(
                meeting_id, transcript_data, db
            )

            # Resolve project/topic mentions
            topic_results = await self._resolve_topics_and_projects(
                meeting_id, transcript_data, summary_data, db
            )

            # Create insights linking to entities
            insight_results = await self._create_entity_insights(
                meeting_id, summary_data, participant_results, topic_results, db
            )

            # Generate review queue items for failed resolutions
            review_items = await self._create_review_queue_items(
                meeting_id, participant_results, topic_results, db
            )

            # Commit all changes
            await db.commit()

            resolution_summary = {
                "meeting_id": meeting_id,
                "participants": participant_results,
                "topics": topic_results,
                "insights": insight_results,
                "review_queue_items": len(review_items),
                "resolution_statistics": {
                    "participants_resolved": sum(1 for p in participant_results["results"] if not p["is_provisional"]),
                    "participants_provisional": sum(1 for p in participant_results["results"] if p["is_provisional"]),
                    "topics_resolved": sum(1 for t in topic_results["results"] if t.get("project_id")),
                    "topics_unresolved": sum(1 for t in topic_results["results"] if not t.get("project_id")),
                    "insights_created": len(insight_results["results"])
                }
            }

            print(f"✅ Entity resolution complete: {resolution_summary['resolution_statistics']}")
            return resolution_summary

        except Exception as e:
            print(f"❌ Entity resolution failed: {str(e)}")
            await db.rollback()
            return {"error": str(e)}

    async def _resolve_participants(
        self,
        meeting_id: str,
        transcript_data: Dict[str, Any],
        db: AsyncSession
    ) -> Dict[str, Any]:
        """Resolve meeting participants to person entities"""

        speakers_detected = transcript_data.get("speakers_detected", [])
        speaker_segments = transcript_data.get("speaker_segments", [])

        results = []

        for speaker_label in speakers_detected:
            try:
                # Extract potential names from speaker segments
                speaker_texts = [
                    segment["text"] for segment in speaker_segments
                    if segment.get("speaker") == speaker_label
                ]

                # Attempt name extraction and entity resolution
                potential_names = self._extract_potential_names(speaker_texts)
                person_match = await self._find_person_entity(potential_names, db)

                # Create voice signature (simplified)
                voice_signature = self._generate_voice_signature(speaker_label, meeting_id)

                # Determine if we can auto-resolve
                if person_match and person_match["confidence"] >= self.auto_resolve_threshold:
                    # Auto-resolve to existing person
                    participant = MeetingParticipant(
                        meeting_file_id=uuid.UUID(meeting_id),
                        person_id=person_match["person_id"],
                        speaker_label=speaker_label,
                        voice_signature_hash=voice_signature,
                        identification_confidence=person_match["confidence"],
                        is_provisional=False,
                        contribution_stats=self._calculate_speaker_stats(speaker_label, speaker_segments)
                    )
                    is_provisional = False
                else:
                    # Create provisional entity
                    participant = MeetingParticipant(
                        meeting_file_id=uuid.UUID(meeting_id),
                        person_id=None,
                        speaker_label=speaker_label,
                        voice_signature_hash=voice_signature,
                        identification_confidence=person_match["confidence"] if person_match else 0.0,
                        is_provisional=True,
                        contribution_stats=self._calculate_speaker_stats(speaker_label, speaker_segments)
                    )
                    is_provisional = True

                db.add(participant)
                await db.flush()  # Get ID without committing

                results.append({
                    "participant_id": str(participant.id),
                    "speaker_label": speaker_label,
                    "person_id": str(person_match["person_id"]) if person_match and not is_provisional else None,
                    "is_provisional": is_provisional,
                    "confidence": person_match["confidence"] if person_match else 0.0,
                    "potential_matches": person_match.get("alternatives", []) if person_match else [],
                    "extracted_names": potential_names
                })

            except Exception as e:
                print(f"⚠️ Failed to resolve participant {speaker_label}: {str(e)}")
                results.append({
                    "speaker_label": speaker_label,
                    "error": str(e),
                    "is_provisional": True,
                    "confidence": 0.0
                })

        return {
            "total_speakers": len(speakers_detected),
            "resolved_count": sum(1 for r in results if not r.get("is_provisional", True)),
            "provisional_count": sum(1 for r in results if r.get("is_provisional", True)),
            "results": results
        }

    async def _resolve_topics_and_projects(
        self,
        meeting_id: str,
        transcript_data: Dict[str, Any],
        summary_data: Dict[str, Any],
        db: AsyncSession
    ) -> Dict[str, Any]:
        """Resolve topics and project mentions to existing entities"""

        # Extract topics from summary
        key_points = summary_data.get("key_points", [])
        decisions = summary_data.get("decisions", [])
        action_items = summary_data.get("action_items", [])

        # Identify potential project/topic mentions
        topic_candidates = self._extract_topic_candidates(
            transcript_data.get("transcript_text", ""),
            key_points, decisions, action_items
        )

        results = []

        for topic_candidate in topic_candidates:
            try:
                # Attempt to match to existing projects
                project_match = await self._find_project_entity(topic_candidate, db)

                # Calculate relevance score
                relevance_score = self._calculate_topic_relevance(
                    topic_candidate, transcript_data.get("transcript_text", "")
                )

                # Create topic record
                meeting_topic = MeetingTopic(
                    meeting_file_id=uuid.UUID(meeting_id),
                    project_id=project_match["project_id"] if project_match else None,
                    topic_name=topic_candidate["name"],
                    relevance_score=relevance_score,
                    topic_category=topic_candidate["category"],
                    first_mentioned=topic_candidate.get("first_mentioned"),
                    last_mentioned=topic_candidate.get("last_mentioned"),
                    context_snippets=topic_candidate.get("context_snippets", {})
                )

                db.add(meeting_topic)
                await db.flush()

                results.append({
                    "topic_id": str(meeting_topic.id),
                    "topic_name": topic_candidate["name"],
                    "category": topic_candidate["category"],
                    "project_id": str(project_match["project_id"]) if project_match else None,
                    "project_name": project_match.get("project_name") if project_match else None,
                    "relevance_score": relevance_score,
                    "match_confidence": project_match["confidence"] if project_match else 0.0,
                    "requires_review": not project_match or project_match["confidence"] < self.project_match_threshold
                })

            except Exception as e:
                print(f"⚠️ Failed to resolve topic {topic_candidate['name']}: {str(e)}")
                results.append({
                    "topic_name": topic_candidate["name"],
                    "error": str(e),
                    "requires_review": True
                })

        return {
            "total_topics": len(topic_candidates),
            "resolved_count": sum(1 for r in results if r.get("project_id")),
            "unresolved_count": sum(1 for r in results if not r.get("project_id")),
            "results": results
        }

    async def _create_entity_insights(
        self,
        meeting_id: str,
        summary_data: Dict[str, Any],
        participant_results: Dict[str, Any],
        topic_results: Dict[str, Any],
        db: AsyncSession
    ) -> Dict[str, Any]:
        """Create insights linking meeting content to resolved entities"""

        insights = []

        # Create person-related insights
        for participant in participant_results["results"]:
            if not participant.get("is_provisional") and participant.get("person_id"):
                # Create insight about person's participation
                insight = MeetingInsight(
                    meeting_file_id=uuid.UUID(meeting_id),
                    insight_type="participant_contribution",
                    insight_text=f"Participant contributed to meeting discussion",
                    confidence_score=participant.get("confidence", 0.0),
                    related_entity_id=uuid.UUID(participant["person_id"]),
                    related_entity_type="person",
                    supporting_evidence={
                        "speaker_label": participant["speaker_label"],
                        "contribution_stats": participant.get("contribution_stats", {})
                    }
                )
                db.add(insight)
                insights.append(insight)

        # Create project-related insights
        for topic in topic_results["results"]:
            if topic.get("project_id"):
                insight = MeetingInsight(
                    meeting_file_id=uuid.UUID(meeting_id),
                    insight_type="project_discussion",
                    insight_text=f"Meeting discussed project-related topic: {topic['topic_name']}",
                    confidence_score=topic.get("match_confidence", 0.0),
                    related_entity_id=uuid.UUID(topic["project_id"]),
                    related_entity_type="project",
                    supporting_evidence={
                        "topic_name": topic["topic_name"],
                        "relevance_score": topic.get("relevance_score", 0.0),
                        "category": topic.get("category", "")
                    }
                )
                db.add(insight)
                insights.append(insight)

        await db.flush()

        return {
            "insights_created": len(insights),
            "person_insights": sum(1 for i in insights if i.related_entity_type == "person"),
            "project_insights": sum(1 for i in insights if i.related_entity_type == "project"),
            "results": [{"insight_id": str(i.id), "type": i.insight_type} for i in insights]
        }

    async def _create_review_queue_items(
        self,
        meeting_id: str,
        participant_results: Dict[str, Any],
        topic_results: Dict[str, Any],
        db: AsyncSession
    ) -> List[Dict[str, Any]]:
        """Create manual review queue items for failed resolutions"""

        review_items = []

        # Create review items for provisional participants
        provisional_participants = [p for p in participant_results["results"] if p.get("is_provisional")]
        if provisional_participants:
            review_item = ReviewQueue(
                meeting_file_id=uuid.UUID(meeting_id),
                review_type="speaker_identification",
                status="pending",
                ai_suggestions={
                    "provisional_participants": provisional_participants,
                    "confidence_threshold": self.person_match_threshold,
                    "suggestion": "Manual identification needed for provisional speakers"
                },
                assigned_to=None  # Will be assigned by review system
            )
            db.add(review_item)
            review_items.append(review_item)

        # Create review items for unresolved topics
        unresolved_topics = [t for t in topic_results["results"] if t.get("requires_review", True)]
        if unresolved_topics:
            review_item = ReviewQueue(
                meeting_file_id=uuid.UUID(meeting_id),
                review_type="project_linking",
                status="pending",
                ai_suggestions={
                    "unresolved_topics": unresolved_topics,
                    "confidence_threshold": self.project_match_threshold,
                    "suggestion": "Manual project linking needed for discussed topics"
                },
                assigned_to=None
            )
            db.add(review_item)
            review_items.append(review_item)

        await db.flush()
        return review_items

    def _extract_potential_names(self, speaker_texts: List[str]) -> List[str]:
        """Extract potential names mentioned by or about a speaker"""

        potential_names = []

        # Simple name patterns (this would be enhanced in production)
        name_patterns = [
            r"I'm (\w+)",
            r"This is (\w+)",
            r"My name is (\w+ \w+)",
            r"(\w+) speaking",
            r"(\w+ \w+) here"
        ]

        for text in speaker_texts:
            for pattern in name_patterns:
                matches = re.findall(pattern, text, re.IGNORECASE)
                potential_names.extend(matches)

        # Clean and deduplicate names
        cleaned_names = []
        for name in potential_names:
            if isinstance(name, tuple):
                name = " ".join(name)
            cleaned_name = self._normalize_name(name.strip())
            if cleaned_name and len(cleaned_name) > 2:
                cleaned_names.append(cleaned_name)

        return list(set(cleaned_names))

    async def _find_person_entity(self, potential_names: List[str], db: AsyncSession) -> Optional[Dict[str, Any]]:
        """Find matching person entity in the database"""

        # This is a simplified implementation
        # In production, this would query an actual Person entity table

        # Placeholder implementation
        # Would query: SELECT * FROM persons WHERE name SIMILAR TO potential_names

        # For now, return a mock result based on name quality
        if potential_names:
            best_name = max(potential_names, key=len)
            confidence = 0.7 if len(best_name.split()) > 1 else 0.4

            return {
                "person_id": uuid.uuid4(),  # Mock person ID
                "matched_name": best_name,
                "confidence": confidence,
                "alternatives": []
            }

        return None

    def _extract_topic_candidates(
        self,
        transcript_text: str,
        key_points: List[Dict[str, Any]],
        decisions: List[Dict[str, Any]],
        action_items: List[Dict[str, Any]]
    ) -> List[Dict[str, Any]]:
        """Extract potential project/topic mentions from meeting content"""

        candidates = []

        # Extract from key points
        for point in key_points:
            point_text = point.get("point", "")
            category = point.get("category", "discussion")

            # Look for project keywords
            for keyword in self.project_keywords:
                if keyword.lower() in point_text.lower():
                    # Extract potential project name
                    topic_name = self._extract_topic_name(point_text, keyword)
                    if topic_name:
                        candidates.append({
                            "name": topic_name,
                            "category": category,
                            "source": "key_point",
                            "context_snippets": {"key_point": point_text}
                        })

        # Extract from decisions
        for decision in decisions:
            decision_text = decision.get("decision", "")
            candidates.append({
                "name": self._extract_topic_name(decision_text, "decision"),
                "category": "decision",
                "source": "decision",
                "context_snippets": {"decision": decision_text}
            })

        # Extract from action items
        for item in action_items:
            item_text = item.get("item", "")
            candidates.append({
                "name": self._extract_topic_name(item_text, "action"),
                "category": "action_item",
                "source": "action_item",
                "context_snippets": {"action_item": item_text}
            })

        # Clean and deduplicate
        unique_candidates = []
        seen_names = set()

        for candidate in candidates:
            if candidate["name"] and candidate["name"].lower() not in seen_names:
                seen_names.add(candidate["name"].lower())
                unique_candidates.append(candidate)

        return unique_candidates

    def _extract_topic_name(self, text: str, keyword: str) -> Optional[str]:
        """Extract topic name from text containing keyword"""

        # Simple extraction logic (would be enhanced in production)
        text = text.strip()

        # Try to find the topic name near the keyword
        words = text.split()

        if len(words) <= 2:
            return None

        # Take first few meaningful words as topic name
        topic_words = []
        for word in words[:5]:  # Limit to prevent overly long names
            cleaned_word = re.sub(r'[^\w\s]', '', word)
            if len(cleaned_word) > 2 and cleaned_word.lower() not in ['the', 'and', 'or', 'but', 'for', 'with']:
                topic_words.append(cleaned_word)

            if len(topic_words) >= 3:  # Limit topic name length
                break

        return " ".join(topic_words) if topic_words else None

    async def _find_project_entity(self, topic_candidate: Dict[str, Any], db: AsyncSession) -> Optional[Dict[str, Any]]:
        """Find matching project entity in the database"""

        # Placeholder implementation
        # Would query: SELECT * FROM projects WHERE name SIMILAR TO topic_candidate['name']

        topic_name = topic_candidate.get("name", "")

        # Mock project matching based on topic quality
        if topic_name and len(topic_name.split()) >= 2:
            confidence = 0.6 if topic_candidate.get("category") == "decision" else 0.4

            return {
                "project_id": uuid.uuid4(),  # Mock project ID
                "project_name": f"Project: {topic_name}",
                "confidence": confidence
            }

        return None

    def _calculate_topic_relevance(self, topic_candidate: Dict[str, Any], transcript_text: str) -> float:
        """Calculate relevance score for a topic in the meeting"""

        topic_name = topic_candidate.get("name", "")

        # Count mentions in transcript
        mention_count = transcript_text.lower().count(topic_name.lower())

        # Base score from mention frequency
        base_score = min(1.0, mention_count / 10.0)  # Normalize to max 1.0

        # Boost score based on source type
        source_boost = {
            "decision": 0.3,
            "action_item": 0.2,
            "key_point": 0.1
        }

        source = topic_candidate.get("source", "")
        boosted_score = base_score + source_boost.get(source, 0)

        return round(min(1.0, boosted_score), 2)

    def _calculate_speaker_stats(self, speaker_label: str, speaker_segments: List[Dict[str, Any]]) -> Dict[str, Any]:
        """Calculate statistics for a speaker's contribution"""

        speaker_segments_filtered = [s for s in speaker_segments if s.get("speaker") == speaker_label]

        if not speaker_segments_filtered:
            return {}

        # Calculate speaking time
        total_speaking_time = sum(
            (segment.get("end_time", 0) - segment.get("start_time", 0))
            for segment in speaker_segments_filtered
        )

        # Calculate word count
        total_words = sum(len(segment.get("text", "").split()) for segment in speaker_segments_filtered)

        # Calculate interruptions (simplified)
        segment_count = len(speaker_segments_filtered)

        return {
            "total_speaking_time": total_speaking_time,
            "word_count": total_words,
            "segment_count": segment_count,
            "average_segment_length": total_speaking_time / segment_count if segment_count > 0 else 0,
            "words_per_minute": (total_words / (total_speaking_time / 60)) if total_speaking_time > 0 else 0
        }

    def _generate_voice_signature(self, speaker_label: str, meeting_id: str) -> str:
        """Generate a voice signature hash for the speaker"""

        # Simplified voice signature (would use actual audio features in production)
        import hashlib
        signature_data = f"{speaker_label}_{meeting_id}_{datetime.utcnow().isoformat()}"
        return hashlib.md5(signature_data.encode()).hexdigest()

    def _normalize_name(self, name: str) -> str:
        """Normalize name for consistent matching"""

        name = name.lower().strip()

        # Apply nickname normalizations
        parts = name.split()
        normalized_parts = []

        for part in parts:
            normalized_part = self.name_normalizations.get(part, part)
            normalized_parts.append(normalized_part.title())

        return " ".join(normalized_parts)


# Global entity resolver instance
entity_resolver = EntityResolver()