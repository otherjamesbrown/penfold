"""
Transcript Editing Interface

Provides web-based interface for manual transcript corrections with
real-time updates, speaker assignment, and inline editing capabilities.
"""
from fastapi import APIRouter, HTTPException, Depends, Query
from fastapi.responses import HTMLResponse, JSONResponse
from fastapi.templating import Jinja2Templates
from fastapi.requests import Request
from sqlalchemy.ext.asyncio import AsyncSession
from pydantic import BaseModel
from typing import Dict, Any, List, Optional
from datetime import datetime
import uuid
import json

from app.database import get_db, MeetingTranscript, MeetingFile, MeetingParticipant
from app.search.privacy import privacy_filter, UserRole
from app.review.versioning import transcript_versioning

router = APIRouter()
templates = Jinja2Templates(directory="app/ui/templates")


class TranscriptEdit(BaseModel):
    """Model for transcript edit operations"""
    segment_id: str
    edit_type: str  # text_correction, speaker_assignment, segment_split, segment_merge
    new_text: Optional[str] = None
    new_speaker: Optional[str] = None
    timestamp: Optional[float] = None
    confidence_override: Optional[float] = None


class SpeakerAssignment(BaseModel):
    """Model for speaker assignment operations"""
    segment_ids: List[str]
    speaker_name: str
    person_id: Optional[str] = None
    confidence: float = 1.0


