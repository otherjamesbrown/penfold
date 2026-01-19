"""
Transcript Version Control

Provides version control for transcript changes with diff tracking,
rollback capabilities, and edit history management.
"""
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select, insert, update, delete, desc, and_
from sqlalchemy.orm import mapped_column, Mapped
from sqlalchemy.dialects.postgresql import UUID, JSONB
from sqlalchemy import String, DateTime, Text, Integer
from sqlalchemy.sql import func
from typing import Dict, Any, List, Optional, Tuple
from datetime import datetime
import uuid
import json
import difflib

from app.database import Base, MeetingTranscript


class TranscriptVersion(Base):
    """Model for transcript version history"""
    __tablename__ = "transcript_versions"

    id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    meeting_file_id: Mapped[uuid.UUID] = mapped_column(UUID(as_uuid=True), nullable=False)
    version_number: Mapped[int] = mapped_column(Integer, nullable=False)
    transcript_text: Mapped[str] = mapped_column(Text, nullable=False)
    speaker_segments: Mapped[Optional[Dict[str, Any]]] = mapped_column(JSONB, nullable=True)
    change_type: Mapped[str] = mapped_column(String(50), nullable=False)  # manual_edit, batch_edit, speaker_assignment, etc.
    change_summary: Mapped[Optional[str]] = mapped_column(Text, nullable=True)
    changed_by: Mapped[str] = mapped_column(String(100), nullable=False)
    created_at: Mapped[datetime] = mapped_column(DateTime(timezone=True), default=func.now())
    diff_from_previous: Mapped[Optional[Dict[str, Any]]] = mapped_column(JSONB, nullable=True)
    metadata: Mapped[Optional[Dict[str, Any]]] = mapped_column(JSONB, nullable=True)


