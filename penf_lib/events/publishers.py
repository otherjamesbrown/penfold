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
    SyncCompletedEvent
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