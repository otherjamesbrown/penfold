"""Gmail Integration Monitoring and Alerting System.

This module provides comprehensive monitoring, health checks, and alerting
capabilities for the Gmail integration system, including metrics collection,
threshold monitoring, and alert dispatching.
"""

import asyncio
import logging
import time
from datetime import datetime, timedelta
from typing import Dict, List, Optional, Any, Callable, Union
from dataclasses import dataclass, field
from enum import Enum
import json
import statistics

from sqlalchemy import select, func, and_, or_
from sqlalchemy.ext.asyncio import AsyncSession

from .error_handling import ErrorSeverity, ErrorCategory, gmail_error_handler
from .recovery import system_recovery, RecoveryStatus
from ...models.gmail_connection import GmailConnection
from ...models.email_message import EmailMessage, EmailThread
from ...storage.database import db_manager


logger = logging.getLogger(__name__)


class AlertLevel(Enum):
    """Alert severity levels."""
    INFO = "info"
    WARNING = "warning"
    ERROR = "error"
    CRITICAL = "critical"


class MetricType(Enum):
    """Types of metrics to track."""
    COUNTER = "counter"
    GAUGE = "gauge"
    HISTOGRAM = "histogram"
    RATE = "rate"


@dataclass
class Alert:
    """Alert notification data structure."""
    level: AlertLevel
    title: str
    message: str
    category: str
    timestamp: datetime = field(default_factory=datetime.utcnow)
    metadata: Dict[str, Any] = field(default_factory=dict)
    source: str = "gmail_integration"
    tags: List[str] = field(default_factory=list)

    def to_dict(self) -> Dict[str, Any]:
        """Convert alert to dictionary format."""
        return {
            "level": self.level.value,
            "title": self.title,
            "message": self.message,
            "category": self.category,
            "timestamp": self.timestamp.isoformat(),
            "metadata": self.metadata,
            "source": self.source,
            "tags": self.tags
        }


@dataclass
class Metric:
    """Metric data structure for monitoring."""
    name: str
    value: Union[int, float]
    metric_type: MetricType
    timestamp: datetime = field(default_factory=datetime.utcnow)
    labels: Dict[str, str] = field(default_factory=dict)
    unit: Optional[str] = None

    def to_dict(self) -> Dict[str, Any]:
        """Convert metric to dictionary format."""
        return {
            "name": self.name,
            "value": self.value,
            "type": self.metric_type.value,
            "timestamp": self.timestamp.isoformat(),
            "labels": self.labels,
            "unit": self.unit
        }


class HealthStatus(Enum):
    """System health status levels."""
    HEALTHY = "healthy"
    DEGRADED = "degraded"
    UNHEALTHY = "unhealthy"
    CRITICAL = "critical"


@dataclass
class HealthCheckResult:
    """Result of a health check operation."""
    component: str
    status: HealthStatus
    message: str
    metrics: Dict[str, Any] = field(default_factory=dict)
    timestamp: datetime = field(default_factory=datetime.utcnow)
    response_time_ms: Optional[float] = None


class AlertChannel:
    """Base class for alert notification channels."""

    async def send_alert(self, alert: Alert) -> bool:
        """Send alert notification.

        Args:
            alert: Alert to send

        Returns:
            True if sent successfully, False otherwise
        """
        raise NotImplementedError


class LoggingAlertChannel(AlertChannel):
    """Alert channel that logs alerts."""

    async def send_alert(self, alert: Alert) -> bool:
        """Send alert to logs."""
        try:
            log_data = alert.to_dict()
            log_message = f"ALERT [{alert.level.value.upper()}] {alert.title}: {alert.message}"

            if alert.level == AlertLevel.CRITICAL:
                logger.critical(f"{log_message} | Data: {json.dumps(log_data, default=str)}")
            elif alert.level == AlertLevel.ERROR:
                logger.error(f"{log_message} | Data: {json.dumps(log_data, default=str)}")
            elif alert.level == AlertLevel.WARNING:
                logger.warning(f"{log_message} | Data: {json.dumps(log_data, default=str)}")
            else:
                logger.info(f"{log_message} | Data: {json.dumps(log_data, default=str)}")

            return True
        except Exception as e:
            logger.error(f"Failed to send alert to logs: {e}")
            return False


