#!/usr/bin/env python3
"""
Alert management API endpoints for Penfold observability framework.

This module provides REST API endpoints for alert management, system monitoring,
and proactive alerting capabilities.
"""

import asyncio
import json
import logging
from datetime import datetime, timedelta
from typing import List, Optional, Dict, Any

from fastapi import APIRouter, HTTPException, Depends, BackgroundTasks
from fastapi.responses import StreamingResponse, JSONResponse
from pydantic import BaseModel, Field
from sse_starlette.sse import EventSourceResponse

from observability_lib.config import get_settings
from observability_lib.services.alert_manager import (
    AlertManagerService, AlertRule, AlertInstance, AlertPattern,
    AlertSeverity, AlertCategory, AlertStatus, DEFAULT_ALERT_RULES
)
from observability_lib.services.system_monitor import (
    SystemMonitorService, ResourceUsage, SystemSnapshot, ResourceAlert, ResourceType
)

logger = logging.getLogger(__name__)

# Initialize services
alert_manager = None
system_monitor = None

# API Models
class AlertRuleRequest(BaseModel):
    """Request model for creating alert rules."""
    name: str = Field(..., description="Unique name for the alert rule")
    category: str = Field(..., description="Alert category (performance, error_rate, etc.)")
    severity: str = Field(..., description="Alert severity (critical, high, medium, low)")
    metric_name: str = Field(..., description="Metric to monitor")
    operator: str = Field(..., description="Comparison operator (>, <, >=, <=, ==, !=)")
    threshold_value: float = Field(..., description="Threshold value to trigger alert")
    duration_minutes: int = Field(5, description="Duration in minutes to evaluate threshold")
    agent_pattern: Optional[str] = Field(None, description="Agent ID pattern (wildcards supported)")
    tenant_id: Optional[str] = Field(None, description="Tenant ID for multi-tenant setups")
    enabled: bool = Field(True, description="Whether the rule is enabled")


class AlertRuleResponse(BaseModel):
    """Response model for alert rules."""
    name: str
    category: str
    severity: str
    metric_name: str
    operator: str
    threshold_value: float
    duration_minutes: int
    agent_pattern: Optional[str]
    tenant_id: Optional[str]
    enabled: bool


class AlertInstanceResponse(BaseModel):
    """Response model for alert instances."""
    alert_id: str
    rule_name: str
    agent_id: str
    severity: str
    category: str
    status: str
    title: str
    description: str
    metric_value: float
    threshold_value: float
    triggered_at: datetime
    acknowledged_at: Optional[datetime]
    resolved_at: Optional[datetime]
    tenant_id: Optional[str]


class AlertPatternResponse(BaseModel):
    """Response model for alert patterns."""
    pattern_type: str
    frequency: int
    affected_agents: List[str]
    description: str
    severity: str
    first_occurrence: datetime
    last_occurrence: datetime
    recommendation: str


class SystemSnapshotResponse(BaseModel):
    """Response model for system snapshots."""
    timestamp: datetime
    cpu_usage_percent: float
    memory_usage_percent: float
    memory_usage_mb: float
    disk_usage_percent: float
    disk_usage_gb: float
    network_io_mb: float
    process_count: int
    agent_processes: List[Dict[str, Any]]
    tenant_id: Optional[str]


class ResourceUsageResponse(BaseModel):
    """Response model for resource usage."""
    resource_type: str
    agent_id: Optional[str]
    usage_value: float
    usage_percent: float
    timestamp: datetime
    tenant_id: Optional[str]
    metadata: Optional[Dict[str, Any]]


class AlertAcknowledgeRequest(BaseModel):
    """Request model for acknowledging alerts."""
    acknowledged_by: str = Field(..., description="User who acknowledged the alert")


class AlertResolveRequest(BaseModel):
    """Request model for resolving alerts."""
    resolved_by: str = Field(..., description="User who resolved the alert")
    resolution_note: str = Field(..., description="Note about resolution")


# Create router
router = APIRouter(prefix="/api/v1/alerts", tags=["alerts"])


async def get_alert_manager() -> AlertManagerService:
    """Dependency to get alert manager service."""
    global alert_manager
    if alert_manager is None:
        settings = get_settings()
        alert_manager = AlertManagerService(settings)
        await alert_manager.start()
    return alert_manager


async def get_system_monitor() -> SystemMonitorService:
    """Dependency to get system monitor service."""
    global system_monitor
    if system_monitor is None:
        settings = get_settings()
        system_monitor = SystemMonitorService(settings)
        await system_monitor.start()
    return system_monitor


