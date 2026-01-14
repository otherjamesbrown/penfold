"""
Transcription Job Management with Procrastinate

Manages background transcription jobs with real-time status tracking,
error handling, and integration with the upload pipeline.
"""
import procrastinate
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, update
from pathlib import Path
from typing import Dict, Any, Optional
import asyncio
from datetime import datetime, timedelta
import json
import uuid

from app.config import get_database_url, settings
from app.database import get_db, MeetingFile, MeetingTranscript, ProcessingJob, MeetingParticipant
from app.transcription.service import transcription_service
from app.upload.progress import progress_tracker
from app.upload.sessions import session_manager


# Initialize Procrastinate app
app = procrastinate.App(
    connector=procrastinate.AiopgConnector(
        get_database_url().replace("postgresql+asyncpg://", "postgresql://")
    )
)


class TranscriptionJobManager:
    """Manages transcription jobs and status tracking"""

    def __init__(self):
        self.app = app
        self.max_retries = 3
        self.retry_delays = [60, 300, 900]  # 1min, 5min, 15min

    async def queue_transcription_job(
        self,
        meeting_id: str,
        file_path: str,
        priority: int = 10,
        language: str = "en"
    ) -> str:
        """Queue a new transcription job"""

        try:
            # Create job in database
            async with AsyncSession(bind=app.connector.engine) as db:
                # Get meeting file
                stmt = select(MeetingFile).where(MeetingFile.id == uuid.UUID(meeting_id))
                result = await db.execute(stmt)
                meeting_file = result.scalar_one_or_none()

                if not meeting_file:
                    raise ValueError(f"Meeting file not found: {meeting_id}")

                # Create processing job record
                job_record = ProcessingJob(
                    meeting_file_id=meeting_file.id,
                    job_type="transcription",
                    status="queued",
                    progress=0,
                    stage="queued_for_transcription",
                    estimated_completion_time=datetime.utcnow() + timedelta(minutes=30),
                    idempotency_key=f"transcription_{meeting_id}_{int(datetime.utcnow().timestamp())}"
                )

                db.add(job_record)
                await db.commit()
                await db.refresh(job_record)

                job_id = str(job_record.id)

            # Queue Procrastinate job
            procrastinate_job = await self.app.configure_task("transcribe_meeting").defer_async(
                job_id=job_id,
                meeting_id=meeting_id,
                file_path=file_path,
                language=language
            )

            print(f"📋 Transcription job queued: {job_id} (Procrastinate: {procrastinate_job.id})")

            # Update progress
            session_id = meeting_file.metadata.get("upload_session_id") if meeting_file.metadata else None
            if session_id:
                await progress_tracker.update_progress(
                    session_id=session_id,
                    progress_percentage=100,  # Upload complete
                    stage="transcription_queued",
                    message="File uploaded successfully, transcription queued",
                    data={"job_id": job_id, "meeting_id": meeting_id}
                )

            return job_id

        except Exception as e:
            print(f"❌ Failed to queue transcription job: {str(e)}")
            raise

    async def get_job_status(self, job_id: str) -> Dict[str, Any]:
        """Get current job status"""

        try:
            async with AsyncSession(bind=app.connector.engine) as db:
                stmt = select(ProcessingJob).where(ProcessingJob.id == uuid.UUID(job_id))
                result = await db.execute(stmt)
                job = result.scalar_one_or_none()

                if not job:
                    return {"error": "Job not found"}

                return {
                    "job_id": str(job.id),
                    "meeting_file_id": str(job.meeting_file_id),
                    "status": job.status,
                    "progress": job.progress,
                    "stage": job.stage,
                    "created_at": job.created_at.isoformat() + "Z",
                    "started_at": job.started_at.isoformat() + "Z" if job.started_at else None,
                    "completed_at": job.completed_at.isoformat() + "Z" if job.completed_at else None,
                    "estimated_completion": job.estimated_completion_time.isoformat() + "Z" if job.estimated_completion_time else None,
                    "error_details": job.error_details
                }

        except Exception as e:
            return {"error": f"Failed to get job status: {str(e)}"}

    async def cancel_job(self, job_id: str, reason: str = "User cancelled") -> bool:
        """Cancel a transcription job"""

        try:
            async with AsyncSession(bind=app.connector.engine) as db:
                # Update job status
                stmt = update(ProcessingJob).where(
                    ProcessingJob.id == uuid.UUID(job_id)
                ).values(
                    status="cancelled",
                    completed_at=datetime.utcnow(),
                    error_details={"cancellation_reason": reason}
                )
                result = await db.execute(stmt)
                await db.commit()

                if result.rowcount > 0:
                    print(f"🚫 Transcription job cancelled: {job_id}")
                    return True
                else:
                    return False

        except Exception as e:
            print(f"❌ Failed to cancel job: {str(e)}")
            return False


