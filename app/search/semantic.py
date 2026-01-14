"""
Semantic Search using pgvector

Provides semantic search capabilities across meeting transcripts and summaries
using OpenAI embeddings stored in PostgreSQL with pgvector extension.
"""
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, text, func
from typing import List, Dict, Any, Optional, Tuple
import numpy as np
from datetime import datetime, timedelta

from app.database import MeetingTranscript, MeetingSummary, MeetingFile, MeetingParticipant
from app.analysis.embeddings import embedding_service


class SemanticSearchService:
    """Service for semantic search operations using pgvector"""

    def __init__(self):
        self.embedding_dim = 1536  # OpenAI embedding dimension
        self.similarity_threshold = 0.7  # Minimum cosine similarity for relevance

    async def initialize_indexes(self, db: AsyncSession) -> None:
        """Create or update pgvector indexes for optimal search performance"""

        try:
            print("🔧 Initializing pgvector indexes for semantic search...")

            # Enable pgvector extension
            await db.execute(text("CREATE EXTENSION IF NOT EXISTS vector"))

            # Create vector indexes for meeting transcripts
            await db.execute(text("""
                CREATE INDEX CONCURRENTLY IF NOT EXISTS meeting_transcript_embedding_idx
                ON meeting_transcripts USING ivfflat (embedding vector_cosine_ops)
                WITH (lists = 100)
            """))

            # Create vector indexes for meeting summaries
            await db.execute(text("""
                CREATE INDEX CONCURRENTLY IF NOT EXISTS meeting_summary_embedding_idx
                ON meeting_summaries USING ivfflat (summary_embedding vector_cosine_ops)
                WITH (lists = 100)
            """))

            # Create composite indexes for privacy-aware search
            await db.execute(text("""
                CREATE INDEX CONCURRENTLY IF NOT EXISTS meeting_files_privacy_created_idx
                ON meeting_files (privacy_level, created_at DESC)
            """))

            # Create full-text search indexes
            await db.execute(text("""
                CREATE INDEX CONCURRENTLY IF NOT EXISTS meeting_transcript_text_idx
                ON meeting_transcripts USING gin(to_tsvector('english', transcript_text))
            """))

            # Create indexes for meeting metadata search
            await db.execute(text("""
                CREATE INDEX CONCURRENTLY IF NOT EXISTS meeting_files_filename_idx
                ON meeting_files USING gin(to_tsvector('english', filename))
            """))

            await db.commit()
            print("✅ pgvector indexes created successfully")

        except Exception as e:
            print(f"⚠️ Index creation warning: {str(e)}")
            # Some indexes might already exist - this is usually fine
            await db.rollback()

    async def search_meetings_by_text(
        self,
        query_text: str,
        db: AsyncSession,
        privacy_levels: List[str] = None,
        limit: int = 20,
        similarity_threshold: float = None
    ) -> List[Dict[str, Any]]:
        """Search meetings using semantic similarity on transcript text"""

        if not query_text.strip():
            return []

        # Use provided threshold or default
        threshold = similarity_threshold or self.similarity_threshold

        # Generate embedding for query
        query_embedding = await embedding_service.generate_text_embedding(query_text)
        if not query_embedding:
            print("⚠️ Failed to generate query embedding")
            return []

        # Default privacy levels if not specified
        if privacy_levels is None:
            privacy_levels = ["public", "organization", "team_only"]

        try:
            # Semantic search query with privacy filtering
            stmt = select(
                MeetingTranscript.id,
                MeetingTranscript.meeting_file_id,
                MeetingTranscript.transcript_text,
                MeetingTranscript.confidence_score,
                MeetingTranscript.created_at,
                MeetingTranscript.speaker_segments,
                MeetingFile.filename,
                MeetingFile.privacy_level,
                MeetingFile.metadata,
                func.round(
                    (1 - (MeetingTranscript.embedding.cosine_distance(query_embedding))) * 100, 2
                ).label("similarity_score")
            ).select_from(
                MeetingTranscript.join(MeetingFile, MeetingTranscript.meeting_file_id == MeetingFile.id)
            ).where(
                MeetingTranscript.embedding.is_not(None),  # Only search meetings with embeddings
                MeetingFile.privacy_level.in_(privacy_levels),
                MeetingFile.upload_completed_at.is_not(None),  # Only completed uploads
                (1 - MeetingTranscript.embedding.cosine_distance(query_embedding)) >= threshold
            ).order_by(
                (1 - MeetingTranscript.embedding.cosine_distance(query_embedding)).desc()
            ).limit(limit)

            result = await db.execute(stmt)
            rows = result.fetchall()

            # Format results
            search_results = []
            for row in rows:
                search_results.append({
                    "meeting_id": str(row.meeting_file_id),
                    "transcript_id": str(row.id),
                    "filename": row.filename,
                    "similarity_score": float(row.similarity_score),
                    "confidence_score": float(row.confidence_score),
                    "privacy_level": row.privacy_level,
                    "created_at": row.created_at.isoformat() + "Z",
                    "transcript_preview": self._create_text_preview(row.transcript_text, query_text),
                    "speaker_count": len(row.speaker_segments) if row.speaker_segments else 0,
                    "metadata": row.metadata or {}
                })

            print(f"📊 Semantic search found {len(search_results)} meetings (threshold: {threshold})")
            return search_results

        except Exception as e:
            print(f"❌ Semantic search failed: {str(e)}")
            return []

    async def search_summaries_by_text(
        self,
        query_text: str,
        db: AsyncSession,
        privacy_levels: List[str] = None,
        limit: int = 20,
        similarity_threshold: float = None
    ) -> List[Dict[str, Any]]:
        """Search meeting summaries using semantic similarity"""

        if not query_text.strip():
            return []

        # Use provided threshold or default
        threshold = similarity_threshold or self.similarity_threshold

        # Generate embedding for query
        query_embedding = await embedding_service.generate_text_embedding(query_text)
        if not query_embedding:
            return []

        # Default privacy levels if not specified
        if privacy_levels is None:
            privacy_levels = ["public", "organization", "team_only"]

        try:
            # Semantic search query on summaries
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
                func.round(
                    (1 - (MeetingSummary.summary_embedding.cosine_distance(query_embedding))) * 100, 2
                ).label("similarity_score")
            ).select_from(
                MeetingSummary.join(MeetingFile, MeetingSummary.meeting_file_id == MeetingFile.id)
            ).where(
                MeetingSummary.summary_embedding.is_not(None),
                MeetingFile.privacy_level.in_(privacy_levels),
                MeetingFile.upload_completed_at.is_not(None),
                (1 - MeetingSummary.summary_embedding.cosine_distance(query_embedding)) >= threshold
            ).order_by(
                (1 - MeetingSummary.summary_embedding.cosine_distance(query_embedding)).desc()
            ).limit(limit)

            result = await db.execute(stmt)
            rows = result.fetchall()

            # Format results
            search_results = []
            for row in rows:
                search_results.append({
                    "meeting_id": str(row.meeting_file_id),
                    "summary_id": str(row.id),
                    "filename": row.filename,
                    "similarity_score": float(row.similarity_score),
                    "quality_score": float(row.quality_score),
                    "privacy_level": row.privacy_level,
                    "created_at": row.created_at.isoformat() + "Z",
                    "summary_preview": self._create_text_preview(row.summary_text, query_text),
                    "key_points_count": len(row.key_points) if row.key_points else 0,
                    "action_items_count": len(row.action_items) if row.action_items else 0,
                    "decisions_count": len(row.decisions) if row.decisions else 0,
                })

            print(f"📊 Summary semantic search found {len(search_results)} meetings")
            return search_results

        except Exception as e:
            print(f"❌ Summary semantic search failed: {str(e)}")
            return []

    async def find_similar_meetings(
        self,
        meeting_id: str,
        db: AsyncSession,
        limit: int = 10,
        similarity_threshold: float = 0.8
    ) -> List[Dict[str, Any]]:
        """Find meetings similar to a given meeting using transcript embeddings"""

        try:
            # Get the target meeting's embedding
            stmt = select(MeetingTranscript.embedding, MeetingFile.privacy_level).select_from(
                MeetingTranscript.join(MeetingFile, MeetingTranscript.meeting_file_id == MeetingFile.id)
            ).where(MeetingTranscript.meeting_file_id == meeting_id)

            result = await db.execute(stmt)
            row = result.first()

            if not row or not row.embedding:
                return []

            target_embedding = row.embedding
            target_privacy = row.privacy_level

            # Find similar meetings (excluding the target)
            stmt = select(
                MeetingTranscript.id,
                MeetingTranscript.meeting_file_id,
                MeetingTranscript.transcript_text,
                MeetingFile.filename,
                MeetingFile.privacy_level,
                MeetingFile.created_at,
                func.round(
                    (1 - (MeetingTranscript.embedding.cosine_distance(target_embedding))) * 100, 2
                ).label("similarity_score")
            ).select_from(
                MeetingTranscript.join(MeetingFile, MeetingTranscript.meeting_file_id == MeetingFile.id)
            ).where(
                MeetingTranscript.embedding.is_not(None),
                MeetingTranscript.meeting_file_id != meeting_id,  # Exclude target meeting
                MeetingFile.upload_completed_at.is_not(None),
                MeetingFile.privacy_level == target_privacy,  # Same privacy level only
                (1 - MeetingTranscript.embedding.cosine_distance(target_embedding)) >= similarity_threshold
            ).order_by(
                (1 - MeetingTranscript.embedding.cosine_distance(target_embedding)).desc()
            ).limit(limit)

            result = await db.execute(stmt)
            rows = result.fetchall()

            # Format results
            similar_meetings = []
            for row in rows:
                similar_meetings.append({
                    "meeting_id": str(row.meeting_file_id),
                    "filename": row.filename,
                    "similarity_score": float(row.similarity_score),
                    "privacy_level": row.privacy_level,
                    "created_at": row.created_at.isoformat() + "Z",
                    "transcript_preview": row.transcript_text[:200] + "..." if len(row.transcript_text) > 200 else row.transcript_text
                })

            print(f"📊 Found {len(similar_meetings)} similar meetings (threshold: {similarity_threshold})")
            return similar_meetings

        except Exception as e:
            print(f"❌ Similar meetings search failed: {str(e)}")
            return []

    async def get_search_suggestions(
        self,
        partial_query: str,
        db: AsyncSession,
        privacy_levels: List[str] = None,
        limit: int = 5
    ) -> List[str]:
        """Get search suggestions based on partial query and existing content"""

        if len(partial_query.strip()) < 2:
            return []

        try:
            # Get suggestions from meeting filenames
            filename_suggestions = await self._get_filename_suggestions(partial_query, db, privacy_levels, limit)

            # Get suggestions from transcript content
            content_suggestions = await self._get_content_suggestions(partial_query, db, privacy_levels, limit)

            # Combine and deduplicate
            all_suggestions = filename_suggestions + content_suggestions
            unique_suggestions = list(dict.fromkeys(all_suggestions))  # Preserve order while removing duplicates

            return unique_suggestions[:limit]

        except Exception as e:
            print(f"❌ Search suggestions failed: {str(e)}")
            return []

    async def _get_filename_suggestions(
        self,
        partial_query: str,
        db: AsyncSession,
        privacy_levels: List[str],
        limit: int
    ) -> List[str]:
        """Get suggestions from meeting filenames"""

        if privacy_levels is None:
            privacy_levels = ["public", "organization", "team_only"]

        try:
            stmt = select(MeetingFile.filename).where(
                MeetingFile.privacy_level.in_(privacy_levels),
                MeetingFile.upload_completed_at.is_not(None),
                MeetingFile.filename.icontains(partial_query)
            ).limit(limit)

            result = await db.execute(stmt)
            rows = result.fetchall()

            # Extract meaningful terms from filenames
            suggestions = []
            for row in rows:
                filename = row.filename
                # Extract words from filename, clean up extensions and common prefixes
                clean_name = filename.replace('_', ' ').replace('-', ' ')
                clean_name = clean_name.split('.')[0]  # Remove extension
                words = [w for w in clean_name.split() if len(w) > 2 and w.lower() not in ['meeting', 'call', 'session']]

                if words:
                    suggestions.append(' '.join(words[:3]))  # First few meaningful words

            return suggestions

        except Exception as e:
            print(f"⚠️ Filename suggestions failed: {str(e)}")
            return []

    async def _get_content_suggestions(
        self,
        partial_query: str,
        db: AsyncSession,
        privacy_levels: List[str],
        limit: int
    ) -> List[str]:
        """Get suggestions from transcript content using full-text search"""

        if privacy_levels is None:
            privacy_levels = ["public", "organization", "team_only"]

        try:
            # Use PostgreSQL full-text search for content suggestions
            stmt = select(
                func.ts_headline('english', MeetingTranscript.transcript_text,
                               func.plainto_tsquery('english', partial_query))
            ).select_from(
                MeetingTranscript.join(MeetingFile, MeetingTranscript.meeting_file_id == MeetingFile.id)
            ).where(
                MeetingFile.privacy_level.in_(privacy_levels),
                MeetingFile.upload_completed_at.is_not(None),
                func.to_tsvector('english', MeetingTranscript.transcript_text).match(
                    func.plainto_tsquery('english', partial_query)
                )
            ).limit(limit)

            result = await db.execute(stmt)
            rows = result.fetchall()

            # Extract meaningful phrases from highlighted content
            suggestions = []
            for row in rows:
                headline = row[0] if row[0] else ""
                # Extract phrases around the matched terms
                # This is a simplified extraction - could be enhanced with NLP
                words = headline.replace('<b>', '').replace('</b>', '').split()
                if len(words) >= 3:
                    # Get a 3-5 word phrase containing the match
                    phrase = ' '.join(words[:5])
                    if len(phrase) > 10:  # Minimum meaningful length
                        suggestions.append(phrase)

            return suggestions

        except Exception as e:
            print(f"⚠️ Content suggestions failed: {str(e)}")
            return []

    def _create_text_preview(self, text: str, query: str, max_length: int = 200) -> str:
        """Create a preview of text highlighting the query terms"""

        if not text or not query:
            return text[:max_length] + "..." if len(text) > max_length else text

        # Find the best snippet that contains query terms
        query_words = query.lower().split()
        text_lower = text.lower()

        # Find the first occurrence of any query word
        best_start = 0
        for word in query_words:
            pos = text_lower.find(word)
            if pos != -1:
                # Start snippet a bit before the match
                best_start = max(0, pos - 50)
                break

        # Create snippet
        snippet = text[best_start:best_start + max_length]
        if best_start > 0:
            snippet = "..." + snippet
        if len(text) > best_start + max_length:
            snippet = snippet + "..."

        return snippet

    async def get_search_statistics(self, db: AsyncSession) -> Dict[str, Any]:
        """Get statistics about searchable content"""

        try:
            # Count meetings with embeddings
            stmt = select(func.count(MeetingTranscript.id)).where(
                MeetingTranscript.embedding.is_not(None)
            )
            result = await db.execute(stmt)
            transcript_count = result.scalar() or 0

            # Count summaries with embeddings
            stmt = select(func.count(MeetingSummary.id)).where(
                MeetingSummary.summary_embedding.is_not(None)
            )
            result = await db.execute(stmt)
            summary_count = result.scalar() or 0

            # Count by privacy level
            stmt = select(
                MeetingFile.privacy_level,
                func.count(MeetingTranscript.id).label("count")
            ).select_from(
                MeetingTranscript.join(MeetingFile, MeetingTranscript.meeting_file_id == MeetingFile.id)
            ).where(
                MeetingTranscript.embedding.is_not(None)
            ).group_by(MeetingFile.privacy_level)

            result = await db.execute(stmt)
            by_privacy = {row[0]: row[1] for row in result.fetchall()}

            return {
                "searchable_transcripts": transcript_count,
                "searchable_summaries": summary_count,
                "by_privacy_level": by_privacy,
                "embedding_dimension": self.embedding_dim,
                "default_similarity_threshold": self.similarity_threshold
            }

        except Exception as e:
            print(f"❌ Statistics query failed: {str(e)}")
            return {
                "searchable_transcripts": 0,
                "searchable_summaries": 0,
                "by_privacy_level": {},
                "error": str(e)
            }


# Global semantic search service
semantic_search = SemanticSearchService()