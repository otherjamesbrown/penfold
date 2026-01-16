"""SQLAlchemy models for Penfold storage layer."""

import uuid
from datetime import datetime, timedelta
from decimal import Decimal
from typing import Any, Dict, List, Optional

from sqlalchemy import (
    Boolean,
    Column,
    DateTime,
    Integer,
    String,
    Text,
    DECIMAL,
    ARRAY,
    Index,
    ForeignKey,
    Float,
    CheckConstraint,
    UniqueConstraint,
    event,
)
from sqlalchemy.dialects.postgresql import UUID, JSONB, BIGINT
from sqlalchemy.ext.declarative import declarative_base
from sqlalchemy.sql import func, text
from sqlalchemy.orm import relationship, validates, Query

# Alias for compatibility with mixed naming conventions in relationship definitions
orm_relationship = relationship
from sqlalchemy.ext.hybrid import hybrid_property
from .validators import (
    validate_email_list,
    validate_content_hash,
    validate_confidence_score,
    validate_processing_status,
    validate_source_system,
    validate_assertion_type,
    validate_project_status,
    validate_project_priority,
    validate_team_type,
    validate_phone_list,
    validate_entity_type,
    validate_string_length,
    validate_positive_integer,
    validate_non_negative_integer,
    validate_vector_dimensions,
    validate_uuid,
    ValidationError,
)

# Import pgvector types
try:
    from pgvector.sqlalchemy import Vector
except ImportError:
    # Fallback for development without pgvector installed
    Vector = None

Base = declarative_base()


class SoftDeleteQueryMixin:
    """
    Query mixin that provides soft delete filtering capabilities.

    This mixin should be used with models that inherit from SoftDeleteMixin
    to provide default query filtering for active records.
    """

    @classmethod
    def active(cls):
        """Return query for active (non-deleted) records."""
        return cls.query.filter(cls.is_deleted == False)

    @classmethod
    def deleted(cls):
        """Return query for soft-deleted records."""
        return cls.query.filter(cls.is_deleted == True)

    @classmethod
    def with_deleted(cls):
        """Return query that includes both active and deleted records."""
        return cls.query

    @classmethod
    def active_count(cls, session):
        """Get count of active records."""
        return session.query(cls).filter(cls.is_deleted == False).count()

    @classmethod
    def deleted_count(cls, session):
        """Get count of deleted records."""
        return session.query(cls).filter(cls.is_deleted == True).count()


# Custom query class with soft delete awareness
class SoftDeleteQuery(Query):
    """
    Custom query class that automatically filters out soft-deleted records
    unless explicitly requested.
    """

    def __new__(cls, *args, **kwargs):
        # Check if the model has soft delete capability
        if args and hasattr(args[0], 'is_deleted'):
            obj = Query.__new__(cls)
            obj._auto_filter_deleted = True
            return obj
        return Query.__new__(cls)

    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self._include_deleted = False
        self._auto_filter_deleted = getattr(self, '_auto_filter_deleted', False)

    def filter_active(self):
        """Explicitly filter to only active records."""
        if hasattr(self.column_descriptions[0]['type'], 'is_deleted'):
            model = self.column_descriptions[0]['type']
            return self.filter(model.is_deleted == False)
        return self

    def filter_deleted(self):
        """Explicitly filter to only deleted records."""
        if hasattr(self.column_descriptions[0]['type'], 'is_deleted'):
            model = self.column_descriptions[0]['type']
            return self.filter(model.is_deleted == True)
        return self

    def with_deleted(self):
        """Include soft-deleted records in results."""
        self._include_deleted = True
        return self

    def all(self):
        """Override all() to apply soft delete filtering."""
        if self._auto_filter_deleted and not self._include_deleted:
            return self.filter_active().all()
        return super().all()

    def first(self):
        """Override first() to apply soft delete filtering."""
        if self._auto_filter_deleted and not self._include_deleted:
            return self.filter_active().first()
        return super().first()

    def one(self):
        """Override one() to apply soft delete filtering."""
        if self._auto_filter_deleted and not self._include_deleted:
            return self.filter_active().one()
        return super().one()

    def one_or_none(self):
        """Override one_or_none() to apply soft delete filtering."""
        if self._auto_filter_deleted and not self._include_deleted:
            return self.filter_active().one_or_none()
        return super().one_or_none()

    def count(self):
        """Override count() to apply soft delete filtering."""
        if self._auto_filter_deleted and not self._include_deleted:
            return self.filter_active().count()
        return super().count()


class TimestampMixin:
    """Mixin for created_at/updated_at timestamps."""

    created_at = Column(
        DateTime(timezone=True),
        server_default=func.now(),
        nullable=False,
        comment="When this record was created",
    )
    updated_at = Column(
        DateTime(timezone=True),
        server_default=func.now(),
        onupdate=func.now(),
        nullable=False,
        comment="When this record was last updated",
    )


class TenantMixin:
    """Mixin for tenant isolation using Row-Level Security."""

    tenant_id = Column(
        UUID(as_uuid=True),
        nullable=False,
        index=True,
        comment="Tenant ID for Row-Level Security",
    )


class SoftDeleteMixin:
    """Mixin for soft delete functionality."""

    deleted_at = Column(
        DateTime(timezone=True),
        nullable=True,
        comment="When this record was soft deleted",
    )
    is_deleted = Column(
        Boolean,
        default=False,
        nullable=False,
        index=True,
        comment="Whether this record is soft deleted",
    )
    deleted_by = Column(
        String(100),
        nullable=True,
        comment="User or system that performed the soft delete",
    )
    deletion_reason = Column(
        String(500),
        nullable=True,
        comment="Reason for deletion for audit purposes",
    )

    def soft_delete(self, deleted_by: str = "system", reason: str = None):
        """
        Perform soft delete on this record.

        Args:
            deleted_by: User or system identifier performing the deletion
            reason: Optional reason for deletion
        """
        if self.is_deleted:
            raise ValueError(f"Record {self.__class__.__name__}:{self.id} is already soft deleted")

        self.is_deleted = True
        self.deleted_at = datetime.utcnow()
        self.deleted_by = deleted_by
        self.deletion_reason = reason

    def restore(self, restored_by: str = "system"):
        """
        Restore a soft-deleted record.

        Args:
            restored_by: User or system identifier performing the restoration
        """
        if not self.is_deleted:
            raise ValueError(f"Record {self.__class__.__name__}:{self.id} is not soft deleted")

        self.is_deleted = False
        self.deleted_at = None
        self.deleted_by = None
        self.deletion_reason = None
        # Note: We maintain the updated_at timestamp from restoration

    @property
    def is_active(self) -> bool:
        """Check if record is active (not soft deleted)."""
        return not self.is_deleted

    @classmethod
    def active_only(cls):
        """Return query filter for active (non-deleted) records."""
        return cls.is_deleted == False

    @classmethod
    def deleted_only(cls):
        """Return query filter for soft-deleted records."""
        return cls.is_deleted == True

    @classmethod
    def all_including_deleted(cls):
        """Return query that includes both active and deleted records."""
        # This is the default behavior, but provided for clarity
        return True

    def get_deletion_info(self) -> Dict[str, Any]:
        """Get detailed information about deletion status."""
        return {
            "is_deleted": self.is_deleted,
            "deleted_at": self.deleted_at,
            "deleted_by": self.deleted_by,
            "deletion_reason": self.deletion_reason,
        }


# Core entity models
# =============================================================================
# TENANT MANAGEMENT MODELS
# =============================================================================


class Tenant(Base, TimestampMixin, SoftDeleteMixin, SoftDeleteQueryMixin):
    """Tenant context definition and configuration.

    Represents a context/environment like 'work', 'personal', 'family' with
    complete data isolation between tenants.
    """

    __tablename__ = "tenants"

    id = Column(
        UUID(as_uuid=True),
        primary_key=True,
        default=uuid.uuid4,
        nullable=False,
        comment="Unique tenant identifier",
    )

    name = Column(
        String(50),
        nullable=False,
        unique=True,
        comment="Tenant name (work, personal, family)",
    )

    display_name = Column(
        String(100),
        nullable=False,
        comment="Human-readable tenant name",
    )

    description = Column(
        Text,
        nullable=True,
        comment="Tenant description and purpose",
    )

    settings = Column(
        JSONB,
        nullable=True,
        default={},
        comment="Tenant-specific configuration settings",
    )

    is_active = Column(
        Boolean,
        nullable=False,
        default=True,
        comment="Whether tenant is active and available",
    )

    owner_email = Column(
        String(254),
        nullable=False,
        comment="Primary owner email for this tenant",
    )

    # Indexes for efficient queries
    __table_args__ = (
        Index("idx_tenants_name", "name"),
        Index("idx_tenants_owner", "owner_email"),
        Index("idx_tenants_active", "is_active", "created_at"),
        CheckConstraint(
            "name IN ('work', 'personal', 'family')",
            name="ck_tenant_valid_names"
        ),
        CheckConstraint(
            "char_length(name) >= 3",
            name="ck_tenant_name_length"
        ),
        CheckConstraint(
            "char_length(display_name) >= 1",
            name="ck_tenant_display_name_not_empty"
        ),
    )

    @validates("name")
    def validate_name(self, key, name):
        """Validate tenant name."""
        from .validators import validate_string_length
        validate_string_length(name, "Tenant name", 3, 50)
        if name not in {"work", "personal", "family"}:
            raise ValidationError(f"Invalid tenant name: {name}. Must be work, personal, or family")
        return name.lower()

    @validates("display_name")
    def validate_display_name(self, key, display_name):
        """Validate tenant display name."""
        from .validators import validate_string_length
        validate_string_length(display_name, "Display name", 1, 100)
        return display_name

    @validates("owner_email")
    def validate_owner_email(self, key, owner_email):
        """Validate owner email."""
        from .validators import validate_email
        validate_email(owner_email)
        return owner_email.lower().strip()


class TenantSession(Base, TimestampMixin):
    """Active tenant context tracking for user sessions.

    Tracks which tenant is currently active for a user session to provide
    persistent tenant selection across CLI operations.
    """

    __tablename__ = "tenant_sessions"

    id = Column(
        BIGINT,
        primary_key=True,
        autoincrement=True,
        comment="Session record identifier",
    )

    session_id = Column(
        String(255),
        nullable=False,
        unique=True,
        comment="Unique session identifier",
    )

    tenant_id = Column(
        UUID(as_uuid=True),
        ForeignKey("tenants.id", ondelete="CASCADE"),
        nullable=False,
        comment="Active tenant for this session",
    )

    user_email = Column(
        String(254),
        nullable=False,
        comment="User email for this session",
    )

    last_activity = Column(
        DateTime(timezone=True),
        nullable=False,
        default=datetime.utcnow,
        comment="Last activity timestamp",
    )

    expires_at = Column(
        DateTime(timezone=True),
        nullable=False,
        default=lambda: datetime.utcnow() + timedelta(hours=24),
        comment="Session expiration timestamp",
    )

    # Relationships
    tenant = relationship("Tenant", backref="sessions")

    # Indexes for efficient queries
    __table_args__ = (
        Index("idx_session_tenant_user", "tenant_id", "user_email"),
        Index("idx_session_activity", "last_activity"),
        Index("idx_session_expires", "expires_at"),
    )

    @validates("session_id")
    def validate_session_id(self, key, session_id):
        """Validate session ID."""
        from .validators import validate_string_length
        validate_string_length(session_id, "Session ID", 1, 255)
        return session_id

    @validates("user_email")
    def validate_user_email(self, key, user_email):
        """Validate user email."""
        from .validators import validate_email
        validate_email(user_email)
        return user_email.lower().strip()

    @property
    def is_expired(self) -> bool:
        """Check if session is expired."""
        return datetime.utcnow() > self.expires_at

    def extend_session(self, hours: int = 24) -> None:
        """Extend session expiration."""
        self.last_activity = datetime.utcnow()
        self.expires_at = datetime.utcnow() + timedelta(hours=hours)


