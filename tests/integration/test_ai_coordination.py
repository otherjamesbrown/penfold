"""Integration tests for AI Coordination framework.

Tests the complete coordination workflow including:
- Model registration and selection
- Multi-model parallel processing
- Ensemble result combination
- Confidence-based escalation
- Performance tracking
"""

import asyncio
import pytest
import uuid
from datetime import datetime, timezone
from typing import Dict, Any, List
from unittest.mock import AsyncMock, MagicMock

from penf_lib.ai_coordination.coordinator import ModelCoordinator
from penf_lib.ai_coordination.ensemble import EnsembleCombiner, EnsembleResult
from penf_lib.ai_coordination.escalation import EscalationManager
from penf_lib.ai_coordination.performance import PerformanceTracker
from penf_lib.ai_coordination.models import (
    ModelProfile, ModelType, ModelCapability, DEFAULT_MODEL_PROFILES
)


class MockAsyncSession:
    """Mock async session for testing."""

    def __init__(self):
        self.executed_queries = []

    async def execute(self, query):
        self.executed_queries.append(query)
        return MagicMock()

    async def commit(self):
        pass

    async def rollback(self):
        pass


class MockEventPublisher:
    """Mock event publisher for testing."""

    def __init__(self):
        self.published_events = []

    async def publish(self, event_type: str, event_data: Dict[str, Any], channel: str = None) -> str:
        event_id = str(uuid.uuid4())
        self.published_events.append({
            "event_id": event_id,
            "event_type": event_type,
            "event_data": event_data,
            "channel": channel
        })
        return event_id


class MockJobManager:
    """Mock job manager for testing."""

    def __init__(self):
        self.jobs = {}
        self.job_results = {}
        self.subscription_manager = MockSubscriptionManager()

    async def create_job(self, event_id: str, processor_name: str, job_type: str,
                        input_data: Dict[str, Any], priority: int, metadata: Dict[str, Any]) -> str:
        job_id = str(uuid.uuid4())
        job = MockJob(
            job_id=job_id,
            event_id=event_id,
            processor_name=processor_name,
            job_type=job_type,
            input_data=input_data,
            priority=priority,
            metadata=metadata
        )
        self.jobs[job_id] = job
        return job_id

    async def get_job(self, job_id: str):
        return self.jobs.get(job_id)

    async def get_job_results(self, job_id: str):
        return self.job_results.get(job_id, [])

    def complete_job(self, job_id: str, results: List[Dict[str, Any]]):
        """Helper method to simulate job completion."""
        if job_id in self.jobs:
            self.jobs[job_id].status = "completed"
            self.jobs[job_id].completed_at = datetime.now(timezone.utc)
            self.job_results[job_id] = [MockJobResult(**result) for result in results]


class MockSubscriptionManager:
    """Mock subscription manager for testing."""

    def __init__(self):
        self.subscriptions = []

    async def create_subscription(self, subscriber_id: str, subscription_name: str,
                                event_channels: List[str], event_types: List[str],
                                filter_criteria: Dict[str, Any]) -> str:
        subscription_id = str(uuid.uuid4())
        self.subscriptions.append({
            "subscription_id": subscription_id,
            "subscriber_id": subscriber_id,
            "subscription_name": subscription_name,
            "event_channels": event_channels,
            "event_types": event_types,
            "filter_criteria": filter_criteria
        })
        return subscription_id


class MockJob:
    """Mock job for testing."""

    def __init__(self, job_id: str, event_id: str, processor_name: str, job_type: str,
                 input_data: Dict[str, Any], priority: int, metadata: Dict[str, Any]):
        self.job_id = job_id
        self.event_id = event_id
        self.processor_name = processor_name
        self.job_type = job_type
        self.input_data = input_data
        self.priority = priority
        self.metadata = metadata
        self.status = "pending"
        self.created_at = datetime.now(timezone.utc)
        self.completed_at = None
        self.failed_at = None
        self.updated_at = datetime.now(timezone.utc)
        self.error_message = None


class MockJobResult:
    """Mock job result for testing."""

    def __init__(self, result_data: Dict[str, Any], confidence_score: float,
                 model_name: str, processing_time_ms: int):
        self.result_data = result_data
        self.confidence_score = confidence_score
        self.model_name = model_name
        self.processing_time_ms = processing_time_ms


@pytest.fixture
def mock_session():
    """Create mock async session."""
    return MockAsyncSession()


@pytest.fixture
def mock_event_publisher():
    """Create mock event publisher."""
    return MockEventPublisher()


@pytest.fixture
def mock_job_manager():
    """Create mock job manager."""
    return MockJobManager()


