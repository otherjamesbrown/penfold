"""Integration tests for the Daily Review workflow.

Tests the complete review workflow end-to-end through the manager layer,
mocking only the repository layer. These tests verify:
- Full session lifecycle (start -> review -> complete)
- Queue population and item retrieval
- Decision recording (accept, reject, modify, skip)
- Undo functionality
- Batch operations
- Rule application during review
- Session persistence and resumption

Based on spec 006-daily-review requirements.
"""

from __future__ import annotations

from datetime import datetime, timedelta, timezone
from decimal import Decimal
from typing import Any, Optional
from unittest.mock import AsyncMock, MagicMock
from uuid import UUID, uuid4

import pytest

from penf_lib.review import (
    # Managers
    BatchManager,
    FeedbackManager,
    QueueManager,
    RuleManager,
    SessionManager,
    # DTOs
    AISuggestion,
    BatchOperationDTO,
    LearningRuleDTO,
    ReviewItemDTO,
    SessionDTO,
    UserCorrection,
    UserFeedbackDTO,
    # Enums
    BatchOperationStatus,
    ContentType,
    DecisionType,
    LearningRuleStatus,
    LearningRuleType,
    PriorityMode,
    ReviewItemStatus,
    ReviewMode,
    SessionStatus,
    # Batch types - BatchActionEnum is the one from batch.py with ACCEPT/REJECT
    BatchActionEnum,
    BatchGroupType,
    BatchPreview,
    BatchResult,
    # Rule types
    RuleApplicationResult,
    # Exceptions
    ActiveSessionExistsError,
    InvalidSessionStateError,
    PendingItemsError,
    SessionNotFoundError,
    UndoNotEligibleError,
)


# =============================================================================
# FIXTURES
# =============================================================================


@pytest.fixture
def tenant_id() -> UUID:
    """Generate a test tenant ID."""
    return uuid4()


@pytest.fixture
def user_email() -> str:
    """Test user email address."""
    return "reviewer@example.com"


def create_mock_session(
    session_id: int = 1,
    tenant_id: UUID | None = None,
    user_email: str = "reviewer@example.com",
    status: SessionStatus = SessionStatus.ACTIVE,
    total_items: int = 10,
    items_reviewed: int = 0,
    items_accepted: int = 0,
    items_rejected: int = 0,
    items_modified: int = 0,
    items_skipped: int = 0,
    current_position: int = 0,
) -> MagicMock:
    """Create a mock session object."""
    if tenant_id is None:
        tenant_id = uuid4()

    now = datetime.now(timezone.utc)
    session = MagicMock()
    session.id = session_id
    session.session_uuid = uuid4()
    session.tenant_id = tenant_id
    session.user_email = user_email
    session.status = status
    session.started_at = now
    session.last_activity_at = now
    session.completed_at = None
    session.expires_at = now + timedelta(hours=24)
    session.current_position = current_position
    session.total_items = total_items
    session.items_reviewed = items_reviewed
    session.items_accepted = items_accepted
    session.items_rejected = items_rejected
    session.items_modified = items_modified
    session.items_skipped = items_skipped
    session.review_mode = ReviewMode.STANDARD
    session.priority_mode = PriorityMode.MIXED
    session.filter_criteria = {}
    session.session_metadata = {}
    session.created_at = now
    session.updated_at = now
    return session


def create_mock_review_item(
    item_id: int = 1,
    tenant_id: UUID | None = None,
    session_id: int | None = None,
    source_id: int = 100,
    status: ReviewItemStatus = ReviewItemStatus.PENDING,
    content_type: ContentType = ContentType.EMAIL,
    ai_confidence: Decimal = Decimal("0.75"),
    business_importance: int = 5,
    queue_position: int | None = None,
    category: str = "work/projects",
    participants: list[str] | None = None,
    tags: list[str] | None = None,
) -> ReviewItemDTO:
    """Create a mock review item DTO."""
    if tenant_id is None:
        tenant_id = uuid4()
    if participants is None:
        participants = ["sender@example.com"]
    if tags is None:
        tags = ["important"]

    now = datetime.now(timezone.utc)
    return ReviewItemDTO(
        id=item_id,
        tenant_id=tenant_id,
        session_id=session_id,
        source_id=source_id,
        processing_result_id=uuid4(),
        queue_position=queue_position,
        status=status,
        content_type=content_type,
        content_preview=f"Test content preview for item {item_id}",
        ai_suggestion=AISuggestion(
            category=category,
            participants=participants,
            tags=tags,
        ),
        ai_confidence=ai_confidence,
        ai_model="gemini-1.5-pro",
        business_importance=business_importance,
        source_timestamp=now - timedelta(hours=2),
        reviewed_at=None,
        review_duration_ms=None,
        user_decision=None,
        user_correction=None,
        batch_id=None,
        undo_eligible=True,
        undo_deadline=None,
        created_at=now,
        updated_at=now,
    )


def create_mock_feedback(
    feedback_id: int = 1,
    tenant_id: UUID | None = None,
    review_item_id: int = 1,
    session_id: int = 1,
    decision_type: DecisionType = DecisionType.ACCEPT,
    time_spent_ms: int = 5000,
    undo_eligible: bool = True,
) -> dict[str, Any]:
    """Create a mock feedback record as dict."""
    if tenant_id is None:
        tenant_id = uuid4()

    now = datetime.now(timezone.utc)
    return {
        "id": feedback_id,
        "feedback_uuid": uuid4(),
        "tenant_id": tenant_id,
        "review_item_id": review_item_id,
        "session_id": session_id,
        "decision_type": decision_type,
        "original_suggestion": {
            "category": "work/projects",
            "participants": ["sender@example.com"],
            "tags": ["important"],
        },
        "user_correction": None,
        "correction_reason": None,
        "correction_notes": None,
        "confidence_feedback": None,
        "time_spent_ms": time_spent_ms,
        "was_batch_decision": False,
        "batch_id": None,
        "learning_rule_id": None,
        "learning_signal_quality": Decimal("0.30"),
        "undo_eligible": undo_eligible,
        "undo_deadline": now + timedelta(minutes=5),
        "created_at": now,
    }


# =============================================================================
# TEST CLASS: Full Review Workflow
# =============================================================================


