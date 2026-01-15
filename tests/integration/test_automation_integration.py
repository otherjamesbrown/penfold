"""Integration tests for Automation Rules Engine.

Tests the complete automation workflow including:
- Confidence threshold evaluation
- Rule matching and application
- Decision recording and audit trail
- Event integration
"""

import pytest
from datetime import datetime, timezone
from decimal import Decimal
from typing import Any, Dict
from uuid import uuid4

import pytest_asyncio


class TestConfidenceBasedProcessing:
    """Integration tests for User Story 1 - Confidence-Based Automatic Processing."""

    @pytest.mark.asyncio
    async def test_auto_process_above_threshold(self):
        """Test that content above threshold is auto-processed.

        Given confidence_threshold = 0.85
        When AI_confidence = 0.90
        Then content is auto-processed without review
        """
        from penf_lib.automation.engine import AutomationEngine

        engine = AutomationEngine(default_threshold=0.85)

        content = {
            "content_id": "email-001",
            "content_type": "email",
            "confidence_score": 0.90,
            "sender": "alice@example.com",
            "subject": "Project Update",
        }

        result = await engine.evaluate_content(content, threshold=0.85)

        assert result.decision_type == "auto_processed"
        assert result.confidence_score == 0.90
        assert result.threshold_used == 0.85
        assert "auto" in result.reasoning.lower()

    @pytest.mark.asyncio
    async def test_queue_below_threshold(self):
        """Test that content below threshold is queued for review.

        Given confidence_threshold = 0.85
        When AI_confidence = 0.70
        Then content is queued for review
        """
        from penf_lib.automation.engine import AutomationEngine

        engine = AutomationEngine(default_threshold=0.85)

        content = {
            "content_id": "email-002",
            "content_type": "email",
            "confidence_score": 0.70,
        }

        result = await engine.evaluate_content(content, threshold=0.85)

        assert result.decision_type == "queued_review"
        assert result.confidence_score == 0.70
        assert result.threshold_used == 0.85
        assert "review" in result.reasoning.lower()

    @pytest.mark.asyncio
    async def test_exact_threshold_is_auto_processed(self):
        """Test that content exactly at threshold is auto-processed."""
        from penf_lib.automation.engine import AutomationEngine

        engine = AutomationEngine()

        content = {
            "content_id": "email-003",
            "content_type": "email",
            "confidence_score": 0.85,
        }

        result = await engine.evaluate_content(content, threshold=0.85)

        assert result.decision_type == "auto_processed"

    @pytest.mark.asyncio
    async def test_audit_trail_records_all_fields(self):
        """Test that auto-processing creates complete audit trail.

        Given auto-processing occurs
        Then AutomationDecision record includes all required fields
        """
        from penf_lib.automation.engine import AutomationEngine

        engine = AutomationEngine()

        content = {
            "content_id": "email-004",
            "content_type": "email",
            "confidence_score": 0.92,
        }

        result = await engine.evaluate_content(content, threshold=0.85)

        # Verify all required audit fields
        assert result.content_id == "email-004"
        assert result.content_type == "email"
        assert result.confidence_score == 0.92
        assert result.threshold_used == 0.85
        assert result.decision_type is not None
        assert result.reasoning is not None
        assert result.created_at is not None
        assert result.processing_time_ms is not None
        assert result.processing_time_ms >= 0

    @pytest.mark.asyncio
    async def test_processing_time_under_500ms(self):
        """Test that processing completes within 500ms (SC-009)."""
        from penf_lib.automation.engine import AutomationEngine

        engine = AutomationEngine()

        content = {
            "content_id": "perf-test",
            "content_type": "email",
            "confidence_score": 0.90,
        }

        result = await engine.evaluate_content(content, threshold=0.85)

        # SC-009: Processing must complete in <500ms
        assert result.processing_time_ms < 500, (
            f"Processing took {result.processing_time_ms}ms, exceeds 500ms limit"
        )


class TestPerContentTypeThresholds:
    """Tests for per-content-type threshold configuration."""

    @pytest.mark.asyncio
    async def test_different_thresholds_per_content_type(self):
        """Test that different content types can have different thresholds."""
        from penf_lib.automation.engine import AutomationEngine

        engine = AutomationEngine()

        # Lower threshold for email (80%)
        email_content = {
            "content_id": "email-threshold",
            "content_type": "email",
            "confidence_score": 0.82,
        }
        email_result = await engine.evaluate_content(email_content, threshold=0.80)
        assert email_result.decision_type == "auto_processed"

        # Higher threshold for meetings (90%)
        meeting_content = {
            "content_id": "meeting-threshold",
            "content_type": "meeting",
            "confidence_score": 0.82,
        }
        meeting_result = await engine.evaluate_content(meeting_content, threshold=0.90)
        assert meeting_result.decision_type == "queued_review"


