"""Integration tests for Event Processing Framework User Stories.

These tests validate the complete user stories from specs/002-event-processing
without requiring external infrastructure (Redis/PostgreSQL).
"""

import pytest
import asyncio
import uuid
from datetime import datetime, timezone
from typing import Dict, Any, List
from unittest.mock import AsyncMock, MagicMock, patch

from penf_lib.storage.events import EventPublisher, EventSubscriber, SubscriptionManager
from penf_lib.storage.jobs import JobManager, JobStatus
from penf_lib.storage.models import ProcessingEvent, ProcessingJob, ProcessingResult, Subscription


class TestUserStory1ContentIngestionEventPublishing:
    """User Story 1: Content Ingestion Event Publishing (Priority: P1)

    As the ingestion system, I need to publish events when new content arrives
    so that multiple AI processors can work on the same content simultaneously.
    """

    @pytest.fixture
    def mock_session(self):
        """Mock database session."""
        session = AsyncMock()
        session.commit = AsyncMock()
        session.rollback = AsyncMock()
        session.add = MagicMock()
        return session

    @pytest.fixture
    def mock_redis(self):
        """Mock Redis client."""
        redis_client = AsyncMock()
        redis_client.publish = AsyncMock(return_value=1)
        return redis_client

    @pytest.fixture
    def event_publisher(self, mock_session, mock_redis):
        """Event publisher with mocked dependencies."""
        return EventPublisher(session=mock_session, redis_client=mock_redis)

    async def test_email_content_ingested_event_published(self, event_publisher, mock_session, mock_redis):
        """
        Acceptance Scenario 1:
        Given: new email content is ingested
        When: ingestion completes
        Then: content.ingested event is published with email metadata and content payload
        """
        # Arrange - new email content
        email_metadata = {
            "from": "sender@example.com",
            "to": ["recipient@example.com"],
            "subject": "Project Update",
            "timestamp": "2024-01-13T10:00:00Z",
            "message_id": "abc123"
        }
        email_content = {
            "body": "Here's an update on the project status...",
            "attachments": ["report.pdf"],
            "content_hash": "sha256:abc123def456"
        }

        # Act - ingestion completes, publish event
        event_id = await event_publisher.publish(
            event_type="content.ingested",
            event_data={
                "content_type": "email",
                "metadata": email_metadata,
                "content": email_content,
                "ingestion_timestamp": datetime.now(timezone.utc).isoformat(),
                "source_id": "gmail_connector"
            },
            channel="email.processing"
        )

        # Assert - event published correctly
        assert event_id is not None
        mock_session.add.assert_called_once()
        mock_session.commit.assert_called_once()

        # Verify Redis publishing
        mock_redis.publish.assert_called_once()
        redis_args = mock_redis.publish.call_args[0]
        assert redis_args[0] == "email.processing"  # Channel
        assert "content.ingested" in str(redis_args[1])  # Event type in payload

    async def test_meeting_recording_preprocessed_event_published(self, event_publisher, mock_session, mock_redis):
        """
        Acceptance Scenario 2:
        Given: meeting recording is uploaded
        When: preprocessing finishes
        Then: meeting.preprocessed event is published with file references and context data
        """
        # Arrange - meeting recording preprocessing
        meeting_metadata = {
            "meeting_id": "meeting_123",
            "title": "Sprint Planning",
            "participants": ["alice@company.com", "bob@company.com"],
            "start_time": "2024-01-13T14:00:00Z",
            "duration_minutes": 60
        }
        preprocessing_data = {
            "audio_file": "/uploads/meeting_123.mp3",
            "transcript_file": "/processed/meeting_123_transcript.txt",
            "video_file": "/uploads/meeting_123.mp4",
            "processing_time_seconds": 120
        }

        # Act - preprocessing completes, publish event
        event_id = await event_publisher.publish(
            event_type="meeting.preprocessed",
            event_data={
                "meeting_metadata": meeting_metadata,
                "file_references": preprocessing_data,
                "preprocessing_timestamp": datetime.now(timezone.utc).isoformat(),
                "ready_for_analysis": True,
                "source_id": "meeting_upload_processor"
            },
            channel="meeting.processing"
        )

        # Assert - event published correctly
        assert event_id is not None
        mock_session.add.assert_called_once()
        mock_redis.publish.assert_called_once()

    async def test_manual_document_categorized_event_published(self, event_publisher, mock_session, mock_redis):
        """
        Acceptance Scenario 3:
        Given: manual document is added
        When: user confirms categorization
        Then: content.categorized event is published with user-provided project assignments
        """
        # Arrange - manual document categorization
        document_data = {
            "document_id": "doc_456",
            "title": "Architecture Decision Record",
            "file_path": "/documents/adr_001.md",
            "content_type": "markdown",
            "user_id": "user_789"
        }
        categorization_data = {
            "project_assignments": ["project_atlas", "project_infrastructure"],
            "document_type": "architecture_decision",
            "confidence_score": 1.0,  # Manual categorization has high confidence
            "tags": ["architecture", "decision", "infrastructure"]
        }

        # Act - user confirms categorization, publish event
        event_id = await event_publisher.publish(
            event_type="content.categorized",
            event_data={
                "document": document_data,
                "categorization": categorization_data,
                "categorization_method": "manual",
                "categorized_timestamp": datetime.now(timezone.utc).isoformat(),
                "source_id": "manual_document_processor"
            },
            channel="document.processing"
        )

        # Assert - event published correctly
        assert event_id is not None
        mock_session.add.assert_called_once()
        mock_redis.publish.assert_called_once()


