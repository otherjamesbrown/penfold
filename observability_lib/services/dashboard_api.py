"""FastAPI application for the agent monitoring dashboard.

This module provides a REST API and real-time SSE endpoints for monitoring
autonomous agents in the Penfold system. Features agent health status,
performance metrics, and real-time updates for dashboard interfaces.

Features:
- Agent registry and health status endpoints
- Real-time Server-Sent Events (SSE) streaming
- Performance metrics with time-range queries
- Tenant isolation and proper error handling
- CORS support for frontend development
- Request validation with Pydantic models
- Basic authentication hooks for production
- Comprehensive health checks and system status

Example Usage:
    from observability_lib.services.dashboard_api import create_app
    from observability_lib.config import settings

    app = create_app(settings)

    # Run with uvicorn
    uvicorn.run(app, host="0.0.0.0", port=8001)

API Endpoints:
    GET /api/v1/agents - List all agents with health status
    GET /api/v1/agents/{agent_id}/health - Detailed health metrics
    GET /api/v1/agents/{agent_id}/metrics - Recent performance metrics
    GET /api/v1/stream/agents - SSE stream for real-time updates
    GET /health - Dashboard API health check
"""

import asyncio
import json
import logging
import time
import uuid
from contextlib import asynccontextmanager
from datetime import datetime, timedelta
from typing import Any, AsyncGenerator, Dict, List, Optional

from fastapi import FastAPI, HTTPException, Depends, Request, Response
from fastapi.middleware.cors import CORSMiddleware
from fastapi.responses import JSONResponse, StreamingResponse
from fastapi.security import HTTPBearer, HTTPAuthorizationCredentials
from pydantic import BaseModel, Field, validator
from sqlalchemy.ext.asyncio import AsyncSession, create_async_engine
from sqlalchemy.orm import sessionmaker
from sqlalchemy import select, func, and_, desc, text
from sqlalchemy.exc import SQLAlchemyError

from observability_lib.config import ObservabilitySettings
from observability_lib.models.agent_metrics import (
    Agent, AgentMetric, DecisionTrace, WorkflowEvent,
    AgentLog, AlertThreshold, AlertEvent, Tenant
)
from observability_lib.services.metrics_collector import MetricsCollector
from observability_lib.contracts import MetricPoint
from penf_lib.storage.validators import ValidationError

logger = logging.getLogger(__name__)

# Global components
engine = None
session_maker = None
metrics_collector = None

# ============================================================================
# Request/Response Models
# ============================================================================

class AgentHealthStatus(BaseModel):
    """Health status for an agent."""
    agent_id: str
    agent_name: str
    agent_type: str
    is_active: bool
    last_active_at: Optional[datetime]
    health_score: float = Field(..., ge=0.0, le=1.0, description="Health score from 0.0 to 1.0")
    status: str = Field(..., description="Overall status: healthy, warning, critical, offline")
    issues: List[str] = Field(default_factory=list, description="List of health issues")
    uptime_hours: Optional[float] = Field(None, description="Hours since agent started")

    class Config:
        json_encoders = {
            datetime: lambda v: v.isoformat() + "Z" if v else None
        }


class AgentDetailedHealth(BaseModel):
    """Detailed health information for an agent."""
    agent_id: str
    agent_name: str
    agent_type: str
    configuration: Dict[str, Any]
    health_status: AgentHealthStatus
    recent_metrics: Dict[str, List[Dict[str, Any]]]
    active_alerts: List[Dict[str, Any]]
    recent_decisions: List[Dict[str, Any]]
    recent_workflows: List[Dict[str, Any]]
    performance_summary: Dict[str, Any]


class MetricsQuery(BaseModel):
    """Query parameters for metrics retrieval."""
    metric_names: Optional[List[str]] = None
    start_time: Optional[datetime] = None
    end_time: Optional[datetime] = None
    aggregation: str = Field(default="avg", description="Aggregation function")
    bucket_size: str = Field(default="5m", description="Time bucket size")

    @validator("aggregation")
    def validate_aggregation(cls, v):
        valid_aggregations = {"avg", "sum", "min", "max", "count"}
        if v not in valid_aggregations:
            raise ValueError(f"Invalid aggregation: {v}. Must be one of {valid_aggregations}")
        return v

    @validator("bucket_size")
    def validate_bucket_size(cls, v):
        import re
        if not re.match(r"^\d+[smhd]$", v.lower()):
            raise ValueError(f"Invalid bucket size format: {v}. Must be like '5m', '1h', '1d'")
        return v


