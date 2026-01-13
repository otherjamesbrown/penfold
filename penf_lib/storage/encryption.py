"""Credential encryption utilities for secure storage."""

import json
import os
from typing import Dict, Any
from cryptography.fernet import Fernet
from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.kdf.pbkdf2 import PBKDF2HMAC
import base64


class CredentialEncryption:
    """Handles encryption/decryption of sensitive credential data."""

    def __init__(self, password: str = None):
        """Initialize encryption with key derivation.

        Args:
            password: Master password for key derivation.
                     If None, will check environment variable PENF_MASTER_KEY
        """
        self.password = password or os.getenv('PENF_MASTER_KEY', 'default-dev-key')
        self._fernet = None

    def _get_fernet(self) -> Fernet:
        """Get or create Fernet instance with derived key."""
        if self._fernet is None:
            # Use a static salt for now (should be configurable in production)
            salt = b'penfold-static-salt'
            kdf = PBKDF2HMAC(
                algorithm=hashes.SHA256(),
                length=32,
                salt=salt,
                iterations=100000,
            )
            key = base64.urlsafe_b64encode(kdf.derive(self.password.encode()))
            self._fernet = Fernet(key)
        return self._fernet

    async def encrypt(self, data: Dict[str, Any]) -> bytes:
        """Encrypt credential data.

        Args:
            data: Dictionary of credential data

        Returns:
            Encrypted data as bytes
        """
        json_data = json.dumps(data).encode()
        fernet = self._get_fernet()
        return fernet.encrypt(json_data)

    async def decrypt(self, encrypted_data: bytes) -> Dict[str, Any]:
        """Decrypt credential data.

        Args:
            encrypted_data: Encrypted data bytes

        Returns:
            Decrypted credential dictionary
        """
        fernet = self._get_fernet()
        decrypted_json = fernet.decrypt(encrypted_data)
        return json.loads(decrypted_json.decode())

    @staticmethod
    def generate_key() -> str:
        """Generate a new encryption key.

        Returns:
            Base64 encoded encryption key
        """
        return Fernet.generate_key().decode()