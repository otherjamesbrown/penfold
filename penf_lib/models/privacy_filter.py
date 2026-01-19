"""Privacy filter models for email processing controls."""

from datetime import datetime
from typing import Optional, List, Dict, Any
from enum import Enum

from sqlalchemy import Column, Integer, String, Boolean, DateTime, Text, JSON, Index
from sqlalchemy.ext.declarative import declarative_base
from sqlalchemy.orm import validates

from ..storage.database import Base


class FilterType(str, Enum):
    """Privacy filter types."""

    LABEL_BASED = "label_based"
    PARTICIPANT_BASED = "participant_based"
    DOMAIN_BASED = "domain_based"
    CONTENT_PATTERN = "content_pattern"
    SENDER_EXCLUSION = "sender_exclusion"
    SUBJECT_PATTERN = "subject_pattern"
    ATTACHMENT_TYPE = "attachment_type"
    COMBINED_RULE = "combined_rule"


class FilterAction(str, Enum):
    """Actions to take when filter matches."""

    EXCLUDE = "exclude"
    MARK_SENSITIVE = "mark_sensitive"
    QUARANTINE = "quarantine"
    LOG_ONLY = "log_only"
    REQUIRE_REVIEW = "require_review"
    LIMITED_PROCESSING = "limited_processing"


