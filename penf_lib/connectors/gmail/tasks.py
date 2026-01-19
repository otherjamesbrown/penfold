"""Celery tasks for Gmail attachment processing."""

import asyncio
import logging
from typing import Optional

from celery import Celery
from sqlalchemy.ext.asyncio import AsyncSession

from ...models.email_message import EmailAttachment, EmailMessage
from ...storage.database import get_db_session
from .attachments import AttachmentProcessor
from .client import GmailClient

logger = logging.getLogger(__name__)

# Initialize Celery app
celery_app = Celery('gmail_attachments')
celery_app.config_from_object('penf_lib.connectors.gmail.celery_config')


@celery_app.task(bind=True, max_retries=3, default_retry_delay=300)
def process_attachment_background(self, attachment_id: int, gmail_message_id: str):
    """Background task for processing large or complex attachments.

    Args:
        attachment_id: EmailAttachment record ID
        gmail_message_id: Gmail message ID
    """
    logger.info(f"Starting background processing for attachment {attachment_id}")

    try:
        # Run async function in sync task
        loop = asyncio.new_event_loop()
        asyncio.set_event_loop(loop)
        result = loop.run_until_complete(
            _process_attachment_async(attachment_id, gmail_message_id)
        )
        loop.close()

        if result:
            logger.info(f"Background processing completed for attachment {attachment_id}")
        else:
            logger.error(f"Background processing failed for attachment {attachment_id}")
            raise Exception("Processing failed")

    except Exception as exc:
        logger.error(f"Background processing error for attachment {attachment_id}: {exc}")

        # Retry logic
        if self.request.retries < self.max_retries:
            logger.info(f"Retrying attachment {attachment_id} (attempt {self.request.retries + 1})")
            raise self.retry(exc=exc)
        else:
            # Mark as failed after all retries
            loop = asyncio.new_event_loop()
            asyncio.set_event_loop(loop)
            loop.run_until_complete(_mark_attachment_failed(attachment_id, str(exc)))
            loop.close()


async def _process_attachment_async(attachment_id: int, gmail_message_id: str) -> bool:
    """Process attachment asynchronously.

    Args:
        attachment_id: EmailAttachment record ID
        gmail_message_id: Gmail message ID

    Returns:
        True if successful, False otherwise
    """
    async with get_db_session() as session:
        # Get attachment record
        attachment = await session.get(EmailAttachment, attachment_id)
        if not attachment:
            logger.error(f"Attachment {attachment_id} not found")
            return False

        # Get Gmail client (would need proper authentication setup)
        # For now, we'll simulate the processing
        try:
            # Update status
            attachment.extraction_status = 'processing'
            await session.flush()

            # Initialize processor
            # Note: In real implementation, we'd need to handle authentication properly
            gmail_client = GmailClient()  # This would need proper setup
            processor = AttachmentProcessor(gmail_client, celery_app)

            # Download and process
            success = await processor._process_attachment_immediate(
                session, attachment, gmail_message_id
            )

            await session.commit()
            return success

        except Exception as e:
            logger.error(f"Error processing attachment {attachment_id}: {e}")
            attachment.extraction_status = 'failed'
            await session.commit()
            return False


async def _mark_attachment_failed(attachment_id: int, error_message: str) -> None:
    """Mark attachment as permanently failed.

    Args:
        attachment_id: EmailAttachment record ID
        error_message: Error description
    """
    async with get_db_session() as session:
        attachment = await session.get(EmailAttachment, attachment_id)
        if attachment:
            attachment.extraction_status = 'failed'
            # Could store error_message in a separate field if needed
            await session.commit()
            logger.info(f"Marked attachment {attachment_id} as permanently failed")


@celery_app.task
def cleanup_old_attachments(max_age_days: int = 30):
    """Cleanup old attachment files.

    Args:
        max_age_days: Maximum age in days
    """
    from .attachments import AttachmentStorageManager

    manager = AttachmentStorageManager()
    loop = asyncio.new_event_loop()
    asyncio.set_event_loop(loop)
    deleted_count = loop.run_until_complete(manager.cleanup_old_attachments(max_age_days))
    loop.close()

    logger.info(f"Cleanup task completed: {deleted_count} files deleted")
    return deleted_count


@celery_app.task
def generate_storage_report():
    """Generate storage usage report."""
    from .attachments import AttachmentStorageManager

    manager = AttachmentStorageManager()
    stats = manager.get_storage_stats()

    logger.info(f"Storage report: {stats}")
    return stats


# Periodic tasks configuration
if hasattr(celery_app, 'conf'):
    celery_app.conf.beat_schedule = {
        'cleanup-attachments': {
            'task': 'penf_lib.connectors.gmail.tasks.cleanup_old_attachments',
            'schedule': 24 * 60 * 60,  # Daily
            'args': (30,)  # 30 days
        },
        'storage-report': {
            'task': 'penf_lib.connectors.gmail.tasks.generate_storage_report',
            'schedule': 7 * 24 * 60 * 60,  # Weekly
        },
    }