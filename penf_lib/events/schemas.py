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
    message_id_header: Optional[str] = Field(default=None, description="Email Message-ID header")

    # Enhanced participant information
    participants: Dict[str, Any] = Field(description="Normalized participant information")
    unique_participant_count: int = Field(description="Number of unique participants")
    has_external_participants: bool = Field(default=False, description="Whether non-organizational participants exist")

    # Enhanced subject and threading
    subject: Optional[str] = Field(default=None, description="Email subject")
    normalized_subject: Optional[str] = Field(default=None, description="Normalized subject for threading")
    subject_hash: str = Field(description="Hash for subject-based threading")

    # Legacy participant fields (for backward compatibility)
    from_email: str = Field(description="Sender email address")
    from_name: Optional[str] = Field(default=None, description="Sender display name")
    to_emails: List[str] = Field(description="Recipient email addresses")
    cc_emails: Optional[List[str]] = Field(default=None, description="CC recipients")
    bcc_emails: Optional[List[str]] = Field(default=None, description="BCC recipients")

    # Thread relationship information
    thread_info: Dict[str, Any] = Field(description="Thread relationship metadata")
    is_root_message: bool = Field(description="Whether this is the root message in thread")
    reply_depth: int = Field(default=0, description="Depth of reply in conversation")
    parent_message_id: Optional[str] = Field(default=None, description="Parent message ID")
    references: List[str] = Field(default_factory=list, description="Message references for threading")

    # Enhanced content information
    content_structure: Dict[str, Any] = Field(description="Content structure metadata")
    message_format: str = Field(description="Message format: plain, html, multipart, rich")
    has_body_text: bool = Field(description="Whether email has plain text body")
    has_body_html: bool = Field(description="Whether email has HTML body")
    has_attachments: bool = Field(description="Whether email has attachments")
    attachment_count: int = Field(default=0, description="Number of attachments")
    content_types: List[str] = Field(default_factory=list, description="MIME content types present")

    # Enhanced priority and importance
    priority_info: Dict[str, Any] = Field(description="Priority and importance metadata")
    importance_level: str = Field(default="normal", description="Extracted importance: high, normal, low")
    priority_score: float = Field(default=0.5, description="Priority score 0-1")
    urgency_indicators: List[str] = Field(default_factory=list, description="Detected urgency indicators")
    is_automated: bool = Field(default=False, description="Whether message is automated")

    # Enhanced Gmail metadata
    internal_date: datetime = Field(description="Gmail internal date")
    sent_date: Optional[datetime] = Field(default=None, description="Date from email headers")
    received_date: Optional[datetime] = Field(default=None, description="Actual receipt date")
    gmail_labels: List[str] = Field(default_factory=list, description="Gmail labels")
    is_unread: bool = Field(default=True, description="Whether email is unread")
    is_starred: bool = Field(default=False, description="Whether email is starred")
    is_important: bool = Field(default=False, description="Whether Gmail marked as important")
    size_estimate: int = Field(default=0, description="Estimated size in bytes")

    # Security and authenticity
    authentication_results: Dict[str, Any] = Field(default_factory=dict, description="Email authentication results")
    spam_score: Optional[float] = Field(default=None, description="Spam score 0-1 if determinable")

    # Thread information
    thread_message_count: int = Field(description="Total messages in thread")
    is_first_message: bool = Field(description="Whether this is first message in thread")

    # Processing context
    content_priority: str = Field(default="normal", description="Processing priority: high, normal, low")
    requires_ai_processing: bool = Field(default=True, description="Whether AI processing is needed")
    entity_resolution_hints: Dict[str, Any] = Field(default_factory=dict, description="Hints for entity resolution")

    # Additional metadata
    custom_headers: Dict[str, str] = Field(default_factory=dict, description="Non-standard headers")
    delivery_info: Dict[str, Any] = Field(default_factory=dict, description="Message delivery information")

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


