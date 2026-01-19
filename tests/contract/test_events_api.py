"""Contract tests for event processing API endpoints.

This module tests the contract compliance of event processing functionality
to ensure proper event publication, subscription, and job management.
"""

import pytest
import asyncio
from typing import Dict, Any, Optional
from unittest.mock import AsyncMock, MagicMock
import msgpack

from sqlalchemy import select

from penf_lib.storage.events import EventPublisher, EventSubscriber
from penf_lib.storage.jobs import JobManager
from penf_lib.storage.models import ProcessingEvent, ProcessingJob, ProcessingResult, Subscription


@pytest.mark.asyncio
class TestEventPublisherContract:
    """Test event publisher contract compliance."""

    async def test_publish_event_returns_event_id(self, test_session, mock_redis):
        """Publishing an event should return a unique event ID."""
        publisher = EventPublisher(test_session, redis_client=mock_redis)

        event_data = {
            "source_id": "test-source",
            "event_type": "document.processed",
            "payload": {"status": "success"}
        }

        event_id = await publisher.publish("document.processed", event_data)
        assert event_id is not None
        assert isinstance(event_id, str)

    async def test_publish_event_stores_in_database(self, test_session, mock_redis):
        """Published events should be stored in the database."""
        publisher = EventPublisher(test_session, redis_client=mock_redis)

        event_data = {
            "source_id": "test-source",
            "event_type": "document.processed",
            "payload": {"status": "success"}
        }

        event_id = await publisher.publish("document.processed", event_data)

        # Verify event was stored
        result = await test_session.execute(
            select(ProcessingEvent).where(ProcessingEvent.id == event_id)
        )
        event = result.scalar_one()
        assert event.event_type == "document.processed"
        assert event.payload == event_data

    async def test_publish_event_sends_to_redis(self, test_session, mock_redis):
        """Published events should be sent to Redis for real-time processing."""
        publisher = EventPublisher(test_session, redis_client=mock_redis)

        event_data = {"source_id": "test-source"}
        await publisher.publish("document.processed", event_data)

        # Verify Redis publish was called
        mock_redis.publish.assert_called_once()
        call_args = mock_redis.publish.call_args
        assert call_args[0][0] == "document.processed"  # channel
        # Payload should be msgpack serialized
        assert isinstance(call_args[0][1], bytes)


@pytest.mark.asyncio
class TestEventSubscriberContract:
    """Test event subscriber contract compliance."""

    async def test_subscribe_receives_published_events(self, mock_redis):
        """Subscribers should receive events published to their channels."""
        subscriber = EventSubscriber(redis_client=mock_redis)
        received_events = []

        async def event_handler(event_type: str, event_data: Dict[str, Any]):
            received_events.append((event_type, event_data))

        # Subscribe and simulate receiving an event
        await subscriber.subscribe(["document.processed"], event_handler)

        # Simulate Redis message
        mock_message = MagicMock()
        mock_message.channel.decode.return_value = "document.processed"
        mock_message.data = msgpack.packb({"source_id": "test"})

        await subscriber._handle_message(mock_message)

        assert len(received_events) == 1
        assert received_events[0][0] == "document.processed"
        assert received_events[0][1] == {"source_id": "test"}

    async def test_subscribe_handles_errors_gracefully(self, mock_redis):
        """Subscriber should handle processing errors without crashing."""
        subscriber = EventSubscriber(redis_client=mock_redis)

        async def failing_handler(event_type: str, event_data: Dict[str, Any]):
            raise ValueError("Processing failed")

        await subscriber.subscribe(["document.processed"], failing_handler)

        # Simulate Redis message that causes handler to fail
        mock_message = MagicMock()
        mock_message.channel.decode.return_value = "document.processed"
        mock_message.data = msgpack.packb({"source_id": "test"})

        # Should not raise exception
        await subscriber._handle_message(mock_message)

    async def test_subscriber_filters_by_subscription_rules(self, mock_redis):
        """Subscribers should only receive events matching their filter rules."""
        subscriber = EventSubscriber(redis_client=mock_redis)
        received_events = []

        async def selective_handler(event_type: str, event_data: Dict[str, Any]):
            # Only process events with specific source
            if event_data.get("source_id") == "target-source":
                received_events.append((event_type, event_data))

        await subscriber.subscribe(["document.processed"], selective_handler)

        # Simulate messages - one should be filtered out
        messages = [
            {"channel": "document.processed", "data": {"source_id": "target-source"}},
            {"channel": "document.processed", "data": {"source_id": "other-source"}},
        ]

        for msg_data in messages:
            mock_message = MagicMock()
            mock_message.channel.decode.return_value = msg_data["channel"]
            mock_message.data = msgpack.packb(msg_data["data"])
            await subscriber._handle_message(mock_message)

        # Only one event should be received
        assert len(received_events) == 1
        assert received_events[0][1]["source_id"] == "target-source"


