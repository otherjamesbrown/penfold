"""Unit tests for the Automation Rules Engine.

Tests the core rule evaluation and decision-making logic for automated
content processing based on confidence scores and user-defined rules.
"""

import pytest
from datetime import datetime, timezone
from decimal import Decimal
from typing import Any, Dict
from uuid import uuid4

import pytest_asyncio


class TestAutomationEngine:
    """Tests for the core automation engine."""

    @pytest.mark.asyncio
    async def test_engine_can_be_instantiated(self):
        """Test that the automation engine can be created."""
        from penf_lib.automation.engine import AutomationEngine

        engine = AutomationEngine()
        assert engine is not None

    @pytest.mark.asyncio
    async def test_engine_evaluates_confidence_threshold(self):
        """Test that engine correctly evaluates confidence against threshold."""
        from penf_lib.automation.engine import AutomationEngine

        engine = AutomationEngine()

        # Default threshold is 0.85
        assert engine.should_auto_process(confidence=0.90, threshold=0.85) is True
        assert engine.should_auto_process(confidence=0.80, threshold=0.85) is False
        assert engine.should_auto_process(confidence=0.85, threshold=0.85) is True

    @pytest.mark.asyncio
    async def test_engine_processes_high_confidence_content(self):
        """Test that high confidence content is processed automatically."""
        from penf_lib.automation.engine import AutomationEngine

        engine = AutomationEngine()

        content = {
            "content_id": "test-123",
            "content_type": "email",
            "confidence_score": 0.92,
        }

        result = await engine.evaluate_content(content, threshold=0.85)

        assert result.decision_type == "auto_processed"
        assert result.confidence_score == 0.92

    @pytest.mark.asyncio
    async def test_engine_queues_low_confidence_content(self):
        """Test that low confidence content is queued for review."""
        from penf_lib.automation.engine import AutomationEngine

        engine = AutomationEngine()

        content = {
            "content_id": "test-456",
            "content_type": "email",
            "confidence_score": 0.70,
        }

        result = await engine.evaluate_content(content, threshold=0.85)

        assert result.decision_type == "queued_review"
        assert result.confidence_score == 0.70


class TestRuleMatching:
    """Tests for rule matching and evaluation."""

    @pytest.mark.asyncio
    async def test_rule_matches_content_type(self):
        """Test that rules can match by content type."""
        from penf_lib.automation.engine import AutomationEngine
        from penf_lib.automation.conditions import RuleCondition

        engine = AutomationEngine()

        rule_conditions = {
            "operator": "AND",
            "conditions": [
                {"field": "content_type", "operator": "equals", "value": "email"}
            ]
        }

        content = {"content_type": "email", "sender": "test@example.com"}

        assert engine.rule_matches(rule_conditions, content) is True

        content_meeting = {"content_type": "meeting", "sender": "test@example.com"}
        assert engine.rule_matches(rule_conditions, content_meeting) is False

    @pytest.mark.asyncio
    async def test_rule_matches_with_compound_conditions(self):
        """Test that rules with AND/OR conditions work correctly."""
        from penf_lib.automation.engine import AutomationEngine

        engine = AutomationEngine()

        rule_conditions = {
            "operator": "AND",
            "conditions": [
                {"field": "content_type", "operator": "equals", "value": "email"},
                {"field": "sender", "operator": "contains", "value": "@acme.com"}
            ]
        }

        content_match = {"content_type": "email", "sender": "alice@acme.com"}
        assert engine.rule_matches(rule_conditions, content_match) is True

        content_no_match = {"content_type": "email", "sender": "bob@other.com"}
        assert engine.rule_matches(rule_conditions, content_no_match) is False

    @pytest.mark.asyncio
    async def test_rule_matches_with_or_conditions(self):
        """Test that rules with OR conditions work correctly."""
        from penf_lib.automation.engine import AutomationEngine

        engine = AutomationEngine()

        rule_conditions = {
            "operator": "OR",
            "conditions": [
                {"field": "sender", "operator": "contains", "value": "@vip.com"},
                {"field": "subject", "operator": "contains", "value": "[URGENT]"}
            ]
        }

        content_vip = {"content_type": "email", "sender": "ceo@vip.com", "subject": "Regular email"}
        assert engine.rule_matches(rule_conditions, content_vip) is True

        content_urgent = {"content_type": "email", "sender": "anyone@other.com", "subject": "[URGENT] Help needed"}
        assert engine.rule_matches(rule_conditions, content_urgent) is True

        content_neither = {"content_type": "email", "sender": "anyone@other.com", "subject": "Regular"}
        assert engine.rule_matches(rule_conditions, content_neither) is False


