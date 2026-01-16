"""Integration tests for correlation discovery.

Tests the cross-content correlation discovery functionality including:
- Finding related content by participants
- Finding related content by project references
- Finding related content by temporal proximity
- Finding related content by semantic similarity
- Finding related content by thread chains
- Combined correlation scoring with weights
"""

from datetime import datetime, timezone
from unittest.mock import AsyncMock, MagicMock, patch
from uuid import uuid4

import pytest

from penf_lib.search.correlations import (
    ContentDetails,
    CorrelationDiscovery,
    CorrelationResult,
    RelatedItem,
    find_related_by_person,
    find_related_by_project,
)
from penf_lib.search.models import ContentTypeFilter


class TestCorrelationDiscovery:
    """Test correlation discovery functionality."""

    @pytest.fixture
    def mock_session(self):
        """Create a mock database session."""
        session = AsyncMock()
        return session

    @pytest.fixture
    def tenant_id(self):
        """Create a test tenant ID."""
        return str(uuid4())

    @pytest.fixture
    def discovery(self, mock_session, tenant_id):
        """Create a CorrelationDiscovery instance."""
        return CorrelationDiscovery(
            session=mock_session,
            tenant_id=tenant_id,
        )

    def test_default_signal_weights(self, discovery):
        """Test that default signal weights are set correctly."""
        assert discovery.weights == {
            "participants": 0.30,
            "project": 0.25,
            "temporal": 0.20,
            "semantic": 0.15,
            "thread": 0.10,
        }
        assert sum(discovery.weights.values()) == pytest.approx(1.0)

    def test_custom_signal_weights(self, mock_session, tenant_id):
        """Test that custom weights can be provided."""
        custom_weights = {
            "participants": 0.40,
            "project": 0.30,
            "temporal": 0.15,
            "semantic": 0.10,
            "thread": 0.05,
        }
        discovery = CorrelationDiscovery(
            session=mock_session,
            tenant_id=tenant_id,
            weights=custom_weights,
        )
        assert discovery.weights == custom_weights

    def test_temporal_window_days(self, discovery):
        """Test temporal window constant."""
        assert discovery.TEMPORAL_WINDOW_DAYS == 7

    def test_min_score_threshold(self, discovery):
        """Test minimum score threshold."""
        assert discovery.MIN_SCORE_THRESHOLD == 0.05

    @pytest.mark.asyncio
    async def test_find_related_content_empty_source(self, discovery, mock_session):
        """Test correlation discovery when source content is not found."""
        # Mock _get_content to return None
        with patch.object(discovery, '_get_content', return_value=None):
            result = await discovery.find_related_content(
                content_id=999,
                content_type="source",
                limit=10,
            )

        assert isinstance(result, CorrelationResult)
        assert result.source_id == 999
        assert result.source_type == "source"
        assert result.related_items == []
        assert result.total_correlations == 0

    @pytest.mark.asyncio
    async def test_find_related_content_with_matches(self, discovery):
        """Test correlation discovery with various signal matches."""
        # Create mock source content
        source_content = ContentDetails(
            entity_id=1,
            entity_type="source",
            content_type="email",
            participants=["alice@example.com", "bob@example.com"],
            project_refs=["project-atlas"],
            timestamp=datetime.now(timezone.utc),
            embedding=[0.1] * 768,
            thread_id="thread-123",
            title="Test Email",
        )

        # Mock all internal methods
        with patch.object(discovery, '_get_content', return_value=source_content):
            with patch.object(discovery, '_find_by_participants', return_value=[
                (2, "source", "email", "Related by participant", datetime.now(timezone.utc), 0.8),
            ]):
                with patch.object(discovery, '_find_by_project', return_value=[
                    (3, "source", "meeting", "Related by project", datetime.now(timezone.utc), 0.7),
                ]):
                    with patch.object(discovery, '_find_by_temporal_proximity', return_value=[
                        (2, "source", "email", "Related by time", datetime.now(timezone.utc), 0.5),
                    ]):
                        with patch.object(discovery, '_find_by_similarity', return_value=[
                            (4, "source", "document", "Related by similarity", datetime.now(timezone.utc), 0.6),
                        ]):
                            with patch.object(discovery, '_find_by_thread', return_value=[
                                (5, "source", "email", "Related by thread", datetime.now(timezone.utc), 1.0),
                            ]):
                                result = await discovery.find_related_content(
                                    content_id=1,
                                    content_type="source",
                                    limit=10,
                                )

        assert isinstance(result, CorrelationResult)
        assert result.source_id == 1
        assert len(result.related_items) > 0
        # Check that items are sorted by score
        scores = [item.correlation_score for item in result.related_items]
        assert scores == sorted(scores, reverse=True)

    def test_merge_and_score_single_signal(self, discovery):
        """Test score merging with a single signal."""
        participant_matches = [
            (2, "source", "email", "Test Email", datetime.now(timezone.utc), 0.8),
        ]

        results = discovery._merge_and_score(
            participant_matches=participant_matches,
            project_matches=[],
            temporal_matches=[],
            semantic_matches=[],
            thread_matches=[],
        )

        assert len(results) == 1
        assert results[0].entity_id == 2
        assert results[0].correlation_type == "participants"
        # Score should be weighted: 0.8 * 0.30 = 0.24
        assert results[0].correlation_score == pytest.approx(0.24)
        assert results[0].score_breakdown == {"participants": 0.8}

    def test_merge_and_score_multiple_signals(self, discovery):
        """Test score merging with multiple signals for same entity."""
        now = datetime.now(timezone.utc)
        # Same entity (2) matched by multiple signals
        participant_matches = [(2, "source", "email", "Test", now, 0.8)]
        temporal_matches = [(2, "source", "email", "Test", now, 0.5)]
        semantic_matches = [(2, "source", "email", "Test", now, 0.6)]

        results = discovery._merge_and_score(
            participant_matches=participant_matches,
            project_matches=[],
            temporal_matches=temporal_matches,
            semantic_matches=semantic_matches,
            thread_matches=[],
        )

        assert len(results) == 1
        assert results[0].entity_id == 2
        # Combined score: 0.8*0.30 + 0.5*0.20 + 0.6*0.15 = 0.24 + 0.10 + 0.09 = 0.43
        expected_score = 0.8 * 0.30 + 0.5 * 0.20 + 0.6 * 0.15
        assert results[0].correlation_score == pytest.approx(expected_score)
        assert results[0].score_breakdown == {
            "participants": 0.8,
            "temporal": 0.5,
            "semantic": 0.6,
        }
        # Primary type should be participants (highest score)
        assert results[0].correlation_type == "participants"

    def test_merge_and_score_multiple_entities(self, discovery):
        """Test score merging with multiple different entities."""
        now = datetime.now(timezone.utc)
        participant_matches = [
            (2, "source", "email", "Email 1", now, 0.8),
            (3, "source", "meeting", "Meeting 1", now, 0.6),
        ]
        project_matches = [
            (4, "source", "document", "Doc 1", now, 0.9),
        ]

        results = discovery._merge_and_score(
            participant_matches=participant_matches,
            project_matches=project_matches,
            temporal_matches=[],
            semantic_matches=[],
            thread_matches=[],
        )

        assert len(results) == 3
        entity_ids = {r.entity_id for r in results}
        assert entity_ids == {2, 3, 4}

    def test_extract_participants(self, discovery):
        """Test participant extraction from metadata."""
        metadata = {
            "from": "alice@example.com",
            "to": ["bob@example.com", "charlie@example.com"],
            "cc": "dave@example.com",
        }

        participants = discovery._extract_participants(metadata)

        assert "alice@example.com" in participants
        assert "bob@example.com" in participants
        assert "charlie@example.com" in participants
        assert "dave@example.com" in participants
        # Check lowercase normalization
        assert all(p == p.lower() for p in participants)

    def test_extract_participants_deduplication(self, discovery):
        """Test that participants are deduplicated."""
        metadata = {
            "from": "alice@example.com",
            "to": ["alice@example.com", "bob@example.com"],
        }

        participants = discovery._extract_participants(metadata)

        assert participants.count("alice@example.com") == 1

    def test_extract_project_refs(self, discovery):
        """Test project reference extraction from metadata."""
        metadata = {
            "projects": ["Atlas", "Beta"],
            "labels": ["urgent", "review"],
        }

        project_refs = discovery._extract_project_refs(metadata)

        assert "atlas" in project_refs
        assert "beta" in project_refs
        assert "urgent" in project_refs
        assert "review" in project_refs

    def test_extract_title(self, discovery):
        """Test title extraction from metadata."""
        # Test with subject
        metadata = {"subject": "Important Email"}
        assert discovery._extract_title(metadata) == "Important Email"

        # Test with title
        metadata = {"title": "Document Title"}
        assert discovery._extract_title(metadata) == "Document Title"

        # Test with name
        metadata = {"name": "File Name"}
        assert discovery._extract_title(metadata) == "File Name"

        # Test empty
        metadata = {}
        assert discovery._extract_title(metadata) is None

    def test_map_content_type(self, discovery):
        """Test content type mapping."""
        assert discovery._map_content_type("email") == ContentTypeFilter.EMAIL
        assert discovery._map_content_type("gmail") == ContentTypeFilter.EMAIL
        assert discovery._map_content_type("meeting") == ContentTypeFilter.MEETING
        assert discovery._map_content_type("calendar") == ContentTypeFilter.MEETING
        assert discovery._map_content_type("slack") == ContentTypeFilter.SLACK
        assert discovery._map_content_type("document") == ContentTypeFilter.DOCUMENT
        assert discovery._map_content_type("unknown") == ContentTypeFilter.ALL
        assert discovery._map_content_type(None) == ContentTypeFilter.ALL


