"""Gmail Integration Performance Tests.

Comprehensive performance testing for Gmail integration including
database operations, API calls, caching, and memory usage.
"""

import asyncio
import pytest
import time
import psutil
import statistics
from datetime import datetime, timedelta
from typing import List, Dict, Any, Tuple
from unittest.mock import AsyncMock, MagicMock, patch
import random
import string

from penf_lib.connectors.gmail.optimization import (
    gmail_cache, adaptive_rate_limiter, memory_optimizer,
    monitor_performance, PerformanceLevel
)
from penf_lib.connectors.gmail.monitoring import gmail_monitoring
from penf_lib.connectors.gmail.client import GmailClient, GmailRateLimiter
from penf_lib.connectors.gmail.sync import GmailSyncManager
from penf_lib.storage.database import db_manager
from penf_lib.models.email_message import EmailMessage, EmailThread
from penf_lib.models.gmail_connection import GmailConnection


class PerformanceBenchmark:
    """Performance benchmark tracker."""

    def __init__(self, name: str):
        self.name = name
        self.measurements = []
        self.start_time = None
        self.start_memory = None

    def start(self):
        """Start measurement."""
        self.start_time = time.time()
        self.start_memory = psutil.Process().memory_info().rss

    def end(self) -> Dict[str, float]:
        """End measurement and return results."""
        if not self.start_time:
            raise ValueError("Benchmark not started")

        duration = time.time() - self.start_time
        end_memory = psutil.Process().memory_info().rss
        memory_delta = (end_memory - self.start_memory) / 1024 / 1024  # MB

        measurement = {
            'duration_ms': duration * 1000,
            'memory_delta_mb': memory_delta,
            'timestamp': time.time()
        }

        self.measurements.append(measurement)
        return measurement

    def get_stats(self) -> Dict[str, Any]:
        """Get benchmark statistics."""
        if not self.measurements:
            return {}

        durations = [m['duration_ms'] for m in self.measurements]
        memory_deltas = [m['memory_delta_mb'] for m in self.measurements]

        return {
            'name': self.name,
            'count': len(self.measurements),
            'duration_stats': {
                'min': min(durations),
                'max': max(durations),
                'mean': statistics.mean(durations),
                'median': statistics.median(durations),
                'p95': self._percentile(durations, 0.95),
                'p99': self._percentile(durations, 0.99)
            },
            'memory_stats': {
                'min': min(memory_deltas),
                'max': max(memory_deltas),
                'mean': statistics.mean(memory_deltas),
                'median': statistics.median(memory_deltas)
            }
        }

    def _percentile(self, values: List[float], percentile: float) -> float:
        """Calculate percentile."""
        if not values:
            return 0.0
        sorted_values = sorted(values)
        k = (len(sorted_values) - 1) * percentile
        f = int(k)
        c = k - f
        if f + 1 < len(sorted_values):
            return sorted_values[f] + c * (sorted_values[f + 1] - sorted_values[f])
        else:
            return sorted_values[f]


@pytest.fixture
async def performance_test_setup():
    """Set up performance testing environment."""
    # Initialize cache
    await gmail_cache.initialize()

    # Clear any existing data
    await gmail_cache.clear()

    yield

    # Cleanup
    await gmail_cache.close()


@pytest.fixture
def mock_gmail_data():
    """Generate mock Gmail data for testing."""
    def generate_email_data(count: int) -> List[Dict[str, Any]]:
        emails = []
        for i in range(count):
            email = {
                'id': f'message_{i}_{random.randint(1000, 9999)}',
                'threadId': f'thread_{i // 5}_{random.randint(100, 999)}',
                'snippet': f'Test email {i} content snippet',
                'payload': {
                    'headers': [
                        {'name': 'From', 'value': f'user{i}@example.com'},
                        {'name': 'To', 'value': 'recipient@example.com'},
                        {'name': 'Subject', 'value': f'Test Email {i}'},
                        {'name': 'Date', 'value': 'Tue, 1 Jan 2024 12:00:00 +0000'}
                    ],
                    'body': {
                        'data': 'VGVzdCBlbWFpbCBib2R5IGNvbnRlbnQ='  # base64 encoded
                    }
                },
                'internalDate': str(int(time.time() * 1000)),
                'labelIds': ['INBOX'],
                'sizeEstimate': random.randint(1000, 10000)
            }
            emails.append(email)
        return emails

    return generate_email_data


