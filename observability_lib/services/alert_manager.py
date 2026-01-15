#!/usr/bin/env python3
"""
Alert manager service for Penfold observability framework.

This service provides comprehensive alert management including threshold configuration,
alert generation, pattern analysis, and resolution tracking for proactive monitoring.
"""

import asyncio
import json
import logging
from datetime import datetime, timedelta
from enum import Enum
from typing import Dict, List, Optional, Any, Set
from dataclasses import dataclass

import asyncpg
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, and_, or_, func, desc, asc
from sqlalchemy.orm import selectinload

from observability_lib.config import ObservabilitySettings
from observability_lib.models.agent_metrics import (
    AlertThreshold, AlertEvent, Agent, AgentMetric, AgentLog, WorkflowEvent
)

logger = logging.getLogger(__name__)


class AlertSeverity(Enum):
    """Alert severity levels."""
    CRITICAL = "critical"
    HIGH = "high"
    MEDIUM = "medium"
    LOW = "low"
    INFO = "info"


class AlertCategory(Enum):
    """Alert categories for classification."""
    PERFORMANCE = "performance"
    ERROR_RATE = "error_rate"
    HEALTH_SCORE = "health_score"
    RESOURCE_USAGE = "resource_usage"
    WORKFLOW = "workflow"
    QUALITY = "quality"
    BUSINESS_KPI = "business_kpi"


class AlertStatus(Enum):
    """Alert status tracking."""
    ACTIVE = "active"
    ACKNOWLEDGED = "acknowledged"
    RESOLVED = "resolved"
    SUPPRESSED = "suppressed"


@dataclass
class AlertRule:
    """Alert rule configuration."""
    name: str
    category: AlertCategory
    severity: AlertSeverity
    metric_name: str
    operator: str  # >, <, >=, <=, ==, !=
    threshold_value: float
    duration_minutes: int
    agent_pattern: Optional[str] = None
    tenant_id: Optional[str] = None
    enabled: bool = True


@dataclass
class AlertInstance:
    """Active alert instance."""
    alert_id: str
    rule_name: str
    agent_id: str
    severity: AlertSeverity
    category: AlertCategory
    status: AlertStatus
    title: str
    description: str
    metric_value: float
    threshold_value: float
    triggered_at: datetime
    acknowledged_at: Optional[datetime] = None
    resolved_at: Optional[datetime] = None
    tenant_id: Optional[str] = None


@dataclass
class AlertPattern:
    """Detected alert pattern."""
    pattern_type: str
    frequency: int
    affected_agents: List[str]
    description: str
    severity: AlertSeverity
    first_occurrence: datetime
    last_occurrence: datetime
    recommendation: str


