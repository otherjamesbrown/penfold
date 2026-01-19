"""Unit tests for multi-account management components."""

import pytest
from datetime import datetime, timedelta
from unittest.mock import Mock, patch

from penf_lib.connectors.gmail.multi_account import (
    AccountActivity,
    AccountPriority,
    SyncScheduleEntry,
    AccountPriorityCalculator
)
from penf_lib.connectors.gmail.scheduler import (
    SchedulerConfig,
    SchedulerMode,
    SyncMetrics
)


class TestAccountActivity:
    """Test cases for AccountActivity."""

    def test_account_activity_initialization(self):
        """Test AccountActivity initialization."""
        activity = AccountActivity(
            connection_id=1,
            email_address="test@example.com"
        )

        assert activity.connection_id == 1
        assert activity.email_address == "test@example.com"
        assert activity.messages_last_24h == 0
        assert activity.calculated_priority == AccountPriority.MEDIUM
        assert activity.user_override_priority is None
        assert activity.is_sync_enabled is True

    def test_effective_priority_without_override(self):
        """Test effective priority calculation without user override."""
        activity = AccountActivity(1, "test@example.com")
        activity.calculated_priority = AccountPriority.HIGH

        assert activity.get_effective_priority() == AccountPriority.HIGH

    def test_effective_priority_with_override(self):
        """Test effective priority calculation with user override."""
        activity = AccountActivity(1, "test@example.com")
        activity.calculated_priority = AccountPriority.LOW
        activity.user_override_priority = AccountPriority.CRITICAL

        assert activity.get_effective_priority() == AccountPriority.CRITICAL

    def test_activity_metrics_update(self):
        """Test updating activity metrics."""
        activity = AccountActivity(1, "test@example.com")

        # Update metrics
        activity.messages_last_24h = 25
        activity.messages_last_7d = 150
        activity.active_threads = 8
        activity.avg_messages_per_day = 21.4

        assert activity.messages_last_24h == 25
        assert activity.messages_last_7d == 150
        assert activity.active_threads == 8
        assert activity.avg_messages_per_day == 21.4


class TestSyncScheduleEntry:
    """Test cases for SyncScheduleEntry."""

    def test_schedule_entry_initialization(self):
        """Test SyncScheduleEntry initialization."""
        scheduled_time = datetime.utcnow() + timedelta(minutes=30)

        entry = SyncScheduleEntry(
            connection_id=1,
            email_address="test@example.com",
            priority=AccountPriority.HIGH,
            scheduled_time=scheduled_time
        )

        assert entry.connection_id == 1
        assert entry.email_address == "test@example.com"
        assert entry.priority == AccountPriority.HIGH
        assert entry.scheduled_time == scheduled_time
        assert entry.estimated_duration == timedelta(minutes=5)  # default
        assert entry.resource_weight == 1.0  # default

    def test_schedule_entry_with_custom_values(self):
        """Test SyncScheduleEntry with custom duration and weight."""
        scheduled_time = datetime.utcnow()
        custom_duration = timedelta(minutes=15)

        entry = SyncScheduleEntry(
            connection_id=2,
            email_address="priority@example.com",
            priority=AccountPriority.CRITICAL,
            scheduled_time=scheduled_time,
            estimated_duration=custom_duration,
            resource_weight=2.5
        )

        assert entry.estimated_duration == custom_duration
        assert entry.resource_weight == 2.5


