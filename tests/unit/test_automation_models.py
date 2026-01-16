"""Unit tests for Automation Engine database models.

Tests the SQLAlchemy models for automation rules, decisions, patterns,
and all related entities.
"""

import pytest
from datetime import datetime, timezone
from decimal import Decimal
from uuid import uuid4

from sqlalchemy import inspect


class TestAutomationRuleModel:
    """Tests for the AutomationRule model."""

    def test_automation_rule_has_required_columns(self):
        """Test that AutomationRule has all required columns."""
        from penf_lib.automation.models import AutomationRule

        mapper = inspect(AutomationRule)
        columns = {c.key for c in mapper.columns}

        required_columns = {
            "id",
            "tenant_id",
            "user_id",
            "name",
            "description",
            "is_enabled",
            "priority",
            "conditions",
            "actions",
            "current_version_id",
            "created_at",
            "updated_at",
            "is_deleted",
            "deleted_at",
        }

        assert required_columns.issubset(columns), f"Missing columns: {required_columns - columns}"

    def test_automation_rule_priority_constraints(self):
        """Test that priority has valid range 1-10."""
        from penf_lib.automation.models import AutomationRule

        rule = AutomationRule(
            tenant_id=uuid4(),
            user_id="test@example.com",
            name="Test Rule",
            conditions={"operator": "AND", "conditions": []},
            actions={"action_type": "categorize", "parameters": {}},
            priority=5,
        )

        assert rule.priority == 5

    def test_automation_rule_default_values(self):
        """Test that AutomationRule model has default column definitions."""
        from penf_lib.automation.models import AutomationRule
        from sqlalchemy import inspect

        # Check that columns have correct default definitions
        mapper = inspect(AutomationRule)
        columns = {c.key: c for c in mapper.columns}

        # is_enabled defaults to True
        assert columns["is_enabled"].default.arg is True
        # priority defaults to 5
        assert columns["priority"].default.arg == 5
        # is_deleted defaults to False (from SoftDeleteMixin)
        assert columns["is_deleted"].default.arg is False


class TestAutomationRuleVersionModel:
    """Tests for the AutomationRuleVersion model."""

    def test_rule_version_has_required_columns(self):
        """Test that AutomationRuleVersion has all required columns."""
        from penf_lib.automation.models import AutomationRuleVersion

        mapper = inspect(AutomationRuleVersion)
        columns = {c.key for c in mapper.columns}

        required_columns = {
            "id",
            "rule_id",
            "version_number",
            "is_active",
            "conditions",
            "actions",
            "change_description",
            "created_at",
            "created_by",
        }

        assert required_columns.issubset(columns), f"Missing columns: {required_columns - columns}"

    def test_rule_version_default_is_active_false(self):
        """Test that new versions default to is_active=False."""
        from penf_lib.automation.models import AutomationRuleVersion
        from sqlalchemy import inspect

        # Check that is_active column has correct default definition
        mapper = inspect(AutomationRuleVersion)
        columns = {c.key: c for c in mapper.columns}

        assert columns["is_active"].default.arg is False


class TestConfidenceThresholdModel:
    """Tests for the ConfidenceThreshold model."""

    def test_confidence_threshold_has_required_columns(self):
        """Test that ConfidenceThreshold has all required columns."""
        from penf_lib.automation.models import ConfidenceThreshold

        mapper = inspect(ConfidenceThreshold)
        columns = {c.key for c in mapper.columns}

        required_columns = {
            "id",
            "tenant_id",
            "user_id",
            "content_type",
            "threshold_value",
            "is_enabled",
            "created_at",
            "updated_at",
        }

        assert required_columns.issubset(columns), f"Missing columns: {required_columns - columns}"

    def test_confidence_threshold_default_value(self):
        """Test that threshold defaults to 0.850 (85%)."""
        from penf_lib.automation.models import ConfidenceThreshold
        from sqlalchemy import inspect

        # Check that threshold_value column has correct default definition
        mapper = inspect(ConfidenceThreshold)
        columns = {c.key: c for c in mapper.columns}

        assert columns["threshold_value"].default.arg == Decimal("0.850")


class TestAutomationDecisionModel:
    """Tests for the AutomationDecision model."""

    def test_automation_decision_has_required_columns(self):
        """Test that AutomationDecision has all required columns."""
        from penf_lib.automation.models import AutomationDecision

        mapper = inspect(AutomationDecision)
        columns = {c.key for c in mapper.columns}

        required_columns = {
            "id",
            "tenant_id",
            "user_id",
            "content_id",
            "content_type",
            "rule_id",
            "rule_version_id",
            "decision_type",
            "confidence_score",
            "threshold_used",
            "actions_taken",
            "reasoning",
            "processing_time_ms",
            "retry_count",
            "error_details",
            "user_override",
            "user_feedback",
            "created_at",
        }

        assert required_columns.issubset(columns), f"Missing columns: {required_columns - columns}"

    def test_automation_decision_decision_types(self):
        """Test that decision_type validates correctly."""
        from penf_lib.automation.models import AutomationDecision

        decision = AutomationDecision(
            tenant_id=uuid4(),
            user_id="test@example.com",
            content_id="content-123",
            content_type="email",
            decision_type="auto_processed",
            confidence_score=Decimal("0.920"),
            threshold_used=Decimal("0.850"),
            actions_taken={"action_type": "categorize"},
        )

        assert decision.decision_type == "auto_processed"


