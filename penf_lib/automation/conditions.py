"""Condition evaluation logic for automation rules.

Provides the condition evaluation engine that powers rule matching.
Supports various operators (equals, contains, greater_than, etc.) and
compound conditions (AND/OR).

Reference: spec.md FR-011 - Rule conditions based on content type,
participants, projects, keywords, and AI confidence.
"""

import re
from dataclasses import dataclass, field
from enum import Enum
from typing import Any, Dict, List, Optional, Union


class ConditionOperator(Enum):
    """Supported condition operators."""

    EQUALS = "equals"
    CONTAINS = "contains"
    GREATER_THAN = "greater_than"
    LESS_THAN = "less_than"
    MATCHES = "matches"  # Regex
    IN = "in"  # List membership


class LogicalOperator(Enum):
    """Logical operators for combining conditions."""

    AND = "AND"
    OR = "OR"


class InvalidConditionError(Exception):
    """Raised when a condition is malformed or uses an invalid operator."""

    pass


@dataclass
class RuleCondition:
    """A single condition in a rule.

    Attributes:
        field: The content field to evaluate (content_type, sender, etc.)
        operator: The comparison operator (equals, contains, etc.)
        value: The value to compare against
    """

    field: str
    operator: ConditionOperator
    value: Any

    @classmethod
    def from_dict(cls, data: Dict[str, Any]) -> "RuleCondition":
        """Create a RuleCondition from a dictionary."""
        if "field" not in data:
            raise InvalidConditionError("Condition must have 'field' key")
        if "operator" not in data:
            raise InvalidConditionError("Condition must have 'operator' key")

        try:
            operator = ConditionOperator(data["operator"])
        except ValueError:
            raise InvalidConditionError(f"Invalid operator: {data['operator']}")

        return cls(
            field=data["field"],
            operator=operator,
            value=data.get("value"),
        )


def evaluate_condition(condition: Dict[str, Any], content: Dict[str, Any]) -> bool:
    """Evaluate a single condition against content.

    Args:
        condition: Dictionary with field, operator, and value keys
        content: Content dictionary to evaluate against

    Returns:
        True if condition matches, False otherwise

    Raises:
        InvalidConditionError: If condition is malformed
    """
    # Validate required fields
    if "field" not in condition:
        raise InvalidConditionError("Condition must have 'field' key")
    if "operator" not in condition:
        raise InvalidConditionError("Condition must have 'operator' key")

    field_name = condition["field"]
    operator = condition["operator"]
    expected_value = condition.get("value")

    # Get the actual value from content
    actual_value = content.get(field_name)

    # Missing or None fields always fail
    if actual_value is None:
        return False

    # Evaluate based on operator
    try:
        op = ConditionOperator(operator)
    except ValueError:
        raise InvalidConditionError(f"Invalid operator: {operator}")

    if op == ConditionOperator.EQUALS:
        return _evaluate_equals(actual_value, expected_value)
    elif op == ConditionOperator.CONTAINS:
        return _evaluate_contains(actual_value, expected_value)
    elif op == ConditionOperator.GREATER_THAN:
        return _evaluate_greater_than(actual_value, expected_value)
    elif op == ConditionOperator.LESS_THAN:
        return _evaluate_less_than(actual_value, expected_value)
    elif op == ConditionOperator.MATCHES:
        return _evaluate_matches(actual_value, expected_value)
    elif op == ConditionOperator.IN:
        return _evaluate_in(actual_value, expected_value)

    return False


def _evaluate_equals(actual: Any, expected: Any) -> bool:
    """Evaluate equality, case-insensitive for strings."""
    if isinstance(actual, str) and isinstance(expected, str):
        return actual.lower() == expected.lower()
    return actual == expected


def _evaluate_contains(actual: Any, expected: Any) -> bool:
    """Evaluate substring containment, case-insensitive."""
    if not isinstance(actual, str):
        return False
    if not isinstance(expected, str):
        return False
    return expected.lower() in actual.lower()


def _evaluate_greater_than(actual: Any, expected: Any) -> bool:
    """Evaluate numeric greater than."""
    try:
        return float(actual) > float(expected)
    except (ValueError, TypeError):
        return False


def _evaluate_less_than(actual: Any, expected: Any) -> bool:
    """Evaluate numeric less than."""
    try:
        return float(actual) < float(expected)
    except (ValueError, TypeError):
        return False


def _evaluate_matches(actual: Any, expected: Any) -> bool:
    """Evaluate regex match."""
    if not isinstance(actual, str):
        return False
    if not isinstance(expected, str):
        return False
    try:
        return bool(re.search(expected, actual))
    except re.error:
        return False


def _evaluate_in(actual: Any, expected: Any) -> bool:
    """Evaluate list membership."""
    if not isinstance(expected, list):
        return False
    # Case-insensitive for strings
    if isinstance(actual, str):
        return any(
            actual.lower() == item.lower() if isinstance(item, str) else actual == item
            for item in expected
        )
    return actual in expected


def evaluate_conditions(
    conditions: Dict[str, Any], content: Dict[str, Any]
) -> bool:
    """Evaluate compound conditions (AND/OR) against content.

    Args:
        conditions: Dictionary with operator (AND/OR) and conditions list
        content: Content dictionary to evaluate against

    Returns:
        True if all conditions match (AND) or any condition matches (OR)
    """
    operator = conditions.get("operator", "AND")
    condition_list = conditions.get("conditions", [])

    if not condition_list:
        return True

    results = []
    for cond in condition_list:
        # Check if this is a nested compound condition
        if "operator" in cond and "conditions" in cond:
            result = evaluate_conditions(cond, content)
        else:
            result = evaluate_condition(cond, content)
        results.append(result)

    if operator == "AND":
        return all(results)
    elif operator == "OR":
        return any(results)

    return False