class CrossTenantPersonLink(Base, TimestampMixin):
    """Links person entities across different tenants.

    Enables entity resolution where the same physical person appears in
    multiple tenant contexts (e.g., colleague who is also a friend) while
    maintaining strict data isolation for all other entities.
    """

    __tablename__ = "cross_tenant_person_links"

    id = Column(
        BIGINT,
        primary_key=True,
        autoincrement=True,
        comment="Link record identifier",
    )

    canonical_person_id = Column(
        String(100),
        nullable=False,
        comment="Canonical identifier for this person across tenants",
    )

    tenant_id = Column(
        UUID(as_uuid=True),
        ForeignKey("tenants.id", ondelete="CASCADE"),
        nullable=False,
        comment="Tenant where this person record exists",
    )

    person_id = Column(
        BIGINT,
        ForeignKey("people.id", ondelete="CASCADE"),
        nullable=False,
        comment="Person record in specific tenant",
    )

    link_confidence = Column(
        DECIMAL(precision=4, scale=3),
        nullable=False,
        default=Decimal("1.000"),
        comment="Confidence that these represent the same person",
    )

    link_method = Column(
        String(50),
        nullable=False,
        default="manual",
        comment="How this link was established (manual, email_match, ai_suggested)",
    )

    validated_by = Column(
        String(254),
        nullable=True,
        comment="Email of user who validated this link",
    )

    validated_at = Column(
        DateTime(timezone=True),
        nullable=True,
        comment="When this link was validated",
    )

    # Relationships
    tenant = relationship("Tenant", backref="person_links")
    person = relationship("Person", backref="cross_tenant_links")

    # Indexes for efficient queries
    __table_args__ = (
        Index("idx_person_links_canonical", "canonical_person_id"),
        Index("idx_person_links_tenant_person", "tenant_id", "person_id"),
        Index("idx_person_links_confidence", "link_confidence"),
        UniqueConstraint(
            "tenant_id", "person_id",
            name="uc_tenant_person_link"
        ),
        CheckConstraint(
            "link_confidence >= 0.000 AND link_confidence <= 1.000",
            name="ck_link_confidence_range"
        ),
        CheckConstraint(
            "link_method IN ('manual', 'email_match', 'ai_suggested', 'name_similarity')",
            name="ck_link_method_valid"
        ),
    )

    @validates("canonical_person_id")
    def validate_canonical_person_id(self, key, canonical_person_id):
        """Validate canonical person ID."""
        from .validators import validate_string_length
        validate_string_length(canonical_person_id, "Canonical person ID", 1, 100)
        return canonical_person_id

    @validates("link_confidence")
    def validate_link_confidence(self, key, link_confidence):
        """Validate link confidence score."""
        from .validators import validate_confidence_score
        validate_confidence_score(link_confidence)
        return link_confidence

    @validates("link_method")
    def validate_link_method(self, key, link_method):
        """Validate link method."""
        valid_methods = {"manual", "email_match", "ai_suggested", "name_similarity"}
        if link_method not in valid_methods:
            raise ValidationError(f"Invalid link method: {link_method}")
        return link_method

    @validates("validated_by")
    def validate_validated_by(self, key, validated_by):
        """Validate validator email."""
        if validated_by:
            from .validators import validate_email
            validate_email(validated_by)
            return validated_by.lower().strip()
        return validated_by


class UserTenantMembership(Base, TimestampMixin):
    """User membership in tenants for access control.

    Tracks which users have access to which tenants with role-based permissions.
    """

    __tablename__ = "user_tenant_memberships"

    id = Column(
        BIGINT,
        primary_key=True,
        autoincrement=True,
        comment="Membership record identifier",
    )

    user_email = Column(
        String(254),
        nullable=False,
        index=True,
        comment="User email address",
    )

    tenant_id = Column(
        UUID(as_uuid=True),
        ForeignKey("tenants.id", ondelete="CASCADE"),
        nullable=False,
        comment="Tenant the user has access to",
    )

    role = Column(
        String(50),
        nullable=False,
        default="member",
        comment="User role in tenant (owner, admin, member)",
    )

    granted_by = Column(
        String(254),
        nullable=True,
        comment="Email of user who granted access",
    )

    granted_at = Column(
        DateTime(timezone=True),
        nullable=False,
        server_default=func.now(),
        comment="When access was granted",
    )

    is_active = Column(
        Boolean,
        nullable=False,
        default=True,
        comment="Whether membership is currently active",
    )

    # Relationships
    tenant = relationship("Tenant", backref="memberships")

    __table_args__ = (
        UniqueConstraint(
            "user_email", "tenant_id",
            name="uq_user_tenant_membership"
        ),
        Index("idx_membership_user", "user_email"),
        Index("idx_membership_tenant", "tenant_id"),
        CheckConstraint(
            "role IN ('owner', 'admin', 'member')",
            name="ck_valid_membership_role"
        ),
    )

    @validates("user_email")
    def validate_user_email(self, key, user_email):
        """Validate user email address."""
        from .validators import validate_email
        validate_email(user_email)
        return user_email.lower().strip()

    @validates("role")
    def validate_role(self, key, role):
        """Validate membership role."""
        valid_roles = {"owner", "admin", "member"}
        if role not in valid_roles:
            raise ValidationError(f"Invalid role: {role}. Must be one of: {valid_roles}")
        return role

    @validates("granted_by")
    def validate_granted_by(self, key, granted_by):
        """Validate granter email."""
        if granted_by:
            from .validators import validate_email
            validate_email(granted_by)
            return granted_by.lower().strip()
        return granted_by


class TenantAuditLog(Base, TimestampMixin):
    """Audit log for tenant access events."""

    __tablename__ = "tenant_audit_logs"

    id = Column(
        BIGINT,
        primary_key=True,
        autoincrement=True,
        comment="Audit log record identifier",
    )

    event_type = Column(
        String(50),
        nullable=False,
        comment="Event type (switch_success, switch_denied, membership_granted, membership_revoked)",
    )

    user_email = Column(
        String(254),
        nullable=False,
        index=True,
        comment="User email address",
    )

    tenant_id = Column(
        UUID(as_uuid=True),
        ForeignKey("tenants.id", ondelete="SET NULL"),
        nullable=True,
        comment="Tenant ID (null for denied access attempts)",
    )

    tenant_name = Column(
        String(50),
        nullable=True,
        comment="Tenant name at time of event",
    )

    details = Column(
        JSONB,
        nullable=True,
        comment="Additional event details",
    )

    ip_address = Column(
        String(45),
        nullable=True,
        comment="Client IP if available",
    )

    session_id = Column(
        String(255),
        nullable=True,
        comment="Session ID for tracking",
    )

    # Relationships
    tenant = relationship("Tenant", backref="audit_logs")

    __table_args__ = (
        Index("idx_audit_user_time", "user_email", "created_at"),
        Index("idx_audit_tenant_time", "tenant_id", "created_at"),
        Index("idx_audit_type_time", "event_type", "created_at"),
    )

    @validates("user_email")
    def validate_user_email(self, key, user_email):
        """Validate user email address."""
        from .validators import validate_email
        validate_email(user_email)
        return user_email.lower().strip()

    @validates("event_type")
    def validate_event_type(self, key, event_type):
        """Validate event type."""
        valid_types = {"switch_success", "switch_denied", "membership_granted", "membership_revoked"}
        if event_type not in valid_types:
            raise ValidationError(f"Invalid event type: {event_type}. Must be one of: {valid_types}")
        return event_type


# =============================================================================
# CORE BUSINESS ENTITY MODELS
# =============================================================================


class Source(Base, TimestampMixin, TenantMixin, SoftDeleteMixin, SoftDeleteQueryMixin):
    """
    Raw content from external systems (email, Slack, documents, meetings).

    This is the foundational entity that stores all ingested content
    with metadata for tracking and processing.
    """

    __tablename__ = "sources"

    id = Column(BIGINT, primary_key=True)

    # Content identification
    source_system = Column(
        String(50), nullable=False, comment="System that provided this content (gmail, slack, etc.)"
    )
    external_id = Column(
        String(255), nullable=False, comment="ID from the source system"
    )
    content_hash = Column(
        String(64), nullable=False, comment="SHA-256 hash of raw content for deduplication"
    )

    # Content storage
    raw_content = Column(Text, comment="Original content as received from source")
    content_type = Column(String(100), comment="MIME type or content classification")
    content_size = Column(Integer, comment="Size of content in bytes")

    # Metadata
    ingestion_metadata = Column(
        JSONB, default={}, comment="Metadata from ingestion process"
    )
    # Indexed participant emails for efficient searching
    participant_emails = Column(
        ARRAY(Text),
        default=[],
        comment="Extracted participant email addresses for indexed search"
    )
    processing_status = Column(
        String(20),
        default="pending",
        nullable=False,
        comment="Current processing status (pending, processing, completed, failed)",
    )

    # Temporal tracking
    source_timestamp = Column(
        DateTime(timezone=True),
        comment="When this content was originally created in the source system",
    )

    # Relationships
    assertions = relationship("Assertion", back_populates="source", lazy="select")
    embeddings = relationship("Embedding", back_populates="source", lazy="select")

    # Validation methods
    @validates('source_system')
    def validate_source_system_value(self, key, value):
        """Validate source system is from allowed list."""
        validate_source_system(value)
        return value

    @validates('external_id')
    def validate_external_id_value(self, key, value):
        """Validate external ID length and format."""
        validate_string_length(value, 'external_id', min_length=1, max_length=255)
        return value.strip()

    @validates('content_hash')
    def validate_content_hash_value(self, key, value):
        """Validate content hash format."""
        validate_content_hash(value)
        return value.lower()

    @validates('processing_status')
    def validate_processing_status_value(self, key, value):
        """Validate processing status is from allowed list."""
        validate_processing_status(value)
        return value

    @validates('content_type')
    def validate_content_type_value(self, key, value):
        """Validate content type length."""
        if value is not None:
            validate_string_length(value, 'content_type', max_length=100)
        return value

    @validates('content_size')
    def validate_content_size_value(self, key, value):
        """Validate content size is non-negative."""
        if value is not None:
            validate_non_negative_integer(value, 'content_size')
        return value

    def __repr__(self):
        return f"<Source(id={self.id}, source_system='{self.source_system}', external_id='{self.external_id}')>"

    __table_args__ = (
        Index("idx_sources_tenant_created", "tenant_id", "created_at"),
        Index(
            "idx_sources_unique_external",
            "tenant_id",
            "source_system",
            "external_id",
            unique=True,
        ),
        Index("idx_sources_hash", "content_hash"),
        Index("idx_sources_status", "processing_status"),
        Index("idx_sources_soft_delete", "is_deleted", "deleted_at"),
        Index("idx_sources_deleted_by", "deleted_by"),
        Index(
            "idx_sources_participant_emails",
            "participant_emails",
            postgresql_using="gin"
        ),
        # Database-level constraints
        CheckConstraint("length(source_system) > 0", name="ck_sources_source_system_not_empty"),
        CheckConstraint("length(external_id) > 0", name="ck_sources_external_id_not_empty"),
        CheckConstraint("length(content_hash) = 64", name="ck_sources_content_hash_length"),
        CheckConstraint("content_hash ~ '^[a-fA-F0-9]{64}$'", name="ck_sources_content_hash_hex"),
        CheckConstraint("content_size >= 0", name="ck_sources_content_size_non_negative"),
        CheckConstraint(
            "processing_status IN ('pending', 'processing', 'completed', 'failed', 'retrying')",
            name="ck_sources_processing_status_valid"
        ),
        CheckConstraint(
            "source_system IN ('gmail', 'slack', 'zoom', 'calendar', 'drive', 'confluence', 'jira', 'teams')",
            name="ck_sources_source_system_valid"
        ),
    )


