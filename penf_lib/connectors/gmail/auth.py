"""Gmail OAuth2 authentication and credential management."""

from typing import Optional, Dict, Any
import asyncio
from pathlib import Path
from datetime import datetime, timedelta

from google.auth.transport.requests import Request
from google.oauth2.credentials import Credentials
from google_auth_oauthlib.flow import Flow
from cryptography.fernet import Fernet

from ...storage.encryption import CredentialEncryption


class GmailAuthenticator:
    """Handles OAuth2 authentication flow for Gmail API access."""

    def __init__(
        self,
        client_config: Dict[str, Any],
        scopes: Optional[list[str]] = None,
        redirect_uri: str = "http://localhost:8080/auth/callback"
    ) -> None:
        """Initialize authenticator with OAuth2 configuration.

        Args:
            client_config: OAuth2 client configuration from Google Console
            scopes: Gmail API scopes to request
            redirect_uri: Redirect URI for OAuth2 flow
        """
        self.client_config = client_config
        self.scopes = scopes or [
            'https://www.googleapis.com/auth/gmail.readonly',
            'https://www.googleapis.com/auth/gmail.modify',
        ]
        self.redirect_uri = redirect_uri
        self._encryption = CredentialEncryption()

    async def initiate_auth_flow(self) -> str:
        """Start OAuth2 authorization flow.

        Returns:
            Authorization URL for user to visit
        """
        flow = Flow.from_client_config(
            self.client_config,
            scopes=self.scopes,
            redirect_uri=self.redirect_uri
        )

        auth_url, _ = flow.authorization_url(
            access_type='offline',
            include_granted_scopes='true',
            prompt='consent'  # Force consent to get refresh token
        )

        return auth_url

    async def complete_auth_flow(self, authorization_response: str) -> Credentials:
        """Complete OAuth2 flow with authorization response.

        Args:
            authorization_response: Full callback URL with authorization code

        Returns:
            Gmail API credentials
        """
        flow = Flow.from_client_config(
            self.client_config,
            scopes=self.scopes,
            redirect_uri=self.redirect_uri
        )

        flow.fetch_token(authorization_response=authorization_response)
        return flow.credentials

    async def refresh_credentials(self, credentials: Credentials) -> Credentials:
        """Refresh expired OAuth2 credentials.

        Args:
            credentials: Existing credentials to refresh

        Returns:
            Refreshed credentials
        """
        if credentials.refresh_token:
            credentials.refresh(Request())
        return credentials

    async def encrypt_credentials(self, credentials: Credentials) -> bytes:
        """Encrypt credentials for secure storage.

        Args:
            credentials: OAuth2 credentials to encrypt

        Returns:
            Encrypted credential data
        """
        credential_data = {
            'token': credentials.token,
            'refresh_token': credentials.refresh_token,
            'token_uri': credentials.token_uri,
            'client_id': credentials.client_id,
            'client_secret': credentials.client_secret,
            'scopes': credentials.scopes,
            'expiry': credentials.expiry.isoformat() if credentials.expiry else None
        }

        return await self._encryption.encrypt(credential_data)

    async def decrypt_credentials(self, encrypted_data: bytes) -> Credentials:
        """Decrypt stored credentials.

        Args:
            encrypted_data: Encrypted credential data

        Returns:
            Decrypted OAuth2 credentials
        """
        credential_data = await self._encryption.decrypt(encrypted_data)

        expiry = None
        if credential_data.get('expiry'):
            expiry = datetime.fromisoformat(credential_data['expiry'])

        return Credentials(
            token=credential_data['token'],
            refresh_token=credential_data['refresh_token'],
            token_uri=credential_data['token_uri'],
            client_id=credential_data['client_id'],
            client_secret=credential_data['client_secret'],
            scopes=credential_data['scopes'],
            expiry=expiry
        )

    async def validate_credentials(self, credentials: Credentials) -> bool:
        """Check if credentials are valid and not expired.

        Args:
            credentials: Credentials to validate

        Returns:
            True if credentials are valid
        """
        if not credentials or not credentials.valid:
            return False

        # Check if expiring soon (within 1 hour)
        if credentials.expiry:
            expires_soon = credentials.expiry <= datetime.utcnow() + timedelta(hours=1)
            if expires_soon and credentials.refresh_token:
                # Auto-refresh if expiring soon
                await self.refresh_credentials(credentials)

        return credentials.valid