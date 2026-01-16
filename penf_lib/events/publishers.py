"""Event publishers for the Penfold system."""

import json
import logging
from typing import Optional, Dict, Any, List
from datetime import datetime
import asyncio
import uuid

try:
    import redis.asyncio as redis
except ImportError:
    import redis

from .schemas import (
    BaseEvent,
    EmailIngestedEvent,
    EmailThreadIngestedEvent,
    EmailAttachmentIngestedEvent,
    SyncProgressEvent,
    SyncCompletedEvent,
    ContentExtractedEvent,
    ManualEmailIngestedEvent,
    IngestJobProgressEvent,
    IngestJobCompletedEvent,
)


logger = logging.getLogger(__name__)


class EventPublisher:
    """Base class for publishing events to the message broker."""

    def __init__(self, redis_url: str = "redis://localhost:6379"):
        """Initialize event publisher.

        Args:
            redis_url: Redis connection URL
        """
        self.redis_url = redis_url
        self._redis_client: Optional[redis.Redis] = None

    async def _get_redis_client(self) -> redis.Redis:
        """Get or create Redis client."""
        if self._redis_client is None:
            self._redis_client = redis.from_url(self.redis_url, decode_responses=True)
        return self._redis_client

    async def publish(
        self,
        event: BaseEvent,
        channel: Optional[str] = None,
        routing_key: Optional[str] = None
    ) -> bool:
        """Publish event to message broker.

        Args:
            event: Event to publish
            channel: Redis channel (default: event_type)
            routing_key: Optional routing key for advanced routing

        Returns:
            True if published successfully
        """
        try:
            channel = channel or f"events.{event.event_type}"

            # Add correlation ID if not set
            if not event.correlation_id:
                event.correlation_id = str(uuid.uuid4())

            # Serialize event
            event_data = event.model_dump()
            event_json = json.dumps(event_data, default=str)

            # Publish to Redis
            client = await self._get_redis_client()
            published = await client.publish(channel, event_json)

            logger.debug(f"Published event {event.event_type} to channel {channel}")
            return published > 0

        except Exception as e:
            logger.error(f"Failed to publish event {event.event_type}: {e}")
            return False

    async def publish_content_extracted(self, event: ContentExtractedEvent) -> bool:
        """Publish content extracted event.

        Args:
            event: Content extracted event

        Returns:
            True if published successfully
        """
        return await self.publish(event, channel="events.content.extracted")

    async def close(self) -> None:
        """Close Redis connection."""
        if self._redis_client:
            await self._redis_client.aclose()
            self._redis_client = None


class EmailEventPublisher(EventPublisher):
    """Specialized publisher for email-related events."""

    async def publish_email_ingested(
        self,
        gmail_message_id: str,
        gmail_thread_id: str,
        connection_id: int,
        subject: Optional[str],
        from_email: str,
        from_name: Optional[str],
        to_emails: List[str],
        internal_date: datetime,
        **kwargs
    ) -> bool:
        """Publish email ingested event.

        Args:
            gmail_message_id: Gmail message ID
            gmail_thread_id: Gmail thread ID
            connection_id: Gmail connection ID
            subject: Email subject
            from_email: Sender email
            from_name: Sender name
            to_emails: Recipient emails
            internal_date: Gmail internal date
            **kwargs: Additional event fields

        Returns:
            True if published successfully
        """
        event = EmailIngestedEvent(
            gmail_message_id=gmail_message_id,
            gmail_thread_id=gmail_thread_id,
            connection_id=connection_id,
            subject=subject,
            from_email=from_email,
            from_name=from_name,
            to_emails=to_emails,
            internal_date=internal_date,
            **kwargs
        )

        return await self.publish(event, channel="events.content.ingested")

    async def publish_thread_ingested(
        self,
        gmail_thread_id: str,
        connection_id: int,
        subject: Optional[str],
        participant_emails: List[str],
        message_count: int,
        first_message_date: datetime,
        last_message_date: datetime,
        **kwargs
    ) -> bool:
        """Publish thread ingested event.

        Args:
            gmail_thread_id: Gmail thread ID
            connection_id: Gmail connection ID
            subject: Thread subject
            participant_emails: All participants
            message_count: Number of messages
            first_message_date: First message date
            last_message_date: Last message date
            **kwargs: Additional event fields

        Returns:
            True if published successfully
        """
        event = EmailThreadIngestedEvent(
            gmail_thread_id=gmail_thread_id,
            connection_id=connection_id,
            subject=subject,
            participant_emails=participant_emails,
            message_count=message_count,
            first_message_date=first_message_date,
            last_message_date=last_message_date,
            **kwargs
        )

        return await self.publish(event, channel="events.content.thread_ingested")

    async def publish_attachment_ingested(
        self,
        gmail_attachment_id: str,
        gmail_message_id: str,
        gmail_thread_id: str,
        connection_id: int,
        filename: str,
        mime_type: str,
        size_bytes: int,
        file_path: str,
        **kwargs
    ) -> bool:
        """Publish attachment ingested event.

        Args:
            gmail_attachment_id: Gmail attachment ID
            gmail_message_id: Parent message ID
            gmail_thread_id: Parent thread ID
            connection_id: Gmail connection ID
            filename: Original filename
            mime_type: MIME type
            size_bytes: File size
            file_path: Local file path
            **kwargs: Additional event fields

        Returns:
            True if published successfully
        """
        event = EmailAttachmentIngestedEvent(
            gmail_attachment_id=gmail_attachment_id,
            gmail_message_id=gmail_message_id,
            gmail_thread_id=gmail_thread_id,
            connection_id=connection_id,
            filename=filename,
            mime_type=mime_type,
            size_bytes=size_bytes,
            file_path=file_path,
            **kwargs
        )

        return await self.publish(event, channel="events.content.attachment_ingested")


