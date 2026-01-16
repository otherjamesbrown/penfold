"""Tests for correlation discovery with parallelized queries."""

import asyncio
import pytest
from unittest.mock import AsyncMock, MagicMock, patch
from datetime import datetime

from penf_lib.search.correlations import CorrelationDiscovery, CorrelationResult


class TestParallelCorrelationQueries:
    """Tests verifying parallel execution of correlation queries."""

    @pytest.fixture
    def discovery(self):
        """Create CorrelationDiscovery instance with mock session."""
        mock_session = AsyncMock()
        return CorrelationDiscovery(
            session=mock_session,
            tenant_id="test-tenant-id",
        )

    @pytest.fixture
    def mock_content(self):
        """Create mock content details."""
        content = MagicMock()
        content.participants = ["alice@example.com", "bob@example.com"]
        content.project_refs = ["project-1"]
        content.timestamp = datetime.utcnow()
        content.embedding = [0.1] * 768
        content.thread_id = "thread-123"
        return content

    @pytest.mark.asyncio
    async def test_all_correlation_methods_called(self, discovery, mock_content):
        """Verify all 5 correlation methods are called."""
        with patch.object(discovery, '_get_content', return_value=mock_content):
            with patch.object(discovery, '_find_by_participants', return_value=[]) as p:
                with patch.object(discovery, '_find_by_project', return_value=[]) as proj:
                    with patch.object(discovery, '_find_by_temporal_proximity', return_value=[]) as t:
                        with patch.object(discovery, '_find_by_similarity', return_value=[]) as s:
                            with patch.object(discovery, '_find_by_thread', return_value=[]) as th:
                                await discovery.find_related_content(1, "email")

                                p.assert_called_once()
                                proj.assert_called_once()
                                t.assert_called_once()
                                s.assert_called_once()
                                th.assert_called_once()

    @pytest.mark.asyncio
    async def test_parallel_execution_timing(self, discovery, mock_content):
        """Verify queries execute in parallel, not sequentially."""
        async def slow_query(*args):
            await asyncio.sleep(0.1)  # 100ms each
            return []

        with patch.object(discovery, '_get_content', return_value=mock_content):
            with patch.object(discovery, '_find_by_participants', side_effect=slow_query):
                with patch.object(discovery, '_find_by_project', side_effect=slow_query):
                    with patch.object(discovery, '_find_by_temporal_proximity', side_effect=slow_query):
                        with patch.object(discovery, '_find_by_similarity', side_effect=slow_query):
                            with patch.object(discovery, '_find_by_thread', side_effect=slow_query):
                                start = asyncio.get_event_loop().time()
                                await discovery.find_related_content(1, "email")
                                duration = asyncio.get_event_loop().time() - start

                                # 5 queries * 100ms = 500ms sequential
                                # Parallel should be ~100ms (plus overhead)
                                assert duration < 0.3, f"Expected parallel execution, took {duration}s"

    @pytest.mark.asyncio
    async def test_content_not_found_returns_empty(self, discovery):
        """Test behavior when source content not found."""
        with patch.object(discovery, '_get_content', return_value=None):
            result = await discovery.find_related_content(999, "email")

            assert isinstance(result, CorrelationResult)
            assert result.related_items == []
            assert result.total_correlations == 0

    @pytest.mark.asyncio
    async def test_results_merged_correctly(self, discovery, mock_content):
        """Test that results from all methods are merged."""
        mock_item = MagicMock()
        mock_item.correlation_score = 0.8

        with patch.object(discovery, '_get_content', return_value=mock_content):
            with patch.object(discovery, '_find_by_participants', return_value=[mock_item]):
                with patch.object(discovery, '_find_by_project', return_value=[]):
                    with patch.object(discovery, '_find_by_temporal_proximity', return_value=[]):
                        with patch.object(discovery, '_find_by_similarity', return_value=[]):
                            with patch.object(discovery, '_find_by_thread', return_value=[]):
                                with patch.object(discovery, '_merge_and_score', return_value=[mock_item]) as merge:
                                    result = await discovery.find_related_content(1, "email")

                                    merge.assert_called_once()
                                    assert len(result.related_items) <= 1

    @pytest.mark.asyncio
    async def test_empty_participants_handled(self, discovery):
        """Test handling of empty participant list."""
        content = MagicMock()
        content.participants = []
        content.project_refs = []
        content.timestamp = datetime.utcnow()
        content.embedding = None
        content.thread_id = None

        with patch.object(discovery, '_get_content', return_value=content):
            with patch.object(discovery, '_find_by_participants', return_value=[]):
                with patch.object(discovery, '_find_by_project', return_value=[]):
                    with patch.object(discovery, '_find_by_temporal_proximity', return_value=[]):
                        with patch.object(discovery, '_find_by_similarity', return_value=[]):
                            with patch.object(discovery, '_find_by_thread', return_value=[]):
                                result = await discovery.find_related_content(1, "email")
                                assert isinstance(result, CorrelationResult)