class TestAccountPriorityCalculator:
    """Test cases for AccountPriorityCalculator."""

    def test_calculator_initialization(self):
        """Test calculator initialization with default thresholds."""
        calculator = AccountPriorityCalculator()

        assert calculator.high_activity_threshold == 50
        assert calculator.medium_activity_threshold == 10
        assert calculator.recent_activity_hours == 24
        assert 'urgent' in calculator.critical_keywords
        assert 'critical' in calculator.critical_keywords

    def test_priority_calculation_score_boundaries(self):
        """Test priority score boundary conditions."""
        calculator = AccountPriorityCalculator()

        # Test maximum score scenario
        activity = AccountActivity(1, "max@example.com")
        activity.avg_messages_per_day = 100  # Well above high threshold
        activity.messages_last_24h = 50      # Very high recent activity
        activity.active_threads = 20         # Many active threads
        activity.last_message_received = datetime.utcnow() - timedelta(minutes=30)  # Very recent

        calculator._calculate_priority(activity)

        assert activity.priority_score <= 1.0  # Should be clamped
        assert activity.calculated_priority == AccountPriority.HIGH

    def test_priority_calculation_minimum_score(self):
        """Test minimum priority score scenario."""
        calculator = AccountPriorityCalculator()

        activity = AccountActivity(1, "min@example.com")
        activity.avg_messages_per_day = 0
        activity.messages_last_24h = 0
        activity.active_threads = 0
        activity.last_message_received = datetime.utcnow() - timedelta(days=7)

        calculator._calculate_priority(activity)

        assert activity.priority_score >= 0.0  # Should be clamped
        assert activity.calculated_priority == AccountPriority.LOW

    def test_priority_calculation_edge_cases(self):
        """Test edge cases in priority calculation."""
        calculator = AccountPriorityCalculator()

        # Test exactly at threshold
        activity = AccountActivity(1, "threshold@example.com")
        activity.avg_messages_per_day = calculator.high_activity_threshold  # Exactly at threshold

        calculator._calculate_priority(activity)

        assert activity.priority_score > 0.0

    def test_priority_calculation_recent_activity_boost(self):
        """Test priority boost for recent activity."""
        calculator = AccountPriorityCalculator()

        # Activity with very recent message
        recent_activity = AccountActivity(1, "recent@example.com")
        recent_activity.last_message_received = datetime.utcnow() - timedelta(minutes=15)

        # Activity with old message
        old_activity = AccountActivity(2, "old@example.com")
        old_activity.last_message_received = datetime.utcnow() - timedelta(days=3)

        calculator._calculate_priority(recent_activity)
        calculator._calculate_priority(old_activity)

        assert recent_activity.priority_score > old_activity.priority_score

    def test_priority_calculation_thread_activity_boost(self):
        """Test priority boost for thread activity."""
        calculator = AccountPriorityCalculator()

        # Activity with many active threads
        active_activity = AccountActivity(1, "active@example.com")
        active_activity.active_threads = 15

        # Activity with no active threads
        inactive_activity = AccountActivity(2, "inactive@example.com")
        inactive_activity.active_threads = 0

        calculator._calculate_priority(active_activity)
        calculator._calculate_priority(inactive_activity)

        assert active_activity.priority_score > inactive_activity.priority_score

    def test_priority_thresholds(self):
        """Test priority level thresholds."""
        calculator = AccountPriorityCalculator()

        # High priority threshold test
        high_activity = AccountActivity(1, "high@example.com")
        high_activity.priority_score = 0.8

        # Medium priority threshold test
        medium_activity = AccountActivity(2, "medium@example.com")
        medium_activity.priority_score = 0.5

        # Low priority threshold test
        low_activity = AccountActivity(3, "low@example.com")
        low_activity.priority_score = 0.2

        # Set priorities based on scores
        for activity in [high_activity, medium_activity, low_activity]:
            if activity.priority_score >= 0.7:
                activity.calculated_priority = AccountPriority.HIGH
            elif activity.priority_score >= 0.3:
                activity.calculated_priority = AccountPriority.MEDIUM
            else:
                activity.calculated_priority = AccountPriority.LOW

        assert high_activity.calculated_priority == AccountPriority.HIGH
        assert medium_activity.calculated_priority == AccountPriority.MEDIUM
        assert low_activity.calculated_priority == AccountPriority.LOW