class TestFullReviewWorkflow:
    """Integration tests for complete review session workflow."""

    @pytest.fixture
    def mock_session_repo(self) -> AsyncMock:
        """Create mock session repository."""
        repo = AsyncMock()
        repo.create_session = AsyncMock()
        repo.get_session_by_id = AsyncMock()
        repo.get_active_session_for_user = AsyncMock()
        repo.update_session = AsyncMock()
        return repo

    @pytest.fixture
    def mock_queue_repo(self) -> AsyncMock:
        """Create mock queue repository."""
        repo = AsyncMock()
        repo.get_pending_items_for_queue = AsyncMock()
        repo.get_queue_items = AsyncMock()
        repo.update_item_queue_position = AsyncMock()
        return repo

    @pytest.fixture
    def mock_feedback_repo(self) -> AsyncMock:
        """Create mock feedback repository."""
        repo = AsyncMock()
        repo.create_feedback = AsyncMock()
        repo.get_feedback_by_id = AsyncMock()
        repo.get_feedback_by_review_item = AsyncMock()
        repo.get_undo_eligible_feedback = AsyncMock()
        repo.mark_feedback_undone = AsyncMock()
        repo.update_feedback = AsyncMock()
        return repo

    @pytest.mark.asyncio
    async def test_complete_review_workflow(
        self,
        mock_session_repo: AsyncMock,
        mock_queue_repo: AsyncMock,
        mock_feedback_repo: AsyncMock,
        tenant_id: UUID,
        user_email: str,
    ):
        """Test complete workflow: start session -> review items -> complete session."""
        # Setup managers
        session_manager = SessionManager(repository=mock_session_repo)
        queue_manager = QueueManager(repository=mock_queue_repo)
        feedback_manager = FeedbackManager(repository=mock_feedback_repo)

        # 1. Start a new review session
        mock_session_repo.get_active_session_for_user.return_value = None
        mock_session = create_mock_session(
            session_id=1,
            tenant_id=tenant_id,
            user_email=user_email,
            total_items=3,
        )
        mock_session_repo.create_session.return_value = mock_session

        session = await session_manager.create_session(
            tenant_id=tenant_id,
            user_email=user_email,
            total_items=3,
            mode=ReviewMode.STANDARD,
            priority=PriorityMode.MIXED,
        )

        assert session.id == 1
        assert session.status == SessionStatus.ACTIVE
        assert session.total_items == 3

        # 2. Populate the queue
        items = [
            create_mock_review_item(item_id=i, tenant_id=tenant_id, source_id=100 + i)
            for i in range(1, 4)
        ]
        mock_queue_repo.get_pending_items_for_queue.return_value = items

        queue_items = await queue_manager.populate_queue(
            tenant_id=tenant_id,
            session_id=session.id,
            priority_mode=PriorityMode.MIXED,
            limit=10,
        )

        assert len(queue_items) == 3
        assert all(item.queue_position is not None for item in queue_items)

        # 3. Get next item for review
        mock_queue_repo.get_queue_items.return_value = [items[0]]
        next_item = await queue_manager.get_next_item(session_id=session.id)

        assert next_item is not None
        assert next_item.id == 1

        # 4. Accept the first item
        mock_feedback_repo.create_feedback.return_value = create_mock_feedback(
            feedback_id=1,
            tenant_id=tenant_id,
            review_item_id=1,
            session_id=session.id,
            decision_type=DecisionType.ACCEPT,
        )

        feedback = await feedback_manager.record_accept(
            item=items[0],
            session_id=session.id,
            time_spent_ms=3000,
        )

        assert feedback.decision_type == DecisionType.ACCEPT

        # 5. Update session progress
        # The mock starts with 0 counters, update_progress increments them
        mock_session_repo.get_session_by_id.return_value = mock_session
        mock_session_repo.update_session.side_effect = lambda s: s

        updated_session = await session_manager.update_progress(
            session_id=session.id,
            position=1,
            decision_type="accept",
        )

        # After first accept, counters should be incremented from 0
        assert mock_session.items_reviewed == 1
        assert mock_session.items_accepted == 1

        # 6. Process remaining items - accept and reject
        await session_manager.update_progress(
            session_id=session.id,
            position=2,
            decision_type="accept",
        )
        await session_manager.update_progress(
            session_id=session.id,
            position=3,
            decision_type="reject",
        )

        # Verify final counts
        assert mock_session.items_reviewed == 3
        assert mock_session.items_accepted == 2
        assert mock_session.items_rejected == 1

        # 7. Complete the session
        await session_manager.complete_session(session_id=session.id)

        assert mock_session.status == SessionStatus.COMPLETED
        mock_session_repo.update_session.assert_called()

    @pytest.mark.asyncio
    async def test_review_all_decision_types(
        self,
        mock_feedback_repo: AsyncMock,
        tenant_id: UUID,
    ):
        """Test recording all decision types: accept, reject, modify, skip."""
        feedback_manager = FeedbackManager(repository=mock_feedback_repo)
        session_id = 1

        # Create items for each decision type
        items = [
            create_mock_review_item(item_id=i, tenant_id=tenant_id)
            for i in range(1, 5)
        ]

        # Test ACCEPT
        mock_feedback_repo.create_feedback.return_value = create_mock_feedback(
            feedback_id=1, decision_type=DecisionType.ACCEPT
        )
        accept_feedback = await feedback_manager.record_accept(
            item=items[0], session_id=session_id, time_spent_ms=2000
        )
        assert accept_feedback.decision_type == DecisionType.ACCEPT

        # Test REJECT
        mock_feedback_repo.create_feedback.return_value = create_mock_feedback(
            feedback_id=2, decision_type=DecisionType.REJECT
        )
        reject_feedback = await feedback_manager.record_reject(
            item=items[1], session_id=session_id, reason="spam", time_spent_ms=1500
        )
        assert reject_feedback.decision_type == DecisionType.REJECT

        # Test MODIFY
        correction = UserCorrection(
            category="personal/finance",
            participants=["accountant@example.com"],
            reason="miscategorized",
        )
        mock_feedback = create_mock_feedback(
            feedback_id=3, decision_type=DecisionType.MODIFY
        )
        mock_feedback["user_correction"] = {
            "category": "personal/finance",
            "participants": ["accountant@example.com"],
            "reason": "miscategorized",
        }
        mock_feedback_repo.create_feedback.return_value = mock_feedback

        modify_feedback = await feedback_manager.record_modify(
            item=items[2], session_id=session_id, correction=correction, time_spent_ms=8000
        )
        assert modify_feedback.decision_type == DecisionType.MODIFY

        # Test SKIP
        mock_feedback_repo.create_feedback.return_value = create_mock_feedback(
            feedback_id=4, decision_type=DecisionType.SKIP
        )
        skip_feedback = await feedback_manager.record_skip(
            item=items[3], session_id=session_id, reason="need_context", time_spent_ms=1000
        )
        assert skip_feedback.decision_type == DecisionType.SKIP