class WebhookAlertChannel(AlertChannel):
    """Alert channel that sends alerts to webhooks."""

    def __init__(self, webhook_url: str, headers: Optional[Dict[str, str]] = None):
        self.webhook_url = webhook_url
        self.headers = headers or {}

    async def send_alert(self, alert: Alert) -> bool:
        """Send alert to webhook."""
        try:
            import aiohttp

            payload = {
                "alert": alert.to_dict(),
                "service": "penfold_gmail_integration"
            }

            async with aiohttp.ClientSession() as session:
                async with session.post(
                    self.webhook_url,
                    json=payload,
                    headers=self.headers,
                    timeout=aiohttp.ClientTimeout(total=10)
                ) as response:
                    return response.status < 400

        except Exception as e:
            logger.error(f"Failed to send alert to webhook {self.webhook_url}: {e}")
            return False


class GmailMetricsCollector:
    """Collects and tracks metrics for Gmail integration system."""

    def __init__(self):
        self.metrics: Dict[str, List[Metric]] = {}
        self._lock = asyncio.Lock()

    async def record_metric(self, metric: Metric) -> None:
        """Record a metric value."""
        async with self._lock:
            if metric.name not in self.metrics:
                self.metrics[metric.name] = []

            self.metrics[metric.name].append(metric)

            # Keep only recent metrics (last 24 hours)
            cutoff = datetime.utcnow() - timedelta(hours=24)
            self.metrics[metric.name] = [
                m for m in self.metrics[metric.name]
                if m.timestamp > cutoff
            ]

    async def get_metric_values(
        self,
        metric_name: str,
        labels: Optional[Dict[str, str]] = None,
        since: Optional[datetime] = None
    ) -> List[float]:
        """Get metric values with optional filtering."""
        async with self._lock:
            if metric_name not in self.metrics:
                return []

            metrics = self.metrics[metric_name]

            # Filter by timestamp
            if since:
                metrics = [m for m in metrics if m.timestamp > since]

            # Filter by labels
            if labels:
                metrics = [
                    m for m in metrics
                    if all(m.labels.get(k) == v for k, v in labels.items())
                ]

            return [m.value for m in metrics]

    async def get_metric_statistics(
        self,
        metric_name: str,
        labels: Optional[Dict[str, str]] = None,
        window_minutes: int = 60
    ) -> Dict[str, float]:
        """Get statistical summary of a metric."""
        since = datetime.utcnow() - timedelta(minutes=window_minutes)
        values = await self.get_metric_values(metric_name, labels, since)

        if not values:
            return {}

        return {
            "count": len(values),
            "sum": sum(values),
            "min": min(values),
            "max": max(values),
            "mean": statistics.mean(values),
            "median": statistics.median(values),
            "p95": self._percentile(values, 0.95),
            "p99": self._percentile(values, 0.99)
        }

    def _percentile(self, values: List[float], percentile: float) -> float:
        """Calculate percentile of values."""
        if not values:
            return 0.0

        sorted_values = sorted(values)
        k = (len(sorted_values) - 1) * percentile
        f = int(k)
        c = k - f

        if f + 1 < len(sorted_values):
            return sorted_values[f] + c * (sorted_values[f + 1] - sorted_values[f])
        else:
            return sorted_values[f]

    async def clear_metrics(self) -> None:
        """Clear all stored metrics."""
        async with self._lock:
            self.metrics.clear()


