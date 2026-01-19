"""AES-256-GCM File Archiver.

Encrypted storage of original .eml files for compliance retrieval.

Features:
- AES-256-GCM authenticated encryption
- Tenant-scoped key derivation using HKDF
- Associated data for integrity binding

Reference: specs/012-manual-ingest/research.md Section 2
"""

import hashlib
import os
from dataclasses import dataclass
from typing import Optional
from uuid import UUID

from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.ciphers.aead import AESGCM
from cryptography.hazmat.primitives.kdf.hkdf import HKDF


class EncryptionError(Exception):
    """Error during encryption or decryption."""

    pass


class KeyDerivationError(Exception):
    """Error during key derivation."""

    pass


@dataclass
class ArchiveResult:
    """Result of file archival.

    Attributes:
        encrypted_content: Encrypted file bytes (nonce || ciphertext || tag)
        original_size: Size of original file in bytes
        content_hash: SHA-256 of original content
        encryption_key_id: Key identifier for rotation tracking
    """

    encrypted_content: bytes
    original_size: int
    content_hash: str
    encryption_key_id: str


class FileArchiver:
    """AES-256-GCM encrypted file archiver with tenant-scoped keys.

    Provides secure storage of original files with:
    - AES-256-GCM authenticated encryption
    - Random nonce per encryption (12 bytes)
    - Tenant-scoped keys derived via HKDF
    - Optional associated data for integrity binding

    Usage:
        master_key = os.environ["PENF_ARCHIVE_MASTER_KEY"].encode()
        archiver = FileArchiver(master_key, tenant_id)
        result = archiver.encrypt(content, source_id)
        decrypted = archiver.decrypt(result.encrypted_content, source_id)
    """

    NONCE_SIZE = 12  # GCM standard nonce size
    KEY_SIZE = 32  # 256 bits
    KEY_VERSION = 1  # For key rotation tracking

    def __init__(self, master_key: bytes, tenant_id: UUID):
        """Initialize archiver with master key and tenant.

        Args:
            master_key: Master encryption key (must be 32 bytes)
            tenant_id: Tenant ID for key derivation

        Raises:
            KeyDerivationError: If master key is invalid
        """
        if len(master_key) != self.KEY_SIZE:
            raise KeyDerivationError(
                f"Master key must be {self.KEY_SIZE} bytes, got {len(master_key)}"
            )

        self.tenant_id = tenant_id
        self.tenant_key = self._derive_tenant_key(master_key, tenant_id)
        self.aesgcm = AESGCM(self.tenant_key)
        self.encryption_key_id = f"tenant-{tenant_id}-v{self.KEY_VERSION}"

    def _derive_tenant_key(self, master_key: bytes, tenant_id: UUID) -> bytes:
        """Derive tenant-specific key from master key.

        Uses HKDF with SHA-256 and tenant_id as info parameter.

        Args:
            master_key: Master encryption key
            tenant_id: Tenant ID for key derivation

        Returns:
            32-byte tenant-specific key
        """
        hkdf = HKDF(
            algorithm=hashes.SHA256(),
            length=self.KEY_SIZE,
            salt=None,  # Salt not needed with high-entropy master key
            info=f"penfold-archive-{tenant_id}".encode(),
        )
        return hkdf.derive(master_key)

    def encrypt(
        self, plaintext: bytes, source_id: Optional[int] = None
    ) -> ArchiveResult:
        """Encrypt file content with AES-256-GCM.

        Args:
            plaintext: Original file content
            source_id: Optional source ID for AAD binding

        Returns:
            ArchiveResult with encrypted data and metadata

        Raises:
            EncryptionError: If encryption fails
        """
        if not plaintext:
            raise EncryptionError("Cannot encrypt empty content")

        try:
            # Generate random nonce
            nonce = os.urandom(self.NONCE_SIZE)

            # Use source_id as associated data if provided
            aad = str(source_id).encode() if source_id else None

            # Encrypt (returns ciphertext || tag)
            ciphertext = self.aesgcm.encrypt(nonce, plaintext, aad)

            # Prepend nonce to ciphertext
            encrypted_content = nonce + ciphertext

            return ArchiveResult(
                encrypted_content=encrypted_content,
                original_size=len(plaintext),
                content_hash=hashlib.sha256(plaintext).hexdigest(),
                encryption_key_id=self.encryption_key_id,
            )
        except Exception as e:
            raise EncryptionError(f"Encryption failed: {e}") from e

    def decrypt(
        self, encrypted_content: bytes, source_id: Optional[int] = None
    ) -> bytes:
        """Decrypt file content.

        Args:
            encrypted_content: Encrypted data (nonce || ciphertext || tag)
            source_id: Source ID used during encryption (for AAD)

        Returns:
            Original plaintext bytes

        Raises:
            EncryptionError: If decryption fails (wrong key, tampered data)
        """
        if len(encrypted_content) < self.NONCE_SIZE + 16:  # nonce + min tag
            raise EncryptionError("Invalid encrypted content: too short")

        try:
            # Extract nonce
            nonce = encrypted_content[: self.NONCE_SIZE]
            ciphertext = encrypted_content[self.NONCE_SIZE :]

            # Use source_id as associated data if provided
            aad = str(source_id).encode() if source_id else None

            # Decrypt and verify
            return self.aesgcm.decrypt(nonce, ciphertext, aad)
        except Exception as e:
            raise EncryptionError(f"Decryption failed: {e}") from e


def create_archiver_from_env(tenant_id: UUID) -> FileArchiver:
    """Create FileArchiver using environment variable for master key.

    Args:
        tenant_id: Tenant ID for key derivation

    Returns:
        Configured FileArchiver

    Raises:
        KeyDerivationError: If PENF_ARCHIVE_MASTER_KEY not set or invalid
    """
    master_key_str = os.environ.get("PENF_ARCHIVE_MASTER_KEY")
    if not master_key_str:
        raise KeyDerivationError(
            "PENF_ARCHIVE_MASTER_KEY environment variable not set"
        )

    # Support both raw bytes and base64 encoded keys
    try:
        if len(master_key_str) == 32:
            master_key = master_key_str.encode()
        elif len(master_key_str) == 44:  # Base64 encoded 32 bytes
            import base64

            master_key = base64.b64decode(master_key_str)
        else:
            master_key = master_key_str.encode()

        return FileArchiver(master_key, tenant_id)
    except Exception as e:
        raise KeyDerivationError(f"Invalid master key: {e}") from e
