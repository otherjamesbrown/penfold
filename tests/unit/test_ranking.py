"""Unit tests for search ranking module.

Tests ranking and scoring functionality for the search interface including:
- RRF (Reciprocal Rank Fusion) algorithm
- Multi-signal relevance scoring
- Result sorting and filtering logic

Based on research.md specification for hybrid search ranking.
"""

from __future__ import annotations

from datetime import datetime, timedelta, timezone

import pytest

from penf_lib.search.models import ContentPreview, ContentTypeFilter, SearchResult
from penf_lib.search.ranking import RankedResult, RRFFusion, SearchRanker


def make_search_result(
    entity_id: int,
    content_type: ContentTypeFilter = ContentTypeFilter.EMAIL,
    participants: list[str] | None = None,
    timestamp: datetime | None = None,
    confidence_score: float | None = None,
) -> SearchResult:
    """Helper to create SearchResult for testing."""
    return SearchResult(
        result_id=f"result-{entity_id}",
        entity_type="source",
        entity_id=entity_id,
        content_type=content_type,
        preview=ContentPreview(title=f"Test {entity_id}", snippet="Test content"),
        source_attribution=f"test://{entity_id}",
        relevance_score=0.5,
        rrf_score=0.1,
        timestamp=timestamp or datetime.utcnow(),
        participants=participants or [],
        confidence_score=confidence_score,
    )


@pytest.mark.unit
class TestRRFFusion:
    """Test RRF fusion algorithm."""

    def test_rrf_fusion_combines_results(self):
        """Test RRF combines multiple ranked lists correctly."""
        fusion = RRFFusion(k=60)

        fts_results = [(1, 0.9), (2, 0.8), (3, 0.7)]
        vector_results = [(2, 0.95), (1, 0.85), (4, 0.75)]

        combined = fusion.fuse(fts_results, vector_results)

        # Should return all unique entity IDs
        entity_ids = [eid for eid, _ in combined]
        assert set(entity_ids) == {1, 2, 3, 4}

        # Entity 2 appears in both (rank 1 in vector, rank 2 in FTS)
        # Entity 1 appears in both (rank 1 in FTS, rank 2 in vector)
        # Both should have higher combined scores than entities in only one list
        scores = dict(combined)
        assert scores[2] > scores[3]  # 2 in both lists, 3 only in FTS
        assert scores[1] > scores[4]  # 1 in both lists, 4 only in vector

    def test_rrf_with_empty_rankings(self):
        """Test RRF handles empty rankings gracefully."""
        fusion = RRFFusion(k=60)

        fts_results = [(1, 0.9), (2, 0.8)]
        vector_results: list[tuple[int, float]] = []

        combined = fusion.fuse(fts_results, vector_results)

        # Should still return FTS results
        entity_ids = [eid for eid, _ in combined]
        assert set(entity_ids) == {1, 2}

    def test_rrf_with_single_ranking(self):
        """Test RRF with only one ranking list."""
        fusion = RRFFusion(k=60)

        fts_results = [(1, 0.9), (2, 0.8), (3, 0.7)]
        vector_results: list[tuple[int, float]] = []

        combined = fusion.fuse(fts_results, vector_results)

        # Order should be preserved
        entity_ids = [eid for eid, _ in combined]
        assert entity_ids == [1, 2, 3]

    def test_rrf_k_parameter_effect(self):
        """Test that k parameter affects score distribution."""
        fusion_low_k = RRFFusion(k=10)
        fusion_high_k = RRFFusion(k=100)

        fts_results = [(1, 0.9), (2, 0.8)]
        vector_results = [(1, 0.9), (2, 0.8)]

        combined_low = dict(fusion_low_k.fuse(fts_results, vector_results))
        combined_high = dict(fusion_high_k.fuse(fts_results, vector_results))

        # With lower k, scores are higher overall
        assert combined_low[1] > combined_high[1]

    def test_rrf_custom_weights(self):
        """Test RRF with custom weights using fuse_hybrid method."""
        fusion = RRFFusion(k=60)

        fts_results = [(1, 0.9)]
        vector_results = [(2, 0.9)]

        # Equal weights
        combined_equal = dict(fusion.fuse_hybrid(fts_results, vector_results, 0.5, 0.5))
        assert abs(combined_equal[1] - combined_equal[2]) < 0.001

        # Heavy FTS weight
        combined_fts = dict(fusion.fuse_hybrid(fts_results, vector_results, 0.9, 0.1))
        assert combined_fts[1] > combined_fts[2]

        # Heavy vector weight
        combined_vec = dict(fusion.fuse_hybrid(fts_results, vector_results, 0.1, 0.9))
        assert combined_vec[2] > combined_vec[1]


