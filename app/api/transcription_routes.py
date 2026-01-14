"""
Transcription API Routes

Provides endpoints for monitoring transcription jobs, testing services,
and managing the transcription pipeline.
"""
from fastapi import APIRouter, HTTPException, Depends
from sqlalchemy.ext.asyncio import AsyncSession
from pydantic import BaseModel
from typing import Dict, Any, Optional
import uuid
from datetime import datetime

from app.database import get_db, MeetingFile, MeetingTranscript, ProcessingJob
from app.transcription.service import transcription_service
from app.transcription.jobs import job_manager

router = APIRouter()


class TranscriptionRequest(BaseModel):
    """Request model for manual transcription"""
    meeting_id: str
    language: str = "en"
    force_local: bool = False
    force_cloud: bool = False


@router.get("/transcription/status")
async def get_transcription_service_status():
    """Get status of all transcription services"""
    try:
        status = await transcription_service.get_service_status()
        return status
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.post("/transcription/test")
async def test_transcription_services():
    """Test transcription services functionality"""
    try:
        test_results = await transcription_service.quick_transcription_test()
        return test_results
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/transcription/jobs/{job_id}")
async def get_transcription_job_status(job_id: str):
    """Get status of a specific transcription job"""
    try:
        status = await job_manager.get_job_status(job_id)
        if "error" in status:
            raise HTTPException(status_code=404, detail=status["error"])
        return status
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.delete("/transcription/jobs/{job_id}")
async def cancel_transcription_job(job_id: str, reason: str = "User cancelled"):
    """Cancel a transcription job"""
    try:
        success = await job_manager.cancel_job(job_id, reason)
        if not success:
            raise HTTPException(status_code=404, detail="Job not found or already completed")

        return {"message": "Job cancelled successfully", "job_id": job_id}
    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.post("/transcription/queue")
async def queue_manual_transcription(
    request: TranscriptionRequest,
    db: AsyncSession = Depends(get_db)
):
    """Manually queue transcription for a meeting file"""
    try:
        # Validate meeting exists
        from sqlalchemy import select
        stmt = select(MeetingFile).where(MeetingFile.id == uuid.UUID(request.meeting_id))
        result = await db.execute(stmt)
        meeting_file = result.scalar_one_or_none()

        if not meeting_file:
            raise HTTPException(status_code=404, detail="Meeting file not found")

        if not meeting_file.upload_completed_at:
            raise HTTPException(status_code=400, detail="Meeting file upload not completed")

        # Check if already transcribed
        stmt = select(MeetingTranscript).where(
            MeetingTranscript.meeting_file_id == meeting_file.id
        )
        result = await db.execute(stmt)
        existing_transcript = result.scalar_one_or_none()

        if existing_transcript:
            raise HTTPException(status_code=409, detail="Meeting already transcribed")

        # Queue transcription job
        file_path = meeting_file.storage_location
        job_id = await job_manager.queue_transcription_job(
            meeting_id=request.meeting_id,
            file_path=file_path,
            language=request.language
        )

        return {
            "message": "Transcription job queued successfully",
            "job_id": job_id,
            "meeting_id": request.meeting_id
        }

    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/meetings/{meeting_id}/transcript")
async def get_meeting_transcript(
    meeting_id: str,
    format: str = "json",
    db: AsyncSession = Depends(get_db)
):
    """Get meeting transcript in various formats"""
    try:
        # Get meeting transcript
        from sqlalchemy import select
        stmt = select(MeetingTranscript).where(
            MeetingTranscript.meeting_file_id == uuid.UUID(meeting_id)
        )
        result = await db.execute(stmt)
        transcript = result.scalar_one_or_none()

        if not transcript:
            raise HTTPException(status_code=404, detail="Transcript not found")

        if format == "json":
            return {
                "meeting_id": meeting_id,
                "transcript_text": transcript.transcript_text,
                "confidence_score": transcript.confidence_score,
                "transcription_method": transcript.transcription_method,
                "speaker_segments": transcript.speaker_segments,
                "quality_metrics": transcript.quality_metrics,
                "created_at": transcript.created_at.isoformat() + "Z"
            }

        elif format == "txt":
            from fastapi.responses import PlainTextResponse
            return PlainTextResponse(
                content=transcript.transcript_text,
                media_type="text/plain"
            )

        elif format == "vtt":
            # Generate WebVTT format
            vtt_content = await _generate_vtt_format(transcript)
            from fastapi.responses import PlainTextResponse
            return PlainTextResponse(
                content=vtt_content,
                media_type="text/vtt"
            )

        elif format == "srt":
            # Generate SubRip format
            srt_content = await _generate_srt_format(transcript)
            from fastapi.responses import PlainTextResponse
            return PlainTextResponse(
                content=srt_content,
                media_type="application/x-subrip"
            )

        else:
            raise HTTPException(status_code=400, detail="Unsupported format. Use: json, txt, vtt, srt")

    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.post("/meetings/{meeting_id}/transcript/corrections")
