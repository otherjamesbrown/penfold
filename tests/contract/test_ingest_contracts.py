"""Contract tests for Manual Ingest CLI and Events.

Verifies implementation matches specifications in:
- specs/012-manual-ingest/contracts/cli.md
- specs/012-manual-ingest/contracts/events.md
"""

from datetime import datetime, timezone
from uuid import uuid4

import pytest
from click.testing import CliRunner

from penf_lib.cli.ingest_commands import ingest_group
from penf_lib.events.schemas import (
    IngestJobCompletedEvent,
    IngestJobProgressEvent,
    ManualAttachmentExtractedEvent,
    ManualEmailIngestedEvent,
)


class TestCLIContractSourceFlag:
    """Contract: --source flag is required for all ingest operations."""

    def test_email_without_source_fails(self):
        """penf ingest email without --source must fail."""
        runner = CliRunner()
        result = runner.invoke(ingest_group, ["email", "test.eml"])

        # Exit code for missing required option
        assert result.exit_code != 0
        assert "--source" in result.output.lower() or "source" in result.output.lower()


class TestCLIContractOptions:
    """Contract: CLI options match specification."""

    def test_email_command_has_required_options(self):
        """Email command has all documented options."""
        runner = CliRunner()
        result = runner.invoke(ingest_group, ["email", "--help"])

        assert result.exit_code == 0
        # Required options per contract
        assert "--source" in result.output
        assert "--projects" in result.output
        assert "--dry-run" in result.output
        assert "--no-preserve-folders" in result.output
        assert "--skip-attachments" in result.output
        assert "--verbose" in result.output
        # Added in US6
        assert "--no-archive" in result.output

    def test_jobs_command_has_required_options(self):
        """Jobs command has all documented options."""
        runner = CliRunner()
        result = runner.invoke(ingest_group, ["jobs", "--help"])

        assert result.exit_code == 0
        assert "--status" in result.output
        assert "--limit" in result.output

    def test_resume_command_exists(self):
        """Resume command is available."""
        runner = CliRunner()
        result = runner.invoke(ingest_group, ["resume", "--help"])

        assert result.exit_code == 0
        assert "JOB_ID" in result.output


class TestEventContractManualEmailIngested:
    """Contract: ManualEmailIngestedEvent schema matches specification."""

    def test_event_type_is_correct(self):
        """Event type must be 'manual_email.ingested'."""
        event = ManualEmailIngestedEvent(
            source_id=1,
            tenant_id=str(uuid4()),
            message_id="<test@example.com>",
            job_id=str(uuid4()),
            from_email="sender@example.com",
            to_emails=["recipient@example.com"],
            email_date=datetime.now(timezone.utc),
            has_attachments=False,
            content_hash="abc123",
            source_tag="test",
            original_file_path="test.eml",
        )

        assert event.event_type == "manual_email.ingested"

    def test_required_fields_present(self):
        """All contract-required fields must be present."""
        event = ManualEmailIngestedEvent(
            source_id=12345,
            tenant_id=str(uuid4()),
            message_id="<unique@example.com>",
            job_id=str(uuid4()),
            from_email="bob@example.com",
            to_emails=["alice@example.com"],
            email_date=datetime.now(timezone.utc),
            has_attachments=True,
            attachment_count=2,
            content_hash="abc123def456",
            source_tag="outlook-archive",
            original_file_path="inbox/customer-issue.eml",
        )

        data = event.model_dump()

        # Required per contract
        assert "source_id" in data
        assert "tenant_id" in data
        assert "message_id" in data
        assert "job_id" in data
        assert "from_email" in data
        assert "to_emails" in data
        assert "email_date" in data
        assert "has_attachments" in data
        assert "content_hash" in data
        assert "source_tag" in data
        assert "original_file_path" in data

    def test_optional_fields_have_defaults(self):
        """Optional fields have correct default values per contract."""
        event = ManualEmailIngestedEvent(
            source_id=1,
            tenant_id=str(uuid4()),
            message_id="<test@example.com>",
            job_id=str(uuid4()),
            from_email="sender@example.com",
            to_emails=["recipient@example.com"],
            email_date=datetime.now(timezone.utc),
            has_attachments=False,
            content_hash="abc123",
            source_tag="test",
            original_file_path="test.eml",
        )

        # Optional defaults per contract
        assert event.from_name is None
        assert event.cc_emails == []
        assert event.subject is None
        assert event.date_is_fallback is False
        assert event.in_reply_to is None
        assert event.thread_id is None
        assert event.attachment_count == 0
        assert event.labels == []


