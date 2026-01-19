"""Unit tests for the Automation Conditions evaluation logic.

Tests the condition evaluation system that powers rule matching,
including all supported operators and field types.
"""

import pytest
from datetime import datetime, timezone
from decimal import Decimal
from typing import Any, Dict


class TestConditionOperators:
    """Tests for individual condition operators."""

    def test_equals_operator_with_string(self):
        """Test equals operator with string values."""
        from penf_lib.automation.conditions import evaluate_condition

        condition = {"field": "content_type", "operator": "equals", "value": "email"}
        content = {"content_type": "email"}

        assert evaluate_condition(condition, content) is True

        content_different = {"content_type": "meeting"}
        assert evaluate_condition(condition, content_different) is False

    def test_contains_operator_with_string(self):
        """Test contains operator for substring matching."""
        from penf_lib.automation.conditions import evaluate_condition

        condition = {"field": "sender", "operator": "contains", "value": "@acme.com"}
        content = {"sender": "alice@acme.com"}

        assert evaluate_condition(condition, content) is True

        content_no_match = {"sender": "bob@other.com"}
        assert evaluate_condition(condition, content_no_match) is False

    def test_greater_than_operator_with_number(self):
        """Test greater_than operator with numeric values."""
        from penf_lib.automation.conditions import evaluate_condition

        condition = {"field": "confidence", "operator": "greater_than", "value": 0.9}
        content = {"confidence": 0.95}

        assert evaluate_condition(condition, content) is True

        content_lower = {"confidence": 0.85}
        assert evaluate_condition(condition, content_lower) is False

    def test_less_than_operator_with_number(self):
        """Test less_than operator with numeric values."""
        from penf_lib.automation.conditions import evaluate_condition

        condition = {"field": "confidence", "operator": "less_than", "value": 0.7}
        content = {"confidence": 0.65}

        assert evaluate_condition(condition, content) is True

        content_higher = {"confidence": 0.75}
        assert evaluate_condition(condition, content_higher) is False

    def test_matches_operator_with_regex(self):
        """Test matches operator for regex matching."""
        from penf_lib.automation.conditions import evaluate_condition

        condition = {"field": "subject", "operator": "matches", "value": r"^\[URGENT\]"}
        content = {"subject": "[URGENT] Need help"}

        assert evaluate_condition(condition, content) is True

        content_no_match = {"subject": "Regular email"}
        assert evaluate_condition(condition, content_no_match) is False

    def test_in_operator_with_list(self):
        """Test in operator for list membership."""
        from penf_lib.automation.conditions import evaluate_condition

        condition = {"field": "content_type", "operator": "in", "value": ["email", "meeting"]}
        content = {"content_type": "email"}

        assert evaluate_condition(condition, content) is True

        content_meeting = {"content_type": "meeting"}
        assert evaluate_condition(condition, content_meeting) is True

        content_document = {"content_type": "document"}
        assert evaluate_condition(condition, content_document) is False


class TestConditionCombination:
    """Tests for combining multiple conditions with AND/OR."""

    def test_and_requires_all_conditions_true(self):
        """Test that AND operator requires all conditions to be true."""
        from penf_lib.automation.conditions import evaluate_conditions

        conditions = {
            "operator": "AND",
            "conditions": [
                {"field": "content_type", "operator": "equals", "value": "email"},
                {"field": "sender", "operator": "contains", "value": "@acme.com"}
            ]
        }

        content_all_match = {"content_type": "email", "sender": "alice@acme.com"}
        assert evaluate_conditions(conditions, content_all_match) is True

        content_one_fails = {"content_type": "email", "sender": "bob@other.com"}
        assert evaluate_conditions(conditions, content_one_fails) is False

    def test_or_requires_any_condition_true(self):
        """Test that OR operator requires at least one condition to be true."""
        from penf_lib.automation.conditions import evaluate_conditions

        conditions = {
            "operator": "OR",
            "conditions": [
                {"field": "sender", "operator": "contains", "value": "@vip.com"},
                {"field": "subject", "operator": "contains", "value": "[PRIORITY]"}
            ]
        }

        content_first_match = {"sender": "ceo@vip.com", "subject": "Hello"}
        assert evaluate_conditions(conditions, content_first_match) is True

        content_second_match = {"sender": "anyone@other.com", "subject": "[PRIORITY] Task"}
        assert evaluate_conditions(conditions, content_second_match) is True

        content_none_match = {"sender": "anyone@other.com", "subject": "Regular"}
        assert evaluate_conditions(conditions, content_none_match) is False

    def test_nested_conditions(self):
        """Test that nested AND/OR conditions work correctly."""
        from penf_lib.automation.conditions import evaluate_conditions

        # Match: (content_type=email AND (vip sender OR priority subject))
        conditions = {
            "operator": "AND",
            "conditions": [
                {"field": "content_type", "operator": "equals", "value": "email"},
                {
                    "operator": "OR",
                    "conditions": [
                        {"field": "sender", "operator": "contains", "value": "@vip.com"},
                        {"field": "subject", "operator": "contains", "value": "[PRIORITY]"}
                    ]
                }
            ]
        }

        content_match = {"content_type": "email", "sender": "ceo@vip.com", "subject": "Hello"}
        assert evaluate_conditions(conditions, content_match) is True

        # Not email - fails
        content_wrong_type = {"content_type": "meeting", "sender": "ceo@vip.com", "subject": "Hello"}
        assert evaluate_conditions(conditions, content_wrong_type) is False


class TestFieldAccess:
    """Tests for accessing content fields."""

    def test_missing_field_returns_false(self):
        """Test that missing fields result in condition being false."""
        from penf_lib.automation.conditions import evaluate_condition

        condition = {"field": "sender", "operator": "contains", "value": "@acme.com"}
        content = {"content_type": "email"}  # No sender field

        assert evaluate_condition(condition, content) is False

    def test_none_field_value_returns_false(self):
        """Test that None field values result in condition being false."""
        from penf_lib.automation.conditions import evaluate_condition

        condition = {"field": "sender", "operator": "contains", "value": "@acme.com"}
        content = {"sender": None}

        assert evaluate_condition(condition, content) is False

    def test_case_insensitive_contains(self):
        """Test that contains is case insensitive."""
        from penf_lib.automation.conditions import evaluate_condition

        condition = {"field": "sender", "operator": "contains", "value": "@ACME.COM"}
        content = {"sender": "alice@acme.com"}

        assert evaluate_condition(condition, content) is True


class TestConditionValidation:
    """Tests for condition validation."""

    def test_invalid_operator_raises_error(self):
        """Test that invalid operators raise appropriate errors."""
        from penf_lib.automation.conditions import evaluate_condition, InvalidConditionError

        condition = {"field": "sender", "operator": "invalid_op", "value": "test"}
        content = {"sender": "test"}

        with pytest.raises(InvalidConditionError):
            evaluate_condition(condition, content)

    def test_missing_field_key_raises_error(self):
        """Test that conditions without field key raise error."""
        from penf_lib.automation.conditions import evaluate_condition, InvalidConditionError

        condition = {"operator": "equals", "value": "test"}  # Missing 'field'
        content = {"sender": "test"}

        with pytest.raises(InvalidConditionError):
            evaluate_condition(condition, content)

    def test_missing_operator_raises_error(self):
        """Test that conditions without operator raise error."""
        from penf_lib.automation.conditions import evaluate_condition, InvalidConditionError

        condition = {"field": "sender", "value": "test"}  # Missing 'operator'
        content = {"sender": "test"}

        with pytest.raises(InvalidConditionError):
            evaluate_condition(condition, content)
