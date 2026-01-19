"""
File Storage Abstraction with Privacy Level Controls

Manages file storage paths, privacy-based organization, and preparation
for encryption based on privacy levels.
"""
from pathlib import Path
from typing import Dict, Any, Optional
import uuid
import shutil
import aiofiles
import os
from datetime import datetime
import json

from app.config import settings, get_upload_path, get_encryption_key_path


class FileStorage:
    """Manages file storage with privacy level organization"""

    def __init__(self):
        self.base_upload_dir = settings.UPLOAD_DIR
        self.base_processed_dir = settings.PROCESSED_DIR
        self._ensure_base_directories()

    def _ensure_base_directories(self):
        """Ensure base storage directories exist"""
        for privacy_level in ["public", "organization", "team_only", "confidential"]:
            upload_path = get_upload_path(privacy_level)
            upload_path.mkdir(parents=True, exist_ok=True)

            processed_path = self.base_processed_dir / privacy_level
            processed_path.mkdir(parents=True, exist_ok=True)

        # Ensure temp directory for uploads
        temp_dir = self.base_upload_dir / "temp"
        temp_dir.mkdir(parents=True, exist_ok=True)

    async def get_upload_path(self, session_id: str, privacy_level: str) -> Path:
        """Get temporary upload path for session"""
        base_path = self.base_upload_dir / "temp"
        return base_path / f"{session_id}.tmp"

    async def get_final_storage_path(
        self,
        meeting_id: str,
        filename: str,
        privacy_level: str
    ) -> Path:
        """Get final storage path based on privacy level"""
        base_path = get_upload_path(privacy_level)

        # Organize by date for easier management
        date_str = datetime.now().strftime("%Y/%m/%d")
        storage_dir = base_path / date_str

        # Ensure directory exists
        storage_dir.mkdir(parents=True, exist_ok=True)

        # Use meeting ID + original filename for uniqueness
        file_extension = Path(filename).suffix
        final_filename = f"{meeting_id}_{filename}"

        return storage_dir / final_filename

    async def finalize_upload(
        self,
        session_id: str,
        meeting_id: str,
        privacy_level: str,
        metadata: Dict[str, Any]
    ) -> Path:
        """Move uploaded file to final storage location"""

        # Get paths
        temp_path = await self.get_upload_path(session_id, privacy_level)
        filename = metadata.get("filename", "unknown_file")
        final_path = await self.get_final_storage_path(meeting_id, filename, privacy_level)

        if not temp_path.exists():
            raise FileNotFoundError(f"Temporary upload file not found: {temp_path}")

        # Move file to final location
        shutil.move(str(temp_path), str(final_path))

        # Create metadata file alongside
        await self._create_metadata_file(final_path, meeting_id, metadata)

        # Apply privacy-specific processing
        if privacy_level == "confidential":
            final_path = await self._apply_encryption(final_path, privacy_level)

        return final_path

    async def _create_metadata_file(
        self,
        file_path: Path,
        meeting_id: str,
        metadata: Dict[str, Any]
    ):
        """Create metadata file alongside the uploaded file"""

        metadata_path = file_path.with_suffix(file_path.suffix + ".metadata")

        file_metadata = {
            "meeting_id": meeting_id,
            "original_filename": metadata.get("filename"),
            "upload_completed_at": datetime.utcnow().isoformat(),
            "file_size": file_path.stat().st_size,
            "privacy_level": metadata.get("privacy_level"),
            "upload_metadata": metadata,
            "storage_version": "1.0"
        }

        async with aiofiles.open(metadata_path, "w") as f:
            await f.write(json.dumps(file_metadata, indent=2))

    async def _apply_encryption(self, file_path: Path, privacy_level: str) -> Path:
        """Apply encryption for confidential files"""
        from app.upload.encryption import file_encryption

        if privacy_level == "confidential":
            # Encrypt the file
            encrypted_path = await file_encryption.encrypt_file(
                file_path, privacy_level, {"original_path": str(file_path)}
            )
            return encrypted_path
        else:
            # No encryption needed
            return file_path

    async def get_file_info(self, file_path: Path) -> Dict[str, Any]:
        """Get information about a stored file"""

        if not file_path.exists():
            raise FileNotFoundError(f"File not found: {file_path}")

        # Get basic file stats
        file_stats = file_path.stat()

        # Try to load metadata file
        metadata_path = file_path.with_suffix(file_path.suffix + ".metadata")
        metadata = {}

        if metadata_path.exists():
            async with aiofiles.open(metadata_path, "r") as f:
                content = await f.read()
                metadata = json.loads(content)

        return {
            "file_path": str(file_path),
            "file_size": file_stats.st_size,
            "created_at": datetime.fromtimestamp(file_stats.st_ctime).isoformat(),
            "modified_at": datetime.fromtimestamp(file_stats.st_mtime).isoformat(),
            "metadata": metadata
        }

    async def delete_file(self, file_path: Path) -> bool:
        """Delete a file and its metadata"""

        try:
            # Delete main file
            if file_path.exists():
                file_path.unlink()

            # Delete metadata file
            metadata_path = file_path.with_suffix(file_path.suffix + ".metadata")
            if metadata_path.exists():
                metadata_path.unlink()

            return True

        except Exception as e:
            print(f"Error deleting file {file_path}: {e}")
            return False

    async def move_to_processed(
        self,
        source_path: Path,
        meeting_id: str,
        privacy_level: str
    ) -> Path:
        """Move file to processed directory after successful processing"""

        # Create processed directory structure
        date_str = datetime.now().strftime("%Y/%m/%d")
        processed_dir = self.base_processed_dir / privacy_level / date_str
        processed_dir.mkdir(parents=True, exist_ok=True)

        # Create processed filename
        processed_filename = f"{meeting_id}_{source_path.name}"
        processed_path = processed_dir / processed_filename

        # Move file
        shutil.move(str(source_path), str(processed_path))

        # Move metadata file if it exists
        source_metadata = source_path.with_suffix(source_path.suffix + ".metadata")
        if source_metadata.exists():
            processed_metadata = processed_path.with_suffix(processed_path.suffix + ".metadata")
            shutil.move(str(source_metadata), str(processed_metadata))

        return processed_path

    async def calculate_storage_usage(self) -> Dict[str, int]:
        """Calculate storage usage by privacy level"""

        usage = {}

        for privacy_level in ["public", "organization", "team_only", "confidential"]:
            total_size = 0
            file_count = 0

            # Check upload directory
            upload_path = get_upload_path(privacy_level)
            if upload_path.exists():
                for file_path in upload_path.rglob("*"):
                    if file_path.is_file() and not file_path.name.endswith(".metadata"):
                        total_size += file_path.stat().st_size
                        file_count += 1

            # Check processed directory
            processed_path = self.base_processed_dir / privacy_level
            if processed_path.exists():
                for file_path in processed_path.rglob("*"):
                    if file_path.is_file() and not file_path.name.endswith(".metadata"):
                        total_size += file_path.stat().st_size
                        file_count += 1

            usage[privacy_level] = {
                "total_size_bytes": total_size,
                "file_count": file_count
            }

        return usage

    async def cleanup_temp_files(self, max_age_hours: int = 24):
        """Clean up temporary upload files older than specified hours"""

        temp_dir = self.base_upload_dir / "temp"
        if not temp_dir.exists():
            return

        cutoff_time = datetime.now().timestamp() - (max_age_hours * 3600)
        cleaned_files = 0

        for file_path in temp_dir.iterdir():
            if file_path.is_file():
                if file_path.stat().st_mtime < cutoff_time:
                    try:
                        file_path.unlink()
                        cleaned_files += 1
                    except Exception as e:
                        print(f"Error cleaning temp file {file_path}: {e}")

        return cleaned_files

    def get_privacy_level_path(self, privacy_level: str) -> Path:
        """Get base path for a privacy level"""
        return get_upload_path(privacy_level)

    def is_valid_privacy_level(self, privacy_level: str) -> bool:
        """Check if privacy level is valid"""
        return privacy_level in ["public", "organization", "team_only", "confidential"]

    async def get_available_space(self) -> Dict[str, int]:
        """Get available disk space for storage"""

        # Get disk usage for upload directory
        upload_usage = shutil.disk_usage(self.base_upload_dir)
        processed_usage = shutil.disk_usage(self.base_processed_dir)

        return {
            "upload_directory": {
                "total_bytes": upload_usage.total,
                "used_bytes": upload_usage.used,
                "free_bytes": upload_usage.free
            },
            "processed_directory": {
                "total_bytes": processed_usage.total,
                "used_bytes": processed_usage.used,
                "free_bytes": processed_usage.free
            }
        }

    async def create_backup(self, meeting_id: str, source_path: Path) -> Optional[Path]:
        """Create backup copy of important files"""

        if not source_path.exists():
            return None

        # Create backup directory
        backup_dir = self.base_processed_dir / "backups" / datetime.now().strftime("%Y/%m")
        backup_dir.mkdir(parents=True, exist_ok=True)

        # Create backup filename
        backup_filename = f"backup_{meeting_id}_{source_path.name}"
        backup_path = backup_dir / backup_filename

        try:
            shutil.copy2(str(source_path), str(backup_path))
            return backup_path
        except Exception as e:
            print(f"Error creating backup for {meeting_id}: {e}")
            return None