"""Unit tests for automation effectiveness monitoring.

Tests the rule effectiveness metrics, degradation detection,
and statistics calculations.
"""

import pytest
from datetime import datetime, timezone, timedelta
from uuid import uuid4


class TestRuleMetrics:
    """Tests for rule metrics calculation."""

    def test_calculate_rule_metrics_basic(self):
        """Test basic metrics calculation from decisions.

        Given rule applied 100 times with 5 overrides
        Then accuracy_rate = 0.95, total_applied = 100
        """
        from penf_lib.automation.effectiveness import calculate_rule_metrics

        rule_id = uuid4()
        now = datetime.now(timezone.utc)
        period_start = now - timedelta(days=7)
        period_end = now

        # 100 decisions, 5 overrides
        decisions = []
        for i in range(100):
            decisions.append({
                "rule_id": rule_id,
                "decision_type": "auto_processed",
                "user_override": i < 5,  # First 5 are overrides
                "confidence_score": 0.90,
                "processing_time_ms": 50,
                "created_at": now - timedelta(hours=i),
            })

        metrics = calculate_rule_metrics(rule_id, decisions, period_start, period_end)

        assert metrics.total_matches == 100
        assert metrics.total_applied == 100
        assert metrics.correct_decisions == 95
        assert metrics.incorrect_decisions == 5
        assert metrics.accuracy_rate == 0.95

    def test_calculate_rule_metrics_empty_decisions(self):
        """Test metrics with no decisions."""
        from penf_lib.automation.effectiveness import calculate_rule_metrics

        rule_id = uuid4()
        now = datetime.now(timezone.utc)

        metrics = calculate_rule_metrics(
            rule_id, [], now - timedelta(days=7), now
        )

        assert metrics.total_matches == 0
        assert metrics.accuracy_rate is None

    def test_calculate_rule_metrics_average_confidence(self):
        """Test average confidence calculation."""
        from penf_lib.automation.effectiveness import calculate_rule_metrics

        rule_id = uuid4()
        now = datetime.now(timezone.utc)

        decisions = [
            {"rule_id": rule_id, "confidence_score": 0.90, "decision_type": "auto_processed", "created_at": now},
            {"rule_id": rule_id, "confidence_score": 0.85, "decision_type": "auto_processed", "created_at": now},
            {"rule_id": rule_id, "confidence_score": 0.95, "decision_type": "auto_processed", "created_at": now},
        ]

        metrics = calculate_rule_metrics(
            rule_id, decisions, now - timedelta(days=1), now + timedelta(days=1)
        )

        assert metrics.avg_confidence == 0.9  # (0.90 + 0.85 + 0.95) / 3


class TestDegradationDetection:
    """Tests for degradation detection logic."""

    def test_detect_degradation_below_threshold(self):
        """Test degradation alert when accuracy drops below threshold.

        Given rule accuracy drops below 85% over 7 days
        Then degradation alert generated
        """
        from penf_lib.automation.effectiveness import detect_degradation, RuleMetrics

        rule_id = uuid4()
        now = datetime.now(timezone.utc)

        current_metrics = RuleMetrics(
            rule_id=rule_id,
            period_start=now - timedelta(days=7),
            period_end=now,
            total_matches=50,
            correct_decisions=40,  # 80% accuracy
            incorrect_decisions=10,
            accuracy_rate=0.80,
        )

        alert = detect_degradation(
            current_metrics=current_metrics,
            previous_metrics=None,
            rule_name="Test Rule",
            accuracy_threshold=0.85,
        )

        assert alert is not None
        assert alert.current_accuracy == 0.80
        assert alert.threshold == 0.85
        assert "low" in alert.severity or "medium" in alert.severity

    def test_detect_degradation_high_severity(self):
        """Test high severity alert for very low accuracy."""
        from penf_lib.automation.effectiveness import detect_degradation, RuleMetrics

        rule_id = uuid4()
        now = datetime.now(timezone.utc)

        current_metrics = RuleMetrics(
            rule_id=rule_id,
            period_start=now - timedelta(days=7),
            period_end=now,
            total_matches=50,
            correct_decisions=30,  # 60% accuracy
            incorrect_decisions=20,
            accuracy_rate=0.60,
        )

        alert = detect_degradation(current_metrics, None, "Test Rule")

        assert alert is not None
        assert alert.severity == "high"
        assert "disabling" in alert.recommendation.lower()

    def test_detect_degradation_significant_drop(self):
        """Test alert when accuracy drops significantly from previous."""
        from penf_lib.automation.effectiveness import detect_degradation, RuleMetrics

        rule_id = uuid4()
        now = datetime.now(timezone.utc)

        previous = RuleMetrics(
            rule_id=rule_id,
            period_start=now - timedelta(days=14),
            period_end=now - timedelta(days=7),
            total_matches=50,
            accuracy_rate=0.98,  # Was 98%
        )

        current = RuleMetrics(
            rule_id=rule_id,
            period_start=now - timedelta(days=7),
            period_end=now,
            total_matches=50,
            accuracy_rate=0.87,  # Now 87% (11% drop, but above 85% threshold)
        )

        alert = detect_degradation(current, previous, "Test Rule")

        assert alert is not None
        assert alert.previous_accuracy == 0.98
        assert "dropped" in alert.recommendation.lower()

    def test_no_degradation_healthy_rule(self):
        """Test no alert for healthy rule."""
        from penf_lib.automation.effectiveness import detect_degradation, RuleMetrics

        rule_id = uuid4()
        now = datetime.now(timezone.utc)

        current_metrics = RuleMetrics(
            rule_id=rule_id,
            period_start=now - timedelta(days=7),
            period_end=now,
            total_matches=50,
            correct_decisions=48,  # 96% accuracy
            incorrect_decisions=2,
            accuracy_rate=0.96,
        )

        alert = detect_degradation(current_metrics, None, "Test Rule")

        assert alert is None

    def test_no_degradation_insufficient_samples(self):
        """Test no alert with insufficient data."""
        from penf_lib.automation.effectiveness import detect_degradation, RuleMetrics

        rule_id = uuid4()
        now = datetime.now(timezone.utc)

        current_metrics = RuleMetrics(
            rule_id=rule_id,
            period_start=now - timedelta(days=7),
            period_end=now,
            total_matches=5,  # Only 5 samples
            accuracy_rate=0.60,  # Low accuracy but not enough data
        )

        alert = detect_degradation(current_metrics, None, "Test Rule", min_samples=10)

        assert alert is None  # Not enough samples to make judgment


