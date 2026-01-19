"""Integration tests for Gmail OAuth2 flow."""

import pytest
from unittest.mock import Mock, patch, AsyncMock
from datetime import datetime, timedelta

from penf_lib.connectors.gmail.auth import GmailAuthenticator
from penf_lib.models.gmail_connection import GmailConnection


class TestOAuthFlow:
    """Test complete OAuth2 authentication flow integration."""

    @pytest.fixture
    def authenticator(self, gmail_client_config):
        """Create GmailAuthenticator instance for testing."""
        return GmailAuthenticator(gmail_client_config)

    @pytest.mark.asyncio
    @pytest.mark.integration
    async def test_complete_oauth_workflow(
        self, authenticator, db_session, sample_gmail_credentials
    ):
        """Test complete OAuth2 workflow from initiation to credential storage."""
        # Mock the OAuth2 flow components
        with patch('penf_lib.connectors.gmail.auth.Flow.from_client_config') as mock_flow_class:
            # Setup mock flow for initiation
            mock_flow = Mock()
            mock_flow.authorization_url.return_value = (
                "https://accounts.google.com/o/oauth2/auth?client_id=test&code_challenge=xyz",
                "state_token"
            )
            mock_flow_class.return_value = mock_flow

            # Test initiation
            auth_url = await authenticator.initiate_auth_flow()
            assert auth_url.startswith("https://accounts.google.com/o/oauth2/auth")

            # Mock credentials for completion
            mock_credentials = Mock()
            mock_credentials.token = sample_gmail_credentials["token"]
            mock_credentials.refresh_token = sample_gmail_credentials["refresh_token"]
            mock_credentials.token_uri = sample_gmail_credentials["token_uri"]
            mock_credentials.client_id = sample_gmail_credentials["client_id"]
            mock_credentials.client_secret = sample_gmail_credentials["client_secret"]
            mock_credentials.scopes = sample_gmail_credentials["scopes"]
            mock_credentials.expiry = datetime.utcnow() + timedelta(hours=1)
            mock_credentials.valid = True

            # Setup mock flow for completion
            mock_flow.credentials = mock_credentials
            mock_flow.fetch_token = Mock()

            # Test completion
            authorization_response = "http://localhost:8080/auth/callback?code=auth_code&state=state"
            credentials = await authenticator.complete_auth_flow(authorization_response)

            assert credentials == mock_credentials
            mock_flow.fetch_token.assert_called_once_with(
                authorization_response=authorization_response
            )

    @pytest.mark.asyncio
    @pytest.mark.integration
    async def test_credential_encryption_roundtrip(self, authenticator, sample_gmail_credentials):
        """Test credential encryption and decryption roundtrip."""
        # Create mock credentials
        mock_credentials = Mock()
        mock_credentials.token = sample_gmail_credentials["token"]
        mock_credentials.refresh_token = sample_gmail_credentials["refresh_token"]
        mock_credentials.token_uri = sample_gmail_credentials["token_uri"]
        mock_credentials.client_id = sample_gmail_credentials["client_id"]
        mock_credentials.client_secret = sample_gmail_credentials["client_secret"]
        mock_credentials.scopes = sample_gmail_credentials["scopes"]
        mock_credentials.expiry = datetime.utcnow() + timedelta(hours=2)

        # Test encryption
        encrypted_data = await authenticator.encrypt_credentials(mock_credentials)
        assert isinstance(encrypted_data, bytes)
        assert len(encrypted_data) > 0

        # Test decryption
        decrypted_credentials = await authenticator.decrypt_credentials(encrypted_data)

        # Verify all fields match
        assert decrypted_credentials.token == mock_credentials.token
        assert decrypted_credentials.refresh_token == mock_credentials.refresh_token
        assert decrypted_credentials.token_uri == mock_credentials.token_uri
        assert decrypted_credentials.client_id == mock_credentials.client_id
        assert decrypted_credentials.client_secret == mock_credentials.client_secret
        assert decrypted_credentials.scopes == mock_credentials.scopes

    @pytest.mark.asyncio
    @pytest.mark.integration
    async def test_credential_refresh_flow(self, authenticator):
        """Test credential refresh functionality."""
        with patch('penf_lib.connectors.gmail.auth.Request') as mock_request:
            # Create mock credentials that are expiring
            mock_credentials = Mock()
            mock_credentials.valid = True
            mock_credentials.refresh_token = "test_refresh_token"
            mock_credentials.expiry = datetime.utcnow() + timedelta(minutes=30)
            mock_credentials.refresh = Mock()

            # Test refresh
            result = await authenticator.refresh_credentials(mock_credentials)

            assert result == mock_credentials
            mock_credentials.refresh.assert_called_once()

    @pytest.mark.asyncio
    @pytest.mark.integration
    async def test_database_connection_integration(
        self, authenticator, db_session, sample_gmail_credentials
    ):
        """Test storing encrypted credentials in database."""
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

        # Store in database
        test_email = "test@example.com"
        connection = GmailConnection(
            account_email=test_email,
            account_name="Test Account",
            encrypted_credentials=encrypted_data,
            is_active=True
        )

        db_session.add(connection)
        await db_session.commit()

        # Verify storage
        assert connection.id is not None
        assert connection.account_email == test_email
        assert connection.encrypted_credentials == encrypted_data
        assert connection.is_active is True

        # Test decryption from database
        decrypted_credentials = await authenticator.decrypt_credentials(
            connection.encrypted_credentials
        )
        assert decrypted_credentials.token == mock_credentials.token
        assert decrypted_credentials.refresh_token == mock_credentials.refresh_token

    @pytest.mark.asyncio
    @pytest.mark.integration
    async def test_token_validation_with_refresh(self, authenticator):
        """Test token validation that triggers automatic refresh."""
        with patch.object(authenticator, 'refresh_credentials') as mock_refresh:
            # Create credentials expiring soon
            mock_credentials = Mock()
            mock_credentials.valid = True
            mock_credentials.expiry = datetime.utcnow() + timedelta(minutes=45)  # Expires in 45 min
            mock_credentials.refresh_token = "test_refresh_token"

            # Mock refresh to return the same credentials
            mock_refresh.return_value = mock_credentials

            # Test validation - should trigger refresh
            result = await authenticator.validate_credentials(mock_credentials)

            assert result is True
            mock_refresh.assert_called_once_with(mock_credentials)

    @pytest.mark.asyncio
    @pytest.mark.integration
    async def test_multiple_account_support(
        self, authenticator, db_session, sample_gmail_credentials
    ):
        """Test support for multiple Gmail accounts."""
        # Create credentials for two accounts
        accounts = [
            {
                "email": "account1@example.com",
                "name": "Account 1",
                "token": "token1"
            },
            {
                "email": "account2@example.com",
                "name": "Account 2",
                "token": "token2"
            }
        ]

        connections = []
        for account in accounts:
            # Create mock credentials
            mock_credentials = Mock()
            mock_credentials.token = account["token"]
            mock_credentials.refresh_token = sample_gmail_credentials["refresh_token"]
            mock_credentials.token_uri = sample_gmail_credentials["token_uri"]
            mock_credentials.client_id = sample_gmail_credentials["client_id"]
            mock_credentials.client_secret = sample_gmail_credentials["client_secret"]
            mock_credentials.scopes = sample_gmail_credentials["scopes"]
            mock_credentials.expiry = datetime.utcnow() + timedelta(hours=1)

            # Encrypt and store
            encrypted_data = await authenticator.encrypt_credentials(mock_credentials)

            connection = GmailConnection(
                account_email=account["email"],
                account_name=account["name"],
                encrypted_credentials=encrypted_data,
                is_active=True
            )

            db_session.add(connection)
            connections.append(connection)

        await db_session.commit()

        # Verify both accounts are stored
        assert len(connections) == 2
        assert connections[0].account_email == "account1@example.com"
        assert connections[1].account_email == "account2@example.com"

        # Verify credentials can be decrypted separately
        for i, connection in enumerate(connections):
            decrypted = await authenticator.decrypt_credentials(
                connection.encrypted_credentials
            )
            assert decrypted.token == f"token{i + 1}"