class TestUserStory2AIProcessorSubscriptionAndJobManagement:
    """User Story 2: AI Processor Subscription and Job Management (Priority: P1)

    As an AI processing component, I need to subscribe to relevant events and
    manage my processing jobs so that I can process content independently.
    """

    @pytest.fixture
    def mock_session(self):
        session = AsyncMock()
        session.commit = AsyncMock()
        session.rollback = AsyncMock()
        session.add = MagicMock()
        session.execute = AsyncMock()
        return session

    @pytest.fixture
    def subscription_manager(self, mock_session):
        return SubscriptionManager(session=mock_session)

    @pytest.fixture
    def job_manager(self, mock_session):
        return JobManager(session=mock_session)

    async def test_processor_subscription_creates_job_on_event(self, subscription_manager, job_manager, mock_session):
        """
        Acceptance Scenario 1:
        Given: AI processor subscribes to content.ingested events
        When: content.ingested event is published
        Then: processing job is created with queued state
        """
        # Arrange - AI processor subscription
        processor_id = "entity_extraction_processor"
        subscription_id = await subscription_manager.create_subscription(
            subscriber_id=processor_id,
            subscription_name="Entity Extraction",
            event_channels=["email.processing", "document.processing"],
            event_types=["content.ingested", "content.categorized"],
            filter_criteria={"content_type": {"$in": ["email", "document"]}}
        )

        # Simulate content.ingested event
        event_data = {
            "event_type": "content.ingested",
            "content_type": "email",
            "content_id": "email_123"
        }

        # Act - create processing job when event matches subscription
        job_id = await job_manager.create_job(
            event_id="event_456",
            processor_name=processor_id,
            job_type="entity_extraction",
            input_data=event_data,
            priority=2
        )

        # Assert - job created in queued (pending) state
        assert job_id is not None
        assert subscription_id is not None
        mock_session.add.assert_called()
        mock_session.commit.assert_called()

    async def test_job_state_transition_to_in_progress(self, job_manager, mock_session):
        """
        Acceptance Scenario 2:
        Given: processing job is claimed by processor
        When: processor starts work
        Then: job state transitions to in_progress
        """
        # Arrange - mock existing job
        job_id = "job_123"
        mock_job = MagicMock()
        mock_job.job_id = job_id
        mock_job.status = "queued"
        mock_job.retry_count = 0

        # Mock job retrieval
        mock_result = AsyncMock()
        mock_result.scalar_one_or_none.return_value = mock_job
        mock_session.execute.return_value = mock_result

        # Act - processor starts work on job
        success = await job_manager.start_job(job_id, worker_id="worker_001")

        # Assert - job state transitions to running (in_progress)
        assert success == True
        assert mock_job.status == "in_progress"
        assert mock_job.started_at is not None
        mock_session.commit.assert_called()

    async def test_job_completion_with_result_storage(self, job_manager, mock_session):
        """
        Acceptance Scenario 3:
        Given: processing completes successfully
        When: processor reports results
        Then: job state transitions to completed with result data stored
        """
        # Arrange - mock running job
        job_id = "job_456"
        mock_job = MagicMock()
        mock_job.id = job_id
        mock_job.status = JobStatus.RUNNING

        mock_result = AsyncMock()
        mock_result.scalar_one_or_none.return_value = mock_job
        mock_session.execute.return_value = mock_result

        # Processing results
        processing_results = {
            "entities_extracted": ["John Doe", "Project Atlas"],
            "confidence_score": 0.85,
            "processing_time_ms": 1500,
            "model_version": "llama-3.1-8b"
        }

        # Act - complete job with results
        success = await job_manager.complete_job(
            job_id,
            result_data=processing_results,
            confidence_score=0.85
        )

        # Assert - job completed with results stored
        assert success == True
        assert mock_job.status == JobStatus.COMPLETED
        assert mock_job.completed_at is not None
        mock_session.add.assert_called()  # For ProcessingResult
        mock_session.commit.assert_called()