class Assertion(Base, TimestampMixin, TenantMixin, SoftDeleteMixin, SoftDeleteQueryMixin):
    """
    Extracted meaningful information (decisions, risks, commitments, milestones, etc.).

    Assertions are the primary knowledge extracted from sources by AI processing.
    """

    __tablename__ = "assertions"

    id = Column(BIGINT, primary_key=True)
    source_id = Column(
        BIGINT,
        ForeignKey("sources.id"),
        nullable=False,
        comment="Reference to the source this assertion was extracted from",
    )

    # Assertion classification
    assertion_type = Column(
        String(50),
        nullable=False,
        comment="Type of assertion (decision, risk, commitment, milestone, etc.)",
    )
    content = Column(Text, nullable=False, comment="The extracted assertion content")
    context = Column(Text, comment="Additional context around the assertion")

    # AI processing metadata
    confidence_score = Column(
        DECIMAL(4, 3), comment="AI confidence in this assertion (0.000-1.000)"
    )
    extraction_model = Column(
        String(100), comment="Model used to extract this assertion"
    )
    processing_metadata = Column(
        JSONB, default={}, comment="Metadata from AI processing"
    )

    # Relationships and tags
    related_entities = Column(
        JSONB, default={}, comment="References to people, projects, etc."
    )
    tags = Column(ARRAY(String), comment="Tags for categorization")

    # Validation
    user_validated = Column(
        Boolean, default=False, comment="Whether this assertion has been validated by a human"
    )
    validation_feedback = Column(
        JSONB, default={}, comment="Human feedback on this assertion"
    )

    # Relationships
    source = relationship("Source", back_populates="assertions", lazy="select")
    embeddings = relationship("Embedding", back_populates="assertion", lazy="select")

    # Validation methods
    @validates('assertion_type')
    def validate_assertion_type_value(self, key, value):
        """Validate assertion type is from allowed list."""
        validate_assertion_type(value)
        return value

    @validates('content')
    def validate_content_value(self, key, value):
        """Validate content is non-empty."""
        validate_string_length(value, 'content', min_length=1, max_length=10000)
        return value.strip()

    @validates('context')
    def validate_context_value(self, key, value):
        """Validate context length."""
        if value is not None:
            validate_string_length(value, 'context', max_length=10000)
            return value.strip()
        return value

    @validates('confidence_score')
    def validate_confidence_score_value(self, key, value):
        """Validate confidence score range."""
        validate_confidence_score(value)
        return value

    @validates('extraction_model')
    def validate_extraction_model_value(self, key, value):
        """Validate extraction model length."""
        if value is not None:
            validate_string_length(value, 'extraction_model', max_length=100)
        return value

    @validates('tags')
    def validate_tags_value(self, key, value):
        """Validate tags array."""
        if value is not None and not isinstance(value, list):
            raise ValidationError("Tags must be a list")
        return value

    def __repr__(self):
        return f"<Assertion(id={self.id}, type='{self.assertion_type}', content='{self.content[:50]}...')>"

    __table_args__ = (
        Index("idx_assertions_tenant_type", "tenant_id", "assertion_type", "created_at"),
        Index("idx_assertions_confidence", "confidence_score"),
        Index("idx_assertions_source", "source_id", "created_at"),
        Index("idx_assertions_validated", "user_validated"),
        Index("idx_assertions_soft_delete", "is_deleted", "deleted_at"),
        Index("idx_assertions_deleted_by", "deleted_by"),
        Index("idx_assertions_tags", "tags", postgresql_using="gin"),
        # Database-level constraints
        CheckConstraint("length(assertion_type) > 0", name="ck_assertions_type_not_empty"),
        CheckConstraint("length(content) > 0", name="ck_assertions_content_not_empty"),
        CheckConstraint("length(content) <= 10000", name="ck_assertions_content_max_length"),
        CheckConstraint(
            "confidence_score IS NULL OR (confidence_score >= 0.000 AND confidence_score <= 1.000)",
            name="ck_assertions_confidence_range"
        ),
        CheckConstraint(
            "assertion_type IN ('decision', 'risk', 'commitment', 'milestone', 'outcome', 'dependency', 'assumption', 'issue')",
            name="ck_assertions_type_valid"
        ),
    )


class Person(Base, TimestampMixin, TenantMixin, SoftDeleteMixin, SoftDeleteQueryMixin):
    """
    Entity resolution for individuals with canonical names and aliases.

    Supports resolving the same person across different communication channels
    and name variations.
    """

    __tablename__ = "people"

    id = Column(BIGINT, primary_key=True)

    # Identity
    canonical_name = Column(
        String(200), nullable=False, comment="Primary name for this person"
    )
    aliases = Column(
        ARRAY(String), default=[], comment="Alternative names, nicknames, email addresses"
    )

    # Contact information
    email_addresses = Column(ARRAY(String), comment="Known email addresses")
    phone_numbers = Column(ARRAY(String), comment="Known phone numbers")

    # Organizational context
    job_title = Column(String(200), comment="Current job title")
    company = Column(String(200), comment="Current company")
    department = Column(String(200), comment="Department within company")

    # Metadata
    entity_resolution_metadata = Column(
        JSONB, default={}, comment="Metadata from entity resolution process"
    )
    confidence_score = Column(
        DECIMAL(4, 3), comment="Confidence in entity resolution"
    )

    # Relationships
    embeddings = relationship("Embedding", back_populates="person", lazy="select")

    # Validation methods
    @validates('canonical_name')
    def validate_canonical_name_value(self, key, value):
        """Validate canonical name length and format."""
        validate_string_length(value, 'canonical_name', min_length=1, max_length=200)
        return value.strip()

    @validates('email_addresses')
    def validate_email_addresses_value(self, key, value):
        """Validate email addresses format."""
        if value is not None:
            validate_email_list(value)
        return value

    @validates('phone_numbers')
    def validate_phone_numbers_value(self, key, value):
        """Validate phone numbers format."""
        if value is not None:
            validate_phone_list(value)
        return value

    @validates('job_title', 'company', 'department')
    def validate_organizational_fields(self, key, value):
        """Validate organizational field lengths."""
        if value is not None:
            validate_string_length(value, key, max_length=200)
            return value.strip()
        return value

    @validates('confidence_score')
    def validate_confidence_score_value(self, key, value):
        """Validate confidence score range."""
        validate_confidence_score(value)
        return value

    def __repr__(self):
        return f"<Person(id={self.id}, canonical_name='{self.canonical_name}')>"

    __table_args__ = (
        Index("idx_people_tenant_name", "tenant_id", "canonical_name"),
        Index("idx_people_aliases", "aliases", postgresql_using="gin"),
        Index("idx_people_emails", "email_addresses", postgresql_using="gin"),
        Index("idx_people_phones", "phone_numbers", postgresql_using="gin"),
        Index("idx_people_company", "company"),
        Index("idx_people_soft_delete", "is_deleted", "deleted_at"),
        Index("idx_people_deleted_by", "deleted_by"),
        # Database-level constraints
        CheckConstraint("length(canonical_name) > 0", name="ck_people_name_not_empty"),
        CheckConstraint("length(canonical_name) <= 200", name="ck_people_name_max_length"),
        CheckConstraint(
            "confidence_score IS NULL OR (confidence_score >= 0.000 AND confidence_score <= 1.000)",
            name="ck_people_confidence_range"
        ),
        CheckConstraint(
            "job_title IS NULL OR length(job_title) <= 200",
            name="ck_people_job_title_max_length"
        ),
        CheckConstraint(
            "company IS NULL OR length(company) <= 200",
            name="ck_people_company_max_length"
        ),
        CheckConstraint(
            "department IS NULL OR length(department) <= 200",
            name="ck_people_department_max_length"
        ),
    )


class Project(Base, TimestampMixin, TenantMixin, SoftDeleteMixin, SoftDeleteQueryMixin):
    """
    Hierarchical structure for organizing initiatives with artifacts and timeline.

    Supports nested project structures and timeline tracking.
    """

    __tablename__ = "projects"

    id = Column(BIGINT, primary_key=True)

    # Project identity
    name = Column(String(200), nullable=False, comment="Project name")
    description = Column(Text, comment="Project description")
    status = Column(
        String(50),
        default="active",
        comment="Project status (active, completed, paused, cancelled)",
    )

    # Hierarchy support using ltree
    path = Column(String(500), comment="Hierarchical path using ltree notation")
    parent_id = Column(BIGINT, ForeignKey("projects.id"), comment="Parent project ID")

    # Timeline
    start_date = Column(DateTime(timezone=True), comment="Project start date")
    end_date = Column(DateTime(timezone=True), comment="Project end date")
    target_completion = Column(DateTime(timezone=True), comment="Target completion date")

    # Project metadata
    project_type = Column(String(100), comment="Type of project")
    priority = Column(String(20), comment="Project priority (high, medium, low)")
    budget_allocated = Column(DECIMAL(12, 2), comment="Budget allocated for project")
    budget_spent = Column(DECIMAL(12, 2), comment="Budget spent so far")

    # Artifacts and references
    artifacts = Column(
        JSONB, default=[], comment="References to documents, files, links"
    )
    milestones = Column(JSONB, default=[], comment="Project milestones")
    team_members = Column(ARRAY(BIGINT), comment="References to people on the team")

    # Relationships
    children = relationship("Project", backref="parent", remote_side=[id], lazy="select")
    embeddings = relationship("Embedding", back_populates="project", lazy="select")

    # Validation methods
    @validates('name')
    def validate_name_value(self, key, value):
        """Validate project name length and format."""
        validate_string_length(value, 'name', min_length=1, max_length=200)
        return value.strip()

    @validates('status')
    def validate_status_value(self, key, value):
        """Validate project status is from allowed list."""
        validate_project_status(value)
        return value

    @validates('priority')
    def validate_priority_value(self, key, value):
        """Validate project priority is from allowed list."""
        if value is not None:
            validate_project_priority(value)
        return value

    @validates('project_type')
    def validate_project_type_value(self, key, value):
        """Validate project type length."""
        if value is not None:
            validate_string_length(value, 'project_type', max_length=100)
        return value

    @validates('path')
    def validate_path_value(self, key, value):
        """Validate hierarchical path length."""
        if value is not None:
            validate_string_length(value, 'path', max_length=500)
        return value

    @validates('budget_allocated', 'budget_spent')
    def validate_budget_values(self, key, value):
        """Validate budget values are non-negative."""
        if value is not None and value < 0:
            raise ValidationError(f"{key} must be non-negative, got {value}")
        return value

    @validates('team_members')
    def validate_team_members_value(self, key, value):
        """Validate team members array."""
        if value is not None and not isinstance(value, list):
            raise ValidationError("Team members must be a list")
        return value

    def __repr__(self):
        return f"<Project(id={self.id}, name='{self.name}', status='{self.status}')>"

    __table_args__ = (
        Index("idx_projects_tenant_name", "tenant_id", "name"),
        Index("idx_projects_status", "status"),
        Index("idx_projects_hierarchy", "path", postgresql_using="gist"),
        Index("idx_projects_timeline", "start_date", "end_date"),
        Index("idx_projects_priority", "priority"),
        Index("idx_projects_budget", "budget_allocated", "budget_spent"),
        Index("idx_projects_soft_delete", "is_deleted", "deleted_at"),
        Index("idx_projects_deleted_by", "deleted_by"),
        # Database-level constraints
        CheckConstraint("length(name) > 0", name="ck_projects_name_not_empty"),
        CheckConstraint("length(name) <= 200", name="ck_projects_name_max_length"),
        CheckConstraint(
            "status IN ('planning', 'active', 'on_hold', 'completed', 'cancelled', 'archived')",
            name="ck_projects_status_valid"
        ),
        CheckConstraint(
            "priority IS NULL OR priority IN ('critical', 'high', 'medium', 'low')",
            name="ck_projects_priority_valid"
        ),
        CheckConstraint(
            "budget_allocated IS NULL OR budget_allocated >= 0",
            name="ck_projects_budget_allocated_non_negative"
        ),
        CheckConstraint(
            "budget_spent IS NULL OR budget_spent >= 0",
            name="ck_projects_budget_spent_non_negative"
        ),
        CheckConstraint(
            "end_date IS NULL OR start_date IS NULL OR end_date >= start_date",
            name="ck_projects_end_after_start"
        ),
        CheckConstraint(
            "parent_id IS NULL OR parent_id != id",
            name="ck_projects_no_self_reference"
        ),
    )