class AlertManagerService:
    """Service for managing alerts, thresholds, and monitoring."""

    def __init__(self, settings: ObservabilitySettings):
        self.settings = settings
        self.db_pool: Optional[asyncpg.Pool] = None
        self._alert_rules: Dict[str, AlertRule] = {}
        self._active_alerts: Dict[str, AlertInstance] = {}
        self._alert_patterns: List[AlertPattern] = []
        self._monitoring_task: Optional[asyncio.Task] = None

    async def start(self):
        """Initialize alert manager service."""
        try:
            # Create database connection pool
            self.db_pool = await asyncpg.create_pool(
                self.settings.database_url,
                min_size=2,
                max_size=10,
                command_timeout=30
            )

            # Load existing alert rules
            await self._load_alert_rules()

            # Start monitoring task
            self._monitoring_task = asyncio.create_task(self._monitor_loop())

            logger.info("Alert manager service started")

        except Exception as e:
            logger.error(f"Failed to start alert manager: {e}")
            raise

    async def stop(self):
        """Stop alert manager service."""
        if self._monitoring_task:
            self._monitoring_task.cancel()
            try:
                await self._monitoring_task
            except asyncio.CancelledError:
                pass

        if self.db_pool:
            await self.db_pool.close()

        logger.info("Alert manager service stopped")

    async def add_alert_rule(self, rule: AlertRule) -> bool:
        """Add new alert rule."""
        try:
            # Store in database
            async with self.db_pool.acquire() as conn:
                await conn.execute("""
                    INSERT INTO alert_thresholds (
                        agent_id, metric_name, operator, threshold_value,
                        severity, duration_minutes, enabled, tenant_id,
                        rule_name, category, agent_pattern
                    ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
                    ON CONFLICT (rule_name, tenant_id) DO UPDATE SET
                        metric_name = $2, operator = $3, threshold_value = $4,
                        severity = $5, duration_minutes = $6, enabled = $7,
                        agent_pattern = $11, updated_at = NOW()
                """,
                rule.agent_pattern or "*",
                rule.metric_name,
                rule.operator,
                rule.threshold_value,
                rule.severity.value,
                rule.duration_minutes,
                rule.enabled,
                rule.tenant_id,
                rule.name,
                rule.category.value,
                rule.agent_pattern
                )

            # Update local cache
            self._alert_rules[rule.name] = rule

            logger.info(f"Added alert rule: {rule.name}")
            return True

        except Exception as e:
            logger.error(f"Failed to add alert rule {rule.name}: {e}")
            return False

    async def remove_alert_rule(self, rule_name: str, tenant_id: Optional[str] = None) -> bool:
        """Remove alert rule."""
        try:
            async with self.db_pool.acquire() as conn:
                result = await conn.execute("""
                    DELETE FROM alert_thresholds
                    WHERE rule_name = $1 AND ($2::text IS NULL OR tenant_id = $2)
                """, rule_name, tenant_id)

            if result == "DELETE 1":
                self._alert_rules.pop(rule_name, None)
                logger.info(f"Removed alert rule: {rule_name}")
                return True
            else:
                logger.warning(f"Alert rule not found: {rule_name}")
                return False

        except Exception as e:
            logger.error(f"Failed to remove alert rule {rule_name}: {e}")
            return False

    async def get_alert_rules(self, tenant_id: Optional[str] = None) -> List[AlertRule]:
        """Get all alert rules."""
        try:
            async with self.db_pool.acquire() as conn:
                rows = await conn.fetch("""
                    SELECT rule_name, category, severity, metric_name, operator,
                           threshold_value, duration_minutes, agent_pattern,
                           tenant_id, enabled
                    FROM alert_thresholds
                    WHERE ($1::text IS NULL OR tenant_id = $1)
                    ORDER BY rule_name
                """, tenant_id)

            rules = []
            for row in rows:
                rule = AlertRule(
                    name=row['rule_name'],
                    category=AlertCategory(row['category']),
                    severity=AlertSeverity(row['severity']),
                    metric_name=row['metric_name'],
                    operator=row['operator'],
                    threshold_value=row['threshold_value'],
                    duration_minutes=row['duration_minutes'],
                    agent_pattern=row['agent_pattern'],
                    tenant_id=row['tenant_id'],
                    enabled=row['enabled']
                )
                rules.append(rule)

            return rules

        except Exception as e:
            logger.error(f"Failed to get alert rules: {e}")
            return []

    async def trigger_alert(
        self,
        rule: AlertRule,
        agent_id: str,
        metric_value: float,
        context: Dict[str, Any]
    ) -> Optional[AlertInstance]:
        """Trigger new alert instance."""
        try:
            alert_id = f"{rule.name}_{agent_id}_{int(datetime.utcnow().timestamp())}"

            # Create alert description
            description = f"Agent {agent_id} {rule.metric_name} is {metric_value:.2f}, "
            description += f"which {rule.operator} threshold {rule.threshold_value:.2f}"

            alert = AlertInstance(
                alert_id=alert_id,
                rule_name=rule.name,
                agent_id=agent_id,
                severity=rule.severity,
                category=rule.category,
                status=AlertStatus.ACTIVE,
                title=f"{rule.category.value.title()} Alert: {agent_id}",
                description=description,
                metric_value=metric_value,
                threshold_value=rule.threshold_value,
                triggered_at=datetime.utcnow(),
                tenant_id=rule.tenant_id
            )

            # Store in database
            async with self.db_pool.acquire() as conn:
                await conn.execute("""
                    INSERT INTO alert_events (
                        alert_id, rule_name, agent_id, alert_type, severity,
                        status, message, metric_value, threshold_value,
                        triggered_at, tenant_id, context
                    ) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)
                """,
                alert_id,
                rule.name,
                agent_id,
                rule.category.value,
                rule.severity.value,
                AlertStatus.ACTIVE.value,
                description,
                metric_value,
                rule.threshold_value,
                alert.triggered_at,
                rule.tenant_id,
                json.dumps(context)
                )

            # Update local cache
            self._active_alerts[alert_id] = alert

            logger.warning(f"Alert triggered: {alert.title} - {alert.description}")
            return alert

        except Exception as e:
            logger.error(f"Failed to trigger alert for rule {rule.name}: {e}")
            return None

    async def acknowledge_alert(
        self,
        alert_id: str,
        acknowledged_by: str,
        tenant_id: Optional[str] = None
    ) -> bool:
        """Acknowledge active alert."""
        try:
            async with self.db_pool.acquire() as conn:
                result = await conn.execute("""
                    UPDATE alert_events
                    SET status = 'acknowledged', acknowledged_at = NOW(),
                        acknowledged_by = $2
                    WHERE alert_id = $1
                    AND ($3::text IS NULL OR tenant_id = $3)
                    AND status = 'active'
                """, alert_id, acknowledged_by, tenant_id)

            if result == "UPDATE 1":
                if alert_id in self._active_alerts:
                    self._active_alerts[alert_id].status = AlertStatus.ACKNOWLEDGED
                    self._active_alerts[alert_id].acknowledged_at = datetime.utcnow()

                logger.info(f"Alert acknowledged: {alert_id} by {acknowledged_by}")
                return True
            else:
                logger.warning(f"Alert not found or already acknowledged: {alert_id}")
                return False

        except Exception as e:
            logger.error(f"Failed to acknowledge alert {alert_id}: {e}")
            return False

    async def resolve_alert(
        self,
        alert_id: str,
        resolved_by: str,
        resolution_note: str,
        tenant_id: Optional[str] = None
    ) -> bool:
        """Resolve active alert."""
        try:
            async with self.db_pool.acquire() as conn:
                result = await conn.execute("""
                    UPDATE alert_events
                    SET status = 'resolved', resolved_at = NOW(),
                        resolved_by = $2, resolution_note = $3
                    WHERE alert_id = $1
                    AND ($4::text IS NULL OR tenant_id = $4)
                    AND status IN ('active', 'acknowledged')
                """, alert_id, resolved_by, resolution_note, tenant_id)

            if result.startswith("UPDATE"):
                if alert_id in self._active_alerts:
                    self._active_alerts[alert_id].status = AlertStatus.RESOLVED
                    self._active_alerts[alert_id].resolved_at = datetime.utcnow()

                logger.info(f"Alert resolved: {alert_id} by {resolved_by}")
                return True
            else:
                logger.warning(f"Alert not found or already resolved: {alert_id}")
                return False

        except Exception as e:
            logger.error(f"Failed to resolve alert {alert_id}: {e}")
            return False

    async def get_active_alerts(
        self,
        tenant_id: Optional[str] = None,
        severity_filter: Optional[AlertSeverity] = None,
        agent_id: Optional[str] = None
    ) -> List[AlertInstance]:
        """Get active alerts with optional filters."""
        try:
            conditions = ["status IN ('active', 'acknowledged')"]
            params = []
            param_count = 0

            if tenant_id:
                param_count += 1
                conditions.append(f"tenant_id = ${param_count}")
                params.append(tenant_id)

            if severity_filter:
                param_count += 1
                conditions.append(f"severity = ${param_count}")
                params.append(severity_filter.value)

            if agent_id:
                param_count += 1
                conditions.append(f"agent_id = ${param_count}")
                params.append(agent_id)

            query = f"""
                SELECT alert_id, rule_name, agent_id, alert_type, severity,
                       status, message, metric_value, threshold_value,
                       triggered_at, acknowledged_at, resolved_at, tenant_id
                FROM alert_events
                WHERE {' AND '.join(conditions)}
                ORDER BY triggered_at DESC
                LIMIT 100
            """

            async with self.db_pool.acquire() as conn:
                rows = await conn.fetch(query, *params)

            alerts = []
            for row in rows:
                alert = AlertInstance(
                    alert_id=row['alert_id'],
                    rule_name=row['rule_name'],
                    agent_id=row['agent_id'],
                    severity=AlertSeverity(row['severity']),
                    category=AlertCategory(row['alert_type']),
                    status=AlertStatus(row['status']),
                    title=f"{row['alert_type'].title()} Alert: {row['agent_id']}",
                    description=row['message'],
                    metric_value=row['metric_value'],
                    threshold_value=row['threshold_value'],
                    triggered_at=row['triggered_at'],
                    acknowledged_at=row['acknowledged_at'],
                    resolved_at=row['resolved_at'],
                    tenant_id=row['tenant_id']
                )
                alerts.append(alert)

            return alerts

        except Exception as e:
            logger.error(f"Failed to get active alerts: {e}")
            return []

    async def analyze_alert_patterns(
        self,
        hours_back: int = 24,
        tenant_id: Optional[str] = None
    ) -> List[AlertPattern]:
        """Analyze alert patterns for trends and insights."""
        try:
            async with self.db_pool.acquire() as conn:
                # Get alert frequency by type and agent
                rows = await conn.fetch("""
                    SELECT alert_type, agent_id, COUNT(*) as frequency,
                           MIN(triggered_at) as first_occurrence,
                           MAX(triggered_at) as last_occurrence
                    FROM alert_events
                    WHERE triggered_at >= NOW() - INTERVAL '%s hours'
                    AND ($2::text IS NULL OR tenant_id = $2)
                    GROUP BY alert_type, agent_id
                    HAVING COUNT(*) >= 3
                    ORDER BY frequency DESC
                """ % hours_back, tenant_id)

            patterns = []
            for row in rows:
                # Determine pattern type and severity
                frequency = row['frequency']
                if frequency >= 20:
                    severity = AlertSeverity.CRITICAL
                    pattern_type = "high_frequency_alerts"
                elif frequency >= 10:
                    severity = AlertSeverity.HIGH
                    pattern_type = "frequent_alerts"
                else:
                    severity = AlertSeverity.MEDIUM
                    pattern_type = "recurring_alerts"

                description = f"Agent {row['agent_id']} has {frequency} {row['alert_type']} alerts in {hours_back}h"

                recommendation = self._generate_pattern_recommendation(
                    row['alert_type'], frequency, row['agent_id']
                )

                pattern = AlertPattern(
                    pattern_type=pattern_type,
                    frequency=frequency,
                    affected_agents=[row['agent_id']],
                    description=description,
                    severity=severity,
                    first_occurrence=row['first_occurrence'],
                    last_occurrence=row['last_occurrence'],
                    recommendation=recommendation
                )
                patterns.append(pattern)

            # Group similar patterns
            grouped_patterns = self._group_similar_patterns(patterns)

            return grouped_patterns

        except Exception as e:
            logger.error(f"Failed to analyze alert patterns: {e}")
            return []

    def _generate_pattern_recommendation(
        self,
        alert_type: str,
        frequency: int,
        agent_id: str
    ) -> str:
        """Generate recommendation for alert pattern."""
        if alert_type == "performance":
            if frequency >= 10:
                return f"Consider scaling resources for {agent_id} or optimizing processing logic"
            else:
                return f"Monitor {agent_id} performance trends and review recent changes"
        elif alert_type == "error_rate":
            if frequency >= 15:
                return f"Investigate {agent_id} error sources immediately - possible data quality issues"
            else:
                return f"Review {agent_id} error logs and input data validation"
        elif alert_type == "health_score":
            return f"Perform comprehensive health check on {agent_id} - multiple systems may be affected"
        elif alert_type == "resource_usage":
            return f"Analyze {agent_id} resource consumption patterns and consider limits or optimization"
        else:
            return f"Investigate {agent_id} for {alert_type} issues and review configuration"

    def _group_similar_patterns(self, patterns: List[AlertPattern]) -> List[AlertPattern]:
        """Group similar patterns for easier analysis."""
        grouped = {}

        for pattern in patterns:
            key = f"{pattern.pattern_type}_{pattern.severity.value}"
            if key not in grouped:
                grouped[key] = pattern
            else:
                # Merge patterns
                existing = grouped[key]
                existing.frequency += pattern.frequency
                existing.affected_agents.extend(pattern.affected_agents)
                existing.description = f"{len(existing.affected_agents)} agents with {existing.pattern_type}"

                if pattern.first_occurrence < existing.first_occurrence:
                    existing.first_occurrence = pattern.first_occurrence
                if pattern.last_occurrence > existing.last_occurrence:
                    existing.last_occurrence = pattern.last_occurrence

        return list(grouped.values())

    async def _load_alert_rules(self):
        """Load alert rules from database."""
        try:
            rules = await self.get_alert_rules()
            self._alert_rules = {rule.name: rule for rule in rules}
            logger.info(f"Loaded {len(self._alert_rules)} alert rules")

        except Exception as e:
            logger.error(f"Failed to load alert rules: {e}")

    async def _monitor_loop(self):
        """Main monitoring loop for checking thresholds."""
        try:
            while True:
                await asyncio.sleep(60)  # Check every minute

                # Check each alert rule
                for rule_name, rule in self._alert_rules.items():
                    if not rule.enabled:
                        continue

                    await self._check_rule(rule)

        except asyncio.CancelledError:
            logger.info("Alert monitoring loop cancelled")
        except Exception as e:
            logger.error(f"Error in alert monitoring loop: {e}")

    async def _check_rule(self, rule: AlertRule):
        """Check individual alert rule against current metrics."""
        try:
            # Get recent metric values
            async with self.db_pool.acquire() as conn:
                # Build agent filter
                if rule.agent_pattern and rule.agent_pattern != "*":
                    agent_condition = "AND agent_id LIKE $3"
                    params = [rule.metric_name, rule.duration_minutes, f"%{rule.agent_pattern}%"]
                else:
                    agent_condition = ""
                    params = [rule.metric_name, rule.duration_minutes]

                if rule.tenant_id:
                    tenant_condition = f"AND tenant_id = ${len(params) + 1}"
                    params.append(rule.tenant_id)
                else:
                    tenant_condition = ""

                query = f"""
                    SELECT agent_id, AVG(metric_value) as avg_value,
                           MAX(metric_value) as max_value,
                           MIN(metric_value) as min_value
                    FROM agent_metrics
                    WHERE metric_name = $1
                    AND timestamp >= NOW() - INTERVAL '{rule.duration_minutes} minutes'
                    {agent_condition}
                    {tenant_condition}
                    GROUP BY agent_id
                """

                rows = await conn.fetch(query, *params)

            # Check thresholds
            for row in rows:
                metric_value = row['avg_value']  # Use average for threshold checking

                # Apply threshold operator
                threshold_breached = self._evaluate_threshold(
                    metric_value, rule.operator, rule.threshold_value
                )

                if threshold_breached:
                    # Check if alert already exists
                    existing_alert = None
                    for alert_id, alert in self._active_alerts.items():
                        if (alert.rule_name == rule.name and
                            alert.agent_id == row['agent_id'] and
                            alert.status == AlertStatus.ACTIVE):
                            existing_alert = alert
                            break

                    # Only trigger new alert if none exists
                    if not existing_alert:
                        context = {
                            "avg_value": float(row['avg_value']),
                            "max_value": float(row['max_value']),
                            "min_value": float(row['min_value']),
                            "duration_minutes": rule.duration_minutes
                        }

                        await self.trigger_alert(
                            rule=rule,
                            agent_id=row['agent_id'],
                            metric_value=metric_value,
                            context=context
                        )

        except Exception as e:
            logger.error(f"Failed to check rule {rule.name}: {e}")

    def _evaluate_threshold(self, value: float, operator: str, threshold: float) -> bool:
        """Evaluate threshold condition."""
        if operator == ">":
            return value > threshold
        elif operator == ">=":
            return value >= threshold
        elif operator == "<":
            return value < threshold
        elif operator == "<=":
            return value <= threshold
        elif operator == "==":
            return abs(value - threshold) < 0.001  # Float comparison
        elif operator == "!=":
            return abs(value - threshold) >= 0.001
        else:
            logger.warning(f"Unknown operator: {operator}")
            return False