# =============================================================================
# TEST CLASS: Session Persistence
# =============================================================================


class TestSessionPersistence:
    """Integration tests for session state persistence across operations."""

    @pytest.fixture
    def mock_repository(self) -> AsyncMock:
        """Create mock session repository."""
        repo = AsyncMock()
        repo.create_session = AsyncMock()
        repo.get_session_by_id = AsyncMock()
        repo.get_active_session_for_user = AsyncMock()
        repo.update_session = AsyncMock()
        return repo

    @pytest.mark.asyncio
    async def test_session_persists_through_pause_resume(
        self,
        mock_repository: AsyncMock,
        tenant_id: UUID,
        user_email: str,
    ):
        """Test session state persists through pause and resume operations."""
        session_manager = SessionManager(repository=mock_repository)

        # Create session with some progress
        session = create_mock_session(
            session_id=1,
            tenant_id=tenant_id,
            user_email=user_email,
            status=SessionStatus.ACTIVE,
            total_items=10,
            items_reviewed=5,
            items_accepted=3,
            items_rejected=1,
            items_modified=1,
            current_position=5,
        )
        mock_repository.get_session_by_id.return_value = session
        mock_repository.update_session.side_effect = lambda s: s

        # Pause the session
        paused = await session_manager.pause_session(session_id=1)
        assert session.status == SessionStatus.PAUSED

        # Verify progress is preserved
        assert session.items_reviewed == 5
        assert session.items_accepted == 3
        assert session.current_position == 5

        # Resume the session
        resumed = await session_manager.resume_session(session_id=1)
        assert session.status == SessionStatus.ACTIVE

        # Verify progress is still preserved
        assert session.items_reviewed == 5
        assert session.items_accepted == 3
        assert session.current_position == 5

    @pytest.mark.asyncio
    async def test_session_counters_update_correctly(
        self,
        mock_repository: AsyncMock,
    ):
        """Test all session counters update correctly through multiple decisions."""
        session_manager = SessionManager(repository=mock_repository)

        session = create_mock_session(
            session_id=1,
            status=SessionStatus.ACTIVE,
            total_items=10,
        )
        mock_repository.get_session_by_id.return_value = session
        mock_repository.update_session.side_effect = lambda s: s

        # Process multiple decisions
        decisions = ["accept", "accept", "reject", "modify", "skip", "accept"]

        for i, decision in enumerate(decisions, start=1):
            await session_manager.update_progress(
                session_id=1,
                position=i,
                decision_type=decision,
            )

        # Verify final counts
        assert session.items_reviewed == 6
        assert session.items_accepted == 3  # 3 accepts
        assert session.items_rejected == 1  # 1 reject
        assert session.items_modified == 1  # 1 modify
        assert session.items_skipped == 1  # 1 skip
        assert session.current_position == 6

    @pytest.mark.asyncio
    async def test_get_existing_active_session(
        self,
        mock_repository: AsyncMock,
        tenant_id: UUID,
        user_email: str,
    ):
        """Test retrieving existing active session for user."""
        session_manager = SessionManager(repository=mock_repository)

        existing_session = create_mock_session(
            session_id=42,
            tenant_id=tenant_id,
            user_email=user_email,
            status=SessionStatus.ACTIVE,
            items_reviewed=7,
        )
        mock_repository.get_active_session_for_user.return_value = existing_session

        session = await session_manager.get_active_session(tenant_id, user_email)

        assert session is not None
        assert session.id == 42
        assert session.items_reviewed == 7

    @pytest.mark.asyncio
    async def test_cannot_create_duplicate_session(
        self,
        mock_repository: AsyncMock,
        tenant_id: UUID,
        user_email: str,
    ):
        """Test that creating session fails when active session exists."""
        session_manager = SessionManager(repository=mock_repository)

        existing_session = create_mock_session(
            session_id=42,
            tenant_id=tenant_id,
            user_email=user_email,
        )
        mock_repository.get_active_session_for_user.return_value = existing_session

        with pytest.raises(ActiveSessionExistsError) as exc_info:
            await session_manager.create_session(
                tenant_id=tenant_id,
                user_email=user_email,
                total_items=10,
            )

        assert exc_info.value.details["existing_session_id"] == 42


# =============================================================================
# TEST CLASS: Batch Workflow
# =============================================================================


