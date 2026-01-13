"""Event-driven processing framework with Redis pub-sub and PostgreSQL fallback.

This module provides event publishing, subscription, and routing capabilities
for coordinating multi-model AI processing pipelines.
"""

import asyncio
import json
import logging
from typing import Dict, Any, List, Optional, Callable, Set
from datetime import datetime, timezone
import uuid

import msgpack
import redis.asyncio as redis
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, update, text
from sqlalchemy.exc import SQLAlchemyError

from penf_lib.storage.models import ProcessingEvent, Subscription
from penf_lib.storage.connections import get_redis_client


logger = logging.getLogger(__name__)


class EventPublisher:
    """Publishes processing events to Redis and stores them in PostgreSQL.

    Provides reliable event publishing with dual storage (Redis for real-time,
    PostgreSQL for persistence and recovery).
    """

    def __init__(
        self,
        session: AsyncSession,
        redis_client: Optional[redis.Redis] = None,
        use_postgresql_fallback: bool = True,
    ):
        """Initialize event publisher.

        Args:
            session: Database session for persistent storage
            redis_client: Redis client for real-time publishing
            use_postgresql_fallback: Whether to use PostgreSQL LISTEN/NOTIFY as fallback
        """
        self.session = session
        self.redis_client = redis_client or get_redis_client()
        self.use_postgresql_fallback = use_postgresql_fallback
        self._redis_available = True

    async def publish(
        self,
        event_type: str,
        event_data: Dict[str, Any],
        channel: Optional[str] = None,
        priority: int = 5,
        retention_days: int = 30,
    ) -> str:
        """Publish an event to Redis and store in PostgreSQL.

        Args:
            event_type: Type of event (e.g., "document.processed")
            event_data: Event payload data
            channel: Redis channel (defaults to event_type)
            priority: Event priority (1=highest, 10=lowest)
            retention_days: How long to keep in PostgreSQL

        Returns:
            Event ID for tracking

        Raises:
            ValueError: Invalid event data
            RuntimeError: Publication failed on both Redis and PostgreSQL
        """
        if not event_type or not isinstance(event_data, dict):
            raise ValueError("Event type and data are required")

        # Generate event ID
        event_uuid = uuid.uuid4()
        channel = channel or event_type

        # Create event record
        event = ProcessingEvent(
            event_id=event_uuid,
            event_type=event_type,
            payload=event_data,
            publisher=f"event_publisher_{channel}",
            published_at=datetime.now(timezone.utc),
        )

        # Store in PostgreSQL first (source of truth)
        try:
            self.session.add(event)
            await self.session.commit()
            logger.debug(f"Stored event {event_uuid} in PostgreSQL")
        except SQLAlchemyError as e:
            await self.session.rollback()
            logger.error(f"Failed to store event {event_uuid} in PostgreSQL: {e}")
            raise RuntimeError(f"Failed to store event: {e}")

        # Try Redis for real-time delivery
        redis_success = await self._publish_to_redis(channel, event_data, str(event_uuid))

        # Fallback to PostgreSQL NOTIFY if Redis fails
        if not redis_success and self.use_postgresql_fallback:
            await self._publish_to_postgresql(channel, event_data, str(event_uuid))

        logger.info(f"Published event {event_uuid} to channel {channel}")
        return str(event_uuid)

    async def _publish_to_redis(
        self, channel: str, event_data: Dict[str, Any], event_id: str
    ) -> bool:
        """Publish event to Redis channel.

        Args:
            channel: Redis channel name
            event_data: Event payload
            event_id: Event identifier

        Returns:
            True if successful, False otherwise
        """
        if not self._redis_available:
            return False

        try:
            # Serialize with msgpack for efficiency
            message_data = {
                "event_id": event_id,
                "timestamp": datetime.now(timezone.utc).isoformat(),
                "payload": event_data,
            }
            message = msgpack.packb(message_data)

            # Publish to Redis
            await self.redis_client.publish(channel, message)
            logger.debug(f"Published event {event_id} to Redis channel {channel}")
            return True

        except (redis.RedisError, ConnectionError) as e:
            logger.warning(f"Redis publish failed for event {event_id}: {e}")
            self._redis_available = False
            return False

    async def _publish_to_postgresql(
        self, channel: str, event_data: Dict[str, Any], event_id: str
    ) -> None:
        """Publish event using PostgreSQL LISTEN/NOTIFY.

        Args:
            channel: Notification channel
            event_data: Event payload
            event_id: Event identifier
        """
        try:
            # Use PostgreSQL NOTIFY as fallback
            notification_payload = json.dumps({
                "event_id": event_id,
                "payload": event_data,
            })

            # NOTIFY with channel and payload
            await self.session.execute(
                text(f"NOTIFY {channel}, :payload"),
                {"payload": notification_payload}
            )
            await self.session.commit()
            logger.debug(f"Sent PostgreSQL notification for event {event_id}")

        except SQLAlchemyError as e:
            logger.error(f"PostgreSQL notification failed for event {event_id}: {e}")
            await self.session.rollback()

    async def get_event_status(self, event_id: str) -> Optional[Dict[str, Any]]:
        """Get the status of a published event.

        Args:
            event_id: Event identifier

        Returns:
            Event status information or None if not found
        """
        try:
            result = await self.session.execute(
                select(ProcessingEvent).where(ProcessingEvent.id == event_id)
            )
            event = result.scalar_one_or_none()

            if not event:
                return None

            return {
                "event_id": event.id,
                "event_type": event.event_type,
                "channel": event.channel,
                "published_at": event.published_at,
                "status": "published",  # Could be extended with processing status
            }

        except SQLAlchemyError as e:
            logger.error(f"Failed to get event status for {event_id}: {e}")
            return None


