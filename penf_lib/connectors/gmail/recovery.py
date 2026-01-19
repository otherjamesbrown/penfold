"""Gmail Integration Recovery Procedures.

This module provides automated recovery procedures for various failure scenarios
in the Gmail integration system, including OAuth token recovery, sync recovery,
webhook recovery, and system health restoration.
"""

import asyncio
import logging
from datetime import datetime, timedelta
from typing import Dict, List, Optional, Any, Tuple
from enum import Enum
from dataclasses import dataclass
import json

from sqlalchemy import select, and_, or_
from sqlalchemy.ext.asyncio import AsyncSession

from .error_handling import ErrorCategory, ErrorSeverity, gmail_error_handler, with_error_handling, RetryConfig
from .auth import GmailAuthenticator
from ...models.gmail_connection import GmailConnection
from ...models.email_message import EmailMessage, EmailThread
from ...storage.database import db_manager


logger = logging.getLogger(__name__)


class RecoveryStatus(Enum):
    """Recovery operation status."""
    PENDING = "pending"
    IN_PROGRESS = "in_progress"
    COMPLETED = "completed"
    FAILED = "failed"
    SKIPPED = "skipped"


@dataclass
class RecoveryResult:
    """Result of a recovery operation."""
    status: RecoveryStatus
    message: str
    actions_taken: List[str]
    metadata: Dict[str, Any]
    duration_seconds: Optional[float] = None
    errors: List[str] = None

    def __post_init__(self):
        if self.errors is None:
            self.errors = []