class PrivacyFilter(Base):
    """Privacy filter configuration model."""

    __tablename__ = 'privacy_filters'

    id = Column(Integer, primary_key=True)
    connection_id = Column(Integer, nullable=False, index=True)

    # Filter identification
    name = Column(String(255), nullable=False)
    description = Column(Text)
    filter_type = Column(String(50), nullable=False)

    # Filter configuration
    filter_config = Column(JSON, nullable=False, default=dict)
    action = Column(String(50), nullable=False, default=FilterAction.EXCLUDE.value)

    # Status and control
    is_active = Column(Boolean, default=True, nullable=False)
    priority = Column(Integer, default=100, nullable=False)  # Lower numbers = higher priority

    # Audit and tracking
    created_at = Column(DateTime, default=datetime.utcnow, nullable=False)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow, nullable=False)
    created_by = Column(String(255))
    last_modified_by = Column(String(255))

    # Usage statistics
    match_count = Column(Integer, default=0, nullable=False)
    last_matched = Column(DateTime)

    # Performance tracking
    avg_processing_time_ms = Column(Integer, default=0)
    total_processing_time_ms = Column(Integer, default=0)

    # Indexes for performance
    __table_args__ = (
        Index('ix_privacy_filter_connection_type', 'connection_id', 'filter_type'),
        Index('ix_privacy_filter_active_priority', 'is_active', 'priority'),
    )

    @validates('filter_type')
    def validate_filter_type(self, key, value):
        """Validate filter type."""
        if value not in [ft.value for ft in FilterType]:
            raise ValueError(f"Invalid filter type: {value}")
        return value

    @validates('action')
    def validate_action(self, key, value):
        """Validate filter action."""
        if value not in [fa.value for fa in FilterAction]:
            raise ValueError(f"Invalid filter action: {value}")
        return value

    @validates('priority')
    def validate_priority(self, key, value):
        """Validate priority range."""
        if not 1 <= value <= 1000:
            raise ValueError("Priority must be between 1 and 1000")
        return value

    def to_dict(self) -> Dict[str, Any]:
        """Convert filter to dictionary representation."""
        return {
            'id': self.id,
            'connection_id': self.connection_id,
            'name': self.name,
            'description': self.description,
            'filter_type': self.filter_type,
            'filter_config': self.filter_config,
            'action': self.action,
            'is_active': self.is_active,
            'priority': self.priority,
            'created_at': self.created_at.isoformat() if self.created_at else None,
            'updated_at': self.updated_at.isoformat() if self.updated_at else None,
            'match_count': self.match_count,
            'last_matched': self.last_matched.isoformat() if self.last_matched else None,
            'avg_processing_time_ms': self.avg_processing_time_ms
        }

    @classmethod
    def create_label_filter(
        cls,
        connection_id: int,
        name: str,
        labels: List[str],
        action: FilterAction = FilterAction.EXCLUDE,
        description: str = None
    ) -> 'PrivacyFilter':
        """Create a label-based privacy filter.

        Args:
            connection_id: Gmail connection ID
            name: Filter name
            labels: List of Gmail labels to filter
            action: Action to take when matched
            description: Optional description

        Returns:
            Privacy filter instance
        """
        return cls(
            connection_id=connection_id,
            name=name,
            description=description or f"Filter emails with labels: {', '.join(labels)}",
            filter_type=FilterType.LABEL_BASED.value,
            action=action.value,
            filter_config={
                'labels': labels,
                'match_any': True  # True = OR logic, False = AND logic
            }
        )

    @classmethod
    def create_participant_filter(
        cls,
        connection_id: int,
        name: str,
        participants: List[str],
        action: FilterAction = FilterAction.EXCLUDE,
        match_type: str = "email",
        description: str = None
    ) -> 'PrivacyFilter':
        """Create a participant-based privacy filter.

        Args:
            connection_id: Gmail connection ID
            name: Filter name
            participants: List of email addresses or names
            action: Action to take when matched
            match_type: "email" or "name" matching
            description: Optional description

        Returns:
            Privacy filter instance
        """
        return cls(
            connection_id=connection_id,
            name=name,
            description=description or f"Filter emails from/to: {', '.join(participants)}",
            filter_type=FilterType.PARTICIPANT_BASED.value,
            action=action.value,
            filter_config={
                'participants': participants,
                'match_type': match_type,
                'case_sensitive': False,
                'match_any': True
            }
        )

    @classmethod
    def create_domain_filter(
        cls,
        connection_id: int,
        name: str,
        domains: List[str],
        action: FilterAction = FilterAction.EXCLUDE,
        description: str = None
    ) -> 'PrivacyFilter':
        """Create a domain-based privacy filter.

        Args:
            connection_id: Gmail connection ID
            name: Filter name
            domains: List of email domains to filter
            action: Action to take when matched
            description: Optional description

        Returns:
            Privacy filter instance
        """
        return cls(
            connection_id=connection_id,
            name=name,
            description=description or f"Filter emails from domains: {', '.join(domains)}",
            filter_type=FilterType.DOMAIN_BASED.value,
            action=action.value,
            filter_config={
                'domains': domains,
                'include_subdomains': True,
                'match_any': True
            }
        )

    @classmethod
    def create_content_pattern_filter(
        cls,
        connection_id: int,
        name: str,
        patterns: List[str],
        action: FilterAction = FilterAction.MARK_SENSITIVE,
        match_subject: bool = True,
        match_body: bool = True,
        case_sensitive: bool = False,
        description: str = None
    ) -> 'PrivacyFilter':
        """Create a content pattern privacy filter.

        Args:
            connection_id: Gmail connection ID
            name: Filter name
            patterns: List of regex patterns to match
            action: Action to take when matched
            match_subject: Whether to match in subject
            match_body: Whether to match in body
            case_sensitive: Whether matching is case sensitive
            description: Optional description

        Returns:
            Privacy filter instance
        """
        return cls(
            connection_id=connection_id,
            name=name,
            description=description or f"Filter emails matching patterns: {', '.join(patterns)}",
            filter_type=FilterType.CONTENT_PATTERN.value,
            action=action.value,
            filter_config={
                'patterns': patterns,
                'match_subject': match_subject,
                'match_body': match_body,
                'case_sensitive': case_sensitive,
                'match_any_pattern': True
            }
        )


