"""TimescaleDB-optimized storage adapter for observability framework.

This module provides async storage operations optimized for TimescaleDB's
time-series capabilities including hypertables, continuous aggregates,
batch operations, and tenant isolation through Row-Level Security.

Key Features:
- Async connection pooling with asyncpg
- Batch insert optimizations for high-throughput metrics
- TimescaleDB hypertable and continuous aggregate management
- Tenant isolation with Row-Level Security
- Query performance monitoring and optimization
- Automatic retry and error recovery
- Dashboard-optimized query methods

Example Usage:
    from observability_lib.storage.timeseries_adapter import TimescaleAdapter
    from observability_lib.config import settings

    adapter = TimescaleAdapter(settings.database_url)
    await adapter.initialize()

    # Insert metrics in batches
    metrics = [
        {
            "agent_id": "email_processor",
            "timestamp": datetime.utcnow(),
            "metric_name": "processing_time_ms",
            "metric_value": 150.0,
            "metric_type": "gauge",
            "labels": {"operation": "extract_entities"},
            "tenant_id": "work"
        }
    ]
    await adapter.batch_insert_metrics(metrics)

    # Query for dashboard
    recent_metrics = await adapter.get_agent_metrics(
        agent_id="email_processor",
        hours_back=24
    )
"""

import asyncio
import logging
import time
import uuid
from contextlib import asynccontextmanager
from datetime import datetime, timedelta
from typing import Any, Dict, List, Optional, Tuple, Union
from urllib.parse import urlparse

import asyncpg
from asyncpg.pool import Pool
from asyncpg.exceptions import (
    ConnectionDoesNotExistError,
    PostgresError,
    TooManyConnectionsError,
)

from observability_lib.config import ObservabilitySettings, get_settings
from observability_lib.models.agent_metrics import (
    Agent,
    AgentMetric,
    DecisionTrace,
    WorkflowEvent,
    AgentLog,
    AlertThreshold,
    AlertEvent,
    Tenant,
    MetricType,
    EventType,
    EventStatus,
    LogLevel,
    ThresholdType,
    ComparisonOperator,
    AlertStatus,
)

logger = logging.getLogger(__name__)


class TimescaleError(Exception):
    """Base exception for TimescaleDB adapter errors."""
    pass


class ConnectionPoolError(TimescaleError):
    """Exception for connection pool issues."""
    pass


class BatchInsertError(TimescaleError):
    """Exception for batch insert failures."""
    pass


class QueryOptimizationError(TimescaleError):
    """Exception for query optimization issues."""
    pass


class HypertableError(TimescaleError):
    """Exception for hypertable operations."""
    pass


class TenantIsolationError(TimescaleError):
    """Exception for tenant isolation violations."""
    pass