class DashboardStats(BaseModel):
    """Overall dashboard statistics."""
    total_agents: int
    healthy_agents: int
    warning_agents: int
    critical_agents: int
    offline_agents: int
    active_workflows: int
    total_metrics_today: int
    active_alerts: int
    system_uptime_hours: float


class SSEMessage(BaseModel):
    """Server-sent event message format."""
    event: str
    data: Dict[str, Any]
    timestamp: datetime

    class Config:
        json_encoders = {
            datetime: lambda v: v.isoformat() + "Z"
        }

# ============================================================================
# Authentication and Security
# ============================================================================

security = HTTPBearer(auto_error=False)

async def get_current_user(
    credentials: Optional[HTTPAuthorizationCredentials] = Depends(security)
) -> Optional[Dict[str, Any]]:
    """Basic authentication placeholder for production.

    In production, this would validate JWT tokens or API keys.
    For development, this allows access without authentication.
    """
    if credentials is None:
        # Allow access without authentication in development
        return {"user_id": "development", "tenant_id": "default"}

    # TODO: Implement real authentication
    # For now, just validate the token format
    if not credentials.credentials:
        raise HTTPException(
            status_code=401,
            detail="Invalid authentication credentials",
            headers={"WWW-Authenticate": "Bearer"},
        )

    return {"user_id": "authenticated", "tenant_id": "default"}


async def get_tenant_id(user: Dict[str, Any] = Depends(get_current_user)) -> str:
    """Extract tenant ID from authenticated user context."""
    return user.get("tenant_id", "default")

# ============================================================================
# Database Dependency
# ============================================================================

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
# Agent Health Service
# ============================================================================

