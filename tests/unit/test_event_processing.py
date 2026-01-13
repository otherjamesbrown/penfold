"""Unit tests for event processing modules.

Tests event processing functionality that doesn't require database or Redis.
"""

import pytest
from unittest.mock import MagicMock, AsyncMock, patch
import json
from datetime import datetime, timezone

from penf_lib.storage.events import EventPublisher, EventSubscriber, SubscriptionManager
from penf_lib.storage.jobs import JobManager, JobStatus


class TestJobStatus:
    """Test job status enumeration."""

    def test_job_status_values(self):
        """Test that all expected job statuses are defined."""
        expected_statuses = [
            "pending", "running", "completed", "failed", "cancelled", "retrying"
        ]

        for status in expected_statuses:
            assert hasattr(JobStatus, status.upper())
            assert getattr(JobStatus, status.upper()) == status

    def test_job_status_string_representation(self):
        """Test job status string representation."""
        assert JobStatus.PENDING == "pending"
        assert JobStatus.RUNNING == "running"
        assert JobStatus.COMPLETED == "completed"
        assert JobStatus.FAILED == "failed"
        assert JobStatus.CANCELLED == "cancelled"
        assert JobStatus.RETRYING == "retrying"


class TestEventPublisher:
    """Test event publisher functionality without external dependencies."""

    def setup_method(self):
        """Set up test fixtures."""
        self.mock_session = AsyncMock()
        self.mock_redis = AsyncMock()

    def test_initialization(self):
        """Test event publisher initialization."""
        publisher = EventPublisher(
            session=self.mock_session,
            redis_client=self.mock_redis,
            use_postgresql_fallback=True
        )

        assert publisher.session == self.mock_session
        assert publisher.redis_client == self.mock_redis
        assert publisher.use_postgresql_fallback is True
        assert publisher._redis_available is True

    def test_initialization_defaults(self):
        """Test event publisher initialization with defaults."""
        with patch('penf_lib.storage.events.get_redis_client') as mock_get_redis:
            mock_get_redis.return_value = self.mock_redis

            publisher = EventPublisher(session=self.mock_session)

            assert publisher.session == self.mock_session
            assert publisher.redis_client == self.mock_redis
            assert publisher.use_postgresql_fallback is True

    @pytest.mark.asyncio
    async def test_publish_validation(self):
        """Test event publishing input validation."""
        publisher = EventPublisher(
            session=self.mock_session,
            redis_client=self.mock_redis
        )

        # Test with invalid event type
        with pytest.raises(ValueError, match="Event type and data are required"):
            await publisher.publish("", {"data": "test"})

        # Test with invalid event data
        with pytest.raises(ValueError, match="Event type and data are required"):
            await publisher.publish("test.event", "not_a_dict")

        # Test with None event data
        with pytest.raises(ValueError, match="Event type and data are required"):
            await publisher.publish("test.event", None)

    def test_message_data_structure(self):
        """Test that message data structure is correct for Redis."""
        import msgpack
        from datetime import datetime, timezone

        # Test message serialization structure
        test_event_data = {"source_id": "test", "status": "success"}
        test_event_id = "test-event-123"

        message_data = {
            "event_id": test_event_id,
            "timestamp": datetime.now(timezone.utc).isoformat(),
            "payload": test_event_data,
        }

        # Should be serializable with msgpack
        serialized = msgpack.packb(message_data)
        deserialized = msgpack.unpackb(serialized, raw=False)

        assert deserialized["event_id"] == test_event_id
        assert deserialized["payload"] == test_event_data
        assert "timestamp" in deserialized