class TestDecisionTracking:
    """Tests for automation decision tracking and audit trail."""

    @pytest.mark.asyncio
    async def test_decision_records_all_required_fields(self):
        """Test that automation decisions include all required audit fields."""
        from penf_lib.automation.engine import AutomationEngine

        engine = AutomationEngine()

        content = {
            "content_id": "test-789",
            "content_type": "email",
            "confidence_score": 0.92,
        }

        result = await engine.evaluate_content(content, threshold=0.85)

        assert result.content_id == "test-789"
        assert result.content_type == "email"
        assert result.confidence_score == 0.92
        assert result.threshold_used == 0.85
        assert result.decision_type in ["auto_processed", "queued_review", "conflict_resolved"]
        assert result.reasoning is not None


class TestRetryMechanism:
    """Tests for retry and failure handling."""

    @pytest.mark.asyncio
    async def test_retry_delays_follow_exponential_backoff(self):
        """Test that retry delays follow exponential backoff pattern."""
        from penf_lib.automation.engine import AutomationEngine

        engine = AutomationEngine()

        # Verify retry delays are 1s, 4s, 16s per spec
        assert engine.retry_delays == [1, 4, 16]

    @pytest.mark.asyncio
    async def test_max_retries_is_three(self):
        """Test that maximum retries is 3."""
        from penf_lib.automation.engine import AutomationEngine

        engine = AutomationEngine()

        assert engine.max_retries == 3