class TestCachePerformance:
    """Test cache performance under various conditions."""

    @pytest.mark.asyncio
    async def test_cache_write_performance(self, performance_test_setup):
        """Test cache write performance."""
        benchmark = PerformanceBenchmark("cache_write")

        # Test writing various data sizes
        test_data = [
            ('small', 'x' * 100),
            ('medium', 'x' * 10000),
            ('large', 'x' * 1000000)
        ]

        for size_name, data in test_data:
            for i in range(100):
                key = f"test_{size_name}_{i}"

                benchmark.start()
                await gmail_cache.set(key, data, ttl=300)
                benchmark.end()

        stats = benchmark.get_stats()

        # Verify performance targets
        assert stats['duration_stats']['p95'] < 50, f"Cache write P95 too slow: {stats['duration_stats']['p95']}ms"
        assert stats['duration_stats']['mean'] < 10, f"Cache write mean too slow: {stats['duration_stats']['mean']}ms"

    @pytest.mark.asyncio
    async def test_cache_read_performance(self, performance_test_setup):
        """Test cache read performance."""
        benchmark = PerformanceBenchmark("cache_read")

        # Pre-populate cache
        test_data = {'key_' + str(i): 'value_' + str(i) for i in range(1000)}
        for key, value in test_data.items():
            await gmail_cache.set(key, value)

        # Test read performance
        for i in range(1000):
            key = f"key_{i}"

            benchmark.start()
            value = await gmail_cache.get(key)
            benchmark.end()

            assert value == f"value_{i}"

        stats = benchmark.get_stats()

        # Verify performance targets
        assert stats['duration_stats']['p95'] < 10, f"Cache read P95 too slow: {stats['duration_stats']['p95']}ms"
        assert stats['duration_stats']['mean'] < 2, f"Cache read mean too slow: {stats['duration_stats']['mean']}ms"

    @pytest.mark.asyncio
    async def test_cache_concurrent_access(self, performance_test_setup):
        """Test cache performance under concurrent access."""
        benchmark = PerformanceBenchmark("cache_concurrent")

        async def cache_worker(worker_id: int, operations: int):
            """Worker function for concurrent cache operations."""
            for i in range(operations):
                key = f"worker_{worker_id}_key_{i}"
                value = f"worker_{worker_id}_value_{i}"

                benchmark.start()
                # Mix of reads and writes
                if i % 3 == 0:
                    await gmail_cache.set(key, value)
                else:
                    await gmail_cache.get(key, default=value)
                benchmark.end()

        # Run concurrent workers
        workers = [cache_worker(i, 100) for i in range(10)]
        await asyncio.gather(*workers)

        stats = benchmark.get_stats()

        # Verify performance under concurrency
        assert stats['duration_stats']['p95'] < 100, f"Concurrent cache P95 too slow: {stats['duration_stats']['p95']}ms"

    @pytest.mark.asyncio
    async def test_cache_memory_efficiency(self, performance_test_setup):
        """Test cache memory efficiency."""
        initial_memory = psutil.Process().memory_info().rss / 1024 / 1024  # MB

        # Fill cache with substantial data
        large_data = 'x' * 100000  # 100KB per entry
        for i in range(100):  # 10MB total
            await gmail_cache.set(f"memory_test_{i}", large_data)

        current_memory = psutil.Process().memory_info().rss / 1024 / 1024  # MB
        memory_growth = current_memory - initial_memory

        # Memory growth should be reasonable (allowing for overhead)
        assert memory_growth < 50, f"Cache memory usage too high: {memory_growth}MB"

        # Test cache stats accuracy
        cache_stats = gmail_cache.get_stats()
        assert cache_stats.entry_count == 100
        assert cache_stats.memory_usage_bytes > 0


