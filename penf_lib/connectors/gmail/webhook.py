"""Gmail webhook handler for push notifications and real-time sync."""

import hmac
import hashlib
import json
import logging
from typing import Optional, Dict, Any, List
from datetime import datetime, timedelta
import asyncio
import base64

from aiohttp import web
from aiohttp.web import Request, Response

from .client import GmailClient
from .sync import GmailSyncManager, create_sync_manager
from ...models.gmail_connection import GmailConnection
from ...storage.database import db_manager
from sqlalchemy import select


logger = logging.getLogger(__name__)


class GmailWebhookHandler:
    """Handles Gmail push notifications via webhooks."""

    def __init__(
        self,
        webhook_secret: Optional[str] = None,
        max_concurrent_syncs: int = 5
    ):
        """Initialize webhook handler.

        Args:
            webhook_secret: Secret for webhook signature verification
            max_concurrent_syncs: Maximum concurrent sync operations
        """
        self.webhook_secret = webhook_secret
        self.max_concurrent_syncs = max_concurrent_syncs
        self._active_syncs: Dict[int, asyncio.Task] = {}  # connection_id -> task
        self._sync_semaphore = asyncio.Semaphore(max_concurrent_syncs)

    async def handle_notification(self, request: Request) -> Response:
        """Handle incoming Gmail push notification.

        Args:
            request: HTTP request from Google Cloud Pub/Sub

        Returns:
            HTTP response
        """
        try:
            # Verify webhook signature if configured
            if self.webhook_secret:
                if not await self._verify_signature(request):
                    logger.warning("Invalid webhook signature")
                    return web.Response(status=401, text="Invalid signature")

            # Parse notification payload
            notification = await self._parse_notification(request)
            if not notification:
                logger.warning("Invalid notification format")
                return web.Response(status=400, text="Invalid notification")

            # Process notification asynchronously
            asyncio.create_task(self._process_notification(notification))

            # Return 200 immediately for Google's requirements
            return web.Response(status=200, text="OK")

        except Exception as e:
            logger.error(f"Error handling webhook notification: {e}")
            return web.Response(status=500, text="Internal error")

    async def _verify_signature(self, request: Request) -> bool:
        """Verify webhook signature from Google Cloud Pub/Sub.

        Args:
            request: HTTP request

        Returns:
            True if signature is valid
        """
        try:
            # Google Cloud Pub/Sub sends signature in header
            signature = request.headers.get('X-Goog-Signature')
            if not signature:
                return False

            # Get request body
            body = await request.read()

            # Calculate expected signature
            expected = hmac.new(
                self.webhook_secret.encode(),
                body,
                hashlib.sha256
            ).hexdigest()

            # Compare signatures
            return hmac.compare_digest(signature, expected)

        except Exception as e:
            logger.error(f"Error verifying webhook signature: {e}")
            return False

    async def _parse_notification(self, request: Request) -> Optional[Dict[str, Any]]:
        """Parse Gmail push notification from Cloud Pub/Sub.

        Args:
            request: HTTP request

        Returns:
            Parsed notification data
        """
        try:
            # Parse JSON payload
            payload = await request.json()

            # Cloud Pub/Sub sends notifications in specific format
            if 'message' not in payload:
                return None

            message = payload['message']

            # Decode base64 data
            if 'data' in message:
                data_b64 = message['data']
                data_json = base64.b64decode(data_b64).decode('utf-8')
                notification_data = json.loads(data_json)
            else:
                notification_data = {}

            # Extract Gmail notification information
            return {
                'email_address': notification_data.get('emailAddress'),
                'history_id': notification_data.get('historyId'),
                'subscription': message.get('subscription'),
                'publish_time': message.get('publishTime'),
                'message_id': message.get('messageId'),
                'attributes': message.get('attributes', {})
            }

        except Exception as e:
            logger.error(f"Error parsing notification: {e}")
            return None

    async def _process_notification(self, notification: Dict[str, Any]) -> None:
        """Process Gmail push notification.

        Args:
            notification: Parsed notification data
        """
        try:
            email_address = notification.get('email_address')
            history_id = notification.get('history_id')

            if not email_address or not history_id:
                logger.warning(f"Incomplete notification data: {notification}")
                return

            logger.info(f"Processing notification for {email_address}, history_id: {history_id}")

            # Find Gmail connection for this email address
            async with db_manager.get_session() as session:
                result = await session.execute(
                    select(GmailConnection).where(
                        GmailConnection.account_email == email_address,
                        GmailConnection.is_active == True
                    )
                )
                connection = result.scalar_one_or_none()

                if not connection:
                    logger.warning(f"No active connection found for {email_address}")
                    return

                # Check if sync is already in progress for this connection
                if connection.id in self._active_syncs:
                    logger.debug(f"Sync already in progress for connection {connection.id}")
                    return

                # Start incremental sync
                await self._start_incremental_sync(connection.id, history_id)

        except Exception as e:
            logger.error(f"Error processing notification: {e}")

    async def _start_incremental_sync(self, connection_id: int, new_history_id: str) -> None:
        """Start incremental sync for a connection.

        Args:
            connection_id: Gmail connection ID
            new_history_id: New history ID from notification
        """
        async with self._sync_semaphore:
            try:
                # Create sync manager
                sync_manager = await create_sync_manager(connection_id)

                # Create sync task
                task = asyncio.create_task(
                    self._run_incremental_sync(sync_manager, new_history_id)
                )

                # Track active sync
                self._active_syncs[connection_id] = task

                # Wait for completion
                await task

            except Exception as e:
                logger.error(f"Error in incremental sync for connection {connection_id}: {e}")
            finally:
                # Remove from active syncs
                self._active_syncs.pop(connection_id, None)

    async def _run_incremental_sync(
        self,
        sync_manager: GmailSyncManager,
        new_history_id: str
    ) -> None:
        """Run incremental synchronization using Gmail History API.

        Args:
            sync_manager: Gmail sync manager
            new_history_id: New history ID to sync to
        """
        try:
            # Get current history ID for connection
            async with db_manager.get_session() as session:
                result = await session.execute(
                    select(GmailConnection).where(
                        GmailConnection.id == sync_manager.connection_id
                    )
                )
                connection = result.scalar_one_or_none()

                if not connection:
                    logger.error(f"Connection {sync_manager.connection_id} not found")
                    return

                start_history_id = connection.current_history_id

                # If no previous history ID, do a small historical sync
                if not start_history_id:
                    logger.info(f"No previous history ID for connection {connection.id}, doing small historical sync")
                    await sync_manager.sync_historical_emails(
                        max_results=50,
                        query="newer_than:7d"  # Last 7 days
                    )
                    return

                # Get history changes
                history_data = await sync_manager.gmail_client.get_history(
                    start_history_id=start_history_id,
                    max_results=500
                )

                if 'history' not in history_data:
                    logger.debug("No history changes found")
                    return

                # Process history changes
                messages_to_sync = []
                for history_record in history_data['history']:
                    # Process added messages
                    if 'messagesAdded' in history_record:
                        for msg_added in history_record['messagesAdded']:
                            messages_to_sync.append(msg_added['message']['id'])

                    # Process deleted messages (mark as deleted)
                    if 'messagesDeleted' in history_record:
                        for msg_deleted in history_record['messagesDeleted']:
                            await self._mark_message_deleted(
                                session,
                                msg_deleted['message']['id']
                            )

                # Sync new messages
                if messages_to_sync:
                    logger.info(f"Syncing {len(messages_to_sync)} new messages")
                    await self._sync_specific_messages(sync_manager, messages_to_sync)

                # Update history ID
                connection.current_history_id = new_history_id
                connection.last_sync = datetime.utcnow()
                await session.commit()

                logger.info(f"Incremental sync completed for connection {connection.id}")

        except Exception as e:
            logger.error(f"Error in incremental sync: {e}")
            raise

    async def _sync_specific_messages(
        self,
        sync_manager: GmailSyncManager,
        message_ids: List[str]
    ) -> None:
        """Sync specific messages by ID.

        Args:
            sync_manager: Gmail sync manager
            message_ids: List of Gmail message IDs to sync
        """
        for message_id in message_ids:
            try:
                result = await sync_manager._process_single_message(message_id)
                if result:
                    logger.debug(f"Processed message {message_id}")
            except Exception as e:
                logger.error(f"Error processing message {message_id}: {e}")

    async def _mark_message_deleted(
        self,
        session,
        gmail_message_id: str
    ) -> None:
        """Mark message as deleted in database.

        Args:
            session: Database session
            gmail_message_id: Gmail message ID that was deleted
        """
        # Note: In a production system, you might want to soft-delete
        # or move to a deleted messages table instead of hard delete
        logger.info(f"Message {gmail_message_id} was deleted from Gmail")
        # Implementation depends on business requirements