class TranscriptEditorService:
    """Service for transcript editing operations"""

    def __init__(self):
        self.edit_types = {
            "text_correction": "Correct transcript text",
            "speaker_assignment": "Assign speaker to segment",
            "segment_split": "Split segment at timestamp",
            "segment_merge": "Merge segments together",
            "timing_adjustment": "Adjust segment timing"
        }

    async def get_transcript_for_editing(
        self,
        meeting_id: str,
        db: AsyncSession,
        user_id: str,
        user_role: UserRole
    ) -> Dict[str, Any]:
        """Get transcript data formatted for editing interface"""

        try:
            # Get transcript and meeting data
            from sqlalchemy import select, join

            stmt = select(
                MeetingTranscript.id,
                MeetingTranscript.transcript_text,
                MeetingTranscript.speaker_segments,
                MeetingTranscript.confidence_score,
                MeetingTranscript.transcription_method,
                MeetingTranscript.quality_metrics,
                MeetingFile.filename,
                MeetingFile.privacy_level,
                MeetingFile.created_at
            ).select_from(
                MeetingTranscript.join(MeetingFile, MeetingTranscript.meeting_file_id == MeetingFile.id)
            ).where(MeetingTranscript.meeting_file_id == uuid.UUID(meeting_id))

            result = await db.execute(stmt)
            row = result.first()

            if not row:
                raise HTTPException(status_code=404, detail="Transcript not found")

            # Check access permissions
            if not privacy_filter.check_content_access(user_id, user_role, row.privacy_level):
                raise HTTPException(status_code=403, detail="Access denied")

            # Format speaker segments for editing
            editing_segments = self._format_segments_for_editing(row.speaker_segments or [])

            # Get participant information
            participants = await self._get_meeting_participants(meeting_id, db)

            # Get edit history
            edit_history = await transcript_versioning.get_edit_history(meeting_id, db, limit=10)

            return {
                "meeting_id": meeting_id,
                "transcript_id": str(row.id),
                "filename": row.filename,
                "privacy_level": row.privacy_level,
                "created_at": row.created_at.isoformat() + "Z",
                "confidence_score": float(row.confidence_score),
                "transcription_method": row.transcription_method,
                "quality_metrics": row.quality_metrics or {},
                "segments": editing_segments,
                "participants": participants,
                "edit_history": edit_history,
                "editor_metadata": {
                    "total_segments": len(editing_segments),
                    "editable_fields": ["text", "speaker", "timestamp"],
                    "available_edit_types": list(self.edit_types.keys()),
                    "auto_save_enabled": True
                }
            }

        except HTTPException:
            raise
        except Exception as e:
            raise HTTPException(status_code=500, detail=f"Failed to load transcript: {str(e)}")

    async def apply_transcript_edit(
        self,
        meeting_id: str,
        edit: TranscriptEdit,
        user_id: str,
        db: AsyncSession
    ) -> Dict[str, Any]:
        """Apply a single edit to the transcript"""

        try:
            # Load current transcript
            stmt = select(MeetingTranscript).where(
                MeetingTranscript.meeting_file_id == uuid.UUID(meeting_id)
            )
            result = await db.execute(stmt)
            transcript = result.scalar_one_or_none()

            if not transcript:
                raise HTTPException(status_code=404, detail="Transcript not found")

            # Create version before editing
            await transcript_versioning.create_version(meeting_id, user_id, "manual_edit", db)

            # Apply the edit
            updated_segments = self._apply_edit_to_segments(
                transcript.speaker_segments or [], edit
            )

            # Update transcript
            from sqlalchemy import update

            # Rebuild full transcript text
            full_text = self._rebuild_transcript_text(updated_segments)

            stmt = update(MeetingTranscript).where(
                MeetingTranscript.meeting_file_id == uuid.UUID(meeting_id)
            ).values(
                speaker_segments=updated_segments,
                transcript_text=full_text,
                quality_metrics={
                    **transcript.quality_metrics,
                    "manual_edits_count": transcript.quality_metrics.get("manual_edits_count", 0) + 1,
                    "last_edited_at": datetime.utcnow().isoformat(),
                    "last_edited_by": user_id
                }
            )

            await db.execute(stmt)
            await db.commit()

            return {
                "success": True,
                "edit_applied": edit.dict(),
                "updated_segment": self._find_segment_by_id(updated_segments, edit.segment_id),
                "total_segments": len(updated_segments),
                "transcript_length": len(full_text),
                "timestamp": datetime.utcnow().isoformat() + "Z"
            }

        except HTTPException:
            raise
        except Exception as e:
            await db.rollback()
            raise HTTPException(status_code=500, detail=f"Edit failed: {str(e)}")

    async def batch_apply_edits(
        self,
        meeting_id: str,
        edits: List[TranscriptEdit],
        user_id: str,
        db: AsyncSession
    ) -> Dict[str, Any]:
        """Apply multiple edits to transcript in batch"""

        try:
            # Load current transcript
            stmt = select(MeetingTranscript).where(
                MeetingTranscript.meeting_file_id == uuid.UUID(meeting_id)
            )
            result = await db.execute(stmt)
            transcript = result.scalar_one_or_none()

            if not transcript:
                raise HTTPException(status_code=404, detail="Transcript not found")

            # Create version before batch edit
            await transcript_versioning.create_version(meeting_id, user_id, "batch_edit", db)

            # Apply all edits
            updated_segments = transcript.speaker_segments or []
            successful_edits = []
            failed_edits = []

            for edit in edits:
                try:
                    updated_segments = self._apply_edit_to_segments(updated_segments, edit)
                    successful_edits.append(edit.segment_id)
                except Exception as e:
                    failed_edits.append({"segment_id": edit.segment_id, "error": str(e)})

            # Update transcript if any edits succeeded
            if successful_edits:
                from sqlalchemy import update

                full_text = self._rebuild_transcript_text(updated_segments)

                stmt = update(MeetingTranscript).where(
                    MeetingTranscript.meeting_file_id == uuid.UUID(meeting_id)
                ).values(
                    speaker_segments=updated_segments,
                    transcript_text=full_text,
                    quality_metrics={
                        **transcript.quality_metrics,
                        "manual_edits_count": transcript.quality_metrics.get("manual_edits_count", 0) + len(successful_edits),
                        "last_edited_at": datetime.utcnow().isoformat(),
                        "last_edited_by": user_id,
                        "batch_edit_count": transcript.quality_metrics.get("batch_edit_count", 0) + 1
                    }
                )

                await db.execute(stmt)
                await db.commit()

            return {
                "success": len(successful_edits) > 0,
                "successful_edits": len(successful_edits),
                "failed_edits": len(failed_edits),
                "failed_details": failed_edits,
                "total_segments": len(updated_segments),
                "timestamp": datetime.utcnow().isoformat() + "Z"
            }

        except HTTPException:
            raise
        except Exception as e:
            await db.rollback()
            raise HTTPException(status_code=500, detail=f"Batch edit failed: {str(e)}")

    async def assign_speakers(
        self,
        meeting_id: str,
        assignment: SpeakerAssignment,
        user_id: str,
        db: AsyncSession
    ) -> Dict[str, Any]:
        """Assign speaker to multiple segments"""

        try:
            # Load transcript
            stmt = select(MeetingTranscript).where(
                MeetingTranscript.meeting_file_id == uuid.UUID(meeting_id)
            )
            result = await db.execute(stmt)
            transcript = result.scalar_one_or_none()

            if not transcript:
                raise HTTPException(status_code=404, detail="Transcript not found")

            # Create version before speaker assignment
            await transcript_versioning.create_version(meeting_id, user_id, "speaker_assignment", db)

            # Update speaker assignments
            updated_segments = transcript.speaker_segments or []
            updated_count = 0

            for segment in updated_segments:
                segment_id = segment.get("id", str(updated_segments.index(segment)))
                if segment_id in assignment.segment_ids:
                    segment["speaker"] = assignment.speaker_name
                    segment["speaker_confidence"] = assignment.confidence
                    segment["manually_assigned"] = True
                    segment["assigned_by"] = user_id
                    segment["assigned_at"] = datetime.utcnow().isoformat()
                    if assignment.person_id:
                        segment["person_id"] = assignment.person_id
                    updated_count += 1

            # Update database
            if updated_count > 0:
                from sqlalchemy import update

                stmt = update(MeetingTranscript).where(
                    MeetingTranscript.meeting_file_id == uuid.UUID(meeting_id)
                ).values(
                    speaker_segments=updated_segments,
                    quality_metrics={
                        **transcript.quality_metrics,
                        "manual_speaker_assignments": transcript.quality_metrics.get("manual_speaker_assignments", 0) + updated_count,
                        "last_edited_at": datetime.utcnow().isoformat(),
                        "last_edited_by": user_id
                    }
                )

                await db.execute(stmt)
                await db.commit()

            return {
                "success": updated_count > 0,
                "segments_updated": updated_count,
                "speaker_assigned": assignment.speaker_name,
                "person_id": assignment.person_id,
                "confidence": assignment.confidence,
                "timestamp": datetime.utcnow().isoformat() + "Z"
            }

        except HTTPException:
            raise
        except Exception as e:
            await db.rollback()
            raise HTTPException(status_code=500, detail=f"Speaker assignment failed: {str(e)}")

    def _format_segments_for_editing(self, segments: List[Dict]) -> List[Dict]:
        """Format segments for the editing interface"""

        formatted_segments = []
        for i, segment in enumerate(segments):
            formatted_segment = {
                "id": segment.get("id", str(i)),
                "text": segment.get("text", ""),
                "speaker": segment.get("speaker", f"Speaker {i+1}"),
                "start_time": segment.get("start_time", 0),
                "end_time": segment.get("end_time", 0),
                "confidence": segment.get("confidence", 0.8),
                "manually_edited": segment.get("manually_corrected", False),
                "speaker_manually_assigned": segment.get("manually_assigned", False),
                "editable": True,
                "word_count": len(segment.get("text", "").split()),
                "duration": segment.get("end_time", 0) - segment.get("start_time", 0),
                "edit_history": segment.get("edit_history", [])
            }
            formatted_segments.append(formatted_segment)

        return formatted_segments

    async def _get_meeting_participants(
        self,
        meeting_id: str,
        db: AsyncSession
    ) -> List[Dict[str, Any]]:
        """Get participant information for the meeting"""

        try:
            from sqlalchemy import select

            stmt = select(MeetingParticipant).where(
                MeetingParticipant.meeting_file_id == uuid.UUID(meeting_id)
            )
            result = await db.execute(stmt)
            participants = result.scalars().all()

            participant_list = []
            for participant in participants:
                participant_list.append({
                    "id": str(participant.id),
                    "speaker_label": participant.speaker_label,
                    "person_id": str(participant.person_id) if participant.person_id else None,
                    "confidence": float(participant.identification_confidence) if participant.identification_confidence else 0.0,
                    "is_provisional": participant.is_provisional,
                    "contribution_stats": participant.contribution_stats or {}
                })

            return participant_list

        except Exception as e:
            print(f"⚠️ Failed to get participants: {str(e)}")
            return []

    def _apply_edit_to_segments(
        self,
        segments: List[Dict],
        edit: TranscriptEdit
    ) -> List[Dict]:
        """Apply a single edit to the segments list"""

        updated_segments = segments.copy()

        for i, segment in enumerate(updated_segments):
            segment_id = segment.get("id", str(i))

            if segment_id == edit.segment_id:
                if edit.edit_type == "text_correction" and edit.new_text:
                    segment["text"] = edit.new_text
                    segment["manually_corrected"] = True
                    segment["last_edited"] = datetime.utcnow().isoformat()

                elif edit.edit_type == "speaker_assignment" and edit.new_speaker:
                    segment["speaker"] = edit.new_speaker
                    segment["manually_assigned"] = True
                    segment["last_edited"] = datetime.utcnow().isoformat()

                elif edit.edit_type == "timing_adjustment" and edit.timestamp:
                    if edit.timestamp < segment.get("end_time", 0):
                        segment["start_time"] = edit.timestamp
                    segment["timing_adjusted"] = True
                    segment["last_edited"] = datetime.utcnow().isoformat()

                if edit.confidence_override:
                    segment["confidence"] = edit.confidence_override

                break

        return updated_segments

    def _rebuild_transcript_text(self, segments: List[Dict]) -> str:
        """Rebuild full transcript text from segments"""

        text_parts = []
        for segment in segments:
            text = segment.get("text", "").strip()
            if text:
                text_parts.append(text)

        return " ".join(text_parts)

    def _find_segment_by_id(self, segments: List[Dict], segment_id: str) -> Optional[Dict]:
        """Find segment by ID"""

        for i, segment in enumerate(segments):
            if segment.get("id", str(i)) == segment_id:
                return segment

        return None