class TestRelatedItem:
    """Test RelatedItem dataclass."""

    def test_related_item_creation(self):
        """Test creating a RelatedItem."""
        now = datetime.now(timezone.utc)
        item = RelatedItem(
            entity_id=123,
            entity_type="source",
            content_type=ContentTypeFilter.EMAIL,
            title="Test Email",
            timestamp=now,
            correlation_score=0.85,
            correlation_type="participants",
            score_breakdown={"participants": 0.9, "temporal": 0.5},
        )

        assert item.entity_id == 123
        assert item.entity_type == "source"
        assert item.content_type == ContentTypeFilter.EMAIL
        assert item.title == "Test Email"
        assert item.timestamp == now
        assert item.correlation_score == 0.85
        assert item.correlation_type == "participants"
        assert item.score_breakdown == {"participants": 0.9, "temporal": 0.5}


class TestCorrelationResult:
    """Test CorrelationResult dataclass."""

    def test_correlation_result_creation(self):
        """Test creating a CorrelationResult."""
        items = [
            RelatedItem(
                entity_id=1,
                entity_type="source",
                content_type=ContentTypeFilter.EMAIL,
                title="Test",
                timestamp=datetime.now(timezone.utc),
                correlation_score=0.8,
                correlation_type="participants",
            ),
        ]
        result = CorrelationResult(
            source_id=100,
            source_type="source",
            related_items=items,
            total_correlations=1,
        )

        assert result.source_id == 100
        assert result.source_type == "source"
        assert len(result.related_items) == 1
        assert result.total_correlations == 1


