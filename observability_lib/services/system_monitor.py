#!/usr/bin/env python3
"""
System resource monitor for Penfold observability framework.

This service monitors system-level resources (CPU, memory, disk) with attribution
to specific agents, enabling resource usage tracking and capacity planning.
"""

import asyncio
import logging
import psutil
import json
from datetime import datetime, timedelta
from enum import Enum
from typing import Dict, List, Optional, Any, NamedTuple
from dataclasses import dataclass

import asyncpg
from observability_lib.config import ObservabilitySettings

logger = logging.getLogger(__name__)


class ResourceType(Enum):
    """System resource types."""
    CPU = "cpu"
    MEMORY = "memory"
    DISK = "disk"
    NETWORK = "network"
    PROCESS = "process"


@dataclass
class ResourceUsage:
    """System resource usage measurement."""
    resource_type: ResourceType
    agent_id: Optional[str]
    usage_value: float
    usage_percent: float
    timestamp: datetime
    tenant_id: Optional[str] = None
    metadata: Optional[Dict[str, Any]] = None


@dataclass
class SystemSnapshot:
    """Complete system resource snapshot."""
    timestamp: datetime
    cpu_usage_percent: float
    memory_usage_percent: float
    memory_usage_mb: float
    disk_usage_percent: float
    disk_usage_gb: float
    network_io_mb: float
    process_count: int
    agent_processes: List[Dict[str, Any]]
    tenant_id: Optional[str] = None


@dataclass
class ResourceAlert:
    """Resource usage alert."""
    resource_type: ResourceType
    agent_id: Optional[str]
    current_usage: float
    threshold: float
    severity: str
    message: str
    timestamp: datetime


class ProcessInfo(NamedTuple):
    """Process information tuple."""
    pid: int
    name: str
    agent_id: Optional[str]
    cpu_percent: float
    memory_mb: float
    status: str


