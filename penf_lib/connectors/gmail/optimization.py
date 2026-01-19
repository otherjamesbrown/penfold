"""Gmail Integration Performance Optimization and Caching.

This module provides performance optimization capabilities for the Gmail integration
including caching, connection pooling, adaptive rate limiting, and resource management.
"""

import asyncio
import logging
import time
import hashlib
import json
import pickle
from datetime import datetime, timedelta
from typing import Dict, Any, Optional, List, Tuple, Union, Callable, TypeVar, Generic
from dataclasses import dataclass, field
from enum import Enum
import weakref
import threading
import psutil
import gc
from functools import wraps
from collections import defaultdict, deque
import aioredis
from sqlalchemy.ext.asyncio import AsyncSession, create_async_engine, async_sessionmaker
from sqlalchemy import text, select, func, Index
from sqlalchemy.pool import QueuePool
from sqlalchemy.engine.events import event

from .client import GmailRateLimiter
from .monitoring import gmail_monitoring, Metric, MetricType
from ...storage.database import db_manager
from ...models.email_message import EmailMessage, EmailThread
from ...models.gmail_connection import GmailConnection

logger = logging.getLogger(__name__)

T = TypeVar('T')


class CacheStrategy(Enum):
    """Cache strategy types."""
    LRU = "lru"
    TTL = "ttl"
    ADAPTIVE = "adaptive"
    WRITE_THROUGH = "write_through"
    WRITE_BEHIND = "write_behind"


class PerformanceLevel(Enum):
    """Performance optimization levels."""
    CONSERVATIVE = "conservative"
    BALANCED = "balanced"
    AGGRESSIVE = "aggressive"
    EXTREME = "extreme"


@dataclass
class CacheEntry:
    """Cache entry with metadata."""
    value: Any
    created_at: datetime = field(default_factory=datetime.utcnow)
    last_accessed: datetime = field(default_factory=datetime.utcnow)
    access_count: int = 0
    ttl_seconds: Optional[int] = None
    size_bytes: int = 0

    @property
    def is_expired(self) -> bool:
        """Check if cache entry is expired."""
        if not self.ttl_seconds:
            return False
        return datetime.utcnow() > self.created_at + timedelta(seconds=self.ttl_seconds)

    def access(self) -> None:
        """Record cache access."""
        self.last_accessed = datetime.utcnow()
        self.access_count += 1


@dataclass
class CacheStats:
    """Cache performance statistics."""
    hits: int = 0
    misses: int = 0
    evictions: int = 0
    memory_usage_bytes: int = 0
    entry_count: int = 0

    @property
    def hit_rate(self) -> float:
        """Calculate cache hit rate."""
        total = self.hits + self.misses
        return self.hits / total if total > 0 else 0.0