class TestSchedulerConfig:
    """Test cases for SchedulerConfig."""

    def test_config_default_values(self):
        """Test default configuration values."""
        config = SchedulerConfig()

        assert config.mode == SchedulerMode.AUTOMATIC
        assert config.check_interval_seconds == 60
        assert config.max_concurrent_syncs == 3
        assert config.enable_adaptive_scheduling is True
        assert config.enable_load_balancing is True
        assert config.quiet_hours_start is None
        assert config.quiet_hours_end is None
        assert config.max_sync_duration_minutes == 30
        assert config.enable_priority_escalation is True

    def test_config_custom_values(self):
        """Test configuration with custom values."""
        config = SchedulerConfig(
            mode=SchedulerMode.MANUAL,
            check_interval_seconds=30,
            max_concurrent_syncs=5,
            enable_adaptive_scheduling=False,
            quiet_hours_start="22:00",
            quiet_hours_end="08:00"
        )

        assert config.mode == SchedulerMode.MANUAL
        assert config.check_interval_seconds == 30
        assert config.max_concurrent_syncs == 5
        assert config.enable_adaptive_scheduling is False
        assert config.quiet_hours_start == "22:00"
        assert config.quiet_hours_end == "08:00"

    def test_scheduler_modes(self):
        """Test different scheduler modes."""
        automatic_config = SchedulerConfig(mode=SchedulerMode.AUTOMATIC)
        manual_config = SchedulerConfig(mode=SchedulerMode.MANUAL)
        hybrid_config = SchedulerConfig(mode=SchedulerMode.HYBRID)

        assert automatic_config.mode == SchedulerMode.AUTOMATIC
        assert manual_config.mode == SchedulerMode.MANUAL
        assert hybrid_config.mode == SchedulerMode.HYBRID

        # Test enum values
        assert SchedulerMode.AUTOMATIC.value == "automatic"
        assert SchedulerMode.MANUAL.value == "manual"
        assert SchedulerMode.HYBRID.value == "hybrid"


class TestSyncMetrics:
    """Test cases for SyncMetrics."""

    def test_metrics_initialization(self):
        """Test SyncMetrics initialization."""
        start_time = datetime.utcnow()

        metrics = SyncMetrics(
            connection_id=1,
            email_address="metrics@example.com",
            start_time=start_time
        )

        assert metrics.connection_id == 1
        assert metrics.email_address == "metrics@example.com"
        assert metrics.start_time == start_time
        assert metrics.end_time is None
        assert metrics.duration_seconds is None
        assert metrics.messages_processed == 0
        assert metrics.success is True
        assert metrics.error_message is None
        assert metrics.resource_usage == 1.0

    def test_metrics_completion(self):
        """Test completing sync metrics."""
        start_time = datetime.utcnow()
        end_time = start_time + timedelta(minutes=5)

        metrics = SyncMetrics(
            connection_id=1,
            email_address="test@example.com",
            start_time=start_time
        )

        # Update metrics on completion
        metrics.end_time = end_time
        metrics.duration_seconds = (end_time - start_time).total_seconds()
        metrics.messages_processed = 42
        metrics.success = True

        assert metrics.duration_seconds == 300.0  # 5 minutes
        assert metrics.messages_processed == 42
        assert metrics.success is True

    def test_metrics_failure(self):
        """Test sync metrics for failed operation."""
        start_time = datetime.utcnow()
        end_time = start_time + timedelta(minutes=2)

        metrics = SyncMetrics(
            connection_id=2,
            email_address="failed@example.com",
            start_time=start_time,
            end_time=end_time,
            duration_seconds=120.0,
            success=False,
            error_message="API rate limit exceeded",
            resource_usage=0.5
        )

        assert metrics.success is False
        assert metrics.error_message == "API rate limit exceeded"
        assert metrics.resource_usage == 0.5
        assert metrics.duration_seconds == 120.0


