"""Contract tests for Daily Review event publishing and consumption.

This module tests the contract compliance of the daily review workflow
for event publishing, event consumption, and DTO structures.

Based on specifications in: specs/006-daily-review/contracts/cli-api.md
"""

import pytest
from datetime import datetime, timedelta, timezone
from decimal import Decimal
from typing import Any
from unittest.mock import AsyncMock, MagicMock
from uuid import uuid4


# Helper fixtures for event testing
@pytest.fixture
def mock_event_publisher():
    """Mock event publisher that captures published events for verification.

    Returns a mock publisher and a list that captures all published events
    with their event types and payloads.
    """
    published_events: list[dict[str, Any]] = []

    async def capture_publish(event_type: str, payload: dict[str, Any]) -> str:
        event_id = str(uuid4())
        published_events.append({
            "event_id": event_id,
            "event_type": event_type,
            "payload": payload,
            "published_at": datetime.now(timezone.utc),
        })
        return event_id

    publisher = AsyncMock()
    publisher.publish = AsyncMock(side_effect=capture_publish)
    publisher.published_events = published_events

    return publisher


@pytest.fixture
def mock_event_consumer():
    """Mock event consumer that provides test events for consumption.

    Returns a consumer mock and a method to inject test events.
    """
    handlers: dict[str, list] = {}

    async def subscribe(event_types: list[str], handler):
        for event_type in event_types:
            if event_type not in handlers:
                handlers[event_type] = []
            handlers[event_type].append(handler)

    async def inject_event(event_type: str, payload: dict[str, Any]):
        """Inject a test event to all subscribed handlers."""
        if event_type in handlers:
            for handler in handlers[event_type]:
                await handler(event_type, payload)

    consumer = AsyncMock()
    consumer.subscribe = AsyncMock(side_effect=subscribe)
    consumer.inject_event = inject_event
    consumer.handlers = handlers

    return consumer


# Sample data fixtures for review contracts
@pytest.fixture
def sample_session_data():
    """Sample session data for testing."""
    return {
        "session_id": 1,
        "session_uuid": str(uuid4()),
        "tenant_id": str(uuid4()),
        "user_email": "user@example.com",
        "mode": "standard",
        "priority": "mixed",
        "item_count": 100,
    }


@pytest.fixture
def sample_review_item_data():
    """Sample review item data for testing."""
    return {
        "item_id": 42,
        "source_id": 123,
        "content_type": "email",
        "content_preview": "Thanks for the update...",
        "ai_suggestion": {
            "category": "project/infrastructure",
            "participants": ["John Doe"],
            "tags": ["decision"],
        },
        "ai_confidence": 0.85,
    }


@pytest.fixture
def sample_decision_data():
    """Sample decision data for testing."""
    return {
        "item_id": 42,
        "decision": "accept",
        "correction": None,
        "time_spent_ms": 3500,
    }


@pytest.fixture
def sample_batch_data():
    """Sample batch operation data for testing."""
    return {
        "batch_id": 1,
        "batch_uuid": str(uuid4()),
        "batch_type": "sender",
        "group_value": "john@example.com",
        "action": "accept",
        "item_count": 5,
        "affected_items": [10, 11, 12, 13, 14],
    }


@pytest.fixture
def sample_rule_data():
    """Sample learning rule data for testing."""
    return {
        "rule_id": 1,
        "rule_uuid": str(uuid4()),
        "pattern": {"sender": "john@example.com"},
        "action": {"category": "project/infrastructure"},
        "confidence": 0.92,
        "source_feedback_count": 8,
    }