class GmailCache:
    """High-performance cache for Gmail data with multiple strategies."""

    def __init__(
        self,
        max_size: int = 10000,
        default_ttl: int = 3600,
        strategy: CacheStrategy = CacheStrategy.LRU,
        redis_url: Optional[str] = None
    ):
        """Initialize Gmail cache.

        Args:
            max_size: Maximum number of cache entries
            default_ttl: Default time-to-live in seconds
            strategy: Cache strategy to use
            redis_url: Optional Redis URL for distributed caching
        """
        self.max_size = max_size
        self.default_ttl = default_ttl
        self.strategy = strategy
        self.redis_url = redis_url

        self._cache: Dict[str, CacheEntry] = {}
        self._access_order: deque = deque()
        self._stats = CacheStats()
        self._lock = asyncio.Lock()
        self._redis: Optional[aioredis.Redis] = None

        # Background cleanup task
        self._cleanup_task: Optional[asyncio.Task] = None
        self._cleanup_interval = 300  # 5 minutes

    async def initialize(self) -> None:
        """Initialize cache and Redis connection if configured."""
        if self.redis_url:
            try:
                self._redis = await aioredis.from_url(self.redis_url)
                logger.info(f"Connected to Redis cache at {self.redis_url}")
            except Exception as e:
                logger.warning(f"Failed to connect to Redis: {e}, using in-memory cache")
                self._redis = None

        # Start background cleanup
        self._cleanup_task = asyncio.create_task(self._cleanup_loop())

    async def close(self) -> None:
        """Close cache and cleanup resources."""
        if self._cleanup_task:
            self._cleanup_task.cancel()
            try:
                await self._cleanup_task
            except asyncio.CancelledError:
                pass

        if self._redis:
            await self._redis.close()

    async def get(self, key: str, default: Any = None) -> Any:
        """Get value from cache.

        Args:
            key: Cache key
            default: Default value if key not found

        Returns:
            Cached value or default
        """
        async with self._lock:
            # Try Redis first if available
            if self._redis:
                try:
                    redis_value = await self._redis.get(f"gmail_cache:{key}")
                    if redis_value:
                        self._stats.hits += 1
                        await gmail_monitoring.metrics_collector.record_metric(
                            Metric("gmail_cache_hits", 1, MetricType.COUNTER, labels={"cache": "redis"})
                        )
                        return pickle.loads(redis_value)
                except Exception as e:
                    logger.warning(f"Redis cache error: {e}")

            # Try local cache
            if key in self._cache:
                entry = self._cache[key]

                if entry.is_expired:
                    # Remove expired entry
                    del self._cache[key]
                    self._stats.evictions += 1
                    self._stats.entry_count -= 1
                    self._stats.misses += 1
                    return default

                # Update access tracking
                entry.access()
                self._update_access_order(key)
                self._stats.hits += 1

                await gmail_monitoring.metrics_collector.record_metric(
                    Metric("gmail_cache_hits", 1, MetricType.COUNTER, labels={"cache": "local"})
                )

                return entry.value

            self._stats.misses += 1
            await gmail_monitoring.metrics_collector.record_metric(
                Metric("gmail_cache_misses", 1, MetricType.COUNTER)
            )
            return default

    async def set(
        self,
        key: str,
        value: Any,
        ttl: Optional[int] = None
    ) -> None:
        """Set value in cache.

        Args:
            key: Cache key
            value: Value to cache
            ttl: Time-to-live in seconds
        """
        async with self._lock:
            ttl = ttl or self.default_ttl

            # Calculate value size
            try:
                size_bytes = len(pickle.dumps(value))
            except Exception:
                size_bytes = 0

            # Store in Redis if available
            if self._redis:
                try:
                    serialized = pickle.dumps(value)
                    await self._redis.setex(f"gmail_cache:{key}", ttl, serialized)
                except Exception as e:
                    logger.warning(f"Redis cache error: {e}")

            # Store in local cache
            entry = CacheEntry(
                value=value,
                ttl_seconds=ttl,
                size_bytes=size_bytes
            )

            # Remove old entry if exists
            if key in self._cache:
                old_entry = self._cache[key]
                self._stats.memory_usage_bytes -= old_entry.size_bytes

            self._cache[key] = entry
            self._update_access_order(key)
            self._stats.memory_usage_bytes += size_bytes
            self._stats.entry_count += 1

            # Enforce size limits
            await self._enforce_size_limits()

    async def delete(self, key: str) -> None:
        """Delete key from cache.

        Args:
            key: Cache key to delete
        """
        async with self._lock:
            if self._redis:
                try:
                    await self._redis.delete(f"gmail_cache:{key}")
                except Exception as e:
                    logger.warning(f"Redis cache error: {e}")

            if key in self._cache:
                entry = self._cache[key]
                del self._cache[key]
                self._stats.memory_usage_bytes -= entry.size_bytes
                self._stats.entry_count -= 1

    async def clear(self) -> None:
        """Clear all cache entries."""
        async with self._lock:
            if self._redis:
                try:
                    await self._redis.flushdb()
                except Exception as e:
                    logger.warning(f"Redis cache error: {e}")

            self._cache.clear()
            self._access_order.clear()
            self._stats = CacheStats()

    def _update_access_order(self, key: str) -> None:
        """Update access order for LRU tracking."""
        if self.strategy == CacheStrategy.LRU:
            try:
                self._access_order.remove(key)
            except ValueError:
                pass
            self._access_order.append(key)

    async def _enforce_size_limits(self) -> None:
        """Enforce cache size limits using configured strategy."""
        while len(self._cache) > self.max_size:
            if self.strategy == CacheStrategy.LRU:
                # Remove least recently used
                if self._access_order:
                    lru_key = self._access_order.popleft()
                    if lru_key in self._cache:
                        entry = self._cache[lru_key]
                        del self._cache[lru_key]
                        self._stats.memory_usage_bytes -= entry.size_bytes
                        self._stats.entry_count -= 1
                        self._stats.evictions += 1
                else:
                    break
            else:
                # Remove oldest entry
                oldest_key = min(self._cache.keys(), key=lambda k: self._cache[k].created_at)
                entry = self._cache[oldest_key]
                del self._cache[oldest_key]
                self._stats.memory_usage_bytes -= entry.size_bytes
                self._stats.entry_count -= 1
                self._stats.evictions += 1

    async def _cleanup_loop(self) -> None:
        """Background cleanup of expired entries."""
        while True:
            try:
                await asyncio.sleep(self._cleanup_interval)
                await self._cleanup_expired()
            except asyncio.CancelledError:
                break
            except Exception as e:
                logger.error(f"Cache cleanup error: {e}")

    async def _cleanup_expired(self) -> None:
        """Remove expired cache entries."""
        async with self._lock:
            expired_keys = []
            for key, entry in self._cache.items():
                if entry.is_expired:
                    expired_keys.append(key)

            for key in expired_keys:
                entry = self._cache[key]
                del self._cache[key]
                self._stats.memory_usage_bytes -= entry.size_bytes
                self._stats.entry_count -= 1
                self._stats.evictions += 1

            if expired_keys:
                logger.debug(f"Cleaned up {len(expired_keys)} expired cache entries")

    def get_stats(self) -> CacheStats:
        """Get cache statistics."""
        return self._stats


