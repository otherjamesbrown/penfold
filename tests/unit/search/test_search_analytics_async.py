"""Tests for async analytics recording in search engine.

These tests verify that:
1. The analytics background wrapper handles exceptions correctly
2. Search returns before analytics completes (fire-and-forget)
3. Analytics errors don't block or fail search operations
"""

import asyncio
import logging
from unittest.mock import AsyncMock, patch, MagicMock

import pytest


class TestAsyncAnalyticsWrapper:
    """Tests for the _record_analytics_background wrapper function."""

    @pytest.mark.asyncio
    async def test_wrapper_handles_exceptions_silently(self):
        """Verify exceptions are logged but not propagated."""
        from penf_lib.search.search_engine import _record_analytics_background

        async def failing_coro():
            raise RuntimeError("Analytics DB error")

        with patch("penf_lib.search.search_engine.logger") as mock_logger:
            # Should not raise - exceptions are caught
            await _record_analytics_background(failing_coro())

            # Should have logged a warning
            mock_logger.warning.assert_called_once()
            warning_call = str(mock_logger.warning.call_args)
            assert "Analytics" in warning_call or "failed" in warning_call

    @pytest.mark.asyncio
    async def test_wrapper_completes_successful_coroutines(self):
        """Verify successful analytics coroutines complete normally."""
        from penf_lib.search.search_engine import _record_analytics_background

        completed = False

        async def success_coro():
            nonlocal completed
            completed = True

        await _record_analytics_background(success_coro())
        assert completed is True

    @pytest.mark.asyncio
    async def test_wrapper_does_not_propagate_exceptions(self):
        """Verify no exceptions escape the wrapper."""
        from penf_lib.search.search_engine import _record_analytics_background

        async def failing_coro():
            raise ValueError("Test error")

        # This should not raise any exception
        try:
            await _record_analytics_background(failing_coro())
        except Exception as e:
            pytest.fail(f"Exception should not propagate: {e}")


class TestFireAndForgetBehavior:
    """Tests for fire-and-forget analytics behavior."""

    @pytest.mark.asyncio
    async def test_create_task_does_not_block(self):
        """Verify asyncio.create_task returns immediately."""
        from penf_lib.search.search_engine import _record_analytics_background

        # Track execution order
        execution_order = []

        async def slow_analytics():
            await asyncio.sleep(0.1)  # 100ms delay
            execution_order.append("analytics_complete")

        async def immediate_work():
            # Create task should return immediately
            task = asyncio.create_task(
                _record_analytics_background(slow_analytics())
            )
            execution_order.append("task_created")

            # Do some quick work
            await asyncio.sleep(0.01)  # 10ms
            execution_order.append("quick_work_done")

            # Wait for analytics to complete for cleanup
            await task

        await immediate_work()

        # Verify order: task created -> quick work -> analytics complete
        assert execution_order == ["task_created", "quick_work_done", "analytics_complete"]

    @pytest.mark.asyncio
    async def test_analytics_failure_does_not_block_parallel_work(self):
        """Verify analytics failure doesn't affect parallel operations."""
        from penf_lib.search.search_engine import _record_analytics_background

        results = []

        async def failing_analytics():
            await asyncio.sleep(0.05)
            raise RuntimeError("Analytics failed")

        async def important_work():
            await asyncio.sleep(0.01)
            results.append("work_completed")
            return "success"

        # Start analytics in background
        analytics_task = asyncio.create_task(
            _record_analytics_background(failing_analytics())
        )

        # Do important work - should complete despite analytics failure
        result = await important_work()
        assert result == "success"
        assert "work_completed" in results

        # Let analytics task complete (with failure)
        await analytics_task


class TestAnalyticsIntegration:
    """Integration tests for analytics recording in search flow."""

    @pytest.mark.asyncio
    async def test_analytics_called_but_not_awaited_directly(self):
        """Verify analytics is started but search doesn't wait for it."""
        from penf_lib.search.search_engine import _record_analytics_background

        analytics_started = asyncio.Event()
        analytics_completed = asyncio.Event()
        search_returned = asyncio.Event()

        async def mock_analytics():
            analytics_started.set()
            await asyncio.sleep(0.1)  # Simulate slow analytics
            analytics_completed.set()

        async def simulate_search_flow():
            # Fire analytics in background (like search does)
            asyncio.create_task(_record_analytics_background(mock_analytics()))

            # Mark search as returned immediately
            search_returned.set()

        await simulate_search_flow()

        # Search should have returned
        assert search_returned.is_set()

        # Analytics should have started
        assert analytics_started.is_set()

        # But analytics should NOT have completed yet (it takes 100ms)
        assert not analytics_completed.is_set()

        # Wait for cleanup
        await asyncio.sleep(0.15)
        assert analytics_completed.is_set()


class TestEdgeCases:
    """Edge case tests for analytics wrapper."""

    @pytest.mark.asyncio
    async def test_handles_cancelled_coroutine(self):
        """Verify wrapper handles asyncio.CancelledError gracefully."""
        from penf_lib.search.search_engine import _record_analytics_background

        async def cancellable_coro():
            await asyncio.sleep(10)  # Long sleep to be cancelled

        with patch("penf_lib.search.search_engine.logger") as mock_logger:
            task = asyncio.create_task(
                _record_analytics_background(cancellable_coro())
            )

            # Give task time to start
            await asyncio.sleep(0.01)

            # Cancel the task
            task.cancel()

            # Task cancellation should be handled
            with pytest.raises(asyncio.CancelledError):
                await task

    @pytest.mark.asyncio
    async def test_handles_none_coroutine_gracefully(self):
        """Verify wrapper handles edge case of awaiting None-like values."""
        from penf_lib.search.search_engine import _record_analytics_background

        async def noop_coro():
            return None

        # Should complete without issue
        result = await _record_analytics_background(noop_coro())
        assert result is None

    @pytest.mark.asyncio
    async def test_logs_specific_error_details(self, caplog):
        """Verify error logging includes useful details."""
        from penf_lib.search.search_engine import _record_analytics_background

        async def detailed_failure():
            raise ConnectionError("Database connection refused at localhost:5432")

        with caplog.at_level(logging.WARNING):
            await _record_analytics_background(detailed_failure())

        # Should have logged the error with details
        assert len(caplog.records) == 1
        assert "Analytics" in caplog.text or "failed" in caplog.text
        assert "connection" in caplog.text.lower() or "Connection" in caplog.text
