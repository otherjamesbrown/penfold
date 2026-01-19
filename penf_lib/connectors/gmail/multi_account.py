"""Multi-account Gmail management with intelligent prioritization."""

import asyncio
import logging
from datetime import datetime, timedelta
from typing import Dict, List, Optional, Tuple, Any
from dataclasses import dataclass, field
from enum import Enum

from sqlalchemy import select, func, and_, desc
from sqlalchemy.ext.asyncio import AsyncSession

from ...models.gmail_connection import GmailConnection
from ...models.email_message import EmailMessage, EmailThread
from ...storage.database import get_db_session
from .sync import GmailSyncManager
from .client import GmailClient
from .auth import GmailAuthenticator

logger = logging.getLogger(__name__)


class AccountPriority(Enum):
    """Account priority levels."""
    CRITICAL = "critical"    # Always high priority
    HIGH = "high"           # High activity, important emails
    MEDIUM = "medium"       # Normal activity
    LOW = "low"            # Low activity, less frequent sync
    PAUSED = "paused"      # Temporarily disabled


@dataclass
class AccountActivity:
    """Account activity metrics."""
    connection_id: int
    email_address: str

    # Message volume metrics
    messages_last_24h: int = 0
    messages_last_7d: int = 0
    messages_last_30d: int = 0

    # Thread activity
    active_threads: int = 0
    threads_last_7d: int = 0

    # Timing metrics
    last_message_received: Optional[datetime] = None
    last_sync_completed: Optional[datetime] = None
    avg_messages_per_day: float = 0.0

    # Priority calculation
    calculated_priority: AccountPriority = AccountPriority.MEDIUM
    priority_score: float = 0.5
    user_override_priority: Optional[AccountPriority] = None

    # Sync configuration
    sync_frequency_minutes: int = 15
    next_sync_time: Optional[datetime] = None
    is_sync_enabled: bool = True

    def get_effective_priority(self) -> AccountPriority:
        """Get the effective priority (user override takes precedence)."""
        return self.user_override_priority or self.calculated_priority


@dataclass
class SyncScheduleEntry:
    """Scheduled sync entry."""
    connection_id: int
    email_address: str
    priority: AccountPriority
    scheduled_time: datetime
    estimated_duration: timedelta = field(default_factory=lambda: timedelta(minutes=5))
    resource_weight: float = 1.0


