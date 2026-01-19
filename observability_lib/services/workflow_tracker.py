"""Comprehensive cross-agent workflow tracking service for Penfold observability.

This module implements WorkflowTrackerInterface providing comprehensive workflow
execution tracking across multiple autonomous agents. It supports nested workflows,
parallel execution tracking, correlation analysis, and workflow debugging with
sub-500ms query performance targets.

Key Features:
    - Cross-agent workflow correlation and tracking
    - Nested workflow support with parent-child relationships
    - Parallel execution tracking and coordination
    - Workflow lifecycle management (start, progress, complete, fail)
    - Failure analysis with error attribution
    - High-performance queries for debugging (<500ms target)
    - Tenant isolation and proper async operations
    - Comprehensive logging and metrics integration

Performance Targets:
    - Workflow event logging: <50ms
    - Workflow queries: <500ms for complex scenarios
    - Concurrent workflow support: 50+ simultaneous workflows
    - Event throughput: 1000+ events/second

Example Usage:
    from observability_lib.services.workflow_tracker import WorkflowTracker
    from observability_lib.storage.timeseries_adapter import create_adapter

    adapter = await create_adapter()
    tracker = WorkflowTracker(adapter, tenant_id="work")

    # Create workflow and track events
    workflow_id = await tracker.create_workflow_id()

    await tracker.log_workflow_event(WorkflowEventData(
        workflow_id=workflow_id,
        agent_id="email_processor",
        event_type=EventType.WORKFLOW_STARTED,
        event_status=EventStatus.SUCCESS,
        event_data={"workflow_type": "nightly_batch"}
    ))

    # Query workflow events for debugging
    events = await tracker.get_workflow_events(workflow_id)
"""

import asyncio
import logging
import time
import uuid
from datetime import datetime, timedelta
from typing import Any, Dict, List, Optional, Set, Tuple, Union
from collections import defaultdict, deque
from dataclasses import dataclass, asdict
from contextlib import asynccontextmanager

from ..contracts.instrumentation_interface import (
    WorkflowTrackerInterface,
    WorkflowEventData,
    EventType,
    EventStatus,
)
from ..models.agent_metrics import (
    WorkflowEvent,
    EventType as ModelEventType,
    EventStatus as ModelEventStatus,
)
from ..storage.timeseries_adapter import TimescaleAdapter, TimescaleError
from ..config import get_settings

logger = logging.getLogger(__name__)


class WorkflowTrackingError(Exception):
    """Base exception for workflow tracking errors."""
    pass


class WorkflowCorrelationError(WorkflowTrackingError):
    """Exception for workflow correlation issues."""
    pass


class WorkflowQueryError(WorkflowTrackingError):
    """Exception for workflow query failures."""
    pass


class WorkflowValidationError(WorkflowTrackingError):
    """Exception for workflow validation failures."""
    pass


