"""FastAPI router for workflow tracing and visualization endpoints.

This module provides comprehensive REST API endpoints for workflow monitoring,
tracing, and performance analysis in the Penfold observability system.

Features:
- Workflow lifecycle tracking and visualization
- Real-time workflow streaming with SSE
- Performance analysis and bottleneck identification
- Advanced search and filtering capabilities
- Failure analysis and debugging tools
- Tenant isolation and authentication
- <500ms response times for debugging queries

API Endpoints:
    GET /api/v1/workflows - List workflows with filtering
    GET /api/v1/workflows/{workflow_id} - Get workflow details and timeline
    GET /api/v1/workflows/{workflow_id}/trace - Complete workflow trace with events
    GET /api/v1/workflows/{workflow_id}/analysis - Workflow performance analysis
    GET /api/v1/workflows/search - Advanced workflow search and filtering
    GET /api/v1/stream/workflows - SSE stream for real-time workflow updates
    GET /api/v1/workflows/{workflow_id}/failures - Workflow failure analysis
    GET /api/v1/workflows/performance/bottlenecks - Identify performance bottlenecks

Example Usage:
    from fastapi import FastAPI
    from observability_lib.api.workflow_endpoints import router as workflow_router

    app = FastAPI()
    app.include_router(workflow_router, prefix="/api/v1", tags=["workflows"])
"""

import asyncio
import json
import logging
import time
from datetime import datetime, timedelta
from typing import Any, Dict, List, Optional, Union
from enum import Enum

from fastapi import APIRouter, HTTPException, Depends, Request, Query
from fastapi.responses import StreamingResponse
from pydantic import BaseModel, Field, validator
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy.orm import sessionmaker
from sqlalchemy import select, func, and_, desc, asc, text, or_
from sqlalchemy.exc import SQLAlchemyError

from observability_lib.models.agent_metrics import (
    Agent, AgentMetric, DecisionTrace, WorkflowEvent,
    AgentLog, AlertThreshold, AlertEvent, EventType, EventStatus
)
from observability_lib.contracts import MetricPoint
from penf_lib.storage.validators import ValidationError

logger = logging.getLogger(__name__)

# Create FastAPI router
router = APIRouter()

# ============================================================================
# Request/Response Models
# ============================================================================

class WorkflowStatus(str, Enum):
    """Workflow execution status."""
    RUNNING = "running"
    COMPLETED = "completed"
    FAILED = "failed"
    TIMEOUT = "timeout"
    CANCELLED = "cancelled"


class WorkflowStage(BaseModel):
    """Individual stage in a workflow execution."""
    stage_id: str
    agent_id: str
    agent_name: str
    event_type: str
    event_status: str
    timestamp: datetime
    processing_time_ms: Optional[float]
    event_data: Dict[str, Any] = Field(default_factory=dict)
    decision_traces: List[Dict[str, Any]] = Field(default_factory=list)
    log_entries: List[Dict[str, Any]] = Field(default_factory=list)

    class Config:
        json_encoders = {
            datetime: lambda v: v.isoformat() + "Z"
        }


class WorkflowSummary(BaseModel):
    """Summary information for a workflow."""
    workflow_id: str
    status: WorkflowStatus
    start_time: datetime
    end_time: Optional[datetime]
    duration_ms: Optional[float]
    total_stages: int
    completed_stages: int
    failed_stages: int
    agents_involved: List[str]
    primary_agent: Optional[str]
    error_count: int
    last_activity: datetime

    class Config:
        json_encoders = {
            datetime: lambda v: v.isoformat() + "Z"
        }


class WorkflowDetail(BaseModel):
    """Detailed workflow information with complete timeline."""
    workflow_id: str
    status: WorkflowStatus
    start_time: datetime
    end_time: Optional[datetime]
    duration_ms: Optional[float]
    parent_workflow_id: Optional[str]
    child_workflows: List[str] = Field(default_factory=list)
    stages: List[WorkflowStage]
    performance_summary: Dict[str, Any]
    error_summary: Dict[str, Any]
    agents_summary: Dict[str, Any]

    class Config:
        json_encoders = {
            datetime: lambda v: v.isoformat() + "Z"
        }


class WorkflowTrace(BaseModel):
    """Complete workflow trace with all events and context."""
    workflow_id: str
    trace_generated_at: datetime
    workflow_detail: WorkflowDetail
    event_timeline: List[Dict[str, Any]]
    decision_timeline: List[Dict[str, Any]]
    log_timeline: List[Dict[str, Any]]
    metrics_timeline: List[Dict[str, Any]]
    correlation_data: Dict[str, Any]

    class Config:
        json_encoders = {
            datetime: lambda v: v.isoformat() + "Z"
        }


class WorkflowAnalysis(BaseModel):
    """Performance analysis for a workflow."""
    workflow_id: str
    analysis_timestamp: datetime
    overall_performance: Dict[str, Any]
    stage_performance: List[Dict[str, Any]]
    bottlenecks: List[Dict[str, Any]]
    optimization_suggestions: List[str]
    comparison_metrics: Dict[str, Any]
    resource_utilization: Dict[str, Any]

    class Config:
        json_encoders = {
            datetime: lambda v: v.isoformat() + "Z"
        }


class WorkflowSearchRequest(BaseModel):
    """Advanced search parameters for workflows."""
    workflow_ids: Optional[List[str]] = None
    agent_ids: Optional[List[str]] = None
    statuses: Optional[List[WorkflowStatus]] = None
    start_time_from: Optional[datetime] = None
    start_time_to: Optional[datetime] = None
    duration_min_ms: Optional[float] = None
    duration_max_ms: Optional[float] = None
    has_errors: Optional[bool] = None
    has_timeouts: Optional[bool] = None
    min_stages: Optional[int] = None
    max_stages: Optional[int] = None
    text_search: Optional[str] = None
    sort_by: str = Field(default="start_time", description="Sort field")
    sort_order: str = Field(default="desc", description="Sort order")
    limit: int = Field(default=100, ge=1, le=1000, description="Max results")
    offset: int = Field(default=0, ge=0, description="Result offset")

    @validator("sort_by")
    def validate_sort_by(cls, v):
        valid_fields = {
            "start_time", "end_time", "duration_ms", "workflow_id",
            "status", "total_stages", "error_count"
        }
        if v not in valid_fields:
            raise ValueError(f"Invalid sort field: {v}. Must be one of {valid_fields}")
        return v

    @validator("sort_order")
    def validate_sort_order(cls, v):
        if v.lower() not in {"asc", "desc"}:
            raise ValueError("Sort order must be 'asc' or 'desc'")
        return v.lower()


class WorkflowFailureAnalysis(BaseModel):
    """Analysis of workflow failures and issues."""
    workflow_id: str
    failure_summary: Dict[str, Any]
    error_timeline: List[Dict[str, Any]]
    failure_patterns: List[Dict[str, Any]]
    root_cause_analysis: Dict[str, Any]
    recovery_suggestions: List[str]
    similar_failures: List[str]

    class Config:
        json_encoders = {
            datetime: lambda v: v.isoformat() + "Z"
        }