# Global transcript editor service
transcript_editor = TranscriptEditorService()


# API Routes for transcript editing

@router.get("/transcript/{meeting_id}/editor", response_class=HTMLResponse)
async def get_transcript_editor(
    request: Request,
    meeting_id: str,
    db: AsyncSession = Depends(get_db)
):
    """Serve transcript editor interface"""

    # In a real implementation, this would serve an HTML template
    # For now, return a placeholder
    return HTMLResponse(content=f"""
    <!DOCTYPE html>
    <html>
    <head>
        <title>Transcript Editor - {meeting_id}</title>
        <meta charset="utf-8">
        <meta name="viewport" content="width=device-width, initial-scale=1">
        <style>
            body {{ font-family: Arial, sans-serif; margin: 20px; }}
            .segment {{ border: 1px solid #ddd; margin: 10px 0; padding: 10px; }}
            .speaker {{ font-weight: bold; color: #333; }}
            .timestamp {{ color: #666; font-size: 0.9em; }}
            .text {{ margin: 5px 0; padding: 5px; border: 1px solid #eee; }}
            .editable {{ background: #f9f9f9; }}
            .controls {{ margin: 10px 0; }}
            button {{ margin: 5px; padding: 8px 15px; }}
        </style>
    </head>
    <body>
        <h1>Transcript Editor</h1>
        <div class="controls">
            <button onclick="loadTranscript()">Load Transcript</button>
            <button onclick="saveChanges()">Save Changes</button>
            <button onclick="exportTranscript()">Export</button>
        </div>
        <div id="transcript-container">
            <p>Loading transcript for meeting: {meeting_id}</p>
        </div>

        <script>
            const meetingId = '{meeting_id}';

            async function loadTranscript() {{
                try {{
                    const response = await fetch(`/api/v1/transcript/${{meetingId}}/editing-data`);
                    const data = await response.json();
                    renderTranscript(data);
                }} catch (error) {{
                    console.error('Failed to load transcript:', error);
                }}
            }}

            function renderTranscript(data) {{
                const container = document.getElementById('transcript-container');
                let html = `<h3>${{data.filename}}</h3>`;

                data.segments.forEach((segment, index) => {{
                    html += `
                        <div class="segment" data-segment-id="${{segment.id}}">
                            <div class="speaker" contenteditable="true">${{segment.speaker}}</div>
                            <div class="timestamp">${{formatTime(segment.start_time)}} - ${{formatTime(segment.end_time)}}</div>
                            <div class="text editable" contenteditable="true">${{segment.text}}</div>
                        </div>
                    `;
                }});

                container.innerHTML = html;
            }}

            function formatTime(seconds) {{
                const mins = Math.floor(seconds / 60);
                const secs = Math.floor(seconds % 60);
                return `${{mins}}:${{secs.toString().padStart(2, '0')}}`;
            }}

            function saveChanges() {{
                alert('Save functionality would be implemented here');
            }}

            function exportTranscript() {{
                alert('Export functionality would be implemented here');
            }}

            // Auto-load transcript on page load
            loadTranscript();
        </script>
    </body>
    </html>
    """)


