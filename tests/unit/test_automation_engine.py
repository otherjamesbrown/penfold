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
