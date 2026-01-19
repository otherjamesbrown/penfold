#!/usr/bin/env python3
"""Gmail monitoring daemon for real-time synchronization."""

import asyncio
import logging
import signal
import sys
from pathlib import Path
from typing import Optional
import argparse
import os

# Add parent directory to path for imports
sys.path.insert(0, str(Path(__file__).parent.parent))

from aiohttp import web
from penf_lib.connectors.gmail.webhook import (
    GmailWebhookHandler,
    GmailPollingMonitor,
    create_webhook_app
)
from penf_lib.storage.database import db_manager


logger = logging.getLogger(__name__)


class GmailMonitorDaemon:
    """Gmail monitoring daemon that handles webhooks and fallback polling."""

    def __init__(
        self,
        webhook_port: int = 8080,
        webhook_secret: Optional[str] = None,
        poll_interval: int = 300,
        max_concurrent_syncs: int = 5
    ):
        """Initialize Gmail monitor daemon.

        Args:
            webhook_port: Port for webhook server
            webhook_secret: Secret for webhook signature verification
            poll_interval: Fallback polling interval in seconds
            max_concurrent_syncs: Maximum concurrent sync operations
        """
        self.webhook_port = webhook_port
        self.webhook_secret = webhook_secret or os.getenv('GMAIL_WEBHOOK_SECRET')
        self.poll_interval = poll_interval
        self.max_concurrent_syncs = max_concurrent_syncs

        # Components
        self.webhook_handler: Optional[GmailWebhookHandler] = None
        self.polling_monitor: Optional[GmailPollingMonitor] = None
        self.web_app: Optional[web.Application] = None
        self.app_runner: Optional[web.AppRunner] = None
        self.site: Optional[web.TCPSite] = None

        # State
        self._shutdown_event = asyncio.Event()
        self._running = False

    async def start(self) -> None:
        """Start the Gmail monitoring daemon."""
        if self._running:
            logger.warning("Gmail monitor daemon is already running")
            return

        logger.info("Starting Gmail monitoring daemon")

        try:
            # Initialize database
            await db_manager.initialize()

            # Create webhook handler
            self.webhook_handler = GmailWebhookHandler(
                webhook_secret=self.webhook_secret,
                max_concurrent_syncs=self.max_concurrent_syncs
            )

            # Create polling monitor
            self.polling_monitor = GmailPollingMonitor(
                webhook_handler=self.webhook_handler,
                poll_interval=self.poll_interval
            )

            # Create and start web application
            self.web_app = create_webhook_app(self.webhook_handler)
            self.app_runner = web.AppRunner(self.web_app)
            await self.app_runner.setup()

            self.site = web.TCPSite(self.app_runner, '0.0.0.0', self.webhook_port)
            await self.site.start()

            logger.info(f"Webhook server started on port {self.webhook_port}")

            # Start polling monitor
            await self.polling_monitor.start_polling()
            logger.info(f"Polling monitor started (interval: {self.poll_interval}s)")

            self._running = True
            logger.info("Gmail monitoring daemon started successfully")

            # Wait for shutdown signal
            await self._shutdown_event.wait()

        except Exception as e:
            logger.error(f"Failed to start Gmail monitor daemon: {e}")
            raise
        finally:
            await self.stop()

    async def stop(self) -> None:
        """Stop the Gmail monitoring daemon."""
        if not self._running:
            return

        logger.info("Stopping Gmail monitoring daemon")

        try:
            # Stop polling monitor
            if self.polling_monitor:
                await self.polling_monitor.stop_polling()

            # Stop web server
            if self.site:
                await self.site.stop()

            if self.app_runner:
                await self.app_runner.cleanup()

            # Close webhook handler
            if self.webhook_handler:
                # Close any remaining sync operations
                for task in self.webhook_handler._active_syncs.values():
                    if not task.done():
                        task.cancel()
                        try:
                            await task
                        except asyncio.CancelledError:
                            pass

            # Close database
            await db_manager.close()

            self._running = False
            logger.info("Gmail monitoring daemon stopped")

        except Exception as e:
            logger.error(f"Error stopping Gmail monitor daemon: {e}")

    def shutdown(self) -> None:
        """Signal shutdown of the daemon."""
        logger.info("Shutdown signal received")
        self._shutdown_event.set()

    async def setup_push_notifications(
        self,
        connection_ids: Optional[list] = None,
        topic_name: str = "projects/your-project/topics/gmail-notifications"
    ) -> None:
        """Set up push notifications for Gmail connections.

        Args:
            connection_ids: List of connection IDs to setup (None for all active)
            topic_name: Cloud Pub/Sub topic name
        """
        from penf_lib.connectors.gmail.sync import create_sync_manager
        from penf_lib.models.gmail_connection import GmailConnection
        from sqlalchemy import select

        logger.info("Setting up Gmail push notifications")

        async with db_manager.get_session() as session:
            # Get connections to setup
            if connection_ids:
                query = select(GmailConnection).where(
                    GmailConnection.id.in_(connection_ids),
                    GmailConnection.is_active == True
                )
            else:
                query = select(GmailConnection).where(
                    GmailConnection.is_active == True
                )

            result = await session.execute(query)
            connections = result.scalars().all()

            for connection in connections:
                try:
                    logger.info(f"Setting up notifications for {connection.account_email}")

                    # Create sync manager
                    sync_manager = await create_sync_manager(connection.id)

                    # Setup push notifications
                    watch_response = await sync_manager.setup_push_notifications(
                        topic_name=topic_name
                    )

                    logger.info(f"Push notifications setup for {connection.account_email}: "
                               f"expires {watch_response.get('expiration')}")

                except Exception as e:
                    logger.error(f"Failed to setup notifications for connection {connection.id}: {e}")

    async def health_check(self) -> dict:
        """Perform health check of the monitoring system.

        Returns:
            Health status dictionary
        """
        health = {
            'status': 'healthy',
            'webhook_server': False,
            'polling_monitor': False,
            'database': False,
            'active_syncs': 0,
            'errors': []
        }

        try:
            # Check webhook server
            if self.site and self._running:
                health['webhook_server'] = True

            # Check polling monitor
            if self.polling_monitor and self.polling_monitor._polling_task:
                health['polling_monitor'] = not self.polling_monitor._polling_task.done()

            # Check database connectivity
            try:
                await db_manager.get_session().__aenter__()
                health['database'] = True
            except Exception as e:
                health['errors'].append(f"Database error: {e}")

            # Check active syncs
            if self.webhook_handler:
                health['active_syncs'] = len(self.webhook_handler._active_syncs)

            # Overall status
            if not all([health['webhook_server'], health['polling_monitor'], health['database']]):
                health['status'] = 'degraded'

            if health['errors']:
                health['status'] = 'unhealthy'

        except Exception as e:
            health['status'] = 'unhealthy'
            health['errors'].append(f"Health check error: {e}")

        return health