# Alert Rules Endpoints
@router.post("/rules", response_model=AlertRuleResponse)
async def create_alert_rule(
    rule_request: AlertRuleRequest,
    alert_mgr: AlertManagerService = Depends(get_alert_manager)
):
    """Create new alert rule."""
    try:
        # Convert request to AlertRule
        rule = AlertRule(
            name=rule_request.name,
            category=AlertCategory(rule_request.category),
            severity=AlertSeverity(rule_request.severity),
            metric_name=rule_request.metric_name,
            operator=rule_request.operator,
            threshold_value=rule_request.threshold_value,
            duration_minutes=rule_request.duration_minutes,
            agent_pattern=rule_request.agent_pattern,
            tenant_id=rule_request.tenant_id,
            enabled=rule_request.enabled
        )

        success = await alert_mgr.add_alert_rule(rule)
        if not success:
            raise HTTPException(status_code=400, detail="Failed to create alert rule")

        return AlertRuleResponse(
            name=rule.name,
            category=rule.category.value,
            severity=rule.severity.value,
            metric_name=rule.metric_name,
            operator=rule.operator,
            threshold_value=rule.threshold_value,
            duration_minutes=rule.duration_minutes,
            agent_pattern=rule.agent_pattern,
            tenant_id=rule.tenant_id,
            enabled=rule.enabled
        )

    except ValueError as e:
        raise HTTPException(status_code=400, detail=f"Invalid parameter: {e}")
    except Exception as e:
        logger.error(f"Error creating alert rule: {e}")
        raise HTTPException(status_code=500, detail="Internal server error")


@router.get("/rules", response_model=List[AlertRuleResponse])
async def get_alert_rules(
    tenant_id: Optional[str] = None,
    alert_mgr: AlertManagerService = Depends(get_alert_manager)
):
    """Get all alert rules."""
    try:
        rules = await alert_mgr.get_alert_rules(tenant_id)

        return [
            AlertRuleResponse(
                name=rule.name,
                category=rule.category.value,
                severity=rule.severity.value,
                metric_name=rule.metric_name,
                operator=rule.operator,
                threshold_value=rule.threshold_value,
                duration_minutes=rule.duration_minutes,
                agent_pattern=rule.agent_pattern,
                tenant_id=rule.tenant_id,
                enabled=rule.enabled
            )
            for rule in rules
        ]

    except Exception as e:
        logger.error(f"Error getting alert rules: {e}")
        raise HTTPException(status_code=500, detail="Internal server error")


@router.delete("/rules/{rule_name}")
async def delete_alert_rule(
    rule_name: str,
    tenant_id: Optional[str] = None,
    alert_mgr: AlertManagerService = Depends(get_alert_manager)
):
    """Delete alert rule."""
    try:
        success = await alert_mgr.remove_alert_rule(rule_name, tenant_id)
        if not success:
            raise HTTPException(status_code=404, detail="Alert rule not found")

        return {"message": f"Alert rule '{rule_name}' deleted successfully"}

    except Exception as e:
        logger.error(f"Error deleting alert rule: {e}")
        raise HTTPException(status_code=500, detail="Internal server error")


# Alert Instances Endpoints
@router.get("/active", response_model=List[AlertInstanceResponse])
async def get_active_alerts(
    tenant_id: Optional[str] = None,
    severity: Optional[str] = None,
    agent_id: Optional[str] = None,
    alert_mgr: AlertManagerService = Depends(get_alert_manager)
):
    """Get active alerts with optional filters."""
    try:
        severity_filter = AlertSeverity(severity) if severity else None
        alerts = await alert_mgr.get_active_alerts(tenant_id, severity_filter, agent_id)

        return [
            AlertInstanceResponse(
                alert_id=alert.alert_id,
                rule_name=alert.rule_name,
                agent_id=alert.agent_id,
                severity=alert.severity.value,
                category=alert.category.value,
                status=alert.status.value,
                title=alert.title,
                description=alert.description,
                metric_value=alert.metric_value,
                threshold_value=alert.threshold_value,
                triggered_at=alert.triggered_at,
                acknowledged_at=alert.acknowledged_at,
                resolved_at=alert.resolved_at,
                tenant_id=alert.tenant_id
            )
            for alert in alerts
        ]

    except ValueError as e:
        raise HTTPException(status_code=400, detail=f"Invalid parameter: {e}")
    except Exception as e:
        logger.error(f"Error getting active alerts: {e}")
        raise HTTPException(status_code=500, detail="Internal server error")


@router.post("/acknowledge/{alert_id}")
async def acknowledge_alert(
    alert_id: str,
    request: AlertAcknowledgeRequest,
    tenant_id: Optional[str] = None,
    alert_mgr: AlertManagerService = Depends(get_alert_manager)
):
    """Acknowledge active alert."""
    try:
        success = await alert_mgr.acknowledge_alert(
            alert_id, request.acknowledged_by, tenant_id
        )
        if not success:
            raise HTTPException(status_code=404, detail="Alert not found or already acknowledged")

        return {"message": f"Alert '{alert_id}' acknowledged successfully"}

    except Exception as e:
        logger.error(f"Error acknowledging alert: {e}")
        raise HTTPException(status_code=500, detail="Internal server error")


