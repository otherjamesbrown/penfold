"""Escalation briefing workflow for comprehensive entity context."""

import asyncio
import logging
import uuid
from dataclasses import dataclass, field
from datetime import datetime, timedelta, timezone
from typing import Any, Dict, List, Optional

from sqlalchemy import text

from .base import WorkflowBase, WorkflowResult, SourceCitation

logger = logging.getLogger(__name__)


@dataclass
class EntityMatch:
    """Matched entity from resolution."""

    entity_type: str  # person, project, topic, unknown
    entity_name: str
    entity_id: Optional[int] = None
    identifiers: List[str] = field(default_factory=list)  # emails, aliases
    confidence: float = 0.0


@dataclass
class TimelineEvent:
    """Event in the escalation timeline."""

    timestamp: datetime
    event_type: str
    summary: str
    source_id: int
    source_type: str
    participants: List[str] = field(default_factory=list)
    importance: str = "normal"  # low, normal, high, critical


@dataclass
class EscalationBriefingResult(WorkflowResult):
    """Result of an escalation briefing workflow."""

    entity_query: str = ""
    resolved_entities: List[str] = field(default_factory=list)
    timeframe_days: int = 180
    timeline: List[TimelineEvent] = field(default_factory=list)
    key_insights: List[str] = field(default_factory=list)
    recommended_actions: List[str] = field(default_factory=list)
    correlation_count: int = 0
    briefing_markdown: str = ""