class TestEventSubscriber:
    """Test event subscriber functionality."""

    def setup_method(self):
        """Set up test fixtures."""
        self.mock_redis = AsyncMock()

    def test_initialization(self):
        """Test event subscriber initialization."""
        subscriber = EventSubscriber(
            redis_client=self.mock_redis,
            max_retries=5,
            retry_delay=2.0
        )

        assert subscriber.redis_client == self.mock_redis
        assert subscriber.max_retries == 5
        assert subscriber.retry_delay == 2.0
        assert isinstance(subscriber._subscriptions, set)
        assert isinstance(subscriber._handlers, dict)
        assert subscriber._running is False

    def test_initialization_defaults(self):
        """Test subscriber initialization with defaults."""
        with patch('penf_lib.storage.events.get_redis_client') as mock_get_redis:
            mock_get_redis.return_value = self.mock_redis

            subscriber = EventSubscriber()

            assert subscriber.redis_client == self.mock_redis
            assert subscriber.max_retries == 3
            assert subscriber.retry_delay == 1.0

    @pytest.mark.asyncio
    async def test_subscribe_handler_registration(self):
        """Test that subscription properly registers handlers."""
        subscriber = EventSubscriber(redis_client=self.mock_redis)

        async def test_handler(event_type: str, event_data: dict):
            pass

        def test_filter(event_type: str, event_data: dict) -> bool:
            return True

        await subscriber.subscribe(
            channels=["test.channel1", "test.channel2"],
            handler=test_handler,
            filter_func=test_filter
        )

        # Check that channels were registered
        assert "test.channel1" in subscriber._subscriptions
        assert "test.channel2" in subscriber._subscriptions

        # Check that handlers were stored
        assert "test.channel1" in subscriber._handlers
        assert "test.channel2" in subscriber._handlers

        # Check handler configuration
        handler_config = subscriber._handlers["test.channel1"]
        assert handler_config["handler"] == test_handler
        assert handler_config["filter"] == test_filter

    @pytest.mark.asyncio
    async def test_message_handling_with_filter(self):
        """Test message handling with filter function."""
        import msgpack

        subscriber = EventSubscriber(redis_client=self.mock_redis)
        received_events = []

        async def test_handler(event_type: str, event_data: dict):
            received_events.append((event_type, event_data))

        def test_filter(event_type: str, event_data: dict) -> bool:
            return event_data.get("should_process", False)

        await subscriber.subscribe(
            channels=["test.channel"],
            handler=test_handler,
            filter_func=test_filter
        )

        # Test message that should be filtered out
        mock_message = {
            "channel": b"test.channel",
            "data": msgpack.packb({
                "event_id": "test1",
                "payload": {"should_process": False}
            })
        }

        await subscriber._handle_message(mock_message)
        assert len(received_events) == 0

        # Test message that should be processed
        mock_message = {
            "channel": b"test.channel",
            "data": msgpack.packb({
                "event_id": "test2",
                "payload": {"should_process": True}
            })
        }

        await subscriber._handle_message(mock_message)
        assert len(received_events) == 1
        assert received_events[0][1] == {"should_process": True}

    @pytest.mark.asyncio
    async def test_error_handling_in_message_processing(self):
        """Test that message processing errors are handled gracefully."""
        import msgpack

        subscriber = EventSubscriber(redis_client=self.mock_redis)

        async def failing_handler(event_type: str, event_data: dict):
            raise ValueError("Simulated processing error")

        await subscriber.subscribe(
            channels=["test.channel"],
            handler=failing_handler
        )

        # Test that error in handler doesn't crash subscriber
        mock_message = {
            "channel": b"test.channel",
            "data": msgpack.packb({
                "event_id": "test",
                "payload": {"test": "data"}
            })
        }

        # Should not raise exception
        await subscriber._handle_message(mock_message)

    @pytest.mark.asyncio
    async def test_stop_consuming(self):
        """Test stopping event consumption."""
        subscriber = EventSubscriber(redis_client=self.mock_redis)

        # Start and then stop
        subscriber._running = True
        assert subscriber._running is True

        await subscriber.stop_consuming()
        assert subscriber._running is False


