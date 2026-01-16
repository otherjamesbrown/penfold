"""Cross-content correlation discovery for search results.

This module identifies relationships between content items based on
multiple weighted signals: participants, projects, temporal proximity,
semantic similarity, and thread chains.

Signal weights (from research.md):
- Shared participants: 0.30 weight
- Shared project references: 0.25 weight
- Temporal proximity: 0.20 weight
- Semantic similarity: 0.15 weight
- Thread/reply chains: 0.10 weight
"""

from __future__ import annotations

import asyncio
import logging
import time
from dataclasses import dataclass, field
from datetime import datetime, timedelta, timezone
from typing import TYPE_CHECKING, Any

from sqlalchemy import text
from sqlalchemy.ext.asyncio import AsyncSession

from penf_lib.search.models import ContentTypeFilter

if TYPE_CHECKING:
    pass

logger = logging.getLogger(__name__)


@dataclass
class RelatedItem:
    """A content item related to a search result.

    Represents a discovered relationship between two content items,
    including the correlation strength and the signals that contributed.

    Attributes:
        entity_id: Database ID of the related entity
        entity_type: Type of entity (source, assertion, person, project)
        content_type: Content type filter enum value
        title: Optional title of the content
        timestamp: Content timestamp
        correlation_score: Combined weighted score (0-1)
        correlation_type: Primary correlation signal type
        score_breakdown: Individual signal scores
    """

    entity_id: int
    entity_type: str
    content_type: ContentTypeFilter
    title: str | None
    timestamp: datetime
    correlation_score: float
    correlation_type: str  # participant, project, temporal, semantic, thread
    score_breakdown: dict[str, float] = field(default_factory=dict)


@dataclass
class CorrelationResult:
    """Result of correlation discovery for a content item.

    Contains all related items found for a source content item,
    sorted by correlation strength.

    Attributes:
        source_id: ID of the source content item
        source_type: Type of the source entity
        related_items: List of related items sorted by score
        total_correlations: Total number of correlations found
    """

    source_id: int
    source_type: str
    related_items: list[RelatedItem]
    total_correlations: int


@dataclass
class ContentDetails:
    """Details about a content item for correlation discovery.

    Internal dataclass used to hold content metadata during
    correlation computations.
    """

    entity_id: int
    entity_type: str
    content_type: str | None
    participants: list[str]
    project_refs: list[str]
    timestamp: datetime | None
    embedding: list[float] | None
    thread_id: str | None
    title: str | None


