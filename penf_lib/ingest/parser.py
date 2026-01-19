"""EML File Parser.

Parses .eml files using Python's standard library email module
for RFC 822/2822/5322 compliance.

Features:
- Header extraction (From, To, Cc, Bcc, Subject, Date, Message-ID)
- Body extraction (plain text and HTML)
- Synthetic Message-ID generation for non-conformant files (FR-103a)
- Date fallback handling for invalid dates (FR-104)
- Attachment extraction with size limits (FR-107, FR-107a)
- Thread reconstruction header parsing (FR-105)

Reference: specs/012-manual-ingest/research.md Section 1
"""

import hashlib
from datetime import datetime, timezone
from email import message_from_bytes, policy
from email.message import EmailMessage
from email.utils import parseaddr, parsedate_to_datetime
from pathlib import Path
from typing import Optional

from .models import AttachmentExtractionStatus, AttachmentInfo, ParsedEmail

# Maximum attachment size in bytes (25MB per FR-107a)
MAX_ATTACHMENT_SIZE = 25 * 1024 * 1024


class ParseError(Exception):
    """Error during email parsing."""

    pass


def parse_eml_file(file_path: Path) -> ParsedEmail:
    """Parse .eml file and extract all metadata and content.

    Args:
        file_path: Path to .eml file

    Returns:
        ParsedEmail with all extracted data

    Raises:
        ParseError: If file cannot be parsed
        FileNotFoundError: If file does not exist
        IOError: If file cannot be read
    """
    with open(file_path, "rb") as f:
        raw_content = f.read()

    parsed = parse_eml_bytes(raw_content)
    parsed.original_file_path = str(file_path)
    return parsed


def parse_eml_bytes(raw_content: bytes) -> ParsedEmail:
    """Parse .eml content from bytes.

    Args:
        raw_content: Raw .eml file bytes

    Returns:
        ParsedEmail with all extracted data

    Raises:
        ParseError: If content cannot be parsed
    """
    if not raw_content:
        raise ParseError("Empty file content")

    try:
        # Use email.policy.default for modern email parsing
        msg = message_from_bytes(raw_content, policy=policy.default)
    except Exception as e:
        raise ParseError(f"Failed to parse email: {e}") from e

    # Generate content hash
    content_hash = hashlib.sha256(raw_content).hexdigest()

    # Extract Message-ID or generate synthetic
    message_id = msg.get("Message-ID", "").strip()
    message_id_synthetic = False
    if not message_id:
        # Generate synthetic Message-ID from content hash (FR-103a)
        message_id = f"<synthetic-{content_hash[:16]}@penfold.local>"
        message_id_synthetic = True

    # Parse sender
    from_header = msg.get("From", "")
    from_name, from_email = parseaddr(from_header)
    if not from_email:
        raise ParseError("Missing From header")

    # Parse recipients
    to_emails = _extract_addresses(msg.get_all("To", []))
    cc_emails = _extract_addresses(msg.get_all("Cc", []))
    bcc_emails = _extract_addresses(msg.get_all("Bcc", []))

    # Parse date with fallback (FR-104)
    email_date, date_fallback_used, original_date_raw = _parse_date(msg)

    # Extract body content
    body_plain, body_html = _extract_body(msg)

    # Extract attachments
    attachments = _extract_attachments(msg)

    return ParsedEmail(
        message_id=message_id,
        message_id_synthetic=message_id_synthetic,
        from_email=from_email.lower(),
        from_name=from_name or None,
        to_emails=[e.lower() for e in to_emails],
        cc_emails=[e.lower() for e in cc_emails],
        bcc_emails=[e.lower() for e in bcc_emails],
        subject=msg.get("Subject"),
        body_plain=body_plain,
        body_html=body_html,
        email_date=email_date,
        date_fallback_used=date_fallback_used,
        original_date_raw=original_date_raw,
        in_reply_to=msg.get("In-Reply-To"),
        references=_parse_references(msg.get("References", "")),
        attachments=attachments,
        raw_content=raw_content,
        content_hash=content_hash,
    )