class TestHelperFunctions:
    """Test helper functions for correlation discovery."""

    @pytest.fixture
    def mock_session(self):
        """Create a mock database session."""
        return AsyncMock()

    @pytest.mark.asyncio
    async def test_find_related_by_person_empty_results(self, mock_session):
        """Test finding related content by person with no results."""
        # Mock empty result
        mock_result = MagicMock()
        mock_result.__iter__ = lambda self: iter([])
        mock_session.execute.return_value = mock_result

        results = await find_related_by_person(
            session=mock_session,
            tenant_id=str(uuid4()),
            person_identifier="alice@example.com",
            limit=20,
        )

        assert results == []

    @pytest.mark.asyncio
    async def test_find_related_by_project_empty_results(self, mock_session):
        """Test finding related content by project with no results."""
        # Mock empty result
        mock_result = MagicMock()
        mock_result.__iter__ = lambda self: iter([])
        mock_session.execute.return_value = mock_result

        results = await find_related_by_project(
            session=mock_session,
            tenant_id=str(uuid4()),
            project_name="Atlas",
            limit=20,
        )

        assert results == []


@pytest.mark.integration
class TestCorrelationDiscoveryIntegration:
    """Integration tests requiring database connection."""

    @pytest.mark.asyncio
    async def test_participant_correlation_accuracy(self):
        """Test that participant-based correlations are accurate.

        Should:
        - Find content with overlapping participants
        - Score based on Jaccard similarity
        - Return higher scores for more overlap
        """
        pytest.skip("Not yet implemented - requires test database")

    @pytest.mark.asyncio
    async def test_project_correlation_accuracy(self):
        """Test that project-based correlations are accurate.

        Should:
        - Find content referencing same projects
        - Score based on project match proportion
        - Handle partial project name matches
        """
        pytest.skip("Not yet implemented - requires test database")

    @pytest.mark.asyncio
    async def test_temporal_correlation_decay(self):
        """Test that temporal correlations use proper decay.

        Should:
        - Score closer content higher
        - Respect temporal window boundaries
        - Handle edge cases at window boundaries
        """
        pytest.skip("Not yet implemented - requires test database")

    @pytest.mark.asyncio
    async def test_semantic_correlation_threshold(self):
        """Test semantic similarity threshold filtering.

        Should:
        - Only return results above similarity threshold
        - Use pgvector cosine distance correctly
        - Handle missing embeddings gracefully
        """
        pytest.skip("Not yet implemented - requires test database")

    @pytest.mark.asyncio
    async def test_thread_correlation_completeness(self):
        """Test thread-based correlations find all thread members.

        Should:
        - Find all content in same thread
        - Score thread members with full weight
        - Handle both thread_id and conversation_id
        """
        pytest.skip("Not yet implemented - requires test database")

    @pytest.mark.asyncio
    async def test_combined_correlation_ranking(self):
        """Test that combined correlations rank correctly.

        Should:
        - Combine multiple signal scores with weights
        - Rank results by combined score
        - Handle entities appearing in multiple signals
        """
        pytest.skip("Not yet implemented - requires test database")

    @pytest.mark.asyncio
    async def test_correlation_tenant_isolation(self):
        """Test that correlations respect tenant boundaries.

        Should:
        - Only find content within same tenant
        - Not leak data across tenants
        - Apply tenant filter to all signals
        """
        pytest.skip("Not yet implemented - requires test database")

    @pytest.mark.asyncio
    async def test_correlation_performance(self):
        """Test correlation discovery performance.

        Should:
        - Complete within acceptable latency (<5s)
        - Handle large content sets
        - Not degrade with many correlations
        """
        pytest.skip("Not yet implemented - requires test database")

    @pytest.mark.asyncio
    async def test_correlation_80_percent_accuracy(self):
        """Test that correlation accuracy meets 80% requirement (SC-004).

        Should:
        - Achieve 80% accuracy in cross-content correlation
        - Correctly identify related discussions
        - Minimize false positives and negatives
        """
        pytest.skip("Not yet implemented - requires test database with labeled data")