class TestAutomationStats:
    """Tests for aggregated statistics."""

    def test_calculate_automation_stats(self):
        """Test overall automation statistics calculation.

        Given multiple rules active for 30 days
        Then stats command shows aggregated metrics
        """
        from penf_lib.automation.effectiveness import calculate_automation_stats

        decisions = [
            {"decision_type": "auto_processed", "user_override": False},
            {"decision_type": "auto_processed", "user_override": False},
            {"decision_type": "auto_processed", "user_override": True},
            {"decision_type": "queued_review", "user_override": False},
            {"decision_type": "conflict_resolved", "user_override": False},
        ]

        rules = [
            {"is_enabled": True, "is_deleted": False},
            {"is_enabled": True, "is_deleted": False},
            {"is_enabled": False, "is_deleted": False},
        ]

        patterns = [
            {"status": "pending"},
            {"status": "accepted"},
        ]

        stats = calculate_automation_stats(decisions, rules, patterns)

        assert stats.total_processed == 5
        assert stats.auto_processed == 3
        assert stats.queued_review == 1
        assert stats.conflicts_resolved == 1
        assert stats.automation_rate == 0.6  # 3/5
        assert stats.overall_accuracy == 0.8  # 4/5 correct
        assert stats.active_rules == 2
        assert stats.pending_patterns == 1

    def test_calculate_automation_stats_empty(self):
        """Test stats with no data."""
        from penf_lib.automation.effectiveness import calculate_automation_stats

        stats = calculate_automation_stats([], [], [])

        assert stats.total_processed == 0
        assert stats.automation_rate == 0.0


class TestRuleImpactAnalysis:
    """Tests for individual rule impact analysis."""

    def test_analyze_rule_impact(self):
        """Test rule impact analysis."""
        from penf_lib.automation.effectiveness import analyze_rule_impact

        rule_id = uuid4()
        rule = {"id": rule_id, "name": "Test Rule"}

        decisions = [
            {"rule_id": rule_id, "decision_type": "auto_processed", "user_override": False},
            {"rule_id": rule_id, "decision_type": "auto_processed", "user_override": False},
            {"rule_id": rule_id, "decision_type": "auto_processed", "user_override": True},
            {"rule_id": uuid4(), "decision_type": "auto_processed", "user_override": False},  # Other rule
        ]

        impact = analyze_rule_impact(rule, decisions)

        assert impact["rule_name"] == "Test Rule"
        assert impact["total_matches"] == 3
        assert impact["auto_processed"] == 3
        assert impact["coverage_rate"] == 0.75  # 3/4
        assert impact["accuracy_rate"] == round(2/3, 3)  # ~0.667
        assert impact["status"] == "needs_attention"  # Below 85%

    def test_analyze_rule_impact_healthy(self):
        """Test impact analysis for healthy rule."""
        from penf_lib.automation.effectiveness import analyze_rule_impact

        rule_id = uuid4()
        rule = {"id": rule_id, "name": "Good Rule"}

        decisions = [
            {"rule_id": rule_id, "decision_type": "auto_processed", "user_override": False}
            for _ in range(10)
        ]

        impact = analyze_rule_impact(rule, decisions)

        assert impact["accuracy_rate"] == 1.0
        assert impact["status"] == "healthy"
        assert impact["estimated_time_saved_minutes"] == 5.0  # 10 * 30s / 60
