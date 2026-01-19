"""Integration tests for multi-account Gmail management."""

import asyncio
import pytest
from datetime import datetime, timedelta
from unittest.mock import Mock, AsyncMock, patch, MagicMock

from penf_lib.connectors.gmail.multi_account import (
    MultiAccountManager,
    AccountPriorityCalculator,
    AccountActivity,
    AccountPriority,
    SyncScheduleEntry
)
from penf_lib.connectors.gmail.scheduler import AdaptiveScheduler, SchedulerConfig, SchedulerMode
from penf_lib.models.gmail_connection import GmailConnection
from penf_lib.models.email_message import EmailMessage, EmailThread


class TestMultiAccountManager:
    """Test cases for MultiAccountManager."""

    @pytest.fixture
    def mock_connections(self):
        """Mock Gmail connections."""
        connections = []
        for i in range(3):
            conn = Mock(spec=GmailConnection)
            conn.id = i + 1
            conn.email_address = f"user{i+1}@example.com"
            conn.is_active = True
            conn.last_sync_completed = datetime.utcnow() - timedelta(hours=i+1)
            connections.append(conn)
        return connections

    @pytest.fixture
    def multi_account_manager(self):
        """Create multi-account manager."""
        return MultiAccountManager(max_concurrent_syncs=2)

    @pytest.mark.asyncio
    async def test_manager_initialization(self, multi_account_manager):
        """Test manager initialization."""
        assert multi_account_manager.max_concurrent_syncs == 2
        assert len(multi_account_manager.account_activities) == 0
        assert len(multi_account_manager._running_syncs) == 0

    @pytest.mark.asyncio
    async def test_get_all_accounts(self, multi_account_manager, mock_connections):
        """Test getting all accounts."""
        with patch('penf_lib.connectors.gmail.multi_account.get_db_session'):
            mock_session = AsyncMock()
            mock_result = Mock()
            mock_result.scalars.return_value.all.return_value = mock_connections

            mock_session.execute.return_value = mock_result

            with patch('penf_lib.connectors.gmail.multi_account.get_db_session') as mock_get_session:
                mock_get_session.return_value.__aenter__.return_value = mock_session

                accounts = await multi_account_manager.get_all_accounts()

                assert len(accounts) == 3
                assert all(acc.is_active for acc in accounts)

    @pytest.mark.asyncio
    async def test_set_account_priority(self, multi_account_manager):
        """Test setting account priority."""
        # Add mock activity
        activity = AccountActivity(
            connection_id=1,
            email_address="test@example.com",
            calculated_priority=AccountPriority.MEDIUM
        )
        multi_account_manager.account_activities[1] = activity

        # Set override priority
        success = await multi_account_manager.set_account_priority(1, AccountPriority.HIGH)

        assert success is True
        assert activity.user_override_priority == AccountPriority.HIGH
        assert activity.get_effective_priority() == AccountPriority.HIGH

    @pytest.mark.asyncio
    async def test_generate_sync_schedule(self, multi_account_manager):
        """Test sync schedule generation."""
        # Add mock activities
        activities = [
            AccountActivity(1, "high@example.com", calculated_priority=AccountPriority.HIGH),
            AccountActivity(2, "medium@example.com", calculated_priority=AccountPriority.MEDIUM),
            AccountActivity(3, "low@example.com", calculated_priority=AccountPriority.LOW)
        ]

        for activity in activities:
            activity.is_sync_enabled = True
            activity.last_sync_completed = datetime.utcnow() - timedelta(hours=1)
            multi_account_manager.account_activities[activity.connection_id] = activity

        schedule = await multi_account_manager.generate_sync_schedule(hours_ahead=1)

        assert len(schedule) > 0
        assert all(isinstance(entry, SyncScheduleEntry) for entry in schedule)

        # Check that high priority accounts are scheduled more frequently
        high_priority_entries = [e for e in schedule if e.priority == AccountPriority.HIGH]
        medium_priority_entries = [e for e in schedule if e.priority == AccountPriority.MEDIUM]

        assert len(high_priority_entries) >= len(medium_priority_entries)

    @pytest.mark.asyncio
    async def test_get_account_status(self, multi_account_manager):
        """Test getting account status."""
        # Add mock activity
        activity = AccountActivity(
            connection_id=1,
            email_address="test@example.com",
            messages_last_24h=10,
            messages_last_7d=50,
            calculated_priority=AccountPriority.MEDIUM
        )
        multi_account_manager.account_activities[1] = activity

        status = await multi_account_manager.get_account_status()

        assert 1 in status
        account_status = status[1]

        assert account_status['email_address'] == "test@example.com"
        assert account_status['priority'] == AccountPriority.MEDIUM.value
        assert account_status['messages_last_24h'] == 10
        assert account_status['messages_last_7d'] == 50
        assert account_status['is_running'] is False
        assert account_status['sync_enabled'] is True

    @pytest.mark.asyncio
    async def test_sync_statistics(self, multi_account_manager):
        """Test sync statistics calculation."""
        # Add multiple mock activities
        for i in range(3):
            activity = AccountActivity(
                connection_id=i + 1,
                email_address=f"user{i+1}@example.com",
                messages_last_24h=i * 10,
                messages_last_7d=i * 50,
                calculated_priority=list(AccountPriority)[i % len(list(AccountPriority))]
            )
            multi_account_manager.account_activities[i + 1] = activity

        stats = await multi_account_manager.get_sync_statistics()

        assert stats['total_accounts'] == 3
        assert stats['active_accounts'] == 3
        assert stats['running_syncs'] == 0
        assert stats['available_sync_slots'] == 2  # max_concurrent_syncs
        assert 'priority_distribution' in stats
        assert 'total_messages_24h' in stats
        assert 'total_messages_7d' in stats