class Team(Base, TimestampMixin, TenantMixin, SoftDeleteMixin, SoftDeleteQueryMixin):
    """
    Organizational units with member relationships and project associations.

    Represents organizational structure and team membership.
    """

    __tablename__ = "teams"

    id = Column(BIGINT, primary_key=True)

    # Team identity
    name = Column(String(200), nullable=False, comment="Team name")
    description = Column(Text, comment="Team description")
    team_type = Column(
        String(100), comment="Type of team (department, project team, working group)"
    )

    # Hierarchy
    parent_team_id = Column(BIGINT, ForeignKey("teams.id"), comment="Parent team ID")

    # Team metadata
    lead_person_id = Column(BIGINT, comment="Reference to team lead person")
    member_person_ids = Column(ARRAY(BIGINT), comment="References to team members")
    associated_project_ids = Column(ARRAY(BIGINT), comment="Associated project IDs")

    # Contact and location
    location = Column(String(200), comment="Team location")
    contact_info = Column(JSONB, comment="Team contact information")

    # Relationships
    children = relationship("Team", backref="parent_team", remote_side=[id], lazy="select")
    embeddings = relationship("Embedding", back_populates="team", lazy="select")

    # Validation methods
    @validates('name')
    def validate_name_value(self, key, value):
        """Validate team name length and format."""
        validate_string_length(value, 'name', min_length=1, max_length=200)
        return value.strip()

    @validates('team_type')
    def validate_team_type_value(self, key, value):
        """Validate team type is from allowed list."""
        if value is not None:
            validate_team_type(value)
        return value

    @validates('location')
    def validate_location_value(self, key, value):
        """Validate location length."""
        if value is not None:
            validate_string_length(value, 'location', max_length=200)
            return value.strip()
        return value

    @validates('member_person_ids', 'associated_project_ids')
    def validate_id_arrays(self, key, value):
        """Validate ID arrays."""
        if value is not None and not isinstance(value, list):
            raise ValidationError(f"{key} must be a list")
        return value

    @validates('lead_person_id')
    def validate_lead_person_id_value(self, key, value):
        """Validate lead person ID is positive."""
        if value is not None:
            validate_positive_integer(value, 'lead_person_id')
        return value

    def __repr__(self):
        return f"<Team(id={self.id}, name='{self.name}', type='{self.team_type}')>"

    __table_args__ = (
        Index("idx_teams_tenant_name", "tenant_id", "name"),
        Index("idx_teams_type", "team_type"),
        Index("idx_teams_lead", "lead_person_id"),
        Index("idx_teams_members", "member_person_ids", postgresql_using="gin"),
        Index("idx_teams_projects", "associated_project_ids", postgresql_using="gin"),
        Index("idx_teams_location", "location"),
        Index("idx_teams_soft_delete", "is_deleted", "deleted_at"),
        Index("idx_teams_deleted_by", "deleted_by"),
        # Database-level constraints
        CheckConstraint("length(name) > 0", name="ck_teams_name_not_empty"),
        CheckConstraint("length(name) <= 200", name="ck_teams_name_max_length"),
        CheckConstraint(
            "team_type IS NULL OR team_type IN ('department', 'project_team', 'working_group', 'committee')",
            name="ck_teams_type_valid"
        ),
        CheckConstraint(
            "location IS NULL OR length(location) <= 200",
            name="ck_teams_location_max_length"
        ),
        CheckConstraint(
            "lead_person_id IS NULL OR lead_person_id > 0",
            name="ck_teams_lead_id_positive"
        ),
        CheckConstraint(
            "parent_team_id IS NULL OR parent_team_id != id",
            name="ck_teams_no_self_reference"
        ),
    )


# Event processing models
class ProcessingEvent(Base, TimestampMixin, TenantMixin):
    """
    Published events for content processing with event type and subscriber tracking.

    Events drive the pub-sub processing framework for AI coordination.
    """

    __tablename__ = "processing_events"

    id = Column(BIGINT, primary_key=True)
    event_id = Column(
        UUID(as_uuid=True),
        default=uuid.uuid4,
        unique=True,
        nullable=False,
        comment="Unique event identifier",
    )

    # Event details
    event_type = Column(
        String(100), nullable=False, comment="Type of event (content.ingested, etc.)"
    )
    payload = Column(JSONB, nullable=False, comment="Event data")
    payload_size = Column(Integer, comment="Size of payload in bytes")

    # Processing tracking
    publisher = Column(String(100), comment="Service that published this event")
    subscriber_count = Column(Integer, default=0, comment="Number of subscribers")
    processing_status = Column(
        String(20), default="published", comment="Event processing status"
    )

    # Temporal with retention (30 days from spec)
    published_at = Column(
        DateTime(timezone=True), server_default=func.now(), nullable=False
    )
    expires_at = Column(
        DateTime(timezone=True),
        server_default=text("NOW() + INTERVAL '30 days'"),
        nullable=False,
    )

    __table_args__ = (
        Index("idx_events_tenant_type", "tenant_id", "event_type", "published_at"),
        Index("idx_events_expiry", "expires_at"),
        Index("idx_events_status", "processing_status"),
    )


class ProcessingJob(Base, TimestampMixin, TenantMixin):
    """
    Individual processing tasks with state management and retry logic.

    Tracks AI processing jobs through their lifecycle with proper error handling.
    """

    __tablename__ = "processing_jobs"

    id = Column(BIGINT, primary_key=True)
    job_id = Column(
        UUID(as_uuid=True),
        default=uuid.uuid4,
        unique=True,
        nullable=False,
        comment="Unique job identifier",
    )

    # Job details
    event_id = Column(
        UUID(as_uuid=True),
        ForeignKey("processing_events.event_id"),
        nullable=False,
        comment="Reference to triggering event",
    )
    processor_name = Column(
        String(100), nullable=False, comment="Name of the processor handling this job"
    )
    job_type = Column(String(100), nullable=False, comment="Type of processing job")

    # State management
    status = Column(
        String(20),
        default="queued",
        nullable=False,
        comment="Job status (queued, in_progress, completed, failed, retrying)",
    )
    priority = Column(Integer, default=100, comment="Job priority (lower = higher priority)")

    # Processing metadata
    input_data = Column(JSONB, comment="Input data for processing")
    processor_metadata = Column(JSONB, comment="Processor-specific metadata")

    # Execution tracking
    started_at = Column(DateTime(timezone=True), comment="When job execution started")
    completed_at = Column(DateTime(timezone=True), comment="When job execution completed")

    # Error handling
    retry_count = Column(Integer, default=0, comment="Number of retry attempts")
    max_retries = Column(Integer, default=3, comment="Maximum number of retries")
    error_message = Column(Text, comment="Error message if job failed")
    error_details = Column(JSONB, comment="Detailed error information")

    # Relationships
    results = relationship("ProcessingResult", back_populates="job", lazy="select")

    __table_args__ = (
        Index("idx_jobs_tenant_status", "tenant_id", "status", "created_at"),
        Index("idx_jobs_processor", "processor_name", "status"),
        Index("idx_jobs_priority", "priority", "created_at"),
        Index("idx_jobs_retry", "status", "retry_count"),
    )


class ProcessingResult(Base, TimestampMixin, TenantMixin):
    """
    Output from AI processors with confidence scores and model attribution.

    Stores results from processing jobs with metadata for comparison and validation.
    """

    __tablename__ = "processing_results"

    id = Column(BIGINT, primary_key=True)
    job_id = Column(
        UUID(as_uuid=True),
        ForeignKey("processing_jobs.job_id"),
        nullable=False,
        comment="Reference to the processing job",
    )

    # Result data
    result_type = Column(
        String(100), nullable=False, comment="Type of result (extraction, classification, etc.)"
    )
    result_data = Column(JSONB, nullable=False, comment="The actual processing result")
    confidence_score = Column(DECIMAL(4, 3), comment="Processor confidence in result")

    # Model attribution
    model_name = Column(String(100), comment="Model used for processing")
    model_version = Column(String(50), comment="Version of the model")
    processor_version = Column(String(50), comment="Version of the processor")

    # Quality metrics
    processing_time_ms = Column(Integer, comment="Processing time in milliseconds")
    quality_score = Column(DECIMAL(4, 3), comment="Quality assessment of result")

    # Comparison and validation
    compared_against = Column(ARRAY(UUID), comment="IDs of results this was compared against")
    validation_status = Column(
        String(20), comment="Validation status (pending, approved, rejected)"
    )
    human_feedback = Column(JSONB, comment="Human feedback on result quality")

    # Retention (30 days from spec)
    expires_at = Column(
        DateTime(timezone=True),
        server_default=text("NOW() + INTERVAL '30 days'"),
        nullable=False,
    )

    # Relationships
    job = relationship("ProcessingJob", back_populates="results", lazy="select")

    __table_args__ = (
        Index("idx_results_tenant_type", "tenant_id", "result_type", "created_at"),
        Index("idx_results_job", "job_id"),
        Index("idx_results_model", "model_name", "model_version"),
        Index("idx_results_expiry", "expires_at"),
        Index("idx_results_validation", "validation_status"),
    )