@pytest.fixture
async def coordinator(mock_session, mock_event_publisher, mock_job_manager):
    """Create ModelCoordinator instance for testing."""
    coordinator = ModelCoordinator(
        session=mock_session,
        event_publisher=mock_event_publisher,
        job_manager=mock_job_manager
    )

    # Register some default models
    await coordinator.register_model(
        model_id="llama-3.1-8b",
        model_profile=DEFAULT_MODEL_PROFILES["llama-3.1-8b"],
        event_types=["email", "document"]
    )

    await coordinator.register_model(
        model_id="gpt-4",
        model_profile=DEFAULT_MODEL_PROFILES["gpt-4"],
        event_types=["email", "document"]
    )

    return coordinator


class TestModelCoordinator:
    """Test cases for ModelCoordinator functionality."""

    async def test_register_model(self, coordinator, mock_job_manager):
        """Test model registration."""
        model_profile = ModelProfile(
            model_id="test-model",
            name="Test Model",
            model_type=ModelType.LOCAL,
            capabilities=[ModelCapability.TEXT_GENERATION],
            supported_content_types=["email"],
            max_input_tokens=4096,
            avg_response_time_ms=1000,
            cost_per_1k_tokens=0.0,
            confidence_reliability=0.8
        )

        success = await coordinator.register_model(
            model_id="test-model",
            model_profile=model_profile,
            event_types=["email"]
        )

        assert success is True
        assert "test-model" in coordinator.registered_models
        assert len(mock_job_manager.subscription_manager.subscriptions) == 3  # 2 default + 1 new

    async def test_coordinate_processing(self, coordinator):
        """Test coordination workflow initiation."""
        content_data = {
            "subject": "Test email",
            "content": "This is a test email for coordination.",
            "sender": "test@example.com"
        }

        coordination_id = await coordinator.coordinate_processing(
            content_id="email-123",
            content_type="email",
            content_data=content_data,
            require_minimum_models=1
        )

        assert coordination_id is not None
        assert coordination_id in coordinator.active_coordinations

        coordination = coordinator.active_coordinations[coordination_id]
        assert coordination["content_id"] == "email-123"
        assert coordination["content_type"] == "email"
        assert coordination["status"] == "processing"
        assert len(coordination["selected_models"]) >= 1

    async def test_coordination_status_tracking(self, coordinator, mock_job_manager):
        """Test coordination status tracking and updates."""
        content_data = {"content": "Test content"}

        coordination_id = await coordinator.coordinate_processing(
            content_id="test-123",
            content_type="email",
            content_data=content_data
        )

        # Simulate job completion
        coordination = coordinator.active_coordinations[coordination_id]
        job_id = coordination["job_ids"][0]

        mock_job_manager.complete_job(job_id, [{
            "result_data": {"extracted_entities": ["person:John", "company:Acme"]},
            "confidence_score": 0.85,
            "model_name": "llama-3.1-8b",
            "processing_time_ms": 1500
        }])

        status = await coordinator.get_coordination_status(coordination_id)

        assert status["coordination_id"] == coordination_id
        assert status["completed_jobs"] == 1
        assert status["results_available"] is True

    async def test_wait_for_coordination_completion(self, coordinator, mock_job_manager):
        """Test waiting for coordination to complete."""
        content_data = {"content": "Test content for completion"}

        coordination_id = await coordinator.coordinate_processing(
            content_id="test-complete-123",
            content_type="email",
            content_data=content_data
        )

        # Simulate completing jobs in the background
        async def complete_jobs():
            await asyncio.sleep(0.1)  # Small delay
            coordination = coordinator.active_coordinations[coordination_id]

            for i, job_id in enumerate(coordination["job_ids"]):
                mock_job_manager.complete_job(job_id, [{
                    "result_data": {"result": f"Model {i} result"},
                    "confidence_score": 0.8 + (i * 0.1),
                    "model_name": f"model-{i}",
                    "processing_time_ms": 1000 + (i * 100)
                }])

        # Start background task to complete jobs
        asyncio.create_task(complete_jobs())

        # Wait for coordination to complete
        results = await coordinator.wait_for_coordination(
            coordination_id=coordination_id,
            timeout=5,  # 5 second timeout
            min_results=1
        )

        assert results["coordination_id"] == coordination_id
        assert results["status"] in ["processing", "timeout"]  # May not complete all
        assert len(results["individual_results"]) >= 1

    async def test_model_selection_optimization(self, coordinator):
        """Test that model selection uses performance optimization."""
        # Register additional models with different capabilities
        model_profile = ModelProfile(
            model_id="specialized-model",
            name="Specialized Model",
            model_type=ModelType.CLOUD,
            capabilities=[ModelCapability.ENTITY_EXTRACTION],
            supported_content_types=["email"],
            max_input_tokens=8192,
            avg_response_time_ms=800,
            cost_per_1k_tokens=0.01,
            confidence_reliability=0.95,
            priority=1  # Higher priority
        )

        await coordinator.register_model(
            model_id="specialized-model",
            model_profile=model_profile,
            event_types=["email"]
        )

        content_data = {"content": "Email requiring entity extraction"}

        coordination_id = await coordinator.coordinate_processing(
            content_id="optimize-test-123",
            content_type="email",
            content_data=content_data
        )

        coordination = coordinator.active_coordinations[coordination_id]
        selected_models = coordination["selected_models"]

        # Should include specialized model due to optimization
        assert len(selected_models) >= 2
        assert any("model" in model_id.lower() for model_id in selected_models)