class OAuthTokenRecovery:
    """Handles OAuth token recovery and re-authentication."""

    def __init__(self, authenticator: GmailAuthenticator):
        self.authenticator = authenticator

    @with_error_handling("oauth_token_recovery")
    async def recover_connection_tokens(self, connection_id: int) -> RecoveryResult:
        """Recover OAuth tokens for a specific connection.

        Args:
            connection_id: Gmail connection ID

        Returns:
            RecoveryResult with operation details
        """
        start_time = datetime.utcnow()
        actions_taken = []
        errors = []

        try:
            async with db_manager.get_session() as session:
                # Get connection
                result = await session.execute(
                    select(GmailConnection).where(GmailConnection.id == connection_id)
                )
                connection = result.scalar_one_or_none()

                if not connection:
                    return RecoveryResult(
                        status=RecoveryStatus.FAILED,
                        message=f"Connection {connection_id} not found",
                        actions_taken=actions_taken,
                        metadata={"connection_id": connection_id}
                    )

                if not connection.encrypted_credentials:
                    return RecoveryResult(
                        status=RecoveryStatus.FAILED,
                        message=f"No stored credentials for connection {connection_id}",
                        actions_taken=actions_taken,
                        metadata={"connection_id": connection_id}
                    )

                # Decrypt existing credentials
                try:
                    credentials = await self.authenticator.decrypt_credentials(connection.encrypted_credentials)
                    actions_taken.append("decrypted_stored_credentials")
                except Exception as e:
                    errors.append(f"Failed to decrypt credentials: {str(e)}")
                    return RecoveryResult(
                        status=RecoveryStatus.FAILED,
                        message=f"Could not decrypt stored credentials: {str(e)}",
                        actions_taken=actions_taken,
                        metadata={"connection_id": connection_id},
                        errors=errors
                    )

                # Check if credentials are valid
                is_valid = await self.authenticator.validate_credentials(credentials)

                if is_valid:
                    # Credentials are already valid
                    actions_taken.append("validated_existing_credentials")
                    return RecoveryResult(
                        status=RecoveryStatus.SKIPPED,
                        message="Credentials are already valid",
                        actions_taken=actions_taken,
                        metadata={"connection_id": connection_id}
                    )

                # Attempt to refresh token
                if credentials.refresh_token:
                    try:
                        refreshed_credentials = await self.authenticator.refresh_credentials(credentials)
                        actions_taken.append("refreshed_access_token")

                        # Validate refreshed credentials
                        if await self.authenticator.validate_credentials(refreshed_credentials):
                            # Update stored credentials
                            connection.encrypted_credentials = await self.authenticator.encrypt_credentials(refreshed_credentials)
                            connection.token_refresh_count = (connection.token_refresh_count or 0) + 1
                            connection.last_token_refresh = datetime.utcnow()
                            await session.commit()

                            actions_taken.append("updated_stored_credentials")

                            duration = (datetime.utcnow() - start_time).total_seconds()
                            return RecoveryResult(
                                status=RecoveryStatus.COMPLETED,
                                message="Successfully refreshed OAuth token",
                                actions_taken=actions_taken,
                                metadata={
                                    "connection_id": connection_id,
                                    "refresh_count": connection.token_refresh_count
                                },
                                duration_seconds=duration
                            )
                        else:
                            errors.append("Refreshed token is still invalid")

                    except Exception as e:
                        errors.append(f"Token refresh failed: {str(e)}")

                # Token refresh failed, mark connection as needing re-authentication
                connection.is_active = False
                connection.auth_status = "requires_reauth"
                connection.auth_error_message = "Token refresh failed - user re-authentication required"
                connection.auth_error_timestamp = datetime.utcnow()
                await session.commit()

                actions_taken.append("marked_for_reauth")

                return RecoveryResult(
                    status=RecoveryStatus.FAILED,
                    message="Token refresh failed - user re-authentication required",
                    actions_taken=actions_taken,
                    metadata={
                        "connection_id": connection_id,
                        "requires_user_action": True
                    },
                    errors=errors
                )

        except Exception as e:
            logger.error(f"OAuth token recovery failed for connection {connection_id}: {e}")
            return RecoveryResult(
                status=RecoveryStatus.FAILED,
                message=f"Recovery operation failed: {str(e)}",
                actions_taken=actions_taken,
                metadata={"connection_id": connection_id},
                errors=errors + [str(e)]
            )

    @with_error_handling("batch_token_recovery")
    async def recover_all_invalid_tokens(self) -> List[RecoveryResult]:
        """Recover OAuth tokens for all connections with invalid credentials.

        Returns:
            List of RecoveryResult for each connection
        """
        results = []

        try:
            async with db_manager.get_session() as session:
                # Find connections that might need token recovery
                result = await session.execute(
                    select(GmailConnection).where(
                        and_(
                            GmailConnection.is_active == True,
                            or_(
                                GmailConnection.auth_status.in_(["token_expired", "refresh_failed"]),
                                GmailConnection.last_token_refresh < datetime.utcnow() - timedelta(hours=1)
                            )
                        )
                    )
                )
                connections = result.scalars().all()

                logger.info(f"Found {len(connections)} connections potentially needing token recovery")

                for connection in connections:
                    try:
                        result = await self.recover_connection_tokens(connection.id)
                        results.append(result)

                        # Add delay between recovery attempts to avoid overwhelming the auth service
                        await asyncio.sleep(1)

                    except Exception as e:
                        logger.error(f"Failed to recover tokens for connection {connection.id}: {e}")
                        results.append(RecoveryResult(
                            status=RecoveryStatus.FAILED,
                            message=f"Recovery failed: {str(e)}",
                            actions_taken=[],
                            metadata={"connection_id": connection.id},
                            errors=[str(e)]
                        ))

                return results

        except Exception as e:
            logger.error(f"Batch token recovery failed: {e}")
            return [RecoveryResult(
                status=RecoveryStatus.FAILED,
                message=f"Batch recovery failed: {str(e)}",
                actions_taken=[],
                metadata={},
                errors=[str(e)]
            )]