class Subscription(Base, TimestampMixin, TenantMixin):
    """Event subscription with filtering and routing configuration.

    Manages how processors subscribe to and filter processing events
    for selective event consumption and routing.
    """

    __tablename__ = "subscriptions"

    # Primary identification
    id = Column(UUID(as_uuid=True), primary_key=True, default=uuid.uuid4)
    subscriber_id = Column(String(100), nullable=False)
    subscription_name = Column(String(200), nullable=False)

    # Event filtering
    event_channels = Column(JSONB, nullable=False)  # List of channels to subscribe to
    event_types = Column(JSONB, nullable=True)  # Optional specific event types filter
    filter_criteria = Column(JSONB, nullable=True)  # JSONPath-style filter rules

    # Subscription configuration
    is_active = Column(Boolean, default=True, nullable=False)
    priority = Column(Integer, default=5, nullable=False)  # 1=highest, 10=lowest
    max_retry_attempts = Column(Integer, default=3, nullable=False)
    batch_size = Column(Integer, default=1, nullable=False)  # For batch processing

    # Status tracking
    last_processed_at = Column(DateTime(timezone=True), nullable=True)
    total_events_processed = Column(Integer, default=0, nullable=False)
    total_events_failed = Column(Integer, default=0, nullable=False)

    # Error handling configuration
    dead_letter_queue = Column(String(100), nullable=True)
    error_threshold = Column(Float, default=0.1, nullable=False)  # Failure rate threshold

    # Subscription metadata
    description = Column(Text, nullable=True)
    tags = Column(JSONB, nullable=True)  # For categorization and management

    __table_args__ = (
        Index("idx_subscription_tenant_subscriber", "tenant_id", "subscriber_id"),
        Index("idx_subscription_active", "is_active", "priority"),
        Index("idx_subscription_channels", "event_channels", postgresql_using="gin"),
        Index("idx_subscription_types", "event_types", postgresql_using="gin"),
        Index("idx_subscription_status", "last_processed_at", "is_active"),
        UniqueConstraint(
            "tenant_id", "subscriber_id", "subscription_name",
            name="uq_subscription_per_tenant"
        ),
    )


# Vector storage model
class Embedding(Base, TimestampMixin, TenantMixin):
    """
    Vector representations linked to content with model version and similarity indexes.

    Stores high-dimensional embeddings for semantic search with HNSW indexing.
    """

    __tablename__ = "embeddings"

    id = Column(BIGINT, primary_key=True)

    # Entity linkage (polymorphic)
    entity_type = Column(
        String(50), nullable=False, comment="Type of entity (source, assertion, person, etc.)"
    )
    entity_id = Column(BIGINT, nullable=False, comment="ID of the linked entity")

    # Specific entity foreign keys for referential integrity
    source_id = Column(BIGINT, ForeignKey("sources.id"), comment="Reference to source if applicable")
    assertion_id = Column(BIGINT, ForeignKey("assertions.id"), comment="Reference to assertion if applicable")
    person_id = Column(BIGINT, ForeignKey("people.id"), comment="Reference to person if applicable")
    project_id = Column(BIGINT, ForeignKey("projects.id"), comment="Reference to project if applicable")
    team_id = Column(BIGINT, ForeignKey("teams.id"), comment="Reference to team if applicable")

    # Vector data (768 dimensions for nomic-embed-text compatibility)
    embedding = Column(
        Vector(768) if Vector else ARRAY(Float),  # Fallback to array if pgvector not available
        nullable=False,
        comment="768-dimensional vector embedding",
    )
    embedding_model = Column(
        String(100), nullable=False, comment="Model used to generate embedding"
    )
    model_version = Column(String(50), comment="Version of the embedding model")

    # Content reference
    text_content = Column(Text, comment="Text content that was embedded")
    content_hash = Column(String(64), comment="Hash of the text content")

    # Quality tracking
    generation_confidence = Column(DECIMAL(4, 3), comment="Confidence in embedding generation")
    search_count = Column(Integer, default=0, comment="Number of times used in searches")
    last_searched_at = Column(DateTime(timezone=True), comment="Last time used in search")

    # Relationships
    source = relationship("Source", back_populates="embeddings", lazy="select")
    assertion = relationship("Assertion", back_populates="embeddings", lazy="select")
    person = relationship("Person", back_populates="embeddings", lazy="select")
    project = relationship("Project", back_populates="embeddings", lazy="select")
    team = relationship("Team", back_populates="embeddings", lazy="select")

    __table_args__ = (
        # HNSW vector index with optimized parameters from spec analysis
        Index(
            "idx_embeddings_vector_hnsw",
            "embedding",
            postgresql_using="hnsw",
            postgresql_with={"m": 16, "ef_construction": 200}  # From spec improvements
        ) if Vector else None,
        Index("idx_embeddings_entity", "tenant_id", "entity_type", "entity_id"),
        Index("idx_embeddings_model", "embedding_model", "model_version"),
        Index("idx_embeddings_content_hash", "content_hash"),
        # Remove None values from table args
        *filter(None, []),
    )


# =============================================================================
# DAILY REVIEW WORKFLOW MODELS
# =============================================================================


class ReviewSessionModel(Base, TimestampMixin, TenantMixin):
    """User review workflow sessions with state management and progress tracking.

    Represents a user's active review workflow session including queue position,
    progress counters, mode configuration, and session lifecycle.
    """

    __tablename__ = "review_sessions"

    id = Column(
        BIGINT,
        primary_key=True,
        autoincrement=True,
        comment="Session identifier",
    )
    session_uuid = Column(
        UUID(as_uuid=True),
        nullable=False,
        unique=True,
        default=uuid.uuid4,
        comment="External session reference",
    )
    user_email = Column(
        String(254),
        nullable=False,
        comment="User performing review",
    )
    status = Column(
        String(20),
        nullable=False,
        server_default="active",
        comment="Session status (active, paused, completed, abandoned)",
    )
    started_at = Column(
        DateTime(timezone=True),
        nullable=False,
        server_default=func.now(),
        comment="Session start time",
    )
    last_activity_at = Column(
        DateTime(timezone=True),
        nullable=False,
        server_default=func.now(),
        comment="Last interaction time",
    )
    completed_at = Column(
        DateTime(timezone=True),
        nullable=True,
        comment="Session completion time",
    )
    expires_at = Column(
        DateTime(timezone=True),
        nullable=False,
        server_default=text("NOW() + INTERVAL '24 hours'"),
        comment="Auto-expiration time",
    )

    # Progress tracking
    current_position = Column(
        Integer,
        nullable=False,
        server_default="0",
        comment="Current queue position",
    )
    total_items = Column(
        Integer,
        nullable=False,
        comment="Total items in queue at start",
    )
    items_reviewed = Column(
        Integer,
        nullable=False,
        server_default="0",
        comment="Count of reviewed items",
    )
    items_accepted = Column(
        Integer,
        nullable=False,
        server_default="0",
        comment="Count of accepted items",
    )
    items_rejected = Column(
        Integer,
        nullable=False,
        server_default="0",
        comment="Count of rejected items",
    )
    items_modified = Column(
        Integer,
        nullable=False,
        server_default="0",
        comment="Count of modified items",
    )
    items_skipped = Column(
        Integer,
        nullable=False,
        server_default="0",
        comment="Count of skipped items",
    )

    # Configuration
    review_mode = Column(
        String(30),
        nullable=False,
        server_default="standard",
        comment="Review mode (quick, standard, detailed)",
    )
    priority_mode = Column(
        String(30),
        nullable=False,
        server_default="mixed",
        comment="Priority mode (confidence, importance, recency, mixed)",
    )
    filter_criteria = Column(
        JSONB,
        nullable=False,
        server_default="{}",
        comment="Active filters",
    )
    session_metadata = Column(
        JSONB,
        nullable=False,
        server_default="{}",
        comment="Additional session state",
    )

    # Relationships
    review_items = relationship(
        "ReviewItemModel", back_populates="session", lazy="select"
    )
    user_feedback = relationship(
        "UserFeedbackModel", back_populates="session", lazy="select"
    )
    batch_operations = relationship(
        "BatchOperationModel", back_populates="session", lazy="select"
    )

    __table_args__ = (
        Index("idx_review_session_tenant_user", "tenant_id", "user_email", "status"),
        Index(
            "idx_review_session_active",
            "status",
            "expires_at",
            postgresql_where=text("status = 'active'"),
        ),
        Index("idx_review_session_uuid", "session_uuid"),
        CheckConstraint(
            "status IN ('active', 'paused', 'completed', 'abandoned')",
            name="ck_review_session_status",
        ),
        CheckConstraint(
            "review_mode IN ('quick', 'standard', 'detailed')",
            name="ck_review_session_mode",
        ),
        CheckConstraint(
            "priority_mode IN ('confidence', 'importance', 'recency', 'mixed')",
            name="ck_review_session_priority_mode",
        ),
        CheckConstraint(
            "items_reviewed = items_accepted + items_rejected + "
            "items_modified + items_skipped",
            name="ck_review_session_counts",
        ),
    )

    def __repr__(self):
        return (
            f"<ReviewSessionModel(id={self.id}, user='{self.user_email}', "
            f"status='{self.status}')>"
        )


class ReviewItemModel(Base, TimestampMixin, TenantMixin):
    """Individual items in the review queue.

    Links AI processing results to the review workflow, tracking queue position,
    AI suggestions, user decisions, and undo eligibility.
    """

    __tablename__ = "review_items"

    id = Column(
        BIGINT,
        primary_key=True,
        autoincrement=True,
        comment="Item identifier",
    )
    session_id = Column(
        BIGINT,
        ForeignKey("review_sessions.id"),
        nullable=True,
        comment="Associated session (NULL if unqueued)",
    )
    source_id = Column(
        BIGINT,
        ForeignKey("sources.id"),
        nullable=False,
        comment="Source content reference",
    )
    processing_result_id = Column(
        UUID(as_uuid=True),
        nullable=True,
        comment="AI processing result reference",
    )

    # Queue position and status
    queue_position = Column(
        Integer,
        nullable=True,
        comment="Position in queue",
    )
    status = Column(
        String(20),
        nullable=False,
        server_default="pending",
        comment="Current item status",
    )

    # Content information
    content_type = Column(
        String(50),
        nullable=False,
        comment="Type of content (email, meeting, document)",
    )
    content_preview = Column(
        Text,
        nullable=False,
        comment="Truncated content for display",
    )

    # AI suggestion
    ai_suggestion = Column(
        JSONB,
        nullable=False,
        comment="AI's suggested categorization",
    )
    ai_confidence = Column(
        DECIMAL(4, 3),
        nullable=False,
        comment="AI confidence score (0.000-1.000)",
    )
    ai_model = Column(
        String(100),
        nullable=False,
        comment="Model that generated suggestion",
    )

    # Business context
    business_importance = Column(
        Integer,
        nullable=False,
        server_default="5",
        comment="Importance score 1-10",
    )
    source_timestamp = Column(
        DateTime(timezone=True),
        nullable=False,
        comment="Original content timestamp",
    )

    # Review tracking
    reviewed_at = Column(
        DateTime(timezone=True),
        nullable=True,
        comment="When reviewed",
    )
    review_duration_ms = Column(
        Integer,
        nullable=True,
        comment="Time spent on review",
    )
    user_decision = Column(
        String(20),
        nullable=True,
        comment="User decision (accept, reject, modify, skip)",
    )
    user_correction = Column(
        JSONB,
        nullable=True,
        comment="User's correction if modified",
    )

    # Batch and undo support
    batch_id = Column(
        UUID(as_uuid=True),
        nullable=True,
        comment="If part of batch operation",
    )
    undo_eligible = Column(
        Boolean,
        nullable=False,
        server_default="true",
        comment="Can be undone",
    )
    undo_deadline = Column(
        DateTime(timezone=True),
        nullable=True,
        comment="Deadline for undo",
    )

    # Relationships
    session = relationship(
        "ReviewSessionModel", back_populates="review_items", lazy="select"
    )
    source = relationship("Source", lazy="select")
    feedback = relationship(
        "UserFeedbackModel", back_populates="review_item", lazy="select"
    )

    __table_args__ = (
        Index("idx_review_item_session", "session_id", "queue_position"),
        Index("idx_review_item_status", "tenant_id", "status", "ai_confidence"),
        Index("idx_review_item_source", "source_id"),
        Index(
            "idx_review_item_batch",
            "batch_id",
            postgresql_where=text("batch_id IS NOT NULL"),
        ),
        Index(
            "idx_review_item_undo",
            "undo_eligible",
            "undo_deadline",
            postgresql_where=text("undo_eligible = TRUE"),
        ),
        CheckConstraint(
            "status IN ('pending', 'in_review', 'accepted', 'rejected', "
            "'modified', 'skipped')",
            name="ck_review_item_status",
        ),
        CheckConstraint(
            "content_type IN ('email', 'meeting', 'document')",
            name="ck_review_item_content_type",
        ),
        CheckConstraint(
            "ai_confidence >= 0.000 AND ai_confidence <= 1.000",
            name="ck_review_item_confidence",
        ),
        CheckConstraint(
            "business_importance >= 1 AND business_importance <= 10",
            name="ck_review_item_importance",
        ),
        CheckConstraint(
            "user_decision IS NULL OR user_decision IN "
            "('accept', 'reject', 'modify', 'skip')",
            name="ck_review_item_decision",
        ),
    )

    def __repr__(self):
        return (
            f"<ReviewItemModel(id={self.id}, status='{self.status}', "
            f"type='{self.content_type}')>"
        )