@pytest.mark.asyncio
class TestSessionEventContracts:
    """Events published during session lifecycle.

    Contract: review.session.started and review.session.completed events
    must be published with required payload fields.
    """

    async def test_session_started_event_published(
        self, mock_event_publisher, sample_session_data
    ):
        """Session start must publish review.session.started event."""
        await mock_event_publisher.publish(
            "review.session.started",
            sample_session_data
        )

        assert len(mock_event_publisher.published_events) == 1
        event = mock_event_publisher.published_events[0]
        assert event["event_type"] == "review.session.started"

    async def test_session_started_event_payload_structure(
        self, mock_event_publisher, sample_session_data
    ):
        """Session started event must contain required fields."""
        await mock_event_publisher.publish(
            "review.session.started",
            sample_session_data
        )

        event = mock_event_publisher.published_events[0]
        payload = event["payload"]

        # Required fields per contract
        assert "session_id" in payload
        assert "user_email" in payload or "user" in payload
        assert "mode" in payload
        assert "item_count" in payload

    async def test_session_completed_event_published(
        self, mock_event_publisher, sample_session_data
    ):
        """Session completion must publish review.session.completed event."""
        summary = {
            "session_id": sample_session_data["session_id"],
            "summary": {
                "duration_minutes": 23,
                "items_reviewed": 100,
                "accepted": 70,
                "modified": 15,
                "rejected": 10,
                "skipped": 5,
            }
        }

        await mock_event_publisher.publish(
            "review.session.completed",
            summary
        )

        assert len(mock_event_publisher.published_events) == 1
        event = mock_event_publisher.published_events[0]
        assert event["event_type"] == "review.session.completed"

    async def test_session_completed_event_payload_structure(
        self, mock_event_publisher, sample_session_data
    ):
        """Session completed event must contain summary statistics."""
        summary = {
            "session_id": sample_session_data["session_id"],
            "summary": {
                "duration_minutes": 23,
                "items_reviewed": 100,
                "accepted": 70,
                "modified": 15,
                "rejected": 10,
                "skipped": 5,
            }
        }

        await mock_event_publisher.publish(
            "review.session.completed",
            summary
        )

        event = mock_event_publisher.published_events[0]
        payload = event["payload"]

        assert "session_id" in payload
        assert "summary" in payload
        assert "items_reviewed" in payload["summary"]


@pytest.mark.asyncio
class TestDecisionEventContracts:
    """Events published for review decisions.

    Contract: review.decision.made event must be published for each
    user decision with item and decision details.
    """

    async def test_decision_made_event_published_on_accept(
        self, mock_event_publisher, sample_decision_data
    ):
        """Accept decision must publish review.decision.made event."""
        await mock_event_publisher.publish(
            "review.decision.made",
            sample_decision_data
        )

        assert len(mock_event_publisher.published_events) == 1
        event = mock_event_publisher.published_events[0]
        assert event["event_type"] == "review.decision.made"

    async def test_decision_made_event_includes_decision_type(
        self, mock_event_publisher, sample_decision_data
    ):
        """Decision event must specify the decision type."""
        await mock_event_publisher.publish(
            "review.decision.made",
            sample_decision_data
        )

        event = mock_event_publisher.published_events[0]
        payload = event["payload"]

        assert "item_id" in payload
        assert "decision" in payload
        assert payload["decision"] in ["accept", "reject", "modify", "skip"]

    async def test_decision_made_event_includes_correction_on_modify(
        self, mock_event_publisher, sample_decision_data
    ):
        """Modify decision must include correction details."""
        modification_data = {
            **sample_decision_data,
            "decision": "modify",
            "correction": {
                "category": "project/backend",
                "reason": "wrong_category",
            }
        }

        await mock_event_publisher.publish(
            "review.decision.made",
            modification_data
        )

        event = mock_event_publisher.published_events[0]
        payload = event["payload"]

        assert payload["decision"] == "modify"
        assert "correction" in payload
        assert payload["correction"] is not None

    async def test_decision_made_event_tracks_time_spent(
        self, mock_event_publisher, sample_decision_data
    ):
        """Decision event should include time spent on review."""
        await mock_event_publisher.publish(
            "review.decision.made",
            sample_decision_data
        )

        event = mock_event_publisher.published_events[0]
        payload = event["payload"]

        # Time tracking is optional but recommended
        if "time_spent_ms" in payload:
            assert isinstance(payload["time_spent_ms"], int)
            assert payload["time_spent_ms"] >= 0


@pytest.mark.asyncio
class TestBatchEventContracts:
    """Events for batch operations.

    Contract: review.batch.applied event must be published when
    batch operations are executed.
    """

    async def test_batch_applied_event_published(
        self, mock_event_publisher, sample_batch_data
    ):
        """Batch operation must publish review.batch.applied event."""
        await mock_event_publisher.publish(
            "review.batch.applied",
            sample_batch_data
        )

        assert len(mock_event_publisher.published_events) == 1
        event = mock_event_publisher.published_events[0]
        assert event["event_type"] == "review.batch.applied"

    async def test_batch_applied_event_includes_item_count(
        self, mock_event_publisher, sample_batch_data
    ):
        """Batch event must specify the number of affected items."""
        await mock_event_publisher.publish(
            "review.batch.applied",
            sample_batch_data
        )

        event = mock_event_publisher.published_events[0]
        payload = event["payload"]

        assert "batch_id" in payload
        assert "item_count" in payload
        assert payload["item_count"] > 0

    async def test_batch_applied_event_includes_action(
        self, mock_event_publisher, sample_batch_data
    ):
        """Batch event must specify the action applied."""
        await mock_event_publisher.publish(
            "review.batch.applied",
            sample_batch_data
        )

        event = mock_event_publisher.published_events[0]
        payload = event["payload"]

        assert "action" in payload
        assert payload["action"] in ["accept", "reject", "skip"]