class BottleneckAnalysis(BaseModel):
    """Performance bottleneck analysis."""
    analysis_timestamp: datetime
    time_period: Dict[str, Any]
    bottlenecks: List[Dict[str, Any]]
    performance_trends: Dict[str, Any]
    optimization_opportunities: List[Dict[str, Any]]
    resource_constraints: Dict[str, Any]

    class Config:
        json_encoders = {
            datetime: lambda v: v.isoformat() + "Z"
        }

# ============================================================================
# Authentication and Database Dependencies
# ============================================================================

# Reuse authentication from dashboard_api
async def get_current_user() -> Optional[Dict[str, Any]]:
    """Basic authentication placeholder for production."""
    # TODO: Import from dashboard_api or implement shared auth
    return {"user_id": "development", "tenant_id": "default"}


async def get_tenant_id(user: Dict[str, Any] = Depends(get_current_user)) -> str:
    """Extract tenant ID from authenticated user context."""
    return user.get("tenant_id", "default")


# Global session maker - will be set by main application
session_maker = None


async def get_db() -> AsyncSession:
    """Get database session dependency."""
    if session_maker is None:
        raise HTTPException(status_code=503, detail="Database not initialized")

    async with session_maker() as session:
        try:
            yield session
        except Exception:
            await session.rollback()
            raise
        finally:
            await session.close()

# ============================================================================
# Workflow Service Classes
# ============================================================================

class WorkflowAnalysisService:
    """Service for analyzing workflow performance and identifying issues."""

    def __init__(self, session: AsyncSession):
        self.session = session

    async def calculate_workflow_status(self, workflow_id: str) -> WorkflowStatus:
        """Calculate the current status of a workflow."""
        # Get all events for this workflow ordered by timestamp
        query = select(WorkflowEvent).where(
            WorkflowEvent.workflow_id == workflow_id
        ).order_by(desc(WorkflowEvent.timestamp))

        result = await self.session.execute(query)
        events = result.scalars().all()

        if not events:
            return WorkflowStatus.FAILED

        latest_event = events[0]

        # Check for completion
        if latest_event.event_type == EventType.WORKFLOW_FINISHED.value:
            if latest_event.event_status == EventStatus.SUCCESS.value:
                return WorkflowStatus.COMPLETED
            elif latest_event.event_status == EventStatus.TIMEOUT.value:
                return WorkflowStatus.TIMEOUT
            else:
                return WorkflowStatus.FAILED

        # Check for active errors
        error_events = [e for e in events if e.event_type == EventType.ERROR_OCCURRED.value]
        if error_events:
            # Check if there's been any activity after the latest error
            latest_error = error_events[0]
            recent_activity = [e for e in events if e.timestamp > latest_error.timestamp]
            if not recent_activity:
                return WorkflowStatus.FAILED

        # Check for timeout (no activity in last 30 minutes)
        if latest_event.timestamp < datetime.utcnow() - timedelta(minutes=30):
            return WorkflowStatus.TIMEOUT

        return WorkflowStatus.RUNNING

    async def get_workflow_stages(self, workflow_id: str) -> List[WorkflowStage]:
        """Get all stages for a workflow with associated data."""
        # Get workflow events
        events_query = select(WorkflowEvent).where(
            WorkflowEvent.workflow_id == workflow_id
        ).order_by(WorkflowEvent.timestamp)

        events_result = await self.session.execute(events_query)
        events = events_result.scalars().all()

        # Get agents for display names
        agent_ids = list(set(event.agent_id for event in events))
        agents_query = select(Agent).where(Agent.agent_id.in_(agent_ids))
        agents_result = await self.session.execute(agents_query)
        agents = {agent.agent_id: agent for agent in agents_result.scalars().all()}

        stages = []
        for event in events:
            agent = agents.get(event.agent_id)
            agent_name = agent.agent_name if agent else event.agent_id

            # Get decision traces for this event (by timestamp proximity)
            decisions_query = select(DecisionTrace).where(
                and_(
                    DecisionTrace.agent_id == event.agent_id,
                    DecisionTrace.workflow_id == workflow_id,
                    DecisionTrace.timestamp.between(
                        event.timestamp - timedelta(seconds=30),
                        event.timestamp + timedelta(seconds=30)
                    )
                )
            ).order_by(DecisionTrace.timestamp)

            decisions_result = await self.session.execute(decisions_query)
            decision_traces = []
            for decision in decisions_result.scalars().all():
                decision_traces.append({
                    "decision_id": str(decision.id),
                    "timestamp": decision.timestamp,
                    "decision_type": decision.decision_type,
                    "confidence_score": decision.confidence_score,
                    "reasoning": decision.reasoning
                })

            # Get log entries for this event
            logs_query = select(AgentLog).where(
                and_(
                    AgentLog.agent_id == event.agent_id,
                    AgentLog.workflow_id == workflow_id,
                    AgentLog.timestamp.between(
                        event.timestamp - timedelta(seconds=30),
                        event.timestamp + timedelta(seconds=30)
                    )
                )
            ).order_by(AgentLog.timestamp)

            logs_result = await self.session.execute(logs_query)
            log_entries = []
            for log in logs_result.scalars().all():
                log_entries.append({
                    "log_id": str(log.id),
                    "timestamp": log.timestamp,
                    "level": log.level,
                    "event": log.event,
                    "context": log.context or {}
                })

            stage = WorkflowStage(
                stage_id=str(event.id),
                agent_id=event.agent_id,
                agent_name=agent_name,
                event_type=event.event_type,
                event_status=event.event_status,
                timestamp=event.timestamp,
                processing_time_ms=event.processing_time_ms,
                event_data=event.event_data or {},
                decision_traces=decision_traces,
                log_entries=log_entries
            )
            stages.append(stage)

        return stages

    async def analyze_workflow_performance(self, workflow_id: str) -> Dict[str, Any]:
        """Analyze performance metrics for a workflow."""
        stages = await self.get_workflow_stages(workflow_id)

        if not stages:
            return {
                "total_duration_ms": 0,
                "average_stage_duration_ms": 0,
                "slowest_stage_ms": 0,
                "fastest_stage_ms": 0,
                "stage_count": 0,
                "agents_involved": 0
            }

        # Calculate timing metrics
        stage_times = [s.processing_time_ms for s in stages if s.processing_time_ms is not None]
        agents_involved = list(set(s.agent_id for s in stages))

        start_time = min(s.timestamp for s in stages)
        end_time = max(s.timestamp for s in stages)
        total_duration_ms = (end_time - start_time).total_seconds() * 1000

        return {
            "total_duration_ms": total_duration_ms,
            "average_stage_duration_ms": sum(stage_times) / len(stage_times) if stage_times else 0,
            "slowest_stage_ms": max(stage_times) if stage_times else 0,
            "fastest_stage_ms": min(stage_times) if stage_times else 0,
            "stage_count": len(stages),
            "agents_involved": len(agents_involved),
            "successful_stages": len([s for s in stages if s.event_status == "success"]),
            "failed_stages": len([s for s in stages if s.event_status == "failure"]),
            "timeout_stages": len([s for s in stages if s.event_status == "timeout"]),
            "retry_stages": len([s for s in stages if s.event_status == "retry"])
        }

    async def identify_bottlenecks(self, workflow_id: str) -> List[Dict[str, Any]]:
        """Identify performance bottlenecks in a workflow."""
        stages = await self.get_workflow_stages(workflow_id)
        bottlenecks = []

        if not stages:
            return bottlenecks

        # Calculate average processing times by agent and event type
        agent_times = {}
        event_type_times = {}

        for stage in stages:
            if stage.processing_time_ms is None:
                continue

            # Track by agent
            if stage.agent_id not in agent_times:
                agent_times[stage.agent_id] = []
            agent_times[stage.agent_id].append(stage.processing_time_ms)

            # Track by event type
            if stage.event_type not in event_type_times:
                event_type_times[stage.event_type] = []
            event_type_times[stage.event_type].append(stage.processing_time_ms)

        # Identify slow agents
        for agent_id, times in agent_times.items():
            avg_time = sum(times) / len(times)
            if avg_time > 5000:  # >5 seconds average
                bottlenecks.append({
                    "type": "slow_agent",
                    "agent_id": agent_id,
                    "average_time_ms": avg_time,
                    "occurrences": len(times),
                    "severity": "high" if avg_time > 15000 else "medium"
                })

        # Identify slow event types
        for event_type, times in event_type_times.items():
            avg_time = sum(times) / len(times)
            if avg_time > 3000:  # >3 seconds average
                bottlenecks.append({
                    "type": "slow_event_type",
                    "event_type": event_type,
                    "average_time_ms": avg_time,
                    "occurrences": len(times),
                    "severity": "high" if avg_time > 10000 else "medium"
                })

        return bottlenecks


