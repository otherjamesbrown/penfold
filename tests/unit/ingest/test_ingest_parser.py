"""Unit tests for EML parser.

Tests parsing of .eml files including:
- Valid email extraction
- Synthetic Message-ID generation
- Date fallback handling
- Attachment extraction
- Malformed file handling

Reference: specs/012-manual-ingest/quickstart.md
"""

from pathlib import Path

import pytest

# Import directly from modules to avoid dependency issues during testing
from penf_lib.ingest.parser import parse_eml_file, parse_eml_bytes, ParseError
from penf_lib.ingest.models import AttachmentExtractionStatus

# Path to test fixtures
FIXTURES_DIR = Path(__file__).parent.parent.parent / "fixtures" / "ingest"


class TestParseValidEmail:
    """Tests for parsing valid .eml files."""

    def test_parse_valid_simple_email(self):
        """Parse a simple valid email with all headers."""
        eml_path = FIXTURES_DIR / "valid_simple.eml"
        result = parse_eml_file(eml_path)

        assert result.message_id == "<test-valid-simple-001@example.com>"
        assert result.message_id_synthetic is False
        assert result.from_email == "bob@example.com"
        assert result.from_name == "Bob Smith"
        assert "alice@example.com" in result.to_emails
        assert result.subject == "Project Update - Q1 Planning"
        assert result.body_plain is not None
        assert "Q1 planning" in result.body_plain

    def test_parse_email_with_attachment(self):
        """Parse email with PDF attachment."""
        eml_path = FIXTURES_DIR / "valid_with_attachment.eml"
        result = parse_eml_file(eml_path)

        assert result.message_id == "<test-with-attachment-001@example.com>"
        assert result.has_attachments is True
        assert result.attachment_count == 1

        attachment = result.attachments[0]
        assert attachment.filename == "q1_budget_2026.pdf"
        assert attachment.mime_type == "application/pdf"
        assert attachment.extraction_status == AttachmentExtractionStatus.PENDING

    def test_parse_thread_reply(self):
        """Parse email that is a reply in a thread."""
        eml_path = FIXTURES_DIR / "valid_thread_reply.eml"
        result = parse_eml_file(eml_path)

        assert result.message_id == "<test-thread-reply-001@example.com>"
        assert result.in_reply_to == "<test-valid-simple-001@example.com>"
        assert "<test-valid-simple-001@example.com>" in result.references
        assert result.subject == "Re: Project Update - Q1 Planning"


class TestSyntheticMessageId:
    """Tests for synthetic Message-ID generation."""

    def test_generate_synthetic_message_id(self):
        """Generate synthetic Message-ID for email without one."""
        eml_path = FIXTURES_DIR / "no_message_id.eml"
        result = parse_eml_file(eml_path)

        assert result.message_id.startswith("<synthetic-")
        assert result.message_id.endswith("@penfold.local>")
        assert result.message_id_synthetic is True

    def test_synthetic_id_is_deterministic(self):
        """Same content should generate same synthetic ID."""
        eml_path = FIXTURES_DIR / "no_message_id.eml"
        result1 = parse_eml_file(eml_path)
        result2 = parse_eml_file(eml_path)

        assert result1.message_id == result2.message_id
        assert result1.content_hash == result2.content_hash


class TestDateHandling:
    """Tests for date parsing with fallback."""

    def test_future_date_triggers_fallback(self):
        """Future dates should trigger fallback to None."""
        eml_path = FIXTURES_DIR / "future_date.eml"
        result = parse_eml_file(eml_path)

        # Future date should be rejected
        assert result.email_date is None
        assert result.date_fallback_used is True
        # Original date string should be preserved
        assert result.original_date_raw is not None
        assert "2099" in result.original_date_raw


class TestMalformedHandling:
    """Tests for handling malformed .eml files."""

    def test_malformed_file_raises_parse_error(self):
        """Malformed file should raise ParseError."""
        eml_path = FIXTURES_DIR / "malformed.eml"

        with pytest.raises(ParseError):
            parse_eml_file(eml_path)

    def test_empty_content_raises_parse_error(self):
        """Empty content should raise ParseError."""
        with pytest.raises(ParseError):
            parse_eml_bytes(b"")


class TestContentHash:
    """Tests for content hash generation."""

    def test_content_hash_is_sha256(self):
        """Content hash should be 64-character SHA-256 hex string."""
        eml_path = FIXTURES_DIR / "valid_simple.eml"
        result = parse_eml_file(eml_path)

        assert len(result.content_hash) == 64
        # Should be valid hex
        int(result.content_hash, 16)

    def test_same_file_same_hash(self):
        """Same file should produce same hash."""
        eml_path = FIXTURES_DIR / "valid_simple.eml"
        result1 = parse_eml_file(eml_path)
        result2 = parse_eml_file(eml_path)

        assert result1.content_hash == result2.content_hash


class TestEmailAddressNormalization:
    """Tests for email address handling."""

    def test_email_addresses_lowercase(self):
        """Email addresses should be normalized to lowercase."""
        eml_path = FIXTURES_DIR / "valid_simple.eml"
        result = parse_eml_file(eml_path)

        # All email addresses should be lowercase
        assert result.from_email == result.from_email.lower()
        for email in result.to_emails:
            assert email == email.lower()