class GmailPollingMonitor:
    """Fallback polling monitor for when push notifications fail."""

    def __init__(
        self,
        webhook_handler: GmailWebhookHandler,
        poll_interval: int = 300  # 5 minutes default
    ):
        """Initialize polling monitor.

        Args:
            webhook_handler: Webhook handler for processing notifications
            poll_interval: Polling interval in seconds
        """
        self.webhook_handler = webhook_handler
        self.poll_interval = poll_interval
        self._polling_task: Optional[asyncio.Task] = None
        self._stop_event = asyncio.Event()

    async def start_polling(self) -> None:
        """Start polling monitor."""
        if self._polling_task and not self._polling_task.done():
            logger.warning("Polling already active")
            return

        logger.info(f"Starting Gmail polling monitor (interval: {self.poll_interval}s)")
        self._stop_event.clear()
        self._polling_task = asyncio.create_task(self._poll_loop())

    async def stop_polling(self) -> None:
        """Stop polling monitor."""
        logger.info("Stopping Gmail polling monitor")
        self._stop_event.set()

        if self._polling_task:
            await self._polling_task

    async def _poll_loop(self) -> None:
        """Main polling loop."""
        while not self._stop_event.is_set():
            try:
                await self._poll_all_connections()
            except Exception as e:
                logger.error(f"Error in polling loop: {e}")

            # Wait for next poll interval or stop event
            try:
                await asyncio.wait_for(
                    self._stop_event.wait(),
                    timeout=self.poll_interval
                )
                break  # Stop event was set
            except asyncio.TimeoutError:
                continue  # Continue polling

    async def _poll_all_connections(self) -> None:
        """Poll all active Gmail connections."""
        async with db_manager.get_session() as session:
            result = await session.execute(
                select(GmailConnection).where(
                    GmailConnection.is_active == True
                )
            )
            connections = result.scalars().all()

            for connection in connections:
                try:
                    await self._poll_connection(connection)
                except Exception as e:
                    logger.error(f"Error polling connection {connection.id}: {e}")

    async def _poll_connection(self, connection: GmailConnection) -> None:
        """Poll a single Gmail connection for changes.

        Args:
            connection: Gmail connection to poll
        """
        try:
            # Create sync manager
            sync_manager = await create_sync_manager(connection.id)

            # Get current profile to check for changes
            profile = await sync_manager.gmail_client.get_profile()
            current_history_id = profile.get('historyId')

            if not current_history_id:
                logger.warning(f"No history ID in profile for connection {connection.id}")
                return

            # Check if history ID has changed
            if connection.current_history_id == current_history_id:
                logger.debug(f"No changes for connection {connection.id}")
                return

            # Simulate a notification to trigger incremental sync
            fake_notification = {
                'email_address': connection.account_email,
                'history_id': current_history_id,
                'subscription_id': 'polling',
                'publish_time': datetime.utcnow().isoformat(),
                'message_id': f"poll_{connection.id}_{datetime.utcnow().timestamp()}",
                'attributes': {'source': 'polling'}
            }

            logger.info(f"Polling detected changes for {connection.account_email}")
            await self.webhook_handler._process_notification(fake_notification)

        except Exception as e:
            logger.error(f"Error polling connection {connection.id}: {e}")


def create_webhook_app(webhook_handler: GmailWebhookHandler) -> web.Application:
    """Create aiohttp web application for webhook handling.

    Args:
        webhook_handler: Gmail webhook handler

    Returns:
        Configured web application
    """
    app = web.Application()

    # Add webhook endpoint
    app.router.add_post('/gmail/webhook', webhook_handler.handle_notification)

    # Add health check endpoint
    async def health_check(request: Request) -> Response:
        return web.Response(text="OK")

    app.router.add_get('/health', health_check)

    return app