"""Unit tests for RuleManager.

Tests the learning rules module that manages automated decision patterns
derived from user feedback in the daily review workflow. Includes tests for
CRUD operations, rule application, effectiveness tracking, and pending suggestions.
"""

from datetime import datetime, timedelta, timezone
from decimal import Decimal
from unittest.mock import AsyncMock, MagicMock, patch
from uuid import uuid4

import pytest

from penf_lib.review.models import (
    AISuggestion,
    ContentType,
    LearningRuleDTO,
    LearningRuleStatus,
    LearningRuleType,
    ReviewItemDTO,
    ReviewItemStatus,
)
from penf_lib.review.rules import RuleApplicationResult, RuleManager, RuleSuggestion
from penf_lib.review.exceptions import ItemNotFoundError, InvalidSessionStateError


# =============================================================================
# FIXTURES
# =============================================================================


@pytest.fixture
def tenant_id():
    """Test tenant ID."""
    return uuid4()


@pytest.fixture
def user_email():
    """Test user email."""
    return "reviewer@example.com"


@pytest.fixture
def mock_repository():
    """Create mock repository for testing."""
    repo = AsyncMock()
    repo.get_rules_for_tenant = AsyncMock()
    repo.get_rule_by_id = AsyncMock()
    repo.create_rule = AsyncMock()
    repo.update_rule = AsyncMock()
    repo.get_pending_suggestions = AsyncMock()
    return repo


@pytest.fixture
def rule_manager(mock_repository):
    """Create RuleManager with mocked repository."""
    return RuleManager(repository=mock_repository)


def create_learning_rule(
    rule_id: int = 1,
    tenant_id=None,
    rule_type: LearningRuleType = LearningRuleType.SENDER_MATCH,
    status: LearningRuleStatus = LearningRuleStatus.ACTIVE,
    match_criteria: dict = None,
    action: dict = None,
    confidence_threshold: Decimal = Decimal("0.800"),
    priority: int = 100,
    times_applied: int = 0,
    times_overridden: int = 0,
    effectiveness_score: Decimal = None,
    name: str = "Test Rule",
    description: str = None,
    created_by: str = "reviewer@example.com",
) -> LearningRuleDTO:
    """Create a LearningRuleDTO for testing."""
    if tenant_id is None:
        tenant_id = uuid4()
    if match_criteria is None:
        match_criteria = {"sender": "newsletter@example.com"}
    if action is None:
        action = {"category": "newsletters/tech", "decision": "accept"}

    now = datetime.now(timezone.utc)
    return LearningRuleDTO(
        id=rule_id,
        rule_uuid=uuid4(),
        tenant_id=tenant_id,
        name=name,
        description=description,
        status=status,
        rule_type=rule_type,
        match_criteria=match_criteria,
        action=action,
        confidence_threshold=confidence_threshold,
        priority=priority,
        derived_from_count=1,
        times_applied=times_applied,
        times_overridden=times_overridden,
        effectiveness_score=effectiveness_score,
        last_applied_at=None,
        expires_at=None,
        created_by=created_by,
        created_at=now,
        updated_at=now,
        is_deleted=False,
        deleted_at=None,
    )


def create_review_item(
    item_id: int = 1,
    tenant_id=None,
    session_id: int = 1,
    content_type: ContentType = ContentType.EMAIL,
    content_preview: str = "Newsletter from tech@example.com",
    category: str = "uncategorized",
    participants: list = None,
    ai_confidence: Decimal = Decimal("0.85"),
) -> ReviewItemDTO:
    """Create a ReviewItemDTO for testing rule application."""
    if tenant_id is None:
        tenant_id = uuid4()
    if participants is None:
        participants = ["tech@example.com"]

    now = datetime.now(timezone.utc)
    return ReviewItemDTO(
        id=item_id,
        tenant_id=tenant_id,
        session_id=session_id,
        source_id=100,
        queue_position=1,
        status=ReviewItemStatus.PENDING,
        content_type=content_type,
        content_preview=content_preview,
        ai_suggestion=AISuggestion(
            category=category,
            participants=participants,
            tags=["newsletter"],
        ),
        ai_confidence=ai_confidence,
        ai_model="gemini-1.5",
        business_importance=5,
        source_timestamp=now - timedelta(hours=2),
        created_at=now,
        updated_at=now,
    )