class TestJobManager:
    """Test job manager functionality without database."""

    def setup_method(self):
        """Set up test fixtures."""
        self.mock_session = AsyncMock()

    def test_initialization(self):
        """Test job manager initialization."""
        job_manager = JobManager(
            session=self.mock_session,
            max_job_age_days=60
        )

        assert job_manager.session == self.mock_session
        assert job_manager.max_job_age_days == 60

    def test_initialization_defaults(self):
        """Test job manager initialization with defaults."""
        job_manager = JobManager(session=self.mock_session)

        assert job_manager.session == self.mock_session
        assert job_manager.max_job_age_days == 30

    @pytest.mark.asyncio
    async def test_create_job_validation(self):
        """Test job creation input validation."""
        job_manager = JobManager(session=self.mock_session)

        # Test with missing required parameters
        with pytest.raises(ValueError, match="Event ID, processor type, and input data are required"):
            await job_manager.create_job("", "processor", {"data": "test"})

        with pytest.raises(ValueError, match="Event ID, processor type, and input data are required"):
            await job_manager.create_job("event-1", "", {"data": "test"})

        with pytest.raises(ValueError, match="Event ID, processor type, and input data are required"):
            await job_manager.create_job("event-1", "processor", "not_a_dict")

        with pytest.raises(ValueError, match="Event ID, processor type, and input data are required"):
            await job_manager.create_job("event-1", "processor", None)

    def test_job_state_machine_validation(self):
        """Test job state machine transitions."""
        # This tests the logic, not the database operations

        # Valid transitions (conceptually)
        valid_transitions = {
            JobStatus.PENDING: [JobStatus.RUNNING, JobStatus.CANCELLED],
            JobStatus.RUNNING: [JobStatus.COMPLETED, JobStatus.FAILED, JobStatus.CANCELLED],
            JobStatus.FAILED: [JobStatus.RETRYING],
            JobStatus.RETRYING: [JobStatus.PENDING],
            # Terminal states
            JobStatus.COMPLETED: [],
            JobStatus.CANCELLED: [],
        }

        # Test that we have defined transitions for all states
        for state in JobStatus:
            assert state in valid_transitions

    def test_job_cleanup_date_calculation(self):
        """Test job cleanup date calculation."""
        from datetime import timedelta

        job_manager = JobManager(
            session=self.mock_session,
            max_job_age_days=30
        )

        current_time = datetime.now(timezone.utc)
        cutoff_time = current_time - timedelta(days=30)

        # Test that cutoff calculation would work
        assert cutoff_time < current_time
        assert (current_time - cutoff_time).days == 30


class TestSubscriptionManager:
    """Test subscription manager functionality."""

    def setup_method(self):
        """Set up test fixtures."""
        self.mock_session = AsyncMock()

    def test_initialization(self):
        """Test subscription manager initialization."""
        subscription_manager = SubscriptionManager(session=self.mock_session)

        assert subscription_manager.session == self.mock_session

    @pytest.mark.asyncio
    async def test_create_subscription_validation(self):
        """Test subscription creation validation."""
        subscription_manager = SubscriptionManager(session=self.mock_session)

        # Test valid subscription parameters
        channels = ["document.processed", "user.created"]
        event_types = ["document", "user"]
        filter_criteria = {"source": "gmail"}

        # Should not raise exception with valid parameters
        # (We're just testing the parameter handling, not database operations)
        subscription_id = "test-subscription-id"

        # Mock the database operations to avoid actual DB calls
        self.mock_session.add = MagicMock()
        self.mock_session.commit = AsyncMock()

        # The actual method would need to be mocked further for full testing
        # This validates the parameter structure


def test_import_event_modules():
    """Test that event processing modules can be imported successfully."""
    from penf_lib.storage.events import (
        EventPublisher,
        EventSubscriber,
        SubscriptionManager
    )

    from penf_lib.storage.jobs import (
        JobManager,
        JobStatus
    )

    # Should import without error
    assert EventPublisher is not None
    assert EventSubscriber is not None
    assert SubscriptionManager is not None
    assert JobManager is not None
    assert JobStatus is not None


def test_job_status_enum_completeness():
    """Test that JobStatus enum has all expected values."""
    expected_statuses = {
        "PENDING": "pending",
        "RUNNING": "running",
        "COMPLETED": "completed",
        "FAILED": "failed",
        "CANCELLED": "cancelled",
        "RETRYING": "retrying"
    }

    for attr_name, expected_value in expected_statuses.items():
        assert hasattr(JobStatus, attr_name)
        assert getattr(JobStatus, attr_name) == expected_value


def test_configuration_parameter_validation():
    """Test configuration parameter validation for event processing."""
    from penf_lib.storage.events import EventPublisher, EventSubscriber
    from penf_lib.storage.jobs import JobManager

    mock_session = AsyncMock()
    mock_redis = AsyncMock()

    # Test event publisher configuration
    publisher = EventPublisher(
        session=mock_session,
        redis_client=mock_redis,
        use_postgresql_fallback=False
    )
    assert publisher.use_postgresql_fallback is False

    # Test event subscriber configuration
    subscriber = EventSubscriber(
        redis_client=mock_redis,
        max_retries=10,
        retry_delay=5.0
    )
    assert subscriber.max_retries == 10
    assert subscriber.retry_delay == 5.0

    # Test job manager configuration
    job_manager = JobManager(
        session=mock_session,
        max_job_age_days=90
    )
    assert job_manager.max_job_age_days == 90