class TestUserStory3MultiModelResultAggregation:
    """User Story 3: Multi-Model Result Aggregation (Priority: P2)

    As the system coordinator, I need to collect and compare results from
    multiple AI processors working on the same content.
    """

    @pytest.fixture
    def mock_session(self):
        session = AsyncMock()
        session.commit = AsyncMock()
        session.execute = AsyncMock()
        return session

    @pytest.fixture
    def job_manager(self, mock_session):
        return JobManager(session=mock_session)

    async def test_multiple_processor_results_aggregated(self, job_manager, mock_session):
        """
        Acceptance Scenario 1:
        Given: multiple processors complete work on same content
        When: all results are available
        Then: aggregated result is created with confidence scores from each processor
        """
        # Arrange - multiple processors working on same content
        content_id = "email_789"

        # Local processor results
        local_results = {
            "processor_type": "local_entity_extractor",
            "entities": ["John Smith", "Budget Review"],
            "confidence": 0.75,
            "model": "llama-3.1-8b"
        }

        # Cloud processor results
        cloud_results = {
            "processor_type": "cloud_entity_extractor",
            "entities": ["John Smith", "Q4 Budget Review", "Finance Team"],
            "confidence": 0.92,
            "model": "gpt-4"
        }

        # Mock multiple result retrieval
        mock_results = [
            MagicMock(processor_type="local_entity_extractor", result_data=local_results, confidence_score=0.75),
            MagicMock(processor_type="cloud_entity_extractor", result_data=cloud_results, confidence_score=0.92)
        ]

        mock_query_result = AsyncMock()
        mock_query_result.scalars.return_value.all.return_value = mock_results
        mock_session.execute.return_value = mock_query_result

        # Act - aggregate results from multiple processors
        aggregated_result = await job_manager.aggregate_results(content_id)

        # Assert - aggregated result contains all processor outputs
        assert aggregated_result is not None
        assert "processors" in aggregated_result
        assert len(aggregated_result["processors"]) == 2
        assert aggregated_result["consensus_confidence"] > 0.75
        mock_session.execute.assert_called()

    async def test_local_vs_cloud_processor_comparison(self, job_manager, mock_session):
        """
        Acceptance Scenario 2:
        Given: local and cloud processors provide different results
        When: comparison is requested
        Then: differences are highlighted with confidence analysis
        """
        # Arrange - conflicting results
        local_result = {
            "entities": ["Project Alpha"],
            "sentiment": "positive",
            "confidence": 0.70
        }

        cloud_result = {
            "entities": ["Project Alpha", "Risk Assessment"],
            "sentiment": "neutral",
            "confidence": 0.90
        }

        # Mock comparison method
        comparison = await job_manager.compare_processor_results(
            local_result, cloud_result
        )

        # Assert - differences identified
        assert comparison["entity_differences"] == ["Risk Assessment"]
        assert comparison["sentiment_conflict"] == True
        assert comparison["confidence_gap"] == 0.20
        assert comparison["recommended_result"] == "cloud"  # Higher confidence

    async def test_best_result_selection_with_reasoning(self, job_manager, mock_session):
        """
        Acceptance Scenario 3:
        Given: result aggregation completes
        When: best result is selected
        Then: selection criteria and reasoning are recorded for learning
        """
        # Arrange - multiple results with different characteristics
        results = [
            {"processor": "fast_local", "confidence": 0.65, "processing_time": 500},
            {"processor": "accurate_cloud", "confidence": 0.95, "processing_time": 3000},
            {"processor": "balanced_local", "confidence": 0.80, "processing_time": 1200}
        ]

        # Act - select best result with selection criteria
        selection = await job_manager.select_best_result(
            results,
            criteria={"confidence_weight": 0.7, "speed_weight": 0.3}
        )

        # Assert - selection reasoning recorded
        assert selection["selected_processor"] == "balanced_local"  # Good balance
        assert selection["reasoning"]["confidence_threshold_met"] == True
        assert selection["reasoning"]["speed_bonus_applied"] == True
        assert "selection_criteria" in selection
        assert "alternatives_considered" in selection


