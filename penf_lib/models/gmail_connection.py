"""Gmail connection and authentication models."""

from datetime import datetime
from typing import Optional
from sqlalchemy import String, DateTime, LargeBinary, Boolean, Text
from sqlalchemy.orm import Mapped, mapped_column, relationship

from ..storage.database import Base


class GmailConnection(Base):
    """Gmail account connection with encrypted OAuth2 credentials."""

    __tablename__ = 'gmail_connections'

    id: Mapped[int] = mapped_column(primary_key=True)
    account_email: Mapped[str] = mapped_column(String(255), unique=True, index=True)
    account_name: Mapped[Optional[str]] = mapped_column(String(255))

    # Encrypted OAuth2 credentials
    encrypted_credentials: Mapped[bytes] = mapped_column(LargeBinary)

    # Connection status
    is_active: Mapped[bool] = mapped_column(Boolean, default=True)
    last_successful_auth: Mapped[Optional[datetime]] = mapped_column(DateTime(timezone=True))
    last_sync: Mapped[Optional[datetime]] = mapped_column(DateTime(timezone=True))

    # Gmail-specific IDs for sync tracking
    gmail_profile_id: Mapped[Optional[str]] = mapped_column(String(255))
    current_history_id: Mapped[Optional[str]] = mapped_column(String(50))

    # Push notification configuration
    push_topic_name: Mapped[Optional[str]] = mapped_column(String(255))
    push_subscription_expiry: Mapped[Optional[datetime]] = mapped_column(DateTime(timezone=True))

    # Privacy and filtering settings
    privacy_filters: Mapped[Optional[str]] = mapped_column(Text)  # JSON string
    sync_labels: Mapped[Optional[str]] = mapped_column(Text)  # JSON string of label IDs

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

    def __repr__(self) -> str:
        return f"<GmailConnection(email='{self.account_email}', active={self.is_active})>"