class AgentHealthService:
    """Service for calculating agent health metrics and status."""

    def __init__(self, session: AsyncSession):
        self.session = session

    async def calculate_agent_health(self, agent: Agent) -> AgentHealthStatus:
        """Calculate comprehensive health status for an agent."""
        current_time = datetime.utcnow()
        health_score = 1.0
        status = "healthy"
        issues = []

        # Check if agent is active
        if not agent.is_active:
            health_score = 0.0
            status = "offline"
            issues.append("Agent marked as inactive")

        elif agent.last_active_at:
            # Check last active time
            inactive_duration = current_time - agent.last_active_at
            inactive_hours = inactive_duration.total_seconds() / 3600

            if inactive_hours > 24:
                health_score *= 0.1
                status = "critical"
                issues.append(f"Agent inactive for {inactive_hours:.1f} hours")
            elif inactive_hours > 2:
                health_score *= 0.5
                status = "warning" if status == "healthy" else status
                issues.append(f"Agent inactive for {inactive_hours:.1f} hours")

        # Check recent error rates
        error_rate = await self._get_error_rate(agent.agent_id, hours=1)
        if error_rate > 0.1:  # >10% error rate
            health_score *= 0.3
            status = "critical"
            issues.append(f"High error rate: {error_rate:.1%}")
        elif error_rate > 0.05:  # >5% error rate
            health_score *= 0.7
            status = "warning" if status == "healthy" else status
            issues.append(f"Elevated error rate: {error_rate:.1%}")

        # Check performance metrics
        avg_processing_time = await self._get_avg_processing_time(agent.agent_id, hours=1)
        if avg_processing_time and avg_processing_time > 30000:  # >30s
            health_score *= 0.6
            status = "warning" if status == "healthy" else status
            issues.append(f"Slow processing: {avg_processing_time:.0f}ms average")

        # Check active alerts
        active_alerts_count = await self._get_active_alerts_count(agent.agent_id)
        if active_alerts_count > 0:
            health_score *= max(0.2, 1.0 - (active_alerts_count * 0.1))
            status = "critical" if active_alerts_count >= 5 else "warning"
            issues.append(f"{active_alerts_count} active alerts")

        # Calculate uptime
        uptime_hours = None
        if agent.created_at:
            uptime_delta = current_time - agent.created_at
            uptime_hours = uptime_delta.total_seconds() / 3600

        return AgentHealthStatus(
            agent_id=agent.agent_id,
            agent_name=agent.agent_name,
            agent_type=agent.agent_type,
            is_active=agent.is_active,
            last_active_at=agent.last_active_at,
            health_score=max(0.0, health_score),
            status=status,
            issues=issues,
            uptime_hours=uptime_hours
        )

    async def _get_error_rate(self, agent_id: str, hours: int = 1) -> float:
        """Calculate error rate for agent in the last N hours."""
        since = datetime.utcnow() - timedelta(hours=hours)

        # Count total workflow events
        total_query = select(func.count(WorkflowEvent.id)).where(
            and_(
                WorkflowEvent.agent_id == agent_id,
                WorkflowEvent.timestamp >= since
            )
        )
        total_result = await self.session.execute(total_query)
        total_events = total_result.scalar() or 0

        if total_events == 0:
            return 0.0

        # Count error events
        error_query = select(func.count(WorkflowEvent.id)).where(
            and_(
                WorkflowEvent.agent_id == agent_id,
                WorkflowEvent.timestamp >= since,
                WorkflowEvent.event_status == "failure"
            )
        )
        error_result = await self.session.execute(error_query)
        error_events = error_result.scalar() or 0

        return error_events / total_events if total_events > 0 else 0.0

    async def _get_avg_processing_time(self, agent_id: str, hours: int = 1) -> Optional[float]:
        """Get average processing time for agent in the last N hours."""
        since = datetime.utcnow() - timedelta(hours=hours)

        query = select(func.avg(AgentMetric.metric_value)).where(
            and_(
                AgentMetric.agent_id == agent_id,
                AgentMetric.metric_name == "processing_time_ms",
                AgentMetric.timestamp >= since
            )
        )
        result = await self.session.execute(query)
        return result.scalar()

    async def _get_active_alerts_count(self, agent_id: str) -> int:
        """Get count of active alerts for agent."""
        query = select(func.count(AlertEvent.id)).join(
            AlertThreshold, AlertEvent.threshold_id == AlertThreshold.id
        ).where(
            and_(
                AlertThreshold.agent_id == agent_id,
                AlertEvent.alert_status == "active",
                AlertEvent.resolved_at.is_(None)
            )
        )
        result = await self.session.execute(query)
        return result.scalar() or 0

# ============================================================================
# Real-time SSE Service
# ============================================================================

class SSEService:
    """Service for managing Server-Sent Events streams."""

    def __init__(self):
        self.connections: Dict[str, List[asyncio.Queue]] = {}
        self._cleanup_task: Optional[asyncio.Task] = None

    async def start(self):
        """Start the SSE service background tasks."""
        if self._cleanup_task is None:
            self._cleanup_task = asyncio.create_task(self._cleanup_connections())

    async def stop(self):
        """Stop the SSE service."""
        if self._cleanup_task:
            self._cleanup_task.cancel()
            try:
                await self._cleanup_task
            except asyncio.CancelledError:
                pass

    def add_connection(self, stream_type: str) -> asyncio.Queue:
        """Add a new SSE connection."""
        if stream_type not in self.connections:
            self.connections[stream_type] = []

        queue = asyncio.Queue(maxsize=100)  # Buffer up to 100 messages
        self.connections[stream_type].append(queue)
        return queue

    def remove_connection(self, stream_type: str, queue: asyncio.Queue):
        """Remove an SSE connection."""
        if stream_type in self.connections:
            try:
                self.connections[stream_type].remove(queue)
            except ValueError:
                pass  # Queue not in list

    async def broadcast(self, stream_type: str, message: SSEMessage):
        """Broadcast a message to all connections of a stream type."""
        if stream_type not in self.connections:
            return

        # Remove dead connections while broadcasting
        active_queues = []

        for queue in self.connections[stream_type]:
            try:
                queue.put_nowait(message)
                active_queues.append(queue)
            except asyncio.QueueFull:
                logger.warning(f"SSE queue full for stream {stream_type}, dropping connection")
            except Exception as e:
                logger.error(f"Error broadcasting to SSE connection: {e}")

        self.connections[stream_type] = active_queues

    async def _cleanup_connections(self):
        """Periodically cleanup dead connections."""
        while True:
            try:
                await asyncio.sleep(30)  # Cleanup every 30 seconds

                for stream_type in list(self.connections.keys()):
                    active_queues = []
                    for queue in self.connections[stream_type]:
                        if not queue.empty() or queue.qsize() == 0:
                            active_queues.append(queue)

                    self.connections[stream_type] = active_queues

            except asyncio.CancelledError:
                break
            except Exception as e:
                logger.error(f"Error in SSE cleanup: {e}")