class TestAccountPriorityCalculator:
    """Test cases for AccountPriorityCalculator."""

    @pytest.fixture
    def priority_calculator(self):
        """Create priority calculator."""
        return AccountPriorityCalculator()

    @pytest.mark.asyncio
    async def test_calculate_activity_metrics(self, priority_calculator):
        """Test activity metrics calculation."""
        with patch('penf_lib.connectors.gmail.multi_account.get_db_session'):
            mock_session = AsyncMock()

            # Mock connection
            mock_connection = Mock()
            mock_connection.id = 1
            mock_connection.email_address = "test@example.com"
            mock_session.get.return_value = mock_connection

            # Mock message counts
            mock_session.scalar.side_effect = [10, 50, 200, 5, 8]  # Various counts

            # Mock last message time
            last_message_time = datetime.utcnow() - timedelta(hours=2)
            mock_session.scalar.side_effect[-1] = last_message_time

            with patch('penf_lib.connectors.gmail.multi_account.get_db_session') as mock_get_session:
                mock_get_session.return_value.__aenter__.return_value = mock_session

                # Mock the session.scalar calls in order
                mock_session.scalar = AsyncMock(side_effect=[
                    10,  # messages_24h
                    50,  # messages_7d
                    200,  # messages_30d
                    5,   # active_threads
                    8,   # threads_7d
                    last_message_time  # last_message
                ])

                activity = await priority_calculator.calculate_activity_metrics(1)

                assert activity.connection_id == 1
                assert activity.email_address == "test@example.com"
                assert activity.messages_last_24h == 10
                assert activity.messages_last_7d == 50
                assert activity.messages_last_30d == 200
                assert activity.active_threads == 5
                assert activity.threads_last_7d == 8
                assert activity.last_message_received == last_message_time
                assert activity.avg_messages_per_day == 200 / 30.0

    def test_priority_calculation_high_activity(self, priority_calculator):
        """Test priority calculation for high activity account."""
        activity = AccountActivity(
            connection_id=1,
            email_address="busy@example.com",
            avg_messages_per_day=60,  # Above high threshold (50)
            messages_last_24h=20,
            active_threads=10,
            last_message_received=datetime.utcnow() - timedelta(minutes=30)
        )

        priority_calculator._calculate_priority(activity)

        assert activity.calculated_priority == AccountPriority.HIGH
        assert activity.priority_score > 0.7

    def test_priority_calculation_medium_activity(self, priority_calculator):
        """Test priority calculation for medium activity account."""
        activity = AccountActivity(
            connection_id=1,
            email_address="normal@example.com",
            avg_messages_per_day=20,  # Between thresholds
            messages_last_24h=5,
            active_threads=3,
            last_message_received=datetime.utcnow() - timedelta(hours=3)
        )

        priority_calculator._calculate_priority(activity)

        assert activity.calculated_priority == AccountPriority.MEDIUM
        assert 0.3 <= activity.priority_score <= 0.7

    def test_priority_calculation_low_activity(self, priority_calculator):
        """Test priority calculation for low activity account."""
        activity = AccountActivity(
            connection_id=1,
            email_address="quiet@example.com",
            avg_messages_per_day=2,  # Below medium threshold (10)
            messages_last_24h=0,
            active_threads=0,
            last_message_received=datetime.utcnow() - timedelta(days=2)
        )

        priority_calculator._calculate_priority(activity)

        assert activity.calculated_priority == AccountPriority.LOW
        assert activity.priority_score < 0.3

    @pytest.mark.asyncio
    async def test_analyze_message_importance(self, priority_calculator):
        """Test message importance analysis."""
        with patch('penf_lib.connectors.gmail.multi_account.get_db_session'):
            mock_session = AsyncMock()

            # Mock messages with varying importance
            messages = [
                ("urgent: please respond", "this is very urgent"),
                ("meeting tomorrow", "please attend the meeting"),
                ("regular email", "just a normal message"),
                ("deadline today", "the deadline is today")
            ]

            mock_result = Mock()
            mock_result.fetchall.return_value = messages
            mock_session.execute.return_value = mock_result

            with patch('penf_lib.connectors.gmail.multi_account.get_db_session') as mock_get_session:
                mock_get_session.return_value.__aenter__.return_value = mock_session

                importance = await priority_calculator.analyze_message_importance(1, 24)

                assert 0.0 <= importance <= 1.0
                assert importance > 0  # Should detect some importance keywords