@pytest.mark.asyncio
class TestJobManagerContract:
    """Test job management contract compliance."""

    async def test_create_job_returns_job_id(self, test_session):
        """Creating a processing job should return a unique job ID."""
        job_manager = JobManager(test_session)

        job_data = {
            "event_id": "test-event",
            "processor_type": "document_extractor",
            "input_data": {"document_path": "/path/to/doc.pdf"}
        }

        job_id = await job_manager.create_job(**job_data)
        assert job_id is not None
        assert isinstance(job_id, str)

    async def test_job_state_transitions(self, test_session):
        """Job states should transition according to the state machine."""
        job_manager = JobManager(test_session)

        job_id = await job_manager.create_job(
            event_id="test-event",
            processor_type="test_processor",
            input_data={"test": "data"}
        )

        # Initial state should be pending
        job = await job_manager.get_job(job_id)
        assert job.status == "pending"

        # Transition to running
        await job_manager.start_job(job_id)
        job = await job_manager.get_job(job_id)
        assert job.status == "running"

        # Complete the job
        await job_manager.complete_job(job_id, {"result": "success"})
        job = await job_manager.get_job(job_id)
        assert job.status == "completed"

    async def test_job_retry_logic(self, test_session):
        """Failed jobs should be retryable with proper retry counting."""
        job_manager = JobManager(test_session)

        job_id = await job_manager.create_job(
            event_id="test-event",
            processor_type="flaky_processor",
            input_data={"test": "data"}
        )

        # Fail the job
        await job_manager.fail_job(job_id, "Processing error")
        job = await job_manager.get_job(job_id)
        assert job.status == "failed"
        assert job.retry_count == 0

        # Retry the job
        new_job_id = await job_manager.retry_job(job_id)
        new_job = await job_manager.get_job(new_job_id)
        assert new_job.retry_count == 1
        assert new_job.status == "pending"

    async def test_job_result_storage(self, test_session):
        """Job results should be properly stored and retrievable."""
        job_manager = JobManager(test_session)

        job_id = await job_manager.create_job(
            event_id="test-event",
            processor_type="test_processor",
            input_data={"test": "data"}
        )

        result_data = {
            "extracted_entities": ["person1", "person2"],
            "confidence": 0.95,
            "processing_time": 1.25
        }

        await job_manager.complete_job(job_id, result_data)

        # Verify result was stored
        results = await job_manager.get_job_results(job_id)
        assert len(results) == 1
        assert results[0].result_data == result_data


@pytest.mark.asyncio
class TestSubscriptionContract:
    """Test subscription management contract compliance."""

    async def test_subscription_filtering(self, test_session):
        """Subscriptions should filter events based on their filter criteria."""
        # This will be implemented when Subscription model is complete
        pytest.skip("Subscription model not yet implemented")

    async def test_subscription_lifecycle(self, test_session):
        """Subscriptions should be creatable, updatable, and deletable."""
        # This will be implemented when Subscription model is complete
        pytest.skip("Subscription model not yet implemented")


# Test fixtures for mocking Redis
@pytest.fixture
def mock_redis():
    """Mock Redis client for testing."""
    mock = AsyncMock()
    mock.publish = AsyncMock()
    mock.subscribe = AsyncMock()
    return mock


@pytest.fixture
async def event_publisher(test_session, mock_redis):
    """Event publisher instance for testing."""
    return EventPublisher(test_session, redis_client=mock_redis)


@pytest.fixture
async def event_subscriber(mock_redis):
    """Event subscriber instance for testing."""
    return EventSubscriber(redis_client=mock_redis)


@pytest.fixture
async def job_manager(test_session):
    """Job manager instance for testing."""
    return JobManager(test_session)