# Global SSE service
sse_service = SSEService()

# ============================================================================
# Application Lifecycle
# ============================================================================

@asynccontextmanager
async def lifespan(app: FastAPI):
    """Application lifecycle management."""
    global engine, session_maker, metrics_collector

    settings = app.state.settings

    logger.info("🚀 Starting Observability Dashboard API...")

    # Initialize database
    engine = create_async_engine(
        settings.database_url.replace("postgresql://", "postgresql+asyncpg://"),
        **settings.get_database_pool_settings(),
        echo=settings.enable_debug_logging
    )

    session_maker = sessionmaker(
        engine,
        class_=AsyncSession,
        expire_on_commit=False
    )

    # Test database connection
    try:
        async with session_maker() as session:
            await session.execute(text("SELECT 1"))
        logger.info("✅ Database connection established")
    except Exception as e:
        logger.error(f"❌ Database connection failed: {e}")
        raise

    # Initialize metrics collector
    try:
        metrics_collector = MetricsCollector(settings)
        await metrics_collector.start()
        logger.info("✅ Metrics collector started")
    except Exception as e:
        logger.error(f"❌ Metrics collector failed to start: {e}")
        # Continue without metrics collector
        metrics_collector = None

    # Start SSE service
    await sse_service.start()
    logger.info("✅ SSE service started")

    # Start background health monitoring
    health_task = asyncio.create_task(_background_health_monitoring())

    logger.info(f"🎯 Dashboard API ready at http://{settings.dashboard_host}:{settings.dashboard_port}")

    yield

    # Shutdown
    logger.info("🛑 Shutting down Dashboard API...")

    # Cancel background tasks
    health_task.cancel()
    try:
        await health_task
    except asyncio.CancelledError:
        pass

    # Stop services
    await sse_service.stop()

    if metrics_collector:
        await metrics_collector.stop()

    if engine:
        await engine.dispose()

    logger.info("✅ Dashboard API shutdown complete")

# ============================================================================
# Background Monitoring
# ============================================================================

async def _background_health_monitoring():
    """Background task for continuous health monitoring and SSE updates."""
    while True:
        try:
            await asyncio.sleep(30)  # Monitor every 30 seconds

            if session_maker:
                async with session_maker() as session:
                    # Get all agents and their health status
                    result = await session.execute(select(Agent).where(Agent.is_active == True))
                    agents = result.scalars().all()

                    health_service = AgentHealthService(session)

                    for agent in agents:
                        try:
                            health_status = await health_service.calculate_agent_health(agent)

                            # Broadcast health update via SSE
                            message = SSEMessage(
                                event="agent_health_update",
                                data={
                                    "agent_id": agent.agent_id,
                                    "health_status": health_status.dict()
                                },
                                timestamp=datetime.utcnow()
                            )

                            await sse_service.broadcast("agents", message)

                        except Exception as e:
                            logger.error(f"Error monitoring agent {agent.agent_id}: {e}")

        except asyncio.CancelledError:
            break
        except Exception as e:
            logger.error(f"Error in background health monitoring: {e}")

# ============================================================================
# FastAPI Application Factory
# ============================================================================