class GmailHealthMonitor:
    """Comprehensive health monitoring for Gmail integration."""

    def __init__(self, metrics_collector: GmailMetricsCollector):
        self.metrics_collector = metrics_collector
        self.health_checks: Dict[str, Callable] = {}
        self.last_health_check: Optional[datetime] = None
        self.health_status: HealthStatus = HealthStatus.HEALTHY

    def register_health_check(self, name: str, check_func: Callable) -> None:
        """Register a health check function."""
        self.health_checks[name] = check_func

    async def run_health_checks(self) -> Dict[str, HealthCheckResult]:
        """Run all registered health checks."""
        results = {}
        overall_status = HealthStatus.HEALTHY

        for name, check_func in self.health_checks.items():
            start_time = time.time()
            try:
                result = await check_func()
                response_time = (time.time() - start_time) * 1000

                if isinstance(result, HealthCheckResult):
                    result.response_time_ms = response_time
                    results[name] = result
                else:
                    # Convert simple return to HealthCheckResult
                    results[name] = HealthCheckResult(
                        component=name,
                        status=HealthStatus.HEALTHY if result else HealthStatus.UNHEALTHY,
                        message="Check completed",
                        response_time_ms=response_time
                    )

                # Track worst status
                if results[name].status.value > overall_status.value:
                    overall_status = results[name].status

            except Exception as e:
                logger.error(f"Health check {name} failed: {e}")
                results[name] = HealthCheckResult(
                    component=name,
                    status=HealthStatus.CRITICAL,
                    message=f"Check failed: {str(e)}",
                    response_time_ms=(time.time() - start_time) * 1000
                )
                overall_status = HealthStatus.CRITICAL

        self.health_status = overall_status
        self.last_health_check = datetime.utcnow()

        # Record health metrics
        await self.metrics_collector.record_metric(
            Metric(
                name="health_check_duration",
                value=(time.time() - (time.time() - sum(r.response_time_ms or 0 for r in results.values()) / 1000)),
                metric_type=MetricType.HISTOGRAM,
                unit="ms"
            )
        )

        await self.metrics_collector.record_metric(
            Metric(
                name="health_status",
                value=1 if overall_status == HealthStatus.HEALTHY else 0,
                metric_type=MetricType.GAUGE,
                labels={"status": overall_status.value}
            )
        )

        return results

    async def check_database_health(self) -> HealthCheckResult:
        """Check database connectivity and performance."""
        try:
            start_time = time.time()

            async with db_manager.get_session() as session:
                # Simple connectivity check
                await session.execute(select(1))

                # Count active connections
                result = await session.execute(
                    select(func.count(GmailConnection.id)).where(GmailConnection.is_active == True)
                )
                active_connections = result.scalar()

                # Count recent messages
                since_hour = datetime.utcnow() - timedelta(hours=1)
                result = await session.execute(
                    select(func.count(EmailMessage.id)).where(EmailMessage.created_at > since_hour)
                )
                recent_messages = result.scalar()

                response_time = (time.time() - start_time) * 1000

                metrics = {
                    "active_connections": active_connections,
                    "recent_messages_1h": recent_messages,
                    "response_time_ms": response_time
                }

                status = HealthStatus.HEALTHY
                message = "Database is healthy"

                if response_time > 5000:  # 5 seconds
                    status = HealthStatus.DEGRADED
                    message = f"Database responding slowly ({response_time:.2f}ms)"
                elif response_time > 10000:  # 10 seconds
                    status = HealthStatus.UNHEALTHY
                    message = f"Database very slow ({response_time:.2f}ms)"

                return HealthCheckResult(
                    component="database",
                    status=status,
                    message=message,
                    metrics=metrics
                )

        except Exception as e:
            return HealthCheckResult(
                component="database",
                status=HealthStatus.CRITICAL,
                message=f"Database check failed: {str(e)}"
            )

    async def check_gmail_api_health(self) -> HealthCheckResult:
        """Check Gmail API connectivity and quota status."""
        try:
            # Get error statistics from error handler
            error_stats = gmail_error_handler.get_error_statistics()

            # Calculate recent error rates
            recent_api_errors = 0
            recent_quota_errors = 0

            for category, severity_counts in error_stats["error_stats"].items():
                if category in ["api_rate_limit", "api_quota"]:
                    recent_quota_errors += sum(severity_counts.values())
                elif category in ["authentication", "permission"]:
                    recent_api_errors += sum(severity_counts.values())

            # Check circuit breaker status
            circuit_breakers = error_stats["circuit_breakers"]
            open_circuits = sum(1 for cb in circuit_breakers.values() if cb["state"] == "OPEN")

            metrics = {
                "recent_api_errors": recent_api_errors,
                "recent_quota_errors": recent_quota_errors,
                "open_circuit_breakers": open_circuits
            }

            status = HealthStatus.HEALTHY
            message = "Gmail API is healthy"

            if recent_quota_errors > 10:
                status = HealthStatus.DEGRADED
                message = f"High quota error rate ({recent_quota_errors} recent errors)"
            elif open_circuits > 0:
                status = HealthStatus.UNHEALTHY
                message = f"{open_circuits} circuit breakers open"
            elif recent_api_errors > 20:
                status = HealthStatus.CRITICAL
                message = f"Critical API error rate ({recent_api_errors} recent errors)"

            return HealthCheckResult(
                component="gmail_api",
                status=status,
                message=message,
                metrics=metrics
            )

        except Exception as e:
            return HealthCheckResult(
                component="gmail_api",
                status=HealthStatus.CRITICAL,
                message=f"API check failed: {str(e)}"
            )

    async def check_sync_operations_health(self) -> HealthCheckResult:
        """Check sync operation health and performance."""
        try:
            async with db_manager.get_session() as session:
                # Check for stuck sync operations
                stuck_cutoff = datetime.utcnow() - timedelta(hours=2)
                result = await session.execute(
                    select(func.count(EmailMessage.id)).where(
                        and_(
                            EmailMessage.processing_status == "processing",
                            EmailMessage.updated_at < stuck_cutoff
                        )
                    )
                )
                stuck_messages = result.scalar()

                # Check sync performance (messages per hour)
                hour_ago = datetime.utcnow() - timedelta(hours=1)
                result = await session.execute(
                    select(func.count(EmailMessage.id)).where(
                        EmailMessage.created_at > hour_ago
                    )
                )
                messages_last_hour = result.scalar()

                # Check failed messages
                result = await session.execute(
                    select(func.count(EmailMessage.id)).where(
                        EmailMessage.processing_status == "failed"
                    )
                )
                failed_messages = result.scalar()

                metrics = {
                    "stuck_messages": stuck_messages,
                    "messages_per_hour": messages_last_hour,
                    "failed_messages": failed_messages
                }

                status = HealthStatus.HEALTHY
                message = "Sync operations are healthy"

                if stuck_messages > 100:
                    status = HealthStatus.DEGRADED
                    message = f"{stuck_messages} messages stuck in processing"
                elif stuck_messages > 500:
                    status = HealthStatus.UNHEALTHY
                    message = f"Many stuck operations ({stuck_messages} messages)"
                elif failed_messages > 1000:
                    status = HealthStatus.CRITICAL
                    message = f"High failure rate ({failed_messages} failed messages)"

                return HealthCheckResult(
                    component="sync_operations",
                    status=status,
                    message=message,
                    metrics=metrics
                )

        except Exception as e:
            return HealthCheckResult(
                component="sync_operations",
                status=HealthStatus.CRITICAL,
                message=f"Sync check failed: {str(e)}"
            )


