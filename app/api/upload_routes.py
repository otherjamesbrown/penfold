"""
Upload API routes for meeting file processing

Handles TUS resumable uploads, file validation, upload session management,
and progress tracking for large meeting files.
"""
from fastapi import APIRouter, HTTPException, Depends, Request, Response
from fastapi.responses import StreamingResponse, JSONResponse
from sqlalchemy.ext.asyncio import AsyncSession
from pydantic import BaseModel, validator
from typing import Dict, Any, Optional, List
import uuid
from datetime import datetime

from app.database import get_db, MeetingFile, ProcessingJob
from app.auth import get_current_user
from app.upload.sessions import session_manager
from app.upload.handlers import tus_handler
from app.upload.progress import progress_tracker
from app.upload.validation import FileValidator

router = APIRouter()


class UploadRequest(BaseModel):
    """Request model for upload initiation"""
    filename: str
    file_size: int
    privacy_level: str
    metadata: Optional[Dict[str, Any]] = None

    @validator("privacy_level")
    def validate_privacy_level(cls, v):
        valid_levels = ["public", "organization", "team_only", "confidential"]
        if v not in valid_levels:
            raise ValueError(f"Privacy level must be one of: {valid_levels}")
        return v

    @validator("file_size")
    def validate_file_size(cls, v):
        if v <= 0:
            raise ValueError("File size must be greater than 0")
        return v


class BatchUploadRequest(BaseModel):
    """Request model for batch upload"""
    meetings: List[UploadRequest]

    @validator("meetings")
    def validate_meeting_count(cls, v):
        if len(v) > 20:
            raise ValueError("Cannot upload more than 20 meetings in a batch")
        return v


@router.post("/meetings/upload")
async def initiate_upload(
    upload_request: UploadRequest,
    db: AsyncSession = Depends(get_db),
    current_user: dict = Depends(get_current_user)
):
    """Initiate meeting file upload session (TUS protocol)"""
    try:
        session_data = await session_manager.create_upload_session(
            filename=upload_request.filename,
            file_size=upload_request.file_size,
            privacy_level=upload_request.privacy_level,
            metadata=upload_request.metadata,
            db=db
        )

        return JSONResponse(
            status_code=201,
            content=session_data
        )

    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Failed to create upload session: {str(e)}")


@router.post("/meetings/batch")
async def batch_upload(
    batch_request: BatchUploadRequest,
    db: AsyncSession = Depends(get_db),
    current_user: dict = Depends(get_current_user)
):
    """Create multiple upload sessions for batch processing"""
    try:
        upload_sessions = []
        batch_id = str(uuid.uuid4())

        for meeting_request in batch_request.meetings:
            session_data = await session_manager.create_upload_session(
                filename=meeting_request.filename,
                file_size=meeting_request.file_size,
                privacy_level=meeting_request.privacy_level,
                metadata={
                    **(meeting_request.metadata or {}),
                    "batch_id": batch_id,
                    "batch_size": len(batch_request.meetings)
                },
                db=db
            )
            upload_sessions.append(session_data)

        return JSONResponse(
            status_code=201,
            content={
                "batch_id": batch_id,
                "upload_sessions": upload_sessions,
                "total_sessions": len(upload_sessions)
            }
        )

    except ValueError as e:
        raise HTTPException(status_code=400, detail=str(e))
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Failed to create batch upload: {str(e)}")


# TUS Protocol Endpoints

@router.options("/uploads/{session_id}")
async def tus_options(session_id: str):
    """TUS OPTIONS request for upload capabilities"""
    return await tus_handler.handle_options_upload()


@router.head("/uploads/{session_id}")
async def tus_head(session_id: str):
    """TUS HEAD request for upload status"""
    try:
        return await tus_handler.handle_head_upload(session_id)
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.patch("/uploads/{session_id}")
async def tus_patch(session_id: str, request: Request):
    """TUS PATCH request for chunk upload"""
    try:
        return await tus_handler.handle_patch_upload(session_id, request)
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/uploads/{session_id}/status")
async def get_upload_status(
    session_id: str,
    db: AsyncSession = Depends(get_db)
):
    """Get detailed upload status"""
    try:
        session_info = await session_manager.get_session_info(session_id, db)
        if not session_info:
            raise HTTPException(status_code=404, detail="Upload session not found")

        return JSONResponse(content=session_info)

    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/uploads/{session_id}/progress")
async def stream_upload_progress(session_id: str, request: Request):
    """Stream upload progress via Server-Sent Events"""
    try:
        return await progress_tracker.stream_progress(session_id, request)
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Failed to stream progress: {str(e)}")


