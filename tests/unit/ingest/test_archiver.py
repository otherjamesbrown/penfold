"""Unit tests for file archiver.

Tests AES-256-GCM encrypted file archival per US6 acceptance criteria:
- Original .eml files stored encrypted
- Compliance retrieval returns original content
- Tenant-scoped encryption keys

Reference: specs/012-manual-ingest/research.md Section 2
"""

import os
from uuid import UUID

import pytest

from penf_lib.ingest.archiver import (
    ArchiveResult,
    EncryptionError,
    FileArchiver,
    KeyDerivationError,
    create_archiver_from_env,
)


class TestFileArchiver:
    """Tests for FileArchiver encryption/decryption."""

    @pytest.fixture
    def master_key(self) -> bytes:
        """32-byte master key for testing."""
        return b"0123456789abcdef0123456789abcdef"

    @pytest.fixture
    def tenant_id(self) -> UUID:
        """Test tenant ID."""
        return UUID("00000000-0000-0000-0000-000000000001")

    @pytest.fixture
    def archiver(self, master_key: bytes, tenant_id: UUID) -> FileArchiver:
        """Create archiver instance."""
        return FileArchiver(master_key, tenant_id)

    def test_encrypt_decrypt_roundtrip(self, archiver: FileArchiver):
        """Encrypted content can be decrypted back to original."""
        original = b"This is the original .eml file content"

        result = archiver.encrypt(original)
        decrypted = archiver.decrypt(result.encrypted_content)

        assert decrypted == original

    def test_encrypt_with_source_id_roundtrip(self, archiver: FileArchiver):
        """Encryption with source_id as AAD works correctly."""
        original = b"Email content with AAD"
        source_id = 12345

        result = archiver.encrypt(original, source_id=source_id)
        decrypted = archiver.decrypt(result.encrypted_content, source_id=source_id)

        assert decrypted == original

    def test_archive_result_fields(self, archiver: FileArchiver):
        """ArchiveResult has all required fields."""
        original = b"Test content for archival"

        result = archiver.encrypt(original)

        assert isinstance(result, ArchiveResult)
        assert len(result.encrypted_content) > len(original)  # Ciphertext + nonce + tag
        assert result.original_size == len(original)
        assert result.content_hash is not None
        assert result.encryption_key_id.startswith("tenant-")

    def test_content_hash_is_sha256(self, archiver: FileArchiver):
        """Content hash is SHA-256 of original content."""
        import hashlib

        original = b"Content for hash verification"
        expected_hash = hashlib.sha256(original).hexdigest()

        result = archiver.encrypt(original)

        assert result.content_hash == expected_hash

    def test_different_tenants_different_keys(self, master_key: bytes):
        """Different tenants derive different keys."""
        tenant1 = UUID("00000000-0000-0000-0000-000000000001")
        tenant2 = UUID("00000000-0000-0000-0000-000000000002")

        archiver1 = FileArchiver(master_key, tenant1)
        archiver2 = FileArchiver(master_key, tenant2)

        assert archiver1.tenant_key != archiver2.tenant_key

    def test_decrypt_with_wrong_source_id_fails(self, archiver: FileArchiver):
        """Decryption with wrong source_id fails (AAD mismatch)."""
        original = b"Content encrypted with AAD"
        correct_id = 12345
        wrong_id = 54321

        result = archiver.encrypt(original, source_id=correct_id)

        with pytest.raises(EncryptionError):
            archiver.decrypt(result.encrypted_content, source_id=wrong_id)

    def test_decrypt_without_source_id_when_encrypted_with_it(
        self, archiver: FileArchiver
    ):
        """Decryption without source_id fails if encrypted with it."""
        original = b"Content encrypted with AAD"
        source_id = 12345

        result = archiver.encrypt(original, source_id=source_id)

        with pytest.raises(EncryptionError):
            archiver.decrypt(result.encrypted_content)  # No source_id

    def test_empty_content_raises_error(self, archiver: FileArchiver):
        """Cannot encrypt empty content."""
        with pytest.raises(EncryptionError, match="Cannot encrypt empty content"):
            archiver.encrypt(b"")

    def test_invalid_encrypted_content_raises_error(self, archiver: FileArchiver):
        """Decryption of invalid content fails."""
        with pytest.raises(EncryptionError, match="too short"):
            archiver.decrypt(b"short")


class TestKeyDerivation:
    """Tests for key derivation."""

    def test_invalid_master_key_length(self):
        """Master key must be exactly 32 bytes."""
        short_key = b"tooshort"
        tenant_id = UUID("00000000-0000-0000-0000-000000000001")

        with pytest.raises(KeyDerivationError, match="32 bytes"):
            FileArchiver(short_key, tenant_id)

    def test_long_master_key_fails(self):
        """Master key longer than 32 bytes fails."""
        long_key = b"0" * 64
        tenant_id = UUID("00000000-0000-0000-0000-000000000001")

        with pytest.raises(KeyDerivationError, match="32 bytes"):
            FileArchiver(long_key, tenant_id)


class TestCreateArchiverFromEnv:
    """Tests for environment-based archiver creation."""

    def test_missing_env_var_raises_error(self, monkeypatch):
        """Missing PENF_ARCHIVE_MASTER_KEY raises error."""
        monkeypatch.delenv("PENF_ARCHIVE_MASTER_KEY", raising=False)

        with pytest.raises(KeyDerivationError, match="not set"):
            create_archiver_from_env(UUID("00000000-0000-0000-0000-000000000001"))

    def test_create_from_raw_key(self, monkeypatch):
        """Can create archiver from raw 32-character key."""
        monkeypatch.setenv("PENF_ARCHIVE_MASTER_KEY", "0123456789abcdef0123456789abcdef")
        tenant_id = UUID("00000000-0000-0000-0000-000000000001")

        archiver = create_archiver_from_env(tenant_id)

        assert archiver is not None
        assert archiver.tenant_id == tenant_id

    def test_create_from_base64_key(self, monkeypatch):
        """Can create archiver from base64-encoded key."""
        import base64

        raw_key = b"0123456789abcdef0123456789abcdef"
        b64_key = base64.b64encode(raw_key).decode()
        monkeypatch.setenv("PENF_ARCHIVE_MASTER_KEY", b64_key)
        tenant_id = UUID("00000000-0000-0000-0000-000000000001")

        archiver = create_archiver_from_env(tenant_id)

        assert archiver is not None


class TestArchiverNonceUniqueness:
    """Tests for nonce uniqueness (security property)."""

    def test_encryptions_produce_different_ciphertext(self):
        """Same plaintext encrypted twice produces different ciphertext."""
        master_key = b"0123456789abcdef0123456789abcdef"
        tenant_id = UUID("00000000-0000-0000-0000-000000000001")
        archiver = FileArchiver(master_key, tenant_id)

        plaintext = b"Same content encrypted twice"

        result1 = archiver.encrypt(plaintext)
        result2 = archiver.encrypt(plaintext)

        # Nonces are random, so ciphertext should differ
        assert result1.encrypted_content != result2.encrypted_content

        # But both should decrypt to the same plaintext
        assert archiver.decrypt(result1.encrypted_content) == plaintext
        assert archiver.decrypt(result2.encrypted_content) == plaintext
