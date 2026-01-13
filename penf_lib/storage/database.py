"""Database configuration and session management."""

import os
import time
import logging
import asyncio
from typing import AsyncGenerator, Optional, Dict, Any
from contextlib import asynccontextmanager
from sqlalchemy.ext.asyncio import AsyncSession, create_async_engine, async_sessionmaker
from sqlalchemy.orm import DeclarativeBase
from sqlalchemy import MetaData, event, text
from sqlalchemy.pool import QueuePool, StaticPool
from sqlalchemy.engine.events import PoolEvents
import psutil


logger = logging.getLogger(__name__)


# SQLAlchemy 2.0 style base
class Base(DeclarativeBase):
    """Base class for all SQLAlchemy models."""
    metadata = MetaData()


class PerformanceTracker:
    """Tracks database performance metrics."""

    def __init__(self):
        self.query_times = []
        self.connection_times = []
        self.slow_queries = []
        self.connection_stats = {
            'total_connections': 0,
            'active_connections': 0,
            'peak_connections': 0,
            'connection_errors': 0
        }

    def record_query_time(self, duration: float, query: str = None):
        """Record query execution time."""
        self.query_times.append({
            'duration': duration,
            'query': query,
            'timestamp': time.time()
        })

        # Track slow queries (>1 second)
        if duration > 1.0:
            self.slow_queries.append({
                'duration': duration,
                'query': query[:500] if query else 'Unknown',
                'timestamp': time.time()
            })

    def record_connection_event(self, event_type: str, duration: float = None):
        """Record connection events."""
        if event_type == 'checkout':
            self.connection_stats['active_connections'] += 1
            self.connection_stats['total_connections'] += 1
            self.connection_stats['peak_connections'] = max(
                self.connection_stats['peak_connections'],
                self.connection_stats['active_connections']
            )
            if duration:
                self.connection_times.append(duration)
        elif event_type == 'checkin':
            self.connection_stats['active_connections'] = max(
                0, self.connection_stats['active_connections'] - 1
            )
        elif event_type == 'error':
            self.connection_stats['connection_errors'] += 1

    def get_stats(self) -> Dict[str, Any]:
        """Get performance statistics."""
        now = time.time()
        recent_queries = [q for q in self.query_times if now - q['timestamp'] < 3600]  # Last hour

        return {
            'query_stats': {
                'total_queries': len(self.query_times),
                'recent_queries': len(recent_queries),
                'slow_queries': len(self.slow_queries),
                'avg_query_time': sum(q['duration'] for q in recent_queries) / max(len(recent_queries), 1),
                'max_query_time': max((q['duration'] for q in recent_queries), default=0),
            },
            'connection_stats': self.connection_stats.copy(),
            'connection_times': {
                'avg_checkout_time': sum(self.connection_times) / max(len(self.connection_times), 1),
                'max_checkout_time': max(self.connection_times, default=0),
                'total_checkouts': len(self.connection_times)
            }
        }