class GmailAlertManager:
    """Manages alert generation, routing, and delivery."""

    def __init__(self):
        self.channels: List[AlertChannel] = []
        self.alert_rules: List[Dict[str, Any]] = []
        self.alert_history: List[Alert] = []
        self.suppression_rules: Dict[str, datetime] = {}

    def add_channel(self, channel: AlertChannel) -> None:
        """Add an alert notification channel."""
        self.channels.append(channel)

    def add_alert_rule(
        self,
        name: str,
        condition: Callable[[Dict[str, Any]], bool],
        level: AlertLevel,
        message_template: str,
        category: str = "general",
        cooldown_minutes: int = 30
    ) -> None:
        """Add an alert rule.

        Args:
            name: Rule name
            condition: Function that returns True when alert should trigger
            level: Alert severity level
            message_template: Template for alert message
            category: Alert category
            cooldown_minutes: Minimum time between similar alerts
        """
        self.alert_rules.append({
            "name": name,
            "condition": condition,
            "level": level,
            "message_template": message_template,
            "category": category,
            "cooldown_minutes": cooldown_minutes
        })

    async def evaluate_rules(self, context: Dict[str, Any]) -> List[Alert]:
        """Evaluate all alert rules against current context."""
        alerts = []

        for rule in self.alert_rules:
            try:
                if rule["condition"](context):
                    # Check suppression
                    suppression_key = f"{rule['name']}:{rule['category']}"
                    if suppression_key in self.suppression_rules:
                        if datetime.utcnow() < self.suppression_rules[suppression_key]:
                            continue  # Still in cooldown period

                    # Create alert
                    alert = Alert(
                        level=rule["level"],
                        title=rule["name"],
                        message=rule["message_template"].format(**context),
                        category=rule["category"],
                        metadata=context
                    )

                    alerts.append(alert)

                    # Set suppression
                    cooldown_until = datetime.utcnow() + timedelta(minutes=rule["cooldown_minutes"])
                    self.suppression_rules[suppression_key] = cooldown_until

            except Exception as e:
                logger.error(f"Error evaluating alert rule {rule['name']}: {e}")

        return alerts

    async def send_alert(self, alert: Alert) -> None:
        """Send alert through all configured channels."""
        logger.info(f"Sending alert: {alert.title} ({alert.level.value})")

        sent_count = 0
        for channel in self.channels:
            try:
                if await channel.send_alert(alert):
                    sent_count += 1
            except Exception as e:
                logger.error(f"Failed to send alert through channel {type(channel).__name__}: {e}")

        if sent_count == 0:
            logger.error(f"Failed to send alert through any channel: {alert.title}")

        # Store in history
        self.alert_history.append(alert)

        # Keep only recent alerts (last 7 days)
        cutoff = datetime.utcnow() - timedelta(days=7)
        self.alert_history = [a for a in self.alert_history if a.timestamp > cutoff]

    async def send_alerts(self, alerts: List[Alert]) -> None:
        """Send multiple alerts."""
        for alert in alerts:
            await self.send_alert(alert)

    def get_recent_alerts(self, hours: int = 24) -> List[Alert]:
        """Get alerts from recent time period."""
        since = datetime.utcnow() - timedelta(hours=hours)
        return [a for a in self.alert_history if a.timestamp > since]