class TestBatchWorkflow:
    """Integration tests for batch operations workflow."""

    @pytest.fixture
    def mock_batch_repo(self) -> AsyncMock:
        """Create mock batch repository."""
        repo = AsyncMock()
        repo.get_pending_items_for_session = AsyncMock()
        repo.get_items_by_source_id = AsyncMock()
        repo.update_items_batch = AsyncMock()
        repo.create_batch_operation = AsyncMock()
        repo.get_batch_operation = AsyncMock()
        repo.update_batch_operation = AsyncMock()
        repo.get_items_by_batch_id = AsyncMock()
        return repo

    @pytest.fixture
    def mock_feedback_repo(self) -> AsyncMock:
        """Create mock feedback repository."""
        repo = AsyncMock()
        repo.create_feedback = AsyncMock()
        repo.get_feedback_by_id = AsyncMock()
        repo.get_feedback_by_review_item = AsyncMock()
        repo.get_undo_eligible_feedback = AsyncMock()
        repo.mark_feedback_undone = AsyncMock()
        repo.update_feedback = AsyncMock()
        return repo

    @pytest.mark.asyncio
    async def test_batch_accept_by_sender(
        self,
        mock_batch_repo: AsyncMock,
        mock_feedback_repo: AsyncMock,
        tenant_id: UUID,
    ):
        """Test batch accepting items from same sender."""
        feedback_manager = FeedbackManager(repository=mock_feedback_repo)
        batch_manager = BatchManager(
            repository=mock_batch_repo,
            feedback_manager=feedback_manager,
        )

        # Create items from same sender
        sender = "reports@company.com"
        items = [
            create_mock_review_item(
                item_id=i,
                tenant_id=tenant_id,
                session_id=1,
                participants=[sender],
            )
            for i in range(1, 6)
        ]

        # Mock repository returns items
        mock_batch_repo.get_pending_items_for_session.return_value = items
        mock_batch_repo.update_items_batch.return_value = 5
        mock_batch_repo.create_batch_operation.return_value = MagicMock()
        mock_feedback_repo.create_feedback.return_value = create_mock_feedback()

        # Find similar items by sender
        reference_item = items[0]
        similar_items = await batch_manager.find_similar_items(
            session_id=1,
            reference_item=reference_item,
            group_by=BatchGroupType.SENDER,
        )

        # All items should match (except reference)
        assert len(similar_items) == 4

        # Execute batch accept
        result = await batch_manager.execute_batch(
            session_id=1,
            items=items,
            action=BatchActionEnum.ACCEPT,
            reason="Trusted sender",
        )

        assert result.success is True
        assert result.items_affected == 5
        assert result.action == BatchActionEnum.ACCEPT
        assert result.undo_eligible is True

    @pytest.mark.asyncio
    async def test_batch_reject_by_category(
        self,
        mock_batch_repo: AsyncMock,
        mock_feedback_repo: AsyncMock,
        tenant_id: UUID,
    ):
        """Test batch rejecting items with same category."""
        feedback_manager = FeedbackManager(repository=mock_feedback_repo)
        batch_manager = BatchManager(
            repository=mock_batch_repo,
            feedback_manager=feedback_manager,
        )

        # Create items with same category
        category = "spam/newsletters"
        items = [
            create_mock_review_item(
                item_id=i,
                tenant_id=tenant_id,
                session_id=1,
                category=category,
            )
            for i in range(1, 4)
        ]

        mock_batch_repo.get_pending_items_for_session.return_value = items
        mock_batch_repo.update_items_batch.return_value = 3
        mock_batch_repo.create_batch_operation.return_value = MagicMock()
        mock_feedback_repo.create_feedback.return_value = create_mock_feedback()

        # Execute batch reject
        result = await batch_manager.execute_batch(
            session_id=1,
            items=items,
            action=BatchActionEnum.REJECT,
            reason="Spam category",
        )

        assert result.success is True
        assert result.items_affected == 3
        assert result.action == BatchActionEnum.REJECT

    @pytest.mark.asyncio
    async def test_batch_preview_shows_time_savings(
        self,
        mock_batch_repo: AsyncMock,
        mock_feedback_repo: AsyncMock,
        tenant_id: UUID,
    ):
        """Test batch preview calculates estimated time savings."""
        feedback_manager = FeedbackManager(repository=mock_feedback_repo)
        batch_manager = BatchManager(
            repository=mock_batch_repo,
            feedback_manager=feedback_manager,
        )

        items = [
            create_mock_review_item(item_id=i, tenant_id=tenant_id)
            for i in range(1, 11)  # 10 items
        ]

        preview = await batch_manager.preview_batch(
            items=items,
            action=BatchActionEnum.ACCEPT,
            group_type=BatchGroupType.SENDER,
            group_value="sender@example.com",
        )

        # 10 items * 15 seconds average = 150 seconds saved
        assert preview.item_count == 10
        assert preview.estimated_time_saved_seconds == 150
        assert preview.action == BatchActionEnum.ACCEPT


# =============================================================================
# TEST CLASS: Rule Application
# =============================================================================


class TestRuleApplication:
    """Integration tests for rule application during review."""

    @pytest.fixture
    def mock_rule_repo(self) -> AsyncMock:
        """Create mock rule repository."""
        repo = AsyncMock()
        repo.get_rules_for_tenant = AsyncMock()
        repo.get_rule_by_id = AsyncMock()
        repo.create_rule = AsyncMock()
        repo.update_rule = AsyncMock()
        repo.get_pending_suggestions = AsyncMock()
        return repo

    @pytest.mark.asyncio
    async def test_rules_applied_to_matching_items(
        self,
        mock_rule_repo: AsyncMock,
        tenant_id: UUID,
    ):
        """Test that matching rules are applied to review items."""
        rule_manager = RuleManager(repository=mock_rule_repo)

        # Create a sender match rule
        sender_rule = LearningRuleDTO(
            id=1,
            rule_uuid=uuid4(),
            tenant_id=tenant_id,
            name="Vendor Emails",
            description="Auto-categorize vendor emails",
            status=LearningRuleStatus.ACTIVE,
            rule_type=LearningRuleType.SENDER_MATCH,
            match_criteria={"sender": "vendor@supplies.com"},
            action={"category": "work/vendors"},
            confidence_threshold=Decimal("0.800"),
            priority=50,
            derived_from_count=5,
            times_applied=10,
            times_overridden=1,
            effectiveness_score=Decimal("0.900"),
            last_applied_at=datetime.now(timezone.utc),
            expires_at=None,
            created_by="user@example.com",
            created_at=datetime.now(timezone.utc),
            updated_at=datetime.now(timezone.utc),
            is_deleted=False,
            deleted_at=None,
        )

        mock_rule_repo.get_rules_for_tenant.return_value = [sender_rule]

        # Create item that matches the rule
        item = create_mock_review_item(
            item_id=1,
            tenant_id=tenant_id,
            ai_confidence=Decimal("0.70"),  # Below rule threshold
            participants=["vendor@supplies.com"],
            category="work/misc",
        )

        result = await rule_manager.apply_rules(tenant_id=tenant_id, item=item)

        assert len(result.applied_rules) == 1
        assert result.applied_rules[0].name == "Vendor Emails"
        assert result.modified_suggestion.category == "work/vendors"
        assert len(result.modifications) == 1

    @pytest.mark.asyncio
    async def test_rules_priority_order(
        self,
        mock_rule_repo: AsyncMock,
        tenant_id: UUID,
    ):
        """Test that rules are applied in priority order."""
        rule_manager = RuleManager(repository=mock_rule_repo)

        now = datetime.now(timezone.utc)

        # Create two rules with different priorities
        high_priority_rule = LearningRuleDTO(
            id=1,
            rule_uuid=uuid4(),
            tenant_id=tenant_id,
            name="High Priority Rule",
            status=LearningRuleStatus.ACTIVE,
            rule_type=LearningRuleType.CONTENT_PATTERN,
            match_criteria={"keywords": ["urgent"]},
            action={"category": "work/urgent"},
            confidence_threshold=Decimal("0.800"),
            priority=10,  # Higher priority (lower number)
            derived_from_count=0,
            times_applied=0,
            times_overridden=0,
            created_by="system",
            created_at=now,
            updated_at=now,
            is_deleted=False,
        )

        low_priority_rule = LearningRuleDTO(
            id=2,
            rule_uuid=uuid4(),
            tenant_id=tenant_id,
            name="Low Priority Rule",
            status=LearningRuleStatus.ACTIVE,
            rule_type=LearningRuleType.CONTENT_PATTERN,
            match_criteria={"keywords": ["urgent"]},
            action={"category": "work/general"},
            confidence_threshold=Decimal("0.800"),
            priority=100,  # Lower priority (higher number)
            derived_from_count=0,
            times_applied=0,
            times_overridden=0,
            created_by="system",
            created_at=now,
            updated_at=now,
            is_deleted=False,
        )

        mock_rule_repo.get_rules_for_tenant.return_value = [
            low_priority_rule,
            high_priority_rule,
        ]

        item = create_mock_review_item(
            item_id=1,
            tenant_id=tenant_id,
            ai_confidence=Decimal("0.70"),
        )
        # Modify content preview to match rule
        item.content_preview = "This is an urgent request"

        result = await rule_manager.apply_rules(tenant_id=tenant_id, item=item)

        # High priority rule should be applied first
        assert len(result.applied_rules) >= 1
        if len(result.applied_rules) > 0:
            assert result.applied_rules[0].priority == 10

    @pytest.mark.asyncio
    async def test_rule_effectiveness_tracking(
        self,
        mock_rule_repo: AsyncMock,
        tenant_id: UUID,
    ):
        """Test that rule effectiveness is tracked based on outcomes."""
        rule_manager = RuleManager(repository=mock_rule_repo)

        now = datetime.now(timezone.utc)
        rule = LearningRuleDTO(
            id=1,
            rule_uuid=uuid4(),
            tenant_id=tenant_id,
            name="Test Rule",
            status=LearningRuleStatus.ACTIVE,
            rule_type=LearningRuleType.SENDER_MATCH,
            match_criteria={"sender": "test@example.com"},
            action={"category": "work/test"},
            confidence_threshold=Decimal("0.800"),
            priority=100,
            derived_from_count=0,
            times_applied=10,
            times_overridden=2,
            effectiveness_score=Decimal("0.800"),
            created_by="system",
            created_at=now,
            updated_at=now,
            is_deleted=False,
        )

        mock_rule_repo.get_rule_by_id.return_value = rule
        mock_rule_repo.update_rule.return_value = rule

        # Record outcome where user accepted rule suggestion
        await rule_manager.record_rule_outcome(rule_id=1, was_overridden=False)

        # Verify stats updated
        assert rule.times_applied == 11
        assert rule.times_overridden == 2  # Unchanged

        # Record outcome where user overrode rule
        await rule_manager.record_rule_outcome(rule_id=1, was_overridden=True)

        assert rule.times_applied == 12
        assert rule.times_overridden == 3


