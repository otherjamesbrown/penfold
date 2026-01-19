"""Data retention and deletion policy enforcement for Gmail integration."""

import logging
import asyncio
from datetime import datetime, timedelta
from typing import List, Dict, Any, Optional, Tuple
from pathlib import Path

from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, delete, and_, func, or_
from sqlalchemy.orm import selectinload

from ...models.privacy_filter import RetentionPolicy, PrivacyAuditLog
from ...models.email_message import EmailMessage, EmailThread, EmailAttachment
from ...models.gmail_connection import GmailConnection
from ...storage.database import db_manager


logger = logging.getLogger(__name__)


class RetentionPolicyEnforcer:
    """Enforces data retention and deletion policies."""

    def __init__(self):
        """Initialize retention policy enforcer."""
        self.dry_run = False  # Set to True to simulate without actual deletion

    async def enforce_retention_policies(
        self,
        connection_id: Optional[int] = None,
        dry_run: bool = False
    ) -> Dict[str, Any]:
        """Enforce retention policies for connections.

        Args:
            connection_id: Specific connection to process (None for all)
            dry_run: If True, simulate without actual deletion

        Returns:
            Enforcement results
        """
        self.dry_run = dry_run
        start_time = datetime.utcnow()

        logger.info(f"Starting retention policy enforcement (dry_run={dry_run})")

        results = {
            'started_at': start_time.isoformat(),
            'dry_run': dry_run,
            'connections_processed': 0,
            'emails_deleted': 0,
            'attachments_deleted': 0,
            'audit_logs_deleted': 0,
            'threads_deleted': 0,
            'errors': [],
            'details': []
        }

        try:
            # Get connections to process
            async with db_manager.get_session() as session:
                if connection_id:
                    connection_query = select(GmailConnection).where(
                        and_(
                            GmailConnection.id == connection_id,
                            GmailConnection.is_active == True
                        )
                    )
                else:
                    connection_query = select(GmailConnection).where(
                        GmailConnection.is_active == True
                    )

                connection_result = await session.execute(connection_query)
                connections = connection_result.scalars().all()

                for connection in connections:
                    try:
                        connection_results = await self._enforce_connection_retention(
                            session, connection
                        )
                        results['details'].append(connection_results)
                        results['connections_processed'] += 1
                        results['emails_deleted'] += connection_results['emails_deleted']
                        results['attachments_deleted'] += connection_results['attachments_deleted']
                        results['audit_logs_deleted'] += connection_results['audit_logs_deleted']
                        results['threads_deleted'] += connection_results['threads_deleted']

                    except Exception as e:
                        error_msg = f"Error processing connection {connection.id}: {e}"
                        logger.error(error_msg)
                        results['errors'].append(error_msg)

                if not self.dry_run:
                    await session.commit()

        except Exception as e:
            error_msg = f"Error during retention policy enforcement: {e}"
            logger.error(error_msg)
            results['errors'].append(error_msg)

        end_time = datetime.utcnow()
        results['completed_at'] = end_time.isoformat()
        results['duration_seconds'] = (end_time - start_time).total_seconds()

        logger.info(f"Retention policy enforcement completed in {results['duration_seconds']:.2f}s")
        return results

    async def _enforce_connection_retention(
        self,
        session: AsyncSession,
        connection: GmailConnection
    ) -> Dict[str, Any]:
        """Enforce retention policies for a specific connection.

        Args:
            session: Database session
            connection: Gmail connection

        Returns:
            Enforcement results for connection
        """
        logger.debug(f"Processing retention for connection {connection.id}")

        # Get active retention policy for connection
        policy_result = await session.execute(
            select(RetentionPolicy).where(
                and_(
                    RetentionPolicy.connection_id == connection.id,
                    RetentionPolicy.is_active == True
                )
            ).limit(1)
        )
        policy = policy_result.scalar_one_or_none()

        if not policy:
            logger.debug(f"No active retention policy for connection {connection.id}")
            return {
                'connection_id': connection.id,
                'account_email': connection.account_email,
                'policy_active': False,
                'emails_deleted': 0,
                'attachments_deleted': 0,
                'audit_logs_deleted': 0,
                'threads_deleted': 0,
                'details': "No active retention policy"
            }

        results = {
            'connection_id': connection.id,
            'account_email': connection.account_email,
            'policy_id': policy.id,
            'policy_name': policy.name,
            'policy_active': True,
            'emails_deleted': 0,
            'attachments_deleted': 0,
            'audit_logs_deleted': 0,
            'threads_deleted': 0,
            'details': []
        }

        # Delete expired emails
        if policy.email_retention_days > 0:
            email_results = await self._delete_expired_emails(
                session, connection.id, policy
            )
            results['emails_deleted'] = email_results['count']
            results['details'].append(f"Deleted {email_results['count']} expired emails")

        # Delete expired attachments
        if policy.attachment_retention_days > 0:
            attachment_results = await self._delete_expired_attachments(
                session, connection.id, policy
            )
            results['attachments_deleted'] = attachment_results['count']
            results['details'].append(f"Deleted {attachment_results['count']} expired attachments")

        # Delete expired audit logs
        if policy.audit_log_retention_days > 0:
            audit_results = await self._delete_expired_audit_logs(
                session, connection.id, policy
            )
            results['audit_logs_deleted'] = audit_results['count']
            results['details'].append(f"Deleted {audit_results['count']} expired audit logs")

        # Clean up empty threads
        thread_results = await self._cleanup_empty_threads(session, connection.id)
        results['threads_deleted'] = thread_results['count']
        if thread_results['count'] > 0:
            results['details'].append(f"Deleted {thread_results['count']} empty threads")

        # Update policy statistics
        if not self.dry_run:
            policy.emails_deleted += results['emails_deleted']
            policy.last_cleanup_run = datetime.utcnow()
            policy.next_cleanup_scheduled = datetime.utcnow() + timedelta(days=1)

        return results

    async def _delete_expired_emails(
        self,
        session: AsyncSession,
        connection_id: int,
        policy: RetentionPolicy
    ) -> Dict[str, Any]:
        """Delete expired emails based on retention policy.

        Args:
            session: Database session
            connection_id: Gmail connection ID
            policy: Retention policy

        Returns:
            Deletion results
        """
        cutoff_date = datetime.utcnow() - timedelta(days=policy.email_retention_days)

        # Handle sensitive data with shorter retention
        sensitive_cutoff_date = datetime.utcnow() - timedelta(days=policy.sensitive_data_retention_days)

        # Build deletion criteria
        deletion_criteria = [
            and_(
                EmailMessage.connection_id == connection_id,
                EmailMessage.internal_date < cutoff_date
            )
        ]

        # Add sensitive data criteria if applicable
        if policy.sensitive_data_retention_days < policy.email_retention_days:
            deletion_criteria.append(
                and_(
                    EmailMessage.connection_id == connection_id,
                    EmailMessage.internal_date < sensitive_cutoff_date,
                    # Add criteria for sensitive emails (e.g., marked by privacy filters)
                    or_(
                        EmailMessage.gmail_labels.contains(['SENSITIVE']),
                        EmailMessage.processing_status == 'sensitive'
                    )
                )
            )

        # Count emails to be deleted
        count_query = select(func.count(EmailMessage.id)).where(
            or_(*deletion_criteria)
        )
        count_result = await session.execute(count_query)
        count = count_result.scalar()

        if count == 0:
            return {'count': 0, 'details': 'No expired emails found'}

        logger.info(f"Deleting {count} expired emails for connection {connection_id}")

        if not self.dry_run:
            # Get message IDs for cascading deletes
            message_query = select(EmailMessage.id).where(
                or_(*deletion_criteria)
            )
            message_result = await session.execute(message_query)
            message_ids = [row[0] for row in message_result]

            # Delete related attachments first
            if message_ids:
                attachment_delete = delete(EmailAttachment).where(
                    EmailAttachment.message_id.in_(message_ids)
                )
                await session.execute(attachment_delete)

            # Delete email messages
            email_delete = delete(EmailMessage).where(
                or_(*deletion_criteria)
            )
            await session.execute(email_delete)

        return {
            'count': count,
            'cutoff_date': cutoff_date.isoformat(),
            'sensitive_cutoff_date': sensitive_cutoff_date.isoformat() if policy.sensitive_data_retention_days < policy.email_retention_days else None
        }

    async def _delete_expired_attachments(
        self,
        session: AsyncSession,
        connection_id: int,
        policy: RetentionPolicy
    ) -> Dict[str, Any]:
        """Delete expired attachments based on retention policy.

        Args:
            session: Database session
            connection_id: Gmail connection ID
            policy: Retention policy

        Returns:
            Deletion results
        """
        cutoff_date = datetime.utcnow() - timedelta(days=policy.attachment_retention_days)

        # Find expired attachments
        attachment_query = select(EmailAttachment).join(EmailMessage).where(
            and_(
                EmailMessage.connection_id == connection_id,
                EmailAttachment.created_at < cutoff_date
            )
        )
        attachment_result = await session.execute(attachment_query)
        attachments = attachment_result.scalars().all()

        if not attachments:
            return {'count': 0, 'details': 'No expired attachments found'}

        count = len(attachments)
        logger.info(f"Deleting {count} expired attachments for connection {connection_id}")

        if not self.dry_run:
            # Delete physical files
            deleted_files = 0
            for attachment in attachments:
                try:
                    if attachment.file_path and Path(attachment.file_path).exists():
                        Path(attachment.file_path).unlink()
                        deleted_files += 1
                except Exception as e:
                    logger.warning(f"Could not delete attachment file {attachment.file_path}: {e}")

            # Delete database records
            attachment_ids = [attachment.id for attachment in attachments]
            attachment_delete = delete(EmailAttachment).where(
                EmailAttachment.id.in_(attachment_ids)
            )
            await session.execute(attachment_delete)

            logger.debug(f"Deleted {deleted_files} physical attachment files")

        return {
            'count': count,
            'cutoff_date': cutoff_date.isoformat(),
            'physical_files_deleted': count if self.dry_run else deleted_files
        }

    async def _delete_expired_audit_logs(
        self,
        session: AsyncSession,
        connection_id: int,
        policy: RetentionPolicy
    ) -> Dict[str, Any]:
        """Delete expired audit logs based on retention policy.

        Args:
            session: Database session
            connection_id: Gmail connection ID
            policy: Retention policy

        Returns:
            Deletion results
        """
        cutoff_date = datetime.utcnow() - timedelta(days=policy.audit_log_retention_days)

        # Count audit logs to be deleted
        count_query = select(func.count(PrivacyAuditLog.id)).where(
            and_(
                PrivacyAuditLog.connection_id == connection_id,
                PrivacyAuditLog.timestamp < cutoff_date
            )
        )
        count_result = await session.execute(count_query)
        count = count_result.scalar()

        if count == 0:
            return {'count': 0, 'details': 'No expired audit logs found'}

        logger.info(f"Deleting {count} expired audit logs for connection {connection_id}")

        if not self.dry_run:
            # Delete audit logs
            audit_delete = delete(PrivacyAuditLog).where(
                and_(
                    PrivacyAuditLog.connection_id == connection_id,
                    PrivacyAuditLog.timestamp < cutoff_date
                )
            )
            await session.execute(audit_delete)

        return {
            'count': count,
            'cutoff_date': cutoff_date.isoformat()
        }

    async def _cleanup_empty_threads(
        self,
        session: AsyncSession,
        connection_id: int
    ) -> Dict[str, Any]:
        """Clean up email threads that no longer have any messages.

        Args:
            session: Database session
            connection_id: Gmail connection ID

        Returns:
            Cleanup results
        """
        # Find threads with no messages
        empty_threads_query = select(EmailThread.id).where(
            and_(
                EmailThread.connection_id == connection_id,
                ~EmailThread.id.in_(
                    select(EmailMessage.thread_id).where(
                        EmailMessage.connection_id == connection_id
                    )
                )
            )
        )
        empty_threads_result = await session.execute(empty_threads_query)
        empty_thread_ids = [row[0] for row in empty_threads_result]

        if not empty_thread_ids:
            return {'count': 0, 'details': 'No empty threads found'}

        count = len(empty_thread_ids)
        logger.debug(f"Deleting {count} empty threads for connection {connection_id}")

        if not self.dry_run:
            # Delete empty threads
            thread_delete = delete(EmailThread).where(
                EmailThread.id.in_(empty_thread_ids)
            )
            await session.execute(thread_delete)

        return {
            'count': count,
            'empty_thread_ids': empty_thread_ids[:10]  # Limit for logging
        }

    async def delete_specific_email(
        self,
        connection_id: int,
        gmail_message_id: str,
        delete_reason: str = "User requested deletion"
    ) -> Dict[str, Any]:
        """Delete a specific email and all related data.

        Args:
            connection_id: Gmail connection ID
            gmail_message_id: Gmail message ID to delete
            delete_reason: Reason for deletion

        Returns:
            Deletion results
        """
        logger.info(f"Deleting specific email {gmail_message_id} for connection {connection_id}")

        async with db_manager.get_session() as session:
            # Find the email message
            email_result = await session.execute(
                select(EmailMessage).where(
                    and_(
                        EmailMessage.connection_id == connection_id,
                        EmailMessage.gmail_message_id == gmail_message_id
                    )
                )
                .options(selectinload(EmailMessage.attachments))
            )
            email_message = email_result.scalar_one_or_none()

            if not email_message:
                return {
                    'success': False,
                    'error': f"Email {gmail_message_id} not found",
                    'deleted_items': {}
                }

            deleted_items = {
                'email_message': 1,
                'attachments': 0,
                'audit_logs': 0,
                'physical_files': 0
            }

            # Delete attachments and their files
            if email_message.attachments:
                for attachment in email_message.attachments:
                    try:
                        if attachment.file_path and Path(attachment.file_path).exists():
                            Path(attachment.file_path).unlink()
                            deleted_items['physical_files'] += 1
                    except Exception as e:
                        logger.warning(f"Could not delete attachment file {attachment.file_path}: {e}")

                deleted_items['attachments'] = len(email_message.attachments)

            # Delete related audit logs
            audit_delete = delete(PrivacyAuditLog).where(
                and_(
                    PrivacyAuditLog.connection_id == connection_id,
                    PrivacyAuditLog.gmail_message_id == gmail_message_id
                )
            )
            audit_result = await session.execute(audit_delete)
            deleted_items['audit_logs'] = audit_result.rowcount

            # Delete the email message (cascades to attachments)
            await session.delete(email_message)

            # Log the deletion
            audit_log = PrivacyAuditLog(
                gmail_message_id=gmail_message_id,
                gmail_thread_id=email_message.thread.gmail_thread_id if email_message.thread else '',
                connection_id=connection_id,
                filter_id=0,  # Special ID for manual deletions
                filter_name="manual_deletion",
                filter_type="manual",
                action_taken="delete",
                match_reason=delete_reason,
                matched_criteria={'manual_deletion': True},
                email_metadata={'deleted_at': datetime.utcnow().isoformat()},
                user_context="manual_deletion"
            )
            session.add(audit_log)

            await session.commit()

            return {
                'success': True,
                'gmail_message_id': gmail_message_id,
                'deleted_items': deleted_items,
                'delete_reason': delete_reason,
                'deleted_at': datetime.utcnow().isoformat()
            }

    async def get_retention_status(self, connection_id: int) -> Dict[str, Any]:
        """Get retention policy status for a connection.

        Args:
            connection_id: Gmail connection ID

        Returns:
            Retention status information
        """
        async with db_manager.get_session() as session:
            # Get active retention policy
            policy_result = await session.execute(
                select(RetentionPolicy).where(
                    and_(
                        RetentionPolicy.connection_id == connection_id,
                        RetentionPolicy.is_active == True
                    )
                ).limit(1)
            )
            policy = policy_result.scalar_one_or_none()

            if not policy:
                return {
                    'has_policy': False,
                    'connection_id': connection_id,
                    'message': 'No active retention policy configured'
                }

            # Get data age statistics
            now = datetime.utcnow()

            # Count emails by age
            email_stats = await session.execute(
                select(
                    func.count(EmailMessage.id).label('total_emails'),
                    func.min(EmailMessage.internal_date).label('oldest_email'),
                    func.max(EmailMessage.internal_date).label('newest_email')
                ).where(EmailMessage.connection_id == connection_id)
            )
            email_data = email_stats.first()

            # Count emails approaching retention limit
            email_cutoff = now - timedelta(days=policy.email_retention_days)
            approaching_cutoff = now - timedelta(days=policy.email_retention_days - 7)  # 7 days warning

            approaching_result = await session.execute(
                select(func.count(EmailMessage.id)).where(
                    and_(
                        EmailMessage.connection_id == connection_id,
                        EmailMessage.internal_date < approaching_cutoff,
                        EmailMessage.internal_date >= email_cutoff
                    )
                )
            )
            approaching_count = approaching_result.scalar()

            # Count expired emails
            expired_result = await session.execute(
                select(func.count(EmailMessage.id)).where(
                    and_(
                        EmailMessage.connection_id == connection_id,
                        EmailMessage.internal_date < email_cutoff
                    )
                )
            )
            expired_count = expired_result.scalar()

            return {
                'has_policy': True,
                'connection_id': connection_id,
                'policy': policy.to_dict(),
                'email_stats': {
                    'total_emails': email_data.total_emails or 0,
                    'oldest_email': email_data.oldest_email.isoformat() if email_data.oldest_email else None,
                    'newest_email': email_data.newest_email.isoformat() if email_data.newest_email else None,
                    'emails_approaching_retention': approaching_count,
                    'expired_emails_pending_deletion': expired_count
                },
                'next_cleanup': policy.next_cleanup_scheduled.isoformat() if policy.next_cleanup_scheduled else None,
                'last_cleanup': policy.last_cleanup_run.isoformat() if policy.last_cleanup_run else None
            }