class AdaptiveRateLimiter(GmailRateLimiter):
    """Adaptive rate limiter that adjusts based on quota usage and response times."""

    def __init__(
        self,
        initial_calls_per_second: int = 10,
        min_calls_per_second: int = 1,
        max_calls_per_second: int = 100,
        adaptation_window: int = 300  # 5 minutes
    ):
        """Initialize adaptive rate limiter.

        Args:
            initial_calls_per_second: Initial rate limit
            min_calls_per_second: Minimum rate limit
            max_calls_per_second: Maximum rate limit
            adaptation_window: Adaptation window in seconds
        """
        super().__init__(initial_calls_per_second)
        self.min_calls_per_second = min_calls_per_second
        self.max_calls_per_second = max_calls_per_second
        self.adaptation_window = adaptation_window

        # Tracking metrics
        self.response_times: deque = deque(maxlen=100)
        self.quota_errors: deque = deque(maxlen=50)
        self.success_count = 0
        self.error_count = 0

        # Adaptation state
        self.last_adaptation = datetime.utcnow()
        self.adaptation_factor = 1.0

    async def record_api_call(
        self,
        response_time: float,
        success: bool,
        is_quota_error: bool = False
    ) -> None:
        """Record API call metrics for adaptation.

        Args:
            response_time: Response time in seconds
            success: Whether the call was successful
            is_quota_error: Whether this was a quota/rate limit error
        """
        self.response_times.append(response_time)

        if success:
            self.success_count += 1
        else:
            self.error_count += 1

        if is_quota_error:
            self.quota_errors.append(datetime.utcnow())

        # Adapt rate limit if needed
        await self._adapt_rate_limit()

    async def _adapt_rate_limit(self) -> None:
        """Adapt rate limit based on recent performance."""
        now = datetime.utcnow()

        # Only adapt if enough time has passed
        if (now - self.last_adaptation).total_seconds() < 60:
            return

        # Calculate recent quota errors
        recent_quota_errors = sum(
            1 for error_time in self.quota_errors
            if (now - error_time).total_seconds() < self.adaptation_window
        )

        # Calculate average response time
        avg_response_time = sum(self.response_times) / len(self.response_times) if self.response_times else 0

        # Calculate error rate
        total_calls = self.success_count + self.error_count
        error_rate = self.error_count / total_calls if total_calls > 0 else 0

        # Determine adaptation direction
        if recent_quota_errors > 5:
            # Too many quota errors - slow down significantly
            self.adaptation_factor *= 0.5
        elif avg_response_time > 2.0:
            # Slow responses - back off slightly
            self.adaptation_factor *= 0.8
        elif error_rate > 0.1:
            # High error rate - be conservative
            self.adaptation_factor *= 0.9
        elif recent_quota_errors == 0 and avg_response_time < 0.5 and error_rate < 0.01:
            # Everything looks good - can be more aggressive
            self.adaptation_factor *= 1.1

        # Apply bounds
        self.adaptation_factor = max(0.1, min(5.0, self.adaptation_factor))

        # Update rate limit
        new_rate = int(self.calls_per_second * self.adaptation_factor)
        new_rate = max(self.min_calls_per_second, min(self.max_calls_per_second, new_rate))

        if new_rate != self.calls_per_second:
            logger.info(f"Adapted Gmail API rate limit: {self.calls_per_second} -> {new_rate} calls/sec")
            self.calls_per_second = new_rate

            # Record metric
            await gmail_monitoring.metrics_collector.record_metric(
                Metric(
                    "gmail_rate_limit_adaptation",
                    new_rate,
                    MetricType.GAUGE,
                    labels={
                        "quota_errors": str(recent_quota_errors),
                        "avg_response_time": f"{avg_response_time:.2f}",
                        "error_rate": f"{error_rate:.3f}"
                    }
                )
            )

        self.last_adaptation = now


