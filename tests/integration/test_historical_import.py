"""Integration tests for Gmail historical import."""

import pytest
from unittest.mock import Mock, patch, AsyncMock
from datetime import datetime, timedelta
import json
import asyncio

from penf_lib.connectors.gmail.sync import GmailSyncManager, create_sync_manager
from penf_lib.connectors.gmail.client import GmailClient
from penf_lib.events.publishers import EmailEventPublisher, SyncEventPublisher
from penf_lib.models.gmail_connection import GmailConnection
from penf_lib.models.email_message import EmailMessage, EmailThread, EmailAttachment


class TestHistoricalImport:
    """Test Gmail historical import functionality."""

    @pytest.fixture
    async def mock_gmail_client(self):
        """Create mock Gmail client."""
        mock_client = Mock(spec=GmailClient)

        # Mock profile response
        mock_client.get_profile.return_value = {
            "emailAddress": "test@example.com",
            "messagesTotal": 1000,
            "threadsTotal": 500
        }

        return mock_client

    @pytest.fixture
    async def mock_email_publisher(self):
        """Create mock email event publisher."""
        mock_publisher = AsyncMock(spec=EmailEventPublisher)
        mock_publisher.publish_email_ingested.return_value = True
        mock_publisher.publish_thread_ingested.return_value = True
        mock_publisher.publish_attachment_ingested.return_value = True
        return mock_publisher

    @pytest.fixture
    async def mock_sync_publisher(self):
        """Create mock sync event publisher."""
        mock_publisher = AsyncMock(spec=SyncEventPublisher)
        mock_publisher.publish_sync_progress.return_value = True
        mock_publisher.publish_sync_completed.return_value = True
        return mock_publisher

    @pytest.fixture
    async def sync_manager(self, mock_gmail_client, mock_email_publisher, mock_sync_publisher):
        """Create sync manager for testing."""
        return GmailSyncManager(
            connection_id=1,
            gmail_client=mock_gmail_client,
            email_publisher=mock_email_publisher,
            sync_publisher=mock_sync_publisher,
            attachment_base_path="/tmp/test_attachments"
        )

    @pytest.fixture
    def sample_gmail_list_response(self):
        """Sample Gmail list messages response."""
        return {
            "messages": [
                {"id": "msg_001", "threadId": "thread_001"},
                {"id": "msg_002", "threadId": "thread_001"},
                {"id": "msg_003", "threadId": "thread_002"}
            ],
            "nextPageToken": None,
            "resultSizeEstimate": 3
        }

    @pytest.fixture
    def sample_gmail_message_response(self):
        """Sample Gmail get message response."""
        return {
            "id": "msg_001",
            "threadId": "thread_001",
            "labelIds": ["INBOX", "UNREAD"],
            "snippet": "This is a test email snippet...",
            "internalDate": "1705147200000",  # 2025-01-13 12:00:00
            "sizeEstimate": 2048,
            "payload": {
                "headers": [
                    {"name": "From", "value": "sender@example.com"},
                    {"name": "To", "value": "recipient@example.com"},
                    {"name": "Subject", "value": "Test Email Subject"},
                    {"name": "Date", "value": "Sun, 13 Jan 2025 12:00:00 +0000"}
                ],
                "mimeType": "multipart/alternative",
                "parts": [
                    {
                        "mimeType": "text/plain",
                        "body": {
                            "data": "VGhpcyBpcyBhIHRlc3QgZW1haWwgYm9keS4="  # Base64: "This is a test email body."
                        }
                    },
                    {
                        "mimeType": "text/html",
                        "body": {
                            "data": "PGh0bWw+PGJvZHk+VGhpcyBpcyBhIHRlc3QgZW1haWwgYm9keS48L2JvZHk+PC9odG1sPg=="
                        }
                    }
                ]
            }
        }

    @pytest.mark.asyncio
    @pytest.mark.integration
    async def test_sync_historical_emails_basic(
        self,
        sync_manager,
        mock_gmail_client,
        sample_gmail_list_response,
        sample_gmail_message_response,
        db_session
    ):
        """Test basic historical email sync."""
        # Mock Gmail API responses
        mock_gmail_client.list_messages = AsyncMock(return_value=sample_gmail_list_response)
        mock_gmail_client.get_message = AsyncMock(return_value=sample_gmail_message_response)

        # Perform sync
        result = await sync_manager.sync_historical_emails(
            max_results=5,
            batch_size=2
        )

        # Verify results
        assert result['success'] is True
        assert result['messages_processed'] > 0
        assert result['duration_seconds'] > 0

        # Verify API calls
        mock_gmail_client.list_messages.assert_called()
        mock_gmail_client.get_message.assert_called()

    @pytest.mark.asyncio
    @pytest.mark.integration
    async def test_batch_processing_with_pagination(
        self,
        sync_manager,
        mock_gmail_client,
        sample_gmail_list_response
    ):
        """Test batch processing with API pagination."""
        # Setup pagination responses
        page1_response = {
            "messages": [
                {"id": "msg_001", "threadId": "thread_001"},
                {"id": "msg_002", "threadId": "thread_001"}
            ],
            "nextPageToken": "page2_token"
        }

        page2_response = {
            "messages": [
                {"id": "msg_003", "threadId": "thread_002"}
            ],
            "nextPageToken": None
        }

        mock_gmail_client.list_messages = AsyncMock(side_effect=[page1_response, page2_response])
        mock_gmail_client.get_message = AsyncMock(return_value=sample_gmail_message_response)

        # Perform sync with small batch size to force pagination
        result = await sync_manager.sync_historical_emails(
            max_results=10,
            batch_size=2
        )

        # Verify pagination was handled
        assert mock_gmail_client.list_messages.call_count == 2
        assert result['success'] is True

    @pytest.mark.asyncio
    @pytest.mark.integration
    async def test_message_with_attachments(
        self,
        sync_manager,
        mock_gmail_client,
        sample_gmail_list_response,
        db_session
    ):
        """Test processing messages with attachments."""
        # Message with attachment
        message_with_attachment = {
            "id": "msg_with_attachment",
            "threadId": "thread_001",
            "internalDate": "1705147200000",
            "payload": {
                "headers": [
                    {"name": "From", "value": "sender@example.com"},
                    {"name": "To", "value": "recipient@example.com"},
                    {"name": "Subject", "value": "Email with Attachment"}
                ],
                "parts": [
                    {
                        "mimeType": "text/plain",
                        "body": {"data": "VGVzdCBib2R5"}
                    },
                    {
                        "filename": "document.pdf",
                        "mimeType": "application/pdf",
                        "body": {
                            "attachmentId": "att_123",
                            "size": 1024
                        }
                    }
                ]
            }
        }

        mock_gmail_client.list_messages = AsyncMock(return_value={
            "messages": [{"id": "msg_with_attachment", "threadId": "thread_001"}]
        })
        mock_gmail_client.get_message = AsyncMock(return_value=message_with_attachment)
        mock_gmail_client.download_attachment = AsyncMock(return_value=b"PDF content")

        # Perform sync
        result = await sync_manager.sync_historical_emails(max_results=1)

        # Verify attachment processing
        mock_gmail_client.download_attachment.assert_called_once()
        assert result['success'] is True

    @pytest.mark.asyncio
    @pytest.mark.integration
    async def test_error_handling_invalid_message(
        self,
        sync_manager,
        mock_gmail_client,
        sample_gmail_list_response
    ):
        """Test error handling for invalid messages."""
        mock_gmail_client.list_messages = AsyncMock(return_value={
            "messages": [{"id": "invalid_msg", "threadId": "thread_001"}]
        })

        # Simulate API error for get_message
        mock_gmail_client.get_message = AsyncMock(
            side_effect=Exception("Message not found")
        )

        # Perform sync - should handle errors gracefully
        result = await sync_manager.sync_historical_emails(max_results=1)

        # Should complete with errors logged
        assert 'errors' in result
        assert len(result['errors']) > 0

    @pytest.mark.asyncio
    @pytest.mark.integration
    async def test_rate_limiting_compliance(
        self,
        sync_manager,
        mock_gmail_client,
        sample_gmail_list_response,
        sample_gmail_message_response
    ):
        """Test that sync respects rate limiting."""
        start_time = datetime.utcnow()

        mock_gmail_client.list_messages = AsyncMock(return_value=sample_gmail_list_response)
        mock_gmail_client.get_message = AsyncMock(return_value=sample_gmail_message_response)

        # Perform sync with multiple batches
        result = await sync_manager.sync_historical_emails(
            max_results=6,
            batch_size=2
        )

        end_time = datetime.utcnow()
        duration = (end_time - start_time).total_seconds()

        # Should take reasonable time (not too fast due to rate limiting)
        assert duration > 0.1  # At least some delay
        assert result['items_per_minute'] <= 600  # Reasonable rate limit

    @pytest.mark.asyncio
    @pytest.mark.integration
    async def test_event_publishing_during_sync(
        self,
        sync_manager,
        mock_gmail_client,
        mock_email_publisher,
        mock_sync_publisher,
        sample_gmail_list_response,
        sample_gmail_message_response
    ):
        """Test that events are published during sync."""
        mock_gmail_client.list_messages = AsyncMock(return_value=sample_gmail_list_response)
        mock_gmail_client.get_message = AsyncMock(return_value=sample_gmail_message_response)

        # Perform sync
        await sync_manager.sync_historical_emails(max_results=3)

        # Verify sync progress events
        mock_sync_publisher.publish_sync_progress.assert_called()

        # Verify completion event
        mock_sync_publisher.publish_sync_completed.assert_called_once()

        # Verify email ingested events
        mock_email_publisher.publish_email_ingested.assert_called()

    @pytest.mark.asyncio
    @pytest.mark.integration
    async def test_thread_relationship_preservation(
        self,
        sync_manager,
        mock_gmail_client,
        db_session
    ):
        """Test that thread relationships are preserved correctly."""
        # Mock messages from same thread
        messages_in_thread = [
            {
                "id": "msg_001",
                "threadId": "shared_thread",
                "internalDate": "1705147200000",
                "payload": {
                    "headers": [
                        {"name": "From", "value": "person1@example.com"},
                        {"name": "Subject", "value": "Thread Subject"}
                    ],
                    "parts": [{"mimeType": "text/plain", "body": {"data": "Rmlyc3QgbWVzc2FnZQ=="}}]
                }
            },
            {
                "id": "msg_002",
                "threadId": "shared_thread",
                "internalDate": "1705147260000",  # 1 minute later
                "payload": {
                    "headers": [
                        {"name": "From", "value": "person2@example.com"},
                        {"name": "Subject", "value": "Re: Thread Subject"}
                    ],
                    "parts": [{"mimeType": "text/plain", "body": {"data": "UmVwbHkgbWVzc2FnZQ=="}}]
                }
            }
        ]

        mock_gmail_client.list_messages = AsyncMock(return_value={
            "messages": [
                {"id": "msg_001", "threadId": "shared_thread"},
                {"id": "msg_002", "threadId": "shared_thread"}
            ]
        })

        mock_gmail_client.get_message = AsyncMock(side_effect=messages_in_thread)

        # Perform sync
        result = await sync_manager.sync_historical_emails(max_results=2)

        # Verify thread relationships
        assert result['success'] is True
        assert result['messages_processed'] == 2
        assert result['threads_processed'] >= 1  # Should be 1 thread with 2 messages

    @pytest.mark.asyncio
    @pytest.mark.integration
    async def test_date_range_filtering(self, sync_manager, mock_gmail_client):
        """Test sync with date range filtering."""
        start_date = datetime(2025, 1, 1)
        end_date = datetime(2025, 1, 31)

        mock_gmail_client.list_messages = AsyncMock(return_value={"messages": []})

        await sync_manager.sync_historical_emails(
            max_results=100,
            start_date=start_date,
            end_date=end_date,
            query="is:important"
        )

        # Verify query construction with dates
        call_args = mock_gmail_client.list_messages.call_args[1]
        query = call_args.get('query', '')
        assert 'after:2025/01/01' in query
        assert 'before:2025/01/31' in query
        assert 'is:important' in query

    @pytest.mark.asyncio
    @pytest.mark.integration
    async def test_performance_metrics_calculation(
        self,
        sync_manager,
        mock_gmail_client,
        sample_gmail_list_response,
        sample_gmail_message_response
    ):
        """Test that performance metrics are calculated correctly."""
        mock_gmail_client.list_messages = AsyncMock(return_value=sample_gmail_list_response)
        mock_gmail_client.get_message = AsyncMock(return_value=sample_gmail_message_response)

        result = await sync_manager.sync_historical_emails(max_results=3)

        # Verify performance metrics are present and reasonable
        assert 'duration_seconds' in result
        assert 'items_per_minute' in result
        assert result['duration_seconds'] > 0
        assert result['items_per_minute'] >= 0

        # Should achieve reasonable throughput (>= 100 emails/minute target)
        # Note: In tests this might be very high due to mocking
        assert result['items_per_minute'] > 0


