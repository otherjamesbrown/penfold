"""
TUS Protocol Handlers for Resumable File Uploads

Implements the TUS resumable upload specification for handling large meeting files
with support for resuming interrupted uploads and progress tracking.
"""
from fastapi import HTTPException, Request, Response
from fastapi.responses import JSONResponse
from typing import Dict, Any, Optional, Tuple
import os
import uuid
import aiofiles
import hashlib
from datetime import datetime, timedelta
from pathlib import Path
import asyncio

from app.config import settings
from app.upload.storage import FileStorage
from app.upload.validation import FileValidator
from app.upload.progress import ProgressTracker


class TUSHandler:
    """TUS protocol implementation for resumable uploads"""

    def __init__(self):
        self.storage = FileStorage()
        self.validator = FileValidator()
        self.progress = ProgressTracker()
        self.upload_sessions: Dict[str, Dict[str, Any]] = {}

    async def create_upload_session(
        self,
        filename: str,
        file_size: int,
        privacy_level: str,
        metadata: Optional[Dict[str, Any]] = None
    ) -> Dict[str, Any]:
        """Create new TUS upload session"""

        # Validate request
        if file_size > settings.MAX_UPLOAD_SIZE:
            raise HTTPException(
                status_code=413,
                detail=f"File size {file_size} exceeds maximum {settings.MAX_UPLOAD_SIZE} bytes"
            )

        # Validate file type
        if not self.validator.is_supported_file_type(filename):
            raise HTTPException(
                status_code=400,
                detail=f"File type not supported: {filename}"
            )

        # Generate session IDs
        session_id = str(uuid.uuid4())
        meeting_id = str(uuid.uuid4())

        # Create upload session
        session_data = {
            "session_id": session_id,
            "meeting_id": meeting_id,
            "filename": filename,
            "file_size": file_size,
            "privacy_level": privacy_level,
            "metadata": metadata or {},
            "created_at": datetime.utcnow(),
            "expires_at": datetime.utcnow() + timedelta(hours=24),
            "uploaded_bytes": 0,
            "status": "created",
            "chunks": {}
        }

        # Store session
        self.upload_sessions[session_id] = session_data

        # Create upload directory
        upload_path = await self.storage.get_upload_path(session_id, privacy_level)
        upload_path.parent.mkdir(parents=True, exist_ok=True)

        return {
            "upload_session_id": session_id,
            "upload_url": f"/api/v1/uploads/{session_id}",
            "meeting_id": meeting_id,
            "expires_at": session_data["expires_at"].isoformat() + "Z",
            "chunk_size": settings.CHUNK_SIZE,
            "max_file_size": settings.MAX_UPLOAD_SIZE
        }

    async def handle_patch_upload(
        self,
        session_id: str,
        request: Request
    ) -> Response:
        """Handle TUS PATCH request for chunk upload"""

        # Get session
        session = self.upload_sessions.get(session_id)
        if not session:
            raise HTTPException(status_code=404, detail="Upload session not found")

        # Check if session expired
        if datetime.utcnow() > session["expires_at"]:
            raise HTTPException(status_code=410, detail="Upload session expired")

        # Get upload offset from headers
        upload_offset = int(request.headers.get("upload-offset", "0"))
        if upload_offset != session["uploaded_bytes"]:
            raise HTTPException(
                status_code=409,
                detail=f"Upload offset mismatch. Expected {session['uploaded_bytes']}, got {upload_offset}"
            )

        # Get content type and length
        content_type = request.headers.get("content-type", "application/offset+octet-stream")
        if content_type != "application/offset+octet-stream":
            raise HTTPException(status_code=400, detail="Invalid content type for TUS upload")

        content_length = int(request.headers.get("content-length", "0"))
        if content_length == 0:
            raise HTTPException(status_code=400, detail="Content length required")

        if upload_offset + content_length > session["file_size"]:
            raise HTTPException(status_code=413, detail="Upload exceeds file size")

        # Read and write chunk
        try:
            chunk_data = await request.body()
            if len(chunk_data) != content_length:
                raise HTTPException(
                    status_code=400,
                    detail=f"Content length mismatch. Expected {content_length}, got {len(chunk_data)}"
                )

            # Write chunk to temporary file
            upload_path = await self.storage.get_upload_path(session_id, session["privacy_level"])
            async with aiofiles.open(upload_path, "ab") as f:
                await f.write(chunk_data)

            # Update session
            session["uploaded_bytes"] += content_length
            chunk_number = len(session["chunks"])
            session["chunks"][chunk_number] = {
                "offset": upload_offset,
                "size": content_length,
                "checksum": hashlib.md5(chunk_data).hexdigest(),
                "uploaded_at": datetime.utcnow().isoformat()
            }

            # Update progress
            progress_percentage = int((session["uploaded_bytes"] / session["file_size"]) * 100)
            await self.progress.update_progress(session_id, progress_percentage, "uploading")

            # Check if upload is complete
            if session["uploaded_bytes"] == session["file_size"]:
                session["status"] = "completed"
                await self._finalize_upload(session_id, session)

            # Return TUS response
            response = Response(
                status_code=204,
                headers={
                    "upload-offset": str(session["uploaded_bytes"]),
                    "tus-resumable": "1.0.0"
                }
            )
            return response

        except Exception as e:
            session["status"] = "error"
            session["error"] = str(e)
            raise HTTPException(status_code=500, detail=f"Upload failed: {str(e)}")

    async def handle_head_upload(self, session_id: str) -> Response:
        """Handle TUS HEAD request for upload status"""

        session = self.upload_sessions.get(session_id)
        if not session:
            raise HTTPException(status_code=404, detail="Upload session not found")

        return Response(
            status_code=200,
            headers={
                "upload-offset": str(session["uploaded_bytes"]),
                "upload-length": str(session["file_size"]),
                "tus-resumable": "1.0.0",
                "cache-control": "no-store"
            }
        )

    async def handle_options_upload(self) -> Response:
        """Handle TUS OPTIONS request for capabilities"""

        return Response(
            status_code=200,
            headers={
                "tus-resumable": "1.0.0",
                "tus-version": "1.0.0",
                "tus-extension": "creation,expiration",
                "tus-max-size": str(settings.MAX_UPLOAD_SIZE)
            }
        )

    async def get_upload_status(self, session_id: str) -> Dict[str, Any]:
        """Get detailed upload status"""

        session = self.upload_sessions.get(session_id)
        if not session:
            raise HTTPException(status_code=404, detail="Upload session not found")

        progress_percentage = int((session["uploaded_bytes"] / session["file_size"]) * 100)

        return {
            "session_id": session_id,
            "meeting_id": session["meeting_id"],
            "filename": session["filename"],
            "file_size": session["file_size"],
            "uploaded_bytes": session["uploaded_bytes"],
            "progress_percentage": progress_percentage,
            "status": session["status"],
            "created_at": session["created_at"].isoformat() + "Z",
            "expires_at": session["expires_at"].isoformat() + "Z",
            "chunks_uploaded": len(session["chunks"])
        }

    async def _finalize_upload(self, session_id: str, session: Dict[str, Any]):
        """Finalize upload and move file to permanent storage"""

        try:
            # Validate complete file
            upload_path = await self.storage.get_upload_path(session_id, session["privacy_level"])

            # Verify file size
            actual_size = upload_path.stat().st_size
            if actual_size != session["file_size"]:
                raise ValueError(f"File size mismatch. Expected {session['file_size']}, got {actual_size}")

            # Move to permanent storage
            final_path = await self.storage.finalize_upload(
                session_id,
                session["meeting_id"],
                session["privacy_level"],
                session["metadata"]
            )

            # Update progress to completed
            await self.progress.update_progress(session_id, 100, "completed")

            # Trigger processing job (will be implemented in Phase 3)
            await self._trigger_processing_job(session["meeting_id"], final_path, session)

        except Exception as e:
            session["status"] = "error"
            session["error"] = str(e)
            await self.progress.update_progress(session_id, -1, "error")
            raise

    async def _trigger_processing_job(
        self,
        meeting_id: str,
        file_path: Path,
        session: Dict[str, Any]
    ):
        """Trigger background processing job for uploaded file"""

        # Import here to avoid circular imports
        from app.upload.sessions import session_manager

        # Finalize upload session with database integration
        await session_manager.finalize_upload_session(
            session["session_id"],
            str(file_path),
            db=None  # Will be passed properly when integrated with API
        )

    async def cleanup_expired_sessions(self):
        """Clean up expired upload sessions"""

        now = datetime.utcnow()
        expired_sessions = [
            session_id for session_id, session in self.upload_sessions.items()
            if now > session["expires_at"]
        ]

        for session_id in expired_sessions:
            try:
                session = self.upload_sessions[session_id]
                upload_path = await self.storage.get_upload_path(session_id, session["privacy_level"])
                if upload_path.exists():
                    upload_path.unlink()
                del self.upload_sessions[session_id]
            except Exception as e:
                print(f"Error cleaning up session {session_id}: {e}")


# Global handler instance
tus_handler = TUSHandler()


async def periodic_cleanup():
    """Periodic cleanup of expired sessions"""
    while True:
        try:
            await tus_handler.cleanup_expired_sessions()
            await asyncio.sleep(3600)  # Run every hour
        except Exception as e:
            print(f"Error in periodic cleanup: {e}")
            await asyncio.sleep(300)  # Retry in 5 minutes