class UserFeedbackModel(Base, TenantMixin):
    """User feedback on AI suggestions.

    Captures user validation decisions for learning system integration,
    including decision type, corrections, timing, and batch context.
    """

    __tablename__ = "user_feedback"

    id = Column(
        BIGINT,
        primary_key=True,
        autoincrement=True,
        comment="Feedback identifier",
    )
    feedback_uuid = Column(
        UUID(as_uuid=True),
        nullable=False,
        unique=True,
        default=uuid.uuid4,
        comment="External feedback reference",
    )
    review_item_id = Column(
        BIGINT,
        ForeignKey("review_items.id"),
        nullable=False,
        comment="Associated review item",
    )
    session_id = Column(
        BIGINT,
        ForeignKey("review_sessions.id"),
        nullable=False,
        comment="Session context",
    )

    # Decision details
    decision_type = Column(
        String(20),
        nullable=False,
        comment="Decision type (accept, reject, modify)",
    )
    original_suggestion = Column(
        JSONB,
        nullable=False,
        comment="AI's original suggestion",
    )
    user_correction = Column(
        JSONB,
        nullable=True,
        comment="User's correction (for modify)",
    )
    correction_reason = Column(
        String(100),
        nullable=True,
        comment="Reason code for correction",
    )
    correction_notes = Column(
        Text,
        nullable=True,
        comment="Free-form explanation",
    )
    confidence_feedback = Column(
        String(20),
        nullable=True,
        comment="Confidence feedback (too_high, accurate, too_low)",
    )

    # Timing
    time_spent_ms = Column(
        Integer,
        nullable=False,
        comment="Time user spent on decision",
    )

    # Batch context
    was_batch_decision = Column(
        Boolean,
        nullable=False,
        server_default="false",
        comment="Part of batch operation",
    )
    batch_id = Column(
        UUID(as_uuid=True),
        nullable=True,
        comment="Batch operation reference",
    )

    # Learning integration
    learning_rule_id = Column(
        BIGINT,
        ForeignKey("learning_rules.id"),
        nullable=True,
        comment="Generated learning rule",
    )
    learning_signal_quality = Column(
        DECIMAL(3, 2),
        nullable=True,
        comment="Quality score (0.00-1.00)",
    )

    # Timestamp
    created_at = Column(
        DateTime(timezone=True),
        nullable=False,
        server_default=func.now(),
        comment="Record creation",
    )

    # Relationships
    review_item = relationship(
        "ReviewItemModel", back_populates="feedback", lazy="select"
    )
    session = relationship(
        "ReviewSessionModel", back_populates="user_feedback", lazy="select"
    )
    learning_rule = relationship(
        "LearningRuleModel", back_populates="feedback", lazy="select"
    )

    __table_args__ = (
        Index("idx_user_feedback_session", "session_id", "created_at"),
        Index("idx_user_feedback_item", "review_item_id"),
        Index(
            "idx_user_feedback_learning",
            "tenant_id",
            "decision_type",
            "correction_reason",
        ),
        Index(
            "idx_user_feedback_batch",
            "batch_id",
            postgresql_where=text("batch_id IS NOT NULL"),
        ),
        CheckConstraint(
            "decision_type IN ('accept', 'reject', 'modify', 'skip')",
            name="ck_user_feedback_decision_type",
        ),
        CheckConstraint(
            "confidence_feedback IS NULL OR confidence_feedback IN "
            "('too_high', 'accurate', 'too_low')",
            name="ck_user_feedback_confidence",
        ),
        CheckConstraint(
            "learning_signal_quality IS NULL OR "
            "(learning_signal_quality >= 0.00 AND learning_signal_quality <= 1.00)",
            name="ck_user_feedback_signal_quality",
        ),
    )

    def __repr__(self):
        return f"<UserFeedbackModel(id={self.id}, decision='{self.decision_type}')>"


class LearningRuleModel(
    Base, TimestampMixin, TenantMixin, SoftDeleteMixin, SoftDeleteQueryMixin
):
    """Automated decision patterns derived from user feedback.

    Represents rules for automatic categorization based on learned patterns,
    with effectiveness tracking and soft delete support.
    """

    __tablename__ = "learning_rules"

    id = Column(
        BIGINT,
        primary_key=True,
        autoincrement=True,
        comment="Rule identifier",
    )
    rule_uuid = Column(
        UUID(as_uuid=True),
        nullable=False,
        unique=True,
        default=uuid.uuid4,
        comment="External rule reference",
    )

    # Rule definition
    name = Column(
        String(200),
        nullable=False,
        comment="Human-readable rule name",
    )
    description = Column(
        Text,
        nullable=True,
        comment="Rule description",
    )
    status = Column(
        String(20),
        nullable=False,
        server_default="active",
        comment="Rule status (active, disabled, expired)",
    )
    rule_type = Column(
        String(50),
        nullable=False,
        comment="Type of rule",
    )
    match_criteria = Column(
        JSONB,
        nullable=False,
        comment="Conditions for rule match",
    )
    action = Column(
        JSONB,
        nullable=False,
        comment="Action to apply when matched",
    )

    # Thresholds
    confidence_threshold = Column(
        DECIMAL(4, 3),
        nullable=False,
        server_default="0.800",
        comment="Min confidence to apply (0.000-1.000)",
    )
    priority = Column(
        Integer,
        nullable=False,
        server_default="100",
        comment="Lower = higher priority (1-1000)",
    )

    # Statistics
    derived_from_count = Column(
        Integer,
        nullable=False,
        server_default="0",
        comment="Number of feedback items that created rule",
    )
    times_applied = Column(
        Integer,
        nullable=False,
        server_default="0",
        comment="Application count",
    )
    times_overridden = Column(
        Integer,
        nullable=False,
        server_default="0",
        comment="User override count",
    )
    effectiveness_score = Column(
        DECIMAL(4, 3),
        nullable=True,
        comment="Calculated effectiveness (0.000-1.000)",
    )
    last_applied_at = Column(
        DateTime(timezone=True),
        nullable=True,
        comment="Last application time",
    )

    # Lifecycle
    expires_at = Column(
        DateTime(timezone=True),
        nullable=True,
        comment="Optional expiration",
    )
    created_by = Column(
        String(254),
        nullable=False,
        comment="User who created rule",
    )

    # Relationships
    feedback = relationship(
        "UserFeedbackModel", back_populates="learning_rule", lazy="select"
    )

    __table_args__ = (
        Index("idx_learning_rule_tenant_status", "tenant_id", "status", "priority"),
        Index("idx_learning_rule_type", "rule_type", "status"),
        Index(
            "idx_learning_rule_effectiveness",
            "effectiveness_score",
            postgresql_where=text("status = 'active'"),
        ),
        Index("idx_learning_rule_soft_delete", "is_deleted", "deleted_at"),
        CheckConstraint(
            "status IN ('active', 'disabled', 'expired')",
            name="ck_learning_rule_status",
        ),
        CheckConstraint(
            "rule_type IN ('sender_match', 'content_pattern', "
            "'category_override', 'participant_mapping')",
            name="ck_learning_rule_type",
        ),
        CheckConstraint(
            "confidence_threshold >= 0.000 AND confidence_threshold <= 1.000",
            name="ck_learning_rule_confidence_threshold",
        ),
        CheckConstraint(
            "priority >= 1 AND priority <= 1000",
            name="ck_learning_rule_priority",
        ),
        CheckConstraint(
            "effectiveness_score IS NULL OR "
            "(effectiveness_score >= 0.000 AND effectiveness_score <= 1.000)",
            name="ck_learning_rule_effectiveness",
        ),
    )

    def __repr__(self):
        return (
            f"<LearningRuleModel(id={self.id}, name='{self.name}', "
            f"status='{self.status}')>"
        )


