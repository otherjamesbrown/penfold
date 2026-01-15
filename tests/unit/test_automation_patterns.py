"""Unit tests for automation pattern detection and progressive automation.

Tests the pattern detection system that identifies recurring user
behaviors and the progressive automation that adjusts thresholds.
"""

import pytest
from datetime import datetime, timezone, timedelta
from uuid import uuid4


class TestPatternDetection:
    """Tests for pattern detection logic."""

    def test_compute_pattern_signature_deterministic(self):
        """Test that pattern signatures are deterministic."""
        from penf_lib.automation.patterns import compute_pattern_signature

        conditions1 = {
            "operator": "AND",
            "conditions": [
                {"field": "sender", "operator": "contains", "value": "@vendor.com"},
            ]
        }
        conditions2 = {
            "operator": "AND",
            "conditions": [
                {"field": "sender", "operator": "contains", "value": "@vendor.com"},
            ]
        }

        sig1 = compute_pattern_signature(conditions1)
        sig2 = compute_pattern_signature(conditions2)

        assert sig1 == sig2
        assert len(sig1) == 64  # SHA-256 hex

    def test_compute_pattern_signature_different_for_different_conditions(self):
        """Test that different conditions produce different signatures."""
        from penf_lib.automation.patterns import compute_pattern_signature

        conditions1 = {
            "operator": "AND",
            "conditions": [
                {"field": "sender", "operator": "contains", "value": "@vendor1.com"},
            ]
        }
        conditions2 = {
            "operator": "AND",
            "conditions": [
                {"field": "sender", "operator": "contains", "value": "@vendor2.com"},
            ]
        }

        sig1 = compute_pattern_signature(conditions1)
        sig2 = compute_pattern_signature(conditions2)

        assert sig1 != sig2

    def test_extract_pattern_conditions_from_email(self):
        """Test extracting conditions from email content."""
        from penf_lib.automation.patterns import extract_pattern_conditions

        content = {
            "content_id": "email-001",
            "content_type": "email",
            "sender": "sales@vendor.com",
            "subject": "[Project Alpha] Status Update",
        }
        feedback = {"action_type": "categorize", "parameters": {"category": "work"}}

        conditions = extract_pattern_conditions(content, feedback)

        assert conditions["operator"] == "AND"
        assert len(conditions["conditions"]) >= 2

        # Check content type condition
        content_type_cond = next(
            (c for c in conditions["conditions"] if c["field"] == "content_type"), None
        )
        assert content_type_cond is not None
        assert content_type_cond["value"] == "email"

        # Check sender domain condition
        sender_cond = next(
            (c for c in conditions["conditions"] if c["field"] == "sender"), None
        )
        assert sender_cond is not None
        assert "@vendor.com" in sender_cond["value"]

    def test_pattern_detector_creates_new_pattern(self):
        """Test that detector creates new pattern on first observation."""
        from penf_lib.automation.patterns import PatternDetector

        detector = PatternDetector(min_occurrences=3, min_confidence=0.6)

        content = {
            "content_id": "email-001",
            "content_type": "email",
            "sender": "alice@vendor.com",
        }
        feedback = {"action_type": "categorize", "parameters": {"category": "vendors"}}

        pattern = detector.analyze_feedback(content, feedback)

        assert pattern is not None
        assert pattern.occurrence_count == 1
        assert len(pattern.signature) == 64

    def test_pattern_detector_updates_existing_pattern(self):
        """Test that detector updates existing pattern on repeat."""
        from penf_lib.automation.patterns import PatternDetector, compute_pattern_signature

        detector = PatternDetector(min_occurrences=3, min_confidence=0.6)

        content1 = {
            "content_id": "email-001",
            "content_type": "email",
            "sender": "alice@vendor.com",
        }
        feedback = {"action_type": "categorize", "parameters": {"category": "vendors"}}

        # First occurrence
        pattern1 = detector.analyze_feedback(content1, feedback)

        # Create existing patterns list
        existing = [{
            "pattern_signature": pattern1.signature,
            "occurrences": pattern1.occurrences,
            "occurrence_count": pattern1.occurrence_count,
            "first_seen_at": pattern1.first_seen_at,
        }]

        # Second occurrence
        content2 = {
            "content_id": "email-002",
            "content_type": "email",
            "sender": "bob@vendor.com",  # Same domain
        }

        pattern2 = detector.analyze_feedback(content2, feedback, existing)

        assert pattern2.occurrence_count == 2
        assert len(pattern2.occurrences) == 2

    def test_pattern_meets_occurrence_threshold(self):
        """Test pattern detection after 3+ occurrences.

        Given user categorizes emails from "@vendor.com" to "Procurement" 5 times
        When pattern detection runs
        Then pattern should be suggested
        """
        from penf_lib.automation.patterns import PatternDetector, DetectedPattern

        detector = PatternDetector(min_occurrences=3, min_confidence=0.5)

        # Simulate pattern with 5 occurrences
        pattern = DetectedPattern(
            signature="abc123",
            conditions={"operator": "AND", "conditions": []},
            suggested_actions={"action_type": "categorize"},
            occurrences=["e1", "e2", "e3", "e4", "e5"],
            occurrence_count=5,
            confidence_score=0.7,
        )

        assert detector.should_suggest_pattern(pattern) is True

    def test_pattern_below_threshold_not_suggested(self):
        """Test that patterns below threshold are not suggested."""
        from penf_lib.automation.patterns import PatternDetector, DetectedPattern

        detector = PatternDetector(min_occurrences=3, min_confidence=0.6)

        # Pattern with only 2 occurrences
        pattern = DetectedPattern(
            signature="abc123",
            conditions={"operator": "AND", "conditions": []},
            suggested_actions={"action_type": "categorize"},
            occurrences=["e1", "e2"],
            occurrence_count=2,
            confidence_score=0.7,
        )

        assert detector.should_suggest_pattern(pattern) is False

    def test_pattern_confidence_calculation(self):
        """Test confidence calculation for patterns."""
        from penf_lib.automation.patterns import calculate_pattern_confidence

        # Low occurrences, high consistency
        conf1 = calculate_pattern_confidence(
            occurrences=1,
            consistency_score=1.0,
            time_span_days=0,
        )
        assert conf1 < 0.5  # Low due to few occurrences

        # High occurrences, high consistency
        conf2 = calculate_pattern_confidence(
            occurrences=10,
            consistency_score=1.0,
            time_span_days=30,
        )
        assert conf2 > 0.8  # High due to many occurrences

    def test_convert_pattern_to_rule(self):
        """Test converting a pattern to an automation rule.

        Given pending pattern suggestion
        When user accepts pattern
        Then AutomationRule can be created from pattern
        """
        from penf_lib.automation.patterns import PatternDetector, DetectedPattern

        detector = PatternDetector()

        pattern = DetectedPattern(
            signature="abc123",
            conditions={
                "operator": "AND",
                "conditions": [
                    {"field": "sender", "operator": "contains", "value": "@vendor.com"},
                ]
            },
            suggested_actions={"action_type": "categorize", "parameters": {"category": "vendors"}},
            occurrences=["e1", "e2", "e3"],
            occurrence_count=3,
            confidence_score=0.75,
        )

        rule = detector.convert_pattern_to_rule(pattern, user_id="test@example.com")

        assert "name" in rule
        assert rule["conditions"] == pattern.conditions
        assert rule["actions"] == pattern.suggested_actions
        assert rule["priority"] == 5


