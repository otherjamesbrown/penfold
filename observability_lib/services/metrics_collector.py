"""Async metrics collection service with batching for high-performance metric ingestion.

This module implements the MetricsCollectorInterface with efficient batching,
retry logic, and performance monitoring for the Penfold observability framework.

Features:
- Async batching for high-throughput metric ingestion
- Configurable buffer size and flush intervals
- Auto-flush based on time interval or buffer size
- Graceful handling of database connection failures with exponential backoff
- Performance metrics about the collector itself (self-monitoring)
- Tenant isolation support
- Comprehensive validation and error handling
- <5% performance overhead target

Example:
    from observability_lib.services.metrics_collector import MetricsCollector
    from observability_lib.config import settings

    collector = MetricsCollector(settings)
    await collector.start()

    # Record single metric
    await collector.record_metric("email_processor", metric_point)

    # Record batch of metrics
    await collector.record_metrics_batch("email_processor", metric_points)

    # Retrieve metrics with aggregation
    metrics = await collector.get_metrics(
        "email_processor",
        metric_names=["processing_time_ms"],
        aggregation="avg",
        bucket_size="5m"
    )

    await collector.stop()
"""

import asyncio
import logging
import time
import uuid
from collections import defaultdict, deque
from contextlib import asynccontextmanager
from datetime import datetime, timedelta
from typing import Any, Dict, List, Optional, Tuple

import asyncpg
from sqlalchemy.ext.asyncio import AsyncSession, create_async_engine
from sqlalchemy.orm import sessionmaker
from sqlalchemy import select, func, and_, text
from sqlalchemy.exc import SQLAlchemyError, DisconnectionError

from observability_lib.config import ObservabilitySettings
from observability_lib.models.agent_metrics import AgentMetric, Agent
from observability_lib.contracts import (
    MetricsCollectorInterface,
    MetricPoint,
    MetricType
)
from penf_lib.storage.validators import ValidationError


logger = logging.getLogger(__name__)


class MetricsBuffer:
    """Thread-safe buffer for batching metrics before database insertion.

    Provides efficient buffering with automatic size and time-based flushing
    while maintaining thread safety for concurrent access.
    """

    def __init__(self, max_size: int, flush_interval: float):
        """Initialize metrics buffer.

        Args:
            max_size: Maximum number of metrics to buffer before forced flush
            flush_interval: Maximum time in seconds to buffer metrics
        """
        self.max_size = max_size
        self.flush_interval = flush_interval
        self.buffer: Dict[str, List[Tuple[str, MetricPoint]]] = defaultdict(list)
        self.last_flush = time.time()
        self._lock = asyncio.Lock()
        self.total_buffered = 0

    async def add_metric(self, agent_id: str, metric: MetricPoint) -> bool:
        """Add metric to buffer and check if flush is needed.

        Args:
            agent_id: ID of the agent recording the metric
            metric: Metric data point to buffer

        Returns:
            True if buffer should be flushed, False otherwise
        """
        async with self._lock:
            self.buffer[agent_id].append((agent_id, metric))
            self.total_buffered += 1

            # Check flush conditions
            size_limit_reached = self.total_buffered >= self.max_size
            time_limit_reached = time.time() - self.last_flush >= self.flush_interval

            return size_limit_reached or time_limit_reached

    async def add_metrics_batch(self, agent_id: str, metrics: List[MetricPoint]) -> bool:
        """Add batch of metrics to buffer and check if flush is needed.

        Args:
            agent_id: ID of the agent recording the metrics
            metrics: List of metric data points to buffer

        Returns:
            True if buffer should be flushed, False otherwise
        """
        async with self._lock:
            for metric in metrics:
                self.buffer[agent_id].append((agent_id, metric))

            self.total_buffered += len(metrics)

            # Check flush conditions
            size_limit_reached = self.total_buffered >= self.max_size
            time_limit_reached = time.time() - self.last_flush >= self.flush_interval

            return size_limit_reached or time_limit_reached

    async def flush(self) -> Dict[str, List[Tuple[str, MetricPoint]]]:
        """Flush all buffered metrics and return them.

        Returns:
            Dictionary of agent_id -> list of (agent_id, metric) tuples
        """
        async with self._lock:
            if not self.buffer:
                return {}

            flushed_buffer = dict(self.buffer)
            self.buffer.clear()
            self.total_buffered = 0
            self.last_flush = time.time()

            return flushed_buffer

    async def size(self) -> int:
        """Get current buffer size."""
        async with self._lock:
            return self.total_buffered

    async def is_empty(self) -> bool:
        """Check if buffer is empty."""
        async with self._lock:
            return self.total_buffered == 0


