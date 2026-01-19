"""Unit tests for ingest Pydantic models.

Tests the Pydantic models used for manual ingest:
- ParsedEmail validation
- IngestJobCreate validation
- IngestResult serialization
- AttachmentInfo status tracking
"""

from datetime import datetime, timezone
from pathlib import Path
from uuid import uuid4

import pytest

from penf_lib.ingest.models import (
    AttachmentExtractionStatus,
    AttachmentInfo,
    IngestError,
    IngestJobCreate,
    IngestJobStatus,
    IngestJobSummary,
    IngestResult,
    ParsedEmail,
)


class TestParsedEmail:
    """Tests for ParsedEmail model."""

    def test_create_valid_parsed_email(self):
        """Create a valid ParsedEmail instance."""
        email = ParsedEmail(
            message_id="<test@example.com>",
            from_email="sender@example.com",
            to_emails=["recipient@example.com"],
            raw_content=b"raw email content",
            content_hash="abc123def456",
        )

        assert email.message_id == "<test@example.com>"
        assert email.from_email == "sender@example.com"
        assert email.has_attachments is False
        assert email.attachment_count == 0

    def test_parsed_email_with_attachments(self):
        """ParsedEmail correctly tracks attachments."""
        attachment = AttachmentInfo(
            filename="test.pdf",
            mime_type="application/pdf",
            size_bytes=1024,
        )

        email = ParsedEmail(
            message_id="<test@example.com>",
            from_email="sender@example.com",
            to_emails=["recipient@example.com"],
            raw_content=b"raw email content",
            content_hash="abc123def456",
            attachments=[attachment],
        )

        assert email.has_attachments is True
        assert email.attachment_count == 1

    def test_parsed_email_synthetic_message_id(self):
        """Track synthetic Message-ID correctly."""
        email = ParsedEmail(
            message_id="<synthetic-abc123@penfold.local>",
            message_id_synthetic=True,
            from_email="sender@example.com",
            to_emails=["recipient@example.com"],
            raw_content=b"raw email content",
            content_hash="abc123def456",
        )

        assert email.message_id_synthetic is True

    def test_parsed_email_date_fallback(self):
        """Track date fallback correctly."""
        email = ParsedEmail(
            message_id="<test@example.com>",
            from_email="sender@example.com",
            to_emails=["recipient@example.com"],
            raw_content=b"raw email content",
            content_hash="abc123def456",
            date_fallback_used=True,
            original_date_raw="Invalid Date String",
        )

        assert email.date_fallback_used is True
        assert email.original_date_raw == "Invalid Date String"


class TestIngestJobCreate:
    """Tests for IngestJobCreate model."""

    def test_valid_source_tag(self):
        """Accept valid source tag format."""
        job = IngestJobCreate(
            source_tag="outlook-archive-2024",
            file_paths=[Path("test.eml")],
        )
        assert job.source_tag == "outlook-archive-2024"

    def test_invalid_source_tag_spaces(self):
        """Reject source tag with spaces."""
        with pytest.raises(ValueError, match="alphanumeric"):
            IngestJobCreate(
                source_tag="outlook archive",
                file_paths=[Path("test.eml")],
            )

    def test_invalid_source_tag_special_chars(self):
        """Reject source tag with special characters."""
        with pytest.raises(ValueError, match="alphanumeric"):
            IngestJobCreate(
                source_tag="outlook@archive!",
                file_paths=[Path("test.eml")],
            )

    def test_source_tag_max_length(self):
        """Accept source tag at max length."""
        job = IngestJobCreate(
            source_tag="a" * 100,  # Max length
            file_paths=[Path("test.eml")],
        )
        assert len(job.source_tag) == 100

    def test_default_options(self):
        """Default options are set correctly."""
        job = IngestJobCreate(
            source_tag="test",
            file_paths=[Path("test.eml")],
        )
        assert job.preserve_folders is True
        assert job.skip_attachments is False
        assert job.dry_run is False


class TestAttachmentInfo:
    """Tests for AttachmentInfo model."""

    def test_default_status_pending(self):
        """Default extraction status is pending."""
        attachment = AttachmentInfo(
            filename="test.pdf",
            mime_type="application/pdf",
            size_bytes=1024,
        )
        assert attachment.extraction_status == AttachmentExtractionStatus.PENDING

    def test_skipped_oversized_status(self):
        """Can set skipped_oversized status."""
        attachment = AttachmentInfo(
            filename="large.pdf",
            mime_type="application/pdf",
            size_bytes=30 * 1024 * 1024,  # 30MB
            extraction_status=AttachmentExtractionStatus.SKIPPED_OVERSIZED,
            content=None,
        )
        assert attachment.extraction_status == AttachmentExtractionStatus.SKIPPED_OVERSIZED


class TestIngestResult:
    """Tests for IngestResult model."""

    def test_imported_result(self):
        """Create result for imported file."""
        result = IngestResult(
            file_path="/path/to/email.eml",
            status="imported",
            source_id=12345,
            message_id="<test@example.com>",
        )
        assert result.status == "imported"
        assert result.source_id == 12345

    def test_skipped_result(self):
        """Create result for skipped (duplicate) file."""
        result = IngestResult(
            file_path="/path/to/email.eml",
            status="skipped",
            message_id="<test@example.com>",
        )
        assert result.status == "skipped"
        assert result.source_id is None

    def test_failed_result_with_error(self):
        """Create result for failed file with error."""
        error = IngestError(
            file_path="/path/to/email.eml",
            error_type="parse_error",
            error_message="Invalid MIME structure",
        )
        result = IngestResult(
            file_path="/path/to/email.eml",
            status="failed",
            error=error,
        )
        assert result.status == "failed"
        assert result.error is not None
        assert result.error.error_type == "parse_error"


class TestIngestJobSummary:
    """Tests for IngestJobSummary model."""

    def test_complete_summary(self):
        """Create complete job summary."""
        summary = IngestJobSummary(
            job_id=uuid4(),
            source_tag="outlook-2024",
            total_files=100,
            imported_count=95,
            skipped_count=3,
            failed_count=2,
            labels_created=["inbox", "sent"],
            attachments_extracted=20,
            duration_seconds=45.5,
        )

        assert summary.total_files == 100
        assert summary.imported_count == 95
        assert len(summary.labels_created) == 2


class TestIngestJobStatus:
    """Tests for IngestJobStatus enum."""

    def test_all_statuses_defined(self):
        """All expected statuses are defined."""
        assert IngestJobStatus.PENDING == "pending"
        assert IngestJobStatus.IN_PROGRESS == "in_progress"
        assert IngestJobStatus.COMPLETED == "completed"
        assert IngestJobStatus.FAILED == "failed"
        assert IngestJobStatus.CANCELLED == "cancelled"
