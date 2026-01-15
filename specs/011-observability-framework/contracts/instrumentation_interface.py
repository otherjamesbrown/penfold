# Observability Framework - Agent Instrumentation Interface Contract
# This file defines the contract interface that Penfold agents must implement
# for comprehensive monitoring and observability integration.

from abc import ABC, abstractmethod
from typing import Dict, Any, List, Optional, AsyncGenerator
from datetime import datetime
from enum import Enum
import uuid
from dataclasses import dataclass, asdict
from contextlib import asynccontextmanager

# ============================================================================
# Core Data Types for Observability
# ============================================================================

class MetricType(str, Enum):
    """Types of metrics that can be recorded"""
    COUNTER = "counter"
    GAUGE = "gauge"
    HISTOGRAM = "histogram"

class EventType(str, Enum):
    """Types of workflow events"""
    WORKFLOW_STARTED = "workflow_started"
    STAGE_COMPLETED = "stage_completed"
    HANDOFF = "handoff"
    WORKFLOW_FINISHED = "workflow_finished"
    ERROR = "error"

class EventStatus(str, Enum):
    """Status of workflow events"""
    SUCCESS = "success"
    FAILURE = "failure"
    TIMEOUT = "timeout"
    RETRY = "retry"

class AlertStatus(str, Enum):
    """Status of alert events"""
    ACTIVE = "active"
    RESOLVED = "resolved"
    ACKNOWLEDGED = "acknowledged"
    SUPPRESSED = "suppressed"

@dataclass
class MetricPoint:
    """Single metric data point"""
    metric_name: str
    metric_value: float
    metric_type: MetricType = MetricType.GAUGE
    timestamp: Optional[datetime] = None
    labels: Optional[Dict[str, Any]] = None

    def __post_init__(self):
        if self.timestamp is None:
            self.timestamp = datetime.utcnow()
        if self.labels is None:
            self.labels = {}

@dataclass
class DecisionContext:
    """Context data for agent decisions"""
    decision_type: str
    decision_context: Dict[str, Any]
    alternatives_considered: List[Dict[str, Any]]
    confidence_score: float
    reasoning: str
    workflow_id: Optional[uuid.UUID] = None

    def __post_init__(self):
        if not 0.0 <= self.confidence_score <= 1.0:
            raise ValueError("Confidence score must be between 0.0 and 1.0")

@dataclass
class WorkflowEventData:
    """Data for workflow events"""
    workflow_id: uuid.UUID
    agent_id: str
    event_type: EventType
    event_status: EventStatus
    event_data: Dict[str, Any]
    processing_time_ms: Optional[float] = None
    parent_workflow_id: Optional[uuid.UUID] = None

# ============================================================================
# Abstract Interfaces for Observability Components
# ============================================================================

class MetricsCollectorInterface(ABC):
    """Interface for collecting and storing agent metrics"""

    @abstractmethod
    async def record_metric(self, agent_id: str, metric: MetricPoint) -> None:
        """Record a single metric point for an agent"""
        pass

    @abstractmethod
    async def record_metrics_batch(self, agent_id: str, metrics: List[MetricPoint]) -> None:
        """Record multiple metrics atomically"""
        pass

    @abstractmethod
    async def get_metrics(
        self,
        agent_id: str,
        metric_names: Optional[List[str]] = None,
        start_time: Optional[datetime] = None,
        end_time: Optional[datetime] = None,
        aggregation: str = "avg",
        bucket_size: str = "5m"
    ) -> Dict[str, List[Dict[str, Any]]]:
        """Retrieve metrics for an agent within time range"""
        pass

class DecisionTracerInterface(ABC):
    """Interface for tracing agent decisions"""

    @abstractmethod
    async def log_decision(self, agent_id: str, decision: DecisionContext) -> uuid.UUID:
        """Log a decision made by an agent"""
        pass

    @abstractmethod
    async def get_decisions(
        self,
        agent_id: str,
        workflow_id: Optional[uuid.UUID] = None,
        decision_type: Optional[str] = None,
        start_time: Optional[datetime] = None,
        end_time: Optional[datetime] = None,
        min_confidence: Optional[float] = None,
        limit: int = 100
    ) -> List[Dict[str, Any]]:
        """Retrieve decision traces for debugging"""
        pass