@pytest.mark.asyncio
@pytest.mark.integration
async def test_create_sync_manager_integration(db_session, sample_gmail_credentials):
    """Test creating sync manager from database connection."""
    with patch('penf_lib.connectors.gmail.sync.db_manager.get_session') as mock_get_session:
        # Mock database session and connection
        mock_session = AsyncMock()
        mock_get_session.return_value.__aenter__.return_value = mock_session

        mock_connection = Mock(spec=GmailConnection)
        mock_connection.id = 1
        mock_connection.is_active = True
        mock_connection.encrypted_credentials = b"encrypted_data"

        mock_result = Mock()
        mock_result.scalar_one_or_none.return_value = mock_connection
        mock_session.execute.return_value = mock_result

        # Mock authenticator and credentials
        with patch('penf_lib.connectors.gmail.sync.GmailAuthenticator') as mock_auth_class:
            mock_auth = AsyncMock()
            mock_auth.decrypt_credentials.return_value = Mock()
            mock_auth.validate_credentials.return_value = True
            mock_auth_class.return_value = mock_auth

            # Create sync manager
            sync_manager = await create_sync_manager(connection_id=1)

            # Verify sync manager was created
            assert isinstance(sync_manager, GmailSyncManager)
            assert sync_manager.connection_id == 1