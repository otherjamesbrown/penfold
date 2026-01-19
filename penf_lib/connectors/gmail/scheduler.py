"""Gmail sync scheduling with intelligent resource management."""

import asyncio
import logging
from datetime import datetime, timedelta
from typing import Dict, List, Optional, Any
from dataclasses import dataclass
from enum import Enum

from .multi_account import MultiAccountManager, AccountPriority, SyncScheduleEntry

logger = logging.getLogger(__name__)


class SchedulerMode(Enum):
    """Scheduler operation modes."""
    AUTOMATIC = "automatic"      # Fully automatic scheduling
    MANUAL = "manual"           # Manual triggers only
    HYBRID = "hybrid"           # Automatic with manual overrides


@dataclass
class SchedulerConfig:
    """Scheduler configuration."""
    mode: SchedulerMode = SchedulerMode.AUTOMATIC
    check_interval_seconds: int = 60
    max_concurrent_syncs: int = 3
    enable_adaptive_scheduling: bool = True
    enable_load_balancing: bool = True
    quiet_hours_start: Optional[str] = None  # "23:00"
    quiet_hours_end: Optional[str] = None    # "07:00"
    max_sync_duration_minutes: int = 30
    enable_priority_escalation: bool = True


@dataclass
class SyncMetrics:
    """Sync performance metrics."""
    connection_id: int
    email_address: str
    start_time: datetime
    end_time: Optional[datetime] = None
    duration_seconds: Optional[float] = None
    messages_processed: int = 0
    success: bool = True
    error_message: Optional[str] = None
    resource_usage: float = 1.0