class SyncRecovery:
    """Handles sync operation recovery and data consistency restoration."""

    @with_error_handling("sync_recovery")
    async def recover_interrupted_sync(self, connection_id: int) -> RecoveryResult:
        """Recover from interrupted sync operations.

        Args:
            connection_id: Gmail connection ID

        Returns:
            RecoveryResult with recovery details
        """
        start_time = datetime.utcnow()
        actions_taken = []
        errors = []

        try:
            async with db_manager.get_session() as session:
                # Find messages in processing state (likely interrupted)
                result = await session.execute(
                    select(EmailMessage).where(
                        and_(
                            EmailMessage.connection_id == connection_id,
                            EmailMessage.processing_status == "processing"
                        )
                    )
                )
                stuck_messages = result.scalars().all()

                if not stuck_messages:
                    return RecoveryResult(
                        status=RecoveryStatus.SKIPPED,
                        message="No stuck messages found",
                        actions_taken=actions_taken,
                        metadata={"connection_id": connection_id}
                    )

                logger.info(f"Found {len(stuck_messages)} stuck messages for connection {connection_id}")

                # Reset stuck messages to pending for reprocessing
                for message in stuck_messages:
                    message.processing_status = "pending"
                    message.processing_error = "Reset due to interrupted sync recovery"
                    actions_taken.append(f"reset_message_{message.gmail_message_id}")

                await session.commit()

                # Find threads that might be incomplete
                thread_result = await session.execute(
                    select(EmailThread).where(
                        and_(
                            EmailThread.connection_id == connection_id,
                            EmailThread.processing_status == "processing"
                        )
                    )
                )
                stuck_threads = thread_result.scalars().all()

                for thread in stuck_threads:
                    thread.processing_status = "pending"
                    actions_taken.append(f"reset_thread_{thread.gmail_thread_id}")

                await session.commit()

                duration = (datetime.utcnow() - start_time).total_seconds()
                return RecoveryResult(
                    status=RecoveryStatus.COMPLETED,
                    message=f"Reset {len(stuck_messages)} messages and {len(stuck_threads)} threads for reprocessing",
                    actions_taken=actions_taken,
                    metadata={
                        "connection_id": connection_id,
                        "messages_reset": len(stuck_messages),
                        "threads_reset": len(stuck_threads)
                    },
                    duration_seconds=duration
                )

        except Exception as e:
            logger.error(f"Sync recovery failed for connection {connection_id}: {e}")
            return RecoveryResult(
                status=RecoveryStatus.FAILED,
                message=f"Sync recovery failed: {str(e)}",
                actions_taken=actions_taken,
                metadata={"connection_id": connection_id},
                errors=errors + [str(e)]
            )

    @with_error_handling("sync_gap_detection")
    async def detect_and_recover_sync_gaps(self, connection_id: int) -> RecoveryResult:
        """Detect gaps in synchronized data and schedule recovery.

        Args:
            connection_id: Gmail connection ID

        Returns:
            RecoveryResult with gap detection and recovery details
        """
        start_time = datetime.utcnow()
        actions_taken = []
        errors = []

        try:
            async with db_manager.get_session() as session:
                # Get connection details
                conn_result = await session.execute(
                    select(GmailConnection).where(GmailConnection.id == connection_id)
                )
                connection = conn_result.scalar_one_or_none()

                if not connection:
                    return RecoveryResult(
                        status=RecoveryStatus.FAILED,
                        message=f"Connection {connection_id} not found",
                        actions_taken=actions_taken,
                        metadata={"connection_id": connection_id}
                    )

                # Analyze sync history to detect gaps
                # Get the last successfully synced message
                last_message_result = await session.execute(
                    select(EmailMessage).where(
                        and_(
                            EmailMessage.connection_id == connection_id,
                            EmailMessage.processing_status == "completed"
                        )
                    ).order_by(EmailMessage.internal_date.desc()).limit(1)
                )
                last_message = last_message_result.scalar_one_or_none()

                if not last_message:
                    # No messages synced yet - this might be a new connection
                    actions_taken.append("detected_new_connection")
                    return RecoveryResult(
                        status=RecoveryStatus.SKIPPED,
                        message="No previous sync data found - appears to be new connection",
                        actions_taken=actions_taken,
                        metadata={"connection_id": connection_id}
                    )

                # Check for large time gaps in message timestamps
                # This is a simplified gap detection - could be made more sophisticated
                gap_threshold = timedelta(days=1)  # Detect gaps larger than 1 day
                current_time = datetime.utcnow()
                time_since_last_sync = current_time - last_message.internal_date

                if time_since_last_sync > gap_threshold:
                    actions_taken.append("detected_sync_gap")

                    # Schedule incremental sync to fill the gap
                    connection.sync_status = "gap_detected"
                    connection.gap_start_date = last_message.internal_date
                    connection.gap_end_date = current_time
                    await session.commit()

                    actions_taken.append("scheduled_gap_recovery")

                    duration = (datetime.utcnow() - start_time).total_seconds()
                    return RecoveryResult(
                        status=RecoveryStatus.COMPLETED,
                        message=f"Detected sync gap of {time_since_last_sync.days} days, scheduled for recovery",
                        actions_taken=actions_taken,
                        metadata={
                            "connection_id": connection_id,
                            "gap_days": time_since_last_sync.days,
                            "gap_start": last_message.internal_date.isoformat(),
                            "gap_end": current_time.isoformat()
                        },
                        duration_seconds=duration
                    )
                else:
                    return RecoveryResult(
                        status=RecoveryStatus.SKIPPED,
                        message="No significant sync gaps detected",
                        actions_taken=actions_taken,
                        metadata={"connection_id": connection_id}
                    )

        except Exception as e:
            logger.error(f"Sync gap detection failed for connection {connection_id}: {e}")
            return RecoveryResult(
                status=RecoveryStatus.FAILED,
                message=f"Gap detection failed: {str(e)}",
                actions_taken=actions_taken,
                metadata={"connection_id": connection_id},
                errors=errors + [str(e)]
            )