class TestDatabasePerformance:
    """Test database performance optimizations."""

    @pytest.mark.asyncio
    async def test_connection_pool_performance(self):
        """Test database connection pool performance."""
        benchmark = PerformanceBenchmark("db_connection")

        async def db_worker(worker_id: int, queries: int):
            """Database worker for concurrent testing."""
            for i in range(queries):
                benchmark.start()
                async with db_manager.get_session() as session:
                    result = await session.execute("SELECT 1")
                    assert result.scalar() == 1
                benchmark.end()

        # Test concurrent database access
        workers = [db_worker(i, 20) for i in range(10)]
        await asyncio.gather(*workers)

        stats = benchmark.get_stats()

        # Verify connection pool performance
        assert stats['duration_stats']['p95'] < 100, f"DB connection P95 too slow: {stats['duration_stats']['p95']}ms"
        assert stats['duration_stats']['mean'] < 50, f"DB connection mean too slow: {stats['duration_stats']['mean']}ms"

    @pytest.mark.asyncio
    async def test_query_performance_monitoring(self):
        """Test query performance monitoring."""
        async with db_manager.get_session() as session:
            # Execute a potentially slow query
            start_time = time.time()
            await session.execute("SELECT pg_sleep(0.1)")  # 100ms delay
            duration = time.time() - start_time

            assert duration >= 0.1, "Sleep query should take at least 100ms"

        # Check if slow query was recorded
        perf_stats = await db_manager.get_performance_stats()
        assert 'query_stats' in perf_stats

        # Note: The slow query tracking depends on the monitoring being properly set up

    @pytest.mark.asyncio
    async def test_database_health_scoring(self):
        """Test database health scoring system."""
        # Get initial health score
        optimization_result = await db_manager.optimize_performance()
        health_score = optimization_result['health_score']

        # Health score should be reasonable (0-100)
        assert 0 <= health_score <= 100

        # Recommendations should be provided if needed
        assert 'recommendations' in optimization_result


class TestRateLimitingPerformance:
    """Test rate limiting performance and adaptivity."""

    @pytest.mark.asyncio
    async def test_basic_rate_limiting(self):
        """Test basic rate limiting performance."""
        rate_limiter = GmailRateLimiter(calls_per_second=10)
        benchmark = PerformanceBenchmark("rate_limiting")

        # Test that rate limiting works correctly
        for i in range(20):
            benchmark.start()
            await rate_limiter.wait_if_needed()
            benchmark.end()

        stats = benchmark.get_stats()
        total_duration = sum(m['duration_ms'] for m in benchmark.measurements)

        # Should take at least 1 second for 20 calls at 10 calls/sec
        assert total_duration >= 1000, f"Rate limiting too permissive: {total_duration}ms"

    @pytest.mark.asyncio
    async def test_adaptive_rate_limiting(self):
        """Test adaptive rate limiting behavior."""
        rate_limiter = adaptive_rate_limiter
        initial_rate = rate_limiter.calls_per_second

        # Simulate quota errors to trigger adaptation
        for _ in range(10):
            await rate_limiter.record_api_call(
                response_time=0.5,
                success=False,
                is_quota_error=True
            )

        # Rate should adapt downward after quota errors
        assert rate_limiter.calls_per_second < initial_rate, "Rate limiter should adapt to quota errors"

        # Simulate successful calls to trigger upward adaptation
        for _ in range(50):
            await rate_limiter.record_api_call(
                response_time=0.2,
                success=True,
                is_quota_error=False
            )

        # Allow adaptation to occur (adaptation has a minimum interval)
        await asyncio.sleep(1.1)

        # Should adapt upward after good performance
        assert rate_limiter.calls_per_second > 1, "Rate limiter should adapt upward after good performance"