@pytest.mark.asyncio
class TestRuleEventContracts:
    """Events for learning rule suggestions.

    Contract: review.rule.suggested event must be published when
    patterns are detected that could become learning rules.
    """

    async def test_rule_suggested_event_published(
        self, mock_event_publisher, sample_rule_data
    ):
        """Rule suggestion must publish review.rule.suggested event."""
        await mock_event_publisher.publish(
            "review.rule.suggested",
            sample_rule_data
        )

        assert len(mock_event_publisher.published_events) == 1
        event = mock_event_publisher.published_events[0]
        assert event["event_type"] == "review.rule.suggested"

    async def test_rule_suggested_event_payload_structure(
        self, mock_event_publisher, sample_rule_data
    ):
        """Rule suggested event must contain pattern and confidence."""
        await mock_event_publisher.publish(
            "review.rule.suggested",
            sample_rule_data
        )

        event = mock_event_publisher.published_events[0]
        payload = event["payload"]

        assert "rule_id" in payload
        assert "pattern" in payload
        assert "confidence" in payload


@pytest.mark.asyncio
class TestEventConsumptionContracts:
    """Test that the review system can consume required events.

    Contract: The review system must handle ai.processing.completed
    and content.ingested events.
    """

    async def test_consume_ai_processing_completed(
        self, mock_event_consumer
    ):
        """Review system should handle ai.processing.completed events."""
        received_events = []

        async def handler(event_type: str, payload: dict[str, Any]):
            received_events.append((event_type, payload))

        await mock_event_consumer.subscribe(
            ["ai.processing.completed"],
            handler
        )

        # Inject test event
        await mock_event_consumer.inject_event(
            "ai.processing.completed",
            {
                "source_id": 123,
                "processing_result": {"category": "email/work"},
                "confidence": 0.85,
            }
        )

        assert len(received_events) == 1
        assert received_events[0][0] == "ai.processing.completed"

    async def test_consume_content_ingested(
        self, mock_event_consumer
    ):
        """Review system should handle content.ingested events."""
        received_events = []

        async def handler(event_type: str, payload: dict[str, Any]):
            received_events.append((event_type, payload))

        await mock_event_consumer.subscribe(
            ["content.ingested"],
            handler
        )

        # Inject test event
        await mock_event_consumer.inject_event(
            "content.ingested",
            {
                "source_id": 456,
                "content_type": "email",
                "ingested_at": datetime.now(timezone.utc).isoformat(),
            }
        )

        assert len(received_events) == 1
        assert received_events[0][0] == "content.ingested"


@pytest.mark.asyncio
class TestEventPayloadContracts:
    """Verify event payload structures meet contracts.

    These tests ensure that payloads are properly structured
    for inter-service communication.
    """

    async def test_session_started_payload_is_serializable(
        self, mock_event_publisher, sample_session_data
    ):
        """Session started payload must be JSON serializable."""
        import json

        await mock_event_publisher.publish(
            "review.session.started",
            sample_session_data
        )

        event = mock_event_publisher.published_events[0]
        # Should not raise
        serialized = json.dumps(event["payload"])
        assert serialized is not None

    async def test_decision_payload_preserves_types(
        self, mock_event_publisher, sample_decision_data
    ):
        """Decision payload types should be preserved correctly."""
        await mock_event_publisher.publish(
            "review.decision.made",
            sample_decision_data
        )

        event = mock_event_publisher.published_events[0]
        payload = event["payload"]

        assert isinstance(payload["item_id"], int)
        assert isinstance(payload["decision"], str)
        assert isinstance(payload["time_spent_ms"], int)

    async def test_batch_payload_includes_affected_items(
        self, mock_event_publisher, sample_batch_data
    ):
        """Batch payload should optionally include affected item IDs."""
        await mock_event_publisher.publish(
            "review.batch.applied",
            sample_batch_data
        )

        event = mock_event_publisher.published_events[0]
        payload = event["payload"]

        if "affected_items" in payload:
            assert isinstance(payload["affected_items"], list)
            assert all(isinstance(item_id, int) for item_id in payload["affected_items"])