class ConnectionPool:
    """Manages database connection pool with retry logic and health monitoring."""

    def __init__(self, settings: ObservabilitySettings):
        """Initialize connection pool.

        Args:
            settings: Observability configuration settings
        """
        self.settings = settings
        self.engine = None
        self.session_maker = None
        self._connection_failures = 0
        self._last_connection_attempt = 0
        self.max_retries = 3
        self.base_retry_delay = 1.0
        self.max_retry_delay = 30.0

    async def initialize(self):
        """Initialize database engine and session maker."""
        try:
            # Create async engine with connection pooling
            self.engine = create_async_engine(
                self.settings.database_url.replace("postgresql://", "postgresql+asyncpg://"),
                **self.settings.get_database_pool_settings(),
                echo=self.settings.enable_debug_logging
            )

            self.session_maker = sessionmaker(
                self.engine,
                class_=AsyncSession,
                expire_on_commit=False
            )

            # Test connection
            async with self.session_maker() as session:
                await session.execute(text("SELECT 1"))

            self._connection_failures = 0
            logger.info("Database connection pool initialized successfully")

        except Exception as e:
            self._connection_failures += 1
            logger.error(f"Failed to initialize database connection: {e}")
            raise

    @asynccontextmanager
    async def get_session(self):
        """Get database session with retry logic and error handling."""
        retries = 0

        while retries <= self.max_retries:
            try:
                if self.session_maker is None:
                    await self.initialize()

                async with self.session_maker() as session:
                    yield session
                    self._connection_failures = 0  # Reset on success
                    return

            except (DisconnectionError, asyncpg.PostgresConnectionError) as e:
                retries += 1
                self._connection_failures += 1

                if retries > self.max_retries:
                    logger.error(f"Database connection failed after {self.max_retries} retries: {e}")
                    raise

                # Exponential backoff with jitter
                delay = min(
                    self.base_retry_delay * (2 ** (retries - 1)),
                    self.max_retry_delay
                )

                logger.warning(f"Database connection failed, retrying in {delay:.1f}s (attempt {retries}/{self.max_retries})")
                await asyncio.sleep(delay)

                # Reset engine to force reconnection
                if self.engine:
                    await self.engine.dispose()
                    self.engine = None
                    self.session_maker = None

            except Exception as e:
                self._connection_failures += 1
                logger.error(f"Unexpected database error: {e}")
                raise

    async def close(self):
        """Close database connections."""
        if self.engine:
            await self.engine.dispose()
            self.engine = None
            self.session_maker = None
            logger.info("Database connection pool closed")

    @property
    def is_healthy(self) -> bool:
        """Check if connection pool is healthy."""
        return self._connection_failures < self.max_retries