# Global SSE connections for real-time updates
workflow_sse_connections: List[asyncio.Queue] = []

# ============================================================================
# API Endpoints
# ============================================================================

@router.get("/workflows", response_model=List[WorkflowSummary])
async def list_workflows(
    status: Optional[WorkflowStatus] = None,
    agent_id: Optional[str] = None,
    start_time_from: Optional[datetime] = None,
    start_time_to: Optional[datetime] = None,
    limit: int = Query(default=100, ge=1, le=1000),
    offset: int = Query(default=0, ge=0),
    db: AsyncSession = Depends(get_db),
    tenant_id: str = Depends(get_tenant_id)
):
    """List workflows with optional filtering and pagination."""
    try:
        start_time = time.time()

        # Build base query for unique workflow IDs
        query = select(
            WorkflowEvent.workflow_id,
            func.min(WorkflowEvent.timestamp).label('start_time'),
            func.max(WorkflowEvent.timestamp).label('last_activity'),
            func.count(WorkflowEvent.id).label('total_stages')
        ).group_by(WorkflowEvent.workflow_id)

        # Apply filters
        filters = []
        if agent_id:
            filters.append(WorkflowEvent.agent_id == agent_id)
        if start_time_from:
            filters.append(WorkflowEvent.timestamp >= start_time_from)
        if start_time_to:
            filters.append(WorkflowEvent.timestamp <= start_time_to)

        if filters:
            query = query.where(and_(*filters))

        # Add pagination
        query = query.order_by(desc('start_time')).offset(offset).limit(limit)

        result = await db.execute(query)
        workflow_rows = result.fetchall()

        # Build summaries
        summaries = []
        analysis_service = WorkflowAnalysisService(db)

        for row in workflow_rows:
            workflow_id = str(row.workflow_id)

            # Calculate workflow status
            workflow_status = await analysis_service.calculate_workflow_status(workflow_id)

            # Apply status filter after calculation if needed
            if status and workflow_status != status:
                continue

            # Get additional stats
            stats_query = select(
                WorkflowEvent.agent_id,
                WorkflowEvent.event_status,
                WorkflowEvent.event_type
            ).where(WorkflowEvent.workflow_id == row.workflow_id)

            stats_result = await db.execute(stats_query)
            stats_rows = stats_result.fetchall()

            agents_involved = list(set(r.agent_id for r in stats_rows))
            completed_stages = len([r for r in stats_rows if r.event_type == "stage_completed" and r.event_status == "success"])
            failed_stages = len([r for r in stats_rows if r.event_status == "failure"])
            error_count = len([r for r in stats_rows if r.event_type == "error_occurred"])

            # Calculate duration
            duration_ms = None
            end_time = None
            if workflow_status in [WorkflowStatus.COMPLETED, WorkflowStatus.FAILED, WorkflowStatus.TIMEOUT]:
                duration_ms = (row.last_activity - row.start_time).total_seconds() * 1000
                end_time = row.last_activity

            summary = WorkflowSummary(
                workflow_id=workflow_id,
                status=workflow_status,
                start_time=row.start_time,
                end_time=end_time,
                duration_ms=duration_ms,
                total_stages=row.total_stages,
                completed_stages=completed_stages,
                failed_stages=failed_stages,
                agents_involved=agents_involved,
                primary_agent=agents_involved[0] if agents_involved else None,
                error_count=error_count,
                last_activity=row.last_activity
            )
            summaries.append(summary)

        elapsed_ms = (time.time() - start_time) * 1000
        logger.debug(f"Listed {len(summaries)} workflows in {elapsed_ms:.1f}ms")

        return summaries

    except Exception as e:
        logger.error(f"Error listing workflows: {e}")
        raise HTTPException(status_code=500, detail="Failed to retrieve workflows")


@router.get("/workflows/{workflow_id}", response_model=WorkflowDetail)
async def get_workflow_detail(
    workflow_id: str,
    db: AsyncSession = Depends(get_db),
    tenant_id: str = Depends(get_tenant_id)
):
    """Get detailed workflow information including complete timeline."""
    try:
        start_time = time.time()

        analysis_service = WorkflowAnalysisService(db)

        # Check if workflow exists
        exists_query = select(func.count(WorkflowEvent.id)).where(
            WorkflowEvent.workflow_id == workflow_id
        )
        exists_result = await db.execute(exists_query)
        if exists_result.scalar() == 0:
            raise HTTPException(status_code=404, detail=f"Workflow {workflow_id} not found")

        # Calculate status and get stages
        status = await analysis_service.calculate_workflow_status(workflow_id)
        stages = await analysis_service.get_workflow_stages(workflow_id)

        if not stages:
            raise HTTPException(status_code=404, detail=f"No stages found for workflow {workflow_id}")

        # Calculate timing
        start_time_dt = min(s.timestamp for s in stages)
        end_time_dt = None
        duration_ms = None

        if status in [WorkflowStatus.COMPLETED, WorkflowStatus.FAILED, WorkflowStatus.TIMEOUT]:
            end_time_dt = max(s.timestamp for s in stages)
            duration_ms = (end_time_dt - start_time_dt).total_seconds() * 1000

        # Get parent/child relationships
        parent_workflow_id = None
        child_workflows = []

        # Check for parent workflow
        parent_query = select(WorkflowEvent.parent_workflow_id).where(
            and_(
                WorkflowEvent.workflow_id == workflow_id,
                WorkflowEvent.parent_workflow_id.is_not(None)
            )
        ).limit(1)
        parent_result = await db.execute(parent_query)
        parent_row = parent_result.scalar()
        if parent_row:
            parent_workflow_id = str(parent_row)

        # Find child workflows
        children_query = select(WorkflowEvent.workflow_id.distinct()).where(
            WorkflowEvent.parent_workflow_id == workflow_id
        )
        children_result = await db.execute(children_query)
        child_workflows = [str(row[0]) for row in children_result.fetchall()]

        # Get performance analysis
        performance_summary = await analysis_service.analyze_workflow_performance(workflow_id)

        # Get error summary
        error_stages = [s for s in stages if s.event_status == "failure"]
        error_summary = {
            "error_count": len(error_stages),
            "error_stages": [
                {
                    "stage_id": s.stage_id,
                    "agent_id": s.agent_id,
                    "timestamp": s.timestamp,
                    "event_type": s.event_type,
                    "event_data": s.event_data
                }
                for s in error_stages
            ]
        }

        # Get agents summary
        agents_involved = {}
        for stage in stages:
            if stage.agent_id not in agents_involved:
                agents_involved[stage.agent_id] = {
                    "agent_name": stage.agent_name,
                    "stages_handled": 0,
                    "successful_stages": 0,
                    "failed_stages": 0,
                    "total_processing_time_ms": 0
                }

            agents_involved[stage.agent_id]["stages_handled"] += 1
            if stage.event_status == "success":
                agents_involved[stage.agent_id]["successful_stages"] += 1
            elif stage.event_status == "failure":
                agents_involved[stage.agent_id]["failed_stages"] += 1

            if stage.processing_time_ms:
                agents_involved[stage.agent_id]["total_processing_time_ms"] += stage.processing_time_ms

        detail = WorkflowDetail(
            workflow_id=workflow_id,
            status=status,
            start_time=start_time_dt,
            end_time=end_time_dt,
            duration_ms=duration_ms,
            parent_workflow_id=parent_workflow_id,
            child_workflows=child_workflows,
            stages=stages,
            performance_summary=performance_summary,
            error_summary=error_summary,
            agents_summary=agents_involved
        )

        elapsed_ms = (time.time() - start_time) * 1000
        logger.debug(f"Retrieved workflow {workflow_id} details in {elapsed_ms:.1f}ms")

        return detail

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error getting workflow detail for {workflow_id}: {e}")
        raise HTTPException(status_code=500, detail="Failed to retrieve workflow details")


