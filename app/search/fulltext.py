"""
Full-Text Search using PostgreSQL

Provides full-text search capabilities across meeting content using PostgreSQL's
built-in text search features with ranking and phrase matching.
"""
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, text, func, and_, or_
from typing import List, Dict, Any, Optional, Tuple
from datetime import datetime, timedelta

from app.database import MeetingTranscript, MeetingSummary, MeetingFile, MeetingParticipant, MeetingTopic


class FullTextSearchService:
    """Service for full-text search operations using PostgreSQL"""

    def __init__(self):
        self.default_language = "english"
        self.min_word_length = 2
        self.max_results = 100

    async def search_transcript_content(
        self,
        query_text: str,
        db: AsyncSession,
        privacy_levels: List[str] = None,
        limit: int = 20,
        use_phrase_search: bool = False,
        include_headlines: bool = True
    ) -> List[Dict[str, Any]]:
        """Search meeting transcript content using full-text search"""

        if not query_text.strip() or len(query_text.strip()) < self.min_word_length:
            return []

        # Default privacy levels if not specified
        if privacy_levels is None:
            privacy_levels = ["public", "organization", "team_only"]

        try:
            # Prepare the search query for PostgreSQL
            search_query = self._prepare_search_query(query_text, use_phrase_search)

            # Base query with full-text search and ranking
            base_query = select(
                MeetingTranscript.id,
                MeetingTranscript.meeting_file_id,
                MeetingTranscript.transcript_text,
                MeetingTranscript.confidence_score,
                MeetingTranscript.created_at,
                MeetingTranscript.speaker_segments,
                MeetingFile.filename,
                MeetingFile.privacy_level,
                MeetingFile.metadata,
                func.ts_rank(
                    func.to_tsvector(self.default_language, MeetingTranscript.transcript_text),
                    func.to_tsquery(self.default_language, search_query)
                ).label("rank_score")
            )

            # Add headline highlighting if requested
            if include_headlines:
                base_query = base_query.add_columns(
                    func.ts_headline(
                        self.default_language,
                        MeetingTranscript.transcript_text,
                        func.to_tsquery(self.default_language, search_query),
                        "MaxFragments=3, MaxWords=50, MinWords=20"
                    ).label("highlighted_text")
                )

            # Complete the query with joins and filters
            stmt = base_query.select_from(
                MeetingTranscript.join(MeetingFile, MeetingTranscript.meeting_file_id == MeetingFile.id)
            ).where(
                and_(
                    MeetingFile.privacy_level.in_(privacy_levels),
                    MeetingFile.upload_completed_at.is_not(None),
                    func.to_tsvector(self.default_language, MeetingTranscript.transcript_text).match(
                        func.to_tsquery(self.default_language, search_query)
                    )
                )
            ).order_by(
                func.ts_rank(
                    func.to_tsvector(self.default_language, MeetingTranscript.transcript_text),
                    func.to_tsquery(self.default_language, search_query)
                ).desc()
            ).limit(limit)

            result = await db.execute(stmt)
            rows = result.fetchall()

            # Format results
            search_results = []
            for row in rows:
                result_dict = {
                    "meeting_id": str(row.meeting_file_id),
                    "transcript_id": str(row.id),
                    "filename": row.filename,
                    "rank_score": float(row.rank_score),
                    "confidence_score": float(row.confidence_score),
                    "privacy_level": row.privacy_level,
                    "created_at": row.created_at.isoformat() + "Z",
                    "speaker_count": len(row.speaker_segments) if row.speaker_segments else 0,
                    "metadata": row.metadata or {},
                    "search_type": "fulltext"
                }

                # Add highlighted text if available
                if include_headlines and hasattr(row, 'highlighted_text'):
                    result_dict["highlighted_text"] = row.highlighted_text
                else:
                    result_dict["text_preview"] = self._create_text_preview(row.transcript_text, query_text)

                search_results.append(result_dict)

            print(f"📄 Full-text search found {len(search_results)} transcript matches")
            return search_results

        except Exception as e:
            print(f"❌ Full-text transcript search failed: {str(e)}")
            return []

    async def search_summary_content(
        self,
        query_text: str,
        db: AsyncSession,
        privacy_levels: List[str] = None,
        limit: int = 20,
        use_phrase_search: bool = False,
        include_structured_data: bool = True
    ) -> List[Dict[str, Any]]:
        """Search meeting summary content including structured data"""

        if not query_text.strip() or len(query_text.strip()) < self.min_word_length:
            return []

        # Default privacy levels if not specified
        if privacy_levels is None:
            privacy_levels = ["public", "organization", "team_only"]

        try:
            # Prepare the search query
            search_query = self._prepare_search_query(query_text, use_phrase_search)

            # Search in summary text
            stmt = select(
                MeetingSummary.id,
                MeetingSummary.meeting_file_id,
                MeetingSummary.summary_text,
                MeetingSummary.key_points,
                MeetingSummary.action_items,
                MeetingSummary.decisions,
                MeetingSummary.quality_score,
                MeetingSummary.created_at,
                MeetingFile.filename,
                MeetingFile.privacy_level,
                func.ts_rank(
                    func.to_tsvector(self.default_language, MeetingSummary.summary_text),
                    func.to_tsquery(self.default_language, search_query)
                ).label("rank_score"),
                func.ts_headline(
                    self.default_language,
                    MeetingSummary.summary_text,
                    func.to_tsquery(self.default_language, search_query),
                    "MaxFragments=2, MaxWords=30"
                ).label("highlighted_summary")
            ).select_from(
                MeetingSummary.join(MeetingFile, MeetingSummary.meeting_file_id == MeetingFile.id)
            ).where(
                and_(
                    MeetingFile.privacy_level.in_(privacy_levels),
                    MeetingFile.upload_completed_at.is_not(None),
                    func.to_tsvector(self.default_language, MeetingSummary.summary_text).match(
                        func.to_tsquery(self.default_language, search_query)
                    )
                )
            ).order_by(
                func.ts_rank(
                    func.to_tsvector(self.default_language, MeetingSummary.summary_text),
                    func.to_tsquery(self.default_language, search_query)
                ).desc()
            ).limit(limit)

            result = await db.execute(stmt)
            rows = result.fetchall()

            # Format results
            search_results = []
            for row in rows:
                result_dict = {
                    "meeting_id": str(row.meeting_file_id),
                    "summary_id": str(row.id),
                    "filename": row.filename,
                    "rank_score": float(row.rank_score),
                    "quality_score": float(row.quality_score),
                    "privacy_level": row.privacy_level,
                    "created_at": row.created_at.isoformat() + "Z",
                    "highlighted_summary": row.highlighted_summary,
                    "search_type": "fulltext_summary"
                }

                # Add structured data if requested
                if include_structured_data:
                    result_dict.update({
                        "key_points_count": len(row.key_points) if row.key_points else 0,
                        "action_items_count": len(row.action_items) if row.action_items else 0,
                        "decisions_count": len(row.decisions) if row.decisions else 0,
                        "matching_structured_data": self._find_matching_structured_data(
                            query_text, row.key_points, row.action_items, row.decisions
                        )
                    })

                search_results.append(result_dict)

            print(f"📄 Full-text summary search found {len(search_results)} matches")
            return search_results

        except Exception as e:
            print(f"❌ Full-text summary search failed: {str(e)}")
            return []

    async def search_by_filename(
        self,
        query_text: str,
        db: AsyncSession,
        privacy_levels: List[str] = None,
        limit: int = 20
    ) -> List[Dict[str, Any]]:
        """Search meetings by filename using full-text search"""

        if not query_text.strip():
            return []

        # Default privacy levels if not specified
        if privacy_levels is None:
            privacy_levels = ["public", "organization", "team_only"]

        try:
            # Use both ILIKE and full-text search for filename matching
            stmt = select(
                MeetingFile.id,
                MeetingFile.filename,
                MeetingFile.privacy_level,
                MeetingFile.created_at,
                MeetingFile.metadata,
                MeetingFile.size_bytes,
                func.ts_rank(
                    func.to_tsvector(self.default_language, MeetingFile.filename),
                    func.plainto_tsquery(self.default_language, query_text)
                ).label("rank_score")
            ).where(
                and_(
                    MeetingFile.privacy_level.in_(privacy_levels),
                    MeetingFile.upload_completed_at.is_not(None),
                    or_(
                        MeetingFile.filename.icontains(query_text),
                        func.to_tsvector(self.default_language, MeetingFile.filename).match(
                            func.plainto_tsquery(self.default_language, query_text)
                        )
                    )
                )
            ).order_by(
                func.ts_rank(
                    func.to_tsvector(self.default_language, MeetingFile.filename),
                    func.plainto_tsquery(self.default_language, query_text)
                ).desc()
            ).limit(limit)

            result = await db.execute(stmt)
            rows = result.fetchall()

            # Format results
            search_results = []
            for row in rows:
                search_results.append({
                    "meeting_id": str(row.id),
                    "filename": row.filename,
                    "privacy_level": row.privacy_level,
                    "created_at": row.created_at.isoformat() + "Z",
                    "size_bytes": row.size_bytes,
                    "rank_score": float(row.rank_score) if row.rank_score else 0.0,
                    "metadata": row.metadata or {},
                    "search_type": "filename"
                })

            print(f"📄 Filename search found {len(search_results)} matches")
            return search_results

        except Exception as e:
            print(f"❌ Filename search failed: {str(e)}")
            return []

    async def search_by_participant(
        self,
        participant_name: str,
        db: AsyncSession,
        privacy_levels: List[str] = None,
        limit: int = 20
    ) -> List[Dict[str, Any]]:
        """Search meetings by participant name or speaker label"""

        if not participant_name.strip():
            return []

        # Default privacy levels if not specified
        if privacy_levels is None:
            privacy_levels = ["public", "organization", "team_only"]

        try:
            # Search in speaker segments within transcripts
            stmt = select(
                MeetingTranscript.meeting_file_id,
                MeetingTranscript.speaker_segments,
                MeetingFile.filename,
                MeetingFile.privacy_level,
                MeetingFile.created_at,
                func.count().label("mention_count")
            ).select_from(
                MeetingTranscript.join(MeetingFile, MeetingTranscript.meeting_file_id == MeetingFile.id)
            ).where(
                and_(
                    MeetingFile.privacy_level.in_(privacy_levels),
                    MeetingFile.upload_completed_at.is_not(None),
                    MeetingTranscript.speaker_segments.isnot(None),
                    func.jsonb_path_exists(
                        MeetingTranscript.speaker_segments,
                        f'$[*] ? (@.speaker like_regex "{participant_name}" flag "i")'
                    )
                )
            ).group_by(
                MeetingTranscript.meeting_file_id,
                MeetingTranscript.speaker_segments,
                MeetingFile.filename,
                MeetingFile.privacy_level,
                MeetingFile.created_at
            ).order_by(
                func.count().desc(),
                MeetingFile.created_at.desc()
            ).limit(limit)

            result = await db.execute(stmt)
            rows = result.fetchall()

            # Format results
            search_results = []
            for row in rows:
                # Extract matching speaker information
                speaker_info = self._extract_speaker_info(row.speaker_segments, participant_name)

                search_results.append({
                    "meeting_id": str(row.meeting_file_id),
                    "filename": row.filename,
                    "privacy_level": row.privacy_level,
                    "created_at": row.created_at.isoformat() + "Z",
                    "mention_count": row.mention_count,
                    "speaker_info": speaker_info,
                    "search_type": "participant"
                })

            print(f"👤 Participant search found {len(search_results)} meetings with '{participant_name}'")
            return search_results

        except Exception as e:
            print(f"❌ Participant search failed: {str(e)}")
            return []

    async def search_by_date_range(
        self,
        start_date: datetime,
        end_date: datetime,
        db: AsyncSession,
        privacy_levels: List[str] = None,
        content_query: str = None,
        limit: int = 50
    ) -> List[Dict[str, Any]]:
        """Search meetings within a date range, optionally with content filter"""

        # Default privacy levels if not specified
        if privacy_levels is None:
            privacy_levels = ["public", "organization", "team_only"]

        try:
            # Base query for date range
            stmt = select(
                MeetingFile.id,
                MeetingFile.filename,
                MeetingFile.privacy_level,
                MeetingFile.created_at,
                MeetingFile.size_bytes,
                MeetingFile.metadata
            ).where(
                and_(
                    MeetingFile.privacy_level.in_(privacy_levels),
                    MeetingFile.upload_completed_at.is_not(None),
                    MeetingFile.created_at >= start_date,
                    MeetingFile.created_at <= end_date
                )
            ).order_by(MeetingFile.created_at.desc()).limit(limit)

            # Add content filter if provided
            if content_query and content_query.strip():
                search_query = self._prepare_search_query(content_query, use_phrase_search=False)
                stmt = select(
                    MeetingFile.id,
                    MeetingFile.filename,
                    MeetingFile.privacy_level,
                    MeetingFile.created_at,
                    MeetingFile.size_bytes,
                    MeetingFile.metadata,
                    func.ts_rank(
                        func.to_tsvector(self.default_language, MeetingTranscript.transcript_text),
                        func.to_tsquery(self.default_language, search_query)
                    ).label("content_rank")
                ).select_from(
                    MeetingFile.join(MeetingTranscript, MeetingFile.id == MeetingTranscript.meeting_file_id)
                ).where(
                    and_(
                        MeetingFile.privacy_level.in_(privacy_levels),
                        MeetingFile.upload_completed_at.is_not(None),
                        MeetingFile.created_at >= start_date,
                        MeetingFile.created_at <= end_date,
                        func.to_tsvector(self.default_language, MeetingTranscript.transcript_text).match(
                            func.to_tsquery(self.default_language, search_query)
                        )
                    )
                ).order_by(
                    func.ts_rank(
                        func.to_tsvector(self.default_language, MeetingTranscript.transcript_text),
                        func.to_tsquery(self.default_language, search_query)
                    ).desc(),
                    MeetingFile.created_at.desc()
                ).limit(limit)

            result = await db.execute(stmt)
            rows = result.fetchall()

            # Format results
            search_results = []
            for row in rows:
                result_dict = {
                    "meeting_id": str(row.id),
                    "filename": row.filename,
                    "privacy_level": row.privacy_level,
                    "created_at": row.created_at.isoformat() + "Z",
                    "size_bytes": row.size_bytes,
                    "metadata": row.metadata or {},
                    "search_type": "date_range"
                }

                # Add content rank if content search was performed
                if hasattr(row, 'content_rank') and row.content_rank:
                    result_dict["content_rank"] = float(row.content_rank)

                search_results.append(result_dict)

            date_str = f"{start_date.strftime('%Y-%m-%d')} to {end_date.strftime('%Y-%m-%d')}"
            content_filter_str = f" with content '{content_query}'" if content_query else ""
            print(f"📅 Date range search found {len(search_results)} meetings from {date_str}{content_filter_str}")
            return search_results

        except Exception as e:
            print(f"❌ Date range search failed: {str(e)}")
            return []

    def _prepare_search_query(self, query_text: str, use_phrase_search: bool = False) -> str:
        """Prepare search query for PostgreSQL full-text search"""

        # Clean the query
        query = query_text.strip()

        if use_phrase_search:
            # For phrase search, use quoted strings
            return f'"{query}"'
        else:
            # Split into words and create an OR query with prefix matching
            words = [word.strip() for word in query.split() if len(word.strip()) >= self.min_word_length]

            if not words:
                return ""

            # Create query with prefix matching for partial words
            query_parts = []
            for word in words:
                # Add both exact word and prefix matching
                if word.isalnum():
                    query_parts.append(f"{word}:*")
                else:
                    # For non-alphanumeric, just use exact match
                    query_parts.append(word)

            return " | ".join(query_parts)  # OR operation

    def _find_matching_structured_data(
        self,
        query_text: str,
        key_points: List[Dict],
        action_items: List[Dict],
        decisions: List[Dict]
    ) -> Dict[str, List[str]]:
        """Find structured data elements that match the search query"""

        matches = {
            "key_points": [],
            "action_items": [],
            "decisions": []
        }

        query_lower = query_text.lower()

        # Search in key points
        if key_points:
            for point in key_points:
                if isinstance(point, dict):
                    point_text = point.get("point", "").lower()
                    if query_lower in point_text:
                        matches["key_points"].append(point.get("point", ""))

        # Search in action items
        if action_items:
            for item in action_items:
                if isinstance(item, dict):
                    item_text = item.get("item", "").lower()
                    if query_lower in item_text:
                        matches["action_items"].append(item.get("item", ""))

        # Search in decisions
        if decisions:
            for decision in decisions:
                if isinstance(decision, dict):
                    decision_text = decision.get("decision", "").lower()
                    if query_lower in decision_text:
                        matches["decisions"].append(decision.get("decision", ""))

        return matches

    def _extract_speaker_info(self, speaker_segments: List[Dict], participant_name: str) -> List[Dict]:
        """Extract speaker information matching the participant name"""

        if not speaker_segments or not participant_name:
            return []

        matching_speakers = []
        participant_lower = participant_name.lower()

        for segment in speaker_segments:
            if isinstance(segment, dict):
                speaker = segment.get("speaker", "")
                if participant_lower in speaker.lower():
                    matching_speakers.append({
                        "speaker": speaker,
                        "text_preview": segment.get("text", "")[:100] + "..." if len(segment.get("text", "")) > 100 else segment.get("text", ""),
                        "timestamp": segment.get("start_time", 0)
                    })

        # Limit to most relevant matches
        return matching_speakers[:5]

    def _create_text_preview(self, text: str, query: str, max_length: int = 150) -> str:
        """Create a text preview highlighting the search terms"""

        if not text or not query:
            return text[:max_length] + "..." if len(text) > max_length else text

        # Find the best position to show the query context
        query_words = query.lower().split()
        text_lower = text.lower()

        best_pos = 0
        for word in query_words:
            pos = text_lower.find(word)
            if pos != -1:
                # Position to show some context before the match
                best_pos = max(0, pos - 30)
                break

        # Create preview
        preview = text[best_pos:best_pos + max_length]
        if best_pos > 0:
            preview = "..." + preview
        if len(text) > best_pos + max_length:
            preview = preview + "..."

        return preview


# Global full-text search service
fulltext_search = FullTextSearchService()