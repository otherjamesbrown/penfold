"""
Upload Session Management

Manages upload sessions, database integration, and coordination between
TUS uploads, file storage, and processing job creation.
"""
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, update, delete
from typing import Dict, Any, Optional, List
import uuid
from datetime import datetime, timedelta
import json

from app.database import get_db, MeetingFile, ProcessingJob
from app.upload.handlers import tus_handler
from app.upload.progress import progress_tracker
from app.upload.validation import FileValidator
from app.config import settings


class UploadSessionManager:
    """Manages upload sessions and database integration"""

    def __init__(self):
        self.validator = FileValidator()

    async def create_upload_session(
        self,
        filename: str,
        file_size: int,
        privacy_level: str,
        metadata: Optional[Dict[str, Any]] = None,
        db: AsyncSession = None
    ) -> Dict[str, Any]:
        """Create new upload session with database record"""

        # Validate request
        validation_result = await self.validator.validate_upload_request(
            filename, file_size, privacy_level, metadata
        )

        if not validation_result["valid"]:
            raise ValueError(f"Validation failed: {validation_result['errors']}")

        # Create TUS session
        tus_session = await tus_handler.create_upload_session(
            filename, file_size, privacy_level, metadata
        )

        # Create database record
        meeting_file = MeetingFile(
            id=uuid.UUID(tus_session["meeting_id"]),
            filename=filename,
            size_bytes=file_size,
            privacy_level=privacy_level,
            storage_location="local",
            metadata={
                "upload_session_id": tus_session["upload_session_id"],
                "estimated_processing_time": validation_result.get("estimated_processing_time"),
                "user_metadata": metadata or {},
                "upload_initiated_at": datetime.utcnow().isoformat()
            }
        )

        # Save to database if session provided
        if db:
            db.add(meeting_file)
            await db.commit()

        # Initialize progress tracking
        await progress_tracker.update_progress(
            session_id=tus_session["upload_session_id"],
            progress_percentage=0,
            stage="session_created",
            message="Upload session created successfully"
        )

        return {
            **tus_session,
            "estimated_processing_time": validation_result.get("estimated_processing_time", 300),
            "supported_formats": self.validator.get_supported_formats()
        }

    async def finalize_upload_session(
        self,
        session_id: str,
        final_file_path: str,
        db: AsyncSession = None
    ) -> bool:
        """Finalize upload session and create processing jobs"""

        try:
            # Get upload status
            session_status = await tus_handler.get_upload_status(session_id)
            if not session_status or session_status["status"] != "completed":
                raise ValueError("Upload session not completed")

            meeting_id = uuid.UUID(session_status["meeting_id"])

            # Update database record
            if db:
                # Update meeting file record
                stmt = update(MeetingFile).where(
                    MeetingFile.id == meeting_id
                ).values(
                    upload_completed_at=datetime.utcnow(),
                    storage_location=final_file_path,
                    metadata=MeetingFile.metadata + {
                        "upload_completed_at": datetime.utcnow().isoformat(),
                        "final_storage_path": final_file_path,
                        "upload_session_finalized": True
                    }
                )
                await db.execute(stmt)

                # Create processing job
                processing_job = ProcessingJob(
                    meeting_file_id=meeting_id,
                    job_type="transcription",
                    status="queued",
                    progress=0,
                    stage="queued_for_transcription",
                    estimated_completion_time=datetime.utcnow() + timedelta(minutes=30),
                    idempotency_key=f"transcription_{meeting_id}_{int(datetime.utcnow().timestamp())}"
                )

                db.add(processing_job)
                await db.commit()

                # Update progress
                await progress_tracker.update_progress(
                    session_id=session_id,
                    progress_percentage=100,
                    stage="upload_complete",
                    message="Upload completed successfully. Queued for processing.",
                    data={
                        "meeting_id": str(meeting_id),
                        "processing_job_id": str(processing_job.id)
                    }
                )

            return True

        except Exception as e:
            # Mark session as failed
            await progress_tracker.update_progress(
                session_id=session_id,
                progress_percentage=-1,
                stage="failed",
                message=f"Session finalization failed: {str(e)}"
            )
            raise

    async def get_session_info(
        self,
        session_id: str,
        db: AsyncSession = None
    ) -> Optional[Dict[str, Any]]:
        """Get comprehensive session information"""

        # Get TUS session status
        tus_status = await tus_handler.get_upload_status(session_id)
        if not tus_status:
            return None

        # Get progress tracking info
        progress_info = await progress_tracker.get_session_status(session_id)

        # Get database info if available
        database_info = None
        if db and tus_status.get("meeting_id"):
            try:
                meeting_id = uuid.UUID(tus_status["meeting_id"])
                stmt = select(MeetingFile).where(MeetingFile.id == meeting_id)
                result = await db.execute(stmt)
                meeting_file = result.scalar_one_or_none()

                if meeting_file:
                    database_info = {
                        "meeting_id": str(meeting_file.id),
                        "filename": meeting_file.filename,
                        "privacy_level": meeting_file.privacy_level,
                        "created_at": meeting_file.created_at.isoformat() + "Z",
                        "upload_completed_at": (
                            meeting_file.upload_completed_at.isoformat() + "Z"
                            if meeting_file.upload_completed_at else None
                        ),
                        "metadata": meeting_file.metadata
                    }
            except Exception as e:
                print(f"Error fetching database info for session {session_id}: {e}")

        return {
            "session_id": session_id,
            "tus_status": tus_status,
            "progress_info": progress_info,
            "database_info": database_info,
            "last_updated": datetime.utcnow().isoformat() + "Z"
        }

    async def cancel_upload_session(
        self,
        session_id: str,
        reason: str = "User cancelled",
        db: AsyncSession = None
    ) -> bool:
        """Cancel an upload session and clean up resources"""

        try:
            # Get session info
            session_info = await self.get_session_info(session_id, db)
            if not session_info:
                return False

            # Update progress to cancelled
            await progress_tracker.update_progress(
                session_id=session_id,
                progress_percentage=-1,
                stage="cancelled",
                message=f"Upload cancelled: {reason}"
            )

            # Clean up database record if exists
            if db and session_info.get("database_info", {}).get("meeting_id"):
                meeting_id = uuid.UUID(session_info["database_info"]["meeting_id"])

                # Delete any processing jobs
                await db.execute(
                    delete(ProcessingJob).where(ProcessingJob.meeting_file_id == meeting_id)
                )

                # Delete meeting file record
                await db.execute(
                    delete(MeetingFile).where(MeetingFile.id == meeting_id)
                )

                await db.commit()

            return True

        except Exception as e:
            print(f"Error cancelling session {session_id}: {e}")
            return False

    async def get_active_sessions(self, db: AsyncSession = None) -> List[Dict[str, Any]]:
        """Get list of active upload sessions"""

        # Get active sessions from progress tracker
        active_sessions = await progress_tracker.get_active_sessions()

        session_list = []

        for session_id, session_data in active_sessions["sessions"].items():
            session_info = await self.get_session_info(session_id, db)
            if session_info:
                session_list.append({
                    "session_id": session_id,
                    "progress": session_data["progress"],
                    "stage": session_data["stage"],
                    "last_update": session_data["last_update"],
                    "meeting_id": session_info.get("database_info", {}).get("meeting_id"),
                    "filename": session_info.get("database_info", {}).get("filename")
                })

        return session_list

    async def cleanup_expired_sessions(self, db: AsyncSession = None) -> int:
        """Clean up expired sessions and orphaned records"""

        cleaned_count = 0

        try:
            # Clean up TUS sessions
            await tus_handler.cleanup_expired_sessions()

            # Clean up progress tracker
            cleaned_count += await progress_tracker.cleanup_old_sessions()

            # Clean up orphaned database records (sessions without upload completion)
            if db:
                cutoff_time = datetime.utcnow() - timedelta(hours=25)  # 1 hour past TUS expiry

                # Find orphaned meeting files
                stmt = select(MeetingFile).where(
                    MeetingFile.upload_completed_at.is_(None),
                    MeetingFile.created_at < cutoff_time
                )
                result = await db.execute(stmt)
                orphaned_files = result.scalars().all()

                for meeting_file in orphaned_files:
                    # Delete associated processing jobs
                    await db.execute(
                        delete(ProcessingJob).where(
                            ProcessingJob.meeting_file_id == meeting_file.id
                        )
                    )

                    # Delete the meeting file
                    await db.delete(meeting_file)
                    cleaned_count += 1

                await db.commit()

        except Exception as e:
            print(f"Error during session cleanup: {e}")

        return cleaned_count

    async def get_upload_statistics(self, db: AsyncSession = None) -> Dict[str, Any]:
        """Get upload statistics"""

        stats = {
            "active_sessions": 0,
            "total_active_clients": 0,
            "uploads_by_privacy_level": {},
            "recent_uploads": {"last_24h": 0, "last_7d": 0},
            "storage_usage": {}
        }

        # Get active session stats
        active_sessions = await progress_tracker.get_active_sessions()
        stats["active_sessions"] = active_sessions["total_sessions"]
        stats["total_active_clients"] = active_sessions["total_clients"]

        # Get database stats if available
        if db:
            try:
                # Count uploads by privacy level (last 30 days)
                cutoff_30d = datetime.utcnow() - timedelta(days=30)
                for privacy_level in ["public", "organization", "team_only", "confidential"]:
                    stmt = select(MeetingFile).where(
                        MeetingFile.privacy_level == privacy_level,
                        MeetingFile.created_at >= cutoff_30d,
                        MeetingFile.upload_completed_at.is_not(None)
                    )
                    result = await db.execute(stmt)
                    count = len(result.scalars().all())
                    stats["uploads_by_privacy_level"][privacy_level] = count

                # Recent uploads
                cutoff_24h = datetime.utcnow() - timedelta(hours=24)
                cutoff_7d = datetime.utcnow() - timedelta(days=7)

                stmt_24h = select(MeetingFile).where(
                    MeetingFile.upload_completed_at >= cutoff_24h
                )
                result_24h = await db.execute(stmt_24h)
                stats["recent_uploads"]["last_24h"] = len(result_24h.scalars().all())

                stmt_7d = select(MeetingFile).where(
                    MeetingFile.upload_completed_at >= cutoff_7d
                )
                result_7d = await db.execute(stmt_7d)
                stats["recent_uploads"]["last_7d"] = len(result_7d.scalars().all())

            except Exception as e:
                print(f"Error getting database stats: {e}")

        return stats


# Global session manager instance
session_manager = UploadSessionManager()