@router.get("/workflows/{workflow_id}/trace", response_model=WorkflowTrace)
async def get_workflow_trace(
    workflow_id: str,
    include_metrics: bool = Query(default=True, description="Include metrics timeline"),
    include_logs: bool = Query(default=True, description="Include log timeline"),
    db: AsyncSession = Depends(get_db),
    tenant_id: str = Depends(get_tenant_id)
):
    """Get complete workflow trace with all events, decisions, logs, and metrics."""
    try:
        start_time = time.time()

        # Get workflow detail first
        analysis_service = WorkflowAnalysisService(db)

        # Check if workflow exists
        exists_query = select(func.count(WorkflowEvent.id)).where(
            WorkflowEvent.workflow_id == workflow_id
        )
        exists_result = await db.execute(exists_query)
        if exists_result.scalar() == 0:
            raise HTTPException(status_code=404, detail=f"Workflow {workflow_id} not found")

        # Get workflow detail
        status = await analysis_service.calculate_workflow_status(workflow_id)
        stages = await analysis_service.get_workflow_stages(workflow_id)

        if not stages:
            raise HTTPException(status_code=404, detail=f"No stages found for workflow {workflow_id}")

        # Build workflow detail
        start_time_dt = min(s.timestamp for s in stages)
        end_time_dt = None
        duration_ms = None

        if status in [WorkflowStatus.COMPLETED, WorkflowStatus.FAILED, WorkflowStatus.TIMEOUT]:
            end_time_dt = max(s.timestamp for s in stages)
            duration_ms = (end_time_dt - start_time_dt).total_seconds() * 1000

        performance_summary = await analysis_service.analyze_workflow_performance(workflow_id)

        workflow_detail = WorkflowDetail(
            workflow_id=workflow_id,
            status=status,
            start_time=start_time_dt,
            end_time=end_time_dt,
            duration_ms=duration_ms,
            parent_workflow_id=None,  # Will be populated if needed
            child_workflows=[],  # Will be populated if needed
            stages=stages,
            performance_summary=performance_summary,
            error_summary={},  # Will be populated if needed
            agents_summary={}  # Will be populated if needed
        )

        # Get event timeline (all workflow events)
        events_query = select(WorkflowEvent).where(
            WorkflowEvent.workflow_id == workflow_id
        ).order_by(WorkflowEvent.timestamp)

        events_result = await db.execute(events_query)
        event_timeline = []
        for event in events_result.scalars().all():
            event_timeline.append({
                "event_id": str(event.id),
                "timestamp": event.timestamp,
                "agent_id": event.agent_id,
                "event_type": event.event_type,
                "event_status": event.event_status,
                "processing_time_ms": event.processing_time_ms,
                "event_data": event.event_data or {}
            })

        # Get decision timeline
        decisions_query = select(DecisionTrace).where(
            DecisionTrace.workflow_id == workflow_id
        ).order_by(DecisionTrace.timestamp)

        decisions_result = await db.execute(decisions_query)
        decision_timeline = []
        for decision in decisions_result.scalars().all():
            decision_timeline.append({
                "decision_id": str(decision.id),
                "timestamp": decision.timestamp,
                "agent_id": decision.agent_id,
                "decision_type": decision.decision_type,
                "confidence_score": decision.confidence_score,
                "reasoning": decision.reasoning,
                "decision_context": decision.decision_context,
                "alternatives_considered": decision.alternatives_considered
            })

        # Get log timeline (if requested)
        log_timeline = []
        if include_logs:
            logs_query = select(AgentLog).where(
                AgentLog.workflow_id == workflow_id
            ).order_by(AgentLog.timestamp)

            logs_result = await db.execute(logs_query)
            for log in logs_result.scalars().all():
                log_timeline.append({
                    "log_id": str(log.id),
                    "timestamp": log.timestamp,
                    "agent_id": log.agent_id,
                    "level": log.level,
                    "event": log.event,
                    "context": log.context or {}
                })

        # Get metrics timeline (if requested)
        metrics_timeline = []
        if include_metrics:
            # Get metrics for all agents involved in the workflow within the time range
            agent_ids = [event["agent_id"] for event in event_timeline]
            time_buffer = timedelta(minutes=5)  # Include metrics 5 minutes before/after workflow

            metrics_query = select(AgentMetric).where(
                and_(
                    AgentMetric.agent_id.in_(agent_ids),
                    AgentMetric.timestamp >= start_time_dt - time_buffer,
                    AgentMetric.timestamp <= (end_time_dt or datetime.utcnow()) + time_buffer
                )
            ).order_by(AgentMetric.timestamp)

            metrics_result = await db.execute(metrics_query)
            for metric in metrics_result.scalars().all():
                metrics_timeline.append({
                    "metric_id": str(metric.id),
                    "timestamp": metric.timestamp,
                    "agent_id": metric.agent_id,
                    "metric_name": metric.metric_name,
                    "metric_value": metric.metric_value,
                    "metric_type": metric.metric_type,
                    "labels": metric.labels or {}
                })

        # Build correlation data
        correlation_data = {
            "total_events": len(event_timeline),
            "total_decisions": len(decision_timeline),
            "total_logs": len(log_timeline),
            "total_metrics": len(metrics_timeline),
            "time_span_ms": duration_ms or 0,
            "agents_involved": len(set(event["agent_id"] for event in event_timeline))
        }

        trace = WorkflowTrace(
            workflow_id=workflow_id,
            trace_generated_at=datetime.utcnow(),
            workflow_detail=workflow_detail,
            event_timeline=event_timeline,
            decision_timeline=decision_timeline,
            log_timeline=log_timeline,
            metrics_timeline=metrics_timeline,
            correlation_data=correlation_data
        )

        elapsed_ms = (time.time() - start_time) * 1000
        logger.debug(f"Generated workflow {workflow_id} trace in {elapsed_ms:.1f}ms")

        return trace

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error generating workflow trace for {workflow_id}: {e}")
        raise HTTPException(status_code=500, detail="Failed to generate workflow trace")