@router.delete("/uploads/{session_id}")
async def cancel_upload(
    session_id: str,
    reason: str = "User cancelled",
    db: AsyncSession = Depends(get_db),
    current_user: dict = Depends(get_current_user)
):
    """Cancel upload session"""
    try:
        success = await session_manager.cancel_upload_session(session_id, reason, db)
        if not success:
            raise HTTPException(status_code=404, detail="Upload session not found")

        return {"message": "Upload session cancelled successfully", "session_id": session_id}

    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


# Meeting Status Endpoints

@router.get("/meetings/{meeting_id}/status")
async def get_meeting_status(
    meeting_id: str,
    db: AsyncSession = Depends(get_db)
):
    """Get processing status for uploaded meeting"""
    try:
        # Get meeting file
        from sqlalchemy import select
        stmt = select(MeetingFile).where(MeetingFile.id == uuid.UUID(meeting_id))
        result = await db.execute(stmt)
        meeting_file = result.scalar_one_or_none()

        if not meeting_file:
            raise HTTPException(status_code=404, detail="Meeting not found")

        # Get processing jobs
        stmt = select(ProcessingJob).where(
            ProcessingJob.meeting_file_id == meeting_file.id
        ).order_by(ProcessingJob.created_at.desc())
        result = await db.execute(stmt)
        jobs = result.scalars().all()

        # Determine overall status
        if not meeting_file.upload_completed_at:
            status = "uploading"
            progress = 0
            stage = "upload_in_progress"
        elif not jobs:
            status = "queued"
            progress = 100  # Upload complete
            stage = "awaiting_processing"
        else:
            latest_job = jobs[0]
            status = latest_job.status
            progress = latest_job.progress
            stage = latest_job.stage

        return {
            "meeting_id": str(meeting_file.id),
            "filename": meeting_file.filename,
            "file_size": meeting_file.size_bytes,
            "privacy_level": meeting_file.privacy_level,
            "status": status,
            "progress": progress,
            "current_stage": stage,
            "upload_completed_at": (
                meeting_file.upload_completed_at.isoformat() + "Z"
                if meeting_file.upload_completed_at else None
            ),
            "created_at": meeting_file.created_at.isoformat() + "Z",
            "estimated_completion": (
                jobs[0].estimated_completion_time.isoformat() + "Z"
                if jobs and jobs[0].estimated_completion_time else None
            ),
            "processing_jobs": [
                {
                    "job_id": str(job.id),
                    "job_type": job.job_type,
                    "status": job.status,
                    "progress": job.progress,
                    "stage": job.stage,
                    "started_at": job.started_at.isoformat() + "Z" if job.started_at else None,
                    "completed_at": job.completed_at.isoformat() + "Z" if job.completed_at else None
                }
                for job in jobs
            ]
        }

    except ValueError:
        raise HTTPException(status_code=400, detail="Invalid meeting ID format")
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/meetings/{meeting_id}/stream")
async def stream_meeting_progress(meeting_id: str, request: Request):
    """Stream meeting processing progress via Server-Sent Events"""
    try:
        # Find the upload session for this meeting
        # For now, we'll use the meeting_id as session_id approximation
        # In production, this would require a proper mapping
        return await progress_tracker.stream_progress(meeting_id, request)
    except Exception as e:
        raise HTTPException(status_code=500, detail=f"Failed to stream meeting progress: {str(e)}")


# Administrative Endpoints

@router.get("/uploads/active")
async def get_active_uploads(db: AsyncSession = Depends(get_db)):
    """Get list of active upload sessions"""
    try:
        active_sessions = await session_manager.get_active_sessions(db)
        return {
            "active_sessions": active_sessions,
            "total_count": len(active_sessions)
        }
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/uploads/statistics")
async def get_upload_statistics(db: AsyncSession = Depends(get_db)):
    """Get upload system statistics"""
    try:
        stats = await session_manager.get_upload_statistics(db)
        return stats
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.post("/uploads/cleanup")
async def cleanup_expired_uploads(
    db: AsyncSession = Depends(get_db),
    current_user: dict = Depends(get_current_user)
):
    """Clean up expired upload sessions (admin endpoint)"""
    # Check admin permissions
    if current_user.get("role") != "admin":
        raise HTTPException(status_code=403, detail="Admin access required")

    try:
        cleaned_count = await session_manager.cleanup_expired_sessions(db)
        return {
            "message": "Cleanup completed",
            "sessions_cleaned": cleaned_count
        }
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/uploads/formats")
async def get_supported_formats():
    """Get list of supported file formats"""
    try:
        validator = FileValidator()
        return {
            "supported_formats": validator.get_supported_formats(),
            "max_file_size": validator.MAX_AUDIO_SIZE,  # Use audio max as general max
            "chunk_size": 10 * 1024 * 1024,  # 10MB
            "supported_privacy_levels": ["public", "organization", "team_only", "confidential"]
        }
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))