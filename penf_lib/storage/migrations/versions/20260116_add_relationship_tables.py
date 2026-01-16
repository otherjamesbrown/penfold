"""Add relationship discovery tables

Revision ID: 003
Revises: 002
Create Date: 2026-01-16 10:00:00.000000

This migration adds the foundational tables for relationship discovery:
- relationships: Core relationship connections between entities
- relationship_evidence: Supporting evidence for relationships
- relationship_feedback: User validation and feedback
- relationship_versions: Historical change tracking
- relationship_conflicts: Conflict detection and resolution
"""
from typing import Sequence, Union

from alembic import op
import sqlalchemy as sa
from sqlalchemy.dialects.postgresql import UUID, JSONB, BIGINT
from sqlalchemy.sql import text


# revision identifiers, used by Alembic.
revision: str = '003'
down_revision: Union[str, None] = '002'
branch_labels: Union[str, Sequence[str], None] = None
depends_on: Union[str, Sequence[str], None] = None


def upgrade() -> None:
    """Upgrade database schema with relationship discovery tables."""

    # =========================================================================
    # RELATIONSHIPS TABLE
    # =========================================================================
    op.create_table(
        'relationships',
        sa.Column('id', BIGINT(), primary_key=True, autoincrement=True, nullable=False),
        sa.Column('tenant_id', UUID(as_uuid=True), nullable=False),

        # Entity references
        sa.Column('source_entity_type', sa.String(length=50), nullable=False,
                  comment="Type of source entity (person, project, topic)"),
        sa.Column('source_entity_id', BIGINT(), nullable=False,
                  comment="ID of the source entity"),
        sa.Column('target_entity_type', sa.String(length=50), nullable=False,
                  comment="Type of target entity (person, project, topic)"),
        sa.Column('target_entity_id', BIGINT(), nullable=False,
                  comment="ID of the target entity"),

        # Relationship classification
        sa.Column('relationship_type', sa.String(length=100), nullable=False,
                  comment="Type of relationship (works_on, manages, collaborates_with)"),
        sa.Column('relationship_subtype', sa.String(length=100), nullable=True,
                  comment="Optional subtype for finer classification"),

        # Confidence and scoring
        sa.Column('confidence_score', sa.DECIMAL(precision=4, scale=3), nullable=False,
                  comment="Overall confidence score (0.000-1.000)"),
        sa.Column('ai_confidence', sa.DECIMAL(precision=4, scale=3), nullable=True,
                  comment="AI extraction confidence"),
        sa.Column('evidence_strength', sa.DECIMAL(precision=4, scale=3), nullable=True,
                  comment="Evidence-based strength score"),

        # Lifecycle management
        sa.Column('lifecycle_state', sa.String(length=20), nullable=False, server_default='pending',
                  comment="State: pending, active, historical, archived"),
        sa.Column('first_observed_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False,
                  comment="When relationship was first discovered"),
        sa.Column('last_evidence_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False,
                  comment="Most recent evidence timestamp"),
        sa.Column('archived_at', sa.DateTime(timezone=True), nullable=True,
                  comment="When relationship was archived"),

        # Interaction tracking
        sa.Column('interaction_count', sa.Integer(), nullable=False, server_default='0',
                  comment="Number of interactions supporting this relationship"),

        # User validation
        sa.Column('user_confirmed', sa.Boolean(), nullable=False, server_default='false',
                  comment="Whether user has validated this relationship"),

        # Timestamps
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),

        # Constraints
        sa.UniqueConstraint(
            'tenant_id', 'source_entity_type', 'source_entity_id',
            'target_entity_type', 'target_entity_id', 'relationship_type',
            name='uq_rel_pair'
        ),
        sa.CheckConstraint(
            "confidence_score >= 0.000 AND confidence_score <= 1.000",
            name="ck_rel_confidence_range"
        ),
        sa.CheckConstraint(
            "lifecycle_state IN ('pending', 'active', 'historical', 'archived')",
            name="ck_rel_lifecycle_valid"
        ),
        sa.CheckConstraint(
            "source_entity_type IN ('person', 'project', 'topic')",
            name="ck_rel_source_entity_type"
        ),
        sa.CheckConstraint(
            "target_entity_type IN ('person', 'project', 'topic')",
            name="ck_rel_target_entity_type"
        ),

        comment="Discovered connections between entities"
    )

    # Indexes for relationships
    op.create_index('idx_rel_tenant_entities', 'relationships',
                    ['tenant_id', 'source_entity_type', 'source_entity_id',
                     'target_entity_type', 'target_entity_id'])
    op.create_index('idx_rel_source_entity', 'relationships',
                    ['source_entity_type', 'source_entity_id'])
    op.create_index('idx_rel_target_entity', 'relationships',
                    ['target_entity_type', 'target_entity_id'])
    op.create_index('idx_rel_type', 'relationships',
                    ['relationship_type', 'relationship_subtype'])
    op.create_index('idx_rel_confidence', 'relationships', ['confidence_score'])
    op.create_index('idx_rel_lifecycle', 'relationships',
                    ['lifecycle_state', 'last_evidence_at'])
    op.create_index('idx_rel_temporal', 'relationships',
                    ['first_observed_at', 'last_evidence_at'])

    # =========================================================================
    # RELATIONSHIP_EVIDENCE TABLE
    # =========================================================================
    op.create_table(
        'relationship_evidence',
        sa.Column('id', BIGINT(), primary_key=True, autoincrement=True, nullable=False),
        sa.Column('tenant_id', UUID(as_uuid=True), nullable=False),

        # Parent relationship
        sa.Column('relationship_id', BIGINT(),
                  sa.ForeignKey('relationships.id', ondelete='CASCADE'),
                  nullable=False, comment="Parent relationship this evidence supports"),

        # Source content reference
        sa.Column('source_id', BIGINT(),
                  sa.ForeignKey('sources.id', ondelete='SET NULL'),
                  nullable=True, comment="Reference to source content"),

        # Evidence classification
        sa.Column('evidence_type', sa.String(length=50), nullable=False,
                  comment="Type: email, meeting, mention, inference, user_input"),
        sa.Column('evidence_content', sa.Text(), nullable=True,
                  comment="Extracted evidence text"),
        sa.Column('evidence_context', JSONB(), nullable=True, server_default='{}',
                  comment="Additional context metadata"),

        # Confidence contribution
        sa.Column('strength_contribution', sa.DECIMAL(precision=4, scale=3), nullable=False,
                  comment="How much this evidence contributes to confidence (0.000-1.000)"),

        # Temporal tracking
        sa.Column('observed_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False,
                  comment="When this evidence was observed"),
        sa.Column('expires_at', sa.DateTime(timezone=True),
                  server_default=text("NOW() + INTERVAL '2 years'"),
                  nullable=False, comment="Retention expiry (2 years default)"),

        # Timestamps
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),

        # Constraints
        sa.CheckConstraint(
            "strength_contribution >= 0.000 AND strength_contribution <= 1.000",
            name="ck_evidence_strength_range"
        ),
        sa.CheckConstraint(
            "evidence_type IN ('email', 'meeting', 'mention', 'inference', 'user_input')",
            name="ck_evidence_type_valid"
        ),

        comment="Supporting evidence that justifies relationship existence"
    )

    # Indexes for relationship_evidence
    op.create_index('idx_evidence_relationship', 'relationship_evidence',
                    ['relationship_id', 'observed_at'])
    op.create_index('idx_evidence_source', 'relationship_evidence', ['source_id'])
    op.create_index('idx_evidence_expiry', 'relationship_evidence', ['expires_at'])
    op.create_index('idx_evidence_type', 'relationship_evidence',
                    ['evidence_type', 'observed_at'])

    # =========================================================================
    # RELATIONSHIP_FEEDBACK TABLE
    # =========================================================================
    op.create_table(
        'relationship_feedback',
        sa.Column('id', BIGINT(), primary_key=True, autoincrement=True, nullable=False),
        sa.Column('tenant_id', UUID(as_uuid=True), nullable=False),

        # Target relationship
        sa.Column('relationship_id', BIGINT(),
                  sa.ForeignKey('relationships.id', ondelete='CASCADE'),
                  nullable=False, comment="Relationship this feedback applies to"),

        # Feedback classification
        sa.Column('feedback_type', sa.String(length=20), nullable=False,
                  comment="Type: confirm, reject, modify"),

        # Original values for learning
        sa.Column('original_type', sa.String(length=100), nullable=True,
                  comment="Original relationship type"),
        sa.Column('corrected_type', sa.String(length=100), nullable=True,
                  comment="User-corrected relationship type"),
        sa.Column('original_confidence', sa.DECIMAL(precision=4, scale=3), nullable=True,
                  comment="Original confidence score"),

        # User explanation
        sa.Column('reasoning', sa.Text(), nullable=True,
                  comment="User explanation for feedback"),
        sa.Column('feedback_metadata', JSONB(), nullable=True, server_default='{}',
                  comment="Additional feedback data"),

        # Attribution
        sa.Column('user_email', sa.String(length=254), nullable=False,
                  comment="User who provided feedback"),

        # Timestamps
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),

        # Constraints
        sa.CheckConstraint(
            "feedback_type IN ('confirm', 'reject', 'modify')",
            name="ck_feedback_type_valid"
        ),

        comment="User validation and feedback for relationships"
    )

    # Indexes for relationship_feedback
    op.create_index('idx_feedback_relationship', 'relationship_feedback',
                    ['relationship_id', 'created_at'])
    op.create_index('idx_feedback_type', 'relationship_feedback',
                    ['feedback_type', 'created_at'])
    op.create_index('idx_feedback_user', 'relationship_feedback',
                    ['user_email', 'created_at'])

    # =========================================================================
    # RELATIONSHIP_VERSIONS TABLE
    # =========================================================================
    op.create_table(
        'relationship_versions',
        sa.Column('id', BIGINT(), primary_key=True, autoincrement=True, nullable=False),
        sa.Column('tenant_id', UUID(as_uuid=True), nullable=False),

        # Target relationship
        sa.Column('relationship_id', BIGINT(),
                  sa.ForeignKey('relationships.id', ondelete='CASCADE'),
                  nullable=False, comment="Relationship this version belongs to"),

        # Version tracking
        sa.Column('version_number', sa.Integer(), nullable=False,
                  comment="Sequential version number"),

        # State at this version
        sa.Column('relationship_type', sa.String(length=100), nullable=False,
                  comment="Relationship type at this version"),
        sa.Column('relationship_subtype', sa.String(length=100), nullable=True,
                  comment="Subtype at this version"),
        sa.Column('confidence_score', sa.DECIMAL(precision=4, scale=3), nullable=False,
                  comment="Confidence score at this version"),
        sa.Column('lifecycle_state', sa.String(length=20), nullable=False,
                  comment="Lifecycle state at this version"),

        # Change tracking
        sa.Column('change_reason', sa.String(length=100), nullable=False,
                  comment="What triggered this change"),
        sa.Column('change_details', JSONB(), nullable=True, server_default='{}',
                  comment="Detailed change information"),

        # Temporal validity
        sa.Column('valid_from', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False,
                  comment="When this version became active"),
        sa.Column('valid_to', sa.DateTime(timezone=True), nullable=True,
                  comment="When this version was superseded (NULL = current)"),

        # Timestamps
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),

        # Constraints
        sa.UniqueConstraint('relationship_id', 'version_number', name='uq_version_number'),
        sa.CheckConstraint(
            "change_reason IN ('discovery', 'evidence_update', 'user_feedback', "
            "'confidence_recalc', 'lifecycle_transition', 'conflict_resolution')",
            name="ck_version_change_reason"
        ),

        comment="Historical tracking of relationship changes"
    )

    # Indexes for relationship_versions
    op.create_index('idx_version_relationship', 'relationship_versions',
                    ['relationship_id', 'version_number'])
    op.create_index('idx_version_temporal', 'relationship_versions',
                    ['relationship_id', 'valid_from', 'valid_to'])
    op.create_index('idx_version_reason', 'relationship_versions', ['change_reason'])

    # =========================================================================
    # RELATIONSHIP_CONFLICTS TABLE
    # =========================================================================
    op.create_table(
        'relationship_conflicts',
        sa.Column('id', BIGINT(), primary_key=True, autoincrement=True, nullable=False),
        sa.Column('tenant_id', UUID(as_uuid=True), nullable=False),

        # Primary relationship
        sa.Column('relationship_id', BIGINT(),
                  sa.ForeignKey('relationships.id', ondelete='CASCADE'),
                  nullable=False, comment="Primary relationship in conflict"),

        # Competing relationship
        sa.Column('conflicting_relationship_id', BIGINT(),
                  sa.ForeignKey('relationships.id', ondelete='SET NULL'),
                  nullable=True, comment="Conflicting relationship"),

        # Conflict classification
        sa.Column('conflict_type', sa.String(length=50), nullable=False,
                  comment="Type: type_mismatch, duplicate, contradictory, circular"),

        # Confidence comparison
        sa.Column('primary_confidence', sa.DECIMAL(precision=4, scale=3), nullable=False,
                  comment="Primary relationship confidence"),
        sa.Column('secondary_confidence', sa.DECIMAL(precision=4, scale=3), nullable=True,
                  comment="Conflicting relationship confidence"),
        sa.Column('confidence_gap', sa.DECIMAL(precision=4, scale=3), nullable=False,
                  comment="Absolute difference in confidence"),

        # Resolution tracking
        sa.Column('auto_resolvable', sa.Boolean(), nullable=False,
                  comment="Whether gap > 30% allows auto-resolution"),
        sa.Column('resolution_status', sa.String(length=20), nullable=False, server_default='pending',
                  comment="Status: pending, auto_resolved, user_resolved, deferred"),
        sa.Column('resolution_details', JSONB(), nullable=True, server_default='{}',
                  comment="Resolution information"),
        sa.Column('resolved_at', sa.DateTime(timezone=True), nullable=True,
                  comment="When conflict was resolved"),
        sa.Column('resolved_by', sa.String(length=254), nullable=True,
                  comment="Who resolved: user email or 'system'"),

        # Timestamps
        sa.Column('created_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),
        sa.Column('updated_at', sa.DateTime(timezone=True), server_default=sa.func.now(), nullable=False),

        # Constraints
        sa.CheckConstraint(
            "conflict_type IN ('type_mismatch', 'duplicate', 'contradictory', 'circular')",
            name="ck_conflict_type_valid"
        ),
        sa.CheckConstraint(
            "resolution_status IN ('pending', 'auto_resolved', 'user_resolved', 'deferred')",
            name="ck_conflict_status_valid"
        ),
        sa.CheckConstraint(
            "confidence_gap >= 0.000 AND confidence_gap <= 1.000",
            name="ck_conflict_gap_range"
        ),

        comment="Pending conflicts between discovered relationships"
    )

    # Indexes for relationship_conflicts
    op.create_index('idx_conflict_status', 'relationship_conflicts',
                    ['resolution_status', 'created_at'])
    op.create_index('idx_conflict_relationship', 'relationship_conflicts', ['relationship_id'])
    op.create_index('idx_conflict_resolvable', 'relationship_conflicts',
                    ['auto_resolvable', 'resolution_status'])

    # =========================================================================
    # ROW-LEVEL SECURITY
    # =========================================================================
    relationship_tables = [
        'relationships',
        'relationship_evidence',
        'relationship_feedback',
        'relationship_versions',
        'relationship_conflicts',
    ]

    for table_name in relationship_tables:
        # Enable RLS
        op.execute(f"ALTER TABLE {table_name} ENABLE ROW LEVEL SECURITY")

        # Create policy for tenant isolation
        op.execute(f"""
            CREATE POLICY tenant_isolation_policy ON {table_name}
            FOR ALL
            TO PUBLIC
            USING (
                tenant_id = COALESCE(
                    current_setting('app.current_tenant_id', true)::uuid,
                    '00000000-0000-0000-0000-000000000000'::uuid
                )
            )
        """)

        # Create policy for superuser access
        op.execute(f"""
            CREATE POLICY superuser_access_policy ON {table_name}
            FOR ALL
            TO PUBLIC
            USING (
                current_setting('app.current_tenant_id', true) IS NULL
                OR current_setting('app.current_tenant_id', true) = ''
                OR current_user = 'postgres'
            )
        """)


def downgrade() -> None:
    """Downgrade database schema by removing relationship tables."""

    relationship_tables = [
        'relationship_conflicts',
        'relationship_versions',
        'relationship_feedback',
        'relationship_evidence',
        'relationships',
    ]

    # Remove RLS policies
    for table_name in relationship_tables:
        op.execute(f"DROP POLICY IF EXISTS tenant_isolation_policy ON {table_name}")
        op.execute(f"DROP POLICY IF EXISTS superuser_access_policy ON {table_name}")

    # Drop tables in reverse order of creation
    op.drop_table('relationship_conflicts')
    op.drop_table('relationship_versions')
    op.drop_table('relationship_feedback')
    op.drop_table('relationship_evidence')
    op.drop_table('relationships')