# =============================================================================
# TEST CLASS: Undo Workflow
# =============================================================================


class TestUndoWorkflow:
    """Integration tests for undo functionality."""

    @pytest.fixture
    def mock_feedback_repo(self) -> AsyncMock:
        """Create mock feedback repository."""
        repo = AsyncMock()
        repo.create_feedback = AsyncMock()
        repo.get_feedback_by_id = AsyncMock()
        repo.get_feedback_by_review_item = AsyncMock()
        repo.get_undo_eligible_feedback = AsyncMock()
        repo.mark_feedback_undone = AsyncMock()
        repo.update_feedback = AsyncMock()
        return repo

    @pytest.fixture
    def mock_batch_repo(self) -> AsyncMock:
        """Create mock batch repository."""
        repo = AsyncMock()
        repo.get_batch_operation = AsyncMock()
        repo.update_batch_operation = AsyncMock()
        repo.get_items_by_batch_id = AsyncMock()
        repo.update_items_batch = AsyncMock()
        repo.get_pending_items_for_session = AsyncMock()
        repo.create_batch_operation = AsyncMock()
        return repo

    @pytest.mark.asyncio
    async def test_undo_single_decision(
        self,
        mock_feedback_repo: AsyncMock,
        tenant_id: UUID,
    ):
        """Test undoing a single review decision."""
        feedback_manager = FeedbackManager(repository=mock_feedback_repo)

        # Create feedback that is undo eligible
        now = datetime.now(timezone.utc)
        feedback = create_mock_feedback(
            feedback_id=1,
            tenant_id=tenant_id,
            decision_type=DecisionType.ACCEPT,
            undo_eligible=True,
        )
        feedback["undo_deadline"] = now + timedelta(minutes=5)

        mock_feedback_repo.get_feedback_by_id.return_value = feedback
        mock_feedback_repo.mark_feedback_undone.return_value = True

        result = await feedback_manager.undo_decision(feedback_id=1)

        assert result is True
        mock_feedback_repo.mark_feedback_undone.assert_called_once_with(1)

    @pytest.mark.asyncio
    async def test_undo_expired_raises_error(
        self,
        mock_feedback_repo: AsyncMock,
        tenant_id: UUID,
    ):
        """Test that undoing expired feedback raises error."""
        feedback_manager = FeedbackManager(repository=mock_feedback_repo)

        # Create feedback with expired undo window
        now = datetime.now(timezone.utc)
        feedback = create_mock_feedback(
            feedback_id=1,
            tenant_id=tenant_id,
            undo_eligible=True,
        )
        feedback["undo_deadline"] = now - timedelta(minutes=10)  # Expired

        mock_feedback_repo.get_feedback_by_id.return_value = feedback

        with pytest.raises(UndoNotEligibleError) as exc_info:
            await feedback_manager.undo_decision(feedback_id=1)

        assert "expired" in str(exc_info.value).lower()

    @pytest.mark.asyncio
    async def test_undo_batch_operation(
        self,
        mock_batch_repo: AsyncMock,
        mock_feedback_repo: AsyncMock,
        tenant_id: UUID,
    ):
        """Test undoing an entire batch operation."""
        feedback_manager = FeedbackManager(repository=mock_feedback_repo)
        batch_manager = BatchManager(
            repository=mock_batch_repo,
            feedback_manager=feedback_manager,
        )

        batch_id = uuid4()
        now = datetime.now(timezone.utc)

        # Create batch operation that is undo eligible
        batch_op = MagicMock()
        batch_op.batch_uuid = batch_id
        batch_op.status = BatchOperationStatus.APPLIED
        batch_op.undo_eligible = True
        batch_op.undo_deadline = now + timedelta(minutes=5)

        mock_batch_repo.get_batch_operation.return_value = batch_op

        # Create items in the batch
        batch_items = [
            {"id": i, "status": ReviewItemStatus.ACCEPTED}
            for i in range(1, 6)
        ]
        mock_batch_repo.get_items_by_batch_id.return_value = batch_items
        mock_batch_repo.update_items_batch.return_value = 5

        restored_count = await batch_manager.undo_batch(batch_id=batch_id)

        assert restored_count == 5
        assert batch_op.status == BatchOperationStatus.UNDONE
        mock_batch_repo.update_batch_operation.assert_called_once()

    @pytest.mark.asyncio
    async def test_get_undo_eligible_feedback(
        self,
        mock_feedback_repo: AsyncMock,
        tenant_id: UUID,
    ):
        """Test retrieving recent undo-eligible feedback."""
        feedback_manager = FeedbackManager(repository=mock_feedback_repo)

        now = datetime.now(timezone.utc)
        eligible_feedback = [
            create_mock_feedback(
                feedback_id=i,
                tenant_id=tenant_id,
                undo_eligible=True,
            )
            for i in range(1, 4)
        ]
        for f in eligible_feedback:
            f["undo_deadline"] = now + timedelta(minutes=3)

        mock_feedback_repo.get_undo_eligible_feedback.return_value = eligible_feedback

        result = await feedback_manager.get_undo_eligible(session_id=1, count=5)

        assert len(result) == 3
        mock_feedback_repo.get_undo_eligible_feedback.assert_called_once_with(
            session_id=1, count=5
        )


