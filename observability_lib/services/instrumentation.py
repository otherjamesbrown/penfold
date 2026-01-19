"""Agent instrumentation services for the Penfold observability framework.

This module provides the core instrumentation capabilities including the
@monitor_agent decorator and WorkflowTraceContext async context manager.
These components enable comprehensive monitoring and tracing of autonomous
AI agents with minimal performance overhead.

The implementation follows the interface contract defined in the contracts
module and supports dependency injection for observability components.

Key Components:
    - @monitor_agent: Class decorator that adds observability methods to agents
    - WorkflowTraceContext: Async context manager for workflow execution tracing
    - Minimal performance overhead (<5% target)
    - Full async/await support
    - Comprehensive error handling and validation

Example Usage:
    @monitor_agent("email_processor")
    class EmailProcessingAgent:
        async def process_emails(self, emails):
            async with self.workflow_trace("nightly_batch") as tracer:
                async with tracer.stage("entity_extraction"):
                    # Processing logic here
                    await tracer.log_decision(
                        decision_type="entity_extraction",
                        decision_context={"email_count": len(emails)},
                        alternatives=[...],
                        confidence=0.95,
                        reasoning="High confidence extraction"
                    )

                    await tracer.log_handoff(
                        to_agent="categorizer",
                        data_passed={"processed_count": len(emails)},
                        handoff_reason="extraction_complete"
                    )
"""

import asyncio
import functools
import time
import uuid
from contextlib import asynccontextmanager
from datetime import datetime
from typing import Any, Dict, List, Optional, Type, TypeVar, Union
from weakref import WeakKeyDictionary

from ..models.agent_metrics import (
    MetricType,
    EventType,
    EventStatus,
    AlertStatus,
)

# Import the interface contracts
from ..contracts.instrumentation_interface import (
    MetricPoint,
    DecisionContext,
    WorkflowEventData,
    MetricsCollectorInterface,
    DecisionTracerInterface,
    WorkflowTrackerInterface,
    AlertManagerInterface,
    WorkflowTraceContext as BaseWorkflowTraceContext,
)


# Type variable for generic class decoration
T = TypeVar('T')


class InstrumentationError(Exception):
    """Exception raised by instrumentation components."""
    pass