class SyncEventPublisher(EventPublisher):
    """Specialized publisher for sync operation events."""

    async def publish_sync_progress(
        self,
        sync_session_id: str,
        connection_id: int,
        sync_type: str,
        processed_items: int,
        current_batch: int,
        status: str,
        current_operation: str,
        **kwargs
    ) -> bool:
        """Publish sync progress event.

        Args:
            sync_session_id: Unique sync session ID
            connection_id: Gmail connection ID
            sync_type: Type of sync operation
            processed_items: Items processed
            current_batch: Current batch number
            status: Current status
            current_operation: Description of current operation
            **kwargs: Additional event fields

        Returns:
            True if published successfully
        """
        event = SyncProgressEvent(
            sync_session_id=sync_session_id,
            connection_id=connection_id,
            sync_type=sync_type,
            processed_items=processed_items,
            current_batch=current_batch,
            status=status,
            current_operation=current_operation,
            **kwargs
        )

        return await self.publish(event, channel="events.sync.progress")

    async def publish_sync_completed(
        self,
        sync_session_id: str,
        connection_id: int,
        sync_type: str,
        total_messages: int,
        total_threads: int,
        duration_seconds: float,
        success: bool,
        **kwargs
    ) -> bool:
        """Publish sync completed event.

        Args:
            sync_session_id: Unique sync session ID
            connection_id: Gmail connection ID
            sync_type: Type of sync operation
            total_messages: Total messages processed
            total_threads: Total threads processed
            duration_seconds: Sync duration
            success: Whether sync succeeded
            **kwargs: Additional event fields

        Returns:
            True if published successfully
        """
        event = SyncCompletedEvent(
            sync_session_id=sync_session_id,
            connection_id=connection_id,
            sync_type=sync_type,
            total_messages=total_messages,
            total_threads=total_threads,
            duration_seconds=duration_seconds,
            success=success,
            average_items_per_minute=(total_messages / duration_seconds * 60) if duration_seconds > 0 else 0,
            **kwargs
        )

        return await self.publish(event, channel="events.sync.completed")


class BatchEventPublisher:
    """Publisher that can batch events for efficient publishing."""

    def __init__(self, event_publisher: EventPublisher, batch_size: int = 50):
        """Initialize batch publisher.

        Args:
            event_publisher: Underlying event publisher
            batch_size: Maximum events per batch
        """
        self.event_publisher = event_publisher
        self.batch_size = batch_size
        self._event_batch: List[BaseEvent] = []
        self._batch_lock = asyncio.Lock()

    async def add_event(self, event: BaseEvent, channel: Optional[str] = None) -> None:
        """Add event to batch.

        Args:
            event: Event to add
            channel: Optional channel override
        """
        async with self._batch_lock:
            self._event_batch.append((event, channel))

            if len(self._event_batch) >= self.batch_size:
                await self.flush_batch()

    async def flush_batch(self) -> int:
        """Flush current batch of events.

        Returns:
            Number of events published
        """
        async with self._batch_lock:
            if not self._event_batch:
                return 0

            published_count = 0
            for event, channel in self._event_batch:
                try:
                    if await self.event_publisher.publish(event, channel):
                        published_count += 1
                except Exception as e:
                    logger.error(f"Failed to publish batched event: {e}")

            batch_size = len(self._event_batch)
            self._event_batch.clear()

            logger.debug(f"Published {published_count}/{batch_size} batched events")
            return published_count

    async def close(self) -> None:
        """Flush remaining events and close."""
        await self.flush_batch()
        await self.event_publisher.close()