class AccountPriorityCalculator:
    """Calculates account priority based on activity metrics."""

    def __init__(self):
        """Initialize priority calculator."""
        # Priority thresholds
        self.high_activity_threshold = 50  # messages per day
        self.medium_activity_threshold = 10  # messages per day
        self.recent_activity_hours = 24
        self.critical_keywords = ['urgent', 'asap', 'critical', 'emergency']

    async def calculate_activity_metrics(self, connection_id: int) -> AccountActivity:
        """Calculate activity metrics for an account.

        Args:
            connection_id: Gmail connection ID

        Returns:
            AccountActivity with calculated metrics
        """
        async with get_db_session() as session:
            # Get connection info
            connection = await session.get(GmailConnection, connection_id)
            if not connection:
                raise ValueError(f"Connection {connection_id} not found")

            # Calculate message counts
            now = datetime.utcnow()
            day_ago = now - timedelta(days=1)
            week_ago = now - timedelta(days=7)
            month_ago = now - timedelta(days=30)

            # Messages in different time periods
            messages_24h = await session.scalar(
                select(func.count(EmailMessage.id))
                .where(
                    and_(
                        EmailMessage.connection_id == connection_id,
                        EmailMessage.internal_date >= day_ago
                    )
                )
            ) or 0

            messages_7d = await session.scalar(
                select(func.count(EmailMessage.id))
                .where(
                    and_(
                        EmailMessage.connection_id == connection_id,
                        EmailMessage.internal_date >= week_ago
                    )
                )
            ) or 0

            messages_30d = await session.scalar(
                select(func.count(EmailMessage.id))
                .where(
                    and_(
                        EmailMessage.connection_id == connection_id,
                        EmailMessage.internal_date >= month_ago
                    )
                )
            ) or 0

            # Active threads (threads with messages in last 7 days)
            active_threads = await session.scalar(
                select(func.count(func.distinct(EmailThread.id)))
                .join(EmailMessage, EmailMessage.thread_id == EmailThread.id)
                .where(
                    and_(
                        EmailMessage.connection_id == connection_id,
                        EmailMessage.internal_date >= week_ago
                    )
                )
            ) or 0

            # Total threads in last 7 days
            threads_7d = await session.scalar(
                select(func.count(EmailThread.id))
                .where(
                    and_(
                        EmailThread.connection_id == connection_id,
                        EmailThread.last_message_date >= week_ago
                    )
                )
            ) or 0

            # Last message received
            last_message = await session.scalar(
                select(EmailMessage.internal_date)
                .where(EmailMessage.connection_id == connection_id)
                .order_by(desc(EmailMessage.internal_date))
                .limit(1)
            )

            # Calculate average messages per day
            avg_messages_per_day = messages_30d / 30.0 if messages_30d > 0 else 0.0

            # Create activity object
            activity = AccountActivity(
                connection_id=connection_id,
                email_address=connection.email_address,
                messages_last_24h=messages_24h,
                messages_last_7d=messages_7d,
                messages_last_30d=messages_30d,
                active_threads=active_threads,
                threads_last_7d=threads_7d,
                last_message_received=last_message,
                last_sync_completed=connection.last_sync_completed,
                avg_messages_per_day=avg_messages_per_day
            )

            # Calculate priority and score
            self._calculate_priority(activity)

            return activity

    def _calculate_priority(self, activity: AccountActivity) -> None:
        """Calculate priority and score for an account.

        Args:
            activity: AccountActivity to update with calculated values
        """
        score = 0.0

        # Base score from message volume
        if activity.avg_messages_per_day >= self.high_activity_threshold:
            score += 0.4
        elif activity.avg_messages_per_day >= self.medium_activity_threshold:
            score += 0.2

        # Recent activity boost
        if activity.messages_last_24h > 0:
            score += 0.2

        if activity.messages_last_24h > 10:
            score += 0.1

        # Active thread boost
        if activity.active_threads > 5:
            score += 0.1
        elif activity.active_threads > 0:
            score += 0.05

        # Recency boost
        if activity.last_message_received:
            hours_since_last = (datetime.utcnow() - activity.last_message_received).total_seconds() / 3600
            if hours_since_last < 1:
                score += 0.2
            elif hours_since_last < 6:
                score += 0.1
            elif hours_since_last < 24:
                score += 0.05

        # Clamp score to 0-1 range
        activity.priority_score = max(0.0, min(1.0, score))

        # Determine priority level
        if activity.priority_score >= 0.7:
            activity.calculated_priority = AccountPriority.HIGH
        elif activity.priority_score >= 0.3:
            activity.calculated_priority = AccountPriority.MEDIUM
        else:
            activity.calculated_priority = AccountPriority.LOW

    async def analyze_message_importance(self, connection_id: int, hours_back: int = 24) -> float:
        """Analyze importance of recent messages.

        Args:
            connection_id: Gmail connection ID
            hours_back: Hours to look back for messages

        Returns:
            Importance score 0-1
        """
        async with get_db_session() as session:
            cutoff_time = datetime.utcnow() - timedelta(hours=hours_back)

            # Get recent messages
            result = await session.execute(
                select(EmailMessage.subject, EmailMessage.body_text)
                .where(
                    and_(
                        EmailMessage.connection_id == connection_id,
                        EmailMessage.internal_date >= cutoff_time
                    )
                )
                .limit(50)  # Analyze up to 50 recent messages
            )

            messages = result.fetchall()
            if not messages:
                return 0.0

            importance_score = 0.0
            total_messages = len(messages)

            for subject, body in messages:
                message_text = (subject or '') + ' ' + (body or '')
                message_text = message_text.lower()

                # Check for critical keywords
                critical_found = any(keyword in message_text for keyword in self.critical_keywords)
                if critical_found:
                    importance_score += 1.0

                # Check for other importance indicators
                if any(indicator in message_text for indicator in ['deadline', 'tomorrow', 'today', 'meeting']):
                    importance_score += 0.5

                if any(indicator in message_text for indicator in ['please', 'thanks', 'follow up']):
                    importance_score += 0.2

            # Normalize by message count
            return min(1.0, importance_score / total_messages)


