"""Observability services for the Penfold framework.

This package provides the core services for agent monitoring and observability,
including instrumentation, metrics collection, decision tracing, workflow
tracking, agent health monitoring, business value tracking, and alert management.

Key Components:
    - instrumentation: Agent monitoring decorators and workflow tracing
    - metrics_collector: Time-series metrics collection and storage
    - agent_health: Comprehensive agent health scoring and trend analysis
    - business_value_tracker: Business KPI tracking and value measurement
    - usage_analytics: User engagement and feature utilization tracking
    - roi_calculator: Return on investment calculation and analysis
    - workflow_tracker: Cross-agent workflow execution monitoring
    - performance_analyzer: Performance metrics and bottleneck analysis
    - quality_tracker: Quality metrics and improvement tracking
    - alert_manager: Proactive monitoring and alerting system
    - system_monitor: System resource monitoring and attribution

The services implement the interface contracts defined in the contracts module
and provide production-ready implementations with comprehensive error handling,
validation, and performance optimization.

Example Usage:
    from observability_lib.services.instrumentation import monitor_agent
    from observability_lib.services.agent_health import AgentHealthService
    from observability_lib.services.business_value_tracker import BusinessValueTracker

    @monitor_agent("email_processor")
    class EmailProcessingAgent:
        async def process_emails(self, emails):
            async with self.workflow_trace("nightly_batch") as tracer:
                # Processing logic with full observability
                pass

    # Agent health monitoring
    health_service = AgentHealthService(settings)
    await health_service.start()
    health_score = await health_service.get_agent_health("email_processor")

    # Business value tracking
    tracker = BusinessValueTracker(settings)
    await tracker.start()
    await tracker.record_context_reconstruction_time(8.5, "medium", 5)
"""

# Import implemented services
from .instrumentation import (
    InstrumentationError,
    WorkflowTraceContext,
    monitor_agent,
    is_agent_monitored,
    get_agent_id,
    get_observability_metadata,
)

from .metrics_collector import (
    MetricsCollector,
    MetricsBuffer,
    ConnectionPool,
)

from .agent_health import (
    AgentHealthService,
    HealthStatus,
    HealthTrendDirection,
    HealthScore,
    HealthTrend,
    HealthAlert,
)

from .business_value_tracker import (
    BusinessValueTracker,
    KpiStatus,
    TrendDirection,
    KpiMeasurement,
    KpiSummary,
    BusinessAlert,
)

from .usage_analytics import (
    UsageAnalyticsService,
    EngagementLevel,
    FeatureCategory,
    UsageSession,
    FeatureUsageInsight,
    UserEngagementProfile,
    UsageAnalytics,
)

from .roi_calculator import (
    ROICalculatorService,
    ValueCategory,
    CalculationMethod,
    ROICalculationInput,
    ROIResult,
    TimeSavingsCalculation,
    ProductivityGainCalculation,
    ROISummary,
)

from .alert_manager import (
    AlertManagerService,
    AlertRule,
    AlertInstance,
    AlertPattern,
    AlertSeverity,
    AlertCategory,
    AlertStatus,
    DEFAULT_ALERT_RULES,
)

from .system_monitor import (
    SystemMonitorService,
    ResourceUsage,
    SystemSnapshot,
    ResourceAlert,
    ResourceType,
    ProcessInfo,
)

from .workflow_tracker import (
    WorkflowTracker,
)

from .performance_analyzer import (
    PerformanceAnalyzer,
)

from .quality_tracker import (
    QualityTracker,
)

__version__ = "1.2.0"

# Export public API
__all__ = [
    # Instrumentation
    "InstrumentationError",
    "WorkflowTraceContext",
    "monitor_agent",
    "is_agent_monitored",
    "get_agent_id",
    "get_observability_metadata",

    # Metrics Collection
    "MetricsCollector",
    "MetricsBuffer",
    "ConnectionPool",

    # Agent Health
    "AgentHealthService",
    "HealthStatus",
    "HealthTrendDirection",
    "HealthScore",
    "HealthTrend",
    "HealthAlert",

    # Business Value Tracking
    "BusinessValueTracker",
    "KpiStatus",
    "TrendDirection",
    "KpiMeasurement",
    "KpiSummary",
    "BusinessAlert",

    # Usage Analytics
    "UsageAnalyticsService",
    "EngagementLevel",
    "FeatureCategory",
    "UsageSession",
    "FeatureUsageInsight",
    "UserEngagementProfile",
    "UsageAnalytics",

    # ROI Calculator
    "ROICalculatorService",
    "ValueCategory",
    "CalculationMethod",
    "ROICalculationInput",
    "ROIResult",
    "TimeSavingsCalculation",
    "ProductivityGainCalculation",
    "ROISummary",

    # Alert Manager
    "AlertManagerService",
    "AlertRule",
    "AlertInstance",
    "AlertPattern",
    "AlertSeverity",
    "AlertCategory",
    "AlertStatus",
    "DEFAULT_ALERT_RULES",

    # System Monitor
    "SystemMonitorService",
    "ResourceUsage",
    "SystemSnapshot",
    "ResourceAlert",
    "ResourceType",
    "ProcessInfo",

    # Workflow Tracker
    "WorkflowTracker",

    # Performance Analyzer
    "PerformanceAnalyzer",

    # Quality Tracker
    "QualityTracker",
]