async def main():
    """Main function for Gmail monitor daemon."""
    parser = argparse.ArgumentParser(description='Gmail monitoring daemon')
    parser.add_argument('--port', type=int, default=8080, help='Webhook port')
    parser.add_argument('--secret', help='Webhook secret for signature verification')
    parser.add_argument('--poll-interval', type=int, default=300, help='Polling interval in seconds')
    parser.add_argument('--max-syncs', type=int, default=5, help='Maximum concurrent syncs')
    parser.add_argument('--setup-push', action='store_true', help='Setup push notifications and exit')
    parser.add_argument('--topic', default='projects/your-project/topics/gmail-notifications',
                       help='Pub/Sub topic name for push notifications')
    parser.add_argument('--debug', action='store_true', help='Enable debug logging')

    args = parser.parse_args()

    # Configure logging
    log_level = logging.DEBUG if args.debug else logging.INFO
    logging.basicConfig(
        level=log_level,
        format='%(asctime)s - %(name)s - %(levelname)s - %(message)s'
    )

    # Create daemon
    daemon = GmailMonitorDaemon(
        webhook_port=args.port,
        webhook_secret=args.secret,
        poll_interval=args.poll_interval,
        max_concurrent_syncs=args.max_syncs
    )

    # Setup push notifications if requested
    if args.setup_push:
        await daemon.setup_push_notifications(topic_name=args.topic)
        logger.info("Push notifications setup completed")
        return

    # Setup signal handlers
    def signal_handler(sig, frame):
        logger.info(f"Received signal {sig}")
        daemon.shutdown()

    signal.signal(signal.SIGINT, signal_handler)
    signal.signal(signal.SIGTERM, signal_handler)

    try:
        # Start daemon
        await daemon.start()
    except KeyboardInterrupt:
        logger.info("Received keyboard interrupt")
    except Exception as e:
        logger.error(f"Daemon failed: {e}")
        sys.exit(1)


if __name__ == "__main__":
    try:
        asyncio.run(main())
    except KeyboardInterrupt:
        logger.info("Gmail monitor daemon stopped")
        sys.exit(0)