class EventSubscriber:
    """Subscribes to and processes events from Redis with error handling.

    Provides reliable event consumption with retry logic, error handling,
    and subscription management.
    """

    def __init__(
        self,
        redis_client: Optional[redis.Redis] = None,
        max_retries: int = 3,
        retry_delay: float = 1.0,
    ):
        """Initialize event subscriber.

        Args:
            redis_client: Redis client for subscriptions
            max_retries: Maximum retry attempts for failed event processing
            retry_delay: Delay between retries in seconds
        """
        self.redis_client = redis_client or get_redis_client()
        self.max_retries = max_retries
        self.retry_delay = retry_delay
        self._subscriptions: Set[str] = set()
        self._handlers: Dict[str, Callable] = {}
        self._running = False

    async def subscribe(
        self,
        channels: List[str],
        handler: Callable[[str, Dict[str, Any]], None],
        filter_func: Optional[Callable[[str, Dict[str, Any]], bool]] = None,
    ) -> None:
        """Subscribe to event channels with handler.

        Args:
            channels: List of channels to subscribe to
            handler: Async function to handle events (event_type, event_data)
            filter_func: Optional filter function to selectively process events
        """
        # Store handler for each channel
        for channel in channels:
            self._handlers[channel] = {
                "handler": handler,
                "filter": filter_func,
            }
            self._subscriptions.add(channel)

        logger.info(f"Subscribed to channels: {channels}")

    async def start_consuming(self) -> None:
        """Start consuming events from subscribed channels.

        This method blocks and continuously processes events until stopped.
        """
        if not self._subscriptions:
            logger.warning("No subscriptions configured, nothing to consume")
            return

        self._running = True
        logger.info(f"Starting event consumption for channels: {list(self._subscriptions)}")

        try:
            # Subscribe to Redis channels
            pubsub = self.redis_client.pubsub()
            await pubsub.subscribe(*self._subscriptions)

            # Process messages
            async for message in pubsub.listen():
                if not self._running:
                    break

                if message["type"] == "message":
                    await self._handle_message(message)

        except (redis.RedisError, ConnectionError) as e:
            logger.error(f"Redis subscription error: {e}")
        finally:
            await pubsub.unsubscribe(*self._subscriptions)
            await pubsub.close()

    async def stop_consuming(self) -> None:
        """Stop consuming events."""
        self._running = False
        logger.info("Stopping event consumption")

    async def _handle_message(self, message: Dict[str, Any]) -> None:
        """Handle a received message with retry logic.

        Args:
            message: Redis message object
        """
        channel = message["channel"].decode()

        try:
            # Deserialize message
            message_data = msgpack.unpackb(message["data"], raw=False)
            event_id = message_data.get("event_id")
            payload = message_data.get("payload", {})

            # Get handler for this channel
            handler_config = self._handlers.get(channel)
            if not handler_config:
                logger.warning(f"No handler configured for channel {channel}")
                return

            # Apply filter if configured
            filter_func = handler_config.get("filter")
            if filter_func and not filter_func(channel, payload):
                logger.debug(f"Event {event_id} filtered out for channel {channel}")
                return

            # Process event with retry logic
            handler = handler_config["handler"]
            await self._process_with_retry(channel, payload, handler, event_id)

        except Exception as e:
            logger.error(f"Failed to handle message from channel {channel}: {e}")

    async def _process_with_retry(
        self,
        event_type: str,
        event_data: Dict[str, Any],
        handler: Callable,
        event_id: Optional[str] = None,
    ) -> None:
        """Process event with retry logic.

        Args:
            event_type: Type/channel of event
            event_data: Event payload
            handler: Handler function
            event_id: Optional event ID for logging
        """
        for attempt in range(self.max_retries + 1):
            try:
                await handler(event_type, event_data)
                if attempt > 0:
                    logger.info(f"Event {event_id} processed successfully on attempt {attempt + 1}")
                return

            except Exception as e:
                if attempt < self.max_retries:
                    logger.warning(
                        f"Event {event_id} processing failed (attempt {attempt + 1}): {e}"
                    )
                    await asyncio.sleep(self.retry_delay * (2 ** attempt))  # Exponential backoff
                else:
                    logger.error(
                        f"Event {event_id} processing failed permanently after {self.max_retries + 1} attempts: {e}"
                    )