async def submit_transcript_corrections(
    meeting_id: str,
    corrections: Dict[str, Any],
    db: AsyncSession = Depends(get_db)
):
    """Submit manual corrections to transcript"""
    try:
        # Get existing transcript
        from sqlalchemy import select, update
        stmt = select(MeetingTranscript).where(
            MeetingTranscript.meeting_file_id == uuid.UUID(meeting_id)
        )
        result = await db.execute(stmt)
        transcript = result.scalar_one_or_none()

        if not transcript:
            raise HTTPException(status_code=404, detail="Transcript not found")

        # Apply corrections
        corrections_applied = 0
        speaker_segments = transcript.speaker_segments or []

        for correction in corrections.get("corrections", []):
            segment_id = correction.get("segment_id")
            corrected_text = correction.get("corrected_text")
            speaker_correction = correction.get("speaker_correction")

            # Find and update segment
            for segment in speaker_segments:
                if segment.get("id") == segment_id or segment_id == str(speaker_segments.index(segment)):
                    if corrected_text:
                        segment["text"] = corrected_text
                        segment["manually_corrected"] = True
                    if speaker_correction:
                        segment["speaker"] = speaker_correction
                        segment["speaker_corrected"] = True
                    corrections_applied += 1
                    break

        # Update transcript in database
        if corrections_applied > 0:
            # Rebuild full transcript text
            full_transcript = " ".join([seg.get("text", "") for seg in speaker_segments])

            stmt = update(MeetingTranscript).where(
                MeetingTranscript.meeting_file_id == uuid.UUID(meeting_id)
            ).values(
                transcript_text=full_transcript,
                speaker_segments=speaker_segments,
                quality_metrics={
                    **transcript.quality_metrics,
                    "manual_corrections_applied": corrections_applied,
                    "last_correction_at": datetime.utcnow().isoformat()
                }
            )
            await db.execute(stmt)
            await db.commit()

        return {
            "corrections_applied": corrections_applied,
            "transcript_updated": corrections_applied > 0,
            "meeting_id": meeting_id
        }

    except HTTPException:
        raise
    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.get("/transcription/statistics")
async def get_transcription_statistics(db: AsyncSession = Depends(get_db)):
    """Get transcription system statistics"""
    try:
        from sqlalchemy import select, func
        from datetime import datetime, timedelta

        # Count transcripts by method
        stmt = select(
            MeetingTranscript.transcription_method,
            func.count(MeetingTranscript.id).label("count")
        ).group_by(MeetingTranscript.transcription_method)
        result = await db.execute(stmt)
        by_method = {row[0]: row[1] for row in result.fetchall()}

        # Count jobs by status
        stmt = select(
            ProcessingJob.status,
            func.count(ProcessingJob.id).label("count")
        ).where(
            ProcessingJob.job_type == "transcription"
        ).group_by(ProcessingJob.status)
        result = await db.execute(stmt)
        by_status = {row[0]: row[1] for row in result.fetchall()}

        # Recent activity (last 24 hours)
        cutoff_24h = datetime.utcnow() - timedelta(hours=24)
        stmt = select(func.count(MeetingTranscript.id)).where(
            MeetingTranscript.created_at >= cutoff_24h
        )
        result = await db.execute(stmt)
        recent_transcripts = result.scalar() or 0

        # Average confidence
        stmt = select(func.avg(MeetingTranscript.confidence_score))
        result = await db.execute(stmt)
        avg_confidence = result.scalar() or 0.0

        return {
            "transcripts_by_method": by_method,
            "jobs_by_status": by_status,
            "recent_activity": {
                "last_24h_transcripts": recent_transcripts
            },
            "quality_metrics": {
                "average_confidence": float(avg_confidence)
            }
        }

    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


async def _generate_vtt_format(transcript: MeetingTranscript) -> str:
    """Generate WebVTT format from transcript"""
    vtt_lines = ["WEBVTT", ""]

    speaker_segments = transcript.speaker_segments or []
    for i, segment in enumerate(speaker_segments):
        start_time = segment.get("start_time", 0)
        end_time = segment.get("end_time", 0)
        speaker = segment.get("speaker", "Unknown")
        text = segment.get("text", "")

        # Format timestamps
        start_vtt = _format_vtt_timestamp(start_time)
        end_vtt = _format_vtt_timestamp(end_time)

        vtt_lines.append(f"{start_vtt} --> {end_vtt}")
        vtt_lines.append(f"<v {speaker}>{text}")
        vtt_lines.append("")

    return "\n".join(vtt_lines)


async def _generate_srt_format(transcript: MeetingTranscript) -> str:
    """Generate SubRip (SRT) format from transcript"""
    srt_lines = []

    speaker_segments = transcript.speaker_segments or []
    for i, segment in enumerate(speaker_segments, 1):
        start_time = segment.get("start_time", 0)
        end_time = segment.get("end_time", 0)
        speaker = segment.get("speaker", "Unknown")
        text = segment.get("text", "")

        # Format timestamps
        start_srt = _format_srt_timestamp(start_time)
        end_srt = _format_srt_timestamp(end_time)

        srt_lines.append(str(i))
        srt_lines.append(f"{start_srt} --> {end_srt}")
        srt_lines.append(f"[{speaker}] {text}")
        srt_lines.append("")

    return "\n".join(srt_lines)


def _format_vtt_timestamp(seconds: float) -> str:
    """Format timestamp for VTT format (HH:MM:SS.mmm)"""
    hours = int(seconds // 3600)
    minutes = int((seconds % 3600) // 60)
    secs = seconds % 60
    return f"{hours:02d}:{minutes:02d}:{secs:06.3f}"


def _format_srt_timestamp(seconds: float) -> str:
    """Format timestamp for SRT format (HH:MM:SS,mmm)"""
    hours = int(seconds // 3600)
    minutes = int((seconds % 3600) // 60)
    secs = seconds % 60
    millisecs = int((secs % 1) * 1000)
    secs = int(secs)
    return f"{hours:02d}:{minutes:02d}:{secs:02d},{millisecs:03d}"