# =============================================================================
# TEST CLASS: Interrupted Session
# =============================================================================


class TestInterruptedSession:
    """Integration tests for session resumption after interruption."""

    @pytest.fixture
    def mock_session_repo(self) -> AsyncMock:
        """Create mock session repository."""
        repo = AsyncMock()
        repo.get_session_by_id = AsyncMock()
        repo.get_active_session_for_user = AsyncMock()
        repo.update_session = AsyncMock()
        return repo

    @pytest.fixture
    def mock_queue_repo(self) -> AsyncMock:
        """Create mock queue repository."""
        repo = AsyncMock()
        repo.get_queue_items = AsyncMock()
        return repo

    @pytest.mark.asyncio
    async def test_resume_paused_session(
        self,
        mock_session_repo: AsyncMock,
        tenant_id: UUID,
        user_email: str,
    ):
        """Test resuming a previously paused session."""
        session_manager = SessionManager(repository=mock_session_repo)

        # Session was paused mid-review
        paused_session = create_mock_session(
            session_id=1,
            tenant_id=tenant_id,
            user_email=user_email,
            status=SessionStatus.PAUSED,
            total_items=20,
            items_reviewed=12,
            items_accepted=8,
            items_rejected=2,
            items_modified=2,
            current_position=12,
        )

        mock_session_repo.get_session_by_id.return_value = paused_session
        mock_session_repo.update_session.side_effect = lambda s: s

        resumed = await session_manager.resume_session(session_id=1)

        assert paused_session.status == SessionStatus.ACTIVE
        # All progress should be preserved
        assert paused_session.items_reviewed == 12
        assert paused_session.current_position == 12

    @pytest.mark.asyncio
    async def test_retrieve_session_with_existing_progress(
        self,
        mock_session_repo: AsyncMock,
        mock_queue_repo: AsyncMock,
        tenant_id: UUID,
        user_email: str,
    ):
        """Test retrieving existing active session and continuing work."""
        session_manager = SessionManager(repository=mock_session_repo)
        queue_manager = QueueManager(repository=mock_queue_repo)

        # Existing session with progress
        existing_session = create_mock_session(
            session_id=1,
            tenant_id=tenant_id,
            user_email=user_email,
            status=SessionStatus.ACTIVE,
            total_items=15,
            items_reviewed=7,
            current_position=7,
        )

        mock_session_repo.get_active_session_for_user.return_value = existing_session

        # Retrieve existing session
        session = await session_manager.get_active_session(tenant_id, user_email)

        assert session is not None
        assert session.id == 1
        assert session.items_reviewed == 7

        # Remaining items in queue
        remaining_items = [
            create_mock_review_item(
                item_id=i,
                tenant_id=tenant_id,
                session_id=1,
                queue_position=i,
            )
            for i in range(8, 16)
        ]

        mock_queue_repo.get_queue_items.return_value = remaining_items[:1]

        # Get next item for review
        next_item = await queue_manager.get_next_item(
            session_id=session.id,
            current_position=7,
        )

        assert next_item is not None
        assert next_item.id == 8

    @pytest.mark.asyncio
    async def test_cannot_resume_completed_session(
        self,
        mock_session_repo: AsyncMock,
    ):
        """Test that completed sessions cannot be resumed."""
        session_manager = SessionManager(repository=mock_session_repo)

        completed_session = create_mock_session(
            session_id=1,
            status=SessionStatus.COMPLETED,
        )
        completed_session.completed_at = datetime.now(timezone.utc)

        mock_session_repo.get_session_by_id.return_value = completed_session

        with pytest.raises(InvalidSessionStateError) as exc_info:
            await session_manager.resume_session(session_id=1)

        assert "completed" in str(exc_info.value).lower()

    @pytest.mark.asyncio
    async def test_cannot_resume_abandoned_session(
        self,
        mock_session_repo: AsyncMock,
    ):
        """Test that abandoned sessions cannot be resumed."""
        session_manager = SessionManager(repository=mock_session_repo)

        abandoned_session = create_mock_session(
            session_id=1,
            status=SessionStatus.ABANDONED,
        )

        mock_session_repo.get_session_by_id.return_value = abandoned_session

        with pytest.raises(InvalidSessionStateError) as exc_info:
            await session_manager.resume_session(session_id=1)

        assert "abandoned" in str(exc_info.value).lower()


# =============================================================================
# TEST CLASS: Queue Priority Modes
# =============================================================================