class MultiAccountManager:
    """Manages multiple Gmail accounts with intelligent prioritization."""

    def __init__(self, max_concurrent_syncs: int = 3):
        """Initialize multi-account manager.

        Args:
            max_concurrent_syncs: Maximum concurrent sync operations
        """
        self.max_concurrent_syncs = max_concurrent_syncs
        self.priority_calculator = AccountPriorityCalculator()
        self.sync_semaphore = asyncio.Semaphore(max_concurrent_syncs)
        self.account_activities: Dict[int, AccountActivity] = {}
        self.sync_schedule: List[SyncScheduleEntry] = []
        self._running_syncs: Dict[int, asyncio.Task] = {}
        self._last_refresh = datetime.min

    async def get_all_accounts(self) -> List[GmailConnection]:
        """Get all active Gmail connections.

        Returns:
            List of active Gmail connections
        """
        async with get_db_session() as session:
            result = await session.execute(
                select(GmailConnection)
                .where(GmailConnection.is_active == True)
                .order_by(GmailConnection.email_address)
            )
            return list(result.scalars().all())

    async def refresh_account_activities(self, force: bool = False) -> None:
        """Refresh activity metrics for all accounts.

        Args:
            force: Force refresh even if recently updated
        """
        if not force and (datetime.utcnow() - self._last_refresh).total_seconds() < 300:
            return  # Don't refresh more than once every 5 minutes

        accounts = await self.get_all_accounts()

        for connection in accounts:
            try:
                activity = await self.priority_calculator.calculate_activity_metrics(
                    connection.id
                )
                self.account_activities[connection.id] = activity
                logger.debug(f"Updated activity for {activity.email_address}: {activity.calculated_priority}")
            except Exception as e:
                logger.error(f"Failed to calculate activity for connection {connection.id}: {e}")

        self._last_refresh = datetime.utcnow()

    async def set_account_priority(self, connection_id: int, priority: AccountPriority) -> bool:
        """Set user override priority for an account.

        Args:
            connection_id: Gmail connection ID
            priority: Priority to set

        Returns:
            True if successful
        """
        if connection_id not in self.account_activities:
            await self.refresh_account_activities()

        if connection_id in self.account_activities:
            self.account_activities[connection_id].user_override_priority = priority
            logger.info(f"Set priority override for connection {connection_id}: {priority}")
            return True

        return False

    async def get_account_status(self) -> Dict[int, Dict[str, Any]]:
        """Get status of all accounts.

        Returns:
            Dictionary mapping connection_id to status info
        """
        await self.refresh_account_activities()

        status = {}
        for connection_id, activity in self.account_activities.items():
            status[connection_id] = {
                'email_address': activity.email_address,
                'priority': activity.get_effective_priority().value,
                'priority_score': activity.priority_score,
                'messages_last_24h': activity.messages_last_24h,
                'messages_last_7d': activity.messages_last_7d,
                'active_threads': activity.active_threads,
                'last_message_received': activity.last_message_received,
                'last_sync_completed': activity.last_sync_completed,
                'sync_frequency_minutes': activity.sync_frequency_minutes,
                'next_sync_time': activity.next_sync_time,
                'is_running': connection_id in self._running_syncs,
                'sync_enabled': activity.is_sync_enabled
            }

        return status

    def _calculate_sync_frequency(self, activity: AccountActivity) -> int:
        """Calculate sync frequency in minutes based on activity.

        Args:
            activity: Account activity metrics

        Returns:
            Sync frequency in minutes
        """
        priority = activity.get_effective_priority()

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

    async def generate_sync_schedule(self, hours_ahead: int = 2) -> List[SyncScheduleEntry]:
        """Generate sync schedule for the next few hours.

        Args:
            hours_ahead: Hours to schedule ahead

        Returns:
            List of scheduled sync entries
        """
        await self.refresh_account_activities()

        schedule = []
        now = datetime.utcnow()
        end_time = now + timedelta(hours=hours_ahead)

        for connection_id, activity in self.account_activities.items():
            if not activity.is_sync_enabled:
                continue

            frequency_minutes = self._calculate_sync_frequency(activity)
            if frequency_minutes == 0:  # Paused
                continue

            # Calculate next sync times
            last_sync = activity.last_sync_completed or (now - timedelta(hours=1))
            next_sync = last_sync + timedelta(minutes=frequency_minutes)

            # Schedule all syncs within the time window
            current_time = max(next_sync, now)
            while current_time <= end_time:
                # Estimate sync duration based on priority and recent activity
                if activity.get_effective_priority() == AccountPriority.CRITICAL:
                    duration = timedelta(minutes=10)
                    weight = 2.0
                elif activity.get_effective_priority() == AccountPriority.HIGH:
                    duration = timedelta(minutes=8)
                    weight = 1.5
                else:
                    duration = timedelta(minutes=5)
                    weight = 1.0

                entry = SyncScheduleEntry(
                    connection_id=connection_id,
                    email_address=activity.email_address,
                    priority=activity.get_effective_priority(),
                    scheduled_time=current_time,
                    estimated_duration=duration,
                    resource_weight=weight
                )

                schedule.append(entry)
                current_time += timedelta(minutes=frequency_minutes)

        # Sort by priority then time
        priority_order = {
            AccountPriority.CRITICAL: 0,
            AccountPriority.HIGH: 1,
            AccountPriority.MEDIUM: 2,
            AccountPriority.LOW: 3
        }

        schedule.sort(key=lambda x: (x.scheduled_time, priority_order.get(x.priority, 99)))

        self.sync_schedule = schedule
        return schedule

    async def start_sync(self, connection_id: int) -> bool:
        """Start sync for a specific account.

        Args:
            connection_id: Gmail connection ID

        Returns:
            True if sync started successfully
        """
        if connection_id in self._running_syncs:
            logger.warning(f"Sync already running for connection {connection_id}")
            return False

        try:
            async with get_db_session() as session:
                connection = await session.get(GmailConnection, connection_id)
                if not connection or not connection.is_active:
                    logger.error(f"Connection {connection_id} not found or inactive")
                    return False

                # Create sync task
                task = asyncio.create_task(
                    self._run_account_sync(connection)
                )
                self._running_syncs[connection_id] = task

                logger.info(f"Started sync for {connection.email_address}")
                return True

        except Exception as e:
            logger.error(f"Failed to start sync for connection {connection_id}: {e}")
            return False

    async def _run_account_sync(self, connection: GmailConnection) -> None:
        """Run sync for a single account with resource management.

        Args:
            connection: Gmail connection to sync
        """
        async with self.sync_semaphore:
            try:
                # Initialize sync components
                authenticator = GmailAuthenticator()
                gmail_client = GmailClient(
                    credentials=await authenticator.get_credentials(connection.id)
                )
                sync_manager = GmailSyncManager(
                    connection=connection,
                    gmail_client=gmail_client
                )

                # Perform sync based on activity
                activity = self.account_activities.get(connection.id)
                if activity:
                    if activity.get_effective_priority() == AccountPriority.CRITICAL:
                        # Full sync for critical accounts
                        await sync_manager.sync_historical_emails(
                            limit=1000, batch_size=50
                        )
                    else:
                        # Incremental sync for others
                        await sync_manager.sync_incremental_emails()
                else:
                    # Default incremental sync
                    await sync_manager.sync_incremental_emails()

                # Update last sync time
                async with get_db_session() as session:
                    connection = await session.get(GmailConnection, connection.id)
                    if connection:
                        connection.last_sync_completed = datetime.utcnow()
                        await session.commit()

                logger.info(f"Completed sync for {connection.email_address}")

            except Exception as e:
                logger.error(f"Sync failed for {connection.email_address}: {e}")

            finally:
                # Remove from running syncs
                self._running_syncs.pop(connection.id, None)

    async def run_scheduled_syncs(self) -> None:
        """Run syncs according to the schedule."""
        await self.generate_sync_schedule()

        now = datetime.utcnow()

        # Find syncs that should run now
        due_syncs = [
            entry for entry in self.sync_schedule
            if entry.scheduled_time <= now and entry.connection_id not in self._running_syncs
        ]

        if not due_syncs:
            return

        # Sort by priority
        priority_order = {
            AccountPriority.CRITICAL: 0,
            AccountPriority.HIGH: 1,
            AccountPriority.MEDIUM: 2,
            AccountPriority.LOW: 3
        }
        due_syncs.sort(key=lambda x: priority_order.get(x.priority, 99))

        # Start syncs up to the concurrency limit
        started_count = 0
        for sync_entry in due_syncs:
            if started_count >= self.max_concurrent_syncs:
                break

            if len(self._running_syncs) >= self.max_concurrent_syncs:
                break

            if await self.start_sync(sync_entry.connection_id):
                started_count += 1

        if started_count > 0:
            logger.info(f"Started {started_count} scheduled syncs")

    async def stop_all_syncs(self) -> None:
        """Stop all running syncs."""
        tasks_to_cancel = list(self._running_syncs.values())

        for task in tasks_to_cancel:
            task.cancel()

        if tasks_to_cancel:
            await asyncio.gather(*tasks_to_cancel, return_exceptions=True)

        self._running_syncs.clear()
        logger.info("Stopped all running syncs")

    async def get_sync_statistics(self) -> Dict[str, Any]:
        """Get sync statistics across all accounts.

        Returns:
            Dictionary with sync statistics
        """
        await self.refresh_account_activities()

        total_accounts = len(self.account_activities)
        active_accounts = sum(1 for a in self.account_activities.values() if a.is_sync_enabled)
        running_syncs = len(self._running_syncs)

        # Priority distribution
        priority_counts = {}
        total_messages_24h = 0
        total_messages_7d = 0

        for activity in self.account_activities.values():
            priority = activity.get_effective_priority()
            priority_counts[priority.value] = priority_counts.get(priority.value, 0) + 1
            total_messages_24h += activity.messages_last_24h
            total_messages_7d += activity.messages_last_7d

        return {
            'total_accounts': total_accounts,
            'active_accounts': active_accounts,
            'running_syncs': running_syncs,
            'available_sync_slots': self.max_concurrent_syncs - running_syncs,
            'priority_distribution': priority_counts,
            'total_messages_24h': total_messages_24h,
            'total_messages_7d': total_messages_7d,
            'avg_messages_per_account_24h': total_messages_24h / max(1, total_accounts),
            'last_refresh': self._last_refresh
        }