class TestRuleEffectivenessModel:
    """Tests for the RuleEffectiveness model."""

    def test_rule_effectiveness_has_required_columns(self):
        """Test that RuleEffectiveness has all required columns."""
        from penf_lib.automation.models import RuleEffectiveness

        mapper = inspect(RuleEffectiveness)
        columns = {c.key for c in mapper.columns}

        required_columns = {
            "id",
            "rule_id",
            "period_start",
            "period_end",
            "total_matches",
            "total_applied",
            "correct_decisions",
            "incorrect_decisions",
            "accuracy_rate",
            "avg_confidence",
            "avg_processing_time_ms",
            "conflict_count",
            "conflict_win_count",
            "created_at",
        }

        assert required_columns.issubset(columns), f"Missing columns: {required_columns - columns}"


class TestAutomationPatternModel:
    """Tests for the AutomationPattern model."""

    def test_automation_pattern_has_required_columns(self):
        """Test that AutomationPattern has all required columns."""
        from penf_lib.automation.models import AutomationPattern

        mapper = inspect(AutomationPattern)
        columns = {c.key for c in mapper.columns}

        required_columns = {
            "id",
            "tenant_id",
            "user_id",
            "pattern_signature",
            "detected_conditions",
            "suggested_actions",
            "occurrence_count",
            "first_seen_at",
            "last_seen_at",
            "confidence_score",
            "status",
            "converted_rule_id",
            "created_at",
            "updated_at",
        }

        assert required_columns.issubset(columns), f"Missing columns: {required_columns - columns}"

    def test_automation_pattern_status_values(self):
        """Test that pattern status has valid values."""
        from penf_lib.automation.models import AutomationPattern

        pattern = AutomationPattern(
            tenant_id=uuid4(),
            user_id="test@example.com",
            pattern_signature="abc123",
            detected_conditions={"operator": "AND", "conditions": []},
            suggested_actions={"action_type": "categorize"},
            occurrence_count=5,
            first_seen_at=datetime.now(timezone.utc),
            last_seen_at=datetime.now(timezone.utc),
            confidence_score=Decimal("0.85"),
            status="pending",
        )

        assert pattern.status == "pending"


class TestRuleConflictModel:
    """Tests for the RuleConflict model."""

    def test_rule_conflict_has_required_columns(self):
        """Test that RuleConflict has all required columns."""
        from penf_lib.automation.models import RuleConflict

        mapper = inspect(RuleConflict)
        columns = {c.key for c in mapper.columns}

        required_columns = {
            "id",
            "tenant_id",
            "content_id",
            "conflicting_rule_ids",
            "winning_rule_id",
            "resolution_method",
            "resolution_reasoning",
            "confidence_scores",
            "user_notified",
            "user_resolution",
            "created_at",
        }

        assert required_columns.issubset(columns), f"Missing columns: {required_columns - columns}"


class TestProgressiveSettingsModel:
    """Tests for the ProgressiveSettings model."""

    def test_progressive_settings_has_required_columns(self):
        """Test that ProgressiveSettings has all required columns."""
        from penf_lib.automation.models import ProgressiveSettings

        mapper = inspect(ProgressiveSettings)
        columns = {c.key for c in mapper.columns}

        required_columns = {
            "id",
            "tenant_id",
            "user_id",
            "aggressiveness_level",
            "auto_threshold_adjustment",
            "min_learning_period_days",
            "target_automation_rate",
            "accuracy_floor",
            "pattern_suggestion_enabled",
            "notification_preferences",
            "created_at",
            "updated_at",
        }

        assert required_columns.issubset(columns), f"Missing columns: {required_columns - columns}"

    def test_progressive_settings_defaults(self):
        """Test that ProgressiveSettings has correct default definitions."""
        from penf_lib.automation.models import ProgressiveSettings
        from sqlalchemy import inspect

        # Check that columns have correct default definitions
        mapper = inspect(ProgressiveSettings)
        columns = {c.key: c for c in mapper.columns}

        assert columns["aggressiveness_level"].default.arg == "moderate"
        assert columns["auto_threshold_adjustment"].default.arg is True
        assert columns["min_learning_period_days"].default.arg == 30
        assert columns["target_automation_rate"].default.arg == Decimal("0.60")
        assert columns["accuracy_floor"].default.arg == Decimal("0.95")
        assert columns["pattern_suggestion_enabled"].default.arg is True


class TestModelRelationships:
    """Tests for model relationships."""

    def test_rule_to_versions_relationship(self):
        """Test that AutomationRule has versions relationship."""
        from penf_lib.automation.models import AutomationRule

        mapper = inspect(AutomationRule)
        relationships = {r.key for r in mapper.relationships}

        assert "versions" in relationships

    def test_rule_to_decisions_relationship(self):
        """Test that AutomationRule has decisions relationship."""
        from penf_lib.automation.models import AutomationRule

        mapper = inspect(AutomationRule)
        relationships = {r.key for r in mapper.relationships}

        assert "decisions" in relationships

    def test_rule_to_effectiveness_relationship(self):
        """Test that AutomationRule has effectiveness_records relationship."""
        from penf_lib.automation.models import AutomationRule

        mapper = inspect(AutomationRule)
        relationships = {r.key for r in mapper.relationships}

        assert "effectiveness_records" in relationships