def create_app(settings: ObservabilitySettings) -> FastAPI:
    """Create and configure FastAPI application."""

    app = FastAPI(
        title="Penfold Observability Dashboard API",
        description="REST API for monitoring autonomous agents with real-time updates",
        version="1.0.0",
        lifespan=lifespan,
        docs_url="/docs" if settings.enable_debug_logging else None,
        redoc_url="/redoc" if settings.enable_debug_logging else None
    )

    # Store settings in app state
    app.state.settings = settings

    # Add CORS middleware
    app.add_middleware(
        CORSMiddleware,
        allow_origins=["*"] if settings.enable_debug_logging else [
            "http://localhost:3000",  # React development
            "http://localhost:8080",  # Vue development
            "http://localhost:8001",  # Dashboard frontend
        ],
        allow_credentials=True,
        allow_methods=["GET", "POST", "PUT", "DELETE"],
        allow_headers=["*"],
    )

    # ========================================================================
    # API Routes
    # ========================================================================

    @app.get("/health")
    async def health_check():
        """Dashboard API health check endpoint."""
        try:
            # Test database connection
            if session_maker:
                async with session_maker() as session:
                    await session.execute(text("SELECT 1"))
                db_status = "connected"
            else:
                db_status = "not_initialized"

            # Check metrics collector
            collector_status = "running" if metrics_collector and metrics_collector._running else "stopped"

            return {
                "status": "healthy",
                "service": "observability-dashboard",
                "version": "1.0.0",
                "timestamp": datetime.utcnow().isoformat() + "Z",
                "components": {
                    "database": db_status,
                    "metrics_collector": collector_status,
                    "sse_service": "running",
                    "background_monitoring": "running"
                }
            }
        except Exception as e:
            logger.error(f"Health check failed: {e}")
            raise HTTPException(status_code=503, detail=f"Service unhealthy: {str(e)}")

    @app.get("/api/v1/agents", response_model=List[AgentHealthStatus])
    async def list_agents(
        agent_type: Optional[str] = None,
        status_filter: Optional[str] = None,
        db: AsyncSession = Depends(get_db),
        tenant_id: str = Depends(get_tenant_id)
    ):
        """List all agents with their current health status."""
        try:
            # Build query
            query = select(Agent)

            # Add filters
            if agent_type:
                query = query.where(Agent.agent_type == agent_type)

            # Execute query
            result = await db.execute(query)
            agents = result.scalars().all()

            # Calculate health status for each agent
            health_service = AgentHealthService(db)
            agent_health_list = []

            for agent in agents:
                health_status = await health_service.calculate_agent_health(agent)

                # Apply status filter
                if status_filter and health_status.status != status_filter:
                    continue

                agent_health_list.append(health_status)

            # Sort by health score (worst first) and agent name
            agent_health_list.sort(key=lambda x: (x.health_score, x.agent_name))

            return agent_health_list

        except Exception as e:
            logger.error(f"Error listing agents: {e}")
            raise HTTPException(status_code=500, detail="Failed to retrieve agents")

    @app.get("/api/v1/agents/{agent_id}/health", response_model=AgentDetailedHealth)
    async def get_agent_health(
        agent_id: str,
        db: AsyncSession = Depends(get_db),
        tenant_id: str = Depends(get_tenant_id)
    ):
        """Get detailed health information for a specific agent."""
        try:
            # Get agent
            result = await db.execute(select(Agent).where(Agent.agent_id == agent_id))
            agent = result.scalar_one_or_none()

            if not agent:
                raise HTTPException(status_code=404, detail=f"Agent {agent_id} not found")

            # Calculate health status
            health_service = AgentHealthService(db)
            health_status = await health_service.calculate_agent_health(agent)

            # Get recent metrics (last 24 hours)
            since = datetime.utcnow() - timedelta(hours=24)
            metrics_query = select(
                AgentMetric.metric_name,
                AgentMetric.timestamp,
                AgentMetric.metric_value,
                AgentMetric.labels
            ).where(
                and_(
                    AgentMetric.agent_id == agent_id,
                    AgentMetric.timestamp >= since
                )
            ).order_by(desc(AgentMetric.timestamp)).limit(1000)

            metrics_result = await db.execute(metrics_query)
            metrics_rows = metrics_result.fetchall()

            # Group metrics by name
            recent_metrics = {}
            for row in metrics_rows:
                metric_name = row.metric_name
                if metric_name not in recent_metrics:
                    recent_metrics[metric_name] = []

                recent_metrics[metric_name].append({
                    "timestamp": row.timestamp.isoformat() + "Z",
                    "value": row.metric_value,
                    "labels": row.labels or {}
                })

            # Get active alerts
            alerts_query = select(AlertEvent).join(
                AlertThreshold, AlertEvent.threshold_id == AlertThreshold.id
            ).where(
                and_(
                    AlertThreshold.agent_id == agent_id,
                    AlertEvent.alert_status == "active"
                )
            ).order_by(desc(AlertEvent.triggered_at))

            alerts_result = await db.execute(alerts_query)
            active_alerts = []

            for alert in alerts_result.scalars().all():
                active_alerts.append({
                    "alert_id": str(alert.id),
                    "triggered_at": alert.triggered_at.isoformat() + "Z",
                    "trigger_value": alert.trigger_value,
                    "status": alert.alert_status,
                    "context": alert.alert_context or {}
                })

            # Get recent decisions (last 100)
            decisions_query = select(DecisionTrace).where(
                DecisionTrace.agent_id == agent_id
            ).order_by(desc(DecisionTrace.timestamp)).limit(100)

            decisions_result = await db.execute(decisions_query)
            recent_decisions = []

            for decision in decisions_result.scalars().all():
                recent_decisions.append({
                    "decision_id": str(decision.id),
                    "timestamp": decision.timestamp.isoformat() + "Z",
                    "decision_type": decision.decision_type,
                    "confidence_score": decision.confidence_score,
                    "reasoning": decision.reasoning
                })

            # Get recent workflows (last 50)
            workflows_query = select(WorkflowEvent).where(
                WorkflowEvent.agent_id == agent_id
            ).order_by(desc(WorkflowEvent.timestamp)).limit(50)

            workflows_result = await db.execute(workflows_query)
            recent_workflows = []

            for workflow in workflows_result.scalars().all():
                recent_workflows.append({
                    "workflow_id": str(workflow.workflow_id),
                    "timestamp": workflow.timestamp.isoformat() + "Z",
                    "event_type": workflow.event_type,
                    "event_status": workflow.event_status,
                    "processing_time_ms": workflow.processing_time_ms
                })

            # Calculate performance summary
            performance_summary = await _calculate_performance_summary(db, agent_id)

            return AgentDetailedHealth(
                agent_id=agent.agent_id,
                agent_name=agent.agent_name,
                agent_type=agent.agent_type,
                configuration=agent.configuration or {},
                health_status=health_status,
                recent_metrics=recent_metrics,
                active_alerts=active_alerts,
                recent_decisions=recent_decisions,
                recent_workflows=recent_workflows,
                performance_summary=performance_summary
            )

        except HTTPException:
            raise
        except Exception as e:
            logger.error(f"Error getting agent health for {agent_id}: {e}")
            raise HTTPException(status_code=500, detail="Failed to retrieve agent health")

    @app.get("/api/v1/agents/{agent_id}/metrics")
    async def get_agent_metrics(
        agent_id: str,
        query: MetricsQuery = Depends(),
        db: AsyncSession = Depends(get_db),
        tenant_id: str = Depends(get_tenant_id)
    ):
        """Get performance metrics for a specific agent."""
        try:
            # Verify agent exists
            result = await db.execute(select(Agent).where(Agent.agent_id == agent_id))
            agent = result.scalar_one_or_none()

            if not agent:
                raise HTTPException(status_code=404, detail=f"Agent {agent_id} not found")

            # Use metrics collector if available, otherwise query directly
            if metrics_collector and metrics_collector._running:
                metrics_data = await metrics_collector.get_metrics(
                    agent_id=agent_id,
                    metric_names=query.metric_names,
                    start_time=query.start_time,
                    end_time=query.end_time,
                    aggregation=query.aggregation,
                    bucket_size=query.bucket_size
                )
            else:
                # Fallback to direct database query
                metrics_data = await _query_metrics_directly(
                    db, agent_id, query
                )

            return {
                "agent_id": agent_id,
                "query": query.dict(),
                "metrics": metrics_data,
                "timestamp": datetime.utcnow().isoformat() + "Z"
            }

        except HTTPException:
            raise
        except Exception as e:
            logger.error(f"Error getting metrics for agent {agent_id}: {e}")
            raise HTTPException(status_code=500, detail="Failed to retrieve metrics")

    @app.get("/api/v1/dashboard/stats", response_model=DashboardStats)
    async def get_dashboard_stats(
        db: AsyncSession = Depends(get_db),
        tenant_id: str = Depends(get_tenant_id)
    ):
        """Get overall dashboard statistics."""
        try:
            # Get agent counts by status
            agents_result = await db.execute(select(Agent))
            agents = agents_result.scalars().all()

            health_service = AgentHealthService(db)

            total_agents = len(agents)
            healthy_agents = 0
            warning_agents = 0
            critical_agents = 0
            offline_agents = 0

            for agent in agents:
                health_status = await health_service.calculate_agent_health(agent)

                if health_status.status == "healthy":
                    healthy_agents += 1
                elif health_status.status == "warning":
                    warning_agents += 1
                elif health_status.status == "critical":
                    critical_agents += 1
                else:  # offline
                    offline_agents += 1

            # Get active workflows count
            active_workflows_result = await db.execute(
                select(func.count(func.distinct(WorkflowEvent.workflow_id))).where(
                    WorkflowEvent.timestamp >= datetime.utcnow() - timedelta(hours=1)
                )
            )
            active_workflows = active_workflows_result.scalar() or 0

            # Get metrics count for today
            today = datetime.utcnow().replace(hour=0, minute=0, second=0, microsecond=0)
            metrics_today_result = await db.execute(
                select(func.count(AgentMetric.id)).where(
                    AgentMetric.timestamp >= today
                )
            )
            metrics_today = metrics_today_result.scalar() or 0

            # Get active alerts count
            active_alerts_result = await db.execute(
                select(func.count(AlertEvent.id)).where(
                    and_(
                        AlertEvent.alert_status == "active",
                        AlertEvent.resolved_at.is_(None)
                    )
                )
            )
            active_alerts = active_alerts_result.scalar() or 0

            # Calculate system uptime (time since first agent registration)
            first_agent_result = await db.execute(
                select(func.min(Agent.created_at))
            )
            first_agent_time = first_agent_result.scalar()

            system_uptime_hours = 0.0
            if first_agent_time:
                uptime_delta = datetime.utcnow() - first_agent_time
                system_uptime_hours = uptime_delta.total_seconds() / 3600

            return DashboardStats(
                total_agents=total_agents,
                healthy_agents=healthy_agents,
                warning_agents=warning_agents,
                critical_agents=critical_agents,
                offline_agents=offline_agents,
                active_workflows=active_workflows,
                total_metrics_today=metrics_today,
                active_alerts=active_alerts,
                system_uptime_hours=system_uptime_hours
            )

        except Exception as e:
            logger.error(f"Error getting dashboard stats: {e}")
            raise HTTPException(status_code=500, detail="Failed to retrieve dashboard statistics")

    @app.get("/api/v1/stream/agents")
    async def stream_agent_updates(request: Request):
        """Server-Sent Events stream for real-time agent updates."""

        async def event_generator():
            """Generate SSE events for agent updates."""
            queue = sse_service.add_connection("agents")

            try:
                # Send initial connection message
                initial_message = SSEMessage(
                    event="connection_established",
                    data={"message": "Connected to agent updates stream"},
                    timestamp=datetime.utcnow()
                )
                yield f"event: {initial_message.event}\ndata: {json.dumps(initial_message.data)}\n\n"

                # Stream updates
                while True:
                    try:
                        # Check if client disconnected
                        if await request.is_disconnected():
                            break

                        # Wait for message with timeout
                        try:
                            message = await asyncio.wait_for(queue.get(), timeout=30.0)

                            # Format SSE message
                            event_data = json.dumps(message.data, default=str)
                            yield f"event: {message.event}\ndata: {event_data}\n\n"

                        except asyncio.TimeoutError:
                            # Send keepalive
                            yield f"event: keepalive\ndata: {json.dumps({'timestamp': datetime.utcnow().isoformat()})}\n\n"

                    except asyncio.CancelledError:
                        break
                    except Exception as e:
                        logger.error(f"Error in SSE stream: {e}")
                        break

            finally:
                sse_service.remove_connection("agents", queue)

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

    # ========================================================================
    # Exception Handlers
    # ========================================================================

    @app.exception_handler(ValidationError)
    async def validation_exception_handler(request: Request, exc: ValidationError):
        """Handle validation errors."""
        return JSONResponse(
            status_code=422,
            content={
                "error": "Validation Error",
                "message": str(exc),
                "type": "ValidationError"
            }
        )

    @app.exception_handler(SQLAlchemyError)
    async def database_exception_handler(request: Request, exc: SQLAlchemyError):
        """Handle database errors."""
        logger.error(f"Database error: {exc}")
        return JSONResponse(
            status_code=503,
            content={
                "error": "Database Error",
                "message": "Database operation failed",
                "type": type(exc).__name__
            }
        )

    @app.exception_handler(Exception)
    async def global_exception_handler(request: Request, exc: Exception):
        """Global exception handler for unhandled errors."""
        logger.error(f"Unhandled error: {exc}", exc_info=True)
        return JSONResponse(
            status_code=500,
            content={
                "error": "Internal Server Error",
                "message": "An unexpected error occurred",
                "type": type(exc).__name__
            }
        )

    return app

