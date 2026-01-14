"""
File Validation and Metadata Extraction

Validates uploaded meeting files for format support, security, and metadata extraction.
Supports audio, video, and document files as specified in the requirements.
"""
import magic
from pathlib import Path
from typing import Dict, Any, List, Optional, Tuple
import mimetypes
from datetime import datetime
import hashlib
import aiofiles
import asyncio
import subprocess
import json
import tempfile
import os

from app.config import settings


class FileValidator:
    """Validates uploaded files and extracts metadata"""

    # Supported file formats from specification
    SUPPORTED_AUDIO_FORMATS = {
        "audio/mpeg": [".mp3"],
        "audio/wav": [".wav"],
        "audio/x-wav": [".wav"],
        "audio/wave": [".wav"],
        "audio/mp4": [".m4a"],
        "audio/aac": [".aac"],
        "audio/ogg": [".ogg"],
        "audio/flac": [".flac"]
    }

    SUPPORTED_VIDEO_FORMATS = {
        "video/mp4": [".mp4"],
        "video/quicktime": [".mov"],
        "video/x-msvideo": [".avi"],
        "video/webm": [".webm"],
        "video/x-ms-wmv": [".wmv"]
    }

    SUPPORTED_DOCUMENT_FORMATS = {
        "application/pdf": [".pdf"],
        "application/vnd.openxmlformats-officedocument.wordprocessingml.document": [".docx"],
        "application/msword": [".doc"],
        "text/plain": [".txt"]
    }

    # Maximum file sizes per type (in bytes)
    MAX_AUDIO_SIZE = settings.MAX_UPLOAD_SIZE  # 2GB
    MAX_VIDEO_SIZE = settings.MAX_UPLOAD_SIZE  # 2GB
    MAX_DOCUMENT_SIZE = 100 * 1024 * 1024     # 100MB

    def __init__(self):
        self.all_supported_formats = {
            **self.SUPPORTED_AUDIO_FORMATS,
            **self.SUPPORTED_VIDEO_FORMATS,
            **self.SUPPORTED_DOCUMENT_FORMATS
        }

    def is_supported_file_type(self, filename: str) -> bool:
        """Check if file type is supported based on extension"""
        file_path = Path(filename)
        extension = file_path.suffix.lower()

        for mime_type, extensions in self.all_supported_formats.items():
            if extension in extensions:
                return True

        return False

    async def validate_file_content(self, file_path: Path) -> Dict[str, Any]:
        """Validate file content and extract basic metadata"""

        if not file_path.exists():
            raise FileNotFoundError(f"File not found: {file_path}")

        # Get file stats
        file_stats = file_path.stat()
        file_size = file_stats.st_size

        # Detect MIME type
        mime_type = await self._detect_mime_type(file_path)

        # Validate MIME type
        if mime_type not in self.all_supported_formats:
            raise ValueError(f"Unsupported file type: {mime_type}")

        # Validate file size based on type
        await self._validate_file_size(file_size, mime_type)

        # Extract detailed metadata based on file type
        metadata = await self._extract_metadata(file_path, mime_type)

        # Calculate file checksum
        checksum = await self._calculate_checksum(file_path)

        return {
            "filename": file_path.name,
            "file_size": file_size,
            "mime_type": mime_type,
            "checksum": checksum,
            "last_modified": datetime.fromtimestamp(file_stats.st_mtime).isoformat(),
            "validation_status": "valid",
            "metadata": metadata
        }

    async def _detect_mime_type(self, file_path: Path) -> str:
        """Detect MIME type using python-magic"""
        try:
            # Use python-magic for accurate MIME type detection
            mime_type = magic.from_file(str(file_path), mime=True)
            return mime_type
        except Exception:
            # Fallback to mimetypes module
            mime_type, _ = mimetypes.guess_type(str(file_path))
            return mime_type or "application/octet-stream"

    async def _validate_file_size(self, file_size: int, mime_type: str):
        """Validate file size based on type"""
        if mime_type in self.SUPPORTED_AUDIO_FORMATS:
            max_size = self.MAX_AUDIO_SIZE
            file_type = "audio"
        elif mime_type in self.SUPPORTED_VIDEO_FORMATS:
            max_size = self.MAX_VIDEO_SIZE
            file_type = "video"
        elif mime_type in self.SUPPORTED_DOCUMENT_FORMATS:
            max_size = self.MAX_DOCUMENT_SIZE
            file_type = "document"
        else:
            raise ValueError(f"Unknown file type: {mime_type}")

        if file_size > max_size:
            raise ValueError(
                f"{file_type.title()} file size {file_size} exceeds maximum {max_size} bytes"
            )

    async def _extract_metadata(self, file_path: Path, mime_type: str) -> Dict[str, Any]:
        """Extract metadata based on file type"""

        if mime_type in self.SUPPORTED_AUDIO_FORMATS or mime_type in self.SUPPORTED_VIDEO_FORMATS:
            return await self._extract_media_metadata(file_path)
        elif mime_type in self.SUPPORTED_DOCUMENT_FORMATS:
            return await self._extract_document_metadata(file_path, mime_type)
        else:
            return {}

    async def _extract_media_metadata(self, file_path: Path) -> Dict[str, Any]:
        """Extract metadata from audio/video files using ffprobe"""
        try:
            cmd = [
                "ffprobe",
                "-v", "quiet",
                "-print_format", "json",
                "-show_format",
                "-show_streams",
                str(file_path)
            ]

            process = await asyncio.create_subprocess_exec(
                *cmd,
                stdout=asyncio.subprocess.PIPE,
                stderr=asyncio.subprocess.PIPE
            )

            stdout, stderr = await process.communicate()

            if process.returncode != 0:
                return {"error": f"ffprobe failed: {stderr.decode()}"}

            metadata = json.loads(stdout.decode())

            # Extract relevant information
            format_info = metadata.get("format", {})
            streams = metadata.get("streams", [])

            # Find audio stream
            audio_stream = next((s for s in streams if s.get("codec_type") == "audio"), None)
            video_stream = next((s for s in streams if s.get("codec_type") == "video"), None)

            result = {
                "duration_seconds": float(format_info.get("duration", 0)),
                "format_name": format_info.get("format_name"),
                "bitrate": int(format_info.get("bit_rate", 0)),
                "size_bytes": int(format_info.get("size", 0)),
            }

            if audio_stream:
                result.update({
                    "audio_codec": audio_stream.get("codec_name"),
                    "sample_rate": int(audio_stream.get("sample_rate", 0)),
                    "channels": int(audio_stream.get("channels", 0)),
                    "channel_layout": audio_stream.get("channel_layout")
                })

            if video_stream:
                result.update({
                    "video_codec": video_stream.get("codec_name"),
                    "width": int(video_stream.get("width", 0)),
                    "height": int(video_stream.get("height", 0)),
                    "frame_rate": video_stream.get("r_frame_rate")
                })

            # Extract tags if available
            tags = format_info.get("tags", {})
            if tags:
                result["tags"] = {k.lower(): v for k, v in tags.items()}

            return result

        except FileNotFoundError:
            return {"error": "ffprobe not found. Install ffmpeg for media metadata extraction."}
        except Exception as e:
            return {"error": f"Metadata extraction failed: {str(e)}"}

    async def _extract_document_metadata(self, file_path: Path, mime_type: str) -> Dict[str, Any]:
        """Extract metadata from document files"""
        try:
            metadata = {
                "file_type": "document",
                "mime_type": mime_type
            }

            if mime_type == "application/pdf":
                metadata.update(await self._extract_pdf_metadata(file_path))
            elif mime_type in [
                "application/vnd.openxmlformats-officedocument.wordprocessingml.document",
                "application/msword"
            ]:
                metadata.update(await self._extract_word_metadata(file_path))

            return metadata

        except Exception as e:
            return {"error": f"Document metadata extraction failed: {str(e)}"}

    async def _extract_pdf_metadata(self, file_path: Path) -> Dict[str, Any]:
        """Extract PDF metadata using PyPDF2 (will be implemented later)"""
        # Placeholder - would use PyPDF2 or similar library
        return {
            "document_type": "pdf",
            "estimated_pages": "unknown"
        }

    async def _extract_word_metadata(self, file_path: Path) -> Dict[str, Any]:
        """Extract Word document metadata (will be implemented later)"""
        # Placeholder - would use python-docx or similar library
        return {
            "document_type": "word",
            "estimated_pages": "unknown"
        }

    async def _calculate_checksum(self, file_path: Path) -> str:
        """Calculate SHA-256 checksum of file"""
        hash_sha256 = hashlib.sha256()

        async with aiofiles.open(file_path, "rb") as f:
            while True:
                chunk = await f.read(8192)
                if not chunk:
                    break
                hash_sha256.update(chunk)

        return hash_sha256.hexdigest()

    def get_estimated_processing_time(self, file_size: int, mime_type: str) -> int:
        """Estimate processing time in seconds based on file type and size"""

        if mime_type in self.SUPPORTED_AUDIO_FORMATS:
            # Audio: ~2x real-time for transcription
            estimated_duration = file_size / (128 * 1024 / 8)  # Assume 128kbps average
            return int(estimated_duration * 2)

        elif mime_type in self.SUPPORTED_VIDEO_FORMATS:
            # Video: ~3x real-time (extraction + transcription)
            estimated_duration = file_size / (1024 * 1024)  # Rough estimate
            return int(estimated_duration * 3)

        elif mime_type in self.SUPPORTED_DOCUMENT_FORMATS:
            # Documents: Quick processing
            return min(60, file_size // (1024 * 1024))  # 1 minute per MB, max 60s

        return 300  # Default 5 minutes

    def get_supported_formats(self) -> Dict[str, List[str]]:
        """Get list of supported formats organized by type"""
        return {
            "audio": list(self.SUPPORTED_AUDIO_FORMATS.keys()),
            "video": list(self.SUPPORTED_VIDEO_FORMATS.keys()),
            "document": list(self.SUPPORTED_DOCUMENT_FORMATS.keys())
        }

    async def validate_upload_request(
        self,
        filename: str,
        file_size: int,
        privacy_level: str,
        metadata: Optional[Dict[str, Any]] = None
    ) -> Dict[str, Any]:
        """Validate upload request parameters"""

        errors = []

        # Validate filename
        if not filename or len(filename.strip()) == 0:
            errors.append("Filename cannot be empty")

        if not self.is_supported_file_type(filename):
            errors.append(f"Unsupported file type: {Path(filename).suffix}")

        # Validate file size
        if file_size <= 0:
            errors.append("File size must be greater than 0")

        if file_size > settings.MAX_UPLOAD_SIZE:
            errors.append(f"File size {file_size} exceeds maximum {settings.MAX_UPLOAD_SIZE}")

        # Validate privacy level
        valid_privacy_levels = ["public", "organization", "team_only", "confidential"]
        if privacy_level not in valid_privacy_levels:
            errors.append(f"Invalid privacy level. Must be one of: {valid_privacy_levels}")

        # Validate metadata if provided
        if metadata:
            if not isinstance(metadata, dict):
                errors.append("Metadata must be a JSON object")

        if errors:
            return {"valid": False, "errors": errors}

        # Calculate estimated processing time
        mime_type, _ = mimetypes.guess_type(filename)
        if mime_type:
            estimated_processing_time = self.get_estimated_processing_time(file_size, mime_type)
        else:
            estimated_processing_time = 300

        return {
            "valid": True,
            "estimated_processing_time": estimated_processing_time,
            "detected_file_type": mime_type or "unknown"
        }