# Job manager instance
job_manager = TranscriptionJobManager()


@app.task(queue="transcription", retry=3)
async def transcribe_meeting(
    ctx,
    job_id: str,
    meeting_id: str,
    file_path: str,
    language: str = "en"
):
    """Procrastinate task for meeting transcription"""

    print(f"🎤 Starting transcription job {job_id} for meeting {meeting_id}")

    async with AsyncSession(bind=app.connector.engine) as db:
        try:
            # Update job to processing
            await _update_job_status(db, job_id, "processing", 5, "starting_transcription")

            # Get meeting file for session tracking
            stmt = select(MeetingFile).where(MeetingFile.id == uuid.UUID(meeting_id))
            result = await db.execute(stmt)
            meeting_file = result.scalar_one_or_none()

            session_id = None
            if meeting_file and meeting_file.metadata:
                session_id = meeting_file.metadata.get("upload_session_id")

            if session_id:
                await progress_tracker.update_progress(
                    session_id=session_id,
                    progress_percentage=10,
                    stage="transcription_started",
                    message="Transcription processing started"
                )

            # Update job progress
            await _update_job_status(db, job_id, "processing", 15, "analyzing_audio_quality")

            if session_id:
                await progress_tracker.update_progress(
                    session_id=session_id,
                    progress_percentage=20,
                    stage="analyzing_quality",
                    message="Analyzing audio quality"
                )

            # Perform transcription
            audio_path = Path(file_path)
            if not audio_path.exists():
                raise FileNotFoundError(f"Audio file not found: {file_path}")

            # Update progress
            await _update_job_status(db, job_id, "processing", 30, "transcribing_audio")

            if session_id:
                await progress_tracker.update_progress(
                    session_id=session_id,
                    progress_percentage=40,
                    stage="transcribing",
                    message="Transcribing audio content"
                )

            # Run transcription
            transcription_result = await transcription_service.transcribe_meeting(
                audio_path=audio_path,
                meeting_id=meeting_id,
                language=language
            )

            if "error" in transcription_result:
                raise RuntimeError(f"Transcription failed: {transcription_result['error']}")

            # Update progress
            await _update_job_status(db, job_id, "processing", 70, "saving_transcript")

            if session_id:
                await progress_tracker.update_progress(
                    session_id=session_id,
                    progress_percentage=75,
                    stage="saving_results",
                    message="Saving transcription results"
                )

            # Save results to database
            await _save_transcription_results(db, meeting_id, transcription_result)

            # Update progress
            await _update_job_status(db, job_id, "processing", 95, "finalizing")

            if session_id:
                await progress_tracker.update_progress(
                    session_id=session_id,
                    progress_percentage=95,
                    stage="finalizing",
                    message="Finalizing transcription"
                )

            # Complete job
            await _update_job_status(
                db, job_id, "completed", 100, "transcription_complete",
                completion_time=datetime.utcnow()
            )

            if session_id:
                await progress_tracker.mark_session_complete(session_id, success=True)

            print(f"✅ Transcription job {job_id} completed successfully")

        except Exception as e:
            error_msg = str(e)
            print(f"❌ Transcription job {job_id} failed: {error_msg}")

            # Update job with error
            await _update_job_status(
                db, job_id, "failed", -1, "transcription_failed",
                error_details={"error": error_msg, "failed_at": datetime.utcnow().isoformat()},
                completion_time=datetime.utcnow()
            )

            # Update session progress
            if session_id:
                await progress_tracker.mark_session_complete(session_id, success=False)

            # Re-raise for Procrastinate retry logic
            raise