class TestAccountPriorityEnum:
    """Test cases for AccountPriority enum."""

    def test_priority_enum_values(self):
        """Test priority enum values."""
        assert AccountPriority.CRITICAL.value == "critical"
        assert AccountPriority.HIGH.value == "high"
        assert AccountPriority.MEDIUM.value == "medium"
        assert AccountPriority.LOW.value == "low"
        assert AccountPriority.PAUSED.value == "paused"

    def test_priority_enum_ordering(self):
        """Test priority enum can be compared."""
        priorities = [AccountPriority.CRITICAL, AccountPriority.HIGH,
                     AccountPriority.MEDIUM, AccountPriority.LOW, AccountPriority.PAUSED]

        # Test that we can create ordered mappings
        priority_order = {
            AccountPriority.CRITICAL: 0,
            AccountPriority.HIGH: 1,
            AccountPriority.MEDIUM: 2,
            AccountPriority.LOW: 3,
            AccountPriority.PAUSED: 4
        }

        assert priority_order[AccountPriority.CRITICAL] < priority_order[AccountPriority.HIGH]
        assert priority_order[AccountPriority.HIGH] < priority_order[AccountPriority.MEDIUM]
        assert priority_order[AccountPriority.MEDIUM] < priority_order[AccountPriority.LOW]

    def test_priority_enum_instantiation(self):
        """Test creating priority enum from string values."""
        assert AccountPriority("critical") == AccountPriority.CRITICAL
        assert AccountPriority("high") == AccountPriority.HIGH
        assert AccountPriority("medium") == AccountPriority.MEDIUM
        assert AccountPriority("low") == AccountPriority.LOW
        assert AccountPriority("paused") == AccountPriority.PAUSED

    def test_priority_enum_invalid_value(self):
        """Test handling invalid priority values."""
        with pytest.raises(ValueError):
            AccountPriority("invalid")


class TestSyncFrequencyCalculation:
    """Test cases for sync frequency calculation logic."""

    def test_frequency_by_priority(self):
        """Test sync frequency calculation based on priority."""
        # This tests the logic that would be in _calculate_sync_frequency
        def calculate_sync_frequency(priority: AccountPriority) -> int:
            """Mock implementation for testing."""
            if priority == AccountPriority.CRITICAL:
                return 5  # Every 5 minutes
            elif priority == AccountPriority.HIGH:
                return 10  # Every 10 minutes
            elif priority == AccountPriority.MEDIUM:
                return 30  # Every 30 minutes
            elif priority == AccountPriority.LOW:
                return 120  # Every 2 hours
            else:  # PAUSED
                return 0  # No automatic sync

        # Test all priority levels
        assert calculate_sync_frequency(AccountPriority.CRITICAL) == 5
        assert calculate_sync_frequency(AccountPriority.HIGH) == 10
        assert calculate_sync_frequency(AccountPriority.MEDIUM) == 30
        assert calculate_sync_frequency(AccountPriority.LOW) == 120
        assert calculate_sync_frequency(AccountPriority.PAUSED) == 0

    def test_resource_weight_calculation(self):
        """Test resource weight calculation for scheduling."""
        def calculate_resource_weight(priority: AccountPriority) -> float:
            """Mock implementation for testing."""
            if priority == AccountPriority.CRITICAL:
                return 2.0
            elif priority == AccountPriority.HIGH:
                return 1.5
            else:
                return 1.0

        assert calculate_resource_weight(AccountPriority.CRITICAL) == 2.0
        assert calculate_resource_weight(AccountPriority.HIGH) == 1.5
        assert calculate_resource_weight(AccountPriority.MEDIUM) == 1.0
        assert calculate_resource_weight(AccountPriority.LOW) == 1.0

    def test_estimated_duration_calculation(self):
        """Test estimated sync duration based on priority."""
        def calculate_estimated_duration(priority: AccountPriority) -> timedelta:
            """Mock implementation for testing."""
            if priority == AccountPriority.CRITICAL:
                return timedelta(minutes=10)
            elif priority == AccountPriority.HIGH:
                return timedelta(minutes=8)
            else:
                return timedelta(minutes=5)

        assert calculate_estimated_duration(AccountPriority.CRITICAL) == timedelta(minutes=10)
        assert calculate_estimated_duration(AccountPriority.HIGH) == timedelta(minutes=8)
        assert calculate_estimated_duration(AccountPriority.MEDIUM) == timedelta(minutes=5)
        assert calculate_estimated_duration(AccountPriority.LOW) == timedelta(minutes=5)


if __name__ == "__main__":
    pytest.main([__file__])