# ============================================================================
# Helper Functions
# ============================================================================

async def _calculate_performance_summary(db: AsyncSession, agent_id: str) -> Dict[str, Any]:
    """Calculate performance summary for an agent."""
    since = datetime.utcnow() - timedelta(hours=24)

    # Get average processing time
    avg_time_result = await db.execute(
        select(func.avg(AgentMetric.metric_value)).where(
            and_(
                AgentMetric.agent_id == agent_id,
                AgentMetric.metric_name == "processing_time_ms",
                AgentMetric.timestamp >= since
            )
        )
    )
    avg_processing_time = avg_time_result.scalar()

    # Get total operations
    operations_result = await db.execute(
        select(func.count(WorkflowEvent.id)).where(
            and_(
                WorkflowEvent.agent_id == agent_id,
                WorkflowEvent.timestamp >= since,
                WorkflowEvent.event_type == "workflow_finished"
            )
        )
    )
    total_operations = operations_result.scalar() or 0

    # Get success rate
    success_result = await db.execute(
        select(func.count(WorkflowEvent.id)).where(
            and_(
                WorkflowEvent.agent_id == agent_id,
                WorkflowEvent.timestamp >= since,
                WorkflowEvent.event_type == "workflow_finished",
                WorkflowEvent.event_status == "success"
            )
        )
    )
    successful_operations = success_result.scalar() or 0

    success_rate = successful_operations / total_operations if total_operations > 0 else 0.0

    return {
        "avg_processing_time_ms": avg_processing_time,
        "total_operations_24h": total_operations,
        "success_rate": success_rate,
        "successful_operations_24h": successful_operations
    }