class TestQueuePriorityModes:
    """Integration tests for queue prioritization."""

    @pytest.fixture
    def mock_queue_repo(self) -> AsyncMock:
        """Create mock queue repository."""
        repo = AsyncMock()
        repo.get_pending_items_for_queue = AsyncMock()
        repo.get_queue_items = AsyncMock()
        repo.update_item_queue_position = AsyncMock()
        return repo

    @pytest.mark.asyncio
    async def test_confidence_priority_mode(
        self,
        mock_queue_repo: AsyncMock,
        tenant_id: UUID,
    ):
        """Test items sorted by AI confidence (lowest first)."""
        queue_manager = QueueManager(repository=mock_queue_repo)

        # Create items with varying confidence
        items = [
            create_mock_review_item(
                item_id=1, tenant_id=tenant_id, ai_confidence=Decimal("0.90")
            ),
            create_mock_review_item(
                item_id=2, tenant_id=tenant_id, ai_confidence=Decimal("0.50")
            ),
            create_mock_review_item(
                item_id=3, tenant_id=tenant_id, ai_confidence=Decimal("0.70")
            ),
        ]

        mock_queue_repo.get_pending_items_for_queue.return_value = items

        queue_items = await queue_manager.populate_queue(
            tenant_id=tenant_id,
            session_id=1,
            priority_mode=PriorityMode.CONFIDENCE,
        )

        # Lowest confidence should be first (higher priority score)
        assert queue_items[0].ai_confidence == Decimal("0.50")
        assert queue_items[1].ai_confidence == Decimal("0.70")
        assert queue_items[2].ai_confidence == Decimal("0.90")

    @pytest.mark.asyncio
    async def test_importance_priority_mode(
        self,
        mock_queue_repo: AsyncMock,
        tenant_id: UUID,
    ):
        """Test items sorted by business importance (highest first)."""
        queue_manager = QueueManager(repository=mock_queue_repo)

        items = [
            create_mock_review_item(
                item_id=1, tenant_id=tenant_id, business_importance=3
            ),
            create_mock_review_item(
                item_id=2, tenant_id=tenant_id, business_importance=9
            ),
            create_mock_review_item(
                item_id=3, tenant_id=tenant_id, business_importance=5
            ),
        ]

        mock_queue_repo.get_pending_items_for_queue.return_value = items

        queue_items = await queue_manager.populate_queue(
            tenant_id=tenant_id,
            session_id=1,
            priority_mode=PriorityMode.IMPORTANCE,
        )

        # Highest importance should be first
        assert queue_items[0].business_importance == 9
        assert queue_items[1].business_importance == 5
        assert queue_items[2].business_importance == 3

    @pytest.mark.asyncio
    async def test_reorder_queue_changes_positions(
        self,
        mock_queue_repo: AsyncMock,
        tenant_id: UUID,
    ):
        """Test that reorder_queue updates queue positions."""
        queue_manager = QueueManager(repository=mock_queue_repo)

        # Items currently ordered by confidence
        items = [
            create_mock_review_item(
                item_id=1,
                tenant_id=tenant_id,
                ai_confidence=Decimal("0.50"),
                business_importance=3,
                queue_position=1,
            ),
            create_mock_review_item(
                item_id=2,
                tenant_id=tenant_id,
                ai_confidence=Decimal("0.90"),
                business_importance=9,
                queue_position=2,
            ),
        ]

        mock_queue_repo.get_queue_items.return_value = items

        # Reorder by importance
        reordered = await queue_manager.reorder_queue(
            session_id=1,
            priority_mode=PriorityMode.IMPORTANCE,
        )

        # Item with importance 9 should now be first
        assert reordered[0].business_importance == 9
        assert reordered[0].queue_position == 1
        assert reordered[1].business_importance == 3
        assert reordered[1].queue_position == 2


# =============================================================================
# TEST CLASS: Learning Signal Quality
# =============================================================================


class TestLearningSignalQuality:
    """Integration tests for learning signal quality calculation."""

    @pytest.fixture
    def mock_feedback_repo(self) -> AsyncMock:
        """Create mock feedback repository."""
        repo = AsyncMock()
        repo.create_feedback = AsyncMock()
        return repo

    @pytest.mark.asyncio
    async def test_high_confidence_correction_high_signal(
        self,
        mock_feedback_repo: AsyncMock,
        tenant_id: UUID,
    ):
        """Test that correcting high-confidence AI produces high signal."""
        feedback_manager = FeedbackManager(repository=mock_feedback_repo)

        # Item with high AI confidence
        item = create_mock_review_item(
            item_id=1,
            tenant_id=tenant_id,
            ai_confidence=Decimal("0.95"),
        )

        # Calculate signal quality for reject decision
        quality = feedback_manager.calculate_learning_signal_quality(
            decision=DecisionType.REJECT,
            confidence=item.ai_confidence,
            time_spent_ms=5000,  # Reasonable time
        )

        # Should be high: base 0.5 * 1.5 (high conf correction) = 0.75
        assert quality >= Decimal("0.7")

    @pytest.mark.asyncio
    async def test_quick_accept_low_signal(
        self,
        mock_feedback_repo: AsyncMock,
        tenant_id: UUID,
    ):
        """Test that quick accept of low confidence has low signal."""
        feedback_manager = FeedbackManager(repository=mock_feedback_repo)

        # Calculate signal for quick accept of low confidence
        quality = feedback_manager.calculate_learning_signal_quality(
            decision=DecisionType.ACCEPT,
            confidence=Decimal("0.40"),  # Low confidence
            time_spent_ms=500,  # Very quick
        )

        # Should be low: base 0.3 * 0.5 (low conf) * 0.7 (quick) = 0.105
        assert quality <= Decimal("0.15")

    @pytest.mark.asyncio
    async def test_thoughtful_modify_high_signal(
        self,
        mock_feedback_repo: AsyncMock,
        tenant_id: UUID,
    ):
        """Test that thoughtful modify decision has high signal."""
        feedback_manager = FeedbackManager(repository=mock_feedback_repo)

        quality = feedback_manager.calculate_learning_signal_quality(
            decision=DecisionType.MODIFY,
            confidence=Decimal("0.85"),  # High confidence
            time_spent_ms=15000,  # Thoughtful time (>10s)
        )

        # Should be very high: base 0.8 * 1.5 (high conf) * 1.2 (thoughtful)
        # = 1.44, clamped to 1.0
        assert quality == Decimal("1.0")


# =============================================================================
# TEST CLASS: Filter Application
# =============================================================================