class TestConflictResolution:
    """Tests for rule conflict detection and resolution (User Story 5)."""

    @pytest.mark.asyncio
    async def test_confidence_based_conflict_resolution(self):
        """Test that higher accuracy rule wins in conflict.

        Given rule_A (accuracy 0.92) and rule_B (accuracy 0.85) match content
        Then rule_A wins based on confidence-weighted scoring
        """
        from penf_lib.automation.engine import AutomationEngine

        engine = AutomationEngine()

        rule_a = {
            "id": uuid4(),
            "name": "Rule A",
            "accuracy_rate": 0.92,
            "confidence_score": 0.90,
            "priority": 5,
            "conditions": {},
            "actions": {"action": "categorize_a"},
        }
        rule_b = {
            "id": uuid4(),
            "name": "Rule B",
            "accuracy_rate": 0.85,
            "confidence_score": 0.88,
            "priority": 5,
            "conditions": {},
            "actions": {"action": "categorize_b"},
        }

        matching_rules = [(rule_a, 0.92), (rule_b, 0.85)]
        winner = engine.resolve_conflict(matching_rules)

        assert winner["name"] == "Rule A"

    @pytest.mark.asyncio
    async def test_conflict_resolution_uses_priority_as_tiebreaker(self):
        """Test that priority breaks ties when accuracies are close.

        Given two rules with similar accuracy
        When priority differs
        Then higher priority rule (lower number) wins
        """
        from penf_lib.automation.engine import AutomationEngine

        engine = AutomationEngine()

        rule_high_priority = {
            "id": uuid4(),
            "name": "High Priority Rule",
            "accuracy_rate": 0.90,
            "confidence_score": 0.90,
            "priority": 1,  # Higher priority (lower number)
            "conditions": {},
            "actions": {"action": "high_priority_action"},
        }
        rule_low_priority = {
            "id": uuid4(),
            "name": "Low Priority Rule",
            "accuracy_rate": 0.90,
            "confidence_score": 0.90,
            "priority": 8,  # Lower priority (higher number)
            "conditions": {},
            "actions": {"action": "low_priority_action"},
        }

        matching_rules = [(rule_high_priority, 0.90), (rule_low_priority, 0.90)]
        winner = engine.resolve_conflict(matching_rules)

        assert winner["name"] == "High Priority Rule"

    @pytest.mark.asyncio
    async def test_detect_multiple_rule_matches(self):
        """Test detecting when multiple rules match content."""
        from penf_lib.automation.engine import AutomationEngine

        engine = AutomationEngine()

        rules = [
            {
                "id": uuid4(),
                "name": "Email Rule",
                "accuracy_rate": 0.90,
                "conditions": {
                    "operator": "AND",
                    "conditions": [
                        {"field": "content_type", "operator": "equals", "value": "email"}
                    ]
                },
                "actions": {"action": "archive"},
            },
            {
                "id": uuid4(),
                "name": "Vendor Rule",
                "accuracy_rate": 0.85,
                "conditions": {
                    "operator": "AND",
                    "conditions": [
                        {"field": "sender", "operator": "contains", "value": "@vendor.com"}
                    ]
                },
                "actions": {"action": "categorize"},
            },
        ]

        content = {
            "content_type": "email",
            "sender": "sales@vendor.com",
            "subject": "Invoice",
        }

        matches = await engine.evaluate_rules(content, rules)

        assert len(matches) == 2
        assert all(isinstance(m[1], float) for m in matches)

    @pytest.mark.asyncio
    async def test_evaluate_content_with_conflict(self):
        """Test full content evaluation with conflict resolution.

        Given content that matches multiple rules
        When evaluated
        Then conflict_resolved decision is returned with winning rule
        """
        from penf_lib.automation.engine import AutomationEngine

        engine = AutomationEngine()

        rules = [
            {
                "id": uuid4(),
                "name": "General Email",
                "accuracy_rate": 0.75,
                "confidence_score": 0.85,
                "priority": 5,
                "conditions": {
                    "operator": "AND",
                    "conditions": [
                        {"field": "content_type", "operator": "equals", "value": "email"}
                    ]
                },
                "actions": {"action": "archive"},
            },
            {
                "id": uuid4(),
                "name": "VIP Sender",
                "accuracy_rate": 0.95,
                "confidence_score": 0.90,
                "priority": 2,
                "conditions": {
                    "operator": "AND",
                    "conditions": [
                        {"field": "sender", "operator": "contains", "value": "@vip.com"}
                    ]
                },
                "actions": {"action": "prioritize"},
            },
        ]

        content = {
            "content_id": "email-001",
            "content_type": "email",
            "sender": "ceo@vip.com",
            "confidence_score": 0.92,
        }

        result = await engine.evaluate_content_with_rules(content, rules, threshold=0.85)

        assert result.decision_type == "conflict_resolved"
        assert result.rule_id is not None
        assert "VIP Sender" in result.reasoning or result.actions_taken.get("action") == "prioritize"

    @pytest.mark.asyncio
    async def test_conflict_detection_creates_record(self):
        """Test that conflicts are recorded for tracking."""
        from penf_lib.automation.engine import AutomationEngine, ConflictRecord

        engine = AutomationEngine()

        rule_a = {"id": uuid4(), "name": "Rule A", "accuracy_rate": 0.90}
        rule_b = {"id": uuid4(), "name": "Rule B", "accuracy_rate": 0.85}

        matching_rules = [(rule_a, 0.90), (rule_b, 0.85)]

        conflict_record = engine.create_conflict_record(
            matching_rules=matching_rules,
            winning_rule=rule_a,
            content_id="test-content-001",
        )

        assert conflict_record is not None
        assert conflict_record.winning_rule_id == rule_a["id"]
        assert len(conflict_record.conflicting_rule_ids) == 2
        assert conflict_record.resolution_method == "confidence_weighted"

    @pytest.mark.asyncio
    async def test_low_confidence_conflict_flags_notification(self):
        """Test that low-confidence conflicts flag for notification.

        Given confidence tie or very close scores between rules
        When conflict is resolved
        Then user notification is flagged
        """
        from penf_lib.automation.engine import AutomationEngine

        engine = AutomationEngine()

        # Two rules with nearly identical scores
        rule_a = {
            "id": uuid4(),
            "name": "Rule A",
            "accuracy_rate": 0.88,
            "confidence_score": 0.88,
            "priority": 5,
        }
        rule_b = {
            "id": uuid4(),
            "name": "Rule B",
            "accuracy_rate": 0.87,
            "confidence_score": 0.89,
            "priority": 5,
        }

        matching_rules = [(rule_a, 0.88), (rule_b, 0.87)]

        conflict_record = engine.create_conflict_record(
            matching_rules=matching_rules,
            winning_rule=rule_a,
            content_id="test-content-002",
        )

        assert conflict_record.needs_notification is True
        assert conflict_record.confidence_margin < 0.05

    @pytest.mark.asyncio
    async def test_conflict_prediction_at_rule_creation(self):
        """Test predicting potential conflicts before rule activation.

        Given new rule creation
        When potential conflicts exist
        Then conflicts are highlighted before activation
        """
        from penf_lib.automation.engine import AutomationEngine

        engine = AutomationEngine()

        existing_rules = [
            {
                "id": uuid4(),
                "name": "Existing Email Rule",
                "conditions": {
                    "operator": "AND",
                    "conditions": [
                        {"field": "content_type", "operator": "equals", "value": "email"},
                        {"field": "sender", "operator": "contains", "value": "@vendor.com"}
                    ]
                },
            }
        ]

        new_rule_conditions = {
            "operator": "AND",
            "conditions": [
                {"field": "content_type", "operator": "equals", "value": "email"},
                {"field": "sender", "operator": "contains", "value": "@"}
            ]
        }

        potential_conflicts = engine.predict_conflicts(new_rule_conditions, existing_rules)

        # The new rule with broader sender condition overlaps with existing
        assert len(potential_conflicts) >= 1
        assert any(r["name"] == "Existing Email Rule" for r in potential_conflicts)

    @pytest.mark.asyncio
    async def test_no_conflict_with_single_rule(self):
        """Test that single rule match doesn't trigger conflict."""
        from penf_lib.automation.engine import AutomationEngine

        engine = AutomationEngine()

        rule = {
            "id": uuid4(),
            "name": "Only Rule",
            "accuracy_rate": 0.90,
            "conditions": {},
            "actions": {"action": "process"},
        }

        matching_rules = [(rule, 0.90)]
        winner = engine.resolve_conflict(matching_rules)

        assert winner["name"] == "Only Rule"

    @pytest.mark.asyncio
    async def test_no_conflict_with_empty_rules(self):
        """Test that empty rule list returns None."""
        from penf_lib.automation.engine import AutomationEngine

        engine = AutomationEngine()

        winner = engine.resolve_conflict([])

        assert winner is None