class ContentExtractedEvent(BaseEvent):
    """Event published when content is extracted from an attachment."""

    event_type: str = "content.extracted"

    # Source identification
    source_id: str = Field(description="Source identifier (e.g., attachment:123)")
    source_type: str = Field(description="Source type (e.g., email_attachment)")

    # Content information
    content_type: str = Field(description="Type of content extracted")
    extracted_text: str = Field(description="Extracted text content")

    # Extraction metadata
    metadata: Dict[str, Any] = Field(default_factory=dict, description="Extraction metadata")

    # Processing information
    extraction_method: str = Field(default="unknown", description="Method used for extraction")
    confidence: float = Field(default=0.0, description="Confidence score 0-1")
    word_count: int = Field(default=0, description="Word count of extracted content")
    language: Optional[str] = Field(default=None, description="Detected language")

    class Config:
        """Pydantic configuration."""
        json_encoders = {
            datetime: lambda v: v.isoformat()
        }


# =============================================================================
# RELATIONSHIP DISCOVERY EVENTS
# =============================================================================


class RelationshipDiscoveredEvent(BaseEvent):
    """Event published when a new relationship is discovered.

    This event is triggered when the AI extraction pipeline identifies
    a potential relationship between entities from content analysis.
    """

    event_type: str = "relationship.discovered"

    # Relationship identification
    relationship_id: int = Field(description="Unique relationship identifier")
    tenant_id: str = Field(description="Tenant ID for isolation")

    # Entity references
    source_entity_type: str = Field(description="Type of source entity (person, project, topic)")
    source_entity_id: int = Field(description="ID of the source entity")
    target_entity_type: str = Field(description="Type of target entity")
    target_entity_id: int = Field(description="ID of the target entity")

    # Relationship classification
    relationship_type: str = Field(description="Type of relationship discovered")
    relationship_subtype: Optional[str] = Field(default=None, description="Optional subtype")

    # Confidence and evidence
    confidence_score: float = Field(ge=0, le=1, description="Overall confidence (0.0-1.0)")
    ai_confidence: Optional[float] = Field(default=None, ge=0, le=1, description="AI extraction confidence")
    evidence_source_id: Optional[int] = Field(default=None, description="Source content that triggered discovery")
    evidence_snippet: Optional[str] = Field(default=None, description="Text snippet supporting relationship")

    # Discovery metadata
    discovery_method: str = Field(default="ai_extraction", description="How relationship was discovered")
    requires_validation: bool = Field(default=True, description="Whether user validation is recommended")

    class Config:
        """Pydantic configuration."""
        json_encoders = {
            datetime: lambda v: v.isoformat()
        }


class RelationshipUpdatedEvent(BaseEvent):
    """Event published when an existing relationship is updated.

    This event is triggered when relationship properties change,
    such as confidence score updates, lifecycle state transitions,
    or user modifications.
    """

    event_type: str = "relationship.updated"

    # Relationship identification
    relationship_id: int = Field(description="Unique relationship identifier")
    tenant_id: str = Field(description="Tenant ID for isolation")

    # Update details
    update_type: str = Field(description="Type of update: confidence, lifecycle, user_feedback, evidence")

    # Previous and new values
    previous_confidence: Optional[float] = Field(default=None, description="Previous confidence score")
    new_confidence: Optional[float] = Field(default=None, description="New confidence score")
    previous_state: Optional[str] = Field(default=None, description="Previous lifecycle state")
    new_state: Optional[str] = Field(default=None, description="New lifecycle state")

    # Attribution
    updated_by: str = Field(default="system", description="Who triggered the update (user email or 'system')")
    update_reason: Optional[str] = Field(default=None, description="Explanation for update")

    # Related entities for downstream processing
    source_entity_type: str = Field(description="Type of source entity")
    source_entity_id: int = Field(description="ID of the source entity")
    target_entity_type: str = Field(description="Type of target entity")
    target_entity_id: int = Field(description="ID of the target entity")

    class Config:
        """Pydantic configuration."""
        json_encoders = {
            datetime: lambda v: v.isoformat()
        }