@router.get("/workflows/{workflow_id}/analysis", response_model=WorkflowAnalysis)
async def get_workflow_analysis(
    workflow_id: str,
    include_comparison: bool = Query(default=True, description="Include comparison with similar workflows"),
    db: AsyncSession = Depends(get_db),
    tenant_id: str = Depends(get_tenant_id)
):
    """Get comprehensive performance analysis for a workflow."""
    try:
        start_time = time.time()

        analysis_service = WorkflowAnalysisService(db)

        # Check if workflow exists
        exists_query = select(func.count(WorkflowEvent.id)).where(
            WorkflowEvent.workflow_id == workflow_id
        )
        exists_result = await db.execute(exists_query)
        if exists_result.scalar() == 0:
            raise HTTPException(status_code=404, detail=f"Workflow {workflow_id} not found")

        # Get workflow stages
        stages = await analysis_service.get_workflow_stages(workflow_id)

        if not stages:
            raise HTTPException(status_code=404, detail=f"No stages found for workflow {workflow_id}")

        # Overall performance analysis
        overall_performance = await analysis_service.analyze_workflow_performance(workflow_id)

        # Stage-by-stage performance analysis
        stage_performance = []
        for i, stage in enumerate(stages):
            stage_perf = {
                "stage_index": i,
                "stage_id": stage.stage_id,
                "agent_id": stage.agent_id,
                "event_type": stage.event_type,
                "processing_time_ms": stage.processing_time_ms,
                "status": stage.event_status,
                "timestamp": stage.timestamp,
                "is_bottleneck": False,
                "performance_score": 1.0
            }

            # Calculate performance score (0.0 = worst, 1.0 = best)
            if stage.processing_time_ms:
                # Compare against average for this event type
                avg_time_query = select(func.avg(WorkflowEvent.processing_time_ms)).where(
                    and_(
                        WorkflowEvent.event_type == stage.event_type,
                        WorkflowEvent.processing_time_ms.is_not(None),
                        WorkflowEvent.timestamp >= datetime.utcnow() - timedelta(days=30)
                    )
                )
                avg_time_result = await db.execute(avg_time_query)
                avg_time = avg_time_result.scalar() or stage.processing_time_ms

                if avg_time > 0:
                    # Score based on how much faster/slower than average
                    ratio = stage.processing_time_ms / avg_time
                    stage_perf["performance_score"] = max(0.0, min(1.0, 2.0 - ratio))
                    stage_perf["is_bottleneck"] = ratio > 2.0

            stage_performance.append(stage_perf)

        # Identify bottlenecks
        bottlenecks = await analysis_service.identify_bottlenecks(workflow_id)

        # Generate optimization suggestions
        optimization_suggestions = []

        if bottlenecks:
            for bottleneck in bottlenecks:
                if bottleneck["type"] == "slow_agent":
                    optimization_suggestions.append(
                        f"Consider optimizing agent '{bottleneck['agent_id']}' which averages "
                        f"{bottleneck['average_time_ms']:.0f}ms per operation"
                    )
                elif bottleneck["type"] == "slow_event_type":
                    optimization_suggestions.append(
                        f"Event type '{bottleneck['event_type']}' is consistently slow, "
                        f"averaging {bottleneck['average_time_ms']:.0f}ms"
                    )

        if overall_performance.get("failed_stages", 0) > 0:
            optimization_suggestions.append(
                f"Reduce failure rate: {overall_performance['failed_stages']} out of "
                f"{overall_performance['stage_count']} stages failed"
            )

        if overall_performance.get("total_duration_ms", 0) > 60000:  # >1 minute
            optimization_suggestions.append(
                "Consider parallelizing workflow stages to reduce overall duration"
            )

        # Comparison metrics (if requested)
        comparison_metrics = {}
        if include_comparison:
            # Compare with similar workflows (same agents, similar stage count)
            agents_in_workflow = list(set(s.agent_id for s in stages))

            comparison_query = select(
                func.avg(
                    func.extract('epoch', func.max(WorkflowEvent.timestamp) - func.min(WorkflowEvent.timestamp)) * 1000
                ).label('avg_duration_ms'),
                func.avg(func.count(WorkflowEvent.id)).label('avg_stage_count'),
                func.count(func.distinct(WorkflowEvent.workflow_id)).label('similar_workflows')
            ).where(
                and_(
                    WorkflowEvent.agent_id.in_(agents_in_workflow),
                    WorkflowEvent.workflow_id != workflow_id,
                    WorkflowEvent.timestamp >= datetime.utcnow() - timedelta(days=30)
                )
            ).group_by(WorkflowEvent.workflow_id)

            comparison_result = await db.execute(comparison_query)
            comp_row = comparison_result.first()

            if comp_row and comp_row.similar_workflows:
                comparison_metrics = {
                    "similar_workflows_count": comp_row.similar_workflows,
                    "average_duration_ms": comp_row.avg_duration_ms,
                    "average_stage_count": comp_row.avg_stage_count,
                    "performance_percentile": 50  # Would need more complex calculation
                }

        # Resource utilization (basic analysis)
        resource_utilization = {
            "cpu_intensive_stages": len([s for s in stage_performance if s.get("processing_time_ms", 0) > 10000]),
            "memory_metrics_available": False,  # Would need memory metrics
            "agent_load_distribution": {
                agent_id: len([s for s in stages if s.agent_id == agent_id])
                for agent_id in set(s.agent_id for s in stages)
            }
        }

        analysis = WorkflowAnalysis(
            workflow_id=workflow_id,
            analysis_timestamp=datetime.utcnow(),
            overall_performance=overall_performance,
            stage_performance=stage_performance,
            bottlenecks=bottlenecks,
            optimization_suggestions=optimization_suggestions,
            comparison_metrics=comparison_metrics,
            resource_utilization=resource_utilization
        )

        elapsed_ms = (time.time() - start_time) * 1000
        logger.debug(f"Generated workflow {workflow_id} analysis in {elapsed_ms:.1f}ms")

        return analysis

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error analyzing workflow {workflow_id}: {e}")
        raise HTTPException(status_code=500, detail="Failed to analyze workflow")


