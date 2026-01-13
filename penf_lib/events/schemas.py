"""Event schema definitions for the Penfold system."""

from typing import Optional, List, Dict, Any
from datetime import datetime
from pydantic import BaseModel, Field


class BaseEvent(BaseModel):
    """Base event class with common fields."""

    event_type: str
    timestamp: datetime = Field(default_factory=datetime.utcnow)
    correlation_id: Optional[str] = None
    source: str = "penfold"
    version: str = "1.0"


class EmailIngestedEvent(BaseEvent):
    """Event published when an email is ingested and ready for processing."""

    event_type: str = "content.ingested"

    # Email identification
    gmail_message_id: str = Field(description="Gmail message ID")
    gmail_thread_id: str = Field(description="Gmail thread ID")
    connection_id: int = Field(description="Gmail connection ID")

    # Email metadata
    subject: Optional[str] = Field(default=None, description="Email subject")
    from_email: str = Field(description="Sender email address")
    from_name: Optional[str] = Field(default=None, description="Sender display name")
    to_emails: List[str] = Field(description="Recipient email addresses")
    cc_emails: Optional[List[str]] = Field(default=None, description="CC recipients")
    bcc_emails: Optional[List[str]] = Field(default=None, description="BCC recipients")

    # Content information
    has_body_text: bool = Field(description="Whether email has plain text body")
    has_body_html: bool = Field(description="Whether email has HTML body")
    has_attachments: bool = Field(description="Whether email has attachments")
    attachment_count: int = Field(default=0, description="Number of attachments")

    # Gmail metadata
    internal_date: datetime = Field(description="Gmail internal date")
    gmail_labels: Optional[List[str]] = Field(default=None, description="Gmail labels")
    is_unread: bool = Field(default=True, description="Whether email is unread")
    size_estimate: Optional[int] = Field(default=None, description="Estimated size in bytes")

    # Thread information
    thread_message_count: int = Field(description="Total messages in thread")
    is_first_message: bool = Field(description="Whether this is first message in thread")

    # Processing context
    content_priority: str = Field(default="normal", description="Processing priority: high, normal, low")
    requires_ai_processing: bool = Field(default=True, description="Whether AI processing is needed")

    class Config:
        """Pydantic configuration."""
        json_encoders = {
            datetime: lambda v: v.isoformat()
        }


class EmailThreadIngestedEvent(BaseEvent):
    """Event published when an entire email thread is ingested."""

    event_type: str = "content.thread_ingested"

    # Thread identification
    gmail_thread_id: str = Field(description="Gmail thread ID")
    connection_id: int = Field(description="Gmail connection ID")

    # Thread metadata
    subject: Optional[str] = Field(description="Thread subject")
    participant_emails: List[str] = Field(description="All participants in thread")
    message_count: int = Field(description="Number of messages in thread")
    unread_count: int = Field(description="Number of unread messages")

    # Temporal information
    first_message_date: datetime = Field(description="Date of first message")
    last_message_date: datetime = Field(description="Date of last message")
    thread_duration_days: Optional[int] = Field(description="Thread duration in days")

    # Content summary
    total_attachments: int = Field(default=0, description="Total attachments in thread")
    message_ids: List[str] = Field(description="List of Gmail message IDs in thread")

    # Processing context
    requires_relationship_analysis: bool = Field(default=True, description="Whether relationship analysis is needed")
    thread_priority: str = Field(default="normal", description="Thread processing priority")

    class Config:
        """Pydantic configuration."""
        json_encoders = {
            datetime: lambda v: v.isoformat()
        }


class EmailAttachmentIngestedEvent(BaseEvent):
    """Event published when an email attachment is downloaded and ready for processing."""

    event_type: str = "content.attachment_ingested"

    # Attachment identification
    gmail_attachment_id: str = Field(description="Gmail attachment ID")
    gmail_message_id: str = Field(description="Parent message ID")
    gmail_thread_id: str = Field(description="Parent thread ID")
    connection_id: int = Field(description="Gmail connection ID")

    # File information
    filename: str = Field(description="Original filename")
    mime_type: str = Field(description="MIME type")
    size_bytes: int = Field(description="File size in bytes")
    file_path: str = Field(description="Local file path after download")

    # Processing hints
    is_text_extractable: bool = Field(description="Whether text extraction is possible")
    requires_ocr: bool = Field(default=False, description="Whether OCR is needed")
    content_type_hint: Optional[str] = Field(description="Hint about content type: document, image, spreadsheet, etc.")

    # Security information
    is_potentially_malicious: bool = Field(default=False, description="Security scan result")
    scan_status: str = Field(default="pending", description="Security scan status")

    class Config:
        """Pydantic configuration."""
        json_encoders = {
            datetime: lambda v: v.isoformat()
        }


class SyncProgressEvent(BaseEvent):
    """Event published to track sync progress."""

    event_type: str = "sync.progress"

    # Sync identification
    sync_session_id: str = Field(description="Unique sync session ID")
    connection_id: int = Field(description="Gmail connection ID")
    sync_type: str = Field(description="Sync type: historical, incremental, real_time")

    # Progress information
    total_items: Optional[int] = Field(description="Total items to process")
    processed_items: int = Field(description="Items processed so far")
    failed_items: int = Field(default=0, description="Items that failed processing")
    current_batch: int = Field(description="Current batch number")

    # Performance metrics
    items_per_minute: Optional[float] = Field(description="Processing rate")
    estimated_completion: Optional[datetime] = Field(description="Estimated completion time")

    # Status information
    status: str = Field(description="Sync status: starting, in_progress, completed, failed, paused")
    current_operation: str = Field(description="Current operation description")
    last_error: Optional[str] = Field(description="Last error message if any")

    class Config:
        """Pydantic configuration."""
        json_encoders = {
            datetime: lambda v: v.isoformat()
        }


class SyncCompletedEvent(BaseEvent):
    """Event published when a sync operation completes."""

    event_type: str = "sync.completed"

    # Sync identification
    sync_session_id: str = Field(description="Unique sync session ID")
    connection_id: int = Field(description="Gmail connection ID")
    sync_type: str = Field(description="Sync type")

    # Summary statistics
    total_messages: int = Field(description="Total messages processed")
    total_threads: int = Field(description="Total threads processed")
    total_attachments: int = Field(description="Total attachments downloaded")
    failed_messages: int = Field(default=0, description="Messages that failed")

    # Performance metrics
    duration_seconds: float = Field(description="Total sync duration")
    average_items_per_minute: float = Field(description="Average processing rate")

    # Result information
    success: bool = Field(description="Whether sync completed successfully")
    error_summary: Optional[str] = Field(description="Summary of errors if any")
    next_sync_token: Optional[str] = Field(description="Token for next incremental sync")

    class Config:
        """Pydantic configuration."""
        json_encoders = {
            datetime: lambda v: v.isoformat()
        }