class TestFilterApplication:
    """Integration tests for queue filtering."""

    @pytest.fixture
    def mock_queue_repo(self) -> AsyncMock:
        """Create mock queue repository."""
        repo = AsyncMock()
        repo.get_pending_items_for_queue = AsyncMock()
        return repo

    @pytest.mark.asyncio
    async def test_filter_by_content_type(
        self,
        mock_queue_repo: AsyncMock,
        tenant_id: UUID,
    ):
        """Test filtering queue by content type."""
        queue_manager = QueueManager(repository=mock_queue_repo)

        items = [
            create_mock_review_item(
                item_id=1, tenant_id=tenant_id, content_type=ContentType.EMAIL
            ),
            create_mock_review_item(
                item_id=2, tenant_id=tenant_id, content_type=ContentType.MEETING
            ),
            create_mock_review_item(
                item_id=3, tenant_id=tenant_id, content_type=ContentType.EMAIL
            ),
        ]

        mock_queue_repo.get_pending_items_for_queue.return_value = items

        queue_items = await queue_manager.populate_queue(
            tenant_id=tenant_id,
            session_id=1,
            filter_criteria={"content_type": "email"},
        )

        # Only email items should be returned
        assert len(queue_items) == 2
        assert all(item.content_type == ContentType.EMAIL for item in queue_items)

    @pytest.mark.asyncio
    async def test_filter_by_confidence_range(
        self,
        mock_queue_repo: AsyncMock,
        tenant_id: UUID,
    ):
        """Test filtering queue by confidence score range."""
        queue_manager = QueueManager(repository=mock_queue_repo)

        items = [
            create_mock_review_item(
                item_id=1, tenant_id=tenant_id, ai_confidence=Decimal("0.30")
            ),
            create_mock_review_item(
                item_id=2, tenant_id=tenant_id, ai_confidence=Decimal("0.60")
            ),
            create_mock_review_item(
                item_id=3, tenant_id=tenant_id, ai_confidence=Decimal("0.90")
            ),
        ]

        mock_queue_repo.get_pending_items_for_queue.return_value = items

        queue_items = await queue_manager.populate_queue(
            tenant_id=tenant_id,
            session_id=1,
            filter_criteria={
                "min_confidence": 0.5,
                "max_confidence": 0.8,
            },
        )

        # Only item with confidence 0.60 should match
        assert len(queue_items) == 1
        assert queue_items[0].ai_confidence == Decimal("0.60")

    @pytest.mark.asyncio
    async def test_filter_by_minimum_importance(
        self,
        mock_queue_repo: AsyncMock,
        tenant_id: UUID,
    ):
        """Test filtering queue by minimum business importance."""
        queue_manager = QueueManager(repository=mock_queue_repo)

        items = [
            create_mock_review_item(
                item_id=1, tenant_id=tenant_id, business_importance=3
            ),
            create_mock_review_item(
                item_id=2, tenant_id=tenant_id, business_importance=7
            ),
            create_mock_review_item(
                item_id=3, tenant_id=tenant_id, business_importance=9
            ),
        ]

        mock_queue_repo.get_pending_items_for_queue.return_value = items

        queue_items = await queue_manager.populate_queue(
            tenant_id=tenant_id,
            session_id=1,
            filter_criteria={"min_importance": 5},
        )

        # Only items with importance >= 5
        assert len(queue_items) == 2
        assert all(item.business_importance >= 5 for item in queue_items)


# =============================================================================
# TEST CLASS: End-to-End Scenario
# =============================================================================


class TestEndToEndScenario:
    """Complete end-to-end scenario tests."""

    @pytest.mark.asyncio
    async def test_daily_review_workflow_scenario(self):
        """
        Scenario: User performs daily review session

        Given: 10 pending items from email processing
        When: User starts review, processes items, pauses, resumes, completes
        Then: All decisions recorded, session completed, analytics available
        """
        # Setup all mocks
        session_repo = AsyncMock()
        queue_repo = AsyncMock()
        feedback_repo = AsyncMock()

        tenant_id = uuid4()
        user_email = "daily.reviewer@company.com"

        # Create managers
        session_manager = SessionManager(repository=session_repo)
        queue_manager = QueueManager(repository=queue_repo)
        feedback_manager = FeedbackManager(repository=feedback_repo)

        # Step 1: Check for existing session (none exists)
        session_repo.get_active_session_for_user.return_value = None

        session = await session_manager.get_active_session(tenant_id, user_email)
        assert session is None

        # Step 2: Create new session
        mock_session = create_mock_session(
            session_id=1,
            tenant_id=tenant_id,
            user_email=user_email,
            total_items=10,
        )
        session_repo.create_session.return_value = mock_session

        session = await session_manager.create_session(
            tenant_id=tenant_id,
            user_email=user_email,
            total_items=10,
            mode=ReviewMode.STANDARD,
        )

        assert session.status == SessionStatus.ACTIVE
        assert session.total_items == 10

        # Step 3: Populate queue
        items = [
            create_mock_review_item(
                item_id=i,
                tenant_id=tenant_id,
                session_id=1,
                ai_confidence=Decimal(str(0.5 + i * 0.05)),
            )
            for i in range(1, 11)
        ]
        queue_repo.get_pending_items_for_queue.return_value = items

        queue_items = await queue_manager.populate_queue(
            tenant_id=tenant_id,
            session_id=session.id,
            priority_mode=PriorityMode.MIXED,
        )

        assert len(queue_items) == 10

        # Step 4: Review first 5 items
        session_repo.get_session_by_id.return_value = mock_session
        session_repo.update_session.side_effect = lambda s: s
        feedback_repo.create_feedback.return_value = create_mock_feedback()

        # Process decisions - the update_progress method increments counters
        decisions = ["accept", "accept", "accept", "reject", "modify"]
        for i, decision in enumerate(decisions):
            if decision == "accept":
                await feedback_manager.record_accept(
                    item=items[i], session_id=session.id, time_spent_ms=3000
                )
            elif decision == "reject":
                await feedback_manager.record_reject(
                    item=items[i], session_id=session.id, time_spent_ms=2000
                )
            else:
                correction = UserCorrection(category="work/modified")
                await feedback_manager.record_modify(
                    item=items[i],
                    session_id=session.id,
                    correction=correction,
                    time_spent_ms=8000,
                )

            await session_manager.update_progress(
                session_id=session.id,
                position=i + 1,
                decision_type=decision,
            )

        # Verify first 5 items reviewed
        assert mock_session.items_reviewed == 5
        assert mock_session.items_accepted == 3
        assert mock_session.items_rejected == 1
        assert mock_session.items_modified == 1

        # Step 5: Pause session
        await session_manager.pause_session(session_id=session.id)
        assert mock_session.status == SessionStatus.PAUSED

        # Step 6: Resume session
        await session_manager.resume_session(session_id=session.id)
        assert mock_session.status == SessionStatus.ACTIVE

        # Step 7: Complete remaining items (all accepts)
        for i in range(5, 10):
            await feedback_manager.record_accept(
                item=items[i], session_id=session.id, time_spent_ms=2500
            )
            await session_manager.update_progress(
                session_id=session.id,
                position=i + 1,
                decision_type="accept",
            )

        # Step 8: Complete session
        # All items reviewed - counters should be at final values
        assert mock_session.items_reviewed == 10
        await session_manager.complete_session(session_id=session.id)

        assert mock_session.status == SessionStatus.COMPLETED
        assert mock_session.completed_at is not None