class TestEventContractIngestJobProgress:
    """Contract: IngestJobProgressEvent schema matches specification."""

    def test_event_type_is_correct(self):
        """Event type must be 'ingest_job.progress'."""
        event = IngestJobProgressEvent(
            job_id=str(uuid4()),
            tenant_id=str(uuid4()),
            total_files=100,
            processed_count=50,
            imported_count=45,
            skipped_count=3,
            failed_count=2,
            elapsed_seconds=30.5,
            status="in_progress",
        )

        assert event.event_type == "ingest_job.progress"

    def test_required_fields_present(self):
        """All contract-required fields must be present."""
        event = IngestJobProgressEvent(
            job_id=str(uuid4()),
            tenant_id=str(uuid4()),
            total_files=150,
            processed_count=75,
            imported_count=70,
            skipped_count=3,
            failed_count=2,
            elapsed_seconds=45.2,
            status="in_progress",
        )

        data = event.model_dump()

        # Required per contract
        assert "job_id" in data
        assert "tenant_id" in data
        assert "total_files" in data
        assert "processed_count" in data
        assert "imported_count" in data
        assert "skipped_count" in data
        assert "failed_count" in data
        assert "elapsed_seconds" in data
        assert "status" in data


class TestEventContractIngestJobCompleted:
    """Contract: IngestJobCompletedEvent schema matches specification."""

    def test_event_type_is_correct(self):
        """Event type must be 'ingest_job.completed'."""
        event = IngestJobCompletedEvent(
            job_id=str(uuid4()),
            tenant_id=str(uuid4()),
            source_tag="test",
            total_files=100,
            imported_count=95,
            skipped_count=3,
            failed_count=2,
            started_at=datetime.now(timezone.utc),
            completed_at=datetime.now(timezone.utc),
            duration_seconds=45.5,
            success=False,
            final_status="completed_with_errors",
        )

        assert event.event_type == "ingest_job.completed"

    def test_success_when_no_failures(self):
        """success=True when failed_count=0."""
        event = IngestJobCompletedEvent(
            job_id=str(uuid4()),
            tenant_id=str(uuid4()),
            source_tag="test",
            total_files=100,
            imported_count=95,
            skipped_count=5,
            failed_count=0,
            started_at=datetime.now(timezone.utc),
            completed_at=datetime.now(timezone.utc),
            duration_seconds=45.5,
            success=True,
            final_status="completed",
        )

        assert event.success is True
        assert event.final_status == "completed"

    def test_final_status_values(self):
        """final_status matches contract values."""
        # completed
        event1 = IngestJobCompletedEvent(
            job_id=str(uuid4()),
            tenant_id=str(uuid4()),
            source_tag="test",
            total_files=100,
            imported_count=100,
            skipped_count=0,
            failed_count=0,
            started_at=datetime.now(timezone.utc),
            completed_at=datetime.now(timezone.utc),
            duration_seconds=45.5,
            success=True,
            final_status="completed",
        )
        assert event1.final_status == "completed"

        # completed_with_errors
        event2 = IngestJobCompletedEvent(
            job_id=str(uuid4()),
            tenant_id=str(uuid4()),
            source_tag="test",
            total_files=100,
            imported_count=98,
            skipped_count=0,
            failed_count=2,
            started_at=datetime.now(timezone.utc),
            completed_at=datetime.now(timezone.utc),
            duration_seconds=45.5,
            success=False,
            final_status="completed_with_errors",
        )
        assert event2.final_status == "completed_with_errors"


class TestEventContractAttachmentExtracted:
    """Contract: ManualAttachmentExtractedEvent schema matches specification."""

    def test_event_type_is_correct(self):
        """Event type must be 'manual_attachment.extracted'."""
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

    def test_extraction_status_values(self):
        """extraction_status matches contract values."""
        # completed
        event1 = ManualAttachmentExtractedEvent(
            attachment_id=str(uuid4()),
            source_id=1,
            tenant_id=str(uuid4()),
            filename="report.pdf",
            mime_type="application/pdf",
            size_bytes=1024,
            extraction_status="completed",
        )
        assert event1.extraction_status == "completed"

        # skipped_oversized
        event2 = ManualAttachmentExtractedEvent(
            attachment_id=str(uuid4()),
            source_id=1,
            tenant_id=str(uuid4()),
            filename="video.mp4",
            mime_type="video/mp4",
            size_bytes=30 * 1024 * 1024,
            extraction_status="skipped_oversized",
        )
        assert event2.extraction_status == "skipped_oversized"