class TestUserStory4ProcessingMonitoringAndHealthManagement:
    """User Story 4: Processing Monitoring and Health Management (Priority: P2)

    As a system administrator, I need to monitor processing jobs and system health.
    """

    @pytest.fixture
    def mock_session(self):
        session = AsyncMock()
        session.execute = AsyncMock()
        return session

    @pytest.fixture
    def job_manager(self, mock_session):
        return JobManager(session=mock_session)

    async def test_job_status_and_processor_health_reporting(self, job_manager, mock_session):
        """
        Acceptance Scenario 1:
        Given: processing jobs are running
        When: status query is requested
        Then: current job states and processor health are reported
        """
        # Arrange - mock job status data
        mock_jobs = [
            MagicMock(status=JobStatus.RUNNING, processor_type="entity_extractor", started_at=datetime.now(timezone.utc)),
            MagicMock(status=JobStatus.PENDING, processor_type="sentiment_analyzer", created_at=datetime.now(timezone.utc)),
            MagicMock(status=JobStatus.COMPLETED, processor_type="entity_extractor", completed_at=datetime.now(timezone.utc))
        ]

        mock_result = AsyncMock()
        mock_result.scalars.return_value.all.return_value = mock_jobs
        mock_session.execute.return_value = mock_result

        # Act - request system health status
        health_report = await job_manager.get_system_health()

        # Assert - comprehensive health report
        assert health_report["job_status"]["running"] == 1
        assert health_report["job_status"]["pending"] == 1
        assert health_report["job_status"]["completed"] == 1
        assert "processor_health" in health_report
        assert "entity_extractor" in health_report["processor_health"]

    async def test_failed_processor_detection_and_job_retry(self, job_manager, mock_session):
        """
        Acceptance Scenario 2:
        Given: processor fails or becomes unresponsive
        When: timeout occurs
        Then: failed jobs are identified and made available for retry
        """
        # Arrange - mock timed-out job
        timed_out_job = MagicMock()
        timed_out_job.id = "job_timeout_123"
        timed_out_job.status = JobStatus.RUNNING
        timed_out_job.started_at = datetime.now(timezone.utc)
        timed_out_job.timeout_seconds = 300
        timed_out_job.retry_count = 0
        timed_out_job.max_retries = 3

        mock_result = AsyncMock()
        mock_result.scalars.return_value.all.return_value = [timed_out_job]
        mock_session.execute.return_value = mock_result

        # Act - detect and handle failed jobs
        failed_jobs = await job_manager.handle_failed_jobs()

        # Assert - failed job identified and prepared for retry
        assert len(failed_jobs) == 1
        assert failed_jobs[0]["job_id"] == "job_timeout_123"
        assert failed_jobs[0]["failure_reason"] == "timeout"
        assert timed_out_job.status == JobStatus.RETRYING

    async def test_processing_queue_bottleneck_analysis(self, job_manager, mock_session):
        """
        Acceptance Scenario 3:
        Given: processing queue builds up
        When: load analysis is performed
        Then: bottlenecks and scaling recommendations are provided
        """
        # Arrange - mock queue buildup
        pending_jobs = [MagicMock(processor_type="entity_extractor") for _ in range(50)]
        running_jobs = [MagicMock(processor_type="entity_extractor") for _ in range(5)]

        mock_pending_result = AsyncMock()
        mock_pending_result.scalars.return_value.all.return_value = pending_jobs
        mock_running_result = AsyncMock()
        mock_running_result.scalars.return_value.all.return_value = running_jobs

        mock_session.execute.side_effect = [mock_pending_result, mock_running_result]

        # Act - analyze processing load
        bottleneck_analysis = await job_manager.analyze_processing_bottlenecks()

        # Assert - bottleneck identified with scaling recommendations
        assert bottleneck_analysis["bottleneck_detected"] == True
        assert bottleneck_analysis["bottleneck_processor"] == "entity_extractor"
        assert bottleneck_analysis["queue_depth"] == 50
        assert bottleneck_analysis["active_workers"] == 5
        assert "scaling_recommendation" in bottleneck_analysis
        assert bottleneck_analysis["scaling_recommendation"]["action"] == "scale_up"