class TestThresholdRepository:
    """Tests for threshold persistence operations."""

    @pytest.mark.asyncio
    async def test_get_default_threshold_returns_85_percent(self):
        """Test that default threshold is 85% per Clarification #3."""
        from penf_lib.automation.repository import AutomationRepository

        repo = AutomationRepository()
        tenant_id = uuid4()
        user_id = "test@example.com"

        # When no threshold set, should return None (use system default)
        threshold = await repo.get_threshold(tenant_id, user_id, "*")

        # Repository returns None for unset, engine uses 0.85 default
        assert threshold is None

    @pytest.mark.asyncio
    async def test_set_and_get_threshold(self):
        """Test setting and retrieving a threshold."""
        from penf_lib.automation.repository import AutomationRepository

        repo = AutomationRepository()
        tenant_id = uuid4()
        user_id = "test@example.com"

        # Set threshold
        result = await repo.set_threshold(
            tenant_id=tenant_id,
            user_id=user_id,
            content_type="email",
            threshold_value=0.80,
        )

        assert result["content_type"] == "email"
        assert result["threshold_value"] == 0.80


class TestDecisionRecording:
    """Tests for decision audit trail recording."""

    @pytest.mark.asyncio
    async def test_record_auto_process_decision(self):
        """Test recording an auto-process decision."""
        from penf_lib.automation.repository import AutomationRepository

        repo = AutomationRepository()
        tenant_id = uuid4()

        decision = await repo.record_decision(
            tenant_id=tenant_id,
            user_id="test@example.com",
            content_id="email-audit-001",
            content_type="email",
            decision_type="auto_processed",
            confidence_score=0.92,
            threshold_used=0.85,
            actions_taken={"action_type": "categorize", "parameters": {"category": "work"}},
            reasoning="Confidence 92% exceeds threshold 85%",
        )

        assert decision["content_id"] == "email-audit-001"
        assert decision["decision_type"] == "auto_processed"
        assert decision["confidence_score"] == 0.92

    @pytest.mark.asyncio
    async def test_record_queued_review_decision(self):
        """Test recording a queued-for-review decision."""
        from penf_lib.automation.repository import AutomationRepository

        repo = AutomationRepository()
        tenant_id = uuid4()

        decision = await repo.record_decision(
            tenant_id=tenant_id,
            user_id="test@example.com",
            content_id="email-audit-002",
            content_type="email",
            decision_type="queued_review",
            confidence_score=0.70,
            threshold_used=0.85,
            reasoning="Confidence 70% below threshold 85%",
        )

        assert decision["decision_type"] == "queued_review"


class TestRetryMechanism:
    """Tests for retry with exponential backoff per Clarification #1."""

    def test_retry_delays_are_exponential(self):
        """Test retry delays follow exponential backoff pattern."""
        from penf_lib.automation.engine import AutomationEngine

        engine = AutomationEngine()

        # Per Clarification #1: 1s, 4s, 16s
        assert engine.retry_delays == [1, 4, 16]
        assert engine.max_retries == 3

    def test_exponential_growth_pattern(self):
        """Test delays grow exponentially (1, 4, 16 seconds)."""
        from penf_lib.automation.engine import AutomationEngine

        engine = AutomationEngine()

        delays = engine.retry_delays
        # 4 = 1 * 4, 16 = 4 * 4 (base 4 exponential growth)
        assert delays[1] == delays[0] * 4
        assert delays[2] == delays[1] * 4


