"""Unit tests for Gmail webhook handler."""

import pytest
from unittest.mock import Mock, patch, AsyncMock, MagicMock
import json
import base64
import hmac
import hashlib
import asyncio
from datetime import datetime

from aiohttp import web
from aiohttp.test_utils import make_mocked_request

from penf_lib.connectors.gmail.webhook import (
    GmailWebhookHandler,
    GmailPollingMonitor,
    create_webhook_app
)


class TestGmailWebhookHandler:
    """Test Gmail webhook handler."""

    @pytest.fixture
    def webhook_handler(self):
        """Create webhook handler for testing."""
        return GmailWebhookHandler(
            webhook_secret="test_secret",
            max_concurrent_syncs=3
        )

    @pytest.fixture
    def webhook_handler_no_secret(self):
        """Create webhook handler without secret verification."""
        return GmailWebhookHandler(max_concurrent_syncs=3)

    @pytest.fixture
    def sample_pubsub_notification(self):
        """Sample Cloud Pub/Sub notification payload."""
        notification_data = {
            "emailAddress": "test@example.com",
            "historyId": "12345678"
        }

        message_data = json.dumps(notification_data)
        encoded_data = base64.b64encode(message_data.encode()).decode()

        return {
            "message": {
                "data": encoded_data,
                "messageId": "msg_123456",
                "publishTime": "2025-01-13T12:00:00Z",
                "subscription": "projects/test/subscriptions/gmail-sub"
            }
        }

    @pytest.fixture
    def signed_request_body(self, sample_pubsub_notification):
        """Create signed request body for testing."""
        body = json.dumps(sample_pubsub_notification).encode()
        signature = hmac.new(
            "test_secret".encode(),
            body,
            hashlib.sha256
        ).hexdigest()

        return body, signature

    @pytest.mark.asyncio
    async def test_handle_notification_with_valid_signature(
        self,
        webhook_handler,
        sample_pubsub_notification,
        signed_request_body
    ):
        """Test handling notification with valid signature."""
        body, signature = signed_request_body

        # Create mock request with proper async methods
        request = Mock()
        request.headers = {'X-Goog-Signature': signature}
        request.read = AsyncMock(return_value=body)
        request.json = AsyncMock(return_value=sample_pubsub_notification)

        # Mock the notification processing
        with patch.object(webhook_handler, '_process_notification') as mock_process:
            mock_process.return_value = None

            response = await webhook_handler.handle_notification(request)

            assert response.status == 200
            mock_process.assert_called_once()

    @pytest.mark.asyncio
    async def test_handle_notification_invalid_signature(
        self,
        webhook_handler,
        sample_pubsub_notification
    ):
        """Test handling notification with invalid signature."""
        body = json.dumps(sample_pubsub_notification).encode()

        # Create mock request with invalid signature
        request = Mock()
        request.headers = {'X-Goog-Signature': 'invalid_signature'}
        request.read = AsyncMock(return_value=body)
        request.json = AsyncMock(return_value=sample_pubsub_notification)

        response = await webhook_handler.handle_notification(request)

        assert response.status == 401

    @pytest.mark.asyncio
    async def test_handle_notification_no_signature_verification(
        self,
        webhook_handler_no_secret,
        sample_pubsub_notification
    ):
        """Test handling notification without signature verification."""
        body = json.dumps(sample_pubsub_notification).encode()

        request = Mock()
        request.headers = {}
        request.read = AsyncMock(return_value=body)
        request.json = AsyncMock(return_value=sample_pubsub_notification)

        # Mock the notification processing
        with patch.object(webhook_handler_no_secret, '_process_notification') as mock_process:
            mock_process.return_value = None

            response = await webhook_handler_no_secret.handle_notification(request)

            assert response.status == 200
            mock_process.assert_called_once()

    @pytest.mark.asyncio
    async def test_parse_notification_valid(self, webhook_handler, sample_pubsub_notification):
        """Test parsing valid notification."""
        body = json.dumps(sample_pubsub_notification).encode()

        request = Mock()
        request.json = AsyncMock(return_value=sample_pubsub_notification)

        notification = await webhook_handler._parse_notification(request)

        assert notification is not None
        assert notification['email_address'] == "test@example.com"
        assert notification['history_id'] == "12345678"
        assert notification['subscription'] == "projects/test/subscriptions/gmail-sub"

    @pytest.mark.asyncio
    async def test_parse_notification_invalid(self, webhook_handler):
        """Test parsing invalid notification."""
        invalid_payload = {"invalid": "data"}

        request = Mock()
        request.json = AsyncMock(return_value=invalid_payload)

        notification = await webhook_handler._parse_notification(request)
        assert notification is None

    @pytest.mark.asyncio
    async def test_process_notification_success(self, webhook_handler):
        """Test successful notification processing."""
        notification = {
            'email_address': 'test@example.com',
            'history_id': '12345678'
        }

        # Mock database and sync manager
        with patch('penf_lib.connectors.gmail.webhook.db_manager') as mock_db_manager, \
             patch.object(webhook_handler, '_start_incremental_sync') as mock_start_sync:

            # Mock database session and connection
            mock_session = AsyncMock()
            mock_db_manager.get_session.return_value.__aenter__.return_value = mock_session

            mock_connection = Mock()
            mock_connection.id = 1
            mock_connection.account_email = 'test@example.com'

            mock_result = Mock()
            mock_result.scalar_one_or_none.return_value = mock_connection
            mock_session.execute.return_value = mock_result

            await webhook_handler._process_notification(notification)

            mock_start_sync.assert_called_once_with(1, '12345678')

    @pytest.mark.asyncio
    async def test_process_notification_no_connection(self, webhook_handler):
        """Test notification processing when no connection found."""
        notification = {
            'email_address': 'unknown@example.com',
            'history_id': '12345678'
        }

        # Mock database with no connection found
        with patch('penf_lib.connectors.gmail.webhook.db_manager') as mock_db_manager:
            mock_session = AsyncMock()
            mock_db_manager.get_session.return_value.__aenter__.return_value = mock_session

            mock_result = Mock()
            mock_result.scalar_one_or_none.return_value = None
            mock_session.execute.return_value = mock_result

            # Should handle gracefully without raising exception
            await webhook_handler._process_notification(notification)

    @pytest.mark.asyncio
    async def test_start_incremental_sync_concurrency_limit(self, webhook_handler):
        """Test that incremental sync respects concurrency limits."""
        # Test basic concurrency tracking
        assert len(webhook_handler._active_syncs) == 0
        assert webhook_handler.max_concurrent_syncs == 3

        # Verify semaphore is created with correct size
        assert webhook_handler._sync_semaphore._value == 3

    @pytest.mark.asyncio
    async def test_verify_signature_valid(self, webhook_handler):
        """Test signature verification with valid signature."""
        body = b"test message body"
        expected_signature = hmac.new(
            "test_secret".encode(),
            body,
            hashlib.sha256
        ).hexdigest()

        request = Mock()
        request.headers = {'X-Goog-Signature': expected_signature}
        request.read = AsyncMock(return_value=body)

        result = await webhook_handler._verify_signature(request)
        assert result is True

    @pytest.mark.asyncio
    async def test_verify_signature_invalid(self, webhook_handler):
        """Test signature verification with invalid signature."""
        body = b"test message body"

        request = Mock()
        request.headers = {'X-Goog-Signature': 'invalid_signature'}
        request.read = AsyncMock(return_value=body)

        result = await webhook_handler._verify_signature(request)
        assert result is False

    @pytest.mark.asyncio
    async def test_verify_signature_missing_header(self, webhook_handler):
        """Test signature verification with missing signature header."""
        body = b"test message body"

        request = Mock()
        request.headers = {}
        request.read = AsyncMock(return_value=body)

        result = await webhook_handler._verify_signature(request)
        assert result is False