class WorkflowTraceContext(BaseWorkflowTraceContext):
    """Enhanced async context manager for workflow execution tracing.

    This implementation extends the base interface with additional safety features,
    performance optimizations, and comprehensive error handling.

    Features:
        - Automatic timing and metric collection
        - Stage-by-stage execution tracking
        - Decision logging with confidence scores
        - Agent handoff tracking
        - Nested workflow support
        - Comprehensive error handling and cleanup
        - Performance monitoring with minimal overhead

    The context manager ensures proper resource cleanup even if exceptions occur
    during workflow execution, maintaining data consistency and avoiding leaks.
    """

    def __init__(
        self,
        workflow_tracker: WorkflowTrackerInterface,
        metrics_collector: MetricsCollectorInterface,
        decision_tracer: DecisionTracerInterface,
        agent_id: str,
        workflow_type: str,
        parent_workflow_id: Optional[uuid.UUID] = None
    ):
        """Initialize workflow trace context with observability components.

        Args:
            workflow_tracker: Service for tracking workflow events
            metrics_collector: Service for collecting agent metrics
            decision_tracer: Service for tracing agent decisions
            agent_id: Unique identifier for the agent executing this workflow
            workflow_type: Type/category of the workflow being executed
            parent_workflow_id: Optional parent workflow for nested workflows

        Raises:
            InstrumentationError: If required components are missing or invalid
        """
        # Validate required arguments
        if not all([workflow_tracker, metrics_collector, decision_tracer]):
            raise InstrumentationError("All observability components are required")

        if not agent_id or not isinstance(agent_id, str):
            raise InstrumentationError("Valid agent_id is required")

        if not workflow_type or not isinstance(workflow_type, str):
            raise InstrumentationError("Valid workflow_type is required")

        super().__init__(
            workflow_tracker=workflow_tracker,
            metrics_collector=metrics_collector,
            decision_tracer=decision_tracer,
            agent_id=agent_id,
            workflow_type=workflow_type,
            parent_workflow_id=parent_workflow_id
        )

        # Enhanced state tracking
        self._stage_stack: List[Dict[str, Any]] = []
        self._metrics_buffer: List[MetricPoint] = []
        self._is_finalized = False
        self._context_lock = asyncio.Lock()

    async def __aenter__(self):
        """Start workflow tracing with enhanced error handling."""
        async with self._context_lock:
            try:
                # Delegate to parent implementation
                result = await super().__aenter__()

                # Record workflow start metric
                await self._record_metric_safe(
                    "workflow_started",
                    1.0,
                    labels={
                        "workflow_type": self.workflow_type,
                        "agent_id": self.agent_id,
                        "workflow_id": str(self.workflow_id)
                    }
                )

                return result

            except Exception as e:
                # Ensure cleanup on initialization failure
                await self._cleanup_on_error(e)
                raise InstrumentationError(f"Failed to start workflow trace: {e}") from e

    async def __aexit__(self, exc_type, exc_val, exc_tb):
        """Complete workflow tracing with proper cleanup."""
        async with self._context_lock:
            if self._is_finalized:
                return  # Already cleaned up

            try:
                # Flush any buffered metrics before completion
                await self._flush_metrics_buffer()

                # Delegate to parent implementation
                await super().__aexit__(exc_type, exc_val, exc_tb)

                # Record final metrics
                final_status = "failure" if exc_type else "success"
                await self._record_metric_safe(
                    "workflow_completed",
                    1.0,
                    labels={
                        "workflow_type": self.workflow_type,
                        "status": final_status,
                        "stage_count": len(self.stage_timings),
                        "workflow_id": str(self.workflow_id)
                    }
                )

            except Exception as cleanup_error:
                # Log cleanup errors but don't suppress original exception
                await self._log_error(
                    "workflow_cleanup_failed",
                    str(cleanup_error),
                    {"original_error": str(exc_val) if exc_val else None}
                )

            finally:
                self._is_finalized = True

    @asynccontextmanager
    async def stage(self, stage_name: str, expected_duration_ms: Optional[float] = None):
        """Enhanced stage context manager with performance tracking.

        Args:
            stage_name: Name of the workflow stage
            expected_duration_ms: Optional expected duration for performance comparison

        Yields:
            Stage context that can be used for stage-specific operations

        Raises:
            InstrumentationError: If stage tracking fails
        """
        if self._is_finalized:
            raise InstrumentationError("Cannot start stage on finalized workflow")

        stage_start_time = time.time()
        stage_context = {
            "stage_name": stage_name,
            "started_at": datetime.utcnow(),
            "expected_duration_ms": expected_duration_ms
        }

        # Push stage onto stack for nested stage tracking
        self._stage_stack.append(stage_context)

        try:
            # Use parent stage implementation but with additional tracking
            async with super().stage(stage_name):
                yield stage_context

            # Stage completed successfully
            stage_end_time = time.time()
            actual_duration_ms = (stage_end_time - stage_start_time) * 1000

            # Compare with expected duration if provided
            if expected_duration_ms:
                performance_ratio = actual_duration_ms / expected_duration_ms
                await self._record_metric_safe(
                    "stage_performance_ratio",
                    performance_ratio,
                    labels={
                        "stage_name": stage_name,
                        "workflow_id": str(self.workflow_id)
                    }
                )

                # Alert if significantly slower than expected
                if performance_ratio > 2.0:
                    await self._log_warning(
                        "stage_performance_degradation",
                        f"Stage {stage_name} took {performance_ratio:.2f}x longer than expected",
                        {
                            "stage_name": stage_name,
                            "expected_ms": expected_duration_ms,
                            "actual_ms": actual_duration_ms,
                            "performance_ratio": performance_ratio
                        }
                    )

        except Exception as e:
            # Stage failed - record failure metrics
            stage_end_time = time.time()
            actual_duration_ms = (stage_end_time - stage_start_time) * 1000

            await self._record_metric_safe(
                "stage_failure",
                1.0,
                labels={
                    "stage_name": stage_name,
                    "error_type": type(e).__name__,
                    "workflow_id": str(self.workflow_id)
                }
            )

            raise

        finally:
            # Pop stage from stack
            if self._stage_stack:
                self._stage_stack.pop()

    async def log_decision(
        self,
        decision_type: str,
        decision_context: Dict[str, Any],
        alternatives: List[Dict[str, Any]],
        confidence: float,
        reasoning: str
    ) -> uuid.UUID:
        """Enhanced decision logging with validation and metrics.

        Args:
            decision_type: Type of decision being made
            decision_context: Context data that influenced the decision
            alternatives: List of alternative options considered
            confidence: Confidence score for the decision (0.0-1.0)
            reasoning: Human-readable explanation of the decision

        Returns:
            UUID of the logged decision record

        Raises:
            InstrumentationError: If decision logging fails
        """
        # Validate inputs
        if not decision_type:
            raise InstrumentationError("Decision type is required")

        if not decision_context:
            raise InstrumentationError("Decision context is required")

        if not alternatives:
            raise InstrumentationError("Alternatives list cannot be empty")

        if not 0.0 <= confidence <= 1.0:
            raise InstrumentationError("Confidence must be between 0.0 and 1.0")

        try:
            # Use parent implementation
            decision_id = await super().log_decision(
                decision_type, decision_context, alternatives, confidence, reasoning
            )

            # Record decision quality metrics
            await self._record_metric_safe(
                "decision_count",
                1.0,
                labels={
                    "decision_type": decision_type,
                    "confidence_bucket": self._get_confidence_bucket(confidence),
                    "workflow_id": str(self.workflow_id)
                }
            )

            # Track low-confidence decisions for review
            if confidence < 0.7:
                await self._log_warning(
                    "low_confidence_decision",
                    f"Decision {decision_type} has low confidence: {confidence:.2f}",
                    {
                        "decision_type": decision_type,
                        "confidence": confidence,
                        "workflow_id": str(self.workflow_id),
                        "alternatives_count": len(alternatives)
                    }
                )

            return decision_id

        except Exception as e:
            await self._log_error(
                "decision_logging_failed",
                str(e),
                {"decision_type": decision_type, "confidence": confidence}
            )
            raise InstrumentationError(f"Failed to log decision: {e}") from e

    async def log_handoff(
        self,
        to_agent: str,
        data_passed: Dict[str, Any],
        handoff_reason: str
    ) -> None:
        """Enhanced agent handoff logging with validation.

        Args:
            to_agent: ID of the agent receiving the handoff
            data_passed: Data being passed to the next agent
            handoff_reason: Reason for the handoff

        Raises:
            InstrumentationError: If handoff logging fails
        """
        # Validate inputs
        if not to_agent:
            raise InstrumentationError("Target agent ID is required")

        if not handoff_reason:
            raise InstrumentationError("Handoff reason is required")

        try:
            # Use parent implementation
            await super().log_handoff(to_agent, data_passed, handoff_reason)

            # Record handoff metrics
            await self._record_metric_safe(
                "agent_handoff",
                1.0,
                labels={
                    "from_agent": self.agent_id,
                    "to_agent": to_agent,
                    "workflow_id": str(self.workflow_id),
                    "data_size": len(str(data_passed))
                }
            )

        except Exception as e:
            await self._log_error(
                "handoff_logging_failed",
                str(e),
                {"to_agent": to_agent, "handoff_reason": handoff_reason}
            )
            raise InstrumentationError(f"Failed to log handoff: {e}") from e

    async def _record_metric_safe(
        self,
        metric_name: str,
        value: float,
        labels: Optional[Dict[str, Any]] = None
    ) -> None:
        """Safely record a metric with error handling."""
        try:
            metric = MetricPoint(
                metric_name=metric_name,
                metric_value=value,
                metric_type=MetricType.GAUGE,
                labels=labels or {}
            )

            # Buffer metrics for batch processing
            self._metrics_buffer.append(metric)

            # Flush buffer if it gets too large
            if len(self._metrics_buffer) >= 10:
                await self._flush_metrics_buffer()

        except Exception as e:
            # Don't let metric recording failures break the workflow
            await self._log_error(
                "metric_recording_failed",
                str(e),
                {"metric_name": metric_name, "value": value}
            )

    async def _flush_metrics_buffer(self) -> None:
        """Flush buffered metrics to the collector."""
        if not self._metrics_buffer:
            return

        try:
            await self.metrics_collector.record_metrics_batch(
                self.agent_id,
                self._metrics_buffer.copy()
            )
            self._metrics_buffer.clear()

        except Exception as e:
            await self._log_error(
                "metrics_flush_failed",
                str(e),
                {"buffer_size": len(self._metrics_buffer)}
            )

    async def _log_error(
        self,
        event: str,
        message: str,
        context: Optional[Dict[str, Any]] = None
    ) -> None:
        """Log an error event safely."""
        try:
            await self.workflow_tracker.log_workflow_event(WorkflowEventData(
                workflow_id=self.workflow_id,
                agent_id=self.agent_id,
                event_type=EventType.ERROR,
                event_status=EventStatus.FAILURE,
                event_data={
                    "event": event,
                    "message": message,
                    "context": context or {},
                    "timestamp": datetime.utcnow().isoformat()
                }
            ))
        except Exception:
            # Last resort - can't even log the error
            pass

    async def _log_warning(
        self,
        event: str,
        message: str,
        context: Optional[Dict[str, Any]] = None
    ) -> None:
        """Log a warning event."""
        try:
            await self.workflow_tracker.log_workflow_event(WorkflowEventData(
                workflow_id=self.workflow_id,
                agent_id=self.agent_id,
                event_type=EventType.STAGE_COMPLETED,  # Use stage completed for warnings
                event_status=EventStatus.RETRY,  # Use retry status for warnings
                event_data={
                    "event": event,
                    "message": message,
                    "context": context or {},
                    "timestamp": datetime.utcnow().isoformat(),
                    "level": "WARNING"
                }
            ))
        except Exception as e:
            await self._log_error("warning_logging_failed", str(e), {"original_event": event})

    async def _cleanup_on_error(self, error: Exception) -> None:
        """Cleanup resources when initialization fails."""
        try:
            if hasattr(self, '_metrics_buffer'):
                self._metrics_buffer.clear()
            self._is_finalized = True
        except Exception:
            # Best effort cleanup
            pass

    def _get_confidence_bucket(self, confidence: float) -> str:
        """Get confidence bucket for metrics grouping."""
        if confidence >= 0.9:
            return "high"
        elif confidence >= 0.7:
            return "medium"
        else:
            return "low"