class TestEnsembleCombiner:
    """Test cases for EnsembleCombiner functionality."""

    def test_ensemble_initialization(self):
        """Test ensemble combiner initialization."""
        combiner = EnsembleCombiner()
        assert combiner is not None

    async def test_weighted_average_combination(self):
        """Test weighted average ensemble combination."""
        combiner = EnsembleCombiner()

        individual_results = [
            {
                "processor_name": "llama-3.1-8b",
                "result_data": {"sentiment": "positive", "confidence": 0.8},
                "confidence_score": 0.8,
                "model_name": "llama-3.1-8b",
                "processing_time_ms": 1200
            },
            {
                "processor_name": "gpt-4",
                "result_data": {"sentiment": "positive", "confidence": 0.9},
                "confidence_score": 0.9,
                "model_name": "gpt-4",
                "processing_time_ms": 2000
            },
            {
                "processor_name": "claude-3-sonnet",
                "result_data": {"sentiment": "neutral", "confidence": 0.7},
                "confidence_score": 0.7,
                "model_name": "claude-3-sonnet",
                "processing_time_ms": 1800
            }
        ]

        result = await combiner.combine_results(
            individual_results=individual_results,
            content_type="email",
            content_data={"content": "Test email content"}
        )

        assert isinstance(result, EnsembleResult)
        assert result.ensemble_id is not None
        assert result.confidence_score > 0
        assert result.combination_strategy == "weighted_average"
        assert result.consensus_strength >= 0
        assert len(result.supporting_results) == len(individual_results)

    async def test_confidence_voting_combination(self):
        """Test confidence voting combination strategy."""
        combiner = EnsembleCombiner()

        individual_results = [
            {
                "processor_name": "model-1",
                "result_data": {"category": "urgent", "priority": "high"},
                "confidence_score": 0.95,
                "model_name": "model-1",
                "processing_time_ms": 1000
            },
            {
                "processor_name": "model-2",
                "result_data": {"category": "urgent", "priority": "high"},
                "confidence_score": 0.85,
                "model_name": "model-2",
                "processing_time_ms": 1500
            },
            {
                "processor_name": "model-3",
                "result_data": {"category": "normal", "priority": "medium"},
                "confidence_score": 0.6,
                "model_name": "model-3",
                "processing_time_ms": 800
            }
        ]

        result = await combiner.combine_results(
            individual_results=individual_results,
            content_type="email",
            content_data={"content": "Email needing classification"},
            strategy="confidence_voting"
        )

        assert result.combination_strategy == "confidence_voting"
        # Higher confidence models should influence the result more
        assert result.confidence_score > 0.7