class TestUserStory5CloudProcessingEscalation:
    """User Story 5: Cloud Processing Escalation (Priority: P3)

    As the processing system, I need to escalate low-confidence local results
    to cloud processors for improved quality.
    """

    @pytest.fixture
    def mock_session(self):
        session = AsyncMock()
        session.commit = AsyncMock()
        session.add = MagicMock()
        return session

    @pytest.fixture
    def job_manager(self, mock_session):
        return JobManager(session=mock_session)

    async def test_low_confidence_result_escalation(self, job_manager, mock_session):
        """
        Acceptance Scenario 1:
        Given: local processor produces low-confidence result
        When: escalation threshold is reached
        Then: cloud processing job is created with context from local result
        """
        # Arrange - low confidence local result
        local_result = {
            "processor_type": "local_entity_extractor",
            "entities": ["John", "project"],  # Vague extraction
            "confidence": 0.45,  # Below escalation threshold of 0.8
            "ambiguity_flags": ["unclear_entity_boundaries", "missing_context"]
        }

        escalation_threshold = 0.8

        # Act - check if escalation is needed
        escalation_needed = local_result["confidence"] < escalation_threshold

        if escalation_needed:
            cloud_job_id = await job_manager.create_job(
                event_id="original_event_123",
                processor_type="cloud_entity_extractor",
                input_data={
                    "escalation": True,
                    "local_result": local_result,
                    "escalation_reason": "low_confidence"
                },
                priority=1  # High priority for escalated jobs
            )

        # Assert - cloud escalation job created
        assert escalation_needed == True
        assert cloud_job_id is not None
        mock_session.add.assert_called()

    async def test_cloud_escalation_quality_improvement_tracking(self, job_manager, mock_session):
        """
        Acceptance Scenario 2:
        Given: cloud processor completes escalation
        When: results are compared
        Then: quality improvement metrics are recorded
        """
        # Arrange - escalation results comparison
        local_result = {"confidence": 0.45, "entities": ["John", "project"]}
        cloud_result = {"confidence": 0.92, "entities": ["John Doe", "Project Atlas Migration"]}

        # Act - calculate quality improvement
        quality_improvement = {
            "confidence_improvement": cloud_result["confidence"] - local_result["confidence"],
            "entity_detail_improvement": len(cloud_result["entities"][0].split()) - len(local_result["entities"][0].split()),
            "escalation_value": "high",  # Significant improvement
            "cost_justified": True
        }

        # Track escalation metrics
        escalation_metrics = await job_manager.record_escalation_metrics(
            local_result, cloud_result, quality_improvement
        )

        # Assert - quality improvement tracked
        assert quality_improvement["confidence_improvement"] > 0.4
        assert quality_improvement["escalation_value"] == "high"
        assert escalation_metrics["total_escalations"] >= 1

    async def test_budget_controlled_escalation_throttling(self, job_manager, mock_session):
        """
        Acceptance Scenario 3:
        Given: cloud processing costs are tracked
        When: budget limits approach
        Then: escalation rules are adjusted to stay within budget
        """
        # Arrange - budget tracking
        current_budget = {"daily_limit": 100.00, "current_spend": 85.00}
        escalation_cost = 20.00  # Cost per cloud API call

        # Act - check budget before escalation
        budget_check = await job_manager.check_escalation_budget(escalation_cost)

        # Budget exceeded, adjust escalation rules
        if not budget_check["within_budget"]:
            adjusted_rules = {
                "escalation_threshold": 0.5,  # Raise threshold (less escalation)
                "priority_escalation_only": True,  # Only high-priority content
                "daily_escalation_limit": 3  # Limit number of escalations
            }

            await job_manager.update_escalation_rules(adjusted_rules)

        # Assert - budget controls working
        assert budget_check["within_budget"] == False
        assert budget_check["budget_warning"] == True
        assert adjusted_rules["escalation_threshold"] > 0.45  # Stricter threshold