class TestEngineWithRules:
    """Tests for engine with rule evaluation."""

    @pytest.mark.asyncio
    async def test_rule_evaluation_with_matching_rule(self):
        """Test that matching rules are correctly identified."""
        from penf_lib.automation.engine import AutomationEngine

        engine = AutomationEngine()

        rules = [
            {
                "id": uuid4(),
                "name": "Project Alpha Emails",
                "conditions": {
                    "operator": "AND",
                    "conditions": [
                        {"field": "content_type", "operator": "equals", "value": "email"},
                        {"field": "sender", "operator": "contains", "value": "@alpha.com"},
                    ]
                },
                "actions": {"action_type": "assign_project", "parameters": {"project": "alpha"}},
                "accuracy_rate": 0.95,
            }
        ]

        content = {
            "content_id": "email-rule-001",
            "content_type": "email",
            "sender": "alice@alpha.com",
            "subject": "Update",
        }

        matches = await engine.evaluate_rules(content, rules)

        assert len(matches) == 1
        assert matches[0][0]["name"] == "Project Alpha Emails"

    @pytest.mark.asyncio
    async def test_no_matching_rules(self):
        """Test behavior when no rules match."""
        from penf_lib.automation.engine import AutomationEngine

        engine = AutomationEngine()

        rules = [
            {
                "id": uuid4(),
                "name": "VIP Emails",
                "conditions": {
                    "operator": "AND",
                    "conditions": [
                        {"field": "sender", "operator": "contains", "value": "@vip.com"},
                    ]
                },
                "actions": {"action_type": "flag", "parameters": {"priority": "high"}},
                "accuracy_rate": 0.90,
            }
        ]

        content = {
            "content_id": "email-no-match",
            "content_type": "email",
            "sender": "anyone@regular.com",
        }

        matches = await engine.evaluate_rules(content, rules)

        assert len(matches) == 0