class WorkflowTrackerInterface(ABC):
    """Interface for tracking cross-agent workflows"""

    @abstractmethod
    async def log_workflow_event(self, event: WorkflowEventData) -> None:
        """Log an event in a workflow execution"""
        pass

    @abstractmethod
    async def get_workflow_events(
        self,
        workflow_id: uuid.UUID,
        include_children: bool = False,
        event_type: Optional[EventType] = None
    ) -> List[Dict[str, Any]]:
        """Get all events for a workflow"""
        pass

    @abstractmethod
    async def create_workflow_id(self, parent_workflow_id: Optional[uuid.UUID] = None) -> uuid.UUID:
        """Generate a new workflow ID with optional parent relationship"""
        pass

class AlertManagerInterface(ABC):
    """Interface for alert management"""

    @abstractmethod
    async def evaluate_thresholds(self, agent_id: str) -> List[Dict[str, Any]]:
        """Evaluate alert thresholds for an agent and trigger alerts if needed"""
        pass

    @abstractmethod
    async def get_active_alerts(self, agent_id: Optional[str] = None) -> List[Dict[str, Any]]:
        """Get currently active alerts"""
        pass

# ============================================================================
# Agent Instrumentation Context Manager
# ============================================================================

class WorkflowTraceContext:
    """Context manager for workflow tracing"""

    def __init__(
        self,
        workflow_tracker: WorkflowTrackerInterface,
        metrics_collector: MetricsCollectorInterface,
        decision_tracer: DecisionTracerInterface,
        agent_id: str,
        workflow_type: str,
        parent_workflow_id: Optional[uuid.UUID] = None
    ):
        self.workflow_tracker = workflow_tracker
        self.metrics_collector = metrics_collector
        self.decision_tracer = decision_tracer
        self.agent_id = agent_id
        self.workflow_type = workflow_type
        self.parent_workflow_id = parent_workflow_id
        self.workflow_id: Optional[uuid.UUID] = None
        self.start_time: Optional[datetime] = None
        self.stage_timings: List[Dict[str, Any]] = []

    async def __aenter__(self):
        """Start workflow tracing"""
        self.workflow_id = await self.workflow_tracker.create_workflow_id(self.parent_workflow_id)
        self.start_time = datetime.utcnow()

        # Log workflow start event
        await self.workflow_tracker.log_workflow_event(WorkflowEventData(
            workflow_id=self.workflow_id,
            agent_id=self.agent_id,
            event_type=EventType.WORKFLOW_STARTED,
            event_status=EventStatus.SUCCESS,
            event_data={
                "workflow_type": self.workflow_type,
                "started_at": self.start_time.isoformat(),
                "parent_workflow_id": str(self.parent_workflow_id) if self.parent_workflow_id else None
            }
        ))

        return self

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        """Complete workflow tracing"""
        end_time = datetime.utcnow()
        total_duration = (end_time - self.start_time).total_seconds() * 1000

        # Determine final status
        if exc_type is None:
            status = EventStatus.SUCCESS
            event_data = {
                "workflow_type": self.workflow_type,
                "completed_at": end_time.isoformat(),
                "total_duration_ms": total_duration,
                "stage_count": len(self.stage_timings)
            }
        else:
            status = EventStatus.FAILURE
            event_data = {
                "workflow_type": self.workflow_type,
                "failed_at": end_time.isoformat(),
                "error_type": exc_type.__name__ if exc_type else None,
                "error_message": str(exc_val) if exc_val else None,
                "partial_duration_ms": total_duration,
                "stage_count": len(self.stage_timings)
            }

        # Log workflow completion event
        await self.workflow_tracker.log_workflow_event(WorkflowEventData(
            workflow_id=self.workflow_id,
            agent_id=self.agent_id,
            event_type=EventType.WORKFLOW_FINISHED,
            event_status=status,
            event_data=event_data,
            processing_time_ms=total_duration
        ))

        # Record workflow duration metric
        await self.metrics_collector.record_metric(
            self.agent_id,
            MetricPoint(
                metric_name="workflow_duration_ms",
                metric_value=total_duration,
                labels={
                    "workflow_type": self.workflow_type,
                    "status": status.value,
                    "workflow_id": str(self.workflow_id)
                }
            )
        )

    @asynccontextmanager
    async def stage(self, stage_name: str):
        """Context manager for individual workflow stages"""
        stage_start = datetime.utcnow()

        try:
            yield

            # Stage completed successfully
            stage_end = datetime.utcnow()
            stage_duration = (stage_end - stage_start).total_seconds() * 1000

            self.stage_timings.append({
                "stage_name": stage_name,
                "duration_ms": stage_duration,
                "status": "success"
            })

            await self.workflow_tracker.log_workflow_event(WorkflowEventData(
                workflow_id=self.workflow_id,
                agent_id=self.agent_id,
                event_type=EventType.STAGE_COMPLETED,
                event_status=EventStatus.SUCCESS,
                event_data={
                    "stage_name": stage_name,
                    "completed_at": stage_end.isoformat()
                },
                processing_time_ms=stage_duration
            ))

            # Record stage timing metric
            await self.metrics_collector.record_metric(
                self.agent_id,
                MetricPoint(
                    metric_name="stage_duration_ms",
                    metric_value=stage_duration,
                    labels={
                        "stage_name": stage_name,
                        "workflow_id": str(self.workflow_id)
                    }
                )
            )

        except Exception as e:
            # Stage failed
            stage_end = datetime.utcnow()
            stage_duration = (stage_end - stage_start).total_seconds() * 1000

            self.stage_timings.append({
                "stage_name": stage_name,
                "duration_ms": stage_duration,
                "status": "failed",
                "error": str(e)
            })

            await self.workflow_tracker.log_workflow_event(WorkflowEventData(
                workflow_id=self.workflow_id,
                agent_id=self.agent_id,
                event_type=EventType.ERROR,
                event_status=EventStatus.FAILURE,
                event_data={
                    "stage_name": stage_name,
                    "failed_at": stage_end.isoformat(),
                    "error_type": type(e).__name__,
                    "error_message": str(e)
                },
                processing_time_ms=stage_duration
            ))

            raise

    async def log_decision(
        self,
        decision_type: str,
        decision_context: Dict[str, Any],
        alternatives: List[Dict[str, Any]],
        confidence: float,
        reasoning: str
    ) -> None:
        """Log a decision within this workflow"""
        decision = DecisionContext(
            decision_type=decision_type,
            decision_context=decision_context,
            alternatives_considered=alternatives,
            confidence_score=confidence,
            reasoning=reasoning,
            workflow_id=self.workflow_id
        )

        await self.decision_tracer.log_decision(self.agent_id, decision)

        # Also record confidence as a metric
        await self.metrics_collector.record_metric(
            self.agent_id,
            MetricPoint(
                metric_name="decision_confidence",
                metric_value=confidence,
                labels={
                    "decision_type": decision_type,
                    "workflow_id": str(self.workflow_id)
                }
            )
        )

    async def log_handoff(
        self,
        to_agent: str,
        data_passed: Dict[str, Any],
        handoff_reason: str
    ) -> None:
        """Log a handoff to another agent"""
        await self.workflow_tracker.log_workflow_event(WorkflowEventData(
            workflow_id=self.workflow_id,
            agent_id=self.agent_id,
            event_type=EventType.HANDOFF,
            event_status=EventStatus.SUCCESS,
            event_data={
                "to_agent": to_agent,
                "handoff_reason": handoff_reason,
                "data_passed": data_passed,
                "handoff_at": datetime.utcnow().isoformat()
            }
        ))