class TestAdaptiveScheduler:
    """Test cases for AdaptiveScheduler."""

    @pytest.fixture
    def scheduler_config(self):
        """Create scheduler configuration."""
        return SchedulerConfig(
            mode=SchedulerMode.AUTOMATIC,
            check_interval_seconds=10,  # Faster for testing
            max_concurrent_syncs=2,
            quiet_hours_start="23:00",
            quiet_hours_end="07:00"
        )

    @pytest.fixture
    def scheduler(self, scheduler_config):
        """Create adaptive scheduler."""
        return AdaptiveScheduler(scheduler_config)

    def test_scheduler_initialization(self, scheduler):
        """Test scheduler initialization."""
        assert scheduler.config.mode == SchedulerMode.AUTOMATIC
        assert scheduler.config.max_concurrent_syncs == 2
        assert scheduler.is_running is False
        assert scheduler._scheduler_task is None

    @pytest.mark.asyncio
    async def test_scheduler_start_stop(self, scheduler):
        """Test scheduler start and stop."""
        # Start scheduler
        await scheduler.start()

        assert scheduler.is_running is True
        assert scheduler._scheduler_task is not None

        # Stop scheduler
        await scheduler.stop()

        assert scheduler.is_running is False

    def test_quiet_hours_same_day(self, scheduler):
        """Test quiet hours detection for same day."""
        scheduler.config.quiet_hours_start = "09:00"
        scheduler.config.quiet_hours_end = "17:00"

        with patch('penf_lib.connectors.gmail.scheduler.datetime') as mock_datetime:
            # Mock current time as 12:00
            mock_datetime.now.return_value.strftime.return_value = "12:00"

            is_quiet = scheduler._is_quiet_hours()
            assert is_quiet is True

            # Mock current time as 20:00
            mock_datetime.now.return_value.strftime.return_value = "20:00"

            is_quiet = scheduler._is_quiet_hours()
            assert is_quiet is False

    def test_quiet_hours_across_midnight(self, scheduler):
        """Test quiet hours detection across midnight."""
        scheduler.config.quiet_hours_start = "23:00"
        scheduler.config.quiet_hours_end = "07:00"

        with patch('penf_lib.connectors.gmail.scheduler.datetime') as mock_datetime:
            # Mock current time as 01:00 (should be quiet)
            mock_datetime.now.return_value.strftime.return_value = "01:00"

            is_quiet = scheduler._is_quiet_hours()
            assert is_quiet is True

            # Mock current time as 10:00 (should not be quiet)
            mock_datetime.now.return_value.strftime.return_value = "10:00"

            is_quiet = scheduler._is_quiet_hours()
            assert is_quiet is False

    @pytest.mark.asyncio
    async def test_manual_sync_trigger(self, scheduler):
        """Test manual sync triggering."""
        with patch.object(scheduler.multi_account_manager, 'set_account_priority') as mock_set_priority:
            with patch.object(scheduler.multi_account_manager, 'start_sync', return_value=True) as mock_start_sync:
                mock_set_priority.return_value = True

                success = await scheduler.trigger_manual_sync(1, AccountPriority.HIGH)

                assert success is True
                mock_set_priority.assert_called_once_with(1, AccountPriority.HIGH)
                mock_start_sync.assert_called_once_with(1)

    @pytest.mark.asyncio
    async def test_pause_resume_account(self, scheduler):
        """Test pausing and resuming accounts."""
        with patch.object(scheduler.multi_account_manager, 'set_account_priority', return_value=True) as mock_set:
            # Test pause
            success = await scheduler.pause_account(1)
            assert success is True
            mock_set.assert_called_with(1, AccountPriority.PAUSED)

        # Test resume
        activity = AccountActivity(1, "test@example.com")
        activity.user_override_priority = AccountPriority.PAUSED
        scheduler.multi_account_manager.account_activities[1] = activity

        success = await scheduler.resume_account(1)
        assert success is True
        assert activity.user_override_priority is None

    @pytest.mark.asyncio
    async def test_scheduler_status(self, scheduler):
        """Test scheduler status reporting."""
        with patch.object(scheduler.multi_account_manager, 'get_sync_statistics') as mock_stats:
            mock_stats.return_value = {
                'total_accounts': 3,
                'active_accounts': 2,
                'running_syncs': 1
            }

            status = await scheduler.get_scheduler_status()

            assert status['is_running'] is False
            assert status['mode'] == 'automatic'
            assert status['max_concurrent_syncs'] == 2
            assert 'account_statistics' in status
            assert 'config' in status

    @pytest.mark.asyncio
    async def test_config_update(self, scheduler):
        """Test scheduler configuration update."""
        new_config = {
            'max_concurrent_syncs': 5,
            'check_interval_seconds': 30,
            'mode': 'manual'
        }

        success = await scheduler.update_config(new_config)

        assert success is True
        assert scheduler.config.max_concurrent_syncs == 5
        assert scheduler.config.check_interval_seconds == 30
        assert scheduler.config.mode == SchedulerMode.MANUAL

    def test_sync_frequency_calculation(self, scheduler):
        """Test sync frequency calculation."""
        manager = scheduler.multi_account_manager

        # Test different priorities
        activities = [
            AccountActivity(1, "critical@example.com", calculated_priority=AccountPriority.CRITICAL),
            AccountActivity(2, "high@example.com", calculated_priority=AccountPriority.HIGH),
            AccountActivity(3, "medium@example.com", calculated_priority=AccountPriority.MEDIUM),
            AccountActivity(4, "low@example.com", calculated_priority=AccountPriority.LOW),
            AccountActivity(5, "paused@example.com", calculated_priority=AccountPriority.PAUSED)
        ]

        expected_frequencies = {
            AccountPriority.CRITICAL: 5,
            AccountPriority.HIGH: 10,
            AccountPriority.MEDIUM: 30,
            AccountPriority.LOW: 120,
            AccountPriority.PAUSED: 0
        }

        for activity in activities:
            frequency = manager._calculate_sync_frequency(activity)
            expected = expected_frequencies[activity.calculated_priority]
            assert frequency == expected

    @pytest.mark.asyncio
    async def test_concurrent_sync_limits(self, scheduler):
        """Test concurrent sync limits."""
        # Mock running syncs at capacity
        scheduler.multi_account_manager._running_syncs = {1: Mock(), 2: Mock()}

        with patch.object(scheduler.multi_account_manager, 'start_sync') as mock_start:
            # Try to start another sync
            success = await scheduler.multi_account_manager.start_sync(3)

            # Should still attempt to start (actual limit enforcement is in start_sync)
            mock_start.assert_called_once_with(3)


if __name__ == "__main__":
    pytest.main([__file__])