class ConnectionPoolOptimizer:
    """Optimizes database connection pool settings for Gmail integration."""

    def __init__(self):
        """Initialize connection pool optimizer."""
        self.original_settings = {}
        self.optimized = False

    async def optimize_pool(
        self,
        performance_level: PerformanceLevel = PerformanceLevel.BALANCED
    ) -> Dict[str, Any]:
        """Optimize database connection pool settings.

        Args:
            performance_level: Optimization aggressiveness level

        Returns:
            Applied optimizations summary
        """
        if self.optimized:
            logger.warning("Connection pool already optimized")
            return {}

        # Get current engine settings
        engine = db_manager.engine

        # Store original settings
        self.original_settings = {
            'pool_size': engine.pool.size(),
            'max_overflow': engine.pool._max_overflow,
            'pool_pre_ping': engine.pool._pre_ping,
            'pool_recycle': engine.pool._recycle,
            'pool_reset_on_return': engine.pool._reset_on_return
        }

        # Calculate optimal settings based on performance level
        optimizations = self._calculate_pool_settings(performance_level)

        # Create new engine with optimized settings
        optimized_engine = create_async_engine(
            str(engine.url),
            echo=engine.echo,
            poolclass=QueuePool,
            **optimizations
        )

        # Replace engine in database manager
        await db_manager.engine.dispose()
        db_manager.engine = optimized_engine
        db_manager.async_session_factory = async_sessionmaker(
            optimized_engine,
            class_=AsyncSession,
            expire_on_commit=False
        )

        self.optimized = True
        logger.info(f"Optimized connection pool with settings: {optimizations}")

        return optimizations

    def _calculate_pool_settings(self, performance_level: PerformanceLevel) -> Dict[str, Any]:
        """Calculate optimal pool settings based on performance level."""
        # Get system resources
        cpu_count = psutil.cpu_count()
        memory_gb = psutil.virtual_memory().total / (1024**3)

        if performance_level == PerformanceLevel.CONSERVATIVE:
            pool_size = min(5, cpu_count)
            max_overflow = min(10, cpu_count * 2)
            pool_timeout = 30

        elif performance_level == PerformanceLevel.BALANCED:
            pool_size = min(20, cpu_count * 2)
            max_overflow = min(40, cpu_count * 4)
            pool_timeout = 20

        elif performance_level == PerformanceLevel.AGGRESSIVE:
            pool_size = min(50, cpu_count * 4)
            max_overflow = min(100, cpu_count * 8)
            pool_timeout = 10

        else:  # EXTREME
            pool_size = min(100, cpu_count * 8)
            max_overflow = min(200, cpu_count * 16)
            pool_timeout = 5

        return {
            'pool_size': pool_size,
            'max_overflow': max_overflow,
            'pool_timeout': pool_timeout,
            'pool_pre_ping': True,
            'pool_recycle': 3600,
            'pool_reset_on_return': 'commit'
        }

    async def restore_original_settings(self) -> None:
        """Restore original connection pool settings."""
        if not self.optimized:
            return

        engine = db_manager.engine

        # Create engine with original settings
        original_engine = create_async_engine(
            str(engine.url),
            echo=engine.echo,
            **self.original_settings
        )

        await engine.dispose()
        db_manager.engine = original_engine
        db_manager.async_session_factory = async_sessionmaker(
            original_engine,
            class_=AsyncSession,
            expire_on_commit=False
        )

        self.optimized = False
        logger.info("Restored original connection pool settings")