class WebhookRecovery:
    """Handles webhook delivery failure recovery and notification restoration."""

    @with_error_handling("webhook_subscription_recovery")
    async def recover_webhook_subscriptions(self) -> RecoveryResult:
        """Recover webhook subscriptions that have expired or failed.

        Returns:
            RecoveryResult with subscription recovery details
        """
        start_time = datetime.utcnow()
        actions_taken = []
        errors = []
        recovered_count = 0

        try:
            async with db_manager.get_session() as session:
                # Find connections with expired or missing webhook subscriptions
                cutoff_time = datetime.utcnow() + timedelta(hours=1)  # Renew if expiring within 1 hour

                result = await session.execute(
                    select(GmailConnection).where(
                        and_(
                            GmailConnection.is_active == True,
                            or_(
                                GmailConnection.push_subscription_expiry == None,
                                GmailConnection.push_subscription_expiry < cutoff_time
                            )
                        )
                    )
                )
                connections = result.scalars().all()

                logger.info(f"Found {len(connections)} connections needing webhook subscription recovery")

                for connection in connections:
                    try:
                        # Import here to avoid circular dependencies
                        from .sync import create_sync_manager

                        # Create sync manager for this connection
                        sync_manager = await create_sync_manager(connection.id)

                        # Set up new webhook subscription
                        topic_name = f"projects/{connection.project_id}/topics/gmail-notifications"  # Configurable

                        watch_response = await sync_manager.setup_push_notifications(
                            topic_name=topic_name
                        )

                        actions_taken.append(f"renewed_webhook_{connection.id}")
                        recovered_count += 1

                        logger.info(f"Renewed webhook subscription for connection {connection.id}")

                    except Exception as e:
                        error_msg = f"Failed to renew webhook for connection {connection.id}: {str(e)}"
                        errors.append(error_msg)
                        logger.error(error_msg)

                duration = (datetime.utcnow() - start_time).total_seconds()
                status = RecoveryStatus.COMPLETED if recovered_count > 0 else RecoveryStatus.FAILED

                return RecoveryResult(
                    status=status,
                    message=f"Recovered {recovered_count} webhook subscriptions",
                    actions_taken=actions_taken,
                    metadata={
                        "connections_processed": len(connections),
                        "subscriptions_recovered": recovered_count,
                        "failures": len(errors)
                    },
                    duration_seconds=duration,
                    errors=errors
                )

        except Exception as e:
            logger.error(f"Webhook subscription recovery failed: {e}")
            return RecoveryResult(
                status=RecoveryStatus.FAILED,
                message=f"Recovery operation failed: {str(e)}",
                actions_taken=actions_taken,
                metadata={},
                errors=errors + [str(e)]
            )