async def _update_job_status(
    db: AsyncSession,
    job_id: str,
    status: str,
    progress: int,
    stage: str,
    error_details: Optional[Dict[str, Any]] = None,
    completion_time: Optional[datetime] = None
):
    """Update job status in database"""

    update_data = {
        "status": status,
        "progress": progress,
        "stage": stage
    }

    if status == "processing" and progress == 5:  # First processing update
        update_data["started_at"] = datetime.utcnow()

    if completion_time:
        update_data["completed_at"] = completion_time

    if error_details:
        update_data["error_details"] = error_details

    stmt = update(ProcessingJob).where(
        ProcessingJob.id == uuid.UUID(job_id)
    ).values(**update_data)

    await db.execute(stmt)
    await db.commit()


async def _save_transcription_results(
    db: AsyncSession,
    meeting_id: str,
    transcription_result: Dict[str, Any]
) -> None:
    """Save transcription results to database"""

    result = transcription_result.get("transcription_result", {})

    # Create transcript record
    transcript = MeetingTranscript(
        meeting_file_id=uuid.UUID(meeting_id),
        transcript_text=result.get("transcript_text", ""),
        confidence_score=result.get("confidence_score", 0.0),
        transcription_method=result.get("transcription_method", "unknown"),
        speaker_segments=result.get("speaker_segments", []),
        quality_metrics=result.get("quality_metrics", {})
    )

    db.add(transcript)

    # Create participant records for identified speakers
    speakers_detected = result.get("speakers_detected", [])
    for i, speaker_label in enumerate(speakers_detected):
        participant = MeetingParticipant(
            meeting_file_id=uuid.UUID(meeting_id),
            speaker_label=speaker_label,
            voice_signature_hash=f"hash_{speaker_label}_{meeting_id}",  # Placeholder
            identification_confidence=0.8,  # Default confidence
            is_provisional=True,  # Mark as provisional until manual verification
            contribution_stats=result.get("speaker_statistics", {}).get(speaker_label, {})
        )
        db.add(participant)

    await db.commit()
    print(f"💾 Saved transcript and {len(speakers_detected)} participants for meeting {meeting_id}")


@app.task(queue="maintenance", retry=1)
async def cleanup_old_jobs(ctx, max_age_days: int = 30):
    """Clean up old completed/failed jobs"""

    print(f"🧹 Cleaning up jobs older than {max_age_days} days")

    async with AsyncSession(bind=app.connector.engine) as db:
        try:
            cutoff_date = datetime.utcnow() - timedelta(days=max_age_days)

            stmt = select(ProcessingJob).where(
                ProcessingJob.completed_at < cutoff_date,
                ProcessingJob.status.in_(["completed", "failed", "cancelled"])
            )
            result = await db.execute(stmt)
            old_jobs = result.scalars().all()

            for job in old_jobs:
                await db.delete(job)

            await db.commit()
            print(f"🗑️ Cleaned up {len(old_jobs)} old jobs")

        except Exception as e:
            print(f"❌ Job cleanup failed: {str(e)}")


# Initialize Procrastinate schema on startup
async def init_job_queue():
    """Initialize job queue and schema"""
    try:
        await app.open_async()
        print("✅ Transcription job queue initialized")
    except Exception as e:
        print(f"❌ Failed to initialize job queue: {str(e)}")


# Export the task functions for external access
__all__ = [
    "app",
    "job_manager",
    "transcribe_meeting",
    "cleanup_old_jobs",
    "init_job_queue"
]