class SystemMonitorService:
    """Service for monitoring system resources and agent attribution."""

    def __init__(self, settings: ObservabilitySettings):
        self.settings = settings
        self.db_pool: Optional[asyncpg.Pool] = None
        self._monitoring_task: Optional[asyncio.Task] = None
        self._agent_process_mapping: Dict[int, str] = {}  # pid -> agent_id
        self._resource_history: List[SystemSnapshot] = []
        self._alert_thresholds = {
            ResourceType.CPU: 80.0,
            ResourceType.MEMORY: 85.0,
            ResourceType.DISK: 90.0,
        }

    async def start(self):
        """Initialize system monitor service."""
        try:
            # Create database connection pool
            self.db_pool = await asyncpg.create_pool(
                self.settings.database_url,
                min_size=2,
                max_size=5,
                command_timeout=30
            )

            # Initialize process mapping
            await self._discover_agent_processes()

            # Start monitoring task
            self._monitoring_task = asyncio.create_task(self._monitor_loop())

            logger.info("System monitor service started")

        except Exception as e:
            logger.error(f"Failed to start system monitor: {e}")
            raise

    async def stop(self):
        """Stop system monitor service."""
        if self._monitoring_task:
            self._monitoring_task.cancel()
            try:
                await self._monitoring_task
            except asyncio.CancelledError:
                pass

        if self.db_pool:
            await self.db_pool.close()

        logger.info("System monitor service stopped")

    async def get_system_snapshot(self, tenant_id: Optional[str] = None) -> SystemSnapshot:
        """Get current system resource snapshot."""
        try:
            timestamp = datetime.utcnow()

            # Get CPU usage
            cpu_usage = psutil.cpu_percent(interval=1)

            # Get memory usage
            memory = psutil.virtual_memory()
            memory_usage_percent = memory.percent
            memory_usage_mb = memory.used / (1024 * 1024)

            # Get disk usage
            disk = psutil.disk_usage('/')
            disk_usage_percent = (disk.used / disk.total) * 100
            disk_usage_gb = disk.used / (1024 ** 3)

            # Get network I/O
            network = psutil.net_io_counters()
            network_io_mb = (network.bytes_sent + network.bytes_recv) / (1024 * 1024)

            # Get agent processes
            agent_processes = await self._get_agent_processes()
            process_count = len(psutil.pids())

            snapshot = SystemSnapshot(
                timestamp=timestamp,
                cpu_usage_percent=cpu_usage,
                memory_usage_percent=memory_usage_percent,
                memory_usage_mb=memory_usage_mb,
                disk_usage_percent=disk_usage_percent,
                disk_usage_gb=disk_usage_gb,
                network_io_mb=network_io_mb,
                process_count=process_count,
                agent_processes=agent_processes,
                tenant_id=tenant_id
            )

            return snapshot

        except Exception as e:
            logger.error(f"Failed to get system snapshot: {e}")
            return SystemSnapshot(
                timestamp=datetime.utcnow(),
                cpu_usage_percent=0.0,
                memory_usage_percent=0.0,
                memory_usage_mb=0.0,
                disk_usage_percent=0.0,
                disk_usage_gb=0.0,
                network_io_mb=0.0,
                process_count=0,
                agent_processes=[],
                tenant_id=tenant_id
            )

    async def get_agent_resource_usage(
        self,
        agent_id: str,
        hours_back: int = 1,
        tenant_id: Optional[str] = None
    ) -> List[ResourceUsage]:
        """Get resource usage history for specific agent."""
        try:
            async with self.db_pool.acquire() as conn:
                rows = await conn.fetch("""
                    SELECT resource_type, usage_value, usage_percent, timestamp, metadata
                    FROM system_resources
                    WHERE agent_id = $1
                    AND timestamp >= NOW() - INTERVAL '%s hours'
                    AND ($3::text IS NULL OR tenant_id = $3)
                    ORDER BY timestamp DESC
                    LIMIT 1000
                """ % hours_back, agent_id, tenant_id)

            usage_history = []
            for row in rows:
                usage = ResourceUsage(
                    resource_type=ResourceType(row['resource_type']),
                    agent_id=agent_id,
                    usage_value=row['usage_value'],
                    usage_percent=row['usage_percent'],
                    timestamp=row['timestamp'],
                    tenant_id=tenant_id,
                    metadata=json.loads(row['metadata']) if row['metadata'] else None
                )
                usage_history.append(usage)

            return usage_history

        except Exception as e:
            logger.error(f"Failed to get agent resource usage for {agent_id}: {e}")
            return []

    async def get_resource_top_consumers(
        self,
        resource_type: ResourceType,
        limit: int = 10,
        hours_back: int = 1,
        tenant_id: Optional[str] = None
    ) -> List[Dict[str, Any]]:
        """Get top resource consumers by agent."""
        try:
            async with self.db_pool.acquire() as conn:
                rows = await conn.fetch("""
                    SELECT agent_id, AVG(usage_percent) as avg_usage,
                           MAX(usage_percent) as max_usage,
                           COUNT(*) as sample_count
                    FROM system_resources
                    WHERE resource_type = $1
                    AND agent_id IS NOT NULL
                    AND timestamp >= NOW() - INTERVAL '%s hours'
                    AND ($3::text IS NULL OR tenant_id = $3)
                    GROUP BY agent_id
                    ORDER BY avg_usage DESC
                    LIMIT $2
                """ % hours_back, resource_type.value, limit, tenant_id)

            consumers = []
            for row in rows:
                consumer = {
                    "agent_id": row['agent_id'],
                    "average_usage_percent": float(row['avg_usage']),
                    "peak_usage_percent": float(row['max_usage']),
                    "measurement_count": row['sample_count'],
                    "resource_type": resource_type.value
                }
                consumers.append(consumer)

            return consumers

        except Exception as e:
            logger.error(f"Failed to get resource top consumers: {e}")
            return []

    async def get_system_resource_trends(
        self,
        hours_back: int = 24,
        tenant_id: Optional[str] = None
    ) -> Dict[str, Any]:
        """Get system-wide resource usage trends."""
        try:
            async with self.db_pool.acquire() as conn:
                # Get hourly aggregated system metrics
                rows = await conn.fetch("""
                    SELECT
                        DATE_TRUNC('hour', timestamp) as hour,
                        resource_type,
                        AVG(usage_percent) as avg_usage,
                        MAX(usage_percent) as max_usage,
                        MIN(usage_percent) as min_usage
                    FROM system_resources
                    WHERE agent_id IS NULL  -- System-level metrics
                    AND timestamp >= NOW() - INTERVAL '%s hours'
                    AND ($2::text IS NULL OR tenant_id = $2)
                    GROUP BY DATE_TRUNC('hour', timestamp), resource_type
                    ORDER BY hour, resource_type
                """ % hours_back, tenant_id)

            # Organize by resource type
            trends = {}
            for row in rows:
                resource_type = row['resource_type']
                if resource_type not in trends:
                    trends[resource_type] = {
                        "hourly_data": [],
                        "average_usage": 0.0,
                        "peak_usage": 0.0
                    }

                trends[resource_type]["hourly_data"].append({
                    "hour": row['hour'],
                    "avg_usage": float(row['avg_usage']),
                    "max_usage": float(row['max_usage']),
                    "min_usage": float(row['min_usage'])
                })

            # Calculate overall statistics
            for resource_type, data in trends.items():
                if data["hourly_data"]:
                    data["average_usage"] = sum(h["avg_usage"] for h in data["hourly_data"]) / len(data["hourly_data"])
                    data["peak_usage"] = max(h["max_usage"] for h in data["hourly_data"])

            return trends

        except Exception as e:
            logger.error(f"Failed to get system resource trends: {e}")
            return {}

    async def record_resource_usage(self, usage: ResourceUsage):
        """Record resource usage measurement."""
        try:
            async with self.db_pool.acquire() as conn:
                await conn.execute("""
                    INSERT INTO system_resources (
                        resource_type, agent_id, usage_value, usage_percent,
                        timestamp, tenant_id, metadata
                    ) VALUES ($1, $2, $3, $4, $5, $6, $7)
                """,
                usage.resource_type.value,
                usage.agent_id,
                usage.usage_value,
                usage.usage_percent,
                usage.timestamp,
                usage.tenant_id,
                json.dumps(usage.metadata) if usage.metadata else None
                )

        except Exception as e:
            logger.error(f"Failed to record resource usage: {e}")

    async def check_resource_alerts(self) -> List[ResourceAlert]:
        """Check for resource usage alerts."""
        alerts = []

        try:
            snapshot = await self.get_system_snapshot()

            # Check system-level thresholds
            if snapshot.cpu_usage_percent > self._alert_thresholds[ResourceType.CPU]:
                alert = ResourceAlert(
                    resource_type=ResourceType.CPU,
                    agent_id=None,
                    current_usage=snapshot.cpu_usage_percent,
                    threshold=self._alert_thresholds[ResourceType.CPU],
                    severity="HIGH",
                    message=f"System CPU usage at {snapshot.cpu_usage_percent:.1f}%",
                    timestamp=snapshot.timestamp
                )
                alerts.append(alert)

            if snapshot.memory_usage_percent > self._alert_thresholds[ResourceType.MEMORY]:
                alert = ResourceAlert(
                    resource_type=ResourceType.MEMORY,
                    agent_id=None,
                    current_usage=snapshot.memory_usage_percent,
                    threshold=self._alert_thresholds[ResourceType.MEMORY],
                    severity="HIGH",
                    message=f"System memory usage at {snapshot.memory_usage_percent:.1f}%",
                    timestamp=snapshot.timestamp
                )
                alerts.append(alert)

            if snapshot.disk_usage_percent > self._alert_thresholds[ResourceType.DISK]:
                alert = ResourceAlert(
                    resource_type=ResourceType.DISK,
                    agent_id=None,
                    current_usage=snapshot.disk_usage_percent,
                    threshold=self._alert_thresholds[ResourceType.DISK],
                    severity="CRITICAL",
                    message=f"System disk usage at {snapshot.disk_usage_percent:.1f}%",
                    timestamp=snapshot.timestamp
                )
                alerts.append(alert)

            # Check agent-specific high usage
            for process in snapshot.agent_processes:
                if process.get('cpu_percent', 0) > 50:  # Agent using >50% CPU
                    alert = ResourceAlert(
                        resource_type=ResourceType.CPU,
                        agent_id=process.get('agent_id'),
                        current_usage=process['cpu_percent'],
                        threshold=50.0,
                        severity="MEDIUM",
                        message=f"Agent {process.get('agent_id')} using {process['cpu_percent']:.1f}% CPU",
                        timestamp=snapshot.timestamp
                    )
                    alerts.append(alert)

                if process.get('memory_mb', 0) > 1024:  # Agent using >1GB memory
                    alert = ResourceAlert(
                        resource_type=ResourceType.MEMORY,
                        agent_id=process.get('agent_id'),
                        current_usage=process['memory_mb'],
                        threshold=1024.0,
                        severity="MEDIUM",
                        message=f"Agent {process.get('agent_id')} using {process['memory_mb']:.1f}MB memory",
                        timestamp=snapshot.timestamp
                    )
                    alerts.append(alert)

        except Exception as e:
            logger.error(f"Failed to check resource alerts: {e}")

        return alerts

    async def _discover_agent_processes(self):
        """Discover running agent processes and map to agent IDs."""
        try:
            self._agent_process_mapping.clear()

            for process in psutil.process_iter(['pid', 'name', 'cmdline']):
                try:
                    info = process.info
                    cmdline = info.get('cmdline', [])

                    # Look for agent identifiers in command line
                    agent_id = self._extract_agent_id_from_cmdline(cmdline)
                    if agent_id:
                        self._agent_process_mapping[info['pid']] = agent_id

                except (psutil.NoSuchProcess, psutil.AccessDenied):
                    continue

            logger.info(f"Discovered {len(self._agent_process_mapping)} agent processes")

        except Exception as e:
            logger.error(f"Failed to discover agent processes: {e}")

    def _extract_agent_id_from_cmdline(self, cmdline: List[str]) -> Optional[str]:
        """Extract agent ID from process command line."""
        if not cmdline:
            return None

        cmdline_str = " ".join(cmdline).lower()

        # Look for common patterns
        patterns = [
            "agent_id=",
            "--agent-id=",
            "--agent=",
            "email_processor",
            "document_processor",
            "meeting_processor",
            "slack_processor",
            "search_agent",
            "context_agent"
        ]

        for pattern in patterns:
            if pattern in cmdline_str:
                if "=" in pattern:
                    # Extract value after equals
                    parts = cmdline_str.split(pattern)
                    if len(parts) > 1:
                        value = parts[1].split()[0].strip()
                        if value:
                            return value
                else:
                    # Pattern is the agent type
                    return pattern

        # Check if this is a Python script with agent in filename
        for arg in cmdline:
            if arg.endswith('.py') and 'agent' in arg.lower():
                return arg.split('/')[-1].replace('.py', '')

        return None

    async def _get_agent_processes(self) -> List[Dict[str, Any]]:
        """Get current agent process information."""
        agent_processes = []

        try:
            for process in psutil.process_iter(['pid', 'name', 'cpu_percent', 'memory_info', 'status']):
                try:
                    info = process.info
                    pid = info['pid']

                    # Get agent ID if mapped
                    agent_id = self._agent_process_mapping.get(pid)

                    # Calculate memory usage in MB
                    memory_info = info.get('memory_info')
                    memory_mb = memory_info.rss / (1024 * 1024) if memory_info else 0

                    process_info = {
                        "pid": pid,
                        "name": info.get('name', 'unknown'),
                        "agent_id": agent_id,
                        "cpu_percent": info.get('cpu_percent', 0.0),
                        "memory_mb": memory_mb,
                        "status": info.get('status', 'unknown')
                    }

                    # Only include processes that are agents or consuming significant resources
                    if agent_id or process_info['cpu_percent'] > 5 or memory_mb > 100:
                        agent_processes.append(process_info)

                except (psutil.NoSuchProcess, psutil.AccessDenied):
                    continue

        except Exception as e:
            logger.error(f"Failed to get agent processes: {e}")

        return agent_processes

    async def _monitor_loop(self):
        """Main monitoring loop for resource collection."""
        try:
            while True:
                # Get system snapshot
                snapshot = await self.get_system_snapshot()

                # Record system-level metrics
                timestamp = snapshot.timestamp

                system_metrics = [
                    ResourceUsage(
                        resource_type=ResourceType.CPU,
                        agent_id=None,
                        usage_value=snapshot.cpu_usage_percent,
                        usage_percent=snapshot.cpu_usage_percent,
                        timestamp=timestamp
                    ),
                    ResourceUsage(
                        resource_type=ResourceType.MEMORY,
                        agent_id=None,
                        usage_value=snapshot.memory_usage_mb,
                        usage_percent=snapshot.memory_usage_percent,
                        timestamp=timestamp
                    ),
                    ResourceUsage(
                        resource_type=ResourceType.DISK,
                        agent_id=None,
                        usage_value=snapshot.disk_usage_gb,
                        usage_percent=snapshot.disk_usage_percent,
                        timestamp=timestamp
                    )
                ]

                for metric in system_metrics:
                    await self.record_resource_usage(metric)

                # Record agent-specific metrics
                for process in snapshot.agent_processes:
                    if process.get('agent_id'):
                        agent_cpu = ResourceUsage(
                            resource_type=ResourceType.CPU,
                            agent_id=process['agent_id'],
                            usage_value=process['cpu_percent'],
                            usage_percent=process['cpu_percent'],
                            timestamp=timestamp,
                            metadata={"pid": process['pid'], "name": process['name']}
                        )
                        await self.record_resource_usage(agent_cpu)

                        agent_memory = ResourceUsage(
                            resource_type=ResourceType.MEMORY,
                            agent_id=process['agent_id'],
                            usage_value=process['memory_mb'],
                            usage_percent=(process['memory_mb'] / (snapshot.memory_usage_mb or 1)) * 100,
                            timestamp=timestamp,
                            metadata={"pid": process['pid'], "name": process['name']}
                        )
                        await self.record_resource_usage(agent_memory)

                # Update process mapping periodically
                if len(self._resource_history) % 10 == 0:  # Every 10 snapshots
                    await self._discover_agent_processes()

                # Keep history for local analysis
                self._resource_history.append(snapshot)
                if len(self._resource_history) > 1440:  # Keep 24 hours (1 minute intervals)
                    self._resource_history.pop(0)

                await asyncio.sleep(60)  # Monitor every minute

        except asyncio.CancelledError:
            logger.info("System monitoring loop cancelled")
        except Exception as e:
            logger.error(f"Error in system monitoring loop: {e}")