class CorrelationDiscovery:
    """Discover relationships between content items.

    Uses weighted signals to find related content across different
    communication channels and content types.

    Signal Weights (from research.md):
        - participants: 0.30 (shared people involved)
        - project: 0.25 (shared project references)
        - temporal: 0.20 (time proximity within window)
        - semantic: 0.15 (vector embedding similarity)
        - thread: 0.10 (explicit conversation chains)

    Example:
        ```python
        async with get_session() as session:
            discovery = CorrelationDiscovery(session, tenant_id)
            result = await discovery.find_related_content(
                content_id=123,
                content_type="source",
                limit=10,
            )
            for item in result.related_items:
                print(f"{item.title}: {item.correlation_score:.2f}")
        ```
    """

    SIGNAL_WEIGHTS: dict[str, float] = {
        "participants": 0.30,
        "project": 0.25,
        "temporal": 0.20,
        "semantic": 0.15,
        "thread": 0.10,
    }

    # Temporal proximity window in days
    TEMPORAL_WINDOW_DAYS: int = 7

    # Minimum score threshold for including a result
    MIN_SCORE_THRESHOLD: float = 0.05

    def __init__(
        self,
        session: AsyncSession,
        tenant_id: str,
        weights: dict[str, float] | None = None,
    ):
        """Initialize correlation discovery.

        Args:
            session: Database session for queries
            tenant_id: Tenant UUID string for isolation
            weights: Custom signal weights (uses defaults if None)
        """
        self.session = session
        self.tenant_id = tenant_id
        self.weights = weights or self.SIGNAL_WEIGHTS.copy()

    async def find_related_content(
        self,
        content_id: int,
        content_type: str,
        limit: int = 10,
    ) -> CorrelationResult:
        """Find content related to a given item.

        Discovers related content through multiple signals:
        - Shared participants (people involved in both items)
        - Shared project references (same project mentioned)
        - Temporal proximity (content within time window)
        - Semantic similarity (embedding vector distance)
        - Thread/conversation chains (explicit relationships)

        Args:
            content_id: ID of the source content
            content_type: Type of the source content (source, assertion)
            limit: Maximum related items to return

        Returns:
            CorrelationResult with ranked related items

        Example:
            ```python
            result = await discovery.find_related_content(
                content_id=123,
                content_type="source",
                limit=10,
            )
            print(f"Found {result.total_correlations} correlations")
            for item in result.related_items:
                print(f"  - {item.title} ({item.correlation_type}): {item.correlation_score:.2f}")
            ```
        """
        start_time = time.perf_counter()

        # Get source content details
        source = await self._get_content(content_id, content_type)
        if not source:
            logger.warning(
                f"Content not found: {content_type}/{content_id} in tenant {self.tenant_id}"
            )
            return CorrelationResult(
                source_id=content_id,
                source_type=content_type,
                related_items=[],
                total_correlations=0,
            )

        # Find related content via each signal - execute in parallel
        (
            participant_matches,
            project_matches,
            temporal_matches,
            semantic_matches,
            thread_matches,
        ) = await asyncio.gather(
            self._find_by_participants(
                source.participants,
                content_id,
                content_type,
            ),
            self._find_by_project(
                source.project_refs,
                content_id,
                content_type,
            ),
            self._find_by_temporal_proximity(
                source.timestamp,
                content_id,
                content_type,
            ),
            self._find_by_similarity(
                source.embedding,
                content_id,
                content_type,
            ),
            self._find_by_thread(
                source.thread_id,
                content_id,
                content_type,
            ),
        )

        # Combine and score all matches
        combined = self._merge_and_score(
            participant_matches,
            project_matches,
            temporal_matches,
            semantic_matches,
            thread_matches,
        )

        # Filter by minimum score
        filtered = [
            item for item in combined if item.correlation_score >= self.MIN_SCORE_THRESHOLD
        ]

        # Sort by correlation score and limit
        sorted_results = sorted(filtered, key=lambda x: x.correlation_score, reverse=True)

        # Log performance metrics
        duration_ms = (time.perf_counter() - start_time) * 1000
        logger.info(
            f"Correlation discovery for {content_type}/{content_id}: "
            f"found {len(sorted_results)} related items in {duration_ms:.1f}ms"
        )
        logger.info(
            "Correlation discovery completed",
            extra={
                "correlation_discovery_ms": round(duration_ms, 2),
                "content_id": content_id,
                "content_type": content_type,
                "results_found": len(sorted_results),
            }
        )

        return CorrelationResult(
            source_id=content_id,
            source_type=content_type,
            related_items=sorted_results[:limit],
            total_correlations=len(sorted_results),
        )

    async def _get_content(
        self, content_id: int, content_type: str
    ) -> ContentDetails | None:
        """Get content details for correlation.

        Fetches the source content and extracts metadata needed for
        correlation discovery (participants, projects, timestamp, etc.).

        Args:
            content_id: ID of the content
            content_type: Type (source or assertion)

        Returns:
            ContentDetails or None if not found
        """
        try:
            if content_type == "source":
                return await self._get_source_details(content_id)
            elif content_type == "assertion":
                return await self._get_assertion_details(content_id)
            else:
                logger.warning(f"Unknown content type: {content_type}")
                return None
        except Exception as e:
            logger.error(f"Error fetching content {content_type}/{content_id}: {e}")
            return None

    async def _get_source_details(self, source_id: int) -> ContentDetails | None:
        """Get source content details."""
        query = text("""
            SELECT
                s.id,
                s.content_type,
                s.source_timestamp,
                s.ingestion_metadata,
                e.embedding
            FROM sources s
            LEFT JOIN embeddings e ON e.entity_id = s.id
                AND e.entity_type = 'source'
                AND e.tenant_id = s.tenant_id
            WHERE s.id = :source_id
                AND s.tenant_id = :tenant_id
                AND s.is_deleted = false
        """)

        result = await self.session.execute(
            query,
            {
                "source_id": source_id,
                "tenant_id": str(self.tenant_id),
            },
        )
        row = result.fetchone()

        if not row:
            return None

        # Extract metadata
        metadata = row.ingestion_metadata or {}
        participants = self._extract_participants(metadata)
        project_refs = self._extract_project_refs(metadata)
        thread_id = metadata.get("thread_id") or metadata.get("conversation_id")
        title = self._extract_title(metadata)

        # Parse embedding if present
        embedding = None
        if row.embedding:
            try:
                if isinstance(row.embedding, str):
                    import json

                    embedding = json.loads(row.embedding)
                else:
                    embedding = list(row.embedding)
            except (ValueError, TypeError):
                pass

        return ContentDetails(
            entity_id=row.id,
            entity_type="source",
            content_type=row.content_type,
            participants=participants,
            project_refs=project_refs,
            timestamp=row.source_timestamp,
            embedding=embedding,
            thread_id=thread_id,
            title=title,
        )

    async def _get_assertion_details(self, assertion_id: int) -> ContentDetails | None:
        """Get assertion content details."""
        query = text("""
            SELECT
                a.id,
                a.assertion_type,
                a.content,
                a.context,
                a.created_at,
                a.metadata,
                e.embedding
            FROM assertions a
            LEFT JOIN embeddings e ON e.entity_id = a.id
                AND e.entity_type = 'assertion'
                AND e.tenant_id = a.tenant_id
            WHERE a.id = :assertion_id
                AND a.tenant_id = :tenant_id
                AND a.is_deleted = false
        """)

        result = await self.session.execute(
            query,
            {
                "assertion_id": assertion_id,
                "tenant_id": str(self.tenant_id),
            },
        )
        row = result.fetchone()

        if not row:
            return None

        # Extract metadata
        metadata = row.metadata or {}
        participants = metadata.get("participants", [])
        project_refs = metadata.get("projects", [])

        # Parse embedding
        embedding = None
        if row.embedding:
            try:
                if isinstance(row.embedding, str):
                    import json

                    embedding = json.loads(row.embedding)
                else:
                    embedding = list(row.embedding)
            except (ValueError, TypeError):
                pass

        return ContentDetails(
            entity_id=row.id,
            entity_type="assertion",
            content_type=row.assertion_type,
            participants=participants,
            project_refs=project_refs,
            timestamp=row.created_at,
            embedding=embedding,
            thread_id=None,
            title=row.content[:100] if row.content else None,
        )

    def _extract_participants(self, metadata: dict[str, Any]) -> list[str]:
        """Extract participant identifiers from metadata."""
        participants: list[str] = []

        for key in ["from", "to", "cc", "participants", "attendees", "sender"]:
            if key in metadata:
                value = metadata[key]
                if isinstance(value, str):
                    participants.append(value.lower())
                elif isinstance(value, list):
                    participants.extend(str(v).lower() for v in value if v)

        # Deduplicate while preserving order
        seen: set[str] = set()
        unique: list[str] = []
        for p in participants:
            if p not in seen:
                seen.add(p)
                unique.append(p)

        return unique

    def _extract_project_refs(self, metadata: dict[str, Any]) -> list[str]:
        """Extract project references from metadata."""
        project_refs: list[str] = []

        for key in ["projects", "project", "labels", "tags"]:
            if key in metadata:
                value = metadata[key]
                if isinstance(value, str):
                    project_refs.append(value.lower())
                elif isinstance(value, list):
                    project_refs.extend(str(v).lower() for v in value if v)

        return list(set(project_refs))

    def _extract_title(self, metadata: dict[str, Any]) -> str | None:
        """Extract title from metadata."""
        for key in ["subject", "title", "name", "filename"]:
            if key in metadata and metadata[key]:
                return str(metadata[key])[:200]
        return None

    async def _find_by_participants(
        self,
        participants: list[str],
        exclude_id: int,
        exclude_type: str,
    ) -> list[tuple[int, str, str | None, str | None, datetime | None, float]]:
        """Find content with overlapping participants.

        Scores based on Jaccard similarity of participant sets.
        Uses the indexed participant_emails column with array overlap (&&)
        for efficient GIN index lookups, with JSONB fallback for legacy data.

        Returns:
            List of (entity_id, entity_type, content_type, title, timestamp, overlap_score)
        """
        if not participants:
            return []

        try:
            # Normalize participants to lowercase for matching
            normalized_participants = [p.lower() for p in participants]

            # Build ILIKE patterns for fallback on legacy data without participant_emails
            participant_patterns = [f"%{p}%" for p in normalized_participants]

            # Find sources with overlapping participants using indexed column
            # Primary: Use participant_emails array overlap (&&) with GIN index
            # Fallback: JSONB::text ILIKE for records where participant_emails is NULL
            query = text("""
                SELECT DISTINCT
                    s.id as entity_id,
                    'source' as entity_type,
                    s.content_type,
                    s.source_timestamp as timestamp,
                    s.ingestion_metadata,
                    s.participant_emails
                FROM sources s
                WHERE s.tenant_id = :tenant_id
                    AND s.is_deleted = false
                    AND NOT (s.id = :exclude_id AND :exclude_type = 'source')
                    AND (
                        -- Primary: GIN-indexed array overlap for participant_emails
                        s.participant_emails && :participants::text[]
                        -- Fallback: JSONB text search for legacy data without participant_emails
                        OR (
                            s.participant_emails IS NULL
                            AND s.ingestion_metadata::text ILIKE ANY(:participant_patterns::text[])
                        )
                    )
                LIMIT 100
            """)

            result = await self.session.execute(
                query,
                {
                    "participants": normalized_participants,
                    "participant_patterns": participant_patterns,
                    "tenant_id": str(self.tenant_id),
                    "exclude_id": exclude_id,
                    "exclude_type": exclude_type,
                },
            )

            matches: list[tuple[int, str, str | None, str | None, datetime | None, float]] = []
            for row in result:
                row_metadata = row.ingestion_metadata or {}

                # Calculate overlap score using participant_emails if available,
                # otherwise fall back to extracting from metadata
                if row.participant_emails:
                    row_participants = {p.lower() for p in row.participant_emails}
                else:
                    row_participants = set(self._extract_participants(row_metadata))
                source_set = set(normalized_participants)

                if not row_participants:
                    continue

                # Jaccard similarity
                intersection = len(source_set & row_participants)
                union = len(source_set | row_participants)
                overlap_score = intersection / union if union > 0 else 0.0

                if overlap_score > 0:
                    title = self._extract_title(row_metadata)
                    matches.append(
                        (
                            row.entity_id,
                            row.entity_type,
                            row.content_type,
                            title,
                            row.timestamp,
                            overlap_score,
                        )
                    )

            logger.debug(
                f"Participant correlation: found {len(matches)} matches for {len(participants)} participants"
            )
            return matches

        except Exception as e:
            logger.error(f"Error in participant correlation: {e}")
            return []

    async def _find_by_project(
        self,
        project_refs: list[str],
        exclude_id: int,
        exclude_type: str,
    ) -> list[tuple[int, str, str | None, str | None, datetime | None, float]]:
        """Find content referencing same projects.

        Returns:
            List of (entity_id, entity_type, content_type, title, timestamp, match_score)
        """
        if not project_refs:
            return []

        try:
            # Find sources with matching project references
            query = text("""
                SELECT
                    s.id as entity_id,
                    'source' as entity_type,
                    s.content_type,
                    s.source_timestamp as timestamp,
                    s.ingestion_metadata
                FROM sources s
                WHERE s.tenant_id = :tenant_id
                    AND s.is_deleted = false
                    AND NOT (s.id = :exclude_id AND :exclude_type = 'source')
                LIMIT 100
            """)

            result = await self.session.execute(
                query,
                {
                    "tenant_id": str(self.tenant_id),
                    "exclude_id": exclude_id,
                    "exclude_type": exclude_type,
                },
            )

            matches: list[tuple[int, str, str | None, str | None, datetime | None, float]] = []
            source_projects = {p.lower() for p in project_refs}

            for row in result:
                row_metadata = row.ingestion_metadata or {}
                row_projects = set(self._extract_project_refs(row_metadata))

                if not row_projects:
                    continue

                # Calculate match score
                intersection = len(source_projects & row_projects)
                if intersection > 0:
                    # Score based on proportion of matched projects
                    match_score = intersection / len(source_projects)
                    title = self._extract_title(row_metadata)
                    matches.append(
                        (
                            row.entity_id,
                            row.entity_type,
                            row.content_type,
                            title,
                            row.timestamp,
                            match_score,
                        )
                    )

            logger.debug(
                f"Project correlation: found {len(matches)} matches for {len(project_refs)} projects"
            )
            return matches

        except Exception as e:
            logger.error(f"Error in project correlation: {e}")
            return []

    async def _find_by_temporal_proximity(
        self,
        timestamp: datetime | None,
        exclude_id: int,
        exclude_type: str,
    ) -> list[tuple[int, str, str | None, str | None, datetime | None, float]]:
        """Find content within temporal window.

        Uses a decay function where closer content scores higher.

        Returns:
            List of (entity_id, entity_type, content_type, title, timestamp, proximity_score)
        """
        if not timestamp:
            return []

        try:
            # Calculate time window
            window_start = timestamp - timedelta(days=self.TEMPORAL_WINDOW_DAYS)
            window_end = timestamp + timedelta(days=self.TEMPORAL_WINDOW_DAYS)

            query = text("""
                SELECT
                    s.id as entity_id,
                    'source' as entity_type,
                    s.content_type,
                    s.source_timestamp as timestamp,
                    s.ingestion_metadata
                FROM sources s
                WHERE s.tenant_id = :tenant_id
                    AND s.is_deleted = false
                    AND s.source_timestamp IS NOT NULL
                    AND s.source_timestamp BETWEEN :window_start AND :window_end
                    AND NOT (s.id = :exclude_id AND :exclude_type = 'source')
                ORDER BY ABS(EXTRACT(EPOCH FROM (s.source_timestamp - :timestamp)))
                LIMIT 50
            """)

            result = await self.session.execute(
                query,
                {
                    "tenant_id": str(self.tenant_id),
                    "exclude_id": exclude_id,
                    "exclude_type": exclude_type,
                    "timestamp": timestamp,
                    "window_start": window_start,
                    "window_end": window_end,
                },
            )

            matches: list[tuple[int, str, str | None, str | None, datetime | None, float]] = []
            max_distance = self.TEMPORAL_WINDOW_DAYS * 24 * 3600  # seconds

            for row in result:
                if row.timestamp:
                    # Calculate temporal proximity score with decay
                    time_diff = abs((row.timestamp - timestamp).total_seconds())
                    # Exponential decay: closer content scores higher
                    proximity_score = 1.0 - (time_diff / max_distance)
                    proximity_score = max(0.0, proximity_score)

                    if proximity_score > 0:
                        row_metadata = row.ingestion_metadata or {}
                        title = self._extract_title(row_metadata)
                        matches.append(
                            (
                                row.entity_id,
                                row.entity_type,
                                row.content_type,
                                title,
                                row.timestamp,
                                proximity_score,
                            )
                        )

            logger.debug(f"Temporal correlation: found {len(matches)} matches within {self.TEMPORAL_WINDOW_DAYS} days")
            return matches

        except Exception as e:
            logger.error(f"Error in temporal correlation: {e}")
            return []

    async def _find_by_similarity(
        self,
        embedding: list[float] | None,
        exclude_id: int,
        exclude_type: str,
    ) -> list[tuple[int, str, str | None, str | None, datetime | None, float]]:
        """Find semantically similar content using vector similarity.

        Uses pgvector's cosine distance for similarity search.

        Returns:
            List of (entity_id, entity_type, content_type, title, timestamp, similarity_score)
        """
        if not embedding:
            return []

        try:
            # Use pgvector similarity search
            query = text("""
                SELECT
                    e.entity_id,
                    e.entity_type,
                    s.content_type,
                    s.source_timestamp as timestamp,
                    s.ingestion_metadata,
                    1 - (e.embedding <=> :embedding::vector) as similarity
                FROM embeddings e
                JOIN sources s ON s.id = e.entity_id
                    AND s.tenant_id = e.tenant_id
                    AND s.is_deleted = false
                WHERE e.tenant_id = :tenant_id
                    AND e.entity_type = 'source'
                    AND NOT (e.entity_id = :exclude_id AND :exclude_type = 'source')
                ORDER BY e.embedding <=> :embedding::vector
                LIMIT 30
            """)

            result = await self.session.execute(
                query,
                {
                    "tenant_id": str(self.tenant_id),
                    "embedding": str(embedding),
                    "exclude_id": exclude_id,
                    "exclude_type": exclude_type,
                },
            )

            matches: list[tuple[int, str, str | None, str | None, datetime | None, float]] = []
            for row in result:
                # Similarity score is already 0-1 range
                similarity = float(row.similarity) if row.similarity else 0.0
                if similarity > 0.3:  # Minimum similarity threshold
                    row_metadata = row.ingestion_metadata or {}
                    title = self._extract_title(row_metadata)
                    matches.append(
                        (
                            row.entity_id,
                            row.entity_type,
                            row.content_type,
                            title,
                            row.timestamp,
                            similarity,
                        )
                    )

            logger.debug(f"Semantic correlation: found {len(matches)} similar items")
            return matches

        except Exception as e:
            logger.error(f"Error in semantic correlation: {e}")
            return []

    async def _find_by_thread(
        self,
        thread_id: str | None,
        exclude_id: int,
        exclude_type: str,
    ) -> list[tuple[int, str, str | None, str | None, datetime | None, float]]:
        """Find content in same thread/conversation.

        Returns full score (1.0) for all items in the same thread.

        Returns:
            List of (entity_id, entity_type, content_type, title, timestamp, thread_score)
        """
        if not thread_id:
            return []

        try:
            query = text("""
                SELECT
                    s.id as entity_id,
                    'source' as entity_type,
                    s.content_type,
                    s.source_timestamp as timestamp,
                    s.ingestion_metadata
                FROM sources s
                WHERE s.tenant_id = :tenant_id
                    AND s.is_deleted = false
                    AND (
                        s.ingestion_metadata->>'thread_id' = :thread_id
                        OR s.ingestion_metadata->>'conversation_id' = :thread_id
                    )
                    AND NOT (s.id = :exclude_id AND :exclude_type = 'source')
                ORDER BY s.source_timestamp
                LIMIT 50
            """)

            result = await self.session.execute(
                query,
                {
                    "tenant_id": str(self.tenant_id),
                    "thread_id": thread_id,
                    "exclude_id": exclude_id,
                    "exclude_type": exclude_type,
                },
            )

            matches: list[tuple[int, str, str | None, str | None, datetime | None, float]] = []
            for row in result:
                row_metadata = row.ingestion_metadata or {}
                title = self._extract_title(row_metadata)
                # Thread matches get full score
                matches.append(
                    (
                        row.entity_id,
                        row.entity_type,
                        row.content_type,
                        title,
                        row.timestamp,
                        1.0,
                    )
                )

            logger.debug(f"Thread correlation: found {len(matches)} items in thread {thread_id[:20]}...")
            return matches

        except Exception as e:
            logger.error(f"Error in thread correlation: {e}")
            return []

    def _merge_and_score(
        self,
        participant_matches: list[tuple[int, str, str | None, str | None, datetime | None, float]],
        project_matches: list[tuple[int, str, str | None, str | None, datetime | None, float]],
        temporal_matches: list[tuple[int, str, str | None, str | None, datetime | None, float]],
        semantic_matches: list[tuple[int, str, str | None, str | None, datetime | None, float]],
        thread_matches: list[tuple[int, str, str | None, str | None, datetime | None, float]],
    ) -> list[RelatedItem]:
        """Merge matches from all signals and compute final weighted scores.

        Aggregates scores from all signal types, applies weights, and
        creates RelatedItem objects with complete metadata.

        Args:
            participant_matches: Matches from participant overlap
            project_matches: Matches from project references
            temporal_matches: Matches from temporal proximity
            semantic_matches: Matches from semantic similarity
            thread_matches: Matches from thread chains

        Returns:
            List of RelatedItem objects with computed scores
        """
        # Aggregate scores and metadata by entity
        scores: dict[tuple[int, str], dict[str, float]] = {}
        metadata: dict[tuple[int, str], dict[str, Any]] = {}

        signal_matches = [
            ("participants", participant_matches),
            ("project", project_matches),
            ("temporal", temporal_matches),
            ("semantic", semantic_matches),
            ("thread", thread_matches),
        ]

        for signal_name, matches in signal_matches:
            for entity_id, entity_type, content_type, title, timestamp, signal_score in matches:
                key = (entity_id, entity_type)

                if key not in scores:
                    scores[key] = {}
                    metadata[key] = {
                        "content_type": content_type,
                        "title": title,
                        "timestamp": timestamp,
                    }

                scores[key][signal_name] = signal_score

                # Update metadata if this has better info
                if title and not metadata[key].get("title"):
                    metadata[key]["title"] = title
                if timestamp and not metadata[key].get("timestamp"):
                    metadata[key]["timestamp"] = timestamp

        # Compute weighted final scores and create RelatedItem objects
        results: list[RelatedItem] = []
        for (entity_id, entity_type), signal_scores in scores.items():
            # Calculate weighted sum
            weighted_score = sum(
                self.weights.get(signal, 0.0) * score
                for signal, score in signal_scores.items()
            )

            # Determine primary correlation type (highest signal)
            primary_type = "unknown"
            if signal_scores:
                primary_type = max(signal_scores.items(), key=lambda x: x[1])[0]

            # Get metadata
            meta = metadata.get((entity_id, entity_type), {})
            content_type_raw = meta.get("content_type")

            # Map content type to enum
            content_type_enum = self._map_content_type(content_type_raw)

            results.append(
                RelatedItem(
                    entity_id=entity_id,
                    entity_type=entity_type,
                    content_type=content_type_enum,
                    title=meta.get("title"),
                    timestamp=meta.get("timestamp") or datetime.now(timezone.utc),
                    correlation_score=weighted_score,
                    correlation_type=primary_type,
                    score_breakdown=signal_scores,
                )
            )

        return results

    def _map_content_type(self, raw_type: str | None) -> ContentTypeFilter:
        """Map raw content type string to ContentTypeFilter enum."""
        if raw_type is None:
            return ContentTypeFilter.ALL

        raw_lower = raw_type.lower()

        if "email" in raw_lower or "gmail" in raw_lower:
            return ContentTypeFilter.EMAIL
        elif "meeting" in raw_lower or "calendar" in raw_lower:
            return ContentTypeFilter.MEETING
        elif "slack" in raw_lower:
            return ContentTypeFilter.SLACK
        elif "document" in raw_lower or "doc" in raw_lower:
            return ContentTypeFilter.DOCUMENT
        else:
            return ContentTypeFilter.ALL