@pytest.mark.unit
class TestSearchRanker:
    """Test SearchRanker functionality."""

    def test_rank_results_basic(self):
        """Test basic result ranking by RRF score."""
        ranker = SearchRanker()

        results = [
            make_search_result(1),
            make_search_result(2),
            make_search_result(3),
        ]
        rrf_scores = {1: 0.3, 2: 0.5, 3: 0.1}

        ranked = ranker.rank_results(results, rrf_scores)

        # Should be sorted by final_score (derived from RRF)
        assert ranked[0].result.entity_id == 2  # highest RRF
        assert ranked[1].result.entity_id == 1
        assert ranked[2].result.entity_id == 3  # lowest RRF

    def test_rank_assigns_positions(self):
        """Test that rank positions are assigned correctly."""
        ranker = SearchRanker()

        results = [make_search_result(i) for i in range(5)]
        rrf_scores = {0: 0.1, 1: 0.5, 2: 0.3, 3: 0.4, 4: 0.2}

        ranked = ranker.rank_results(results, rrf_scores)

        # Check positions are 1-indexed and sequential
        positions = [r.rank_position for r in ranked]
        assert positions == [1, 2, 3, 4, 5]

    def test_personalization_boost_frequent_contacts(self):
        """Test personalization boost for frequent contacts."""
        ranker = SearchRanker(personalization_weight=0.5)

        results = [
            make_search_result(1, participants=["alice@example.com"]),
            make_search_result(2, participants=["bob@example.com"]),
        ]
        rrf_scores = {1: 0.5, 2: 0.5}  # Equal base scores

        ranked = ranker.rank_results(
            results,
            rrf_scores,
            frequent_contacts=["alice@example.com"],
        )

        # Result with frequent contact should be boosted
        assert ranked[0].result.entity_id == 1
        assert ranked[0].personalization_boost > 0

    def test_personalization_boost_preferred_types(self):
        """Test personalization boost for preferred content types."""
        ranker = SearchRanker(personalization_weight=0.5)

        results = [
            make_search_result(1, content_type=ContentTypeFilter.EMAIL),
            make_search_result(2, content_type=ContentTypeFilter.DOCUMENT),
        ]
        rrf_scores = {1: 0.5, 2: 0.5}

        ranked = ranker.rank_results(
            results,
            rrf_scores,
            preferred_types=[ContentTypeFilter.EMAIL],
        )

        # Email should be boosted
        assert ranked[0].result.entity_id == 1

    def test_recency_boost(self):
        """Test recency boost for recent content."""
        ranker = SearchRanker(recency_weight=0.5, recency_half_life_days=30)

        now = datetime.utcnow()
        results = [
            make_search_result(1, timestamp=now - timedelta(days=60)),  # Old
            make_search_result(2, timestamp=now - timedelta(days=1)),  # Recent
        ]
        rrf_scores = {1: 0.5, 2: 0.5}

        ranked = ranker.rank_results(results, rrf_scores, reference_date=now)

        # Recent content should be boosted
        assert ranked[0].result.entity_id == 2
        assert ranked[0].recency_boost > ranked[1].recency_boost

    def test_filter_by_confidence(self):
        """Test filtering results by confidence score."""
        ranker = SearchRanker()

        results = [
            make_search_result(1, confidence_score=0.9),
            make_search_result(2, confidence_score=0.3),
            make_search_result(3, confidence_score=None),  # No score
        ]
        rrf_scores = {1: 0.5, 2: 0.5, 3: 0.5}

        ranked = ranker.rank_results(results, rrf_scores)
        filtered = ranker.filter_by_confidence(ranked, min_confidence=0.5)

        # Should keep high confidence and None (unknown)
        entity_ids = [r.result.entity_id for r in filtered]
        assert 1 in entity_ids  # 0.9 >= 0.5
        assert 2 not in entity_ids  # 0.3 < 0.5
        assert 3 in entity_ids  # None passes filter

    def test_diversify_results(self):
        """Test result diversification."""
        ranker = SearchRanker()

        results = [
            make_search_result(1, content_type=ContentTypeFilter.EMAIL),
            make_search_result(2, content_type=ContentTypeFilter.EMAIL),
            make_search_result(3, content_type=ContentTypeFilter.MEETING),
            make_search_result(4, content_type=ContentTypeFilter.EMAIL),
        ]
        rrf_scores = {1: 0.9, 2: 0.85, 3: 0.8, 4: 0.75}

        ranked = ranker.rank_results(results, rrf_scores)
        diversified = ranker.diversify_results(ranked, diversity_factor=0.5)

        # Meeting should be promoted to avoid 3 emails in a row
        entity_ids = [r.result.entity_id for r in diversified[:3]]
        assert 3 in entity_ids  # Meeting promoted