class RelationshipConflictEvent(BaseEvent):
    """Event published when a relationship conflict is detected.

    This event is triggered when incompatible relationships are
    discovered, requiring resolution either automatically or by user.
    """

    event_type: str = "relationship.conflict"

    # Conflict identification
    conflict_id: int = Field(description="Unique conflict identifier")
    tenant_id: str = Field(description="Tenant ID for isolation")

    # Conflicting relationships
    primary_relationship_id: int = Field(description="Primary relationship in conflict")
    conflicting_relationship_id: Optional[int] = Field(default=None, description="Competing relationship")

    # Conflict details
    conflict_type: str = Field(description="Type: type_mismatch, duplicate, contradictory, circular")
    primary_confidence: float = Field(ge=0, le=1, description="Primary relationship confidence")
    secondary_confidence: Optional[float] = Field(default=None, ge=0, le=1, description="Conflicting relationship confidence")
    confidence_gap: float = Field(ge=0, le=1, description="Absolute difference in confidence")

    # Resolution guidance
    auto_resolvable: bool = Field(description="Whether gap > 30% allows auto-resolution")
    recommended_action: str = Field(description="Suggested resolution: auto_resolve, user_review, defer")

    class Config:
        """Pydantic configuration."""
        json_encoders = {
            datetime: lambda v: v.isoformat()
        }


class RelationshipValidatedEvent(BaseEvent):
    """Event published when a user validates a relationship.

    This event is triggered when a user provides feedback to
    confirm, reject, or modify a discovered relationship.
    """

    event_type: str = "relationship.validated"

    # Relationship identification
    relationship_id: int = Field(description="Unique relationship identifier")
    tenant_id: str = Field(description="Tenant ID for isolation")

    # Validation details
    feedback_type: str = Field(description="Type: confirm, reject, modify")
    user_email: str = Field(description="User who provided validation")

    # Changes made
    original_type: Optional[str] = Field(default=None, description="Original relationship type")
    corrected_type: Optional[str] = Field(default=None, description="User-corrected type if modified")
    original_confidence: Optional[float] = Field(default=None, description="Original confidence score")
    new_confidence: Optional[float] = Field(default=None, description="New confidence after validation")

    # Resulting state
    resulting_state: str = Field(description="Lifecycle state after validation")
    user_reasoning: Optional[str] = Field(default=None, description="User's explanation")

    class Config:
        """Pydantic configuration."""
        json_encoders = {
            datetime: lambda v: v.isoformat()
        }


class RelationshipArchivedEvent(BaseEvent):
    """Event published when a relationship is archived.

    This event is triggered when a relationship transitions to
    archived state due to user action or inactivity.
    """

    event_type: str = "relationship.archived"

    # Relationship identification
    relationship_id: int = Field(description="Unique relationship identifier")
    tenant_id: str = Field(description="Tenant ID for isolation")

    # Archive details
    archive_reason: str = Field(description="Reason: user_rejected, inactivity, project_completed, conflict_resolution")
    archived_by: str = Field(default="system", description="Who triggered archival")

    # Final state
    final_confidence: float = Field(ge=0, le=1, description="Confidence at time of archival")
    total_evidence_count: int = Field(default=0, description="Total evidence items collected")
    total_interactions: int = Field(default=0, description="Total interactions observed")
    lifetime_days: int = Field(description="Days from first observation to archival")

    # Related entities
    source_entity_type: str = Field(description="Type of source entity")
    source_entity_id: int = Field(description="ID of the source entity")
    target_entity_type: str = Field(description="Type of target entity")
    target_entity_id: int = Field(description="ID of the target entity")

    class Config:
        """Pydantic configuration."""
        json_encoders = {
            datetime: lambda v: v.isoformat()
        }