class PrivacyAuditLog(Base):
    """Privacy filter audit log for tracking filter actions."""

    __tablename__ = 'privacy_audit_logs'

    id = Column(Integer, primary_key=True)

    # Email identification
    gmail_message_id = Column(String(255), nullable=False, index=True)
    gmail_thread_id = Column(String(255), nullable=False, index=True)
    connection_id = Column(Integer, nullable=False, index=True)

    # Filter information
    filter_id = Column(Integer, nullable=False, index=True)
    filter_name = Column(String(255), nullable=False)
    filter_type = Column(String(50), nullable=False)
    action_taken = Column(String(50), nullable=False)

    # Match details
    match_reason = Column(Text)
    matched_criteria = Column(JSON)
    confidence_score = Column(Integer, default=100)  # 0-100 confidence in match

    # Processing info
    processing_time_ms = Column(Integer)
    timestamp = Column(DateTime, default=datetime.utcnow, nullable=False, index=True)

    # Context
    email_metadata = Column(JSON)  # Limited email metadata for audit
    user_context = Column(String(255))  # User or system that triggered

    # Indexes
    __table_args__ = (
        Index('ix_audit_filter_time', 'filter_id', 'timestamp'),
        Index('ix_audit_connection_time', 'connection_id', 'timestamp'),
        Index('ix_audit_action_time', 'action_taken', 'timestamp'),
    )

    def to_dict(self) -> Dict[str, Any]:
        """Convert audit log to dictionary."""
        return {
            'id': self.id,
            'gmail_message_id': self.gmail_message_id,
            'gmail_thread_id': self.gmail_thread_id,
            'connection_id': self.connection_id,
            'filter_id': self.filter_id,
            'filter_name': self.filter_name,
            'filter_type': self.filter_type,
            'action_taken': self.action_taken,
            'match_reason': self.match_reason,
            'matched_criteria': self.matched_criteria,
            'confidence_score': self.confidence_score,
            'processing_time_ms': self.processing_time_ms,
            'timestamp': self.timestamp.isoformat() if self.timestamp else None,
            'email_metadata': self.email_metadata,
            'user_context': self.user_context
        }


class RetentionPolicy(Base):
    """Data retention policy configuration."""

    __tablename__ = 'retention_policies'

    id = Column(Integer, primary_key=True)
    connection_id = Column(Integer, nullable=False, index=True)

    # Policy identification
    name = Column(String(255), nullable=False)
    description = Column(Text)

    # Retention rules
    email_retention_days = Column(Integer, default=365)  # How long to keep email data
    attachment_retention_days = Column(Integer, default=90)  # How long to keep attachments
    audit_log_retention_days = Column(Integer, default=2557)  # 7 years for compliance

    # Data types to retain/delete
    retain_email_metadata = Column(Boolean, default=True)
    retain_email_content = Column(Boolean, default=True)
    retain_attachments = Column(Boolean, default=True)
    retain_processing_results = Column(Boolean, default=True)

    # Special handling
    sensitive_data_retention_days = Column(Integer, default=30)  # Shorter retention for sensitive data
    auto_delete_enabled = Column(Boolean, default=False)
    require_manual_approval = Column(Boolean, default=True)

    # Status
    is_active = Column(Boolean, default=True)
    created_at = Column(DateTime, default=datetime.utcnow)
    updated_at = Column(DateTime, default=datetime.utcnow, onupdate=datetime.utcnow)

    # Statistics
    emails_deleted = Column(Integer, default=0)
    last_cleanup_run = Column(DateTime)
    next_cleanup_scheduled = Column(DateTime)

    def to_dict(self) -> Dict[str, Any]:
        """Convert retention policy to dictionary."""
        return {
            'id': self.id,
            'connection_id': self.connection_id,
            'name': self.name,
            'description': self.description,
            'email_retention_days': self.email_retention_days,
            'attachment_retention_days': self.attachment_retention_days,
            'audit_log_retention_days': self.audit_log_retention_days,
            'retain_email_metadata': self.retain_email_metadata,
            'retain_email_content': self.retain_email_content,
            'retain_attachments': self.retain_attachments,
            'retain_processing_results': self.retain_processing_results,
            'sensitive_data_retention_days': self.sensitive_data_retention_days,
            'auto_delete_enabled': self.auto_delete_enabled,
            'require_manual_approval': self.require_manual_approval,
            'is_active': self.is_active,
            'emails_deleted': self.emails_deleted,
            'last_cleanup_run': self.last_cleanup_run.isoformat() if self.last_cleanup_run else None,
            'next_cleanup_scheduled': self.next_cleanup_scheduled.isoformat() if self.next_cleanup_scheduled else None
        }