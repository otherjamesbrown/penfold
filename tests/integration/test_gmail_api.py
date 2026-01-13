"""Integration tests for Gmail API."""

import pytest
from unittest.mock import Mock, patch, AsyncMock

from penf_lib.connectors.gmail.client import GmailClient, GmailRateLimiter


class TestGmailClient:
    """Test Gmail API client integration."""

    @pytest.fixture
    def mock_credentials(self):
        """Mock Gmail OAuth2 credentials."""
        creds = Mock()
        creds.valid = True
        creds.token = "test_access_token"
        return creds

    @pytest.fixture
    def gmail_client(self, mock_credentials):
        """Create GmailClient instance for testing."""
        return GmailClient(mock_credentials)

    @pytest.mark.asyncio
    @patch('penf_lib.connectors.gmail.client.build')
    async def test_service_creation(self, mock_build, gmail_client):
        """Test Gmail API service creation."""
        mock_service = Mock()
        mock_build.return_value = mock_service

        service = gmail_client.service

        assert service == mock_service
        mock_build.assert_called_once_with('gmail', 'v1', credentials=gmail_client.credentials)

    @pytest.mark.asyncio
    @patch('penf_lib.connectors.gmail.client.build')
    async def test_get_profile(self, mock_build, gmail_client):
        """Test getting Gmail profile."""
        # Mock the API service chain
        mock_profile_data = {
            "emailAddress": "test@example.com",
            "messagesTotal": 1000,
            "threadsTotal": 500
        }

        mock_service = Mock()
        mock_users = Mock()
        mock_get_profile = Mock()
        mock_execute = Mock(return_value=mock_profile_data)

        mock_service.users.return_value = mock_users
        mock_users.getProfile.return_value = mock_get_profile
        mock_get_profile.execute = mock_execute

        mock_build.return_value = mock_service

        result = await gmail_client.get_profile()

        assert result == mock_profile_data
        mock_users.getProfile.assert_called_once_with(userId='me')

    @pytest.mark.asyncio
    @patch('penf_lib.connectors.gmail.client.build')
    async def test_list_messages(self, mock_build, gmail_client):
        """Test listing Gmail messages."""
        mock_messages_data = {
            "messages": [
                {"id": "msg1", "threadId": "thread1"},
                {"id": "msg2", "threadId": "thread2"}
            ],
            "nextPageToken": "next_token"
        }

        # Mock the API service chain
        mock_service = Mock()
        mock_users = Mock()
        mock_messages = Mock()
        mock_list = Mock()
        mock_execute = Mock(return_value=mock_messages_data)

        mock_service.users.return_value = mock_users
        mock_users.messages.return_value = mock_messages
        mock_messages.list.return_value = mock_list
        mock_list.execute = mock_execute

        mock_build.return_value = mock_service

        result = await gmail_client.list_messages(query="is:unread", max_results=10)

        assert result == mock_messages_data
        mock_messages.list.assert_called_once_with(
            userId='me',
            q='is:unread',
            maxResults=10,
            pageToken=None
        )

    @pytest.mark.asyncio
    @patch('penf_lib.connectors.gmail.client.build')
    async def test_get_message(self, mock_build, gmail_client, sample_gmail_message):
        """Test getting individual Gmail message."""
        # Mock the API service chain
        mock_service = Mock()
        mock_users = Mock()
        mock_messages = Mock()
        mock_get = Mock()
        mock_execute = Mock(return_value=sample_gmail_message)

        mock_service.users.return_value = mock_users
        mock_users.messages.return_value = mock_messages
        mock_messages.get.return_value = mock_get
        mock_get.execute = mock_execute

        mock_build.return_value = mock_service

        result = await gmail_client.get_message("test_message_id_123")

        assert result == sample_gmail_message
        mock_messages.get.assert_called_once_with(
            userId='me',
            id='test_message_id_123',
            format='full',
            metadataHeaders=None
        )

    @pytest.mark.asyncio
    async def test_rate_limiter(self):
        """Test rate limiting functionality."""
        rate_limiter = GmailRateLimiter(calls_per_second=2)

        # First calls should not wait
        await rate_limiter.wait_if_needed()
        await rate_limiter.wait_if_needed()

        # Should have 2 calls recorded
        assert len(rate_limiter.call_times) == 2

        # Third call within same second should trigger wait
        # Note: This is hard to test precisely without mocking time
        await rate_limiter.wait_if_needed()
        assert len(rate_limiter.call_times) == 3