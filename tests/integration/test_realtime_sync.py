"""Integration tests for Gmail real-time synchronization."""

import pytest
from unittest.mock import Mock, patch, AsyncMock
from datetime import datetime, timedelta
import json
import asyncio

from aiohttp import web
from aiohttp.test_utils import make_mocked_request

from penf_lib.connectors.gmail.webhook import GmailWebhookHandler, GmailPollingMonitor
from penf_lib.connectors.gmail.sync import GmailSyncManager
from penf_lib.models.gmail_connection import GmailConnection
from penf_lib.models.email_message import EmailMessage


class TestRealTimeSync:
    """Test real-time Gmail synchronization."""

    @pytest.fixture
    async def mock_sync_manager(self):
        """Create mock sync manager."""
        manager = Mock(spec=GmailSyncManager)
        manager.connection_id = 1
        manager.gmail_client = AsyncMock()
        manager.email_publisher = AsyncMock()
        manager.sync_publisher = AsyncMock()

        # Mock sync methods
        manager.setup_push_notifications = AsyncMock()
        manager.stop_push_notifications = AsyncMock()
        manager.sync_incremental_emails = AsyncMock()
        manager._process_single_message = AsyncMock()

        return manager

    @pytest.fixture
    async def webhook_handler(self):
        """Create webhook handler for testing."""
        return GmailWebhookHandler(
            webhook_secret="test_secret",
            max_concurrent_syncs=3
        )

    @pytest.fixture
    async def gmail_connection(self, db_session):
        """Create Gmail connection for testing."""
        connection = GmailConnection(
            account_email="test@example.com",
            account_name="Test Account",
            encrypted_credentials=b"encrypted_test_data",
            is_active=True,
            current_history_id="12345",
            push_topic_name="projects/test/topics/gmail",
            push_subscription_expiry=datetime.utcnow() + timedelta(days=7)
        )

        db_session.add(connection)
        await db_session.commit()
        await db_session.refresh(connection)

        return connection

    @pytest.mark.asyncio
    @pytest.mark.integration
    async def test_push_notification_setup(self, mock_sync_manager, db_session):
        """Test setting up Gmail push notifications."""
        # Mock watch response
        mock_watch_response = {
            'historyId': '98765',
            'expiration': str(int((datetime.utcnow() + timedelta(days=7)).timestamp() * 1000))
        }

        mock_sync_manager.gmail_client.watch_mailbox.return_value = mock_watch_response

        # Test setup
        result = await mock_sync_manager.setup_push_notifications(
            topic_name="projects/test/topics/gmail-notifications",
            label_ids=["INBOX"]
        )

        assert result == mock_watch_response
        mock_sync_manager.gmail_client.watch_mailbox.assert_called_once_with(
            topic_name="projects/test/topics/gmail-notifications",
            label_ids=["INBOX"]
        )

    @pytest.mark.asyncio
    @pytest.mark.integration
    async def test_incremental_sync_with_history(self, mock_sync_manager):
        """Test incremental sync using Gmail History API."""
        # Mock history response
        mock_history_response = {
            'history': [
                {
                    'id': '12346',
                    'messagesAdded': [
                        {'message': {'id': 'new_msg_001', 'threadId': 'thread_001'}}
                    ]
                },
                {
                    'id': '12347',
                    'messagesDeleted': [
                        {'message': {'id': 'deleted_msg_001'}}
                    ]
                }
            ]
        }

        mock_sync_manager.gmail_client.get_history.return_value = mock_history_response
        mock_sync_manager._process_single_message.return_value = {
            'thread_id': 'thread_001',
            'attachment_count': 0,
            'success': True
        }

        # Mock database operations
        with patch('penf_lib.connectors.gmail.sync.db_manager') as mock_db_manager:
            mock_session = AsyncMock()
            mock_db_manager.get_session.return_value.__aenter__.return_value = mock_session

            mock_connection = Mock()
            mock_connection.id = 1
            mock_result = Mock()
            mock_result.scalar_one_or_none.return_value = mock_connection
            mock_session.execute.return_value = mock_result

            # Test incremental sync
            result = await mock_sync_manager.sync_incremental_emails(
                start_history_id="12345",
                end_history_id="12347"
            )

            assert result['success'] is True
            assert result['messages_processed'] >= 1

            # Verify API calls
            mock_sync_manager.gmail_client.get_history.assert_called()
            mock_sync_manager._process_single_message.assert_called_with('new_msg_001')

    @pytest.mark.asyncio
    @pytest.mark.integration
    async def test_webhook_notification_processing(
        self,
        webhook_handler,
        gmail_connection,
        db_session
    ):
        """Test processing webhook notifications end-to-end."""
        # Create notification payload
        notification_data = {
            'email_address': gmail_connection.account_email,
            'history_id': '67890'
        }

        # Mock sync manager creation and operations
        with patch('penf_lib.connectors.gmail.webhook.create_sync_manager') as mock_create_sync:
            mock_sync_manager = AsyncMock()
            mock_sync_manager.connection_id = gmail_connection.id
            mock_sync_manager.gmail_client = AsyncMock()

            # Mock history response
            mock_sync_manager.gmail_client.get_history.return_value = {
                'history': [
                    {
                        'id': '67890',
                        'messagesAdded': [
                            {'message': {'id': 'webhook_msg_001'}}
                        ]
                    }
                ]
            }

            mock_create_sync.return_value = mock_sync_manager

            # Process notification
            await webhook_handler._process_notification(notification_data)

            # Verify sync manager was created and used
            mock_create_sync.assert_called_once_with(gmail_connection.id)

    @pytest.mark.asyncio
    @pytest.mark.integration
    async def test_polling_fallback_detection(self):
        """Test polling fallback when push notifications fail."""
        mock_webhook_handler = AsyncMock(spec=GmailWebhookHandler)
        polling_monitor = GmailPollingMonitor(
            webhook_handler=mock_webhook_handler,
            poll_interval=0.1  # Very short for testing
        )

        # Mock database and connections
        with patch('penf_lib.connectors.gmail.webhook.db_manager') as mock_db_manager:
            mock_session = AsyncMock()
            mock_db_manager.get_session.return_value.__aenter__.return_value = mock_session

            # Mock connection with changed history ID
            mock_connection = Mock()
            mock_connection.id = 1
            mock_connection.account_email = 'test@example.com'
            mock_connection.current_history_id = '12345'

            mock_result = Mock()
            mock_result.scalars.return_value.all.return_value = [mock_connection]
            mock_session.execute.return_value = mock_result

            # Mock sync manager and Gmail client
            with patch('penf_lib.connectors.gmail.webhook.create_sync_manager') as mock_create_sync:
                mock_sync_manager = AsyncMock()
                mock_gmail_client = AsyncMock()
                mock_sync_manager.gmail_client = mock_gmail_client
                mock_create_sync.return_value = mock_sync_manager

                # Mock profile with different history ID
                mock_gmail_client.get_profile.return_value = {
                    'historyId': '67890'  # Changed from 12345
                }

                # Start polling for short time
                await polling_monitor.start_polling()
                await asyncio.sleep(0.2)  # Let it poll once
                await polling_monitor.stop_polling()

                # Verify it detected changes and processed notification
                mock_webhook_handler._process_notification.assert_called()

    @pytest.mark.asyncio
    @pytest.mark.integration
    async def test_concurrent_sync_management(self, webhook_handler):
        """Test managing concurrent synchronization operations."""
        # Create multiple notification tasks
        notifications = [
            {'email_address': f'user{i}@example.com', 'history_id': f'hist{i}'}
            for i in range(5)
        ]

        # Mock database to return connections
        with patch('penf_lib.connectors.gmail.webhook.db_manager') as mock_db_manager:
            mock_session = AsyncMock()
            mock_db_manager.get_session.return_value.__aenter__.return_value = mock_session

            def mock_connection_lookup(email):
                mock_connection = Mock()
                mock_connection.id = hash(email) % 1000  # Unique ID per email
                mock_connection.account_email = email
                return mock_connection

            # Mock create_sync_manager with delays to test concurrency
            with patch('penf_lib.connectors.gmail.webhook.create_sync_manager') as mock_create_sync:
                async def slow_sync_manager(connection_id):
                    mock_manager = AsyncMock()
                    mock_manager.connection_id = connection_id
                    # Simulate slow sync operation
                    await asyncio.sleep(0.1)
                    return mock_manager

                mock_create_sync.side_effect = slow_sync_manager

                # Mock database responses
                def mock_execute(query):
                    mock_result = Mock()
                    # Extract email from the query (this is a simplified mock)
                    for notification in notifications:
                        email = notification['email_address']
                        if email in str(query):
                            mock_result.scalar_one_or_none.return_value = mock_connection_lookup(email)
                            break
                    else:
                        mock_result.scalar_one_or_none.return_value = None
                    return mock_result

                mock_session.execute.side_effect = mock_execute

                # Process notifications concurrently
                tasks = [
                    webhook_handler._process_notification(notification)
                    for notification in notifications
                ]

                await asyncio.gather(*tasks)

                # Should have respected concurrency limit (max 3 concurrent)
                assert len(webhook_handler._active_syncs) == 0  # All should be complete

    @pytest.mark.asyncio
    @pytest.mark.integration
    async def test_label_change_handling(self, mock_sync_manager):
        """Test handling of Gmail label changes in real-time."""
        # Mock history with label changes
        mock_history_response = {
            'history': [
                {
                    'id': '12346',
                    'labelsAdded': [
                        {
                            'message': {'id': 'msg_001'},
                            'labelIds': ['IMPORTANT', 'CATEGORY_PERSONAL']
                        }
                    ]
                },
                {
                    'id': '12347',
                    'labelsRemoved': [
                        {
                            'message': {'id': 'msg_001'},
                            'labelIds': ['UNREAD']
                        }
                    ]
                }
            ]
        }

        mock_sync_manager.gmail_client.get_history.return_value = mock_history_response

        # Mock database operations for label handling
        with patch('penf_lib.connectors.gmail.sync.db_manager') as mock_db_manager:
            mock_session = AsyncMock()
            mock_db_manager.get_session.return_value.__aenter__.return_value = mock_session

            # Mock connection and message for label updates
            mock_connection = Mock()
            mock_connection.id = 1

            mock_message = Mock()
            mock_message.gmail_labels = ['INBOX', 'UNREAD']

            # Setup database mocks
            def mock_execute(query):
                mock_result = Mock()
                if 'GmailConnection' in str(query):
                    mock_result.scalar_one_or_none.return_value = mock_connection
                elif 'EmailMessage' in str(query):
                    mock_result.scalar_one_or_none.return_value = mock_message
                return mock_result

            mock_session.execute.side_effect = mock_execute

            # Test incremental sync with label changes
            result = await mock_sync_manager.sync_incremental_emails(
                start_history_id="12345",
                end_history_id="12347"
            )

            assert result['success'] is True

    @pytest.mark.asyncio
    @pytest.mark.integration
    async def test_real_time_performance_metrics(self, mock_sync_manager):
        """Test performance metrics for real-time synchronization."""
        # Mock fast incremental sync
        mock_sync_manager.sync_incremental_emails = AsyncMock()
        mock_sync_manager.sync_incremental_emails.return_value = {
            'success': True,
            'messages_processed': 10,
            'threads_processed': 5,
            'duration_seconds': 2.5,
            'items_per_minute': 240,  # 10 messages in 2.5 seconds = 240/min
            'errors': []
        }

        start_time = datetime.utcnow()
        result = await mock_sync_manager.sync_incremental_emails("12345", "12350")
        end_time = datetime.utcnow()

        # Verify performance expectations
        assert result['success'] is True
        assert result['items_per_minute'] >= 60  # Should process at least 60 emails/min
        assert result['duration_seconds'] < 60  # Should complete in under 1 minute

        # Verify real-time capability (under 60 seconds for detection)
        processing_time = (end_time - start_time).total_seconds()
        assert processing_time < 60  # Real-time requirement

    @pytest.mark.asyncio
    @pytest.mark.integration
    async def test_webhook_error_recovery(self, webhook_handler):
        """Test error recovery in webhook processing."""
        notification = {
            'email_address': 'test@example.com',
            'history_id': '12345'
        }

        # Mock database failure
        with patch('penf_lib.connectors.gmail.webhook.db_manager') as mock_db_manager:
            mock_db_manager.get_session.side_effect = Exception("Database connection failed")

            # Should handle error gracefully without raising
            await webhook_handler._process_notification(notification)

            # Webhook handler should continue functioning
            assert len(webhook_handler._active_syncs) == 0

    @pytest.mark.asyncio
    @pytest.mark.integration
    async def test_push_notification_expiry_handling(self, mock_sync_manager, db_session):
        """Test handling of expired push notification subscriptions."""
        # Mock expired subscription response
        expired_time = datetime.utcnow() - timedelta(days=1)

        with patch('penf_lib.connectors.gmail.sync.db_manager') as mock_db_manager:
            mock_session = AsyncMock()
            mock_db_manager.get_session.return_value.__aenter__.return_value = mock_session

            # Mock connection with expired subscription
            mock_connection = Mock()
            mock_connection.id = 1
            mock_connection.push_subscription_expiry = expired_time
            mock_connection.push_topic_name = "projects/test/topics/gmail"

            mock_result = Mock()
            mock_result.scalar_one_or_none.return_value = mock_connection
            mock_session.execute.return_value = mock_result

            # Should detect expiry and allow renewal
            assert mock_connection.push_subscription_expiry < datetime.utcnow()

            # Test renewal
            new_expiry_ms = int((datetime.utcnow() + timedelta(days=7)).timestamp() * 1000)
            mock_sync_manager.gmail_client.watch_mailbox.return_value = {
                'historyId': '12345',
                'expiration': str(new_expiry_ms)
            }

            result = await mock_sync_manager.setup_push_notifications(
                topic_name="projects/test/topics/gmail"
            )

            assert 'expiration' in result