def monitor_agent(agent_id: str):
    """Enhanced decorator to add comprehensive monitoring capabilities to agent classes.

    This decorator extends agent classes with observability methods and ensures
    proper integration with the observability framework while maintaining
    minimal performance overhead.

    Features:
        - Dependency injection for observability components
        - Workflow tracing context management
        - Metric recording with buffering
        - Decision logging with validation
        - Error handling and recovery
        - Performance monitoring
        - Thread-safe operation

    Args:
        agent_id: Unique identifier for the agent (must be valid string)

    Returns:
        Decorated class with observability capabilities

    Raises:
        InstrumentationError: If agent_id is invalid or decoration fails

    Example:
        @monitor_agent("email_processor")
        class EmailProcessingAgent:
            async def process_emails(self, emails):
                async with self.workflow_trace("nightly_batch") as tracer:
                    # Processing logic with full observability
                    pass
    """

    # Validate agent_id at decoration time
    if not agent_id or not isinstance(agent_id, str):
        raise InstrumentationError("Valid agent_id string is required")

    if len(agent_id) < 3 or len(agent_id) > 100:
        raise InstrumentationError("agent_id must be between 3 and 100 characters")

    # Only allow alphanumeric characters and underscores
    if not all(c.isalnum() or c == '_' for c in agent_id):
        raise InstrumentationError(
            "agent_id must contain only alphanumeric characters and underscores"
        )

    def decorator(agent_class: Type[T]) -> Type[T]:
        """Apply monitoring to agent class with enhanced features."""

        if not hasattr(agent_class, '__init__'):
            raise InstrumentationError("Agent class must have __init__ method")

        # Store original methods to avoid infinite recursion
        original_init = agent_class.__init__

        # Weak reference dictionary to track observability state per instance
        _observability_state: WeakKeyDictionary = WeakKeyDictionary()

        def __init__(self, *args, **kwargs):
            """Enhanced initialization with observability setup."""
            # Call original constructor first
            original_init(self, *args, **kwargs)

            # Initialize observability state
            self.agent_id = agent_id
            self._metrics_collector: Optional[MetricsCollectorInterface] = None
            self._decision_tracer: Optional[DecisionTracerInterface] = None
            self._workflow_tracker: Optional[WorkflowTrackerInterface] = None
            self._alert_manager: Optional[AlertManagerInterface] = None

            # Track observability readiness
            _observability_state[self] = {
                'initialized': True,
                'components_injected': False,
                'active_workflows': set(),
                'metrics_buffer': [],
                'last_activity': datetime.utcnow()
            }

        def set_observability_components(
            self,
            metrics_collector: MetricsCollectorInterface,
            decision_tracer: DecisionTracerInterface,
            workflow_tracker: WorkflowTrackerInterface,
            alert_manager: AlertManagerInterface
        ):
            """Inject observability components with validation.

            This method is called by the dependency injection container
            to provide the agent with all required observability services.

            Args:
                metrics_collector: Service for collecting agent metrics
                decision_tracer: Service for tracing agent decisions
                workflow_tracker: Service for tracking workflow execution
                alert_manager: Service for alert management

            Raises:
                InstrumentationError: If components are invalid or injection fails
            """
            # Validate all components are provided
            if not all([metrics_collector, decision_tracer, workflow_tracker, alert_manager]):
                raise InstrumentationError("All observability components are required")

            # Validate component interfaces
            required_interfaces = [
                (metrics_collector, MetricsCollectorInterface),
                (decision_tracer, DecisionTracerInterface),
                (workflow_tracker, WorkflowTrackerInterface),
                (alert_manager, AlertManagerInterface)
            ]

            for component, interface in required_interfaces:
                if not isinstance(component, interface):
                    raise InstrumentationError(
                        f"Component must implement {interface.__name__}"
                    )

            # Inject components
            self._metrics_collector = metrics_collector
            self._decision_tracer = decision_tracer
            self._workflow_tracker = workflow_tracker
            self._alert_manager = alert_manager

            # Update state tracking
            if self in _observability_state:
                _observability_state[self]['components_injected'] = True
                _observability_state[self]['last_activity'] = datetime.utcnow()

        def workflow_trace(
            self,
            workflow_type: str,
            parent_workflow_id: Optional[uuid.UUID] = None
        ) -> WorkflowTraceContext:
            """Create enhanced workflow trace context manager.

            Args:
                workflow_type: Type/category of workflow being executed
                parent_workflow_id: Optional parent workflow for nested workflows

            Returns:
                WorkflowTraceContext for tracking workflow execution

            Raises:
                InstrumentationError: If observability components not injected
                    or workflow creation fails
            """
            # Verify observability components are injected
            if not all([
                self._metrics_collector,
                self._decision_tracer,
                self._workflow_tracker
            ]):
                raise InstrumentationError(
                    "Observability components not properly injected. "
                    "Call set_observability_components() first."
                )

            # Validate workflow_type
            if not workflow_type or not isinstance(workflow_type, str):
                raise InstrumentationError("Valid workflow_type is required")

            try:
                # Create enhanced workflow context
                context = WorkflowTraceContext(
                    workflow_tracker=self._workflow_tracker,
                    metrics_collector=self._metrics_collector,
                    decision_tracer=self._decision_tracer,
                    agent_id=self.agent_id,
                    workflow_type=workflow_type,
                    parent_workflow_id=parent_workflow_id
                )

                # Track active workflow
                if self in _observability_state:
                    _observability_state[self]['last_activity'] = datetime.utcnow()

                return context

            except Exception as e:
                raise InstrumentationError(f"Failed to create workflow trace: {e}") from e

        async def record_metric(
            self,
            metric_name: str,
            value: float,
            metric_type: MetricType = MetricType.GAUGE,
            labels: Optional[Dict[str, Any]] = None
        ):
            """Record a metric for this agent with validation.

            Args:
                metric_name: Name of the metric to record
                value: Numeric value of the metric
                metric_type: Type of metric (counter, gauge, histogram)
                labels: Optional labels for metric dimensions

            Raises:
                InstrumentationError: If metric recording fails
            """
            if not self._metrics_collector:
                # Silently skip if metrics collector not available
                return

            # Validate inputs
            if not metric_name or not isinstance(metric_name, str):
                raise InstrumentationError("Valid metric name is required")

            if not isinstance(value, (int, float)) or not (value == value):  # Check for NaN
                raise InstrumentationError("Metric value must be a finite number")

            try:
                metric = MetricPoint(
                    metric_name=metric_name,
                    metric_value=float(value),
                    metric_type=metric_type,
                    labels=labels or {}
                )

                await self._metrics_collector.record_metric(self.agent_id, metric)

                # Update activity tracking
                if self in _observability_state:
                    _observability_state[self]['last_activity'] = datetime.utcnow()

            except Exception as e:
                raise InstrumentationError(f"Failed to record metric: {e}") from e

        async def log_decision(
            self,
            decision_type: str,
            context: Dict[str, Any],
            alternatives: List[Dict[str, Any]],
            confidence: float,
            reasoning: str,
            workflow_id: Optional[uuid.UUID] = None
        ) -> Optional[uuid.UUID]:
            """Log a decision made by this agent with validation.

            Args:
                decision_type: Type of decision being made
                context: Context data that influenced the decision
                alternatives: List of alternative options considered
                confidence: Confidence score for the decision (0.0-1.0)
                reasoning: Human-readable explanation
                workflow_id: Optional workflow ID for context

            Returns:
                UUID of the decision record if logging succeeds

            Raises:
                InstrumentationError: If decision logging fails
            """
            if not self._decision_tracer:
                return None  # Skip if decision tracer not available

            # Validate inputs
            if not decision_type or not isinstance(decision_type, str):
                raise InstrumentationError("Valid decision type is required")

            if not context:
                raise InstrumentationError("Decision context is required")

            if not alternatives or not isinstance(alternatives, list):
                raise InstrumentationError("Alternatives list is required")

            if not 0.0 <= confidence <= 1.0:
                raise InstrumentationError("Confidence must be between 0.0 and 1.0")

            try:
                decision = DecisionContext(
                    decision_type=decision_type,
                    decision_context=context,
                    alternatives_considered=alternatives,
                    confidence_score=confidence,
                    reasoning=reasoning or "",
                    workflow_id=workflow_id
                )

                decision_id = await self._decision_tracer.log_decision(
                    self.agent_id,
                    decision
                )

                # Update activity tracking
                if self in _observability_state:
                    _observability_state[self]['last_activity'] = datetime.utcnow()

                return decision_id

            except Exception as e:
                raise InstrumentationError(f"Failed to log decision: {e}") from e

        def get_observability_status(self) -> Dict[str, Any]:
            """Get current observability status for this agent instance.

            Returns:
                Dictionary with observability status information
            """
            state = _observability_state.get(self, {})

            return {
                "agent_id": self.agent_id,
                "initialized": state.get('initialized', False),
                "components_injected": state.get('components_injected', False),
                "active_workflows": len(state.get('active_workflows', set())),
                "last_activity": state.get('last_activity'),
                "metrics_collector_ready": self._metrics_collector is not None,
                "decision_tracer_ready": self._decision_tracer is not None,
                "workflow_tracker_ready": self._workflow_tracker is not None,
                "alert_manager_ready": self._alert_manager is not None
            }

        # Add methods to agent class
        agent_class.__init__ = __init__
        agent_class.set_observability_components = set_observability_components
        agent_class.workflow_trace = workflow_trace
        agent_class.record_metric = record_metric
        agent_class.log_decision = log_decision
        agent_class.get_observability_status = get_observability_status

        # Add agent_id as class attribute for identification
        agent_class.AGENT_ID = agent_id

        # Add metadata for inspection
        agent_class.__observability_enabled__ = True
        agent_class.__observability_agent_id__ = agent_id
        agent_class.__observability_version__ = "1.0.0"

        return agent_class

    return decorator


