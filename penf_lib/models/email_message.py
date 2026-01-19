"""Email message and thread models."""

from datetime import datetime
from typing import Optional, List
from sqlalchemy import String, DateTime, Text, Integer, Boolean, ForeignKey, JSON
from sqlalchemy.orm import Mapped, mapped_column, relationship

from ..storage.database import Base


class EmailThread(Base):
    """Gmail conversation thread."""

    __tablename__ = 'email_threads'

    id: Mapped[int] = mapped_column(primary_key=True)
    gmail_thread_id: Mapped[str] = mapped_column(String(50), unique=True, index=True)
    connection_id: Mapped[int] = mapped_column(ForeignKey('gmail_connections.id'))

    # Thread metadata
    subject: Mapped[Optional[str]] = mapped_column(Text)
    participant_emails: Mapped[List[str]] = mapped_column(JSON)  # Array of participant emails
    label_ids: Mapped[Optional[List[str]]] = mapped_column(JSON)

    # Thread statistics
    message_count: Mapped[int] = mapped_column(Integer, default=0)
    unread_count: Mapped[int] = mapped_column(Integer, default=0)

    # Temporal information
    first_message_date: Mapped[Optional[datetime]] = mapped_column(DateTime(timezone=True))
    last_message_date: Mapped[Optional[datetime]] = mapped_column(DateTime(timezone=True))

    # Processing status
    processing_status: Mapped[str] = mapped_column(String(50), default='pending')  # pending, processing, completed, failed
    last_processed_at: Mapped[Optional[datetime]] = mapped_column(DateTime(timezone=True))

    # Audit fields
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True),
        default=datetime.utcnow
    )
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True),
        default=datetime.utcnow,
        onupdate=datetime.utcnow
    )

    # Relationships
    messages: Mapped[List["EmailMessage"]] = relationship(
        "EmailMessage",
        back_populates="thread",
        cascade="all, delete-orphan"
    )

    def __repr__(self) -> str:
        return f"<EmailThread(id='{self.gmail_thread_id}', messages={self.message_count})>"


class EmailMessage(Base):
    """Individual email message within a thread."""

    __tablename__ = 'email_messages'

    id: Mapped[int] = mapped_column(primary_key=True)
    gmail_message_id: Mapped[str] = mapped_column(String(50), unique=True, index=True)
    thread_id: Mapped[int] = mapped_column(ForeignKey('email_threads.id'))

    # Message headers and content
    subject: Mapped[Optional[str]] = mapped_column(Text)
    from_email: Mapped[str] = mapped_column(String(255), index=True)
    from_name: Mapped[Optional[str]] = mapped_column(String(255))
    to_emails: Mapped[List[str]] = mapped_column(JSON)
    cc_emails: Mapped[Optional[List[str]]] = mapped_column(JSON)
    bcc_emails: Mapped[Optional[List[str]]] = mapped_column(JSON)

    # Message content
    body_text: Mapped[Optional[str]] = mapped_column(Text)
    body_html: Mapped[Optional[str]] = mapped_column(Text)
    snippet: Mapped[Optional[str]] = mapped_column(Text)

    # Gmail metadata
    gmail_labels: Mapped[Optional[List[str]]] = mapped_column(JSON)
    internal_date: Mapped[datetime] = mapped_column(DateTime(timezone=True))
    is_unread: Mapped[bool] = mapped_column(Boolean, default=True)
    size_estimate: Mapped[Optional[int]] = mapped_column(Integer)

    # Attachment information
    has_attachments: Mapped[bool] = mapped_column(Boolean, default=False)
    attachment_count: Mapped[int] = mapped_column(Integer, default=0)

    # Processing status
    processing_status: Mapped[str] = mapped_column(String(50), default='pending')
    event_published_at: Mapped[Optional[datetime]] = mapped_column(DateTime(timezone=True))
    extraction_completed_at: Mapped[Optional[datetime]] = mapped_column(DateTime(timezone=True))

    # Audit fields
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True),
        default=datetime.utcnow
    )
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True),
        default=datetime.utcnow,
        onupdate=datetime.utcnow
    )

    # Relationships
    thread: Mapped["EmailThread"] = relationship("EmailThread", back_populates="messages")
    attachments: Mapped[List["EmailAttachment"]] = relationship(
        "EmailAttachment",
        back_populates="message",
        cascade="all, delete-orphan"
    )

    def __repr__(self) -> str:
        return f"<EmailMessage(id='{self.gmail_message_id}', from='{self.from_email}')>"


class EmailAttachment(Base):
    """Email attachment metadata and content reference."""

    __tablename__ = 'email_attachments'

    id: Mapped[int] = mapped_column(primary_key=True)
    message_id: Mapped[int] = mapped_column(ForeignKey('email_messages.id'))
    gmail_attachment_id: Mapped[str] = mapped_column(String(255), index=True)

    # Attachment metadata
    filename: Mapped[str] = mapped_column(String(255))
    mime_type: Mapped[str] = mapped_column(String(100))
    size_bytes: Mapped[int] = mapped_column(Integer)

    # Download and processing status
    download_status: Mapped[str] = mapped_column(String(50), default='pending')  # pending, downloading, completed, failed
    file_path: Mapped[Optional[str]] = mapped_column(Text)  # Local file path after download
    content_extracted: Mapped[bool] = mapped_column(Boolean, default=False)
    extraction_status: Mapped[Optional[str]] = mapped_column(String(50))  # pending, processing, completed, failed, unsupported

    # Processing timestamps
    downloaded_at: Mapped[Optional[datetime]] = mapped_column(DateTime(timezone=True))
    processed_at: Mapped[Optional[datetime]] = mapped_column(DateTime(timezone=True))

    # Audit fields
    created_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True),
        default=datetime.utcnow
    )
    updated_at: Mapped[datetime] = mapped_column(
        DateTime(timezone=True),
        default=datetime.utcnow,
        onupdate=datetime.utcnow
    )

    # Relationships
    message: Mapped["EmailMessage"] = relationship("EmailMessage", back_populates="attachments")

    def __repr__(self) -> str:
        return f"<EmailAttachment(filename='{self.filename}', size={self.size_bytes})>"