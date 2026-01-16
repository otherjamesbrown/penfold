"""Unit tests for attachment extraction.

Tests attachment extraction from emails per US4 acceptance criteria:
- PDF attachments extracted and linked
- Multiple attachments handled
- Oversized attachments (>25MB) skipped with warning
"""

from pathlib import Path

import pytest

from penf_lib.ingest.models import AttachmentExtractionStatus, AttachmentInfo
from penf_lib.ingest.parser import parse_eml_file, parse_eml_bytes


class TestAttachmentExtraction:
    """Tests for extracting attachments from emails."""

    def test_extract_pdf_attachment(self, tmp_path):
        """Extract PDF attachment from email."""
        # Create email with PDF attachment
        eml_content = b"""From: sender@example.com
To: recipient@example.com
Subject: Test with PDF
MIME-Version: 1.0
Content-Type: multipart/mixed; boundary="boundary123"

--boundary123
Content-Type: text/plain

Email body text.

--boundary123
Content-Type: application/pdf
Content-Disposition: attachment; filename="report.pdf"
Content-Transfer-Encoding: base64

JVBERi0xLjQKJeLjz9MKMyAwIG9iajw8L1R5cGUvUGFnZS9QYXJlbnQgMiAwIFI+PmVuZG9iago=

--boundary123--
"""
        eml_file = tmp_path / "with_pdf.eml"
        eml_file.write_bytes(eml_content)

        parsed = parse_eml_file(eml_file)

        assert parsed.has_attachments is True
        assert parsed.attachment_count == 1
        assert parsed.attachments[0].filename == "report.pdf"
        assert parsed.attachments[0].mime_type == "application/pdf"
        assert parsed.attachments[0].extraction_status == AttachmentExtractionStatus.PENDING

    def test_extract_multiple_attachments(self, tmp_path):
        """Extract multiple attachments from email."""
        eml_content = b"""From: sender@example.com
To: recipient@example.com
Subject: Multiple attachments
MIME-Version: 1.0
Content-Type: multipart/mixed; boundary="boundary123"

--boundary123
Content-Type: text/plain

Email body.

--boundary123
Content-Type: application/pdf
Content-Disposition: attachment; filename="doc1.pdf"

PDF content 1

--boundary123
Content-Type: image/png
Content-Disposition: attachment; filename="image.png"

PNG content

--boundary123
Content-Type: text/csv
Content-Disposition: attachment; filename="data.csv"

CSV content

--boundary123--
"""
        eml_file = tmp_path / "multiple.eml"
        eml_file.write_bytes(eml_content)

        parsed = parse_eml_file(eml_file)

        assert parsed.has_attachments is True
        assert parsed.attachment_count == 3

        filenames = [att.filename for att in parsed.attachments]
        assert "doc1.pdf" in filenames
        assert "image.png" in filenames
        assert "data.csv" in filenames

    def test_inline_vs_attachment(self, tmp_path):
        """Distinguish inline images from attachments."""
        eml_content = b"""From: sender@example.com
To: recipient@example.com
Subject: Inline and attachment
MIME-Version: 1.0
Content-Type: multipart/mixed; boundary="boundary123"

--boundary123
Content-Type: text/plain

Body text.

--boundary123
Content-Type: image/png
Content-Disposition: inline; filename="inline.png"

Inline image (should NOT be extracted as attachment)

--boundary123
Content-Type: application/pdf
Content-Disposition: attachment; filename="attached.pdf"

Attached PDF

--boundary123--
"""
        eml_file = tmp_path / "inline_and_attached.eml"
        eml_file.write_bytes(eml_content)

        parsed = parse_eml_file(eml_file)

        # Only actual attachments, not inline
        assert parsed.attachment_count == 1
        assert parsed.attachments[0].filename == "attached.pdf"

    def test_email_without_attachments(self, tmp_path):
        """Email without attachments."""
        eml_content = b"""From: sender@example.com
To: recipient@example.com
Subject: Plain email

Just plain text, no attachments.
"""
        eml_file = tmp_path / "no_attach.eml"
        eml_file.write_bytes(eml_content)

        parsed = parse_eml_file(eml_file)

        assert parsed.has_attachments is False
        assert parsed.attachment_count == 0
        assert parsed.attachments == []


class TestOversizedAttachments:
    """Tests for handling oversized attachments (>25MB)."""

    def test_attachment_size_tracking(self):
        """AttachmentInfo tracks size correctly."""
        attachment = AttachmentInfo(
            filename="small.pdf",
            mime_type="application/pdf",
            size_bytes=1024,  # 1KB
        )
        assert attachment.size_bytes == 1024
        assert attachment.extraction_status == AttachmentExtractionStatus.PENDING

    def test_skipped_oversized_status(self):
        """Can mark attachment as skipped_oversized."""
        attachment = AttachmentInfo(
            filename="huge.zip",
            mime_type="application/zip",
            size_bytes=30 * 1024 * 1024,  # 30MB
            extraction_status=AttachmentExtractionStatus.SKIPPED_OVERSIZED,
            content=None,  # Content not stored for oversized
        )
        assert attachment.extraction_status == AttachmentExtractionStatus.SKIPPED_OVERSIZED
        assert attachment.content is None

    def test_max_attachment_size_constant(self):
        """Verify max attachment size is 25MB."""
        from penf_lib.ingest.parser import MAX_ATTACHMENT_SIZE

        assert MAX_ATTACHMENT_SIZE == 25 * 1024 * 1024


class TestAttachmentEventSchema:
    """Tests for ManualAttachmentExtractedEvent schema."""

    def test_attachment_event_fields(self):
        """Verify event has required fields."""
        from penf_lib.events.schemas import ManualAttachmentExtractedEvent
        from uuid import uuid4

        event = ManualAttachmentExtractedEvent(
            attachment_id=str(uuid4()),
            source_id=12345,
            tenant_id=str(uuid4()),
            filename="document.pdf",
            mime_type="application/pdf",
            size_bytes=1024000,
            extraction_status="completed",
        )

        assert event.event_type == "manual_attachment.extracted"
        assert event.filename == "document.pdf"
        assert event.mime_type == "application/pdf"
        assert event.extraction_status == "completed"

    def test_skipped_oversized_event(self):
        """Event for oversized attachment."""
        from penf_lib.events.schemas import ManualAttachmentExtractedEvent
        from uuid import uuid4

        event = ManualAttachmentExtractedEvent(
            attachment_id=str(uuid4()),
            source_id=12345,
            tenant_id=str(uuid4()),
            filename="large_video.mp4",
            mime_type="video/mp4",
            size_bytes=30 * 1024 * 1024,
            extraction_status="skipped_oversized",
        )

        assert event.extraction_status == "skipped_oversized"
        assert event.document_id is None
