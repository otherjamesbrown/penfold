"""Unit tests for ingest event schemas.

Tests the Pydantic event models for manual ingest:
- ManualEmailIngestedEvent serialization
- IngestJobProgressEvent validation
- IngestJobCompletedEvent serialization
"""

from datetime import datetime, timezone
from uuid import uuid4

import pytest

from penf_lib.events.schemas import (
    ManualEmailIngestedEvent,
    IngestJobProgressEvent,
    IngestJobCompletedEvent,
    ManualAttachmentExtractedEvent,
)


class TestManualEmailIngestedEvent:
    """Tests for ManualEmailIngestedEvent schema."""

    def test_create_valid_event(self):
        """Create a valid ManualEmailIngestedEvent."""
        event = ManualEmailIngestedEvent(
            source_id=12345,
            tenant_id=str(uuid4()),
            message_id="<test@example.com>",
            job_id=str(uuid4()),
            from_email="sender@example.com",
            to_emails=["recipient@example.com"],
            email_date=datetime.now(timezone.utc),
            has_attachments=False,
            content_hash="abc123def456",
            source_tag="outlook-archive",
            original_file_path="inbox/email.eml",
        )

        assert event.event_type == "manual_email.ingested"
        assert event.source_id == 12345
        assert event.from_email == "sender@example.com"

    def test_event_serialization(self):
        """Event serializes to JSON correctly."""
        event = ManualEmailIngestedEvent(
            source_id=12345,
            tenant_id=str(uuid4()),
            message_id="<test@example.com>",
            job_id=str(uuid4()),
            from_email="sender@example.com",
            to_emails=["recipient@example.com"],
            email_date=datetime.now(timezone.utc),
            has_attachments=True,
            attachment_count=2,
            content_hash="abc123def456",
            source_tag="outlook-archive",
            original_file_path="inbox/email.eml",
            labels=["inbox", "projects/atlas"],
        )

        data = event.model_dump()

        assert data["event_type"] == "manual_email.ingested"
        assert data["source_id"] == 12345
        assert data["has_attachments"] is True
        assert data["attachment_count"] == 2
        assert len(data["labels"]) == 2

    def test_optional_fields_default(self):
        """Optional fields have correct defaults."""
        event = ManualEmailIngestedEvent(
            source_id=12345,
            tenant_id=str(uuid4()),
            message_id="<test@example.com>",
            job_id=str(uuid4()),
            from_email="sender@example.com",
            to_emails=["recipient@example.com"],
            email_date=datetime.now(timezone.utc),
            has_attachments=False,
            content_hash="abc123def456",
            source_tag="outlook-archive",
            original_file_path="inbox/email.eml",
        )

        assert event.from_name is None
        assert event.in_reply_to is None
        assert event.thread_id is None
        assert event.date_is_fallback is False
        assert event.cc_emails == []
        assert event.labels == []


class TestIngestJobProgressEvent:
    """Tests for IngestJobProgressEvent schema."""

    def test_create_progress_event(self):
        """Create a valid progress event."""
        event = IngestJobProgressEvent(
            job_id=str(uuid4()),
            tenant_id=str(uuid4()),
            total_files=100,
            processed_count=50,
            imported_count=45,
            skipped_count=3,
            failed_count=2,
            current_file="inbox/email_50.eml",
            elapsed_seconds=30.5,
            status="in_progress",
        )

        assert event.event_type == "ingest_job.progress"
        assert event.processed_count == 50
        assert event.status == "in_progress"

    def test_progress_event_serialization(self):
        """Progress event serializes correctly."""
        event = IngestJobProgressEvent(
            job_id=str(uuid4()),
            tenant_id=str(uuid4()),
            total_files=100,
            processed_count=50,
            imported_count=45,
            skipped_count=3,
            failed_count=2,
            elapsed_seconds=30.5,
            estimated_remaining_seconds=28.0,
            status="in_progress",
        )

        data = event.model_dump()

        assert data["total_files"] == 100
        assert data["estimated_remaining_seconds"] == 28.0


class TestIngestJobCompletedEvent:
    """Tests for IngestJobCompletedEvent schema."""

    def test_create_completed_event(self):
        """Create a valid completed event."""
        started = datetime.now(timezone.utc)
        completed = datetime.now(timezone.utc)

        event = IngestJobCompletedEvent(
            job_id=str(uuid4()),
            tenant_id=str(uuid4()),
            source_tag="outlook-archive",
            total_files=100,
            imported_count=95,
            skipped_count=3,
            failed_count=2,
            started_at=started,
            completed_at=completed,
            duration_seconds=45.5,
            success=False,
            final_status="completed_with_errors",
        )

        assert event.event_type == "ingest_job.completed"
        assert event.success is False
        assert event.final_status == "completed_with_errors"

    def test_successful_completion(self):
        """Create event for successful completion."""
        started = datetime.now(timezone.utc)
        completed = datetime.now(timezone.utc)

        event = IngestJobCompletedEvent(
            job_id=str(uuid4()),
            tenant_id=str(uuid4()),
            source_tag="outlook-archive",
            total_files=100,
            imported_count=95,
            skipped_count=5,
            failed_count=0,
            started_at=started,
            completed_at=completed,
            duration_seconds=45.5,
            success=True,
            final_status="completed",
            labels_created=["inbox", "sent"],
            attachments_extracted=20,
        )

        assert event.success is True
        assert event.final_status == "completed"
        assert len(event.labels_created) == 2
        assert event.attachments_extracted == 20


class TestManualAttachmentExtractedEvent:
    """Tests for ManualAttachmentExtractedEvent schema."""

    def test_create_extraction_event(self):
        """Create a valid attachment extraction event."""
        event = ManualAttachmentExtractedEvent(
            attachment_id=str(uuid4()),
            source_id=12345,
            tenant_id=str(uuid4()),
            filename="report.pdf",
            mime_type="application/pdf",
            size_bytes=1024000,
            extraction_status="completed",
        )

        assert event.event_type == "manual_attachment.extracted"
        assert event.filename == "report.pdf"
        assert event.extraction_status == "completed"

    def test_skipped_oversized_event(self):
        """Create event for oversized attachment."""
        event = ManualAttachmentExtractedEvent(
            attachment_id=str(uuid4()),
            source_id=12345,
            tenant_id=str(uuid4()),
            filename="large_video.mp4",
            mime_type="video/mp4",
            size_bytes=30 * 1024 * 1024,  # 30MB
            extraction_status="skipped_oversized",
        )

        assert event.extraction_status == "skipped_oversized"
        assert event.document_id is None
