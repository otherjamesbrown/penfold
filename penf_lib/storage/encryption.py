"""Credential encryption utilities for secure storage."""

import json
import os
from pathlib import Path
from typing import Dict, Any
from cryptography.fernet import Fernet
from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.kdf.pbkdf2 import PBKDF2HMAC
import base64
import logging

logger = logging.getLogger(__name__)


def _get_salt_path() -> Path:
    """Get the path to the encryption salt file."""
    return Path(os.getenv('PENF_SALT_PATH', os.path.expanduser('~/.penfold/encryption_salt')))


def _load_or_create_salt() -> bytes:
    """Load existing salt or generate new one on first use.

    Returns:
        16-byte random salt
    """
    salt_path = _get_salt_path()
    if salt_path.exists():
        return salt_path.read_bytes()
    # First run: generate and persist salt
    salt_path.parent.mkdir(parents=True, exist_ok=True)
    salt = os.urandom(16)
    salt_path.write_bytes(salt)
    return salt


class CredentialEncryption:
    """Handles encryption/decryption of sensitive credential data."""

    def __init__(self, password: str = None):
        """Initialize encryption with key derivation.

        Args:
            password: Master password for key derivation.
                     If None, will check environment variable PENF_MASTER_KEY
        """
        self.password = password or os.getenv('PENF_MASTER_KEY', 'default-dev-key')
        # Validate master key in production
        if self.password == 'default-dev-key':
            from penf_lib.storage.config import config
            if config.is_production:
                raise ValueError(
                    'PENF_MASTER_KEY is required in production. '
                    'Set the PENF_MASTER_KEY environment variable to a secure random string.'
                )
            else:
                logger.warning(
                    'Using default development encryption key. '
                    'Set PENF_MASTER_KEY environment variable for production use.'
                )
        self._fernet = None

    def _get_fernet(self) -> Fernet:
        """Get or create Fernet instance with derived key."""
        if self._fernet is None:
            # Load per-installation salt (generated on first use)
            salt = _load_or_create_salt()
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