@router.post("/resolve/{alert_id}")
async def resolve_alert(
    alert_id: str,
    request: AlertResolveRequest,
    tenant_id: Optional[str] = None,
    alert_mgr: AlertManagerService = Depends(get_alert_manager)
):
    """Resolve active alert."""
    try:
        success = await alert_mgr.resolve_alert(
            alert_id, request.resolved_by, request.resolution_note, tenant_id
        )
        if not success:
            raise HTTPException(status_code=404, detail="Alert not found or already resolved")

        return {"message": f"Alert '{alert_id}' resolved successfully"}

    except Exception as e:
        logger.error(f"Error resolving alert: {e}")
        raise HTTPException(status_code=500, detail="Internal server error")


# Alert Patterns Endpoints
@router.get("/patterns", response_model=List[AlertPatternResponse])
async def get_alert_patterns(
    hours_back: int = 24,
    tenant_id: Optional[str] = None,
    alert_mgr: AlertManagerService = Depends(get_alert_manager)
):
    """Get detected alert patterns."""
    try:
        patterns = await alert_mgr.analyze_alert_patterns(hours_back, tenant_id)

        return [
            AlertPatternResponse(
                pattern_type=pattern.pattern_type,
                frequency=pattern.frequency,
                affected_agents=pattern.affected_agents,
                description=pattern.description,
                severity=pattern.severity.value,
                first_occurrence=pattern.first_occurrence,
                last_occurrence=pattern.last_occurrence,
                recommendation=pattern.recommendation
            )
            for pattern in patterns
        ]

    except Exception as e:
        logger.error(f"Error getting alert patterns: {e}")
        raise HTTPException(status_code=500, detail="Internal server error")


# System Monitor Endpoints
@router.get("/system/snapshot", response_model=SystemSnapshotResponse)
async def get_system_snapshot(
    tenant_id: Optional[str] = None,
    monitor: SystemMonitorService = Depends(get_system_monitor)
):
    """Get current system resource snapshot."""
    try:
        snapshot = await monitor.get_system_snapshot(tenant_id)

        return SystemSnapshotResponse(
            timestamp=snapshot.timestamp,
            cpu_usage_percent=snapshot.cpu_usage_percent,
            memory_usage_percent=snapshot.memory_usage_percent,
            memory_usage_mb=snapshot.memory_usage_mb,
            disk_usage_percent=snapshot.disk_usage_percent,
            disk_usage_gb=snapshot.disk_usage_gb,
            network_io_mb=snapshot.network_io_mb,
            process_count=snapshot.process_count,
            agent_processes=snapshot.agent_processes,
            tenant_id=snapshot.tenant_id
        )

    except Exception as e:
        logger.error(f"Error getting system snapshot: {e}")
        raise HTTPException(status_code=500, detail="Internal server error")


@router.get("/system/resources/{agent_id}", response_model=List[ResourceUsageResponse])
async def get_agent_resource_usage(
    agent_id: str,
    hours_back: int = 1,
    tenant_id: Optional[str] = None,
    monitor: SystemMonitorService = Depends(get_system_monitor)
):
    """Get resource usage history for specific agent."""
    try:
        usage_history = await monitor.get_agent_resource_usage(agent_id, hours_back, tenant_id)

        return [
            ResourceUsageResponse(
                resource_type=usage.resource_type.value,
                agent_id=usage.agent_id,
                usage_value=usage.usage_value,
                usage_percent=usage.usage_percent,
                timestamp=usage.timestamp,
                tenant_id=usage.tenant_id,
                metadata=usage.metadata
            )
            for usage in usage_history
        ]

    except Exception as e:
        logger.error(f"Error getting agent resource usage: {e}")
        raise HTTPException(status_code=500, detail="Internal server error")


@router.get("/system/top-consumers/{resource_type}")
async def get_top_resource_consumers(
    resource_type: str,
    limit: int = 10,
    hours_back: int = 1,
    tenant_id: Optional[str] = None,
    monitor: SystemMonitorService = Depends(get_system_monitor)
):
    """Get top resource consumers by agent."""
    try:
        resource_enum = ResourceType(resource_type)
        consumers = await monitor.get_resource_top_consumers(
            resource_enum, limit, hours_back, tenant_id
        )

        return consumers

    except ValueError as e:
        raise HTTPException(status_code=400, detail=f"Invalid resource type: {e}")
    except Exception as e:
        logger.error(f"Error getting top resource consumers: {e}")
        raise HTTPException(status_code=500, detail="Internal server error")