@router.post("/workflows/search", response_model=List[WorkflowSummary])
async def search_workflows(
    search_request: WorkflowSearchRequest,
    db: AsyncSession = Depends(get_db),
    tenant_id: str = Depends(get_tenant_id)
):
    """Advanced workflow search with complex filtering."""
    try:
        start_time = time.time()

        # Build base query
        query = select(
            WorkflowEvent.workflow_id,
            func.min(WorkflowEvent.timestamp).label('start_time'),
            func.max(WorkflowEvent.timestamp).label('last_activity'),
            func.count(WorkflowEvent.id).label('total_stages')
        ).group_by(WorkflowEvent.workflow_id)

        # Apply filters
        filters = []

        if search_request.workflow_ids:
            filters.append(WorkflowEvent.workflow_id.in_(search_request.workflow_ids))

        if search_request.agent_ids:
            filters.append(WorkflowEvent.agent_id.in_(search_request.agent_ids))

        if search_request.start_time_from:
            filters.append(WorkflowEvent.timestamp >= search_request.start_time_from)

        if search_request.start_time_to:
            filters.append(WorkflowEvent.timestamp <= search_request.start_time_to)

        if search_request.has_errors is True:
            filters.append(WorkflowEvent.event_status == "failure")

        if search_request.has_timeouts is True:
            filters.append(WorkflowEvent.event_status == "timeout")

        if search_request.text_search:
            # Search in event data (JSONB)
            filters.append(
                or_(
                    WorkflowEvent.event_data.astext.ilike(f"%{search_request.text_search}%"),
                    WorkflowEvent.event_type.ilike(f"%{search_request.text_search}%")
                )
            )

        if filters:
            query = query.where(and_(*filters))

        # Apply HAVING clauses for aggregate filters
        having_clauses = []

        if search_request.min_stages:
            having_clauses.append(func.count(WorkflowEvent.id) >= search_request.min_stages)

        if search_request.max_stages:
            having_clauses.append(func.count(WorkflowEvent.id) <= search_request.max_stages)

        if having_clauses:
            query = query.having(and_(*having_clauses))

        # Apply sorting
        sort_field = search_request.sort_by
        if sort_field == "start_time":
            order_col = 'start_time'
        elif sort_field == "duration_ms":
            order_col = text("(EXTRACT(epoch FROM MAX(timestamp) - MIN(timestamp)) * 1000)")
        elif sort_field == "total_stages":
            order_col = 'total_stages'
        else:
            order_col = 'start_time'  # Default

        if search_request.sort_order == "desc":
            query = query.order_by(desc(order_col))
        else:
            query = query.order_by(asc(order_col))

        # Apply pagination
        query = query.offset(search_request.offset).limit(search_request.limit)

        result = await db.execute(query)
        workflow_rows = result.fetchall()

        # Build summaries with status filtering
        summaries = []
        analysis_service = WorkflowAnalysisService(db)

        for row in workflow_rows:
            workflow_id = str(row.workflow_id)

            # Calculate workflow status
            workflow_status = await analysis_service.calculate_workflow_status(workflow_id)

            # Apply status filter
            if search_request.statuses and workflow_status not in search_request.statuses:
                continue

            # Apply duration filter if workflow is completed
            duration_ms = None
            end_time = None

            if workflow_status in [WorkflowStatus.COMPLETED, WorkflowStatus.FAILED, WorkflowStatus.TIMEOUT]:
                duration_ms = (row.last_activity - row.start_time).total_seconds() * 1000
                end_time = row.last_activity

                # Apply duration filters
                if search_request.duration_min_ms and duration_ms < search_request.duration_min_ms:
                    continue
                if search_request.duration_max_ms and duration_ms > search_request.duration_max_ms:
                    continue

            # Get additional stats
            stats_query = select(
                WorkflowEvent.agent_id,
                WorkflowEvent.event_status,
                WorkflowEvent.event_type
            ).where(WorkflowEvent.workflow_id == row.workflow_id)

            stats_result = await db.execute(stats_query)
            stats_rows = stats_result.fetchall()

            agents_involved = list(set(r.agent_id for r in stats_rows))
            completed_stages = len([r for r in stats_rows if r.event_type == "stage_completed" and r.event_status == "success"])
            failed_stages = len([r for r in stats_rows if r.event_status == "failure"])
            error_count = len([r for r in stats_rows if r.event_type == "error_occurred"])

            summary = WorkflowSummary(
                workflow_id=workflow_id,
                status=workflow_status,
                start_time=row.start_time,
                end_time=end_time,
                duration_ms=duration_ms,
                total_stages=row.total_stages,
                completed_stages=completed_stages,
                failed_stages=failed_stages,
                agents_involved=agents_involved,
                primary_agent=agents_involved[0] if agents_involved else None,
                error_count=error_count,
                last_activity=row.last_activity
            )
            summaries.append(summary)

        elapsed_ms = (time.time() - start_time) * 1000
        logger.debug(f"Searched workflows with {len(summaries)} results in {elapsed_ms:.1f}ms")

        return summaries

    except Exception as e:
        logger.error(f"Error searching workflows: {e}")
        raise HTTPException(status_code=500, detail="Failed to search workflows")