class SystemHealthRecovery:
    """Provides comprehensive system health checks and recovery coordination."""

    def __init__(self):
        self.oauth_recovery = OAuthTokenRecovery(GmailAuthenticator({}))
        self.sync_recovery = SyncRecovery()
        self.webhook_recovery = WebhookRecovery()

    @with_error_handling("system_health_check")
    async def perform_health_check(self) -> Dict[str, Any]:
        """Perform comprehensive system health check.

        Returns:
            Health check results and recommendations
        """
        health_results = {
            "timestamp": datetime.utcnow().isoformat(),
            "overall_status": "healthy",
            "components": {},
            "recommendations": [],
            "statistics": {}
        }

        try:
            # Check database connectivity
            try:
                async with db_manager.get_session() as session:
                    await session.execute(select(1))
                health_results["components"]["database"] = {"status": "healthy", "details": "Connection successful"}
            except Exception as e:
                health_results["components"]["database"] = {"status": "unhealthy", "details": str(e)}
                health_results["overall_status"] = "degraded"

            # Check Gmail connections
            async with db_manager.get_session() as session:
                # Count active connections
                active_result = await session.execute(
                    select(GmailConnection).where(GmailConnection.is_active == True)
                )
                active_connections = active_result.scalars().all()

                # Count connections needing attention
                problem_result = await session.execute(
                    select(GmailConnection).where(
                        and_(
                            GmailConnection.is_active == True,
                            or_(
                                GmailConnection.auth_status.in_(["token_expired", "refresh_failed", "requires_reauth"]),
                                GmailConnection.push_subscription_expiry < datetime.utcnow() + timedelta(hours=1)
                            )
                        )
                    )
                )
                problem_connections = problem_result.scalars().all()

                # Count stuck sync operations
                stuck_messages_result = await session.execute(
                    select(EmailMessage).where(
                        EmailMessage.processing_status == "processing",
                        EmailMessage.updated_at < datetime.utcnow() - timedelta(hours=1)
                    )
                )
                stuck_messages = stuck_messages_result.scalars().all()

                health_results["statistics"] = {
                    "active_connections": len(active_connections),
                    "problem_connections": len(problem_connections),
                    "stuck_messages": len(stuck_messages),
                    "total_messages": await self._count_total_messages(session),
                    "messages_today": await self._count_messages_today(session)
                }

                # Determine connection health
                if len(problem_connections) == 0:
                    health_results["components"]["connections"] = {"status": "healthy", "details": "All connections active"}
                elif len(problem_connections) < len(active_connections) * 0.2:  # Less than 20% have problems
                    health_results["components"]["connections"] = {
                        "status": "degraded",
                        "details": f"{len(problem_connections)} connections need attention"
                    }
                    health_results["overall_status"] = "degraded"
                else:
                    health_results["components"]["connections"] = {
                        "status": "unhealthy",
                        "details": f"{len(problem_connections)} connections have serious issues"
                    }
                    health_results["overall_status"] = "unhealthy"

                # Check for stuck sync operations
                if len(stuck_messages) > 0:
                    health_results["components"]["sync_operations"] = {
                        "status": "degraded",
                        "details": f"{len(stuck_messages)} messages stuck in processing"
                    }
                    health_results["recommendations"].append("Run sync recovery for stuck operations")
                    if health_results["overall_status"] == "healthy":
                        health_results["overall_status"] = "degraded"
                else:
                    health_results["components"]["sync_operations"] = {"status": "healthy", "details": "No stuck operations"}

                # Generate recommendations
                if len(problem_connections) > 0:
                    health_results["recommendations"].append(f"Run OAuth token recovery for {len(problem_connections)} connections")

                # Check error rates from error handler
                error_stats = gmail_error_handler.get_error_statistics()
                if error_stats["error_stats"]:
                    health_results["components"]["error_rates"] = {"status": "monitored", "details": error_stats}
                    # Add recommendations based on error patterns
                    for category, severity_counts in error_stats["error_stats"].items():
                        high_error_count = severity_counts.get("high", 0) + severity_counts.get("critical", 0)
                        if high_error_count > 5:
                            health_results["recommendations"].append(f"Investigate high error rate in {category}")

        except Exception as e:
            logger.error(f"Health check failed: {e}")
            health_results["overall_status"] = "unhealthy"
            health_results["error"] = str(e)

        return health_results

    async def _count_total_messages(self, session: AsyncSession) -> int:
        """Count total messages in the system."""
        try:
            result = await session.execute(select(EmailMessage))
            return len(result.scalars().all())
        except Exception:
            return -1

    async def _count_messages_today(self, session: AsyncSession) -> int:
        """Count messages received today."""
        try:
            today = datetime.utcnow().date()
            result = await session.execute(
                select(EmailMessage).where(EmailMessage.internal_date >= today)
            )
            return len(result.scalars().all())
        except Exception:
            return -1

    @with_error_handling("automated_recovery")
    async def run_automated_recovery(self, fix_tokens: bool = True, fix_sync: bool = True, fix_webhooks: bool = True) -> Dict[str, Any]:
        """Run automated recovery procedures.

        Args:
            fix_tokens: Whether to run OAuth token recovery
            fix_sync: Whether to run sync recovery
            fix_webhooks: Whether to run webhook recovery

        Returns:
            Comprehensive recovery results
        """
        start_time = datetime.utcnow()
        recovery_results = {
            "timestamp": start_time.isoformat(),
            "operations": {},
            "summary": {
                "total_operations": 0,
                "successful_operations": 0,
                "failed_operations": 0
            }
        }

        # OAuth token recovery
        if fix_tokens:
            try:
                logger.info("Running OAuth token recovery...")
                token_results = await self.oauth_recovery.recover_all_invalid_tokens()
                recovery_results["operations"]["oauth_recovery"] = {
                    "status": "completed",
                    "results": [result.__dict__ for result in token_results],
                    "summary": {
                        "total_connections": len(token_results),
                        "successful": len([r for r in token_results if r.status == RecoveryStatus.COMPLETED]),
                        "failed": len([r for r in token_results if r.status == RecoveryStatus.FAILED])
                    }
                }
                recovery_results["summary"]["total_operations"] += 1
                if len([r for r in token_results if r.status == RecoveryStatus.FAILED]) == 0:
                    recovery_results["summary"]["successful_operations"] += 1
                else:
                    recovery_results["summary"]["failed_operations"] += 1

            except Exception as e:
                logger.error(f"OAuth token recovery failed: {e}")
                recovery_results["operations"]["oauth_recovery"] = {"status": "error", "error": str(e)}
                recovery_results["summary"]["total_operations"] += 1
                recovery_results["summary"]["failed_operations"] += 1

        # Sync recovery
        if fix_sync:
            try:
                logger.info("Running sync recovery...")
                # Get all active connections and check for sync issues
                async with db_manager.get_session() as session:
                    result = await session.execute(
                        select(GmailConnection).where(GmailConnection.is_active == True)
                    )
                    connections = result.scalars().all()

                    sync_results = []
                    for connection in connections:
                        result = await self.sync_recovery.recover_interrupted_sync(connection.id)
                        sync_results.append(result)

                recovery_results["operations"]["sync_recovery"] = {
                    "status": "completed",
                    "results": [result.__dict__ for result in sync_results],
                    "summary": {
                        "total_connections": len(sync_results),
                        "successful": len([r for r in sync_results if r.status in [RecoveryStatus.COMPLETED, RecoveryStatus.SKIPPED]]),
                        "failed": len([r for r in sync_results if r.status == RecoveryStatus.FAILED])
                    }
                }
                recovery_results["summary"]["total_operations"] += 1
                if len([r for r in sync_results if r.status == RecoveryStatus.FAILED]) == 0:
                    recovery_results["summary"]["successful_operations"] += 1
                else:
                    recovery_results["summary"]["failed_operations"] += 1

            except Exception as e:
                logger.error(f"Sync recovery failed: {e}")
                recovery_results["operations"]["sync_recovery"] = {"status": "error", "error": str(e)}
                recovery_results["summary"]["total_operations"] += 1
                recovery_results["summary"]["failed_operations"] += 1

        # Webhook recovery
        if fix_webhooks:
            try:
                logger.info("Running webhook recovery...")
                webhook_result = await self.webhook_recovery.recover_webhook_subscriptions()
                recovery_results["operations"]["webhook_recovery"] = {
                    "status": "completed",
                    "result": webhook_result.__dict__
                }
                recovery_results["summary"]["total_operations"] += 1
                if webhook_result.status in [RecoveryStatus.COMPLETED, RecoveryStatus.SKIPPED]:
                    recovery_results["summary"]["successful_operations"] += 1
                else:
                    recovery_results["summary"]["failed_operations"] += 1

            except Exception as e:
                logger.error(f"Webhook recovery failed: {e}")
                recovery_results["operations"]["webhook_recovery"] = {"status": "error", "error": str(e)}
                recovery_results["summary"]["total_operations"] += 1
                recovery_results["summary"]["failed_operations"] += 1

        # Calculate duration
        duration = (datetime.utcnow() - start_time).total_seconds()
        recovery_results["duration_seconds"] = duration

        logger.info(f"Automated recovery completed in {duration:.2f}s: "
                   f"{recovery_results['summary']['successful_operations']}/{recovery_results['summary']['total_operations']} operations successful")

        return recovery_results


# Global recovery coordinator instance
system_recovery = SystemHealthRecovery()