# ============================================================================
# Agent Instrumentation Decorator
# ============================================================================

def monitor_agent(agent_id: str):
    """Decorator to add monitoring capabilities to agent classes"""

    def decorator(agent_class):
        """Apply monitoring to agent class"""

        # Add observability interfaces to agent
        original_init = agent_class.__init__

        def __init__(self, *args, **kwargs):
            # Call original init
            original_init(self, *args, **kwargs)

            # Add observability components (will be injected by DI container)
            self.agent_id = agent_id
            self._metrics_collector: Optional[MetricsCollectorInterface] = None
            self._decision_tracer: Optional[DecisionTracerInterface] = None
            self._workflow_tracker: Optional[WorkflowTrackerInterface] = None
            self._alert_manager: Optional[AlertManagerInterface] = None

        def set_observability_components(
            self,
            metrics_collector: MetricsCollectorInterface,
            decision_tracer: DecisionTracerInterface,
            workflow_tracker: WorkflowTrackerInterface,
            alert_manager: AlertManagerInterface
        ):
            """Inject observability components (called by DI container)"""
            self._metrics_collector = metrics_collector
            self._decision_tracer = decision_tracer
            self._workflow_tracker = workflow_tracker
            self._alert_manager = alert_manager

        def workflow_trace(self, workflow_type: str, parent_workflow_id: Optional[uuid.UUID] = None):
            """Create workflow trace context manager"""
            if not all([self._metrics_collector, self._decision_tracer, self._workflow_tracker]):
                raise RuntimeError("Observability components not properly injected")

            return WorkflowTraceContext(
                workflow_tracker=self._workflow_tracker,
                metrics_collector=self._metrics_collector,
                decision_tracer=self._decision_tracer,
                agent_id=self.agent_id,
                workflow_type=workflow_type,
                parent_workflow_id=parent_workflow_id
            )

        async def record_metric(self, metric_name: str, value: float, labels: Optional[Dict[str, Any]] = None):
            """Record a metric for this agent"""
            if self._metrics_collector:
                await self._metrics_collector.record_metric(
                    self.agent_id,
                    MetricPoint(metric_name=metric_name, metric_value=value, labels=labels)
                )

        async def log_decision(
            self,
            decision_type: str,
            context: Dict[str, Any],
            alternatives: List[Dict[str, Any]],
            confidence: float,
            reasoning: str,
            workflow_id: Optional[uuid.UUID] = None
        ):
            """Log a decision made by this agent"""
            if self._decision_tracer:
                decision = DecisionContext(
                    decision_type=decision_type,
                    decision_context=context,
                    alternatives_considered=alternatives,
                    confidence_score=confidence,
                    reasoning=reasoning,
                    workflow_id=workflow_id
                )
                await self._decision_tracer.log_decision(self.agent_id, decision)

        # Add methods to agent class
        agent_class.__init__ = __init__
        agent_class.set_observability_components = set_observability_components
        agent_class.workflow_trace = workflow_trace
        agent_class.record_metric = record_metric
        agent_class.log_decision = log_decision

        # Add agent_id as class attribute
        agent_class.AGENT_ID = agent_id

        return agent_class

    return decorator