class EscalationBriefing(WorkflowBase):
    """Orchestrates comprehensive escalation briefing generation.

    This workflow implements the core value proposition: generating
    15-minute escalation briefings by:
    1. Resolving entity references
    2. Gathering correlated content
    3. Reconstructing timeline
    4. Generating structured briefing

    Example:
        ```python
        async with get_session() as session:
            workflow = EscalationBriefing(session, tenant_id)
            result = await workflow.execute("Project Alpha", timeframe_days=30)
            print(result.briefing_markdown)
        ```
    """

    DEFAULT_TIMEFRAME_DAYS = 180

    def __init__(
        self,
        session,
        tenant_id: str,
        config: Optional[Dict[str, Any]] = None,
    ):
        """Initialize escalation briefing workflow.

        Args:
            session: Database session
            tenant_id: Current tenant ID
            config: Optional configuration with keys:
                - max_results: Maximum sources to consider (default: 100)
                - include_correlations: Whether to expand via correlations (default: True)
        """
        super().__init__(session, tenant_id, config)
        self.max_results = self.config.get("max_results", 100)
        self.include_correlations = self.config.get("include_correlations", True)

    async def execute(
        self,
        entity_query: str,
        timeframe_days: int = DEFAULT_TIMEFRAME_DAYS,
    ) -> EscalationBriefingResult:
        """Execute escalation briefing workflow.

        Args:
            entity_query: Entity to brief on (person, project, topic)
            timeframe_days: Number of days to look back (default: 180)

        Returns:
            EscalationBriefingResult with timeline, insights, and markdown briefing
        """
        self._start_timing()
        errors: List[str] = []

        try:
            # Step 1: Resolve entity references
            resolved_entities = await self._resolve_entities(entity_query)

            # Step 2: Gather relevant sources
            sources = await self._gather_sources(resolved_entities, timeframe_days)

            # Step 3: Expand via correlations if enabled
            if self.include_correlations:
                sources = await self._expand_correlations(sources)

            # Step 4: Reconstruct timeline
            timeline = await self._build_timeline(sources)

            # Step 5: Extract key insights
            insights = await self._extract_insights(sources, timeline)

            # Step 6: Generate recommendations
            recommendations = await self._generate_recommendations(insights)

            # Step 7: Generate markdown briefing
            briefing_md = self._generate_briefing_markdown(
                entity_query,
                resolved_entities,
                timeline,
                insights,
                recommendations,
            )

            return EscalationBriefingResult(
                success=True,
                execution_time_ms=self._get_elapsed_ms(),
                sources_consulted=len(sources),
                citations=self._get_citations(),
                entity_query=entity_query,
                resolved_entities=resolved_entities,
                timeframe_days=timeframe_days,
                timeline=timeline,
                key_insights=insights,
                recommended_actions=recommendations,
                correlation_count=len(sources),
                briefing_markdown=briefing_md,
            )

        except Exception as e:
            logger.exception(f"Escalation briefing failed for '{entity_query}'")
            errors.append(str(e))
            return EscalationBriefingResult(
                success=False,
                execution_time_ms=self._get_elapsed_ms(),
                errors=errors,
                entity_query=entity_query,
                timeframe_days=timeframe_days,
            )

    async def _resolve_entities(self, query: str) -> List[str]:
        """Resolve entity query to canonical identifiers.

        Searches across Person, Source tables to find matching entities.
        Returns list of resolved entity identifiers.
        """
        from penf_lib.storage.repositories import PersonRepository

        resolved: List[str] = []
        tenant_uuid = uuid.UUID(self.tenant_id)

        try:
            person_repo = PersonRepository(self.session)

            # Try exact email match first
            if "@" in query:
                person = await person_repo.find_by_email(tenant_uuid, query.lower())
                if person:
                    resolved.append(person.canonical_name or query)
                    logger.info(
                        f"Resolved email '{query}' to person: {person.canonical_name}"
                    )
                    return resolved

            # Search by name (returns multiple matches)
            persons = await person_repo.search_by_name(tenant_uuid, query, limit=3)
            if persons:
                for person in persons:
                    if person.canonical_name:
                        resolved.append(person.canonical_name)
                logger.info(
                    f"Resolved name '{query}' to {len(resolved)} person(s): {resolved}"
                )
                return resolved

            # Try exact name match as fallback
            person = await person_repo.find_by_name(tenant_uuid, query)
            if person and person.canonical_name:
                resolved.append(person.canonical_name)
                logger.info(
                    f"Resolved exact name '{query}' to person: {person.canonical_name}"
                )
                return resolved

        except Exception as e:
            logger.warning(f"Entity resolution failed for '{query}': {e}")

        # If no person found, use query as-is (topic/project search)
        if not resolved:
            resolved.append(query)
            logger.info(f"No entity match for '{query}', using as-is")

        return resolved

    async def _gather_sources(
        self,
        entities: List[str],
        timeframe_days: int,
    ) -> List[Dict[str, Any]]:
        """Gather relevant sources for entities within timeframe.

        Searches for sources that mention the entities within the given timeframe.
        Uses full-text search and participant matching.

        Args:
            entities: List of resolved entity names to search for
            timeframe_days: Number of days to look back

        Returns:
            List of source dictionaries with metadata
        """
        if not entities:
            return []

        logger.info(f"Gathering sources for {len(entities)} entities over {timeframe_days} days")

        cutoff_date = datetime.now(timezone.utc) - timedelta(days=timeframe_days)
        all_sources: Dict[int, Dict[str, Any]] = {}

        try:
            # Search for sources mentioning each entity
            for entity in entities:
                # Normalize entity for search
                entity_lower = entity.lower()

                # Search via participant_emails (GIN-indexed) and content
                query = text("""
                    SELECT DISTINCT
                        s.id,
                        s.content_type,
                        s.source_system,
                        s.source_timestamp,
                        s.raw_content,
                        s.ingestion_metadata,
                        s.participant_emails
                    FROM sources s
                    WHERE s.tenant_id = :tenant_id
                        AND s.is_deleted = false
                        AND (s.source_timestamp IS NULL OR s.source_timestamp >= :cutoff)
                        AND (
                            -- Match by participant_emails array (GIN index)
                            EXISTS (
                                SELECT 1 FROM unnest(s.participant_emails) AS pe
                                WHERE pe ILIKE :entity_pattern
                            )
                            -- Match by content
                            OR s.raw_content ILIKE :entity_pattern
                            -- Match by metadata
                            OR s.ingestion_metadata::text ILIKE :entity_pattern
                        )
                    ORDER BY s.source_timestamp DESC NULLS LAST
                    LIMIT :limit
                """)

                result = await self.session.execute(
                    query,
                    {
                        "tenant_id": self.tenant_id,
                        "cutoff": cutoff_date,
                        "entity_pattern": f"%{entity_lower}%",
                        "limit": self.max_results,
                    },
                )

                for row in result:
                    if row.id not in all_sources:
                        metadata = row.ingestion_metadata or {}
                        all_sources[row.id] = {
                            "id": row.id,
                            "content_type": row.content_type,
                            "source_system": row.source_system,
                            "timestamp": row.source_timestamp,
                            "content": row.raw_content,
                            "metadata": metadata,
                            "participant_emails": row.participant_emails or [],
                            "title": self._extract_title_from_metadata(metadata),
                            "matched_entities": [entity],
                        }
                    else:
                        # Add entity to matched list
                        if entity not in all_sources[row.id]["matched_entities"]:
                            all_sources[row.id]["matched_entities"].append(entity)

            sources = list(all_sources.values())
            logger.info(f"Gathered {len(sources)} unique sources for {len(entities)} entities")
            return sources

        except Exception as e:
            logger.error(f"Error gathering sources: {e}")
            return []

    def _extract_title_from_metadata(self, metadata: Dict[str, Any]) -> Optional[str]:
        """Extract title from source metadata."""
        for key in ["subject", "title", "name", "filename"]:
            if key in metadata and metadata[key]:
                return str(metadata[key])[:200]
        return None

    async def _expand_correlations(
        self,
        sources: List[Dict[str, Any]],
    ) -> List[Dict[str, Any]]:
        """Expand source list via correlation discovery.

        Uses the CorrelationDiscovery engine to find related content
        that wasn't directly matched by entity search.

        Args:
            sources: Initial list of matched sources

        Returns:
            Expanded list including correlated sources
        """
        if not sources:
            return sources

        logger.info(f"Expanding correlations for {len(sources)} sources")

        try:
            from penf_lib.search.correlations import CorrelationDiscovery

            discovery = CorrelationDiscovery(self.session, self.tenant_id)
            existing_ids = {s["id"] for s in sources}
            correlated_sources: Dict[int, Dict[str, Any]] = {}

            # Find correlations for each source (limit concurrency)
            correlation_tasks = []
            for source in sources[:20]:  # Limit to top 20 sources for performance
                correlation_tasks.append(
                    discovery.find_related_content(
                        content_id=source["id"],
                        content_type="source",
                        limit=5,
                    )
                )

            # Execute in batches to control concurrency
            results = await asyncio.gather(*correlation_tasks, return_exceptions=True)

            for result in results:
                if isinstance(result, Exception):
                    logger.warning(f"Correlation discovery failed: {result}")
                    continue

                for related in result.related_items:
                    if related.entity_id in existing_ids:
                        continue
                    if related.entity_id in correlated_sources:
                        continue

                    # Fetch the correlated source details
                    query = text("""
                        SELECT
                            s.id,
                            s.content_type,
                            s.source_system,
                            s.source_timestamp,
                            s.raw_content,
                            s.ingestion_metadata,
                            s.participant_emails
                        FROM sources s
                        WHERE s.id = :source_id
                            AND s.tenant_id = :tenant_id
                            AND s.is_deleted = false
                    """)

                    fetch_result = await self.session.execute(
                        query,
                        {"source_id": related.entity_id, "tenant_id": self.tenant_id},
                    )
                    row = fetch_result.fetchone()

                    if row:
                        metadata = row.ingestion_metadata or {}
                        correlated_sources[row.id] = {
                            "id": row.id,
                            "content_type": row.content_type,
                            "source_system": row.source_system,
                            "timestamp": row.source_timestamp,
                            "content": row.raw_content,
                            "metadata": metadata,
                            "participant_emails": row.participant_emails or [],
                            "title": self._extract_title_from_metadata(metadata),
                            "matched_entities": [],
                            "correlation_type": related.correlation_type,
                            "correlation_score": related.correlation_score,
                        }

            # Merge correlated sources with original sources
            expanded = sources + list(correlated_sources.values())
            logger.info(
                f"Correlation expansion: {len(sources)} -> {len(expanded)} sources "
                f"(+{len(correlated_sources)} correlated)"
            )
            return expanded

        except ImportError:
            logger.warning("CorrelationDiscovery not available, skipping expansion")
            return sources
        except Exception as e:
            logger.error(f"Error in correlation expansion: {e}")
            return sources

    async def _build_timeline(
        self,
        sources: List[Dict[str, Any]],
    ) -> List[TimelineEvent]:
        """Build chronological timeline from sources.

        Converts sources into timeline events, sorted chronologically
        with importance levels based on content analysis.

        Args:
            sources: List of source dictionaries

        Returns:
            List of TimelineEvent objects sorted by timestamp
        """
        if not sources:
            return []

        logger.info(f"Building timeline from {len(sources)} sources")

        events: List[TimelineEvent] = []

        for source in sources:
            timestamp = source.get("timestamp")
            if not timestamp:
                continue  # Skip sources without timestamps

            # Ensure timestamp is datetime
            if isinstance(timestamp, str):
                try:
                    timestamp = datetime.fromisoformat(timestamp.replace("Z", "+00:00"))
                except ValueError:
                    continue

            # Determine event type from content type
            content_type = source.get("content_type", "unknown") or "unknown"
            event_type = self._classify_event_type(content_type)

            # Extract participants from metadata or participant_emails
            participants = source.get("participant_emails", [])
            if not participants:
                metadata = source.get("metadata", {})
                participants = self._extract_participants_from_metadata(metadata)

            # Generate summary
            title = source.get("title")
            summary = title if title else self._generate_summary(source)

            # Determine importance based on content signals
            importance = self._assess_importance(source)

            # Add citation for audit trail
            self._add_citation(
                SourceCitation(
                    source_id=source["id"],
                    source_type=content_type,
                    title=title or f"{event_type} from {source.get('source_system', 'unknown')}",
                    timestamp=timestamp,
                    relevance_score=source.get("correlation_score", 1.0),
                )
            )

            events.append(
                TimelineEvent(
                    timestamp=timestamp,
                    event_type=event_type,
                    summary=summary,
                    source_id=source["id"],
                    source_type=content_type,
                    participants=participants[:10],  # Limit participants shown
                    importance=importance,
                )
            )

        # Sort by timestamp (oldest first for narrative flow)
        events.sort(key=lambda e: e.timestamp)

        logger.info(f"Built timeline with {len(events)} events")
        return events

    def _classify_event_type(self, content_type: str) -> str:
        """Classify content type into event type."""
        content_lower = content_type.lower()
        if "email" in content_lower or "gmail" in content_lower:
            return "email"
        elif "meeting" in content_lower or "calendar" in content_lower:
            return "meeting"
        elif "slack" in content_lower:
            return "message"
        elif "document" in content_lower or "doc" in content_lower:
            return "document"
        elif "note" in content_lower:
            return "note"
        else:
            return "communication"

    def _extract_participants_from_metadata(self, metadata: Dict[str, Any]) -> List[str]:
        """Extract participants from metadata."""
        participants: List[str] = []
        for key in ["from", "to", "cc", "participants", "attendees", "sender"]:
            if key in metadata:
                value = metadata[key]
                if isinstance(value, str):
                    participants.append(value)
                elif isinstance(value, list):
                    participants.extend(str(v) for v in value if v)
        return list(dict.fromkeys(participants))[:20]  # Dedupe, limit

    def _generate_summary(self, source: Dict[str, Any]) -> str:
        """Generate a brief summary for a source without a title."""
        content = source.get("content", "")
        if content:
            # Take first 100 chars, clean up
            summary = content[:100].replace("\n", " ").strip()
            if len(content) > 100:
                summary += "..."
            return summary
        return f"{source.get('source_system', 'Unknown')} content"

    def _assess_importance(self, source: Dict[str, Any]) -> str:
        """Assess importance level of a source.

        Returns: low, normal, high, or critical
        """
        metadata = source.get("metadata", {})
        content = (source.get("content") or "").lower()

        # Check for critical indicators
        critical_keywords = ["urgent", "critical", "emergency", "asap", "immediately"]
        if any(kw in content for kw in critical_keywords):
            return "critical"

        # Check for high importance indicators
        high_keywords = ["important", "priority", "deadline", "blocking", "escalate"]
        if any(kw in content for kw in high_keywords):
            return "high"

        # Check metadata for importance flags
        if metadata.get("importance") in ["high", "urgent"]:
            return "high"
        if metadata.get("priority") in ["high", "urgent", "1", 1]:
            return "high"

        # Meeting invites are often important
        if source.get("content_type", "").lower() in ["meeting", "calendar"]:
            return "normal"

        # Correlation score can indicate importance
        correlation_score = source.get("correlation_score", 0)
        if correlation_score > 0.8:
            return "high"

        return "normal"

    async def _extract_insights(
        self,
        sources: List[Dict[str, Any]],
        timeline: List[TimelineEvent],
    ) -> List[str]:
        """Extract key insights from sources and timeline.

        Analyzes patterns in the timeline and sources to identify
        key observations for the briefing.

        Args:
            sources: List of source dictionaries
            timeline: List of timeline events

        Returns:
            List of insight strings
        """
        if not sources and not timeline:
            return []

        logger.info("Extracting insights")
        insights: List[str] = []

        # Insight 1: Communication volume and frequency
        if timeline:
            total_events = len(timeline)
            if total_events > 0:
                first_event = timeline[0]
                last_event = timeline[-1]
                time_span = (last_event.timestamp - first_event.timestamp).days + 1
                if time_span > 0:
                    avg_per_day = total_events / time_span
                    if avg_per_day > 5:
                        insights.append(
                            f"High communication activity: {total_events} events "
                            f"over {time_span} days (avg {avg_per_day:.1f}/day)"
                        )
                    elif total_events >= 3:
                        insights.append(
                            f"Moderate activity: {total_events} events "
                            f"over {time_span} days"
                        )

        # Insight 2: Key participants
        participant_counts: Dict[str, int] = {}
        for event in timeline:
            for participant in event.participants:
                participant_counts[participant] = participant_counts.get(participant, 0) + 1

        if participant_counts:
            top_participants = sorted(
                participant_counts.items(), key=lambda x: x[1], reverse=True
            )[:5]
            if top_participants and top_participants[0][1] > 1:
                names = [p[0].split("@")[0] for p, _ in top_participants[:3]]
                insights.append(f"Key participants: {', '.join(names)}")

        # Insight 3: High priority items
        critical_events = [e for e in timeline if e.importance == "critical"]
        high_events = [e for e in timeline if e.importance == "high"]

        if critical_events:
            insights.append(
                f"{len(critical_events)} critical priority item(s) found"
            )
        if high_events:
            insights.append(
                f"{len(high_events)} high priority item(s) require attention"
            )

        # Insight 4: Communication channels used
        channel_counts: Dict[str, int] = {}
        for event in timeline:
            channel_counts[event.event_type] = channel_counts.get(event.event_type, 0) + 1

        if len(channel_counts) > 1:
            channels = [f"{k}({v})" for k, v in sorted(
                channel_counts.items(), key=lambda x: x[1], reverse=True
            )]
            insights.append(f"Communication channels: {', '.join(channels)}")

        # Insight 5: Recent activity trend
        if len(timeline) >= 5:
            mid_point = len(timeline) // 2
            recent_count = len(timeline) - mid_point
            earlier_count = mid_point
            if recent_count > earlier_count * 1.5:
                insights.append("Activity has been increasing recently")
            elif earlier_count > recent_count * 1.5:
                insights.append("Activity has been decreasing recently")

        # Insight 6: Correlated content found
        correlated_sources = [s for s in sources if s.get("correlation_type")]
        if correlated_sources:
            types = set(s.get("correlation_type", "unknown") for s in correlated_sources)
            insights.append(
                f"Found {len(correlated_sources)} related items via correlation "
                f"({', '.join(types)})"
            )

        logger.info(f"Extracted {len(insights)} insights")
        return insights

    async def _generate_recommendations(
        self,
        insights: List[str],
    ) -> List[str]:
        """Generate recommended actions based on insights.

        Creates actionable recommendations based on the extracted insights.

        Args:
            insights: List of insight strings

        Returns:
            List of recommendation strings
        """
        logger.info("Generating recommendations")
        recommendations: List[str] = []

        insights_text = " ".join(insights).lower()

        # Recommendation based on critical items
        if "critical" in insights_text:
            recommendations.append("Review critical priority items immediately")

        # Recommendation based on high priority items
        if "high priority" in insights_text:
            recommendations.append("Schedule time to address high priority items")

        # Recommendation based on activity trends
        if "increasing" in insights_text:
            recommendations.append("Consider proactive outreach given increased activity")
        elif "decreasing" in insights_text:
            recommendations.append("Check if follow-up is needed given decreased activity")

        # Recommendation based on participant patterns
        if "key participants" in insights_text:
            recommendations.append("Sync with key participants for context")

        # Recommendation based on correlated content
        if "correlation" in insights_text:
            recommendations.append("Review correlated content for additional context")

        # General recommendations if none generated
        if not recommendations:
            recommendations.append("Review timeline for any action items")
            recommendations.append("Consider scheduling a sync meeting if needed")

        logger.info(f"Generated {len(recommendations)} recommendations")
        return recommendations

    def _generate_briefing_markdown(
        self,
        entity_query: str,
        resolved_entities: List[str],
        timeline: List[TimelineEvent],
        insights: List[str],
        recommendations: List[str],
    ) -> str:
        """Generate markdown-formatted briefing document."""
        lines = [
            f"# Escalation Briefing: {entity_query}",
            "",
            f"**Generated:** {datetime.utcnow().strftime('%Y-%m-%d %H:%M UTC')}",
            f"**Execution Time:** {self._get_elapsed_ms():.1f}ms",
            "",
            "## Resolved Entities",
            "",
        ]

        for entity in resolved_entities:
            lines.append(f"- {entity}")

        lines.extend([
            "",
            "## Timeline",
            "",
        ])

        if timeline:
            for event in timeline:
                lines.append(
                    f"- **{event.timestamp.strftime('%Y-%m-%d')}** - "
                    f"{event.summary} ({event.event_type})"
                )
        else:
            lines.append("_No timeline events found._")

        lines.extend([
            "",
            "## Key Insights",
            "",
        ])

        if insights:
            for insight in insights:
                lines.append(f"- {insight}")
        else:
            lines.append("_No insights extracted._")

        lines.extend([
            "",
            "## Recommended Actions",
            "",
        ])

        if recommendations:
            for rec in recommendations:
                lines.append(f"- [ ] {rec}")
        else:
            lines.append("_No recommendations generated._")

        lines.extend([
            "",
            "---",
            f"_Sources consulted: {len(self._citations)}_",
        ])

        return "\n".join(lines)