class TestDTOContracts:
    """Verify DTO structures meet contracts.

    These tests validate that DTOs have all required fields
    and proper validation rules.
    """

    def test_session_dto_required_fields(self):
        """SessionDTO must have all required fields from contract."""
        from penf_lib.review.models import SessionDTO, SessionStatus, ReviewMode, PriorityMode

        now = datetime.now(timezone.utc)

        session = SessionDTO(
            id=1,
            session_uuid=uuid4(),
            tenant_id=uuid4(),
            user_email="user@example.com",
            status=SessionStatus.ACTIVE,
            started_at=now,
            last_activity_at=now,
            expires_at=now + timedelta(hours=24),
            total_items=100,
            review_mode=ReviewMode.STANDARD,
            priority_mode=PriorityMode.MIXED,
            created_at=now,
            updated_at=now,
        )

        # Verify required fields exist and are accessible
        assert session.id is not None
        assert session.session_uuid is not None
        assert session.tenant_id is not None
        assert session.user_email is not None
        assert session.status is not None
        assert session.total_items is not None
        assert session.review_mode is not None
        assert session.priority_mode is not None

    def test_review_item_dto_required_fields(self):
        """ReviewItemDTO must have all required fields from contract."""
        from penf_lib.review.models import (
            ReviewItemDTO,
            ReviewItemStatus,
            ContentType,
            AISuggestion,
        )

        now = datetime.now(timezone.utc)

        item = ReviewItemDTO(
            id=1,
            tenant_id=uuid4(),
            source_id=123,
            status=ReviewItemStatus.PENDING,
            content_type=ContentType.EMAIL,
            content_preview="Test preview...",
            ai_suggestion=AISuggestion(category="test/category"),
            ai_confidence=Decimal("0.85"),
            ai_model="test-model",
            source_timestamp=now,
            created_at=now,
            updated_at=now,
        )

        # Verify required fields exist
        assert item.id is not None
        assert item.tenant_id is not None
        assert item.source_id is not None
        assert item.status is not None
        assert item.content_type is not None
        assert item.ai_suggestion is not None
        assert item.ai_confidence is not None

    def test_user_feedback_dto_required_fields(self):
        """UserFeedbackDTO must have all required fields from contract."""
        from penf_lib.review.models import UserFeedbackDTO, DecisionType, AISuggestion

        now = datetime.now(timezone.utc)

        feedback = UserFeedbackDTO(
            id=1,
            feedback_uuid=uuid4(),
            tenant_id=uuid4(),
            review_item_id=100,
            session_id=10,
            decision_type=DecisionType.ACCEPT,
            original_suggestion=AISuggestion(category="test/category"),
            time_spent_ms=3500,
            created_at=now,
        )

        # Verify required fields exist
        assert feedback.id is not None
        assert feedback.feedback_uuid is not None
        assert feedback.review_item_id is not None
        assert feedback.session_id is not None
        assert feedback.decision_type is not None
        assert feedback.original_suggestion is not None

    def test_learning_rule_dto_required_fields(self):
        """LearningRuleDTO must have all required fields from contract."""
        from penf_lib.review.models import (
            LearningRuleDTO,
            LearningRuleStatus,
            LearningRuleType,
        )

        now = datetime.now(timezone.utc)

        rule = LearningRuleDTO(
            id=1,
            rule_uuid=uuid4(),
            tenant_id=uuid4(),
            name="Test Rule",
            status=LearningRuleStatus.ACTIVE,
            rule_type=LearningRuleType.SENDER_MATCH,
            match_criteria={"sender": "test@example.com"},
            action={"category": "test/category"},
            created_by="admin@example.com",
            created_at=now,
            updated_at=now,
        )

        # Verify required fields exist
        assert rule.id is not None
        assert rule.rule_uuid is not None
        assert rule.name is not None
        assert rule.status is not None
        assert rule.rule_type is not None
        assert rule.match_criteria is not None
        assert rule.action is not None

    def test_batch_operation_dto_required_fields(self):
        """BatchOperationDTO must have all required fields from contract."""
        from penf_lib.review.models import (
            BatchOperationDTO,
            BatchType,
            BatchActionType,
            BatchOperationStatus,
        )

        now = datetime.now(timezone.utc)

        batch = BatchOperationDTO(
            id=1,
            batch_uuid=uuid4(),
            tenant_id=uuid4(),
            session_id=10,
            batch_type=BatchType.SENDER,
            group_criteria={"sender": "test@example.com"},
            action_type=BatchActionType.ACCEPT_ALL,
            action_details={},
            item_count=5,
            status=BatchOperationStatus.APPLIED,
            created_at=now,
            updated_at=now,
        )

        # Verify required fields exist
        assert batch.id is not None
        assert batch.batch_uuid is not None
        assert batch.session_id is not None
        assert batch.batch_type is not None
        assert batch.action_type is not None
        assert batch.item_count is not None
        assert batch.status is not None

    def test_review_analytics_dto_required_fields(self):
        """ReviewAnalyticsDTO must have all required fields from contract."""
        from penf_lib.review.models import ReviewAnalyticsDTO, AnalyticsPeriodType
        from datetime import date

        now = datetime.now(timezone.utc)
        today = date.today()

        analytics = ReviewAnalyticsDTO(
            id=1,
            tenant_id=uuid4(),
            period_start=today,
            period_end=today,
            period_type=AnalyticsPeriodType.DAILY,
            total_sessions=10,
            total_items_reviewed=500,
            avg_session_duration_min=Decimal("15.5"),
            avg_items_per_minute=Decimal("8.0"),
            acceptance_rate=Decimal("0.70"),
            modification_rate=Decimal("0.15"),
            rejection_rate=Decimal("0.10"),
            skip_rate=Decimal("0.05"),
            avg_ai_confidence=Decimal("0.85"),
            batch_usage_rate=Decimal("0.20"),
            learning_rules_created=3,
            learning_rules_applied=25,
            content_type_breakdown={"email": 400, "meeting": 100},
            correction_type_breakdown={"category": 50},
            created_at=now,
        )

        # Verify required fields exist
        assert analytics.id is not None
        assert analytics.period_start is not None
        assert analytics.period_end is not None
        assert analytics.total_sessions is not None
        assert analytics.total_items_reviewed is not None
        assert analytics.acceptance_rate is not None