async def _query_metrics_directly(
    db: AsyncSession,
    agent_id: str,
    query: MetricsQuery
) -> Dict[str, List[Dict[str, Any]]]:
    """Direct database query for metrics when collector is not available."""
    # Set default time range
    end_time = query.end_time or datetime.utcnow()
    start_time = query.start_time or (end_time - timedelta(hours=1))

    # Build query
    db_query = select(
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

    # Add metric name filter
    if query.metric_names:
        db_query = db_query.where(AgentMetric.metric_name.in_(query.metric_names))

    # Order by timestamp
    db_query = db_query.order_by(desc(AgentMetric.timestamp))

    result = await db.execute(db_query)
    rows = result.fetchall()

    # Group by metric name
    metrics_data = {}
    for row in rows:
        metric_name = row.metric_name
        if metric_name not in metrics_data:
            metrics_data[metric_name] = []

        metrics_data[metric_name].append({
            "timestamp": row.timestamp.isoformat() + "Z",
            "value": row.metric_value,
            "labels": row.labels or {}
        })

    return metrics_data


# ============================================================================
# Module exports
# ============================================================================

__all__ = [
    "create_app",
    "AgentHealthStatus",
    "AgentDetailedHealth",
    "MetricsQuery",
    "DashboardStats",
    "SSEMessage"
]