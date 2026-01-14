"""
Encryption Support for Confidential Meeting Files

Provides AES-256 encryption for confidential files with privacy-level
key management and secure file handling.
"""
from cryptography.fernet import Fernet
from cryptography.hazmat.primitives import hashes
from cryptography.hazmat.primitives.kdf.pbkdf2 import PBKDF2HMAC
from pathlib import Path
from typing import Dict, Any, Optional, Tuple
import os
import base64
import hashlib
import json
import aiofiles
from datetime import datetime

from app.config import settings, get_encryption_key_path


class FileEncryption:
    """Handles file encryption for confidential content"""

    def __init__(self):
        self.key_cache: Dict[str, bytes] = {}
        self._ensure_key_directory()

    def _ensure_key_directory(self):
        """Ensure encryption key directory exists"""
        settings.ENCRYPTION_KEY_PATH.mkdir(parents=True, exist_ok=True)

    async def get_or_create_key(self, privacy_level: str) -> bytes:
        """Get or create encryption key for privacy level"""

        if privacy_level in self.key_cache:
            return self.key_cache[privacy_level]

        key_path = get_encryption_key_path(privacy_level)

        if key_path.exists():
            # Load existing key
            async with aiofiles.open(key_path, "rb") as f:
                key_data = await f.read()
                self.key_cache[privacy_level] = key_data
                return key_data
        else:
            # Create new key
            key = Fernet.generate_key()
            async with aiofiles.open(key_path, "wb") as f:
                await f.write(key)

            # Secure the key file (readable only by owner)
            os.chmod(key_path, 0o600)

            self.key_cache[privacy_level] = key
            return key

    async def encrypt_file(
        self,
        source_path: Path,
        privacy_level: str,
        metadata: Optional[Dict[str, Any]] = None
    ) -> Path:
        """Encrypt a file with privacy-level encryption"""

        if not source_path.exists():
            raise FileNotFoundError(f"Source file not found: {source_path}")

        # Get encryption key
        encryption_key = await self.get_or_create_key(privacy_level)
        fernet = Fernet(encryption_key)

        # Create encrypted filename
        encrypted_path = source_path.with_suffix(source_path.suffix + ".encrypted")

        # Read, encrypt, and write file
        async with aiofiles.open(source_path, "rb") as source_file:
            file_data = await source_file.read()

        encrypted_data = fernet.encrypt(file_data)

        async with aiofiles.open(encrypted_path, "wb") as encrypted_file:
            await encrypted_file.write(encrypted_data)

        # Create encryption metadata
        await self._create_encryption_metadata(
            encrypted_path,
            privacy_level,
            source_path,
            metadata
        )

        # Remove original file for security
        source_path.unlink()

        return encrypted_path

    async def decrypt_file(
        self,
        encrypted_path: Path,
        output_path: Optional[Path] = None
    ) -> Path:
        """Decrypt an encrypted file"""

        if not encrypted_path.exists():
            raise FileNotFoundError(f"Encrypted file not found: {encrypted_path}")

        # Load encryption metadata
        metadata = await self._load_encryption_metadata(encrypted_path)
        privacy_level = metadata.get("privacy_level")

        if not privacy_level:
            raise ValueError("Cannot determine privacy level for decryption")

        # Get encryption key
        encryption_key = await self.get_or_create_key(privacy_level)
        fernet = Fernet(encryption_key)

        # Determine output path
        if output_path is None:
            original_suffix = metadata.get("original_suffix", "")
            output_path = encrypted_path.with_suffix(original_suffix)

        # Read, decrypt, and write file
        async with aiofiles.open(encrypted_path, "rb") as encrypted_file:
            encrypted_data = await encrypted_file.read()

        try:
            decrypted_data = fernet.decrypt(encrypted_data)
        except Exception as e:
            raise ValueError(f"Decryption failed: {str(e)}")

        async with aiofiles.open(output_path, "wb") as output_file:
            await output_file.write(decrypted_data)

        return output_path

    async def _create_encryption_metadata(
        self,
        encrypted_path: Path,
        privacy_level: str,
        original_path: Path,
        metadata: Optional[Dict[str, Any]] = None
    ):
        """Create metadata file for encrypted content"""

        metadata_path = encrypted_path.with_suffix(encrypted_path.suffix + ".encmeta")

        encryption_metadata = {
            "privacy_level": privacy_level,
            "original_filename": original_path.name,
            "original_suffix": original_path.suffix,
            "encrypted_at": datetime.utcnow().isoformat(),
            "encryption_version": "1.0",
            "algorithm": "AES-256 (Fernet)",
            "file_checksum": await self._calculate_file_hash(encrypted_path),
            "metadata": metadata or {}
        }

        async with aiofiles.open(metadata_path, "w") as f:
            await f.write(json.dumps(encryption_metadata, indent=2))

        # Secure the metadata file
        os.chmod(metadata_path, 0o600)

    async def _load_encryption_metadata(self, encrypted_path: Path) -> Dict[str, Any]:
        """Load encryption metadata for a file"""

        metadata_path = encrypted_path.with_suffix(encrypted_path.suffix + ".encmeta")

        if not metadata_path.exists():
            raise FileNotFoundError(f"Encryption metadata not found: {metadata_path}")

        async with aiofiles.open(metadata_path, "r") as f:
            content = await f.read()
            return json.loads(content)

    async def _calculate_file_hash(self, file_path: Path) -> str:
        """Calculate SHA-256 hash of file"""

        hash_sha256 = hashlib.sha256()

        async with aiofiles.open(file_path, "rb") as f:
            while True:
                chunk = await f.read(8192)
                if not chunk:
                    break
                hash_sha256.update(chunk)

        return hash_sha256.hexdigest()

    async def verify_encrypted_file(self, encrypted_path: Path) -> Dict[str, Any]:
        """Verify integrity of encrypted file"""

        if not encrypted_path.exists():
            return {"valid": False, "error": "File not found"}

        try:
            # Load metadata
            metadata = await self._load_encryption_metadata(encrypted_path)

            # Verify checksum
            current_hash = await self._calculate_file_hash(encrypted_path)
            stored_hash = metadata.get("file_checksum")

            if current_hash != stored_hash:
                return {
                    "valid": False,
                    "error": "Checksum mismatch - file may be corrupted"
                }

            # Try to decrypt first few bytes to verify key works
            privacy_level = metadata.get("privacy_level")
            encryption_key = await self.get_or_create_key(privacy_level)
            fernet = Fernet(encryption_key)

            async with aiofiles.open(encrypted_path, "rb") as f:
                # Read small portion for verification
                small_chunk = await f.read(1024)

            try:
                fernet.decrypt(small_chunk)
                decryption_test = True
            except:
                decryption_test = False

            return {
                "valid": True,
                "checksum_valid": True,
                "decryption_test": decryption_test,
                "metadata": metadata
            }

        except Exception as e:
            return {"valid": False, "error": str(e)}

    async def rotate_encryption_key(self, privacy_level: str) -> bool:
        """Rotate encryption key for a privacy level"""

        try:
            # Get old key
            old_key = await self.get_or_create_key(privacy_level)

            # Generate new key
            new_key = Fernet.generate_key()

            # Save old key with timestamp
            old_key_path = get_encryption_key_path(f"{privacy_level}_old_{int(datetime.utcnow().timestamp())}")
            async with aiofiles.open(old_key_path, "wb") as f:
                await f.write(old_key)

            # Save new key
            key_path = get_encryption_key_path(privacy_level)
            async with aiofiles.open(key_path, "wb") as f:
                await f.write(new_key)

            # Update cache
            self.key_cache[privacy_level] = new_key

            return True

        except Exception as e:
            print(f"Error rotating key for {privacy_level}: {e}")
            return False

    async def get_encryption_status(self) -> Dict[str, Any]:
        """Get encryption system status"""

        status = {
            "key_directory": str(settings.ENCRYPTION_KEY_PATH),
            "keys_loaded": len(self.key_cache),
            "privacy_levels": {}
        }

        for privacy_level in ["public", "organization", "team_only", "confidential"]:
            key_path = get_encryption_key_path(privacy_level)
            status["privacy_levels"][privacy_level] = {
                "key_exists": key_path.exists(),
                "key_cached": privacy_level in self.key_cache
            }

        return status

    def requires_encryption(self, privacy_level: str) -> bool:
        """Check if privacy level requires encryption"""
        return privacy_level == "confidential"

    async def cleanup_old_keys(self, max_age_days: int = 30):
        """Clean up old encryption keys"""

        if not settings.ENCRYPTION_KEY_PATH.exists():
            return 0

        cutoff_time = datetime.utcnow().timestamp() - (max_age_days * 24 * 3600)
        cleaned_keys = 0

        for key_file in settings.ENCRYPTION_KEY_PATH.iterdir():
            if key_file.is_file() and "_old_" in key_file.name:
                if key_file.stat().st_mtime < cutoff_time:
                    try:
                        key_file.unlink()
                        cleaned_keys += 1
                    except Exception as e:
                        print(f"Error cleaning old key {key_file}: {e}")

        return cleaned_keys


class StreamEncryption:
    """Handle streaming encryption for large files"""

    def __init__(self, privacy_level: str):
        self.privacy_level = privacy_level
        self.file_encryption = FileEncryption()
        self.fernet = None

    async def initialize(self):
        """Initialize encryption for streaming"""
        encryption_key = await self.file_encryption.get_or_create_key(self.privacy_level)
        self.fernet = Fernet(encryption_key)

    async def encrypt_chunk(self, chunk: bytes) -> bytes:
        """Encrypt a data chunk"""
        if not self.fernet:
            await self.initialize()

        return self.fernet.encrypt(chunk)

    async def decrypt_chunk(self, encrypted_chunk: bytes) -> bytes:
        """Decrypt a data chunk"""
        if not self.fernet:
            await self.initialize()

        return self.fernet.decrypt(encrypted_chunk)


# Global encryption instance
file_encryption = FileEncryption()