# ============================================================================
# Example Usage Contract
# ============================================================================

"""
Example of how agents should use the observability framework:

@monitor_agent("email_processor")
class EmailProcessingAgent:
    async def process_nightly_emails(self, emails: List[Email]):
        async with self.workflow_trace("nightly_batch") as tracer:
            await tracer.record_metric("batch_size", len(emails))

            async with tracer.stage("entity_extraction"):
                entities_found = 0

                for email in emails:
                    entities = await self.extract_entities(email)
                    entities_found += len(entities)

                    # Log decision with context
                    await tracer.log_decision(
                        decision_type="entity_extraction",
                        decision_context={
                            "email_id": email.id,
                            "subject": email.subject,
                            "text_length": len(email.text)
                        },
                        alternatives=[
                            {"model": "spacy_large", "confidence": entities.confidence},
                            {"model": "transformer", "confidence": 0.82}
                        ],
                        confidence=entities.confidence,
                        reasoning="Used spaCy large model for high accuracy"
                    )

                await tracer.record_metric("entities_found", entities_found)

            async with tracer.stage("project_categorization"):
                # Project categorization logic
                categorized = await self.categorize_emails(emails)

                # Handoff to next agent
                await tracer.log_handoff(
                    to_agent="project_organizer",
                    data_passed={"categorized_emails": len(categorized)},
                    handoff_reason="categorization_complete"
                )

            return categorized

# Agent initialization with dependency injection
email_agent = EmailProcessingAgent()
email_agent.set_observability_components(
    metrics_collector=metrics_collector_impl,
    decision_tracer=decision_tracer_impl,
    workflow_tracker=workflow_tracker_impl,
    alert_manager=alert_manager_impl
)
"""