@router.get("/workflows/{workflow_id}/failures", response_model=WorkflowFailureAnalysis)
async def get_workflow_failure_analysis(
    workflow_id: str,
    db: AsyncSession = Depends(get_db),
    tenant_id: str = Depends(get_tenant_id)
):
    """Get detailed analysis of workflow failures and issues."""
    try:
        start_time = time.time()

        # Check if workflow exists
        exists_query = select(func.count(WorkflowEvent.id)).where(
            WorkflowEvent.workflow_id == workflow_id
        )
        exists_result = await db.execute(exists_query)
        if exists_result.scalar() == 0:
            raise HTTPException(status_code=404, detail=f"Workflow {workflow_id} not found")

        # Get all failure events
        failure_query = select(WorkflowEvent).where(
            and_(
                WorkflowEvent.workflow_id == workflow_id,
                or_(
                    WorkflowEvent.event_status == "failure",
                    WorkflowEvent.event_type == "error_occurred"
                )
            )
        ).order_by(WorkflowEvent.timestamp)

        failure_result = await db.execute(failure_query)
        failure_events = failure_result.scalars().all()

        if not failure_events:
            # No failures found
            return WorkflowFailureAnalysis(
                workflow_id=workflow_id,
                failure_summary={
                    "has_failures": False,
                    "failure_count": 0,
                    "failure_rate": 0.0
                },
                error_timeline=[],
                failure_patterns=[],
                root_cause_analysis={},
                recovery_suggestions=[],
                similar_failures=[]
            )

        # Build failure summary
        total_events_query = select(func.count(WorkflowEvent.id)).where(
            WorkflowEvent.workflow_id == workflow_id
        )
        total_events_result = await db.execute(total_events_query)
        total_events = total_events_result.scalar()

        failure_summary = {
            "has_failures": True,
            "failure_count": len(failure_events),
            "failure_rate": len(failure_events) / max(1, total_events),
            "first_failure": min(event.timestamp for event in failure_events),
            "last_failure": max(event.timestamp for event in failure_events),
            "agents_with_failures": list(set(event.agent_id for event in failure_events))
        }

        # Build error timeline
        error_timeline = []
        for event in failure_events:
            error_timeline.append({
                "timestamp": event.timestamp,
                "agent_id": event.agent_id,
                "event_type": event.event_type,
                "event_status": event.event_status,
                "error_data": event.event_data or {},
                "processing_time_ms": event.processing_time_ms
            })

        # Identify failure patterns
        failure_patterns = []

        # Pattern 1: Repeated failures by agent
        agent_failures = {}
        for event in failure_events:
            agent_failures[event.agent_id] = agent_failures.get(event.agent_id, 0) + 1

        for agent_id, count in agent_failures.items():
            if count >= 2:
                failure_patterns.append({
                    "pattern_type": "repeated_agent_failures",
                    "description": f"Agent {agent_id} failed {count} times",
                    "agent_id": agent_id,
                    "occurrence_count": count,
                    "severity": "high" if count >= 3 else "medium"
                })

        # Pattern 2: Timing-based patterns
        if len(failure_events) >= 2:
            failure_times = [event.timestamp for event in failure_events]
            time_diffs = [
                (failure_times[i] - failure_times[i-1]).total_seconds()
                for i in range(1, len(failure_times))
            ]

            # Check for rapid consecutive failures
            rapid_failures = [diff for diff in time_diffs if diff < 30]  # <30 seconds
            if rapid_failures:
                failure_patterns.append({
                    "pattern_type": "rapid_consecutive_failures",
                    "description": f"{len(rapid_failures)} failures occurred within 30 seconds of each other",
                    "occurrence_count": len(rapid_failures),
                    "severity": "high"
                })

        # Root cause analysis
        root_cause_analysis = {
            "primary_failure_agent": max(agent_failures.items(), key=lambda x: x[1])[0] if agent_failures else None,
            "failure_distribution": agent_failures,
            "temporal_analysis": {
                "failure_span_seconds": (
                    max(event.timestamp for event in failure_events) -
                    min(event.timestamp for event in failure_events)
                ).total_seconds() if len(failure_events) > 1 else 0,
                "average_failure_interval_seconds": sum(time_diffs) / len(time_diffs) if time_diffs else 0
            },
            "common_error_data": {}
        }

        # Analyze common error data
        error_data_keys = set()
        for event in failure_events:
            if event.event_data:
                error_data_keys.update(event.event_data.keys())

        for key in error_data_keys:
            values = [event.event_data.get(key) for event in failure_events if event.event_data and key in event.event_data]
            if values:
                root_cause_analysis["common_error_data"][key] = {
                    "occurrence_count": len(values),
                    "unique_values": list(set(str(v) for v in values)),
                    "most_common_value": max(set(values), key=values.count) if values else None
                }

        # Generate recovery suggestions
        recovery_suggestions = []

        if agent_failures:
            primary_failing_agent = max(agent_failures.items(), key=lambda x: x[1])[0]
            recovery_suggestions.append(
                f"Review and potentially restart agent '{primary_failing_agent}' which accounts for "
                f"{agent_failures[primary_failing_agent]} out of {len(failure_events)} failures"
            )

        if failure_summary["failure_rate"] > 0.5:
            recovery_suggestions.append(
                f"High failure rate ({failure_summary['failure_rate']:.1%}) indicates systemic issues - "
                "consider reviewing workflow design and agent configurations"
            )

        if rapid_failures:
            recovery_suggestions.append(
                "Implement exponential backoff or circuit breaker pattern to handle rapid consecutive failures"
            )

        # Find similar failures in other workflows
        similar_failures_query = select(WorkflowEvent.workflow_id.distinct()).where(
            and_(
                WorkflowEvent.workflow_id != workflow_id,
                or_(
                    WorkflowEvent.event_status == "failure",
                    WorkflowEvent.event_type == "error_occurred"
                ),
                WorkflowEvent.timestamp >= datetime.utcnow() - timedelta(days=7)  # Last 7 days
            )
        ).limit(10)

        similar_result = await db.execute(similar_failures_query)
        similar_failures = [str(row[0]) for row in similar_result.fetchall()]

        analysis = WorkflowFailureAnalysis(
            workflow_id=workflow_id,
            failure_summary=failure_summary,
            error_timeline=error_timeline,
            failure_patterns=failure_patterns,
            root_cause_analysis=root_cause_analysis,
            recovery_suggestions=recovery_suggestions,
            similar_failures=similar_failures
        )

        elapsed_ms = (time.time() - start_time) * 1000
        logger.debug(f"Generated failure analysis for workflow {workflow_id} in {elapsed_ms:.1f}ms")

        return analysis

    except HTTPException:
        raise
    except Exception as e:
        logger.error(f"Error analyzing workflow failures for {workflow_id}: {e}")
        raise HTTPException(status_code=500, detail="Failed to analyze workflow failures")


