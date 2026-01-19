"""Tests for credential encryption with dynamic salt.

Tests for _get_salt_path(), _load_or_create_salt(), and CredentialEncryption
class with dynamic per-installation salt support and PENF_MASTER_KEY validation.
"""

import logging
import os
from pathlib import Path

import pytest
from unittest.mock import patch

from penf_lib.storage.encryption import (
    CredentialEncryption,
    _get_salt_path,
    _load_or_create_salt,
)


class TestMasterKeyValidation:
    """Tests for PENF_MASTER_KEY validation behavior."""

    def test_raises_error_in_production_with_default_key(self, tmp_path):
        """Production environment raises ValueError with default key"""
        salt_file = tmp_path / 'salt'

        with patch.dict(os.environ, {
            'PENF_SALT_PATH': str(salt_file),
            'PENFOLD_ENV': 'production',
        }):
            # Remove PENF_MASTER_KEY to use default
            os.environ.pop('PENF_MASTER_KEY', None)

            with pytest.raises(ValueError) as exc_info:
                CredentialEncryption()

            assert 'PENF_MASTER_KEY is required in production' in str(exc_info.value)

    def test_succeeds_in_production_with_explicit_key(self, tmp_path):
        """Production environment works with explicit key"""
        salt_file = tmp_path / 'salt'

        with patch.dict(os.environ, {
            'PENF_SALT_PATH': str(salt_file),
            'PENFOLD_ENV': 'production',
            'PENF_MASTER_KEY': 'secure-production-key-here',
        }):
            # Should not raise
            enc = CredentialEncryption()
            assert enc.password == 'secure-production-key-here'

    def test_succeeds_in_production_with_password_argument(self, tmp_path):
        """Production environment works with password constructor argument"""
        salt_file = tmp_path / 'salt'

        with patch.dict(os.environ, {
            'PENF_SALT_PATH': str(salt_file),
            'PENFOLD_ENV': 'production',
        }):
            os.environ.pop('PENF_MASTER_KEY', None)

            # Should not raise when password is provided directly
            enc = CredentialEncryption(password='explicit-secure-key')
            assert enc.password == 'explicit-secure-key'

    def test_logs_warning_in_development_with_default_key(self, tmp_path, caplog):
        """Development environment logs warning with default key"""
        salt_file = tmp_path / 'salt'

        with patch.dict(os.environ, {
            'PENF_SALT_PATH': str(salt_file),
            'PENFOLD_ENV': 'development',
        }):
            os.environ.pop('PENF_MASTER_KEY', None)

            with caplog.at_level(logging.WARNING):
                enc = CredentialEncryption()

            assert 'Using default development encryption key' in caplog.text
            assert enc.password == 'default-dev-key'

    def test_logs_warning_in_testing_env_with_default_key(self, tmp_path, caplog):
        """Testing environment also logs warning with default key"""
        salt_file = tmp_path / 'salt'

        with patch.dict(os.environ, {
            'PENF_SALT_PATH': str(salt_file),
            'PENFOLD_ENV': 'testing',
        }):
            os.environ.pop('PENF_MASTER_KEY', None)

            with caplog.at_level(logging.WARNING):
                enc = CredentialEncryption()

            assert 'Using default development encryption key' in caplog.text

    def test_no_warning_with_explicit_key(self, tmp_path, caplog):
        """No warning logged when explicit key is provided"""
        salt_file = tmp_path / 'salt'

        with patch.dict(os.environ, {
            'PENF_SALT_PATH': str(salt_file),
            'PENFOLD_ENV': 'development',
            'PENF_MASTER_KEY': 'my-custom-key',
        }):
            with caplog.at_level(logging.WARNING):
                enc = CredentialEncryption()

            assert 'Using default development encryption key' not in caplog.text
            assert enc.password == 'my-custom-key'

    def test_no_warning_with_password_argument(self, tmp_path, caplog):
        """No warning logged when password is passed to constructor"""
        salt_file = tmp_path / 'salt'

        with patch.dict(os.environ, {
            'PENF_SALT_PATH': str(salt_file),
            'PENFOLD_ENV': 'development',
        }):
            os.environ.pop('PENF_MASTER_KEY', None)

            with caplog.at_level(logging.WARNING):
                enc = CredentialEncryption(password='constructor-key')

            assert 'Using default development encryption key' not in caplog.text
            assert enc.password == 'constructor-key'

    def test_env_key_takes_precedence_over_default(self, tmp_path):
        """PENF_MASTER_KEY env var overrides default"""
        salt_file = tmp_path / 'salt'

        with patch.dict(os.environ, {
            'PENF_SALT_PATH': str(salt_file),
            'PENFOLD_ENV': 'development',
            'PENF_MASTER_KEY': 'env-key',
        }):
            enc = CredentialEncryption()
            assert enc.password == 'env-key'

    def test_password_argument_takes_precedence_over_env(self, tmp_path):
        """Constructor password argument takes precedence over env var"""
        salt_file = tmp_path / 'salt'

        with patch.dict(os.environ, {
            'PENF_SALT_PATH': str(salt_file),
            'PENFOLD_ENV': 'development',
            'PENF_MASTER_KEY': 'env-key',
        }):
            enc = CredentialEncryption(password='constructor-key')
            assert enc.password == 'constructor-key'


# =============================================================================
# SALT PATH TESTS
# =============================================================================