# Default alert rules for common scenarios
DEFAULT_ALERT_RULES = [
    AlertRule(
        name="high_error_rate",
        category=AlertCategory.ERROR_RATE,
        severity=AlertSeverity.HIGH,
        metric_name="error_rate",
        operator=">",
        threshold_value=0.1,  # 10% error rate
        duration_minutes=5
    ),
    AlertRule(
        name="low_health_score",
        category=AlertCategory.HEALTH_SCORE,
        severity=AlertSeverity.MEDIUM,
        metric_name="health_score",
        operator="<",
        threshold_value=60.0,  # Health score below 60
        duration_minutes=10
    ),
    AlertRule(
        name="slow_processing",
        category=AlertCategory.PERFORMANCE,
        severity=AlertSeverity.MEDIUM,
        metric_name="processing_time_ms",
        operator=">",
        threshold_value=30000.0,  # 30 seconds
        duration_minutes=5
    ),
    AlertRule(
        name="high_cpu_usage",
        category=AlertCategory.RESOURCE_USAGE,
        severity=AlertSeverity.HIGH,
        metric_name="cpu_usage_percent",
        operator=">",
        threshold_value=90.0,  # 90% CPU
        duration_minutes=5
    ),
    AlertRule(
        name="high_memory_usage",
        category=AlertCategory.RESOURCE_USAGE,
        severity=AlertSeverity.HIGH,
        metric_name="memory_usage_percent",
        operator=">",
        threshold_value=85.0,  # 85% memory
        duration_minutes=10
    )
]