class QueryOptimizer:
    """Database query optimization for Gmail operations."""

    def __init__(self):
        """Initialize query optimizer."""
        self.query_plans: Dict[str, Dict] = {}
        self.slow_queries: deque = deque(maxlen=100)

    async def analyze_query_performance(self) -> Dict[str, Any]:
        """Analyze query performance and suggest optimizations."""
        async with db_manager.get_session() as session:
            # Analyze slow queries
            slow_query_analysis = await self._analyze_slow_queries(session)

            # Check index usage
            index_analysis = await self._analyze_index_usage(session)

            # Check table statistics
            table_stats = await self._analyze_table_statistics(session)

            return {
                'slow_queries': slow_query_analysis,
                'index_analysis': index_analysis,
                'table_statistics': table_stats,
                'recommendations': self._generate_recommendations(
                    slow_query_analysis, index_analysis, table_stats
                )
            }

    async def _analyze_slow_queries(self, session: AsyncSession) -> List[Dict[str, Any]]:
        """Analyze slow queries from pg_stat_statements."""
        try:
            result = await session.execute(text("""
                SELECT query, calls, total_time, mean_time, rows, 100.0 * shared_blks_hit /
                       nullif(shared_blks_hit + shared_blks_read, 0) AS hit_percent
                FROM pg_stat_statements
                WHERE query LIKE '%gmail%' OR query LIKE '%email%'
                ORDER BY mean_time DESC
                LIMIT 10
            """))

            return [
                {
                    'query': row[0][:200] + '...' if len(row[0]) > 200 else row[0],
                    'calls': row[1],
                    'total_time_ms': row[2],
                    'mean_time_ms': row[3],
                    'rows_avg': row[4] / max(row[1], 1),
                    'cache_hit_percent': row[5] or 0
                }
                for row in result.fetchall()
            ]
        except Exception as e:
            logger.warning(f"Could not analyze slow queries: {e}")
            return []

    async def _analyze_index_usage(self, session: AsyncSession) -> Dict[str, Any]:
        """Analyze index usage patterns."""
        try:
            # Get index usage stats
            result = await session.execute(text("""
                SELECT
                    schemaname,
                    tablename,
                    indexname,
                    idx_scan,
                    idx_tup_read,
                    idx_tup_fetch
                FROM pg_stat_user_indexes
                WHERE schemaname = 'public'
                AND (tablename LIKE '%email%' OR tablename LIKE '%gmail%')
                ORDER BY idx_scan DESC
            """))

            indexes = []
            for row in result.fetchall():
                indexes.append({
                    'table': row[1],
                    'index': row[2],
                    'scans': row[3],
                    'tuples_read': row[4],
                    'tuples_fetched': row[5],
                    'efficiency': row[5] / max(row[4], 1) if row[4] > 0 else 0
                })

            return {
                'indexes': indexes,
                'unused_indexes': [idx for idx in indexes if idx['scans'] == 0],
                'inefficient_indexes': [idx for idx in indexes if idx['efficiency'] < 0.1]
            }

        except Exception as e:
            logger.warning(f"Could not analyze index usage: {e}")
            return {'indexes': [], 'unused_indexes': [], 'inefficient_indexes': []}

    async def _analyze_table_statistics(self, session: AsyncSession) -> Dict[str, Any]:
        """Analyze table statistics and sizes."""
        try:
            result = await session.execute(text("""
                SELECT
                    tablename,
                    n_live_tup as row_count,
                    n_dead_tup as dead_rows,
                    last_vacuum,
                    last_autovacuum,
                    last_analyze,
                    last_autoanalyze
                FROM pg_stat_user_tables
                WHERE schemaname = 'public'
                AND (tablename LIKE '%email%' OR tablename LIKE '%gmail%')
            """))

            tables = []
            for row in result.fetchall():
                tables.append({
                    'table': row[0],
                    'row_count': row[1],
                    'dead_rows': row[2],
                    'dead_row_ratio': row[2] / max(row[1], 1),
                    'last_vacuum': row[3],
                    'last_analyze': row[5],
                    'needs_vacuum': row[2] / max(row[1], 1) > 0.1,
                    'needs_analyze': row[5] is None or
                                   (row[5] and (datetime.utcnow() - row[5]).days > 7)
                })

            return {
                'tables': tables,
                'total_rows': sum(t['row_count'] for t in tables),
                'maintenance_needed': [t for t in tables if t['needs_vacuum'] or t['needs_analyze']]
            }

        except Exception as e:
            logger.warning(f"Could not analyze table statistics: {e}")
            return {'tables': [], 'total_rows': 0, 'maintenance_needed': []}

    def _generate_recommendations(
        self,
        slow_queries: List[Dict],
        index_analysis: Dict[str, Any],
        table_stats: Dict[str, Any]
    ) -> List[str]:
        """Generate optimization recommendations."""
        recommendations = []

        # Slow query recommendations
        if slow_queries:
            avg_time = sum(q['mean_time_ms'] for q in slow_queries) / len(slow_queries)
            if avg_time > 100:
                recommendations.append("Consider optimizing queries with mean time > 100ms")

        # Index recommendations
        if index_analysis.get('unused_indexes'):
            recommendations.append(f"Remove {len(index_analysis['unused_indexes'])} unused indexes")

        if index_analysis.get('inefficient_indexes'):
            recommendations.append(f"Review {len(index_analysis['inefficient_indexes'])} inefficient indexes")

        # Table maintenance recommendations
        maintenance_needed = table_stats.get('maintenance_needed', [])
        if maintenance_needed:
            vacuum_needed = [t for t in maintenance_needed if t['needs_vacuum']]
            analyze_needed = [t for t in maintenance_needed if t['needs_analyze']]

            if vacuum_needed:
                recommendations.append(f"VACUUM needed for {len(vacuum_needed)} tables")
            if analyze_needed:
                recommendations.append(f"ANALYZE needed for {len(analyze_needed)} tables")

        return recommendations

    async def create_optimized_indexes(self) -> List[str]:
        """Create optimized indexes for Gmail queries."""
        indexes_created = []

        async with db_manager.get_session() as session:
            # Common Gmail query patterns
            index_definitions = [
                # Email message lookups
                "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_email_message_gmail_id ON email_message(gmail_message_id)",
                "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_email_message_thread_id ON email_message(thread_id)",
                "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_email_message_internal_date ON email_message(internal_date DESC)",
                "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_email_message_from_email ON email_message(from_email)",

                # Thread lookups
                "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_email_thread_gmail_id ON email_thread(gmail_thread_id)",
                "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_email_thread_connection_id ON email_thread(connection_id)",

                # Connection status
                "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_gmail_connection_active ON gmail_connection(is_active) WHERE is_active = true",

                # Composite indexes for common queries
                "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_email_message_connection_date ON email_message(connection_id, internal_date DESC)",
                "CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_email_message_processing_status ON email_message(processing_status) WHERE processing_status != 'completed'"
            ]

            for index_sql in index_definitions:
                try:
                    await session.execute(text(index_sql))
                    await session.commit()

                    index_name = index_sql.split('idx_')[1].split(' ')[0]
                    indexes_created.append(index_name)
                    logger.info(f"Created index: {index_name}")

                except Exception as e:
                    logger.warning(f"Failed to create index: {e}")
                    await session.rollback()

        return indexes_created