# Utility functions for instrumentation management

def is_agent_monitored(agent_class_or_instance) -> bool:
    """Check if a class or instance has monitoring enabled.

    Args:
        agent_class_or_instance: Agent class or instance to check

    Returns:
        True if the agent has monitoring enabled, False otherwise
    """
    return getattr(agent_class_or_instance, '__observability_enabled__', False)


def get_agent_id(agent_class_or_instance) -> Optional[str]:
    """Get the agent ID from a monitored class or instance.

    Args:
        agent_class_or_instance: Agent class or instance

    Returns:
        Agent ID if monitoring is enabled, None otherwise
    """
    return getattr(agent_class_or_instance, '__observability_agent_id__', None)


def get_observability_metadata(agent_class_or_instance) -> Dict[str, Any]:
    """Get observability metadata from a monitored agent.

    Args:
        agent_class_or_instance: Agent class or instance

    Returns:
        Dictionary with observability metadata
    """
    if not is_agent_monitored(agent_class_or_instance):
        return {"enabled": False}

    return {
        "enabled": True,
        "agent_id": getattr(agent_class_or_instance, '__observability_agent_id__', None),
        "version": getattr(agent_class_or_instance, '__observability_version__', None),
        "has_workflow_trace": hasattr(agent_class_or_instance, 'workflow_trace'),
        "has_record_metric": hasattr(agent_class_or_instance, 'record_metric'),
        "has_log_decision": hasattr(agent_class_or_instance, 'log_decision'),
        "has_status_check": hasattr(agent_class_or_instance, 'get_observability_status')
    }


# Export public API
__all__ = [
    'InstrumentationError',
    'WorkflowTraceContext',
    'monitor_agent',
    'is_agent_monitored',
    'get_agent_id',
    'get_observability_metadata'
]