@router.get("/transcript/{meeting_id}/editing-data")
async def get_transcript_editing_data(
    meeting_id: str,
    db: AsyncSession = Depends(get_db)
):
    """Get transcript data formatted for editing"""

    # Mock user for demo - in production, get from auth
    user_id = "demo_user"
    user_role = UserRole.MEMBER

    try:
        editing_data = await transcript_editor.get_transcript_for_editing(
            meeting_id, db, user_id, user_role
        )
        return editing_data

    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.post("/transcript/{meeting_id}/edit")
async def apply_transcript_edit(
    meeting_id: str,
    edit: TranscriptEdit,
    db: AsyncSession = Depends(get_db)
):
    """Apply a single edit to the transcript"""

    # Mock user for demo
    user_id = "demo_user"

    try:
        result = await transcript_editor.apply_transcript_edit(
            meeting_id, edit, user_id, db
        )
        return result

    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.post("/transcript/{meeting_id}/batch-edit")
async def apply_batch_edits(
    meeting_id: str,
    edits: List[TranscriptEdit],
    db: AsyncSession = Depends(get_db)
):
    """Apply multiple edits to transcript in batch"""

    # Mock user for demo
    user_id = "demo_user"

    try:
        result = await transcript_editor.batch_apply_edits(
            meeting_id, edits, user_id, db
        )
        return result

    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))


@router.post("/transcript/{meeting_id}/assign-speakers")
async def assign_speakers_to_segments(
    meeting_id: str,
    assignment: SpeakerAssignment,
    db: AsyncSession = Depends(get_db)
):
    """Assign speaker to multiple segments"""

    # Mock user for demo
    user_id = "demo_user"

    try:
        result = await transcript_editor.assign_speakers(
            meeting_id, assignment, user_id, db
        )
        return result

    except Exception as e:
        raise HTTPException(status_code=500, detail=str(e))