@dataclass
class WorkflowState:
    """Internal state tracking for active workflows."""
    workflow_id: uuid.UUID
    parent_workflow_id: Optional[uuid.UUID]
    agent_id: str
    workflow_type: str
    started_at: datetime
    last_activity: datetime
    status: EventStatus
    child_workflows: Set[uuid.UUID]
    event_count: int
    total_processing_time_ms: float
    error_count: int
    current_stage: Optional[str] = None

    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary for JSON serialization."""
        return {
            "workflow_id": str(self.workflow_id),
            "parent_workflow_id": str(self.parent_workflow_id) if self.parent_workflow_id else None,
            "agent_id": self.agent_id,
            "workflow_type": self.workflow_type,
            "started_at": self.started_at.isoformat(),
            "last_activity": self.last_activity.isoformat(),
            "status": self.status.value if hasattr(self.status, 'value') else str(self.status),
            "child_workflows": [str(wf_id) for wf_id in self.child_workflows],
            "event_count": self.event_count,
            "total_processing_time_ms": self.total_processing_time_ms,
            "error_count": self.error_count,
            "current_stage": self.current_stage,
        }


@dataclass
class WorkflowAnalytics:
    """Analytics data for workflow performance analysis."""
    workflow_id: uuid.UUID
    total_duration_ms: float
    stage_count: int
    agent_count: int
    success_rate: float
    error_types: List[str]
    bottleneck_stages: List[Dict[str, Any]]
    parallel_efficiency: float

    def to_dict(self) -> Dict[str, Any]:
        """Convert to dictionary for JSON serialization."""
        return {
            "workflow_id": str(self.workflow_id),
            "total_duration_ms": self.total_duration_ms,
            "stage_count": self.stage_count,
            "agent_count": self.agent_count,
            "success_rate": self.success_rate,
            "error_types": self.error_types,
            "bottleneck_stages": self.bottleneck_stages,
            "parallel_efficiency": self.parallel_efficiency,
        }


class WorkflowTracker(WorkflowTrackerInterface):
    """Comprehensive workflow tracking service with cross-agent correlation.

    This implementation provides high-performance workflow tracking across multiple
    autonomous agents with support for nested workflows, parallel execution,
    failure analysis, and comprehensive debugging capabilities.

    The service maintains in-memory workflow state for active workflows while
    persisting all events to TimescaleDB for historical analysis and debugging.
    """

    def __init__(
        self,
        storage_adapter: TimescaleAdapter,
        tenant_id: str,
        max_active_workflows: int = 1000,
        workflow_timeout_hours: int = 24,
        cleanup_interval_minutes: int = 30,
        enable_analytics: bool = True,
    ):
        """Initialize workflow tracker with storage and configuration.

        Args:
            storage_adapter: TimescaleDB adapter for persistence
            tenant_id: Tenant context for isolation
            max_active_workflows: Maximum concurrent workflows to track in memory
            workflow_timeout_hours: Hours after which inactive workflows are cleaned up
            cleanup_interval_minutes: Interval for cleanup operations
            enable_analytics: Whether to compute workflow analytics

        Raises:
            WorkflowTrackingError: If initialization fails
        """
        if not storage_adapter:
            raise WorkflowTrackingError("Storage adapter is required")

        if not tenant_id or not isinstance(tenant_id, str):
            raise WorkflowTrackingError("Valid tenant_id is required")

        self.storage = storage_adapter
        self.tenant_id = tenant_id
        self.settings = get_settings()

        # Configuration
        self.max_active_workflows = max_active_workflows
        self.workflow_timeout = timedelta(hours=workflow_timeout_hours)
        self.cleanup_interval = timedelta(minutes=cleanup_interval_minutes)
        self.enable_analytics = enable_analytics

        # In-memory state for active workflows
        self._active_workflows: Dict[uuid.UUID, WorkflowState] = {}
        self._workflow_hierarchy: Dict[uuid.UUID, Set[uuid.UUID]] = defaultdict(set)  # parent -> children
        self._agent_workflows: Dict[str, Set[uuid.UUID]] = defaultdict(set)  # agent -> workflows

        # Performance monitoring
        self._event_buffer: deque = deque(maxlen=100)  # Recent events for debugging
        self._performance_metrics = {
            "events_logged": 0,
            "workflows_created": 0,
            "workflows_completed": 0,
            "workflows_failed": 0,
            "query_count": 0,
            "avg_query_time_ms": 0.0,
            "last_cleanup": datetime.utcnow(),
        }

        # Concurrency control
        self._state_lock = asyncio.RLock()
        self._cleanup_task: Optional[asyncio.Task] = None

        # Query optimization caches
        self._query_cache: Dict[str, Tuple[List[Dict[str, Any]], datetime]] = {}
        self._cache_ttl = timedelta(minutes=5)

        logger.info(
            f"WorkflowTracker initialized for tenant '{tenant_id}' with "
            f"max_workflows={max_active_workflows}, timeout={workflow_timeout_hours}h"
        )

    async def initialize(self) -> None:
        """Initialize the workflow tracker and start background tasks.

        Raises:
            WorkflowTrackingError: If initialization fails
        """
        try:
            # Start cleanup task
            self._cleanup_task = asyncio.create_task(self._periodic_cleanup())

            # Load active workflows from storage (crash recovery)
            await self._recover_active_workflows()

            logger.info(f"WorkflowTracker initialized with {len(self._active_workflows)} active workflows")

        except Exception as e:
            logger.error(f"Failed to initialize WorkflowTracker: {e}")
            raise WorkflowTrackingError(f"Initialization failed: {e}")

    async def shutdown(self) -> None:
        """Shutdown the workflow tracker and cleanup resources."""
        try:
            # Cancel cleanup task
            if self._cleanup_task:
                self._cleanup_task.cancel()
                try:
                    await self._cleanup_task
                except asyncio.CancelledError:
                    pass

            # Persist final state
            await self._persist_active_workflows()

            # Clear in-memory state
            self._active_workflows.clear()
            self._workflow_hierarchy.clear()
            self._agent_workflows.clear()
            self._query_cache.clear()

            logger.info("WorkflowTracker shutdown complete")

        except Exception as e:
            logger.error(f"Error during WorkflowTracker shutdown: {e}")

    async def create_workflow_id(self, parent_workflow_id: Optional[uuid.UUID] = None) -> uuid.UUID:
        """Generate a new workflow ID with optional parent relationship.

        Args:
            parent_workflow_id: Optional parent workflow for nested workflows

        Returns:
            New workflow UUID

        Raises:
            WorkflowTrackingError: If workflow creation fails
        """
        try:
            workflow_id = uuid.uuid4()

            async with self._state_lock:
                # Validate parent workflow exists if specified
                if parent_workflow_id and parent_workflow_id not in self._active_workflows:
                    # Check if parent exists in storage (may be completed)
                    parent_exists = await self._check_workflow_exists(parent_workflow_id)
                    if not parent_exists:
                        raise WorkflowValidationError(f"Parent workflow {parent_workflow_id} not found")

                # Track hierarchy relationship
                if parent_workflow_id:
                    self._workflow_hierarchy[parent_workflow_id].add(workflow_id)

                    # Update parent's child list if still active
                    if parent_workflow_id in self._active_workflows:
                        self._active_workflows[parent_workflow_id].child_workflows.add(workflow_id)

                # Update metrics
                self._performance_metrics["workflows_created"] += 1

                logger.debug(
                    f"Created workflow {workflow_id} with parent {parent_workflow_id} for tenant {self.tenant_id}"
                )

                return workflow_id

        except Exception as e:
            logger.error(f"Failed to create workflow ID: {e}")
            raise WorkflowTrackingError(f"Workflow creation failed: {e}")

    async def log_workflow_event(self, event: WorkflowEventData) -> None:
        """Log an event in a workflow execution with comprehensive tracking.

        Args:
            event: Workflow event data to log

        Raises:
            WorkflowTrackingError: If event logging fails
        """
        start_time = time.time()

        try:
            # Validate event data
            self._validate_event_data(event)

            async with self._state_lock:
                # Update in-memory state
                await self._update_workflow_state(event)

                # Add to event buffer for debugging
                self._event_buffer.append({
                    "timestamp": datetime.utcnow().isoformat(),
                    "workflow_id": str(event.workflow_id),
                    "agent_id": event.agent_id,
                    "event_type": event.event_type.value,
                    "event_status": event.event_status.value,
                    "processing_time_ms": event.processing_time_ms,
                })

                # Persist to storage
                await self._persist_workflow_event(event)

                # Update performance metrics
                duration_ms = (time.time() - start_time) * 1000
                self._performance_metrics["events_logged"] += 1

                logger.debug(
                    f"Logged {event.event_type.value} event for workflow {event.workflow_id} "
                    f"by agent {event.agent_id} in {duration_ms:.2f}ms"
                )

        except Exception as e:
            logger.error(f"Failed to log workflow event: {e}")
            raise WorkflowTrackingError(f"Event logging failed: {e}")

    async def get_workflow_events(
        self,
        workflow_id: uuid.UUID,
        include_children: bool = False,
        event_type: Optional[EventType] = None,
        limit: Optional[int] = None,
    ) -> List[Dict[str, Any]]:
        """Get all events for a workflow with optimized querying.

        Args:
            workflow_id: Workflow to get events for
            include_children: Whether to include child workflow events
            event_type: Filter by specific event type
            limit: Maximum number of events to return

        Returns:
            List of workflow events ordered by timestamp

        Raises:
            WorkflowQueryError: If query fails
        """
        query_start = time.time()

        try:
            # Check cache first
            cache_key = f"events_{workflow_id}_{include_children}_{event_type}_{limit}"
            cached_result = self._get_cached_result(cache_key)
            if cached_result:
                logger.debug(f"Returned cached events for workflow {workflow_id}")
                return cached_result

            # Build workflow ID list
            workflow_ids = [workflow_id]
            if include_children:
                workflow_ids.extend(await self._get_child_workflow_ids(workflow_id))

            # Query storage with optimization
            events = await self._query_workflow_events(
                workflow_ids=workflow_ids,
                event_type=event_type,
                limit=limit or 10000
            )

            # Cache result
            self._cache_result(cache_key, events)

            # Update performance metrics
            query_time_ms = (time.time() - query_start) * 1000
            self._update_query_metrics(query_time_ms)

            logger.debug(
                f"Retrieved {len(events)} events for workflow {workflow_id} in {query_time_ms:.2f}ms"
            )

            return events

        except Exception as e:
            logger.error(f"Failed to get workflow events: {e}")
            raise WorkflowQueryError(f"Event query failed: {e}")

    async def get_workflow_status(self, workflow_id: uuid.UUID) -> Dict[str, Any]:
        """Get comprehensive status information for a workflow.

        Args:
            workflow_id: Workflow to get status for

        Returns:
            Comprehensive workflow status information

        Raises:
            WorkflowQueryError: If status query fails
        """
        try:
            async with self._state_lock:
                # Check if workflow is active in memory
                if workflow_id in self._active_workflows:
                    workflow_state = self._active_workflows[workflow_id]

                    return {
                        "workflow_id": str(workflow_id),
                        "status": "active",
                        "state": workflow_state.to_dict(),
                        "children": [str(child_id) for child_id in workflow_state.child_workflows],
                        "parent": str(workflow_state.parent_workflow_id) if workflow_state.parent_workflow_id else None,
                        "analytics": await self._compute_workflow_analytics(workflow_id) if self.enable_analytics else None,
                    }

                # Query storage for completed workflow
                events = await self.get_workflow_events(workflow_id, include_children=False)
                if not events:
                    return {
                        "workflow_id": str(workflow_id),
                        "status": "not_found",
                        "error": "Workflow not found",
                    }

                # Analyze events to determine status
                status = self._analyze_workflow_status(events)

                return {
                    "workflow_id": str(workflow_id),
                    "status": "completed",
                    "summary": status,
                    "event_count": len(events),
                    "analytics": await self._compute_workflow_analytics(workflow_id) if self.enable_analytics else None,
                }

        except Exception as e:
            logger.error(f"Failed to get workflow status: {e}")
            raise WorkflowQueryError(f"Status query failed: {e}")

    async def get_agent_workflows(
        self,
        agent_id: str,
        active_only: bool = True,
        hours_back: int = 24,
    ) -> List[Dict[str, Any]]:
        """Get workflows associated with a specific agent.

        Args:
            agent_id: Agent identifier
            active_only: Only return active workflows
            hours_back: Hours of history to include

        Returns:
            List of workflows for the agent

        Raises:
            WorkflowQueryError: If query fails
        """
        try:
            workflows = []

            async with self._state_lock:
                # Add active workflows
                if active_only:
                    agent_workflow_ids = self._agent_workflows.get(agent_id, set())
                    for workflow_id in agent_workflow_ids:
                        if workflow_id in self._active_workflows:
                            state = self._active_workflows[workflow_id]
                            workflows.append({
                                "workflow_id": str(workflow_id),
                                "status": "active",
                                "started_at": state.started_at.isoformat(),
                                "workflow_type": state.workflow_type,
                                "event_count": state.event_count,
                                "error_count": state.error_count,
                            })

            # Query storage for historical workflows if needed
            if not active_only:
                since_time = datetime.utcnow() - timedelta(hours=hours_back)
                historical_workflows = await self._query_agent_workflows(agent_id, since_time)
                workflows.extend(historical_workflows)

            # Sort by start time descending
            workflows.sort(key=lambda w: w.get("started_at", ""), reverse=True)

            logger.debug(f"Retrieved {len(workflows)} workflows for agent {agent_id}")
            return workflows

        except Exception as e:
            logger.error(f"Failed to get agent workflows: {e}")
            raise WorkflowQueryError(f"Agent workflows query failed: {e}")

    async def analyze_workflow_performance(
        self,
        workflow_id: uuid.UUID,
        include_children: bool = True,
    ) -> Dict[str, Any]:
        """Perform comprehensive workflow performance analysis.

        Args:
            workflow_id: Workflow to analyze
            include_children: Whether to include child workflow analysis

        Returns:
            Detailed performance analysis

        Raises:
            WorkflowQueryError: If analysis fails
        """
        try:
            # Get all events
            events = await self.get_workflow_events(workflow_id, include_children=include_children)
            if not events:
                return {"error": "No events found for workflow"}

            # Perform analysis
            analysis = self._analyze_workflow_performance(events)

            # Add bottleneck analysis
            bottlenecks = self._identify_bottlenecks(events)
            analysis["bottlenecks"] = bottlenecks

            # Add parallel execution analysis if child workflows exist
            if include_children:
                parallel_analysis = self._analyze_parallel_execution(events)
                analysis["parallel_execution"] = parallel_analysis

            # Add failure analysis if errors occurred
            failures = [event for event in events if event.get("event_status") == "failure"]
            if failures:
                failure_analysis = self._analyze_failures(failures)
                analysis["failure_analysis"] = failure_analysis

            logger.debug(f"Completed performance analysis for workflow {workflow_id}")
            return analysis

        except Exception as e:
            logger.error(f"Failed to analyze workflow performance: {e}")
            raise WorkflowQueryError(f"Performance analysis failed: {e}")

    async def get_workflow_metrics(
        self,
        hours_back: int = 24,
        group_by: str = "agent",
    ) -> Dict[str, Any]:
        """Get aggregated workflow metrics for monitoring.

        Args:
            hours_back: Hours of history to analyze
            group_by: Grouping dimension ("agent", "type", "status")

        Returns:
            Aggregated workflow metrics

        Raises:
            WorkflowQueryError: If metrics query fails
        """
        try:
            since_time = datetime.utcnow() - timedelta(hours=hours_back)

            # Query aggregated metrics from storage
            metrics = await self._query_workflow_metrics(since_time, group_by)

            # Add current active workflow stats
            active_stats = self._get_active_workflow_stats()
            metrics["active_workflows"] = active_stats

            # Add performance metrics
            metrics["performance"] = self._performance_metrics.copy()

            return metrics

        except Exception as e:
            logger.error(f"Failed to get workflow metrics: {e}")
            raise WorkflowQueryError(f"Metrics query failed: {e}")

    async def cleanup_expired_workflows(self) -> int:
        """Clean up expired workflows from memory and perform maintenance.

        Returns:
            Number of workflows cleaned up

        Raises:
            WorkflowTrackingError: If cleanup fails
        """
        try:
            cleaned_count = 0
            cutoff_time = datetime.utcnow() - self.workflow_timeout

            async with self._state_lock:
                workflows_to_remove = []

                for workflow_id, state in self._active_workflows.items():
                    if state.last_activity < cutoff_time:
                        workflows_to_remove.append(workflow_id)

                for workflow_id in workflows_to_remove:
                    # Remove from all tracking structures
                    state = self._active_workflows.pop(workflow_id)

                    # Remove from agent tracking
                    agent_workflows = self._agent_workflows.get(state.agent_id, set())
                    agent_workflows.discard(workflow_id)
                    if not agent_workflows:
                        del self._agent_workflows[state.agent_id]

                    # Remove from hierarchy
                    if state.parent_workflow_id in self._workflow_hierarchy:
                        self._workflow_hierarchy[state.parent_workflow_id].discard(workflow_id)

                    if workflow_id in self._workflow_hierarchy:
                        del self._workflow_hierarchy[workflow_id]

                    cleaned_count += 1
                    logger.debug(f"Cleaned up expired workflow {workflow_id}")

            # Clear expired cache entries
            self._cleanup_cache()

            # Update cleanup timestamp
            self._performance_metrics["last_cleanup"] = datetime.utcnow()

            if cleaned_count > 0:
                logger.info(f"Cleaned up {cleaned_count} expired workflows")

            return cleaned_count

        except Exception as e:
            logger.error(f"Failed to cleanup expired workflows: {e}")
            raise WorkflowTrackingError(f"Cleanup failed: {e}")

    # Private helper methods

    def _validate_event_data(self, event: WorkflowEventData) -> None:
        """Validate workflow event data."""
        if not isinstance(event.workflow_id, uuid.UUID):
            raise WorkflowValidationError("workflow_id must be a UUID")

        if not event.agent_id or not isinstance(event.agent_id, str):
            raise WorkflowValidationError("agent_id is required and must be a string")

        if not isinstance(event.event_type, EventType):
            raise WorkflowValidationError("event_type must be an EventType enum")

        if not isinstance(event.event_status, EventStatus):
            raise WorkflowValidationError("event_status must be an EventStatus enum")

        if event.processing_time_ms is not None and event.processing_time_ms < 0:
            raise WorkflowValidationError("processing_time_ms cannot be negative")

    async def _update_workflow_state(self, event: WorkflowEventData) -> None:
        """Update in-memory workflow state based on event."""
        workflow_id = event.workflow_id
        now = datetime.utcnow()

        # Get or create workflow state
        if workflow_id not in self._active_workflows:
            # Check if this is a new workflow start event
            if event.event_type == EventType.WORKFLOW_STARTED:
                workflow_type = event.event_data.get("workflow_type", "unknown")

                state = WorkflowState(
                    workflow_id=workflow_id,
                    parent_workflow_id=event.parent_workflow_id,
                    agent_id=event.agent_id,
                    workflow_type=workflow_type,
                    started_at=now,
                    last_activity=now,
                    status=event.event_status,
                    child_workflows=set(),
                    event_count=0,
                    total_processing_time_ms=0.0,
                    error_count=0,
                )

                self._active_workflows[workflow_id] = state
                self._agent_workflows[event.agent_id].add(workflow_id)

            else:
                # Event for unknown workflow - may be recovered or late event
                logger.warning(
                    f"Received event {event.event_type.value} for unknown workflow {workflow_id}"
                )
                return

        # Update existing state
        state = self._active_workflows[workflow_id]
        state.last_activity = now
        state.event_count += 1

        if event.processing_time_ms:
            state.total_processing_time_ms += event.processing_time_ms

        if event.event_status == EventStatus.FAILURE:
            state.error_count += 1
            state.status = EventStatus.FAILURE
        elif event.event_type == EventType.WORKFLOW_FINISHED:
            state.status = event.event_status
            # Workflow completed - will be cleaned up later

        # Update current stage for stage events
        if event.event_type == EventType.STAGE_COMPLETED:
            stage_name = event.event_data.get("stage_name")
            if stage_name:
                state.current_stage = stage_name

        # Remove workflow from active tracking if finished
        if event.event_type == EventType.WORKFLOW_FINISHED:
            # Update final metrics
            if event.event_status == EventStatus.SUCCESS:
                self._performance_metrics["workflows_completed"] += 1
            else:
                self._performance_metrics["workflows_failed"] += 1

    async def _persist_workflow_event(self, event: WorkflowEventData) -> None:
        """Persist workflow event to storage."""
        try:
            # Convert to storage format
            storage_data = {
                "workflow_id": event.workflow_id,
                "agent_id": event.agent_id,
                "timestamp": datetime.utcnow(),
                "event_type": event.event_type.value,
                "event_status": event.event_status.value,
                "event_data": event.event_data or {},
                "processing_time_ms": event.processing_time_ms,
                "parent_workflow_id": event.parent_workflow_id,
                "tenant_id": self.tenant_id,
                "created_at": datetime.utcnow(),
                "updated_at": datetime.utcnow(),
            }

            # Insert via storage adapter
            await self.storage._execute_with_monitoring(
                """
                INSERT INTO workflow_events (
                    id, workflow_id, agent_id, timestamp, event_type, event_status,
                    event_data, processing_time_ms, parent_workflow_id, tenant_id,
                    created_at, updated_at
                ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
                """,
                str(uuid.uuid4()),
                str(event.workflow_id),
                event.agent_id,
                storage_data["timestamp"],
                storage_data["event_type"],
                storage_data["event_status"],
                storage_data["event_data"],
                storage_data["processing_time_ms"],
                str(event.parent_workflow_id) if event.parent_workflow_id else None,
                self.tenant_id,
                storage_data["created_at"],
                storage_data["updated_at"],
                tenant_id=self.tenant_id,
            )

        except Exception as e:
            raise WorkflowTrackingError(f"Failed to persist workflow event: {e}")

    async def _query_workflow_events(
        self,
        workflow_ids: List[uuid.UUID],
        event_type: Optional[EventType] = None,
        limit: int = 10000,
    ) -> List[Dict[str, Any]]:
        """Query workflow events from storage with optimization."""
        try:
            # Build query parameters
            id_placeholders = ", ".join(f"${i+1}" for i in range(len(workflow_ids)))
            args = [str(wf_id) for wf_id in workflow_ids]

            # Add event type filter if specified
            event_filter = ""
            if event_type:
                args.append(event_type.value)
                event_filter = f"AND event_type = ${len(args)}"

            query = f"""
            SELECT
                workflow_id,
                agent_id,
                timestamp,
                event_type,
                event_status,
                event_data,
                processing_time_ms,
                parent_workflow_id
            FROM workflow_events
            WHERE workflow_id::text IN ({id_placeholders}) {event_filter}
            ORDER BY timestamp DESC, workflow_id
            LIMIT {limit}
            """

            result = await self.storage._execute_with_monitoring(
                query, *args, fetch=True, tenant_id=self.tenant_id
            )

            # Convert to dictionaries
            events = []
            for row in result or []:
                events.append({
                    "workflow_id": row[0],
                    "agent_id": row[1],
                    "timestamp": row[2].isoformat() if row[2] else None,
                    "event_type": row[3],
                    "event_status": row[4],
                    "event_data": row[5] or {},
                    "processing_time_ms": row[6],
                    "parent_workflow_id": row[7],
                })

            return events

        except Exception as e:
            raise WorkflowQueryError(f"Failed to query workflow events: {e}")

    async def _check_workflow_exists(self, workflow_id: uuid.UUID) -> bool:
        """Check if workflow exists in storage."""
        try:
            result = await self.storage._execute_with_monitoring(
                """
                SELECT COUNT(*) FROM workflow_events
                WHERE workflow_id = $1 LIMIT 1
                """,
                str(workflow_id),
                fetch=True,
                tenant_id=self.tenant_id,
            )

            return result and result[0][0] > 0

        except Exception as e:
            logger.error(f"Failed to check workflow existence: {e}")
            return False

    async def _get_child_workflow_ids(self, parent_id: uuid.UUID) -> List[uuid.UUID]:
        """Get child workflow IDs for a parent workflow."""
        # Check in-memory hierarchy first
        children = list(self._workflow_hierarchy.get(parent_id, set()))

        # Query storage for any children not in memory
        try:
            result = await self.storage._execute_with_monitoring(
                """
                SELECT DISTINCT workflow_id FROM workflow_events
                WHERE parent_workflow_id = $1
                """,
                str(parent_id),
                fetch=True,
                tenant_id=self.tenant_id,
            )

            for row in result or []:
                child_id = uuid.UUID(row[0])
                if child_id not in children:
                    children.append(child_id)

        except Exception as e:
            logger.error(f"Failed to query child workflows: {e}")

        return children

    async def _recover_active_workflows(self) -> None:
        """Recover active workflows from storage after restart."""
        try:
            # Look for workflows started in the last 24 hours that haven't finished
            recent_time = datetime.utcnow() - timedelta(hours=24)

            result = await self.storage._execute_with_monitoring(
                """
                SELECT DISTINCT
                    w1.workflow_id,
                    w1.agent_id,
                    w1.parent_workflow_id,
                    w1.event_data->>'workflow_type' as workflow_type,
                    MIN(w1.timestamp) as started_at,
                    MAX(w1.timestamp) as last_activity
                FROM workflow_events w1
                WHERE w1.timestamp >= $1
                AND w1.event_type = 'workflow_started'
                AND NOT EXISTS (
                    SELECT 1 FROM workflow_events w2
                    WHERE w2.workflow_id = w1.workflow_id
                    AND w2.event_type = 'workflow_finished'
                )
                GROUP BY w1.workflow_id, w1.agent_id, w1.parent_workflow_id, w1.event_data->>'workflow_type'
                """,
                recent_time,
                fetch=True,
                tenant_id=self.tenant_id,
            )

            recovered_count = 0
            for row in result or []:
                workflow_id = uuid.UUID(row[0])
                agent_id = row[1]
                parent_workflow_id = uuid.UUID(row[2]) if row[2] else None
                workflow_type = row[3] or "unknown"
                started_at = row[4]
                last_activity = row[5]

                # Create workflow state
                state = WorkflowState(
                    workflow_id=workflow_id,
                    parent_workflow_id=parent_workflow_id,
                    agent_id=agent_id,
                    workflow_type=workflow_type,
                    started_at=started_at,
                    last_activity=last_activity,
                    status=EventStatus.RETRY,  # Unknown status, mark for monitoring
                    child_workflows=set(),
                    event_count=0,
                    total_processing_time_ms=0.0,
                    error_count=0,
                )

                self._active_workflows[workflow_id] = state
                self._agent_workflows[agent_id].add(workflow_id)

                if parent_workflow_id:
                    self._workflow_hierarchy[parent_workflow_id].add(workflow_id)

                recovered_count += 1

            if recovered_count > 0:
                logger.info(f"Recovered {recovered_count} active workflows from storage")

        except Exception as e:
            logger.error(f"Failed to recover active workflows: {e}")

    async def _persist_active_workflows(self) -> None:
        """Persist current state of active workflows for crash recovery."""
        # This could save workflow state to a separate recovery table
        # For now, we rely on the event log for recovery
        pass

    async def _periodic_cleanup(self) -> None:
        """Background task for periodic cleanup operations."""
        while True:
            try:
                await asyncio.sleep(self.cleanup_interval.total_seconds())
                await self.cleanup_expired_workflows()

            except asyncio.CancelledError:
                logger.info("Cleanup task cancelled")
                break
            except Exception as e:
                logger.error(f"Error in periodic cleanup: {e}")

    def _get_cached_result(self, cache_key: str) -> Optional[List[Dict[str, Any]]]:
        """Get cached query result if still valid."""
        if cache_key in self._query_cache:
            result, timestamp = self._query_cache[cache_key]
            if datetime.utcnow() - timestamp < self._cache_ttl:
                return result
            else:
                del self._query_cache[cache_key]
        return None

    def _cache_result(self, cache_key: str, result: List[Dict[str, Any]]) -> None:
        """Cache query result with timestamp."""
        # Limit cache size
        if len(self._query_cache) >= 100:
            # Remove oldest entry
            oldest_key = min(self._query_cache.keys(), key=lambda k: self._query_cache[k][1])
            del self._query_cache[oldest_key]

        self._query_cache[cache_key] = (result, datetime.utcnow())

    def _cleanup_cache(self) -> None:
        """Remove expired cache entries."""
        expired_keys = []
        cutoff_time = datetime.utcnow() - self._cache_ttl

        for key, (result, timestamp) in self._query_cache.items():
            if timestamp < cutoff_time:
                expired_keys.append(key)

        for key in expired_keys:
            del self._query_cache[key]

    def _update_query_metrics(self, query_time_ms: float) -> None:
        """Update query performance metrics."""
        self._performance_metrics["query_count"] += 1

        # Update rolling average
        current_avg = self._performance_metrics["avg_query_time_ms"]
        count = self._performance_metrics["query_count"]

        new_avg = ((current_avg * (count - 1)) + query_time_ms) / count
        self._performance_metrics["avg_query_time_ms"] = new_avg

    def _get_active_workflow_stats(self) -> Dict[str, Any]:
        """Get statistics about currently active workflows."""
        total_active = len(self._active_workflows)
        by_agent = defaultdict(int)
        by_status = defaultdict(int)
        total_events = 0
        total_errors = 0

        for state in self._active_workflows.values():
            by_agent[state.agent_id] += 1
            by_status[state.status.value if hasattr(state.status, 'value') else str(state.status)] += 1
            total_events += state.event_count
            total_errors += state.error_count

        return {
            "total_active": total_active,
            "by_agent": dict(by_agent),
            "by_status": dict(by_status),
            "total_events": total_events,
            "total_errors": total_errors,
            "memory_usage": {
                "active_workflows": len(self._active_workflows),
                "hierarchy_entries": len(self._workflow_hierarchy),
                "agent_mappings": len(self._agent_workflows),
                "cache_entries": len(self._query_cache),
            }
        }

    def _analyze_workflow_status(self, events: List[Dict[str, Any]]) -> Dict[str, Any]:
        """Analyze workflow events to determine overall status."""
        if not events:
            return {"status": "unknown", "reason": "no events"}

        # Sort events by timestamp
        events.sort(key=lambda e: e.get("timestamp", ""))

        start_event = None
        finish_event = None
        error_count = 0
        stage_count = 0
        total_processing_time = 0.0

        for event in events:
            if event.get("event_type") == "workflow_started":
                start_event = event
            elif event.get("event_type") == "workflow_finished":
                finish_event = event
            elif event.get("event_type") == "stage_completed":
                stage_count += 1
            elif event.get("event_status") == "failure":
                error_count += 1

            if event.get("processing_time_ms"):
                total_processing_time += event["processing_time_ms"]

        # Determine final status
        if finish_event:
            status = finish_event.get("event_status", "completed")
        elif error_count > 0:
            status = "failed"
        elif start_event:
            status = "incomplete"
        else:
            status = "unknown"

        return {
            "status": status,
            "started_at": start_event.get("timestamp") if start_event else None,
            "finished_at": finish_event.get("timestamp") if finish_event else None,
            "stage_count": stage_count,
            "error_count": error_count,
            "total_processing_time_ms": total_processing_time,
            "event_count": len(events),
        }

    def _analyze_workflow_performance(self, events: List[Dict[str, Any]]) -> Dict[str, Any]:
        """Analyze workflow performance from events."""
        if not events:
            return {"error": "No events to analyze"}

        # Group events by type and agent
        by_type = defaultdict(list)
        by_agent = defaultdict(list)
        processing_times = []

        for event in events:
            by_type[event.get("event_type", "unknown")].append(event)
            by_agent[event.get("agent_id", "unknown")].append(event)

            if event.get("processing_time_ms"):
                processing_times.append(event["processing_time_ms"])

        # Calculate statistics
        total_processing_time = sum(processing_times)
        avg_processing_time = total_processing_time / len(processing_times) if processing_times else 0

        return {
            "total_events": len(events),
            "event_types": {k: len(v) for k, v in by_type.items()},
            "agents_involved": list(by_agent.keys()),
            "processing_time": {
                "total_ms": total_processing_time,
                "average_ms": avg_processing_time,
                "min_ms": min(processing_times) if processing_times else 0,
                "max_ms": max(processing_times) if processing_times else 0,
            },
            "timeline": {
                "start": min(e.get("timestamp", "") for e in events),
                "end": max(e.get("timestamp", "") for e in events),
            }
        }

    def _identify_bottlenecks(self, events: List[Dict[str, Any]]) -> List[Dict[str, Any]]:
        """Identify performance bottlenecks in workflow execution."""
        bottlenecks = []

        # Find stages with high processing times
        stage_times = defaultdict(list)
        for event in events:
            if event.get("event_type") == "stage_completed" and event.get("processing_time_ms"):
                stage_name = event.get("event_data", {}).get("stage_name", "unknown")
                stage_times[stage_name].append(event["processing_time_ms"])

        # Calculate percentiles for each stage
        for stage, times in stage_times.items():
            if len(times) > 1:
                times.sort()
                p95 = times[int(0.95 * len(times))]
                avg_time = sum(times) / len(times)

                if p95 > avg_time * 2:  # 95th percentile more than 2x average
                    bottlenecks.append({
                        "type": "stage_performance",
                        "stage_name": stage,
                        "average_ms": avg_time,
                        "p95_ms": p95,
                        "sample_count": len(times),
                    })

        return bottlenecks

    def _analyze_parallel_execution(self, events: List[Dict[str, Any]]) -> Dict[str, Any]:
        """Analyze parallel execution efficiency."""
        # Group events by workflow ID to separate parallel streams
        workflow_streams = defaultdict(list)

        for event in events:
            wf_id = event.get("workflow_id")
            if wf_id:
                workflow_streams[wf_id].append(event)

        if len(workflow_streams) <= 1:
            return {"parallel_workflows": 0, "efficiency": 1.0}

        # Calculate overlap and efficiency
        workflow_timelines = {}
        for wf_id, wf_events in workflow_streams.items():
            if wf_events:
                start_time = min(e.get("timestamp", "") for e in wf_events)
                end_time = max(e.get("timestamp", "") for e in wf_events)
                workflow_timelines[wf_id] = (start_time, end_time)

        # Calculate parallel efficiency (simplified)
        total_parallel_time = sum(
            (datetime.fromisoformat(end.replace('Z', '+00:00')) -
             datetime.fromisoformat(start.replace('Z', '+00:00'))).total_seconds() * 1000
            for start, end in workflow_timelines.values()
        )

        if workflow_timelines:
            overall_start = min(start for start, end in workflow_timelines.values())
            overall_end = max(end for start, end in workflow_timelines.values())
            wall_clock_time = (datetime.fromisoformat(overall_end.replace('Z', '+00:00')) -
                             datetime.fromisoformat(overall_start.replace('Z', '+00:00'))).total_seconds() * 1000

            efficiency = wall_clock_time / total_parallel_time if total_parallel_time > 0 else 0
        else:
            efficiency = 0

        return {
            "parallel_workflows": len(workflow_streams),
            "efficiency": efficiency,
            "wall_clock_time_ms": wall_clock_time if workflow_timelines else 0,
            "total_parallel_time_ms": total_parallel_time,
        }

    def _analyze_failures(self, failures: List[Dict[str, Any]]) -> Dict[str, Any]:
        """Analyze failure events for patterns and attribution."""
        if not failures:
            return {"failure_count": 0}

        # Group by error type and agent
        by_error_type = defaultdict(int)
        by_agent = defaultdict(int)
        failure_timeline = []

        for failure in failures:
            error_type = failure.get("event_data", {}).get("error_type", "unknown")
            agent_id = failure.get("agent_id", "unknown")

            by_error_type[error_type] += 1
            by_agent[agent_id] += 1

            failure_timeline.append({
                "timestamp": failure.get("timestamp"),
                "agent": agent_id,
                "error_type": error_type,
                "error_message": failure.get("event_data", {}).get("error_message", ""),
            })

        # Sort timeline by timestamp
        failure_timeline.sort(key=lambda f: f.get("timestamp", ""))

        return {
            "failure_count": len(failures),
            "error_types": dict(by_error_type),
            "failures_by_agent": dict(by_agent),
            "timeline": failure_timeline,
        }

    async def _compute_workflow_analytics(self, workflow_id: uuid.UUID) -> Optional[WorkflowAnalytics]:
        """Compute comprehensive analytics for a workflow."""
        try:
            events = await self.get_workflow_events(workflow_id, include_children=True)
            if not events:
                return None

            # Calculate metrics
            total_duration_ms = 0.0
            stage_count = 0
            agents = set()
            success_count = 0
            error_types = []

            for event in events:
                if event.get("processing_time_ms"):
                    total_duration_ms += event["processing_time_ms"]

                if event.get("event_type") == "stage_completed":
                    stage_count += 1

                if event.get("agent_id"):
                    agents.add(event["agent_id"])

                if event.get("event_status") == "success":
                    success_count += 1
                elif event.get("event_status") == "failure":
                    error_type = event.get("event_data", {}).get("error_type")
                    if error_type:
                        error_types.append(error_type)

            success_rate = success_count / len(events) if events else 0.0

            # Identify bottlenecks
            bottlenecks = self._identify_bottlenecks(events)

            # Calculate parallel efficiency
            parallel_analysis = self._analyze_parallel_execution(events)
            parallel_efficiency = parallel_analysis.get("efficiency", 0.0)

            return WorkflowAnalytics(
                workflow_id=workflow_id,
                total_duration_ms=total_duration_ms,
                stage_count=stage_count,
                agent_count=len(agents),
                success_rate=success_rate,
                error_types=list(set(error_types)),
                bottleneck_stages=bottlenecks,
                parallel_efficiency=parallel_efficiency,
            )

        except Exception as e:
            logger.error(f"Failed to compute workflow analytics: {e}")
            return None

    async def _query_agent_workflows(self, agent_id: str, since_time: datetime) -> List[Dict[str, Any]]:
        """Query historical workflows for an agent from storage."""
        try:
            result = await self.storage._execute_with_monitoring(
                """
                SELECT DISTINCT
                    workflow_id,
                    MIN(timestamp) as started_at,
                    MAX(timestamp) as last_activity,
                    COUNT(*) as event_count,
                    SUM(CASE WHEN event_status = 'failure' THEN 1 ELSE 0 END) as error_count,
                    COALESCE(
                        (SELECT event_data->>'workflow_type'
                         FROM workflow_events we2
                         WHERE we2.workflow_id = we.workflow_id
                         AND we2.event_type = 'workflow_started'
                         LIMIT 1),
                        'unknown'
                    ) as workflow_type
                FROM workflow_events we
                WHERE agent_id = $1 AND timestamp >= $2
                GROUP BY workflow_id
                ORDER BY started_at DESC
                """,
                agent_id,
                since_time,
                fetch=True,
                tenant_id=self.tenant_id,
            )

            workflows = []
            for row in result or []:
                workflows.append({
                    "workflow_id": row[0],
                    "status": "completed",
                    "started_at": row[1].isoformat() if row[1] else None,
                    "last_activity": row[2].isoformat() if row[2] else None,
                    "event_count": row[3],
                    "error_count": row[4],
                    "workflow_type": row[5],
                })

            return workflows

        except Exception as e:
            logger.error(f"Failed to query agent workflows: {e}")
            return []

    async def _query_workflow_metrics(
        self,
        since_time: datetime,
        group_by: str
    ) -> Dict[str, Any]:
        """Query aggregated workflow metrics from storage."""
        try:
            if group_by == "agent":
                query = """
                SELECT
                    agent_id,
                    COUNT(DISTINCT workflow_id) as workflow_count,
                    COUNT(*) as event_count,
                    AVG(processing_time_ms) as avg_processing_time,
                    SUM(CASE WHEN event_status = 'failure' THEN 1 ELSE 0 END) as failure_count
                FROM workflow_events
                WHERE timestamp >= $1
                GROUP BY agent_id
                ORDER BY workflow_count DESC
                """
            elif group_by == "type":
                query = """
                SELECT
                    event_type,
                    COUNT(*) as event_count,
                    AVG(processing_time_ms) as avg_processing_time,
                    SUM(CASE WHEN event_status = 'failure' THEN 1 ELSE 0 END) as failure_count
                FROM workflow_events
                WHERE timestamp >= $1
                GROUP BY event_type
                ORDER BY event_count DESC
                """
            else:  # status
                query = """
                SELECT
                    event_status,
                    COUNT(*) as event_count,
                    AVG(processing_time_ms) as avg_processing_time
                FROM workflow_events
                WHERE timestamp >= $1
                GROUP BY event_status
                ORDER BY event_count DESC
                """

            result = await self.storage._execute_with_monitoring(
                query, since_time, fetch=True, tenant_id=self.tenant_id
            )

            metrics = {"group_by": group_by, "data": []}
            for row in result or []:
                if group_by == "agent":
                    metrics["data"].append({
                        "agent_id": row[0],
                        "workflow_count": row[1],
                        "event_count": row[2],
                        "avg_processing_time": float(row[3]) if row[3] else 0.0,
                        "failure_count": row[4],
                    })
                elif group_by == "type":
                    metrics["data"].append({
                        "event_type": row[0],
                        "event_count": row[1],
                        "avg_processing_time": float(row[2]) if row[2] else 0.0,
                        "failure_count": row[3],
                    })
                else:  # status
                    metrics["data"].append({
                        "event_status": row[0],
                        "event_count": row[1],
                        "avg_processing_time": float(row[2]) if row[2] else 0.0,
                    })

            return metrics

        except Exception as e:
            logger.error(f"Failed to query workflow metrics: {e}")
            return {"group_by": group_by, "data": [], "error": str(e)}


# Factory function for easy instantiation
async def create_workflow_tracker(
    storage_adapter: TimescaleAdapter,
    tenant_id: str,
    **kwargs
) -> WorkflowTracker:
    """Create and initialize a WorkflowTracker instance.

    Args:
        storage_adapter: TimescaleDB storage adapter
        tenant_id: Tenant context for isolation
        **kwargs: Additional configuration options

    Returns:
        Initialized WorkflowTracker instance

    Raises:
        WorkflowTrackingError: If creation or initialization fails
    """
    tracker = WorkflowTracker(storage_adapter, tenant_id, **kwargs)
    await tracker.initialize()
    return tracker


# Export public API
__all__ = [
    'WorkflowTracker',
    'WorkflowTrackingError',
    'WorkflowCorrelationError',
    'WorkflowQueryError',
    'WorkflowValidationError',
    'WorkflowState',
    'WorkflowAnalytics',
    'create_workflow_tracker',
]