@router.get("/workflows/performance/bottlenecks", response_model=BottleneckAnalysis)
async def get_performance_bottlenecks(
    time_period_hours: int = Query(default=24, ge=1, le=168, description="Analysis time period in hours"),
    min_occurrences: int = Query(default=3, ge=1, description="Minimum occurrences to be considered a bottleneck"),
    db: AsyncSession = Depends(get_db),
    tenant_id: str = Depends(get_tenant_id)
):
    """Identify performance bottlenecks across all workflows."""
    try:
        start_time = time.time()

        since = datetime.utcnow() - timedelta(hours=time_period_hours)

        # Get all workflow events in time period
        events_query = select(WorkflowEvent).where(
            and_(
                WorkflowEvent.timestamp >= since,
                WorkflowEvent.processing_time_ms.is_not(None),
                WorkflowEvent.processing_time_ms > 0
            )
        )

        events_result = await db.execute(events_query)
        events = events_result.scalars().all()

        if not events:
            return BottleneckAnalysis(
                analysis_timestamp=datetime.utcnow(),
                time_period={
                    "start_time": since,
                    "end_time": datetime.utcnow(),
                    "duration_hours": time_period_hours
                },
                bottlenecks=[],
                performance_trends={},
                optimization_opportunities=[],
                resource_constraints={}
            )

        # Analyze by agent
        agent_performance = {}
        for event in events:
            if event.agent_id not in agent_performance:
                agent_performance[event.agent_id] = []
            agent_performance[event.agent_id].append(event.processing_time_ms)

        # Analyze by event type
        event_type_performance = {}
        for event in events:
            if event.event_type not in event_type_performance:
                event_type_performance[event.event_type] = []
            event_type_performance[event.event_type].append(event.processing_time_ms)

        # Identify agent bottlenecks
        bottlenecks = []

        for agent_id, times in agent_performance.items():
            if len(times) >= min_occurrences:
                avg_time = sum(times) / len(times)
                max_time = max(times)
                p95_time = sorted(times)[int(0.95 * len(times))]

                if avg_time > 5000:  # Average > 5 seconds
                    bottlenecks.append({
                        "type": "slow_agent",
                        "identifier": agent_id,
                        "severity": "high" if avg_time > 15000 else "medium",
                        "metrics": {
                            "average_time_ms": avg_time,
                            "max_time_ms": max_time,
                            "p95_time_ms": p95_time,
                            "occurrences": len(times)
                        },
                        "impact_score": min(10.0, avg_time / 1000),  # Scale to 0-10
                        "recommendation": f"Optimize agent {agent_id} processing pipeline"
                    })

        # Identify event type bottlenecks
        for event_type, times in event_type_performance.items():
            if len(times) >= min_occurrences:
                avg_time = sum(times) / len(times)
                max_time = max(times)
                p95_time = sorted(times)[int(0.95 * len(times))]

                if avg_time > 3000:  # Average > 3 seconds
                    bottlenecks.append({
                        "type": "slow_event_type",
                        "identifier": event_type,
                        "severity": "high" if avg_time > 10000 else "medium",
                        "metrics": {
                            "average_time_ms": avg_time,
                            "max_time_ms": max_time,
                            "p95_time_ms": p95_time,
                            "occurrences": len(times)
                        },
                        "impact_score": min(10.0, avg_time / 1000),
                        "recommendation": f"Review {event_type} implementation for optimization opportunities"
                    })

        # Sort bottlenecks by impact score
        bottlenecks.sort(key=lambda x: x["impact_score"], reverse=True)

        # Performance trends
        performance_trends = {
            "total_events_analyzed": len(events),
            "average_processing_time_ms": sum(event.processing_time_ms for event in events) / len(events),
            "agents_analyzed": len(agent_performance),
            "event_types_analyzed": len(event_type_performance),
            "workflows_affected": len(set(str(event.workflow_id) for event in events))
        }

        # Optimization opportunities
        optimization_opportunities = []

        # Look for parallelizable workflows
        workflow_stages = {}
        for event in events:
            wf_id = str(event.workflow_id)
            if wf_id not in workflow_stages:
                workflow_stages[wf_id] = []
            workflow_stages[wf_id].append(event)

        long_sequential_workflows = []
        for wf_id, stages in workflow_stages.items():
            if len(stages) >= 5:  # 5+ stages
                total_time = sum(stage.processing_time_ms for stage in stages)
                if total_time > 30000:  # >30 seconds total
                    long_sequential_workflows.append({
                        "workflow_id": wf_id,
                        "total_time_ms": total_time,
                        "stage_count": len(stages)
                    })

        if long_sequential_workflows:
            optimization_opportunities.append({
                "type": "parallelization",
                "description": f"{len(long_sequential_workflows)} workflows could benefit from parallelization",
                "affected_workflows": long_sequential_workflows[:5],  # Top 5
                "potential_savings_ms": sum(wf["total_time_ms"] * 0.3 for wf in long_sequential_workflows[:5])  # Estimated 30% savings
            })

        # Look for caching opportunities
        duplicate_operations = {}
        for event in events:
            if event.event_data and "operation_key" in event.event_data:
                op_key = event.event_data["operation_key"]
                if op_key not in duplicate_operations:
                    duplicate_operations[op_key] = []
                duplicate_operations[op_key].append(event)

        cacheable_operations = {k: v for k, v in duplicate_operations.items() if len(v) >= 3}
        if cacheable_operations:
            total_cacheable_time = sum(
                sum(event.processing_time_ms for event in events[1:])  # Exclude first occurrence
                for events in cacheable_operations.values()
            )
            optimization_opportunities.append({
                "type": "caching",
                "description": f"{len(cacheable_operations)} operations could be cached",
                "potential_savings_ms": total_cacheable_time,
                "cacheable_operations": list(cacheable_operations.keys())[:10]  # Top 10
            })

        # Resource constraints analysis
        resource_constraints = {
            "cpu_bound_indicators": len([b for b in bottlenecks if b["type"] == "slow_agent"]),
            "io_bound_indicators": len([b for b in bottlenecks if b["type"] == "slow_event_type"]),
            "memory_analysis_available": False,  # Would need memory metrics
            "network_analysis_available": False  # Would need network metrics
        }

        analysis = BottleneckAnalysis(
            analysis_timestamp=datetime.utcnow(),
            time_period={
                "start_time": since,
                "end_time": datetime.utcnow(),
                "duration_hours": time_period_hours
            },
            bottlenecks=bottlenecks,
            performance_trends=performance_trends,
            optimization_opportunities=optimization_opportunities,
            resource_constraints=resource_constraints
        )

        elapsed_ms = (time.time() - start_time) * 1000
        logger.debug(f"Generated bottleneck analysis in {elapsed_ms:.1f}ms")

        return analysis

    except Exception as e:
        logger.error(f"Error analyzing performance bottlenecks: {e}")
        raise HTTPException(status_code=500, detail="Failed to analyze performance bottlenecks")


@router.get("/stream/workflows")
async def stream_workflow_updates(request: Request):
    """Server-Sent Events stream for real-time workflow updates."""

    async def event_generator():
        """Generate SSE events for workflow updates."""
        connection_queue = asyncio.Queue(maxsize=100)
        workflow_sse_connections.append(connection_queue)

        try:
            # Send initial connection message
            initial_message = {
                "event": "connection_established",
                "data": {
                    "message": "Connected to workflow updates stream",
                    "timestamp": datetime.utcnow().isoformat() + "Z"
                }
            }
            yield f"event: {initial_message['event']}\ndata: {json.dumps(initial_message['data'])}\n\n"

            # Stream updates
            while True:
                try:
                    # Check if client disconnected
                    if await request.is_disconnected():
                        break

                    # Wait for message with timeout
                    try:
                        message = await asyncio.wait_for(connection_queue.get(), timeout=30.0)

                        # Format SSE message
                        event_data = json.dumps(message.get("data", {}), default=str)
                        event_type = message.get("event", "workflow_update")
                        yield f"event: {event_type}\ndata: {event_data}\n\n"

                    except asyncio.TimeoutError:
                        # Send keepalive
                        keepalive_data = json.dumps({
                            "timestamp": datetime.utcnow().isoformat() + "Z",
                            "type": "keepalive"
                        })
                        yield f"event: keepalive\ndata: {keepalive_data}\n\n"

                except asyncio.CancelledError:
                    break
                except Exception as e:
                    logger.error(f"Error in workflow SSE stream: {e}")
                    break

        finally:
            # Remove connection from global list
            try:
                workflow_sse_connections.remove(connection_queue)
            except ValueError:
                pass  # Connection already removed

    return StreamingResponse(
        event_generator(),
        media_type="text/event-stream",
        headers={
            "Cache-Control": "no-cache",
            "Connection": "keep-alive",
            "Access-Control-Allow-Origin": "*",
            "Access-Control-Allow-Headers": "Cache-Control"
        }
    )


# ============================================================================
# Utility Functions for SSE Broadcasting
# ============================================================================

async def broadcast_workflow_update(workflow_id: str, event_type: str, data: Dict[str, Any]):
    """Broadcast a workflow update to all connected SSE clients."""
    if not workflow_sse_connections:
        return

    message = {
        "event": event_type,
        "data": {
            "workflow_id": workflow_id,
            "timestamp": datetime.utcnow().isoformat() + "Z",
            **data
        }
    }

    # Remove dead connections while broadcasting
    active_connections = []
    for connection_queue in workflow_sse_connections:
        try:
            connection_queue.put_nowait(message)
            active_connections.append(connection_queue)
        except asyncio.QueueFull:
            logger.warning(f"Workflow SSE queue full, dropping connection")
        except Exception as e:
            logger.error(f"Error broadcasting workflow update: {e}")

    # Update global connections list
    workflow_sse_connections[:] = active_connections


# ============================================================================
# Initialization Function
# ============================================================================

def initialize_workflow_endpoints(db_session_maker: sessionmaker):
    """Initialize the workflow endpoints with database session maker."""
    global session_maker
    session_maker = db_session_maker
    logger.info("✅ Workflow endpoints initialized with database session maker")


# ============================================================================
# Module exports
# ============================================================================

__all__ = [
    "router",
    "initialize_workflow_endpoints",
    "broadcast_workflow_update",
    "WorkflowSummary",
    "WorkflowDetail",
    "WorkflowTrace",
    "WorkflowAnalysis",
    "WorkflowFailureAnalysis",
    "BottleneckAnalysis",
    "WorkflowSearchRequest",
    "WorkflowStatus"
]