class MemoryOptimizer:
    """Memory usage optimization and monitoring."""

    def __init__(self):
        """Initialize memory optimizer."""
        self.memory_snapshots: deque = deque(maxlen=100)
        self.gc_stats: List[Dict] = []

    async def monitor_memory_usage(self) -> Dict[str, Any]:
        """Monitor current memory usage."""
        process = psutil.Process()
        memory_info = process.memory_info()
        memory_percent = process.memory_percent()

        # Python-specific memory info
        gc_stats = {
            'generation_0': gc.get_count()[0],
            'generation_1': gc.get_count()[1],
            'generation_2': gc.get_count()[2],
            'gc_collections': sum(gc.get_stats()[i]['collections'] for i in range(3)),
            'gc_collected': sum(gc.get_stats()[i]['collected'] for i in range(3)),
            'gc_uncollectable': sum(gc.get_stats()[i]['uncollectable'] for i in range(3))
        }

        memory_snapshot = {
            'timestamp': datetime.utcnow(),
            'rss_mb': memory_info.rss / 1024 / 1024,
            'vms_mb': memory_info.vms / 1024 / 1024,
            'percent': memory_percent,
            'gc_stats': gc_stats
        }

        self.memory_snapshots.append(memory_snapshot)

        # Record metrics
        await gmail_monitoring.metrics_collector.record_metric(
            Metric("gmail_memory_usage_mb", memory_snapshot['rss_mb'], MetricType.GAUGE)
        )
        await gmail_monitoring.metrics_collector.record_metric(
            Metric("gmail_memory_percent", memory_percent, MetricType.GAUGE)
        )

        return memory_snapshot

    async def optimize_memory_usage(self) -> Dict[str, Any]:
        """Perform memory optimization."""
        before_snapshot = await self.monitor_memory_usage()

        optimizations_applied = []

        # Force garbage collection
        collected_objects = []
        for generation in range(3):
            collected = gc.collect(generation)
            collected_objects.append(collected)

        if sum(collected_objects) > 0:
            optimizations_applied.append(f"Garbage collection freed {sum(collected_objects)} objects")

        # Clear weak references
        weak_refs_cleared = len(gc.get_referrers())
        if weak_refs_cleared > 1000:
            gc.collect()
            optimizations_applied.append("Cleared excessive weak references")

        # Optimize SQLAlchemy connection pool
        if hasattr(db_manager.engine.pool, 'invalidate'):
            db_manager.engine.pool.invalidate()
            optimizations_applied.append("Invalidated connection pool")

        after_snapshot = await self.monitor_memory_usage()

        memory_saved_mb = before_snapshot['rss_mb'] - after_snapshot['rss_mb']

        return {
            'before_mb': before_snapshot['rss_mb'],
            'after_mb': after_snapshot['rss_mb'],
            'memory_saved_mb': memory_saved_mb,
            'optimizations_applied': optimizations_applied,
            'gc_collections': collected_objects
        }

    def get_memory_trends(self, hours: int = 1) -> Dict[str, Any]:
        """Analyze memory usage trends."""
        since = datetime.utcnow() - timedelta(hours=hours)
        recent_snapshots = [
            s for s in self.memory_snapshots
            if s['timestamp'] > since
        ]

        if not recent_snapshots:
            return {}

        rss_values = [s['rss_mb'] for s in recent_snapshots]

        return {
            'sample_count': len(recent_snapshots),
            'min_memory_mb': min(rss_values),
            'max_memory_mb': max(rss_values),
            'avg_memory_mb': sum(rss_values) / len(rss_values),
            'current_memory_mb': rss_values[-1],
            'memory_growth_mb': rss_values[-1] - rss_values[0] if len(rss_values) > 1 else 0,
            'trend': 'increasing' if rss_values[-1] > rss_values[0] else 'decreasing'
        }


