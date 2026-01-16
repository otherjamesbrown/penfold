"""Pattern detection logic for automation rule suggestions.

Analyzes user feedback patterns to detect recurring categorizations
and suggest automation rules per User Story 3 (FR-009).

Pattern detection algorithm:
1. Track user feedback on content categorizations
2. Extract conditions from similar categorizations
3. Generate pattern signature (hash) for deduplication
4. Create pattern when occurrence threshold (3+) is met
5. Calculate confidence based on consistency

Reference: research.md Pattern Detection Approach
"""

import hashlib
import json
from dataclasses import dataclass, field
from datetime import datetime, timezone, timedelta
from decimal import Decimal
from typing import Any, Dict, List, Optional, Tuple
from uuid import UUID


@dataclass
class DetectedPattern:
    """A pattern detected from user feedback.

    Attributes:
        signature: Hash of pattern conditions for deduplication
        conditions: Detected conditions in JSONB format
        suggested_actions: Actions to suggest based on feedback
        occurrences: List of content IDs that formed this pattern
        occurrence_count: Number of times pattern was observed
        first_seen_at: When pattern was first detected
        last_seen_at: Most recent occurrence
        confidence_score: How reliable the pattern is (0-1)
    """

    signature: str
    conditions: Dict[str, Any]
    suggested_actions: Dict[str, Any]
    occurrences: List[str] = field(default_factory=list)
    occurrence_count: int = 0
    first_seen_at: datetime = field(default_factory=lambda: datetime.now(timezone.utc))
    last_seen_at: datetime = field(default_factory=lambda: datetime.now(timezone.utc))
    confidence_score: float = 0.0


def compute_pattern_signature(conditions: Dict[str, Any]) -> str:
    """Compute a unique signature hash for pattern conditions.

    Creates a deterministic hash of the conditions for pattern
    deduplication and lookup.

    Args:
        conditions: Pattern conditions dictionary

    Returns:
        64-character hex string (SHA-256 hash)
    """
    # Sort keys for deterministic serialization
    serialized = json.dumps(conditions, sort_keys=True, default=str)
    return hashlib.sha256(serialized.encode()).hexdigest()


def extract_pattern_conditions(
    content: Dict[str, Any], feedback: Dict[str, Any]
) -> Dict[str, Any]:
    """Extract pattern conditions from content and user feedback.

    Analyzes what made the user categorize content a certain way
    and extracts those characteristics as rule conditions.

    Args:
        content: The content that was categorized
        feedback: User's categorization feedback

    Returns:
        Conditions dictionary suitable for rule matching
    """
    conditions_list = []

    # Extract content type condition
    if content.get("content_type"):
        conditions_list.append({
            "field": "content_type",
            "operator": "equals",
            "value": content["content_type"],
        })

    # Extract sender pattern (email domain)
    sender = content.get("sender", "")
    if sender and "@" in sender:
        domain = sender.split("@")[1]
        conditions_list.append({
            "field": "sender",
            "operator": "contains",
            "value": f"@{domain}",
        })

    # Extract subject keywords if present
    subject = content.get("subject", "")
    if subject:
        # Look for common patterns like [Project Name] or prefixes
        if subject.startswith("[") and "]" in subject:
            prefix = subject.split("]")[0] + "]"
            conditions_list.append({
                "field": "subject",
                "operator": "contains",
                "value": prefix,
            })

    return {
        "operator": "AND",
        "conditions": conditions_list,
    }


def extract_suggested_actions(feedback: Dict[str, Any]) -> Dict[str, Any]:
    """Extract suggested actions from user feedback.

    Analyzes what action the user took and creates an action
    specification for the suggested rule.

    Args:
        feedback: User's categorization feedback

    Returns:
        Actions dictionary suitable for rule execution
    """
    action_type = feedback.get("action_type", "categorize")
    parameters = feedback.get("parameters", {})

    return {
        "action_type": action_type,
        "parameters": parameters,
    }


def calculate_pattern_confidence(
    occurrences: int,
    consistency_score: float,
    time_span_days: float,
) -> float:
    """Calculate confidence score for a detected pattern.

    Confidence is based on:
    - Number of occurrences (more = higher confidence)
    - Consistency of user actions (same action = higher)
    - Time span (longer patterns = more reliable)

    Args:
        occurrences: Number of times pattern observed
        consistency_score: How consistent user actions were (0-1)
        time_span_days: Days between first and last occurrence

    Returns:
        Confidence score between 0 and 1
    """
    # Occurrence factor (saturates at ~10 occurrences)
    occurrence_factor = min(occurrences / 10.0, 1.0)

    # Time factor (patterns spanning more days are more reliable)
    time_factor = min(time_span_days / 30.0, 1.0)  # Saturates at 30 days

    # Weighted combination
    # Occurrences: 50%, Consistency: 40%, Time: 10%
    confidence = (
        occurrence_factor * 0.5 +
        consistency_score * 0.4 +
        time_factor * 0.1
    )

    return round(min(max(confidence, 0.0), 1.0), 3)