class BatchOperationModel(Base, TimestampMixin, TenantMixin):
    """Batch operation records for bulk review actions.

    Groups related review items for bulk actions with confirmation
    and undo support.
    """

    __tablename__ = "batch_operations"

    id = Column(
        BIGINT,
        primary_key=True,
        autoincrement=True,
        comment="Batch identifier",
    )
    batch_uuid = Column(
        UUID(as_uuid=True),
        nullable=False,
        unique=True,
        default=uuid.uuid4,
        comment="External batch reference",
    )
    session_id = Column(
        BIGINT,
        ForeignKey("review_sessions.id"),
        nullable=False,
        comment="Parent session",
    )

    # Batch definition
    batch_type = Column(
        String(50),
        nullable=False,
        comment="Grouping type (thread, sender, category, time_window, custom)",
    )
    group_criteria = Column(
        JSONB,
        nullable=False,
        comment="How items were grouped",
    )
    action_type = Column(
        String(20),
        nullable=False,
        comment="Action to apply (accept_all, reject_all, apply_category, skip_all)",
    )
    action_details = Column(
        JSONB,
        nullable=False,
        comment="Action parameters",
    )
    item_count = Column(
        Integer,
        nullable=False,
        comment="Number of items in batch",
    )

    # Status tracking
    status = Column(
        String(20),
        nullable=False,
        server_default="pending",
        comment="Operation status (pending, confirmed, applied, undone)",
    )
    confirmed_at = Column(
        DateTime(timezone=True),
        nullable=True,
        comment="User confirmation time",
    )
    applied_at = Column(
        DateTime(timezone=True),
        nullable=True,
        comment="Action application time",
    )
    undone_at = Column(
        DateTime(timezone=True),
        nullable=True,
        comment="Undo time if reversed",
    )

    # Undo support
    undo_eligible = Column(
        Boolean,
        nullable=False,
        server_default="true",
        comment="Can be undone",
    )
    undo_deadline = Column(
        DateTime(timezone=True),
        nullable=True,
        comment="Deadline for undo",
    )

    # Relationships
    session = relationship(
        "ReviewSessionModel", back_populates="batch_operations", lazy="select"
    )

    __table_args__ = (
        Index("idx_batch_session", "session_id", "created_at"),
        Index("idx_batch_status", "tenant_id", "status", "created_at"),
        Index(
            "idx_batch_undo",
            "undo_eligible",
            "undo_deadline",
            postgresql_where=text("undo_eligible = TRUE"),
        ),
        CheckConstraint(
            "batch_type IN ('thread', 'sender', 'category', 'time_window', 'custom')",
            name="ck_batch_operation_type",
        ),
        CheckConstraint(
            "action_type IN ('accept_all', 'reject_all', 'apply_category', 'skip_all')",
            name="ck_batch_operation_action_type",
        ),
        CheckConstraint(
            "status IN ('pending', 'confirmed', 'applied', 'undone')",
            name="ck_batch_operation_status",
        ),
        CheckConstraint(
            "item_count > 0",
            name="ck_batch_operation_item_count",
        ),
    )

    def __repr__(self):
        return (
            f"<BatchOperationModel(id={self.id}, type='{self.batch_type}', "
            f"status='{self.status}')>"
        )


class ReviewAnalyticsModel(Base, TenantMixin):
    """Aggregated metrics for review patterns and system learning progress (P3).

    Stores periodic analytics snapshots for tracking review velocity,
    AI accuracy, and learning system effectiveness.
    """

    __tablename__ = "review_analytics"

    id = Column(
        BIGINT,
        primary_key=True,
        autoincrement=True,
        comment="Analytics identifier",
    )

    # Period - using Date type to match migration
    period_start = Column(
        DateTime(timezone=True),
        nullable=False,
        comment="Analytics period start",
    )
    period_end = Column(
        DateTime(timezone=True),
        nullable=False,
        comment="Analytics period end",
    )
    period_type = Column(
        String(20),
        nullable=False,
        comment="Period granularity (daily, weekly, monthly)",
    )

    # Session metrics
    total_sessions = Column(
        Integer,
        nullable=False,
        comment="Session count",
    )
    total_items_reviewed = Column(
        Integer,
        nullable=False,
        comment="Items reviewed",
    )
    avg_session_duration_min = Column(
        DECIMAL(6, 2),
        nullable=False,
        comment="Average session length in minutes",
    )
    avg_items_per_minute = Column(
        DECIMAL(5, 2),
        nullable=False,
        comment="Review velocity",
    )

    # Decision breakdown
    acceptance_rate = Column(
        DECIMAL(5, 4),
        nullable=False,
        comment="Accept percentage",
    )
    modification_rate = Column(
        DECIMAL(5, 4),
        nullable=False,
        comment="Modify percentage",
    )
    rejection_rate = Column(
        DECIMAL(5, 4),
        nullable=False,
        comment="Reject percentage",
    )
    skip_rate = Column(
        DECIMAL(5, 4),
        nullable=False,
        comment="Skip percentage",
    )

    # AI performance
    avg_ai_confidence = Column(
        DECIMAL(4, 3),
        nullable=False,
        comment="Mean confidence score",
    )
    ai_accuracy = Column(
        DECIMAL(5, 4),
        nullable=True,
        comment="Calculated AI accuracy",
    )

    # Feature usage
    batch_usage_rate = Column(
        DECIMAL(5, 4),
        nullable=False,
        comment="Batch operation frequency",
    )
    learning_rules_created = Column(
        Integer,
        nullable=False,
        comment="New rules count",
    )
    learning_rules_applied = Column(
        Integer,
        nullable=False,
        comment="Rule applications",
    )

    # Breakdowns
    content_type_breakdown = Column(
        JSONB,
        nullable=False,
        comment="By content type metrics",
    )
    correction_type_breakdown = Column(
        JSONB,
        nullable=False,
        comment="By correction type",
    )

    # Timestamp
    created_at = Column(
        DateTime(timezone=True),
        nullable=False,
        server_default=func.now(),
        comment="Record creation",
    )

    __table_args__ = (
        Index(
            "idx_analytics_tenant_period",
            "tenant_id",
            "period_type",
            "period_start",
        ),
        CheckConstraint(
            "period_type IN ('daily', 'weekly', 'monthly')",
            name="ck_review_analytics_period_type",
        ),
        CheckConstraint(
            "period_end >= period_start",
            name="ck_review_analytics_period",
        ),
    )

    def __repr__(self):
        return (
            f"<ReviewAnalyticsModel(id={self.id}, period='{self.period_type}', "
            f"start='{self.period_start}')>"
        )



# =============================================================================
# RELATIONSHIP DISCOVERY MODELS
# =============================================================================


class Relationship(Base, TimestampMixin, TenantMixin):
    """
    Core entity representing a discovered connection between two entities.

    Relationships are automatically discovered from content analysis and can be
    validated by users. They evolve through lifecycle states from pending to
    active, historical, and archived.
    """

    __tablename__ = "relationships"

    id = Column(BIGINT, primary_key=True)

    # Entity references
    source_entity_type = Column(
        String(50),
        nullable=False,
        comment="Type of source entity (person, project, topic)",
    )
    source_entity_id = Column(
        BIGINT,
        nullable=False,
        comment="ID of the source entity",
    )
    target_entity_type = Column(
        String(50),
        nullable=False,
        comment="Type of target entity (person, project, topic)",
    )
    target_entity_id = Column(
        BIGINT,
        nullable=False,
        comment="ID of the target entity",
    )

    # Relationship classification
    relationship_type = Column(
        String(100),
        nullable=False,
        comment="Type of relationship (works_on, manages, collaborates_with)",
    )
    relationship_subtype = Column(
        String(100),
        nullable=True,
        comment="Optional subtype for finer classification",
    )

    # Confidence and scoring
    confidence_score = Column(
        DECIMAL(4, 3),
        nullable=False,
        comment="Overall confidence score (0.000-1.000)",
    )
    ai_confidence = Column(
        DECIMAL(4, 3),
        nullable=True,
        comment="AI extraction confidence",
    )
    evidence_strength = Column(
        DECIMAL(4, 3),
        nullable=True,
        comment="Evidence-based strength score",
    )

    # Lifecycle management
    lifecycle_state = Column(
        String(20),
        nullable=False,
        default="pending",
        comment="State: pending, active, historical, archived",
    )
    first_observed_at = Column(
        DateTime(timezone=True),
        nullable=False,
        server_default=func.now(),
        comment="When relationship was first discovered",
    )
    last_evidence_at = Column(
        DateTime(timezone=True),
        nullable=False,
        server_default=func.now(),
        comment="Most recent evidence timestamp",
    )
    archived_at = Column(
        DateTime(timezone=True),
        nullable=True,
        comment="When relationship was archived",
    )

    # Interaction tracking
    interaction_count = Column(
        Integer,
        default=0,
        nullable=False,
        comment="Number of interactions supporting this relationship",
    )

    # User validation
    user_confirmed = Column(
        Boolean,
        default=False,
        nullable=False,
        comment="Whether user has validated this relationship",
    )

    # Relationships
    evidence = orm_relationship("RelationshipEvidence", back_populates="relationship", lazy="select")
    feedback = orm_relationship("RelationshipFeedback", back_populates="relationship", lazy="select")
    versions = orm_relationship("RelationshipVersion", back_populates="relationship", lazy="select")
    conflicts = orm_relationship(
        "RelationshipConflict",
        foreign_keys="RelationshipConflict.relationship_id",
        back_populates="relationship",
        lazy="select",
    )

    # Validation methods
    @validates('source_entity_type', 'target_entity_type')
    def validate_entity_type(self, key, value):
        """Validate entity type is from allowed list."""
        valid_types = {'person', 'project', 'topic'}
        if value not in valid_types:
            raise ValidationError(f"Invalid {key}: {value}. Must be one of {valid_types}")
        return value

    @validates('lifecycle_state')
    def validate_lifecycle_state(self, key, value):
        """Validate lifecycle state is from allowed list."""
        valid_states = {'pending', 'active', 'historical', 'archived'}
        if value not in valid_states:
            raise ValidationError(f"Invalid lifecycle state: {value}. Must be one of {valid_states}")
        return value

    @validates('confidence_score', 'ai_confidence', 'evidence_strength')
    def validate_confidence_values(self, key, value):
        """Validate confidence scores are in valid range."""
        if value is not None:
            validate_confidence_score(value)
        return value

    def __repr__(self):
        return (
            f"<Relationship(id={self.id}, "
            f"type='{self.relationship_type}', "
            f"source={self.source_entity_type}:{self.source_entity_id}, "
            f"target={self.target_entity_type}:{self.target_entity_id})>"
        )

    __table_args__ = (
        Index("idx_rel_tenant_entities", "tenant_id", "source_entity_type", "source_entity_id",
              "target_entity_type", "target_entity_id"),
        Index("idx_rel_source_entity", "source_entity_type", "source_entity_id"),
        Index("idx_rel_target_entity", "target_entity_type", "target_entity_id"),
        Index("idx_rel_type", "relationship_type", "relationship_subtype"),
        Index("idx_rel_confidence", "confidence_score"),
        Index("idx_rel_lifecycle", "lifecycle_state", "last_evidence_at"),
        Index("idx_rel_temporal", "first_observed_at", "last_evidence_at"),
        UniqueConstraint(
            "tenant_id", "source_entity_type", "source_entity_id",
            "target_entity_type", "target_entity_id", "relationship_type",
            name="uq_rel_pair"
        ),
        CheckConstraint(
            "confidence_score >= 0.000 AND confidence_score <= 1.000",
            name="ck_rel_confidence_range"
        ),
        CheckConstraint(
            "lifecycle_state IN ('pending', 'active', 'historical', 'archived')",
            name="ck_rel_lifecycle_valid"
        ),
        CheckConstraint(
            "source_entity_type IN ('person', 'project', 'topic')",
            name="ck_rel_source_entity_type"
        ),
        CheckConstraint(
            "target_entity_type IN ('person', 'project', 'topic')",
            name="ck_rel_target_entity_type"
        ),
    )