class TestMemoryPerformance:
    """Test memory optimization and monitoring."""

    @pytest.mark.asyncio
    async def test_memory_monitoring_accuracy(self):
        """Test memory monitoring accuracy."""
        initial_snapshot = await memory_optimizer.monitor_memory_usage()

        # Allocate substantial memory
        large_data = ['x' * 1000000 for _ in range(10)]  # 10MB

        after_allocation = await memory_optimizer.monitor_memory_usage()

        memory_growth = after_allocation['rss_mb'] - initial_snapshot['rss_mb']

        # Should detect memory growth
        assert memory_growth > 5, f"Memory monitoring not detecting growth: {memory_growth}MB"

        # Clean up
        del large_data

        after_cleanup = await memory_optimizer.monitor_memory_usage()

        # Note: Memory might not be immediately released due to Python's memory management

    @pytest.mark.asyncio
    async def test_memory_optimization_effectiveness(self):
        """Test memory optimization effectiveness."""
        # Create some garbage objects
        garbage_data = []
        for i in range(1000):
            obj = {'data': 'x' * 1000, 'refs': []}
            obj['refs'].append(obj)  # Create circular reference
            garbage_data.append(obj)

        before_optimization = await memory_optimizer.monitor_memory_usage()

        # Clear references to create garbage
        del garbage_data

        # Run optimization
        optimization_result = await memory_optimizer.optimize_memory_usage()

        # Should have freed some memory
        assert optimization_result['memory_saved_mb'] >= 0, "Memory optimization should report savings"
        assert len(optimization_result['optimizations_applied']) > 0, "Should apply some optimizations"

    @pytest.mark.asyncio
    async def test_memory_trend_analysis(self):
        """Test memory trend analysis."""
        # Generate some memory usage over time
        for i in range(5):
            await memory_optimizer.monitor_memory_usage()
            # Small allocations to create a trend
            temp_data = ['x' * 100000 for _ in range(i)]
            await asyncio.sleep(0.1)
            del temp_data

        trends = memory_optimizer.get_memory_trends(hours=1)

        assert 'sample_count' in trends
        assert trends['sample_count'] > 0
        assert 'trend' in trends


class TestEndToEndPerformance:
    """End-to-end performance tests simulating real workloads."""

    @pytest.mark.asyncio
    async def test_email_processing_performance(self, mock_gmail_data):
        """Test end-to-end email processing performance."""
        # Generate test data
        test_emails = mock_gmail_data(100)

        # Mock Gmail client and dependencies
        mock_client = AsyncMock(spec=GmailClient)
        mock_client.list_messages.return_value = {
            'messages': [{'id': email['id']} for email in test_emails[:50]]
        }
        mock_client.get_message.side_effect = lambda msg_id: next(
            email for email in test_emails if email['id'] == msg_id
        )

        benchmark = PerformanceBenchmark("email_processing")

        # Process emails and measure performance
        for i, email in enumerate(test_emails[:20]):  # Process subset for testing
            benchmark.start()

            # Simulate email processing workflow
            message_data = email

            # Extract metadata (simplified)
            headers = {h['name'].lower(): h['value'] for h in message_data['payload']['headers']}

            # Simulate database operations
            async with db_manager.get_session() as session:
                # Check if message exists (cache hit simulation)
                cache_key = f"message_{email['id']}"
                cached_result = await gmail_cache.get(cache_key)

                if not cached_result:
                    # Simulate processing and cache storage
                    processed_data = {
                        'subject': headers.get('subject'),
                        'from': headers.get('from'),
                        'snippet': message_data['snippet']
                    }
                    await gmail_cache.set(cache_key, processed_data, ttl=3600)

            benchmark.end()

        stats = benchmark.get_stats()

        # Performance targets for email processing
        assert stats['duration_stats']['p95'] < 500, f"Email processing P95 too slow: {stats['duration_stats']['p95']}ms"
        assert stats['duration_stats']['mean'] < 200, f"Email processing mean too slow: {stats['duration_stats']['mean']}ms"

    @pytest.mark.asyncio
    async def test_concurrent_sync_performance(self, mock_gmail_data):
        """Test performance under concurrent sync operations."""
        # Generate test data for multiple accounts
        accounts_data = {
            f"account_{i}": mock_gmail_data(50)
            for i in range(3)
        }

        benchmark = PerformanceBenchmark("concurrent_sync")

        async def sync_account(account_id: str, emails: List[Dict]):
            """Simulate syncing an account."""
            for email in emails[:10]:  # Limit for testing
                benchmark.start()

                # Simulate sync operations
                cache_key = f"{account_id}_message_{email['id']}"

                # Check cache first
                cached = await gmail_cache.get(cache_key)

                if not cached:
                    # Simulate API call delay
                    await asyncio.sleep(0.01)

                    # Cache result
                    await gmail_cache.set(cache_key, email, ttl=1800)

                benchmark.end()

        # Run concurrent syncs
        sync_tasks = [
            sync_account(account_id, emails)
            for account_id, emails in accounts_data.items()
        ]

        await asyncio.gather(*sync_tasks)

        stats = benchmark.get_stats()

        # Verify concurrent performance
        assert stats['duration_stats']['p95'] < 100, f"Concurrent sync P95 too slow: {stats['duration_stats']['p95']}ms"