class ManualIngestEventPublisher(EventPublisher):
    """Specialized publisher for manual ingest events."""

    async def publish_manual_email_ingested(
        self,
        source_id: int,
        tenant_id: str,
        message_id: str,
        job_id: str,
        from_email: str,
        to_emails: List[str],
        email_date: datetime,
        content_hash: str,
        source_tag: str,
        original_file_path: str,
        from_name: Optional[str] = None,
        cc_emails: Optional[List[str]] = None,
        subject: Optional[str] = None,
        date_is_fallback: bool = False,
        in_reply_to: Optional[str] = None,
        thread_id: Optional[str] = None,
        has_attachments: bool = False,
        attachment_count: int = 0,
        labels: Optional[List[str]] = None,
    ) -> bool:
        """Publish manual email ingested event.

        This event triggers the AI processing pipeline for the ingested email.

        Args:
            source_id: Database ID in sources table
            tenant_id: Tenant ID for isolation
            message_id: Email Message-ID (real or synthetic)
            job_id: Parent IngestJob ID
            from_email: Sender email address
            to_emails: Recipient emails
            email_date: Original or fallback email date
            content_hash: SHA-256 of raw .eml content
            source_tag: User-provided source identifier
            original_file_path: Path in upload batch
            from_name: Optional sender display name
            cc_emails: Optional CC recipients
            subject: Optional email subject
            date_is_fallback: True if using ingestion timestamp
            in_reply_to: Optional In-Reply-To header
            thread_id: Optional linked thread ID
            has_attachments: Whether email has attachments
            attachment_count: Number of attachments
            labels: Labels from folder structure

        Returns:
            True if published successfully
        """
        event = ManualEmailIngestedEvent(
            source_id=source_id,
            tenant_id=tenant_id,
            message_id=message_id,
            job_id=job_id,
            from_email=from_email,
            from_name=from_name,
            to_emails=to_emails,
            cc_emails=cc_emails or [],
            subject=subject,
            email_date=email_date,
            date_is_fallback=date_is_fallback,
            in_reply_to=in_reply_to,
            thread_id=thread_id,
            has_attachments=has_attachments,
            attachment_count=attachment_count,
            content_hash=content_hash,
            source_tag=source_tag,
            original_file_path=original_file_path,
            labels=labels or [],
        )

        return await self.publish(event, channel="events.manual_email.ingested")

    async def publish_ingest_job_progress(
        self,
        job_id: str,
        tenant_id: str,
        total_files: int,
        processed_count: int,
        imported_count: int,
        skipped_count: int,
        failed_count: int,
        elapsed_seconds: float,
        status: str,
        current_file: Optional[str] = None,
        estimated_remaining_seconds: Optional[float] = None,
    ) -> bool:
        """Publish ingest job progress event.

        Args:
            job_id: Ingest job ID
            tenant_id: Tenant ID for isolation
            total_files: Total files in batch
            processed_count: Files processed so far
            imported_count: Successfully imported
            skipped_count: Skipped (duplicates)
            failed_count: Failed to process
            elapsed_seconds: Seconds since job started
            status: Current status
            current_file: Currently processing file
            estimated_remaining_seconds: Estimated time remaining

        Returns:
            True if published successfully
        """
        event = IngestJobProgressEvent(
            job_id=job_id,
            tenant_id=tenant_id,
            total_files=total_files,
            processed_count=processed_count,
            imported_count=imported_count,
            skipped_count=skipped_count,
            failed_count=failed_count,
            elapsed_seconds=elapsed_seconds,
            estimated_remaining_seconds=estimated_remaining_seconds,
            status=status,
            current_file=current_file,
        )

        return await self.publish(event, channel="events.ingest_job.progress")

    async def publish_ingest_job_completed(
        self,
        job_id: str,
        tenant_id: str,
        source_tag: str,
        total_files: int,
        imported_count: int,
        skipped_count: int,
        failed_count: int,
        started_at: datetime,
        completed_at: datetime,
        duration_seconds: float,
        success: bool,
        final_status: str,
        labels_created: Optional[List[str]] = None,
        attachments_extracted: int = 0,
    ) -> bool:
        """Publish ingest job completed event.

        Args:
            job_id: Ingest job ID
            tenant_id: Tenant ID for isolation
            source_tag: User-provided source identifier
            total_files: Total files in batch
            imported_count: Successfully imported
            skipped_count: Skipped (duplicates)
            failed_count: Failed to process
            started_at: Job start time
            completed_at: Job completion time
            duration_seconds: Total processing time
            success: Whether job succeeded (no failures)
            final_status: Final job status
            labels_created: Labels from folder structure
            attachments_extracted: Count of extracted attachments

        Returns:
            True if published successfully
        """
        from .schemas import IngestJobCompletedEvent

        event = IngestJobCompletedEvent(
            job_id=job_id,
            tenant_id=tenant_id,
            source_tag=source_tag,
            total_files=total_files,
            imported_count=imported_count,
            skipped_count=skipped_count,
            failed_count=failed_count,
            started_at=started_at,
            completed_at=completed_at,
            duration_seconds=duration_seconds,
            success=success,
            final_status=final_status,
            labels_created=labels_created or [],
            attachments_extracted=attachments_extracted,
        )

        return await self.publish(event, channel="events.ingest_job.completed")