@router.get("/system/trends")
async def get_system_trends(
    hours_back: int = 24,
    tenant_id: Optional[str] = None,
    monitor: SystemMonitorService = Depends(get_system_monitor)
):
    """Get system-wide resource usage trends."""
    try:
        trends = await monitor.get_system_resource_trends(hours_back, tenant_id)
        return trends

    except Exception as e:
        logger.error(f"Error getting system trends: {e}")
        raise HTTPException(status_code=500, detail="Internal server error")


# Real-time streaming endpoints
@router.get("/stream/alerts")
async def stream_alerts(
    tenant_id: Optional[str] = None,
    alert_mgr: AlertManagerService = Depends(get_alert_manager)
):
    """Stream real-time alert updates via Server-Sent Events."""

    async def event_stream():
        """Generate alert events for streaming."""
        last_check = datetime.utcnow()

        while True:
            try:
                # Get new alerts since last check
                alerts = await alert_mgr.get_active_alerts(tenant_id)
                recent_alerts = [
                    alert for alert in alerts
                    if alert.triggered_at > last_check
                ]

                for alert in recent_alerts:
                    alert_data = {
                        "alert_id": alert.alert_id,
                        "rule_name": alert.rule_name,
                        "agent_id": alert.agent_id,
                        "severity": alert.severity.value,
                        "category": alert.category.value,
                        "title": alert.title,
                        "description": alert.description,
                        "triggered_at": alert.triggered_at.isoformat()
                    }

                    yield {
                        "event": "alert_triggered",
                        "data": json.dumps(alert_data)
                    }

                last_check = datetime.utcnow()
                await asyncio.sleep(5)  # Check every 5 seconds

            except Exception as e:
                logger.error(f"Error in alert stream: {e}")
                yield {
                    "event": "error",
                    "data": json.dumps({"error": str(e)})
                }
                await asyncio.sleep(10)

    return EventSourceResponse(event_stream())


@router.get("/stream/system")
async def stream_system_metrics(
    tenant_id: Optional[str] = None,
    monitor: SystemMonitorService = Depends(get_system_monitor)
):
    """Stream real-time system metrics via Server-Sent Events."""

    async def event_stream():
        """Generate system metric events for streaming."""
        while True:
            try:
                snapshot = await monitor.get_system_snapshot(tenant_id)

                snapshot_data = {
                    "timestamp": snapshot.timestamp.isoformat(),
                    "cpu_usage_percent": snapshot.cpu_usage_percent,
                    "memory_usage_percent": snapshot.memory_usage_percent,
                    "disk_usage_percent": snapshot.disk_usage_percent,
                    "process_count": snapshot.process_count,
                    "agent_process_count": len([p for p in snapshot.agent_processes if p.get('agent_id')])
                }

                yield {
                    "event": "system_metrics",
                    "data": json.dumps(snapshot_data)
                }

                await asyncio.sleep(10)  # Update every 10 seconds

            except Exception as e:
                logger.error(f"Error in system metrics stream: {e}")
                yield {
                    "event": "error",
                    "data": json.dumps({"error": str(e)})
                }
                await asyncio.sleep(15)

    return EventSourceResponse(event_stream())


# Utility endpoints
@router.post("/setup/default-rules")
async def setup_default_rules(
    tenant_id: Optional[str] = None,
    alert_mgr: AlertManagerService = Depends(get_alert_manager)
):
    """Setup default alert rules for quick start."""
    try:
        created_rules = []

        for default_rule in DEFAULT_ALERT_RULES:
            # Customize for tenant if provided
            rule = AlertRule(
                name=default_rule.name,
                category=default_rule.category,
                severity=default_rule.severity,
                metric_name=default_rule.metric_name,
                operator=default_rule.operator,
                threshold_value=default_rule.threshold_value,
                duration_minutes=default_rule.duration_minutes,
                agent_pattern=default_rule.agent_pattern,
                tenant_id=tenant_id,
                enabled=default_rule.enabled
            )

            success = await alert_mgr.add_alert_rule(rule)
            if success:
                created_rules.append(rule.name)

        return {
            "message": f"Created {len(created_rules)} default alert rules",
            "rules": created_rules
        }

    except Exception as e:
        logger.error(f"Error setting up default rules: {e}")
        raise HTTPException(status_code=500, detail="Internal server error")


@router.get("/health")
async def health_check():
    """Health check endpoint for alert services."""
    try:
        global alert_manager, system_monitor

        status = {
            "alert_manager": "healthy" if alert_manager else "not_initialized",
            "system_monitor": "healthy" if system_monitor else "not_initialized",
            "timestamp": datetime.utcnow().isoformat()
        }

        return status

    except Exception as e:
        logger.error(f"Health check error: {e}")
        raise HTTPException(status_code=500, detail="Health check failed")