class PatternDetector:
    """Detects automation patterns from user feedback.

    Analyzes user categorization decisions to identify recurring
    patterns that could be converted to automation rules.

    Attributes:
        min_occurrences: Minimum occurrences before suggesting (default 3)
        min_confidence: Minimum confidence for suggestions (default 0.6)
    """

    def __init__(
        self,
        min_occurrences: int = 3,
        min_confidence: float = 0.6,
    ):
        """Initialize pattern detector.

        Args:
            min_occurrences: Minimum occurrences to create pattern
            min_confidence: Minimum confidence to suggest pattern
        """
        self.min_occurrences = min_occurrences
        self.min_confidence = min_confidence

    def analyze_feedback(
        self,
        content: Dict[str, Any],
        feedback: Dict[str, Any],
        existing_patterns: Optional[List[Dict[str, Any]]] = None,
    ) -> Optional[DetectedPattern]:
        """Analyze user feedback for potential patterns.

        Args:
            content: Content that was categorized
            feedback: User's categorization feedback
            existing_patterns: Current patterns to update

        Returns:
            New or updated pattern, or None if no pattern detected
        """
        # Extract conditions and actions
        conditions = extract_pattern_conditions(content, feedback)
        actions = extract_suggested_actions(feedback)

        # Compute signature
        signature = compute_pattern_signature(conditions)

        # Check if pattern exists
        existing = None
        if existing_patterns:
            for p in existing_patterns:
                if p.get("pattern_signature") == signature:
                    existing = p
                    break

        now = datetime.now(timezone.utc)
        content_id = content.get("content_id", "unknown")

        if existing:
            # Update existing pattern
            occurrences = existing.get("occurrences", []) + [content_id]
            occurrence_count = existing.get("occurrence_count", 0) + 1
            first_seen = existing.get("first_seen_at", now)

            time_span_days = (now - first_seen).days if isinstance(first_seen, datetime) else 0

            confidence = calculate_pattern_confidence(
                occurrences=occurrence_count,
                consistency_score=0.9,  # Assume high consistency for matching signature
                time_span_days=time_span_days,
            )

            return DetectedPattern(
                signature=signature,
                conditions=conditions,
                suggested_actions=actions,
                occurrences=occurrences,
                occurrence_count=occurrence_count,
                first_seen_at=first_seen,
                last_seen_at=now,
                confidence_score=confidence,
            )
        else:
            # Create new pattern
            return DetectedPattern(
                signature=signature,
                conditions=conditions,
                suggested_actions=actions,
                occurrences=[content_id],
                occurrence_count=1,
                first_seen_at=now,
                last_seen_at=now,
                confidence_score=calculate_pattern_confidence(1, 1.0, 0),
            )

    def should_suggest_pattern(self, pattern: DetectedPattern) -> bool:
        """Determine if a pattern should be suggested to the user.

        Args:
            pattern: The pattern to evaluate

        Returns:
            True if pattern should be suggested
        """
        return (
            pattern.occurrence_count >= self.min_occurrences and
            pattern.confidence_score >= self.min_confidence
        )

    def convert_pattern_to_rule(
        self,
        pattern: DetectedPattern,
        user_id: str,
        rule_name: Optional[str] = None,
    ) -> Dict[str, Any]:
        """Convert a detected pattern to an automation rule.

        Args:
            pattern: The pattern to convert
            user_id: User who will own the rule
            rule_name: Optional custom name for the rule

        Returns:
            Rule dictionary ready for creation
        """
        if not rule_name:
            # Generate name from pattern conditions
            conditions = pattern.conditions.get("conditions", [])
            if conditions:
                first_cond = conditions[0]
                rule_name = f"Auto: {first_cond.get('field', 'Unknown')} pattern"
            else:
                rule_name = "Auto-generated rule from pattern"

        return {
            "name": rule_name,
            "description": f"Auto-generated from pattern detected {pattern.occurrence_count} times",
            "conditions": pattern.conditions,
            "actions": pattern.suggested_actions,
            "priority": 5,
            "is_enabled": True,
            "source_pattern_signature": pattern.signature,
        }