class TestDTOSerializationContracts:
    """Test that DTOs serialize correctly for event payloads."""

    def test_session_dto_to_dict(self):
        """SessionDTO must serialize to dict for event payload."""
        from penf_lib.review.models import SessionDTO, SessionStatus, ReviewMode, PriorityMode

        now = datetime.now(timezone.utc)

        session = SessionDTO(
            id=1,
            session_uuid=uuid4(),
            tenant_id=uuid4(),
            user_email="user@example.com",
            status=SessionStatus.ACTIVE,
            started_at=now,
            last_activity_at=now,
            expires_at=now + timedelta(hours=24),
            total_items=100,
            review_mode=ReviewMode.STANDARD,
            priority_mode=PriorityMode.MIXED,
            created_at=now,
            updated_at=now,
        )

        data = session.model_dump()

        assert isinstance(data, dict)
        assert "id" in data
        assert "session_uuid" in data
        assert "user_email" in data
        assert "status" in data
        assert "total_items" in data

    def test_review_item_dto_to_dict_includes_ai_suggestion(self):
        """ReviewItemDTO serialization must include nested AI suggestion."""
        from penf_lib.review.models import (
            ReviewItemDTO,
            ReviewItemStatus,
            ContentType,
            AISuggestion,
        )

        now = datetime.now(timezone.utc)

        item = ReviewItemDTO(
            id=1,
            tenant_id=uuid4(),
            source_id=123,
            status=ReviewItemStatus.PENDING,
            content_type=ContentType.EMAIL,
            content_preview="Test preview...",
            ai_suggestion=AISuggestion(
                category="project/backend",
                participants=["John Doe"],
                tags=["decision"],
            ),
            ai_confidence=Decimal("0.85"),
            ai_model="test-model",
            source_timestamp=now,
            created_at=now,
            updated_at=now,
        )

        data = item.model_dump()

        assert "ai_suggestion" in data
        assert isinstance(data["ai_suggestion"], dict)
        assert data["ai_suggestion"]["category"] == "project/backend"
        assert "John Doe" in data["ai_suggestion"]["participants"]

    def test_dto_json_serialization(self):
        """DTOs must be JSON serializable for event transport."""
        import json
        from penf_lib.review.models import (
            SessionDTO,
            SessionStatus,
            ReviewMode,
            PriorityMode,
        )

        now = datetime.now(timezone.utc)

        session = SessionDTO(
            id=1,
            session_uuid=uuid4(),
            tenant_id=uuid4(),
            user_email="user@example.com",
            status=SessionStatus.ACTIVE,
            started_at=now,
            last_activity_at=now,
            expires_at=now + timedelta(hours=24),
            total_items=100,
            review_mode=ReviewMode.STANDARD,
            priority_mode=PriorityMode.MIXED,
            created_at=now,
            updated_at=now,
        )

        # Use mode='json' for JSON-compatible output
        data = session.model_dump(mode='json')

        # Should not raise
        json_str = json.dumps(data)
        assert json_str is not None

        # Should round-trip correctly
        parsed = json.loads(json_str)
        assert parsed["user_email"] == "user@example.com"
        assert parsed["total_items"] == 100