@pytest.mark.unit
class TestRankedResult:
    """Test RankedResult dataclass."""

    def test_ranked_result_creation(self):
        """Test RankedResult initialization."""
        result = make_search_result(1)
        ranked = RankedResult(
            result=result,
            final_score=0.8,
            rrf_score=0.6,
            personalization_boost=0.1,
            recency_boost=0.1,
            rank_position=1,
        )

        assert ranked.result == result
        assert ranked.final_score == 0.8
        assert ranked.rank_position == 1

    def test_ranked_result_score_breakdown(self):
        """Test score breakdown is initialized."""
        result = make_search_result(1)
        ranked = RankedResult(result=result, final_score=0.5)

        # score_breakdown should be initialized to empty dict
        assert ranked.score_breakdown == {}


@pytest.mark.unit
class TestRRFFusionValidation:
    """Test RRF validation and edge cases."""

    def test_rrf_k_validation(self):
        """Test RRF raises error for invalid k value."""
        with pytest.raises(ValueError, match="k must be at least 1"):
            RRFFusion(k=0)

        with pytest.raises(ValueError, match="k must be at least 1"):
            RRFFusion(k=-5)

    def test_rrf_formula_correctness(self):
        """Test RRF formula: RRF(d) = sum(1 / (k + rank_i(d)))."""
        rrf = RRFFusion(k=60)
        list1 = [(1, 0.9), (2, 0.8)]

        result = rrf.fuse(list1)
        scores = dict(result)

        # Entity 1 at rank 1: 1/(60+1) = 0.01639...
        assert scores[1] == pytest.approx(1 / 61, rel=1e-6)
        # Entity 2 at rank 2: 1/(60+2) = 0.01613...
        assert scores[2] == pytest.approx(1 / 62, rel=1e-6)

    def test_rrf_fusion_multiple_lists(self):
        """Test fuse method with variadic arguments."""
        rrf = RRFFusion(k=60)
        list1 = [(1, 0.9)]
        list2 = [(2, 0.9)]
        list3 = [(3, 0.9)]

        result = rrf.fuse(list1, list2, list3)
        entity_ids = [e[0] for e in result]

        assert set(entity_ids) == {1, 2, 3}
        # All at rank 1 in their respective lists, so equal scores
        scores = dict(result)
        assert scores[1] == scores[2] == scores[3]


@pytest.mark.unit
class TestSearchRankerMultiSignal:
    """Test multi-signal ranking with configurable weights."""

    def test_default_weights_from_research(self):
        """Test default weights match research.md specification."""
        ranker = SearchRanker()

        assert ranker.weights["rrf_score"] == 0.40
        assert ranker.weights["recency"] == 0.20
        assert ranker.weights["participant"] == 0.15
        assert ranker.weights["content_type"] == 0.10
        assert ranker.weights["confidence"] == 0.10
        assert ranker.weights["popularity"] == 0.05

    def test_custom_weights(self):
        """Test ranking with custom weights."""
        custom_weights = {
            "rrf_score": 0.50,
            "recency": 0.30,
            "participant": 0.10,
            "content_type": 0.05,
            "confidence": 0.05,
            "popularity": 0.00,
        }
        ranker = SearchRanker(weights=custom_weights)

        assert ranker.weights == custom_weights

    def test_rank_with_signals_method(self):
        """Test rank_with_signals produces correct score breakdown."""
        ranker = SearchRanker()

        now = datetime.now(timezone.utc)
        results = [
            make_search_result(
                1,
                timestamp=now,
                participants=["alice@example.com"],
                confidence_score=0.9,
            ),
        ]
        rrf_scores = {1: 0.5}

        ranked = ranker.rank_with_signals(
            results,
            rrf_scores,
            frequent_contacts=["alice@example.com"],
            preferred_types=["email"],
        )

        assert len(ranked) == 1
        breakdown = ranked[0].score_breakdown

        # All signals should be present
        assert "rrf_score" in breakdown
        assert "recency" in breakdown
        assert "participant" in breakdown
        assert "content_type" in breakdown
        assert "confidence" in breakdown
        assert "popularity" in breakdown

    def test_rank_with_signals_popularity(self):
        """Test popularity scores affect ranking."""
        ranker = SearchRanker()

        now = datetime.now(timezone.utc)
        results = [
            make_search_result(1, timestamp=now),
            make_search_result(2, timestamp=now),
        ]
        rrf_scores = {1: 0.5, 2: 0.5}  # Same RRF
        popularity_scores = {1: 0.0, 2: 1.0}  # Entity 2 is popular

        ranked = ranker.rank_with_signals(
            results,
            rrf_scores,
            popularity_scores=popularity_scores,
        )

        # Entity 2 should rank higher due to popularity
        assert ranked[0].result.entity_id == 2
        assert ranked[0].score_breakdown["popularity"] == 1.0
        assert ranked[1].score_breakdown["popularity"] == 0.0