def _extract_addresses(headers: list) -> list[str]:
    """Extract email addresses from header values.

    Args:
        headers: List of header values (may contain multiple addresses)

    Returns:
        List of email addresses
    """
    addresses = []
    for header in headers:
        if isinstance(header, str):
            # Handle comma-separated addresses
            for part in header.split(","):
                _, addr = parseaddr(part.strip())
                if addr and "@" in addr:
                    addresses.append(addr)
    return addresses


def _parse_date(msg: EmailMessage) -> tuple[Optional[datetime], bool, Optional[str]]:
    """Parse email date with fallback handling.

    Per FR-104: Use original Date header for timeline positioning.
    For unparseable or future dates, use ingestion timestamp as fallback.

    Args:
        msg: Parsed email message

    Returns:
        Tuple of (parsed_date, fallback_used, original_raw_string)
    """
    date_str = msg.get("Date")
    if not date_str:
        return None, True, None

    try:
        email_date = parsedate_to_datetime(date_str)
        # Check for future date (FR-104)
        if email_date > datetime.now(timezone.utc):
            return None, True, date_str
        return email_date, False, date_str
    except (ValueError, TypeError):
        return None, True, date_str


def _extract_body(msg: EmailMessage) -> tuple[Optional[str], Optional[str]]:
    """Extract plain text and HTML body content.

    Args:
        msg: Parsed email message

    Returns:
        Tuple of (plain_text, html)
    """
    body_plain = None
    body_html = None

    if msg.is_multipart():
        for part in msg.walk():
            content_type = part.get_content_type()
            content_disposition = part.get_content_disposition()

            # Skip attachments
            if content_disposition == "attachment":
                continue

            try:
                if content_type == "text/plain" and not body_plain:
                    body_plain = part.get_content()
                elif content_type == "text/html" and not body_html:
                    body_html = part.get_content()
            except Exception:
                # Handle encoding errors gracefully
                continue
    else:
        content_type = msg.get_content_type()
        try:
            content = msg.get_content()
            if content_type == "text/html":
                body_html = content
            else:
                body_plain = content
        except Exception:
            pass

    return body_plain, body_html


def _extract_attachments(
    msg: EmailMessage, max_size: int = MAX_ATTACHMENT_SIZE
) -> list[AttachmentInfo]:
    """Extract attachment information from email.

    Per FR-107a: Skip extraction for attachments >25MB.

    Args:
        msg: Parsed email message
        max_size: Maximum attachment size in bytes

    Returns:
        List of AttachmentInfo objects
    """
    attachments = []

    if not msg.is_multipart():
        return attachments

    for part in msg.walk():
        content_disposition = part.get_content_disposition()
        if content_disposition != "attachment":
            continue

        filename = part.get_filename()
        if not filename:
            continue

        try:
            content = part.get_payload(decode=True)
            if content is None:
                continue

            size_bytes = len(content)
            mime_type = part.get_content_type()

            if size_bytes > max_size:
                attachments.append(
                    AttachmentInfo(
                        filename=filename,
                        mime_type=mime_type,
                        size_bytes=size_bytes,
                        extraction_status=AttachmentExtractionStatus.SKIPPED_OVERSIZED,
                        content=None,
                    )
                )
            else:
                attachments.append(
                    AttachmentInfo(
                        filename=filename,
                        mime_type=mime_type,
                        size_bytes=size_bytes,
                        extraction_status=AttachmentExtractionStatus.PENDING,
                        content=content,
                    )
                )
        except Exception:
            # Handle extraction errors gracefully
            continue

    return attachments


def _parse_references(references_str: str) -> list[str]:
    """Parse References header into list of Message-IDs.

    Args:
        references_str: References header value

    Returns:
        List of Message-ID values
    """
    if not references_str:
        return []

    # References are whitespace-separated Message-IDs
    return [ref.strip() for ref in references_str.split() if ref.strip()]