class SubscriptionManager:
    """Manages event subscriptions and routing configuration.

    Provides subscription lifecycle management with filtering and routing
    capabilities for event-driven processing.
    """

    def __init__(self, session: AsyncSession):
        """Initialize subscription manager.

        Args:
            session: Database session for subscription persistence
        """
        self.session = session

    async def create_subscription(
        self,
        subscriber_id: str,
        subscription_name: str,
        event_channels: List[str],
        event_types: Optional[List[str]] = None,
        filter_criteria: Optional[Dict[str, Any]] = None,
        **kwargs,
    ) -> str:
        """Create a new event subscription.

        Args:
            subscriber_id: ID of the subscribing component
            subscription_name: Name of the subscription
            event_channels: List of channels to subscribe to
            event_types: Optional specific event types to filter
            filter_criteria: Optional JSONPath-style filter rules
            **kwargs: Additional subscription configuration

        Returns:
            Subscription ID
        """
        subscription = Subscription(
            subscriber_id=subscriber_id,
            subscription_name=subscription_name,
            event_channels=event_channels,
            event_types=event_types,
            filter_criteria=filter_criteria,
            **kwargs,
        )

        try:
            self.session.add(subscription)
            await self.session.commit()
            logger.info(f"Created subscription {subscription.id} for {subscriber_id}")
            return str(subscription.id)

        except SQLAlchemyError as e:
            await self.session.rollback()
            logger.error(f"Failed to create subscription: {e}")
            raise

    async def get_active_subscriptions(
        self, channels: Optional[List[str]] = None
    ) -> List[Subscription]:
        """Get active subscriptions, optionally filtered by channels.

        Args:
            channels: Optional list of channels to filter by

        Returns:
            List of active subscriptions
        """
        query = select(Subscription).where(Subscription.is_active == True)

        if channels:
            # Filter by channels that overlap with subscription channels
            channel_conditions = []
            for channel in channels:
                channel_conditions.append(
                    text("event_channels @> :channel").params(channel=json.dumps([channel]))
                )
            if channel_conditions:
                query = query.where(text(" OR ".join([str(c) for c in channel_conditions])))

        result = await self.session.execute(query)
        return result.scalars().all()

    async def update_subscription_stats(
        self,
        subscription_id: str,
        events_processed: int = 0,
        events_failed: int = 0,
    ) -> None:
        """Update subscription processing statistics.

        Args:
            subscription_id: Subscription to update
            events_processed: Number of events successfully processed
            events_failed: Number of events that failed processing
        """
        try:
            await self.session.execute(
                update(Subscription)
                .where(Subscription.id == subscription_id)
                .values(
                    total_events_processed=Subscription.total_events_processed + events_processed,
                    total_events_failed=Subscription.total_events_failed + events_failed,
                    last_processed_at=datetime.now(timezone.utc),
                )
            )
            await self.session.commit()

        except SQLAlchemyError as e:
            logger.error(f"Failed to update subscription stats: {e}")
            await self.session.rollback()