"""Search result ranking with multi-signal scoring.

This module provides ranking algorithms that combine multiple
relevance signals to produce final search result ordering.

Key components:
- RRFFusion: Reciprocal Rank Fusion for combining ranked lists
- SearchRanker: Multi-signal ranking with configurable weights
- RankedResult: Search result with computed ranking score

Reference: specs/007-search-interface/research.md
"""

from __future__ import annotations

import logging
import math
from dataclasses import dataclass
from datetime import datetime, timezone

from penf_lib.search.models import ContentTypeFilter, SearchResult

logger = logging.getLogger(__name__)


@dataclass
class RankedResult:
    """Search result with computed ranking score.

    Holds the search result along with ranking scores and detailed breakdown.

    Attributes:
        result: The underlying SearchResult.
        final_score: Combined weighted score from all signals.
        score_breakdown: Individual scores for each ranking signal.
        rrf_score: Raw RRF fusion score.
        personalization_boost: Boost from user preferences.
        recency_boost: Boost from content freshness.
        rank_position: Final rank position (1-indexed).
    """

    result: SearchResult
    final_score: float
    score_breakdown: dict[str, float] = None  # type: ignore[assignment]
    rrf_score: float = 0.0
    personalization_boost: float = 0.0
    recency_boost: float = 0.0
    rank_position: int = 0

    def __post_init__(self) -> None:
        """Initialize score_breakdown if not provided."""
        if self.score_breakdown is None:
            self.score_breakdown = {}


class RRFFusion:
    """Reciprocal Rank Fusion for combining ranked lists.

    RRF provides a simple, effective method for combining ranked lists
    from different retrieval methods without requiring score normalization.

    Formula: RRF(d) = sum(1 / (k + rank_i(d)))

    The k parameter controls how much impact high ranks have relative to
    lower ranks. The standard value of k=60 provides balanced weighting.

    Attributes:
        k: Ranking constant (default 60, standard value).

    Example:
        >>> rrf = RRFFusion(k=60)
        >>> list1 = [(1, 0.9), (2, 0.8), (3, 0.7)]  # (entity_id, score)
        >>> list2 = [(2, 0.95), (1, 0.85), (4, 0.6)]
        >>> fused = rrf.fuse(list1, list2)
        >>> # Entity 2 appears high in both lists, so ranks highest
    """

    def __init__(self, k: int = 60) -> None:
        """Initialize RRF fusion.

        Args:
            k: Ranking constant. Higher k reduces impact of high ranks.
               Standard value is 60. Must be at least 1.

        Raises:
            ValueError: If k is less than 1.
        """
        if k < 1:
            raise ValueError("k must be at least 1")
        self.k = k

    def fuse(
        self,
        *ranked_lists: list[tuple[int, float]],
    ) -> list[tuple[int, float]]:
        """Fuse multiple ranked lists using RRF.

        Args:
            *ranked_lists: Variable number of ranked lists.
                          Each list contains (entity_id, score) tuples
                          ordered by score descending.

        Returns:
            Fused list of (entity_id, rrf_score) tuples, sorted by RRF score
            descending.
        """
        rrf_scores: dict[int, float] = {}

        for ranked_list in ranked_lists:
            for rank, (entity_id, _score) in enumerate(ranked_list, start=1):
                if entity_id not in rrf_scores:
                    rrf_scores[entity_id] = 0.0
                rrf_scores[entity_id] += 1.0 / (self.k + rank)

        # Sort by RRF score descending
        sorted_results = sorted(
            rrf_scores.items(),
            key=lambda x: x[1],
            reverse=True,
        )

        logger.debug(
            f"RRF fusion: combined {len(ranked_lists)} lists "
            f"into {len(sorted_results)} results"
        )

        return sorted_results

    def fuse_hybrid(
        self,
        fts_results: list[tuple[int, float]],
        vector_results: list[tuple[int, float]],
        fts_weight: float = 0.4,
        vector_weight: float = 0.6,
    ) -> list[tuple[int, float]]:
        """Fuse full-text and vector search results using weighted RRF.

        Convenience method for the common case of hybrid search combining
        full-text and vector similarity results.

        Args:
            fts_results: Full-text search results as (entity_id, score) tuples.
            vector_results: Vector search results as (entity_id, score) tuples.
            fts_weight: Weight for full-text scores (default 0.4).
            vector_weight: Weight for vector scores (default 0.6).

        Returns:
            Combined results as (entity_id, rrf_score) tuples, sorted by score.
        """
        return self.fuse_with_weights([
            (fts_results, fts_weight),
            (vector_results, vector_weight),
        ])

    def fuse_with_weights(
        self,
        ranked_lists: list[tuple[list[tuple[int, float]], float]],
    ) -> list[tuple[int, float]]:
        """Fuse ranked lists with per-list weights.

        This allows giving different importance to different retrieval
        methods (e.g., weighting semantic search higher than full-text).

        Args:
            ranked_lists: List of (ranked_list, weight) tuples.
                         Higher weights give more importance to that list.

        Returns:
            Fused list of (entity_id, weighted_rrf_score) tuples,
            sorted by weighted RRF score descending.
        """
        rrf_scores: dict[int, float] = {}

        for ranked_list, weight in ranked_lists:
            for rank, (entity_id, _score) in enumerate(ranked_list, start=1):
                if entity_id not in rrf_scores:
                    rrf_scores[entity_id] = 0.0
                rrf_scores[entity_id] += weight * (1.0 / (self.k + rank))

        sorted_results = sorted(
            rrf_scores.items(),
            key=lambda x: x[1],
            reverse=True,
        )

        return sorted_results