class TestProgressiveAutomation:
    """Tests for progressive automation threshold adjustment."""

    def test_calculate_automation_rate(self):
        """Test automation rate calculation."""
        from penf_lib.automation.progressive import calculate_automation_rate

        decisions = [
            {"decision_type": "auto_processed"},
            {"decision_type": "auto_processed"},
            {"decision_type": "queued_review"},
            {"decision_type": "auto_processed"},
        ]

        rate = calculate_automation_rate(decisions)
        assert rate == 0.75  # 3/4

    def test_calculate_accuracy_rate(self):
        """Test accuracy rate calculation from overrides."""
        from penf_lib.automation.progressive import calculate_accuracy_rate

        decisions = [
            {"decision_type": "auto_processed", "user_override": False},
            {"decision_type": "auto_processed", "user_override": True},  # Wrong
            {"decision_type": "auto_processed", "user_override": False},
            {"decision_type": "auto_processed", "user_override": False},
        ]

        rate = calculate_accuracy_rate(decisions)
        assert rate == 0.75  # 3/4 correct

    def test_progressive_analysis_not_enough_data(self):
        """Test that analysis requires minimum learning period."""
        from penf_lib.automation.progressive import ProgressiveAutomation

        progressive = ProgressiveAutomation(default_learning_days=30)

        decisions = [
            {"decision_type": "auto_processed", "user_override": False}
            for _ in range(10)  # Only 10 decisions
        ]

        settings = {
            "aggressiveness_level": "moderate",
            "accuracy_floor": 0.95,
            "min_learning_period_days": 30,
            "target_automation_rate": 0.60,
            "auto_threshold_adjustment": True,
        }

        analysis = progressive.analyze(decisions, 0.85, settings)

        assert analysis.can_adjust is False
        assert "Learning period" in analysis.reasoning

    def test_progressive_analysis_recommends_adjustment(self):
        """Test threshold adjustment when conditions are met.

        Given 30-day learning period complete AND accuracy > 95%
        When progressive analysis runs
        Then threshold adjusted to increase automation rate
        """
        from penf_lib.automation.progressive import ProgressiveAutomation

        progressive = ProgressiveAutomation()

        # 50 decisions, 40% auto-processed, 98% accuracy
        decisions = []
        for i in range(50):
            if i < 20:  # 40% auto-processed
                decisions.append({"decision_type": "auto_processed", "user_override": False})
            else:
                decisions.append({"decision_type": "queued_review", "user_override": False})

        # Add one override to bring accuracy to 98%
        decisions[0]["user_override"] = True

        settings = {
            "aggressiveness_level": "moderate",
            "accuracy_floor": 0.95,
            "min_learning_period_days": 30,
            "target_automation_rate": 0.60,
            "auto_threshold_adjustment": True,
        }

        analysis = progressive.analyze(decisions, 0.85, settings)

        assert analysis.can_adjust is True
        assert analysis.recommended_threshold is not None
        assert analysis.recommended_threshold < 0.85  # Threshold should be lowered

    def test_progressive_analysis_respects_accuracy_floor(self):
        """Test that adjustments don't compromise accuracy."""
        from penf_lib.automation.progressive import ProgressiveAutomation

        progressive = ProgressiveAutomation()

        # 50 decisions with poor accuracy (90%)
        decisions = [
            {"decision_type": "auto_processed", "user_override": i < 5}
            for i in range(50)
        ]

        settings = {
            "aggressiveness_level": "moderate",
            "accuracy_floor": 0.95,  # Floor is 95%
            "min_learning_period_days": 30,
            "target_automation_rate": 0.60,
            "auto_threshold_adjustment": True,
        }

        analysis = progressive.analyze(decisions, 0.85, settings)

        assert analysis.can_adjust is False
        assert "Accuracy" in analysis.reasoning

    def test_progressive_analysis_respects_disabled_setting(self):
        """Test that auto-adjustment can be disabled."""
        from penf_lib.automation.progressive import ProgressiveAutomation

        progressive = ProgressiveAutomation()

        decisions = [
            {"decision_type": "auto_processed", "user_override": False}
            for _ in range(50)
        ]

        settings = {
            "aggressiveness_level": "moderate",
            "accuracy_floor": 0.95,
            "min_learning_period_days": 30,
            "target_automation_rate": 0.60,
            "auto_threshold_adjustment": False,  # Disabled
        }

        analysis = progressive.analyze(decisions, 0.85, settings)

        assert analysis.can_adjust is False
        assert "disabled" in analysis.reasoning.lower()

    def test_aggressiveness_levels(self):
        """Test that aggressiveness affects adjustment magnitude."""
        from penf_lib.automation.progressive import ADJUSTMENT_PARAMS, AggressivenessLevel

        conservative = ADJUSTMENT_PARAMS[AggressivenessLevel.CONSERVATIVE]
        moderate = ADJUSTMENT_PARAMS[AggressivenessLevel.MODERATE]
        aggressive = ADJUSTMENT_PARAMS[AggressivenessLevel.AGGRESSIVE]

        # Conservative should have smallest max delta
        assert conservative["max_delta"] < moderate["max_delta"]
        assert moderate["max_delta"] < aggressive["max_delta"]

        # Conservative should require more buffer above floor
        assert conservative["min_accuracy_buffer"] > moderate["min_accuracy_buffer"]

    def test_get_default_settings(self):
        """Test default progressive settings generation."""
        from penf_lib.automation.progressive import ProgressiveAutomation

        progressive = ProgressiveAutomation()

        settings = progressive.get_default_settings("test@example.com", "tenant-123")

        assert settings["aggressiveness_level"] == "moderate"
        assert settings["auto_threshold_adjustment"] is True
        assert settings["min_learning_period_days"] == 30
        assert settings["target_automation_rate"] == 0.60
        assert settings["accuracy_floor"] == 0.95