class TimescaleAdapter:
    """Async TimescaleDB storage adapter for observability framework.

    Provides high-performance time-series operations optimized for TimescaleDB
    with comprehensive error handling, connection pooling, and monitoring.
    """

    def __init__(
        self,
        database_url: str,
        settings: Optional[ObservabilitySettings] = None,
        pool_size: Optional[int] = None,
        max_connections: Optional[int] = None,
    ):
        """Initialize TimescaleDB adapter.

        Args:
            database_url: PostgreSQL/TimescaleDB connection URL
            settings: Observability settings (defaults to global settings)
            pool_size: Minimum pool size (overrides settings)
            max_connections: Maximum connections (overrides settings)
        """
        self.database_url = database_url
        self.settings = settings or get_settings()
        self.pool: Optional[Pool] = None

        # Parse database URL for connection parameters
        parsed = urlparse(database_url)
        self.host = parsed.hostname or "localhost"
        self.port = parsed.port or 5432
        self.database = parsed.path.lstrip("/") or "penfold_dev"
        self.user = parsed.username or "postgres"
        self.password = parsed.password or ""

        # Pool configuration
        pool_settings = self.settings.get_database_pool_settings()
        self.pool_min_size = pool_size or pool_settings["min_size"]
        self.pool_max_size = max_connections or pool_settings["max_size"]
        self.pool_max_queries = pool_settings["max_queries"]
        self.pool_max_inactive = pool_settings["max_inactive_connection_lifetime"]
        self.command_timeout = pool_settings["command_timeout"]

        # Performance monitoring
        self._query_metrics = {
            "total_queries": 0,
            "total_duration": 0.0,
            "slow_queries": 0,
            "failed_queries": 0,
        }
        self._slow_query_threshold = 1.0  # seconds

        # Batch operation settings
        self.batch_size = min(1000, self.settings.metrics_buffer_size)
        self.max_retry_attempts = 3
        self.retry_delay = 1.0  # seconds

        # Schema validation cache
        self._schema_validated = False
        self._hypertables_created = set()

        logger.info(
            f"TimescaleAdapter initialized for {self.host}:{self.port}/{self.database} "
            f"with pool size {self.pool_min_size}-{self.pool_max_size}"
        )

    async def initialize(self) -> None:
        """Initialize connection pool and database schema.

        Raises:
            ConnectionPoolError: If connection pool creation fails
            HypertableError: If hypertable creation fails
        """
        try:
            logger.info("Initializing TimescaleDB connection pool...")

            # Create connection pool
            self.pool = await asyncpg.create_pool(
                host=self.host,
                port=self.port,
                user=self.user,
                password=self.password,
                database=self.database,
                min_size=self.pool_min_size,
                max_size=self.pool_max_size,
                max_queries=self.pool_max_queries,
                max_inactive_connection_lifetime=self.pool_max_inactive,
                command_timeout=self.command_timeout,
                server_settings={
                    "application_name": "penfold_observability",
                    "timezone": "UTC",
                },
            )

            # Validate database schema
            await self._validate_schema()

            # Create hypertables if needed
            await self._ensure_hypertables()

            # Setup Row-Level Security
            await self._setup_tenant_isolation()

            # Create continuous aggregates if enabled
            if self.settings.enable_continuous_aggregates:
                await self._ensure_continuous_aggregates()

            logger.info("TimescaleDB adapter initialization complete")

        except Exception as e:
            logger.error(f"Failed to initialize TimescaleDB adapter: {e}")
            if self.pool:
                await self.pool.close()
                self.pool = None
            raise ConnectionPoolError(f"Adapter initialization failed: {e}")

    async def close(self) -> None:
        """Close connection pool and cleanup resources."""
        if self.pool:
            logger.info("Closing TimescaleDB connection pool...")
            await self.pool.close()
            self.pool = None

        # Log performance metrics
        if self._query_metrics["total_queries"] > 0:
            avg_duration = self._query_metrics["total_duration"] / self._query_metrics["total_queries"]
            logger.info(
                f"Query performance stats - "
                f"Total: {self._query_metrics['total_queries']}, "
                f"Avg duration: {avg_duration:.3f}s, "
                f"Slow queries: {self._query_metrics['slow_queries']}, "
                f"Failed queries: {self._query_metrics['failed_queries']}"
            )

    @asynccontextmanager
    async def get_connection(self):
        """Get database connection from pool with error handling.

        Yields:
            asyncpg.Connection: Database connection

        Raises:
            ConnectionPoolError: If connection cannot be acquired
        """
        if not self.pool:
            raise ConnectionPoolError("Connection pool not initialized")

        connection = None
        try:
            connection = await self.pool.acquire()
            yield connection
        except TooManyConnectionsError:
            logger.warning("Connection pool exhausted, waiting for available connection")
            await asyncio.sleep(0.1)
            connection = await self.pool.acquire()
            yield connection
        except Exception as e:
            logger.error(f"Database connection error: {e}")
            raise ConnectionPoolError(f"Connection acquisition failed: {e}")
        finally:
            if connection:
                await self.pool.release(connection)

    async def _execute_with_monitoring(
        self,
        query: str,
        *args,
        fetch: bool = False,
        tenant_id: Optional[str] = None,
    ) -> Optional[List[Any]]:
        """Execute query with performance monitoring and retry logic.

        Args:
            query: SQL query to execute
            *args: Query parameters
            fetch: Whether to fetch results
            tenant_id: Tenant context for RLS

        Returns:
            Query results if fetch=True, otherwise None

        Raises:
            QueryOptimizationError: If query consistently fails or is too slow
        """
        start_time = time.time()

        for attempt in range(self.max_retry_attempts):
            try:
                async with self.get_connection() as conn:
                    # Set tenant context for RLS if provided
                    if tenant_id:
                        await conn.execute(
                            "SELECT set_config('row_security.tenant_id', $1, true)",
                            tenant_id
                        )

                    if fetch:
                        result = await conn.fetch(query, *args)
                    else:
                        await conn.execute(query, *args)
                        result = None

                    # Update metrics
                    duration = time.time() - start_time
                    self._query_metrics["total_queries"] += 1
                    self._query_metrics["total_duration"] += duration

                    if duration > self._slow_query_threshold:
                        self._query_metrics["slow_queries"] += 1
                        logger.warning(
                            f"Slow query detected ({duration:.3f}s): {query[:100]}..."
                        )

                    return result

            except (ConnectionDoesNotExistError, PostgresError) as e:
                self._query_metrics["failed_queries"] += 1

                if attempt < self.max_retry_attempts - 1:
                    logger.warning(f"Query attempt {attempt + 1} failed, retrying: {e}")
                    await asyncio.sleep(self.retry_delay * (attempt + 1))
                else:
                    logger.error(f"Query failed after {self.max_retry_attempts} attempts: {e}")
                    raise QueryOptimizationError(f"Query execution failed: {e}")

    async def _validate_schema(self) -> None:
        """Validate database schema and TimescaleDB extension.

        Raises:
            HypertableError: If schema validation fails
        """
        try:
            # Check TimescaleDB extension
            result = await self._execute_with_monitoring(
                "SELECT COUNT(*) FROM pg_extension WHERE extname = 'timescaledb'",
                fetch=True
            )

            if not result or result[0][0] == 0:
                raise HypertableError("TimescaleDB extension not installed")

            # Verify observability tables exist
            tables_to_check = [
                "agents",
                "agent_metrics",
                "decision_traces",
                "workflow_events",
                "agent_logs",
                "alert_thresholds",
                "alert_events",
                "observability_tenants",
            ]

            for table in tables_to_check:
                result = await self._execute_with_monitoring(
                    """
                    SELECT COUNT(*) FROM information_schema.tables
                    WHERE table_name = $1 AND table_schema = 'public'
                    """,
                    table,
                    fetch=True
                )

                if not result or result[0][0] == 0:
                    raise HypertableError(f"Required table '{table}' not found")

            self._schema_validated = True
            logger.info("Database schema validation successful")

        except Exception as e:
            raise HypertableError(f"Schema validation failed: {e}")

    async def _ensure_hypertables(self) -> None:
        """Create TimescaleDB hypertables for time-series data.

        Raises:
            HypertableError: If hypertable creation fails
        """
        try:
            # Time-series tables to convert to hypertables
            time_series_tables = [
                ("agent_metrics", "timestamp"),
                ("decision_traces", "timestamp"),
                ("workflow_events", "timestamp"),
                ("agent_logs", "timestamp"),
                ("alert_events", "triggered_at"),
            ]

            for table_name, time_column in time_series_tables:
                if table_name in self._hypertables_created:
                    continue

                # Check if already a hypertable
                result = await self._execute_with_monitoring(
                    """
                    SELECT COUNT(*) FROM timescaledb_information.hypertables
                    WHERE hypertable_name = $1
                    """,
                    table_name,
                    fetch=True
                )

                if result and result[0][0] > 0:
                    logger.info(f"Hypertable {table_name} already exists")
                    self._hypertables_created.add(table_name)
                    continue

                # Create hypertable with optimized chunk time interval
                chunk_interval = "1 day" if table_name in ["agent_metrics", "agent_logs"] else "7 days"

                await self._execute_with_monitoring(
                    f"""
                    SELECT create_hypertable(
                        '{table_name}',
                        '{time_column}',
                        chunk_time_interval => interval '{chunk_interval}',
                        if_not_exists => true
                    )
                    """
                )

                # Set compression policy if enabled
                if self.settings.enable_compression:
                    await self._execute_with_monitoring(
                        f"""
                        SELECT add_compression_policy(
                            '{table_name}',
                            interval '{self.settings.compression_interval_days} days',
                            if_not_exists => true
                        )
                        """
                    )

                # Set retention policy
                await self._execute_with_monitoring(
                    f"""
                    SELECT add_retention_policy(
                        '{table_name}',
                        interval '{self.settings.retention_interval_days} days',
                        if_not_exists => true
                    )
                    """
                )

                self._hypertables_created.add(table_name)
                logger.info(f"Created hypertable {table_name} with {chunk_interval} chunks")

        except Exception as e:
            raise HypertableError(f"Hypertable creation failed: {e}")

    async def _setup_tenant_isolation(self) -> None:
        """Setup Row-Level Security for tenant isolation.

        Raises:
            TenantIsolationError: If RLS setup fails
        """
        try:
            # Tables requiring tenant isolation
            tenant_tables = [
                "agents",
                "agent_metrics",
                "decision_traces",
                "workflow_events",
                "agent_logs",
                "alert_thresholds",
                "alert_events",
                "observability_tenants",
            ]

            for table in tenant_tables:
                # Enable RLS
                await self._execute_with_monitoring(
                    f"ALTER TABLE {table} ENABLE ROW LEVEL SECURITY"
                )

                # Create policy for tenant isolation
                policy_name = f"{table}_tenant_isolation"

                # Drop existing policy if exists
                await self._execute_with_monitoring(
                    f"DROP POLICY IF EXISTS {policy_name} ON {table}"
                )

                # Create new policy
                if table == "observability_tenants":
                    # Special case for tenants table - users can only see their own tenant
                    await self._execute_with_monitoring(
                        f"""
                        CREATE POLICY {policy_name} ON {table}
                        USING (id::text = current_setting('row_security.tenant_id', true))
                        """
                    )
                else:
                    # Standard tenant isolation policy
                    await self._execute_with_monitoring(
                        f"""
                        CREATE POLICY {policy_name} ON {table}
                        USING (tenant_id = current_setting('row_security.tenant_id', true))
                        """
                    )

                logger.debug(f"Setup RLS policy for {table}")

            logger.info("Tenant isolation (RLS) setup complete")

        except Exception as e:
            raise TenantIsolationError(f"RLS setup failed: {e}")

    async def _ensure_continuous_aggregates(self) -> None:
        """Create continuous aggregates for dashboard performance.

        Raises:
            HypertableError: If continuous aggregate creation fails
        """
        try:
            # Hourly metrics aggregation
            await self._execute_with_monitoring(
                """
                CREATE MATERIALIZED VIEW IF NOT EXISTS agent_metrics_hourly
                WITH (timescaledb.continuous) AS
                SELECT
                    time_bucket('1 hour', timestamp) as hour,
                    agent_id,
                    metric_name,
                    tenant_id,
                    AVG(metric_value) as avg_value,
                    MIN(metric_value) as min_value,
                    MAX(metric_value) as max_value,
                    COUNT(*) as sample_count
                FROM agent_metrics
                GROUP BY hour, agent_id, metric_name, tenant_id
                """
            )

            # Daily metrics aggregation
            await self._execute_with_monitoring(
                """
                CREATE MATERIALIZED VIEW IF NOT EXISTS agent_metrics_daily
                WITH (timescaledb.continuous) AS
                SELECT
                    time_bucket('1 day', timestamp) as day,
                    agent_id,
                    metric_name,
                    tenant_id,
                    AVG(metric_value) as avg_value,
                    MIN(metric_value) as min_value,
                    MAX(metric_value) as max_value,
                    COUNT(*) as sample_count
                FROM agent_metrics
                GROUP BY day, agent_id, metric_name, tenant_id
                """
            )

            # Workflow performance aggregation
            await self._execute_with_monitoring(
                """
                CREATE MATERIALIZED VIEW IF NOT EXISTS workflow_performance_hourly
                WITH (timescaledb.continuous) AS
                SELECT
                    time_bucket('1 hour', timestamp) as hour,
                    workflow_id,
                    agent_id,
                    event_type,
                    event_status,
                    tenant_id,
                    AVG(processing_time_ms) as avg_processing_time,
                    MIN(processing_time_ms) as min_processing_time,
                    MAX(processing_time_ms) as max_processing_time,
                    COUNT(*) as event_count
                FROM workflow_events
                WHERE processing_time_ms IS NOT NULL
                GROUP BY hour, workflow_id, agent_id, event_type, event_status, tenant_id
                """
            )

            # Setup refresh policies
            refresh_interval = f"{self.settings.aggregate_refresh_interval_hours} hours"

            for view_name in ["agent_metrics_hourly", "agent_metrics_daily", "workflow_performance_hourly"]:
                await self._execute_with_monitoring(
                    f"""
                    SELECT add_continuous_aggregate_policy(
                        '{view_name}',
                        start_offset => interval '{refresh_interval}',
                        end_offset => interval '1 hour',
                        schedule_interval => interval '{refresh_interval}',
                        if_not_exists => true
                    )
                    """
                )

            logger.info(f"Continuous aggregates created with {refresh_interval} refresh")

        except Exception as e:
            raise HypertableError(f"Continuous aggregate creation failed: {e}")

    async def batch_insert_metrics(
        self,
        metrics: List[Dict[str, Any]],
        tenant_id: str,
    ) -> int:
        """Insert metrics in optimized batches.

        Args:
            metrics: List of metric dictionaries with required fields:
                - agent_id: str
                - timestamp: datetime
                - metric_name: str
                - metric_value: float
                - metric_type: str
                - labels: dict (optional)
            tenant_id: Tenant context for isolation

        Returns:
            Number of metrics successfully inserted

        Raises:
            BatchInsertError: If batch insert fails
        """
        if not metrics:
            return 0

        try:
            # Prepare batch data
            batch_data = []
            for metric in metrics:
                batch_data.append((
                    str(uuid.uuid4()),  # id
                    metric["agent_id"],
                    metric["timestamp"],
                    metric["metric_name"],
                    float(metric["metric_value"]),
                    metric["metric_type"],
                    metric.get("labels", {}),
                    tenant_id,
                    datetime.utcnow(),  # created_at
                    datetime.utcnow(),  # updated_at
                ))

            # Insert in batches to avoid memory issues
            inserted_count = 0
            batch_size = min(self.batch_size, 1000)

            for i in range(0, len(batch_data), batch_size):
                batch_chunk = batch_data[i:i + batch_size]

                await self._execute_with_monitoring(
                    """
                    INSERT INTO agent_metrics (
                        id, agent_id, timestamp, metric_name, metric_value,
                        metric_type, labels, tenant_id, created_at, updated_at
                    ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
                    """,
                    *zip(*batch_chunk),
                    tenant_id=tenant_id,
                )

                inserted_count += len(batch_chunk)

            logger.info(f"Inserted {inserted_count} metrics for tenant {tenant_id}")
            return inserted_count

        except Exception as e:
            logger.error(f"Batch insert failed: {e}")
            raise BatchInsertError(f"Failed to insert metrics: {e}")

    async def get_agent_metrics(
        self,
        agent_id: str,
        tenant_id: str,
        hours_back: int = 24,
        metric_names: Optional[List[str]] = None,
        aggregation: str = "raw",
    ) -> List[Dict[str, Any]]:
        """Get agent metrics for dashboard display.

        Args:
            agent_id: Agent identifier
            tenant_id: Tenant context
            hours_back: Hours of history to retrieve
            metric_names: Specific metrics to fetch (all if None)
            aggregation: "raw", "hourly", or "daily"

        Returns:
            List of metric records

        Raises:
            QueryOptimizationError: If query fails
        """
        try:
            # Choose table based on aggregation level
            if aggregation == "hourly":
                table = "agent_metrics_hourly"
                time_col = "hour"
            elif aggregation == "daily":
                table = "agent_metrics_daily"
                time_col = "day"
            else:
                table = "agent_metrics"
                time_col = "timestamp"

            # Build query
            time_filter = f"{time_col} >= NOW() - interval '{hours_back} hours'"

            if metric_names:
                metric_filter = f"AND metric_name = ANY($2)"
                args = [agent_id, metric_names]
            else:
                metric_filter = ""
                args = [agent_id]

            if aggregation in ["hourly", "daily"]:
                query = f"""
                SELECT
                    {time_col} as timestamp,
                    metric_name,
                    avg_value as metric_value,
                    min_value,
                    max_value,
                    sample_count,
                    'aggregate' as metric_type
                FROM {table}
                WHERE agent_id = $1 AND {time_filter} {metric_filter}
                ORDER BY {time_col} DESC, metric_name
                LIMIT 10000
                """
            else:
                query = f"""
                SELECT
                    timestamp,
                    metric_name,
                    metric_value,
                    metric_type,
                    labels
                FROM {table}
                WHERE agent_id = $1 AND {time_filter} {metric_filter}
                ORDER BY timestamp DESC, metric_name
                LIMIT 10000
                """

            result = await self._execute_with_monitoring(
                query, *args, fetch=True, tenant_id=tenant_id
            )

            # Convert to dictionaries
            metrics = []
            for row in result:
                if aggregation in ["hourly", "daily"]:
                    metrics.append({
                        "timestamp": row[0],
                        "metric_name": row[1],
                        "metric_value": row[2],
                        "min_value": row[3],
                        "max_value": row[4],
                        "sample_count": row[5],
                        "metric_type": row[6],
                    })
                else:
                    metrics.append({
                        "timestamp": row[0],
                        "metric_name": row[1],
                        "metric_value": row[2],
                        "metric_type": row[3],
                        "labels": row[4] if len(row) > 4 else {},
                    })

            logger.debug(f"Retrieved {len(metrics)} metrics for agent {agent_id}")
            return metrics

        except Exception as e:
            raise QueryOptimizationError(f"Failed to get agent metrics: {e}")

    async def get_workflow_performance(
        self,
        tenant_id: str,
        hours_back: int = 24,
        workflow_id: Optional[str] = None,
        agent_id: Optional[str] = None,
    ) -> List[Dict[str, Any]]:
        """Get workflow performance metrics for analysis.

        Args:
            tenant_id: Tenant context
            hours_back: Hours of history to retrieve
            workflow_id: Specific workflow (all if None)
            agent_id: Specific agent (all if None)

        Returns:
            List of workflow performance records
        """
        try:
            filters = ["timestamp >= NOW() - interval '%s hours'" % hours_back]
            args = []
            arg_count = 0

            if workflow_id:
                arg_count += 1
                filters.append(f"workflow_id = ${arg_count}")
                args.append(workflow_id)

            if agent_id:
                arg_count += 1
                filters.append(f"agent_id = ${arg_count}")
                args.append(agent_id)

            query = f"""
            SELECT
                workflow_id,
                agent_id,
                timestamp,
                event_type,
                event_status,
                processing_time_ms,
                event_data
            FROM workflow_events
            WHERE {' AND '.join(filters)}
            ORDER BY timestamp DESC, workflow_id
            LIMIT 5000
            """

            result = await self._execute_with_monitoring(
                query, *args, fetch=True, tenant_id=tenant_id
            )

            workflows = []
            for row in result:
                workflows.append({
                    "workflow_id": str(row[0]),
                    "agent_id": row[1],
                    "timestamp": row[2],
                    "event_type": row[3],
                    "event_status": row[4],
                    "processing_time_ms": row[5],
                    "event_data": row[6] or {},
                })

            return workflows

        except Exception as e:
            raise QueryOptimizationError(f"Failed to get workflow performance: {e}")

    async def get_alert_summary(
        self,
        tenant_id: str,
        active_only: bool = True,
    ) -> Dict[str, Any]:
        """Get alert summary for dashboard.

        Args:
            tenant_id: Tenant context
            active_only: Only return active alerts

        Returns:
            Alert summary statistics
        """
        try:
            status_filter = "AND alert_status = 'active'" if active_only else ""

            query = f"""
            SELECT
                COUNT(*) as total_alerts,
                COUNT(CASE WHEN alert_status = 'active' THEN 1 END) as active_alerts,
                COUNT(CASE WHEN alert_status = 'acknowledged' THEN 1 END) as acknowledged_alerts,
                AVG(EXTRACT(EPOCH FROM (COALESCE(resolved_at, NOW()) - triggered_at)) / 60) as avg_duration_minutes
            FROM alert_events ae
            JOIN alert_thresholds at ON ae.threshold_id = at.id
            WHERE ae.triggered_at >= NOW() - interval '24 hours' {status_filter}
            """

            result = await self._execute_with_monitoring(
                query, fetch=True, tenant_id=tenant_id
            )

            if result:
                row = result[0]
                return {
                    "total_alerts": row[0] or 0,
                    "active_alerts": row[1] or 0,
                    "acknowledged_alerts": row[2] or 0,
                    "avg_duration_minutes": float(row[3]) if row[3] else 0.0,
                }
            else:
                return {
                    "total_alerts": 0,
                    "active_alerts": 0,
                    "acknowledged_alerts": 0,
                    "avg_duration_minutes": 0.0,
                }

        except Exception as e:
            raise QueryOptimizationError(f"Failed to get alert summary: {e}")

    async def insert_decision_trace(
        self,
        trace_data: Dict[str, Any],
        tenant_id: str,
    ) -> str:
        """Insert decision trace record.

        Args:
            trace_data: Decision trace data
            tenant_id: Tenant context

        Returns:
            ID of inserted trace

        Raises:
            BatchInsertError: If insert fails
        """
        try:
            trace_id = str(uuid.uuid4())

            await self._execute_with_monitoring(
                """
                INSERT INTO decision_traces (
                    id, agent_id, timestamp, decision_type, decision_context,
                    alternatives_considered, confidence_score, reasoning,
                    workflow_id, tenant_id, created_at, updated_at
                ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
                """,
                trace_id,
                trace_data["agent_id"],
                trace_data.get("timestamp", datetime.utcnow()),
                trace_data["decision_type"],
                trace_data["decision_context"],
                trace_data["alternatives_considered"],
                float(trace_data["confidence_score"]),
                trace_data.get("reasoning"),
                trace_data.get("workflow_id"),
                tenant_id,
                datetime.utcnow(),
                datetime.utcnow(),
                tenant_id=tenant_id,
            )

            return trace_id

        except Exception as e:
            raise BatchInsertError(f"Failed to insert decision trace: {e}")

    async def log_agent_event(
        self,
        log_data: Dict[str, Any],
        tenant_id: str,
    ) -> str:
        """Insert agent log entry.

        Args:
            log_data: Log entry data
            tenant_id: Tenant context

        Returns:
            ID of inserted log entry
        """
        try:
            log_id = str(uuid.uuid4())

            await self._execute_with_monitoring(
                """
                INSERT INTO agent_logs (
                    id, timestamp, agent_id, level, event, context,
                    workflow_id, tenant_id, created_at, updated_at
                ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
                """,
                log_id,
                log_data.get("timestamp", datetime.utcnow()),
                log_data["agent_id"],
                log_data["level"],
                log_data["event"],
                log_data.get("context", {}),
                log_data.get("workflow_id"),
                tenant_id,
                datetime.utcnow(),
                datetime.utcnow(),
                tenant_id=tenant_id,
            )

            return log_id

        except Exception as e:
            raise BatchInsertError(f"Failed to insert log entry: {e}")

    async def check_health(self) -> Dict[str, Any]:
        """Check adapter and database health.

        Returns:
            Health status information
        """
        health = {
            "status": "healthy",
            "timestamp": datetime.utcnow().isoformat(),
            "pool_status": {},
            "query_metrics": self._query_metrics.copy(),
            "schema_validated": self._schema_validated,
            "hypertables_created": len(self._hypertables_created),
        }

        try:
            if self.pool:
                health["pool_status"] = {
                    "size": self.pool.get_size(),
                    "available": self.pool.get_idle_size(),
                    "max_size": self.pool_max_size,
                }

                # Test database connectivity
                start_time = time.time()
                await self._execute_with_monitoring("SELECT 1", fetch=True)
                health["query_latency_ms"] = (time.time() - start_time) * 1000
            else:
                health["status"] = "disconnected"
                health["error"] = "Connection pool not initialized"

        except Exception as e:
            health["status"] = "unhealthy"
            health["error"] = str(e)

        return health

    async def get_performance_stats(self) -> Dict[str, Any]:
        """Get detailed performance statistics.

        Returns:
            Performance metrics and statistics
        """
        stats = {
            "query_metrics": self._query_metrics.copy(),
            "pool_config": {
                "min_size": self.pool_min_size,
                "max_size": self.pool_max_size,
                "max_queries": self.pool_max_queries,
                "command_timeout": self.command_timeout,
            },
            "settings": {
                "batch_size": self.batch_size,
                "slow_query_threshold": self._slow_query_threshold,
                "max_retry_attempts": self.max_retry_attempts,
            },
        }

        if self.pool:
            stats["current_pool_status"] = {
                "size": self.pool.get_size(),
                "idle": self.pool.get_idle_size(),
                "busy": self.pool.get_size() - self.pool.get_idle_size(),
            }

        return stats


# Convenience functions for common operations

async def create_adapter(
    database_url: Optional[str] = None,
    settings: Optional[ObservabilitySettings] = None,
) -> TimescaleAdapter:
    """Create and initialize a TimescaleDB adapter.

    Args:
        database_url: Database URL (uses settings if None)
        settings: Configuration settings

    Returns:
        Initialized TimescaleAdapter instance
    """
    settings = settings or get_settings()
    database_url = database_url or settings.database_url

    adapter = TimescaleAdapter(database_url, settings)
    await adapter.initialize()

    return adapter


async def get_global_adapter() -> TimescaleAdapter:
    """Get or create global TimescaleDB adapter instance.

    Returns:
        Global TimescaleAdapter instance
    """
    global _global_adapter

    if not hasattr(get_global_adapter, '_global_adapter') or _global_adapter is None:
        _global_adapter = await create_adapter()

    return _global_adapter


# Global adapter instance (created on first use)
_global_adapter: Optional[TimescaleAdapter] = None