class SearchRanker:
    """Multi-signal ranking for search results.

    Combines multiple relevance signals with configurable weights
    to produce final ranking scores.

    Default weights (from research.md):
        - rrf_score: 0.40 (hybrid search relevance)
        - recency: 0.20 (newer content preferred)
        - participant: 0.15 (frequently contacted people)
        - content_type: 0.10 (user's preferred content types)
        - confidence: 0.10 (AI extraction confidence)
        - popularity: 0.05 (frequently returned in searches)

    The recency score uses exponential decay with a configurable half-life.
    By default, the score halves every 30 days.

    Example:
        >>> ranker = SearchRanker()
        >>> ranked = ranker.rank_results(
        ...     results=search_results,
        ...     rrf_scores={1: 0.5, 2: 0.3},
        ...     frequent_contacts=["alice@example.com"],
        ...     preferred_types=["email", "meeting"],
        ... )
    """

    # Default weights from research.md
    DEFAULT_WEIGHTS: dict[str, float] = {
        "rrf_score": 0.40,
        "recency": 0.20,
        "participant": 0.15,
        "content_type": 0.10,
        "confidence": 0.10,
        "popularity": 0.05,
    }

    # Recency decay: score halves every 30 days
    RECENCY_HALF_LIFE_DAYS: int = 30

    def __init__(
        self,
        weights: dict[str, float] | None = None,
        recency_half_life_days: int = 30,
        # Legacy parameters for backwards compatibility
        personalization_weight: float | None = None,
        recency_weight: float | None = None,
    ) -> None:
        """Initialize search ranker.

        Args:
            weights: Custom signal weights. Uses defaults if not provided.
                    Weights should sum to approximately 1.0 for intuitive scores.
            recency_half_life_days: Days for recency score to halve.
                                   Default is 30 days.
            personalization_weight: Deprecated. Use weights dict instead.
            recency_weight: Deprecated. Use weights dict instead.
        """
        self.weights = weights or self.DEFAULT_WEIGHTS.copy()
        self.recency_half_life_days = recency_half_life_days

        # Legacy compatibility
        self.personalization_weight = personalization_weight or 0.1
        self.recency_weight = recency_weight or 0.1

    def rank_results(
        self,
        results: list[SearchResult],
        rrf_scores: dict[int, float],
        frequent_contacts: list[str] | None = None,
        preferred_types: list[ContentTypeFilter] | None = None,
        reference_date: datetime | None = None,
    ) -> list[RankedResult]:
        """Rank results with personalization and boosting.

        Args:
            results: SearchResult objects to rank
            rrf_scores: RRF scores by entity_id
            frequent_contacts: Email addresses of frequent contacts for boosting
            preferred_types: Preferred content types for boosting
            reference_date: Reference date for recency calculation (default: now)

        Returns:
            List of RankedResult objects sorted by final score
        """
        if reference_date is None:
            reference_date = datetime.utcnow()

        frequent_contacts_set = {c.lower() for c in (frequent_contacts or [])}
        preferred_types_set = set(preferred_types or [])

        ranked_results: list[RankedResult] = []

        for result in results:
            rrf_score = rrf_scores.get(result.entity_id, 0.0)

            # Calculate personalization boost
            personalization_boost = self._calculate_personalization_boost(
                result, frequent_contacts_set, preferred_types_set
            )

            # Calculate recency boost
            recency_boost = self._calculate_recency_boost(
                result.timestamp, reference_date
            )

            # Combine scores
            final_score = (
                rrf_score
                + (personalization_boost * self.personalization_weight)
                + (recency_boost * self.recency_weight)
            )

            ranked_results.append(
                RankedResult(
                    result=result,
                    final_score=final_score,
                    rrf_score=rrf_score,
                    personalization_boost=personalization_boost,
                    recency_boost=recency_boost,
                )
            )

        # Sort by final score
        ranked_results.sort(key=lambda r: r.final_score, reverse=True)

        # Assign rank positions
        for i, ranked in enumerate(ranked_results, start=1):
            ranked.rank_position = i

        logger.debug(f"Ranked {len(ranked_results)} results")

        return ranked_results

    def _calculate_personalization_boost(
        self,
        result: SearchResult,
        frequent_contacts: set[str],
        preferred_types: set[ContentTypeFilter],
    ) -> float:
        """Calculate personalization boost for a result.

        Args:
            result: SearchResult to evaluate
            frequent_contacts: Set of frequent contact emails (lowercase)
            preferred_types: Set of preferred content types

        Returns:
            Personalization boost score (0-1)
        """
        boost = 0.0

        # Boost for frequent contacts
        if frequent_contacts:
            participant_emails = {p.lower() for p in result.participants}
            matching_contacts = participant_emails & frequent_contacts
            if matching_contacts:
                # Boost proportional to number of matching contacts (max 0.5)
                boost += min(0.5, len(matching_contacts) * 0.25)

        # Boost for preferred content types
        if preferred_types and result.content_type in preferred_types:
            boost += 0.3

        return min(1.0, boost)

    def _calculate_recency_boost(
        self,
        timestamp: datetime,
        reference_date: datetime,
    ) -> float:
        """Calculate recency boost based on content age.

        Uses exponential decay with configurable half-life.

        Args:
            timestamp: Content timestamp
            reference_date: Reference date (typically now)

        Returns:
            Recency boost score (0-1)
        """
        if timestamp is None:
            return 0.0

        # Make both datetimes naive for comparison if needed
        if timestamp.tzinfo is not None and reference_date.tzinfo is None:
            timestamp = timestamp.replace(tzinfo=None)
        elif timestamp.tzinfo is None and reference_date.tzinfo is not None:
            reference_date = reference_date.replace(tzinfo=None)

        age_days = (reference_date - timestamp).total_seconds() / 86400.0

        if age_days < 0:
            # Future timestamp, give full boost
            return 1.0

        # Exponential decay
        decay_rate = 0.693 / self.recency_half_life_days  # ln(2) / half_life
        boost = pow(2.718, -decay_rate * age_days)

        return max(0.0, min(1.0, boost))

    def filter_by_confidence(
        self,
        results: list[RankedResult],
        min_confidence: float,
    ) -> list[RankedResult]:
        """Filter results by minimum confidence score.

        Args:
            results: RankedResult list to filter
            min_confidence: Minimum confidence threshold (0-1)

        Returns:
            Filtered list of RankedResult objects
        """
        filtered = [
            r for r in results
            if r.result.confidence_score is None
            or r.result.confidence_score >= min_confidence
        ]

        if len(filtered) < len(results):
            logger.debug(
                f"Confidence filter removed {len(results) - len(filtered)} results"
            )

        return filtered

    def diversify_results(
        self,
        results: list[RankedResult],
        diversity_factor: float = 0.3,
    ) -> list[RankedResult]:
        """Diversify results to avoid clustering from single sources.

        Reorders results to spread out content types and sources while
        maintaining overall relevance.

        Args:
            results: RankedResult list to diversify
            diversity_factor: How much to prioritize diversity (0-1)

        Returns:
            Diversified list of RankedResult objects
        """
        if diversity_factor <= 0 or len(results) <= 2:
            return results

        diversified: list[RankedResult] = []
        remaining = list(results)
        seen_types: dict[ContentTypeFilter, int] = {}

        while remaining:
            best_idx = 0
            best_score = float("-inf")

            for i, result in enumerate(remaining):
                content_type = result.result.content_type
                type_count = seen_types.get(content_type, 0)

                # Penalize repeated content types
                diversity_penalty = diversity_factor * (type_count * 0.2)
                adjusted_score = result.final_score - diversity_penalty

                if adjusted_score > best_score:
                    best_score = adjusted_score
                    best_idx = i

            selected = remaining.pop(best_idx)
            diversified.append(selected)
            seen_types[selected.result.content_type] = (
                seen_types.get(selected.result.content_type, 0) + 1
            )

        # Reassign rank positions
        for i, ranked in enumerate(diversified, start=1):
            ranked.rank_position = i

        return diversified

    def recency_score(self, timestamp: datetime) -> float:
        """Calculate recency score with exponential decay.

        Score = 0.5 ^ (days_ago / half_life)

        This gives:
        - Today: 1.0
        - 30 days ago: 0.5 (with default half-life)
        - 60 days ago: 0.25
        - 90 days ago: 0.125

        Args:
            timestamp: Content timestamp.

        Returns:
            Recency score between 0 and 1.
        """
        now = datetime.now(timezone.utc)

        # Ensure timestamp is timezone-aware
        if timestamp.tzinfo is None:
            timestamp = timestamp.replace(tzinfo=timezone.utc)

        days_ago = (now - timestamp).days
        if days_ago < 0:
            # Future dates get full recency score
            days_ago = 0

        decay_factor = days_ago / self.recency_half_life_days
        return math.pow(0.5, decay_factor)

    def participant_score(
        self,
        result_participants: list[str],
        frequent_contacts: list[str],
    ) -> float:
        """Calculate participant relevance score.

        Higher score if result involves frequently contacted people.
        Score is based on overlap between result participants and
        the user's frequent contacts.

        Args:
            result_participants: Participants in the search result.
            frequent_contacts: User's frequently contacted people.

        Returns:
            Score between 0 and 1.
        """
        if not result_participants or not frequent_contacts:
            return 0.0

        frequent_set = {p.lower() for p in frequent_contacts}
        result_set = {p.lower() for p in result_participants}

        overlap = len(frequent_set & result_set)
        if overlap == 0:
            return 0.0

        # Normalize by the result set size
        return min(overlap / len(result_set), 1.0)

    def content_type_score(
        self,
        result_content_type: str,
        preferred_types: list[str],
    ) -> float:
        """Calculate content type preference score.

        Higher score for content types that appear earlier in the
        user's preference list.

        Args:
            result_content_type: Content type of the result.
            preferred_types: User's preferred content types (ordered by preference).

        Returns:
            Score between 0 and 1.
        """
        if not preferred_types:
            return 0.5  # Neutral if no preference

        result_type = result_content_type.lower()
        for i, pref_type in enumerate(preferred_types):
            if pref_type.lower() == result_type:
                # Higher score for earlier preferences
                # First preference: 1.0, second: 0.8, third: 0.6, etc.
                return max(1.0 - (i * 0.2), 0.2)

        return 0.2  # Low score if not in preferences

    def rank_with_signals(
        self,
        results: list[SearchResult],
        rrf_scores: dict[int, float],
        frequent_contacts: list[str] | None = None,
        preferred_types: list[str] | None = None,
        popularity_scores: dict[int, float] | None = None,
    ) -> list[RankedResult]:
        """Rank search results using multi-signal scoring.

        Combines all ranking signals with configured weights to produce
        a final score for each result. This is the primary ranking method
        that implements the research.md specification.

        Args:
            results: List of search results to rank.
            rrf_scores: RRF scores from hybrid search (entity_id -> score).
            frequent_contacts: User's frequently contacted people.
            preferred_types: User's preferred content types.
            popularity_scores: Historical popularity scores (entity_id -> score).

        Returns:
            List of RankedResult sorted by final score descending.
        """
        frequent_contacts = frequent_contacts or []
        preferred_types = preferred_types or []
        popularity_scores = popularity_scores or {}

        ranked: list[RankedResult] = []

        # Normalize RRF scores to 0-1 range
        max_rrf = max(rrf_scores.values()) if rrf_scores else 1.0

        for result in results:
            breakdown: dict[str, float] = {}

            # RRF score (normalized)
            raw_rrf = rrf_scores.get(result.entity_id, 0.0)
            breakdown["rrf_score"] = raw_rrf / max_rrf if max_rrf > 0 else 0.0

            # Recency score
            breakdown["recency"] = self.recency_score(result.timestamp)

            # Participant score
            breakdown["participant"] = self.participant_score(
                result.participants,
                frequent_contacts,
            )

            # Content type score
            breakdown["content_type"] = self.content_type_score(
                result.content_type.value,
                preferred_types,
            )

            # Confidence score (use result's confidence or neutral 0.5)
            breakdown["confidence"] = result.confidence_score or 0.5

            # Popularity score
            breakdown["popularity"] = popularity_scores.get(result.entity_id, 0.0)

            # Calculate final weighted score
            final_score = sum(
                self.weights.get(signal, 0.0) * score
                for signal, score in breakdown.items()
            )

            ranked.append(
                RankedResult(
                    result=result,
                    final_score=final_score,
                    score_breakdown=breakdown,
                    rrf_score=raw_rrf,
                    recency_boost=breakdown["recency"],
                    personalization_boost=breakdown["participant"],
                )
            )

        # Sort by final score descending
        ranked.sort(key=lambda r: r.final_score, reverse=True)

        # Assign rank positions
        for i, r in enumerate(ranked, start=1):
            r.rank_position = i

        logger.debug(f"Ranked {len(ranked)} results with multi-signal scoring")

        return ranked
