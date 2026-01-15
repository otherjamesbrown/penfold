"""API module for Penfold observability framework.

This module provides FastAPI routers and endpoints for observability
data access, real-time monitoring, and dashboard integration.

Features:
- RESTful API endpoints for workflow and agent monitoring
- Real-time Server-Sent Events (SSE) streaming
- Comprehensive query and filtering capabilities
- Performance analysis and bottleneck identification
- Tenant isolation and authentication support

Modules:
    workflow_endpoints: Workflow tracing and visualization endpoints

Example Usage:
    from fastapi import FastAPI
    from observability_lib.api.workflow_endpoints import router as workflow_router

    app = FastAPI()
    app.include_router(workflow_router, prefix="/api/v1", tags=["workflows"])
"""

from observability_lib.api.workflow_endpoints import (
    router as workflow_router,
    initialize_workflow_endpoints,
    broadcast_workflow_update,
    WorkflowSummary,
    WorkflowDetail,
    WorkflowTrace,
    WorkflowAnalysis,
    WorkflowFailureAnalysis,
    BottleneckAnalysis,
    WorkflowSearchRequest,
    WorkflowStatus
)

from observability_lib.api.business_endpoints import (
    router as business_router,
    initialize_business_endpoints,
)

__all__ = [
    "workflow_router",
    "business_router",
    "initialize_workflow_endpoints",
    "initialize_business_endpoints",
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