class TranscriptVersioningService:
    """Service for managing transcript versions"""

    def __init__(self):
        self.max_versions_per_meeting = 50  # Keep last 50 versions
        self.diff_context_lines = 3

    async def create_version(
        self,
        meeting_id: str,
        user_id: str,
        change_type: str,
        db: AsyncSession,
        change_summary: str = None
    ) -> str:
        """Create a new version before making changes"""

        try:
            # Get current transcript
            stmt = select(MeetingTranscript).where(
                MeetingTranscript.meeting_file_id == uuid.UUID(meeting_id)
            )
            result = await db.execute(stmt)
            current_transcript = result.scalar_one_or_none()

            if not current_transcript:
                raise ValueError(f"Transcript not found for meeting {meeting_id}")

            # Get next version number
            version_number = await self._get_next_version_number(meeting_id, db)

            # Get previous version for diff calculation
            previous_version = await self._get_latest_version(meeting_id, db)
            diff_data = None

            if previous_version:
                diff_data = self._calculate_diff(
                    previous_version.transcript_text,
                    current_transcript.transcript_text,
                    previous_version.speaker_segments,
                    current_transcript.speaker_segments
                )

            # Create new version
            version_id = uuid.uuid4()
            stmt = insert(TranscriptVersion).values(
                id=version_id,
                meeting_file_id=uuid.UUID(meeting_id),
                version_number=version_number,
                transcript_text=current_transcript.transcript_text,
                speaker_segments=current_transcript.speaker_segments,
                change_type=change_type,
                change_summary=change_summary,
                changed_by=user_id,
                diff_from_previous=diff_data,
                metadata={
                    "confidence_score": float(current_transcript.confidence_score),
                    "transcription_method": current_transcript.transcription_method,
                    "quality_metrics": current_transcript.quality_metrics
                }
            )

            await db.execute(stmt)

            # Clean up old versions if needed
            await self._cleanup_old_versions(meeting_id, db)

            await db.commit()

            print(f"📝 Created transcript version {version_number} for meeting {meeting_id}")
            return str(version_id)

        except Exception as e:
            await db.rollback()
            raise RuntimeError(f"Failed to create version: {str(e)}")

    async def get_edit_history(
        self,
        meeting_id: str,
        db: AsyncSession,
        limit: int = 20
    ) -> List[Dict[str, Any]]:
        """Get edit history for a meeting"""

        try:
            stmt = select(TranscriptVersion).where(
                TranscriptVersion.meeting_file_id == uuid.UUID(meeting_id)
            ).order_by(desc(TranscriptVersion.version_number)).limit(limit)

            result = await db.execute(stmt)
            versions = result.scalars().all()

            history = []
            for version in versions:
                history.append({
                    "version_id": str(version.id),
                    "version_number": version.version_number,
                    "change_type": version.change_type,
                    "change_summary": version.change_summary,
                    "changed_by": version.changed_by,
                    "created_at": version.created_at.isoformat() + "Z",
                    "diff_summary": self._summarize_diff(version.diff_from_previous) if version.diff_from_previous else None,
                    "metadata": version.metadata or {}
                })

            return history

        except Exception as e:
            print(f"❌ Failed to get edit history: {str(e)}")
            return []

    async def get_version_details(
        self,
        version_id: str,
        db: AsyncSession
    ) -> Optional[Dict[str, Any]]:
        """Get detailed information about a specific version"""

        try:
            stmt = select(TranscriptVersion).where(
                TranscriptVersion.id == uuid.UUID(version_id)
            )
            result = await db.execute(stmt)
            version = result.scalar_one_or_none()

            if not version:
                return None

            return {
                "version_id": str(version.id),
                "meeting_id": str(version.meeting_file_id),
                "version_number": version.version_number,
                "transcript_text": version.transcript_text,
                "speaker_segments": version.speaker_segments,
                "change_type": version.change_type,
                "change_summary": version.change_summary,
                "changed_by": version.changed_by,
                "created_at": version.created_at.isoformat() + "Z",
                "diff_from_previous": version.diff_from_previous,
                "metadata": version.metadata or {},
                "word_count": len(version.transcript_text.split()),
                "segment_count": len(version.speaker_segments) if version.speaker_segments else 0
            }

        except Exception as e:
            print(f"❌ Failed to get version details: {str(e)}")
            return None

    async def rollback_to_version(
        self,
        meeting_id: str,
        version_id: str,
        user_id: str,
        db: AsyncSession
    ) -> Dict[str, Any]:
        """Rollback transcript to a specific version"""

        try:
            # Get the target version
            target_version = await self.get_version_details(version_id, db)
            if not target_version:
                raise ValueError(f"Version {version_id} not found")

            if target_version["meeting_id"] != meeting_id:
                raise ValueError("Version does not belong to the specified meeting")

            # Create backup of current state
            current_backup_id = await self.create_version(
                meeting_id, user_id, "pre_rollback_backup", db,
                f"Backup before rollback to version {target_version['version_number']}"
            )

            # Update transcript with version data
            stmt = update(MeetingTranscript).where(
                MeetingTranscript.meeting_file_id == uuid.UUID(meeting_id)
            ).values(
                transcript_text=target_version["transcript_text"],
                speaker_segments=target_version["speaker_segments"],
                quality_metrics={
                    **target_version["metadata"].get("quality_metrics", {}),
                    "rollback_count": target_version["metadata"].get("rollback_count", 0) + 1,
                    "last_rollback_at": datetime.utcnow().isoformat(),
                    "last_rollback_by": user_id,
                    "rollback_to_version": target_version["version_number"]
                }
            )

            await db.execute(stmt)

            # Create rollback version
            rollback_version_id = await self.create_version(
                meeting_id, user_id, "rollback", db,
                f"Rollback to version {target_version['version_number']}"
            )

            await db.commit()

            return {
                "success": True,
                "rolled_back_to_version": target_version["version_number"],
                "backup_version_id": current_backup_id,
                "new_version_id": rollback_version_id,
                "restored_word_count": target_version["word_count"],
                "restored_segment_count": target_version["segment_count"],
                "timestamp": datetime.utcnow().isoformat() + "Z"
            }

        except Exception as e:
            await db.rollback()
            raise RuntimeError(f"Rollback failed: {str(e)}")

    async def compare_versions(
        self,
        version_id_1: str,
        version_id_2: str,
        db: AsyncSession
    ) -> Dict[str, Any]:
        """Compare two versions and show differences"""

        try:
            # Get both versions
            version_1 = await self.get_version_details(version_id_1, db)
            version_2 = await self.get_version_details(version_id_2, db)

            if not version_1 or not version_2:
                raise ValueError("One or both versions not found")

            if version_1["meeting_id"] != version_2["meeting_id"]:
                raise ValueError("Versions are from different meetings")

            # Calculate detailed diff
            text_diff = self._calculate_text_diff(
                version_1["transcript_text"],
                version_2["transcript_text"]
            )

            segments_diff = self._calculate_segments_diff(
                version_1["speaker_segments"] or [],
                version_2["speaker_segments"] or []
            )

            return {
                "version_1": {
                    "id": version_id_1,
                    "version_number": version_1["version_number"],
                    "changed_by": version_1["changed_by"],
                    "created_at": version_1["created_at"]
                },
                "version_2": {
                    "id": version_id_2,
                    "version_number": version_2["version_number"],
                    "changed_by": version_2["changed_by"],
                    "created_at": version_2["created_at"]
                },
                "text_diff": text_diff,
                "segments_diff": segments_diff,
                "comparison_stats": {
                    "word_count_change": version_2["word_count"] - version_1["word_count"],
                    "segment_count_change": version_2["segment_count"] - version_1["segment_count"],
                    "lines_added": text_diff["lines_added"],
                    "lines_removed": text_diff["lines_removed"],
                    "lines_changed": text_diff["lines_changed"]
                }
            }

        except Exception as e:
            raise RuntimeError(f"Version comparison failed: {str(e)}")

    async def _get_next_version_number(self, meeting_id: str, db: AsyncSession) -> int:
        """Get the next version number for a meeting"""

        stmt = select(func.max(TranscriptVersion.version_number)).where(
            TranscriptVersion.meeting_file_id == uuid.UUID(meeting_id)
        )
        result = await db.execute(stmt)
        max_version = result.scalar()

        return (max_version or 0) + 1

    async def _get_latest_version(
        self,
        meeting_id: str,
        db: AsyncSession
    ) -> Optional[TranscriptVersion]:
        """Get the latest version for a meeting"""

        stmt = select(TranscriptVersion).where(
            TranscriptVersion.meeting_file_id == uuid.UUID(meeting_id)
        ).order_by(desc(TranscriptVersion.version_number)).limit(1)

        result = await db.execute(stmt)
        return result.scalar_one_or_none()

    async def _cleanup_old_versions(self, meeting_id: str, db: AsyncSession) -> None:
        """Remove old versions beyond the limit"""

        # Get count of versions
        stmt = select(func.count(TranscriptVersion.id)).where(
            TranscriptVersion.meeting_file_id == uuid.UUID(meeting_id)
        )
        result = await db.execute(stmt)
        version_count = result.scalar()

        if version_count > self.max_versions_per_meeting:
            # Get oldest versions to delete
            versions_to_delete = version_count - self.max_versions_per_meeting

            stmt = select(TranscriptVersion.id).where(
                TranscriptVersion.meeting_file_id == uuid.UUID(meeting_id)
            ).order_by(TranscriptVersion.version_number).limit(versions_to_delete)

            result = await db.execute(stmt)
            old_version_ids = [row[0] for row in result.fetchall()]

            # Delete old versions
            stmt = delete(TranscriptVersion).where(
                TranscriptVersion.id.in_(old_version_ids)
            )
            await db.execute(stmt)

            print(f"🧹 Cleaned up {versions_to_delete} old versions for meeting {meeting_id}")

    def _calculate_diff(
        self,
        old_text: str,
        new_text: str,
        old_segments: Optional[List[Dict]],
        new_segments: Optional[List[Dict]]
    ) -> Dict[str, Any]:
        """Calculate diff between two transcript versions"""

        # Text diff
        text_diff = self._calculate_text_diff(old_text, new_text)

        # Segments diff
        segments_diff = self._calculate_segments_diff(
            old_segments or [], new_segments or []
        )

        return {
            "text_diff": text_diff,
            "segments_diff": segments_diff,
            "summary": {
                "text_changed": text_diff["lines_changed"] > 0,
                "segments_changed": segments_diff["segments_modified"] > 0,
                "total_changes": text_diff["lines_changed"] + segments_diff["segments_modified"]
            }
        }

    def _calculate_text_diff(self, old_text: str, new_text: str) -> Dict[str, Any]:
        """Calculate text diff between two strings"""

        old_lines = old_text.splitlines()
        new_lines = new_text.splitlines()

        differ = difflib.unified_diff(
            old_lines, new_lines,
            fromfile="previous", tofile="current",
            lineterm="", n=self.diff_context_lines
        )

        diff_lines = list(differ)
        added_lines = len([line for line in diff_lines if line.startswith("+")])
        removed_lines = len([line for line in diff_lines if line.startswith("-")])

        return {
            "unified_diff": "\n".join(diff_lines),
            "lines_added": added_lines,
            "lines_removed": removed_lines,
            "lines_changed": max(added_lines, removed_lines),
            "similarity_ratio": difflib.SequenceMatcher(None, old_text, new_text).ratio()
        }

    def _calculate_segments_diff(
        self,
        old_segments: List[Dict],
        new_segments: List[Dict]
    ) -> Dict[str, Any]:
        """Calculate diff between segment arrays"""

        old_count = len(old_segments)
        new_count = len(new_segments)

        segments_added = max(0, new_count - old_count)
        segments_removed = max(0, old_count - new_count)

        # Check for modified segments
        segments_modified = 0
        min_count = min(old_count, new_count)

        for i in range(min_count):
            old_segment = old_segments[i]
            new_segment = new_segments[i]

            # Compare key fields
            if (old_segment.get("text", "") != new_segment.get("text", "") or
                old_segment.get("speaker", "") != new_segment.get("speaker", "")):
                segments_modified += 1

        return {
            "segments_added": segments_added,
            "segments_removed": segments_removed,
            "segments_modified": segments_modified,
            "old_count": old_count,
            "new_count": new_count,
            "net_change": new_count - old_count
        }

    def _summarize_diff(self, diff_data: Dict[str, Any]) -> str:
        """Create a human-readable summary of changes"""

        if not diff_data or not diff_data.get("summary"):
            return "No changes"

        summary_parts = []
        summary = diff_data["summary"]

        if summary.get("text_changed"):
            text_diff = diff_data.get("text_diff", {})
            lines_changed = text_diff.get("lines_changed", 0)
            if lines_changed > 0:
                summary_parts.append(f"{lines_changed} text changes")

        if summary.get("segments_changed"):
            segments_diff = diff_data.get("segments_diff", {})
            segments_modified = segments_diff.get("segments_modified", 0)
            if segments_modified > 0:
                summary_parts.append(f"{segments_modified} segment changes")

            segments_added = segments_diff.get("segments_added", 0)
            segments_removed = segments_diff.get("segments_removed", 0)
            if segments_added > 0:
                summary_parts.append(f"+{segments_added} segments")
            if segments_removed > 0:
                summary_parts.append(f"-{segments_removed} segments")

        return ", ".join(summary_parts) if summary_parts else "Minor changes"


# Global versioning service
transcript_versioning = TranscriptVersioningService()