@pytest.mark.unit
class TestSearchRankerRecencyScore:
    """Test recency score calculation."""

    def test_recency_score_today(self):
        """Test that today's content gets maximum recency score."""
        ranker = SearchRanker()
        now = datetime.now(timezone.utc)

        score = ranker.recency_score(now)
        assert score == pytest.approx(1.0, rel=1e-2)

    def test_recency_score_30_days_ago(self):
        """Test recency score halves at half-life (30 days)."""
        ranker = SearchRanker(recency_half_life_days=30)
        past = datetime.now(timezone.utc) - timedelta(days=30)

        score = ranker.recency_score(past)
        assert score == pytest.approx(0.5, rel=1e-2)

    def test_recency_score_60_days_ago(self):
        """Test recency score is 0.25 at two half-lives."""
        ranker = SearchRanker(recency_half_life_days=30)
        past = datetime.now(timezone.utc) - timedelta(days=60)

        score = ranker.recency_score(past)
        assert score == pytest.approx(0.25, rel=1e-2)

    def test_recency_score_future_date(self):
        """Test future dates get full recency score."""
        ranker = SearchRanker()
        future = datetime.now(timezone.utc) + timedelta(days=7)

        score = ranker.recency_score(future)
        assert score == pytest.approx(1.0, rel=1e-2)

    def test_recency_score_naive_datetime(self):
        """Test recency score handles naive datetimes."""
        ranker = SearchRanker()
        naive_now = datetime.now()  # No timezone

        score = ranker.recency_score(naive_now)
        assert 0.0 <= score <= 1.0


@pytest.mark.unit
class TestSearchRankerParticipantScore:
    """Test participant relevance score calculation."""

    def test_participant_score_full_overlap(self):
        """Test score when all participants are frequent contacts."""
        ranker = SearchRanker()

        score = ranker.participant_score(
            ["alice@example.com", "bob@example.com"],
            ["alice@example.com", "bob@example.com"],
        )
        assert score == 1.0

    def test_participant_score_partial_overlap(self):
        """Test score with partial participant overlap."""
        ranker = SearchRanker()

        score = ranker.participant_score(
            ["alice@example.com", "bob@example.com"],
            ["alice@example.com", "charlie@example.com"],
        )
        assert score == 0.5  # 1 out of 2 match

    def test_participant_score_no_overlap(self):
        """Test score when no participants are frequent contacts."""
        ranker = SearchRanker()

        score = ranker.participant_score(
            ["alice@example.com"],
            ["bob@example.com"],
        )
        assert score == 0.0

    def test_participant_score_empty_participants(self):
        """Test score with empty result participants."""
        ranker = SearchRanker()

        score = ranker.participant_score([], ["alice@example.com"])
        assert score == 0.0

    def test_participant_score_case_insensitive(self):
        """Test participant matching is case-insensitive."""
        ranker = SearchRanker()

        score = ranker.participant_score(
            ["Alice@Example.COM"],
            ["alice@example.com"],
        )
        assert score == 1.0


@pytest.mark.unit
class TestSearchRankerContentTypeScore:
    """Test content type preference score calculation."""

    def test_content_type_score_first_preference(self):
        """Test first preference gets maximum score."""
        ranker = SearchRanker()

        score = ranker.content_type_score("email", ["email", "meeting", "document"])
        assert score == 1.0

    def test_content_type_score_second_preference(self):
        """Test second preference gets reduced score."""
        ranker = SearchRanker()

        score = ranker.content_type_score("meeting", ["email", "meeting", "document"])
        assert score == 0.8

    def test_content_type_score_not_in_preferences(self):
        """Test score for type not in preferences."""
        ranker = SearchRanker()

        score = ranker.content_type_score("slack", ["email", "meeting"])
        assert score == 0.2  # Minimum score

    def test_content_type_score_no_preferences(self):
        """Test neutral score when no preferences defined."""
        ranker = SearchRanker()

        score = ranker.content_type_score("email", [])
        assert score == 0.5

    def test_content_type_score_case_insensitive(self):
        """Test content type matching is case-insensitive."""
        ranker = SearchRanker()

        score = ranker.content_type_score("EMAIL", ["email", "meeting"])
        assert score == 1.0