class OptimizedDatabaseManager:
    """Manages async database connections and sessions with performance optimization."""

    def __init__(self, database_url: Optional[str] = None, performance_level: str = "balanced"):
        """Initialize optimized database manager.

        Args:
            database_url: PostgreSQL connection URL.
                         If None, uses PENF_DATABASE_URL environment variable.
            performance_level: Performance optimization level (conservative, balanced, aggressive)
        """
        self.database_url = database_url or os.getenv(
            'PENF_DATABASE_URL',
            'postgresql+asyncpg://penfold:penfold@localhost/penfold'
        )
        self.performance_level = performance_level
        self.performance_tracker = PerformanceTracker()

        # Calculate optimal pool settings based on system resources and performance level
        pool_settings = self._calculate_pool_settings()

        self.engine = create_async_engine(
            self.database_url,
            echo=os.getenv('PENF_DB_ECHO', 'false').lower() == 'true',
            poolclass=QueuePool,
            **pool_settings,
            # Performance optimizations
            pool_pre_ping=True,
            pool_recycle=3600,
            pool_reset_on_return='commit',
            # Connection-level optimizations
            connect_args={
                "server_settings": {
                    "application_name": "penfold_gmail_integration",
                    "timezone": "UTC",
                    # Optimize for Gmail workloads
                    "shared_preload_libraries": "pg_stat_statements",
                    "track_activity_query_size": "2048",
                    "log_min_duration_statement": "1000",  # Log slow queries
                    "work_mem": "4MB",  # Optimize for sorting/joining
                    "maintenance_work_mem": "64MB",
                    "effective_cache_size": "256MB",
                    "random_page_cost": "1.1",  # SSD optimization
                }
            }
        )

        self.async_session_factory = async_sessionmaker(
            self.engine,
            class_=AsyncSession,
            expire_on_commit=False
        )

        # Set up performance monitoring
        self._setup_performance_monitoring()

    def _calculate_pool_settings(self) -> Dict[str, Any]:
        """Calculate optimal pool settings based on system resources."""
        cpu_count = psutil.cpu_count()
        memory_gb = psutil.virtual_memory().total / (1024**3)

        if self.performance_level == "conservative":
            pool_size = min(5, cpu_count)
            max_overflow = min(10, cpu_count * 2)
            pool_timeout = 30
        elif self.performance_level == "balanced":
            pool_size = min(20, cpu_count * 2)
            max_overflow = min(40, cpu_count * 4)
            pool_timeout = 20
        else:  # aggressive
            pool_size = min(50, cpu_count * 4)
            max_overflow = min(100, cpu_count * 8)
            pool_timeout = 10

        return {
            'pool_size': pool_size,
            'max_overflow': max_overflow,
            'pool_timeout': pool_timeout,
        }

    def _setup_performance_monitoring(self):
        """Set up database performance monitoring."""
        # Monitor connection checkout/checkin
        @event.listens_for(self.engine.sync_engine.pool, "checkout")
        def checkout_listener(dbapi_connection, connection_record, connection_proxy):
            checkout_time = time.time()
            connection_record.checkout_time = checkout_time
            self.performance_tracker.record_connection_event('checkout')

        @event.listens_for(self.engine.sync_engine.pool, "checkin")
        def checkin_listener(dbapi_connection, connection_record):
            if hasattr(connection_record, 'checkout_time'):
                duration = time.time() - connection_record.checkout_time
                self.performance_tracker.record_connection_event('checkin', duration)

        @event.listens_for(self.engine.sync_engine.pool, "invalidate")
        def invalidate_listener(dbapi_connection, connection_record, exception):
            self.performance_tracker.record_connection_event('error')
            logger.warning(f"Database connection invalidated: {exception}")

    @asynccontextmanager
    async def get_session(self) -> AsyncGenerator[AsyncSession, None]:
        """Get async database session with performance monitoring.

        Yields:
            Database session
        """
        session_start_time = time.time()
        async with self.async_session_factory() as session:
            try:
                # Monkey-patch execute to monitor query performance
                original_execute = session.execute

                async def monitored_execute(statement, parameters=None, execution_options=None, bind_arguments=None, **kwargs):
                    query_start_time = time.time()
                    try:
                        result = await original_execute(statement, parameters, execution_options, bind_arguments, **kwargs)
                        query_duration = time.time() - query_start_time

                        # Record query performance
                        query_str = str(statement).replace('\n', ' ').strip()
                        self.performance_tracker.record_query_time(query_duration, query_str)

                        # Log slow queries
                        if query_duration > 1.0:
                            logger.warning(f"Slow query ({query_duration:.2f}s): {query_str[:200]}...")

                        return result
                    except Exception as e:
                        query_duration = time.time() - query_start_time
                        logger.error(f"Query failed after {query_duration:.2f}s: {e}")
                        raise

                session.execute = monitored_execute
                yield session

            finally:
                session_duration = time.time() - session_start_time
                if session_duration > 5.0:
                    logger.warning(f"Long-running session: {session_duration:.2f}s")
                await session.close()

    async def get_performance_stats(self) -> Dict[str, Any]:
        """Get database performance statistics."""
        pool_status = self.engine.pool.status()

        base_stats = self.performance_tracker.get_stats()

        # Add pool status
        base_stats['pool_status'] = {
            'pool_size': self.engine.pool.size(),
            'checked_in': self.engine.pool.checkedin(),
            'checked_out': self.engine.pool.checkedout(),
            'overflow': self.engine.pool.overflow(),
            'total_connections': self.engine.pool.size() + self.engine.pool.overflow()
        }

        return base_stats

    async def optimize_performance(self) -> Dict[str, Any]:
        """Optimize database performance based on current metrics."""
        stats = await self.get_performance_stats()
        optimizations = []

        # Check if we need more connections
        pool_status = stats['pool_status']
        if pool_status['checked_out'] / pool_status['pool_size'] > 0.8:
            optimizations.append("Consider increasing pool_size")

        # Check for slow queries
        if stats['query_stats']['slow_queries'] > 10:
            optimizations.append("Investigate slow queries and add indexes")

        # Check connection checkout times
        conn_times = stats['connection_times']
        if conn_times['avg_checkout_time'] > 0.1:
            optimizations.append("Connection checkout time is high - check network/auth")

        return {
            'current_stats': stats,
            'recommendations': optimizations,
            'health_score': self._calculate_health_score(stats)
        }

    def _calculate_health_score(self, stats: Dict[str, Any]) -> float:
        """Calculate database health score (0-100)."""
        score = 100.0

        # Penalize slow queries
        slow_queries = stats['query_stats']['slow_queries']
        if slow_queries > 0:
            score -= min(50, slow_queries * 5)

        # Penalize connection errors
        conn_errors = stats['connection_stats']['connection_errors']
        if conn_errors > 0:
            score -= min(30, conn_errors * 10)

        # Penalize high connection usage
        pool_status = stats['pool_status']
        if pool_status['pool_size'] > 0:
            usage_ratio = pool_status['checked_out'] / pool_status['pool_size']
            if usage_ratio > 0.9:
                score -= 20

        # Penalize slow connection checkouts
        avg_checkout = stats['connection_times']['avg_checkout_time']
        if avg_checkout > 0.1:
            score -= min(20, avg_checkout * 100)

        return max(0, score)

    async def create_tables(self):
        """Create all tables with performance optimizations."""
        async with self.engine.begin() as conn:
            await conn.run_sync(Base.metadata.create_all)

            # Apply performance optimizations
            await self._apply_table_optimizations(conn)

    async def _apply_table_optimizations(self, conn):
        """Apply table-level performance optimizations."""
        optimizations = [
            # Enable auto-vacuum and analyze for Gmail tables
            "ALTER TABLE email_message SET (autovacuum_enabled = on, autovacuum_analyze_scale_factor = 0.1)",
            "ALTER TABLE email_thread SET (autovacuum_enabled = on, autovacuum_analyze_scale_factor = 0.1)",
            "ALTER TABLE gmail_connection SET (autovacuum_enabled = on)",

            # Set fillfactor for tables that get frequent updates
            "ALTER TABLE email_message SET (fillfactor = 90)",
            "ALTER TABLE gmail_connection SET (fillfactor = 90)",
        ]

        for sql in optimizations:
            try:
                await conn.execute(text(sql))
                logger.debug(f"Applied optimization: {sql}")
            except Exception as e:
                logger.warning(f"Failed to apply optimization '{sql}': {e}")

    async def drop_tables(self):
        """Drop all tables."""
        async with self.engine.begin() as conn:
            await conn.run_sync(Base.metadata.drop_all)

    async def close(self):
        """Close database engine."""
        await self.engine.dispose()


# Legacy DatabaseManager for backward compatibility
class DatabaseManager(OptimizedDatabaseManager):
    """Legacy database manager - redirects to optimized version."""

    def __init__(self, database_url: Optional[str] = None):
        super().__init__(database_url, performance_level="conservative")


# Global database manager instance with optimizations
db_manager = OptimizedDatabaseManager(
    performance_level=os.getenv('PENF_DB_PERFORMANCE_LEVEL', 'balanced')
)