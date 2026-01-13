"""Unit tests for Gmail authentication."""

import pytest
from unittest.mock import Mock, patch, AsyncMock
from datetime import datetime, timedelta

from penf_lib.connectors.gmail.auth import GmailAuthenticator
from penf_lib.storage.encryption import CredentialEncryption


class TestGmailAuthenticator:
    """Test Gmail OAuth2 authentication."""

    @pytest.fixture
    def authenticator(self, gmail_client_config):
        """Create GmailAuthenticator instance for testing."""
        return GmailAuthenticator(gmail_client_config)

    def test_initialization(self, authenticator, gmail_client_config):
        """Test authenticator initialization."""
        assert authenticator.client_config == gmail_client_config
        assert 'gmail.readonly' in str(authenticator.scopes)
        assert authenticator.redirect_uri == "http://localhost:8080/auth/callback"

    def test_custom_scopes_and_redirect(self, gmail_client_config):
        """Test authenticator with custom scopes and redirect URI."""
        custom_scopes = ['https://www.googleapis.com/auth/gmail.modify']
        custom_redirect = "http://localhost:9000/callback"

        authenticator = GmailAuthenticator(
            gmail_client_config,
            scopes=custom_scopes,
            redirect_uri=custom_redirect
        )

        assert authenticator.scopes == custom_scopes
        assert authenticator.redirect_uri == custom_redirect

    @pytest.mark.asyncio
    @patch('penf_lib.connectors.gmail.auth.Flow.from_client_config')
    async def test_initiate_auth_flow(self, mock_flow_class, authenticator):
        """Test OAuth2 authorization flow initiation."""
        # Mock the flow instance
        mock_flow = Mock()
        mock_flow.authorization_url.return_value = (
            "https://accounts.google.com/o/oauth2/auth?client_id=test",
            "state_token"
        )
        mock_flow_class.return_value = mock_flow

        auth_url = await authenticator.initiate_auth_flow()

        assert auth_url.startswith("https://accounts.google.com/o/oauth2/auth")
        mock_flow_class.assert_called_once()
        mock_flow.authorization_url.assert_called_once_with(
            access_type='offline',
            include_granted_scopes='true',
            prompt='consent'
        )

    @pytest.mark.asyncio
    @patch('penf_lib.connectors.gmail.auth.Flow.from_client_config')
    async def test_complete_auth_flow(self, mock_flow_class, authenticator):
        """Test OAuth2 flow completion."""
        # Mock credentials
        mock_credentials = Mock()
        mock_credentials.token = "access_token"
        mock_credentials.refresh_token = "refresh_token"

        # Mock the flow instance
        mock_flow = Mock()
        mock_flow.credentials = mock_credentials
        mock_flow_class.return_value = mock_flow

        authorization_response = "http://localhost:8080/auth/callback?code=auth_code&state=state"
        result = await authenticator.complete_auth_flow(authorization_response)

        assert result == mock_credentials
        mock_flow.fetch_token.assert_called_once_with(
            authorization_response=authorization_response
        )

    @pytest.mark.asyncio
    @patch('penf_lib.connectors.gmail.auth.Request')
    async def test_refresh_credentials(self, mock_request, authenticator):
        """Test credential refresh."""
        mock_credentials = Mock()
        mock_credentials.refresh_token = "refresh_token"

        result = await authenticator.refresh_credentials(mock_credentials)

        assert result == mock_credentials
        mock_credentials.refresh.assert_called_once()

    @pytest.mark.asyncio
    async def test_encrypt_decrypt_credentials(self, authenticator, sample_gmail_credentials):
        """Test credential encryption and decryption."""
        # Create mock credentials
        mock_credentials = Mock()
        mock_credentials.token = sample_gmail_credentials["token"]
        mock_credentials.refresh_token = sample_gmail_credentials["refresh_token"]
        mock_credentials.token_uri = sample_gmail_credentials["token_uri"]
        mock_credentials.client_id = sample_gmail_credentials["client_id"]
        mock_credentials.client_secret = sample_gmail_credentials["client_secret"]
        mock_credentials.scopes = sample_gmail_credentials["scopes"]
        mock_credentials.expiry = datetime.utcnow() + timedelta(hours=1)

        # Encrypt credentials
        encrypted_data = await authenticator.encrypt_credentials(mock_credentials)
        assert isinstance(encrypted_data, bytes)
        assert len(encrypted_data) > 0

        # Decrypt credentials
        decrypted_credentials = await authenticator.decrypt_credentials(encrypted_data)
        assert decrypted_credentials.token == mock_credentials.token
        assert decrypted_credentials.refresh_token == mock_credentials.refresh_token
        assert decrypted_credentials.client_id == mock_credentials.client_id

    @pytest.mark.asyncio
    async def test_validate_credentials_valid(self, authenticator):
        """Test credential validation with valid credentials."""
        mock_credentials = Mock()
        mock_credentials.valid = True
        mock_credentials.expiry = datetime.utcnow() + timedelta(hours=2)

        result = await authenticator.validate_credentials(mock_credentials)
        assert result is True

    @pytest.mark.asyncio
    async def test_validate_credentials_invalid(self, authenticator):
        """Test credential validation with invalid credentials."""
        mock_credentials = Mock()
        mock_credentials.valid = False

        result = await authenticator.validate_credentials(mock_credentials)
        assert result is False

    @pytest.mark.asyncio
    async def test_validate_credentials_none(self, authenticator):
        """Test credential validation with None credentials."""
        result = await authenticator.validate_credentials(None)
        assert result is False

    @pytest.mark.asyncio
    @patch.object(GmailAuthenticator, 'refresh_credentials')
    async def test_validate_credentials_expiring_soon(self, mock_refresh, authenticator):
        """Test credential validation with credentials expiring soon."""
        mock_credentials = Mock()
        mock_credentials.valid = True
        mock_credentials.expiry = datetime.utcnow() + timedelta(minutes=30)  # Expiring in 30 min
        mock_credentials.refresh_token = "refresh_token"

        mock_refresh.return_value = mock_credentials

        result = await authenticator.validate_credentials(mock_credentials)

        assert result is True
        mock_refresh.assert_called_once_with(mock_credentials)