async def find_related_by_person(
    session: AsyncSession,
    tenant_id: str,
    person_identifier: str,
    limit: int = 20,
) -> list[RelatedItem]:
    """Find content related to a specific person.

    Convenience function to find all content involving a person.
    Uses the indexed participant_emails column with array contains (@>)
    for efficient GIN index lookups, with JSONB fallback for legacy data.

    Args:
        session: Database session
        tenant_id: Tenant UUID string
        person_identifier: Email or name of person
        limit: Maximum items to return

    Returns:
        List of RelatedItem objects
    """
    try:
        # Normalize person identifier to lowercase for matching
        normalized_person = person_identifier.lower()
        person_pattern = f"%{normalized_person}%"

        # Primary: Use participant_emails array contains (@>) with GIN index
        # Fallback: JSONB::text ILIKE for records where participant_emails is NULL
        query = text("""
            SELECT
                s.id as entity_id,
                'source' as entity_type,
                s.content_type,
                s.source_timestamp as timestamp,
                s.ingestion_metadata
            FROM sources s
            WHERE s.tenant_id = :tenant_id
                AND s.is_deleted = false
                AND (
                    -- Primary: GIN-indexed array contains for participant_emails
                    s.participant_emails @> ARRAY[:person]::text[]
                    -- Fallback: JSONB text search for legacy data without participant_emails
                    OR (
                        s.participant_emails IS NULL
                        AND s.ingestion_metadata::text ILIKE :person_pattern
                    )
                )
            ORDER BY s.source_timestamp DESC
            LIMIT :limit
        """)

        result = await session.execute(
            query,
            {
                "tenant_id": str(tenant_id),
                "person": normalized_person,
                "person_pattern": person_pattern,
                "limit": limit,
            },
        )

        items: list[RelatedItem] = []
        for row in result:
            metadata = row.ingestion_metadata or {}
            title = None
            for field in ["subject", "title", "name"]:
                if field in metadata:
                    title = str(metadata[field])[:200]
                    break

            content_type = ContentTypeFilter.ALL
            if row.content_type:
                raw_lower = row.content_type.lower()
                if "email" in raw_lower:
                    content_type = ContentTypeFilter.EMAIL
                elif "meeting" in raw_lower:
                    content_type = ContentTypeFilter.MEETING
                elif "slack" in raw_lower:
                    content_type = ContentTypeFilter.SLACK
                elif "document" in raw_lower:
                    content_type = ContentTypeFilter.DOCUMENT

            items.append(
                RelatedItem(
                    entity_id=row.entity_id,
                    entity_type=row.entity_type,
                    content_type=content_type,
                    title=title,
                    timestamp=row.timestamp or datetime.now(timezone.utc),
                    correlation_score=1.0,
                    correlation_type="participants",
                    score_breakdown={"participants": 1.0},
                )
            )

        return items

    except Exception as e:
        logger.error(f"Error finding content by person: {e}")
        return []