class GmailMonitoringSystem:
    """Main monitoring system that coordinates all components."""

    def __init__(self):
        self.metrics_collector = GmailMetricsCollector()
        self.health_monitor = GmailHealthMonitor(self.metrics_collector)
        self.alert_manager = GmailAlertManager()
        self._monitoring_task: Optional[asyncio.Task] = None
        self._monitoring_interval = 60  # seconds

        # Set up default health checks
        self.health_monitor.register_health_check(
            "database", self.health_monitor.check_database_health
        )
        self.health_monitor.register_health_check(
            "gmail_api", self.health_monitor.check_gmail_api_health
        )
        self.health_monitor.register_health_check(
            "sync_operations", self.health_monitor.check_sync_operations_health
        )

        # Set up default alert channels
        self.alert_manager.add_channel(LoggingAlertChannel())

        # Set up default alert rules
        self._setup_default_alert_rules()

    def _setup_default_alert_rules(self) -> None:
        """Set up default monitoring alert rules."""
        # High error rate alert
        self.alert_manager.add_alert_rule(
            name="High Error Rate",
            condition=lambda ctx: ctx.get("error_rate_per_minute", 0) > 10,
            level=AlertLevel.WARNING,
            message_template="High error rate detected: {error_rate_per_minute} errors/minute",
            category="errors"
        )

        # Database slow response alert
        self.alert_manager.add_alert_rule(
            name="Database Slow Response",
            condition=lambda ctx: ctx.get("database_response_time_ms", 0) > 5000,
            level=AlertLevel.WARNING,
            message_template="Database responding slowly: {database_response_time_ms}ms",
            category="performance"
        )

        # Stuck sync operations alert
        self.alert_manager.add_alert_rule(
            name="Stuck Sync Operations",
            condition=lambda ctx: ctx.get("stuck_messages", 0) > 100,
            level=AlertLevel.ERROR,
            message_template="Many sync operations stuck: {stuck_messages} messages",
            category="sync"
        )

        # Circuit breaker open alert
        self.alert_manager.add_alert_rule(
            name="Circuit Breaker Open",
            condition=lambda ctx: ctx.get("open_circuit_breakers", 0) > 0,
            level=AlertLevel.CRITICAL,
            message_template="Circuit breakers open: {open_circuit_breakers} services unavailable",
            category="availability"
        )

    async def start_monitoring(self) -> None:
        """Start the monitoring system."""
        if self._monitoring_task and not self._monitoring_task.done():
            logger.warning("Monitoring already started")
            return

        logger.info("Starting Gmail monitoring system")
        self._monitoring_task = asyncio.create_task(self._monitoring_loop())

    async def stop_monitoring(self) -> None:
        """Stop the monitoring system."""
        if self._monitoring_task:
            logger.info("Stopping Gmail monitoring system")
            self._monitoring_task.cancel()
            try:
                await self._monitoring_task
            except asyncio.CancelledError:
                pass

    async def _monitoring_loop(self) -> None:
        """Main monitoring loop."""
        while True:
            try:
                # Run health checks
                health_results = await self.health_monitor.run_health_checks()

                # Collect current metrics
                context = await self._collect_monitoring_context(health_results)

                # Record system metrics
                await self._record_system_metrics(context)

                # Evaluate alert rules
                alerts = await self.alert_manager.evaluate_rules(context)

                # Send alerts
                await self.alert_manager.send_alerts(alerts)

                logger.debug(f"Monitoring cycle completed: {len(alerts)} alerts generated")

            except Exception as e:
                logger.error(f"Error in monitoring loop: {e}")

            # Wait for next monitoring cycle
            await asyncio.sleep(self._monitoring_interval)

    async def _collect_monitoring_context(self, health_results: Dict[str, HealthCheckResult]) -> Dict[str, Any]:
        """Collect monitoring context from various sources."""
        context = {}

        # Add health check results
        for name, result in health_results.items():
            context.update({f"{name}_{k}": v for k, v in result.metrics.items()})
            context[f"{name}_status"] = result.status.value
            context[f"{name}_response_time_ms"] = result.response_time_ms or 0

        # Add error statistics
        error_stats = gmail_error_handler.get_error_statistics()
        total_errors = sum(
            sum(severity_counts.values())
            for severity_counts in error_stats["error_stats"].values()
        )
        context["total_errors"] = total_errors
        context["error_rate_per_minute"] = total_errors / max(1, self._monitoring_interval / 60)

        # Add circuit breaker status
        context["open_circuit_breakers"] = sum(
            1 for cb in error_stats["circuit_breakers"].values()
            if cb["state"] == "OPEN"
        )

        # Add timestamp
        context["timestamp"] = datetime.utcnow().isoformat()

        return context

    async def _record_system_metrics(self, context: Dict[str, Any]) -> None:
        """Record system metrics."""
        timestamp = datetime.utcnow()

        # Record key metrics
        for metric_name in ["total_errors", "error_rate_per_minute", "open_circuit_breakers"]:
            if metric_name in context:
                await self.metrics_collector.record_metric(
                    Metric(
                        name=metric_name,
                        value=context[metric_name],
                        metric_type=MetricType.GAUGE,
                        timestamp=timestamp
                    )
                )

    async def get_system_status(self) -> Dict[str, Any]:
        """Get current system status summary."""
        health_results = await self.health_monitor.run_health_checks()
        recent_alerts = self.alert_manager.get_recent_alerts()

        # Calculate metrics summaries
        error_stats = await self.metrics_collector.get_metric_statistics("total_errors")
        response_time_stats = await self.metrics_collector.get_metric_statistics("database_response_time_ms")

        return {
            "overall_status": self.health_monitor.health_status.value,
            "last_check": self.health_monitor.last_health_check.isoformat() if self.health_monitor.last_health_check else None,
            "health_checks": {name: result.__dict__ for name, result in health_results.items()},
            "recent_alerts": [alert.to_dict() for alert in recent_alerts[-10:]],  # Last 10 alerts
            "metrics": {
                "error_stats": error_stats,
                "response_time_stats": response_time_stats
            },
            "monitoring_active": self._monitoring_task is not None and not self._monitoring_task.done()
        }


# Global monitoring system instance
gmail_monitoring = GmailMonitoringSystem()