class TestGmailPollingMonitor:
    """Test Gmail polling monitor."""

    @pytest.fixture
    def webhook_handler_mock(self):
        """Mock webhook handler."""
        return Mock(spec=GmailWebhookHandler)

    @pytest.fixture
    def polling_monitor(self, webhook_handler_mock):
        """Create polling monitor for testing."""
        return GmailPollingMonitor(
            webhook_handler=webhook_handler_mock,
            poll_interval=1  # Short interval for testing
        )

    @pytest.mark.asyncio
    async def test_start_stop_polling(self, polling_monitor):
        """Test starting and stopping polling."""
        # Start polling
        await polling_monitor.start_polling()
        assert polling_monitor._polling_task is not None
        assert not polling_monitor._polling_task.done()

        # Stop polling
        await polling_monitor.stop_polling()
        assert polling_monitor._polling_task.done()

    @pytest.mark.asyncio
    async def test_poll_all_connections(self, polling_monitor):
        """Test polling all connections."""
        # Mock database and connections
        with patch('penf_lib.connectors.gmail.webhook.db_manager') as mock_db_manager:
            mock_session = AsyncMock()
            mock_db_manager.get_session.return_value.__aenter__.return_value = mock_session

            # Mock two connections
            mock_connection1 = Mock()
            mock_connection1.id = 1
            mock_connection1.account_email = 'test1@example.com'
            mock_connection1.current_history_id = '12345'

            mock_connection2 = Mock()
            mock_connection2.id = 2
            mock_connection2.account_email = 'test2@example.com'
            mock_connection2.current_history_id = '67890'

            mock_result = Mock()
            mock_result.scalars.return_value.all.return_value = [mock_connection1, mock_connection2]
            mock_session.execute.return_value = mock_result

            # Mock _poll_connection
            with patch.object(polling_monitor, '_poll_connection') as mock_poll_connection:
                await polling_monitor._poll_all_connections()

                # Should have polled both connections
                assert mock_poll_connection.call_count == 2

    @pytest.mark.asyncio
    async def test_poll_connection_no_changes(self, polling_monitor):
        """Test polling connection with no changes."""
        mock_connection = Mock()
        mock_connection.id = 1
        mock_connection.account_email = 'test@example.com'
        mock_connection.current_history_id = '12345'

        # Mock create_sync_manager and Gmail client
        with patch('penf_lib.connectors.gmail.webhook.create_sync_manager') as mock_create_sync:
            mock_sync_manager = AsyncMock()
            mock_gmail_client = AsyncMock()
            mock_sync_manager.gmail_client = mock_gmail_client
            mock_create_sync.return_value = mock_sync_manager

            # Mock profile response with same history ID
            mock_gmail_client.get_profile.return_value = {
                'historyId': '12345'  # Same as current
            }

            await polling_monitor._poll_connection(mock_connection)

            # Should not trigger notification processing
            assert not polling_monitor.webhook_handler._process_notification.called

    @pytest.mark.asyncio
    async def test_poll_connection_with_changes(self, polling_monitor):
        """Test polling connection with changes detected."""
        mock_connection = Mock()
        mock_connection.id = 1
        mock_connection.account_email = 'test@example.com'
        mock_connection.current_history_id = '12345'

        # Mock create_sync_manager and Gmail client
        with patch('penf_lib.connectors.gmail.webhook.create_sync_manager') as mock_create_sync:
            mock_sync_manager = AsyncMock()
            mock_gmail_client = AsyncMock()
            mock_sync_manager.gmail_client = mock_gmail_client
            mock_create_sync.return_value = mock_sync_manager

            # Mock profile response with different history ID
            mock_gmail_client.get_profile.return_value = {
                'historyId': '67890'  # Different from current
            }

            # Mock webhook handler processing
            polling_monitor.webhook_handler._process_notification = AsyncMock()

            await polling_monitor._poll_connection(mock_connection)

            # Should trigger notification processing
            polling_monitor.webhook_handler._process_notification.assert_called_once()

            # Verify notification data
            call_args = polling_monitor.webhook_handler._process_notification.call_args[0][0]
            assert call_args['email_address'] == 'test@example.com'
            assert call_args['history_id'] == '67890'


def test_create_webhook_app():
    """Test creating webhook application."""
    mock_handler = Mock(spec=GmailWebhookHandler)
    app = create_webhook_app(mock_handler)

    assert isinstance(app, web.Application)

    # Check routes are configured
    routes = [route.resource.canonical for route in app.router.routes()]
    assert '/gmail/webhook' in routes
    assert '/health' in routes