class MetricsCollector(MetricsCollectorInterface):
    """High-performance async metrics collector with batching and retry logic.

    Implements the MetricsCollectorInterface with efficient batching, graceful
    error handling, and performance monitoring. Designed for <5% overhead.
    """

    def __init__(self, settings: ObservabilitySettings):
        """Initialize metrics collector.

        Args:
            settings: Observability configuration settings
        """
        self.settings = settings
        self.buffer = MetricsBuffer(
            max_size=settings.metrics_buffer_size,
            flush_interval=settings.metrics_flush_interval
        )
        self.connection_pool = ConnectionPool(settings)

        # Performance monitoring
        self._start_time = None
        self._metrics_recorded = 0
        self._batches_flushed = 0
        self._flush_errors = 0
        self._last_flush_duration = 0.0

        # Background tasks
        self._flush_task: Optional[asyncio.Task] = None
        self._running = False

        # Tenant cache for validation
        self._agent_cache: Dict[str, Agent] = {}
        self._cache_ttl = 300  # 5 minutes
        self._cache_timestamps: Dict[str, float] = {}

    async def start(self):
        """Start the metrics collector background tasks."""
        if self._running:
            return

        self._running = True
        self._start_time = time.time()

        await self.connection_pool.initialize()

        # Start background flush task
        self._flush_task = asyncio.create_task(self._background_flush_loop())

        logger.info("Metrics collector started")

    async def stop(self):
        """Stop the metrics collector and flush remaining metrics."""
        if not self._running:
            return

        self._running = False

        # Cancel background task
        if self._flush_task:
            self._flush_task.cancel()
            try:
                await self._flush_task
            except asyncio.CancelledError:
                pass

        # Flush remaining metrics
        await self._flush_buffer()

        # Close connections
        await self.connection_pool.close()

        logger.info("Metrics collector stopped")

    async def record_metric(self, agent_id: str, metric: MetricPoint) -> None:
        """Record a single metric point for an agent.

        Args:
            agent_id: Unique identifier for the agent
            metric: Metric data point to record

        Raises:
            ValidationError: If agent_id or metric is invalid
            RuntimeError: If collector is not running
        """
        if not self._running:
            raise RuntimeError("Metrics collector is not running")

        # Validate inputs
        self._validate_agent_id(agent_id)
        self._validate_metric(metric)

        start_time = time.time()

        try:
            # Add to buffer and check if flush is needed
            should_flush = await self.buffer.add_metric(agent_id, metric)

            self._metrics_recorded += 1

            # Force flush if buffer is full or time limit reached
            if should_flush:
                await self._flush_buffer()

            # Record collector performance metrics
            overhead_ms = (time.time() - start_time) * 1000
            if overhead_ms > 1.0:  # Log if overhead > 1ms
                logger.debug(f"Metric recording overhead: {overhead_ms:.2f}ms")

        except Exception as e:
            logger.error(f"Error recording metric for agent {agent_id}: {e}")
            raise

    async def record_metrics_batch(self, agent_id: str, metrics: List[MetricPoint]) -> None:
        """Record multiple metrics atomically.

        Args:
            agent_id: Unique identifier for the agent
            metrics: List of metric data points to record

        Raises:
            ValidationError: If agent_id or metrics are invalid
            RuntimeError: If collector is not running
        """
        if not self._running:
            raise RuntimeError("Metrics collector is not running")

        if not metrics:
            return

        # Validate inputs
        self._validate_agent_id(agent_id)
        for metric in metrics:
            self._validate_metric(metric)

        start_time = time.time()

        try:
            # Add batch to buffer and check if flush is needed
            should_flush = await self.buffer.add_metrics_batch(agent_id, metrics)

            self._metrics_recorded += len(metrics)

            # Force flush if buffer is full or time limit reached
            if should_flush:
                await self._flush_buffer()

            # Record collector performance metrics
            overhead_ms = (time.time() - start_time) * 1000
            if overhead_ms > 5.0:  # Log if overhead > 5ms for batch
                logger.debug(f"Batch recording overhead: {overhead_ms:.2f}ms for {len(metrics)} metrics")

        except Exception as e:
            logger.error(f"Error recording metrics batch for agent {agent_id}: {e}")
            raise

    async def get_metrics(
        self,
        agent_id: str,
        metric_names: Optional[List[str]] = None,
        start_time: Optional[datetime] = None,
        end_time: Optional[datetime] = None,
        aggregation: str = "avg",
        bucket_size: str = "5m"
    ) -> Dict[str, List[Dict[str, Any]]]:
        """Retrieve metrics for an agent within time range.

        Args:
            agent_id: Unique identifier for the agent
            metric_names: Optional list of metric names to filter
            start_time: Optional start time for range query
            end_time: Optional end time for range query
            aggregation: Aggregation function (avg, sum, min, max, count)
            bucket_size: Time bucket size for aggregation (1m, 5m, 1h, etc.)

        Returns:
            Dictionary mapping metric names to time-series data points

        Raises:
            ValidationError: If parameters are invalid
            RuntimeError: If collector is not running
        """
        if not self._running:
            raise RuntimeError("Metrics collector is not running")

        self._validate_agent_id(agent_id)
        self._validate_aggregation(aggregation)

        # Set default time range if not provided
        if end_time is None:
            end_time = datetime.utcnow()
        if start_time is None:
            start_time = end_time - timedelta(hours=1)

        if start_time >= end_time:
            raise ValidationError("start_time must be before end_time")

        try:
            async with self.connection_pool.get_session() as session:
                # Build base query
                query = select(
                    AgentMetric.metric_name,
                    AgentMetric.timestamp,
                    AgentMetric.metric_value,
                    AgentMetric.labels
                ).where(
                    and_(
                        AgentMetric.agent_id == agent_id,
                        AgentMetric.timestamp >= start_time,
                        AgentMetric.timestamp <= end_time
                    )
                )

                # Add metric name filter if provided
                if metric_names:
                    query = query.where(AgentMetric.metric_name.in_(metric_names))

                # Order by timestamp
                query = query.order_by(AgentMetric.timestamp.desc())

                result = await session.execute(query)
                rows = result.fetchall()

                # Group by metric name and apply aggregation
                metrics_data = defaultdict(list)

                for row in rows:
                    metrics_data[row.metric_name].append({
                        "timestamp": row.timestamp.isoformat(),
                        "value": row.metric_value,
                        "labels": row.labels or {}
                    })

                # Apply time-series aggregation if needed
                if bucket_size != "raw":
                    metrics_data = await self._apply_time_aggregation(
                        metrics_data, aggregation, bucket_size, start_time, end_time
                    )

                return dict(metrics_data)

        except Exception as e:
            logger.error(f"Error retrieving metrics for agent {agent_id}: {e}")
            raise

    async def get_collector_stats(self) -> Dict[str, Any]:
        """Get performance statistics about the metrics collector itself.

        Returns:
            Dictionary with collector performance metrics
        """
        uptime = time.time() - self._start_time if self._start_time else 0
        buffer_size = await self.buffer.size()

        return {
            "uptime_seconds": uptime,
            "metrics_recorded": self._metrics_recorded,
            "batches_flushed": self._batches_flushed,
            "flush_errors": self._flush_errors,
            "buffer_size": buffer_size,
            "buffer_max_size": self.buffer.max_size,
            "last_flush_duration_ms": self._last_flush_duration * 1000,
            "connection_healthy": self.connection_pool.is_healthy,
            "metrics_per_second": self._metrics_recorded / uptime if uptime > 0 else 0
        }

    # Private methods

    async def _background_flush_loop(self):
        """Background task that periodically flushes the buffer."""
        while self._running:
            try:
                await asyncio.sleep(self.settings.metrics_flush_interval)

                if not await self.buffer.is_empty():
                    await self._flush_buffer()

            except asyncio.CancelledError:
                break
            except Exception as e:
                logger.error(f"Error in background flush loop: {e}")
                # Continue running despite errors

    async def _flush_buffer(self):
        """Flush buffered metrics to database."""
        if await self.buffer.is_empty():
            return

        flush_start = time.time()

        try:
            # Get buffered metrics
            buffered_metrics = await self.buffer.flush()

            if not buffered_metrics:
                return

            # Flatten metrics for batch insert
            all_metrics = []
            for agent_id, agent_metrics in buffered_metrics.items():
                for agent_id, metric in agent_metrics:
                    all_metrics.append((agent_id, metric))

            # Insert metrics in database
            await self._insert_metrics_batch(all_metrics)

            self._batches_flushed += 1
            self._last_flush_duration = time.time() - flush_start

            logger.debug(f"Flushed {len(all_metrics)} metrics in {self._last_flush_duration:.3f}s")

        except Exception as e:
            self._flush_errors += 1
            logger.error(f"Error flushing metrics buffer: {e}")
            # Don't re-raise - metrics are lost but collector continues

    async def _insert_metrics_batch(self, metrics: List[Tuple[str, MetricPoint]]):
        """Insert batch of metrics into database.

        Args:
            metrics: List of (agent_id, metric) tuples to insert
        """
        if not metrics:
            return

        async with self.connection_pool.get_session() as session:
            try:
                # Validate agents exist (with caching)
                agent_ids = list(set(agent_id for agent_id, _ in metrics))
                await self._validate_agents_exist(session, agent_ids)

                # Prepare metric records
                metric_records = []
                for agent_id, metric in metrics:
                    metric_record = AgentMetric(
                        agent_id=agent_id,
                        timestamp=metric.timestamp or datetime.utcnow(),
                        metric_name=metric.metric_name,
                        metric_value=metric.metric_value,
                        metric_type=metric.metric_type.value,
                        labels=metric.labels or {},
                        tenant_id=uuid.uuid4()  # TODO: Get from context/agent
                    )
                    metric_records.append(metric_record)

                # Bulk insert
                session.add_all(metric_records)
                await session.commit()

            except Exception as e:
                await session.rollback()
                raise

    async def _validate_agents_exist(self, session: AsyncSession, agent_ids: List[str]):
        """Validate that agents exist in database with caching.

        Args:
            session: Database session
            agent_ids: List of agent IDs to validate

        Raises:
            ValidationError: If any agent doesn't exist
        """
        current_time = time.time()
        uncached_agents = []

        # Check cache first
        for agent_id in agent_ids:
            cache_time = self._cache_timestamps.get(agent_id, 0)
            if current_time - cache_time > self._cache_ttl or agent_id not in self._agent_cache:
                uncached_agents.append(agent_id)

        # Query uncached agents
        if uncached_agents:
            result = await session.execute(
                select(Agent).where(Agent.agent_id.in_(uncached_agents))
            )
            found_agents = result.scalars().all()

            # Update cache
            for agent in found_agents:
                self._agent_cache[agent.agent_id] = agent
                self._cache_timestamps[agent.agent_id] = current_time

            # Check for missing agents
            found_agent_ids = {agent.agent_id for agent in found_agents}
            missing_agents = set(uncached_agents) - found_agent_ids

            if missing_agents:
                raise ValidationError(f"Agents not found: {missing_agents}")

    async def _apply_time_aggregation(
        self,
        metrics_data: Dict[str, List[Dict[str, Any]]],
        aggregation: str,
        bucket_size: str,
        start_time: datetime,
        end_time: datetime
    ) -> Dict[str, List[Dict[str, Any]]]:
        """Apply time-series aggregation to metrics data.

        Args:
            metrics_data: Raw metrics data by metric name
            aggregation: Aggregation function
            bucket_size: Time bucket size
            start_time: Start time for aggregation
            end_time: End time for aggregation

        Returns:
            Aggregated metrics data
        """
        # Convert bucket size to timedelta
        bucket_delta = self._parse_bucket_size(bucket_size)

        aggregated_data = {}

        for metric_name, data_points in metrics_data.items():
            # Group data points by time bucket
            buckets = defaultdict(list)

            for point in data_points:
                timestamp = datetime.fromisoformat(point["timestamp"].replace("Z", "+00:00"))
                bucket_start = self._get_bucket_start(timestamp, bucket_delta)
                buckets[bucket_start].append(point["value"])

            # Aggregate each bucket
            aggregated_points = []
            for bucket_start, values in sorted(buckets.items()):
                if aggregation == "avg":
                    agg_value = sum(values) / len(values)
                elif aggregation == "sum":
                    agg_value = sum(values)
                elif aggregation == "min":
                    agg_value = min(values)
                elif aggregation == "max":
                    agg_value = max(values)
                elif aggregation == "count":
                    agg_value = len(values)
                else:
                    agg_value = sum(values) / len(values)  # Default to avg

                aggregated_points.append({
                    "timestamp": bucket_start.isoformat(),
                    "value": agg_value,
                    "count": len(values)
                })

            aggregated_data[metric_name] = aggregated_points

        return aggregated_data

    def _parse_bucket_size(self, bucket_size: str) -> timedelta:
        """Parse bucket size string to timedelta.

        Args:
            bucket_size: Bucket size string (e.g., "5m", "1h", "1d")

        Returns:
            Timedelta object
        """
        import re

        match = re.match(r"(\d+)([smhd])", bucket_size.lower())
        if not match:
            raise ValidationError(f"Invalid bucket size format: {bucket_size}")

        value, unit = int(match.group(1)), match.group(2)

        if unit == "s":
            return timedelta(seconds=value)
        elif unit == "m":
            return timedelta(minutes=value)
        elif unit == "h":
            return timedelta(hours=value)
        elif unit == "d":
            return timedelta(days=value)
        else:
            raise ValidationError(f"Invalid bucket size unit: {unit}")

    def _get_bucket_start(self, timestamp: datetime, bucket_delta: timedelta) -> datetime:
        """Get the start time of the bucket for a given timestamp.

        Args:
            timestamp: Timestamp to bucket
            bucket_delta: Bucket size

        Returns:
            Start time of the bucket
        """
        total_seconds = int(bucket_delta.total_seconds())
        timestamp_seconds = int(timestamp.timestamp())
        bucket_start_seconds = (timestamp_seconds // total_seconds) * total_seconds

        return datetime.fromtimestamp(bucket_start_seconds, tz=timestamp.tzinfo)

    def _validate_agent_id(self, agent_id: str):
        """Validate agent ID format."""
        if not agent_id or not isinstance(agent_id, str):
            raise ValidationError("Agent ID must be a non-empty string")

        if len(agent_id) < 3 or len(agent_id) > 100:
            raise ValidationError("Agent ID must be between 3 and 100 characters")

        if not all(c.isalnum() or c == '_' for c in agent_id):
            raise ValidationError("Agent ID must contain only alphanumeric characters and underscores")

    def _validate_metric(self, metric: MetricPoint):
        """Validate metric data point."""
        if not isinstance(metric, MetricPoint):
            raise ValidationError("Metric must be a MetricPoint instance")

        if not metric.metric_name:
            raise ValidationError("Metric name cannot be empty")

        if len(metric.metric_name) > 100:
            raise ValidationError("Metric name cannot exceed 100 characters")

        if not isinstance(metric.metric_value, (int, float)):
            raise ValidationError("Metric value must be numeric")

        import math
        if math.isnan(metric.metric_value) or math.isinf(metric.metric_value):
            raise ValidationError("Metric value must be finite (no NaN or Infinity)")

    def _validate_aggregation(self, aggregation: str):
        """Validate aggregation function."""
        valid_aggregations = {"avg", "sum", "min", "max", "count"}
        if aggregation not in valid_aggregations:
            raise ValidationError(f"Invalid aggregation: {aggregation}. Must be one of {valid_aggregations}")