class AdaptiveScheduler:
    """Adaptive scheduler for Gmail sync operations."""

    def __init__(self, config: SchedulerConfig = None):
        """Initialize scheduler.

        Args:
            config: Scheduler configuration
        """
        self.config = config or SchedulerConfig()
        self.multi_account_manager = MultiAccountManager(self.config.max_concurrent_syncs)
        self.sync_metrics: List[SyncMetrics] = []
        self.is_running = False
        self._scheduler_task: Optional[asyncio.Task] = None
        self._sync_history: Dict[int, List[SyncMetrics]] = {}

    async def start(self) -> None:
        """Start the scheduler."""
        if self.is_running:
            logger.warning("Scheduler is already running")
            return

        self.is_running = True
        self._scheduler_task = asyncio.create_task(self._scheduler_loop())
        logger.info(f"Started Gmail sync scheduler in {self.config.mode.value} mode")

    async def stop(self) -> None:
        """Stop the scheduler."""
        if not self.is_running:
            return

        self.is_running = False

        if self._scheduler_task:
            self._scheduler_task.cancel()
            try:
                await self._scheduler_task
            except asyncio.CancelledError:
                pass

        await self.multi_account_manager.stop_all_syncs()
        logger.info("Stopped Gmail sync scheduler")

    async def _scheduler_loop(self) -> None:
        """Main scheduler loop."""
        while self.is_running:
            try:
                await self._scheduler_tick()
                await asyncio.sleep(self.config.check_interval_seconds)
            except asyncio.CancelledError:
                break
            except Exception as e:
                logger.error(f"Scheduler loop error: {e}")
                await asyncio.sleep(self.config.check_interval_seconds)

    async def _scheduler_tick(self) -> None:
        """Single scheduler tick."""
        if self.config.mode == SchedulerMode.MANUAL:
            return  # No automatic scheduling in manual mode

        # Check if we're in quiet hours
        if self._is_quiet_hours():
            logger.debug("Skipping sync during quiet hours")
            return

        # Run scheduled syncs
        if self.config.mode in [SchedulerMode.AUTOMATIC, SchedulerMode.HYBRID]:
            await self._run_scheduled_syncs()

        # Adaptive scheduling adjustments
        if self.config.enable_adaptive_scheduling:
            await self._adapt_scheduling()

        # Priority escalation
        if self.config.enable_priority_escalation:
            await self._check_priority_escalation()

    def _is_quiet_hours(self) -> bool:
        """Check if current time is within quiet hours.

        Returns:
            True if in quiet hours
        """
        if not self.config.quiet_hours_start or not self.config.quiet_hours_end:
            return False

        now = datetime.now()
        current_time = now.strftime("%H:%M")

        start = self.config.quiet_hours_start
        end = self.config.quiet_hours_end

        if start < end:
            # Same day (e.g., 09:00 to 17:00)
            return start <= current_time <= end
        else:
            # Across midnight (e.g., 23:00 to 07:00)
            return current_time >= start or current_time <= end

    async def _run_scheduled_syncs(self) -> None:
        """Run scheduled syncs with load balancing."""
        try:
            # Generate schedule and run due syncs
            await self.multi_account_manager.run_scheduled_syncs()

            # Load balancing - avoid overwhelming system
            if self.config.enable_load_balancing:
                await self._balance_load()

        except Exception as e:
            logger.error(f"Error running scheduled syncs: {e}")

    async def _balance_load(self) -> None:
        """Balance load across accounts and time."""
        status = await self.multi_account_manager.get_account_status()
        running_count = sum(1 for account in status.values() if account['is_running'])

        # If we're at capacity, consider adjusting frequencies
        if running_count >= self.config.max_concurrent_syncs:
            await self._adjust_sync_frequencies()

    async def _adjust_sync_frequencies(self) -> None:
        """Adjust sync frequencies based on system load."""
        # Get recent performance metrics
        recent_metrics = self._get_recent_metrics(hours=2)

        # Calculate average sync duration by priority
        avg_durations = {}
        for priority in AccountPriority:
            priority_metrics = [
                m for m in recent_metrics
                if m.duration_seconds is not None
            ]

            if priority_metrics:
                avg_duration = sum(m.duration_seconds for m in priority_metrics) / len(priority_metrics)
                avg_durations[priority] = avg_duration

        # Adjust frequencies based on performance
        for connection_id, activity in self.multi_account_manager.account_activities.items():
            priority = activity.get_effective_priority()

            if priority in avg_durations:
                avg_duration = avg_durations[priority]

                # If syncs are taking too long, reduce frequency
                if avg_duration > 300:  # More than 5 minutes
                    activity.sync_frequency_minutes = int(activity.sync_frequency_minutes * 1.5)
                elif avg_duration < 60:  # Less than 1 minute
                    activity.sync_frequency_minutes = max(5, int(activity.sync_frequency_minutes * 0.8))

    async def _adapt_scheduling(self) -> None:
        """Adapt scheduling based on historical performance."""
        # Analyze sync patterns
        patterns = await self._analyze_sync_patterns()

        # Adjust scheduling based on patterns
        for connection_id, pattern in patterns.items():
            if connection_id in self.multi_account_manager.account_activities:
                activity = self.multi_account_manager.account_activities[connection_id]

                # Adjust frequency based on success rate
                if pattern['success_rate'] < 0.8:
                    # Poor success rate, reduce frequency
                    activity.sync_frequency_minutes = int(activity.sync_frequency_minutes * 1.2)
                elif pattern['success_rate'] > 0.95 and pattern['avg_duration'] < 120:
                    # High success rate and fast syncs, can increase frequency
                    activity.sync_frequency_minutes = max(5, int(activity.sync_frequency_minutes * 0.9))

    async def _check_priority_escalation(self) -> None:
        """Check if any accounts need priority escalation."""
        status = await self.multi_account_manager.get_account_status()

        for connection_id, account_status in status.items():
            # Escalate priority if last sync was too long ago
            if account_status['last_sync_completed']:
                last_sync = account_status['last_sync_completed']
                hours_since_sync = (datetime.utcnow() - last_sync).total_seconds() / 3600

                # Escalate if sync is overdue
                max_hours = {
                    AccountPriority.CRITICAL: 0.5,   # 30 minutes
                    AccountPriority.HIGH: 1.0,      # 1 hour
                    AccountPriority.MEDIUM: 4.0,    # 4 hours
                    AccountPriority.LOW: 12.0       # 12 hours
                }

                current_priority = AccountPriority(account_status['priority'])
                max_delay = max_hours.get(current_priority, 12.0)

                if hours_since_sync > max_delay and not account_status['is_running']:
                    # Force a sync
                    await self.multi_account_manager.start_sync(connection_id)
                    logger.info(f"Escalated sync for overdue account {account_status['email_address']}")

    async def _analyze_sync_patterns(self) -> Dict[int, Dict[str, Any]]:
        """Analyze historical sync patterns.

        Returns:
            Dictionary mapping connection_id to pattern analysis
        """
        patterns = {}
        recent_metrics = self._get_recent_metrics(hours=24)

        # Group metrics by connection
        metrics_by_connection = {}
        for metric in recent_metrics:
            if metric.connection_id not in metrics_by_connection:
                metrics_by_connection[metric.connection_id] = []
            metrics_by_connection[metric.connection_id].append(metric)

        # Analyze patterns for each connection
        for connection_id, metrics in metrics_by_connection.items():
            if not metrics:
                continue

            successful_syncs = [m for m in metrics if m.success]
            success_rate = len(successful_syncs) / len(metrics)

            durations = [m.duration_seconds for m in successful_syncs if m.duration_seconds]
            avg_duration = sum(durations) / len(durations) if durations else 0

            patterns[connection_id] = {
                'total_syncs': len(metrics),
                'successful_syncs': len(successful_syncs),
                'success_rate': success_rate,
                'avg_duration': avg_duration,
                'last_sync': max(m.start_time for m in metrics),
                'error_rate': 1.0 - success_rate
            }

        return patterns

    def _get_recent_metrics(self, hours: int) -> List[SyncMetrics]:
        """Get recent sync metrics.

        Args:
            hours: Hours to look back

        Returns:
            List of recent sync metrics
        """
        cutoff = datetime.utcnow() - timedelta(hours=hours)
        return [m for m in self.sync_metrics if m.start_time >= cutoff]

    async def trigger_manual_sync(self, connection_id: int, priority_override: Optional[AccountPriority] = None) -> bool:
        """Trigger manual sync for specific account.

        Args:
            connection_id: Gmail connection ID
            priority_override: Optional priority override

        Returns:
            True if sync was triggered
        """
        if priority_override:
            await self.multi_account_manager.set_account_priority(connection_id, priority_override)

        return await self.multi_account_manager.start_sync(connection_id)

    async def pause_account(self, connection_id: int) -> bool:
        """Pause sync for specific account.

        Args:
            connection_id: Gmail connection ID

        Returns:
            True if paused successfully
        """
        return await self.multi_account_manager.set_account_priority(connection_id, AccountPriority.PAUSED)

    async def resume_account(self, connection_id: int) -> bool:
        """Resume sync for paused account.

        Args:
            connection_id: Gmail connection ID

        Returns:
            True if resumed successfully
        """
        # Remove priority override to return to calculated priority
        if connection_id in self.multi_account_manager.account_activities:
            self.multi_account_manager.account_activities[connection_id].user_override_priority = None
            return True
        return False

    async def get_scheduler_status(self) -> Dict[str, Any]:
        """Get scheduler status and statistics.

        Returns:
            Dictionary with scheduler status
        """
        account_stats = await self.multi_account_manager.get_sync_statistics()
        recent_metrics = self._get_recent_metrics(hours=1)

        return {
            'is_running': self.is_running,
            'mode': self.config.mode.value,
            'check_interval_seconds': self.config.check_interval_seconds,
            'max_concurrent_syncs': self.config.max_concurrent_syncs,
            'in_quiet_hours': self._is_quiet_hours(),
            'recent_syncs_1h': len(recent_metrics),
            'recent_success_rate': (
                len([m for m in recent_metrics if m.success]) / len(recent_metrics)
                if recent_metrics else 1.0
            ),
            'account_statistics': account_stats,
            'config': {
                'adaptive_scheduling': self.config.enable_adaptive_scheduling,
                'load_balancing': self.config.enable_load_balancing,
                'priority_escalation': self.config.enable_priority_escalation,
                'quiet_hours': {
                    'start': self.config.quiet_hours_start,
                    'end': self.config.quiet_hours_end
                }
            }
        }

    def record_sync_metric(self, metric: SyncMetrics) -> None:
        """Record sync performance metric.

        Args:
            metric: Sync metric to record
        """
        self.sync_metrics.append(metric)

        # Keep only recent metrics (last 7 days)
        cutoff = datetime.utcnow() - timedelta(days=7)
        self.sync_metrics = [m for m in self.sync_metrics if m.start_time >= cutoff]

        # Update connection-specific history
        if metric.connection_id not in self._sync_history:
            self._sync_history[metric.connection_id] = []

        self._sync_history[metric.connection_id].append(metric)

        # Keep only recent history per connection
        self._sync_history[metric.connection_id] = [
            m for m in self._sync_history[metric.connection_id]
            if m.start_time >= cutoff
        ]

    async def update_config(self, new_config: Dict[str, Any]) -> bool:
        """Update scheduler configuration.

        Args:
            new_config: New configuration values

        Returns:
            True if updated successfully
        """
        try:
            # Update config fields
            for key, value in new_config.items():
                if hasattr(self.config, key):
                    if key == 'mode' and isinstance(value, str):
                        value = SchedulerMode(value)
                    setattr(self.config, key, value)

            # Update multi-account manager if max_concurrent_syncs changed
            if 'max_concurrent_syncs' in new_config:
                self.multi_account_manager.max_concurrent_syncs = self.config.max_concurrent_syncs
                self.multi_account_manager.sync_semaphore = asyncio.Semaphore(self.config.max_concurrent_syncs)

            logger.info("Updated scheduler configuration")
            return True

        except Exception as e:
            logger.error(f"Failed to update scheduler configuration: {e}")
            return False