class RelationshipEvidence(Base, TimestampMixin, TenantMixin):
    """
    Supporting evidence that justifies a relationship's existence.

    Evidence is collected from content analysis (emails, meetings, documents)
    and contributes to the overall confidence score of a relationship.
    """

    __tablename__ = "relationship_evidence"

    id = Column(BIGINT, primary_key=True)

    # Parent relationship
    relationship_id = Column(
        BIGINT,
        ForeignKey("relationships.id", ondelete="CASCADE"),
        nullable=False,
        comment="Parent relationship this evidence supports",
    )

    # Source content reference
    source_id = Column(
        BIGINT,
        ForeignKey("sources.id", ondelete="SET NULL"),
        nullable=True,
        comment="Reference to source content",
    )

    # Evidence classification
    evidence_type = Column(
        String(50),
        nullable=False,
        comment="Type: email, meeting, mention, inference, user_input",
    )
    evidence_content = Column(
        Text,
        nullable=True,
        comment="Extracted evidence text",
    )
    evidence_context = Column(
        JSONB,
        nullable=True,
        default={},
        comment="Additional context metadata",
    )

    # Confidence contribution
    strength_contribution = Column(
        DECIMAL(4, 3),
        nullable=False,
        comment="How much this evidence contributes to confidence (0.000-1.000)",
    )

    # Temporal tracking
    observed_at = Column(
        DateTime(timezone=True),
        nullable=False,
        server_default=func.now(),
        comment="When this evidence was observed",
    )
    expires_at = Column(
        DateTime(timezone=True),
        nullable=False,
        server_default=text("NOW() + INTERVAL '2 years'"),
        comment="Retention expiry (2 years default)",
    )

    # Relationships
    relationship = orm_relationship("Relationship", back_populates="evidence", lazy="select")
    source = orm_relationship("Source", lazy="select")

    # Validation
    @validates('evidence_type')
    def validate_evidence_type(self, key, value):
        """Validate evidence type is from allowed list."""
        valid_types = {'email', 'meeting', 'mention', 'inference', 'user_input'}
        if value not in valid_types:
            raise ValidationError(f"Invalid evidence type: {value}. Must be one of {valid_types}")
        return value

    @validates('strength_contribution')
    def validate_strength(self, key, value):
        """Validate strength contribution is in valid range."""
        validate_confidence_score(value)
        return value

    def __repr__(self):
        return f"<RelationshipEvidence(id={self.id}, type='{self.evidence_type}', rel_id={self.relationship_id})>"

    __table_args__ = (
        Index("idx_evidence_relationship", "relationship_id", "observed_at"),
        Index("idx_evidence_source", "source_id"),
        Index("idx_evidence_expiry", "expires_at"),
        Index("idx_evidence_type", "evidence_type", "observed_at"),
        CheckConstraint(
            "strength_contribution >= 0.000 AND strength_contribution <= 1.000",
            name="ck_evidence_strength_range"
        ),
        CheckConstraint(
            "evidence_type IN ('email', 'meeting', 'mention', 'inference', 'user_input')",
            name="ck_evidence_type_valid"
        ),
    )


class RelationshipFeedback(Base, TimestampMixin, TenantMixin):
    """
    User validation and feedback for relationships.

    Feedback is used to improve relationship discovery accuracy and
    allows users to confirm, reject, or modify discovered relationships.
    """

    __tablename__ = "relationship_feedback"

    id = Column(BIGINT, primary_key=True)

    # Target relationship
    relationship_id = Column(
        BIGINT,
        ForeignKey("relationships.id", ondelete="CASCADE"),
        nullable=False,
        comment="Relationship this feedback applies to",
    )

    # Feedback classification
    feedback_type = Column(
        String(20),
        nullable=False,
        comment="Type: confirm, reject, modify",
    )

    # Original values for learning
    original_type = Column(
        String(100),
        nullable=True,
        comment="Original relationship type",
    )
    corrected_type = Column(
        String(100),
        nullable=True,
        comment="User-corrected relationship type",
    )
    original_confidence = Column(
        DECIMAL(4, 3),
        nullable=True,
        comment="Original confidence score",
    )

    # User explanation
    reasoning = Column(
        Text,
        nullable=True,
        comment="User explanation for feedback",
    )
    feedback_metadata = Column(
        JSONB,
        nullable=True,
        default={},
        comment="Additional feedback data",
    )

    # Attribution
    user_email = Column(
        String(254),
        nullable=False,
        comment="User who provided feedback",
    )

    # Relationships
    relationship = orm_relationship("Relationship", back_populates="feedback", lazy="select")

    # Validation
    @validates('feedback_type')
    def validate_feedback_type(self, key, value):
        """Validate feedback type is from allowed list."""
        valid_types = {'confirm', 'reject', 'modify'}
        if value not in valid_types:
            raise ValidationError(f"Invalid feedback type: {value}. Must be one of {valid_types}")
        return value

    @validates('user_email')
    def validate_user_email(self, key, value):
        """Validate user email format."""
        from .validators import validate_email
        validate_email(value)
        return value.lower().strip()

    def __repr__(self):
        return f"<RelationshipFeedback(id={self.id}, type='{self.feedback_type}', rel_id={self.relationship_id})>"

    __table_args__ = (
        Index("idx_feedback_relationship", "relationship_id", "created_at"),
        Index("idx_feedback_type", "feedback_type", "created_at"),
        Index("idx_feedback_user", "user_email", "created_at"),
        CheckConstraint(
            "feedback_type IN ('confirm', 'reject', 'modify')",
            name="ck_feedback_type_valid"
        ),
    )


class RelationshipVersion(Base, TimestampMixin, TenantMixin):
    """
    Historical tracking of relationship changes.

    Maintains a complete history of changes to relationships for
    auditing, analysis, and potential rollback.
    """

    __tablename__ = "relationship_versions"

    id = Column(BIGINT, primary_key=True)

    # Target relationship
    relationship_id = Column(
        BIGINT,
        ForeignKey("relationships.id", ondelete="CASCADE"),
        nullable=False,
        comment="Relationship this version belongs to",
    )

    # Version tracking
    version_number = Column(
        Integer,
        nullable=False,
        comment="Sequential version number",
    )

    # State at this version
    relationship_type = Column(
        String(100),
        nullable=False,
        comment="Relationship type at this version",
    )
    relationship_subtype = Column(
        String(100),
        nullable=True,
        comment="Subtype at this version",
    )
    confidence_score = Column(
        DECIMAL(4, 3),
        nullable=False,
        comment="Confidence score at this version",
    )
    lifecycle_state = Column(
        String(20),
        nullable=False,
        comment="Lifecycle state at this version",
    )

    # Change tracking
    change_reason = Column(
        String(100),
        nullable=False,
        comment="What triggered this change",
    )
    change_details = Column(
        JSONB,
        nullable=True,
        default={},
        comment="Detailed change information",
    )

    # Temporal validity
    valid_from = Column(
        DateTime(timezone=True),
        nullable=False,
        server_default=func.now(),
        comment="When this version became active",
    )
    valid_to = Column(
        DateTime(timezone=True),
        nullable=True,
        comment="When this version was superseded (NULL = current)",
    )

    # Relationships
    relationship = orm_relationship("Relationship", back_populates="versions", lazy="select")

    # Validation
    @validates('change_reason')
    def validate_change_reason(self, key, value):
        """Validate change reason is from allowed list."""
        valid_reasons = {
            'discovery', 'evidence_update', 'user_feedback',
            'confidence_recalc', 'lifecycle_transition', 'conflict_resolution'
        }
        if value not in valid_reasons:
            raise ValidationError(f"Invalid change reason: {value}. Must be one of {valid_reasons}")
        return value

    def __repr__(self):
        return f"<RelationshipVersion(id={self.id}, rel_id={self.relationship_id}, v={self.version_number})>"

    __table_args__ = (
        Index("idx_version_relationship", "relationship_id", "version_number"),
        Index("idx_version_temporal", "relationship_id", "valid_from", "valid_to"),
        Index("idx_version_reason", "change_reason"),
        UniqueConstraint(
            "relationship_id", "version_number",
            name="uq_version_number"
        ),
        CheckConstraint(
            "change_reason IN ('discovery', 'evidence_update', 'user_feedback', "
            "'confidence_recalc', 'lifecycle_transition', 'conflict_resolution')",
            name="ck_version_change_reason"
        ),
    )


class RelationshipConflict(Base, TimestampMixin, TenantMixin):
    """
    Pending conflicts between discovered relationships.

    Conflicts are detected when incompatible relationships are discovered
    and require resolution, either automatically or by user intervention.
    """

    __tablename__ = "relationship_conflicts"

    id = Column(BIGINT, primary_key=True)

    # Primary relationship
    relationship_id = Column(
        BIGINT,
        ForeignKey("relationships.id", ondelete="CASCADE"),
        nullable=False,
        comment="Primary relationship in conflict",
    )

    # Competing relationship
    conflicting_relationship_id = Column(
        BIGINT,
        ForeignKey("relationships.id", ondelete="SET NULL"),
        nullable=True,
        comment="Conflicting relationship",
    )

    # Conflict classification
    conflict_type = Column(
        String(50),
        nullable=False,
        comment="Type: type_mismatch, duplicate, contradictory, circular",
    )

    # Confidence comparison
    primary_confidence = Column(
        DECIMAL(4, 3),
        nullable=False,
        comment="Primary relationship confidence",
    )
    secondary_confidence = Column(
        DECIMAL(4, 3),
        nullable=True,
        comment="Conflicting relationship confidence",
    )
    confidence_gap = Column(
        DECIMAL(4, 3),
        nullable=False,
        comment="Absolute difference in confidence",
    )

    # Resolution tracking
    auto_resolvable = Column(
        Boolean,
        nullable=False,
        comment="Whether gap > 30% allows auto-resolution",
    )
    resolution_status = Column(
        String(20),
        nullable=False,
        default="pending",
        comment="Status: pending, auto_resolved, user_resolved, deferred",
    )
    resolution_details = Column(
        JSONB,
        nullable=True,
        default={},
        comment="Resolution information",
    )
    resolved_at = Column(
        DateTime(timezone=True),
        nullable=True,
        comment="When conflict was resolved",
    )
    resolved_by = Column(
        String(254),
        nullable=True,
        comment="Who resolved: user email or 'system'",
    )

    # Relationships
    relationship = orm_relationship(
        "Relationship",
        foreign_keys=[relationship_id],
        back_populates="conflicts",
        lazy="select",
    )
    conflicting_relationship = orm_relationship(
        "Relationship",
        foreign_keys=[conflicting_relationship_id],
        lazy="select",
    )

    # Validation
    @validates('conflict_type')
    def validate_conflict_type(self, key, value):
        """Validate conflict type is from allowed list."""
        valid_types = {'type_mismatch', 'duplicate', 'contradictory', 'circular'}
        if value not in valid_types:
            raise ValidationError(f"Invalid conflict type: {value}. Must be one of {valid_types}")
        return value

    @validates('resolution_status')
    def validate_resolution_status(self, key, value):
        """Validate resolution status is from allowed list."""
        valid_statuses = {'pending', 'auto_resolved', 'user_resolved', 'deferred'}
        if value not in valid_statuses:
            raise ValidationError(f"Invalid resolution status: {value}. Must be one of {valid_statuses}")
        return value

    def __repr__(self):
        return f"<RelationshipConflict(id={self.id}, type='{self.conflict_type}', status='{self.resolution_status}')>"

    __table_args__ = (
        Index("idx_conflict_status", "resolution_status", "created_at"),
        Index("idx_conflict_relationship", "relationship_id"),
        Index("idx_conflict_resolvable", "auto_resolvable", "resolution_status"),
        CheckConstraint(
            "conflict_type IN ('type_mismatch', 'duplicate', 'contradictory', 'circular')",
            name="ck_conflict_type_valid"
        ),
        CheckConstraint(
            "resolution_status IN ('pending', 'auto_resolved', 'user_resolved', 'deferred')",
            name="ck_conflict_status_valid"
        ),
        CheckConstraint(
            "confidence_gap >= 0.000 AND confidence_gap <= 1.000",
            name="ck_conflict_gap_range"
        ),
    )