# =============================================================================
# LIST RULES TESTS
# =============================================================================


class TestListRules:
    """Tests for RuleManager.list_rules()."""

    @pytest.mark.asyncio
    async def test_list_all_rules(
        self,
        rule_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should return all rules for tenant."""
        rules = [
            create_learning_rule(rule_id=1, tenant_id=tenant_id, status=LearningRuleStatus.ACTIVE),
            create_learning_rule(rule_id=2, tenant_id=tenant_id, status=LearningRuleStatus.DISABLED),
            create_learning_rule(rule_id=3, tenant_id=tenant_id, status=LearningRuleStatus.EXPIRED),
        ]
        mock_repository.get_rules_for_tenant.return_value = rules

        result = await rule_manager.list_rules(tenant_id=tenant_id)

        assert len(result) == 3
        mock_repository.get_rules_for_tenant.assert_called_once_with(tenant_id, None)

    @pytest.mark.asyncio
    async def test_list_active_rules(
        self,
        rule_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should filter to active rules only."""
        active_rules = [
            create_learning_rule(rule_id=1, tenant_id=tenant_id, status=LearningRuleStatus.ACTIVE),
            create_learning_rule(rule_id=2, tenant_id=tenant_id, status=LearningRuleStatus.ACTIVE),
        ]
        mock_repository.get_rules_for_tenant.return_value = active_rules

        result = await rule_manager.list_rules(
            tenant_id=tenant_id, status=LearningRuleStatus.ACTIVE
        )

        assert len(result) == 2
        assert all(r.status == LearningRuleStatus.ACTIVE for r in result)
        mock_repository.get_rules_for_tenant.assert_called_once_with(
            tenant_id, LearningRuleStatus.ACTIVE
        )

    @pytest.mark.asyncio
    async def test_list_disabled_rules(
        self,
        rule_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should filter to disabled rules only."""
        disabled_rules = [
            create_learning_rule(rule_id=1, tenant_id=tenant_id, status=LearningRuleStatus.DISABLED),
        ]
        mock_repository.get_rules_for_tenant.return_value = disabled_rules

        result = await rule_manager.list_rules(
            tenant_id=tenant_id, status=LearningRuleStatus.DISABLED
        )

        assert len(result) == 1
        assert result[0].status == LearningRuleStatus.DISABLED
        mock_repository.get_rules_for_tenant.assert_called_once_with(
            tenant_id, LearningRuleStatus.DISABLED
        )

    @pytest.mark.asyncio
    async def test_list_empty(
        self,
        rule_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should handle no rules gracefully."""
        mock_repository.get_rules_for_tenant.return_value = []

        result = await rule_manager.list_rules(tenant_id=tenant_id)

        assert result == []
        mock_repository.get_rules_for_tenant.assert_called_once()


# =============================================================================
# GET RULE TESTS
# =============================================================================


class TestGetRule:
    """Tests for RuleManager.get_rule()."""

    @pytest.mark.asyncio
    async def test_get_existing_rule(
        self,
        rule_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should return rule by ID."""
        rule = create_learning_rule(rule_id=42, tenant_id=tenant_id)
        mock_repository.get_rule_by_id.return_value = rule

        result = await rule_manager.get_rule(rule_id=42)

        assert result is not None
        assert result.id == 42
        mock_repository.get_rule_by_id.assert_called_once_with(42)

    @pytest.mark.asyncio
    async def test_get_nonexistent_rule(
        self,
        rule_manager,
        mock_repository: AsyncMock,
    ):
        """Should return None for missing rule."""
        mock_repository.get_rule_by_id.return_value = None

        result = await rule_manager.get_rule(rule_id=999)

        assert result is None
        mock_repository.get_rule_by_id.assert_called_once_with(999)


# =============================================================================
# CREATE RULE TESTS
# =============================================================================


class TestCreateRule:
    """Tests for RuleManager.create_rule()."""

    @pytest.mark.asyncio
    async def test_create_sender_match_rule(
        self,
        rule_manager,
        mock_repository: AsyncMock,
        tenant_id,
        user_email,
    ):
        """Should create sender match rule."""
        match_criteria = {"sender": "newsletter@example.com"}
        action = {"category": "newsletters", "decision": "accept"}
        created_rule = create_learning_rule(
            rule_id=1,
            tenant_id=tenant_id,
            rule_type=LearningRuleType.SENDER_MATCH,
            match_criteria=match_criteria,
            action=action,
            name="Newsletter Rule",
        )
        mock_repository.create_rule.return_value = created_rule

        result = await rule_manager.create_rule(
            tenant_id=tenant_id,
            name="Newsletter Rule",
            rule_type=LearningRuleType.SENDER_MATCH,
            match_criteria=match_criteria,
            action=action,
            created_by=user_email,
        )

        assert result.rule_type == LearningRuleType.SENDER_MATCH
        assert result.match_criteria == match_criteria
        mock_repository.create_rule.assert_called_once()
        call_args = mock_repository.create_rule.call_args[0][0]
        assert call_args["tenant_id"] == tenant_id
        assert call_args["rule_type"] == LearningRuleType.SENDER_MATCH

    @pytest.mark.asyncio
    async def test_create_content_pattern_rule(
        self,
        rule_manager,
        mock_repository: AsyncMock,
        tenant_id,
        user_email,
    ):
        """Should create content pattern rule."""
        match_criteria = {"pattern": r"invoice.*\d{6}", "field": "subject"}
        action = {"category": "finance/invoices", "decision": "accept"}
        created_rule = create_learning_rule(
            rule_id=2,
            tenant_id=tenant_id,
            rule_type=LearningRuleType.CONTENT_PATTERN,
            match_criteria=match_criteria,
            action=action,
        )
        mock_repository.create_rule.return_value = created_rule

        result = await rule_manager.create_rule(
            tenant_id=tenant_id,
            name="Invoice Pattern Rule",
            rule_type=LearningRuleType.CONTENT_PATTERN,
            match_criteria=match_criteria,
            action=action,
            created_by=user_email,
        )

        assert result.rule_type == LearningRuleType.CONTENT_PATTERN
        call_args = mock_repository.create_rule.call_args[0][0]
        assert call_args["match_criteria"]["pattern"] == r"invoice.*\d{6}"

    @pytest.mark.asyncio
    async def test_create_category_override_rule(
        self,
        rule_manager,
        mock_repository: AsyncMock,
        tenant_id,
        user_email,
    ):
        """Should create category override rule."""
        match_criteria = {"original_category": "general/misc"}
        action = {"category": "projects/alpha", "decision": "accept"}
        created_rule = create_learning_rule(
            rule_id=3,
            tenant_id=tenant_id,
            rule_type=LearningRuleType.CATEGORY_OVERRIDE,
            match_criteria=match_criteria,
            action=action,
        )
        mock_repository.create_rule.return_value = created_rule

        result = await rule_manager.create_rule(
            tenant_id=tenant_id,
            name="Category Override Rule",
            rule_type=LearningRuleType.CATEGORY_OVERRIDE,
            match_criteria=match_criteria,
            action=action,
            created_by=user_email,
        )

        assert result.rule_type == LearningRuleType.CATEGORY_OVERRIDE

    @pytest.mark.asyncio
    async def test_create_with_custom_threshold(
        self,
        rule_manager,
        mock_repository: AsyncMock,
        tenant_id,
        user_email,
    ):
        """Should respect custom confidence threshold."""
        custom_threshold = Decimal("0.950")
        created_rule = create_learning_rule(
            rule_id=4,
            tenant_id=tenant_id,
            confidence_threshold=custom_threshold,
        )
        mock_repository.create_rule.return_value = created_rule

        result = await rule_manager.create_rule(
            tenant_id=tenant_id,
            name="High Confidence Rule",
            rule_type=LearningRuleType.SENDER_MATCH,
            match_criteria={"sender": "ceo@company.com"},
            action={"category": "executive", "decision": "accept"},
            created_by=user_email,
            confidence_threshold=custom_threshold,
        )

        assert result.confidence_threshold == custom_threshold
        call_args = mock_repository.create_rule.call_args[0][0]
        assert call_args["confidence_threshold"] == custom_threshold

    @pytest.mark.asyncio
    async def test_create_with_custom_priority(
        self,
        rule_manager,
        mock_repository: AsyncMock,
        tenant_id,
        user_email,
    ):
        """Should respect custom priority."""
        created_rule = create_learning_rule(
            rule_id=5,
            tenant_id=tenant_id,
            priority=10,  # Higher priority (lower number)
        )
        mock_repository.create_rule.return_value = created_rule

        result = await rule_manager.create_rule(
            tenant_id=tenant_id,
            name="High Priority Rule",
            rule_type=LearningRuleType.SENDER_MATCH,
            match_criteria={"sender": "urgent@company.com"},
            action={"category": "urgent", "decision": "accept"},
            created_by=user_email,
            priority=10,
        )

        assert result.priority == 10
        call_args = mock_repository.create_rule.call_args[0][0]
        assert call_args["priority"] == 10


# =============================================================================
# ENABLE/DISABLE TESTS
# =============================================================================


class TestEnableDisable:
    """Tests for RuleManager.enable_rule() and disable_rule()."""

    @pytest.mark.asyncio
    async def test_enable_disabled_rule(
        self,
        rule_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should change status to active."""
        disabled_rule = create_learning_rule(
            rule_id=1, tenant_id=tenant_id, status=LearningRuleStatus.DISABLED
        )
        enabled_rule = create_learning_rule(
            rule_id=1, tenant_id=tenant_id, status=LearningRuleStatus.ACTIVE
        )
        mock_repository.get_rule_by_id.return_value = disabled_rule
        mock_repository.update_rule.return_value = enabled_rule

        result = await rule_manager.enable_rule(rule_id=1)

        assert result.status == LearningRuleStatus.ACTIVE
        mock_repository.update_rule.assert_called_once()
        # The update receives the modified DTO
        call_args = mock_repository.update_rule.call_args[0][0]
        assert call_args.status == LearningRuleStatus.ACTIVE

    @pytest.mark.asyncio
    async def test_disable_active_rule(
        self,
        rule_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should change status to disabled."""
        active_rule = create_learning_rule(
            rule_id=1, tenant_id=tenant_id, status=LearningRuleStatus.ACTIVE
        )
        disabled_rule = create_learning_rule(
            rule_id=1, tenant_id=tenant_id, status=LearningRuleStatus.DISABLED
        )
        mock_repository.get_rule_by_id.return_value = active_rule
        mock_repository.update_rule.return_value = disabled_rule

        result = await rule_manager.disable_rule(rule_id=1)

        assert result.status == LearningRuleStatus.DISABLED
        mock_repository.update_rule.assert_called_once()
        call_args = mock_repository.update_rule.call_args[0][0]
        assert call_args.status == LearningRuleStatus.DISABLED

    @pytest.mark.asyncio
    async def test_enable_already_active_raises_error(
        self,
        rule_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should raise InvalidSessionStateError when rule is already active."""
        active_rule = create_learning_rule(
            rule_id=1, tenant_id=tenant_id, status=LearningRuleStatus.ACTIVE
        )
        mock_repository.get_rule_by_id.return_value = active_rule

        with pytest.raises(InvalidSessionStateError):
            await rule_manager.enable_rule(rule_id=1)

    @pytest.mark.asyncio
    async def test_disable_already_disabled_raises_error(
        self,
        rule_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should raise InvalidSessionStateError when rule is already disabled."""
        disabled_rule = create_learning_rule(
            rule_id=1, tenant_id=tenant_id, status=LearningRuleStatus.DISABLED
        )
        mock_repository.get_rule_by_id.return_value = disabled_rule

        with pytest.raises(InvalidSessionStateError):
            await rule_manager.disable_rule(rule_id=1)

    @pytest.mark.asyncio
    async def test_enable_nonexistent_rule_raises_error(
        self,
        rule_manager,
        mock_repository: AsyncMock,
    ):
        """Should raise ItemNotFoundError for nonexistent rule."""
        mock_repository.get_rule_by_id.return_value = None

        with pytest.raises(ItemNotFoundError):
            await rule_manager.enable_rule(rule_id=999)

    @pytest.mark.asyncio
    async def test_disable_nonexistent_rule_raises_error(
        self,
        rule_manager,
        mock_repository: AsyncMock,
    ):
        """Should raise ItemNotFoundError for nonexistent rule."""
        mock_repository.get_rule_by_id.return_value = None

        with pytest.raises(ItemNotFoundError):
            await rule_manager.disable_rule(rule_id=999)


# =============================================================================
# PENDING SUGGESTIONS TESTS
# =============================================================================


class TestPendingSuggestions:
    """Tests for pending rule suggestion management."""

    @pytest.mark.asyncio
    async def test_get_pending_suggestions(
        self,
        rule_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should return pending rules awaiting user approval."""
        pending_rules = [
            create_learning_rule(rule_id=1, tenant_id=tenant_id),
            create_learning_rule(rule_id=2, tenant_id=tenant_id),
        ]
        mock_repository.get_pending_suggestions.return_value = pending_rules

        result = await rule_manager.get_pending_suggestions(tenant_id=tenant_id)

        assert len(result) == 2
        mock_repository.get_pending_suggestions.assert_called_once_with(tenant_id)

    @pytest.mark.asyncio
    async def test_accept_suggestion(
        self,
        rule_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should change suggestion status to active."""
        pending_suggestion = create_learning_rule(
            rule_id=1, tenant_id=tenant_id, status=LearningRuleStatus.DISABLED
        )
        activated_rule = create_learning_rule(
            rule_id=1, tenant_id=tenant_id, status=LearningRuleStatus.ACTIVE
        )
        mock_repository.get_rule_by_id.return_value = pending_suggestion
        mock_repository.update_rule.return_value = activated_rule

        result = await rule_manager.accept_suggestion(suggestion_id=1)

        assert result.status == LearningRuleStatus.ACTIVE
        mock_repository.update_rule.assert_called_once()
        call_args = mock_repository.update_rule.call_args[0][0]
        assert call_args.status == LearningRuleStatus.ACTIVE

    @pytest.mark.asyncio
    async def test_reject_suggestion(
        self,
        rule_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should mark suggestion as rejected via soft delete."""
        pending_suggestion = create_learning_rule(
            rule_id=1, tenant_id=tenant_id, status=LearningRuleStatus.DISABLED
        )
        mock_repository.get_rule_by_id.return_value = pending_suggestion
        mock_repository.update_rule.return_value = pending_suggestion

        result = await rule_manager.reject_suggestion(suggestion_id=1)

        # reject_suggestion returns bool
        assert result is True
        mock_repository.update_rule.assert_called_once()
        call_args = mock_repository.update_rule.call_args[0][0]
        assert call_args.is_deleted is True

    @pytest.mark.asyncio
    async def test_get_pending_suggestions_empty(
        self,
        rule_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should return empty list when no pending suggestions."""
        mock_repository.get_pending_suggestions.return_value = []

        result = await rule_manager.get_pending_suggestions(tenant_id=tenant_id)

        assert result == []

    @pytest.mark.asyncio
    async def test_accept_nonexistent_suggestion_raises_error(
        self,
        rule_manager,
        mock_repository: AsyncMock,
    ):
        """Should raise ItemNotFoundError when suggestion doesn't exist."""
        mock_repository.get_rule_by_id.return_value = None

        with pytest.raises(ItemNotFoundError):
            await rule_manager.accept_suggestion(suggestion_id=999)

    @pytest.mark.asyncio
    async def test_reject_nonexistent_suggestion_raises_error(
        self,
        rule_manager,
        mock_repository: AsyncMock,
    ):
        """Should raise ItemNotFoundError when suggestion doesn't exist."""
        mock_repository.get_rule_by_id.return_value = None

        with pytest.raises(ItemNotFoundError):
            await rule_manager.reject_suggestion(suggestion_id=999)


# =============================================================================
# APPLY RULES TESTS
# =============================================================================


class TestApplyRules:
    """Tests for RuleManager.apply_rules()."""

    @pytest.mark.asyncio
    async def test_apply_matching_sender_rule_with_low_confidence(
        self,
        rule_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should apply rule when sender matches and confidence is below threshold."""
        sender_rule = create_learning_rule(
            rule_id=1,
            tenant_id=tenant_id,
            rule_type=LearningRuleType.SENDER_MATCH,
            match_criteria={"sender": "newsletter@example.com"},
            action={"category": "newsletters/tech"},
            confidence_threshold=Decimal("0.900"),  # Rule applies when AI confidence < 0.9
        )
        mock_repository.get_rules_for_tenant.return_value = [sender_rule]

        # Low confidence item - rule should apply
        item = create_review_item(
            tenant_id=tenant_id,
            participants=["newsletter@example.com"],
            ai_confidence=Decimal("0.70"),  # Below threshold
        )

        result = await rule_manager.apply_rules(tenant_id=tenant_id, item=item)

        assert result is not None
        assert isinstance(result, RuleApplicationResult)
        assert len(result.applied_rules) >= 1
        assert result.modified_suggestion.category == "newsletters/tech"

    @pytest.mark.asyncio
    async def test_apply_matching_content_rule_with_low_confidence(
        self,
        rule_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should apply rule when content matches pattern and confidence is low."""
        content_rule = create_learning_rule(
            rule_id=2,
            tenant_id=tenant_id,
            rule_type=LearningRuleType.CONTENT_PATTERN,
            match_criteria={"keywords": ["weekly report"]},
            action={"category": "reports/weekly"},
            confidence_threshold=Decimal("0.900"),
        )
        mock_repository.get_rules_for_tenant.return_value = [content_rule]

        item = create_review_item(
            tenant_id=tenant_id,
            content_preview="Here is the weekly report for Q1",
            ai_confidence=Decimal("0.70"),
        )

        result = await rule_manager.apply_rules(tenant_id=tenant_id, item=item)

        assert result is not None
        assert len(result.applied_rules) >= 1

    @pytest.mark.asyncio
    async def test_no_matching_rules(
        self,
        rule_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should return empty result when no rules match."""
        sender_rule = create_learning_rule(
            rule_id=1,
            tenant_id=tenant_id,
            match_criteria={"sender": "different@example.com"},
        )
        mock_repository.get_rules_for_tenant.return_value = [sender_rule]

        item = create_review_item(
            tenant_id=tenant_id,
            participants=["someone@example.com"],
        )

        result = await rule_manager.apply_rules(tenant_id=tenant_id, item=item)

        assert result.applied_rules == []

    @pytest.mark.asyncio
    async def test_rule_not_applied_when_confidence_too_high(
        self,
        rule_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should NOT apply rule when item confidence is above threshold."""
        # Rules only apply when AI confidence is LOW (below threshold)
        high_confidence_rule = create_learning_rule(
            rule_id=1,
            tenant_id=tenant_id,
            match_criteria={"sender": "sender@example.com"},
            action={"category": "high-conf"},
            confidence_threshold=Decimal("0.800"),
        )
        mock_repository.get_rules_for_tenant.return_value = [high_confidence_rule]

        # Item with HIGH confidence - rule should NOT apply
        item = create_review_item(
            tenant_id=tenant_id,
            participants=["sender@example.com"],
            ai_confidence=Decimal("0.90"),  # Above threshold
        )

        result = await rule_manager.apply_rules(tenant_id=tenant_id, item=item)

        # Rule should not apply because confidence is above threshold
        assert result.applied_rules == []

    @pytest.mark.asyncio
    async def test_apply_rules_with_no_active_rules(
        self,
        rule_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should handle case with no active rules."""
        mock_repository.get_rules_for_tenant.return_value = []

        item = create_review_item(tenant_id=tenant_id)

        result = await rule_manager.apply_rules(tenant_id=tenant_id, item=item)

        assert result.applied_rules == []

    @pytest.mark.asyncio
    async def test_rules_applied_in_priority_order(
        self,
        rule_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should apply rules sorted by priority."""
        # Higher priority rule (lower number)
        high_priority_rule = create_learning_rule(
            rule_id=1,
            tenant_id=tenant_id,
            rule_type=LearningRuleType.SENDER_MATCH,
            match_criteria={"sender": "vip@example.com"},
            action={"category": "vip"},
            priority=10,
            confidence_threshold=Decimal("0.900"),
        )
        # Lower priority rule (higher number)
        low_priority_rule = create_learning_rule(
            rule_id=2,
            tenant_id=tenant_id,
            rule_type=LearningRuleType.CONTENT_PATTERN,
            match_criteria={"keywords": ["urgent"]},
            action={"category": "urgent"},
            priority=100,
            confidence_threshold=Decimal("0.900"),
        )
        # Return unsorted - manager should sort by priority
        mock_repository.get_rules_for_tenant.return_value = [low_priority_rule, high_priority_rule]

        item = create_review_item(
            tenant_id=tenant_id,
            participants=["vip@example.com"],
            content_preview="urgent: please review",
            ai_confidence=Decimal("0.70"),
        )

        result = await rule_manager.apply_rules(tenant_id=tenant_id, item=item)

        # Both rules should match
        assert len(result.applied_rules) == 2
        # High priority rule should be first (priority 10 < 100)
        assert result.applied_rules[0].priority < result.applied_rules[1].priority


# =============================================================================
# EFFECTIVENESS TRACKING TESTS
# =============================================================================


class TestEffectivenessTracking:
    """Tests for rule effectiveness tracking."""

    @pytest.mark.asyncio
    async def test_record_accepted_outcome(
        self,
        rule_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should increment times_applied only when user accepts rule suggestion."""
        rule = create_learning_rule(
            rule_id=1,
            tenant_id=tenant_id,
            times_applied=5,
            times_overridden=1,
        )
        mock_repository.get_rule_by_id.return_value = rule
        mock_repository.update_rule.return_value = rule

        await rule_manager.record_rule_outcome(rule_id=1, was_overridden=False)

        mock_repository.update_rule.assert_called_once()
        call_args = mock_repository.update_rule.call_args[0][0]
        assert call_args.times_applied == 6
        assert call_args.times_overridden == 1  # Not incremented

    @pytest.mark.asyncio
    async def test_record_overridden_outcome(
        self,
        rule_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should increment both counters when user overrides rule."""
        rule = create_learning_rule(
            rule_id=1,
            tenant_id=tenant_id,
            times_applied=5,
            times_overridden=1,
        )
        mock_repository.get_rule_by_id.return_value = rule
        mock_repository.update_rule.return_value = rule

        await rule_manager.record_rule_outcome(rule_id=1, was_overridden=True)

        mock_repository.update_rule.assert_called_once()
        call_args = mock_repository.update_rule.call_args[0][0]
        assert call_args.times_applied == 6
        assert call_args.times_overridden == 2

    @pytest.mark.asyncio
    async def test_record_outcome_nonexistent_rule_raises_error(
        self,
        rule_manager,
        mock_repository: AsyncMock,
    ):
        """Should raise ItemNotFoundError when rule doesn't exist."""
        mock_repository.get_rule_by_id.return_value = None

        with pytest.raises(ItemNotFoundError):
            await rule_manager.record_rule_outcome(rule_id=999, was_overridden=False)

    def test_calculate_effectiveness_perfect(self, rule_manager):
        """100% effectiveness when never overridden."""
        effectiveness = rule_manager.calculate_effectiveness(
            times_applied=10,
            times_overridden=0,
        )

        assert effectiveness == Decimal("1.000")

    def test_calculate_effectiveness_partial(self, rule_manager):
        """Calculate correct percentage for partial effectiveness."""
        # 8 out of 10 applications were accepted (2 overridden)
        effectiveness = rule_manager.calculate_effectiveness(
            times_applied=10,
            times_overridden=2,
        )

        # (10 - 2) / 10 = 0.8 = 80%
        assert effectiveness == Decimal("0.800")

    def test_calculate_effectiveness_zero_applied(self, rule_manager):
        """Should return 1.0 when never applied (no data)."""
        effectiveness = rule_manager.calculate_effectiveness(
            times_applied=0,
            times_overridden=0,
        )

        # No applications means no failures, treat as 100%
        assert effectiveness == Decimal("1.000")

    def test_calculate_effectiveness_all_overridden(self, rule_manager):
        """Should return 0.0 when always overridden."""
        effectiveness = rule_manager.calculate_effectiveness(
            times_applied=5,
            times_overridden=5,
        )

        assert effectiveness == Decimal("0.000")

    def test_calculate_effectiveness_fractional(self, rule_manager):
        """Should handle fractional effectiveness correctly."""
        # 7 out of 10 applications were accepted
        effectiveness = rule_manager.calculate_effectiveness(
            times_applied=10,
            times_overridden=3,
        )

        assert effectiveness == Decimal("0.700")


# =============================================================================
# RULE APPLICATION RESULT TESTS
# =============================================================================


class TestRuleApplicationResult:
    """Tests for RuleApplicationResult dataclass."""

    def test_result_with_no_rules(self):
        """Should create result with empty applied rules list."""
        original = AISuggestion(category="test", participants=[], tags=[])
        result = RuleApplicationResult(
            applied_rules=[],
            original_suggestion=original,
            modified_suggestion=original,
            modifications=[],
        )

        assert result.applied_rules == []
        assert result.original_suggestion == original

    def test_result_with_single_rule(self, tenant_id):
        """Should create result with single applied rule."""
        rule = create_learning_rule(rule_id=1, tenant_id=tenant_id)
        original = AISuggestion(category="old", participants=[], tags=[])
        modified = AISuggestion(category="newsletters", participants=[], tags=[])

        result = RuleApplicationResult(
            applied_rules=[rule],
            original_suggestion=original,
            modified_suggestion=modified,
            modifications=["Category: old -> newsletters"],
        )

        assert len(result.applied_rules) == 1
        assert result.modified_suggestion.category == "newsletters"
        assert len(result.modifications) == 1

    def test_result_tracks_modifications(self, tenant_id):
        """Should track all modifications made by rules."""
        rule1 = create_learning_rule(rule_id=1, tenant_id=tenant_id)
        rule2 = create_learning_rule(rule_id=2, tenant_id=tenant_id)
        original = AISuggestion(category="old", participants=[], tags=[])
        modified = AISuggestion(category="new", participants=[], tags=["tag1"])

        result = RuleApplicationResult(
            applied_rules=[rule1, rule2],
            original_suggestion=original,
            modified_suggestion=modified,
            modifications=["Category: old -> new", "Added tags: tag1"],
        )

        assert len(result.applied_rules) == 2
        assert len(result.modifications) == 2


# =============================================================================
# RULE SUGGESTION DATACLASS TESTS
# =============================================================================


class TestRuleSuggestion:
    """Tests for RuleSuggestion dataclass."""

    def test_create_suggestion(self):
        """Should create a rule suggestion."""
        now = datetime.now(timezone.utc)
        suggestion = RuleSuggestion(
            id=1,
            rule_type=LearningRuleType.SENDER_MATCH,
            match_criteria={"sender": "test@example.com"},
            action={"category": "test"},
            pattern_count=5,
            confidence=Decimal("0.85"),
            created_at=now,
        )

        assert suggestion.id == 1
        assert suggestion.rule_type == LearningRuleType.SENDER_MATCH
        assert suggestion.pattern_count == 5
        assert suggestion.confidence == Decimal("0.85")


# =============================================================================
# RULE NAME GENERATION TESTS
# =============================================================================


class TestRuleNameGeneration:
    """Tests for auto-generated rule names."""

    @pytest.mark.asyncio
    async def test_auto_generate_sender_name(
        self,
        rule_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should auto-generate name for sender match rule."""
        created_rule = create_learning_rule(
            rule_id=1,
            tenant_id=tenant_id,
            name="Sender: newsletter@example.com",
        )
        mock_repository.create_rule.return_value = created_rule

        await rule_manager.create_rule(
            tenant_id=tenant_id,
            rule_type=LearningRuleType.SENDER_MATCH,
            match_criteria={"sender": "newsletter@example.com"},
            action={"category": "newsletters"},
        )

        call_args = mock_repository.create_rule.call_args[0][0]
        assert "sender" in call_args["name"].lower() or "newsletter" in call_args["name"].lower()

    @pytest.mark.asyncio
    async def test_auto_generate_content_pattern_name(
        self,
        rule_manager,
        mock_repository: AsyncMock,
        tenant_id,
    ):
        """Should auto-generate name for content pattern rule."""
        created_rule = create_learning_rule(
            rule_id=1,
            tenant_id=tenant_id,
            name="Keywords: invoice, budget",
        )
        mock_repository.create_rule.return_value = created_rule

        await rule_manager.create_rule(
            tenant_id=tenant_id,
            rule_type=LearningRuleType.CONTENT_PATTERN,
            match_criteria={"keywords": ["invoice", "budget"]},
            action={"category": "finance"},
        )

        call_args = mock_repository.create_rule.call_args[0][0]
        # Should contain keywords or pattern indicator
        assert "keyword" in call_args["name"].lower() or "content" in call_args["name"].lower()