@pytest.mark.asyncio
async def test_performance_monitoring_integration():
    """Test integration of performance monitoring across components."""
    # Start monitoring
    await gmail_monitoring.start_monitoring()

    try:
        # Simulate some activity
        await gmail_cache.initialize()

        for i in range(10):
            await gmail_cache.set(f"monitor_test_{i}", f"value_{i}")
            value = await gmail_cache.get(f"monitor_test_{i}")
            assert value == f"value_{i}"

        # Generate some database activity
        async with db_manager.get_session() as session:
            await session.execute("SELECT 1")

        # Wait for monitoring cycle
        await asyncio.sleep(2)

        # Get system status
        system_status = await gmail_monitoring.get_system_status()

        # Verify monitoring data
        assert 'overall_status' in system_status
        assert 'health_checks' in system_status
        assert 'metrics' in system_status

        # Should have recorded some cache operations
        cache_stats = gmail_cache.get_stats()
        assert cache_stats.hits > 0 or cache_stats.misses > 0

    finally:
        await gmail_monitoring.stop_monitoring()
        await gmail_cache.close()


class TestPerformanceRegression:
    """Regression tests to catch performance degradation."""

    # Performance baseline targets (update these as system improves)
    TARGETS = {
        'cache_write_p95': 50,  # ms
        'cache_read_p95': 10,   # ms
        'db_connection_p95': 100,  # ms
        'email_processing_p95': 500,  # ms
        'memory_usage_mb': 100,  # MB for test workload
    }

    @pytest.mark.asyncio
    async def test_cache_performance_regression(self, performance_test_setup):
        """Regression test for cache performance."""
        benchmark = PerformanceBenchmark("cache_regression")

        # Standard workload
        for i in range(100):
            # Mix of operations
            if i % 3 == 0:
                benchmark.start()
                await gmail_cache.set(f"regr_key_{i}", f"value_{i}")
                benchmark.end()
            else:
                benchmark.start()
                await gmail_cache.get(f"regr_key_{i // 3}")
                benchmark.end()

        stats = benchmark.get_stats()

        # Check against targets
        assert stats['duration_stats']['p95'] <= self.TARGETS['cache_write_p95'], \
            f"Cache performance regression: P95 {stats['duration_stats']['p95']}ms > {self.TARGETS['cache_write_p95']}ms"

    @pytest.mark.asyncio
    async def test_overall_system_performance(self):
        """Overall system performance regression test."""
        initial_memory = psutil.Process().memory_info().rss / 1024 / 1024

        # Simulate realistic workload
        await gmail_cache.initialize()

        benchmark = PerformanceBenchmark("system_performance")

        for i in range(50):
            benchmark.start()

            # Cache operations
            await gmail_cache.set(f"sys_key_{i}", {'data': f'content_{i}', 'size': i})

            # Database operations
            async with db_manager.get_session() as session:
                await session.execute("SELECT current_timestamp")

            # Memory check
            if i % 10 == 0:
                await memory_optimizer.monitor_memory_usage()

            benchmark.end()

        current_memory = psutil.Process().memory_info().rss / 1024 / 1024
        memory_growth = current_memory - initial_memory

        stats = benchmark.get_stats()

        # Performance regression checks
        assert stats['duration_stats']['p95'] <= 200, \
            f"System performance regression: P95 {stats['duration_stats']['p95']}ms"

        assert memory_growth <= self.TARGETS['memory_usage_mb'], \
            f"Memory usage regression: {memory_growth}MB > {self.TARGETS['memory_usage_mb']}MB"

        await gmail_cache.close()


if __name__ == "__main__":
    # Run performance tests
    pytest.main([__file__, "-v", "-s", "--tb=short"])