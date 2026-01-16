"""Escalation briefing workflow for comprehensive entity context."""

import logging
import uuid
from dataclasses import dataclass, field
from datetime import datetime, timedelta
from typing import Any, Dict, List, Optional

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
        """Gather relevant sources for entities within timeframe."""
        # TODO: Implement source gathering using search_engine
        logger.info(f"Gathering sources for {len(entities)} entities")
        return []  # Placeholder

    async def _expand_correlations(
        self,
        sources: List[Dict[str, Any]],
    ) -> List[Dict[str, Any]]:
        """Expand source list via correlation discovery."""
        # TODO: Implement correlation expansion
        logger.info(f"Expanding correlations for {len(sources)} sources")
        return sources  # Placeholder

    async def _build_timeline(
        self,
        sources: List[Dict[str, Any]],
    ) -> List[TimelineEvent]:
        """Build chronological timeline from sources."""
        # TODO: Implement timeline construction
        logger.info(f"Building timeline from {len(sources)} sources")
        return []  # Placeholder

    async def _extract_insights(
        self,
        sources: List[Dict[str, Any]],
        timeline: List[TimelineEvent],
    ) -> List[str]:
        """Extract key insights from sources and timeline."""
        # TODO: Implement insight extraction
        logger.info("Extracting insights")
        return []  # Placeholder

    async def _generate_recommendations(
        self,
        insights: List[str],
    ) -> List[str]:
        """Generate recommended actions based on insights."""
        # TODO: Implement recommendation generation
        logger.info("Generating recommendations")
        return []  # Placeholder

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