class TestSaltPath:
    """Tests for salt path configuration."""

    def test_default_salt_path(self):
        """Default salt path is ~/.penfold/encryption_salt."""
        with patch.dict(os.environ, {}, clear=True):
            # Remove PENF_SALT_PATH if set
            os.environ.pop('PENF_SALT_PATH', None)
            path = _get_salt_path()
            expected = Path.home() / '.penfold' / 'encryption_salt'
            assert path == expected

    def test_custom_salt_path_from_env(self, tmp_path):
        """PENF_SALT_PATH env var overrides default."""
        custom_path = tmp_path / 'custom_salt'
        with patch.dict(os.environ, {'PENF_SALT_PATH': str(custom_path)}):
            path = _get_salt_path()
            assert path == custom_path


# =============================================================================
# SALT GENERATION TESTS
# =============================================================================


class TestSaltGeneration:
    """Tests for salt generation and persistence."""

    def test_salt_generated_on_first_use(self, tmp_path):
        """Salt file created when it doesn't exist."""
        salt_file = tmp_path / 'test_salt'
        with patch.dict(os.environ, {'PENF_SALT_PATH': str(salt_file)}):
            assert not salt_file.exists()
            salt = _load_or_create_salt()
            assert salt_file.exists()
            assert len(salt) == 16  # 16 bytes
            assert isinstance(salt, bytes)

    def test_salt_persists_across_calls(self, tmp_path):
        """Same salt loaded on subsequent uses."""
        salt_file = tmp_path / 'test_salt'
        with patch.dict(os.environ, {'PENF_SALT_PATH': str(salt_file)}):
            salt1 = _load_or_create_salt()
            salt2 = _load_or_create_salt()
            assert salt1 == salt2

    def test_different_installations_have_different_salts(self, tmp_path):
        """Two installations generate different salts."""
        salt_file1 = tmp_path / 'salt1'
        salt_file2 = tmp_path / 'salt2'

        with patch.dict(os.environ, {'PENF_SALT_PATH': str(salt_file1)}):
            salt1 = _load_or_create_salt()

        with patch.dict(os.environ, {'PENF_SALT_PATH': str(salt_file2)}):
            salt2 = _load_or_create_salt()

        # Salts should be different (extremely unlikely to collide)
        assert salt1 != salt2

    def test_creates_parent_directories(self, tmp_path):
        """Salt function creates parent directories if needed."""
        deep_path = tmp_path / 'nested' / 'dirs' / 'salt'
        with patch.dict(os.environ, {'PENF_SALT_PATH': str(deep_path)}):
            _load_or_create_salt()
            assert deep_path.exists()


# =============================================================================
# CREDENTIAL ENCRYPTION TESTS
# =============================================================================


class TestCredentialEncryption:
    """Tests for encryption/decryption with dynamic salt."""

    @pytest.fixture
    def encryption_with_tmp_salt(self, tmp_path):
        """Create encryption instance with temporary salt path."""
        salt_file = tmp_path / 'test_salt'
        with patch.dict(os.environ, {
            'PENF_SALT_PATH': str(salt_file),
            'PENF_MASTER_KEY': 'test-key-for-testing',
            'PENFOLD_ENV': 'testing'
        }):
            yield CredentialEncryption()

    @pytest.mark.asyncio
    async def test_encrypt_decrypt_roundtrip(self, encryption_with_tmp_salt):
        """Data encrypted can be decrypted correctly."""
        original_data = {'username': 'test', 'password': 'secret123'}

        encrypted = await encryption_with_tmp_salt.encrypt(original_data)
        assert encrypted != original_data
        assert isinstance(encrypted, bytes)

        decrypted = await encryption_with_tmp_salt.decrypt(encrypted)
        assert decrypted == original_data

    @pytest.mark.asyncio
    async def test_encryption_uses_dynamic_salt(self, tmp_path):
        """Encryption actually uses the per-installation salt."""
        salt_file1 = tmp_path / 'salt1'
        salt_file2 = tmp_path / 'salt2'

        data = {'key': 'value'}

        # Encrypt with first salt
        with patch.dict(os.environ, {
            'PENF_SALT_PATH': str(salt_file1),
            'PENF_MASTER_KEY': 'same-key',
            'PENFOLD_ENV': 'testing'
        }):
            enc1 = CredentialEncryption()
            encrypted1 = await enc1.encrypt(data)

        # Encrypt with second salt (different installation)
        with patch.dict(os.environ, {
            'PENF_SALT_PATH': str(salt_file2),
            'PENF_MASTER_KEY': 'same-key',
            'PENFOLD_ENV': 'testing'
        }):
            enc2 = CredentialEncryption()
            encrypted2 = await enc2.encrypt(data)

        # Same data + same password but different salts = different ciphertext
        assert encrypted1 != encrypted2

    @pytest.mark.asyncio
    async def test_same_salt_produces_consistent_encryption(self, tmp_path):
        """Same salt allows decryption across instances."""
        salt_file = tmp_path / 'shared_salt'
        data = {'secret': 'data'}

        # Encrypt with first instance
        with patch.dict(os.environ, {
            'PENF_SALT_PATH': str(salt_file),
            'PENF_MASTER_KEY': 'shared-key',
            'PENFOLD_ENV': 'testing'
        }):
            enc1 = CredentialEncryption()
            encrypted = await enc1.encrypt(data)

        # Decrypt with second instance (same salt)
        with patch.dict(os.environ, {
            'PENF_SALT_PATH': str(salt_file),
            'PENF_MASTER_KEY': 'shared-key',
            'PENFOLD_ENV': 'testing'
        }):
            enc2 = CredentialEncryption()
            decrypted = await enc2.decrypt(encrypted)

        assert decrypted == data

    def test_generate_key_returns_valid_fernet_key(self):
        """generate_key() returns a valid base64-encoded Fernet key."""
        key = CredentialEncryption.generate_key()
        assert isinstance(key, str)
        # Fernet keys are 44 characters (32 bytes base64 encoded)
        assert len(key) == 44