# Performance monitoring decorator
def monitor_performance(operation_name: str):
    """Decorator to monitor operation performance."""
    def decorator(func: Callable) -> Callable:
        @wraps(func)
        async def wrapper(*args, **kwargs):
            start_time = time.time()
            start_memory = psutil.Process().memory_info().rss

            try:
                result = await func(*args, **kwargs)
                success = True
                error = None
            except Exception as e:
                success = False
                error = str(e)
                raise
            finally:
                end_time = time.time()
                end_memory = psutil.Process().memory_info().rss

                duration_ms = (end_time - start_time) * 1000
                memory_delta_mb = (end_memory - start_memory) / 1024 / 1024

                # Record metrics
                await gmail_monitoring.metrics_collector.record_metric(
                    Metric(
                        f"gmail_operation_duration_ms",
                        duration_ms,
                        MetricType.HISTOGRAM,
                        labels={
                            "operation": operation_name,
                            "success": str(success)
                        }
                    )
                )

                await gmail_monitoring.metrics_collector.record_metric(
                    Metric(
                        f"gmail_operation_memory_delta_mb",
                        memory_delta_mb,
                        MetricType.HISTOGRAM,
                        labels={"operation": operation_name}
                    )
                )

                if not success:
                    logger.error(f"Operation {operation_name} failed after {duration_ms:.2f}ms: {error}")
                elif duration_ms > 5000:  # Log slow operations
                    logger.warning(f"Slow operation {operation_name}: {duration_ms:.2f}ms")

        return wrapper
    return decorator


# Global optimization instances
gmail_cache = GmailCache()
connection_pool_optimizer = ConnectionPoolOptimizer()
query_optimizer = QueryOptimizer()
memory_optimizer = MemoryOptimizer()
adaptive_rate_limiter = AdaptiveRateLimiter()