class TestIntegrationWorkflow:
    """Integration tests for complete AI coordination workflows."""

    async def test_complete_coordination_workflow(self, coordinator, mock_job_manager):
        """Test a complete end-to-end coordination workflow."""
        content_data = {
            "subject": "Quarterly Review Meeting",
            "content": "Please prepare the quarterly review slides for next week's board meeting.",
            "sender": "manager@company.com",
            "received_at": "2024-01-15T10:00:00Z"
        }

        # Start coordination
        coordination_id = await coordinator.coordinate_processing(
            content_id="email-quarterly-review",
            content_type="email",
            content_data=content_data,
            require_minimum_models=2
        )

        # Verify coordination started
        assert coordination_id in coordinator.active_coordinations
        coordination = coordinator.active_coordinations[coordination_id]
        assert len(coordination["job_ids"]) >= 2

        # Simulate model processing and completion
        for i, job_id in enumerate(coordination["job_ids"]):
            result_data = {
                "entities": [
                    {"type": "event", "value": "Quarterly Review Meeting", "confidence": 0.9},
                    {"type": "person", "value": "manager@company.com", "confidence": 0.85},
                    {"type": "deadline", "value": "next week", "confidence": 0.8}
                ],
                "sentiment": "neutral",
                "urgency": "medium",
                "action_items": [
                    "Prepare quarterly review slides",
                    "Schedule board meeting"
                ]
            }

            mock_job_manager.complete_job(job_id, [{
                "result_data": result_data,
                "confidence_score": 0.8 + (i * 0.05),
                "model_name": f"model-{i}",
                "processing_time_ms": 1500 + (i * 200)
            }])

        # Get final results
        results = await coordinator.wait_for_coordination(
            coordination_id=coordination_id,
            min_results=2,
            timeout=10
        )

        # Verify results
        assert results["coordination_id"] == coordination_id
        assert len(results["individual_results"]) >= 2
        assert results["performance_summary"]["completion_rate"] > 0

        # Check if ensemble result was created
        if len(results["individual_results"]) > 1:
            assert results["ensemble_result"] is not None

    async def test_coordination_with_failures(self, coordinator, mock_job_manager):
        """Test coordination handling when some models fail."""
        content_data = {"content": "Test content with partial failures"}

        coordination_id = await coordinator.coordinate_processing(
            content_id="test-failures",
            content_type="email",
            content_data=content_data,
            require_minimum_models=1
        )

        coordination = coordinator.active_coordinations[coordination_id]
        job_ids = coordination["job_ids"]

        # Complete first job successfully
        mock_job_manager.complete_job(job_ids[0], [{
            "result_data": {"result": "Success"},
            "confidence_score": 0.85,
            "model_name": "working-model",
            "processing_time_ms": 1200
        }])

        # Simulate second job failure
        if len(job_ids) > 1:
            failed_job = mock_job_manager.jobs[job_ids[1]]
            failed_job.status = "failed"
            failed_job.failed_at = datetime.now(timezone.utc)
            failed_job.error_message = "Model processing failed"

        # Wait for coordination
        results = await coordinator.wait_for_coordination(
            coordination_id=coordination_id,
            min_results=1,
            timeout=5
        )

        # Should have partial success
        assert len(results["individual_results"]) >= 1
        assert results["performance_summary"]["successful_models"] >= 1
        if len(job_ids) > 1:
            assert len(results["failed_jobs"]) >= 1


class TestPerformanceTracking:
    """Test performance tracking and optimization."""

    async def test_performance_recording(self, mock_session):
        """Test recording model performance metrics."""
        tracker = PerformanceTracker(mock_session)

        await tracker.record_performance(
            model_id="test-model",
            content_type="email",
            processing_time=1500,
            confidence_score=0.85,
            success=True
        )

        # Verify session was called (would execute INSERT in real implementation)
        assert len(mock_session.executed_queries) > 0

    async def test_model_selection_optimization(self, mock_session):
        """Test model selection optimization based on performance."""
        tracker = PerformanceTracker(mock_session)

        available_models = ["llama-3.1-8b", "gpt-4", "claude-3-sonnet"]

        optimized = await tracker.optimize_model_selection(
            content_type="email",
            available_models=available_models
        )

        # Should return optimized list (may reorder based on performance)
        assert len(optimized) <= len(available_models)
        assert all(model in available_models for model in optimized)


# Performance benchmarks
@pytest.mark.performance
class TestCoordinationPerformance:
    """Performance tests for coordination framework."""

    async def test_coordination_latency(self, coordinator, mock_job_manager):
        """Test coordination startup latency."""
        content_data = {"content": "Performance test content"}

        start_time = datetime.now()

        coordination_id = await coordinator.coordinate_processing(
            content_id="perf-test",
            content_type="email",
            content_data=content_data
        )

        end_time = datetime.now()
        latency_ms = (end_time - start_time).total_seconds() * 1000

        # Should start coordination quickly (< 100ms for mocked components)
        assert latency_ms < 100
        assert coordination_id in coordinator.active_coordinations

    async def test_concurrent_coordinations(self, coordinator, mock_job_manager):
        """Test handling multiple concurrent coordination workflows."""
        coordination_ids = []

        # Start multiple coordinations concurrently
        tasks = []
        for i in range(5):
            task = coordinator.coordinate_processing(
                content_id=f"concurrent-{i}",
                content_type="email",
                content_data={"content": f"Content {i}"}
            )
            tasks.append(task)

        results = await asyncio.gather(*tasks)

        # All should succeed
        assert len(results) == 5
        assert all(coord_id in coordinator.active_coordinations for coord_id in results)

        # Should have separate tracking for each
        coordination_ids = list(coordinator.active_coordinations.keys())
        assert len(coordination_ids) == 5