async def find_related_by_project(
    session: AsyncSession,
    tenant_id: str,
    project_name: str,
    limit: int = 20,
) -> list[RelatedItem]:
    """Find content related to a specific project.

    Convenience function to find all content mentioning a project.

    Args:
        session: Database session
        tenant_id: Tenant UUID string
        project_name: Name of the project
        limit: Maximum items to return

    Returns:
        List of RelatedItem objects
    """
    try:
        # NOTE: ILIKE on JSONB::text and raw_content is a known performance limitation.
        # Future optimization: use PostgreSQL full-text search with GIN indexes.
        query = text("""
            SELECT
                s.id as entity_id,
                'source' as entity_type,
                s.content_type,
                s.source_timestamp as timestamp,
                s.ingestion_metadata
            FROM sources s
            WHERE s.tenant_id = :tenant_id
                AND s.is_deleted = false
                AND (
                    s.ingestion_metadata::text ILIKE :project_pattern
                    OR s.raw_content ILIKE :project_pattern
                )
            ORDER BY s.source_timestamp DESC
            LIMIT :limit
        """)

        result = await session.execute(
            query,
            {
                "tenant_id": str(tenant_id),
                "project_pattern": f"%{project_name.lower()}%",
                "limit": limit,
            },
        )

        items: list[RelatedItem] = []
        for row in result:
            metadata = row.ingestion_metadata or {}
            title = None
            for field in ["subject", "title", "name"]:
                if field in metadata:
                    title = str(metadata[field])[:200]
                    break

            content_type = ContentTypeFilter.ALL
            if row.content_type:
                raw_lower = row.content_type.lower()
                if "email" in raw_lower:
                    content_type = ContentTypeFilter.EMAIL
                elif "meeting" in raw_lower:
                    content_type = ContentTypeFilter.MEETING
                elif "slack" in raw_lower:
                    content_type = ContentTypeFilter.SLACK
                elif "document" in raw_lower:
                    content_type = ContentTypeFilter.DOCUMENT

            items.append(
                RelatedItem(
                    entity_id=row.entity_id,
                    entity_type=row.entity_type,
                    content_type=content_type,
                    title=title,
                    timestamp=row.timestamp or datetime.now(timezone.utc),
                    correlation_score=1.0,
                    correlation_type="project",
                    score_breakdown={"project": 1.0},
                )
            )

        return items

    except Exception as e:
        logger.error(f"Error finding content by project: {e}")
        return []