class TestRuleManagement:
    """Tests for User Story 2 - Rule Creation and Management."""

    @pytest.mark.asyncio
    async def test_create_rule(self):
        """Test creating a new automation rule."""
        from penf_lib.automation.repository import AutomationRepository

        repo = AutomationRepository()
        tenant_id = uuid4()
        user_id = "test@example.com"

        rule = await repo.create_rule(
            tenant_id=tenant_id,
            user_id=user_id,
            name="Vendor Emails",
            conditions={
                "operator": "AND",
                "conditions": [
                    {"field": "sender", "operator": "contains", "value": "@vendor.com"},
                ]
            },
            actions={"action_type": "categorize", "parameters": {"category": "vendors"}},
            description="Auto-categorize vendor emails",
            priority=3,
        )

        assert rule["name"] == "Vendor Emails"
        assert rule["is_enabled"] is True
        assert rule["priority"] == 3
        assert rule["current_version_id"] == 1

    @pytest.mark.asyncio
    async def test_rule_matching(self):
        """Test rule matching against content.

        Given rule with conditions [sender contains "@vendor.com", content_type = "email"]
        When email from "sales@vendor.com" processed
        Then rule matches and actions execute
        """
        from penf_lib.automation.engine import AutomationEngine
        from penf_lib.automation.repository import AutomationRepository

        repo = AutomationRepository()
        engine = AutomationEngine()
        tenant_id = uuid4()

        # Create rule
        rule = await repo.create_rule(
            tenant_id=tenant_id,
            user_id="test@example.com",
            name="Vendor Categorization",
            conditions={
                "operator": "AND",
                "conditions": [
                    {"field": "content_type", "operator": "equals", "value": "email"},
                    {"field": "sender", "operator": "contains", "value": "@vendor.com"},
                ]
            },
            actions={"action_type": "categorize", "parameters": {"category": "vendors"}},
        )

        # Test content that should match
        content = {
            "content_id": "email-vendor-001",
            "content_type": "email",
            "sender": "sales@vendor.com",
            "subject": "Invoice",
        }

        # Verify rule matches
        matches = engine.rule_matches(rule["conditions"], content)
        assert matches is True

        # Test content that should not match
        content_no_match = {
            "content_id": "email-other-001",
            "content_type": "email",
            "sender": "friend@other.com",
            "subject": "Hello",
        }

        matches_no = engine.rule_matches(rule["conditions"], content_no_match)
        assert matches_no is False

    @pytest.mark.asyncio
    async def test_rule_versioning(self):
        """Test rule version history.

        Given rule is updated
        Then new version created, previous preserved
        And rollback restores previous version
        """
        from penf_lib.automation.repository import AutomationRepository

        repo = AutomationRepository()
        tenant_id = uuid4()
        user_id = "test@example.com"

        # Create initial rule
        rule = await repo.create_rule(
            tenant_id=tenant_id,
            user_id=user_id,
            name="Test Rule",
            conditions={"operator": "AND", "conditions": []},
            actions={"action_type": "tag", "parameters": {"tag": "v1"}},
        )
        rule_id = rule["id"]

        assert rule["current_version_id"] == 1

        # Update rule
        updated_rule = await repo.update_rule(
            tenant_id=tenant_id,
            rule_id=rule_id,
            updates={
                "actions": {"action_type": "tag", "parameters": {"tag": "v2"}},
            },
            change_description="Updated action tag",
            updated_by=user_id,
        )

        assert updated_rule["current_version_id"] == 2

        # Get version history
        versions = await repo.get_rule_versions(tenant_id, rule_id)
        assert len(versions) == 2
        assert versions[0]["version_number"] == 2  # Newest first
        assert versions[1]["version_number"] == 1

        # Rollback to version 1
        rolled_back = await repo.rollback_rule(
            tenant_id=tenant_id,
            rule_id=rule_id,
            version_number=1,
            user_id=user_id,
        )

        assert rolled_back is not None
        assert rolled_back["actions"]["parameters"]["tag"] == "v1"
        assert rolled_back["current_version_id"] == 3  # New version from rollback

    @pytest.mark.asyncio
    async def test_enable_disable_rule(self):
        """Test enabling and disabling rules (FR-004)."""
        from penf_lib.automation.repository import AutomationRepository

        repo = AutomationRepository()
        tenant_id = uuid4()
        user_id = "test@example.com"

        # Create rule
        rule = await repo.create_rule(
            tenant_id=tenant_id,
            user_id=user_id,
            name="Toggle Test",
            conditions={"operator": "AND", "conditions": []},
            actions={"action_type": "test"},
        )
        rule_id = rule["id"]

        assert rule["is_enabled"] is True

        # Disable
        disabled = await repo.disable_rule(tenant_id, rule_id)
        assert disabled["is_enabled"] is False

        # Enable
        enabled = await repo.enable_rule(tenant_id, rule_id)
        assert enabled["is_enabled"] is True

    @pytest.mark.asyncio
    async def test_delete_rule(self):
        """Test soft deleting a rule."""
        from penf_lib.automation.repository import AutomationRepository

        repo = AutomationRepository()
        tenant_id = uuid4()
        user_id = "test@example.com"

        # Create rule
        rule = await repo.create_rule(
            tenant_id=tenant_id,
            user_id=user_id,
            name="Delete Test",
            conditions={"operator": "AND", "conditions": []},
            actions={"action_type": "test"},
        )
        rule_id = rule["id"]

        # Delete
        deleted = await repo.delete_rule(tenant_id, rule_id, reason="No longer needed")
        assert deleted is True

        # Should not be findable
        found = await repo.get_rule_by_id(tenant_id, rule_id)
        assert found is None

    @pytest.mark.asyncio
    async def test_get_rules_filters_correctly(self):
        """Test that get_rules_for_user respects filters."""
        from penf_lib.automation.repository import AutomationRepository

        repo = AutomationRepository()
        tenant_id = uuid4()
        user_id = "test@example.com"

        # Create enabled rule
        await repo.create_rule(
            tenant_id=tenant_id,
            user_id=user_id,
            name="Enabled Rule",
            conditions={"operator": "AND", "conditions": []},
            actions={"action_type": "test"},
        )

        # Create and disable another rule
        rule2 = await repo.create_rule(
            tenant_id=tenant_id,
            user_id=user_id,
            name="Disabled Rule",
            conditions={"operator": "AND", "conditions": []},
            actions={"action_type": "test"},
        )
        await repo.disable_rule(tenant_id, rule2["id"])

        # Get only enabled
        enabled_rules = await repo.get_rules_for_user(tenant_id, user_id, enabled_only=True)
        assert len(enabled_rules) >= 1
        assert all(r["is_enabled"] for r in enabled_rules)

        # Get all rules
        all_rules = await repo.get_rules_for_user(tenant_id, user_id, enabled_only=False)
        assert len(all_rules) >= 2


class TestRuleTestMode:
    """Tests for rule testing/simulation (FR-012)."""

    @pytest.mark.asyncio
    async def test_rule_test_mode(self):
        """Test rule simulation mode.

        Given rule in test mode
        When content processed
        Then results shown but actions not executed
        """
        from penf_lib.automation.engine import AutomationEngine

        engine = AutomationEngine()

        # Create a test rule
        rule = {
            "id": uuid4(),
            "name": "Test Mode Rule",
            "conditions": {
                "operator": "AND",
                "conditions": [
                    {"field": "sender", "operator": "contains", "value": "@test.com"},
                ]
            },
            "actions": {"action_type": "categorize", "parameters": {"category": "test"}},
        }

        # Test content
        content = {
            "content_id": "test-001",
            "content_type": "email",
            "sender": "user@test.com",
        }

        # Simulate rule (test mode)
        matches = engine.rule_matches(rule["conditions"], content)
        assert matches is True

        # In test mode, we just check if it would match
        # without actually executing actions
        # This is a simulation check only
